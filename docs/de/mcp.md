# KI-Assistenten (MCP)

Yuva hat einen eingebauten [MCP](https://modelcontextprotocol.io)-Server. Assistenten wie Claude,
ChatGPT, Cursor, VS Code und Gemini CLI können Ihren Posteingang durchsuchen, Unterhaltungen lesen
und sortieren und Antworten entwerfen, mit dem Zugriff, den Sie ihnen geben.

## 1. Die Anmeldung des Assistenten wählen

Der Endpunkt ist `/mcp` auf Ihrem Server, zum Beispiel `https://support.example.com/mcp`.

- **OAuth**: Der Assistent handelt als Sie, mit nicht mehr Zugriff, als Sie selbst haben.
- **API-Schlüssel**: für Automatisierungen ohne Person. Legen Sie unter Settings → API keys einen
  Schlüssel mit den nötigen [Scopes](#tools) und einem Bot-Namen wie „Claude“ an
  ([API-Keys](headless.md#api-keys)).

## 2. Den Server im Assistenten eintragen

In Claude Code, mit OAuth:

```sh
claude mcp add --transport http yuva https://support.example.com/mcp
claude mcp login yuva
```

Mit einem API-Schlüssel ergänzen Sie stattdessen den ersten Befehl um
`--header "Authorization: Bearer $YUVA_API_KEY"`. Andere Assistenten:

| Assistent | So geht's |
|---|---|
| claude.ai, Claude Desktop | Customize → Connectors → **Add custom connector**, die `/mcp`-URL eingeben, anmelden. Braucht eine öffentliche `https`-URL ([Anleitung](https://claude.com/docs/connectors/custom/add-unlisted)). |
| Claude Desktop, privater Server | `yuva.mcpb` aus einem Release öffnen; es fragt nach Server-URL und API-Schlüssel ([Bundle](#lokale-clients)). |
| ChatGPT | [Plugins](https://chatgpt.com/plugins) → Plus → **Add custom MCP server**, nur OAuth. Braucht eine öffentliche `https`-URL ([Anleitung](https://developers.openai.com/plugins/deploy/connect-chatgpt)). |
| Cursor | `.cursor/mcp.json`: `{"mcpServers": {"yuva": {"url": "https://support.example.com/mcp"}}}`; für einen Schlüssel `headers` ergänzen ([Doku](https://cursor.com/docs/context/mcp)). |
| VS Code | **MCP: Add Server**, oder `.vscode/mcp.json` mit `"type": "http"` und der `url`; für einen Schlüssel `headers` ergänzen ([Doku](https://code.visualstudio.com/docs/copilot/customization/mcp-servers)). |
| Gemini CLI | `gemini mcp add --transport http yuva https://support.example.com/mcp`, dann `/mcp auth yuva`; oder `--header` für einen Schlüssel ([Doku](https://geminicli.com/docs/tools/mcp-server/)). |
| MCP Inspector | `npx @modelcontextprotocol/inspector`, um jedes Tool, jede Ressource und jeden Prompt zu sehen. |

## 3. Fragen

Zum Beispiel: „Liste meine Yuva-Posteingänge auf und entwirf eine Antwort auf Türkisch für die
offene Unterhaltung im türkischen Posteingang.“

Antworten sind Entwürfe. Ein Mitglied prüft jeden in der Unterhaltung und sendet, bearbeitet oder
verwirft ihn, außer der Workspace lässt Bots senden ([Bots may send](headless.md#bots-may-send)).
Was ein Assistent über OAuth schreibt, erscheint als „Ayşe via Claude“; mit einem API-Schlüssel als
der Bot des Schlüssels.

## Referenz

### Tools

Tools durchlaufen dieselben Prüfungen wie `/v1`. Tools, die Sie nicht nutzen dürfen, werden nicht
aufgelistet.

| Tools | Scope |
|---|---|
| `search_conversations`, `get_conversation`, `list_labels`, `list_canned_replies`, `get_counts`, `list_feedback` | `conversations:read` |
| `list_inboxes` | `inboxes:read` |
| `get_contact`, `lookup_contact` | `contacts:read` |
| `draft_reply` | `messages:write` |
| `send_reply` | `messages:write`; nur wenn Bots senden dürfen |
| `send_draft` (sendet einen Entwurf aus `draft_reply`) | `messages:write` und `drafts:send`; nur wenn Bots senden dürfen |
| `add_note` | `notes:write` |
| `assign`, `set_status`, `snooze`, `add_labels`, `remove_labels`, `move_conversation`, `bulk_update` | `conversations:write` |
| `merge_contacts` (Inhaber, Admins, Schlüssel ohne Posteingangs-Beschränkung) | `contacts:write` |

Ist in einer Unterhaltung Ihr Entwurf offen, antwortet `send_reply` dort mit `draft_pending`.

### Ressourcen und Prompts

| Art | Einträge |
|---|---|
| Ressourcen | `yuva://inbox/{id}` (der Posteingang und seine neuesten offenen Unterhaltungen), `yuva://conversation/{id}`, `yuva://contact/{id}`; Abonnements folgen Änderungen. |
| Prompts | `triage_inbox`, `draft_reply`, `summarize_conversation`, `weekly_report`. |

### Sicherheit und Grenzen

| Punkt | Detail |
|---|---|
| Kundentext | Wird nur in `customer_content`-Feldern zurückgegeben, als Daten markiert, nie als Anweisungen. |
| Hinweise | Lese-Tools haben `readOnlyHint`, wiederholbare Schreib-Tools `idempotentHint`, `merge_contacts` `destructiveHint`. Jedes Tool hat ein Ausgabeschema. |
| Rate Limit | 600 Anfragen pro Minute und Token oder Schlüssel; darüber `429` mit `Retry-After`. |
| Abschalten | Mit `YUVA_MCP=off` antwortet `/mcp` mit `404` ([Konfiguration](configuration.md#server)). Die OAuth-Endpunkte bleiben. |

### OAuth

Yuva ist sein eigener OAuth-2.1-Server (Authorization Code, PKCE `S256`, Resource Indicators).

| Punkt | Detail |
|---|---|
| Discovery | `401` mit `WWW-Authenticate` → `/.well-known/oauth-protected-resource/mcp` → `/.well-known/oauth-authorization-server`. |
| Registrierung | Dynamisch (`POST /oauth/register`) oder ein Client ID Metadata Document (eine `client_id`, die eine `https`-URL ist). |
| Zustimmung | Im Panel: Workspace und Scopes wählen. Mit `YUVA_PANEL=off` bekommen Clients `temporarily_unavailable`. |
| Tokens | Handeln als Sie, eingeschränkt auf die erlaubten Scopes. Access Tokens gelten eine Stunde und lassen sich bis zu 30 Tage erneuern. |
| Redirect-URIs | `https`, `http` auf einem Loopback-Host oder ein Private-Use-Schema, exakt verglichen; der Port einer Loopback-`http`-URI darf abweichen. |
| Freigaben | Settings → Connected apps listet und widerruft sie. |

### Lokale Clients

`yuva mcp stdio` verbindet stdio mit `/mcp`, für Clients, die nur lokale Programme starten.

```sh
yuva mcp stdio --url https://support.example.com --key "$YUVA_API_KEY"
```

| Punkt | Detail |
|---|---|
| Optionen | `--url` (`/mcp` wird angehängt, wenn es fehlt) und `--key` (ein API-Schlüssel oder OAuth-Access-Token), oder `YUVA_URL` und `YUVA_API_KEY`. |
| Binary | In `yuva.mcpb` (ein Zip: `server/<os>-<arch>/yuva`, `server/yuva.exe` unter Windows), oder `docker run --rm -i -e YUVA_URL -e YUVA_API_KEY ghcr.io/productdevbook/yuva:<version> mcp stdio`. |
| Claude-Desktop-Bundle | Releases ab 0.0.4 enthalten `yuva.mcpb` für macOS, Linux und Windows. Ohne Bundle tragen Sie `"command": "/usr/local/bin/yuva"`, `"args": ["mcp", "stdio"]` und die beiden Variablen in `env` in `claude_desktop_config.json` ein. |
| Fehler | Ein fehlgeschlagener Aufruf liefert einen JSON-RPC-Fehler; ein `401` beendet mit Status 1. |
| MCP Registry | Eingetragen als `io.github.productdevbook/yuva`, mit dem Remote-Endpunkt und dem Bundle. |
