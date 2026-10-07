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
| Member | A person who answers. Role `owner`, `admin` or `agent`; access is granted per inbox. |
| Inbox | One product or brand, e.g. "Durma". Branding, languages, business hours, mode (`live` or `async`), expected reply time, identity secret, webhooks. |
| Channel | How messages reach an inbox: `email`, `chat` (web widget), `app` (mobile SDKs), `api` (server-to-server, e.g. a feedback form). An inbox has any number of channels. |
| Contact | The person writing in. Known by an external user id from the host app, one or more e-mail addresses, or an anonymous visitor id. |
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
- Realtime: WebSocket (`coder/websocket`). An in-process hub fans events out to connections;
  Postgres `LISTEN/NOTIFY` carries events between replicas. Clients resume with the last event id
  and fetch anything missed over HTTP.
- The panel and the widget bundles are embedded with `go:embed`; one binary serves everything.
- Configuration through environment variables. Secrets stored in the database (SMTP passwords,
  identity secrets, webhook secrets) are encrypted with a master key.
- `slog` structured logs, Prometheus metrics, `/healthz` and `/readyz`.

### API surfaces

| Prefix | Used by | Auth |
|---|---|---|
| `/v1` | panel, host backends, scripts | member session cookie, or workspace API key |
| `/client/v1` | widget, mobile SDKs | channel public key + contact session |
| `/ingress/email` | edge Worker, MTAs | HMAC signature over the raw message |
| `/ingress/ses` | SES bounce and complaint notifications via SNS | SNS signature |
| `/v1/realtime`, `/client/v1/realtime` | panel, widget, SDKs | as above |

### Identity

The host app's backend knows who its user is; Yuva trusts it through a short-lived identity token:
a JWT (HS256) signed with the inbox's identity secret, with `sub` (the host's user id), optional
`email`, `name`, `locale` and `attrs` (plan, app version, anything the panel should show), and
`exp` at most 10 minutes ahead. The client exchanges it at `/client/v1/session` for a contact
session. `sdk/go` signs these tokens and verifies webhooks.

Channels can allow anonymous visitors (web chat on a public site). An anonymous contact is merged
into the identified one when the same browser later sends an identity token. E-mail addresses are
matched to contacts only when they arrive by e-mail or inside a valid identity token.

### E-mail

Inbound:
1. A Cloudflare Email Worker (`edge/`) receives mail for the support addresses and POSTs the raw
   message with the envelope recipient to `/ingress/email`. Any other MTA can do the same; the
   endpoint is not tied to Cloudflare.
2. The recipient selects the channel. Threading uses `In-Reply-To` and `References` against stored
   Message-IDs; our outbound Message-IDs carry the conversation token, so a reply finds its
   conversation even when the client drops `References`. Otherwise a new conversation starts.
3. MIME parsing with enmime; visible text with quotes and signatures stripped (our own Go
   implementation, tested against a fixture corpus); HTML sanitized with bluemonday for display;
   the original message kept in object storage.
4. Loops: messages with `Auto-Submitted` other than `no`, `Precedence: bulk|junk|list|auto_reply`,
   `X-Autoreply`, or our own Message-ID domain never trigger an automatic message. Per-sender rate
   limits stop runaway loops.
5. Spam: the upstream verdict (`Authentication-Results`) is stored and shown; DMARC failures go to
   a spam view; contacts can be blocked.

Outbound: SMTP per channel (works with SES, Postmark, any relay). `From` is the channel address,
`Message-ID`, `In-Reply-To` and `References` keep the thread, automatic messages carry
`Auto-Submitted: auto-replied`. Bounces and complaints mark the address as undeliverable and are
shown in the conversation.

### Live chat and async messaging (web)

`sdk/js` ships a framework-free web component, `<yuva-chat>`, in Shadow DOM, translated with Lingui,
loaded by one script tag. Two layouts: a floating launcher for websites, and `embedded` for an
inline thread inside a product's own panel.

- `live` inboxes show who is available (members with access, active, within business hours),
  typing indicators and read receipts.
- `async` inboxes show the expected reply time instead and no presence.
- If the contact has left when a reply arrives and an e-mail address is known, the reply is e-mailed
  after a delay; their e-mail answer continues the same conversation.

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
- Sign-in with an e-mailed one-time code and passkeys.

### Storage

Attachments and raw e-mails in S3-compatible storage (R2, S3, MinIO) or on local disk for a
single-node install. Size limit and content-type allowlist per channel.

## Privacy and data

- Retention per workspace: closed conversations and raw e-mails deleted after a set period.
- Contact deletion and export through the API, for GDPR and KVKK requests.
- Nothing is sent to third parties except what a channel is configured to send (SMTP, webhooks).

## Hosting for others later

- `workspace_id` on every row; every store query is scoped by it.
  Two exceptions: `workspaces` itself, whose `id` is the workspace, and River's `river_*` queue
  tables, which the library owns; job arguments carry the `workspace_id` instead.
- Usage counters (conversations, messages, members, storage) recorded per workspace per month.
- Billing, plans and the signup flow are not part of the open-source core.

## Deployment

One Docker image; `deploy/compose.yaml` for a single host; a Helm chart later. Runs on Postgres 16+.
The minimal install is the binary, Postgres and an SMTP account.
