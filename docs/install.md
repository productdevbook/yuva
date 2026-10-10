# Install

Run Yuva on one host with Docker Compose, behind a TLS reverse proxy.

> [!WARNING]
> Yuva is pre-alpha. Read the warning in the [README](../README.md) before you put real
> conversations in it.

## In short

- You need a host with Docker, a host name with TLS and an SMTP account.
- Write two files, start them, and create the first owner.
- Everything else, from the proxy to the master key, is in [Details](#details).

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

Next to it, readable only by root (`chmod 600 .env`). Replace each `<openssl rand …>` with what
the command prints:

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

Keep `YUVA_MASTER_KEY` somewhere safe outside the server; see [Master key](#master-key).

### 3. Start

```sh
docker run --rm ghcr.io/productdevbook/yuva:0.0.6 vapid-keys >> .env
docker compose up -d
curl -s http://127.0.0.1:8080/readyz     # {"status":"ok"}
```

The first line adds the Web Push keys ([VAPID keys](#vapid-keys)). The server applies database
migrations before it starts serving.

### 4. Create the first owner

There is no open sign-up:

```sh
docker compose exec yuva /yuva bootstrap --email you@example.com --workspace "Example" --name "Your Name"
```

Open `YUVA_PUBLIC_URL`, enter the address and sign in with the code that arrives by e-mail
(without SMTP, find it in `docker compose logs yuva`). Then invite your team from the panel.

### 5. Put it behind HTTPS

Point your reverse proxy at `127.0.0.1:8080`. With Caddy:

```
support.example.com {
	reverse_proxy 127.0.0.1:8080 {
		header_up X-Real-IP {remote_host}
	}
}
```

For nginx and what any proxy must do, see [Reverse proxy and TLS](#reverse-proxy-and-tls).

### Next

Create an inbox and its channels in the panel, then set up [e-mail](email.md), the
[web widget](widget.md) or the [mobile SDKs](mobile.md).

## Details

### Requirements

- **Postgres 16 or newer.** Yuva keeps everything in it, its job queue included. No Redis.
- **An SMTP account** for the server's own mail: sign-in codes, invitations and notification
  e-mails. Without one, mails are written to the log, which is enough to try Yuva but not to run
  it. E-mail channels use their own SMTP accounts (see [E-mail](email.md)).
- **Storage for attachments**: a directory on the host (a Docker volume), or any S3-compatible
  bucket (S3, R2, MinIO, …). For S3, replace the two `YUVA_STORAGE*` lines with `YUVA_STORAGE=s3`
  and the `YUVA_S3_*` settings in [Configuration](configuration.md).
- **A host name with TLS**, e.g. `support.example.com`, behind a reverse proxy that passes
  WebSockets. Passkeys and Web Push need HTTPS.
- Docker with Compose, or any other way to run a container image.

### Docker image

Each release publishes `ghcr.io/productdevbook/yuva:<version>` for `linux/amd64` and
`linux/arm64`, built from [`deploy/Dockerfile`](../deploy/Dockerfile). It contains the server, the
panel and the widget scripts, runs as a non-root user, and exposes `8080` (HTTP) and `9090`
(metrics). Its entrypoint is the `yuva` binary, so every
[command](operations.md#operator-commands) runs in the same image.

To build it yourself: check out the release tag and run
`docker build -f deploy/Dockerfile --build-arg VERSION=0.0.6 -t ghcr.io/productdevbook/yuva:0.0.6 .`

`deploy/compose.yaml` in the repository is the development setup (a fixed master key, Mailpit,
private webhooks allowed). Do not run it in production.

### First owner options

`bootstrap` prints the workspace and member ids. It refuses to run when a workspace exists;
`--allow-existing` creates another one. `--locale tr` sends the owner's e-mails in Turkish. After
signing in, add a passkey from your profile. Inboxes and channels can also be created with the
[operator commands](operations.md#operator-commands).

### Reverse proxy and TLS

Yuva speaks plain HTTP on port 8080; terminate TLS in front of it. Everything is served from the
root of one host name: the panel, `/v1`, `/client/v1`, the WebSockets, `/yuva.js` and
`/yuva-chat.js`, `/ingress/email`, `/ingress/ses`, `/healthz` and `/readyz`. Do not expose port
9090.

The proxy must:

- pass **WebSocket upgrades** on `/v1/realtime` and `/client/v1/realtime`;
- allow idle connections for **more than 60 seconds** (the server pings every 30 seconds);
- accept request bodies of **at least 26 MiB** (25 MiB e-mails and attachments);
- set the header named in `YUVA_CLIENT_IP_HEADER` to the client's address, overwriting any value
  the client sent;
- forward the `Host` header unchanged and keep the `Origin` header.

Caddy does all of this with the configuration above. nginx:

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

Behind Cloudflare's proxy, `YUVA_CLIENT_IP_HEADER=CF-Connecting-IP` works without extra proxy
settings.

### Public URL

`YUVA_PUBLIC_URL` is the address everyone uses: members open the panel there, the widget is loaded
from it, apps and the Email Worker call it, and e-mails link to it. From it Yuva derives the
passkey relying party id (its host name), the allowed origin of the panel, and whether the session
cookie is `Secure`. Choose it before the team adds passkeys: passkeys are bound to the host name,
and moving to another one means adding them again (or setting `YUVA_WEBAUTHN_RP_ID` and
`YUVA_WEBAUTHN_ORIGINS`, see [Configuration](configuration.md#sign-in-and-passkeys)).

### VAPID keys

Members get notifications on their phones and desktops by installing the panel as an app (PWA) and
turning on Web Push. That needs a VAPID key pair:

```sh
docker run --rm ghcr.io/productdevbook/yuva:0.0.6 vapid-keys
# YUVA_VAPID_PUBLIC_KEY=…
# YUVA_VAPID_PRIVATE_KEY=…
```

Put both lines in `.env`. The push services are told how to reach you through
`YUVA_VAPID_SUBJECT`, which defaults to the address in `YUVA_SMTP_FROM`. Without keys Web Push is
off and members get e-mail notifications only. Generate the pair once and keep it: browser
subscriptions are bound to the public key.

### Master key

`YUVA_MASTER_KEY` encrypts the secrets Yuva keeps in the database (AES-256-GCM): SMTP passwords of
e-mail channels, inbox identity secrets and webhook signing secrets. Generate it once:

```sh
openssl rand -base64 32
```

Keep it outside the database and its backups, for example in your password manager or secret
store, and back it up separately. **Without the key, a database backup cannot be used fully**:
stored SMTP passwords, identity secrets and webhook secrets cannot be read, so e-mail stops going
out, apps cannot sign users in and webhooks cannot be signed until every one of them is entered
again. Anyone who has both the key and a database dump can read those secrets. Changing the key
has the same effect as losing it; there is no key rotation yet.
