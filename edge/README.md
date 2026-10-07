# yuva-edge

A Cloudflare Email Worker that forwards inbound mail to Yuva. It does nothing else: it reads the raw
message and the envelope, signs the request, POSTs it to `/ingress/email` and rejects the message
when Yuva refuses it. Parsing, threading and spam decisions belong to the server.

## Configuration

Nothing account-specific is in `wrangler.toml`. Set these on the Worker:

| Name | Kind | Value |
|---|---|---|
| `YUVA_URL` | variable | Base URL of the Yuva server, e.g. `https://yuva.example.com` |
| `INGRESS_SECRET` | secret | Shared secret, the same value the server uses to verify requests |

```sh
bun install
bunx wrangler secret put INGRESS_SECRET
bunx wrangler deploy --var YUVA_URL:https://yuva.example.com
```

`keep_vars = true` keeps variables set in the dashboard across deploys. Then route the support
addresses to the Worker in Email Routing (dashboard, or your own infrastructure code).

## Check

```sh
bun run check   # typecheck + tests
```

## Ingress contract

Any MTA or relay can implement the same request; the endpoint is not tied to Cloudflare.

```
POST {YUVA_URL}/ingress/email
Content-Type: message/rfc822
X-Yuva-Envelope-To: <envelope recipient (RCPT TO)>
X-Yuva-Envelope-From: <envelope sender (MAIL FROM)>
X-Yuva-Timestamp: <Unix time in seconds, decimal>
X-Yuva-Signature: v1=<hex>

<raw RFC 5322 message, bytes exactly as received>
```

Signature: lowercase hex of HMAC-SHA256, keyed with `INGRESS_SECRET` (its UTF-8 bytes), over

```
<X-Yuva-Timestamp> "." <X-Yuva-Envelope-To> "." <raw body bytes>
```

The prefix is UTF-8; the body is not re-encoded. The server must

- recompute the HMAC over the bytes it received and compare in constant time;
- refuse a timestamp too far from its own clock (replay window, e.g. 5 minutes);
- treat any prefix other than `v1=` as invalid; other versions are reserved.

Responses:

| Status | Meaning | Worker does |
|---|---|---|
| 2xx | Accepted (stored or deliberately dropped) | nothing, the message is delivered |
| 4xx with `{ "reason": string }` | Permanent refusal (unknown recipient, blocked sender, bad signature, too large) | `setReject(reason)`: the sender gets a permanent SMTP error with that text |
| 4xx without a JSON reason | Same | `setReject("Rejected by recipient server (<status>)")` |
| 5xx, network error | Temporary failure | throws, so delivery fails and the sending server retries |

The `reason` is shown to the original sender in the bounce, so it must not leak internal details.
Inbound messages are at most 25 MiB (Cloudflare Email Routing limit); the server should accept at
least that.
