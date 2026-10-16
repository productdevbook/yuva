# Install

Run Yuva on one host with Docker Compose, or on Kubernetes with the [Helm chart](#kubernetes-with-helm),
behind a reverse proxy that serves HTTPS. You need Docker (or a cluster), a host name with TLS and an
SMTP account for sign-in codes.

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
    image: ghcr.io/productdevbook/yuva:0.0.17
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
YUVA_TRUSTED_PROXIES=172.16.0.0/12

YUVA_SMTP_HOST=smtp.example.com
YUVA_SMTP_USERNAME=yuva@example.com
YUVA_SMTP_PASSWORD=<password>
YUVA_SMTP_FROM=Yuva <yuva@example.com>
```

Keep a copy of `YUVA_MASTER_KEY` outside the server ([Master key](#master-key)).

### 3. Start

```sh
docker run --rm ghcr.io/productdevbook/yuva:0.0.17 vapid-keys >> .env
docker compose up -d
curl -s http://127.0.0.1:8080/readyz     # {"status":"ok"}
```

The first line adds the Web Push keys. The server runs database migrations before it starts.

### 4. Create the first owner

Sign-up is invite only unless you set `YUVA_SIGNUP=open` ([configuration](configuration.md)); create the first owner:

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
the code is in `docker compose logs yuva`.

### 6. Set up what you need

The first owner lands on **Home** (`/`), which shows what is not set up yet. Open **Set up Yuva**
(`/setup`) from that list. It has six cards; pick the ones you want, each takes a few minutes, and
you can come back at any time:

| Card | What it does | Guide |
|---|---|---|
| Website chat | Chat on your website | [Web widget](widget.md) |
| E-mail | A support address that becomes conversations | [E-mail](email.md) |
| Mobile app | Chat inside your app | [Mobile SDKs](mobile.md) |
| Documentation site | Ratings, feedback and discussions on your docs | [Documentation pages](documentation-pages.md) |
| Analytics | Visitors of a website or app, without cookies | [Analytics](analytics.md) |
| Newsletter | A mailing list with subscribe forms | [Lists](lists.md) |

A card that is set up shows how many you have and opens the place where you change it. Yuva does not
open the setup by itself; **Do this later** returns you to where you were. Then invite your team.

Owners and admins open Home at `/` (shortcut `g h`): the inbox today, each website and app with its
visitors today, in the last 30 minutes and over seven days, the last broadcast and the setup list.
Agents open the inbox at `/`; the inbox is also at `/conversations`. A shortcut to the analytics setup,
`/analytics/new`, still works.

## Kubernetes with Helm

Yuva has a Helm chart, published with every release as `oci://ghcr.io/productdevbook/charts/yuva` under the server's version. It brings no database: use a managed Postgres 16 or newer, or one run by an operator such as [CloudNativePG](https://cloudnative-pg.io).

### 1. Create the Secret

Put the database URL and the master key ([Master key](#master-key)) in a Secret, with the ingress secret and SMTP login. S3 keys and the Web Push keys from `yuva vapid-keys` go in the same Secret under `YUVA_S3_ACCESS_KEY_ID`, `YUVA_S3_SECRET_ACCESS_KEY`, `YUVA_VAPID_PUBLIC_KEY` and `YUVA_VAPID_PRIVATE_KEY`:

```sh
kubectl create namespace yuva
kubectl -n yuva create secret generic yuva \
  --from-literal=YUVA_DATABASE_URL='postgres://yuva:<password>@<host>:5432/yuva?sslmode=require' \
  --from-literal=YUVA_MASTER_KEY="$(openssl rand -base64 32)" \
  --from-literal=YUVA_INGRESS_SECRET="$(openssl rand -hex 32)" \
  --from-literal=YUVA_SMTP_USERNAME='yuva@example.com' \
  --from-literal=YUVA_SMTP_PASSWORD='<password>'
```

### 2. Write `values.yaml`

Set `publicUrl`, point `secrets.existingSecret` at the Secret, and put other [settings](configuration.md) under `env`. The ingress class and annotations below are for ingress-nginx; whatever you use must accept request bodies of at least 26 MiB and pass WebSockets ([Reverse proxy and TLS](#reverse-proxy-and-tls)):

```yaml
publicUrl: https://support.example.com
secrets:
  existingSecret: yuva
env:
  YUVA_CLIENT_IP_HEADER: X-Real-IP
  YUVA_TRUSTED_PROXIES: 10.0.0.0/8
  YUVA_SMTP_HOST: smtp.example.com
  YUVA_SMTP_FROM: Yuva <yuva@example.com>
ingress:
  enabled: true
  className: nginx
  annotations:
    nginx.ingress.kubernetes.io/proxy-body-size: 30m
    nginx.ingress.kubernetes.io/proxy-read-timeout: "120"
    nginx.ingress.kubernetes.io/proxy-send-timeout: "120"
  tls:
    - secretName: yuva-tls
      hosts: [support.example.com]
```

### 3. Install

```sh
helm install yuva oci://ghcr.io/productdevbook/charts/yuva --version 0.0.17 -n yuva -f values.yaml
```

### 4. Create the first owner

```sh
kubectl -n yuva exec deploy/yuva -c yuva -- \
  /yuva bootstrap --email you@example.com --workspace "Example" --name "Your Name"
```

Migrations run in an init container before each pod starts. Attachments are kept on a volume by default, which allows one replica only; for several replicas use S3 (`storage.driver: s3` and the `YUVA_S3_*` [settings](configuration.md#attachments-and-storage)). If your load balancer sets sticky-session cookies, set `service.analytics.separateService: true` so the analytics script and endpoint never get one. All values are in the chart's [README](https://github.com/productdevbook/yuva/blob/main/deploy/helm/yuva/README.md).

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

List your proxy's addresses in `YUVA_TRUSTED_PROXIES` as well. Without it the header is read from
whoever sends it, so anyone who reaches the server directly picks their own rate-limit identity.

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
docker build -f deploy/Dockerfile --build-arg VERSION=0.0.17 -t ghcr.io/productdevbook/yuva:0.0.17 .
```

`deploy/compose.yaml` in the repository is for development only. Do not run it in production.
