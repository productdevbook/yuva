package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"golang.org/x/net/html"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

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
		var me *store.MessageEmail
		if e, err := q.GetMessageEmail(ctx, store.GetMessageEmailParams{WorkspaceID: p.workspaceID, MessageID: m.ID}); err == nil {
			me = &e
		} else if !store.IsNotFound(err) {
			return err
		}
		first, err := q.GetFirstPublicMessage(ctx, store.GetFirstPublicMessageParams{WorkspaceID: p.workspaceID, ConversationID: c.ID})
		if err != nil && !store.IsNotFound(err) {
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
		if me != nil && me.Direction == string(oas.Out) && m.Kind == string(oas.MessageKindMessage) {
			quoting, err := dropQuotesOf(ctx, q, p.workspaceID, m, me.HeaderMessageID)
			if err != nil {
				return err
			}
			keys = append(keys, quoting...)
		}
		red := redaction{messageID: m.ID.String(), convID: c.ID.String()}
		next := c
		if c.Kind == string(oas.ConversationKindFeedback) && first == m.ID {
			if next, err = q.ClearFeedbackText(ctx, store.ClearFeedbackTextParams{WorkspaceID: p.workspaceID, ID: c.ID, Now: now}); err != nil {
				return err
			}
			red.feedback = true
		}
		if me != nil && me.Direction == string(oas.In) && first == m.ID && me.Subject != "" && me.Subject == c.Subject {
			if next, err = q.ClearConversationSubject(ctx, store.ClearConversationSubjectParams{WorkspaceID: p.workspaceID, ID: c.ID, Now: now}); err != nil {
				return err
			}
			if err := q.ClearEmailSubjects(ctx, store.ClearEmailSubjectsParams{WorkspaceID: p.workspaceID, ConversationID: c.ID, Subject: c.Subject}); err != nil {
				return err
			}
			red.subject = c.Subject
		}
		if out, err = s.oneMessage(ctx, q, messageRow(r)); err != nil {
			return err
		}
		red.message = jsonObject(out)
		if err := red.rewriteCopies(ctx, q, p.workspaceID, c.ID); err != nil {
			return err
		}
		events.deleteStored(p.workspaceID, keys)
		if err := s.scrubSlackMessage(ctx, q, events, p.workspaceID, c.ID, m.ID); err != nil {
			return err
		}
		if red.subject != "" {
			if err := s.scrubSlackConversations(ctx, q, events, p.workspaceID, slackScrubRedacted, []uuid.UUID{c.ID}); err != nil {
				return err
			}
		}
		events.conversation(realtime.MessageRedacted, c, out)
		if red.feedback || red.subject != "" {
			conv, err := oneConversation(ctx, q, next)
			if err != nil {
				return err
			}
			events.conversation(realtime.ConversationUpdated, next, conv)
		}
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

// dropQuotesOf takes the quoted history and raw source from the contact's later e-mails that
// answer the redacted e-mail (headerID in In-Reply-To or References) and quote all of its text,
// keeping only what they newly said; it returns the raw sources to delete.
func dropQuotesOf(ctx context.Context, q *store.Queries, workspaceID uuid.UUID, m store.GetMessageRow, headerID string) ([]string, error) {
	quoted := quoteText(m.Body)
	if quoted == "" {
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
		answers := r.InReplyTo != nil && *r.InReplyTo == headerID || slices.Contains(r.ReferencesIds, headerID)
		if !answers {
			continue
		}
		if !strings.Contains(quoteText(r.FullText), quoted) && (r.FullHtml == nil || !strings.Contains(quoteText(htmlText(*r.FullHtml)), quoted)) {
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

const storageDeleteChunk = 500

// storageDeleteJobs are the retried jobs that delete stored files whose rows the same transaction
// deletes, so a failing store neither loses a file nor keeps one nothing points to any more.
func storageDeleteJobs(workspaceID uuid.UUID, keys []string) []river.InsertManyParams {
	var out []river.InsertManyParams
	for c := range slices.Chunk(keys, storageDeleteChunk) {
		out = append(out, river.InsertManyParams{Args: StorageDeleteArgs{WorkspaceID: workspaceID, Keys: c}})
	}
	return out
}

func (b *eventBatch) deleteStored(workspaceID uuid.UUID, keys []string) {
	b.jobs = append(b.jobs, storageDeleteJobs(workspaceID, keys)...)
}

func (s *Server) deleteStoredTx(ctx context.Context, tx pgx.Tx, workspaceID uuid.UUID, keys []string) error {
	jobs := storageDeleteJobs(workspaceID, keys)
	if len(jobs) == 0 {
		return nil
	}
	_, err := s.jobs.InsertManyTx(ctx, tx, jobs)
	return err
}

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
