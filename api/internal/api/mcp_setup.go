package api

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"uuid"

	"golang.org/x/text/unicode/norm"

	"github.com/productdevbook/yuva/api/internal/oas"
)

const docsBaseURL = "https://github.com/productdevbook/yuva/blob/main/docs"

var (
	errNotManager    = problem(http.StatusForbidden, "forbidden", "only owners and admins set up inboxes and channels")
	errNotChatTool   = errValidation("this tool changes chat channels only; list_channels shows each channel's kind")
	releasePattern   = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	snippetFramework = []any{"html", "react", "nextjs", "nuxt", "vue", "astro", "shadcn", "docs-html", "docs-nuxt", "docs-react",
		"subscribe-html", "subscribe-react", "subscribe-nuxt", "subscribe-shadcn"}
)

func mayManage(p principal) bool { return requireManagerOrKey(p) == nil }

// slugify makes an inbox slug from a name the way the panel does.
func slugify(s string) string {
	s = strings.ReplaceAll(strings.ToLowerSpecial(unicode.TurkishCase, s), "ı", "i")
	var b strings.Builder
	dash := false
	for _, r := range norm.NFKD.String(s) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
		case unicode.Is(unicode.Mn, r):
		default:
			dash = true
		}
	}
	out := b.String()
	if len(out) > 60 {
		out = strings.TrimRight(out[:60], "-")
	}
	return out
}

type mcpChannel struct {
	ID                uuid.UUID `json:"id"`
	InboxID           uuid.UUID `json:"inbox_id"`
	Kind              string    `json:"kind" jsonschema:"email, chat (web widget and documentation pages), app (mobile SDKs) or api"`
	Name              string    `json:"name"`
	PublicKey         string    `json:"public_key,omitempty" jsonschema:"the chat or app channel's key for the widget and the SDKs; not a secret"`
	AllowedOrigins    []string  `json:"allowed_origins,omitempty" jsonschema:"the only origins whose pages may use a chat channel"`
	AllowAnonymous    *bool     `json:"allow_anonymous,omitempty" jsonschema:"visitors without an identity token may chat"`
	Discussions       string    `json:"discussions,omitempty" jsonschema:"off, moderated or open: discussions on documentation pages"`
	Greeting          string    `json:"greeting,omitempty" jsonschema:"shown before the first message; the inbox's greeting when empty"`
	Platforms         []string  `json:"platforms,omitempty" jsonschema:"ios and android, for an app channel"`
	EmailAddress      string    `json:"email_address,omitempty" jsonschema:"the address an e-mail channel receives mail at"`
	FromAddress       string    `json:"from_address,omitempty" jsonschema:"the address replies are sent from, when it differs"`
	ForwardingAddress string    `json:"forwarding_address,omitempty" jsonschema:"an e-mail channel's second address: the mailbox forwards its mail here, keeping a copy, so it reaches Yuva"`
	CreatedAt         time.Time `json:"created_at"`
}

func channelOut(ch oas.Channel) mcpChannel {
	out := mcpChannel{ID: ch.Id, InboxID: ch.InboxId, Kind: string(ch.Kind), Name: ch.Name, CreatedAt: ch.CreatedAt}
	if c := ch.Chat; c != nil {
		out.PublicKey, out.Discussions, out.Greeting = c.PublicKey, string(c.Discussions), c.Greeting
		out.AllowAnonymous = &c.AllowAnonymous
		out.AllowedOrigins = make([]string, len(c.AllowedOrigins))
		for i, o := range c.AllowedOrigins {
			out.AllowedOrigins[i] = string(o)
		}
	}
	if a := ch.App; a != nil {
		out.PublicKey = a.PublicKey
		out.AllowAnonymous = &a.AllowAnonymous
		for _, p := range a.Platforms {
			out.Platforms = append(out.Platforms, string(p))
		}
	}
	if e := ch.Email; e != nil {
		out.EmailAddress = string(e.Address)
		if e.FromAddress != nil {
			out.FromAddress = string(*e.FromAddress)
		}
		out.ForwardingAddress = deref(ch.ForwardingAddress)
	}
	return out
}

type inboxIDIn struct {
	InboxID uuid.UUID `json:"inbox_id"`
}

type createInboxIn struct {
	Name                 string `json:"name" jsonschema:"the product or site the inbox serves, shown to members and contacts"`
	Slug                 string `json:"slug,omitempty" jsonschema:"lowercase letters, digits and single dashes; made from the name when empty"`
	Locale               string `json:"locale" jsonschema:"the inbox's language, used for the widget and e-mails"`
	Timezone             string `json:"timezone,omitempty" jsonschema:"an IANA time zone such as Europe/Istanbul; UTC when empty"`
	Mode                 string `json:"mode" jsonschema:"live shows who is available, typing and read receipts; async shows the expected reply time"`
	ExpectedReplyMinutes int32  `json:"expected_reply_minutes,omitempty" jsonschema:"shown to contacts in async mode, 1 to 43200"`
	Purpose              string `json:"purpose,omitempty" jsonschema:"support (the default) or mail; a mail inbox takes only e-mail channels and stays out of support reports, ratings, SLA and auto-assignment"`
}

type createChatChannelIn struct {
	InboxID        uuid.UUID `json:"inbox_id"`
	Name           string    `json:"name" jsonschema:"shown to members, such as the site's host"`
	Origins        []string  `json:"origins" jsonschema:"1 to 20 origins of the site, such as https://www.example.com, without a path; add the dev server too, such as http://localhost:3000"`
	AllowAnonymous *bool     `json:"allow_anonymous,omitempty" jsonschema:"visitors without an identity token may chat; true when absent"`
	Discussions    string    `json:"discussions,omitempty" jsonschema:"discussions on documentation pages; off when absent, moderated for a docs site"`
	Greeting       string    `json:"greeting,omitempty" jsonschema:"shown before the first message; the inbox's greeting when empty"`
}

type updateChatChannelIn struct {
	ChannelID      uuid.UUID `json:"channel_id"`
	AddOrigins     []string  `json:"add_origins,omitempty" jsonschema:"origins to allow, such as https://www.example.com, without a path"`
	RemoveOrigins  []string  `json:"remove_origins,omitempty" jsonschema:"origins to stop allowing; at least one origin must stay"`
	AllowAnonymous *bool     `json:"allow_anonymous,omitempty" jsonschema:"visitors without an identity token may chat"`
	Discussions    string    `json:"discussions,omitempty" jsonschema:"discussions on documentation pages"`
	Greeting       *string   `json:"greeting,omitempty" jsonschema:"the new greeting; an empty string uses the inbox's greeting"`
}

type installSnippetIn struct {
	ChannelID uuid.UUID   `json:"channel_id" jsonschema:"a chat channel from list_channels or create_chat_channel"`
	Framework string      `json:"framework" jsonschema:"the site's framework; html for plain HTML or anything else; the docs- values are for documentation sites, the subscribe- values for a mailing list's subscribe form"`
	Purpose   string      `json:"purpose,omitempty" jsonschema:"chat for the live chat bubble (default), docs for page feedback and discussions under documentation pages, subscribe for a mailing list's subscribe form"`
	ListIDs   []uuid.UUID `json:"list_ids,omitempty" jsonschema:"for subscribe: the public lists of the channel's inbox the form offers, from list_mailing_lists; every public list when empty"`
}

type installSnippetOut struct {
	ChannelID      uuid.UUID   `json:"channel_id"`
	Framework      string      `json:"framework"`
	Purpose        string      `json:"purpose" jsonschema:"chat, docs or subscribe"`
	ListIDs        []uuid.UUID `json:"list_ids,omitempty" jsonschema:"the lists a subscribe form offers"`
	Language       string      `json:"language" jsonschema:"the code's language: html, tsx, ts, vue or astro"`
	Code           string      `json:"code" jsonschema:"ready to paste, with this server's URL and the channel's public key; lines starting with // or <!-- name the file"`
	Install        string      `json:"install,omitempty" jsonschema:"run this in the site's project first"`
	Placement      string      `json:"placement" jsonschema:"where the code goes"`
	GuideURL       string      `json:"guide_url" jsonschema:"the full guide"`
	ServerURL      string      `json:"server_url"`
	PublicKey      string      `json:"public_key"`
	AllowedOrigins []string    `json:"allowed_origins" jsonschema:"the code works only on pages of these origins; add others with update_chat_channel"`
}

type snippet struct {
	language, code, install, placement, guide string
}

func joinLines(lines ...string) string { return strings.Join(lines, "\n") }

func npmInstall(version string) string {
	if releasePattern.MatchString(version) {
		return `npm install "useyuva@>=` + version + `"`
	}
	return "npm install useyuva"
}

// installSnippet holds the canonical install code; the panel's setup shows the same lines.
func installSnippet(framework, purpose, server, key, version string) snippet {
	widget, docs := docsBaseURL+"/widget.md", docsBaseURL+"/documentation-pages.md"
	npm := npmInstall(version)
	if purpose == "docs" {
		switch framework {
		case "docs-nuxt", "nuxt":
			return snippet{language: "ts", install: npm, guide: docs + "#nuxt-content",
				placement: "Add the module to nuxt.config.ts and put the two components in the page component that renders every docs page, after the content.",
				code: joinLines(
					`// nuxt.config.ts`,
					`export default defineNuxtConfig({`,
					`  modules: ["useyuva/nuxt"],`,
					`  yuva: {`,
					`    server: "`+server+`",`,
					`    channel: "`+key+`",`,
					`  },`,
					`})`,
					``,
					`<!-- your docs page, after the content -->`,
					`<YuvaDiscussions :page-title="page.title" />`,
					`<YuvaPageFeedback :page-title="page.title" />`,
				)}
		case "docs-react", "react", "nextjs":
			return snippet{language: "tsx", install: npm, guide: docs + "#fumadocs",
				placement: "Save it as components/page-help.tsx and render <PageHelp title={…} /> after the article of every docs page.",
				code: joinLines(
					`"use client"`,
					`import { YuvaDiscussions, YuvaPageFeedback } from "useyuva/react"`,
					``,
					`const server = "`+server+`"`,
					`const channel = "`+key+`"`,
					``,
					`export function PageHelp({ title }: { title: string }) {`,
					`  return (`,
					`    <>`,
					`      <YuvaDiscussions channel={channel} server={server} pageTitle={title} />`,
					`      <YuvaPageFeedback channel={channel} server={server} pageTitle={title} />`,
					`    </>`,
					`  )`,
					`}`,
				)}
		case "shadcn":
			return snippet{language: "tsx", guide: docs + "#shadcnui",
				install:   "npx shadcn@latest add https://useyuva.com/r/yuva-discussions.json https://useyuva.com/r/yuva-page-feedback.json",
				placement: "The install copies the components into your project; wrap the app in the provider in app/layout.tsx and put the two components under the article of every docs page.",
				code: joinLines(
					`// app/layout.tsx`,
					`import { YuvaDiscussionsProvider } from "@/hooks/use-yuva-discussions"`,
					``,
					`<YuvaDiscussionsProvider`,
					`  server="`+server+`"`,
					`  channel="`+key+`"`,
					`>`,
					`  {children}`,
					`</YuvaDiscussionsProvider>`,
					``,
					`// under the article`,
					`<YuvaPageFeedback />`,
					`<YuvaDiscussions />`,
				)}
		}
		placement := "Put it in the template every docs page shares, after the article."
		if framework == "vue" {
			placement += " In Vue, make yuva- tags custom elements (compilerOptions.isCustomElement)."
		}
		return snippet{language: "html", guide: docs + "#plain-html", placement: placement,
			code: joinLines(
				`<script src="`+server+`/yuva-docs.js" defer></script>`,
				``,
				`<yuva-discussions`,
				`  channel="`+key+`"`,
				`  server="`+server+`"`,
				`></yuva-discussions>`,
				`<yuva-page-feedback`,
				`  channel="`+key+`"`,
				`  server="`+server+`"`,
				`></yuva-page-feedback>`,
			)}
	}
	effect := []string{
		`export function YuvaChat() {`,
		`  useEffect(() => {`,
		`    const chat = document.createElement("yuva-chat")`,
		`    chat.setAttribute("channel", "` + key + `")`,
		`    chat.setAttribute("server", "` + server + `")`,
		`    document.body.append(chat)`,
		`    import("useyuva/chat")`,
		`    return () => chat.remove()`,
		`  }, [])`,
		`  return null`,
		`}`,
	}
	switch framework {
	case "nextjs", "shadcn":
		return snippet{language: "tsx", install: npm, guide: widget,
			placement: "Save it as components/yuva-chat.tsx and render <YuvaChat /> once inside <body> in app/layout.tsx.",
			code:      joinLines(append([]string{`"use client"`, `import { useEffect } from "react"`, ``}, effect...)...)}
	case "react":
		return snippet{language: "tsx", install: npm, guide: widget,
			placement: "Save it as src/YuvaChat.tsx and render <YuvaChat /> once in your root component.",
			code:      joinLines(append([]string{`import { useEffect } from "react"`, ``}, effect...)...)}
	case "nuxt":
		return snippet{language: "ts", install: npm, guide: widget + "#2-embed-it",
			placement: "Add the module to nuxt.config.ts and put <YuvaChat /> once in app.vue or your default layout.",
			code: joinLines(
				`// nuxt.config.ts`,
				`export default defineNuxtConfig({`,
				`  modules: ["useyuva/nuxt"],`,
				`  yuva: {`,
				`    server: "`+server+`",`,
				`    channel: "`+key+`",`,
				`  },`,
				`})`,
				``,
				`<!-- app.vue or your default layout -->`,
				`<YuvaChat />`,
			)}
	case "vue":
		return snippet{language: "vue", install: npm, guide: widget + "#2-embed-it",
			placement: "Import the element once in src/main.ts, mark yuva- tags as custom elements in vite.config.ts and put the tag once in App.vue.",
			code: joinLines(
				`// src/main.ts`,
				`import "useyuva/chat"`,
				``,
				`// vite.config.ts`,
				`vue({ template: { compilerOptions: { isCustomElement: (tag) => tag.startsWith("yuva-") } } })`,
				``,
				`<!-- App.vue -->`,
				`<yuva-chat`,
				`  channel="`+key+`"`,
				`  server="`+server+`"`,
				`></yuva-chat>`,
			)}
	case "astro":
		return snippet{language: "astro", install: npm, guide: widget + "#2-embed-it",
			placement: "Save it as src/components/YuvaChat.astro and put <YuvaChat /> once in the layout every page uses.",
			code: joinLines(
				`<yuva-chat`,
				`  channel="`+key+`"`,
				`  server="`+server+`"`,
				`></yuva-chat>`,
				``,
				`<script>`,
				`  import "useyuva/chat"`,
				`</script>`,
			)}
	}
	return snippet{language: "html", guide: widget,
		placement: "Paste it before the closing </body> tag of every page, or in the layout every page shares.",
		code: joinLines(
			`<script defer`,
			`  src="`+server+`/yuva.js"></script>`,
			`<yuva-chat`,
			`  channel="`+key+`"`,
			`  server="`+server+`"`,
			`></yuva-chat>`,
		)}
}

// subscribeSnippet holds the subscribe form code; the panel's Form tab shows the same lines.
func subscribeSnippet(framework, server, key, version string, listIDs []uuid.UUID, nameRequired bool) snippet {
	guide := docsBaseURL + "/lists.md#3-add-the-form-to-your-site"
	ids := make([]string, len(listIDs))
	for i, id := range listIDs {
		ids[i] = id.String()
	}
	quoted := func(q string) string {
		parts := make([]string, len(ids))
		for i, id := range ids {
			parts[i] = q + id + q
		}
		return strings.Join(parts, ", ")
	}
	var lists, name []string
	switch strings.TrimPrefix(framework, "subscribe-") {
	case "react", "nextjs":
		if len(ids) > 0 {
			lists = []string{`      lists={[` + quoted(`"`) + `]}`}
		}
		if nameRequired {
			name = []string{`      name="required"`}
		}
		code := []string{`"use client"`, `import { YuvaSubscribe } from "useyuva/react"`, ``, `export function Newsletter() {`, `  return (`,
			`    <YuvaSubscribe`, `      channel="` + key + `"`, `      server="` + server + `"`}
		code = append(append(append(code, lists...), name...), `    />`, `  )`, `}`)
		return snippet{language: "tsx", install: npmInstall(version), guide: guide,
			placement: "Save it as components/newsletter.tsx and render <Newsletter /> where the form should appear.",
			code:      joinLines(code...)}
	case "nuxt":
		attrs := ""
		if len(ids) > 0 {
			attrs += ` :lists="[` + quoted(`'`) + `]"`
		}
		if nameRequired {
			attrs += ` name="required"`
		}
		return snippet{language: "ts", install: npmInstall(version), guide: guide,
			placement: "Add the module to nuxt.config.ts (once; it is the same as for the chat) and put <YuvaSubscribe /> where the form should appear.",
			code: joinLines(
				`// nuxt.config.ts`,
				`export default defineNuxtConfig({`,
				`  modules: ["useyuva/nuxt"],`,
				`  yuva: {`,
				`    server: "`+server+`",`,
				`    channel: "`+key+`",`,
				`  },`,
				`})`,
				``,
				`<!-- any page or component -->`,
				`<YuvaSubscribe`+attrs+` />`,
			)}
	case "shadcn":
		if len(ids) > 0 {
			lists = []string{`  lists={[` + quoted(`"`) + `]}`}
		}
		if nameRequired {
			name = []string{`  name="required"`}
		}
		code := []string{`import { YuvaSubscribe } from "@/components/yuva-subscribe"`, ``, `<YuvaSubscribe`, `  server="` + server + `"`, `  channel="` + key + `"`}
		code = append(append(append(code, lists...), name...), `/>`)
		return snippet{language: "tsx", guide: guide,
			install:   "npx shadcn@latest add https://useyuva.com/r/yuva-subscribe.json",
			placement: "The install copies the component into your project; render it where the form should appear.",
			code:      joinLines(code...)}
	}
	if len(ids) > 0 {
		lists = []string{`  lists="` + strings.Join(ids, ",") + `"`}
	}
	if nameRequired {
		name = []string{`  name="required"`}
	}
	code := []string{`<script src="` + server + `/yuva-subscribe.js" defer></script>`, ``, `<yuva-subscribe`, `  channel="` + key + `"`, `  server="` + server + `"`}
	code = append(append(append(code, lists...), name...), `></yuva-subscribe>`)
	return snippet{language: "html", guide: guide,
		placement: "Paste it where the form should appear; the script tag is needed once per page.",
		code:      joinLines(code...)}
}

// subscribeFormChecks refuses lists a form on the channel cannot offer and reports whether the
// form asks for a name, as it does for an inbox in Turkish.
func (c *mcpCall) subscribeFormChecks(ctx context.Context, ch oas.Channel, listIDs []uuid.UUID) (bool, error) {
	if len(listIDs) > 0 {
		if err := keyMayCall(c.p, "GetMailingList"); err != nil {
			return false, mcpScopeError(err)
		}
	}
	for _, id := range listIDs {
		res, err := c.s.GetMailingList(ctx, oas.GetMailingListRequestObject{ListId: id})
		if err != nil {
			return false, err
		}
		l := res.(oas.GetMailingList200JSONResponse)
		if l.InboxId != ch.InboxId {
			return false, errValidation("the list " + id.String() + " belongs to another inbox than the chat channel; use a chat channel of the list's inbox")
		}
		if !l.Public {
			return false, errValidation("the list " + id.String() + " is private and forms do not offer it; make it public first")
		}
	}
	res, err := c.s.GetInbox(ctx, oas.GetInboxRequestObject{InboxId: ch.InboxId})
	if err != nil {
		return false, err
	}
	return res.(oas.GetInbox200JSONResponse).DefaultLocale == "tr", nil
}

func (c *mcpCall) getChannel(ctx context.Context, id uuid.UUID) (oas.Channel, error) {
	res, err := c.s.GetChannel(ctx, oas.GetChannelRequestObject{ChannelId: id})
	if err != nil {
		return oas.Channel{}, err
	}
	return oas.Channel(res.(oas.GetChannel200JSONResponse)), nil
}

func (c *mcpCall) createInbox(ctx context.Context, in createInboxIn) (mcpInbox, error) {
	locale, mode := in.Locale, oas.InboxMode(in.Mode)
	body := oas.InboxCreate{Name: in.Name, Slug: in.Slug, DefaultLocale: &locale, Mode: &mode, ExpectedReplyMinutes: optLimit(in.ExpectedReplyMinutes)}
	if in.Timezone != "" {
		body.Timezone = &in.Timezone
	}
	if in.Purpose != "" {
		body.Purpose = new(oas.InboxPurpose(in.Purpose))
	}
	base := slugify(in.Name)
	if base == "" {
		base = "inbox"
	}
	for n := 1; ; n++ {
		if in.Slug == "" {
			body.Slug = base
			if n > 1 {
				body.Slug = base + "-" + strconv.Itoa(n)
			}
		}
		res, err := c.s.CreateInbox(ctx, oas.CreateInboxRequestObject{Body: &body})
		var e *apiError
		if in.Slug == "" && n < 20 && errors.As(err, &e) && e.Code == "slug_taken" {
			continue
		}
		if err != nil {
			return mcpInbox{}, err
		}
		return inboxOut(res.(oas.CreateInbox201JSONResponse).Inbox), nil
	}
}

func (c *mcpCall) updateChatChannel(ctx context.Context, in updateChatChannelIn) (mcpChannel, error) {
	cur, err := c.getChannel(ctx, in.ChannelID)
	if err != nil {
		return mcpChannel{}, err
	}
	if cur.Chat == nil {
		return mcpChannel{}, errNotChatTool
	}
	removed := map[string]bool{}
	for _, o := range in.RemoveOrigins {
		if n, ok := normalizeOrigin(o); ok {
			removed[n] = true
		}
		removed[strings.TrimSpace(o)] = true
	}
	origins := []oas.Origin{}
	for _, o := range cur.Chat.AllowedOrigins {
		if !removed[string(o)] {
			origins = append(origins, o)
		}
	}
	for _, o := range in.AddOrigins {
		origins = append(origins, oas.Origin(o))
	}
	chat := oas.ChatChannelInput{
		AllowedOrigins: origins, AllowAnonymous: &cur.Chat.AllowAnonymous, AskEmailOffline: &cur.Chat.AskEmailOffline,
		Discussions: &cur.Chat.Discussions, Greeting: &cur.Chat.Greeting, Launcher: &cur.Chat.Launcher, AnnounceUpdates: &cur.Chat.AnnounceUpdates,
	}
	if in.AllowAnonymous != nil {
		chat.AllowAnonymous = in.AllowAnonymous
	}
	if in.Discussions != "" {
		chat.Discussions = optString[oas.DiscussionMode](in.Discussions)
	}
	if in.Greeting != nil {
		chat.Greeting = in.Greeting
	}
	res, err := c.s.UpdateChannel(ctx, oas.UpdateChannelRequestObject{ChannelId: in.ChannelID, Body: &oas.ChannelUpdate{Chat: &chat}})
	if err != nil {
		return mcpChannel{}, err
	}
	return channelOut(oas.Channel(res.(oas.UpdateChannel200JSONResponse))), nil
}

var discussionModes = []any{"off", "moderated", "open"}

var mcpSetupTools = []mcpTool{
	defineTool("list_channels", "List an inbox's channels",
		"Lists the channels of an inbox: e-mail addresses, chat channels with their public key, allowed origins and discussions mode, and app channels. Secrets are never returned. Maps to GET /v1/inboxes/{id}/channels.",
		readOnly, nil, mcpTool{ops: []string{"ListChannels"}},
		func(ctx context.Context, c *mcpCall, in inboxIDIn) (mcpPage[mcpChannel], error) {
			res, err := c.s.ListChannels(ctx, oas.ListChannelsRequestObject{InboxId: in.InboxID})
			if err != nil {
				return mcpPage[mcpChannel]{}, err
			}
			out := mcpPage[mcpChannel]{Items: []mcpChannel{}}
			for _, ch := range res.(oas.ListChannels200JSONResponse).Items {
				out.Items = append(out.Items, channelOut(ch))
			}
			return out, nil
		}),
	defineTool("create_inbox", "Create an inbox",
		"Creates an inbox for a product or site; add a chat channel to it next with create_chat_channel. The inbox's identity secret is not returned: an owner or admin rotates it in Yuva when the site signs users in. Owners and admins only. Maps to POST /v1/inboxes.",
		additive, map[string][]any{"locale": {"en", "tr"}, "mode": {"live", "async"}},
		mcpTool{ops: []string{"CreateInbox"}, allow: mayManage, denied: errNotManager},
		func(ctx context.Context, c *mcpCall, in createInboxIn) (mcpInbox, error) {
			return c.createInbox(ctx, in)
		}),
	defineTool("create_chat_channel", "Create a chat channel",
		"Creates a chat channel for a website: the live chat widget, and page feedback and discussions on documentation pages. Returns its public key; get the code for the site with get_install_snippet. Owners and admins only. Maps to POST /v1/inboxes/{id}/channels with kind chat.",
		additive, map[string][]any{"discussions": discussionModes},
		mcpTool{ops: []string{"CreateChannel"}, allow: mayManage, denied: errNotManager},
		func(ctx context.Context, c *mcpCall, in createChatChannelIn) (mcpChannel, error) {
			origins := make([]oas.Origin, len(in.Origins))
			for i, o := range in.Origins {
				origins[i] = oas.Origin(o)
			}
			chat := oas.ChatChannelInput{AllowedOrigins: origins, AllowAnonymous: boolPtr(true), Discussions: new(oas.DiscussionModeOff)}
			if in.AllowAnonymous != nil {
				chat.AllowAnonymous = in.AllowAnonymous
			}
			if in.Discussions != "" {
				chat.Discussions = optString[oas.DiscussionMode](in.Discussions)
			}
			if in.Greeting != "" {
				chat.Greeting = &in.Greeting
			}
			res, err := c.s.CreateChannel(ctx, oas.CreateChannelRequestObject{InboxId: in.InboxID, Body: &oas.ChannelCreate{
				Kind: oas.ChannelKindChat, Name: in.Name, Chat: &chat,
			}})
			if err != nil {
				return mcpChannel{}, err
			}
			return channelOut(oas.Channel(res.(oas.CreateChannel201JSONResponse))), nil
		}),
	defineTool("update_chat_channel", "Change a chat channel",
		"Adds or removes allowed origins, turns anonymous visitors on or off, and sets the discussions mode or the greeting of a chat channel; what is not given stays as it is. Owners and admins only. Maps to PATCH /v1/channels/{id}.",
		idempotent, map[string][]any{"discussions": discussionModes},
		mcpTool{ops: []string{"GetChannel", "UpdateChannel"}, allow: mayManage, denied: errNotManager},
		func(ctx context.Context, c *mcpCall, in updateChatChannelIn) (mcpChannel, error) {
			return c.updateChatChannel(ctx, in)
		}),
	defineTool("get_install_snippet", "Get the install code",
		"Returns the code that adds a chat channel to a site, filled with this server's URL and the channel's public key (not a secret), the install command when one is needed, where the code goes and the guide. purpose chat is the live chat bubble; docs is page feedback and discussions under documentation pages; subscribe is a <yuva-subscribe> form for the mailing lists in list_ids (html, react, nuxt or shadcn).",
		readOnly, map[string][]any{"framework": snippetFramework, "purpose": {"chat", "docs", "subscribe"}},
		mcpTool{ops: []string{"GetChannel"}},
		func(ctx context.Context, c *mcpCall, in installSnippetIn) (installSnippetOut, error) {
			ch, err := c.getChannel(ctx, in.ChannelID)
			if err != nil {
				return installSnippetOut{}, err
			}
			if ch.Chat == nil {
				return installSnippetOut{}, errNotChatTool
			}
			purpose := in.Purpose
			if strings.HasPrefix(in.Framework, "docs-") {
				purpose = "docs"
			} else if strings.HasPrefix(in.Framework, "subscribe-") {
				purpose = "subscribe"
			} else if purpose == "" {
				purpose = "chat"
			}
			server := c.s.auth.PublicURL
			var sn snippet
			if purpose == "subscribe" {
				nameRequired, err := c.subscribeFormChecks(ctx, ch, in.ListIDs)
				if err != nil {
					return installSnippetOut{}, err
				}
				sn = subscribeSnippet(in.Framework, server, ch.Chat.PublicKey, c.s.version, in.ListIDs, nameRequired)
			} else {
				sn = installSnippet(in.Framework, purpose, server, ch.Chat.PublicKey, c.s.version)
			}
			out := installSnippetOut{
				ChannelID: ch.Id, Framework: in.Framework, Purpose: purpose, Language: sn.language, Code: sn.code,
				Install: sn.install, Placement: sn.placement, GuideURL: sn.guide, ServerURL: server,
				PublicKey: ch.Chat.PublicKey, AllowedOrigins: channelOut(ch).AllowedOrigins,
			}
			if purpose == "subscribe" {
				out.ListIDs = in.ListIDs
			}
			return out, nil
		}),
}
