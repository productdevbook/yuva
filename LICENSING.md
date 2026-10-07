# Licensing

Yuva is open source with a split license, so that the server stays open while the pieces you embed
in your own apps carry no copyleft obligations.

| Path | License | Why |
|---|---|---|
| everything not listed below (`api/`, `web/`, `edge/`, `deploy/`, `docs/`) | [AGPL-3.0-only](LICENSE) | The server and agent panel. If you run a modified Yuva as a network service, you must offer its source to its users. |
| `sdk/` (web widget, Swift, Kotlin, Go helpers) | [MIT](sdk/LICENSE) | These ship inside your own websites and apps, including App Store and Play builds. |
| `openapi/` | [MIT](openapi/LICENSE) | Anyone may generate clients from the API contract. |

A `LICENSE` file inside a directory overrides the root license for that directory.

## Commercial license

The copyright holder (productdevbook) may also offer Yuva under a commercial license to
organisations that cannot meet the AGPL's terms, and may run a hosted Yuva service. This is possible
because every contribution is made under the [Contributor License Agreement](CLA.md).

Features that are only available under a commercial license, if there ever are any, will live in a
separate top-level `ee/` directory with its own license file. Nothing outside `ee/` will ever be
moved under a more restrictive license than the one it was published under.

## Name and logo

The AGPL and MIT licenses cover the code, not the Yuva name or logo. See [TRADEMARK.md](TRADEMARK.md).

## Third-party code

Dependencies keep their own licenses. A dependency must be compatible with AGPL-3.0 (for the
server and panel) or MIT (for `sdk/` and `openapi/`). Code copied from another project (fixtures,
test corpora, snippets) keeps its notice and is listed in `THIRD_PARTY_NOTICES.md` next to it.
