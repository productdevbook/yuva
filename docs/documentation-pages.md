# Documentation pages

Add "Was this page helpful?" and a questions box to your documentation site, and read the answers
in the Yuva inbox. Your pages stay where they are; two web components send ratings, feedback and
questions to Yuva.

## 1. A chat channel

Use a chat channel, set up as for the [web widget](widget.md#1-create-a-chat-channel); one channel
can serve both.

- Add the docs site's origin to the channel's **allowed origins** (`https://docs.example.com`,
  plus your dev server, such as `http://localhost:3000`). On any other origin the elements stay
  empty.
- Give the inbox an [e-mail channel](email.md): replies reach readers by e-mail.

## 2. Add the elements

### Plain HTML

```html
<script src="https://support.example.com/yuva-docs.js" defer></script>

<yuva-page-questions channel="yuva_pk_xxxxxxxxxxxxxxxx"></yuva-page-questions>
<yuva-page-feedback channel="yuva_pk_xxxxxxxxxxxxxxxx"></yuva-page-feedback>
```

Put them in the template every docs page shares, after the article. Your Yuva server hosts
`/yuva-docs.js`.

npm has them as `useyuva/docs` and `useyuva/react` from the release after `useyuva` 0.0.6. Until
then, use the hosted script or depend on `sdk/js` by path, as the examples do.

### Nuxt Content

Runnable example: [`examples/nuxt-content`](../examples/nuxt-content)
([how to run it](../examples/README.md#nuxt-content)). The lines to add:

```ts
// app/plugins/yuva.client.ts
import "useyuva/docs";
export default defineNuxtPlugin(() => {});
```

```ts
// nuxt.config.ts
export default defineNuxtConfig({
  vue: { compilerOptions: { isCustomElement: (tag) => tag.startsWith("yuva-") } },
  runtimeConfig: { public: { yuvaChannel: "", yuvaServer: "" } },
});
```

```vue
<!-- app/pages/[...slug].vue, after <ContentRenderer> -->
<yuva-page-questions :channel="config.yuvaChannel" :server="config.yuvaServer" :page-title="page.title" />
<yuva-page-feedback :channel="config.yuvaChannel" :server="config.yuvaServer" :page-title="page.title" />
```

`config` is `useRuntimeConfig().public`. On a fresh site, `nuxt build` fails with "Nuxt Content
requires better-sqlite3": install it, or set `content.experimental.sqliteConnector: "native"` as the
example does. The page queries a `content` collection; define it as in the example's
[`content.config.ts`](../examples/nuxt-content/content.config.ts).

### Fumadocs

Runnable example: [`examples/fumadocs`](../examples/fumadocs)
([how to run it](../examples/README.md#fumadocs)). The docs page is a server component, so wrap
the React components in a client component:

```tsx
// components/yuva.tsx
"use client";
import { YuvaPageFeedback, YuvaPageQuestions } from "useyuva/react";

const channel = process.env.NEXT_PUBLIC_YUVA_CHANNEL!;
const server = process.env.NEXT_PUBLIC_YUVA_SERVER;

export const PageQuestions = ({ title }: { title: string }) =>
  <YuvaPageQuestions channel={channel} server={server} pageTitle={title} />;
export const PageFeedback = ({ title }: { title: string }) =>
  <YuvaPageFeedback channel={channel} server={server} pageTitle={title} />;
```

```tsx
// app/docs/[[...slug]]/page.tsx, after <DocsBody>
<PageQuestions title={page.data.title} />
<PageFeedback title={page.data.title} />
```

If you added Fumadocs' own `<Feedback>` component, replace it with `<PageFeedback />`, or keep it
and add only `<PageQuestions />`. With `useyuva` linked by path from `sdk/js`, Next can load a
second copy of React; set `turbopack.root` as in the example's `next.config.mjs`.

### Other generators

Load the script (or `useyuva/docs`) and put the elements under the article:

- **Starlight**: add the elements to an overridden `Footer`.
- **VitePress**: use the `doc-after` slot; set `isCustomElement` as for Nuxt.
- **Docusaurus**: swizzle `DocItem/Footer` (wrap).
- **MkDocs, Hugo, Jekyll and others**: as [plain HTML](#plain-html).

## What readers and your team see

- **Readers** rate a page Yes or No (an anonymous counter), then may write feedback; anyone can
  ask a question. Both take an optional e-mail address for the reply and stay private.
- **Your team** gets feedback and questions in the inbox as `feedback` and `question`
  conversations. The **Docs** view lists pages with their ratings, feedback and questions over 7, 30
  or 90 days. On a question you answered, **Publish to page** puts the edited question and answer
  under "Questions and answers" on that page, within a minute.

## Good to know

- **A page is its URL without query and fragment.** Per-language URLs (`/en/docs/install`,
  `/de/docs/install`) are separate pages with their own ratings; pass one URL as `page` to count
  them together, and set `locale` to the page's language.
- **Signed-in readers**: give the elements an [identity token](identity.md) with
  `setIdentityToken` (in React, the `identityToken` prop). They see no e-mail field and replies go
  to the address in the token. Without **Anonymous visitors** on the channel, feedback and
  questions need a token; ratings never do.
- **Content Security Policy**: allow the Yuva server in `script-src` (for `/yuva-docs.js`) and
  `connect-src`.
- **Elements stay empty**: the origin is not allowed or the key is wrong; see the browser console.

## Reference

### Attributes

| Attribute | Meaning |
|---|---|
| `channel` | The chat channel's public key (`yuva_pk_…`). Required. |
| `server` | Yuva server URL. Defaults to the origin `yuva-docs.js` was loaded from. |
| `locale` | `en` or `tr`. Defaults to the browser's language, else English. |
| `dir` | `ltr` or `rtl`. Defaults to the locale's direction. |
| `identity-token` | A signed identity token. `setIdentityToken` is usually better. |
| `page` | The page's URL. Defaults to the current URL; client-side navigation is followed. |
| `page-title` | The page's title. Defaults to `document.title`. |

The React wrappers take the same as props (`channel`, `server`, `locale`, `dir`, `page`,
`pageTitle`, `identityToken`) plus `className` and `style`.

### Properties and methods

| Element | Member | Meaning |
|---|---|---|
| both | `channel`, `server`, `locale`, `page`, `pageTitle` | The attributes as properties. |
| both | `setIdentityToken(fn)` | A function returning a token, or `null` for a reader not signed in. |
| `yuva-page-feedback` | `rating` | `"up"`, `"down"` or `null` for this page in this browser. |
| `yuva-page-feedback` | `rate("up" \| "down")` | The same as pressing Yes or No. |
| `yuva-page-questions` | `answers` | The published answers shown. |
| `yuva-page-questions` | `reload()` | Fetch the answers again. |

### Events

They bubble out of the Shadow DOM, so you can listen on `document`.

| Event | Element | `detail` | React prop |
|---|---|---|---|
| `yuva-rating` | `yuva-page-feedback` | `{ page, rating, previous }` | `onRating` |
| `yuva-feedback` | `yuva-page-feedback` | `{ page, rating, conversation_id }` | `onFeedback` |
| `yuva-question` | `yuva-page-questions` | `{ page, conversation_id }` | `onQuestion` |

### CSS variables

The elements take `color` and `font` from the page and follow dark mode. The accent comes from the
channel's launcher colour; to use your own, set it with `!important`.

| Variable | Used for |
|---|---|
| `--yuva-accent` | Buttons, the selected rating, focus rings, links in answers. |
| `--yuva-on-accent` | Text on the accent. |
| `--yuva-border` | Borders of buttons, fields and answers. |
| `--yuva-soft` | The selected rating's background, code in answers. |
| `--yuva-danger` | Error messages. |

### Endpoints

| Endpoint | Used for |
|---|---|
| `POST /client/v1/channels/{channel_key}/page-ratings` | Count a rating. |
| `POST /client/v1/feedback` | Send feedback with `page_url`, `page_title`, `rating`. |
| `POST /client/v1/questions` | Ask a question. |
| `GET /client/v1/channels/{channel_key}/page-answers?page=` | A page's published answers (cached for a minute). |
| `POST /v1/conversations/{conversationId}/publish` | Publish a question and its answer. |
| `GET /v1/page-answers`, `GET`, `PATCH`, `DELETE /v1/page-answers/{pageAnswerId}` | List, edit and unpublish answers. |
| `GET /v1/docs/pages`, `GET /v1/docs/summary`, `GET /v1/docs/page` | The Docs view. |
| `GET /v1/conversations?page=&kind=` | A page's feedback or questions. |

Details: [API contract](../openapi/openapi.yaml).
