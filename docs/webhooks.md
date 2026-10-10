# Webhooks

Yuva sends your backend a signed HTTP request when something happens, so you can send pushes, sync
contacts or automate. Requests follow [Standard Webhooks](https://www.standardwebhooks.com).

## Add an endpoint

In the panel: Settings → Webhooks. Or with an API key that holds `webhooks:manage`:

```sh
curl -X POST https://support.example.com/v1/webhooks \
  -H "Authorization: Bearer $YUVA_API_KEY" -H "Content-Type: application/json" \
  -d '{"url": "https://api.example.com/yuva/webhook", "events": ["message.created"]}'
```

Save the signing secret (`whsec_…`) from the answer; it is shown once.

## Verify the signature

Check every request on its **raw** body, before parsing. In Go:

```go
import "github.com/productdevbook/yuva/sdk/go/webhook"

err := webhook.Verify(os.Getenv("YUVA_WEBHOOK_SECRET"), r.Header, body, 5*time.Minute)
```

Any Standard Webhooks library works too.

## Answer quickly

Answer `2xx` within 10 seconds. Failures are retried for about 24 hours. A delivery can arrive twice
or out of order: skip a `webhook-id` you have handled and order by `timestamp`.

## Reference

### Events

| Type | When | `data` |
|---|---|---|
| `conversation.created` | A conversation started, feedback included | `conversation`, `contact` |
| `conversation.updated` | Status, assignee, priority, labels, subject or spam flag changed, or it moved | `conversation`, `contact` |
| `conversation.rated` | The contact rated it (`conversation.rating`) | `conversation`, `contact` |
| `message.created` | A message from or to the contact; notes if enabled | `message`, `conversation`, `contact` |
| `feedback.created` | Feedback from an app or `POST /v1/feedback` | `message`, `conversation`, `contact` |
| `contact.updated` | A contact changed | `contact` |
| `contact.deleted` | A contact was deleted, or merged (`merged_into_id`) | `contact` |
| `draft.created`, `draft.updated`, `draft.deleted` | A [draft](headless.md#a-bot-that-answers-with-drafts) changed; sending it emits `message.created` | `message`, `conversation`, `contact` |

Every body has `type`, `timestamp` (when it happened), `workspace_id`, `inbox_id` (not on contact
events) and `data`. `data.contact.online` is `false` when the contact has no live connection: the
moment for a [push](mobile.md#4-push-notifications). Full schemas are in the `webhooks` section of
[openapi.yaml](../openapi/openapi.yaml).

### Endpoint fields

| Field | Meaning |
|---|---|
| `url` | Public `http` or `https`. Private and reserved addresses are refused unless `YUVA_WEBHOOK_ALLOW_PRIVATE` is set ([Configuration](configuration.md#webhooks)). |
| `events` | Event types to send. |
| `inbox_id` | Only this inbox; without it, the whole workspace. |
| `include_notes` | Also send members' notes. Off by default. |

### Signature

| Header | Meaning |
|---|---|
| `webhook-id` | Same on every retry. |
| `webhook-timestamp` | Unix seconds. Refuse values a few minutes off. |
| `webhook-signature` | `v1,` + `base64(HMAC-SHA256(key, "<id>.<timestamp>.<raw body>"))`, `key` being the base64-decoded secret after `whsec_`. Two, space-separated, during a rotation. |

### Delivery

| Rule | Detail |
|---|---|
| Retries | After 5 s, 1 min, 5 min, 30 min, 1 h, 2 h, 4 h, 8 h, 9 h, plus up to 10%. Redirects count as failures. |
| Disabled | After 24 hours of failures, or a `410 Gone`. Enable it again in the panel. |
| Rotate secret | `POST /v1/webhooks/{webhookId}/secret`; the old one keeps signing for 24 hours. |
| Log | Last 100 attempts: `GET /v1/webhooks/{webhookId}/deliveries`, `GET /v1/webhooks/{webhookId}/attempts`. |
| Redeliver | `POST /v1/webhooks/{webhookId}/deliveries/{deliveryId}/redeliver`. |
