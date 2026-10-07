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
(WhatsApp, Instagram, …). They may come later; the model must not block them.

## Concepts

| Concept | Meaning |
|---|---|
| Workspace | The tenant. A self-hosted install usually has one. Owns everything below. |
| Member | A person who answers. Role `owner`, `admin` or `agent`. Owners and admins see every inbox; agents see the inboxes they were granted. An inbox, conversation or attachment the caller cannot see answers `404`. |
| Inbox | One product or brand. Branding, languages, business hours, mode (`live` or `async`), expected reply time, identity secret, webhooks. |
| Channel | How messages reach an inbox: `email`, `chat` (web widget), `app` (mobile SDKs), `api` (server-to-server, e.g. a feedback form). An inbox has any number of channels. |
| Contact | The person writing in. Known by an external user id from the host app (per inbox), one or more e-mail addresses (unique within the workspace), or an anonymous visitor id. Contacts belong to the workspace, not to an inbox. |
| Conversation | A thread between a contact and an inbox. Status `open`, `pending`, `snoozed`, `closed`; assignee; labels; priority; the channel it started on. |
| Message | One entry in a conversation: `message` (to or from the contact), `note` (members only) or `event` (assigned, closed, …). Has attachments and a per-channel delivery state. |

Feedback is a conversation that starts on the `app` or `api` channel with a `feedback` kind and
metadata (category, app version, build, OS, device, locale, screen). It is answered like any other
conversation; the answer reaches the contact in the app, or by e-mail when they allowed it.

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
  replays newer events (kept 24 hours, cleaned by a River job) before live ones, or answers
  `resync_required` and the client reloads over HTTP. A slow connection is closed instead of
  holding up the hub; when the listener reconnects, every connection is closed so it resumes.
  Typing and presence notices are not stored: they travel as the payload of a second `NOTIFY`
  channel, have no id and are never replayed. Each open realtime connection is a row in
  `realtime_connections`, seen on every heartbeat; presence is read from the rows seen in the last
  75 seconds.
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

### Identity

The host app's backend knows who its user is; Yuva trusts it through a short-lived identity token:
a JWT (HS256) signed with the inbox's identity secret, with `sub` (the host's user id), optional
`email`, `name`, `locale` and `attrs` (plan, app version, anything the panel should show), and
`exp` at most 10 minutes ahead. The client exchanges it at `/client/v1/session` for a contact
session. `sdk/go` signs these tokens and verifies webhooks.

The token is checked strictly: `alg` must be `HS256` (anything else, `none` included, is
refused), the signature must match the inbox secret, `exp` is required and at most 10 minutes
ahead, `sub` is required. The contact is found by `sub` among the inbox's external ids, then by the
token's `email`, or created; `name`, `email`, `locale` and `attrs` are saved on it. Contact
sessions are opaque tokens, stored as hashes, valid for 7 days after their last use.

Channels can allow anonymous visitors (web chat on a public site). An anonymous visitor is a
contact plus a random visitor id that the browser keeps; the id resumes that visitor, and only in
the inbox that issued it. When the same browser later sends an identity token with its visitor id,
the visitor's conversations move to the identified contact (or, for a contact Yuva has not seen
yet, the visitor becomes it) and the visitor id ends. E-mail addresses are matched to contacts only
when they arrive by e-mail or inside a valid identity token: an address a visitor types in the
widget is kept apart (`typed_email`) and used only to e-mail them replies; it becomes one of their
addresses when mail from it answers our e-mail to that address in the conversation's thread.

### E-mail

Inbound:
1. A Cloudflare Email Worker (`edge/`) receives mail for the support addresses and POSTs the raw
   message with the envelope recipient to `/ingress/email`, signed with HMAC-SHA256 under
   `YUVA_INGRESS_SECRET` (one per install; the request format is in `edge/README.md`). The
   timestamp must be within 5 minutes and a Message-ID already stored for the channel is accepted
   again without a second copy. Any other MTA can do the same; the endpoint is not tied to
   Cloudflare. An MTA that pipes into a command uses `yuva ingest-email --to <address>`, which
   exits with sysexits codes (67 unknown recipient, 77 refused, 75 try later).
2. The recipient selects the channel: an e-mail address belongs to one channel of the whole server
   (case-insensitive; `local+tag@` falls back to `local@`), because the request names nothing
   else. Threading uses `In-Reply-To` and `References` against stored Message-IDs of the inbox;
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
   the panel shows that link, and nothing is added to the other conversation. The `Cc` of inbound
   mail is recorded and shown; nobody is copied automatically on replies.
3. MIME parsing with enmime; visible text with quotes and signatures stripped (our own Go
   implementation in `api/internal/email/reply`, tested against a fixture corpus); the quoted
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
   automatic message. A sender opens at most 20 new conversations per channel and hour; more are
   refused (429 `rate_limited`) until the hour has passed. Cloudflare Email Workers can only refuse
   a message permanently (`setReject`; a temporary failure is not documented), so the refusal is
   permanent and its text says the message was not accepted rather than asking for a retry.
5. Spam: the receiving server's verdict (the topmost `Authentication-Results`) is stored and
   shown; a new conversation whose first mail fails DMARC is flagged `spam`, which keeps it out of
   lists and counts (they have a spam view) and away from automatic replies; contacts can be
   blocked.

Outbound: SMTP per channel (works with SES, Postmark, any relay); the password is stored
encrypted under the master key and never returned, and it is only sent over TLS. A member's reply
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
only on an `sns.<region>.amazonaws.com` URL, and a notification counts only for a message we sent
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
  channel. Session starts and contact writes are rate limited per IP address and per channel,
  counted in each process.
- Contacts see their own conversations of the inbox, messages only (no notes, no internal events),
  conversation status, and of members only the display name and initials.
- `live` inboxes show who is available: members with access to the inbox, with an open
  `/v1/realtime` connection, not set to `away`, while the inbox is within business hours. They also
  show members typing and how far members have read.
- `async` inboxes show the expected reply time instead and no presence.
- If the contact has left when a reply arrives and an e-mail address is known, the reply is e-mailed
  after a delay; their e-mail answer continues the same conversation. A River job checks the
  conversation `YUVA_CHAT_EMAIL_DELAY` (5 minutes by default) after a member's reply and after the
  contact's last connection closes: the members' replies the contact has not read, once the oldest
  is that old and the contact has been gone that long, go out as one e-mail through the inbox's
  e-mail channel, threaded so an answer finds the conversation, at most one e-mail per delay. Notes
  are never sent.

### In-app messaging (mobile)

- `sdk/swift` (Swift package `YuvaKit`): a headless client plus SwiftUI screens (conversation list,
  thread, composer, attachments, feedback form).
- `sdk/kotlin`: the same for Android, Compose UI.
- The host app gets an identity token from its own backend and opens Yuva's screens or drives its
  own UI from the client.
- Push: when a contact has no live connection, Yuva sends a `message.created` webhook to the host
  backend, which already holds the device tokens and APNs/FCM keys. Yuva sending push itself is
  a later, optional channel setting.

### Webhooks

Signed per the Standard Webhooks specification, retried with backoff by the job queue, visible
with their last deliveries in the panel. Events: `conversation.created`, `conversation.updated`,
`message.created`, `contact.updated`. The host backend also calls `DELETE /v1/contacts/{id}` (by
external id) when a user deletes their account.

### Panel (`web/`)

React, Vite, shadcn, TanStack Query, Lingui; built to static files and embedded in the binary.

- Sidebar: all, mine, unassigned, per inbox, per label. Conversation list with filters and
  full-text search (Postgres FTS).
- Thread: reply and note in one composer, canned replies on `/`, attachments, keyboard shortcuts,
  contact sidebar with identity attributes, earlier conversations and channel delivery state.
- Settings: inboxes, channels, members and access, labels, canned replies, business hours,
  auto-replies, webhooks, API keys.
- Installable PWA with Web Push (VAPID), so members get notifications on phones without a native
  app. E-mail notifications as a fallback.
- Sign-in with an e-mailed one-time code and passkeys. There is no open sign-up: the first owner
  is created with `yuva bootstrap`, everyone else is invited. Sign-in and invitation e-mails are
  sent directly, not through the job queue, so a code never lands in job arguments.

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

- Retention per workspace: closed conversations and raw e-mails deleted after a set period.
- Contact deletion and export through the API, for GDPR and KVKK requests.
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
  - The widget names only a chat channel's public key, which is unique across the server: a new
    contact session finds its channel by `chat_channels.public_key`, and a CORS preflight, which
    carries neither the key nor the session, asks whether any chat channel allows its origin. A
    contact session token is looked up by its hash in `contact_sessions` before its workspace is
    known. Everything after those lookups is scoped by the workspace they returned.
  - The person-level identity tables `people`, `sessions`, `login_codes`, `passkeys` and
    `webauthn_ceremonies`. A person signs in once and can be a member of several workspaces, so
    these rows belong to a person (or, for `login_codes`, an e-mail address before sign-in), not
    to a workspace. Their queries are scoped by the person id, the e-mail address or a secret
    hash instead; everything a person may do in a workspace goes through their `members` row.
- Usage counters (conversations, messages, members, storage) recorded per workspace per month.
- Billing, plans and the signup flow are not part of the open-source core.

## Deployment

One Docker image; `deploy/compose.yaml` for a single host; a Helm chart later. Runs on Postgres 16+.
The minimal install is the binary, Postgres and an SMTP account.
