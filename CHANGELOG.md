# Changelog

All notable changes to Yuva are listed here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/). Until 1.0, any release may change the API and the
database schema.

## [Unreleased]

### Added

- **Workspace deletion**: owners delete a workspace in the panel (Settings → Workspace → Danger
  zone) or with `DELETE /v1/workspace`; it stops working at once and a background job deletes its
  data and stored files in batches. Operators use `yuva workspace delete`.
- **Account deletion**: members delete their account in the panel (Settings → My profile) or with
  `DELETE /v1/me`, refused while they are a workspace's only owner; their messages stay, shown as
  from a deleted member. Operators use `yuva person delete`.
- **Panel**: replies from a visitor's typed, unconfirmed address are marked "Unverified sender".

### Changed

- A person who belongs to no workspace can sign in, sees that, and can delete their account.

## [0.0.1] - 2026-10-07

The first public release. Pre-alpha: do not put real customer data in it yet.

### Added

- **Server**: one Go binary with Postgres, applying its own migrations on start; River jobs in the
  same process; attachments and raw e-mails on local disk or S3-compatible storage; `/healthz`,
  `/readyz` and Prometheus metrics. Docker image for `linux/amd64` and `linux/arm64`.
- **Workspaces and members**: owners, admins and agents with per-inbox access; sign-in with
  e-mailed codes and passkeys; invites; workspace API keys; usage counters.
- **Inboxes, contacts and conversations**: per-inbox branding, language, time zone, business hours
  and `live` or `async` mode; messages, notes, attachments, assignment, status, priority, labels,
  canned replies, events and the `/v1` REST API for all of them, described in
  `openapi/openapi.yaml`.
- **Agent panel**: conversation list with filters and search, thread view, composer, contact
  sidebar, settings, realtime updates over WebSocket, English and Turkish; installable as a PWA.
- **E-mail channel**: signed `/ingress/email` fed by a Cloudflare Email Worker (`edge/`, with a
  fallback address when the server is unreachable) or by any MTA through `yuva ingest-email`;
  threading, quote stripping, HTML sanitizing, attachments and raw message storage; outbound SMTP
  per channel with thread headers; loop protection; bounce and complaint handling, including
  Amazon SES notifications.
- **Catch-all e-mail channels**: `*@example.com` receives every address of a domain that no other
  channel has, and replies go out from the address the contact wrote to.
- **Volume without bounces**: a sender opens at most 20 new conversations per channel per hour;
  further mail joins their latest conversation. Only senders above `YUVA_EMAIL_SENDER_HOURLY_CAP`
  are refused.
- **Web widget**: the `<yuva-chat>` web component for live chat and embedded threads, on the
  `/client/v1` API with contact sessions, anonymous visitors, identity tokens signed by the host
  backend, allowed origins, presence, typing, read receipts and e-mail continuity for unread
  replies.
- **Mobile SDKs**: `sdk/swift` (YuvaKit) and `sdk/kotlin` with a client, conversation list, thread,
  composer and feedback form.
- **Feedback and webhooks**: feedback conversations with metadata, posting by API key, Standard
  Webhooks delivery with retries and a delivery log, contact deletion by external id.
- **`sdk/go`**: identity token signing and webhook verification for host backends.
- **Member notifications**: Web Push with per-member preferences and e-mail fallback.
- **Retention**: an optional per-workspace period after which closed conversations and raw e-mails
  are deleted.
- **Operator commands**: `yuva bootstrap`, `migrate`, `api-key`, `inbox`, `channel`, `vapid-keys`.

### Security

- Hardening of sign-in, sessions, client sessions and identity tokens, inbound mail and outbound
  connections, attachment handling, rate limiting behind proxies, and the panel's security
  headers. Report vulnerabilities as described in `SECURITY.md`.

[Unreleased]: https://github.com/productdevbook/yuva/compare/v0.0.1...HEAD
[0.0.1]: https://github.com/productdevbook/yuva/releases/tag/v0.0.1
