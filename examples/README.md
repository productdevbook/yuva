# Examples

Small programs that use Yuva without its panel. They are MIT licensed ([LICENSE](LICENSE)), so you
can copy them into your own code. The walkthrough is the [Headless guide](../docs/headless.md).

| Example | What | Runs on |
|---|---|---|
| [headless-chat](headless-chat) | A terminal chat for a contact, built only on `createYuvaClient()` from `useyuva` | Node 22.18+ |
| [inbox-feed](inbox-feed) | Follows a workspace through the event feed `GET /v1/events` with the typed `/v1` client from `useyuva/api` | Node 22.18+ |
| [draft-bot](draft-bot) | A webhook receiver that verifies the signature and answers every incoming message with a draft | Go 1.27 |

The JS examples point at the SDK in this repository (`file:../../sdk/js`), since `useyuva` is
not on npm yet; build it once with `bun install && bun run build` in `sdk/js`. In your own project,
depend on `useyuva` instead. The Go example uses `sdk/go` through a `replace` directive in its
`go.mod`; drop that line in your own module.

## headless-chat

Needs an `app` channel (made in the panel, or with `POST /v1/inboxes/{inboxId}/channels` with
`"kind": "app"`). With `allow_anonymous` on, the chat starts as an anonymous visitor; set
`YUVA_IDENTITY_TOKEN` to a token signed by your backend ([Identity tokens](../docs/identity.md)) to
chat as a signed-in user.

```sh
cd examples/headless-chat
npm install
YUVA_SERVER=https://support.example.com YUVA_CHANNEL=yuva_pk_… npm start
```

Type a line to send it. The first line starts a conversation; replies from members and bots appear
as they arrive over realtime. The session and visitor id are kept in `.yuva-chat.json`, so the next
run resumes the same conversation.

## inbox-feed

Needs an API key with `conversations:read`.

```sh
cd examples/inbox-feed
npm install
YUVA_SERVER=https://support.example.com YUVA_API_KEY=yuva_… npm start
```

The first run lists the open conversations and starts at the current end of the feed; later runs
continue from the cursor saved in `.yuva-cursor`. An expired cursor (`410 cursor_expired`) reloads
the list and continues from `GET /v1/events/latest`. `--once` stops when it has caught up.

## draft-bot

Needs an API key with `messages:write` (and `drafts:send` to send) and a webhook endpoint for
`message.created` pointing at the bot, whose `whsec_…` secret it verifies with.

```sh
cd examples/draft-bot
YUVA_SERVER=https://support.example.com YUVA_API_KEY=yuva_… YUVA_WEBHOOK_SECRET=whsec_… \
  LISTEN_ADDR=127.0.0.1:3000 go run .
```

For every incoming message the bot posts a draft reply (`BOT_REPLY` sets the text) with an
`Idempotency-Key` derived from the message, so a webhook Yuva retries gives back the same draft.
With `BOT_SEND=true` it also sends the draft, which works only when the workspace lets bots send.
