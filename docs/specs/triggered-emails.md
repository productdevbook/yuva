# Triggered e-mails (#205)

Status: planned, milestone M40 (M40.1–M40.3). Owner of the design: yuva-broadcasts. Builders:
yuva-api (`openapi/`, `api/`, MCP), yuva-panel (`web/`), yuva-widget (`sdk/js` schema), yuva-docs
(`docs/`). Built in a fresh session per step, from this file.

A triggered e-mail sends one design to each contact once, a set time after something happened to
that contact in an inbox: first signed in to the product (the SDK or the host backend identified a
new user), later also subscribed to a list or an attribute took a value. It is per inbox, through
one of its e-mail channels, with the designs, languages, footer, unsubscribe handling, pace,
sending hours, daily limit, bounce and complaint handling, replies and reports that broadcasts
already have.

Not part of it: multi-step journeys with branches, waits between steps or goals, A/B tests, lead
scoring, open tracking (Yuva has no open pixel, a closed decision). Several triggered e-mails with
different delays on one trigger give a simple sequence (day 1, day 3, day 10), each with its own
once-per-contact rule.

## 1. Decisions

1. **A triggered e-mail is a broadcast of kind `triggered`.** The `broadcasts` row and its
   `broadcast_deliveries` carry it, so the renderer, review, test sends, pace, sending hours,
   SES tenants, DKIM, `Message-ID` tokens, replies as conversations, bounces, complaints, automatic
   pauses, click tracking, per-language reports, `/m/` tokens, the daily limit, warm-up and the
   operator's approval apply unchanged. What's new broadcasts set this precedent (a column on
   `broadcasts`). What differs: an audience that grows (enrollments), a state `active` that never
   ends in `sent`, and the trigger, delay and consent basis.
2. **Lifecycle.** `draft` → `scheduled` (when it starts later) → `active` ⇄ `paused` → `cancelled`
   (shown as "Stopped"); `failed` when its channel is removed. Pausing stops sending, not
   enrolling, so nobody is missed. Stopping ends enrolling for good, drops waiting contacts
   (`cancelled`) and skips queued deliveries (`cancelled`), as cancelling a broadcast does.
3. **Once per contact, for as long as the triggered e-mail and the contact exist.** An enrollment
   row per (triggered e-mail, contact) is the guard (`PRIMARY KEY`), kept after its delivery is
   deleted at 400 days. A contact is evaluated once, at its due time; not matching then is final
   (`skipped` with the reason).
4. **Existing contacts are never mailed unless asked.** Only triggers that happen while the
   triggered e-mail is `active` or `paused` enroll. Starting it may include contacts whose trigger
   happened in the last `backfill_days` (0 to 30, default 0) before the start, with the count shown
   in the review and required by the MCP start tool. Contact imports, help-desk migrations and
   merges never trigger; `/v1/contacts` writes trigger unless they say `skip_triggers: true` (for a
   host backend that back-fills users the product already had).
5. **The consent basis is chosen explicitly and fixed after the draft.**
   - `marketing`: only contacts subscribed (state `subscribed`) to one of the chosen lists of the
     inbox (1 to 10, never a status list) at their due time; footer and one-click unsubscribe from
     that list, as a broadcast. This is the only basis for advertising, newsletters and offers.
   - `service`: every matching contact, for mail about the person's use of the product (onboarding,
     account, a feedback request). No lists. Starting it needs `confirm_service: true`, stored with
     who confirmed and when, and shown in the audit log and the operator digest. The footer says
     why the person gets it ("You get this e-mail because you use <inbox>.") and still carries the
     postal address and one-click unsubscribe, which stops all `service` triggered e-mails and
     designed outreach (#209) of that inbox for the contact (`inbox_mail_optouts`). The FTC reads
     "transactional or relationship" narrowly and an existing relationship alone does not make a
     mail transactional, so even service mail keeps the CAN-SPAM opt-out and address (ftc.gov
     CAN-SPAM compliance guide); Türkiye's Regulation on Commercial Communication applies only to
     messages of a commercial nature (ticaret.gov.tr, Ticari Elektronik İletiler › Genel Bilgiler),
     so a service mail needs no İYS consent while a marketing one needs consent registered in İYS,
     which the list's `format=iys` export serves as for broadcasts.
   Both bases respect hard bounces and complaints (`scope` `all` and `broadcast`), `email_optouts`
   and blocked contacts; a complaint about a triggered e-mail is handled as one about a broadcast
   (broadcast suppression, every list of the workspace unsubscribed, an opt-out).
6. **Delay and sending hours.** `delay_minutes` 0 to 525,600 (365 days). Due time = trigger time +
   delay, computed from the current delay, so a changed delay applies to everyone waiting. The
   broadcast's `timezone`, `send_window`, `pace` and the workspace's daily limit gate sending as
   for broadcasts. A contact whose due time passed more than 7 days before it is processed (a long
   pause, the daily limit) is skipped `late`, so nobody gets an onboarding mail a month late.
7. **Content.** A design (frozen at start and at every resume after an edit), subject and preheader
   per language (`translations`), the contact's version chosen at its due time as for broadcasts.
   Merge fields as for broadcasts; `list.name` is refused with `service`. A design with a `message`
   block (#209) is refused, as for broadcasts.
8. **Replies** open or continue a conversation in the inbox with `broadcast_id` and the
   `broadcast_reply` marker, exactly as broadcast replies; reply notifications are grouped as for
   broadcasts.
9. **Sending order.** In a channel's send job, active triggered e-mails with queued mail go before
   one-off broadcasts (they are few and time-bound); both share the channel's rate, the pace and the
   workspace's daily limit. A triggered e-mail with nothing queued is passed over, never marked
   `sent`.
10. **Who.** As broadcasts: owners, admins and keys with `broadcasts:write` create and change them;
    `broadcasts:send` starts and resumes, only while the workspace allows keys to
    (`broadcasts_by_keys`); `broadcasts:read` reads them, their waiting list and their report.
    `YUVA_BROADCASTS=off` refuses them (`403 broadcasts_disabled`); `approval` needs approval.

## 2. Triggers

| Event | Fires when | Trigger time | Back-fill | Step |
|---|---|---|---|---|
| `contact_identified` | the contact gets its first external id in the inbox: an identity token (widget, Swift, Kotlin, `/client/v1` sessions), `POST`/`PATCH /v1/contacts` with an external id of the inbox (unless `skip_triggers`), feedback or a subscription that carries an external id | the external id's `created_at` | contacts whose external id in the inbox (else, for rows from before this step, the contact) was created in the window | M40.1 |
| `subscribed` | a subscription to the named list of the inbox (never a status page's) becomes `subscribed`, from any source but `import` (a confirmed double opt-in counts when confirmed; a subscription an import created never does, also when its person confirms it later) | `consent_at` | subscriptions of the list confirmed in the window, imports excluded | M40.3 |
| `attribute_changed` | a write (identity token, `/v1/contacts`, the panel, a subscription that creates its contact with attributes) makes the contact match the trigger's condition (a `BroadcastCondition`) when it did not before; creating a contact that matches counts; `skip_triggers` applies. Only triggered e-mails of an inbox the contact belongs to (an external id or a conversation in it, a subscribed subscription to one of its lists) or the write came through (the identity token's inbox, the subscription's list's) | the write | none (`backfill_days` must be 0) | M40.3 |

- An identity token without `email_verified` stores the address as `typed_email`, which is not a
  contact address: such contacts are skipped `no_address` at their due time. The guide says to send
  verified addresses.
- A merge moves the source contact's enrollments to the target where the target has none for that
  triggered e-mail and drops the rest; a merge never fires a trigger.
- The trigger is fixed once the triggered e-mail leaves the draft.

## 3. Data model

One migration per step, each the next free number when it is written.

M40.1:

```sql
ALTER TABLE broadcasts
    ADD COLUMN kind text NOT NULL DEFAULT 'once' CHECK (kind IN ('once', 'triggered')),
    ADD COLUMN trigger jsonb,
    ADD COLUMN delay_minutes integer CHECK (delay_minutes BETWEEN 0 AND 525600),
    ADD COLUMN basis text CHECK (basis IN ('service', 'marketing')),
    ADD COLUMN backfill_days integer NOT NULL DEFAULT 0 CHECK (backfill_days BETWEEN 0 AND 30),
    ADD COLUMN service_confirmed_by uuid,
    ADD COLUMN service_confirmed_by_key uuid,
    ADD COLUMN service_confirmed_at timestamptz,
    ADD CONSTRAINT broadcasts_triggered_check CHECK (
        (kind = 'once' AND trigger IS NULL AND delay_minutes IS NULL AND basis IS NULL)
        OR (kind = 'triggered' AND trigger IS NOT NULL AND delay_minutes IS NOT NULL AND basis IS NOT NULL)),
    DROP CONSTRAINT broadcasts_state_check,
    ADD CONSTRAINT broadcasts_state_check
        CHECK (state IN ('draft', 'scheduled', 'preparing', 'sending', 'active', 'paused', 'sent', 'cancelled', 'failed')),
    ADD CONSTRAINT broadcasts_service_confirmed_by_fkey FOREIGN KEY (workspace_id, service_confirmed_by)
        REFERENCES members (workspace_id, id) ON DELETE SET NULL (service_confirmed_by);
CREATE INDEX broadcasts_triggered_idx ON broadcasts (workspace_id, inbox_id)
    WHERE kind = 'triggered' AND state IN ('active', 'paused');
DROP INDEX broadcasts_active_idx;
CREATE INDEX broadcasts_active_idx ON broadcasts (workspace_id, state)
    WHERE state IN ('scheduled', 'preparing', 'sending', 'active', 'paused');
DROP INDEX broadcasts_channel_idx;
CREATE INDEX broadcasts_channel_idx ON broadcasts (workspace_id, channel_id, started_at)
    WHERE state IN ('sending', 'active');

CREATE TABLE triggered_enrollments (
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    broadcast_id uuid NOT NULL,
    contact_id   uuid NOT NULL,
    triggered_at timestamptz NOT NULL,
    state        text NOT NULL DEFAULT 'waiting' CHECK (state IN ('waiting', 'queued', 'skipped', 'cancelled')),
    skip_reason  text CHECK (skip_reason IN ('no_address', 'blocked', 'suppressed', 'opted_out', 'stopped',
                                             'not_subscribed', 'not_matching', 'late', 'removed')),
    delivery_id  uuid,
    source       text NOT NULL CHECK (source IN ('trigger', 'backfill')),
    created_at   timestamptz NOT NULL,
    updated_at   timestamptz NOT NULL,
    PRIMARY KEY (workspace_id, broadcast_id, contact_id),
    FOREIGN KEY (workspace_id, broadcast_id) REFERENCES broadcasts (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, contact_id) REFERENCES contacts (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, delivery_id) REFERENCES broadcast_deliveries (workspace_id, id) ON DELETE SET NULL (delivery_id)
);
CREATE INDEX triggered_enrollments_due_idx ON triggered_enrollments (workspace_id, broadcast_id, triggered_at, contact_id)
    WHERE state = 'waiting';
CREATE INDEX triggered_enrollments_contact_idx ON triggered_enrollments (workspace_id, contact_id);

ALTER TABLE contact_external_ids ADD COLUMN created_at timestamptz;
CREATE INDEX contact_external_ids_inbox_created_idx ON contact_external_ids (workspace_id, inbox_id, created_at);

ALTER TABLE broadcast_deliveries
    DROP CONSTRAINT broadcast_deliveries_skip_reason_check,
    ADD CONSTRAINT broadcast_deliveries_skip_reason_check
        CHECK (skip_reason IN ('suppressed', 'opted_out', 'unsubscribed', 'cancelled', 'stopped'));
```

- `trigger` jsonb: `{"event": "contact_identified"}`; M40.3 adds `{"event": "subscribed",
  "list_id": "…"}` and `{"event": "attribute_changed", "condition": {"attribute": "plan", "op":
  "eq", "value": "pro"}}`. `audience` keeps `list_ids` (marketing lists; empty for service),
  `conditions` and `locales`.
- `inbox_mail_optouts` comes from M13.10 (`docs/specs/conversation-designs.md`); if M40.1 is built
  first, its migration creates that table as defined there.
- `contact_external_ids.created_at` is set on every new row from M40.1 on; rows from before stay
  NULL and the back-fill reads the contact's `created_at` for them (earlier, so it only excludes).
- The constraint names above follow Postgres defaults for the inline checks of migration 00037;
  the implementer reads the real names from the database before dropping them.
- Every query on `triggered_enrollments` filters by `workspace_id`; the sweep walks workspaces as
  `SweepBroadcasts` does, so no cross-workspace query is added.
- Retention: enrollments live as long as their triggered e-mail and contact; deliveries keep their
  400 days, so a long-running triggered e-mail's report covers the last 400 days. Enrollments hold no
  address or content, only the link and times; deleting the contact deletes them. They are part of
  the workspace export wherever deliveries are.

## 4. Jobs and flow

- **Enrolling (in the writing transaction).** `s.triggerContact(ctx, q, events, ws, inboxID,
  contactID, event, at)` runs where a trigger happens (section 2): one indexed query for the inbox's
  `active` or `paused` triggered e-mails with that event (`broadcasts_triggered_idx`), then
  `INSERT … ON CONFLICT DO NOTHING` per match (`source` `trigger`), then
  `events.job(TriggeredDueArgs{ws, broadcast}, ScheduledAt: at + delay)`. Callers that import,
  migrate or merge do not call it; `eventBatch` carries a `noTriggers` flag for code paths shared
  with imports.
- **`triggered_due {workspace_id, broadcast_id}`** (queue `broadcasts`, unique by args among
  `available`, `scheduled` and `retryable` jobs, so one waits per triggered e-mail and a running one
  can queue its successor). When the triggered e-mail is `active`: in transactions of 500, lock due
  waiting enrollments (`triggered_at + delay <= now`, `FOR UPDATE SKIP LOCKED`, oldest first) and
  for each decide, in this order: contact blocked → `blocked`; no contact address (first
  `contact_emails` row) → `no_address`; due more than 7 days ago → `late`; suppression of scope
  `all` or `broadcast` → `suppressed`; `email_optouts` → `opted_out`; `service` and an
  `inbox_mail_optouts` row → `stopped`; `marketing` and not `subscribed` to one of the lists →
  `not_subscribed`; audience conditions or locales not matching (the same SQL builder as the
  broadcast audience, restricted to these contact ids) → `not_matching`. Otherwise create the
  `broadcast_deliveries` row (`queued`, address, token, `Message-ID`, the first matching list in
  order, the resolved version in `locale`; `ON CONFLICT (workspace_id, broadcast_id, contact_id) DO
  NOTHING`) and set the enrollment `queued` with `delivery_id`. Recount the broadcast, queue the
  channel's `broadcast_send`, and when waiting enrollments remain queue the next `triggered_due` at
  the earliest due time (at most 5 minutes ahead, so a changed delay applies soon). When the
  triggered e-mail is `paused` the job does nothing; resuming queues it.
- **Starting.** `POST /schedule` runs the review, freezes the design and sets `scheduled` (with
  `send_at` in the future) or goes straight to the start. `broadcast_start` branches on `kind`: for a
  triggered e-mail it sets `active` and `started_at`, then back-fills when `backfill_days > 0`:
  pages of 1,000 contacts whose trigger time lies in `[start − backfill_days, start)`, inserted with
  `source` `backfill` and their own trigger time (so the delay counts from then and an old one is due
  at once, spread by the pace), snoozing every ~30 s as the audience snapshot does; then it queues
  `triggered_due`.
- **Sending.** `ListSendingBroadcastsOfChannel` takes `state IN ('sending', 'active')`, ordered
  triggered first; for an `active` one with nothing queued the loop passes over it instead of
  calling `FinishBroadcast`. Each recipient is re-checked before their mail as today, plus
  `inbox_mail_optouts` for `service` (`skipped`, `stopped`). Automatic pauses (5% bounces after 200,
  0.3% complaints after 1,000, sender errors, the SES tenant, workspace suspension) apply, counted
  since start or the last resume.
- **Sweep.** `SweepBroadcasts` (every 5 minutes) also queues `triggered_due` for every `active`
  triggered e-mail with a due waiting enrollment and `broadcast_start` for a `scheduled` one whose
  time has come, so a lost job costs at most 5 minutes.
- **Edits.** While `active`: `pace`, `timezone`, `send_window`, `delay_minutes`, `click_tracking`
  off. While `paused`: also `design_id`, `subject`, `preheader`, `translations`, `audience.conditions`,
  `audience.locales`, `click_tracking`; resuming runs the review again and freezes the new design
  version (queued deliveries go out with it). Never after the draft: `kind`, `trigger`, `basis`,
  `audience.list_ids`, `inbox_id`, `channel_id`. To change those, stop it and create another
  (enrollments do not carry over).

## 5. Mail: footer, headers, unsubscribe

- `marketing`: the broadcast footer and headers unchanged (`List-Unsubscribe`,
  `List-Unsubscribe-Post`, `List-Id`, `Feedback-ID` with the list, `X-Auto-Response-Suppress`); its
  one-click unsubscribes from that list.
- `service`: renderer footer mode `service` (new, beside #209's modes): the identity line, the new
  sentence `account` ("You get this e-mail because you use %s.", the inbox's name), Unsubscribe and
  Manage preferences. The sentence is added to all 44 footer languages in `i18n.go`. Headers as a
  broadcast without `List-Id` (`Feedback-ID` list part `00000000`). Its `/m/` page (token: workspace,
  contact, broadcast, no list) names the inbox and offers "Stop e-mails like this from <inbox>"
  (writes `inbox_mail_optouts`, source `triggered`), the inbox's public lists and "Unsubscribe from
  everything"; its one-click POST does the first. Opening it changes nothing.
- `Message-ID`, Reply-To, the broadcast SMTP account (else the support account), Yuva DKIM when
  enabled, SES tenant headers: as broadcasts. Never the server's `YUVA_SMTP_*`.

## 6. Contract (`openapi/openapi.yaml`)

Schemas:

- `BroadcastKind` `once | triggered`; `Broadcast.kind` (required); `BroadcastCreate.kind`
  (default `once`).
- `BroadcastState` gains `active`; its description gains the triggered lifecycle.
- `BroadcastTrigger` `{event: contact_identified (M40.1) | subscribed | attribute_changed (M40.3),
  list_id?, condition?: BroadcastCondition, delay_minutes: 0..525600, basis: service | marketing,
  backfill_days: 0..30}`; `Broadcast.trigger` (present for `triggered`), settable on create and on
  update within the rules of section 4. `400 trigger_invalid` for a wrong combination (`list_id`
  only and always with `subscribed`, `condition` only and always with `attribute_changed`,
  `backfill_days` above 0 with `attribute_changed`, also on `/schedule`); a `list_id` of another
  inbox or of a status page is `400 validation_failed`; on a draft a new `event` drops the
  `list_id` and `condition` it does not take. `400 basis_lists` for `service` with lists or
  `marketing` without.
- `BroadcastSchedule` gains `confirm_service: boolean` (required true for `service`:
  `400 service_basis_unconfirmed`); `Broadcast.service_confirmed {by, at}`.
- `BroadcastCounts` gains `enrolled`, `waiting`, and `skipped` reasons `no_address`, `blocked`,
  `not_subscribed`, `not_matching`, `late`, `removed`, `stopped` beside today's.
- `BroadcastReview` items `trigger`, `basis` (`service`: a warning that names what service mail may
  be), `message_block` (blocking); the audience count for a triggered e-mail is the back-fill count
  plus `waiting` (`BroadcastAudienceCount.backfill`).
- `TriggeredEnrollment` `{contact: {id, name, email}, state, triggered_at, sends_at (waiting:
  due time moved into the send window; absent otherwise), skip_reason, delivery_id, source}`;
  `TriggeredEnrollmentPage`.
- `ContactCreate` and `ContactUpdate` gain `skip_triggers: boolean`.

Operations:

| Method and path | Change | Scope | Errors |
|---|---|---|---|
| `GET /v1/broadcasts` | `kind` filter | `broadcasts:read` | — |
| `POST /v1/broadcasts` | `kind`, `trigger` | `broadcasts:write` | `400 trigger_invalid`, `400 basis_lists`, `409 too_many_triggered` (20 live per inbox) |
| `PATCH /v1/broadcasts/{id}` | trigger fields per section 4 | `broadcasts:write` | `409 invalid_state` for a field not changeable in the state |
| `POST /v1/broadcasts/{id}/schedule` | starts it; `confirm_service`, `backfill_days`; `confirm_recipients` is the back-fill count | `broadcasts:send` | today's, `400 service_basis_unconfirmed` |
| `POST /v1/broadcasts/{id}/pause`, `/resume`, `/cancel`, `/unschedule`, `/test`, `/review`, `/count`, `GET …/report`, `GET …/deliveries` | work for both kinds | as today | as today |
| `GET /v1/broadcasts/{id}/enrollments` | new: `state`, `cursor`, `limit` (1–100), newest trigger first, `waiting` oldest due first | `broadcasts:read` | `404`, `400` for a `once` broadcast |
| `DELETE /v1/broadcasts/{id}/enrollments/{contactId}` | new: a waiting contact is dropped (`skipped`, `removed`), kept for the once rule | `broadcasts:write` | `404`, `409 invalid_state` (not waiting) |
| `POST`, `PATCH /v1/contacts` | `skip_triggers` | `contacts:write` | — |

Events and webhooks: `broadcast.updated` on every state or setting change, as today; nothing per
enrollment (volume and privacy). Realtime `broadcast.progress` while it sends. Audit
`broadcast.scheduled` records the basis, the confirmation and `backfill_days`; removing a waiting
contact writes `broadcast.updated` with `enrollment_removed`.

## 7. MCP tools

- `create_triggered_email` (`POST /v1/broadcasts` with `kind: triggered`): inbox, channel, name,
  design, subject and per-language subjects, trigger `{event, delay (minutes, hours or days), basis,
  list_ids, conditions, locales}`, from M40.3 `trigger_list_id` (`subscribed`) and
  `trigger_condition` (`attribute_changed`); always a draft.
- `schedule_broadcast` starts a triggered e-mail too, with two optional inputs: `backfill_days` and
  `confirm_service` (must be true for `service`). Its `confirm_recipients` is, for a triggered
  e-mail, the back-fill count `get_broadcast` showed (0 without back-fill); a changed count
  schedules nothing, as today. Its description names both kinds.
- `list_triggered_waiting` (`GET …/enrollments`), `remove_triggered_contact` (`DELETE
  …/enrollments/{contactId}`). Contact names and addresses go in `customer_content`.
- Existing tools take both kinds: `list_broadcasts` (`kind` filter), `get_broadcast`,
  `update_broadcast` (trigger fields per section 4), `send_test_broadcast`, `pause_broadcast`,
  `resume_broadcast`, `cancel_broadcast` (its description says it stops a triggered e-mail for
  good), `get_broadcast_report` (per language). Same scopes and the `broadcasts_by_keys` gate.

## 8. Panel (`web/`)

- **Broadcasts › Triggered** (sidebar item beside Broadcasts and Designs): list of triggered
  e-mails with name, inbox, a sentence for the trigger ("3 days after someone first signs in to
  Acme"), state badge (Draft, Scheduled, Active, Paused, Stopped, Failed), waiting, sent in the last
  30 days, replies. Empty state: "Send a mail a set time after people sign up, once each" with "New
  triggered e-mail". The approval banner of broadcasts shows here too.
- **Editor** (the broadcast composer with these sections): Trigger (event; delay with value and
  minutes/hours/days), Who gets it (basis as two cards: "Service mail: about using <inbox>, sent to
  everyone who matches" and "Marketing: only to subscribers of" with the list picker; conditions
  and languages as for broadcasts), Content (design, subjects per language), Sending (channel,
  time zone, sending hours, pace). Fields not changeable in the current state are read-only with
  the reason.
- **Start** dialog: the review (as for broadcasts, plus trigger and basis items), "Also send to
  people whose trigger happened in the last [N] days" with the live count, and for service the
  checkbox "This mail is about using <inbox>, not advertising" (required). "Start now" or a start
  time.
- **Detail page**: header with state, trigger sentence and actions (Pause, Resume, Edit while
  paused, Send test, Stop with a confirmation naming how many are waiting); tabs Report (the
  broadcast report with languages, plus enrolled, waiting and skipped by reason), Waiting (contact,
  triggered, sends at in the broadcast's zone, Remove; paged), Sent (the deliveries list), Settings.
  Pause reasons Yuva sets show as for broadcasts.
- States everywhere: loading skeletons, errors with the server's message, empty lists with a short
  line. Lingui strings in `en`, extracted to every panel catalog.

## 9. Limits

| Limit | Value |
|---|---|
| Live (scheduled, active, paused) triggered e-mails per inbox | 20 |
| Delay | 0 minutes to 365 days |
| Back-fill | 0 to 30 days |
| Lateness before `late` | 7 days after the due time |
| Enrollments processed per transaction | 500 |
| Recipients | count toward `YUVA_BROADCAST_DAILY_RECIPIENTS`, warm-up, the channel's rate and the pace |
| Test sends | as broadcasts (20 an hour per workspace) |

On hosted servers (`YUVA_SIGNUP=open`, `approval` mode) starting and resuming need approved
broadcasts; the operator digest lists triggered e-mails started that day with their basis and
`yuva workspace broadcasts show` lists live ones with basis and 30-day counts. No content reaches
the operator.

## 10. Order of work and acceptance

### M40.1 Triggered e-mails: server, API and MCP (yuva-api, then yuva-widget, yuva-docs)

1. yuva-api: contract (section 6, `contact_identified` only); lint; regenerate.
2. yuva-api: migration, enrolling at every `contact_identified` site, `skip_triggers`,
   `triggered_due`, start and back-fill, sender changes, review items, `service` footer with the
   `account` sentence in all 44 languages, `/m/` stop page, enrollments endpoints, MCP tools.
3. yuva-widget: regenerate the `sdk/js` schema.
4. yuva-docs: `docs/broadcasts.md` "Triggered e-mails" section and the API reference, `docs/mcp.md`,
   with `tr` and `de`.

Accept (tests with Postgres, Mailpit for mail):
- A service triggered e-mail with a 3-day delay: a contact identified by an identity token with a
  verified address and `locale` `tr` is waiting with `sends_at` 3 days later; moving the clock
  sends the Turkish version once through the inbox's channel; identifying the same user again and
  again sends nothing more; the mail has the service footer, `List-Unsubscribe` with one-click and
  no `List-Id`.
- Contacts identified before the start get nothing; with `backfill_days: 7` those identified in the
  last 7 days are enrolled and the review showed that count.
- A marketing triggered e-mail skips a contact not subscribed to its list (`not_subscribed`) and
  sends to a subscribed one; unsubscribing by one-click stops later broadcasts to that list.
- The service one-click writes `inbox_mail_optouts`; a second service triggered e-mail of the inbox
  skips the contact (`stopped`).
- Paused for 8 days with a contact due on day 1: on resume that contact is skipped `late`; a
  contact due on day 6 is sent.
- A reply to the mail opens a conversation in the inbox with `broadcast_id`; an out-of-office reply
  only counts.
- Starting a `service` one without `confirm_service` answers `400 service_basis_unconfirmed`; a key
  starts one only with `broadcasts_by_keys`; another workspace sees none of it.
- `POST /v1/contacts` with `skip_triggers: true` and an external id enrolls nobody.
- Every check of `api/` passes.

### M40.2 Panel (yuva-panel, then yuva-docs)

1. yuva-panel: section 8.
2. yuva-docs: screenshots and the panel steps in `docs/broadcasts.md` with `tr` and `de`.

Accept: the `web/` check passes; device-tester screenshots (WebP, desktop and phone, light and
dark): the empty list, the editor with service and marketing basis, the start dialog with the
back-fill count and the service checkbox, the detail page's Report and Waiting tabs, a paused one
with an automatic pause reason; a member creates, starts, pauses, edits the subject, resumes and
stops one in the dev stack with Mailpit catching the mail.

### M40.3 More triggers (yuva-api, yuva-panel, yuva-widget, yuva-docs)

1. yuva-api: `subscribed` and `attribute_changed` in the contract, enrolling at subscription
   confirmation (not imports) and at attribute writes (identity token, `/v1/contacts`, panel), the
   back-fill for `subscribed`, MCP.
2. yuva-widget: regenerate the `sdk/js` schema.
3. yuva-panel: the two events in the editor (list picker, condition editor).
4. yuva-docs.

Accept: a welcome sequence of two marketing triggered e-mails on `subscribed` (0 minutes, 3 days)
sends the first after confirmation, never on a pending subscription or an import, and the second 3
days later; an `attribute_changed` on `plan` `eq` `pro` sends when an identity token moves `plan`
from `free` to `pro`, not when it is set to `pro` again, and not for a contact already `pro` when it
started.

## 11. Sources

- Google, Email sender guidelines FAQ (support.google.com/a/answer/14229414): one-click
  unsubscribe for marketing and promotional mail, transactional mail excluded; spam rate kept under
  0.1%, never 0.3%; bulk sender about 5,000 messages a day to Gmail accounts.
- FTC, CAN-SPAM Act: A Compliance Guide for Business (ftc.gov/business-guidance/resources/can-spam-act-compliance-guide-business):
  transactional or relationship categories read narrowly, primary purpose test, a valid postal
  address in every commercial message.
- T.C. Ticaret Bakanlığı, Ticari Elektronik İletiler › Genel Bilgiler
  (ticaret.gov.tr/ic-ticaret/ticari-elektronik-iletiler/genel-bilgiler): the Regulation applies
  only to messages of a commercial nature.

## 12. Proposed text for `docs/architecture.md` and `docs/roadmap.md`

### `docs/architecture.md`, "Goals", the "Not goals" paragraph

Replace "Since M13 it sends broadcasts (newsletters, announcements) to contacts who subscribed, and
replies to them land in the inbox (see "Broadcasts and e-mail designs")." with:

```markdown
Since M13 it sends broadcasts (newsletters, announcements) to contacts who subscribed, and
replies to them land in the inbox (see "Broadcasts and e-mail designs"). Since M40 it sends
triggered e-mails: one design to each contact once, a set time after a trigger; journeys with
several steps, branches or goals stay out.
```

### `docs/architecture.md`, "Broadcasts and e-mail designs (M13)", after the "Sending hours (M13.8)" bullet

```markdown
- **Triggered e-mails (M40).** A broadcast of `kind` `triggered` sends its design once to each
  contact of its inbox, `delay_minutes` (0 to 365 days) after a trigger: `contact_identified` (the
  contact's first external id in the inbox, from an identity token, `/v1/contacts` or a form that
  carries one), and from M40.3 `subscribed` (a list subscription confirmed, imports excluded) and
  `attribute_changed` (a contact newly matching a condition). Lifecycle `draft` → `scheduled` →
  `active` ⇄ `paused` → `cancelled` (Stopped), never `sent`; pausing stops sending, not enrolling.
  The trigger's transaction inserts `triggered_enrollments (workspace, broadcast, contact,
  triggered_at, state waiting|queued|skipped|cancelled)`, whose primary key is the once-per-contact
  rule for as long as both exist, and queues `triggered_due` at the due time (unique per broadcast;
  the 5-minute broadcast sweep re-queues it). `triggered_due` takes due enrollments 500 at a time,
  decides each once (blocked, no address, more than 7 days late, suppressed, opted out, stopped,
  not subscribed, not matching the audience conditions and locales) and creates the
  `broadcast_deliveries` row the channel's `broadcast_send` sends, triggered e-mails before
  one-off broadcasts, under the pace, sending hours, daily limit, warm-up and automatic pauses of
  broadcasts. Only triggers while it is active or paused enroll; starting may back-fill contacts
  triggered in the last 0 to 30 days, counted in the review; imports, help-desk migrations and
  merges never trigger, and `/v1/contacts` writes with `skip_triggers` do not. The consent basis is
  explicit and fixed after the draft: `marketing` mails only contacts subscribed to its lists, with
  the broadcast footer and one-click from the list; `service` (mail about using the product) mails
  every matching contact, needs `confirm_service` when started (stored and audited), and has a
  footer saying the person uses the inbox's product, the postal address and a one-click
  unsubscribe that writes `inbox_mail_optouts`, which stops the inbox's service triggered e-mails
  and designed outreach. Both respect suppressions of either scope and opt-outs; complaints count
  as about a broadcast. Design, subjects and preheaders per language, merge fields, review, test
  sends, replies, click tracking and the per-language report are a broadcast's; the report adds
  enrolled, waiting and skips by reason. Changeable while active: pace, zone, sending hours, delay;
  while paused also the content and audience conditions (resume reviews and freezes again); never
  the trigger, basis, lists, inbox or channel. At most 20 live per inbox. `GET
  /v1/broadcasts/{id}/enrollments` lists who waits and why others were skipped; `DELETE
  …/enrollments/{contactId}` drops one waiting contact. No event or webhook per enrollment.
```

### `docs/roadmap.md`, a new milestone after "M39 — Bulk actions"

```markdown
## M40 — Triggered e-mails

Status: planned (#205); spec `docs/specs/triggered-emails.md`, decisions in `architecture.md` ›
Broadcasts and e-mail designs › Triggered e-mails (M40).

- **M40.1 Server, API and MCP:** broadcasts of kind `triggered` on `contact_identified`, a delay,
  the sending hours, pace and limits of broadcasts, once per contact, a `service` or `marketing`
  basis, back-fill only when asked, the service footer in every footer language with its stop page,
  the waiting list, reports, `skip_triggers` on contacts, MCP tools.
- **M40.2 Panel:** Triggered under Broadcasts: the list, the editor, the start dialog with the
  back-fill count and the service confirmation, the detail page with report and waiting list.
- **M40.3 More triggers:** `subscribed` (a confirmed list subscription) and `attribute_changed`,
  in the API, MCP and panel.

Accept: an app identifies a new user with a verified address and `locale` `tr`; three days later a
service triggered e-mail sends them the Turkish version once, with one-click unsubscribe and the
postal address, and identifying them again sends nothing; users identified before it started get
nothing unless the start back-filled them; a marketing one skips a contact not subscribed to its
list; a reply becomes a conversation in the inbox; the report shows each language; a hosted
workspace cannot start one before approval; another workspace sees none of it.
```

## 13. Issue bodies

### M40 umbrella (#205)

```markdown
Plan: `docs/specs/triggered-emails.md`; decisions in `architecture.md` › Broadcasts and e-mail
designs › Triggered e-mails (M40).

- [ ] M40.1 Server, API and MCP (yuva-api, then yuva-widget schema, yuva-docs)
- [ ] M40.2 Panel (yuva-panel, yuva-docs)
- [ ] M40.3 More triggers: `subscribed`, `attribute_changed` (yuva-api, yuva-panel, yuva-widget, yuva-docs)

Not part of it: multi-step journeys, branches, A/B tests, open tracking.
```

### M40.1

```markdown
Triggered e-mails: server, API and MCP. Spec: `docs/specs/triggered-emails.md` sections 1–7, 9 and
10 (M40.1). Part of #205.

Order: yuva-api contract → migration and server (enrolling at every `contact_identified` site,
`skip_triggers`, `triggered_due`, start and back-fill, sender, review, `service` footer in all 44
footer languages, `/m/` stop page, enrollments endpoints) → MCP tools → yuva-widget regenerates the
`sdk/js` schema → yuva-docs (`broadcasts.md`, `mcp.md`, tr, de).

Acceptance: the M40.1 list in section 10 of the spec; every `api/` check passes; the change is
reviewed in a fresh context before commit (contract, migration, customer mail).
```

### M40.2

```markdown
Triggered e-mails in the panel. Spec: `docs/specs/triggered-emails.md` section 8 and 10 (M40.2).
Part of #205; needs M40.1.

Acceptance: the M40.2 list in section 10 of the spec; the `web/` check passes; device-tester
screenshots in WebP, desktop and phone, light and dark.
```

### M40.3

```markdown
More triggers for triggered e-mails: `subscribed` and `attribute_changed`. Spec:
`docs/specs/triggered-emails.md` sections 2 and 10 (M40.3). Part of #205; needs M40.1 and M40.2.

Acceptance: the M40.3 list in section 10 of the spec; the `api/`, `sdk/js` and `web/` checks pass.
```
