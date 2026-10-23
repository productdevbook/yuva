package api

import (
	"bytes"
	"context"
	"encoding/json"
	"html/template"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	roadmapCacheTTL     = time.Minute
	roadmapCacheMax     = 5000
	roadmapCacheControl = "public, max-age=60, stale-while-revalidate=300, stale-if-error=86400"
	roadmapColumnShown  = 50
	roadmapStageShown   = 200
	roadmapShippedDays  = 90
	roadmapReplyShown   = 200
	roadmapPageType     = "text/html; charset=utf-8"
)

var (
	roadmapStages  = []string{stageIdeas, stagePlanned, stageInProgress, stageShipped}
	errRoadmapGone = problem(http.StatusNotFound, "not_found", "no such roadmap page")
)

type roadmapCard struct {
	Title, URL, Excerpt, Votes, Comments, DateLabel string
	Date                                            timeView
}

type roadmapColumn struct {
	Name, More, MoreLabel string
	Cards                 []roadmapCard
}

type roadmapReply struct {
	Name, Initials string
	Body           template.HTML
	At             timeView
}

type roadmapHistory struct {
	Label string
	At    timeView
}

type roadmapIdeaView struct {
	ID, StageName, Votes, Comments string
	Body                           template.HTML
	History                        []roadmapHistory
	Replies                        []roadmapReply
}

type roadmapPageView struct {
	W                           roadmapWords
	SW                          statusWords
	Lang, Dir, View             string
	Title, OGTitle, Description string
	Canonical, OGType, OGLocale string
	NoIndex                     bool
	JSONLD                      template.HTML
	Name, Logo                  string
	Accent                      template.CSS
	FontSans, FontMono          string
	Home, H1, Lead              string
	Crumbs                      []helpCrumb
	Columns                     []roadmapColumn
	Idea                        *roadmapIdeaView
	Suggest                     bool
	Langs                       []helpLangLink
	Chat                        *helpChatView
	Mark                        template.HTML
	YuvaSite                    string
	Script                      template.JS
}

func (s *Server) routeRoadmap(mux *http.ServeMux) {
	mux.HandleFunc("GET "+roadmapPathPrefix+"{slug}/fonts/{name}", servePublicFont)
	mux.HandleFunc("GET "+roadmapPathPrefix+"{slug}", s.serveRoadmap)
	mux.HandleFunc("GET "+roadmapPathPrefix+"{slug}/{rest...}", s.serveRoadmap)
}

func roadmapGone(w http.ResponseWriter, locale string) {
	h := w.Header()
	h.Set("Content-Type", "text/plain; charset=utf-8")
	h.Set("Cache-Control", "public, max-age=30")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte(roadmapWordsFor(locale).Gone + "\n"))
}

// publicRoadmapBoard finds the board a public URL names. A private board, an unknown slug and a
// former slug of a private board all answer the same plain 404.
func (s *Server) publicRoadmapBoard(w http.ResponseWriter, r *http.Request) (store.RoadmapBoard, bool) {
	ctx := r.Context()
	slug := r.PathValue("slug")
	if s.roadmap.Off {
		roadmapGone(w, "en")
		return store.RoadmapBoard{}, false
	}
	if err := s.rateLimit(ctx, rateCheck{"roadmap:ip:" + rateIP(s.clientIP(r)), limitPublicPerIP}); err != nil {
		s.writeError(w, r, err)
		return store.RoadmapBoard{}, false
	}
	var b store.RoadmapBoard
	var err error
	moved := false
	if on, ok := ownDomainOn(ctx, kindRoadmap); ok {
		b, err = s.st.GetRoadmapBoard(ctx, store.GetRoadmapBoardParams{WorkspaceID: on.workspace, ID: on.page})
		moved = err == nil && !stillOn(b.CustomDomain, b.DomainState, on.host)
	} else if b, err = s.st.FindRoadmapBoardBySlug(ctx, slug); store.IsNotFound(err) {
		b, err = s.st.FindRoadmapBoardByFormerSlug(ctx, slug)
		if err == nil && b.Public {
			redirectPublic(w, r, s.roadmapURL(b)+strings.TrimPrefix(r.URL.Path, roadmapPathPrefix+slug), true)
			return b, false
		}
	}
	if moved || store.IsNotFound(err) || (err == nil && !b.Public) {
		roadmapGone(w, "en")
		return b, false
	}
	if err != nil {
		s.log.ErrorContext(ctx, "roadmap board", slog.Any("error", err))
		writeProblem(w, errInternal)
		return b, false
	}
	return b, true
}

func (s *Server) roadmapPageCSP(b store.RoadmapBoard, logo string, chat bool) string {
	imgs := []string{"'self'"}
	if o := originOf(logo); o != "" {
		imgs = append(imgs, o)
	}
	csp := publicCSP{scriptHash: publicScriptHash(roadmapScript), frameAncestors: "'none'", formAction: "'self'", connect: "'self'"}
	if chat {
		pub := originOf(s.auth.PublicURL)
		csp.script = pub
		csp.connect = "'self' " + pub + " " + strings.Replace(pub, "http", "ws", 1)
		imgs = append(imgs, "https:", "data:")
	}
	csp.img = strings.Join(imgs, " ")
	return csp.String()
}

type roadmapRoute struct {
	view  string
	idea  uuid.UUID
	stage string
}

func parseRoadmapRoute(rest, stage string) (roadmapRoute, bool) {
	rest = strings.Trim(rest, "/")
	if rest == "" {
		if stage == stageShipped {
			return roadmapRoute{view: "stage", stage: stage}, true
		}
		return roadmapRoute{view: "home"}, true
	}
	parts := strings.Split(rest, "/")
	if len(parts) == 2 && parts[0] == "ideas" {
		id, err := uuid.Parse(parts[1])
		return roadmapRoute{view: "idea", idea: id}, err == nil
	}
	return roadmapRoute{}, false
}

func (s *Server) serveRoadmap(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	b, ok := s.publicRoadmapBoard(w, r)
	if !ok {
		return
	}
	rt, ok := parseRoadmapRoute(r.PathValue("rest"), r.URL.Query().Get("stage"))
	if !ok {
		roadmapGone(w, "en")
		return
	}
	in, err := s.st.GetInbox(ctx, store.GetInboxParams{WorkspaceID: b.WorkspaceID, ID: b.InboxID})
	if err != nil {
		s.log.ErrorContext(ctx, "roadmap inbox", slog.Any("error", err))
		writeProblem(w, errInternal)
		return
	}
	locale := in.DefaultLocale
	if code := r.URL.Query().Get("lang"); code != "" {
		if sw, ok := statusLanguage(code); ok {
			locale = sw.Lang
		}
	}
	key := b.ID.String() + "|" + rt.view + "|" + rt.idea.String() + "|" + rt.stage + "|" + statusWordsFor(locale).Lang
	now := s.now()
	e, hit := s.roadmapCache.get(key, b.ChangedAt, now)
	if !hit {
		body, err := s.roadmapBody(ctx, b, in, rt, locale, !s.noWidget)
		if err == errRoadmapGone {
			roadmapGone(w, locale)
			return
		}
		if err != nil {
			s.log.ErrorContext(ctx, "roadmap page", slog.Any("error", err))
			writeProblem(w, errInternal)
			return
		}
		e = publicEntry{changed: b.ChangedAt, at: now, ctype: roadmapPageType, body: body}
		s.roadmapCache.put(key, e)
	}
	chat := !s.noWidget
	writePublicAnswer(w, r, e, roadmapCacheControl, s.roadmapPageCSP(b, inboxLogo(in), chat))
}

func inboxLogo(in store.Inbox) string {
	var br oas.InboxBranding
	if json.Unmarshal(in.Branding, &br) == nil && br.LogoUrl != nil {
		return *br.LogoUrl
	}
	return ""
}

func (s *Server) roadmapBody(ctx context.Context, b store.RoadmapBoard, in store.Inbox, rt roadmapRoute, locale string, chat bool) ([]byte, error) {
	v, err := s.roadmapBaseView(ctx, b, in, locale, rt.view, chat)
	if err != nil {
		return nil, err
	}
	switch rt.view {
	case "home":
		err = s.roadmapHomeView(ctx, b, v)
	case "stage":
		err = s.roadmapStageView(ctx, b, v, rt.stage)
	default:
		err = s.roadmapIdeaView(ctx, b, v, rt.idea)
	}
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := roadmapTemplate.Execute(&buf, v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (s *Server) roadmapLangHref(b store.RoadmapBoard, path, code string) string {
	return s.roadmapURL(b) + path + "?lang=" + code
}

func (s *Server) roadmapBaseView(ctx context.Context, b store.RoadmapBoard, in store.Inbox, locale, view string, chat bool) (*roadmapPageView, error) {
	sw := statusWordsFor(locale)
	v := &roadmapPageView{
		W: roadmapWordsFor(locale), SW: sw, Lang: sw.Lang, Dir: sw.direction(), View: view, Name: b.Name, OGLocale: strings.ReplaceAll(sw.Lang, "-", "_"),
		OGType: "website", FontSans: s.roadmapURL(b) + publicFontPath("Geist"), FontMono: s.roadmapURL(b) + publicFontPath("GeistMono"),
		Home: s.roadmapURL(b), Mark: yuvaMark, YuvaSite: yuvaSite, Script: template.JS(roadmapScript), Logo: inboxLogo(in),
		Suggest: b.Suggestions != string(oas.RoadmapSuggestionsOff),
	}
	var br oas.InboxBranding
	if json.Unmarshal(in.Branding, &br) == nil && br.Color != nil && colorPattern.MatchString(*br.Color) {
		v.Accent = template.CSS(*br.Color)
	}
	if chat {
		ch, err := s.st.GetChatChannel(ctx, store.GetChatChannelParams{WorkspaceID: b.WorkspaceID, ChannelID: b.ChannelID})
		if err != nil && !store.IsNotFound(err) {
			return nil, err
		}
		if err == nil {
			v.Chat = &helpChatView{Script: s.auth.PublicURL + "/yuva.js", Server: s.auth.PublicURL, Key: ch.PublicKey}
		}
	}
	for _, code := range []string{"en", "tr", "de"} {
		v.Langs = append(v.Langs, helpLangLink{Code: code, Label: helpLanguageName(code), Href: "?lang=" + code, On: code == sw.Lang})
	}
	return v, nil
}

func (s *Server) roadmapCards(w roadmapWords, sw statusWords, rows []store.ListPublicIdeasRow, b store.RoadmapBoard) []roadmapCard {
	out := make([]roadmapCard, len(rows))
	for i, r := range rows {
		c := roadmapCard{
			Title: r.Title, URL: s.roadmapURL(b) + "/ideas/" + r.ID.String(), Excerpt: postExcerpt(r.Body),
			Votes: w.votes(r.VoteCount), Comments: w.comments(r.ReplyCount), DateLabel: w.stageOn(r.Stage),
		}
		at := r.CreatedAt
		if r.StageChangedAt != nil {
			at = *r.StageChangedAt
		}
		c.Date = timeView{ISO: at.UTC().Format(time.RFC3339), Text: sw.day(at.UTC())}
		out[i] = c
	}
	return out
}

func (s *Server) roadmapHomeView(ctx context.Context, b store.RoadmapBoard, v *roadmapPageView) error {
	v.H1, v.Lead = b.Name, b.Description
	v.Title, v.OGTitle = b.Name+" – "+v.W.Roadmap, b.Name
	v.Description = helpDescription(b.Description, b.Name+" – "+v.W.Roadmap)
	v.Canonical = v.Home
	since := s.now().AddDate(0, 0, -roadmapShippedDays)
	for _, stage := range roadmapStages {
		arg := store.ListPublicIdeasParams{WorkspaceID: b.WorkspaceID, BoardID: &b.ID, Stage: stage, Lim: roadmapColumnShown}
		col := roadmapColumn{Name: v.W.stage(stage)}
		if stage == stageShipped {
			arg.Since = &since
			col.More, col.MoreLabel = v.Home+"?stage=shipped", v.W.AllShipped
		}
		rows, err := s.st.ListPublicIdeas(ctx, arg)
		if err != nil {
			return err
		}
		col.Cards = s.roadmapCards(v.W, v.SW, rows, b)
		v.Columns = append(v.Columns, col)
	}
	return nil
}

func (s *Server) roadmapStageView(ctx context.Context, b store.RoadmapBoard, v *roadmapPageView, stage string) error {
	rows, err := s.st.ListPublicIdeas(ctx, store.ListPublicIdeasParams{WorkspaceID: b.WorkspaceID, BoardID: &b.ID, Stage: stage, Lim: roadmapStageShown})
	if err != nil {
		return err
	}
	v.H1 = v.W.AllShipped
	v.Title, v.OGTitle = v.W.AllShipped+" – "+b.Name, v.W.AllShipped
	v.Description = helpDescription("", b.Name+" – "+v.W.AllShipped)
	v.Canonical = v.Home + "?stage=shipped"
	v.Crumbs = []helpCrumb{{Name: v.W.Home, URL: v.Home}, {Name: v.W.Shipped}}
	v.Columns = []roadmapColumn{{Cards: s.roadmapCards(v.W, v.SW, rows, b)}}
	v.JSONLD = jsonLD(breadcrumbLD(v.Crumbs[:1], v.W.AllShipped, v.Canonical))
	return nil
}

func (s *Server) roadmapIdeaView(ctx context.Context, b store.RoadmapBoard, v *roadmapPageView, id uuid.UUID) error {
	t, err := loadThread(ctx, s.st.Queries, b.WorkspaceID, id, false)
	if err == errDiscussionGone {
		return errRoadmapGone
	}
	if err != nil {
		return err
	}
	d := t.d
	if d.Kind != kindIdea || d.BoardID == nil || *d.BoardID != b.ID || d.State != statePublished {
		return errRoadmapGone
	}
	op := t.opening()
	if op.DeletedAt != nil {
		return errRoadmapGone
	}
	stage := orZero(d.Stage)
	iv := &roadmapIdeaView{ID: d.ID.String(), StageName: v.W.stage(stage), Votes: v.W.votes(d.VoteCount), Comments: v.W.comments(d.ReplyCount), Body: template.HTML(renderMarkdown(op.Body))}
	changes, err := s.st.ListRoadmapStageChanges(ctx, store.ListRoadmapStageChangesParams{WorkspaceID: b.WorkspaceID, IdeaID: d.ID, Lim: 50})
	if err != nil {
		return err
	}
	iv.History = []roadmapHistory{{Label: v.W.SuggestedOn, At: timeView{ISO: d.CreatedAt.UTC().Format(time.RFC3339), Text: v.SW.day(d.CreatedAt.UTC())}}}
	for _, c := range slices.Backward(changes) {
		if c.ToStage == stageIdeas {
			continue
		}
		iv.History = append(iv.History, roadmapHistory{Label: v.W.stageOn(c.ToStage), At: timeView{ISO: c.CreatedAt.UTC().Format(time.RFC3339), Text: v.SW.day(c.CreatedAt.UTC())}})
	}
	for _, p := range t.posts {
		if p.Opening || p.State != statePublished || p.DeletedAt != nil || len(iv.Replies) >= roadmapReplyShown {
			continue
		}
		a := discussionAuthor(p, false)
		r := roadmapReply{Name: v.W.Reader, Body: template.HTML(renderMarkdown(p.Body)), At: timeView{ISO: p.CreatedAt.UTC().Format(time.RFC3339), Text: v.SW.day(p.CreatedAt.UTC())}}
		if a.Name != nil {
			r.Name, r.Initials = *a.Name, a.Initials
		}
		iv.Replies = append(iv.Replies, r)
	}
	v.Idea = iv
	v.H1 = d.Title
	v.OGType = "article"
	canonical := v.Home + "/ideas/" + d.ID.String()
	v.Canonical = canonical
	v.Title, v.OGTitle = d.Title+" – "+b.Name, d.Title
	v.Description = helpDescription(postExcerpt(op.Body), d.Title+" – "+b.Name)
	v.Crumbs = []helpCrumb{{Name: v.W.Home, URL: v.Home}, {Name: d.Title}}
	v.JSONLD = jsonLD(breadcrumbLD(v.Crumbs[:1], d.Title, canonical))
	return nil
}

type roadmapPreviewResponse []byte

func (b roadmapPreviewResponse) VisitPreviewRoadmapBoardResponse(w http.ResponseWriter) error {
	h := w.Header()
	h.Set("Content-Type", roadmapPageType)
	h.Set("Cache-Control", "no-store")
	h.Set("Content-Security-Policy", publicCSP{img: "'self' https:", scriptHash: publicScriptHash(roadmapScript), frameAncestors: "'self'", formAction: "'none'"}.String())
	w.WriteHeader(http.StatusOK)
	_, err := w.Write(b)
	return err
}

func (s *Server) PreviewRoadmapBoard(ctx context.Context, req oas.PreviewRoadmapBoardRequestObject) (oas.PreviewRoadmapBoardResponseObject, error) {
	if err := s.roadmapOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	prm := req.Params
	rt := roadmapRoute{view: "home"}
	if prm.View != nil && *prm.View == oas.PreviewRoadmapBoardViewIdea {
		if prm.Id == nil {
			return nil, errValidation("id is required for view=idea")
		}
		rt = roadmapRoute{view: "idea", idea: *prm.Id}
	} else if prm.View != nil && *prm.View != oas.PreviewRoadmapBoardViewHome {
		return nil, errValidation("view must be home or idea")
	}
	if err := s.rateLimit(ctx, rateCheck{"roadmap-render:" + p.viewerID().String(), limitHelpRender}); err != nil {
		return nil, err
	}
	b, err := visibleBoard(ctx, s.st.Queries, p, req.BoardId, false)
	if err != nil {
		return nil, err
	}
	in, err := s.st.GetInbox(ctx, store.GetInboxParams{WorkspaceID: b.WorkspaceID, ID: b.InboxID})
	if err != nil {
		return nil, err
	}
	locale := in.DefaultLocale
	if prm.Locale != nil {
		if err := validHelpLocale(*prm.Locale, "locale"); err != nil {
			return nil, err
		}
		locale = *prm.Locale
	}
	body, err := s.roadmapBody(ctx, b, in, rt, locale, false)
	if err == errRoadmapGone {
		return nil, problem(http.StatusNotFound, "not_found", "no such published idea on this board")
	}
	if err != nil {
		return nil, err
	}
	return roadmapPreviewResponse(body), nil
}
