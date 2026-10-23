# Backlog

Every work item, numbered once in build order. `Y-N` is GitHub issue `#N`; IDs are never reused, and
a withdrawn item stays here marked withdrawn. Work before `Y-117` is tracked in the milestone issues
#1–#116 listed in `roadmap.md`.

## M24 — Tidy for 0.1.0

### Y-117 · Roadmap status lines match the code

- **Area** docs · **Owner** orchestrator · **Priority** P0 · **Depends on** – · **Chapters** roadmap
- **Acceptance**
  - M6.1: API, widget, Swift and Kotlin SDKs and panel done; acceptance pending (Y-126).
  - M8: done in `v0.0.1`, without the `CHANGELOG.md` pointer.
  - M17 and M18: done, each with the first tag that carries it.
  - M19: done; the Status lines of M19.3 and M19.6 come before their bullets like the other parts.
  - M13 and M14: the tag that carried them next to the deploy date.
  - M16: a one-line entry saying it was withdrawn (#63 closed as not planned, #72 closed).
  - M10: no version in the heading; listed under Later.

### Y-118 · `architecture.md` matches the code

- **Area** docs · **Owner** orchestrator · **Priority** P1 · **Depends on** – · **Chapters** architecture
- **Acceptance**
  - The panel's Lingui catalogs are the six in `web/lingui.config.ts` (now says three).
  - The mobile SDK summary lists rating, status line, subscriptions, shake feedback and analytics.
  - The OAuth and token passages that name "member apps (M10)" say these apps are planned, not
    present.

### Y-119 · Nothing internal in the repository; `CLAUDE.md` current

- **Area** docs · **Owner** orchestrator · **Priority** P0 · **Depends on** – · **Chapters** –
- **Acceptance**
  - `.claude/agents/yuva-video.md` names no path on our servers or the Mac; it says where to look
    in words (the ops notes, the Mac clone) without the path.
  - `CLAUDE.md` lists `yuva-video` among the agents.
  - The `CLAUDE.md` check table covers what CI runs: govulncheck, the generated-code diff, Helm lint
    and kind install, the `examples/*` builds and the Dovecot migration tests.
  - `git grep -nE '/srv/|/Users/|server1'` outside test fixtures finds nothing.
- **As built**
  - `.claude/agents/yuva-video.md` is excluded in `.git/info/exclude` and was never in the
    repository, so it keeps its paths and stays out of the public agent list.

### Y-120 · Configuration reference covers every server setting

- **Area** docs · **Owner** yuva-docs · **Priority** P1 · **Depends on** – · **Chapters** configuration
- **Acceptance**
  - `YUVA_STATUS_CANARY_URLS` and `YUVA_STATUS_MIN_INTERVAL` are in `docs/configuration.md` and its
    `tr` and `de` copies, with defaults from `api/internal/config/config.go`.
  - Every `YUVA_*` variable read by the server (`YUVA_SMTP_*`, `YUVA_SES_*`, `YUVA_MICROSOFT_*`,
    `YUVA_MAIL_PREVIEW_DIR`) is documented or listed as internal; CLI and test-fixture variables
    stay out.

### Y-121 · Stale pointers in README, examples and the changelog

- **Area** docs · **Owner** yuva-docs · **Priority** P2 · **Depends on** – · **Chapters** –
- **Acceptance**
  - `README.md` points "What changed" at GitHub Releases.
  - `examples/README.md` drops the "0.0.7 or later" constraint.
  - `CHANGELOG.md` stays frozen at 0.0.3 and says so in its first lines.

### Y-122 · Swift SDK German strings

- **Area** sdk · **Owner** yuva-mobile · **Priority** P1 · **Depends on** – · **Chapters** –
- **Acceptance**
  - The 15 keys without `de` in `sdk/swift/Sources/YuvaKit/Resources/Localizable.xcstrings`
    (`conversations.messages`, `feedback.attachments`, `feedback.resolvedNote`, `feedback.state.*`,
    `feedback.view`, `feedback.progressNote.*`) have German, worded like the Kotlin `de` strings.
  - `swift build && swift test` passes on the Mac.

### Y-123 · Unused queries and the empty Go package

- **Area** api · **Owner** yuva-api · **Priority** P2 · **Depends on** – · **Chapters** –
- **Acceptance**
  - The 13 queries no code calls are removed from `api/internal/store/*.sql` and regenerated:
    `CountPublicIdeas`, `FindThreadSentTo`, `GetPersonReaderSettings`, `IsChatVisitor`,
    `ListBroadcastsByIDs`, `ListMigrationFolderSeen`, `LockChangelogEntryByID`,
    `MarkEmailImagesSent`, `SetConversationSpam`, `SetMigrationFolderLabel`, `TouchHelpCollection`,
    `TouchRoadmapBoard`, `TrimAnalyticsDaily`; a query that turns out to be needed is used instead.
  - The empty `sdk/go/health/` directory is gone.
  - The `api/` and `sdk/go/` checks pass.
- **As built**
  - `TrimAnalyticsDaily` stays: the analytics retention job calls it through a function list.

### Y-124 · Unused shadcn primitives in the panel (withdrawn)

- **Area** web · **Owner** yuva-panel · **Priority** – · **Depends on** – · **Chapters** –
- Withdrawn on 2026-10-23: the owner keeps the unused primitives in `web/src/components/ui/`.

### Y-125 · Release stage wording

- **Area** docs · **Owner** yuva-docs · **Priority** P1 · **Depends on** owner decision · **Chapters** –
- **Acceptance**
  - `SECURITY.md`, `site/src/copy/contact.ts` and the README use the stage the owner chose for
    0.1.0 instead of "pre-alpha", in `en`, `tr` and `de`.
  - `SECURITY.md` › Supported versions names the latest 0.1.x.

### Y-133 · Mobile SDK strings in every locale

- **Area** sdk · **Owner** yuva-mobile · **Priority** P1 · **Depends on** Y-122 · **Chapters** –
- **Acceptance**
  - Every key in `sdk/swift/Sources/YuvaKit/Resources/Localizable.xcstrings` has a value in all 47
    locales (now 15 keys missing in each of 45).
  - No string in `sdk/kotlin/yuva/src/main/res/values-*/strings.xml` repeats the English text unless
    the word is the same in that language (e.g. "Feedback" in German); the Messages tab, feedback
    states, notes and "View feedback" are translated in all 45.
  - Swift and Kotlin use the same wording per locale.
  - The `sdk/swift` and `sdk/kotlin` checks pass on the Mac.

## M25 — Acceptance for 0.1.0

### Y-126 · M6.1 satisfaction ratings on a running server

- **Area** verify · **Owner** yuva-verifier · **Priority** P0 · **Depends on** – · **Chapters** roadmap M6.1
- **Acceptance**
  - The M6.1 Accept: a widget visitor and an e-mail contact each rate a closed conversation; the
    panel shows both; a webhook receiver gets `conversation.rated`; a second rating of the same close
    gets `409`.
  - Screenshots (WebP) of the widget rating card, the e-mail rating page and the panel thread.

### Y-127 · M17 Home and setup on a running server

- **Area** verify · **Owner** yuva-verifier · **Priority** P0 · **Depends on** – · **Chapters** roadmap M17
- **Acceptance**
  - The M17 Accept, including that an agent at `/` sees no inbox or property they may not see.
  - Screenshots of Home for an owner, `/setup` before and after, and `/` for an agent.

### Y-128 · M23 MCP coverage with Claude Code

- **Area** verify · **Owner** yuva-verifier · **Priority** P0 · **Depends on** – · **Chapters** roadmap M23
- **Acceptance**
  - The M23 Accept on a fresh throwaway workspace, test mail to simulator addresses only; the
    workspace is deleted afterwards.

### Y-162 · Home shows each site's visitors and the last broadcast from one request

- **Area** web · **Owner** yuva-panel · **Priority** P0 · **Depends on** – · **Chapters** architecture
- **Acceptance**
  - Found by Y-127: the panel redesign (`513aa3bb`) dropped `PropertyCard` and the last-broadcast
    block from Home, and Home fetches about 20 endpoints next to `GET /v1/home`.
  - Home shows, in the redesigned layout, visitors today, in the last 30 minutes and per day for
    seven days for each property in `home.analytics`, and the last sent broadcast's recipients,
    sent, clicked and replied from `home.broadcasts`.
  - Home's own content reads only `GET /v1/home`; requests the sidebar and top bar make on every
    page stay. The network log on `/` lists them and nothing else.
  - An agent's `/` is unchanged (the inbox).
  - The `web/` check passes; screenshots of Home for an owner with two properties and a sent
    broadcast, light and dark.

### Y-177 · Deleting your account right after deleting your workspace answers 500

- **Area** api · **Owner** yuva-api · **Priority** P1 · **Depends on** – · **Chapters** architecture
- **Acceptance**
  - Found by the M17 re-run: `DELETE /v1/me` sent right after `DELETE /v1/workspace` (or `yuva
    workspace delete`) answered `500`, the server log showing `deadlock detected (SQLSTATE 40P01)`.
  - The cause is found and named in the commit; the two deletes in that order succeed, and a test
    runs them back to back against Postgres.
  - The `api/` check passes with `YUVA_TEST_DATABASE_URL` set.

## M26 — Release 0.1.0

### Y-129 · Security review since 0.0.1

- **Area** api · **Owner** yuva-api · **Priority** P0 · **Depends on** Y-123 · **Chapters** architecture
- **Acceptance**
  - Reviewed: OAuth and MCP, scoped keys, reader accounts, status page own domains, list imports
    and consent, e-mail image uploads, migrations' source credentials, analytics ingest.
  - Each finding that affects correctness or security is its own backlog item, fixed before Y-132.

### Y-130 · Version 0.1.0 everywhere

- **Area** release · **Owner** orchestrator · **Priority** P0 · **Depends on** M24, M25, M27–M33, Y-129 · **Chapters** –
- **Acceptance**
  - `0.1.0` in `openapi/openapi.yaml`, `sdk/js/package.json`, `sdk/swift/Sources/YuvaKit/Yuva.swift`,
    `sdk/kotlin/gradle.properties`, `deploy/helm/yuva/Chart.yaml` (version and appVersion),
    `deploy/mcpb/manifest.json`, `server.json`, the site's `VERSION`, `README.md`, `docs/mobile.md`
    and the `docs/install.md` build example.
  - `git grep -n '0\.0\.32'` finds only history (blog posts, release notes).
  - Every check in the `CLAUDE.md` table passes.

### Y-131 · Release notes and the 0.1.0 post

- **Area** docs · **Owner** yuva-docs · **Priority** P1 · **Depends on** Y-130 · **Chapters** –
- **Acceptance**
  - A post on the site for 0.1.0 in `en`, `tr` and `de`: what Yuva does today, how to install, what
    0.1 means for upgrades.
  - The site check passes.

### Y-132 · Tag and publish 0.1.0

- **Area** release · **Owner** orchestrator · **Priority** P0 · **Depends on** Y-130, Y-131 · **Chapters** –
- **Acceptance**
  - `v0.1.0` tagged after the owner's go-ahead; GitHub Release notes from changelogithub.
  - The container image, npm `useyuva`, JitPack, SwiftPM and the MCP registry entry report `0.1.0`.
  - A fresh install from `docs/install.md` on a clean host serves `/healthz` and `/v1/version`
    with `0.1.0`.

## M27 — Scale and safety

### Y-134 · Rate limits shared across replicas

- **Area** api · **Owner** yuva-api · **Priority** P0 · **Depends on** – · **Chapters** architecture › Server
- **Acceptance**
  - Every limit in `api/internal/api/ratelimit.go` and the per-feature limiters (sign-up, analytics
    collect and query, list imports, monitors, migration tests) counts in Postgres, not in process
    memory, so N replicas allow the same number of requests as one.
  - The hot path stays one statement per check; expired windows are swept by a River job.
  - A test runs two servers on one database and shows the 31st session request in a minute from one
    address gets `429` whichever server answers.
  - `deploy/helm/yuva/values.yaml` and `docs/operations.md` say limits hold across replicas.

### Y-135 · Workspace audit log

- **Area** api · **Owner** yuva-api, then yuva-panel · **Priority** P1 · **Depends on** – · **Chapters** architecture › Privacy and data
- **Acceptance**
  - An `audit_log` table with `workspace_id`, actor (member, API key, OAuth grant, operator CLI),
    action, target and time, written in the same transaction as: member invite, role and access
    changes, member removal, API key and webhook create/revoke/secret rotation, channel credential
    and identity secret changes, retention change, exports, deletes of inboxes, contacts and the
    workspace, broadcast approval.
  - No message content and no secret in a row.
  - `GET /v1/audit-log` for owners and keys with a new `audit:read` scope, filtered by actor, action
    and time; kept `YUVA_AUDIT_DAYS` (365).
  - Panel: Settings › Audit log for owners. `migration_audit` stays as it is.

### Y-136 · Attachment limits per channel

- **Area** api · **Owner** yuva-api, then yuva-panel · **Priority** P2 · **Depends on** – · **Chapters** architecture › Storage
- **Acceptance**
  - A channel can narrow `YUVA_ATTACHMENT_MAX_BYTES` and `YUVA_ATTACHMENT_TYPES`, never widen them.
  - The widget and the SDKs read the channel's limits from `/client/v1` and refuse a file before upload.
  - Inbound e-mail over the limit keeps the message and records the dropped attachment as now.
  - The "per-channel limits can narrow them later" line in `architecture.md` is replaced.

### Y-137 · OpenTelemetry tracing

- **Area** api · **Owner** yuva-api · **Priority** P2 · **Depends on** – · **Chapters** architecture › Server
- **Acceptance**
  - OTLP traces for HTTP requests, River jobs, SMTP sends, webhook deliveries and Postgres queries,
    off unless `OTEL_EXPORTER_OTLP_ENDPOINT` is set (standard `OTEL_*` variables).
  - No message body, address or secret in span attributes; workspace and inbox ids only.
  - Log lines carry the trace id when tracing is on.
  - `docs/operations.md` and `docs/configuration.md` (with `tr`, `de`) document it.

## M28 — Team workflow

### Y-138 · Teams

- **Area** api · **Owner** yuva-api, then yuva-panel · **Priority** P0 · **Depends on** – · **Chapters** architecture › Concepts
- **Acceptance**
  - Owners and admins create teams of members; a team can be granted inbox access like a member, and a
    member's access is the union of their own grants and their teams'.
  - A conversation can be assigned to a team, a member or both; "Unassigned" and "Mine" filters gain
    "My teams".
  - `/v1/teams`, events, webhooks and MCP `list_teams`; agents see the teams they belong to.
  - A removed member loses team access at once; an agent never sees a conversation of an inbox no
    grant gives them.

### Y-139 · Automatic assignment

- **Area** api · **Owner** yuva-api, then yuva-panel · **Priority** P0 · **Depends on** Y-138 · **Chapters** architecture › Concepts
- **Acceptance**
  - Per inbox (and per team): off, round robin or least open conversations, with a per-member cap.
  - Only members who are online, not `away` and inside the inbox's business hours are chosen; a
    conversation waits unassigned when nobody is, and is assigned when someone becomes available.
  - Reopened conversations go back to their last assignee when that member is still eligible.
  - The assignment is an event like a manual one, with webhooks; concurrent arrivals never exceed a cap
    (test with parallel inserts).

### Y-140 · SLA policies

- **Area** api · **Owner** yuva-api, then yuva-panel · **Priority** P0 · **Depends on** – · **Chapters** architecture › Concepts
- **Acceptance**
  - Policies with first-reply, next-reply and resolution targets, counted in the inbox's business hours,
    applied by inbox, channel kind, label or priority.
  - Each conversation shows its due time and state (on track, due soon, breached) in the list and the
    thread; the list sorts by due time.
  - A breach and an approaching breach notify the assignee (and team) and fire
    `conversation.sla_breached`; snoozed and pending time pauses the clock.
  - `/v1/stats` and the reports count met and breached per inbox and member.

### Y-141 · Automation rules

- **Area** api · **Owner** yuva-api, then yuva-panel · **Priority** P0 · **Depends on** Y-138 · **Chapters** architecture › Concepts
- **Acceptance**
  - Rules per workspace or inbox: a trigger (conversation created, message received, status changed,
    idle for a time), conditions (channel, kind, category, subject or body contains, contact attribute,
    label, priority, business hours) and actions (label, assign, priority, status, snooze, note, reply
    with a canned reply, webhook).
  - Rules run in a River job after the change commits, in a set order, each at most once per trigger, and
    a rule's own change never triggers a rule (no loops).
  - Every action is an event naming the rule; a rule log shows the last runs and failures.
  - A rule never sends a reply to an auto-reply or bounce (the existing loop protection holds).

### Y-142 · Macros

- **Area** web · **Owner** yuva-api, then yuva-panel · **Priority** P1 · **Depends on** – · **Chapters** architecture › Panel
- **Acceptance**
  - A macro is a canned reply plus actions (labels, assignee, status, priority, snooze), personal or
    shared, chosen from the composer's `/` menu and the command palette.
  - Applying it fills the composer and applies the actions only when the reply is sent, with the usual
    undo.
  - `/v1/macros` and MCP `list_macros`; canned replies keep working unchanged.

### Y-143 · Saved views

- **Area** web · **Owner** yuva-api, then yuva-panel · **Priority** P1 · **Depends on** – · **Chapters** architecture › Panel
- **Acceptance**
  - Any list filter and search can be saved as a view, personal or shared with the workspace or a team,
    with its count in the rail.
  - A shared view shows each member only conversations they may see.
  - `/v1/views`; views survive a renamed or deleted label (the filter drops it).

### Y-144 · Custom fields

- **Area** api · **Owner** yuva-api, then yuva-panel · **Priority** P1 · **Depends on** – · **Chapters** architecture › Concepts
- **Acceptance**
  - Owners define typed fields (text, number, date, select, checkbox) for conversations and contacts;
    contact attributes that match a field are validated by it, others stay free-form.
  - Fields show and edit in the thread sidebar, filter the conversation list and views, and appear in
    the API, webhooks and MCP.
  - A field marked required on close blocks closing without it (`409 field_required`).

## M29 — E-mail and reports

### Y-145 · Cc and Bcc on replies

- **Area** api · **Owner** yuva-api, then yuva-panel · **Priority** P0 · **Depends on** – · **Chapters** architecture › E-mail
- **Acceptance**
  - Inbound `Cc` addresses are kept on the conversation as participants; a reply goes to the contact
    and, by default, to the participants, editable per reply with Cc and Bcc fields.
  - A participant's reply joins the same conversation, not a new one; loop and spam protection hold.
  - Bcc is never shown to the contact or stored in sent headers.
  - The "no `Cc`" line in `architecture.md` › E-mail is replaced by the rule.

### Y-146 · Forward a conversation by e-mail

- **Area** api · **Owner** yuva-api, then yuva-panel · **Priority** P1 · **Depends on** Y-145 · **Chapters** architecture › E-mail
- **Acceptance**
  - A member forwards a message or the whole conversation to any address, with a note and attachments,
    from the inbox's e-mail channel.
  - A reply from that address joins the conversation as a participant message.
  - The forward is an event in the timeline; notes are never forwarded.

### Y-147 · Reports

- **Area** web · **Owner** yuva-api, then yuva-panel · **Priority** P0 · **Depends on** Y-140 · **Chapters** architecture › Panel
- **Acceptance**
  - A Reports section: conversations opened and closed, first-reply and resolution times (median, p90),
    SLA met and breached, ratings, by day or week, broken down by inbox, channel, label, team and member,
    over up to 13 months.
  - Answers come from daily rollups, not from scanning `messages`, and agree with `/v1/stats` for today.
  - CSV export of each report; agents see only their inboxes.

## M30 — Own domains and content

### Y-148 · Own domains for help centers, roadmaps and What's new

- **Area** api · **Owner** yuva-api, then yuva-panel · **Priority** P1 · **Depends on** – · **Chapters** architecture › Status pages, Help center
- **Acceptance**
  - The status page custom-domain machinery (CNAME and TXT checks, `YUVA_STATUS_DOMAINS=ask` and
    `cloudflare`) serves help centers, roadmap boards and changelogs too.
  - The domain check answers for any of them; one domain belongs to one page of one workspace.
  - The "later" lines in M20, M21 and M22 of `architecture.md` are replaced.

### Y-149 · Feedback categories per inbox

- **Area** api · **Owner** yuva-api, then yuva-panel, yuva-mobile, yuva-widget · **Priority** P2 · **Depends on** – · **Chapters** architecture › Concepts
- **Acceptance**
  - An inbox lists its feedback categories (key, name per locale, icon); the default stays `bug`, `idea`,
    `praise`, `other`.
  - `/client/v1` returns them and the widget and SDKs render them without a release.
  - A removed category keeps its feedback, shown under Other.

### Y-150 · Help article import from Zendesk and Help Scout

- **Area** api · **Owner** yuva-api, then yuva-panel · **Priority** P2 · **Depends on** – · **Chapters** architecture › Help center
- **Acceptance**
  - Categories, sections and articles with translations and images import into a help center as drafts,
    with redirects from the old URLs listed for the owner.
  - Read-only access to the source; tests against recorded API answers.
  - Guides in en, tr, de.

## M31 — More help desks

### Y-151 · Help-desk API tokens and a source registry

- **Area** api · **Owner** yuva-api · **Priority** P1 · **Depends on** – · **Chapters** architecture › Moving to Yuva
- **Acceptance**
  - The two-way `if source == zendesk … else helpscout` switches (`helpdeskSource`, `helpdeskHost`,
    `helpdeskScopes`, `syncStart`, `deskAttributes`, `presetSource`, `Helpdesk`) read one registry
    entry per help desk: API hosts, file hosts, auth kind, limit and retry headers, the limit's
    window (per minute, per 10 seconds, per day), default budget, contact attribute prefix and sync
    cursor. Zendesk and Help Scout behave exactly as before (their recorded-answer tests unchanged).
  - A help desk whose auth is `token` connects with `PUT /v1/migrations/{id}/credentials`
    (`kind: token`, the token, and for a source that needs them a second secret, an account
    subdomain or an HTTPS base URL); the secret is sealed like OAuth tokens, never answered, logged
    or put in job arguments, and deleted when the migration completes or is cancelled. The
    `set_credentials` action is listed for these sources; `sign_in` is not.
  - The token goes only in the source's own auth header (Bearer, Basic or a named header) and only
    to the source's API hosts; a team-entered base URL is dialled through `checkedDial` (private
    addresses only with the operator's `AllowPrivate`). A transport test proves that a non-`GET`,
    a non-HTTPS URL, another host, a redirect and an attachment host never receive the token.
  - `Budget` keeps a per-day class next to per-minute, and reads a limit header whose window is
    not a minute; `420` answers count as limit answers like `429`. The dry run's time estimate uses
    the daily budget when there is one.
  - Panel: the Connect step shows a token form (fields per source, the guide inline) in place of
    "Sign in"; strings in the `en`, `tr`, `de` catalogs. MCP: `create_migration` for such a source
    returns `connect_url` to that form; no tool takes the token.
  - Spec first: `MigrationAuth` gains `token`, `MigrationCredentials` documents it; Redocly lint,
    regenerate, and the generated-code diff is clean.

### Y-152 · Front history import

- **Area** api · **Owner** yuva-api · **Priority** P1 · **Depends on** Y-151 · **Chapters** architecture › Moving to Yuva
- **Acceptance**
  - Connects with an API token a Front admin creates (Settings › Developers › API tokens) with the
    feature "Access resources", the Global and Shared namespaces (Private only if personal inboxes
    move) and the permission Read only. Yuva sends it as Bearer to `api2.frontapp.com` only. Unsure:
    whether Front lets a caller read the token's permissions; if the token's claims carry them, a
    token with Write, Delete or Send is refused as `scope_too_broad`, else the guide asks for Read
    and the GET-only transport enforces it.
  - Maps shared inboxes to folders and inboxes, conversations to conversations, inbound messages
    to messages in and teammates' messages to messages out, comments to notes, tags to labels,
    teammates to members, the assignee to the assignee, contacts and their e-mail handles to
    contacts, message attachments (on the API host, with the token) to files. Status: archived or
    resolved to closed, waiting or snoozed to pending, others open; deleted and spam never read
    (confirm Front's current status fields).
  - Copy lists each inbox's conversations with Front's page token; sync reads changes through
    `GET /conversations/search/{query}` with `after:` (a `GET`, under the search guard of 40% of
    the limit) from the scan's start minus the overlap (confirm the filter).
  - Budget: half of `x-ratelimit-limit` (Starter 50, Professional 100, Enterprise 200 a minute),
    at most 2 a second per conversation for Front's per-resource guard, `retry-after` honoured
    on 429; a killed copy resumes from the stored page token without duplicates.
  - Tests against recorded answers in `helpdesktest/testdata/front` (invented data, listed in
    `NOTICE`): two inboxes, paged conversations, messages, comments, an attachment, a 429 with
    `retry-after`, and a request log showing only `GET`s to `api2.frontapp.com` with the token.
  - Panel: Front on the Source step, its token form on Connect; guide `docs/migrate-front.md` with
    `docs/tr/` and `docs/de/` (where to create and later delete the token, what is and is not
    copied, limits, the cutover). MCP: `front` in `create_migration`'s providers and the
    `move_to_yuva` prompt names where to delete the token.

### Y-153 · Freshdesk history import

- **Area** api · **Owner** yuva-api · **Priority** P1 · **Depends on** Y-151 · **Chapters** architecture › Moving to Yuva
- **Acceptance**
  - Connects with the API key of a Freshdesk admin (Profile settings › View API key) and the
    account subdomain; sent as Basic `key:X` to `<subdomain>.freshdesk.com` only. Freshdesk has no
    scopes or read-only key: the GET-only transport is the only guard, and the guide says so. The
    connection test reads `GET /api/v2/agents/me` and refuses an agent who is not an admin, as
    Zendesk does.
  - Maps tickets to conversations (the description as the first message), incoming conversations
    to messages in, replies to messages out, private and public notes to notes (public notes are
    not mailed to the requester; confirm), tags to labels, agents to members, the responder to the
    assignee, requesters to contacts, products to folders and inboxes (tickets without a product to
    the default), attachments to files. Status 2 open, 3 pending, 4 and 5 closed, custom statuses
    open. Spam and deleted tickets are not in Freshdesk's default listing and are never read.
  - Lists `GET /api/v2/tickets?updated_since=…&order_by=updated_at&order_type=asc&per_page=100`;
    Freshdesk stops a listing after 300 pages, so the reader starts a new listing at the last
    ticket's `updated_at` and dedupes by ticket id. Attachments are pre-signed URLs on another host
    that expire: fetched by the attachment client without the key, and a refused one refetches the
    conversation once for a fresh URL.
  - Budget: half of `X-RateLimit-Total` (the plan's per-minute limit; 50 a minute until the first
    answer), `Retry-After` honoured on 429; invalid requests count against Freshdesk's limit, so a
    4xx other than 404 on an item stops the slice. Resume and sync from the stored `updated_since`.
  - Tests against recorded answers in `helpdesktest/testdata/freshdesk`: a listing that hits the
    300-page cut (served small), conversations with notes, an expired attachment URL, a 429, and a
    request log with only `GET`s and the key only on the account host.
  - Panel and guide `docs/migrate-freshdesk.md` in en, tr, de (reset the API key after the move,
    since Yuva cannot revoke it); MCP `freshdesk` provider as for Zendesk.

### Y-154 · Intercom history import

- **Area** api · **Owner** yuva-api · **Priority** P1 · **Depends on** Y-151 · **Chapters** architecture › Moving to Yuva
- **Acceptance**
  - Connects with the access token of a private app the team creates in its own workspace
    (Developer Hub › Configure › Authentication) with only the read permissions: read
    conversations, read and list users and companies, read admins, read tags. Sent as Bearer to the
    workspace's region host (`api.intercom.io`, `api.eu.intercom.io` or `api.au.intercom.io`,
    chosen on Connect) with a pinned `Intercom-Version` (2.17 is current in the docs; pin what the
    recorded answers follow). Unsure: whether Intercom reports a token's permissions; the GET-only
    transport is the guard either way. OAuth is not used: a team's own app needs none, and a public
    app needs Intercom's review.
  - Maps conversations to conversations, user comments to messages in, admin and bot comments to
    messages out, `note` parts to notes, assignment and state parts to nothing, tags to labels,
    admins to members, the admin assignee to the assignee, teams to folders and inboxes
    (unassigned to the default), contacts (users and leads) to contacts, attachments
    (Intercom's CDN, fetched without the token) to files. State: open open, snoozed pending,
    closed closed.
  - No search: Intercom's search endpoints are `POST`, which the transport refuses. Copy and sync
    page through `GET /conversations` (cursor `starting_after`, `per_page` 150, newest change
    first; confirm the order) and stop at the cursor's time; `GET /conversations/{id}` returns
    the parts. Confirm whether a very long conversation's parts are capped; if so the cut is
    recorded as a failed item, not skipped silently.
  - Budget: half of the limit, read from `X-RateLimit-Limit`, which counts 10-second windows
    (10,000 a minute per workspace's private apps, shared between them); 429 honoured with
    `X-RateLimit-Reset` when `Retry-After` is missing.
  - Tests against recorded answers in `helpdesktest/testdata/intercom`: paged conversations, parts
    of every kind, a team, a contact without e-mail, a CDN attachment, a 429, and a request log
    with only `GET`s and the token only on the region host.
  - Panel and guide `docs/migrate-intercom.md` in en, tr, de (delete the app afterwards); MCP
    `intercom` provider as for Zendesk.

### Y-155 · Crisp history import

- **Area** api · **Owner** yuva-api · **Priority** P1 · **Depends on** Y-151 · **Chapters** architecture › Moving to Yuva
- **Acceptance**
  - Connects with a website token the workspace owner generates (Settings › Workspace Settings ›
    Advanced configuration › API Token) and the website id; sent as Basic `identifier:key` with
    `X-Crisp-Tier: website` to `api.crisp.chat` only. Crisp documents no read-only scope for a
    website token: the GET-only transport is the guard. Plugin tokens (Marketplace, scope review)
    are not used. Unsure: which Crisp plans offer the website token.
  - Maps the website to one folder and inbox, conversations (sessions) to conversations, user
    messages to messages in, operator messages to messages out, `note` messages to notes, events
    and pickers to nothing, segments to labels, operators to members, the assigned operator to the
    assignee, the conversation's person (email, nickname, phone) to a contact, `file` and
    `animation` messages to files fetched from Crisp's storage hosts without the token. State:
    pending and unresolved open, resolved closed.
  - Lists conversations page by page ordered by last update (confirm `order_date_updated` and the
    date filters) and each conversation's messages backwards with `timestamp_before`; the cursor
    is the page and the newest update seen, so a resume repeats at most one page.
  - Budget: the website token's quota is 10,000 requests a day, so Yuva uses at most 5,000 a day
    through the daily class, about 2 requests per conversation; the dry run states the days it
    will take. Crisp sends no limit headers: a 429 or 420 waits a minute, then halves the pace.
  - Tests against recorded answers in `helpdesktest/testdata/crisp`: two pages of conversations,
    paged messages with a note and a file, an anonymous visitor, a 420, the daily budget running
    out and resuming the next day on the test clock, and a request log with only `GET`s.
  - Panel and guide `docs/migrate-crisp.md` in en, tr, de (roll the token afterwards); MCP `crisp`
    provider as for Zendesk.

### Y-156 · Chatwoot history import

- **Area** api · **Owner** yuva-api · **Priority** P1 · **Depends on** Y-151 · **Chapters** architecture › Moving to Yuva
- **Acceptance**
  - Connects with the access token of a Chatwoot administrator (Profile Settings › Access Token),
    the base URL (`https://app.chatwoot.com` or the team's own HTTPS host, dialled through the
    address checks) and the account, picked from `GET /api/v1/profile`; sent in the
    `api_access_token` header to that host only. Chatwoot has no read-only token: the GET-only
    transport is the guard. The connection test refuses a user who is not an administrator of the
    account, since agents see only their inboxes.
  - Maps inboxes to folders and inboxes, conversations to conversations, incoming messages to
    messages in, outgoing to messages out, private messages to notes, activity messages to
    nothing, conversation labels to labels, agents to members, the assignee to the assignee,
    contacts to contacts, attachments to files (the instance's storage URLs, fetched without the
    token). Status: open open, pending and snoozed pending, resolved closed.
  - Lists `GET /api/v1/accounts/{id}/conversations?status=all` by last activity and each
    conversation's messages backwards with `before=<message id>` (confirm page sizes); sync reads
    the listing until the cursor's time.
  - Budget: Chatwoot sends no limit headers and throttles per IP (self-hosted default from
    `RACK_ATTACK_LIMIT`; Chatwoot Cloud publishes no figure): a fixed 60 requests a minute,
    `Retry-After` honoured on 429 when present, else a minute.
  - Tests against recorded answers in `helpdesktest/testdata/chatwoot`: profile with two accounts,
    inboxes, paged conversations and messages with a private note and an attachment on another
    host, a 429 without `Retry-After`, a base URL resolving to a private address refused, and a
    request log with only `GET`s and the token only on the base host.
  - Panel and guide `docs/migrate-chatwoot.md` in en, tr, de (reset the access token afterwards);
    MCP `chatwoot` provider as for Zendesk.

## M32 — Integrations and push

### Y-157 · Slack

- **Area** api · **Owner** yuva-api, then yuva-panel · **Priority** P1 · **Depends on** – · **Chapters** architecture › Webhooks
- **Acceptance**
  - A workspace connects Slack through OAuth; an inbox posts new conversations, mentions and SLA
    breaches to a chosen channel.
  - Replying in the Slack thread posts a note (or, with a command, a reply) as the Slack user's linked
    member; unlinked users can do nothing.
  - No message body leaves for Slack unless the inbox setting allows content; otherwise a link only.

### Y-158 · Feedback to GitHub and Linear issues

- **Area** api · **Owner** yuva-api, then yuva-panel · **Priority** P1 · **Depends on** – · **Chapters** architecture › Feedback (M18)
- **Acceptance**
  - From a feedback or a roadmap idea a member creates or links a GitHub or Linear issue.
  - The issue's state sets the feedback's progress (open → planned, in progress, closed → done) the way
    roadmap stages do; a progress set by hand holds until the next change.
  - Tokens stored encrypted under the master key; the link shows in the thread and the API.

### Y-159 · Push sent by Yuva

- **Area** api · **Owner** yuva-api, then yuva-mobile · **Priority** P2 · **Depends on** – · **Chapters** architecture › In-app messaging
- **Acceptance**
  - An `app` channel can hold APNs (token key) and FCM (service account) credentials, encrypted; Yuva
    then sends the reply push itself when the contact is not connected, with `yuva_conversation_id`.
  - Hosts that send their own push keep working with the setting off.
  - Swift and Kotlin register the device token with `/client/v1`; invalid tokens are removed.
  - The "later, optional channel setting" line in `architecture.md` is replaced.

## M33 — Single sign-on for members

### Y-160 · Sign in with OpenID Connect

- **Area** api · **Owner** yuva-api, then yuva-panel · **Priority** P1 · **Depends on** Y-135 · **Chapters** architecture › Panel
- **Acceptance**
  - A workspace configures an OIDC provider (Google Workspace, Microsoft Entra, Okta, Keycloak) and
    claims its e-mail domains after a DNS TXT check.
  - People of a claimed domain sign in through it; optionally codes and passkeys are refused for them.
  - Just-in-time membership with a default role, off by default; sign-ins in the audit log.

### Y-161 · Sign in with SAML

- **Area** api · **Owner** yuva-api, then yuva-panel · **Priority** P2 · **Depends on** Y-160 · **Chapters** architecture › Panel
- **Acceptance**
  - SAML 2.0 SP-initiated sign-in on the same domain claim and membership rules as OIDC.
  - Signed assertions only; replay and clock-skew tests.

## M34 — Mail inboxes

### Y-163 · Inbox purpose `support` or `mail`

- **Area** api · **Owner** yuva-api · **Priority** P0 · **Depends on** – · **Chapters** architecture › Team mail
- **Acceptance**
  - `inboxes.purpose` is `support` (every existing inbox) or `mail`, set on create and changeable by
    owners and admins; contract first, in `/v1`, MCP and the CLI (`yuva inbox create --purpose`).
  - A `mail` inbox has only e-mail channels (`409 purpose_mismatch` for chat, app and api), asks no
    rating, is left out of SLA policies, automatic assignment, the greeting auto-reply, status, help,
    roadmap and What's new pickers, and out of support stats and reports.
  - Conversations keep their model: `closed` is Archived for a `mail` inbox, `pending` and `snoozed`
    unchanged; webhooks and events carry `purpose`.
  - Moving a conversation between a `support` and a `mail` inbox works as `move` does today.

### Y-164 · Mail section in the panel

- **Area** web · **Owner** yuva-panel · **Priority** P0 · **Depends on** Y-163 · **Chapters** architecture › Team mail
- **Acceptance**
  - A Mail entry in the rail (`/mail`, its own `G` shortcut, an unread badge) for members with a
    `mail` inbox; Conversations and its counts leave `mail` inboxes out, as Feedback is left out today.
  - Folders Inbox, Starred, Snoozed, Sent, Drafts, Archive and Trash per mail inbox and for all of
    them; rows read as mail (sender, subject, first line, attachment mark, time), the thread as stacked
    messages with headers, not chat bubbles.
  - Archive, Trash, Star, Snooze, Mark unread with undo; a starred and a trashed state per conversation
    (`starred_by` per member, `trashed_at` emptied after 30 days by the retention job).
  - en, tr, de and the other panel catalogs; screenshots (WebP) at desktop and phone width.

### Y-165 · Choosing support, mail or both

- **Area** web · **Owner** yuva-panel · **Priority** P1 · **Depends on** Y-164 · **Chapters** architecture › Panel › Home and setup
- **Acceptance**
  - A new workspace's owner is asked "Support, team mail or both"; the answer orders the `/setup`
    cards and the rail, and can be changed in workspace settings.
  - Mail-only workspaces land on `/mail`; Home leaves out support blocks they do not use.
  - `get_setup_state` and `GET /v1/home` report the mail card.

## M35 — Writing mail

### Y-166 · New mail to many recipients

- **Area** api · **Owner** yuva-api, then yuva-panel · **Priority** P0 · **Depends on** Y-145, Y-163 · **Chapters** architecture › E-mail
- **Acceptance**
  - A member writes a new mail from a `mail` inbox with To, Cc and Bcc (each up to 50 addresses), a
    subject and attachments; it opens a conversation whose participants are the recipients.
  - Any participant's reply joins it; "Reply" and "Reply all" choose the recipients; loop, spam and
    suppression rules hold.
  - Recipients are suggested from contacts the member may see; an unknown address becomes a contact.

### Y-167 · Sending as the member

- **Area** api · **Owner** yuva-api, then yuva-panel · **Priority** P0 · **Depends on** Y-166 · **Chapters** architecture › E-mail
- **Acceptance**
  - A `mail` channel sends either from the channel address with the member's name
    (`Ayşe Yılmaz (Acme) <sales@acme.com>`) or, for a personal mailbox, from the member's own address.
  - A member's own address is used only after it is verified by a code sent to it and its domain is a
    verified sending domain of the channel (SPF, DKIM, DMARC alignment by the deliverability check).
  - Replies to either come back into the conversation through the channel's address or the member's
    address routed to Yuva.

### Y-168 · Signatures

- **Area** api · **Owner** yuva-api, then yuva-panel · **Priority** P1 · **Depends on** – · **Chapters** architecture › E-mail
- **Acceptance**
  - Each member has a signature per inbox (Markdown, images from `/i/{token}`), and an inbox has a
    default with `{{member.name}}` and `{{member.title}}`.
  - The composer inserts it below the reply and above the quote; members can turn it off per message.
  - Support inboxes can use it too; chat and app replies never carry one.

### Y-169 · Scheduled send and member drafts

- **Area** api · **Owner** yuva-api, then yuva-panel · **Priority** P1 · **Depends on** Y-166 · **Chapters** architecture › E-mail
- **Acceptance**
  - A member's unsent reply or new mail is saved as their own draft as they type and opens on any
    device; drafts are visible only to their author (bot drafts keep their review flow).
  - "Send later" schedules a mail; it is listed under Scheduled, can be edited or cancelled until it
    goes, and goes once even with several replicas.
  - Undo send keeps its few seconds for both.

## M36 — Personal mailboxes

### Y-170 · Personal mailboxes and their privacy

- **Area** api · **Owner** yuva-api · **Priority** P0 · **Depends on** Y-163, Y-135 · **Chapters** architecture › Team mail, Concepts
- **Acceptance**
  - A `mail` inbox can be personal: `owner_member_id` set, created by the member for themself or by an
    owner or admin for a member.
  - Only the owner and the members they delegate to (Y-171) read its conversations,
    contacts' mail in it, attachments, drafts and events; owners and admins, API keys and other
    members get `404`, in `/v1`, realtime, MCP, search, exports and webhooks. Owners and admins see its
    name, address, size and usage only.
  - A personal mailbox sends no webhooks and no MCP access except through its owner's own OAuth grant.
  - Workspace retention, contact export and deletion, and workspace deletion still cover it.
  - Tests for each surface: an owner of the workspace gets `404` on a conversation of a member's
    personal mailbox.

### Y-171 · Delegation

- **Area** api · **Owner** yuva-api, then yuva-panel · **Priority** P1 · **Depends on** Y-170 · **Chapters** architecture › Team mail
- **Acceptance**
  - The owner of a personal mailbox grants another member read, or read and send; sending shows
    "on behalf of" in `Sender` and the timeline names who sent it.
  - Revoking takes effect at once, including open realtime connections.
  - Each grant and revoke is in the audit log.

### Y-172 · When a member leaves

- **Area** api · **Owner** yuva-api, then yuva-panel · **Priority** P1 · **Depends on** Y-170 · **Chapters** architecture › Team mail, Privacy and data
- **Acceptance**
  - Removing a member with a personal mailbox asks the owner or admin to choose: delete it, or hand it
    to a named member; a handed-over mailbox becomes that member's personal mailbox.
  - The choice is in the audit log; nobody reads the mailbox before the hand-over.
  - Deleting one's own account follows the same choice for its mailboxes, made by an owner afterwards.

## M37 — Organizing mail

### Y-173 · Mail filters

- **Area** api · **Owner** yuva-api, then yuva-panel · **Priority** P1 · **Depends on** Y-141, Y-164 · **Chapters** architecture › Team mail
- **Acceptance**
  - Filters on the automation rules engine with mail conditions (from, to, cc, subject, has
    attachment, list-id) and actions (label, star, archive, trash, snooze, forward to a teammate).
  - A personal mailbox's filters are its owner's; a shared mailbox's are set by owners and admins.
  - "Filter messages like this" from a message.

### Y-174 · Mail search operators

- **Area** api · **Owner** yuva-api, then yuva-panel · **Priority** P2 · **Depends on** Y-164 · **Chapters** architecture › Team mail
- **Acceptance**
  - The conversation search takes `from:`, `to:`, `cc:`, `subject:`, `has:attachment`, `is:starred`,
    `in:<folder>`, `before:` and `after:` on top of the existing full-text search.
  - Operators are parsed on the server, in `/v1` and MCP alike; unknown ones are searched as text.

## M38 — Team mail acceptance

### Y-175 · Team mail guides

- **Area** docs · **Owner** yuva-docs · **Priority** P1 · **Depends on** M34–M37 · **Chapters** –
- **Acceptance**
  - `docs/team-mail.md` with `tr` and `de`: shared and personal mailboxes, routing a domain's mail to
    Yuva, moving history with M14, sending as yourself, privacy, delegation, leaving members.
  - The site's feature pages say what team mail is and is not.

### Y-176 · Team mail on a running server

- **Area** verify · **Owner** yuva-verifier · **Priority** P0 · **Depends on** M34–M37 · **Chapters** roadmap M34–M38
- **Acceptance**
  - The Accept items of M34 to M37 on a server built from `main`, mail to simulator addresses and
    Mailpit only, in a throwaway workspace deleted afterwards.
  - Screenshots (WebP) of the Mail section, the composer and a delegated mailbox.
