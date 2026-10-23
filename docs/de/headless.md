# Headless

Alles, was Panel und Widget tun, steht auch Ihrem eigenen Code offen: Ihr eigener Posteingang über
`/v1`, Ihr eigener Chat über die Kontakt-API oder ein Bot, der mit Entwürfen antwortet.
Lauffähiger Code liegt unter MIT-Lizenz in [`examples/`](../../examples).

## API-Keys

Ihr Code spricht `/v1` mit einem API-Schlüssel an. Inhaber und Admins legen Schlüssel unter
Settings → API keys an, oder auf dem Server:

```sh
yuva api-key create --workspace <id|name> --name "Support bot" --scope conversations:read --scope messages:write
```

Geben Sie einem Schlüssel nur die [Scopes](#scopes), die er braucht (ohne `--scope` bekommt er
alle); `--inbox <id>` beschränkt ihn auf bestimmte Posteingänge. Jeder Schlüssel ist ein Bot,
dessen Name an allem steht, was er schreibt. Bewahren Sie Schlüssel nur auf Ihren Servern auf.

## Ihre eigene Posteingangs-Oberfläche

Lesen und schreiben Sie Unterhaltungen mit dem typisierten Client (`npm install useyuva`):

```ts
import { createYuvaApi } from "useyuva/api";

const api = createYuvaApi({ server: "https://support.example.com", apiKey: process.env.YUVA_API_KEY! });
const { data } = await api.GET("/v1/conversations", { params: { query: { status: ["open"] } } });
```

In Go: `github.com/productdevbook/yuva/sdk/go/client` ([README](../../sdk/go/README.md)). Andere
Sprachen: der [OpenAPI-Vertrag](../../openapi/openapi.yaml). Eine Oberfläche für Ihr Team kann über
[OAuth](mcp.md#oauth) als das jeweilige Mitglied handeln.

Um Änderungen zu verfolgen, haben Sie zwei Wege:

- **Den Event-Feed abfragen.** Beginnen Sie bei `GET /v1/events/latest`, rufen Sie dann
  `GET /v1/events?after=<position>&limit=100` auf, verarbeiten Sie `events`, speichern Sie `next`
  und wiederholen Sie sofort, solange `has_more` true ist. Siehe
  [`examples/inbox-feed`](../../examples/inbox-feed).
- **Einen WebSocket offen halten** auf `GET /v1/realtime` mit `Authorization: Bearer <key>`.
  Verbinden Sie sich mit `?last_event_id=<last id handled>` neu, um Verpasstes zu bekommen.

## Ihre eigene Chat-Oberfläche

Kontakte nutzen die Kontakt-API (`/client/v1`) mit ihrer eigenen Sitzung, nie mit einem
API-Schlüssel. `createYuvaClient()` kapselt sie ohne DOM, für Browser, Node 22+, Bun, Deno und
Worker:

```ts
import { createYuvaClient } from "useyuva";

const yuva = createYuvaClient({
  server: "https://support.example.com",
  channel: "yuva_pk_…",
  identityToken: () => fetch("/my/yuva-token").then((r) => r.text()),
});

yuva.on("event", (event) => console.log(event.type));
await yuva.connect();
const { conversation } = await yuva.startConversation({ body: "Hello" });
```

Eine Unterhaltung, die ein Mitglied gelöscht hat, verschwindet aus `/client/v1`, und der Realtime-Stream sendet
einen `conversation.removed`-Frame (`data: { id }`): Entfernen Sie sie aus Ihrer Liste und schließen Sie sie,
wenn sie offen ist. Eine Nachricht an diese ID startet eine neue Unterhaltung; die `conversation_id` der
Antwort nennt sie. Wird die Unterhaltung wiederhergestellt, kommt sie erneut als `conversation.created`. `createYuvaClient()`
meldet ihn als `conversationRemoved`-Event (`{ id }`), die React-Hooks entfernen die Unterhaltung, und die Swift-
und Kotlin-SDKs senden `YuvaEvent.conversationRemoved(id:)` / `YuvaEvent.ConversationRemoved(id)`.

Außerhalb eines Browsers verwenden Sie den Schlüssel eines `app`-Kanals; `chat`-Kanäle akzeptieren
nur ihre erlaubten Origins. React-Hooks liegen in `useyuva/react`; alle Optionen stehen in der
[SDK-README](../../sdk/js/README.md).

## Ein Bot, der Antworten entwirft

1. Legen Sie einen [Webhook](webhooks.md) für `message.created` an, der auf Ihren Bot zeigt.
2. [Verifizieren Sie die Signatur](webhooks.md#die-signatur-verifizieren) und reagieren Sie nur auf
   `data.message` mit `direction: "in"` und `kind: "message"`.
3. Senden Sie die Antwort als Entwurf. Ein `Idempotency-Key` aus der eingehenden Nachricht
   verhindert, dass ein wiederholter Webhook einen zweiten Entwurf anlegt:

```sh
curl -X POST https://support.example.com/v1/conversations/$CONVERSATION/messages \
  -H "Authorization: Bearer $YUVA_API_KEY" -H "Content-Type: application/json" \
  -H "Idempotency-Key: draft-bot:$INCOMING_MESSAGE_ID" \
  -d '{"kind": "message", "direction": "out", "draft": true, "body": "Thanks, we are on it."}'
```

Ein Mitglied sieht den Entwurf mit dem Namen des Bots und sendet, bearbeitet oder verwirft ihn.
Siehe [`examples/draft-bot`](../../examples/draft-bot).

### Bots may send

Die Workspace-Einstellung **Bots may send** (Settings → API keys) ist standardmäßig aus: Eine
ausgehende Nachricht eines Schlüssels muss dann ein Entwurf sein (sonst `403 bot_sending_disabled`).
Ist sie an, antwortet ein Schlüssel mit `messages:write` direkt und sendet mit `drafts:send` auch
Entwürfe. Ändern kann sie nur ein im Panel angemeldeter Inhaber oder Admin (`bots_may_send` bei
`PATCH /v1/workspace`), nie ein Schlüssel.

## Referenz

### Scopes

Ein Aufruf ohne den nötigen Scope antwortet mit `403 insufficient_scope`.

| Scope | Erlaubt |
|---|---|
| `conversations:read` | Unterhaltungen, Nachrichten, Anhänge, Labels, Textbausteine, den Event-Feed und Realtime |
| `conversations:write` | Status, Zuständige, Zurückstellen, Priorität, Labels; Unterhaltungen verschieben und gesammelt ändern |
| `conversations:delete` | Unterhaltungen in den Papierkorb legen, wiederherstellen, den Papierkorb auflisten. In keinem anderen Scope enthalten: Nennen Sie ihn ausdrücklich. Nur für Schlüssel und verbundene Apps von Inhabern und Admins verfügbar. |
| `messages:write` | Nachrichten und Entwürfe: anlegen, bearbeiten, verwerfen |
| `drafts:send` | Einen Entwurf senden, zusammen mit `messages:write` |
| `notes:write` | Notizen |
| `contacts:read` | Kontakte, Suchen, Anwesenheit |
| `contacts:write` | Kontakte anlegen, ändern, löschen und zusammenführen |
| `inboxes:read` | Workspace, Mitglieder, Posteingänge, Kanäle |
| `inboxes:manage` | Posteingänge und Kanäle anlegen, ändern und löschen, Zugriff auf Posteingänge, Secrets |
| `labels:write` | Labels |
| `canned_replies:write` | Textbausteine |
| `webhooks:manage` | Webhooks, ihre Zustellungen und Versuche |
| `workspace:manage` | Workspace-Einstellungen und Nutzung |
| `migrations:manage` | [Ein Postfach zu Yuva umziehen](migrate.md): erkennen, anlegen, verbinden, scannen, starten, abschließen, importierten Verlauf entfernen. Nie für Agents oder auf Posteingänge beschränkte Keys. |
| `analytics:read` | [Analytics](analytics.md#api): Properties, Zahlen, Realtime, Health |
| `analytics:write` | [Serverseitige Events](analytics-server.md) senden (`POST /v1/analytics/properties/{propertyId}/events`). Nur API-Keys, OAuth-Clients nie angeboten. |
| `analytics:manage` | Properties anlegen, ändern und löschen, ihre Schlüssel erneuern, ihre Daten löschen. Nie für Agents oder auf Posteingänge beschränkte Keys. |
| `status:read` | [Statusseiten](status-pages.md): Seiten, Komponenten, Vorfälle, Benachrichtigungen, Monitore und ihre Prüfungen |
| `status:manage` | Statusseiten ändern, Zustände setzen, Vorfall-Updates senden, Monitore verwalten, Konversationen mit Vorfällen verknüpfen und ihnen antworten. Enthält `status:read` nicht. |
| `help:read` | [Hilfecenter](help-center.md): Center, Sammlungen, Artikel mit Entwürfen und Revisionen, Suche, Bericht |
| `help:write` | Sammlungen und Artikel anlegen und ändern, Entwürfe speichern, Bilder hochladen |
| `help:publish` | Hilfe-Artikel veröffentlichen, zurückziehen und veröffentlichte löschen. In keiner anderen Berechtigung enthalten: Nennen Sie sie ausdrücklich. |
| `roadmap:read` | [Roadmap-Boards](roadmap-board.md): Boards, Phasenverlauf, das Ergebnis einer Antwort an verknüpftes Feedback |
| `roadmap:write` | Boards anlegen, ändern und löschen, Feedback mit Ideen verknüpfen, an verknüpftes Feedback antworten. Ideen zu lesen und zu verknüpfen braucht zusätzlich `conversations:read`. |
| `roadmap:stage` | Die Phase einer Idee verschieben, was den Fortschritt ihres verknüpften Feedbacks setzt. In keinem anderen Scope enthalten: Nennen Sie ihn ausdrücklich. |
| `changelog:read` | [Neuigkeiten](whats-new.md): Changelogs, Einträge mit Entwürfen, Vorschau |
| `changelog:write` | Einträge und Übersetzungen schreiben, Bilder hochladen |
| `changelog:publish` | Changelogs anlegen, ändern und löschen; veröffentlichte Einträge veröffentlichen, planen, zurückziehen und löschen. In keinem anderen Scope enthalten: nennen Sie ihn ausdrücklich. |
| `audit:read` | `GET /v1/audit-log`, das [Audit-Log](operations.md#audit-log). In keinem anderen Scope enthalten: nennen Sie ihn. Nur ein Inhaber kann ihn einem Schlüssel geben, und ein auf Postfächer begrenzter Schlüssel kann ihn nicht halten. Er gilt nur, solange das Mitglied, das den Schlüssel erstellt hat, noch Inhaber ist. `yuva api-key create` lässt ihn weg, wenn Sie ihn nicht nennen. |
| `feedback:write` | `POST /v1/feedback` |

### Schlüssel

| Einstellung | Detail |
|---|---|
| Scopes, Posteingänge | Beim Anlegen festgelegt. Ein auf Posteingänge beschränkter Schlüssel kann `inboxes:manage`, `webhooks:manage` und `workspace:manage` nicht haben. |
| Ablauf | Optional, fest. Danach `401 api_key_expired`. |
| Name, Bot-Name, Avatar | Stehen an allem, was der Schlüssel schreibt. Änderbar mit `PATCH /v1/api-keys/{id}`. |

### Entwürfe

| Aufruf | Tut |
|---|---|
| `PATCH /v1/messages/{id}` | Bearbeitet einen Entwurf. |
| `DELETE /v1/messages/{id}` | Verwirft ihn. |
| `POST /v1/messages/{id}/send` | Stellt ihn zu; `sent_by` nennt, wer ihn gesendet hat. |

Bei einer Nachricht, die kein Entwurf ist: `409 not_a_draft`. Entwürfe lösen `draft.created`,
`draft.updated` und `draft.deleted` aus.

### Event-Feed und Realtime

| Regel | Detail |
|---|---|
| Aufbewahrung | 7 Tage. Eine ältere Position antwortet mit `410 cursor_expired`; Realtime sendet `resync_required`. Neu laden und bei `GET /v1/events/latest` neu beginnen. |
| `after=0` | Beginnt beim ältesten aufbewahrten Ereignis. |
| Seiten | Können weniger als `limit` enthalten, solange `has_more` true ist. |
| Scopes | `conversations:read`; Kontaktereignisse zusätzlich `contacts:read`. |
| Tippen | `typing`-Frames haben keine `id` und werden nicht erneut abgespielt. |

### Unterhaltungen löschen

`DELETE /v1/conversations/{conversationId}` legt eine Unterhaltung in den Papierkorb. Das dürfen nur Inhaber
und Admins sowie Schlüssel und verbundene Apps mit `conversations:delete`; Agents erhalten `403 forbidden`.
Ein auf Postfächer beschränkter Schlüssel darf nur in diesen Postfächern löschen. `yuva api-key create`
lässt den Scope weg, solange Sie ihn nicht nennen.

| Regel | Detail |
|---|---|
| Papierkorb | Die Unterhaltung verschwindet für das Team, den Kontakt, Apps und Assistenten aus allen Listen, Zählungen, Suchen und Kennzahlen; niemand kann sie ändern. Zweimal ausgeführt ändert nichts. |
| Papierkorb ansehen | `GET /v1/conversations?trashed=true`, für Inhaber, Admins und `conversations:delete`; die Unterhaltung hat dann `trashed_at` und `deletes_at`. |
| Wiederherstellen | `POST /v1/conversations/{conversationId}/restore` innerhalb von 30 Tagen, mit Status, Zuständigem und Labels. Danach antwortet es mit `409 trash_expired`. |
| Nach 30 Tagen | Die stündliche Bereinigung löscht sie endgültig, mit Nachrichten, Anhängen und gespeicherten Dateien. |
| Mehrere auf einmal | `POST /v1/conversations/bulk` mit `trashed: true` oder `false` und sonst nichts, bis zu 100. |
| Der Kontakt schreibt wieder | Per E-Mail, Chat oder App beginnt eine neue Unterhaltung; die im Papierkorb wird nie wieder geöffnet. |

Im Panel finden Inhaber und Admins Löschen im Menü einer Unterhaltung und für eine Auswahl, als Kürzel `#`, und
einen Papierkorb-Tab mit Wiederherstellen.

Webhooks senden `conversation.trashed` und `conversation.restored` (Unterhaltung und Kontakt) und nach 30 Tagen
`conversation.deleted` mit nur `id`, `inbox_id` und `contact_id`. Das [Audit-Log](operations.md#audit-log)
hält fest, wer jede Unterhaltung in den Papierkorb gelegt oder wiederhergestellt hat, und die endgültige
Löschung als vom System ausgeführt.

### Idempotenz

Jeder authentifizierte `POST` auf `/v1` und `/client/v1` akzeptiert `Idempotency-Key` (1 bis 255
druckbare ASCII-Zeichen), 24 Stunden pro Aufrufer gespeichert. `5xx`-Antworten werden nicht
gespeichert. Anmeldung, die Anfrage für die Kontaktsitzung und `/v1/me/…` ignorieren ihn.

| Wiederholung | Antwort |
|---|---|
| Gleicher Key, gleiche Anfrage | Die erste Antwort, mit `Idempotent-Replayed: true` |
| Gleicher Key, andere Anfrage | `409 idempotency_key_reused` |
| Gleicher Key, erste Anfrage läuft noch | `409 idempotency_key_in_use` |

### Server nur mit API

| Variable | Wirkung |
|---|---|
| `YUVA_PANEL=off` | Kein Panel und keine OAuth-Zustimmungsseite; API-Schlüssel funktionieren weiter. |
| `YUVA_WIDGET=off` | Kein `/yuva.js` oder `/yuva-chat.js`; die Kontakt-API bleibt. |

Siehe [Konfiguration](configuration.md#server).
