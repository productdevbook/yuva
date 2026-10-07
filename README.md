# Yuva

Open-source customer messaging for teams that run many products: one inbox for e-mail, live chat
and private in-app conversations.

> [!WARNING]
> **Pre-alpha. Not usable yet.** Yuva is being designed and built in the open. There is no release,
> the API and database schema will change without migration paths, and nothing has had a security
> review. Do not put real customer data in it. Watch the repository for the first release.

## What it will do

- **One inbox for every product.** Each product is an inbox with its own branding, languages,
  business hours and team.
- **E-mail** with proper threading, quote stripping, attachments, loop protection and bounce
  handling. Works with any SMTP provider; inbound through a Cloudflare Email Worker or any MTA.
- **Live chat** on websites, and **embedded threads** inside your own panels, from one web component.
- **In-app messaging and feedback** in iOS and Android apps through native Swift and Kotlin SDKs.
- **Live or async per inbox**: presence and typing where you promise live support, an expected
  reply time where you don't.
- **Your users, your identity**: your backend signs a short-lived token; Yuva never sees your user
  database.
- **Signed webhooks** (Standard Webhooks) so your backend sends push notifications with the keys it
  already has.
- **Simple to run**: one Go binary and Postgres. No Redis, no separate workers.

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
