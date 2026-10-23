# Automatisierungsregeln

Eine Automatisierungsregel läuft von selbst, wenn in einer Unterhaltung etwas passiert: Sie prüft Bedingungen
und vergibt dann Labels, weist zu, antwortet oder ruft einen Webhook auf. Regeln werden über die API unter
`/v1/automation-rules` verwaltet; alle Beispiele nutzen einen [API-Schlüssel](headless.md#scopes).

## 1. Eine Regel anlegen

Regeln legen Inhaber, Admins und Schlüssel an, die nicht auf Posteingänge beschränkt sind. Diese Regel vergibt
ein Label und sendet eine Textvorlage an jede neue E-Mail-Unterhaltung außerhalb der Geschäftszeiten:

```sh
curl -X POST https://support.example.com/v1/automation-rules \
  -H "Authorization: Bearer $YUVA_API_KEY" -H "Content-Type: application/json" \
  -d '{"name": "After hours",
       "trigger": "conversation_created",
       "conditions": [
         {"field": "channel_kind", "operator": "in", "values": ["email"]},
         {"field": "business_hours", "operator": "equals", "value": "outside"}
       ],
       "actions": [
         {"type": "add_labels", "label_ids": ["<label id>"]},
         {"type": "send_reply", "canned_reply_id": "<canned reply id>"}
       ]}'
```

Die Antwort ist die Regel, `201`. Sie steht ganz hinten in der Reihenfolge und reagiert auf alles, was nach dem
Speichern passiert. Jedes Label, Mitglied, Team, jede Textvorlage, jeder Webhook-Endpunkt, jedes benutzerdefinierte Feld
und jeder Posteingang, den eine Regel nennt, muss existieren (`400 validation_failed`). Ein Workspace hält höchstens
100 Regeln (`403 automation_rule_limit`).

| Feld | Bedeutung |
|---|---|
| `name` | 1 bis 100 Zeichen. |
| `enabled` | Standardmäßig `true`. |
| `inbox_id` | Weglassen für eine Regel auf allen Posteingängen; setzen, damit sie nur auf Unterhaltungen dieses Posteingangs wirkt. |
| `trigger` | Wann die Regel geprüft wird, siehe [Auslöser](#2-auslöser). |
| `idle_minutes` | 5 bis 43200. Pflicht bei `idle`, bei anderen Auslösern abgelehnt. |
| `conditions` | Bis zu 20; alle müssen zutreffen. Eine leere Liste trifft auf jede Unterhaltung zu. |
| `actions` | 1 bis 10. |

`PATCH /v1/automation-rules/{automationRuleId}` ändert, was Sie senden; `conditions` und `actions` werden ganz
ersetzt, und `inbox_id` oder `idle_minutes` mit `null` werden geleert.

| Aufruf | Bewirkt |
|---|---|
| `GET /v1/automation-rules` | Listet die Regeln der Reihe nach: `{"items": [...]}`. |
| `POST /v1/automation-rules` | Legt eine an. |
| `GET`, `PATCH`, `DELETE /v1/automation-rules/{automationRuleId}` | Liest, ändert oder löscht eine. Löschen entfernt auch ihr Protokoll; was sie bereits getan hat, bleibt. |
| `PUT /v1/automation-rules/order` | Legt die Reihenfolge fest: `{"ids": [...]}` nennt jede Regel einmal. |
| `GET /v1/automation-rules/{automationRuleId}/runs` | Das [Protokoll](#7-das-ausführungsprotokoll) der Regel. |

## 2. Auslöser

| Auslöser | Wird ausgelöst, wenn |
|---|---|
| `conversation_created` | Eine Unterhaltung angelegt wurde, von einem Kontakt, einem Mitglied oder einem Schlüssel. |
| `message_received` | Der Kontakt eine Nachricht geschrieben hat. Importierte Nachrichten und Entwürfe zählen nicht. |
| `status_changed` | Der Status sich geändert hat, auch wenn die Nachricht eines Kontakts oder das Ende einer Zurückstellung ihn wieder geöffnet hat. |
| `idle` | In einer offenen oder wartenden Unterhaltung `idle_minutes` lang nichts geschrieben wurde. |

`idle` wird alle 5 Minuten geprüft, die Regel läuft also höchstens ein paar Minuten zu spät und einmal je ruhiger
Phase: Die nächste beginnt erst, wenn jemand schreibt. Eine Notiz oder Antwort der Regel selbst beginnt keine neue
ruhige Phase. Eine Unterhaltung, die ruhig wurde, während der Server länger als 15 Minuten stand, wird nicht erfasst.
Spam, Papierkorb und geschlossene Unterhaltungen gelten nie als ruhig, und eine Unterhaltung, in die vor dem Lauf der
Regel geschrieben wurde oder die geschlossen wurde, bleibt unberührt.

## 3. Bedingungen

Eine Bedingung hat ein `field`, einen `operator` und den Vergleichswert: `values` bei `in` und `not_in`, `value` bei
`equals`, `not_equals`, `contains` und `not_contains`, nichts bei `is_set` und `is_not_set`.

| Feld | Operatoren | Vergleicht |
|---|---|---|
| `channel_kind` | `in`, `not_in` | `email`, `chat`, `app` oder `api`. |
| `kind` | `in`, `not_in` | Die Art der Unterhaltung. |
| `feedback_category` | `in`, `not_in` | Kategorie-Schlüssel von Feedback. |
| `priority`, `status` | `in`, `not_in` | Priorität oder Status der Unterhaltung. |
| `subject`, `body` | `contains`, `not_contains` | Ohne Beachtung der Groß-/Kleinschreibung. `body` ist die Nachricht, die die Regel ausgelöst hat, bei anderen Auslösern die letzte Nachricht des Kontakts. |
| `contact_attribute` | `equals`, `not_equals`, `contains`, `not_contains`, `is_set`, `is_not_set` | Das in `key` genannte Kontaktattribut. |
| `custom_field` | gleich | Das [benutzerdefinierte Feld](custom-fields.md) der Unterhaltung mit diesem `key`. Zahlen, Kontrollkästchen und Daten werden als JSON-Text verglichen (`42`, `true`, `2026-01-31`). |
| `label` | `in`, `not_in` | `in`: hat eines der Labels. `not_in`: hat keines davon. |
| `team` | `in`, `not_in`, `is_set`, `is_not_set` | Team-IDs. |
| `assignee` | `is_set`, `is_not_set` | Ob ein Mitglied zugewiesen ist. |
| `business_hours` | `equals` | `inside` oder `outside` der Geschäftszeiten des Posteingangs. Ein Posteingang ohne Zeiten ist immer innerhalb. |

`equals` vergleicht exakt; `contains` ignoriert die Schreibweise. Bedingungen werden auf die Unterhaltung geprüft,
wie sie beim Lauf der Regel ist; eine Regel sieht also, was frühere Regeln geändert haben.

## 4. Aktionen

| Aktion | Felder | Bewirkt |
|---|---|---|
| `add_labels`, `remove_labels` | `label_ids` (bis zu 50) | Fügt Labels hinzu oder entfernt sie. Sie dürfen kein Label teilen. |
| `assign_member` | `member_id` | Weist ein Mitglied zu, das Zugriff auf den Posteingang hat. |
| `assign_team` | `team_id` | Weist ein Team zu, dem der Posteingang freigegeben ist. |
| `unassign` | keine | Entfernt das zugewiesene Mitglied. |
| `assign_pool` | keine | Entfernt das zugewiesene Mitglied; nachdem die Regeln des Auslösers gelaufen sind, wählt die [automatische Zuweisung](assignment.md) jemanden aus dem Pool. |
| `set_priority` | `priority` | Setzt die Priorität. |
| `set_status` | `status` | `open`, `pending` oder `closed`. Zum Schließen braucht es die Pflichtfelder der Unterhaltung. |
| `snooze` | `snooze_minutes` (1 bis 525600) | Stellt die Unterhaltung zurück. |
| `add_note` | `body` (bis zu 10000 Zeichen) | Fügt eine interne Notiz hinzu. |
| `send_reply` | `canned_reply_id` | Sendet den Text der Textvorlage an den Kontakt, siehe [Antworten](#6-antworten). |
| `send_webhook` | `webhook_id` | Sendet [`automation.triggered`](#8-webhook-aktion) an diesen Endpunkt. |

Jeder Typ kommt in einer Regel höchstens einmal vor, nur mit seinen eigenen Feldern (sonst `400 validation_failed`).
`set_status` und `snooze` schließen sich aus, ebenso `assign_member`, `assign_pool` und `unassign`.

Innerhalb einer Regel laufen die Aktionen in fester Reihenfolge, gleich wie Sie sie auflisten: Notiz, Antwort, dann
Labels, Zuweisung, Team, Priorität und Status oder Zurückstellung gemeinsam, zuletzt der Webhook. Alles, was eine
Regel an einer Unterhaltung tut, steht im Verlauf mit dem Autor `system`, und jede Nachricht trägt `rule` mit `id` und
`name` der Regel zu diesem Zeitpunkt. Ein Lauf fügt außerdem ein `rule_applied`-Ereignis mit den Aktionen und ihren
Ergebnissen hinzu.

## 5. Reihenfolge und Schleifen

Regeln stehen in einer Liste, geordnet nach `position`; Workspace- und Posteingangsregeln teilen sie. Für einen
Auslöser laufen die aktiven, passenden Regeln in dieser Reihenfolge, jede höchstens einmal je Auslöser, auch wenn der
Server es wiederholt: Eine Regel, die schon gelaufen ist, ob zutreffend oder nicht, läuft für ihn nie wieder.

Die eigenen Änderungen einer Regel lösen nie eine Regel aus, Regeln können sich also nicht im Kreis drehen: Ein von
ihr vergebenes Label, ein gesetzter Status oder eine geschriebene Nachricht startet nichts. Das gilt auch für eine
von einer Regel gesetzte Zurückstellung: Wenn sie endet und die Unterhaltung wieder öffnet, läuft für das Aufwecken
keine Regel.

## 6. Antworten

`send_reply` sendet den Text der Textvorlage als automatische Antwort (per E-Mail mit `Auto-Submitted: auto-replied`).
Sie wird abgelehnt, und der Lauf schlägt mit `error: auto_reply` fehl, wenn:

- die auslösende Nachricht eine automatische Antwort oder ein Bounce ist;
- die auslösende Nachricht eine E-Mail ist, die DMARC nicht bestanden hat (Chat- und API-Nachrichten sind nicht betroffen);
- der Kontakt gesperrt ist;
- die Unterhaltung Spam ist;
- eine Regel in dieser Unterhaltung schon innerhalb der letzten Stunde oder dreimal in den letzten 24 Stunden geantwortet hat.

Ob eine Regel Kunden anschreiben darf, folgt außerdem der Workspace-Einstellung `bots_may_send`
([Bots may send](headless.md#bots-may-send)). Solange sie aus ist, geht eine Antwort nur hinaus, wenn Regel und
Textvorlage zuletzt von einem Mitglied gespeichert wurden, das beim Lauf der Regel noch Inhaber oder Admin ist;
sonst schlägt der Lauf mit `bot_sending_disabled` fehl, und `detail` nennt, was fehlt.

Beide tragen `saved_by` (`type` und `member_id` oder `api_key_id`):

- `saved_by` einer Regel ist, wer sie angelegt oder zuletzt `trigger`, `idle_minutes`, `inbox_id`, `conditions`,
  `actions` oder `enabled` geändert hat. Umbenennen und Umsortieren ändern es nicht.
- `saved_by` einer Textvorlage ist, wer sie angelegt oder ihren Text zuletzt geändert hat. Bei Antworten, die vor der
  Erfassung gespeichert wurden, fehlt es; ein Inhaber oder Admin muss sie erneut speichern, bevor eine Regel sie
  senden kann, solange `bots_may_send` aus ist.
- API-Schlüssel und verbundene Apps gelten als Bots, auch wenn ein Inhaber sie erstellt hat.

## 7. Das Ausführungsprotokoll

Jedes Mal, wenn eine Regel für einen Auslöser geprüft wird, hinterlässt sie einen Lauf, 30 Tage aufbewahrt:

```sh
curl -H "Authorization: Bearer $YUVA_API_KEY" \
  "https://support.example.com/v1/automation-rules/$RULE_ID/runs?outcome=failed"
```

Neueste zuerst, mit `limit` und `next_cursor` zum Blättern. `outcome` filtert nach einem Wert.

| Ergebnis | Bedeutung |
|---|---|
| `applied` | Die Bedingungen trafen zu und die Aktionen wurden angewendet. |
| `failed` | Eine Aktion ließ sich nicht ausführen. Von dieser Regel wurde nichts angewendet. `error` enthält den Problemcode, `detail` einen Satz. |
| `not_matched` | Die Bedingungen trafen nicht zu. Keine Aktionen. |

Jeder Lauf hat `conversation_id`, `trigger` und `actions`: je Aktion ein `type` und `result`. Ein Ergebnis ist
`applied`, `unchanged` (die Unterhaltung war schon so), `skipped` (nur in alten Läufen) oder `not_run` (der Lauf schlug fehl).

Häufige `error`-Werte:

| Fehler | Grund |
|---|---|
| `auto_reply` | Eine Antwort wurde zum Schutz des Kunden abgelehnt, siehe [Antworten](#6-antworten). |
| `bot_sending_disabled` | `bots_may_send` ist aus und Regel oder Textvorlage wurden nicht zuletzt von einem Inhaber oder Admin gespeichert. |
| `field_required` | Zum Schließen fehlt ein Wert in einem benutzerdefinierten Feld. |
| `team_access`, `validation_failed` | Das Team hat keinen Zugriff auf den Posteingang, oder das Mitglied hat keinen. |
| `email_undeliverable`, `email_not_configured` | Die Antwort ließ sich nicht per E-Mail senden. |
| `label_gone`, `canned_reply_gone`, `webhook_unavailable` | Das von der Aktion Genannte wurde gelöscht, oder der Endpunkt ist deaktiviert oder auf einen anderen Posteingang beschränkt. |

Eine Regel, die ein gelöschtes Label, Mitglied, Team, eine gelöschte Textvorlage oder einen gelöschten Endpunkt nennt,
wird nicht verändert: Die Aktionen, die es brauchen, schlagen fehl. Eine Bedingung auf ein gelöschtes Label oder
Team trifft nie zu (`in`) oder immer zu (`not_in`). Das Löschen eines Posteingangs löscht dessen Regeln.

## 8. Webhook-Aktion

`send_webhook` sendet ein `automation.triggered`-Ereignis an den genannten [Webhook-Endpunkt](webhooks.md), signiert
wie jede andere Zustellung. Der Endpunkt führt es nicht in `events` (dort wird es abgelehnt) und erhält es unabhängig
von seinen `events`, außer er ist deaktiviert oder auf einen anderen Posteingang beschränkt. Regeln speichern keine
URLs oder Geheimnisse. `data` enthält `rule` (`id`, `name`), `trigger`, `conversation` im Zustand nach den übrigen
Aktionen der Regel und `contact`.

## 9. Ratenlimit

Die Regeln eines Workspace wenden höchstens 600 Aktionen pro Minute an. Würde eine Regel das Limit überschreiten,
schlägt sie nicht fehl, sondern wartet auf die nächste Minute; spätere Regeln warten mit. Regeln, deren Bedingungen
nicht zutreffen, kosten nichts.

## 10. Zugriff

| Scope | Erlaubt |
|---|---|
| `automations:read` | Regeln und ihre Protokolle lesen. |
| `automations:manage` | Regeln anlegen, ändern, sortieren und löschen. Nur Inhaber und Admins; bei auf Posteingänge beschränkten Schlüsseln abgelehnt; in keinem anderen Scope enthalten. |

Agents und auf einzelne Posteingänge beschränkte Schlüssel lesen die Workspace-Regeln und die Regeln ihrer
Posteingänge (andere sind `404`) und sehen nur Läufe auf Unterhaltungen ihrer Posteingänge. Schreiben können sie
nicht: `403 forbidden`. Da eine Regel in jedem Posteingang, den sie abdeckt, mit der Macht des Workspace handelt,
vergeben Sie `automations:manage` mit Bedacht.

Das Anlegen, Ändern und Löschen einer Regel steht im [Audit-Log](operations.md#audit-log) als
`automation_rule.created`, `automation_rule.updated` und `automation_rule.deleted`, mit Name, Auslöser, Posteingang,
Aktionstypen und den Hosts ihrer Webhook-Endpunkte, nie deren URLs.

Automatisierungsregeln stehen [MCP](mcp.md)-Assistenten absichtlich nicht zur Verfügung.
