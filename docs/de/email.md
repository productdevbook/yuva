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

Der Email Worker signiert mit `v3`, was der Server prüft, bevor er die Nachricht liest. Früher
deployte Worker signieren mit `v2`; das wird weiter akzeptiert, solange `YUVA_INGRESS_ACCEPT_V2` an
ist (vorerst der Standard). Bei `v2` wird die Nachricht gelesen, bevor feststeht, dass sie echt ist,
sodass jeder den Server unsignierte Uploads puffern lassen kann (Advisory GHSA-jfvx-vc82-56f7).
Deployen Sie den Worker neu und setzen Sie dann `YUVA_INGRESS_ACCEPT_V2=false`. Ein Proxy vor der
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
docker create --name yuva-bin ghcr.io/productdevbook/yuva:0.0.32
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
sonst wird DMARC nicht genutzt. Ihr MTA muss jeder Nachricht, die er weitergibt, oben ein eigenes
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
E-Mail weitergehen.

```sh
yuva channel create-email --workspace Example --inbox support --name "Everything else" \
  --address '*@example.com' --from-address support@example.com \
  --smtp-host smtp.example.com --smtp-username support@example.com --smtp-password-file -
```

### So werden eingehende E-Mails verarbeitet

| Thema | Verhalten |
|---|---|
| Kanal | Der Kanal des Envelope-Empfängers; `local+tag@domain` fällt auf `local@domain` zurück. Unbekannte Adressen werden abgewiesen. |
| Threading | Über `In-Reply-To` und `References`; alles andere beginnt eine neue Unterhaltung. Eine Antwort öffnet die Unterhaltung wieder. |
| Cc und Beteiligte | Die `To`- und `Cc`-Adressen einer E-Mail, die nicht Yuva gehören, werden Beteiligte der Unterhaltung (höchstens 20). Aktiv sind sie nur, wenn die E-Mail DMARC bestanden hat (dafür ist `YUVA_INGRESS_AUTHSERV_ID` nötig); sonst sind sie vorgeschlagen, für Mitglieder sichtbar, aber nie in Kopie. Mitglieder können Beteiligte hinzufügen und entfernen. Die Antwort eines aktiven Beteiligten, die DMARC besteht, kommt als seine Nachricht in die Unterhaltung; jeder andere, der auf den Thread antwortet, beginnt eine neue Unterhaltung, die auf sie verweist. E-Mails, die DMARC nicht bestehen oder automatisch sind, fügen niemanden hinzu. |
| Antworten | Gehen per `To` an den Kontakt und per `Cc` an die aktiven Beteiligten; automatische E-Mails (Begrüßungen, Regeln) setzen niemanden in Kopie. Jede Antwort kann das `Cc` ersetzen und `Bcc` hinzufügen (je 20 Adressen); `Bcc`-Empfänger erhalten die E-Mail, erscheinen aber nie darin, und nur Mitglieder sehen sie. Eine Adresse mit Bounce oder Beschwerde wird abgelehnt oder als Beteiligte ausgelassen; eine Abmeldung von Broadcasts stoppt keine Antworten. |
| Automatische E-Mails | Abwesenheitsnotizen, Massen-E-Mails und Zustellberichte werden gespeichert, aber nie beantwortet. E-Mails von Yuvas eigenen Adressen werden verworfen. |
| DMARC | Nur mit `YUVA_INGRESS_AUTHSERV_ID` genutzt, aus dem obersten `Authentication-Results`, wenn es diese Authserv-ID trägt. Tragen mehrere Header sie, hat der Absender einen hinzugefügt: ein `fail` darunter zählt, ein `pass` nicht. Eine neue Unterhaltung, die durchfällt, wird als Spam markiert; eine Antwort, die durchfällt, beginnt eine neue. |

### Grenzen

| Grenze | Wert |
|---|---|
| Nachrichtengröße | 25 MiB |
| Anhänge | Die [Datei-Grenzen](configuration.md#dateien-und-speicher) des Kanals; ein Teil darüber wird übersprungen, die Nachricht bleibt erhalten und das Teil bleibt in der Roh-E-Mail |
| Neue Unterhaltungen pro Absender und Kanal | 20 pro Stunde; weitere E-Mails landen in seiner neuesten Unterhaltung |
| Eingehende E-Mails pro Absender und Workspace | `YUVA_EMAIL_SENDER_HOURLY_CAP`, 500 pro Stunde; weitere werden abgelehnt |
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
