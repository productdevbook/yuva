package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
	"github.com/productdevbook/yuva/api/internal/webhook"
)

type WebhookSettings struct {
	// AllowPrivate lets endpoints point to private, loopback and link-local addresses (development).
	AllowPrivate bool
	Resolver     webhook.Resolver
}

const (
	webhookKeepAttempts      = 100
	webhookDisableAfter      = 24 * time.Hour
	webhookPreviousSecretTTL = 24 * time.Hour
	webhookAttemptTimeout    = 30 * time.Second
	maxWebhookErrorRunes     = 1000
)

var (
	errWebhookGone     = problem(http.StatusNotFound, "not_found", "no such webhook endpoint")
	errDeliveryGone    = problem(http.StatusNotFound, "not_found", "no such delivery")
	errWebhookDisabled = problem(http.StatusConflict, "webhook_disabled", "the endpoint is disabled; enable it first")
	errWebhookRetry    = errors.New("webhook attempt failed")
)

var webhookEventTypes = []oas.WebhookEventType{
	oas.WebhookEventTypeConversationCreated, oas.WebhookEventTypeConversationUpdated, oas.WebhookEventTypeMessageCreated,
	oas.WebhookEventTypeFeedbackCreated, oas.WebhookEventTypeContactUpdated, oas.WebhookEventTypeContactDeleted,
	oas.WebhookEventTypeDraftCreated, oas.WebhookEventTypeDraftUpdated, oas.WebhookEventTypeDraftDeleted,
}

func webhookSecretContext(workspaceID, endpointID uuid.UUID) []byte {
	return append(append([]byte("webhook:"), workspaceID[:]...), endpointID[:]...)
}

func (s *Server) webhookEndpointBody(e store.WebhookEndpoint) oas.WebhookEndpoint {
	out := oas.WebhookEndpoint{
		Id: e.ID, InboxId: e.InboxID, Url: e.Url, Description: e.Description, IncludeNotes: e.IncludeNotes,
		Enabled: e.Enabled, DisabledAt: e.DisabledAt, DisabledReason: e.DisabledReason, FailingSince: e.FailingSince,
		CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt, Events: make([]oas.WebhookEventType, len(e.Events)),
	}
	for i, t := range e.Events {
		out.Events[i] = oas.WebhookEventType(t)
	}
	if e.PreviousSecretUntil != nil && e.PreviousSecretUntil.After(s.now()) {
		at := e.PreviousSecretUntil.Add(-webhookPreviousSecretTTL)
		out.SecretRotatedAt = &at
	}
	return out
}

func webhookEvents(in []oas.WebhookEventType) ([]string, error) {
	if len(in) == 0 {
		return nil, errValidation("events must name at least one event type")
	}
	var out []string
	for _, t := range webhookEventTypes {
		if slices.Contains(in, t) {
			out = append(out, string(t))
		}
	}
	for _, t := range in {
		if !slices.Contains(webhookEventTypes, t) {
			return nil, errValidation("events names an unknown event type " + string(t))
		}
	}
	return out, nil
}

func (s *Server) webhookURL(raw string) (string, error) {
	if len(raw) > 2000 {
		return "", errValidation("url must be at most 2000 characters")
	}
	u, err := webhook.CheckURL(raw, s.webhooks.AllowPrivate)
	if errors.Is(err, webhook.ErrRefusedAddress) {
		return "", errValidation("url points to a private, loopback or link-local address")
	}
	if err != nil {
		return "", errValidation(err.Error())
	}
	return u.String(), nil
}

func (s *Server) visibleWebhook(ctx context.Context, q *store.Queries, p principal, id uuid.UUID, lock bool) (store.WebhookEndpoint, error) {
	var (
		e   store.WebhookEndpoint
		err error
	)
	if lock {
		e, err = q.LockWebhookEndpoint(ctx, store.LockWebhookEndpointParams{WorkspaceID: p.workspaceID, ID: id})
	} else {
		e, err = q.GetWebhookEndpoint(ctx, store.GetWebhookEndpointParams{WorkspaceID: p.workspaceID, ID: id})
	}
	if store.IsNotFound(err) {
		return e, errWebhookGone
	}
	return e, err
}

func (s *Server) ListWebhooks(ctx context.Context, req oas.ListWebhooksRequestObject) (oas.ListWebhooksResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	if req.Params.InboxId != nil {
		if _, err := visibleInbox(ctx, s.st.Queries, p, *req.Params.InboxId); err != nil {
			return nil, err
		}
	}
	rows, err := s.st.ListWebhookEndpoints(ctx, store.ListWebhookEndpointsParams{WorkspaceID: p.workspaceID, InboxID: req.Params.InboxId})
	if err != nil {
		return nil, err
	}
	out := oas.ListWebhooks200JSONResponse{Items: make([]oas.WebhookEndpoint, len(rows))}
	for i, r := range rows {
		out.Items[i] = s.webhookEndpointBody(r)
	}
	return out, nil
}

func (s *Server) CreateWebhook(ctx context.Context, req oas.CreateWebhookRequestObject) (oas.CreateWebhookResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	b := req.Body
	url, err := s.webhookURL(b.Url)
	if err != nil {
		return nil, err
	}
	events, err := webhookEvents(b.Events)
	if err != nil {
		return nil, err
	}
	desc := ""
	if b.Description != nil {
		if desc, err = trimmed(*b.Description, 0, 200, "description"); err != nil {
			return nil, err
		}
	}
	if b.InboxId != nil {
		if _, err := visibleInbox(ctx, s.st.Queries, p, *b.InboxId); err != nil {
			return nil, err
		}
	}
	id := newID()
	secret, key := webhook.NewSecret()
	e, err := s.st.CreateWebhookEndpoint(ctx, store.CreateWebhookEndpointParams{
		ID: id, WorkspaceID: p.workspaceID, InboxID: b.InboxId, Url: url, Description: desc, Events: events,
		IncludeNotes: b.IncludeNotes != nil && *b.IncludeNotes, Enabled: b.Enabled == nil || *b.Enabled,
		Secret: s.secrets.Seal(key, webhookSecretContext(p.workspaceID, id)), Now: s.now(),
	})
	if store.IsForeignKeyViolation(err) {
		return nil, errInboxGone
	}
	if err != nil {
		return nil, err
	}
	return oas.CreateWebhook201JSONResponse{Endpoint: s.webhookEndpointBody(e), Secret: secret}, nil
}

func (s *Server) GetWebhook(ctx context.Context, req oas.GetWebhookRequestObject) (oas.GetWebhookResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	e, err := s.visibleWebhook(ctx, s.st.Queries, p, req.WebhookId, false)
	if err != nil {
		return nil, err
	}
	return oas.GetWebhook200JSONResponse(s.webhookEndpointBody(e)), nil
}

func (s *Server) UpdateWebhook(ctx context.Context, req oas.UpdateWebhookRequestObject) (oas.UpdateWebhookResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	b := req.Body
	var out oas.WebhookEndpoint
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		cur, err := s.visibleWebhook(ctx, q, p, req.WebhookId, true)
		if err != nil {
			return err
		}
		arg := store.UpdateWebhookEndpointParams{
			WorkspaceID: p.workspaceID, ID: cur.ID, Url: cur.Url, Description: cur.Description, Events: cur.Events,
			IncludeNotes: cur.IncludeNotes, Enabled: cur.Enabled, Now: s.now(),
		}
		if b.Url != nil {
			if arg.Url, err = s.webhookURL(*b.Url); err != nil {
				return err
			}
		}
		if b.Description != nil {
			if arg.Description, err = trimmed(*b.Description, 0, 200, "description"); err != nil {
				return err
			}
		}
		if b.Events != nil {
			if arg.Events, err = webhookEvents(*b.Events); err != nil {
				return err
			}
		}
		if b.IncludeNotes != nil {
			arg.IncludeNotes = *b.IncludeNotes
		}
		if b.Enabled != nil {
			arg.Enabled = *b.Enabled
		}
		e, err := q.UpdateWebhookEndpoint(ctx, arg)
		out = s.webhookEndpointBody(e)
		return err
	})
	if err != nil {
		return nil, err
	}
	return oas.UpdateWebhook200JSONResponse(out), nil
}

func (s *Server) DeleteWebhook(ctx context.Context, req oas.DeleteWebhookRequestObject) (oas.DeleteWebhookResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	n, err := s.st.DeleteWebhookEndpoint(ctx, store.DeleteWebhookEndpointParams{WorkspaceID: p.workspaceID, ID: req.WebhookId})
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, errWebhookGone
	}
	return oas.DeleteWebhook204Response{}, nil
}

func (s *Server) RotateWebhookSecret(ctx context.Context, req oas.RotateWebhookSecretRequestObject) (oas.RotateWebhookSecretResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	if _, err := s.visibleWebhook(ctx, s.st.Queries, p, req.WebhookId, false); err != nil {
		return nil, err
	}
	secret, key := webhook.NewSecret()
	now := s.now()
	e, err := s.st.RotateWebhookSecret(ctx, store.RotateWebhookSecretParams{
		WorkspaceID: p.workspaceID, ID: req.WebhookId, Secret: s.secrets.Seal(key, webhookSecretContext(p.workspaceID, req.WebhookId)),
		PreviousUntil: new(now.Add(webhookPreviousSecretTTL)), Now: now,
	})
	if store.IsNotFound(err) {
		return nil, errWebhookGone
	}
	if err != nil {
		return nil, err
	}
	return oas.RotateWebhookSecret200JSONResponse{Endpoint: s.webhookEndpointBody(e), Secret: secret}, nil
}

func attemptBody(a store.WebhookAttempt) oas.WebhookAttempt {
	out := oas.WebhookAttempt{
		Id: a.ID, DeliveryId: a.DeliveryID, AttemptedAt: a.AttemptedAt, Manual: a.Manual, Success: a.Success,
		StatusCode: a.StatusCode, LatencyMs: a.LatencyMs, ResponseBody: a.ResponseBody, Error: a.Error,
	}
	return out
}

func deliveryBody(d store.WebhookDelivery, last *store.WebhookAttempt) oas.WebhookDelivery {
	out := oas.WebhookDelivery{
		Id: d.ID, MessageId: d.MessageID, EventType: oas.WebhookEventType(d.EventType), State: oas.WebhookDeliveryState(d.State),
		Attempts: d.Attempts, CreatedAt: d.CreatedAt,
	}
	if d.State == string(oas.WebhookDeliveryStatePending) {
		out.NextAttemptAt = d.NextAttemptAt
	}
	if last != nil {
		a := attemptBody(*last)
		out.LastAttempt = &a
	}
	return out
}

func (s *Server) endpointDelivery(ctx context.Context, p principal, webhookID, deliveryID uuid.UUID) (store.WebhookEndpoint, store.WebhookDelivery, error) {
	var d store.WebhookDelivery
	e, err := s.visibleWebhook(ctx, s.st.Queries, p, webhookID, false)
	if err != nil {
		return e, d, err
	}
	d, err = s.st.GetEndpointDelivery(ctx, store.GetEndpointDeliveryParams{WorkspaceID: p.workspaceID, EndpointID: e.ID, ID: deliveryID})
	if store.IsNotFound(err) {
		return e, d, errDeliveryGone
	}
	return e, d, err
}

func (s *Server) ListWebhookDeliveries(ctx context.Context, req oas.ListWebhookDeliveriesRequestObject) (oas.ListWebhookDeliveriesResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	lim, err := pageSize(req.Params.Limit)
	if err != nil {
		return nil, err
	}
	at, id, err := decodeCursor(req.Params.Cursor)
	if err != nil {
		return nil, err
	}
	e, err := s.visibleWebhook(ctx, s.st.Queries, p, req.WebhookId, false)
	if err != nil {
		return nil, err
	}
	var state *string
	if req.Params.State != nil {
		if !req.Params.State.Valid() {
			return nil, errValidation("state must be pending, succeeded or failed")
		}
		v := string(*req.Params.State)
		state = &v
	}
	rows, err := s.st.ListWebhookDeliveries(ctx, store.ListWebhookDeliveriesParams{
		WorkspaceID: p.workspaceID, EndpointID: e.ID, State: state, CursorAt: at, CursorID: id, Lim: lim + 1,
	})
	if err != nil {
		return nil, err
	}
	var next *string
	if len(rows) > int(lim) {
		rows = rows[:lim]
		c := encodeCursor(rows[lim-1].CreatedAt, rows[lim-1].ID)
		next = &c
	}
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	attempts, err := s.st.ListDeliveryAttempts(ctx, store.ListDeliveryAttemptsParams{WorkspaceID: p.workspaceID, DeliveryIds: ids})
	if err != nil {
		return nil, err
	}
	last := map[uuid.UUID]*store.WebhookAttempt{}
	for i := range attempts {
		if _, ok := last[attempts[i].DeliveryID]; !ok {
			last[attempts[i].DeliveryID] = &attempts[i]
		}
	}
	out := oas.ListWebhookDeliveries200JSONResponse{Items: make([]oas.WebhookDelivery, len(rows)), NextCursor: next}
	for i, r := range rows {
		out.Items[i] = deliveryBody(r, last[r.ID])
	}
	return out, nil
}

func (s *Server) GetWebhookDelivery(ctx context.Context, req oas.GetWebhookDeliveryRequestObject) (oas.GetWebhookDeliveryResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	_, d, err := s.endpointDelivery(ctx, p, req.WebhookId, req.DeliveryId)
	if err != nil {
		return nil, err
	}
	attempts, err := s.st.ListDeliveryAttempts(ctx, store.ListDeliveryAttemptsParams{WorkspaceID: p.workspaceID, DeliveryIds: []uuid.UUID{d.ID}})
	if err != nil {
		return nil, err
	}
	var last *store.WebhookAttempt
	if len(attempts) > 0 {
		last = &attempts[0]
	}
	b := deliveryBody(d, last)
	out := oas.GetWebhookDelivery200JSONResponse{
		Id: b.Id, MessageId: b.MessageId, EventType: b.EventType, State: b.State, Attempts: b.Attempts,
		NextAttemptAt: b.NextAttemptAt, LastAttempt: b.LastAttempt, CreatedAt: b.CreatedAt,
		Payload: map[string]any{}, AttemptLog: make([]oas.WebhookAttempt, len(attempts)),
	}
	_ = json.Unmarshal([]byte(d.Payload), &out.Payload)
	for i, a := range attempts {
		out.AttemptLog[i] = attemptBody(a)
	}
	return out, nil
}

func (s *Server) RedeliverWebhook(ctx context.Context, req oas.RedeliverWebhookRequestObject) (oas.RedeliverWebhookResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	e, d, err := s.endpointDelivery(ctx, p, req.WebhookId, req.DeliveryId)
	if err != nil {
		return nil, err
	}
	if !e.Enabled {
		return nil, errWebhookDisabled
	}
	if _, err := s.jobs.Insert(ctx, WebhookDeliveryArgs{WorkspaceID: p.workspaceID, DeliveryID: d.ID, Manual: true}, &river.InsertOpts{MaxAttempts: 1}); err != nil {
		return nil, err
	}
	return oas.RedeliverWebhook202JSONResponse(deliveryBody(d, nil)), nil
}

func (s *Server) ListWebhookAttempts(ctx context.Context, req oas.ListWebhookAttemptsRequestObject) (oas.ListWebhookAttemptsResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	lim, err := pageSize(req.Params.Limit)
	if err != nil {
		return nil, err
	}
	e, err := s.visibleWebhook(ctx, s.st.Queries, p, req.WebhookId, false)
	if err != nil {
		return nil, err
	}
	rows, err := s.st.ListWebhookAttempts(ctx, store.ListWebhookAttemptsParams{WorkspaceID: p.workspaceID, EndpointID: e.ID, Lim: lim})
	if err != nil {
		return nil, err
	}
	out := oas.ListWebhookAttempts200JSONResponse{Items: make([]oas.WebhookAttempt, len(rows))}
	for i, r := range rows {
		out.Items[i] = attemptBody(r)
	}
	return out, nil
}

// Events that may become webhooks. A contact.deleted event becomes one only when the deletion
// recorded the contact's external ids (a merged anonymous visitor does not).
func webhookSource(e pendingEvent) bool {
	switch e.typ {
	case realtime.ConversationCreated, realtime.ConversationUpdated, realtime.MessageCreated, realtime.ContactUpdated,
		realtime.DraftCreated, realtime.DraftUpdated, realtime.DraftDeleted:
		return true
	case realtime.ContactDeleted:
		return e.deleted != nil
	}
	return false
}

// queueWebhooks runs in the transaction that wrote the events, so a fan-out job exists exactly
// when the events committed.
func (s *Server) queueWebhooks(ctx context.Context, tx pgx.Tx, q *store.Queries, workspaceID uuid.UUID, items []pendingEvent, ids []int64) error {
	var jobs []river.InsertManyParams
	for i, e := range items {
		if webhookSource(e) {
			jobs = append(jobs, river.InsertManyParams{Args: WebhookFanoutArgs{WorkspaceID: workspaceID, EventID: ids[i], Deleted: e.deleted}})
		}
	}
	if len(jobs) == 0 {
		return nil
	}
	found, err := q.WorkspaceHasWebhooks(ctx, workspaceID)
	if err != nil || !found {
		return err
	}
	_, err = s.jobs.InsertManyTx(ctx, tx, jobs)
	return err
}

type deletedContact struct {
	Contact oas.WebhookDeletedContact `json:"contact"`
	Inboxes []uuid.UUID               `json:"inboxes"`
}

type WebhookFanoutArgs struct {
	WorkspaceID uuid.UUID       `json:"workspace_id"`
	EventID     int64           `json:"event_id"`
	Deleted     *deletedContact `json:"deleted,omitempty"`
}

func (WebhookFanoutArgs) Kind() string { return "webhook_fanout" }

type webhookFanoutWorker struct {
	river.WorkerDefaults[WebhookFanoutArgs]
	s *Server
}

func (w *webhookFanoutWorker) Work(ctx context.Context, job *river.Job[WebhookFanoutArgs]) error {
	return w.s.FanOutWebhooks(ctx, job.Args)
}

type WebhookDeliveryArgs struct {
	WorkspaceID uuid.UUID `json:"workspace_id"`
	DeliveryID  uuid.UUID `json:"delivery_id"`
	Manual      bool      `json:"manual,omitempty"`
}

func (WebhookDeliveryArgs) Kind() string { return "webhook_delivery" }

type webhookDeliveryWorker struct {
	river.WorkerDefaults[WebhookDeliveryArgs]
	s *Server
}

func (w *webhookDeliveryWorker) Work(ctx context.Context, job *river.Job[WebhookDeliveryArgs]) error {
	retry, err := w.s.DeliverWebhook(ctx, job.Args.WorkspaceID, job.Args.DeliveryID, job.Attempt, job.MaxAttempts, job.Args.Manual)
	if err != nil {
		return err
	}
	if retry {
		return errWebhookRetry
	}
	return nil
}

func (w *webhookDeliveryWorker) NextRetry(job *river.Job[WebhookDeliveryArgs]) time.Time {
	d, ok := webhook.NextDelay(job.Attempt, job.Args.DeliveryID[:])
	if !ok {
		return time.Time{}
	}
	return time.Now().Add(d)
}

func (w *webhookDeliveryWorker) Timeout(*river.Job[WebhookDeliveryArgs]) time.Duration {
	return webhookAttemptTimeout
}

func webhookDeliveryOpts() *river.InsertOpts {
	return &river.InsertOpts{MaxAttempts: len(webhook.RetryDelays) + 1}
}

type webhookPayload struct {
	Type        string     `json:"type"`
	Timestamp   time.Time  `json:"timestamp"`
	WorkspaceID uuid.UUID  `json:"workspace_id"`
	InboxID     *uuid.UUID `json:"inbox_id,omitempty"`
	Data        any        `json:"data"`
}

type webhookOut struct {
	typ  string
	note bool
	body []byte
}

// contactPresence tells whether the contact has a live client connection and when they were last
// seen on the client API.
func (s *Server) contactPresence(ctx context.Context, q *store.Queries, workspaceID, contactID uuid.UUID) (bool, *time.Time, error) {
	online, err := q.ContactConnected(ctx, store.ContactConnectedParams{WorkspaceID: workspaceID, ContactID: &contactID, FreshAfter: s.now().Add(-presenceFresh)})
	if err != nil {
		return false, nil, err
	}
	last, err := q.ContactLastActivity(ctx, store.ContactLastActivityParams{WorkspaceID: workspaceID, ContactID: contactID})
	if err != nil {
		return false, nil, err
	}
	if last.Unix() <= 0 {
		return online, nil, nil
	}
	return online, &last, nil
}

func (s *Server) webhookContactOf(ctx context.Context, q *store.Queries, workspaceID uuid.UUID, c oas.Contact) (oas.WebhookContact, error) {
	out := oas.WebhookContact{
		Id: c.Id, Name: c.Name, Emails: c.Emails, ExternalIds: c.ExternalIds, Attributes: c.Attributes, Locale: c.Locale,
	}
	var err error
	out.Online, out.LastSeenAt, err = s.contactPresence(ctx, q, workspaceID, c.Id)
	return out, err
}

func (s *Server) webhookContact(ctx context.Context, q *store.Queries, workspaceID, id uuid.UUID) (*oas.WebhookContact, error) {
	r, err := q.GetContact(ctx, store.GetContactParams{WorkspaceID: workspaceID, ID: id})
	if store.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c, err := s.contactBody(ctx, q, workspaceID, r)
	if err != nil {
		return nil, err
	}
	out, err := s.webhookContactOf(ctx, q, workspaceID, c)
	return &out, err
}

// webhookPayloads turns a stored event into the webhooks it causes and the inboxes they concern.
func (s *Server) webhookPayloads(ctx context.Context, q *store.Queries, ev store.Event, deleted *deletedContact) ([]webhookOut, []uuid.UUID, error) {
	ws := ev.WorkspaceID
	payload := func(typ string, inbox *uuid.UUID, data any) []byte {
		return mustJSON(webhookPayload{Type: typ, Timestamp: ev.CreatedAt.UTC(), WorkspaceID: ws, InboxID: inbox, Data: data})
	}
	switch ev.Type {
	case realtime.ConversationCreated, realtime.ConversationUpdated:
		var conv oas.Conversation
		if err := json.Unmarshal(ev.Payload, &conv); err != nil {
			return nil, nil, err
		}
		contact, err := s.webhookContact(ctx, q, ws, conv.ContactId)
		if err != nil || contact == nil {
			return nil, nil, err
		}
		inbox := conv.InboxId
		out := []webhookOut{{typ: ev.Type, body: payload(ev.Type, &inbox, oas.WebhookConversationData{Conversation: conv, Contact: *contact})}}
		if ev.Type == realtime.ConversationCreated && conv.Kind == oas.ConversationKindFeedback {
			id, err := q.GetFirstPublicMessage(ctx, store.GetFirstPublicMessageParams{WorkspaceID: ws, ConversationID: conv.Id})
			if store.IsNotFound(err) {
				return out, []uuid.UUID{inbox}, nil
			}
			if err != nil {
				return nil, nil, err
			}
			m, err := q.GetMessage(ctx, store.GetMessageParams{WorkspaceID: ws, ID: id})
			if err != nil {
				return nil, nil, err
			}
			atts, err := messageAttachments(ctx, q, ws, []uuid.UUID{id})
			if err != nil {
				return nil, nil, err
			}
			typ := string(oas.WebhookEventTypeFeedbackCreated)
			out = append(out, webhookOut{typ: typ, body: payload(typ, &inbox, oas.WebhookMessageData{
				Message: messageBody(messageRow(m), atts[id]), Conversation: conv, Contact: *contact,
			})})
		}
		return out, []uuid.UUID{inbox}, nil
	case realtime.MessageCreated, realtime.DraftCreated, realtime.DraftUpdated, realtime.DraftDeleted:
		var m oas.Message
		if err := json.Unmarshal(ev.Payload, &m); err != nil {
			return nil, nil, err
		}
		if m.Kind == oas.MessageKindEvent {
			return nil, nil, nil
		}
		c, err := q.GetConversation(ctx, store.GetConversationParams{WorkspaceID: ws, ID: m.ConversationId})
		if store.IsNotFound(err) {
			return nil, nil, nil
		}
		if err != nil {
			return nil, nil, err
		}
		conv, err := oneConversation(ctx, q, c)
		if err != nil {
			return nil, nil, err
		}
		contact, err := s.webhookContact(ctx, q, ws, c.ContactID)
		if err != nil || contact == nil {
			return nil, nil, err
		}
		inbox := c.InboxID
		return []webhookOut{{typ: ev.Type, note: m.Kind == oas.MessageKindNote, body: payload(ev.Type, &inbox, oas.WebhookMessageData{
			Message: m, Conversation: conv, Contact: *contact,
		})}}, []uuid.UUID{inbox}, nil
	case realtime.ContactUpdated:
		var c oas.Contact
		if err := json.Unmarshal(ev.Payload, &c); err != nil {
			return nil, nil, err
		}
		wc, err := s.webhookContactOf(ctx, q, ws, c)
		if err != nil {
			return nil, nil, err
		}
		inboxes, err := q.ContactInboxIDs(ctx, store.ContactInboxIDsParams{WorkspaceID: ws, ContactID: c.Id})
		if err != nil {
			return nil, nil, err
		}
		return []webhookOut{{typ: ev.Type, body: payload(ev.Type, nil, oas.WebhookContactData{Contact: wc})}}, inboxes, nil
	case realtime.ContactDeleted:
		if deleted == nil {
			return nil, nil, nil
		}
		return []webhookOut{{typ: ev.Type, body: payload(ev.Type, nil, oas.WebhookContactDeletedData{Contact: deleted.Contact})}}, deleted.Inboxes, nil
	}
	return nil, nil, nil
}

// FanOutWebhooks creates one delivery per enabled endpoint that subscribed to the event's
// webhooks and covers its inbox, and queues the first attempt of each.
func (s *Server) FanOutWebhooks(ctx context.Context, a WebhookFanoutArgs) error {
	ev, err := s.st.GetEvent(ctx, store.GetEventParams{WorkspaceID: a.WorkspaceID, ID: a.EventID})
	if store.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	endpoints, err := s.st.ListEnabledWebhookEndpoints(ctx, a.WorkspaceID)
	if err != nil || len(endpoints) == 0 {
		return err
	}
	hooks, inboxes, err := s.webhookPayloads(ctx, s.st.Queries, ev, a.Deleted)
	if err != nil || len(hooks) == 0 {
		return err
	}
	now := s.now()
	return s.st.InTxRaw(ctx, func(tx pgx.Tx, q *store.Queries) error {
		var jobs []river.InsertManyParams
		for _, e := range endpoints {
			if e.InboxID != nil && !slices.Contains(inboxes, *e.InboxID) {
				continue
			}
			for _, h := range hooks {
				if !slices.Contains(e.Events, h.typ) || (h.note && !e.IncludeNotes) {
					continue
				}
				id := newID()
				d, err := q.CreateWebhookDelivery(ctx, store.CreateWebhookDeliveryParams{
					ID: id, WorkspaceID: a.WorkspaceID, EndpointID: e.ID, MessageID: "msg_" + strings.ReplaceAll(id.String(), "-", ""),
					EventType: h.typ, Payload: string(h.body), Now: now,
				})
				if err != nil {
					return err
				}
				jobs = append(jobs, river.InsertManyParams{
					Args: WebhookDeliveryArgs{WorkspaceID: a.WorkspaceID, DeliveryID: d.ID}, InsertOpts: webhookDeliveryOpts(),
				})
			}
		}
		if len(jobs) == 0 {
			return nil
		}
		_, err := s.jobs.InsertManyTx(ctx, tx, jobs)
		return err
	})
}

func (s *Server) webhookKeys(e store.WebhookEndpoint) ([][]byte, error) {
	key, err := s.secrets.Open(e.Secret, webhookSecretContext(e.WorkspaceID, e.ID))
	if err != nil {
		return nil, err
	}
	keys := [][]byte{key}
	if e.PreviousSecret != nil && e.PreviousSecretUntil != nil && e.PreviousSecretUntil.After(s.now()) {
		prev, err := s.secrets.Open(e.PreviousSecret, webhookSecretContext(e.WorkspaceID, e.ID))
		if err != nil {
			return nil, err
		}
		keys = append(keys, prev)
	}
	return keys, nil
}

// DeliverWebhook makes one attempt of a delivery, records it in the delivery log and disables the
// endpoint after sustained failure or a 410. It reports whether the job should try again.
func (s *Server) DeliverWebhook(ctx context.Context, workspaceID, deliveryID uuid.UUID, attempt, maxAttempts int, manual bool) (bool, error) {
	d, err := s.st.GetWebhookDelivery(ctx, store.GetWebhookDeliveryParams{WorkspaceID: workspaceID, ID: deliveryID})
	if store.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	e, err := s.st.GetWebhookEndpoint(ctx, store.GetWebhookEndpointParams{WorkspaceID: workspaceID, ID: d.EndpointID})
	if store.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !e.Enabled {
		return false, s.st.FailPendingWebhookDelivery(ctx, store.FailPendingWebhookDeliveryParams{WorkspaceID: workspaceID, ID: d.ID, Now: s.now()})
	}
	if !manual && d.State != string(oas.WebhookDeliveryStatePending) {
		return false, nil
	}
	keys, err := s.webhookKeys(e)
	if err != nil {
		return false, err
	}
	now := s.now()
	res := s.hooks.Send(ctx, webhook.Request{
		URL: e.Url, MessageID: d.MessageID, Timestamp: now, Body: []byte(d.Payload), Keys: keys, UserAgent: "Yuva-Webhooks/" + s.version,
	})
	ok := res.OK()
	var errText *string
	if !ok {
		v := truncateRunes(res.Err.Error(), maxWebhookErrorRunes)
		errText = &v
	}
	retry := false
	err = s.st.InTx(ctx, func(q *store.Queries) error {
		var status *int32
		if res.StatusCode != 0 {
			v := int32(res.StatusCode)
			status = &v
		}
		if err := q.RecordWebhookAttempt(ctx, store.RecordWebhookAttemptParams{
			ID: newID(), WorkspaceID: workspaceID, EndpointID: e.ID, DeliveryID: d.ID, AttemptedAt: now, Manual: manual,
			Success: ok, StatusCode: status, LatencyMs: int32(res.Latency.Milliseconds()), ResponseBody: res.Body, Error: errText,
		}); err != nil {
			return err
		}
		disabled := false
		if ok {
			if err := q.WebhookSucceeded(ctx, store.WebhookSucceededParams{WorkspaceID: workspaceID, ID: e.ID}); err != nil {
				return err
			}
		} else {
			f, err := q.WebhookFailed(ctx, store.WebhookFailedParams{WorkspaceID: workspaceID, ID: e.ID, Now: now})
			if err != nil {
				return err
			}
			reason := ""
			if res.StatusCode == http.StatusGone {
				reason = "The endpoint answered 410 Gone."
			} else if !f.FailingSince.After(now.Add(-webhookDisableAfter)) {
				reason = fmt.Sprintf("Every attempt since %s failed; the last one: %s", f.FailingSince.UTC().Format(time.RFC3339), *errText)
			}
			if reason != "" {
				if _, err := q.DisableWebhookEndpoint(ctx, store.DisableWebhookEndpointParams{
					WorkspaceID: workspaceID, ID: e.ID, Now: now, Reason: new(truncateRunes(reason, maxWebhookErrorRunes)),
				}); err != nil {
					return err
				}
				disabled = true
			}
		}
		arg := store.FinishWebhookAttemptParams{WorkspaceID: workspaceID, ID: d.ID, Now: now, State: string(oas.WebhookDeliveryStateFailed)}
		switch {
		case ok:
			arg.State = string(oas.WebhookDeliveryStateSucceeded)
		case disabled:
		case manual:
			if d.State == string(oas.WebhookDeliveryStatePending) {
				arg.State, arg.NextAttemptAt = d.State, d.NextAttemptAt
			}
		case attempt < maxAttempts:
			if delay, more := webhook.NextDelay(attempt, d.ID[:]); more {
				arg.State, arg.NextAttemptAt, retry = string(oas.WebhookDeliveryStatePending), new(now.Add(delay)), true
			}
		}
		if _, err := q.FinishWebhookAttempt(ctx, arg); err != nil {
			return err
		}
		return q.PruneWebhookAttempts(ctx, store.PruneWebhookAttemptsParams{WorkspaceID: workspaceID, EndpointID: e.ID, Offset: webhookKeepAttempts})
	})
	return retry, err
}

func (s *Server) DeleteContactByExternalId(ctx context.Context, req oas.DeleteContactByExternalIdRequestObject) (oas.DeleteContactByExternalIdResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrFullKey(p); err != nil {
		return nil, err
	}
	if _, err := visibleInbox(ctx, s.st.Queries, p, req.Params.InboxId); err != nil {
		return nil, err
	}
	id, err := s.st.GetContactIDByExternalID(ctx, store.GetContactIDByExternalIDParams{
		WorkspaceID: p.workspaceID, InboxID: req.Params.InboxId, ExternalID: req.Params.ExternalId,
	})
	if store.IsNotFound(err) {
		return nil, errContactGone
	}
	if err != nil {
		return nil, err
	}
	if err := s.deleteContact(ctx, p, id); err != nil {
		return nil, err
	}
	return oas.DeleteContactByExternalId204Response{}, nil
}

func (s *Server) GetContactPresence(ctx context.Context, req oas.GetContactPresenceRequestObject) (oas.GetContactPresenceResponseObject, error) {
	p := principalFrom(ctx)
	found, err := s.st.ContactExists(ctx, store.ContactExistsParams{WorkspaceID: p.workspaceID, ID: req.ContactId})
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errContactGone
	}
	if err := visibleContact(ctx, s.st.Queries, p, req.ContactId); err != nil {
		return nil, err
	}
	out := oas.GetContactPresence200JSONResponse{ContactId: req.ContactId}
	out.Online, out.LastSeenAt, err = s.contactPresence(ctx, s.st.Queries, p.workspaceID, req.ContactId)
	if err != nil {
		return nil, err
	}
	return out, nil
}
