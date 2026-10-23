package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"uuid"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	issueRepo     = "productdevbook/yuva"
	issueLabel    = "from-assistant"
	issueMaxField = 4000
	issueMaxURL   = 7500
	docsBase      = "https://useyuva.com/docs/"
)

type setupIntegration struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	What    string `json:"what"`
	Signals string `json:"signals" jsonschema:"what in a codebase suggests it fits"`
	How     string `json:"how" jsonschema:"the prompt or tools that set it up"`
	Scope   string `json:"scope,omitempty" jsonschema:"the scope the connection needs to set it up"`
	Guide   string `json:"guide"`
}

var setupIntegrations = []setupIntegration{
	{ID: "live_chat", Title: "Live chat", What: "A chat bubble on a website or web app that lands in a Yuva inbox.",
		Signals: "a website or web app (React, Next.js, Nuxt, Vue, Astro, plain HTML) with no chat yet, or with Intercom, Crisp, Tawk, Zendesk or HubSpot chat to replace",
		How:     "the setup_live_chat prompt: create_inbox, create_chat_channel, get_install_snippet", Scope: "inboxes:manage", Guide: docsBase + "widget/"},
	{ID: "signed_in_users", Title: "Signed-in users", What: "The chat knows who a signed-in user is, so their conversations follow them across devices.",
		Signals: "the app has accounts and a login (NextAuth, Clerk, Supabase, Firebase Auth, Devise, a session cookie)",
		How:     "sign an identity token on the backend with the inbox's identity secret; an owner rotates that secret in the inbox settings, it is never given to an assistant", Guide: docsBase + "identity/"},
	{ID: "docs_feedback", Title: "Documentation feedback and discussions", What: "Was this page helpful, comments and discussions under documentation pages.",
		Signals: "a documentation site (Fumadocs, Docusaurus, VitePress, Starlight, Nextra, Mintlify-like Markdown docs)",
		How:     "create_chat_channel with discussions, then get_install_snippet with a docs- framework", Scope: "inboxes:manage", Guide: docsBase + "documentation-pages/"},
	{ID: "web_analytics", Title: "Web analytics", What: "Cookieless visits, pages, sources, events, goals and revenue, next to the inbox.",
		Signals: "a public website, or Google Analytics, Plausible, Fathom or Umami already installed",
		How:     "the setup_analytics prompt: create_property, get_analytics_snippet, create_goal", Scope: "analytics:manage", Guide: docsBase + "analytics/"},
	{ID: "app_chat", Title: "Chat in mobile apps", What: "The conversation list, thread and composer inside an iOS or Android app.",
		Signals: "an Xcode project or Package.swift, or an Android Gradle project with Compose",
		How:     "an app channel in the inbox settings, then the YuvaKit or Kotlin SDK", Guide: docsBase + "mobile/"},
	{ID: "app_analytics", Title: "Analytics in apps", What: "Screens, events and revenue from iOS and Android apps, without device or advertising ids.",
		Signals: "an iOS or Android app", How: "turn on the platform in the property, then the YuvaAnalytics or yuva-analytics library", Scope: "analytics:manage", Guide: docsBase + "analytics-apps/"},
	{ID: "server_events", Title: "Server-side events", What: "Sign-ups, payments and renewals that only the backend sees.",
		Signals: "a backend that handles sign-up, checkout or billing (Stripe, Paddle, Polar, LemonSqueezy webhooks)",
		How:     "an API key with analytics:write on the server, or the Go package", Guide: docsBase + "analytics-server/"},
	{ID: "newsletter", Title: "Newsletters", What: "Mailing lists with recorded consent, a subscribe form and broadcasts.",
		Signals: "a newsletter form, or Mailchimp, ConvertKit, Brevo, Resend audiences or Buttondown",
		How:     "create_mailing_list, update_inbox_sender with the person's own values, subscribers from create_list_import, add_subscription or a subscribe form (get_install_snippet with a subscribe- framework), then the draft_newsletter prompt", Scope: "lists:write and broadcasts:write", Guide: docsBase + "lists/"},
	{ID: "email_inbox", Title: "Support e-mail", What: "A support address whose mail and replies live in the inbox.",
		Signals: "a support@ or help@ address in the code, footer or docs", How: "create_email_channel, then the person sets its SMTP account in the panel; get_deliverability and create_dkim_key for the DNS records", Scope: "inboxes:manage", Guide: docsBase + "email/"},
	{ID: "move_history", Title: "Bring the history over", What: "Copies old support mail or help-desk tickets into the inbox, reading only.",
		Signals: "an existing help desk (Zendesk, Help Scout, Front, Freshdesk) or a support mailbox (Gmail, Outlook, IMAP)",
		How:     "the move_to_yuva prompt", Scope: "migrations:manage", Guide: docsBase + "migrate/"},
	{ID: "webhooks", Title: "Webhooks and the API", What: "The backend reacts to conversations and contacts, or drives Yuva itself.",
		Signals: "a backend that should open tickets, sync contacts or post to Slack", How: "Settings → Webhooks, or an API key", Guide: docsBase + "webhooks/"},
}

type setupStateIn struct{}

type setupInbox struct {
	ID       uuid.UUID    `json:"id"`
	Name     string       `json:"name"`
	Channels []mcpChannel `json:"channels,omitempty"`
}

type setupProperty struct {
	ID        uuid.UUID  `json:"id"`
	Name      string     `json:"name"`
	Domains   []string   `json:"domains"`
	Platforms []string   `json:"platforms"`
	InboxID   *uuid.UUID `json:"inbox_id,omitempty"`
}

type setupList struct {
	ID      uuid.UUID `json:"id"`
	Name    string    `json:"name"`
	InboxID uuid.UUID `json:"inbox_id"`
}

type setupStateOut struct {
	ServerURL     string             `json:"server_url"`
	ServerVersion string             `json:"server_version"`
	Inboxes       []setupInbox       `json:"inboxes"`
	Properties    []setupProperty    `json:"analytics_properties"`
	Lists         []setupList        `json:"mailing_lists"`
	NoSender      []setupInbox       `json:"inboxes_without_sender,omitempty" jsonschema:"inboxes with mailing lists but no sender identity (legal name and postal address); their broadcasts are blocked until update_inbox_sender sets it from the person's own values"`
	NotVisible    []string           `json:"not_visible,omitempty" jsonschema:"parts this connection cannot read, with the scope to grant"`
	Integrations  []setupIntegration `json:"integrations" jsonschema:"what Yuva can add to a project, how to tell it fits and how to set it up"`
}

type reportIssueIn struct {
	Title    string `json:"title" jsonschema:"one line, what went wrong"`
	Area     string `json:"area,omitempty"`
	What     string `json:"what_happened" jsonschema:"what happened, in a few sentences"`
	Expected string `json:"expected,omitempty"`
	Steps    string `json:"steps,omitempty" jsonschema:"the shortest steps that show it"`
	Error    string `json:"error,omitempty" jsonschema:"the error text or log lines, trimmed to what matters"`
	Env      string `json:"environment,omitempty" jsonschema:"versions: SDK, framework, language, OS, browser"`
}

type reportIssueOut struct {
	Repository string `json:"repository"`
	Title      string `json:"title"`
	Body       string `json:"body"`
	Redactions int    `json:"redactions" jsonschema:"how many names, addresses, keys and ids were removed"`
	SearchURL  string `json:"search_url" jsonschema:"existing issues with similar words; check it before opening a new one"`
	NewIssue   string `json:"new_issue_url" jsonschema:"opens GitHub's new-issue form with the report filled in"`
	GhCommand  string `json:"gh_command" jsonschema:"opens the issue with the GitHub CLI, as the person signed in to gh"`
	Next       string `json:"next"`
}

var issueAreas = []any{"widget", "js-sdk", "swift-sdk", "kotlin-sdk", "go-sdk", "api", "mcp", "panel", "analytics", "email", "broadcasts", "migrations", "docs", "other"}

var mcpInitTools = []mcpTool{
	defineTool("get_setup_state", "See what Yuva is set up",
		"Returns what this workspace already has (inboxes and their channels, analytics properties, mailing lists, inboxes whose lists lack a sender identity) and the catalog of what Yuva can add to a project: what each part does, what in a codebase suggests it fits, and the prompt or tools that set it up. Use it with the init prompt to recommend a plan for the project you are working in. Read only.",
		readOnly, nil, mcpTool{},
		func(ctx context.Context, c *mcpCall, _ setupStateIn) (setupStateOut, error) {
			return c.setupState(ctx)
		}),
	defineTool("report_yuva_issue", "Prepare a Yuva bug report",
		"Turns a problem you hit with Yuva itself (the server, an SDK, the widget, the MCP server, the docs) into a GitHub issue for "+issueRepo+", with names, addresses, keys, ids and local paths removed. It files nothing: show the person the title and body, and when they agree open it with gh_command or new_issue_url. Check search_url for an existing issue first. Not for questions about the person's own data or setup.",
		readOnly, map[string][]any{"area": issueAreas}, mcpTool{},
		func(ctx context.Context, c *mcpCall, in reportIssueIn) (reportIssueOut, error) {
			return c.reportIssue(ctx, in)
		}),
}

func (c *mcpCall) setupState(ctx context.Context) (setupStateOut, error) {
	out := setupStateOut{
		ServerURL: c.s.auth.PublicURL, ServerVersion: c.s.version, Inboxes: []setupInbox{}, Properties: []setupProperty{},
		Lists: []setupList{}, Integrations: setupIntegrations,
	}
	hidden := func(op, what string) bool {
		err := keyMayCall(c.p, op)
		if err == nil {
			return false
		}
		var ae *apiError
		if errors.As(mcpScopeError(err), &ae) && ae.Scope != "" {
			what += ": grant " + ae.Scope
		}
		out.NotVisible = append(out.NotVisible, what)
		return true
	}
	var want setupWant
	if want.inboxes = !hidden("ListInboxes", "inboxes"); want.inboxes {
		want.channels = !hidden("ListChannels", "channels")
	}
	want.properties = c.s.analytics != nil && !hidden("ListAnalyticsProperties", "analytics properties")
	want.lists = !hidden("ListMailingLists", "mailing lists")
	parts, err := c.s.setupParts(ctx, c.p, want)
	if err != nil {
		return out, err
	}
	var ids []uuid.UUID
	for _, chs := range parts.channels {
		for _, ch := range chs {
			ids = append(ids, ch.ID)
		}
	}
	var emails map[uuid.UUID]store.EmailChannel
	var chats map[uuid.UUID]store.ChatChannel
	tenants := map[uuid.UUID]*store.SesTenant{}
	if want.channels {
		if emails, err = emailChannelsByID(ctx, c.s.st.Queries, c.p.workspaceID, ids); err != nil {
			return out, err
		}
		if chats, err = chatChannelsByID(ctx, c.s.st.Queries, c.p.workspaceID, ids); err != nil {
			return out, err
		}
		rows, err := c.s.st.ListSESTenantsOfWorkspace(ctx, c.p.workspaceID)
		if err != nil {
			return out, err
		}
		for _, t := range rows {
			tenants[t.InboxID] = &t
		}
	}
	for _, in := range parts.inboxes {
		si := setupInbox{ID: in.ID, Name: in.Name}
		for _, ch := range parts.channels[in.ID] {
			var e *store.EmailChannel
			if v, ok := emails[ch.ID]; ok {
				e = &v
			}
			var chat *store.ChatChannel
			if v, ok := chats[ch.ID]; ok {
				chat = &v
			}
			row := store.Channel{ID: ch.ID, WorkspaceID: c.p.workspaceID, InboxID: in.ID, Kind: ch.Kind, Name: ch.Name, CreatedAt: ch.CreatedAt}
			si.Channels = append(si.Channels, channelOut(c.s.channelBody(row, e, chat, tenants[in.ID])))
		}
		out.Inboxes = append(out.Inboxes, si)
	}
	for _, p := range parts.properties {
		out.Properties = append(out.Properties, setupProperty{ID: p.ID, Name: p.Name, Domains: nonNil(p.Domains), Platforms: nonNil(p.Platforms), InboxID: p.InboxID})
	}
	for _, l := range parts.lists[:min(len(parts.lists), maxPageSize)] {
		out.Lists = append(out.Lists, setupList{ID: l.ID, Name: l.Name, InboxID: *l.InboxID})
	}
	withLists := map[uuid.UUID]bool{}
	for _, l := range parts.lists {
		withLists[*l.InboxID] = true
	}
	for _, in := range parts.inboxes {
		if !withLists[in.ID] {
			continue
		}
		row, err := c.s.st.GetInbox(ctx, store.GetInboxParams{WorkspaceID: c.p.workspaceID, ID: in.ID})
		if err != nil {
			return out, err
		}
		if !senderReady(row.SenderLegalName, row.SenderPostalAddress) {
			out.NoSender = append(out.NoSender, setupInbox{ID: in.ID, Name: in.Name})
		}
	}
	return out, nil
}

func (c *mcpCall) reportIssue(ctx context.Context, in reportIssueIn) (reportIssueOut, error) {
	title := strings.TrimSpace(in.Title)
	what := strings.TrimSpace(in.What)
	if title == "" || what == "" {
		return reportIssueOut{}, errValidation("title and what_happened are required")
	}
	if in.Area != "" && !slices.Contains(issueAreas, any(in.Area)) {
		return reportIssueOut{}, errValidation("unknown area " + in.Area)
	}
	r := newRedactor(c.workspaceNames(ctx))
	title = truncateRunes(r.clean(title), 120)
	var b strings.Builder
	section := func(head, text string, code bool) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		text = truncateRunes(r.clean(text), issueMaxField)
		b.WriteString("### " + head + "\n\n")
		if code {
			b.WriteString("```\n" + strings.ReplaceAll(text, "```", "'''") + "\n```\n\n")
		} else {
			b.WriteString(text + "\n\n")
		}
	}
	section("What happened", what, false)
	section("What I expected", in.Expected, false)
	section("Steps to reproduce", in.Steps, false)
	section("Error", in.Error, true)
	env := "- Yuva server: " + c.s.version + "\n"
	if in.Area != "" {
		env += "- Area: " + in.Area + "\n"
	}
	if e := strings.TrimSpace(in.Env); e != "" {
		env += "- " + strings.ReplaceAll(truncateRunes(r.clean(e), 1000), "\n", "\n- ") + "\n"
	}
	b.WriteString("### Environment\n\n" + env + "\n")
	b.WriteString("<sub>Prepared by an AI assistant with the Yuva MCP tool report_yuva_issue. Names, addresses, keys, ids and local paths were removed before the person reviewed it.</sub>\n")
	body := b.String()

	q := url.Values{"title": {title}, "body": {body}, "labels": {issueLabel}}
	newIssue := "https://github.com/" + issueRepo + "/issues/new?" + q.Encode()
	if len(newIssue) > issueMaxURL {
		short := url.Values{"title": {title}, "labels": {issueLabel}, "body": {"Paste the body from the assistant here."}}
		newIssue = "https://github.com/" + issueRepo + "/issues/new?" + short.Encode()
	}
	search := "https://github.com/" + issueRepo + "/issues?" + url.Values{"q": {"is:issue " + title}}.Encode()
	gh := fmt.Sprintf("gh issue create --repo %s --label %s --title %s --body-file - <<'YUVA_ISSUE_BODY'\n%sYUVA_ISSUE_BODY",
		issueRepo, issueLabel, shellQuote(title), strings.ReplaceAll(body, "\nYUVA_ISSUE_BODY", "\nYUVA_ISSUE_BODY."))
	return reportIssueOut{
		Repository: issueRepo, Title: title, Body: body, Redactions: r.count, SearchURL: search, NewIssue: newIssue, GhCommand: gh,
		Next: "Show the person the title and body and ask before opening it. Then run gh_command if gh is signed in, or give them new_issue_url.",
	}, nil
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func (c *mcpCall) workspaceNames(ctx context.Context) []string {
	ws := c.p.workspaceID
	var names []string
	if w, err := c.s.st.GetWorkspace(ctx, ws); err == nil {
		names = append(names, w.Name)
	}
	inboxes, _ := c.s.st.ListInboxes(ctx, store.ListInboxesParams{WorkspaceID: ws, AllInboxes: true})
	for _, in := range inboxes {
		names = append(names, in.Name, in.Slug)
		chs, _ := c.s.st.ListChannels(ctx, store.ListChannelsParams{WorkspaceID: ws, InboxID: in.ID})
		for _, ch := range chs {
			names = append(names, ch.Name)
			names = append(names, settingsHosts(ch.Settings)...)
		}
	}
	props, _ := c.s.st.ListAnalyticsProperties(ctx, store.ListAnalyticsPropertiesParams{WorkspaceID: ws, AllInboxes: true})
	for _, p := range props {
		names = append(names, p.Name)
		names = append(names, p.Domains...)
	}
	lists, _ := c.s.st.ListMailingLists(ctx, store.ListMailingListsParams{WorkspaceID: ws, AllInboxes: true, Lim: 500})
	for _, l := range lists {
		names = append(names, l.Name)
	}
	return names
}

func settingsHosts(raw []byte) []string {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return nil
	}
	var out []string
	var walk func(any)
	walk = func(v any) {
		switch t := v.(type) {
		case map[string]any:
			for _, x := range t {
				walk(x)
			}
		case []any:
			for _, x := range t {
				walk(x)
			}
		case string:
			if u, err := url.Parse(t); err == nil && u.Host != "" {
				out = append(out, u.Hostname())
			} else if at := strings.LastIndexByte(t, '@'); at > 0 && !strings.ContainsAny(t, " \n") {
				out = append(out, t, t[at+1:])
			}
		}
	}
	walk(v)
	return out
}

type redactor struct {
	names []*regexp.Regexp
	count int
}

var (
	redactRules = []struct {
		re   *regexp.Regexp
		with string
	}{
		{regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]{8,}`), "Bearer <token>"},
		{regexp.MustCompile(`\byuva_[a-z0-9]+_[A-Za-z0-9_-]{6,}`), "<yuva-key>"},
		{regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{5,}\.[A-Za-z0-9_-]{5,}\.[A-Za-z0-9_-]{5,}`), "<token>"},
		{regexp.MustCompile(`(?i)[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}`), "<email>"},
		{regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`), "<id>"},
		{regexp.MustCompile(`(?i)(?:/Users|/home|[A-Z]:\\Users)[/\\][^/\\\s]+`), "~"},
		{regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`), "<ip>"},
		{regexp.MustCompile(`(?i)\b(?:[0-9a-f]{1,4}:){3,7}[0-9a-f]{1,4}\b`), "<ip>"},
		{regexp.MustCompile(`\b[A-Fa-f0-9]{32,}\b`), "<secret>"},
		{regexp.MustCompile(`\b[A-Za-z0-9+/_-]{40,}={0,2}`), "<secret>"},
	}
	redactURL      = regexp.MustCompile(`(?i)\bhttps?://[^\s<>"'()\[\]]+`)
	redactBareHost = regexp.MustCompile(`(?i)\b(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+(?:com|net|org|io|dev|app|co|ai|xyz|tech|cloud|site|online|store|shop|me|tr|de|uk|eu|us|ca|fr|nl|es|it|pl|ru|br|in|jp|cn|au)\b`)
	keptHosts      = []string{"github.com", "useyuva.com", "localhost", "example.com", "example.org", "example.net"}
)

func newRedactor(names []string) *redactor {
	r := &redactor{}
	seen := map[string]bool{}
	slices.SortFunc(names, func(a, b string) int { return len(b) - len(a) })
	for _, n := range names {
		n = strings.TrimSpace(n)
		key := strings.ToLower(n)
		if len([]rune(n)) < 3 || seen[key] || slices.Contains(keptHosts, key) {
			continue
		}
		seen[key] = true
		r.names = append(r.names, regexp.MustCompile(`(?i)(^|[^\pL\pN])`+regexp.QuoteMeta(n)+`($|[^\pL\pN])`))
	}
	return r
}

func keptHost(h string) bool {
	h = strings.ToLower(strings.TrimSuffix(h, "."))
	for _, k := range keptHosts {
		if h == k || strings.HasSuffix(h, "."+k) {
			return true
		}
	}
	return false
}

func (r *redactor) clean(s string) string {
	s = redactURL.ReplaceAllStringFunc(s, func(m string) string {
		if u, err := url.Parse(m); err == nil && keptHost(u.Hostname()) {
			return m
		}
		r.count++
		return "<url>"
	})
	for _, rule := range redactRules {
		s = rule.re.ReplaceAllStringFunc(s, func(string) string { r.count++; return rule.with })
	}
	for _, re := range r.names {
		s = re.ReplaceAllStringFunc(s, func(m string) string {
			r.count++
			sub := re.FindStringSubmatch(m)
			return sub[1] + "<name>" + sub[2]
		})
	}
	return redactBareHost.ReplaceAllStringFunc(s, func(m string) string {
		if keptHost(m) {
			return m
		}
		r.count++
		return "<host>"
	})
}

func addInitPrompt(srv *mcp.Server) {
	srv.AddPrompt(&mcp.Prompt{
		Name: "init", Title: "Add Yuva to this project",
		Description: "Look at the project you are working in, see what Yuva already has, and recommend which parts of Yuva to add and how, step by step.",
		Arguments:   []*mcp.PromptArgument{{Name: "goal", Description: "What the person wants from Yuva, such as live chat, analytics or newsletters; found from the project when empty."}},
	}, func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		goal := strings.TrimSpace(req.Params.Arguments["goal"])
		focus := "Recommend what fits the project best."
		if goal != "" {
			focus = "The person wants: " + goal + ". Start with that, then mention anything else that clearly fits."
		}
		return promptResult("Add Yuva to this project",
			"Help me add Yuva to the project open in this workspace. "+focus,
			"1. Read the project before calling Yuva: the framework and language (package.json, go.mod, Gemfile, pyproject, composer.json, Package.swift or an Xcode project, build.gradle), whether it is a website, a web app with sign-in, a documentation site, a mobile app or a backend, how users sign in, and which support, chat, analytics, newsletter or help-desk tools it uses today. Do not send any code, file contents, secrets or customer data to Yuva.",
			"2. Call get_setup_state to see what this Yuva workspace already has and the catalog of integrations, each with the signals that suggest it fits and how to set it up.",
			"3. Answer with a short plan, most useful first: for each step what it gives this project, the evidence from the code, what already exists in Yuva, the prompt or tools that do it (setup_live_chat, setup_analytics, move_to_yuva, get_install_snippet and so on), the scope it needs, and the guide link. Leave out what does not fit and say why in one line.",
			"4. Change nothing until I pick steps. Then do them one by one in the code and in Yuva, and say what you changed. When a step needs a scope this connection lacks (get_setup_state lists it under not_visible, or a tool answers insufficient_scope), tell me to reconnect and grant it.",
			"5. If Yuva itself misbehaves along the way (an error from the server, an SDK that does not work as its guide says, a wrong guide), offer to report it with report_yuva_issue. Show me the report before opening it."), nil
	})
}
