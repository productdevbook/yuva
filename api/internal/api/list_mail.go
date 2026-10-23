package api

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"log/slog"
	"time"
	"uuid"

	"github.com/riverqueue/river"

	"github.com/productdevbook/yuva/api/internal/email"
	"github.com/productdevbook/yuva/api/internal/jobs"
	"github.com/productdevbook/yuva/api/internal/mail"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	listMailConfirm = "confirm"
	listMailWelcome = "welcome"

	confirmPathPrefix = "/s/"
	managePathPrefix  = "/m/"
	confirmLinkTTL    = 7 * 24 * time.Hour
	listMailAttempts  = 8
)

var (
	confirmTokenContext = []byte("yuva list confirm v1")
	manageTokenContext  = []byte("yuva list manage v1")
	formTokenContext    = []byte("yuva subscribe form v1")
)

// sealToken seals ids and a time under the master key; nothing is stored for the token and it
// cannot be made or changed without the key.
func (s *Server) sealToken(label []byte, ids []uuid.UUID, at *time.Time) string {
	plain := make([]byte, 0, 16*len(ids)+8)
	for _, id := range ids {
		plain = append(plain, id[:]...)
	}
	if at != nil {
		plain = binary.BigEndian.AppendUint64(plain, uint64(at.UnixMicro()))
	}
	return base64.RawURLEncoding.EncodeToString(s.secrets.Seal(plain, label))
}

func (s *Server) openToken(token string, label []byte, n int, timed bool) ([]uuid.UUID, time.Time, bool) {
	if len(token) > 300 {
		return nil, time.Time{}, false
	}
	sealed, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, time.Time{}, false
	}
	plain, err := s.secrets.Open(sealed, label)
	size := 16 * n
	if timed {
		size += 8
	}
	if err != nil || len(plain) != size {
		return nil, time.Time{}, false
	}
	ids := make([]uuid.UUID, n)
	for i := range ids {
		copy(ids[i][:], plain[16*i:16*(i+1)])
	}
	var at time.Time
	if timed {
		at = time.UnixMicro(int64(binary.BigEndian.Uint64(plain[16*n:])))
	}
	return ids, at, true
}

func (s *Server) confirmURL(sub store.ListSubscription) string {
	return s.linkBase() + confirmPathPrefix + s.sealToken(confirmTokenContext, []uuid.UUID{sub.WorkspaceID, sub.ID}, new(s.now()))
}

// manageURL is the unsubscribe and preference page of a contact for a list; broadcast is zero
// outside broadcasts. It never expires while the contact exists.
func (s *Server) manageURL(ws, listID, contactID, broadcast uuid.UUID) string {
	return s.linkBase() + managePathPrefix + s.sealToken(manageTokenContext, []uuid.UUID{ws, listID, contactID, broadcast}, nil)
}

type ListMailArgs struct {
	WorkspaceID    uuid.UUID `json:"workspace_id"`
	SubscriptionID uuid.UUID `json:"subscription_id"`
	Mail           string    `json:"mail"`
}

func (ListMailArgs) Kind() string { return "list_mail" }

func (ListMailArgs) InsertOpts() river.InsertOpts { return river.InsertOpts{Queue: jobs.EmailQueue} }

func listMailOpts() *river.InsertOpts { return &river.InsertOpts{MaxAttempts: listMailAttempts} }

type listMailWorker struct {
	river.WorkerDefaults[ListMailArgs]
	s *Server
}

func (w *listMailWorker) Work(ctx context.Context, job *river.Job[ListMailArgs]) error {
	return w.s.SendListMail(ctx, job.Args, job.Attempt >= job.MaxAttempts)
}

func (w *listMailWorker) Timeout(*river.Job[ListMailArgs]) time.Duration { return 2 * time.Minute }

// SendListMail sends a list's confirmation or welcome mail through the list's e-mail channel,
// never through the server's own mailer. Mail that no longer fits (the subscription moved on, the
// channel went away, the address bounced) is dropped; a temporary SMTP failure is retried.
func (s *Server) SendListMail(ctx context.Context, a ListMailArgs, lastAttempt bool) error {
	if live, err := s.workspaceLive(ctx, a.WorkspaceID); err != nil || !live {
		return err
	}
	ws := a.WorkspaceID
	sub, err := s.st.GetSubscription(ctx, store.GetSubscriptionParams{WorkspaceID: ws, ID: a.SubscriptionID})
	if store.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if (a.Mail == listMailConfirm && sub.State != subPending) || (a.Mail == listMailWelcome && sub.State != subSubscribed) {
		return nil
	}
	list, err := s.st.GetMailingList(ctx, store.GetMailingListParams{WorkspaceID: ws, ID: sub.ListID})
	if err != nil {
		return err
	}
	skip := func(reason string) error {
		s.log.InfoContext(ctx, "list mail not sent", slog.String("subscription_id", sub.ID.String()), slog.String("kind", a.Mail), slog.String("reason", mail.RedactAddresses(reason)))
		return nil
	}
	if list.ChannelID == nil {
		return skip("the list has no e-mail channel")
	}
	ch, err := s.st.GetEmailChannel(ctx, store.GetEmailChannelParams{WorkspaceID: ws, ChannelID: *list.ChannelID})
	if store.IsNotFound(err) {
		return skip("the list's channel is gone")
	}
	if err != nil {
		return err
	}
	from, ok := listSender(ch)
	if !ok {
		return skip("the list's channel cannot send")
	}
	if suppressed, err := s.st.IsEmailSuppressedAnyScope(ctx, store.IsEmailSuppressedAnyScopeParams{WorkspaceID: ws, Email: sub.Email}); err != nil {
		return err
	} else if suppressed {
		return skip("the address is suppressed")
	}
	in, err := s.st.GetInbox(ctx, store.GetInboxParams{WorkspaceID: ws, ID: list.InboxID})
	if err != nil {
		return err
	}
	locale := in.DefaultLocale
	if c, err := s.st.GetContact(ctx, store.GetContactParams{WorkspaceID: ws, ID: sub.ContactID}); err == nil && c.Locale != nil && *c.Locale != "" {
		locale = *c.Locale
	}
	headers := []email.Header{{Name: "Auto-Submitted", Value: "auto-generated"}, {Name: "X-Auto-Response-Suppress", Value: "All"}}
	var msg mail.Message
	page, isStatus, err := statusListPage(ctx, s.st.Queries, list)
	if err != nil {
		return err
	}
	switch {
	case a.Mail == listMailConfirm && isStatus:
		var data map[string]string
		if data, err = s.statusConfirmData(ctx, page, sub); err == nil {
			msg, err = mail.RenderAs(inboxMailBrand(in), "status_confirm", locale, data)
		}
	case a.Mail == listMailConfirm:
		msg, err = mail.RenderAs(inboxMailBrand(in), "list_confirm", locale, map[string]string{
			"Inbox": in.Name, "List": list.Name, "URL": s.confirmURL(sub),
		})
	case isStatus:
		return skip("a status list sends no welcome mail")
	default:
		manage := s.manageURL(ws, list.ID, sub.ContactID, uuid.Nil())
		msg, err = mail.RenderAs(inboxMailBrand(in), "list_welcome", locale, map[string]string{
			"Inbox": in.Name, "List": list.Name, "Description": list.Description, "UnsubscribeURL": manage, "ManageURL": manage,
			"Sender": senderLine(in),
		})
		headers = append(headers, email.Header{Name: "List-Unsubscribe", Value: "<" + manage + ">"},
			email.Header{Name: "List-Unsubscribe-Post", Value: "List-Unsubscribe=One-Click"})
	}
	if err != nil {
		return err
	}
	name := ch.DisplayName
	if name == "" {
		name = in.Name
	}
	replyTo := ch.Address
	if isCatchAll(replyTo) {
		replyTo = from
	}
	sesH, err := s.sesHeaders(ctx, ch, email.SMTPConfig{Host: ch.SmtpHost, Username: ch.SmtpUsername})
	if err != nil {
		return err
	}
	headers = append(headers, sesH...)
	raw, err := email.Build(email.Outgoing{
		From: email.Address{Name: name, Email: from}, ReplyTo: email.Address{Name: name, Email: replyTo},
		To: email.Address{Email: sub.Email}, Subject: msg.Subject, Text: msg.Text, HTML: msg.HTML, Date: s.now(),
		MessageID: email.NewMessageID(email.NewConversationToken(), email.Domain(from)), Headers: headers,
	})
	if err != nil {
		return err
	}
	key, err := s.dkimKey(ch, from, ch.SmtpHost)
	if err != nil {
		return skip(err.Error())
	}
	if raw, err = signWith(key, raw); err != nil {
		return err
	}
	var password string
	if len(ch.SmtpPassword) > 0 {
		plain, err := s.secrets.Open(ch.SmtpPassword, smtpSecretContext(ws, ch.ChannelID))
		if err != nil {
			return skip("the SMTP password cannot be decrypted with this master key")
		}
		password = string(plain)
	}
	cfg := email.SMTPConfig{Host: ch.SmtpHost, Port: int(ch.SmtpPort), Username: ch.SmtpUsername, Password: password, TLS: ch.SmtpTls}
	sendCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	if err := s.sender.Send(sendCtx, cfg, from, []string{sub.Email}, raw); err != nil {
		if email.IsPermanent(err) || lastAttempt {
			return skip(err.Error())
		}
		return mail.RedactError(err)
	}
	if a.Mail != listMailConfirm {
		return nil
	}
	return s.st.InTx(ctx, func(q *store.Queries) error {
		return s.consentEvent(ctx, q, sub, consentConfirmationSent, consentProof{source: "mail", detail: map[string]any{"channel_id": ch.ChannelID.String()}})
	})
}
