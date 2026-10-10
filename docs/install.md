# Install

Run Yuva on one host with Docker Compose, behind a reverse proxy that serves HTTPS. You need Docker,
a host name with TLS and an SMTP account for sign-in codes.

> [!WARNING]
> Yuva is pre-alpha. Read the warning in the [README](../README.md) before you put real
> conversations in it.

## Quick start with Compose

### 1. Write `compose.yaml`

Make a directory on the host, for example `/opt/yuva`, and put this file in it:

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

### 2. Write `.env`

Put it next to `compose.yaml` and run `chmod 600 .env`. Replace each `<openssl rand …>` with what
that command prints:

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

Keep a copy of `YUVA_MASTER_KEY` outside the server ([Master key](#master-key)).

### 3. Start

```sh
docker run --rm ghcr.io/productdevbook/yuva:0.0.6 vapid-keys >> .env
docker compose up -d
curl -s http://127.0.0.1:8080/readyz     # {"status":"ok"}
```

The first line adds the Web Push keys. The server runs database migrations before it starts.

### 4. Create the first owner

There is no open sign-up:

```sh
docker compose exec yuva /yuva bootstrap --email you@example.com --workspace "Example" --name "Your Name"
```

### 5. Put it behind HTTPS

Point your reverse proxy at `127.0.0.1:8080`. With Caddy:

```
support.example.com {
	reverse_proxy 127.0.0.1:8080 {
		header_up X-Real-IP {remote_host}
	}
}
```

Open `YUVA_PUBLIC_URL`, enter your address and sign in with the code from the e-mail. Without SMTP
the code is in `docker compose logs yuva`. Then invite your team, create an inbox, and set up
[e-mail](email.md), the [web widget](widget.md) or the [mobile SDKs](mobile.md).

## Reference

### Requirements

| What | Notes |
|---|---|
| Postgres 16 or newer | Holds everything. No Redis. |
| SMTP account | For sign-in codes and notifications. Without it, mail goes to the log. |
| Attachment storage | A Docker volume, or S3 with `YUVA_STORAGE=s3` and the `YUVA_S3_*` [settings](configuration.md#attachments-and-storage). |
| Host name with TLS | Passkeys and Web Push need HTTPS. |

### Reverse proxy and TLS

Yuva speaks plain HTTP on port 8080 and serves everything from the root of one host name. Do not
expose the metrics port 9090. The proxy must:

- pass WebSocket upgrades on `/v1/realtime` and `/client/v1/realtime`;
- keep idle connections open longer than 60 seconds;
- accept request bodies of at least 26 MiB;
- overwrite the header named in `YUVA_CLIENT_IP_HEADER` with the client address;
- keep the `Host` and `Origin` headers.

Caddy does all of this with the configuration above. Behind Cloudflare's proxy, use
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

### Public URL

`YUVA_PUBLIC_URL` is the address of the panel, the widget, the API and links in e-mails. Pick it
before your team adds passkeys, which are bound to its host name
([Configuration](configuration.md#sign-in-and-passkeys)).

### Master key

`YUVA_MASTER_KEY` encrypts the secrets Yuva stores: SMTP passwords of e-mail channels, inbox
identity secrets and webhook signing secrets. Generate it once with `openssl rand -base64 32` and
back it up apart from the database, for example in a password manager.

Without the key, a restored database loses those secrets: e-mail stops, apps cannot sign users in
and webhooks are unsigned until each one is entered again. Changing the key does the same; there
is no key rotation yet.

### VAPID keys

Push notifications for members need the key pair from `yuva vapid-keys` in `.env`. Without it,
members get e-mail notifications only. Keep the pair: if it changes, members must turn
notifications on again.

### Docker image

`ghcr.io/productdevbook/yuva:<version>` runs on `linux/amd64` and `linux/arm64` as a non-root user,
with ports `8080` (HTTP) and `9090` (metrics). Its entrypoint is the `yuva` binary, which also runs
the [operator commands](operations.md#operator-commands). To build it, check out the release tag
and run:

```sh
docker build -f deploy/Dockerfile --build-arg VERSION=0.0.6 -t ghcr.io/productdevbook/yuva:0.0.6 .
```

`deploy/compose.yaml` in the repository is for development only. Do not run it in production.
