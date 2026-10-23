# Automatische Zuweisung

Die automatische Zuweisung gibt eine Unterhaltung ohne Mitglied an jemanden, der verfügbar ist, damit
nichts in der Liste der nicht zugewiesenen Unterhaltungen wartet, solange das Team online ist. Sie arbeitet
pro Posteingang oder [Team](headless.md#teams), reihum (Round Robin) oder nach den wenigsten offenen
Unterhaltungen, auf Wunsch mit einer Obergrenze pro Mitglied.

## 1. Einschalten

Setzen Sie `assignment_mode` an einem Posteingang oder einem Team. Beide nehmen auch `assignment_cap` an.

```sh
curl -X PATCH https://support.example.com/v1/inboxes/$INBOX \
  -H "Authorization: Bearer $YUVA_API_KEY" -H "Content-Type: application/json" \
  -d '{"assignment_mode": "least_open", "assignment_cap": 20}'
```

| Feld | Werte |
|---|---|
| `assignment_mode` | `off` (Standard), `round_robin` oder `least_open`. |
| `assignment_cap` | 1 bis 500, oder das Feld fehlt, wenn es keine Grenze gibt. `null` bei `PATCH` entfernt sie. |

Die Felder gehören zu `Inbox`, `InboxCreate`, `InboxUpdate`, `Team`, `TeamCreate` und `TeamUpdate`
(`PATCH /v1/inboxes/{inboxId}`, `PATCH /v1/teams/{teamId}` und die zugehörigen `POST`-Aufrufe). Zum Schreiben
braucht es dieselbe Berechtigung wie für die übrigen Felder des Posteingangs oder Teams (`inboxes:manage`);
ein unbekannter Modus oder eine Grenze außerhalb von 1 bis 500 ergibt `400 validation_failed`.

- `round_robin` gibt die Unterhaltung an das geeignete Mitglied, das am längsten keine erhalten hat; wer noch
  nie eine erhielt, kommt zuerst.
- `least_open` gibt sie an das geeignete Mitglied mit den wenigsten offenen Unterhaltungen, bei Gleichstand an
  das, das am längsten keine erhalten hat.

Die Grenze ist die höchste Zahl offener Unterhaltungen, die ein Mitglied halten darf. Gezählt werden ihm
zugewiesene Unterhaltungen, die `open`, nicht Spam und nicht im Papierkorb sind, im ganzen Workspace.
Zuweisen von Hand ignoriert sie.

## 2. Wer geeignet ist

Ein Mitglied ist für eine Unterhaltung geeignet, wenn alles Folgende zutrifft:

- es hat Zugriff auf den Posteingang der Unterhaltung über eine eigene Freigabe oder die eines Teams;
  Inhaber und Admins brauchen ebenfalls eine Freigabe
- das Panel oder eine App ist für es verbunden
- seine `availability` ist `auto` (`PATCH /v1/me` setzt `auto` oder `away`)
- der Posteingang liegt innerhalb seiner Geschäftszeiten
- es liegt unter der Grenze des Pools
- bei einem Team-Pool ist es Mitglied des Teams

## 3. Welcher Pool

Eine Unterhaltung mit Team und ohne Mitglied nutzt Einstellungen und Mitglieder des Teams. Eine mit keinem
von beiden nutzt die des Posteingangs. Ist der Modus des Teams `off`, wird nichts zugewiesen; die
Einstellungen des Posteingangs springen nicht ein. Eine Unterhaltung, die schon ein Mitglied hat, wird nie
angefasst, außer wenn sie wieder geöffnet wird (unten).

Betrachtet werden Unterhaltungen, wenn sie entstehen und wenn ein Kontakt sie wieder öffnet. Eine von Hand
aus der Zuweisung genommene Unterhaltung bleibt unzugewiesen.

## 4. Wenn niemand verfügbar ist

Die Unterhaltung wartet. Sie erhält das Feld `assign_waiting_since` (ein
Zeitpunkt; sonst fehlt es), und ein `conversation.updated` wird gesendet. Sobald jemand geeignet ist, wird
sie zugewiesen, die älteste zuerst: ein Mitglied verbindet sich oder stellt `availability` auf `auto`, ein
Mitglied wird frei, weil es eine Unterhaltung schließt oder weitergibt, die Grenze wird erhöht, ein Pool wird
eingeschaltet oder die Geschäftszeiten des Posteingangs beginnen. Yuva prüft außerdem jede Minute, solange
jemand online ist. Das Feld verschwindet, wenn die Unterhaltung ein Mitglied bekommt, nicht mehr `open` ist,
als Spam markiert oder in den Papierkorb gelegt wird.

## 5. Wieder geöffnete Unterhaltungen

Schreibt ein Kontakt in eine geschlossene, ausstehende oder zurückgestellte Unterhaltung, oder läuft eine
Zurückstellung ab, wird sie wieder geöffnet. Das Mitglied behält sie, solange es noch Zugriff auf den
Posteingang hat und nicht `away` ist; offline zu sein oder über der Grenze zu liegen nimmt sie nicht weg.
Sonst wird die Zuweisung aufgehoben und die Unterhaltung geht an den Pool, oder sie wartet, wenn niemand
geeignet ist. Posteingänge und Teams mit Modus `off` behalten das bisherige Verhalten.

## 6. Ereignisse

Die Zuweisung wird als übliches Zeitleisten-Ereignis `assigned` oder `unassigned` mit dem Autor `system`
geschrieben. Das Ereignis hat `auto: true` und, falls es ein früheres Mitglied gab, `previous_assignee_id`.
Die zugewiesene Person erhält die Benachrichtigung `assigned_to_me`; `conversation.updated` und
[Webhooks](webhooks.md#ereignisse) folgen wie bei einer Zuweisung von Hand.

## 7. Automatisierungsregeln

[Automatisierungsregeln](automation.md) laufen zuerst. Die Aktion `assign_pool` entfernt das Mitglied der
Unterhaltung und übergibt sie, nachdem alle Regeln des Auslösers gelaufen sind, an die automatische
Zuweisung. Eine Regel, die ein Mitglied zuweist, die Zuweisung aufhebt (`unassign`) oder `assign_pool`
nutzt, verhindert auch, dass eine wieder geöffnete Unterhaltung ihr altes Mitglied behält.
