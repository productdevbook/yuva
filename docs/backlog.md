# Backlog

Every work item, numbered once in build order. `Y-N` is GitHub issue `#N`; IDs are never reused, and
a withdrawn item stays here marked withdrawn. Work before `Y-117` is tracked in the milestone issues
#1–#116 listed in `roadmap.md`.

## M24 — Tidy for 0.1.0

### Y-117 · Roadmap status lines match the code

- **Area** docs · **Owner** orchestrator · **Priority** P0 · **Depends on** – · **Chapters** roadmap
- **Acceptance**
  - M6.1: API, widget, Swift and Kotlin SDKs and panel done; acceptance pending (Y-126).
  - M8: done in `v0.0.1`, without the `CHANGELOG.md` pointer.
  - M17 and M18: done, each with the first tag that carries it.
  - M19: done; the Status lines of M19.3 and M19.6 come before their bullets like the other parts.
  - M13 and M14: the tag that carried them next to the deploy date.
  - M16: a one-line entry saying it was withdrawn (#63 closed as not planned, #72 closed).
  - M10: no version in the heading; listed under Later.

### Y-118 · `architecture.md` matches the code

- **Area** docs · **Owner** orchestrator · **Priority** P1 · **Depends on** – · **Chapters** architecture
- **Acceptance**
  - The panel's Lingui catalogs are the six in `web/lingui.config.ts` (now says three).
  - The mobile SDK summary lists rating, status line, subscriptions, shake feedback and analytics.
  - The OAuth and token passages that name "member apps (M10)" say these apps are planned, not
    present.

### Y-119 · Nothing internal in the repository; `CLAUDE.md` current

- **Area** docs · **Owner** orchestrator · **Priority** P0 · **Depends on** – · **Chapters** –
- **Acceptance**
  - `.claude/agents/yuva-video.md` names no path on our servers or the Mac; it says where to look
    in words (the ops notes, the Mac clone) without the path.
  - `CLAUDE.md` lists `yuva-video` among the agents.
  - The `CLAUDE.md` check table covers what CI runs: govulncheck, the generated-code diff, Helm lint
    and kind install, the `examples/*` builds and the Dovecot migration tests.
  - `git grep -nE '/srv/|/Users/|server1'` outside test fixtures finds nothing.
- **As built**
  - `.claude/agents/yuva-video.md` is excluded in `.git/info/exclude` and was never in the
    repository, so it keeps its paths and stays out of the public agent list.

### Y-120 · Configuration reference covers every server setting

- **Area** docs · **Owner** yuva-docs · **Priority** P1 · **Depends on** – · **Chapters** configuration
- **Acceptance**
  - `YUVA_STATUS_CANARY_URLS` and `YUVA_STATUS_MIN_INTERVAL` are in `docs/configuration.md` and its
    `tr` and `de` copies, with defaults from `api/internal/config/config.go`.
  - Every `YUVA_*` variable read by the server (`YUVA_SMTP_*`, `YUVA_SES_*`, `YUVA_MICROSOFT_*`,
    `YUVA_MAIL_PREVIEW_DIR`) is documented or listed as internal; CLI and test-fixture variables
    stay out.

### Y-121 · Stale pointers in README, examples and the changelog

- **Area** docs · **Owner** yuva-docs · **Priority** P2 · **Depends on** – · **Chapters** –
- **Acceptance**
  - `README.md` points "What changed" at GitHub Releases.
  - `examples/README.md` drops the "0.0.7 or later" constraint.
  - `CHANGELOG.md` stays frozen at 0.0.3 and says so in its first lines.

### Y-122 · Swift SDK German strings

- **Area** sdk · **Owner** yuva-mobile · **Priority** P1 · **Depends on** – · **Chapters** –
- **Acceptance**
  - The 15 keys without `de` in `sdk/swift/Sources/YuvaKit/Resources/Localizable.xcstrings`
    (`conversations.messages`, `feedback.attachments`, `feedback.resolvedNote`, `feedback.state.*`,
    `feedback.view`, `feedback.progressNote.*`) have German, worded like the Kotlin `de` strings.
  - `swift build && swift test` passes on the Mac.

### Y-123 · Unused queries and the empty Go package

- **Area** api · **Owner** yuva-api · **Priority** P2 · **Depends on** – · **Chapters** –
- **Acceptance**
  - The 13 queries no code calls are removed from `api/internal/store/*.sql` and regenerated:
    `CountPublicIdeas`, `FindThreadSentTo`, `GetPersonReaderSettings`, `IsChatVisitor`,
    `ListBroadcastsByIDs`, `ListMigrationFolderSeen`, `LockChangelogEntryByID`,
    `MarkEmailImagesSent`, `SetConversationSpam`, `SetMigrationFolderLabel`, `TouchHelpCollection`,
    `TouchRoadmapBoard`, `TrimAnalyticsDaily`; a query that turns out to be needed is used instead.
  - The empty `sdk/go/health/` directory is gone.
  - The `api/` and `sdk/go/` checks pass.
- **As built**
  - `TrimAnalyticsDaily` stays: the analytics retention job calls it through a function list.

### Y-124 · Unused shadcn primitives in the panel (withdrawn)

- **Area** web · **Owner** yuva-panel · **Priority** – · **Depends on** – · **Chapters** –
- Withdrawn on 2026-10-23: the owner keeps the unused primitives in `web/src/components/ui/`.

### Y-125 · Release stage wording

- **Area** docs · **Owner** yuva-docs · **Priority** P1 · **Depends on** owner decision · **Chapters** –
- **Acceptance**
  - `SECURITY.md`, `site/src/copy/contact.ts` and the README use the stage the owner chose for
    0.1.0 instead of "pre-alpha", in `en`, `tr` and `de`.
  - `SECURITY.md` › Supported versions names the latest 0.1.x.

### Y-133 · Mobile SDK strings in every locale

- **Area** sdk · **Owner** yuva-mobile · **Priority** P1 · **Depends on** Y-122 · **Chapters** –
- **Acceptance**
  - Every key in `sdk/swift/Sources/YuvaKit/Resources/Localizable.xcstrings` has a value in all 47
    locales (now 15 keys missing in each of 45).
  - No string in `sdk/kotlin/yuva/src/main/res/values-*/strings.xml` repeats the English text unless
    the word is the same in that language (e.g. "Feedback" in German); the Messages tab, feedback
    states, notes and "View feedback" are translated in all 45.
  - Swift and Kotlin use the same wording per locale.
  - The `sdk/swift` and `sdk/kotlin` checks pass on the Mac.

## M25 — Acceptance for 0.1.0

### Y-126 · M6.1 satisfaction ratings on a running server

- **Area** verify · **Owner** yuva-verifier · **Priority** P0 · **Depends on** – · **Chapters** roadmap M6.1
- **Acceptance**
  - The M6.1 Accept: a widget visitor and an e-mail contact each rate a closed conversation; the
    panel shows both; a webhook receiver gets `conversation.rated`; a second rating of the same close
    gets `409`.
  - Screenshots (WebP) of the widget rating card, the e-mail rating page and the panel thread.

### Y-127 · M17 Home and setup on a running server

- **Area** verify · **Owner** yuva-verifier · **Priority** P0 · **Depends on** – · **Chapters** roadmap M17
- **Acceptance**
  - The M17 Accept, including that an agent at `/` sees no inbox or property they may not see.
  - Screenshots of Home for an owner, `/setup` before and after, and `/` for an agent.

### Y-128 · M23 MCP coverage with Claude Code

- **Area** verify · **Owner** yuva-verifier · **Priority** P0 · **Depends on** – · **Chapters** roadmap M23
- **Acceptance**
  - The M23 Accept on a fresh throwaway workspace, test mail to simulator addresses only; the
    workspace is deleted afterwards.

## M26 — Release 0.1.0

### Y-129 · Security review since 0.0.1

- **Area** api · **Owner** yuva-api · **Priority** P0 · **Depends on** Y-123 · **Chapters** architecture
- **Acceptance**
  - Reviewed: OAuth and MCP, scoped keys, reader accounts, status page own domains, list imports
    and consent, e-mail image uploads, migrations' source credentials, analytics ingest.
  - Each finding that affects correctness or security is its own backlog item, fixed before Y-132.

### Y-130 · Version 0.1.0 everywhere

- **Area** release · **Owner** orchestrator · **Priority** P0 · **Depends on** M24, M25, Y-129 · **Chapters** –
- **Acceptance**
  - `0.1.0` in `openapi/openapi.yaml`, `sdk/js/package.json`, `sdk/swift/Sources/YuvaKit/Yuva.swift`,
    `sdk/kotlin/gradle.properties`, `deploy/helm/yuva/Chart.yaml` (version and appVersion),
    `deploy/mcpb/manifest.json`, `server.json`, the site's `VERSION`, `README.md`, `docs/mobile.md`
    and the `docs/install.md` build example.
  - `git grep -n '0\.0\.32'` finds only history (blog posts, release notes).
  - Every check in the `CLAUDE.md` table passes.

### Y-131 · Release notes and the 0.1.0 post

- **Area** docs · **Owner** yuva-docs · **Priority** P1 · **Depends on** Y-130 · **Chapters** –
- **Acceptance**
  - A post on the site for 0.1.0 in `en`, `tr` and `de`: what Yuva does today, how to install, what
    0.1 means for upgrades.
  - The site check passes.

### Y-132 · Tag and publish 0.1.0

- **Area** release · **Owner** orchestrator · **Priority** P0 · **Depends on** Y-130, Y-131 · **Chapters** –
- **Acceptance**
  - `v0.1.0` tagged after the owner's go-ahead; GitHub Release notes from changelogithub.
  - The container image, npm `useyuva`, JitPack, SwiftPM and the MCP registry entry report `0.1.0`.
  - A fresh install from `docs/install.md` on a clean host serves `/healthz` and `/v1/version`
    with `0.1.0`.
