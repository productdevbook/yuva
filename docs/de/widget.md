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
`import "useyuva/chat"` und das Attribut `server` setzen. Sendet Ihre Website eine Content Security
Policy, erlauben Sie Ihren Yuva-Server in `script-src` und `connect-src` (`https://` und `wss://`).

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
| `locale` | `en` oder `tr`. Standard ist die Sprache des Browsers, sonst Englisch. |
| `dir` | `ltr` oder `rtl`. Standard ist die Schreibrichtung der Sprache. |
| `identity-token` | Ein Token als String; `setIdentityToken` ist meist besser. |
| `open` | Vorhanden: Das Fenster ist offen. |

### Methoden und Ereignisse

| Member | Bedeutung |
|---|---|
| `open()`, `close()`, `toggle()`, `isOpen` | Das Fenster des Launchers. |
| `unread`, Ereignis `yuva-unread` | Noch nicht gesehene Antworten; das Ereignis hat `detail.count`. |
| `setIdentityToken(fn)`, `signOut()` | Angemeldete Nutzer, wie oben. |
| `rate(conversationId, "good" \| "bad", comment?)` | Bewertet eine geschlossene Unterhaltung; `409 already_rated` oder `rating_unavailable`, wenn das nicht geht. |

### Kanaleinstellungen

| Einstellung (API-Feld) | Bedeutung |
|---|---|
| Allowed origins (`allowed_origins`) | 1 bis 20 Origins, exakt verglichen. |
| Anonymous visitors (`allow_anonymous`) | Besucher ohne Identitäts-Token dürfen chatten. |
| Ask for e-mail (`ask_email_offline`) | Fragt nach einer Adresse, wenn niemand erreichbar ist. Ungelesene Antworten werden nach `YUVA_CHAT_EMAIL_DELAY` (Standard 5 Minuten) per E-Mail geschickt. Braucht einen E-Mail-Kanal im Posteingang. |
| Greeting (`greeting`) | Wird vor der ersten Nachricht angezeigt. |
| Launcher (`launcher`) | `position` und `color` (`#rrggbb`). |

Über die API: `POST /v1/inboxes/{inboxId}/channels` mit `kind: "chat"` und einem `chat`-Objekt, als
Inhaber oder Admin.
