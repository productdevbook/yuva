# Web-Widget

`<yuva-chat>` bringt einen Live-Chat auf Ihre Website oder einen Unterhaltungsverlauf in Ihr eigenes
Produkt. Es funktioniert auf jeder Seite und mit jedem Framework.

## 1. Einen Chat-Kanal anlegen

Im Panel: Settings → den Posteingang → Channels → Chat. Tragen Sie jede Origin ein, die das Widget
zeigen darf, etwa `https://www.example.com`, und kopieren Sie dann den öffentlichen Schlüssel des
Kanals (`yuva_pk_…`). Der Schlüssel ist kein Geheimnis.

## 2. Einbinden

```html
<script src="https://support.example.com/yuva.js" defer></script>
<yuva-chat channel="yuva_pk_xxxxxxxxxxxxxxxx"></yuva-chat>
```

Das zeigt einen schwebenden Launcher. Mit einem Bundler: `npm install useyuva`,
`import "useyuva/chat"` und das Attribut `server` setzen. In Nuxt: `modules: ["useyuva/nuxt"]`,
`yuva: { server, channel }` in der `nuxt.config` und `<YuvaChat />`. Sendet Ihre Website eine
Content Security Policy, erlauben Sie Ihren Yuva-Server in `script-src` und `connect-src`
(`https://` und `wss://`).

## 3. Angemeldete Nutzer

Ihr Backend signiert ein [Identitäts-Token](identity.md) für den aktuellen Nutzer; das Widget fragt
es ab:

```js
document.querySelector("yuva-chat").setIdentityToken(async () => {
  const response = await fetch("/api/yuva-identity-token");
  return response.ok ? (await response.json()).token : null;
});
```

Geben Sie für anonyme Besucher `null` zurück. Rufen Sie die Methode nach der Anmeldung erneut auf und
nach der Abmeldung `signOut()`.

## Was Besucher sehen

Das Panel öffnet mit einem Startbildschirm: wer erreichbar ist, die letzte Unterhaltung zum
Weitermachen und eine Schaltfläche für eine neue Nachricht. Eine Unterhaltung zeigt Tagestrenner,
Zeilen wie "ist beigetreten" und "geschlossen", Anhangskarten (wird hochgeladen, fehlgeschlagen,
gesendet), eine Tippanzeige, "Gelesen" unter der letzten vom Team gelesenen Nachricht und eine
Schaltfläche "Zur neuesten Nachricht" mit der Zahl ungelesener Nachrichten, wenn ein Besucher nach
oben gescrollt hat. Das Eingabefeld nimmt Dateien, eingefügte Bilder und, wo der Browser es erlaubt,
einen Screenshot der Seite entgegen.

Der Startbildschirm öffnet außerdem Apps im selben Panel, jede nur, wenn der Kanal hat, was sie
braucht, und lädt sie erst bei der ersten Nutzung. **Feedback** sendet eine Kategorie, Text und
Dateien als Feedback-Unterhaltung, die der Besucher in seinen Nachrichten findet. **Status** zeigt
die veröffentlichte Statusseite der Inbox: den Gesamtzustand, die Komponenten mit ihren letzten
60 Tagen sowie offene und aktuelle Störungen. **Updates erhalten** meldet den Besucher bei den
öffentlichen Mailinglisten der Inbox an und sendet den Einwilligungstext, wie er angezeigt wurde. **Hilfecenter** erscheint, wenn die Inbox ein
veröffentlichtes [Hilfecenter](help-center.md) hat, zusammen mit einem Suchfeld auf der Startseite:
Es zeigt die Artikel, die zur Seite des Besuchers passen, die beliebten und die Sammlungen, öffnet
Artikel im Panel mit „War das hilfreich?“ und endet mit einer Schaltfläche „Schreiben Sie uns“. Eine
Unterhaltung, die nach dem Lesen eines Artikels beginnt, hält ihn fest, und ein Artikel, den Ihr Team
in einer Antwort sendet, öffnet sich in derselben App. **Roadmap** erscheint, wenn der Posteingang ein öffentliches [Roadmap-Board](roadmap-board.md) hat: Ideen nach Phase und Stimmen, eine Idee mit ihren Antworten und Idee vorschlagen. Abstimmen und Vorschlagen erfordern eine Anmeldung, die das Leserfenster öffnet, sofern der Besucher kein Identitäts-Token hat. **Neuigkeiten** erscheint, wenn der Posteingang einen veröffentlichten [Neuigkeiten](whats-new.md)-Changelog mit einem Eintrag hat: eine Kachel mit der Zahl ungelesener Einträge, dann die Einträge, neueste zuerst, mit Labels, Datum, Bild und Text.

Ist niemand erreichbar und fragt der Kanal nach einer E-Mail-Adresse, bittet die Unterhaltung darum.
Nach dem Speichern sagt sie, dass ein Bestätigungslink verschickt wurde (mit Adresse ändern und
Erneut senden), fragt erneut, wenn die Adresse unzustellbar war oder der Link nach 24 Stunden
ablief, und zeigt nach der Bestätigung eine unauffällige Zeile. Das Panel folgt
`prefers-color-scheme`, nutzt die Launcher-Farbe des Kanals als Akzent und die Geist-Schrift der
Seite, falls vorhanden, sonst die Systemschrift.

## Was das Widget speichert

Das Widget hält seine Sitzung im `localStorage` Ihrer Seite, damit die Unterhaltung eines Besuchers
ein Neuladen übersteht. Alles gehört zum Origin Ihrer Seite.

| Schlüssel | Inhalt |
|---|---|
| `yuva:<channel>` | Das Sitzungs-Token mit Ablauf, die anonyme Besucher-ID und der Zustand des Launchers. |
| `yuva:reader:<server> <channel>` | Das Leser-Token, nachdem sich ein Leser auf einer Dokumentationsseite angemeldet hat. |

Die Token sind zufällige Zeichenfolgen, die Yuva nur als Hash speichert und jederzeit beenden kann. Da
der Speicher zu Ihrem Origin gehört, kann ein in Ihre Seite eingeschleustes Skript (XSS) sie lesen und
als dieser Besucher handeln, bis das Token abläuft oder beendet wird. Was Sie tun können:

- **Abmelden.** `signOut()` beendet die Sitzung auf dem Server und entfernt sie aus dem Speicher;
  Besucher-ID und Launcher-Zustand bleiben. `readerSignOut()` beendet das Leser-Token und leert dessen
  Schlüssel. Rufen Sie beide auf, wenn sich Ihr Nutzer aus Ihrer App abmeldet.
- **Identitäts-Token nutzen** für angemeldete Nutzer. Ein von Ihrem Backend signiertes
  [kurzlebiges Token](identity.md) belegt, wer der Nutzer ist; die gespeicherte Sitzung ist dann nicht
  das Einzige, was für ihn bürgt.
- **Eine Content Security Policy senden**, die `script-src` auf vertrauenswürdige Quellen beschränkt;
  sie ist der wichtigste Schutz gegen eingeschleuste Skripte.
- **Das Token aus `localStorage` heraushalten**, wenn Sie eine eigene Oberfläche bauen:
  `createYuvaClient()` nimmt eine Option `storage` an, ein beliebiges Objekt mit `getItem` und
  `setItem`, etwa eines, das `sessionStorage` umschließt oder die Werte im Speicher hält. Das Element
  `<yuva-chat>` hat diese Option nicht und nutzt immer `localStorage`; ist `localStorage` nicht
  verfügbar, wird der Arbeitsspeicher genutzt.

## Fehlersuche

- **Nichts erscheint**: Die Origin der Seite ist nicht erlaubt (die Konsole nennt sie), oder der
  Schlüssel ist falsch.
- **„Sign in to chat with us.“**: Der Kanal ist für anonyme Besucher geschlossen, und es wurde kein
  Token übergeben.

## Referenz

### Attribute

| Attribut | Bedeutung |
|---|---|
| `channel` | Der öffentliche Schlüssel des Chat-Kanals. Pflicht. |
| `server` | URL des Yuva-Servers. Standard ist die Herkunft von `yuva.js`. |
| `layout` | `launcher` (Standard) oder `embedded` für einen Verlauf in Ihrer Seite; geben Sie ihm eine Höhe. |
| `locale` | Eine der [47 Sprachen](#sprachen), etwa `en`, `tr`, `de`, `pt-BR` oder `ar`. Standard ist die Sprache des Browsers; eine Sprache, die Yuva nicht hat, fällt auf Englisch zurück. |
| `dir` | `ltr` oder `rtl`. Standard ist die Schreibrichtung der Sprache. |
| `identity-token` | Ein Token als String; `setIdentityToken` ist meist besser. |
| `open` | Vorhanden: Das Fenster ist offen. |

### Sprachen

Das Widget spricht 47 Sprachen: `en`, `tr`, `de`, `fr`, `fr-CA`, `es`, `es-MX`, `pt-BR`, `pt-PT`, `it`, `nl`, `sv`, `da`, `nb`, `fi`, `pl`, `cs`, `sk`, `sl`, `hr`, `hu`, `ro`, `ca`, `ja`, `ko`, `zh-Hans`, `zh-Hant`, `ru`, `uk`, `el`, `id`, `ms`, `ar`, `he`, `ur`, `hi`, `bn`, `mr`, `pa`, `gu`, `or`, `kn`, `ml`, `ta`, `te`, `th`, `vi`. Arabisch, Hebräisch und Urdu werden von rechts nach links gesetzt. Ein Tag wie `pt-BR`, `zh-TW` oder `es-AR` wählt die nächstliegende davon; `no` und `nn` nutzen `nb`. Eine Sprache, die nicht in der Liste steht, fällt auf Englisch zurück.

### Methoden und Ereignisse

| Member | Bedeutung |
|---|---|
| `open()`, `close()`, `toggle()`, `isOpen` | Das Fenster des Launchers. |
| `unread`, Ereignis `yuva-unread` | Noch nicht gesehene Antworten; das Ereignis hat `detail.count`. |
| `setIdentityToken(fn)`, `signOut()` | Angemeldete Nutzer, wie oben. |
| `rate(conversationId, "good" \| "bad", comment?)` | Bewertet eine geschlossene Unterhaltung; `409 already_rated` oder `rating_unavailable`, wenn das nicht geht. |
| `openApp(kind, params?)` | Öffnet das Panel in einer App: `"help"` (`{ article }` oder `{ query }`), `"feedback"`, `"status"`, `"subscribe"` `"roadmap"` (`{ idea }` öffnet diese Idee) oder `"whats-new"` (`{ entry }` öffnet diesen Eintrag). |

### Kanaleinstellungen

| Einstellung (API-Feld) | Bedeutung |
|---|---|
| Allowed origins (`allowed_origins`) | 1 bis 20 Origins, exakt verglichen. |
| Anonymous visitors (`allow_anonymous`) | Besucher ohne Identitäts-Token dürfen chatten. |
| Ask for e-mail (`ask_email_offline`) | Fragt nach einer Adresse, wenn niemand erreichbar ist. Ungelesene Antworten werden nach `YUVA_CHAT_EMAIL_DELAY` (Standard 5 Minuten) per E-Mail geschickt. Braucht einen E-Mail-Kanal im Posteingang. |
| Greeting (`greeting`) | Wird vor der ersten Nachricht angezeigt. |
| Launcher (`launcher`) | `position` und `color` (`#rrggbb`). |
| Updates ankündigen (`announce_updates`) | Standardmäßig aus. Der Launcher zeigt die Zahl ungelesener [Neuigkeiten](whats-new.md#4-im-widget)-Einträge, solange keine Nachrichten ungelesen sind. |

Über die API: `POST /v1/inboxes/{inboxId}/channels` mit `kind: "chat"` und einem `chat`-Objekt, als
Inhaber oder Admin.

Ein Chat-Kanal kann mit `attachments` auch die [Datei-Grenzen](configuration.md#dateien-und-speicher) des Servers
einengen. Das Widget liest sie noch nicht: Es sendet die Datei und zeigt die Ablehnung des Servers
(`413 attachment_too_large` oder `415 attachment_type_not_allowed`).
