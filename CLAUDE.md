# Yuva

Open-source customer messaging: e-mail, live chat and in-app conversations for many products in one
inbox. This repository is public. Design in `docs/architecture.md`, milestones in
`docs/roadmap.md`; both are binding — change them in the same commit when a decision changes.

Work is tracked as GitHub issues on `productdevbook/yuva`, one per milestone plus smaller ones. The
main session orchestrates; the agents in `.claude/agents/` each own one part of the tree.

## Parts and checks

The commands are set up in M0. Until a part exists, its row is the target, not something to run.
Keep this table current when a command changes.

| Part | Owner agent | Check |
|---|---|---|
| `openapi/` | yuva-api | `REDOCLY_TELEMETRY=off npx --yes @redocly/cli@2.54.3 lint --config openapi/redocly.yaml --format=stylish openapi/openapi.yaml` |
| `api/`, `sdk/go` | yuva-api | in `api/`: `go generate ./... && go vet ./... && go test ./... && go build ./...` (tests that need Postgres run when `YUVA_TEST_DATABASE_URL` is set, as in CI, and skip otherwise); in `sdk/go/`: `go vet ./... && go test ./... && go build ./...` |
| `web/` | yuva-panel | in `web/`: `npm run -s extract && npx lingui compile --strict && npm run -s build` |
| `edge/` | yuva-edge | in `edge/`: `bun run check` |
| `sdk/js` | yuva-widget | in `sdk/js/`: `bun run check && bun run build` (reports the gzip size) |
| `sdk/swift` | yuva-mobile | on the Mac, at the root: `swift build && swift test`; in `sdk/swift/Example/`: `xcodebuild -project YuvaExample.xcodeproj -scheme YuvaExample -destination "generic/platform=iOS Simulator" build` |
| `sdk/kotlin` | yuva-mobile | in `sdk/kotlin/` on the Mac (JDK 21 as `JAVA_HOME`, `ANDROID_HOME` set), never on a machine without swap: `./gradlew check :sample:assembleDebug` |
| `docs/`, site | yuva-docs | links resolve; every documented endpoint exists in `openapi/` |
| `site/` | yuva-docs | in `site/`: `bun run i18n && bun run build && bun run links` (Lingui extract and `compile --strict`, Astro static build, link check over `dist/`) |

- OpenAPI is 3.1: no `nullable`. Contract first: change the spec, lint, regenerate, then implement.
- Migrations: `api/internal/store/migrations/000NN_name.sql` (goose). Take the next free number
  only when you start writing it; never renumber one that is pushed.
- Every table has `workspace_id`; every query filters by it. No exception without a written reason
  in `docs/architecture.md`.

## Versions

- One version for the whole repository, SemVer, starting at `0.0.1`. Tags are `v0.0.1`, `v0.0.2`, …;
  there is no `1.0` until the owner says so.
- The server reports it from `-X main.version` (`VERSION` build arg); `openapi/openapi.yaml`
  `info.version`, `sdk/js/package.json` and later SDK versions move with it.
- `/v1` and `/client/v1` in URLs are the API contract version, not the release version.

## Licensing in practice

- `sdk/` and `openapi/` are MIT, everything else AGPL-3.0 (`LICENSING.md`). Never move code from
  an AGPL directory into `sdk/`; write it again there or ask.
- A new dependency must be compatible with the license of the directory it is added to. GPL-only
  dependencies are not allowed anywhere; in `sdk/` only permissive licenses (MIT, BSD, Apache-2.0,
  ISC).
- Copied fixtures or snippets keep their notice and are listed in `THIRD_PARTY_NOTICES.md` in that
  directory.

## Testing

- Test e-mail goes only to simulator addresses (`success@simulator.amazonses.com`), never to a real
  person.
- `deploy/compose.yaml` runs Mailpit, which catches everything e-mail channels send in
  development. Give a channel SMTP host `mailpit`, port `1025`, TLS `none` and no username. Read
  caught mail at `127.0.0.1:58025`: the web UI, `curl -s 127.0.0.1:58025/api/v1/messages`, one
  message's headers or source at `/api/v1/message/<ID>/headers` and `/api/v1/message/<ID>/raw`;
  delete what you created with `DELETE /api/v1/messages` and `{"IDs": [...]}`. Mail into the dev
  server is a POST to `/ingress/email` signed with the compose `YUVA_INGRESS_SECRET` (format in
  `edge/README.md`).
- Keep temporary files in your own subdirectory of the scratch dir; other agents share the root.
- Test data on a shared instance is created in a throwaway workspace and deleted afterwards.
- Screenshots are WebP.
- Tests are written only when the issue asks for them; checks always run.

## Rules

- Code, identifiers, commits, docs and strings in English; Turkish only in the `tr` catalogs.
  No comments except what the code cannot say.
- Nothing internal goes into this repository: no hostnames, IPs or filesystem paths of our own
  servers, no secrets, no customer data. Our own deployment lives outside the repo.
- Several agents share one working tree and one index. Never `git add -A`, `git stash`,
  `git reset` or `git checkout` on files you did not write. Commit with explicit paths:
  `git commit -m "..." -- <your paths>`.
- Push only when `git push` goes through as is; otherwise leave the commit local and say so.
- Commit per finished piece; Conventional Commits; end the message with
  `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- Deletes, data drops and history rewrites only when explicitly asked.
