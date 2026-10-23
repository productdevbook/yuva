package api

import (
	"context"
	"net/http"
	"strconv"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	kindPage = string(oas.DiscussionKindPage)
	kindIdea = string(oas.DiscussionKindIdea)

	stageIdeas      = string(oas.IdeaStageIdeas)
	stagePlanned    = string(oas.IdeaStagePlanned)
	stageInProgress = string(oas.IdeaStageInProgress)
	stageShipped    = string(oas.IdeaStageShipped)
	stageClosed     = string(oas.IdeaStageClosed)

	roadmapPathPrefix      = "/roadmap/"
	roadmapFormerSlugsKept = 10
	roadmapFormerSlugDays  = 90
	ideaVotesPerHour       = 120
	ideaSearchPerMinute    = 30
	maxIdeaQueryLen        = 200

	eventDiscussionStageChanged = string(oas.DiscussionStageEventTypeDiscussionStageChanged)
	signalRoadmapBoard          = string(oas.RoadmapBoardUpdated)
)

var (
	maxPendingIdeas = 200

	limitRoadmapSlug = limit{10, 24 * time.Hour}
	limitIdeaVotes   = limit{ideaVotesPerHour, time.Hour}
	limitIdeaSearch  = limit{ideaSearchPerMinute, time.Minute}

	errRoadmapOff         = problem(http.StatusNotFound, "roadmap_off", "roadmap boards are off on this server")
	errRoadmapNoBoard     = problem(http.StatusNotFound, "roadmap_off", "this inbox has no public roadmap board")
	errBoardGone          = problem(http.StatusNotFound, "not_found", "no such roadmap board")
	errChannelHasBoard    = problem(http.StatusConflict, "channel_has_roadmap_board", "a roadmap board files its ideas on this channel: delete the board or point it at another channel first")
	errBoardExists        = problem(http.StatusConflict, "roadmap_board_exists", "this inbox already has a roadmap board")
	errBoardSlugTaken     = problem(http.StatusConflict, "slug_taken", "another roadmap board has this slug")
	errNotAnIdea          = problem(http.StatusConflict, "not_an_idea", "this discussion is not a roadmap idea")
	errIdeaClosed         = problem(http.StatusConflict, "idea_closed", "this idea is shipped or closed and takes no new votes")
	errNotSupported       = problem(http.StatusConflict, "not_supported", "ideas have no accepted answer")
	errSuggestionsOff     = problem(http.StatusForbidden, "suggestions_off", "this board takes no suggestions")
	errSuggestionsFull    = &apiError{Status: http.StatusTooManyRequests, Code: "suggestions_full", Detail: "too many suggestions wait for a member; try again later", RetryAfter: 3600}
	errIdeaPageNotAllowed = errValidation("an idea belongs to no page: leave out channel_id, page_url and page_title")
)

type RoadmapSettings struct {
	Off      bool
	MaxIdeas int
}

func (s *Server) roadmapOn() error {
	if s.roadmap.Off {
		return errRoadmapOff
	}
	return nil
}

func (s *Server) roadmapURL(b store.RoadmapBoard) string {
	if s.domainLive(b.CustomDomain, b.DomainState, b.CfStatus, b.CfSslStatus) {
		return s.domainURL(*b.CustomDomain, "")
	}
	return s.auth.PublicURL + roadmapPathPrefix + b.Slug
}

func roadmapUniqueConflict(err error) error {
	switch store.UniqueViolationConstraint(err) {
	case "roadmap_boards_workspace_id_inbox_id_key":
		return errBoardExists
	case "roadmap_boards_slug_key", "roadmap_board_slugs_pkey":
		return errBoardSlugTaken
	}
	return err
}

func visibleBoard(ctx context.Context, q *store.Queries, p principal, id uuid.UUID, lock bool) (store.RoadmapBoard, error) {
	get := q.GetRoadmapBoard
	if lock {
		get = func(ctx context.Context, arg store.GetRoadmapBoardParams) (store.RoadmapBoard, error) {
			return q.LockRoadmapBoard(ctx, store.LockRoadmapBoardParams(arg))
		}
	}
	b, err := get(ctx, store.GetRoadmapBoardParams{WorkspaceID: p.workspaceID, ID: id})
	if store.IsNotFound(err) {
		return b, errBoardGone
	}
	if err != nil {
		return b, err
	}
	ok, err := canSeeInbox(ctx, q, p, b.InboxID)
	if err != nil {
		return b, err
	}
	if !ok {
		return b, errBoardGone
	}
	return b, nil
}

// Boards change what the public sees, so OAuth tokens never manage them.
func requireBoardManager(p principal) error {
	if p.isGrant() {
		return errMemberSessionRequired
	}
	return requireManagerOrKey(p)
}

type roadmapBoardFields struct {
	name, description, slug string
	channel                 uuid.UUID
	public                  bool
	suggestions             string
	property                *uuid.UUID
}

func (f *roadmapBoardFields) check(ctx context.Context, q *store.Queries, ws, inboxID uuid.UUID) error {
	var err error
	if f.name, err = trimmed(f.name, 1, 100, "name"); err != nil {
		return err
	}
	if f.description, err = trimmed(f.description, 0, 1000, "description"); err != nil {
		return err
	}
	if err := validPublicSlug(f.slug); err != nil {
		return err
	}
	if !oas.RoadmapSuggestions(f.suggestions).Valid() {
		return errValidation("suggestions must be off, moderated or open")
	}
	ch, err := q.GetChannel(ctx, store.GetChannelParams{WorkspaceID: ws, ID: f.channel})
	if store.IsNotFound(err) || (err == nil && (ch.InboxID != inboxID || ch.Kind != string(oas.ChannelKindChat))) {
		return errValidation("channel_id must be a chat channel of the board's inbox")
	}
	if err != nil {
		return err
	}
	if f.property != nil {
		_, err := q.GetAnalyticsProperty(ctx, store.GetAnalyticsPropertyParams{WorkspaceID: ws, ID: *f.property})
		if store.IsNotFound(err) {
			return errValidation("analytics_property_id must be an analytics property of the workspace")
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) roadmapBoardOut(ctx context.Context, q *store.Queries, b store.RoadmapBoard) (oas.RoadmapBoard, error) {
	rows, err := q.CountRoadmapBoardIdeas(ctx, store.CountRoadmapBoardIdeasParams{WorkspaceID: b.WorkspaceID, BoardID: &b.ID})
	if err != nil {
		return oas.RoadmapBoard{}, err
	}
	var counts oas.RoadmapBoardCounts
	for _, r := range rows {
		if r.State == statePending {
			counts.Pending += r.N
			continue
		}
		switch r.Stage {
		case stageIdeas:
			counts.Ideas = r.N
		case stagePlanned:
			counts.Planned = r.N
		case stageInProgress:
			counts.InProgress = r.N
		case stageShipped:
			counts.Shipped = r.N
		case stageClosed:
			counts.Closed = r.N
		}
	}
	return oas.RoadmapBoard{
		Id: b.ID, InboxId: b.InboxID, ChannelId: b.ChannelID, Name: b.Name, Description: b.Description, Slug: oas.RoadmapBoardSlug(b.Slug),
		Url: s.roadmapURL(b), Public: b.Public, Suggestions: oas.RoadmapSuggestions(b.Suggestions), AnalyticsPropertyId: b.AnalyticsPropertyID,
		Counts: counts, CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt, CustomDomain: s.customDomainBody(boardOwn(b)), OwnDomains: s.ownDomainsBody(),
	}, nil
}

func (s *Server) ListRoadmapBoards(ctx context.Context, req oas.ListRoadmapBoardsRequestObject) (oas.ListRoadmapBoardsResponseObject, error) {
	if err := s.roadmapOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	rows, err := s.st.ListRoadmapBoards(ctx, store.ListRoadmapBoardsParams{
		WorkspaceID: p.workspaceID, InboxID: req.Params.InboxId, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(),
	})
	if err != nil {
		return nil, err
	}
	out := oas.ListRoadmapBoards200JSONResponse{Items: make([]oas.RoadmapBoard, len(rows))}
	for i, b := range rows {
		if out.Items[i], err = s.roadmapBoardOut(ctx, s.st.Queries, b); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Server) CreateRoadmapBoard(ctx context.Context, req oas.CreateRoadmapBoardRequestObject) (oas.CreateRoadmapBoardResponseObject, error) {
	if err := s.roadmapOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	if err := requireBoardManager(p); err != nil {
		return nil, err
	}
	b := req.Body
	var out oas.RoadmapBoard
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		in, err := visibleInbox(ctx, q, p, b.InboxId)
		if err != nil {
			return err
		}
		f := roadmapBoardFields{
			name: b.Name, slug: string(b.Slug), channel: b.ChannelId, public: boolOr(b.Public, false),
			suggestions: string(oas.RoadmapSuggestionsModerated), property: b.AnalyticsPropertyId,
		}
		if b.Description != nil {
			f.description = *b.Description
		}
		if b.Suggestions != nil {
			f.suggestions = string(*b.Suggestions)
		}
		if err := f.check(ctx, q, p.workspaceID, in.ID); err != nil {
			return err
		}
		if _, err := q.GetInboxRoadmapBoard(ctx, store.GetInboxRoadmapBoardParams{WorkspaceID: p.workspaceID, InboxID: in.ID}); err == nil {
			return errBoardExists
		} else if !store.IsNotFound(err) {
			return err
		}
		if taken, err := q.RoadmapSlugTaken(ctx, store.RoadmapSlugTakenParams{Slug: f.slug, ID: uuid.Nil()}); err != nil || taken {
			if err == nil {
				err = errBoardSlugTaken
			}
			return err
		}
		board, err := q.CreateRoadmapBoard(ctx, store.CreateRoadmapBoardParams{
			ID: newID(), WorkspaceID: p.workspaceID, InboxID: in.ID, ChannelID: f.channel, Name: f.name, Description: f.description,
			Slug: f.slug, Public: f.public, Suggestions: f.suggestions, AnalyticsPropertyID: f.property, Now: s.now(),
		})
		if err != nil {
			return err
		}
		out, err = s.roadmapBoardOut(ctx, q, board)
		return err
	})
	if err != nil {
		return nil, roadmapUniqueConflict(err)
	}
	s.statusHint(ctx, p.workspaceID, out.InboxId)
	return oas.CreateRoadmapBoard201JSONResponse(out), nil
}

func (s *Server) GetRoadmapBoard(ctx context.Context, req oas.GetRoadmapBoardRequestObject) (oas.GetRoadmapBoardResponseObject, error) {
	if err := s.roadmapOn(); err != nil {
		return nil, err
	}
	b, err := visibleBoard(ctx, s.st.Queries, principalFrom(ctx), req.BoardId, false)
	if err != nil {
		return nil, err
	}
	out, err := s.roadmapBoardOut(ctx, s.st.Queries, b)
	if err != nil {
		return nil, err
	}
	return oas.GetRoadmapBoard200JSONResponse(out), nil
}

func (s *Server) UpdateRoadmapBoard(ctx context.Context, req oas.UpdateRoadmapBoardRequestObject) (oas.UpdateRoadmapBoardResponseObject, error) {
	if err := s.roadmapOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	if err := requireBoardManager(p); err != nil {
		return nil, err
	}
	b := req.Body
	var out oas.RoadmapBoard
	var hosts []string
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, ev *eventBatch) error {
		cur, err := visibleBoard(ctx, q, p, req.BoardId, true)
		if err != nil {
			return err
		}
		if cur.CustomDomain != nil {
			hosts = append(hosts, *cur.CustomDomain)
		}
		f := roadmapBoardFields{
			name: cur.Name, description: cur.Description, slug: cur.Slug, channel: cur.ChannelID, public: boolOr(b.Public, cur.Public),
			suggestions: cur.Suggestions, property: nullableID(b.AnalyticsPropertyId, cur.AnalyticsPropertyID),
		}
		if b.Name != nil {
			f.name = *b.Name
		}
		if b.Description != nil {
			f.description = *b.Description
		}
		if b.Slug != nil {
			f.slug = string(*b.Slug)
		}
		if b.ChannelId != nil {
			f.channel = *b.ChannelId
		}
		if b.Suggestions != nil {
			f.suggestions = string(*b.Suggestions)
		}
		if err := f.check(ctx, q, p.workspaceID, cur.InboxID); err != nil {
			return err
		}
		now := s.now()
		if f.slug != cur.Slug {
			if err := s.rateLimit(rateCheck{"roadmap-slug:" + cur.ID.String(), limitRoadmapSlug}); err != nil {
				return err
			}
			if taken, err := q.RoadmapSlugTaken(ctx, store.RoadmapSlugTakenParams{Slug: f.slug, ID: cur.ID}); err != nil || taken {
				if err == nil {
					err = errBoardSlugTaken
				}
				return err
			}
			if err := q.ReleaseRoadmapFormerSlug(ctx, store.ReleaseRoadmapFormerSlugParams{WorkspaceID: cur.WorkspaceID, BoardID: cur.ID, Slug: f.slug}); err != nil {
				return err
			}
			if err := q.AddRoadmapFormerSlug(ctx, store.AddRoadmapFormerSlugParams{Slug: cur.Slug, WorkspaceID: cur.WorkspaceID, BoardID: cur.ID, Now: now}); err != nil {
				return err
			}
			if err := q.PruneRoadmapFormerSlugs(ctx, store.PruneRoadmapFormerSlugsParams{WorkspaceID: cur.WorkspaceID, BoardID: cur.ID, Keep: roadmapFormerSlugsKept}); err != nil {
				return err
			}
		}
		if f.channel != cur.ChannelID {
			if err := q.MoveBoardIdeasToChannel(ctx, store.MoveBoardIdeasToChannelParams{WorkspaceID: p.workspaceID, BoardID: &cur.ID, ChannelID: f.channel}); err != nil {
				return err
			}
		}
		board, err := q.UpdateRoadmapBoard(ctx, store.UpdateRoadmapBoardParams{
			WorkspaceID: p.workspaceID, ID: cur.ID, ChannelID: f.channel, Name: f.name, Description: f.description, Slug: f.slug, Public: f.public,
			Suggestions: f.suggestions, AnalyticsPropertyID: f.property, Now: now,
		})
		if err != nil {
			return err
		}
		if sent, raw := customDomainField(b.CustomDomain); sent {
			d, err := s.setOwnDomain(ctx, q, ev, boardOwn(board), raw)
			if err != nil {
				return err
			}
			board = d.row.(store.RoadmapBoard)
			if board.CustomDomain != nil {
				hosts = append(hosts, *board.CustomDomain)
			}
		}
		out, err = s.roadmapBoardOut(ctx, q, board)
		return err
	})
	if err != nil {
		return nil, roadmapUniqueConflict(err)
	}
	s.statusDomainCache.forget(req.BoardId, hosts...)
	s.statusHint(ctx, p.workspaceID, out.InboxId)
	s.boardSignal(ctx, p.workspaceID, out.InboxId, out.Id)
	return oas.UpdateRoadmapBoard200JSONResponse(out), nil
}

func (s *Server) DeleteRoadmapBoard(ctx context.Context, req oas.DeleteRoadmapBoardRequestObject) (oas.DeleteRoadmapBoardResponseObject, error) {
	if err := s.roadmapOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	if err := requireBoardManager(p); err != nil {
		return nil, err
	}
	var inbox uuid.UUID
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, ev *eventBatch) error {
		b, err := visibleBoard(ctx, q, p, req.BoardId, true)
		if err != nil {
			return err
		}
		inbox = b.InboxID
		releaseOwnCloudflare(ev, boardOwn(b))
		_, err = q.DeleteRoadmapBoard(ctx, store.DeleteRoadmapBoardParams{WorkspaceID: p.workspaceID, ID: b.ID})
		return err
	})
	if err != nil {
		return nil, err
	}
	s.statusDomainCache.forget(req.BoardId)
	s.statusHint(ctx, p.workspaceID, inbox)
	s.boardSignal(ctx, p.workspaceID, inbox, req.BoardId)
	return oas.DeleteRoadmapBoard204Response{}, nil
}

func (s *Server) boardSignal(ctx context.Context, ws, inboxID, boardID uuid.UUID) {
	s.signal(ctx, realtime.Event{Type: signalRoadmapBoard, WorkspaceID: ws, InboxID: &inboxID, Data: mustJSON(map[string]uuid.UUID{"board_id": boardID})})
}

func (s *Server) inboxBoard(ctx context.Context, q *store.Queries, ws, inboxID uuid.UUID) (store.RoadmapBoard, error) {
	if s.roadmap.Off {
		return store.RoadmapBoard{}, errRoadmapNoBoard
	}
	b, err := q.GetInboxRoadmapBoard(ctx, store.GetInboxRoadmapBoardParams{WorkspaceID: ws, InboxID: inboxID})
	if store.IsNotFound(err) || (err == nil && !b.Public) {
		return b, errRoadmapNoBoard
	}
	return b, err
}

func (s *Server) clientRoadmapInbox(ctx context.Context, q *store.Queries, in store.Inbox) (*oas.ClientInboxRoadmap, error) {
	b, err := s.inboxBoard(ctx, q, in.WorkspaceID, in.ID)
	if err == errRoadmapNoBoard {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &oas.ClientInboxRoadmap{Url: s.roadmapURL(b), Suggestions: oas.RoadmapSuggestions(b.Suggestions)}, nil
}

func ideaVotesClosed(d store.Discussion) bool {
	return d.Kind == kindIdea && d.Stage != nil && (*d.Stage == stageShipped || *d.Stage == stageClosed)
}

func (s *Server) ideaLimit(ctx context.Context, q *store.Queries, ws uuid.UUID) error {
	max := s.roadmap.MaxIdeas
	if max <= 0 {
		return nil
	}
	n, err := q.CountWorkspaceIdeas(ctx, ws)
	if err != nil {
		return err
	}
	if int(n) >= max {
		return errHostedLimit(oas.LimitRoadmapMaxIdeas, "this workspace has as many roadmap ideas as the server allows ("+strconv.Itoa(max)+", YUVA_ROADMAP_MAX_IDEAS)")
	}
	return nil
}

func (s *Server) CleanRoadmap(ctx context.Context) error {
	ids, err := s.st.ListWorkspaceIDs(ctx)
	if err != nil {
		return err
	}
	for _, ws := range ids {
		if _, err := s.st.DeleteOldRoadmapFormerSlugs(ctx, store.DeleteOldRoadmapFormerSlugsParams{WorkspaceID: ws, Before: s.now().AddDate(0, 0, -roadmapFormerSlugDays)}); err != nil {
			return err
		}
	}
	return nil
}
