package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"uuid"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"golang.org/x/net/html"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

const quoteProbeRunes = 120

var (
	errDraftNotRedactable = problem(http.StatusConflict, "not_redactable", "a draft is discarded with DELETE /v1/messages/{id}, not redacted")
	errEventNotRedactable = problem(http.StatusConflict, "not_redactable", "a timeline event cannot be redacted")
)

func trueOrNil(b bool) *bool {
	if !b {
		return nil
	}
	return &b
}

// mayRedact admits keys (their scope is checked before), owners and admins, and the member who
// wrote the message.
func mayRedact(p principal, m store.GetMessageRow) error {
	if p.isKey() || p.role == roleOwner || p.role == roleAdmin {
		return nil
	}
	if m.AuthorMemberID != nil && *m.AuthorMemberID == p.memberID {
		return nil
	}
	return errForbidden
}

func (s *Server) RedactMessage(ctx context.Context, req oas.RedactMessageRequestObject) (oas.RedactMessageResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireScope(p, oas.MessagesRedact); err != nil {
		return nil, err
	}
	var (
		out      oas.Message
		redacted bool
	)
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		m, err := q.GetMessage(ctx, store.GetMessageParams{WorkspaceID: p.workspaceID, ID: req.MessageId})
		if store.IsNotFound(err) {
			return errMessageGone
		}
		if err != nil {
			return err
		}
		c, err := visibleConversation(ctx, q, p, m.ConversationID, true)
		if errors.Is(err, errConversationGone) {
			return errMessageGone
		}
		if err != nil {
			return err
		}
		if m, err = q.GetMessage(ctx, store.GetMessageParams{WorkspaceID: p.workspaceID, ID: req.MessageId}); err != nil {
			return err
		}
		if err := mayRedact(p, m); err != nil {
			return err
		}
		switch {
		case m.Draft:
			return errDraftNotRedactable
		case m.Kind == string(oas.MessageKindEvent):
			return errEventNotRedactable
		case m.RedactedAt != nil:
			out, err = s.oneMessage(ctx, q, messageRow(m))
			return err
		}
		now := s.now()
		arg := store.RedactMessageParams{WorkspaceID: p.workspaceID, ID: m.ID, Now: now}
		if p.isKey() {
			arg.ApiKeyID = new(p.keyID)
		} else {
			arg.MemberID = new(p.memberID)
		}
		r, err := q.RedactMessage(ctx, arg)
		if err != nil {
			return err
		}
		keys, err := q.DeleteMessageAttachments(ctx, store.DeleteMessageAttachmentsParams{WorkspaceID: p.workspaceID, MessageID: m.ID})
		if err != nil {
			return err
		}
		raw, err := q.RedactMessageEmail(ctx, store.RedactMessageEmailParams{WorkspaceID: p.workspaceID, MessageID: m.ID})
		if err != nil {
			return err
		}
		for _, k := range raw {
			if k != nil {
				keys = append(keys, *k)
			}
		}
		quoting, err := dropQuotesOf(ctx, q, p.workspaceID, m)
		if err != nil {
			return err
		}
		keys = append(keys, quoting...)
		if out, err = s.oneMessage(ctx, q, messageRow(r)); err != nil {
			return err
		}
		payload := mustJSON(out)
		if _, err := q.RedactMessageEvents(ctx, store.RedactMessageEventsParams{
			WorkspaceID: p.workspaceID, ConversationID: &c.ID, MessageID: m.ID.String(), Message: payload,
		}); err != nil {
			return err
		}
		if _, err := q.RedactMessageWebhookDeliveries(ctx, store.RedactMessageWebhookDeliveriesParams{
			WorkspaceID: p.workspaceID, MessageID: m.ID.String(), Message: payload,
		}); err != nil {
			return err
		}
		if err := redactStoredReplies(ctx, q, p.workspaceID, m.ID.String(), payload); err != nil {
			return err
		}
		if len(keys) > 0 {
			events.job(StorageDeleteArgs{WorkspaceID: p.workspaceID, Keys: keys}, nil)
		}
		events.conversation(realtime.MessageRedacted, c, out)
		redacted = true
		return s.audit(ctx, q, p, oas.AuditMessageRedacted, oas.AuditTargetMessage, m.ID, auditDetails{
			"conversation_id": c.ID, "inbox_id": c.InboxID, "kind": m.Kind, "files": len(keys),
		})
	})
	if err != nil {
		return nil, err
	}
	if redacted {
		s.dropPushes(ctx, req.MessageId)
	}
	return oas.RedactMessage200JSONResponse(out), nil
}

// redactStoredReplies rewrites the answers kept for Idempotency-Key replays that show the
// message, so a retried request answers it redacted instead of with its content or a new copy.
func redactStoredReplies(ctx context.Context, q *store.Queries, workspaceID uuid.UUID, id string, message []byte) error {
	rows, err := q.ListIdempotentRepliesMentioning(ctx, store.ListIdempotentRepliesMentioningParams{WorkspaceID: workspaceID, Needle: id})
	if err != nil {
		return err
	}
	for _, r := range rows {
		body, ok := redactedReply(r.Body, id, message, strings.HasPrefix(r.Path, "/client/"))
		if !ok {
			continue
		}
		if err := q.SetIdempotentReplyBody(ctx, store.SetIdempotentReplyBodyParams{
			WorkspaceID: workspaceID, CallerType: r.CallerType, CallerID: r.CallerID, Key: r.Key, Body: body,
		}); err != nil {
			return err
		}
	}
	return nil
}

func redactedReply(body []byte, id string, message []byte, client bool) ([]byte, bool) {
	var doc map[string]any
	if json.Unmarshal(body, &doc) != nil {
		return nil, false
	}
	changed := false
	fix := func(m map[string]any) map[string]any {
		if m["id"] != id {
			return m
		}
		changed = true
		if !client {
			var out map[string]any
			_ = json.Unmarshal(message, &out)
			return out
		}
		delete(m, "html")
		delete(m, "cards")
		m["body"], m["attachments"], m["redacted"] = "", []any{}, true
		return m
	}
	doc = fix(doc)
	if m, ok := doc["message"].(map[string]any); ok {
		doc["message"] = fix(m)
	}
	if c, ok := doc["conversation"].(map[string]any); ok {
		if lm, ok := c["last_message"].(map[string]any); ok && lm["id"] == id {
			lm["text"], lm["redacted"], changed = "", true, true
		}
	}
	if !changed {
		return nil, false
	}
	return mustJSON(doc), true
}

func (s *Server) oneMessage(ctx context.Context, q *store.Queries, m messageRow) (oas.Message, error) {
	atts, err := messageAttachments(ctx, q, m.WorkspaceID, []uuid.UUID{m.ID})
	if err != nil {
		return oas.Message{}, err
	}
	ems, err := emailSummaries(ctx, q, m.WorkspaceID, []uuid.UUID{m.ID})
	if err != nil {
		return oas.Message{}, err
	}
	return withEmail(messageBody(m, atts[m.ID]), ems[m.ID]), nil
}

// dropQuotesOf takes the quoted history and raw source from the contact's later e-mails in the
// conversation that quote the redacted message, keeping only what they newly said; it returns the
// raw sources to delete.
func dropQuotesOf(ctx context.Context, q *store.Queries, workspaceID uuid.UUID, m store.GetMessageRow) ([]string, error) {
	probe := quoteProbe(m.Body)
	if probe == "" {
		return nil, nil
	}
	rows, err := q.ListLaterInboundEmails(ctx, store.ListLaterInboundEmailsParams{
		WorkspaceID: workspaceID, ConversationID: m.ConversationID, MessageID: m.ID, Since: m.CreatedAt,
	})
	if err != nil {
		return nil, err
	}
	var keys []string
	for _, r := range rows {
		if !strings.Contains(quoteText(r.FullText), probe) && (r.FullHtml == nil || !strings.Contains(quoteText(htmlText(*r.FullHtml)), probe)) {
			continue
		}
		if err := q.DropEmailQuotes(ctx, store.DropEmailQuotesParams{WorkspaceID: workspaceID, MessageID: r.MessageID, FullText: r.Body, FullHtml: r.Html}); err != nil {
			return nil, err
		}
		if r.RawKey != nil {
			keys = append(keys, *r.RawKey)
		}
	}
	return keys, nil
}

// quoteText folds what mail clients change when they quote: case, line breaks and the leading '>'.
func quoteText(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.ReplaceAll(s, ">", " "))), " ")
}

// quoteProbe is the start of a message as a later e-mail would quote it.
func quoteProbe(body string) string {
	words := strings.Fields(quoteText(body))
	var b strings.Builder
	for _, w := range words {
		if b.Len() > 0 && len([]rune(b.String()))+1+len([]rune(w)) > quoteProbeRunes {
			break
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(w)
	}
	return b.String()
}

func htmlText(src string) string {
	z := html.NewTokenizer(strings.NewReader(src))
	var b strings.Builder
	for {
		switch z.Next() {
		case html.ErrorToken:
			return b.String()
		case html.TextToken:
			b.Write(z.Text())
			b.WriteByte(' ')
		}
	}
}

// dropPushes deletes the pushes a redacted message caused, sent or not: their payload quotes it.
// A push being sent right now cannot be stopped.
func (s *Server) dropPushes(ctx context.Context, messageID uuid.UUID) {
	ctx = context.WithoutCancel(ctx)
	params := river.NewJobListParams().Kinds(PushArgs{}.Kind()).First(1000).
		States(rivertype.JobStateAvailable, rivertype.JobStateCancelled, rivertype.JobStateCompleted, rivertype.JobStateDiscarded,
			rivertype.JobStatePending, rivertype.JobStateRetryable, rivertype.JobStateScheduled).
		Where("args->>'message_id' = @message_id", river.NamedArgs{"message_id": messageID.String()})
	res, err := s.jobs.JobList(ctx, params)
	if err != nil {
		s.log.ErrorContext(ctx, "redact: list pushes", slog.String("message_id", messageID.String()), slog.Any("error", err))
		return
	}
	for _, j := range res.Jobs {
		if _, err := s.jobs.JobDelete(ctx, j.ID); err != nil {
			s.log.WarnContext(ctx, "redact: delete push", slog.Int64("job_id", j.ID), slog.Any("error", err))
		}
	}
}

type StorageDeleteArgs struct {
	WorkspaceID uuid.UUID `json:"workspace_id"`
	Keys        []string  `json:"keys"`
}

func (StorageDeleteArgs) Kind() string { return "storage_delete" }

type storageDeleteWorker struct {
	river.WorkerDefaults[StorageDeleteArgs]
	s *Server
}

func (w *storageDeleteWorker) Work(ctx context.Context, job *river.Job[StorageDeleteArgs]) error {
	return w.s.DeleteStoredObjects(ctx, job.Args)
}

func (w *storageDeleteWorker) Timeout(*river.Job[StorageDeleteArgs]) time.Duration { return 2 * time.Minute }

// DeleteStoredObjects removes stored files whose rows are already gone; a failure is retried.
func (s *Server) DeleteStoredObjects(ctx context.Context, a StorageDeleteArgs) error {
	var errs []error
	for _, k := range a.Keys {
		if err := s.objects.Delete(ctx, k); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
