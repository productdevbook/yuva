<p align="center">
  <img src="https://raw.githubusercontent.com/productdevbook/yuva/main/.github/assets/yuva-banner.png" alt="Yuva — one inbox for every product you run" width="760">
</p>

<p align="center">
  <b>One home for every product's conversations.</b><br>
  E-mail, live chat and in-app messages in one open-source inbox that you host yourself.
</p>

<p align="center">
  <a href="https://useyuva.com">Website</a> ·
  <a href="https://github.com/productdevbook/yuva/blob/{{TAG}}/docs/install.md">Install guide</a> ·
  <a href="https://github.com/productdevbook/yuva/blob/{{TAG}}/docs/operations.md">Upgrading</a> ·
  <a href="https://github.com/productdevbook/yuva/blob/{{TAG}}/CHANGELOG.md">Changelog</a> ·
  <a href="https://github.com/productdevbook/yuva/blob/{{TAG}}/SECURITY.md">Security</a>
</p>

{{WARNING}}

## Get Yuva {{VERSION}}

```sh
docker pull {{IMAGE}}:{{VERSION}}
```

| Piece | How to get it |
|---|---|
| Server and team panel | `{{IMAGE}}:{{VERSION}}` for `linux/amd64` and `linux/arm64` — one binary, Postgres is all it needs |
| Web widget | served by your own Yuva at `/yuva.js`, embed `<yuva-chat>` ([guide](https://github.com/productdevbook/yuva/blob/{{TAG}}/docs/widget.md)) |
| iOS | Swift Package `https://github.com/productdevbook/yuva`, version `{{VERSION}}` ([guide](https://github.com/productdevbook/yuva/blob/{{TAG}}/docs/mobile.md)) |
| Android | `implementation("com.github.productdevbook:yuva:{{TAG}}")` from `https://jitpack.io` ([guide](https://github.com/productdevbook/yuva/blob/{{TAG}}/docs/mobile.md)) |
| Go helpers | `go get github.com/productdevbook/yuva/sdk/go@{{TAG}}` — identity tokens and webhook verification |
| API contract | [`openapi/openapi.yaml`](https://github.com/productdevbook/yuva/blob/{{TAG}}/openapi/openapi.yaml) |

## What's in this release

{{NOTES}}

## Before you upgrade

Back up the database (and attachment storage) first. Migrations run when the new server starts,
and until 1.0 they have no way back. The steps are in the
[operations guide](https://github.com/productdevbook/yuva/blob/{{TAG}}/docs/operations.md).

## Thank you

Yuva is AGPL-3.0 at its core and MIT for everything you embed in your own apps, so it can stay
open and keep going. Issues, ideas and pull requests are welcome — see
[CONTRIBUTING.md](https://github.com/productdevbook/yuva/blob/{{TAG}}/CONTRIBUTING.md).
Found a vulnerability? Please report it privately as described in
[SECURITY.md](https://github.com/productdevbook/yuva/blob/{{TAG}}/SECURITY.md).

{{COMPARE}}
