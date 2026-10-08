# Web widget

`<yuva-chat>` is a web component that puts live chat on a website, or a conversation thread inside
your own product's panel. It works in any page or framework, renders in Shadow DOM (your styles do
not leak in, its styles do not leak out) and is translated (English and Turkish; other locales
fall back to English).

## In short

- [Create a chat channel](#1-create-a-chat-channel) and allow your site's origin.
- [Embed](#2-embed-it) one script tag and the `<yuva-chat>` element.
- Pass an [identity token](#3-signed-in-users) to let signed-in users write as themselves.

## 1. Create a chat channel

In the panel: Settings → the inbox → Channels → Chat. Set:

- **Allowed origins**: every origin whose pages may load the widget, exactly as the browser sends
  it (`https://www.example.com`, scheme and host and port, no path). Requests from other pages are
  refused and the widget stays hidden.
- **Anonymous visitors**: on for a public website; off when only signed-in users of your app may
  write (they need an [identity token](identity.md)).
- Ask for an e-mail address when nobody is available, a greeting, and the launcher position and
  colour.

The channel's **public key** (`yuva_pk_…`) identifies it to the widget. It is not a secret; you can
rotate it in the panel.

Over the API the same is `POST /v1/inboxes/{inboxId}/channels` with `kind: "chat"` and a `chat`
object (`allowed_origins`, `allow_anonymous`, `ask_email_offline`, `greeting`, `launcher`), called
by an owner or admin signed in to the panel (API keys cannot manage inboxes and channels).

## 2. Embed it

The server hosts the widget at `/yuva.js`. Add it once per page with the element:

```html
<script src="https://support.example.com/yuva.js" defer></script>
<yuva-chat channel="yuva_pk_xxxxxxxxxxxxxxxx"></yuva-chat>
```

This shows a floating launcher. `yuva.js` is small; the chat itself (`/yuva-chat.js`) loads when
the visitor opens it or when they already have a conversation.

### Embedded thread

Inside your own panel, use the embedded layout and give the element a size:

```html
<yuva-chat channel="yuva_pk_xxxxxxxxxxxxxxxx" layout="embedded" style="display:block;height:600px"></yuva-chat>
```

### Attributes

| Attribute | Meaning |
|---|---|
| `channel` | The chat channel's public key. Required. |
| `server` | Base URL of the Yuva server. Defaults to the origin `yuva.js` was loaded from. |
| `layout` | `launcher` (default) or `embedded`. |
| `locale` | `en` or `tr`. Defaults to the browser's language. |
| `dir` | `ltr` or `rtl`. Defaults to the locale's direction. |
| `identity-token` | A signed identity token (see below). The method `setIdentityToken` is usually better. |
| `open` | Opens the launcher's panel; removing it closes the panel. |

Changing `channel` or `server` restarts the widget.

### Methods, properties and events

```js
const chat = document.querySelector("yuva-chat");

await chat.open();
chat.close();
chat.toggle();
chat.isOpen;   // boolean
chat.unread;   // replies the visitor has not seen

chat.addEventListener("yuva-unread", (event) => {
  console.log(event.detail.count);
});
```

## 3. Signed-in users

On a site where people are signed in, let them chat as themselves: your backend signs a short-lived
identity token for the current user ([Identity tokens](identity.md)), and the widget asks for one
whenever it starts a session:

```js
chat.setIdentityToken(async () => {
  const response = await fetch("/api/yuva-identity-token", { credentials: "same-origin" });
  return response.ok ? (await response.json()).token : null;
});
```

Return `null` for an anonymous visitor. When the user signs in later, call `setIdentityToken`
again: the visitor's earlier conversations move to the signed-in contact. When the user signs out
of your app, end their Yuva session too:

```js
await chat.signOut();
```

The user then sees their conversations on every device and in your mobile apps, and the panel shows
the name, e-mail and attributes from the token.

## What the visitor sees

- Their own conversations of the inbox: messages, attachments and status; never notes or internal
  events, and of members only the display name and initials.
- In a `live` inbox, who is available (members with access, online in the panel, not away, within
  business hours), typing and read receipts. In an `async` inbox, the expected reply time.
- When nobody answers while they are on the page, they can leave an e-mail address; a reply they
  have not read is e-mailed to them after `YUVA_CHAT_EMAIL_DELAY`. Yuva also mails the address a
  confirmation link; once they confirm it, their answers by e-mail continue the conversation. This
  needs an e-mail channel in the same inbox.

## Content Security Policy

If your site sends a CSP, allow the Yuva server for scripts and connections (HTTP and WebSocket):

```
script-src  'self' https://support.example.com;
connect-src 'self' https://support.example.com wss://support.example.com;
```

## Troubleshooting

- **Nothing appears**: the page's origin is not in the channel's allowed origins (the browser
  console names the origin), or the key is wrong. The widget does not retry.
- **"Sign in to chat with us."**: the channel does not allow anonymous visitors and no identity token
  was given.
- **Signed-in users appear as new contacts**: the token's `sub` must be the same user id every time.
