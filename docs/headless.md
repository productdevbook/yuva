# Headless

Everything the panel and the widget do is reachable without them. Build your own inbox for your
team on `/v1`, your own chat for your users on the contact API, and bots that answer through
webhooks and drafts. Working code for each is in [`examples/`](../examples), MIT licensed.

## In short

- Make an [API key](#api-keys) with only the scopes it needs, limited to some inboxes if you like.
  Every key is a bot with a name, which is shown on what it writes.
- [Your own inbox UI](#your-own-inbox-ui): read and write over `/v1`, and follow changes through
  the [event feed](#event-feed) or [realtime](#realtime).
- [Your own chat UI](#your-own-chat-ui): `createYuvaClient()` from `useyuva` speaks the contact
  API without any DOM.
- [A bot that answers with drafts](#a-bot-that-answers-with-drafts): a webhook tells it a message
  came in; it posts a draft, and a member sends it, unless the workspace lets bots send.
- Retried `POST`s are safe with an [`Idempotency-Key`](#idempotency).
- [`YUVA_PANEL=off` and `YUVA_WIDGET=off`](#api-only-servers) serve only the API.

## API keys

Owners and admins make keys in Settings → API keys, or on the server with
`yuva api-key create --workspace <id|name> --name <name> --scope <scope>… --inbox <id>…` (without
`--scope`, the key gets every scope). A key holds:

- **Scopes.** Every `/v1` operation a key may call needs one; the operation's description in the
  [API reference](api.md) names it. A call without it gets `403 insufficient_scope`, with the missing
  scope in `scope`.
- **Inboxes** (optional). The key then sees what an agent with access to exactly those inboxes
  sees. Such a key cannot hold `inboxes:manage`, `webhooks:manage` or `workspace:manage`.
- **Expiry** (optional). After it, calls get `401 api_key_expired`.
- **Bot name and avatar.** Messages, notes and events written with the key have the author type
  `bot` and show the bot's name (the key's name unless you give one) in the panel, the widget, the
  SDKs, webhook payloads and as the display name on outgoing e-mail.

Scopes, inboxes and expiry are fixed when the key is made; the name, bot name and avatar can be
changed (`PATCH /v1/api-keys/{id}`). Keys made before scopes existed hold every scope.

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

Keep keys on your servers. A browser or app never holds one; contacts use the
[contact API](#your-own-chat-ui) with their own session instead.

## Your own inbox UI

The panel is one client of `/v1`; yours can be another. A key with `conversations:read`,
`messages:write` and `conversations:write` covers a basic inbox:

```sh
curl https://support.example.com/v1/conversations?status=open \
  -H "Authorization: Bearer $YUVA_API_KEY"

curl https://support.example.com/v1/conversations/$CONVERSATION/messages \
  -H "Authorization: Bearer $YUVA_API_KEY"

curl -X POST https://support.example.com/v1/conversations/$CONVERSATION/messages \
  -H "Authorization: Bearer $YUVA_API_KEY" -H "Content-Type: application/json" \
  -d '{"kind": "message", "body": "We have shipped a fix.", "draft": true}'
```

A reply written with a key is a draft unless the workspace lets bots send (see
[below](#bots-may-send)). For a UI used by your team, members can sign in to Yuva and use their own
session, or you can register your app as an [OAuth client](mcp.md#oauth) so it acts as each member.

Typed clients are generated from the [OpenAPI contract](../openapi/openapi.yaml):

```ts
import { createYuvaApi } from "useyuva/api";

const api = createYuvaApi({ server: "https://support.example.com", apiKey: process.env.YUVA_API_KEY! });
const { data, error } = await api.GET("/v1/conversations", { params: { query: { status: "open" } } });
```

In Go, `github.com/productdevbook/yuva/sdk/go/client` is the same contract
([sdk/go/README.md](../sdk/go/README.md)). `useyuva` is not on npm yet; until it is, build it
from `sdk/js` and depend on it by path, as the examples do.

### Event feed

`GET /v1/events?after=<id>` returns the events realtime and webhooks carry, in order, filtered by
what the caller may see. It suits scripts, cron jobs and servers that were offline for a while:

1. On the first start, load what you show over the lists, then take the current position from
   `GET /v1/events/latest`.
2. Call `GET /v1/events?after=<position>&limit=100`. Handle `events`, store `next` as your
   position. While `has_more` is true, call again at once; otherwise poll a little later.
3. Events are kept 7 days. A position older than that answers `410 cursor_expired`: reload the
   lists and continue from `GET /v1/events/latest`.

`after=0` starts at the oldest kept event. A page can hold fewer events than `limit` while
`has_more` is true, because events you may not see are skipped. The key needs
`conversations:read`; contact events also need `contacts:read`.
[`examples/inbox-feed`](../examples/inbox-feed) does exactly this.

### Realtime

For a live UI, open the WebSocket `GET /v1/realtime` with `Authorization: Bearer <key>`. Each frame
is one JSON event with an increasing `id`. Remember the largest `id` you handled (and the
`last_event_id` of the `ready` frame) and reconnect with `?last_event_id=<it>`: the server replays
what you missed before the live events, or sends `resync_required` when the gap is older than 7
days. `typing` frames have no `id` and are not replayed.

## Your own chat UI

Contacts use the contact API (`/client/v1`) with a session of their own, never an API key.
`createYuvaClient()` from `useyuva` wraps it without any DOM, so it runs in browsers, Node 22+,
Bun, Deno and workers. The [`<yuva-chat>` element](widget.md) is built on it.

```ts
import { createYuvaClient } from "useyuva";

const yuva = createYuvaClient({
  server: "https://support.example.com",
  channel: "yuva_pk_…",
  identityToken: () => fetch("/my/yuva-token").then((r) => r.text()),
});

yuva.on("event", (event) => {
  if (event.type === "message.created") render(event.data);
});
await yuva.connect();

const { conversation } = await yuva.startConversation({ body: "Hello" });
await yuva.sendMessage(conversation.id, { body: "One more thing" });
```

- `channel` is the public key of a `chat` or `app` channel. `chat` channels accept only their
  allowed origins, so outside a browser use an `app` channel.
- `identityToken` returns a token your backend signs for the signed-in user
  ([Identity tokens](identity.md)). Without it the contact is an anonymous visitor, if the channel
  allows them.
- `storage` keeps the session and visitor id: `localStorage` in browsers, memory elsewhere unless
  you pass your own `{ getItem, setItem }`.
- `connect()` keeps realtime open and resumes after the last event on reconnect.
- `useyuva/react` adds `YuvaProvider`, `useConversations()` and `useMessages(id)`.

Every option and method is in the [SDK README](../sdk/js/README.md).
[`examples/headless-chat`](../examples/headless-chat) is a terminal chat built only on the client.
On iOS and Android the [mobile SDKs](mobile.md) have the same kind of client under their UI.

## A bot that answers with drafts

A bot is an API key plus your code. The usual loop:

1. Add a [webhook](webhooks.md) for `message.created` that points at your bot, and keep its
   `whsec_…` secret.
2. On each request, [verify the signature](webhooks.md#verify-the-signature) over the raw body,
   then look at `data.message`: act on `direction: "in"` and `kind: "message"`, ignore the rest
   (your own replies come back as `message.created` too).
3. Post the answer as a draft, with an `Idempotency-Key` derived from the incoming message, so a
   webhook Yuva retries gives back the same draft instead of a second one:

```sh
curl -X POST https://support.example.com/v1/conversations/$CONVERSATION/messages \
  -H "Authorization: Bearer $YUVA_API_KEY" -H "Content-Type: application/json" \
  -H "Idempotency-Key: draft-bot:$INCOMING_MESSAGE_ID" \
  -d '{"kind": "message", "direction": "out", "draft": true, "body": "Thanks, we are on it."}'
```

A draft is never delivered on its own. Members see it in the conversation with the bot's name and
send, edit or discard it. Over the API: `PATCH /v1/messages/{id}` edits it, `DELETE /v1/messages/{id}`
discards it and `POST /v1/messages/{id}/send` delivers it; anything that is not a draft answers
`409 not_a_draft`. A draft never reaches the contact, the widget, notifications or unread counts.
Drafts emit `draft.created`, `draft.updated` and `draft.deleted` (on realtime, the event feed, and
as webhook types you can subscribe to); sending one emits the usual `message.created`, with the bot
kept as author and `sent_by` naming who sent it.

[`examples/draft-bot`](../examples/draft-bot) is this bot in Go, with `sdk/go/webhook` and
`sdk/go/client`.

### Bots may send

The workspace setting **Bots may send** (Settings → API keys, `bots_may_send` on
`PATCH /v1/workspace`) decides whether keys deliver anything:

- **Off** (the default for new workspaces): a key's outgoing message must be a draft, else
  `403 bot_sending_disabled`, and a key cannot send drafts.
- **On**: a key with `messages:write` posts replies that are delivered at once, and a key that also
  holds `drafts:send` sends drafts.

Only owners and admins change it, with their own session; no key can, so a bot never lets itself
send. A bot's e-mail goes out from the inbox's address with the bot's name as display name.

## Idempotency

Every authenticated `POST` on `/v1` and `/client/v1` accepts an `Idempotency-Key` header (1 to 255
printable ASCII characters). Yuva remembers it for 24 hours per caller, with the method, path and
body:

| Retry | Answer |
|---|---|
| Same key, same request | The first response again, with `Idempotent-Replayed: true` |
| Same key, different request | `409 idempotency_key_reused` |
| Same key while the first is still running | `409 idempotency_key_in_use` |

`5xx` answers are not stored, so the request can be retried. Sign-in, the contact session request
and the `/v1/me/…` endpoints ignore the header. In the contact API, a message's `client_id` also
makes a retry safe; the JS client always sends one, and outside browsers also sends it as
`Idempotency-Key` (`idempotencyKeys` option).

## API-only servers

A server that only backs your own UIs and bots does not need the panel or the widget scripts:

| Variable | Effect |
|---|---|
| `YUVA_PANEL=off` | The panel's pages answer `404`. The OAuth consent page goes too, so OAuth clients cannot connect; API keys keep working. |
| `YUVA_WIDGET=off` | `/yuva.js` and `/yuva-chat.js` answer `404`; the contact API stays for the JS client and the mobile SDKs. |

The API, realtime, e-mail ingress and the [MCP endpoint](mcp.md) stay. See
[Configuration](configuration.md#server).
