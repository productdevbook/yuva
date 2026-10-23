# Webhooks

Yuva schickt Ihrem Backend eine signierte HTTP-Anfrage, wenn etwas passiert, damit Sie Pushes
senden, Kontakte abgleichen oder automatisieren können. Die Anfragen folgen
[Standard Webhooks](https://www.standardwebhooks.com).

## Einen Endpunkt anlegen

Im Panel: Settings → Webhooks. Oder mit einem API-Schlüssel, der `webhooks:manage` hat:

```sh
curl -X POST https://support.example.com/v1/webhooks \
  -H "Authorization: Bearer $YUVA_API_KEY" -H "Content-Type: application/json" \
  -d '{"url": "https://api.example.com/yuva/webhook", "events": ["message.created"]}'
```

Speichern Sie das Signatur-Secret (`whsec_…`) aus der Antwort; es wird nur einmal angezeigt.

## Die Signatur verifizieren

Prüfen Sie jede Anfrage am **rohen** Body, vor dem Parsen. In Go:

```go
import "github.com/productdevbook/yuva/sdk/go/webhook"

err := webhook.Verify(os.Getenv("YUVA_WEBHOOK_SECRET"), r.Header, body, 5*time.Minute)
```

Jede Standard-Webhooks-Bibliothek funktioniert ebenso.

## Schnell antworten

Antworten Sie innerhalb von 10 Sekunden mit `2xx`. Fehlschläge werden etwa 24 Stunden lang
wiederholt. Eine Zustellung kann doppelt oder in falscher Reihenfolge ankommen: Überspringen Sie
eine `webhook-id`, die Sie schon verarbeitet haben, und sortieren Sie nach `timestamp`.

## Referenz

### Ereignisse

| Typ | Wann | `data` |
|---|---|---|
| `conversation.created` | Eine Unterhaltung hat begonnen, Feedback eingeschlossen | `conversation`, `contact` |
| `conversation.updated` | Status, Zuständige, Priorität, Labels, Betreff oder Spam-Markierung haben sich geändert, oder sie wurde verschoben | `conversation`, `contact` |
| `conversation.rated` | Der Kontakt hat sie bewertet (`conversation.rating`) | `conversation`, `contact` |
| `conversation.trashed`, `conversation.restored` | Eine Unterhaltung wurde in den [Papierkorb](headless.md#unterhaltungen-löschen) gelegt oder wiederhergestellt | `conversation`, `contact` |
| `conversation.deleted` | Der Papierkorb hat sie 30 Tage nach dem Ablegen endgültig gelöscht | `conversation` nur mit `id`, `inbox_id`, `contact_id` |
| `message.created` | Eine Nachricht vom oder an den Kontakt; Notizen, wenn aktiviert | `message`, `conversation`, `contact` |
| `feedback.created` | Feedback aus einer App oder über `POST /v1/feedback`; Feedback aus einer App kann `feedback.score` (1 bis 5) tragen | `message`, `conversation`, `contact` |
| `contact.updated` | Ein Kontakt hat sich geändert | `contact` |
| `contact.deleted` | Ein Kontakt wurde gelöscht oder zusammengeführt (`merged_into_id`) | `contact` |
| `draft.created`, `draft.updated`, `draft.deleted` | Ein [Entwurf](headless.md#ein-bot-der-antworten-entwirft) hat sich geändert; wird er gesendet, folgt `message.created` | `message`, `conversation`, `contact` |
| `discussion.created` | Ein [Diskussions](documentation-pages.md)-Thread wurde begonnen, auch während er auf ein Mitglied wartet (`state: pending`) | `discussion` |
| `discussion.post_created` | In einem Thread wurde eine Antwort geschrieben, auch während sie auf ein Mitglied wartet | `discussion`, `post` |
| `discussion.answered` | Eine Antwort wurde als Lösung akzeptiert | `discussion`, `post` |
| `discussion.updated` | Ein Thread oder Beitrag wurde freigegeben, ausgeblendet, als Spam markiert, bearbeitet, gelöscht, gesperrt oder entsperrt, oder die Lösung wurde zurückgenommen | `discussion`, und `post`, wenn sich ein Beitrag geändert hat |
| `subscription.created` | Jemand hat eine [Liste](lists.md) des Posteingangs des Endpunkts abonniert, in jedem Zustand (`pending`, solange die Bestätigung aussteht) | `subscription`, `list`, `contact` |
| `subscription.updated` | Ein Abonnement wurde bestätigt, abgemeldet (mit `unsubscribe_reason`) oder erneut abonniert | `subscription`, `list`, `contact`, `previous_state` |
| `analytics.alert` | Eine [Analytics](analytics.md)-Website hat einen [Alert](analytics-support.md#alerts) ausgelöst: Spike, Einbruch, Goal-Einbruch, Support-Last oder neuer Referrer | `alert`, `property` |
| `broadcast.updated` | Ein [Broadcast](broadcasts.md) hat den Status gewechselt: geplant, sendend, pausiert, gesendet, abgebrochen, fehlgeschlagen oder zurück zum Entwurf, oder sein Tempo hat sich geändert | `broadcast`, `previous_state` |
| `status.component.updated` | Eine Komponente einer [Statusseite](status-pages.md) wurde angelegt, geändert oder hat einen neuen Zustand | `component`, `page`, `previous_state` |
| `status.incident.created`, `status.incident.updated`, `status.incident.resolved` | Ein veröffentlichter Vorfall oder eine Wartung wurde veröffentlicht, geändert oder gelöst bzw. abgeschlossen | `incident`, `page`, `change`, `update` |
| `status.monitor.down`, `status.monitor.up` | Ein Monitor ist ausgefallen oder hat sich erholt | `monitor`, `page`, `incident_id` |
| `help.article.published`, `help.article.unpublished`, `help.article.deleted` | Die Übersetzung eines Artikels eines [Hilfecenters](help-center.md) wurde veröffentlicht, zurückgezogen oder gelöscht (ein ganzer Artikel, wenn `locale` fehlt) | `center_id`, `article_id`, `number`, `locale`; bei Veröffentlichung `title` und `url` |
| `changelog.entry.published`, `changelog.entry.unpublished`, `changelog.entry.deleted` | Ein [Neuigkeiten](whats-new.md)-Eintrag wurde veröffentlicht (auch planmäßig oder wenn eine veröffentlichte Übersetzung ersetzt wurde), zurückgezogen, oder ein veröffentlichter Eintrag wurde gelöscht | `changelog` (`id`, `name`, `url`), `entry` (`id`, `number`, `labels`, `published_at`, `translations` mit `locale`, `title`, `url`) |
| `discussion.stage_changed` | Eine Idee der [Roadmap](roadmap-board.md) wurde in eine andere Phase verschoben | `discussion`, `from`, `to`, `linked` (mit der Idee verknüpftes Feedback) |

Jeder Body hat `type`, `timestamp` (wann es passiert ist), `workspace_id`, `inbox_id` (nicht bei
Kontaktereignissen) und `data`. `data.contact.online` ist `false`, wenn der Kontakt keine
Live-Verbindung hat: der Moment für einen [Push](mobile.md#4-push-benachrichtigungen). Die
vollständigen Schemas stehen im Abschnitt `webhooks` von
[openapi.yaml](../../openapi/openapi.yaml).

### Felder eines Endpunkts

| Feld | Bedeutung |
|---|---|
| `url` | Öffentliches `http` oder `https`. Private und reservierte Adressen werden abgelehnt, außer `YUVA_WEBHOOK_ALLOW_PRIVATE` ist gesetzt ([Konfiguration](configuration.md#webhooks)). |
| `events` | Zu sendende Ereignistypen. |
| `inbox_id` | Nur dieser Posteingang; ohne Angabe der ganze Workspace. |
| `include_notes` | Sendet auch Notizen von Mitgliedern. Standardmäßig aus. |

### Signatur

| Header | Bedeutung |
|---|---|
| `webhook-id` | Bei jeder Wiederholung gleich. |
| `webhook-timestamp` | Unix-Sekunden. Lehnen Sie Werte ab, die einige Minuten abweichen. |
| `webhook-signature` | `v1,` + `base64(HMAC-SHA256(key, "<id>.<timestamp>.<raw body>"))`, wobei `key` das Base64-dekodierte Secret nach `whsec_` ist. Während einer Rotation zwei, durch Leerzeichen getrennt. |

### Zustellung

| Regel | Detail |
|---|---|
| Wiederholungen | Nach 5 s, 1 min, 5 min, 30 min, 1 h, 2 h, 4 h, 8 h, 9 h, plus bis zu 10 %. Weiterleitungen zählen als Fehlschlag. |
| Deaktiviert | Nach 24 Stunden mit Fehlschlägen oder einem `410 Gone`. Im Panel wieder aktivieren. |
| Secret rotieren | `POST /v1/webhooks/{webhookId}/secret`; das alte signiert noch 24 Stunden lang mit. |
| Protokoll | Die letzten 100 Versuche: `GET /v1/webhooks/{webhookId}/deliveries`, `GET /v1/webhooks/{webhookId}/attempts`. |
| Erneut zustellen | `POST /v1/webhooks/{webhookId}/deliveries/{deliveryId}/redeliver`. |
