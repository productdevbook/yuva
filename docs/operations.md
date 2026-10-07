# Operations

Running Yuva after the [install](install.md): upgrades, backups, monitoring and the commands an
operator runs on the server. The examples use the Compose setup from the install guide, run from
its directory; the Compose project is `yuva`, so the volumes are `yuva_db` and `yuva_attachments`.

## Operator commands

The image's entrypoint is the `yuva` binary. Run commands next to the server with
`docker compose exec yuva /yuva <command>` (add `-T` when you pipe into it). They read the same
environment as the server.

| Command | What it does |
|---|---|
| `serve [--migrate=false]` | Runs the server (the image's default command). Applies migrations first unless `--migrate=false`. |
| `migrate up\|down\|status` | Applies all pending migrations, rolls back the most recent one, or lists them with the time each was applied. |
| `bootstrap --email <address> --workspace <name> [--name <name>] [--locale en\|tr] [--allow-existing]` | Creates a workspace and its first owner. Refuses when a workspace exists, unless `--allow-existing`. |
| `api-key create --workspace <id\|name> --name <name>` | Creates a workspace API key and prints the secret once on stdout. |
| `api-key list --workspace <id\|name>` | Lists keys: id, prefix, name, created, last used, revoked. Never a secret. |
| `api-key revoke <id>` | Revokes a key. |
| `inbox create --workspace <id\|name> --name <name> [--slug <slug>] [--locale <tag>] [--timezone <zone>] [--mode live\|async] [--expected-reply-minutes <n>]` | Creates an inbox and prints its id. The slug is made from the name when omitted; defaults are `en`, `UTC`, `async`. |
| `inbox list --workspace <id\|name>` | Lists inboxes. |
| `channel create-email --workspace <id\|name> --inbox <id\|slug> --name <name> --address <address> [--display-name <name>] [--from-address <address>] [--smtp-host <host> [--smtp-port <n>] [--tls starttls\|tls\|none] [--smtp-username <name>] [--smtp-password-file <path\|->]]` | Creates an e-mail channel and prints its id. The password is read from a file, or from stdin with `-`, never from the command line. |
| `channel list --workspace <id\|name> --inbox <id\|slug>` | Lists an inbox's channels with their SMTP settings; says only whether a password is set. |
| `ingest-email --to <address> [--from <address>]` | Delivers a raw message from stdin, for MTAs (see [E-mail](email.md#any-mta-yuva-ingest-email)). |
| `vapid-keys` | Prints a new Web Push key pair. Needs no database. |

`--workspace` takes the workspace id or its exact name; a name several workspaces share is refused
with their ids. Commands that print an id or a secret write only that to stdout and the rest to
stderr, so they work in scripts:

```sh
KEY=$(docker compose exec -T yuva /yuva api-key create --workspace Example --name provisioning)
INBOX=$(docker compose exec -T yuva /yuva inbox create --workspace Example --name "Example App" --mode live)
printf '%s' "$SMTP_PASSWORD" | docker compose exec -T yuva /yuva channel create-email \
  --workspace Example --inbox example-app --name Support --address support@example.com \
  --smtp-host smtp.example.com --smtp-username support@example.com --smtp-password-file -
```

An inbox created on the command line does not print its identity secret; rotate it in the panel
when an app needs one. Chat and app channels, members, webhooks and everything else are managed in
the panel or through `/v1`.

## Upgrades

1. Read the release notes of every version between yours and the new one.
2. [Back up](#backups) the database and the attachments.
3. Build or pull the new image and change the tag in `compose.yaml`.
4. `docker compose up -d`.

The server applies pending database migrations when it starts, before it serves requests, so the
new version is ready once `/readyz` answers `200`. Migrations only go forward on start; run one
server process while upgrading so an old process does not work against the new schema.

To run migrations as a separate step (for example from a deploy job), start the server with
`serve --migrate=false` and run `yuva migrate up` first. `yuva migrate status` shows what is
applied. `yuva migrate down` rolls back one migration at a time; going back to an older version
is safer by restoring the backup from step 2.

## Backups

A complete backup has three parts:

1. **The database.** Everything except files: workspaces, conversations, messages, contacts,
   settings, the job queue.
2. **The attachments storage.** Attachments and the original of every inbound e-mail.
3. **The master key**, `YUVA_MASTER_KEY`. Kept apart from the other two (see
   [Master key](install.md#master-key)); without it the stored SMTP passwords, identity secrets and
   webhook secrets cannot be read.

### Database with pg_dump

```sh
docker compose exec -T db pg_dump -U yuva -d yuva -Fc > yuva-$(date +%F).dump
```

Run it from cron and copy the files off the host. `pg_dump` takes a consistent snapshot while Yuva
keeps running.

### Database on Kubernetes with CloudNativePG

With a CloudNativePG cluster that has a backup method configured (for example the Barman Cloud
plugin writing to object storage), schedule base backups; WAL archiving then gives point-in-time
recovery:

```yaml
apiVersion: postgresql.cnpg.io/v1
kind: ScheduledBackup
metadata:
  name: yuva-daily
spec:
  schedule: "0 0 3 * * *"
  backupOwnerReference: self
  cluster:
    name: yuva-db
  method: plugin
  pluginConfiguration:
    name: barman-cloud.cloudnative-pg.io
```

### Attachments

With `YUVA_STORAGE=local`, archive the volume:

```sh
docker run --rm -v yuva_attachments:/data:ro -v "$PWD":/backup alpine \
  tar czf /backup/yuva-attachments-$(date +%F).tgz -C /data .
```

With `YUVA_STORAGE=s3`, use the provider's own protection (versioning, replication) or copy the
bucket with a tool such as `rclone sync`.

Take the database backup first and the attachments after it, so every file the database refers to
is in the copy.

## Restore

```sh
docker compose stop yuva
docker compose exec -T db sh -c 'dropdb -U yuva --force yuva && createdb -U yuva yuva'
docker compose exec -T db pg_restore -U yuva -d yuva --no-owner < yuva-2026-10-07.dump
docker run --rm -v yuva_attachments:/data -v "$PWD":/backup alpine \
  sh -c 'rm -rf /data/* && tar xzf /backup/yuva-attachments-2026-10-07.tgz -C /data'
docker compose start yuva
```

Start the server with the same `YUVA_MASTER_KEY` the backup was made with. Restore into an empty
database; a restore of an older version's backup is migrated forward when the server starts.

## Retention

What Yuva deletes by itself, once an hour:

| Data | Kept |
|---|---|
| Realtime events (replayed to clients that reconnect) | 24 hours |
| Expired contact sessions | until they expire, 7 days after their last use |
| Finished webhook deliveries | 7 days; the newest 100 attempts per endpoint stay in the log |
| Stale realtime connection records | 1 hour |

Conversations, messages, attachments and raw e-mails are kept until their contact is deleted (in
the panel, with `DELETE /v1/contacts/{contactId}`, or with `DELETE /v1/contacts/by-external-id`
from your backend), which removes the contact's conversations and files with it. Retention periods per workspace for closed
conversations and raw e-mails are not in 0.0.1.

## Monitoring

| Endpoint | Port | Answers |
|---|---|---|
| `GET /healthz` | 8080 | `200 {"status":"ok"}` while the process runs. Does not touch the database. Use it as the liveness check. |
| `GET /readyz` | 8080 | `200` when the database is reachable, `503` otherwise. Use it as the readiness check and for uptime monitoring. |
| `GET /v1/version` | 8080 | The running version. |
| `GET /metrics` | 9090 (`YUVA_METRICS_ADDR`) | Prometheus metrics: `yuva_http_requests_total` (by method and status), `yuva_http_request_seconds`, and the Go runtime and process metrics. |

Logs are JSON lines on stderr, one per request plus warnings and errors. Watch for:

- `mail not sent` — the server's SMTP account fails (sign-in codes do not arrive).
- `email send failed, retrying` — a channel's SMTP account fails; the message shows `failed` in the
  panel once the retries are used up.
- `YUVA_SMTP_HOST is not set`, `YUVA_INGRESS_SECRET is not set`, `YUVA_VAPID_PUBLIC_KEY is not set`
  at start — a feature is off by configuration.
- `not ready` — the database is unreachable.
