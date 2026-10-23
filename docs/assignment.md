# Automatic assignment

Automatic assignment gives a conversation that has no member to someone who is available, so nothing
waits in the unassigned list while the team is online. It works per inbox or per [team](headless.md#teams),
by round robin or by the fewest open conversations, with an optional cap per member.

## 1. Turn it on

Set `assignment_mode` on an inbox or a team. Both take `assignment_cap` too.

```sh
curl -X PATCH https://support.example.com/v1/inboxes/$INBOX \
  -H "Authorization: Bearer $YUVA_API_KEY" -H "Content-Type: application/json" \
  -d '{"assignment_mode": "least_open", "assignment_cap": 20}'
```

| Field | Values |
|---|---|
| `assignment_mode` | `off` (default), `round_robin` or `least_open`. |
| `assignment_cap` | 1 to 500, or absent for no cap. `null` on `PATCH` removes it. |

They are on `Inbox`, `InboxCreate`, `InboxUpdate`, `Team`, `TeamCreate` and `TeamUpdate`
(`PATCH /v1/inboxes/{inboxId}`, `PATCH /v1/teams/{teamId}`, and the matching `POST` calls). Writing
them needs the same permission as the rest of the inbox or team (`inboxes:manage`; an unknown mode or a
cap outside 1 to 500 is `400 validation_failed`).

- `round_robin` gives the conversation to the eligible member who was given one longest ago; a member
  never given one goes first.
- `least_open` gives it to the eligible member with the fewest open conversations, then the one given one
  longest ago.

The cap is the most open conversations a member may hold. It counts the conversations assigned to them
that are `open`, not spam and not in the trash, across the whole workspace. Assigning by hand ignores it.

## 2. Who is eligible

A member is eligible for a conversation when all of these hold:

- they have access to the conversation's inbox through their own grant or a team's; owners and admins
  need a grant too
- the panel or an app is connected for them
- their `availability` is `auto` (`PATCH /v1/me` sets `auto` or `away`)
- the inbox is inside its business hours
- they are under the pool's cap
- for a team's pool, they are in the team

## 3. Which pool

A conversation with a team and no member uses the team's settings and members. One with neither uses the
inbox's. If the team's mode is `off`, nothing is assigned: the inbox's settings are not used instead. A
conversation that already has a member is never touched, except when it is reopened (below).

Conversations are considered when they are created and when a contact reopens them. A conversation
unassigned by hand stays unassigned.

## 4. When nobody is available

The conversation waits. It gets `assign_waiting_since` (a time; absent otherwise) and a
`conversation.updated` is sent. It is assigned, oldest first, as soon as someone becomes eligible: a
member connects or switches `availability` to `auto`, a member frees up by closing or handing on a
conversation, the cap is raised, a pool is turned on, or the inbox's hours open. Yuva also checks every
minute while anyone is online. The field clears when the conversation gets a member, stops being `open`,
is marked spam or is trashed.

## 5. Reopened conversations

When a contact writes into a closed, pending or snoozed conversation, or it wakes from a snooze, it
reopens. The member keeps it while they still have access to the inbox and are not `away`; being offline
or over the cap does not take it away. Otherwise it is unassigned and goes to the pool, or waits if nobody
is eligible. Inboxes and teams with mode `off` keep today's behaviour.

## 6. Events

Assignment is written as the usual `assigned` or `unassigned` timeline event by the author `system`.
The event has `auto: true`, and `previous_assignee_id` when there was one. The assignee gets the
`assigned_to_me` notification, and `conversation.updated` and [webhooks](webhooks.md#events) follow as for
a hand assignment.

## 7. Automation rules

[Automation rules](automation.md) run first. The action `assign_pool` clears the conversation's member
and, once all rules of the trigger have run, hands the conversation to automatic assignment. A rule that
assigns a member, assigns nobody (`unassign`) or uses `assign_pool` also stops a reopened conversation from
keeping its old member.
