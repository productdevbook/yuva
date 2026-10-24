# Roadmap

Each milestone is a GitHub issue. A milestone is done when every acceptance item is shown working
on a running instance, not when the code exists.

## M0 — Foundation

Status: done.

- `api/` Go module, `web/` panel, `edge/`, `sdk/js`, `sdk/go` skeletons with their check commands.
- `openapi/openapi.yaml` with health and version, linted; generated strict server.
- Goose migrations, sqlc, River set up; `deploy/compose.yaml` starts Postgres and the server.
- CI: every check command runs on pull requests.

Accept: `docker compose up` serves `/healthz` and an empty panel; CI is green.

## M1 — Core model and panel sign-in

Status: done.

- Workspaces, members, roles, per-inbox access; sign-in with e-mailed code and passkeys.
- Inboxes and channels (settings only), contacts, conversations, messages, notes, events,
  attachments in object storage.
- Workspace API keys; `/v1` endpoints for all of the above; usage counters.

Accept: through the API alone, create an inbox, a contact and a conversation, add messages and a
note, assign and close it.

## M2 — Agent panel

Status: done.

- Conversation list with filters and search, thread view, composer (reply/note, canned replies,
  attachments), contact sidebar, assignment, status, labels.
- Settings screens for M1 objects. Realtime updates over WebSocket. Turkish and English.

Accept: two members answer and hand over a conversation in the panel and see each other's changes
live.

## M3 — E-mail channel

Status: done.

- `/ingress/email`, the Cloudflare Email Worker in `edge/`, threading, quote stripping, HTML
  sanitizing, attachments, raw message storage.
- Outbound SMTP, thread headers, loop protection, SES bounce/complaint ingress.

Accept: a mail to the support address opens a conversation, the panel reply arrives in the
sender's mailbox in the same thread, their reply joins the conversation, an auto-reply does not
loop.

## M4 — Web widget: live chat and embedded threads

Status: done.

- `/client/v1`, contact sessions, identity tokens, anonymous visitors, allowed origins.
- `<yuva-chat>` launcher and embedded layouts, presence, typing, read receipts, business hours,
  `live`/`async` modes, e-mail continuity.
- `sdk/go`: identity token signing.

Accept: on a test page an anonymous visitor chats live with a member; a signed-in user of a host
app sees their earlier conversations; after leaving, the visitor gets the reply by e-mail.

## M5 — Mobile SDKs, feedback and webhooks

Status: done.

- `sdk/swift` and `sdk/kotlin`: client, conversation list, thread, composer, feedback form.
- Feedback kind and metadata; `api` channel for server-side forms.
- Standard Webhooks delivery with retries and a delivery log; contact deletion by external id.
- `sdk/go`: webhook verification.

Accept: in a sample iOS and Android app a user sends feedback, a member answers in the panel, the
host backend receives the webhook and the app shows the reply.

## M6 — Member notifications

Status: done.

- PWA install, Web Push, per-member notification preferences, e-mail fallback.

Accept: a new conversation in a `live` inbox notifies a member's phone within seconds.

## M6.1 — Satisfaction ratings

Status: done; every Accept item verified on 2026-10-23 at `bafdcc9` (#126). Decisions in `architecture.md` › Satisfaction ratings.

- An inbox setting; contacts rate a closed conversation `good` or `bad` with an optional comment,
  once per close, in the widget, the mobile SDKs and through two links in an e-mail.
- The rating in the panel thread and on the conversation, a `conversation.rated` webhook, ratings
  per inbox in the team stats.

Accept: a widget visitor and an e-mail contact each rate a closed conversation; the panel shows
both, the host backend receives `conversation.rated`, and a second rating of the same close is
refused.

## M7 — First internal rollout

Status: in progress.

- Our own instance; an existing in-app feedback form moved from e-mail to Yuva; e-mail channels for the products
  that already receive support mail.

Accept: one week of real support handled only in Yuva.

## M8 — First public release (0.0.1)

Status: done in `v0.0.1`.

- Docker image, install guide, configuration reference, upgrade notes, backup guide.
- Security review of authentication, client sessions, ingress and attachment handling.

Accept: a fresh install from the guide works on a clean host.

## M9 — Headless and MCP (0.0.4)

Status: done in `v0.0.4`; plan in issue #17, decisions in `architecture.md` › Headless access and
› OAuth and MCP. Checked on a public server with claude.ai, Claude Code and Codex.

- Scoped API keys, bot authors, drafts, an event feed, idempotency keys, a published headless JS
  client and an API-only mode.
- An MCP server at `/mcp` with OAuth for members, tools, resources and prompts over the existing
  `/v1` access rules; replies from assistants are drafts unless the workspace allows sending.
  Yuva runs no model itself.

Accept: Claude Code and claude.ai triage an inbox and draft replies a member sends from the panel;
a prompt-injection e-mail sends nothing; a headless chat works with the panel turned off.

## M10 — Native apps for members

Status: later, not scheduled; not part of 0.1.0. iOS in issue #18, Android in issue #19.

- iOS 27 (Swift 6.4, SwiftUI) and Android 16+ (Compose, Material 3 Expressive) apps for members,
  on the App Store and Google Play. One build signs in to any Yuva server: the member picks
  useyuva.com or enters their own server's URL, and signs in through OAuth (M9) with a code or a
  passkey on the server's own page.
- Push through a relay we run, with end-to-end encrypted payloads so the relay never sees content;
  self-hosted servers use it without setup. Quick reply from the notification.
- Offline cache and outbox, tablet and foldable layouts, English, Turkish and German.

Accept: the same store build works against useyuva.com and a self-hosted server; a push reaches a
real phone through the relay within seconds and the relay holds no message text.

## M11 — Feedback and questions on documentation pages

Status: done in `v0.0.7`; questions and published answers were replaced by discussions in M12. Plan in
issue #23, decisions in `architecture.md` › Documentation pages.

- `<yuva-page-feedback>` (was this page helpful, then optional text and e-mail) and
  `<yuva-page-questions>` (published answers and a question form) for any documentation site,
  on an existing chat channel; guides for Nuxt Content and Fumadocs.
- Per-page rating counters, page feedback as feedback conversations, questions as `question`
  conversations, answers members publish to the page.
- A Docs view in the panel: pages by rating, their feedback, questions and published answers.

Accept: on a Nuxt Content and a Fumadocs site, a visitor rates a page, sends feedback and asks a
question; the panel shows the counts and both conversations; a member answers the question and
publishes it, and the answer appears on that page without the visitor's name or address; an
element on an origin the channel does not allow is refused.

## M12 — Discussions on documentation pages and reader accounts

Status: done in `v0.0.7`; plan in issue #24, decisions in `architecture.md` › Discussions (M12).

- Public discussions under each documentation page, like GitHub Discussions: threads, replies,
  votes, accepted answers, followers, moderation. They replace page questions and published answers.
- Server-wide reader accounts (e-mail code or passkey), connected to documentation sites through a
  popup; identity tokens keep working for host apps with their own login.
- A reader account page: my discussions, followed threads, notifications, e-mail settings.
- `<yuva-discussions>` for any site, Nuxt and React wrappers, and a shadcn registry for React sites.
- A Discussions view and moderation queue in the team panel.

Accept: on demo-nuxt.useyuva.com a reader signs in with an e-mail code, starts a discussion and
waits for approval; a member approves it and answers; the reader is e-mailed, sees the answer on
the page and in their account, and marks it accepted; a second reader replies and votes; a React
site installs the shadcn components and shows the same thread; nothing private of another
workspace or another reader is visible.

## M13 — Broadcasts and e-mail designs

Status: M13.1–M13.5 done in `v0.0.9` and deployed on 2026-10-12 (#26–#30); M13.6 (#91) and M13.7 (#92)
done in `v0.0.18`, verified on 2026-10-19. Umbrella issue #25, decisions in `architecture.md` › Broadcasts and e-mail designs (M13).

- **M13.1 Lists and subscribing:** mailing lists with recorded consent and double opt-in,
  `<yuva-subscribe>` (HTML, React, Nuxt, shadcn), unsubscribe and preference pages with one-click, an API
  for host backends, consent export.
- **M13.2 Designs:** the design document, the server renderer to responsive HTML and text, images, the
  block designer in the panel, six starter templates in `templates/email/`.
- **M13.3 Sending:** broadcasts to lists narrowed by attributes, review, test sends, scheduling, pause and
  cancel, sending through an inbox's e-mail channel (optionally its own SMTP account), bounces,
  complaints and suppression, replies as conversations, reports, daily caps and the approval gate for
  hosted servers.
- **M13.4 Import and deliverability:** CSV import with attestation and progress; SPF, DKIM and DMARC
  check by probe; optional DKIM signing by Yuva.
- **M13.5 Reach:** the template gallery on the website with "Use this template", MCP tools and prompts,
  subscriptions in the mobile SDKs, the İYS export.
- **M13.6 Pace:** a broadcast's own pace, at most so many mails an hour, a day and in 30 days, spread
  evenly below the channel's rate and the workspace's daily limit; set in the composer, changed while it
  runs, checked by the review (at most 365 days), in the API and MCP.
- **M13.7 SES tenants:** on a shared Amazon SES account each inbox sends in its own SES tenant, created
  and watched by the server with an operator's IAM key limited to its tenants; support replies and
  broadcasts carry `X-SES-TENANT`; a channel's own tenant for self-hosters without keys; pauses shown
  on the inbox and its channels and acted on by its broadcasts; the operator's `inbox ses-tenant`
  commands, with the back-fill.

Accept: a visitor subscribes through `<yuva-subscribe>` and confirms; a member designs a newsletter from a
gallery template, sends a test and schedules it to the list; the deliverability check on one of our own
sending domains behind the edge shows SPF, DKIM and DMARC passing with the list headers signed, and its
WebKit and Chromium renders match the preview, in light and dark; one recipient replies and the reply is a conversation in the inbox;
an out-of-office reply is not; another unsubscribes with one click and gets no further broadcast; a bounce
suppresses the address and a complaint unsubscribes it; a hosted workspace cannot send before approval;
another workspace sees none of it.
A broadcast with a pace of 60 an hour sends one mail a minute to Mailpit, shows when its next mail goes
out, and goes on at the channel's rate within five minutes of removing the pace.
With the server's SES keys set, two inboxes sending through the server's SES account from one domain
get a tenant each, holding the domain and the configuration set; a reply and a broadcast to the SES
success simulator count under their inbox's tenant in CloudWatch; pausing one inbox's tenant in SES
pauses its running broadcast within five minutes, blocks resuming and shows on the inbox and its
channels, while the other inbox's broadcast finishes; `inbox ses-tenant reinstate` lets mail go again;
the IAM key cannot read a tenant outside its prefix; deleting the workspace deletes its tenants.

## M14 — Move to Yuva

Status: done in `v0.0.9` and deployed on 2026-10-12 (all four steps, #31–#34); umbrella issue #35, decisions in
`architecture.md` › Moving to Yuva (M14).

- **M14.1 IMAP with presets:** every mailbox over IMAP with app passwords (Gmail, Google Workspace,
  Yandex, iCloud, Zoho, Yahoo, Fastmail, GMX, any server), detection from the address and MX, a
  connection tester with precise errors, the read-only guard and its tests, scan and dry run,
  resumable copy with live progress, catch-up sync, forwarding addresses, delivery verification,
  finish with credential deletion, the wizard and list in the panel, guides in en, tr and de, MCP
  tools and the `move_to_yuva` prompt.
- **M14.2 Microsoft over IMAP:** "Sign in with Microsoft" for Outlook.com and Microsoft 365, only to
  obtain an IMAP XOAUTH2 token (`IMAP.AccessAsUser.All`); the same read-only IMAP client.
- **M14.3 Export files:** Google Takeout, mbox, .eml and Maildir archives with resumable upload; the
  fast way for very large Gmail mailboxes, followed by IMAP for newer mail and the cutover.
- **M14.4 Help desks:** Zendesk and Help Scout.

Accept: an owner moves a mailbox with an app password through the wizard; the dry run matches the
mailbox; killing the server mid-copy loses and duplicates nothing; the inbox shows the threads with
original dates, members as authors and labels, nothing unread and no notification, webhook or push;
mail arriving during the switch is stored once and notifies; the forwarding is verified by a probe;
finishing deletes the credentials; the source mailbox is byte-identical (UIDs, flags, modseq) before
and after, on the Dovecot fixture; the wizard and the guides tell people where each provider keeps
its IMAP settings and app passwords. A Takeout file imports the same way, and the help-desk imports
pass against recorded API answers; no check needs a real provider account. An assistant runs the
whole flow through MCP without ever handling a password.

## M15 — Analytics

Status: done in `v0.0.11`; umbrella issue #42, decisions in `architecture.md` › Analytics
(M15). Ships in six steps; M15.1 first, then M15.2, M15.3 and M15.5 in parallel, then M15.4 (after M15.2; its broadcast part
after M13.3) and M15.6 (after M15.2 and M15.3).

- **M15.1 Count the web:** properties, the cookieless collect endpoint with daily salts and shortened
  addresses, GeoIP, bot filtering, partitions and rollups, the ~1 KB script and `useyuva/analytics` with
  React and Nuxt helpers, overview, realtime, pages, sources, locations and devices with filters and
  comparisons, consent mode, usage and limits, the privacy guide.
- **M15.2 Events, goals, funnels:** custom and built-in events, props, revenue, goals, funnels, segments,
  annotations.
- **M15.3 Apps and servers:** Swift `YuvaAnalytics` and Kotlin `yuva-analytics` with offline queues and
  store privacy answers, server-side events and the Go helper.
- **M15.4 Support, alerts, reports, MCP:** support load per page next to traffic, docs ratings and
  discussions with views, broadcast campaigns to visits, alerts, insights, e-mail reports, MCP tools.
- **M15.5 Sharing and export:** read-only share links with password, expiry and embedding; CSV and
  NDJSON exports.
- **M15.6 Consent features:** a stored id and identified users only with consent: returning visitors,
  retention, cross-day funnels, a contact's recent visits.

Accept: a demo site with the script and the Swift and Kotlin samples report pageviews, screens and a
revenue goal that appear in Realtime and, the next day, from rollups with the same numbers; the
browser holds no cookie or storage entry and Postgres holds no IP address; GPC stops collection; a
children's property keeps country only; the Support report ranks pages by conversations per 1,000 views
without joining any visitor to a contact; Claude Code answers a traffic question through the MCP tools;
a share link shows only its views; after consent and identify a contact's page lists their visits and
without consent it never does; another workspace sees none of it; the load test meets the targets at
10M events a month.

## M16 — API monitoring

Status: withdrawn; #63 closed as not planned and its pull request #72 closed unmerged.

## M17 — Home and one setup

Status: done in `v0.0.15`; every Accept item verified on 2026-10-23 at `6de390b` after the Home fix (#127, #162); issue #74, decisions in `architecture.md` › Panel (`web/`) › Home and setup (M17).

- `GET /v1/home` with inbox counts, visitors per property (today, last 30 minutes, seven days), the
  last broadcast and the setup checklist shared with `get_setup_state`.
- `/setup` as one hub of six cards with done states; the analytics and newsletter flows inside it.
- Home at `/` for owners and admins, the inbox at `/conversations`; agents keep the inbox at `/`.

Accept: a new workspace's owner lands on Home, sets up chat and analytics from `/setup` and sees both
turn done; Home shows the inbox counts, each site's visitors and the last broadcast from one request;
an agent opens the inbox at `/` and never sees an inbox or property they may not see.

## M18 — Feedback on its own

Status: done in `v0.0.25`. API, panel, SDKs and progress verified on 2026-10-19 with the Swift and Kotlin samples against a server; the widget shows Resolved for a closed feedback (`5a73a1e`) and the SDK threads keep the resolved line when a progress is set (`2e344c1`). Issue #93, decisions in `architecture.md` › Feedback (M18).

- `kind` on `GET /v1/conversations/counts` and `GET /client/v1/conversations`; `can_rate` false for
  feedback.
- A Feedback section in the panel: Open and Resolved, category tabs with counts, triage rows, the
  conversation beside the list; Conversations without feedback; Resolve, Reopen, Reply and resolve.
- Swift and Kotlin: Messages and Feedback tabs, feedback states (Sent, Answered, Resolved), the
  resolved line in the thread, View feedback after sending. The widget calls a resolved feedback
  Resolved.
- Progress on feedback (Planned, In progress, Done, Won't do) set in the panel, shown to the contact
  in the list and the thread; the Swift and Kotlin views redrawn as a native messaging app.

Accept: in the Swift and Kotlin samples a user sends a bug report with a screenshot and finds it under
Feedback as Sent; a member finds it in the panel's Feedback section under Bug, not in Conversations,
answers it and the app shows Answered, then resolves it with Reply and resolve and the app shows
Resolved with no rating asked; the user writes again and it is open in the panel again; an agent sees
only feedback of inboxes they may see; a member sets In progress and then Done and the app shows each
in the row and as a line in the thread.

## M19 — Status pages

Status: done in `v0.0.25`; decisions in `architecture.md` › Status pages (M19). Built in this order, each its own
issue; each part is done when its Accept holds on a running instance.

### M19.1 — Pages, components and incidents

Status: done, issue #95; every Accept item verified on 2026-10-19 against a server built from `015570a`.

- Tables, `/v1/status-pages` with components, groups, incidents, maintenance and updates; scopes
  `status:read`, `status:manage`; stored events and webhook types; MCP tools.
- The public page rendered by the server: `/status/{slug}`, history, incident pages, Atom feed, badge,
  `GET /client/v1/status/{slug}`; en, tr, de; cache and rate limit.
- Panel: a Status section per inbox's page with components, incidents, maintenance, preview and
  publish.

Accept: an owner makes a page with two components, publishes it and opens `/status/{slug}` signed out;
an agent opens an incident that sets one component to `major_outage`, posts an update and resolves it,
and the page, the feed, the badge and the JSON follow each step; a scheduled maintenance starts and
completes on its own; an agent of another inbox sees neither the page nor its incidents in the panel or
the API.

### M19.2 — Heartbeats and pushed states

Status: done, issue #96; every Accept item verified on 2026-10-19 against a server built from `015570a`.

- `heartbeat` and `push` monitors with `/ingress/heartbeat/{token}` and `/ingress/status/{token}`
  (own JSON, Alertmanager, Grafana); thresholds, draft or published incidents on failure; member
  notifications; `status.monitor.down` and `.up`.

Accept: a heartbeat monitor whose beats stop sets its component to its failure state and opens a draft
incident after the grace; beats again bring it back; an Alertmanager firing body fails a push monitor
and its resolved body recovers it; a member who set a state by hand later than the monitor keeps it.

### M19.3 — Status in the widget, the apps and the inbox

Status: done, issue #97; widget, panel and API verified at `410afaf`, the Swift and Kotlin samples on 2026-10-19 at `b9d23c6`.

- `status` on the client inbox and `channel.updated`; the line in `<yuva-chat>`, Swift and Kotlin.
- Linking conversations to an incident, its list of conversations, "Reply to linked".

Accept: while an incident is published the widget and both sample apps show its line and it goes away
when it is resolved; a member links three conversations (e-mail, chat, app), replies once with "Reply
to linked" and each contact gets the reply on their own channel.

### M19.4 — E-mail subscribers

Status: done, issue #98; every Accept item verified on 2026-10-19 at `410afaf`; notice content follow-ups in #101.

- `mailing_lists.kind` `status` and its rules (no audiences, imports, public forms, İYS export or
  opt-out register); per-component choices.
- `/status/{slug}/subscribe` with double opt-in, limits and the sweep of unconfirmed contacts; `/s/` and
  `/m/` for status lists.
- Notices: `notify`, the 60-second hold, coalescing, the 15-minute gap and 10 an hour per page; sending
  through the inbox's channel with list headers; bounces, complaints and replies linked to the incident.
- Panel: e-mail subscribers setting, Subscribers tab, Notify subscribers with counts, notice lines and
  Undo. Guides in en, tr, de.

Accept: a simulator address subscribes from the page, confirms, gets a notice for a published incident
and its resolution and none for a draft or an update sent without notifying, and unsubscribes with one
click.

### M19.5 — Outside checks

Status: done, issue #99; every Accept item verified on 2026-10-19 at `e7939af`.

- `http` and `tcp` monitors with the outbound address rules, keyword, certificate expiry, response
  times in `status_checks` and their chart in the panel; hosted limits.

Accept: an `http` monitor on a URL that starts answering 500 fails after its threshold and recovers;
a monitor whose URL resolves to a private or metadata address is refused when saved and at check time;
the panel shows the response times of the last 24 hours.

### M19.6 — Own domains

Status: done, issue #100; verified on 2026-10-19 at `9290284`: behind Caddy on demand, and on useyuva.com with a Cloudflare custom hostname and certificate active.

- `custom_domain` with CNAME and TXT checks; `YUVA_STATUS_DOMAINS=ask` with `/status-domains/check` and
  the Caddy example in `deploy/`, and `cloudflare` with Cloudflare for SaaS custom hostnames.

Accept: a page on a verified domain answers at `/` behind Caddy with a certificate issued on demand, and
on a server behind Cloudflare with the custom hostname active; an unverified or removed domain gets 404
from the check and is not (or no longer) a custom hostname.

## M20 — Help center

Status: done in `v0.0.27`; decisions in `architecture.md` › Help center (M20).

- Help centers per inbox with collections and articles written in Markdown in the panel, drafts,
  revisions, publishing and translations per locale; scopes `help:read`, `help:write`,
  `help:publish`; owners and admins publish, agents write drafts, and the center setting
  `agents_may_publish` lets agents publish; events and webhook types; MCP tools that search and
  draft and never publish.
- Public help pages rendered by the server at `/help/{slug}` (or `YUVA_HELP_URL`) with search,
  collections, articles, "Was this helpful?", hreflang and a sitemap; en, tr, de words; cache and
  rate limits. Own domains are a later step, as are help-article imports from Zendesk and Help Scout.
- Postgres full-text search per language with prefix matching, shared by the pages, the widget, the
  panel and MCP.
- `/client/v1/channels/{key}/help*`: Home search and a Help app in `<yuva-chat>` with best matches for
  the page, collections, the article view and "Was this helpful?"; shadcn registry items.
- Article cards: members send an article in a conversation; chat and app show a card, e-mail a link.
- A Help report: top articles, helpful share, conversations started from an article, searches with no
  result.

Accept: an owner creates a help center for an inbox, a member writes an article in English with an
image, saves a draft and cannot publish it, an owner publishes it, then a Turkish translation is added
and published; signed out, `/help/{slug}/en/` and `/help/{slug}/tr/` show it with hreflang links, and
a search for a word in it and for the first letters of a word in it finds it in each language; an
unpublished article answers 404; with `agents_may_publish` on, an agent publishes; on a test page
`<yuva-chat>` shows Home search and the Help tile, finds the article, opens it, counts a "yes", and a
visitor who then writes from the article starts a conversation that the panel shows with the article
and that the report counts; a member sends the article in a chat conversation and in an e-mail
conversation, and the widget shows a card and the mail a link; a search with no result appears in the
report the next minute without the visitor's address or any id; Claude Code drafts an article through
MCP and cannot publish it; an agent of another inbox and another workspace see none of it in the
panel, the API or MCP; 31 searches in a minute from one address get `429`.

## M21 — Roadmap board

Status: done in `v0.0.27`; issue #104, decisions in `architecture.md` › Roadmap board (M21).

- One board per inbox, private or public. Ideas are discussions of kind `idea` in five stages (Ideas,
  Planned, In progress, Shipped, Closed), with votes, replies and follows from reader accounts or
  identity-token users, and suggestions moderated by default.
- Feedback linked to ideas, many to one. A stage change sets the linked feedback's progress (planned,
  in progress, done, won't do). "Reply to linked", shared with incidents, is offered when an idea ships.
- Scopes `roadmap:read`, `roadmap:write` and `roadmap:stage` (never implied). `discussion.stage_changed`
  as an event and webhook type. MCP tools that list ideas, link feedback and move a stage only with the
  named scope and a confirmed count.
- Panel: a Roadmap view with columns per stage and "Link to idea" on feedback. Widget: a Roadmap tile in
  Home for public boards. shadcn items on the discussion hooks.
- A public roadmap page rendered by the server at `/roadmap/{slug}`, with words in en, tr and de, cache
  and rate limits.

Accept: an owner creates a board for an inbox and makes it public. On a test page `<yuva-chat>` shows the
Roadmap tile, and an anonymous visitor can browse the stages but cannot vote. They sign in with a reader
account in the popup and suggest an idea, which waits for approval. A member approves it, and a second
reader with an identity token votes for it at once. In the panel a member links three feedback
conversations (two from the Swift and Kotlin samples, one page feedback) to the idea and moves it to
Planned. The confirmation names three feedback, and each app shows Planned in its row and thread within
a minute. A feedback whose progress a member set by hand afterwards keeps it until the next move. The
member moves the idea to Shipped and each linked feedback shows Done. "Reply to linked" sends one reply
that each contact gets on their own channel, and "Reply to linked" on an incident still works as before.
The followers' digest e-mail says the idea shipped. Signed out, `/roadmap/{slug}` shows the idea under
Shipped with its date, and a private board answers 404. Claude Code lists ideas and links a feedback
through MCP. Without the named `roadmap:stage` it cannot move a stage, and with it, it moves one only
with the confirmed count. A documentation page's discussion list and the Docs counts show no idea. An
agent of another inbox and another workspace see none of it in the panel, the API or MCP. A 201st
pending suggestion on one board gets `429`.

## M22 — What's new

Status: done in `v0.0.27`; issue #105, decisions in `architecture.md` › What's new (M22).

- A What's new feed per inbox. Entries in its own table, written in Markdown in the panel, with labels
  (New, Improved, Fixed), an optional image, drafts, scheduled publishing and translations per locale.
  Scopes `changelog:read`, `changelog:write` and `changelog:publish`. Owners and admins publish, or
  agents with `agents_may_publish`. Events and webhook types. MCP tools that draft and never publish.
- A What's new tile in the widget's Home with an unread count kept in the visitor's browser per channel.
  A launcher badge only with the channel setting `announce_updates`.
- Public pages rendered by the server at `/changelog/{slug}`, with an Atom feed per locale, hreflang,
  a sitemap, words in en, tr and de, cache and rate limits.
- "Mail this to subscribers": a broadcast draft from the entry on the product-update template for chosen
  lists, reviewed and sent by the unchanged broadcast flow.

Accept: an owner creates a What's new feed for an inbox. A member writes an entry in English labelled
Fixed, with an image, and cannot publish it. An owner schedules it five minutes ahead and it publishes on
time. A Turkish translation is added and published. Signed out, `/changelog/{slug}/en/` and `/tr/` show
it with hreflang links, both Atom feeds carry it, and a draft entry answers 404. On a test page
`<yuva-chat>` shows the What's new tile with 1 unread for an anonymous visitor, and no launcher badge.
After `announce_updates` is turned on, the launcher shows the badge. Opening the entry clears both, a
reload keeps them cleared, and a second browser still shows 1. Publishing another entry raises the count
in the open widget without a reload. An owner mails the entry to a list: a broadcast draft opens in the
composer from the product-update template with the entry's title, image and text. The review passes, a
test lands in Mailpit, and the broadcast is sent and unsubscribed from exactly as any broadcast. A status
list is not offered. Claude Code drafts an entry and a German translation through MCP and cannot publish
or schedule it. A key with the named `changelog:publish` publishes one. An agent of another inbox and
another workspace see none of it in the panel, the API or MCP. 121 page requests in a minute from one
address get `429`.

## M23 — MCP coverage

Status: released in `v0.0.32`; every Accept item verified on 2026-10-23 at `bafdcc9` (#128); issues #107, #108, #109,
decisions in `architecture.md` › MCP coverage (M23).

- Sender identity, e-mail channel and deliverability records, mailing lists, subscribers and CSV
  imports through MCP, with consent and attestation taken from the person and a confirmed count to
  start an import.
- E-mail images from a public URL or bytes, so a design gets the sender's logo.
- Designs and broadcast control (pause, resume, unschedule, cancel), members for assigning,
  contacts and their notes, starting a conversation, drafts waiting for review, labels and canned
  replies, discussion replies and answers, asking the operator to allow broadcasts, and creating status pages, help centers, changelogs, roadmap boards and
  funnels.
- Left out on purpose: credentials, access, webhooks, workspace settings, deletes, exports, passwords
  and every publish a milestone keeps for members.

Accept: Claude Code, connected as an owner to a fresh workspace, creates an inbox with an e-mail
channel, sets the sender identity from values the person typed, shows the DNS records, creates a
mailing list, imports a 3-row CSV with the attestation the person typed and starts it only with the
confirmed count, uploads a logo from a public URL into an `announcement` design, and drafts a
broadcast whose review has no `sender_identity_missing`. A URL on a private address is refused. An
agent sees none of the owner-only tools; a token without `lists:write` gets `insufficient_scope`
naming it. `start_conversation` with `send: true` leaves a draft while `bots_may_send` is off.

## M24 — Tidy for 0.1.0

Status: done on 2026-10-23; items Y-117 to Y-125 and Y-133 in `backlog.md` (Y-124 withdrawn).

- The roadmap, `architecture.md`, `CLAUDE.md` and the guides say what the code does today; nothing
  internal in the repository.
- Unused generated queries and an empty Go package removed; every string translated in every SDK.

Accept: every Status line names the version it shipped in or says what is left; a grep for internal
paths finds none; every check in the `CLAUDE.md` table passes.

## M25 — Acceptance for 0.1.0

Status: done on 2026-10-23; items Y-126 to Y-128, Y-162 and Y-177 in `backlog.md`.

- The Accept items of M6.1, M17 and M23, the milestones released without them, run on a server built
  from `main`.

Accept: each run reports pass with evidence; each failure becomes its own backlog item and is fixed
before M26.

## M27 — Scale and safety

Status: done on 2026-10-23 (#134–#137, #178, #179); items Y-134 to Y-137, Y-178 and Y-179 in `backlog.md`. Part of 0.1.0.

- Rate limits counted in Postgres so several replicas keep them; a workspace audit log; attachment
  limits per channel; OpenTelemetry tracing; deleting a conversation through a 30-day trash and
  redacting a single message.

Accept: two replicas on one database refuse the same request the same way one does; an owner reads who
created, revoked and deleted what in Settings › Audit log, with no content or secret in it; a chat
channel with a 2 MB limit refuses a 3 MB file in the widget; a trace of a reply shows the request, the
job and the SMTP send; an owner trashes a conversation, restores it, trashes it again and it is gone
for good after 30 days, while an agent cannot delete; a redacted message shows as removed in the panel
and the widget and its attachment no longer downloads.

## M28 — Team workflow

Status: done on 2026-10-23 (#138–#144); items Y-138 to Y-144 in `backlog.md`. Part of 0.1.0.

- Teams with inbox access and assignment, automatic assignment, SLA policies on business hours,
  automation rules, macros, saved views and custom fields.

Accept: a new chat conversation is assigned round robin to an online member of the inbox's team, never
over their cap; its first-reply target shows in the list and its breach notifies the assignee and fires
`conversation.sla_breached`; a rule labels and assigns a mail whose subject contains "invoice" without
looping on its own change; a macro replies, labels and closes in one step with undo; a shared view
shows each member only what they may see; a required custom field blocks closing without it.

## M29 — E-mail and reports

Status: done on main (#145–#147), verified on 2026-10-23 on a running server with Mailpit and signed ingress (DMARC pass and none); real DNS, DMARC and SMTP delivery wait for the hosted server.

- Cc and Bcc on replies with participants kept on the conversation; forwarding a conversation to an
  outside address; a Reports section from daily rollups with CSV export.

Accept: a mail with two Cc addresses becomes one conversation whose reply reaches all three, and a Cc'd
person's answer joins it; a forwarded conversation's reply comes back into it; the Reports section shows
first-reply and resolution times per inbox and member that match `/v1/stats` for today, and an agent
sees only their inboxes.

## M30 — Own domains and content

Status: done on main (#148–#150), verified on 2026-10-23 on a running server; own domains with the domain marked verified in the database (real DNS behind Caddy and Cloudflare, and an import from a real Zendesk Guide, wait for the hosted server and the owner's accounts).

- Own domains for help centers, roadmap boards and What's new; feedback categories per inbox; help
  articles imported from Zendesk and Help Scout.

Accept: a help center answers on a verified domain behind Caddy and behind Cloudflare; an inbox's own
feedback category appears in the widget and both sample apps without a release; a recorded Zendesk Guide
imports as drafts with their images.

## M31 — More help desks

Status: done on main (#151–#156), verified on 2026-10-23 with the recorded-answer end-to-end tests against Postgres and the live API's token and access checks; plan by `yuva-migration`. A server kill mid-import was not repeated for the new desks.

- History imports from Front, Freshdesk, Intercom, Crisp and Chatwoot on the M14 engine, read-only, with
  the wizard, guides and MCP tools.

Accept: each import passes against recorded API answers with no real account; killing the server
mid-import loses and duplicates nothing; the inbox shows the history with original dates, nothing unread
and no notification, webhook or push.

## M32 — Integrations and push

Status: done on main (#157–#159), verified on 2026-10-23 on a running server: GitHub live against a fake GitHub Enterprise, Slack, Linear and APNs/FCM delivery through the end-to-end tests with fake providers; a real Slack app and pushes to real devices wait for the owner's keys.

- Slack notifications and replies, feedback linked to GitHub and Linear issues, push sent by Yuva itself
  through APNs and FCM.

Accept: a new conversation appears in Slack and a reply in its thread becomes a note by the linked
member; closing the linked GitHub issue sets the feedback to Done; with APNs and FCM keys on the app
channel, the Swift and Kotlin samples get the reply push while closed.

## M33 — Single sign-on for members

Status: done on main (#160, #161), verified on 2026-10-23 with Keycloak 26 by OIDC and SAML in a real browser; the domain claim was marked verified in the database (no public DNS here). SSO required is enforced on workspace access, not at sign-in (`architecture.md` › Single sign-on).

- OpenID Connect and SAML sign-in for a workspace's verified e-mail domains, with optional just-in-time
  membership.

Accept: a member of a claimed domain signs in through Keycloak by OIDC and by SAML; with SSO required, a
session that did not come through the workspace's SSO is refused in that workspace (owners exempt) while
the person can still sign in by code as a reader; every sign-in is in the audit log.

## M26 — Release 0.1.0

Status: released in `v0.1.0` on 2026-10-24 (#129–#132, security findings #180–#197); a clean install from `install.md` serves `/v1/version` 0.1.0.

- A security review of everything added since the 0.0.1 review (#15).
- One version, `0.1.0`, in the server, the contract, every SDK, the chart, the MCP manifests and the
  guides; release notes and a post on the site.

Accept: `v0.1.0` is tagged; the image, npm package, JitPack and SwiftPM builds and the MCP registry
entry all report `0.1.0`; a fresh install from `docs/install.md` works on a clean host.

## M34 — Mail inboxes

Status: done on main (#163–#165), verified on 2026-10-24 on a server built from main. Decisions in `architecture.md` › Team mail.

- An inbox's purpose, `support` or `mail`; a Mail section in the panel with folders; choosing support,
  mail or both when a workspace starts.

Accept: an owner of a new workspace chooses team mail and lands on `/mail`; a mail to the inbox's
address appears under Inbox, is archived, starred and trashed with undo, and never shows in
Conversations, support stats or a rating request.

## M35 — Writing mail

Status: done on main (#166–#169), verified on 2026-10-24 on a server built from main with two API replicas; the address proof and the deliverability check were marked passed in the database (no DNS here).

- New mail to many recipients with Cc and Bcc, sending as the member, signatures, member drafts and
  scheduled send.

Accept: a member writes to two people with one Cc from their verified own address with their signature;
all three replies join the conversation; a draft started on a phone opens on a desktop; a mail scheduled
for five minutes ahead goes once with two replicas.

## M36 — Personal mailboxes

Status: done on main (#170–#172), verified on 2026-10-24 on a server built from main.

- Mailboxes that belong to one member, delegation, and what happens when a member leaves.

Accept: an owner of the workspace gets `404` on every conversation of a member's personal mailbox in the
panel, the API, realtime, search and MCP; a delegate reads and sends on behalf, and loses access at once
when revoked; removing the member hands the mailbox to a named member, and the audit log has each step.

## M37 — Organizing mail

Status: done on main (#173, #174), verified on 2026-10-24 in the panel and through MCP.

- Mail filters on the automation rules engine and search operators.

Accept: a filter archives and labels mail from a list; `from:` and `has:attachment` find the right
conversations in the panel and through MCP.

## M38 — Team mail acceptance

Status: done on main (#175, #176), verified on 2026-10-24.

- Guides in en, tr, de and the verifier's run of M34 to M37.

Accept: each run reports pass with evidence; each failure becomes its own backlog item.

## M39 — Bulk actions

Status: done; items Y-198 to Y-200 in `backlog.md`. Independent of team mail; ran alongside it.

- Deleting conversations for good and emptying the Trash, deleting contacts from the panel, and
  selecting many in the content lists.

Accept: an owner empties a Trash of 250 conversations and deletes 30 selected contacts with one
confirmation each, and the audit log has both; 20 help center articles are unpublished at once and
the undo puts them back.

## Later

- M10, native apps for members (#18, #19): not scheduled; not part of 0.1.0.

## 0.1.0 summary

| Milestone | Items | IDs | api | web | sdk | docs | verify | release |
|---|---|---|---|---|---|---|---|---|
| M24 Tidy | 9 (+1 withdrawn) | Y-117–Y-125, Y-133 | 1 | – | 2 | 6 | – | – |
| M25 Acceptance | 3 | Y-126–Y-128 | – | – | – | – | 3 | – |
| M26 Release | 4 | Y-129–Y-132 | 1 | – | – | 1 | – | 2 |
| M27 Scale and safety | 6 | Y-134–Y-137, Y-178–Y-179 | 6 | – | – | – | – | – |
| M28 Team workflow | 7 | Y-138–Y-144 | 5 | 2 | – | – | – | – |
| M29 E-mail and reports | 3 | Y-145–Y-147 | 2 | 1 | – | – | – | – |
| M30 Own domains and content | 3 | Y-148–Y-150 | 3 | – | – | – | – | – |
| M31 More help desks | 6 | Y-151–Y-156 | 6 | – | – | – | – | – |
| M32 Integrations and push | 3 | Y-157–Y-159 | 3 | – | – | – | – | – |
| M33 Single sign-on | 2 | Y-160–Y-161 | 2 | – | – | – | – | – |
