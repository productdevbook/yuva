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
- `get_broadcast` zeigt dieselbe Prüfung wie die Seite des Newsletters: jeden Punkt als ok, Warnung oder
  blockierend. Der Assistent behebt Probleme im Design mit `update_design` und alles andere am Entwurf
  (Betreff, Vorschautext, Kanal, Listen, Klick-Tracking, UTM-Parameter, Tempo) mit `update_broadcast`,
  das die Prüfung erneut zurückgibt. Freigabe und die Domain-Prüfungen des Kanals bleiben bei Ihnen.
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

## Hilfecenter über einen Assistenten

Mit den Berechtigungen `help:read` und `help:write` kann ein Assistent ein [Hilfecenter](help-center.md)
durchsuchen, Artikel lesen und Entwürfe schreiben. Fragen Sie zum Beispiel: „Finde, wonach Kunden in
den letzten 30 Tagen gesucht haben, ohne eine Antwort zu finden, und schreibe Entwürfe für die drei
wichtigsten Artikel.“ Der Prompt `help_gaps_review` macht das; `write_help_article` entwirft einen
Artikel aus einer Unterhaltung oder einem Thema und sucht vorher, damit nichts doppelt geschrieben
wird. Ein Assistent veröffentlicht nie: Er speichert Entwürfe, und ein Mitglied liest und
veröffentlicht sie in Yuva. In `draft_reply` hängt `article_ids` bis zu drei veröffentlichte Artikel
als Karten an die Antwort.

## Roadmap aus einem Assistenten

Mit `roadmap:read`, `roadmap:write` und `conversations:read` kann ein Assistent die Ideen eines
[Roadmap-Boards](roadmap-board.md) auflisten und Feedback mit ihnen verknüpfen. Der Prompt `link_feedback_to_ideas`
geht das noch nicht verknüpfte Feedback durch, durchsucht das Board und schlägt Verknüpfungen vor; er ändert
nichts, bis Sie zustimmen, und verschiebt nie eine Phase. Eine Phase zu verschieben erfahren die Kunden:
`move_idea_stage` wird nur einem Schlüssel oder einer Freigabe gelistet, der den Scope `roadmap:stage` ausdrücklich
nennt; kein anderer Scope schließt ihn ein. Zusätzlich braucht es `confirm_linked`, die Zahl des verknüpften
Feedbacks, die `get_roadmap_idea` für diese Phase zeigte (`would_change`) und die Sie gesehen haben; jede andere
Zahl antwortet mit `409 confirm_mismatch`, und nichts wird verschoben. Kein Werkzeug legt Ideen an, bearbeitet oder
moderiert sie oder sendet eine Antwort an verknüpftes Feedback.

## Neuigkeiten aus einem Assistenten

Mit `changelog:read` und `changelog:write` kann ein Assistent einen [Neuigkeiten](whats-new.md)-Changelog lesen und Einträge als Entwürfe schreiben. Geben Sie dem Prompt `draft_release_notes` Ihre Commit-Titel oder Release Notes (Yuva ruft selbst nichts ab); er entwirft für jede Änderung, die ein Kunde bemerkt, einen Eintrag mit dem Label new, improved oder fixed. Ein Assistent veröffentlicht, plant, zieht zurück oder versendet einen Eintrag nie per E-Mail und kann einen geplanten nicht ändern: Die Entwürfe liest ein Mitglied und veröffentlicht sie in Yuva.

## Listen, Designs und Einrichtung über einen Assistenten

Über Entwürfe hinaus kann ein Assistent die gewöhnlichen Schritte erledigen, die vor einem Newsletter oder einer neuen Produktseite kommen; Rolle und Scope jedes Tools stehen in der [Tabelle unten](#tools). Tools, die Sie nicht nutzen können, werden nicht aufgelistet.

- **Absender und Kanal.** `update_inbox_sender` setzt den Firmennamen, die Postanschrift, die Registrierung und den Kontakt, die die Fußzeile jedes Broadcasts zeigt. `create_email_channel` fügt eine Adresse ohne SMTP-Konto hinzu: Die Antwort nennt die Adresse, an die das Postfach weiterleitet, und einen Link, unter dem Sie SMTP selbst eintragen. `create_dkim_key` und `get_deliverability` liefern den DNS-Eintrag und das, was die Prüfungen gefunden haben.
- **Listen und Abonnenten.** `create_mailing_list`, `update_mailing_list`, `list_subscriptions`, `add_subscription` und `unsubscribe`. `add_subscription` schickt eine Bestätigungsmail und lässt die Person ausstehend. Als angemeldet gilt sie nur, wenn Sie angeben, dass sie bereits zugestimmt hat, mit Ort und Zeitpunkt in Ihren eigenen Worten; der Assistent gibt Ihre Worte weiter und schreibt sie nie selbst. Gesperrte und ausgetragene Adressen werden abgelehnt.
- **CSV-Import.** `create_list_import` nimmt den Text der Datei (über einen Assistenten bis etwa 5 MB; größere Dateien lädt man in Yuva hoch, siehe [Abonnenten importieren](importing.md)) mit `attestation` und `consent_source`, die als Einwilligungsnachweis gespeichert werden und Ihre Worte sein müssen. Es validiert nur. Nachdem `get_list_import` die Zahlen gezeigt hat, braucht `start_list_import` `confirm_count`, die Zahl gültiger Zeilen, die Ihnen gezeigt wurde; jede andere Zahl antwortet `409 count_mismatch`, und nichts wird hinzugefügt.
- **Bilder und Designs.** `upload_email_image` nimmt eine öffentliche `https`-URL (innerhalb von 10 Sekunden abgerufen; private und lokale Adressen werden abgelehnt) oder `data_base64`, über einen Assistenten höchstens etwa 5 MB Bild, und gibt die ID zurück, die ein Bildblock von `update_design` nimmt. `list_designs`, `get_design`, `list_email_images` und `duplicate_design` vervollständigen das Set. Die Bildregeln stehen unter [E-Mail-Designs](designs.md).
- **Designs in Unterhaltungen.** `send_reply`, `draft_reply`, `compose_mail` und `start_conversation` nehmen ein in Unterhaltungen angebotenes Design (`design_id`; `list_designs` mit `for_conversations: true` listet sie) und optional `design_locale`. Der Text wird in der Sprache des Kontakts in den Message-Block des Designs gesetzt. `update_draft` nimmt ebenfalls `design_id` und `design_locale`; `no_design: true` sendet den Entwurf als reinen Text. `preview_message_design` zeigt, was der Kontakt bekäme (Betreff, Fußzeile, Text, Größe und Warnungen; mit `include_html` das HTML als eingebettete Ressource), ohne etwas zu speichern oder zu senden. Mitglieder, die Designs nicht bearbeiten, und Verbindungen mit `conversations:read` sehen nur angebotene Designs. Regeln, Fußzeilen und Grenzen für Mails an Personen, die noch nicht geschrieben haben, stehen unter [E-Mail-Designs](designs.md#8-ein-design-in-einer-unterhaltung-verwenden).
- **Laufende Broadcasts.** `pause_broadcast`, `resume_broadcast`, `unschedule_broadcast` und `cancel_broadcast` werden nach derselben Regel aufgelistet wie `schedule_broadcast`. `cancel_broadcast` braucht `confirm_name`, den Namen des Broadcasts, wie Sie ihn bestätigt haben.
- **Ausgelöste E-Mails.** `create_triggered_email` legt für einen Posteingang den Entwurf einer [ausgelösten E-Mail](broadcasts.md#8-ausgelöste-e-mails) an: das Ereignis `contact_identified`, eine `delay` mit `delay_unit` (`minutes`, `hours` oder `days`, höchstens 365 Tage) und eine `basis`, `service` oder `marketing` (`list_ids` nur für `marketing`). Optional sind Sprachen, Bedingungen, Klick-Tracking, Zeitzone und Versandzeiten. `update_broadcast` nimmt auch `delay`, `delay_unit` und `backfill_days`. `get_broadcast` und `list_broadcasts` (mit `kind`) zeigen den `trigger`, die Prüfung und bei einer gestarteten `enrolled`, `waiting` und `skipped_triggered`. Nichts wird gesendet und niemand erfasst, bis `schedule_broadcast` sie startet, nach derselben Regel wie bei Broadcasts; dann ist `confirm_recipients` die Zahl der Rückwirkung (0 ohne `backfill_days`), und eine `service`-E-Mail braucht `confirm_service: true`, das Sie erst geben, nachdem die Person, für die Sie arbeiten, bestätigt hat, dass die Mail die Nutzung des Produkts betrifft und keine Werbung ist. `list_triggered_waiting` listet auf, wer wartet und wann seine Mail rausgeht (`state` auch `queued`, `skipped` oder `cancelled`), und `remove_triggered_contact` nimmt einen wartenden Kontakt endgültig heraus, nur auf Ihre Bitte. `pause_broadcast`, `resume_broadcast` und `cancel_broadcast` pausieren, setzen fort und stoppen auch eine ausgelöste E-Mail.
- **Personen und Unterhaltungen.** `list_members` liefert `assign` die IDs; Kontakte, ihre Notizen und Abonnements lassen sich suchen, anlegen und ändern; `start_conversation` schreibt die erste Nachricht als Entwurf, solange der Workspace Assistenten nicht zustellen lässt; `list_drafts` zeigt Entwürfe, die auf Prüfung warten, `update_draft` schreibt einen um und `discard_draft` löscht einen; Labels und Textbausteine lassen sich anlegen. `list_discussions` und `get_discussion` lesen Diskussionen; `reply_to_discussion` und `mark_discussion_answer` antworten und markieren die Antwort in Ihrem Namen, nur über OAuth und nur, solange der Workspace Assistenten zustellen lässt, da eine Antwort sofort öffentlich ist. `request_broadcast_approval` bittet den Betreiber, Broadcasts zu erlauben; nur für Inhaber über OAuth, und Website sowie die Beschreibungen des Absenders und der Einwilligung stammen von Ihnen, nie vom Assistenten.
- **Weitere Produkte.** `create_status_page`, `create_status_component`, `create_status_monitor`, `update_incident`, `create_help_center`, `create_help_collection`, `create_changelog`, `create_roadmap_board`, `create_funnel` und `analytics_retention`. Nichts davon wird veröffentlicht: Das tut ein Mitglied in Yuva. `create_status_monitor` zeigt das Push-Token nur einmal, damit Sie es in Ihren Monitoring-Job eintragen.

Was ein Assistent absichtlich nicht kann: Zugangsdaten und Zugriff verwalten (API-Schlüssel, OAuth-Freigaben, Rollen, Einladungen, Passkeys, Push-Abos, Webhooks, ein Secret rotieren), Workspace-Einstellungen ändern oder den Workspace löschen, etwas löschen (außer `remove_migration_history`), Einwilligungsnachweise oder Analytics exportieren oder Analytics-Daten löschen, Migrationsdateien hochladen, Rohnachrichten und Anhänge lesen, SMTP- oder andere Passwörter entgegennehmen, Hilfeartikel, Neuigkeiten-Einträge und Störungen veröffentlichen. Austragen ist eine Zustandsänderung, kein Löschen.

## Referenz

### Tools

Tools durchlaufen dieselben Prüfungen wie `/v1`. Tools, die Sie nicht nutzen dürfen, werden nicht
aufgelistet; ein Aufruf dennoch antwortet mit `insufficient_scope` und dem zu erteilenden Scope.

| Tools | Scope |
|---|---|
| `get_setup_state`, `report_yuva_issue` | keiner; jede Verbindung |
| `search_conversations`, `get_conversation`, `list_labels`, `list_canned_replies`, `get_counts` | `conversations:read` |
| `get_report` ([Berichte](reports.md)) | `reports:read` |
| `list_feedback` (mit `feedback_score`, 1 bis 5, wenn der Kontakt eine Bewertung gab; `without_idea` behält Feedback ohne Verknüpfung mit einer Idee) | `conversations:read` |
| `list_inboxes` | `inboxes:read` |
| `get_contact`, `lookup_contact` | `contacts:read` |
| `draft_reply` | `messages:write` |
| `preview_message_design` | `conversations:read` |
| `send_reply` | `messages:write`; nur wenn Bots senden dürfen |
| `send_draft` (sendet einen Entwurf aus `draft_reply`) | `messages:write` und `drafts:send`; nur wenn Bots senden dürfen |
| `update_draft`, `discard_draft` (nur ungesendete Entwürfe; der Entwurf eines Mitglieds nur von diesem Mitglied) | `messages:write` |
| `add_note` | `notes:write` |
| `assign`, `set_status`, `snooze`, `add_labels`, `remove_labels`, `move_conversation`, `bulk_update` | `conversations:write` |
| `merge_contacts` (Inhaber, Admins, Schlüssel ohne Posteingangs-Beschränkung) | `contacts:write` |
| `list_channels`, `get_install_snippet` (Code für `html`, `react`, `nextjs`, `nuxt`, `vue`, `astro`, `shadcn`, Dokumentationsseiten mit den `docs-`-Varianten und Anmeldeformulare mit `subscribe-html`, `subscribe-react`, `subscribe-nuxt` und `subscribe-shadcn`, die `list_ids` nehmen) | `inboxes:read` |
| `create_inbox`, `create_chat_channel`, `update_chat_channel`, `create_app_channel`, `update_app_channel` (Inhaber und Admins) | `inboxes:manage` |
| `list_properties`, `analytics_overview`, `analytics_query` (auch `dataset: support` und `insights`), `analytics_realtime`, `analytics_support_report`, `list_goals`, `funnel_report`, `get_analytics_snippet`, `add_annotation` | `analytics:read` |
| `create_property`, `create_goal` (Inhaber, Admins, Keys ohne Posteingangs-Beschränkung) | `analytics:manage` |
| `list_status_pages`, `get_status_page`, `list_monitors` | `status:read` |
| `set_component_state`, `create_incident` (immer ein Entwurf), `post_incident_update`, `resolve_incident` | `status:manage` |
| `link_conversation_to_incident` | `status:manage` und `conversations:read` |
| `list_roadmap_ideas`, `get_roadmap_idea` | `conversations:read` und `roadmap:read` |
| `link_feedback_to_idea`, `unlink_feedback_from_idea` | `roadmap:write` und `conversations:read` |
| `move_idea_stage` (nur wenn `roadmap:stage` ausdrücklich genannt ist; braucht `confirm_linked`) | `roadmap:stage` |
| `list_help_centers`, `search_help_articles`, `get_help_article`, `help_search_gaps` | `help:read` |
| `draft_help_article`, `update_help_article_draft` (nur Entwürfe, mit `base_version`; eine `locale` schreibt eine Übersetzung) | `help:write` |
| `import_help_articles` (aus einer angemeldeten Zendesk-Migration ins Center, als Entwürfe), `get_help_import` | `help:write` |
| `list_whats_new_entries`, `get_whats_new_entry` | `changelog:read` |
| `draft_whats_new_entry`, `update_whats_new_draft` (nur Entwürfe, mit `base_version`; eine `locale` schreibt eine Übersetzung) | `changelog:write` |
| `list_mailing_lists` | `lists:read` |
| `list_broadcasts`, `get_broadcast` (mit Prüfung und, bei einem Entwurf, der Zielgruppenzahl), `get_broadcast_report` | `broadcasts:read` |
| `list_email_templates`, `preview_design` (Inhaber und Admins) | `broadcasts:read` |
| `create_design`, `update_design`, `create_broadcast_draft`, `update_broadcast`, `send_test_broadcast` (Inhaber und Admins) | `broadcasts:write` |
| `create_triggered_email`, `remove_triggered_contact` (Inhaber, Admins, Keys) | `broadcasts:write` |
| `list_triggered_waiting` | `broadcasts:read` und `contacts:read` |
| `schedule_broadcast` (nur wenn der Workspace Keys und Assistenten das Senden von Broadcasts erlaubt) | `broadcasts:send` |
| `list_members` | `inboxes:read` |
| `list_teams` | `inboxes:read` |
| `list_macros` | `conversations:read` |
| `list_custom_fields` | `conversations:read` oder `contacts:read` |
| `set_conversation_fields` | `conversations:write` |
| `search_contacts`, `list_contact_notes` | `contacts:read` |
| `list_contact_subscriptions` | `contacts:read` und `lists:read` |
| `create_contact`, `update_contact`, `add_contact_note` | `contacts:write` |
| `start_conversation` (die erste Nachricht ist ein Entwurf, außer der Aufrufer darf zustellen und übergibt `send: true`) | `conversations:write` und `messages:write` |
| `list_drafts`, `list_discussions`, `get_discussion` | `conversations:read` |
| `reply_to_discussion`, `mark_discussion_answer` (nur OAuth; nur wenn Bots senden dürfen) | `messages:write` |
| `request_broadcast_approval` (nur Inhaber über OAuth) | `workspace:manage` |
| `create_label` (Inhaber, Admins, Schlüssel) | `labels:write` |
| `create_canned_reply`, `update_canned_reply` (Inhaber, Admins, Schlüssel) | `canned_replies:write` |
| `update_inbox_sender`, `create_email_channel`, `create_dkim_key` (Inhaber, Admins, Schlüssel) | `inboxes:manage` |
| `get_deliverability` (mit `check: true` sendet es eine Testnachricht und braucht `inboxes:manage`) | `inboxes:read` |
| `list_subscriptions`, `get_list_import`, `list_list_imports`, `get_list_import_errors` | `lists:read` |
| `create_mailing_list`, `update_mailing_list`, `add_subscription`, `unsubscribe`, `create_list_import`, `update_list_import_mapping`, `cancel_list_import`, `start_list_import` (Inhaber, Admins, Schlüssel) | `lists:write` |
| `list_designs`, `get_design`, `list_email_images` (Inhaber, Admins, Schlüssel) | `broadcasts:read` |
| `list_designs` (andere Mitglieder und Verbindungen nur mit diesem Scope: die in Unterhaltungen angebotenen Designs) | `conversations:read` |
| `duplicate_design`, `upload_email_image` (Inhaber, Admins, Schlüssel) | `broadcasts:write` |
| `pause_broadcast`, `resume_broadcast`, `unschedule_broadcast`, `cancel_broadcast` (nur wenn der Workspace Keys und Assistenten das Senden von Broadcasts erlaubt) | `broadcasts:send` |
| `create_status_page`, `create_status_component`, `create_status_monitor`, `update_incident` (Inhaber, Admins, Schlüssel) | `status:manage` |
| `create_help_collection` | `help:write` |
| `create_help_center` (Inhaber, Admins, Schlüssel; kein OAuth) | `help:publish` |
| `create_changelog` (Inhaber, Admins, Schlüssel; kein OAuth) | `changelog:publish` |
| `create_roadmap_board` (Inhaber, Admins, Schlüssel; kein OAuth) | `roadmap:write` |
| `create_funnel` (Inhaber, Admins, Schlüssel) | `analytics:manage` |
| `analytics_retention` | `analytics:read` |
| `detect_mail_source`, `list_migrations`, `get_migration`, `create_migration`, `update_migration`, `test_migration_connection`, `scan_migration`, `preview_migration`, `start_migration`, `pause_migration`, `resume_migration`, `cancel_migration`, `finish_migration`, `retry_migration_items`, `list_migration_errors`, `send_migration_probe`, `remove_migration_history` (Owner und Admins) | `migrations:manage` |

Ist in einer Unterhaltung Ihr Entwurf offen, antwortet `send_reply` dort mit `draft_pending`.

Kein Umzugs-Tool nimmt ein Passwort entgegen: `create_migration` liefert eine Yuva-Seite (`connect_url`), auf der Sie das App-Passwort eingeben. `remove_migration_history` ist als destruktiv markiert und braucht `confirm_count`, die Zahl der Unterhaltungen, die Sie bestätigt haben.

Mit `inbox_id` und ohne `channel_id` verwendet `create_migration` den E-Mail-Kanal des Posteingangs für die Postfachadresse oder legt einen an (zusätzlich `inboxes:manage`). Ein neuer Kanal hat kein SMTP-Konto; die Antwort enthält daher `channel_created`, `channel_setup` (was im Panel einzustellen ist, bevor Antworten gesendet werden können) und `channel_settings_url`; der Prompt `move_to_yuva` gibt sie an Sie weiter.

`assign` nimmt `team_id` (ein Team aus `list_teams`, das den Posteingang der Unterhaltung hat) oder `remove_team: true`; wird eines davon gesendet und `assignee_id` weggelassen, bleibt das zuständige Mitglied unverändert. `search_conversations` nimmt `team_id`, und Unterhaltungen tragen `team_id`.

`list_macros` liefert die geteilten Makros des Workspace und Ihre eigenen mit `id`, `name`, `shortcut`, `visibility`, `body` und `actions` (`add_labels`, `remove_labels`, `assignee_id`, `unassign`, `status`, `snooze_minutes`, `priority`). Die Aktionen dienen nur der Information; kein Tool wendet ein Makro an.

`list_custom_fields` liefert die [benutzerdefinierten Felder](custom-fields.md) des Workspace (`object`, `key`, `name`, `type`, `options`, `required_on_close`), optional für `object` `conversation` oder `contact`. `set_conversation_fields` nimmt `conversation_id` und `fields`, Werte nach Schlüssel (`null` leert einen); ein Wert, der nicht passt, antwortet mit `field_invalid`. `search_conversations` nimmt `fields`, ein Objekt aus Schlüssel und Wert, und Unterhaltungen tragen `fields`. Schließen mit `set_status` oder `bulk_update`, während ein Pflichtfeld leer ist, antwortet mit `field_required`. Kein Tool definiert Felder.

`search_conversations` nimmt `sla_state` (`on_track`, `due_soon`, `breached`, `met` oder `paused`) und `sort` (`last_activity` oder `sla_due` für die früheste [SLA](sla.md)-Fälligkeit zuerst); Unterhaltungen tragen `sla`, wenn eine Richtlinie gilt. Kein Werkzeug verwaltet Richtlinien.

### Ressourcen und Prompts

| Art | Einträge |
|---|---|
| Ressourcen | `yuva://inbox/{id}` (der Posteingang und seine neuesten offenen Unterhaltungen), `yuva://conversation/{id}`, `yuva://contact/{id}`, `yuva://broadcast/{id}`, `yuva://migration/{id}` (mit `migrations:manage`), `yuva://property/{id}` (eine Website mit ihren Einstellungen, den letzten sieben Tagen und ihren Goals; mit `analytics:read`), `yuva://help-center/{id}` (mit `help:read`), `yuva://roadmap/{board_id}` (ein Board mit seinen beliebtesten Ideen; mit `roadmap:read` und `conversations:read`), `yuva://changelog/{id}` (ein Changelog mit seinen neuesten Einträgen; mit `changelog:read`); Abonnements folgen Änderungen. |
| Prompts | `init` (optionales Ziel; jede Verbindung), `triage_inbox`, `draft_reply`, `summarize_conversation`, `weekly_report`, `setup_live_chat`, `draft_newsletter`, `broadcast_report`, `move_to_yuva` (Adresse, optional Posteingang; mit `migrations:manage`), `analytics_review` (optional Website und Zeitraum; mit `analytics:read`), `setup_analytics` (Site-Adresse, optional Framework; mit `analytics:manage`), `write_help_article` (Center, optional Unterhaltung oder Thema, Sprache; mit `help:write`), `help_gaps_review` (Center, optional Tage; mit `help:read`), `link_feedback_to_ideas` (optional Posteingang und Board; mit `roadmap:write`, `roadmap:read` und `conversations:read`), `draft_release_notes` (Notizen als Vorlage, optional Changelog und Sprache; mit `changelog:write`). |

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
