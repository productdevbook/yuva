# Broadcasts versenden

Ein Broadcast ist eine einzelne E-Mail an die Abonnenten Ihrer [Mailinglisten](lists.md): ein
Newsletter, ein Produkt-Update, eine Ankündigung. Er nutzt ein [E-Mail-Design](designs.md), geht über
einen E-Mail-Kanal Ihres Posteingangs hinaus, und eine Antwort darauf wird eine Unterhaltung in diesem
Posteingang. Danach zeigt Yuva, was geschehen ist: wer die Mail bekam, wer gebounct ist, sich abgemeldet
oder geantwortet hat.

Der Ablauf ist Listen, dann ein Design, dann ein Broadcast. Broadcasts versenden Owner und Admins;
Agents lesen die Broadcasts ihrer Posteingänge, damit sie eine Antwort verstehen.

## 1. Vor dem ersten Broadcast

1. Legen Sie eine [Liste](lists.md#2-eine-liste-anlegen) an und bringen Sie Abonnenten darauf, per Formular,
   aus dem Backend oder durch [Import einer CSV-Datei](importing.md). Ein Broadcast geht nur an Personen, die
   auf einer Liste `subscribed` sind.
2. Tragen Sie die [Absenderangaben](lists.md#1-absenderangaben-eintragen) des Posteingangs ein. Jeder
   Broadcast endet damit, und die Prüfung blockiert den Versand ohne rechtlichen Namen und Postanschrift.
3. Bauen Sie ein [Design](designs.md) oder starten Sie mit einer Vorlage.
4. Der Posteingang braucht einen [E-Mail-Kanal](email.md) mit SMTP-Konto und einer Absenderadresse. Ein
   Catch-all-Kanal braucht eine From-Adresse. Richten Sie SPF, DKIM und DMARC für die Domain dieser
   Adresse ein und starten Sie die [Zustellbarkeitsprüfung](deliverability.md).
5. Optional, aber ratsam: ein eigenes Konto des Kanals für Broadcasts ([unten](#der-absender)).

Auf einem Server, den jemand anderes betreibt, muss ein neuer Workspace vor dem Versand eventuell auch
[freigegeben](#freigabe-auf-gehosteten-servern) werden.

## 2. Den Broadcast schreiben

Im Panel: Broadcasts, dann New broadcast. Geben Sie einen Namen, den nur Ihr Team sieht, und wählen Sie
den Posteingang. Er bleibt ein Entwurf, und jede Änderung wird beim Machen gespeichert, bis Sie ihn
senden oder planen.

### Der Absender

Wählen Sie den E-Mail-Kanal, der ihn versendet. Der Composer sagt, ob der Kanal bereit ist, welches
Konto er nutzt und wie schnell er sendet.

Broadcast-Mails laufen nie über den eigenen Mailer des Servers. Es wird das separate Broadcast-Konto des
Kanals genutzt, falls vorhanden, sonst sein Support-Konto. Ein eigenes Konto hält Beschwerden über
Werbemails von dem Konto fern, das Ihre Support-Antworten trägt, und ein langsamer Versand verzögert
nie eine Support-Antwort.

So richten Sie eines ein: Settings → den Posteingang → Channels → E-mail, dort der Abschnitt Broadcast
sending. Schalten Sie „Use a separate account for broadcasts“ ein und tragen Sie Host, Port,
Verschlüsselung, Benutzername und Passwort ein. Speichern mit ausgeschaltetem Schalter entfernt es. Das
Tempo ist die Zahl der Mails pro Sekunde, die der Kanal sendet, von 1 bis 100 (5 bei einem neuen Kanal);
bleiben Sie unter dem, was Ihr Anbieter erlaubt.

### Die Zielgruppe

Wählen Sie bis zu 10 Listen des Posteingangs. Wer auf mehreren steht, bekommt eine Mail, und zwar unter
der ersten seiner Listen in der von Ihnen gewählten Reihenfolge.

Auf Wunsch schränken Sie ein:

- **Bedingungen** (bis zu 10; alle müssen passen) vergleichen ein Kontaktattribut: ist, ist nicht,
  enthält (ohne Beachtung der Groß-/Kleinschreibung), ist größer als, ist kleiner als, ist eines von,
  ist gesetzt, ist nicht gesetzt. Ein Kontakt ohne das Attribut passt nur zu „ist nicht“ und „ist nicht
  gesetzt“.
- **Sprachen** (bis zu 20) behalten nur Kontakte, deren Sprache eine davon ist; `tr` behält auch
  `tr-TR`. Leer behält alle.

Der Composer zeigt die Empfängerzahl, während Sie die Zielgruppe ändern, und unter „Left out“, warum die
anderen nichts bekommen:

| Grund | Wer |
|---|---|
| Unsubscribed | Von den Listen abgemeldet. |
| Not confirmed yet | Auf den Listen nur `pending` (Double-Opt-in nicht abgeschlossen). |
| Bounced or reported as spam | Die Adresse ist gesperrt. |
| Objected to all mail | Die Person hat allen Mails Ihres Workspaces widersprochen. |
| Blocked contacts | Sie haben den Kontakt blockiert. |
| Left out by a condition or language | Eine Bedingung oder Sprache passt nicht. |
| On several lists, one mail each | Die überzähligen Abonnements. |

### Der Inhalt

Wählen Sie das Design. Schreiben Sie den Betreff (bis zu 300 Zeichen; Posteingänge zeigen etwa die ersten
80) und, wenn Sie möchten, den Vorschautext, der in den meisten Posteingängen auf den Betreff folgt. Leer
nimmt den des Designs. Merge-Felder wie `{{ contact.first_name | "there" }}` funktionieren auch im
Betreff; die Prüfung hält Sie auf, wenn ein Feld ohne Fallback bei jemandem leer wäre.

### Optionen

„Count clicks“ ist aus. Siehe [Klick-Tracking](#klick-tracking). Links tragen außerdem Kampagnenparameter für
[Analytics](#kampagnen-tracking-utm), an, solange Sie es nicht ausschalten.

### Versandtempo

Ein Broadcast kann ein eigenes Tempo haben: höchstens so viele Mails pro Stunde, pro Tag und pro Monat
(30 Tage), jeweils optional. „Tempo begrenzen“ im Editor schaltet es ein. Yuva verteilt die Mails dann
gleichmäßig nach dem engsten der drei Werte. 100 am Tag heißt also etwa alle 14 Minuten eine Mail, nicht
100 um Mitternacht, und keine 60 Minuten, 24 Stunden oder 30 Tage bekommen mehr, als Sie festgelegt haben.
Das Tempo des Kanals und das [Tageslimit](#das-tageslimit) des Workspaces gelten weiter; das Tempo des
Broadcasts kann es nur verlangsamen. Jeder Versuch zählt, auch ein erneuter; Personen, die inzwischen
ausgetragen wurden und übersprungen werden, nicht. Ein Broadcast darf mit seinem Tempo höchstens 365 Tage
dauern; die Prüfung hält Sie vorher auf. Das Tempo eines geplanten, sendenden oder pausierten Broadcasts
ändern Sie auf seiner Seite; es gilt ab der nächsten Mail. Eine Pause spart keine Mails an: Nach dem
Fortsetzen wartet die nächste Mail auf ihren Platz.

### Testversand

Senden Sie vor jedem Versand einen Test an sich selbst oder an bis zu fünf Mitglieder. Er sieht genau
aus wie die echte Mail, mit `[Test]` vor dem Betreff. Tests gehen nur an Mitglieder des Workspaces, nie
an eine andere Adresse, und ihr Abmeldelink sagt, dass es ein Test war. Die Merge-Felder füllen Sie mit
Beispielwerten oder mit den Werten eines Kontakts (der Test geht trotzdem nur an die gewählten
Mitglieder). Höchstens 20 Tests pro Stunde und Workspace.

## 3. Prüfen und senden

„Review and send“ zeigt die Checkliste, die der Server beim Planen ausführt. Blockierende Punkte stoppen
den Versand; Warnungen nicht.

| Punkt | Blockiert, wenn |
|---|---|
| Sender details for the footer | Der Posteingang hat keinen rechtlichen Namen oder keine Postanschrift. |
| Sending channel | Kein Kanal gewählt, oder er hat kein SMTP-Konto oder keine From-Adresse. |
| Permission to send | Broadcasts sind auf dem Server aus, der Workspace wartet auf [Freigabe](#freigabe-auf-gehosteten-servern) oder ist gesperrt. |
| Design | Keines gewählt, oder es ergibt mehr als 500 KB HTML. |
| Recipients | Keine Liste gewählt, oder niemand bekäme die Mail. |
| Merge fields | Ein Feld ohne Fallback wäre bei jemandem leer. |
| Mail size | Über 102 KB HTML, wo Gmail die Mail abschneidet (samt Fußzeile und Abmeldelink). Ab 90 KB eine Warnung. |
| Image descriptions | Warnung: einem Bild fehlt der Alternativtext. |
| Links | Warnung: ein Link nutzt `http:`. |
| Subject | Leer oder länger als 300 Zeichen. Ab 80 eine Warnung. |
| Sending pace | Das Tempo des Broadcasts würde den Versand länger als 365 Tage dauern lassen. |
| Daily limit | Warnung: das [Tageslimit](#das-tageslimit) macht den Versand länger als einen Tag. |
| Test mail | Warnung: seit der letzten Änderung ging kein Test hinaus. |
| Click tracking | Warnung, wenn an: Links zeigen auf die Link-Domain des Servers. |
| Amazon-SES-Tenant | Blockiert, wenn der SES-Tenant des Posteingangs pausiert ist oder fehlt, nicht eingerichtet werden konnte oder die Identität des Kanals fehlt. Warnung bei einem Reputationsbefund hoher Schwere oder solange der Server den Tenant noch einrichtet (der Versand wartet Sekunden). |

Wählen Sie dann Send now oder Schedule mit einer Zeit in Ihrer Zeitzone, höchstens 90 Tage im Voraus. Das
Panel bittet Sie, die Empfängerzahl zu bestätigen. Hat sich die Zielgruppe geändert, während Sie
hinschauten, bittet es um eine neue Prüfung.

Was beim Planen passiert:

- Das Design wird in seiner aktuellen Version eingefroren. Spätere Änderungen am Design ändern diesen
  Broadcast nicht.
- Beim Start wird die Zielgruppe einmal festgelegt. Wer sich später anmeldet, ist nicht dabei. Wer sich
  abmeldet, bounct oder sich beschwert, bevor seine Mail hinausgeht, wird übersprungen, und jede Adresse
  wird direkt vor ihrer Mail noch einmal geprüft.
- Bis zum Start holt „Back to draft“ einen geplanten Broadcast zur Änderung zurück.

## 4. Während des Versands

Die Broadcast-Seite zeigt den Fortschritt live: festgelegte Empfänger, gesendet, fehlgeschlagen,
gebounct und wann er ungefähr fertig ist. Mit einem Tempo zeigt die Seite, wann die nächste Mail rausgeht, und „Tempo ändern“ passt es an.

| Status | Bedeutung |
|---|---|
| Draft | Wird geschrieben. |
| Scheduled | Wartet auf seine Zeit. |
| Preparing | Die Zielgruppe wird festgelegt. |
| Sending | Mails gehen hinaus. |
| Paused | Gestoppt, bis jemand fortsetzt. |
| Sent | Fertig. |
| Cancelled | Endgültig gestoppt. |
| Failed | Er kann nicht weiter, zum Beispiel weil sein Kanal entfernt wurde. |

**Pause** stoppt den Versand innerhalb des laufenden Stapels (höchstens 50 Mails). **Resume** macht dort
weiter, wo er aufgehört hat. **Cancel** beendet ihn endgültig; noch nicht gesendete Mails werden
übersprungen. Ein Entwurf oder ein beendeter Broadcast lässt sich samt Empfängerliste löschen;
Unterhaltungen, die Antworten geöffnet haben, bleiben.

Vorübergehende SMTP-Fehler werden nach 5 Minuten, 30 Minuten, 2 Stunden und 6 Stunden wiederholt, dann
gilt die Mail als fehlgeschlagen. Eine SMTP-Ablehnung der Adresse selbst (5.1.x und 5.2.1) ist ein
Bounce.

### Das Tageslimit

Ein Workspace kann ein Limit für Empfänger pro Tag (UTC) haben, für alle seine Broadcasts zusammen. Es
kommt aus [`YUVA_BROADCAST_DAILY_RECIPIENTS`](configuration.md#broadcasts) des Servers, dem Limit des
Operators für den Workspace und auf einem gehosteten Server aus dem Warm-up
([unten](#freigabe-auf-gehosteten-servern)); das kleinste gilt. Erreicht ein Versand es, wartet der
Broadcast, und das Panel zeigt, wann er um 00:00 UTC weitergeht. Ein Broadcast an 250 Empfänger bei einem
Limit von 100 am Tag dauert drei Tage. Die Prüfung warnt vor dem Start; Settings → Workspace →
Broadcasts zeigt die heutige Zahl neben dem Limit.

### Pausen, die Yuva selbst macht

Ein Broadcast pausiert von selbst, mit dem Grund auf der Seite, wenn:

| Grund | Auslöser |
|---|---|
| Bounce-Rate | 5 % oder mehr der seit dem Start oder der letzten Fortsetzung gesendeten Mails sind gebounct (nach mindestens 200 Mails). |
| Beschwerderate | 0,3 % oder mehr wurden als Spam gemeldet (nach mindestens 1.000 Mails). |
| Absenderfehler | Der SMTP-Server lehnt das Konto oder sein TLS ab, lehnt 20 Mails hintereinander ab oder ist 30 Minuten nicht erreichbar. Seine Antwort wird angezeigt. |
| Workspace gesperrt | Siehe unten. |
| Amazon-SES-Tenant | Amazon SES hat den [Tenant](email.md#amazon-ses-tenants) des Posteingangs pausiert, oder der Kanal hat seine Tenant-Isolation verloren. Die Seite sagt, was von beidem. Andere Posteingänge senden weiter. |
| Mitglied, Operator | Jemand hat pausiert. |

Beheben Sie die Ursache und setzen Sie dann fort. Nach dem Fortsetzen zählen die Raten ab diesem

Fortsetzen wird mit `409 ses_tenant_blocked` abgelehnt, solange der Tenant pausiert ist; der Operator des Servers hebt die Pause auf.
Moment.

Ein Workspace wird vom Versand **gesperrt**, wenn in den letzten 30 Tagen mindestens 0,3 % von
mindestens 1.000 Broadcast-Mails als Spam gemeldet wurden oder mindestens 8 % von mindestens 2.000
gebounct sind. Seine laufenden Broadcasts pausieren, und Planen und Fortsetzen werden abgelehnt. Nur der
Operator des Servers hebt eine Sperre auf: Ein Owner sieht den Grund unter Settings → Workspace →
Broadcasts.

### Bounces und Beschwerden

Yuva liest Bounces von Amazon SES (über SNS, siehe [Bounces](email.md#bounces)), aus Zustellberichten an
den Kanal, aus SMTP-Ablehnungen beim Versand und Missbrauchsberichte (ARF) an den Kanal; Letztere zählen
nur von einem Absender, dessen DMARC-Ergebnis der empfangende Server bestätigt.

- Ein harter Bounce sperrt die Adresse für alle Mails des Workspaces.
- Eine Spam-Beschwerde über einen Broadcast beendet Broadcasts an diese Adresse, meldet sie von jeder
  Liste des Workspaces ab und merkt sich den Widerspruch. Support-Antworten können sie weiter erreichen.

## 5. Antworten

Eine Antwort auf einen Broadcast eröffnet mit diesem Kontakt eine Unterhaltung im Posteingang des
Broadcasts, markiert als „Reply to {Broadcast}“ und mit ihm verknüpft. Weitere Antworten setzen die
Unterhaltung fort. Im Posteingang zeigt der Filter nach Typ mit „Replies to a broadcast“ sie an, und der
Bericht des Broadcasts listet die ersten 50.

Automatische Antworten (Abwesenheit) werden nicht als Unterhaltungen gespeichert: Sie werden nur als
`auto_replied` gezählt, damit ein Versand den Posteingang nicht füllt. Mail von jemand anderem als dem
Empfänger, etwa eine Weiterleitung, eröffnet eine eigene Unterhaltung.

Mitglieder bekommen pro Broadcast eine Push-Benachrichtigung über seine neuen Antworten, alle 10 Minuten
gebündelt, und keine E-Mail dazu.

## 6. Berichte

Die Broadcast-Seite zeigt ihren Bericht, sobald der Versand beginnt.

| Zahl | Bedeutung |
|---|---|
| Recipients | Die beim Start des Versands festgelegte Zielgruppe. |
| Sent | Mails, die der SMTP-Server angenommen hat. |
| Failed | Mails, die dauerhaft abgelehnt wurden, ohne eine schlechte Adresse zu nennen. |
| Bounced | Empfänger, deren Mail gebounct ist. |
| Reported as spam | Empfänger, die sich beschwert haben. |
| Unsubscribed | Empfänger, die sich über die Links dieses Broadcasts von einer Liste abgemeldet haben. |
| Replied | Empfänger, die geantwortet haben; jeder ist eine Unterhaltung. |
| Automatic replies | Abwesenheitsantworten, nur gezählt. |
| Clicked | Empfänger, die einen Link geöffnet haben; nur mit Klick-Tracking. |
| Skipped | Ausgelassen, weil gesperrt, widersprochen, vor ihrer Mail abgemeldet oder der Broadcast abgebrochen wurde. |

Raten sind Anteile der gesendeten Mails. Die Zeitleiste ist stündlich für die ersten 72 Stunden nach dem
Start, danach täglich. Die Empfängerliste zeigt den Status jeder Person und bei Fehlern die SMTP-Antwort;
filtern Sie nach Status, suchen Sie nach Adresse oder Name oder zeigen Sie nur die, die geantwortet
haben. Die Empfängerliste wird 400 Tage nach dem Ende des Broadcasts gelöscht; die Zahlen bleiben.

### Klick-Tracking

Standardmäßig aus. Eingeschaltet laufen die Links der Mail über `/c/…` auf der Link-Domain des Servers,
und der Bericht zeigt eindeutige Klicks pro Link. Nur Web-Links, die für alle Empfänger gleich sind (ohne
Merge-Feld), werden verfolgt; sie werden beim Planen gefunden. Klicks sind ungefähr: Eine `HEAD`-Anfrage
und drei Links einer Mail, die innerhalb von zwei Sekunden geöffnet werden, wie es Sicherheitsscanner
tun, werden nicht gezählt.

### Kampagnen-Tracking (UTM)

Standardmäßig an. Web-Links in der Mail bekommen `utm_source=newsletter`, `utm_medium=email` und
`utm_campaign=<Kampagne>`, damit [Yuva Analytics](analytics.md) die Besuche zählt, die die Mail auf Ihre Website
bringt, unter dem Channel E-mail. Die Kampagne wird aus dem Namen des Broadcasts gebildet; ändern Sie sie im Entwurf
(1 bis 100 Kleinbuchstaben, Ziffern, `-`, `_` und `.`). Links, die schon einen eigenen `utm_`-Parameter haben, Links,
die nicht `http` oder `https` sind, und Yuvas eigene Links (Abmelden, Einstellungen, Klicks) bleiben unberührt.
Schalten Sie es in den Optionen aus oder mit `utm: false` im Entwurf. Broadcasts, die schon über den Entwurf hinaus
waren, bevor es das gab, behalten ihre Links, wie sie gesendet wurden.

Der Bericht zeigt dann **Besuche aus dieser Kampagne**: pro Website, die Sie sehen dürfen, die Besucher, Besuche und
Seitenaufrufe, deren `utm_campaign` die des Broadcasts ist, ab dem Tag, an dem der Versand begann (mit
`analytics:read`; `campaign_visits` in der API). Mehr unter
[Support und Traffic](analytics-support.md#broadcast-kampagnen-verfolgen).

Es gibt kein Öffnungs-Tracking und kein Tracking-Pixel. Öffnungsraten sind unzuverlässig, weil
Mail-Apps Bilder vorab laden, und ein Pixel wäre das Einzige in der Mail, das an Dritte meldet.

## 7. Keys und Assistenten

Versenden ist das Einzige, das API-Keys und OAuth-Tokens standardmäßig nicht bekommen:

- Der Scope `broadcasts:send` ist nie impliziert; nennen Sie ihn beim Anlegen eines Keys ausdrücklich.
- Auch dann kann ein Key oder Token Broadcasts nur planen und fortsetzen, solange ein Owner oder Admin
  unter Settings → Workspace → Broadcasts „Keys and assistants may send broadcasts“ eingeschaltet hat.
  Der Schalter ist standardmäßig aus, und das Umschalten erfordert eine Mitgliedssitzung. Sonst
  antwortet das Planen mit `403 broadcasts_by_keys_disabled`.
- Entwürfe anlegen, ändern und testen braucht `broadcasts:write`, Lesen `broadcasts:read`. Pausieren und
  Abbrechen brauchen `broadcasts:send` und werden vom Schalter nicht zurückgehalten.
- Über einen [Assistenten](mcp.md#listen-designs-und-einrichtung-über-einen-assistenten) werden `pause_broadcast`, `resume_broadcast`, `unschedule_broadcast` und `cancel_broadcast` wie `schedule_broadcast` nur aufgelistet, solange der Schalter an ist.

## 8. Ausgelöste E-Mails

Eine ausgelöste E-Mail sendet ein [Design](designs.md) einmal an jeden Kontakt eines Posteingangs, eine
festgelegte Zeit nachdem der Kontakt dort zum ersten Mal erkannt wurde: in dem Moment, in dem eine App oder Ihr
Backend dem Kontakt seine erste externe ID gibt (über ein [Identitäts-Token](identity.md), `POST /v1/contacts` oder
ein Formular, das eine ID mitbringt). Nutzen Sie sie für eine Willkommensmail einen Tag nach der Anmeldung oder eine
Nachfrage "Wie läuft es?" eine Woche später. Sie ist ein Broadcast mit `kind` `triggered`; Design, Betreffs je
Sprache, Merge-Felder, Prüfung, Testversand, Klick-Tracking, Tempo, Versandzeiten und Tageslimit funktionieren wie
oben beschrieben. Nichts geht auf einmal an eine Liste: Kontakte werden einzeln erfasst, sobald sie erkannt werden.

### Einrichten

Legen Sie im Panel im Posteingang eine ausgelöste E-Mail statt eines einmaligen Broadcasts an. Oder per API mit
`kind: "triggered"` und einem `trigger`:

```sh
curl -X POST https://support.example.com/v1/broadcasts \
  -H "Authorization: Bearer $YUVA_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "inbox_id": "INBOX-ID",
    "channel_id": "CHANNEL-ID",
    "name": "Welcome",
    "subject": "Welcome, {{ contact.first_name | \"there\" }}",
    "design_id": "DESIGN-ID",
    "kind": "triggered",
    "trigger": { "event": "contact_identified", "delay_minutes": 1440, "basis": "service" }
  }'
```

- **Ereignis.** `contact_identified` ist das einzige Ereignis.
- **Verzögerung.** Von 0 Minuten bis 365 Tage nach dem Auslöser. Die Mail wartet so lange und wird dann unter
  Tempo, Versandzeiten und Tageslimit von Broadcasts versendet; sind die Zeiten dann geschlossen, wartet sie auf deren
  Öffnung. Ausgelöste E-Mails werden vor den einmaligen Broadcasts desselben Kanals versendet.
- **Versandzeiten.** `timezone` und `send_window` funktionieren wie bei [Broadcasts](#versandtempo) und lassen sich
  ändern, während die E-Mail läuft.
- **Zielgruppe.** Bedingungen und Sprachen (bis zu 10 und 20) werden zur Fälligkeit des Kontakts geprüft, nicht bei
  der Erkennung. Wer nicht mehr passt, wird übersprungen.

### Marketing oder Service

Die Einwilligungsgrundlage wird beim Anlegen gewählt und lässt sich danach nicht ändern.

| Grundlage | Geht an | Fußzeile | Braucht |
|---|---|---|---|
| `marketing` | Nur Kontakte, die eine ihrer 1 bis 10 `audience.list_ids` abonniert haben. | Die übliche Broadcast-Fußzeile mit Ein-Klick-Abmeldung von der Liste. | Listen. |
| `service` | Jeden passenden Kontakt, keine Listen. | Eine Fußzeile, die sagt, dass die Person das Produkt des Posteingangs nutzt, mit Ihrer Postadresse und einem Ein-Klick-Stopp. | `confirm_service: true` beim Start. |

Wählen Sie `service` nur für Mails über die Nutzung des Produkts: Einstieg, Konto, Bitte um Feedback. Werbung und
Angebote sind immer `marketing`. Eine `service`-E-Mail darf das Merge-Feld `list.name` nicht verwenden. Eine
`service`-E-Mail ohne `confirm_service: true` zu starten, ergibt `400 service_basis_unconfirmed`; Yuva speichert, wer
wann bestätigt hat, und das Audit-Log hält den Start fest.

Der Ein-Klick-Stopp einer Service-Fußzeile beendet für diese Person die ausgelösten Service-E-Mails des Posteingangs
und [gestaltete Nachrichten](designs.md#8-ein-design-in-einer-unterhaltung-verwenden); er meldet sie von keiner Liste ab.
Beide Grundlagen überspringen Adressen, die gebounct sind oder sich beschwert haben, und Personen, die jeder Mail
des Workspace widersprochen haben.

### Starten, Rückwirkung und Stoppen

Das Starten funktioniert wie das Planen (`POST /v1/broadcasts/{broadcastId}/schedule`): zu `send_at` (bis dahin
`scheduled`) oder sofort wird sie `active`. Zuerst muss die Prüfung bestehen. Von da an wird jeder Kontakt, dessen
Auslöser eintritt, einmal erfasst; niemand wird zweimal erfasst.

Kontakte, die vor dem Start erkannt wurden, bekommen nichts, außer Sie nutzen die **Rückwirkung**: `backfill_days`
(0 bis 30, beim Start oder in `trigger`) erfasst auch Kontakte, deren Auslöser in dieser Zahl von Tagen vor dem
Start eintrat. Die Verzögerung zählt ab dem Auslöser, aber niemand ist vor dem Start fällig. Die Prüfung zählt sie,
und `confirm_recipients` muss dieser Zahl entsprechen (0 ohne Rückwirkung).

Kontakte, die Sie aus Ihrem eigenen Backend hineinkopieren, sind ebenfalls ein Auslöser: ein Nutzer, den das
Produkt schon hatte, bekäme eine "Willkommen"-Mail. Senden Sie `"skip_triggers": true` mit `POST /v1/contacts` oder
`PATCH /v1/contacts/{contactId}`, damit die neue externe ID niemanden erfasst. Importe, Help-Desk-Umzüge und
Zusammenführungen lösen nie aus.

| Aktion | Wirkung |
|---|---|
| Pausieren (`POST .../pause`) | Stoppt das Senden. Neue Auslöser werden weiter erfasst und warten. |
| Fortsetzen (`POST .../resume`) | Führt die Prüfung erneut aus und friert das Design im jetzt gespeicherten Stand ein. Kontakte, deren Fälligkeit mehr als 7 Tage zurückliegt, werden als `late` übersprungen. |
| Stoppen (`POST .../cancel`) | Hält für immer an. Erfasst nicht mehr und verwirft die Wartenden. Eine gestoppte E-Mail lässt sich nicht neu starten; legen Sie eine neue an. |

Solange sie **aktiv** ist, können Sie Tempo, Zeitzone, Versandzeiten, die Verzögerung (sie verschiebt die
Fälligkeit aller Wartenden) und das Klick-Tracking ändern. Solange sie **pausiert** ist, auch Design, Betreffs,
Vorschautext, Übersetzungen, Bedingungen und Sprachen; das Fortsetzen prüft sie erneut. Ereignis, Grundlage, Listen
und Kanal ändern sich nach dem Entwurf nie; legen Sie dafür eine neue E-Mail an. Pausieren, Fortsetzen und Stoppen
brauchen `broadcasts:send`.

### Wer wartet

Die Broadcast-Seite listet die erfassten Kontakte: wartend mit dem Zeitpunkt, an dem ihre Mail fällig ist,
versendet und übersprungen mit dem Grund. Über die API listet `GET /v1/broadcasts/{broadcastId}/enrollments` sie
auf (`state`: `waiting`, `queued`, `skipped`, `cancelled`); bei `waiting` zuerst der am frühesten Fällige. Soll ein
wartender Kontakt die Mail nicht bekommen, nimmt ihn `DELETE /v1/broadcasts/{broadcastId}/enrollments/{contactId}`
heraus: er wird als `removed` übersprungen und nie wieder erfasst. Nur ein wartender Kontakt lässt sich entfernen
(sonst `409 invalid_state`).

| Grund | Bedeutung |
|---|---|
| `no_address` | Der Kontakt hat keine E-Mail-Adresse. |
| `blocked` | Sie haben den Kontakt gesperrt. |
| `suppressed` | Die Adresse ist gebounct oder hat sich beschwert. |
| `opted_out` | Die Person widersprach jeder Mail des Workspace. |
| `stopped` | `service`: die Person hat solche Mails des Posteingangs gestoppt. |
| `not_subscribed` | `marketing`: auf keiner ihrer Listen. |
| `not_matching` | Eine Bedingung oder Sprache passt nicht. |
| `late` | Mehr als 7 Tage vor der Verarbeitung fällig, nach langer Pause oder wegen des Tageslimits. |
| `removed` | Von der Warteliste genommen. |

Der Bericht ergänzt erfasste und wartende Kontakte und die Übersprungenen nach Grund. Die Zahlen sind innerhalb von
5 Minuten aktuell. Pro erfasstem Kontakt wird kein Ereignis und kein Webhook gesendet.

Pro Posteingang können höchstens 20 ausgelöste E-Mails gleichzeitig geplant, aktiv oder pausiert sein
(`409 too_many_triggered`). Ein Merge-Feld ohne Fallback ist in der Prüfung nur eine Warnung, da die Kontakte noch
nicht bekannt sind.

## Freigabe auf gehosteten Servern

Ein Server, der die Anmeldung für alle öffnet (`YUVA_SIGNUP=open`), betreibt Broadcasts standardmäßig im
Modus `approval`, damit ein nachlässiger Workspace den Versandruf des ganzen Servers nicht beschädigen
kann. Wenn Sie Ihren eigenen Server nur mit Einladungen betreiben, gilt das nicht für Sie.

Im Modus `approval` kann ein über die Anmeldung angelegter Workspace erst Broadcasts planen, wenn der
Operator ihn freigibt. Entwürfe schreiben und Tests senden können Sie in der Zwischenzeit.

1. Öffnen Sie Settings → Workspace → Broadcasts. Ein Owner sieht den Status: allowed, needs the
   operator's approval, waiting for the operator oder suspended.
2. Wählen Sie Ask for approval und tragen Sie Ihre Website ein, wer sendet und worum es in den Mails
   geht, und wie Ihre Abonnenten zugestimmt haben. Genau das liest der Operator.
3. Der Status wechselt auf „Waiting for the operator“. Der Operator gibt frei oder lehnt mit einer
   Begründung ab, die Sie sehen; nach einer Ablehnung können Sie erneut anfragen. Bis dahin zeigt die
   Prüfung „Permission to send“ als blockierend.

Freigegebene Workspaces und jeder Workspace auf einem `approval`-Server wärmen sich auf: Sie dürfen am
ersten Tag an 500 Empfänger senden, und das Limit verdoppelt sich nach jedem sauberen Tag (mindestens 100
Mails gesendet, Bounces unter 3 %, Beschwerden unter 0,1 %). Ein schlechter Tag nimmt eine Verdopplung
zurück. Ein Workspace hat auf einem solchen Server höchstens 1.000 Broadcasts; löschen Sie alte.

Auf einem Server, auf dem Broadcasts `off` sind, funktionieren Listen und Formulare weiter, aber
Broadcasts anlegen, ändern und senden antwortet mit `403 broadcasts_disabled`.

## Zustellbarkeit

Mail eines neuen Absenders landet im Spam, wenn die Grundlagen nicht stimmen. Der [Leitfaden zur
Zustellbarkeit](deliverability.md) behandelt sie: die Prüfung, die SPF, DKIM und DMARC eines Kanals testet, die
Einrichtung einer Domain mit Amazon SES, Postmark oder einem anderen Relay, Yuva DKIM, die Regeln von Gmail und
Yahoo, Google Postmaster Tools und das Aufwärmen.

- **Schreiben Sie nur Personen an, die zugestimmt haben.** Eine Liste aus Einwilligungsnachweisen ist Ihr
  bester Schutz.
- **Fangen Sie klein an.** Senden Sie zuerst an ein paar hundert interessierte Abonnenten und steigern Sie
  sich.
- **Nutzen Sie ein eigenes Konto** für Broadcasts, wenn Sie möchten von einer Subdomain wie
  `news.example.com`.

Yuva setzt auf jede Broadcast-Mail `List-Unsubscribe` mit One-Click (RFC 8058), `List-Id`, `Feedback-ID`
und `X-Auto-Response-Suppress`, damit Mail-Programme ihren eigenen Abmelde-Button zeigen.

## Die API

Das Panel nutzt dieselbe API wie alles andere. Ein Broadcast aus einem Skript:

```sh
curl -X POST https://support.example.com/v1/broadcasts \
  -H "Authorization: Bearer $YUVA_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "inbox_id": "INBOX-ID",
    "channel_id": "CHANNEL-ID",
    "name": "October newsletter",
    "subject": "What is new in October",
    "design_id": "DESIGN-ID",
    "audience": { "list_ids": ["LIST-ID"] },
    "pace": { "per_hour": 10, "per_day": 100 }
  }'
```

`PATCH` mit `pace` allein ändert das Tempo eines geplanten oder laufenden Broadcasts; `{}` entfernt es.

Danach `GET /v1/broadcasts/{broadcastId}/review` für die Checkliste, `POST .../audience` für die
Empfängerzahl und `POST .../test` für eine Testmail an ein Mitglied (ein API-Key nennt sie in
`member_ids`). Keys brauchen die [Scopes unten](#scopes). Planen Sie mit `confirm_recipients` auf der
Zahl, die Sie gesehen haben:

```sh
curl -X POST https://support.example.com/v1/broadcasts/BROADCAST-ID/schedule \
  -H "Authorization: Bearer $YUVA_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"send_at": "2026-10-20T08:00:00Z", "confirm_recipients": 1280}'
```

## Referenz

### Endpunkte

| Endpunkt | Zweck |
|---|---|
| `GET`, `POST /v1/broadcasts` | Entwürfe auflisten (`inbox_id`, `state`, `kind`) und Entwürfe oder [ausgelöste E-Mails](#8-ausgelöste-e-mails) anlegen (`kind: "triggered"`, `trigger`). |
| `GET`, `PATCH`, `DELETE /v1/broadcasts/{broadcastId}` | Lesen, einen Entwurf ändern (oder nur das Tempo eines geplanten oder laufenden), einen Entwurf oder beendeten Broadcast löschen. |
| `POST /v1/broadcasts/{broadcastId}/audience` | Die Zielgruppe zählen: Empfänger, wer warum ausgelassen wird, Merge-Felder, die leer wären. |
| `GET /v1/broadcasts/{broadcastId}/review` | Die Checkliste. |
| `POST /v1/broadcasts/{broadcastId}/test` | Einen Test an Mitglieder senden (`member_ids`, `contact_id`). |
| `POST /v1/broadcasts/{broadcastId}/schedule` | Planen oder sofort senden (`send_at`, `confirm_recipients`). |
| `POST /v1/broadcasts/{broadcastId}/unschedule` | Zurück zum Entwurf, vor dem Start. |
| `POST /v1/broadcasts/{broadcastId}/pause`, `resume`, `cancel` | Einen laufenden Broadcast steuern. |
| `GET /v1/broadcasts/{broadcastId}/deliveries` | Die Empfänger (`state`, `q`, `replied`). |
| `GET /v1/broadcasts/{broadcastId}/enrollments` | Die Kontakte einer ausgelösten E-Mail (`state`): wer wann wartet, wer die Mail bekam, wer übersprungen wurde. |
| `DELETE /v1/broadcasts/{broadcastId}/enrollments/{contactId}` | Einen wartenden Kontakt aus einer ausgelösten E-Mail nehmen. |
| `GET /v1/broadcasts/{broadcastId}/report` | Zahlen, Raten, Zeitleiste, Links und Antworten. |
| `GET /v1/conversations?broadcast_id=` | Unterhaltungen, die Antworten auf einen Broadcast geöffnet haben. |
| `PATCH /v1/workspace` | `broadcasts_by_keys`, der Schalter für Keys und Assistenten. |
| `POST /v1/workspace/broadcast-approval` | Den Operator um Freigabe bitten (Owner, Mitgliedssitzung). |

`GET /v1/workspace` liefert ein Objekt `broadcasts` mit dem `mode` des Servers sowie `status`,
`daily_limit`, `sent_today` und `by_keys` des Workspaces. `email.broadcast_smtp` und
`email.broadcast_rate` eines Kanals setzen Sie mit `PATCH /v1/channels/{channelId}`
(`broadcast_smtp_remove: true` entfernt das Konto). Die vollständigen Formen stehen im
[API-Vertrag](../../openapi/openapi.yaml). Fehler, die Sie behandeln sollten: `409 review_failed` (mit
den blockierenden `items`), `409 audience_changed`, `409 not_draft`, `409 invalid_state`, `409 pace_too_slow`,
`403 broadcasts_approval_required`, `403 broadcasts_suspended`, `403 broadcasts_by_keys_disabled`,
`403 broadcasts_disabled` und `429 test_limit`.

### Scopes

| Scope | Bedeutung |
|---|---|
| `broadcasts:read` | Broadcasts, Zielgruppenzahlen, Prüfungen und Berichte lesen; die Empfängerliste und die erfassten Kontakte einer ausgelösten E-Mail brauchen zusätzlich `contacts:read`. |
| `broadcasts:write` | Entwürfe anlegen, ändern und löschen; Tests senden; einen Kontakt aus einer ausgelösten E-Mail nehmen. |
| `broadcasts:send` | Planen, Planung zurücknehmen, pausieren, fortsetzen und abbrechen. Nie impliziert. |

Keys, die vor diesen Scopes angelegt wurden, bekommen sie nicht.

### Ereignisse

| Name | Bedeutung |
|---|---|
| `broadcast.updated` | [Webhook](webhooks.md)- und Event-Feed-Ereignis: Der Broadcast hat den Status oder sein Tempo gewechselt. `data.broadcast` ist der Broadcast, `data.previous_state` der alte Status. |
| `broadcast.progress` | Nur Realtime (`/v1/realtime`): die Zahlen eines vorbereitenden oder sendenden Broadcasts, höchstens alle 5 Sekunden. Nicht gespeichert und nicht wiedergegeben; lesen Sie den Broadcast nach einem Neuaufbau der Verbindung erneut. |

Beide erreichen Aufrufer mit `broadcasts:read`, die den Posteingang des Broadcasts sehen können.
Antworten fügen der Zeitleiste der Unterhaltung einen Eintrag `broadcast_reply` hinzu.

### Grenzen

| Grenze | Wert |
|---|---|
| Listen pro Broadcast | 10 |
| Bedingungen, Sprachen | 10, 20 |
| Name, Betreff, Vorschautext | 200, 300, 300 Zeichen |
| Vorausplanen | 90 Tage |
| Testversand | 20 pro Stunde und Workspace; je 1 bis 5 Mitglieder |
| Pause und Abbruch wirken innerhalb von | einem Stapel, höchstens 50 Mails |
| Tempo | 1 bis 100 Mails pro Sekunde und Kanal; standardmäßig 5 |
| Broadcast-Tempo | 1 bis 10.000.000 pro Stunde, Tag und 30-Tage-Monat; höchstens 365 Tage pro Versand |
| Wiederholungen bei vorübergehendem Fehler | nach 5 Minuten, 30 Minuten, 2 Stunden, 6 Stunden |
| HTML | 90 KB Warnung, 102 KB blockiert, 500 KB abgelehnt |
| Broadcasts pro Workspace | 1.000 auf einem `approval`-Server |
| Empfängerliste aufbewahrt | 400 Tage nach dem Ende des Broadcasts |
| Ausgelöste E-Mails pro Posteingang | 20 geplant, aktiv oder pausiert |
| Verzögerung einer ausgelösten E-Mail | 0 Minuten bis 365 Tage |
| Rückwirkung | 0 bis 30 Tage |
