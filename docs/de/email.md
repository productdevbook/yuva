# E-Mail

Ein E-Mail-Kanal gibt einem Posteingang eine Support-Adresse. Eine E-Mail an sie eröffnet eine
Unterhaltung oder setzt sie fort, und Antworten gehen von dieser Adresse im selben Thread hinaus.

## Den Kanal anlegen

Öffnen Sie im Panel **Settings → den Posteingang → Channels → E-mail**. Oder auf dem Server:

```sh
yuva channel create-email --workspace Example --inbox support --name "Support" \
  --address support@example.com --display-name "Example Support" \
  --smtp-host email-smtp.eu-west-1.amazonaws.com --smtp-username <user> --smtp-password-file -
```

Das SMTP-Konto versendet die Antworten ([Ausgehendes SMTP](#ausgehendes-smtp)). Leiten Sie dann
eingehende E-Mails auf einem der drei Wege unten zu Yuva und richten Sie das [DNS](#dns-checkliste)
ein.

## Adresse nachweisen

Ein Kanal empfängt E-Mails an seiner eigenen Adresse (ein MX-Eintrag, der auf Yuva zeigt,
`/ingress/email` oder `yuva ingest-email --to`) erst, nachdem sein Workspace die Adresse nachgewiesen hat.
Bis dahin empfängt er weiter E-Mails an seiner [Weiterleitungsadresse](#weiterleitungsadresse) und sendet
Antworten über sein SMTP-Konto. `email.verification` des Kanals enthält den Zustand (`state`: `unverified`
oder `verified`), die Methode, mit der die Adresse nachgewiesen wurde (`method`: `code`, `dns` oder
`operator`), den hinzuzufügenden TXT-Eintrag und, solange ein Code offen ist, wann er abläuft.

| Weg | Für | So geht es |
|---|---|---|
| Code | Eine genaue Adresse, nicht eine auf der Weiterleitungsdomain | `POST /v1/channels/{channelId}/verification-code` sendet einen 8-stelligen Code an die Adresse. Senden Sie ihn mit `POST /v1/channels/{channelId}/verify` und `{"code": "…"}` zurück. Ein Code gilt eine Stunde und für 5 Versuche; ein neuer ersetzt den alten. Leitet das Postfach an die Weiterleitungsadresse des Kanals weiter, weist die Code-E-Mail den Kanal von selbst nach. |
| DNS | Jede Adresse; der einzige Weg für einen [Catch-all](#catch-all-adressen) | Legen Sie den TXT-Eintrag `email.verification.txt_name` (`_yuva-mail.<domain>`) mit dem Wert `txt_value` (`yuva-mail-verify=…`) an und rufen Sie dann `POST /v1/channels/{channelId}/verify` ohne Body auf. Ein Eintrag weist alle Kanäle des Workspace auf dieser Domain nach. |
| Operator | Eine Adresse, bei der der Operator dem Workspace vertraut | Der Operator des Servers führt `yuva channel prove` aus ([Betrieb](operations.md#e-mail-adressen-müssen-nachgewiesen-werden)). |

Die ersten beiden Wege gehen für Owner, Admins und API-Schlüssel mit `inboxes:manage`. Ein falscher oder
abgelaufener Code antwortet `400 verification_code_invalid`; eine DNS-Prüfung ohne Fund lässt den Kanal mit
einem `problem` auf `unverified`. Code-E-Mails sind auf eine pro Minute und Kanal, 20 pro Tag und Workspace
und 5 pro Tag an dieselbe Adresse serverweit begrenzt (`429 rate_limited`), DNS-Prüfungen auf 30 pro Stunde
und Workspace.

- Ein Catch-all auf der Weiterleitungsdomain des Servers oder bei einem öffentlichen Mail-Anbieter
  (gmail.com, outlook.com, …) wird abgelehnt (`400 validation_failed`): Er würde die E-Mails anderer
  empfangen.
- Mehrere Workspaces können dieselbe Adresse beanspruchen, aber nur der, der sie zuletzt nachgewiesen hat,
  empfängt ihre E-Mails. Weist ein anderer Workspace eine Adresse nach, die Sie nachgewiesen hatten, geht Ihr
  Kanal mit dem Problem `taken` zurück auf `unverified`, und Ihre Owner bekommen eine E-Mail (höchstens
  eine pro Tag, mit den Adressen). Weisen Sie sie erneut nach, um sie zurückzuholen. Ein nicht
  nachgewiesener Anspruch blockiert niemanden.
- Wer eine genaue Adresse nachweist, übernimmt auch E-Mails, die ein Catch-all- oder `local@`-Kanal eines
  anderen Workspace bis dahin dafür empfing; dieser Workspace wird auf dieselbe Weise benachrichtigt.
- Wer die Adresse eines Kanals ändert, beginnt den Nachweis von vorn.
- Jeder Nachweis und Verlust steht im [Audit-Log](operations.md#audit-log).

## Eingehende E-Mails

Yuva nimmt selbst kein SMTP entgegen. Wählen Sie einen dieser drei Wege, um E-Mails hereinzuholen.

### Cloudflare Email Worker

`edge/` ist ein Cloudflare Email Worker, der E-Mails an Yuva weiterleitet. Cloudflare Email Routing
wird zum MX der Domain; nehmen Sie also eine Domain oder Subdomain, die kein anderer Mailserver
bedient.

1. Setzen Sie auf dem Server diese Werte und starten Sie neu:

   ```sh
   YUVA_INGRESS_SECRET=<openssl rand -hex 32>
   YUVA_INGRESS_AUTHSERV_ID=mx.cloudflare.net
   ```

2. Deployen Sie den Worker mit Ihrem Cloudflare-Konto:

   ```sh
   cd edge
   bun install
   bunx wrangler login
   bunx wrangler secret put INGRESS_SECRET          # derselbe Wert wie YUVA_INGRESS_SECRET
   bunx wrangler deploy --var YUVA_URL:https://support.example.com
   ```

3. Öffnen Sie im Cloudflare-Dashboard die Domain → **Email → Email Routing**, aktivieren Sie es und
   lassen Sie es seine MX- und SPF-Einträge anlegen.
4. Leiten Sie unter **Routing rules** jede Support-Adresse an den Worker `yuva-edge`. Für einen
   [Catch-all-Kanal](#catch-all-adressen) richten Sie die Catch-all-Regel auf ihn.
5. Senden Sie eine Test-E-Mail und sehen Sie zu, wie die Unterhaltung im Panel erscheint.

Ist Yuva nicht erreichbar oder ausgelastet, versucht es der Absender später erneut. Um solche E-Mails
zu behalten, deployen Sie mit `--var FALLBACK_FORWARD:you@example.org` (eine verifizierte
Email-Routing-Adresse): Sie gehen dann dorthin.

Ein älterer Email Worker signiert mit `v1`, was der Server ablehnt. Deployen Sie ihn neu, oder setzen
Sie bis dahin `YUVA_INGRESS_ACCEPT_V1=true`.

Der Email Worker signiert mit `v3`, was der Server prüft, bevor er die Nachricht liest. Ein früher
deployter Worker signiert mit `v2`; das wird nur akzeptiert, solange `YUVA_INGRESS_ACCEPT_V2=true` ist
(standardmäßig aus). Bei `v2` wird die Nachricht gelesen, bevor feststeht, dass sie echt ist,
sodass jeder den Server unsignierte Uploads puffern lassen kann (Advisory GHSA-jfvx-vc82-56f7):
Deployen Sie den Worker neu, statt die Option einzuschalten. Ein Proxy vor der
API muss `Content-Length` an `/ingress/email` weitergeben: `v3` lehnt einen Body in Chunks
(chunked) ab.

### Beliebiger MTA: `yuva ingest-email`

Ein MTA, der an einen Befehl zustellt, leitet die Rohnachricht hinein:

```sh
yuva ingest-email --to <envelope recipient> [--from <envelope sender>] < message.eml
```

Der Befehl braucht `YUVA_DATABASE_URL`, `YUVA_MASTER_KEY` und die Speichereinstellungen des
Servers. Auf einem separaten Mail-Host verwenden Sie S3 als Speicher, damit beide an denselben Ort
schreiben. Kopieren Sie das Binary aus dem Image:

```sh
docker create --name yuva-bin ghcr.io/productdevbook/yuva:0.1.0
docker cp yuva-bin:/yuva /usr/local/bin/yuva && docker rm yuva-bin
```

Für Postfix schreiben Sie einen Wrapper `/usr/local/bin/yuva-ingest`, der die Einstellungen lädt:

```sh
#!/bin/sh
set -a
. /etc/yuva/ingest.env
exec /usr/local/bin/yuva ingest-email "$@"
```

Tragen Sie ihn in `master.cf` ein und leiten Sie die Support-Adressen in `transport_maps` dorthin
(`support@example.com yuva:`):

```
yuva      unix  -       n       n       -       -       pipe
  flags=q user=yuva argv=/usr/local/bin/yuva-ingest --to ${recipient} --from ${sender}
```

### Beliebiger MTA: HTTP

Senden Sie die Rohnachricht per POST an `/ingress/email`, signiert mit `YUVA_INGRESS_SECRET`. Das
Format steht in [edge/README.md](../../edge/README.md#ingress-contract). Setzen Sie
`YUVA_INGRESS_AUTHSERV_ID` auf die Authserv-ID, die Ihr MTA in `Authentication-Results` schreibt,
sonst wird DMARC nicht genutzt und kein Absender bekommt eine automatische Antwort. Ihr MTA muss jeder Nachricht, die er weitergibt, oben ein eigenes
`Authentication-Results` voranstellen: Yuva liest das oberste, eine Nachricht ohne den Header Ihres
MTA würde also nach einem Header beurteilt, den ihr Absender geschrieben hat.

## Weiterleitungsadresse

Hat der Server eine [Weiterleitungsdomain](configuration.md#zu-yuva-umziehen), empfängt jeder E-Mail-Kanal
E-Mails zusätzlich unter einer zweiten Adresse, `<Token>@<Weiterleitungsdomain>`. Leiten Sie ein Postfach
dorthin weiter, zum Beispiel von einem anderen Anbieter, wenn Sie [zu Yuva umziehen](migrate.md). Ein Postfach
an die eigene Adresse weiterzuleiten würde eine Schleife bilden; deshalb gibt es diese Adresse. E-Mails an sie
werden wie E-Mails an die eigene Adresse des Kanals behandelt.

Sie finden sie unter **Settings → den Posteingang → Channels → E-mail**, mit einer Kopieren-Taste. „Rotate“
gibt dem Kanal eine neue (`POST /v1/channels/{channelId}/forwarding-address/rotate`, `inboxes:manage`):
E-Mails an die alte Adresse werden danach abgelehnt, ändern Sie also die Weiterleitung beim Anbieter sofort.

Der Betreiber richtet die E-Mails der Weiterleitungsdomain einmal auf Yuva aus, auf demselben Weg wie die
Domains der Kanäle (die drei Wege unter [Eingehende E-Mails](#eingehende-e-mails)). Ohne Weiterleitungsdomain
haben Kanäle keine Weiterleitungsadresse.

## Ausgehendes SMTP

Jeder E-Mail-Kanal sendet über sein eigenes SMTP-Konto (SES, Postmark, Ihr eigenes Relay). Sie
setzen es in den Kanaleinstellungen. Ohne Konto empfängt der Kanal E-Mails, Antworten werden aber
abgelehnt (`email_not_configured`).

Chat- und Feedback-Antworten, die ein Kontakt nicht gelesen hat, gehen über den E-Mail-Kanal des
Posteingangs per E-Mail hinaus (nach `YUVA_CHAT_EMAIL_DELAY`, siehe
[Konfiguration](configuration.md#chat)). Geben Sie deshalb jedem Posteingang mit Chat- oder
App-Kanälen auch einen E-Mail-Kanal.

Private und Loopback-SMTP-Hosts, etwa Mailpit in der Entwicklung, brauchen
`YUVA_SMTP_ALLOW_PRIVATE=true`.

### Amazon-SES-Tenants

SES führt Reputation und Sendestopp pro Tenant. Hat der Operator des Servers die
[Tenant-Verwaltung](operations.md#amazon-ses-tenants-auf-einem-gehosteten-server) eingerichtet und sendet ein Kanal über
dessen SES-Konto, sendet jeder Posteingang in seinem eigenen Tenant: Bounces oder Beschwerden eines
Posteingangs pausieren nur diesen Posteingang, nie einen anderen Posteingang oder Workspace. Ihr Team muss
nichts tun. Die Sendeseite des Kanals und die Einstellungen des Posteingangs zeigen den Zustand, für Inhaber
und Admins:

| Zustand | Bedeutung |
|---|---|
| Isoliert | Mails tragen den Tenant des Posteingangs. |
| Wird eingerichtet | Der Server legt den Tenant an. Das dauert Sekunden; ein Broadcast wartet und startet von selbst. |
| Identität fehlt | Die Absenderadresse oder ihre Domain ist keine verifizierte Identität im SES-Konto. Siehe [Zustellbarkeit](deliverability.md#amazon-ses). Mails gehen ohne Tenant hinaus, Broadcasts sind blockiert. |
| Pausiert | Amazon SES hat den Tenant des Posteingangs pausiert. Antworten scheitern mit der Antwort von SES; der Operator des Servers hebt die Pause auf. |
| Fehler | Ein Schritt ist fehlgeschlagen; die Seite zeigt den Grund. |
| Fehlt | Der Tenant existiert in Amazon SES nicht mehr. Mails gehen ohne ihn hinaus, Broadcasts sind blockiert, bis der Server ihn neu anlegt. |

Mails über das verwaltete Konto tragen immer `X-SES-CONFIGURATION-SET` und `X-SES-TENANT`, sobald der Tenant
bereit ist. Beide Header kommen nur auf Mails über Amazon SES.

Mit einem eigenen SES-Konto, das der Server nicht verwaltet, tragen Sie den Tenant selbst ein: am Kanal
`ses.tenant` und, falls Sie eines nutzen, `ses.configuration_set` (Buchstaben, Ziffern, `-` und `_`, bis zu 64).
Yuva fügt sie als Header zu Mails über SES hinzu und prüft in AWS nichts. Sie werden ignoriert, solange der
Server den Tenant des Posteingangs verwaltet.

## Neue E-Mails schreiben

`POST /v1/conversations/mail` (MCP `compose_mail`) schreibt eine neue E-Mail über den E-Mail-Kanal
eines Posteingangs und legt ihre Unterhaltung in einem Schritt an. Sie geht per `To` an bis zu 50
Adressen, mit bis zu 50 in `Cc` und 50 in `Bcc`, unter dem angegebenen Betreff (ohne `Re:`). Die
erste `To`-Adresse wird zum Kontakt der Unterhaltung; die übrigen `To`- und die `Cc`-Adressen werden
beim Senden Beteiligte, sodass ihre Antworten in die Unterhaltung kommen. `Bcc`-Empfänger erhalten
die E-Mail, erscheinen aber nie darin. Mit `draft: true` wird die E-Mail als Entwurf gespeichert und
mit `POST /v1/messages/{id}/send` so gesendet, wie sie geschrieben wurde. Ein Mitglied oder
Schlüssel schreibt höchstens 60 neue E-Mails pro Minute und erreicht höchstens 300 Empfänger pro
Stunde. API-Schlüssel brauchen `conversations:write`, `messages:write` und `contacts:write`.

Spätere Antworten bestimmen, wer in Kopie steht: Eine Antwort ohne `cc` geht an den Kontakt und
setzt alle aktiven Beteiligten in Kopie (Allen antworten); `cc: []` geht nur an den Kontakt
(Antworten); eine `cc`-Liste setzt genau diese Adressen in Kopie. Ein Bounce für eine von mehreren
`To`-Adressen markiert nur diese Adresse als unzustellbar.

## E-Mails durchsuchen

Das Suchfeld der Konversationsliste (`q` von `GET /v1/conversations`, `query` von MCP `search_conversations`) nimmt
neben einfachen Wörtern auch Operatoren an. Schreiben Sie `schlüssel:wert`, `schlüssel:"zwei Wörter"`
oder `-schlüssel:wert`, um Treffer auszuschließen. Jeder Operator und jeder andere Filter muss
zutreffen. Schlüssel und Werte unterscheiden nicht zwischen Groß- und Kleinschreibung; ein Begriff, der
keiner dieser Operatoren ist oder dessen Wert nicht passt, wird als Text gesucht, sodass `ratio:3:1`
und Links weiter funktionieren. Ein Wert von `from:`, `to:` oder `cc:` braucht mindestens 3 Zeichen und kein Leerzeichen und kein Komma; Datumsangaben gelten von Jahr 1 bis 9999. Entwürfe anderer Mitglieder werden nie durchsucht.

| Operator | Trifft | Beispiel |
|---|---|---|
| `from:` | eine gesendete Nachricht, deren Absenderadresse oder -name den Wert enthält; `from:me` sind Sie | `from:ada@example.com` |
| `to:`, `cc:` | eine Nachricht mit einer Empfängeradresse, die den Wert enthält; `to:me` sind Sie | `to:me cc:@example.com` |
| `subject:` | den Betreff, wie ihn einfache Wörter treffen | `subject:"quarterly invoice"` |
| `has:attachment` | eine Nachricht mit Anhang | `has:attachment invoice` |
| `is:starred` | Konversationen, die Sie markiert haben | `-is:starred` |
| `in:` | einen Mail-Ordner: `inbox`, `starred`, `snoozed`, `sent`, `archive`, `trash`, `drafts` oder `scheduled` | `in:archive from:ada` |
| `after:`, `before:` | eine Nachricht, die ab oder vor Mitternacht eines Tages `YYYY-MM-DD` gesendet wurde | `after:2026-01-01 before:2026-02-01` |

`after:` und `before:` verwenden die Zeitzone, die der Client als `timezone` sendet (ohne Angabe UTC).
`in:trash` braucht dieselben Rechte wie `trashed=true`. Eine Suche hat höchstens 200 Zeichen und 20
Operatoren.

## Bounces

Ein Bounce oder eine Beschwerde markiert die Nachricht als fehlgeschlagen und die Adresse als
unzustellbar; Antworten an sie werden abgelehnt, bis ein Mitglied die Markierung am Kontakt
entfernt. Bounce-Berichte an die Kanaladresse brauchen keine Einrichtung. Für Amazon SES verbinden
Sie SNS:

1. Legen Sie ein SNS-Topic an, z. B. `yuva-ses`, in der Region Ihrer SES-Identität.
2. Senden Sie die Bounce- und Beschwerde-Benachrichtigungen der Identität (oder des Configuration
   Sets) an dieses Topic.
3. Setzen Sie `YUVA_SES_TOPIC_ARNS` auf die Topic-ARN und starten Sie Yuva neu.
4. Abonnieren Sie das Topic per HTTPS mit `https://support.example.com/ingress/ses`, mit
   ausgeschalteter Raw Message Delivery. Yuva bestätigt das Abonnement selbst.

## DNS-Checkliste

Für jede Domain, von der ein Kanal sendet:

| Eintrag | Wert |
|---|---|
| SPF | TXT, der Ihren SMTP-Anbieter einschließt, z. B. `v=spf1 include:amazonses.com ~all`. Ein SPF-Eintrag pro Name. |
| DKIM | Die CNAME- oder TXT-Einträge, die Ihr SMTP-Anbieter vorgibt. |
| DMARC | `_dmarc.example.com TXT "v=DMARC1; p=none; rua=mailto:dmarc@example.com"`; wechseln Sie zu `p=quarantine`, sobald die Berichte stimmen. |
| MX | Für eingehende E-Mails: die Einträge, die Email Routing anlegt, oder die Ihres MTA. |
| Custom MAIL FROM | Die MX- und SPF-Einträge, die Ihr Anbieter für die Bounce-Subdomain verlangt (SES und andere). |

Senden Sie eine Testantwort an ein Postfach, auf das Sie Zugriff haben, und prüfen Sie auf
`spf=pass`, `dkim=pass` und `dmarc=pass`.

## Referenz

### Catch-all-Adressen

Ein Kanal mit der Adresse `*@example.com` empfängt E-Mails an alle übrigen Adressen der Domain.
Antworten gehen von der Adresse hinaus, an die der Kontakt geschrieben hat; sein SMTP-Konto muss
deshalb für die ganze Domain senden dürfen. Geben Sie ihm eine `--from-address` für Chats, die per
E-Mail weitergehen. Ein Catch-all empfängt E-Mails erst, nachdem seine Domain
[mit dem TXT-Eintrag nachgewiesen](#adresse-nachweisen) ist, und nicht auf einer gemeinsam genutzten Domain.

```sh
yuva channel create-email --workspace Example --inbox support --name "Everything else" \
  --address '*@example.com' --from-address support@example.com \
  --smtp-host smtp.example.com --smtp-username support@example.com --smtp-password-file -
```

### So werden eingehende E-Mails verarbeitet

| Thema | Verhalten |
|---|---|
| Kanal | Gewählt nach dem Envelope-Empfänger, nur über eine nachgewiesene Adresse: zuerst die Weiterleitungsadresse des Kanals, dann die nachgewiesene genaue Adresse (`local+tag@domain` fällt auf ein nachgewiesenes `local@domain` zurück), dann der nachgewiesene Catch-all der Domain. Unbekannte und nicht nachgewiesene Adressen werden abgewiesen. |
| Threading | Über `In-Reply-To` und `References`; alles andere beginnt eine neue Unterhaltung. Eine Antwort öffnet die Unterhaltung wieder. |
| Cc und Beteiligte | Die `To`- und `Cc`-Adressen einer E-Mail, die nicht zum Workspace gehören, werden Beteiligte der Unterhaltung (höchstens 100). Aktiv sind sie nur, wenn die E-Mail DMARC bestanden hat (dafür ist `YUVA_INGRESS_AUTHSERV_ID` nötig); sonst sind sie vorgeschlagen, für Mitglieder sichtbar, aber nie in Kopie. Mitglieder können Beteiligte hinzufügen und entfernen. Die Antwort eines aktiven Beteiligten, die DMARC besteht, kommt als seine Nachricht in die Unterhaltung; jeder andere, der auf den Thread antwortet, beginnt eine neue Unterhaltung, die auf sie verweist. E-Mails, die DMARC nicht bestehen oder automatisch sind, fügen niemanden hinzu. Die eigenen Adressen des Workspace sind die Adressen des Kontakts, der Absender, jede Kanaladresse des Workspace (nachgewiesen oder nicht), seine Catch-all-Domains, Weiterleitungsadressen und der Absender des Servers; die Adresse eines anderen Workspace ist eine gewöhnliche Adresse. |
| Antworten | Gehen per `To` an den Kontakt und per `Cc` an die aktiven Beteiligten; automatische E-Mails (Begrüßungen, Regeln) setzen niemanden in Kopie. Jede Antwort kann das `Cc` ersetzen und `Bcc` hinzufügen (je 50 Adressen); `Bcc`-Empfänger erhalten die E-Mail, erscheinen aber nie darin, und nur Mitglieder sehen sie. Eine Adresse mit Bounce oder Beschwerde wird abgelehnt oder als Beteiligte ausgelassen; eine Abmeldung von Broadcasts stoppt keine Antworten. |
| Weiterleiten | Ein Mitglied kann eine Nachricht oder die ganze Unterhaltung (ihre Nachrichten, nie Notizen oder Entwürfe, mit oder ohne Dateien bis 20 MiB) mit einer eigenen Notiz darüber an bis zu 10 externe Adressen weiterleiten. Sie geht über den E-Mail-Kanal des Posteingangs mit dem Namen des Mitglieds, `Reply-To` der Kanaladresse und dem Betreff `Fwd: …` und ist nur für Mitglieder sichtbar, nie für den Kontakt. Die Adresse des Kontakts, Adressen, die schon in Kopie stehen, Yuvas eigene Adressen und Adressen mit Bounce oder Beschwerde werden abgelehnt; ein Mitglied leitet höchstens 20-mal pro Stunde weiter (200 pro Workspace). Eine Antwort auf die Weiterleitung, die DMARC besteht, kommt nur für Mitglieder sichtbar in die Unterhaltung und macht ihren Absender zu einem weitergeleiteten Beteiligten, der bei Antworten nicht in Kopie steht; jede andere E-Mail von ihm beginnt eine neue Unterhaltung. Über eine Antwort werden nur das weiterleitende Mitglied und die zugewiesene Person benachrichtigt. Assistenten leiten nur weiter, wo sie senden dürfen; sonst hinterlassen sie einen Entwurf. |
| Automatische E-Mails | Abwesenheitsnotizen, Massen-E-Mails und Zustellberichte werden gespeichert, aber nie beantwortet. E-Mails von der eigenen Adresse des empfangenden Kanals, vom Absender des Servers oder von jeder Kanaladresse desselben Workspace (nachgewiesen oder nicht) werden verworfen; Adressen anderer Workspaces verwerfen nie eine E-Mail. |
| DMARC | Nur mit `YUVA_INGRESS_AUTHSERV_ID` genutzt, aus dem obersten `Authentication-Results`, wenn es diese Authserv-ID trägt. Tragen mehrere Header sie, hat der Absender einen hinzugefügt: ein `fail` darunter zählt, ein `pass` nicht. Der Header wird gelesen, wie RFC 8601 es sagt: Nur das Ergebnis, das jede Methode nennt, zählt, nie Text in einer Adresse oder Begründung; ein Header, der sich nicht parsen lässt, ist `unknown`. Ein `pass` zählt nur bei einer Nachricht mit einer einzigen From-Adresse. Eine neue Unterhaltung, die durchfällt, wird als Spam markiert; eine Antwort, die durchfällt, beginnt eine neue. E-Mails, die beim [Umzug zu Yuva](migrate.md) aus Ihrem alten Postfach gelesen werden, sind immer `unknown`. |
| Automatische Antworten | Eine Begrüßung, das `send_reply` einer Regel und eine Bewertungsanfrage gehen nur an einen E-Mail-Absender, dessen E-Mail DMARC bestanden hat. Ohne `YUVA_INGRESS_AUTHSERV_ID` wird keine gesendet. Eine zurückgehaltene Regelantwort lässt den Lauf mit `auto_reply` fehlschlagen. Chat- und API-Unterhaltungen sind nicht betroffen. |

### Grenzen

| Grenze | Wert |
|---|---|
| Nachrichtengröße | 25 MiB |
| Anhänge | Die [Datei-Grenzen](configuration.md#dateien-und-speicher) des Kanals; ein Teil darüber wird übersprungen, die Nachricht bleibt erhalten und das Teil bleibt in der Roh-E-Mail |
| Neue Unterhaltungen pro Absender und Kanal | 20 pro Stunde; weitere E-Mails landen in seiner neuesten Unterhaltung |
| Eingehende E-Mails pro Absender und Workspace | `YUVA_EMAIL_SENDER_HOURLY_CAP`, 500 pro Stunde; weitere werden abgelehnt. E-Mails mit bestandenem DMARC und andere werden getrennt gezählt. |
| Gleichzeitig verarbeitete Nachrichten | `YUVA_INGRESS_MAX_CONCURRENT`, 8; weitere erhalten `503` |
| Zeit zum Senden des Nachrichteninhalts | 30 Sekunden; langsamere Uploads erhalten `503` |
| Wartezeit des Workers auf Yuva | 20 Sekunden |

### Exit-Codes von `ingest-email`

| Code | Bedeutung |
|---|---|
| `0` | Angenommen (`stored`, `duplicate`, `bounce` oder `dropped`) |
| `64` | Falsche Argumente |
| `65` | Fehlerhafte oder zu große Nachricht |
| `67` | Unbekannter Empfänger |
| `75` | Vorübergehender Fehler; erneut versuchen |
| `77` | Abgelehnt, z. B. gesperrter Absender |
