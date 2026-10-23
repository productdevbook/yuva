package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/oapi-codegen/nullable"
	"github.com/riverqueue/river"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	roadmapProgressAttempts = 8
	roadmapProgressBatch    = 500
)

var (
	errNotFeedbackLink    = problem(http.StatusConflict, "not_feedback", "only feedback conversations link to ideas")
	errInboxNoBoard       = problem(http.StatusNotFound, "roadmap_off", "this inbox has no roadmap board")
	errCommentNeedsMember = errValidation("comment needs a member session")
)

// stageProgress is the progress a move to the stage sets on the linked feedback; none for ideas.
func stageProgress(stage string) string {
	switch stage {
	case stagePlanned:
		return string(oas.FeedbackProgressPlanned)
	case stageInProgress:
		return string(oas.FeedbackProgressInProgress)
	case stageShipped:
		return string(oas.FeedbackProgressDone)
	case stageClosed:
		return string(oas.FeedbackProgressWontDo)
	}
	return ""
}

type confirmLinkedKeyType struct{}

// confirmLinkedKey carries the count of linked feedback the caller was shown; the move checks it inside its transaction.
var confirmLinkedKey confirmLinkedKeyType

type ideaProgressKeyType struct{}

var ideaProgressKey ideaProgressKeyType

func ideaProgressFrom(ctx context.Context) *uuid.UUID {
	id, ok := ctx.Value(ideaProgressKey).(uuid.UUID)
	if !ok {
		return nil
	}
	return &id
}

// ideaTsQuery turns the words of a title search into a tsquery for the simple configuration: every
// word ANDed and the last one a prefix. Words hold letters and digits only, so each is safe as a lexeme.
func ideaTsQuery(q *string) (*string, error) {
	if q == nil {
		return nil, nil
	}
	if len([]rune(*q)) > maxIdeaQueryLen {
		return nil, errValidation("q must be at most 200 characters")
	}
	words := helpQueryWords(*q)
	if len(words) == 0 {
		return nil, errHelpQueryEmpty
	}
	words[len(words)-1] += ":*"
	return new(strings.Join(words, " & ")), nil
}

func (s *Server) searchTx(ctx context.Context, search bool, fn func(q *store.Queries) error) error {
	if !search {
		return fn(s.st.Queries)
	}
	err := s.st.InTxRaw(ctx, func(tx pgx.Tx, q *store.Queries) error {
		if _, err := tx.Exec(ctx, "SET LOCAL statement_timeout = '500ms'"); err != nil {
			return err
		}
		return fn(q)
	})
	if store.IsQueryCanceled(err) {
		return errHelpSearchTimeout
	}
	return err
}

func visibleIdea(ctx context.Context, q *store.Queries, p principal, id uuid.UUID, lock bool) (thread, error) {
	t, err := teamThread(ctx, q, p, id, lock)
	if err != nil {
		return t, err
	}
	if t.d.Kind != kindIdea {
		return t, errDiscussionGone
	}
	return t, nil
}

func stageChangeBody(c store.RoadmapStageChange) oas.RoadmapStageChange {
	out := oas.RoadmapStageChange{
		Id: c.ID, DiscussionId: c.IdeaID, FromStage: oas.IdeaStage(c.FromStage), ToStage: oas.IdeaStage(c.ToStage), MemberId: c.MemberID,
		ApiKeyId: c.ApiKeyID, CommentPostId: c.CommentPostID, Linked: c.Linked, ProgressSet: c.ProgressSet, ProgressSkipped: c.ProgressSkipped,
		FinishedAt: c.FinishedAt, CreatedAt: c.CreatedAt,
	}
	if c.Via != "" {
		out.Via = &c.Via
	}
	out.Progress = (*oas.FeedbackProgress)(c.Progress)
	return out
}

func (s *Server) MoveIdeaStage(ctx context.Context, req oas.MoveIdeaStageRequestObject) (oas.MoveIdeaStageResponseObject, error) {
	if err := s.roadmapOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	b := req.Body
	if !b.Stage.Valid() {
		return nil, errValidation("stage must be ideas, planned, in_progress, shipped or closed")
	}
	to := string(b.Stage)
	var comment string
	if b.Comment != nil {
		if p.isKey() {
			return nil, errCommentNeedsMember
		}
		var err error
		if comment, err = discussionBodyText(*b.Comment); err != nil {
			return nil, err
		}
	}
	var change *store.RoadmapStageChange
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		t, err := teamThread(ctx, q, p, req.DiscussionId, true)
		if err != nil {
			return err
		}
		if t.d.Kind != kindIdea {
			return errNotAnIdea
		}
		from := orZero(t.d.Stage)
		if from == to {
			return nil
		}
		linked, err := q.CountIdeaLinks(ctx, store.CountIdeaLinksParams{WorkspaceID: p.workspaceID, IdeaID: t.d.ID})
		if err != nil {
			return err
		}
		if confirm, ok := ctx.Value(confirmLinkedKey).(int32); ok {
			var want int32
			if progress := stageProgress(to); progress != "" {
				if want, err = q.CountIdeaLinksToChange(ctx, store.CountIdeaLinksToChangeParams{WorkspaceID: p.workspaceID, IdeaID: t.d.ID, Progress: progress}); err != nil {
					return err
				}
			}
			if confirm != want {
				return errConfirmMismatch(to, want)
			}
		}
		var commentID *uuid.UUID
		if comment != "" {
			id, err := s.addReply(ctx, q, events, t, memberActor(p), comment, nil, statePublished)
			if err != nil {
				return err
			}
			commentID = &id
		}
		target := stageProgress(to)
		arg := store.CreateRoadmapStageChangeParams{
			ID: newID(), WorkspaceID: p.workspaceID, IdeaID: t.d.ID, FromStage: from, ToStage: to, Via: p.via, CommentPostID: commentID,
			Linked: linked, Now: s.now(), SsoSettingsID: p.ssoSettings,
		}
		arg.MemberID, arg.ApiKeyID = statusAuthor(p)
		if p.isGrant() {
			arg.GrantID = new(p.grantID)
		}
		if target != "" {
			arg.Progress = &target
		}
		c, err := q.CreateRoadmapStageChange(ctx, arg)
		if err != nil {
			return err
		}
		change = &c
		if _, err := q.SetIdeaStage(ctx, store.SetIdeaStageParams{WorkspaceID: p.workspaceID, ID: t.d.ID, Stage: &to, Now: s.now()}); err != nil {
			return err
		}
		moved, err := loadThread(ctx, q, p.workspaceID, t.d.ID, false)
		if err != nil {
			return err
		}
		inbox := moved.d.InboxID
		events.add(eventDiscussionStageChanged, &inbox, nil, oas.DiscussionStageEventData{
			Discussion: discussionBody(moved.d, moved.opening(), &teamViewer), From: oas.IdeaStage(from), To: b.Stage, Linked: linked,
		})
		if err := touchIdeaBoard(ctx, q, events, moved.d); err != nil {
			return err
		}
		if to != stageIdeas {
			if err := s.notifyStageFollowers(ctx, q, events, p.workspaceID, t.d.ID); err != nil {
				return err
			}
		}
		if target != "" {
			events.job(RoadmapProgressArgs{WorkspaceID: p.workspaceID, ChangeID: c.ID}, nil)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	d, err := s.teamDiscussion(ctx, p, req.DiscussionId)
	if err != nil {
		return nil, err
	}
	out := oas.RoadmapStageMoveResult{Discussion: d}
	if change != nil {
		body := stageChangeBody(*change)
		out.Change = &body
	}
	return oas.MoveIdeaStage200JSONResponse(out), nil
}

func (s *Server) ListIdeaStageChanges(ctx context.Context, req oas.ListIdeaStageChangesRequestObject) (oas.ListIdeaStageChangesResponseObject, error) {
	if err := s.roadmapOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	lim, err := pageSize(req.Params.Limit)
	if err != nil {
		return nil, err
	}
	at, cid, err := decodeCursor(req.Params.Cursor)
	if err != nil {
		return nil, err
	}
	t, err := visibleIdea(ctx, s.st.Queries, p, req.DiscussionId, false)
	if err != nil {
		return nil, err
	}
	rows, err := s.st.ListRoadmapStageChanges(ctx, store.ListRoadmapStageChangesParams{
		WorkspaceID: p.workspaceID, IdeaID: t.d.ID, CursorAt: at, CursorID: cid, Lim: lim + 1,
	})
	if err != nil {
		return nil, err
	}
	out := oas.ListIdeaStageChanges200JSONResponse{Items: []oas.RoadmapStageChange{}}
	if len(rows) > int(lim) {
		rows = rows[:lim]
		c := encodeCursor(rows[lim-1].CreatedAt, rows[lim-1].ID)
		out.NextCursor = &c
	}
	for _, r := range rows {
		out.Items = append(out.Items, stageChangeBody(r))
	}
	return out, nil
}

func (s *Server) notifyStageFollowers(ctx context.Context, q *store.Queries, events *eventBatch, ws, ideaID uuid.UUID) error {
	return s.notifyAll(ctx, q, events, ws, ideaID, nil, notifyStage, nil)
}

// A feedback links to one idea; linking it to another moves the link.
func (s *Server) linkIdea(ctx context.Context, q *store.Queries, events *eventBatch, p principal, d store.Discussion, c store.Conversation) error {
	cur, err := q.GetConversationIdeaLink(ctx, store.GetConversationIdeaLinkParams{WorkspaceID: p.workspaceID, ConversationID: c.ID})
	if err == nil && cur == d.ID {
		return nil
	}
	if err != nil && !store.IsNotFound(err) {
		return err
	}
	n, err := q.CountIdeaLinks(ctx, store.CountIdeaLinksParams{WorkspaceID: p.workspaceID, IdeaID: d.ID})
	if err != nil {
		return err
	}
	if int(n) >= maxIdeaLinks {
		return errTooManyIdeaLinks()
	}
	arg := store.LinkIdeaConversationParams{WorkspaceID: p.workspaceID, ConversationID: c.ID, IdeaID: d.ID, Now: s.now()}
	arg.LinkedByMemberID, arg.LinkedByApiKeyID = statusAuthor(p)
	if err := q.LinkIdeaConversation(ctx, arg); err != nil {
		return err
	}
	if target := stageProgress(orZero(d.Stage)); target != "" {
		return s.setFeedbackProgress(ctx, q, events, p, d.ID, c.ID, target)
	}
	return nil
}

// setFeedbackProgress sets a linked feedback's progress as a member would on the conversation, so the
// timeline, the contact and the webhooks see the usual change, with the idea named on the event.
func (s *Server) setFeedbackProgress(ctx context.Context, q *store.Queries, events *eventBatch, p principal, ideaID, conversationID uuid.UUID, progress string) error {
	ictx := context.WithValue(ctx, ideaProgressKey, ideaID)
	_, err := s.changeConversation(ictx, q, events, p, conversationID, oas.ConversationUpdate{
		FeedbackProgress: nullable.NewNullableWithValue(oas.FeedbackProgress(progress)),
	}, nil, nil)
	return err
}

func (s *Server) LinkIdeaConversation(ctx context.Context, req oas.LinkIdeaConversationRequestObject) (oas.LinkIdeaConversationResponseObject, error) {
	if err := s.roadmapOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		t, err := teamThread(ctx, q, p, req.DiscussionId, true)
		if err != nil {
			return err
		}
		if t.d.Kind != kindIdea {
			return errNotAnIdea
		}
		c, err := visibleConversation(ctx, q, p, req.ConversationId, true)
		if err != nil {
			return err
		}
		if c.InboxID != t.d.InboxID {
			return errDiscussionGone
		}
		if c.Kind != string(oas.ConversationKindFeedback) {
			return errNotFeedbackLink
		}
		return s.linkIdea(ctx, q, events, p, t.d, c)
	})
	if err != nil {
		return nil, err
	}
	return oas.LinkIdeaConversation204Response{}, nil
}

func (s *Server) UnlinkIdeaConversation(ctx context.Context, req oas.UnlinkIdeaConversationRequestObject) (oas.UnlinkIdeaConversationResponseObject, error) {
	if err := s.roadmapOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		t, err := teamThread(ctx, q, p, req.DiscussionId, true)
		if err != nil {
			return err
		}
		if t.d.Kind != kindIdea {
			return errNotAnIdea
		}
		c, err := visibleConversation(ctx, q, p, req.ConversationId, true)
		if err != nil {
			return err
		}
		_, err = q.UnlinkIdeaConversation(ctx, store.UnlinkIdeaConversationParams{WorkspaceID: p.workspaceID, IdeaID: t.d.ID, ConversationID: c.ID})
		return err
	})
	if err != nil {
		return nil, err
	}
	return oas.UnlinkIdeaConversation204Response{}, nil
}

func fillIdeas(ctx context.Context, q *store.Queries, ws uuid.UUID, convs ...*oas.Conversation) error {
	var ids []uuid.UUID
	for _, c := range convs {
		if c.Kind == oas.ConversationKindFeedback {
			ids = append(ids, c.Id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := q.ListConversationIdeas(ctx, store.ListConversationIdeasParams{WorkspaceID: ws, ConversationIds: ids})
	if err != nil {
		return err
	}
	for _, r := range rows {
		for _, c := range convs {
			if c.Id == r.ConversationID {
				c.Idea = &oas.IdeaRef{Id: r.ID, Title: r.Title, Stage: oas.IdeaStage(r.Stage)}
			}
		}
	}
	return nil
}

type RoadmapProgressArgs struct {
	WorkspaceID uuid.UUID `json:"workspace_id"`
	ChangeID    uuid.UUID `json:"change_id"`
}

func (RoadmapProgressArgs) Kind() string { return "roadmap_progress" }

func (RoadmapProgressArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: roadmapProgressAttempts, UniqueOpts: river.UniqueOpts{ByArgs: true}}
}

type roadmapProgressWorker struct {
	river.WorkerDefaults[RoadmapProgressArgs]
	s *Server
}

func (w *roadmapProgressWorker) Work(ctx context.Context, job *river.Job[RoadmapProgressArgs]) error {
	err := w.s.RunRoadmapProgress(ctx, job.Args)
	if err != nil && job.Attempt >= job.MaxAttempts {
		w.s.finishRoadmapProgress(context.WithoutCancel(ctx), job.Args, 0)
	}
	return err
}

var roadmapBatchSize = roadmapProgressBatch

// RunRoadmapProgress walks the feedback linked to an idea in batches and sets the progress its
// stage move names, each conversation in its own transaction. Before every batch it checks that no
// newer move has replaced this one and that the member, key or grant that moved the idea still may.
func (s *Server) RunRoadmapProgress(ctx context.Context, a RoadmapProgressArgs) error {
	ch, err := s.st.GetRoadmapStageChange(ctx, store.GetRoadmapStageChangeParams{WorkspaceID: a.WorkspaceID, ID: a.ChangeID})
	if store.IsNotFound(err) {
		return nil
	}
	if err != nil || ch.FinishedAt != nil || ch.Progress == nil {
		return err
	}
	target := *ch.Progress
	var after uuid.UUID
	var visited int32
	for {
		latest, err := s.st.LatestRoadmapStageChange(ctx, store.LatestRoadmapStageChangeParams{WorkspaceID: a.WorkspaceID, IdeaID: ch.IdeaID})
		if err != nil {
			return err
		}
		if latest != ch.ID {
			break
		}
		p, refused, err := s.callerPrincipal(ctx, a.WorkspaceID, ch.MemberID, ch.ApiKeyID, ch.GrantID, ch.SsoSettingsID, ch.Via, false)
		if err != nil {
			return err
		}
		if refused != "" && refused != reasonRevoked {
			s.log.WarnContext(ctx, "roadmap progress stopped", slog.String("idea_id", ch.IdeaID.String()), slog.String("code", refused))
		}
		if refused != "" || requireScope(p, oas.RoadmapStage) != nil {
			break
		}
		rows, err := s.st.IdeaLinkBatch(ctx, store.IdeaLinkBatchParams{WorkspaceID: a.WorkspaceID, IdeaID: ch.IdeaID, After: after, Lim: int32(roadmapBatchSize)})
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			visited = max(visited, ch.Linked)
			break
		}
		for _, r := range rows {
			after = r.ConversationID
			visited++
			if r.FeedbackProgress != nil && *r.FeedbackProgress == target {
				continue
			}
			if err := s.applyIdeaProgress(ctx, p, ch, r.ConversationID, target); err != nil {
				return err
			}
		}
	}
	s.finishRoadmapProgress(ctx, a, visited)
	return nil
}

func (s *Server) applyIdeaProgress(ctx context.Context, p principal, ch store.RoadmapStageChange, conversationID uuid.UUID, target string) error {
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		if _, err := q.LockDiscussion(ctx, store.LockDiscussionParams{WorkspaceID: p.workspaceID, ID: ch.IdeaID}); err != nil {
			return err
		}
		latest, err := q.LatestRoadmapStageChange(ctx, store.LatestRoadmapStageChangeParams{WorkspaceID: p.workspaceID, IdeaID: ch.IdeaID})
		if err != nil || latest != ch.ID {
			return err
		}
		linked, err := q.IdeaConversationLinked(ctx, store.IdeaConversationLinkedParams{WorkspaceID: p.workspaceID, IdeaID: ch.IdeaID, ConversationID: conversationID})
		if err != nil || !linked {
			return err
		}
		if err := s.setFeedbackProgress(ctx, q, events, p, ch.IdeaID, conversationID, target); err != nil {
			return err
		}
		return q.BumpRoadmapStageSet(ctx, store.BumpRoadmapStageSetParams{WorkspaceID: p.workspaceID, ID: ch.ID})
	})
	var e *apiError
	if store.IsNotFound(err) {
		return nil
	}
	if errors.As(err, &e) && e.Status < http.StatusInternalServerError && e.Status != http.StatusTooManyRequests {
		s.log.WarnContext(ctx, "idea progress not set", slog.String("conversation_id", conversationID.String()), slog.String("code", e.Code))
		return nil
	}
	return err
}

func (s *Server) finishRoadmapProgress(ctx context.Context, a RoadmapProgressArgs, visited int32) {
	ch, err := s.st.GetRoadmapStageChange(ctx, store.GetRoadmapStageChangeParams{WorkspaceID: a.WorkspaceID, ID: a.ChangeID})
	if err == nil {
		err = s.st.FinishRoadmapStageChange(ctx, store.FinishRoadmapStageChangeParams{
			WorkspaceID: a.WorkspaceID, ID: a.ChangeID, Skipped: max(max(ch.Linked, visited)-ch.ProgressSet, 0), Now: s.now(),
		})
	}
	if err != nil {
		s.log.WarnContext(ctx, "roadmap progress not closed", slog.String("change_id", a.ChangeID.String()), slog.Any("error", err))
	}
}
