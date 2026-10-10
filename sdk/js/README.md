# useyuva

JavaScript SDK for [Yuva](https://github.com/productdevbook/yuva), open-source customer messaging.

| Import | What |
|---|---|
| `useyuva` | `createYuvaClient()`: the contact side (`/client/v1`) without DOM. Browsers, Node 22+, Bun, Deno, workers. |
| `useyuva/chat` | The `<yuva-chat>` web component, built on that client. Browser only. |
| `useyuva/docs` | `<yuva-page-feedback>` and `<yuva-page-questions>` for documentation pages. Registers in the browser only, so it is safe to import during SSR. |
| `useyuva/react` | `YuvaProvider`, `useConversations`, `useMessages`. React 18+ is an optional peer. |
| `useyuva/api` | A typed `/v1` client for backends (API key), generated from the OpenAPI contract. |

```sh
npm install useyuva
```

Without a bundler, the server hosts the widget itself: `<script src="https://<your-yuva>/yuva.js" defer></script>`,
and the documentation page elements without the chat: `<script src="https://<your-yuva>/yuva-docs.js" defer></script>`.

## Client

```ts
import { createYuvaClient } from "useyuva";

const yuva = createYuvaClient({
  server: "https://support.example.com",
  channel: "yuva_pk_…",
  identityToken: () => fetch("/my/yuva-token").then((r) => r.text()),
});

yuva.on("event", (event) => {
  if (event.type === "message.created") console.log(event.data.author.type, event.data.body);
});
await yuva.connect();

const { conversation } = await yuva.startConversation({ body: "Hello" });
await yuva.sendMessage(conversation.id, { body: "Here is a screenshot", files: [file] });
```

Options:

| Option | Meaning |
|---|---|
| `server` | Base URL of the Yuva server. |
| `channel` | Public key of a `chat` or `app` channel. Outside a browser use an `app` channel: `chat` channels accept only their allowed origins. |
| `identityToken` | `() => string \| null \| Promise<…>`: an identity token signed by your backend. Without it the contact is an anonymous visitor (if the channel allows them). |
| `storage` | `{ getItem, setItem }` keeping the session and visitor id. Defaults to `localStorage`, else memory. |
| `fetch`, `WebSocket` | Replacements for the global ones. |
| `idempotencyKeys` | Send `client_id` as `Idempotency-Key` too. On by default outside browsers; off in browsers until the server allows the header cross-origin. |

Session:

- `start()` opens a session, or resumes the stored one for the same identity. Every other call starts one when needed; a session that ended (`401`) is renewed once and the call retried.
- `session` (contact, inbox settings, expiry), `token`, `refresh()`, `channelSettings()` (the inbox's public settings without a session).
- `setIdentityToken(source)` switches identity; `reset()` forgets the session in memory; `signOut()` ends it on the server and in storage.

Conversations and messages:

- `listConversations({ cursor, limit })`, `getConversation(id)`, `startConversation({ body, subject, files, client_id })`.
- `listMessages(id, { order, cursor, limit })`, `sendMessage(id, { body, files, client_id })`. `client_id` (generated when absent) makes a retry safe: the server returns the stored message for a repeated `client_id`, and with `idempotencyKeys` the request also carries it as `Idempotency-Key`.
- `markRead(id, messageId?)`, `setTyping(id, typing)`, `setEmail(email)`, `attachment(id)` (a `Blob`).

Realtime:

- `connect()` opens `/client/v1/realtime`, resumes after the last event on reconnect, retries with backoff and renews an ended session. `disconnect()`, `reconnect()` (try now, e.g. when the page becomes visible), `connected`.
- `on(type, handler)` returns an unsubscribe function. `event`: every realtime frame (`message.created`, `conversation.updated`, `typing`, `presence`, `resync_required`, …). `connection`: `{ state: "connecting" | "open" | "reconnecting" | "closed", attempt }`. `session`: `{ session, renewed }`; after `renewed` or `resync_required`, reload what you show.

Errors are `ApiError` with `status` and the problem `code` (`status` 0 and `network` when offline).

## Chat element

```ts
import "useyuva/chat";
```

```html
<yuva-chat channel="yuva_pk_…" server="https://support.example.com"></yuva-chat>
```

Attributes, methods and events are in [docs/widget.md](https://github.com/productdevbook/yuva/blob/main/docs/widget.md).

## Documentation pages

```ts
import "useyuva/docs";
```

```html
<yuva-page-feedback channel="yuva_pk_…" server="https://support.example.com"></yuva-page-feedback>
<yuva-page-questions channel="yuva_pk_…" server="https://support.example.com"></yuva-page-questions>
```

Both take `channel`, `server`, `locale`, `dir` and `identity-token` like `<yuva-chat>`, plus `page`
and `page-title`, which default to the URL without query and fragment and `document.title`. They
follow client-side navigation. Events: `yuva-rating` (`{ page, rating, previous }`),
`yuva-feedback` (`{ page, rating, conversation_id }`) and `yuva-question` (`{ page, conversation_id }`).
In React use `YuvaPageFeedback` and `YuvaPageQuestions` from `useyuva/react`.

## React

```tsx
import { createYuvaClient } from "useyuva";
import { YuvaProvider, useConversations, useMessages } from "useyuva/react";

const yuva = createYuvaClient({ server, channel });

function App() {
  return (
    <YuvaProvider client={yuva}>
      <Inbox />
    </YuvaProvider>
  );
}

function Thread({ id }: { id: string }) {
  const { messages, send, loadOlder, hasMore } = useMessages(id);
  // …
}
```

`YuvaProvider` connects realtime while mounted (`connect={false}` to leave that to you).
`useConversations()` returns `{ conversations, loading, error, hasMore, loadMore, reload }`;
`useMessages(id)` returns `{ messages, loading, error, hasMore, loadOlder, send, markRead }`. Both
follow realtime events. Start a new conversation with `client.startConversation()`.

## Typed /v1 client

```ts
import { createYuvaApi } from "useyuva/api";

const api = createYuvaApi({ server: "https://support.example.com", apiKey: process.env.YUVA_API_KEY! });
const { data, error } = await api.GET("/v1/inboxes");
```

Built on [openapi-fetch](https://openapi-ts.dev/openapi-fetch/); paths, parameters and bodies are
typed from the contract. Keep API keys on the server.

## License

MIT
