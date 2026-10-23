# Web widget

`<yuva-chat>` puts live chat on your website, or a conversation thread inside your own product. It
works in any page or framework.

## 1. Create a chat channel

In the panel: Settings → the inbox → Channels → Chat. Add every origin that may show the widget,
such as `https://www.example.com`, then copy the channel's public key (`yuva_pk_…`). The key is not
a secret.

## 2. Embed it

```html
<script src="https://support.example.com/yuva.js" defer></script>
<yuva-chat channel="yuva_pk_xxxxxxxxxxxxxxxx"></yuva-chat>
```

This shows a floating launcher. With a bundler: `npm install useyuva`, `import "useyuva/chat"` and
set the `server` attribute. In Nuxt: `modules: ["useyuva/nuxt"]`, `yuva: { server, channel }` in
`nuxt.config` and `<YuvaChat />`. If your site sends a Content Security Policy, allow your Yuva
server in `script-src` and `connect-src` (`https://` and `wss://`).

## 3. Signed-in users

Your backend signs an [identity token](identity.md) for the current user; the widget asks for it:

```js
document.querySelector("yuva-chat").setIdentityToken(async () => {
  const response = await fetch("/api/yuva-identity-token");
  return response.ok ? (await response.json()).token : null;
});
```

Return `null` for anonymous visitors. Call it again after sign-in, and `signOut()` after sign-out.

## What visitors see

The panel opens on a home screen: who is available, the latest conversation to continue and a button
to write a message. A conversation shows day separators, "joined" and "closed" lines, attachment
cards (uploading, failed and sent), a typing indicator, "Seen" under the last message the team has
read, and a jump-to-latest button with the unread count when a visitor has scrolled up. The composer
takes files, pasted images and, where the browser allows it, a screenshot of the page.

Home also opens apps inside the same panel, each only when the channel has what it needs, and loads
them on first use. **Feedback** sends a category, text and files as a feedback conversation that
the visitor finds in their messages. **Status** shows the inbox's published status page: the overall
state, the components with their last 60 days, and open and recent incidents. **Get updates**
subscribes the visitor to the inbox's public mailing lists and sends the consent text as it was
shown. **Help center** appears when the inbox has a published [help center](help-center.md), together
with a search field on Home: it shows the articles that fit the page the visitor is on, the popular
ones and the collections, opens articles inside the panel with "Was this helpful?" and ends with a
"Message us" button. A conversation started after reading an article records it, and an article your
team sends in a reply opens in the same app. **Roadmap** appears when the inbox has a public [roadmap board](roadmap-board.md): ideas by stage and votes, an idea with its replies, and Suggest an idea. Voting and suggesting need a sign-in, which opens the reader window unless the visitor has an identity token. **What's new** appears when the inbox has a published [What's new](whats-new.md) changelog with an entry: a tile with the unread count, then the entries newest first with labels, date, picture and text.

A message a member removed shows as "This message was removed", in the conversation and in its preview on
the conversations list, and changes live while the panel is open.

When nobody is available and the channel asks for an e-mail address, the thread asks for one. After
the visitor saves it, the thread says a confirmation link went out (with Change address and Send
again), asks again when the address bounced or the link ran out after 24 hours, and shows a quiet
line once it is confirmed. The panel follows `prefers-color-scheme`, uses the channel's launcher
colour as its accent and the page's Geist font when it has one, else the system font.

Replies are e-mailed to a typed address only after the visitor confirms it: anyone can type any address, so
nothing but the confirmation link goes to it before. Confirmation links are limited: 3 a day per address from
one network, 10 a day per address on the server, 6 a day per visitor, 30 a day per network and workspace, and
2,000 a day per workspace. When a limit holds a link back, none goes out and the visitor's
`email_confirmation` has the state `limited` ([details](headless.md#the-visitors-e-mail-address)). An `email`
in an identity token that your backend has not marked `email_verified` gets one confirmation link in the same
way, while `email_verified: true` makes it the visitor's address at once.

## What the widget stores

The widget keeps its session in your page's `localStorage`, so a visitor's conversation survives a
reload. Everything is scoped to your page's origin.

| Key | Holds |
|---|---|
| `yuva:<channel>` | The session token and its expiry, the anonymous visitor id and the launcher state. |
| `yuva:reader:<server> <channel>` | The reader token, after a reader signs in on a documentation page. |

The tokens are random strings that Yuva stores only hashed and can end at any time. Because the
storage belongs to your origin, a script injected into your page (XSS) can read them and act as that
visitor until the token expires or is ended. What you can do:

- **Sign out.** `signOut()` ends the session on the server and removes it from storage; the visitor id
  and launcher state stay. `readerSignOut()` ends the reader token and clears its key. Call them when
  your user signs out of your app.
- **Use identity tokens** for signed-in users. A [short-lived token](identity.md) signed by your
  backend proves who the user is, so the stored session is not the only thing that vouches for them.
- **Send a Content Security Policy** that limits `script-src` to sources you trust; it is the main
  protection against injected scripts.
- **Keep the token out of `localStorage`** when you build your own UI: `createYuvaClient()` takes a
  `storage` option, any object with `getItem` and `setItem`, for example one that wraps
  `sessionStorage` or keeps the values in memory. The `<yuva-chat>` element has no such option and
  always uses `localStorage`; if `localStorage` is unavailable, memory is used.

## Troubleshooting

- **Nothing appears**: the page's origin is not allowed (the console names it), or the key is wrong.
- **"Sign in to chat with us."**: the channel is closed to anonymous visitors and no token was given.

## Reference

### Attributes

| Attribute | Meaning |
|---|---|
| `channel` | The chat channel's public key. Required. |
| `server` | Yuva server URL. Defaults to where `yuva.js` came from. |
| `layout` | `launcher` (default) or `embedded`, for a thread inside your page; give it a height. |
| `locale` | One of the 47 [languages](#languages), such as `en`, `tr`, `de`, `pt-BR` or `ar`. Defaults to the browser's language; a language Yuva does not have falls back to English. |
| `dir` | `ltr` or `rtl`. Defaults to the locale's direction. |
| `identity-token` | A token string; `setIdentityToken` is usually better. |
| `open` | Present: the panel is open. |

### Languages

The widget speaks 47 languages: `en`, `tr`, `de`, `fr`, `fr-CA`, `es`, `es-MX`, `pt-BR`, `pt-PT`, `it`, `nl`, `sv`, `da`, `nb`, `fi`, `pl`, `cs`, `sk`, `sl`, `hr`, `hu`, `ro`, `ca`, `ja`, `ko`, `zh-Hans`, `zh-Hant`, `ru`, `uk`, `el`, `id`, `ms`, `ar`, `he`, `ur`, `hi`, `bn`, `mr`, `pa`, `gu`, `or`, `kn`, `ml`, `ta`, `te`, `th`, `vi`. Arabic, Hebrew and Urdu lay out right to left. A tag such as `pt-BR`, `zh-TW` or `es-AR` picks the closest of these; `no` and `nn` use `nb`. A language not in the list falls back to English.

### Methods and events

| Member | Meaning |
|---|---|
| `open()`, `close()`, `toggle()`, `isOpen` | The launcher's panel. |
| `unread`, `yuva-unread` event | Replies not yet seen; the event has `detail.count`. |
| `setIdentityToken(fn)`, `signOut()` | Signed-in users, as above. |
| `rate(conversationId, "good" \| "bad", comment?)` | Rates a closed conversation; `409 already_rated` or `rating_unavailable` when it cannot. |
| `openApp(kind, params?)` | Opens the panel on an app: `"help"` (`{ article }` or `{ query }`), `"feedback"`, `"status"`, `"subscribe"` `"roadmap"` (`{ idea }` opens that idea) or `"whats-new"` (`{ entry }` opens that entry). |

### Channel settings

| Setting (API field) | Meaning |
|---|---|
| Allowed origins (`allowed_origins`) | 1 to 20 origins, compared exactly. |
| Anonymous visitors (`allow_anonymous`) | Visitors without an identity token may chat. |
| Ask for e-mail (`ask_email_offline`) | Ask for an address when nobody is available. Unread replies are mailed after `YUVA_CHAT_EMAIL_DELAY` (default 5 minutes). Needs an e-mail channel in the inbox. |
| Greeting (`greeting`) | Shown before the first message. |
| Launcher (`launcher`) | `position` and `color` (`#rrggbb`). |
| Announce updates (`announce_updates`) | Off by default. The launcher shows the number of unread [What's new](whats-new.md#4-in-the-widget) entries while no messages are unread. |

Over the API: `POST /v1/inboxes/{inboxId}/channels` with `kind: "chat"` and a `chat` object, as an
owner or admin.

A chat channel can also narrow the server's [attachment limits](configuration.md#attachments-and-storage) with
`attachments`. The widget reads them: it refuses a file that is too large or of another type with a note before uploading it,
restricts the file picker to the allowed types, and hides the attach button and the screenshot picker when no
type is allowed. Channel owners and admins set them in the panel under the channel's settings, Attachments.
