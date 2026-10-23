package api

import (
	"cmp"
	"context"
	"fmt"
	"strings"
	"time"
	"uuid"

	"github.com/riverqueue/river"

	"github.com/productdevbook/yuva/api/internal/slack"
	"github.com/productdevbook/yuva/api/internal/store"
)

// SlackSLAArgs posts one SLA warning or breach to the inbox's SLA channel. It is queued by the
// check that marks the target warned or breached, which happens once, so a post is never repeated.
type SlackSLAArgs struct {
	WorkspaceID    uuid.UUID `json:"workspace_id"`
	ConversationID uuid.UUID `json:"conversation_id"`
	Target         string    `json:"target"`
	DueAt          time.Time `json:"due_at"`
	At             time.Time `json:"at"`
	Breached       bool      `json:"breached,omitempty"`
}

func (SlackSLAArgs) Kind() string { return "slack_sla_post" }

type slackSLAWorker struct {
	river.WorkerDefaults[SlackSLAArgs]
	s *Server
}

func (w *slackSLAWorker) Work(ctx context.Context, job *river.Job[SlackSLAArgs]) error {
	wait, err := w.s.PostSLAToSlack(ctx, job.Args)
	if err != nil {
		return err
	}
	if wait > 0 {
		return river.JobSnooze(wait)
	}
	return nil
}

func (w *slackSLAWorker) Timeout(*river.Job[SlackSLAArgs]) time.Duration { return 2 * time.Minute }

func slackSLAWanted(set store.SlackInboxSetting, breached bool) bool {
	return deref(set.SlaChannel) != "" && (breached || set.SlaWarnings)
}

func (s *Server) queueSlackSLA(ctx context.Context, q *store.Queries, ev *eventBatch, c store.Conversation, a SlackSLAArgs) error {
	if !s.slack.on() {
		return nil
	}
	set, err := q.GetSlackInboxSettings(ctx, store.GetSlackInboxSettingsParams{WorkspaceID: c.WorkspaceID, InboxID: c.InboxID})
	if store.IsNotFound(err) {
		return nil
	}
	if err != nil || !slackSLAWanted(set, a.Breached) {
		return err
	}
	ev.job(a, &river.InsertOpts{MaxAttempts: slackPostAttempts, UniqueOpts: river.UniqueOpts{ByArgs: true}})
	return nil
}

func slackDuration(d time.Duration) string {
	m := max(int(d.Round(time.Minute)/time.Minute), 1)
	switch {
	case m >= 1440:
		return fmt.Sprintf("%dd %dh", m/1440, m%1440/60)
	case m >= 60:
		return fmt.Sprintf("%dh %dm", m/60, m%60)
	}
	return fmt.Sprintf("%dm", m)
}

var slackSLATargets = map[string]string{"first_reply": "first reply", "next_reply": "next reply", "resolution": "resolution"}

// PostSLAToSlack makes one attempt at an SLA post, in the conversation's thread in that channel
// when it has one.
func (s *Server) PostSLAToSlack(ctx context.Context, a SlackSLAArgs) (time.Duration, error) {
	ws := a.WorkspaceID
	if live, err := s.workspaceLive(ctx, ws); err != nil || !live || !s.slack.on() {
		return 0, err
	}
	inst, err := s.st.GetSlackInstallation(ctx, ws)
	if store.IsNotFound(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	c, err := s.st.GetConversation(ctx, store.GetConversationParams{WorkspaceID: ws, ID: a.ConversationID})
	if store.IsNotFound(err) || (err == nil && (c.Spam || c.TrashedAt != nil)) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	set, err := s.st.GetSlackInboxSettings(ctx, store.GetSlackInboxSettingsParams{WorkspaceID: ws, InboxID: c.InboxID})
	if store.IsNotFound(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if !slackSLAWanted(set, a.Breached) {
		return 0, nil
	}
	channel := deref(set.SlaChannel)
	text, err := s.slackSLAText(ctx, inst, set, c, a)
	if err != nil {
		return 0, err
	}
	msg := slack.Message{Channel: channel, Text: text}
	t, err := s.st.GetSlackThreadOfConversation(ctx, store.GetSlackThreadOfConversationParams{WorkspaceID: ws, ConversationID: c.ID, ChannelID: channel})
	switch {
	case err == nil:
		msg.ThreadTS = t.ThreadTs
	case !store.IsNotFound(err):
		return 0, err
	}
	return s.sendSlackPost(ctx, inst, c.ID, msg)
}

func (s *Server) slackSLAText(ctx context.Context, inst store.SlackInstallation, set store.SlackInboxSetting, c store.Conversation, a SlackSLAArgs) (string, error) {
	ws := c.WorkspaceID
	in, err := s.st.GetInbox(ctx, store.GetInboxParams{WorkspaceID: ws, ID: c.InboxID})
	if err != nil {
		return "", err
	}
	contact, err := s.contactDisplayName(ctx, ws, c.ContactID)
	if err != nil {
		return "", err
	}
	owner := "Unassigned"
	switch {
	case c.AssigneeID != nil:
		name, err := s.memberDisplayName(ctx, ws, c.AssigneeID)
		if err != nil {
			return "", err
		}
		owner = "Assigned to " + slack.Escape(cmp.Or(name, "a member"))
		link, err := s.st.GetSlackMemberLink(ctx, store.GetSlackMemberLinkParams{WorkspaceID: ws, MemberID: *c.AssigneeID})
		if err == nil {
			owner = "Assigned to <@" + link.SlackUserID + ">"
		} else if !store.IsNotFound(err) {
			return "", err
		}
	case c.TeamID != nil:
		team, err := s.st.GetTeam(ctx, store.GetTeamParams{WorkspaceID: ws, ID: *c.TeamID})
		if err != nil && !store.IsNotFound(err) {
			return "", err
		}
		owner = "Team " + slack.Escape(cmp.Or(team.Name, "(removed)"))
	}
	subject := cmp.Or(strings.TrimSpace(c.Subject), "(no subject)")
	target := cmp.Or(slackSLATargets[a.Target], a.Target)
	var b strings.Builder
	if a.Breached {
		fmt.Fprintf(&b, "*SLA breached*: %s, %s late", target, slackDuration(a.At.Sub(a.DueAt)))
	} else {
		fmt.Fprintf(&b, "*SLA warning*: %s due in %s", target, slackDuration(a.DueAt.Sub(a.At)))
	}
	fmt.Fprintf(&b, "\n*%s*\n%s · %s · %s", slack.Escape(subject), slack.Escape(cmp.Or(contact, "Unknown contact")), slack.Escape(in.Name), owner)
	if set.IncludeContent {
		rows, err := s.st.ListConversationPreviews(ctx, store.ListConversationPreviewsParams{WorkspaceID: ws, ConversationIds: []uuid.UUID{c.ID}})
		if err != nil {
			return "", err
		}
		if len(rows) > 0 && !rows[0].Redacted {
			for line := range strings.SplitSeq(excerpt(rows[0].Body, slackPreviewRunes), "\n") {
				b.WriteString("\n> " + slack.Escape(line))
			}
		}
	}
	link := strings.TrimRight(s.auth.PublicURL, "/") + conversationPath(ws, c.ID)
	fmt.Fprintf(&b, "\n<%s|Open in Yuva> · Reply in this thread to add a note; start with `%s` to answer the customer.", link, inst.ReplyCommand)
	return b.String(), nil
}
