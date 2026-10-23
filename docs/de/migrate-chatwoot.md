# Von Chatwoot umziehen

Kopieren Sie die Unterhaltungen eines Chatwoot-Kontos nach Yuva und leiten Sie Ihre Support-Adresse an Yuva
weiter. Yuva liest Chatwoot nur. Die Übersicht steht unter [Zu Yuva umziehen](migrate.md). Das gilt für
Chatwoot Cloud und für ein Chatwoot, das Sie selbst betreiben.

## Bevor Sie anfangen

- Nehmen Sie einen Administrator. Die API von Chatwoot zeigt einem Agenten nur die Posteingänge, in denen
  er Mitglied ist; Yuva braucht deshalb den Zugriffstoken eines Benutzers, der **Administrator** des Kontos
  ist. Den Token eines Agenten lehnt Yuva ab.
- Kopieren Sie den Zugriffstoken. Öffnen Sie in Chatwoot **Profile Settings** (Ihr Avatar unten links),
  scrollen Sie zu **Access Token** und kopieren Sie ihn. Seite von Chatwoot:
  [API](https://developers.chatwoot.com/api-reference/introduction).
- Suchen Sie die Kontonummer. Es ist die Zahl nach `/app/accounts/` in der Adressleiste, während Sie in
  Chatwoot sind, etwa `1` in `https://app.chatwoot.com/app/accounts/1/dashboard`.
- Geben Sie im Schritt Connect die **Adresse** Ihres Chatwoot (`https://app.chatwoot.com` oder die
  HTTPS-Adresse Ihrer eigenen Installation), die **Kontonummer** und den **Zugriffstoken** ein und drücken
  Sie **Verbindung testen**. Die Adresse einer eigenen Installation muss aus dem Internet erreichbar sein;
  ein Server auf einer privaten Adresse wird abgelehnt, außer der Betreiber von Yuva hat ihn erlaubt.

Chatwoot hat keinen schreibgeschützten Token, er könnte also Daten ändern. Die Verbindung von Yuva hält ihn
schreibgeschützt: Sie sendet Chatwoot nur lesende Anfragen und den Token nur an die Adresse, die Sie
eingegeben haben.

## Was kopiert wird

| Chatwoot | Yuva |
|---|---|
| Unterhaltungen | Unterhaltungen mit allen Nachrichten |
| Eingehende Nachrichten | Eingehende Nachrichten |
| Ausgehende Nachrichten | Ausgehende Nachrichten |
| Private Notizen | Notizen, die nur Ihr Team sieht |
| Aktivitätszeilen (zugewiesen, gelöst, Label hinzugefügt) | Nichts |
| Status: open | Offen |
| Status: pending und snoozed | Wartend |
| Status: resolved | Geschlossen |
| Labels der Unterhaltung | Labels, wie Sie sie zuordnen (oder keine) |
| Agenten | Teammitglieder, wie Sie sie zuordnen; andere bleiben mit Namen erhalten |
| Zuständiger | Der Zuständige, wenn der Agent zugeordnet ist |
| Kontakte | Kontakte mit Name, E-Mail-Adresse und Telefon |
| Posteingänge | Posteingänge, je Chatwoot-Posteingang einer, wie Sie sie zuordnen |
| Anhänge | Dateien an den Nachrichten |

Unterhaltungen behalten ihre ursprünglichen Zeiten und kommen [still](migrate.md#wie-der-verlauf-still-bleibt) an.

## Was nicht kopiert wird

Textbausteine, Automatisierungsregeln, Makros, Teams, eigene Attribute, Zufriedenheitsbewertungen, das
Help Center und Berichte. Yuva kopiert nur, was die Tabelle oben aufführt.

## Anhänge

Chatwoot gibt jedem Anhang einen Link auf seinen eigenen Speicher (Ihre Installation oder der Speicherdienst
dahinter). Yuva holt die Datei ohne den Token von diesem Link. Eine Datei, die größer als das Anhanglimit
des Servers ist, bleibt zurück.

## Limits

Chatwoot meldet kein Limit. Es drosselt Anfragen je IP-Adresse, mit einem Wert, den der Betreiber einer
Installation festlegt und den Chatwoot Cloud nicht veröffentlicht. Yuva sendet höchstens 60 Anfragen pro
Minute, wartet so lange, wie Chatwoot es mit `429 Too Many Requests` und einem `Retry-After`-Header
verlangt, und ohne Header eine Minute. Ein großes Konto braucht deshalb Zeit: Jede Unterhaltung braucht
mindestens eine Anfrage für ihre Nachrichten und je zwanzig Nachrichten eine weitere.

## Die Umstellung

Heute erreicht Ihre Support-Adresse Chatwoot, über einen E-Mail-Posteingang dort oder eine Weiterleitung
beim Mailanbieter, der sie hostet. Kopieren Sie im Schritt Switch die
[Weiterleitungsadresse](email.md#weiterleitungsadresse) des Kanals und ändern Sie die Weiterleitung so,
dass die Support-Adresse stattdessen Yuva erreicht. Bis dahin liest Yuva Chatwoot alle zwei Minuten, sodass
keine Unterhaltung verloren geht. Antworten aus Yuva gehen von der Adresse des Kanals ab.

Drücken Sie dann „Send a test message“: Es funktioniert, wenn die Nachricht auf dem neuen Weg Yuva erreicht.
Nach einer ruhigen Phase oder mit „Finish now“ endet der Umzug
([so funktioniert es](migrate.md#testnachricht-ruhige-phase-und-abschluss)).

## Nach dem Abschluss

Setzen Sie Ihren Zugriffstoken zurück: in Chatwoot unter **Profile Settings → Access Token → Reset**. Yuva
löscht seine Kopie, wenn der Umzug endet, kann sie in Chatwoot aber nicht widerrufen; das Zurücksetzen
beendet den Zugriff. Lassen Sie die Weiterleitung, wie sie ist, und behalten Sie Chatwoot gern als Archiv.
