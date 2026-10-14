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

## Reference

### Tools

Tools run the same checks as `/v1`. Tools you cannot use are not listed; calling one anyway
answers `insufficient_scope` with the scope to grant.

| Tools | Scope |
|---|---|
| `get_setup_state`, `report_yuva_issue` | none; every connection |
| `search_conversations`, `get_conversation`, `list_labels`, `list_canned_replies`, `get_counts`, `list_feedback` | `conversations:read` |
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
| `list_mailing_lists` | `lists:read` |
| `list_broadcasts`, `get_broadcast` (with the review and, for a draft, the audience count), `get_broadcast_report` | `broadcasts:read` |
| `list_email_templates`, `preview_design` (owners and admins) | `broadcasts:read` |
| `create_design`, `update_design`, `create_broadcast_draft`, `send_test_broadcast` (owners and admins) | `broadcasts:write` |
| `schedule_broadcast` (only while the workspace allows keys and assistants to send broadcasts) | `broadcasts:send` |
| `detect_mail_source`, `list_migrations`, `get_migration`, `create_migration`, `update_migration`, `test_migration_connection`, `scan_migration`, `preview_migration`, `start_migration`, `pause_migration`, `resume_migration`, `cancel_migration`, `finish_migration`, `retry_migration_items`, `list_migration_errors`, `send_migration_probe`, `remove_migration_history` (owners and admins) | `migrations:manage` |

With your draft open in a conversation, `send_reply` there answers `draft_pending`.

No migration tool takes a password: `create_migration` returns a Yuva page (`connect_url`) where you enter the app password. `remove_migration_history` is marked destructive and needs `confirm_count`, the number of conversations you confirmed.

With `inbox_id` and no `channel_id`, `create_migration` uses the inbox's e-mail channel for the mailbox address, or creates one (needs `inboxes:manage` too). A new channel has no SMTP account, so the answer carries `channel_created`, `channel_setup` (what to set in the panel before replies can be sent) and `channel_settings_url`; the `move_to_yuva` prompt passes these on to you.

### Resources and prompts

| Kind | Items |
|---|---|
| Resources | `yuva://inbox/{id}` (the inbox and its latest open conversations), `yuva://conversation/{id}`, `yuva://contact/{id}`, `yuva://broadcast/{id}`, `yuva://migration/{id}` (with `migrations:manage`), `yuva://property/{id}` (a website with its settings, the last seven days and its goals; with `analytics:read`); subscriptions follow changes. |
| Prompts | `init` (optional goal; every connection), `triage_inbox`, `draft_reply`, `summarize_conversation`, `weekly_report`, `setup_live_chat`, `draft_newsletter`, `broadcast_report`, `move_to_yuva` (address, optional inbox; with `migrations:manage`), `analytics_review` (optional website and period; with `analytics:read`), `setup_analytics` (site address, optional framework; with `analytics:manage`). |

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
