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

### Live connection limits

`GET /client/v1/realtime` limits what one visitor or one address can hold open:

| Limit | Value |
|---|---|
| Open connections | 20 per session and 200 per address, on each server |
| New connections | 30 a minute per session and 120 a minute per address; more answer `429 rate_limited` with `Retry-After`, which a browser reports as a closed socket (1006) |

A connection beyond an open-connection limit is accepted and closed at once with code `1013` and the
reason `too_many_connections;retry_after=30`; connections that are open stay. Wait at least that many
seconds, keep backing off while it repeats, and reconnect only when the page or app is in use again.

A connection replays at most 1,000 events and nothing from before its session was created. When more lie
between `last_event_id` and now it sends `resync_required`: reload over HTTP, as for an expired position.

### The visitor's e-mail address

An address a visitor types (`PUT /client/v1/contact/email`, the feedback form, or an unconfirmed `email` in an
[identity token](identity.md)) is kept apart. Nothing but one confirmation link is mailed to it, and replies
are e-mailed only after it is confirmed. `email_confirmation` on the contact says where it stands:

| `state` | Meaning |
|---|---|
| `sent` | A link went out and is neither confirmed nor bounced; `expires_at` is when it runs out (24 hours). |
| `undeliverable` | The link or an earlier mail bounced: ask for another address. |
| `confirmed` | The address is now the contact's `email`. |
| `limited` | No link went out because the address, the visitor's network, the contact or the workspace reached its limit of confirmation mails for now. Say so, and ask the visitor to try later or to use another address; do not say a link was sent. |

A `contact.updated` frame on the live connection carries each change. The limits are 3 a day per address from
one network, 10 a day per address on the server, 6 a day per contact, 30 a day per network and workspace and
2,000 a day per workspace.

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

The setting holds for every assistant, not only keys: a token of a connected app (an [OAuth](mcp.md#oauth)
client) delivers as drafts unless it is on, whichever resource the token was issued for. Only the server's own
member apps send as the member whatever it says; the operator lists them in
[`YUVA_OAUTH_FIRST_PARTY_CLIENTS`](configuration.md#server), and the list is empty by default.

[Automation rules](automation.md) reply to customers with a canned reply. A canned reply has `saved_by`: who
last set its text. While **Bots may send** is off, a rule sends it only when an owner or admin saved it.

## Teams

A team is a named group of members that is granted inboxes. A member reaches an inbox through their
own grant, through any team they belong to, or both: access is the union, so removing one grant
changes nothing while another still gives the inbox. A conversation can be assigned to a team as well
as to a member.

```sh
curl -X POST https://support.example.com/v1/teams \
  -H "Authorization: Bearer $YUVA_API_KEY" -H "Content-Type: application/json" \
  -d '{"name": "Billing", "description": "Invoices and refunds"}'
curl -X PUT https://support.example.com/v1/teams/$TEAM/inboxes/$INBOX -H "Authorization: Bearer $YUVA_API_KEY"
curl -X PUT https://support.example.com/v1/teams/$TEAM/members/$MEMBER -H "Authorization: Bearer $YUVA_API_KEY"
```

| Call | Does |
|---|---|
| `GET /v1/teams` | Lists teams by name: `{"items": [...]}`. |
| `POST /v1/teams` | Creates one (`name` 1 to 64 characters, unique: `409 team_taken`; optional `description`, up to 500). `201`. |
| `GET`, `PATCH`, `DELETE /v1/teams/{teamId}` | Reads, renames or describes (`PATCH` needs at least one field), or deletes a team. |
| `PUT`, `DELETE /v1/teams/{teamId}/members/{memberId}` | Adds or removes a member. `204`; repeating it changes nothing. |
| `PUT`, `DELETE /v1/teams/{teamId}/inboxes/{inboxId}` | Grants or revokes an inbox. `204`; repeating it changes nothing. |
| `GET /v1/inboxes/{inboxId}/members` | Who has access to an inbox. Each member has `via` (`direct`, or `team` when only teams give it) and `team_ids`, the teams that give the inbox. |

A team and an inbox can also hand out conversations by themselves: see [Automatic assignment](assignment.md).

A team has `id`, `name`, `description`, `member_ids`, `inbox_ids` (both in the order added),
`created_at` and `updated_at`. At most 500 members and 100 inboxes per team (`403 team_limit`); an
unknown team, member or inbox is `404`.

Reading needs `inboxes:read`; writing needs `inboxes:manage` and an owner, admin or key not limited to
inboxes (agents get `403 forbidden`). Owners, admins and unlimited keys see every team; an agent sees
the teams they belong to and the teams granted an inbox they see; a key limited to inboxes sees the
teams granted one of those inboxes. A team you cannot see is `404`, and the `inbox_ids` you get list
only the inboxes you can see.

Removing the direct grant of a member (`DELETE /v1/inboxes/{inboxId}/members/{memberId}`) does not
remove what a team gives them; take the member out of the team or revoke the team's inbox instead.

### Assigning to a team

`PATCH /v1/conversations/{conversationId}` takes `team_id` (a team id, or `null` to clear it) next to
`assignee_id`. The team must hold the conversation's inbox, otherwise the call answers
`422 team_access`; so does a team deleted or revoked while the call runs. Moving a conversation with
`POST /v1/conversations/{conversationId}/move` keeps its team only if the team holds the target
inbox. Deleting a team, or revoking its inbox, clears `team_id` on the conversations concerned, each
with a `conversation.updated`. The conversation carries `team_id` when it has one; it is a separate
field, and `assignee=unassigned` still means "no member" whatever the team. Bulk update and
`POST /v1/conversations` do not take a team.

| List filter | Shows |
|---|---|
| `GET /v1/conversations?team_id=<id>` | Conversations assigned to this team. |
| `GET /v1/conversations?team=mine` | Conversations assigned to a team of the calling member. Member sessions only; a key gets `400`. |

`GET /v1/conversations/counts` has `teams` (open conversations that are not spam, per team id) and
`mine_teams` (open conversations assigned to any team of the caller; `0` for keys).

### Events

- `team.created`, `team.updated` and `team.deleted` go to the [event feed and realtime](#event-feed-and-realtime)
  and to [webhooks](webhooks.md#events). `team.updated` follows a rename, a new description and every
  member or inbox change; members taken out of the team are in `removed_member_ids`. They need
  `inboxes:read` and reach callers who see every inbox, the team's members and callers who see one of
  its inboxes.
- `inbox_access.changed` is sent once for each member and inbox whose access flipped, whether the
  cause was a direct grant, a team or its removal. A change that leaves access as it was sends none.
- Assigning a team shows in `conversation.updated`, whose payload carries `team_id`.
- The [audit log](operations.md#audit-log) records `team.created`, `team.deleted`,
  `team.member_added`, `team.member_removed`, `team.inbox_access_granted` and `team.inbox_access_revoked`.

[MCP](mcp.md) assistants use `list_teams`, `assign` with `team_id` or `remove_team`, and
`search_conversations` with `team_id`.

## Automation rules

Triggers, conditions, actions and a log of every run: see [Automation rules](automation.md). Manage them with
`/v1/automation-rules` (`automations:read` to read, `automations:manage` to write).

## SLA policies

Response-time targets with business hours, warnings and breaches: see [SLA policies](sla.md). Manage them
with `GET`, `POST /v1/sla-policies`, `GET`, `PATCH`, `DELETE /v1/sla-policies/{slaPolicyId}` and
`PUT /v1/sla-policies/order` (`inboxes:read` to read, `inboxes:manage` to write; owners, admins and keys
not limited to inboxes). Conversations carry `sla`, the list takes `sort=sla_due` and `sla_state`, and
`conversation.sla_warning` and `conversation.sla_breached` reach the event feed and webhooks.

## Saved views

A saved view is a named set of conversation list filters. Personal views belong to one member; workspace views are
shared with everyone; team views are shared with one team.

```sh
curl -X POST https://support.example.com/v1/views \
  -H "Authorization: Bearer $YUVA_API_KEY" -H "Content-Type: application/json" \
  -d '{"visibility": "workspace", "name": "Open Pro refunds",
       "filters": {"status": "open,pending", "q": "refund -spam", "field.plan": "pro"}}'
```

| Call | Does |
|---|---|
| `GET /v1/views` | Your personal views, the workspace's views and the views of your teams, each by `position` and name: `{"items": [...]}`. With `?team_id=` only that team's views, for its members, owners, admins and keys not limited to inboxes (others get `404`). |
| `POST /v1/views` | Saves one: `visibility` (`personal`, `workspace` or `team`), `name`, `filters`, `team_id` for a team view, optional `position`. `201`. |
| `GET`, `PATCH`, `DELETE /v1/views/{viewId}` | Reads, changes or deletes a view. `PATCH` takes `name`, `filters` (replaces the whole set) and `position`; visibility and team cannot change. |
| `PUT /v1/views/order` | `{"ids": [...]}`, 1 to 50 views: each gets the position of its index. Every view must be visible to you (`404`) and changeable by you (`403`); otherwise nothing moves. `204`. |

A view has `id`, `visibility`, `owner_member_id` (personal only), `team_id` (team only), `name`, `filters`,
`position`, `count`, `count_capped`, `count_unavailable`, `created_at` and `updated_at`.

### Filters

`filters` is an object of the query parameters of `GET /v1/conversations` and their values as strings, for example
`status`, `label_id`, `assignee`, `team`, `q` and `field.<key>`. `cursor` and `limit` are not filters. A filter is
checked as the list checks it, for the caller who saves it: an unknown name, a value the list refuses, or a label or
team that does not exist answers `400 validation_failed` with `field` naming the parameter. A view holds at most 30
filters of 2000 characters. `assignee=me`, `team=mine` and `pinned` resolve for whoever reads the view.

### Counts

Each view carries `count`: its open conversations under its filters, counted with the reader's own inbox access, so a
shared view never counts what the reader cannot see. A status filter without `open` counts 0.

| Field | Meaning |
|---|---|
| `count` | At most 1000. Absent when the filters do not apply to the reader (an inbox they cannot see, `assignee=me` for an API key) and when counting failed. |
| `count_capped` | `true` when more than 1000 matched. |
| `count_unavailable` | `true` when counting took longer than a second. |

### Who may do what

| Who | May |
|---|---|
| Any member, signed in | Manage their own personal views and read the workspace views and those of their teams. |
| Owners and admins | Also manage workspace and team views. Agents get `403 forbidden`. |
| Connected apps (OAuth) | Personal views with `views:write`; workspace and team views for owners and admins with `views:write`. |
| API keys | Workspace views, and team views when not limited to inboxes, with `views:write`. Personal views: `403 member_session_required`. |

Team members see their team's views but cannot change them. Another member's personal view answers `404`.
A key with only `views:write` gets `403 insufficient_scope` on every views endpoint: add `conversations:read`.

A member keeps at most 50 personal views, the workspace 50 workspace views and each team 50 team views
(`403 view_limit`).

### When something is deleted

Deleting a label, a team or a custom field removes `label_id`, `team_id` or `field.<key>` from every view that used
it; the rest of each view stays. Renaming changes nothing. Deleting a team deletes its team views, and removing a
member deletes their personal views. A view that names a deleted inbox, contact or idea stays and counts 0 or has no
`count`.

## Custom fields

Typed values on conversations and contacts, filters on the list, and a rule that blocks closing without one: see [Custom fields](custom-fields.md). The key needs `fields:manage` to define them.

## Macros

A macro is a saved reply plus actions: labels to add or remove, an assignee, a status, a snooze or a
priority. Sending a message with a macro writes the message and applies the actions in one step. A
macro is `personal` (yours alone) or `shared` (the whole workspace).

```sh
curl -X POST https://support.example.com/v1/macros \
  -H "Authorization: Bearer $YUVA_API_KEY" -H "Content-Type: application/json" \
  -d '{"visibility": "shared", "name": "Refund done", "shortcut": "refund-done",
       "body": "Your refund is on its way.",
       "actions": {"add_labels": ["'$LABEL'"], "status": "closed"}}'

curl -X POST https://support.example.com/v1/conversations/$CONVERSATION/messages \
  -H "Authorization: Bearer $YUVA_API_KEY" -H "Content-Type: application/json" \
  -d '{"kind": "message", "direction": "out", "body": "Your refund is on its way.", "macro_id": "'$MACRO'"}'
```

| Call | Does |
|---|---|
| `GET /v1/macros` | Your personal macros first, then the shared ones, each by `position` and name: `{"items": [...]}`. A key sees the shared ones only. |
| `POST /v1/macros` | Creates one: `visibility`, `name`, `shortcut`, and a `body`, `actions` or both; optional `position` (0 to 10000). `201`. |
| `GET`, `PATCH`, `DELETE /v1/macros/{macroId}` | Reads, changes or deletes a macro. `PATCH` takes `name`, `shortcut`, `body` (`null` removes the text), `actions` (replaces the whole set) and `position`; the visibility cannot change. |

A macro has `id`, `visibility`, `owner_member_id` (personal only), `name`, `shortcut`, `body`, `actions`,
`position`, `created_at` and `updated_at`. `actions` may hold:

| Action | Effect |
|---|---|
| `add_labels`, `remove_labels` | Label ids, up to 50 each; a label cannot be in both. A deleted label drops out of the macro. |
| `assignee_id` | A member to assign; `null` unassigns. A removed member drops out of the macro. |
| `status` | `open`, `pending`, `snoozed` or `closed`. `snoozed` needs `snooze_minutes`. |
| `snooze_minutes` | 1 to 525600; snoozes from the moment the message is sent and implies `status: snoozed`. |
| `priority` | The conversation's priority. |

A shortcut is unique among the shared macros and among each member's personal ones
(`409 shortcut_taken`); it may equal a canned reply's. Unknown labels, an assignee who is not a member,
the same label added and removed, `snoozed` without `snooze_minutes`, or a macro with neither body nor
actions answer `400 validation_failed`.

| Who | May |
|---|---|
| Any member, signed in | Read, create, change and delete their own personal macros and read the shared ones. |
| Owners and admins | Also create, change and delete shared macros. Agents get `403 forbidden`. |
| Connected apps (OAuth) | Personal macros with `conversations:write`; shared macros with `canned_replies:write`. |
| API keys | Read shared macros with `conversations:read`; manage them with `canned_replies:write` (`403 insufficient_scope` without it). Personal macros: `403 member_session_required`. |

Another member's personal macro, and a macro of another workspace, answer `404`.

### Sending with a macro

`POST /v1/conversations/{conversationId}/messages` takes two optional fields (as a JSON string
`then` in a multipart request):

| Field | Meaning |
|---|---|
| `macro_id` | Apply this macro's actions with the message. Shared macros work for keys. |
| `then` | A `ConversationUpdate` applied after the macro, field by field: `then.status` replaces the macro's status and snooze, `then.labels` replaces the labels (the macro's additions and removals included). |

The message and the changes are one transaction. If the message is refused (invalid mention, delivery
refusal) nothing is applied; if an action is refused (an assignee in `then` without access to the inbox,
a snooze in the past) the message is not written either. The exception is the macro's own assignee: when
that member was removed or has lost access to the inbox, the reply and the other actions go through and
the assignee is skipped. Both fields need `conversations:write` for keys and tokens, are refused on a
draft (`400 validation_failed`), and are not applied again when a repeated `client_id` returns the
stored message. A snooze counts from the server time of the send.

The conversation's timeline gets a `macro_applied` event right after the message, with `macro` (`id`,
`name`) and, when the assignee was skipped, `skipped: ["assignee"]`, then the usual `assigned`,
`status_changed` and `labels_changed` events. Priority changes have no timeline event.

[MCP](mcp.md) assistants read macros with `list_macros`; tools never apply them. Use a macro's `body`
with `draft_reply`.

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
| `fields:manage` | Create, change and delete [custom field](custom-fields.md) definitions. Owners and admins only; a key limited to inboxes cannot hold it. Not included in any other scope: name it. Writing values needs `conversations:write` or `contacts:write`. |
| `views:write` | Save, change, reorder and delete [saved views](#saved-views). Every views endpoint, reads included, also needs `conversations:read`. |
| `automations:read` | [Automation rules](automation.md) and their logs |
| `automations:manage` | Create, change, reorder and delete automation rules. Owners and admins only; a key limited to inboxes cannot hold it. Not included in any other scope: name it. |
| `reports:read` | [Reports](reports.md) and their CSV export, over the inboxes the caller can see |
| `integrations:read` | `GET /v1/issue-integrations` and one integration: the [GitHub and Linear](issue-trackers.md) connections, never a token. |
| `integrations:manage` | Connect, change and disconnect GitHub and Linear, make a new webhook secret. A key limited to inboxes cannot hold it. |
| `issues:write` | Create, link and unlink GitHub and Linear issues on feedback and ideas (`/v1/conversations/{id}/issues`, `/v1/discussions/{id}/issues`), together with `conversations:read`. |
| `feedback:write` | `POST /v1/feedback` |

### OAuth clients

| Rule | Detail |
|---|---|
| Resource | A client other than a member app may ask only for `resource=<YUVA_PUBLIC_URL>/mcp`. Asking for `<YUVA_PUBLIC_URL>`, the whole API, is refused on the redirect with `error=invalid_target`; the server does not fall back to `/mcp`. |
| Earlier grants | A grant for `<YUVA_PUBLIC_URL>` made before 0.1.0 by such a client keeps its `/v1` access but sends like any assistant, as drafts unless **Bots may send** is on. |
| `redirect_uri` | If the authorization request carried one, the token request must repeat it exactly, else `400 invalid_grant`. |
| Presence | A member counts as online (for automatic assignment and presence) only through their own session or a member app, never through an API key or an assistant. |

### Sign-in sessions

A member sees and ends their own sessions, with a member session (keys and connected apps get
`403 member_session_required`).

| Call | Does |
|---|---|
| `GET /v1/me/sessions` | `{"items": [...]}`, most recently seen first. Each has `id`, `method` (`code`, `passkey` or `sso`), `current`, `device` (such as `Firefox on Linux`, absent when unknown), `user_agent`, `created_at`, `last_seen_at` (updated at most once a minute) and `expires_at`. |
| `DELETE /v1/me/sessions` | Ends every other session of the person, with their push subscriptions and reader tokens, and answers `{"revoked": n}`. Their live updates close within a minute. |

Removing a passkey (`DELETE /v1/me/passkeys/{passkeyId}`) ends the other sessions too. A session made through
single sign-on lists only itself and cannot end the others (`403 sso_session_scope`).

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
