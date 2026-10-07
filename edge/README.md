# yuva-edge

A Cloudflare Email Worker that forwards inbound mail to Yuva. It does nothing else: it reads the raw
message and the envelope, signs the request, POSTs it to `/ingress/email` and rejects the message
when Yuva refuses it. When Yuva cannot be reached it can forward the message to a fallback address
instead. Parsing, threading and spam decisions belong to the server.

## Configuration

Nothing account-specific is in `wrangler.toml`. Set these on the Worker:

| Name | Kind | Value |
|---|---|---|
| `YUVA_URL` | variable | Base URL of the Yuva server, e.g. `https://yuva.example.com` |
| `INGRESS_SECRET` | secret | Shared secret, the same value the server uses to verify requests |
| `FALLBACK_FORWARD` | variable, optional | Address that receives the message when Yuva fails (5xx, network error, no answer within 20 seconds). It must be a verified destination address in Email Routing. |

```sh
bun install
bunx wrangler secret put INGRESS_SECRET
bunx wrangler deploy --var YUVA_URL:https://yuva.example.com
```

Add `--var FALLBACK_FORWARD:you@example.org` to keep mail during an outage: the message then
arrives as a plain forward to that address and does not appear in Yuva. Without it a failure ends
the delivery with an error. `keep_vars = true` keeps variables set in the dashboard across deploys. Then route the support
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
X-Yuva-Signature: v2=<hex>

<raw RFC 5322 message, bytes exactly as received>
```

Signature: lowercase hex of HMAC-SHA256, keyed with `INGRESS_SECRET` (its UTF-8 bytes), over

```
<X-Yuva-Timestamp> "." <X-Yuva-Envelope-To> "." <X-Yuva-Envelope-From> "." <raw body bytes>
```

An empty envelope sender (a bounce's null reverse-path) leaves its place empty: `<ts>.<to>..<body>`.
The header values are used exactly as sent. The prefix is UTF-8; the body is not re-encoded. The
server

- refuses a missing or malformed signature or timestamp, and a timestamp more than 5 minutes from
  its own clock, before it reads the body;
- recomputes the HMAC over the bytes it receives and compares in constant time;
- treats any prefix other than `v2=` as invalid. `v1=`, the deprecated version over
  `<X-Yuva-Timestamp> "." <X-Yuva-Envelope-To> "." <raw body bytes>` without the envelope sender, is
  accepted only while the server sets `YUVA_INGRESS_ACCEPT_V1=true`, and then the envelope sender is
  not trusted (an empty one does not make the message a delivery report). This Worker signs `v2`;
  redeploy a Worker that still signs `v1`.

Responses:

| Status | Meaning | Worker does |
|---|---|---|
| 2xx | Accepted (stored or deliberately dropped) | nothing, the message is delivered |
| 4xx with `{ "reason": string }` | Permanent refusal (unknown recipient, blocked sender, bad signature, too large) | `setReject(reason)`: the sender gets a permanent SMTP error with that text |
| 4xx without a JSON reason | Same | `setReject("Rejected by recipient server (<status>)")` |
| 5xx, network error, timeout (20 s) | Temporary failure, including `503 unavailable` when the server is processing `YUVA_INGRESS_MAX_CONCURRENT` messages already | `forward(FALLBACK_FORWARD)` when set; otherwise throws, so delivery fails and the sending server may retry |

The `reason` is shown to the original sender in the bounce, so it must not leak internal details.
Inbound messages are at most 25 MiB (Cloudflare Email Routing limit); the server should accept at
least that.

Cloudflare Email Routing adds an `Authentication-Results` header with the authserv-id
`mx.cloudflare.net` on top of the message the Worker receives. Set `YUVA_INGRESS_AUTHSERV_ID` to that
value on the server so its DMARC verdict is used; any other `Authentication-Results` is ignored.
