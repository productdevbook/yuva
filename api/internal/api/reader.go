package api

import (
	"context"
	"strings"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	readerTokenTTL          = 24 * time.Hour
	readerNotificationsKept = 90 * 24 * time.Hour
	excerptRunes            = 200
)

func readerProfile(name string) oas.ReaderProfile {
	out := oas.ReaderProfile{Initials: initials(name)}
	if n := strings.TrimSpace(name); n != "" {
		out.Name = &n
	}
	return out
}

func (s *Server) readerAccount(ctx context.Context, personID uuid.UUID) (oas.ReaderAccount, error) {
	p, err := s.st.GetPerson(ctx, personID)
	if err != nil {
		return oas.ReaderAccount{}, err
	}
	member, err := s.st.PersonIsMember(ctx, personID)
	if err != nil {
		return oas.ReaderAccount{}, err
	}
	return oas.ReaderAccount{
		PersonId: p.ID, Email: oas.Email(p.Email), Name: p.Name, Initials: initials(p.Name), Locale: oas.Locale(p.Locale),
		EmailNotifications: p.ReaderEmailNotifications, Member: member,
	}, nil
}

func (s *Server) GetReaderAccount(ctx context.Context, _ oas.GetReaderAccountRequestObject) (oas.GetReaderAccountResponseObject, error) {
	out, err := s.readerAccount(ctx, principalFrom(ctx).personID)
	if err != nil {
		return nil, err
	}
	return oas.GetReaderAccount200JSONResponse(out), nil
}

func (s *Server) UpdateReaderAccount(ctx context.Context, req oas.UpdateReaderAccountRequestObject) (oas.UpdateReaderAccountResponseObject, error) {
	b, personID := req.Body, principalFrom(ctx).personID
	if b.Name == nil && b.EmailNotifications == nil {
		return nil, errValidation("send name or email_notifications")
	}
	if b.Name != nil {
		name, err := trimmed(*b.Name, 0, 200, "name")
		if err != nil {
			return nil, err
		}
		if _, err := s.st.UpdatePerson(ctx, store.UpdatePersonParams{ID: personID, Name: &name}); err != nil {
			return nil, err
		}
	}
	if b.EmailNotifications != nil {
		if err := s.st.SetPersonReaderSettings(ctx, store.SetPersonReaderSettingsParams{ID: personID, ReaderEmailNotifications: *b.EmailNotifications}); err != nil {
			return nil, err
		}
	}
	out, err := s.readerAccount(ctx, personID)
	if err != nil {
		return nil, err
	}
	return oas.UpdateReaderAccount200JSONResponse(out), nil
}

func (s *Server) CreateReaderToken(ctx context.Context, req oas.CreateReaderTokenRequestObject) (oas.CreateReaderTokenResponseObject, error) {
	p := principalFrom(ctx)
	key := strings.TrimSpace(req.Body.ChannelKey)
	if key == "" || len(key) > 200 {
		return nil, errUnknownChannelKey
	}
	ch, err := s.st.FindChatChannelByKey(ctx, key)
	if store.IsNotFound(err) || (err == nil && ch.Kind != string(oas.ChannelKindChat)) {
		return nil, errUnknownChannelKey
	}
	if err != nil {
		return nil, err
	}
	if err := p.ssoScope(&ch.WorkspaceID); err != nil {
		return nil, err
	}
	origin, ok := normalizeOrigin(string(req.Body.Origin))
	if !ok || !s.originAllowedFor(ctx, ch.WorkspaceID, origin, ch.Kind, ch.AllowedOrigins) {
		return nil, errOriginRefused
	}
	if ch.Discussions == string(oas.DiscussionModeOff) {
		if _, err := s.inboxBoard(ctx, s.st.Queries, ch.WorkspaceID, ch.InboxID); err != nil {
			if err == errRoadmapNoBoard {
				return nil, errDiscussionsOff
			}
			return nil, err
		}
	}
	person, err := s.st.GetPerson(ctx, p.personID)
	if err != nil {
		return nil, err
	}
	now := s.now()
	token := readerTokenPrefix + randomToken(32)
	expires := now.Add(readerTokenTTL)
	if err := s.st.CreateReaderToken(ctx, store.CreateReaderTokenParams{
		ID: newID(), WorkspaceID: ch.WorkspaceID, ChannelID: ch.ChannelID, PersonID: p.personID, SessionID: p.sessionID,
		TokenHash: hashSecret(token), Origin: origin, Now: now, ExpiresAt: expires,
	}); err != nil {
		return nil, err
	}
	return oas.CreateReaderToken201JSONResponse{
		Token: token, ExpiresAt: expires, Origin: oas.Origin(origin), ChannelKey: key, Reader: readerProfile(person.Name),
	}, nil
}

func containsString(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func (s *Server) readerDiscussions(ctx context.Context, following bool, cursor *string, limit *int32) (oas.ReaderDiscussionPage, error) {
	lim, err := pageSize(limit)
	if err != nil {
		return oas.ReaderDiscussionPage{}, err
	}
	at, cid, err := decodeCursor(cursor)
	if err != nil {
		return oas.ReaderDiscussionPage{}, err
	}
	rows, err := s.st.ListPersonDiscussions(ctx, store.ListPersonDiscussionsParams{
		PersonID: new(principalFrom(ctx).personID), FollowingOnly: following, CursorAt: at, CursorID: cid, Lim: lim + 1,
	})
	if err != nil {
		return oas.ReaderDiscussionPage{}, err
	}
	out := oas.ReaderDiscussionPage{Items: make([]oas.ReaderDiscussion, 0, len(rows))}
	if len(rows) > int(lim) {
		rows = rows[:lim]
		c := encodeCursor(rows[lim-1].LastActivityAt, rows[lim-1].ID)
		out.NextCursor = &c
	}
	for _, r := range rows {
		item := oas.ReaderDiscussion{
			Id: r.ID, WorkspaceName: r.WorkspaceName, InboxName: r.InboxName, Page: r.Page, Title: r.Title,
			State: oas.DiscussionState(r.State), Locked: r.LockedAt != nil, Answered: r.AnswerPostID != nil, ReplyCount: r.ReplyCount,
			Votes: r.Votes, Started: r.Started, Following: r.Following, CreatedAt: r.CreatedAt, LastActivityAt: r.LastActivityAt,
		}
		if r.PageTitle != "" {
			t := r.PageTitle
			item.PageTitle = &t
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}

func (s *Server) ListReaderDiscussions(ctx context.Context, req oas.ListReaderDiscussionsRequestObject) (oas.ListReaderDiscussionsResponseObject, error) {
	out, err := s.readerDiscussions(ctx, false, req.Params.Cursor, req.Params.Limit)
	if err != nil {
		return nil, err
	}
	return oas.ListReaderDiscussions200JSONResponse(out), nil
}

func (s *Server) ListReaderFollowing(ctx context.Context, req oas.ListReaderFollowingRequestObject) (oas.ListReaderFollowingResponseObject, error) {
	out, err := s.readerDiscussions(ctx, true, req.Params.Cursor, req.Params.Limit)
	if err != nil {
		return nil, err
	}
	return oas.ListReaderFollowing200JSONResponse(out), nil
}

func (s *Server) UnfollowReaderDiscussion(ctx context.Context, req oas.UnfollowReaderDiscussionRequestObject) (oas.UnfollowReaderDiscussionResponseObject, error) {
	personID := principalFrom(ctx).personID
	n, err := s.st.UnfollowByPerson(ctx, store.UnfollowByPersonParams{PersonID: &personID, DiscussionID: req.DiscussionId})
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, errDiscussionGone
	}
	return oas.UnfollowReaderDiscussion204Response{}, nil
}

func (s *Server) ListReaderNotifications(ctx context.Context, req oas.ListReaderNotificationsRequestObject) (oas.ListReaderNotificationsResponseObject, error) {
	personID := principalFrom(ctx).personID
	prm := req.Params
	lim, err := pageSize(prm.Limit)
	if err != nil {
		return nil, err
	}
	at, cid, err := decodeCursor(prm.Cursor)
	if err != nil {
		return nil, err
	}
	rows, err := s.st.ListReaderNotifications(ctx, store.ListReaderNotificationsParams{
		PersonID: personID, UnreadOnly: prm.Unread != nil && *prm.Unread, CursorAt: at, CursorID: cid, Lim: lim + 1,
	})
	if err != nil {
		return nil, err
	}
	unread, err := s.st.CountUnreadReaderNotifications(ctx, personID)
	if err != nil {
		return nil, err
	}
	out := oas.ReaderNotificationPage{Items: make([]oas.ReaderNotification, 0, len(rows)), Unread: unread}
	if len(rows) > int(lim) {
		rows = rows[:lim]
		c := encodeCursor(rows[lim-1].CreatedAt, rows[lim-1].ID)
		out.NextCursor = &c
	}
	for _, r := range rows {
		n := oas.ReaderNotification{
			Id: r.ID, Kind: oas.ReaderNotificationKind(r.Kind), DiscussionId: r.DiscussionID, PostId: r.PostID, Title: r.Title,
			Page: r.Page, InboxName: r.InboxName, CreatedAt: r.CreatedAt, ReadAt: r.ReadAt,
		}
		if r.PostID != nil {
			p, err := s.st.GetDiscussionPost(ctx, store.GetDiscussionPostParams{WorkspaceID: r.WorkspaceID, ID: *r.PostID})
			if err != nil && !store.IsNotFound(err) {
				return nil, err
			}
			if err == nil && p.State == statePublished {
				a := discussionAuthor(p, false)
				n.Author, n.Excerpt = &a, postExcerpt(p.Body)
			}
		}
		out.Items = append(out.Items, n)
	}
	return oas.ListReaderNotifications200JSONResponse(out), nil
}

func (s *Server) MarkReaderNotificationsRead(ctx context.Context, req oas.MarkReaderNotificationsReadRequestObject) (oas.MarkReaderNotificationsReadResponseObject, error) {
	arg := store.MarkReaderNotificationsReadParams{PersonID: principalFrom(ctx).personID, Now: s.now(), Ids: []uuid.UUID{}}
	if req.Body.Ids != nil {
		if len(*req.Body.Ids) > 100 {
			return nil, errValidation("ids may list at most 100 notifications")
		}
		arg.OnlyIds, arg.Ids = true, *req.Body.Ids
	}
	if err := s.st.MarkReaderNotificationsRead(ctx, arg); err != nil {
		return nil, err
	}
	return oas.MarkReaderNotificationsRead204Response{}, nil
}

// forgetReader removes a person's reader traces from every workspace before the person goes: their
// posts lose their author, votes and follows go, and contacts that existed only for them are deleted.
func (s *Server) forgetReader(ctx context.Context, q *store.Queries, personID uuid.UUID) error {
	contacts, err := q.ListPersonContacts(ctx, &personID)
	if err != nil {
		return err
	}
	for _, c := range contacts {
		ids, err := q.ForgetContactDiscussions(ctx, store.ForgetContactDiscussionsParams{WorkspaceID: c.WorkspaceID, ID: &c.ID})
		if err != nil {
			return err
		}
		removable, err := q.ReaderContactRemovable(ctx, store.ReaderContactRemovableParams{WorkspaceID: c.WorkspaceID, ID: c.ID})
		if err != nil {
			return err
		}
		if removable {
			if err := s.rememberContactObjections(ctx, q, c.WorkspaceID, c.ID); err != nil {
				return err
			}
			if err := deleteContactAnalytics(ctx, q, c.WorkspaceID, c.ID); err != nil {
				return err
			}
			_, err = q.DeleteContact(ctx, store.DeleteContactParams{WorkspaceID: c.WorkspaceID, ID: c.ID})
		} else {
			err = q.ClearContactPerson(ctx, store.ClearContactPersonParams{WorkspaceID: c.WorkspaceID, ID: c.ID})
		}
		if err != nil {
			return err
		}
		if err := recount(ctx, q, c.WorkspaceID, ids...); err != nil {
			return err
		}
	}
	return nil
}
