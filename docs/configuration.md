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
| `YUVA_TRUSTED_PROXIES` | unset | Addresses or CIDR ranges of your proxies, e.g. `10.0.0.0/8`. Then the client IP header is read only from them. The analytics collector reads the visitor's `X-Forwarded-For` hop only from these addresses, taking the rightmost entry that is not one of them; unset, it uses the connecting address. |
| `YUVA_COOKIE_SECURE` | `true` when `YUVA_PUBLIC_URL` is `https` | Marks the session cookie `Secure`. |
| `YUVA_LOCALE_COOKIE_DOMAIN` | unset | A domain the host of `YUVA_PUBLIC_URL` is inside of, e.g. `example.com` for `https://app.example.com` (a leading dot is ignored). The panel then writes a `yuva_locale` cookie on that domain, so a site on the same domain can open in the person's language. Leave it unset for a panel without such a site. The server refuses to start when the domain does not contain the host. |
| `YUVA_MCP` | `on` | `off` turns off the MCP endpoint `/mcp` (it answers `404`). OAuth stays on. |
| `YUVA_PANEL` | `on` | `off` stops serving the panel and OAuth consent; API keys keep working. |
| `YUVA_PANEL_ANALYTICS_KEY` | unset | The public key (`yuva_ak_…`) of a website in [Analytics](analytics.md) on this server whose domain is the panel's. The panel then counts its own screens there, with record IDs replaced by `:id` (`/conversations/:id`), so no conversation or contact ID is sent. |
| `YUVA_WIDGET` | `on` | `off` stops serving `/yuva.js`, `/yuva-chat.js`, `/yuva-docs.js` and `/yuva-analytics.js`; `/client/v1` stays. |

## Sign-in and passkeys

| Variable | Default | Meaning |
|---|---|---|
| `YUVA_WEBAUTHN_RP_ID` | host name of `YUVA_PUBLIC_URL` | Passkey relying party id. Changing it makes existing passkeys unusable. |
| `YUVA_WEBAUTHN_RP_NAME` | `Yuva` | Name browsers show for a new passkey. |
| `YUVA_WEBAUTHN_ORIGINS` | origin of `YUVA_PUBLIC_URL` | Origins allowed for passkeys and the panel's realtime socket. |
| `YUVA_SIGNUP` | `invite` | `invite`: only existing people and invited addresses get a sign-in code; workspaces come from `yuva bootstrap`. `open`: anyone can sign in with a code and create workspaces in the panel. |
| `YUVA_SIGNUP_WORKSPACES_PER_PERSON` | `3` | With `open` sign-up, workspaces one person may have created and not deleted; more get `403`. |
| `YUVA_SIGNUP_WORKSPACES_PER_IP` | `5` | With `open` sign-up, workspaces created from one IP address (IPv6 /64) in 24 hours; more get `429`. |
| `YUVA_OPERATOR_EMAIL` | unset | Addresses, comma-separated, that get an e-mail when a new person signs up with a code (readers marked as such) or creates a workspace: the address, the workspace name, the time and the country (behind Cloudflare, with `YUVA_CLIENT_IP_HEADER=CF-Connecting-IP`) or else the IP address. At most one e-mail every 5 minutes, listing everything since the previous one. In English, through the server's SMTP. It also lists broadcast [approval requests, suspensions and automatic pauses](operations.md#broadcasts-on-a-hosted-server). Unset means off. |

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
| `YUVA_EMAIL_IMAGES_MAX_BYTES` | `524288000` (500 MiB) | Total size of one workspace's [e-mail design](designs.md) images in bytes; `0` is no limit. Uploads beyond it are refused (`409 image_quota`). |

Default types: `image/png,image/jpeg,image/gif,image/webp,image/heic,application/pdf,text/plain,text/csv,application/zip,application/json,video/mp4,video/quicktime,audio/mpeg,audio/mp4`.

These two settings are the server's limits. A channel can narrow them, never widen them: send `attachments` with
`max_bytes` (a size in bytes) and `types` (content types such as `application/pdf`, or `image/*` for a family) on
`POST /v1/inboxes/{inboxId}/channels` or `PATCH /v1/channels/{channelId}`. A size above the server's, or a type the
server does not allow at all, is refused with `400 validation_failed`; `null` makes the channel follow the server
again. What applies is always the smaller size and the types both lists allow (`image/*` under a server that lists
only `image/png` allows `image/png`), so lowering the server's limit later still wins. `attachments` on the channel
shows your own values and the `effective_max_bytes` and `effective_types` in force; no effective type means no file is
accepted. The limits hold for files from your team, for the web widget and mobile SDKs, for feedback screenshots and
for inbound e-mail. [Moving to Yuva](migrate.md) belongs to no channel and keeps the server's limits.

## Inbound e-mail

| Variable | Default | Meaning |
|---|---|---|
| `YUVA_INGRESS_SECRET` | unset | Signs requests to `/ingress/email`; the Email Worker uses the same value. Unset, that endpoint refuses all mail. |
| `YUVA_INGRESS_ACCEPT_V1` | `false` | Also accepts the old `v1` signature, until you redeploy an older Email Worker. |
| `YUVA_INGRESS_ACCEPT_V2` | `true` | Also accepts the old `v2` signature, whose message is read before it is authenticated. Set `false` once every Email Worker signs `v3`; that closes advisory GHSA-jfvx-vc82-56f7. |
| `YUVA_INGRESS_AUTHSERV_ID` | unset | Authserv-id of your receiving mail server, e.g. `mx.cloudflare.net`. Needed to use DMARC results; without it every inbound message is stored with DMARC `unknown` and the server warns at startup. |
| `YUVA_INGRESS_MAX_CONCURRENT` | `8` | Messages `/ingress/email` handles at once per process; more get `503`. |
| `YUVA_SES_TOPIC_ARNS` | unset | SNS topic ARNs whose SES bounces and complaints `/ingress/ses` accepts. |
| `YUVA_SES_REGION` | unset | The Amazon SES region whose [tenants](operations.md#amazon-ses-tenants-on-a-hosted-server) Yuva manages, e.g. `eu-west-1`. Channels that send through SES in another region are not isolated. Set it with the next three, or none. |
| `YUVA_SES_ACCESS_KEY_ID` | unset | Access key of an IAM user with the tenant policy. |
| `YUVA_SES_SECRET_ACCESS_KEY` | unset | Its secret. |
| `YUVA_SES_CONFIGURATION_SET` | unset | The configuration set Yuva associates with every tenant and names in `X-SES-CONFIGURATION-SET`. 1 to 64 letters, digits, `-` or `_`. |
| `YUVA_SES_TENANT_PREFIX` | `yuva-` | Start of every tenant name Yuva creates, up to 32 letters, digits, `-` or `_`. The IAM policy allows only tenants with it. |
| `YUVA_EMAIL_SENDER_HOURLY_CAP` | `500` | Inbound e-mails one sender may send to a workspace per hour; more are refused (`429`). |

## Moving to Yuva

For [moving a mailbox to Yuva](migrate.md). The settings that limit a workspace apply to hosted servers;
a request over one is refused with `422 hosted_limit` naming it.

| Variable | Default | Meaning |
|---|---|---|
| `YUVA_FORWARDING_DOMAIN` | unset | A domain name such as `forward.example.com` whose mail you point at Yuva like the channels' domains (see [Inbound](email.md#inbound)). Every e-mail channel then has a [forwarding address](email.md#forwarding-address) `<token>@` this domain. Unset, there are no forwarding addresses and moving a mailbox switches mail by MX only. |
| `YUVA_MIGRATION_SOURCES` | all built | Comma-separated sources teams may move from: `imap`, `file`, `zendesk`, `helpscout`, `chatwoot`, `front`, `freshdesk`, `intercom`, `crisp`. Mailboxes over IMAP and export files are built so far. Empty turns moving off. |
| `YUVA_MIGRATION_WORKERS` | `2` | Migrations copied at once per process; `0` runs none in this process. |
| `YUVA_MIGRATION_MAX_ACTIVE` | `0` | Most migrations of one workspace that scan, copy or sync at once; `0` is no limit. |
| `YUVA_MIGRATION_MAX_BYTES` | `0` | Most bytes one workspace imports in a month; `0` is no limit. |
| `YUVA_MIGRATION_MAX_MESSAGE_BYTES` | `52428800` (50 MiB) | Largest message imported; larger ones are skipped. |
| `YUVA_MIGRATION_UPLOAD_MAX_BYTES` | `21474836480` (20 GiB) | Largest [export file](migrate-files.md) one migration takes; a larger one is refused with `422 hosted_limit`. `0` turns file imports off. |
| `YUVA_MIGRATION_SYNC_MAX_DAYS` | `30` | Days a migration keeps reading the old mailbox after the copy; then it stops and its credentials are deleted. From 1 to 365. |
| `YUVA_MIGRATION_AUDIT_DAYS` | `365` | Days a migration's activity is kept after it ended. From 1 to 36500. |
| `YUVA_AUDIT_DAYS` | `365` | Days a workspace's [audit log](operations.md#audit-log) entries are kept; the hourly retention job deletes older ones. From 1 to 36500. A workspace's `retention_days` does not affect it. |
| `YUVA_MIGRATION_ALLOW_PRIVATE` | `false` | Also lets a migration connect to mail servers on a private or local network (development, servers of your own network). |
| `YUVA_MICROSOFT_CLIENT_ID`, `YUVA_MICROSOFT_CLIENT_SECRET` | unset | The server's own Microsoft Entra app for "Sign in with Microsoft" ([how to register it](migrate-operators.md#registering-the-microsoft-app)). Set both or neither. Unset, a team registers an app of its own on the Connect step, and Outlook.com and Microsoft 365 are offered only when the server's [`YUVA_PUBLIC_URL`](#server) is an HTTPS address (or `localhost`), which Microsoft accepts as a redirect URL. |
| `YUVA_MICROSOFT_TENANT` | `common` | The authority of the server's Microsoft app: `common` for an app that accepts any work, school or personal account; `organizations`, `consumers`, a tenant ID or a verified domain for an app limited to those. |

## Broadcasts

[Sending broadcasts](broadcasts.md). Lists and their forms work whatever these say.

| Variable | Default | Meaning |
|---|---|---|
| `YUVA_BROADCASTS` | `approval` when `YUVA_SIGNUP=open`, else `on` | `on`: every workspace sends. `approval`: a workspace created through sign-up sends only after the operator approves it, and every workspace is held to a [warm-up limit](broadcasts.md#approval-on-hosted-servers). `off`: creating, changing and sending broadcasts answers `403 broadcasts_disabled`. |
| `YUVA_BROADCAST_WORKERS` | `2` | Broadcast send jobs one process runs at once, each for one sending channel. Support replies have their own queue and are not slowed down. At least 1. |
| `YUVA_BROADCAST_DAILY_RECIPIENTS` | `0` (no limit) | Most recipients one workspace's broadcasts may have per UTC day. A send that reaches it continues the next day. The operator can set a smaller limit per workspace ([Operations](operations.md#broadcasts-on-a-hosted-server)). |
| `YUVA_IMPORT_MAX_ROWS` | `100000` | Most data rows a CSV file may have when [importing subscribers](importing.md). From 1 to 100000; a higher value stops the server from starting. A file over it fails and nothing is added. |
| `YUVA_MAIL_LINK_URL` | `YUVA_PUBLIC_URL` | Origin (scheme and host, no path) that the unsubscribe, confirmation, click and image links in list and broadcast mail point at, e.g. `https://links.example.com`. It must reach this server. That host answers only those links (`/m/`, `/s/`, `/c/`, `/i/`) and `404` for everything else, so the panel and API stay on your main host. |

## Status pages

[Status pages](status-pages.md).

| Variable | Default | Meaning |
|---|---|---|
| `YUVA_STATUS_URL` | unset | Origin that public status pages are served from, such as `https://status.example.com` (no path). It must reach this server and differ from `YUVA_PUBLIC_URL`. Pages are then at `<origin>/{slug}`, and `<YUVA_PUBLIC_URL>/status/{slug}` redirects there. Unset, pages are at `<YUVA_PUBLIC_URL>/status/{slug}`. |
| `YUVA_STATUS_HOME` | `on` | `off` turns off the home page at the root of the `YUVA_STATUS_URL` origin (and its language paths such as `/tr/`, `robots.txt`, `sitemap.xml`); the root then answers a plain `404` page. Own domains are not affected. Needs `YUVA_STATUS_URL`. |
| `YUVA_STATUS_HOME_PAGE` | unset | Slug of a published status page that the home page shows as its live card (state, first five components, 60-day strips). Unset, or while that page is unpublished, the card is left out. Sign-in and create links go to `YUVA_PUBLIC_URL`. |
| `YUVA_STATUS_MONITORS` | `20` | Most monitors one workspace may have; `0` is no limit. Beyond it, creating one answers `403 status_monitor_limit`. |
| `YUVA_STATUS_MIN_INTERVAL` | `60`, or `300` when `YUVA_SIGNUP=open` | Shortest check interval of http and tcp monitors, in seconds (`60` to `86400`). The server refuses to start with a value outside that range. |
| `YUVA_STATUS_CANARY_URLS` | the built-in two (`https://www.google.com/generate_204`, `https://www.cloudflare.com/cdn-cgi/trace`) | Comma-separated http or https URLs the server requests to tell its own network trouble from its customers'. When every canary fails right after one answered, the failed checks of that round are dropped instead of counted as downtime. Set URLs this server can reach, or `off` to turn the guard off. |
| `YUVA_STATUS_NOTICES` | `on` | `off` hides the subscribe form (it answers `404`) and sends no notices. |
| `YUVA_STATUS_NOTICES_DAILY` | `5000` when `YUVA_SIGNUP=open`, else `0` | Most notice mails one workspace sends per UTC day; `0` is no limit. What is over waits for the next day. |
| `YUVA_STATUS_MAX_SUBSCRIBERS` | `10000` when `YUVA_SIGNUP=open`, else `0` | Most subscribed addresses one page takes; `0` is no limit. At the cap the form says the page takes no more subscribers. |
| `YUVA_STATUS_DOMAINS` | `ask` | Own domains of status pages, help centers, roadmap boards and changelogs (`status.example.com`, `help.example.com`). `ask`: your TLS proxy issues certificates on demand and asks `GET /status-domains/check?domain=` first (`200` for a verified domain, `404` otherwise); a Caddy example is in `deploy/caddy/Caddyfile`. `cloudflare`: the server registers each verified domain as a Cloudflare for SaaS custom hostname (HTTP validation) and deletes it when the domain is removed or turned off; needs `YUVA_CLOUDFLARE_ZONE_ID` and `YUVA_CLOUDFLARE_API_TOKEN`. `off`: no own domains. |
| `YUVA_STATUS_CNAME` | host of `YUVA_PUBLIC_URL` | The host own domains point their CNAME at. In `cloudflare` mode, a host of the zone that leads to its fallback origin. |
| `YUVA_STATUS_DOMAIN_LIMIT` | `1` when `YUVA_SIGNUP=open`, else `0` | Most status pages, help centers, roadmap boards and changelogs of one workspace, counted together, with an own domain (not turned off); `0` is no limit. Beyond it, setting one answers `403 status_domain_limit`. |
| `YUVA_CLOUDFLARE_ZONE_ID` | unset | The Cloudflare zone whose custom hostnames hold own domains, for `YUVA_STATUS_DOMAINS=cloudflare`. |
| `YUVA_CLOUDFLARE_API_TOKEN` | unset | A Cloudflare API token with the zone's *SSL and Certificates: Edit* permission, for `YUVA_STATUS_DOMAINS=cloudflare`. |

## Help center

[Help centers](help-center.md).

| Variable | Default | Meaning |
|---|---|---|
| `YUVA_HELP` | `on` | `off` removes help centers: their endpoints and public pages answer `404`, and the widget shows no Help app. |
| `YUVA_HELP_URL` | unset | Origin that public help centers are served from, such as `https://help.example.com` (scheme and host, no path). It must reach this server and differ from `YUVA_PUBLIC_URL`. Centers are then at `<origin>/{slug}`. Unset, they are at `<YUVA_PUBLIC_URL>/help/{slug}`. |
| `YUVA_HELP_MAX_ARTICLES` | `500` when `YUVA_SIGNUP=open`, else `0` | Most help articles one workspace may have; `0` is no limit. Beyond it, creating one answers `422 hosted_limit`. |

## Roadmap board

[Roadmap boards](roadmap-board.md).

| Variable | Default | Meaning |
|---|---|---|
| `YUVA_ROADMAP` | `on` | `off` removes roadmap boards: their endpoints and public pages answer `404`, and the widget shows no Roadmap app. |
| `YUVA_ROADMAP_MAX_IDEAS` | `2000` when `YUVA_SIGNUP=open`, else `0` | Most ideas one workspace may have; `0` is no limit. Beyond it, starting one answers `422 hosted_limit`. |

## What's new

[What's new changelogs](whats-new.md).

| Variable | Default | Meaning |
|---|---|---|
| `YUVA_WHATS_NEW` | `on` | `off` removes What's new: its endpoints and public pages answer `404`, and the widget shows no What's new app. |
| `YUVA_CHANGELOG_MAX_ENTRIES` | `1000` when `YUVA_SIGNUP=open`, else `0` | Most What's new entries one workspace may have; `0` is no limit. Beyond it, creating one answers `422 hosted_limit`. |

## Analytics

[Counting your websites](analytics.md).

| Variable | Default | Meaning |
|---|---|---|
| `YUVA_ANALYTICS` | `on` | `off` removes analytics: its endpoints, the collection endpoint and `/yuva-analytics.js` answer `404`. |
| `YUVA_ANALYTICS_BUFFER` | `50000` | Events one process holds before writing them (they are written every second). A full buffer answers `503`. At least 100. |
| `YUVA_ANALYTICS_VISIT_CACHE` | `200000` | Visits one process keeps in memory to tell new visits from running ones. At least 1000. |
| `YUVA_ANALYTICS_SALT_GRACE` | `2h` | How long a day's salt survives after the day ends, for late events. Up to `6h`. |
| `YUVA_ANALYTICS_MONTHLY_EVENTS` | `1000000` | Events one workspace may count per UTC month; more are dropped until the month ends. `0` is no limit, and the server warns at startup when it is set.  Per workspace, [`yuva workspace analytics-limit`](operations.md#operator-commands) overrides it. |
| `YUVA_ANALYTICS_PROPERTY_EVENTS_PER_MINUTE` | `60000` | Events one website may send per minute; more are dropped. |
| `YUVA_ANALYTICS_PROPERTIES_PER_WORKSPACE` | `50` | Websites one workspace may have; more are refused (`403 property_limit`). |
| `YUVA_ANALYTICS_RAW_DAYS` | `90` | Days the detailed visits and events are kept, from 7 to 400. Older days keep only their daily totals. |
| `YUVA_ANALYTICS_WORKERS` | `2` | Analytics jobs (daily totals, cleanup) one process runs at once; `0` runs none in this process. |
| `YUVA_ANALYTICS_ROLLUP_TOP` | `1000` | Values per breakdown kept in a day's totals; the rest is added up as `(other)`. From 10 to 100000. |
| `YUVA_GEOIP` | `dbip` | Where places come from. `dbip`: the free DB-IP Lite city database, downloaded by the server every month. `file`: an MMDB city database of your own. `off`: no location database. |
| `YUVA_GEOIP_DIR` | `data/geoip` | Directory for the downloaded `dbip` database. Put it on a volume that survives restarts, e.g. `/data/geoip` in the compose setup. |
| `YUVA_GEOIP_FILE` | unset | Path of the MMDB file, required with `YUVA_GEOIP=file`. |

With `dbip`, the database is licensed [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/) and your
server must credit it wherever locations are shown. The panel does this ("IP Geolocation by DB-IP",
linking to `https://db-ip.com`); if you show the numbers somewhere else, say it there too. The text and
link are in `geoip.attribution` and `geoip.attribution_url` of `GET /v1/analytics/properties/{propertyId}/health`.

The commands, run as in [Operator commands](operations.md#operator-commands):

| Command | What it does |
|---|---|
| `analytics property create --workspace <ws> --name <name> --domain <host>... [--inbox <id\|slug>] [--platform web\|ios\|android]... [--timezone <zone>]` | Creates a website and prints its id and public key, separated by a tab. |
| `analytics property list --workspace <ws>` | Lists websites with their keys, domains and visitors today, and the month's events against the limit. |
| `analytics geoip-update` | Downloads this month's DB-IP Lite database into `YUVA_GEOIP_DIR` unless it is there, and prints the credit to give. With `file`, checks your file. |
| `workspace analytics-limit --workspace <ws> <events\|0>` | Sets the workspace's events per month; `0` removes the limit. |

## Chat

| Variable | Default | Meaning |
|---|---|---|
| `YUVA_CHAT_EMAIL_DELAY` | `5m` | How long a reply stays unread before it is e-mailed to a contact who left the chat. At least `1s`. |
| `YUVA_DISCUSSION_EMAIL_DELAY` | `10m` | At most one e-mail per follower and discussion thread in this time, listing the new replies and the accepted answer. At least `1s`. |
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

## Tracing

Tracing is off unless an endpoint is set. Yuva adds no variables of its own: it reads the standard
[OpenTelemetry](https://opentelemetry.io/docs/specs/otel/configuration/sdk-environment-variables/)
ones. A value the server cannot use stops it at start with a message that begins `OpenTelemetry:`. See
[Operations](operations.md#tracing).

| Variable | Default | Meaning |
|---|---|---|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | unset | OTLP collector URL, for example `http://otel-collector:4318`. Setting it turns tracing on. |
| `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | unset | Collector URL for traces only, used as given. Setting it also turns tracing on. |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | `http/protobuf` | `http/protobuf` or `grpc` (usually port `4317`). `OTEL_EXPORTER_OTLP_TRACES_PROTOCOL` overrides it for traces. |
| `OTEL_EXPORTER_OTLP_HEADERS` | unset | Headers sent to the collector, `key=value` pairs separated by commas, for example an API key. |
| `OTEL_SERVICE_NAME` | `yuva` | Service name on every span. |
| `OTEL_RESOURCE_ATTRIBUTES` | unset | Extra attributes on every span, for example `deployment.environment=prod`. |
| `OTEL_TRACES_SAMPLER` | every request is traced | Sampler, for example `parentbased_traceidratio`. Without it the server ignores the sampled flag of an incoming `traceparent`. |
| `OTEL_TRACES_SAMPLER_ARG` | unset | Argument of the sampler, for example `0.1`. |
| `OTEL_SDK_DISABLED` | `false` | `true` keeps tracing off although an endpoint is set. `OTEL_TRACES_EXPORTER=none` does the same. |

The other `OTEL_EXPORTER_OTLP_*` variables (timeout, compression, certificates) and `OTEL_BSP_*` are read by the
OpenTelemetry libraries as their specification defines.

## Build

The version the server reports (`GET /v1/version`) is set at build time with the Docker build
argument `VERSION`. It is not an environment variable.

`GET /v1/version` needs no sign-in. It reveals only the server version and whether sign-up is open (`signup`).
