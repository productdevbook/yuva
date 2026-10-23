package api

import (
	"context"
	"encoding/base64"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	readerTokenPrefix = "yuva_rd_"
	privateNoStore    = "private, no-store"
)

var limitDiscussionPostsPerHour = limit{discussionPostsPerHour, time.Hour}

// discussant is a caller of the /client/v1 discussion endpoints: a reader token or a contact
// session of a chat channel.
type discussant struct {
	actor
	workspaceID, inboxID, channelID uuid.UUID
	kind                            string
	chat                            store.ChatChannel
	personID                        *uuid.UUID
	readerTokenID                   *uuid.UUID
	identified                      bool
	roadmapOff                      bool
}

const discussantKey ctxKey = iota + 200

func discussantFrom(ctx context.Context) *discussant {
	d, _ := ctx.Value(discussantKey).(*discussant)
	return d
}

func (s *Server) resolveDiscussant(ctx context.Context, token string) (*discussant, error) {
	if !strings.HasPrefix(token, readerTokenPrefix) {
		cp, err := s.resolveContact(ctx, token)
		if err != nil {
			return nil, err
		}
		contact := cp.contactID
		return &discussant{
			actor: actor{contactID: &contact}, workspaceID: cp.workspaceID, inboxID: cp.inboxID, channelID: cp.channelID,
			kind: cp.kind, chat: cp.chat, identified: cp.identified, roadmapOff: s.roadmap.Off,
		}, nil
	}
	now := s.now()
	tok, err := s.st.GetReaderTokenByHash(ctx, store.GetReaderTokenByHashParams{TokenHash: hashSecret(token), Now: now})
	if store.IsNotFound(err) {
		return nil, errContactUnauthenticated
	}
	if err != nil {
		return nil, err
	}
	ch, err := s.st.GetSessionChannel(ctx, store.GetSessionChannelParams{WorkspaceID: tok.WorkspaceID, ChannelID: tok.ChannelID})
	if store.IsNotFound(err) || (err == nil && ch.Kind != string(oas.ChannelKindChat)) {
		return nil, errContactUnauthenticated
	}
	if err != nil {
		return nil, err
	}
	if !s.originAllowedFor(ctx, ch.WorkspaceID, originFrom(ctx), ch.Kind, ch.AllowedOrigins) {
		return nil, errOriginRefused
	}
	if originFrom(ctx) != tok.Origin {
		return nil, errContactUnauthenticated
	}
	inbox, err := s.st.GetChannel(ctx, store.GetChannelParams{WorkspaceID: tok.WorkspaceID, ID: tok.ChannelID})
	if err != nil {
		return nil, err
	}
	person, tokenID := tok.PersonID, tok.ID
	d := &discussant{
		workspaceID: tok.WorkspaceID, inboxID: inbox.InboxID, channelID: tok.ChannelID, kind: ch.Kind, chat: sessionChat(ch),
		personID: &person, readerTokenID: &tokenID, roadmapOff: s.roadmap.Off,
	}
	m, err := s.st.GetMemberByPerson(ctx, store.GetMemberByPersonParams{WorkspaceID: tok.WorkspaceID, PersonID: person})
	if err != nil && !store.IsNotFound(err) {
		return nil, err
	}
	if err == nil {
		ok, err := memberHasInbox(ctx, s.st.Queries, tok.WorkspaceID, d.inboxID, m.ID)
		if err != nil {
			return nil, err
		}
		if ok {
			id := m.ID
			d.memberID = &id
			return d, nil
		}
	}
	c, err := s.st.FindContactByPerson(ctx, store.FindContactByPersonParams{WorkspaceID: tok.WorkspaceID, PersonID: &person})
	if err != nil && !store.IsNotFound(err) {
		return nil, err
	}
	if err == nil {
		if c.Blocked {
			return nil, errContactBlocked
		}
		id := c.ID
		d.contactID = &id
	}
	return d, nil
}

// bindWriter gives a reader the contact linked to their person in this workspace, inside the
// transaction of their first write and only once that write has passed its checks, so a refused
// write discloses nothing about the reader to the workspace.
func (s *Server) bindWriter(ctx context.Context, q *store.Queries, d *discussant) error {
	if d.memberID != nil || d.contactID != nil {
		return nil
	}
	id, err := s.readerContact(ctx, q, d.workspaceID, *d.personID)
	if err != nil {
		return err
	}
	d.contactID = &id
	return nil
}

// readerContact links the person to the workspace's contact with their address, or creates one.
func (s *Server) readerContact(ctx context.Context, q *store.Queries, ws, personID uuid.UUID) (uuid.UUID, error) {
	person, err := q.GetPerson(ctx, personID)
	if err != nil {
		return uuid.Nil(), err
	}
	now := s.now()
	linked, err := q.LinkContactByEmail(ctx, store.LinkContactByEmailParams{WorkspaceID: ws, Email: person.Email, PersonID: &personID, Now: now})
	if err == nil {
		return linked.ID, nil
	}
	if !store.IsNotFound(err) {
		return uuid.Nil(), err
	}
	c, err := q.CreateContact(ctx, store.CreateContactParams{ID: newID(), WorkspaceID: ws, Name: person.Name, Attributes: []byte("{}"), Now: now})
	if err != nil {
		return uuid.Nil(), err
	}
	if err := q.SetContactPerson(ctx, store.SetContactPersonParams{WorkspaceID: ws, ID: c.ID, PersonID: &personID}); err != nil {
		return uuid.Nil(), err
	}
	if _, err := q.GetContactIDByEmail(ctx, store.GetContactIDByEmailParams{WorkspaceID: ws, Email: person.Email}); store.IsNotFound(err) {
		if err := q.AddContactEmail(ctx, store.AddContactEmailParams{WorkspaceID: ws, ContactID: c.ID, Email: person.Email, Position: 0}); err != nil {
			return uuid.Nil(), err
		}
	} else if err != nil {
		return uuid.Nil(), err
	}
	return c.ID, q.RefreshContactSearch(ctx, store.RefreshContactSearchParams{WorkspaceID: ws, ID: c.ID})
}

func (s *Server) discussionChecks(ctx context.Context, d *discussant) []rateCheck {
	who := d.rateWho()
	return append(s.writeChecks(ctx, contactPrincipal{channelID: d.channelID}),
		rateCheck{"discussion:post:" + who, limit{discussionPostsPerMin, time.Minute}},
		rateCheck{"discussion:posth:" + who, limitDiscussionPostsPerHour})
}

func (d *discussant) rateWho() string {
	if d.contactID != nil {
		return "contact:" + d.contactID.String()
	}
	if d.memberID != nil {
		return "member:" + d.memberID.String()
	}
	return "person:" + uuidString(d.personID)
}

func uuidString(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

// clientThread loads a thread the discussant may see: in its inbox, on a page its channel serves,
// published or their own pending one.
func (d *discussant) thread(ctx context.Context, q *store.Queries, id uuid.UUID, lock bool) (thread, error) {
	t, err := loadThread(ctx, q, d.workspaceID, id, lock)
	if err != nil {
		return t, err
	}
	if t.d.InboxID != d.inboxID {
		return t, errDiscussionGone
	}
	if t.d.Kind == kindIdea {
		if _, err := publicBoardOf(ctx, q, t.d, d.roadmapOff); err != nil {
			return t, errDiscussionGone
		}
	} else if _, err := channelPage(orZero(t.d.Page), d.kind, d.chat.AllowedOrigins); err != nil {
		return t, errDiscussionGone
	}
	v := viewer{actor: d.actor}
	if !v.sees(t.opening()) {
		return t, errDiscussionGone
	}
	return t, nil
}

func (d *discussant) viewer() *viewer { return &viewer{actor: d.actor, client: true} }

func publicBoardOf(ctx context.Context, q *store.Queries, d store.Discussion, off bool) (store.RoadmapBoard, error) {
	if off || d.BoardID == nil {
		return store.RoadmapBoard{}, errRoadmapNoBoard
	}
	b, err := q.GetRoadmapBoard(ctx, store.GetRoadmapBoardParams{WorkspaceID: d.WorkspaceID, ID: *d.BoardID})
	if store.IsNotFound(err) || (err == nil && !b.Public) {
		return b, errRoadmapNoBoard
	}
	return b, err
}

// writeThread loads a thread the discussant is about to write in and the moderation mode that applies
// to the post: the channel's for a page thread, the board's for an idea (replies on a board that takes
// no suggestions still wait for a member like a moderated board's).
func (d *discussant) writeThread(ctx context.Context, q *store.Queries, id uuid.UUID) (thread, string, error) {
	t, err := d.thread(ctx, q, id, true)
	if err == errDiscussionGone {
		if off := d.checkOpen(); off != nil {
			return t, "", off
		}
	}
	if err != nil {
		return t, "", err
	}
	if t.d.Kind != kindIdea {
		return t, d.chat.Discussions, d.checkOpen()
	}
	b, err := publicBoardOf(ctx, q, t.d, d.roadmapOff)
	if err != nil {
		return t, "", err
	}
	if b.Suggestions == string(oas.RoadmapSuggestionsOff) {
		return t, string(oas.RoadmapSuggestionsModerated), nil
	}
	return t, b.Suggestions, nil
}

func (d *discussant) checkOpen() error {
	if d.kind != string(oas.ChannelKindChat) || d.chat.Discussions == string(oas.DiscussionModeOff) || d.chat.Discussions == "" {
		return errDiscussionsOff
	}
	return nil
}

func (d *discussant) mayChange(t thread) error {
	if t.d.LockedAt != nil && d.memberID == nil {
		return errDiscussionLocked
	}
	return nil
}

// positionCursor is an offset within one sort, for lists without a stable key.
func encodePosition(sort string, n int32) string {
	return base64.RawURLEncoding.EncodeToString([]byte(sort + "." + strconv.Itoa(int(n))))
}

func decodePosition(sort string, s *string) (int32, error) {
	if s == nil || *s == "" {
		return 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(*s)
	if err != nil {
		return 0, errInvalidCursor
	}
	got, num, ok := strings.Cut(string(raw), ".")
	if !ok || got != sort {
		return 0, errInvalidCursor
	}
	n, err := strconv.ParseInt(num, 10, 32)
	if err != nil || n < 0 {
		return 0, errInvalidCursor
	}
	return int32(n), nil
}

// readerOf resolves the optional token on the public read endpoints; it must belong to the channel.
func (s *Server) readerOf(ctx context.Context, ch store.FindChatChannelByKeyRow) (*viewer, string, error) {
	token, ok := bearerToken(requestFrom(ctx))
	if !ok {
		return &viewer{client: true}, publicMaxAge, nil
	}
	d, err := s.resolveDiscussant(ctx, token)
	if err != nil {
		return nil, "", err
	}
	if d.channelID != ch.ChannelID || d.workspaceID != ch.WorkspaceID {
		return nil, "", errContactUnauthenticated
	}
	return d.viewer(), privateNoStore, nil
}

func (s *Server) discussionChannel(ctx context.Context, key string) (store.FindChatChannelByKeyRow, error) {
	if err := s.rateLimit(ctx, rateCheck{"discussions:ip:" + rateIP(s.clientIP(requestFrom(ctx))), limitChannelPerIP}); err != nil {
		return store.FindChatChannelByKeyRow{}, err
	}
	return s.docsChannel(ctx, key, nil)
}

func pageDiscussionsOn(ch store.FindChatChannelByKeyRow) error {
	if ch.Discussions == string(oas.DiscussionModeOff) {
		return errDiscussionsOff
	}
	return nil
}

func (s *Server) ListClientDiscussions(ctx context.Context, req oas.ListClientDiscussionsRequestObject) (oas.ListClientDiscussionsResponseObject, error) {
	ch, err := s.discussionChannel(ctx, req.ChannelKey)
	if err != nil {
		return nil, err
	}
	prm := req.Params
	if prm.Kind != nil {
		if !prm.Kind.Valid() {
			return nil, errValidation("kind must be page or idea")
		}
		if *prm.Kind == oas.DiscussionKindIdea {
			return s.listClientIdeas(ctx, ch, prm)
		}
	}
	if err := pageDiscussionsOn(ch); err != nil {
		return nil, err
	}
	if prm.Stage != nil || prm.Q != nil {
		return nil, errValidation("stage and q are only for kind=idea")
	}
	if prm.Page == nil {
		return nil, errValidation("page is required")
	}
	page, err := channelPage(*prm.Page, ch.Kind, ch.AllowedOrigins)
	if err != nil {
		return nil, err
	}
	sort := oas.DiscussionSortNew
	if prm.Sort != nil {
		if !prm.Sort.Valid() {
			return nil, errValidation("sort must be new, top or unanswered")
		}
		sort = *prm.Sort
	}
	lim, err := pageSize(prm.Limit)
	if err != nil {
		return nil, err
	}
	off, err := decodePosition(string(sort), prm.Cursor)
	if err != nil {
		return nil, err
	}
	v, cache, err := s.readerOf(ctx, ch)
	if err != nil {
		return nil, err
	}
	rows, err := s.st.ListPageDiscussions(ctx, store.ListPageDiscussionsParams{
		WorkspaceID: ch.WorkspaceID, InboxID: ch.InboxID, Page: &page, ViewerContact: v.contactID, ViewerMember: v.memberID,
		Sort: string(sort), Lim: lim + 1, Off: off,
	})
	if err != nil {
		return nil, err
	}
	var next *string
	if len(rows) > int(lim) {
		rows = rows[:lim]
		c := encodePosition(string(sort), off+lim)
		next = &c
	}
	items, err := discussionsBody(ctx, s.st.Queries, ch.WorkspaceID, rows, v)
	if err != nil {
		return nil, err
	}
	return oas.ListClientDiscussions200JSONResponse{
		Body:    oas.DiscussionPage{Items: items, NextCursor: next},
		Headers: oas.ListClientDiscussions200ResponseHeaders{CacheControl: &cache},
	}, nil
}

func (s *Server) GetClientDiscussion(ctx context.Context, req oas.GetClientDiscussionRequestObject) (oas.GetClientDiscussionResponseObject, error) {
	ch, err := s.discussionChannel(ctx, req.ChannelKey)
	if err != nil {
		return nil, err
	}
	v, cache, err := s.readerOf(ctx, ch)
	if err != nil {
		return nil, err
	}
	t, err := loadThread(ctx, s.st.Queries, ch.WorkspaceID, req.DiscussionId, false)
	if err == errDiscussionGone {
		if off := pageDiscussionsOn(ch); off != nil {
			return nil, off
		}
	}
	if err != nil {
		return nil, err
	}
	if t.d.InboxID != ch.InboxID || !v.sees(t.opening()) {
		return nil, errDiscussionGone
	}
	if t.d.Kind == kindIdea {
		if _, err := publicBoardOf(ctx, s.st.Queries, t.d, s.roadmap.Off); err != nil {
			return nil, err
		}
	} else {
		if err := pageDiscussionsOn(ch); err != nil {
			return nil, err
		}
		if _, err := channelPage(orZero(t.d.Page), ch.Kind, ch.AllowedOrigins); err != nil {
			return nil, errDiscussionGone
		}
	}
	if err := fillViewer(ctx, s.st.Queries, ch.WorkspaceID, v, []uuid.UUID{t.d.ID}); err != nil {
		return nil, err
	}
	return oas.GetClientDiscussion200JSONResponse{
		Body:    t.detail(v),
		Headers: oas.GetClientDiscussion200ResponseHeaders{CacheControl: &cache},
	}, nil
}

func (s *Server) CreateClientDiscussion(ctx context.Context, req oas.CreateClientDiscussionRequestObject) (oas.CreateClientDiscussionResponseObject, error) {
	d := discussantFrom(ctx)
	b := req.Body
	if b.Kind != nil && !b.Kind.Valid() {
		return nil, errValidation("kind must be page or idea")
	}
	if b.Kind != nil && *b.Kind == oas.DiscussionKindIdea {
		return s.createClientIdea(ctx, d, b)
	}
	if err := d.checkOpen(); err != nil {
		return nil, err
	}
	if err := s.rateLimit(ctx, s.discussionChecks(ctx, d)...); err != nil {
		return nil, err
	}
	if b.PageUrl == nil {
		return nil, errValidation("page_url is required")
	}
	page, err := channelPage(*b.PageUrl, d.kind, d.chat.AllowedOrigins)
	if err != nil {
		return nil, err
	}
	title, err := discussionTitle(b.Title)
	if err != nil {
		return nil, err
	}
	body, err := discussionBodyText(b.Body)
	if err != nil {
		return nil, err
	}
	pageTitle, err := pageTitle(b.PageTitle)
	if err != nil {
		return nil, err
	}
	var id uuid.UUID
	err = s.inTx(ctx, d.workspaceID, func(q *store.Queries, events *eventBatch) error {
		if err := s.bindWriter(ctx, q, d); err != nil {
			return err
		}
		state, err := newPostState(ctx, q, d.workspaceID, d.inboxID, d.chat.Discussions, d.actor, d.identified)
		if err != nil {
			return err
		}
		n := newThread{ws: d.workspaceID, inbox: d.inboxID, channel: d.channelID, page: page, title: title, body: body, author: d.actor, state: state}
		if pageTitle != nil {
			n.pageTitle = *pageTitle
		}
		t, err := s.createThread(ctx, q, events, n)
		id = t.ID
		return err
	})
	if err != nil {
		return nil, err
	}
	out, err := threadView(ctx, s.st.Queries, d.workspaceID, id, d.viewer())
	if err != nil {
		return nil, err
	}
	return oas.CreateClientDiscussion201JSONResponse(out), nil
}

func (s *Server) CreateClientDiscussionPost(ctx context.Context, req oas.CreateClientDiscussionPostRequestObject) (oas.CreateClientDiscussionPostResponseObject, error) {
	d := discussantFrom(ctx)
	if err := s.rateLimit(ctx, s.discussionChecks(ctx, d)...); err != nil {
		return nil, err
	}
	body, err := discussionBodyText(req.Body.Body)
	if err != nil {
		return nil, err
	}
	var id uuid.UUID
	err = s.inTx(ctx, d.workspaceID, func(q *store.Queries, events *eventBatch) error {
		t, mode, err := d.writeThread(ctx, q, req.DiscussionId)
		if err != nil {
			return err
		}
		if err := d.mayChange(t); err != nil {
			return err
		}
		if err := s.bindWriter(ctx, q, d); err != nil {
			return err
		}
		state, err := newPostState(ctx, q, d.workspaceID, d.inboxID, mode, d.actor, d.identified)
		if err != nil {
			return err
		}
		id, err = s.addReply(ctx, q, events, t, d.actor, body, req.Body.ParentPostId, state)
		return err
	})
	if err != nil {
		return nil, err
	}
	out, err := postView(ctx, s.st.Queries, d.workspaceID, id, d.viewer())
	if err != nil {
		return nil, err
	}
	return oas.CreateClientDiscussionPost201JSONResponse(out), nil
}

func (s *Server) clientAnswer(ctx context.Context, id uuid.UUID, postID *uuid.UUID) (oas.Discussion, error) {
	d := discussantFrom(ctx)
	err := s.inTx(ctx, d.workspaceID, func(q *store.Queries, events *eventBatch) error {
		t, err := d.thread(ctx, q, id, true)
		if err != nil {
			return err
		}
		if t.d.Kind == kindIdea {
			return errNotSupported
		}
		if err := d.mayChange(t); err != nil {
			return err
		}
		if d.memberID == nil && !d.owns(t.opening()) {
			return errNotPostAuthor
		}
		return s.setAnswer(ctx, q, events, t, postID, d.actor)
	})
	if err != nil {
		return oas.Discussion{}, err
	}
	return threadView(ctx, s.st.Queries, d.workspaceID, id, d.viewer())
}

func (s *Server) AcceptClientDiscussionAnswer(ctx context.Context, req oas.AcceptClientDiscussionAnswerRequestObject) (oas.AcceptClientDiscussionAnswerResponseObject, error) {
	out, err := s.clientAnswer(ctx, req.DiscussionId, &req.Body.PostId)
	if err != nil {
		return nil, err
	}
	return oas.AcceptClientDiscussionAnswer200JSONResponse(out), nil
}

func (s *Server) ClearClientDiscussionAnswer(ctx context.Context, req oas.ClearClientDiscussionAnswerRequestObject) (oas.ClearClientDiscussionAnswerResponseObject, error) {
	out, err := s.clientAnswer(ctx, req.DiscussionId, nil)
	if err != nil {
		return nil, err
	}
	return oas.ClearClientDiscussionAnswer200JSONResponse(out), nil
}

func (s *Server) clientFollow(ctx context.Context, id uuid.UUID, follow bool) error {
	d := discussantFrom(ctx)
	if err := s.rateLimit(ctx, s.writeChecks(ctx, contactPrincipal{channelID: d.channelID})...); err != nil {
		return err
	}
	return s.inTx(ctx, d.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		t, err := d.thread(ctx, q, id, false)
		if err != nil || d.memberID != nil || (!follow && d.contactID == nil) {
			return err
		}
		if !follow {
			_, err = q.UnfollowDiscussion(ctx, store.UnfollowDiscussionParams{WorkspaceID: d.workspaceID, DiscussionID: t.d.ID, ContactID: *d.contactID})
			return err
		}
		if err := s.bindWriter(ctx, q, d); err != nil {
			return err
		}
		return q.FollowDiscussion(ctx, store.FollowDiscussionParams{WorkspaceID: d.workspaceID, DiscussionID: t.d.ID, ContactID: *d.contactID, Now: s.now()})
	})
}

func (s *Server) FollowClientDiscussion(ctx context.Context, req oas.FollowClientDiscussionRequestObject) (oas.FollowClientDiscussionResponseObject, error) {
	if err := s.clientFollow(ctx, req.DiscussionId, true); err != nil {
		return nil, err
	}
	return oas.FollowClientDiscussion204Response{}, nil
}

func (s *Server) UnfollowClientDiscussion(ctx context.Context, req oas.UnfollowClientDiscussionRequestObject) (oas.UnfollowClientDiscussionResponseObject, error) {
	if err := s.clientFollow(ctx, req.DiscussionId, false); err != nil {
		return nil, err
	}
	return oas.UnfollowClientDiscussion204Response{}, nil
}

// clientPost loads a post the discussant may see, with its thread, for a change.
func (d *discussant) post(ctx context.Context, q *store.Queries, id uuid.UUID) (thread, store.DiscussionPostView, error) {
	p, err := q.GetDiscussionPost(ctx, store.GetDiscussionPostParams{WorkspaceID: d.workspaceID, ID: id})
	if store.IsNotFound(err) {
		return thread{}, p, errDiscussionPostGone
	}
	if err != nil {
		return thread{}, p, err
	}
	t, err := d.thread(ctx, q, p.DiscussionID, true)
	if err == errDiscussionGone {
		return t, p, errDiscussionPostGone
	}
	if err != nil {
		return t, p, err
	}
	if p, _ = t.post(id); !(&viewer{actor: d.actor}).sees(p) {
		return t, p, errDiscussionPostGone
	}
	return t, p, nil
}

func (s *Server) UpdateClientDiscussionPost(ctx context.Context, req oas.UpdateClientDiscussionPostRequestObject) (oas.UpdateClientDiscussionPostResponseObject, error) {
	d := discussantFrom(ctx)
	if req.Body.Body == nil && req.Body.Title == nil {
		return nil, errValidation("send body or title")
	}
	err := s.inTx(ctx, d.workspaceID, func(q *store.Queries, events *eventBatch) error {
		t, p, err := d.post(ctx, q, req.PostId)
		if err != nil {
			return err
		}
		if !d.owns(p) {
			return errNotPostAuthor
		}
		if err := d.mayChange(t); err != nil {
			return err
		}
		if d.memberID == nil && s.now().Sub(p.CreatedAt) > discussionEditWindow {
			return errEditWindowClosed
		}
		return s.editPost(ctx, q, events, t, p, req.Body.Body, req.Body.Title)
	})
	if err != nil {
		return nil, err
	}
	out, err := postView(ctx, s.st.Queries, d.workspaceID, req.PostId, d.viewer())
	if err != nil {
		return nil, err
	}
	return oas.UpdateClientDiscussionPost200JSONResponse(out), nil
}

func (s *Server) DeleteClientDiscussionPost(ctx context.Context, req oas.DeleteClientDiscussionPostRequestObject) (oas.DeleteClientDiscussionPostResponseObject, error) {
	d := discussantFrom(ctx)
	err := s.inTx(ctx, d.workspaceID, func(q *store.Queries, events *eventBatch) error {
		t, p, err := d.post(ctx, q, req.PostId)
		if err != nil {
			return err
		}
		if p.DeletedAt != nil {
			return errDiscussionPostGone
		}
		if !d.owns(p) {
			return errNotPostAuthor
		}
		return s.deletePost(ctx, q, events, t, p)
	})
	if err != nil {
		return nil, err
	}
	return oas.DeleteClientDiscussionPost204Response{}, nil
}

func (s *Server) clientVote(ctx context.Context, id uuid.UUID, add bool) (oas.DiscussionPost, error) {
	d := discussantFrom(ctx)
	if err := s.rateLimit(ctx, s.writeChecks(ctx, contactPrincipal{channelID: d.channelID})...); err != nil {
		return oas.DiscussionPost{}, err
	}
	err := s.inTx(ctx, d.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		t, p, err := d.post(ctx, q, id)
		if err != nil {
			return err
		}
		if err := d.mayChange(t); err != nil {
			return err
		}
		if p.State != statePublished || p.DeletedAt != nil {
			return errDiscussionPostGone
		}
		if !add && d.memberID == nil && d.contactID == nil {
			return nil
		}
		if add && ideaVotesClosed(t.d) {
			return errIdeaClosed
		}
		if t.d.Kind == kindIdea {
			if err := s.rateLimitTx(ctx, q, rateCheck{"idea:vote:" + d.rateWho(), limitIdeaVotes}); err != nil {
				return err
			}
		}
		if err := s.bindWriter(ctx, q, d); err != nil {
			return err
		}
		if err := vote(ctx, q, d.workspaceID, p, d.actor, add, s.now()); err != nil {
			return err
		}
		if add && t.d.Kind == kindIdea && d.contactID != nil {
			return q.FollowDiscussion(ctx, store.FollowDiscussionParams{WorkspaceID: d.workspaceID, DiscussionID: t.d.ID, ContactID: *d.contactID, Now: s.now()})
		}
		return nil
	})
	if err != nil {
		return oas.DiscussionPost{}, err
	}
	return postView(ctx, s.st.Queries, d.workspaceID, id, d.viewer())
}

func (s *Server) VoteClientDiscussionPost(ctx context.Context, req oas.VoteClientDiscussionPostRequestObject) (oas.VoteClientDiscussionPostResponseObject, error) {
	out, err := s.clientVote(ctx, req.PostId, true)
	if err != nil {
		return nil, err
	}
	return oas.VoteClientDiscussionPost200JSONResponse(out), nil
}

func (s *Server) UnvoteClientDiscussionPost(ctx context.Context, req oas.UnvoteClientDiscussionPostRequestObject) (oas.UnvoteClientDiscussionPostResponseObject, error) {
	out, err := s.clientVote(ctx, req.PostId, false)
	if err != nil {
		return nil, err
	}
	return oas.UnvoteClientDiscussionPost200JSONResponse(out), nil
}

func (s *Server) DeleteReaderToken(ctx context.Context, _ oas.DeleteReaderTokenRequestObject) (oas.DeleteReaderTokenResponseObject, error) {
	d := discussantFrom(ctx)
	if d.readerTokenID == nil {
		return nil, errContactUnauthenticated
	}
	if err := s.st.DeleteReaderToken(ctx, store.DeleteReaderTokenParams{WorkspaceID: d.workspaceID, ID: *d.readerTokenID}); err != nil {
		return nil, err
	}
	return oas.DeleteReaderToken204Response{}, nil
}

func (s *Server) createClientIdea(ctx context.Context, d *discussant, b *oas.ClientDiscussionCreate) (oas.CreateClientDiscussionResponseObject, error) {
	if err := s.roadmapOn(); err != nil {
		return nil, err
	}
	if err := s.rateLimit(ctx, s.discussionChecks(ctx, d)...); err != nil {
		return nil, err
	}
	if b.PageUrl != nil || b.PageTitle != nil {
		return nil, errIdeaPageNotAllowed
	}
	title, err := discussionTitle(b.Title)
	if err != nil {
		return nil, err
	}
	body, err := discussionBodyText(b.Body)
	if err != nil {
		return nil, err
	}
	var id uuid.UUID
	err = s.inTx(ctx, d.workspaceID, func(q *store.Queries, events *eventBatch) error {
		board, err := s.inboxBoard(ctx, q, d.workspaceID, d.inboxID)
		if err != nil {
			return err
		}
		if board.Suggestions == string(oas.RoadmapSuggestionsOff) {
			return errSuggestionsOff
		}
		if err := s.bindWriter(ctx, q, d); err != nil {
			return err
		}
		state, err := newPostState(ctx, q, d.workspaceID, d.inboxID, board.Suggestions, d.actor, d.identified)
		if err != nil {
			return err
		}
		if state == statePending {
			n, err := q.CountPendingBoardIdeas(ctx, store.CountPendingBoardIdeasParams{WorkspaceID: d.workspaceID, BoardID: &board.ID})
			if err != nil {
				return err
			}
			if int(n) >= maxPendingIdeas {
				return errSuggestionsFull
			}
		}
		if err := s.ideaLimit(ctx, q, d.workspaceID); err != nil {
			return err
		}
		t, err := s.createThread(ctx, q, events, newThread{
			ws: d.workspaceID, inbox: d.inboxID, channel: board.ChannelID, board: &board.ID, title: title, body: body, author: d.actor, state: state,
		})
		id = t.ID
		return err
	})
	if err != nil {
		return nil, err
	}
	out, err := threadView(ctx, s.st.Queries, d.workspaceID, id, d.viewer())
	if err != nil {
		return nil, err
	}
	return oas.CreateClientDiscussion201JSONResponse(out), nil
}

func (s *Server) listClientIdeas(ctx context.Context, ch store.FindChatChannelByKeyRow, prm oas.ListClientDiscussionsParams) (oas.ListClientDiscussionsResponseObject, error) {
	if err := s.roadmapOn(); err != nil {
		return nil, err
	}
	board, err := s.inboxBoard(ctx, s.st.Queries, ch.WorkspaceID, ch.InboxID)
	if err != nil {
		return nil, err
	}
	sort := oas.DiscussionSortNew
	if prm.Sort != nil {
		if !prm.Sort.Valid() || *prm.Sort == oas.DiscussionSortUnanswered {
			return nil, errValidation("sort must be new or top for ideas")
		}
		sort = *prm.Sort
	}
	arg := store.ListBoardIdeasParams{WorkspaceID: ch.WorkspaceID, BoardID: &board.ID, Sort: string(sort)}
	if prm.Stage != nil {
		if !prm.Stage.Valid() {
			return nil, errValidation("stage must be ideas, planned, in_progress, shipped or closed")
		}
		arg.Stage = new(string(*prm.Stage))
	}
	if arg.Tsq, err = ideaTsQuery(prm.Q); err != nil {
		return nil, err
	}
	if arg.Tsq != nil {
		if err := s.rateLimit(ctx, rateCheck{"ideas:search:" + rateIP(s.clientIP(requestFrom(ctx))) + ":" + ch.ChannelID.String(), limitIdeaSearch}); err != nil {
			return nil, err
		}
	}
	lim, err := pageSize(prm.Limit)
	if err != nil {
		return nil, err
	}
	off, err := decodePosition(string(sort), prm.Cursor)
	if err != nil {
		return nil, err
	}
	v, cache, err := s.readerOf(ctx, ch)
	if err != nil {
		return nil, err
	}
	arg.ViewerContact, arg.ViewerMember, arg.Lim, arg.Off = v.contactID, v.memberID, lim+1, off
	var rows []store.Discussion
	if err := s.searchTx(ctx, arg.Tsq != nil, func(q *store.Queries) (err error) {
		rows, err = q.ListBoardIdeas(ctx, arg)
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
	items, err := discussionsBody(ctx, s.st.Queries, ch.WorkspaceID, rows, v)
	if err != nil {
		return nil, err
	}
	return oas.ListClientDiscussions200JSONResponse{
		Body:    oas.DiscussionPage{Items: items, NextCursor: next},
		Headers: oas.ListClientDiscussions200ResponseHeaders{CacheControl: &cache},
	}, nil
}

func (s *Server) GetClientRoadmap(ctx context.Context, req oas.GetClientRoadmapRequestObject) (oas.GetClientRoadmapResponseObject, error) {
	if err := s.roadmapOn(); err != nil {
		return nil, errRoadmapNoBoard
	}
	ch, err := s.discussionChannel(ctx, req.ChannelKey)
	if err != nil {
		return nil, err
	}
	board, err := s.inboxBoard(ctx, s.st.Queries, ch.WorkspaceID, ch.InboxID)
	if err != nil {
		return nil, err
	}
	counts, err := s.roadmapBoardOut(ctx, s.st.Queries, board)
	if err != nil {
		return nil, err
	}
	cache := publicMaxAge
	return oas.GetClientRoadmap200JSONResponse{
		Body: oas.ClientRoadmap{
			Name: board.Name, Description: board.Description, Url: s.roadmapURL(board), Suggestions: oas.RoadmapSuggestions(board.Suggestions),
			Stages: oas.ClientRoadmapStages{
				Ideas: counts.Counts.Ideas, Planned: counts.Counts.Planned, InProgress: counts.Counts.InProgress, Shipped: counts.Counts.Shipped,
			},
		},
		Headers: oas.GetClientRoadmap200ResponseHeaders{CacheControl: &cache},
	}, nil
}
