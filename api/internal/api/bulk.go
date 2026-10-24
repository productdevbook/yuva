package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const maxBulkItems = 100

// bulkChange changes one item inside its transaction; after, when set, runs once it is committed.
type bulkChange func(q *store.Queries, ev *eventBatch, id uuid.UUID) (after func(), err error)

func bulkIDs(ids []uuid.UUID) ([]uuid.UUID, error) {
	if len(ids) == 0 || len(ids) > maxBulkItems {
		return nil, errValidation("ids must name 1 to 100 items")
	}
	return uniqueIDs(ids), nil
}

// bulkApply applies change to each id in its own transaction; a refusal lists the id in failed and
// the others go on.
func (s *Server) bulkApply(ctx context.Context, p principal, ids []uuid.UUID, change bulkChange) (oas.BulkResult, error) {
	out := oas.BulkResult{Succeeded: []uuid.UUID{}, Failed: []oas.BulkFailure{}}
	for _, id := range ids {
		var after func()
		err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, ev *eventBatch) error {
			var err error
			after, err = change(q, ev, id)
			return err
		})
		var ae *apiError
		switch {
		case err == nil:
			out.Succeeded = append(out.Succeeded, id)
			if after != nil {
				after()
			}
		case errors.As(err, &ae) && ae.Status < http.StatusInternalServerError:
			f, err := bulkFailure(id, ae)
			if err != nil {
				return out, err
			}
			out.Failed = append(out.Failed, f)
		default:
			return out, err
		}
	}
	return out, nil
}

// bulkFailure carries the problem's extras that BulkFailure types, such as used_by.
func bulkFailure(id uuid.UUID, ae *apiError) (oas.BulkFailure, error) {
	var f oas.BulkFailure
	if len(ae.Extra) > 0 {
		raw, err := json.Marshal(ae.Extra)
		if err != nil {
			return f, err
		}
		if err := json.Unmarshal(raw, &f); err != nil {
			return f, err
		}
	}
	f.Id, f.Status, f.Code = id, int32(ae.Status), ae.Code
	f.Detail, f.Field = nil, nil
	if ae.Detail != "" {
		f.Detail = new(ae.Detail)
	}
	if ae.Field != "" {
		f.Field = new(ae.Field)
	}
	return f, nil
}

func noAfter(err error) (func(), error) { return nil, err }

func (s *Server) BulkChangeHelpArticles(ctx context.Context, req oas.BulkChangeHelpArticlesRequestObject) (oas.BulkChangeHelpArticlesResponseObject, error) {
	if err := s.helpOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	b := req.Body
	ids, err := bulkIDs(b.Ids)
	if err != nil {
		return nil, err
	}
	if !b.Action.Valid() {
		return nil, errValidation("action must be delete, publish, unpublish or move")
	}
	move := b.Action == oas.HelpArticleBulkChangeActionMove
	publishing := b.Action == oas.HelpArticleBulkChangeActionPublish || b.Action == oas.HelpArticleBulkChangeActionUnpublish
	if b.CollectionId.IsSpecified() != move {
		return nil, errValidation("collection_id goes with move, and only with move")
	}
	if b.Locale != nil && !publishing {
		return nil, errValidation("locale goes with publish and unpublish only")
	}
	if publishing {
		err = requireScope(p, oas.HelpPublish)
	} else {
		err = requireScope(p, oas.HelpWrite)
	}
	if err != nil {
		return nil, err
	}
	c, err := visibleHelpCenter(ctx, s.st.Queries, p, req.CenterId, false)
	if err != nil {
		return nil, err
	}
	var change bulkChange
	switch b.Action {
	case oas.HelpArticleBulkChangeActionDelete:
		change = func(q *store.Queries, ev *eventBatch, id uuid.UUID) (func(), error) {
			return noAfter(s.deleteHelpArticle(ctx, q, ev, p, c.ID, id))
		}
	case oas.HelpArticleBulkChangeActionPublish, oas.HelpArticleBulkChangeActionUnpublish:
		if err := mayPublishHelp(p, c); err != nil {
			return nil, err
		}
		locale := c.DefaultLocale
		if b.Locale != nil {
			if err := validHelpLocale(*b.Locale, "locale"); err != nil {
				return nil, err
			}
			locale = *b.Locale
		}
		publish := b.Action == oas.HelpArticleBulkChangeActionPublish
		change = func(q *store.Queries, ev *eventBatch, id uuid.UUID) (func(), error) {
			var err error
			if publish {
				_, err = s.publishHelpTranslation(ctx, q, ev, p, c.ID, id, locale, nil)
			} else {
				_, err = s.unpublishHelpTranslation(ctx, q, ev, p, c.ID, id, locale)
			}
			return noAfter(err)
		}
	case oas.HelpArticleBulkChangeActionMove:
		var col *uuid.UUID
		if !b.CollectionId.IsNull() {
			col = new(b.CollectionId.MustGet())
		}
		if err := helpCollectionOf(ctx, s.st.Queries, c, col); err != nil {
			return nil, err
		}
		upd := oas.HelpArticleUpdate{CollectionId: b.CollectionId}
		change = func(q *store.Queries, _ *eventBatch, id uuid.UUID) (func(), error) {
			_, err := s.updateHelpArticle(ctx, q, p, c.ID, id, upd)
			return noAfter(err)
		}
	}
	out, err := s.bulkApply(ctx, p, ids, change)
	if err != nil {
		return nil, err
	}
	return oas.BulkChangeHelpArticles200JSONResponse(out), nil
}

func (s *Server) BulkChangeChangelogEntries(ctx context.Context, req oas.BulkChangeChangelogEntriesRequestObject) (oas.BulkChangeChangelogEntriesResponseObject, error) {
	if err := s.whatsNewOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	b := req.Body
	ids, err := bulkIDs(b.Ids)
	if err != nil {
		return nil, err
	}
	if !b.Action.Valid() {
		return nil, errValidation("action must be delete or publish")
	}
	publish := b.Action == oas.ChangelogEntryBulkChangeActionPublish
	if publish {
		err = requireScope(p, oas.ChangelogPublish)
	} else {
		err = requireScope(p, oas.ChangelogWrite)
	}
	if err != nil {
		return nil, err
	}
	c, err := visibleChangelog(ctx, s.st.Queries, p, req.ChangelogId, false)
	if err != nil {
		return nil, err
	}
	if publish {
		if err := mayPublishChangelog(p, c); err != nil {
			return nil, err
		}
	}
	out, err := s.bulkApply(ctx, p, ids, func(q *store.Queries, ev *eventBatch, id uuid.UUID) (func(), error) {
		if publish {
			e, err := changelogEntry(ctx, q, c, id, true)
			if err != nil {
				return nil, err
			}
			if e.State == changelogStatePublished {
				return nil, nil
			}
			_, after, err := s.publishChangelogEntry(ctx, q, ev, p, c.ID, id, "", nil)
			return after, err
		}
		return s.deleteChangelogEntry(ctx, q, ev, p, c.ID, id)
	})
	if err != nil {
		return nil, err
	}
	return oas.BulkChangeChangelogEntries200JSONResponse(out), nil
}

func (s *Server) BulkChangeSubscriptions(ctx context.Context, req oas.BulkChangeSubscriptionsRequestObject) (oas.BulkChangeSubscriptionsResponseObject, error) {
	p := principalFrom(ctx)
	b := req.Body
	ids, err := bulkIDs(b.Ids)
	if err != nil {
		return nil, err
	}
	if !b.Action.Valid() {
		return nil, errValidation("action must be unsubscribe or delete")
	}
	list, err := visibleList(ctx, s.st.Queries, p, req.ListId, false)
	if err != nil {
		return nil, err
	}
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	out, err := s.bulkApply(ctx, p, ids, func(q *store.Queries, ev *eventBatch, id uuid.UUID) (func(), error) {
		if b.Action == oas.SubscriptionBulkChangeActionUnsubscribe {
			_, err := s.setSubscriptionState(ctx, q, ev, p, list.ID, id, oas.SubscriptionStateUnsubscribed)
			return noAfter(err)
		}
		return noAfter(s.deleteSubscription(ctx, q, p, list.ID, id))
	})
	if err != nil {
		return nil, err
	}
	return oas.BulkChangeSubscriptions200JSONResponse(out), nil
}

func (s *Server) BulkChangeEmailDesigns(ctx context.Context, req oas.BulkChangeEmailDesignsRequestObject) (oas.BulkChangeEmailDesignsResponseObject, error) {
	p := principalFrom(ctx)
	ids, err := bulkIDs(req.Body.Ids)
	if err != nil {
		return nil, err
	}
	if !req.Body.Action.Valid() {
		return nil, errValidation("action must be delete")
	}
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	out, err := s.bulkApply(ctx, p, ids, func(q *store.Queries, _ *eventBatch, id uuid.UUID) (func(), error) {
		return noAfter(s.archiveEmailDesign(ctx, q, p, id))
	})
	if err != nil {
		return nil, err
	}
	return oas.BulkChangeEmailDesigns200JSONResponse(out), nil
}

func (s *Server) BulkChangeEmailImages(ctx context.Context, req oas.BulkChangeEmailImagesRequestObject) (oas.BulkChangeEmailImagesResponseObject, error) {
	p := principalFrom(ctx)
	ids, err := bulkIDs(req.Body.Ids)
	if err != nil {
		return nil, err
	}
	if !req.Body.Action.Valid() {
		return nil, errValidation("action must be delete")
	}
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	out, err := s.bulkApply(ctx, p, ids, func(q *store.Queries, _ *eventBatch, id uuid.UUID) (func(), error) {
		key, err := s.deleteEmailImage(ctx, q, p, id, false)
		if err != nil {
			return nil, err
		}
		return func() { s.deleteObjects(ctx, []string{key}) }, nil
	})
	if err != nil {
		return nil, err
	}
	return oas.BulkChangeEmailImages200JSONResponse(out), nil
}

func (s *Server) BulkChangeBroadcasts(ctx context.Context, req oas.BulkChangeBroadcastsRequestObject) (oas.BulkChangeBroadcastsResponseObject, error) {
	p := principalFrom(ctx)
	ids, err := bulkIDs(req.Body.Ids)
	if err != nil {
		return nil, err
	}
	if !req.Body.Action.Valid() {
		return nil, errValidation("action must be delete")
	}
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	out, err := s.bulkApply(ctx, p, ids, func(q *store.Queries, _ *eventBatch, id uuid.UUID) (func(), error) {
		return noAfter(deleteBroadcast(ctx, q, p, id))
	})
	if err != nil {
		return nil, err
	}
	return oas.BulkChangeBroadcasts200JSONResponse(out), nil
}

func (s *Server) BulkChangeDiscussions(ctx context.Context, req oas.BulkChangeDiscussionsRequestObject) (oas.BulkChangeDiscussionsResponseObject, error) {
	p := principalFrom(ctx)
	b := req.Body
	ids, err := bulkIDs(b.Ids)
	if err != nil {
		return nil, err
	}
	if !b.Action.Valid() {
		return nil, errValidation("action must be hide, unhide, spam or delete")
	}
	if b.Action != oas.DiscussionBulkChangeActionDelete {
		if err := requireManagerOrKey(p); err != nil {
			return nil, err
		}
	}
	out, err := s.bulkApply(ctx, p, ids, func(q *store.Queries, ev *eventBatch, id uuid.UUID) (func(), error) {
		t, err := teamThread(ctx, q, p, id, false)
		if err != nil {
			return nil, err
		}
		t, post, err := teamPost(ctx, q, p, t.d.OpeningPostID)
		if err != nil {
			return nil, err
		}
		switch b.Action {
		case oas.DiscussionBulkChangeActionHide:
			return noAfter(s.setPostState(ctx, q, ev, t, post, stateHidden))
		case oas.DiscussionBulkChangeActionUnhide:
			return noAfter(s.setPostState(ctx, q, ev, t, post, statePublished))
		case oas.DiscussionBulkChangeActionSpam:
			return noAfter(s.markSpam(ctx, q, ev, t, post))
		}
		return noAfter(s.deleteOwnPost(ctx, q, ev, p, t, post))
	})
	if err != nil {
		return nil, err
	}
	return oas.BulkChangeDiscussions200JSONResponse(out), nil
}
