# Team-Mail

Team-Mail ist Yuva als Postfach eines Teams: gemeinsame Adressen wie sales@ oder billing@ und die eigenen Postfächer
einzelner Personen. Unterhaltungen, Labels, Zuständige und Notizen sind dieselben wie im Support, nur mit eigenen
Begriffen und Ordnern. Die Mail Ihrer Domain wird zu Yuva geleitet, Sie lesen und schreiben sie im Panel, und der
Verlauf Ihres alten Postfachs zieht ein.

## Was es ist und was nicht

Ein Posteingang hat einen Zweck: **Support** (Kundschaft per Chat, E-Mail und In-App), **Team-Mail** (ein
`mail`-Posteingang) oder **Beides**, also ein Workspace mit Posteingängen beider Arten. Neue Workspaces werden
gefragt, was passt. Ein Team-Mail-Posteingang hat nur E-Mail-Kanäle. Die Support-Funktionen bleiben für ihn aus:
Bewertungen, SLA-Richtlinien, automatische Zuweisung, Begrüßungen, Status, Hilfe, Roadmap, Neuigkeiten und
Support-Berichte.

- Yuva ist das Postfach. Mail kommt über die [Weiterleitung Ihrer Domain zu Yuva](email.md#eingehende-e-mails) an und
  wird im Panel, in der Mobile-App und von [Assistenten](mcp.md) gelesen.
- Yuva ist kein IMAP-, POP3- oder SMTP-Server für Mail-Programme. Sie können es nicht in Apple Mail, Thunderbird oder
  Outlook einrichten, und es gleicht kein Postfach ab, das bei Gmail oder Outlook bleibt: [Den Verlauf
  umziehen](#den-verlauf-umziehen) liest das alte Postfach einmal und schreibt nie hinein.
- Eine Antwort geht über das SMTP-Konto des E-Mail-Kanals hinaus; zum Senden braucht es also dieselbe Einrichtung
  wie für Support-E-Mail.

## Gemeinsame und persönliche Postfächer

| | Gemeinsames Postfach | Persönliches Postfach |
|---|---|---|
| Wofür | Eine Adresse, die das Team gemeinsam beantwortet: sales@, billing@ | Die eigene Mail einer Person |
| Wer liest | Mitglieder mit Zugriff auf den Posteingang, wie bei jedem Posteingang | Die Inhaberin oder der Inhaber und die Mitglieder, denen sie oder er [Zugriff überträgt](#zugriff-übertragen) |
| Wer richtet es ein | Owner und Admins | Jedes Mitglied für sich; ein Owner oder Admin kann eines für ein anderes Mitglied anlegen |
| Filter | Owner und Admins | Nur die Inhaberin oder der Inhaber |

Ein persönliches Postfach ist privat, auch vor Ownern und Admins: siehe [Datenschutz](#datenschutz).

1. Öffnen Sie **Einstellungen → Produkte → Postfach hinzufügen**, in **Mail** die Postfachauswahl → **Neues Postfach**.
2. Beantworten Sie unter **Team-Mail** die Frage **Wer liest es?** mit **Mit dem Team geteilt** oder **Persönlich (nur Sie)**.
   Owner und Admins sehen zusätzlich **Persönlich für ein Mitglied**: Wählen Sie das Mitglied.

In **Mail** gruppiert die Postfachauswahl die Postfächer als **Ihre Postfächer**, **Mit Ihnen geteilt** (mit **Lesen**
oder **Lesen und senden**) und **Team-Mail**. Ein persönliches Postfach lässt sich auch per API anlegen:
`POST /v1/inboxes` mit `owner_member_id`.

## Die Mail einer Domain zu Yuva leiten

Jede Postfach-Adresse ist ein [E-Mail-Kanal](email.md) des Posteingangs: **Einstellungen → der Posteingang → Kanäle →
E-Mail**. Danach:

1. [Weisen Sie die Adresse nach](email.md#adresse-nachweisen), mit einem Code per Mail oder einem DNS-Eintrag.
2. Leiten Sie ihre Mail mit dem [Cloudflare Email Worker oder einem beliebigen MTA](email.md#eingehende-e-mails) zu
   Yuva, oder leiten Sie das alte Postfach an die [Weiterleitungsadresse](email.md#weiterleitungsadresse) des Kanals
   weiter.
3. Geben Sie dem Kanal ein SMTP-Konto zum Senden der Antworten und richten Sie die [DNS-Einträge](email.md#dns-checkliste)
   ein.

Ein Catch-all-Kanal (`*@example.com`) nimmt die Mail aller übrigen Adressen der Domain an.

## Den Verlauf umziehen

[Zu Yuva umziehen](migrate.md) kopiert die Nachrichten Ihres alten Postfachs (Gmail, Outlook, Yandex, iCloud und
jeder IMAP-Server oder eine Exportdatei) in einen Posteingang. Yuva liest das alte Postfach nur; dort ändert sich
nichts. Wählen Sie die Anleitung Ihres Anbieters in der [Liste](migrate.md). Einen Umzug starten Owner und Admins:
**Einstellungen → Zu Yuva umziehen → Neue Migration**. Einen Umzug in ein persönliches Postfach legen nur dessen Inhaberin oder Inhaber an, sehen und führen ihn aus, und nur
wenn sie oder er Owner oder Admin des Workspace ist: Umzüge sind eine Manager-Funktion, ein Agent kann also nicht in
ein eigenes Postfach umziehen. Andere Owner, Admins, Berechtigte und API-Schlüssel sehen ihn nicht. Das Löschen des
Postfachs löscht seine Umzüge; es wird abgelehnt, solange einer scannt, kopiert, synchronisiert oder Verlauf entfernt.

## Der Bereich Mail

**Mail** in der Seitenleiste listet die Mail aller Team-Mail-Posteingänge, die Sie lesen können (**Alle Mails**), oder
die eines Postfachs. Seine Ordner:

| Ordner | Inhalt |
|---|---|
| Postfach | Offene Unterhaltungen |
| Mit Stern markiert | Unterhaltungen, die Sie markiert haben; ein Stern gehört nur Ihnen |
| Zurückgestellt | Unterhaltungen, die bis zu ihrer Zeit ausgeblendet sind |
| Gesendet | Unterhaltungen, in denen Sie Mail gesendet haben |
| Entwürfe | Ihre eigene ungesendete Mail und Antworten |
| Geplant | Ihre Mail, die auf ihre Sendezeit wartet |
| Archiv | Unterhaltungen, die Sie archiviert haben (geschlossene) |
| Papierkorb | Gelöschte Unterhaltungen, nach 30 Tagen geleert; für Owner und Admins gelistet |

Öffnen Sie eine Mail, um zu antworten, sie mit einem Stern zu markieren, zu archivieren, zurückzustellen oder in den
Papierkorb zu legen. **Filter** in der Kopfzeile von Mail öffnet die [Filter](#filter) des Postfachs.

## Neue Mail schreiben

Wählen Sie **Neue Mail**. Wählen Sie unter **Von** das Postfach, aus dem sie kommt, und füllen Sie dann aus:

- **An**: bis zu 50 Adressen. Die erste wird der Kontakt der Unterhaltung.
- **Cc** und **Bcc**: je bis zu 50. Bcc-Empfänger erhalten die Mail, bleiben aber für alle anderen verborgen.
- **Betreff** und die Nachricht, mit Anhängen.

Die Mail geht wie geschrieben hinaus, ohne `Re:`. Die übrigen An- und Cc-Adressen werden mit dem Senden zu
Teilnehmenden, ihre Antworten gehören also zur Unterhaltung. Eine Unterhaltung, die Sie beginnen, wartet unter
**Gesendet**, und die erste Antwort öffnet sie wieder im Postfach. Sie können 60 neue Mails pro Minute schreiben und
300 Empfänger pro Stunde erreichen. Der API-Aufruf ist `POST /v1/conversations/mail`.

### Signaturen

Jedes Mitglied hat pro Posteingang eine Signatur, und ein Posteingang hat eine Standardsignatur, die Owner und Admins
festlegen. Öffnen Sie **Einstellungen → Sie → Signatur**, wählen Sie den Posteingang und schreiben Sie Ihre eigene,
nutzen Sie die Standardsignatur oder keine. In der Standardsignatur werden `{{member.name}}` und `{{member.title}}`
zu Ihrem Namen und Titel. Die Signatur beendet Ihre per E-Mail gesendeten Antworten und neue Mails; **Weglassen**
lässt sie bei einer Nachricht aus. Notizen und Chat-Antworten tragen keine.

## Als sich selbst senden

Eine Mail, die Sie aus einem Team-Mail-Posteingang senden, geht von der Adresse des Kanals mit Ihrem Namen und dem
des Kanals hinaus: `Ada Lovelace (Sales) <sales@example.com>`. Antworten kommen beim Kanal an.

So senden Sie stattdessen von Ihrer eigenen Adresse (`ada@example.com`):

1. Öffnen Sie **Einstellungen → Sie → Deine Adressen** und wählen Sie **Adresse hinzufügen**. Sie können 10
   behalten.
2. Yuva sendet einen 6-stelligen Code an die Adresse, 10 Minuten gültig. Geben Sie ihn ein. Fängt ein Kanal des
   Workspace alle Mail dieser Domain ab, wird kein Code gesendet: Die Adresse wartet dann auf die Freigabe durch einen
   Owner oder Admin unter **Adressen zum Senden** bei Ihrem Mitgliedseintrag in **Einstellungen → Team → Mitglieder**.
3. Wählen Sie sie bei einer neuen Mail oder einer Antwort in der Adressauswahl neben **Von**.

Eine Adresse funktioniert mit einem Kanal nur, wenn ihre Domain die Sende-Domain des Kanals ist und die letzte
[Zustellbarkeitsprüfung](deliverability.md) des Kanals für diese Domain mit ausgerichtetem DKIM und DMARC bestanden
wurde. Sonst lehnt Yuva das Senden ab und nennt den Grund. Eine Adresse, die ein Kanal nutzt oder ein anderes
Mitglied bestätigt hat, wird abgelehnt.

## Entwürfe, Später senden und Rückgängig

- **Entwürfe** werden beim Schreiben gespeichert. Schließen Sie das Fenster, wartet die Mail unter **Entwürfe**; nur
  Sie sehen sie. Ihre Empfänger werden beim Speichern zu Kontakten, wie jede Adresse, an die Sie schreiben.
- **Später senden** (neben Senden) plant eine Mail für eine Zeit zwischen einer Minute und 366 Tagen voraus. Sie wartet
  unter **Geplant**, wo Sie die Zeit ändern können (**Planung abbrechen** macht sie wieder zum Entwurf). Kann sie zur
  vorgesehenen Zeit nicht gesendet werden (die Adresse wurde entfernt, Sie haben den Zugriff verloren), wird sie
  mit dem Grund zum Entwurf, und Sie werden benachrichtigt.
- **Rückgängig**: Eine Antwort in einer Unterhaltung wartet 6 Sekunden, bevor sie hinausgeht, und der Hinweis nach dem
  Planen einer Mail hat ein **Rückgängig**, das die Planung abbricht. Eine neue Mail wird sofort gesendet.

## Filter

Ein Filter sieht jede Mail an, die in einem Postfach ankommt, und handelt danach. Öffnen Sie **Mail → Filter** und
fügen Sie einen mit Name, Bedingungen und Aktionen hinzu. Filter laufen in der Reihenfolge der Liste.

| Bedingungen | Aktionen |
|---|---|
| Von, An, Cc, die Mailingliste (`List-Id`), Betreff, hat einen Anhang | Labels hinzufügen, mit Stern markieren, archivieren, in den Papierkorb legen, zurückstellen, an ein Mitglied weiterleiten, Status oder Priorität ändern, zuweisen |

Textwerte haben 3 bis 200 Zeichen. Ein Filter, der für ein Mitglied einen Stern setzt, zuweist oder weiterleitet,
funktioniert nur für Mitglieder, die das Postfach lesen können. Entwürfe, gesendete Mail, importierter Verlauf und
Zustellberichte lösen keinen Filter aus. Ein Postfach hat höchstens 100.

Um von einer Mail auszugehen, öffnen Sie sie und wählen **Nachrichten wie diese filtern**: Yuva füllt Absender und
Mailingliste oder sonst den Betreff aus, die Aktionen wählen Sie.

Die Filter eines gemeinsamen Postfachs verwalten Owner und Admins. Die Filter eines persönlichen Postfachs gehören
seiner Inhaberin oder seinem Inhaber: Niemand sonst sieht oder bearbeitet sie, und sie wirken nur auf dieses Postfach.

## Suche

Das Suchfeld von Mail nimmt einfache Wörter und Operatoren. Schreiben Sie `key:value`, `key:"zwei Wörter"` oder
`-key:value`, um auszuschließen, was passt. Jeder Operator muss zutreffen; ein Begriff mit anderem Schlüssel wird als
Text gesucht.

| Operator | Passt auf | Beispiel |
|---|---|---|
| `from:` | Absenderadresse oder -name enthält den Wert; `from:me` sind Sie | `from:ada@example.com` |
| `to:`, `cc:` | eine Empfängeradresse enthält den Wert; `to:me` sind Sie | `to:me cc:@example.com` |
| `subject:` | der Betreff | `subject:"quarterly invoice"` |
| `has:attachment` | eine Nachricht mit Anhang | `has:attachment invoice` |
| `is:starred` | Unterhaltungen, die Sie mit Stern markiert haben | `-is:starred` |
| `in:` | ein Ordner: `inbox`, `starred`, `snoozed`, `sent`, `archive`, `trash`, `drafts`, `scheduled` | `in:archive from:ada` |
| `after:`, `before:` | eine Nachricht, die an oder nach, bzw. vor einem Tag `YYYY-MM-DD` gesendet wurde | `after:2026-01-01 before:2026-02-01` |

Einzelheiten wie die Längengrenzen stehen unter [E-Mails durchsuchen](email.md#e-mails-durchsuchen).

## Datenschutz

Owner und Admins finden das Postfach einer anderen Person unter **Einstellungen → Produkte** als `Persönliches
Postfach von <owner>`, mit Schloss. Die Seite zeigt **Adressen**, die Zahl der Unterhaltungen und **Größe** und bietet
nur **Postfach umbenennen** und **Postfach löschen**.

Ein persönliches Postfach liest nur seine Inhaberin oder sein Inhaber und die Mitglieder, denen sie oder er Zugriff
überträgt.

- Owner und Admins sehen, dass ein persönliches Postfach existiert: seinen Namen, seine Adressen, die Zahl seiner
  Unterhaltungen und seine Größe. Nie seine Mail. Sie können es umbenennen oder löschen. Die Übergabe funktioniert nur bei einem verwaisten Postfach und braucht einen im Panel angemeldeten Owner oder Admin, keinen API-Schlüssel.
- API-Schlüssel und Webhooks erreichen ein persönliches Postfach nie, ebenso wenig Automatisierungsregeln des
  Workspace, Slack und Team- oder Mitgliedsfreigaben von Posteingängen. Ein Assistent erreicht es nur über die eigene
  OAuth-Verbindung der Inhaberin oder des Inhabers. Seine Ereignisse gehen an niemand anderen.
- Mailinglisten, Broadcasts, E-Mail-Designs und Analytics-Properties eines persönlichen Postfachs gehören nur seiner
  Inhaberin oder seinem Inhaber: Berechtigte, Owner und Admins sehen oder ändern sie nicht, auch nicht auf Home.
- Kontakte gelten workspace-weit: Es gibt einen Kontakt pro Adresse. Ein gemeinsamer Posteingang, der an eine Adresse
  schreibt, die ein persönliches Postfach kennt, verwendet diesen Kontakt, und eine solche Adresse von Hand
  hinzuzufügen, wird mit „bereits vergeben“ beantwortet. Privat ist nur die Mail im persönlichen Postfach, nicht der
  Kontakteintrag. Ein Kontakt ist nur verborgen, wenn alle seine Unterhaltungen in persönlichen Postfächern liegen, die
  Sie nicht lesen können, und er keine externe ID hat.
- Aufbewahrung, Kontaktlöschung und Workspace-Löschung erfassen auch ein persönliches Postfach, und jede
  Zugriffsübertragung und Übergabe steht im [Audit-Log](operations.md#audit-log).

## Zugriff übertragen

1. Die Inhaberin oder der Inhaber öffnet **Einstellungen → Produkte**, das Postfach, dann **Stellvertreter**.
2. Wählen Sie **Mitglied hinzufügen**, das Mitglied und **Lesen** oder **Lesen und senden**.
3. In derselben Liste ändern oder entfernen Sie Stellvertreter: Sie lesen dieses Postfach sofort nicht mehr.

Wer **Lesen** hat, sieht `Sie können dieses Postfach lesen, aber nicht ändern.` Bei **Lesen und senden** steht im
Editor `Sendet im Auftrag von <name>` und im Verlauf `<delegate> im Auftrag von <owner>`. Stellvertreter erhalten
keinen Einstellungen-Link für das Postfach.

Die Inhaberin oder der Inhaber eines persönlichen Postfachs lässt andere Mitglieder einzeln mit einem von zwei Rechten
hinein. Nur sie oder er kann das; Owner, Admins und bereits Berechtigte können es nicht.

| Recht | Darf | Darf nicht |
|---|---|---|
| Lesen | Alles lesen, was die Inhaberin oder der Inhaber liest: Unterhaltungen, Notizen, Anhänge, Suche, Zähler, Benachrichtigungen | Antworten, Notizen schreiben oder Status, Stern, Label, Zurückstellen oder Zuständige ändern |
| Senden | Alles von Lesen, dazu antworten, neue Mail schreiben und weiterleiten, Notizen schreiben sowie Status, Stern, Zurückstellen, Zuständige und Labels ändern | Mail aus dem Postfach verschieben (bei beiden Rechten nur die Inhaberin oder der Inhaber), seine Einstellungen, Kanäle, Filter oder Berechtigten ändern, Umzüge ausführen oder seine Listen und Broadcasts verwalten |

Kein Recht zeigt die eigenen Entwürfe der Inhaberin oder des Inhabers. Mail, die ein Berechtigter sendet, geht „im
Auftrag“ der Inhaberin oder des Inhabers hinaus: `From` ist deren Name und Adresse, der Berechtigte steht im
`Sender`-Header. Die Nachricht zeigt beide Personen. Wer entzogen wird, verliert den Zugriff sofort. API:
`GET|PUT|DELETE /v1/inboxes/{id}/delegates[/{memberId}]`.

## Wenn ein Mitglied geht

1. Öffnen Sie **Team**, wählen Sie das Mitglied und dann **Entfernen**. Der Dialog `<name> entfernen?` listet jedes
   persönliche Postfach auf.
2. Wählen Sie je Postfach **Dieses Postfach löschen** oder **Übergeben** und das Mitglied unter **Übergeben an**.

Ein verwaistes Postfach zeigt in **Einstellungen → Produkte** **Kein Besitzer** und **Braucht einen Besitzer**. Wählen
Sie **Übergeben** und den **Neuer Besitzer**, oder **Postfach löschen**.

Ein persönliches Postfach wird nicht gemeinsam, wenn seine Inhaberin oder sein Inhaber geht.

- **Ein Owner oder Admin entfernt das Mitglied.** Für jedes persönliche Postfach des Mitglieds verlangt das Entfernen
  von Ihnen, es entweder mitsamt der Mail zu **löschen** oder an ein genanntes Mitglied **zu übergeben**, das zur
  neuen Inhaberin oder zum neuen Inhaber wird. Die Berechtigten eines übergebenen Postfachs verlieren ihren Zugriff.
  Das Entfernen wird abgelehnt, bis für jedes persönliche Postfach des Mitglieds eine Wahl getroffen ist; das Mitglied, an das Sie übergeben, muss aktiv sein und darf nicht das ausscheidende sein. Bis Sie wählen, kann niemand das Postfach lesen.
- **Ein Mitglied löscht das eigene Konto.** Seine persönlichen Postfächer werden nicht gelöscht. Jedes verliert seine
  Inhaberin oder seinen Inhaber und seine Berechtigten und ist verwaist: Niemand liest es. Owner und Admins sehen
  es weiterhin mit Name und Größe und können es, bis sie entscheiden, an ein Mitglied übergeben
  (`POST /v1/inboxes/{id}/hand-over`) oder löschen.

Beide Entscheidungen stehen im Audit-Log.
