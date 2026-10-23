# Von Freshdesk umziehen

Kopieren Sie die Tickets eines Freshdesk-Kontos nach Yuva und leiten Sie seine Support-Adresse an Yuva um.
Yuva liest Freshdesk nur. Die Übersicht steht unter [Zu Yuva umziehen](migrate.md).

## Bevor Sie anfangen

- Suchen Sie Ihren API-Schlüssel. Melden Sie sich in Freshdesk als Admin an, klicken Sie oben rechts auf Ihr
  Profilbild und wählen Sie **Profile settings**. Der Schlüssel steht rechts unter **Your API Key**.
  Freshdesks Seite: [Authentication](https://developers.freshdesk.com/api/#authentication).
- Geben Sie im Schritt Connect den **API-Schlüssel** und Ihre **Subdomain** ein, das `acme` von
  `acme.freshdesk.com`, und drücken Sie **Test connection**. Nehmen Sie den Schlüssel eines Admins: Der
  Schlüssel eines Agenten wird abgelehnt, weil nur Admins die Produkte und Rollen des Kontos lesen können.

Freshdesk kennt weder Scopes noch einen reinen Lese-Schlüssel: Der Schlüssel kann alles, was der Admin kann.
Schreibgeschützt hält ihn Yuvas Verbindung: Sie sendet Freshdesk nur Leseanfragen, und zwar nur an die
`freshdesk.com`-Adresse Ihres Kontos, sodass sich kein Ticket, kein Kontakt und keine Einstellung ändert.

## Was kopiert wird

| Freshdesk | Yuva |
|---|---|
| Tickets | Unterhaltungen mit allen Nachrichten |
| Beschreibung eines Tickets | Die erste Nachricht: eingehend, oder ausgehend bei einem Ticket, das ein Agent per ausgehender E-Mail angelegt hat |
| Kundenantworten | Eingehende Nachrichten |
| Agentenantworten | Ausgehende Nachrichten |
| Private Notizen | Notizen, die nur Ihr Team sieht |
| Öffentliche Notizen eines Agenten | Notizen, die nur Ihr Team sieht |
| Öffentliche Notiz eines Kunden | Eine eingehende Nachricht |
| Status: open und eigene Status | Offen |
| Status: pending | Ausstehend |
| Status: resolved und closed | Geschlossen |
| Tags | Labels, wie Sie sie zuordnen (oder keine) |
| Agenten | Teammitglieder, wie Sie sie zuordnen; andere bleiben mit Namen erhalten |
| Responder | Die zuständige Person, wenn der Agent zugeordnet ist |
| Anfragende | Kontakte, mit Name und Telefon |
| Produkte | Posteingänge, je Produkt einer, wie Sie sie zuordnen |
| Tickets ohne Produkt | Der Standard-Posteingang, als Ordner „No product“ |
| Anhänge | Dateien an den Nachrichten |

Spam und gelöschte Tickets werden nie gelesen: Freshdesk lässt sie in seiner Liste weg, und Yuva fragt sie
nie an. Tickets behalten ihre ursprünglichen Zeiten und kommen [still](migrate.md#wie-der-verlauf-still-bleibt)
an, sodass keine importierte Notiz und keine importierte Antwort an jemanden gesendet wird.

## Was nicht kopiert wird

Automatisierungen, gespeicherte Antworten, Zufriedenheitsbewertungen, eigene Ticketfelder, Zeiteinträge,
Unternehmen, Gruppen, Rollen, Lösungsartikel und Berichte. Yuva kopiert nur, was die Tabelle oben nennt.

## Anhänge

Freshdesk bewahrt Anhänge an befristeten, signierten Adressen auf einem anderen Host auf. Yuva holt sie dort
ohne Ihren API-Schlüssel ab. Ist eine Adresse abgelaufen, liest Yuva das Ticket einmal erneut, um eine frische
zu bekommen; eine Datei, die weiter abgelehnt wird oder größer als das Anhang-Limit des Servers ist, bleibt
zurück.

## Limits

Freshdesk begrenzt Anfragen pro Minute je nach Tarif und zählt auch ungültige Anfragen. Yuva bleibt bei der
Hälfte des Limits, das Freshdesk meldet (50 pro Minute, bis es eines gemeldet hat), fragt die Ticketliste
höchstens 20-mal pro Minute ab und wartet so lange, wie Freshdesk bei 429 verlangt. Eine Anfrage, die
Freshdesk aus einem anderen Grund als einem fehlenden Ticket ablehnt, wird nicht wiederholt, sondern hält das
Kopieren eine Weile an. Ein großes Konto dauert deshalb eine Weile. Freshdesks Seite:
[Rate limit](https://developers.freshdesk.com/api/#ratelimit).

Freshdesk beendet eine Ticketliste nach 300 Seiten, also 30.000 Tickets. Yuva beginnt die nächste Liste bei
der Änderungszeit des zuletzt gelesenen Tickets, sodass kein Ticket doppelt gelesen oder übersehen wird.

## Die Umstellung

Heute erreicht Ihre Support-Adresse Freshdesk, weitergeleitet vom Mail-Anbieter, der sie hostet, oder durch
eine Regel dort. Kopieren Sie im Schritt Switch die [Weiterleitungsadresse](email.md#weiterleitungsadresse)
des Kanals und ändern Sie diese Weiterleitung so, dass die Support-Adresse stattdessen Yuva erreicht. Bis
dahin liest Yuva Freshdesk alle zwei Minuten, sodass kein Ticket verloren geht. Antworten aus Yuva gehen von
der Adresse des Kanals ab.

Drücken Sie dann „Send a test message“: Es funktioniert, wenn die Nachricht auf dem neuen Weg Yuva erreicht.
Nach einer ruhigen Phase oder mit „Finish now“ endet der Umzug
([so funktioniert es](migrate.md#testnachricht-ruhige-phase-und-abschluss)).

## Nach dem Abschluss

Setzen Sie Ihren API-Schlüssel zurück: in Freshdesk unter **Profile settings** mit **Reset API key**. Yuva
löscht seine Kopie des Schlüssels, wenn der Umzug endet, kann ihn in Freshdesk aber nicht zurücksetzen; den
Zugriff beendet deshalb das Zurücksetzen. Lassen Sie die Weiterleitung, wie sie ist, und behalten Sie
Freshdesk gern als Archiv.
