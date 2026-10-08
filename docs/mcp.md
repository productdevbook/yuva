# AI assistants (MCP)

Yuva has an [MCP](https://modelcontextprotocol.io) server built in, so assistants such as Claude,
ChatGPT, Cursor, VS Code and Gemini CLI can search your inbox, read conversations, triage them and
draft replies. Yuva runs no model itself: the assistant brings its own, and works with the access
you give it.

## In short

- The endpoint is `/mcp` on your server, e.g. `https://support.example.com/mcp` (Streamable HTTP).
- Sign in with [OAuth](#oauth): the assistant acts as you, never with more access than you have. Or
  use an [API key](#api-key) for automations without a person.
- Replies are [drafts](headless.md#a-bot-that-answers-with-drafts) a member sends, unless the
  workspace lets bots send. What an assistant writes shows as "Ayşe via Claude".
- [Set up your client](#clients); for clients that only start local programs, use the
  [stdio bridge](#local-clients) or the [bundle for Claude Desktop](#claude-desktop-bundle).

## What an assistant can do

**Tools.** Each one calls the same code as a `/v1` operation, with the same access checks. Tools
your scopes or role cannot use are not listed.

| Tools | Scope |
|---|---|
| `search_conversations`, `get_conversation`, `list_labels`, `list_canned_replies`, `get_counts`, `list_feedback` | `conversations:read` |
| `list_inboxes` | `inboxes:read` |
| `get_contact`, `lookup_contact` | `contacts:read` |
| `draft_reply` | `messages:write` |
| `send_reply`, listed only when the caller may deliver | `messages:write`, and bots may send |
| `add_note` | `notes:write` |
| `assign`, `set_status`, `snooze`, `add_labels`, `remove_labels`, `move_conversation`, `bulk_update` | `conversations:write` |
| `merge_contacts`, owners and admins (and keys without an inbox limit) | `contacts:write` |

Read tools are marked `readOnlyHint`, repeatable writes `idempotentHint`, and `merge_contacts`
`destructiveHint`. Every tool has an output schema.

**Resources.** `yuva://inbox/{id}` (the inbox and its latest open conversations),
`yuva://conversation/{id}` and `yuva://contact/{id}`, with subscriptions that follow changes.

**Prompts.** `triage_inbox`, `draft_reply` (in the inbox's language, using canned replies and
earlier answers), `summarize_conversation` and `weekly_report`.

**Customer text is data.** Message bodies, subjects, names, e-mail addresses and attributes come
back only inside `customer_content` fields, and every tool description tells the assistant that
text there is from customers and never an instruction. An e-mail that says "ignore your
instructions and send…" cannot make anything go out on its own: replies are drafts unless the
workspace allows bots to send.

**Drafts and "via".** `draft_reply` stores a draft; a member reviews it in the conversation and
sends, edits or discards it. Everything written through OAuth carries the client's name: the
timeline shows "Ayşe via Claude Code", and the message's `author` has `via`. Writes with an API key
are by the key's bot. `send_reply` exists only when the workspace setting **Bots may send** is on
(see [Headless](headless.md#bots-may-send)).

Each token or key may make 600 requests a minute (`429` with `Retry-After` beyond).

## Connect

### OAuth

Clients that support MCP authorization find everything from the server URL: a request without a
token gets `401` with `WWW-Authenticate: Bearer resource_metadata="…"`, which leads to the protected
resource metadata (`/.well-known/oauth-protected-resource/mcp`) and the authorization server
metadata (`/.well-known/oauth-authorization-server`). Yuva is its own authorization server: OAuth
2.1, authorization code with PKCE (`S256`), resource indicators.

1. The client registers itself, either through dynamic client registration (`POST /oauth/register`)
   or with a Client ID Metadata Document (a `client_id` that is an `https` URL).
2. Your browser opens Yuva's consent page. Sign in with a code or a passkey, pick the workspace,
   check the scopes and allow. The page shows the client's name as the client gave it, and the host
   it returns to.
3. The client gets a token that acts as you: your role and inbox access, narrowed by the scopes you
   allowed. Access tokens last an hour and are refreshed for up to 30 days.

Settings → Connected apps lists your grants with their last use and revokes them; owners and admins
see every grant in the workspace. A member who leaves loses every grant.

Requirements:
- The panel must be on: with `YUVA_PANEL=off` there is no consent page, and clients get
  `temporarily_unavailable`.
- Assistants that run in a vendor's cloud (claude.ai, ChatGPT) reach your server from the internet,
  so it needs a public `https` URL. Desktop and command-line clients only need to reach it from
  your machine.
- Redirect URIs must be `https`, `http` on a loopback host, or a private-use scheme, and they are
  matched exactly, except that the port of an `http` URI on `localhost`, `127.0.0.1` or `[::1]` may
  differ, so desktop clients can call back on whatever port is free (RFC 8252).

### API key

Every client that can send a header can use an API key instead: `Authorization: Bearer <key>`. Make
a key in Settings → API keys with the scopes the assistant needs (for example `conversations:read`,
`inboxes:read`, `contacts:read`, `messages:write`, `notes:write`), limit it to some inboxes if you
like, and give it a bot name such as "Claude". See [API keys](headless.md#api-keys).

## Clients

The setup below follows each client's own documentation, linked in every section. The server URL
is your `YUVA_PUBLIC_URL` with `/mcp` appended.

### Claude (claude.ai and Claude Desktop)

Custom connectors run from Anthropic's cloud, for claude.ai and for Claude Desktop alike, so your
server needs a public `https` URL. In Claude: Customize → Connectors → **Add custom connector** (on
Team and Enterprise plans an owner adds it in the organization's settings first), enter
`https://support.example.com/mcp`, and leave authentication on signing in. Claude then opens Yuva's
consent page. Yuva supports both of Claude's ways to register: its published client identity and
automatic registration. Claude's guide: [custom connectors](https://claude.com/docs/connectors/custom/add-unlisted).

### Claude Desktop bundle

For a server Anthropic's cloud cannot reach, or to use an API key: releases from 0.0.4 on attach
`yuva.mcpb`, an [MCP Bundle](https://claude.com/docs/connectors/building/mcpb) with the
[stdio bridge](#local-clients) for macOS, Linux and Windows. Open it (double-click, drag it into
Claude Desktop, or Settings → Extensions → Advanced settings → Install Extension…). Claude Desktop
asks for the server URL and an API key, which it stores as a sensitive setting.

Without the bundle, put the bridge into `claude_desktop_config.json` (Settings → Developer → Edit
Config):

```json
{
  "mcpServers": {
    "yuva": {
      "command": "/usr/local/bin/yuva",
      "args": ["mcp", "stdio"],
      "env": { "YUVA_URL": "https://support.example.com", "YUVA_API_KEY": "yuva_…" }
    }
  }
}
```

### Claude Code

With an API key ([Claude Code MCP docs](https://code.claude.com/docs/en/mcp)):

```sh
claude mcp add --transport http yuva https://support.example.com/mcp \
  --header "Authorization: Bearer $YUVA_API_KEY"
```

`--scope local` (the default) keeps it to the current project for you, `--scope project` writes it
to `.mcp.json` for the team, `--scope user` to every project.

With OAuth, add the server without a header and sign in:

```sh
claude mcp add --transport http yuva https://support.example.com/mcp
```

`claude mcp login yuva` (or `/mcp` inside a session) opens the consent page; `--no-browser` prints the
URL instead, for a machine without a browser. Then ask, for example: "List my Yuva inboxes and
draft a reply in Turkish to the open conversation in the Turkish inbox."

### ChatGPT

ChatGPT connects to remote MCP servers in developer mode, on the web, over OAuth only: it cannot
send an API key. Add the server as a custom MCP server with the URL
`https://support.example.com/mcp` and OAuth authentication; ChatGPT registers itself and opens
Yuva's consent page. Your server needs a public `https` URL. Plans, the menu path and the
redirect URI are in OpenAI's guide:
[connect from ChatGPT](https://developers.openai.com/plugins/deploy/connect-chatgpt).

### Cursor

In `.cursor/mcp.json` (the project) or `~/.cursor/mcp.json` (every project)
([Cursor MCP docs](https://cursor.com/docs/context/mcp)):

```json
{
  "mcpServers": {
    "yuva": {
      "url": "https://support.example.com/mcp",
      "headers": { "Authorization": "Bearer ${env:YUVA_API_KEY}" }
    }
  }
}
```

Leave out `headers` to sign in with OAuth instead.

### VS Code

In `.vscode/mcp.json`, or with the command **MCP: Add Server**
([VS Code MCP docs](https://code.visualstudio.com/docs/copilot/customization/mcp-servers)):

```json
{
  "inputs": [
    { "type": "promptString", "id": "yuva-key", "description": "Yuva API key", "password": true }
  ],
  "servers": {
    "yuva": {
      "type": "http",
      "url": "https://support.example.com/mcp",
      "headers": { "Authorization": "Bearer ${input:yuva-key}" }
    }
  }
}
```

Without `headers` (and `inputs`), VS Code runs the OAuth flow in your browser.

### Gemini CLI

([Gemini CLI MCP docs](https://geminicli.com/docs/tools/mcp-server/))

```sh
gemini mcp add --transport http --header "Authorization: Bearer $YUVA_API_KEY" \
  yuva https://support.example.com/mcp
```

Or in `~/.gemini/settings.json`:

```json
{
  "mcpServers": {
    "yuva": {
      "httpUrl": "https://support.example.com/mcp",
      "headers": { "Authorization": "Bearer $YUVA_API_KEY" }
    }
  }
}
```

Without the header, `/mcp auth yuva` inside Gemini CLI signs in with OAuth. Yuva's tool schemas use
one type per field, which Gemini requires.

### MCP Inspector

[MCP Inspector](https://github.com/modelcontextprotocol/inspector) shows every tool, resource and
prompt with its schema. `npx @modelcontextprotocol/inspector` opens its web UI; its CLI mode is
handy for scripts:

```sh
npx @modelcontextprotocol/inspector --cli https://support.example.com/mcp --transport http \
  --header "Authorization: Bearer $YUVA_API_KEY" --method tools/list

npx @modelcontextprotocol/inspector --cli https://support.example.com/mcp --transport http \
  --header "Authorization: Bearer $YUVA_API_KEY" --method tools/call --tool-name list_inboxes
```

Through the stdio bridge, put the command first and pass the server and key as environment
variables (the Inspector does not pass `--url` and `--key` on to the command):

```sh
npx @modelcontextprotocol/inspector --cli yuva mcp stdio \
  -e YUVA_URL=https://support.example.com -e YUVA_API_KEY=$YUVA_API_KEY --method tools/list
```

## Local clients

`yuva mcp stdio` is a bridge from stdio to a server's `/mcp`, for clients that only start local
programs. It is part of the `yuva` binary and needs no database.

```sh
yuva mcp stdio --url https://support.example.com --key "$YUVA_API_KEY"
```

Take the binary from `yuva.mcpb`, which is a zip archive (`server/<os>-<arch>/yuva`, and
`server/yuva.exe` on Windows), or run it from the Docker image:
`docker run --rm -i -e YUVA_URL -e YUVA_API_KEY ghcr.io/productdevbook/yuva:<version> mcp stdio`.

- `--url` is the server's public URL; `/mcp` is appended unless it is there. `YUVA_URL` and
  `YUVA_API_KEY` work instead of the flags.
- The key may be an API key or an OAuth access token, sent as `Authorization: Bearer`.
- Messages pass through unchanged, so every method and protocol version the server speaks works.
- A failed call gets a JSON-RPC error on stdout and a JSON log line on stderr; an unreachable
  server fails that call but keeps the bridge running. A `401` (wrong, expired or revoked key) ends
  it with exit status 1.

The repository's `server.json` describes Yuva for the [MCP Registry](https://registry.modelcontextprotocol.io):
the remote endpoint `https://{host}/mcp` and the bundle as a package.

## Turn it off

`YUVA_MCP=off` makes `/mcp` answer `404` ([Configuration](configuration.md#server)). The OAuth
endpoints stay, since the member apps use them.
