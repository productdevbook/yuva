package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

// ratingWindow is how long after a close the contact may rate it.
const (
	ratingWindow     = 30 * 24 * time.Hour
	maxRatingComment = 2000
)

var (
	errRatingUnavailable = problem(http.StatusConflict, "rating_unavailable", "this conversation cannot be rated now")
	errAlreadyRated      = problem(http.StatusConflict, "already_rated", "this conversation was already rated since it was closed")
)

// ratingState tells whether a conversation may be rated now and the rating given since its last
// close; since is when the inbox started asking for ratings, nil when it does not.
func ratingState(since *time.Time, status string, closedAt, ratedAt *time.Time, rating *string, now time.Time) (bool, *oas.Rating) {
	var current *oas.Rating
	rated := closedAt != nil && ratedAt != nil && !ratedAt.Before(*closedAt)
	if rated && status == string(oas.ConversationStatusClosed) && rating != nil {
		current = (*oas.Rating)(rating)
	}
	can := since != nil && status == string(oas.ConversationStatusClosed) && closedAt != nil && !closedAt.Before(*since) &&
		!rated && now.Sub(*closedAt) <= ratingWindow
	return can, current
}

func conversationRating(c store.Conversation) *oas.ConversationRating {
	if c.Rating == nil || c.RatedAt == nil {
		return nil
	}
	return &oas.ConversationRating{Rating: oas.Rating(*c.Rating), Comment: c.RatingComment, RatedAt: *c.RatedAt}
}

// rate records the contact's rating of the conversation's current close and its `rated` event;
// the conversation must be locked.
func (s *Server) rate(ctx context.Context, q *store.Queries, events *eventBatch, in store.Inbox, c store.Conversation, rating oas.Rating, comment string) (store.Conversation, error) {
	now := s.now()
	if !rating.Valid() {
		return c, errValidation("rating must be good or bad")
	}
	comment = strings.TrimSpace(comment)
	if len([]rune(comment)) > maxRatingComment {
		return c, errValidation("comment must be at most 2000 characters")
	}
	can, current := ratingState(in.RatingSince, c.Status, c.ClosedAt, c.RatedAt, c.Rating, now)
	if current != nil {
		return c, errAlreadyRated
	}
	if !can || c.Spam {
		return c, errRatingUnavailable
	}
	var stored *string
	if comment != "" {
		stored = &comment
	}
	updated, err := q.RateConversation(ctx, store.RateConversationParams{
		WorkspaceID: c.WorkspaceID, ID: c.ID, Rating: string(rating), Comment: stored, Now: now, ClosedAt: c.ClosedAt,
	})
	if store.IsNotFound(err) {
		return c, errAlreadyRated
	}
	if err != nil {
		return c, err
	}
	msg, err := q.CreateMessage(ctx, store.CreateMessageParams{
		ID: newID(), WorkspaceID: c.WorkspaceID, ConversationID: c.ID, Kind: string(oas.MessageKindEvent),
		AuthorType: string(oas.AuthorTypeContact), AuthorContactID: &c.ContactID, Body: comment,
		Event: mustJSON(oas.MessageEvent{Type: oas.Rated, Rating: &rating}), CreatedAt: now,
	})
	if err != nil {
		return c, err
	}
	body, err := oneConversation(ctx, q, updated)
	if err != nil {
		return c, err
	}
	events.conversation(realtime.ConversationUpdated, updated, body)
	events.conversation(realtime.MessageCreated, updated, messageBody(msg, nil))
	return updated, nil
}

func (s *Server) RateClientConversation(ctx context.Context, req oas.RateClientConversationRequestObject) (oas.RateClientConversationResponseObject, error) {
	cp := contactFrom(ctx)
	comment := ""
	if req.Body.Comment != nil {
		comment = *req.Body.Comment
	}
	var updated store.Conversation
	err := s.inTx(ctx, cp.workspaceID, func(q *store.Queries, events *eventBatch) error {
		c, err := clientConversation(ctx, q, cp, req.ConversationId, true)
		if err != nil {
			return err
		}
		in, err := q.GetInbox(ctx, store.GetInboxParams{WorkspaceID: cp.workspaceID, ID: c.InboxID})
		if err != nil {
			return err
		}
		updated, err = s.rate(ctx, q, events, in, c, req.Body.Rating, comment)
		return err
	})
	if err != nil {
		return nil, err
	}
	items, err := clientConversationItems(ctx, s.st.Queries, cp, []store.Conversation{updated}, s.now())
	if err != nil {
		return nil, err
	}
	return oas.RateClientConversation201JSONResponse(items[0]), nil
}
