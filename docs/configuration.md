# Configuration

Yuva reads its whole configuration from environment variables when it starts. Values are trimmed;
an empty variable counts as unset and takes the default. Lists are comma-separated. Booleans
accept `true`/`false`, `1`/`0`, `t`/`f`. The server refuses to start on an invalid value and
says which variable is wrong.

Settings that belong to one workspace, inbox or channel (SMTP accounts of e-mail channels, allowed
origins of chat channels, webhooks, business hours, …) are not environment variables; they are
made in the panel, through `/v1`, or with the [operator commands](operations.md#operator-commands).

## Required

| Variable | Meaning |
|---|---|
| `YUVA_DATABASE_URL` | Postgres connection URL, e.g. `postgres://yuva:secret@db:5432/yuva?sslmode=disable`. Postgres 16 or newer. Every command that touches the database needs it. |
| `YUVA_MASTER_KEY` | 32 random bytes, base64-encoded (`openssl rand -base64 32`). Encrypts the secrets Yuva stores: SMTP passwords, inbox identity secrets and webhook signing secrets. `serve`, `ingest-email`, `inbox` and `channel` refuse to run without it. See [Master key](install.md#master-key). |
| `YUVA_PUBLIC_URL` | The URL people and apps reach the server at, e.g. `https://support.example.com`. Default `http://localhost:8080`, which is only right for trying Yuva on your own machine. Must be an absolute `http` or `https` URL; serve Yuva at the root of its host name. It sets the defaults of the cookie, passkey and origin settings below and the links in e-mails. |

## Server

| Variable | Default | Meaning |
|---|---|---|
| `YUVA_LISTEN_ADDR` | `:8080` | Address of the HTTP server: panel, API, WebSockets, widget scripts, ingress, `/healthz`, `/readyz`. |
| `YUVA_METRICS_ADDR` | `:9090` | Address of the Prometheus listener (`GET /metrics`). Keep it off the public internet. `off` or an empty value turns the listener off. |
| `YUVA_CLIENT_IP_HEADER` | unset | Header your reverse proxy puts the client's IP address in, e.g. `X-Real-IP`. Rate limits are counted per client IP; unset, the TCP peer address is used, which behind a proxy is the proxy. With a comma-separated list such as `X-Forwarded-For`, Yuva takes the rightmost address that is not in `YUVA_TRUSTED_PROXIES` (the one your proxy appended); values further left are whatever the client sent. |
| `YUVA_TRUSTED_PROXIES` | unset | Comma-separated addresses or CIDR ranges of your reverse proxies, e.g. `10.0.0.0/8,fd00::/8`. When set, `YUVA_CLIENT_IP_HEADER` is used only for requests whose TCP peer is one of them, and their own entries are skipped when reading the header from the right. Set it when more than one proxy appends to the header. |
| `YUVA_COOKIE_SECURE` | `true` when `YUVA_PUBLIC_URL` is `https` | Marks the member session cookie `Secure`. |

## Sign-in and passkeys

| Variable | Default | Meaning |
|---|---|---|
| `YUVA_WEBAUTHN_RP_ID` | host name of `YUVA_PUBLIC_URL` | Passkey relying party id. Changing it later makes existing passkeys unusable. |
| `YUVA_WEBAUTHN_RP_NAME` | `Yuva` | Name browsers show when a passkey is created. |
| `YUVA_WEBAUTHN_ORIGINS` | origin of `YUVA_PUBLIC_URL` | Origins allowed for passkey ceremonies. The panel's realtime socket also accepts these origins next to `YUVA_PUBLIC_URL`. |

## Server e-mail

The server's own mail: sign-in codes, invitations and member notification e-mails. E-mail channels
send replies to contacts through their own SMTP account (see [E-mail](email.md#outbound-smtp)).

| Variable | Default | Meaning |
|---|---|---|
| `YUVA_SMTP_HOST` | unset | SMTP server. Unset, mails are written to the log instead of being sent (the server warns at start), which is enough to read a sign-in code while testing. |
| `YUVA_SMTP_PORT` | `465` with `YUVA_SMTP_TLS=tls`, else `587` | SMTP port. |
| `YUVA_SMTP_TLS` | `starttls` | `starttls`, `tls` (implicit TLS) or `none`. |
| `YUVA_SMTP_USERNAME` | unset | SMTP user name. |
| `YUVA_SMTP_PASSWORD` | unset | SMTP password. |
| `YUVA_SMTP_FROM` | unset | Sender, e.g. `Yuva <yuva@example.com>`. Required when `YUVA_SMTP_HOST` is set. |
| `YUVA_SMTP_ALLOW_PRIVATE` | `false` | Lets e-mail channels send through SMTP servers on loopback and private addresses, such as Mailpit in development (`deploy/compose.yaml` sets it). Without it, a channel cannot be saved with such a literal address or `localhost` as its SMTP host, and a channel whose host name resolves to such an address fails to send. Link-local and cloud metadata addresses stay refused. Does not affect `YUVA_SMTP_HOST`. |

## Attachments and storage

Attachments and the original of every inbound e-mail are stored as objects, never public; members
download them through the API, which checks inbox access.

| Variable | Default | Meaning |
|---|---|---|
| `YUVA_STORAGE` | `local` | `local` (a directory) or `s3` (any S3-compatible service: S3, R2, MinIO, …). |
| `YUVA_STORAGE_DIR` | `data/attachments` | Directory for `local`. The Docker image has `/data/attachments`; mount a volume at `/data`. |
| `YUVA_S3_ENDPOINT` | unset | S3 endpoint URL, e.g. `https://s3.eu-central-1.amazonaws.com` or your R2 or MinIO endpoint. |
| `YUVA_S3_REGION` | `auto` | Region. |
| `YUVA_S3_BUCKET` | unset | Bucket. |
| `YUVA_S3_ACCESS_KEY_ID` | unset | Access key id. |
| `YUVA_S3_SECRET_ACCESS_KEY` | unset | Secret access key. |
| `YUVA_S3_PATH_STYLE` | `false` | Path-style URLs (`endpoint/bucket/key`), which MinIO usually needs. |
| `YUVA_ATTACHMENT_MAX_BYTES` | `26214400` (25 MiB) | Largest attachment accepted, in bytes. |
| `YUVA_ATTACHMENT_TYPES` | see below | Allowed content types, comma-separated; `image/*` style wildcards are allowed. |

The default `YUVA_ATTACHMENT_TYPES` is `image/png,image/jpeg,image/gif,image/webp,image/heic,application/pdf,text/plain,text/csv,application/zip,application/json,video/mp4,video/quicktime,audio/mpeg,audio/mp4`.
Inbound e-mail parts outside these rules are skipped; they stay in the stored original.

## Inbound e-mail

| Variable | Default | Meaning |
|---|---|---|
| `YUVA_INGRESS_SECRET` | unset | Shared secret that signs requests to `/ingress/email` (the Cloudflare Email Worker uses the same value). Unset, `/ingress/email` refuses all mail. `yuva ingest-email` does not need it. Generate it with `openssl rand -hex 32`. |
| `YUVA_INGRESS_ACCEPT_V1` | `false` | Also accept the deprecated `v1` request signature, which does not cover `X-Yuva-Envelope-From` (see `edge/README.md`). Under `v1` the envelope sender is not trusted: an empty one never makes a message a delivery report or automatic. Set it only while an older Email Worker or relay still signs with `v1`; the server warns at start while it is on. |
| `YUVA_INGRESS_AUTHSERV_ID` | unset | The authserv-id of the server that receives your mail and stamps `Authentication-Results`, e.g. `mx.cloudflare.net` for Cloudflare Email Routing. Only the topmost `Authentication-Results` is read, and its DMARC verdict is used (spam flag, threading) only when it starts with this id; any other header could have come with the message. Unset, the header is stored and shown but DMARC is treated as unknown. |
| `YUVA_INGRESS_MAX_CONCURRENT` | `8` | Most messages `/ingress/email` processes at once per server process. Further requests are answered `503 unavailable` before their body is read; the Email Worker then forwards to its fallback address or fails the delivery so the sending server retries. |
| `YUVA_SES_TOPIC_ARNS` | unset | Amazon SNS topic ARNs whose SES bounce and complaint notifications `/ingress/ses` accepts, comma-separated. Unset, none are accepted. |
| `YUVA_EMAIL_SENDER_HOURLY_CAP` | `500` | Most inbound e-mails one sender may send to a workspace per hour. Mail beyond it is refused (`429 rate_limited`, a permanent rejection through the Email Worker). Below it no mail is refused for volume. |

## Chat

| Variable | Default | Meaning |
|---|---|---|
| `YUVA_CHAT_EMAIL_DELAY` | `5m` | How long a member's reply stays unread, and the contact gone, before it is e-mailed to a contact who left the chat. A Go duration of at least `1s`, such as `90s` or `10m`. |
| `YUVA_ANONYMOUS_CONTACTS_PER_HOUR` | `20` | Most new anonymous visitors one IP address (an IPv6 /64) may start per chat or app channel and hour; more answer `429 anonymous_limit`. Resuming a visitor with its `visitor_id` does not count. Counted per server process. |

## Webhooks

| Variable | Default | Meaning |
|---|---|---|
| `YUVA_WEBHOOK_ALLOW_PRIVATE` | `false` | Lets webhooks reach loopback and private addresses, for development. Never set it on a server others use. Link-local and cloud metadata addresses stay refused. |

## Web Push

Member notifications on phones and desktops through the installable panel. Without keys, Web Push
is off and members get e-mail notifications only.

| Variable | Default | Meaning |
|---|---|---|
| `YUVA_VAPID_PUBLIC_KEY` | unset | VAPID public key. Set both keys or neither; `yuva vapid-keys` prints a pair. |
| `YUVA_VAPID_PRIVATE_KEY` | unset | VAPID private key. |
| `YUVA_VAPID_SUBJECT` | `mailto:` + the address in `YUVA_SMTP_FROM`, else `YUVA_PUBLIC_URL` when it is `https` | Contact for push services, `mailto:you@example.com` or an `https://` URL. Required when neither default applies. |

Keep the key pair once members have subscribed: browser subscriptions are bound to the public key,
so after a change members have to turn notifications on again.

## Build

The version the server reports (`GET /v1/version`, the start log) is set at build time with the
Docker build argument `VERSION` (`-X main.version`); it is not an environment variable.
