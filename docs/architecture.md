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
a reply e-mail does not confirm the address.

### E-mail

Inbound:
1. A Cloudflare Email Worker (`edge/`) receives mail for the support addresses and POSTs the raw
   message with the envelope recipient to `/ingress/email`, signed with HMAC-SHA256 under
   `YUVA_INGRESS_SECRET` (one per install; the request format is in `edge/README.md`). The
   timestamp must be within 5 minutes and a Message-ID already stored for the channel is accepted
   again without a second copy. When the server cannot be reached (5xx, network error, no answer within 20
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
   the panel shows that link, and nothing is added to the other conversation. The `Cc` of inbound
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
  channel: a request for a chat channel must carry one of its origins, and one without an `Origin`
  header is refused. Session starts and contact writes are rate limited per IP address and per
  channel, counted in each process.
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
  are never sent. A conversation without a subject is mailed as the inbox name, ` — ` and the first
  60 characters of its first message.

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

Events: `conversation.created`, `conversation.updated`, `message.created`, `feedback.created`,
`contact.updated`, `contact.deleted`. Payloads follow Standard Webhooks: `{type, timestamp,
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

- Sidebar: all, mine, unassigned, per inbox, per label. Conversation list with filters and
  full-text search (Postgres FTS).
- Thread: reply and note in one composer, canned replies on `/`, attachments, keyboard shortcuts,
  contact sidebar with identity attributes, earlier conversations and channel delivery state.
- Settings: inboxes, channels, members and access, labels, canned replies, business hours,
  auto-replies, webhooks, API keys.
- Installable PWA with Web Push (VAPID), so members get notifications on phones without a native
  app. E-mail notifications as a fallback (see Member notifications).
- Sign-in with an e-mailed one-time code and passkeys. There is no open sign-up: the first owner
  is created with `yuva bootstrap`, everyone else is invited. Sign-in and invitation e-mails are
  sent directly, not through the job queue, so a code never lands in job arguments; a code request
  answers after the same fixed delay for every address and its mail goes out after the answer, so
  timing does not reveal members. Each code takes five attempts; after 20 wrong codes for an
  address within 24 hours its codes are refused (`429 sign_in_paused`) and the member gets one
  e-mail about it. Passkeys are not affected.

### Member notifications

Five events notify members: the first message of a conversation, from the contact, in a `live`
or an `async` inbox (every member with access is a candidate); a later contact message in a
conversation assigned to the member, or in an unassigned one (every member with access); and
someone else assigning a conversation to the member. A River job queued in the transaction that
stored the message picks the recipients. Nobody is notified about their own action, about a
conversation marked spam, or about a conversation their open panel shows: the panel reports it
with a `viewing` frame on `/v1/realtime`, stored on the connection's row and trusted while the
connection is fresh (75 seconds). A member set to `away` gets only the events about
conversations assigned to them.

Each member chooses push and e-mail per event, and can override events per inbox. Defaults by
role: push for everything, except that agents get no push for new `async` conversations and for
messages in unassigned conversations (owners and admins triage those); e-mail only for messages
in conversations assigned to the member and for assignments, for every role, since being
assigned makes anyone responsible.

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

E-mail fallback: an event with e-mail on schedules one check per member and conversation after the
member's delay (15 minutes by default). If the conversation still has contact messages (or an
assignment) newer than the member's read position and the previous notification e-mail, one
e-mail through the server's own mailer, in the member's locale, lists them and links to the
conversation and to the notification settings; at most one per member and conversation per hour. A
member's own message or note moves their read position to it, so the e-mail never lists what
they already answered.

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
