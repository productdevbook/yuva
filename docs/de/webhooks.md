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
| `message.created` | Eine Nachricht vom oder an den Kontakt; Notizen, wenn aktiviert | `message`, `conversation`, `contact` |
| `feedback.created` | Feedback aus einer App oder über `POST /v1/feedback` | `message`, `conversation`, `contact` |
| `contact.updated` | Ein Kontakt hat sich geändert | `contact` |
| `contact.deleted` | Ein Kontakt wurde gelöscht oder zusammengeführt (`merged_into_id`) | `contact` |
| `draft.created`, `draft.updated`, `draft.deleted` | Ein [Entwurf](headless.md#ein-bot-der-antworten-entwirft) hat sich geändert; wird er gesendet, folgt `message.created` | `message`, `conversation`, `contact` |

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
