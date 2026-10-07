# Yuva

Open-source customer messaging for teams that run many products: one inbox for e-mail, live chat
and private in-app conversations.

Website: https://useyuva.com

> [!WARNING]
> **Pre-alpha.** Yuva is being designed and built in the open; 0.0.1 is its first public release.
> The API and database schema will change without migration paths, and nothing has had a security
> review. Do not put real customer data in it.

## What it does

- **One inbox for every product.** Each product is an inbox with its own branding, language,
  business hours, mode and team; members see the inboxes they were given.
- **E-mail** with threading, quote stripping, attachments, loop protection and bounce handling.
  Sends through any SMTP provider per channel; receives through a Cloudflare Email Worker or any
  MTA.
- **Live chat** on websites, and **embedded threads** inside your own panels, from one web
  component, `<yuva-chat>`.
- **In-app messaging and feedback** in iOS and Android apps through native Swift and Kotlin SDKs.
- **Live or async per inbox**: presence, typing and read receipts where you promise live support,
  an expected reply time where you don't; unread replies follow the user by e-mail.
- **Your users, your identity**: your backend signs a short-lived token; Yuva never sees your user
  database.
- **Signed webhooks** (Standard Webhooks) so your backend sends push notifications with the keys it
  already has.
- **A panel for the team**: conversation list and search, assignment, labels, notes, canned
  replies, sign-in with e-mailed codes and passkeys, installable with Web Push notifications.
- **Simple to run**: one Go binary and Postgres. No Redis, no separate workers.

## Quick start

```sh
git clone https://github.com/productdevbook/yuva.git && cd yuva
docker build -f deploy/Dockerfile --build-arg VERSION=0.0.1 -t yuva:0.0.1 .
```

Then follow [docs/install.md](docs/install.md): a Compose file with Postgres, a `.env` with your
public URL and a master key, `docker compose up -d`, and the first owner with
`yuva bootstrap --email you@example.com --workspace "Example"`.

## Documentation

- [Install](docs/install.md): requirements, Docker, reverse proxy, first owner, keys
- [Configuration](docs/configuration.md): every environment variable
- [E-mail](docs/email.md): inbound, outbound SMTP, bounces, DNS
- [Web widget](docs/widget.md): live chat and embedded threads
- [Mobile SDKs](docs/mobile.md): iOS and Android, push notifications
- [Identity tokens](docs/identity.md): signed-in users
- [Webhooks](docs/webhooks.md): events and signature verification
- [Operations](docs/operations.md): upgrades, backups, monitoring, operator commands
- API contract: [openapi/openapi.yaml](openapi/openapi.yaml)

Design: [docs/architecture.md](docs/architecture.md). Plan: [docs/roadmap.md](docs/roadmap.md).

## Repository

| Path | What | License |
|---|---|---|
| `api/` | Go server: API, realtime, e-mail, jobs | AGPL-3.0 |
| `web/` | Agent panel (React) | AGPL-3.0 |
| `edge/` | Cloudflare Email Worker for inbound mail | AGPL-3.0 |
| `deploy/` | Docker and Compose | AGPL-3.0 |
| `openapi/` | API contract | MIT |
| `sdk/js` | `<yuva-chat>` web component and client | MIT |
| `sdk/swift`, `sdk/kotlin` | iOS and Android SDKs | MIT |
| `sdk/go` | Identity tokens and webhook verification for host backends | MIT |

## License

The server and panel are [AGPL-3.0](LICENSE); the SDKs and the API contract are [MIT](sdk/LICENSE),
so embedding them in your apps carries no copyleft obligations. Details in
[LICENSING.md](LICENSING.md). Contributions require the [CLA](CLA.md). The name and logo are covered
by [TRADEMARK.md](TRADEMARK.md).

Security issues: [SECURITY.md](SECURITY.md).
