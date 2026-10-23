package api

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/email"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	suppressBounce    = "bounce"
	suppressComplaint = "complaint"
	maxSNSBodyBytes   = 256 << 10
)

type bounced struct {
	email     string
	reason    string
	detail    string
	permanent bool
}

// recordBounce marks the permanently failed addresses undeliverable in the workspace and, when the
// original message is known and failMessage is set, the message failed.
func (s *Server) recordBounce(ctx context.Context, ws uuid.UUID, messageID *uuid.UUID, recipients []bounced, failMessage bool) error {
	return s.inTx(ctx, ws, func(q *store.Queries, events *eventBatch) error {
		var details []string
		for _, r := range recipients {
			if r.detail != "" {
				details = append(details, r.detail)
			}
			if !r.permanent {
				continue
			}
			if err := q.SuppressEmail(ctx, store.SuppressEmailParams{
				WorkspaceID: ws, Email: r.email, Reason: r.reason, Detail: truncateRunes(r.detail, 2000), Now: s.now(),
			}); err != nil {
				return err
			}
			if r.reason == suppressComplaint {
				if err := s.complainedAbout(ctx, q, events, ws, r.email); err != nil {
					return err
				}
			}
			id, err := q.GetContactIDByEmail(ctx, store.GetContactIDByEmailParams{WorkspaceID: ws, Email: r.email})
			if store.IsNotFound(err) {
				continue
			}
			if err != nil {
				return err
			}
			row, err := q.GetContact(ctx, store.GetContactParams{WorkspaceID: ws, ID: id})
			if err != nil {
				return err
			}
			body, err := s.contactBody(ctx, q, ws, allInboxScope, row)
			if err != nil {
				return err
			}
			events.add(realtime.ContactUpdated, nil, nil, body)
		}
		if messageID == nil || !failMessage {
			return nil
		}
		detail := "The message bounced"
		if len(details) > 0 {
			detail += ": " + strings.Join(details, "; ")
		}
		detail = truncateRunes(detail, maxDeliveryError)
		return s.setDeliveryTx(ctx, q, events, ws, *messageID, deliveryFailed, &detail)
	})
}

// applyDSN handles a delivery report only when it names a message we sent from this workspace to the
// one address it reports; anything else is not ours to act on and handled is false.
func (s *Server) applyDSN(ctx context.Context, ch store.FindEmailChannelByAddressRow, m *email.Message) (IngestResult, bool, error) {
	var sent *store.FindOutboundEmailByHeaderRow
	for _, id := range slices.Concat([]string{m.Bounce.OriginalMessageID}, m.InReplyTo, m.References) {
		if id == "" {
			continue
		}
		row, err := s.st.FindOutboundEmailByHeader(ctx, store.FindOutboundEmailByHeaderParams{WorkspaceID: ch.WorkspaceID, HeaderMessageID: id})
		if err == nil {
			sent = &row
			break
		}
		if !store.IsNotFound(err) {
			return IngestResult{}, false, err
		}
	}
	if sent == nil {
		return s.applyBroadcastDSN(ctx, ch, m)
	}
	failed := m.Bounce.Failed()
	if len(failed) == 0 {
		return IngestResult{Status: IngestDropped}, true, nil
	}
	to := soleRecipient(*sent)
	i := slices.IndexFunc(failed, func(r email.BounceRecipient) bool { return to != "" && r.Email == to })
	if i < 0 {
		return IngestResult{}, false, nil
	}
	if ok, err := s.reportConcerns(ctx, ch.WorkspaceID, to, sent.HeaderMessageID, sent.RelayMessageID); err != nil || !ok {
		return IngestResult{}, false, err
	}
	r := failed[i]
	recipients := []bounced{{email: to, reason: suppressBounce, detail: strings.TrimSpace(r.Status + " " + r.Diagnostic), permanent: r.Permanent()}}
	if err := s.recordBounce(ctx, ch.WorkspaceID, &sent.MessageID, recipients, true); err != nil {
		return IngestResult{}, false, err
	}
	return IngestResult{Status: IngestBounce, ConversationID: &sent.ConversationID, MessageID: &sent.MessageID}, true, nil
}

func soleRecipient(sent store.FindOutboundEmailByHeaderRow) string {
	if sent.Shared || len(sent.ToAddresses) != 1 || len(sent.CcAddresses) != 0 || len(sent.BccAddresses) != 0 {
		return ""
	}
	return sent.ToAddresses[0]
}

// Anyone given a Message-ID can name it in a report, so only its sole holder's reports count.
func (s *Server) reportConcerns(ctx context.Context, ws uuid.UUID, addr, headerID string, relayID *string) (bool, error) {
	ids := []string{headerID}
	if relayID != nil && *relayID != "" {
		ids = append(ids, *relayID)
	}
	shared, err := s.st.MessageIDSharedBeyond(ctx, store.MessageIDSharedBeyondParams{WorkspaceID: ws, Ids: ids, Email: addr})
	if err != nil {
		return false, err
	}
	if shared {
		s.log.InfoContext(ctx, "report about mail whose Message-ID others were given", slog.String("message_id", headerID))
	}
	return !shared, nil
}

type certCache struct {
	client *http.Client
	mu     sync.Mutex
	certs  map[string]*x509.Certificate
}

func newCertCache(c *http.Client) *certCache {
	return &certCache{client: c, certs: map[string]*x509.Certificate{}}
}

func (c *certCache) get(ctx context.Context, url string) (*x509.Certificate, error) {
	c.mu.Lock()
	cert, ok := c.certs[url]
	c.mu.Unlock()
	if ok {
		return cert, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	res, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, errors.New("signing certificate: " + res.Status)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("signing certificate is not PEM")
	}
	cert, err = x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	if len(c.certs) > 32 {
		clear(c.certs)
	}
	c.certs[url] = cert
	c.mu.Unlock()
	return cert, nil
}

var (
	errSNSForbidden   = problem(http.StatusForbidden, "forbidden", "the notification is not signed by Amazon SNS for an allowed topic")
	errSNSUnavailable = problem(http.StatusServiceUnavailable, "unavailable", "try again later")
	errSNSStale       = problem(http.StatusForbidden, "stale_notification", "the notification is older than an hour or from the future")
)

const snsMaxAge = time.Hour

// snsFresh refuses replays of captured notifications: SNS delivers within minutes.
func (s *Server) snsFresh(m *email.SNSMessage) bool {
	t, err := time.Parse(time.RFC3339, m.Timestamp)
	if err != nil {
		return false
	}
	now := s.now()
	return now.Sub(t) <= snsMaxAge && t.Sub(now) <= 5*time.Minute
}

func (s *Server) serveIngressSES(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxSNSBodyBytes+1))
	if err != nil || len(body) > maxSNSBodyBytes {
		writeProblem(w, errValidation("the body is not an SNS message"))
		return
	}
	msg, err := email.ParseSNS(body)
	if err != nil {
		writeProblem(w, errValidation("the body is not an SNS message"))
		return
	}
	if !slices.Contains(s.ingress.SESTopicARNs, msg.TopicArn) {
		writeProblem(w, errSNSForbidden)
		return
	}
	if err := msg.Verify(r.Context(), s.snsCerts.get); err != nil {
		if !errors.Is(err, email.ErrSNSSignature) {
			s.log.WarnContext(r.Context(), "sns certificate", slog.Any("error", err))
			writeProblem(w, errSNSUnavailable)
			return
		}
		writeProblem(w, errSNSForbidden)
		return
	}
	switch msg.Type {
	case "SubscriptionConfirmation":
		if !email.ValidSNSURL(msg.SubscribeURL) {
			writeProblem(w, errSNSForbidden)
			return
		}
		if err := s.confirmSubscription(r.Context(), msg.SubscribeURL); err != nil {
			s.log.WarnContext(r.Context(), "sns subscription confirmation", slog.Any("error", err))
			writeProblem(w, errSNSUnavailable)
			return
		}
		s.log.InfoContext(r.Context(), "sns subscription confirmed", slog.String("topic", msg.TopicArn))
	case "Notification":
		if !s.snsFresh(msg) {
			writeProblem(w, errSNSStale)
			return
		}
		if err := s.applySES(r.Context(), msg.Message); err != nil {
			s.log.ErrorContext(r.Context(), "ses notification", slog.Any("error", err))
			writeProblem(w, errSNSUnavailable)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) confirmSubscription(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	res, err := s.fetch.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))
	if res.StatusCode != http.StatusOK {
		return errors.New("SubscribeURL answered " + res.Status)
	}
	return nil
}

func (s *Server) applySES(ctx context.Context, payload string) error {
	var n email.SESNotification
	if err := json.Unmarshal([]byte(payload), &n); err != nil {
		s.log.WarnContext(ctx, "ses notification is not JSON", slog.Any("error", err))
		return nil
	}
	if n.Source == "aws.ses" && n.DetailType != "" {
		return s.applySESTenantEvent(ctx, n.DetailType, n.Resources)
	}
	var recipients []bounced
	failMessage := false
	switch n.Kind() {
	case "Bounce":
		if n.Bounce == nil || n.Bounce.BounceType != "Permanent" {
			return nil
		}
		failMessage = true
		for _, r := range n.Bounce.BouncedRecipients {
			recipients = append(recipients, bounced{
				email: strings.ToLower(strings.TrimSpace(r.EmailAddress)), reason: suppressBounce, permanent: true,
				detail: strings.TrimSpace(r.Status + " " + r.DiagnosticCode),
			})
		}
	case "Complaint":
		if n.Complaint == nil {
			return nil
		}
		for _, r := range n.Complaint.ComplainedRecipients {
			recipients = append(recipients, bounced{
				email: strings.ToLower(strings.TrimSpace(r.EmailAddress)), reason: suppressComplaint, permanent: true,
				detail: n.Complaint.ComplaintFeedbackType,
			})
		}
	default:
		return nil
	}
	id := n.OriginalMessageID()
	if id == "" || len(recipients) == 0 {
		return nil
	}
	tenant := n.TenantName()
	row, err := s.st.FindOutboundEmailAnyWorkspace(ctx, id)
	if store.IsNotFound(err) {
		return s.applyBroadcastSES(ctx, id, recipients, tenant)
	}
	if err != nil {
		return err
	}
	if foreign, err := s.sesTenantForeign(ctx, row.WorkspaceID, row.ChannelID, tenant); err != nil {
		return err
	} else if foreign {
		s.log.WarnContext(ctx, "ses notification of another tenant dropped", slog.String("message_id", id), slog.String("tenant", tenant))
		return nil
	}
	recipients = slices.DeleteFunc(recipients, func(r bounced) bool {
		return !slices.Contains(row.ToAddresses, r.email) && !slices.Contains(row.CcAddresses, r.email) && !slices.Contains(row.BccAddresses, r.email)
	})
	if len(recipients) == 0 {
		s.log.InfoContext(ctx, "ses notification names no recipient of the message", slog.String("message_id", id))
		return nil
	}
	failMessage = failMessage && slices.ContainsFunc(recipients, func(r bounced) bool { return slices.Contains(row.ToAddresses, r.email) })
	return s.recordBounce(ctx, row.WorkspaceID, &row.MessageID, recipients, failMessage)
}

// applyBroadcastSES applies an SES bounce or complaint about broadcast mail, for its recipient only.
func (s *Server) applyBroadcastSES(ctx context.Context, id string, recipients []bounced, tenant string) error {
	d, err := s.st.FindBroadcastDeliveryAnyWorkspace(ctx, id)
	if store.IsNotFound(err) {
		return s.applyNoticeSES(ctx, id, recipients, tenant)
	}
	if err != nil {
		return err
	}
	if tenant != "" {
		b, err := s.st.GetBroadcast(ctx, store.GetBroadcastParams{WorkspaceID: d.WorkspaceID, ID: d.BroadcastID})
		if err != nil && !store.IsNotFound(err) {
			return err
		}
		if foreign, err := s.sesTenantForeign(ctx, d.WorkspaceID, b.ChannelID, tenant); err != nil {
			return err
		} else if foreign {
			s.log.WarnContext(ctx, "ses notification of another tenant dropped", slog.String("message_id", id), slog.String("tenant", tenant))
			return nil
		}
	}
	for _, r := range recipients {
		if r.email == d.Email {
			return s.deliveryFeedback(ctx, d, r.reason == suppressComplaint, r.detail)
		}
	}
	s.log.InfoContext(ctx, "ses notification names no recipient of the message", slog.String("message_id", id))
	return nil
}

// applyBroadcastDSN applies a delivery report about broadcast mail of the channel's workspace: a
// permanent failure of the delivery's own address is a bounce, anything else changes nothing.
func (s *Server) applyBroadcastDSN(ctx context.Context, ch store.FindEmailChannelByAddressRow, m *email.Message) (IngestResult, bool, error) {
	for _, id := range slices.Concat([]string{m.Bounce.OriginalMessageID}, m.InReplyTo, m.References) {
		if id == "" {
			continue
		}
		d, err := s.st.FindBroadcastDeliveryByHeader(ctx, store.FindBroadcastDeliveryByHeaderParams{WorkspaceID: ch.WorkspaceID, HeaderMessageID: id})
		if store.IsNotFound(err) {
			continue
		}
		if err != nil {
			return IngestResult{}, false, err
		}
		for _, r := range m.Bounce.Failed() {
			if r.Email != d.Email || !r.Permanent() {
				continue
			}
			if ok, err := s.reportConcerns(ctx, ch.WorkspaceID, d.Email, d.HeaderMessageID, d.RelayMessageID); err != nil || !ok {
				return IngestResult{}, false, err
			}
			if err := s.deliveryFeedback(ctx, d, false, strings.TrimSpace(r.Status+" "+r.Diagnostic)); err != nil {
				return IngestResult{}, false, err
			}
			break
		}
		return IngestResult{Status: IngestBounce}, true, nil
	}
	return s.applyNoticeDSN(ctx, ch, m)
}

func (s *Server) applyNoticeDSN(ctx context.Context, ch store.FindEmailChannelByAddressRow, m *email.Message) (IngestResult, bool, error) {
	d, err := noticeDeliveryOf(ctx, s.st.Queries, ch.WorkspaceID, slices.Concat([]string{m.Bounce.OriginalMessageID}, m.InReplyTo, m.References))
	if err != nil {
		return IngestResult{}, false, err
	}
	if d == nil {
		return s.applyConfirmationDSN(ctx, m)
	}
	for _, r := range m.Bounce.Failed() {
		if r.Email != d.Email || !r.Permanent() {
			continue
		}
		if ok, err := s.reportConcerns(ctx, ch.WorkspaceID, d.Email, d.HeaderMessageID, d.RelayMessageID); err != nil || !ok {
			return IngestResult{}, false, err
		}
		if err := s.noticeFeedback(ctx, *d, false, strings.TrimSpace(r.Status+" "+r.Diagnostic)); err != nil {
			return IngestResult{}, false, err
		}
		break
	}
	return IngestResult{Status: IngestBounce}, true, nil
}

// SES names only the message, so the delivery is found across workspaces and everything after is
// scoped by its own.
func (s *Server) applyNoticeSES(ctx context.Context, id string, recipients []bounced, tenant string) error {
	d, err := s.st.FindStatusNoticeDeliveryAnyWorkspace(ctx, id)
	if store.IsNotFound(err) {
		return s.applyConfirmationSES(ctx, id, recipients)
	}
	if err != nil {
		return err
	}
	if tenant != "" {
		var channel *uuid.UUID
		if inc, err := s.st.StatusNoticeIncident(ctx, store.StatusNoticeIncidentParams{WorkspaceID: d.WorkspaceID, NoticeID: d.NoticeID}); err == nil {
			if pg, err := s.st.GetStatusPage(ctx, store.GetStatusPageParams{WorkspaceID: d.WorkspaceID, ID: inc.PageID}); err == nil && pg.MailingListID != nil {
				if l, err := s.st.GetMailingList(ctx, store.GetMailingListParams{WorkspaceID: d.WorkspaceID, ID: *pg.MailingListID}); err == nil {
					channel = l.ChannelID
				}
			}
		}
		if foreign, err := s.sesTenantForeign(ctx, d.WorkspaceID, channel, tenant); err != nil {
			return err
		} else if foreign {
			s.log.WarnContext(ctx, "ses notification of another tenant dropped", slog.String("message_id", id), slog.String("tenant", tenant))
			return nil
		}
	}
	for _, r := range recipients {
		if r.email == d.Email {
			return s.noticeFeedback(ctx, d, r.reason == suppressComplaint, r.detail)
		}
	}
	s.log.InfoContext(ctx, "ses notification names no recipient of the message", slog.String("message_id", id))
	return nil
}

// The report reaches the workspace owning the server's address, so the confirmation is found by its Message-ID in any.
func (s *Server) applyConfirmationDSN(ctx context.Context, m *email.Message) (IngestResult, bool, error) {
	for _, id := range slices.Concat([]string{m.Bounce.OriginalMessageID}, m.InReplyTo, m.References) {
		if id == "" {
			continue
		}
		c, err := s.st.FindEmailConfirmationByHeaderAnyWorkspace(ctx, id)
		if store.IsNotFound(err) {
			continue
		}
		if err != nil {
			return IngestResult{}, false, err
		}
		for _, r := range m.Bounce.Failed() {
			if !strings.EqualFold(r.Email, c.Email) || !r.Permanent() {
				continue
			}
			if err := s.confirmationBounced(ctx, c, strings.TrimSpace(r.Status+" "+r.Diagnostic)); err != nil {
				return IngestResult{}, false, err
			}
			break
		}
		return IngestResult{Status: IngestBounce}, true, nil
	}
	return IngestResult{}, false, nil
}

func (s *Server) applyConfirmationSES(ctx context.Context, id string, recipients []bounced) error {
	c, err := s.st.FindEmailConfirmationByHeaderAnyWorkspace(ctx, id)
	if store.IsNotFound(err) {
		s.log.InfoContext(ctx, "ses notification for an unknown message", slog.String("message_id", id))
		return nil
	}
	if err != nil {
		return err
	}
	for _, r := range recipients {
		switch {
		case !strings.EqualFold(r.email, c.Email):
		case r.reason == suppressComplaint:
			return s.confirmationComplained(ctx, c, r.detail)
		case r.permanent:
			return s.confirmationBounced(ctx, c, r.detail)
		}
	}
	s.log.InfoContext(ctx, "ses notification names no recipient of the message", slog.String("message_id", id))
	return nil
}

func (s *Server) confirmationBounced(ctx context.Context, c store.FindEmailConfirmationByHeaderAnyWorkspaceRow, detail string) error {
	return s.inTx(ctx, c.WorkspaceID, func(q *store.Queries, events *eventBatch) error {
		if err := q.SuppressConfirmations(ctx, store.SuppressConfirmationsParams{WorkspaceID: c.WorkspaceID, Email: c.Email, Reason: suppressBounce, Detail: truncateRunes(detail, 2000), Now: s.now()}); err != nil {
			return err
		}
		contactID, err := q.MarkEmailConfirmationBounced(ctx, store.MarkEmailConfirmationBouncedParams{WorkspaceID: c.WorkspaceID, ID: c.ID, Now: s.now()})
		return s.confirmationEnded(ctx, q, events, c.WorkspaceID, contactID, err)
	})
}

func (s *Server) confirmationComplained(ctx context.Context, c store.FindEmailConfirmationByHeaderAnyWorkspaceRow, detail string) error {
	return s.inTx(ctx, c.WorkspaceID, func(q *store.Queries, events *eventBatch) error {
		if err := q.SuppressConfirmations(ctx, store.SuppressConfirmationsParams{WorkspaceID: c.WorkspaceID, Email: c.Email, Reason: suppressComplaint, Detail: truncateRunes(detail, 2000), Now: s.now()}); err != nil {
			return err
		}
		contactID, err := q.MarkEmailConfirmationComplained(ctx, store.MarkEmailConfirmationComplainedParams{WorkspaceID: c.WorkspaceID, ID: c.ID, Now: s.now()})
		return s.confirmationEnded(ctx, q, events, c.WorkspaceID, contactID, err)
	})
}

func (s *Server) confirmationEnded(ctx context.Context, q *store.Queries, events *eventBatch, ws, contactID uuid.UUID, markErr error) error {
	if store.IsNotFound(markErr) {
		return nil
	}
	if markErr != nil {
		return markErr
	}
	row, err := q.GetContact(ctx, store.GetContactParams{WorkspaceID: ws, ID: contactID})
	if err != nil {
		return err
	}
	body, err := s.contactBody(ctx, q, ws, allInboxScope, row)
	if err != nil {
		return err
	}
	events.add(realtime.ContactUpdated, nil, nil, body)
	return nil
}
