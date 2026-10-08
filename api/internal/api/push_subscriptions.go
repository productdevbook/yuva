package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/url"
	"strings"
	"time"
	"uuid"

	"github.com/riverqueue/river"

	"github.com/productdevbook/yuva/api/internal/mail"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/push"
	"github.com/productdevbook/yuva/api/internal/store"
	"github.com/productdevbook/yuva/api/internal/webhook"
)

const (
	maxPushSubscriptions = 20
	pushAttempts         = 5
	pushAttemptTimeout   = 30 * time.Second
	testPushTTL          = 10 * time.Minute
)

var errPushEndpoint = errValidation("endpoint must be an https URL on a public address")

func (s *Server) checkPushEndpoint(raw string) (*url.URL, error) {
	u, err := webhook.CheckURL(raw, s.webhooks.AllowPrivate)
	if err != nil || u.Scheme != "https" {
		return nil, errPushEndpoint
	}
	return u, nil
}

func pushSubscriptionBody(r store.PushSubscription, session uuid.UUID) oas.PushSubscription {
	return oas.PushSubscription{
		Id: r.ID, Endpoint: r.Endpoint, UserAgent: r.UserAgent, Current: r.SessionID == session,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, LastSuccessAt: r.LastSuccessAt, LastFailureAt: r.LastFailureAt, LastError: r.LastError,
	}
}

func (s *Server) GetVapidPublicKey(ctx context.Context, _ oas.GetVapidPublicKeyRequestObject) (oas.GetVapidPublicKeyResponseObject, error) {
	if s.push == nil {
		return nil, errPushDisabled
	}
	return oas.GetVapidPublicKey200JSONResponse{PublicKey: s.push.Keys.PublicKey}, nil
}

func (s *Server) ListPushSubscriptions(ctx context.Context, _ oas.ListPushSubscriptionsRequestObject) (oas.ListPushSubscriptionsResponseObject, error) {
	p := principalFrom(ctx)
	rows, err := s.st.ListPushSubscriptions(ctx, p.personID)
	if err != nil {
		return nil, err
	}
	out := oas.ListPushSubscriptions200JSONResponse{Items: make([]oas.PushSubscription, len(rows))}
	for i, r := range rows {
		out.Items[i] = pushSubscriptionBody(r, p.sessionID)
	}
	return out, nil
}

func (s *Server) CreatePushSubscription(ctx context.Context, req oas.CreatePushSubscriptionRequestObject) (oas.CreatePushSubscriptionResponseObject, error) {
	if s.push == nil {
		return nil, errPushDisabled
	}
	p := principalFrom(ctx)
	b := req.Body
	if len(b.Endpoint) > 2000 {
		return nil, errPushEndpoint
	}
	u, err := s.checkPushEndpoint(b.Endpoint)
	if err != nil {
		return nil, err
	}
	if err := push.CheckSubscriptionKeys(b.Keys.P256dh, b.Keys.Auth); err != nil {
		return nil, errValidation(err.Error())
	}
	agent := ""
	if b.UserAgent != nil {
		if agent, err = trimmed(*b.UserAgent, 0, 200, "user_agent"); err != nil {
			return nil, err
		}
	}
	var out store.PushSubscription
	err = s.st.InTx(ctx, func(q *store.Queries) error {
		n, err := q.CountOtherPushSubscriptions(ctx, store.CountOtherPushSubscriptionsParams{PersonID: p.personID, Endpoint: u.String()})
		if err != nil {
			return err
		}
		if n >= maxPushSubscriptions {
			return errValidation("at most 20 push subscriptions; remove one first")
		}
		out, err = q.UpsertPushSubscription(ctx, store.UpsertPushSubscriptionParams{
			ID: newID(), PersonID: p.personID, SessionID: p.sessionID, Endpoint: u.String(),
			P256dh: b.Keys.P256dh, Auth: b.Keys.Auth, UserAgent: agent, Now: s.now(),
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	return oas.CreatePushSubscription201JSONResponse(pushSubscriptionBody(out, p.sessionID)), nil
}

func (s *Server) DeletePushSubscription(ctx context.Context, req oas.DeletePushSubscriptionRequestObject) (oas.DeletePushSubscriptionResponseObject, error) {
	n, err := s.st.DeletePushSubscription(ctx, store.DeletePushSubscriptionParams{PersonID: principalFrom(ctx).personID, ID: req.PushSubscriptionId})
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, errNotFound
	}
	return oas.DeletePushSubscription204Response{}, nil
}

func (s *Server) TestPushSubscription(ctx context.Context, req oas.TestPushSubscriptionRequestObject) (oas.TestPushSubscriptionResponseObject, error) {
	p := principalFrom(ctx)
	sub, err := s.st.GetPushSubscription(ctx, store.GetPushSubscriptionParams{PersonID: p.personID, ID: req.PushSubscriptionId})
	if store.IsNotFound(err) {
		return nil, errNotFound
	}
	if err != nil {
		return nil, err
	}
	if s.push == nil {
		return nil, errPushDisabled
	}
	person, err := s.st.GetPerson(ctx, p.personID)
	if err != nil {
		return nil, err
	}
	msg, err := mail.Render("push_test", person.Locale, nil)
	if err != nil {
		return nil, err
	}
	payload := oas.PushNotification{Event: oas.Test, Title: msg.Subject, Body: strings.TrimSpace(msg.Text), Url: "/", Tag: "test"}
	args := PushArgs{SubscriptionID: sub.ID, Payload: mustJSON(payload), TTL: int(testPushTTL / time.Second), Urgency: "normal", Topic: "test"}
	if _, err := s.jobs.Insert(ctx, args, pushOpts()); err != nil {
		return nil, err
	}
	return oas.TestPushSubscription202JSONResponse(payload), nil
}

type PushArgs struct {
	WorkspaceID    *uuid.UUID      `json:"workspace_id,omitempty"`
	SubscriptionID uuid.UUID       `json:"subscription_id"`
	Payload        json.RawMessage `json:"payload"`
	TTL            int             `json:"ttl"`
	Urgency        string          `json:"urgency"`
	Topic          string          `json:"topic,omitempty"`
}

func (PushArgs) Kind() string { return "push_send" }

func pushOpts() *river.InsertOpts { return &river.InsertOpts{MaxAttempts: pushAttempts} }

type pushWorker struct {
	river.WorkerDefaults[PushArgs]
	s *Server
}

func (w *pushWorker) Work(ctx context.Context, job *river.Job[PushArgs]) error {
	retry, err := w.s.SendPush(ctx, job.Args)
	if err != nil {
		return err
	}
	if retry {
		return errPushRetry
	}
	return nil
}

func (w *pushWorker) Timeout(*river.Job[PushArgs]) time.Duration { return pushAttemptTimeout }

// SendPush makes one attempt to deliver a push. A subscription the push service reports gone
// (404, 410), or whose endpoint is not allowed, is deleted. It reports whether to try again.
func (s *Server) SendPush(ctx context.Context, a PushArgs) (bool, error) {
	if s.push == nil {
		return false, nil
	}
	if a.WorkspaceID != nil {
		if live, err := s.workspaceLive(ctx, *a.WorkspaceID); err != nil || !live {
			return false, err
		}
	}
	sub, err := s.st.GetLivePushSubscription(ctx, store.GetLivePushSubscriptionParams{ID: a.SubscriptionID, Now: s.now()})
	if store.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	drop := func(reason error) (bool, error) {
		s.log.InfoContext(ctx, "push subscription removed", slog.String("subscription_id", sub.ID.String()), slog.Any("reason", reason))
		_, err := s.st.DropPushSubscription(ctx, store.DropPushSubscriptionParams{ID: sub.ID, Endpoint: sub.Endpoint})
		return false, err
	}
	if _, err := s.checkPushEndpoint(sub.Endpoint); err != nil {
		return drop(err)
	}
	r := s.push.Send(ctx, push.Subscription{Endpoint: sub.Endpoint, P256dh: sub.P256dh, Auth: sub.Auth}, push.Message{
		Payload: a.Payload, TTL: time.Duration(a.TTL) * time.Second, Urgency: a.Urgency, Topic: a.Topic,
	})
	if r.OK() {
		return false, s.st.PushSucceeded(ctx, store.PushSucceededParams{ID: sub.ID, Now: s.now()})
	}
	if r.Gone || errors.Is(r.Err, webhook.ErrRefusedAddress) {
		return drop(r.Err)
	}
	reason := truncateRunes(r.Err.Error(), 1000)
	if err := s.st.PushFailed(ctx, store.PushFailedParams{ID: sub.ID, Now: s.now(), Error: &reason}); err != nil {
		return false, err
	}
	return r.Retry, nil
}
