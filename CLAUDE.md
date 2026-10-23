# Yuva

Open-source customer messaging: e-mail, live chat and in-app conversations for many products in one
inbox. This repository is public. Design in `docs/architecture.md`, milestones in
`docs/roadmap.md`; both are binding — change them in the same commit when a decision changes.

Work is tracked as GitHub issues on `productdevbook/yuva`, one per milestone plus smaller ones. The
main session orchestrates; the agents in `.claude/agents/` each own one part of the tree.

Each agent definition sets its own `model` and `effort`, so an agent never inherits the main
session's level: architects run Opus at `high`, the code owners Opus at `medium`, docs, edge and the
verifier Sonnet at `medium`. Pass a higher `effort` with a task only when that task is hard
(security, data migrations, concurrency, a bug nobody could find), a lower one for mechanical work.
Agents cannot start other agents (`disallowedTools: Agent`): fan-out multiplies cost and its
results die with a stopped parent; the orchestrator alone runs agents, one area at a time.

## Parts and checks

The commands are set up in M0. Until a part exists, its row is the target, not something to run.
Keep this table current when a command changes.

| Part | Owner agent | Check |
|---|---|---|
| `openapi/` | yuva-api | `REDOCLY_TELEMETRY=off npx --yes @redocly/cli@2.54.3 lint --config openapi/redocly.yaml --format=stylish openapi/openapi.yaml` |
| `api/`, `sdk/go` | yuva-api | in `api/`: `go generate ./... && go vet ./... && go test ./... && go build ./...` (tests that need Postgres run when `YUVA_TEST_DATABASE_URL` is set, as in CI, and skip otherwise); in `sdk/go/`: `go generate ./... && go vet ./... && go test ./... && go build ./...`; then in both `go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...`, and `git diff --exit-code -- api sdk/go` (generated code committed). The IMAP migration tests run against the Dovecot fixture when `YUVA_TEST_IMAP_URL` and `YUVA_TEST_IMAP_FIXTURES` are set, as in CI, and skip otherwise |
| `web/` | yuva-panel | in `web/`: `npm run -s format:check && npm run -s lint && npm run -s extract && npx lingui compile --strict && npm run -s build` |
| `edge/` | yuva-edge | in `edge/`: `bun run check` (oxfmt check, oxlint, tsc, tests) |
| `sdk/js` | yuva-widget | in `sdk/js/`: `bun run check && bun run build` (`check` runs oxfmt check, oxlint, Lingui compile, tsc and tests; `build` reports the gzip size) |
| `sdk/shadcn` | yuva-widget | in `sdk/shadcn/`: `bun run check` (oxfmt, oxlint, `shadcn build`); the site serves the registry at `/r/*.json` from it; `examples/shadcn-next`: `npx next build`; `examples/shadcn-vite`: `bun install && bun run build` (type-checks, then builds) |
| `sdk/swift` | yuva-mobile | on the Mac, at the root: `swift build && swift test`; in `sdk/swift/Example/`: `xcodebuild -project YuvaExample.xcodeproj -scheme YuvaExample -destination "generic/platform=iOS Simulator" build` |
| `sdk/kotlin` | yuva-mobile | in `sdk/kotlin/` on the Mac (JDK 21 as `JAVA_HOME`, `ANDROID_HOME` set), never on a machine without swap: `./gradlew check :sample:assembleDebug` |
| `examples/` | yuva-widget | after `bun run build` in `sdk/js/`: `npm ci && npm run -s build` in `nuxt-content` and `shadcn-next`, `npm run -s types:check && npm run -s build` in `fumadocs`, `npm run -s typecheck` in `headless-chat` and `inbox-feed` |
| `deploy/helm` | yuva-api | `Chart.yaml` `version` and `appVersion` equal `openapi/openapi.yaml` `info.version`; `helm lint --strict` and `helm template` with each `deploy/helm/yuva/ci/*-values.yaml`; CI also installs it into kind and waits for `/readyz` |
| `docs/`, site | yuva-docs | links resolve; every documented endpoint exists in `openapi/` |
| `site/` | yuva-docs | in `site/`: `bun run format:check && bun run lint && bun run i18n && bun run build && bun run links` (oxfmt check, oxlint, Lingui extract and `compile --strict`, the shadcn registry from `sdk/shadcn` into `public/r`, Astro static build, link check over `dist/`) |

- TypeScript and JavaScript are formatted by oxfmt and linted by oxlint with the root `.oxfmtrc.json` and
  `.oxlintrc.json` (no semicolons, double quotes); `format` in a part rewrites its files.
- OpenAPI is 3.1: no `nullable`. Contract first: change the spec, lint, regenerate, then implement.
- Migrations: `api/internal/store/migrations/000NN_name.sql` (goose). Take the next free number
  only when you start writing it; never renumber one that is pushed.
- Every table has `workspace_id`; every query filters by it. No exception without a written reason
  in `docs/architecture.md`.

## Domain architects

Three agents own the design of a whole area across the parts above. They plan and review; the part
owners implement from their plan, and the orchestrator writes their decisions into
`docs/architecture.md` and `docs/roadmap.md`.

| Area | Architect agent |
|---|---|
| Broadcasts: mailing lists and consent, bulk sending and deliverability, the e-mail designer and renderer, the template gallery | yuva-broadcasts |
| Migrations: importing mail and help-desk history from Gmail, Outlook, Yandex, any IMAP server and other help desks, with read-only access to the source | yuva-migration |
| Analytics: cookieless web and app analytics with the web script, Swift and Kotlin SDKs, dashboards and MCP tools | yuva-analytics |

## Versions

- One version for the whole repository, SemVer, starting at `0.0.1`. Tags are `v0.0.1`, `v0.0.2`, …;
  there is no `1.0` until the owner says so.
- The server reports it from `-X main.version` (`VERSION` build arg); `openapi/openapi.yaml`
  `info.version`, `sdk/js/package.json` and later SDK versions move with it.
- `/v1` and `/client/v1` in URLs are the API contract version, not the release version.
- Release notes are generated by changelogithub from the `feat`, `fix` and `perf` commit subjects
  since the previous tag, so write those subjects for the people who read the release.

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
- How broadcast mail looks and lands is checked without anyone's personal mailbox: every design is
  rendered and checked against caniemail data (Mailpit's html-check) and screenshotted in WebKit (Apple
  Mail's engine, on the Mac) and Chromium at desktop, phone and dark; delivery is checked with the
  deliverability probe on one of our own sending domains behind the edge (SPF, DKIM, DMARC, signed list
  headers). Nobody creates provider test accounts or seed mailboxes (Gmail, Yandex, Outlook, iCloud):
  migrations are checked on the Dovecot fixture, help desks against recorded API answers. A paid
  rendering service is used only on the owner's go-ahead.
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

- Code, identifiers, commits and strings in English; Turkish and German only in the `tr` and `de`
  catalogs. User guides are written in English in `docs/` and translated in `docs/tr/` and
  `docs/de/`; a change to an English guide updates both translations in the same commit.
  `architecture.md` and `roadmap.md` stay English only.
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

## Orchestration and cost

- The orchestrator plans, writes briefs and reviews; agents write the code. Agents run on Sonnet
  at `medium`; `yuva-migration` keeps its own model because it touches customers' mailboxes. The
  orchestrator passes `model: opus` for design, architecture, the contract, money or concurrency,
  and a bug whose cause is unclear; a bug fix in existing code gets `effort: high`.
- Agents: `yuva-api` (`openapi/`, `api/`, `sdk/go`), `yuva-panel` (`web/`), `yuva-widget`
  (`sdk/js`, `sdk/shadcn`), `yuva-mobile` (`sdk/swift`, `sdk/kotlin`), `yuva-edge` (`edge/`),
  `yuva-docs` (`docs/`, README); architects `yuva-analytics`, `yuva-broadcasts`, `yuva-migration`
  plan; `yuva-verifier` checks the running product.
- A brief names the goal, the files, the check to run, what is out of scope, and asks for a final
  message of at most 300 words; long output goes to a file under /tmp and is referenced by path.
- Agents run in parallel only on independent work (e.g. SDKs after the contract is live);
  dependent steps of one feature run in order.
- Changes to the contract, migrations, permissions or anything that touches customer mail are
  reviewed in a fresh context (`/code-review`) before commit; only findings that affect
  correctness are fixed.
- One feature per session: a large piece of work gets a spec file and is built in a fresh session.

# Compact instructions

Keep the list of changed files, open bugs, the checks that ran and their results, and the decisions
taken; drop tool output.
