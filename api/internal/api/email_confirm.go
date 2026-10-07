package api

import (
	"context"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/mail"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	emailConfirmTTL      = 24 * time.Hour
	emailConfirmsPerHour = 3
	emailConfirmPath     = "/email/confirm"
	asyncMailTimeout     = time.Minute
)

// sendAsync sends a mail after the request has answered, so its timing reveals nothing.
func (s *Server) sendAsync(msg mail.Message) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), asyncMailTimeout)
		defer cancel()
		if err := s.mailer.Send(ctx, msg); err != nil {
			s.log.WarnContext(ctx, "mail", slog.String("subject", msg.Subject), slog.Any("error", err))
		}
	}()
}

// requestEmailConfirmation records a typed address for confirmation and returns the mail to send
// once the transaction commits, or nil when no mail is due: the address already belongs to a
// contact, or the contact asked too often.
func (s *Server) requestEmailConfirmation(ctx context.Context, q *store.Queries, inbox store.Inbox, contactID uuid.UUID, addr string) (*mail.Message, error) {
	ws, now := inbox.WorkspaceID, s.now()
	if _, err := q.GetContactIDByEmail(ctx, store.GetContactIDByEmailParams{WorkspaceID: ws, Email: addr}); !store.IsNotFound(err) {
		return nil, err
	}
	n, err := q.CountEmailConfirmations(ctx, store.CountEmailConfirmationsParams{WorkspaceID: ws, ContactID: contactID, Since: now.Add(-time.Hour)})
	if err != nil || n >= emailConfirmsPerHour {
		return nil, err
	}
	token := ws.String() + "." + randomToken(32)
	if err := q.CreateEmailConfirmation(ctx, store.CreateEmailConfirmationParams{
		WorkspaceID: ws, ID: newID(), ContactID: contactID, InboxID: inbox.ID, Email: addr,
		TokenHash: hashSecret(token), Now: now, ExpiresAt: now.Add(emailConfirmTTL),
	}); err != nil {
		return nil, err
	}
	msg, err := mail.Render("confirm_email", inbox.DefaultLocale, map[string]any{
		"Inbox": inbox.Name, "URL": s.auth.PublicURL + emailConfirmPath + "?token=" + url.QueryEscape(token),
		"Hours": int(emailConfirmTTL / time.Hour),
	})
	if err != nil {
		return nil, err
	}
	msg.To = addr
	return &msg, nil
}

func confirmationWorkspace(token string) (uuid.UUID, bool) {
	head, _, ok := strings.Cut(token, ".")
	if !ok || len(token) > 200 {
		return uuid.Nil(), false
	}
	ws, err := uuid.Parse(head)
	return ws, err == nil
}

var confirmPage = template.Must(template.New("confirm").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<style>body{font:16px/1.5 system-ui,sans-serif;max-width:32rem;margin:4rem auto;padding:0 1rem;color:#222}button{font:inherit;padding:.5rem 1rem;cursor:pointer}</style>
</head><body><h1>{{.Title}}</h1><p>{{.Text}}</p>
{{if .Token}}<form method="post" action="{{.Action}}"><input type="hidden" name="token" value="{{.Token}}"><button type="submit">Confirm</button></form>{{end}}
</body></html>`))

type confirmView struct {
	Title, Text, Token, Action string
}

func writeConfirmPage(w http.ResponseWriter, status int, v confirmView) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = confirmPage.Execute(w, v)
}

var confirmGone = confirmView{Title: "Link expired", Text: "This confirmation link is no longer valid. Leave your address in the chat again to get a new one."}

// serveEmailConfirm shows a button for a confirmation link; only the POST it sends confirms, so a
// mail scanner that opens links confirms nothing.
func (s *Server) serveEmailConfirm(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	ws, ok := confirmationWorkspace(token)
	if !ok {
		writeConfirmPage(w, http.StatusNotFound, confirmGone)
		return
	}
	c, err := s.st.GetEmailConfirmation(r.Context(), store.GetEmailConfirmationParams{WorkspaceID: ws, TokenHash: hashSecret(token), Now: s.now()})
	if store.IsNotFound(err) {
		writeConfirmPage(w, http.StatusNotFound, confirmGone)
		return
	}
	if err != nil {
		s.log.ErrorContext(r.Context(), "email confirmation", slog.Any("error", err))
		writeConfirmPage(w, http.StatusInternalServerError, confirmView{Title: "Something went wrong", Text: "Try the link again later."})
		return
	}
	writeConfirmPage(w, http.StatusOK, confirmView{
		Title: "Confirm your e-mail address", Text: "Confirm that replies for " + c.Email + " may be linked to your conversations.",
		Token: token, Action: emailConfirmPath,
	})
}

func (s *Server) serveEmailConfirmPost(w http.ResponseWriter, r *http.Request) {
	token := r.PostFormValue("token")
	ws, ok := confirmationWorkspace(token)
	if !ok {
		writeConfirmPage(w, http.StatusNotFound, confirmGone)
		return
	}
	confirmed := false
	err := s.inTx(r.Context(), ws, func(q *store.Queries, events *eventBatch) error {
		var err error
		confirmed, err = s.confirmEmail(r.Context(), q, events, ws, token)
		return err
	})
	if err != nil {
		s.log.ErrorContext(r.Context(), "email confirmation", slog.Any("error", err))
		writeConfirmPage(w, http.StatusInternalServerError, confirmView{Title: "Something went wrong", Text: "Try the link again later."})
		return
	}
	if !confirmed {
		writeConfirmPage(w, http.StatusNotFound, confirmGone)
		return
	}
	writeConfirmPage(w, http.StatusOK, confirmView{Title: "Address confirmed", Text: "Thank you. You can close this page."})
}

// confirmEmail makes a typed address one of its contact's addresses when the contact still has it
// typed and no contact owns it.
func (s *Server) confirmEmail(ctx context.Context, q *store.Queries, events *eventBatch, ws uuid.UUID, token string) (bool, error) {
	now := s.now()
	c, err := q.TakeEmailConfirmation(ctx, store.TakeEmailConfirmationParams{WorkspaceID: ws, TokenHash: hashSecret(token), Now: now})
	if store.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	ct, err := q.LockContact(ctx, store.LockContactParams{WorkspaceID: ws, ID: c.ContactID})
	if err != nil {
		return false, err
	}
	if ct.TypedEmail == nil || *ct.TypedEmail != c.Email {
		return false, nil
	}
	if _, err := q.GetContactIDByEmail(ctx, store.GetContactIDByEmailParams{WorkspaceID: ws, Email: c.Email}); !store.IsNotFound(err) {
		return false, err
	}
	n, err := q.CountContactEmails(ctx, store.CountContactEmailsParams{WorkspaceID: ws, ContactID: ct.ID})
	if err != nil {
		return false, err
	}
	if err := q.AddContactEmail(ctx, store.AddContactEmailParams{WorkspaceID: ws, ContactID: ct.ID, Email: c.Email, Position: int32(n)}); err != nil {
		return false, err
	}
	r, err := q.SetContactTypedEmail(ctx, store.SetContactTypedEmailParams{WorkspaceID: ws, ID: ct.ID, Now: now})
	if err != nil {
		return false, err
	}
	if err := q.DeleteContactEmailConfirmations(ctx, store.DeleteContactEmailConfirmationsParams{WorkspaceID: ws, ContactID: ct.ID, Now: now}); err != nil {
		return false, err
	}
	if err := q.RefreshContactSearch(ctx, store.RefreshContactSearchParams{WorkspaceID: ws, ID: ct.ID}); err != nil {
		return false, err
	}
	body, err := s.contactBody(ctx, q, ws, contactRow(r))
	if err != nil {
		return false, err
	}
	events.add(realtime.ContactUpdated, nil, nil, body)
	return true, nil
}
