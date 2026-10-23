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

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	maxPageURL         = 2000
	maxPageTitle       = 300
	pageRatingTakeBack = 90
	publicMaxAge       = "public, max-age=60"
	defaultDocsDays    = 30
	helpfulMinRatings  = 5
)

var errPageOrigin = problem(http.StatusBadRequest, "page_origin", "the page's origin is not one of the channel's allowed origins")

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
	if !s.originAllowedFor(ctx, ch.WorkspaceID, originFrom(ctx), ch.Kind, ch.AllowedOrigins) {
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
				WorkspaceID: ch.WorkspaceID, ChannelID: ch.ChannelID, Page: page, Up: *b.Previous == oas.PageRatingUp,
				Since: today.AddDate(0, 0, -(pageRatingTakeBack - 1)),
			}); err != nil {
				return err
			}
		}
		arg := store.AddPageRatingParams{WorkspaceID: ch.WorkspaceID, InboxID: ch.InboxID, ChannelID: ch.ChannelID, Page: page, Title: title, Day: today}
		if b.Rating == oas.PageRatingUp {
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

func conversationPage(c store.Conversation) *oas.SourcePage {
	if c.PageUrl == nil {
		return nil
	}
	return &oas.SourcePage{PageUrl: *c.PageUrl, PageTitle: c.PageTitle}
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

// The pages list is an aggregate without a stable key, so its cursor is a position within one sort.
func encodeOffset(sort oas.DocsPageSort, n int32) string {
	return base64.RawURLEncoding.EncodeToString([]byte(string(sort) + "." + strconv.Itoa(int(n))))
}

func decodeOffset(sort oas.DocsPageSort, s *string) (int32, error) {
	if s == nil || *s == "" {
		return 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(*s)
	if err != nil {
		return 0, errInvalidCursor
	}
	got, num, ok := strings.Cut(string(raw), ".")
	if !ok || got != string(sort) {
		return 0, errInvalidCursor
	}
	n, err := strconv.ParseInt(num, 10, 32)
	if err != nil || n < 0 {
		return 0, errInvalidCursor
	}
	return int32(n), nil
}

func docsPageBody(r store.ListDocsPagesRow) oas.DocsPage {
	return oas.DocsPage{
		InboxId: r.InboxID, Page: r.Page, Title: r.Title, Up: r.Up, Down: r.Down,
		OpenFeedback: r.OpenFeedback, OpenThreads: r.OpenThreads, UnansweredThreads: r.UnansweredThreads, PendingPosts: r.PendingPosts,
	}
}

func (s *Server) ListDocsPages(ctx context.Context, req oas.ListDocsPagesRequestObject) (oas.ListDocsPagesResponseObject, error) {
	p := principalFrom(ctx)
	prm := req.Params
	lim, err := pageSize(prm.Limit)
	if err != nil {
		return nil, err
	}
	sort := oas.DocsPageSortDown
	if prm.Sort != nil {
		if !prm.Sort.Valid() {
			return nil, errValidation("sort must be down, up, helpful or activity")
		}
		sort = *prm.Sort
	}
	off, err := decodeOffset(sort, prm.Cursor)
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
	site, err := docsSite(prm.Site)
	if err != nil {
		return nil, err
	}
	rows, err := s.st.ListDocsPages(ctx, store.ListDocsPagesParams{
		WorkspaceID: p.workspaceID, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(), InboxID: prm.InboxId,
		Site: site, Since: since, Lim: lim + 1, Off: off, Sort: string(sort), HelpfulMin: helpfulMinRatings,
	})
	if err != nil {
		return nil, err
	}
	var next *string
	if len(rows) > int(lim) {
		rows = rows[:lim]
		c := encodeOffset(sort, off+lim)
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
	page, origin, err := pageURL(prm.Page)
	if err != nil {
		return nil, err
	}
	site, err := docsSite(prm.Site)
	if err != nil {
		return nil, err
	}
	if site != nil && *site != origin {
		return nil, errValidation("page is not on site")
	}
	if _, err := visibleInbox(ctx, s.st.Queries, p, prm.InboxId); err != nil {
		return nil, err
	}
	inboxID := prm.InboxId
	rows, err := s.st.ListDocsPages(ctx, store.ListDocsPagesParams{
		WorkspaceID: p.workspaceID, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(), InboxID: &inboxID,
		Page: &page, Since: since, Lim: 1, Sort: string(oas.DocsPageSortDown), HelpfulMin: helpfulMinRatings,
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

func (s *Server) GetDocsSummary(ctx context.Context, req oas.GetDocsSummaryRequestObject) (oas.GetDocsSummaryResponseObject, error) {
	p := principalFrom(ctx)
	prm := req.Params
	since, err := docsSince(s, prm.Days)
	if err != nil {
		return nil, err
	}
	if prm.InboxId != nil {
		if _, err := visibleInbox(ctx, s.st.Queries, p, *prm.InboxId); err != nil {
			return nil, err
		}
	}
	site, err := docsSite(prm.Site)
	if err != nil {
		return nil, err
	}
	days, err := s.st.ListDocsSummaryDays(ctx, store.ListDocsSummaryDaysParams{
		WorkspaceID: p.workspaceID, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(), InboxID: prm.InboxId,
		Site: site, Since: since, Until: utcDay(s.now()), SinceAt: since,
	})
	if err != nil {
		return nil, err
	}
	counts, err := s.st.GetDocsSummaryCounts(ctx, store.GetDocsSummaryCountsParams{
		WorkspaceID: p.workspaceID, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(), InboxID: prm.InboxId, Site: site,
		Since: since,
	})
	if err != nil {
		return nil, err
	}
	out := oas.DocsSummary{
		Totals: oas.DocsTotals{RatedPages: counts.RatedPages, PendingPosts: counts.PendingPosts},
		Days:   make([]oas.DocsSummaryDay, len(days)),
	}
	for i, d := range days {
		out.Days[i] = oas.DocsSummaryDay{Day: openapi_types.Date{Time: d.Day}, Up: d.Up, Down: d.Down, Feedback: d.Feedback, Threads: d.Threads}
		out.Totals.Up += d.Up
		out.Totals.Down += d.Down
		out.Totals.Feedback += d.Feedback
		out.Totals.Threads += d.Threads
	}
	return oas.GetDocsSummary200JSONResponse(out), nil
}

func docsSite(v *string) (*string, error) {
	if v == nil {
		return nil, nil
	}
	site, ok := normalizeOrigin(*v)
	if !ok {
		return nil, errValidation("site must be an origin, scheme://host[:port]")
	}
	return &site, nil
}

func (s *Server) ListDocsSites(ctx context.Context, req oas.ListDocsSitesRequestObject) (oas.ListDocsSitesResponseObject, error) {
	p := principalFrom(ctx)
	prm := req.Params
	since, err := docsSince(s, prm.Days)
	if err != nil {
		return nil, err
	}
	if prm.InboxId != nil {
		if _, err := visibleInbox(ctx, s.st.Queries, p, *prm.InboxId); err != nil {
			return nil, err
		}
	}
	rows, err := s.st.ListDocsSites(ctx, store.ListDocsSitesParams{
		WorkspaceID: p.workspaceID, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(), InboxID: prm.InboxId, Since: since,
	})
	if err != nil {
		return nil, err
	}
	items := make([]oas.DocsSite, len(rows))
	for i, r := range rows {
		items[i] = oas.DocsSite{
			Origin: r.Origin, InboxId: r.InboxID, ChannelId: r.ChannelID, ChannelName: r.ChannelName, Pages: r.Pages,
			Up: r.Up, Down: r.Down, OpenFeedback: r.OpenFeedback, OpenThreads: r.OpenThreads, UnansweredThreads: r.UnansweredThreads, PendingPosts: r.PendingPosts,
		}
	}
	return oas.ListDocsSites200JSONResponse{Items: items}, nil
}
