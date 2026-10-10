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

### Beliebiger MTA: `yuva ingest-email`

Ein MTA, der an einen Befehl zustellt, leitet die Rohnachricht hinein:

```sh
yuva ingest-email --to <envelope recipient> [--from <envelope sender>] < message.eml
```

Der Befehl braucht `YUVA_DATABASE_URL`, `YUVA_MASTER_KEY` und die Speichereinstellungen des
Servers. Auf einem separaten Mail-Host verwenden Sie S3 als Speicher, damit beide an denselben Ort
schreiben. Kopieren Sie das Binary aus dem Image:

```sh
docker create --name yuva-bin ghcr.io/productdevbook/yuva:0.0.6
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
sonst wird DMARC nicht genutzt.

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
| Automatische E-Mails | Abwesenheitsnotizen, Massen-E-Mails und Zustellberichte werden gespeichert, aber nie beantwortet. E-Mails von Yuvas eigenen Adressen werden verworfen. |
| DMARC | Nur mit `YUVA_INGRESS_AUTHSERV_ID` genutzt. Eine neue Unterhaltung, die durchfällt, wird als Spam markiert; eine Antwort, die durchfällt, beginnt eine neue. |

### Grenzen

| Grenze | Wert |
|---|---|
| Nachrichtengröße | 25 MiB |
| Neue Unterhaltungen pro Absender und Kanal | 20 pro Stunde; weitere E-Mails landen in seiner neuesten Unterhaltung |
| Eingehende E-Mails pro Absender und Workspace | `YUVA_EMAIL_SENDER_HOURLY_CAP`, 500 pro Stunde; weitere werden abgelehnt |
| Gleichzeitig verarbeitete Nachrichten | `YUVA_INGRESS_MAX_CONCURRENT`, 8; weitere erhalten `503` |
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
