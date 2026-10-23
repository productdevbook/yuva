# Help center

A help center answers common questions in writing, where customers look first: public pages for your
product with collections and articles in several languages, searchable from the page, from the home
screen of the [web widget](widget.md) and from the reply box of your team. It belongs to an inbox, so
a customer who still needs help writes to the same inbox, and the searches that found nothing tell you
which article to write next.

The public pages are plain HTML rendered by your Yuva server, without a panel and without required
JavaScript. Each inbox has at most one help center. Product documentation stays on your own site; the
help center holds support articles written in Yuva.

## 1. Create and publish a center

In the panel: Help in the sidebar, then create a help center on the inbox. Owners and admins create
and change centers; see [who may publish](#drafts-and-publishing) for articles.

| Setting (API field) | Meaning |
|---|---|
| Name (`name`) | Up to 100 characters. |
| Description (`description`) | Up to 1,000 characters, shown on the home page. |
| Address (`slug`) | 3 to 48 characters: `a-z`, `0-9` and inner hyphens. Unique on the server; some words are reserved. |
| Languages (`locales`), default (`default_locale`) | Up to 20 [languages](widget.md#languages) the center publishes; the default is one of them. |
| Logo (`logo_url`), accent colour (`accent_color`) | An `https` image and a colour such as `#2563eb`. |
| Site link (`site_url`) | Your product, an `http(s)` link. |
| Chat (`chat_channel_id`) | A [chat channel](widget.md) whose chat the pages offer under "Still need help?". A conversation started there carries the article the visitor read. |
| Analytics (`analytics_property_id`) | A website of [Analytics](analytics.md) that counts cookieless page views of the public pages. |
| Agents may publish (`agents_may_publish`) | Off by default. See below. |
| Published (`published`) | Off by default. An unpublished center answers `404` to everyone but your team, which previews it in the panel. |

Renaming the address keeps the old one: it stays reserved for the center and every public address
under it redirects to the new one.

Over the API: `POST /v1/help-centers` with `inbox_id`, `name` and `slug`, then
`PATCH /v1/help-centers/{centerId}` with `{"published": true}`.
`GET /v1/help-centers/{centerId}/preview` returns a page as visitors would see it, also while the
center is unpublished.

## 2. Collections and articles

A *collection* groups articles, such as "Billing" or "Getting started", with an icon from a fixed set
and one level of sub-collections at most. An *article* is in one collection or none and has one
*translation* per language. Collections have a name and a description per language; one with no
published article in a language is left out in that language.

An article is written in Markdown: headings, lists, tables, code, links and images. Raw HTML is
removed, and images stay only when you uploaded them to your workspace. Upload from the editor, or
`POST /v1/help-centers/{centerId}/images` (same types, sizes and re-encoding as [e-mail
images](designs.md#4-images); the answer carries the Markdown to paste). An image that no draft uses
and no published article showed is deleted a week after its upload.

| Field | Limit |
|---|---|
| Title (`title`) | 3 to 200 characters. |
| Body (`body`) | Markdown, up to 100,000 characters. |
| Keywords (`keywords`) | Up to 500 characters of extra search words, such as synonyms. |
| Address (`slug`) | Made from the title, editable. A wrong slug in a URL redirects to the right one, so renaming an article breaks no link. |
| Pages (`paths`) | Up to 20 page paths such as `/billing/*`. The widget suggests the article as a best match on those pages. |

A workspace has at most `YUVA_HELP_MAX_ARTICLES` articles ([Configuration](configuration.md#help-center));
beyond that, creating one answers `422 hosted_limit`.

Over the API, under `/v1/help-centers/{centerId}`: `/collections`, `/articles` and
`/articles/{articleId}`. `GET /articles` filters by `collection_id`, `locale` and `state`.

## Drafts and publishing

A translation has a working draft and, once published, a frozen published copy. Saving changes only
the draft; the public page changes when you publish.

- **Saving.** Every save carries the `base_version` the editor loaded. If someone saved in between, the
  save is refused with `409 version_conflict` and the answer holds the translation as it is now, so
  nobody overwrites another person's work. Other open editors are told that the article changed.
- **Publishing.** Publish freezes the draft, renders it and puts it on the web; unpublish takes the
  translation off the web and keeps its draft. A translation shows "changes not published" while its
  draft differs from the published copy.
- **Who may publish.** Agents with access to the inbox write drafts, collections and translations.
  Owners and admins also publish, unpublish, delete published articles and change the center's
  settings. Turn on Agents may publish (`agents_may_publish`) to let agents with access to the inbox
  publish too.
- **Assistants and OAuth.** [MCP](mcp.md#help-center-from-an-assistant) tools and OAuth tokens never
  publish, whatever the setting: an assistant writes a draft and a member publishes it. API keys can
  publish with the scope `help:publish`.

Over the API: `PUT /v1/help-centers/{centerId}/articles/{articleId}/translations/{locale}` saves a
draft, and `POST` to the same path with `/publish` or `/unpublish` publishes or unpublishes it.
[Webhooks](webhooks.md) send `help.article.published`, `help.article.unpublished` and
`help.article.deleted`.

## Languages and translations

A center publishes the languages in `locales`; articles and collections are written in those only
(`400 locale_not_offered` otherwise). Yuva runs no translation model: a translation is text you or
your team write. The editor shows the default-language text next to the one you are translating, marks
translations older than the default and lists the missing ones.

The public pages follow the language in their address (`/en/…`, `/tr/…`); Yuva does not redirect from
the browser's language. Every page lists its other published languages for search engines. The words
of the page itself (search, "Was this helpful?") exist in English, Turkish and German; other languages
show English. The widget picks the visitor's language from the center's list (exact match, then the
base language, else the default) and shows an article in the default language when it has no
translation in theirs.

## Revisions

Each translation keeps its last 50 revisions: a saved draft (at most one every 10 minutes per author),
every publish and every restore. Open the history of a translation and restore a revision into the
draft; it is published only when you publish. Over the API:
`GET …/translations/{locale}/revisions` and `POST …/revisions/{revisionId}/restore`.

## Public pages

Each published center is at `/help/{slug}` on your server, or at `<YUVA_HELP_URL>/{slug}`. Under it:

| Address | Page |
|---|---|
| `/{locale}/` | Home: search, collections and popular articles. |
| `/{locale}/collections/{number}-{slug}` | A collection with its sub-collections and articles. |
| `/{locale}/articles/{number}-{slug}` | An article with a table of contents, related articles and "Was this helpful?". |
| `/{locale}/search?q=` | Search results. |
| `/sitemap.xml` | Every published page, for search engines. |

"Was this helpful?" works without JavaScript: the Yes and No buttons post a form to
`/{locale}/articles/{number}-{slug}/helpful` and the page comes back with `?thanks=up` or
`?thanks=down`, showing a thank-you line and a button to change the answer. A script posts the form in
place when it can. Pages are cached for a minute and set a canonical link, alternates for every
language, Open Graph tags and structured data. Public pages answer 120 requests a minute per address.

`YUVA_HELP_URL` is an origin you point at the server, such as `https://help.example.com`. Set it in
[Configuration](configuration.md#help-center); centers are then at `<origin>/{slug}`, and the origin must differ from `YUVA_PUBLIC_URL`.
A center can also have its own domain, such as `help.example.com`: add it in the center's settings,
create the CNAME and TXT records shown there, and once they are verified the center answers at `/` on that
domain (the server setting is `YUVA_STATUS_DOMAINS` in [Configuration](configuration.md)). `YUVA_HELP=off` removes help centers altogether: their
routes answer `404`.

## Search and the report

The panel, the widget, the public pages and assistants share one full-text search per language. All
words must match and the last one matches as a prefix, so results appear while typing; when no article
has every word, articles with some of them are shown. Titles weigh most, then keywords, then the body.
Members' searches in the panel are not counted.

The report (Help → the center → Report, or `GET /v1/help-centers/{centerId}/report?days=7|30|90`)
shows, for the last 7, 30 or 90 days:

- the most viewed articles, the share of Yes against No, conversations started from an article and
  times it was sent as a card;
- searches, and the searches that found nothing, most frequent first: the articles to write next;
- the articles with the most No answers: the ones to improve.

Counters keep no visitor data. Views are approximate: one per address, article and hour, and pages a
CDN served are not seen. Searches that look like an e-mail address, a URL or a number are counted as
`(scrubbed)`.

## In the web widget

When the inbox has a published center, the [widget](widget.md#what-visitors-see)'s home screen gets a
search field and a Help center app. It shows the best matches for the page the visitor is on (from
the articles' `paths`), the popular articles and the collections, opens articles inside the panel
and asks "Was this helpful?". A button at the bottom starts a conversation. When the visitor writes
after reading an article, the conversation records it as `help_article_id`, and your team sees which
article the customer read first. The id is kept only for a published article of the inbox's center.

For your own interface, the [shadcn components](https://useyuva.com/shadcn) `yuva-help`,
`yuva-help-search`, `yuva-help-article` and the hooks in `use-yuva-help` use the same
`/client/v1/channels/{channel_key}/help` endpoints of the [headless client](headless.md).

## Article cards in conversations

In a conversation, the Help button of the reply box and the `/` search find published articles. Pick
up to three and send: the customer gets a card with the title and an excerpt in their language (the
contact's, else the inbox's, else the center's default). In the chat the card opens the article inside
the widget; by e-mail the text part has "Title - URL" and the HTML part a block with a link, without
tracking. Over the API, `POST /v1/conversations/{id}/messages` takes `article_ids`, up to 3 published
articles of the inbox's center. If you unpublish an article later, old messages stay readable and the
link shows the center's not-found page.

## Import from Zendesk or Help Scout

Bring a help center you already have into this one, as drafts. Nothing is published and the source
is only read.

- **Zendesk Guide.** Use a Zendesk migration that is signed in (Move to Yuva); its read-only sign-in
  is reused, no new permission is asked. Categories become collections, sections their
  sub-collections (a section inside a section becomes `Parent / Child`). Every article comes with
  each translation in a language the center offers; other languages are listed as skipped, so add the
  language to the center's locales and import again.
- **Help Scout Docs.** Create a Docs API key in Help Scout (Your Profile, API Keys) and give it to the
  import; Yuva keeps it sealed until the import ends. Collections become collections, categories their
  sub-collections. Docs sites have no languages: pick the one the articles are written in.
- **What changes on the way.** Article HTML becomes Markdown. Images are downloaded, stored as the
  center's images and rewritten; one that cannot be fetched stays a link. Links between imported
  articles point to their new pages.
- **Run it again.** An article imported before updates its draft and is never duplicated. A draft you
  edited in Yuva since is kept unless you ask to overwrite it.
- **Redirects.** The import lists each old URL with its new URL (download it as CSV). Setting up
  the redirects at the old address is up to you, and a new URL answers `404` until you publish the
  article.

Start it with `POST /v1/help-centers/{id}/imports` (scope `help:write`); an assistant can start a
Zendesk import and follow it with the `import_help_articles` and `get_help_import` tools.

## API

All of it is under `/v1/help-centers` with the tag `help` in the [API reference](headless.md).

| Scope | Allows |
|---|---|
| `help:read` | Centers, collections, articles with drafts and revisions, search, report. Given to keys made without named scopes. |
| `help:write` | Create and change collections and articles, save drafts, upload images. Given to keys made without named scopes. |
| `help:publish` | Publish, unpublish, and delete published articles (with `help:write`). Never implied: name it. |

A key limited to inboxes sees only the centers of those inboxes; any other center answers `404`.
Existing keys gain none of these scopes.
