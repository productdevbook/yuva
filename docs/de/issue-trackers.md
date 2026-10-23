# GitHub- und Linear-Issues

Verbinden Sie GitHub oder Linear mit einem Workspace, und aus einem Feedback oder einer
[Roadmap-Idee](roadmap-board.md) kann ein Issue dort werden, wo Ihr Team seine Arbeit plant. Der Status
des Issues zeigt dem Kunden dann, wo sein Feedback steht, ohne dass jemand ihn von Hand setzt.

## 1. Einen Tracker verbinden

Inhaber und Admins verbinden Tracker; jedes Mitglied sieht, welche verbunden sind. Niemand muss eine App
registrieren.

**GitHub.** Erstellen Sie ein [Fine-grained Personal Access
Token](https://github.com/settings/personal-access-tokens/new), beschränkt auf die Repositories, in die
Issues gehen sollen, mit der Repository-Berechtigung Issues: Read and write. Bei einem GitHub Enterprise
Server geben Sie zusätzlich dessen API-Adresse an, `https://<host>/api/v3`; sie muss über eine
öffentliche Adresse erreichbar sein.

**Linear.** Erstellen Sie unter Settings › Account › Security & access einen persönlichen API-Schlüssel.

Verbinden Sie ihn dann über die API (die Panel-Ansicht folgt):

```sh
curl -X POST "$YUVA_URL/v1/issue-integrations" -H "Authorization: Bearer $YUVA_KEY" \
  -H "Content-Type: application/json" \
  -d '{"provider": "github", "token": "github_pat_…", "default_target": "acme/app"}'
```

| Feld | Bedeutung |
|---|---|
| `provider` | `github` oder `linear`. |
| `token` | Token oder Schlüssel. Yuva prüft ihn zuerst beim Tracker (`422 tracker_auth_failed`, wenn abgelehnt), speichert ihn verschlüsselt unter dem Master-Schlüssel des Servers und zeigt ihn nie wieder. |
| `name` | Für Ihr Team sichtbar; Standard sind Anbieter und Konto. |
| `api_url` | Nur für GitHub Enterprise Server. |
| `default_target` | Wohin neue Issues gehen, wenn kein Ziel genannt wird: `owner/repo` oder ein Linear-Teamschlüssel wie `ENG`. |
| `webhook_secret` | Das Geheimnis, mit dem Webhooks signiert werden. Für GitHub weglassen: Yuva erzeugt eines und zeigt es einmal. |

Die Antwort enthält `webhook_url`. Ein neuer Token (`PATCH /v1/issue-integrations/{id}`) wird zuerst
geprüft; `POST /v1/issue-integrations/{id}/webhook-secret` erzeugt ein neues Geheimnis. Trennen
(`DELETE`) entfernt den Token und alle Verknüpfungen mit Issues dieses Trackers; bereits gesetzter
Fortschritt bleibt. Verbinden, Ändern und Trennen stehen im [Audit-Log](operations.md#audit-log).

## 2. Den Webhook einrichten

Damit Yuva sofort von einer Änderung erfährt:

- **GitHub:** im Repository (oder in der Organisation) Settings › Webhooks › Add webhook: Payload URL
  `webhook_url`, Content type `application/json`, Secret das angezeigte `webhook_secret`, und „Let me
  select individual events“ › Issues.
- **Linear:** Settings › API › Webhooks › New webhook: URL `webhook_url`, Datenänderungen für Issues.
  Kopieren Sie das Signing Secret, das Linear anzeigt, und senden Sie es:
  `PATCH /v1/issue-integrations/{id}` mit `{"webhook_secret": "lin_wh_…"}`.

Eine Anfrage ohne die richtige Signatur wird abgelehnt. Yuva vertraut nicht dem Inhalt eines Webhooks:
Es liest das Issue mit Ihrem Token neu. Ohne Webhook, und mit ihm als Rückfallebene, liest Yuva jedes
verknüpfte Issue etwa einmal pro Stunde neu; nach einem fehlgeschlagenen Lesen wartet es jedes Mal länger,
höchstens einen Tag.

## 3. Ein Issue erstellen oder verknüpfen

In einer Feedback-Konversation:

```sh
# erstellen
curl -X POST "$YUVA_URL/v1/conversations/$ID/issues" -H "Authorization: Bearer $YUVA_KEY" \
  -H "Content-Type: application/json" \
  -d '{"integration_id": "…", "title": "Dark mode", "body": "Several customers ask for it."}'
# ein vorhandenes verknüpfen
curl -X POST "$YUVA_URL/v1/conversations/$ID/issues" -H "Authorization: Bearer $YUVA_KEY" \
  -H "Content-Type: application/json" \
  -d '{"integration_id": "…", "url": "https://github.com/acme/app/issues/12"}'
```

`url` nimmt auch `owner/repo#12`, eine Linear-Issue-Adresse oder `ENG-123`. Ein erstelltes Issue hat den
Titel und Text, den Sie schreiben, und danach eine Zeile mit einem Link zurück zur Konversation im
Panel. Sonst geht nichts aus der Konversation an den Tracker: Name, Adresse und Nachrichten des Kunden
bleiben in Yuva, solange Sie sie nicht in den Text schreiben.

Ideen nehmen Issues genauso unter `/v1/discussions/{id}/issues`, zeigen ihre Issues und deren Status
aber nur an: Ein Issue verschiebt nie die Phase einer Idee, das bleibt die Entscheidung Ihres Teams.
Agenten mit Zugriff auf den
Posteingang erstellen, verknüpfen und lösen Verknüpfungen (`DELETE …/issues/{linkId}`). Ein Feedback
oder eine Idee nimmt bis zu 10 Issues.

## 4. Wie der Status den Fortschritt setzt

| GitHub | Linear | Feedback-Fortschritt |
|---|---|---|
| open | Triage, Backlog, Todo (unstarted) | Geplant |
| – | In Progress (started) | In Arbeit |
| als completed geschlossen | Done (completed) | Erledigt |
| als not planned oder duplicate geschlossen | Canceled | Wird nicht umgesetzt |

Der Fortschritt wird gesetzt, wenn das Issue verknüpft wird, und jedes Mal, wenn sich sein Status
ändert. Setzen Sie den Fortschritt eines Feedbacks von Hand, bleibt er, bis sich das Issue wieder
bewegt. Die Änderung erfolgt im Namen der Person oder des API-Schlüssels, die das Issue verknüpft haben.

Der Kunde sieht den Fortschritt als Zeile in seiner Konversation. Ihr Team sieht, welches Issue ihn gesetzt
hat.

## 5. API-Schlüssel, Webhooks und Assistenten

- Scopes: `integrations:read` listet Tracker, `integrations:manage` verbindet sie (nicht für Schlüssel,
  die auf Posteingänge beschränkt sind), `issues:write` mit `conversations:read` erstellt und verknüpft
  Issues.
- `issues` an einer Konversation und an einer Idee listet ihre Verknüpfungen. [Webhooks](webhooks.md)
  und `/v1/realtime` senden `issue_link.created`, `issue_link.updated` und `issue_link.deleted`; der
  Fortschritt selbst sendet das übliche `conversation.updated`.
- [MCP](mcp.md): `list_issue_integrations`, `link_issue` und `create_issue` für ein Feedback.
