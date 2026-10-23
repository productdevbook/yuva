package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
	"uuid"

	"github.com/emersion/go-msgauth/dmarc"
	"github.com/riverqueue/river"

	"github.com/productdevbook/yuva/api/internal/email"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	probeTimeout      = 10 * time.Minute
	probeDNSTimeout   = 5 * time.Second
	probeAttempts     = 3
	probeSubject      = "Yuva deliverability check"
	probeText         = "This message checks how mail from this channel is authenticated. Yuva reads it when it arrives and keeps nothing of it.\n"
	probeListName     = "Deliverability check"
	checksPerHour     = 6
	probeStateQueued  = "queued"
	probeStateSent    = "sent"
	probeStateTimeout = "timeout"
)

var (
	errNeverChecked    = problem(http.StatusNotFound, "not_found", "the channel's deliverability was never checked")
	errNotEmailChannel = problem(http.StatusConflict, "email_channel_required", "only an e-mail channel that can send has a deliverability check and a DKIM key")
	errDkimExists      = problem(http.StatusConflict, "dkim_exists", "the channel has a DKIM key; remove it before creating another")
	errDkimDNSMissing  = problem(http.StatusConflict, "dkim_dns_missing", "DNS has no TXT record with the channel's DKIM public key at its dns_name yet; publish it and try again")
	errDkimSES         = problem(http.StatusConflict, "dkim_relay_unsupported", dkimSESReason)
	errDkimNoKey       = errValidation("email.dkim.enabled: create a key first with POST /v1/channels/{id}/dkim-key")
	dkimSelector       = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
)

// DNSResolver looks up the records a deliverability check reads; net.Resolver is one.
type DNSResolver interface {
	LookupTXT(ctx context.Context, name string) ([]string, error)
	LookupMX(ctx context.Context, name string) ([]*net.MX, error)
}

func (s *Server) resolver() DNSResolver {
	if s.broadcasts.Resolver != nil {
		return s.broadcasts.Resolver
	}
	return net.DefaultResolver
}

func dkimSecretContext(workspaceID, channelID uuid.UUID) []byte {
	return append(append([]byte("dkim:"), workspaceID[:]...), channelID[:]...)
}

// sendingDomain is the domain of the address a channel's broadcasts go out from.
func sendingDomain(e store.EmailChannel) string {
	if e.FromAddress != nil {
		return email.Domain(*e.FromAddress)
	}
	return email.Domain(e.Address)
}

const dkimSESReason = "the channel's broadcasts go through Amazon SES, which rewrites From, To, Reply-To and Date after any signature added before it and signs with your verified domain itself; Yuva DKIM is for other relays"

// broadcastsViaSES reports whether the account that sends the channel's broadcasts is Amazon SES.
func broadcastsViaSES(e store.EmailChannel) bool {
	if e.BroadcastSmtpHost != nil && *e.BroadcastSmtpHost != "" {
		return email.IsSESHost(*e.BroadcastSmtpHost)
	}
	return email.IsSESHost(e.SmtpHost)
}

func emailDkimBody(e store.EmailChannel) *oas.EmailDkim {
	if e.DkimSelector == nil || len(e.DkimPublicKey) == 0 {
		return nil
	}
	out := &oas.EmailDkim{
		Selector: *e.DkimSelector, Enabled: e.DkimEnabled,
		DnsName: email.DKIMRecordName(*e.DkimSelector, sendingDomain(e)), DnsValue: email.DKIMRecord(e.DkimPublicKey),
	}
	if broadcastsViaSES(e) {
		out.UnsupportedReason = new(dkimSESReason)
	}
	return out
}

// dkimKey is the key a channel's mail through the relay at host is signed with; nil when it has
// none enabled or the relay is Amazon SES.
func (s *Server) dkimKey(e store.EmailChannel, from, host string) (*email.DKIMKey, error) {
	if !e.DkimEnabled || e.DkimSelector == nil || len(e.DkimPrivateKey) == 0 || email.IsSESHost(host) {
		return nil, nil
	}
	plain, err := s.secrets.Open(e.DkimPrivateKey, dkimSecretContext(e.WorkspaceID, e.ChannelID))
	if err != nil {
		return nil, errors.New("the DKIM key cannot be decrypted with this master key")
	}
	signer, err := email.ParseDKIMKey(plain)
	if err != nil {
		return nil, err
	}
	return &email.DKIMKey{Domain: email.Domain(from), Selector: *e.DkimSelector, Signer: signer}, nil
}

func signWith(k *email.DKIMKey, raw []byte) ([]byte, error) {
	if k == nil {
		return raw, nil
	}
	return email.SignDKIM(raw, *k)
}

func (s *Server) lookupTXT(ctx context.Context, name string) []string {
	ctx, cancel := context.WithTimeout(ctx, probeDNSTimeout)
	defer cancel()
	txts, err := s.resolver().LookupTXT(ctx, name)
	if err != nil {
		return nil
	}
	return txts
}

func (s *Server) dkimPublished(ctx context.Context, e store.EmailChannel) bool {
	if e.DkimSelector == nil || len(e.DkimPublicKey) == 0 {
		return false
	}
	return email.DKIMRecordMatches(s.lookupTXT(ctx, email.DKIMRecordName(*e.DkimSelector, sendingDomain(e))), e.DkimPublicKey)
}

// applyDkimInput turns signing on or off after the channel's e-mail settings were saved. Turning
// it on needs the key in DNS; a new sending domain turns it off unless the input turns it on.
func (s *Server) applyDkimInput(ctx context.Context, q *store.Queries, prev *store.EmailChannel, e store.EmailChannel, in *oas.EmailDkimInput) error {
	enabled := e.DkimEnabled
	switch {
	case in != nil && in.Enabled:
		if len(e.DkimPrivateKey) == 0 {
			return errDkimNoKey
		}
		if broadcastsViaSES(e) {
			return errDkimSES
		}
		if !s.dkimPublished(ctx, e) {
			return errDkimDNSMissing
		}
		enabled = true
	case in != nil:
		enabled = false
	case prev != nil && e.DkimEnabled && sendingDomain(*prev) != sendingDomain(e):
		enabled = false
	}
	if enabled == e.DkimEnabled {
		return nil
	}
	_, err := q.SetEmailChannelDkimEnabled(ctx, store.SetEmailChannelDkimEnabledParams{WorkspaceID: e.WorkspaceID, ChannelID: e.ChannelID, Enabled: enabled})
	return err
}

func (s *Server) visibleEmailChannel(ctx context.Context, p principal, id uuid.UUID) (store.Channel, store.EmailChannel, error) {
	c, err := s.visibleChannel(ctx, p, id)
	if err != nil {
		return c, store.EmailChannel{}, err
	}
	if c.Kind != string(oas.ChannelKindEmail) {
		return c, store.EmailChannel{}, errNotEmailChannel
	}
	e, err := s.st.GetEmailChannel(ctx, store.GetEmailChannelParams{WorkspaceID: p.workspaceID, ChannelID: c.ID})
	if store.IsNotFound(err) {
		return c, e, errNotEmailChannel
	}
	return c, e, err
}

func (s *Server) CreateDkimKey(ctx context.Context, req oas.CreateDkimKeyRequestObject) (oas.CreateDkimKeyResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	selector := strings.TrimSpace(req.Body.Selector)
	if !dkimSelector.MatchString(selector) {
		return nil, errValidation("selector is a DNS label: 1 to 63 lower-case letters, digits and inner hyphens")
	}
	c, e, err := s.visibleEmailChannel(ctx, p, req.ChannelId)
	if err != nil {
		return nil, err
	}
	if len(e.DkimPrivateKey) > 0 {
		return nil, errDkimExists
	}
	private, public, err := email.NewDKIMKey()
	if err != nil {
		return nil, err
	}
	err = s.st.InTx(ctx, func(q *store.Queries) error {
		var err error
		e, err = q.CreateEmailChannelDkimKey(ctx, store.CreateEmailChannelDkimKeyParams{
			WorkspaceID: p.workspaceID, ChannelID: c.ID, Selector: &selector,
			PrivateKey: s.secrets.Seal(private, dkimSecretContext(p.workspaceID, c.ID)), PublicKey: public,
		})
		if err != nil {
			return err
		}
		return s.audit(ctx, q, p, oas.AuditChannelDkimKeyCreated, oas.AuditTargetChannel, c.ID, auditDetails{"selector": selector})
	})
	if store.IsNotFound(err) {
		return nil, errDkimExists
	}
	if err != nil {
		return nil, err
	}
	return oas.CreateDkimKey201JSONResponse(*emailDkimBody(e)), nil
}

func (s *Server) DeleteDkimKey(ctx context.Context, req oas.DeleteDkimKeyRequestObject) (oas.DeleteDkimKeyResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	c, e, err := s.visibleEmailChannel(ctx, p, req.ChannelId)
	if err != nil {
		return nil, err
	}
	err = s.st.InTx(ctx, func(q *store.Queries) error {
		if err := q.DeleteEmailChannelDkimKey(ctx, store.DeleteEmailChannelDkimKeyParams{WorkspaceID: p.workspaceID, ChannelID: c.ID}); err != nil {
			return err
		}
		if len(e.DkimPrivateKey) == 0 {
			return nil
		}
		return s.audit(ctx, q, p, oas.AuditChannelDkimKeyDeleted, oas.AuditTargetChannel, c.ID, nil)
	})
	if err != nil {
		return nil, err
	}
	return oas.DeleteDkimKey204Response{}, nil
}

// probeResult is a check's stored result. The probe job and the ingress write disjoint top-level
// keys, merged with `||`, since the probe can arrive before the job has saved what it sent.
type probeResult struct {
	Domain string     `json:"domain,omitempty"`
	TLS    *bool      `json:"tls,omitempty"`
	Error  string     `json:"error,omitempty"`
	DNS    *probeDNS  `json:"dns,omitempty"`
	Auth   *probeAuth `json:"auth,omitempty"`
}

type probeDNS struct {
	SPFRecord   string                      `json:"spf_record,omitempty"`
	DMARCRecord string                      `json:"dmarc_record,omitempty"`
	DMARCPolicy string                      `json:"dmarc_policy,omitempty"`
	MX          *oas.DeliverabilityMx       `json:"mx,omitempty"`
	YuvaDKIM    *oas.DeliverabilityYuvaDkim `json:"yuva_dkim,omitempty"`
}

type probeAuth struct {
	SPF   oas.DeliverabilitySpf  `json:"spf"`
	DKIM  oas.DeliverabilityDkim `json:"dkim"`
	DMARC oas.AuthStatus         `json:"dmarc"`
}

func parseProbeResult(raw []byte) probeResult {
	var r probeResult
	_ = json.Unmarshal(raw, &r)
	return r
}

func (s *Server) probeState(c store.DeliverabilityCheck) string {
	if (c.State == probeStateQueued || c.State == probeStateSent) && s.now().Sub(c.CreatedAt) > probeTimeout {
		return probeStateTimeout
	}
	return c.State
}

func (s *Server) deliverabilityBody(c store.DeliverabilityCheck) oas.DeliverabilityCheck {
	r := parseProbeResult(c.Result)
	out := oas.DeliverabilityCheck{
		State: oas.DeliverabilityState(s.probeState(c)), Domain: r.Domain, Tls: r.TLS, CreatedAt: c.CreatedAt, CheckedAt: c.FinishedAt,
		Spf:   oas.DeliverabilitySpf{Status: oas.AuthUnknown},
		Dkim:  oas.DeliverabilityDkim{Status: oas.AuthUnknown, Signatures: []oas.DkimSignature{}},
		Dmarc: oas.DeliverabilityDmarc{Status: oas.AuthUnknown},
	}
	if r.Error != "" {
		out.Error = new(r.Error)
	}
	if a := r.Auth; a != nil {
		out.Spf, out.Dkim, out.Dmarc.Status = a.SPF, a.DKIM, a.DMARC
		if out.Dkim.Signatures == nil {
			out.Dkim.Signatures = []oas.DkimSignature{}
		}
	}
	if d := r.DNS; d != nil {
		if d.SPFRecord != "" {
			out.Spf.Record = new(d.SPFRecord)
		}
		if d.DMARCRecord != "" {
			out.Dmarc.Record = new(d.DMARCRecord)
		}
		if d.DMARCPolicy != "" {
			out.Dmarc.Policy = new(oas.DmarcPolicy(d.DMARCPolicy))
		}
		out.Mx, out.YuvaDkim = d.MX, d.YuvaDKIM
	}
	return out
}

func (s *Server) GetDeliverability(ctx context.Context, req oas.GetDeliverabilityRequestObject) (oas.GetDeliverabilityResponseObject, error) {
	p := principalFrom(ctx)
	c, err := s.visibleChannel(ctx, p, req.ChannelId)
	if err != nil {
		return nil, err
	}
	row, err := s.st.GetDeliverabilityCheck(ctx, store.GetDeliverabilityCheckParams{WorkspaceID: p.workspaceID, ChannelID: c.ID})
	if store.IsNotFound(err) {
		return nil, errNeverChecked
	}
	if err != nil {
		return nil, err
	}
	return oas.GetDeliverability200JSONResponse(s.deliverabilityBody(row)), nil
}

type DeliverabilityProbeArgs struct {
	WorkspaceID uuid.UUID `json:"workspace_id"`
	ChannelID   uuid.UUID `json:"channel_id"`
	Token       string    `json:"token"`
}

func (DeliverabilityProbeArgs) Kind() string { return "deliverability_probe" }

type deliverabilityProbeWorker struct {
	river.WorkerDefaults[DeliverabilityProbeArgs]
	s *Server
}

func (w *deliverabilityProbeWorker) Timeout(*river.Job[DeliverabilityProbeArgs]) time.Duration {
	return 2 * time.Minute
}

func (w *deliverabilityProbeWorker) Work(ctx context.Context, job *river.Job[DeliverabilityProbeArgs]) error {
	return w.s.SendDeliverabilityProbe(ctx, job.Args, job.Attempt >= job.MaxAttempts)
}

func (s *Server) CheckDeliverability(ctx context.Context, req oas.CheckDeliverabilityRequestObject) (oas.CheckDeliverabilityResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	c, e, err := s.visibleEmailChannel(ctx, p, req.ChannelId)
	if err != nil {
		return nil, err
	}
	_, from, ok, err := s.broadcastAccount(e)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errNotEmailChannel
	}
	if err := s.rateLimit(rateCheck{"deliverability:" + c.ID.String(), limit{checksPerHour, time.Hour}}); err != nil {
		return nil, err
	}
	token := email.NewConversationToken()
	var out store.DeliverabilityCheck
	err = s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		out, err = q.StartDeliverabilityCheck(ctx, store.StartDeliverabilityCheckParams{
			WorkspaceID: p.workspaceID, ChannelID: c.ID, ProbeToken: token,
			Result: mustJSON(probeResult{Domain: email.Domain(from)}), Now: s.now(),
		})
		if err != nil {
			return err
		}
		events.job(DeliverabilityProbeArgs{WorkspaceID: p.workspaceID, ChannelID: c.ID, Token: token}, &river.InsertOpts{MaxAttempts: probeAttempts})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return oas.CheckDeliverability202JSONResponse(s.deliverabilityBody(out)), nil
}

// probeDNSRecords looks up what the receiving side of a broadcast reads in DNS: the sending
// domain's SPF and DMARC records, the reply domain's MX and the channel's Yuva DKIM record.
func (s *Server) probeDNSRecords(ctx context.Context, e store.EmailChannel, fromDomain, replyDomain string) probeDNS {
	var out probeDNS
	for _, t := range s.lookupTXT(ctx, fromDomain) {
		if strings.HasPrefix(strings.ToLower(t), "v=spf1") {
			out.SPFRecord = t
			break
		}
	}
	dmarcAt := func(domain string, sub bool) bool {
		for _, t := range s.lookupTXT(ctx, "_dmarc."+domain) {
			if !strings.HasPrefix(t, "v=DMARC1") {
				continue
			}
			out.DMARCRecord = t
			if rec, err := dmarc.Parse(t); err == nil {
				policy := rec.Policy
				if sub && rec.SubdomainPolicy != "" {
					policy = rec.SubdomainPolicy
				}
				if oas.DmarcPolicy(policy).Valid() {
					out.DMARCPolicy = string(policy)
				}
			}
			return true
		}
		return false
	}
	if !dmarcAt(fromDomain, false) {
		if org := email.OrgDomain(fromDomain); org != fromDomain {
			dmarcAt(org, true)
		}
	}
	mx := &oas.DeliverabilityMx{Domain: replyDomain, Hosts: []string{}}
	mctx, cancel := context.WithTimeout(ctx, probeDNSTimeout)
	if hosts, err := s.resolver().LookupMX(mctx, replyDomain); err == nil {
		for _, h := range hosts {
			if host := strings.TrimSuffix(h.Host, "."); host != "" {
				mx.Hosts = append(mx.Hosts, host)
			}
		}
	}
	cancel()
	out.MX = mx
	if e.DkimSelector != nil && len(e.DkimPublicKey) > 0 {
		name := email.DKIMRecordName(*e.DkimSelector, fromDomain)
		out.YuvaDKIM = &oas.DeliverabilityYuvaDkim{
			Selector: *e.DkimSelector, DnsName: name, Published: email.DKIMRecordMatches(s.lookupTXT(ctx, name), e.DkimPublicKey),
		}
	}
	return out
}

// SendDeliverabilityProbe sends a check's probe through the channel's broadcast account to the
// channel's own address, with the headers a broadcast carries, and records the DNS it found.
func (s *Server) SendDeliverabilityProbe(ctx context.Context, a DeliverabilityProbeArgs, lastAttempt bool) error {
	ws := a.WorkspaceID
	if live, err := s.workspaceLive(ctx, ws); err != nil || !live {
		return err
	}
	row, err := s.st.GetDeliverabilityCheck(ctx, store.GetDeliverabilityCheckParams{WorkspaceID: ws, ChannelID: a.ChannelID})
	if store.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if row.ProbeToken != a.Token || row.State != probeStateQueued {
		return nil
	}
	fail := func(why string) error {
		_, err := s.st.FailDeliverabilityCheck(ctx, store.FailDeliverabilityCheckParams{
			WorkspaceID: ws, ChannelID: a.ChannelID, ProbeToken: a.Token, Result: mustJSON(probeResult{Error: truncateRunes(why, 500)}), Now: new(s.now()),
		})
		if store.IsNotFound(err) {
			return nil
		}
		return err
	}
	e, err := s.st.GetEmailChannel(ctx, store.GetEmailChannelParams{WorkspaceID: ws, ChannelID: a.ChannelID})
	if store.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	cfg, from, ok, err := s.broadcastAccount(e)
	if err != nil {
		return fail(err.Error())
	}
	if !ok {
		return fail("the channel has no SMTP account or address to send from")
	}
	name := e.DisplayName
	if name == "" {
		if c, err := s.st.GetChannel(ctx, store.GetChannelParams{WorkspaceID: ws, ID: e.ChannelID}); err == nil {
			name = c.Name
		}
	}
	to := e.Address
	if isCatchAll(to) {
		to = from
	}
	domain := email.Domain(from)
	dns := s.probeDNSRecords(ctx, e, domain, email.Domain(to))
	manage := s.manageURL(ws, uuid.Nil(), uuid.Nil(), uuid.Nil())
	listID := "<l000000000000." + domain + ">"
	sesH, err := s.sesHeaders(ctx, e, cfg)
	if err != nil {
		return err
	}
	raw, err := email.Build(email.Outgoing{
		From: email.Address{Name: name, Email: from}, ReplyTo: email.Address{Name: name, Email: to}, To: email.Address{Email: to},
		Subject: probeSubject, Text: probeText, Date: s.now(), MessageID: email.NewMessageID(a.Token, domain),
		Headers: append([]email.Header{
			{Name: "List-Unsubscribe", Value: "<" + manage + ">"},
			{Name: "List-Unsubscribe-Post", Value: "List-Unsubscribe=One-Click"},
			{Name: "X-Auto-Response-Suppress", Value: "OOF, AutoReply"},
			{Name: "List-Id", Value: headerPhrase(probeListName) + " " + listID},
			{Name: "Feedback-ID", Value: "00000000:00000000:" + hex8(ws) + ":" + feedbackSenderID},
			{Name: email.ProbeHeader, Value: a.Token},
		}, sesH...),
	})
	if err != nil {
		return err
	}
	key, err := s.dkimKey(e, from, cfg.Host)
	if err != nil {
		return fail(err.Error())
	}
	if raw, err = signWith(key, raw); err != nil {
		return err
	}
	sendCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	if err := s.sender.Send(sendCtx, cfg, from, []string{to}, raw); err != nil {
		if email.IsPermanent(err) || lastAttempt {
			return fail(err.Error())
		}
		return err
	}
	tls := cfg.TLS != smtpTLSNone
	_, err = s.st.SaveDeliverabilitySent(ctx, store.SaveDeliverabilitySentParams{
		WorkspaceID: ws, ChannelID: a.ChannelID, ProbeToken: a.Token, Result: mustJSON(probeResult{Domain: domain, TLS: &tls, DNS: &dns}),
	})
	if store.IsNotFound(err) {
		return nil
	}
	return err
}

func authStatus(v string) oas.AuthStatus {
	st := oas.AuthStatus(strings.ToLower(v))
	if v == "hardfail" {
		st = oas.AuthFail
	}
	if !st.Valid() {
		return oas.AuthUnknown
	}
	return st
}

// readProbe judges a probe from the trusted Authentication-Results and the DKIM signatures it
// carries; ok is false when the results cannot be trusted.
func readProbe(m *email.Message, authservID string) (probeAuth, bool) {
	v, ok := email.TrustedAuth(m.AllAuthResults, authservID)
	if !ok {
		return probeAuth{}, false
	}
	from := email.Domain(m.From.Email)
	out := probeAuth{
		SPF:   oas.DeliverabilitySpf{Status: authStatus(v.SPF), Aligned: email.Aligned(v.SPFDomain, from)},
		DKIM:  oas.DeliverabilityDkim{Status: oas.AuthUnknown, Signatures: []oas.DkimSignature{}},
		DMARC: authStatus(v.DMARC),
	}
	if v.SPFDomain != "" {
		out.SPF.Domain = new(v.SPFDomain)
	}
	used := make([]bool, len(v.DKIM))
	aligned := -1
	for _, raw := range m.DKIMSignatures {
		sig, ok := email.ParseDKIMSignature(raw)
		if !ok {
			continue
		}
		d := oas.DkimSignature{Domain: sig.Domain, Status: oas.AuthUnknown, Aligned: email.Aligned(sig.Domain, from), CoversListUnsubscribe: sig.CoversListUnsubscribe()}
		if sig.Selector != "" {
			d.Selector = new(sig.Selector)
		}
		for i, r := range v.DKIM {
			if !used[i] && r.Domain == sig.Domain {
				used[i], d.Status = true, authStatus(r.Result)
				break
			}
		}
		if d.Aligned && (aligned < 0 || (out.DKIM.Signatures[aligned].Status != oas.AuthPass && d.Status == oas.AuthPass)) {
			aligned = len(out.DKIM.Signatures)
		}
		out.DKIM.Signatures = append(out.DKIM.Signatures, d)
	}
	switch {
	case aligned >= 0:
		a := out.DKIM.Signatures[aligned]
		out.DKIM.Status, out.DKIM.Domain = a.Status, new(a.Domain)
		out.DKIM.Aligned = a.Status == oas.AuthPass
		out.DKIM.CoversListUnsubscribe = out.DKIM.Aligned && a.CoversListUnsubscribe
	case len(out.DKIM.Signatures) > 0:
		first := out.DKIM.Signatures[0]
		out.DKIM.Status, out.DKIM.Domain = first.Status, new(first.Domain)
	case len(v.DKIM) > 0:
		out.DKIM.Status = authStatus(v.DKIM[0].Result)
		if v.DKIM[0].Domain != "" {
			out.DKIM.Domain = new(v.DKIM[0].Domain)
		}
	}
	return out, true
}

// takeProbe records a deliverability probe that reached a channel and reports it consumed; the
// probe is recognised by its token before any other rule, and nothing of it is stored.
func (s *Server) takeProbe(ctx context.Context, ws uuid.UUID, m *email.Message) (bool, error) {
	var tokens []string
	if t, _, ok := email.TokenFromID(m.MessageID); ok {
		tokens = append(tokens, t)
	}
	if t := strings.ToLower(m.Probe); t != "" && (len(tokens) == 0 || tokens[0] != t) {
		tokens = append(tokens, t)
	}
	for _, t := range tokens {
		row, err := s.st.GetDeliverabilityProbe(ctx, store.GetDeliverabilityProbeParams{WorkspaceID: ws, ProbeToken: t})
		if store.IsNotFound(err) {
			continue
		}
		if err != nil {
			return false, err
		}
		if row.State != probeStateQueued && row.State != probeStateSent {
			return true, nil
		}
		state := string(oas.DeliverabilityUnverifiable)
		res := probeResult{}
		if auth, ok := readProbe(m, s.ingress.AuthservID); ok {
			state, res.Auth = string(oas.DeliverabilityReceived), &auth
		}
		_, err = s.st.FinishDeliverabilityProbe(ctx, store.FinishDeliverabilityProbeParams{
			WorkspaceID: ws, ProbeToken: t, Result: mustJSON(res), State: state, Now: new(s.now()),
		})
		if err != nil && !store.IsNotFound(err) {
			return false, err
		}
		s.log.InfoContext(ctx, "deliverability probe received", slog.String("channel_id", row.ChannelID.String()), slog.String("state", state))
		return true, nil
	}
	return false, nil
}

// deliverabilityReview is the broadcast review item of the channel's last check: a warning when it
// never ran, did not finish or found no DMARC record, blocking when no DKIM signature passed.
func (s *Server) deliverabilityReview(ctx context.Context, q *store.Queries, e store.EmailChannel) (oas.BroadcastReviewStatus, string, error) {
	row, err := q.GetDeliverabilityCheck(ctx, store.GetDeliverabilityCheckParams{WorkspaceID: e.WorkspaceID, ChannelID: e.ChannelID})
	if store.IsNotFound(err) {
		return oas.ReviewWarning, "the channel's deliverability was never checked; run the check (Settings, channel, Deliverability)", nil
	}
	if err != nil {
		return "", "", err
	}
	c := s.deliverabilityBody(row)
	var warnings []string
	switch c.State {
	case oas.DeliverabilityQueued, oas.DeliverabilitySent:
		return oas.ReviewWarning, "the deliverability check has not finished yet", nil
	case oas.DeliverabilityTimeout:
		return oas.ReviewWarning, "the last deliverability check's probe did not arrive within 10 minutes; check again", nil
	case oas.DeliverabilityFailed:
		return oas.ReviewWarning, "the last deliverability check could not send its probe: " + deref(c.Error), nil
	case oas.DeliverabilityUnverifiable:
		warnings = append(warnings, "the receiving server's results could not be read, so only DNS was checked")
	case oas.DeliverabilityReceived:
		fix := "turn on Yuva DKIM for the channel"
		if broadcastsViaSES(e) {
			fix = "verify the sending domain with Easy DKIM in Amazon SES"
		}
		if c.Dkim.Status != oas.AuthPass {
			return oas.ReviewBlocking, "the last deliverability check found no passing DKIM signature; set up DKIM with the SMTP provider or " + fix, nil
		}
		if !c.Dkim.Aligned {
			warnings = append(warnings, "DKIM passes for "+deref(c.Dkim.Domain)+", not for the sending domain "+c.Domain)
		} else if !c.Dkim.CoversListUnsubscribe {
			warnings = append(warnings, "the DKIM signature does not cover List-Unsubscribe and List-Unsubscribe-Post; "+fix+" or ask the provider")
		}
	}
	if c.Dmarc.Record == nil {
		warnings = append(warnings, "the sending domain "+c.Domain+" has no DMARC record; publish one, p=none at least")
	}
	if len(warnings) > 0 {
		return oas.ReviewWarning, strings.Join(warnings, "; "), nil
	}
	return oas.ReviewOk, "", nil
}
