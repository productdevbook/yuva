# AI assistants (MCP)

Yuva has a built-in [MCP](https://modelcontextprotocol.io) server. Assistants such as Claude,
ChatGPT, Cursor, VS Code and Gemini CLI can search your inbox, read and triage conversations and
draft replies, with the access you give them.

## 1. Pick how the assistant signs in

The endpoint is `/mcp` on your server, for example `https://support.example.com/mcp`.

- **OAuth**: the assistant acts as you, with no more access than you have.
- **API key**: for automations without a person. Make a key in Settings → API keys with the
  [scopes](#tools) it needs and a bot name such as "Claude" ([API keys](headless.md#api-keys)).

## 2. Add the server to your assistant

In Claude Code, with OAuth:

```sh
claude mcp add --transport http yuva https://support.example.com/mcp
claude mcp login yuva
```

With an API key instead, add `--header "Authorization: Bearer $YUVA_API_KEY"` to the first command.
Other assistants:

| Assistant | How |
|---|---|
| claude.ai, Claude Desktop | Customize → Connectors → **Add custom connector**, enter the `/mcp` URL, sign in. Needs a public `https` URL ([guide](https://claude.com/docs/connectors/custom/add-unlisted)). |
| Claude Desktop, private server | Open `yuva.mcpb` from a release; it asks for the server URL and an API key ([bundle](#local-clients)). |
| ChatGPT | [Plugins](https://chatgpt.com/plugins) → plus → **Add custom MCP server**, OAuth only. Needs a public `https` URL ([guide](https://developers.openai.com/plugins/deploy/connect-chatgpt)). |
| Cursor | `.cursor/mcp.json`: `{"mcpServers": {"yuva": {"url": "https://support.example.com/mcp"}}}`; add `headers` for a key ([docs](https://cursor.com/docs/context/mcp)). |
| VS Code | **MCP: Add Server**, or `.vscode/mcp.json` with `"type": "http"` and the `url`; add `headers` for a key ([docs](https://code.visualstudio.com/docs/copilot/customization/mcp-servers)). |
| Gemini CLI | `gemini mcp add --transport http yuva https://support.example.com/mcp`, then `/mcp auth yuva`; or `--header` for a key ([docs](https://geminicli.com/docs/tools/mcp-server/)). |
| MCP Inspector | `npx @modelcontextprotocol/inspector`, to see every tool, resource and prompt. |

## 3. Ask

For example: "List my Yuva inboxes and draft a reply in Turkish to the open conversation in the
Turkish inbox."

Replies are drafts. A member reviews each one in the conversation and sends, edits or discards it,
unless the workspace lets bots send ([Bots may send](headless.md#bots-may-send)). What an assistant
writes over OAuth shows as "Ayşe via Claude"; with an API key, as the key's bot.

## Add Yuva to a project with init

Open your assistant in the project's folder and run the `init` prompt (in Claude Code
`/mcp__yuva__init`), or ask: "Look at this project and tell me how to add Yuva." The assistant reads
the project on your machine (framework, sign-in, documentation site, mobile apps, the support and
analytics tools you use today), calls `get_setup_state` to see what your workspace already has, and
answers with a short plan: what each step gives the project, the evidence from the code and the
prompt or tools that do it. Nothing changes until you pick steps.

Your code and files stay with the assistant: `init` sends nothing from the project to Yuva. Steps
that create inboxes, channels or analytics properties need the scope they name; `get_setup_state`
lists what the connection cannot read and the scope to grant.

## Report a problem with Yuva

When Yuva itself misbehaves while an assistant works with it (a server error, an SDK that does not
do what its guide says, a wrong guide), the assistant can call `report_yuva_issue`. The server
writes a GitHub issue for [productdevbook/yuva](https://github.com/productdevbook/yuva/issues) with
your workspace's names, its sites and addresses, e-mail addresses, other URLs and host names, IP
addresses, keys, tokens, ids and home-directory paths removed, and returns the text with a
prefilled link and a `gh issue create` command. It files nothing itself: the assistant shows you
the report, and you open it under your own GitHub account. Security problems go to
[private reporting](https://github.com/productdevbook/yuva/security/advisories/new) instead.

## Set up live chat from an assistant

Owners and admins can let an assistant do the setup. Grant `inboxes:manage` when you connect
(an API key needs it too), then ask, for example: "Connect Yuva live chat to https://www.example.com.
It's the Acme site, built with Next.js."

The `setup_live_chat` prompt does the same. The assistant finds or creates the inbox, creates the
chat channel for the site's origin (or adds the origin to the one there is) and gets the install
code, filled with your server's URL and the channel's public key. With access to your code it adds
the code there; otherwise it shows it. A connection without `inboxes:manage` gets an error naming
the scope: reconnect and grant it. The identity secret for [signed-in users](identity.md) is not
given to assistants; rotate it in the inbox settings when you need it.

## Draft newsletters from an assistant

An assistant can write a newsletter into a design and a draft broadcast, send you a test and read the
report afterwards. It cannot send to your subscribers unless you allow it: scheduling is a separate
step that you control.

- Grant `lists:read`, `broadcasts:read` and `broadcasts:write` when you connect (an API key needs them
  too). The `draft_newsletter` prompt starts from a brief and never schedules.
- `get_broadcast` shows the same review as the broadcast page: each check with ok, warning or blocking.
  The assistant fixes design problems with `update_design` and everything else on the draft (subject,
  preheader, channel, lists, click tracking, UTM tags, pace) with `update_broadcast`, which answers with
  the review again. Approval and the channel's domain checks stay with you.
- A test sent through a connected assistant goes only to you. An API key names the members who get it.
- `schedule_broadcast` is listed only when the key or token has `broadcasts:send` **and** an owner or
  admin has switched on "Keys and assistants may send broadcasts" ([Keys and
  assistants](broadcasts.md#7-keys-and-assistants)). It needs `send_at` and `confirm_recipients`, the
  audience count that `get_broadcast` showed and you confirmed. If the audience changed since, nothing
  is scheduled.

An example:

> **You:** Draft a newsletter to the "Product news" list from the newsletter template: version 2.1 is
> out, with dark mode and faster search. Link to the changelog.
>
> **Assistant:** I made the design "Release 2.1" from the newsletter template and the draft broadcast
> "Release 2.1" with the subject "Dark mode and faster search are here". The review passes. The
> audience is 1,248 people. Open the draft in Yuva: *link*.
>
> **You:** Send me a test.
>
> **Assistant:** Sent to you. Check how it looks in your mail app.
>
> **You:** Good. Schedule it for tomorrow at 9:00.
>
> **Assistant:** It goes to 1,248 people, as the draft says. Shall I schedule it for tomorrow at 9:00?
>
> **You:** Yes.
>
> **Assistant:** Scheduled for tomorrow at 9:00.

The assistant schedules only after you confirm the count, so ask for the number if it does not show it.

## Analytics from an assistant

With `analytics:read`, an assistant reads the numbers of your websites and apps ([Analytics](analytics.md)) and, for a
website linked to an inbox, the support its pages lead to ([Support and traffic](analytics-support.md)). Ask, for
example: "Which pages lead to the most conversations this month?" The assistant lists your websites, calls
`analytics_support_report` with `period: month` and `order_by: conversations`, and answers with each page's
conversations, page views and conversations per 1,000 views.

- The `analytics_review` prompt reviews a website over a range: headline numbers against the range before, sources,
  pages, goals and, with an inbox, the support per page, ending with suggestions. The `weekly_report` prompt adds a
  support-and-traffic section when the assistant may read analytics.
- The `setup_analytics` prompt (needs `analytics:manage`) creates a website or app, gets the install code and adds it
  to your project, as `setup_live_chat` does for chat.
- Page paths, referrers, campaigns, event names and prop values come from visitors' browsers and are returned only in
  `customer_content`, like customer text. Analytics tools are listed only while the server has analytics on.

## Move a mailbox from an assistant

With the scope `migrations:manage`, run the `move_to_yuva` prompt with the mailbox address. The assistant
detects the provider, creates the migration and gives you a Yuva page to enter the app password; it never
asks for one in the chat. It starts the copy only after you confirm the dry run, then explains how to
forward the mail and finishes the migration. The steps are in [Move to Yuva](migrate.md).

## Help center from an assistant

With the `help:read` and `help:write` scopes an assistant can search a [help center](help-center.md),
read its articles and write drafts. Ask: "Find what customers searched for in the last 30 days
without finding an answer and draft the three most important articles." The `help_gaps_review` prompt
does this; `write_help_article` drafts one article from a conversation or a topic, after searching so
nothing is written twice. An assistant never publishes: it saves drafts, and a member reads and
publishes them in Yuva. In `draft_reply`, `article_ids` attaches up to three published articles to the
reply as cards.

## Roadmap from an assistant

With `roadmap:read`, `roadmap:write` and `conversations:read` an assistant can list the ideas of a
[roadmap board](roadmap-board.md) and link feedback to them. The `link_feedback_to_ideas` prompt goes
through feedback that is not linked yet, searches the board and proposes links; it changes nothing until
you agree and it never moves a stage. Moving a stage tells customers: `move_idea_stage` is listed only to a
key or grant that names the scope `roadmap:stage`, which no other scope implies. It also needs
`confirm_linked`, the number of linked feedback that `get_roadmap_idea` showed (`would_change`) for that
stage, which you have seen; any other number answers `409 confirm_mismatch` and nothing moves. No tool
creates, edits or moderates ideas or sends a reply to linked feedback.

## What's new from an assistant

With the `changelog:read` and `changelog:write` scopes an assistant can read a [What's new](whats-new.md) changelog and write entries as drafts. Give the `draft_release_notes` prompt your commit subjects or release notes (Yuva fetches nothing itself) and it drafts one entry per change a customer would notice, labelled new, improved or fixed. An assistant never publishes, schedules, unpublishes or mails an entry, and cannot change a scheduled one: a member reads the drafts and publishes them in Yuva.

## Lists, designs and setup from an assistant

Beyond drafting, an assistant can do the ordinary steps that come before a newsletter or a new product page,
with the role and scope each tool names in the [table below](#tools). Tools you cannot use are not listed.

- **Sender and channel.** `update_inbox_sender` sets the legal name, postal address, registration and contact that
  every broadcast footer shows. `create_email_channel` adds an address without an SMTP account: the answer gives the
  address to forward mail to and a link where you set SMTP yourself. `create_dkim_key` and `get_deliverability` give the
  DNS record to add and what the checks found.
- **Lists and subscribers.** `create_mailing_list`, `update_mailing_list`, `list_subscriptions`, `add_subscription` and
  `unsubscribe`. `add_subscription` mails a confirmation and leaves the person pending. It marks them subscribed only
  when you state that they already agreed, with where and when in your own words; the assistant passes your words and
  never writes them. Suppressed and opted-out addresses are refused.
- **CSV import.** `create_list_import` takes the file's text (up to about 5 MB through an assistant; larger files are
  uploaded in Yuva, see [Importing subscribers](importing.md)) with `attestation` and `consent_source`, which are
  stored as your proof of consent and must be your words. It only validates. After `get_list_import` shows the counts,
  `start_list_import` needs `confirm_count`, the number of valid rows you were shown; any other number answers
  `409 count_mismatch` and nothing is added.
- **Images and designs.** `upload_email_image` takes a public `https` URL (fetched within 10 seconds; private and local
  addresses are refused) or `data_base64`, about 5 MB of image at most through an assistant, and returns the id that an
  image block of `update_design` takes. `list_designs`, `get_design`, `list_email_images` and `duplicate_design` complete
  the set. See [E-mail designs](designs.md#4-images) for the image rules.
- **Running broadcasts.** `pause_broadcast`, `resume_broadcast`, `unschedule_broadcast` and `cancel_broadcast` are
  listed under the same rule as `schedule_broadcast`. `cancel_broadcast` needs `confirm_name`, the broadcast's name as
  you confirmed it. `request_broadcast_approval` asks the operator to allow broadcasts; it is offered only to an owner
  signed in through OAuth, and the website and the descriptions of the sender and of how people agreed are yours, never
  the assistant's.
- **People and conversations.** `list_members` gives `assign` its ids; contacts, their notes and subscriptions can be
  searched, created and updated; `start_conversation` writes the first message as a draft unless the workspace lets
  assistants deliver; `list_drafts` shows drafts waiting for review; labels and canned replies can be created.
  `list_discussions` and `get_discussion` read discussions; `reply_to_discussion` and `mark_discussion_answer` reply and
  mark the answer as you, through OAuth only and only while the workspace lets assistants deliver, since a reply is public
  at once.
- **Other products.** `create_status_page`, `create_status_component`, `create_status_monitor`, `update_incident`,
  `create_help_center`, `create_help_collection`, `create_changelog`, `create_roadmap_board`, `create_funnel` and
  `analytics_retention`. Nothing created is published: a member does that in Yuva. `create_status_monitor` shows the push
  token once, for you to put in your monitoring job.

What an assistant cannot do on purpose: manage credentials and access (API keys, OAuth grants, roles, invites, passkeys,
push subscriptions, webhooks, rotating a secret), change workspace settings or delete the workspace, delete things
(except `remove_migration_history`), export consent records or analytics or delete analytics data, upload migration
files, read raw messages and attachments, take SMTP or any other password, or publish help articles, What's new entries
and incidents. Unsubscribing is a state change, not a deletion.

## Reference

### Tools

Tools run the same checks as `/v1`. Tools you cannot use are not listed; calling one anyway
answers `insufficient_scope` with the scope to grant.

| Tools | Scope |
|---|---|
| `get_setup_state`, `report_yuva_issue` | none; every connection |
| `search_conversations`, `get_conversation`, `list_labels`, `list_canned_replies`, `get_counts` | `conversations:read` |
| `list_feedback` (with `feedback_score`, 1 to 5, when the contact gave one; `without_idea` keeps feedback not linked to an idea) | `conversations:read` |
| `list_inboxes` | `inboxes:read` |
| `get_contact`, `lookup_contact` | `contacts:read` |
| `draft_reply` | `messages:write` |
| `send_reply` | `messages:write`; only when bots may send |
| `send_draft` (sends a `draft_reply` draft) | `messages:write` and `drafts:send`; only when bots may send |
| `add_note` | `notes:write` |
| `assign`, `set_status`, `snooze`, `add_labels`, `remove_labels`, `move_conversation`, `bulk_update` | `conversations:write` |
| `merge_contacts` (owners, admins, keys without an inbox limit) | `contacts:write` |
| `list_channels`, `get_install_snippet` (code for `html`, `react`, `nextjs`, `nuxt`, `vue`, `astro`, `shadcn`, documentation sites with the `docs-` variants, and subscribe forms with `subscribe-html`, `subscribe-react`, `subscribe-nuxt` and `subscribe-shadcn`, which take `list_ids`) | `inboxes:read` |
| `create_inbox`, `create_chat_channel`, `update_chat_channel` (owners and admins) | `inboxes:manage` |
| `list_properties`, `analytics_overview`, `analytics_query` (also `dataset: support` and `insights`), `analytics_realtime`, `analytics_support_report`, `list_goals`, `funnel_report`, `get_analytics_snippet`, `add_annotation` | `analytics:read` |
| `create_property`, `create_goal` (owners, admins, keys without an inbox limit) | `analytics:manage` |
| `list_status_pages`, `get_status_page`, `list_monitors` | `status:read` |
| `set_component_state`, `create_incident` (always a draft), `post_incident_update`, `resolve_incident` | `status:manage` |
| `link_conversation_to_incident` | `status:manage` and `conversations:read` |
| `list_roadmap_ideas`, `get_roadmap_idea` | `conversations:read` and `roadmap:read` |
| `link_feedback_to_idea`, `unlink_feedback_from_idea` | `roadmap:write` and `conversations:read` |
| `move_idea_stage` (only when `roadmap:stage` is named; needs `confirm_linked`) | `roadmap:stage` |
| `list_help_centers`, `search_help_articles`, `get_help_article`, `help_search_gaps` | `help:read` |
| `draft_help_article`, `update_help_article_draft` (drafts only, with `base_version`; a `locale` writes a translation) | `help:write` |
| `import_help_articles` (a signed-in Zendesk migration into a center, as drafts), `get_help_import` | `help:write` |
| `list_whats_new_entries`, `get_whats_new_entry` | `changelog:read` |
| `draft_whats_new_entry`, `update_whats_new_draft` (drafts only, with `base_version`; a `locale` writes a translation) | `changelog:write` |
| `list_mailing_lists` | `lists:read` |
| `list_broadcasts`, `get_broadcast` (with the review and, for a draft, the audience count), `get_broadcast_report` | `broadcasts:read` |
| `list_email_templates`, `preview_design` (owners and admins) | `broadcasts:read` |
| `create_design`, `update_design`, `create_broadcast_draft`, `update_broadcast`, `send_test_broadcast` (owners and admins) | `broadcasts:write` |
| `schedule_broadcast` (only while the workspace allows keys and assistants to send broadcasts) | `broadcasts:send` |
| `list_members` | `inboxes:read` |
| `list_teams` | `inboxes:read` |
| `list_macros` | `conversations:read` |
| `search_contacts`, `list_contact_notes` | `contacts:read` |
| `list_contact_subscriptions` | `contacts:read` and `lists:read` |
| `create_contact`, `update_contact`, `add_contact_note` | `contacts:write` |
| `start_conversation` (the first message is a draft unless the caller may deliver and passes `send: true`) | `conversations:write` and `messages:write` |
| `list_drafts`, `list_discussions`, `get_discussion` | `conversations:read` |
| `reply_to_discussion`, `mark_discussion_answer` (OAuth only; only when bots may send) | `messages:write` |
| `request_broadcast_approval` (owners through OAuth only) | `workspace:manage` |
| `create_label` (owners, admins, keys) | `labels:write` |
| `create_canned_reply`, `update_canned_reply` (owners, admins, keys) | `canned_replies:write` |
| `update_inbox_sender`, `create_email_channel`, `create_dkim_key` (owners, admins, keys) | `inboxes:manage` |
| `get_deliverability` (with `check: true` it sends a test message and needs `inboxes:manage`) | `inboxes:read` |
| `list_subscriptions`, `get_list_import`, `list_list_imports`, `get_list_import_errors` | `lists:read` |
| `create_mailing_list`, `update_mailing_list`, `add_subscription`, `unsubscribe`, `create_list_import`, `update_list_import_mapping`, `cancel_list_import`, `start_list_import` (owners, admins, keys) | `lists:write` |
| `list_designs`, `get_design`, `list_email_images` (owners, admins, keys) | `broadcasts:read` |
| `duplicate_design`, `upload_email_image` (owners, admins, keys) | `broadcasts:write` |
| `pause_broadcast`, `resume_broadcast`, `unschedule_broadcast`, `cancel_broadcast` (only while the workspace allows keys and assistants to send broadcasts) | `broadcasts:send` |
| `create_status_page`, `create_status_component`, `create_status_monitor`, `update_incident` (owners, admins, keys) | `status:manage` |
| `create_help_collection` | `help:write` |
| `create_help_center` (owners, admins, keys; no OAuth) | `help:publish` |
| `create_changelog` (owners, admins, keys; no OAuth) | `changelog:publish` |
| `create_roadmap_board` (owners, admins, keys; no OAuth) | `roadmap:write` |
| `create_funnel` (owners, admins, keys) | `analytics:manage` |
| `analytics_retention` | `analytics:read` |
| `detect_mail_source`, `list_migrations`, `get_migration`, `create_migration`, `update_migration`, `test_migration_connection`, `scan_migration`, `preview_migration`, `start_migration`, `pause_migration`, `resume_migration`, `cancel_migration`, `finish_migration`, `retry_migration_items`, `list_migration_errors`, `send_migration_probe`, `remove_migration_history` (owners and admins) | `migrations:manage` |

With your draft open in a conversation, `send_reply` there answers `draft_pending`.

No migration tool takes a password: `create_migration` returns a Yuva page (`connect_url`) where you enter the app password. `remove_migration_history` is marked destructive and needs `confirm_count`, the number of conversations you confirmed.

With `inbox_id` and no `channel_id`, `create_migration` uses the inbox's e-mail channel for the mailbox address, or creates one (needs `inboxes:manage` too). A new channel has no SMTP account, so the answer carries `channel_created`, `channel_setup` (what to set in the panel before replies can be sent) and `channel_settings_url`; the `move_to_yuva` prompt passes these on to you.

`assign` takes `team_id` (a team from `list_teams` that holds the conversation's inbox) or `remove_team: true`; when either is sent and `assignee_id` is left out, the member assignee stays as it is. `search_conversations` takes `team_id`, and conversations carry `team_id`.

`list_macros` returns the workspace's shared macros and your own with `id`, `name`, `shortcut`, `visibility`, `body` and `actions` (`add_labels`, `remove_labels`, `assignee_id`, `unassign`, `status`, `snooze_minutes`, `priority`). The actions are for reference; no tool applies a macro.

### Resources and prompts

| Kind | Items |
|---|---|
| Resources | `yuva://inbox/{id}` (the inbox and its latest open conversations), `yuva://conversation/{id}`, `yuva://contact/{id}`, `yuva://broadcast/{id}`, `yuva://migration/{id}` (with `migrations:manage`), `yuva://property/{id}` (a website with its settings, the last seven days and its goals; with `analytics:read`), `yuva://help-center/{id}` (with `help:read`), `yuva://roadmap/{board_id}` (a board with its top ideas; with `roadmap:read` and `conversations:read`), `yuva://changelog/{id}` (a changelog with its newest entries; with `changelog:read`); subscriptions follow changes. |
| Prompts | `init` (optional goal; every connection), `triage_inbox`, `draft_reply`, `summarize_conversation`, `weekly_report`, `setup_live_chat`, `draft_newsletter`, `broadcast_report`, `move_to_yuva` (address, optional inbox; with `migrations:manage`), `analytics_review` (optional website and period; with `analytics:read`), `setup_analytics` (site address, optional framework; with `analytics:manage`), `write_help_article` (center, optional conversation or topic, locale; with `help:write`), `help_gaps_review` (center, optional days; with `help:read`), `link_feedback_to_ideas` (optional inbox and board; with `roadmap:write`, `roadmap:read` and `conversations:read`), `draft_release_notes` (notes to write from, optional changelog and locale; with `changelog:write`). |

### Safety and limits

| Item | Detail |
|---|---|
| Customer text | Returned only inside `customer_content` fields, marked as data, never instructions. |
| Hints | Read tools are `readOnlyHint`, repeatable writes `idempotentHint`, `merge_contacts` `destructiveHint`. Every tool has an output schema. |
| Rate limit | 600 requests a minute per token or key; beyond that `429` with `Retry-After`. |
| Turn it off | `YUVA_MCP=off` makes `/mcp` answer `404` ([Configuration](configuration.md#server)). The OAuth endpoints stay. |

### OAuth

Yuva is its own OAuth 2.1 server (authorization code, PKCE `S256`, resource indicators).

| Item | Detail |
|---|---|
| Discovery | `401` with `WWW-Authenticate` → `/.well-known/oauth-protected-resource/mcp` → `/.well-known/oauth-authorization-server`. |
| Registration | Dynamic (`POST /oauth/register`) or a Client ID Metadata Document (a `client_id` that is an `https` URL). |
| Consent | In the panel: pick the workspace and scopes. With `YUVA_PANEL=off`, clients get `temporarily_unavailable`. |
| Tokens | Act as you, narrowed by the allowed scopes. Access tokens last an hour and refresh for up to 30 days. |
| Redirect URIs | `https`, `http` on a loopback host, or a private-use scheme, matched exactly; the port of a loopback `http` URI may differ. |
| Grants | Settings → Connected apps lists and revokes them. |

### Local clients

`yuva mcp stdio` bridges stdio to `/mcp`, for clients that only start local programs.

```sh
yuva mcp stdio --url https://support.example.com --key "$YUVA_API_KEY"
```

| Item | Detail |
|---|---|
| Options | `--url` (`/mcp` is appended unless present) and `--key` (an API key or OAuth access token), or `YUVA_URL` and `YUVA_API_KEY`. |
| Binary | In `yuva.mcpb` (a zip: `server/<os>-<arch>/yuva`, `server/yuva.exe` on Windows), or `docker run --rm -i -e YUVA_URL -e YUVA_API_KEY ghcr.io/productdevbook/yuva:<version> mcp stdio`. |
| Claude Desktop bundle | Releases from 0.0.4 attach `yuva.mcpb` for macOS, Linux and Windows. Without it, add `"command": "/usr/local/bin/yuva"`, `"args": ["mcp", "stdio"]` and the two variables in `env` to `claude_desktop_config.json`. |
| Errors | A failed call returns a JSON-RPC error; a `401` exits with status 1. |
| MCP Registry | Listed as `io.github.productdevbook/yuva`, with the remote endpoint and the bundle. |
