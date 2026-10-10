package api

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"uuid"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/productdevbook/yuva/api/internal/mail"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	maxPageURL          = 2000
	maxPageTitle        = 300
	pageRatingTakeBack  = 90
	pageAnswersMaxAge   = "public, max-age=60"
	defaultDocsDays     = 30
	maxPublishedAnswers = 20000
)

var (
	errPageOrigin       = problem(http.StatusBadRequest, "page_origin", "the page's origin is not one of the channel's allowed origins")
	errQuestionChannel  = problem(http.StatusForbidden, "forbidden", "questions are asked from chat channels")
	errPageAnswerGone   = problem(http.StatusNotFound, "not_found", "no such published answer")
	errAlreadyPublished = problem(http.StatusConflict, "already_published", "this conversation is already published; edit the published answer")
	errNotQuestion      = problem(http.StatusConflict, "not_publishable", "only question conversations are published")
	errNoMemberReply    = problem(http.StatusConflict, "not_publishable", "publish once a member has replied")
	errPageNotOnChannel = problem(http.StatusConflict, "not_publishable", "the conversation's channel does not serve the page's origin")
)

// pageURL is a documentation page as Yuva keys it: the URL without query and fragment, with the
// origin written as browsers send it.
func pageURL(raw string) (string, string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Opaque != "" {
		return "", "", errValidation("page must be an absolute http or https URL")
	}
	origin, ok := normalizeOrigin(u.Scheme + "://" + u.Host)
	if !ok {
		return "", "", errValidation("page must be an absolute http or https URL")
	}
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	page := origin + path
	if len(page) > maxPageURL {
		return "", "", errValidation("page must be at most 2000 characters")
	}
	return page, origin, nil
}

// channelPage is pageURL for a chat channel, whose allowed origins the page must be on.
func channelPage(raw, kind string, allowed []string) (string, error) {
	page, origin, err := pageURL(raw)
	if err != nil {
		return "", err
	}
	if kind != string(oas.ChannelKindChat) || !slices.Contains(allowed, origin) {
		return "", errPageOrigin
	}
	return page, nil
}

func pageTitle(v *string) (*string, error) {
	if v == nil {
		return nil, nil
	}
	t, err := trimmed(*v, 0, maxPageTitle, "page_title")
	if err != nil || t == "" {
		return nil, err
	}
	return &t, nil
}

type pageInput struct {
	url, title *string
	rating     *oas.PageRating
}

// apply puts the documentation page a piece of feedback was sent from into its metadata.
func (in pageInput) apply(cp contactPrincipal, fb *oas.Feedback) error {
	if in.url == nil || strings.TrimSpace(*in.url) == "" {
		if in.rating != nil || (in.title != nil && strings.TrimSpace(*in.title) != "") {
			return errValidation("page_title and rating need page_url")
		}
		return nil
	}
	page, err := channelPage(*in.url, cp.kind, cp.chat.AllowedOrigins)
	if err != nil {
		return err
	}
	title, err := pageTitle(in.title)
	if err != nil {
		return err
	}
	if in.rating != nil && !in.rating.Valid() {
		return errValidation("rating must be up or down")
	}
	fb.PageUrl, fb.PageTitle, fb.Rating = &page, title, in.rating
	return nil
}

func utcDay(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// docsChannel finds a chat channel by its public key for the session-less page endpoints and
// checks the browser's origin against it.
func (s *Server) docsChannel(ctx context.Context, key string, perChannel *limit) (store.FindChatChannelByKeyRow, error) {
	key = strings.TrimSpace(key)
	if key == "" || len(key) > 200 {
		return store.FindChatChannelByKeyRow{}, errUnknownChannelKey
	}
	ch, err := s.st.FindChatChannelByKey(ctx, key)
	if store.IsNotFound(err) || (err == nil && ch.Kind != string(oas.ChannelKindChat)) {
		return ch, errUnknownChannelKey
	}
	if err != nil {
		return ch, err
	}
	if perChannel != nil {
		if err := s.rateLimit(rateCheck{"page-rating:channel:" + ch.ChannelID.String(), *perChannel}); err != nil {
			return ch, err
		}
	}
	if !s.originAllowedFor(originFrom(ctx), ch.Kind, ch.AllowedOrigins) {
		return ch, errOriginRefused
	}
	return ch, nil
}

func (s *Server) CreatePageRating(ctx context.Context, req oas.CreatePageRatingRequestObject) (oas.CreatePageRatingResponseObject, error) {
	if err := s.rateLimit(rateCheck{"page-rating:ip:" + rateIP(s.clientIP(requestFrom(ctx))), limitSessionPerIP}); err != nil {
		return nil, err
	}
	ch, err := s.docsChannel(ctx, req.ChannelKey, &limitSessionPerChannel)
	if err != nil {
		return nil, err
	}
	b := req.Body
	if !b.Rating.Valid() || (b.Previous != nil && !b.Previous.Valid()) {
		return nil, errValidation("rating and previous must be up or down")
	}
	page, err := channelPage(b.Page, ch.Kind, ch.AllowedOrigins)
	if err != nil {
		return nil, err
	}
	title := ""
	if b.Title != nil {
		if title, err = trimmed(*b.Title, 0, maxPageTitle, "title"); err != nil {
			return nil, err
		}
	}
	if b.Previous != nil && *b.Previous == b.Rating {
		return oas.CreatePageRating204Response{}, nil
	}
	today := utcDay(s.now())
	err = s.inTx(ctx, ch.WorkspaceID, func(q *store.Queries, _ *eventBatch) error {
		if b.Previous != nil {
			if err := q.TakeBackPageRating(ctx, store.TakeBackPageRatingParams{
				WorkspaceID: ch.WorkspaceID, ChannelID: ch.ChannelID, Page: page, Up: *b.Previous == oas.Up,
				Since: today.AddDate(0, 0, -(pageRatingTakeBack - 1)),
			}); err != nil {
				return err
			}
		}
		arg := store.AddPageRatingParams{WorkspaceID: ch.WorkspaceID, InboxID: ch.InboxID, ChannelID: ch.ChannelID, Page: page, Title: title, Day: today}
		if b.Rating == oas.Up {
			arg.Up = 1
		} else {
			arg.Down = 1
		}
		return q.AddPageRating(ctx, arg)
	})
	if err != nil {
		return nil, err
	}
	return oas.CreatePageRating204Response{}, nil
}

func (s *Server) ListClientPageAnswers(ctx context.Context, req oas.ListClientPageAnswersRequestObject) (oas.ListClientPageAnswersResponseObject, error) {
	if err := s.rateLimit(rateCheck{"page-answers:ip:" + rateIP(s.clientIP(requestFrom(ctx))), limitChannelPerIP}); err != nil {
		return nil, err
	}
	ch, err := s.docsChannel(ctx, req.ChannelKey, nil)
	if err != nil {
		return nil, err
	}
	page, err := channelPage(req.Params.Page, ch.Kind, ch.AllowedOrigins)
	if err != nil {
		return nil, err
	}
	rows, err := s.st.ListClientPageAnswers(ctx, store.ListClientPageAnswersParams{WorkspaceID: ch.WorkspaceID, InboxID: ch.InboxID, Page: page})
	if err != nil {
		return nil, err
	}
	items := make([]oas.ClientPageAnswer, len(rows))
	for i, a := range rows {
		items[i] = oas.ClientPageAnswer{Id: a.ID, Question: a.Question, Answer: a.Answer, PublishedAt: a.PublishedAt, UpdatedAt: a.UpdatedAt}
	}
	cache := pageAnswersMaxAge
	return oas.ListClientPageAnswers200JSONResponse{
		Body:    oas.ClientPageAnswerList{Items: items},
		Headers: oas.ListClientPageAnswers200ResponseHeaders{CacheControl: &cache},
	}, nil
}

func (s *Server) CreateClientQuestion(ctx context.Context, req oas.CreateClientQuestionRequestObject) (oas.CreateClientQuestionResponseObject, error) {
	cp := contactFrom(ctx)
	if cp.kind != string(oas.ChannelKindChat) {
		return nil, errQuestionChannel
	}
	if err := s.rateLimit(s.writeChecks(ctx, cp)...); err != nil {
		return nil, err
	}
	b := req.Body
	in := &messageInput{body: b.Body, clientID: b.ClientId, subject: b.Subject}
	if err := s.validateClientInput(in); err != nil {
		return nil, err
	}
	page, err := channelPage(b.PageUrl, cp.kind, cp.chat.AllowedOrigins)
	if err != nil {
		return nil, err
	}
	title, err := pageTitle(b.PageTitle)
	if err != nil {
		return nil, err
	}
	var addr *string
	if b.Email != nil && strings.TrimSpace(string(*b.Email)) != "" {
		a, err := normalizeEmail(*b.Email)
		if err != nil {
			return nil, err
		}
		addr = &a
	}
	if in.clientID != nil {
		prev, err := s.st.FindContactMessageByClientID(ctx, store.FindContactMessageByClientIDParams{
			WorkspaceID: cp.workspaceID, InboxID: cp.inboxID, ContactID: cp.contactID, ClientID: *in.clientID,
		})
		if err == nil {
			out, err := s.clientConversationCreated(ctx, cp, prev.ConversationID, prev.ID)
			if err != nil {
				return nil, err
			}
			return oas.CreateClientQuestion201JSONResponse(out), nil
		}
		if !store.IsNotFound(err) {
			return nil, err
		}
	}
	var (
		convID, msgID uuid.UUID
		confirm       *mail.Message
	)
	err = s.inTx(ctx, cp.workspaceID, func(q *store.Queries, events *eventBatch) error {
		if addr != nil {
			r, err := q.SetContactTypedEmail(ctx, store.SetContactTypedEmailParams{WorkspaceID: cp.workspaceID, ID: cp.contactID, TypedEmail: addr, Now: s.now()})
			if err != nil {
				return err
			}
			if confirm, err = s.confirmTypedEmail(ctx, q, cp, *addr); err != nil {
				return err
			}
			body, err := s.contactBody(ctx, q, cp.workspaceID, contactRow(r))
			if err != nil {
				return err
			}
			events.add(realtime.ContactUpdated, nil, nil, body)
		}
		subject := ""
		if in.subject != nil {
			subject = *in.subject
		}
		kind, channelID := string(oas.ConversationKindQuestion), cp.channelID
		c, err := q.CreateConversation(ctx, store.CreateConversationParams{
			ID: newID(), WorkspaceID: cp.workspaceID, InboxID: cp.inboxID, ContactID: cp.contactID, ChannelID: &channelID,
			Subject: subject, Priority: string(oas.Normal), Kind: &kind, PageUrl: &page, PageTitle: title, Now: s.now(),
		})
		if err != nil {
			return err
		}
		events.conversation(realtime.ConversationCreated, c, conversationBody(c, nil))
		msg, _, err := s.addContactMessage(ctx, q, events, cp, c, in, true)
		convID, msgID = c.ID, msg.ID
		return err
	})
	if err != nil {
		return nil, err
	}
	if confirm != nil {
		s.sendMail(ctx, *confirm)
	}
	out, err := s.clientConversationCreated(ctx, cp, convID, msgID)
	if err != nil {
		return nil, err
	}
	return oas.CreateClientQuestion201JSONResponse(out), nil
}

func conversationQuestion(c store.Conversation) *oas.PageQuestion {
	if c.Kind != string(oas.ConversationKindQuestion) || c.PageUrl == nil {
		return nil
	}
	return &oas.PageQuestion{PageUrl: *c.PageUrl, PageTitle: c.PageTitle}
}

func pageAnswerBody(a store.PageAnswer) oas.PageAnswer {
	return oas.PageAnswer{
		Id: a.ID, InboxId: a.InboxID, ChannelId: a.ChannelID, Page: a.Page, Title: a.Title, Question: a.Question, Answer: a.Answer,
		MemberId: a.MemberID, ConversationId: a.ConversationID, PublishedAt: a.PublishedAt, UpdatedAt: a.UpdatedAt,
	}
}

func publishedText(question, answer *string) (string, string, error) {
	var q, a string
	var err error
	if question != nil {
		if q, err = trimmed(*question, 1, 2000, "question"); err != nil {
			return q, a, err
		}
	}
	if answer != nil {
		if a, err = trimmed(*answer, 1, maxPublishedAnswers, "answer"); err != nil {
			return q, a, err
		}
	}
	return q, a, nil
}

func (s *Server) PublishConversation(ctx context.Context, req oas.PublishConversationRequestObject) (oas.PublishConversationResponseObject, error) {
	p := principalFrom(ctx)
	question, answer, err := publishedText(&req.Body.Question, &req.Body.Answer)
	if err != nil {
		return nil, err
	}
	var out store.PageAnswer
	err = s.inTx(ctx, p.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		c, err := visibleConversation(ctx, q, p, req.ConversationId, true)
		if err != nil {
			return err
		}
		if c.Kind != string(oas.ConversationKindQuestion) || c.PageUrl == nil {
			return errNotQuestion
		}
		replied, err := q.ConversationHasMemberReply(ctx, store.ConversationHasMemberReplyParams{WorkspaceID: p.workspaceID, ConversationID: c.ID})
		if err != nil {
			return err
		}
		if !replied {
			return errNoMemberReply
		}
		if c.ChannelID == nil {
			return errPageNotOnChannel
		}
		ch, err := q.GetSessionChannel(ctx, store.GetSessionChannelParams{WorkspaceID: p.workspaceID, ChannelID: *c.ChannelID})
		if store.IsNotFound(err) {
			return errPageNotOnChannel
		}
		if err != nil {
			return err
		}
		page, err := channelPage(*c.PageUrl, ch.Kind, ch.AllowedOrigins)
		if err != nil {
			return errPageNotOnChannel
		}
		title := ""
		if c.PageTitle != nil {
			title = *c.PageTitle
		}
		memberID, convID := p.memberID, c.ID
		out, err = q.CreatePageAnswer(ctx, store.CreatePageAnswerParams{
			ID: newID(), WorkspaceID: p.workspaceID, InboxID: c.InboxID, ChannelID: ch.ChannelID, Page: page, Title: title,
			Question: question, Answer: answer, MemberID: &memberID, ConversationID: &convID, Now: s.now(),
		})
		if store.IsUniqueViolation(err) {
			return errAlreadyPublished
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return oas.PublishConversation201JSONResponse(pageAnswerBody(out)), nil
}

func visiblePageAnswer(ctx context.Context, q *store.Queries, p principal, id uuid.UUID) (store.PageAnswer, error) {
	a, err := q.GetPageAnswer(ctx, store.GetPageAnswerParams{WorkspaceID: p.workspaceID, ID: id})
	if store.IsNotFound(err) {
		return a, errPageAnswerGone
	}
	if err != nil {
		return a, err
	}
	ok, err := canSeeInbox(ctx, q, p, a.InboxID)
	if err != nil {
		return a, err
	}
	if !ok {
		return a, errPageAnswerGone
	}
	return a, nil
}

func (s *Server) ListPageAnswers(ctx context.Context, req oas.ListPageAnswersRequestObject) (oas.ListPageAnswersResponseObject, error) {
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
	arg := store.ListPageAnswersParams{
		WorkspaceID: p.workspaceID, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(),
		InboxID: prm.InboxId, ConversationID: prm.ConversationId, CursorAt: at, CursorID: cid, Lim: lim + 1,
	}
	if prm.InboxId != nil {
		if _, err := visibleInbox(ctx, s.st.Queries, p, *prm.InboxId); err != nil {
			return nil, err
		}
	}
	if prm.Page != nil {
		page, _, err := pageURL(*prm.Page)
		if err != nil {
			return nil, err
		}
		arg.Page = &page
	}
	rows, err := s.st.ListPageAnswers(ctx, arg)
	if err != nil {
		return nil, err
	}
	var next *string
	if len(rows) > int(lim) {
		rows = rows[:lim]
		c := encodeCursor(rows[lim-1].PublishedAt, rows[lim-1].ID)
		next = &c
	}
	items := make([]oas.PageAnswer, len(rows))
	for i, a := range rows {
		items[i] = pageAnswerBody(a)
	}
	return oas.ListPageAnswers200JSONResponse{Items: items, NextCursor: next}, nil
}

func (s *Server) GetPageAnswer(ctx context.Context, req oas.GetPageAnswerRequestObject) (oas.GetPageAnswerResponseObject, error) {
	a, err := visiblePageAnswer(ctx, s.st.Queries, principalFrom(ctx), req.PageAnswerId)
	if err != nil {
		return nil, err
	}
	return oas.GetPageAnswer200JSONResponse(pageAnswerBody(a)), nil
}

func (s *Server) UpdatePageAnswer(ctx context.Context, req oas.UpdatePageAnswerRequestObject) (oas.UpdatePageAnswerResponseObject, error) {
	p := principalFrom(ctx)
	if req.Body.Question == nil && req.Body.Answer == nil {
		return nil, errValidation("send question or answer")
	}
	question, answer, err := publishedText(req.Body.Question, req.Body.Answer)
	if err != nil {
		return nil, err
	}
	var out store.PageAnswer
	err = s.inTx(ctx, p.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		a, err := visiblePageAnswer(ctx, q, p, req.PageAnswerId)
		if err != nil {
			return err
		}
		if req.Body.Question == nil {
			question = a.Question
		}
		if req.Body.Answer == nil {
			answer = a.Answer
		}
		out, err = q.UpdatePageAnswer(ctx, store.UpdatePageAnswerParams{WorkspaceID: p.workspaceID, ID: a.ID, Question: question, Answer: answer, Now: s.now()})
		if store.IsNotFound(err) {
			return errPageAnswerGone
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return oas.UpdatePageAnswer200JSONResponse(pageAnswerBody(out)), nil
}

func (s *Server) DeletePageAnswer(ctx context.Context, req oas.DeletePageAnswerRequestObject) (oas.DeletePageAnswerResponseObject, error) {
	p := principalFrom(ctx)
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		a, err := visiblePageAnswer(ctx, q, p, req.PageAnswerId)
		if err != nil {
			return err
		}
		n, err := q.DeletePageAnswer(ctx, store.DeletePageAnswerParams{WorkspaceID: p.workspaceID, ID: a.ID})
		if err == nil && n == 0 {
			return errPageAnswerGone
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return oas.DeletePageAnswer204Response{}, nil
}

func docsSince(s *Server, days *int32) (time.Time, error) {
	d := int32(defaultDocsDays)
	if days != nil {
		d = *days
	}
	if d != 7 && d != 30 && d != 90 {
		return time.Time{}, errValidation("days must be 7, 30 or 90")
	}
	return utcDay(s.now()).AddDate(0, 0, -int(d-1)), nil
}

func encodeOffset(n int32) string {
	return base64.RawURLEncoding.EncodeToString([]byte("o" + strconv.Itoa(int(n))))
}

func decodeOffset(s *string) (int32, error) {
	if s == nil || *s == "" {
		return 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(*s)
	if err != nil || len(raw) < 2 || raw[0] != 'o' {
		return 0, errInvalidCursor
	}
	n, err := strconv.ParseInt(string(raw[1:]), 10, 32)
	if err != nil || n < 0 {
		return 0, errInvalidCursor
	}
	return int32(n), nil
}

func docsPageBody(r store.ListDocsPagesRow) oas.DocsPage {
	return oas.DocsPage{
		InboxId: r.InboxID, Page: r.Page, Title: r.Title, Up: r.Up, Down: r.Down,
		OpenFeedback: r.OpenFeedback, OpenQuestions: r.OpenQuestions, PublishedAnswers: r.PublishedAnswers,
	}
}

func (s *Server) ListDocsPages(ctx context.Context, req oas.ListDocsPagesRequestObject) (oas.ListDocsPagesResponseObject, error) {
	p := principalFrom(ctx)
	prm := req.Params
	lim, err := pageSize(prm.Limit)
	if err != nil {
		return nil, err
	}
	off, err := decodeOffset(prm.Cursor)
	if err != nil {
		return nil, err
	}
	since, err := docsSince(s, prm.Days)
	if err != nil {
		return nil, err
	}
	if prm.InboxId != nil {
		if _, err := visibleInbox(ctx, s.st.Queries, p, *prm.InboxId); err != nil {
			return nil, err
		}
	}
	rows, err := s.st.ListDocsPages(ctx, store.ListDocsPagesParams{
		WorkspaceID: p.workspaceID, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(), InboxID: prm.InboxId,
		Since: since, Lim: lim + 1, Off: off,
	})
	if err != nil {
		return nil, err
	}
	var next *string
	if len(rows) > int(lim) {
		rows = rows[:lim]
		c := encodeOffset(off + lim)
		next = &c
	}
	items := make([]oas.DocsPage, len(rows))
	for i, r := range rows {
		items[i] = docsPageBody(r)
	}
	return oas.ListDocsPages200JSONResponse{Items: items, NextCursor: next}, nil
}

func (s *Server) GetDocsPage(ctx context.Context, req oas.GetDocsPageRequestObject) (oas.GetDocsPageResponseObject, error) {
	p := principalFrom(ctx)
	prm := req.Params
	since, err := docsSince(s, prm.Days)
	if err != nil {
		return nil, err
	}
	page, _, err := pageURL(prm.Page)
	if err != nil {
		return nil, err
	}
	if _, err := visibleInbox(ctx, s.st.Queries, p, prm.InboxId); err != nil {
		return nil, err
	}
	inboxID := prm.InboxId
	rows, err := s.st.ListDocsPages(ctx, store.ListDocsPagesParams{
		WorkspaceID: p.workspaceID, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(), InboxID: &inboxID,
		Page: &page, Since: since, Lim: 1,
	})
	if err != nil {
		return nil, err
	}
	out := oas.DocsPageDetail{Page: oas.DocsPage{InboxId: inboxID, Page: page}, Days: []oas.DocsPageDay{}}
	if len(rows) == 1 {
		out.Page = docsPageBody(rows[0])
	}
	days, err := s.st.ListPageRatingDays(ctx, store.ListPageRatingDaysParams{WorkspaceID: p.workspaceID, InboxID: inboxID, Page: page, Since: since})
	if err != nil {
		return nil, err
	}
	for _, d := range days {
		out.Days = append(out.Days, oas.DocsPageDay{Day: openapi_types.Date{Time: d.Day}, Up: d.Up, Down: d.Down})
	}
	return oas.GetDocsPage200JSONResponse(out), nil
}
