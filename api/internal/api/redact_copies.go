package api

import (
	"context"
	"encoding/json"
	"strings"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/store"
)

// redaction is what stored JSON copies (events, webhook deliveries, replies kept for
// Idempotency-Key) must lose when a message is redacted: the message's content, and when they
// came from it, the conversation's subject and a feedback's copied text.
type redaction struct {
	messageID string
	// message is the redacted message as /v1 shows it.
	message map[string]any
	convID  string
	// subject is the conversation's former subject when the redaction cleared it.
	subject  string
	feedback bool
}

func jsonObject(v any) map[string]any {
	var out map[string]any
	_ = json.Unmarshal(mustJSON(v), &out)
	return out
}

func (r redaction) needles() []string {
	out := []string{r.messageID}
	if r.convID != "" && (r.subject != "" || r.feedback) {
		out = append(out, r.convID)
	}
	return out
}

// apply rewrites a JSON document in place wherever it holds the message (a /v1 message, a
// /client/v1 message or a preview), the conversation, or an e-mail summary with the old subject.
func (r redaction) apply(raw []byte) ([]byte, bool) {
	var doc any
	if json.Unmarshal(raw, &doc) != nil {
		return nil, false
	}
	doc, changed := r.walk(doc)
	if !changed {
		return nil, false
	}
	return mustJSON(doc), true
}

func (r redaction) walk(v any) (any, bool) {
	switch x := v.(type) {
	case []any:
		changed := false
		for i := range x {
			var c bool
			if x[i], c = r.walk(x[i]); c {
				changed = true
			}
		}
		return x, changed
	case map[string]any:
		_, hasKind := x["kind"]
		_, hasBody := x["body"]
		_, hasText := x["text"]
		changed := false
		if x["id"] == r.messageID {
			switch {
			case hasKind && hasBody:
				out := map[string]any{}
				for k, v := range r.message {
					out[k] = v
				}
				if d, ok := x["draft"]; ok {
					out["draft"] = d
				}
				return out, true
			case hasBody:
				delete(x, "html")
				delete(x, "cards")
				x["body"], x["attachments"], x["redacted"] = "", []any{}, true
				return x, true
			case hasText:
				x["text"], x["redacted"] = "", true
				changed = true
			}
		}
		if r.convID != "" && x["id"] == r.convID {
			if s, ok := x["subject"].(string); ok && r.subject != "" && s != "" {
				x["subject"], changed = "", true
			}
			if fb, ok := x["feedback"].(map[string]any); ok && r.feedback && (fb["text"] != "" || fb["attachment_count"] != float64(0)) {
				fb["text"], fb["attachment_count"], changed = "", 0, true
			}
		}
		if s, ok := x["subject"].(string); ok && r.subject != "" && x["id"] != r.convID && strings.Contains(strings.ToLower(s), strings.ToLower(r.subject)) {
			delete(x, "subject")
			changed = true
		}
		for k, val := range x {
			if nv, c := r.walk(val); c {
				x[k], changed = nv, true
			}
		}
		return x, changed
	}
	return v, false
}

// rewriteCopies rewrites the conversation's stored events, then the webhook deliveries and kept
// replies that hold what goes. The events are locked first: webhook fan-out reads its event under
// a shared lock, so deliveries written from an event read before this redaction are committed
// before the deliveries are read here.
func (r redaction) rewriteCopies(ctx context.Context, q *store.Queries, workspaceID, convID uuid.UUID) error {
	evs, err := q.LockConversationEvents(ctx, store.LockConversationEventsParams{WorkspaceID: workspaceID, ConversationID: &convID})
	if err != nil {
		return err
	}
	for _, e := range evs {
		if b, ok := r.apply(e.Payload); ok {
			if err := q.SetEventPayload(ctx, store.SetEventPayloadParams{WorkspaceID: workspaceID, ID: e.ID, Payload: b}); err != nil {
				return err
			}
		}
	}
	ds, err := q.LockDeliveriesMentioning(ctx, store.LockDeliveriesMentioningParams{WorkspaceID: workspaceID, Needles: r.needles()})
	if err != nil {
		return err
	}
	for _, d := range ds {
		if b, ok := r.apply([]byte(d.Payload)); ok {
			if err := q.SetWebhookDeliveryPayload(ctx, store.SetWebhookDeliveryPayloadParams{WorkspaceID: workspaceID, ID: d.ID, Payload: string(b)}); err != nil {
				return err
			}
		}
	}
	replies, err := q.ListIdempotentRepliesMentioning(ctx, store.ListIdempotentRepliesMentioningParams{WorkspaceID: workspaceID, Needles: r.needles()})
	if err != nil {
		return err
	}
	for _, rp := range replies {
		if b, ok := r.apply(rp.Body); ok {
			if err := q.SetIdempotentReplyBody(ctx, store.SetIdempotentReplyBodyParams{
				WorkspaceID: workspaceID, CallerType: rp.CallerType, CallerID: rp.CallerID, Key: rp.Key, Body: b,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

// redactedMessages returns the redactions for the messages among ids that are redacted, after
// waiting for any redaction of them in progress.
func (s *Server) redactedMessages(ctx context.Context, q *store.Queries, workspaceID uuid.UUID, ids []uuid.UUID) ([]redaction, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := q.LockMessagesShared(ctx, store.LockMessagesSharedParams{WorkspaceID: workspaceID, Ids: ids})
	if err != nil {
		return nil, err
	}
	var out []redaction
	for _, r := range rows {
		if !r.Redacted {
			continue
		}
		m, err := q.GetMessage(ctx, store.GetMessageParams{WorkspaceID: workspaceID, ID: r.ID})
		if err != nil {
			return nil, err
		}
		msg, err := s.oneMessage(ctx, q, messageRow(m))
		if err != nil {
			return nil, err
		}
		out = append(out, redaction{messageID: r.ID.String(), message: jsonObject(msg)})
	}
	return out, nil
}

// messageIDsIn lists the ids of the messages a JSON document holds (objects with an id and a body).
func messageIDsIn(raw []byte) []uuid.UUID {
	var doc any
	if json.Unmarshal(raw, &doc) != nil {
		return nil
	}
	var out []uuid.UUID
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case []any:
			for _, e := range x {
				walk(e)
			}
		case map[string]any:
			if id, ok := x["id"].(string); ok {
				if _, hasBody := x["body"]; hasBody {
					if u, err := uuid.Parse(id); err == nil {
						out = append(out, u)
					}
				}
			}
			for _, e := range x {
				walk(e)
			}
		}
	}
	walk(doc)
	return out
}

// scrubRedacted rewrites a JSON copy written outside a redaction's transaction (a kept reply, a
// webhook payload) for the messages it holds that are redacted by now.
func (s *Server) scrubRedacted(ctx context.Context, q *store.Queries, workspaceID uuid.UUID, raw []byte) ([]byte, bool, error) {
	reds, err := s.redactedMessages(ctx, q, workspaceID, messageIDsIn(raw))
	if err != nil || len(reds) == 0 {
		return nil, false, err
	}
	changed := false
	for _, r := range reds {
		if b, ok := r.apply(raw); ok {
			raw, changed = b, true
		}
	}
	return raw, changed, nil
}
