package api

import (
	"context"
	"encoding/base64"
	"html/template"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"uuid"

	"github.com/riverqueue/river"

	"github.com/productdevbook/yuva/api/internal/mail"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	unsubscribePathPrefix = "/u/"
	digestPosts           = 10
)

var unsubscribeTokenContext = []byte("yuva discussion unsubscribe v1")

type DiscussionDigestArgs struct {
	WorkspaceID  uuid.UUID `json:"workspace_id"`
	DiscussionID uuid.UUID `json:"discussion_id"`
	ContactID    uuid.UUID `json:"contact_id"`
}

func (DiscussionDigestArgs) Kind() string { return "discussion_digest" }

type discussionDigestWorker struct {
	river.WorkerDefaults[DiscussionDigestArgs]
	s *Server
}

func (w *discussionDigestWorker) Work(ctx context.Context, job *river.Job[DiscussionDigestArgs]) error {
	next, err := w.s.SendDiscussionDigest(ctx, job.Args)
	if err != nil || next == nil {
		return err
	}
	_, err = w.s.jobs.Insert(ctx, job.Args, &river.InsertOpts{ScheduledAt: *next})
	return err
}

func (s *Server) unsubscribeToken(a DiscussionDigestArgs) string {
	plain := make([]byte, 0, 48)
	plain = append(plain, a.WorkspaceID[:]...)
	plain = append(plain, a.DiscussionID[:]...)
	plain = append(plain, a.ContactID[:]...)
	return base64.RawURLEncoding.EncodeToString(s.secrets.Seal(plain, unsubscribeTokenContext))
}

func (s *Server) parseUnsubscribeToken(token string) (DiscussionDigestArgs, bool) {
	var a DiscussionDigestArgs
	if len(token) > 200 {
		return a, false
	}
	sealed, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return a, false
	}
	plain, err := s.secrets.Open(sealed, unsubscribeTokenContext)
	if err != nil || len(plain) != 48 {
		return a, false
	}
	copy(a.WorkspaceID[:], plain[:16])
	copy(a.DiscussionID[:], plain[16:32])
	copy(a.ContactID[:], plain[32:])
	return a, true
}

type digestPost struct{ Author, Text string }

func digestAuthor(p store.DiscussionPostView) string {
	a := discussionAuthor(p, false)
	if a.Name != nil {
		return *a.Name
	}
	return ""
}

// SendDiscussionDigest e-mails a follower the replies and the accepted answer of a thread since
// their last e-mail, at most once per delay. It returns when to try again, or nil.
func (s *Server) SendDiscussionDigest(ctx context.Context, a DiscussionDigestArgs) (*time.Time, error) {
	if live, err := s.workspaceLive(ctx, a.WorkspaceID); err != nil || !live {
		return nil, err
	}
	now, delay := s.now(), s.discussionDelay()
	var (
		next *time.Time
		msg  *mail.Message
	)
	err := s.inTx(ctx, a.WorkspaceID, func(q *store.Queries, _ *eventBatch) error {
		f, err := q.LockDiscussionFollow(ctx, store.LockDiscussionFollowParams{WorkspaceID: a.WorkspaceID, DiscussionID: a.DiscussionID, ContactID: a.ContactID})
		if store.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if f.EmailedAt != nil && f.EmailedAt.Add(delay).After(now) {
			at := f.EmailedAt.Add(delay)
			next = &at
			return nil
		}
		d, err := q.GetDiscussion(ctx, store.GetDiscussionParams{WorkspaceID: a.WorkspaceID, ID: a.DiscussionID})
		if err != nil || d.State != statePublished {
			return err
		}
		if d.Kind == kindIdea {
			board, err := q.GetRoadmapBoard(ctx, store.GetRoadmapBoardParams{WorkspaceID: a.WorkspaceID, ID: orZero(d.BoardID)})
			if err != nil || !board.Public {
				return err
			}
		}
		contact := a.ContactID
		posts, err := q.ListDigestPosts(ctx, store.ListDigestPostsParams{WorkspaceID: a.WorkspaceID, DiscussionID: d.ID, After: f.SeenThrough, ContactID: &contact})
		if err != nil {
			return err
		}
		seen := f.SeenThrough
		data := map[string]any{}
		var list []digestPost
		for _, p := range posts {
			seen = later(seen, &p.CreatedAt)
			if d.AnswerPostID != nil && *d.AnswerPostID == p.ID && d.AnsweredAt != nil && d.AnsweredAt.After(f.SeenThrough) {
				continue
			}
			list = append(list, digestPost{Author: digestAuthor(p), Text: postExcerpt(p.Body)})
		}
		if len(list) > digestPosts {
			data["More"] = len(list) - digestPosts
			list = list[:digestPosts]
		}
		if d.AnswerPostID != nil && d.AnsweredAt != nil && d.AnsweredAt.After(f.SeenThrough) {
			ans, err := q.GetDiscussionPost(ctx, store.GetDiscussionPostParams{WorkspaceID: a.WorkspaceID, ID: *d.AnswerPostID})
			if err != nil && !store.IsNotFound(err) {
				return err
			}
			if err == nil && (ans.AuthorContactID == nil || *ans.AuthorContactID != a.ContactID) {
				data["Answer"] = digestPost{Author: digestAuthor(ans), Text: postExcerpt(ans.Body)}
			}
			seen = later(seen, d.AnsweredAt)
		}
		var stages []map[string]string
		if d.Kind == kindIdea {
			moves, err := q.ListStageDigestChanges(ctx, store.ListStageDigestChangesParams{WorkspaceID: a.WorkspaceID, IdeaID: d.ID, After: f.SeenThrough})
			if err != nil {
				return err
			}
			for _, m := range moves {
				stages = append(stages, map[string]string{"Stage": m.ToStage})
				seen = later(seen, &m.CreatedAt)
			}
		}
		if len(list) == 0 && data["Answer"] == nil && len(stages) == 0 {
			return nil
		}
		if err := q.MarkDiscussionFollowEmailed(ctx, store.MarkDiscussionFollowEmailedParams{
			WorkspaceID: a.WorkspaceID, DiscussionID: d.ID, ContactID: a.ContactID, Now: now, SeenThrough: seen,
		}); err != nil {
			return err
		}
		to, locale, settings, err := s.digestRecipient(ctx, q, a, d.InboxID)
		if err != nil || to == "" {
			return err
		}
		in, err := q.GetInbox(ctx, store.GetInboxParams{WorkspaceID: a.WorkspaceID, ID: d.InboxID})
		if err != nil {
			return err
		}
		if locale == "" {
			locale = in.DefaultLocale
		}
		unsubscribe := s.auth.PublicURL + unsubscribePathPrefix + s.unsubscribeToken(a)
		data["Inbox"], data["Title"], data["Posts"] = in.Name, d.Title, list
		data["URL"], data["UnsubscribeURL"], data["SettingsURL"] = orZero(d.Page)+"#yuva-discussion-"+d.ID.String(), unsubscribe, settings
		if d.Kind == kindIdea {
			board, err := q.GetRoadmapBoard(ctx, store.GetRoadmapBoardParams{WorkspaceID: a.WorkspaceID, ID: orZero(d.BoardID)})
			if err != nil {
				return err
			}
			data["URL"], data["Idea"], data["Stages"] = s.roadmapURL(board)+"/ideas/"+d.ID.String(), true, stages
		}
		m, err := mail.RenderAs(inboxMailBrand(in), "discussion_digest", locale, data)
		if err != nil {
			return err
		}
		m.To, m.Unsubscribe = to, unsubscribe
		msg = &m
		return nil
	})
	if err != nil {
		return nil, err
	}
	if msg != nil {
		s.sendMail(ctx, *msg)
	}
	return next, nil
}

// digestRecipient is a reader's sign-in address while they want e-mails, or an identity-token
// user's first address.
func (s *Server) digestRecipient(ctx context.Context, q *store.Queries, a DiscussionDigestArgs, inbox uuid.UUID) (to, locale, settings string, err error) {
	st, err := q.GetContactDiscussionState(ctx, store.GetContactDiscussionStateParams{WorkspaceID: a.WorkspaceID, ID: a.ContactID, InboxID: inbox})
	if store.IsNotFound(err) {
		return "", "", "", nil
	}
	if err != nil {
		return "", "", "", err
	}
	if st.PersonID != nil {
		p, err := q.GetPerson(ctx, *st.PersonID)
		if err != nil || !p.ReaderEmailNotifications {
			return "", "", "", err
		}
		return p.Email, p.Locale, s.auth.PublicURL + "/account", nil
	}
	emails, err := q.ListContactEmails(ctx, store.ListContactEmailsParams{WorkspaceID: a.WorkspaceID, ContactIds: []uuid.UUID{a.ContactID}})
	if err != nil || len(emails) == 0 {
		return "", "", "", err
	}
	c, err := q.GetContact(ctx, store.GetContactParams{WorkspaceID: a.WorkspaceID, ID: a.ContactID})
	if err != nil {
		return "", "", "", err
	}
	if c.Locale != nil {
		locale = *c.Locale
	}
	return emails[0].Email, locale, "", nil
}

type unsubscribeWords struct {
	Lang, Title, Question, Button, Done, Gone, Error string
}

var unsubscribeLanguages = map[string]unsubscribeWords{
	"en": {Lang: "en", Title: "Stop e-mails about this thread", Question: "You follow “%s”. Stop getting e-mails about new replies there?", Button: "Stop following", Done: "You no longer follow this thread.", Gone: "This link is no longer valid.", Error: "Something went wrong. Try the link again later."},
	"tr": {Lang: "tr", Title: "Bu konuyla ilgili e-postaları durdurun", Question: "“%s” konusunu takip ediyorsunuz. Yeni yanıtlar için e-posta almayı bırakmak istiyor musunuz?", Button: "Takibi bırak", Done: "Bu konuyu artık takip etmiyorsunuz.", Gone: "Bu bağlantı artık geçerli değil.", Error: "Bir sorun oluştu. Bağlantıyı daha sonra yeniden deneyin."},
	"de": {Lang: "de", Title: "Keine E-Mails mehr zu diesem Thema", Question: "Sie folgen „%s“. Keine E-Mails mehr zu neuen Antworten erhalten?", Button: "Nicht mehr folgen", Done: "Sie folgen diesem Thema nicht mehr.", Gone: "Dieser Link ist nicht mehr gültig.", Error: "Etwas ist schiefgelaufen. Versuchen Sie den Link später erneut."},
}

func unsubscribeWordsFor(locale string) unsubscribeWords {
	base, _, _ := strings.Cut(strings.ToLower(strings.ReplaceAll(locale, "_", "-")), "-")
	if w, ok := unsubscribeLanguages[base]; ok {
		return w
	}
	return unsubscribeLanguages["en"]
}

var unsubscribePage = template.Must(template.New("unsubscribe").Parse(`<!doctype html>
<html lang="{{.W.Lang}}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.W.Title}}</title>
<style>
:root{--brand:{{.Color}};color-scheme:light dark}
body{font:16px/1.5 system-ui,sans-serif;max-width:30rem;margin:3rem auto;padding:0 1rem;color:#1f2328;background:#fff}
@media (prefers-color-scheme:dark){body{color:#e6e6e6;background:#16181d}}
.inbox{font-weight:600;color:var(--brand);margin:0 0 .5rem}
h1{font-size:1.5rem;margin:0 0 1rem}
button{font:inherit;font-weight:600;padding:.7rem 1.4rem;border:0;border-radius:.6rem;background:var(--brand);color:#fff;cursor:pointer}
</style>
</head><body>
{{if .Inbox}}<p class="inbox">{{.Inbox}}</p>{{end}}
<h1>{{.Heading}}</h1>
{{if .Text}}<p>{{.Text}}</p>{{end}}
{{if .Form}}<form method="post" action="{{.Action}}"><button type="submit">{{.W.Button}}</button></form>{{end}}
</body></html>`))

type unsubscribeView struct {
	W                    unsubscribeWords
	Inbox, Heading, Text string
	Action               string
	Color                template.CSS
	Form                 bool
}

func writeUnsubscribePage(w http.ResponseWriter, status int, v unsubscribeView) {
	if v.Color == "" {
		v.Color = defaultBrandColor
	}
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = unsubscribePage.Execute(w, v)
}

// unsubscribeTarget loads the thread and inbox a link names while its follow exists.
func (s *Server) unsubscribeTarget(ctx context.Context, token string) (DiscussionDigestArgs, store.Discussion, store.Inbox, bool, error) {
	a, ok := s.parseUnsubscribeToken(token)
	if !ok {
		return a, store.Discussion{}, store.Inbox{}, false, nil
	}
	if live, err := s.workspaceLive(ctx, a.WorkspaceID); err != nil || !live {
		return a, store.Discussion{}, store.Inbox{}, false, err
	}
	d, err := s.st.GetDiscussion(ctx, store.GetDiscussionParams{WorkspaceID: a.WorkspaceID, ID: a.DiscussionID})
	if store.IsNotFound(err) {
		return a, d, store.Inbox{}, false, nil
	}
	if err != nil {
		return a, d, store.Inbox{}, false, err
	}
	in, err := s.st.GetInbox(ctx, store.GetInboxParams{WorkspaceID: a.WorkspaceID, ID: d.InboxID})
	return a, d, in, err == nil, err
}

func unsubscribeViewFor(in store.Inbox) unsubscribeView {
	r := ratingViewFor(in)
	return unsubscribeView{W: unsubscribeWordsFor(in.DefaultLocale), Inbox: r.Inbox, Color: r.Color}
}

// serveUnsubscribePage asks before unfollowing: mail scanners open every link, so only the
// POST (the button, or a mail client's one-click unsubscribe) changes anything.
func (s *Server) serveUnsubscribePage(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	_, d, in, ok, err := s.unsubscribeTarget(r.Context(), token)
	v := unsubscribeView{W: unsubscribeWordsFor("en")}
	if ok {
		v = unsubscribeViewFor(in)
	}
	switch {
	case err != nil:
		s.log.ErrorContext(r.Context(), "unsubscribe page", slog.Any("error", err))
		v.Heading = v.W.Error
		writeUnsubscribePage(w, http.StatusInternalServerError, v)
	case !ok:
		v.Heading = v.W.Gone
		writeUnsubscribePage(w, http.StatusNotFound, v)
	default:
		v.Heading, v.Text, v.Form, v.Action = v.W.Title, strings.Replace(v.W.Question, "%s", d.Title, 1), true, unsubscribePathPrefix+token
		writeUnsubscribePage(w, http.StatusOK, v)
	}
}

func (s *Server) serveUnsubscribePost(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	token := r.PathValue("token")
	a, _, in, ok, err := s.unsubscribeTarget(r.Context(), token)
	v := unsubscribeView{W: unsubscribeWordsFor("en")}
	if ok {
		v = unsubscribeViewFor(in)
		_, err = s.st.UnfollowDiscussion(r.Context(), store.UnfollowDiscussionParams{WorkspaceID: a.WorkspaceID, DiscussionID: a.DiscussionID, ContactID: a.ContactID})
	}
	switch {
	case err != nil:
		s.log.ErrorContext(r.Context(), "unsubscribe", slog.Any("error", err))
		v.Heading = v.W.Error
		writeUnsubscribePage(w, http.StatusInternalServerError, v)
	case !ok:
		v.Heading = v.W.Gone
		writeUnsubscribePage(w, http.StatusNotFound, v)
	default:
		v.Heading = v.W.Done
		writeUnsubscribePage(w, http.StatusOK, v)
	}
}
