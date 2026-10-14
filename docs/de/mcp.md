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

## Yuva mit init zu einem Projekt hinzufügen

Öffnen Sie Ihren Assistenten im Ordner des Projekts und starten Sie den Prompt `init` (in Claude
Code `/mcp__yuva__init`), oder fragen Sie: „Sieh dir dieses Projekt an und sag mir, wie ich Yuva
hinzufüge.“ Der Assistent liest das Projekt auf Ihrem Rechner (Framework, Anmeldung,
Dokumentationsseite, Mobile Apps, die Support- und Analyse-Tools, die Sie heute nutzen), ruft
`get_setup_state` auf, um zu sehen, was Ihr Workspace schon hat, und antwortet mit einem kurzen
Plan: was jeder Schritt dem Projekt bringt, woran er das im Code erkennt und welcher Prompt oder
welche Tools ihn umsetzen. Nichts ändert sich, bevor Sie Schritte auswählen.

Ihr Code und Ihre Dateien bleiben beim Assistenten: `init` sendet nichts aus dem Projekt an Yuva.
Schritte, die Posteingänge, Kanäle oder Analytics-Properties anlegen, brauchen den Scope, den sie
nennen; `get_setup_state` listet, was die Verbindung nicht lesen kann, und welchen Scope Sie erteilen
müssen.

## Ein Problem mit Yuva melden

Verhält sich Yuva selbst falsch, während ein Assistent damit arbeitet (ein Serverfehler, ein SDK,
das nicht tut, was seine Anleitung sagt, eine falsche Anleitung), kann der Assistent
`report_yuva_issue` aufrufen. Der Server schreibt ein GitHub-Issue für
[productdevbook/yuva](https://github.com/productdevbook/yuva/issues), entfernt dabei die Namen Ihres
Workspace, seine Websites und Adressen, E-Mail-Adressen, andere URLs und Hostnamen, IP-Adressen,
Schlüssel, Tokens, IDs und Pfade im Home-Verzeichnis, und gibt den Text mit einem vorausgefüllten
Link und einem `gh issue create`-Befehl zurück. Selbst legt er nichts an: Der Assistent zeigt Ihnen
den Bericht, und Sie öffnen ihn mit Ihrem eigenen GitHub-Konto. Sicherheitsprobleme gehen stattdessen
an die [private Meldung](https://github.com/productdevbook/yuva/security/advisories/new).

## Live-Chat von einem Assistenten einrichten lassen

Inhaber und Admins können die Einrichtung einem Assistenten überlassen. Erteilen Sie beim Verbinden
`inboxes:manage` (ein API-Schlüssel braucht es ebenfalls) und fragen Sie zum Beispiel: „Verbinde den
Yuva-Live-Chat mit https://www.example.com. Es ist die Acme-Website, gebaut mit Next.js.“

Der Prompt `setup_live_chat` macht dasselbe. Der Assistent findet oder erstellt den Posteingang,
erstellt den Chat-Kanal für den Origin der Website (oder fügt den Origin dem vorhandenen hinzu) und
holt den Installationscode, ausgefüllt mit der URL Ihres Servers und dem öffentlichen Schlüssel des
Kanals. Hat er Zugriff auf Ihren Code, fügt er ihn dort ein, sonst zeigt er ihn. Eine Verbindung ohne
`inboxes:manage` bekommt einen Fehler, der den Scope nennt: neu verbinden und ihn erteilen. Das
Identitätsgeheimnis für [angemeldete Nutzer](identity.md) bekommen Assistenten nicht; erneuern Sie
es bei Bedarf in den Einstellungen des Posteingangs.

## Newsletter-Entwürfe von einem Assistenten

Ein Assistent kann einen Newsletter in ein Design und einen Broadcast-Entwurf schreiben, Ihnen einen Test
schicken und danach den Bericht lesen. An Ihre Abonnenten sendet er nur, wenn Sie es erlauben: Das
Planen ist ein eigener Schritt, den Sie steuern.

- Geben Sie beim Verbinden `lists:read`, `broadcasts:read` und `broadcasts:write` frei (ein API-Key braucht
  sie ebenfalls). Der Prompt `draft_newsletter` geht von einem Briefing aus und plant nie.
- Ein Test über einen verbundenen Assistenten geht nur an Sie. Ein API-Key nennt die Mitglieder, die ihn bekommen.
- `schedule_broadcast` wird nur aufgelistet, wenn der Key oder das Token `broadcasts:send` hat **und** ein
  Inhaber oder Admin „Keys and assistants may send broadcasts“ eingeschaltet hat ([Keys und
  Assistenten](broadcasts.md#7-keys-und-assistenten)). Es braucht `send_at` und `confirm_recipients`, die
  Empfängerzahl, die `get_broadcast` gezeigt hat und die Sie bestätigt haben. Hat sich die Zielgruppe
  inzwischen geändert, wird nichts geplant.

Ein Beispiel:

> **Sie:** Entwirf einen Newsletter an die Liste „Produktneuigkeiten“ aus der Newsletter-Vorlage: Version
> 2.1 ist da, mit Dark Mode und schnellerer Suche. Verlinke das Änderungsprotokoll.
>
> **Assistent:** Ich habe aus der Newsletter-Vorlage das Design „Release 2.1“ und den Broadcast-Entwurf
> „Release 2.1“ mit dem Betreff „Dark Mode und schnellere Suche sind da“ erstellt. Die Prüfung ist
> bestanden. Die Zielgruppe sind 1.248 Personen. Öffnen Sie den Entwurf in Yuva: *Link*.
>
> **Sie:** Schick mir einen Test.
>
> **Assistent:** An Sie gesendet. Sehen Sie in Ihrer Mail-App nach, wie er aussieht.
>
> **Sie:** Gut. Plane ihn für morgen um 9:00.
>
> **Assistent:** Er geht an 1.248 Personen, wie im Entwurf steht. Soll ich ihn für morgen um 9:00 planen?
>
> **Sie:** Ja.
>
> **Assistent:** Für morgen um 9:00 geplant.

Der Assistent plant erst, nachdem Sie die Zahl bestätigt haben; fragen Sie danach, wenn er sie nicht zeigt.

## Analytics über einen Assistenten

Mit `analytics:read` liest ein Assistent die Zahlen Ihrer Websites und Apps ([Analytics](analytics.md)) und, bei einer
mit einem Posteingang verknüpften Website, den Support, den ihre Seiten auslösen ([Support und Traffic](analytics-support.md)).
Fragen Sie zum Beispiel: „Welche Seiten lösen diesen Monat die meisten Unterhaltungen aus?“ Der Assistent listet Ihre
Websites, ruft `analytics_support_report` mit `period: month` und `order_by: conversations` auf und antwortet mit den
Unterhaltungen, Seitenaufrufen und Unterhaltungen pro 1.000 Aufrufe jeder Seite.

- Der Prompt `analytics_review` prüft eine Website über einen Zeitraum: Kennzahlen gegenüber dem Zeitraum davor, Quellen,
  Seiten, Goals und, mit Posteingang, den Support pro Seite, mit Vorschlägen am Ende. Der Prompt `weekly_report` ergänzt
  einen Abschnitt zu Support und Traffic, wenn der Assistent Analytics lesen darf.
- Der Prompt `setup_analytics` (braucht `analytics:manage`) legt eine Website oder App an, holt den Installationscode und
  fügt ihn Ihrem Projekt hinzu, wie `setup_live_chat` es für den Chat tut.
- Seitenpfade, Referrer, Kampagnen, Eventnamen und Prop-Werte stammen aus den Browsern der Besucher und kommen wie
  Kundentext nur in `customer_content` zurück. Analytics-Tools werden nur aufgelistet, solange der Server Analytics an hat.

## Ein Postfach von einem Assistenten umziehen lassen

Starten Sie mit dem Scope `migrations:manage` den Prompt `move_to_yuva` mit der Postfachadresse. Der Assistent
erkennt den Anbieter, legt den Umzug an und gibt Ihnen eine Yuva-Seite, auf der Sie das App-Passwort
eingeben; im Chat fragt er nie danach. Er startet das Kopieren erst, wenn Sie den Probelauf bestätigt haben,
erklärt dann, wie die E-Mails weitergeleitet werden, und schließt den Umzug ab. Die Schritte stehen unter
[Zu Yuva umziehen](migrate.md).

## Referenz

### Tools

Tools durchlaufen dieselben Prüfungen wie `/v1`. Tools, die Sie nicht nutzen dürfen, werden nicht
aufgelistet; ein Aufruf dennoch antwortet mit `insufficient_scope` und dem zu erteilenden Scope.

| Tools | Scope |
|---|---|
| `get_setup_state`, `report_yuva_issue` | keiner; jede Verbindung |
| `search_conversations`, `get_conversation`, `list_labels`, `list_canned_replies`, `get_counts`, `list_feedback` | `conversations:read` |
| `list_inboxes` | `inboxes:read` |
| `get_contact`, `lookup_contact` | `contacts:read` |
| `draft_reply` | `messages:write` |
| `send_reply` | `messages:write`; nur wenn Bots senden dürfen |
| `send_draft` (sendet einen Entwurf aus `draft_reply`) | `messages:write` und `drafts:send`; nur wenn Bots senden dürfen |
| `add_note` | `notes:write` |
| `assign`, `set_status`, `snooze`, `add_labels`, `remove_labels`, `move_conversation`, `bulk_update` | `conversations:write` |
| `merge_contacts` (Inhaber, Admins, Schlüssel ohne Posteingangs-Beschränkung) | `contacts:write` |
| `list_channels`, `get_install_snippet` (Code für `html`, `react`, `nextjs`, `nuxt`, `vue`, `astro`, `shadcn`, Dokumentationsseiten mit den `docs-`-Varianten und Anmeldeformulare mit `subscribe-html`, `subscribe-react`, `subscribe-nuxt` und `subscribe-shadcn`, die `list_ids` nehmen) | `inboxes:read` |
| `create_inbox`, `create_chat_channel`, `update_chat_channel` (Inhaber und Admins) | `inboxes:manage` |
| `list_properties`, `analytics_overview`, `analytics_query` (auch `dataset: support` und `insights`), `analytics_realtime`, `analytics_support_report`, `list_goals`, `funnel_report`, `get_analytics_snippet`, `add_annotation` | `analytics:read` |
| `create_property`, `create_goal` (Inhaber, Admins, Keys ohne Posteingangs-Beschränkung) | `analytics:manage` |
| `list_mailing_lists` | `lists:read` |
| `list_broadcasts`, `get_broadcast` (mit Prüfung und, bei einem Entwurf, der Zielgruppenzahl), `get_broadcast_report` | `broadcasts:read` |
| `list_email_templates`, `preview_design` (Inhaber und Admins) | `broadcasts:read` |
| `create_design`, `update_design`, `create_broadcast_draft`, `send_test_broadcast` (Inhaber und Admins) | `broadcasts:write` |
| `schedule_broadcast` (nur wenn der Workspace Keys und Assistenten das Senden von Broadcasts erlaubt) | `broadcasts:send` |
| `detect_mail_source`, `list_migrations`, `get_migration`, `create_migration`, `update_migration`, `test_migration_connection`, `scan_migration`, `preview_migration`, `start_migration`, `pause_migration`, `resume_migration`, `cancel_migration`, `finish_migration`, `retry_migration_items`, `list_migration_errors`, `send_migration_probe`, `remove_migration_history` (Owner und Admins) | `migrations:manage` |

Ist in einer Unterhaltung Ihr Entwurf offen, antwortet `send_reply` dort mit `draft_pending`.

Kein Umzugs-Tool nimmt ein Passwort entgegen: `create_migration` liefert eine Yuva-Seite (`connect_url`), auf der Sie das App-Passwort eingeben. `remove_migration_history` ist als destruktiv markiert und braucht `confirm_count`, die Zahl der Unterhaltungen, die Sie bestätigt haben.

Mit `inbox_id` und ohne `channel_id` verwendet `create_migration` den E-Mail-Kanal des Posteingangs für die Postfachadresse oder legt einen an (zusätzlich `inboxes:manage`). Ein neuer Kanal hat kein SMTP-Konto; die Antwort enthält daher `channel_created`, `channel_setup` (was im Panel einzustellen ist, bevor Antworten gesendet werden können) und `channel_settings_url`; der Prompt `move_to_yuva` gibt sie an Sie weiter.

### Ressourcen und Prompts

| Art | Einträge |
|---|---|
| Ressourcen | `yuva://inbox/{id}` (der Posteingang und seine neuesten offenen Unterhaltungen), `yuva://conversation/{id}`, `yuva://contact/{id}`, `yuva://broadcast/{id}`, `yuva://migration/{id}` (mit `migrations:manage`), `yuva://property/{id}` (eine Website mit ihren Einstellungen, den letzten sieben Tagen und ihren Goals; mit `analytics:read`); Abonnements folgen Änderungen. |
| Prompts | `init` (optionales Ziel; jede Verbindung), `triage_inbox`, `draft_reply`, `summarize_conversation`, `weekly_report`, `setup_live_chat`, `draft_newsletter`, `broadcast_report`, `move_to_yuva` (Adresse, optional Posteingang; mit `migrations:manage`), `analytics_review` (optional Website und Zeitraum; mit `analytics:read`), `setup_analytics` (Site-Adresse, optional Framework; mit `analytics:manage`). |

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
