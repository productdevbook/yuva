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
			body, err := s.contactBody(ctx, q, ws, row)
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

// applyDSN handles a delivery report only when it names a message we sent from this workspace and
// reports a recipient of that message; anything else is not ours to act on and handled is false.
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
		return IngestResult{}, false, nil
	}
	failed := m.Bounce.Failed()
	if len(failed) == 0 {
		return IngestResult{Status: IngestDropped}, true, nil
	}
	recipients := make([]bounced, 0, len(failed))
	for _, r := range failed {
		if !slices.Contains(sent.ToAddresses, r.Email) {
			continue
		}
		detail := strings.TrimSpace(r.Status + " " + r.Diagnostic)
		recipients = append(recipients, bounced{email: r.Email, reason: suppressBounce, detail: detail, permanent: r.Permanent()})
	}
	if len(recipients) == 0 {
		return IngestResult{}, false, nil
	}
	if err := s.recordBounce(ctx, ch.WorkspaceID, &sent.MessageID, recipients, true); err != nil {
		return IngestResult{}, false, err
	}
	return IngestResult{Status: IngestBounce, ConversationID: &sent.ConversationID, MessageID: &sent.MessageID}, true, nil
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
)

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
	row, err := s.st.FindOutboundEmailAnyWorkspace(ctx, id)
	if store.IsNotFound(err) {
		s.log.InfoContext(ctx, "ses notification for an unknown message", slog.String("message_id", id))
		return nil
	}
	if err != nil {
		return err
	}
	recipients = slices.DeleteFunc(recipients, func(r bounced) bool { return !slices.Contains(row.ToAddresses, r.email) })
	if len(recipients) == 0 {
		s.log.InfoContext(ctx, "ses notification names no recipient of the message", slog.String("message_id", id))
		return nil
	}
	return s.recordBounce(ctx, row.WorkspaceID, &row.MessageID, recipients, failMessage)
}
