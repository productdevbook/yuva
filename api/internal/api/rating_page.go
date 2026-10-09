package api

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"uuid"

	"github.com/riverqueue/river"

	"github.com/productdevbook/yuva/api/internal/mail"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	// ratingRequestDelay leaves time to undo a close before the contact is asked.
	ratingRequestDelay = 2 * time.Minute
	ratingPathPrefix   = "/r/"
	defaultBrandColor  = "#2563eb"
	// maxRatingFormBytes fits a percent-encoded comment of maxRatingComment four-byte characters.
	maxRatingFormBytes = 32 << 10
)

var ratingTokenContext = []byte("yuva rating link v1")

// ratingToken names one close of a conversation, sealed under the master key so it cannot be
// made or changed without it: the link stops working once the conversation is closed again.
func (s *Server) ratingToken(c store.Conversation) string {
	plain := make([]byte, 0, 40)
	plain = append(plain, c.WorkspaceID[:]...)
	plain = append(plain, c.ID[:]...)
	plain = binary.BigEndian.AppendUint64(plain, uint64(c.ClosedAt.UnixMicro()))
	return base64.RawURLEncoding.EncodeToString(s.secrets.Seal(plain, ratingTokenContext))
}

type ratingRef struct {
	workspaceID, conversationID uuid.UUID
	closedAt                    time.Time
}

func (s *Server) parseRatingToken(token string) (ratingRef, bool) {
	if len(token) > 200 {
		return ratingRef{}, false
	}
	sealed, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return ratingRef{}, false
	}
	plain, err := s.secrets.Open(sealed, ratingTokenContext)
	if err != nil || len(plain) != 40 {
		return ratingRef{}, false
	}
	var ref ratingRef
	copy(ref.workspaceID[:], plain[:16])
	copy(ref.conversationID[:], plain[16:32])
	ref.closedAt = time.UnixMicro(int64(binary.BigEndian.Uint64(plain[32:])))
	return ref, true
}

type RatingRequestArgs struct {
	WorkspaceID    uuid.UUID `json:"workspace_id"`
	ConversationID uuid.UUID `json:"conversation_id"`
}

func (RatingRequestArgs) Kind() string { return "rating_request" }

type ratingRequestWorker struct {
	river.WorkerDefaults[RatingRequestArgs]
	s *Server
}

func (w *ratingRequestWorker) Work(ctx context.Context, job *river.Job[RatingRequestArgs]) error {
	return w.s.RequestRating(ctx, job.Args.WorkspaceID, job.Args.ConversationID)
}

// scheduleRatingRequest asks an e-mail contact for a rating a little after a member or API key
// closes their conversation.
func (s *Server) scheduleRatingRequest(events *eventBatch, before, after store.Conversation, now time.Time) {
	if before.Status == string(oas.ConversationStatusClosed) || after.Status != string(oas.ConversationStatusClosed) ||
		after.ChannelID == nil || after.Spam {
		return
	}
	events.job(RatingRequestArgs{WorkspaceID: after.WorkspaceID, ConversationID: after.ID}, &river.InsertOpts{ScheduledAt: now.Add(ratingRequestDelay)})
}

// RequestRating e-mails the contact of a closed e-mail conversation two rating links, once per
// close, when the inbox asks for ratings, a member or bot replied in it and it is still closed and
// unrated.
func (s *Server) RequestRating(ctx context.Context, workspaceID, conversationID uuid.UUID) error {
	if live, err := s.workspaceLive(ctx, workspaceID); err != nil || !live {
		return err
	}
	return s.inTx(ctx, workspaceID, func(q *store.Queries, events *eventBatch) error {
		c, err := q.LockConversation(ctx, store.LockConversationParams{WorkspaceID: workspaceID, ID: conversationID})
		if store.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		in, err := q.GetInbox(ctx, store.GetInboxParams{WorkspaceID: workspaceID, ID: c.InboxID})
		if err != nil {
			return err
		}
		now := s.now()
		if can, _ := ratingState(in.RatingSince, c.Status, c.ClosedAt, c.RatedAt, c.Rating, now); !can || c.Spam {
			return nil
		}
		replied, err := q.ConversationHasReply(ctx, store.ConversationHasReplyParams{WorkspaceID: workspaceID, ConversationID: c.ID})
		if err != nil || !replied {
			return err
		}
		plan, err := s.planEmail(ctx, q, c)
		var apiErr *apiError
		if errors.As(err, &apiErr) || (err == nil && plan == nil) {
			return nil
		}
		if err != nil {
			return err
		}
		if _, err := q.ClaimRatingRequest(ctx, store.ClaimRatingRequestParams{WorkspaceID: workspaceID, ID: c.ID, ClosedAt: c.ClosedAt, Now: now}); err != nil {
			if store.IsNotFound(err) {
				return nil
			}
			return err
		}
		locale := in.DefaultLocale
		if ct, err := q.GetContact(ctx, store.GetContactParams{WorkspaceID: workspaceID, ID: c.ContactID}); err == nil && ct.Locale != nil && *ct.Locale != "" {
			locale = *ct.Locale
		}
		link := s.auth.PublicURL + ratingPathPrefix + s.ratingToken(c) + "?rating="
		msg, err := mail.Render("rating_request", locale, map[string]string{
			"Inbox": in.Name, "GoodURL": link + string(oas.Good), "BadURL": link + string(oas.Bad),
		})
		if err != nil {
			return err
		}
		plan.auto = true
		direction, queued := string(oas.Out), deliveryQueued
		row, err := q.CreateMessage(ctx, store.CreateMessageParams{
			ID: newID(), WorkspaceID: workspaceID, ConversationID: c.ID, Kind: string(oas.MessageKindMessage),
			Direction: &direction, AuthorType: string(oas.AuthorTypeSystem), Body: strings.TrimSpace(msg.Text), CreatedAt: now,
			DeliveryState: &queued,
		})
		if err != nil {
			return err
		}
		summary, err := s.queueEmail(ctx, q, events, plan, row)
		if err != nil {
			return err
		}
		if err := q.TouchConversation(ctx, store.TouchConversationParams{WorkspaceID: workspaceID, ID: c.ID, Now: now, IsMessage: true}); err != nil {
			return err
		}
		if err := s.addUsage(ctx, q, workspaceID, 0, 1, 0); err != nil {
			return err
		}
		events.conversation(realtime.MessageCreated, c, withEmail(messageBody(row, nil), summary))
		return nil
	})
}

type ratingWords struct {
	Lang, Title, Question, Good, Bad, Comment, Send, Thanks, ThanksText, Already, Gone, Error string
}

var ratingLanguages = map[string]ratingWords{
	"en": {
		Lang: "en", Title: "How did we do?", Question: "How was your conversation with %s?", Good: "Good", Bad: "Not good",
		Comment: "Anything you would like to add? (optional)", Send: "Send", Thanks: "Thank you!",
		ThanksText: "Your rating was sent to %s. You can close this page.", Already: "This conversation has already been rated. Thank you!",
		Gone: "This link is no longer valid.", Error: "Something went wrong. Try the link again later.",
	},
	"tr": {
		Lang: "tr", Title: "Nasıldık?", Question: "%s ile konuşmanız nasıldı?", Good: "İyi", Bad: "İyi değil",
		Comment: "Eklemek istediğiniz bir şey var mı? (isteğe bağlı)", Send: "Gönder", Thanks: "Teşekkürler!",
		ThanksText: "Değerlendirmeniz %s ekibine iletildi. Bu sayfayı kapatabilirsiniz.", Already: "Bu konuşma zaten değerlendirildi. Teşekkürler!",
		Gone: "Bu bağlantı artık geçerli değil.", Error: "Bir sorun oluştu. Bağlantıyı daha sonra yeniden deneyin.",
	},
	"de": {
		Lang: "de", Title: "Wie waren wir?", Question: "Wie war Ihre Unterhaltung mit %s?", Good: "Gut", Bad: "Nicht gut",
		Comment: "Möchten Sie noch etwas hinzufügen? (optional)", Send: "Senden", Thanks: "Vielen Dank!",
		ThanksText: "Ihre Bewertung wurde an %s gesendet. Sie können diese Seite schließen.", Already: "Diese Unterhaltung wurde bereits bewertet. Vielen Dank!",
		Gone: "Dieser Link ist nicht mehr gültig.", Error: "Etwas ist schiefgelaufen. Versuchen Sie den Link später erneut.",
	},
}

func ratingWordsFor(locale string) ratingWords {
	base, _, _ := strings.Cut(strings.ToLower(strings.ReplaceAll(locale, "_", "-")), "-")
	if w, ok := ratingLanguages[base]; ok {
		return w
	}
	return ratingLanguages["en"]
}

var ratingPage = template.Must(template.New("rating").Parse(`<!doctype html>
<html lang="{{.W.Lang}}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Heading}}</title>
<style>
:root{--brand:{{.Color}};color-scheme:light dark}
body{font:16px/1.5 system-ui,sans-serif;max-width:30rem;margin:3rem auto;padding:0 1rem;color:#1f2328;background:#fff}
@media (prefers-color-scheme:dark){body{color:#e6e6e6;background:#16181d}textarea{background:#22252b;color:inherit}}
.inbox{font-weight:600;color:var(--brand);margin:0 0 .5rem}
h1{font-size:1.5rem;margin:0 0 1rem}
fieldset{border:0;padding:0;margin:0 0 1rem;display:flex;gap:.75rem}
legend{margin-bottom:.75rem}
label.choice{flex:1}
label.choice input{position:absolute;opacity:0}
label.choice span{display:block;text-align:center;padding:.9rem;border:2px solid #8884;border-radius:.75rem;cursor:pointer;font-weight:600}
label.choice input:checked+span{border-color:var(--brand);background:color-mix(in srgb,var(--brand) 14%,transparent)}
label.choice input:focus-visible+span{outline:2px solid var(--brand);outline-offset:2px}
textarea{width:100%;box-sizing:border-box;min-height:6rem;font:inherit;padding:.6rem;border:1px solid #8886;border-radius:.5rem;margin:.25rem 0 1rem}
button{font:inherit;font-weight:600;padding:.7rem 1.4rem;border:0;border-radius:.6rem;background:var(--brand);color:#fff;cursor:pointer}
</style>
</head><body>
{{if .Inbox}}<p class="inbox">{{.Inbox}}</p>{{end}}
<h1>{{.Heading}}</h1>
{{if .Text}}<p>{{.Text}}</p>{{end}}
{{if .Form}}<form method="post" action="{{.Action}}">
<fieldset><legend>{{.Question}}</legend>
<label class="choice"><input type="radio" name="rating" value="good" required{{if eq .Chosen "good"}} checked{{end}}><span>{{.W.Good}}</span></label>
<label class="choice"><input type="radio" name="rating" value="bad"{{if eq .Chosen "bad"}} checked{{end}}><span>{{.W.Bad}}</span></label>
</fieldset>
<label for="comment">{{.W.Comment}}</label>
<textarea id="comment" name="comment" maxlength="2000"></textarea>
<button type="submit">{{.W.Send}}</button>
</form>{{end}}
</body></html>`))

type ratingView struct {
	W                                      ratingWords
	Inbox, Heading, Text, Question, Chosen string
	Action                                 string
	Color                                  template.CSS
	Form                                   bool
}

func writeRatingPage(w http.ResponseWriter, status int, v ratingView) {
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
	_ = ratingPage.Execute(w, v)
}

// ratingTarget loads the conversation and inbox a rating link names; ok is false when the link
// is not one of ours or names a close that is no longer the conversation's latest.
func (s *Server) ratingTarget(ctx context.Context, q *store.Queries, token string, lock bool) (store.Conversation, store.Inbox, bool, error) {
	ref, ok := s.parseRatingToken(token)
	if !ok {
		return store.Conversation{}, store.Inbox{}, false, nil
	}
	get := q.GetConversation
	if lock {
		get = func(ctx context.Context, arg store.GetConversationParams) (store.Conversation, error) {
			return q.LockConversation(ctx, store.LockConversationParams(arg))
		}
	}
	c, err := get(ctx, store.GetConversationParams{WorkspaceID: ref.workspaceID, ID: ref.conversationID})
	if store.IsNotFound(err) {
		return c, store.Inbox{}, false, nil
	}
	if err != nil {
		return c, store.Inbox{}, false, err
	}
	in, err := q.GetInbox(ctx, store.GetInboxParams{WorkspaceID: ref.workspaceID, ID: c.InboxID})
	if err != nil {
		return c, in, false, err
	}
	if live, err := s.workspaceLive(ctx, ref.workspaceID); err != nil || !live {
		return c, in, false, err
	}
	return c, in, c.ClosedAt != nil && c.ClosedAt.UnixMicro() == ref.closedAt.UnixMicro(), nil
}

func ratingViewFor(in store.Inbox) ratingView {
	v := ratingView{W: ratingWordsFor(in.DefaultLocale), Inbox: in.Name}
	var b oas.InboxBranding
	if json.Unmarshal(in.Branding, &b) == nil && b.Color != nil && colorPattern.MatchString(*b.Color) {
		v.Color = template.CSS(*b.Color)
	}
	return v
}

// serveRatingPage shows the rating form for a link from a rating e-mail. Opening the link stores
// nothing: mail scanners open every link, so only the form's POST rates.
func (s *Server) serveRatingPage(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	c, in, ok, err := s.ratingTarget(r.Context(), s.st.Queries, token, false)
	s.answerRatingPage(w, r, token, c, in, ok, err, nil)
}

func (s *Server) serveRatingPost(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRatingFormBytes)
	token := r.PathValue("token")
	var (
		c       store.Conversation
		in      store.Inbox
		ok      bool
		rateErr error
	)
	ref, valid := s.parseRatingToken(token)
	if !valid {
		s.answerRatingPage(w, r, token, c, in, false, nil, nil)
		return
	}
	err := s.inTx(r.Context(), ref.workspaceID, func(q *store.Queries, events *eventBatch) error {
		var err error
		if c, in, ok, err = s.ratingTarget(r.Context(), q, token, true); err != nil || !ok {
			return err
		}
		_, rateErr = s.rate(r.Context(), q, events, in, c, oas.Rating(r.PostFormValue("rating")), r.PostFormValue("comment"))
		var apiErr *apiError
		if errors.As(rateErr, &apiErr) {
			return nil
		}
		return rateErr
	})
	if err == nil && rateErr == nil {
		v := ratingViewFor(in)
		v.Heading, v.Text = v.W.Thanks, strings.Replace(v.W.ThanksText, "%s", in.Name, 1)
		writeRatingPage(w, http.StatusOK, v)
		return
	}
	s.answerRatingPage(w, r, token, c, in, ok, err, rateErr)
}

func (s *Server) answerRatingPage(w http.ResponseWriter, r *http.Request, token string, c store.Conversation, in store.Inbox, ok bool, err, rateErr error) {
	v := ratingView{W: ratingWordsFor("en")}
	if in.ID != uuid.Nil() {
		v = ratingViewFor(in)
	}
	switch {
	case err != nil:
		s.log.ErrorContext(r.Context(), "rating page", slog.Any("error", err))
		v.Heading = v.W.Error
		writeRatingPage(w, http.StatusInternalServerError, v)
		return
	case !ok:
		v.Heading = v.W.Gone
		writeRatingPage(w, http.StatusNotFound, v)
		return
	}
	can, current := ratingState(in.RatingSince, c.Status, c.ClosedAt, c.RatedAt, c.Rating, s.now())
	switch {
	case current != nil || errors.Is(rateErr, errAlreadyRated):
		v.Heading = v.W.Already
		writeRatingPage(w, http.StatusOK, v)
	case !can || c.Spam || errors.Is(rateErr, errRatingUnavailable):
		v.Heading = v.W.Gone
		writeRatingPage(w, http.StatusNotFound, v)
	default:
		v.Heading, v.Question, v.Form, v.Action = v.W.Title, strings.Replace(v.W.Question, "%s", in.Name, 1), true, ratingPathPrefix+token
		if ch := r.FormValue("rating"); ch == string(oas.Good) || ch == string(oas.Bad) {
			v.Chosen = ch
		}
		status := http.StatusOK
		if rateErr != nil {
			status = http.StatusBadRequest
		}
		writeRatingPage(w, status, v)
	}
}
