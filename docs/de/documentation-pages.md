# Dokumentationsseiten

Bringen Sie „War diese Seite hilfreich?“ und ein Fragefeld auf Ihre Dokumentationsseite und lesen Sie
die Antworten im Yuva-Posteingang. Ihre Seiten bleiben, wo sie sind; zwei Web Components senden
Bewertungen, Feedback und Fragen an Yuva.

## 1. Ein Chat-Kanal

Verwenden Sie einen Chat-Kanal, eingerichtet wie für das
[Web-Widget](widget.md#1-einen-chat-kanal-anlegen); ein Kanal kann beides bedienen.

- Tragen Sie die Origin der Doku-Seite in die **Allowed origins** des Kanals ein
  (`https://docs.example.com`, dazu Ihren Entwicklungsserver, etwa `http://localhost:3000`). Auf
  jeder anderen Origin bleiben die Elemente leer.
- Geben Sie dem Posteingang einen [E-Mail-Kanal](email.md): Antworten erreichen Leser per E-Mail.

## 2. Die Elemente einbauen

### Reines HTML

```html
<script src="https://support.example.com/yuva-docs.js" defer></script>

<yuva-page-questions channel="yuva_pk_xxxxxxxxxxxxxxxx"></yuva-page-questions>
<yuva-page-feedback channel="yuva_pk_xxxxxxxxxxxxxxxx"></yuva-page-feedback>
```

Setzen Sie sie in die Vorlage, die alle Doku-Seiten teilen, nach dem Artikel. Ihr Yuva-Server
liefert `/yuva-docs.js` aus.

Auf npm gibt es sie als `useyuva/docs` und `useyuva/react` ab dem Release nach `useyuva` 0.0.6.
Bis dahin nutzen Sie das gehostete Skript oder binden `sdk/js` per Pfad ein, wie es die Beispiele
tun.

### Nuxt Content

Lauffähiges Beispiel: [`examples/nuxt-content`](../../examples/nuxt-content)
([so starten Sie es](../../examples/README.md#nuxt-content)). Die Zeilen, die Sie ergänzen:

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
<!-- app/pages/[...slug].vue, nach <ContentRenderer> -->
<yuva-page-questions :channel="config.yuvaChannel" :server="config.yuvaServer" :page-title="page.title" />
<yuva-page-feedback :channel="config.yuvaChannel" :server="config.yuvaServer" :page-title="page.title" />
```

`config` ist `useRuntimeConfig().public`. Auf einer frischen Seite scheitert `nuxt build` mit „Nuxt
Content requires better-sqlite3“: Installieren Sie es, oder setzen Sie
`content.experimental.sqliteConnector: "native"` wie im Beispiel. Die Seite fragt eine
`content`-Collection ab; definieren Sie sie wie in der
[`content.config.ts`](../../examples/nuxt-content/content.config.ts) des Beispiels.

### Fumadocs

Lauffähiges Beispiel: [`examples/fumadocs`](../../examples/fumadocs)
([so starten Sie es](../../examples/README.md#fumadocs)). Die Doku-Seite ist eine Server Component;
packen Sie die React-Komponenten deshalb in eine Client Component:

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
// app/docs/[[...slug]]/page.tsx, nach <DocsBody>
<PageQuestions title={page.data.title} />
<PageFeedback title={page.data.title} />
```

Haben Sie die eigene `<Feedback>`-Komponente von Fumadocs eingebaut, ersetzen Sie sie durch
`<PageFeedback />`, oder behalten Sie sie und ergänzen nur `<PageQuestions />`. Ist `useyuva` per
Pfad aus `sdk/js` verlinkt, kann Next eine zweite Kopie von React laden; setzen Sie
`turbopack.root` wie in der `next.config.mjs` des Beispiels.

### Andere Generatoren

Laden Sie das Skript (oder `useyuva/docs`) und setzen Sie die Elemente unter den Artikel:

- **Starlight**: die Elemente in einen überschriebenen `Footer` einfügen.
- **VitePress**: den Slot `doc-after` nutzen; `isCustomElement` wie bei Nuxt setzen.
- **Docusaurus**: `DocItem/Footer` swizzlen (wrap).
- **MkDocs, Hugo, Jekyll und andere**: wie [reines HTML](#reines-html).

## Was Leser und Ihr Team sehen

- **Leser** bewerten eine Seite mit Ja oder Nein (ein anonymer Zähler) und können danach Feedback
  schreiben; jeder kann eine Frage stellen. Beides nimmt eine optionale E-Mail-Adresse für die
  Antwort und bleibt privat.
- **Ihr Team** bekommt Feedback und Fragen im Posteingang als Unterhaltungen vom Typ `feedback` und
  `question`. Die Ansicht **Docs** listet Seiten mit ihren Bewertungen, ihrem Feedback und ihren
  Fragen über 7, 30 oder 90 Tage. Bei einer beantworteten Frage setzt **Publish to page** die
  bearbeitete Frage und Antwort innerhalb einer Minute unter „Questions and answers“ auf diese
  Seite.

## Gut zu wissen

- **Eine Seite ist ihre URL ohne Query und Fragment.** URLs pro Sprache (`/en/docs/install`,
  `/de/docs/install`) sind getrennte Seiten mit eigenen Bewertungen; übergeben Sie eine URL als
  `page`, um sie zusammen zu zählen, und setzen Sie `locale` auf die Sprache der Seite.
- **Angemeldete Leser**: Geben Sie den Elementen mit `setIdentityToken` ein
  [Identitäts-Token](identity.md) (in React die Prop `identityToken`). Sie sehen dann kein
  E-Mail-Feld, und Antworten gehen an die Adresse im Token. Ohne **Anonymous visitors** am Kanal
  brauchen Feedback und Fragen ein Token; Bewertungen nie.
- **Content Security Policy**: Erlauben Sie den Yuva-Server in `script-src` (für `/yuva-docs.js`)
  und `connect-src`.
- **Elemente bleiben leer**: Die Origin ist nicht erlaubt oder der Schlüssel ist falsch; siehe die
  Browser-Konsole.

## Referenz

### Attribute

| Attribut | Bedeutung |
|---|---|
| `channel` | Der öffentliche Schlüssel des Chat-Kanals (`yuva_pk_…`). Pflicht. |
| `server` | URL des Yuva-Servers. Standard ist die Origin, von der `yuva-docs.js` geladen wurde. |
| `locale` | `en` oder `tr`. Standard ist die Sprache des Browsers, sonst Englisch. |
| `dir` | `ltr` oder `rtl`. Standard ist die Schreibrichtung der Sprache. |
| `identity-token` | Ein signiertes Identitäts-Token. `setIdentityToken` ist meist besser. |
| `page` | Die URL der Seite. Standard ist die aktuelle URL; clientseitige Navigation wird verfolgt. |
| `page-title` | Der Titel der Seite. Standard ist `document.title`. |

Die React-Wrapper nehmen dasselbe als Props (`channel`, `server`, `locale`, `dir`, `page`,
`pageTitle`, `identityToken`), dazu `className` und `style`.

### Properties und Methoden

| Element | Member | Bedeutung |
|---|---|---|
| beide | `channel`, `server`, `locale`, `page`, `pageTitle` | Die Attribute als Properties. |
| beide | `setIdentityToken(fn)` | Eine Funktion, die ein Token zurückgibt, oder `null` für einen nicht angemeldeten Leser. |
| `yuva-page-feedback` | `rating` | `"up"`, `"down"` oder `null` für diese Seite in diesem Browser. |
| `yuva-page-feedback` | `rate("up" \| "down")` | Dasselbe wie ein Klick auf Ja oder Nein. |
| `yuva-page-questions` | `answers` | Die angezeigten veröffentlichten Antworten. |
| `yuva-page-questions` | `reload()` | Lädt die Antworten neu. |

### Ereignisse

Sie steigen aus dem Shadow DOM auf, Sie können also auf `document` lauschen.

| Ereignis | Element | `detail` | React-Prop |
|---|---|---|---|
| `yuva-rating` | `yuva-page-feedback` | `{ page, rating, previous }` | `onRating` |
| `yuva-feedback` | `yuva-page-feedback` | `{ page, rating, conversation_id }` | `onFeedback` |
| `yuva-question` | `yuva-page-questions` | `{ page, conversation_id }` | `onQuestion` |

### CSS-Variablen

Die Elemente übernehmen `color` und `font` von der Seite und folgen dem Dark Mode. Die Akzentfarbe
kommt von der Launcher-Farbe des Kanals; für eine eigene setzen Sie sie mit `!important`.

| Variable | Verwendet für |
|---|---|
| `--yuva-accent` | Buttons, die gewählte Bewertung, Fokusringe, Links in Antworten. |
| `--yuva-on-accent` | Text auf der Akzentfarbe. |
| `--yuva-border` | Rahmen von Buttons, Feldern und Antworten. |
| `--yuva-soft` | Hintergrund der gewählten Bewertung, Code in Antworten. |
| `--yuva-danger` | Fehlermeldungen. |

### Endpunkte

| Endpunkt | Verwendet für |
|---|---|
| `POST /client/v1/channels/{channel_key}/page-ratings` | Eine Bewertung zählen. |
| `POST /client/v1/feedback` | Feedback mit `page_url`, `page_title`, `rating` senden. |
| `POST /client/v1/questions` | Eine Frage stellen. |
| `GET /client/v1/channels/{channel_key}/page-answers?page=` | Die veröffentlichten Antworten einer Seite (eine Minute gecacht). |
| `POST /v1/conversations/{conversationId}/publish` | Eine Frage und ihre Antwort veröffentlichen. |
| `GET /v1/page-answers`, `GET`, `PATCH`, `DELETE /v1/page-answers/{pageAnswerId}` | Antworten auflisten, bearbeiten und zurückziehen. |
| `GET /v1/docs/pages`, `GET /v1/docs/summary`, `GET /v1/docs/page` | Die Ansicht Docs. |
| `GET /v1/conversations?page=&kind=` | Feedback oder Fragen einer Seite. |

Details: [API-Vertrag](../../openapi/openapi.yaml).
