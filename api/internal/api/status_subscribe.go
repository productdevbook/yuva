package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

var (
	statusSubscribeContext   = []byte("yuva status subscribe v1")
	limitStatusSubscribeIP   = limit{10, time.Hour}
	limitStatusSubscribePage = limit{300, time.Hour}
	errStatusSubscribersFull = errors.New("the page takes no more subscribers")
	statusSubscribeHoneypot  = "website"
)

func (s *Server) statusEmailOpen(pg store.StatusPage) bool {
	return !s.status.NoticesOff && pg.Published && pg.EmailEnabled && pg.MailingListID != nil
}

type statusSubscribeForm struct {
	pg         store.StatusPage
	list       store.MailingList
	inbox      store.Inbox
	components []store.StatusComponent
}

// It is never cached and never framed, because the form carries a sealed token.
func (s *Server) serveStatusSubscribe(w http.ResponseWriter, r *http.Request) {
	pg, ok := s.publicStatusPage(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	if !s.statusEmailOpen(pg) {
		goneListPage(w)
		return
	}
	f := statusSubscribeForm{pg: pg}
	var err error
	if f.list, err = s.st.GetMailingList(ctx, store.GetMailingListParams{WorkspaceID: pg.WorkspaceID, ID: *pg.MailingListID}); store.IsNotFound(err) {
		goneListPage(w)
		return
	}
	if err == nil {
		f.inbox, err = s.st.GetInbox(ctx, store.GetInboxParams{WorkspaceID: pg.WorkspaceID, ID: pg.InboxID})
	}
	if err == nil {
		f.components, err = visibleComponents(ctx, s.st.Queries, pg)
	}
	if err != nil {
		s.listPageError(w, r, err)
		return
	}
	if live, err := s.workspaceLive(ctx, pg.WorkspaceID); err != nil || !live {
		goneListPage(w)
		return
	}
	words := listWordsFor(pg.Locale)
	if strings.HasSuffix(r.URL.Path, "/sent") {
		v := s.statusSubscribeView(f, words)
		v.Heading, v.Texts = words.SentTitle, []string{words.SentText}
		writeListPage(w, http.StatusOK, v)
		return
	}
	if r.Method == http.MethodGet {
		note := ""
		if err := s.statusSubscriberRoom(ctx, s.st.Queries, f); errors.Is(err, errStatusSubscribersFull) {
			note = words.PageFull
		} else if err != nil {
			s.listPageError(w, r, err)
			return
		}
		s.statusSubscribeFormPage(w, r, f, words, http.StatusOK, note, "", formLists(r.URL.Query()["components"]))
		return
	}
	if err := s.rateLimit(ctx, rateCheck{"status-subscribe:ip:" + rateIP(s.clientIP(r)), limitStatusSubscribeIP},
		rateCheck{"status-subscribe:page:" + pg.ID.String(), limitStatusSubscribePage}); err != nil {
		var ae *apiError
		if errors.As(err, &ae) && ae.RetryAfter > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(ae.RetryAfter))
		}
		v := s.statusSubscribeView(f, words)
		v.Heading, v.Texts = fill(words.SubscribeTitle, pg.Name, f.inbox.Name, ""), []string{words.TooMany}
		writeListPage(w, http.StatusTooManyRequests, v)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxListFormBytes)
	_ = r.ParseForm()
	sent := statusLocalPath(r, pg.Slug, "/subscribe/sent")
	if strings.TrimSpace(r.PostFormValue(statusSubscribeHoneypot)) != "" {
		http.Redirect(w, r, sent, http.StatusSeeOther)
		return
	}
	typed := r.PostFormValue("email")
	chosen := formLists(r.PostForm["components"])
	ids, issued, ok := s.openToken(r.PostFormValue("token"), statusSubscribeContext, 1, true)
	if age := s.now().Sub(issued); !ok || ids[0] != pg.ID || age < formTokenMinAge || age > formTokenMaxAge {
		s.statusSubscribeFormPage(w, r, f, words, http.StatusBadRequest, words.FormExpired, typed, chosen)
		return
	}
	addr, err := subscriptionAddress(typed)
	if err != nil {
		s.statusSubscribeFormPage(w, r, f, words, http.StatusBadRequest, words.InvalidEmail, typed, chosen)
		return
	}
	visible := make([]uuid.UUID, len(f.components))
	for i, c := range f.components {
		visible[i] = c.ID
	}
	if len(visible) > 1 && !slices.ContainsFunc(chosen, func(id uuid.UUID) bool { return slices.Contains(visible, id) }) {
		s.statusSubscribeFormPage(w, r, f, words, http.StatusBadRequest, words.ChooseOne, typed, chosen)
		return
	}
	chosen = slices.DeleteFunc(chosen, func(id uuid.UUID) bool { return !slices.Contains(visible, id) })
	pr := s.requestProof(r, string(oas.SubscriptionSourceForm))
	page, text := s.pageURL(pg, "/subscribe"), fill(words.SubscribeConsent, pg.Name, f.inbox.Name, "")
	pr.pageURL, pr.text = &page, &text
	pr.detail = map[string]any{"status_page_id": pg.ID.String()}
	if len(chosen) > 0 && len(chosen) < len(visible) {
		named := make([]string, len(chosen))
		for i, id := range chosen {
			named[i] = id.String()
		}
		pr.detail["components"] = named
	}
	locale := pg.Locale
	channel := uuid.Nil()
	if f.list.ChannelID != nil {
		channel = *f.list.ChannelID
	}
	err = s.inTx(ctx, pg.WorkspaceID, func(q *store.Queries, events *eventBatch) error {
		if err := s.statusSubscriberRoom(ctx, q, f); err != nil {
			return err
		}
		if err := s.subscribeFromForm(ctx, q, events, channel, []store.MailingList{f.list}, addr, "", &locale, pr); err != nil {
			return err
		}
		sub, err := q.LockSubscriptionByEmail(ctx, store.LockSubscriptionByEmailParams{WorkspaceID: pg.WorkspaceID, ListID: f.list.ID, Email: addr})
		if store.IsNotFound(err) || (err == nil && sub.State != subPending) {
			return nil
		}
		if err != nil {
			return err
		}
		return setSubscriptionComponents(ctx, q, sub, visible, chosen)
	})
	switch {
	case errors.Is(err, errStatusSubscribersFull):
		s.statusSubscribeFormPage(w, r, f, words, http.StatusOK, words.PageFull, "", nil)
	case err != nil:
		s.log.ErrorContext(ctx, "status subscribe", slog.Any("error", err))
		s.listPageError(w, r, err)
	default:
		http.Redirect(w, r, sent, http.StatusSeeOther)
	}
}

func (s *Server) statusSubscriberRoom(ctx context.Context, q *store.Queries, f statusSubscribeForm) error {
	if s.status.MaxSubscribers <= 0 {
		return nil
	}
	counts, err := q.CountSubscriptions(ctx, store.CountSubscriptionsParams{WorkspaceID: f.pg.WorkspaceID, ListIds: []uuid.UUID{f.list.ID}})
	if err != nil {
		return err
	}
	for _, c := range counts {
		if c.Subscribed >= int64(s.status.MaxSubscribers) {
			return errStatusSubscribersFull
		}
	}
	return nil
}

func (s *Server) statusSubscribeView(f statusSubscribeForm, words listWords) listPageView {
	v := listPageView{W: words, Inbox: f.pg.Name}
	statusPageColor(&v, &f.pg)
	v.Links = []pageLink{{URL: s.pageURL(f.pg, ""), Label: words.StatusBack}}
	return v
}

func (s *Server) statusSubscribeFormPage(w http.ResponseWriter, r *http.Request, f statusSubscribeForm, words listWords, status int, note, typed string, chosen []uuid.UUID) {
	v := s.statusSubscribeView(f, words)
	v.Heading = fill(words.SubscribeTitle, f.pg.Name, f.inbox.Name, "")
	if note != "" {
		v.Texts = []string{note}
	}
	if note == words.PageFull {
		writeListPage(w, status, v)
		return
	}
	v.Target = statusLocalPath(r, f.pg.Slug, "/subscribe")
	form := pageForm{
		Action: "subscribe", Button: words.SubscribeButton, Field: "components", Legend: words.StatusLegend,
		Note:   fill(words.SubscribeConsent, f.pg.Name, f.inbox.Name, ""),
		Hidden: map[string]string{"token": s.sealToken(statusSubscribeContext, []uuid.UUID{f.pg.ID}, new(s.now()))},
		Inputs: []formInput{{Name: "email", Type: "email", Label: words.EmailLabel, Value: typed, Auto: "email"}},
		Trap:   statusSubscribeHoneypot,
	}
	if len(f.components) > 1 {
		for _, c := range f.components {
			form.Choices = append(form.Choices, pageChoice{ID: c.ID.String(), Name: c.Name, Description: c.Description,
				Checked: len(chosen) == 0 || slices.Contains(chosen, c.ID)})
		}
	}
	v.Forms = []pageForm{form}
	writeListPage(w, status, v)
}

// statusLocalPath is a path of the page on the host the request came to: an own domain serves its
// page at /, the status host pages at /{slug}, the server's own host at /status/{slug}.
func statusLocalPath(r *http.Request, slug, path string) string {
	if on, ok := r.Context().Value(statusHostKey{}).(statusOn); ok {
		if on.domain {
			return path
		}
		return "/" + slug + path
	}
	return statusPathPrefix + slug + path
}
