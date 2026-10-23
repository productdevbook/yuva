# Operations

How to upgrade, back up, restore and watch a Yuva server. The examples use the Compose setup from
the [install guide](install.md) and run from its directory.

## Upgrading to 0.1.0

Four things change when 0.1.0 starts. Read them, then follow [Upgrades](#upgrades).

### E-mail addresses must be proven

From 0.1.0 an e-mail channel receives mail at its own address (an MX record that points at Yuva,
`/ingress/email` or `yuva ingest-email --to`) only after its workspace has proven the address. Nothing
is carried over: every existing e-mail channel starts unproven, because stored mail cannot tell
delivery by MX from delivery through a forwarding address, which anyone can send to themselves.

| An unproven channel | |
|---|---|
| Stops | Routing mail by its own address, for channels whose domain's MX points at Yuva. A catch-all (`*@domain`) routes nothing. |
| Keeps working | Mail to its [forwarding address](email.md#forwarding-address) (`<token>@YUVA_FORWARDING_DOMAIN`), so Gmail or Outlook forwarding carries on. Sending through its SMTP account. |
| Is refused | A catch-all on `YUVA_FORWARDING_DOMAIN` or on a public mail provider such as gmail.com. It receives nothing; change its address. |

The server tells you about each channel once: in the operator digest under "E-mail address to prove"
when [`YUVA_OPERATOR_EMAIL`](configuration.md#sign-in-and-passkeys) is set, otherwise as a warning in
the log (`e-mail channel address must be proven`). Each names the workspace and the channel.

Prove a channel in one of three ways:

- **DNS.** The workspace adds a TXT record, `_yuva-mail.<domain>` with a value starting
  `yuva-mail-verify=` (both are in the channel's `email.verification`), and asks Yuva to check it. One
  record proves every channel of that workspace on the domain. This is the only way to prove a
  catch-all.
- **Code.** The workspace has a code mailed to the address and sends it back. When the mailbox
  forwards to the channel's forwarding address, the code mail proves the channel by itself.
- **You.** If you know the channel belongs to its workspace, vouch for it. Without `--yes` the command
  only lists what it would prove:

  ```sh
  docker compose exec yuva /yuva channel prove --all-unproven --workspace <ws>
  docker compose exec yuva /yuva channel prove --all-unproven --workspace <ws> --yes --plan <code>
  docker compose exec yuva /yuva channel prove --channel <id> --yes --address support@example.com
  docker compose exec yuva /yuva channel prove --channel support@example.com --workspace <ws> --yes
  ```

  The list shows each channel, workspace, address and name with a result: `would prove`,
  `already proven` or `refused: …`. It exits with an error while anything would be proven, and prints
  the plan code. `--yes` must repeat what you were shown: `--plan <code>` for `--all-unproven` (a
  code of the listed channels and addresses, so a changed list is refused), and `--address` for a
  channel given by id. `--workspace` is needed with `--all-unproven` and when several workspaces claim
  the same address. The proof is recorded as `channel.address_proven` by `operator` in the
  workspace's [audit log](#audit-log), and the channel shows the method `operator`.

  It is refused for an address another workspace has proven, for a catch-all on a shared domain, and
  when proving would take mail away from another workspace's proven channel ("would take from
  workspace …"). The operator never takes an address from anyone.

To avoid a gap, prove the channels that receive by MX right after the upgrade, or ask the workspaces
to add their TXT records first (the value appears in the channel once 0.1.0 runs). Workspaces prove
addresses through the API, as described in [E-mail](email.md#prove-an-address).

### Signatures and automatic replies

- **`YUVA_INGRESS_ACCEPT_V2` is now off by default.** The server accepts only `v3` signatures. The
  Email Worker in `edge/` signs `v3`; if you still run an older one, redeploy it before upgrading or
  set `YUVA_INGRESS_ACCEPT_V2=true` until you have. Mail signed `v2` is otherwise refused with `401`,
  and the Worker forwards it to `FALLBACK_FORWARD` or the sending server retries.
- **Automatic replies need DMARC.** Greetings, rule replies and rating requests are e-mailed only to
  senders whose mail passed DMARC according to [`YUVA_INGRESS_AUTHSERV_ID`](configuration.md#inbound-e-mail).
  A server without it sends none of them and warns at start. Set it before you rely on them.

### Mail without SMTP

Without `YUVA_SMTP_HOST` the server sends no mail and writes it to the log, redacted. A sign-in code is
one line, `sign-in code (log mailer, not sent)`, with the address reduced to `*@domain` and the `code`.
Every other mail logs only the recipient's domain and the sizes. `YUVA_MAIL_LOG=full` writes whole
mails (recipient, subject, text) and is refused at start unless `YUVA_PUBLIC_URL` is a localhost or
loopback address; use it only on your own machine. Log lines and retried send errors keep only the
domain of an address (`*@example.com`); the delivery error shown on a failed message in the panel
still holds the mail server's full answer.

### Job queues

Background work now runs on more queues, so a slow customer endpoint cannot hold up assignment or
notifications. A job that calls a server chosen by a workspace or another service runs off the
`default` queue. Every process runs all of them; the sizes are fixed and there is nothing to set.

| Queue | Workers per process | Runs |
|---|---|---|
| `default` | 10 | Assignment, automation, SLA checks, notifications, snooze wake-ups, webhook fan-out, retention and the rest that only reads and writes the database. |
| `webhooks` | 10 | Webhook deliveries. |
| `outbound` | 10 | Web Push, app push (APNs and FCM), Slack posts and issue-tracker sync. |
| `email` | 10 | Mail sent through a workspace's SMTP account: replies, list mail and deliverability checks. |
| `imports` | 2 | Help center imports. |

The `broadcast`, `migrations` and `analytics` queues are unchanged ([Configuration](configuration.md)).
Jobs queued on `default` before the upgrade are still worked there. To see a backlog per queue:

```sql
SELECT queue, state, count(*) FROM river_job
WHERE state IN ('available','running','retryable') GROUP BY 1, 2;
```

A growing `webhooks` backlog means slow or failing customer endpoints; it no longer delays anything else.

## Upgrades

1. Read the release notes of every version between yours and the new one.
2. [Back up](#backups) the database and the attachments.
3. Change the image tag in `compose.yaml` and start the new version:

   ```sh
   docker compose up -d
   ```

Migrations run on start; the server is ready when `/readyz` answers `200`. Run one server process
while upgrading. To go back, restore the backup from step 2.

## Backups

A full backup has three parts: the database, the attachments and the master key.

1. Dump the database. `pg_dump` takes a consistent snapshot while Yuva runs:

   ```sh
   docker compose exec -T db pg_dump -U yuva -d yuva -Fc > yuva-$(date +%F).dump
   ```

2. Then archive the attachments. With `YUVA_STORAGE=local`:

   ```sh
   docker run --rm -v yuva_attachments:/data:ro -v "$PWD":/backup alpine \
     tar czf /backup/yuva-attachments-$(date +%F).tgz -C /data .
   ```

   With S3, use the provider's versioning or replication, or copy the bucket with `rclone sync`.

3. Keep `YUVA_MASTER_KEY` apart from both ([Master key](install.md#master-key)).

Run steps 1 and 2 from cron, in that order, and copy the files off the host.

## Restore

Use the same `YUVA_MASTER_KEY` the backup was made with:

```sh
docker compose stop yuva
docker compose exec -T db sh -c 'dropdb -U yuva --force yuva && createdb -U yuva yuva'
docker compose exec -T db pg_restore -U yuva -d yuva --no-owner < yuva-2026-10-07.dump
docker run --rm -v yuva_attachments:/data -v "$PWD":/backup alpine \
  sh -c 'rm -rf /data/* && tar xzf /backup/yuva-attachments-2026-10-07.tgz -C /data'
docker compose start yuva
```

A backup from an older version is migrated forward when the server starts.

## Operator commands

Run them with `docker compose exec yuva /yuva <command>` (`-T` when piping). `<ws>` is a workspace id
or exact name.

```sh
docker compose exec -T yuva /yuva api-key create --workspace Example --name provisioning
```

Commands that print an id or a secret write only that to stdout.

| Command | What it does |
|---|---|
| `serve [--migrate=false]` | Runs the server (the default), after migrations unless `--migrate=false`. |
| `migrate up\|down\|status` | Applies pending migrations, rolls back the latest one, or lists them. |
| `bootstrap --email <address> --workspace <name> [--name <name>] [--locale en\|tr] [--allow-existing]` | Creates a workspace and its first owner. `--allow-existing` adds another workspace. |
| `api-key create --workspace <ws> --name <name> [--scope <scope>]... [--inbox <id>]...` | Creates an API key and prints the secret once. Default: every scope, every inbox. |
| `api-key list --workspace <ws>` | Lists keys, never their secrets. |
| `api-key revoke <id>` | Revokes a key. |
| `inbox create --workspace <ws> --name <name> [--slug <slug>] [--locale <tag>] [--timezone <zone>] [--mode live\|async] [--expected-reply-minutes <n>]` | Creates an inbox and prints its id. Defaults: slug from the name, `en`, `UTC`, `async`. |
| `inbox list --workspace <ws>` | Lists inboxes. |
| `inbox ses-tenant show --workspace <ws> [--inbox <id\|slug>]` | Prints each inbox's stored Amazon SES tenant state, sending status, cause, associated resources and channels. Calls no AWS operation. |
| `inbox ses-tenant sync (--workspace <ws> [--inbox <id\|slug>] \| --all) [--dry-run]` | Creates and updates tenants from AWS, then refreshes their status, one line per inbox. `--all` is the back-fill; `--dry-run` only prints what it would change. |
| `inbox ses-tenant reinstate --workspace <ws> --inbox <id\|slug>` | Lifts a pause that came from the reputation policy. |
| `channel create-email --workspace <ws> --inbox <id\|slug> --name <name> --address <address> [--display-name <name>] [--from-address <address>] [--smtp-host <host> [--smtp-port <n>] [--tls starttls\|tls\|none] [--smtp-username <name>] [--smtp-password-file <path\|->]]` | Creates an e-mail channel and prints its id. Password from a file, or stdin with `-`. |
| `channel create-chat --workspace <ws> --inbox <id\|slug> --name <name> --origin <origin>... [--allow-anonymous] [--ask-email-offline=false] [--greeting <text>]` | Creates a chat (web widget) channel and prints its id and public key (`yuva_pk_…`), separated by a tab. Repeat `--origin` for each site. |
| `channel list --workspace <ws> --inbox <id\|slug>` | Lists an inbox's channels, with a chat channel's public key and origins. |
| `channel prove --channel <id\|address> [--workspace <ws>] [--yes [--address <address>]]` | Vouches for an e-mail channel's address as the operator ([Upgrading to 0.1.0](#e-mail-addresses-must-be-proven)). Without `--yes`, lists what it would prove. Refused for an address another workspace proved and for catch-alls on shared domains. |
| `channel prove --all-unproven --workspace <ws> [--yes --plan <code>]` | The same for every unproven e-mail channel of the workspace. `--plan` is the code the list printed. |
| `workspace delete --workspace <ws> --yes` | Deletes a workspace ([below](#deleting-a-workspace-or-an-account)). Without `--yes`, a dry run. |
| `workspace broadcasts approve\|deny\|suspend --workspace <ws> [--reason <text>]` | Answers a workspace's broadcast approval, or suspends it ([below](#broadcasts-on-a-hosted-server)). |
| `workspace broadcasts limit --workspace <ws> --daily <n>` | Sets the workspace's recipients per UTC day; `0` removes the limit. |
| `workspace broadcasts show --workspace <ws>` | Prints its status, limit, 30-day rates and latest broadcasts. |
| `analytics property create\|list`, `analytics geoip-update`, `workspace analytics-limit` | Websites for [Analytics](analytics.md), its location database and a workspace's monthly event limit ([Configuration](configuration.md#analytics)). |
| `person delete --email <address> --yes` | Deletes a person's account. Refused while they are a workspace's only owner. |
| `ingest-email --to <address> [--from <address>]` | Delivers a raw message from stdin ([E-mail](email.md#any-mta-yuva-ingest-email)). |
| `master-key rotate\|check [--batch <n>]` | Re-seals stored secrets under the new master key, or reports what is not under it yet ([Rotating the master key](#rotating-the-master-key)). |
| `vapid-keys` | Prints a new Web Push key pair. Needs no database. |

Everything else (chat and app channels, members, webhooks, identity secrets) is managed in the
panel or through `/v1`.

## Rotating the master key

`YUVA_MASTER_KEY` seals the secrets Yuva stores (SMTP passwords, DKIM keys, webhook and integration
secrets, Slack, GitHub and Linear tokens, single sign-on client secrets, push credentials, import
and migration credentials, identity secrets, analytics visitor keys) and the links in the mail it
sends (unsubscribe, preferences, confirmation, rating and click links). Rotate it on a schedule your
policy sets. Mail keeps flowing and nobody is signed out while you do. If the key may have leaked,
use [If the master key leaked](#if-the-master-key-leaked) instead.

1. Start the server once on this version with the current key alone (any normal upgrade does this).
   It keeps each workspace's opt-out key (see below). While `YUVA_MASTER_KEY_PREVIOUS` is set, the
   server and `yuva master-key rotate` refuse to start if a workspace has opt-outs but no kept key,
   and say so.
2. Make a new key: `openssl rand -base64 32`.
3. On every process that has the key (the server, `yuva ingest-email` on a mail host, the machine
   you run `yuva` commands from), set the new key as `YUVA_MASTER_KEY` and the old one as
   `YUVA_MASTER_KEY_PREVIOUS`, then restart. New secrets are sealed under the new key and
   everything sealed under the old one still opens. The server logs a warning at start while
   `YUVA_MASTER_KEY_PREVIOUS` is set.
4. Re-seal what is stored:

   ```sh
   docker compose exec yuva /yuva master-key rotate
   ```

   It goes column by column and workspace by workspace in batches (`--batch`, default 500), prints a
   line per column (re-sealed, already under the new key, unreadable) and can be stopped and run
   again at any time: a second run only touches what is left. A secret someone changes in the panel
   meanwhile is not overwritten. It ends by checking everything again and exits with an error,
   listing the rows, if anything is still under the old key (for example a process that still runs
   with the old key as `YUVA_MASTER_KEY` wrote it): fix that process and run it again.
5. Confirm:

   ```sh
   docker compose exec yuva /yuva master-key check
   ```

   It writes nothing and exits with an error while any stored value is still under the old key (it
   lists the rows; run `rotate` again) or opens with neither key.
6. Keep `YUVA_MASTER_KEY_PREVIOUS` while links in mail sent before the switch must keep working:
   for unsubscribe links in broadcasts already sent as long as recipients may still use them, and for
   rating and click links as long as
   you want them to count. Then remove it, restart and run `master-key check` once more.

What changes and what does not:

- Opt-outs stay. Addresses that unsubscribed or complained are kept only as a keyed hash, which
  cannot be made again under a new key. Each workspace's hashing key is therefore stored sealed and
  re-sealed like the other secrets, so the list of objections survives every rotation. If the
  previous key is removed before `rotate` has run, the server refuses to start rather than mail
  people who objected: put `YUVA_MASTER_KEY_PREVIOUS` back. Rolling back (old key as
  `YUVA_MASTER_KEY`, new one as `YUVA_MASTER_KEY_PREVIOUS`) works at any point.
- The verification TXT value for e-mail addresses (`_yuva-mail.<domain>`) changes: the panel shows
  the new value at once. An address already proven stays proven, and a record with the old value
  still proves an address while `YUVA_MASTER_KEY_PREVIOUS` is set.
- Rate limits start over: their keys are derived from the master key.
- Codes already sent (address codes, Slack link codes), analytics share passwords already entered
  and migration test messages in flight keep working while the previous key is set.
- A value that opens with neither key is listed with its table, workspace and row. Nothing can read
  it: set that secret again in the panel, or delete the integration.
- A backup taken before the switch needs the old key. Keep it as long as you keep those backups
  ([Backups](#backups)).
- Only one previous key is accepted. Finish a rotation (`check` passes, previous key removed) before
  starting the next.

## If the master key leaked

Whoever has the key and a copy of the database can read every stored secret and forge the links in
your mail. Work through these in order.

1. Rotate: steps 1 to 5 of [Rotating the master key](#rotating-the-master-key). Run
   `master-key check` until it passes.
2. Remove `YUVA_MASTER_KEY_PREVIOUS` at once and restart. Unsubscribe, preferences, confirmation,
   rating and click links in mail sent before the switch stop working, because anyone holding the
   old key could forge them. People can still unsubscribe from the next mail.
3. Change the secrets themselves at their source, since the leaked key could open them: SMTP
   passwords (the broadcast SMTP account too), Slack, GitHub and Linear tokens, help-desk and
   mailbox migration credentials, single sign-on client secrets, APNs keys and FCM service
   accounts. Rotate webhook signing secrets and inbox identity secrets in Yuva and tell the
   receivers and the apps that sign with them. Make a new DKIM key for each sending domain and
   publish its record.
4. Opt-out hashes keep their key across a rotation by design, so the old key still derives it:
   someone with the old key and the database can test whether a given address objected to mail from
   a workspace. They cannot list the addresses. Treat that as exposed together with the database.
5. Sessions, API keys and contact sessions are stored as unkeyed hashes and do not depend on the
   master key. Revoke them separately only if the database itself leaked.

## Broadcasts on a hosted server

How [broadcasts](broadcasts.md) are limited is set by `YUVA_BROADCASTS` ([Configuration](configuration.md#broadcasts)).
With `on` there is nothing to do. With `approval`, which is the default when `YUVA_SIGNUP=open`, a
workspace created through sign-up cannot schedule broadcasts until you approve it, and every workspace
warms up from 500 recipients a day, doubling after each clean day. Workspaces made with
`yuva bootstrap` before or after are `allowed`, but still warm up.

Set `YUVA_OPERATOR_EMAIL` so you hear about it. The digest (at most one e-mail every 5 minutes) lists,
without mail content or recipients' addresses:

| Line | When |
|---|---|
| Broadcast approval requested | An owner asked: the workspace name, the person's address, the website, who sends and what, how subscribers agreed. |
| Workspace suspended from broadcasts | The server suspended it because of its 30-day complaint or bounce rate, with the rates. |
| Broadcast paused | A broadcast paused itself (bounce rate, complaint rate or sender error), with the broadcast id and the reason. |
| SES tenant paused | Amazon SES paused an inbox's tenant: the workspace and inbox names, who paused it and SES's cause. |

A workspace is suspended when at least 0.3% of at least 1,000 broadcast mails of the last 30 days were
reported as spam, or at least 8% of at least 2,000 bounced. Its running broadcasts pause.

Answer with the CLI. A workspace is a name or an id, and `--reason` (up to 500 characters) is shown to
the workspace's owners:

```sh
docker compose exec -T yuva /yuva workspace broadcasts approve --workspace Example
docker compose exec -T yuva /yuva workspace broadcasts deny --workspace Example --reason "No subscribe form on the site"
docker compose exec -T yuva /yuva workspace broadcasts suspend --workspace Example --reason "Complaints"
docker compose exec -T yuva /yuva workspace broadcasts limit --workspace Example --daily 2000
docker compose exec -T yuva /yuva workspace broadcasts show --workspace Example
```

| Command | Effect |
|---|---|
| `approve` | The workspace may send. Also lifts a suspension; broadcasts that paused stay paused until a member resumes them. |
| `deny` | The request is answered with no; the workspace may ask again. |
| `suspend` | Refuses scheduling and resuming, and pauses the running broadcasts (reason `workspace_suspended`). |
| `limit --daily <n>` | The workspace's recipients per UTC day, across its broadcasts. The smallest of this, `YUVA_BROADCAST_DAILY_RECIPIENTS` and the warm-up applies. `0` removes this limit only. |
| `show` | The status and its reason, the daily limit and today's count, the last 30 days (sent, bounce and complaint rates), the sender domains it used and its latest 10 broadcasts. |

Only the operator lifts a suspension. Before approving, look at the website and how the subscribers
agreed.

## Amazon SES tenants on a hosted server

Several workspaces sending through one Amazon SES account share its reputation. SES tenants separate
them: Yuva keeps **one tenant per inbox**, and SES tracks reputation and pauses sending per tenant, so one
inbox's bad sending pauses only that inbox. Set it up once.

1. Verify your sending domains as identities in the SES account (see [Deliverability](deliverability.md#amazon-ses)).
2. Create a configuration set and, to learn bounces and complaints, an event destination on an SNS topic
   listed in `YUVA_SES_TOPIC_ARNS` (see [E-mail](email.md#bounces)). Give the set no suppression options
   of its own, so the tenant's apply.

   ```sh
   aws sesv2 create-configuration-set --configuration-set-name yuva --region REGION
   aws sesv2 create-configuration-set-event-destination --configuration-set-name yuva \
     --event-destination-name yuva-feedback --region REGION \
     --event-destination '{"Enabled":true,"MatchingEventTypes":["BOUNCE","COMPLAINT"],"SnsDestination":{"TopicArn":"arn:aws:sns:REGION:ACCOUNT:yuva-ses"}}'
   ```

3. Create an IAM user for Yuva with this policy and an access key. Replace `REGION`, `ACCOUNT`, `PREFIX`
   (the value of `YUVA_SES_TENANT_PREFIX`) and `CONFIG_SET`.

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "YuvaOwnTenants",
      "Effect": "Allow",
      "Action": [
        "ses:CreateTenant",
        "ses:GetTenant",
        "ses:DeleteTenant",
        "ses:TagResource",
        "ses:ListTenantResources",
        "ses:PutTenantSuppressionAttributes",
        "ses:UpdateReputationEntityCustomerManagedStatus"
      ],
      "Resource": "arn:aws:ses:REGION:ACCOUNT:tenant/PREFIX*"
    },
    {
      "Sid": "YuvaTenantResources",
      "Effect": "Allow",
      "Action": [
        "ses:CreateTenantResourceAssociation",
        "ses:DeleteTenantResourceAssociation"
      ],
      "Resource": [
        "arn:aws:ses:REGION:ACCOUNT:tenant/PREFIX*",
        "arn:aws:ses:REGION:ACCOUNT:identity/*",
        "arn:aws:ses:REGION:ACCOUNT:configuration-set/CONFIG_SET"
      ]
    },
    {
      "Sid": "YuvaReadAccount",
      "Effect": "Allow",
      "Action": [
        "ses:ListReputationEntities",
        "ses:ListEmailIdentities",
        "sts:GetAccessKeyInfo"
      ],
      "Resource": "*"
    }
  ]
}
```

   Yuva never sends with this key: the channels' SMTP users do. `ses:TagResource` is needed because tenants
   are created with tags. `ses:ListReputationEntities` can only be allowed on `*`; it is read-only, and Yuva
   asks only for tenants with its prefix. The policy cannot reach tenants outside the prefix.
4. Set `YUVA_SES_REGION`, `YUVA_SES_ACCESS_KEY_ID`, `YUVA_SES_SECRET_ACCESS_KEY` and
   `YUVA_SES_CONFIGURATION_SET` ([Configuration](configuration.md#inbound-e-mail)), and restart. All four or none.

A channel is managed when its SMTP host is the Amazon SES endpoint of `YUVA_SES_REGION` and its user name
is an access key of the same AWS account as the server's key. Channels through another account or region
are left alone. For a managed channel the server creates the inbox's tenant (with tenant-level
suppression of bounces and complaints), associates the configuration set and the sending identity, and
adds the headers to every mail through the channel. It does so when a channel is saved and on
`workspace broadcasts approve`; it never calls AWS to write at startup. For channels that existed before,
run the back-fill once:

```sh
docker compose exec -T yuva /yuva inbox ses-tenant sync --all --dry-run
docker compose exec -T yuva /yuva inbox ses-tenant sync --all
```

Every five minutes the server reads the status of all its tenants. When one becomes disabled, the
inbox's running and scheduled broadcasts through managed channels pause (reason `ses_tenant`; the
other inboxes keep sending), scheduling and resuming are refused with `409 ses_tenant_blocked`, support
replies are still tried and fail with SES's answer, and the digest gets a line. Nothing resumes by
itself. Deleting an inbox or a workspace deletes its tenants.

`show` prints who paused a tenant. If the pause came from the policy, `reinstate` sets SES's
reinstated status, and a member resumes the broadcasts. If Amazon SES itself paused it, only an AWS
Support case lifts it, and `show` says so.

Optional, for a reaction within seconds instead of five minutes: send SES's tenant events to the SNS
topic. The event only makes Yuva read the status again; its content is not trusted.

```sh
aws events put-rule --name yuva-ses-tenants --region REGION \
  --event-pattern '{"source":["aws.ses"],"detail-type":["Sending Status Enabled","Sending Status Disabled","Advisor Recommendation Status Open","Advisor Recommendation Status Resolved"]}'
aws events put-targets --rule yuva-ses-tenants --region REGION --targets Id=yuva,Arn=arn:aws:sns:REGION:ACCOUNT:yuva-ses
```

Allow EventBridge to publish to the topic with this statement in the topic's access policy:

```json
{
  "Sid": "AllowEventBridge",
  "Effect": "Allow",
  "Principal": { "Service": "events.amazonaws.com" },
  "Action": "sns:Publish",
  "Resource": "arn:aws:sns:REGION:ACCOUNT:yuva-ses"
}
```

## Deleting a workspace or an account

**A workspace** is deleted by an owner under **Settings → Workspace → Danger zone**, with
`DELETE /v1/workspace` and `{"name": "<exact name>"}`, or with `yuva workspace delete`. It stops
working at once; a background job then deletes its data and logs `workspace deletion` lines. People
keep their accounts.

**An account** is deleted by its person under **Settings → My profile**, with `DELETE /v1/me` and
`{"email": "<their address>"}`, or with `yuva person delete`. Their messages stay without an
author. A workspace's only owner must hand over ownership or delete the workspace first.

## Audit log

The audit log records who changed a workspace's access, credentials and data: who joined, left or
changed role; invites; API keys and connected apps created or revoked; webhooks and their secrets;
e-mail channel credentials and keys; e-mail addresses proven or lost; inbox access and deletions; conversations moved to the Trash, restored or deleted; messages removed (`message.redacted`: the conversation, whether it was a message or note, how many files, never the text); contact deletions; exports of
consent data and analytics; retention changes; whether API keys and assistants may send to customers or schedule broadcasts; and the operator's answers about broadcasts. Each
entry has the actor (a member, an API key, a member through a connected app, or the server's command
line), the action such as `api_key.created`, its target and a few details, and the time. An entry is
written in the same step as the change.

It never holds message content, secrets, passwords, a webhook's URL (only its host) or a contact's
name or address. A workspace's deletion is not in its log, which goes with the workspace; the
operator's digest names it when [`YUVA_OPERATOR_EMAIL`](configuration.md) is set.

Team changes are recorded as `team.created`, `team.deleted`, `team.member_added`, `team.member_removed`,
`team.inbox_access_granted` and `team.inbox_access_revoked`.

E-mail address proofs are recorded as `channel.address_verified` (`address`, `method`), `channel.address_lost`
(`address`; by the system, when another workspace proved the address later) and `channel.address_proven`
(by the operator, [above](#e-mail-addresses-must-be-proven)). The two settings are recorded as
`workspace.bots_may_send_changed` and `workspace.broadcasts_by_keys_changed`, with `from` and `to`, only when the
value changes.

Automation rule changes are recorded as `automation_rule.created`, `automation_rule.updated` and
`automation_rule.deleted` ([Automation rules](automation.md#10-access)).

Only owners read it: in the panel under **Settings → Audit log**, or with `GET /v1/audit-log`
(admins and agents get `403 forbidden`). An API key reads it with the `audit:read`
[scope](headless.md#scopes), as long as the member who made the key is still an owner.

```sh
curl -H "Authorization: Bearer $YUVA_KEY" \
  "$YUVA_URL/v1/audit-log?action=api_key.created&action=api_key.revoked&since=2026-01-01T00:00:00Z"
```

Newest first, 25 entries a page (up to 100 with `limit`), `next_cursor` for the next page.

| Filter | Shows |
|---|---|
| `actor_id` | Entries by this member, key or connected app. A member's id also matches what they did through connected apps. |
| `actor_kind` | `member`, `api_key`, `oauth_grant` or `operator`. |
| `action` | These actions; repeat the parameter or separate them with commas. |
| `since`, `until` | A time range: at or after `since`, before `until`. |

Entries are kept for [`YUVA_AUDIT_DAYS`](configuration.md) days (365 by default).

## Retention

Conversations, messages and files stay until their contact is deleted (in the panel,
`DELETE /v1/contacts/{contactId}` or `DELETE /v1/contacts/by-external-id`). An owner can set a
retention period under **Settings → Workspace**, or with `PATCH /v1/workspace` and
`{"retention_days": 90}`; `null` keeps everything (the default).

Yuva cleans up once an hour:

| Data | Kept |
|---|---|
| Events (realtime replay and `GET /v1/events`) | 7 days |
| Contact sessions | until they expire, 7 days after last use |
| Finished webhook deliveries | 7 days; the newest 100 per endpoint stay |
| Stale realtime connection records | 1 hour |
| Conversations in the [Trash](headless.md#deleting-conversations), with messages and files | 30 days |
| Closed conversations, with messages and files (when `retention_days` is set) | that many days since their last change |
| Recipients of a finished broadcast (its counts stay) | 400 days after it finished |
| Daily sending totals behind the limits | 90 days |
| [Audit log](#audit-log) entries | `YUVA_AUDIT_DAYS` days (365 by default) |
| Original `.eml` of inbound mail (when `retention_days` is set) | that many days; the message itself stays |

Whenever conversations are deleted for good (a contact, an inbox, retention, the Trash cleanup or a workspace),
the same step also deletes their stored events and every webhook delivery, pending or kept, that carries them:
`/v1/events`, realtime replay and the delivery log stop showing the content at once, and a pending delivery is not
retried. The stored files (attachments, raw e-mails, e-mail images, import uploads) are deleted by a job that
retries until the store answers, so a failing store leaves no files behind that nothing points to.

## Several server processes

Any number of server processes can share one database (more `replicaCount` in the Helm chart, or
several containers behind one proxy). They coordinate through Postgres:

- **Rate limits hold across processes.** Every limit (session starts, widget writes, OAuth grants and
  MCP tokens, public pages, status monitor calls, analytics collection, ...) counts in the
  `rate_limits` table, so ten processes allow what one does, whichever process a request reaches.
  The 4 analytics queries a workspace may run at once are leases in `rate_slots`, shared the same way.
- Both tables are `UNLOGGED`: a Postgres crash or a failover to a standby empties them, which only
  starts every limit's window afresh. Backups need not keep them
  (`pg_dump --no-unlogged-table-data` leaves their rows out).
- Keys are keyed hashes (HMAC under the master key), never client addresses or e-mail addresses.
- The `rate_limit_sweep` job deletes ended windows and leases every minute.
- Windows and leases are measured by the database's clock, so the processes' clocks do not matter to
  the limits.
- Limits that all visitors of one chat channel, help center or page share are split per network. After
  the limit per address (IPv4 /32, IPv6 /64), one network (IPv4 /24, IPv6 /48) may use only part of the
  shared budget, so a flood from a few sources cannot lock everyone else out:

  | Shared budget | One network | All visitors |
  |---|---|---|
  | Session starts of a channel | 120 a minute | 6,000 a minute |
  | Writes of a channel's visitors | 240 a minute | 12,000 a minute |
  | Help center search | 120 a minute | 12,000 a minute |
  | List sign-ups of a channel | 40 an hour | 6,000 an hour |
  | Status page sign-ups | 40 an hour | 3,000 an hour |
  | What's new reads | 480 a minute | 24,000 a minute |

- A check that fails in the database (a lost connection, a timeout) never locks out a signed-in member,
  API key or connected app: those requests go through. Every other request (visitors, contacts) is
  counted in the process's own memory with the same limits until the database answers; the server
  logs `rate limit check failed; counting anonymous requests in this process` at most once a minute
  while it lasts.
- [`YUVA_CLIENT_IP_HEADER` and `YUVA_TRUSTED_PROXIES`](configuration.md) must be the same on every
  process, or the per-address limits count different addresses.

## Monitoring

| Endpoint | Port | Answers |
|---|---|---|
| `GET /healthz` | 8080 | `200` while the process runs. Liveness check. |
| `GET /readyz` | 8080 | `200` when the database is reachable, else `503`. Readiness and uptime check. |
| `GET /v1/version` | 8080 | The running version. |
| `GET /metrics` | 9090 | Prometheus: `yuva_http_requests_total`, `yuva_http_request_seconds`, Go and process metrics. |

Logs are JSON lines on stderr. Lines worth an alert:

| Log message | Meaning |
|---|---|
| `mail not sent` | The server's SMTP account fails; sign-in codes do not arrive. |
| `e-mail channel address must be proven` | An e-mail channel is unproven and `YUVA_OPERATOR_EMAIL` is not set to hear of it ([Upgrading to 0.1.0](#e-mail-addresses-must-be-proven)). |
| `rate limit check failed` | The database could not count rate limits ([Several server processes](#several-server-processes)). |
| `email send failed, retrying` | A channel's SMTP account fails. |
| `YUVA_SMTP_HOST is not set` (and the same for `YUVA_INGRESS_SECRET`, `YUVA_VAPID_PUBLIC_KEY`) | A feature is off by configuration. |
| `not ready` | The database is unreachable. |

## Tracing

Yuva exports OpenTelemetry traces over OTLP to any collector or backend that accepts them (Jaeger,
Tempo, Grafana Cloud, Honeycomb and others). Tracing is off, with no overhead, until you set
`OTEL_EXPORTER_OTLP_ENDPOINT`; the variables are in [Configuration](configuration.md#tracing). At start the
server logs `OpenTelemetry traces are on`, and it sends the last spans when it stops.

What is traced:

| Span | Named | Covers |
|---|---|---|
| HTTP request | the route pattern, for example `GET /v1/conversations/{id}` | Every request, with its method, route and status code. An incoming `traceparent` header is continued. |
| Background job | `river.work/<kind>` | Each run of a job, linked to the request that queued it. |
| Database query | the query name, for example `GetConversation` | Queries made while serving a request or running a job. The job queue's own polling is not exported. |
| Mail send | `mail.send`, `smtp.send` | An outgoing message and its SMTP transaction. |
| Webhook delivery | `webhook.deliver` | One delivery attempt. |

What is never recorded: message bodies, e-mail addresses, header values, tokens, secrets, URLs with
their query strings, client addresses, SQL text and query arguments, and error messages. A span carries
only attributes from a fixed list, enforced before export, so no later change in a library can add more:

- request: `http.request.method`, `http.route` (the pattern, never the path), `http.response.status_code`
- workspace and inbox ids: `yuva.workspace_id`, `yuva.inbox_id`
- jobs: `kind`, `queue`, `attempt`, `priority`, `id`, `status`, `messaging.system`
- mail: `yuva.recipients` (a count) and `yuva.message_bytes`
- database and errors: `db.system.name`, the SQL state code and the exception type

Log lines written while a request or job is traced carry `trace_id` and `span_id`. Copy the id from a
log line into your tracing backend to open the trace.

Sampling: by default every request is traced. To keep a share, set the sampler, for example
`OTEL_TRACES_SAMPLER=parentbased_traceidratio` and `OTEL_TRACES_SAMPLER_ARG=0.1`.

Example with a local collector, which prints the spans it receives:

```yaml
# otel-collector.yaml
receivers:
  otlp:
    protocols:
      http:
        endpoint: 0.0.0.0:4318
exporters:
  debug:
    verbosity: detailed
service:
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [debug]
```

```sh
docker run -d --name otel-collector -p 4318:4318 \
  -v "$PWD/otel-collector.yaml:/etc/otelcol/config.yaml" \
  otel/opentelemetry-collector:latest
```

Add to `.env`, then restart the server:

```sh
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
```

Open any page of the panel and read the collector's log (`docker logs otel-collector`): one span per
request with the route as its name. If Yuva runs in Docker too, use the collector's service name instead of
`localhost`.
