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
| `YUVA_TRUSTED_PROXIES` | nicht gesetzt | Adressen oder CIDR-Bereiche Ihrer Proxys, z. B. `10.0.0.0/8`. Dann wird der Client-IP-Header nur von ihnen gelesen. Der Analytics-Collector liest den `X-Forwarded-For`-Eintrag des Besuchers nur von diesen Adressen und nimmt den rechtesten Eintrag, der keine davon ist; nicht gesetzt, nutzt er die verbindende Adresse. |
| `YUVA_COOKIE_SECURE` | `true`, wenn `YUVA_PUBLIC_URL` `https` ist | Markiert das Sitzungs-Cookie als `Secure`. |
| `YUVA_LOCALE_COOKIE_DOMAIN` | nicht gesetzt | Eine Domain, in der der Host von `YUVA_PUBLIC_URL` liegt, z. B. `example.com` für `https://app.example.com` (ein führender Punkt wird ignoriert). Das Panel schreibt dann ein `yuva_locale`-Cookie auf diese Domain, damit sich eine Site auf derselben Domain in der Sprache der Person öffnen kann. Ohne solche Site leer lassen. Liegt der Host nicht in der Domain, startet der Server nicht. |
| `YUVA_MCP` | `on` | `off` schaltet den MCP-Endpunkt `/mcp` ab (er antwortet mit `404`). OAuth bleibt an. |
| `YUVA_PANEL` | `on` | `off` liefert das Panel und die OAuth-Zustimmung nicht mehr aus; API-Schlüssel funktionieren weiter. |
| `YUVA_PANEL_ANALYTICS_KEY` | nicht gesetzt | Der öffentliche Schlüssel (`yuva_ak_…`) einer Website in [Analytics](analytics.md) auf diesem Server, deren Domain die des Panels ist. Das Panel zählt dann dort seine eigenen Ansichten, mit `:id` statt Datensatz-IDs (`/conversations/:id`); keine Konversations- oder Kontakt-ID wird gesendet. |
| `YUVA_WIDGET` | `on` | `off` liefert `/yuva.js`, `/yuva-chat.js`, `/yuva-docs.js` und `/yuva-analytics.js` nicht mehr aus; `/client/v1` bleibt. |

## Anmeldung und Passkeys

| Variable | Standard | Bedeutung |
|---|---|---|
| `YUVA_WEBAUTHN_RP_ID` | Hostname von `YUVA_PUBLIC_URL` | Relying-Party-ID für Passkeys. Eine Änderung macht vorhandene Passkeys unbrauchbar. |
| `YUVA_WEBAUTHN_RP_NAME` | `Yuva` | Name, den Browser für einen neuen Passkey anzeigen. |
| `YUVA_WEBAUTHN_ORIGINS` | Origin von `YUVA_PUBLIC_URL` | Erlaubte Origins für Passkeys und den Realtime-Socket des Panels. |
| `YUVA_SIGNUP` | `invite` | `invite`: Nur bestehende Personen und eingeladene Adressen erhalten einen Anmeldecode; Workspaces entstehen mit `yuva bootstrap`. `open`: Jeder kann sich mit einem Code anmelden und im Panel Workspaces anlegen. |
| `YUVA_SIGNUP_WORKSPACES_PER_PERSON` | `3` | Bei `open`: Workspaces, die eine Person angelegt und nicht gelöscht hat; weitere erhalten `403`. |
| `YUVA_SIGNUP_WORKSPACES_PER_IP` | `5` | Bei `open`: Workspaces, die von einer IP-Adresse (IPv6 /64) in 24 Stunden angelegt werden; weitere erhalten `429`. |
| `YUVA_OPERATOR_EMAIL` | nicht gesetzt | Adressen, kommagetrennt, die eine E-Mail erhalten, wenn sich eine neue Person mit einem Code registriert (Leser als solche markiert) oder einen Workspace anlegt: Adresse, Workspace-Name, Zeit und Land (hinter Cloudflare, mit `YUVA_CLIENT_IP_HEADER=CF-Connecting-IP`) oder sonst die IP-Adresse. Höchstens eine E-Mail alle 5 Minuten, mit allem seit der vorigen. Auf Englisch, über das SMTP des Servers. Dieselbe E-Mail listet auch [Freigabeanfragen, Sperren und automatische Pausen](operations.md#broadcasts-auf-einem-gehosteten-server) von Broadcasts. Nicht gesetzt heißt aus. |

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
| `YUVA_EMAIL_IMAGES_MAX_BYTES` | `524288000` (500 MiB) | Gesamtgröße der Bilder der [E-Mail-Designs](designs.md) eines Workspace in Bytes; `0` bedeutet keine Grenze. Weitere Uploads werden abgelehnt (`409 image_quota`). |

Standardtypen: `image/png,image/jpeg,image/gif,image/webp,image/heic,application/pdf,text/plain,text/csv,application/zip,application/json,video/mp4,video/quicktime,audio/mpeg,audio/mp4`.

## Eingehende E-Mail

| Variable | Standard | Bedeutung |
|---|---|---|
| `YUVA_INGRESS_SECRET` | nicht gesetzt | Signiert Anfragen an `/ingress/email`; der Email Worker verwendet denselben Wert. Ohne Wert lehnt dieser Endpunkt jede E-Mail ab. |
| `YUVA_INGRESS_ACCEPT_V1` | `false` | Akzeptiert zusätzlich die alte `v1`-Signatur, bis Sie einen älteren Email Worker neu deployen. |
| `YUVA_INGRESS_ACCEPT_V2` | `true` | Akzeptiert zusätzlich die alte `v2`-Signatur, bei der die Nachricht vor der Prüfung gelesen wird. Setzen Sie `false`, sobald jeder Email Worker mit `v3` signiert; das schließt Advisory GHSA-jfvx-vc82-56f7. |
| `YUVA_INGRESS_AUTHSERV_ID` | nicht gesetzt | Authserv-ID Ihres empfangenden Mailservers, z. B. `mx.cloudflare.net`. Nötig, damit DMARC-Ergebnisse genutzt werden; ohne sie wird jede eingehende Nachricht mit DMARC `unknown` gespeichert und der Server warnt beim Start. |
| `YUVA_INGRESS_MAX_CONCURRENT` | `8` | Nachrichten, die `/ingress/email` pro Prozess gleichzeitig verarbeitet; weitere erhalten `503`. |
| `YUVA_SES_TOPIC_ARNS` | nicht gesetzt | SNS-Topic-ARNs, deren SES-Bounces und -Beschwerden `/ingress/ses` annimmt. |
| `YUVA_SES_REGION` | nicht gesetzt | Die Amazon-SES-Region, deren [Tenants](operations.md#amazon-ses-tenants-auf-einem-gehosteten-server) Yuva verwaltet, z. B. `eu-west-1`. Kanäle, die über SES in einer anderen Region senden, werden nicht isoliert. Zusammen mit den nächsten drei setzen, oder keine. |
| `YUVA_SES_ACCESS_KEY_ID` | nicht gesetzt | Zugriffsschlüssel eines IAM-Benutzers mit der Tenant-Richtlinie. |
| `YUVA_SES_SECRET_ACCESS_KEY` | nicht gesetzt | Sein Secret. |
| `YUVA_SES_CONFIGURATION_SET` | nicht gesetzt | Das Configuration Set, das Yuva jedem Tenant zuordnet und in `X-SES-CONFIGURATION-SET` nennt. 1 bis 64 Buchstaben, Ziffern, `-` oder `_`. |
| `YUVA_SES_TENANT_PREFIX` | `yuva-` | Anfang jedes Tenant-Namens, den Yuva anlegt, bis zu 32 Buchstaben, Ziffern, `-` oder `_`. Die IAM-Richtlinie erlaubt nur Tenants mit diesem Präfix. |
| `YUVA_EMAIL_SENDER_HOURLY_CAP` | `500` | Eingehende E-Mails, die ein Absender pro Stunde an einen Workspace senden darf; weitere werden abgelehnt (`429`). |

## Zu Yuva umziehen

Für das [Umziehen eines Postfachs zu Yuva](migrate.md). Die Einstellungen, die einen Workspace begrenzen,
gelten für gehostete Server; eine Anfrage über einer Grenze wird mit `422 hosted_limit` abgelehnt, das die
Einstellung nennt.

| Variable | Standard | Bedeutung |
|---|---|---|
| `YUVA_FORWARDING_DOMAIN` | nicht gesetzt | Ein Domainname wie `forward.example.com`, dessen E-Mails Sie wie die Domains der Kanäle auf Yuva ausrichten (siehe [Eingehende E-Mails](email.md#eingehende-e-mails)). Dann hat jeder E-Mail-Kanal eine [Weiterleitungsadresse](email.md#weiterleitungsadresse) `<Token>@` dieser Domain. Nicht gesetzt, gibt es keine Weiterleitungsadressen, und der Umzug eines Postfachs stellt E-Mails nur per MX um. |
| `YUVA_MIGRATION_SOURCES` | alle gebauten | Kommagetrennte Quellen, von denen Teams umziehen dürfen: `imap`, `file`, `zendesk`, `helpscout`. Bisher gebaut sind Postfächer über IMAP und Exportdateien. Leer schaltet den Umzug aus. |
| `YUVA_MIGRATION_WORKERS` | `2` | Umzüge, die pro Prozess gleichzeitig kopiert werden; `0` führt in diesem Prozess keine aus. |
| `YUVA_MIGRATION_MAX_ACTIVE` | `0` | Höchstzahl der Umzüge eines Workspace, die gleichzeitig scannen, kopieren oder synchronisieren; `0` heißt unbegrenzt. |
| `YUVA_MIGRATION_MAX_BYTES` | `0` | Höchstzahl Bytes, die ein Workspace pro Monat importieren darf; `0` heißt unbegrenzt. |
| `YUVA_MIGRATION_MAX_MESSAGE_BYTES` | `52428800` (50 MiB) | Größte importierte Nachricht; größere werden übersprungen. |
| `YUVA_MIGRATION_UPLOAD_MAX_BYTES` | `21474836480` (20 GiB) | Größte [Exportdatei](migrate-files.md), die ein Umzug annimmt; eine größere wird mit `422 hosted_limit` abgelehnt. `0` schaltet Dateiimporte aus. |
| `YUVA_MIGRATION_SYNC_MAX_DAYS` | `30` | Tage, die ein Umzug nach dem Kopieren das alte Postfach weiterliest; danach stoppt er und seine Zugangsdaten werden gelöscht. 1 bis 365. |
| `YUVA_MIGRATION_AUDIT_DAYS` | `365` | Tage, die die Aktivität eines Umzugs nach seinem Ende aufbewahrt wird. 1 bis 36500. |
| `YUVA_MICROSOFT_CLIENT_ID`, `YUVA_MICROSOFT_CLIENT_SECRET` | nicht gesetzt | Die eigene Microsoft-Entra-App des Servers für „Sign in with Microsoft“ ([so registrieren Sie sie](migrate-operators.md#die-microsoft-app-registrieren)). Setzen Sie beide oder keine. Ohne sie registriert ein Team im Schritt Connect eine eigene App, und Outlook.com und Microsoft 365 werden nur angeboten, wenn die [`YUVA_PUBLIC_URL`](#server) des Servers eine HTTPS-Adresse (oder `localhost`) ist, da Microsoft nur sie als Redirect-URL akzeptiert. |
| `YUVA_MICROSOFT_TENANT` | `common` | Die Autorität der Microsoft-App des Servers: `common` für eine App, die jedes Arbeits-, Schul- oder persönliche Konto akzeptiert; `organizations`, `consumers`, eine Mandanten-ID oder eine verifizierte Domain für eine App, die darauf beschränkt ist. |
| `YUVA_MIGRATION_ALLOW_PRIVATE` | `false` | Erlaubt einem Umzug auch die Verbindung zu Mailservern in einem privaten oder lokalen Netz (Entwicklung, Server im eigenen Netz). |

## Broadcasts

[Broadcasts versenden](broadcasts.md). Listen und ihre Formulare funktionieren unabhängig davon, was hier steht.

| Variable | Standard | Bedeutung |
|---|---|---|
| `YUVA_BROADCASTS` | `approval` bei `YUVA_SIGNUP=open`, sonst `on` | `on`: jeder Workspace sendet. `approval`: ein über die Anmeldung angelegter Workspace sendet erst nach Freigabe durch den Operator, und jeder Workspace unterliegt einem [Warm-up-Limit](broadcasts.md#freigabe-auf-gehosteten-servern). `off`: Broadcasts anlegen, ändern und senden antwortet mit `403 broadcasts_disabled`. |
| `YUVA_BROADCAST_WORKERS` | `2` | Broadcast-Versandjobs, die ein Prozess gleichzeitig ausführt, je einer für einen sendenden Kanal. Support-Antworten haben eine eigene Queue und werden nicht langsamer. Mindestens 1. |
| `YUVA_BROADCAST_DAILY_RECIPIENTS` | `0` (kein Limit) | Höchstzahl der Empfänger der Broadcasts eines Workspaces pro UTC-Tag. Ein Versand, der es erreicht, geht am nächsten Tag weiter. Der Operator kann pro Workspace ein kleineres Limit setzen ([Betrieb](operations.md#broadcasts-auf-einem-gehosteten-server)). |
| `YUVA_IMPORT_MAX_ROWS` | `100000` | Höchstzahl der Datenzeilen einer CSV-Datei beim [Importieren von Abonnenten](importing.md). Von 1 bis 100000; ein höherer Wert verhindert den Start des Servers. Eine Datei darüber schlägt fehl, und nichts wird eingetragen. |
| `YUVA_MAIL_LINK_URL` | `YUVA_PUBLIC_URL` | Ursprung (Schema und Host, kein Pfad), auf den die Abmelde-, Bestätigungs-, Klick- und Bild-Links in Listen- und Broadcast-Mails zeigen, z. B. `https://links.example.com`. Er muss diesen Server erreichen. Dieser Host beantwortet nur diese Links (`/m/`, `/s/`, `/c/`, `/i/`) und liefert für alles andere `404`; Panel und API bleiben auf Ihrem Haupt-Host. |

## Statusseiten

[Statusseiten](status-pages.md).

| Variable | Standard | Bedeutung |
|---|---|---|
| `YUVA_STATUS_URL` | nicht gesetzt | Ursprung, von dem öffentliche Statusseiten ausgeliefert werden, z. B. `https://status.example.com` (kein Pfad). Er muss diesen Server erreichen und sich von `YUVA_PUBLIC_URL` unterscheiden. Die Seiten liegen dann unter `<Ursprung>/{slug}`, und `<YUVA_PUBLIC_URL>/status/{slug}` leitet dorthin um. Nicht gesetzt, liegen die Seiten unter `<YUVA_PUBLIC_URL>/status/{slug}`. |
| `YUVA_STATUS_HOME` | `on` | `off` schaltet die Startseite an der Wurzel des Ursprungs von `YUVA_STATUS_URL` ab (samt Sprachpfaden wie `/tr/`, `robots.txt` und `sitemap.xml`); die Wurzel antwortet dann mit einer schlichten `404`-Seite. Eigene Domains bleiben unberührt. Braucht `YUVA_STATUS_URL`. |
| `YUVA_STATUS_HOME_PAGE` | nicht gesetzt | Slug einer veröffentlichten Statusseite, die die Startseite als Live-Karte zeigt (Zustand, die ersten fünf Komponenten, 60-Tage-Streifen). Nicht gesetzt oder solange die Seite unveröffentlicht ist, entfällt die Karte. Die Links zu Anmeldung und Erstellen führen zu `YUVA_PUBLIC_URL`. |
| `YUVA_STATUS_MONITORS` | `20` | Höchstzahl der Monitore eines Workspace; `0` heißt unbegrenzt. Darüber antwortet das Anlegen mit `403 status_monitor_limit`. |
| `YUVA_STATUS_MIN_INTERVAL` | `60`, bei `YUVA_SIGNUP=open` `300` | Kürzestes Prüfintervall von http- und tcp-Monitoren in Sekunden (`60` bis `86400`). Mit einem Wert außerhalb dieses Bereichs startet der Server nicht. |
| `YUVA_STATUS_CANARY_URLS` | die zwei eingebauten (`https://www.google.com/generate_204`, `https://www.cloudflare.com/cdn-cgi/trace`) | Kommagetrennte http- oder https-URLs, die der Server abruft, um eigene Netzprobleme von denen seiner Kunden zu unterscheiden. Schlagen alle kurz nach einer Antwort fehl, werden die fehlgeschlagenen Prüfungen dieser Runde verworfen statt als Ausfall gezählt. Geben Sie URLs an, die dieser Server erreicht, oder `off`, um den Schutz abzuschalten. |
| `YUVA_STATUS_NOTICES` | `on` | `off` blendet das Anmeldeformular aus (es antwortet `404`) und sendet keine Benachrichtigungen. |
| `YUVA_STATUS_NOTICES_DAILY` | `5000` bei `YUVA_SIGNUP=open`, sonst `0` | Höchstzahl der Benachrichtigungsmails, die ein Workspace pro UTC-Tag sendet; `0` heißt unbegrenzt. Der Rest wartet auf den nächsten Tag. |
| `YUVA_STATUS_MAX_SUBSCRIBERS` | `10000` bei `YUVA_SIGNUP=open`, sonst `0` | Höchstzahl der angemeldeten Adressen je Seite; `0` heißt unbegrenzt. An der Grenze sagt das Formular, dass die Seite keine Abonnenten mehr annimmt. |
| `YUVA_STATUS_DOMAINS` | `ask` | Eigene Domains von Statusseiten (`status.example.com`). `ask`: Ihr TLS-Proxy stellt Zertifikate bei Bedarf aus und fragt vorher `GET /status-domains/check?domain=` (`200` für eine bestätigte Domain, sonst `404`); ein Caddy-Beispiel liegt in `deploy/caddy/Caddyfile`. `cloudflare`: Der Server legt jede bestätigte Domain als Custom Hostname von Cloudflare for SaaS an (HTTP-Validierung) und löscht ihn, wenn die Domain entfernt oder abgeschaltet wird; braucht `YUVA_CLOUDFLARE_ZONE_ID` und `YUVA_CLOUDFLARE_API_TOKEN`. `off`: keine eigenen Domains. |
| `YUVA_STATUS_CNAME` | Host von `YUVA_PUBLIC_URL` | Der Host, auf den der CNAME eigener Domains zeigt. Im Modus `cloudflare` ein Host der Zone, der zu ihrem Fallback-Origin führt. |
| `YUVA_STATUS_DOMAIN_LIMIT` | `1` bei `YUVA_SIGNUP=open`, sonst `0` | Höchstzahl der Statusseiten eines Workspace mit eigener (nicht abgeschalteter) Domain; `0` heißt unbegrenzt. Darüber antwortet das Setzen mit `403 status_domain_limit`. |
| `YUVA_CLOUDFLARE_ZONE_ID` | nicht gesetzt | Die Cloudflare-Zone, deren Custom Hostnames die eigenen Domains tragen, für `YUVA_STATUS_DOMAINS=cloudflare`. |
| `YUVA_CLOUDFLARE_API_TOKEN` | nicht gesetzt | Ein Cloudflare-API-Token mit der Berechtigung *SSL and Certificates: Edit* für die Zone, für `YUVA_STATUS_DOMAINS=cloudflare`. |

## Hilfecenter

[Hilfecenter](help-center.md).

| Variable | Standard | Bedeutung |
|---|---|---|
| `YUVA_HELP` | `on` | `off` entfernt Hilfecenter: ihre Endpunkte und öffentlichen Seiten antworten mit `404`, und das Widget zeigt keine Hilfe-App. |
| `YUVA_HELP_URL` | nicht gesetzt | Ursprung, von dem öffentliche Hilfecenter ausgeliefert werden, z. B. `https://help.example.com` (Schema und Host, kein Pfad). Er muss diesen Server erreichen und sich von `YUVA_PUBLIC_URL` unterscheiden. Die Center liegen dann unter `<Ursprung>/{slug}`. Nicht gesetzt, liegen sie unter `<YUVA_PUBLIC_URL>/help/{slug}`. |
| `YUVA_HELP_MAX_ARTICLES` | `500` bei `YUVA_SIGNUP=open`, sonst `0` | Höchstzahl Hilfe-Artikel eines Workspace; `0` heißt unbegrenzt. Darüber antwortet das Anlegen eines Artikels mit `422 hosted_limit`. |

## Roadmap-Board

[Roadmap-Boards](roadmap-board.md).

| Variable | Standard | Bedeutung |
|---|---|---|
| `YUVA_ROADMAP` | `on` | `off` entfernt Roadmap-Boards: ihre Endpunkte und öffentlichen Seiten antworten mit `404`, und das Widget zeigt keine Roadmap-App. |
| `YUVA_ROADMAP_MAX_IDEAS` | `2000` bei `YUVA_SIGNUP=open`, sonst `0` | Höchstzahl der Ideen, die ein Workspace haben darf; `0` ist unbegrenzt. Darüber antwortet das Anlegen einer Idee mit `422 hosted_limit`. |

## Neuigkeiten

[Neuigkeiten-Changelogs](whats-new.md).

| Variable | Standard | Bedeutung |
|---|---|---|
| `YUVA_WHATS_NEW` | `on` | `off` entfernt Neuigkeiten: ihre Endpunkte und öffentlichen Seiten antworten mit `404`, und das Widget zeigt keine Neuigkeiten-App. |
| `YUVA_CHANGELOG_MAX_ENTRIES` | `1000` bei `YUVA_SIGNUP=open`, sonst `0` | Höchstzahl an Neuigkeiten-Einträgen je Workspace; `0` heißt unbegrenzt. Darüber antwortet das Anlegen mit `422 hosted_limit`. |

## Analytics

[Ihre Websites zählen](analytics.md).

| Variable | Standard | Bedeutung |
|---|---|---|
| `YUVA_ANALYTICS` | `on` | `off` entfernt Analytics: Seine Endpunkte, der Erfassungs-Endpunkt und `/yuva-analytics.js` antworten mit `404`. |
| `YUVA_ANALYTICS_BUFFER` | `50000` | Events, die ein Prozess hält, bevor er sie schreibt (geschrieben wird jede Sekunde). Ein voller Puffer antwortet mit `503`. Mindestens 100. |
| `YUVA_ANALYTICS_VISIT_CACHE` | `200000` | Besuche, die ein Prozess im Speicher hält, um laufende von neuen Besuchen zu unterscheiden. Mindestens 1000. |
| `YUVA_ANALYTICS_SALT_GRACE` | `2h` | Wie lange der Salt eines Tages nach dessen Ende für verspätete Events weiterlebt. Höchstens `6h`. |
| `YUVA_ANALYTICS_MONTHLY_EVENTS` | `1000000` | Events, die ein Workspace pro UTC-Monat zählen darf; mehr werden verworfen, bis der Monat endet. `0` heißt kein Limit; der Server warnt dann beim Start. [`yuva workspace analytics-limit`](operations.md#operator-befehle) überschreibt es je Workspace. |
| `YUVA_ANALYTICS_PROPERTY_EVENTS_PER_MINUTE` | `60000` | Events, die eine Website pro Minute senden darf; mehr werden verworfen. |
| `YUVA_ANALYTICS_PROPERTIES_PER_WORKSPACE` | `50` | Websites, die ein Workspace haben darf; weitere werden abgelehnt (`403 property_limit`). |
| `YUVA_ANALYTICS_RAW_DAYS` | `90` | Tage, die die detaillierten Besuche und Events aufbewahrt werden, von 7 bis 400. Ältere Tage behalten nur ihre Tagessummen. |
| `YUVA_ANALYTICS_WORKERS` | `2` | Analytics-Jobs (Tagessummen, Aufräumen), die ein Prozess gleichzeitig ausführt; `0` führt in diesem Prozess keine aus. |
| `YUVA_ANALYTICS_ROLLUP_TOP` | `1000` | Werte je Aufschlüsselung, die in den Summen eines Tages behalten werden; der Rest wird als `(other)` zusammengefasst. Von 10 bis 100000. |
| `YUVA_GEOIP` | `dbip` | Woher Orte kommen. `dbip`: die kostenlose Städte-Datenbank DB-IP Lite, die der Server jeden Monat herunterlädt. `file`: eine eigene MMDB-Städte-Datenbank. `off`: keine Standortdatenbank. |
| `YUVA_GEOIP_DIR` | `data/geoip` | Verzeichnis für die heruntergeladene `dbip`-Datenbank. Legen Sie es auf ein Volume, das Neustarts übersteht, etwa `/data/geoip` im Compose-Setup. |
| `YUVA_GEOIP_FILE` | nicht gesetzt | Pfad der MMDB-Datei, Pflicht bei `YUVA_GEOIP=file`. |

Mit `dbip` steht die Datenbank unter [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/), und Ihr
Server muss sie überall nennen, wo Standorte gezeigt werden. Das Panel tut das („IP Geolocation by DB-IP“,
mit Link auf `https://db-ip.com`); zeigen Sie die Zahlen anderswo, sagen Sie es auch dort. Text und Link
stehen in `geoip.attribution` und `geoip.attribution_url` der Antwort von
`GET /v1/analytics/properties/{propertyId}/health`.

Die Befehle laufen wie unter [Operator-Befehle](operations.md#operator-befehle):

| Befehl | Was er tut |
|---|---|
| `analytics property create --workspace <ws> --name <name> --domain <host>... [--inbox <id\|slug>] [--platform web\|ios\|android]... [--timezone <zone>]` | Legt eine Website an und gibt ihre ID und ihren öffentlichen Schlüssel aus, durch einen Tab getrennt. |
| `analytics property list --workspace <ws>` | Listet Websites mit Schlüsseln, Domains und heutigen Besuchern, dazu die Events des Monats gegen das Limit. |
| `analytics geoip-update` | Lädt die DB-IP-Lite-Datenbank dieses Monats nach `YUVA_GEOIP_DIR`, sofern sie nicht schon dort liegt, und gibt die zu nennende Quelle aus. Mit `file` prüft er Ihre Datei. |
| `workspace analytics-limit --workspace <ws> <events\|0>` | Setzt die Events pro Monat des Workspaces; `0` hebt das Limit auf. |

## Chat

| Variable | Standard | Bedeutung |
|---|---|---|
| `YUVA_CHAT_EMAIL_DELAY` | `5m` | Wie lange eine Antwort ungelesen bleibt, bevor sie einem Kontakt, der den Chat verlassen hat, per E-Mail geschickt wird. Mindestens `1s`. |
| `YUVA_DISCUSSION_EMAIL_DELAY` | `10m` | Pro Follower und Diskussionsthread in dieser Zeit höchstens eine E-Mail mit den neuen Antworten und der akzeptierten Antwort. Mindestens `1s`. |
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

## Tracing

Tracing ist aus, solange kein Endpunkt gesetzt ist. Yuva bringt keine eigenen Variablen mit, sondern liest die
Standardvariablen von
[OpenTelemetry](https://opentelemetry.io/docs/specs/otel/configuration/sdk-environment-variables/). Ein Wert,
den der Server nicht verwenden kann, stoppt ihn beim Start mit einer Meldung, die mit `OpenTelemetry:`
beginnt. Siehe [Betrieb](operations.md#tracing).

| Variable | Standard | Bedeutung |
|---|---|---|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | nicht gesetzt | URL des OTLP-Collectors, zum Beispiel `http://otel-collector:4318`. Setzen schaltet Tracing ein. |
| `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | nicht gesetzt | Collector-URL nur für Traces, unverändert verwendet. Setzen schaltet Tracing ebenfalls ein. |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | `http/protobuf` | `http/protobuf` oder `grpc` (meist Port `4317`). `OTEL_EXPORTER_OTLP_TRACES_PROTOCOL` überschreibt es für Traces. |
| `OTEL_EXPORTER_OTLP_HEADERS` | nicht gesetzt | Header, die an den Collector gehen, `schlüssel=wert`-Paare mit Komma getrennt, zum Beispiel ein API-Schlüssel. |
| `OTEL_SERVICE_NAME` | `yuva` | Dienstname auf jedem Span. |
| `OTEL_RESOURCE_ATTRIBUTES` | nicht gesetzt | Zusätzliche Attribute auf jedem Span, zum Beispiel `deployment.environment=prod`. |
| `OTEL_TRACES_SAMPLER` | jede Anfrage wird getraced | Sampler, zum Beispiel `parentbased_traceidratio`. Ohne ihn ignoriert der Server das Sampled-Flag eines eingehenden `traceparent`. |
| `OTEL_TRACES_SAMPLER_ARG` | nicht gesetzt | Argument des Samplers, zum Beispiel `0.1`. |
| `OTEL_SDK_DISABLED` | `false` | `true` hält Tracing aus, auch wenn ein Endpunkt gesetzt ist. `OTEL_TRACES_EXPORTER=none` bewirkt dasselbe. |

Die übrigen `OTEL_EXPORTER_OTLP_*`-Variablen (Timeout, Kompression, Zertifikate) und `OTEL_BSP_*` liest die
OpenTelemetry-Bibliothek, wie die Spezifikation sie definiert.

## Build

Die Version, die der Server meldet (`GET /v1/version`), wird beim Build mit dem Docker-Build-Argument
`VERSION` festgelegt. Sie ist keine Umgebungsvariable.

`GET /v1/version` erfordert keine Anmeldung. Sie verrät nur die Serverversion und ob die Registrierung offen ist (`signup`).
