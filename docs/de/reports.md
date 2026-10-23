# Berichte

Berichte zählen die Arbeit Ihres Teams: geöffnete, geschlossene und wieder geöffnete Unterhaltungen,
Antworten, wie schnell Sie antworten und lösen, ob Sie Ihre [SLA](sla.md)-Ziele eingehalten haben und wie
Kundinnen und Kunden Sie bewertet haben. Fragen Sie jeden Zeitraum bis zu 13 Monaten ab, nach Tag, Woche
oder Monat, aufgeschlüsselt nach Posteingang, Kanal, Label, Team oder Mitglied, und laden Sie ihn als CSV herunter. Der Bereich Berichte im Panel hat dafür den Tab Trends; die Tabs Übersicht, Bewertungen und Team zeigen heute, die letzten 7 und die letzten 30 Tage. Die API und der MCP-Server unten liefern dieselben Zahlen an Ihre eigenen Werkzeuge.

## Der Tab Trends

Öffnen Sie Berichte › Trends. Wählen Sie einen Zeitraum (eine Vorgabe bis zu 12 Monaten oder eigene Daten bis zu 13 Monaten), Tag, Woche oder Monat und schlüsseln Sie die Zahlen nach Posteingang, Kanal, Label, Team oder Mitglied auf. Filter grenzen den Bericht auf einen Posteingang, Kanal, ein Label, Team oder Mitglied ein. Kennzahlenkacheln zeigen jede Zahl im Vergleich zum Vorzeitraum, Diagramme zeigen Volumen und Antwortzeiten, eine sortierbare Tabelle listet die Gruppen. Ein Hinweis „Daten bis“ sagt, wie weit die Zählungen reichen. **Als CSV exportieren** lädt herunter, was Sie sehen.

## 1. Was gezählt wird

| Zahl | Zählt |
|---|---|
| `opened` | Im Zeitraum begonnene Unterhaltungen. Spam ist ausgenommen. |
| `closed` | Jedes Schließen, auch bei einer mehrfach geschlossenen Unterhaltung. |
| `reopened` | Ein Wechsel von geschlossen zu einem anderen Status. |
| `replies` | Von Mitgliedern gesendete Antworten, auch ein Bot-Entwurf, den ein Mitglied gesendet hat. |
| `first_replies` | Unterhaltungen, deren erste Antwort eines Mitglieds in den Zeitraum fällt, mit Median und p90 in Sekunden (`first_reply_median_seconds`, `first_reply_p90_seconds`). |
| `resolutions` | Schließungen nach einer Nachricht der Kundschaft, mit Median und p90 in Sekunden. Die Uhr beginnt bei der ersten Kundennachricht nach der vorherigen Schließung. |
| `sla` | Eingehaltene und verfehlte Ziele für `first_reply`, `next_reply` und `resolution`. |
| `ratings` | `good` und `bad`, gezählt an dem Tag, an dem die Kundschaft bewertet hat. Feedback wird nie bewertet. |

Antworten, die Bots, [Automatisierungsregeln](automation.md) und automatische Antworten senden, werden nie
gezählt, weder als Antwort noch als erste Antwort. Unterhaltungen im Papierkorb und importierter Verlauf
sind ausgenommen. Ein Median oder p90 ohne Messwerte fehlt in der Antwort.

Median und p90 liegen innerhalb von 2,5 % des genauen Werts. Die Zahlen von heute stimmen mit der Übersicht
im Panel überein.

## 2. Zeitzonen und Zeiträume

`from` und `to` sind lokale Tage in `timezone` (ein IANA-Name wie `Europe/Istanbul`; standardmäßig `UTC`),
`to` ist eingeschlossen. Zwischen beiden dürfen höchstens 13 Monate liegen. Tage nach heute sind leer. Eine
Woche läuft von Montag bis Sonntag; eine durch `from` angeschnittene Woche oder ein Monat beginnt dort. Der
Bericht listet jeden Zeitraum auf, auch leere.

## 3. Gruppierung und Filter

`group_by` schlüsselt jede Zahl nach `inbox`, `channel`, `label`, `team` oder `member` auf. Jede Gruppe hat
eigene Summen und eine eigene Reihe. Zahlen ohne Team, Mitglied oder Kanal landen in einer Gruppe ohne
`key`. Eine Unterhaltung zählt unter jedem ihrer Labels; ohne Label kommt sie in der Gruppierung `label`
nicht vor.

Eine Zahl gehört dem Mitglied, das die Arbeit getan hat: dem Autor einer Antwort, wer geschlossen oder
wieder geöffnet hat, wer die erste Antwort gesendet hat, wer für eine Lösung geschlossen hat. Geöffnete
Unterhaltungen und Bewertungen gehören der zugewiesenen Person, SLA-Ergebnisse der zugewiesenen Person und
dem Team, die die Unterhaltung zum Zeitpunkt des Ergebnisses hatte.

`inbox_id`, `channel_kind` (`email`, `chat`, `app` oder `api`), `label_id`, `team_id` und `member_id`
grenzen den Bericht auf je eines davon ein. `metrics` beschränkt ihn auf einige von `opened`, `closed`,
`reopened`, `replies`, `first_reply`, `resolution`, `sla` und `ratings`; ohne Angabe erhalten Sie alle.

Labels und Team sind die, die eine Unterhaltung hatte, als ihr Zeitraum zuletzt gezählt wurde.

## 4. Wer was sieht

Ein Bericht deckt die Posteingänge ab, die der Aufrufer sehen kann. Inhaber und Admins sehen jeden
Posteingang, Agenten nur die Posteingänge, auf die sie oder ihre Teams Zugriff haben, und ein auf
Posteingänge beschränkter API-Schlüssel nur diese. Wer einen Posteingang abfragt, den er nicht sehen kann,
erhält `404`.

## 5. Wie aktuell die Zahlen sind

Yuva speichert Zählungen pro Viertelstunde, damit ein lokaler Tag in jeder Zeitzone exakt ist. Heute wird
alle paar Minuten neu gezählt, die letzten 31 Tage jede Nacht; dabei werden Label- und Teamänderungen
übernommen. Alles, was neuer ist als die letzte Zählung, wird live ergänzt, die Summen von heute sind also
aktuell. Auf einem frisch aktualisierten Server wird der Verlauf im Hintergrund gezählt: `built_until` in
der Antwort sagt, wie weit die Zählungen reichen, spätere Zeiträume sind leer, bis sie aufgeholt haben.

Die Zählungen enthalten nur Zahlen und IDs: keinen Nachrichtentext, keine Namen oder Adressen von Kunden.
Sie werden mit dem Workspace gelöscht. Die [Aufbewahrung](operations.md#aufbewahrung) löscht sie nicht:
Zeiträume, deren Unterhaltungen durch die Aufbewahrung entfernt wurden, behalten ihre Zahlen.

## 6. API

Verwenden Sie einen [API-Schlüssel](headless.md#scopes) oder eine OAuth-Verbindung mit dem Scope
`reports:read`. Jedes Mitglied darf ihn halten; Schlüssel, die ohne benannte Scopes angelegt werden, haben
ihn, ältere Schlüssel nicht.

```sh
curl -G https://support.example.com/v1/reports \
  -H "Authorization: Bearer $YUVA_API_KEY" \
  -d from=2026-09-01 -d to=2026-09-30 -d timezone=Europe/Istanbul \
  -d granularity=week -d group_by=inbox -d metrics=opened,closed,first_reply
```

Die Antwort enthält `totals`, eine `series` mit einem Eintrag je Zeitraum und mit `group_by` `groups`, jede
mit eigenen `totals` und `series`, dazu `built_until`. Posteingänge, Labels, Teams und Mitglieder erscheinen
als IDs; ihre Namen lesen Sie über deren eigene Endpunkte.

`GET /v1/reports/export` nimmt dieselben Parameter und liefert eine CSV-Datei: eine Kopfzeile, dann eine
Zeile je Zeitraum, mit `group_by` je Gruppe und Zeitraum. Die Spalten sind `period` (der erste Tag des
Zeitraums), mit `group_by` außerdem `group_id` und `group_name`, dann `opened`, `closed`, `reopened`,
`replies`, `first_replies`, `first_reply_median_seconds`, `first_reply_p90_seconds`, `resolutions`,
`resolution_median_seconds`, `resolution_p90_seconds`, `sla_first_reply_met`, `sla_first_reply_breached`,
`sla_next_reply_met`, `sla_next_reply_breached`, `sla_resolution_met`, `sla_resolution_breached`,
`ratings_good` und `ratings_bad`, beschränkt auf die gewählten Metriken. Eine Zeit ohne Messwert ist eine
leere Zelle. Zellen, die mit `=`, `+`, `-`, `@`, Tab oder Wagenrücklauf beginnen, erhalten ein führendes
`'`, damit eine Tabellenkalkulation sie nicht ausführt.

| Parameter | |
|---|---|
| `from`, `to` | Pflicht. Erster und letzter lokaler Tag. |
| `timezone` | IANA-Name; standardmäßig `UTC`. |
| `granularity` | `day` (Standard), `week` oder `month`. |
| `metrics` | Wiederholen oder mit Komma trennen; standardmäßig alle. |
| `group_by` | `inbox`, `channel`, `label`, `team` oder `member`. |
| `inbox_id`, `channel_kind`, `label_id`, `team_id`, `member_id` | Filter. |

## 7. MCP

Mit dem Scope `reports:read` kann ein [Assistent](mcp.md) `get_report` aufrufen; das Werkzeug nimmt
dieselben Parameter (`from` und `to` als `YYYY-MM-DD`) und antwortet mit denselben Zahlen. Fragen Sie zum
Beispiel: „Wie schnell haben wir letzten Monat je Posteingang geantwortet, und wer hat am meisten
geschlossen?“
