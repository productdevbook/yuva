# Zu Yuva umziehen

Holen Sie den Verlauf des Postfachs, von dem aus Sie heute antworten, in einen Yuva-Posteingang und leiten
Sie danach seine E-Mails um. Yuva liest das alte Postfach nur: nichts darin wird verändert oder gelöscht,
und Ihr Team arbeitet währenddessen weiter.

Ein Postfach umziehen können Owner und Admins: **Settings → Move to Yuva → New migration**. Dieselben
Schritte stehen [Assistenten](#von-claude-code-oder-einem-anderen-assistenten) und der
[API](#aus-ihrem-backend) offen. Wählen Sie die Anleitung Ihres Anbieters: [Gmail](migrate-gmail.md),
[Google Workspace](migrate-google-workspace.md), [Outlook.com](migrate-outlook.md), [Microsoft 365](migrate-microsoft365.md), [Yandex](migrate-yandex.md), [iCloud](migrate-icloud.md),
[Zoho Mail](migrate-zoho.md), [Yahoo Mail](migrate-yahoo.md), [Fastmail](migrate-fastmail.md),
[GMX und mail.com](migrate-gmx.md), [Zendesk](migrate-zendesk.md), [Help Scout](migrate-helpscout.md), [Chatwoot](migrate-chatwoot.md), [Front](migrate-front.md), [jeder andere IMAP-Server](migrate-imap.md) oder [eine Exportdatei](migrate-files.md) (Google Takeout, mbox, .eml oder Maildir in einer Zip). Betreiben Sie einen
eigenen Server? Siehe [Zu Yuva umziehen für Betreiber](migrate-operators.md). Zendesk, Help Scout, Outlook.com und Microsoft 365 verbinden sich per Anmeldung statt mit einem Passwort, Front mit einem API-Token.

## Was Yuva mit Ihrem Postfach tut und was nicht

Yuva löscht, verschiebt oder archiviert in Ihrem alten Postfach nichts, markiert nichts als gelesen und
ändert dort keine Einstellung. Das ist dreifach abgesichert, damit kein einzelner Fehler Ihre E-Mails
verändern kann:

- Yuva erreicht das Postfach nur über Code, der Ordner schreibgeschützt öffnet (`EXAMINE`) und Nachrichten
  abruft, ohne sie zu markieren (`BODY.PEEK`).
- Unter diesem Code wird jeder Befehl geprüft, bevor er den Server verlässt. Erlaubt sind nur `CAPABILITY`,
  `NOOP`, `LOGOUT`, `STARTTLS`, `LOGIN`, `AUTHENTICATE`, `LIST`, `STATUS`, `EXAMINE`, `UNSELECT`, `SEARCH`
  und `FETCH` von Elementen, die eine Nachricht nicht markieren. Alles andere schließt die Verbindung,
  bevor es gesendet wird.
- Tests, die bei jeder Änderung laufen, lassen den Build fehlschlagen, wenn ein schreibender Befehl
  gesendet werden könnte, und ein Testpostfach wird nach einem Umzug auf Byte-Gleichheit geprüft.

Mit einem App-Passwort ließen sich E-Mails ändern; Yuvas Verbindung lehnt jeden Befehl ab, der das tut. Die
Seite des Umzugs zeigt „Changes made to {Anbieter}: none“.

## Was kopiert wird

| Kopiert | Nicht kopiert |
|---|---|
| Nachrichten der gewählten Ordner, in beide Richtungen, mit ihren ursprünglichen Zeiten | Spam, Papierkorb und Entwürfe: werden nie gelesen |
| Anhänge (zu große oder von einem Typ, den der Server nicht speichert, bleiben in der Originalnachricht) | Nachrichten über der Grenze des Servers für eine Nachricht ([`YUVA_MIGRATION_MAX_MESSAGE_BYTES`](configuration.md#zu-yuva-umziehen), 50 MiB) |
| Verläufe als Unterhaltungen, verbunden über `References` und `In-Reply-To`, nie über den Betreff | Verläufe nur aus automatischen E-Mails (Auto-Antworten, Newsletter, Zustellberichte), außer Sie schalten die Option ab |
| Kunden als Kontakte; die E-Mails Ihres Teams als ausgehende Nachrichten | Verläufe nur aus E-Mails Ihres Teams |
| Ordner und Labels als Yuva-Labels, wie Sie sie zuordnen | Nachrichten außerhalb des gewählten Zeitraums |
| Der Originalquelltext jeder Nachricht, wenn Sie „Keep original messages“ ankreuzen (braucht mehr Speicher) | |

Unterhaltungen, deren letzte Nachricht älter ist als die gewählte Zahl von Tagen (standardmäßig 14), werden
geschlossen; neuere bleiben offen, wenn zuletzt der Kunde schrieb, und stehen auf Wartend, wenn Ihr Team
zuletzt schrieb.

## Wie der Verlauf still bleibt

Importierte Unterhaltungen behalten ihre ursprünglichen Zeiten, sind im Posteingang als **Imported**
markiert und zeigen nach der letzten importierten Nachricht eine Trennlinie. E-Mails von den Adressen Ihres
Teams erscheinen als gesendet von dem Mitglied, dem Sie sie zuordnen, oder sonst unter ihrem ursprünglichen
Namen und ihrer Adresse.

- Für importierte E-Mails wird keine Benachrichtigung, E-Mail, Push-Nachricht, kein Webhook, keine
  Auto-Antwort und keine Bewertungsanfrage gesendet.
- Nichts Importiertes gilt als ungelesen.
- Importierte E-Mails bleiben aus den Statistiken (Antworten, erste Antworten, Schließungen) heraus und
  zählen in den Import-Spalten der Nutzung statt in den abgerechneten.
- Die [Aufbewahrungsfrist](operations.md#aufbewahrung) des Workspace zählt ab dem Tag des Imports.
- Während des Kopierens zeigt der Posteingang ein Banner; jeder, der den Posteingang sieht, sieht es.

## Die Schritte

1. **Source.** Geben Sie die Adresse des Postfachs ein. Yuva findet den Anbieter aus der Adresse und den
   Mailservern der Domain; noch wird nichts gelesen. Oder wählen Sie den Anbieter selbst.
2. **Connect.** Geben Sie den Benutzernamen und ein App-Passwort ein (die Anleitung Ihres Anbieters steht
   neben dem Feld) und drücken Sie „Test connection“. Der Test zeigt, welcher Schritt scheiterte und wie
   Sie ihn beheben. Zendesk, Help Scout, Outlook.com und Microsoft 365 nutzen stattdessen eine Anmeldung.
3. **Choose.** Wählen Sie die Ordner und wie weit zurück. Yuva scannt nur die Header und kann Ihnen so genau
   sagen, was importiert wird.
4. **Map.** Wählen Sie den Posteingang und den E-Mail-Kanal für die Adresse (Yuva bietet an, sie anzulegen),
   welchem Mitglied jede Teamadresse gehört, wie Ordner zu Labels werden, wann alte Unterhaltungen
   geschlossen werden und ob automatische E-Mails übersprungen oder Originale behalten werden.
5. **Review.** Der Probelauf listet die Zahlen je Ordner (die Summe ist jede Nachricht der gewählten Ordner, Duplikate und übersprungene eingeschlossen), was übersprungen wird und warum, Kontakte, Labels,
   Speicher und Zeit und zeigt, wie die neuesten Unterhaltungen aussehen. Bis Sie „Start migration“ drücken,
   wird nichts importiert. Eine Änderung hier plant das Kopieren neu.
6. **Copy.** Neueste Unterhaltungen zuerst, mit Live-Fortschritt. Sie können die Seite schließen, pausieren,
   fortsetzen oder abbrechen; ein Kopiervorgang, der stoppt (widerrufenes App-Passwort, Limit des Anbieters),
   läuft nach der Behebung dort weiter, wo er stand; „Retry failed items“ versucht fehlgeschlagene
   Nachrichten erneut.
7. **Switch.** Der Verlauf ist drin, und Yuva liest das alte Postfach weiter alle zwei Minuten, damit nichts
   verloren geht, während Sie seine E-Mails an Yuva schicken (unten).
8. **Done.** Der Umzug wird abgeschlossen, seine Zugangsdaten werden gelöscht, und die Seite sagt, wo Sie das
   App-Passwort entfernen.

## Die Zustellung umstellen

E-Mails, die das alte Postfach vor dem Ende des Kopierens erhielt, sind Verlauf. Was danach eintrifft, ist
neue E-Mail: Sie erscheint mit Benachrichtigungen, als wäre sie direkt bei Yuva eingegangen, aber ohne die
Begrüßung des Posteingangs. Was auf beiden Wegen bei Yuva ankommt, wird einmal gespeichert.

Schicken Sie die E-Mails des Postfachs auf einem von drei Wegen an Yuva:

| Weg | Wer | Das alte Postfach behält eine Kopie |
|---|---|---|
| Das Postfach an die [Weiterleitungsadresse](email.md#weiterleitungsadresse) des Kanals weiterleiten | der Postfachinhaber | ja |
| Auf dem Server als Admin an dieselbe Adresse routen | ein Google-Workspace- oder Mailserver-Admin | ja |
| Die MX-Einträge einer Domain oder Subdomain ändern, die nur für den Support dient ([E-Mail-Anleitung](email.md)) | wer das DNS verwaltet | nein |

Ein Server ohne Weiterleitungsdomain ([`YUVA_FORWARDING_DOMAIN`](configuration.md#zu-yuva-umziehen)) hat
keine Weiterleitungsadresse und bietet nur den MX-Weg an. Jede Anbieteranleitung zeigt die
Weiterleitungseinstellungen. Schickt Google eine Bestätigung an die Weiterleitungsadresse, erscheinen Code
und Link in diesem Schritt.

## Testnachricht, ruhige Phase und Abschluss

Drücken Sie „Send a test message“. Yuva schickt dem Postfach eine kurze Nachricht; kommt sie auf dem neuen
Weg bei Yuva an, funktioniert die Umstellung, und Yuva sagt, ob sie per Weiterleitung oder direkt kam. Wird
sie nach einigen Minuten nur im alten Postfach gefunden, erreichen E-Mails noch den alten Anbieter, werden
aber noch nicht weitergeleitet.

Ist eine Testnachricht angekommen, liest Yuva das alte Postfach noch eine ruhige Phase lang (standardmäßig
24 Stunden). Taucht in dieser Zeit nichts nur dort auf, schließt sich der Umzug von selbst ab. Mit „Finish
now“ beenden Sie ihn früher, mit „Finish without checking“ ohne Testnachricht; E-Mails, die dann nur das
alte Postfach erreichen, werden nicht kopiert. Ein Umzug, der nach
[`YUVA_MIGRATION_SYNC_MAX_DAYS`](configuration.md#zu-yuva-umziehen) (30 Tage) nicht abgeschlossen ist,
stoppt von selbst.

Lassen Sie die Weiterleitung oder den MX nach dem Abschluss, wie sie sind: So erreichen E-Mails jetzt Yuva.
Das alte Postfach ist unberührt; behalten Sie es gern als Archiv.

## Das Passwort

Yuva versiegelt das Passwort mit dem Master-Key des Servers, sobald Sie es eingeben. Es wird nie wieder
angezeigt und erscheint nie in Logs, Ereignissen, API-Antworten oder Assistenten-Tools. Yuva löscht es, wenn
der Umzug abgeschlossen oder abgebrochen wird, wenn die Sync-Grenze erreicht ist, wenn Sie es ersetzen und mit
dem Workspace. Die Seite sagt dann „Credentials deleted“. Yuva kann ein App-Passwort bei Ihrem Anbieter nicht
widerrufen: Entfernen Sie dort das für Yuva angelegte (jede Anbieteranleitung verlinkt die Seite).

## Importierten Verlauf entfernen

Settings → Move to Yuva → der Umzug → „Remove imported history“, sobald er abgeschlossen oder abgebrochen
ist. Yuva löscht die importierten Unterhaltungen, in die seitdem niemand geschrieben hat, mit Nachrichten und
Dateien, dann die Kontakte, die der Umzug angelegt hat und die ohne Unterhaltung bleiben. Unterhaltungen, in
die Ihr Team geschrieben hat, und Labels bleiben. Die Zahl bestätigen Sie vorher. Im alten Postfach ändert
sich nichts.

## Von Claude Code oder einem anderen Assistenten

Verbinden Sie den Assistenten mit dem Scope `migrations:manage` mit [Yuvas MCP-Server](mcp.md) und starten
Sie den Prompt `move_to_yuva` mit der Postfachadresse (und optional einem Posteingang). Der Assistent findet
den Anbieter, legt den Umzug an, scannt, zeigt den Probelauf und die neuesten Unterhaltungen und startet das
Kopieren erst, wenn Sie die Zahlen bestätigt haben. Er erklärt, wie die E-Mails umgestellt werden, sendet die
Testnachricht und schließt den Umzug ab.

Der Assistent fragt nie nach dem Passwort. Er gibt Ihnen eine Yuva-Seite; dort geben Sie das App-Passwort
ein, und kein Tool nimmt eines entgegen. Assistenten sehen weder das Passwort noch das Eingegebene.

Nennen Sie einen Posteingang, aber keinen Kanal, verwendet der Assistent den E-Mail-Kanal des Posteingangs für
die Adresse oder legt einen an. Ein neuer Kanal hat kein SMTP-Konto: Der Assistent sagt Ihnen, was Sie in den
Einstellungen des Kanals im Panel eintragen müssen (SMTP-Server, Benutzername und Passwort des Postfachs),
bevor Yuva von dort Antworten senden kann.

## Ihre Daten

- Importierte E-Mails sind Daten Ihres Workspace wie jede Unterhaltung. Sie bleiben in Datenbank und Speicher
  des Workspace; Kontaktexport und -löschung, Aufbewahrung (ab dem Import gezählt) und das Löschen des
  Workspace erfassen sie.
- Yuva kontaktiert nur das Postfach, das Sie verbinden, mit Ihren Zugangsdaten und nur zum Lesen. Die
  Erkennung schlägt DNS-Einträge der Domain und deren eigene Mail-Einstellungsdatei nach; an niemand
  sonst wird etwas gesendet.
- Sie entscheiden, was importiert wird (Ordner, Zeiträume), und sind Verantwortlicher für die darin
  enthaltenen personenbezogenen Daten. Ein Hosting-Betreiber verarbeitet sie in Ihrem Auftrag, nur um den
  gewünschten Import zu erbringen: nie für Werbung oder Modelltraining, und von Menschen nur gelesen für
  Sicherheit, Support auf Ihren Wunsch oder aufgrund eines Gesetzes.
- Owner und Admins sehen jede Aktion in der Aktivität des Umzugs; sie wird nach dessen Ende
  [`YUVA_MIGRATION_AUDIT_DAYS`](configuration.md#zu-yuva-umziehen) (365) Tage aufbewahrt.

## Aus Ihrem Backend

Owner, Admins und API-Keys ohne Posteingangsbeschränkung mit `migrations:manage` (Agents bekommen es nie; sie
erhalten `403`). Die vollständigen Formen stehen im [API-Vertrag](../../openapi/openapi.yaml).

| Endpunkt | Zweck |
|---|---|
| `GET /v1/migrations/presets` | Die Anbieter dieses Servers mit ihren Servern und Ordnern. |
| `POST /v1/migrations/detect` | Den Anbieter einer Adresse finden. |
| `GET`, `POST /v1/migrations` | Umzüge auflisten; einen starten (`draft`). |
| `GET`, `PATCH`, `DELETE /v1/migrations/{migrationId}` | Lesen; Postfach, Posteingang, Kanal und Optionen ändern; löschen. |
| `PUT /v1/migrations/{migrationId}/credentials` | Das Postfach mit Benutzername und Passwort verbinden. |
| `GET /v1/migrations/{migrationId}/upload` | Die Teile einer [Exportdatei](migrate-files.md), die der Server hat, um einen abgebrochenen Upload fortzusetzen. |
| `POST /v1/migrations/{migrationId}/upload/parts/{part}` | Ein Teil (höchstens 8 MiB) der Exportdatei mit seinem SHA-256 hochladen. |
| `POST /v1/migrations/{migrationId}/upload/complete` | Den Upload abschließen; Yuva liest die Datei und der Umzug wird `connected`. |
| `POST /v1/migrations/{migrationId}/test` | Die Verbindung testen. |
| `GET /v1/migrations/{migrationId}/folders` | Die Ordner des Postfachs. |
| `POST /v1/migrations/{migrationId}/scan` | Die Header für den Probelauf lesen. |
| `GET /v1/migrations/{migrationId}/preview` | Die neuesten Unterhaltungen, wie sie aussehen werden. |
| `POST /v1/migrations/{migrationId}/start` | Das Kopieren starten. |
| `POST /v1/migrations/{migrationId}/pause`, `resume`, `cancel` | Pausieren, fortsetzen oder abbrechen. |
| `POST /v1/migrations/{migrationId}/retry` | Fehlgeschlagene Nachrichten erneut versuchen. |
| `GET /v1/migrations/{migrationId}/errors` | Was schiefging. |
| `POST /v1/migrations/{migrationId}/probe` | Die Testnachricht senden. |
| `POST /v1/migrations/{migrationId}/finish` | Abschließen (`force` überspringt die Prüfung). |
| `POST /v1/migrations/{migrationId}/remove-history` | Importierten Verlauf entfernen (`confirm_count`). |
| `POST /v1/channels/{channelId}/forwarding-address/rotate` | Einem E-Mail-Kanal eine neue [Weiterleitungsadresse](email.md#weiterleitungsadresse) geben. |

Zustände: `draft`, `connected`, `scanning`, `ready`, `copying`, `paused`, `needs_attention`, `syncing`,
`completed`, `cancelled`, `removing`, `removed`; jeder Umzug listet die jetzt erlaubten `actions`. Fehler, die
Sie behandeln sollten: `409 migration_state`, `409 provider_not_configured`, `409 confirm_count_mismatch`,
`422 hosted_limit` (die Einstellung steht in `limit`) und `429` bei der Testnachricht und den
Verbindungstests. Der Fortschritt erreicht Owner und Admins auf `/v1/realtime` als `migration.updated`,
`migration.progress` und `migration.forwarding_confirmation`; Umzüge senden nie Webhooks.
