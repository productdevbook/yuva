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

Wenn ein Mitglied [eine Nachricht entfernt](#eine-nachricht-entfernen), sendet der Stream einen `message.redacted`-Frame;
`data` ist die Nachricht mit `redacted: true`, leerem `body` und ohne Anhänge: ersetzen Sie sie an Ort und Stelle
und zeigen Sie "Diese Nachricht wurde entfernt". Dieselbe Nachricht trägt auch in einem späteren Replay oder einer
Liste `redacted: true`. `createYuvaClient()` reicht den Frame im Event `event` weiter, die React-Hooks aktualisieren
die Nachricht, und die Swift- und Kotlin-SDKs senden `YuvaEvent.messageRedacted(_:)` / `YuvaEvent.MessageRedacted(message)`.

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

## Teams

Ein Team ist eine benannte Gruppe von Mitgliedern, der Posteingänge zugewiesen werden. Ein Mitglied
erreicht einen Posteingang über die eigene Freigabe, über jedes Team, dem es angehört, oder über beides:
Der Zugriff ist die Vereinigung. Eine Freigabe zu entfernen ändert also nichts, solange eine andere den
Posteingang weiter gibt. Eine Unterhaltung kann einem Team ebenso zugewiesen werden wie einem Mitglied.

```sh
curl -X POST https://support.example.com/v1/teams \
  -H "Authorization: Bearer $YUVA_API_KEY" -H "Content-Type: application/json" \
  -d '{"name": "Billing", "description": "Invoices and refunds"}'
curl -X PUT https://support.example.com/v1/teams/$TEAM/inboxes/$INBOX -H "Authorization: Bearer $YUVA_API_KEY"
curl -X PUT https://support.example.com/v1/teams/$TEAM/members/$MEMBER -H "Authorization: Bearer $YUVA_API_KEY"
```

| Aufruf | Bewirkt |
|---|---|
| `GET /v1/teams` | Listet die Teams nach Namen: `{"items": [...]}`. |
| `POST /v1/teams` | Legt eines an (`name` 1 bis 64 Zeichen und eindeutig: `409 team_taken`; optional `description`, bis 500). `201`. |
| `GET`, `PATCH`, `DELETE /v1/teams/{teamId}` | Liest ein Team, benennt es um oder beschreibt es neu (`PATCH` braucht mindestens ein Feld) oder löscht es. |
| `PUT`, `DELETE /v1/teams/{teamId}/members/{memberId}` | Fügt ein Mitglied hinzu oder entfernt es. `204`; Wiederholen ändert nichts. |
| `PUT`, `DELETE /v1/teams/{teamId}/inboxes/{inboxId}` | Gibt einen Posteingang frei oder entzieht ihn. `204`; Wiederholen ändert nichts. |
| `GET /v1/inboxes/{inboxId}/members` | Wer Zugriff auf einen Posteingang hat. Jedes Mitglied hat `via` (`direct`, oder `team`, wenn nur Teams den Zugriff geben) und `team_ids`, die Teams, die den Posteingang geben. |

Ein Team hat `id`, `name`, `description`, `member_ids`, `inbox_ids` (beide in der Reihenfolge des
Hinzufügens), `created_at` und `updated_at`. Höchstens 500 Mitglieder und 100 Posteingänge pro Team
(`403 team_limit`); ein unbekanntes Team, Mitglied oder ein unbekannter Posteingang ergibt `404`.

Lesen braucht `inboxes:read`; Schreiben braucht `inboxes:manage` und einen Inhaber, Admin oder einen
Schlüssel ohne Posteingangs-Beschränkung (Agents erhalten `403 forbidden`). Inhaber, Admins und
unbeschränkte Schlüssel sehen jedes Team; ein Agent sieht die Teams, in denen er Mitglied ist, und die
Teams mit Zugriff auf einen Posteingang, den er sieht; ein auf Posteingänge beschränkter Schlüssel sieht
die Teams mit Zugriff auf einen dieser Posteingänge. Ein Team, das Sie nicht sehen, ist `404`, und die
`inbox_ids`, die Sie erhalten, nennen nur die Posteingänge, die Sie sehen.

Die direkte Freigabe eines Mitglieds zu entziehen (`DELETE /v1/inboxes/{inboxId}/members/{memberId}`)
nimmt nicht, was ein Team ihm gibt; nehmen Sie stattdessen das Mitglied aus dem Team oder entziehen Sie
dem Team den Posteingang.

### Einem Team zuweisen

`PATCH /v1/conversations/{conversationId}` nimmt neben `assignee_id` auch `team_id` (eine Team-ID, oder
`null`, um es zu entfernen). Das Team muss den Posteingang der Unterhaltung haben, sonst antwortet der
Aufruf mit `422 team_access`; ebenso bei einem Team, das während des Aufrufs gelöscht oder dem der Zugriff
entzogen wird. Beim Verschieben mit `POST /v1/conversations/{conversationId}/move` behält die Unterhaltung
ihr Team nur, wenn das Team den Ziel-Posteingang hat. Ein Team zu löschen oder ihm einen Posteingang zu
entziehen, leert `team_id` der betroffenen Unterhaltungen, jeweils mit einem `conversation.updated`. Die
Unterhaltung trägt `team_id`, wenn sie eines hat; das ist ein eigenes Feld, und `assignee=unassigned`
bedeutet weiterhin „kein Mitglied“, egal welches Team. Die Sammelaktualisierung und
`POST /v1/conversations` nehmen kein Team.

| Listenfilter | Zeigt |
|---|---|
| `GET /v1/conversations?team_id=<id>` | Unterhaltungen, die diesem Team zugewiesen sind. |
| `GET /v1/conversations?team=mine` | Unterhaltungen, die einem Team des aufrufenden Mitglieds zugewiesen sind. Nur Mitglieds-Sitzungen; ein Schlüssel erhält `400`. |

`GET /v1/conversations/counts` hat `teams` (offene Unterhaltungen ohne Spam, pro Team-ID) und
`mine_teams` (offene Unterhaltungen, die einem Team des Aufrufers zugewiesen sind; `0` für Schlüssel).

### Ereignisse

- `team.created`, `team.updated` und `team.deleted` gehen an den [Event-Feed und Realtime](#event-feed-und-realtime)
  und an [Webhooks](webhooks.md#ereignisse). `team.updated` folgt auf jede Umbenennung, neue Beschreibung
  und jede Änderung bei Mitgliedern oder Posteingängen; aus dem Team entfernte Mitglieder stehen in
  `removed_member_ids`. Sie brauchen `inboxes:read` und erreichen Aufrufer, die jeden Posteingang sehen,
  die Mitglieder des Teams und Aufrufer, die einen seiner Posteingänge sehen.
- `inbox_access.changed` wird einmal pro Mitglied und Posteingang gesendet, dessen Zugriff sich geändert
  hat, ob durch eine direkte Freigabe, ein Team oder dessen Entfernung. Eine Änderung, die den Zugriff
  lässt, wie er war, sendet keines.
- Die Zuweisung an ein Team zeigt sich in `conversation.updated`, dessen Payload `team_id` trägt.
- Das [Audit-Log](operations.md#audit-log) erfasst `team.created`, `team.deleted`, `team.member_added`,
  `team.member_removed`, `team.inbox_access_granted` und `team.inbox_access_revoked`.

[MCP](mcp.md)-Assistenten nutzen `list_teams`, `assign` mit `team_id` oder `remove_team` und
`search_conversations` mit `team_id`.

## Makros

Ein Makro ist eine gespeicherte Antwort plus Aktionen: Labels zum Hinzufügen oder Entfernen, ein
Zuständiger, ein Status, eine Zurückstellung oder eine Priorität. Wer eine Nachricht mit einem Makro
sendet, schreibt die Nachricht und wendet die Aktionen in einem Schritt an. Ein Makro ist `personal`
(nur Ihres) oder `shared` (für den ganzen Workspace).

```sh
curl -X POST https://support.example.com/v1/macros \
  -H "Authorization: Bearer $YUVA_API_KEY" -H "Content-Type: application/json" \
  -d '{"visibility": "shared", "name": "Refund done", "shortcut": "refund-done",
       "body": "Your refund is on its way.",
       "actions": {"add_labels": ["'$LABEL'"], "status": "closed"}}'

curl -X POST https://support.example.com/v1/conversations/$CONVERSATION/messages \
  -H "Authorization: Bearer $YUVA_API_KEY" -H "Content-Type: application/json" \
  -d '{"kind": "message", "direction": "out", "body": "Your refund is on its way.", "macro_id": "'$MACRO'"}'
```

| Aufruf | Bewirkt |
|---|---|
| `GET /v1/macros` | Zuerst Ihre persönlichen Makros, dann die geteilten, jeweils nach `position` und Namen: `{"items": [...]}`. Ein Schlüssel sieht nur die geteilten. |
| `POST /v1/macros` | Legt eines an: `visibility`, `name`, `shortcut` und einen `body`, `actions` oder beides; optional `position` (0 bis 10000). `201`. |
| `GET`, `PATCH`, `DELETE /v1/macros/{macroId}` | Liest, ändert oder löscht ein Makro. `PATCH` nimmt `name`, `shortcut`, `body` (`null` entfernt den Text), `actions` (ersetzt die ganze Menge) und `position`; die Sichtbarkeit lässt sich nicht ändern. |

Ein Makro hat `id`, `visibility`, `owner_member_id` (nur persönlich), `name`, `shortcut`, `body`, `actions`,
`position`, `created_at` und `updated_at`. `actions` kann enthalten:

| Aktion | Wirkung |
|---|---|
| `add_labels`, `remove_labels` | Label-IDs, je bis zu 50; ein Label darf nicht in beiden stehen. Ein gelöschtes Label fällt aus dem Makro. |
| `assignee_id` | Ein zuzuweisendes Mitglied; `null` hebt die Zuweisung auf. Ein entferntes Mitglied fällt aus dem Makro. |
| `status` | `open`, `pending`, `snoozed` oder `closed`. `snoozed` braucht `snooze_minutes`. |
| `snooze_minutes` | 1 bis 525600; stellt ab dem Moment des Sendens zurück und bedeutet `status: snoozed`. |
| `priority` | Die Priorität der Unterhaltung. |

Ein Kürzel ist unter den geteilten Makros und unter den persönlichen jedes Mitglieds eindeutig
(`409 shortcut_taken`); es darf dem Kürzel eines Textbausteins gleichen. Unbekannte Labels, ein
Zuständiger, der kein Mitglied ist, dasselbe Label zum Hinzufügen und Entfernen, `snoozed` ohne
`snooze_minutes` oder ein Makro ohne Text und ohne Aktionen ergeben `400 validation_failed`.

| Wer | Darf |
|---|---|
| Jedes angemeldete Mitglied | Eigene persönliche Makros lesen, anlegen, ändern und löschen und die geteilten lesen. |
| Inhaber und Admins | Zusätzlich geteilte Makros anlegen, ändern und löschen. Agents erhalten `403 forbidden`. |
| Verbundene Apps (OAuth) | Persönliche Makros mit `conversations:write`; geteilte mit `canned_replies:write`. |
| API-Keys | Geteilte mit `conversations:read` lesen; mit `canned_replies:write` verwalten (ohne: `403 insufficient_scope`). Persönliche Makros: `403 member_session_required`. |

Das persönliche Makro eines anderen Mitglieds und ein Makro eines anderen Workspace ergeben `404`.

### Mit einem Makro senden

`POST /v1/conversations/{conversationId}/messages` nimmt zwei optionale Felder (in einer Multipart-Anfrage
ist `then` ein JSON-String):

| Feld | Bedeutung |
|---|---|
| `macro_id` | Wendet die Aktionen dieses Makros mit der Nachricht an. Geteilte Makros funktionieren für Schlüssel. |
| `then` | Ein `ConversationUpdate`, das nach dem Makro feldweise angewendet wird: `then.status` ersetzt Status und Zurückstellung des Makros, `then.labels` ersetzt die Labels (die Hinzufügungen und Entfernungen des Makros eingeschlossen). |

Nachricht und Änderungen sind eine Transaktion. Wird die Nachricht abgelehnt (ungültige Erwähnung,
Zustellung abgelehnt), wird nichts angewendet; wird eine Aktion abgelehnt (ein Zuständiger in `then` ohne
Zugriff auf den Posteingang, eine Zurückstellung in der Vergangenheit), wird auch die Nachricht nicht
geschrieben. Ausnahme ist der Zuständige des Makros selbst: Wurde dieses Mitglied entfernt oder hat es den
Zugriff auf den Posteingang verloren, gehen die Antwort und die übrigen Aktionen durch, und der Zuständige
wird übersprungen. Beide Felder brauchen für Schlüssel und Tokens `conversations:write`, werden bei einem
Entwurf abgelehnt (`400 validation_failed`) und nicht erneut angewendet, wenn eine wiederholte `client_id`
die gespeicherte Nachricht zurückgibt. Eine Zurückstellung zählt ab der Serverzeit des Sendens.

Die Timeline der Unterhaltung erhält direkt nach der Nachricht ein `macro_applied`-Ereignis mit `macro`
(`id`, `name`) und, wenn der Zuständige übersprungen wurde, `skipped: ["assignee"]`, danach die üblichen
Ereignisse `assigned`, `status_changed` und `labels_changed`. Prioritätsänderungen haben kein
Timeline-Ereignis.

[MCP](mcp.md)-Assistenten lesen Makros mit `list_macros`; Tools wenden sie nie an. Nutzen Sie den `body`
eines Makros mit `draft_reply`.

## Referenz

### Scopes

Ein Aufruf ohne den nötigen Scope antwortet mit `403 insufficient_scope`.

| Scope | Erlaubt |
|---|---|
| `conversations:read` | Unterhaltungen, Nachrichten, Anhänge, Labels, Textbausteine, den Event-Feed und Realtime |
| `conversations:write` | Status, Zuständige, Zurückstellen, Priorität, Labels; Unterhaltungen verschieben und gesammelt ändern |
| `conversations:delete` | Unterhaltungen in den Papierkorb legen, wiederherstellen, den Papierkorb auflisten. In keinem anderen Scope enthalten: Nennen Sie ihn ausdrücklich. Nur für Schlüssel und verbundene Apps von Inhabern und Admins verfügbar. |
| `messages:write` | Nachrichten und Entwürfe: anlegen, bearbeiten, verwerfen |
| `messages:redact` | [Den Inhalt einer Nachricht entfernen](#eine-nachricht-entfernen). In keinem anderen Scope enthalten: Nennen Sie ihn ausdrücklich. |
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

### Eine Nachricht entfernen

`POST /v1/messages/{messageId}/redact` entfernt den Inhalt einer einzelnen Nachricht oder Notiz und lässt eine Zeile
im Verlauf stehen: "Entfernt von <Name>" mit der Uhrzeit. Die Antwort ist `200` mit der Nachricht, die `redacted_at`,
`redacted_by`, einen leeren `body` und kein `html`, keine Anhänge, Karten oder Erwähnungen hat. Inhaber und Admins
dürfen jede Nachricht entfernen, andere Mitglieder nur die Nachrichten und Notizen, die sie selbst geschrieben haben
(`403 forbidden`). Ein Schlüssel braucht `messages:redact` und darf jede Nachricht entfernen, die er sehen kann;
`yuva api-key create` lässt den Scope weg, solange Sie ihn nicht nennen.

| Regel | Detail |
|---|---|
| Entfernt | Text, HTML, Anhänge samt gespeicherten Dateien und bei E-Mail Betreff, Header und Originalnachricht, in einem Schritt. Die Dateien löscht gleich danach ein Job. |
| Bleibt | Der Autor, die Zeit, die Stelle im Verlauf und bei E-Mail der Zustellstatus. |
| Andere Kopien | Frühere Ereignisse, gespeicherte Webhook-Zustellungen und Vorschauen verlieren den Inhalt; die Nachricht verschwindet aus der Suche. |
| Zitiert vom Kontakt | Beantwortet eine spätere E-Mail des Kontakts eine entfernte E-Mail und zitiert sie ganz, werden Zitat und Original dieser E-Mail ebenfalls entfernt; was neu geschrieben wurde, bleibt. Ein Teilzitat bleibt. |
| Erste Nachricht | Wird die erste Nachricht eines Feedbacks entfernt, wird `feedback.text` geleert; wird die E-Mail entfernt, die der Unterhaltung den Betreff gab, wird der Betreff geleert. Beides sendet `conversation.updated`. |
| Noch nicht gesendete E-Mail | Eine wartende E-Mail wird nicht gesendet und zeigt `failed`. |
| Schon gesendete E-Mail | Sie bleibt im Postfach des Empfängers: Yuva kann sie nicht zurückholen. |
| Nochmal | Eine bereits entfernte Nachricht erneut zu entfernen gibt sie unverändert zurück. |
| Nicht entfernbar | Ein Entwurf (verwerfen Sie ihn mit `DELETE`) und ein Verlaufsereignis antworten mit `409 not_redactable`. Eine Unterhaltung im Papierkorb antwortet mit `404`. |

Webhooks senden [`message.redacted`](webhooks.md#ereignisse) ohne den alten Inhalt; löschen Sie jede Kopie, die Sie
aufbewahren. Das [Audit-Log](operations.md#audit-log) hält jede Entfernung fest, nie den Text.
Im Panel steht **Entfernen** bei jeder Nachricht und Notiz für Inhaber, Admins und die Autorin oder den Autor. Eine
Bestätigung nennt die Folgen, auch dass eine gesendete E-Mail im Postfach des Empfängers bleibt; `Entf` bei einer
fokussierten Nachricht öffnet sie ebenfalls. Alle, die die Unterhaltung geöffnet haben, sehen dann live die Zeile
„Entfernt von <Name>“ mit der Uhrzeit.


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
