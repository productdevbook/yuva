# Sending broadcasts

A broadcast is one e-mail sent to the subscribers of your [mailing lists](lists.md): a newsletter, a
product update, an announcement. It uses an [e-mail design](designs.md), goes out through one of your
inbox's e-mail channels, and a reply to it becomes a conversation in that inbox. Yuva shows what
happened afterwards: who got it, who bounced, unsubscribed or replied.

The flow is lists, then a design, then a broadcast. Owners and admins send broadcasts; agents read the
broadcasts of their inboxes so they understand a reply.

## 1. Before the first broadcast

1. Create a [list](lists.md#2-create-a-list) and get subscribers onto it, with a form, from your backend or
   by [importing a CSV file](importing.md). A broadcast goes only to people who are `subscribed` to a list.
2. Fill in the inbox's [sender identity](lists.md#1-set-the-sender-identity). Every broadcast ends with
   it, and the review blocks sending without a legal name and a postal address.
3. Make a [design](designs.md), or start from a template.
4. Check that the inbox has an [e-mail channel](email.md) with an SMTP account and an address to send
   from. A catch-all channel needs a From address. Set up SPF, DKIM and DMARC for that address's domain
   and run the [deliverability check](deliverability.md).
5. Optional but wise: give the channel its own account for broadcasts ([below](#the-sender)).

On a server run by someone else, a new workspace may also have to be [approved](#approval-on-hosted-servers)
before it can send.

## 2. Write the broadcast

In the panel: Broadcasts, then New broadcast. Give it a name that only your team sees and pick the
inbox. It stays a draft, and every change is saved as you make it, until you send or schedule it.

### The sender

Pick the e-mail channel that sends it. The composer says whether the channel is ready, which account
it will use and how fast it sends.

Broadcast mail never goes through the server's own mailer. It uses the channel's separate broadcast
account when there is one, otherwise its support account. A separate account keeps complaints about
marketing mail away from the account that carries your support replies, and a slow send never delays a
support reply.

To set one: Settings → the inbox → Channels → E-mail, then the Broadcast sending section. Switch on "Use a separate
account for broadcasts" and fill in host, port, encryption, user name and password. Saving with the
switch off removes it. The pace is the number of mails a second the channel sends, from 1 to 100 (5 for
a new channel); keep it under what your provider allows.

### The audience

Pick up to 10 lists of the inbox. A contact on several of them gets one mail, under the first of their
lists in the order you chose.

Narrow it down if you like:

- **Conditions** (up to 10; all must match) compare a contact attribute: is, is not, contains (ignoring
  case), is more than, is less than, is one of, is set, is not set. A contact without the attribute
  matches only "is not" and "is not set".
- **Languages** (up to 20) keep only contacts whose language is one of them; `tr` also keeps `tr-TR`.
  Empty keeps everyone.

The composer shows the recipient count as you change the audience, and under "Left out" why the others
get nothing:

| Reason | Who |
|---|---|
| Unsubscribed | Only unsubscribed from the lists. |
| Not confirmed yet | Only `pending` on the lists (double opt-in not completed). |
| Bounced or reported as spam | The address is suppressed. |
| Objected to all mail | The person objected to mail from your workspace. |
| Blocked contacts | You blocked the contact. |
| Left out by a condition or language | A condition or language does not match. |
| On several lists, one mail each | The extra subscriptions. |

### The content

Pick the design. Write the subject (up to 300 characters; inboxes show about the first 80) and,
optionally, the preview text that follows it in most inboxes. Empty uses the design's own. Merge fields
such as `{{ contact.first_name | "there" }}` work in the subject too; the review stops you if a field
without a fallback would be empty for anyone.

### Options

"Count clicks" is off. See [Click tracking](#click-tracking). Links also carry campaign parameters for
[Analytics](#campaign-tracking-utm), on unless you turn them off.

### Sending pace

A broadcast can have its own pace: at most so many mails an hour, a day and a month (30 days), each
optional. "Limit the pace" in the composer turns it on. Yuva then spaces the mails evenly at the
tightest of the three, so 100 a day means one mail about every 14 minutes, not 100 at midnight, and no
60 minutes, 24 hours or 30 days get more than you set. The channel's pace and the workspace's
[daily limit](#the-daily-limit) still apply; the broadcast's pace can only make it slower. Every
attempt counts, a retry too; people skipped because they unsubscribed in the meantime do not. A
broadcast may take at most 365 days at its pace; the review stops you before that. You can change the
pace of a scheduled, sending or paused broadcast from its page; it applies to the next mail. Pausing
does not save up mails: after a resume, the next mail waits for its turn.

### Test sends

Send a test to yourself, or to up to five members, before every send. It looks exactly like the real
mail, with `[Test]` before the subject. Tests go to members of the workspace only, never to another
address, and its unsubscribe link says it was a test. Fill the merge fields with sample values or with
a contact's values (the test still goes only to the members). At most 20 tests an hour per workspace.

## 3. Review and send

"Review and send" shows the checklist the server runs when you schedule. Items marked as blocking stop
the send; warnings do not.

| Item | Blocks when |
|---|---|
| Sender details for the footer | The inbox has no legal name or postal address. |
| Sending channel | No channel is chosen, or it has no SMTP account or From address. |
| Permission to send | The server has broadcasts off, the workspace awaits [approval](#approval-on-hosted-servers) or is suspended. |
| Design | None is chosen, or it renders to more than 500 KB of HTML. |
| Recipients | No list is chosen, or nobody would get the mail. |
| Merge fields | A field without a fallback would be empty for some recipient. |
| Mail size | Over 102 KB of HTML, where Gmail cuts the mail off (footer and unsubscribe link with it). A warning above 90 KB. |
| Image descriptions | Warning: an image has no alt text. |
| Links | Warning: a link uses `http:`. |
| Subject | Empty or over 300 characters. A warning above 80. |
| Sending pace | The broadcast's pace would make sending take more than 365 days. |
| Daily limit | Warning: the [daily limit](#the-daily-limit) makes it take more than one day. |
| Test mail | Warning: no test went out since the last change. |
| Click tracking | Warning when on: links point at the server's link domain. |
| Amazon SES tenant | Blocks when the inbox's SES tenant is paused or missing, could not be set up, or the channel's identity is missing. A warning on a high-severity reputation finding, or while the server is still setting the tenant up (sending waits seconds). |

Then choose Send now, or Schedule and pick a time in your time zone, at most 90 days ahead. The panel
asks you to confirm the recipient count. If the audience changed while you were looking, it asks you
to review again.

What happens when you schedule:

- The design is frozen at its current version. Later edits to the design do not change this broadcast.
- At the start, the audience is taken once. People who subscribe later are not included. Anyone who
  unsubscribes, bounces or complains before their mail goes out is skipped, and each address is checked
  again just before its mail.
- Until it starts, "Back to draft" takes a scheduled broadcast back for changes.

## 4. While it sends

The broadcast page follows the progress live: recipients taken, sent, failed, bounced, and when it is
about done. With a pace, the page shows when the next mail goes out and Change pace adjusts it.

| State | Meaning |
|---|---|
| Draft | Being written. |
| Scheduled | Waits for its time. |
| Preparing | Taking the audience. |
| Sending | Mails are going out. |
| Paused | Stopped until someone resumes it. |
| Sent | Done. |
| Cancelled | Stopped for good. |
| Failed | It cannot go on, for example because its channel was removed. |

**Pause** stops sending within the batch in progress (at most 50 mails). **Resume** continues where it
stopped. **Cancel** ends it for good; mail not sent yet is skipped. A draft or a finished broadcast can
be deleted with its list of recipients; conversations opened by replies stay.

Temporary SMTP failures are retried after 5 minutes, 30 minutes, 2 hours and 6 hours, then the mail is
marked failed. An SMTP refusal of the address itself (5.1.x and 5.2.1) is a bounce.

### The daily limit

A workspace may have a limit on recipients a day (UTC), for all its broadcasts together. It comes from
the server's [`YUVA_BROADCAST_DAILY_RECIPIENTS`](configuration.md#broadcasts), the operator's limit for
the workspace, and on a hosted server the warm-up ([below](#approval-on-hosted-servers)); the smallest
applies. When a send reaches it, the broadcast waits and the panel shows when it continues, at 00:00
UTC. A 250-recipient broadcast under a limit of 100 a day takes three days. The review warns before you
start, and Settings → Workspace → Broadcasts shows today's count against the limit.

### Pauses Yuva makes

A broadcast pauses itself, with the reason on the page, when:

| Reason | Trigger |
|---|---|
| Bounce rate | 5% or more of the mails sent since it started or last resumed bounced (after at least 200 mails). |
| Complaint rate | 0.3% or more were reported as spam (after at least 1,000 mails). |
| Sender error | The SMTP server refuses the account or its TLS, refuses 20 mails in a row, or cannot be reached for 30 minutes. The server's answer is shown. |
| Workspace suspended | See below. |
| Amazon SES tenant | Amazon SES paused the inbox's [tenant](email.md#amazon-ses-tenants), or the channel lost its tenant isolation. The page says which. Other inboxes keep sending. |
| Member, operator | Someone paused it. |

Fix the cause, then resume. After a resume the rates count from that moment.

Resuming is refused with `409 ses_tenant_blocked` while the tenant is paused; the server's operator reinstates it.

A workspace is **suspended** from sending when, over the last 30 days, at least 0.3% of at least 1,000
broadcast mails were reported as spam, or at least 8% of at least 2,000 bounced. Its running broadcasts
pause, and scheduling and resuming are refused. Only the server's operator lifts a suspension: an owner
sees the reason in Settings → Workspace → Broadcasts.

### Bounces and complaints

Yuva reads bounces from Amazon SES (through SNS, see [Bounces](email.md#bounces)), from delivery reports
mailed to the channel, from SMTP refusals while sending, and abuse reports (ARF) mailed to the channel,
which count only from a sender whose DMARC result the receiving server vouches for.

- A hard bounce suppresses the address for all mail from the workspace.
- A spam complaint about a broadcast stops broadcasts to that address, unsubscribes it from every list
  of the workspace and remembers the objection. Support replies can still reach it.

## 5. Replies

A reply to a broadcast opens a conversation with that contact in the broadcast's inbox, marked "Reply to
{broadcast}" and linked to it. Further replies continue the conversation. In the inbox, the filter by
type has "Replies to a broadcast" to show them, and the broadcast's report lists the first 50.

Automatic replies (out of office) are not stored as conversations: they are only counted as
`auto_replied`, so a send does not fill the inbox. Mail from someone other than the recipient, for
example a forward, opens its own conversation.

Members get one push notification per broadcast about its new replies, grouped every 10 minutes, and
no e-mail for them.

## 6. Reports

The broadcast page shows its report as soon as sending starts.

| Number | Meaning |
|---|---|
| Recipients | The audience taken when sending started. |
| Sent | Mails the SMTP server accepted. |
| Failed | Mails refused for good without a bad address. |
| Bounced | Recipients whose mail bounced. |
| Reported as spam | Recipients who complained. |
| Unsubscribed | Recipients who left a list through this broadcast's links. |
| Replied | Recipients who answered; each is a conversation. |
| Automatic replies | Out-of-office answers, counted only. |
| Clicked | Recipients who opened a link; only with click tracking. |
| Skipped | Left out because they were suppressed, opted out, unsubscribed before their mail, or the broadcast was cancelled. |

Rates are shares of the mails sent. The timeline is per hour for the first 72 hours after sending
started, then per day. The recipients list shows each person's state and the SMTP answer for failures;
filter by state, search by address or name, or show only those who replied. The recipient list is
deleted 400 days after the broadcast finished; the counts stay.

### Click tracking

Off by default. Switched on, links in the mail go through `/c/…` on the server's link domain and the
report shows unique clicks per link. Only web links that are the same for every recipient (no merge
field) are tracked, found when the broadcast is scheduled. Clicks are approximate: a `HEAD` request
and three links of one mail opened within two seconds, which is how security scanners behave, are not
counted.

### Campaign tracking (UTM)

On by default. Links to the web in the mail get `utm_source=newsletter`, `utm_medium=email` and
`utm_campaign=<campaign>`, so [Yuva Analytics](analytics.md) counts the visits the mail brings to your website
under the E-mail channel. The campaign is made from the broadcast's name; change it on the draft (1 to 100
lowercase letters, digits, `-`, `_` and `.`). Links that already have a `utm_` parameter of their own, links that
are not `http` or `https` and Yuva's own links (unsubscribe, preferences, clicks) are left alone. Switch it off in
the options, or with `utm: false` on the draft. Broadcasts that were past draft before this existed keep their
links as sent.

The report then shows **visits from this campaign**: per website you may see, the visitors, visits and page views
whose `utm_campaign` is the broadcast's, from the day sending started (with `analytics:read`; `campaign_visits` in
the API). More in [Support and traffic](analytics-support.md#tracking-broadcast-campaigns).

There is no open tracking and no tracking pixel. Open rates are unreliable since mail apps preload
images, and a pixel would be the only thing in the mail that reports to a third party.

## 7. Keys and assistants

Sending is the one thing API keys and OAuth tokens do not get by default:

- The scope `broadcasts:send` is never implied; name it when you create a key.
- Even then, a key or token can schedule and resume broadcasts only while an owner or admin has
  switched on "Keys and assistants may send broadcasts" in Settings → Workspace → Broadcasts. It is off
  by default, and switching it takes a member session. Otherwise scheduling answers
  `403 broadcasts_by_keys_disabled`.
- Creating, changing and testing drafts needs `broadcasts:write`, reading needs `broadcasts:read`.
  Pausing and cancelling need `broadcasts:send` and are not held back by the switch.
- Through an [assistant](mcp.md#lists-designs-and-setup-from-an-assistant), `pause_broadcast`, `resume_broadcast`, `unschedule_broadcast` and `cancel_broadcast` are listed only while the switch is on, like `schedule_broadcast`.

## 8. Triggered e-mails

A triggered e-mail sends one [design](designs.md) once to each contact of an inbox, a set time after the
contact is first identified there: the moment an app or your backend gives the contact its first external id
(through an [identity token](identity.md), `POST /v1/contacts`, or a form that carries one). Use it for a
welcome mail a day after sign-up or a "how is it going" note a week later. It is a broadcast of `kind`
`triggered`, so the design, subjects per language, merge fields, review, test sends, click tracking, pace,
sending hours and daily limit work as described above. Nothing is sent to a list at once: contacts are enrolled one by one
as they are identified.

### Set one up

In the panel, create a triggered e-mail in the inbox instead of a one-off broadcast. Or through the API,
with `kind: "triggered"` and a `trigger`:

```sh
curl -X POST https://support.example.com/v1/broadcasts \
  -H "Authorization: Bearer $YUVA_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "inbox_id": "INBOX-ID",
    "channel_id": "CHANNEL-ID",
    "name": "Welcome",
    "subject": "Welcome, {{ contact.first_name | \"there\" }}",
    "design_id": "DESIGN-ID",
    "kind": "triggered",
    "trigger": { "event": "contact_identified", "delay_minutes": 1440, "basis": "service" }
  }'
```

- **Event.** `contact_identified` (above), `subscribed` or `attribute_changed`, see [Events](#events).
- **Delay.** From 0 minutes to 365 days after the trigger. The mail waits that long, then goes out under
  the pace, the sending hours and the daily limit of broadcasts; if the hours are closed at that time it waits for them.
  Triggered e-mails go out before one-off broadcasts of the same channel.
- **Sending hours.** `timezone` and `send_window` work as for [broadcasts](#sending-pace) and can be changed
  while it is running.
- **Audience.** Conditions and languages (up to 10 and 20) are checked at the contact's due time, not when
  they are identified. A contact who no longer matches is skipped.

### Events

| Event | Needs | Triggers when |
|---|---|---|
| `contact_identified` | Nothing. | The contact gets its first external id in the inbox. |
| `subscribed` | `trigger.list_id`: a list of the inbox (not a status page's). | A subscription to that list is confirmed. The trigger time is the moment it is confirmed; a double opt-in counts when the person confirms, not while it is pending. Subscriptions created by an import never trigger it. |
| `attribute_changed` | `trigger.condition`: a [broadcast condition](#2-write-the-broadcast). | A write makes a contact match the condition when it did not before; a new contact that matches counts. The writes are an identity token, `POST` and `PATCH /v1/contacts` and a subscription that creates its contact. |

```json
{ "event": "subscribed", "list_id": "LIST-ID", "delay_minutes": 60, "basis": "marketing" }
```

`attribute_changed` has no back-fill: it takes only `backfill_days` 0, and contacts that already match
when it starts get nothing. A write enrolls a contact only in the inboxes it belongs to: those where it has an
external id or a conversation, a subscription to one of their lists, or through which the write came. Missing,
misplaced or unfit fields are `400 trigger_invalid`. `skip_triggers` and imports skip all three events.

A list named by a scheduled, active or paused triggered e-mail cannot be deleted
(`DELETE /v1/lists/{listId}` answers `409 list_in_use`); stop the e-mail first.

### Marketing or service

The consent basis is chosen when you create it and cannot change afterwards.

| Basis | Goes to | Footer | Needs |
|---|---|---|---|
| `marketing` | Only contacts subscribed to its 1 to 10 `audience.list_ids`. | The usual broadcast footer, with one-click unsubscribe from the list. | Lists. |
| `service` | Every matching contact, no lists. | A footer saying the person uses the inbox's product, your postal address and a one-click stop. | `confirm_service: true` when you start it. |

Choose `service` only for mail about using the product: getting started, the account, a request for
feedback. Advertising and offers are always `marketing`. A `service` e-mail cannot use the `list.name` merge field.
Starting a `service` e-mail without `confirm_service: true` answers `400 service_basis_unconfirmed`; Yuva stores
who confirmed and when, and the audit log records the start.

The one-click stop of a service footer stops the inbox's service triggered e-mails and
[designed outreach](designs.md#8-use-a-design-in-a-conversation) to that person; it does not unsubscribe them from any list.
Both bases skip addresses that bounced or complained and people who objected to all mail from the workspace.

### Start, back-fill and stop

Starting works like scheduling (`POST /v1/broadcasts/{broadcastId}/schedule`): at `send_at` (it is
`scheduled` until then) or now it becomes `active`. The review must pass first. From then on every contact
whose trigger happens is enrolled once, and a contact is never enrolled twice.

Contacts identified before the start get nothing, unless you **back-fill**: `backfill_days` (0 to 30, on the
start or in `trigger`) also enrolls contacts whose trigger happened in that many days before the start. Their
delay counts from the trigger, but nobody is due earlier than the start. The review counts them, and
`confirm_recipients` must equal that count (0 without back-fill).

Contacts you copy in from your own backend are a trigger too: a user the product already had would get a
"welcome" mail. Send `"skip_triggers": true` with `POST /v1/contacts` or `PATCH /v1/contacts/{contactId}` so that
the write enrolls nobody. Imports, help-desk migrations and merges never trigger.

| Action | What it does |
|---|---|
| Pause (`POST .../pause`) | Stops sending. New triggers are still enrolled and wait. |
| Resume (`POST .../resume`) | Runs the review again and freezes the design as saved now. Contacts whose due time passed more than 7 days before are skipped as `late`. |
| Stop (`POST .../cancel`) | Stops for good. It no longer enrolls and drops the contacts waiting. A stopped e-mail cannot be started again; make a new one. |

While it is **active** you can change the pace, time zone, sending hours, the delay (it moves the due time of
everyone waiting) and click tracking. While it is **paused** you can also change the design, subjects, preheader,
translations, conditions and languages; resuming reviews it again. The event, its list or condition, the basis, the lists and the channel
never change after the draft; make a new e-mail for that. Pause, resume and stop need `broadcasts:send`.

### Who waits

The broadcast page lists the contacts it enrolled: waiting with the time their mail is due, mailed, and skipped
with the reason. Through the API, `GET /v1/broadcasts/{broadcastId}/enrollments` lists them (`state`:
`waiting`, `queued`, `skipped`, `cancelled`), the one due first first for `waiting`. To keep one waiting contact from getting the mail,
`DELETE /v1/broadcasts/{broadcastId}/enrollments/{contactId}` takes them off: they are skipped as `removed` and
never enrolled again. Only a waiting contact can be removed (`409 invalid_state` otherwise).

| Skip reason | Meaning |
|---|---|
| `no_address` | The contact has no e-mail address. |
| `blocked` | You blocked the contact. |
| `suppressed` | The address bounced or complained. |
| `opted_out` | The person objected to all mail from the workspace. |
| `stopped` | `service`: the person stopped the inbox's mail like this. |
| `not_subscribed` | `marketing`: on none of its lists. |
| `not_matching` | A condition or language does not match. |
| `late` | Due more than 7 days before it could be processed, after a long pause or the daily limit. |
| `removed` | Taken off the waiting list. |

The report adds contacts enrolled, waiting and the skips by reason. Counts are brought up to date within
5 minutes. No event or webhook is sent per enrolled contact.

At most 20 triggered e-mails of an inbox can be scheduled, active or paused at the same time
(`409 too_many_triggered`). A merge field without a fallback is only a warning in the review, since the contacts are not
known yet.
## Approval on hosted servers

A server that offers sign-up to anyone (`YUVA_SIGNUP=open`) runs broadcasts in `approval` mode by
default, so one careless workspace cannot damage the sending reputation of the whole server. If you
run your own server with invitations only, this does not apply to you.

In `approval` mode, a workspace created through sign-up cannot schedule broadcasts until the operator
approves it. You can write drafts and send tests meanwhile.

1. Open Settings → Workspace → Broadcasts. An owner sees the status: allowed, needs the operator's
   approval, waiting for the operator, or suspended.
2. Choose Ask for approval and fill in your website, who sends and what the mails are about, and how
   your subscribers agreed. The operator reads exactly this.
3. The status becomes "Waiting for the operator". The operator approves, or denies with a reason you
   see; you may ask again after a denial. The review shows "Permission to send" as blocking until then.

Approved workspaces, and every workspace on an `approval` server, warm up: they may send to 500
recipients on the first day, and the limit doubles after each clean day (at least 100 mails sent,
bounces under 3%, complaints under 0.1%). A bad day takes one doubling back. A workspace also has at
most 1,000 broadcasts on such a server; delete old ones.

On a server where broadcasts are `off`, lists and forms keep working, but creating, changing and
sending broadcasts answers `403 broadcasts_disabled`.

## Deliverability

Mail from a new sender lands in spam unless the basics are right. The [Deliverability guide](deliverability.md)
covers them: the check that tests a channel's SPF, DKIM and DMARC, setting up a domain with Amazon SES,
Postmark or another relay, Yuva DKIM, the rules of Gmail and Yahoo, Google Postmaster Tools and warming up.

- **Only mail people who agreed.** A list built from consent records is your best protection.
- **Start small.** Mail a few hundred engaged subscribers first and grow.
- **Use a separate account** for broadcasts, from a subdomain such as `news.example.com` if you like.

Yuva sets `List-Unsubscribe` with one-click (RFC 8058), `List-Id`, `Feedback-ID` and
`X-Auto-Response-Suppress` on every broadcast mail, so mail programs show their own unsubscribe button.

## The API

The panel uses the same API as everything else. A broadcast from a script:

```sh
curl -X POST https://support.example.com/v1/broadcasts \
  -H "Authorization: Bearer $YUVA_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "inbox_id": "INBOX-ID",
    "channel_id": "CHANNEL-ID",
    "name": "October newsletter",
    "subject": "What is new in October",
    "design_id": "DESIGN-ID",
    "audience": { "list_ids": ["LIST-ID"] },
    "pace": { "per_hour": 10, "per_day": 100 }
  }'

```

`PATCH` with `pace` alone changes the pace of a scheduled or running broadcast; `{}` removes it.

Then `GET /v1/broadcasts/{broadcastId}/review` for the checklist, `POST .../audience` for the recipient
count and `POST .../test` for a test mail to a member (an API key names them in `member_ids`). Keys
need the [scopes below](#scopes). Schedule it, with `confirm_recipients` set to the count you saw:

```sh
curl -X POST https://support.example.com/v1/broadcasts/BROADCAST-ID/schedule \
  -H "Authorization: Bearer $YUVA_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"send_at": "2026-10-20T08:00:00Z", "confirm_recipients": 1280}'
```

## Reference

### Endpoints

| Endpoint | Use |
|---|---|
| `GET`, `POST /v1/broadcasts` | List (`inbox_id`, `state`, `kind`) and create drafts or [triggered e-mails](#8-triggered-e-mails) (`kind: "triggered"`, `trigger`). |
| `GET`, `PATCH`, `DELETE /v1/broadcasts/{broadcastId}` | Read, change a draft (or only the pace of a scheduled or running one), delete a draft or finished broadcast. |
| `POST /v1/broadcasts/{broadcastId}/audience` | Count the audience: recipients, who is left out and why, merge fields that would be empty. |
| `GET /v1/broadcasts/{broadcastId}/review` | The checklist. |
| `POST /v1/broadcasts/{broadcastId}/test` | Send a test to members (`member_ids`, `contact_id`). |
| `POST /v1/broadcasts/{broadcastId}/schedule` | Schedule or send now (`send_at`, `confirm_recipients`). |
| `POST /v1/broadcasts/{broadcastId}/unschedule` | Back to draft, before it starts. |
| `POST /v1/broadcasts/{broadcastId}/pause`, `resume`, `cancel` | Control a running broadcast. |
| `GET /v1/broadcasts/{broadcastId}/deliveries` | The recipients (`state`, `q`, `replied`). |
| `GET /v1/broadcasts/{broadcastId}/enrollments` | A triggered e-mail's contacts (`state`): who waits and when, who was mailed, who was skipped. |
| `DELETE /v1/broadcasts/{broadcastId}/enrollments/{contactId}` | Take a waiting contact off a triggered e-mail. |
| `GET /v1/broadcasts/{broadcastId}/report` | Counts, rates, timeline, links and replies. |
| `GET /v1/conversations?broadcast_id=` | Conversations opened by replies to a broadcast. |
| `PATCH /v1/workspace` | `broadcasts_by_keys`, the switch for keys and assistants. |
| `POST /v1/workspace/broadcast-approval` | Ask the operator for approval (owners, member session). |

`GET /v1/workspace` returns a `broadcasts` object with the server's `mode`, the workspace's `status`,
`daily_limit`, `sent_today` and `by_keys`. A channel's `email.broadcast_smtp` and `email.broadcast_rate`
are set with `PATCH /v1/channels/{channelId}` (`broadcast_smtp_remove: true` removes the account).
Full shapes are in the [API contract](../openapi/openapi.yaml). Errors worth handling:
`409 review_failed` (with the blocking `items`), `409 audience_changed`, `409 not_draft`,
`409 invalid_state`, `409 pace_too_slow`, `403 broadcasts_approval_required`, `403 broadcasts_suspended`,
`403 broadcasts_by_keys_disabled`, `403 broadcasts_disabled` and `429 test_limit`.

### Scopes

| Scope | Meaning |
|---|---|
| `broadcasts:read` | Read broadcasts, audience counts, reviews and reports; deliveries and the enrollments of a triggered e-mail also need `contacts:read`. |
| `broadcasts:write` | Create, change and delete drafts; send tests; take a contact off a triggered e-mail. |
| `broadcasts:send` | Schedule, unschedule, pause, resume and cancel. Never implied. |

Keys made before these scopes existed do not get them.

### Events

| Name | Meaning |
|---|---|
| `broadcast.updated` | [Webhook](webhooks.md) and event-feed event: the broadcast changed state or its pace. `data.broadcast` is the broadcast, `data.previous_state` the old state. |
| `broadcast.progress` | Realtime only (`/v1/realtime`): the counts of a preparing or sending broadcast, at most every 5 seconds. Not stored or replayed; reload the broadcast after a reconnect. |

Both reach callers with `broadcasts:read` who can see the broadcast's inbox. Replies add a
`broadcast_reply` entry to the conversation's timeline.

### Limits

| Limit | Value |
|---|---|
| Lists per broadcast | 10 |
| Conditions, languages | 10, 20 |
| Name, subject, preview text | 200, 300, 300 characters |
| Scheduling ahead | 90 days |
| Test sends | 20 an hour per workspace; 1 to 5 members each |
| Pause and cancel act within | One batch, at most 50 mails |
| Pace | 1 to 100 mails a second per channel; 5 by default |
| Broadcast pace | 1 to 10,000,000 an hour, a day, a 30-day month; at most 365 days per send |
| Retries of a temporary failure | After 5 minutes, 30 minutes, 2 hours, 6 hours |
| HTML | 90 KB warning, 102 KB blocks, 500 KB refused |
| Broadcasts per workspace | 1,000 on an `approval` server |
| Recipient list kept | 400 days after the broadcast finished |
| Triggered e-mails per inbox | 20 scheduled, active or paused |
| Delay of a triggered e-mail | 0 minutes to 365 days |
| Back-fill | 0 to 30 days |
