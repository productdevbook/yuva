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

Not goals: a CRM, a marketing e-mail tool, a public knowledge base, social-media channels
(WhatsApp, Instagram, …). They may come later; the model must not block them. Yuva does not host
documentation, but it collects feedback and questions on the documentation sites products already
have (see "Documentation pages").

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
  fan-out, attachment cleanup, retention.
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
| `/client/v1` | widget, mobile SDKs | channel public key + contact session |
| `/ingress/email` | edge Worker, MTAs | HMAC signature over the raw message |
| `/ingress/ses` | SES bounce and complaint notifications via SNS | SNS signature |
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
| `feedback:write` | feedback |

A call without its scope gets `403 insufficient_scope` with the missing scope in `scope`. Keys made
before scopes existed hold every scope above. `yuva api-key create` takes `--scope` (repeatable,
default every scope) and `--inbox` (repeatable).

A key may be limited to some inboxes. It then sees what an agent with access to exactly those
inboxes sees; it cannot hold `inboxes:manage`, `webhooks:manage` or `workspace:manage` (refused when
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
limit, scopes: a caller sees an event only if it holds `conversations:read`, and contact events
also need `contacts:read`). `after` is the last event id the caller handled; the answer carries
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
  are not offered `inboxes:manage`, `labels:write`, `canned_replies:write`, `webhooks:manage` and
  `workspace:manage`, and nobody `feedback:write` (API keys only). A token may call exactly the
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
- Writes carry `destructiveHint: false` except `merge_contacts`; `assign`, `set_status`, `snooze`,
  the label tools, `move_conversation` and `bulk_update` carry `idempotentHint`; `send_reply` and
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
  subscriptions. Prompts `triage_inbox`, `draft_reply`, `summarize_conversation`, `weekly_report`;
  they only arrange tool calls and text, Yuva runs no model.
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
   `References` or a relay rewrites the domain. Otherwise a new conversation starts, with the
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
and for its recipients. Undeliverable addresses are shown on the
contact, replies to them are refused until a member clears them.

### Live chat and async messaging (web)

`sdk/js` ships a framework-free web component, `<yuva-chat>`, in Shadow DOM, translated with Lingui,
loaded by one script tag. Two layouts: a floating launcher for websites, and `embedded` for an
inline thread inside a product's own panel.

- The server hosts the widget scripts itself at `/yuva.js` and `/yuva-chat.js`.
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
`feedback.created`, `contact.updated`, `contact.deleted`. Payloads follow Standard Webhooks: `{type, timestamp,
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
  conversation: name the inbox, pick a channel (website chat, e-mail or app), install it (snippet,
  forwarding address or SDK lines), then wait live for the first message, which the preview's own
  chat widget can send. It opens on its own while the workspace has no conversation (it can be put
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
  and `assignee`, Unread filtering the loaded rows by `unread`) and an inbox picker; then the
  conversation list, most recent activity first, paged as it scrolls and kept current from the
  realtime connection. A row shows the contact, the last message (marked when a bot or the team
  wrote it, replaced by the member's unsent draft or by "typing…"; a team reply in a chat or app
  conversation shows ✓, or ✓✓ once `last_read_by_contact_at` reaches it), the time, an unread mark, the
  channel, the assignee and a green dot when the contact of a chat or app conversation is online.
  ↑/↓ move through the list, Enter opens, J/K open the next or previous one. Each member can pin
  conversations to the top of their own list and mark one unread again, from the row's context
  menu or the conversation's menu (P, U): `PUT`/`DELETE /v1/conversations/{id}/pin` and
  `POST /v1/conversations/{id}/unread`, kept per member in `conversation_member_states` next to the
  read cursors. The list item carries `pinned_at` for the caller, `pinned=true|false` filters on
  it, a mark shows as `unread` until the member reads the conversation or writes in it, and
  `conversation.pin` and `conversation.read` events reach only that member's connections. Pinned
  conversations of the current filter stand in their own section above the rest. In the middle, the
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
  the conversation is narrower than 36rem, so the contact's name keeps its room. On a phone the list and the conversation are
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
- Sign-in with an e-mailed one-time code and passkeys. There is no open sign-up: the first owner
  is created with `yuva bootstrap`, everyone else is invited. A person whose memberships are all
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
- **Page questions.** `<yuva-page-questions>` lists the answers published on the page and lets a
  visitor ask a question, which starts a conversation of kind `question` with `page_url` and
  `page_title` (`POST /client/v1/questions`), from a visitor session, with an optional e-mail
  address for the answer. Conversations keep `page_url` and `page_title` as columns for both page
  feedback and questions, so the panel filters and counts them per page. Questions are private
  conversations: nothing a visitor writes is ever shown on the page by itself.
- **Published answers.** A member who answered a question publishes it to the page
  (`POST /v1/conversations/{id}/publish`): the question and the answer as the member edits them,
  stored as a `page_answers` row (workspace, inbox, channel, page, page title, question, answer,
  the member, the source conversation, published and updated times), with no name or address of
  the visitor. A conversation publishes once, while it is still on a chat channel that allows the
  page's origin; deleting that channel removes its ratings and answers.
  Members edit, unpublish (delete) and list them per page under the same inbox access as the
  conversation. `GET /client/v1/channels/{key}/page-answers?page=` returns a page's published
  answers newest first without a session and is cacheable for a minute. A published answer is the
  member's text, so retention and contact deletion leave it; deleting the source conversation
  keeps it without the link.
- **Panel.** A Docs view lists pages of the inboxes the member can see with their up and down
  counts over a window (7, 30 or 90 days), the share of `down`, open feedback and questions and
  the number of published answers (`GET /v1/docs/pages`); a page shows its ratings per day
  (`GET /v1/docs/page`), its feedback and questions (`GET /v1/conversations?page=`) and its
  published answers (`GET /v1/page-answers?page=`). A `question` conversation offers "Publish to
  page" once it has a member reply.
- Ratings are counters, so they send no events or webhooks; page feedback and questions are
  conversations and send the same ones as any conversation and feedback.

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

### Storage

Attachments and raw e-mails in S3-compatible storage (R2, S3, MinIO) or on local disk for a
single-node install: `YUVA_STORAGE=local|s3`, with `YUVA_STORAGE_DIR` for local disk and
`YUVA_S3_ENDPOINT`, `YUVA_S3_REGION`, `YUVA_S3_BUCKET`, `YUVA_S3_ACCESS_KEY_ID`,
`YUVA_S3_SECRET_ACCESS_KEY`, `YUVA_S3_PATH_STYLE` for S3. Objects are never public; members and API
keys download them through `/v1/attachments/{id}`, which checks inbox access. The size limit
(`YUVA_ATTACHMENT_MAX_BYTES`, 25 MiB by default) and the content-type allowlist
(`YUVA_ATTACHMENT_TYPES`, comma-separated, `image/*` style wildcards allowed) are server-wide for
now; per-channel limits can narrow them later.

## Privacy and data

- Retention per workspace: `workspaces.retention_days`, set by an owner (`PATCH /v1/workspace`);
  `null` keeps everything. An hourly River job deletes closed conversations whose last change and
  last activity are older than that, with their messages, attachments and stored files, and
  removes raw e-mails older than that from storage (the parsed message stays).
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
  codes; messages they wrote stay without an author, shown as a deleted member.
- Contact deletion and export through the API, for GDPR and KVKK requests. A host backend deletes
  a user who deleted their account with `DELETE /v1/contacts/by-external-id?inbox_id=&external_id=`
  (an API key or an owner or admin): the contact goes with all their conversations, messages,
  attachments, sessions and addresses in every inbox, the stored files are removed, and
  `contact.deleted` tells the other endpoints which external ids are gone.
- Nothing is sent to third parties except what a channel is configured to send (SMTP, webhooks).

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
    by the workspace it returned.
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
  - `yuva api-key revoke <id>` runs on the server with no workspace given: it finds the key by its
    id, unique across the server, and revokes it scoped by the workspace that lookup returned.
- Usage counters (conversations, messages, members, storage) recorded per workspace per month.
- Billing, plans and the signup flow are not part of the open-source core.

## Deployment

One Docker image; `deploy/compose.yaml` for a single host; a Helm chart later. Runs on Postgres 16+.
The minimal install is the binary, Postgres and an SMTP account.
