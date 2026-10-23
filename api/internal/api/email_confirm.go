package api

import (
	"context"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/jobs"
	"github.com/productdevbook/yuva/api/internal/mail"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	emailConfirmTTL      = 24 * time.Hour
	emailConfirmsPerHour = 3
	emailConfirmPath     = "/email/confirm"
	confirmInboxNameMax  = 60
)

var (
	limitTypedConfirmPerAddress   = limit{3, 24 * time.Hour}
	limitTypedConfirmPerChannel   = limit{300, 24 * time.Hour}
	limitTypedConfirmPerWorkspace = limit{1000, 24 * time.Hour}
)

// sendMail hands a mail to the server mailer, which sends it after the request has answered
// (mail.Async), and only logs a failure.
func (s *Server) sendMail(ctx context.Context, msg mail.Message) {
	if err := s.mailer.Send(context.WithoutCancel(ctx), msg); err != nil {
		s.log.WarnContext(ctx, "mail", slog.String("subject", msg.Subject), slog.Any("error", err))
	}
}

// requestEmailConfirmation records a typed address for confirmation and returns the mail to send
// once the transaction commits, or nil when no mail is due.
func (s *Server) requestEmailConfirmation(ctx context.Context, q *store.Queries, inbox store.Inbox, contactID uuid.UUID, addr string) (*mail.Message, error) {
	ws, now := inbox.WorkspaceID, s.now()
	if _, err := q.GetContactIDByEmail(ctx, store.GetContactIDByEmailParams{WorkspaceID: ws, Email: addr}); !store.IsNotFound(err) {
		return nil, err
	}
	n, err := q.CountEmailConfirmations(ctx, store.CountEmailConfirmationsParams{WorkspaceID: ws, ContactID: contactID, Since: now.Add(-time.Hour)})
	if err != nil || n >= emailConfirmsPerHour {
		return nil, err
	}
	outcome, err := typedEmailOutcome(ctx, q, ws, addr, now)
	if err != nil {
		return nil, err
	}
	token := ws.String() + "." + randomToken(32)
	id, expires := newID(), now.Add(emailConfirmTTL)
	switch outcome {
	case typedEmailUndeliverable:
		return nil, q.CreateBouncedEmailConfirmation(ctx, store.CreateBouncedEmailConfirmationParams{
			WorkspaceID: ws, ID: id, ContactID: contactID, InboxID: inbox.ID, Email: addr, TokenHash: hashSecret(token), Now: now, ExpiresAt: expires,
		})
	case typedEmailSilent:
		return nil, q.CreateEmailConfirmation(ctx, store.CreateEmailConfirmationParams{
			WorkspaceID: ws, ID: id, ContactID: contactID, InboxID: inbox.ID, Email: addr, TokenHash: hashSecret(token), Now: now, ExpiresAt: expires,
		})
	}
	msgID := s.confirmationMessageID()
	if err := q.CreateEmailConfirmation(ctx, store.CreateEmailConfirmationParams{
		WorkspaceID: ws, ID: id, ContactID: contactID, InboxID: inbox.ID, Email: addr,
		TokenHash: hashSecret(token), HeaderMessageID: &msgID, Now: now, ExpiresAt: expires,
	}); err != nil {
		return nil, err
	}
	brand := inboxMailBrand(inbox)
	brand.Name = confirmInboxName(inbox.Name)
	msg, err := mail.RenderAs(brand, "confirm_email", inbox.DefaultLocale, map[string]any{
		"Inbox": brand.Name, "URL": s.auth.PublicURL + emailConfirmPath + "?token=" + url.QueryEscape(token),
		"Hours": int(emailConfirmTTL / time.Hour),
	})
	if err != nil {
		return nil, err
	}
	msg.To, msg.MessageID = addr, msgID
	msg.OnRelayed = func(ctx context.Context, relayID string) {
		if err := s.st.SetEmailConfirmationRelayID(ctx, store.SetEmailConfirmationRelayIDParams{WorkspaceID: ws, ID: id, RelayMessageID: &relayID}); err != nil {
			s.log.WarnContext(ctx, "confirmation relay id", slog.Any("error", err))
		}
	}
	return &msg, nil
}

// sendTypedConfirmation sends a confirmation mail for a typed address, if any, within the
// server-wide limit for that address and the daily limits of the channel and workspace. It runs
// after the transaction committed: a mail it refuses leaves the confirmation looking sent, as for a
// suppressed address, so a visitor cannot tell the address was capped.
func (s *Server) sendTypedConfirmation(ctx context.Context, cp contactPrincipal, msg *mail.Message) {
	if msg == nil || s.rateLimit(ctx,
		rateCheck{"typed-confirm:address:" + msg.To, limitTypedConfirmPerAddress},
		rateCheck{"typed-confirm:channel:" + cp.channelID.String(), limitTypedConfirmPerChannel},
		rateCheck{"typed-confirm:workspace:" + cp.workspaceID.String(), limitTypedConfirmPerWorkspace},
	) != nil {
		return
	}
	s.sendMail(ctx, *msg)
}

// confirmInboxName is the inbox name as the confirmation mail shows it: the workspace chose it, and
// the mail goes to an address nobody has confirmed, so it is one short line.
func confirmInboxName(name string) string {
	name = strings.Join(strings.Fields(oneLine(name)), " ")
	if utf8.RuneCountInString(name) <= confirmInboxNameMax {
		return name
	}
	return strings.TrimSpace(truncateRunes(name, confirmInboxNameMax-1)) + "…"
}

type typedEmailDelivery int

const (
	typedEmailSend typedEmailDelivery = iota
	typedEmailUndeliverable
	typedEmailSilent
)

// A bounce is public (anyone can mail the address); any other suppression must look like a sent link.
func typedEmailOutcome(ctx context.Context, q *store.Queries, ws uuid.UUID, addr string, now time.Time) (typedEmailDelivery, error) {
	reason, err := q.EmailSuppressionReason(ctx, store.EmailSuppressionReasonParams{WorkspaceID: ws, Email: addr, Expired: now.Add(-jobs.EmailConfirmationRetention)})
	switch {
	case store.IsNotFound(err):
		return typedEmailSend, nil
	case err != nil:
		return 0, err
	case reason == suppressBounce:
		return typedEmailUndeliverable, nil
	default:
		return typedEmailSilent, nil
	}
}

func (s *Server) confirmationMessageID() string {
	domain := "yuva.invalid"
	if len(s.ingress.OwnAddresses) > 0 {
		if _, d, ok := strings.Cut(s.ingress.OwnAddresses[0], "@"); ok && d != "" {
			domain = d
		}
	} else if u, err := url.Parse(s.auth.PublicURL); err == nil && u.Hostname() != "" {
		domain = u.Hostname()
	}
	return "confirm." + randomToken(18) + "@" + domain
}

func (s *Server) emailConfirmationState(ctx context.Context, q *store.Queries, c contactRow, current *string) (*oas.ClientEmailConfirmation, error) {
	r, err := q.LatestEmailConfirmation(ctx, store.LatestEmailConfirmationParams{WorkspaceID: c.WorkspaceID, ContactID: c.ID})
	if store.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := &oas.ClientEmailConfirmation{Email: oas.Email(r.Email)}
	switch {
	case c.TypedEmail == nil:
		if r.ConfirmedAt == nil || current == nil || *current != r.Email {
			return nil, nil
		}
		out.State = oas.EmailConfirmationConfirmed
	case *c.TypedEmail != r.Email:
		return nil, nil
	case r.BouncedAt != nil:
		out.State = oas.EmailConfirmationUndeliverable
	case r.ConfirmedAt == nil && r.ExpiresAt.After(s.now()):
		out.State, out.ExpiresAt = oas.EmailConfirmationSent, &r.ExpiresAt
	default:
		return nil, nil
	}
	return out, nil
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
	discard := func() (bool, error) {
		if err := q.DeleteEmailConfirmation(ctx, store.DeleteEmailConfirmationParams{WorkspaceID: ws, ID: c.ID}); err != nil {
			return false, err
		}
		row, err := q.GetContact(ctx, store.GetContactParams{WorkspaceID: ws, ID: ct.ID})
		if err != nil {
			return false, err
		}
		body, err := s.contactBody(ctx, q, ws, allInboxScope, row)
		if err != nil {
			return false, err
		}
		events.add(realtime.ContactUpdated, nil, nil, body)
		return false, nil
	}
	if ct.TypedEmail == nil || *ct.TypedEmail != c.Email {
		return discard()
	}
	if _, err := q.GetContactIDByEmail(ctx, store.GetContactIDByEmailParams{WorkspaceID: ws, Email: c.Email}); err == nil {
		return discard()
	} else if !store.IsNotFound(err) {
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
	if err := q.DeleteContactEmailConfirmations(ctx, store.DeleteContactEmailConfirmationsParams{
		WorkspaceID: ws, ContactID: ct.ID, Keep: c.ID,
	}); err != nil {
		return false, err
	}
	if err := q.RefreshContactSearch(ctx, store.RefreshContactSearchParams{WorkspaceID: ws, ID: ct.ID}); err != nil {
		return false, err
	}
	body, err := s.contactBody(ctx, q, ws, allInboxScope, contactRow(r))
	if err != nil {
		return false, err
	}
	events.add(realtime.ContactUpdated, nil, nil, body)
	return true, nil
}
