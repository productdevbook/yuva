# Installation

Yuva läuft auf einem Host mit Docker Compose, hinter einem Reverse Proxy, der HTTPS ausliefert. Sie
brauchen Docker, einen Hostnamen mit TLS und ein SMTP-Konto für Anmeldecodes.

> [!WARNING]
> Yuva ist Pre-Alpha. Lesen Sie den Hinweis in der [README](../../README.md), bevor Sie echte
> Unterhaltungen darin führen.

## Schnellstart mit Compose

### 1. `compose.yaml` schreiben

Legen Sie auf dem Host ein Verzeichnis an, zum Beispiel `/opt/yuva`, und speichern Sie darin diese
Datei:

```yaml
name: yuva

services:
  db:
    image: postgres:17
    environment:
      POSTGRES_USER: yuva
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:?set POSTGRES_PASSWORD in .env}
      POSTGRES_DB: yuva
    volumes:
      - db:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U yuva -d yuva"]
      interval: 5s
      timeout: 3s
      retries: 30
    restart: unless-stopped

  yuva:
    image: ghcr.io/productdevbook/yuva:0.0.6
    env_file: .env
    environment:
      YUVA_DATABASE_URL: postgres://yuva:${POSTGRES_PASSWORD}@db:5432/yuva?sslmode=disable
      YUVA_STORAGE: local
      YUVA_STORAGE_DIR: /data/attachments
    ports:
      - "127.0.0.1:8080:8080"
    volumes:
      - attachments:/data
    depends_on:
      db:
        condition: service_healthy
    restart: unless-stopped

volumes:
  db:
  attachments:
```

### 2. `.env` schreiben

Legen Sie die Datei neben `compose.yaml` und führen Sie `chmod 600 .env` aus. Ersetzen Sie jedes
`<openssl rand …>` durch die Ausgabe dieses Befehls:

```sh
POSTGRES_PASSWORD=<openssl rand -hex 24>
YUVA_PUBLIC_URL=https://support.example.com
YUVA_MASTER_KEY=<openssl rand -base64 32>
YUVA_INGRESS_SECRET=<openssl rand -hex 32>
YUVA_CLIENT_IP_HEADER=X-Real-IP

YUVA_SMTP_HOST=smtp.example.com
YUVA_SMTP_USERNAME=yuva@example.com
YUVA_SMTP_PASSWORD=<password>
YUVA_SMTP_FROM=Yuva <yuva@example.com>
```

Bewahren Sie eine Kopie von `YUVA_MASTER_KEY` außerhalb des Servers auf ([Master-Key](#master-key)).

### 3. Starten

```sh
docker run --rm ghcr.io/productdevbook/yuva:0.0.6 vapid-keys >> .env
docker compose up -d
curl -s http://127.0.0.1:8080/readyz     # {"status":"ok"}
```

Die erste Zeile fügt die Web-Push-Schlüssel hinzu. Der Server führt vor dem Start die
Datenbankmigrationen aus.

### 4. Den ersten Inhaber anlegen

Eine offene Registrierung gibt es nicht:

```sh
docker compose exec yuva /yuva bootstrap --email you@example.com --workspace "Example" --name "Your Name"
```

### 5. Hinter HTTPS stellen

Richten Sie Ihren Reverse Proxy auf `127.0.0.1:8080`. Mit Caddy:

```
support.example.com {
	reverse_proxy 127.0.0.1:8080 {
		header_up X-Real-IP {remote_host}
	}
}
```

Öffnen Sie `YUVA_PUBLIC_URL`, geben Sie Ihre Adresse ein und melden Sie sich mit dem Code aus der
E-Mail an. Ohne SMTP steht der Code in `docker compose logs yuva`. Laden Sie dann Ihr Team ein,
legen Sie einen Posteingang an und richten Sie [E-Mail](email.md), das [Web-Widget](../widget.md)
oder die [Mobile-SDKs](../mobile.md) ein.

## Referenz

### Voraussetzungen

| Was | Hinweise |
|---|---|
| Postgres 16 oder neuer | Enthält alles. Kein Redis. |
| SMTP-Konto | Für Anmeldecodes und Benachrichtigungen. Ohne Konto landen E-Mails im Log. |
| Speicher für Anhänge | Ein Docker-Volume oder S3 mit `YUVA_STORAGE=s3` und den `YUVA_S3_*`-[Einstellungen](configuration.md#dateien-und-speicher). |
| Hostname mit TLS | Passkeys und Web Push brauchen HTTPS. |

### Reverse Proxy und TLS

Yuva spricht unverschlüsseltes HTTP auf Port 8080 und liefert alles vom Wurzelpfad eines Hostnamens
aus. Machen Sie den Metrik-Port 9090 nicht öffentlich. Der Proxy muss:

- WebSocket-Upgrades auf `/v1/realtime` und `/client/v1/realtime` durchreichen;
- inaktive Verbindungen länger als 60 Sekunden offen halten;
- Request-Bodys von mindestens 26 MiB annehmen;
- den in `YUVA_CLIENT_IP_HEADER` genannten Header mit der Client-Adresse überschreiben;
- die Header `Host` und `Origin` unverändert lassen.

Caddy erledigt das alles mit der Konfiguration oben. Hinter dem Proxy von Cloudflare setzen Sie
`YUVA_CLIENT_IP_HEADER=CF-Connecting-IP`. nginx:

```nginx
server {
    listen 443 ssl;
    http2 on;
    server_name support.example.com;
    ssl_certificate     /etc/ssl/support.example.com/fullchain.pem;
    ssl_certificate_key /etc/ssl/support.example.com/privkey.pem;

    client_max_body_size 30m;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
        proxy_read_timeout 120s;
        proxy_buffering off;
    }
}

map $http_upgrade $connection_upgrade {
    default upgrade;
    ''      close;
}
```

### Öffentliche URL

`YUVA_PUBLIC_URL` ist die Adresse des Panels, des Widgets, der API und der Links in E-Mails. Legen
Sie sie fest, bevor Ihr Team Passkeys anlegt, denn diese sind an ihren Hostnamen gebunden
([Konfiguration](configuration.md#anmeldung-und-passkeys)).

### Master-Key

`YUVA_MASTER_KEY` verschlüsselt die Geheimnisse, die Yuva speichert: SMTP-Passwörter von
E-Mail-Kanälen, Identitäts-Secrets von Posteingängen und Signatur-Secrets von Webhooks. Erzeugen Sie
ihn einmal mit `openssl rand -base64 32` und sichern Sie ihn getrennt von der Datenbank, zum
Beispiel in einem Passwortmanager.

Ohne den Schlüssel verliert eine wiederhergestellte Datenbank diese Geheimnisse: E-Mail funktioniert
nicht mehr, Apps können keine Nutzer anmelden und Webhooks bleiben unsigniert, bis jedes Geheimnis
neu eingetragen ist. Ein geänderter Schlüssel hat dieselbe Wirkung; eine Schlüsselrotation gibt es
noch nicht.

### VAPID-Schlüssel

Push-Benachrichtigungen für Mitglieder brauchen das Schlüsselpaar aus `yuva vapid-keys` in `.env`.
Ohne das Paar bekommen Mitglieder nur Benachrichtigungen per E-Mail. Behalten Sie das Paar: Ändert
es sich, müssen Mitglieder die Benachrichtigungen neu einschalten.

### Docker-Image

`ghcr.io/productdevbook/yuva:<version>` läuft auf `linux/amd64` und `linux/arm64` als
Nicht-Root-Benutzer, mit den Ports `8080` (HTTP) und `9090` (Metriken). Sein Entrypoint ist das
Binary `yuva`, das auch die [Operator-Befehle](operations.md#operator-befehle) ausführt. Zum Selbstbauen checken
Sie den Release-Tag aus und führen aus:

```sh
docker build -f deploy/Dockerfile --build-arg VERSION=0.0.6 -t ghcr.io/productdevbook/yuva:0.0.6 .
```

`deploy/compose.yaml` im Repository ist nur für die Entwicklung gedacht. Betreiben Sie es nicht in
Produktion.
