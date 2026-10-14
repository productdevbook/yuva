# Architecture

Yuva is one place to answer every user of every product you run: e-mail, live chat on websites and
panels, and private in-app conversations in mobile apps. It is a single Go binary and a Postgres
database.

## Goals

- One inbox for many products. Each product is an inbox; members see the inboxes they belong to.
- One conversation model for every channel. Live chat and asynchronous messaging differ only in
  settings, not in code paths.
- E-mail done properly: threading, quote stripping, attachments, loop protection, bounces.
- Native embedding: a web component for sites and panels, Swift and Kotlin SDKs for apps, a Go
  helper for the host backend.
- Easy to run: one binary, one Postgres. No Redis, no separate worker process, no Node at runtime.
- Typed everywhere: OpenAPI 3.1 is the contract; clients are generated from it.
- Ready to be hosted for others: every row belongs to a workspace and usage is counted from day one.

Not goals: a CRM, a marketing automation suite (journeys, A/B tests, lead scoring), a public knowledge base, social-media channels
(WhatsApp, Instagram, …). They may come later; the model must not block them. Yuva does not host
documentation, but it collects feedback and questions on the documentation sites products already
have (see "Documentation pages"). Since M13 it sends broadcasts (newsletters, announcements) to
contacts who subscribed, and replies to them land in the inbox (see "Broadcasts and e-mail designs").
Since M15 it measures the workspace's own websites and apps without cookies, for the team and in
aggregate with support (see "Analytics").

## Concepts

| Concept | Meaning |
|---|---|
| Workspace | The tenant. A self-hosted install usually has one. Owns everything below. |
| Member | A person who answers. Role `owner`, `admin` or `agent`. Owners and admins see every inbox; agents see the inboxes they were granted, and only contacts with a conversation or external id in one of them or in no inbox at all; they set external ids only in their inboxes (ids in other inboxes are kept) and change e-mail addresses only of contacts that appear in no other inbox (`403 forbidden`). An inbox, conversation, contact or attachment the caller cannot see answers `404`. |
| Inbox | One product or brand. Branding, languages, business hours, mode (`live` or `async`), expected reply time, identity secret, webhooks. |
| Channel | How messages reach an inbox: `email`, `chat` (web widget), `app` (mobile SDKs), `api` (server-to-server, e.g. a feedback form). An inbox has any number of channels. |
| Contact | The person writing in. Known by an external user id from the host app (per inbox), one or more e-mail addresses (unique within the workspace), or an anonymous visitor id. Contacts belong to the workspace, not to an inbox. |
| Conversation | A thread between a contact and an inbox. Status `open`, `pending`, `snoozed`, `closed`; assignee; labels; priority; the channel it started on. |
| Message | One entry in a conversation: `message` (to or from the contact), `note` (members only) or `event` (assigned, closed, …). Has attachments and a per-channel delivery state. |

Feedback is a conversation that starts on the `app` or `api` channel with a `feedback` kind and
metadata (category, app version, build, OS, OS version, device model, locale, screen, installation
id). The categories are fixed for now (`bug`, `idea`, `praise`, `other`; a per-inbox list can come
later) and `/client/v1` returns them with the inbox so the SDKs never hard-code them. Apps send it
with `POST /client/v1/feedback` (JSON, or multipart with screenshots); host backends whose forms
are server-rendered send it with an API key to `POST /v1/feedback`, naming the user by their
external id, into any inbox of the key's workspace. The conversation starts on the inbox's `api`
channel, which the first such request creates (named `API`) when the inbox has none. It is
answered like any other conversation; the answer reaches the contact in the app, or by e-mail when
they allowed it (`allow_email`): members' replies the contact has not read go out through the same
delayed e-mail as chat replies. The panel
filters conversations by `kind` and feedback `category` and counts open feedback per category.

Members change up to 100 conversations in one request (`POST /v1/conversations/bulk`: status,
assignee, labels to add and remove); each is checked against the caller's inbox access and
changed in its own transaction with the same events and webhooks as a single change, and the
answer lists which changed and which were refused. A member who sees both inboxes moves a
conversation to another inbox of the workspace (`POST /v1/conversations/{id}/move`) with its
messages, notes, labels and attachments; the assignee stays only with access to the new inbox,
replies continue on the new inbox's channel of the same kind, and an e-mail conversation is
refused when the new inbox has no e-mail channel. Owners, admins and API keys merge one contact
into another (`POST /v1/contacts/{id}/merge`): the target keeps its id and gains the source's
addresses, external ids, attributes (its own value wins) and conversations; the source is deleted,
so identity tokens and mail for its keys find the target.

## Components

```
                ┌──────────────────────── yuva (one Go binary) ─────────────────────────┐
e-mail ─► edge/ Email Worker ─► /ingress/email ─┐                                       │
any MTA ─────────────────────► /ingress/email ─┤                                        │
widget, SDKs ─────────────────► /client/v1 + WS ┼─► store (Postgres) ◄─ jobs (River) ──┼─► SMTP
host backends ────────────────► /v1 (API keys) ─┤          ▲                            ├─► webhooks ─► host backend ─► APNs/FCM
panel (embedded SPA) ─────────► /v1 + WS ───────┘          └─ LISTEN/NOTIFY realtime    ├─► web push to members
                └───────────────────────────────────────────────────────────────────────┘
                                        object storage: S3-compatible or local disk
```

### Server (`api/`)

- Go, `net/http`, OpenAPI 3.1 contract in `openapi/openapi.yaml`, strict server generated with
  oapi-codegen. Postgres through pgx and sqlc; goose migrations.
- Background work on River (Postgres-backed queue): outbound e-mail, webhooks, notification
  fan-out, attachment cleanup, retention, and migrations on their own `migrations` queue (see
  "Moving to Yuva").
- Realtime: WebSocket (`coder/websocket`). Every change that members see live writes a row to
  `events` in the same transaction, last, under a per-workspace advisory lock, so event ids within
  a workspace follow commit order; the transaction's `NOTIFY` carries the id. One listener per
  process loads each notified event and an in-process hub fans it out to that workspace's
  connections, filtered by inbox access. Clients resume with the last event id: the server
  replays newer events (kept 7 days, cleaned by a River job) before live ones, or answers
  `resync_required` and the client reloads over HTTP. A slow connection is closed instead of
  holding up the hub; when the listener reconnects, every connection is closed so it resumes.
  Typing and presence notices are not stored: they travel as the payload of a second `NOTIFY`
  channel, have no id and are never replayed. Each open realtime connection is a row in
  `realtime_connections`, seen on every heartbeat; presence is read from the rows seen in the last
  75 seconds. Members see their teammates the same way: `/v1/members` reports each member's
  `availability` and whether they are `online`, and a `member.presence` notice reaches the
  workspace's member connections when a member connects, disconnects or changes availability.
  Contacts likewise: a `contact.presence` notice reaches the member connections that can see the
  contact whenever one of its `/client/v1/realtime` connections opens or closes. When a contact
  reads further in the widget or an app, the conversation's `last_read_by_contact_at` moves (kept
  on the conversation row next to `contact_reads`) and a stored `contact.read` event reaches the
  members who can see the conversation; it never goes to webhooks or to contacts.
  A connection that ends without closing (a process that died) only stops being seen, so a River
  job every minute announces the members and contacts whose last connection went stale in the
  previous two minutes.
- The panel and the widget bundles are embedded with `go:embed`; one binary serves everything.
- Configuration through environment variables. Secrets stored in the database (SMTP passwords,
  identity secrets, webhook secrets) are encrypted with AES-256-GCM under a master key,
  `YUVA_MASTER_KEY` (32 random bytes, base64, e.g. `openssl rand -base64 32`). The server does not
  start without it, and losing it makes the stored secrets unreadable.
- `slog` structured logs, Prometheus metrics, `/healthz` and `/readyz`.

### API surfaces

| Prefix | Used by | Auth |
|---|---|---|
| `/v1` | panel, host backends, scripts | member session cookie, or workspace API key |
| `/client/v1` | widget, mobile SDKs | channel public key + contact session, or a reader token on the discussion endpoints |
| `/client/v1/analytics/{key}` | the analytics script, `useyuva/analytics`, the mobile analytics modules | an analytics property's public key; browsers only from the property's domains |
| `/v1/analytics/shared/{token}` | read-only analytics shares | a share token (and its password session) |
| `/ingress/email` | edge Worker, MTAs | HMAC signature over the raw message |
| `/ingress/ses` | SES bounce and complaint notifications via SNS | SNS signature |
| `/s/{token}`, `/m/{token}`, `/c/{token}`, `/i/{token}` | people who receive list and broadcast mail | a token sealed under the master key; an image token |
| `/v1/realtime`, `/client/v1/realtime` | panel, widget, SDKs | as above |

A member session belongs to a person, not to a workspace. Workspace endpoints act on the workspace
named in the `Yuva-Workspace` header, or on the person's only workspace when the header is absent.
An API key acts on its own workspace only and cannot manage members or keys.
A `/v1` request other than `GET`/`HEAD` that carries the session cookie (and no bearer token) must
come from the panel: its `Origin` must be `YUVA_PUBLIC_URL` or a WebAuthn origin (without
`Origin`, `Sec-Fetch-Site: same-origin` is required; else `403 origin_not_allowed`), and a body
must be `application/json` or `multipart/form-data` (else `415 unsupported_media_type`), which a
cross-site form or a no-cors `fetch` cannot send. `SameSite=Lax` alone lets sibling subdomains
post with the cookie.
Owners and admins make keys in the panel or with `/v1/api-keys`. An operator with access to the
server's database makes them from the command line with the same code, for scripts and
provisioning before anyone has signed in:

```sh
yuva api-key create --workspace <id|name> --name <name>   # prints only the secret, once, on stdout
yuva api-key list --workspace <id|name>                   # id, prefix, name, dates; never a secret
yuva api-key revoke <id>
```

`--workspace` takes the workspace id or its exact name; a name that more than one workspace has
is refused with their ids. A key made this way has no creating member.

Inboxes and channels are managed by owners and admins; the same operator can create them from the
command line, which runs the HTTP handlers as an owner of the workspace (same validation, SMTP
password encryption and events):

```sh
yuva inbox create --workspace <id|name> --name <name> [--slug <slug>] [--locale <tag>] \
  [--timezone <zone>] [--mode live|async] [--expected-reply-minutes <n>]   # prints the inbox id
yuva inbox list --workspace <id|name>
yuva channel create-email --workspace <id|name> --inbox <id|slug> --name <name> --address <address> \
  [--display-name <name>] [--from-address <address>] [--smtp-host <host> [--smtp-port <n>] \
  [--tls starttls|tls|none] [--smtp-username <name>] [--smtp-password-file <path|->]]   # prints the channel id
yuva channel create-chat --workspace <id|name> --inbox <id|slug> --name <name> --origin <origin>... \
  [--allow-anonymous] [--ask-email-offline=false] [--greeting <text>]   # prints the channel id and public key
yuva channel list --workspace <id|name> --inbox <id|slug>                 # never a password
```

The slug is made from the name when omitted. The SMTP password is read from a file or, with `-`,
from stdin, never from the command line; the auto-reply stays off. The inbox's identity secret is
not printed; rotate it when an app needs one.

Unlike the commands above, the MCP bridge needs no database or server configuration; it runs on the
machine of whoever uses an assistant (see "MCP for local clients"):

```sh
yuva mcp stdio --url <server> --key <key>   # or YUVA_URL, YUVA_API_KEY; stdout carries only MCP
```

### Headless access

Everything the panel does is reachable without it: scripts, bots and other UIs use `/v1` with
scoped API keys, and every key-authored message names its bot.

**Scopes.** A key holds a list of scopes; every `/v1` operation a key may call needs exactly one:

| Scope | Operations |
|---|---|
| `conversations:read` | list, get and count conversations; list messages and pending drafts; attachments, message e-mail and raw source; list labels and canned replies |
| `conversations:write` | create, update (status, assignee, snooze, priority, labels), move and bulk-update conversations |
| `messages:write` | outgoing and incoming messages, drafts: create, edit, discard |
| `drafts:send` | send a draft (`POST /v1/messages/{id}/send`), together with `messages:write` |
| `notes:write` | notes |
| `contacts:read` | list, look up and get contacts, presence |
| `contacts:write` | create, update, delete and merge contacts |
| `inboxes:read` | workspace, members, inboxes, inbox members, channels |
| `inboxes:manage` | create, update and delete inboxes and channels, inbox access, secret and key rotation |
| `labels:write` | create, update and delete labels |
| `canned_replies:write` | create, update and delete canned replies |
| `webhooks:manage` | webhooks, their deliveries and attempts |
| `workspace:manage` | update the workspace, usage |
| `migrations:manage` | migrations: detect, create, change, connect, test, scan, start, pause, resume, cancel, finish, remove imported history, errors, probe, uploads |
| `feedback:write` | feedback |
| `lists:read` | lists, subscriptions, growth, consent export, imports |
| `lists:write` | lists, subscriptions, confirmations, imports |
| `broadcasts:read` | broadcasts, deliveries, reports, designs, templates, images, previews |
| `broadcasts:write` | drafts, designs, images, test sends |
| `broadcasts:send` | schedule, unschedule, pause, resume, cancel (only while the workspace allows keys and assistants to) |
| `analytics:read` | analytics properties, queries, realtime, goals, funnels, segments, annotations, health, exports (read) |
| `analytics:write` | server-side analytics events (API keys only) |
| `analytics:manage` | create, change and delete properties, keys, goals, funnels, shares, exports, data deletion |

A call without its scope gets `403 insufficient_scope` with the missing scope in `scope`. Keys made
before scopes existed hold every scope above. Scopes added later (`lists:*`, `broadcasts:*`) are never
added to existing keys, and `broadcasts:send` is never implied: a key made without naming its scopes holds
every scope but that one. `yuva api-key create` takes `--scope` (repeatable, default every scope but
`broadcasts:send`) and `--inbox` (repeatable).

A key may be limited to some inboxes. It then sees what an agent with access to exactly those
inboxes sees; it cannot hold `inboxes:manage`, `webhooks:manage`, `workspace:manage` or `migrations:manage` (refused when
the key is made). An inbox that is deleted drops out of the limit; a key whose every inbox is gone
sees nothing. A key may have an expiry; after it, the key gets `401 api_key_expired`. Scopes, inbox
limit and expiry are fixed when the key is made; a different set needs a new key. Name, bot name and
bot avatar can be changed (`PATCH /v1/api-keys/{id}`).

**Bot authors.** Every key is a bot: a display name (the key's name unless one is given) and an
optional avatar, an `https` image URL. Messages, notes and conversation events written with a key
have author type `bot` and point at the key; the bot's current name and avatar are shown in the
panel, the widget, the SDKs and webhook payloads. Outgoing e-mail from a bot uses the inbox's From
address with the bot's name as display name. Rows written by keys before this have author `system`
and stay so.

**Drafts.** An outgoing message can be a draft (`draft: true` on create). A draft is visible only on
`/v1` and to members: never on `/client/v1`, never e-mailed, never a notification, unread count,
preview, first response, usage or search hit. It can be edited (`PATCH /v1/messages/{id}`) and
discarded (`DELETE /v1/messages/{id}`); both answer `409 not_a_draft` for anything else.
`POST /v1/messages/{id}/send` delivers it: the message becomes a normal outgoing message with the
send time as its time, `sent_by` the member who sent it, and the draft author kept as author.
Events: `draft.created`, `draft.updated`, `draft.deleted` on realtime and as webhook types an
endpoint subscribes to; sending emits the usual `message.created`.
`GET /v1/drafts` lists the drafts waiting in the conversations the caller can see, newest first,
each with a conversation summary (id, inbox, subject, status, contact), filtered by `inbox_id` and
by `author`: `bot` (written with a key), `assistant` (a member through an OAuth client, `via` set)
or `member` (the panel); `total` counts the matches over all pages for a badge. Keys and tokens call
it with `conversations:read`. A client keeps it current from the draft events above and
`message.created` (a sent draft keeps its id), plus `conversation.updated`, `conversation.moved` and
`inbox_access.changed`.

The workspace setting `bots_may_send` (off for new workspaces; on after the upgrade for workspaces
that had an active key, so their integrations keep working) decides whether keys deliver at all.
Off: a key's outgoing message must be a draft, else `403 bot_sending_disabled`, and a key cannot
send drafts. On: a key with `messages:write` sends directly and one that also has `drafts:send`
sends drafts. Members always may. Owners and admins change it with a member session; a key, even
one with `workspace:manage`, cannot, so a bot never lets itself send.

**Idempotency.** Every authenticated `POST` on `/v1` and `/client/v1` accepts an `Idempotency-Key`
header (1 to 255 printable ASCII characters). The key is remembered for 24 hours per caller (API
key, member or contact) together with the method, path, a SHA-256 of the body and the response. The
same key with the same request returns the stored response with `Idempotent-Replayed: true`; with a
different request `409 idempotency_key_reused`; while the first is still running
`409 idempotency_key_in_use`. `5xx` answers are not stored, so the request can be retried.
Unauthenticated endpoints (sign-in, client session) and those that act on the signed-in person rather
than a workspace (`/v1/me/...`, sign-out) ignore the header, since every stored key belongs to a
workspace. The existing `client_id` on messages stays as it is.

**Event feed.** `GET /v1/events?after=<id>&limit=<n>` returns the events realtime and webhooks
carry, in id order, filtered exactly as the caller's realtime stream is (inbox access, key inbox
limit, scopes: a caller sees an event only if it holds `conversations:read`; subscription events also
need `lists:read`, broadcast events `broadcasts:read`, and contact events `contacts:read`). `after` is the last event id the caller handled; the answer carries
`next` (the last id returned) and `has_more`. Events are kept 7 days (realtime replay uses the same
rows). An `after` older than the oldest kept event answers `410 cursor_expired`: the caller
resyncs from the lists and continues from `GET /v1/events/latest`'s id. Keys and tokens open
`/v1/realtime` with `Authorization: Bearer`. Details: `after=0` starts at the oldest kept event and
never expires; a workspace with no kept events answers `410` to any other `after`, so a caller
idle through a silent week resyncs once. `limit` is 1 to 500 (100 by default). Events the caller
may not see are skipped, so `next` can be past the last returned event (the last one examined) and
a page can be short while `has_more` is true; one call examines at most 5000 events. The expiry
check runs after the page is read, so a cleanup in between cannot drop events silently.
`/latest` returns the workspace's newest id whatever the caller may see (`0` when none).

**API-only mode.** `YUVA_PANEL=off` stops serving the panel (and so the OAuth consent page:
`/oauth/authorize` then answers `temporarily_unavailable`; API keys still work). `YUVA_WIDGET=off`
stops serving the widget scripts. The API, realtime, ingress and `/mcp` stay. With the panel off,
its paths answer `404` and `/oauth/authorize` checks the client and `redirect_uri` as usual, then
redirects back with `error=temporarily_unavailable`. `/client/v1` stays with the widget off.

**JS packages.** The JS SDK is published to npm as the unscoped package `useyuva` (`yuva` is refused by npm as too
close to other names, and the `@yuva` scope belongs to someone else), MIT, versioned with the repository:
- `useyuva`: `createYuvaClient()`, the contact side (`/client/v1`) without DOM: sessions,
  identity tokens, conversations, messages, attachments, typing, read state, realtime with resume.
- `useyuva/chat`: the `<yuva-chat>` element, built on that client; the script-tag build stays.
- `useyuva/react`: `YuvaProvider`, `useConversations`, `useMessages` (React is an optional peer).
- `useyuva/nuxt`: a Nuxt module with `YuvaChat`, `YuvaPageFeedback` and `YuvaPageQuestions`
  components (`@nuxt/kit` and Vue are optional peers).
- `useyuva/api`: a typed `/v1` client for TypeScript backends, generated from the contract.
`sdk/go` gets a `/v1` client generated from the contract (`sdk/go/client`), next to `identity` and
`webhook`.

**MCP for local clients.** `yuva mcp stdio --url <server> --key <key>` (or `YUVA_URL`,
`YUVA_API_KEY`) bridges stdio to a server's `/mcp` over HTTP; it needs no database. Releases attach
an MCP Bundle (`yuva.mcpb`, the bridge for macOS, Linux and Windows, asking for the server URL and
key) and the repository keeps `server.json` for the MCP Registry, describing the remote endpoint
with the server URL as a variable and the stdio bridge as a package.
- The bridge relays JSON-RPC messages unchanged, so every protocol version and method the server
  speaks (including `subscriptions/listen`) passes through. `--url` is the server's public URL;
  `/mcp` is appended unless it is there. The key is sent as `Authorization: Bearer` and may be an
  API key or an OAuth access token. A call that fails gets a JSON-RPC error on stdout and a log
  line on stderr (JSON); an unreachable server fails each call but keeps the bridge running. A
  `401` (wrong, expired or revoked key) ends it with exit status 1; end of stdin with 0.
- The bundle follows MCPB manifest 0.3 (`deploy/mcpb/manifest.json`) and is packed by the release
  workflow with `@anthropic-ai/mcpb` 2.1.2. Server type `binary`: `server/yuva` is a POSIX shell
  launcher that picks `server/<os>-<arch>/yuva` (darwin arm64 and amd64, linux amd64 and arm64);
  Windows runs `server/yuva.exe` (amd64). `user_config` asks for `url` and `api_key` (sensitive)
  and passes them as `YUVA_URL` and `YUVA_API_KEY`. The binaries are the full `yuva` without the
  built panel, so the bundle stays near 60 MB.
- `server.json` uses the registry schema `2025-12-11`. The remote is `https://{yuva_host}/mcp` with
  `yuva_host` as a required variable (the registry refuses a remote URL another server already
  lists, and `https://{host}/mcp` is taken) (the schema allows a variable in the host, not yet a whole base
  URL). The package is `registryType: mcpb` pointing at the release's `yuva.mcpb`. The file in the
  repository has no `fileSha256`; the release workflow fills the version, the asset URL and the
  hash and attaches that `server.json` to the release; for a stable tag the `registry` job publishes it
  with GitHub OIDC (no token), as `io.github.productdevbook/yuva`. Its version and the manifest's
  move with the repository version.

### OAuth and MCP

**Authorization server.** Yuva is its own OAuth 2.1 authorization server, for MCP clients and for
the member apps (M10). Authorization code with PKCE (`S256` only), no implicit or password grant.
Client metadata must list `authorization_code` among its `grant_types`; other types it lists are
ignored (claude.ai lists `jwt-bearer`), and registration answers with the two Yuva offers.
- Metadata: `/.well-known/oauth-authorization-server` (RFC 8414) and
  `/.well-known/oauth-protected-resource/mcp` plus `/.well-known/oauth-protected-resource`
  (RFC 9728). A `401` on `/mcp` carries `WWW-Authenticate: Bearer resource_metadata="…"`.
- Clients: dynamic registration at `POST /oauth/register` (RFC 7591, public clients, no secret),
  and Client ID Metadata Documents (a `client_id` that is an `https` URL, fetched with the same
  private-address rules as webhooks and cached for 24 hours; its `client_id` must equal the URL and
  it must be a public client). Redirect URIs must be `https`, or `http` on loopback, or a
  private-use scheme for native apps; matched exactly, except that an `http` URI on a loopback host
  (`127.0.0.1`, `[::1]`, and `localhost`, which Claude Code registers) matches on any port, at
  authorize as RFC 8252 §7.3 and OAuth 2.1 ask (the token request must repeat the URI the
  authorization request used). `redirect_uri` may be left out only when the client has one.
  Registration answers `token_endpoint_auth_method: none` whatever was asked, allows 20
  registrations per address and hour, and clients that never got a grant are deleted after 30
  days. Client ids are `yuva_client_<random>`.
- `GET /oauth/authorize` checks the request and sends the browser to the panel's consent page
  (`/oauth/consent?request=<id>`). The panel signs the person in if needed (code or passkey), shows
  the client's name and redirect host, lets them pick one workspace, and shows the scopes. Approving
  returns the redirect with a code (single use, 60 seconds). The panel reads and answers the request
  with `GET /v1/oauth/requests/{id}`, `POST …/approve` (workspace and scopes) and `POST …/deny`, all
  with a member session; a pending request lives 10 minutes and is used once. Every redirect back
  carries `state` and `iss` (RFC 9207).
- `POST /oauth/token`: `authorization_code` and `refresh_token`. Access tokens are opaque, 1 hour;
  refresh tokens 30 days, rotated on every use, and reusing a rotated one revokes the grant, as does
  using a code twice. Both are stored as hashes (`yuva_at_…`, `yuva_rt_…`) and looked up by hash
  alone, like API keys. `POST /oauth/revoke` (RFC 7009): a refresh token revokes the grant, an
  access token only itself.
- Resource indicators (RFC 8707) are required. `<YUVA_PUBLIC_URL>/mcp` gives a token for `/mcp`
  only; `<YUVA_PUBLIC_URL>` gives one for `/v1`, `/v1/realtime` and `/mcp` (the member apps). A
  trailing `/` is ignored. A `/mcp`-only token on `/v1` gets `401 token_wrong_resource`; an expired
  one `401 token_expired`.
- Scopes are the API key scopes. A token acts as its member: what the member's role and inbox access
  allow, narrowed by the granted scopes, never more. A member who leaves or is removed loses every
  grant. Management scopes a member's role cannot use are not offered on the consent page: agents
  are not offered `inboxes:manage`, `labels:write`, `canned_replies:write`, `webhooks:manage`,
  `workspace:manage` and `migrations:manage`, and nobody `feedback:write` (API keys only). A token may call exactly the
  operations an API key may; the rest (read state, typing, notification settings, `/v1/me`,
  members, keys, Connected apps, the consent endpoints) need a member session, and a token never
  changes `bots_may_send`.
- A grant is one member, one workspace and one client; approving the same client again in that
  workspace replaces the grant's scopes and resource, and its live tokens follow. The 600 requests a
  minute are counted per grant (its tokens rotate hourly), in the process like the other rate
  limits, before scope checks. Settings → Connected apps lists a member's
  own grants (client, scopes, created, last used, request count this month) and revokes them;
  owners and admins see and revoke every grant in the workspace.
- Registered clients and Client ID Metadata Documents belong to no workspace (a client is used by
  many); they are the exception to the `workspace_id` rule, as are pending authorization requests,
  which exist before the person picks a workspace. Grants, codes and tokens carry the workspace.

**Writes through a client.** Everything a token writes records the client: messages, notes,
drafts and conversation events get `via` (the client name at the time), and the timeline shows
"Ayşe via Claude Code". `via` is on the message's `author`, and a draft sent with a token also gets
it on `sent_by`. Delivery from a `/mcp`-only token follows `bots_may_send` like an API key:
off, an outgoing message must be a draft. A `<YUVA_PUBLIC_URL>` token (member apps) sends as the
member.

**MCP endpoint.** `/mcp` in the same binary, Streamable HTTP, MCP specification 2026-07-28, built
on `github.com/modelcontextprotocol/go-sdk` (MIT/Apache-2.0). Bearer is an OAuth token or a scoped
API key. `YUVA_MCP=off` turns it off (the OAuth endpoints stay for the apps). The transport is
stateless, as 2026-07-28 requires: there are no MCP sessions, every request is authenticated and
metered on its own and gets a server built for its principal. Older protocol versions work request
by request; their `resources/subscribe` is refused, so subscriptions use `subscriptions/listen`.
A listen is fed from the realtime hub (any instance the stream is on), filtered like `/v1/realtime`,
re-checks its token every 30 seconds and ends when the token stops working. The endpoint sends
`Access-Control-Allow-Origin: *` and skips the SDK's localhost Host check: it takes no cookies.
- Tool results are Yuva's own shapes, not the `/v1` schemas: ids, enums, counts and times at the
  top level, customer text under `customer_content`; names written by members (inboxes, labels,
  canned replies, bots) stay outside. Schemas have one type per field, without `null` unions, so
  clients with a single-type dialect (Gemini) accept them; absent fields are left out.
- Mappings that are not one operation: `get_conversation` is the conversation plus its latest 50
  messages oldest first (`next_cursor` pages back) and, with `contacts:read`, the contact's name and
  address; `list_feedback` is `ListConversations` with `kind=feedback`; `add_labels` and
  `remove_labels` are `BulkUpdateConversations` with one id. `merge_contacts` is listed only to
  owners, admins and keys without an inbox limit.
- Setup tools: `list_channels` (`ListChannels`; kind, name, public key, allowed origins,
  anonymous, discussions, e-mail addresses, never secrets), `create_inbox` (`CreateInbox`; the slug
  is made from the name as the panel does, with `-2`, `-3` … when taken; the identity secret is not
  returned), `create_chat_channel` (`CreateChannel` with kind `chat`; anonymous on and discussions
  `off` unless given), `update_chat_channel` (`GetChannel`, then `UpdateChannel` with the current
  chat settings and the added and removed origins, anonymous, discussions and greeting changed) and
  `get_install_snippet` (`GetChannel`; the code for `html`, `react`, `nextjs`, `nuxt`, `vue`,
  `astro`, `shadcn`, the `docs-` variants and the `subscribe-` variants for `html`, `react`, `nuxt` and
  `shadcn` with `list_ids` (public lists of the channel's inbox), filled with `YUVA_PUBLIC_URL` and
  the public key, an `npm install "useyuva@>=<server version>"` line where needed, where it goes and the guide on
  GitHub). The templates live in the server (`mcp_setup.go`) with the panel's wording. The three
  writes are listed to owners, admins and keys with `inboxes:manage`. Calling a tool that is not
  listed answers `insufficient_scope` naming the scope to reconnect with, or the role or
  `bots_may_send` reason.
- Broadcast tools: `list_mailing_lists`, `list_broadcasts`, `get_broadcast` (with the review and the
  audience for a draft) and `get_broadcast_report` are listed by scope (`lists:read`,
  `broadcasts:read`); `list_email_templates` and `preview_design` (the HTML as an embedded resource)
  with `broadcasts:read`, and `create_design`, `update_design` (with `base_version`),
  `create_broadcast_draft` and `send_test_broadcast` with `broadcasts:write`, are listed only to
  owners, admins and keys. A test through an OAuth token goes to the caller only; keys name members. `schedule_broadcast` is listed only to principals
  with `broadcasts:send` while the workspace allows `broadcasts_by_keys`, and needs `send_at` and
  `confirm_recipients` equal to the audience count it was shown; the approval gate applies. Reply
  contacts in reports are customer content. Resource `yuva://broadcast/{id}`; prompts
  `draft_newsletter` (never schedules) and `broadcast_report`.
- Migration tools (`detect_mail_source`, `list_migrations`, `get_migration`, `create_migration`,
  `update_migration`, `test_migration_connection`, `scan_migration`, `preview_migration`,
  `start_migration`, `pause_migration`, `resume_migration`, `cancel_migration`, `finish_migration`,
  `retry_migration_items`, `list_migration_errors`, `send_migration_probe`,
  `remove_migration_history`) are listed to owners, admins and keys with `migrations:manage`. None
  takes a credential: `create_migration` returns the panel link where the person connects.
  `remove_migration_history` carries `destructiveHint` and needs `confirm_count`. Resource
  `yuva://migration/{id}`; prompt `move_to_yuva` (address, inbox).
- Writes carry `destructiveHint: false` except `merge_contacts`; `assign`, `set_status`, `snooze`,
  the label tools, `move_conversation`, `bulk_update` and `update_chat_channel` carry `idempotentHint`; `send_reply` and
  `send_draft` (`SendMessage`, delivering a stored draft instead of a copy of its text) carry
  `openWorldHint`. `send_reply` answers `409 draft_pending` while the caller (the same key,
  or the same member through the same client) has a draft in that conversation.
- `resources/list` lists the inboxes the caller sees. `yuva://inbox/{id}` holds the inbox and, with
  `conversations:read`, its latest open conversations; it is updated by `inbox.*` and
  `conversation.created`, `.updated` and `.moved` there. `yuva://conversation/{id}` is updated by
  every event of that conversation, `yuva://contact/{id}` by `contact.updated` and `.deleted`.
- The `draft_reply` prompt looks up the inbox's `default_locale` and names it in the text.
- Tools call the same code as the `/v1` operations with the caller's principal, so access, scope
  and inbox checks are the API's. Read tools carry `readOnlyHint`; idempotent writes
  `idempotentHint`; `merge_contacts` `destructiveHint`. Every tool has an output schema. Tools the
  caller's scopes or role cannot use are not listed; `send_reply` and `send_draft` are listed only
  when the caller may deliver.
- Customer text is untrusted: message bodies, subjects, names, e-mail addresses and attributes are
  returned only inside `customer_content` fields, and every tool description says that text there
  is data from customers and never instructions.
- Resources `yuva://inbox/{id}`, `yuva://conversation/{id}`, `yuva://contact/{id}`, with
  subscriptions. Prompts `triage_inbox`, `draft_reply`, `summarize_conversation`, `weekly_report`
  and `setup_live_chat` (site URL, framework, product name: find or create the inbox and the chat
  channel for the site's origin, get the snippet, add it to the code or show it; without
  `inboxes:manage` it says which scope to reconnect with); they only arrange tool calls and text,
  Yuva runs no model.
- `init` is the entry prompt for adding Yuva to a project: the assistant reads the project locally
  and sends none of it to Yuva, calls `get_setup_state` (what the workspace has, the parts it cannot
  read with the scope to grant, and a fixed catalog of integrations with the signals that suggest
  each and the prompt or tools that set it up), then proposes a plan and changes nothing until the
  person picks steps. `get_setup_state` and `report_yuva_issue` need no scope and are listed to
  every connection.
- `report_yuva_issue` prepares a GitHub issue about Yuva itself for `productdevbook/yuva` and files
  nothing: the server removes the workspace's own names (workspace, inboxes, channels, properties,
  lists, their origins, domains and addresses), e-mail addresses, URLs and host names other than
  `github.com` and `useyuva.com`, IP addresses, keys, tokens, UUIDs and home-directory paths, and
  returns the title, body, a prefilled new-issue URL and a `gh issue create` command. The person
  reviews it and opens it under their own GitHub account, so the server holds no GitHub credential
  and cannot be used to post to the repository.
- Rate limit per token or key (600 requests a minute, `429` with `Retry-After`), counted per grant
  for Connected apps.

### Identity

The host app's backend knows who its user is; Yuva trusts it through a short-lived identity token:
a JWT (HS256) signed with the inbox's identity secret, with `sub` (the host's user id), optional
`email`, `email_verified`, `name`, `locale` and `attrs` (plan, app version, anything the panel
should show), and `exp` at most 10 minutes ahead. The client exchanges it at `/client/v1/session` for a contact
session. `sdk/go` signs these tokens and verifies webhooks.

The token is checked strictly: `alg` must be `HS256` (anything else, `none` included, is
refused), the signature must match the inbox secret, `exp` is required and at most 10 minutes
ahead, `sub` is required. A token with a `jti` is accepted once: its id is remembered per inbox
until the token expires (tokens without one can be replayed until `exp`). The contact is found by `sub` among the inbox's external ids, then by the
token's `email` when the token says `email_verified: true` and the contact with that address has no
external id in this inbox, or created; a contact already bound to another `sub` of the inbox is
never taken over. `name`, `locale` and `attrs` are saved on the contact; when it was found by
e-mail and has external ids in other inboxes, they only fill what is missing. A verified `email` becomes
one of the contact's addresses. Unverified e-mails (`email_verified` absent or false) are stored
like a typed address but never used to link a contact. Contact
sessions are opaque tokens, stored as hashes, valid for 7 days after their last use.

Channels can allow anonymous visitors (web chat on a public site). An anonymous visitor is a
contact plus a random visitor id that the browser keeps; the id resumes that visitor, and only in
the inbox that issued it. When the same browser later sends an identity token with its visitor id,
the visitor's conversations move to the identified contact (or, for a contact Yuva has not seen
yet, the visitor becomes it) and the visitor id ends. E-mail addresses are matched to contacts only
when they arrive by e-mail or inside a valid identity token: an address a visitor types in the
widget is kept apart (`typed_email`) and used only to e-mail them replies; it becomes one of their
addresses only when someone opens the confirmation link Yuva mails to it and confirms there. The
link (`/email/confirm?token=…`, valid 24 hours, at most 3 per contact and hour, not sent for an
address a contact already has) opens a page whose button posts the token, so a mail scanner that
fetches links confirms nothing; the mail names the inbox and never repeats visitor text. Answering
a reply e-mail does not confirm the address. Such an answer (from the typed address, naming the
visitor's thread, while no contact has the address and DMARC does not fail) joins the visitor's
conversation as their message, marked `unverified_sender` (its `from` is not one of the contact's
addresses, computed when read), and links nothing; once the address is confirmed it is the
contact's and the mark goes away.

### E-mail

Inbound:
1. A Cloudflare Email Worker (`edge/`) receives mail for the support addresses and POSTs the raw
   message with the envelope recipient and sender to `/ingress/email`, signed with HMAC-SHA256
   under `YUVA_INGRESS_SECRET` (one per install; the request format is in `edge/README.md`). The
   `v2` signature covers the timestamp, both envelope addresses and the body; the older `v1`, which
   leaves the envelope sender out, is accepted only with `YUVA_INGRESS_ACCEPT_V1`, and then the
   envelope sender is not trusted (an empty one marks nothing as a delivery report or automatic).
   The signature headers and the timestamp (within 5 minutes) are checked before the body is read,
   and the MAC while it is read; at most `YUVA_INGRESS_MAX_CONCURRENT` (8) messages are processed
   at once per process, more are answered 503. A Message-ID already stored for the channel is
   accepted again without a second copy. When the server cannot be reached (5xx, network error, no answer within 20
   seconds) the Worker forwards the message to an optional fallback address (`FALLBACK_FORWARD`, a
   verified Email Routing destination) instead of failing the delivery; refusals (4xx) still
   bounce. Any other MTA can do the same; the endpoint is not tied to
   Cloudflare. An MTA that pipes into a command uses `yuva ingest-email --to <address>`, which
   exits with sysexits codes (67 unknown recipient, 77 refused, 75 try later).
2. The recipient selects the channel: an e-mail address belongs to one channel of the whole server
   (case-insensitive; `local+tag@` falls back to `local@`), because the request names nothing
   else. A channel with the address `*@domain` is the domain's catch-all (one per domain, unique on
   the server like any address): it receives mail to every address of the domain that no channel
   has exactly. Its conversations remember the address the contact wrote to, and replies go out
   from that address (`From` and `Reply-To`), so its SMTP account must be allowed to send from the
   whole domain; its `from_address` is used only for a conversation without such an address
   (a chat that continues by e-mail), and without either the reply is refused (`email_no_sender`).
   An inbox's chat continuity prefers an exact-address channel over a catch-all. Threading uses `In-Reply-To` and `References` against stored Message-IDs of the inbox
   (at most the first five `In-Reply-To` ids, and the first and the last 49 `References` ids; ids longer
   than 998 bytes dropped, read from at most 64 KiB at each end of the header);
   our outbound Message-IDs are `<token.random@sending domain>` where the token is an opaque random
   value stored on the conversation, so a reply finds its conversation even when the client drops
   `References` or a relay rewrites the domain. A relay that replaces our Message-ID (Amazon SES writes
   `<id@region.amazonses.com>`, `email.amazonses.com` in us-east-1, from the id in its reply to `DATA`)
   has that id stored next to ours as `relay_message_id`, and threading, broadcast replies and delivery
   reports match either. Otherwise a new conversation starts, with the
   subject of the mail. A closed, pending or snoozed conversation that gets a reply is reopened.
   The sender is matched to a contact by address or becomes a new contact; mail from a blocked
   contact is refused, mail from the channel's own address is dropped. A thread joins only a
   conversation of the sender's own contact: anyone who holds one of our Message-IDs (a CC'd
   colleague, a forward) could otherwise write into a customer's conversation. Mail that names the
   thread of another contact's conversation opens a new conversation for its sender, counted
   against the hourly limit, with `related_conversation_id` pointing to the conversation it named;
   the panel shows that link, and nothing is added to the other conversation. `From` can be
   forged, so a reply whose trusted DMARC result fails never joins a conversation that is not
   flagged spam either: it opens a new spam conversation with `related_conversation_id`. The `Cc` of inbound
   mail is recorded and shown; nobody is copied automatically on replies.
   Every e-mail channel also has a forwarding address, `<token>@<YUVA_FORWARDING_DOMAIN>` (16 random
   characters, rotatable), for mailboxes that forward to Yuva instead of moving their MX; ingress
   resolves a recipient on that domain by token before the address lookup. Without the setting there
   is none.
3. MIME parsing with enmime; visible text with quotes and signatures stripped (our own Go
   implementation in `api/internal/email/reply`, tested against a fixture corpus, which scans only
   the first 64 KiB of the text: when nothing is hidden there the whole text is shown); the quoted
   containers that clients mark in HTML are removed too; HTML sanitized with bluemonday for
   display. The full text and full sanitized HTML stay available, and the original message is kept
   in object storage for members to download. Attachments and inline images follow the server's
   attachment size and type rules; the rest is skipped (it stays in the original). Inline parts
   keep their `Content-ID` (`content_id`, `inline` on the attachment) and the sanitized HTML keeps
   `cid:` image sources, so the panel can show them from the attachment. Remote images stay in the
   stored HTML; `has_remote_images` on the message's e-mail data tells the panel to block them
   until the member asks.
4. Loops: messages with `Auto-Submitted` other than `no`, `Precedence: bulk|junk|list|auto_reply`,
   `X-Autoreply`, `X-Autorespond`, `X-Auto-Response-Suppress` (other than `None`), `List-Id`, a
   null sender, a delivery report, or our own Message-ID domain are stored but never trigger an
   automatic message. Mail from an address the server sends from (any channel's address or
   `from_address`, and `YUVA_SMTP_FROM`) is dropped, so notifications and replies that reach a
   catch-all never loop. Volume never loses mail: a sender opens at most 20 new conversations per
   channel and hour, and further mail in that hour is added to the sender's latest conversation on
   the channel (reopened if needed, no greeting) instead of opening a new one. We chose this over a
   separate "bulk" state because it needs no new view and keeps every message in front of a member.
   Only a sender over a hard cap of inbound mails per hour in the workspace
   (`YUVA_EMAIL_SENDER_HOURLY_CAP`, default 500) is refused (429 `rate_limited`). Cloudflare Email
   Workers can only refuse a message permanently (`setReject`; a temporary failure is not
   documented), so that refusal is permanent and its text says the message was not accepted.
5. Spam: the receiving server's verdict (the topmost `Authentication-Results`) is stored and
   shown, and its DMARC result is used only when the header's authserv-id is
   `YUVA_INGRESS_AUTHSERV_ID` (`mx.cloudflare.net` behind Cloudflare Email Routing); any other
   header could have come with the message, so without a match DMARC is `unknown`. A new
   conversation whose first mail fails DMARC is flagged `spam`, which keeps it out of
   lists and counts (they have a spam view) and away from automatic replies; contacts can be
   blocked.

Outbound: SMTP per channel (works with SES, Postmark, any relay); the password is stored
encrypted under the master key and never returned, and it is only sent over TLS. The SMTP host is
resolved once per send and refused when it resolves to a loopback, private or other non-public
address (unless `YUVA_SMTP_ALLOW_PRIVATE`) or to a link-local or cloud metadata address (always),
like webhooks, so a channel cannot probe the server's network; a literal address or `localhost`
that would be refused is already refused when the channel is saved. A member's reply
in a conversation that started on an e-mail channel goes out through a River job: `From` is the
channel address (or its per-channel sending address) with its display name, `Reply-To` the channel
address, `To` the address the conversation's contact last wrote from among the contact's own
addresses (or the contact's first address) and never another sender in the thread, no `Cc`,
`Subject` `Re: …`, and `In-Reply-To` and
`References` keep the thread. Notes are never sent. The message carries its delivery state
(`queued`, `sent`, `failed` with the error), reported live as `message.updated`; temporary SMTP
errors are retried, 5xx replies fail at once. An e-mail channel can greet new conversations once
per contact within a set interval; the greeting is a `system` message with
`Auto-Submitted: auto-replied`.

Bounces: an inbound delivery report (`multipart/report; report-type=delivery-status`) to a channel
address counts only when its envelope sender is empty (the null reverse-path RFC 5321 §4.5.5
requires of delivery reports), it names a message we sent from that workspace (its Message-ID in the
returned headers, `In-Reply-To` or `References`) and a `Final-Recipient` that was a recipient of
that message; then it fails the message and marks the permanently failed recipients
undeliverable. Anything else is stored as an ordinary automatic mail from its sender and changes
nothing; a blocked sender is refused before the report is looked at. Amazon SES bounces and
complaints arrive at `/ingress/ses` through SNS: the SNS signature is checked against the AWS
certificate, only topics listed in `YUVA_SES_TOPIC_ARNS` are accepted, a subscription is confirmed
only on an `sns.<region>.amazonaws.com` URL, a notification whose `Timestamp` is more than an hour
old (or ahead) is refused as a replay, and a notification counts only for a message we sent
and for its recipients. Suppressions have a scope: a hard bounce or a complaint about a reply
stops all mail to the address; a complaint about a broadcast stops broadcasts only (see Broadcasts).
Undeliverable addresses are shown on the
contact, replies to them are refused until a member clears them.

Mail the server writes itself (sign-in code and pause notice, invite, member notification, address
confirmation, discussion digest, rating request) is `multipart/alternative`: the text part from
`api/internal/mail/templates/<locale>/<name>.txt`, unchanged, and an HTML part from `<name>.html`
in the same locale (en, tr, de; English when the locale has no text template) inside one shared
layout (`templates/layout.html`, Go `html/template`, so every value is escaped). The layout is
table-based with inline CSS, at most 560px wide, a system font stack, no remote images, fonts or
tracking; a `prefers-color-scheme` media query is the only `<style>` and only darkens it. Mail to
members and people signing in carries the "Yuva" wordmark in the panel's brand colour; mail sent to
a contact on behalf of an inbox (address confirmation, discussion digest, rating request) carries
the inbox's name and `branding.color` (else `#2563eb`) and never names Yuva, with a readable
button and text colour derived from it. `List-Unsubscribe` links also appear as footer links. Both
parts are quoted-printable UTF-8 through the server mailer (`YUVA_SMTP_*`); the rating request goes
out through the inbox's e-mail channel like a reply, its HTML stored as the message e-mail's
`full_html` while the message itself stays text. List confirmation and welcome mails (`list_confirm`,
`list_welcome`) are rendered the same way under the inbox's brand and go out through the list's e-mail
channel, never through the server mailer. The continuity mail renders only its subject (its
body is the members' replies) and push payloads stay text.

### Live chat and async messaging (web)

`sdk/js` ships a framework-free web component, `<yuva-chat>`, in Shadow DOM, translated with Lingui,
loaded by one script tag. Two layouts: a floating launcher for websites, and `embedded` for an
inline thread inside a product's own panel.

- The server hosts the widget scripts itself at `/yuva.js`, `/yuva-chat.js` and `/yuva-docs.js` (cached
  five minutes, ETag). What a script loads on first use (the discussion thread, composer and preview of
  `/yuva-docs.js`) is a content-hashed chunk at `/widget/<name>-<hash>.js`, cached as immutable; a
  request for a previous build's hash of a known chunk is redirected to the current one.
- Public channel settings come from `GET /client/v1/channels/{key}` without creating a contact.

- A `chat` channel has a public key (not a secret; it is embedded in pages and can be rotated), the
  exact origins whose pages may use it, whether anonymous visitors may chat, whether to ask for an
  e-mail address when nobody is available, a greeting and launcher overrides. `/client/v1` answers
  browsers only from origins some chat channel allows (CORS) and then checks the session's own
  channel: a request for a chat channel must carry one of its origins, and one without an `Origin`
  header is refused. The server's own origin (`YUVA_PUBLIC_URL`) counts as allowed for every chat
  channel, so the panel can show a live widget (the setup page's preview) without changing the
  channel; a same-origin request without `Origin` counts as that origin only when it carries
  `Sec-Fetch-Site: same-origin`. Session starts and contact writes are rate limited per IP address (per /64
  for IPv6) and per channel, counted in each process; finished windows are swept every minute and
  at most 100,000 are open at once (new keys beyond that are refused until a sweep). An IP address
  starts at most `YUVA_ANONYMOUS_CONTACTS_PER_HOUR` (20) new anonymous visitors per channel and
  hour (`429 anonymous_limit`); resuming a visitor does not count.
- Contacts see their own conversations of the inbox, messages only (no notes, no internal events),
  conversation status, and of members only the display name and initials.
- `live` inboxes show who is available: members with access to the inbox, with an open
  `/v1/realtime` connection, not set to `away`, while the inbox is within business hours. They also
  show members typing and how far members have read.
- `async` inboxes show the expected reply time instead and no presence.
- When a member changes the inbox or the session's chat or app channel, open client connections of
  that inbox get an `inbox.updated` frame with the same public settings as
  `GET /client/v1/channels/{key}` (sent only when they differ from what the connection last saw).
  The frame has no id and is not replayed (a channel change travels as a signal like typing), so
  clients fetch the settings again after a reconnect.
- If the contact has left when a reply arrives and an e-mail address is known, the reply is e-mailed
  after a delay; their e-mail answer continues the same conversation. A River job checks the
  conversation `YUVA_CHAT_EMAIL_DELAY` (5 minutes by default) after a member's reply and after the
  contact's last connection closes: the members' replies the contact has not read, once the oldest
  is that old and the contact has been gone that long, go out as one e-mail through the inbox's
  e-mail channel, threaded so an answer finds the conversation, at most one e-mail per delay. Notes
  are never sent. The subject is the inbox name and a fixed "new reply" phrase in the contact's
  language (else the inbox's), never visitor text: the e-mail may go to an address nobody has
  confirmed. The contact's last seen time is saved before their connection is removed, so a check
  never finds them both disconnected and not recently seen.

### In-app messaging (mobile)

- `sdk/swift` (Swift package `YuvaKit`): a headless client plus SwiftUI screens (conversation list,
  thread, composer, attachments, feedback form).
- `sdk/kotlin`: the same for Android, Compose UI.
- The apps talk to `/client/v1` with the public key of an `app` channel: like a chat key it starts
  contact sessions and can be rotated, anonymous use is off unless the channel allows it, and the
  channel lists the platforms it ships on. Native apps send no `Origin`, so app keys are not tied to
  origins and skip the origin check (a browser on an origin no chat channel allows is still
  refused by CORS). App channels share the chat channels' settings table, with no origins.
- The host app gets an identity token from its own backend and opens Yuva's screens or drives its
  own UI from the client.
- Push: Yuva sends `message.created` webhooks to the host backend, which already holds the device
  tokens and APNs/FCM keys. The payload's `contact.online` says whether the contact had a live
  `/client/v1/realtime` connection (seen in the last 75 seconds) when the webhook was prepared;
  when it is false the app is closed or in the background and the host sends its push. The host
  puts the conversation's id (`data.message.conversation_id`) in the push payload as
  `yuva_conversation_id` (APNs: a top-level key next to `aps`; FCM: a `data` entry);
  `Yuva.handleNotification` in both SDKs reads it and returns the conversation to open.
  `GET /v1/contacts/{id}/presence` answers the same question on demand. Yuva sending push itself
  is a later, optional channel setting.

### Webhooks

Endpoints belong to the workspace (all events) or to one inbox (that inbox's conversations, and
contact events for contacts with an external id or a conversation there). Each has a URL, the event
types it subscribed to, whether `message.created` includes members' notes (never by default;
internal events are never sent), an enabled flag and a signing secret: 32 random bytes shown once
as `whsec_<base64>`, stored encrypted under the master key, rotatable. After a rotation the old
secret keeps signing next to the new one for 24 hours.

Events: `conversation.created`, `conversation.updated`, `conversation.rated`, `message.created`,
`feedback.created`, `contact.updated`, `contact.deleted`, `subscription.created`, `subscription.updated`,
`broadcast.updated`. Payloads follow Standard Webhooks: `{type, timestamp,
workspace_id, inbox_id, data}`, with the conversation, message and contact in `data`; the contact
carries `external_ids` (so the host maps it to its own user) and `online`. `contact.deleted`
carries the external ids the contact had.

Delivery: the transaction that writes an event also queues a River fan-out job when the workspace
has an enabled endpoint; the job builds the payload once and creates a delivery per matching
endpoint, each sent by its own job. Requests are signed per Standard Webhooks (`webhook-id`, the
same on every retry; `webhook-timestamp`; `webhook-signature: v1,<base64 HMAC-SHA256 over
id.timestamp.body>`, space-separated during a rotation). A 2xx answer succeeds; anything else is
retried after 5 s, 1 min, 5 min, 30 min, 1 h, 2 h, 4 h, 8 h and 9 h (up to 10% jitter), about 24
hours in all. An endpoint whose every attempt has failed for 24 hours, or that answers `410 Gone`,
is disabled with the reason shown to members; enabling it again clears the reason, and any
delivery can be sent again by hand with the same `webhook-id`. The delivery log keeps the newest
100 attempts per endpoint (status code, latency, the first 1 KiB of the answer, the error);
finished deliveries are kept 7 days.

Outbound requests never reach internal networks: the URL must be http(s) without credentials, and
on every attempt the host is resolved once, refused when any address is loopback, private,
link-local (cloud metadata included), CGNAT, multicast or otherwise reserved, and the connection
goes to that resolved address only. No proxy, no redirects (a 3xx is a failure), a 10 second
timeout and at most 64 KiB of the answer read. `YUVA_WEBHOOK_ALLOW_PRIVATE=true` lifts the address
check for development (the dev compose file sets it); never set it on a shared server. Link-local
and cloud metadata addresses (169.254.0.0/16, fe80::/10, fd00:ec2::254, IPv4-mapped forms
included) are refused even then, when the endpoint is saved and on every attempt.

`sdk/go/webhook` verifies the signatures for Go backends.

### Panel (`web/`)

React, Vite, shadcn, TanStack Query, Lingui; built to static files and embedded in the binary.
The server sends the panel with `X-Frame-Options: DENY`, `Referrer-Policy: same-origin` and a CSP
that allows only its own scripts, connections and workers, no framing (`frame-ancestors 'none'`) and
no plugins; inline styles and any http(s) image stay allowed for the sandboxed e-mail frame, which
inherits the policy. Every response carries `X-Content-Type-Options: nosniff`.

E-mail HTML has two views, both in a sandboxed `srcdoc` iframe under a CSP of its own (no
scripts, no forms, images only from `data:`, the server's attachments and, once the member allows
remote images, http(s)):
- Reading: the sanitized HTML (`html`, `full_html`; bluemonday's UGC policy, so no
  styles or colours survive) drawn in the panel's own type and theme colours, light or dark, redrawn
  when the theme changes. It never scrolls sideways: tables, `width` attributes and images are held
  to the frame's width and long words break. Table cells are padded, rows divided by a thin line
  in the theme's border colour, and header cells bold. Images sit on a light plate so dark-on-transparent
  logos stay visible in the dark theme.
- Original (the default when the message has one): `original_html` from `GET /v1/messages/{id}/email`,
  sanitized with a second policy that keeps inline styles limited to an allowlist of CSS
  properties and the colour, background and size attributes, never `<style>` blocks, scripts,
  forms or external stylesheets. In the light theme it is drawn on white, as the sender designed
  it. In the dark theme it is darkened automatically before it reaches the frame (the frame runs
  no scripts): colours from inline styles and `bgcolor`/`color` attributes are mapped in OKLCH,
  light backgrounds to dark ones of the same hue, text lightened until it reaches WCAG 4.5:1
  against the background it sits on, borders alike; elements without a colour take the theme's;
  images are left as they are. A button shows the mail as sent, on white, when the conversion
  gets one wrong. Content wider than the frame is scaled down to fit (to 60% at most) and only
  beyond that scrolls inside the frame, never the page; a soft fade on the edge that has more to
  scroll shows that it does.

- Which e-mail view opens first is a per-device preference next to the theme (Settings ›
  Appearance: Original or Reading, Original by default); each message can still be switched.
- Setup: a full-screen, two-column page in the sign-in design (steps and form on the left, a live
  preview on the right) takes an owner or admin from an empty workspace to its first
  conversation: name the inbox, pick a channel (website chat, e-mail, app or documentation site),
  install it (snippet, forwarding address, SDK lines or the documentation elements), then wait live
  for the first message, which the preview's own chat widget can send. A documentation site asks
  for the site's name and address, creates a chat channel named after it with that origin and
  anonymous visitors allowed, and shows the elements for plain HTML, Nuxt and Fumadocs/React filled
  with this server's URL and the channel key, with a link to the guide; the inbox settings start
  the same flow for another docs site. It opens on its own while the workspace has no conversation (it can be put
  off; that is remembered per workspace on the device), and adding an inbox later uses the same page.
- Connect an assistant (Settings › Connected apps): one page per client (Claude, Claude Code, Codex,
  ChatGPT, Cursor, VS Code, other MCP clients) with the server's own `/mcp` URL, a copyable command
  or config, install links where the client has them, and the steps; a new grant shows up there live.
- Main screen, in the shape of a messaging app: a narrow icon rail and three columns on a desktop.
  Every page except setup and the OAuth consent page lives in this shell: the rail picks what the
  left column lists, and the chosen item opens in the main area (nothing chosen shows a short
  placeholder there). Contacts (`/contacts`) lists the directory with its search and filters on
  the left and opens a contact's page in the main area; Settings (`/settings`) lists the settings
  groups on the left (You, Inboxes, Team, Developer for owners and admins, Workspace) and opens
  each page in the main area, at the same URLs as before.
  The rail switches the left column between Conversations (with the number of unread
  conversations among the loaded ones), Mentions (the notes that mention the member from
  `/v1/me/mentions`, unseen ones marked and counted on the rail from `unseen`), Team (`/team`: each member's presence, what they are looking at
  and their open load from `assignees` in `/v1/conversations/counts`; a member opens in the main
  area with their open assigned conversations and today's replies and closes), Reports (`/reports`:
  today's `/v1/stats` in three pages opened in the main area: replies, closes and first replies;
  ratings, in total and per inbox; per member) and Assistants
  (the pending drafts from `/v1/drafts`, filtered by author kind and inbox, with `total` on the
  rail, above the connected apps from `/v1/oauth/grants`); both lists refresh from the realtime
  events named with their endpoints, and a row opens its conversation at the note or draft
  (`?m=<message id>`, loading older pages until it is there). The rail also leads to contacts and
  settings and holds
  the member's avatar with the availability switch. Each entry has a tooltip and a `G` shortcut.
  On a phone the rail is a tab bar under the left column, and every section has the two screens
  of the conversations: the list first, then the chosen page with a back button. The conversations column has the workspace, new
  conversation and a menu to contacts, setup, the command palette and the shortcuts; a search field (Postgres FTS over people and messages);
  filter chips (All, Unread, Mine, Unassigned, Snoozed, Done, which are the list filters `status`
  and `assignee`, Unread filtering the loaded rows by `unread`) and an inbox picker; a Filter
  popover after the chips narrows by type (Live chat, E-mail, App: `kind=conversation` with that
  `channel_kind`; Feedback, Question: that `kind`) and by site (`site=<origin>`, which keeps the
  feedback and questions sent from that site's pages; the sites come from `GET /v1/docs/sites` and
  the chat channels' origins), kept for the session; then the
  conversation list, most recent activity first, paged as it scrolls and kept current from the
  realtime connection. A row shows the contact, the last message (marked when a bot or the team
  wrote it, replaced by the member's unsent draft or by "typing…"; a team reply in a chat or app
  conversation shows ✓, or ✓✓ once `last_read_by_contact_at` reaches it), the time, an unread mark, the
  channel, the assignee and a green dot when the contact of a chat or app conversation is online.
  It also says where the conversation came from: a source badge (Live chat, E-mail, App or API from
  the starting channel's `channel_kind`; Feedback and Question take precedence) and, for feedback
  and questions, the page's host and path, for chat and app conversations the channel's name. A
  contact with neither name nor e-mail is "Visitor · <site host or channel name>" with a neutral
  avatar, never initials; the conversation header shows the same name, badge and place.
  ↑/↓ move through the list, Enter opens, J/K open the next or previous one. Each member can pin
  conversations to the top of their own list and mark one unread again, from the row's context
  menu or the conversation's menu (P, U): `PUT`/`DELETE /v1/conversations/{id}/pin` and
  `POST /v1/conversations/{id}/unread`, kept per member in `conversation_member_states` next to the
  read cursors. The list item carries `pinned_at` for the caller, `pinned=true|false` filters on
  it, a mark shows as `unread` until the member reads the conversation or writes in it, and
  `conversation.pin` and `conversation.read` events reach only that member's connections. Pinned
  conversations of the current filter stand in their own section above the rest. The list also
  filters by where a conversation came from: `kind` and `channel_kind` (several values, repeated
  or comma-separated) and `site`, the origin of a feedback's or question's page. Each conversation
  carries `channel_kind`, the kind of the channel it started on (stored on the conversation when
  it is created and kept when the channel goes or the conversation moves), so a row shows its
  source without another request; a page's URL and title are under `feedback` or `question`. In the middle, the
  open conversation (`/conversations/:id`): a header with the contact, channel, status, assignee
  and the actions (assign or hand off, snooze, labels, close or reopen, more), the thread built on
  shadcn's chat components (message, bubble, marker, attachment, message scroller) with date
  separators. Every item is a chat bubble in a message with header and footer slots, the contact
  on one side and the team on the other, never a wide card: an e-mail shows its subject on the
  first message and whenever it changes, then the body, its HTML (original or reading view) in a
  bubble that grows to the thread's width, with sender and tags in the header and time, view
  toggles, quoted text, remote images and the `.eml` download in the footer; a note is a bubble
  from its author on the team's side in the note colour, marked in its footer as seen only by the
  team; feedback is a bubble from the contact with its category in the header; attachments of any
  message use the attachment component, images as thumbnails; drafts are outgoing bubbles with
  Send / Edit / Discard; events and dates are markers. The thread sticks to the newest message
  while the member is at the bottom and loads older pages on scrolling up; the composer is at the
  bottom. On the right, the contact details, toggled
  from the header; below 1200 px wide they open as a sheet, never as a third column, and the
  header drops the inbox name, the assignee, the labels button and the close button's text when
  the conversation is narrower than 36rem, so the contact's name keeps its room. On a phone the line under the name
  (source, place, status) wraps instead of being cut, and assign, snooze and search move into the
  more menu next to close. On a phone the list and the conversation are
  two screens with a back button. With nothing open the middle shows what the team did today
  from `GET /v1/stats`: replies sent by members, conversations closed and the median time to the
  first reply, per member too, over the inboxes the caller can see. Nothing is stored for it: it
  is counted from `messages` on each request (indexed by workspace and time) for a window of at
  most 366 days; "today" starts at midnight in the `timezone` the panel sends (the browser's),
  since members and workspaces have no time zone of their own.
- Acting on a conversation never opens another one. Close, snooze and hand-off apply at once and
  offer an undo in a toast. A reply is held for a few seconds before it is posted, shown in the
  thread as sending, so it can be undone; "send" also sets `pending`, "send and close" sets
  `closed`. A bot's draft is offered as the suggested reply. Live conversations are different: a
  conversation from a chat or app channel whose contact is online right now is talked through.
  Its replies are sent at once (no hold) and leave the status `open`. In every chat or app
  conversation, online or not, Enter sends and Shift+Enter starts a new line; in e-mail
  ⌘/Ctrl+Enter sends and Enter starts a new line. A note takes the same keys as its conversation.
  The hand-off menu shows each teammate's presence and open load
  (`assignees` in `/v1/conversations/counts`, over the inboxes the member can see). Everything
  else is in a command palette.
- Conversation: reply and note in one box, canned replies on `/`, attachments (picked, pasted or
  dropped anywhere on the conversation), an emoji picker (frimousse, MIT; its Emojibase data is
  served by the server itself under `/emojibase/`, loaded on first use and not precached),
  keyboard shortcuts, contact details with identity attributes, the contact's other conversations,
  the conversation's media, links and documents (from the loaded messages), channel delivery
  state, ✓/✓✓ read receipts on the team's chat and app messages, actions on each message (reply
  with a quote, copy, forward to a teammate as a note that mentions them), a search within the
  loaded messages of the open conversation that highlights the matches and jumps between them, and a notice when another member types a reply in the same conversation or has it open.
  The panel reports the conversation it shows with a `viewing` frame on `/v1/realtime`; the server
  keeps it on the connection's row (see Member notifications), sends a `viewing` notice to the
  other members who can see the conversation when a member opens or leaves it (hiding the page and
  disconnecting count as leaving), and answers the frame with the members already there.
- Settings: inboxes, channels, members and access, labels, canned replies, business hours,
  auto-replies, webhooks, API keys, retention and deleting the workspace; deleting one's own
  account in the profile.
- Installable PWA with Web Push (VAPID), so members get notifications on phones without a native
  app. E-mail notifications as a fallback (see Member notifications).
- Sign-in with an e-mailed one-time code and passkeys. Sign-up is a server setting,
  `YUVA_SIGNUP`, reported to the panel without sign-in on `GET /v1/version` (`signup`). `invite`
  (the default): the first owner is created with `yuva bootstrap`, everyone else is invited, and
  a code goes only to a person or an invited address. `open`: a code goes to any address (same
  rate limits, same fixed delay, same `202`), verifying it creates the person with no workspace,
  and `POST /v1/workspaces` lets a signed-in person create a workspace they own, through the same
  store path as `yuva bootstrap`. The workspace records who created it and from which IP address
  (IPv6 /64); a person may have `YUVA_SIGNUP_WORKSPACES_PER_PERSON` (3) live workspaces they
  created (`403 workspace_limit`) and one address may create `YUVA_SIGNUP_WORKSPACES_PER_IP` (5)
  in 24 hours (`429 rate_limited`). Under `invite` that endpoint answers `403 signup_closed`.
  A code request carries the sign-in page's `locale`, kept on the code row: for an address with
  no person and no invite it is the code mail's language and the new person's locale; a person
  keeps their own and an invite's locale wins.
  `YUVA_OPERATOR_EMAIL` (comma-separated, unset = off) tells the server's operator about
  growth: a new person created by verifying a code (no person, no invite; readers marked) and a
  workspace created with `POST /v1/workspaces` are written as an `operator_notices` row in the
  same transaction (address, workspace name, client IP, and the country only from `CF-IPCountry`
  when `YUVA_CLIENT_IP_HEADER` is `CF-Connecting-IP`, since Cloudflare overwrites it). A River
  job, periodic every minute, sends one English e-mail (`operator_digest`) to every address with
  the unsent rows, at most once per 5 minutes after the last one, so a burst becomes one mail.
  Sent rows are deleted after an hour; they are kept only to know when the last mail went out.
  No conversation, message or contact content goes into it.
  The panel follows `signup`: with `open` its sign-in page says a new address gets an account
  (`/?signup=1` opens it as "Create your Yuva account"), a person with no workspace is asked to
  create one (name prefilled from the e-mail domain) and goes on to setup, and the workspace
  switcher offers "Create workspace" (`/workspaces/new`); with `invite` the page says the server
  is invite only and a person with no workspace sees that they are not in one yet.
  A person whose memberships are all
  gone can still sign in, to see that and to delete their account. Sign-in and invitation e-mails are
  sent directly, not through the job queue, so a code never lands in job arguments; a code request
  answers after the same fixed delay for every address and its mail goes out after the answer, so
  timing does not reveal members. Each code takes five attempts; after 20 wrong codes for an
  address within 24 hours its codes are refused (`429 sign_in_paused`) and the member gets one
  e-mail about it. Passkeys are not affected.

### Member notifications

Six events notify members: the first message of a conversation, from the contact, in a `live`
or an `async` inbox (every member with access is a candidate); a later contact message in a
conversation assigned to the member, or in an unassigned one (every member with access);
someone else assigning a conversation to the member; and someone else mentioning the member in a
note. A note names its mentions as member ids (`mentions` on `POST .../messages`, each a member
who can see the inbox), stored on the message; the server does not parse `@` in the text. A River job queued in the transaction that
stored the message picks the recipients. Nobody is notified about their own action, about a
conversation marked spam, or about a conversation their open panel shows: the panel reports it
with a `viewing` frame on `/v1/realtime`, stored on the connection's row and trusted while the
connection is fresh (75 seconds). A member set to `away` gets only the events about
conversations assigned to them and mentions.

Each member chooses push and e-mail per event, and can override events per inbox. Defaults by
role: push for everything, except that agents get no push for new `async` conversations and for
messages in unassigned conversations (owners and admins triage those); e-mail only for messages
in conversations assigned to the member, for assignments and for mentions, for every role, since
being assigned or asked makes anyone responsible.

`GET /v1/me/mentions` lists the notes that mention the calling member, written by someone else,
newest first, in the conversations the member can see now (losing an inbox hides its mentions), each
with a preview of the note, its author and a conversation summary. A mention is `seen` once the
member's read position in that conversation (the same cursor as `unread`, moved by reading or
writing there) reaches the note; there is no separate mark. `unseen` in the answer counts the
unseen ones over all pages. Mentions belong to a member, so the list needs a member session; keys and
OAuth tokens get `403 member_session_required`. A partial GIN index on `messages.mentions` for notes
serves it. A client keeps it current from `message.created` (a note whose `mentions` holds the
member), `conversation.read`, `conversation.moved` and `inbox_access.changed`.

Web Push follows RFC 8030, 8291 and 8292 through `webpush-go`, with the server's VAPID keys
(`YUVA_VAPID_PUBLIC_KEY`, `YUVA_VAPID_PRIVATE_KEY`, `YUVA_VAPID_SUBJECT`; `yuva vapid-keys` makes
a pair; push is off without them). A subscription belongs to the browser and to the session that
registered it, so sign-out ends it; it receives notifications from every workspace of the person.
One River job sends each push, with a TTL (4 hours for `live` inboxes, 24 for `async`), an
urgency (`high` for `live`) and the conversation as `Topic`, so a newer push replaces an
undelivered one. The payload is minimal: inbox and contact name, the start of the message (140
characters), the conversation's path, a per-conversation `tag`, the conversation, inbox and
workspace ids. 404 and 410 from the push service delete the subscription, 429, 5xx and network
errors are retried (5 attempts), other answers are recorded on the subscription. Push endpoints
pass the webhook address checks: `https` only, never a private or link-local address, resolved
and checked on every attempt.

While the panel is open, each browser can also alert in the tab itself (Settings ›
Notifications, off by default, kept on the device): while the tab is hidden, a contact's new
message in a conversation assigned to the member or to nobody (only the assigned ones while away)
plays a short sound and, once the member has allowed it with the button there (never asked on
load), shows a desktop notification with the same `tag` as the Web Push one, so the two replace
each other instead of stacking.

E-mail fallback: an event with e-mail on schedules one check per member and conversation after the
member's delay (15 minutes by default). If the conversation still has contact messages (or an
assignment, or notes mentioning the member while they have e-mail on for mentions) newer than the member's read position and the previous notification e-mail, one
e-mail through the server's own mailer, in the member's locale, lists them and links to the
conversation and to the notification settings; at most one per member and conversation per hour. A
member's own message or note moves their read position to it, so the e-mail never lists what
they already answered.

### Satisfaction ratings

An inbox can ask contacts to rate their conversations (`ask_for_rating`, off by default). A
contact rates a closed conversation `good` or `bad`, with an optional comment of at most 2,000
characters, once per close: a message from the contact reopens it and the next close allows a new
rating. A close can be rated for 30 days, and only a close after the inbox started asking
(`inboxes.rating_since`, set when `ask_for_rating` turns on and cleared when it turns off), so
turning it on never offers old conversations. Ratings are not stored for spam.

- Conversations keep `closed_at` (set when the status becomes `closed`) and the latest rating
  (`rating`, `rating_comment`, `rated_at`); a rating counts for the current close when `rated_at`
  is not before `closed_at`. The rating is also a `rated` event message in the thread, authored by
  the contact, with the comment as its body; contacts never see event messages.
- Widget and SDKs: `ask_for_rating` on the inbox's public settings, `can_rate` and `rating` on each
  `ClientConversation` (and on the realtime status frame when a conversation closes), and
  `POST /client/v1/conversations/{id}/rating`.
- Members see it on the conversation (`rating`, `closed_at`) and in the thread; `conversation.rated`
  goes to webhooks; `/v1/stats` counts ratings given in its window, in total and per inbox.
- E-mail contacts: two minutes after a member or API key closes an e-mail conversation (time to
  undo), a River job checks that it is still closed and unrated, a member or bot replied in it and
  the address is deliverable, then sends one automatic reply in the thread (`Auto-Submitted`, in
  the contact's language, else the inbox's) with two links, `/r/{token}?rating=good|bad`, once per
  close. The token seals the workspace, the conversation and the close time under the master key,
  so nothing is stored for it and a later close makes it invalid; the request is a message in the
  thread, so members can see the links.
- `/r/{token}` is a page the server renders itself: no scripts or external assets, the inbox's
  name and branding colour, English, Turkish or German by the inbox's language. Opening it stores
  nothing, since mail scanners open links: it shows the chosen rating preselected and a comment
  field, and only its POST rates.

### Documentation pages

Products keep their documentation on their own sites (Nuxt Content, Fumadocs, Starlight,
VitePress, plain HTML). Yuva adds two elements to those pages and stores nothing of the
documentation itself. Both use an existing `chat` channel: its public key, its origins and its
rate limits, so a docs site is set up like a site with the chat widget, and the same channel may
serve both.

- **A page** is the page's URL without query and fragment, and its origin must be one of the
  channel's origins (`400 page_origin` otherwise). The title is what the page sent last. The
  elements follow the URL themselves, since documentation sites navigate without reloading;
  a `page` attribute overrides it.
- **Page ratings.** `<yuva-page-feedback>` asks whether the page helped: `up` or `down`. A rating
  alone creates no contact and no conversation: `POST /client/v1/channels/{key}/page-ratings`
  needs no session and adds one to a per-page, per-day counter (`page_ratings`: workspace, inbox,
  channel, page, title, day, up, down). Nothing about the visitor is stored; the element
  remembers the visitor's rating in the browser and a changed mind moves the count (one off the
  latest day that has it), and the endpoint is rate limited per IP address and channel like
  session starts.
- **Page feedback.** After a rating the element offers a text field and, optionally, an e-mail
  address. Sending it is `POST /client/v1/feedback` from a visitor session on the chat channel, a
  `feedback` conversation like the app's, with `page_url`, `page_title` and `rating` in its
  feedback metadata and category `other` unless the visitor picked one. It is answered like any
  feedback; replies reach the visitor by e-mail when they allowed it.
- **Page questions (M11, replaced by discussions).** Until M12 a visitor could ask a private
  question from a page (a conversation of kind `question`) and members published an edited
  question and answer back to the page (`page_answers`). Migration 00029 turned every published
  answer into a discussion and every `question` conversation into an ordinary `conversation`;
  conversations keep `page_url` and `page_title` as columns (page feedback uses them too) and show
  them as `page` on the conversation, so the panel still filters and counts conversations per page.
- **Sites.** A site is the origin of page URLs, per inbox: one product's documentation.
  `GET /v1/docs/sites` lists every origin that has pages in the window (as `GET /v1/docs/pages`
  counts them) and every origin allowed on a chat channel of the visible inboxes, with zeros for
  a site added but not yet used, named by the chat channel that allows it, with its page count and
  summed counts. `site=<origin>` narrows `GET /v1/docs/pages`, `/v1/docs/summary`,
  `/v1/docs/page`, `GET /v1/discussions` and `GET /v1/conversations` to one site.
- **Panel.** A Docs view is per site: it starts from the sites list and lists pages of the inboxes
  the member can see with their up and down counts over a window (7, 30 or 90 days), the share of
  `down`, open feedback and the page's discussion counts now: open threads (published, no
  accepted answer, not locked), unanswered threads (open and nobody replied) and pending posts
  (`GET /v1/docs/pages`), sorted by most `down` (default), most `up`, most helpful (share of `up`,
  first among pages with at least 5 ratings in the window, so a single `up` never leads) or most
  activity (ratings plus open feedback, open threads and pending posts). The list is an
  aggregate, so its cursor is a position within one sort and window. Above it, totals over the
  window and a zero-filled series per day of ratings, page feedback and new threads
  (`GET /v1/docs/summary`) feed the charts. A site picker at the top of the Docs column (all
  sites, or each site by its channel name and host, from `GET /v1/docs/sites`) passes `site=` to
  the list, the summary and the page; changing it while a page is open goes back to the overview.
  A page shows its ratings per day (`GET /v1/docs/page`), its feedback
  (`GET /v1/conversations?page=`) and its threads (`GET /v1/discussions?page=`).
- Ratings are counters, so they send no events or webhooks; page feedback is a conversation and
  sends the same ones as any conversation and feedback.

### Discussions (M12)

Discussions replace page questions and published answers. A documentation page gets a public
discussion area like GitHub Discussions: threads, replies, votes, an accepted answer, followers.
Private help stays private: page feedback, live chat and e-mail are unchanged. Decisions:

- **Model.** Discussions are not conversations: a conversation is one contact and the team,
  private; a discussion is public and has many authors. `discussions` (workspace, inbox, channel,
  page, page title, title, opening post, state, `locked_at`, `answer_post_id`, `answered_at`,
  votes, reply count, pending and hidden counts, source conversation, last activity) and
  `discussion_posts` (discussion, author, body as Markdown, `opening`, `parent_post_id`, state
  `published|pending|hidden`, votes, edited and deleted times), plus `discussion_votes` and
  `discussion_follows`. The thread's body, author and votes are those of its **opening post**, a
  post like the replies, so editing, deleting, voting and moderating work the same for both;
  `discussions.state` follows the opening post's state and is changed with it in one transaction.
  Replies are one level deep: a reply to the thread has no parent, a reply to a reply points at
  it, and a reply to a reply to a reply is refused. The accepted answer is a published reply to the
  thread. State `open|answered|locked|hidden` from the plan is split into `state`
  (`published|pending|hidden`), `answer_post_id` and `locked_at`, since a thread can be answered and
  locked at once. Every table has `workspace_id`; every query filters by it.
- **Authors.** A post's author is a member or a contact (`author_type` `member|contact|anonymous`,
  with the ids nulled on delete: a removed member or a deleted reader shows as deleted, like
  deleted members in conversations). The public sees a display name and initials only: a member's
  name, a reader's person name (live, so renaming the account renames their posts), else the
  contact's name; no e-mail addresses, ids or avatars from elsewhere. `anonymous` is used only for
  threads migrated from published answers, whose visitor was never named.
- **Bodies** are stored as Markdown and served with `body_html`, rendered on the server by goldmark
  (no raw HTML, GitHub-flavoured tables, strikethrough and autolinks) and sanitized with
  bluemonday's UGC policy (`rel="nofollow noreferrer"`, `target="_blank"` with `noopener` on links to other sites), so every client can show it without a
  Markdown library. Titles 3 to 200 characters, bodies up to 20,000.
- **Migration.** Each `page_answers` row became a published discussion on its page, inbox and
  channel: the question as an `anonymous` opening post, the answer as an accepted reply by the
  publishing member (`anonymous` once that member is gone), `conversation_id` kept. Every
  `question` conversation became `conversation`, keeping its page columns (see "Documentation
  pages"). `kind=question`, `/v1/conversations/{id}/publish`, `/v1/page-answers*`,
  `/client/v1/questions` and `/client/v1/channels/{key}/page-answers` are removed from the
  contract and `page_answers` is dropped; this is pre-1.0 and they shipped in M11.
- **Readers.** A reader is a person with a server-wide account (`people`, the table members sign in
  with): one account per Yuva server, signing in by e-mail code or passkey on the server's own page,
  never on the documentation site. `POST /v1/auth/code` with `reader: true` sends a code to any
  address (same rate limits; the code row records it) and verifying that code creates the person,
  whatever `YUVA_SIGNUP` says; a member signing in there is the same person. The reader's
  account page and its popup are panel routes (`/account`, `/account/connect`) served like the
  OAuth consent page, with the person's session cookie.
- **Reader tokens.** `<yuva-discussions>` opens `/account/connect?channel=<key>&origin=<origin>` in
  a popup. Once signed in, the page calls `POST /v1/account/reader-tokens` with the key and the
  origin; the server refuses an origin that is not exactly one of the channel's allowed origins
  (`403 origin_not_allowed`) or a channel with discussions off, and the page posts the token to
  `window.opener` with that origin as `targetOrigin`, so no other page can receive it. A token
  (`yuva_rd_…`, stored as a hash in `reader_tokens` with the workspace, channel, person and the
  session that made it) lives 24 hours, is bound to its channel (its origin check is the
  channel's, like a contact session), and ends with that session (sign-out), with the account,
  or with `DELETE /client/v1/reader-token`. When it expires the element opens the popup again,
  which answers at once while the person is still signed in on the server.
- **Readers in a workspace.** A reader token acts in its channel's workspace as the contact linked to
  the person (`contacts.person_id`, unique per workspace), created on the first write: a contact of
  the workspace with the person's e-mail address and no person is linked, else a new contact is
  made with that address and the person's name. So the team sees the name and e-mail address of
  readers who write in their workspace, and the connect page says so; other workspaces see
  nothing of it. A person who is a member of that workspace with access to the inbox writes as that
  member instead. Host apps with their own login keep using identity tokens: a contact session
  started with an identity token writes as its contact; anonymous visitor sessions only read.
- **Reading is public** and needs no session: `GET /client/v1/channels/{key}/discussions?page=` and
  `…/discussions/{id}`, cacheable for a minute, authors shown by display name and initials only. A
  reader token or contact session of that channel adds the caller's own pending posts and the
  `mine`, `voted` and `following` flags, and the answer is then `private, no-store`. Threads are
  listed per inbox and page, so two chat channels of one inbox that allow the same origin show the
  same threads; the channel decides whether discussions are on.
- **Writing** needs a reader token, an identity-token contact session or a member (`/v1`). Per chat
  channel `discussions: off|moderated|open` (default `moderated`). Moderated: a reader's post waits
  (`pending`) until one of their posts in the workspace has been approved, after which they publish
  at once; identity-token users and members publish at once. Open: everyone publishes at once.
  Marking a post spam hides it and the author's other pending posts and sets
  `contacts.discussions_blocked_at`, after which the author's posts always wait. Pending posts are
  seen only by their author and members, hidden ones only by members. Authors edit their posts for
  30 minutes (members their own without limit) and delete them at any time: a post with replies
  stays as deleted without body or author, the thread goes when nobody else wrote in it. A locked
  thread takes nothing from readers (posts, edits, votes, answers); members still may. The thread's
  author or a member accepts an answer. Rate limits: 10 posts a minute and 60 an hour per author,
  and the client write limits per IP address and channel.
- **Team.** `/v1/discussions` lists threads per inbox, site, page and state; `/v1/discussion-posts`
  with `state=pending` is the moderation queue. Members approve or hide posts (`PATCH`), mark spam,
  lock and retitle threads, reply, vote, accept and clear answers, and start threads. Reading needs
  `conversations:read` (keys and tokens too); writing needs a member session. Agents see the
  threads of their inboxes only; another inbox or workspace answers `404`.
- **From a conversation.** `POST /v1/conversations/{id}/discussion` starts a published thread with
  the title and body the member writes (and optionally an accepted answer by the member); nothing
  of the conversation becomes public. Its contact is the author only when it is linked to a person
  or has an external id in the inbox (they are a known user and can see and edit the thread);
  otherwise the member is. One thread per conversation.
- **Notifications.** Followers are contacts (members see threads in the panel). An author follows
  their thread and any thread they reply to. A published reply (on creation or on approval) and an
  accepted answer add a `reader_notifications` row for every follower linked to a person (except
  the author) and queue a River job per follower and thread, unique by its arguments for
  `YUVA_DISCUSSION_EMAIL_DELAY` (10 minutes by default). The job sends at most one e-mail per
  follower and thread per delay, listing the replies and the answer since the last e-mail, through
  the server's own mailer in the person's locale (else the contact's, else the inbox's), to the
  person's address when they kept `email_notifications` on, or to an identity-token contact's first
  address. Each e-mail carries `List-Unsubscribe` with `List-Unsubscribe-Post: List-Unsubscribe=One-Click`
  (RFC 8058) pointing at `/u/{token}`: the token seals the workspace, thread and contact under the
  master key like rating links; opening the page changes nothing and its button (or the one-click
  POST) unfollows. Reader notifications are kept 90 days.
- **Events.** `discussion.created`, `discussion.post_created`, `discussion.answered` and
  `discussion.updated` (moderation, edits, deletes, lock, a cleared answer) are stored events for
  members who can see the inbox and webhook types under Standard Webhooks like the others, with
  `{discussion, post}` as data. Pending posts send them too, with their state.
- **Reader account.** `/account` on the server, in `web/`, separate from the team panel:
  my discussions, followed threads, notifications, e-mail settings, delete account
  (`/v1/account*`, `DELETE /v1/me`). A person who is also a member reaches the team panel from it.
- **Docs counts.** `/v1/docs/pages` and `/v1/docs/sites` count open threads, unanswered threads and
  pending posts per page instead of open questions and published answers; `/v1/docs/summary`
  counts new threads per day and pending posts now.
- **UI for sites.** `<yuva-discussions>` (web component, Shadow DOM, in `/yuva-docs.js` and
  `useyuva/docs`, replacing `<yuva-page-questions>`; Nuxt and React wrappers as today) and a
  shadcn registry for React sites that want the code in their own design system: components built
  on shadcn/ui primitives and the headless `useyuva` client, sources in `sdk/shadcn/` (MIT), served
  as registry JSON by the site so `npx shadcn add <site>/r/discussions.json` installs them.

### Broadcasts and e-mail designs (M13)

A workspace sends e-mail to many contacts at once: newsletters, product updates, announcements. Every
broadcast is sent on behalf of an inbox through one of its e-mail channels, and a reply becomes a
conversation in that inbox. Plan in issue #25; it ships in five steps (M13.1–M13.5 in the roadmap).
Decisions:

- **Lists.** `mailing_lists` belong to an inbox: name, public description, `double_opt_in` (on by
  default), `public` (offered on forms and the preference page), `commercial` (marketing content, for the
  consent export), `welcome` (a receipt after subscribing) and the e-mail channel that sends its
  confirmation and welcome mail. Owners and admins manage lists; agents read the lists of their inboxes.
- **Subscriptions and consent.** `list_subscriptions` (list, contact, address, `pending|subscribed|
  unsubscribed`, source `form|identity|api|member|import|preferences`) and an append-only `consent_events` log (what
  happened, when, IP, user agent, page, the label the person saw, the member, key or import). Consent is
  never assumed: API calls and imports declare it (an import needs the importer's attestation and where
  people agreed, stored with every row). Only the person moves an `unsubscribed` subscription back, through
  a confirmation. Unsubscribing from all of an inbox's lists, a complaint and deleting a contact who had
  objected leave an `email_optouts` row (a keyed HMAC of the address, nothing else) that the API and imports
  respect. Pending subscriptions go after 30 days (one that was unsubscribed before and asked again returns
  to `unsubscribed`, so the objection stays); unsubscribed ones and their events after 3 years, leaving an
  opt-out.
- **Signing up.** `<yuva-subscribe>` (`/yuva-subscribe.js`, `useyuva/subscribe`, React and Nuxt
  wrappers, shadcn registry items) on an existing chat channel's key and origins; anonymous forms post to
  `POST /client/v1/channels/{key}/subscriptions` with a sealed form token, a honeypot and per-IP,
  per-channel and per-address limits, and get the same answer for every address. Identity-token sessions
  subscribe at once when the contact has a known address (verified by the token, or given by a member or an
  e-mail conversation), else through a confirmation, as does a subscription the person left before
  (`PUT /client/v1/subscriptions/{list}`). Double
  opt-in mails a link to `/s/{token}`, a server-rendered page like `/r/{token}` whose button confirms
  (opening it changes nothing). Host backends use `/v1/lists/{id}/subscriptions`; the panel imports CSV
  files (below).
- **Importing.** A CSV upload (`/v1/lists/{id}/imports`, at most 20 MiB and `YUVA_IMPORT_MAX_ROWS` rows,
  default and maximum 100,000, one running import per workspace) needs the importer's attestation and where
  people agreed, both stored on every consent event with the import's id. The file goes to object storage
  and is first validated without writing anything: the encoding (UTF-8 with or without BOM, UTF-16 with
  BOM, else Windows-1254 when the bytes include a letter only Turkish uses there, else Windows-1252), the
  delimiter (`,`, `;` or tab) from the header row, a guessed column mapping and the counts a member
  confirms. It refuses invalid addresses, domains that take no mail (no MX or a null MX), role addresses,
  duplicates and every opted-out, unsubscribed or suppressed address. Importing writes 1,000 rows per
  transaction and snoozes every ~30 s; progress reaches `/v1/realtime` only (`list_import.progress` at
  most once a second, `list_import.updated` on each state change), never as stored events, webhooks or a
  `subscription.*` event per row. Imports send no welcome mail; in re-confirm mode every row becomes
  `pending` and gets a confirmation, spread over the days by the workspace's daily broadcast limit. The
  upload is deleted when the import ends, refused rows stay 7 days for the error CSV, and an import never
  started is cancelled after 7 days.
- **Unsubscribing.** Every broadcast and welcome mail carries `List-Unsubscribe` with
  `List-Unsubscribe-Post: List-Unsubscribe=One-Click` (RFC 8058) and footer links to `/m/{token}`: a page
  with "unsubscribe from this list", the inbox's public lists and "unsubscribe from everything"; opening it
  changes nothing, its one-click POST unsubscribes at once. Tokens seal the workspace, list, contact and
  broadcast under the master key and never expire while the contact exists.
- **Designs.** A design is a JSON document (`EmailDesignDocument` in the contract, `schema: 1` with
  upgrade steps on read) of blocks (section, columns, heading, text, image, button, divider, spacer,
  social, footer) and a theme (colours with dark overrides, a fixed font stack, width, radius,
  direction). Rich text is a stored subset of HTML checked by a bluemonday policy on every write; there is
  no raw-HTML block and no remote image. `api/internal/mail/design` renders it to table HTML with inline
  styles (hybrid columns, dark-mode classes, Outlook wrappers, a preheader) and a text part; it shares its
  colour and button code with the server's own `layout.html`. Merge fields `{{ contact.first_name |
  "there" }}` are checked on save, and a broadcast is not scheduled while a field without fallback would be
  empty for anyone. A required footer (the inbox's legal name, postal address, registration and contact,
  why the person gets the mail, unsubscribe and preference links) is always added. `email_designs` keep
  a version for concurrent edits and up to 50 snapshots.
- **Images** are uploaded (PNG, JPEG, GIF, WebP; never SVG), re-encoded without metadata at most 1,200 px
  wide (WebP stored as PNG or JPEG) and served publicly at `/i/{token}` with immutable caching and nothing
  recorded per request: the one exception to "objects are never public".
- **Broadcasts.** A draft names the inbox, the e-mail channel, lists (up to 10) with attribute and
  language conditions, the design, subject and preheader. Scheduling runs a review (sender identity,
  sender ready, approval, merge fields, HTML under Gmail's clipping size, deliverability) and freezes the
  design. At send time `broadcast_start` snapshots the audience into `broadcast_deliveries` (one per
  contact; suppressed, opted-out and unsubscribed addresses left out) and `broadcast_send`, one job per
  sender channel on its own River queue (`YUVA_BROADCAST_WORKERS`, default 2), sends in batches over one
  SMTP connection at the channel's pace, re-checking each address, so pause and cancel act within a batch.
  The channel's `broadcast_smtp_*` account is used when set, else its support account; confirmation,
  welcome, test and broadcast mail never use the server's own `YUVA_SMTP_*`. Test sends go to members
  only. `YUVA_BROADCAST_DAILY_RECIPIENTS` (0 = no limit) caps recipients per workspace and UTC day; a send
  that reaches it continues the next day.
  Implementation: each recipient's mail is rendered from the frozen document with their merge values
  when it goes out (no stored HTML template). A batch is 50 mails or 5 seconds, whichever ends first, and
  the broadcast's state is read again before every mail; the send job marks a broadcast `sent` itself once
  nothing is queued (there is no separate finish job), and a sweep every 5 minutes queues the job of any
  broadcast that should be moving. Recipients are checked again before their mail (still subscribed, not
  suppressed, not opted out) and skipped with the reason otherwise. Temporary SMTP failures are retried
  after 5 minutes, 30 minutes, 2 hours and 6 hours, then the delivery fails; `5.1.x` and `5.2.1` refusals
  are bounces; 20 refusals in a row, a refused account or TLS, or a server unreachable for 30 minutes
  pause the broadcast (`sender_error`, the SMTP answer in `state_detail`). Test sends: 20 mails an hour
  per workspace; their unsubscribe link names no list and its page says it was a test. Progress reaches
  `/v1/realtime` as `broadcast.progress` (counts, not stored, at most every 5 seconds).
- **Headers.** `Message-ID` `<delivery token.random@sending domain>` (the conversation shape, so loop
  protection is unchanged), `List-Unsubscribe`, `List-Unsubscribe-Post`, `List-Id`, `Feedback-ID`
  (`broadcast:list:workspace:yuvabroadcast`), `X-Auto-Response-Suppress: OOF, AutoReply`; no
  `Auto-Submitted`.
- **Bounces and complaints.** SES (SNS), delivery reports to the channel, SMTP 5xx at send time and ARF
  feedback reports find the delivery by Message-ID. A hard bounce suppresses the address for all mail; a
  complaint about a broadcast suppresses broadcasts only, unsubscribes every list of the workspace and
  writes an opt-out. A broadcast pauses itself at 5% bounces (after 200) or 0.3% complaints (after 1,000),
  counted since it started or was last resumed; a workspace whose 30-day complaint rate reaches 0.3% (over
  at least 1,000 mails) or bounce rate 8% (over at least 2,000) is suspended from sending and its running
  broadcasts pause. ARF reports count only from a sender whose trusted DMARC result is `pass`.
- **Deliverability.** A check (six an hour per channel) sends a probe through the broadcast account to
  the channel's own address; ingress knows it by its token (in the Message-ID or `X-Yuva-Probe`), never
  makes it a conversation, and reads only the trusted `Authentication-Results` (SPF, each DKIM signature
  with its domain and alignment, DMARC, whether the aligned signature's `h=` covers `List-Unsubscribe` and
  `List-Unsubscribe-Post`) plus DNS (SPF, DMARC policy, MX of the reply domain, the Yuva DKIM record).
  Each channel keeps its latest result; a probe not back after 10 minutes reads as `timeout`, and the
  panel polls. The review warns when the channel was never checked, the check did not finish or cannot be
  verified, DMARC is missing, DKIM is not aligned with `From` or the list headers are unsigned, and blocks
  when no DKIM signature passed. A channel may have its own DKIM key (RSA 2048, private key sealed under
  the master key, only the DNS record shown; enabling needs that record in DNS, a new sending domain turns
  it off, deleting allows a new key). Yuva then signs broadcasts, tests, probes, confirmation and welcome
  mail over the list headers; support replies keep their provider's signature. The signature covers
  `From`, `To`, `Subject`, `Reply-To`, `MIME-Version`, `Content-Type` and the list headers (absent ones
  listed too) but not `Message-ID`, `Feedback-ID` or `Date`, which relays replace, and address headers
  are written in the minimal form normalizing relays leave alone (a display name quoted only when RFC
  5322 needs it, a bare address without angle brackets). Amazon SES channels never get Yuva DKIM
  (enabling it is refused, an older enabled key is not used): SES rewrites `From`, `To`, `Reply-To` and
  `Date` after any signature added before it and signs every mail with the customer's domain itself
  (Easy DKIM), list headers included.
- **Replies.** A reply to a broadcast finds its delivery by the token in `In-Reply-To`/`References` (or
  by the relay's Message-ID stored for the delivery) and
  opens (or continues) a conversation with that contact in the inbox with `broadcast_id` and a
  `broadcast_reply` marker; someone else's reply opens their own conversation; automatic replies are only
  counted (when they name no conversation). Notifications about replies to one broadcast are grouped per
  member every 10 minutes (one push "N new replies"; no e-mail fallback for them).
- **Measuring.** Per broadcast: recipients, sent, failed, bounced, complained, unsubscribed, replied,
  automatic replies, skipped by reason, and unique clicks only when the broadcast turns click tracking on
  (links go through `/c/{token}`). There is no open pixel. Tracked links are the web links of the HTML part
  that are the same for every recipient (no merge field), found when the broadcast is scheduled; a `HEAD`
  request and a third link of one mail opened within 2 seconds are not counted.
- **Hosted servers.** `YUVA_BROADCASTS=on|approval|off` (unset: `approval` with `YUVA_SIGNUP=open`, else
  `on`). In `approval` mode a workspace asks the operator, who answers with
  `yuva workspace broadcasts approve|deny|suspend|limit|show`; approved workspaces warm up from 500
  recipients a day, doubling after each clean day (at least 100 sent, bounces under 3%, complaints under
  0.1%) and losing a doubling after a bad one. Only the operator lifts a suspension (`approve`). The
  operator digest reports approval requests, suspensions and auto-pauses without content.
  `YUVA_BROADCASTS=off` refuses broadcast writes (`403 broadcasts_disabled`); lists and their forms keep
  working. `YUVA_MAIL_LINK_URL` puts `/m/`, `/s/`, `/c/` and `/i/` on a separate domain, which answers only
  those routes.

  ```sh
  yuva workspace broadcasts approve|deny|suspend --workspace <id|name> [--reason <text>]
  yuva workspace broadcasts limit --workspace <id|name> --daily <n>   # 0 removes the limit
  yuva workspace broadcasts show --workspace <id|name>                # status, limits, 30-day rates, last 10
  ```
- **Scopes, events, MCP.** Scopes `lists:read`, `lists:write`, `broadcasts:read`, `broadcasts:write`,
  `broadcasts:send`; keys and OAuth tokens schedule only while the workspace allows it
  (`broadcasts_by_keys`, off by default). Events and webhook types `subscription.created`,
  `subscription.updated` and `broadcast.updated` (imports send none per row). MCP tools draft designs and
  broadcasts, preview, send tests to the caller and read reports; `schedule_broadcast` needs the audience
  count it was shown.
- **Law.** Consent records and their export serve GDPR Art. 7 and 15 and KVKK; objections are kept after
  erasure; the welcome receipt and the footer follow Türkiye's Regulation on Commercial Communication
  (Art. 7–10); links never expire (CAN-SPAM 30 days, CASL 60). Senders in Türkiye register their consents
  in İYS themselves; a list's consent export with `format=iys` gives İYS's consent fields (`recipient`,
  `type` `EPOSTA`, `source` `HS_WEB` for forms and the preference page and `HS_EORTAM` otherwise,
  `status` `ONAY` or `RET`, `consentDate` in Türkiye's time, `recipientType` `BIREYSEL`), leaving
  pending subscriptions out.
- **Template gallery.** Six starters ship in `templates/email/` (MIT, listed in `LICENSING.md`; each a
  design JSON per language, assets and generated previews): newsletter, product update, announcement,
  welcome, event, digest. The panel shows them when creating a design; the website shows them at
  `/templates` (en, tr, de) with "Use this template". New templates come by pull request
  (`templates/email/README.md`); there is no paid store.

### Contacts directory

The panel lists contacts newest first or most recently active first (`sort=last_seen`), filtered
by `kind` (`known`: an e-mail address or an external id; `visitor`: neither) and by whether they
have an open conversation, with search over names, addresses and external ids.

- `contacts.last_active_at` is the contact's last message (any channel), rating or use of a widget
  or app session. It is kept by the queries that write those (message insert, session start,
  session use at most once per throttle period, realtime close), carried over on a merge, and
  indexed with `created_at` as fallback for the sort. It is the same for every member; an agent may
  see a contact rise in the order because of a conversation in an inbox they cannot see, but never
  that conversation. Pages follow it by keyset, as conversations follow `last_activity_at`; it
  only grows, so a contact active while someone pages moves above the pages already read and is
  neither repeated nor on the later pages, which the API reference says.
- `activity` (conversation counts, open ones, the last message time) is added only to
  `GET /v1/contacts` and `GET /v1/contacts/{id}`, by one grouped query per page over the
  conversations the caller can see, spam left out; events and webhooks carry the contact without it.
- `GET /v1/contacts/{id}/summary` counts first replies with their median time and the latest
  rating of each conversation, over the same conversations, computed on request.
- A contact's conversations are `GET /v1/conversations?contact_id=` with the usual filters.
- Contact notes (`/v1/contacts/{id}/notes`) are team-only notes about the person, not tied to a
  conversation: a `contact_notes` row with the author (a member or an API key) and the text.
  Whoever can see the contact reads and adds them, under the same rule as the contact itself; only
  the author or an owner or admin deletes one. Contacts never see them, no event or webhook
  carries them, a merge moves them to the remaining contact and deleting the contact deletes them.

### Moving to Yuva (M14)

A team brings its history from the service it uses today and then switches its mail over, without
losing a message and without changing anything at the source. Decisions:

- **Sources.** Every mailbox is read over IMAP. Presets for Gmail and Google Workspace, Yandex,
  iCloud, Zoho, Yahoo, Fastmail, GMX and any server connect with an app password or password.
  Outlook.com and Microsoft 365 accept no passwords: "Sign in with Microsoft" obtains only an IMAP
  token (`https://outlook.office.com/IMAP.AccessAsUser.All` and `offline_access`, sent with
  `AUTHENTICATE XOAUTH2`) through the operator's Entra app (`YUVA_MICROSOFT_*`) or, without one, an
  app registration the team created and entered on the connect page, as for help desks; the
  authority is `consumers` for Outlook.com addresses, `organizations` for domains whose MX is
  Exchange Online, else `common`. The app is not publisher-verified, so Outlook.com and tenants that
  allow user consent connect, and other tenants use export files or an admin's consent (the panel
  shows the admin-consent link). Export
  files (Google Takeout mbox, mbox, .eml or Maildir in a zip; no PST, for want of a permissively
  licensed reader) need no access to the account and are the fast way for very large Gmail
  mailboxes: Takeout first, then IMAP for newer mail and the cutover. Help desks: Zendesk with OAuth
  `read`, Help Scout; each team creates the OAuth client in its own help desk with Yuva's redirect URL
  and enters its id and secret on the panel's connect page (there is no operator-wide client), and a
  help-desk migration goes through the same cutover as a mailbox. Detection from the address and the domain's MX picks the preset; autoconfig is
  read only from the team's own domain. Provider mail APIs and other sign-ins were considered and
  rejected for one IMAP path.
- **Read-only, enforced three times.** No credential a mailbox accepts is read-only (Microsoft's
  consent screen says "Read and write access to mailboxes via IMAP"), so Yuva enforces it. IMAP goes
  through `internal/migrate/imapread`, the only package that may import the IMAP client: it opens
  mailboxes with `EXAMINE` and fetches with `BODY.PEEK`, and a wire guard under the connection lets
  through only `CAPABILITY`, `NOOP`, `LOGOUT`, `STARTTLS`, `LOGIN`, `AUTHENTICATE`, `LIST`, `STATUS`,
  `EXAMINE`, `UNSELECT`, `SEARCH` and `FETCH` of non-marking items; anything else closes the
  connection before it is sent. Help-desk sources use a transport that refuses every method but `GET`
  (the token exchange and refresh `POST` to the provider's token endpoint aside) and every host not on
  the source's list; attachments the provider keeps on other public hosts are fetched by a separate
  client without the token. A sign-in must grant exactly the requested scopes. Tests
  fail the build if a write command or method could be sent, and an integration test proves a Dovecot
  mailbox byte-identical after a migration.
- **Two phases.** A scan reads headers only (the whole message only when it has no Message-ID, to take
  the same hash the live ingest stores), joins threads (Takeout and help-desk thread ids, else
  `References`/`In-Reply-To` with the live bounds, never by subject) and gives an exact dry run:
  counts per folder, what is skipped and why, contacts, labels, storage and time. The copy then
  imports whole threads newest first. A thread is a conversation of the inbox's e-mail channel whose
  address is the mailbox address; mail from a team address (the mailbox, aliases, members) is an
  outgoing message by the mapped member or, without one, by its original name and address
  (`imported_author`). Spam, trash, drafts and chats are never read; threads of only automatic or
  only internal mail are skipped by default. Labels and user folders become labels; a status rule
  closes old threads and leaves recent ones open or pending.
- **Imported history is quiet.** Imported rows keep their original times and carry `imported_at`
  (and `migration_id` on conversations and created contacts). They write no `conversation.*` or
  `message.*` events, so no webhook, push, notification, auto-reply, rating request or continuity
  mail follows; they are never unread, are left out of `/v1/stats` and the bounce lookups, count in
  `imported_*` usage columns instead of the billed ones, and the retention clock starts at the import.
  Parsing, quote stripping, sanitising and attachments are the live ingest code's. "Remove imported
  history" deletes the conversations nobody wrote in after the import.
- **Engine.** River jobs on a queue of their own (`migrations`, `YUVA_MIGRATION_WORKERS` per process,
  default 2) in 30-second slices, one job per migration (unique options and a lease), a watchdog every
  minute that resumes a migration whose process died. Checkpoints per folder (`UIDVALIDITY` and UIDs,
  file offsets, help-desk cursors); every item has a source id and a dedupe key (Message-ID, else the
  hash the live ingest uses), its rows get ids derived from both, and it is imported in one
  transaction, so a retry never duplicates. A message the channel already has, in either direction,
  is not imported again, which also lets a Takeout import and a later IMAP migration of the same
  mailbox meet without duplicates; a thread whose earlier messages the channel already has continues
  in that conversation, in every migration. Each source has a budget below its published limit (Gmail IMAP
  1,500 MB a day, other servers one connection and 4 MB/s, help desks half their rate limit), kept on
  the migration row; limit answers and `Retry-After` are honoured. Microsoft tokens are refreshed
  before a connection and resealed. Pause stops after the batch in progress (at most 50 messages);
  any change to a ready migration's options sends it back for a new dry run. Memory does not grow
  with the mailbox: the scan reads headers a page at a time, the plan joins threads in the database
  (`migration_plan_nodes`, union by size, a page of messages per step) and plans a batch of whole
  threads at a time, and the copy holds one batch and keeps a folder open across batches instead of
  opening it again for each. Progress reaches owners
  and admins as `migration.updated` (stored) and `migration.progress` (a signal, at most every two
  seconds); never webhooks.
- **Cutover.** After the copy, sync runs every two minutes. The migration goes live when the copy ends:
  mail received before that is history (mail that arrives during the copy is imported quietly); mail
  after it is new mail and takes the live path (events, notifications), without the greeting. Every
  e-mail channel has a forwarding address `<token>@YUVA_FORWARDING_DOMAIN` that ingress resolves by
  token (rotatable); the forwarding domain may also carry ordinary channel addresses, so a local part
  that is no channel's token falls through to the exact, `local+tag` and catch-all lookup. The team forwards the mailbox there (or routes it, or moves the MX). While
  waiting, Google's forwarding confirmation to that address is shown on the migration (and sent as the
  `migration.forwarding_confirmation` signal) instead of opening a conversation. A probe mail with an
  HMAC header (context `migration-probe:`) is recognised at ingress before the own-sender drop and
  verifies delivery. After verification and a quiet period (24 hours by default) in which sync found
  nothing ingress did not also deliver, the migration completes on its own; an owner may finish it
  earlier once delivery is verified, or without verification on purpose (`force`). The channel dedupe
  keeps one copy of mail that came both ways. "Remove imported history" also deletes the contacts the
  migration created that are left without a conversation; labels stay.
- **Credentials.** Sealed under the master key (context `migration:`+workspace+migration) in
  `migration_credentials`, never in job arguments, events, logs or answers; deleted in the transaction
  that completes or cancels the migration, after `YUVA_MIGRATION_SYNC_MAX_DAYS` (30) of sync, and with
  the workspace. Tokens are not revoked at the provider (that would take a method other than `GET`);
  the completion screen and the guides say where to remove the OAuth client, the app or the app
  password. Sign-in uses PKCE, refuses a grant whose scopes
  differ from the requested set and, for Microsoft, a sign-in whose account cannot open the mailbox:
  when the token's claims name another address or none (a Microsoft 365 UPN is often not the SMTP
  address), the mailbox decides by accepting the token in `AUTHENTICATE XOAUTH2` with its address as
  the user, and only its refusal is `account_mismatch`; the audit row names the account that signed in.
  Assistants never handle a password: MCP tools return a panel link where the person connects.
- **Who.** Owners, admins, and keys without an inbox limit or owner/admin tokens with
  `migrations:manage`. Every action is in `migration_audit` (actor, client), kept
  `YUVA_MIGRATION_AUDIT_DAYS` (365). Connection tests are limited (10 per migration, 30 per workspace an
  hour) so the server is no password-guessing tool. Hosted limits: `YUVA_MIGRATION_SOURCES`,
  `YUVA_MIGRATION_MAX_ACTIVE`, `YUVA_MIGRATION_MAX_BYTES`, `YUVA_MIGRATION_UPLOAD_MAX_BYTES`;
  `422 hosted_limit` names the one reached in the problem's `limit`.
- **Panel and MCP.** A wizard in the setup layout (Source, Connect, Choose, Map, Review, Copy, Switch,
  Done) with live progress, provider guides inline and in the docs (en, tr, de), Settings → Move to
  Yuva for the list, a banner in the target inbox while it runs. The banner reads the inbox's
  `import` (address, state, percent), which everyone who sees the inbox gets; `inbox.updated` comes
  when it appears, changes state or ends, and the percent between those is read by refetching the
  inbox. MCP tools for every operation
  (`create_migration` returns the connect link) and the `move_to_yuva` prompt. Like the wizard's Map
  step, `create_migration` given an inbox and no channel uses the inbox's e-mail channel for the
  mailbox address or creates it (with `inboxes:manage`) without an SMTP account, and its answer
  tells the person to set SMTP in the panel; no tool takes that password.

### Analytics (M15)

A workspace measures its own websites and apps: pageviews, screens, events, goals, funnels, revenue,
sources, locations and devices, in the panel, the API and MCP. Plan in the M15 umbrella issue; it ships
in six steps (M15.1–M15.6 in the roadmap). Decisions:

- **Properties.** `analytics_properties` belong to a workspace and optionally to an inbox (who sees them,
  what support data they join): name, a public key (`yuva_ak_…`, unique on the server, rotatable with
  24 hours' overlap), 1–50 domains (`*.example.com` allowed), platforms (`web`, `ios`, `android`), time
  zone (the property's day), currency, `collection` (`anonymous` or `consent`), `consent_features`
  (`persistent`, `identify`; none by default), `audience` (`general` or `children`; children's
  properties keep the country only, general ones keep the city), exclusions, path rules, up to 10
  tracked props, `retention_months` (25). Owners and admins manage them; agents read the properties of
  their inboxes.
- **Default mode: nothing on the device, no IP stored.** The address is shortened (IPv4 /24, IPv6 /48)
  in the first function that sees it; GeoIP and the hash use only that. A visitor is
  `HMAC-SHA256(salt, property ‖ short IP ‖ user agent)` truncated to 64 bits; apps hash a random key held
  in memory for the life of the process instead of the address. Salts are random, one per property and
  local day in `analytics_salts`, deleted 2 hours after the day ends (`YUVA_ANALYTICS_SALT_GRACE`) and
  never derived from a persistent secret. So visitors are counted per day; a visitor over a range is the
  sum of daily visitors. A visit is a visitor's events without a 30-minute gap within the day.
  Processes agree on a new visit through `analytics_visit_claims`: the first claim wins for 5 minutes
  (`ClaimWindow`, at most the visit gap); claims commit without waiting for the disk sync (a crash at
  worst splits a visit) and are deleted with the expired salts. A visit's description (source, place,
  device) is the one its first event carries, even when another process wrote the visit first.
- **Law.** Built to meet the CNIL audience-measurement exemption (purpose, no cross-checking, one
  publisher, shortened IP, no tracker), PECR Schedule A1 (statistics with information and an
  objection), the KVKK cookie guide's Criterion B for first-party analytics, CCPA (no sale or sharing),
  LGPD and COPPA (children's properties keep country only, no props, no consent features). The EDPB
  (Guidelines 2/2023) reads ePrivacy Art. 5(3) as covering any script that makes a browser send data, so
  the docs say where a banner is still needed and `collection: consent` sends nothing until
  `yuva.consent({analytics: true})`. DNT and GPC stop collection unconditionally; `yuva.optOut()` stores
  the objection, the only default-mode write to the device. Raw rows are treated as pseudonymous
  personal data until the raw window ends; rollups are anonymous.
- **Consent features.** Only for visitors who consented, through the host's own consent tool:
  `persistent` keeps a random id on the device for at most 13 months (returning visitors, retention,
  cross-day funnels), hashed with a per-property key; `identify` sends the inbox's identity JWT, checked
  like `/client/v1/session`, and links the visitor's visits to the contact, shown on the contact page.
  Without consent no visitor is ever joined to a contact or conversation.
- **Collection.** `POST /client/v1/analytics/{key}` (one event, or up to 100 for apps), a plain handler
  outside the strict server; `text/plain` JSON so browsers send no preflight; its CORS branch matches the
  `Origin` host against the property's domains; no cookie, no session, request logs without addresses.
  It takes the visitor's address from the `X-Forwarded-For` entry that a site's own proxy for the
  script and endpoint puts in front of the connecting address (a valid public address only, otherwise
  the connecting address); a spoofed one only skews that property's own counts, and the per-property
  limit still applies.
  Host backends send `POST /v1/analytics/properties/{id}/events` with `analytics:write`, forwarding the
  visitor's IP and user agent (never stored) or their own visitor key. Ingest scrubs e-mail-shaped
  values, keeps only UTM keys of the query string, parses the user agent (`medama-io/go-useragent`), looks
  up GeoIP (DB-IP Lite downloaded monthly by default under CC BY 4.0 with attribution, any MMDB file, or
  off; GeoLite2 only from the operator's own account), drops bots (crawler list, `webdriver`, header and
  volume rules, referrer spam), and answers `202` before writing.
- **Storage.** Events wait in a per-process buffer (`YUVA_ANALYTICS_BUFFER`, full → `503`) and are flushed
  every second by `COPY` into `analytics_events` with one upsert per visit into `analytics_visits`; both
  are partitioned by the property's local `day`, created ahead and dropped after
  `YUVA_ANALYTICS_RAW_DAYS` (90) by the `analytics_partitions` job. Finished days are folded into rollups
  (`analytics_daily` per dimension and value, top 1,000 values plus `(other)`; hourly, goals, props,
  support) by `analytics_rollup` jobs on the `analytics` River queue; late events and new goals mark days
  dirty. Queries add rollups for built days and raw rows for open ones; filter combinations the rollups
  cannot answer are served from raw rows inside the raw window only. The flusher also keeps each
  visit's numbers per path (`analytics_visit_pages`), which page breakdowns and page filters read instead
  of the events. Every five minutes an open day is folded up to two minutes ago (`analytics_partial_*`)
  for the visitors whose every visit begun by then had ended half an hour earlier, so a query adds only
  the other visitors' raw rows and takes away what the fold already holds of theirs, and a change to a
  folded visit (a delayed write, an engagement joining an old visit, a bot found later, a deletion)
  moves the day's sequence so the fold stops counting until the next one. No Postgres extension. The query
  layer sits behind a `Store` interface so a column store can be added later without a contract change;
  Postgres is the only store in M15 and fits up to about 100M events a month per property.
- **Events, goals, funnels (M15.2).** Custom and built-in events (`outbound_click` and `file_download`
  with an http(s) `url`, `not_found`, `chat_open`) carry up to 30 props (keys 1–300 characters, none
  starting with `$` or `yuva_`; text up to 2,000 characters; e-mail-shaped values dropped and counted as
  scrubbed; none on children's properties but a built-in event's `url`) and revenue in the currency's
  minor units, never converted. Goals are a page view, an event with an optional prop equal to a value,
  or a scroll depth on matching pages; there is no separate revenue goal, because an event goal reports
  the revenue its events carry, per currency. A change to a goal or to `tracked_props` moves the
  property's `goals_version`; a rolled-up day records the version its goal and prop rows were built with,
  so a goal is read from rollups only on days built at or after its own version and from raw rows
  elsewhere in the raw window, and the rollup scan rebuilds those days' goal and prop rows without
  touching traffic rollups. Prop breakdowns read raw rows for any key inside the raw window and
  `tracked_props` rollups before it. Funnels follow visits within a day over the raw window (90 days, or
  31 when the property counted more than 10M events in the last 30), answers cached 10 minutes.
  Segments and annotations need `analytics:read` and are edited by their author, owners, admins and keys
  without an inbox limit; Yuva's own annotations (`key_rotated`, `goal_created`) cannot be edited.
- **Apps and servers (M15.3).** An app screen is a page view of `app://<app id>/<screen>`; the app id is
  the visit's hostname and is not checked against `domains`. App requests have no `Origin`, use the batch
  form `{k, a, e}` with one process key per request (a fresh UUID per launch, kept in memory only) and
  need `a.platform` in the property's `platforms`; they skip the browser-header bot rules and the
  user-agent parser, the volume rules still apply. A late event (at most 72 hours old, `t` from apps,
  `timestamp` from servers) is stored on its own day at its own time, hashed under its day's salt while
  that salt exists and under the arrival day's otherwise; visits, claims and bot counts use the event's
  time. `POST /v1/analytics/properties/{id}/events` (keys with `analytics:write`) takes server events that
  forward `ip`, `user_agent` and the client hints, hashed exactly like the browser's request so they
  join its visit, or a `visitor_key` hashed in a scope of its own; a visit a server event starts has
  platform `server`. `dnt`, `gpc` or a missing `consent` mean the event is not stored; invalid events
  come back in `errors`, and only a body or auth problem refuses the request. `sdk/go/analytics` drops
  DNT and GPC requests before sending and retries 429, 5xx and network errors with backoff that honours
  `Retry-After`.
- **Sharing and export (M15.5).** `analytics_shares` keep a SHA-256 of the link token (`yuva_sh_…`, shown
  once) and an argon2id password hash (m=19 MiB, t=2, p=1); unlocking gives a 12-hour `yuva_sa_` token
  HMAC-signed over the link id, its expiry and the password hash, so a new password ends every token;
  revoke and expiry stop a link at once. 50 links per property; 120 requests a minute per address, 10
  unlocks a minute per address and link, 30 in all. A link lists its views (`overview`, `realtime`,
  `pages`, `sources`, `locations`, `devices`, `events`, `goals`): totals and time grains come with every
  view, other dimensions, filters and event or goal metrics only with theirs, saved segments never; its
  queries share the workspace's query slots and cache. The server serves the panel at `/share/{token}`
  with `frame-ancestors` set to the link's embed origins, otherwise `'none'` and `X-Frame-Options: DENY`,
  without the panel's analytics script and with the token left out of request logs; the share API is
  `/client/v1/analytics-shares/{token}`. Exports are `view` (one query), `daily` (a row per day, optionally
  by one dimension) and `raw` (NDJSON, at most 31 days inside the raw window, bots left out, visits
  renumbered per file, no visitor hash, salt, stored id, contact or referrer path), written by
  `analytics_export` jobs to storage (1 GiB cap, 3 at a time per workspace) and deleted 7 days after they
  finish. `view` and `daily` need `analytics:read`; `raw` needs `analytics:manage` and an owner, admin or
  key without an inbox limit, because raw rows are pseudonymous. CSV cells starting with `=`, `+`, `-`,
  `@`, tab or CR get a leading `'`.
- **Support, alerts and reports (M15.4).** `analytics_daily_support` keeps conversations, page feedback,
  ratings and discussion threads of the linked inbox by (host, path, day), paths after the property's
  path rules, built in the same job as the day's traffic rollup; a day is read from it only while
  `analytics_support_days` names the property's current inbox, other days from the inbox's own tables.
  Conversations and discussions count on their local day, page ratings on the UTC day they were given.
  `support_rate` is conversations per 1,000 page views, joined to traffic by path (hostname as its own
  dimension or filter), never through a visitor; `<yuva-chat>` sends the page URL without query or
  fragment and the title with a new conversation, kept only for a chat channel that allows the page's
  origin and dropped silently otherwise, so the page never refuses a conversation. Alerts: spike and drop compare the last two finished
  hours with the same hours averaged over the 7 days before (3× or more, half or less, a baseline of at
  least 20 views); goal drop is yesterday at half or less of its 7-day average with at least 10 a day;
  support load is yesterday's rate at twice the 28-day rate with at least 5 conversations; a new referrer
  has 10 or more visits yesterday and none in the 28 days before. An alert is unique by (property, kind,
  key) and fires once a day (a new referrer once per host), writing an annotation, the `analytics.alert`
  stored event and webhook, and a mail to subscribers. Weekly (Monday to Sunday) and monthly reports go
  out at 08:00 property time once per member and period, in the member's language, without images or
  link tracking; subscriptions are per member session. Broadcast links get `utm_source=newsletter`,
  `utm_medium=email` and the campaign (on by default; links that carry a `utm_` parameter and the
  server's own links are left alone; broadcasts already past draft at upgrade keep their links), and the
  report shows the campaign's visits to callers with `analytics:read`. `ListAnalyticsReportWorkspaceIDs`
  reads `analytics_subscriptions` across workspaces to find which have work, the same maintenance
  exception as `ListAnalyticsWorkspaceIDs`; every query after it filters by workspace. MCP adds the
  analytics tools (read: properties, overview, query, realtime, support report, goals and funnels,
  funnel report, snippet, notes; manage: create goal, create property), the `analytics_review` and
  `setup_analytics` prompts and the `yuva://property/{id}` resource; visitor-sent values are
  `customer_content`. Insights compare in percent only from a baseline of at least 10; a total with none
  before is `metric_new`, a row `new_value`; each insight appears once per answer.
- **Consent features (M15.6).** A consented `persistent` id (kept on the device for 13 months, made only
  without DNT or GPC) is hashed with the property's sealed random `visitor_key` and is that visitor on
  every day (`persistent`, `visitor_type`); ids first seen more than 13 months ago are forgotten by the
  hourly `analytics_cohorts` job, which also rebuilds `analytics_cohorts` (days to 30 periods, weeks and
  months to 12). `identify` needs `persistent` and a linked inbox; its token is checked exactly as for
  `/client/v1/session` (single-use `jti`) and links the id and its raw visits to the contact; forged,
  expired or reused tokens are ignored. `yuva_consent` events carry identification and withdrawal and
  are never stored, counted or billed; a withdrawal, honoured under GPC and DNT, deletes the id's link
  and cohort days, and `DELETE …/visitors/{persistentId}` erases everything stored under the id. A
  consented visitor counts once over a range's totals and per week or month bucket (from cohort days
  without filters, from raw rows inside the raw window with filters); default-mode traffic stays a sum
  of days. Funnels follow consented visitors across visits and days in `consented_visitors`. Deleting a
  contact deletes its linked visits, merging moves them, and contacts left from an import that have
  linked visits are kept. Children's properties refuse consent features.
  The script tag keeps `/yuva-analytics.js` at 1.5 KB without them; script-tag sites that use consent
  features load `/yuva-analytics-consent.js` (≤ 2 KB) instead, served and routed like the first.
  Every client treats consent to analytics without `persistent: true` as a withdrawal of a stored id, so
  hosts pass `persistent: true` on each call where the visitor agreed to it; identify and withdrawal
  are sent once and never queued.
- **SDKs.** `/yuva-analytics.js` (≤ 1.5 KB gzip, served like `/yuva.js`, off with `YUVA_WIDGET=off`) with
  automatic and SPA pageviews, engagement, opt-in outbound, file and chat-open events; `useyuva/analytics`,
  React and Nuxt helpers; Swift product `YuvaAnalytics` (no `YuvaKit` dependency, SwiftUI `.yuvaScreen`,
  privacy manifest with `CA92.1`, no IDFA) and Kotlin `yuva-analytics` (Compose `TrackScreen`, no
  advertising id); `sdk/go/analytics` for server events. Apps queue offline up to 72 hours.
- **Support, in aggregate.** A property linked to an inbox gets conversations, page feedback, ratings and
  discussion threads per page and day next to its views (`<yuva-chat>` records the page a conversation
  starts on); broadcasts add UTM parameters by default (a broadcast can turn them off) and show the
  visits of their campaign. Alerts (spike, drop,
  goal drop, support load, new referrer), insights and weekly or monthly e-mail reports are fixed rules;
  Yuva runs no model.
- **Panel, API, MCP.** An Analytics rail section per property (overview, realtime, pages, sources,
  locations, devices, events, goals, funnels, retention for consented traffic, support, settings),
  filters, comparisons, segments, annotations, read-only share links (password, expiry, embeddable on
  listed origins), CSV and NDJSON exports. One query operation (`POST
  /v1/analytics/properties/{id}/query`) serves the panel, the API and the MCP tools. Scopes
  `analytics:read`, `analytics:write`, `analytics:manage`; like `lists:*`, they go to keys created
  after M15 without named scopes, never to existing keys. Analytics writes no stored event per page view;
  `analytics.alert` is a stored event and webhook type.
- **Hosting.** `usage_counters.analytics_events`; `YUVA_ANALYTICS_MONTHLY_EVENTS` (per workspace;
  unset: 1,000,000 with `YUVA_SIGNUP=open`, else no limit; operator overrides with `yuva workspace
  analytics-limit`), per-property and per-address ingest limits, 4 concurrent queries per workspace;
  `YUVA_ANALYTICS=off` removes the feature. The script and endpoint keep their names
  (`/yuva-analytics.js`, `/client/v1/analytics/`); the docs teach serving both from the site's own domain
  for visitors whose blockers stop them.
- **Counting the panel.** `YUVA_PANEL_ANALYTICS_KEY` (unset by default) names a web property on the same
  server; the server puts the script, in manual mode, ahead of the panel's own scripts in the panel's
  page, and the panel sends one page view per screen with every UUID path segment replaced by `:id`, so
  no record id of any workspace leaves the browser. A load balancer's sticky cookie must skip the
  script and `/client/v1/analytics/`, or the promise of nothing on the device breaks.

### Storage

Attachments and raw e-mails in S3-compatible storage (R2, S3, MinIO) or on local disk for a
single-node install: `YUVA_STORAGE=local|s3`, with `YUVA_STORAGE_DIR` for local disk and
`YUVA_S3_ENDPOINT`, `YUVA_S3_REGION`, `YUVA_S3_BUCKET`, `YUVA_S3_ACCESS_KEY_ID`,
`YUVA_S3_SECRET_ACCESS_KEY`, `YUVA_S3_PATH_STYLE` for S3. Objects are never public (the exception is e-mail images, served at `/i/{token}`, see Broadcasts);
members and API
keys download them through `/v1/attachments/{id}`, which checks inbox access. The size limit
(`YUVA_ATTACHMENT_MAX_BYTES`, 25 MiB by default) and the content-type allowlist
(`YUVA_ATTACHMENT_TYPES`, comma-separated, `image/*` style wildcards allowed) are server-wide for
now; per-channel limits can narrow them later.

## Privacy and data

- Retention per workspace: `workspaces.retention_days`, set by an owner or an API key with
  `workspace:manage` (`PATCH /v1/workspace`). Admins can create such a key; that adds no destructive
  power they lack, since they can already delete inboxes with their conversations. `null` keeps
  everything. An hourly River job deletes closed conversations whose last change and
  last activity are older than that, with their messages, attachments and stored files, and
  removes raw e-mails older than that from storage (the parsed message stays).
- Imported history (see "Moving to Yuva") is the workspace's data like any conversation: contact
  export and deletion, retention (counted from the import) and workspace deletion cover it. Source
  credentials are deleted when the migration ends.
- Workspace deletion: an owner deletes the workspace in the panel or with `DELETE /v1/workspace`
  (the body repeats its name), an operator with `yuva workspace delete --workspace <id|name> --yes`.
  One transaction sets `workspaces.deleted_at`, removes the workspace's webhook endpoints and the
  push subscriptions of members who have no other workspace, and queues a River job; the request
  answers `202`. From then on every lookup a request starts from ignores the workspace (member
  sessions, API keys, contact sessions, chat and app keys, CORS origins, e-mail recipients, SES
  reports, invites), so it is unusable at once; open realtime connections close at their next
  heartbeat check, and jobs queued for it earlier (e-mail, chat continuity, notifications, push)
  do nothing. The job deletes the conversations 200 at a time with their messages, attachments
  and stored files, then the contacts, then the workspace row, which takes everything else with
  it. Each run works for about 30 seconds, records its progress in the job's output and the log,
  and snoozes the job until the next run, so a large workspace never hits a timeout; the hourly
  retention job queues it again for a deleted workspace whose job was lost. People keep their
  account: someone left without a workspace can still sign in and sees that they belong to none.
- Account deletion: a person deletes their account in the panel or with `DELETE /v1/me` (the body
  repeats their e-mail address), an operator with `yuva person delete --email <address> --yes`.
  It is refused (`409 last_owner`) while the person is the only owner of a workspace. Otherwise
  the person goes with their memberships, sessions, passkeys, push subscriptions and sign-in
  codes; messages they wrote stay without an author, shown as a deleted member. As a reader they
  also lose their reader tokens, notifications, votes and follows; their discussion posts stay
  without an author, shown as a deleted reader; in each workspace the contact linked to them is
  deleted when it has no conversations and no external ids (it existed only for their posts), else
  only unlinked, so the workspace keeps its own record of a customer.
- Contact deletion and export through the API, for GDPR and KVKK requests. A host backend deletes
  a user who deleted their account with `DELETE /v1/contacts/by-external-id?inbox_id=&external_id=`
  (an API key or an owner or admin): the contact goes with all their conversations, messages,
  attachments, sessions and addresses in every inbox, the stored files are removed, and
  `contact.deleted` tells the other endpoints which external ids are gone.
- Analytics keeps no IP address and puts nothing on a visitor's device by default (see "Analytics"). Raw
  visits and events are kept `YUVA_ANALYTICS_RAW_DAYS` (90), rollups the property's `retention_months`
  (25); salts are deleted two hours after their day. Members delete a property's data for a range or in
  full; a property, a consented visitor, a contact with linked visits and a deleted workspace take their
  analytics rows with them. Workspace `retention_days` does not apply to analytics.
- Nothing is sent to third parties except what a channel is configured to send (SMTP, webhooks);
  the monthly GeoIP download from DB-IP fetches a file and sends nothing about anyone.

## Hosting for others later

- `workspace_id` on every row; every store query is scoped by it.
  Exceptions:
  - `workspaces` itself, whose `id` is the workspace.
  - River's `river_*` queue tables, which the library owns; job arguments carry the
    `workspace_id` instead.
  - Inbound e-mail arrives before any workspace is known: `/ingress/email` finds its channel by
    the recipient address (`email_channels.address`, unique across the server), and
    `/ingress/ses` finds the message a bounce refers to by our Message-ID
    (`message_emails.header_message_id` of outbound mail). Everything after that lookup is scoped
    by the workspace it returned. `/ingress/ses` also finds a broadcast delivery by its Message-ID
    (`broadcast_deliveries.header_message_id`, unique across the server), and `/i/{token}` finds an
    e-mail image by its token (`email_images.token`, unique across the server) before the workspace
    is known; everything after those lookups is scoped by the workspace they returned.
  - The widget and the apps name only a chat or app channel's public key, which is unique across
    the server: a new contact session finds its channel by `chat_channels.public_key` (app
    channels live in the same table), and a CORS preflight, which
    carries neither the key nor the session, asks whether any chat channel allows its origin. A
    contact session token is looked up by its hash in `contact_sessions` before its workspace is
    known. Everything after those lookups is scoped by the workspace they returned.
  - The person-level identity tables `people`, `sessions`, `login_codes`, `passkeys`,
    `webauthn_ceremonies` and `push_subscriptions` (a browser's subscription follows the person
    into every workspace; the send job finds it by its id). A person signs in once and can be a member of several workspaces, so
    these rows belong to a person (or, for `login_codes`, an e-mail address before sign-in), not
    to a workspace. Their queries are scoped by the person id, the e-mail address or a secret
    hash instead; everything a person may do in a workspace goes through their `members` row.
  - The reader account endpoints (`/v1/account/*`) list a person's discussions, follows and
    notifications across every workspace. `reader_tokens` and `reader_notifications` carry
    `workspace_id` (and every write and workspace-side read filters by it), but these account
    reads are scoped by the person id instead (through `contacts.person_id` or the row's own
    `person_id`), leaving out deleted workspaces, like `/v1/me` lists a person's memberships. A
    reader token is looked up by its hash before its workspace is known, like a contact session.
    `people.reader_email_notifications` is a person-level setting.
  - Open sign-up's limits count `workspaces` by `created_by` (a person) and `created_ip` across
    the server, before the new workspace exists.
  - `operator_notices` is the server operator's feed of new people and workspaces, broadcast approval
    requests, suspensions and automatic pauses (`YUVA_OPERATOR_EMAIL`): it is about the server, not a
    workspace (a line names the workspace), and is read only by the operator digest job.
  - Analytics collection names only a property's public key: `POST /client/v1/analytics/{key}` finds
    the property by `analytics_properties.public_key` (or its `previous_key` during a rotation), unique
    across the server, and a share link finds its share by `analytics_shares.token_hash`; everything
    after those lookups is scoped by the workspace they returned. Whole day partitions of
    `analytics_visits`, `analytics_events` and `analytics_visit_pages`, which hold every workspace's
    rows of that day, are created by the flusher (yesterday to the day after tomorrow in UTC when it starts, and any day it
    is about to write that has none) and by the `analytics_partitions` job, which also drops them;
    every other read, write and delete of those tables filters by `workspace_id`.
  - A forwarding address names only its token: ingress finds the channel by
    `email_channels.forward_token` (unique across the server), like an address. A migration probe is
    recognised by its HMAC before the channel's workspace matters.
  - The migration watchdog lists migrations whose lease expired across workspaces
    (`migrations_lease_idx`), and the hourly upload sweep lists ended migrations that still have
    uploaded parts (`ListEndedMigrationUploads`); everything after those lookups is scoped by the
    workspace they returned.
  - `migration_audit` rows outlive their migration (no foreign key to it) but carry `workspace_id`
    and go with the workspace.
  - `yuva api-key revoke <id>` runs on the server with no workspace given: it finds the key by its
    id, unique across the server, and revokes it scoped by the workspace that lookup returned.
- Usage counters (conversations, messages, members, storage, broadcast recipients) recorded per workspace per month.
- Billing and plans are not part of the open-source core. Sign-up is: `YUVA_SIGNUP=open` lets
  anyone create an account and a workspace, within per-person and per-address limits (see Panel).

## Deployment

One Docker image; `deploy/compose.yaml` for a single host; the Helm chart `deploy/helm/yuva` (published as `oci://ghcr.io/productdevbook/charts/yuva`, same version as the server, no bundled database) for Kubernetes. Runs on Postgres 16+.
The minimal install is the binary, Postgres and an SMTP account.
