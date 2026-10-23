# GitHub and Linear issues

Connect GitHub or Linear to a workspace, and a feedback or a [roadmap idea](roadmap-board.md) can
become an issue where your team plans its work. The issue's state then tells the customer where their
feedback stands, without anyone setting it by hand.

## 1. Connect a tracker

Owners and admins connect trackers; every member sees which are connected. Nobody registers an app.

**GitHub.** Create a [fine-grained personal access
token](https://github.com/settings/personal-access-tokens/new) limited to the repositories issues should
go to, with the repository permission Issues: Read and write. On a GitHub Enterprise Server, also give
its API address, `https://<host>/api/v3`; it must be reachable on a public address.

**Linear.** Create a personal API key under Settings › Account › Security & access.

Then connect it with the API (the panel screen follows):

```sh
curl -X POST "$YUVA_URL/v1/issue-integrations" -H "Authorization: Bearer $YUVA_KEY" \
  -H "Content-Type: application/json" \
  -d '{"provider": "github", "token": "github_pat_…", "default_target": "acme/app"}'
```

| Field | Meaning |
|---|---|
| `provider` | `github` or `linear`. |
| `token` | The token or key. Yuva checks it with the tracker first (`422 tracker_auth_failed` when refused), stores it encrypted under the server's master key and never shows it again. |
| `name` | Shown to your team; defaults to the provider and the account. |
| `api_url` | GitHub Enterprise Server only. |
| `default_target` | Where new issues go when none is named: `owner/repo`, or a Linear team key such as `ENG`. |
| `webhook_secret` | The secret webhooks are signed with. Leave it out for GitHub: Yuva makes one and shows it once. |

The answer has `webhook_url`. Changing the token (`PATCH /v1/issue-integrations/{id}`) checks the new
one first; `POST /v1/issue-integrations/{id}/webhook-secret` makes a new secret. Disconnecting
(`DELETE`) removes the token and every link to that tracker's issues; progress already set stays.
Connecting, changing and disconnecting are written to the [audit log](operations.md#audit-log).

## 2. Add the webhook

So Yuva hears of a change at once:

- **GitHub:** in the repository (or organization) Settings › Webhooks › Add webhook: Payload URL
  `webhook_url`, content type `application/json`, Secret the `webhook_secret` you were shown, and
  "Let me select individual events" › Issues.
- **Linear:** Settings › API › Webhooks › New webhook: URL `webhook_url`, data change events for
  Issues. Copy the signing secret Linear shows and send it:
  `PATCH /v1/issue-integrations/{id}` with `{"webhook_secret": "lin_wh_…"}`.

A request without the right signature is refused. Yuva does not trust what a webhook says: it reads the
issue again with your token. Without a webhook, and as a backstop with one, Yuva reads every linked
issue again about once an hour; after a failed read it waits longer each time, up to a day.

## 3. Create or link an issue

On a feedback conversation:

```sh
# create
curl -X POST "$YUVA_URL/v1/conversations/$ID/issues" -H "Authorization: Bearer $YUVA_KEY" \
  -H "Content-Type: application/json" \
  -d '{"integration_id": "…", "title": "Dark mode", "body": "Several customers ask for it."}'
# link an existing one
curl -X POST "$YUVA_URL/v1/conversations/$ID/issues" -H "Authorization: Bearer $YUVA_KEY" \
  -H "Content-Type: application/json" \
  -d '{"integration_id": "…", "url": "https://github.com/acme/app/issues/12"}'
```

`url` also takes `owner/repo#12`, a Linear issue address or `ENG-123`. A created issue has the title and
body you write, then a line linking back to the conversation in the panel. Nothing else from the
conversation goes to the tracker: the customer's name, address and messages stay in Yuva unless you
write them into the body.

Ideas take issues the same way at `/v1/discussions/{id}/issues`, but an idea only shows its issues and
their state: an issue never moves the idea's stage, which stays your team's decision. Agents with access to the inbox create, link and
unlink (`DELETE …/issues/{linkId}`). A feedback or idea takes up to 10 issues.

## 4. How the state sets the progress

| GitHub | Linear | Feedback progress |
|---|---|---|
| open | Triage, Backlog, Todo (unstarted) | Planned |
| – | In Progress (started) | In progress |
| closed as completed | Done (completed) | Done |
| closed as not planned or duplicate | Canceled | Won't do |

The progress is set when the issue is linked and every time the issue's state changes. If you set a
feedback's progress by hand, it holds until the issue moves again. The change is made as the person or
API key that linked the issue.

The customer sees the progress as a line in their thread. Your team sees which issue set it.

## 5. API keys, webhooks and assistants

- Scopes: `integrations:read` lists trackers, `integrations:manage` connects them (not for keys limited
  to inboxes), `issues:write` with `conversations:read` creates and links issues.
- A conversation's and an idea's `issues` list their links. [Webhooks](webhooks.md) and `/v1/realtime`
  send `issue_link.created`, `issue_link.updated` and `issue_link.deleted`; the progress itself sends
  the usual `conversation.updated`.
- [MCP](mcp.md): `list_issue_integrations`, `link_issue` and `create_issue` on a feedback.
