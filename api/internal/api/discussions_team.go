package api

import (
	"context"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

// teamThread loads a thread in an inbox the caller can see.
func teamThread(ctx context.Context, q *store.Queries, p principal, id uuid.UUID, lock bool) (thread, error) {
	t, err := loadThread(ctx, q, p.workspaceID, id, lock)
	if err != nil {
		return t, err
	}
	ok, err := canSeeInbox(ctx, q, p, t.d.InboxID)
	if err != nil {
		return t, err
	}
	if !ok {
		return t, errDiscussionGone
	}
	return t, nil
}

func teamPost(ctx context.Context, q *store.Queries, p principal, id uuid.UUID) (thread, store.DiscussionPostView, error) {
	post, err := q.GetDiscussionPost(ctx, store.GetDiscussionPostParams{WorkspaceID: p.workspaceID, ID: id})
	if store.IsNotFound(err) {
		return thread{}, post, errDiscussionPostGone
	}
	if err != nil {
		return thread{}, post, err
	}
	t, err := teamThread(ctx, q, p, post.DiscussionID, true)
	if err == errDiscussionGone {
		return t, post, errDiscussionPostGone
	}
	post, _ = t.post(id)
	return t, post, err
}

func memberActor(p principal) actor {
	id := p.memberID
	return actor{memberID: &id, via: p.via}
}

// requireDiscussionWriter lets members and OAuth tokens post and mark answers as their member; a
// key has no person to show as the author, and a /mcp token follows bots_may_send.
func requireDiscussionWriter(p principal) error {
	if p.isKey() {
		return errMemberSessionRequired
	}
	if !mayDeliver(p) {
		return errBotPostingDisabled
	}
	return nil
}

func (s *Server) teamDetail(ctx context.Context, p principal, id uuid.UUID) (oas.DiscussionDetail, error) {
	t, err := teamThread(ctx, s.st.Queries, p, id, false)
	if err != nil {
		return oas.DiscussionDetail{}, err
	}
	v := viewer{actor: memberActor(p), team: true}
	if err := fillLinked(ctx, s.st.Queries, p.workspaceID, &v, []store.Discussion{t.d}); err != nil {
		return oas.DiscussionDetail{}, err
	}
	out := t.detail(&v)
	if t.d.Kind == kindIdea {
		if err := fillIdeaIssues(ctx, s.st.Queries, p.workspaceID, &out.Discussion); err != nil {
			return out, err
		}
	}
	return out, nil
}

func (s *Server) teamDiscussion(ctx context.Context, p principal, id uuid.UUID) (oas.Discussion, error) {
	d, err := s.teamDetail(ctx, p, id)
	return d.Discussion, err
}

func (s *Server) ListDiscussions(ctx context.Context, req oas.ListDiscussionsRequestObject) (oas.ListDiscussionsResponseObject, error) {
	p := principalFrom(ctx)
	prm := req.Params
	lim, err := pageSize(prm.Limit)
	if err != nil {
		return nil, err
	}
	sort := oas.DiscussionTeamSortActivity
	if prm.Sort != nil {
		if !prm.Sort.Valid() {
			return nil, errValidation("sort must be activity, new, top or votes")
		}
		sort = *prm.Sort
	}
	kind := kindPage
	if prm.Kind != nil {
		if !prm.Kind.Valid() {
			return nil, errValidation("kind must be page or idea")
		}
		kind = string(*prm.Kind)
	}
	off, err := decodePosition(string(sort), prm.Cursor)
	if err != nil {
		return nil, err
	}
	arg := store.ListTeamDiscussionsParams{
		WorkspaceID: p.workspaceID, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(), InboxID: prm.InboxId, Kind: kind,
		Answered: prm.Answered, Locked: prm.Locked, Pending: prm.Pending, Sort: string(sort), Lim: lim + 1, Off: off,
	}
	if kind == kindIdea {
		if err := s.roadmapOn(); err != nil {
			return nil, err
		}
		if prm.Site != nil || prm.Page != nil {
			return nil, errValidation("site and page are only for page threads")
		}
		if prm.BoardId != nil {
			b, err := visibleBoard(ctx, s.st.Queries, p, *prm.BoardId, false)
			if err != nil {
				return nil, err
			}
			arg.BoardID = &b.ID
		}
		if prm.Stage != nil {
			if !prm.Stage.Valid() {
				return nil, errValidation("stage must be ideas, planned, in_progress, shipped or closed")
			}
			arg.Stage = new(string(*prm.Stage))
		}
		if arg.Tsq, err = ideaTsQuery(prm.Q); err != nil {
			return nil, err
		}
	} else if prm.BoardId != nil || prm.Stage != nil || prm.Q != nil {
		return nil, errValidation("board_id, stage and q are only for kind=idea")
	}
	if prm.InboxId != nil {
		if _, err := visibleInbox(ctx, s.st.Queries, p, *prm.InboxId); err != nil {
			return nil, err
		}
	}
	if prm.State != nil {
		if !prm.State.Valid() {
			return nil, errValidation("state must be published, pending or hidden")
		}
		st := string(*prm.State)
		arg.State = &st
	}
	if arg.Site, err = docsSite(prm.Site); err != nil {
		return nil, err
	}
	if prm.Page != nil {
		page, _, err := pageURL(*prm.Page)
		if err != nil {
			return nil, err
		}
		arg.Page = &page
	}
	var rows []store.Discussion
	if err := s.searchTx(ctx, arg.Tsq != nil, func(q *store.Queries) (err error) {
		rows, err = q.ListTeamDiscussions(ctx, arg)
		return err
	}); err != nil {
		return nil, err
	}
	var next *string
	if len(rows) > int(lim) {
		rows = rows[:lim]
		c := encodePosition(string(sort), off+lim)
		next = &c
	}
	items, err := discussionsBody(ctx, s.st.Queries, p.workspaceID, rows, &viewer{team: true})
	if err != nil {
		return nil, err
	}
	return oas.ListDiscussions200JSONResponse{Items: items, NextCursor: next}, nil
}

func (s *Server) CreateDiscussion(ctx context.Context, req oas.CreateDiscussionRequestObject) (oas.CreateDiscussionResponseObject, error) {
	p := principalFrom(ctx)
	b := req.Body
	title, err := discussionTitle(b.Title)
	if err != nil {
		return nil, err
	}
	body, err := discussionBodyText(b.Body)
	if err != nil {
		return nil, err
	}
	if b.Kind != nil && !b.Kind.Valid() {
		return nil, errValidation("kind must be page or idea")
	}
	if b.Kind != nil && *b.Kind == oas.DiscussionKindIdea {
		return s.createTeamIdea(ctx, p, b, title, body)
	}
	if b.BoardId != nil {
		return nil, errValidation("board_id is only for kind=idea")
	}
	if b.ChannelId == nil || b.PageUrl == nil {
		return nil, errValidation("channel_id and page_url are required")
	}
	pageTitle, err := pageTitle(b.PageTitle)
	if err != nil {
		return nil, err
	}
	ch, err := s.st.GetSessionChannel(ctx, store.GetSessionChannelParams{WorkspaceID: p.workspaceID, ChannelID: *b.ChannelId})
	if store.IsNotFound(err) || (err == nil && ch.Kind != string(oas.ChannelKindChat)) {
		return nil, errChannelGone
	}
	if err != nil {
		return nil, err
	}
	c, err := s.st.GetChannel(ctx, store.GetChannelParams{WorkspaceID: p.workspaceID, ID: *b.ChannelId})
	if err != nil {
		return nil, err
	}
	if _, err := visibleInbox(ctx, s.st.Queries, p, c.InboxID); err != nil {
		return nil, errChannelGone
	}
	if ch.Discussions == string(oas.DiscussionModeOff) {
		return nil, errDiscussionsOff
	}
	page, err := channelPage(*b.PageUrl, ch.Kind, ch.AllowedOrigins)
	if err != nil {
		return nil, err
	}
	var id uuid.UUID
	err = s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		n := newThread{ws: p.workspaceID, inbox: c.InboxID, channel: c.ID, page: page, title: title, body: body, author: memberActor(p), state: statePublished}
		if pageTitle != nil {
			n.pageTitle = *pageTitle
		}
		d, err := s.createThread(ctx, q, events, n)
		id = d.ID
		return err
	})
	if err != nil {
		return nil, err
	}
	out, err := s.teamDiscussion(ctx, p, id)
	if err != nil {
		return nil, err
	}
	return oas.CreateDiscussion201JSONResponse(out), nil
}

func (s *Server) createTeamIdea(ctx context.Context, p principal, b *oas.DiscussionCreate, title, body string) (oas.CreateDiscussionResponseObject, error) {
	if err := s.roadmapOn(); err != nil {
		return nil, err
	}
	if b.ChannelId != nil || b.PageUrl != nil || b.PageTitle != nil {
		return nil, errIdeaPageNotAllowed
	}
	if b.BoardId == nil {
		return nil, errValidation("board_id is required for an idea")
	}
	var id uuid.UUID
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		board, err := visibleBoard(ctx, q, p, *b.BoardId, false)
		if err != nil {
			return err
		}
		if err := s.ideaLimit(ctx, q, p.workspaceID); err != nil {
			return err
		}
		d, err := s.createThread(ctx, q, events, newThread{
			ws: p.workspaceID, inbox: board.InboxID, channel: board.ChannelID, board: &board.ID, title: title, body: body,
			author: memberActor(p), state: statePublished,
		})
		id = d.ID
		return err
	})
	if err != nil {
		return nil, err
	}
	out, err := s.teamDiscussion(ctx, p, id)
	if err != nil {
		return nil, err
	}
	return oas.CreateDiscussion201JSONResponse(out), nil
}

func (s *Server) GetDiscussion(ctx context.Context, req oas.GetDiscussionRequestObject) (oas.GetDiscussionResponseObject, error) {
	out, err := s.teamDetail(ctx, principalFrom(ctx), req.DiscussionId)
	if err != nil {
		return nil, err
	}
	return oas.GetDiscussion200JSONResponse(out), nil
}

func (s *Server) UpdateDiscussion(ctx context.Context, req oas.UpdateDiscussionRequestObject) (oas.UpdateDiscussionResponseObject, error) {
	p := principalFrom(ctx)
	b := req.Body
	if b.Title == nil && b.Locked == nil {
		return nil, errValidation("send title or locked")
	}
	var title string
	if b.Title != nil {
		var err error
		if title, err = discussionTitle(*b.Title); err != nil {
			return nil, err
		}
	}
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		t, err := teamThread(ctx, q, p, req.DiscussionId, true)
		if err != nil {
			return err
		}
		if b.Title != nil {
			if err := q.SetDiscussionTitle(ctx, store.SetDiscussionTitleParams{WorkspaceID: p.workspaceID, ID: t.d.ID, Title: title}); err != nil {
				return err
			}
		}
		if b.Locked != nil && *b.Locked != (t.d.LockedAt != nil) {
			arg := store.SetDiscussionLockedParams{WorkspaceID: p.workspaceID, ID: t.d.ID}
			if *b.Locked {
				now := s.now()
				arg.LockedAt = &now
			}
			if err := q.SetDiscussionLocked(ctx, arg); err != nil {
				return err
			}
		}
		return discussionEvent(ctx, q, events, eventDiscussionUpdated, p.workspaceID, t.d.ID, nil)
	})
	if err != nil {
		return nil, err
	}
	out, err := s.teamDiscussion(ctx, p, req.DiscussionId)
	if err != nil {
		return nil, err
	}
	return oas.UpdateDiscussion200JSONResponse(out), nil
}

func (s *Server) CreateDiscussionPost(ctx context.Context, req oas.CreateDiscussionPostRequestObject) (oas.CreateDiscussionPostResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireDiscussionWriter(p); err != nil {
		return nil, err
	}
	body, err := discussionBodyText(req.Body.Body)
	if err != nil {
		return nil, err
	}
	var id uuid.UUID
	err = s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		t, err := teamThread(ctx, q, p, req.DiscussionId, true)
		if err != nil {
			return err
		}
		id, err = s.addReply(ctx, q, events, t, memberActor(p), body, req.Body.ParentPostId, statePublished)
		return err
	})
	if err != nil {
		return nil, err
	}
	out, err := postView(ctx, s.st.Queries, p.workspaceID, id, &viewer{actor: memberActor(p), team: true})
	if err != nil {
		return nil, err
	}
	return oas.CreateDiscussionPost201JSONResponse(out), nil
}

func (s *Server) teamAnswer(ctx context.Context, id uuid.UUID, postID *uuid.UUID) (oas.Discussion, error) {
	p := principalFrom(ctx)
	if err := requireDiscussionWriter(p); err != nil {
		return oas.Discussion{}, err
	}
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		t, err := teamThread(ctx, q, p, id, true)
		if err != nil {
			return err
		}
		return s.setAnswer(ctx, q, events, t, postID, memberActor(p))
	})
	if err != nil {
		return oas.Discussion{}, err
	}
	return s.teamDiscussion(ctx, p, id)
}

func (s *Server) AcceptDiscussionAnswer(ctx context.Context, req oas.AcceptDiscussionAnswerRequestObject) (oas.AcceptDiscussionAnswerResponseObject, error) {
	out, err := s.teamAnswer(ctx, req.DiscussionId, &req.Body.PostId)
	if err != nil {
		return nil, err
	}
	return oas.AcceptDiscussionAnswer200JSONResponse(out), nil
}

func (s *Server) ClearDiscussionAnswer(ctx context.Context, req oas.ClearDiscussionAnswerRequestObject) (oas.ClearDiscussionAnswerResponseObject, error) {
	out, err := s.teamAnswer(ctx, req.DiscussionId, nil)
	if err != nil {
		return nil, err
	}
	return oas.ClearDiscussionAnswer200JSONResponse(out), nil
}

func (s *Server) ListDiscussionPosts(ctx context.Context, req oas.ListDiscussionPostsRequestObject) (oas.ListDiscussionPostsResponseObject, error) {
	p := principalFrom(ctx)
	prm := req.Params
	lim, err := pageSize(prm.Limit)
	if err != nil {
		return nil, err
	}
	at, cid, err := decodeCursor(prm.Cursor)
	if err != nil {
		return nil, err
	}
	state := statePending
	if prm.State != nil {
		if !prm.State.Valid() {
			return nil, errValidation("state must be published, pending or hidden")
		}
		state = string(*prm.State)
	}
	arg := store.ListTeamDiscussionPostsParams{
		WorkspaceID: p.workspaceID, State: state, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(), InboxID: prm.InboxId,
		CursorAt: at, CursorID: cid, Lim: lim + 1,
	}
	if prm.InboxId != nil {
		if _, err := visibleInbox(ctx, s.st.Queries, p, *prm.InboxId); err != nil {
			return nil, err
		}
	}
	if arg.Site, err = docsSite(prm.Site); err != nil {
		return nil, err
	}
	if prm.Page != nil {
		page, _, err := pageURL(*prm.Page)
		if err != nil {
			return nil, err
		}
		arg.Page = &page
	}
	rows, err := s.st.ListTeamDiscussionPosts(ctx, arg)
	if err != nil {
		return nil, err
	}
	var next *string
	if len(rows) > int(lim) {
		rows = rows[:lim]
		c := encodeCursor(rows[lim-1].CreatedAt, rows[lim-1].ID)
		next = &c
	}
	var ids []uuid.UUID
	for _, r := range rows {
		ids = append(ids, r.DiscussionID)
	}
	ds, err := s.st.ListDiscussionsByIDs(ctx, store.ListDiscussionsByIDsParams{WorkspaceID: p.workspaceID, Ids: uniqueIDs(ids)})
	if err != nil {
		return nil, err
	}
	bodies, err := discussionsBody(ctx, s.st.Queries, p.workspaceID, ds, &teamViewer)
	if err != nil {
		return nil, err
	}
	byID := map[uuid.UUID]store.Discussion{}
	bodyByID := map[uuid.UUID]oas.Discussion{}
	for i, d := range ds {
		byID[d.ID], bodyByID[d.ID] = d, bodies[i]
	}
	items := make([]oas.DiscussionPostItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, oas.DiscussionPostItem{Post: postBody(r, byID[r.DiscussionID].AnswerPostID, &teamViewer), Discussion: bodyByID[r.DiscussionID]})
	}
	return oas.ListDiscussionPosts200JSONResponse{Items: items, NextCursor: next}, nil
}

func (s *Server) UpdateDiscussionPost(ctx context.Context, req oas.UpdateDiscussionPostRequestObject) (oas.UpdateDiscussionPostResponseObject, error) {
	p := principalFrom(ctx)
	b := req.Body
	if b.State == nil && b.Body == nil && b.Title == nil {
		return nil, errValidation("send state, body or title")
	}
	if b.State != nil && !b.State.Valid() {
		return nil, errValidation("state must be published or hidden")
	}
	if b.State != nil {
		if err := requireManagerOrKey(p); err != nil {
			return nil, err
		}
	}
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		t, post, err := teamPost(ctx, q, p, req.PostId)
		if err != nil {
			return err
		}
		if b.Body != nil || b.Title != nil {
			if !memberActor(p).owns(post) {
				return errNotPostAuthor
			}
			if err := s.editPost(ctx, q, events, t, post, b.Body, b.Title); err != nil {
				return err
			}
		}
		if b.State != nil {
			if post.DeletedAt != nil {
				return errDiscussionPostGone
			}
			if t, err = loadThread(ctx, q, p.workspaceID, t.d.ID, false); err != nil {
				return err
			}
			post, _ = t.post(req.PostId)
			return s.moderate(ctx, q, events, t, post, string(*b.State))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out, err := postView(ctx, s.st.Queries, p.workspaceID, req.PostId, &teamViewer)
	if err != nil {
		return nil, err
	}
	return oas.UpdateDiscussionPost200JSONResponse(out), nil
}

func (s *Server) DeleteDiscussionPost(ctx context.Context, req oas.DeleteDiscussionPostRequestObject) (oas.DeleteDiscussionPostResponseObject, error) {
	p := principalFrom(ctx)
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		t, post, err := teamPost(ctx, q, p, req.PostId)
		if err != nil {
			return err
		}
		if post.DeletedAt != nil {
			return errDiscussionPostGone
		}
		if !memberActor(p).owns(post) {
			return errNotPostAuthor
		}
		return s.deletePost(ctx, q, events, t, post)
	})
	if err != nil {
		return nil, err
	}
	return oas.DeleteDiscussionPost204Response{}, nil
}

func (s *Server) MarkDiscussionPostSpam(ctx context.Context, req oas.MarkDiscussionPostSpamRequestObject) (oas.MarkDiscussionPostSpamResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		t, post, err := teamPost(ctx, q, p, req.PostId)
		if err != nil {
			return err
		}
		return s.markSpam(ctx, q, events, t, post)
	})
	if err != nil {
		return nil, err
	}
	out, err := postView(ctx, s.st.Queries, p.workspaceID, req.PostId, &teamViewer)
	if err != nil {
		return nil, err
	}
	return oas.MarkDiscussionPostSpam200JSONResponse(out), nil
}

func (s *Server) teamVote(ctx context.Context, id uuid.UUID, add bool) (oas.DiscussionPost, error) {
	p := principalFrom(ctx)
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		t, post, err := teamPost(ctx, q, p, id)
		if err != nil {
			return err
		}
		if post.State != statePublished || post.DeletedAt != nil {
			return errDiscussionPostGone
		}
		if add && ideaVotesClosed(t.d) {
			return errIdeaClosed
		}
		return vote(ctx, q, p.workspaceID, post, memberActor(p), add, s.now())
	})
	if err != nil {
		return oas.DiscussionPost{}, err
	}
	return postView(ctx, s.st.Queries, p.workspaceID, id, &teamViewer)
}

func (s *Server) VoteDiscussionPost(ctx context.Context, req oas.VoteDiscussionPostRequestObject) (oas.VoteDiscussionPostResponseObject, error) {
	out, err := s.teamVote(ctx, req.PostId, true)
	if err != nil {
		return nil, err
	}
	return oas.VoteDiscussionPost200JSONResponse(out), nil
}

func (s *Server) UnvoteDiscussionPost(ctx context.Context, req oas.UnvoteDiscussionPostRequestObject) (oas.UnvoteDiscussionPostResponseObject, error) {
	out, err := s.teamVote(ctx, req.PostId, false)
	if err != nil {
		return nil, err
	}
	return oas.UnvoteDiscussionPost200JSONResponse(out), nil
}

// createIdeaFromConversation starts a published idea on the inbox's board with the member's text and
// links the feedback to it; the feedback stays private.
func (s *Server) createIdeaFromConversation(ctx context.Context, q *store.Queries, events *eventBatch, p principal, c store.Conversation, title, body string) (uuid.UUID, error) {
	if c.Kind != string(oas.ConversationKindFeedback) {
		return uuid.Nil(), errNotFeedbackLink
	}
	board, err := q.GetInboxRoadmapBoard(ctx, store.GetInboxRoadmapBoardParams{WorkspaceID: p.workspaceID, InboxID: c.InboxID})
	if store.IsNotFound(err) {
		return uuid.Nil(), errInboxNoBoard
	}
	if err != nil {
		return uuid.Nil(), err
	}
	if err := s.ideaLimit(ctx, q, p.workspaceID); err != nil {
		return uuid.Nil(), err
	}
	author := memberActor(p)
	known, err := q.ContactHasPersonOrIdentity(ctx, store.ContactHasPersonOrIdentityParams{WorkspaceID: p.workspaceID, ID: c.ContactID, InboxID: c.InboxID})
	if err != nil {
		return uuid.Nil(), err
	}
	if known {
		contact := c.ContactID
		author = actor{contactID: &contact}
	}
	d, err := s.createThread(ctx, q, events, newThread{
		ws: p.workspaceID, inbox: board.InboxID, channel: board.ChannelID, board: &board.ID, title: title, body: body,
		author: author, state: statePublished,
	})
	if err != nil {
		return uuid.Nil(), err
	}
	return d.ID, s.linkIdea(ctx, q, events, p, d, c)
}

func (s *Server) CreateDiscussionFromConversation(ctx context.Context, req oas.CreateDiscussionFromConversationRequestObject) (oas.CreateDiscussionFromConversationResponseObject, error) {
	p := principalFrom(ctx)
	b := req.Body
	title, err := discussionTitle(b.Title)
	if err != nil {
		return nil, err
	}
	body, err := discussionBodyText(b.Body)
	if err != nil {
		return nil, err
	}
	if b.Kind != nil && !b.Kind.Valid() {
		return nil, errValidation("kind must be page or idea")
	}
	idea := b.Kind != nil && *b.Kind == oas.DiscussionKindIdea
	if idea {
		if err := s.roadmapOn(); err != nil {
			return nil, err
		}
		if b.Answer != nil || b.PageUrl != nil || b.PageTitle != nil {
			return nil, errValidation("answer, page_url and page_title are not allowed for an idea")
		}
	}
	var answer string
	if b.Answer != nil {
		if answer, err = discussionBodyText(*b.Answer); err != nil {
			return nil, err
		}
	}
	reqTitle, err := pageTitle(b.PageTitle)
	if err != nil {
		return nil, err
	}
	var id uuid.UUID
	err = s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		c, err := visibleConversation(ctx, q, p, req.ConversationId, true)
		if err != nil {
			return err
		}
		if idea {
			id, err = s.createIdeaFromConversation(ctx, q, events, p, c, title, body)
			return err
		}
		if _, err := q.GetDiscussionByConversation(ctx, store.GetDiscussionByConversationParams{WorkspaceID: p.workspaceID, ConversationID: &c.ID}); err == nil {
			return errAlreadyDiscussed
		} else if !store.IsNotFound(err) {
			return err
		}
		raw, title2 := c.PageUrl, c.PageTitle
		if b.PageUrl != nil {
			raw, title2 = b.PageUrl, reqTitle
		}
		if raw == nil {
			return errValidation("page_url is required: the conversation was not sent from a page")
		}
		page, origin, err := pageURL(*raw)
		if err != nil {
			return err
		}
		channel, err := q.FindDiscussionChannel(ctx, store.FindDiscussionChannelParams{WorkspaceID: p.workspaceID, InboxID: c.InboxID, Origin: origin, Prefer: c.ChannelID})
		if store.IsNotFound(err) {
			return errNoDiscussionChannel
		}
		if err != nil {
			return err
		}
		author := memberActor(p)
		known, err := q.ContactHasPersonOrIdentity(ctx, store.ContactHasPersonOrIdentityParams{WorkspaceID: p.workspaceID, ID: c.ContactID, InboxID: c.InboxID})
		if err != nil {
			return err
		}
		if known {
			contact := c.ContactID
			author = actor{contactID: &contact}
		}
		n := newThread{ws: p.workspaceID, inbox: c.InboxID, channel: channel, page: page, title: title, body: body, author: author, state: statePublished, conversation: &c.ID}
		if title2 != nil {
			n.pageTitle = *title2
		}
		d, err := s.createThread(ctx, q, events, n)
		if err != nil {
			return err
		}
		id = d.ID
		if b.Answer == nil {
			return nil
		}
		t, err := loadThread(ctx, q, p.workspaceID, id, true)
		if err != nil {
			return err
		}
		postID, err := s.addReply(ctx, q, events, t, memberActor(p), answer, nil, statePublished)
		if err != nil {
			return err
		}
		if t, err = loadThread(ctx, q, p.workspaceID, id, false); err != nil {
			return err
		}
		return s.setAnswer(ctx, q, events, t, &postID, memberActor(p))
	})
	if err != nil {
		return nil, err
	}
	out, err := s.teamDetail(ctx, p, id)
	if err != nil {
		return nil, err
	}
	return oas.CreateDiscussionFromConversation201JSONResponse(out), nil
}
