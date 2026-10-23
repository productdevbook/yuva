package api

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"
	"uuid"

	"github.com/riverqueue/river"

	"github.com/productdevbook/yuva/api/internal/jobs"
	"github.com/productdevbook/yuva/api/internal/slack"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	slackScrubRedacted = "redacted"
	slackScrubTrashed  = "trashed"
	slackScrubDeleted  = "deleted"
)

// SlackScrubArgs rewrites one earlier Slack post to a line without names, subject or text. It
// carries only the post's address, never what it said.
type SlackScrubArgs struct {
	WorkspaceID    uuid.UUID `json:"workspace_id"`
	ChannelID      string    `json:"channel_id"`
	TS             string    `json:"ts"`
	Reason         string    `json:"reason"`
	ConversationID uuid.UUID `json:"conversation_id"`
}

func (SlackScrubArgs) Kind() string { return "slack_scrub" }

func (SlackScrubArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: jobs.OutboundQueue}
}

type slackScrubWorker struct {
	river.WorkerDefaults[SlackScrubArgs]
	s *Server
}

func (w *slackScrubWorker) Work(ctx context.Context, job *river.Job[SlackScrubArgs]) error {
	wait, err := w.s.ScrubSlackPost(ctx, job.Args)
	if err != nil {
		return err
	}
	if wait > 0 {
		return river.JobSnooze(wait)
	}
	return nil
}

func (w *slackScrubWorker) Timeout(*river.Job[SlackScrubArgs]) time.Duration { return 2 * time.Minute }

type slackPostRef struct {
	ChannelID      string
	Ts             string
	ConversationID uuid.UUID
}

// queueSlackScrub queues the rewrite of posts already taken out of slack_posts. Nothing is queued
// while Slack is off for this server; a disconnected workspace has no posts left to take.
func (s *Server) queueSlackScrub(events *eventBatch, ws uuid.UUID, reason string, posts []slackPostRef) {
	if !s.slack.on() {
		return
	}
	for _, p := range posts {
		events.job(SlackScrubArgs{WorkspaceID: ws, ChannelID: p.ChannelID, TS: p.Ts, Reason: reason, ConversationID: p.ConversationID}, &river.InsertOpts{MaxAttempts: slackPostAttempts})
	}
}

func (s *Server) scrubSlackConversations(ctx context.Context, q *store.Queries, events *eventBatch, ws uuid.UUID, reason string, ids []uuid.UUID) error {
	rows, err := q.TakeSlackPostsOfConversations(ctx, store.TakeSlackPostsOfConversationsParams{WorkspaceID: ws, ConversationIds: ids})
	if err != nil {
		return err
	}
	posts := make([]slackPostRef, len(rows))
	for i, r := range rows {
		posts[i] = slackPostRef{r.ChannelID, r.Ts, r.ConversationID}
	}
	s.queueSlackScrub(events, ws, reason, posts)
	return nil
}

func (s *Server) scrubSlackInbox(ctx context.Context, q *store.Queries, events *eventBatch, ws, inboxID uuid.UUID) error {
	rows, err := q.TakeSlackPostsOfInbox(ctx, store.TakeSlackPostsOfInboxParams{WorkspaceID: ws, InboxID: inboxID})
	if err != nil {
		return err
	}
	posts := make([]slackPostRef, len(rows))
	for i, r := range rows {
		posts[i] = slackPostRef{r.ChannelID, r.Ts, r.ConversationID}
	}
	s.queueSlackScrub(events, ws, slackScrubDeleted, posts)
	return nil
}

func (s *Server) scrubSlackContact(ctx context.Context, q *store.Queries, events *eventBatch, ws, contactID uuid.UUID) error {
	rows, err := q.TakeSlackPostsOfContact(ctx, store.TakeSlackPostsOfContactParams{WorkspaceID: ws, ContactID: contactID})
	if err != nil {
		return err
	}
	posts := make([]slackPostRef, len(rows))
	for i, r := range rows {
		posts[i] = slackPostRef{r.ChannelID, r.Ts, r.ConversationID}
	}
	s.queueSlackScrub(events, ws, slackScrubDeleted, posts)
	return nil
}

func (s *Server) scrubSlackMessage(ctx context.Context, q *store.Queries, events *eventBatch, ws, conversationID, messageID uuid.UUID) error {
	rows, err := q.TakeSlackPostsOfMessage(ctx, store.TakeSlackPostsOfMessageParams{WorkspaceID: ws, ConversationID: conversationID, MessageID: &messageID})
	if err != nil {
		return err
	}
	posts := make([]slackPostRef, len(rows))
	for i, r := range rows {
		posts[i] = slackPostRef{r.ChannelID, r.Ts, r.ConversationID}
	}
	s.queueSlackScrub(events, ws, slackScrubRedacted, posts)
	return nil
}

// slackGoneCodes are refusals for one post that no retry fixes and that say nothing about the
// connection: the post or its channel is gone, or Slack no longer lets the bot edit it.
var slackGoneCodes = map[string]bool{
	"message_not_found": true, "channel_not_found": true, "cant_update_message": true,
	"edit_window_closed": true, "is_archived": true, "not_in_channel": true, "restricted_action": true,
}

// ScrubSlackPost makes one attempt at rewriting a post. A rate limit returns how long to wait. The
// link to the conversation stays only while the conversation exists.
func (s *Server) ScrubSlackPost(ctx context.Context, a SlackScrubArgs) (time.Duration, error) {
	ws := a.WorkspaceID
	if !s.slack.on() {
		return 0, nil
	}
	inst, err := s.st.GetSlackInstallation(ctx, ws)
	if store.IsNotFound(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	token, err := s.slackToken(inst)
	if err != nil {
		return 0, s.slackError(ctx, ws, "the Slack token cannot be decrypted; connect Slack again")
	}
	var b strings.Builder
	switch a.Reason {
	case slackScrubRedacted:
		b.WriteString("_Message removed._")
	case slackScrubTrashed:
		b.WriteString("_This conversation is in the trash._")
	default:
		b.WriteString("_This conversation was deleted._")
	}
	if a.Reason != slackScrubDeleted {
		_, err := s.st.GetConversation(ctx, store.GetConversationParams{WorkspaceID: ws, ID: a.ConversationID})
		switch {
		case err == nil:
			b.WriteString("\n<" + strings.TrimRight(s.auth.PublicURL, "/") + conversationPath(ws, a.ConversationID) + "|Open in Yuva>")
		case !store.IsNotFound(err):
			return 0, err
		}
	}
	cctx, cancel := context.WithTimeout(ctx, slackCallTimeout)
	defer cancel()
	err = s.slackAPI.Update(cctx, token, a.ChannelID, a.TS, b.String())
	var se *slack.Error
	switch {
	case errors.As(err, &se) && se.RetryAfter > 0:
		return se.RetryAfter, nil
	case errors.As(err, &se) && slackGoneCodes[se.Code]:
		s.log.InfoContext(ctx, "slack post not rewritten", slog.String("workspace_id", ws.String()), slog.String("reason", se.Code))
		return 0, nil
	case errors.As(err, &se) && !se.Temporary():
		return 0, s.slackError(ctx, ws, se.Code)
	}
	return 0, err
}

// scrubLatePost queues the rewrite of a post whose conversation was deleted or trashed, or whose
// message was redacted, while it was being posted. It reports whether it did.
func (s *Server) scrubLatePost(ctx context.Context, ws uuid.UUID, channel, ts string, conversationID uuid.UUID, messageID *uuid.UUID) (bool, error) {
	reason := ""
	c, err := s.st.GetConversation(ctx, store.GetConversationParams{WorkspaceID: ws, ID: conversationID})
	switch {
	case store.IsNotFound(err):
		reason = slackScrubDeleted
	case err != nil:
		return false, err
	case c.TrashedAt != nil:
		reason = slackScrubTrashed
	case messageID != nil:
		m, err := s.st.GetMessage(ctx, store.GetMessageParams{WorkspaceID: ws, ID: *messageID})
		if err != nil && !store.IsNotFound(err) {
			return false, err
		}
		if err == nil && m.RedactedAt != nil {
			reason = slackScrubRedacted
		}
	}
	if reason == "" {
		return false, nil
	}
	_, err = s.jobs.Insert(ctx, SlackScrubArgs{WorkspaceID: ws, ChannelID: channel, TS: ts, Reason: reason, ConversationID: conversationID}, &river.InsertOpts{MaxAttempts: slackPostAttempts})
	return true, err
}

// scrubWorkspaceSlack rewrites every recorded post of a workspace being purged before its
// installation goes with the workspace row. A rate limit or failure stops it; the purge runs again.
func (s *Server) scrubWorkspaceSlack(ctx context.Context, ws uuid.UUID) error {
	for {
		rows, err := s.st.ListSlackPostsBatch(ctx, store.ListSlackPostsBatchParams{WorkspaceID: ws, MaxRows: 100})
		if err != nil || len(rows) == 0 {
			return err
		}
		for _, r := range rows {
			wait, err := s.ScrubSlackPost(ctx, SlackScrubArgs{WorkspaceID: ws, ChannelID: r.ChannelID, TS: r.Ts, Reason: slackScrubDeleted, ConversationID: r.ConversationID})
			if err != nil {
				return err
			}
			if wait > 0 {
				return errors.New("slack is rate limiting; the purge goes on later")
			}
			if err := s.st.DeleteSlackPost(ctx, store.DeleteSlackPostParams{WorkspaceID: ws, ChannelID: r.ChannelID, Ts: r.Ts}); err != nil {
				return err
			}
		}
	}
}
