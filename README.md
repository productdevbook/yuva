<div align="center">

<img src=".github/assets/yuva-logo.svg" width="88" height="88" alt="Yuva logo">

# Yuva

**One inbox for every product you run.**

[Website](https://useyuva.com) · [Docs](docs/install.md) · [API](openapi/openapi.yaml) ·
[Releases](https://github.com/productdevbook/yuva/releases) · [Roadmap](docs/roadmap.md)

[![Release](https://img.shields.io/github/v/release/productdevbook/yuva?include_prereleases&label=release&color=d4431c)](https://github.com/productdevbook/yuva/releases)
[![CI](https://github.com/productdevbook/yuva/actions/workflows/ci.yml/badge.svg)](https://github.com/productdevbook/yuva/actions/workflows/ci.yml)
[![License: AGPL-3.0](https://img.shields.io/badge/server-AGPL--3.0-0b0b0f)](LICENSE)
[![SDKs: MIT](https://img.shields.io/badge/SDKs-MIT-0b0b0f)](sdk/LICENSE)

</div>

Yuva is an open-source support inbox. Support e-mail, live chat and in-app messages from all your
products land in one shared inbox. You host it yourself: one Docker image and Postgres.

<picture>
  <source media="(prefers-color-scheme: dark)" srcset=".github/assets/panel-dark.webp">
  <img src=".github/assets/panel-light.webp" alt="The Yuva panel: conversations from three products in one list, and an e-mail conversation open as a chat, with a teammate's note and a suggested reply.">
</picture>

> [!WARNING]
> **Pre-alpha.** The API and database schema can change without migration paths, and the code has
> not had an independent security audit. Try it with test data, not real customer conversations.

## What you get

- **One inbox per product**, each with its own branding, language, hours and team.
- **E-mail, live chat and in-app messages** in one conversation: the `<yuva-chat>` web component,
  iOS and Android SDKs, and an API.
- **Your users stay yours.** Your backend signs identity tokens; signed webhooks tell it what happened.
- **Build on it** with API keys, bots, an event feed and an MCP server for AI assistants.

## Quick start

1. Write `compose.yaml` and `.env` from the [install guide](docs/install.md#quick-start-with-compose).
2. Start it:

   ```sh
   docker compose up -d
   ```

3. Create the first owner:

   ```sh
   docker compose exec yuva /yuva bootstrap --email you@example.com --workspace "Example"
   ```

4. Put it behind HTTPS and sign in with the code that arrives by e-mail.

## Documentation

| Get started | Channels | Integrate | Build on it |
|---|---|---|---|
| [Install](docs/install.md) | [E-mail](docs/email.md) | [Identity tokens](docs/identity.md) | [Headless](docs/headless.md) |
| [Configuration](docs/configuration.md) | [Web widget](docs/widget.md) | [Webhooks](docs/webhooks.md) | [AI assistants (MCP)](docs/mcp.md) |
| [Operations](docs/operations.md) | [Mobile SDKs](docs/mobile.md) | [API contract](openapi/openapi.yaml) | [Examples](examples) |
| | [Documentation pages](docs/documentation-pages.md) | | |

How it is built: [architecture](docs/architecture.md). What changed: [changelog](CHANGELOG.md).

## Contributing

Read [CONTRIBUTING.md](CONTRIBUTING.md) first; contributions are made under the [CLA](CLA.md).
Report security issues privately as described in [SECURITY.md](SECURITY.md).

## License

The server and panel are [AGPL-3.0](LICENSE). The SDKs (`sdk/`) and the API contract (`openapi/`)
are [MIT](sdk/LICENSE), so you can ship them in closed-source apps. Details in
[LICENSING.md](LICENSING.md); name and logo in [TRADEMARK.md](TRADEMARK.md).

<div align="center">
<sub>“Yuva” is Turkish for nest, or home.</sub>
</div>
