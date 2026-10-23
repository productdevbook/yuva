# Von Intercom umziehen

Kopieren Sie die Unterhaltungen eines Intercom-Workspace nach Yuva und leiten Sie dessen Support-Adresse an Yuva
um. Yuva liest Intercom nur. Die Übersicht steht unter [Zu Yuva umziehen](migrate.md).

## Bevor Sie beginnen

- Legen Sie in Ihrem eigenen Workspace eine private App für Yuva an. Öffnen Sie in Intercom **Settings →
  Integrations → Developer Hub**, wählen Sie **New app** und nennen Sie sie "Yuva". Aktivieren Sie unter
  **Configure → Authentication** nur die Leseberechtigungen: Konversationen lesen, Benutzer und Unternehmen
  lesen, Administratoren lesen und Tags lesen. Die Seite von Intercom:
  [Authentication](https://developers.intercom.com/docs/build-an-integration/learn-more/authentication).
- Kopieren Sie das **Zugriffstoken** (access token) der App von derselben Seite.
- Wählen Sie im Schritt Connect die **Region**, in der die Daten Ihres Workspace liegen (USA, Europa oder
  Australien), und fügen Sie das Token ein. Yuva sendet es als Bearer-Token nur an den API-Host dieser Region
  und zeigt es niemandem.

Yuva verwendet kein OAuth: Eine App in Ihrem eigenen Workspace braucht keines. Yuva kann nicht erkennen, welche
Berechtigungen ein Token hat; schreibgeschützt hält die Verbindung: Außer dem Token sendet sie Intercom nur
Leseanfragen (`GET`), sodass sich keine Unterhaltung, kein Kontakt und keine Einstellung ändert. Die Suche von
Intercom, die `POST` braucht, ruft Yuva nie auf.

## Was kopiert wird

| Intercom | Yuva |
|---|---|
| Unterhaltungen | Unterhaltungen mit allen Nachrichten |
| Kommentare eines Benutzers oder Leads | Eingehende Nachrichten, von der Person, die sie geschrieben hat |
| Kommentare eines Admins oder Bots | Ausgehende Nachrichten |
| Notizen | Notizen, die nur Ihr Team sieht |
| Status: open | Offen |
| Status: snoozed | Ausstehend |
| Status: closed | Geschlossen |
| Zuweisungen und Statuswechsel | Nichts |
| Tags | Labels, wie Sie sie zuordnen (oder keine) |
| Admins | Teammitglieder, wie Sie sie zuordnen; andere bleiben mit Namen erhalten |
| Zugewiesener Admin | Die zuständige Person, wenn der Admin zugeordnet ist |
| Teams | Posteingänge, je Team einer, wie Sie sie zuordnen; Unterhaltungen ohne Team sind der Ordner "Unassigned" |
| Benutzer und Leads | Kontakte, mit Name, E-Mail und Telefon (ein Kontakt ohne E-Mail-Adresse bleibt mit Namen erhalten) |
| Anhänge | Dateien an den Nachrichten |

Unterhaltungen behalten ihre ursprünglichen Zeiten und kommen [still](migrate.md#wie-der-verlauf-still-bleibt) an.

## Was nicht kopiert wird

Tickets (Intercoms eigene Ticket-Typen), Unternehmen, benutzerdefinierte Attribute, Help-Center-Artikel,
Workflows, die Konfiguration von Fin, Bewertungen und Berichte. Yuva kopiert nur, was die Tabelle oben nennt.

## Sehr lange Unterhaltungen

Intercom liefert höchstens die 500 jüngsten Teile einer Unterhaltung. Yuva importiert diese und hält die
Unterhaltung als Fehler der Migration fest (`truncated`), sodass Sie sehen, bei welchen die ältesten Nachrichten
fehlen. Links, die jemand in eine Nachricht getippt hat, werden nie geöffnet; nur die Dateien, die Intercom
angehängt hat, werden geholt, von Intercoms Dateiservern und ohne das Token.

## Limits

Intercom erlaubt privaten Apps 10.000 Anfragen pro Minute und Workspace, geteilt von allen, gezählt in
10-Sekunden-Fenstern. Yuva bleibt bei der Hälfte des Limits, das Intercom meldet (`X-RateLimit-Limit`), und
wartet, wenn Intercom 429 antwortet. Die Seite von Intercom:
[Rate limiting](https://developers.intercom.com/docs/references/rest-api/errors/rate-limiting). Ein großer
Workspace braucht deshalb eine Weile: Eine Unterhaltung zu lesen kostet zwei Anfragen.

## Die Umstellung

Heute erreicht Ihre Support-Adresse Intercom, weitergeleitet vom Mailanbieter, der sie hostet, oder durch eine
Regel dort. Kopieren Sie im Schritt Switch die [Weiterleitungsadresse](email.md#weiterleitungsadresse) des Kanals
und ändern Sie die Weiterleitung so, dass die Support-Adresse stattdessen Yuva erreicht. Bis dahin liest Yuva
Intercom alle zwei Minuten, sodass keine Unterhaltung verloren geht. Antworten aus Yuva gehen von der Adresse des
Kanals ab.

Drücken Sie dann „Send a test message“: Es funktioniert, wenn die Nachricht über den neuen Weg bei Yuva
ankommt. Nach der ruhigen Phase oder mit „Finish now“ endet die Migration
([wie](migrate.md#testnachricht-ruhige-phase-und-abschluss)).

## Danach

Löschen Sie die App, die Sie für Yuva angelegt haben: in Intercom unter **Settings → Integrations → Developer
Hub**. Yuva löscht sein Token, wenn die Migration endet, kann es bei Intercom aber nicht widerrufen; das Löschen
der App beendet den Zugriff. Lassen Sie die Weiterleitung, wie sie ist, und behalten Sie Intercom gern als
Archiv.
