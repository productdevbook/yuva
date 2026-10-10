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

## Reference

### Tools

Tools run the same checks as `/v1`. Tools you cannot use are not listed.

| Tools | Scope |
|---|---|
| `search_conversations`, `get_conversation`, `list_labels`, `list_canned_replies`, `get_counts`, `list_feedback` | `conversations:read` |
| `list_inboxes` | `inboxes:read` |
| `get_contact`, `lookup_contact` | `contacts:read` |
| `draft_reply` | `messages:write` |
| `send_reply` | `messages:write`; only when bots may send |
| `send_draft` (sends a `draft_reply` draft) | `messages:write` and `drafts:send`; only when bots may send |
| `add_note` | `notes:write` |
| `assign`, `set_status`, `snooze`, `add_labels`, `remove_labels`, `move_conversation`, `bulk_update` | `conversations:write` |
| `merge_contacts` (owners, admins, keys without an inbox limit) | `contacts:write` |

With your draft open in a conversation, `send_reply` there answers `draft_pending`.

### Resources and prompts

| Kind | Items |
|---|---|
| Resources | `yuva://inbox/{id}` (the inbox and its latest open conversations), `yuva://conversation/{id}`, `yuva://contact/{id}`; subscriptions follow changes. |
| Prompts | `triage_inbox`, `draft_reply`, `summarize_conversation`, `weekly_report`. |

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
