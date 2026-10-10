# Documentation pages

Put "Was this page helpful?" and a questions box on your product's documentation site, and read
what readers say in the Yuva inbox. Yuva hosts no documentation: your pages stay where they are
(Nuxt Content, Fumadocs, Starlight, VitePress, plain HTML), and two web components send ratings,
feedback and questions to Yuva.

## In short

- [Use a chat channel](#1-a-chat-channel) whose allowed origins include the docs site.
- [Add the elements](#2-add-the-elements): `<yuva-page-feedback>` and `<yuva-page-questions>`, in
  [plain HTML](#plain-html), [Nuxt Content](#nuxt-content), [Fumadocs](#fumadocs) or
  [another generator](#other-generators).
- Answer feedback and questions in the panel, and [publish answers](#in-the-panel) back to the
  page.

## What it does and what Yuva stores

- **Ratings.** `<yuva-page-feedback>` asks "Was this page helpful?" with Yes and No. A rating is
  only a counter: Yuva adds one to the page's `up` or `down` for that day. It creates no contact,
  no conversation and no session, and stores nothing about the reader. The element remembers the
  reader's rating in their browser; if they change their mind, the old count goes down and the
  new one up.
- **Feedback.** After a rating the element offers a text field and an optional e-mail address.
  Sending it starts a `feedback` conversation with the page's URL, title and the rating. It is
  answered like any other feedback; the reply is e-mailed only if the reader left an address (or
  is [signed in](#signed-in-readers)).
- **Questions.** `<yuva-page-questions>` lets a reader ask a question about the page, with an
  optional e-mail address for the answer. It starts a private conversation of kind `question`.
  Nothing a reader writes appears on the page by itself.
- **Published answers.** A member who answered a question can publish it to the page: the
  question and the answer as the member edits them. The element lists them under "Questions and
  answers". A published answer is the member's text only, never the reader's name or address, so
  retention and contact deletion leave it; deleting the source conversation keeps the answer.

## 1. A chat channel

The elements use an existing chat channel: its public key, its allowed origins and its rate
limits. A docs site is set up like a site with the [web widget](widget.md#1-create-a-chat-channel),
and one channel may serve both.

- Add the docs site's origin to the channel's **allowed origins**
  (`https://docs.example.com`, plus `http://localhost:3000` or whatever your dev server uses while
  you try it out). Elements on any other origin are refused and stay empty; the browser console
  says so.
- With **Anonymous visitors** off, readers can still rate pages, but feedback and questions need
  an [identity token](#signed-in-readers).
- Feedback and questions land in the channel's inbox. Replies reach readers by e-mail through the
  inbox's e-mail channel, so give the inbox one ([E-mail](email.md)).

## 2. Add the elements

The Yuva server hosts both elements at `/yuva-docs.js` (without the chat). The npm package
`useyuva` has them as `useyuva/docs`, and React wrappers in `useyuva/react`. `useyuva/docs`
registers the elements in the browser only, so importing it during server rendering is safe.
These entry points are newer than `useyuva` 0.0.6; until the next release, use the hosted script
or build `sdk/js` and depend on it by path.

### Attributes

Both elements take the same attributes:

| Attribute | Meaning |
|---|---|
| `channel` | The chat channel's public key (`yuva_pk_…`). Required. |
| `server` | Base URL of the Yuva server. Defaults to the origin `yuva-docs.js` was loaded from. |
| `locale` | `en` or `tr`. Defaults to the browser's language; other locales fall back to English. |
| `dir` | `ltr` or `rtl`. Defaults to the locale's direction. |
| `identity-token` | A signed [identity token](identity.md) for a signed-in reader. `setIdentityToken` is usually better. |
| `page` | The page's URL. Defaults to the current URL. |
| `page-title` | The page's title. Defaults to `document.title`. |

**A page is its URL without query and fragment**: `/docs/install?tab=npm#linux` and
`/docs/install` are one page. Its origin must be one of the channel's allowed origins. The
elements follow client-side navigation (`pushState`, `replaceState`, back and forward) and switch
to the new page by themselves; set `page` only to count a page under another URL. Yuva keeps the
title a page sent last.

### Properties, methods and events

```js
const feedback = document.querySelector("yuva-page-feedback");
const questions = document.querySelector("yuva-page-questions");

feedback.rating;            // "up", "down" or null for this page in this browser
await feedback.rate("up");  // the same as pressing Yes
questions.answers;          // the published answers shown
await questions.reload();   // fetch them again

feedback.addEventListener("yuva-rating", (event) => {
  console.log(event.detail); // { page, rating, previous }
});
feedback.addEventListener("yuva-feedback", (event) => {
  console.log(event.detail); // { page, rating, conversation_id }
});
questions.addEventListener("yuva-question", (event) => {
  console.log(event.detail); // { page, conversation_id }
});
```

`channel`, `server`, `locale`, `page` and `pageTitle` are also properties. `setIdentityToken` is
described under [Signed-in readers](#signed-in-readers). The events bubble out of the Shadow DOM,
so you can listen on `document`, for example to send them to your analytics.

### Plain HTML

```html
<script src="https://support.example.com/yuva-docs.js" defer></script>

<article>
  <!-- the page -->
</article>
<yuva-page-questions channel="yuva_pk_xxxxxxxxxxxxxxxx"></yuva-page-questions>
<yuva-page-feedback channel="yuva_pk_xxxxxxxxxxxxxxxx"></yuva-page-feedback>
```

Put the elements in the template every documentation page shares, usually at the end of the
article. The same page may also load `/yuva.js` and `<yuva-chat>` on the same channel.

### Nuxt Content

Checked with Nuxt 4.6 and Nuxt Content 3.16.

Register the elements in a client-only plugin:

```ts
// app/plugins/yuva.client.ts
import "useyuva/docs";

export default defineNuxtPlugin(() => {});
```

Tell Vue that `yuva-*` tags are custom elements, not Vue components, and keep the key and server
in public runtime config:

```ts
// nuxt.config.ts
export default defineNuxtConfig({
  modules: ["@nuxt/content"],
  vue: {
    compilerOptions: {
      isCustomElement: (tag) => tag.startsWith("yuva-"),
    },
  },
  runtimeConfig: {
    public: {
      yuvaChannel: "yuva_pk_xxxxxxxxxxxxxxxx",
      yuvaServer: "https://support.example.com",
    },
  },
});
```

Place the elements in the page component that renders your documents (or in the docs layout, if
every page under it should have them):

```vue
<!-- app/pages/[...slug].vue -->
<script setup lang="ts">
const route = useRoute();
const { public: config } = useRuntimeConfig();
const { data: page } = await useAsyncData(route.path, () =>
  queryCollection("content").path(route.path).first(),
);
</script>

<template>
  <article v-if="page">
    <ContentRenderer :value="page" />
    <yuva-page-questions :channel="config.yuvaChannel" :server="config.yuvaServer" :page-title="page.title" />
    <yuva-page-feedback :channel="config.yuvaChannel" :server="config.yuvaServer" :page-title="page.title" />
  </article>
</template>
```

During server rendering the tags are rendered as they are; the plugin upgrades them in the
browser. Without `isCustomElement` Vue warns that it cannot resolve the components.

### Fumadocs

Checked with Fumadocs UI 16.17 on Next.js (the `create-fumadocs-app` template).

The React wrappers `YuvaPageFeedback` and `YuvaPageQuestions` use hooks, and the docs page
(`app/docs/[[...slug]]/page.tsx`) is a server component, so wrap them in a client component:

```tsx
// components/yuva.tsx
"use client";

import { YuvaPageFeedback, YuvaPageQuestions } from "useyuva/react";

const channel = process.env.NEXT_PUBLIC_YUVA_CHANNEL!;
const server = process.env.NEXT_PUBLIC_YUVA_SERVER;

export function PageQuestions({ title }: { title: string }) {
  return <YuvaPageQuestions channel={channel} server={server} pageTitle={title} />;
}

export function PageFeedback({ title }: { title: string }) {
  return <YuvaPageFeedback channel={channel} server={server} pageTitle={title} />;
}
```

Use them in the docs page, after `DocsBody`:

```tsx
// app/docs/[[...slug]]/page.tsx
import { PageFeedback, PageQuestions } from "@/components/yuva";

export default async function Page(props: PageProps<"/docs/[[...slug]]">) {
  // ...
  return (
    <DocsPage toc={page.data.toc} full={page.data.full}>
      <DocsTitle>{page.data.title}</DocsTitle>
      <DocsDescription>{page.data.description}</DocsDescription>
      <DocsBody>
        <MDX components={getMDXComponents()} />
      </DocsBody>
      <PageQuestions title={page.data.title} />
      <PageFeedback title={page.data.title} />
    </DocsPage>
  );
}
```

The wrappers take the attributes as props (`channel`, `server`, `locale`, `dir`, `page`,
`pageTitle`, `identityToken`, `className`, `style`) and the events as callbacks: `onRating` and
`onFeedback` on `YuvaPageFeedback`, `onQuestion` on `YuvaPageQuestions`.

**Fumadocs' own feedback component.** Fumadocs offers a `Feedback` component that you add to
your project with `npx @fumadocs/cli@latest add feedback`; it lands in
`components/feedback/client.tsx` and passes `{ opinion, url, message }` to an `onSendAction` you
write (Fumadocs' example posts it to GitHub Discussions). It is not in a new project unless you
added it. If you have it:

- **Replace it**: remove `<Feedback onSendAction={…} />` from the docs page and put
  `<PageFeedback />` where it was. `FeedbackText`, its per-paragraph variant, is separate and can
  stay.
- **Keep it**: leave `<Feedback />` as it is and add only `<PageQuestions />`. Yuva then shows no
  ratings for those pages.

### Other generators

- **Starlight**: override the `Footer` component (`components: { Footer: "./src/components/Footer.astro" }`),
  render the default footer and the two elements in it, and load `/yuva-docs.js` or
  `import "useyuva/docs"` in a `<script>`.
- **VitePress**: extend the default theme, put the elements in the `doc-after` layout slot, import
  `useyuva/docs` in `enhanceApp` when running in the browser, and set
  `vue.template.compilerOptions.isCustomElement` in the config as for Nuxt.
- **Docusaurus**: swizzle `DocItem/Footer` (wrap), add the elements, and load `/yuva-docs.js`
  through `scripts` in the config.
- **MkDocs, Hugo, Jekyll and other static generators**: add the script and the elements to the
  page template, as in [plain HTML](#plain-html).

## Signed-in readers

If readers sign in to your docs (or to your product on the same site), give the elements an
[identity token](identity.md) and their feedback and questions come from the contact you already
know. They see no e-mail field, and replies go to the address in the token.

```js
const token = async () => {
  const response = await fetch("/api/yuva-identity-token", { credentials: "same-origin" });
  return response.ok ? (await response.json()).token : null;
};
for (const element of document.querySelectorAll("yuva-page-feedback, yuva-page-questions")) {
  element.setIdentityToken(token);
}
```

With React, pass the same function as `identityToken`. Return `null` for a reader who is not
signed in. Ratings never use the token: they stay anonymous counters.

## Languages

Each URL is its own page. A site with `/en/docs/install` and `/de/docs/install` has two pages in
Yuva, each with its own ratings, feedback, questions and published answers, which is usually what
you want: a German page can be unclear where the English one is not. Set `locale` (and `dir` for
right-to-left languages) to the page's language so the elements speak it where Yuva has a
translation. To count every language under one URL, pass that URL as `page`.

## Styling

The elements render in Shadow DOM: your styles do not reach inside and theirs do not leak out.
They take `color` and `font` from where they are placed, follow light and dark mode, and are
`display: block`; give them margins from outside. Inside they use these custom properties:

| Property | Used for |
|---|---|
| `--yuva-accent` | Buttons, the selected rating, focus rings, links in answers. |
| `--yuva-on-accent` | Text on the accent. |
| `--yuva-border` | Borders of buttons, fields and answers. |
| `--yuva-soft` | The selected rating's background, code in answers. |
| `--yuva-danger` | Error messages. |

The accent is the chat channel's launcher colour (or the workspace's brand colour), which the
element sets on itself. To use your site's colour instead, set it with `!important`:

```css
yuva-page-feedback,
yuva-page-questions {
  margin-block: 2rem;
  --yuva-accent: var(--color-fd-primary) !important;
  --yuva-on-accent: var(--color-fd-primary-foreground) !important;
  --yuva-border: var(--color-fd-border);
}
```

(The variables are Fumadocs'; use your theme's own elsewhere.)

## In the panel

- **Docs view.** Lists the pages of the inboxes you can see over the last 7, 30 or 90 days: `up`
  and `down` counts, the share of `down`, open feedback and questions, and published answers,
  sorted by most `down` (default), most `up`, most helpful or most activity, with totals and a
  chart per day above. A page shows its ratings per day, its feedback and questions, and its
  published answers.
- **Publish to page.** A `question` conversation with a member reply offers "Publish to page":
  edit the question and the answer, then publish. A conversation publishes once; edit or
  unpublish the answer afterwards. It shows on the page within a minute, since the list is cached
  for that long.
- Feedback and questions are conversations: they appear in the inbox and send the same events and
  webhooks as any other. Ratings are counters and send no events or webhooks.

## API

| Endpoint | Used by |
|---|---|
| `POST /client/v1/channels/{channel_key}/page-ratings` | `<yuva-page-feedback>`: count a rating, no session. |
| `POST /client/v1/feedback` | `<yuva-page-feedback>`: feedback with `page_url`, `page_title` and `rating`. |
| `POST /client/v1/questions` | `<yuva-page-questions>`: ask a question. |
| `GET /client/v1/channels/{channel_key}/page-answers?page=` | `<yuva-page-questions>`: a page's published answers, no session, cached for a minute. |
| `POST /v1/conversations/{conversationId}/publish` | Publish a question and its answer (member sessions). |
| `GET /v1/page-answers`, `GET`, `PATCH`, `DELETE /v1/page-answers/{pageAnswerId}` | List, edit and unpublish published answers. |
| `GET /v1/docs/pages`, `GET /v1/docs/summary`, `GET /v1/docs/page` | The Docs view: pages with counts, totals per day, one page. |
| `GET /v1/conversations?page=` | A page's feedback and questions (with `kind=feedback` or `kind=question`). |

Details are in the [API contract](../openapi/openapi.yaml).

## Content Security Policy

If the docs site sends a CSP, allow the Yuva server for the script (when you load
`/yuva-docs.js`) and for requests:

```
script-src  'self' https://support.example.com;
connect-src 'self' https://support.example.com;
```

## Troubleshooting

- **The elements stay empty**: the page's origin is not in the channel's allowed origins, the key
  is wrong, or it is an app channel's key. The browser console names the channel and the page.
- **"Sign in to write to us." or "Sign in to ask a question."**: the channel does not allow
  anonymous visitors and no identity token was given. Ratings still work.
- **"Too many attempts."**: ratings, feedback and questions are rate limited per IP address and
  per channel.
- **A published answer does not show yet**: the list is cached for a minute.
