package api

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	defaultAutoReplyHours = 24
	smtpTLSStartTLS       = "starttls"
	smtpTLSImplicit       = "tls"
	smtpTLSNone           = "none"
)

var (
	errEmailAddressTaken = problem(http.StatusConflict, "email_address_taken", "another channel receives mail at this address")
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
		out.FromAddress = &from
	}
	if sm := in.Smtp; sm != nil {
		host := strings.TrimSpace(sm.Host)
		if host == "" || len(host) > 253 || !hostPattern.MatchString(host) {
			return out, errValidation("email.smtp.host must be a host name or IP address")
		}
		out.SmtpHost = host
		if sm.Tls != nil {
			if !sm.Tls.Valid() {
				return out, errValidation("email.smtp.tls must be starttls, tls or none")
			}
			out.SmtpTls = string(*sm.Tls)
		}
		switch {
		case sm.Port != nil:
			if *sm.Port < 1 || *sm.Port > 65535 {
				return out, errValidation("email.smtp.port must be 1 to 65535")
			}
			out.SmtpPort = *sm.Port
		case out.SmtpTls == smtpTLSImplicit:
			out.SmtpPort = 465
		}
		if sm.Username != nil {
			if out.SmtpUsername, err = trimmed(*sm.Username, 0, 320, "email.smtp.username"); err != nil {
				return out, err
			}
		}
		if out.SmtpUsername != "" && out.SmtpTls == smtpTLSNone && !loopbackHost(host) {
			return out, errValidation("email.smtp.username needs tls or starttls: the password is never sent unencrypted")
		}
		switch {
		case sm.Password == nil:
			if cur != nil {
				out.SmtpPassword = cur.SmtpPassword
			}
		case *sm.Password == "":
		case len(*sm.Password) > 1024:
			return out, errValidation("email.smtp.password must be at most 1024 bytes")
		default:
			out.SmtpPassword = s.secrets.Seal([]byte(*sm.Password), smtpSecretContext(workspaceID, channelID))
		}
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

func loopbackHost(h string) bool {
	return h == "localhost" || h == "127.0.0.1" || h == "::1"
}
