# Konfiguration

Yuva liest seine Einstellungen beim Start aus Umgebungsvariablen. Ein falscher Wert hält den Server
an, mit einer Meldung, die die Variable nennt.

Einstellungen eines Workspaces, Posteingangs oder Kanals liegen im Panel, in der API und in den
[Operator-Befehlen](operations.md#operator-befehle), nicht hier.

## Einen Wert setzen

1. Tragen Sie die Variable in `.env` neben `compose.yaml` ein:

   ```sh
   YUVA_CHAT_EMAIL_DELAY=10m
   ```

2. Starten Sie den Server neu:

   ```sh
   docker compose up -d
   ```

Eine leere Variable gilt als nicht gesetzt und nimmt den Standardwert, außer bei
`YUVA_METRICS_ADDR`. Listen sind kommagetrennt. Boolesche Werte akzeptieren `true`/`false`, `1`/`0`
und `t`/`f`.

## Erforderlich

| Variable | Bedeutung |
|---|---|
| `YUVA_DATABASE_URL` | Postgres-16+-URL, z. B. `postgres://yuva:secret@db:5432/yuva?sslmode=disable`. |
| `YUVA_MASTER_KEY` | 32 Zufallsbytes, Base64 (`openssl rand -base64 32`). Verschlüsselt gespeicherte Geheimnisse. Siehe [Master-Key](install.md#master-key). |
| `YUVA_PUBLIC_URL` | Die URL, die Menschen und Apps verwenden, z. B. `https://support.example.com`, im Wurzelpfad ihres Hosts. Standard `http://localhost:8080`, nur zum lokalen Ausprobieren. |

## Server

| Variable | Standard | Bedeutung |
|---|---|---|
| `YUVA_LISTEN_ADDR` | `:8080` | HTTP-Adresse für alles außer Metriken. |
| `YUVA_METRICS_ADDR` | `:9090` | Prometheus-Adresse (`GET /metrics`). Halten Sie sie privat. `off` oder ein leerer Wert schaltet sie ab. |
| `YUVA_CLIENT_IP_HEADER` | nicht gesetzt | Header, in den Ihr Proxy die Client-IP schreibt, z. B. `X-Real-IP`, für Rate Limits. In einer Liste wie `X-Forwarded-For` zählt die Adresse ganz rechts, die nicht in `YUVA_TRUSTED_PROXIES` steht. |
| `YUVA_TRUSTED_PROXIES` | nicht gesetzt | Adressen oder CIDR-Bereiche Ihrer Proxys, z. B. `10.0.0.0/8`. Dann wird der Client-IP-Header nur von ihnen gelesen. |
| `YUVA_COOKIE_SECURE` | `true`, wenn `YUVA_PUBLIC_URL` `https` ist | Markiert das Sitzungs-Cookie als `Secure`. |
| `YUVA_MCP` | `on` | `off` schaltet den MCP-Endpunkt `/mcp` ab (er antwortet mit `404`). OAuth bleibt an. |
| `YUVA_PANEL` | `on` | `off` liefert das Panel und die OAuth-Zustimmung nicht mehr aus; API-Schlüssel funktionieren weiter. |
| `YUVA_WIDGET` | `on` | `off` liefert `/yuva.js`, `/yuva-chat.js` und `/yuva-docs.js` nicht mehr aus; `/client/v1` bleibt. |

## Anmeldung und Passkeys

| Variable | Standard | Bedeutung |
|---|---|---|
| `YUVA_WEBAUTHN_RP_ID` | Hostname von `YUVA_PUBLIC_URL` | Relying-Party-ID für Passkeys. Eine Änderung macht vorhandene Passkeys unbrauchbar. |
| `YUVA_WEBAUTHN_RP_NAME` | `Yuva` | Name, den Browser für einen neuen Passkey anzeigen. |
| `YUVA_WEBAUTHN_ORIGINS` | Origin von `YUVA_PUBLIC_URL` | Erlaubte Origins für Passkeys und den Realtime-Socket des Panels. |

## Server-E-Mail

Anmeldecodes, Einladungen und Benachrichtigungen. E-Mail-Kanäle haben [ihr eigenes SMTP](email.md#ausgehendes-smtp).

| Variable | Standard | Bedeutung |
|---|---|---|
| `YUVA_SMTP_HOST` | nicht gesetzt | SMTP-Server. Ohne Wert werden E-Mails stattdessen ins Log geschrieben. |
| `YUVA_SMTP_PORT` | `465` mit `tls`, sonst `587` | SMTP-Port. |
| `YUVA_SMTP_TLS` | `starttls` | `starttls`, `tls` (implizites TLS) oder `none`. |
| `YUVA_SMTP_USERNAME` | nicht gesetzt | SMTP-Benutzername. |
| `YUVA_SMTP_PASSWORD` | nicht gesetzt | SMTP-Passwort. |
| `YUVA_SMTP_FROM` | nicht gesetzt | Absender, z. B. `Yuva <yuva@example.com>`. Pflicht zusammen mit `YUVA_SMTP_HOST`. |
| `YUVA_SMTP_ALLOW_PRIVATE` | `false` | Erlaubt E-Mail-Kanälen SMTP-Hosts auf Loopback- und privaten Adressen, etwa Mailpit. |

## Dateien und Speicher

| Variable | Standard | Bedeutung |
|---|---|---|
| `YUVA_STORAGE` | `local` | `local` (ein Verzeichnis) oder `s3` (S3, R2, MinIO, …). |
| `YUVA_STORAGE_DIR` | `data/attachments` | Verzeichnis für `local`; `/data/attachments` im Docker-Image. |
| `YUVA_S3_ENDPOINT` | nicht gesetzt | Endpunkt-URL, z. B. `https://s3.eu-central-1.amazonaws.com`. |
| `YUVA_S3_REGION` | `auto` | Region. |
| `YUVA_S3_BUCKET` | nicht gesetzt | Bucket. |
| `YUVA_S3_ACCESS_KEY_ID` | nicht gesetzt | Access-Key-ID. |
| `YUVA_S3_SECRET_ACCESS_KEY` | nicht gesetzt | Secret Access Key. |
| `YUVA_S3_PATH_STYLE` | `false` | URLs im Path-Style, die MinIO meist braucht. |
| `YUVA_ATTACHMENT_MAX_BYTES` | `26214400` (25 MiB) | Größter Anhang in Bytes. |
| `YUVA_ATTACHMENT_TYPES` | siehe unten | Erlaubte Content-Types; Platzhalter wie `image/*` funktionieren. |

Standardtypen: `image/png,image/jpeg,image/gif,image/webp,image/heic,application/pdf,text/plain,text/csv,application/zip,application/json,video/mp4,video/quicktime,audio/mpeg,audio/mp4`.

## Eingehende E-Mail

| Variable | Standard | Bedeutung |
|---|---|---|
| `YUVA_INGRESS_SECRET` | nicht gesetzt | Signiert Anfragen an `/ingress/email`; der Email Worker verwendet denselben Wert. Ohne Wert lehnt dieser Endpunkt jede E-Mail ab. |
| `YUVA_INGRESS_ACCEPT_V1` | `false` | Akzeptiert zusätzlich die alte `v1`-Signatur, bis Sie einen älteren Email Worker neu deployen. |
| `YUVA_INGRESS_AUTHSERV_ID` | nicht gesetzt | Authserv-ID Ihres empfangenden Mailservers, z. B. `mx.cloudflare.net`. Nötig, damit DMARC-Ergebnisse genutzt werden. |
| `YUVA_INGRESS_MAX_CONCURRENT` | `8` | Nachrichten, die `/ingress/email` pro Prozess gleichzeitig verarbeitet; weitere erhalten `503`. |
| `YUVA_SES_TOPIC_ARNS` | nicht gesetzt | SNS-Topic-ARNs, deren SES-Bounces und -Beschwerden `/ingress/ses` annimmt. |
| `YUVA_EMAIL_SENDER_HOURLY_CAP` | `500` | Eingehende E-Mails, die ein Absender pro Stunde an einen Workspace senden darf; weitere werden abgelehnt (`429`). |

## Chat

| Variable | Standard | Bedeutung |
|---|---|---|
| `YUVA_CHAT_EMAIL_DELAY` | `5m` | Wie lange eine Antwort ungelesen bleibt, bevor sie einem Kontakt, der den Chat verlassen hat, per E-Mail geschickt wird. Mindestens `1s`. |
| `YUVA_ANONYMOUS_CONTACTS_PER_HOUR` | `20` | Neue anonyme Besucher pro IP-Adresse (IPv6 /64), Kanal und Stunde; weitere erhalten `429`. |

## Webhooks

| Variable | Standard | Bedeutung |
|---|---|---|
| `YUVA_WEBHOOK_ALLOW_PRIVATE` | `false` | Erlaubt Webhooks, Loopback- und private Adressen zu erreichen. Nur für die Entwicklung. |

## Web Push

| Variable | Standard | Bedeutung |
|---|---|---|
| `YUVA_VAPID_PUBLIC_KEY` | nicht gesetzt | Öffentlicher VAPID-Schlüssel. Setzen Sie beide Schlüssel oder keinen; `yuva vapid-keys` gibt ein Paar aus. |
| `YUVA_VAPID_PRIVATE_KEY` | nicht gesetzt | Privater VAPID-Schlüssel. |
| `YUVA_VAPID_SUBJECT` | `mailto:` + Adresse aus `YUVA_SMTP_FROM`, sonst `YUVA_PUBLIC_URL`, wenn `https` | Kontakt für Push-Dienste: `mailto:…` oder `https://…`. |

## Build

Die Version, die der Server meldet (`GET /v1/version`), wird beim Build mit dem Docker-Build-Argument
`VERSION` festgelegt. Sie ist keine Umgebungsvariable.
