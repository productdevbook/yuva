# SLA policies

An SLA policy sets how fast your team answers: a time for the first reply, for each next reply and for
resolving a conversation. Yuva counts the time in the inbox's business hours, shows every conversation's
due time and state, warns before a target is missed and tells you when it was.

## 1. Create a policy

Owners and admins create policies, or a key with `inboxes:manage` that is not limited to inboxes:

```sh
curl -X POST https://support.example.com/v1/sla-policies \
  -H "Authorization: Bearer $YUVA_API_KEY" -H "Content-Type: application/json" \
  -d '{"name": "Urgent", "priorities": ["urgent", "high"],
       "first_reply_minutes": 30, "next_reply_minutes": 120, "resolution_minutes": 1440,
       "warn_minutes": 15}'
```

The answer is the policy, `201`. Set at least one of the three targets; a target without minutes is not
tracked. Minutes are 1 to 525600. `warn_minutes` is 0 to 1440 (15 by default; `0` sends no warning).

| Field | Meaning |
|---|---|
| `name` | 1 to 100 characters. |
| `enabled` | `true` by default. A disabled policy never applies. |
| `inbox_ids`, `channel_kinds`, `label_ids`, `priorities` | Conditions. All must hold; inside a list any entry matches; an empty list matches anything. `channel_kinds` are `email`, `chat`, `app` and `api`. |
| `first_reply_minutes`, `next_reply_minutes`, `resolution_minutes` | The targets, in business minutes. |
| `warn_minutes` | How long before a due time the warning is sent. |

`PATCH /v1/sla-policies/{slaPolicyId}` changes what you send; `null` stops tracking a target, and one must
remain. A workspace holds at most 100 policies (`403 sla_policy_limit`).

| Call | Does |
|---|---|
| `GET /v1/sla-policies` | Lists the policies in order: `{"items": [...]}`. |
| `POST /v1/sla-policies` | Creates one; it goes last. |
| `GET`, `PATCH`, `DELETE /v1/sla-policies/{slaPolicyId}` | Reads, changes or deletes one. |
| `PUT /v1/sla-policies/order` | Sets the order: `{"ids": [...]}` names every policy once. |

Reading needs `inboxes:read`, writing and ordering `inboxes:manage`. Agents and keys limited to some
inboxes can read, but only the policies without `inbox_ids` and those naming one of their inboxes (any
other is `404`, and `inbox_ids` lists only what they see). They cannot write or reorder: `403 forbidden`.

## 2. Which policy applies

The first enabled policy in the order whose conditions match the conversation applies. A change of
priority, labels or inbox picks the policy again. A target that already met or breached keeps its outcome.

A new policy applies to conversations that have no SLA when they next change or get a message. It never
starts a target before the policy was created.

## 3. Targets

| Target | Starts | Met by |
|---|---|---|
| `first_reply` | The contact's first message. | A public reply from a member or a bot. |
| `next_reply` | A contact message that follows a reply. | The next public reply. |
| `resolution` | The conversation is created. | The first close. |

Notes, drafts and automatic replies never meet a target. Closing a conversation stops a reply target that
is still running, without an outcome. A reopened conversation gets no new resolution target; the first
close stands. A contact message after a close starts a next-reply target.

## 4. Business hours and time zones

Each target counts only the time the inbox is open, in the inbox's time zone (`timezone` and
`business_hours` of the inbox). With business hours off, or turned on without intervals,
every hour counts. Clock changes are handled: a wall time that does not exist moves forward to the jump.

## 5. Pausing

`pending` and `snoozed` pause every running target and keep the time that was left; `open` resumes it with
that time. A snoozed conversation reopens by itself at its `snooze_until`, which sends `conversation.updated`
and resumes the clock. A spam or trashed conversation stays snoozed.

## 6. When a policy stops applying

Spam, the trash, a policy that is turned off, deleted or no longer matches, and an edit that narrows its
conditions all drop the SLA: `sla` disappears from the conversation and `conversation.updated` is sent.
If another policy matches, it takes over and keeps the business time already used. If a policy applies
again later, the time while none applied does not count, and no target gets a second outcome.

New minutes apply to targets that start afterwards; running targets keep their due times. Deleting a label or
an inbox removes it from the policies that name it, and turns a policy off when that empties its list, so it does not start to match everything.

## 7. Warnings and breaches

- `warn_minutes` before a due time, `conversation.sla_warning` is sent, unless that moment has already
  passed when the target starts or resumes.
- At the due time, `conversation.sla_breached` is sent. A breached target stays breached until it is met
  (`met_at` then records the late reply), the conversation is paused or it closes.
- Each is sent once per target, followed by `conversation.updated` with the new `sla`.

Both go to the [event feed and realtime](headless.md#event-feed-and-realtime) and to
[webhooks](webhooks.md#events). `data` is `{target, due_at, conversation}`; the webhook adds `contact`.

Members are notified too, through two kinds in their [notification settings](#member-notifications).

### Member notifications

| Kind | Default | Sent when |
|---|---|---|
| `sla_warning` | Push | A target is due soon. |
| `sla_breached` | Push, and e-mail at once | A target was breached. |

The assignee is notified; without one, the members of the conversation's team; without either, nobody.
The member must still have access to the inbox. A member who is `away` is still notified of their own
conversations, not of their team's. Members change the kinds with `PATCH /v1/me/notifications` (member
sessions only).

## 8. Read the state

A conversation carries `sla` when a policy applies:

```json
{"policy_id": "…", "state": "due_soon", "due_at": "2026-03-04T10:30:00Z",
 "targets": [{"target": "first_reply", "state": "due_soon", "started_at": "2026-03-04T09:45:00Z",
              "due_at": "2026-03-04T10:30:00Z", "warned_at": "2026-03-04T10:15:00Z"}]}
```

| Field | Meaning |
|---|---|
| `state` | `on_track`, `due_soon` (the earliest running target is due within `warn_minutes`), `breached`, `met` or `paused`. |
| `due_at` | The earliest due time of a running target; absent when none runs. |
| `targets[].target` | `first_reply`, `next_reply` or `resolution`. Only the latest next-reply target is kept. |
| `targets[].state` | `on_track`, `due_soon`, `breached`, `met`, `paused` or `stopped` (closed before a reply). |
| `targets[]` | Also `started_at`, `due_at` while it runs, `remaining_seconds` while paused, `met_at`, `breached_at` and `warned_at`. |

`GET /v1/conversations` takes `sort=sla_due`: conversations with a running target first, the earliest
`sla.due_at` first (breached ones lead), then the rest by recent activity. A cursor continues only the
order it came from. `sla_state` keeps conversations in one or more states (repeat it or separate them by
commas); conversations without an SLA never match.

```sh
curl -G https://support.example.com/v1/conversations \
  -H "Authorization: Bearer $YUVA_API_KEY" \
  --data-urlencode "sort=sla_due" --data-urlencode "sla_state=breached,due_soon"
```

`GET /v1/stats` has `sla`: `met` and `breached` targets in the window, in total, per inbox (`inboxes`)
and per member (`members`, by the assignee at the time; unassigned outcomes count in the totals and per
inbox only).

## Assistants

[MCP](mcp.md) `search_conversations` takes `sla_state` and `sort`, and it and `get_conversation` return
`sla`. No tool manages policies.
