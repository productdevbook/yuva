# Status pages

A status page tells your customers whether your product works: components with a state, incidents,
planned maintenance and a history. It belongs to an inbox, so the live chat and the apps of that inbox
show an open incident before a customer writes, and the conversations an outage brings can be answered
together.

The public page is plain HTML without a panel and without required JavaScript, rendered by your Yuva
server. Each inbox has at most one status page.

## 1. Create and publish a page

In the panel: Status (shortcut `g s`), then Create status page on the inbox. Owners and admins create
and change pages; agents with access to the inbox work with components and incidents.

| Setting (API field) | Meaning |
|---|---|
| Name (`name`) | Up to 100 characters. |
| Description (`description`) | Up to 1,000 characters, shown under the name. |
| Address (`slug`) | 3 to 48 characters: `a-z`, `0-9` and inner hyphens. Unique on the server; some words are reserved. |
| Logo (`logo_url`), accent colour (`accent_color`) | An `https` image and a colour such as `#2563eb`. |
| Dark mode logo (`logo_dark_url`) | Optional. An `https` image shown instead of the logo when the visitor's system is in dark mode, such as a light version of a dark logo. Used only together with a logo. |
| Site link (`site_url`), support link (`support_url`) | Your product, and an `http(s)` or `mailto:` link where customers get help. |
| Language (`locale`) | `en`, `tr` or `de`; the page's words follow it. Other languages show English. |
| Time zone (`timezone`) | The zone of the day bars. Visitors see times in their own zone. |
| Days of history (`history_days`) | 7 to 90, default 90. |
| Show uptime (`show_uptime`) | A percentage next to each component. On by default. |
| Show response times (`show_response_times`) | The last 24 hours of response times for components with an `http` or `tcp` monitor, with the median and p95. Off by default. |
| Show the name next to the logo (`show_name`) | On by default. Turn it off when the logo already contains the name: the header then shows only the logo, with the name as its alt text. Without a logo the name always shows. |
| Show feed, badge and embed (`show_follow`) | On by default. Off leaves out the Follow and embed section, its link in the header, the `feed.atom` and `badge.svg` links in the footer and the feed link on incident pages. The feed, badge and JSON addresses keep answering. |
| Published (`published`) | Off by default. An unpublished page answers `404` to everyone but your team, which previews it in the panel. |

Renaming the address keeps the old one: it stays reserved for the page and every public address under
it redirects to the new one.

Over the API: `POST /v1/status-pages` with `inbox_id`, `name` and `slug`, then `PATCH
/v1/status-pages/{pageId}` with `{"published": true}`. `GET /v1/status-pages/{pageId}/preview` returns
the page as visitors would see it, also while it is unpublished.

## 2. Components, groups and states

Components are the parts of your product that customers depend on: "Website", "API", "Checkout". Add
them on the Components tab, optionally inside groups, and order them. A page has at most 100.
A hidden component is left off the page and out of its overall state.

| State | Meaning |
|---|---|
| `operational` | Working. |
| `degraded` | Slow or partly impaired. |
| `partial_outage` | Part of the users or features are down. |
| `major_outage` | Down. |
| `maintenance` | Planned work. |

The page's overall state is the worst visible component state; `maintenance` counts only when nothing is
worse. Uptime for a day is the share of the day not in `major_outage`, with `partial_outage` counting as
half and `maintenance` left out. A day bar shows the worst state of its day.

Set a state by hand in the panel or with `POST /v1/status-pages/{pageId}/components/{componentId}/state`
(`{"state": "degraded"}`). An open incident, a failing [monitor](#5-heartbeat-and-push-monitors) or you
can hold a component; the panel says who ("Held by an incident", "Set by a monitor").

Over the API: `/v1/status-pages/{pageId}/components` and `/v1/status-pages/{pageId}/groups`.

## 3. Incidents and maintenance

On the Incidents tab, Open an incident reports a problem and Plan maintenance announces work. Both have
a title, the components they touch with the state each gets while it is open, and a first update.

| Kind | Impact | States |
|---|---|---|
| Incident (`incident`) | `none`, `minor`, `major`, `critical` | `investigating`, `identified`, `monitoring`, `resolved` |
| Maintenance (`maintenance`) | the same, `none` by default | `scheduled`, `in_progress`, `completed` |

- **Draft or published.** Save as draft keeps it visible to your team only and changes no component.
  Publish shows it on the page, sets the components and, unless you untick Notify subscribers, queues a
  [notice](#8-e-mail-subscribers). Only a draft can be deleted.
- **Updates.** Post update adds text and can move the state. Text is plain, with links and line breaks,
  at most 5,000 characters. A published update cannot be removed; correct it with a new update, or edit
  it, which shows "edited".
- **Resolving.** Resolve (or completing maintenance) puts each touched component back to `operational`,
  unless another open incident or a failing monitor still holds it.
- **Maintenance windows.** Maintenance needs a start and an end. Yuva starts and completes it on time,
  unless you do it first, and posts "The maintenance has started." and "The maintenance is complete."
  in the page's language.

Over the API: `POST /v1/status-pages/{pageId}/incidents` (add `"publish": true` to skip the draft),
`PATCH /v1/status-pages/{pageId}/incidents/{incidentId}`, `POST …/incidents/{incidentId}/publish`,
`POST …/incidents/{incidentId}/updates` and `PATCH …/updates/{updateId}`. A resolved incident takes no
more changes (`409 incident_closed`).

```sh
curl -X POST https://support.example.com/v1/status-pages/$PAGE/incidents \
  -H "Authorization: Bearer $YUVA_API_KEY" -H "Content-Type: application/json" \
  -d '{"title": "Checkout is failing", "message": "We are looking into it.", "impact": "major",
       "components": [{"component_id": "'$COMPONENT'", "state": "major_outage"}], "publish": true}'
```

## 4. The public page

A published page is at `<status URL>/{slug}`, with the overall state, open incidents and maintenance,
the components with day bars, and the incidents of the last 14 days.

| Address | What it is |
|---|---|
| `/status/{slug}` | The page. |
| `/status/{slug}/history` | Past incidents by month. |
| `/status/{slug}/incidents/{id}` | One incident or maintenance with its updates. |
| `/status/{slug}/feed.atom` | An Atom feed of incidents. Needs no subscription. |
| `/status/{slug}/badge.svg` | The overall state as a badge, for a README. |
| `GET /client/v1/status/{slug}` | The same as JSON, for your own site or app. Open to any origin, no credentials. |
| `/status/{slug}/subscribe` | The [e-mail subscribe form](#8-e-mail-subscribers). |

The status URL is `YUVA_PUBLIC_URL` plus `/status`, such as `https://support.example.com/status/acme`.
To serve pages from their own host, set `YUVA_STATUS_URL` to an origin such as
`https://status.example.com`; the paths are then `https://status.example.com/acme`,
`/acme/history` and so on, with no `/status` prefix, and the old addresses redirect there. The host must
reach your Yuva server and differ from `YUVA_PUBLIC_URL`. See [Configuration](configuration.md#status-pages).

The page refreshes itself every minute and is also complete without JavaScript. Answers are cached for
30 seconds, so a CDN in front of Yuva can serve visitors while your product is down. There are 120
requests a minute per address.

To embed the page, put it in an `iframe`; to show its state elsewhere, use the badge or the JSON:

```html
<iframe src="https://support.example.com/status/acme" title="Status" height="600"></iframe>
<img src="https://support.example.com/status/acme/badge.svg" alt="Status">
```

```sh
curl -s https://support.example.com/client/v1/status/acme
```

The inbox settings a widget or app loads carry the linked page's slug as `status.slug`, so a client can fetch this JSON, also on a custom domain, without knowing the slug beforehand.

## 5. Heartbeat and push monitors

A monitor sets a component's state without anyone's hand. Add one on the Monitors tab (owners and
admins): pick the component and a kind. Yuva shows the monitor's address once, with copy-paste
snippets. Anyone who has the address can report for the monitor, so keep it secret. Make new address
replaces it and the old one stops working at once.

### Heartbeat: your job calls Yuva

For servers and cron jobs that Yuva cannot reach. The job calls the address at least every `interval`
(30 seconds to 30 days, default 5 minutes). A beat that is missing for `interval` plus `grace` (up to a
day, default 1 minute) fails the monitor.

```sh
curl -fsS -m 10 --retry 3 -o /dev/null https://support.example.com/ingress/heartbeat/TOKEN
```

In a crontab, beat only when the job succeeded:

```sh
*/5 * * * * /path/to/job.sh && curl -fsS -m 10 --retry 3 -o /dev/null https://support.example.com/ingress/heartbeat/TOKEN
```

Report a failed run at once, without waiting for the grace:

```sh
curl -fsS -m 10 -o /dev/null "https://support.example.com/ingress/heartbeat/TOKEN?fail=1"
```

`GET` and `POST` do the same, and `/ingress/heartbeat/TOKEN/fail` is the same as `?fail=1`.

### Push: your monitoring sends the state

Send `operational` to recover, or `degraded`, `partial_outage` or `major_outage` to fail. The state you
push is the one the component gets; `message` (up to 1,000 characters) is kept with the check.

```sh
curl -fsS -X POST -H "Content-Type: application/json" \
  -d '{"state":"major_outage","message":"Checkout is failing"}' \
  https://support.example.com/ingress/status/TOKEN

curl -fsS -X POST -H "Content-Type: application/json" \
  -d '{"state":"operational"}' https://support.example.com/ingress/status/TOKEN
```

Alertmanager and Grafana alerts are accepted as they are: firing fails the monitor, resolved recovers it.

```yaml
receivers:
  - name: yuva-status
    webhook_configs:
      - url: https://support.example.com/ingress/status/TOKEN
        send_resolved: true
```

In Grafana, add a contact point of type Webhook, paste the address, keep the method `POST` and use the
contact point in a notification policy.

### Settings, thresholds and overrides

| Setting (API field) | Meaning |
|---|---|
| State while down (`failure_state`) | `degraded`, `partial_outage` or `major_outage` (default). A pushed state that names its own state keeps it. |
| When it goes down (`on_failure`) | Open a draft incident (`draft`, default), a published one (`publish`) or none (`none`). |
| Failures to go down (`failure_threshold`) | Failures in a row before the monitor is down, 1 to 10, default 1. |
| Successes to recover (`recovery_threshold`) | Successes in a row before it is up again, 1 to 10, default 1. |
| Paused (`paused`) | Ignores results and lets the component go back. Resuming starts again from `pending`. |

Your job or monitoring already decided what a failure is, which is why the thresholds start at 1.

A monitor never overrides your hand: if a member or an API key set the component's state after the
monitor's last change, the monitor leaves it alone until its own state changes again. While a monitor
is down, its incident follows the state it reports, and recovering resolves that incident and puts the
component back, unless something else holds it. Pausing or deleting a monitor does the same. The panel
shows a component that a member set as holding.

When a monitor goes down or up, the members of the inbox are notified like for a new conversation, and
`status.monitor.down` and `status.monitor.up` are sent to [webhooks](webhooks.md). The Monitors tab
lists recent checks, which are kept 30 days. A workspace has at most `YUVA_STATUS_MONITORS` monitors
(20 by default, `403 status_monitor_limit`). A token takes 60 calls a minute.

Over the API: `/v1/status-pages/{pageId}/monitors`, `POST …/monitors/{monitorId}/token` for a new
address and `GET …/monitors/{monitorId}/checks`.

## 6. Status in the widget and the apps

While the page is published and an incident or maintenance is open, the [web widget](widget.md) and the
iOS and Android [SDKs](mobile.md) show a line above the conversation list and the composer, with the
state in words and a link to the incident. The worst open one is shown first. A customer can dismiss
it; it comes back when the incident gets a new update. Nothing to set up.

## 7. Link conversations to an incident

An outage brings many conversations. In the panel, open a conversation and link it to an incident
(or use `PUT /v1/status-pages/{pageId}/incidents/{incidentId}/conversations/{conversationId}`).
The incident's page lists its linked conversations and their states; an incident takes at most 5,000.

Reply to linked writes one answer and sends it to every linked conversation that is still open, as you,
through each conversation's own channel (e-mail, chat or app). Tick the box to close each conversation
afterwards. Review shows how many conversations get it. It cannot be recalled once sent.
The panel reports per conversation whether it was sent, skipped (closed, unlinked, deleted or your access
ended) or failed.

Over the API: `POST …/incidents/{incidentId}/reply` answers `202` with an id, and
`GET …/incidents/{incidentId}/replies/{replyId}` reports the outcome. A key needs `status:manage`,
`conversations:read` and `messages:write` (and `conversations:write` to close), and the workspace must let
bots send ([Bots may send](headless.md#bots-may-send)).

## 8. E-mail subscribers

Visitors can subscribe to notices about incidents and maintenance. Turn it on in Settings of the page:
Send notices by e-mail, and the e-mail channel that sends. The channel must be able to send
(`409 sender_not_ready` otherwise). The page warns, without blocking, when the sending domain has no
passed deliverability check, the inbox has no postal address or the mail is not DKIM-signed with the
sender's own domain; see [Deliverability](deliverability.md).

- **Subscribing.** The page links to its subscribe form: an address, one checkbox per component (all
  ticked; untick to hear only about some) and the sentence the visitor agrees to. It always uses double
  opt-in: the answer is the same for every address, and a confirmation mail follows. A subscriber who
  chose no components gets everything, also for components you add later. Subscribers change their
  choice or leave through the link in every mail.
- **What a notice contains.** The state in words (Investigating, Identified, Monitoring, Resolved,
  Scheduled, In progress, Completed), the impact, the affected components with their states, the updates
  since the last notice with times in the page's zone, a link to the incident, and a link to choose
  components or unsubscribe. It is in the subscriber's language. Subjects read `[Page] Title` and
  `[Page] Maintenance: Title`, and notices of one incident share a subject and thread. It has no
  images, tracking or promotion. Notices go out through the inbox's e-mail channel, from its address.
- **When it is sent.** A new published incident notifies by default; an update notifies when it changes
  the state or the window or resolves, and not when it only adds text. Change it with Notify
  subscribers on the incident or update (it shows how many it would reach), or `notify` over the API.
  Component changes alone and drafts send nothing.
- **Hold and undo.** A notice waits 60 seconds so you can fix a typo; updates posted meanwhile join it.
  Undo in the incident's timeline (or `POST /v1/status-pages/{pageId}/notices/{noticeId}/cancel`)
  stops it before it is sent (`409 not_pending` after).
- **Gap and hourly limit.** Notices about one incident are at least 15 minutes apart, and a page sends
  at most 10 an hour; later ones are skipped and the timeline says so.
- **Final notices.** The notice that an incident is resolved or maintenance has started or completed
  ignores the gap and the hourly limit. It goes only to people who got an earlier notice of that incident:
  with none sent before it, it is skipped (`nothing_earlier`), because nobody heard of the incident.
- **Limits.** `YUVA_STATUS_NOTICES`, `YUVA_STATUS_NOTICES_DAILY` and `YUVA_STATUS_MAX_SUBSCRIBERS` on the
  server; see [Configuration](configuration.md#status-pages). Subscribing is limited to 10 an hour per
  visitor address and 300 an hour per page. Unconfirmed subscriptions are removed after 30 days.

The Subscribers tab lists addresses with their state and chosen components, adds an address (which
sends the confirmation), removes one and exports the consent records. Subscribers belong to a mailing
list of the inbox with kind `status`; it is never offered in forms, broadcasts or CSV imports, and
leaving it does not opt a person out of anything else. The notices list on the page is at
`GET /v1/status-pages/{pageId}/notices`.

## API, webhooks and MCP

| Scope | Allows |
|---|---|
| `status:read` | Pages, components, incidents, notices, monitors and their checks. |
| `status:manage` | Create and change all of that, post updates, set states, link conversations. It does not include `status:read`: give a key both. |

Pages, groups, components and monitors are for owners and admins; agents and keys with the scope also
work with states, incidents and notices of the inboxes they can see. The whole surface is in the
`status` section of [openapi.yaml](../openapi/openapi.yaml).

[Webhooks](webhooks.md) can subscribe to `status.component.updated`, `status.incident.created`,
`status.incident.updated`, `status.incident.resolved`, `status.monitor.down` and `status.monitor.up`.
Drafts are not sent to webhooks.

[AI assistants (MCP)](mcp.md) get `list_status_pages`, `get_status_page`, `list_monitors`,
`set_component_state`, `create_incident` (always a draft that a member publishes), `post_incident_update`,
`resolve_incident` and `link_conversation_to_incident`.
