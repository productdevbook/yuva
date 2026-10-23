# Automation rules

An automation rule runs on its own when something happens to a conversation: it checks conditions and
then labels, assigns, replies or calls a webhook for you. Rules are managed with the API under
`/v1/automation-rules`; every example uses an [API key](headless.md#scopes).

## 1. Create a rule

Owners, admins and keys that are not limited to inboxes create rules. This one adds a label and sends a
canned reply to every new e-mail conversation outside business hours:

```sh
curl -X POST https://support.example.com/v1/automation-rules \
  -H "Authorization: Bearer $YUVA_API_KEY" -H "Content-Type: application/json" \
  -d '{"name": "After hours",
       "trigger": "conversation_created",
       "conditions": [
         {"field": "channel_kind", "operator": "in", "values": ["email"]},
         {"field": "business_hours", "operator": "equals", "value": "outside"}
       ],
       "actions": [
         {"type": "add_labels", "label_ids": ["<label id>"]},
         {"type": "send_reply", "canned_reply_id": "<canned reply id>"}
       ]}'
```

The answer is the rule, `201`. It goes last in the order and reacts to what happens after it is saved.
Every label, member, team, canned reply, webhook endpoint, custom field and inbox a rule names must exist
(`400 validation_failed`). A workspace holds at most 100 rules (`403 automation_rule_limit`).

| Field | Meaning |
|---|---|
| `name` | 1 to 100 characters. |
| `enabled` | `true` by default. |
| `inbox_id` | Leave out for a rule on every inbox; set it to act only on that inbox's conversations. |
| `trigger` | When the rule is looked at, see [triggers](#2-triggers). |
| `idle_minutes` | 5 to 43200. Required for `idle`, refused for other triggers. |
| `conditions` | Up to 20; all must match. An empty list matches every conversation. |
| `actions` | 1 to 10. |

`PATCH /v1/automation-rules/{automationRuleId}` changes what you send; `conditions` and `actions` are
replaced whole, and `inbox_id` or `idle_minutes` set to `null` clear them.

| Call | Does |
|---|---|
| `GET /v1/automation-rules` | Lists the rules in order: `{"items": [...]}`. |
| `POST /v1/automation-rules` | Creates one. |
| `GET`, `PATCH`, `DELETE /v1/automation-rules/{automationRuleId}` | Reads, changes or deletes one. Deleting also deletes its log; what it already did stays. |
| `PUT /v1/automation-rules/order` | Sets the order: `{"ids": [...]}` names every rule once. |
| `GET /v1/automation-rules/{automationRuleId}/runs` | The rule's [log](#7-the-run-log). |

## 2. Triggers

| Trigger | Fires when |
|---|---|
| `conversation_created` | A conversation was created, by a contact, a member or a key. |
| `message_received` | The contact wrote a message. Imported messages and drafts do not count. |
| `status_changed` | The status changed, also when a contact's message or the end of a snooze reopened it. |
| `idle` | Nothing was written in an open or pending conversation for `idle_minutes`. |

`idle` is checked every 5 minutes, so a rule fires a few minutes late at most, and once per quiet
period: the next one starts only after someone writes. A rule's own note or reply never starts a new
quiet period. A conversation that was quiet while the server was down for more than 15 minutes is not
picked up. Spam, trashed and closed conversations never count as idle, and a conversation that
was written to or closed before the rule runs is left alone.

## 3. Conditions

A condition has a `field`, an `operator` and what to compare with: `values` for `in` and `not_in`,
`value` for `equals`, `not_equals`, `contains` and `not_contains`, nothing for `is_set` and `is_not_set`.

| Field | Operators | Compares |
|---|---|---|
| `channel_kind` | `in`, `not_in` | `email`, `chat`, `app` or `api`. |
| `kind` | `in`, `not_in` | The conversation kind. |
| `feedback_category` | `in`, `not_in` | Category keys of feedback. |
| `priority`, `status` | `in`, `not_in` | The conversation's priority or status. |
| `subject`, `body` | `contains`, `not_contains` | Case-insensitive. `body` is the message that triggered the rule, or the contact's latest message for other triggers. |
| `contact_attribute` | `equals`, `not_equals`, `contains`, `not_contains`, `is_set`, `is_not_set` | The contact attribute named in `key`. |
| `custom_field` | same | The conversation [custom field](custom-fields.md) with that `key`. Numbers, checkboxes and dates compare as their JSON text (`42`, `true`, `2026-01-31`). |
| `label` | `in`, `not_in` | `in`: has any of the labels. `not_in`: has none of them. |
| `team` | `in`, `not_in`, `is_set`, `is_not_set` | Team ids. |
| `assignee` | `is_set`, `is_not_set` | Whether a member is assigned. |
| `business_hours` | `equals` | `inside` or `outside` the inbox's business hours. An inbox without hours is always inside. |

`equals` is exact; `contains` ignores case. Conditions are checked on the conversation as it is when the
rule runs, so a rule sees what earlier rules changed.

## 4. Actions

| Action | Fields | Does |
|---|---|---|
| `add_labels`, `remove_labels` | `label_ids` (up to 50) | Adds or removes labels. They may not share a label. |
| `assign_member` | `member_id` | Assigns a member who has access to the inbox. |
| `assign_team` | `team_id` | Assigns a team that is granted the inbox. |
| `unassign` | none | Clears the assigned member. |
| `assign_pool` | none | Clears the assigned member; once the trigger's rules have run, [automatic assignment](assignment.md) picks one from the pool. |
| `set_priority` | `priority` | Sets the priority. |
| `set_status` | `status` | `open`, `pending` or `closed`. Closing needs the conversation's required custom fields. |
| `snooze` | `snooze_minutes` (1 to 525600) | Snoozes the conversation. |
| `add_note` | `body` (up to 10000 characters) | Adds an internal note. |
| `send_reply` | `canned_reply_id` | Sends the canned reply's text to the contact, see [replies](#6-replies). |
| `send_webhook` | `webhook_id` | Sends [`automation.triggered`](#8-webhook-action) to that endpoint. |

Each type appears at most once in a rule, with only its own fields (`400 validation_failed` otherwise).
`set_status` and `snooze` exclude each other, and so do `assign_member`, `assign_pool` and `unassign`.

Inside one rule the actions run in a fixed order whatever order you list them: note, reply, then labels,
assignee, team, priority and status or snooze together, then the webhook. Everything a rule does to a
conversation is written to its timeline as the author `system`, and each message carries `rule` with the
rule's `id` and `name` at that time. A run also adds a `rule_applied` event listing the actions and their
results.

## 5. Order and loops

Rules are in one list ordered by `position`; workspace rules and inbox rules share it. For one trigger
the enabled rules that apply run in that order, and each rule at most once per trigger, even when the
server retries: a rule that has already run, matched or not, is never run again for it.

A rule's own changes never trigger a rule, so rules cannot loop: a label it adds, a status it sets or
a message it writes starts nothing. This holds for a snooze a rule set too: when that snooze ends and
reopens the conversation, no rule runs for the wake-up.

## 6. Replies

`send_reply` sends the canned reply's text as an automatic reply (by e-mail with
`Auto-Submitted: auto-replied`). It is refused, and the run fails with `error: auto_reply`, when:

- the message that triggered the rule is an automatic reply or a bounce;
- the contact is blocked;
- the conversation is spam;
- a rule already replied in this conversation within the last hour, or three times in the last 24 hours.

Whether a rule may mail customers also follows the workspace setting `bots_may_send`
([Bots may send](headless.md#bots-may-send)). While it is off, a reply goes out only when both the
rule and the canned reply were last saved by a member who is still an owner or admin when the rule runs;
otherwise the run fails with `bot_sending_disabled`, and `detail` names which one is missing.

Both carry `saved_by` (`type`, and `member_id` or `api_key_id`):

- A rule's `saved_by` is whoever created it or last changed its `trigger`, `idle_minutes`, `inbox_id`,
  `conditions`, `actions` or `enabled`. A rename or a reorder does not change it.
- A canned reply's `saved_by` is whoever created it or last changed its text. It is absent for replies
  saved before it was recorded; an owner or admin must save such a reply again before a rule can send it
  while `bots_may_send` is off.
- API keys and connected apps count as bots, even when an owner made them.

## 7. The run log

Each time a rule is looked at for a trigger, it leaves one run, kept 30 days:

```sh
curl -H "Authorization: Bearer $YUVA_API_KEY" \
  "https://support.example.com/v1/automation-rules/$RULE_ID/runs?outcome=failed"
```

Newest first, with `limit` and `next_cursor` for paging. `outcome` filters by one value.

| Outcome | Meaning |
|---|---|
| `applied` | The conditions matched and the actions were applied. |
| `failed` | An action could not be done. Nothing of that rule was applied. `error` holds the problem code and `detail` a sentence. |
| `not_matched` | The conditions did not match. No actions. |

Each run has `conversation_id`, `trigger` and `actions`: one `type` and `result` per action. A result is
`applied`, `unchanged` (the conversation already was that way), `skipped` (only in old runs) or `not_run`
(the run failed).

Common `error` values:

| Error | Reason |
|---|---|
| `auto_reply` | A reply was refused to protect the customer, see [replies](#6-replies). |
| `bot_sending_disabled` | `bots_may_send` is off and the rule or canned reply was not last saved by an owner or admin. |
| `field_required` | Closing needs a custom field value. |
| `team_access`, `validation_failed` | The team has no access to the inbox, or the member has none. |
| `email_undeliverable`, `email_not_configured` | The reply could not be mailed. |
| `label_gone`, `canned_reply_gone`, `webhook_unavailable` | What the action names was deleted, or the endpoint is disabled or limited to another inbox. |

A rule that names a deleted label, member, team, canned reply or endpoint is not changed: its actions
that need it fail. A condition on a deleted label or team never matches (`in`) or always matches
(`not_in`). Deleting an inbox deletes its inbox rules.

## 8. Webhook action

`send_webhook` sends one `automation.triggered` event to the [webhook endpoint](webhooks.md) it names,
signed like any other delivery. The endpoint does not list it in `events` (it is refused there), and
receives it whatever its `events` say, unless it is disabled or limited to another inbox. Rules store no
URLs or secrets. `data` holds `rule` (`id`, `name`), `trigger`, `conversation` as it is after the rule's
other actions, and `contact`.

## 9. Rate limit

A workspace's rules apply at most 600 actions a minute. When a rule would go over, it waits for the next
minute rather than failing, and later rules wait with it. Rules whose conditions do not match cost
nothing.

## 10. Access

| Scope | Allows |
|---|---|
| `automations:read` | Read rules and their logs. |
| `automations:manage` | Create, change, reorder and delete rules. Owners and admins only; refused for keys limited to inboxes; not implied by any other scope. |

Agents and keys limited to some inboxes can read the workspace rules and the rules of their inboxes (any
other is `404`), and see only runs on conversations in their inboxes. They cannot write: `403 forbidden`.
A rule acts on every inbox it covers with the workspace's power, so grant `automations:manage` with care.

Creating, changing and deleting a rule is recorded in the [audit log](operations.md#audit-log) as
`automation_rule.created`, `automation_rule.updated` and `automation_rule.deleted`, with the rule's name,
trigger, inbox, action types and the hosts of its webhook endpoints, never their URLs.

Automation rules are not available to [MCP](mcp.md) assistants, on purpose.
