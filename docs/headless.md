# Headless

Everything the panel and the widget do is open to your own code: your own inbox on `/v1`, your own
chat on the contact API, or a bot that answers with drafts. Working code is in
[`examples/`](../examples), MIT licensed.

## API keys

Your code talks to `/v1` with an API key. Owners and admins make keys in Settings → API keys, or on
the server:

```sh
yuva api-key create --workspace <id|name> --name "Support bot" --scope conversations:read --scope messages:write
```

Give a key only the [scopes](#scopes) it needs (no `--scope` means all); `--inbox <id>` limits it
to some inboxes. Every key is a bot whose name shows on what it writes. Keep keys on your servers.

## Your own inbox UI

Read and write conversations with the typed client (`npm install useyuva`):

```ts
import { createYuvaApi } from "useyuva/api";

const api = createYuvaApi({ server: "https://support.example.com", apiKey: process.env.YUVA_API_KEY! });
const { data } = await api.GET("/v1/conversations", { params: { query: { status: "open" } } });
```

In Go: `github.com/productdevbook/yuva/sdk/go/client` ([README](../sdk/go/README.md)). Other
languages: the [OpenAPI contract](../openapi/openapi.yaml). A UI for your team can act as each
member through [OAuth](mcp.md#oauth).

To follow changes, either:

- **Poll the event feed.** Start from `GET /v1/events/latest`, then call
  `GET /v1/events?after=<position>&limit=100`, handle `events`, store `next`, and repeat at once
  while `has_more` is true. See [`examples/inbox-feed`](../examples/inbox-feed).
- **Keep a WebSocket open** on `GET /v1/realtime` with `Authorization: Bearer <key>`. Reconnect
  with `?last_event_id=<last id handled>` to get what you missed.

## Your own chat UI

Contacts use the contact API (`/client/v1`) with their own session, never an API key.
`createYuvaClient()` wraps it without any DOM, for browsers, Node 22+, Bun, Deno and workers:

```ts
import { createYuvaClient } from "useyuva";

const yuva = createYuvaClient({
  server: "https://support.example.com",
  channel: "yuva_pk_…",
  identityToken: () => fetch("/my/yuva-token").then((r) => r.text()),
});

yuva.on("event", (event) => console.log(event.type));
await yuva.connect();
const { conversation } = await yuva.startConversation({ body: "Hello" });
```

Outside a browser, use an `app` channel's key; `chat` channels accept only their allowed origins.
React hooks are in `useyuva/react`; every option is in the [SDK README](../sdk/js/README.md).

## A bot that answers with drafts

1. Add a [webhook](webhooks.md) for `message.created` that points at your bot.
2. [Verify the signature](webhooks.md#verify-the-signature) and act only on `data.message` with
   `direction: "in"` and `kind: "message"`.
3. Post the answer as a draft. An `Idempotency-Key` from the incoming message stops a retried
   webhook from making a second draft:

```sh
curl -X POST https://support.example.com/v1/conversations/$CONVERSATION/messages \
  -H "Authorization: Bearer $YUVA_API_KEY" -H "Content-Type: application/json" \
  -H "Idempotency-Key: draft-bot:$INCOMING_MESSAGE_ID" \
  -d '{"kind": "message", "direction": "out", "draft": true, "body": "Thanks, we are on it."}'
```

A member sees the draft with the bot's name and sends, edits or discards it. See
[`examples/draft-bot`](../examples/draft-bot).

### Bots may send

The workspace setting **Bots may send** (Settings → API keys) is off by default: a key's outgoing
message must then be a draft (`403 bot_sending_disabled` otherwise). When on, a key with
`messages:write` replies directly, and with `drafts:send` also sends drafts. Only an owner or admin
signed in to the panel can change it (`bots_may_send` on `PATCH /v1/workspace`), never a key.

## Reference

### Scopes

A call without the needed scope answers `403 insufficient_scope`.

| Scope | Allows |
|---|---|
| `conversations:read` | Conversations, messages, attachments, labels, canned replies, the event feed and realtime |
| `conversations:write` | Status, assignee, snooze, priority, labels; move and bulk-update conversations |
| `messages:write` | Messages and drafts: create, edit, discard |
| `drafts:send` | Send a draft, together with `messages:write` |
| `notes:write` | Notes |
| `contacts:read` | Contacts, lookups, presence |
| `contacts:write` | Create, update, delete and merge contacts |
| `inboxes:read` | Workspace, members, inboxes, channels |
| `inboxes:manage` | Create, change and delete inboxes and channels, inbox access, secrets |
| `labels:write` | Labels |
| `canned_replies:write` | Canned replies |
| `webhooks:manage` | Webhooks, their deliveries and attempts |
| `workspace:manage` | Workspace settings and usage |
| `feedback:write` | `POST /v1/feedback` |

### Keys

| Setting | Detail |
|---|---|
| Scopes, inboxes | Fixed at creation. An inbox-limited key cannot hold `inboxes:manage`, `webhooks:manage` or `workspace:manage`. |
| Expiry | Optional, fixed. Then `401 api_key_expired`. |
| Name, bot name, avatar | Shown on what the key writes. Change with `PATCH /v1/api-keys/{id}`. |

### Drafts

| Call | Does |
|---|---|
| `PATCH /v1/messages/{id}` | Edits a draft. |
| `DELETE /v1/messages/{id}` | Discards it. |
| `POST /v1/messages/{id}/send` | Delivers it; `sent_by` names who sent it. |

On a message that is not a draft: `409 not_a_draft`. Drafts emit `draft.created`, `draft.updated`
and `draft.deleted`.

### Event feed and realtime

| Rule | Detail |
|---|---|
| Retention | 7 days. An older position answers `410 cursor_expired`; realtime sends `resync_required`. Reload and restart from `GET /v1/events/latest`. |
| `after=0` | Starts at the oldest kept event. |
| Pages | May hold fewer than `limit` while `has_more` is true. |
| Scopes | `conversations:read`; contact events also `contacts:read`. |
| Typing | `typing` frames have no `id` and are not replayed. |

### Idempotency

Every authenticated `POST` on `/v1` and `/client/v1` accepts `Idempotency-Key` (1 to 255 printable
ASCII characters), kept 24 hours per caller. `5xx` answers are not stored. Sign-in, the contact
session request and `/v1/me/…` ignore it.

| Retry | Answer |
|---|---|
| Same key, same request | The first response, with `Idempotent-Replayed: true` |
| Same key, different request | `409 idempotency_key_reused` |
| Same key, first still running | `409 idempotency_key_in_use` |

### API-only servers

| Variable | Effect |
|---|---|
| `YUVA_PANEL=off` | No panel, and no OAuth consent page; API keys keep working. |
| `YUVA_WIDGET=off` | No `/yuva.js` or `/yuva-chat.js`; the contact API stays. |

See [Configuration](configuration.md#server).
