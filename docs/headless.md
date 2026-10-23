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
const { data } = await api.GET("/v1/conversations", { params: { query: { status: ["open"] } } });
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

A conversation a member deleted is gone from `/client/v1`, and the realtime stream sends a
`conversation.removed` frame (`data: { id }`): drop it from your list and close it if it is open. A message
sent to that id starts a new conversation; the answer's `conversation_id` names it. If the conversation is
restored, it arrives again as `conversation.created`. `createYuvaClient()` emits it as the
`conversationRemoved` event (`{ id }`), the React hooks drop the conversation, and the Swift and Kotlin SDKs
send `YuvaEvent.conversationRemoved(id:)` / `YuvaEvent.ConversationRemoved(id)`.

When a member [removes a message](#removing-a-message), the stream sends a `message.redacted` frame whose
`data` is the message with `redacted: true`, an empty `body` and no attachments: replace it in place and show
"This message was removed". The same message in a later replay or list carries `redacted: true` too.
`createYuvaClient()` passes the frame on its `event` event, the React hooks update the message, and the Swift and
Kotlin SDKs send `YuvaEvent.messageRedacted(_:)` / `YuvaEvent.MessageRedacted(message)`.

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
| `conversations:delete` | Move conversations to the Trash, restore them, list the Trash. Not included in any other scope: name it. Offered only to owners' and admins' keys and tokens. |
| `messages:write` | Messages and drafts: create, edit, discard |
| `messages:redact` | [Remove a message's content](#removing-a-message). Not included in any other scope: name it. |
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
| `migrations:manage` | [Moving a mailbox to Yuva](migrate.md): detect, create, connect, scan, start, finish, remove imported history. Never given to agents or to keys limited to inboxes. |
| `analytics:read` | [Analytics](analytics.md#api): properties, numbers, realtime, health |
| `analytics:write` | Send [server-side events](analytics-server.md) (`POST /v1/analytics/properties/{propertyId}/events`). API keys only, never offered to OAuth clients. |
| `analytics:manage` | Create, change and delete properties, rotate their keys, delete their data. Never given to agents or to keys limited to inboxes. |
| `status:read` | [Status pages](status-pages.md): pages, components, incidents, notices, monitors and their checks |
| `status:manage` | Change status pages, set states, post incident updates, manage monitors, link conversations to incidents and reply to them. Does not include `status:read`. |
| `help:read` | [Help centers](help-center.md): centers, collections, articles with drafts and revisions, search, the report |
| `help:write` | Create and change collections and articles, save drafts, upload images |
| `help:publish` | Publish, unpublish and delete published help articles. Not included in any other scope: name it. |
| `roadmap:read` | [Roadmap boards](roadmap-board.md): boards, stage history, what became of a reply to linked feedback |
| `roadmap:write` | Create, change and delete boards, link feedback to ideas, reply to linked feedback. Linking and reading ideas also need `conversations:read`. |
| `roadmap:stage` | Move an idea's stage, which sets the progress of its linked feedback. Not included in any other scope: name it. |
| `changelog:read` | [What's new](whats-new.md): changelogs, entries with drafts, preview |
| `changelog:write` | Write entries and translations, upload images |
| `changelog:publish` | Create, change and delete changelogs; publish, schedule, unpublish and delete published entries. Not included in any other scope: name it. |
| `audit:read` | `GET /v1/audit-log`, the [audit log](operations.md#audit-log). Not included in any other scope: name it. Only an owner can give it to a key, and a key limited to inboxes cannot hold it. It works only while the member who made the key is still an owner. `yuva api-key create` leaves it out unless you name it. |
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

### Deleting conversations

`DELETE /v1/conversations/{conversationId}` moves a conversation to the Trash. Only owners and admins,
and keys and connected apps holding `conversations:delete`, can do it; agents get `403 forbidden`. A key
limited to inboxes can delete in those inboxes. `yuva api-key create` leaves the scope out unless you
name it.

| Rule | Detail |
|---|---|
| Trash | The conversation disappears from every list, count, search and figure, for the team, the contact, apps and assistants, and nobody can change it. Doing it twice changes nothing. |
| See the Trash | `GET /v1/conversations?trashed=true`, for owners, admins and `conversations:delete`; the conversation then has `trashed_at` and `deletes_at`. |
| Restore | `POST /v1/conversations/{conversationId}/restore` within 30 days, with its status, assignee and labels. After that it answers `409 trash_expired`. |
| After 30 days | The hourly cleanup deletes it for good, with its messages, attachments and stored files. |
| Several at once | `POST /v1/conversations/bulk` with `trashed: true` or `false` and nothing else, up to 100. |
| The contact writes again | A new conversation starts, by e-mail, chat or app; the trashed one is never reopened. |

In the panel, owners and admins find Delete in a conversation's menu and for a selection, `#` as the shortcut, and
a Trash tab with Restore.

Webhooks send `conversation.trashed` and `conversation.restored` (the conversation and contact) and, after
30 days, `conversation.deleted` with only `id`, `inbox_id` and `contact_id`. The
[audit log](operations.md#audit-log) records who trashed or restored each conversation, and the final
deletion as done by the system.

### Removing a message

`POST /v1/messages/{messageId}/redact` removes the content of one message or note and keeps a line in the
thread: "Removed by <name>" with the time. It answers `200` with the message, which has `redacted_at`,
`redacted_by`, an empty `body` and no `html`, attachments, cards or mentions. Owners and admins can remove any
message, other members only the messages and notes they wrote (`403 forbidden`). A key needs `messages:redact`
and can remove any message it can see; `yuva api-key create` leaves the scope out unless you name it.

| Rule | Detail |
|---|---|
| Removed | The text, HTML, attachments and their stored files, and for e-mail the subject, headers and original message, in one step. The files are deleted by a job right after. |
| Stays | The author, the time, the position in the thread and, for e-mail, the delivery state. |
| Other copies | Earlier events, stored webhook deliveries and previews lose the content, and the message leaves search. |
| Quoted by the contact | If the contact's later e-mail answers a removed e-mail and quotes all of it, the quote and that e-mail's original are removed too; what they wrote new stays. A partial quote stays. |
| First message | Removing a feedback's first message clears its `feedback.text`; removing the e-mail that gave a conversation its subject clears the subject. Both send `conversation.updated`. |
| E-mail not yet sent | A queued e-mail is not sent and shows `failed`. |
| E-mail already sent | It stays in the recipient's mailbox: Yuva cannot take it back. |
| Again | Removing a removed message answers it unchanged. |
| Not removable | A draft (discard it with `DELETE`) and a timeline event answer `409 not_redactable`. A conversation in the Trash answers `404`. |

Webhooks send [`message.redacted`](webhooks.md#events) without the old content; delete any copy you keep. The
[audit log](operations.md#audit-log) records each removal, never the text.
In the panel, **Remove** is on every message and note for owners, admins and the author. A confirmation names the
consequence, including that a sent e-mail stays in the recipient's mailbox, and `Delete` on a focused message opens
it too. Everyone with the conversation open then sees the line "Removed by <member>" with the time, live.


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
