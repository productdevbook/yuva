# SLA-Richtlinien

Eine SLA-Richtlinie legt fest, wie schnell Ihr Team antwortet: eine Frist für die erste Antwort, für jede
weitere Antwort und für die Lösung einer Unterhaltung. Yuva zählt die Zeit in den Geschäftszeiten des
Posteingangs, zeigt zu jeder Unterhaltung Fälligkeit und Status, warnt, bevor ein Ziel verfehlt wird, und
meldet es, wenn es verfehlt wurde.

## 1. Eine Richtlinie anlegen

Richtlinien legen Inhaber und Admins an oder ein Schlüssel mit `inboxes:manage`, der nicht auf Posteingänge
beschränkt ist:

```sh
curl -X POST https://support.example.com/v1/sla-policies \
  -H "Authorization: Bearer $YUVA_API_KEY" -H "Content-Type: application/json" \
  -d '{"name": "Urgent", "priorities": ["urgent", "high"],
       "first_reply_minutes": 30, "next_reply_minutes": 120, "resolution_minutes": 1440,
       "warn_minutes": 15}'
```

Die Antwort ist die Richtlinie, `201`. Setzen Sie mindestens eines der drei Ziele; ein Ziel ohne Minuten
wird nicht verfolgt. Minuten liegen zwischen 1 und 525600. `warn_minutes` liegt zwischen 0 und 1440
(Standard 15; `0` sendet keine Warnung).

| Feld | Bedeutung |
|---|---|
| `name` | 1 bis 100 Zeichen. |
| `enabled` | Standard `true`. Eine deaktivierte Richtlinie gilt nie. |
| `inbox_ids`, `channel_kinds`, `label_ids`, `priorities` | Bedingungen. Alle müssen zutreffen; innerhalb einer Liste passt jeder Eintrag; eine leere Liste passt auf alles. `channel_kinds` sind `email`, `chat`, `app` und `api`. |
| `first_reply_minutes`, `next_reply_minutes`, `resolution_minutes` | Die Ziele in Geschäftsminuten. |
| `warn_minutes` | Wie lange vor der Fälligkeit die Warnung gesendet wird. |

`PATCH /v1/sla-policies/{slaPolicyId}` ändert, was Sie senden; `null` beendet die Verfolgung eines Ziels,
und eines muss bleiben. Ein Arbeitsbereich enthält höchstens 100 Richtlinien (`403 sla_policy_limit`).

| Aufruf | Zweck |
|---|---|
| `GET /v1/sla-policies` | Listet die Richtlinien der Reihe nach: `{"items": [...]}`. |
| `POST /v1/sla-policies` | Legt eine an; sie kommt ans Ende. |
| `GET`, `PATCH`, `DELETE /v1/sla-policies/{slaPolicyId}` | Liest, ändert oder löscht eine. |
| `PUT /v1/sla-policies/order` | Legt die Reihenfolge fest: `{"ids": [...]}` nennt jede Richtlinie einmal. |

Lesen braucht `inboxes:read`, Schreiben und Sortieren `inboxes:manage`. Agenten und auf einige Posteingänge
beschränkte Schlüssel dürfen lesen, sehen aber nur die Richtlinien ohne `inbox_ids` und die, die einen ihrer
Posteingänge nennen (alle anderen sind `404`, und `inbox_ids` listet nur, was sie sehen). Schreiben oder
Sortieren dürfen sie nicht: `403 forbidden`.

## 2. Welche Richtlinie gilt

Es gilt die erste aktivierte Richtlinie der Reihenfolge, deren Bedingungen auf die Unterhaltung passen.
Ändern sich Priorität, Labels oder Posteingang, wird die Richtlinie neu gewählt. Ein Ziel, das schon
erreicht oder verfehlt wurde, behält sein Ergebnis.

Eine neue Richtlinie gilt für Unterhaltungen ohne SLA bei ihrer nächsten Änderung oder Nachricht. Sie
startet nie ein Ziel vor dem Anlegen der Richtlinie.

## 3. Ziele

| Ziel | Beginnt | Erreicht durch |
|---|---|---|
| `first_reply` | Die erste Nachricht des Kontakts. | Eine öffentliche Antwort eines Mitglieds oder eines Bots. |
| `next_reply` | Eine Kontaktnachricht nach einer Antwort. | Die nächste öffentliche Antwort. |
| `resolution` | Die Unterhaltung wird angelegt. | Das erste Schließen. |

Notizen, Entwürfe und automatische Antworten erreichen nie ein Ziel. Schließen beendet ein noch laufendes
Antwortziel ohne Ergebnis. Eine wieder geöffnete Unterhaltung bekommt kein neues Lösungsziel; das erste
Schließen gilt. Eine Kontaktnachricht nach dem Schließen startet ein Ziel für die nächste Antwort.

## 4. Geschäftszeiten und Zeitzonen

Jedes Ziel zählt nur die Zeit, in der der Posteingang geöffnet ist, in dessen Zeitzone (`timezone` und
`business_hours` des Posteingangs). Sind die Geschäftszeiten aus oder ohne Intervalle eingeschaltet, zählt
jede Stunde. Zeitumstellungen werden berücksichtigt: Eine Uhrzeit, die es nicht gibt, rückt auf das Ende
des Sprungs vor.

## 5. Pausieren

`pending` und `snoozed` pausieren jedes laufende Ziel und merken sich die verbleibende Zeit; `open` setzt
mit dieser Zeit fort. Eine zurückgestellte Unterhaltung öffnet sich zu `snooze_until` von selbst, sendet
`conversation.updated` und setzt die Uhr fort. Eine Spam- oder gelöschte Unterhaltung bleibt zurückgestellt.

## 6. Wenn eine Richtlinie nicht mehr gilt

Spam, der Papierkorb, eine Richtlinie, die ausgeschaltet, gelöscht wird oder nicht mehr passt, und eine
Änderung, die ihre Bedingungen einengt, entfernen das SLA: `sla` verschwindet von der Unterhaltung und
`conversation.updated` wird gesendet. Passt eine andere Richtlinie, übernimmt sie und behält die bereits
verbrauchte Geschäftszeit. Gilt später wieder eine Richtlinie, zählt die Zeit ohne Richtlinie nicht, und
kein Ziel bekommt ein zweites Ergebnis.

Neue Minuten gelten für Ziele, die danach starten; laufende Ziele behalten ihre Fälligkeit. Wird ein Label
oder ein Posteingang gelöscht, verschwindet er aus den Richtlinien, die ihn nennen; wird deren Liste dadurch
leer, schaltet sich die Richtlinie aus, damit sie nicht auf alles passt.

## 7. Warnungen und Verstöße

- `warn_minutes` vor der Fälligkeit wird `conversation.sla_warning` gesendet, außer dieser Zeitpunkt ist
  beim Start oder Fortsetzen des Ziels schon vorbei.
- Zur Fälligkeit wird `conversation.sla_breached` gesendet. Ein verfehltes Ziel bleibt verfehlt, bis es
  erreicht wird (`met_at` hält die späte Antwort fest), die Unterhaltung pausiert oder geschlossen wird.
- Beides wird einmal je Ziel gesendet, gefolgt von `conversation.updated` mit dem neuen `sla`.

Beide gehen an den [Event-Feed und Realtime](headless.md#event-feed-und-realtime) und an
[Webhooks](webhooks.md#ereignisse). `data` ist `{target, due_at, conversation}`; der Webhook ergänzt
`contact`.

Mitglieder werden außerdem benachrichtigt, über zwei Arten in ihren [Benachrichtigungseinstellungen](#mitglieder-benachrichtigungen).

### Mitglieder-Benachrichtigungen

| Art | Standard | Gesendet, wenn |
|---|---|---|
| `sla_warning` | Push | Ein Ziel bald fällig ist. |
| `sla_breached` | Push und sofort E-Mail | Ein Ziel verfehlt wurde. |

Benachrichtigt wird das zugewiesene Mitglied; ohne eines die Mitglieder des Teams der Unterhaltung; ohne
beides niemand. Das Mitglied muss weiterhin Zugriff auf den Posteingang haben. Ein Mitglied mit Status
`away` wird für eigene Unterhaltungen weiter benachrichtigt, nicht für die seines Teams. Mitglieder ändern
die Arten mit `PATCH /v1/me/notifications` (nur Mitgliedssitzungen).

## 8. Den Status lesen

Eine Unterhaltung trägt `sla`, wenn eine Richtlinie gilt:

```json
{"policy_id": "…", "state": "due_soon", "due_at": "2026-03-04T10:30:00Z",
 "targets": [{"target": "first_reply", "state": "due_soon", "started_at": "2026-03-04T09:45:00Z",
              "due_at": "2026-03-04T10:30:00Z", "warned_at": "2026-03-04T10:15:00Z"}]}
```

| Feld | Bedeutung |
|---|---|
| `state` | `on_track`, `due_soon` (das früheste laufende Ziel ist innerhalb von `warn_minutes` fällig), `breached`, `met` oder `paused`. |
| `due_at` | Die früheste Fälligkeit eines laufenden Ziels; fehlt, wenn keines läuft. |
| `targets[].target` | `first_reply`, `next_reply` oder `resolution`. Nur das letzte Ziel für die nächste Antwort bleibt erhalten. |
| `targets[].state` | `on_track`, `due_soon`, `breached`, `met`, `paused` oder `stopped` (vor einer Antwort geschlossen). |
| `targets[]` | Außerdem `started_at`, `due_at` während es läuft, `remaining_seconds` während der Pause, `met_at`, `breached_at` und `warned_at`. |

`GET /v1/conversations` nimmt `sort=sla_due`: Unterhaltungen mit laufendem Ziel zuerst, das früheste
`sla.due_at` voran (verfehlte vorne), danach der Rest nach letzter Aktivität. Ein Cursor setzt nur die
Sortierung fort, aus der er stammt. `sla_state` behält Unterhaltungen in einem oder mehreren Status
(wiederholen oder mit Komma trennen); Unterhaltungen ohne SLA passen nie.

```sh
curl -G https://support.example.com/v1/conversations \
  -H "Authorization: Bearer $YUVA_API_KEY" \
  --data-urlencode "sort=sla_due" --data-urlencode "sla_state=breached,due_soon"
```

`sla` in `GET /v1/stats` liefert die erreichten (`met`) und verfehlten (`breached`) Ziele im Zeitraum, gesamt,
je Posteingang (`inboxes`) und je Mitglied (`members`); das Mitglied ist das zugewiesene zum Zeitpunkt des
Ergebnisses. Nicht zugewiesene Ergebnisse zählen nur in der Summe und je Posteingang.

## Assistenten

[MCP](mcp.md) `search_conversations` nimmt `sla_state` und `sort`; es und `get_conversation` liefern `sla`.
Kein Werkzeug verwaltet Richtlinien.
