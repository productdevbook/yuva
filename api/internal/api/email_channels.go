package api

import (
	"bytes"
	"context"
	"crypto/hmac"
	"net/http"
	"regexp"
	"strings"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/email"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	defaultAutoReplyHours = 24
	smtpTLSStartTLS       = "starttls"
	smtpTLSImplicit       = "tls"
	smtpTLSNone           = "none"
	catchAllLocal         = "*"
	defaultBroadcastRate  = 5
)

var (
	errEmailAddressTaken = problem(http.StatusConflict, "email_address_taken", "another channel of this workspace has this address")
	errEmailRequired     = errValidation("an email channel needs email settings")
	errEmailNotAllowed   = errValidation("email settings are only for email channels")
	hostPattern          = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?)*$`)
)

func smtpSecretContext(workspaceID, channelID uuid.UUID) []byte {
	return append(append([]byte("smtp:"), workspaceID[:]...), channelID[:]...)
}

func emailChannelBody(e store.EmailChannel) *oas.EmailChannel {
	out := &oas.EmailChannel{
		Address: oas.Email(e.Address), DisplayName: e.DisplayName,
		AutoReply: oas.EmailAutoReply{Enabled: e.AutoReplyEnabled, Text: e.AutoReplyText, IntervalHours: e.AutoReplyIntervalHours},
	}
	if e.FromAddress != nil {
		f := oas.Email(*e.FromAddress)
		out.FromAddress = &f
	}
	if e.SmtpHost != "" {
		out.Smtp = &oas.SmtpSettings{
			Host: e.SmtpHost, Port: e.SmtpPort, Username: e.SmtpUsername, Tls: oas.SmtpTls(e.SmtpTls),
			PasswordSet: len(e.SmtpPassword) > 0,
		}
	}
	if e.BroadcastSmtpHost != nil && *e.BroadcastSmtpHost != "" {
		out.BroadcastSmtp = &oas.SmtpSettings{
			Host: *e.BroadcastSmtpHost, Port: 587, Username: deref(e.BroadcastSmtpUsername), Tls: oas.SmtpTls(smtpTLSStartTLS),
			PasswordSet: len(e.BroadcastSmtpPassword) > 0,
		}
		if e.BroadcastSmtpPort != nil {
			out.BroadcastSmtp.Port = *e.BroadcastSmtpPort
		}
		if e.BroadcastSmtpTls != nil {
			out.BroadcastSmtp.Tls = oas.SmtpTls(*e.BroadcastSmtpTls)
		}
	}
	out.BroadcastRate = e.BroadcastRate
	out.Dkim = emailDkimBody(e)
	return out
}

func (s *Server) emailChannelParams(workspaceID, channelID uuid.UUID, in *oas.EmailChannelInput, cur *store.EmailChannel) (store.CreateEmailChannelParams, error) {
	out := store.CreateEmailChannelParams{
		WorkspaceID: workspaceID, ChannelID: channelID, SmtpPort: 587, SmtpTls: smtpTLSStartTLS,
		AutoReplyIntervalHours: defaultAutoReplyHours,
	}
	var err error
	if out.Address, err = normalizeEmail(in.Address); err != nil {
		return out, errValidation("email.address is not a valid address")
	}
	if local, _, _ := strings.Cut(out.Address, "@"); strings.Contains(local, catchAllLocal) && local != catchAllLocal {
		return out, errValidation("email.address: a catch-all address is *@domain")
	}
	if err := s.checkChannelAddress(out.Address); err != nil {
		return out, err
	}
	if in.DisplayName != nil {
		if out.DisplayName, err = trimmed(*in.DisplayName, 0, 200, "email.display_name"); err != nil {
			return out, err
		}
	}
	if in.FromAddress != nil && strings.TrimSpace(string(*in.FromAddress)) != "" {
		from, err := normalizeEmail(*in.FromAddress)
		if err != nil {
			return out, errValidation("email.from_address is not a valid address")
		}
		if isCatchAll(from) {
			return out, errValidation("email.from_address cannot be a catch-all address")
		}
		out.FromAddress = &from
	}
	if sm := in.Smtp; sm != nil {
		acc, err := s.smtpAccount("email.smtp", *sm, curPassword(cur), smtpSecretContext(workspaceID, channelID))
		if err != nil {
			return out, err
		}
		out.SmtpHost, out.SmtpPort, out.SmtpUsername, out.SmtpPassword, out.SmtpTls = acc.host, acc.port, acc.username, acc.password, acc.tls
	}
	if ar := in.AutoReply; ar != nil {
		out.AutoReplyEnabled = ar.Enabled
		if ar.Text != nil {
			if out.AutoReplyText, err = trimmed(*ar.Text, 0, 5000, "email.auto_reply.text"); err != nil {
				return out, err
			}
		}
		if ar.IntervalHours != nil {
			if *ar.IntervalHours < 1 || *ar.IntervalHours > 8760 {
				return out, errValidation("email.auto_reply.interval_hours must be 1 to 8760")
			}
			out.AutoReplyIntervalHours = *ar.IntervalHours
		}
		if out.AutoReplyEnabled && out.AutoReplyText == "" {
			return out, errValidation("email.auto_reply.text is required when the auto-reply is enabled")
		}
	}
	return out, nil
}

type smtpAccount struct {
	host, username, tls string
	port                int32
	password            []byte
}

func curPassword(cur *store.EmailChannel) []byte {
	if cur == nil {
		return nil
	}
	return cur.SmtpPassword
}

// smtpAccount checks an SMTP account from the API; a password left out keeps keep, an empty one
// removes it, and a new one is sealed under secretCtx.
func (s *Server) smtpAccount(field string, sm oas.SmtpSettingsInput, keep, secretCtx []byte) (smtpAccount, error) {
	out := smtpAccount{port: 587, tls: smtpTLSStartTLS}
	host := strings.TrimSpace(sm.Host)
	if host == "" || len(host) > 253 || !hostPattern.MatchString(host) {
		return out, errValidation(field + ".host must be a host name or IP address")
	}
	if email.CheckSMTPHost(host, s.smtpPriv) != nil {
		return out, errValidation(field + ".host points to a private, loopback or link-local address")
	}
	out.host = host
	if sm.Tls != nil {
		if !sm.Tls.Valid() {
			return out, errValidation(field + ".tls must be starttls, tls or none")
		}
		out.tls = string(*sm.Tls)
	}
	switch {
	case sm.Port != nil:
		if *sm.Port < 1 || *sm.Port > 65535 {
			return out, errValidation(field + ".port must be 1 to 65535")
		}
		out.port = *sm.Port
	case out.tls == smtpTLSImplicit:
		out.port = 465
	}
	var err error
	if sm.Username != nil {
		if out.username, err = trimmed(*sm.Username, 0, 320, field+".username"); err != nil {
			return out, err
		}
	}
	if out.username != "" && out.tls == smtpTLSNone && !loopbackHost(host) {
		return out, errValidation(field + ".username needs tls or starttls: the password is never sent unencrypted")
	}
	switch {
	case sm.Password == nil:
		out.password = keep
	case *sm.Password == "":
	case len(*sm.Password) > 1024:
		return out, errValidation(field + ".password must be at most 1024 bytes")
	case s.samePassword(keep, *sm.Password, secretCtx):
		out.password = keep
	default:
		out.password = s.secrets.Seal([]byte(*sm.Password), secretCtx)
	}
	return out, nil
}

// samePassword reports a password sent again unchanged, which keeps its sealed form so the audit
// log does not record a change.
func (s *Server) samePassword(sealed []byte, plain string, secretCtx []byte) bool {
	if len(sealed) == 0 {
		return false
	}
	cur, err := s.secrets.Open(sealed, secretCtx)
	return err == nil && hmac.Equal(cur, []byte(plain))
}

// emailBroadcastParams are a channel's broadcast account and pace after an update: what the input
// leaves out stays as it was.
func (s *Server) emailBroadcastParams(workspaceID, channelID uuid.UUID, in *oas.EmailChannelInput, cur *store.EmailChannel) (*store.SetEmailChannelBroadcastParams, error) {
	out := store.SetEmailChannelBroadcastParams{WorkspaceID: workspaceID, ChannelID: channelID, Rate: defaultBroadcastRate}
	if cur != nil {
		out.Host, out.Port, out.Username, out.Password, out.Tls, out.Rate = cur.BroadcastSmtpHost, cur.BroadcastSmtpPort,
			cur.BroadcastSmtpUsername, cur.BroadcastSmtpPassword, cur.BroadcastSmtpTls, cur.BroadcastRate
	}
	if in.BroadcastSmtp == nil && in.BroadcastSmtpRemove == nil && in.BroadcastRate == nil {
		return nil, nil
	}
	if in.BroadcastSmtpRemove != nil && *in.BroadcastSmtpRemove {
		if in.BroadcastSmtp != nil {
			return nil, errValidation("send email.broadcast_smtp or email.broadcast_smtp_remove, not both")
		}
		out.Host, out.Port, out.Username, out.Password, out.Tls = nil, nil, nil, nil, nil
	}
	if sm := in.BroadcastSmtp; sm != nil {
		acc, err := s.smtpAccount("email.broadcast_smtp", *sm, out.Password, broadcastSMTPSecretContext(workspaceID, channelID))
		if err != nil {
			return nil, err
		}
		out.Host, out.Port, out.Username, out.Password, out.Tls = &acc.host, &acc.port, &acc.username, acc.password, &acc.tls
	}
	if r := in.BroadcastRate; r != nil {
		if *r < 1 || *r > 100 {
			return nil, errValidation("email.broadcast_rate must be 1 to 100 mails a second")
		}
		out.Rate = *r
	}
	return &out, nil
}

func saveEmailChannel(ctx context.Context, q *store.Queries, arg store.CreateEmailChannelParams) (store.EmailChannel, error) {
	e, err := q.CreateEmailChannel(ctx, arg)
	if store.IsUniqueViolation(err) {
		return e, errEmailAddressTaken
	}
	return e, err
}

func emailChannelsByID(ctx context.Context, q *store.Queries, workspaceID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]store.EmailChannel, error) {
	rows, err := q.ListEmailChannels(ctx, store.ListEmailChannelsParams{WorkspaceID: workspaceID, ChannelIds: ids})
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]store.EmailChannel, len(rows))
	for _, r := range rows {
		out[r.ChannelID] = r
	}
	return out, nil
}

func isCatchAll(address string) bool {
	return strings.HasPrefix(address, catchAllLocal+"@")
}

func loopbackHost(h string) bool {
	return h == "localhost" || h == "127.0.0.1" || h == "::1"
}

// auditCredentials records which SMTP accounts of an e-mail channel were set or changed: host,
// username or password, never their values.
func (s *Server) auditCredentials(ctx context.Context, q *store.Queries, p principal, prev *store.EmailChannel, cur store.EmailChannel) error {
	var before store.EmailChannel
	if prev != nil {
		before = *prev
	}
	var fields []string
	if before.SmtpHost != cur.SmtpHost || before.SmtpUsername != cur.SmtpUsername || !bytes.Equal(before.SmtpPassword, cur.SmtpPassword) {
		fields = append(fields, "smtp")
	}
	if deref(before.BroadcastSmtpHost) != deref(cur.BroadcastSmtpHost) || deref(before.BroadcastSmtpUsername) != deref(cur.BroadcastSmtpUsername) ||
		!bytes.Equal(before.BroadcastSmtpPassword, cur.BroadcastSmtpPassword) {
		fields = append(fields, "broadcast_smtp")
	}
	if len(fields) == 0 {
		return nil
	}
	return s.audit(ctx, q, p, oas.AuditChannelCredentialsChanged, oas.AuditTargetChannel, cur.ChannelID, auditDetails{"fields": fields})
}
