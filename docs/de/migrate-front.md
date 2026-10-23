# Von Front umziehen

Kopieren Sie die Unterhaltungen eines Front-Unternehmens nach Yuva und leiten Sie seine Support-Adresse an
Yuva um. Yuva liest Front nur. Die Übersicht steht unter [Zu Yuva umziehen](migrate.md).

## Bevor Sie anfangen

- Erstellen Sie einen API-Token für Yuva. Öffnen Sie in Front **Settings → Developers → API tokens** und
  klicken Sie auf **Create API token**. Nennen Sie ihn „Yuva“. Wählen Sie die Funktion **Access resources**,
  die Namespaces **Global resources** und **Shared resources** (**Private resources** nur, wenn persönliche
  Postfächer mit umziehen sollen; dafür muss jedes Teammitglied den API-Zugriff in seinen eigenen
  Einstellungen erlauben) und die Berechtigung **Read only**: Write, Delete und Send bleiben aus. Erstellen
  Sie ihn und kopieren Sie den Token, den Front nur einmal anzeigt. Fronts Seite:
  [Create and revoke API tokens](https://dev.frontapp.com/docs/create-and-revoke-api-tokens).
- Fügen Sie im Schritt Connect den Token ein und drücken Sie **Test connection**. Erstellen Sie den Token als
  Admin, der jedes Postfach sieht, das Sie umziehen möchten: Yuva liest nur, was der Token sehen kann.

Front zeigt Admins die Berechtigungen eines Tokens, lässt ein Programm sie aber nicht lesen; Yuva kann also
nicht prüfen, ob der Token schreibgeschützt ist. Schreibgeschützt hält ihn Yuvas Verbindung: Sie sendet Front
nur Leseanfragen, und nur an Fronts API-Adresse, sodass sich keine Unterhaltung, kein Kontakt und keine
Einstellung ändert.

## Was kopiert wird

| Front | Yuva |
|---|---|
| Unterhaltungen | Unterhaltungen mit allen Nachrichten |
| Empfangene Nachrichten | Eingehende Nachrichten |
| Von Teammitgliedern gesendete Nachrichten | Ausgehende Nachrichten |
| Kommentare und interne Nachrichten | Notizen, die nur Ihr Team sieht |
| Archiviert oder gelöst | Geschlossen |
| Wartet auf Antwort | Ausstehend |
| Übrige offene Unterhaltungen | Offen |
| Tags | Labels, wie Sie sie zuordnen (oder keine) |
| Teammitglieder | Teammitglieder, wie Sie sie zuordnen; andere bleiben mit Namen erhalten |
| Zuständige Person | Die zuständige Person, wenn das Teammitglied zugeordnet ist |
| Kontakte | Kontakte, mit Name, E-Mail-Adresse und Telefon |
| Posteingänge | Posteingänge, je Front-Posteingang einer, wie Sie sie zuordnen |
| Anhänge | Dateien an den Nachrichten |

Nachrichten anderer Kanäle (SMS, Chat, WhatsApp) kommen als einfache Nachrichten mit ihrem Text an. Entwürfe
und gelöschte Unterhaltungen werden nie importiert. Unterhaltungen behalten ihre ursprünglichen Zeiten und
kommen [still](migrate.md#wie-der-verlauf-still-bleibt) an.

## Was nicht kopiert wird

Regeln, Makros, Textbausteine, eigene Felder, Verknüpfungen, Erinnerungen, Diskussionen zwischen
Teammitgliedern, die nicht zu einer Unterhaltung gehören, Analysen und die Originalquelle einer E-Mail: Front
liefert Yuva jede Nachricht als formatierten Text, daher bleiben Header und Originalformatierung nicht
erhalten. Yuva kopiert nur, was die Tabelle oben nennt. Eine Unterhaltung, deren Absender keine
E-Mail-Adresse hat (etwa bei SMS), bleibt als Verlauf ohne Kanal erhalten.

## Limits

Front begrenzt Anfragen pro Minute je nach Tarif für das ganze Unternehmen (Starter 50, Professional 100,
Enterprise 200), und alle Tokens teilen sich das Limit. Yuva bleibt bei der Hälfte des Limits, das Front
meldet, hält seine Suchen nach Änderungen unter den 40 %, die Front ihnen erlaubt, und wartet so lange, wie
Fronts `retry-after` sagt, wenn Front um Verlangsamung bittet. Ein großes Konto dauert deshalb eine Weile.
Fronts Seite: [Rate limiting](https://dev.frontapp.com/docs/rate-limiting).

## Die Umstellung

Heute erreicht Ihre Support-Adresse Front, weitergeleitet vom Mail-Anbieter, der sie hostet, oder durch eine
Regel dort. Kopieren Sie im Schritt Switch die [Weiterleitungsadresse](email.md#weiterleitungsadresse) des
Kanals und ändern Sie diese Weiterleitung so, dass die Support-Adresse stattdessen Yuva erreicht. Bis dahin
fragt Yuva Front alle zwei Minuten nach Unterhaltungen mit neuer Aktivität, sodass keine Unterhaltung
verloren geht. Antworten aus Yuva gehen von der Adresse des Kanals ab.

Drücken Sie dann „Send a test message“: Es funktioniert, wenn die Nachricht auf dem neuen Weg Yuva erreicht.
Nach einer ruhigen Phase oder mit „Finish now“ endet der Umzug
([so funktioniert es](migrate.md#testnachricht-ruhige-phase-und-abschluss)).

## Nach dem Abschluss

Löschen Sie den Token, den Sie für Yuva erstellt haben: in Front unter **Settings → Developers → API
tokens** den Token öffnen und auf **Delete** klicken. Yuva löscht seine Kopie, wenn der Umzug endet, kann den
Token in Front aber nicht widerrufen; den Zugriff beendet deshalb das Löschen des Tokens. Lassen Sie die
Weiterleitung, wie sie ist, und behalten Sie Front gern als Archiv.
