package api

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/email"
	"github.com/productdevbook/yuva/api/internal/mail"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	emailProofTXT         = "_yuva-mail."
	emailProofPrefix      = "yuva-mail-verify="
	verifyCodeTTL         = time.Hour
	verifyCodeMaxAttempts = 5
	verifyCodeGap         = time.Minute
	verifyWindow          = 24 * time.Hour
	verifyChannelSends    = 5
	verifyWorkspaceSends  = 20
	verifyProblemTaken    = "taken"
	verifiedByCode        = string(oas.EmailVerifiedByCode)
	verifiedByDNS         = string(oas.EmailVerifiedByDns)
)

var (
	errVerifyCodeInvalid = problem(http.StatusBadRequest, "verification_code_invalid", "the code is wrong, used or expired; request a new one")
	errAlreadyVerified   = problem(http.StatusConflict, "email_verified", "this channel's address is already proven")
	errVerifyRateLimited = problem(http.StatusTooManyRequests, "rate_limited", "too many address codes requested; try again later")
	errNoAddressToProve  = errValidation("only e-mail channels have an address to prove")
)

// sharedMailDomain reports a domain whose addresses belong to many unrelated people: the
// forwarding domain (every channel's forwarding address) and public mail providers. Nobody holds
// a catch-all there.
func (s *Server) sharedMailDomain(domain string) bool {
	return (s.ingress.ForwardingDomain != "" && domain == s.ingress.ForwardingDomain) || email.PublicMailDomain(domain)
}

// addressProven reports a channel whose address routes mail and counts as its workspace's own.
func (s *Server) addressProven(e store.EmailChannel) bool {
	return e.VerifiedAt != nil && !(isCatchAll(e.Address) && s.sharedMailDomain(email.Domain(e.Address)))
}

func (s *Server) emailProofValue(workspaceID uuid.UUID, domain string) string {
	m := hmac.New(sha256.New, s.secrets.Derive("yuva email domain proof v1"))
	m.Write(workspaceID[:])
	m.Write([]byte(domain))
	return emailProofPrefix + hex.EncodeToString(m.Sum(nil))[:32]
}

func (s *Server) verifyCodeHash(channelID uuid.UUID, code string) []byte {
	m := hmac.New(sha256.New, s.secrets.Derive("yuva email address codes v1"))
	m.Write(channelID[:])
	m.Write([]byte(code))
	return m.Sum(nil)
}

func (s *Server) emailVerificationBody(e store.EmailChannel) *oas.EmailVerification {
	domain := email.Domain(e.Address)
	out := &oas.EmailVerification{
		State: oas.EmailVerificationUnverified, TxtName: emailProofTXT + domain, TxtValue: s.emailProofValue(e.WorkspaceID, domain),
		CodeAllowed: s.codeAllowed(e.Address), Problem: e.VerifyProblem, CheckedAt: e.VerifyCheckedAt,
	}
	if s.addressProven(e) {
		m := oas.EmailVerificationMethod(deref(e.VerifiedBy))
		out.State, out.Method, out.VerifiedAt, out.Problem = oas.EmailVerificationVerified, &m, e.VerifiedAt, nil
		return out
	}
	if e.VerifiedAt != nil {
		p := "a catch-all on a shared mail domain never receives mail; use an address of your own domain"
		out.Problem = &p
	}
	if e.VerifyCodeHash != nil && e.VerifyCodeExpiresAt != nil && e.VerifyCodeExpiresAt.After(s.now()) {
		out.CodeSentAt, out.CodeExpiresAt = e.VerifySentAt, e.VerifyCodeExpiresAt
	}
	return out
}

// codeAllowed reports an address a code mail can prove: not a catch-all, and not on the
// forwarding domain, whose mail comes back to this server.
func (s *Server) codeAllowed(address string) bool {
	return !isCatchAll(address) && (s.ingress.ForwardingDomain == "" || email.Domain(address) != s.ingress.ForwardingDomain)
}

// checkChannelAddress refuses addresses no workspace can hold: catch-alls on shared domains.
func (s *Server) checkChannelAddress(address string) error {
	if isCatchAll(address) && s.sharedMailDomain(email.Domain(address)) {
		return errValidation("email.address: a catch-all on a shared mail domain (the forwarding domain or a public mail provider) is not possible")
	}
	return nil
}

func newVerifyCode() string {
	n, err := rand.Int(rand.Reader, big.NewInt(100_000_000))
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("%08d", n.Int64())
}

// emailChannelFor returns the e-mail settings of a channel the principal manages.
func (s *Server) emailChannelFor(ctx context.Context, p principal, id oas.ChannelId) (store.Channel, store.EmailChannel, error) {
	if err := requireManagerOrKey(p); err != nil {
		return store.Channel{}, store.EmailChannel{}, err
	}
	c, err := s.visibleChannel(ctx, p, id)
	if err != nil {
		return c, store.EmailChannel{}, err
	}
	if c.Kind != string(oas.ChannelKindEmail) {
		return c, store.EmailChannel{}, errNoAddressToProve
	}
	e, err := s.st.GetEmailChannel(ctx, store.GetEmailChannelParams{WorkspaceID: p.workspaceID, ChannelID: c.ID})
	if store.IsNotFound(err) {
		return c, e, errNoAddressToProve
	}
	return c, e, err
}

func (s *Server) SendEmailVerificationCode(ctx context.Context, req oas.SendEmailVerificationCodeRequestObject) (oas.SendEmailVerificationCodeResponseObject, error) {
	p := principalFrom(ctx)
	c, e, err := s.emailChannelFor(ctx, p, req.ChannelId)
	if err != nil {
		return nil, err
	}
	if !s.codeAllowed(e.Address) {
		return nil, errValidation("a catch-all or an address on the forwarding domain is proven by its domain's TXT record")
	}
	if e.VerifiedAt != nil {
		return nil, errAlreadyVerified
	}
	now := s.now()
	code := newVerifyCode()
	msgID := s.confirmationMessageID()
	addr := e.Address
	var out oas.Channel
	err = s.st.InTx(ctx, func(q *store.Queries) error {
		cur, err := q.LockEmailChannel(ctx, store.LockEmailChannelParams{WorkspaceID: p.workspaceID, ChannelID: c.ID})
		if store.IsNotFound(err) {
			return errNoAddressToProve
		}
		if err != nil {
			return err
		}
		if cur.VerifiedAt != nil {
			return errAlreadyVerified
		}
		if !s.codeAllowed(cur.Address) {
			return errValidation("a catch-all or an address on the forwarding domain is proven by its domain's TXT record")
		}
		addr = cur.Address
		windowSince := now.Add(-verifyWindow)
		if cur.VerifySentAt != nil && cur.VerifySentAt.After(now.Add(-verifyCodeGap)) {
			return errVerifyRateLimited
		}
		if cur.VerifyWindowAt != nil && cur.VerifyWindowAt.After(windowSince) && cur.VerifySends >= verifyChannelSends {
			return errVerifyRateLimited
		}
		n, err := q.CountWorkspaceVerificationSends(ctx, store.CountWorkspaceVerificationSendsParams{WorkspaceID: p.workspaceID, Since: &windowSince})
		if err != nil {
			return err
		}
		if n >= verifyWorkspaceSends {
			return errVerifyRateLimited
		}
		expires := now.Add(verifyCodeTTL)
		saved, err := q.StartEmailVerificationCode(ctx, store.StartEmailVerificationCodeParams{
			WorkspaceID: p.workspaceID, ChannelID: c.ID, Address: cur.Address, CodeHash: s.verifyCodeHash(c.ID, code),
			ExpiresAt: &expires, MessageID: &msgID, Now: &now, WindowSince: &windowSince,
		})
		if err != nil {
			return err
		}
		out, err = s.oneChannelBodyWith(ctx, q, c, saved)
		return err
	})
	if err != nil {
		return nil, err
	}
	msg, err := mail.Render("address_code", "en", map[string]any{"Address": addr, "Code": code, "Minutes": int(verifyCodeTTL / time.Minute)})
	if err != nil {
		return nil, err
	}
	ws, ch := p.workspaceID, c.ID
	msg.To, msg.MessageID = addr, msgID
	msg.OnRelayed = func(ctx context.Context, relayID string) {
		id := email.NormalizeID(relayID)
		if err := s.st.SetEmailVerificationRelayID(ctx, store.SetEmailVerificationRelayIDParams{WorkspaceID: ws, ChannelID: ch, MessageID: &msgID, RelayMessageID: &id}); err != nil {
			s.log.WarnContext(ctx, "address code relay id", slog.Any("error", err))
		}
	}
	s.sendMail(ctx, msg)
	return oas.SendEmailVerificationCode202JSONResponse(out), nil
}

func (s *Server) VerifyEmailChannel(ctx context.Context, req oas.VerifyEmailChannelRequestObject) (oas.VerifyEmailChannelResponseObject, error) {
	p := principalFrom(ctx)
	c, e, err := s.emailChannelFor(ctx, p, req.ChannelId)
	if err != nil {
		return nil, err
	}
	if err := s.checkChannelAddress(e.Address); err != nil {
		return nil, err
	}
	var code string
	if req.Body != nil && req.Body.Code != nil {
		code = strings.TrimSpace(*req.Body.Code)
	}
	if e.VerifiedAt != nil {
		out, err := s.oneChannelBodyWith(ctx, s.st.Queries, c, e)
		if err != nil {
			return nil, err
		}
		return oas.VerifyEmailChannel200JSONResponse(out), nil
	}
	if code != "" {
		return s.verifyByCode(ctx, p, c, code)
	}
	domain := email.Domain(e.Address)
	seen, problemText := s.lookupEmailProof(ctx, domain, s.emailProofValue(p.workspaceID, domain))
	now := s.now()
	var out oas.Channel
	err = s.st.InTx(ctx, func(q *store.Queries) error {
		var saved store.EmailChannel
		var err error
		if seen {
			saved, err = s.proveAddress(ctx, q, &p, e, verifiedByDNS, now)
		} else {
			saved, err = q.SetEmailVerifyProblem(ctx, store.SetEmailVerifyProblemParams{WorkspaceID: p.workspaceID, ChannelID: c.ID, Problem: &problemText, Now: &now})
			if store.IsNotFound(err) {
				saved, err = q.GetEmailChannel(ctx, store.GetEmailChannelParams{WorkspaceID: p.workspaceID, ChannelID: c.ID})
			}
		}
		if err != nil {
			return err
		}
		out, err = s.oneChannelBodyWith(ctx, q, c, saved)
		return err
	})
	if store.IsNotFound(err) {
		return nil, errChannelGone
	}
	if err != nil {
		return nil, err
	}
	return oas.VerifyEmailChannel200JSONResponse(out), nil
}

func (s *Server) verifyByCode(ctx context.Context, p principal, c store.Channel, code string) (oas.VerifyEmailChannelResponseObject, error) {
	now := s.now()
	wrong := false
	var out oas.Channel
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		cur, err := q.LockEmailChannel(ctx, store.LockEmailChannelParams{WorkspaceID: p.workspaceID, ChannelID: c.ID})
		if store.IsNotFound(err) {
			return errChannelGone
		}
		if err != nil {
			return err
		}
		if cur.VerifiedAt == nil {
			if cur.VerifyCodeHash == nil || cur.VerifyCodeExpiresAt == nil || !cur.VerifyCodeExpiresAt.After(now) ||
				!hmac.Equal(cur.VerifyCodeHash, s.verifyCodeHash(c.ID, code)) {
				wrong = true
				return q.FailEmailVerificationCode(ctx, store.FailEmailVerificationCodeParams{WorkspaceID: p.workspaceID, ChannelID: c.ID, MaxAttempts: verifyCodeMaxAttempts})
			}
			if cur, err = s.proveAddress(ctx, q, &p, cur, verifiedByCode, now); err != nil {
				return err
			}
		}
		out, err = s.oneChannelBodyWith(ctx, q, c, cur)
		return err
	})
	if err != nil {
		return nil, err
	}
	if wrong {
		return nil, errVerifyCodeInvalid
	}
	return oas.VerifyEmailChannel200JSONResponse(out), nil
}

// proveAddress verifies e's address for its workspace and takes it from whichever channel held
// it, in any workspace; p is nil when Yuva verified it by itself.
func (s *Server) proveAddress(ctx context.Context, q *store.Queries, p *principal, e store.EmailChannel, method string, now time.Time) (store.EmailChannel, error) {
	if err := q.LockEmailAddress(ctx, e.Address); err != nil {
		return e, err
	}
	lost, err := q.ReleaseEmailAddress(ctx, store.ReleaseEmailAddressParams{Address: e.Address, WorkspaceID: e.WorkspaceID, ChannelID: e.ChannelID, Now: &now})
	if err != nil {
		return e, err
	}
	for _, l := range lost {
		if err := writeSystemAudit(ctx, q, l.WorkspaceID, now, oas.AuditChannelAddressLost, oas.AuditTargetChannel, l.ChannelID, auditDetails{"address": e.Address}); err != nil {
			return e, err
		}
	}
	saved, err := q.MarkEmailChannelVerified(ctx, store.MarkEmailChannelVerifiedParams{
		WorkspaceID: e.WorkspaceID, ChannelID: e.ChannelID, Address: e.Address, Method: &method, Now: &now,
	})
	if err != nil {
		return e, err
	}
	details := auditDetails{"address": e.Address, "method": method}
	if p == nil {
		err = writeSystemAudit(ctx, q, e.WorkspaceID, now, oas.AuditChannelAddressVerified, oas.AuditTargetChannel, e.ChannelID, details)
	} else {
		err = s.audit(ctx, q, *p, oas.AuditChannelAddressVerified, oas.AuditTargetChannel, e.ChannelID, details)
	}
	return saved, err
}

func (s *Server) lookupEmailProof(ctx context.Context, domain, value string) (bool, string) {
	r := s.status.Resolver
	if r == nil {
		r = net.DefaultResolver
	}
	ctx, cancel := context.WithTimeout(ctx, statusDomainLookup)
	defer cancel()
	name := emailProofTXT + domain
	txts, err := r.LookupTXT(ctx, name)
	switch {
	case err != nil && !dnsMissing(err):
		return false, "DNS lookup of " + name + " failed: " + err.Error()
	case slices.ContainsFunc(txts, func(t string) bool { return strings.TrimSpace(t) == value }):
		return true, ""
	case len(txts) > 0:
		return false, "the TXT record " + name + " does not hold the verification value"
	}
	return false, "no TXT record " + name
}

// takeAddressCode verifies the channel when our own code mail to its address comes back through
// its forwarding address: whoever reads that mailbox forwards it to this channel. The mail is never
// stored.
func (s *Server) takeAddressCode(ctx context.Context, ch store.FindEmailChannelByAddressRow, m *email.Message) (bool, error) {
	if m.MessageID == "" || ch.VerifyMessageID == nil {
		return false, nil
	}
	if m.MessageID != *ch.VerifyMessageID && (ch.VerifyRelayMessageID == nil || m.MessageID != *ch.VerifyRelayMessageID) {
		return false, nil
	}
	now := s.now()
	if ch.VerifiedAt != nil || ch.VerifyCodeExpiresAt == nil || !ch.VerifyCodeExpiresAt.After(now) {
		return true, nil
	}
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		cur, err := q.LockEmailChannel(ctx, store.LockEmailChannelParams{WorkspaceID: ch.WorkspaceID, ChannelID: ch.ChannelID})
		if store.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if cur.VerifiedAt != nil || cur.VerifyMessageID == nil || *cur.VerifyMessageID != *ch.VerifyMessageID {
			return nil
		}
		_, err = s.proveAddress(ctx, q, nil, cur, verifiedByCode, now)
		return err
	})
	return true, err
}
