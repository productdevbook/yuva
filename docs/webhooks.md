# Webhooks

Yuva tells your backend what happens in conversations by sending signed HTTP requests that follow
[Standard Webhooks](https://www.standardwebhooks.com). Use them to send push notifications to your
apps, sync contacts with your users, or start your own automations.

## In short

- [Add an endpoint](#add-an-endpoint) in the panel or with an API key and keep its signing secret.
- Pick the [events](#events) you need; each request carries the [payload](#payload) described here.
- [Verify the signature](#verify-the-signature) on every request; failed deliveries are
  [retried](#delivery-and-retries).

## Add an endpoint

In the panel: Settings → Webhooks. Or with a workspace API key (or as an owner or admin):

```sh
curl -X POST https://support.example.com/v1/webhooks \
  -H "Authorization: Bearer $YUVA_API_KEY" -H "Content-Type: application/json" \
  -d '{"url": "https://api.example.com/yuva/webhook", "events": ["message.created", "contact.deleted"]}'
```

- Without `inbox_id`, the endpoint gets the events of the whole workspace; with it, only that
  inbox's conversations (and contact events for contacts with an external id or a conversation
  there).
- `include_notes: true` also sends `message.created` for members' notes; it is off by default.
  Internal events (assignments, status changes as messages) are never sent as messages.
- The answer contains the signing secret, `whsec_…`, **once**. Rotate it with
  `POST /v1/webhooks/{webhookId}/secret`; the old secret keeps signing next to the new one for 24
  hours, so you can switch without losing requests.

The URL must be `http` or `https` and reach the public internet: loopback, private, link-local,
CGNAT and other reserved addresses are refused when the endpoint is saved and again on every
delivery after resolving the name (`YUVA_WEBHOOK_ALLOW_PRIVATE` lifts this for development, see
[Configuration](configuration.md#webhooks)).

## Events

| Type | When | `data` |
|---|---|---|
| `conversation.created` | A conversation started on any channel (feedback included) | `conversation`, `contact` |
| `conversation.updated` | Status, assignee, priority, labels, subject or spam flag changed, or it moved to another contact or inbox (one per conversation for bulk changes) | `conversation`, `contact` |
| `message.created` | A message from or to the contact (and notes, when enabled) | `message`, `conversation`, `contact` |
| `feedback.created` | Feedback arrived from an app or `POST /v1/feedback` | `message`, `conversation`, `contact` |
| `contact.updated` | A contact changed | `contact` |
| `contact.deleted` | A contact was deleted, or merged into another (`merged_into_id`, which now has its external ids) | `contact` with the external ids it had |
| `draft.created`, `draft.updated`, `draft.deleted` | A bot or member wrote, edited or discarded a [draft](headless.md#a-bot-that-answers-with-drafts); nothing was delivered. Sending it emits `message.created` | `message` (the draft), `conversation`, `contact` |

The full schemas are in the `webhooks` section of [openapi/openapi.yaml](../openapi/openapi.yaml).

## Payload

```json
{
  "type": "message.created",
  "timestamp": "2026-10-07T12:00:00Z",
  "workspace_id": "0192f0c4-0000-7000-8000-000000000001",
  "inbox_id": "0192f0c4-0000-7000-8000-000000000002",
  "data": {
    "message": {
      "id": "0192f0c4-0000-7000-8000-000000000003",
      "conversation_id": "0192f0c4-0000-7000-8000-000000000004",
      "kind": "message",
      "direction": "out",
      "author": { "type": "member", "member_id": "0192f0c4-0000-7000-8000-000000000005" },
      "body": "We have shipped a fix.",
      "attachments": [],
      "created_at": "2026-10-07T12:00:00Z"
    },
    "conversation": { "id": "0192f0c4-0000-7000-8000-000000000004", "status": "open", "…": "…" },
    "contact": {
      "id": "0192f0c4-0000-7000-8000-000000000006",
      "name": "Jane Doe",
      "emails": ["jane@example.com"],
      "external_ids": [{ "inbox_id": "0192f0c4-0000-7000-8000-000000000002", "external_id": "user-42" }],
      "attributes": { "plan": "pro" },
      "online": false
    }
  }
}
```

`timestamp` is when the event happened, not when it was sent. `inbox_id` is absent for contact
events. `contact.online` is `false` when the contact had no live connection from the widget or an
SDK when the webhook was prepared; that is the moment to send a push (see
[Mobile SDKs](mobile.md#4-push-notifications)).

## Verify the signature

Every request carries three headers:

```
webhook-id: msg_…            the same on every retry of this delivery
webhook-timestamp: 1791374400
webhook-signature: v1,<base64>   space-separated when two secrets sign during a rotation
```

The signature is `base64(HMAC-SHA256(key, "<webhook-id>.<webhook-timestamp>.<raw body>"))`, where
the key is the base64-decoded part of the secret after `whsec_`. Verify the **raw** body before
parsing it, accept any of the `v1` signatures, compare in constant time and refuse timestamps more
than a few minutes away from your clock. Any Standard Webhooks library does this.

In Go, with `sdk/go/webhook`:

```go
import "github.com/productdevbook/yuva/sdk/go/webhook"

func handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err := webhook.Verify(os.Getenv("YUVA_WEBHOOK_SECRET"), r.Header, body, 5*time.Minute); err != nil {
		http.Error(w, "bad signature", http.StatusUnauthorized)
		return
	}
	var event struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	_ = json.Unmarshal(body, &event)
	w.WriteHeader(http.StatusNoContent)
}
```

`webhook.Sign` computes the same header, for testing your handler.

In Node.js:

```js
import { createHmac, timingSafeEqual } from "node:crypto";

function verify(secret, headers, rawBody) {
  const id = headers["webhook-id"], ts = headers["webhook-timestamp"];
  if (Math.abs(Date.now() / 1000 - Number(ts)) > 300) return false;
  const key = Buffer.from(secret.replace(/^whsec_/, ""), "base64");
  const want = createHmac("sha256", key).update(`${id}.${ts}.`).update(rawBody).digest();
  return headers["webhook-signature"].split(" ").some((s) => {
    const [version, sig] = s.split(",");
    const got = Buffer.from(sig ?? "", "base64");
    return version === "v1" && got.length === want.length && timingSafeEqual(got, want);
  });
}
```

## Delivery and retries

- Answer with any `2xx` within 10 seconds. Redirects count as failures; at most 64 KiB of the
  answer is read.
- Anything else is retried after 5 s, 1 min, 5 min, 30 min, 1 h, 2 h, 4 h, 8 h and 9 h (with up to
  10% jitter), about 24 hours in all, with the same `webhook-id`. Deliveries can arrive more than
  once and out of order: ignore a `webhook-id` you have handled, and order by `timestamp`.
- An endpoint whose every attempt has failed for 24 hours, or that answers `410 Gone`, is disabled;
  the reason is shown in the panel. Enabling it again clears the reason.
- The delivery log keeps the newest 100 attempts per endpoint (status, latency, the start of the
  answer, the error): `GET /v1/webhooks/{webhookId}/deliveries` and
  `GET /v1/webhooks/{webhookId}/attempts`. Send a delivery again with
  `POST /v1/webhooks/{webhookId}/deliveries/{deliveryId}/redeliver`. Finished deliveries are kept
  7 days.
