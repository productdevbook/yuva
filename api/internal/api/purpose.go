package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	purposeSupport = string(oas.InboxPurposeSupport)
	purposeMail    = string(oas.InboxPurposeMail)

	eventConversationStar = "conversation.star"
)

func errPurposeMismatch(detail string) *apiError {
	return problem(http.StatusConflict, "purpose_mismatch", detail)
}

func supportInbox(ctx context.Context, q *store.Queries, workspaceID, inboxID uuid.UUID, what string) error {
	purpose, err := q.ShareInboxPurpose(ctx, store.ShareInboxPurposeParams{WorkspaceID: workspaceID, ID: inboxID})
	if store.IsNotFound(err) {
		return errInboxGone
	}
	if err != nil {
		return err
	}
	if purpose == purposeMail {
		return errPurposeMismatch("a mail inbox cannot have " + what)
	}
	return nil
}

func ratingSince(in store.Inbox) *time.Time {
	if in.Purpose == purposeMail {
		return nil
	}
	return in.RatingSince
}

func validPurpose(p string) error {
	if p != purposeSupport && p != purposeMail {
		return errValidation("purpose must be support or mail")
	}
	return nil
}

func mailBlockers(ctx context.Context, q *store.Queries, workspaceID, inboxID uuid.UUID) error {
	l, err := q.InboxSupportLinks(ctx, store.InboxSupportLinksParams{WorkspaceID: workspaceID, InboxID: inboxID})
	if err != nil {
		return err
	}
	var blocks []string
	for _, k := range l.ChannelKinds {
		blocks = append(blocks, "a "+k+" channel")
	}
	for _, b := range []struct {
		on   bool
		what string
	}{{l.HelpCenter, "a help center"}, {l.StatusPage, "a status page"}, {l.RoadmapBoard, "a roadmap board"}, {l.WhatsNew, "a What's new"}, {l.SlaPolicy, "an SLA policy naming it"}} {
		if b.on {
			blocks = append(blocks, b.what)
		}
	}
	if len(blocks) == 0 {
		return nil
	}
	return errPurposeMismatch("the inbox cannot become mail while it has " + strings.Join(blocks, ", "))
}

func fillPurposes(ctx context.Context, q *store.Queries, workspaceID uuid.UUID, convs ...*oas.Conversation) error {
	var ids []uuid.UUID
	for _, c := range convs {
		ids = append(ids, c.InboxId)
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := q.ListInboxPurposes(ctx, store.ListInboxPurposesParams{WorkspaceID: workspaceID, Ids: ids})
	if err != nil {
		return err
	}
	by := make(map[uuid.UUID]string, len(rows))
	for _, r := range rows {
		by[r.ID] = r.Purpose
	}
	for _, c := range convs {
		c.InboxPurpose = oas.InboxPurpose(by[c.InboxId])
		if c.InboxPurpose == "" {
			c.InboxPurpose = oas.InboxPurposeSupport
		}
	}
	return nil
}

func conversationBody(ctx context.Context, q *store.Queries, c store.Conversation, labels []uuid.UUID) (oas.Conversation, error) {
	out := conversationFields(c, labels)
	return out, fillPurposes(ctx, q, c.WorkspaceID, &out)
}

func memberStarred(ctx context.Context, q *store.Queries, p principal, ids []uuid.UUID) (map[uuid.UUID]bool, error) {
	out := map[uuid.UUID]bool{}
	if p.isKey() || len(ids) == 0 {
		return out, nil
	}
	rows, err := q.ListStarredConversations(ctx, store.ListStarredConversationsParams{WorkspaceID: p.workspaceID, MemberID: p.memberID, ConversationIds: ids})
	if err != nil {
		return nil, err
	}
	for _, id := range rows {
		out[id] = true
	}
	return out, nil
}

type conversationStar struct {
	MemberID     uuid.UUID        `json:"member_id"`
	Conversation oas.Conversation `json:"conversation"`
}

func (s *Server) setStar(ctx context.Context, conversationID uuid.UUID, on bool) error {
	p := principalFrom(ctx)
	return s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		c, err := visibleConversation(ctx, q, p, conversationID, false)
		if err != nil {
			return err
		}
		var n int64
		if on {
			n, err = q.StarConversation(ctx, store.StarConversationParams{WorkspaceID: p.workspaceID, MemberID: p.memberID, ConversationID: c.ID, Now: s.now()})
		} else {
			n, err = q.UnstarConversation(ctx, store.UnstarConversationParams{WorkspaceID: p.workspaceID, MemberID: p.memberID, ConversationID: c.ID})
		}
		if err != nil || n == 0 {
			return err
		}
		body, err := oneConversation(ctx, q, c)
		if err != nil {
			return err
		}
		if err := s.fillSLA(ctx, q, p.workspaceID, &body); err != nil {
			return err
		}
		body.Starred = &on
		events.conversation(eventConversationStar, c, conversationStar{MemberID: p.memberID, Conversation: body})
		return nil
	})
}

func (s *Server) StarConversation(ctx context.Context, req oas.StarConversationRequestObject) (oas.StarConversationResponseObject, error) {
	if err := s.setStar(ctx, req.ConversationId, true); err != nil {
		return nil, err
	}
	return oas.StarConversation204Response{}, nil
}

func (s *Server) UnstarConversation(ctx context.Context, req oas.UnstarConversationRequestObject) (oas.UnstarConversationResponseObject, error) {
	if err := s.setStar(ctx, req.ConversationId, false); err != nil {
		return nil, err
	}
	return oas.UnstarConversation204Response{}, nil
}

func starView(e realtime.Event) (realtime.Event, bool) {
	var st struct {
		Conversation json.RawMessage `json:"conversation"`
	}
	if err := json.Unmarshal(e.Data, &st); err != nil {
		return e, false
	}
	e.Type, e.Data = realtime.ConversationUpdated, []byte(st.Conversation)
	return e, true
}

func starredFor(p principal, on bool) *bool {
	if p.isKey() {
		return nil
	}
	return &on
}
