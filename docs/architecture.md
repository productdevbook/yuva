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

Not goals: a CRM, a marketing automation suite (journeys, A/B tests, lead scoring), social-media channels
(WhatsApp, Instagram, …). They may come later; the model must not block them. Yuva does not host
product documentation, but it collects feedback and questions on the documentation sites products already
have (see "Documentation pages"). Since M13 it sends broadcasts (newsletters, announcements) to
contacts who subscribed, and replies to them land in the inbox (see "Broadcasts and e-mail designs").
Since M15 it measures the workspace's own websites and apps without cookies, for the team and in
aggregate with support (see "Analytics"). Since M20 it hosts help centers, the support
articles of a product next to the documentation on its own site (see "Help center").

## Concepts

| Concept | Meaning |
|---|---|
| Workspace | The tenant. A self-hosted install usually has one. Owns everything below. |
| Member | A person who answers. Role `owner`, `admin` or `agent`. Owners and admins see every inbox; agents see the inboxes they were granted, and only contacts with a conversation or external id in one of them, plus contacts with neither that they created themselves (`contacts.created_by`; a contact nobody in an inbox has touched yet, such as one the feedback API made, is seen by owners and admins only); they set external ids only in their inboxes (ids in other inboxes are kept) and change e-mail addresses only of contacts that appear in no other inbox (`403 forbidden`). An inbox, conversation, contact or attachment the caller cannot see answers `404`. |
| Inbox | One product or brand. Branding, languages, business hours, mode (`live` or `async`), expected reply time, identity secret, webhooks. |
| Channel | How messages reach an inbox: `email`, `chat` (web widget), `app` (mobile SDKs), `api` (server-to-server, e.g. a feedback form). An inbox has any number of channels. |
| Contact | The person writing in. Known by an external user id from the host app (per inbox), one or more e-mail addresses (unique within the workspace), or an anonymous visitor id. Contacts belong to the workspace, not to an inbox; a contact lists the inboxes it has a conversation or external id in (cut to the inboxes the reader may see), and the contact list filters by inbox. |
| Conversation | A thread between a contact and an inbox. Status `open`, `pending`, `snoozed`, `closed`; assignee; labels; priority; the channel it started on. |
| Message | One entry in a conversation: `message` (to or from the contact), `note` (members only) or `event` (assigned, closed, …). Has attachments and a per-channel delivery state. |

Feedback is a conversation that starts on the `app` or `api` channel with a `feedback` kind and
metadata (category, app version, build, OS, OS version, device model, locale, screen, installation
id). An inbox owns its categories (`inboxes.feedback_categories`, migration 00093): 1 to 20 entries
of `key` (`^[a-z][a-z0-9_]{0,31}$`), `names` (language tag to name) and `icon` (a lowercase Lucide
name), in display order, always including `other`. Every inbox, existing and new, starts with `bug`,
`idea`, `praise` and `other`. Owners, admins and keys with `inboxes:manage` replace the list with
`PATCH /v1/inboxes/{id}` (`feedback_categories`, also on the `Inbox` body; `inbox.updated` is emitted).
`POST /client/v1/feedback` (no category means `other`) and `POST /v1/feedback` answer `400` for a
category the inbox does not list. A category that leaves the list moves its feedback to `other` in
the same transaction (the stored `feedback.category` changes, so listings, filters, counts, webhooks
and MCP all agree; re-adding the key does not bring it back). `/client/v1` returns
`feedback_category_items`, `{key, name, icon}` for every category with `name` in the request's
`Accept-Language` (by q-weight, else the inbox's default locale, then `en`), so the SDKs never
hard-code names. Clients built before custom categories keep working: the legacy
`feedback_categories` there lists only the keys among the four built-in ones, and a feedback's
`category` on `/client/v1` is its key if built-in, else `other`; `category_key` and `category_name`
carry the real key and its name. `/v1` and webhooks show the real key. The category is checked
inside the insert transaction under `FOR SHARE` on the inbox row, which conflicts with the update's
`FOR UPDATE`, so no feedback is stored under a key that has just been removed. Counts and the `category` filter work on any key. Apps send it
with `POST /client/v1/feedback` (JSON, or multipart with screenshots); host backends whose forms
are server-rendered send it with an API key to `POST /v1/feedback`, naming the user by their
external id, into any inbox of the key's workspace. The conversation starts on the inbox's `api`
channel, which the first such request creates (named `API`) when the inbox has none. It is
answered like any other conversation; the answer reaches the contact in the app, or by e-mail when
they allowed it (`allow_email`): members' replies the contact has not read go out through the same
delayed e-mail as chat replies. The panel
filters conversations by `kind` and feedback `category` and counts open feedback per category.
A feedback may carry a `score`, an integer 1 to 5 (`POST /client/v1/feedback`, `400` outside the range), kept in the
`feedback` JSON next to the category (a check constraint holds the range, migration 00079) and returned wherever
feedback is: the conversation and feedback listings, `feedback.score` in webhook payloads and `feedback_score` in
the MCP `list_feedback` tool. Feedback sent without one, or before it existed, has none.

Feedback has its own place (M18). It stays a conversation, so replies, notes, assignment, e-mail
fallback, events and webhooks are the ones every conversation has; only where it is shown and what it
is called change. Decisions:

- **Panel.** A Feedback entry in the rail (`/feedback`, its own shortcut, a badge with open feedback
  from the `feedback` count) opens a list with Open and Resolved tabs and category tabs with their
  open counts (All, then the inbox categories, Bug, Idea, Praise and Other by default), filtered by inbox like the inbox. A row is a triage
  line, not a chat preview: category, the first line of the text, the contact, app version, OS and
  device (or the page's title for page feedback), a mark when screenshots are attached, a mark when
  the team has answered, and the time. Opening a row shows the existing conversation view beside the
  list. The Conversations view, its counts and its unread badge show every kind except `feedback`,
  and its source filter loses the Feedback option. `GET /v1/conversations/counts` takes `kind` like
  the list does.
- **Resolved.** For a feedback, `closed` is called Resolved everywhere a person reads it (panel,
  SDKs, widget); there is no new status. The panel's Close and Reopen read Resolve and Reopen, and
  Send and close reads Reply and resolve. A contact who writes to a resolved feedback reopens it, as
  any conversation: a fix that did not work comes back to the same thread.
- **No rating.** Feedback never asks for a satisfaction rating: `can_rate` is false for `kind`
  `feedback`, and ratings reports leave it out.
- **Contact side.** `GET /client/v1/conversations` takes `kind` (several values, like `/v1`). The
  Swift and Kotlin conversation lists show Messages and Feedback as two tabs when the contact has
  sent feedback, one list otherwise. A feedback row shows its category, the first line and a state:
  Sent (no answer yet), Answered (the team's message is the last one), Resolved. The thread of a
  feedback shows the category and state in its header and, when resolved, a line saying the team
  marked it resolved, with the composer still open. The feedback form's sent screen has View
  feedback next to Done, which opens that thread.
- **Progress.** A member can tell the contact where a feedback stands: `feedback.progress` is one of
  `planned`, `in_progress`, `done`, `wont_do`, or absent when nobody set it. It is separate from the
  status: a feedback can be `done` and still open, or resolved with no progress. Members set and clear
  it in the panel's feedback header and on the conversation update; each change is a conversation
  event (`feedback_progress`) in the timeline, the contact sees it as a line in the thread, and the
  contact's row shows the progress (Planned, In progress, Done, Won't do) instead of Sent or Answered
  while it is set. The panel's feedback row shows it as a chip. Progress never hides the resolved
  line: a resolved feedback's thread shows that line whether or not a progress is set, and the
  header keeps the progress tag.
- **SDK look.** The Swift and Kotlin views look like a native messaging app, not a settings list: rows
  edge to edge with a round avatar or category icon, a short time (14:32, Yesterday), unread in bold
  with a dot, the team name and reply time as the title's subtitle, the feedback state as a small
  colored tag, a thread of bubbles with the composer riding the keyboard. There is no theming API;
  apps that want their own look build on `YuvaClient`.

Members change up to 100 conversations in one request (`POST /v1/conversations/bulk`: status,
assignee, labels to add and remove); each is checked against the caller's inbox access and
changed in its own transaction with the same events and webhooks as a single change, and the
answer lists which changed and which were refused. A member who sees both inboxes moves a
conversation to another inbox of the workspace (`POST /v1/conversations/{id}/move`) with its
messages, notes, labels and attachments; the assignee stays only with access to the new inbox,
replies continue on the new inbox's channel of the same kind, and an e-mail conversation is
refused when the new inbox has no e-mail channel. Owners, admins and API keys merge one contact
into another (`POST /v1/contacts/{id}/merge`): the target keeps its id and gains the source's
addresses, external ids, attributes (its own value wins) and conversations; the source is deleted,
so identity tokens and mail for its keys find the target.

### Team workflow (M28)

Planned; built in the order teams and macros, SLA and custom fields, automation and automatic
assignment, saved views. Snoozed conversations wake on the server at `snooze_until`; a rule made by
an owner or admin may reply to customers while `bots_may_send` is off, one made by an API key only
while it is on; a reopened conversation gets no new resolution target.

**Teams (#138).**
- **Tables.** `teams (workspace_id, id, name unique per workspace, description, assignment_mode, assignment_cap, created_at, updated_at)`, `team_members (workspace_id, team_id, member_id)` and `team_inboxes (workspace_id, team_id, inbox_id, created_at)`. All foreign keys are composite `(workspace_id, …)` with `ON DELETE CASCADE`.
- **Access.** A member's inbox access is the union of their own grants and their teams' grants. A migration adds the view `member_inbox_access` (`inbox_members ∪ team_inboxes⋈team_members`) and redefines `inbox_viewers` as `member_inbox_access ∪ api_key_inboxes`. Every query on `inbox_viewers` follows without change.
- **Switching the seven direct readers.** `HasInboxAccess`, `ListNotificationCandidates`, `ListInboxNotifications`, `ListAvailableMembers` and `ListAnalyticsSubscriberAddresses` move to `member_inbox_access`. `ListViewerInboxIDs` gains `DISTINCT`. `ListInboxMembers` returns each member with `via: direct | team` and the team ids.
- **No raw reads.** A test greps `queries/*.sql` and fails on any `FROM inbox_members` outside the view and the grant and revoke queries.
- **Assigning to a team.** `conversations.team_id` (nullable, `ON DELETE SET NULL`). A conversation has a team, a member, both or neither. A team can be set only if it holds a grant on the conversation's inbox; otherwise `422 team_access`. Moving a conversation keeps `team_id` only when the team can see the new inbox, the same rule as the assignee.
- **List filters.** `team_id=<id>` and `team=mine` (any team of the caller); "Unassigned" stays `assignee_id IS NULL`. `/v1/conversations/counts` gains `teams` (open per team) and `mine_teams`.
- **Endpoints.** `/v1/teams` (CRUD), `PUT|DELETE /v1/teams/{id}/members/{member_id}`, `PUT|DELETE /v1/teams/{id}/inboxes/{inbox_id}`. Reads need `inboxes:read`; writes need `inboxes:manage` (owners, admins, keys not limited to inboxes).
- **Who sees teams.** Agents and inbox-limited keys see the teams they belong to and the teams granted an inbox they see. Owners and admins see all.
- **Revocation takes effect at once.** Every change that alters a member's effective access diffs `member_inbox_access` for the affected members inside the same transaction. It emits one `inbox_access.changed` per `(member, inbox)` whose effective access flipped (existing schema), so open realtime connections reload their inbox set. Removing a member deletes their `team_members` rows by cascade. The heartbeat re-check already closes their connections.
- **Events and audit.** Events `team.created`, `team.updated` and `team.deleted` (payload carries `member_ids` and `inbox_ids`) go to realtime, to webhooks, and to members named in the payload or who see all inboxes. Audit actions: `team.created`, `team.deleted`, `team.member_added`, `team.member_removed`, `team.inbox_access_granted`, `team.inbox_access_revoked`.

- **As built (#138, migrations 00101 and 00102).** `member_inbox_access` has a fourth column `team_id`
  (null for a member's own grant), so `ListInboxMembers` reads `via` and `team_ids` from the view and
  only the grant and revoke queries touch `inbox_members` (`TestNoRawInboxMembersReads` checks
  queries, later migrations and Go strings). Every access change takes a per-workspace advisory lock
  first, then diffs; a direct revoke that a team still covers sends no `inbox_access.changed`.
  Deleting a team or revoking its grant on an inbox clears `team_id` on the affected conversations in
  the same transaction, one `conversation.updated` each, no timeline entry; wherever a team is set or
  kept the team row is locked `FOR SHARE` before the conversation, so a team deleted or revoked
  mid-change answers `422 team_access`. Team payloads list only the `inbox_ids` the receiver sees;
  `team.updated` carries `removed_member_ids` and reaches removed members. Bulk update and conversation
  create take no `team_id`; MCP `assign` takes `team_id` or `remove_team`. The assignment columns on
  `teams` exist and #139 exposes them. `team.updated` is not audited.

**Automatic assignment (#139).**
- **Settings.** `inboxes.assignment_mode` and `teams.assignment_mode` (`off | round_robin | least_open`), and `assignment_cap` (1–500, null for no cap).
- **Turn tracking.** `assignment_turns (workspace_id, pool_id, member_id, last_at)` holds round-robin turns. `pool_id` is the inbox id or the team id.
- **Eligible members.** A member is eligible when all of these hold:
  - they have access to the inbox (`member_inbox_access`);
  - they have a fresh realtime connection (`presenceFresh`) and `availability = 'auto'`;
  - the inbox is inside its business hours (`inboxOpen`);
  - they are under the cap;
  - for a team pool, they are a member of the team.
- **Cap.** The cap counts the member's `open`, non-spam, untrashed conversations across the workspace. Manual assignment ignores the cap; the hand-off menu shows "over cap".
- **Which pool.** A conversation with a team and no member uses the team's pool. One with neither uses the inbox's pool. One with `assignee_id` set is never touched.
- **The job.** `auto_assign {workspace_id, conversation_id}` (unique by args) runs after the automation job for the same trigger, or directly when the workspace has no enabled rule for it. Rules decide first.
- **Nobody eligible.** `conversations.assign_waiting_since` is set; a partial index `(workspace_id, assign_waiting_since) WHERE assign_waiting_since IS NOT NULL` covers it.
- **Picking up waiting conversations.** `assign_waiting {workspace_id}` (unique while pending) takes waiting conversations oldest first, 50 per run. It is queued when:
  - a member connects or sets availability to `auto`;
  - a member's conversation leaves `open` or is unassigned;
  - the per-minute sweep finds an inbox whose business hours just opened.
- **Reopen.** If the assignee still has inbox access and is not `away`, they keep the conversation; otherwise it is unassigned and enters the pool.
- **Events.** An automatic assignment writes the same `assigned` timeline event and `conversation.updated` as a manual one, with `auto: true`, and notifies like a manual assignment.

- **As built (#139, migrations 00115 and 00116).** Settings are on inboxes and teams with the same
  permission as their other fields; turning a pool on or widening its cap queues the waiting run.
  Round robin picks the member with the oldest turn (never-turned first, ties by id); least open
  picks the fewest open, then by turn; every automatic assignment writes a turn, strictly increasing
  per pool. A team with no member uses only the team's pool (team off assigns nothing). Pool entries
  are a non-imported `conversation.created` and a reopen written by `system`; when an automation run
  is queued for the same conversation it carries the assignment and queues `auto_assign` after its
  last rule, and a rule that assigned, unassigned or handed to the pool decides instead of the
  reopen check. Each automatic assignment takes the per-workspace advisory lock, then the
  conversation, then the event lock; the waiting run reads 50 ids per page to the end and decides
  each conversation in its own short transaction, remembering an empty pool per (pool, inbox). The
  load count reads a partial index on open, non-spam, untrashed conversations per assignee.
  `assign_waiting_since` is set once and cleared when the conversation gets a member, leaves `open`,
  is marked spam, moved with a member or trashed; a manual unassign doesn't put it in the pool. The
  per-minute sweep queues the waiting run for every workspace with a waiting conversation and a member
  online and available, not only when hours open, since River skips a trigger while the same run is
  in progress. On reopen the assignee keeps it while they still have inbox access and aren't away
  (online and cap not required); only when the conversation's pool is on. While the rules job is held
  by the rate limit, its new conversation is marked waiting but held for the rules. Manual assignment
  and contact reopens take no assignment lock: a race ends as if the automatic one ran first.

**SLA policies (#140).**
- **Policies.** `sla_policies (workspace_id, id, name, position, enabled, inbox_ids uuid[], channel_kinds text[], label_ids uuid[], priorities text[], first_reply_minutes, next_reply_minutes, resolution_minutes, warn_minutes default 15)`. An empty array matches anything; at least one target is required. The first enabled policy by `position` whose conditions match applies.
- **Per-conversation state.** `conversation_sla (workspace_id, conversation_id PK, policy_id, assignee_id_at_outcome, team_id_at_outcome)`, plus for each target `t ∈ {first_reply, next_reply, resolution}`: `t_due_at`, `t_remaining_s` (business seconds left while paused), `t_met_at`, `t_breached_at`, `t_warned_at`, and `paused_at`. The list's `sla_due_at` is `least(open due times)`, stored on `conversations` (indexed `(workspace_id, sla_due_at) WHERE sla_due_at IS NOT NULL`) so the list sorts by `sort=sla_due`.
- **Clocks.** Every target counts in the inbox's business hours and timezone, and in plain time when hours are off. The arithmetic is one Go function `addBusinessSeconds(inbox, from, secs)`.
  - First reply: starts at the first contact message.
  - Next reply: starts at a contact message that follows a team reply.
  - Resolution: starts at creation.
- **What meets a target.** A member's or a bot's sent public reply meets first and next reply. Notes, drafts, system auto-replies and rule replies never do. Resolution is met at the first close.
- **Reopen.** A reopened conversation runs no new resolution target: the outcome of the first close stands. Only next reply applies. 
- **Pausing.** `pending` and `snoozed` pause every running target: `t_remaining_s` is stored and `t_due_at` cleared. `open` resumes them with `t_due_at = addBusinessSeconds(now, t_remaining_s)`.
- **Snooze wake.** New per-minute job `snooze_wake`. Per workspace it runs `UPDATE … SET status='open' WHERE status='snoozed' AND snooze_until <= now() … FOR UPDATE SKIP LOCKED LIMIT 500`. Each change emits `conversation.updated` and a `status_changed` event and resumes the clock.
- **Re-evaluation.** A change of priority, labels or inbox (move) picks the policy again. A different policy recomputes due times from the original start instants; outcomes already met or breached stay as they are.
- **Exclusions.** Spam and trash drop the row. `mail` inboxes (M34) never get one.
- **Breach detection.**
  - When a due time is set, the same transaction queues `sla_check {conversation_id, target, due_at}` with `ScheduledAt = due_at`, and a warning check at `due_at − warn_minutes`.
  - The job acts only when the stored `t_due_at` still equals its `due_at`: `UPDATE … SET t_breached_at = now() WHERE … AND t_breached_at IS NULL AND t_met_at IS NULL AND t_due_at = $due RETURNING`. Stale jobs do nothing, and each breach fires once.
  - A per-minute sweep catches lost jobs via the `sla_due_at` index.
- **Breach output.** A breach emits `conversation.sla_breached` and a warning emits `conversation.sla_warning` (realtime and webhooks). Both notify the assignee, or the team's members when there is no assignee, through two new member notification kinds (`sla_warning` push; `sla_breached` push and e-mail).
- **API, stats, reports.** `Conversation.sla` = `{policy_id, due_at, state: on_track | due_soon | breached | met | paused, targets[]}`. `/v1/stats` gains `sla.met` and `sla.breached`, in total, per inbox and per member (`assignee_id_at_outcome`). Y-147 rolls up the met and breached instants of `conversation_sla`.
- **Endpoints.** `/v1/sla-policies` (CRUD plus `PUT /order`): `inboxes:read` and `inboxes:manage`, owners and admins.

- **As built (#140, migrations 00109 and 00110).** Outcomes live in `sla_outcomes` (one per target
  start, unique on conversation, target and start; inserts `ON CONFLICT DO NOTHING`), because
  next-reply targets replace one another on `conversation_sla`; `/v1/stats` and #147 count from it.
  A breached target keeps its `due_at` until met, so breached conversations sort first. Every
  evaluation re-checks that the row's policy is enabled and matches; when no policy applies (spam,
  trash, no match, a policy disabled, narrowed or deleted) the row is kept with `dropped_at` instead
  of being deleted, and when one applies again the running targets get back the business time they
  had left, so time while dropped never counts and a breach is never recorded twice. A conversation
  that never had a row starts no target before the policy's `created_at`. Policy edits and label or
  inbox deletions re-evaluate the conversations they drive in the same transaction; deleting the last
  label or inbox in a policy's list turns the policy off. The clock walks civil dates, so days whose
  midnight does not exist (America/Santiago) are neither skipped nor counted twice; enabled hours with
  no intervals count as plain time. A warning is queued only while its time is still ahead.
  `snooze_wake` leaves spam and trashed conversations snoozed. Inbox-limited keys cannot write
  policies; agents and limited keys see policies with no inbox list or one naming their inboxes.
  `conversation.updated` is also sent when only `sla` changes. Notification recipients still need
  inbox access; `away` skips team members, not the assignee.

**Automation rules (#141).**
- **Rules.** `automation_rules (workspace_id, id, name, enabled, position, inbox_id nullable, trigger, idle_minutes, conditions jsonb, actions jsonb, created_by, …)`.
  - Triggers: `conversation_created`, `message_received` (contact message), `status_changed`, `idle`.
  - Conditions are an all-of list over: channel kind, kind, feedback category, subject or body contains, contact attribute, label, priority, team, assignee empty, inside or outside business hours.
  - Actions, at most 10 per rule: add or remove label, assign (member, team, or the auto-assignment pool), priority, status, snooze, note, reply with a canned reply, send to a webhook endpoint.
- **One ordered list.** Workspace and inbox rules share one list ordered by `position`; an inbox rule is a rule with `inbox_id`.
- **Runs.** `automation_runs (workspace_id, id, rule_id, conversation_id, trigger_key, outcome: applied | failed, error, actions jsonb, created_at)` with a unique key on `(workspace_id, rule_id, trigger_key)`. Kept 30 days. `GET /v1/automation-rules/{id}/runs` is the rule log.
- **Queuing.** `inTx` calls `queueAutomations(ev.items)`. Each eligible event queues `automation_run {workspace_id, event_id}`, using the ids `writeEvents` returns. `trigger_key` is `event:<id>`. For `idle`, a 5-minute sweep uses `idle:<conversation_id>:<last_activity_at>`, so each idle period fires once.
- **Execution.** The job loads the enabled rules for the trigger in order. For each rule, one transaction:
  1. lock the conversation;
  2. `INSERT automation_runs … ON CONFLICT DO NOTHING`; zero rows means the rule already ran, so skip it;
  3. evaluate the conditions on the current state, so a later rule sees an earlier rule's changes; when they do not match, roll back, so no row is written;
  4. apply the actions through the same code as `/v1` (`changeConversation`, message create) under a rule principal that sees every inbox.

  A failed action rolls back that rule's transaction and records `failed` in a separate transaction. Rules already committed stay committed on retry, so each rule has at most one effect per trigger.
- **No loops.** `eventBatch` carries `origin_rule_id`; events written by a rule store it in the payload (`rule: {id, name}`), and `queueAutomations` skips them. A rule's change never triggers a rule.
- **Customer-facing safety.** A canned-reply action is skipped (outcome `failed`, reason `auto_reply`) when any of these hold:
  - the triggering message is `message_emails.auto` or a bounce;
  - the contact is blocked;
  - the conversation is spam;
  - a rule already replied in this conversation within the hour (`ClaimAutoReply`-style claim).
- **Webhook action.** It delivers an `automation.triggered` event to an existing endpoint through the normal delivery jobs (same signing and address checks). Rules store no URLs or secrets.
- **Rate limit.** `automation_actions`, 600 per workspace per minute, checked in the job before any transaction opens. Over the limit, the job snoozes 60 s.
- **Order with assignment.** The automation job queues `auto_assign` last for `conversation_created` and `message_received` (reopen).
- **Scopes.** `automations:read`; `automations:manage` is a manager scope, never implied, refused for inbox-limited keys. A rule acts with workspace-wide power.
- **Audit.** `automation_rule.created`, `.updated`, `.deleted` (target host only for webhook actions).

- **As built (#141, migration 00113).** While `bots_may_send` is off a rule reply needs both the rule
  and its canned reply last saved by a member who is owner or admin at run time; a rule's
  `saved_by` changes when its trigger, idle minutes, inbox, conditions, actions or `enabled` change,
  not on rename or reorder, and canned replies record `saved_by` when their body changes (replies
  saved before 00113 count as unknown and must be saved again). Rule replies are also capped at three
  per conversation in 24 hours. A rule whose conditions did not match commits a `not_matched` run, so a
  retry never evaluates it again on a later state; database errors are retried by River, business
  refusals become `failed` runs with a specific reason (`auto_reply`, `bot_sending_disabled`,
  `field_required`, `team_access`, `label_gone`, `canned_reply_gone`, `webhook_unavailable` and more).
  Every message a rule writes has author `system` and `Message.rule {id, name}`; each applied run adds
  a `rule_applied` timeline event; `conversation.updated` does not name the rule. Rule notes and
  replies move `last_message_at` but not `last_activity_at`, so a rule never starts a new idle period;
  an idle run re-checks under the conversation lock that nothing happened since and the conversation
  is still open or pending and not spam. `conversations.snooze_rule_id` records the rule that set a
  snooze, and its wake triggers no rule. The idle sweep runs every 5 minutes over open and pending
  conversations quiet for the rule's minutes (a window of 15 minutes, 2000 per value per sweep).
  Inside one rule: note, reply, one conversation change, then the webhook. The rate limit is taken
  per rule before its transaction, all or nothing, only for rules that will apply; a refused job
  snoozes until the window ends and resumes from that rule. Deleted references are not stripped:
  an action needing one fails the run; deleting an inbox deletes its rules. Conditions include
  `status`; operators are `in`, `not_in`, `equals`, `not_equals`, `contains`, `not_contains`,
  `is_set`, `is_not_set`. `automation.triggered` cannot be subscribed to; it reaches only the endpoint
  a rule names.

**Macros (#142).**
- **Table.** `macros (workspace_id, id, owner_member_id nullable, name, shortcut, body nullable, actions jsonb, position, …)`. Null owner means shared. Shortcut is unique per owner scope.
- **Actions.** Labels add and remove, assignee or team, status, priority, snooze duration.
- **Endpoints and scopes.** `/v1/macros`. Read with `conversations:read` (shared ones, plus the caller's own). Personal macros need a member session or OAuth token. Shared macros are written by owners, admins and keys with `canned_replies:write`.
- **Composer.** The `/` menu and the command palette list canned replies and macros together. Choosing a macro fills the composer and holds its actions.
- **Applying on send.** `MessageCreate` gains `then: ConversationUpdate` plus `macro_id`. The server applies them in the same transaction as the message, so the existing undo hold (which only delays the POST) covers both. The timeline event names the macro.
- **Unchanged.** Canned replies stay as they are. `then` is also how "send and close" can become atomic later.

- **As built (#142, migration 00100).** Actions are `add_labels`, `remove_labels`, `assignee_id` (null
  unassigns), `status`, `snooze_minutes` (implies snoozed) and `priority`; members only, no team yet.
  Shortcuts are unique among shared macros and per member among personal ones. `then` overrides the
  macro field by field. A macro's assignee who can no longer take the conversation is skipped and the
  `macro_applied` event says `skipped: ["assignee"]`; an assignee in `then` is still refused. Removing
  a member clears them from macros, deleting a label strips it, both in the same transaction; saves
  check labels and the assignee under `FOR SHARE` locks. A repeated `client_id` applies nothing.

**Saved views (#143).**
- **Table.** `saved_views (workspace_id, id, name, visibility: personal | workspace | team, owner_member_id, team_id, filters jsonb, position, …)`.
- **Filters.** The stored filters use exactly the `ListConversations` query parameters, validated by the same parser.
- **Who manages them.** Personal views: any member. Workspace and team views: owners, admins and keys with `views:write`. Team views are listed to team members, owners and admins.
- **Counts.** `GET /v1/views` returns each view with `count`, open conversations counted under the caller's own `inbox_viewers` filter, capped at 1000. A shared view therefore never counts or lists anything the caller cannot see. At most 50 views per caller.
- **Deleted labels and teams.** The delete transaction strips the id from `filters` with a jsonb update. A rename changes nothing, because views store ids.

- **As built (#143, migration 00111).** `filters` are parsed by `conversationFilter`, the same code
  `ListConversations` calls, so a list parameter added there is a view filter with no change in
  views; at most 30 parameters of 2000 characters. Every views endpoint needs `conversations:read`,
  writes also `views:write`. Limits are 50 personal views per member, 50 workspace views and 50 per
  team; `GET /v1/views` lists the caller's own, the workspace's and their teams' views without
  truncation, and owners and admins reach other teams' views with `?team_id=`. Inbox-limited keys
  neither see nor write team views. Counts wrap the list's own statement in `count(*)`, four at a
  time, each stopped after one second (`count_unavailable`). Deleting a team deletes its team views;
  PATCH locks the view and writes only the fields sent.

**Custom fields (#144).**
- **Definitions.** `custom_fields (workspace_id, id, object: conversation | contact, key ^[a-z][a-z0-9_]{0,39}$, name, type: text | number | date | select | checkbox, options jsonb, required_on_close, position)`. At most 50 per object.
- **Values.**
  - Conversations: `conversations.fields jsonb NOT NULL DEFAULT '{}'`.
  - Contacts: the existing `attributes`. A key with a definition is validated (`400 field_invalid` on `/v1`). On `/client/v1` and identity tokens an invalid value is dropped and logged, never refused. Other keys stay free-form.
- **Filtering.** `field.<key>=<value>` on the list and in views, via `fields @> $1`, served by a GIN index (`jsonb_path_ops`).
- **Required on close.** Closing without a required field answers `409 field_required` (with `field`). This holds on PATCH, bulk (per item), MCP, macros and rules (the rule logs `failed`).
- **Deletions.** Deleting a field or a select option clears the conversation values in a River batch job; contact attributes are kept.
- **Endpoints and payloads.** `/v1/custom-fields` with the new manager scope `fields:manage`. Values are written with `conversations:write` / `contacts:write`. `fields` appears on webhook and realtime payloads.

- **As built (#144, migrations 00106 and 00107).** Object, key and type are immutable; `options`
  replaces the list; `required_on_close` exists only on conversation fields. Every deletion of a field
  or a select option queues its own `custom_field_clear` job, which walks conversations by id 500 at
  a time; a create refused with `409 field_clearing` queues a fresh clear, so a lost job never blocks
  a key. Lifting spam from a closed conversation also needs the required fields. Spam conversations
  close without them. A "reply to linked" with close still sends and leaves a conversation open when
  its required field is missing.

**MCP (coverage rules).**
- **Added:** `list_teams`, `list_macros` (body as text for `draft_reply`; actions listed), `list_custom_fields`, `set_conversation_fields`. Also: `assign` gains `team_id`; `search_conversations` gains `team_id`, `sla_state`, `sort=sla_due` and `fields`; `get_conversation` returns `sla` and `fields`.
- **Left out on purpose:**
  - Team management and membership: access, under the credentials-and-access rule.
  - Automation rules, read and write: they act with workspace power and can mail customers and call webhooks.
  - SLA policies and field definitions: workspace settings.
  - Saved views: a panel construct; search covers the filters.
  - Macro writes and applying a macro: actions are applied only on a member's send.

**Webhook and event types added:** `team.created`, `team.updated`, `team.deleted`, `conversation.sla_warning`, `conversation.sla_breached`, `automation.triggered` (endpoint action only).

**Left out of M28:**
- holiday calendars;
- per-member business hours;
- skills-based routing;
- load weighting by channel;
- SLA on `mail` inboxes;
- rules with OR groups;
- rules triggered by notes or by other rules.

### Reports (M29)

Built in #147 (migration 00114). Reports answer from rollups, never by scanning `messages` over the
range, and agree with `/v1/stats` for today.

- **Granularity: UTC quarter hours.** `report_rollups (workspace_id, bucket, inbox_id, channel_kind,
  team_id, member_id, label_id, metric, bin, n)` holds sums per 15-minute UTC bucket, not per day or
  hour: every offset in use today is a multiple of 15 minutes (India +5:30, Nepal +5:45, Chatham
  +12:45), so any local day, week or month is a whole number of buckets and a report in any IANA zone
  is exact. Reads sum buckets into `date_trunc(granularity, bucket AT TIME ZONE tz)`. Rows exist only
  where something happened. Index `(workspace_id, bucket)`.
- **Facts.** One query (`ReportFacts`) lists everything that happened in a window, each with the
  conversation's inbox, channel kind and team and the member it is attributed to, counted exactly as
  `/v1/stats` counts: `replies` (member replies or bot drafts a member sent; bots, rules and automatic
  replies never), `closed`, `reopened` (`previous_status` closed), `opened` (not spam, not imported),
  `first_reply` (stats' definition, attributed to the replier), `resolution` (each close after a
  contact message, from the first such message after the previous close, attributed to the closer),
  `rating_good|bad` (at `rated_at`, to the assignee), and `sla_<target>_<met|breached>` from
  `sla_outcomes` (inbox, assignee and team at the outcome). Trashed conversations and imported rows
  are left out. Labels are the conversation's labels when its bucket is built; each fact is written
  once with `label_id` null and once per label, and totals read only the null rows.
- **Percentiles: fixed log histograms.** `first_reply` and `resolution` rows count per bin: bin 0 is
  under a second, bin b holds [1.05^(b-1), 1.05^b) seconds, capped at 400 (about 9.5 years). Bins add
  up across buckets, days and groups, so medians and p90 over any range are exact up to the bin:
  within 2.5% when read at the bin's geometric middle, interpolated between neighbouring samples so an
  even count's median is the mean of the middle two as in `/v1/stats`. No t-digest: merging fixed
  bins in SQL is a plain `sum`.
- **The job.** A periodic `report_rollup_scan` (5 minutes) queues `report_rollup {workspace_id}`
  (unique while pending) for every workspace (`ListWorkspaceIDs`, the SLA sweep's listing).
  `report_rollup_state (workspace_id, built_until, recomputed_on, recompute_at)` drives it, each step
  one transaction under the state row's lock that deletes the window's buckets and copies fresh ones:
  first a UTC day at a time from the first activity reports count (the earliest of a native
  conversation, a message that was not imported, so replies in imported threads count, and an SLA
  outcome; the backfill), then today from midnight UTC (or 10 minutes before `built_until`,
  whichever is earlier) to the last quarter-hour edge on every run; `built_until` is always a bucket
  edge, so a bucket is never part rolled up and part counted live, and
  once per UTC day the 31 days before today, a day per step (`recompute_at` resumes an unfinished
  rebuild). A run stops after 20 seconds and snoozes itself to go on. Deleted workspaces are skipped.
- **Reading.** `GET /v1/reports` reads the rollups before `built_until` and, when `built_until` is
  less than a day old, counts the facts after it live and passes them to the same SQL as arrays, so
  one query filters, checks access and groups both. Inboxes are filtered through `inbox_viewers`.
  Deleted inboxes and labels drop out at read time; deleted teams and members fall into the group
  without a key. The ids are not foreign keys, so no deletion rewrites the table. `from`/`to` are local
  days, at most 13 months apart; series list every period, the first cut at `from`.
- **API.** `GET /v1/reports` and `GET /v1/reports/export` (CSV, the same parameters, formula-safe
  cells as the consent export), scope `reports:read` (every member's token may have it; implied for
  keys made without named scopes from now on, never added to older keys). MCP `get_report`.
- **Privacy.** Rollups hold counts and ids, no content. They go with the workspace (foreign key on
  `workspace_id`, `ON DELETE CASCADE`). Retention days do not apply to them; the nightly rebuild stops
  at the retention cutoff, so periods whose conversations retention deleted keep their counts.
  Conversations deleted or trashed inside the rebuild window drop out of it.

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
  fan-out, attachment cleanup, retention, and migrations on their own `migrations` queue (see
  "Moving to Yuva").
- Rate limits count in Postgres so every replica shares them (#134): `rate_limits` (UNLOGGED, one
  row per key) holds a fixed window that starts with the key's first request; one upsert per check
  counts and answers, and several keys go in one statement when a batch is checked (analytics server
  events; takes of one key are merged first). Windows and leases are measured by the database's
  clock (`clock_timestamp()`), never a replica's. A check that fails in the database lets the
  request through and is logged at most once a minute: a broken limiter must not take the API down.
  A check that depends on rows a transaction read (slug changes, idea votes, subscription
  confirmations, new anonymous visitors) runs on that transaction's connection in a savepoint, so it
  never waits for a second pool connection; all other checks run before any transaction opens. Keys
  are HMAC-SHA256 (derived from the master key) of the limit name and its subject, truncated to 16
  bytes, so no address or e-mail address is stored. The 4 concurrent analytics queries per workspace
  are leases in `rate_slots` (2 × the query timeout + 30 s, so a crashed process frees them). A River
  job every minute deletes ended windows and leases. `rate_limits` and `rate_slots` are the written
  exception to the `workspace_id` rule: most limits are per address or token before any workspace is
  known, and the key is a hash. Sign-ups, migration connection tests and list imports count their
  own rows, as before.
- Realtime: WebSocket (`coder/websocket`). Every change that members see live writes a row to
  `events` in the same transaction, last, under a per-workspace advisory lock, so event ids within
  a workspace follow commit order; the transaction's `NOTIFY` carries the id. One listener per
  process loads each notified event and an in-process hub fans it out to that workspace's
  connections, filtered by inbox access. Clients resume with the last event id: the server
  replays newer events (kept 7 days, cleaned by a River job) before live ones, or answers
  `resync_required` and the client reloads over HTTP. A slow connection is closed instead of
  holding up the hub; when the listener reconnects, every connection is closed so it resumes.
  Typing and presence notices are not stored: they travel as the payload of a second `NOTIFY`
  channel, have no id and are never replayed. Each open realtime connection is a row in
  `realtime_connections`, seen on every heartbeat; presence is read from the rows seen in the last
  75 seconds. Members see their teammates the same way: `/v1/members` reports each member's
  `availability` and whether they are `online`, and a `member.presence` notice reaches the
  workspace's member connections when a member connects, disconnects or changes availability.
  Contacts likewise: a `contact.presence` notice reaches the member connections that can see the
  contact whenever one of its `/client/v1/realtime` connections opens or closes. When a contact
  reads further in the widget or an app, the conversation's `last_read_by_contact_at` moves (kept
  on the conversation row next to `contact_reads`) and a stored `contact.read` event reaches the
  members who can see the conversation; it never goes to webhooks or to contacts.
  A connection that ends without closing (a process that died) only stops being seen, so a River
  job every minute announces the members and contacts whose last connection went stale in the
  previous two minutes.
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
| `/client/v1` | widget, mobile SDKs | channel public key + contact session, or a reader token on the discussion endpoints |
| `/client/v1/analytics/{key}` | the analytics script, `useyuva/analytics`, the mobile analytics modules | an analytics property's public key; browsers only from the property's domains |
| `/v1/analytics/shared/{token}` | read-only analytics shares | a share token (and its password session) |
| `/ingress/email` | edge Worker, MTAs | HMAC signature over the raw message |
| `/ingress/ses` | SES bounce and complaint notifications via SNS | SNS signature |
| `/s/{token}`, `/m/{token}`, `/c/{token}`, `/i/{token}` | people who receive list and broadcast mail | a token sealed under the master key; an image token |
| `/status/{slug}`, `/client/v1/status/{slug}`, a verified status domain | anyone reading a published status page | none |
| `/roadmap/{slug}` | anyone reading a public roadmap board | none |
| `/changelog/{slug}` | anyone reading a published What's new feed | none |
| `/ingress/heartbeat/{token}`, `/ingress/status/{token}` | host jobs and monitoring (heartbeats, Alertmanager, Grafana) | a monitor token |
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
yuva channel create-chat --workspace <id|name> --inbox <id|slug> --name <name> --origin <origin>... \
  [--allow-anonymous] [--ask-email-offline=false] [--greeting <text>]   # prints the channel id and public key
yuva channel list --workspace <id|name> --inbox <id|slug>                 # never a password
```

The slug is made from the name when omitted. The SMTP password is read from a file or, with `-`,
from stdin, never from the command line; the auto-reply stays off. The inbox's identity secret is
not printed; rotate it when an app needs one.

Unlike the commands above, the MCP bridge needs no database or server configuration; it runs on the
machine of whoever uses an assistant (see "MCP for local clients"):

```sh
yuva mcp stdio --url <server> --key <key>   # or YUVA_URL, YUVA_API_KEY; stdout carries only MCP
```

### Headless access

Everything the panel does is reachable without it: scripts, bots and other UIs use `/v1` with
scoped API keys, and every key-authored message names its bot.

**Scopes.** A key holds a list of scopes; every `/v1` operation a key may call needs exactly one:

| Scope | Operations |
|---|---|
| `conversations:read` | list, get and count conversations; list messages and pending drafts; attachments, message e-mail and raw source; list labels and canned replies |
| `conversations:write` | create, update (status, assignee, snooze, priority, labels), move and bulk-update conversations |
| `messages:write` | outgoing and incoming messages, drafts: create, edit, discard |
| `drafts:send` | send a draft (`POST /v1/messages/{id}/send`), together with `messages:write` |
| `notes:write` | notes |
| `contacts:read` | list, look up and get contacts, presence |
| `contacts:write` | create, update, delete and merge contacts |
| `inboxes:read` | workspace, members, inboxes, inbox members, channels |
| `inboxes:manage` | create, update and delete inboxes and channels, inbox access, secret and key rotation |
| `labels:write` | create, update and delete labels |
| `canned_replies:write` | create, update and delete canned replies |
| `webhooks:manage` | webhooks, their deliveries and attempts |
| `workspace:manage` | update the workspace, usage |
| `migrations:manage` | migrations: detect, create, change, connect, test, scan, start, pause, resume, cancel, finish, remove imported history, errors, probe, uploads |
| `feedback:write` | feedback |
| `lists:read` | lists, subscriptions, growth, consent export, imports |
| `lists:write` | lists, subscriptions, confirmations, imports |
| `broadcasts:read` | broadcasts, deliveries, reports, designs, templates, images, previews |
| `broadcasts:write` | drafts, designs, images, test sends |
| `broadcasts:send` | schedule, unschedule, pause, resume, cancel (only while the workspace allows keys and assistants to) |
| `analytics:read` | analytics properties, queries, realtime, goals, funnels, segments, annotations, health, exports (read) |
| `analytics:write` | server-side analytics events (API keys only) |
| `analytics:manage` | create, change and delete properties, keys, goals, funnels, shares, exports, data deletion |
| `roadmap:read` | roadmap boards, stage history, linked counts |
| `roadmap:write` | links between feedback and ideas, linked replies, board settings for keys |
| `roadmap:stage` | move an idea's stage (never implied) |
| `changelog:read` | What's new feeds and entries |
| `changelog:write` | feeds, entries and translation drafts |
| `changelog:publish` | publish, schedule, unpublish and delete published entries (never implied) |
| `audit:read` | the workspace audit log (never implied; owner-only, see Audit log) |
| `integrations:read` | GitHub and Linear integrations |
| `integrations:manage` | connect, change and disconnect GitHub and Linear, webhook secrets |
| `issues:write` | create, link and unlink GitHub and Linear issues on feedback and ideas |

A call without its scope gets `403 insufficient_scope` with the missing scope in `scope`. Keys made
before scopes existed hold every scope above. Scopes added later (`lists:*`, `broadcasts:*`, `integrations:*`, `issues:write`) are never
added to existing keys, and `broadcasts:send`, `status:manage`, `roadmap:stage`, `changelog:publish` and `audit:read` are never implied: a key made without
naming its scopes holds every scope but those. `yuva api-key create` takes `--scope` (repeatable, default
every scope but those five) and `--inbox` (repeatable).

A key may be limited to some inboxes. It then sees what an agent with access to exactly those
inboxes sees; it cannot hold `inboxes:manage`, `webhooks:manage`, `workspace:manage`, `migrations:manage`, `integrations:manage` or `audit:read` (refused when
the key is made). An inbox that is deleted drops out of the limit; a key whose every inbox is gone
sees nothing. A key may have an expiry; after it, the key gets `401 api_key_expired`. Scopes, inbox
limit and expiry are fixed when the key is made; a different set needs a new key. Name, bot name and
bot avatar can be changed (`PATCH /v1/api-keys/{id}`).

**Bot authors.** Every key is a bot: a display name (the key's name unless one is given) and an
optional avatar, an `https` image URL. Messages, notes and conversation events written with a key
have author type `bot` and point at the key; the bot's current name and avatar are shown in the
panel, the widget, the SDKs and webhook payloads. Outgoing e-mail from a bot uses the inbox's From
address with the bot's name as display name. Rows written by keys before this have author `system`
and stay so.

**Drafts.** An outgoing message can be a draft (`draft: true` on create). A draft is visible only on
`/v1` and to members: never on `/client/v1`, never e-mailed, never a notification, unread count,
preview, first response, usage or search hit. It can be edited (`PATCH /v1/messages/{id}`) and
discarded (`DELETE /v1/messages/{id}`); both answer `409 not_a_draft` for anything else.
`POST /v1/messages/{id}/send` delivers it: the message becomes a normal outgoing message with the
send time as its time, `sent_by` the member who sent it, and the draft author kept as author.
Events: `draft.created`, `draft.updated`, `draft.deleted` on realtime and as webhook types an
endpoint subscribes to; sending emits the usual `message.created`.
`GET /v1/drafts` lists the drafts waiting in the conversations the caller can see, newest first,
each with a conversation summary (id, inbox, subject, status, contact), filtered by `inbox_id` and
by `author`: `bot` (written with a key), `assistant` (a member through an OAuth client, `via` set)
or `member` (the panel); `total` counts the matches over all pages for a badge. Keys and tokens call
it with `conversations:read`. A client keeps it current from the draft events above and
`message.created` (a sent draft keeps its id), plus `conversation.updated`, `conversation.moved` and
`inbox_access.changed`.

The workspace setting `bots_may_send` (off for new workspaces; on after the upgrade for workspaces
that had an active key, so their integrations keep working) decides whether keys deliver at all.
Off: a key's outgoing message must be a draft, else `403 bot_sending_disabled`, and a key cannot
send drafts. On: a key with `messages:write` sends directly and one that also has `drafts:send`
sends drafts. Members always may. Owners and admins change it with a member session; a key, even
one with `workspace:manage`, cannot, so a bot never lets itself send.

**Idempotency.** Every authenticated `POST` on `/v1` and `/client/v1` accepts an `Idempotency-Key`
header (1 to 255 printable ASCII characters). The key is remembered for 24 hours per caller (API
key, member or contact) together with the method, path, a SHA-256 of the body and the response. The
same key with the same request returns the stored response with `Idempotent-Replayed: true`; with a
different request `409 idempotency_key_reused`; while the first is still running
`409 idempotency_key_in_use`. `5xx` answers are not stored, so the request can be retried.
Unauthenticated endpoints (sign-in, client session) and those that act on the signed-in person rather
than a workspace (`/v1/me/...`, sign-out) ignore the header, since every stored key belongs to a
workspace. The existing `client_id` on messages stays as it is.

**Event feed.** `GET /v1/events?after=<id>&limit=<n>` returns the events realtime and webhooks
carry, in id order, filtered exactly as the caller's realtime stream is (inbox access, key inbox
limit, scopes: a caller sees an event only if it holds `conversations:read`; subscription events also
need `lists:read`, broadcast events `broadcasts:read`, and contact events `contacts:read`). `after` is the last event id the caller handled; the answer carries
`next` (the last id returned) and `has_more`. Events are kept 7 days (realtime replay uses the same
rows). An `after` older than the oldest kept event answers `410 cursor_expired`: the caller
resyncs from the lists and continues from `GET /v1/events/latest`'s id. Keys and tokens open
`/v1/realtime` with `Authorization: Bearer`. Details: `after=0` starts at the oldest kept event and
never expires; a workspace with no kept events answers `410` to any other `after`, so a caller
idle through a silent week resyncs once. `limit` is 1 to 500 (100 by default). Events the caller
may not see are skipped, so `next` can be past the last returned event (the last one examined) and
a page can be short while `has_more` is true; one call examines at most 5000 events. The expiry
check runs after the page is read, so a cleanup in between cannot drop events silently.
`/latest` returns the workspace's newest id whatever the caller may see (`0` when none).

**API-only mode.** `YUVA_PANEL=off` stops serving the panel (and so the OAuth consent page:
`/oauth/authorize` then answers `temporarily_unavailable`; API keys still work). `YUVA_WIDGET=off`
stops serving the widget scripts. The API, realtime, ingress and `/mcp` stay. With the panel off,
its paths answer `404` and `/oauth/authorize` checks the client and `redirect_uri` as usual, then
redirects back with `error=temporarily_unavailable`. `/client/v1` stays with the widget off.

**JS packages.** The JS SDK is published to npm as the unscoped package `useyuva` (`yuva` is refused by npm as too
close to other names, and the `@yuva` scope belongs to someone else), MIT, versioned with the repository:
- `useyuva`: `createYuvaClient()`, the contact side (`/client/v1`) without DOM: sessions,
  identity tokens, conversations, messages, attachments, typing, read state, realtime with resume.
- `useyuva/chat`: the `<yuva-chat>` element, built on that client; the script-tag build stays.
- `useyuva/react`: `YuvaProvider`, `useConversations`, `useMessages` (React is an optional peer).
- `useyuva/nuxt`: a Nuxt module with `YuvaChat`, `YuvaPageFeedback` and `YuvaPageQuestions`
  components (`@nuxt/kit` and Vue are optional peers).
- `useyuva/api`: a typed `/v1` client for TypeScript backends, generated from the contract.

**Server helpers.** `useyuva/server` signs identity tokens, verifies webhooks and queues server-side
analytics events with only Web Crypto, `Request` and `fetch`; `useyuva/h3` wraps it as h3 v2
handlers and a middleware; `useyuva/nitro` is a Nitro v3 module, and the Nuxt module installs the
same server part on Nitro v2 or v3. That part serves `GET /_yuva/identity` (a token for what the
app's `server/yuva.ts` returns from `user(event)`, which the Nuxt components then use on their own)
and `POST /_yuva/webhook` (verified, then dispatched to that file's `webhooks` by type), each added
only when its secret is known at build time and answering `404` while it is empty at runtime, and
flushes server analytics through `waitUntil` per request and on Nitro's `close` hook. The runtime
handlers talk to Nitro through a generated module that adapts h3 v1 (`toWebRequest`) or v2
(`event.req`) to Web `Request` and `Response`. The identity secret, webhook secret and analytics
API key live only in the server runtime config (`yuva.identitySecret`, `yuva.webhookSecret`,
`yuva.analytics.apiKey` and `propertyId`, overridable as `NUXT_YUVA_*` or `NITRO_YUVA_*`), never in
the public config or a client bundle.

`sdk/go` gets a `/v1` client generated from the contract (`sdk/go/client`), next to `identity` and
`webhook`.

**MCP for local clients.** `yuva mcp stdio --url <server> --key <key>` (or `YUVA_URL`,
`YUVA_API_KEY`) bridges stdio to a server's `/mcp` over HTTP; it needs no database. Releases attach
an MCP Bundle (`yuva.mcpb`, the bridge for macOS, Linux and Windows, asking for the server URL and
key) and the repository keeps `server.json` for the MCP Registry, describing the remote endpoint
with the server URL as a variable and the stdio bridge as a package.
- The bridge relays JSON-RPC messages unchanged, so every protocol version and method the server
  speaks (including `subscriptions/listen`) passes through. `--url` is the server's public URL;
  `/mcp` is appended unless it is there. The key is sent as `Authorization: Bearer` and may be an
  API key or an OAuth access token. A call that fails gets a JSON-RPC error on stdout and a log
  line on stderr (JSON); an unreachable server fails each call but keeps the bridge running. A
  `401` (wrong, expired or revoked key) ends it with exit status 1; end of stdin with 0.
- The bundle follows MCPB manifest 0.3 (`deploy/mcpb/manifest.json`) and is packed by the release
  workflow with `@anthropic-ai/mcpb` 2.1.2. Server type `binary`: `server/yuva` is a POSIX shell
  launcher that picks `server/<os>-<arch>/yuva` (darwin arm64 and amd64, linux amd64 and arm64);
  Windows runs `server/yuva.exe` (amd64). `user_config` asks for `url` and `api_key` (sensitive)
  and passes them as `YUVA_URL` and `YUVA_API_KEY`. The binaries are the full `yuva` without the
  built panel, so the bundle stays near 60 MB.
- `server.json` uses the registry schema `2025-12-11`. The remote is `https://{yuva_host}/mcp` with
  `yuva_host` as a required variable (the registry refuses a remote URL another server already
  lists, and `https://{host}/mcp` is taken) (the schema allows a variable in the host, not yet a whole base
  URL). The package is `registryType: mcpb` pointing at the release's `yuva.mcpb`. The file in the
  repository has no `fileSha256`; the release workflow fills the version, the asset URL and the
  hash and attaches that `server.json` to the release; for a stable tag the `registry` job publishes it
  with GitHub OIDC (no token), as `io.github.productdevbook/yuva`. Its version and the manifest's
  move with the repository version.

### OAuth and MCP

**Authorization server.** Yuva is its own OAuth 2.1 authorization server, for MCP clients and for
the native member apps planned in M10. Authorization code with PKCE (`S256` only), no implicit or password grant.
Client metadata must list `authorization_code` among its `grant_types`; other types it lists are
ignored (claude.ai lists `jwt-bearer`), and registration answers with the two Yuva offers.
- Metadata: `/.well-known/oauth-authorization-server` (RFC 8414) and
  `/.well-known/oauth-protected-resource/mcp` plus `/.well-known/oauth-protected-resource`
  (RFC 9728). A `401` on `/mcp` carries `WWW-Authenticate: Bearer resource_metadata="…"`.
- Clients: dynamic registration at `POST /oauth/register` (RFC 7591, public clients, no secret),
  and Client ID Metadata Documents (a `client_id` that is an `https` URL, fetched with the same
  private-address rules as webhooks and cached for 24 hours; its `client_id` must equal the URL and
  it must be a public client). Redirect URIs must be `https`, or `http` on loopback, or a
  private-use scheme for native apps; matched exactly, except that an `http` URI on a loopback host
  (`127.0.0.1`, `[::1]`, and `localhost`, which Claude Code registers) matches on any port, at
  authorize as RFC 8252 §7.3 and OAuth 2.1 ask (the token request must repeat the URI the
  authorization request used). `redirect_uri` may be left out only when the client has one.
  Registration answers `token_endpoint_auth_method: none` whatever was asked, allows 20
  registrations per address and hour, and clients that never got a grant are deleted after 30
  days. Client ids are `yuva_client_<random>`.
- `GET /oauth/authorize` checks the request and sends the browser to the panel's consent page
  (`/oauth/consent?request=<id>`). The panel signs the person in if needed (code or passkey), shows
  the client's name and redirect host, lets them pick one workspace, and shows the scopes. Approving
  returns the redirect with a code (single use, 60 seconds). The panel reads and answers the request
  with `GET /v1/oauth/requests/{id}`, `POST …/approve` (workspace and scopes) and `POST …/deny`, all
  with a member session; a pending request lives 10 minutes and is used once. Every redirect back
  carries `state` and `iss` (RFC 9207).
- `POST /oauth/token`: `authorization_code` and `refresh_token`. Access tokens are opaque, 1 hour;
  refresh tokens 30 days, rotated on every use, and reusing a rotated one revokes the grant, as does
  using a code twice. Both are stored as hashes (`yuva_at_…`, `yuva_rt_…`) and looked up by hash
  alone, like API keys. `POST /oauth/revoke` (RFC 7009): a refresh token revokes the grant, an
  access token only itself.
- Resource indicators (RFC 8707) are required. `<YUVA_PUBLIC_URL>/mcp` gives a token for `/mcp`
  only; `<YUVA_PUBLIC_URL>` gives one for `/v1`, `/v1/realtime` and `/mcp` (for the planned member apps). A
  trailing `/` is ignored. A `/mcp`-only token on `/v1` gets `401 token_wrong_resource`; an expired
  one `401 token_expired`.
  Accepting a `/v1` token at `/mcp` is intentional: MCP acts as the member with the same scopes.
- Scopes are the API key scopes. A token acts as its member: what the member's role and inbox access
  allow, narrowed by the granted scopes, never more. A member who leaves or is removed loses every
  grant. Management scopes a member's role cannot use are not offered on the consent page: agents
  are not offered `inboxes:manage`, `labels:write`, `canned_replies:write`, `webhooks:manage`,
  `workspace:manage` and `migrations:manage`, and nobody `feedback:write` (API keys only). A token may call exactly the
  operations an API key may; the rest (read state, typing, notification settings, `/v1/me`,
  members, keys, Connected apps, the consent endpoints) need a member session, and a token never
  changes `bots_may_send`.
- A grant is one member, one workspace and one client; approving the same client again in that
  workspace replaces the grant's scopes and resource, and its live tokens follow. The 600 requests a
  minute are counted per grant (its tokens rotate hourly), in the process like the other rate
  limits, before scope checks. Settings → Connected apps lists a member's
  own grants (client, scopes, created, last used, request count this month) and revokes them;
  owners and admins see and revoke every grant in the workspace.
- Registered clients and Client ID Metadata Documents belong to no workspace (a client is used by
  many); they are the exception to the `workspace_id` rule, as are pending authorization requests,
  which exist before the person picks a workspace. Grants, codes and tokens carry the workspace.

**Writes through a client.** Everything a token writes records the client: messages, notes,
drafts and conversation events get `via` (the client name at the time), and the timeline shows
"Ayşe via Claude Code". `via` is on the message's `author`, and a draft sent with a token also gets
it on `sent_by`. Delivery from a `/mcp`-only token follows `bots_may_send` like an API key:
off, an outgoing message must be a draft. A `<YUVA_PUBLIC_URL>` token (for the planned member apps) sends
as the member.

**MCP endpoint.** `/mcp` in the same binary, Streamable HTTP, MCP specification 2026-07-28, built
on `github.com/modelcontextprotocol/go-sdk` (MIT/Apache-2.0). Bearer is an OAuth token or a scoped
API key. `YUVA_MCP=off` turns it off (the OAuth endpoints stay for the apps). The transport is
stateless, as 2026-07-28 requires: there are no MCP sessions, every request is authenticated and
metered on its own and gets a server built for its principal. Older protocol versions work request
by request; their `resources/subscribe` is refused, so subscriptions use `subscriptions/listen`.
A listen is fed from the realtime hub (any instance the stream is on), filtered like `/v1/realtime`,
re-checks its token every 30 seconds and ends when the token stops working. The endpoint sends
`Access-Control-Allow-Origin: *` and skips the SDK's localhost Host check: it takes no cookies.
- Tool results are Yuva's own shapes, not the `/v1` schemas: ids, enums, counts and times at the
  top level, customer text under `customer_content`; names written by members (inboxes, labels,
  canned replies, bots) stay outside. Schemas have one type per field, without `null` unions, so
  clients with a single-type dialect (Gemini) accept them; absent fields are left out.
- Mappings that are not one operation: `get_conversation` is the conversation plus its latest 50
  messages oldest first (`next_cursor` pages back) and, with `contacts:read`, the contact's name and
  address; `list_feedback` is `ListConversations` with `kind=feedback`; `add_labels` and
  `remove_labels` are `BulkUpdateConversations` with one id. `merge_contacts` is listed only to
  owners, admins and keys without an inbox limit.
- Setup tools: `list_channels` (`ListChannels`; kind, name, public key, allowed origins,
  anonymous, discussions, e-mail addresses, never secrets), `create_inbox` (`CreateInbox`; the slug
  is made from the name as the panel does, with `-2`, `-3` … when taken; the identity secret is not
  returned), `create_chat_channel` (`CreateChannel` with kind `chat`; anonymous on and discussions
  `off` unless given), `update_chat_channel` (`GetChannel`, then `UpdateChannel` with the current
  chat settings and the added and removed origins, anonymous, discussions and greeting changed) and
  `get_install_snippet` (`GetChannel`; the code for `html`, `react`, `nextjs`, `nuxt`, `vue`,
  `astro`, `shadcn`, the `docs-` variants and the `subscribe-` variants for `html`, `react`, `nuxt` and
  `shadcn` with `list_ids` (public lists of the channel's inbox), filled with `YUVA_PUBLIC_URL` and
  the public key, an `npm install "useyuva@>=<server version>"` line where needed, where it goes and the guide on
  GitHub). The templates live in the server (`mcp_setup.go`) with the panel's wording. The three
  writes are listed to owners, admins and keys with `inboxes:manage`. Calling a tool that is not
  listed answers `insufficient_scope` naming the scope to reconnect with, or the role or
  `bots_may_send` reason.
- Broadcast tools: `list_mailing_lists`, `list_broadcasts`, `get_broadcast` (with the review and the
  audience for a draft) and `get_broadcast_report` are listed by scope (`lists:read`,
  `broadcasts:read`); `list_email_templates` and `preview_design` (the HTML as an embedded resource)
  with `broadcasts:read`, and `create_design`, `update_design` (with `base_version`),
  `create_broadcast_draft`, `update_broadcast` (the draft's fields or a running broadcast's pace; it
  answers with the review again) and `send_test_broadcast` with `broadcasts:write`, are listed only to
  owners, admins and keys. A test through an OAuth token goes to the caller only; keys name members. `schedule_broadcast` is listed only to principals
  with `broadcasts:send` while the workspace allows `broadcasts_by_keys`, and needs `send_at` and
  `confirm_recipients` equal to the audience count it was shown; the approval gate applies. Reply
  contacts in reports are customer content. Resource `yuva://broadcast/{id}`; prompts
  `draft_newsletter` (never schedules) and `broadcast_report`.
- Migration tools (`detect_mail_source`, `list_migrations`, `get_migration`, `create_migration`,
  `update_migration`, `test_migration_connection`, `scan_migration`, `preview_migration`,
  `start_migration`, `pause_migration`, `resume_migration`, `cancel_migration`, `finish_migration`,
  `retry_migration_items`, `list_migration_errors`, `send_migration_probe`,
  `remove_migration_history`) are listed to owners, admins and keys with `migrations:manage`. None
  takes a credential: `create_migration` returns the panel link where the person connects.
  `remove_migration_history` carries `destructiveHint` and needs `confirm_count`. Resource
  `yuva://migration/{id}`; prompt `move_to_yuva` (address, inbox).
- Writes carry `destructiveHint: false` except `merge_contacts`; `assign`, `set_status`, `snooze`,
  the label tools, `move_conversation`, `bulk_update` and `update_chat_channel` carry `idempotentHint`; `send_reply` and
  `send_draft` (`SendMessage`, delivering a stored draft instead of a copy of its text) carry
  `openWorldHint`. `send_reply` answers `409 draft_pending` while the caller (the same key,
  or the same member through the same client) has a draft in that conversation.
- `resources/list` lists the inboxes the caller sees. `yuva://inbox/{id}` holds the inbox and, with
  `conversations:read`, its latest open conversations; it is updated by `inbox.*` and
  `conversation.created`, `.updated` and `.moved` there. `yuva://conversation/{id}` is updated by
  every event of that conversation, `yuva://contact/{id}` by `contact.updated` and `.deleted`.
- The `draft_reply` prompt looks up the inbox's `default_locale` and names it in the text.
- Tools call the same code as the `/v1` operations with the caller's principal, so access, scope
  and inbox checks are the API's. Read tools carry `readOnlyHint`; idempotent writes
  `idempotentHint`; `merge_contacts` `destructiveHint`. Every tool has an output schema. Tools the
  caller's scopes or role cannot use are not listed; `send_reply` and `send_draft` are listed only
  when the caller may deliver.
- Customer text is untrusted: message bodies, subjects, names, e-mail addresses and attributes are
  returned only inside `customer_content` fields, and every tool description says that text there
  is data from customers and never instructions.
- Resources `yuva://inbox/{id}`, `yuva://conversation/{id}`, `yuva://contact/{id}`, with
  subscriptions. Prompts `triage_inbox`, `draft_reply`, `summarize_conversation`, `weekly_report`
  and `setup_live_chat` (site URL, framework, product name: find or create the inbox and the chat
  channel for the site's origin, get the snippet, add it to the code or show it; without
  `inboxes:manage` it says which scope to reconnect with); they only arrange tool calls and text,
  Yuva runs no model.
- `init` is the entry prompt for adding Yuva to a project: the assistant reads the project locally
  and sends none of it to Yuva, calls `get_setup_state` (what the workspace has, the parts it cannot
  read with the scope to grant, and a fixed catalog of integrations with the signals that suggest
  each and the prompt or tools that set it up), then proposes a plan and changes nothing until the
  person picks steps. `get_setup_state` and `report_yuva_issue` need no scope and are listed to
  every connection.
- `report_yuva_issue` prepares a GitHub issue about Yuva itself for `productdevbook/yuva` and files
  nothing: the server removes the workspace's own names (workspace, inboxes, channels, properties,
  lists, their origins, domains and addresses), e-mail addresses, URLs and host names other than
  `github.com` and `useyuva.com`, IP addresses, keys, tokens, UUIDs and home-directory paths, and
  returns the title, body, a prefilled new-issue URL and a `gh issue create` command. The person
  reviews it and opens it under their own GitHub account, so the server holds no GitHub credential
  and cannot be used to post to the repository.
- Rate limit per token or key (600 requests a minute, `429` with `Retry-After`), counted per grant
  for Connected apps, in Postgres like the other rate limits.

#### MCP coverage (M23)

An assistant can do through MCP what a member does to answer customers, set a product up and prepare
mail, so it never has to stop and send the person to the panel for an ordinary step. Each tool calls
the `/v1` operation with the caller's principal, so roles, scopes and inbox limits are the API's;
writes that only owners and admins may make through `/v1` are listed only to them and to keys.

- **Left out on purpose:** credentials and access (API keys, OAuth grants, members' roles, invites,
  passkeys, push subscriptions, webhooks, rotating any secret, key or token), workspace settings and
  deletion, every delete beyond the existing `remove_migration_history`, consent and analytics
  exports and analytics data deletion, migration file uploads, typing and read markers, raw message and
  attachment downloads, SMTP and other passwords, and every publish a milestone keeps for members
  (help articles, What's new entries, incidents under the draft rule). Unsubscribing is a state
  change, not a delete. Moving conversations to the trash and restoring them (#178) are left out
  too.
- **Sender identity (#109).** `update_inbox_sender` (`UpdateInbox` with `sender` only: legal name,
  postal address, registration, contact; owners, admins and keys with `inboxes:manage`). Its
  description says every value comes from the person and is never invented. `list_inboxes` returns
  each inbox's `sender` and `sender_ready`; `get_setup_state` names inboxes with lists but no sender.
- **E-mail channel (#107).** `create_email_channel` (`CreateChannel` with kind `email`): the address and
  name, no SMTP account, as `create_migration` does; the answer carries the forwarding address for
  inbound mail and `channel_settings_url` where the person sets SMTP. `get_deliverability`
  (`GetDeliverability`, and `CheckDeliverability` with `check: true`) returns the DKIM record to add
  and what each check found or is missing; `create_dkim_key` (`CreateDkimKey`) makes the key whose record it lists.
- **Lists and subscribers (#107).** `create_mailing_list`, `update_mailing_list` (`lists:write`,
  owners, admins and keys); `list_subscriptions` (`ListSubscriptions`, addresses and names in
  `customer_content`); `add_subscription` (`CreateSubscription`; `consent.confirmed` false unless the
  person states that the subscriber already agreed, with the statement and where they agreed in their
  own words, where they agreed going into the consent's `reference`); `unsubscribe` (`UpdateSubscription` state `unsubscribed`).
- **CSV import (#107).** `create_list_import` takes the CSV as text (at most 5 MB, the same multipart
  upload the panel sends) with `attestation` and `consent_source`, which the description says to take
  from the person word for word and never to write; `get_list_import`, `list_list_imports`,
  `update_list_import_mapping`, `get_list_import_errors` (the refused rows, `customer_content`),
  `cancel_list_import`. Through `/mcp` the CSV text and `data_base64` below fit only up to about 5 MB
  (the request limit is 8 MiB). `start_list_import` needs `confirm_count` equal to the import's `counts.valid`,
  as the person was shown, and answers `409 count_mismatch` otherwise.
- **E-mail images (#108).** `list_email_images` (`ListEmailImages`) and `upload_email_image`
  (`UploadEmailImage`'s code): `url` (`https` only, fetched through the webhook client that refuses
  private and reserved addresses, 10 seconds, the upload's size and type limits) or `data_base64`;
  the answer is the image id `update_design` takes. Owners, admins and keys, `broadcasts:write`.
- **Designs and broadcasts.** `list_designs`, `get_design` (`ListEmailDesigns`, `GetEmailDesign`,
  `broadcasts:read`), `duplicate_design` (`broadcasts:write`); `pause_broadcast`, `resume_broadcast`,
  `unschedule_broadcast` and `cancel_broadcast` (`destructiveHint`, needs `confirm_name` equal to the
  broadcast's name) with `broadcasts:send`, listed under the same rule as `schedule_broadcast`.
  `request_broadcast_approval` (`RequestBroadcastApproval`) is listed only to an owner's OAuth
  connection with `workspace:manage`, the scope of the workspace row whose broadcast status it
  changes; `/v1` takes the owner's session or OAuth token, and refuses API keys, which have no
  person behind them. Its description says `website`, `sender_description` and
  `consent_description` come from the person and are never invented; the operator's notice names
  the owner's address.
- **Team, contacts, conversations.** `list_members` (`ListMembers`: id, name, role; addresses only to
  owners and admins), so `assign` has ids. `search_contacts` (`ListContacts`), `create_contact`,
  `update_contact` (names and attributes; `contacts:write`), `list_contact_notes`,
  `add_contact_note`, `list_contact_subscriptions`. `start_conversation` (`CreateConversation`): the
  first outgoing message is a draft unless the caller may deliver and `send: true` is given.
  `list_drafts` (`ListDrafts`) lists drafts waiting for review. `create_label`, `create_canned_reply`,
  `update_canned_reply`.
- **Discussions.** `list_discussions`, `get_discussion` (posts in `customer_content`).
  `reply_to_discussion` (`CreateDiscussionPost`) and `mark_discussion_answer` (`AcceptDiscussionAnswer`,
  `ClearDiscussionAnswer`, idempotent) take `messages:write` and an OAuth token, which acts as its
  member; API keys are refused, since a post has no bot author. A post written with a token keeps
  `via` (`discussion_posts.via`) and shows it on `author.via` on `/v1` only. Both are public at once
  and notify followers, so they are listed only when the caller may deliver (`bots_may_send` for a
  `/mcp` token, which the operations also check: `403 bot_sending_disabled`).
- **Setting up the other products.** `create_status_page`, `create_status_component`,
  `create_status_monitor`, `update_incident` (a draft's or a published incident's title and impact;
  text goes through `post_incident_update`); `create_help_center`, `create_help_collection`;
  `create_changelog`; `create_roadmap_board`; `create_funnel` and `analytics_retention`. Each takes the
  scope and role of its `/v1` operation (`status:manage`, `help:write`, `changelog:write`,
  `roadmap:write`, `analytics:manage`, `analytics:read`); creating a help center needs `help:publish`
  and a changelog `changelog:publish`, and help centers, changelogs and boards take no OAuth token,
  only owners, admins and keys. Nothing created here is published. `create_status_monitor` returns the
  push token and URL once, for the person to put in their job.
- The server instructions name each group's tools, and the prompts use them: `draft_newsletter`
  creates a list, imports or adds subscribers and fixes the sender before the draft when they are
  missing, and still never schedules.

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
a reply e-mail does not confirm the address. Such an answer (from the typed address, naming the
visitor's thread, while no contact has the address and DMARC does not fail) joins the visitor's
conversation as their message, marked `unverified_sender` (its `from` is not one of the contact's
addresses, computed when read), and links nothing; once the address is confirmed it is the
contact's and the mark goes away.

The visitor, not the inbox, learns what became of the link. Each request leaves an
`email_confirmations` row (deleted by the event cleanup job 30 days after it expires) and the client contact carries
`email_confirmation` `{email, state, expires_at}`: `sent` (with the link's expiry), `undeliverable`
or `confirmed` (only while that address is still the contact's first). Before mailing, one helper
reads the address's suppression in the workspace, for the chat form and the feedback form alike: a
bounce of any scope is public knowledge (anyone can mail the address) and the row is stored bounced
and shown `undeliverable`, with nothing sent; a complaint or manual suppression stores an ordinary
row and sends nothing, so the answer is `sent` exactly as for a clean address and the endpoint tells
nobody what is on the suppression list. Confirmation mails go out through the server mailer with
their own Message-ID (`confirm.<random>@<sender domain>`), stored on the row with the id a relay
put in its place (Amazon SES names its own in the reply to DATA and in its delivery reports). A delivery report
(inbound DSN after the reply, broadcast and notice lookups, under the same null-sender and
`Final-Recipient` rules, the address compared without case) or an SES notification (server mail,
so no tenant check) finds the row by that Message-ID or the relay's. A permanent failure of its own
address always suppresses the address with scope `confirmations` (the bounce is about the address,
not the row), and marks the row bounced and tells the visitor only while it is unconfirmed,
unexpired, the contact's latest and still their typed address. The scope stops confirmation mails
and nothing else (replies, notices,
broadcasts, list imports and the contact's undeliverable list ignore it; any wider suppression
replaces it and it never narrows one). Such a bounce expires after 30 days (ignored, then deleted by
the cleanup job), so a mailbox that did not exist yet is not locked out. A complaint about a
confirmation (SES notification or ARF report) suppresses the address with the same scope, reason
`complaint`, which never expires, and ends a live link: it stops working and the visitor sees what
the silent case shows. Temporary failures change nothing; no report about a
confirmation opens a conversation or a contact. Every change reaches the visitor's
`/client/v1/realtime` as a stored `contact.updated` frame carrying the `ClientContact`, sent only
when what it shows changed. The row is found across workspaces (`FindEmailConfirmationByHeaderAnyWorkspace`),
an exception to the `workspace_id` rule: the server mailer sends every workspace's confirmations
from one address, so the report arrives at whichever workspace owns that address, and SES names
only the message. The Message-ID is random and given only to the one recipient, the report must name
that recipient, and everything after the lookup is scoped by the row's own `workspace_id`.

### E-mail

Inbound:
1. A Cloudflare Email Worker (`edge/`) receives mail for the support addresses and POSTs the raw
   message with the envelope recipient and sender to `/ingress/email`, signed with HMAC-SHA256
   under `YUVA_INGRESS_SECRET` (one per install; the request format is in `edge/README.md`). The
   `v3` signature carries two MACs: one over the timestamp, both envelope addresses and the
   `Content-Length`, checked with the timestamp (within 5 minutes) before a byte of the body is
   read, so an unauthenticated request never makes the server buffer a body; and one that adds the
   body, checked once it is read. The body must arrive within 30 seconds. The older `v2`, a single
   MAC over the timestamp, both envelope addresses and the body, is accepted while
   `YUVA_INGRESS_ACCEPT_V2` is on (the default until Workers have moved to `v3`); the oldest `v1`,
   which leaves the envelope sender out, only with `YUVA_INGRESS_ACCEPT_V1`, and then the envelope
   sender is not trusted (an empty one marks nothing as a delivery report or automatic). At most
   `YUVA_INGRESS_MAX_CONCURRENT` (8) messages are processed at once per process, counted from the
   signature header check, before the body is read; more are answered 503. A Message-ID already stored for the channel is
   accepted again without a second copy. When the server cannot be reached (5xx, network error, no answer within 20
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
   `References` or a relay rewrites the domain. A relay that replaces our Message-ID (Amazon SES writes
   `<id@region.amazonses.com>`, `email.amazonses.com` in us-east-1, from the id in its reply to `DATA`)
   has that id stored next to ours as `relay_message_id`, and threading, broadcast replies and delivery
   reports match either. Otherwise a new conversation starts, with the
   subject of the mail. A closed, pending or snoozed conversation that gets a reply is reopened.
   The sender is matched to a contact by address or becomes a new contact; mail from a blocked
   contact is refused, mail from the channel's own address is dropped. A thread joins only a
   conversation of the sender's own contact, or, for mail that passed DMARC, one whose active
   participants include the sender's address (below): anyone else who holds one of our Message-IDs (a Bcc recipient, a forward) could
   otherwise write into a customer's conversation. Mail that names the
   thread of another contact's conversation opens a new conversation for its sender, counted
   against the hourly limit, with `related_conversation_id` pointing to the conversation it named;
   the panel shows that link, and nothing is added to the other conversation. `From` can be
   forged, so a reply whose trusted DMARC result fails never joins a conversation that is not
   flagged spam either: it opens a new spam conversation with `related_conversation_id`.
   Participants: the `To` and `Cc` of inbound mail (and the `cc` of members' replies) are kept as
   `conversation_participants` rows (workspace, conversation, lowercased address, role `cc`, status,
   first seen; the contact is found by address when there is one, else the row is a bare address),
   at most 20 per conversation. Never kept: the sender, the conversation contact's own addresses,
   and any address this server receives or sends mail for in any workspace (channel addresses with
   tags and catch-alls, forwarding addresses, `from_address`, `YUVA_SMTP_FROM`). Mail that fails
   DMARC or is automatic adds no one. `From` can be forged wherever DMARC is not enforced (a domain
   without a DMARC record, or an install without `YUVA_INGRESS_AUTHSERV_ID`), and a forged mail
   "from" the customer could otherwise name an attacker in `Cc` and have every reply copied to them;
   so header addresses are `active` only when the mail's trusted DMARC result is `pass`, and
   `suggested` otherwise: shown to members, never copied, never allowed to answer into the
   conversation. A later mail that passes makes a suggested address active. Addresses a member names
   in a reply's `cc` or adds with `POST /v1/conversations/{id}/participants` are active;
   `DELETE /v1/conversations/{id}/participants/{email}` removes one (a later inbound mail naming it
   adds it again). Changes emit `conversation.participants` (data: the conversation id and the full
   list) on realtime and `/v1/events` only: no webhook, nothing on the contact's stream. Mail that
   passes DMARC from an active participant's address and names the thread joins the conversation as
   a message from the participant's contact (created by address like any sender); the
   conversation's contact stays. It passes the same rules as the contact's own mail: blocked
   senders, loop and auto-reply handling, and the hourly sender cap, which counts inbound mail by
   its author. Any other mail from them, and answers from Bcc recipients (never participants), opens
   a new related conversation. `GET /v1/conversations/{id}/participants` lists them with their
   status. The add endpoint answers an address of this server like an invalid one
   (`400 validation_failed`, same text), as do `cc` and `bcc`, so nobody learns another workspace's
   addresses.
   Every e-mail channel also has a forwarding address, `<token>@<YUVA_FORWARDING_DOMAIN>` (16 random
   characters, rotatable), for mailboxes that forward to Yuva instead of moving their MX; ingress
   resolves a recipient on that domain by token before the address lookup. Without the setting there
   is none.
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
   shown, and its DMARC result is used only when the header's authserv-id is
   `YUVA_INGRESS_AUTHSERV_ID` (`mx.cloudflare.net` behind Cloudflare Email Routing); any other
   header could have come with the message, so without a match DMARC is `unknown`. A sender can
   also write a header with the trusted authserv-id, and the receiver need not remove it (RFC 8601
   §5 is a SHOULD and Cloudflare does not document doing it), so when more than one header carries
   that id DMARC is `fail` if any of them fails and `unknown` otherwise, and a deliverability probe
   reads as untrusted. This needs nothing from the MTA beyond putting its own header on top; the
   edge Worker cannot do better, since it sees the same headers and observes no verdict itself. A new
   conversation whose first mail fails DMARC is flagged `spam`, which keeps it out of
   lists and counts (they have a spam view) and away from automatic replies; contacts can be
   blocked.

Outbound: SMTP per channel (works with SES, Postmark, any relay); the password is stored
encrypted under the master key and never returned, and it is only sent over TLS. The SMTP host is
resolved once per send and refused when it resolves to a loopback, private or other non-public
address (unless `YUVA_SMTP_ALLOW_PRIVATE`) or to a link-local or cloud metadata address (always),
like webhooks, so a channel cannot probe the server's network; a literal address or `localhost`
that would be refused is already refused when the channel is saved. A member's reply
in a conversation that started on an e-mail channel goes out through a River job: `From` is the
channel address (or its per-channel sending address) with its display name, `Reply-To` the channel
address, `To` the address the conversation's contact last wrote from among the contact's own
addresses (or the contact's first address) and never another sender in the thread, `Cc` the
conversation's active participants unless the reply names its own `cc` (`[]` copies no one), `Bcc` never
as a header, `Subject` `Re: …`, and `In-Reply-To` and
`References` keep the thread. A reply (`createMessage`, `sendMessage` for a draft, MCP
`send_reply` and `send_draft`, so members, keys with send rights and assistants while sending is
allowed) may set `cc` and `bcc`, at most 20 addresses each. Each address is validated, may not be
an address of this server (answered like an invalid address), and is refused with `409 email_undeliverable` when it has a suppression
of scope `all` (a hard bounce, or a complaint about transactional mail); a broadcast-only
suppression (a complaint about or unsubscribe from a broadcast) does not stop a reply, since the
person is part of a conversation, not a list. Default participants with a suppression are left out
silently. The contact's address is dropped from both lists and an address in both stays in `cc`.
Addresses in `cc` become participants. `bcc` is stored on the message's e-mail row for members:
it is in the message's `email.bcc` only in answers to `/v1` requests (message list, create, send),
never in events (so never in `/v1/events`, realtime, webhooks or the contact's stream), never on
`/client/v1` and never in the stored or sent headers; the sender passes `To`, `Cc` and `Bcc` as
envelope recipients (suppressions are checked again at send time and drop a copy, not the
reply), and a relay that refuses one recipient fails the whole send. The stored e-mail keeps
`to`, `cc` and `bcc` for the thread view. Automatic mail copies no one: greetings, rating
requests, chat continuity and automation rule replies, since an automatic Cc to addresses taken
from inbound headers would turn every rule into a way to send mail to strangers. `Reply-To` stays the channel address. Notes are never sent. The message carries its delivery state
(`queued`, `sent`, `failed` with the error), reported live as `message.updated`; temporary SMTP
errors are retried, 5xx replies fail at once. An e-mail channel can greet new conversations once
per contact within a set interval; the greeting is a `system` message with
`Auto-Submitted: auto-replied`.
Mail through Amazon SES can carry the SES tenant of the channel's inbox (see Broadcasts and e-mail
designs › Amazon SES tenants): every send path (replies, greetings, rating requests, continuity,
broadcasts, tests, list mails, both probes) adds `X-SES-TENANT` and `X-SES-CONFIGURATION-SET` through
one helper when the account used is an SES endpoint; server mail (`YUVA_SMTP_*`) never does.

Bounces: an inbound delivery report (`multipart/report; report-type=delivery-status`) to a channel
address counts only when its envelope sender is empty (the null reverse-path RFC 5321 §4.5.5
requires of delivery reports), it names a message we sent from that workspace (its Message-ID in the
returned headers, `In-Reply-To` or `References`) that went to exactly one address, its
`Final-Recipient` is that address, and no other stored mail that refers to that Message-ID went to,
came from or was copied to another address (otherwise someone else holds the Message-ID and could
name it); then it fails the message and marks the recipient undeliverable. An ARF complaint about
transactional mail follows the same one-recipient rule. Anything else is stored as an ordinary automatic mail from its sender and changes
nothing; a blocked sender is refused before the report is looked at. Amazon SES bounces and
complaints arrive at `/ingress/ses` through SNS: the SNS signature is checked against the AWS
certificate, only topics listed in `YUVA_SES_TOPIC_ARNS` are accepted, a subscription is confirmed
only on an `sns.<region>.amazonaws.com` URL, a notification whose `Timestamp` is more than an hour
old (or ahead) is refused as a replay, and a notification counts only for a message we sent
and for its recipients (`To`, `Cc` and `Bcc`; only a `To` recipient fails the message). Suppressions have a scope: a hard bounce or a complaint about a reply
stops all mail to the address; a complaint about a broadcast stops broadcasts only (see Broadcasts).
Undeliverable addresses are shown on the
contact, replies to them are refused until a member clears them.

Mail the server writes itself (sign-in code and pause notice, invite, member notification, address
confirmation, discussion digest, rating request) is `multipart/alternative`: the text part from
`api/internal/mail/templates/<locale>/<name>.txt`, unchanged, and an HTML part from `<name>.html`
in the same locale (en, tr, de; English when the locale has no text template) inside one shared
layout (`templates/layout.html`, Go `html/template`, so every value is escaped). The layout is
table-based with inline CSS, at most 560px wide, a system font stack, no remote images, fonts or
tracking; a `prefers-color-scheme` media query is the only `<style>` and only darkens it. Mail to
members and people signing in carries the "Yuva" wordmark in the panel's brand colour; mail sent to
a contact on behalf of an inbox (address confirmation, discussion digest, rating request) carries
the inbox's name and `branding.color` (else `#2563eb`) and never names Yuva, with a readable
button and text colour derived from it. `List-Unsubscribe` links also appear as footer links. Both
parts are quoted-printable UTF-8 through the server mailer (`YUVA_SMTP_*`); the rating request goes
out through the inbox's e-mail channel like a reply, its HTML stored as the message e-mail's
`full_html` while the message itself stays text. List confirmation and welcome mails (`list_confirm`,
`list_welcome`) are rendered the same way under the inbox's brand and go out through the list's e-mail
channel, never through the server mailer. The continuity mail renders only its subject (its
body is the members' replies) and push payloads stay text.

### Live chat and async messaging (web)

`sdk/js` ships a framework-free web component, `<yuva-chat>`, in Shadow DOM, translated with Lingui,
loaded by one script tag. Two layouts: a floating launcher for websites, and `embedded` for an
inline thread inside a product's own panel.

- The panel is the "lined" design: rules 20px in from each side with a "+" where section rules meet
  them, numbered mono section labels, Geist and Geist Mono when the host page loads them (else the
  system fonts; the widget never fetches fonts), the channel's colour as accent, `prefers-color-scheme`
  for light and dark. It opens on Home (availability, the latest conversation, a button for a new
  message); a thread follows shadcn's message and bubble patterns. The e-mail confirmation card reads
  `email_confirmation` on the client contact and the `contact.updated` frame: `sent` (until
  `expires_at`, then it asks for the address again), `undeliverable` (an inline field to fix the
  address) and `confirmed` (a quiet line).
- Each panel language is its own content-hashed chunk (`/widget/yuva-panel-<locale>-<hash>.js`), so
  `/yuva-chat.js` carries no catalog and a visitor downloads one.
- Home offers apps inside the same panel, loaded on first use from a chunk of their own
  (`/widget/yuva-chat-apps-<hash>.js`, with one catalog chunk per language): Feedback
  (`POST /client/v1/feedback` with the inbox's `feedback_categories`, files as multipart), Status
  (`inbox.status`, and the page JSON at `/client/v1/status/{slug}` with `inbox.status.slug`, which works on a custom domain too)
  and Get updates (`/client/v1/channels/{key}/lists` and `/subscriptions`). A tile appears only when
  its API answers for the channel; a help center has no client API yet and has no tile. The Roadmap tile
  (`inbox.roadmap`, public boards only) and the What's new tile (`inbox.whats_new`, with an unread count) appear when
  their APIs answer; `announce_updates` on the channel puts the update count on the launcher badge.

- The server hosts the widget scripts itself at `/yuva.js`, `/yuva-chat.js` and `/yuva-docs.js` (cached
  five minutes, ETag). What a script loads on first use (the discussion thread, composer and preview of
  `/yuva-docs.js`) is a content-hashed chunk at `/widget/<name>-<hash>.js`, cached as immutable; a
  request for a previous build's hash of a known chunk is redirected to the current one.
- Public channel settings come from `GET /client/v1/channels/{key}` without creating a contact.

- A `chat` channel has a public key (not a secret; it is embedded in pages and can be rotated), the
  exact origins whose pages may use it, whether anonymous visitors may chat, whether to ask for an
  e-mail address when nobody is available, a greeting and launcher overrides. `/client/v1` answers
  browsers only from origins some chat channel allows (CORS) and then checks the session's own
  channel: a request for a chat channel must carry one of its origins, and one without an `Origin`
  header is refused. The server's own origin (`YUVA_PUBLIC_URL`) counts as allowed for every chat
  channel, so the panel can show a live widget (the setup page's preview) without changing the
  channel; a same-origin request without `Origin` counts as that origin only when it carries
  `Sec-Fetch-Site: same-origin`. Session starts and contact writes are rate limited per IP address (per /64
  for IPv6) and per channel, counted in each process; finished windows are swept every minute and
  at most 100,000 are open at once (new keys beyond that are refused until a sweep). An IP address
  starts at most `YUVA_ANONYMOUS_CONTACTS_PER_HOUR` (20) new anonymous visitors per channel and
  hour (`429 anonymous_limit`); resuming a visitor does not count.
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
  a connection that resumes with `last_event_id` gets one `inbox.updated` frame with the current
  settings and status right after `ready`, whatever it missed while away.
- If the contact has left when a reply arrives and an e-mail address is known, the reply is e-mailed
  after a delay; their e-mail answer continues the same conversation. A River job checks the
  conversation `YUVA_CHAT_EMAIL_DELAY` (5 minutes by default) after a member's reply and after the
  contact's last connection closes: the members' replies the contact has not read, once the oldest
  is that old and the contact has been gone that long, go out as one e-mail through the inbox's
  e-mail channel, threaded so an answer finds the conversation, at most one e-mail per delay. Notes
  are never sent. The subject is the inbox name and a fixed "new reply" phrase in the contact's
  language (else the inbox's), never visitor text: the e-mail may go to an address nobody has
  confirmed. The contact's last seen time is saved before their connection is removed, so a check
  never finds them both disconnected and not recently seen.

### In-app messaging (mobile)

- `sdk/swift` (Swift package `YuvaKit`): a headless client plus SwiftUI screens (conversation list,
  thread, composer, attachments, feedback form, the rating card and the status line), mailing list
  subscriptions, opt-in shake to send feedback, and analytics (`YuvaAnalytics`).
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
  `GET /v1/contacts/{id}/presence` answers the same question on demand.
- Push sent by Yuva (optional, off by default, so hosts that push after the webhook keep working):
  an `app` channel's `push` holds an APNs token key (`.p8`, key id, team id, bundle id, `sandbox`
  or `production`) and an FCM service account (JSON key file, project id), each sealed under
  `YUVA_MASTER_KEY`, write-only (the answer says which are `set`; an absent key or file keeps
  the stored one, an empty one removes it; changes are audited as `channel.credentials_changed`), plus `show_content`. The SDKs register
  the device with `POST /client/v1/push-devices` (platform, token, the app's bundle or package id)
  on every start and token change and remove it on sign-out (`DELETE …/{id}`); a contact keeps
  its 20 newest devices, a token registered by another contact of the channel moves to the new
  one (its registration time restarts), and devices follow a contact merge. A member's or bot's reply that is not e-mailed queues a
  River check when the contact has a device on an app channel of the inbox with push on. The check
  pushes the newest reply the contact has neither read nor been pushed (`push_through` on the
  conversation) and was created after the contact's first device registration (so turning push on
  never pushes old history), unless they have a live `/client/v1/realtime` connection; a conversation is pushed
  at most every 30 seconds (later replies wait for the window and go as one push) and a newer push
  replaces the older one (`apns-collapse-id`, FCM `collapse_key` and `tag` are the conversation
  id). The contact's last connection closing, or lapsing, queues the same check for replies that
  arrived while they were connected. The push's title is the inbox name, its body the start of the
  reply (140 characters) when `show_content`, else a fixed "new reply" line in the contact's
  language; the badge is the contact's number of unread conversations in the inbox; the payload
  carries `yuva_conversation_id` exactly as a host-sent push does. iOS devices whose app id is not
  the channel's bundle id are skipped. One River job per device, naming the contact it was meant for (a token that moved since gets
  nothing), sends it with the standard library
  (an ES256 provider token for APNs over HTTP/2, an RS256 assertion exchanged at Google's token
  endpoint for FCM HTTP v1; both cached in memory). APNs 410 and FCM `UNREGISTERED`
  delete the device. APNs `BadDeviceToken` and `DeviceTokenNotForTopic` delete it only once an
  APNs push has succeeded with the channel's current APNs settings; before that they usually mean
  the wrong environment, so the token stays and the answer becomes `push.last_error`. 429, 5xx and
  network errors retry (5 attempts); other answers (a refused key, FCM `SENDER_ID_MISMATCH` for a
  service account of another project) keep the tokens and are kept on the channel as
  `push.last_error` until the next success or save.

### Webhooks

Endpoints belong to the workspace (all events) or to one inbox (that inbox's conversations, and
contact events for contacts with an external id or a conversation there). Each has a URL, the event
types it subscribed to, whether `message.created` includes members' notes (never by default;
internal events are never sent), an enabled flag and a signing secret: 32 random bytes shown once
as `whsec_<base64>`, stored encrypted under the master key, rotatable. After a rotation the old
secret keeps signing next to the new one for 24 hours.

Events: `conversation.created`, `conversation.updated`, `conversation.rated`, `message.created`,
`feedback.created`, `contact.updated`, `contact.deleted`, `subscription.created`, `subscription.updated`,
`broadcast.updated`, `discussion.stage_changed`, `changelog.entry.published`,
`changelog.entry.unpublished`, `changelog.entry.deleted`, `issue_link.created`, `issue_link.updated`,
`issue_link.deleted`. Payloads follow Standard Webhooks: `{type, timestamp,
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

Languages: the panel ships 47 languages, one list in `web/src/localeList.ts` that `web/lingui.config.ts`
and the loader both read; adding a language is one line there plus its six catalogs
(`messages`, `analytics`, `designs`, `help`, `roadmap`, `changelog`). Catalogs load lazily per locale (`import.meta.glob`), only the
active language is fetched, and the service worker precaches just `en` and `tr`. Before sign-in a `lang`
query parameter (the marketing site's choice, matched the same way, unknown values ignored) wins on the
first load, is remembered like a picker choice and is removed from the address bar; otherwise the
`yuva_locale` cookie, then the saved choice, then the browser's languages pick the best match (exact, alias such as `zh-TW` or `no`, base language, then
`en`); after sign-in the person's `locale` rules, and every picker lists languages by native name and
saves to it. Every language change also writes `yuva_locale` (locale code, `Path=/`, one year, `SameSite=Lax`,
`Secure` on https): with `Domain=<locale_cookie_domain>` from `GET /v1/version` when the server reports one, so a
site on the same parent domain shares the choice, otherwise host-only. The write waits for `/v1/version` (start-up
never does) and a host-only cookie is expired once a domain is known. `<html lang>` follows the language and `dir` is `rtl` for `ar`, `he` and `ur`; shared
shells use logical Tailwind utilities (`ms`, `pe`, `start`, `end`, `rtl:`). Dates, numbers and
relative times format with the active locale.

Home and setup (M17): owners and admins open `/` on Home, agents on the inbox; the managers' inbox
lives at `/conversations`. Home's own content is one request, `GET /v1/home` (member sessions
only), for a period of `days` (7, 14 or 30, by default 14) ending today in `timezone`, optionally
narrowed to one inbox with `inbox_id` (`404` for an inbox the member cannot see). It answers the
period and the previous one of the same length; inbox counts, open feedback and the five
conversations that have waited longest (open, not spam, with a contact message after the last reply
from a member or bot; notes and automatic replies are not replies), with their contact, channel kind
and last contact message; the period's activity (conversations started, first replies with their
median time and ratings, counted as `GET /v1/stats` counts them) in total, for the previous period
and per day, each day's conversations split per inbox; per inbox its open conversations and the
period's conversations and ratings; open conversations per assignee; each status page with its
state and its visible components' worst state and uptime per day of the period; the five published
ideas with the most votes that are not shipped or closed; the next five of scheduled broadcasts,
open incidents, maintenance not completed and scheduled what's new entries; visitors today, in the
last 30 minutes and per day for seven days for each property the member may see; the last sent
broadcast's recipients, sent, clicked and replied (there is no open tracking); and, for owners and
admins, the setup checklist, which `inbox_id` does not narrow. Every block is gated like the
endpoint it mirrors: only the inboxes the member can see, and ideas, broadcasts, what's new and
analytics only when the server has them. At most fourteen queries whatever the workspace's size,
each bounded by the open backlog, the period or the workspace's configuration, never by its
history: conversations started are found through the activity index, ratings through the rated
index, first replies through the messages' time index, uptime from the status component spans. It
is read from the counters, rollups and indexed tables the server already keeps, so it spends none
of the workspace's analytics query slots; blocks the member may not see are left out. What the
shell already loads on every page stays out of it and Home reads it from there: inbox names and
colours (`GET /v1/inboxes`), members and their live presence (`GET /v1/members` and realtime), and
mentions (`GET /v1/me/mentions`, whose seen state changes live); the conversation a teammate is
looking at is named from the open conversations list, as in the team menu. The checklist is the same setup state
the MCP `get_setup_state` tool returns. `/setup` is one hub of cards (live chat, e-mail, mobile
app, documentation pages, analytics, newsletter); a card that is set up links to its place, one that
is not runs its flow inside the setup layout, and `/analytics/new` opens the same analytics flow. The
panel no longer jumps to `/setup` on its own; Home shows what is left.

E-mail HTML has two views, both in a sandboxed `srcdoc` iframe under a CSP of its own (no
scripts, no forms, images only from `data:`, the server's attachments and, once the member allows
remote images, http(s)):
- Reading: the sanitized HTML (`html`, `full_html`; bluemonday's UGC policy, so no
  styles or colours survive) drawn in the panel's own type and theme colours, light or dark, redrawn
  when the theme changes. It never scrolls sideways: tables, `width` attributes and images are held
  to the frame's width and long words break. Table cells are padded, rows divided by a thin line
  in the theme's border colour, and header cells bold. Images sit on a light plate so dark-on-transparent
  logos stay visible in the dark theme.
- Original (the default when the message has one): `original_html` from `GET /v1/messages/{id}/email`,
  sanitized with a second policy that keeps inline styles limited to an allowlist of CSS
  properties and the colour, background and size attributes, never `<style>` blocks, scripts,
  forms or external stylesheets. In the light theme it is drawn on white, as the sender designed
  it. In the dark theme it is darkened automatically before it reaches the frame (the frame runs
  no scripts): colours from inline styles and `bgcolor`/`color` attributes are mapped in OKLCH,
  light backgrounds to dark ones of the same hue, text lightened until it reaches WCAG 4.5:1
  against the background it sits on, borders alike; elements without a colour take the theme's;
  images are left as they are. A button shows the mail as sent, on white, when the conversion
  gets one wrong. Content wider than the frame is scaled down to fit (to 60% at most) and only
  beyond that scrolls inside the frame, never the page; a soft fade on the edge that has more to
  scroll shows that it does.

- Which e-mail view opens first is a per-device preference next to the theme (Settings ›
  Appearance: Original or Reading, Original by default); each message can still be switched.
- Setup: a full-screen, two-column page in the sign-in design (steps and form on the left, a live
  preview on the right) takes an owner or admin from an empty workspace to its first
  conversation: name the inbox, pick a channel (website chat, e-mail, app or documentation site),
  install it (snippet, forwarding address, SDK lines or the documentation elements), then wait live
  for the first message, which the preview's own chat widget can send. A documentation site asks
  for the site's name and address, creates a chat channel named after it with that origin and
  anonymous visitors allowed, and shows the elements for plain HTML, Nuxt and Fumadocs/React filled
  with this server's URL and the channel key, with a link to the guide. Nothing is created until the channel step is submitted: the name, language,
  time zone and channel fields live in a per-tab draft (`/setup/new/...`), then the inbox and its
  channel are created with `Idempotency-Key`s; a failed channel POST keeps the inbox id in the draft so a
  retry sends only the channel, and Cancel deletes an inbox that got no channel. The inbox settings
  start the same flow for chat, app or docs on an existing inbox with a trimmed step list (sites or
  platforms, install, first message), and the last step offers inviting the team and the inbox
  settings. Home's checklist links to it (it no longer opens on its own), and adding an
  inbox later uses the same page (`/setup/new`). E-mail has its own five steps (Address, Replies,
  Receiving, Test, Done; after Name and Channel for a new inbox): the channel is created when
  Replies is submitted (Amazon SES, another SMTP server, or receive only), the SMTP password lives
  in component state and never in the draft, and an `email_address_taken` answer sends the user back
  to Address. From Receiving on the channel exists, earlier steps are unreachable and Cancel reads
  "Finish later"; Test reuses the deliverability check, the SES tenant status and the DKIM section.
  The inbox settings' "+ E-mail" opens this flow; only "+ API channel" is still a dialog.
  Adding a webhook endpoint is a three-step wizard (`/setup/webhook`, `?inbox=` locks the inbox:
  Endpoint, Events, Done) with a per-tab draft; the endpoint is created when Events is submitted
  (a private or invalid URL sends the user back to Endpoint), and the signing secret is shown once
  on Done and never stored in the draft.
  A mailing list is a four-step wizard (`/setup/newsletter`, `/sender`, `/rules`, then Done; "New list" in
  Broadcasts opens it): list, sender (the inbox's e-mail channels, "None for now", or a detour to
  set up e-mail that returns to Sender with the per-tab draft kept), sign-up rules; `POST /v1/lists`
  runs when the rules are submitted, `name_taken` sends the user back to the first step. A list's
  settings are sections that save on their own: name and description with Save, the sender and each
  sign-up switch at once with a toast.
  A new API key is a four-step wizard (`/setup/api-key`: Name, Access, Review, Done) with a per-tab
  draft; Access offers the presets Read only, Support bot, Everything and Custom, and inboxes
  (workspace-wide scopes drop out when inboxes are limited). The key is created when Review is
  submitted; its secret and a `curl` line are shown once on Done and never stored in the draft.
  The key dialog only edits name and bot.
- Connect an assistant (Settings › Connected apps): one page per client (Claude, Claude Code, Codex,
  ChatGPT, Cursor, VS Code, other MCP clients) with the server's own `/mcp` URL, a copyable command
  or config, install links where the client has them, and the steps; a new grant shows up there live.
- Main screen, in the shape of a messaging app: a narrow icon rail and three columns on a desktop.
  Every page except setup and the OAuth consent page lives in this shell: the rail picks what the
  left column lists, and the chosen item opens in the main area (nothing chosen shows a short
  placeholder there). Contacts (`/contacts`) lists the directory with its search and filters on
  the left and opens a contact's page in the main area; Settings (`/settings`) lists the settings
  groups on the left (You, Inboxes, Team, Developer for owners and admins, Workspace) and opens
  each page in the main area, at the same URLs as before.
  The rail switches the left column between Conversations (with the number of unread
  conversations among the loaded ones), Mentions (the notes that mention the member from
  `/v1/me/mentions`, unseen ones marked and counted on the rail from `unseen`), Team (`/team`: each member's presence, what they are looking at
  and their open load from `assignees` in `/v1/conversations/counts`; a member opens in the main
  area with their open assigned conversations and today's replies and closes), Reports (`/reports`:
  today's `/v1/stats` in three pages opened in the main area: replies, closes and first replies;
  ratings, in total and per inbox; per member) and Assistants
  (the pending drafts from `/v1/drafts`, filtered by author kind and inbox, with `total` on the
  rail, above the connected apps from `/v1/oauth/grants`); both lists refresh from the realtime
  events named with their endpoints, and a row opens its conversation at the note or draft
  (`?m=<message id>`, loading older pages until it is there). The rail also leads to contacts and
  settings and holds
  the member's avatar with the availability switch. Each entry has a tooltip and a `G` shortcut.
  On a phone the rail is a tab bar under the left column, and every section has the two screens
  of the conversations: the list first, then the chosen page with a back button. The conversations column has the workspace, new
  conversation and a menu to contacts, setup, the command palette and the shortcuts; a search field (Postgres FTS over people and messages);
  filter chips (All, Unread, Mine, Unassigned, Snoozed, Done, which are the list filters `status`
  and `assignee`, Unread filtering the loaded rows by `unread`) and an inbox picker; a Filter
  popover after the chips narrows by type (Live chat, E-mail, App: `kind=conversation` with that
  `channel_kind`; Feedback, Question: that `kind`) and by site (`site=<origin>`, which keeps the
  feedback and questions sent from that site's pages; the sites come from `GET /v1/docs/sites` and
  the chat channels' origins), kept for the session; then the
  conversation list, most recent activity first, paged as it scrolls and kept current from the
  realtime connection. A row shows the contact, the last message (marked when a bot or the team
  wrote it, replaced by the member's unsent draft or by "typing…"; a team reply in a chat or app
  conversation shows ✓, or ✓✓ once `last_read_by_contact_at` reaches it), the time, an unread mark, the
  channel, the assignee and a green dot when the contact of a chat or app conversation is online.
  It also says where the conversation came from: a source badge (Live chat, E-mail, App or API from
  the starting channel's `channel_kind`; Feedback and Question take precedence) and, for feedback
  and questions, the page's host and path, for chat and app conversations the channel's name. A
  contact with neither name nor e-mail is "Visitor · <site host or channel name>" with a neutral
  avatar, never initials; the conversation header shows the same name, badge and place.
  ↑/↓ move through the list, Enter opens, J/K open the next or previous one. Each member can pin
  conversations to the top of their own list and mark one unread again, from the row's context
  menu or the conversation's menu (P, U): `PUT`/`DELETE /v1/conversations/{id}/pin` and
  `POST /v1/conversations/{id}/unread`, kept per member in `conversation_member_states` next to the
  read cursors. The list item carries `pinned_at` for the caller, `pinned=true|false` filters on
  it, a mark shows as `unread` until the member reads the conversation or writes in it, and
  `conversation.pin` and `conversation.read` events reach only that member's connections. Pinned
  conversations of the current filter stand in their own section above the rest. The list also
  filters by where a conversation came from: `kind` and `channel_kind` (several values, repeated
  or comma-separated) and `site`, the origin of a feedback's or question's page. Each conversation
  carries `channel_kind`, the kind of the channel it started on (stored on the conversation when
  it is created and kept when the channel goes or the conversation moves), so a row shows its
  source without another request; a page's URL and title are under `feedback` or `question`. In the middle, the
  open conversation (`/conversations/:id`): a header with the contact, channel, status, assignee
  and the actions (assign or hand off, snooze, labels, close or reopen, more), the thread built on
  shadcn's chat components (message, bubble, marker, attachment, message scroller) with date
  separators. Every item is a chat bubble in a message with header and footer slots, the contact
  on one side and the team on the other, never a wide card: an e-mail shows its subject on the
  first message and whenever it changes, then the body, its HTML (original or reading view) in a
  bubble that grows to the thread's width, with sender and tags in the header and time, view
  toggles, quoted text, remote images and the `.eml` download in the footer; a note is a bubble
  from its author on the team's side in the note colour, marked in its footer as seen only by the
  team; feedback is a bubble from the contact with its category in the header; attachments of any
  message use the attachment component, images as thumbnails; drafts are outgoing bubbles with
  Send / Edit / Discard; events and dates are markers. The thread sticks to the newest message
  while the member is at the bottom and loads older pages on scrolling up; the composer is at the
  bottom. On the right, the contact details, toggled
  from the header; below 1200 px wide they open as a sheet, never as a third column, and the
  header drops the inbox name, the assignee, the labels button and the close button's text when
  the conversation is narrower than 36rem, so the contact's name keeps its room. On a phone the line under the name
  (source, place, status) wraps instead of being cut, and assign, snooze and search move into the
  more menu next to close. On a phone the list and the conversation are
  two screens with a back button. With nothing open the middle shows what the team did today
  from `GET /v1/stats`: replies sent by members, conversations closed and the median time to the
  first reply, per member too, over the inboxes the caller can see. Nothing is stored for it: it
  is counted from `messages` on each request (indexed by workspace and time) for a window of at
  most 366 days; "today" starts at midnight in the `timezone` the panel sends (the browser's),
  since members and workspaces have no time zone of their own.
- Acting on a conversation never opens another one. Close, snooze and hand-off apply at once and
  offer an undo in a toast. A reply is held for a few seconds before it is posted, shown in the
  thread as sending, so it can be undone; "send" also sets `pending`, "send and close" sets
  `closed`. A bot's draft is offered as the suggested reply. Live conversations are different: a
  conversation from a chat or app channel whose contact is online right now is talked through.
  Its replies are sent at once (no hold) and leave the status `open`. In every chat or app
  conversation, online or not, Enter sends and Shift+Enter starts a new line; in e-mail
  ⌘/Ctrl+Enter sends and Enter starts a new line. A note takes the same keys as its conversation.
  The hand-off menu shows each teammate's presence and open load
  (`assignees` in `/v1/conversations/counts`, over the inboxes the member can see). Everything
  else is in a command palette.
- Conversation: reply and note in one box, canned replies on `/`, attachments (picked, pasted or
  dropped anywhere on the conversation), an emoji picker (frimousse, MIT; its Emojibase data is
  served by the server itself under `/emojibase/`, loaded on first use and not precached),
  keyboard shortcuts, contact details with identity attributes, the contact's other conversations,
  the conversation's media, links and documents (from the loaded messages), channel delivery
  state, ✓/✓✓ read receipts on the team's chat and app messages, actions on each message (reply
  with a quote, copy, forward to a teammate as a note that mentions them), a search within the
  loaded messages of the open conversation that highlights the matches and jumps between them, and a notice when another member types a reply in the same conversation or has it open.
  The panel reports the conversation it shows with a `viewing` frame on `/v1/realtime`; the server
  keeps it on the connection's row (see Member notifications), sends a `viewing` notice to the
  other members who can see the conversation when a member opens or leaves it (hiding the page and
  disconnecting count as leaving), and answers the frame with the members already there.
- Settings: inboxes, channels, members and access, labels, canned replies, business hours,
  auto-replies, webhooks, API keys, retention and deleting the workspace; deleting one's own
  account in the profile.
  A channel has its own page, `/settings/inboxes/:inboxId/channels/:channelId`, with one card per
  topic (general, receiving, replies, Amazon SES, broadcast sending, deliverability, DKIM,
  auto-reply for e-mail; sites, visitors, look, discussions, install for chat; access, install
  for apps; settings for API channels; remove for all). Each card saves on its own and sends
  only its fields on top of the latest stored channel, so another card's unsaved text and the
  stored SMTP passwords are untouched; cards with only switches save on change. The inbox page
  lists channels as links to it; the add dialog is only for creating.
  A webhook endpoint page works the same way: endpoint (URL, description), events, sending,
  signing secret, deliveries, attempts, how to verify and remove, each saved on its own.
- Installable PWA with Web Push (VAPID), so members get notifications on phones without a native
  app. E-mail notifications as a fallback (see Member notifications).
- Sign-in with an e-mailed one-time code and passkeys. Sign-up is a server setting,
  `YUVA_SIGNUP`, reported to the panel without sign-in on `GET /v1/version` (`signup`). `invite`
  (the default): the first owner is created with `yuva bootstrap`, everyone else is invited, and
  a code goes only to a person or an invited address. `open`: a code goes to any address (same
  rate limits, same fixed delay, same `202`), verifying it creates the person with no workspace,
  and `POST /v1/workspaces` lets a signed-in person create a workspace they own, through the same
  store path as `yuva bootstrap`. The workspace records who created it and from which IP address
  (IPv6 /64); a person may have `YUVA_SIGNUP_WORKSPACES_PER_PERSON` (3) live workspaces they
  created (`403 workspace_limit`) and one address may create `YUVA_SIGNUP_WORKSPACES_PER_IP` (5)
  in 24 hours (`429 rate_limited`). Under `invite` that endpoint answers `403 signup_closed`.
  A code request carries the sign-in page's `locale`, kept on the code row: for an address with
  no person and no invite it is the code mail's language and the new person's locale; a person
  keeps their own and an invite's locale wins.
  `YUVA_OPERATOR_EMAIL` (comma-separated, unset = off) tells the server's operator about
  growth: a new person created by verifying a code (no person, no invite; readers marked) and a
  workspace created with `POST /v1/workspaces` are written as an `operator_notices` row in the
  same transaction (address, workspace name, client IP, and the country only from `CF-IPCountry`
  when `YUVA_CLIENT_IP_HEADER` is `CF-Connecting-IP`, since Cloudflare overwrites it). A River
  job, periodic every minute, sends one English e-mail (`operator_digest`) to every address with
  the unsent rows, at most once per 5 minutes after the last one, so a burst becomes one mail.
  Sent rows are deleted after an hour; they are kept only to know when the last mail went out.
  No conversation, message or contact content goes into it.
  The panel follows `signup`: with `open` its sign-in page says a new address gets an account
  (`/?signup=1` opens it as "Create your Yuva account"), a person with no workspace is asked to
  create one (name prefilled from the e-mail domain) and goes on to setup, and the workspace
  switcher offers "Create workspace" (`/workspaces/new`); with `invite` the page says the server
  is invite only and a person with no workspace sees that they are not in one yet.
  A person whose memberships are all
  gone can still sign in, to see that and to delete their account. Sign-in and invitation e-mails are
  sent directly, not through the job queue, so a code never lands in job arguments; a code request
  answers after the same fixed delay for every address and its mail goes out after the answer, so
  timing does not reveal members. Each code takes five attempts; after 20 wrong codes for an
  address within 24 hours its codes are refused (`429 sign_in_paused`) and the member gets one
  e-mail about it. Passkeys are not affected.
  The sign-in page asks for the e-mail first and sends people of a domain claimed for single
  sign-on to its identity provider (see Single sign-on (M33)).

### Member notifications

Six events notify members: the first message of a conversation, from the contact, in a `live`
or an `async` inbox (every member with access is a candidate); a later contact message in a
conversation assigned to the member, or in an unassigned one (every member with access);
someone else assigning a conversation to the member; and someone else mentioning the member in a
note. A note names its mentions as member ids (`mentions` on `POST .../messages`, each a member
who can see the inbox), stored on the message; the server does not parse `@` in the text. A River job queued in the transaction that
stored the message picks the recipients. Nobody is notified about their own action, about a
conversation marked spam, or about a conversation their open panel shows: the panel reports it
with a `viewing` frame on `/v1/realtime`, stored on the connection's row and trusted while the
connection is fresh (75 seconds). A member set to `away` gets only the events about
conversations assigned to them and mentions.

Each member chooses push and e-mail per event, and can override events per inbox. Defaults by
role: push for everything, except that agents get no push for new `async` conversations and for
messages in unassigned conversations (owners and admins triage those); e-mail only for messages
in conversations assigned to the member, for assignments and for mentions, for every role, since
being assigned or asked makes anyone responsible.

`GET /v1/me/mentions` lists the notes that mention the calling member, written by someone else,
newest first, in the conversations the member can see now (losing an inbox hides its mentions), each
with a preview of the note, its author and a conversation summary. A mention is `seen` once the
member's read position in that conversation (the same cursor as `unread`, moved by reading or
writing there) reaches the note; there is no separate mark. `unseen` in the answer counts the
unseen ones over all pages. Mentions belong to a member, so the list needs a member session; keys and
OAuth tokens get `403 member_session_required`. A partial GIN index on `messages.mentions` for notes
serves it. A client keeps it current from `message.created` (a note whose `mentions` holds the
member), `conversation.read`, `conversation.moved` and `inbox_access.changed`.

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

While the panel is open, each browser can also alert in the tab itself (Settings ›
Notifications, off by default, kept on the device): while the tab is hidden, a contact's new
message in a conversation assigned to the member or to nobody (only the assigned ones while away)
plays a short sound and, once the member has allowed it with the button there (never asked on
load), shows a desktop notification with the same `tag` as the Web Push one, so the two replace
each other instead of stacking.

E-mail fallback: an event with e-mail on schedules one check per member and conversation after the
member's delay (15 minutes by default). If the conversation still has contact messages (or an
assignment, or notes mentioning the member while they have e-mail on for mentions) newer than the member's read position and the previous notification e-mail, one
e-mail through the server's own mailer, in the member's locale, lists them and links to the
conversation and to the notification settings; at most one per member and conversation per hour. A
member's own message or note moves their read position to it, so the e-mail never lists what
they already answered.

### Slack (M32)

A workspace connects one Slack workspace (a Slack team belongs to one Yuva workspace at a time)
through Slack's OAuth v2 with the app the server's operator registers (`YUVA_SLACK_CLIENT_ID`,
`YUVA_SLACK_CLIENT_SECRET`, `YUVA_SLACK_SIGNING_SECRET`, all or none; without them Slack is off,
`GET /v1/slack` answers `available: false` and the other endpoints `409 slack_not_configured`). The
manifest and scopes are in `docs/slack.md`. Self-contained: no shared integrations table (#158 builds
its own).

- **Tables.** `slack_installations` (one per workspace: team, app, bot user, the bot token sealed
  under the master key with the workspace id as context, granted scopes, `reply_command` default
  `!reply`, the last post error), `slack_connect_states` (one pending install per workspace, the
  state's hash, 10 minutes), `slack_inbox_settings` (per inbox: the channel for new conversations,
  the channel for mentions, `include_content` off by default), `slack_threads` (channel and thread
  ts of each post Yuva made, to the conversation), `slack_member_links` (member to Slack user, one
  each way), `slack_link_codes` (a pending code's HMAC, 10 minutes, 5 tries) and `slack_events`
  (event ids seen in the last 24 hours, pruned on insert). All carry `workspace_id`.
- **Install.** `POST /v1/slack/connect` (owners and admins, member session) returns Slack's authorize
  URL with a state that names the workspace and member plus 32 random bytes, of which only the hash
  is stored, as for migration sign-ins. `GET /slack/oauth/callback` takes the state once, needs the
  same member session, exchanges the code at `oauth.v2.access`, requires a bot token with every
  scope (else revokes it, `scope_missing`), refuses a team another workspace has (`team_taken`), and
  redirects to the panel's `/settings/slack?result=…`. Connecting another team drops the previous
  team's channels, threads and links. `DELETE /v1/slack` removes everything and revokes the token
  at `auth.revoke` after the commit (a failure is logged). Audit: `workspace.slack_connected`,
  `workspace.slack_disconnected` (by `system` with `reason: uninstalled` on Slack's
  `app_uninstalled` or `tokens_revoked`), `member.slack_linked`, `member.slack_unlinked`.
- **Posts.** The notification job (`notify`) queues a `slack_post` job, unique by message and
  event, for a contact's first message (`new_conversation`) and for a note with mentions
  (`mention`) when the inbox names a channel for it. A post carries the
  event, the subject, the contact's name, the inbox and `<public URL>/conversations/<id>`, and the
  start of the text (300 characters) only with `include_content`; every value from people is
  escaped (`&`, `<`, `>`), so customer text cannot become a Slack link, mention or `@channel`. A
  mentioned member with a linked Slack user is mentioned as `<@U…>`. A mention goes into the
  conversation's existing thread in that channel, else starts one; every top-level post is recorded
  in `slack_threads`.
- **SLA posts.** `slack_inbox_settings.sla_channel` and `sla_warnings` (migration 00111). The
  `sla_check` transaction that marks a target breached (or warned, only with `sla_warnings`) also
  queues one `slack_sla_post` job, unique by its arguments, when the inbox has an SLA channel; the
  mark happens once, so the post is never repeated across replicas or retries. The post names the
  target, the lateness (or time left), the assignee (mentioned as `<@U…>` when linked) or team, the
  subject, contact, inbox and link, and the latest customer-visible message start only with
  `include_content`. It joins the conversation's thread in that channel or starts one, so replies
  behave as in other threads. Delivery, rate limits and error recording are those of `slack_post`.
- **Outbound.** Only `https://slack.com/api/` (tests point the client elsewhere). Each call is
  tried up to three times: a 429 whose `Retry-After` is at most 10 seconds is waited out, a 5xx or
  failed connection after 1 and 2 seconds. A longer rate limit snoozes the `slack_post` job for
  `Retry-After`; other temporary failures retry the job (5 attempts); a refusal that will not pass
  (`not_in_channel`, `channel_not_found`, `invalid_auth`, …) is recorded as the connection's
  `last_error` instead, cleared by the next post that succeeds.
- **Events.** `POST /ingress/slack` verifies `X-Slack-Signature` (HMAC-SHA256 of
  `v0:<timestamp>:<body>` with the signing secret, compared in constant time) and a timestamp
  within 5 minutes before parsing, answers `url_verification`, finds the installation by `team_id`
  (and `api_app_id`), claims the `event_id` in `slack_events` and queues a `slack_event` job in the
  same transaction, then answers `200` (Slack retries carry the same id and change nothing). The
  job carries the `team_id` and `event_time` it was routed by and drops the event when the
  installation now belongs to another team; `app_uninstalled` and `tokens_revoked` are also dropped
  when the installation is newer than the event, so a queued uninstall never removes a reconnect.
  The job acts only on a plain `message` in a thread Yuva posted (no subtype, no bot, not the bot
  user).
- **Replies.** The Slack user must be linked to a member; the reply runs `CreateMessage` as that
  member's principal with `via: Slack`, so inbox access, the send rules and the events are the
  panel's. Without the command it is a note (Slack mentions of linked members who see the inbox
  become its `mentions`); with `reply_command` (`!` and 1 to 20 letters; Slack keeps `/` for its own
  commands) it is an outgoing message, refused with an ephemeral explanation while the conversation
  is marked spam (a note stays allowed). `client_id` `slack:<channel>:<ts>` makes a retried job
  idempotent. An unlinked user, a member without access to the inbox (or a deleted conversation)
  and any other refusal get an ephemeral explanation and nothing is written. The text is Slack's
  mrkdwn turned into plain text (links as `label (url)`, entities decoded); it is the member's own
  text, not instructions to anything.
- **Member links.** `POST /v1/me/slack/link` (member session, 5 an hour per member, DB-backed
  limit checked before any transaction) finds the Slack user by the member's address (or a given
  one) with `users.lookupByEmail` and DMs a 6-digit code; `POST /v1/me/slack/link/confirm` links
  it. `GET`/`DELETE /v1/me/slack`.
- **Access.** `GET /v1/slack`, `GET /v1/slack/channels` and `GET /v1/inboxes/{id}/slack` need
  `inboxes:read` for keys and tokens; `PUT /v1/inboxes/{id}/slack` owners and admins or
  `inboxes:manage` (a channel must be one the bot is in); connect, change, disconnect and the
  `/v1/me/slack` endpoints need a member session. No new realtime or webhook events: notes and
  replies from Slack emit `message.created` as any other.

### Satisfaction ratings

An inbox can ask contacts to rate their conversations (`ask_for_rating`, off by default). A
contact rates a closed conversation `good` or `bad`, with an optional comment of at most 2,000
characters, once per close: a message from the contact reopens it and the next close allows a new
rating. A close can be rated for 30 days, and only a close after the inbox started asking
(`inboxes.rating_since`, set when `ask_for_rating` turns on and cleared when it turns off), so
turning it on never offers old conversations. Ratings are not stored for spam.

- Conversations keep `closed_at` (set when the status becomes `closed`) and the latest rating
  (`rating`, `rating_comment`, `rated_at`); a rating counts for the current close when `rated_at`
  is not before `closed_at`. The rating is also a `rated` event message in the thread, authored by
  the contact, with the comment as its body; contacts never see event messages.
- Widget and SDKs: `ask_for_rating` on the inbox's public settings, `can_rate` and `rating` on each
  `ClientConversation` (and on the realtime status frame when a conversation closes), and
  `POST /client/v1/conversations/{id}/rating`.
- Members see it on the conversation (`rating`, `closed_at`) and in the thread; `conversation.rated`
  goes to webhooks; `/v1/stats` counts ratings given in its window, in total and per inbox.
- E-mail contacts: two minutes after a member or API key closes an e-mail conversation (time to
  undo), a River job checks that it is still closed and unrated, a member or bot replied in it and
  the address is deliverable, then sends one automatic reply in the thread (`Auto-Submitted`, in
  the contact's language, else the inbox's) with two links, `/r/{token}?rating=good|bad`, once per
  close. The token seals the workspace, the conversation and the close time under the master key,
  so nothing is stored for it and a later close makes it invalid; the request is a message in the
  thread, so members can see the links.
- `/r/{token}` is a page the server renders itself: no scripts or external assets, the inbox's
  name and branding colour, English, Turkish or German by the inbox's language. Opening it stores
  nothing, since mail scanners open links: it shows the chosen rating preselected and a comment
  field, and only its POST rates. A POST for a close that is already rated stores nothing and
  answers the same page with "already rated" (`200`, since it is a browser form); the client API
  answers `409 already_rated`.

### Documentation pages

Products keep their documentation on their own sites (Nuxt Content, Fumadocs, Starlight,
VitePress, plain HTML). Yuva adds two elements to those pages and stores nothing of the
documentation itself. Both use an existing `chat` channel: its public key, its origins and its
rate limits, so a docs site is set up like a site with the chat widget, and the same channel may
serve both.

- **A page** is the page's URL without query and fragment, and its origin must be one of the
  channel's origins (`400 page_origin` otherwise). The title is what the page sent last. The
  elements follow the URL themselves, since documentation sites navigate without reloading;
  a `page` attribute overrides it.
- **Page ratings.** `<yuva-page-feedback>` asks whether the page helped: `up` or `down`. A rating
  alone creates no contact and no conversation: `POST /client/v1/channels/{key}/page-ratings`
  needs no session and adds one to a per-page, per-day counter (`page_ratings`: workspace, inbox,
  channel, page, title, day, up, down). Nothing about the visitor is stored; the element
  remembers the visitor's rating in the browser and a changed mind moves the count (one off the
  latest day that has it), and the endpoint is rate limited per IP address and channel like
  session starts.
- **Page feedback.** After a rating the element offers a text field and, optionally, an e-mail
  address. Sending it is `POST /client/v1/feedback` from a visitor session on the chat channel, a
  `feedback` conversation like the app's, with `page_url`, `page_title` and `rating` in its
  feedback metadata and category `other` unless the visitor picked one. It is answered like any
  feedback; replies reach the visitor by e-mail when they allowed it.
- **Page questions (M11, replaced by discussions).** Until M12 a visitor could ask a private
  question from a page (a conversation of kind `question`) and members published an edited
  question and answer back to the page (`page_answers`). Migration 00029 turned every published
  answer into a discussion and every `question` conversation into an ordinary `conversation`;
  conversations keep `page_url` and `page_title` as columns (page feedback uses them too) and show
  them as `page` on the conversation, so the panel still filters and counts conversations per page.
- **Sites.** A site is the origin of page URLs, per inbox: one product's documentation.
  `GET /v1/docs/sites` lists every origin that has pages in the window (as `GET /v1/docs/pages`
  counts them) and every origin allowed on a chat channel of the visible inboxes, with zeros for
  a site added but not yet used, named by the chat channel that allows it, with its page count and
  summed counts. `site=<origin>` narrows `GET /v1/docs/pages`, `/v1/docs/summary`,
  `/v1/docs/page`, `GET /v1/discussions` and `GET /v1/conversations` to one site.
- **Panel.** A Docs view is per site: it starts from the sites list and lists pages of the inboxes
  the member can see with their up and down counts over a window (7, 30 or 90 days), the share of
  `down`, open feedback and the page's discussion counts now: open threads (published, no
  accepted answer, not locked), unanswered threads (open and nobody replied) and pending posts
  (`GET /v1/docs/pages`), sorted by most `down` (default), most `up`, most helpful (share of `up`,
  first among pages with at least 5 ratings in the window, so a single `up` never leads) or most
  activity (ratings plus open feedback, open threads and pending posts). The list is an
  aggregate, so its cursor is a position within one sort and window. Above it, totals over the
  window and a zero-filled series per day of ratings, page feedback and new threads
  (`GET /v1/docs/summary`) feed the charts. A site picker at the top of the Docs column (all
  sites, or each site by its channel name and host, from `GET /v1/docs/sites`) passes `site=` to
  the list, the summary and the page; changing it while a page is open goes back to the overview.
  A page shows its ratings per day (`GET /v1/docs/page`), its feedback
  (`GET /v1/conversations?page=`) and its threads (`GET /v1/discussions?page=`).
- Ratings are counters, so they send no events or webhooks; page feedback is a conversation and
  sends the same ones as any conversation and feedback.

### Discussions (M12)

Discussions replace page questions and published answers. A documentation page gets a public
discussion area like GitHub Discussions: threads, replies, votes, an accepted answer, followers.
Private help stays private: page feedback, live chat and e-mail are unchanged. A `kind` separates page
threads (`page`) from roadmap ideas (`idea`, M21); page queries and counts see page threads only. Decisions:

- **Model.** Discussions are not conversations: a conversation is one contact and the team,
  private; a discussion is public and has many authors. `discussions` (workspace, inbox, channel,
  page, page title, title, opening post, state, `locked_at`, `answer_post_id`, `answered_at`,
  votes, reply count, pending and hidden counts, source conversation, last activity) and
  `discussion_posts` (discussion, author, body as Markdown, `opening`, `parent_post_id`, state
  `published|pending|hidden`, votes, edited and deleted times), plus `discussion_votes` and
  `discussion_follows`. The thread's body, author and votes are those of its **opening post**, a
  post like the replies, so editing, deleting, voting and moderating work the same for both;
  `discussions.state` follows the opening post's state and is changed with it in one transaction.
  Replies are one level deep: a reply to the thread has no parent, a reply to a reply points at
  it, and a reply to a reply to a reply is refused. The accepted answer is a published reply to the
  thread. State `open|answered|locked|hidden` from the plan is split into `state`
  (`published|pending|hidden`), `answer_post_id` and `locked_at`, since a thread can be answered and
  locked at once. Every table has `workspace_id`; every query filters by it.
- **Authors.** A post's author is a member or a contact (`author_type` `member|contact|anonymous`,
  with the ids nulled on delete: a removed member or a deleted reader shows as deleted, like
  deleted members in conversations). The public sees a display name and initials only: a member's
  name, a reader's person name (live, so renaming the account renames their posts), else the
  contact's name; no e-mail addresses, ids or avatars from elsewhere. `anonymous` is used only for
  threads migrated from published answers, whose visitor was never named.
- **Bodies** are stored as Markdown and served with `body_html`, rendered on the server by goldmark
  (no raw HTML, GitHub-flavoured tables, strikethrough and autolinks) and sanitized with
  bluemonday's UGC policy (`rel="nofollow noreferrer"`, `target="_blank"` with `noopener` on links to other sites), so every client can show it without a
  Markdown library. Titles 3 to 200 characters, bodies up to 20,000.
- **Migration.** Each `page_answers` row became a published discussion on its page, inbox and
  channel: the question as an `anonymous` opening post, the answer as an accepted reply by the
  publishing member (`anonymous` once that member is gone), `conversation_id` kept. Every
  `question` conversation became `conversation`, keeping its page columns (see "Documentation
  pages"). `kind=question`, `/v1/conversations/{id}/publish`, `/v1/page-answers*`,
  `/client/v1/questions` and `/client/v1/channels/{key}/page-answers` are removed from the
  contract and `page_answers` is dropped; this is pre-1.0 and they shipped in M11.
- **Readers.** A reader is a person with a server-wide account (`people`, the table members sign in
  with): one account per Yuva server, signing in by e-mail code or passkey on the server's own page,
  never on the documentation site. `POST /v1/auth/code` with `reader: true` sends a code to any
  address (same rate limits; the code row records it) and verifying that code creates the person,
  whatever `YUVA_SIGNUP` says; a member signing in there is the same person. The reader's
  account page and its popup are panel routes (`/account`, `/account/connect`) served like the
  OAuth consent page, with the person's session cookie.
- **Reader tokens.** `<yuva-discussions>` opens `/account/connect?channel=<key>&origin=<origin>` in
  a popup. Once signed in, the page calls `POST /v1/account/reader-tokens` with the key and the
  origin; the server refuses an origin that is not exactly one of the channel's allowed origins
  (`403 origin_not_allowed`) or a channel with discussions off, and the page posts the token to
  `window.opener` with that origin as `targetOrigin`, so no other page can receive it. A token
  (`yuva_rd_…`, stored as a hash in `reader_tokens` with the workspace, channel, person and the
  session that made it) lives 24 hours, is bound to its channel (its origin check is the
  channel's, like a contact session), and ends with that session (sign-out), with the account,
  or with `DELETE /client/v1/reader-token`. When it expires the element opens the popup again,
  which answers at once while the person is still signed in on the server.
- **Readers in a workspace.** A reader token acts in its channel's workspace as the contact linked to
  the person (`contacts.person_id`, unique per workspace), created on the first write: a contact of
  the workspace with the person's e-mail address and no person is linked, else a new contact is
  made with that address and the person's name. So the team sees the name and e-mail address of
  readers who write in their workspace, and the connect page says so; other workspaces see
  nothing of it. A person who is a member of that workspace with access to the inbox writes as that
  member instead. Host apps with their own login keep using identity tokens: a contact session
  started with an identity token writes as its contact; anonymous visitor sessions only read.
- **Reading is public** and needs no session: `GET /client/v1/channels/{key}/discussions?page=` and
  `…/discussions/{id}`, cacheable for a minute, authors shown by display name and initials only. A
  reader token or contact session of that channel adds the caller's own pending posts and the
  `mine`, `voted` and `following` flags, and the answer is then `private, no-store`. Threads are
  listed per inbox and page, so two chat channels of one inbox that allow the same origin show the
  same threads; the channel decides whether discussions are on.
- **Writing** needs a reader token, an identity-token contact session or a member (`/v1`). Per chat
  channel `discussions: off|moderated|open` (default `moderated`). Moderated: a reader's post waits
  (`pending`) until one of their posts in the workspace has been approved, after which they publish
  at once; identity-token users and members publish at once. Open: everyone publishes at once.
  Marking a post spam hides it and the author's other pending posts and sets
  `contacts.discussions_blocked_at`, after which the author's posts always wait. Pending posts are
  seen only by their author and members, hidden ones only by members. Authors edit their posts for
  30 minutes (members their own without limit) and delete them at any time: a post with replies
  stays as deleted without body or author, the thread goes when nobody else wrote in it. A locked
  thread takes nothing from readers (posts, edits, votes, answers); members still may. The thread's
  author or a member accepts an answer. Rate limits: 10 posts a minute and 60 an hour per author,
  and the client write limits per IP address and channel.
- **Team.** `/v1/discussions` lists threads per inbox, site, page and state; `/v1/discussion-posts`
  with `state=pending` is the moderation queue. Members approve or hide posts (`PATCH`), mark spam,
  lock and retitle threads, reply, vote, accept and clear answers, and start threads. Reading needs
  `conversations:read` (keys and tokens too); writing needs a member session. Agents see the
  threads of their inboxes only; another inbox or workspace answers `404`.
- **From a conversation.** `POST /v1/conversations/{id}/discussion` starts a published thread with
  the title and body the member writes (and optionally an accepted answer by the member); nothing
  of the conversation becomes public. Its contact is the author only when it is linked to a person
  or has an external id in the inbox (they are a known user and can see and edit the thread);
  otherwise the member is. One thread per conversation.
- **Notifications.** Followers are contacts (members see threads in the panel). An author follows
  their thread and any thread they reply to. A published reply (on creation or on approval) and an
  accepted answer add a `reader_notifications` row for every follower linked to a person (except
  the author) and queue a River job per follower and thread, unique by its arguments for
  `YUVA_DISCUSSION_EMAIL_DELAY` (10 minutes by default). The job sends at most one e-mail per
  follower and thread per delay, listing the replies and the answer since the last e-mail, through
  the server's own mailer in the person's locale (else the contact's, else the inbox's), to the
  person's address when they kept `email_notifications` on, or to an identity-token contact's first
  address. Each e-mail carries `List-Unsubscribe` with `List-Unsubscribe-Post: List-Unsubscribe=One-Click`
  (RFC 8058) pointing at `/u/{token}`: the token seals the workspace, thread and contact under the
  master key like rating links; opening the page changes nothing and its button (or the one-click
  POST) unfollows. Reader notifications are kept 90 days.
- **Events.** `discussion.created`, `discussion.post_created`, `discussion.answered` and
  `discussion.updated` (moderation, edits, deletes, lock, a cleared answer) are stored events for
  members who can see the inbox and webhook types under Standard Webhooks like the others, with
  `{discussion, post}` as data. Pending posts send them too, with their state.
- **Reader account.** `/account` on the server, in `web/`, separate from the team panel:
  my discussions, followed threads, notifications, e-mail settings, delete account
  (`/v1/account*`, `DELETE /v1/me`). A person who is also a member reaches the team panel from it.
- **Docs counts.** `/v1/docs/pages` and `/v1/docs/sites` count open threads, unanswered threads and
  pending posts per page instead of open questions and published answers; `/v1/docs/summary`
  counts new threads per day and pending posts now.
- **UI for sites.** `<yuva-discussions>` (web component, Shadow DOM, in `/yuva-docs.js` and
  `useyuva/docs`, replacing `<yuva-page-questions>`; Nuxt and React wrappers as today) and a
  shadcn registry for React sites that want the code in their own design system: components built
  on shadcn/ui primitives and the headless `useyuva` client, sources in `sdk/shadcn/` (MIT), served
  as registry JSON by the site so `npx shadcn add <site>/r/discussions.json` installs them.

### Broadcasts and e-mail designs (M13)

A workspace sends e-mail to many contacts at once: newsletters, product updates, announcements. Every
broadcast is sent on behalf of an inbox through one of its e-mail channels, and a reply becomes a
conversation in that inbox. Plan in issue #25; it ships in steps M13.1–M13.7 in the roadmap.
Decisions:

- **Lists.** `mailing_lists` belong to an inbox: name, public description, `double_opt_in` (on by
  default), `public` (offered on forms and the preference page), `commercial` (marketing content, for the
  consent export), `welcome` (a receipt after subscribing) and the e-mail channel that sends its
  confirmation and welcome mail. Owners and admins manage lists; agents read the lists of their inboxes.
  `mailing_lists.kind` is `list`, or `status` for the subscriber list a status page makes (M19); a status
  list is not offered, audience-selectable or importable and sits outside the opt-out register.
- **Subscriptions and consent.** `list_subscriptions` (list, contact, address, `pending|subscribed|
  unsubscribed`, source `form|identity|api|member|import|preferences`) and an append-only `consent_events` log (what
  happened, when, IP, user agent, page, the label the person saw, the member, key or import). Consent is
  never assumed: API calls and imports declare it (an import needs the importer's attestation and where
  people agreed, stored with every row). Only the person moves an `unsubscribed` subscription back, through
  a confirmation. Unsubscribing from all of an inbox's lists, a complaint and deleting a contact who had
  objected leave an `email_optouts` row (a keyed HMAC of the address, nothing else) that the API and imports
  respect. Pending subscriptions go after 30 days (one that was unsubscribed before and asked again returns
  to `unsubscribed`, so the objection stays); unsubscribed ones and their events after 3 years, leaving an
  opt-out.
- **Signing up.** `<yuva-subscribe>` (`/yuva-subscribe.js`, `useyuva/subscribe`, React and Nuxt
  wrappers, shadcn registry items) on an existing chat channel's key and origins; anonymous forms post to
  `POST /client/v1/channels/{key}/subscriptions` with a sealed form token, a honeypot and per-IP,
  per-channel and per-address limits, and get the same answer for every address. Identity-token sessions
  subscribe at once when the contact has a known address (verified by the token, or given by a member or an
  e-mail conversation), else through a confirmation, as does a subscription the person left before
  (`PUT /client/v1/subscriptions/{list}`). Double
  opt-in mails a link to `/s/{token}`, a server-rendered page like `/r/{token}` whose button confirms
  (opening it changes nothing). Host backends use `/v1/lists/{id}/subscriptions`; the panel imports CSV
  files (below).
- **Importing.** A CSV upload (`/v1/lists/{id}/imports`, at most 20 MiB and `YUVA_IMPORT_MAX_ROWS` rows,
  default and maximum 100,000, one running import per workspace) needs the importer's attestation and where
  people agreed, both stored on every consent event with the import's id. The file goes to object storage
  and is first validated without writing anything: the encoding (UTF-8 with or without BOM, UTF-16 with
  BOM, else Windows-1254 when the bytes include a letter only Turkish uses there, else Windows-1252), the
  delimiter (`,`, `;` or tab) from the header row, a guessed column mapping and the counts a member
  confirms. It refuses invalid addresses, domains that take no mail (no MX or a null MX), role addresses,
  duplicates and every opted-out, unsubscribed or suppressed address. Importing writes 1,000 rows per
  transaction and snoozes every ~30 s; progress reaches `/v1/realtime` only (`list_import.progress` at
  most once a second, `list_import.updated` on each state change), never as stored events, webhooks or a
  `subscription.*` event per row. Imports send no welcome mail; in re-confirm mode every row becomes
  `pending` and gets a confirmation, spread over the days by the workspace's daily broadcast limit. The
  upload is deleted when the import ends, refused rows stay 7 days for the error CSV, and an import never
  started is cancelled after 7 days.
- **Unsubscribing.** Every broadcast and welcome mail carries `List-Unsubscribe` with
  `List-Unsubscribe-Post: List-Unsubscribe=One-Click` (RFC 8058) and footer links to `/m/{token}`: a page
  with "unsubscribe from this list", the inbox's public lists and "unsubscribe from everything"; opening it
  changes nothing, its one-click POST unsubscribes at once. `/m/` of a status list shows only that list
  (unsubscribe, its component choices, a link back to the page). Tokens seal the workspace, list, contact and
  broadcast under the master key and never expire while the contact exists.
- **Designs.** A design is a JSON document (`EmailDesignDocument` in the contract, `schema: 1` with
  upgrade steps on read) of blocks (section, columns, heading, text, image, button, divider, spacer,
  social, footer) and a theme (colours with dark overrides, a fixed font stack, width, radius,
  direction). Rich text is a stored subset of HTML checked by a bluemonday policy on every write; there is
  no raw-HTML block and no remote image. `api/internal/mail/design` renders it to table HTML with inline
  styles (hybrid columns, dark-mode classes, Outlook wrappers, a preheader) and a text part; it shares its
  colour and button code with the server's own `layout.html`. Merge fields `{{ contact.first_name |
  "there" }}` are checked on save, and a broadcast is not scheduled while a field without fallback would be
  empty for anyone. A required footer (the inbox's legal name, postal address, registration and contact,
  why the person gets the mail, unsubscribe and preference links) is always added. `email_designs` keep
  a version for concurrent edits and up to 50 snapshots.
- **Images** are uploaded (PNG, JPEG, GIF, WebP; never SVG), re-encoded without metadata at most 1,200 px
  wide (WebP stored as PNG or JPEG) and served publicly at `/i/{token}` with immutable caching and nothing
  recorded per request: the one exception to "objects are never public". What's new entries (M22) keep
  their images too: the weekly cleanup skips images an entry or its draft refers to.
- **Broadcasts.** A draft names the inbox, the e-mail channel, lists (up to 10) with attribute and
  language conditions, the design, subject and preheader. Scheduling runs a review (sender identity,
  sender ready, approval, merge fields, HTML under Gmail's clipping size, deliverability) and freezes the
  design. At send time `broadcast_start` snapshots the audience into `broadcast_deliveries` (one per
  contact; suppressed, opted-out and unsubscribed addresses left out) and `broadcast_send`, one job per
  sender channel on its own River queue (`YUVA_BROADCAST_WORKERS`, default 2), sends in batches over one
  SMTP connection at the channel's pace, re-checking each address, so pause and cancel act within a batch.
  The channel's `broadcast_smtp_*` account is used when set, else its support account; confirmation,
  welcome, test and broadcast mail never use the server's own `YUVA_SMTP_*`. Test sends go to members
  only. `YUVA_BROADCAST_DAILY_RECIPIENTS` (0 = no limit) caps recipients per workspace and UTC day; a send
  that reaches it continues the next day.
  Implementation: each recipient's mail is rendered from the frozen document with their merge values
  when it goes out (no stored HTML template). A batch is 50 mails or 5 seconds, whichever ends first, and
  the broadcast's state is read again before every mail; the send job marks a broadcast `sent` itself once
  nothing is queued (there is no separate finish job), and a sweep every 5 minutes queues the job of any
  broadcast that should be moving. Recipients are checked again before their mail (still subscribed, not
  suppressed, not opted out) and skipped with the reason otherwise. Temporary SMTP failures are retried
  after 5 minutes, 30 minutes, 2 hours and 6 hours, then the delivery fails; `5.1.x` and `5.2.1` refusals
  are bounces; 20 refusals in a row, a refused account or TLS, or a server unreachable for 30 minutes
  pause the broadcast (`sender_error`, the SMTP answer in `state_detail`). Test sends: 20 mails an hour
  per workspace; their unsubscribe link names no list and its page says it was a test. Progress reaches
  `/v1/realtime` as `broadcast.progress` (counts, not stored, at most every 5 seconds).
- **Pace.** A broadcast may have its own pace (`pace`: `per_hour`, `per_day`, `per_month`, each optional,
  1 to 10,000,000), set when it is created, on a draft, and alone while it is scheduled, preparing,
  sending or paused (`409 invalid_state` once finished); a change applies to the next mail. Windows are
  rolling (any 60 minutes, 24 hours, 30 days), so no time zone applies, and the sender spaces mails
  evenly at the tightest gap (3,600 / `per_hour`, 86,400 / `per_day`, 2,592,000 / `per_month` seconds),
  which keeps every window within its limit without bursts. The channel's `broadcast_rate` and the
  workspace's daily limit stay upper bounds: the pace only slows sending. Before each mail of a paced
  broadcast the sender claims the slot with one conditional update of `broadcasts.pace_next_at` (taken
  when `<= now`, then set to `now + gap` from the row's current pace), so two pods or a rescued job never
  send two mails in one slot; every SMTP attempt takes a slot, skipped recipients none. A wait longer
  than a batch ends the batch and snoozes the channel's job until the slot, at most 5 minutes, so other
  broadcasts of the channel keep sending and a changed pace applies within 5 minutes. Pausing keeps the
  next slot (no catch-up burst on resume); changing the pace moves it to the last mail plus the new gap.
  `pace_next_at` is shown while sending with a gap of at least a minute; `waiting_until` stays the daily
  limit's. A send may take at most 365 days at its pace: the review's `pace` item blocks above that and
  `PATCH` refuses such a pace on a started broadcast (`409 pace_too_slow`); the review's
  `duration_seconds` estimates the whole send from the pace, the channel's rate and the daily limit.
- **Amazon SES tenants.** On a shared SES account one inbox's bounces and complaints pause only its own
  SES tenant, never another inbox of the workspace or another workspace (M13.7). One tenant per inbox,
  not per workspace, because inboxes are where a workspace keeps its products. With
  `YUVA_SES_REGION`, `YUVA_SES_ACCESS_KEY_ID`, `YUVA_SES_SECRET_ACCESS_KEY` and
  `YUVA_SES_CONFIGURATION_SET` (all or none; an IAM user limited to tenants named
  `YUVA_SES_TENANT_PREFIX`, default `yuva-`, identity associations and that configuration set) the
  server manages the tenants; customers never enter AWS keys. A channel's SMTP account is managed when
  its host is an SES endpoint in that region and its user name is an access key of the server's own
  AWS account (`sts:GetAccessKeyInfo`), since customers with their own SES use the same hosts. A
  `ses_tenant_sync` job per inbox (on saving or deleting such a channel, on `broadcasts approve` for
  every inbox of the workspace, and from `yuva inbox ses-tenant sync [--all]`, the back-fill; never at
  boot) creates the inbox's tenant (`ses_tenants`, keyed on `workspace_id` and `inbox_id`) with a random
  name, tags and tenant-level suppression (bounces and complaints stay inside the tenant), associates
  the configuration set and, per channel, the sending address's identity and the most specific
  verified domain identity, and dissociates identities none of the inbox's channels uses any more
  (never the configuration set, never a tenant Yuva did not create). A domain shared by two inboxes is
  associated with both tenants. Support mail is isolated from the first managed channel, not only after
  approval. Every tenant keeps SES's Standard reputation policy. The configuration set is one, created
  by the operator with its SNS bounce and complaint destination; Yuva never sets an identity's default
  configuration set. A channel whose sending domain is not a verified identity is `identity_missing`:
  its mail goes out without the tenant (a header without the association would fail the send) and the
  review blocks its broadcasts. Mail through the managed account always carries the configuration set;
  the tenant header only once the tenant is ready. A `pending` tenant (after a channel save) holds a
  running broadcast's batch for 15 seconds instead of pausing it. A ready tenant absent from a complete
  listing and confirmed gone by `GetTenant` is `missing`: it blocks broadcasts, loses the header and a
  sync creates it again. Tenant names, the configuration set and AWS details reach owners, admins and
  keys with `inboxes:manage` only. Once isolated, a managed channel's mail always carries the tenant, also
  while it is paused. Without server keys a channel may name a tenant (and configuration set) of its
  own SES account, which Yuva only writes as headers. A periodic job every 5 minutes reads
  `ListReputationEntities` under the prefix (aggregate and both origins); optionally, an EventBridge
  rule on SES's tenant events sent to an SNS topic in `YUVA_SES_TOPIC_ARNS` makes `/ingress/ses`
  refresh that tenant at once (its body is not trusted for the status). A paused tenant pauses that
  inbox's running broadcasts through managed channels (`pause_reason: ses_tenant`), blocks their
  scheduling and resuming (`409 ses_tenant_blocked`), shows on the inbox (`Inbox.ses_tenant`), its
  channel pages and in the operator digest; the inbox's replies are still attempted and fail with SES's
  answer; other inboxes keep sending. Only the operator reinstates (`yuva inbox ses-tenant reinstate`,
  customer-managed status `REINSTATED`); a pause by SES itself needs an AWS Support case. Deleting an
  inbox or a workspace deletes its tenants. An SES notification tagged `ses:tenant-name` of a managed
  tenant counts only for that tenant's workspace and inbox. The status poll (`ListSESTenantsAll`), the event lookup
  (`FindSESTenantByARN`) and the operator back-fill (`ListInboxesWithSESChannels`) read across
  workspaces (SES names only the tenant), and every write that follows is scoped
  to the row's `workspace_id`: the written exception to the workspace filter. The AWS calls are a small
  client on minio-go's SigV4 signer (already a dependency), using only operations without path
  parameters except the reinstate call.

  ```sh
  yuva inbox ses-tenant show --workspace <id|name> [--inbox <id|slug>]
  yuva inbox ses-tenant sync (--workspace <id|name> [--inbox <id|slug>] | --all) [--dry-run]
  yuva inbox ses-tenant reinstate --workspace <id|name> --inbox <id|slug>
  ```
- **Headers.** `Message-ID` `<delivery token.random@sending domain>` (the conversation shape, so loop
  protection is unchanged), `List-Unsubscribe`, `List-Unsubscribe-Post`, `List-Id`, `Feedback-ID`
  (`broadcast:list:workspace:yuvabroadcast`), `X-Auto-Response-Suppress: OOF, AutoReply`; no
  `Auto-Submitted`.
  Mail through an SES account also carries `X-SES-TENANT` and, when set, `X-SES-CONFIGURATION-SET`
  (Amazon SES tenants).
- **Bounces and complaints.** SES (SNS), delivery reports to the channel, SMTP 5xx at send time and ARF
  feedback reports find the delivery by Message-ID. A hard bounce suppresses the address for all mail; a
  complaint about a broadcast suppresses broadcasts only, unsubscribes every list of the workspace and
  writes an opt-out. A broadcast pauses itself at 5% bounces (after 200) or 0.3% complaints (after 1,000),
  counted since it started or was last resumed; a workspace whose 30-day complaint rate reaches 0.3% (over
  at least 1,000 mails) or bounce rate 8% (over at least 2,000) is suspended from sending and its running
  broadcasts pause. ARF reports count only from a sender whose trusted DMARC result is `pass`.
- **Deliverability.** A check (six an hour per channel) sends a probe through the broadcast account to
  the channel's own address; ingress knows it by its token (in the Message-ID or `X-Yuva-Probe`), never
  makes it a conversation, and reads only the trusted `Authentication-Results` (SPF, each DKIM signature
  with its domain and alignment, DMARC, whether the aligned signature's `h=` covers `List-Unsubscribe` and
  `List-Unsubscribe-Post`) plus DNS (SPF, DMARC policy, MX of the reply domain, the Yuva DKIM record).
  Each channel keeps its latest result; a probe not back after 10 minutes reads as `timeout`, and the
  panel polls. The review warns when the channel was never checked, the check did not finish or cannot be
  verified, DMARC is missing, DKIM is not aligned with `From` or the list headers are unsigned, and blocks
  when no DKIM signature passed. A channel may have its own DKIM key (RSA 2048, private key sealed under
  the master key, only the DNS record shown; enabling needs that record in DNS, a new sending domain turns
  it off, deleting allows a new key). Yuva then signs broadcasts, tests, probes, confirmation and welcome
  mail over the list headers; support replies keep their provider's signature. The signature covers
  `From`, `To`, `Subject`, `Reply-To`, `MIME-Version`, `Content-Type` and the list headers (absent ones
  listed too) but not `Message-ID`, `Feedback-ID` or `Date`, which relays replace, and address headers
  are written in the minimal form normalizing relays leave alone (a display name quoted only when RFC
  5322 needs it, a bare address without angle brackets). Amazon SES channels never get Yuva DKIM
  (enabling it is refused, an older enabled key is not used): SES rewrites `From`, `To`, `Reply-To` and
  `Date` after any signature added before it and signs every mail with the customer's domain itself
  (Easy DKIM), list headers included.
- **Replies.** A reply to a broadcast finds its delivery by the token in `In-Reply-To`/`References` (or
  by the relay's Message-ID stored for the delivery) and
  opens (or continues) a conversation with that contact in the inbox with `broadcast_id` and a
  `broadcast_reply` marker; someone else's reply opens their own conversation; automatic replies are only
  counted (when they name no conversation). Notifications about replies to one broadcast are grouped per
  member every 10 minutes (one push "N new replies"; no e-mail fallback for them).
- **Measuring.** Per broadcast: recipients, sent, failed, bounced, complained, unsubscribed, replied,
  automatic replies, skipped by reason, and unique clicks only when the broadcast turns click tracking on
  (links go through `/c/{token}`). There is no open pixel. Tracked links are the web links of the HTML part
  that are the same for every recipient (no merge field), found when the broadcast is scheduled; a `HEAD`
  request and a third link of one mail opened within 2 seconds are not counted.
- **Hosted servers.** `YUVA_BROADCASTS=on|approval|off` (unset: `approval` with `YUVA_SIGNUP=open`, else
  `on`). In `approval` mode a workspace asks the operator, who answers with
  `yuva workspace broadcasts approve|deny|suspend|limit|show`; approved workspaces warm up from 500
  recipients a day, doubling after each clean day (at least 100 sent, bounces under 3%, complaints under
  0.1%) and losing a doubling after a bad one. Only the operator lifts a suspension (`approve`). The
  operator digest reports approval requests, suspensions and auto-pauses without content.
  `YUVA_BROADCASTS=off` refuses broadcast writes (`403 broadcasts_disabled`); lists and their forms keep
  working. `YUVA_MAIL_LINK_URL` puts `/m/`, `/s/`, `/c/` and `/i/` on a separate domain, which answers only
  those routes.

  ```sh
  yuva workspace broadcasts approve|deny|suspend --workspace <id|name> [--reason <text>]
  yuva workspace broadcasts limit --workspace <id|name> --daily <n>   # 0 removes the limit
  yuva workspace broadcasts show --workspace <id|name>                # status, limits, 30-day rates, last 10
  ```
- **Scopes, events, MCP.** Scopes `lists:read`, `lists:write`, `broadcasts:read`, `broadcasts:write`,
  `broadcasts:send`; keys and OAuth tokens schedule only while the workspace allows it
  (`broadcasts_by_keys`, off by default). Events and webhook types `subscription.created`,
  `subscription.updated` and `broadcast.updated` (imports send none per row). MCP tools draft designs and
  broadcasts, preview, send tests to the caller and read reports; `schedule_broadcast` needs the audience
  count it was shown.
- **Law.** Consent records and their export serve GDPR Art. 7 and 15 and KVKK; objections are kept after
  erasure; the welcome receipt and the footer follow Türkiye's Regulation on Commercial Communication
  (Art. 7–10); links never expire (CAN-SPAM 30 days, CASL 60). Senders in Türkiye register their consents
  in İYS themselves; a list's consent export with `format=iys` gives İYS's consent fields (`recipient`,
  `type` `EPOSTA`, `source` `HS_WEB` for forms and the preference page and `HS_EORTAM` otherwise,
  `status` `ONAY` or `RET`, `consentDate` in Türkiye's time, `recipientType` `BIREYSEL`), leaving
  pending subscriptions out.
- **Template gallery.** Six starters ship in `templates/email/` (MIT, listed in `LICENSING.md`; each a
  design JSON per language, assets and generated previews): newsletter, product update, announcement,
  welcome, event, digest. The panel shows them when creating a design; the website shows them at
  `/templates` (en, tr, de) with "Use this template". New templates come by pull request
  (`templates/email/README.md`); there is no paid store.

### Contacts directory

The panel lists contacts newest first or most recently active first (`sort=last_seen`), filtered
by `kind` (`known`: an e-mail address or an external id; `visitor`: neither) and by whether they
have an open conversation, with search over names, addresses and external ids.

- `contacts.last_active_at` is the contact's last message (any channel), rating or use of a widget
  or app session. It is kept by the queries that write those (message insert, session start,
  session use at most once per throttle period, realtime close), carried over on a merge, and
  indexed with `created_at` as fallback for the sort. It is the same for every member; an agent may
  see a contact rise in the order because of a conversation in an inbox they cannot see, but never
  that conversation. Pages follow it by keyset, as conversations follow `last_activity_at`; it
  only grows, so a contact active while someone pages moves above the pages already read and is
  neither repeated nor on the later pages, which the API reference says.
- `activity` (conversation counts, open ones, the last message time) is added only to
  `GET /v1/contacts` and `GET /v1/contacts/{id}`, by one grouped query per page over the
  conversations the caller can see, spam left out; events and webhooks carry the contact without it.
- `GET /v1/contacts/{id}/summary` counts first replies with their median time and the latest
  rating of each conversation, over the same conversations, computed on request.
- A contact's conversations are `GET /v1/conversations?contact_id=` with the usual filters.
- Contact notes (`/v1/contacts/{id}/notes`) are team-only notes about the person, not tied to a
  conversation: a `contact_notes` row with the author (a member or an API key) and the text.
  Whoever can see the contact reads and adds them, under the same rule as the contact itself; only
  the author or an owner or admin deletes one. Contacts never see them, no event or webhook
  carries them, a merge moves them to the remaining contact and deleting the contact deletes them.

### Moving to Yuva (M14)

A team brings its history from the service it uses today and then switches its mail over, without
losing a message and without changing anything at the source. Decisions:

- **Sources.** Every mailbox is read over IMAP. Presets for Gmail and Google Workspace, Yandex,
  iCloud, Zoho, Yahoo, Fastmail, GMX and any server connect with an app password or password.
  Outlook.com and Microsoft 365 accept no passwords: "Sign in with Microsoft" obtains only an IMAP
  token (`https://outlook.office.com/IMAP.AccessAsUser.All` and `offline_access`, sent with
  `AUTHENTICATE XOAUTH2`) through the operator's Entra app (`YUVA_MICROSOFT_*`) or, without one, an
  app registration the team created and entered on the connect page, as for help desks; the
  authority is `consumers` for Outlook.com addresses, `organizations` for domains whose MX is
  Exchange Online, else `common`. The app is not publisher-verified, so Outlook.com and tenants that
  allow user consent connect, and other tenants use export files or an admin's consent (the panel
  shows the admin-consent link). Export
  files (Google Takeout mbox, mbox, .eml or Maildir in a zip; no PST, for want of a permissively
  licensed reader) need no access to the account and are the fast way for very large Gmail
  mailboxes: Takeout first, then IMAP for newer mail and the cutover. Help desks: Zendesk with OAuth
  `read`, Help Scout, Chatwoot (an administrator's access token, the install's HTTPS base URL and the account
  number; conversations by last activity, messages backwards with `before`, 60 requests a minute); Intercom (a private app's access token, see below); Crisp (a website token, identifier and key as Basic auth plus `X-Crisp-Tier: website`, and
  the website id; conversations ordered by last update a page at a time, messages read backwards with
  `timestamp_before`, files fetched from Crisp's storage host without the token, 5,000 requests a day of the
  token's 10,000, and a 429 or 420 waits a minute and halves the pace); each team creates the OAuth client in its own help desk with Yuva's redirect URL
  and enters its id and secret on the panel's connect page (there is no operator-wide client), and a
  help-desk migration goes through the same cutover as a mailbox. Front connects with an API token a
  Front admin creates (feature Access resources, namespaces Global and Shared, permission Read only),
  sent as Bearer to `api2.frontapp.com` only; Front does not let a caller read a token's permissions
  (`GET /me` answers the company alone), so read-only rests on the GET-only transport and the guide.
  Front's copy lists each inbox's conversations (`GET /inboxes/{id}/conversations`, page token kept
  in the cursor, so a killed copy resumes where it stopped) and its sync searches each inbox
  (`GET /conversations/search/inbox:… after:…`, a `GET`, capped at 40% of `x-ratelimit-limit` next to
  the half of it the whole budget uses; `retry-after` is honoured on 429); attachments are fetched
  by id from `/download/{id}` on the API host, never from the company subdomain URL a message names. Every help desk is one entry of a
  registry (`internal/migrate/helpdesk`): its API hosts, attachment hosts, auth kind (`oauth` or
  `token`) and how the token is sent (Bearer, Basic or a named header), its limits with their windows
  (a minute, ten seconds or a day), limit and retry headers and default budgets, its contacts'
  attribute prefix and how its changes are read after the scan; the preset, the sign-in, the reader
  and the copy read the entry, so a new help desk is a new entry. A `token` desk connects with
  `PUT /v1/migrations/{id}/credentials` (`kind: token`, the API token and, where the entry asks, a
  second secret, the account's subdomain, the HTTPS base URL of a self-hosted install, an account number, the data region or a website id), sealed like
  OAuth tokens and deleted with them; the panel shows its form in place of "Sign in". Freshdesk is
  a token desk: an admin's API key (no scopes, no read-only key) sent as Basic `key:X` to
  `<subdomain>.freshdesk.com` only; tickets listed by `updated_at` from `updated_since`, started again at the
  last change time after Freshdesk's 300-page cut and deduplicated by id; attachment URLs are pre-signed,
  fetched without the key and read again once when expired; a budget of half of `X-RateLimit-Total` (50 a
  minute until reported) and 20 list requests a minute. Intercom (`internal/migrate/helpdesk/intercom.go`) connects with the access token of a private app the team creates in its own workspace and a region (`us`, `eu`, `au`: the API host); it sends a pinned `Intercom-Version` and counts 10-second windows (`X-RateLimit-Limit`, half of it; a 429 waits until `X-RateLimit-Reset`, a Unix time). Its search endpoints are `POST`, so conversations are listed with `GET /conversations` (`starting_after`, 150 a page; Intercom documents no order, so every pass reads the listing to its end and keeps what changed since the cursor) and read with `GET /conversations/{id}`, which returns at most the 500 most recent parts (500 parts count as cut whatever `total_count` says): a longer conversation imports those and records a `truncated` error. Teams are the folders, with an "Unassigned" folder (id 0) for the rest; an incoming message's author is its actual sender (a second contact), the requester only when the sender is missing; attachments are the `upload` files Intercom lists, fetched without the token, and a link typed into a message is never fetched. Detection from the address and the domain's MX picks the preset; autoconfig is
  read only from the team's own domain. Provider mail APIs and other sign-ins were considered and
  rejected for one IMAP path.
- **Read-only, enforced three times.** No credential a mailbox accepts is read-only (Microsoft's
  consent screen says "Read and write access to mailboxes via IMAP"), so Yuva enforces it. IMAP goes
  through `internal/migrate/imapread`, the only package that may import the IMAP client: it opens
  mailboxes with `EXAMINE` and fetches with `BODY.PEEK`, and a wire guard under the connection lets
  through only `CAPABILITY`, `NOOP`, `LOGOUT`, `STARTTLS`, `LOGIN`, `AUTHENTICATE`, `LIST`, `STATUS`,
  `EXAMINE`, `UNSELECT`, `SEARCH` and `FETCH` of non-marking items; anything else closes the
  connection before it is sent. Help-desk sources use a transport that refuses every method but `GET`
  (the token exchange and refresh `POST` to the provider's token endpoint aside) and every host not on
  the source's list; attachments the provider keeps on other public hosts are fetched by a separate
  client without the token. A `token` desk's token goes only to its API hosts, never to an attachment
  host, and a team-entered base URL is dialled with the same address checks as an IMAP server
  (private addresses only with `YUVA_MIGRATION_ALLOW_PRIVATE`). A sign-in must grant exactly the
  requested scopes. Tests
  fail the build if a write command or method could be sent, and an integration test proves a Dovecot
  mailbox byte-identical after a migration.
- **Two phases.** A scan reads headers only (the whole message only when it has no Message-ID, to take
  the same hash the live ingest stores), joins threads (Takeout and help-desk thread ids, else
  `References`/`In-Reply-To` with the live bounds, never by subject) and gives an exact dry run:
  counts per folder, what is skipped and why, contacts, labels, storage and time. The copy then
  imports whole threads newest first. A thread is a conversation of the inbox's e-mail channel whose
  address is the mailbox address; mail from a team address (the mailbox, aliases, members) is an
  outgoing message by the mapped member or, without one, by its original name and address
  (`imported_author`). Spam, trash, drafts and chats are never read; threads of only automatic or
  only internal mail are skipped by default. Labels and user folders become labels; a status rule
  closes old threads and leaves recent ones open or pending.
- **Imported history is quiet.** Imported rows keep their original times and carry `imported_at`
  (and `migration_id` on conversations and created contacts). They write no `conversation.*` or
  `message.*` events, so no webhook, push, notification, auto-reply, rating request or continuity
  mail follows; they are never unread, are left out of `/v1/stats` and the bounce lookups, count in
  `imported_*` usage columns instead of the billed ones, and the retention clock starts at the import.
  Parsing, quote stripping, sanitising and attachments are the live ingest code's. "Remove imported
  history" deletes the conversations nobody wrote in after the import.
- **Engine.** River jobs on a queue of their own (`migrations`, `YUVA_MIGRATION_WORKERS` per process,
  default 2) in 30-second slices, one job per migration (unique options and a lease), a watchdog every
  minute that resumes a migration whose process died. Checkpoints per folder (`UIDVALIDITY` and UIDs,
  file offsets, help-desk cursors); every item has a source id and a dedupe key (Message-ID, else the
  hash the live ingest uses), its rows get ids derived from both, and it is imported in one
  transaction, so a retry never duplicates. A message the channel already has, in either direction,
  is not imported again, which also lets a Takeout import and a later IMAP migration of the same
  mailbox meet without duplicates; a thread whose earlier messages the channel already has continues
  in that conversation, in every migration. Each source has a budget below its published limit (Gmail IMAP
  1,500 MB a day, other servers one connection and 4 MB/s, help desks half their rate limit in its
  window, and half their daily limit next to it where they have one), kept on the migration row;
  limit answers (`429`, and `420` where a help desk sends it) and `Retry-After` are honoured. Microsoft tokens are refreshed
  before a connection and resealed. Pause stops after the batch in progress (at most 50 messages);
  any change to a ready migration's options sends it back for a new dry run. Memory does not grow
  with the mailbox: the scan reads headers a page at a time, the plan joins threads in the database
  (`migration_plan_nodes`, union by size, a page of messages per step) and plans a batch of whole
  threads at a time, and the copy holds one batch and keeps a folder open across batches instead of
  opening it again for each. Progress reaches owners
  and admins as `migration.updated` (stored) and `migration.progress` (a signal, at most every two
  seconds); never webhooks.
- **Cutover.** After the copy, sync runs every two minutes. The migration goes live when the copy ends:
  mail received before that is history (mail that arrives during the copy is imported quietly); mail
  after it is new mail and takes the live path (events, notifications), without the greeting. Every
  e-mail channel has a forwarding address `<token>@YUVA_FORWARDING_DOMAIN` that ingress resolves by
  token (rotatable); the forwarding domain may also carry ordinary channel addresses, so a local part
  that is no channel's token falls through to the exact, `local+tag` and catch-all lookup. The team forwards the mailbox there (or routes it, or moves the MX). While
  waiting, Google's forwarding confirmation to that address is shown on the migration (and sent as the
  `migration.forwarding_confirmation` signal) instead of opening a conversation. A probe mail with an
  HMAC header (context `migration-probe:`) is recognised at ingress before the own-sender drop and
  verifies delivery. After verification and a quiet period (24 hours by default) in which sync found
  nothing ingress did not also deliver, the migration completes on its own; an owner may finish it
  earlier once delivery is verified, or without verification on purpose (`force`). The channel dedupe
  keeps one copy of mail that came both ways. "Remove imported history" also deletes the contacts the
  migration created that are left without a conversation; labels stay.
- **Credentials.** Sealed under the master key (context `migration:`+workspace+migration) in
  `migration_credentials`, never in job arguments, events, logs or answers; deleted in the transaction
  that completes or cancels the migration, after `YUVA_MIGRATION_SYNC_MAX_DAYS` (30) of sync, and with
  the workspace. Tokens are not revoked at the provider (that would take a method other than `GET`);
  the completion screen and the guides say where to remove the OAuth client, the app or the app
  password. Sign-in uses PKCE, refuses a grant whose scopes
  differ from the requested set and, for Microsoft, a sign-in whose account cannot open the mailbox:
  when the token's claims name another address or none (a Microsoft 365 UPN is often not the SMTP
  address), the mailbox decides by accepting the token in `AUTHENTICATE XOAUTH2` with its address as
  the user, and only its refusal is `account_mismatch`; the audit row names the account that signed in.
  Assistants never handle a password: MCP tools return a panel link where the person connects.
- **Who.** Owners, admins, and keys without an inbox limit or owner/admin tokens with
  `migrations:manage`. Every action is in `migration_audit` (actor, client), kept
  `YUVA_MIGRATION_AUDIT_DAYS` (365). Connection tests are limited (10 per migration, 30 per workspace an
  hour) so the server is no password-guessing tool. Hosted limits: `YUVA_MIGRATION_SOURCES`,
  `YUVA_MIGRATION_MAX_ACTIVE`, `YUVA_MIGRATION_MAX_BYTES`, `YUVA_MIGRATION_UPLOAD_MAX_BYTES`;
  `422 hosted_limit` names the one reached in the problem's `limit`.
- **Panel and MCP.** A wizard in the setup layout (Source, Connect, Choose, Map, Review, Copy, Switch,
  Done) with live progress, provider guides inline and in the docs (en, tr, de), Settings → Move to
  Yuva for the list, a banner in the target inbox while it runs. The banner reads the inbox's
  `import` (address, state, percent), which everyone who sees the inbox gets; `inbox.updated` comes
  when it appears, changes state or ends, and the percent between those is read by refetching the
  inbox. MCP tools for every operation
  (`create_migration` returns the connect link) and the `move_to_yuva` prompt. Like the wizard's Map
  step, `create_migration` given an inbox and no channel uses the inbox's e-mail channel for the
  mailbox address or creates it (with `inboxes:manage`) without an SMTP account, and its answer
  tells the person to set SMTP in the panel; no tool takes that password.

### Analytics (M15)

A workspace measures its own websites and apps: pageviews, screens, events, goals, funnels, revenue,
sources, locations and devices, in the panel, the API and MCP. Plan in the M15 umbrella issue; it ships
in six steps (M15.1–M15.6 in the roadmap). Decisions:

- **Properties.** `analytics_properties` belong to a workspace and optionally to an inbox (who sees them,
  what support data they join): name, a public key (`yuva_ak_…`, unique on the server, rotatable with
  24 hours' overlap), 1–50 domains (`*.example.com` allowed), platforms (`web`, `ios`, `android`), time
  zone (the property's day), currency, `collection` (`anonymous` or `consent`), `consent_features`
  (`persistent`, `identify`; none by default), `audience` (`general` or `children`; children's
  properties keep the country only, general ones keep the city), exclusions, path rules, up to 10
  tracked props, `retention_months` (25). Owners and admins manage them; agents read the properties of
  their inboxes.
- **Default mode: nothing on the device, no IP stored.** The address is shortened (IPv4 /24, IPv6 /48)
  in the first function that sees it; GeoIP and the hash use only that. A visitor is
  `HMAC-SHA256(salt, property ‖ short IP ‖ user agent)` truncated to 64 bits; apps hash a random key held
  in memory for the life of the process instead of the address. Salts are random, one per property and
  local day in `analytics_salts`, deleted 2 hours after the day ends (`YUVA_ANALYTICS_SALT_GRACE`) and
  never derived from a persistent secret. So visitors are counted per day; a visitor over a range is the
  sum of daily visitors. A visit is a visitor's events without a 30-minute gap within the day.
  Processes agree on a new visit through `analytics_visit_claims`: the first claim wins for 5 minutes
  (`ClaimWindow`, at most the visit gap); claims commit without waiting for the disk sync (a crash at
  worst splits a visit) and are deleted with the expired salts. A visit's description (source, place,
  device) is the one its first event carries, even when another process wrote the visit first.
- **Law.** Built to meet the CNIL audience-measurement exemption (purpose, no cross-checking, one
  publisher, shortened IP, no tracker), PECR Schedule A1 (statistics with information and an
  objection), the KVKK cookie guide's Criterion B for first-party analytics, CCPA (no sale or sharing),
  LGPD and COPPA (children's properties keep country only, no props, no consent features). The EDPB
  (Guidelines 2/2023) reads ePrivacy Art. 5(3) as covering any script that makes a browser send data, so
  the docs say where a banner is still needed and `collection: consent` sends nothing until
  `yuva.consent({analytics: true})`. DNT and GPC stop collection unconditionally; `yuva.optOut()` stores
  the objection, the only default-mode write to the device. Raw rows are treated as pseudonymous
  personal data until the raw window ends; rollups are anonymous.
- **Consent features.** Only for visitors who consented, through the host's own consent tool:
  `persistent` keeps a random id on the device for at most 13 months (returning visitors, retention,
  cross-day funnels), hashed with a per-property key; `identify` sends the inbox's identity JWT, checked
  like `/client/v1/session`, and links the visitor's visits to the contact, shown on the contact page.
  Without consent no visitor is ever joined to a contact or conversation.
- **Collection.** `POST /client/v1/analytics/{key}` (one event, or up to 100 for apps), a plain handler
  outside the strict server; `text/plain` JSON so browsers send no preflight; its CORS branch matches the
  `Origin` host against the property's domains; no cookie, no session, request logs without addresses.
  It takes the visitor's address from `X-Forwarded-For` only when the connecting address is one of
  `YUVA_TRUSTED_PROXIES` (empty trusts nobody): the rightmost entry that is not a trusted proxy, if it
  is a public address, otherwise the connecting address. A direct client's header is ignored, so rate
  limits, GeoIP and the excluded-IP check cannot be steered by forged entries.
  Host backends send `POST /v1/analytics/properties/{id}/events` with `analytics:write`, forwarding the
  visitor's IP and user agent (never stored) or their own visitor key. Ingest scrubs e-mail-shaped
  values, keeps only UTM keys of the query string, parses the user agent (`medama-io/go-useragent`), looks
  up GeoIP (DB-IP Lite downloaded monthly by default under CC BY 4.0 with attribution, any MMDB file, or
  off; GeoLite2 only from the operator's own account), drops bots (crawler list, `webdriver`, header and
  volume rules, referrer spam), and answers `202` before writing.
- **Storage.** Events wait in a per-process buffer (`YUVA_ANALYTICS_BUFFER`, full → `503`) and are flushed
  every second by `COPY` into `analytics_events` with one upsert per visit into `analytics_visits`; both
  are partitioned by the property's local `day`, created ahead and dropped after
  `YUVA_ANALYTICS_RAW_DAYS` (90) by the `analytics_partitions` job. Finished days are folded into rollups
  (`analytics_daily` per dimension and value, top 1,000 values plus `(other)`; hourly, goals, props,
  support) by `analytics_rollup` jobs on the `analytics` River queue; late events and new goals mark days
  dirty. Queries add rollups for built days and raw rows for open ones; filter combinations the rollups
  cannot answer are served from raw rows inside the raw window only. The flusher also keeps each
  visit's numbers per path (`analytics_visit_pages`), which page breakdowns and page filters read instead
  of the events. Every five minutes an open day is folded up to two minutes ago (`analytics_partial_*`)
  for the visitors whose every visit begun by then had ended half an hour earlier, so a query adds only
  the other visitors' raw rows and takes away what the fold already holds of theirs, and a change to a
  folded visit (a delayed write, an engagement joining an old visit, a bot found later, a deletion)
  moves the day's sequence so the fold stops counting until the next one. No Postgres extension. The query
  layer sits behind a `Store` interface so a column store can be added later without a contract change;
  Postgres is the only store in M15 and fits up to about 100M events a month per property.
- **Events, goals, funnels (M15.2).** Custom and built-in events (`outbound_click` and `file_download`
  with an http(s) `url`, `not_found`, `chat_open`) carry up to 30 props (keys 1–300 characters, none
  starting with `$` or `yuva_`; text up to 2,000 characters; e-mail-shaped values dropped and counted as
  scrubbed; none on children's properties but a built-in event's `url`) and revenue in the currency's
  minor units, never converted. Goals are a page view, an event with an optional prop equal to a value,
  or a scroll depth on matching pages; there is no separate revenue goal, because an event goal reports
  the revenue its events carry, per currency. A change to a goal or to `tracked_props` moves the
  property's `goals_version`; a rolled-up day records the version its goal and prop rows were built with,
  so a goal is read from rollups only on days built at or after its own version and from raw rows
  elsewhere in the raw window, and the rollup scan rebuilds those days' goal and prop rows without
  touching traffic rollups. Prop breakdowns read raw rows for any key inside the raw window and
  `tracked_props` rollups before it. Funnels follow visits within a day over the raw window (90 days, or
  31 when the property counted more than 10M events in the last 30), answers cached 10 minutes.
  Segments and annotations need `analytics:read` and are edited by their author, owners, admins and keys
  without an inbox limit; Yuva's own annotations (`key_rotated`, `goal_created`) cannot be edited.
- **Apps and servers (M15.3).** An app screen is a page view of `app://<app id>/<screen>`; the app id is
  the visit's hostname and is not checked against `domains`. App requests have no `Origin`, use the batch
  form `{k, a, e}` with one process key per request (a fresh UUID per launch, kept in memory only) and
  need `a.platform` in the property's `platforms`; they skip the browser-header bot rules and the
  user-agent parser, the volume rules still apply. A late event (at most 72 hours old, `t` from apps,
  `timestamp` from servers) is stored on its own day at its own time, hashed under its day's salt while
  that salt exists and under the arrival day's otherwise; visits, claims and bot counts use the event's
  time. `POST /v1/analytics/properties/{id}/events` (keys with `analytics:write`) takes server events that
  forward `ip`, `user_agent` and the client hints, hashed exactly like the browser's request so they
  join its visit, or a `visitor_key` hashed in a scope of its own; a visit a server event starts has
  platform `server`. `dnt`, `gpc` or a missing `consent` mean the event is not stored; invalid events
  come back in `errors`, and only a body or auth problem refuses the request. `sdk/go/analytics` drops
  DNT and GPC requests before sending and retries 429, 5xx and network errors with backoff that honours
  `Retry-After`.
- **Sharing and export (M15.5).** `analytics_shares` keep a SHA-256 of the link token (`yuva_sh_…`, shown
  once) and an argon2id password hash (m=19 MiB, t=2, p=1); unlocking gives a 12-hour `yuva_sa_` token
  HMAC-signed over the link id, its expiry and the password hash, so a new password ends every token;
  revoke and expiry stop a link at once. 50 links per property; 120 requests a minute per address, 10
  unlocks a minute per address and link, 30 in all. A link lists its views (`overview`, `realtime`,
  `pages`, `sources`, `locations`, `devices`, `events`, `goals`): totals and time grains come with every
  view, other dimensions, filters and event or goal metrics only with theirs, saved segments never; its
  queries share the workspace's query slots and cache. The server serves the panel at `/share/{token}`
  with `frame-ancestors` set to the link's embed origins, otherwise `'none'` and `X-Frame-Options: DENY`,
  without the panel's analytics script and with the token left out of request logs; the share API is
  `/client/v1/analytics-shares/{token}`. Exports are `view` (one query), `daily` (a row per day, optionally
  by one dimension) and `raw` (NDJSON, at most 31 days inside the raw window, bots left out, visits
  renumbered per file, no visitor hash, salt, stored id, contact or referrer path), written by
  `analytics_export` jobs to storage (1 GiB cap, 3 at a time per workspace) and deleted 7 days after they
  finish. `view` and `daily` need `analytics:read`; `raw` needs `analytics:manage` and an owner, admin or
  key without an inbox limit, because raw rows are pseudonymous. CSV cells starting with `=`, `+`, `-`,
  `@`, tab or CR get a leading `'`.
- **Support, alerts and reports (M15.4).** `analytics_daily_support` keeps conversations, page feedback,
  ratings and discussion threads of the linked inbox by (host, path, day), paths after the property's
  path rules, built in the same job as the day's traffic rollup; a day is read from it only while
  `analytics_support_days` names the property's current inbox, other days from the inbox's own tables.
  Conversations and discussions count on their local day, page ratings on the UTC day they were given.
  `support_rate` is conversations per 1,000 page views, joined to traffic by path (hostname as its own
  dimension or filter), never through a visitor; `<yuva-chat>` sends the page URL without query or
  fragment and the title with a new conversation, kept only for a chat channel that allows the page's
  origin and dropped silently otherwise, so the page never refuses a conversation. Alerts: spike and drop compare the last two finished
  hours with the same hours averaged over the 7 days before (3× or more, half or less, a baseline of at
  least 20 views); goal drop is yesterday at half or less of its 7-day average with at least 10 a day;
  support load is yesterday's rate at twice the 28-day rate with at least 5 conversations; a new referrer
  has 10 or more visits yesterday and none in the 28 days before. An alert is unique by (property, kind,
  key) and fires once a day (a new referrer once per host), writing an annotation, the `analytics.alert`
  stored event and webhook, and a mail to subscribers. Weekly (Monday to Sunday) and monthly reports go
  out at 08:00 property time once per member and period, in the member's language, without images or
  link tracking; subscriptions are per member session. Broadcast links get `utm_source=newsletter`,
  `utm_medium=email` and the campaign (on by default; links that carry a `utm_` parameter and the
  server's own links are left alone; broadcasts already past draft at upgrade keep their links), and the
  report shows the campaign's visits to callers with `analytics:read`. `ListAnalyticsReportWorkspaceIDs`
  reads `analytics_subscriptions` across workspaces to find which have work, the same maintenance
  exception as `ListAnalyticsWorkspaceIDs`; every query after it filters by workspace. MCP adds the
  analytics tools (read: properties, overview, query, realtime, support report, goals and funnels,
  funnel report, snippet, notes; manage: create goal, create property), the `analytics_review` and
  `setup_analytics` prompts and the `yuva://property/{id}` resource; visitor-sent values are
  `customer_content`. Insights compare in percent only from a baseline of at least 10; a total with none
  before is `metric_new`, a row `new_value`; each insight appears once per answer.
- **Consent features (M15.6).** A consented `persistent` id (kept on the device for 13 months, made only
  without DNT or GPC) is hashed with the property's sealed random `visitor_key` and is that visitor on
  every day (`persistent`, `visitor_type`); ids first seen more than 13 months ago are forgotten by the
  hourly `analytics_cohorts` job, which also rebuilds `analytics_cohorts` (days to 30 periods, weeks and
  months to 12). `identify` needs `persistent` and a linked inbox; its token is checked exactly as for
  `/client/v1/session` (single-use `jti`) and links the id and its raw visits to the contact; forged,
  expired or reused tokens are ignored. `yuva_consent` events carry identification and withdrawal and
  are never stored, counted or billed; a withdrawal, honoured under GPC and DNT, deletes the id's link
  and cohort days, and `DELETE …/visitors/{persistentId}` erases everything stored under the id. A
  consented visitor counts once over a range's totals and per week or month bucket (from cohort days
  without filters, from raw rows inside the raw window with filters); default-mode traffic stays a sum
  of days. Funnels follow consented visitors across visits and days in `consented_visitors`. Deleting a
  contact deletes its linked visits, merging moves them, and contacts left from an import that have
  linked visits are kept. Children's properties refuse consent features.
  The script tag keeps `/yuva-analytics.js` at 1.5 KB without them; script-tag sites that use consent
  features load `/yuva-analytics-consent.js` (≤ 2 KB) instead, served and routed like the first.
  Every client treats consent to analytics without `persistent: true` as a withdrawal of a stored id, so
  hosts pass `persistent: true` on each call where the visitor agreed to it; identify and withdrawal
  are sent once and never queued.
- **SDKs.** `/yuva-analytics.js` (≤ 1.5 KB gzip, served like `/yuva.js`, off with `YUVA_WIDGET=off`) with
  automatic and SPA pageviews, engagement, opt-in outbound, file and chat-open events; `useyuva/analytics`,
  React and Nuxt helpers; Swift product `YuvaAnalytics` (no `YuvaKit` dependency, SwiftUI `.yuvaScreen`,
  privacy manifest with `CA92.1`, no IDFA) and Kotlin `yuva-analytics` (Compose `TrackScreen`, no
  advertising id); `sdk/go/analytics` for server events. Apps queue offline up to 72 hours.
- **Support, in aggregate.** A property linked to an inbox gets conversations, page feedback, ratings and
  discussion threads per page and day next to its views (`<yuva-chat>` records the page a conversation
  starts on); broadcasts add UTM parameters by default (a broadcast can turn them off) and show the
  visits of their campaign. Alerts (spike, drop,
  goal drop, support load, new referrer), insights and weekly or monthly e-mail reports are fixed rules;
  Yuva runs no model.
- **Panel, API, MCP.** An Analytics rail section per property (overview, realtime, pages, sources,
  locations, devices, events, goals, funnels, retention for consented traffic, support, settings),
  filters, comparisons, segments, annotations, read-only share links (password, expiry, embeddable on
  listed origins), CSV and NDJSON exports. One query operation (`POST
  /v1/analytics/properties/{id}/query`) serves the panel, the API and the MCP tools. Scopes
  `analytics:read`, `analytics:write`, `analytics:manage`; like `lists:*`, they go to keys created
  after M15 without named scopes, never to existing keys. Analytics writes no stored event per page view;
  `analytics.alert` is a stored event and webhook type.
- **Hosting.** `usage_counters.analytics_events`; `YUVA_ANALYTICS_MONTHLY_EVENTS` (per workspace;
  unset: 1,000,000, `0` is no limit and warns at startup; operator overrides with `yuva workspace
  analytics-limit`), per-property and per-address ingest limits, 4 concurrent queries per workspace;
  `YUVA_ANALYTICS=off` removes the feature. The script and endpoint keep their names
  (`/yuva-analytics.js`, `/client/v1/analytics/`); the docs teach serving both from the site's own domain
  for visitors whose blockers stop them.
- **Counting the panel.** `YUVA_PANEL_ANALYTICS_KEY` (unset by default) names a web property on the same
  server; the server puts the script, in manual mode, ahead of the panel's own scripts in the panel's
  page, and the panel sends one page view per screen with every UUID path segment replaced by `:id`, so
  no record id of any workspace leaves the browser. A load balancer's sticky cookie must skip the
  script and `/client/v1/analytics/`, or the promise of nothing on the device breaks.

### Status pages (M19)

A workspace tells its customers whether its products work: a public status page per product with
components, incidents and planned maintenance, kept up to date by members, by checks Yuva runs and by
the product's own servers. Being the support inbox is the point: the widget and the apps show an open
incident before a customer writes, and the conversations an outage brings are answered together.

- **Pages.** `status_pages` belong to an inbox (one page per inbox at most): name, description, `slug`
  (unique across the server, `a-z0-9-`, 3–48 characters, reserved words refused), logo (storage), a
  dark mode logo (`logo_dark_url`, a `<picture>` source for `prefers-color-scheme: dark`, used only with a
  logo), accent colour, site and support links, locale (`en`, `tr`, `de`, …; the page's words follow it), time zone,
  `history_days` (90), whether uptime is shown, whether response times are shown (`show_response_times`,
  off by default), whether the header shows the name next to a logo (`show_name`, on by default; off
  leaves only the logo, with the name as its alt text), whether the page shows the Follow and embed
  section with its header link, the footer's feed and badge links and the incident pages' feed link
  (`show_follow`, on by default; the addresses keep answering), and `published`. An unpublished page answers 404 to
  everyone but members, who preview it in the panel. Renaming a slug keeps the old one in
  `status_page_slugs`: it stays reserved for that page, its public paths (page, history, incidents, feed,
  badge, JSON) answer `301` to the current slug, and it is freed only when the page is deleted; a page
  may take one of its own former slugs back. Owners and admins manage pages and monitors;
  agents with access to the inbox open, update and resolve incidents and set component states.
- **Components.** `status_components` (name, description, optional group from `status_component_groups`,
  position, `hidden`) have a state: `operational`, `degraded`, `partial_outage`, `major_outage`,
  `maintenance`. Every change closes the open row of `status_component_states` (component, state,
  source, started, ended) and opens a new one, in the same transaction as a stored event. The page's
  overall state is the worst visible component state, `maintenance` only when nothing is worse. Uptime
  for a day is the share of the day not in `major_outage`, counting `partial_outage` as half and leaving
  `maintenance` out of the day; a day bar's colour is its worst state. The source is `member`,
  `api_key`, `monitor` or `incident`.
- **Incidents and maintenance.** `status_incidents` have a `kind` (`incident`, `maintenance`), title,
  impact (`none`, `minor`, `major`, `critical`), the components they touch with the state each is set to
  (`status_incident_components`), and updates (`status_incident_updates`: state, text, author, time).
  An incident moves `investigating` → `identified` → `monitoring` → `resolved`; maintenance is
  `scheduled` (with a window) → `in_progress` → `completed`, started and completed on time by a River job
  unless a member does it first. Text is plain with links and line breaks, at most 5,000 characters, never
  HTML. A draft (`published_at` null) is seen only in the panel; publishing makes it public and, unless
  `notify` is false, queues a notice (see Subscribers). Resolving or completing puts each touched component back to `operational` unless
  another open incident or a failing monitor holds it. An update that is published cannot be removed,
  only corrected with a new one or edited with an "edited" mark; an incident can be deleted only as a
  draft.
- **The public page.** The server renders it with `html/template` like the list pages, without the
  panel and without JavaScript, so it loads when the product it describes does not:
  `<status URL>/{slug}`, `/status/{slug}/history`, `/status/{slug}/incidents/{id}`,
  `/status/{slug}/feed.atom`, `/status/{slug}/badge.svg` (overall state, for READMEs) and the JSON
  `GET /client/v1/status/{slug}` (`Access-Control-Allow-Origin: *`, no credentials). The pages follow
  the site's lined style: full-width rules, a centred column with crosses where rules meet, numbered
  mono section labels, light and dark through `prefers-color-scheme`; the page's accent colour replaces
  the brand orange. Geist and Geist Mono (OFL) are embedded in the binary and served at
  `{page}/fonts/{name}.{hash}.woff2` with a year's immutable cache, so the page asks no other host; the
  footer carries the Yuva mark inline and a link to the Yuva site, the header the page's own logo.
  - The page: overall state and headline, three cells (uptime over the last 60 days of all visible
    components, components not operational now, last change), open incidents beside running or planned
    maintenance, components by group with 60 day bars drawn as SVG cells in five colours with a legend
    and each component's uptime over those days, response times when shown, incidents of the last 14
    days, subscribing (a GET form to the existing `/subscribe` page with the components ticked, which
    pre-ticks them there; nothing is sealed or cached on the page) and follow and embed (feed, badge,
    JSON URL, iframe snippet).
  - Response times: only with `show_response_times` and only for components with an `http` or `tcp`
    monitor (paused ones too). The last 24 hours in 96 steps of 15 minutes, the median of the
    successful checks of each step as a stepped SVG line, and median and p95 over the day. Up to
    three components are tabs made of radio inputs and `:checked` rules; more are all shown, and the
    inline script turns them into a select. The JSON carries the same aggregates as
    `response_times`; addresses, errors, messages and status codes of checks are never public.
  - An incident page shows start, end and duration, response times of its monitored components from
    a margin before to a margin after it (peak during it, median of the 24 hours before it), its
    updates as rows and how each affected component fared (worst state and time off operational).
  - The history page lists every month of the page's `history_days`, newest first, with the month's
    uptime and a day strip, and `?filter=incidents` or `?filter=maintenance` keeps one kind.
  - Words come in the page's locale from the server's catalogue (`status_words.go`; en, tr and de
    are complete, a language is one map entry with its name and direction). `?lang=<code>` picks
    another catalogue language for that visitor, links on the page carry it, and an unknown code
    shows the page's locale; the language select submits a GET form (the script submits it on
    change). Times are shown in the visitor's zone through `<time>` with the page's zone as fallback.
    One small inline script (its hash in the CSP) localises times, switches the response select and
    the language, and refreshes the page every 60 seconds; without it the page is complete.
  - Every public answer is cached in the process per page, view, language and filter and dropped on
    the page's next change; it is sent with `Cache-Control: public, max-age=30, stale-if-error=86400`
    so a CDN in front takes an outage's traffic and keeps serving the last page for a day when this
    server cannot answer. Rate limit 120 requests a minute per address (fonts are not counted).
    `frame-ancestors` is `*` on the page so a host may embed it, `font-src` and `form-action` are
    `'self'`; the badge and JSON have no cookies. The status URL is `YUVA_STATUS_URL` (an origin such
    as `https://status.example.com`, served by the same server when requests for its host reach it;
    unset: `<YUVA_PUBLIC_URL>/status`); every link, the feed, the badge and the JSON name pages from
    it, and with it set `<YUVA_PUBLIC_URL>/status/{slug}` redirects there. When the page names an
    analytics property of the workspace, the page sends a cookieless page view to it with the
    analytics script.
    The bare status origin also serves a server-rendered home page (`status_home.go`): English at `/`,
    every panel locale at `/{locale}/` (lowercase code; `?lang=` answers a 301 to that path, no
    Accept-Language redirect), each with its own canonical, `hreflang` set with `x-default`, Open Graph
    and Twitter tags, a static 1200x630 share image and JSON-LD (`Organization`, `WebSite`,
    `SoftwareApplication` without offers, `FAQPage`). Words live in `homeCatalog` (one entry per locale,
    English fills what is missing); the page has no script, so its CSP has no `script-src`. The live card
    shows the published page named by `YUVA_STATUS_HOME_PAGE`; `YUVA_STATUS_HOME=off` makes `/` a plain
    404. `/robots.txt`, `/sitemap.xml` (home pages only: status pages carry no listing flag),
    `/favicon.svg`, `/og.png` and `/fonts/` are served on the same origin. Own domains keep `/` for
    their page.
- **Panel.** A Status entry in the rail (`/status`, shortcut `g s`) follows the inbox filter: the page of each
  visible inbox, or "Create status page" for owners and admins on an inbox without one. A page opens at
  `/status-pages/{id}` (not under `/status/`, which the server answers with the public page) with Incidents,
  Components, Monitors (`/status-pages/{id}/monitors`, where member notifications about a monitor going
  down or up link; keys `1` to `4` switch tabs) and, for owners and admins, Settings; an incident opens at
  `/status-pages/{id}/incidents/{incidentId}`. Owners and admins add, edit, pause and delete monitors and
  make a new address; agents with access to the inbox read them and their recent checks. A monitor's
  address is shown once, after creating it or making a new one, with copyable setup snippets (curl, cron,
  `?fail=1`, a push body, Alertmanager `webhook_configs`, a Grafana contact point note) and never again;
  a member's later state on the component is shown on the monitor as holding. Agents with access to the inbox set component states and
  open, update, publish and resolve incidents; owners and admins also manage components, groups and the page
  and publish or unpublish it. The preview shows the `/preview` HTML in a sandboxed frame. Lists and incidents
  refresh on the `status.*` realtime events.
- **Own domain.** A page may name `custom_domain` (`status.example.com`). The owner adds a CNAME to
  `YUVA_STATUS_CNAME` (unset: the host of `YUVA_PUBLIC_URL`) and a TXT record `_yuva-status.<domain>`
  with the page's verification value, so nobody claims a domain they do not hold; a River job checks
  both every 10 minutes until they are seen and every day afterwards, and a domain whose records go
  away is turned off after 3 days. State (`status_pages.domain_state`): `pending` (records not seen),
  `failed` (records found with another target or value; also checked every 10 minutes), `verified`,
  `off` (not checked again; setting the same domain again starts it over with the same value). A lookup
  that fails for another reason than a missing record changes nothing and is tried again in 10 minutes,
  so a resolver outage never turns domains off. A CNAME counts when the domain's canonical name is the
  target or, for a provider that flattens it, its addresses are all the target's. The domain is a
  hostname that is not, contains or sits inside one of the server's own hosts (`YUVA_PUBLIC_URL`,
  `YUVA_STATUS_URL`, `YUVA_STATUS_CNAME`, `YUVA_HELP_URL`); setting it queues a check at once.
  - **Other kinds (#148).** Help centers, roadmap boards and changelogs have the same own domain, with the
    same columns on `help_centers`, `roadmap_boards` and `changelogs`, the same `custom_domain` in their
    PATCH and answers, the same CNAME target, `_yuva-status.` TXT record, states, timings, claims, check job
    (`status_domain_check` with a `kind`), sweep and certificate modes; one code path in
    `status_domains.go` serves all four kinds. One domain belongs to one page of one workspace across the
    four kinds: availability, the verification under the advisory lock on the name and the failing of the
    other claims look at all four tables, and `YUVA_STATUS_DOMAIN_LIMIT` counts them together. A verified
    domain serves its page at `/`: every path maps under the page's own prefix (`/help/{slug}`,
    `/roadmap/{slug}`, `/changelog/{slug}`), so the domain shows only that page's paths (fonts, sitemap,
    the helpful form included), and `/robots.txt` names `/sitemap.xml` of the domain for help centers and
    changelogs. The page is read again by id on every request; a private board, an unpublished center or
    changelog, or one whose domain moved answers its plain `404`. Links, canonical and `hreflang` links,
    sitemaps, the Atom feed, the API's and widgets' `url` and notification links name the page on its
    domain once it is live (as for status pages). The chat embedded in help and roadmap pages accepts the
    verified own domain of a help center or roadmap board as an origin of every chat channel of the same
    workspace (checked after the channel's own origins), since the server serves every path there. Domain moves reach `/v1/realtime` as
    `help.center.updated` (`data.center_id`, `help:read`), `roadmap.board.updated` and `changelog.updated`
    (`data.changelog_id`, `changelog:read`), each also sent after a PATCH and not stored.
  - **Claims.** Only a verified domain holds its name: a partial unique index over
    `status_pages.custom_domain` where `domain_state = 'verified'`. Any number of pages, in any workspace,
    may claim a name while it is `pending` or `failed`; setting another page's verified domain is
    `409 domain_taken`, and an `off` domain blocks nobody. A check that sees both records verifies under an
    advisory lock on the name: when another page already holds it the claim becomes `failed` with problem
    `taken`, otherwise it is verified and every other claim of the name becomes `failed` (`taken`). A claim
    not verified within 7 days of being set (`status_pages.domain_claimed_at`) becomes `failed` with problem
    `expired`. `taken` and `expired` claims are not checked again until the owner sends `custom_domain`
    again, which starts the claim over (same value, 7 more days); sending a verified domain again only
    checks it at once.
  - **Serving.** A request whose `Host` is a verified domain is served that page at `/`, with
    `/history`, `/incidents/{id}`, `/feed.atom`, `/badge.svg`, `/subscribe` and `/subscribe/sent`, and
    `404` for every other path (no API or panel on a customer's domain). Hosts that are not syntactically
    domains are never looked up. Verified domains are cached host → workspace and page id apart from the
    hosts that are not one (capped and aged on their own, so arbitrary hosts never push a verified domain
    out), for 30 seconds; an entry is dropped after the commit of any change to its page (fields, slug,
    domain, check state, deletion, the inbox's deletion), and the page is read again by id on every
    request, so a stale entry never serves a page whose domain moved. Links, the feed, the badge, the
    JSON, notices and the API's `url` name the page on its domain once it is verified and, with
    `cloudflare`, once the custom hostname and its certificate are both `active`; until then they stay
    on the status URL.
  - **Certificates.** Yuva does not terminate TLS; who issues the certificate is `YUVA_STATUS_DOMAINS`:
    - `ask` (default): the operator's proxy issues certificates on demand and asks
      `GET /status-domains/check?domain=` on the same server (the Caddy `on_demand_tls` `ask` form).
      It answers `200` only for the verified domain of a published status page, help center or changelog,
      or a public roadmap board (its feature not off on the server), in a live workspace, and `404`
      otherwise (also in the other modes); it is public, rate-limited like the other public status
      routes (120 a minute per address, `429`), and reveals only domains that already serve public
      pages. `deploy/caddy/Caddyfile` documents Caddy.
    - `cloudflare`: the server registers each verified domain as a Cloudflare for SaaS custom hostname
      in `YUVA_CLOUDFLARE_ZONE_ID` (token `YUVA_CLOUDFLARE_API_TOKEN`, permission SSL and Certificates
      edit) with HTTP validation, so Cloudflare issues and renews the certificate once the CNAME points
      at the zone's fallback origin; the panel shows the hostname's and the certificate's state from
      Cloudflare (checked every 10 minutes until both are `active`), and a removed or turned-off domain,
      or one whose page, inbox or workspace is deleted, is deleted there by a job. The hostname is
      created only after the check's transaction commits, from the page read again, so a domain that was
      removed or turned off meanwhile never gets one. For servers behind Cloudflare.
    - `off`: no own domains; `custom_domain` is left out of answers, and a PATCH accepts only `null`,
      `""` or the page's current domain (unchanged, not an error); anything else is `400`. Removing a
      domain still deletes its Cloudflare hostname when `YUVA_CLOUDFLARE_ZONE_ID` and
      `YUVA_CLOUDFLARE_API_TOKEN` are set, which they may be in any mode for that.
  - **Availability and realtime.** `GET /v1/status-pages` and `/{id}` carry `own_domains: {mode}` (`ask` or
    `cloudflare`), absent when `YUVA_STATUS_DOMAINS` is `off`, so a client tells "no domain set" from
    "domains off". Realtime `status.page.updated` (`data.page_id`; `/v1/realtime` only, not stored, no
    replay) goes to members with `status:read` or `status:manage` who see the page's inbox when its
    domain changes state (verified, failed, turned off, claim lost to another page), its Cloudflare
    hostname or certificate status moves, or the page is patched; clients reload the page. The
    contacts' `status.hint` is separate.
  - `YUVA_STATUS_DOMAIN_LIMIT` caps the pages of a workspace with a domain that is not `off`, checked when
    one is set under an advisory lock per workspace, so concurrent PATCHes cannot pass it
    (`403 status_domain_limit`; unset: 1 with `YUVA_SIGNUP=open`, else no limit).
- **Subscribers.** A page may have e-mail subscribers, who get a notice when an incident or maintenance
  is published or updated. They are not a new kind of recipient: they are the subscriptions of a
  mailing list of the page's inbox, so consent, confirmation, `/s/` and `/m/` pages, suppression,
  complaints, retention and the consent export are the ones of M13. The Atom feed and webhooks need no
  subscription.
  - **Storage.** `mailing_lists.kind` (`list` default, `status`) and `status_pages.mailing_list_id`
    (unique; the list is made when e-mail subscribers are first turned on, named like the page, with
    ` (status)` added when the inbox already has a list of that name). A status list is fixed by a
    check: not `public`, not `commercial`, `double_opt_in` on, no `welcome`. It is left out of broadcast
    audiences (`400 validation_failed` naming `audience.list_ids`, as every audience check), of `GET /v1/lists` unless `kind=status`, of the forms
    and `GET /client/v1/channels/{key}/lists`, of CSV imports (`409 list_kind`), of the İYS export, and
    of the lists a marketing `/m/` page offers or "unsubscribe from everything" ends. It sits outside
    the opt-out register: leaving it writes no `email_optouts` row (also not after 3 years or when the
    contact is deleted), confirming it lifts none, and an existing opt-out does not stop a person who
    asks for notices again (they confirm by mail). A complaint still ends every subscription of the
    address and writes its opt-out, like any complaint. Purpose limitation is the reason for the
    separate kind: people agreed to incident notices, never to newsletters.
  - **Choosing components.** A subscription gets all notices of the page, or only those of chosen
    components: `status_subscriber_components` (`workspace_id`, `subscription_id`, `component_id`,
    primary key on the last two, cascade on both) and `list_subscriptions.chose_components`. A subscription
    that did not choose gets everything, including components added later. The form and the `/m/` page show
    one checkbox per visible component, all ticked; ticking all stores no rows and clears the flag, ticking
    none is refused ("unsubscribe instead"). An incident or maintenance reaches a subscriber when they did
    not choose or the incident touches one of their components; an incident touching no component reaches
    those who did not choose. The flag outlives the rows: when every chosen component is deleted the
    subscriber has the flag and no rows, and gets only incidents that touch no component, never everything
    (the subscription then lists `components: []`). Choices are preferences, not consent:
    they change without a consent event, and the consent event of the request stores the chosen
    components in `detail`. Reason for choosing per component and not per page only: a page with
    regional or product components mailing everyone about everything is what produces spam
    complaints, and the complaint rate is the one number that can suspend a workspace.
  - **Subscribing.** The page links to `GET /status/{slug}/subscribe` (own page, `no-store`,
    `frame-ancestors 'none'`; on a verified custom domain `/subscribe`). The cached page itself carries
    no form because a sealed form token in a body cached per page and per language would outlive its
    two hours. The form is plain HTML without JavaScript: address, component checkboxes, a honeypot,
    the language of the page, a sealed token (`yuva status subscribe v1`: page id and issue time, at
    least 2 seconds and at most 2 hours old) and a visible sentence in the page's language ("Send me
    notices about incidents and maintenance of {page}. I confirm by e-mail and can unsubscribe in every
    mail."), which is stored as the consent text. It posts to the same URL; the answer is a redirect to
    `/status/{slug}/subscribe/sent` ("If the address can receive mail, a confirmation is on its way"),
    identical for every address, taken or not, suppressed or not. Behind it the M13 form path runs
    (`subscribeFromForm` with the page's list): contact by address or a new unconfirmed one with the
    page's language as locale, `pending` subscription, `requested` consent event with IP, user agent,
    the page URL and the text, confirmation job. Always double opt-in. The confirmation mail
    (`status_confirm`) and the `/s/` page name the page and the chosen components; a confirmed
    subscriber gets no welcome mail. Hosts add subscribers only through `/v1/lists/{id}/subscriptions`,
    which for a status list always creates `pending` and sends the confirmation (`202`, never
    `subscribed`); there is no JSON subscribe endpoint for visitors, a host links to the page.
  - **Abuse limits.** The page's 120 requests a minute per address stay. Subscribe: 10 an hour per
    address of the visitor and 300 an hour per page, then `429` with `Retry-After` (the page says so);
    confirmation mails 3 a day per recipient address, one per 10 minutes per recipient and page, and
    1,000 a day per page, all the M13 counters keyed on the page; the honeypot and a token that is
    too young or too old answer as success (honeypot) or ask to load the form again (token).
    `YUVA_STATUS_MAX_SUBSCRIBERS` caps subscribed addresses per page (unset: 10,000 with
    `YUVA_SIGNUP=open`, else no cap; at the cap the form says the page takes no more subscribers).
    Pending ones go after 30 days like any list's, and the sweep also deletes the unconfirmed contacts
    those requests created when nothing else refers to them (no address, identity, conversation or other
    subscription), so a flood of fake addresses leaves nothing in the contacts directory.
    `YUVA_STATUS_NOTICES=off` hides the form (404) and sends nothing; owners may switch a page's e-mail
    off in its settings. The confirmation puts only the page name (at most 100 characters, plain text) in
    the subject and body; incident text appears only in notices to confirmed subscribers.
  - **Notices.** Incident and maintenance updates carry `notify` (API, MCP and panel). A new incident
    defaults to `true`; an update defaults to `true` when it changes the state, the window or is the
    resolution, and to `false` when it only adds text; a maintenance start and completion done by the
    River job follow the incident's stored `notify` and carry a text in the page's locale ("The
    maintenance has started." / "The maintenance is complete.", also in `tr` and `de`), so pages and notices
    say something. Component changes alone, drafts, and published updates with `notify=false`
    send nothing. Every update stores `notify`; the opening update of an incident takes the incident's.
    Publishing a draft sends one notice attaching every update of the draft, `notify` or not, so the notice
    always carries the opening text (shown newest first). A window edit of a maintenance mails when the
    window actually moved and the request's `notify`, else the incident's, is true.
    Resolving or completing with `notify=false` cancels the notices still waiting (`state` `cancelled`,
    `skip_reason` `closed_quietly`), so nothing mails after a quiet close. Monitor-opened published incidents notify like a member's; the budget below
    keeps a flapping monitor from mailing people. A notice is a `status_notices` row (page, incident,
    `due_at`, `state` `pending|sending|sent|skipped|cancelled`, `final`, `skip_reason`, counts) covering
    exactly the updates whose `status_incident_updates.notice_id` points at it (a later update is attached
    only when it asked to notify; attaching takes only updates of the same incident that no notice
    covers yet, and a notice never falls back to another update), `window_changed` (the notice was queued or
    joined by a change of the maintenance window), and one `status_notice_deliveries`
    row per recipient (notice, subscription, address, `email_token` and unique `header_message_id` like
    broadcast deliveries, `previous_message_id`, `state` `queued|sent|failed|bounced|complained|skipped`,
    skip reason, attempts, next attempt, times, error). Deliveries and addresses go after 90 days; the
    notice keeps its counts.
  - **Content.** Server-rendered, no designer, no merge fields, no images, no tracked links and no
    open pixel: `status_notice` and `status_confirm` in `mail/templates/{en,tr,de}` through the shared
    layout (brand: the inbox's name, the page's accent colour; dark mode as the other mails) with a text
    part. The language is the subscriber's locale (set from the page's language when they subscribed),
    else the page's. Subject `[{page}] {title}`, `[{page}] Maintenance: {title}` in the language; the
    subject does not change between notices of one incident, so mail clients keep them in one thread,
    and each notice also sets `In-Reply-To` to the recipient's previous notice of the incident and
    `References` to the chain. Body, first line: the state in words (`Investigating`, `Identified`,
    `Monitoring`, `Resolved`, `Scheduled`, `In progress`, `Completed`, never colour alone), then impact,
    the affected components with the states the incident set (a final notice, resolved or completed, lists
    them by name only), the attached updates newest first with times in the page's zone and its name, the
    incident's text (plain text; the HTML part escapes it and links only `http` and `https` URLs, marks
    "edited"); a notice with `window_changed` shows a sentence for what
    changed ("The maintenance is now scheduled for {window}", the window in the page's zone, also in `tr`
    and `de`) above its updates; the first announcement of a scheduled maintenance uses the normal wording. Then a link to
    `/status/{slug}/incidents/{id}` (on the verified custom domain when there is one), the footer: why
    the person gets it, the page, "Choose components or unsubscribe" to `/m/{token}`, the inbox's
    legal name and postal address when set (never required: notices are not commercial). A notice is
    rendered when it is sent from the current rows, so a corrected update is mailed as corrected.
  - **Sending.** Through the inbox's e-mail channel named on the list, never the server's own SMTP:
    the channel's broadcast account when set, else its support account, its From address, display name
    and Reply-To, its own DKIM key or SES identity and the SES tenant headers, so SPF, DKIM and DMARC
    align exactly as for broadcasts and bulk mail never shares reputation with a support reply's
    account. Headers: `Message-ID` `<email_token.random@sending domain>`, `List-Unsubscribe` and
    `List-Unsubscribe-Post: List-Unsubscribe=One-Click` (RFC 8058) with the recipient's `/m/` link for
    the status list, `List-Id` (`<s{list}.{domain}>` with the page name), `Feedback-ID`
    `{incident}:{list}:{workspace}:yuvastatus`, `X-Auto-Response-Suppress: OOF, AutoReply`, no
    `Auto-Submitted`. `/m/` of a status list shows only that list: unsubscribe, the component choices,
    a link back to the page; the one-click POST unsubscribes at once. Not subject to broadcast approval,
    warm-up or `YUVA_BROADCAST_DAILY_RECIPIENTS` (a notice about an outage must not wait for a
    quota), but never above these: a workspace suspended from sending (30-day rates) sends none
    (`skipped`, `suspended`), a channel without a sender or a tenant that is `identity_missing` is
    attempted as for confirmations, and `YUVA_STATUS_NOTICES_DAILY` (notice mails per workspace and UTC
    day, 0 = no limit, unset: 5,000 with `YUVA_SIGNUP=open`, else 0; beyond it deliveries stay `queued`
    with their next attempt at the start of the next UTC day, where the 6-hour staleness rule still
    applies: one older than that by then is `skipped` with `stale`). Notices count in `broadcast_sending_days` (sent, bounced, complained), so they stay
    inside the workspace's 30-day complaint and bounce limits, and in the usage counters as
    `status_notices`. Jobs: `status_notice` on its own River queue (2 workers, so a notice never waits
    behind a newsletter), one job per notice, scheduled at `due_at`; a sweep every minute queues pending
    notices that are due and sending ones whose lease ran out. The job claims the notice with one
    conditional update (`pending` to `sending`, `due_at <= now`), so a duplicate or moved job does
    nothing; checks the page is published, e-mail on, the incident published and the channel ready;
    snapshots recipients (subscribed, not suppressed in any scope, component match) into deliveries in
    1,000-row transactions with `ON CONFLICT DO NOTHING`; sends in batches of 50 or 5 seconds over one
    SMTP session at the channel's `broadcast_rate`, re-checking each address before its mail
    (still subscribed, not suppressed, notice not cancelled). Per-broadcast `pace` does not apply.
    Temporary failures retry after 1, 5 and 30 minutes, then fail; a delivery more than 6 hours old
    is `skipped` (`stale`): late news is worse than none. `5.1.x` and `5.2.1` answers are bounces.
    The job does not pause anything; the panel shows the counts and the page's last notices.
  - **Bounces, complaints, replies.** Delivery reports, ARF reports and SES notifications find a
    notice delivery by `header_message_id` or the relay's id in the places that find broadcast
    deliveries (`FindStatusNoticeDeliveryByHeader` and `...AnyWorkspace`, the second across
    workspaces because SES names only the message; everything after it is scoped by the delivery's
    `workspace_id`). A hard bounce suppresses the address for all mail; a complaint suppresses
    broadcasts, unsubscribes every list of the address and writes an opt-out, exactly as for a
    broadcast. A reply to a notice (found by the token in `In-Reply-To` or `References`) opens or
    continues a conversation with that contact in the inbox and links it to the notice's incident
    (`status_incident_conversations`), so "Reply to linked" reaches it; automatic replies are only
    counted.
  - **Throttling.** Notices of one incident coalesce and keep a distance: a notice is due 60 seconds
    after the update that asked for it (the window for fixing a typo, and for "Undo" in the panel,
    `POST /v1/status-pages/{id}/notices/{notice_id}/cancel`, `409 not_pending` once sending); an update
    with `notify` that arrives while a notice of the incident is `pending` joins it and keeps its
    `due_at`; a notice of an incident is due no earlier than 15 minutes after the previous one of that
    incident was sent, except `final` ones (resolved, completed, maintenance started), which are due
    after 60 seconds and pull a waiting notice forward. A page sends at most 10 notices in any rolling
    hour; further ones are `skipped` (`rate`) and the panel says so; `final` notices are exempt from the
    gap and the rate, but go only to subscribers who got a sent notice of that incident, and a final notice
    with none sent before it is `skipped` (`nothing_earlier`): nobody heard of the incident, so nobody is
    told it ended, and a skipped opening notice cannot be bypassed through the closing one (an incident
    resolved while its first notice still waits sends nothing). With the
    gap this means a subscriber gets at most one mail per quarter of an hour about an incident and ten
    an hour from a page, however often members post.
  - **Panel and API.** Settings of a page: e-mail subscribers on or off and the e-mail channel (the
    inbox's channels that can send; `409 sender_not_ready` otherwise; a missing deliverability check,
    a missing postal address or an unaligned DKIM warn, never block). A Subscribers tab (counts,
    search, state, chosen components, remove, add by address which sends the confirmation, consent
    export; the list endpoints of M13 with `kind=status`, scopes `lists:*`). In the incident
    composer a "Notify subscribers" checkbox beside Publish and beside each update, with the number it
    would reach, and in the timeline a notice line (scheduled for, sending, sent to N, N failed, N
    bounced, skipped and why) with Undo during the hold. `GET /v1/status-pages/{id}/notices`
    (`incident_id`, `state`, cursor) and `/{notice_id}`, scope `status:read`; cancel needs
    `status:manage`. `GET /v1/status-pages/{id}` shows `email_subscribers` (`enabled`, `channel_id`,
    `list_id`, counts) and `status.*` events and webhooks are unchanged; progress reaches
    `/v1/realtime` as `status_notice.updated` (not stored). `subscription.*` events fire for status
    lists with `list.kind`. MCP: `create_incident` and `post_incident_update` take `notify`,
    `get_status_page` returns the subscriber counts; no tool reads addresses.
  - **Law.** Subscribers asked for the notice and confirmed by mail; the consent events (IP, user
    agent, page, the sentence they saw, the components) serve GDPR Art. 7 and 15 and KVKK, and the
    consent export includes status lists without an İYS format. Notices contain no promotion, so they
    are service messages: no İYS record, CAN-SPAM's transactional exemption (accurate From and
    routing) applies, and the footer still names the sender and offers one-click unsubscribe, which
    Gmail and Yahoo require of subscribed bulk mail. Links never expire.
- **Monitors.** `status_monitors` set a component's state from outside a member's hand. Kinds:
  - `heartbeat`: the host's job calls `POST` or `GET /ingress/heartbeat/{token}` at least every
    `interval` plus `grace`; a missed beat fails the monitor, `?fail=1` or `/fail` fails it at once.
    For servers and cron jobs Yuva cannot reach.
  - `push`: the host's monitoring sends the state: `POST /ingress/status/{token}` with
    `{"state":"major_outage","message":"..."}`, or an Alertmanager or Grafana alert webhook body as it
    is (firing fails, resolved recovers).
  - `http`: Yuva sends a request (method, headers, body; header values encrypted under the master key)
    every `interval` (60 seconds at least, `YUVA_STATUS_MIN_INTERVAL` on a hosted server; unset: 60, or
    300 with `YUVA_SIGNUP=open`) and expects a status range, optionally a keyword in the first 64 KiB, an
    answer within `timeout` (at most 30 seconds) and, for https, a certificate valid for more than
    `cert_warn_days` (default 14; a certificate expiring sooner fails the check, it does not degrade).
    The certificate is the one of the monitored URL's own host, never of a redirect target.
    Header names are returned, values never; a redirect to another host drops the monitor's headers.
  - `tcp`: a connection to host and port within `timeout`. The host is a name or an IP address, an
    IPv6 address bare (no brackets); a host with a port or brackets is refused, the port is its own field.
  Outside checks follow the webhook rules for outbound requests (resolve once, refuse loopback, private,
  link-local, CGNAT and reserved addresses, connect to the resolved address, no proxy; redirects are
  followed only when the monitor allows it, at most 5, each hop checked again), with
  `YUVA_WEBHOOK_ALLOW_PRIVATE` lifting the address check on a self-hosted server. A monitor fails after
  `failure_threshold` failures in a row and recovers after `recovery_threshold` successes (outside
  checks default to 3 and 2, heartbeat and push monitors to 1 and 1, since the sender already decided);
  then it sets its component to its failure state (`degraded`, `partial_outage` or
  `major_outage`) or back, never over a state a member or key set later than the monitor's last change (a later change of the
  monitor, such as a new failure after a recovery, applies again; the same rule holds when other holders
  are settled), and opens a draft incident, a published one, or none (`on_failure`). Checks run on a River queue of
  their own, at most 20 at a time per process, spread over the interval; results go to
  `status_checks` (time, ok, duration, status code, error) kept 30 days for the response-time chart.
  While a monitor is down its incident follows the state it claims (impact and component state, worse or better),
  and pausing or deleting it resolves that incident with an update saying so and puts the component back
  unless something else holds it. A heartbeat or push call counts first against a per-address limit
  (600 a minute per client address, IPv6 per /64; unknown tokens count too) and then against the monitor's
  own limit (60 a minute), which is created only after the token is found. The overdue-beat sweep is one job at a time and finds the due monitors of all workspaces
  in one query, then handles each in its workspace; one failing monitor is logged and the others go on.
  A monitor going down or up notifies the inbox's members like a new conversation does (push, e-mail
  per their settings), one job per member and channel carrying the transition's state, message and
  time, so a retry never sends anything twice. Hosted limits: `YUVA_STATUS_MONITORS` per workspace (unset: 20);
  for subscribers `YUVA_STATUS_NOTICES` (`on|off`), `YUVA_STATUS_NOTICES_DAILY` and
  `YUVA_STATUS_MAX_SUBSCRIBERS` (see Subscribers).
  Checks run from the server's own place and a multi-location check is not planned, so the server
  tells its own trouble from its customers' with canaries alone: `YUVA_STATUS_CANARY_URLS` (unset:
  `https://www.google.com/generate_204` and `https://www.cloudflare.com/cdn-cgi/trace`; any answer
  counts; `off` turns the guard off). How many checks fail never decides it, since across workspaces
  one tenant's failing monitors would erase another's real outage and within one it would hide a
  shared backend dying. The canaries are probed only when a round has a failure, and their verdict
  is kept for 60 seconds. The guard is armed only while the canaries answered at least once in the last
  hour: then a round whose canaries all fail has its failures dropped, no state changes, the operator
  gets a log line and the `yuva_status_rounds_dropped_total` metric. Canaries that have not answered
  for an hour (a firewalled or offline server) leave the guard off, failures count, and the server
  logs once an hour that the canaries are unreachable and `YUVA_STATUS_CANARY_URLS` should name URLs
  it can reach or be `off`. A round is one sweep: it claims the checks due in the next 30 seconds,
  oldest first, as long as their timeouts add up to no more than the check queue's workers can run
  before the round's deadline (20 workers × 40 seconds), so the rest wait for the next sweep instead
  of starting late; it keeps their results pending in `status_checks` and judges them together after
  the last can have finished. The judgement is stored on the round's checks (`round_dropped`) before
  any is applied, so a retried judging applies the same one; a round's successes always count. A
  check that still starts too late to finish by its round's deadline runs anyway and is judged at once
  in a round of its own, by the same rule, so no monitor goes unchecked. The sweep, the claim and the
  judging read across workspaces (the documented exception, like the overdue-beat sweep) and act on
  each monitor in its workspace; heartbeats and pushes are not affected by this, but a heartbeat whose
  beats stop while the server itself was not taking requests (its own start within the grace) waits
  one more interval.
- **Support.** `GET /client/v1/channels/{key}`, the client session and the realtime `inbox.updated` carry `status` on the
  inbox: the page URL, the overall state and the open published incidents and running maintenance (id,
  title, impact, state, last update, URL). The widget and the Swift and Kotlin SDKs show a dismissible
  line above the conversation list and the composer while one is open. In the panel a conversation can
  be linked to an incident (`status_incident_conversations`), the incident lists its conversations and
  their states, and "Reply to linked" sends one reply, written once, to every linked open conversation
  as the member, then optionally closes them; it is one job per conversation through the normal reply
  path, so e-mail, chat and app all get it the usual way. The body is stored once per request
  (`status_incident_replies`) and the jobs reference it; jobs are inserted in batches, and an incident
  takes at most 5,000 linked conversations (`422 too_many_links`). Each conversation's outcome
  (`pending`, `sent`, `skipped` with `closed`, `unlinked`, `conversation_deleted` or `revoked`, `failed`
  with the problem code or `gave_up`) is kept on `status_incident_reply_targets` and read with
  `GET /v1/status-pages/{id}/incidents/{id}/replies/{replyId}`; the `202` carries that id. A job
  checks its caller again when it runs: a revoked or expired key, a revoked OAuth grant, a removed
  member or a lost scope sends nothing.
  The panel reads a reply's outcome every two seconds while any conversation is `pending`, since no realtime
  event carries it, and lists the skipped and failed ones when it is done.
- **API and access.** `/v1/status-pages`, `/v1/status-pages/{id}/components`, `/groups`,
  `/incidents`, `/incidents/{id}/updates`, `/incidents/{id}/conversations`, `/monitors`,
  `/monitors/{id}/checks`; `/v1/status-pages/{id}/preview` renders the public page for members.
  Scopes `status:read` and `status:manage`; neither goes to existing keys, `status:read` goes to keys
  made after M19 without named scopes, and `status:manage` never does (like `broadcasts:send`: name it
  when the key is made). A key limited to inboxes sees the pages of those inboxes. Stored
  events and webhook types: `status.component.updated`, `status.incident.created`,
  `status.incident.updated` (every published update), `status.incident.resolved`,
  `status.monitor.down`, `status.monitor.up`. MCP tools: `list_status_pages`, `get_status_page`,
  `set_component_state`, `create_incident`, `post_incident_update`, `resolve_incident`,
  `list_monitors`, `link_conversation_to_incident`; publishing from MCP follows the same draft rule as the panel unless the workspace lets
  assistants publish incidents.
- **Lookups before the workspace is known.** A public page finds its page by `status_pages.slug`, a
  former slug in `status_page_slugs` or a verified `status_pages.custom_domain`, all unique across the
  server, and a slug's availability is checked against current and former slugs of every workspace and a domain's
  against the verified domains of every workspace, and a domain's verification turns the other claims of
  that name, in every workspace, to `failed`; the domain sweep lists the own domains due for a check of every workspace in
  one query; a heartbeat or push finds its
  monitor by `status_monitors.token_hash`, and the sweep of overdue heartbeats lists the due monitors of every
  workspace in one query; an SES notification finds a notice delivery by
  `status_notice_deliveries.header_message_id`, unique across the server. Everything after those lookups is scoped by the workspace
  they returned.

### Help center (M20)

A workspace answers common questions once, in writing, where customers look first: a public help
center per product with collections and articles in several languages, searched from a page, from the
widget's Home and from the inbox's composer. Yuva hosts these support articles; product documentation
stays on the products' own sites (see "Documentation pages"). Being the support inbox is the point:
a visitor finds the answer before writing, a member sends an article as a card instead of retyping it,
and the searches that found nothing say which article to write next.

- **Concepts.** A *help center* belongs to an inbox (one per inbox at most) and has a public slug, the
  locales it publishes and `published`. A *collection* groups articles, with one level of
  sub-collections at most. An *article* belongs to one collection or none and has one *translation* per
  locale; a translation has a working draft and, once published, a frozen published copy. A *card* is
  an article sent in a conversation.
- **Tables.** Every table carries `workspace_id` with composite foreign keys `(workspace_id, x_id)`.
  - `help_centers`: `inbox_id` (unique per workspace), name, description, `slug` (unique across the
    server, `a-z0-9-`, 3-48, the reserved words of status pages), `default_locale`, `locales` (1-20),
    logo, accent colour, site link, `chat_channel_id` (the chat the public pages embed under "Still
    need help?"), `analytics_property_id`, `agents_may_publish` (default false), `published`, and
    `changed_at`, moved by every change the public side shows (the cache key). `help_center_slugs`
    keeps former slugs, which answer `301` as with status pages.
  - `help_collections` (center, `parent_id`, a per-center `number` for URLs, a fixed-set `icon`,
    `position`) and `help_collection_translations` (locale, name, description, slug). A collection with
    no published article in a locale is hidden in that locale.
  - `help_articles` (center, collection, per-center `number`, `position`, up to 20 `paths` patterns
    such as `/billing/*` where the widget suggests it, creator) and `help_article_translations` keyed
    by `(article, locale)`: draft `title` (3-200), `body` (Markdown, 100,000 characters), `keywords`
    and `slug`; `version` (writes carry `base_version`, `409 version_conflict`); the `email_images`
    the draft uses; the frozen `published_*` copy with `published_html` rendered once at publish and an
    excerpt; `has_unpublished_changes`; and a `search` tsvector generated from the published copy with
    the configuration of the locale (title weight A, keywords B, body C, `İ`/`ı` folded).
  - `help_article_revisions`: snapshots of title, body and keywords (kind `save`, at most one per 10
    minutes per author, `publish`, `restore`), the newest 50 per article and locale are kept.
  - `help_article_daily` (views, helpful up and down, conversations started, times sent as a card)
    and `help_search_daily` (normalized query, searches, searches without result, clicks): counters
    per day, without any visitor data; 25 months and 90 days.
  - `messages.help_cards` (up to 3 snapshots taken at send time) and `conversations.help_article_id`
    (the article read just before the conversation was started from it).
- **Writing and publishing.** Agents with access to the inbox write drafts, collections and
  translations. Publishing, unpublishing, deleting a published article and the center's settings are
  for owners and admins, unless the center's `agents_may_publish` is on, which lets agents with
  inbox access publish too. MCP and OAuth tokens never publish, whatever the setting. A save needs
  `base_version`; another member's open editor is told by a realtime-only signal that the article
  changed elsewhere. Publishing freezes the draft, renders it, moves `changed_at` and stores an event
  (`help.article.published`, `.unpublished`, `.deleted`, also webhook types). Translations exist only
  in the center's locales. The editor shows the default-locale text beside the one being translated,
  marks translations that are older than the default and lists the missing ones; Yuva runs no
  translation model.
- **Endpoints.** Tag `help`. Members, under `/v1/help-centers`: centers (list, create, update, delete,
  a preview rendered for a sandboxed frame), collections, articles (list with filters for collection,
  locale and state `draft`, `published`, `changed` or `missing`), `PUT` and `DELETE` of a translation,
  `publish` and `unpublish`, revisions with restore into the draft, a Markdown render for the editor,
  image upload into `email_images` (same rules and size cap), search (with drafts for members) and a
  report (7, 30 or 90 days: top articles, helpful share, conversations started, searches, queries
  without result, articles with most "no"). `POST /v1/conversations/{id}/messages` takes
  `article_ids` (up to 3 published articles). The client side, for `chat` and `app` channel keys under
  `/client/v1/channels/{channel_key}/help`: the center with collections, best matches for the page
  and popular articles; `search`; collections; articles (with the locale actually served, falling back
  to the default, and related articles); `reactions`; and `searches/click`. Browsers need an allowed
  origin; nothing creates a contact; `404 help_off` without a published center. `ClientInbox.help`
  (`url`, `locales`, `default_locale`) is present only with a published center, and the Help tile and
  the Home search show only then. `ClientMessage.cards` carries article cards, and
  `ClientConversationCreate.help_article_id` is kept only for a published article of the inbox's
  center and dropped silently otherwise.
- **Scopes and access.** `help:read`, `help:write` and `help:publish` are API key scopes. The first
  two go to keys made after M20 without named scopes; `help:publish` is not implied and must be named
  (like `status:manage`); no existing key gains any. Key inbox limits and OAuth narrowing apply as
  everywhere, and a center of another inbox answers `404`. No OAuth client is offered
  `help:publish`, and OAuth tokens cannot create, change or delete a center.
- **Public pages.** Rendered by the server with `html/template`, no JavaScript needed, in the style of
  status pages and sharing their fonts, CSP builder and cache with them: `/help/{slug}` (or under
  `YUVA_HELP_URL`, an origin served by the same server, as `YUVA_STATUS_URL` works), then
  `/{locale}/`, `/{locale}/collections/{number}-{slug}`, `/{locale}/articles/{number}-{slug}`,
  `/{locale}/search?q=` and `/sitemap.xml`. A wrong slug in a URL answers `301` to the canonical one,
  so a rename needs no slug history. There is no redirect from `Accept-Language`; every page has
  `hreflang` alternates for its published translations, a canonical link, Open Graph tags and JSON-LD
  `Article` and `BreadcrumbList`. An unpublished center or article answers `404` with the plain text
  of status pages. "Was this helpful?" is two POST buttons that work without JavaScript: a form post to
  `/{locale}/articles/{number}-{slug}/helpful` with `rating` (`up` or `down`) and `previous` (the earlier
  answer, which it replaces), 4 KiB at most, rate limited per address and per center. It answers
  `303` to the article with `?thanks=up` or `?thanks=down` and the fragment `#helpful`, where the page
  shows the thank-you line and one button to change the answer, which sends the earlier one as
  `previous` (any other `thanks` value is ignored); a small script posts the form in place with
  `Accept: application/json` and gets `204`, and falls back to the form post if that fails. The answer is `no-store`; an unknown or unpublished article,
  or a locale the center does not offer, answers the center's `404`. With a
  `chat_channel_id` the page embeds `<yuva-chat>` under "Still need help?", and a conversation started
  there carries the article.
  Own domains (`help.example.com`) are the status pages' own domains (M19 › Own domain › Other kinds,
  #148): the center answers at `/` of its verified domain, its links, `hreflang`, sitemap and `url` name it
  there.
- **Rendering.** One goldmark instance for members' content: GFM tables, strikethrough, autolinks,
  heading ids (anchors and a table of contents from h2 and h3), fenced code without highlighting; raw
  HTML is dropped. Images stay only when their URL is this server's `/i/{token}` of the same
  workspace, else they become links. A bluemonday policy based on the discussions' one adds heading
  ids, `loading="lazy"` on images and `rel="noopener"` on outside links. HTML is rendered once at
  publish and stored.
- **Search.** Postgres full-text search per language, shared by the pages, the widget, the panel and
  MCP. The query is normalized, cut to 8 words of at most 50 characters and built as a `tsquery`
  from parameters, never concatenated: every word ANDed and the last one a prefix for type-ahead;
  when nothing matches it is tried again with OR and the results are marked `partial`. Ranking is
  `ts_rank_cd` with the weights, a bonus for an exact title prefix and views over 30 days as a
  tie-break. Highlights are word offsets computed on title and excerpt as plain text, never HTML.
  The requested locale is searched, and the default locale too when the requested one has fewer than
  3 published articles. A search runs with a 500 ms statement timeout on a partial GIN index of
  published rows. Only a final search (Enter, a result opened, or 1.5 seconds without typing) is
  counted; counts are buffered in process and flushed every minute, so a search never writes in its
  request.
- **Languages.** The panel's help screens have their own Lingui catalog, `help`, in every panel
  language; the widget's Help app uses the existing `apps` catalog. Server pages have a word map per
  language like status pages, complete in en, tr and de, with English as the fallback. The widget
  picks its locale from the center's `locales` (exact, then base language, else the default); a page
  uses the locale in its path.
- **Caching.** An in-process cache per center, view, locale, id and query, keyed on `changed_at` with
  a 5 minute TTL and a cap of 5,000 entries. Pages: `public, max-age=60, stale-while-revalidate=300,
  stale-if-error=86400` with an `ETag`; client JSON `public, max-age=60`; reactions and clicks
  `no-store`. Views are counted before the cache lookup, so cached answers count; answers served by
  a CDN do not, and the report says views are approximate.
- **Abuse limits.** Public pages 120 per minute per address; client search 30 per minute per address
  and channel and 1,200 per minute per center (`429` with `Retry-After`); one counted view per
  address, article and hour; reactions as `page-ratings` (per address and channel, `previous` moves the
  count); search clicks 60 per minute per address; the editor preview 60 per minute per caller; at
  most 3 cards per message; queries at most 200 characters. `YUVA_HELP_MAX_ARTICLES` caps articles
  per workspace (unset: 500 with `YUVA_SIGNUP=open`, else no limit). `YUVA_HELP=off` removes the
  feature: its routes answer `404` and the client inbox has no `help`. `usage_counters.help_searches`
  counts searches per workspace and month.
- **Cards.** The composer's Help button and `/` palette search articles; picking one adds a chip and
  sending posts `article_ids`. The server checks each article is published, picks the locale (the
  contact's, else the inbox's, else the center default), stores the snapshot in `messages.help_cards`
  and counts `sent`. Chat and app get `cards` and the widget opens the article inside its Help app;
  clients that do not know `cards` show the message body, whose last line is the URL. E-mail gets
  "Title - URL" in the text part and a bordered block in the shared layout in the HTML part, without
  tracking. A later unpublish leaves old messages readable; the link then shows the center's `404`
  text.
- **MCP.** Yuva's own shapes as for status pages. Article text written by members is not
  `customer_content`; visitors' search queries are. Tools: `list_help_centers`, `search_help_articles`,
  `get_help_article`, `help_search_gaps` (read-only; queries without result and articles with most
  "no"), `draft_help_article` and `update_help_article_draft` (drafts only, `base_version`, a
  `locale` lets an assistant write a translation draft; the result says a member publishes it in
  Yuva), and `draft_reply` takes `article_ids`. Resource `yuva://help-center/{id}`; prompts
  `write_help_article` and `help_gaps_review`. No tool publishes.
- **Analytics.** A center with an `analytics_property_id` sends cookieless page views exactly as
  status pages do; `analytics_daily_support` then counts conversations started on help pages next to
  their views, with no new analytics table. Searches without result and conversations per article
  are Yuva counters, not analytics events: they work without a property and never hold a visitor
  hash. Whether the widget's Help app sends a `help_article_view` event to a linked property is
  decided with the analytics architect before the widget step.
- **Importing articles (#150).** `POST /v1/help-centers/{id}/imports` reads a Zendesk Guide or Help
  Scout Docs into the center as drafts, never published, through the help-desk transport (GET only,
  the desk's budget, images fetched without the token outside its hosts). Zendesk reuses the sealed
  OAuth sign-in of a Zendesk migration (`migration_id`), so it needs a migration manager (owner, admin,
  or a key or token with `migrations:manage`); anyone else gets the same `404` as for an unknown id; its `read` scope already covers the Help
  Center API, so no scope was added. Help Scout's Docs API takes a Docs API key (Basic auth to
  `docsapi.helpscout.net`), not the Mailbox sign-in, so the request carries `docs_api_key`, sealed in
  `help_imports.sealed` under context `help-import:`+workspace+import and cleared when the import
  ends; browser sessions only, so an assistant starts Zendesk imports only. Categories become
  collections and sections sub-collections (Docs: collections and categories), deeper sections
  `Parent / Child`; translations map to the center's locales (same tag, else the one center locale of
  that language), others are listed in `skipped_locales`. `internal/migrate/helpdesk/htmlmd.go`
  converts the HTML subset to Markdown. A River job (`help_import`, default queue, 25-second slices,
  checkpoint on the row) runs the source, collections, articles and links phases;
  `help_import_items` keys every collection and article translation by origin, kind, source id and
  locale, so a re-run updates instead of duplicating, leaves an unchanged one alone and keeps a draft
  whose `version` moved since (unless `overwrite_edited`). Links between imported articles are
  rewritten in a last pass. The same table holds the old URL, and `listHelpImportRedirects` and
  `redirects.csv` pair it with the page's current URL; setting up redirects is the owner's. One import
  per center at a time, 10 starts an hour per workspace. Drafts send `help.article.updated`;
  `help.import.updated` and `help.import.progress` are realtime signals for `help:write` holders who
  see the inbox, never stored or sent to webhooks.

### Roadmap board (M21)

A workspace shows its customers what it is building and lets them say what they need: a roadmap board
per inbox with ideas in stages, votes and suggestions from readers, and the feedback behind each idea.
Being the support inbox is the point. A feedback about the same need is linked to the idea instead of
being answered again. Moving the idea tells every linked customer where it stands, and when the idea
ships, one reply reaches all of them on their own channel. An idea is a discussion of its own kind, so
posts, votes, follows, moderation, reader accounts, rendering and notifications are the ones of M12.

- **Concepts.** A *board* belongs to an inbox (one per inbox at most), is switched on by creating it,
  and is `public` or not. A private board is a triage tool for the team only. An *idea* is a discussion
  with `kind` `idea` and no page. It has a *stage*: `ideas`, `planned`, `in_progress`, `shipped` or
  `closed`. A *link* ties one feedback conversation to one idea. An idea has many links, and a feedback
  has at most one.
- **Tables.** Every table carries `workspace_id`, with composite foreign keys `(workspace_id, x_id)`.
  - `roadmap_boards` holds:
    - `inbox_id` (unique per workspace) and `channel_id` (the chat channel of the inbox that the public
      page embeds and member-made ideas are filed on; required)
    - name, description, and `slug` (unique across the server, `a-z0-9-`, 3-48 characters, the reserved
      words of status pages)
    - `public` (default false), `suggestions` `off|moderated|open` (default `moderated`) and
      `analytics_property_id`
    - `changed_at`, moved by every change the public side shows except votes (the cache key)

    `roadmap_board_slugs` keeps former slugs, which answer `301` as for status pages. Slug checks,
    reserved words, availability and the redirect are the status pages' helper, made generic for status,
    help and roadmap. The page's logo and accent colour are the inbox's `branding` (`logo_url`,
    `color`), so there is no second place to keep them.
  - `discussions` gains:
    - `kind` (`page` default, `idea`) and `board_id`, which is set exactly when `kind = 'idea'` (FK to
      the board, `ON DELETE CASCADE`, so deleting a board deletes its ideas)
    - `stage`, which is set exactly when `kind = 'idea'`, and `stage_changed_at`
    - `vote_count`, the opening post's votes, kept on the row in the vote's transaction because the board
      sorts by it
    - `search`, a `tsvector` generated from the title with the `simple` configuration (`İ`/`ı` folded)

    `page` becomes nullable, with `CHECK ((kind = 'page') = (page IS NOT NULL))`. An index on
    `(workspace_id, board_id, stage, vote_count DESC, id)` serves the board. Every existing page query
    (docs pages, sites, summary, the client page list) adds `kind = 'page'`, so ideas never count as
    documentation threads.
  - `roadmap_idea_conversations`: `conversation_id`, `idea_id`, `linked_by_member_id`,
    `linked_by_api_key_id` and `linked_at`, with primary key `(workspace_id, conversation_id)`, which
    makes the link many to one. Both FKs cascade. It is shaped like `status_incident_conversations`.
  - `roadmap_stage_changes`: idea, `from_stage`, `to_stage`, the member or key and the grant, `via`,
    `progress_set` and `progress_skipped` counts, and time. It is the idea's history in the panel and the
    "Planned on", "Shipped on" dates on the public page.
  - `status_incident_replies` and `status_incident_reply_targets` are renamed `linked_replies` and
    `linked_reply_targets`. `linked_replies.incident_id` becomes nullable next to a new `idea_id`, with
    `CHECK (num_nonnulls(incident_id, idea_id) = 1)`. "Reply to linked" is one mechanism for incidents
    and ideas, not two.
  - `reader_notifications.kind` gains `stage`.
- **Stages and progress.** Agents with access to the inbox move an idea's stage, and so do owners and
  admins. A move is `POST /v1/discussions/{id}/stage` with `stage` and an optional `comment`, which is
  posted as the member's published reply in the same transaction. The move writes a
  `roadmap_stage_changes` row, moves `changed_at` and stores `discussion.stage_changed`.
  - **Progress on linked feedback.** For `planned`, `in_progress`, `shipped` and `closed`, the move also
    queues one `roadmap_progress` River job. It sets every linked feedback's progress: `planned` to
    `planned`, `in_progress` to `in_progress`, `shipped` to `done` and `closed` to `wont_do`. A move back
    to `ideas` changes no progress.
  - **How the job runs.** It walks the links in batches of 500. Each conversation changes in its own
    transaction through the same code as setting progress on the conversation update, so the timeline
    gets a `feedback_progress` event (with `idea_id` in its data, shown as "via idea …"), the contact
    sees the line, and `conversation.updated` goes to webhooks. Feedback that already has that progress,
    and deleted conversations, are skipped and counted. Before each batch the job reads the idea's stage
    again and stops when it no longer matches its change, so a newer move wins and two moves never
    interleave. The job checks its caller again as "Reply to linked" does: a revoked key or grant, a
    removed member or a lost scope sets nothing.
  - **Later changes.** A member may still set a linked feedback's progress by hand. It holds until the
    next move. Linking a feedback to an idea past `ideas` sets its progress at once, in the link's
    transaction. Unlinking leaves progress as it is.
  - **Votes and replies after the end.** `shipped` and `closed` ideas take no new votes
    (`409 idea_closed`). Replies follow the lock rules of M12. Ideas have no accepted answer, and
    `…/answer` answers `409 not_supported`.
- **Linking and Reply to linked.**
  - **Linking.** Only `feedback` conversations link (`409 not_feedback`), and only to an idea of a board
    of the same inbox (`404` otherwise). A link replaces the feedback's previous link. An idea takes at
    most 5,000 links (`422 too_many_links`, the incidents' limit and code).
  - **Starting an idea from feedback.** `POST /v1/conversations/{id}/discussion` takes `kind: idea`. It
    starts a published idea with the title and body the member writes and links the conversation. The
    author rule of M12 applies: the contact when it is a known user, else the member. Nothing of the
    feedback becomes public.
  - **Reply to linked.** It is the incidents' mechanism, unchanged: one reply written once to every open
    linked conversation, optionally resolving them, one job per conversation through the normal reply
    path, with outcomes on `linked_reply_targets`. The River job kind becomes `linked_reply`, and a
    worker for `status_reply` stays for one release so queued jobs finish. The job's required scope
    follows the source: `status:manage` for incidents, `roadmap:write` for ideas, plus `messages:write`
    (and `conversations:write` to close). When an idea moves to `shipped`, the panel offers "Reply to
    linked" right away.
- **Suggestions and votes.** These follow the discussion write rule. A reader token, an identity-token
  contact session or a member writes. Anonymous visitors only read, and there are no anonymous votes.
  - **Moderation.** A board's `suggestions` mode works as a channel's `discussions` mode does for page
    threads. `moderated` (the default) holds a reader's idea as `pending` until one of their posts in the
    workspace has been approved. `open` publishes at once. `off` takes no suggestions, but votes and
    replies still work. Pending ideas sit in the existing moderation queue (`/v1/discussion-posts`,
    `state=pending`, items carry `kind`), and spam marking is M12's.
  - **Votes and following.** A vote is the existing vote on the idea's opening post. Voting follows the
    idea.
  - **Notifying followers.** A move to `planned`, `in_progress`, `shipped` or `closed` adds a `stage`
    reader notification for every follower linked to a person. It queues the existing discussion
    e-mail job, so the digest lists the stage change next to the replies, with the same delay,
    unsubscribe and locale rules.
  - **Reader tokens.** They are issued for a channel when it has discussions on, or when its inbox has a
    public board. The origin must be one of the channel's, or `YUVA_PUBLIC_URL`, where the server's own
    pages live, as `/client/v1` already counts it.
- **Endpoints.** Tag `roadmap`.
  - **Members.**
    - Boards: `/v1/roadmap-boards`: list, create, get, update, delete, and `GET …/{id}/preview` (the
      public page for a sandboxed frame).
    - Ideas are discussions. `GET /v1/discussions` takes `kind` (default `page`, so existing callers see
      no change), `board_id`, `stage`, `sort` (`votes`, `new`, `activity`) and `q` (the title search).
      Members start ideas with `POST /v1/discussions` and `kind: idea`, and retitle, lock, hide and
      delete them with the existing calls.
    - Stages: `POST /v1/discussions/{id}/stage` and `GET /v1/discussions/{id}/stage-changes`.
    - Links: `PUT` and `DELETE /v1/discussions/{id}/conversations/{conversationId}`. Linked feedback is
      `GET /v1/conversations?idea_id=` with its filters, and `idea=none` lists feedback without one. A
      `Conversation` carries `idea` (`id`, `title`, `stage`) when linked.
    - Replies: `POST /v1/discussions/{id}/linked-replies` and `GET …/linked-replies/{replyId}`, the
      incident schemas made shared (`LinkedReplyCreate`, `LinkedReply`, `LinkedReplyTarget`).
  - **Client**, for `chat` keys:
    - `GET /client/v1/channels/{channel_key}/roadmap` returns the board: name, description, URL,
      `suggestions` and counts per public stage.
    - The existing `GET …/discussions` and `…/discussions/{id}` take `kind=idea` with `stage`, `sort`
      (`top`, `new`) and `q` instead of `page`.
    - `POST /client/v1/discussions` takes `kind: idea` without `page_url`.
    - Votes, replies, follows, edits and deletes are the existing calls.
    - Answers: `404 roadmap_off` without a public board. `ClientInbox.roadmap` (`url`, `suggestions`) is
      present only with a public board, and the Roadmap tile shows only then. `closed` ideas are left out
      of public lists, while their own URL answers with the stage.
- **Scopes and access.** `roadmap:read` (boards, stage history, linked counts), `roadmap:write`
  (links, linked replies, board settings for keys) and `roadmap:stage` (moving a stage). Reading ideas
  through `/v1/discussions` keeps `conversations:read`, and linking also needs it.
  - **Which keys hold them.** The first two go to keys made after M21 without named scopes.
    `roadmap:stage` is never implied and must be named, like `status:manage`. No existing key gains any.
  - **Roles.** Board settings and deleting a board are for owners and admins. Agents with access to the
    inbox link feedback, start ideas and move stages. Key inbox limits and OAuth narrowing apply as
    everywhere, and a board or idea of another inbox answers `404`.
- **Events and webhooks.** Ideas send the existing `discussion.created`, `.post_created` and `.updated`,
  with `kind` and `stage` on the discussion. The new `discussion.stage_changed` (`{discussion, from, to,
  linked}`) is a stored event for members who see the inbox and a webhook type. Links store no event, as
  with incidents. Progress set by the job sends the usual `conversation.updated`. Board changes reach
  `/v1/realtime` as `roadmap.board.updated` (not stored).
- **Public page.** It is rendered by the server with `html/template` and needs no JavaScript. It uses the
  style, fonts, CSP builder and cache of status and help pages.
  - **Routes.** `/roadmap/{slug}` shows columns for Ideas, Planned, In progress and Shipped (the last 90
    days, with "All shipped" at `?stage=shipped`). Ideas are sorted by votes, with title, excerpt, votes,
    comment count and the stage date. `/roadmap/{slug}/ideas/{id}` shows the idea, its stage history and
    its published replies, authors by display name and initials only.
  - **Head tags.** Canonical link, Open Graph and JSON-LD `BreadcrumbList`.
  - **Not found.** A private board, an unknown slug and a pending or hidden idea answer the plain `404`
    text of status pages.
  - **Voting and suggesting.** The page embeds `<yuva-chat>` on the board's channel. "Vote" and "Suggest
    an idea" open its Roadmap app at that idea through the element's new `openApp("roadmap", {idea})`.
    Without JavaScript the page reads complete and the buttons are absent.
  - **Analytics.** With an `analytics_property_id` the page sends cookieless page views as status pages
    do.
  - **Own domain.** A board answers at `/` of its verified own domain like a status page (M19 › Own domain ›
    Other kinds, #148); a private board answers `404` there too. A separate origin for every board is not
    planned.
- **Rendering.** Idea bodies and replies are discussion posts: Markdown rendered by M12's goldmark and
  sanitized by bluemonday's UGC policy into `body_html`. There is no second renderer. Titles are 3 to 200
  characters and bodies up to 20,000, as for discussions.
- **Search.** The title search serves the panel's "Link to idea" picker, the widget's "similar ideas"
  while a reader types a suggestion, and MCP. It uses M20's query builder: normalized, 8 words, the last
  one a prefix, parameters only, a 500 ms timeout.
- **Caching.** It reuses the public pages' in-process cache, keyed on the board's `changed_at` with a
  60-second TTL. Votes do not move `changed_at`, so a count may lag a minute. Pages are sent with
  `public, max-age=60, stale-while-revalidate=300, stale-if-error=86400` and an `ETag`. Client JSON is
  `public, max-age=60` without a session and `private, no-store` with one, as discussions are.
- **Abuse limits.**
  - Public pages: 120 a minute per address.
  - Suggestions and replies: M12's 10 posts a minute and 60 an hour per author, and the client write
    limits per address and channel.
  - Moderation queue: at most 200 pending ideas per board, beyond which `429 suggestions_full` is sent
    with `Retry-After`.
  - Votes: 120 an hour per author.
  - Search: 30 a minute per address and channel, queries at most 200 characters.
  - Links: at most 5,000 per idea.
  - `YUVA_ROADMAP_MAX_IDEAS` caps ideas per workspace (unset: 2,000 with `YUVA_SIGNUP=open`, else no
    limit).
  - `YUVA_ROADMAP=off` removes the feature: its routes answer `404` and the client inbox has no
    `roadmap`.
- **Languages.**
  - Ideas are written by readers in any language and are not translated.
  - Stage names and page words come from the server's page word map shared with help pages, complete in
    en, tr and de with English as fallback. `?lang=` and the locale rules are those of status pages.
  - The panel's roadmap screens have their own Lingui catalog, `roadmap`, in every panel language.
  - The widget's Roadmap app uses the `apps` catalog.
- **Panel.**
  - **Navigation.** A Roadmap entry in the rail (`/board`, shortcut `g p`; `/roadmap` is the public pages' path and `g r` is Reports) follows the inbox filter:
    each visible inbox's board, or "Create board" for owners and admins.
  - **The board.** It opens as columns per stage. Cards show votes, linked feedback count and the
    pending mark. Dragging a card or using its stage select moves the stage, after a confirmation that
    names how many linked feedback get which progress. A pending strip lists suggestions waiting for
    approval.
  - **An idea.** It opens beside the board with the discussion thread (the Docs thread pane, reused), its
    stage history, its linked feedback (the incidents' linked list and "Reply to linked" dialog, made
    generic) and, after a move to `shipped`, the "Reply to linked" offer.
  - **On feedback.** The feedback header gets "Link to idea" (search the inbox's board, or "New idea
    from this feedback"). The feedback row shows the idea as a chip.
  - **Settings.** Owners and admins see Settings: public, suggestions, channel, slug, name, description,
    analytics property and delete.
  - **Live updates.** The board refreshes on `discussion.*` and `roadmap.board.updated`.
- **Widget.** Home gets a Roadmap tile when `inbox.roadmap` is present. The Roadmap app is in the apps
  chunk: tabs by stage, ideas by top votes, the idea view with its body, votes and replies, and "Suggest
  an idea" with similar ideas while typing.
  - **Signing in.** An identity-token session votes and suggests at once. An anonymous visitor who votes
    or suggests gets the reader sign-in popup (`client/reader.ts`, as `<yuva-discussions>` uses).
  - **After suggesting.** A pending idea shows "Waiting for approval" to its author.
  - **Reuse.** The thread rendering and composer are the docs ones, imported into the apps chunk; the
    check reports the gzip size.
- **shadcn.** Items on the discussion hooks:
  - `use-yuva-roadmap` wraps `use-yuva-discussions` with `kind=idea`, stage and sort.
  - `yuva-roadmap` shows stage tabs and the list.
  - `yuva-idea` shows the detail with vote, built on `yuva-discussion-thread` and
    `yuva-discussion-composer`.
  - `yuva-idea-suggest` is the suggest form with similar ideas.
  - Each item lists its registry dependencies instead of copying them.
- **MCP.** Yuva's own shapes. Idea titles and bodies and comment text go under `customer_content`,
  because readers write them.
  - `list_roadmap_ideas` (board, stage, `q`, sort) and `get_roadmap_idea` (votes, stage history, linked
    feedback count, and how many linked feedback a move to each stage would change) are read-only with
    `conversations:read` and `roadmap:read`.
  - `link_feedback_to_idea` and `unlink_feedback_from_idea` need `roadmap:write` and
    `conversations:read`, and carry `idempotentHint`.
  - `move_idea_stage` is listed only to principals holding the named `roadmap:stage`. It carries
    `openWorldHint` (customers see the progress) and needs `confirm_linked` equal to the count
    `get_roadmap_idea` showed for that stage; otherwise it answers `409 confirm_mismatch`.
  - Resource `yuva://roadmap/{board_id}`.
  - Prompt `link_feedback_to_ideas`: it lists feedback with category `idea` and `idea=none`, searches
    the board and proposes links. It changes nothing until the person agrees, and it never moves a
    stage.
  - No tool creates, edits or moderates ideas, and none sends "Reply to linked".
- **Elsewhere in this document.** Add `roadmap:read`, `roadmap:write` and `roadmap:stage` to the scopes
  table and the "never implied" sentence. Add `discussion.stage_changed` to the webhook events. In
  "Live chat", replace "a roadmap … have no client API yet" with the Roadmap tile. Add `/roadmap/{slug}`
  to the API surfaces table. In "Discussions (M12)", say that `kind` separates page threads from ideas.
- **As built (steps 1 and 2).** Where the build differs from the text above, this wins.
  - **Triggers.** `discussions.search` and `discussions.vote_count` are kept by triggers, not a
    generated column or the vote's handler: a generated column rewrites the table, and a trigger
    also covers merges, account deletion and cascades. The backfill runs under a share lock on the
    votes after the trigger exists, so the count ends exact.
  - **Contract.** The API has `votes` (the opening post's) and no `vote_count`. `Discussion.page` is
    an empty string for ideas. `GET /v1/discussions` takes `sort=votes` next to `top`.
    `YUVA_ROADMAP_MAX_IDEAS` answers `422 hosted_limit`. `…/{id}/preview` moves to step 3. A move to
    the stage the idea is in changes nothing and returns no `change`. Writes on a discussion that is
    not an idea answer `409 not_an_idea`, reads `404`. `ReaderNotificationKind` gains `stage`.
  - **Stage changes** also hold `linked`, `progress`, `comment_post_id` and `finished_at`. Each
    per-conversation transaction of `roadmap_progress` locks the idea row first (idea, then
    conversation, as linking does) and sets nothing unless its change is still the idea's latest, so
    an older move never overwrites a newer one.
  - **Private boards mail nobody.** Ideas on a private board are not followed by their author, send
    no stage or reply notifications and no digest, even when a contact is the author.
  - **Channels.** A board's channel cannot be deleted while the board exists
    (`409 channel_has_roadmap_board`; the foreign key has no cascade). Every idea is filed on the
    board's channel, and changing the board's channel moves its ideas.
  - **Moving feedback.** Moving a conversation to another inbox unlinks it from its idea, since a
    board belongs to one inbox; its progress stays.
- **As built (step 3).** `/roadmap/{slug}` and `/roadmap/{slug}/ideas/{id}` use the status page shell with
  the roadmap word map (`roadmap_words.go`, en, tr, de, English fallback). The page's language is the
  inbox's, `?lang=` picks another. Routes are always registered and answer the plain `404` when
  `YUVA_ROADMAP=off`. A private board, an unknown slug and a former slug of a private board answer
  the same `404`; an unpublished idea, one of another board and a bad id too. Pages are cached per
  board, view, idea and language on `changed_at` for 60 seconds and rate limited to 120 a minute
  per address. Vote and Suggest are buttons that only the page script reveals. The page sends no
  analytics yet (neither do status or help pages), so `analytics_property_id` is stored only.
  `GET /v1/roadmap-boards/{id}/preview` renders the board or a published idea without the chat
  element, also while private. MCP: `list_roadmap_ideas`, `get_roadmap_idea`, `link_feedback_to_idea`,
  `unlink_feedback_from_idea`, `move_idea_stage` (listed only to a key or grant that names
  `roadmap:stage`, and it answers `409 confirm_mismatch` unless `confirm_linked` equals the
  `would_change` count for that stage), resource `yuva://roadmap/{board_id}`, prompt
  `link_feedback_to_ideas`; `list_feedback` takes `without_idea`. Idea text is under `customer_content`.
- **Not in M21.** Merging duplicate ideas, a `<yuva-roadmap>` web component, the roadmap in the Swift and
  Kotlin SDKs, own domains, and private boards visible to signed-in customers.
- **Order of work.**
  Each step is one agent. Changes to the contract, the migration and permissions go through
  `/code-review` before commit.

  1. **yuva-api, contract.** Add the `roadmap` tag, the board schemas, `kind`, `stage`, `board_id`,
     `vote_count` and `idea` on `Discussion` and `Conversation`, the `kind` and `stage` params, the stage
     and link operations, and the shared `LinkedReply*` schemas (incident operations pointed at them).
     Also `ClientInbox.roadmap`, the client board operation, the three scopes, `discussion.stage_changed`
     and `ConversationCreateDiscussion.kind`. Check: the redocly lint command.
  2. **yuva-api, model and API.**
     - The migration: discussions columns and checks, the board tables, links and stage changes, the
       rename to `linked_replies` with `idea_id`, and the `stage` notification kind.
     - `kind = 'page'` on every page query, and the sqlc queries.
     - The board and idea handlers, stage moves with the `roadmap_progress` job, links, the generic
       linked-reply job with the `status_reply` alias, the reader-token rule, and the client operations.
     - Events, webhooks and limits.
     - Tests for: progress mapping and supersession, the many-to-one link, scope checks with
       `roadmap:stage` never implied, page queries without ideas, and incidents' "Reply to linked"
       unchanged.

     Check: in `api/`, `go generate ./... && go vet ./... && go test ./... && go build ./...` with
     `YUVA_TEST_DATABASE_URL`, then `sdk/go`'s check.
  3. **yuva-api, public page and MCP.** `/roadmap/{slug}` and the idea page with the shared page shell,
     words in en, tr and de, cache, rate limit and the slug helper made generic. Then the MCP tools,
     resource and prompt. Check: as step 2.
  4. **yuva-panel.** The Roadmap rail entry, board columns, idea pane, Settings, "Link to idea" in the
     feedback header and the row chip, the generic linked list and "Reply to linked" dialog shared with
     status, and the `roadmap` catalog. Check: in `web/`,
     `npm run -s format:check && npm run -s lint && npm run -s extract && npx lingui compile --strict && npm run -s build`.
  5. **yuva-widget.** The Roadmap tile and app in the apps chunk, `openApp` on `<yuva-chat>`, the reader
     popup for votes and suggestions, and the four shadcn items. Check: in `sdk/js/`,
     `bun run check && bun run build`; in `sdk/shadcn/`, `bun run check`; `examples/shadcn-next`
     `npx next build`.
  6. **yuva-docs.** A roadmap guide in en, tr and de (board, stages and progress, linking, "Reply to
     linked", the widget and shadcn items, MCP) and README lines. Check: links resolve, and every
     documented endpoint exists in `openapi/`.

### What's new (M22)

A workspace tells its customers what changed: a What's new feed per inbox of short entries labelled New,
Improved or Fixed, shown in the widget's Home with an unread count, on a public page, in an Atom feed,
and, when the team chooses, mailed to its subscribers as a broadcast. Being the support inbox is the
point. A customer sees that the bug they reported is fixed where they would have asked about it. The
mail goes out through the broadcast flow the workspace already trusts, and nothing new sends mail.
Entries are their own table, not help articles. They are dated, labelled and scheduled, and they never
appear in help search. Writing, rendering and translating are M20's.

- **Concepts.** A *changelog* belongs to an inbox (one per inbox at most) and has a public slug, the
  locales it publishes and `published`. An *entry* has labels, an optional image, a state (`draft`,
  `scheduled` or `published`), and one *translation* per locale. A translation has a working draft and,
  once published, a frozen published copy, as help articles have. *Unread* is kept in the visitor's
  browser only.
- **Tables.** Every table carries `workspace_id`, with composite foreign keys `(workspace_id, x_id)`.
  - `changelogs` holds:
    - `inbox_id` (unique per workspace), name, description, and `slug` (unique across the server,
      `a-z0-9-`, 3-48 characters, the reserved words of status pages)
    - `default_locale` and `locales` (1-20)
    - `agents_may_publish` (default false), `analytics_property_id` and `published`
    - `changed_at`, moved by every change the public side shows (the cache key)

    `changelog_slugs` keeps former slugs, which answer `301` through the slug helper status, help and
    roadmap share. Logo and accent colour are the inbox's `branding`.
  - `changelog_entries` holds:
    - the changelog, a per-changelog `number` for URLs, and `labels` (a non-empty subset of `new`,
      `improved`, `fixed`)
    - `image_id` (an `email_images` row of the workspace, `ON DELETE SET NULL`)
    - `state`, `publish_at` (set exactly when `scheduled`, at most 365 days ahead), `published_at` (the
      date shown, the sort key and the unread mark) and `scheduled_by` (member or key)
    - creator and timestamps

    An index on `(workspace_id, changelog_id, published_at DESC, id)` covers published rows.
  - `changelog_entry_translations` is keyed by `(entry, locale)`. Its fields follow
    `help_article_translations`: a draft `title` (3-200), `body` (Markdown, 50,000 characters) and
    `slug`; `version` (writes carry `base_version`, `409 version_conflict`); the `email_images` the draft
    uses; the frozen `published_*` copy with `published_html` rendered once at publish and an excerpt
    (300 characters of plain text); and `has_unpublished_changes`. There is no search vector and no
    revision table: the draft and the published copy are the two versions.
  - `broadcasts.changelog_entry_id` (nullable, `ON DELETE SET NULL`) records which broadcasts were made
    from an entry.
  - `chat_channels.announce_updates` (boolean, default false).
  - `DeleteUnusedEmailImages` also keeps images referred to by an entry or a translation draft, so the
    weekly cleanup never removes an entry's picture.
- **Writing, publishing, scheduling.** Agents with access to the inbox write entries and translations.
  - **Who publishes.** Publishing, scheduling, unpublishing and deleting a published entry are for owners
    and admins, unless the changelog's `agents_may_publish` is on, which lets agents with inbox access do
    them too, exactly as in M20. API keys holding the named `changelog:publish` publish (a release
    pipeline posting its entry). MCP and OAuth tokens never publish or schedule, whatever the setting.
  - **Publishing.** It needs a translation in the default locale. It freezes and renders every
    translation that has a draft, sets `published_at` the first time, moves `changed_at`, signals the
    inbox's contact connections (an `inbox.updated` through the hint status uses) and stores
    `changelog.entry.published`. A translation added or edited later is published on its own with
    `locale`. Republishing keeps the date.
  - **Scheduling.** It sets `publish_at`. A River job at that time, plus a sweep every minute, publishes
    with one conditional update (`scheduled` and `publish_at <= now` to `published`), so a duplicate job
    does nothing. The job checks the scheduler again: a member who lost the right to publish, a revoked
    key or a lost scope puts the entry back to `draft` with `schedule_failed`, which the panel shows.
    Unscheduling returns it to `draft`.
  - **Editor.** As in M20: the default-locale text beside the one being translated, outdated and missing
    translations marked, and no translation model. A save needs `base_version`, and another member's open
    editor gets the realtime-only "changed elsewhere" signal.
- **Mail this to subscribers.** `POST /v1/changelogs/{id}/entries/{entryId}/broadcast` takes `locale`,
  `list_ids` (1-10 lists of the inbox) and an optional `channel_id`. It is for owners, admins and keys
  with `broadcasts:write` (`requireBroadcastWriter`), and only for a published entry
  (`409 not_published`). It builds the draft from existing parts:
  - **The design.** `documentFromTemplate` with `product-update` in the entry's locale (else English).
    Then `title` gets the entry's title, `release` the labels and date in the locale, and `intro` and
    the blocks after it the body. The body is rendered by the M20 renderer and mapped: `h2`/`h3` become
    heading blocks, workspace images become image blocks, and paragraphs, lists and quotes become text
    blocks through the design's own rich-text policy. A table or code block the policy cannot carry is
    replaced by a line linking to the entry. `hero` becomes the entry's image or is removed. `f1`-`f4`
    are removed. The `changelog` button links to the entry's public page.
  - **The broadcast.** The existing `CreateBroadcast` with name "What's new: {title}", subject = title,
    preheader = excerpt, the lists as audience and `changelog_entry_id`. `checkAudience` refuses status
    lists as for any broadcast.
  - **What follows.** The answer is the broadcast, and the panel opens it in the broadcast composer.
    Review, test sends, consent, unsubscribe, approval, pace and sending are M13's, unchanged. Nothing
    is scheduled by this call, and MCP never makes it.
- **Endpoints.** Tag `changelog`.
  - **Members**, under `/v1/changelogs`:
    - Changelogs: list, create, update, delete, and a preview rendered for a sandboxed frame.
    - Entries: list with filters for state `draft`, `scheduled`, `published` or `changed`, and for
      `label` and `locale`; create, update (labels, image) and delete.
    - Translations: `PUT` and `DELETE`, with `base_version`.
    - Actions: `publish` (optional `locale`), `schedule` (`publish_at`), `unschedule`, `unpublish`, and
      the broadcast call above.
    - Editor support: image upload into `email_images` (same rules and size cap, through
      `storeEmailImage`) and the Markdown render for the editor, which is M20's. If M20 ships it under a
      help-center path, step 1 moves it to `POST /v1/markdown/render` for both.
  - **Client**, for `chat` and `app` keys, under `/client/v1/channels/{channel_key}/whats-new`:
    - The changelog (name, URL, locales) with published entries newest first: number, title, excerpt,
      `html`, labels, image URL and size, `published_at`, URL, and the locale actually served (falling
      back to the default). Cursor-paged.
    - `…/whats-new/{entryId}` for one entry.
  - **Client rules.**
    - Browsers need an allowed origin, and nothing creates a contact.
    - Without a published changelog the answer is `404 whats_new_off`.
    - `ClientInbox.whats_new` (`url`, `locales`, `default_locale`, `latest_published_at`) is present only
      with a published changelog that has an entry. The tile shows only then.
    - `ClientChatSettings.announce_updates` carries the channel setting.
    - There are no client writes.
- **Scopes and access.** `changelog:read`, `changelog:write` and `changelog:publish` are API key scopes.
  The first two go to keys made after M22 without named scopes. `changelog:publish` is not implied and
  must be named, and the consent page never offers it, since tokens cannot publish. No existing key gains
  any. "Mail this to subscribers" also needs `broadcasts:write` and `lists:read`. Key inbox limits and
  OAuth narrowing apply as everywhere, and a changelog of another inbox answers `404`.
- **Events and webhooks.** `changelog.entry.published` (also when a schedule fires, and on republish),
  `changelog.entry.unpublished` and `changelog.entry.deleted` are stored events for members who see the
  inbox, and webhook types, with `{changelog, entry}` as data. Draft saves reach `/v1/realtime` only.
- **Public page and feed.** They are rendered by the server with `html/template` and need no JavaScript.
  They use the style, fonts, CSP builder and cache of status and help pages.
  - **Routes.** `/changelog/{slug}` redirects to `/{default_locale}/`. Then:
    - `/{locale}/`: entries newest first, `?label=` and `?before=` for paging
    - `/{locale}/entries/{number}-{slug}`
    - `/{locale}/feed.atom`: the newest 50 published entries in that locale, each with its
      `published_html` as `content type="html"`, its labels as categories and `updated` from its last
      publish, through the Atom writer of status pages made shared
    - `/sitemap.xml`
  - **URLs.** A wrong entry slug answers `301` to the canonical one. There is no `Accept-Language`
    redirect.
  - **Head tags.** `hreflang` alternates for published translations, a canonical link, Open Graph (with
    the entry's image) and JSON-LD `BlogPosting` and `BreadcrumbList`.
  - **Not found.** An unpublished changelog, a draft or scheduled entry and an unknown locale answer the
    plain `404` text of status pages.
  - **Analytics.** With an `analytics_property_id` the pages send cookieless page views as status and
    help pages do.
  - **Own domain.** A changelog answers at `/` of its verified own domain like a status page (M19 › Own
    domain › Other kinds, #148), with its feeds and sitemap there.
- **Rendering.** M20's one goldmark instance for members' content and its bluemonday policy: images only
  from this server's `/i/{token}` of the same workspace, raw HTML dropped, rendered once at publish and
  stored. The entry image is served from `/i/{token}` with its immutable cache.
- **Unread and the badge.**
  - **Where the mark lives.** The widget keeps `whats_new_seen` (the newest `published_at` the visitor
    opened) in its stored state per channel (`readStored`/`writeStored`, localStorage keyed by channel
    key). Anonymous visitors get unread state too, and the server stores nothing.
  - **Counting.** Without a mark, entries older than 30 days count as read. Unread is the number of
    entries newer than the mark, shown up to "9+". The widget fetches the list only when
    `latest_published_at` is newer than the mark.
  - **Clearing.** Opening the app marks everything shown as seen.
  - **Live updates.** A publish reaches widgets with a live connection through `inbox.updated`; a
    visitor without one gets it when the panel opens or the tab becomes visible again, at most once a
    minute. A newer `latest_published_at` than the held list refetches the list past the browser cache.
  - **The badge.** The Home tile always shows the count. The launcher badge shows it only when the
    channel's `announce_updates` is on, and only while there are no unread messages. Messages keep the
    badge, and the update count has its own accessible label ("N updates").
- **Caching.**
  - In-process: the public pages' cache per changelog, view, locale, label and cursor, keyed on
    `changed_at` with a 5-minute TTL and the shared entry cap.
  - Pages and the feed: `public, max-age=60, stale-while-revalidate=300, stale-if-error=86400` with an
    `ETag`.
  - Client JSON: `public, max-age=60`. Because the client side has no sessions or writes, every answer
    is cacheable.
- **Abuse limits.**
  - Public pages and feeds: 120 a minute per address.
  - Client reads: 120 a minute per address and channel, and 2,400 a minute per changelog (`429` with
    `Retry-After`).
  - The editor preview and render: 60 a minute per caller.
  - Image uploads: the `email_images` rules.
  - Scheduling: at most 50 scheduled entries per changelog.
  - "Mail this to subscribers": counts as a broadcast draft, inside M13's hosted limits.
  - `YUVA_CHANGELOG_MAX_ENTRIES` caps entries per workspace (unset: 1,000 with `YUVA_SIGNUP=open`, else
    no limit).
  - `YUVA_WHATS_NEW=off` removes the feature: its routes answer `404` and the client inbox has no
    `whats_new`.
- **Languages.** Translations exist only in the changelog's locales, following M20's pattern. The widget
  picks its locale from `locales` (exact, then base language, else the default), and a page uses the
  locale in its path. Labels and page words come from the server's page word map shared with help pages,
  complete in en, tr and de with English as fallback. The panel's screens have their own Lingui catalog,
  `changelog`, in every panel language. The widget's app uses the `apps` catalog. The broadcast takes
  the template's words in the entry's locale.
- **Panel.**
  - **Navigation.** A What's new entry in the rail (`/updates`, shortcut `g w`) follows the inbox
    filter: each visible inbox's changelog, or "Create" for owners and admins.
  - **The list.** Entries with state, labels, date and translation marks.
  - **The editor.** M20's Markdown editor and side-by-side translation, the label chips, an image picker
    over `email_images`, and Publish, Schedule, Unpublish for those allowed. Others see "A member who
    may publish does it".
  - **Mailing.** "Mail this to subscribers" for owners and admins on a published entry: a dialog picking
    lists (the inbox's lists of kind `list`), channel and locale, which then opens the broadcast. The
    entry lists its broadcasts and their states.
  - **Settings.** Owners and admins see Settings. The chat channel page gets the `announce_updates`
    switch.
  - **Live updates.** Lists refresh on `changelog.*`.
- **Widget.** Home gets a What's new tile with the unread count when `inbox.whats_new` is present. The
  app is in the apps chunk: entries newest first with label chips, date, image and body, and a link to
  the public page. There are no shadcn items or SDK screens in M22. `useyuva`'s client gets
  `whatsNew()` so React sites can build their own.
- **MCP.** Yuva's own shapes. Entry text written by members is not `customer_content`.
  - `list_whats_new_entries` and `get_whats_new_entry` are read-only with `changelog:read`.
  - `draft_whats_new_entry` (title, body, labels, `locale`) and `update_whats_new_draft`
    (`base_version`, `locale` for a translation draft) need `changelog:write`. Their result says a
    member publishes it in Yuva.
  - Resource `yuva://changelog/{id}`, and prompt `draft_release_notes` (from the commit subjects or
    notes the person gives, never fetched by Yuva).
  - No tool publishes, schedules, unpublishes or mails an entry.
- **Elsewhere in this document.** Add `changelog:read`, `changelog:write` and `changelog:publish` to the
  scopes table and the "never implied" sentence. Add the three `changelog.entry.*` types to the webhook
  events. In "Live chat", replace "a changelog … no client API yet" with the What's new tile and add
  `announce_updates` to the chat channel settings. Add `/changelog/{slug}` to the API surfaces table.
  Under "Images", say entries keep their images.
- **Not in M22.** Reactions or comments on entries, per-contact read state on the server, linking entries
  to shipped ideas, What's new screens in the Swift and Kotlin SDKs, own domains, and mailing without the
  broadcast composer.
- **As built (steps 1 and 2).** Where the build differs from the text above, this wins.
  - **Contract.** `renderMarkdown` at `POST /v1/markdown/render` is new and shared (any member, key with
    `help:write` or `changelog:write`); `renderHelpMarkdown` stays because the panel uses it. The
    changelog preview (`previewChangelog`) ships with the public pages in step 3. `ClientChatSettings.announce_updates`
    is optional in the schema (absent means `false`) so SDKs keep working against older servers; the server
    always sends it. `ChatChannel.announce_updates` is required. `YUVA_CHANGELOG_MAX_ENTRIES` answers
    `422 hosted_limit`.
  - **Tables.** Migration `00084_whats_new.sql`. Entries also hold `schedule_failed` (`lost_right`,
    `key_revoked`, `key_scope_lost` or `no_default_translation`) and `schedule_failed_at`; a new schedule
    clears them. Translations keep `image_ids` of the draft and the published copy, as help articles do.
  - **States.** `unpublish` returns a published entry to `draft`, drops every published copy and keeps
    `published_at`, so publishing again keeps the date. `publish` with `locale` needs a published entry
    (`409 not_published`) and publishes that translation only; republishing freezes only translations
    that changed. A published or scheduled entry keeps its default-locale translation
    (`409 default_translation`), and changing the changelog's `default_locale` is refused while one of
    them lacks a translation there (`409 default_locale_missing`). Changing labels or the image of a
    published entry, and deleting a non-draft entry or a published translation, need the right to publish.
    A scheduled entry goes public as it is when its time comes, so changing it, saving or deleting its
    translations also need that right (`403 entry_scheduled`; tokens `403 member_session_required`).
  - **Scheduling.** The job a schedule queues is not unique; a schedule that moves leaves the first job,
    which finds the entry not yet due. The sweep (every minute, across workspaces) queues unique
    `sweep` jobs. Both end in `PublishScheduledEntry`: it locks the changelog and then the entry, returns
    unless the entry is still `scheduled` and due, asks again whether the scheduler may publish (the
    member's role and inbox access, the key's revocation, expiry, `changelog:publish` and inbox limit),
    and publishes with `PublishDueChangelogEntry`, one conditional update.
  - **Events.** The realtime signal `changelog.entry.updated` (`changelog_id`, `entry_id`, `locale`,
    `version`, `member_id`) follows every change to an entry or draft, not only draft saves. Stored
    events and webhooks carry `{changelog: {id, name, url}, entry: {id, number, labels, published_at,
    translations: [{locale, title, url}]}}`; `unpublished` and `deleted` list the translations the entry
    had. Replacing a published translation by `publish` with `locale` sends `published` again.
    Publishing, unpublishing and deleting a published entry send the `inbox.updated` hint through
    `statusHint`.
  - **Mail.** The design is the `product-update` template with `logo` and `f1`-`f4` removed (the logo is
    the template's placeholder), `hero` moved ahead of the text, the footer replaced by a line naming
    `{{ inbox.name }}` in the entry's language (en, tr, de; the template's other words are its own), and
    the body after the title. `{{` in the title, text, link or alt is broken so a member's words are never
    merge fields; at most 150 body blocks, then a link to the entry. The release line uses the inbox's
    time zone. `CreateBroadcast` and the entry call share `insertBroadcast`, and the call makes its design
    in the same transaction, so a refused draft leaves nothing behind. The excerpt (and so the preheader)
    has no space before punctuation.
  - **Images.** `DeleteEmailImage` also refuses an image an entry shows or a draft or published text uses
    (`used_by.changelog_entries`), as for help articles; the weekly cleanup keeps the same images.
  - **Client.** `ClientChangelog.latest_published_at` is absent for a changelog without entries. Client
    answers carry `Cache-Control: public, max-age=60`; there is no in-process cache for them.
- **As built (step 3).** `/changelog/{slug}` uses the status page shell with its own word map
  (`changelog_words.go`, en, tr, de, English fallback). A page shows only the entries that have a published
  translation in the locale of its path; there is no fallback to the default locale on pages (the client API
  has one). `/changelog/{slug}` answers `302` to `/{default_locale}/`; `?label=` filters and `?before=`
  (a cursor) pages, 20 entries a page. Unpublished changelogs, drafts, scheduled entries, locales outside
  `locales` and former slugs of unpublished changelogs answer the same plain `404`; former slugs of published
  ones answer `301`. Pages, the feed and the sitemap share one cache per changelog keyed on `changed_at`,
  5 minutes, with the `Cache-Control` and `ETag` of help pages, and 120 requests a minute per address. The
  Atom writer is `marshalAtom` in `status_public.go`, shared with the status feed, whose entries gain
  `category` elements only when they have labels. The pages do not send page views: no existing public page
  does, so `analytics_property_id` is stored only. `previewChangelog` renders the home page or one entry for a
  sandboxed frame with `noindex`; it shows published copies, or with `draft=true` the drafts of entries in any
  state, and works while the changelog is unpublished.
  - **MCP.** `list_whats_new_entries`, `get_whats_new_entry`, `draft_whats_new_entry`, `update_whats_new_draft`
    (`changelog:read` and `changelog:write`), the resource `yuva://changelog/{id}` and the prompt
    `draft_release_notes`. `changelog_id` may be left out when the caller sees one changelog. The tools call
    the same handlers as the API, so a token cannot publish, schedule or change a scheduled entry
    (`403 member_session_required`). Entry text is not wrapped in `customer_content`, as planned.
- **Order of work.**
  M22 starts after M20's API step, whose renderer, translation and editor code it reuses. Changes to the
  contract, the migration and permissions go through `/code-review` before commit.

  1. **yuva-api, contract.** Add the `changelog` tag, the changelog, entry and translation schemas and
     operations, the broadcast-from-entry operation, `ClientInbox.whats_new`, the client operations,
     `announce_updates` on chat channel settings (member and client), `Broadcast.changelog_entry_id`, the
     three scopes and the three event types. Move the Markdown render to `/v1/markdown/render` if needed.
     Check: the redocly lint command.
  2. **yuva-api, model, API and mail.**
     - The migration and queries, including the image cleanup that keeps entry images.
     - The handlers with the M20 publish rules, the schedule job and sweep, and the client operations with
       the cache.
     - The Markdown-to-design mapping on `documentFromTemplate`, and the broadcast call through
       `CreateBroadcast`.
     - Events, webhooks, limits and the `inbox.updated` hint.
     - Tests for: the scheduled publish being idempotent and checking the scheduler again, tokens never
       publishing, the mapping keeping only policy-allowed HTML and the design passing `design.Validate`,
       and the image cleanup.

     Check: in `api/`, `go generate ./... && go vet ./... && go test ./... && go build ./...` with
     `YUVA_TEST_DATABASE_URL`, then `sdk/go`'s check. The generated broadcast is also rendered to Mailpit
     and checked with its html-check.
  3. **yuva-api, public pages, feed and MCP.** The pages and Atom feed with the shared shell, word map,
     cache and rate limit, and the Atom writer shared with status. Then the MCP tools, resource and prompt.
     Check: as step 2.
  4. **yuva-panel.** The What's new rail entry, list, editor (M20's components), Settings, the "Mail this
     to subscribers" dialog into the broadcast composer, the `announce_updates` switch, and the `changelog`
     catalog. Check: in `web/`,
     `npm run -s format:check && npm run -s lint && npm run -s extract && npx lingui compile --strict && npm run -s build`.
  5. **yuva-widget.** The What's new tile and app in the apps chunk, unread state in the stored channel
     state, the launcher badge behind `announce_updates`, and `whatsNew()` on the client. Check: in
     `sdk/js/`, `bun run check && bun run build`.
  6. **yuva-docs.** A What's new guide in en, tr and de (writing and translating, scheduling, the widget
     and badge, the feed, mailing an entry through broadcasts, MCP) and README lines. Check: links resolve,
     and every documented endpoint exists in `openapi/`.

### Single sign-on (M33)

Members of a workspace can sign in through the workspace's identity provider, with OpenID Connect
(#160) or SAML 2.0 (#161). Both use the same settings row, domain claims, rules, membership, sessions
and audit; `sso_settings.protocol` (`oidc` or `saml`) says which one, so a workspace has one provider.

- **Settings** (migration 00099): one `sso_settings` row per workspace (created on first change)
  holds the provider (`protocol`, the issuer, client id, the client secret sealed under
  `YUVA_MASTER_KEY` with the workspace and row id as context and never returned, extra scopes, the
  groups claim and allowed groups) and the rules (`sso_required`, `jit`, `jit_role`). Owners only,
  with a member session (keys and OAuth tokens get `403 member_session_required`): `GET`/`PATCH
  /v1/sso`, `PUT`/`DELETE /v1/sso/oidc`, `POST /v1/sso/domains`, `POST
  /v1/sso/domains/{id}/verify`, `DELETE /v1/sso/domains/{id}`. Saving the provider reads
  `<issuer>/.well-known/openid-configuration` (its `issuer` must match exactly; Entra's shared
  `common` issuer is therefore refused) and answers `400 sso_discovery_failed` otherwise. Removing
  it turns `sso_required` off and keeps the domains. Not offered through MCP (credentials and access).
- **Domain claims**: `sso_domains` rows, `pending` until `POST …/verify` finds the TXT record
  `_yuva-sso.<domain>` holding the row's `yuva-verify-…` token (the status-page resolver, a lookup on
  demand, no sweep). A verified domain is unique across the server (partial unique index): a domain
  another workspace verified answers `409 domain_taken` when added or verified; pending claims may
  overlap. Only the exact domain is claimed, not subdomains. At most 50 per workspace. A verified
  claim stays until removed; removing the last verified one turns `sso_required` off.
- **Sign-in**: the panel asks for the e-mail first and posts it to `POST /v1/auth/sso` (30 a minute per
  client IP address). When the address's domain is verified by a workspace with a provider, the answer carries
  `authorization_url` (code flow, PKCE S256, `state`, `nonce`, `login_hint`) and `required`, and sets
  `yuva_sso` (the state, HttpOnly, SameSite=Lax, path `/auth/sso`, 10 minutes), binding the sign-in
  to the browser; else `sso: false` and the page offers a code or a passkey. An `sso_logins` row
  (state hash, nonce, verifier, `return_to`, 10 minutes) is taken once by `GET /auth/sso/callback`,
  which needs the cookie to equal `state`, exchanges the code with the client secret and verifier,
  and verifies the `id_token` with go-oidc: signature against the provider's JWKS (kept in memory
  with the discovery document for an hour, fetched again when a token names an unknown key),
  `iss`, `aud`, `exp`, then the nonce. The address must be verified (`email_verified: true`, or
  Entra's `xms_edov: true`, which Entra sends instead) and in a domain this workspace verified; with
  allowed groups, the groups claim must name one. Success redirects to `return_to` (a path on the
  panel, else `/`) with the session cookie; a refusal to `/sign-in?sso_error=<code>`. Every session
  records how it was made (`sessions.method` `code`, `passkey` or `sso`, and for `sso` the
  workspace and `sso_settings` row it came through).
- **Membership**: pending invites for the address are accepted as with a code. A person who is not a
  member joins with `jit_role` (`agent` by default, `admin` allowed, never `owner`) when `jit` is on
  (off by default), creating the person if needed: a `members` row only, inbox access follows the
  usual grants. A person who already exists signs in even when not a member of the claiming
  workspace (the session is the person's); with no person, no invite and `jit` off the answer is
  `not_member`. Sessions stay person-level: a person's e-mail domain decides the one provider they
  use, whatever workspaces they belong to.
- **`sso_required`** is enforced on workspace access, not on sign-in. It needs a provider and a
  verified domain (`409 sso_not_ready`). Every request that acts in the workspace as a member with a
  session (`/v1`, `/v1/realtime`, approving an OAuth request for it) is refused with `403
  sso_required` and `sso_email` (to send to `POST /v1/auth/sso`) when the person's address is in one
  of the workspace's verified domains and the session was not made through this workspace's
  provider; the check runs on every request, so sessions that exist when the rule is turned on are
  refused at once and realtime streams at their next re-check. Signing in with a code or a passkey is
  never refused for single sign-on: the person still reaches their other workspaces and the reader
  account (discussions on any server page). Owners are exempt whatever their session, so a broken
  provider never locks the workspace out. OAuth grants record whether they were approved with such a
  session (`oauth_grants.sso_settings_id`); a covered member's grant approved otherwise (for example
  before the rule was on) is refused the same way until it is approved again through the provider.
  API keys are not member sessions and are not affected. Removing the provider or the last verified
  domain turns the rule off.
- **SAML 2.0** (migration 00105, #161): service-provider-initiated only, with crewjam/saml and
  goxmldsig. `PUT /v1/sso/saml` takes the identity provider's metadata by URL (fetched through the
  outbound checks below) or pasted, refuses any DTD or other directive, and stores its entity id, the
  HTTP-Redirect single sign-on URL and the signing certificates (DER, at most 10) in the `saml_*`
  columns, clearing the OIDC ones (and `PUT /v1/sso/oidc` clears the SAML ones); `DELETE
  /v1/sso/saml` and `DELETE /v1/sso/oidc` remove only their own protocol. Metadata from a URL is
  fetched again when a sign-in starts and the copy is over an hour old (a certificate change is
  `sso.provider_updated` by `system`, `via: metadata_refresh`); a failed fetch keeps the stored copy
  and is recorded in `saml_metadata_tried_at`, so the next try waits 15 minutes and sign-ins do not
  wait on a host that is down. Certificates past their `NotAfter` are left out (named in `warnings`
  of the `PUT` answer); metadata without a certificate valid now is refused.
  Yuva's entity id and audience per workspace is `<public URL>/auth/saml/metadata/<workspace id>`,
  which serves the SP metadata (ACS with HTTP-POST only, NameID format `emailAddress`, no keys: Yuva
  neither signs requests nor takes encrypted assertions); the ACS is `POST /auth/saml/acs` for every
  workspace. `POST /v1/auth/sso` answers the `AuthnRequest` redirect URL and sets `yuva_saml` (the
  request id, HttpOnly, Secure, SameSite=None, path `/auth/saml/acs`, 10 minutes: the identity
  provider's form POST is cross-site, so a Lax cookie never arrives). Browsers keep a Secure cookie
  on plain http only for localhost, so `PUT /v1/sso/saml` answers `400 saml_requires_https` when
  `YUVA_PUBLIC_URL` is http on any other host. The `sso_logins` row is keyed by the request
  id's hash (nonce and verifier empty). The ACS reads at most 256 KiB, refuses directives, needs a
  non-empty `InResponseTo` equal to the cookie (unsolicited and identity-provider-initiated
  responses fail as `expired`, unaudited, since nothing ties them to a workspace yet), takes the
  login row once, then requires a `Response` with `Destination` equal to the ACS, exactly one plain
  `Assertion` (no `EncryptedAssertion`), bearer subject confirmations with data and an audience
  restriction, before crewjam verifies the enveloped signature of the Response or the Assertion
  against the stored certificates only (a signature must reference the element that is consumed),
  issuer, `InResponseTo` on the response and every confirmation, `Recipient`, audience, validity
  (`MaxClockSkew` 180 s, `MaxIssueDelay` 90 s). The assertion id is then recorded in
  `saml_assertions` (per workspace, kept until the assertion could no longer be accepted, at least
  10 minutes); a second use is refused. The address is the NameID or the configured
  `saml_email_attribute` (by `Name` or `FriendlyName`), groups the attribute named by
  `groups_claim`; SAML has no `email_verified`, the domain claim is the guard. From there the sign-in
  is the OIDC one (claimed domain, groups, invites, JIT, `sso` session naming the settings row,
  audit with `protocol: saml`). Refusals of a verified workspace's response are `invalid_token` with
  the check in `detail`; a non-success status is `denied`.
- **Outbound calls** to the issuer (discovery, keys, token) go through the webhook address checks
  (`webhook.Dial`): never link-local or metadata addresses, private ones only with
  `YUVA_WEBHOOK_ALLOW_PRIVATE`, no redirects, 10-second timeout, answers read up to 1 MiB.
- **Audit**: `sso.provider_created`, `sso.provider_updated`, `sso.provider_deleted`,
  `sso.settings_changed`, `sso_domain.added`, `sso_domain.verified`, `sso_domain.deleted`,
  `sso.signed_in` and `sso.sign_in_refused` (reasons `failed`, `invalid_token`, `email_unverified`,
  `domain_not_claimed`, `group_not_allowed`, `not_member`), actor the member when the address is one
  in the workspace, else `system`; `member.joined` with `via: sso` for JIT. Requests refused with
  `sso_required` are not recorded (they repeat on every call).
### Issue trackers (M32)

A workspace connects GitHub and Linear so that what customers ask for becomes an issue where the team
plans its work, and the issue's state tells the customer where it stands, the way a roadmap stage does.

- **Connecting.** Owners and admins (and API keys without an inbox limit holding `integrations:manage`)
  connect with a token a self-hoster gets without registering an app: a GitHub fine-grained personal
  access token limited to the chosen repositories (Issues: read and write), or a Linear personal API
  key. GitHub Apps and Linear OAuth apps are not supported. The token is checked with the tracker
  (`GET /user`, Linear's `organization`) before it is stored, sealed under `YUVA_MASTER_KEY` with the
  integration as context, never answered and never logged; `account` keeps the name the tracker gave.
  A GitHub Enterprise Server is reached through its `api_url` (`https`, refused on private, loopback and
  link-local addresses when saved, as webhook URLs are). Every outbound call goes through the webhook
  client's address checks, without redirects, to `api.github.com`, the configured Enterprise API or
  `api.linear.app` only. A change or a new webhook secret locks the integration's row in its
  transaction and writes only what it was sent, so a token check that takes a while never undoes a
  concurrent rotation. At most 20 integrations per workspace. Connect, change, disconnect and a new
  webhook secret write audit entries (`issue_integration.*`, never a token or secret).
- **Tables.** `issue_integrations` (provider, name, account, `api_url`, `default_target`, the sealed
  `token` and `webhook_secret`, last error), `tracked_issues` (one row per issue of an integration that
  is linked: external id, key, URL, title, the tracker's state, the mapped `progress`, `synced_at`,
  `next_sync_at`, `sync_failures`, `state_changed_at`) and `issue_links` (an issue to exactly one feedback conversation or idea, who
  linked it, and `applied_progress`). All carry `workspace_id` with composite foreign keys; links
  cascade with their conversation, idea, issue and integration, and an issue without links is deleted
  with its last link. Migration 00099.
- **Mapping.** GitHub: open is `planned`; closed as completed (or without a reason) `done`; closed as
  not planned or duplicate `wont_do`. GitHub issues have no in-progress state, so none is inferred
  from labels. Linear's state type: `triage`, `backlog` and `unstarted` are `planned`, `started`
  `in_progress`, `completed` `done`, `canceled` (and `duplicate`) `wont_do`. An idea only shows its
  issues and their state: an issue never moves an idea's stage, which notifies followers and stays a
  deliberate action of the team, and never touches the idea's linked feedback.
- **Linking.** `POST /v1/conversations/{id}/issues` (feedback only, `409 not_feedback`) and
  `POST /v1/discussions/{id}/issues` (ideas only) take `url` to link an existing issue (a GitHub URL
  or `owner/repo#12`, a Linear URL or `ENG-123`) or `title`, `body` and `target` (`owner/repo`, a
  Linear team key or id, or the integration's `default_target`) to create one. A created issue's body
  is what the member wrote, then a line with the conversation's or idea's address in the panel;
  nothing of the conversation and no customer's name or address goes to the tracker unless the member
  writes it. A target takes at most 10 issues, an issue at most 1,000 links. The link's transaction
  stores the issue, writes the link and `issue_link.created`, and on a feedback applies the issue's
  progress at once.
  Linking the same issue again answers `200` with the existing link. Unlinking keeps the progress.
- **Applying the state.** A feedback link is applied as the member, key or grant that made it,
  through the conversation update a member makes: the `feedback_progress` event carries
  `issue_link_id`, the contact sees the line, `conversation.updated` goes to webhooks. A key or grant
  without `issues:write`, and a link whose maker was removed or revoked or lost access to the inbox,
  records the state and sets nothing.
  `applied_progress` is set either way, so a progress set by hand holds until the issue's mapped
  progress changes again.
- **Webhooks in.** `POST /ingress/issues/{integrationId}` finds the integration by its id before the
  workspace is known, then checks GitHub's `X-Hub-Signature-256` or Linear's `Linear-Signature` (and a
  `webhookTimestamp` within a minute) with the integration's secret: Yuva makes one (shown once,
  rotatable) or stores Linear's signing secret. A missing or wrong signature answers
  `401 invalid_signature` and changes nothing. The body is only a hint: for GitHub `issues` events that
  may change state and Linear `Issue` events on linked issues it queues an `issue_sync` job, which
  reads the issue again with the token. The job stores the answer only when it was read after the
  stored one (`synced_at`), so replayed or out-of-order deliveries change nothing, sends
  `issue_link.updated` when the state, title or address changed, and applies the new progress to each
  link whose `applied_progress` differs, each in its own transaction. 1 MiB and 600 calls a minute per
  integration, counted after the lookup.
- **Reconciliation.** Every issue has `next_sync_at`: an hour after a successful read; after a failed
  one (the tracker unreachable, rate limited, refusing the token, or no longer showing the issue)
  15 minutes doubled for each failure in a row, up to a day, with `sync_failures` counting them. The
  `issue_sweep` job every 15 minutes takes up to 500 due linked issues of every workspace, oldest
  `next_sync_at` first and at most 50 of one integration, moves each `next_sync_at` 15 minutes ahead
  while its `issue_sync` job is queued, and queues the jobs. A tracker that keeps failing thus backs
  off and never takes another integration's turn. The last failure is kept on the integration as
  `last_error`.
- **Scopes.** `integrations:read` (integrations), `integrations:manage` (connect, change,
  disconnect, rotate; refused for keys limited to inboxes, never offered to agents' tokens) and
  `issues:write` (create, link, unlink, with `conversations:read`). Listing links needs
  `conversations:read`. None is added to keys made before them.
- **Events.** `issue_link.created`, `issue_link.updated` (with `previous_progress` when it moved) and
  `issue_link.deleted` (also for each link when an integration is disconnected) are stored events for
  callers who see the inbox and webhook types, with the `IssueLink` as data. `Conversation.issues` and,
  when one idea is read, `Discussion.issues` list the links.
- **MCP.** `list_issue_integrations` (read-only), `link_issue` (`idempotentHint`, open-world: the
  customer sees the progress) and `create_issue` (open-world), both on feedback with `issues:write` and
  `conversations:read`. `create_issue` tells the assistant to write the body itself and leave out
  personal data unless asked.
- **Not in M32.** GitHub Apps and Linear OAuth, issue comments synced into the thread, pull requests,
  GitHub Projects status fields, and changing an issue from Yuva after it was created.

### Team mail (M34–M38)

Some teams use Yuva as their mail client rather than for support: shared addresses such as sales@ and
people's own mailboxes. They get the same conversation model in a place and with words of their own, as
Feedback did in M18. Planned after 0.1.0; issues in `backlog.md`. Decisions:

- **Purpose, not a new model.** An inbox is `support` or `mail`. A `mail` inbox has only e-mail
  channels, and the support features stay off for it: ratings, SLA policies, automatic assignment,
  greetings, status, help, roadmap and What's new, support stats and reports. `closed` reads Archived.
  Starred is per member, Trash is a state emptied after 30 days.
- **Personal mailboxes are private.** A personal mailbox (`owner_member_id`) is read only by its owner
  and the members they delegate to. Owners and admins see that it exists, its address, size and usage,
  never its content; API keys and webhooks never reach it, and assistants only through the owner's own
  OAuth grant. This is the one place where an owner does not see every inbox, so that people can
  move their own mail into Yuva. Retention, contact export and deletion, and workspace deletion still
  cover it, and every delegation and hand-over is in the audit log.
- **Leaving.** When a member with a personal mailbox is removed, an owner or admin deletes the mailbox
  or hands it to a named member; nobody reads it before that choice.
- **Sending as a person.** A mail goes from the channel address with the member's name, or from the
  member's own address once that address is verified by a code and its domain is a verified sending
  domain of the channel. Delegates send "on behalf of".
- **Yuva is the mailbox.** A domain's mail is routed to Yuva (the edge Worker or any MTA) and history
  comes in through Moving to Yuva (M14). Two-way IMAP sync with a mailbox that stays at Gmail or
  Outlook is not planned: it would write to the source, which M14's read-only rule forbids.

### Storage

Attachments and raw e-mails in S3-compatible storage (R2, S3, MinIO) or on local disk for a
single-node install: `YUVA_STORAGE=local|s3`, with `YUVA_STORAGE_DIR` for local disk and
`YUVA_S3_ENDPOINT`, `YUVA_S3_REGION`, `YUVA_S3_BUCKET`, `YUVA_S3_ACCESS_KEY_ID`,
`YUVA_S3_SECRET_ACCESS_KEY`, `YUVA_S3_PATH_STYLE` for S3. Objects are never public (the exception is e-mail images, served at `/i/{token}`, see Broadcasts);
members and API
keys download them through `/v1/attachments/{id}`, which checks inbox access. The size limit (`YUVA_ATTACHMENT_MAX_BYTES`, 25 MiB by default) and the content-type allowlist
(`YUVA_ATTACHMENT_TYPES`, comma-separated, `image/*` style wildcards allowed) are the server's; a
channel can narrow both (`channels.attachment_max_bytes`, `channels.attachment_types`, set with
`attachments` on `POST /v1/inboxes/{id}/channels` and `PATCH /v1/channels/{id}`), never widen them:
a size above the server's or a type that allows nothing the server allows is refused with `400
validation_failed` naming the field (a channel's `image/*` is accepted under a server that lists
`image/png` and allows just that), and `null` makes the channel follow the server again. The
effective limits are computed on every use, never stored: the smaller size and the types both lists
allow (a channel's `image/*` under a server's `image/png` allows `image/png`), so a server limit
lowered after a channel was narrowed still wins. Every upload is held to the effective limits of its
channel: members' and API keys' files on a conversation (the conversation's channel; the server's
limits when it has none; no file at all when its channel row cannot be read), contacts' files on
`/client/v1` conversations and messages and feedback screenshots (the session's channel), and
inbound e-mail (the receiving channel), where a part over the limit is skipped like one over the
server's: the message is kept and the part stays in the raw e-mail. `/client/v1` returns the
effective limits as `attachments` on the inbox's public settings so the widget and the SDKs refuse a
file before uploading it. Imports through Moving to Yuva keep the server's limits by design, since
they belong to no channel. (#136, migration 00095)

## Privacy and data

- Retention per workspace: `workspaces.retention_days`, set by an owner or an API key with
  `workspace:manage` (`PATCH /v1/workspace`). Admins can create such a key; that adds no destructive
  power they lack, since they can already delete inboxes with their conversations. `null` keeps
  everything. An hourly River job deletes closed conversations whose last change and
  last activity are older than that, with their messages, attachments and stored files, and
  removes raw e-mails older than that from storage (the parsed message stays).
- Deleting a conversation (#178): `DELETE /v1/conversations/{id}` moves it to the trash
  (`conversations.trashed_at`, `trashed_by_member_id` or `trashed_by_api_key_id`);
  `POST /v1/conversations/{id}/restore` brings it back with its status, assignee and labels
  until `deletes_at`, 30 days after `trashed_at` (afterwards `409 trash_expired`). `trashed_at`,
  the restore window and the purge use the database's clock, as rate limits do. Owners,
  admins, and keys and OAuth tokens with `conversations:delete` (never implied, offered only to
  owners' and admins' tokens, allowed for inbox-limited keys in their inboxes); agents get
  `403 forbidden` in every inbox, so a mistake cannot be hidden. `/v1/conversations/bulk` takes
  `trashed: true|false` alone for up to 100, per-item results as for the other bulk changes;
  a `trashed` change needs `conversations:delete` only, every other bulk change
  `conversations:write`.
  Trashing or restoring twice answers the conversation unchanged, without an event or entry.
- A trashed conversation is read-only and left out of every list, count, search, stat, Home
  figure, contact summary, docs and analytics support figure, linked replies and roadmap link
  counts, drafts, mentions, rating links, chat continuity mail, MCP and all of `/client/v1`.
  The Trash view is `GET /v1/conversations?trashed=true` (owners, admins, `conversations:delete`);
  those callers can also `GET` the conversation, its messages, attachments, e-mail and raw
  source; everyone else gets `404`.
- A new message from the contact never joins a trashed conversation: e-mail threading
  (In-Reply-To, References, the conversation token, broadcast and status notice replies, the
  per-sender hourly cap) skips it and starts a new conversation without `related_conversation_id`;
  a widget or SDK message posted to its id starts a new conversation with the trashed one's
  kind, subject, page, help article and feedback context (not the feedback text)
  (`conversation_id` in the answer, `conversation.created` on the stream). Same `client_id`,
  same message: a `client_id` already used in the trashed conversation answers that message
  (`200`, nothing new is started); one used in the replacement answers it there; starting a
  conversation again with a `client_id` from a trashed one answers `404`. The per-sender limit
  of 20 new e-mail conversations an hour still counts trashed ones; over it, with only trashed
  ones to join, the mail is refused `429 rate_limited`.
- The hourly retention job (before the per-workspace retention) deletes conversations trashed
  30 days ago, 200 per transaction per workspace through the same batch delete as retention and
  workspace deletion (workspace row locked first, then the trash rows `FOR UPDATE`, so a restore
  either wins or finds the conversation gone), with messages, attachments, raw e-mails, stored
  files and remaining events, for at most 30 seconds a run; an error is logged and the job goes
  on, as for the audit log sweep. The per-workspace retention of closed conversations skips
  trashed ones: the trash purge owns them.
- Events and webhooks: `conversation.trashed` and `conversation.restored` carry the conversation
  (webhooks: with the contact); `conversation.deleted` carries only `{id, inbox_id, contact_id}`.
  The contact's realtime stream sends `conversation.removed` (`{id}`) when its conversation is
  trashed (or deleted, if the stream still knew it) and announces a restored one as
  `conversation.created`. Trashing deletes the conversation's earlier events (all but its
  `conversation.trashed` and `conversation.restored`), so `/v1/events`, member and contact
  replays never show it again; restore does not need them, since `conversation.restored` carries
  the conversation. A webhook of a deleted event not yet fanned out is dropped. The purge deletes
  the rest. Index `events (workspace_id, conversation_id)` makes this cheap.
- Audit log: `conversation.trashed`, `conversation.restored` (the member, key or grant) and
  `conversation.deleted` (actor kind `system`, no id or name), target type `conversation`, details
  `inbox_id` only, never the subject or content.
- MCP has no delete or restore tool (deletes are left out on purpose, see MCP coverage);
  `get_conversation` and the conversation resource answer "no such conversation" for a trashed
  one even to an owner's token with `conversations:delete`.
- Schema: migration 00094 (`trashed_at`, `trashed_by_member_id`, `trashed_by_api_key_id`, the
  partial index `(workspace_id, trashed_at)`, `events_conversation_idx`, `member_mentions`
  without trashed conversations, audit actor kind `system`).
- Redacting one message (#179): `POST /v1/messages/{id}/redact` removes a message's or note's
  content and keeps the line in the thread. One transaction empties `body`, `html`, `help_cards`
  and `mentions`, sets `redacted_at` and `redacted_by_member_id` or `redacted_by_api_key_id`
  (migration 00096 adds them with `NOT VALID` constraints, `ON DELETE SET NULL` like the trash;
  00097, `NO TRANSACTION`, validates them without holding a lock on `messages`), deletes the
  attachment rows and, for e-mail, clears `message_emails` subject, full text, full HTML, headers
  and `raw_key` (Message-ID, In-Reply-To, References and addresses stay, so threading and import
  dedupe keep working). When the message is a feedback's first message, `feedback.text` becomes
  empty and `attachment_count` 0; when it is the inbound e-mail whose subject became the
  conversation's subject, the conversation subject becomes empty (later replies are titled with
  the channel's name) and the e-mail subjects in the conversation that repeat it are cleared;
  either emits `conversation.updated`. Raw sources of later e-mails keep their `Subject:` header.
  Stored copies are rewritten in the same transaction by one JSON walk: the conversation's events
  (locked first), webhook deliveries and answers kept for `Idempotency-Key` that hold the message,
  its preview, the conversation's subject or feedback text, or an e-mail summary repeating the
  subject. Webhook fan-out reads its event under a shared lock, so a fan-out that read the event
  before the redaction commits its deliveries before they are rewritten; a delivery attempt and a
  stored idempotent answer are also checked against redacted messages when they are sent or
  stored, under a shared lock on the messages that waits for a redaction in progress. The
  contact's later inbound e-mails lose their quoted history and raw source only when the redacted
  message is an e-mail we sent, the later e-mail answers it (its Message-ID in In-Reply-To or
  References) and quotes all of its text (compared case-, whitespace- and `>`-insensitively in
  the full text or HTML); partial quotes keep their history. A queued e-mail is not sent (its
  delivery becomes `failed`, "removed before it was sent"); sending reads its messages under row
  locks, so it waits for a redaction in progress and leaves redacted ones out of a batch while the
  others go; an e-mail already on its way when the redaction comes is recorded as `sent`. A River
  job `storage_delete` deletes the files after the commit, retried on failure. Pushes the message
  caused are deleted from the job table after the commit (one being sent at that moment cannot be
  stopped); `notify`, the e-mail digest and chat continuity skip redacted messages. Search forgets
  it at once (the vector is generated from `body`); list previews keep the line with
  `redacted: true` and empty text, the Home waiting preview is empty. Who: owners and admins any
  message they can see, other members their own (`author_member_id`, also through an OAuth
  token); API keys and OAuth tokens need `messages:redact` (never implied, not added to older keys,
  offered to agents' tokens). Drafts and timeline events answer `409 not_redactable`, a trashed
  conversation `404`; redacting again answers the message unchanged without an event or audit
  row. Emits `message.redacted` (realtime, `/v1/events`, webhooks with the `include_notes` rule for
  notes, the `/client/v1` stream for messages); the audit log records `message.redacted` on target
  type `message` with `conversation_id`, `inbox_id`, `kind` and `files`, never content. A sent
  e-mail cannot be taken back from the recipient's mailbox, and notification e-mails already sent
  to members keep what they quoted.
  Notes are redacted the same way; they never reach `/client/v1`, so no client frame, and their
  mentions are cleared, so they leave members' Mentions. A chat or app conversation's subject is not
  touched: it is the conversation's, not a message's.
- Audit log (#135): `audit_log` (migration 00089) records who changed access, credentials and data
  of a workspace, in the same transaction as the change: a failed change leaves no row, and a row is
  kept small enough never to fail one (details under 1500 bytes of JSON; longer lists become
  `<name>_count` with `truncated: true`). Each row has `workspace_id`, the actor (`member`, `api_key`,
  `oauth_grant` for a member through a connected app or MCP, with the member and the app's name in
  `via`, or `operator` for the command line), the actor's display name at the time, the action
  (`<target type>.<verb>`), the target's type and id, small details and the time; no foreign keys to
  actors or targets, so an entry outlives what it names and goes with the workspace. Actions:
  `member.joined`, `member.role_changed`, `member.removed`, `member.inbox_access_granted`,
  `member.inbox_access_revoked`, `invite.created`, `invite.deleted`, `api_key.created`,
  `api_key.revoked`, `oauth_grant.created`, `oauth_grant.revoked`, `webhook.created`,
  `webhook.updated`, `webhook.deleted`, `webhook.secret_rotated`, `channel.credentials_changed`,
  `channel.key_rotated`, `channel.forwarding_address_rotated`, `channel.dkim_key_created`,
  `channel.dkim_key_deleted`, `inbox.identity_secret_rotated`, `inbox.deleted`, `contact.deleted`,
  `contact.consent_exported` and `list.consent_exported` (written when the export starts streaming,
  `started: true`), `analytics_export.created`, `workspace.retention_changed`,
  `workspace.broadcast_approval_requested`, and the operator's `workspace.broadcasts_approved`,
  `workspace.broadcasts_denied`, `workspace.broadcasts_suspended` and
  `workspace.broadcast_limit_changed`; `conversation.trashed`, `conversation.restored`,
  `conversation.deleted` (actor `system`) and `message.redacted` (target type `message`); the
  single sign-on actions (targets `sso` and `sso_domain`, see Single sign-on (M33)). Revocations and deletes write their entry only when the row
  changed, so repeated or concurrent calls record one; a password sent again unchanged records
  nothing. No row holds message content, a secret, a password, a webhook URL (only its host), or a
  contact's name or address. A workspace's own deletion is not in its log, which goes with it: it is
  an operator notice `workspace_deleted` in the operator digest. MCP tools write through the same
  handlers, so their rows name the OAuth grant. `GET /v1/audit-log` lists entries newest first for
  owners and for keys and tokens with `audit:read`, filtered by `actor_id` (a member's id also
  matches what they did through connected apps), `actor_kind`, `action`, `since` and `until`, with
  cursor pagination; Settings › Audit log shows it to owners. `audit:read` on an API key holds only
  while the member who made the key is still an owner, checked on every read; keys made with
  `yuva api-key create` keep it. Entries are kept `YUVA_AUDIT_DAYS` (365). `migration_audit` stays as
  it is.
- Imported history (see "Moving to Yuva") is the workspace's data like any conversation: contact
  export and deletion, retention (counted from the import) and workspace deletion cover it. Source
  credentials are deleted when the migration ends.
- Workspace deletion: an owner deletes the workspace in the panel or with `DELETE /v1/workspace`
  (the body repeats its name), an operator with `yuva workspace delete --workspace <id|name> --yes`.
  One transaction sets `workspaces.deleted_at`, removes the workspace's webhook endpoints and the
  push subscriptions of members who have no other workspace, and queues a River job; the request
  answers `202`. From then on every lookup a request starts from ignores the workspace (member
  sessions, API keys, contact sessions, chat and app keys, CORS origins, e-mail recipients, SES
  reports, invites), so it is unusable at once; open realtime connections close at their next
  heartbeat check, and jobs queued for it earlier (e-mail, chat continuity, notifications, push)
  do nothing. The job deletes the conversations 200 at a time with their messages, attachments
  and stored files, then the contacts, then the workspace row, which takes everything else with
  it. Each run works for about 30 seconds, records its progress in the job's output and the log,
  and snoozes the job until the next run, so a large workspace never hits a timeout; the hourly
  retention job queues it again for a deleted workspace whose job was lost. People keep their
  account: someone left without a workspace can still sign in and sees that they belong to none.
- Account deletion: a person deletes their account in the panel or with `DELETE /v1/me` (the body
  repeats their e-mail address), an operator with `yuva person delete --email <address> --yes`.
  It is refused (`409 last_owner`) while the person is the only owner of a workspace. Otherwise
  the person goes with their memberships, sessions, passkeys, push subscriptions and sign-in
  codes; messages they wrote stay without an author, shown as a deleted member. As a reader they
  also lose their reader tokens, notifications, votes and follows; their discussion posts stay
  without an author, shown as a deleted reader; in each workspace the contact linked to them is
  deleted when it has no conversations and no external ids (it existed only for their posts), else
  only unlinked, so the workspace keeps its own record of a customer.
- Contact deletion and export through the API, for GDPR and KVKK requests. A host backend deletes
  a user who deleted their account with `DELETE /v1/contacts/by-external-id?inbox_id=&external_id=`
  (an API key or an owner or admin): the contact goes with all their conversations, messages,
  attachments, sessions and addresses in every inbox, the stored files are removed, and
  `contact.deleted` tells the other endpoints which external ids are gone. Their addresses are
  removed from other conversations' participants and from the `to`, `cc` and `bcc` lists of the
  stored e-mails that remain (other people's raw sources and full text keep them, as with
  redaction); their messages as a participant go with them. Before the lists are cut, a row that
  went to or came with more than one address gets `shared = true`, a flag that names no one, and
  the one-recipient rule for delivery reports and complaints (`soleRecipient`,
  `MessageIDSharedBeyond`) treats a shared row as having several recipients, so the deleted person
  cannot later forge a report that suppresses the customer. Export:
  `GET /v1/contacts/{id}/participations` lists the other contacts' conversations that hold one of
  the contact's addresses as a participant or in a stored recipient list; someone only copied gets
  a contact with their address first.
- Analytics keeps no IP address and puts nothing on a visitor's device by default (see "Analytics"). Raw
  visits and events are kept `YUVA_ANALYTICS_RAW_DAYS` (90), rollups the property's `retention_months`
  (25); salts are deleted two hours after their day. Members delete a property's data for a range or in
  full; a property, a consented visitor, a contact with linked visits and a deleted workspace take their
  analytics rows with them. Workspace `retention_days` does not apply to analytics.
- Report rollups (M29) hold counts per inbox, channel kind, team, member and label, no content; they
  go with the workspace, and `retention_days` does not apply to them.
- Status page subscribers (M19) are subscriptions of a status list and follow the list rules above
  (pending ones after 30 days with the contacts only they created, unsubscribed ones after 3 years);
  notice deliveries and their addresses go after 90 days, the notice keeps its counts.
- Help center (M20) counters hold no visitor data: `help_article_daily` counts views, ratings,
  conversations and cards per article and day, and `help_search_daily` counts searches per normalized
  query and day (kept 90 days; queries shaped like an e-mail address, a URL or a number are stored as
  `(scrubbed)`), with no address, cookie, session or id. Articles and their revisions are the
  workspace's content: workspace deletion takes them, and retention days do not apply.
- Nothing is sent to third parties except what a channel is configured to send (SMTP, webhooks);
  the monthly GeoIP download from DB-IP fetches a file and sends nothing about anyone.

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
    by the workspace it returned. `/ingress/ses` also finds a broadcast delivery by its Message-ID
    (`broadcast_deliveries.header_message_id`, unique across the server) and a status notice delivery by
    `status_notice_deliveries.header_message_id` (unique across the server too), and `/i/{token}` finds an
    e-mail image by its token (`email_images.token`, unique across the server) before the workspace
    is known; everything after those lookups is scoped by the workspace they returned.
  - The widget and the apps name only a chat or app channel's public key, which is unique across
    the server: a new contact session finds its channel by `chat_channels.public_key` (app
    channels live in the same table), and a CORS preflight, which
    carries neither the key nor the session, asks whether any chat channel allows its origin. A
    contact session token is looked up by its hash in `contact_sessions` before its workspace is
    known. Everything after those lookups is scoped by the workspace they returned.
  - Status pages (M19): a public page finds its page by `status_pages.slug`, a former slug in
    `status_page_slugs` or a verified `status_pages.custom_domain` (a slug's and a domain's availability is
    checked the same way, a verified domain fails the other claims of its name in every workspace, and the
    domain sweep lists the domains due for a check of every workspace), and a
    heartbeat or push finds its monitor by
    `status_monitors.token_hash` (the sweep of overdue heartbeats lists the due monitors of every workspace
    in one query, and the notice sweep the due notices of every workspace), and an SES notification finds a notice delivery by
    `status_notice_deliveries.header_message_id`, all unique across the server; everything after those lookups is
    scoped by the workspace they returned.
  - Help centers (M20): a public help page finds its center by `help_centers.slug` or a former slug in
    `help_center_slugs`, both unique across the server (availability is checked against current and
    former slugs of every workspace); everything after that lookup is scoped by the workspace it
    returned. The in-process counter flush writes per workspace. No other exception.
  - Own domains (M19, #148): a request on an own domain, and the certificate check, find the page by a
    verified `custom_domain` of `status_pages`, `help_centers`, `roadmap_boards` or `changelogs` in one
    query; a verified name is unique across the server and the four kinds. A domain's availability is
    checked against the verified domains of all four in every workspace, its verification fails the
    other claims of its name in all four and every workspace, and the domain sweep lists the domains due
    for a check of all four and every workspace. Everything after those lookups is scoped by the
    workspace they returned.
  - Single sign-on (M33): a sign-in finds the workspace whose identity provider an address uses by its
    verified `sso_domains.domain`, unique across the server, before any workspace is known (when
    starting a sign-in), and the
    OIDC callback and the SAML ACS find their `sso_logins` row by the hash of the state or request id
    alone; adding or verifying a claim checks
    the verified domains of every workspace. Everything after those lookups is scoped by the workspace
    they returned.
  - The person-level identity tables `people`, `sessions`, `login_codes`, `passkeys`,
    `webauthn_ceremonies` and `push_subscriptions` (a browser's subscription follows the person
    into every workspace; the send job finds it by its id). A person signs in once and can be a member of several workspaces, so
    these rows belong to a person (or, for `login_codes`, an e-mail address before sign-in), not
    to a workspace. Their queries are scoped by the person id, the e-mail address or a secret
    hash instead; everything a person may do in a workspace goes through their `members` row.
  - The reader account endpoints (`/v1/account/*`) list a person's discussions, follows and
    notifications across every workspace. `reader_tokens` and `reader_notifications` carry
    `workspace_id` (and every write and workspace-side read filters by it), but these account
    reads are scoped by the person id instead (through `contacts.person_id` or the row's own
    `person_id`), leaving out deleted workspaces, like `/v1/me` lists a person's memberships. A
    reader token is looked up by its hash before its workspace is known, like a contact session.
    `people.reader_email_notifications` is a person-level setting.
  - Open sign-up's limits count `workspaces` by `created_by` (a person) and `created_ip` across
    the server, before the new workspace exists.
  - `operator_notices` is the server operator's feed of new people and workspaces, broadcast approval
    requests, suspensions and automatic pauses (`YUVA_OPERATOR_EMAIL`): it is about the server, not a
    workspace (a line names the workspace), and is read only by the operator digest job.
  - Analytics collection names only a property's public key: `POST /client/v1/analytics/{key}` finds
    the property by `analytics_properties.public_key` (or its `previous_key` during a rotation), unique
    across the server, and a share link finds its share by `analytics_shares.token_hash`; everything
    after those lookups is scoped by the workspace they returned. Whole day partitions of
    `analytics_visits`, `analytics_events` and `analytics_visit_pages`, which hold every workspace's
    rows of that day, are created by the flusher (yesterday to the day after tomorrow in UTC when it starts, and any day it
    is about to write that has none) and by the `analytics_partitions` job, which also drops them;
    every other read, write and delete of those tables filters by `workspace_id`.
  - A forwarding address names only its token: ingress finds the channel by
    `email_channels.forward_token` (unique across the server), like an address. A migration probe is
    recognised by its HMAC before the channel's workspace matters.
  - The migration watchdog lists migrations whose lease expired across workspaces
    (`migrations_lease_idx`), and the hourly upload sweep lists ended migrations that still have
    uploaded parts (`ListEndedMigrationUploads`); everything after those lookups is scoped by the
    workspace they returned.
  - The hourly audit log sweep deletes entries older than `YUVA_AUDIT_DAYS` across workspaces, 5000
    rows at a time (`audit_log_created_idx`): the period is the same for every workspace, and no row
    is read or returned, only deleted by age.
  - `migration_audit` rows outlive their migration (no foreign key to it) but carry `workspace_id`
    and go with the workspace.
  - Slack (M32): `POST /ingress/slack` finds the installation by `slack_installations.team_id`
    (unique across the server) before the workspace is known; everything after that lookup is
    scoped by the workspace it returned. The OAuth callback's state names its workspace.
  - Issue trackers (M32): `/ingress/issues/{integrationId}` finds its integration by
    `issue_integrations.id` (unique across the server) before the workspace is known, and the
    reconciliation sweep takes due linked `tracked_issues` of every workspace
    (`tracked_issues_next_sync_idx`); everything after those lookups is scoped by the workspace they
    returned.
  - `yuva api-key revoke <id>` runs on the server with no workspace given: it finds the key by its
    id, unique across the server, and revokes it scoped by the workspace that lookup returned.
- Usage counters (conversations, messages, members, storage, broadcast recipients, status notice mails) recorded per workspace per month.
- Billing and plans are not part of the open-source core. Sign-up is: `YUVA_SIGNUP=open` lets
  anyone create an account and a workspace, within per-person and per-address limits (see Panel).

## Deployment

One Docker image; `deploy/compose.yaml` for a single host; the Helm chart `deploy/helm/yuva` (published as `oci://ghcr.io/productdevbook/charts/yuva`, same version as the server, no bundled database) for Kubernetes. Runs on Postgres 16+; CI, the development compose file and production run Postgres 18.
The minimal install is the binary, Postgres and an SMTP account.
