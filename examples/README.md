# Examples

Small programs that use Yuva without its panel, and documentation sites with Yuva's page
feedback and questions. They are MIT licensed ([LICENSE](LICENSE)), so you can copy them into
your own code. The walkthroughs are the [Headless guide](../docs/headless.md) and
[Documentation pages](../docs/documentation-pages.md).

| Example | What | Runs on |
|---|---|---|
| [headless-chat](headless-chat) | A terminal chat for a contact, built only on `createYuvaClient()` from `useyuva` | Node 22.18+ |
| [inbox-feed](inbox-feed) | Follows a workspace through the event feed `GET /v1/events` with the typed `/v1` client from `useyuva/api` | Node 22.18+ |
| [draft-bot](draft-bot) | A webhook receiver that verifies the signature and answers every incoming message with a draft | Go 1.27 |
| [nuxt-content](nuxt-content) | A Nuxt Content documentation site with `<yuva-page-feedback>` and `<yuva-page-questions>` on every page | Node 22.5+, Nuxt 4, Nuxt Content 3 |
| [fumadocs](fumadocs) | A Fumadocs site (from `create-fumadocs-app`) with `YuvaPageFeedback` and `YuvaPageQuestions` on every docs page | Node 20.9+, Next.js 16, Fumadocs 16 |

The JS examples point at the SDK in this repository (`file:../../sdk/js`); build it once with
`bun install && bun run build` in `sdk/js`. In your own project, depend on `useyuva` from npm
instead (`useyuva/docs` and the docs components in `useyuva/react` arrive in the release after
0.0.6). The Go example uses `sdk/go` through a `replace` directive in its
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

## nuxt-content

A four-page Nuxt Content site with both elements on every page, set up as in
[Documentation pages](../docs/documentation-pages.md#nuxt-content). Needs a chat channel with
`http://localhost:3101` among its allowed origins.

```sh
cd examples/nuxt-content
npm install
cp .env.example .env   # set NUXT_PUBLIC_YUVA_SERVER and NUXT_PUBLIC_YUVA_CHANNEL
npm run dev            # http://localhost:3101
```

## fumadocs

The `create-fumadocs-app` template with four pages and both components on every docs page, set up
as in [Documentation pages](../docs/documentation-pages.md#fumadocs). `app/global.css` gives the
elements Fumadocs' colours. Needs a chat channel with `http://localhost:3102` among its allowed
origins.

```sh
cd examples/fumadocs
npm install
cp .env.example .env.local   # set NEXT_PUBLIC_YUVA_SERVER and NEXT_PUBLIC_YUVA_CHANNEL
npm run dev                  # http://localhost:3102/docs
```

`NEXT_PUBLIC_` values are built into the client bundle, so build again after changing them.
`turbopack.root` in `next.config.mjs` is there only because `useyuva` is linked from `sdk/js`;
drop it when you use `useyuva` from npm.
