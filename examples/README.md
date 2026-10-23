# Examples

Small programs that use Yuva without its panel, and documentation sites with Yuva's page
feedback and discussions. They are MIT licensed ([LICENSE](LICENSE)), so you can copy them into
your own code. The walkthroughs are the [Headless guide](../docs/headless.md) and
[Documentation pages](../docs/documentation-pages.md).

| Example | What | Runs on |
|---|---|---|
| [headless-chat](headless-chat) | A terminal chat for a contact, built only on `createYuvaClient()` from `useyuva` | Node 22.18+ |
| [inbox-feed](inbox-feed) | Follows a workspace through the event feed `GET /v1/events` with the typed `/v1` client from `useyuva/api` | Node 22.18+ |
| [draft-bot](draft-bot) | A webhook receiver that verifies the signature and answers every incoming message with a draft | Go 1.27 |
| [nuxt-content](nuxt-content) | A Nuxt Content documentation site with `<yuva-page-feedback>` and `<yuva-discussions>` on every page, signed in as a demo reader through `/_yuva/identity` | Node 22.5+, Nuxt 4, Nuxt Content 3 |
| [nitro](nitro) | A Nitro app with `useyuva/nitro`: a chat for a demo user signed in through `/_yuva/identity`, a webhook receiver and a server-side analytics event | Node 22+, Nitro 3 |
| [fumadocs](fumadocs) | A Fumadocs site (from `create-fumadocs-app`) with `YuvaPageFeedback` and `YuvaDiscussions` on every docs page | Node 20.9+, Next.js 16, Fumadocs 16 |
| [shadcn-next](shadcn-next) | A Next.js app on shadcn/ui with the `yuva-discussions` and `yuva-page-feedback` components installed from Yuva's shadcn registry | Node 20.9+, Next.js 16, shadcn 4 |
| [shadcn-vite](shadcn-vite) | A Vite app on shadcn/ui with the `yuva-chat` and `yuva-discussions` blocks installed from Yuva's shadcn registry, themed with Yuva's lined look | Bun, Vite 7, React 19, Tailwind 4, shadcn 4 |

The JS examples point at the SDK in this repository (`file:../../sdk/js`); build it once with
`bun install && bun run build` in `sdk/js`. In your own project, depend on `useyuva` from npm
instead. The Go example uses `sdk/go` through a `replace` directive in its `go.mod`;
drop that line in your own module.

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
cp .env.example .env   # set NUXT_PUBLIC_YUVA_SERVER, NUXT_PUBLIC_YUVA_CHANNEL and NUXT_YUVA_IDENTITY_SECRET
npm run dev            # http://localhost:3101
```

`server/yuva.ts` signs every visitor in as the same demo reader, so discussions are written as that
contact; replace `user(event)` with your own session. Without `NUXT_YUVA_IDENTITY_SECRET` the
readers stay anonymous.

## nitro

Needs a chat channel with `http://localhost:3104` among its allowed origins, the inbox's identity
secret and, for the optional parts, a webhook endpoint pointing at `/_yuva/webhook` and an API key
with `analytics:write` for an analytics property.

```sh
cd examples/nitro
npm install
cp .env.example .env   # set the NITRO_YUVA_ values; leave out the parts you do not need
npm run dev            # http://localhost:3104
```

"Sign in as Ada" sets a demo cookie that `user(event)` in `server/yuva.ts` turns into an identity,
and tracks a `Sign in` event; the page asks `/_yuva/identity` for the token. `message.created`
webhooks are logged. `npm run build` and `npm start` run the same app from `.output`; the secrets are
read at runtime, but a route is built only when its secret is set at build time too.

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

## shadcn-next

A `create-next-app` project with `shadcn init`, then
`npx shadcn add <site>/r/yuva-discussions.json <site>/r/yuva-page-feedback.json` from the registry
the site builds out of [`sdk/shadcn`](../sdk/shadcn), set up as in
[Documentation pages](../docs/documentation-pages.md#shadcnui). Everything that command wrote is
committed (`components/`, `hooks/`, `lib/`), so it shows what you get. Three pages render the
block. Needs a chat channel with `http://localhost:3103` among its allowed origins.

```sh
cd examples/shadcn-next
npm install
cp .env.example .env.local   # set NEXT_PUBLIC_YUVA_SERVER and NEXT_PUBLIC_YUVA_CHANNEL
npm run dev                  # http://localhost:3103/docs/getting-started
```

As in the Fumadocs example, `turbopack.root` in `next.config.ts` is there only because `useyuva` is
linked from `sdk/js`.

## shadcn-vite

A Vite, React and Tailwind v4 project with a `components.json` and the `@` alias, then
`bunx shadcn add <site>/r/yuva-chat.json <site>/r/yuva-discussions.json`. Everything that command wrote
is committed (`src/components/`, `src/hooks/`, `src/lib/`). The look is only the shadcn CSS variables
in `src/index.css` (Yuva's lined theme, light and dark, Geist from Fontsource) and the frame around
each section; the registry items are untouched. Needs a chat channel with `http://localhost:3104` among
its allowed origins.

```sh
cd examples/shadcn-vite
bun install
cp .env.example .env.local   # set VITE_YUVA_SERVER and VITE_YUVA_CHANNEL
bun run dev                  # http://localhost:3104
```

`bun run build` type-checks and builds; `bun run preview` serves the build.
