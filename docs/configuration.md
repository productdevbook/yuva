# Configuration

Yuva reads its settings from environment variables when it starts. A wrong value stops the server
with a message that names the variable.

Settings of a workspace, inbox or channel live in the panel, the API and the
[operator commands](operations.md#operator-commands), not here.

## Set a value

1. Add the variable to `.env`, next to `compose.yaml`:

   ```sh
   YUVA_CHAT_EMAIL_DELAY=10m
   ```

2. Restart the server:

   ```sh
   docker compose up -d
   ```

An empty variable means unset and takes the default, except for `YUVA_METRICS_ADDR`. Lists are comma-separated. Booleans accept
`true`/`false`, `1`/`0` and `t`/`f`.

## Required

| Variable | Meaning |
|---|---|
| `YUVA_DATABASE_URL` | Postgres 16+ URL, e.g. `postgres://yuva:secret@db:5432/yuva?sslmode=disable`. |
| `YUVA_MASTER_KEY` | 32 random bytes, base64 (`openssl rand -base64 32`). Encrypts stored secrets. See [Master key](install.md#master-key). |
| `YUVA_PUBLIC_URL` | The URL people and apps use, e.g. `https://support.example.com`, at the root of its host. Default `http://localhost:8080`, for local tries only. |

## Server

| Variable | Default | Meaning |
|---|---|---|
| `YUVA_LISTEN_ADDR` | `:8080` | HTTP address for everything except metrics. |
| `YUVA_METRICS_ADDR` | `:9090` | Prometheus address (`GET /metrics`). Keep it private. `off` or an empty value turns it off. |
| `YUVA_CLIENT_IP_HEADER` | unset | Header your proxy writes the client IP into, e.g. `X-Real-IP`, for rate limits. In a list like `X-Forwarded-For`, the rightmost address not in `YUVA_TRUSTED_PROXIES` counts. |
| `YUVA_TRUSTED_PROXIES` | unset | Addresses or CIDR ranges of your proxies, e.g. `10.0.0.0/8`. Then the client IP header is read only from them. |
| `YUVA_COOKIE_SECURE` | `true` when `YUVA_PUBLIC_URL` is `https` | Marks the session cookie `Secure`. |
| `YUVA_MCP` | `on` | `off` turns off the MCP endpoint `/mcp` (it answers `404`). OAuth stays on. |
| `YUVA_PANEL` | `on` | `off` stops serving the panel and OAuth consent; API keys keep working. |
| `YUVA_WIDGET` | `on` | `off` stops serving `/yuva.js`, `/yuva-chat.js` and `/yuva-docs.js`; `/client/v1` stays. |

## Sign-in and passkeys

| Variable | Default | Meaning |
|---|---|---|
| `YUVA_WEBAUTHN_RP_ID` | host name of `YUVA_PUBLIC_URL` | Passkey relying party id. Changing it makes existing passkeys unusable. |
| `YUVA_WEBAUTHN_RP_NAME` | `Yuva` | Name browsers show for a new passkey. |
| `YUVA_WEBAUTHN_ORIGINS` | origin of `YUVA_PUBLIC_URL` | Origins allowed for passkeys and the panel's realtime socket. |

## Server e-mail

Sign-in codes, invitations and notifications. E-mail channels have [their own SMTP](email.md#outbound-smtp).

| Variable | Default | Meaning |
|---|---|---|
| `YUVA_SMTP_HOST` | unset | SMTP server. Unset, mail is written to the log instead. |
| `YUVA_SMTP_PORT` | `465` with `tls`, else `587` | SMTP port. |
| `YUVA_SMTP_TLS` | `starttls` | `starttls`, `tls` (implicit TLS) or `none`. |
| `YUVA_SMTP_USERNAME` | unset | SMTP user name. |
| `YUVA_SMTP_PASSWORD` | unset | SMTP password. |
| `YUVA_SMTP_FROM` | unset | Sender, e.g. `Yuva <yuva@example.com>`. Required with `YUVA_SMTP_HOST`. |
| `YUVA_SMTP_ALLOW_PRIVATE` | `false` | Lets e-mail channels use SMTP hosts on loopback and private addresses, such as Mailpit. |

## Attachments and storage

| Variable | Default | Meaning |
|---|---|---|
| `YUVA_STORAGE` | `local` | `local` (a directory) or `s3` (S3, R2, MinIO, …). |
| `YUVA_STORAGE_DIR` | `data/attachments` | Directory for `local`; `/data/attachments` in the Docker image. |
| `YUVA_S3_ENDPOINT` | unset | Endpoint URL, e.g. `https://s3.eu-central-1.amazonaws.com`. |
| `YUVA_S3_REGION` | `auto` | Region. |
| `YUVA_S3_BUCKET` | unset | Bucket. |
| `YUVA_S3_ACCESS_KEY_ID` | unset | Access key id. |
| `YUVA_S3_SECRET_ACCESS_KEY` | unset | Secret access key. |
| `YUVA_S3_PATH_STYLE` | `false` | Path-style URLs, which MinIO usually needs. |
| `YUVA_ATTACHMENT_MAX_BYTES` | `26214400` (25 MiB) | Largest attachment in bytes. |
| `YUVA_ATTACHMENT_TYPES` | see below | Allowed content types; `image/*` style wildcards work. |

Default types: `image/png,image/jpeg,image/gif,image/webp,image/heic,application/pdf,text/plain,text/csv,application/zip,application/json,video/mp4,video/quicktime,audio/mpeg,audio/mp4`.

## Inbound e-mail

| Variable | Default | Meaning |
|---|---|---|
| `YUVA_INGRESS_SECRET` | unset | Signs requests to `/ingress/email`; the Email Worker uses the same value. Unset, that endpoint refuses all mail. |
| `YUVA_INGRESS_ACCEPT_V1` | `false` | Also accepts the old `v1` signature, until you redeploy an older Email Worker. |
| `YUVA_INGRESS_AUTHSERV_ID` | unset | Authserv-id of your receiving mail server, e.g. `mx.cloudflare.net`. Needed to use DMARC results. |
| `YUVA_INGRESS_MAX_CONCURRENT` | `8` | Messages `/ingress/email` handles at once per process; more get `503`. |
| `YUVA_SES_TOPIC_ARNS` | unset | SNS topic ARNs whose SES bounces and complaints `/ingress/ses` accepts. |
| `YUVA_EMAIL_SENDER_HOURLY_CAP` | `500` | Inbound e-mails one sender may send to a workspace per hour; more are refused (`429`). |

## Chat

| Variable | Default | Meaning |
|---|---|---|
| `YUVA_CHAT_EMAIL_DELAY` | `5m` | How long a reply stays unread before it is e-mailed to a contact who left the chat. At least `1s`. |
| `YUVA_ANONYMOUS_CONTACTS_PER_HOUR` | `20` | New anonymous visitors per IP address (IPv6 /64), channel and hour; more get `429`. |

## Webhooks

| Variable | Default | Meaning |
|---|---|---|
| `YUVA_WEBHOOK_ALLOW_PRIVATE` | `false` | Lets webhooks reach loopback and private addresses. Development only. |

## Web Push

| Variable | Default | Meaning |
|---|---|---|
| `YUVA_VAPID_PUBLIC_KEY` | unset | VAPID public key. Set both keys or neither; `yuva vapid-keys` prints a pair. |
| `YUVA_VAPID_PRIVATE_KEY` | unset | VAPID private key. |
| `YUVA_VAPID_SUBJECT` | `mailto:` + address of `YUVA_SMTP_FROM`, else `YUVA_PUBLIC_URL` if `https` | Contact for push services: `mailto:…` or `https://…`. |

## Build

The version the server reports (`GET /v1/version`) is set at build time with the Docker build
argument `VERSION`. It is not an environment variable.
