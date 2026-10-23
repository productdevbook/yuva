# E-mail

An e-mail channel gives an inbox a support address. Mail to it opens or continues a conversation,
and replies go out from that address in the same thread.

## Create the channel

In the panel, open **Settings → the inbox → Channels → E-mail**. Or on the server:

```sh
yuva channel create-email --workspace Example --inbox support --name "Support" \
  --address support@example.com --display-name "Example Support" \
  --smtp-host email-smtp.eu-west-1.amazonaws.com --smtp-username <user> --smtp-password-file -
```

The SMTP account sends the replies ([Outbound SMTP](#outbound-smtp)). Then route incoming mail to
Yuva with one of the three ways below, and set up [DNS](#dns-checklist).

## Inbound

Yuva does not listen for SMTP. Pick one of these three ways to bring mail in.

### Cloudflare Email Worker

`edge/` is a Cloudflare Email Worker that forwards mail to Yuva. Cloudflare Email Routing becomes
the MX of the domain, so use a domain or subdomain no other mail server handles.

1. On the server, set these and restart:

   ```sh
   YUVA_INGRESS_SECRET=<openssl rand -hex 32>
   YUVA_INGRESS_AUTHSERV_ID=mx.cloudflare.net
   ```

2. Deploy the Worker with your Cloudflare account:

   ```sh
   cd edge
   bun install
   bunx wrangler login
   bunx wrangler secret put INGRESS_SECRET          # the same value as YUVA_INGRESS_SECRET
   bunx wrangler deploy --var YUVA_URL:https://support.example.com
   ```

3. In the Cloudflare dashboard, open the domain → **Email → Email Routing**, enable it and let it
   add its MX and SPF records.
4. Under **Routing rules**, send each support address to the Worker `yuva-edge`. For a
   [catch-all channel](#catch-all-channels), point the catch-all rule at it.
5. Send a test mail and watch the conversation appear in the panel.

When Yuva is down or busy, the sender retries later. To keep such mail, deploy with
`--var FALLBACK_FORWARD:you@example.org` (a verified Email Routing address): it goes there instead.

An older Email Worker signs with `v1`, which the server refuses. Redeploy it, or set
`YUVA_INGRESS_ACCEPT_V1=true` until you do.

The Email Worker signs with `v3`, which the server checks before it reads the message. Workers
deployed before it sign with `v2`, which is still accepted while `YUVA_INGRESS_ACCEPT_V2` is on (the
default for now). With `v2` the message is read before it is known to be genuine, so anyone can
make the server buffer unsigned uploads (advisory GHSA-jfvx-vc82-56f7). Redeploy the Worker, then
set `YUVA_INGRESS_ACCEPT_V2=false`. A proxy in front of the API must pass `Content-Length` on to
`/ingress/email`: `v3` refuses a chunked body.

### Any MTA: `yuva ingest-email`

An MTA that delivers to a command pipes the raw message in:

```sh
yuva ingest-email --to <envelope recipient> [--from <envelope sender>] < message.eml
```

It needs the server's `YUVA_DATABASE_URL`, `YUVA_MASTER_KEY` and storage settings. On a separate
mail host, use S3 storage so both write to the same place. Copy the binary out of the image:

```sh
docker create --name yuva-bin ghcr.io/productdevbook/yuva:0.0.32
docker cp yuva-bin:/yuva /usr/local/bin/yuva && docker rm yuva-bin
```

For Postfix, write a wrapper `/usr/local/bin/yuva-ingest` that loads the settings:

```sh
#!/bin/sh
set -a
. /etc/yuva/ingest.env
exec /usr/local/bin/yuva ingest-email "$@"
```

Add it to `master.cf`, and route the support addresses to it in `transport_maps`
(`support@example.com yuva:`):

```
yuva      unix  -       n       n       -       -       pipe
  flags=q user=yuva argv=/usr/local/bin/yuva-ingest --to ${recipient} --from ${sender}
```

### Any MTA: HTTP

Post the raw message to `/ingress/email`, signed with `YUVA_INGRESS_SECRET`. The format is in
[edge/README.md](../edge/README.md#ingress-contract). Set `YUVA_INGRESS_AUTHSERV_ID` to the
authserv-id your MTA writes into `Authentication-Results`, or DMARC is not used. Your MTA must put
its own `Authentication-Results` on top of every message it passes on: Yuva reads the topmost one,
so a message the MTA leaves without one would be judged by a header its sender wrote.

## Forwarding address

When the server has a [forwarding domain](configuration.md#moving-to-yuva), every e-mail channel also
receives mail at a second address, `<token>@<forwarding domain>`. Forward a mailbox there, for example
from another provider while you [move to Yuva](migrate.md); forwarding a mailbox to its own address would
loop, which is why this address exists. Mail sent to it is handled like mail to the channel's own address.

You find it under **Settings → the inbox → Channels → E-mail**, with a copy button. "Rotate" gives the
channel a new one (`POST /v1/channels/{channelId}/forwarding-address/rotate`, `inboxes:manage`): mail to
the old address is refused from then on, so change the forwarding at the provider right away.

The operator points the forwarding domain's mail at Yuva once, the same way as the channels' own
domains (the three ways under [Inbound](#inbound)). Without a forwarding domain channels have no
forwarding address.

## Outbound SMTP

Each e-mail channel sends through its own SMTP account (SES, Postmark, your own relay). Set it in
the channel settings. Without one, the channel receives mail but replies are refused
(`email_not_configured`).

Chat and feedback replies a contact has not read are e-mailed through the inbox's e-mail channel
(after `YUVA_CHAT_EMAIL_DELAY`, see [Configuration](configuration.md#chat)). Give every inbox with
chat or app channels an e-mail channel too.

Private and loopback SMTP hosts, such as Mailpit in development, need `YUVA_SMTP_ALLOW_PRIVATE=true`.

### Amazon SES tenants

SES keeps reputation, and pauses sending, per tenant. When the server's operator has set up
[tenant management](operations.md#amazon-ses-tenants-on-a-hosted-server) and a channel sends through
the operator's SES account, each inbox sends in its own tenant: bounces or complaints of one inbox
pause only that inbox, never another inbox or workspace. There is nothing for your team to do. The
channel's sending page and the inbox settings show the state, to owners and admins:

| State | Meaning |
|---|---|
| Isolated | Mail carries the inbox's tenant. |
| Setting up | The server is creating the tenant. It takes seconds; a broadcast waits and starts by itself. |
| Identity missing | The sending address or its domain is not a verified identity in the SES account. See [Deliverability](deliverability.md#amazon-ses). Mail goes out without the tenant, and broadcasts are blocked. |
| Paused | Amazon SES paused the inbox's tenant. Replies fail with SES's answer; the server's operator reinstates it. |
| Error | A step failed; the page shows why. |
| Missing | The tenant no longer exists in Amazon SES. Mail goes out without it and broadcasts are blocked until the server creates it again. |

Mail through the managed account always carries `X-SES-CONFIGURATION-SET`, and `X-SES-TENANT` once the
tenant is ready. Both headers are added only on mail through Amazon SES.

With your own SES account, which the server does not manage, type the tenant yourself: set `ses.tenant`
and, if you use one, `ses.configuration_set` on the channel (letters, digits, `-` and `_`, up to 64).
Yuva adds them as headers on mail through SES and checks nothing in AWS. They are ignored while the
server manages the inbox's tenant.

## Bounces

A bounce or complaint marks the message failed and the address undeliverable; replies to it are
refused until a member clears the mark on the contact. Bounce reports sent to the channel address
need no setup. For Amazon SES, connect SNS:

1. Create an SNS topic, e.g. `yuva-ses`, in the region of your SES identity.
2. Send the identity's (or configuration set's) bounce and complaint notifications to it.
3. Set `YUVA_SES_TOPIC_ARNS` to the topic ARN and restart Yuva.
4. Subscribe `https://support.example.com/ingress/ses` to the topic over HTTPS, with raw message
   delivery off. Yuva confirms the subscription itself.

## DNS checklist

For every domain a channel sends from:

| Record | Value |
|---|---|
| SPF | TXT that includes your SMTP provider, e.g. `v=spf1 include:amazonses.com ~all`. One SPF record per name. |
| DKIM | The CNAME or TXT records your SMTP provider gives you. |
| DMARC | `_dmarc.example.com TXT "v=DMARC1; p=none; rua=mailto:dmarc@example.com"`; move to `p=quarantine` once reports look right. |
| MX | For inbound: the records Email Routing adds, or your MTA's. |
| Custom MAIL FROM | The MX and SPF records your provider asks for on the bounce subdomain (SES and others). |

Send a test reply to a mailbox you control and check for `spf=pass`, `dkim=pass` and `dmarc=pass`.

## Reference

### Catch-all channels

A channel with the address `*@example.com` receives mail for every other address of the domain.
Replies go out from the address the contact wrote to, so its SMTP account must be allowed to send
for the whole domain. Give it a `--from-address` for chats that continue by e-mail.

```sh
yuva channel create-email --workspace Example --inbox support --name "Everything else" \
  --address '*@example.com' --from-address support@example.com \
  --smtp-host smtp.example.com --smtp-username support@example.com --smtp-password-file -
```

### How inbound mail is handled

| Topic | Behaviour |
|---|---|
| Channel | The envelope recipient's channel; `local+tag@domain` falls back to `local@domain`. Unknown addresses are rejected. |
| Threading | By `In-Reply-To` and `References`; anything else starts a new conversation. A reply reopens the conversation. |
| Automatic mail | Auto-replies, bulk mail and delivery reports are stored, never answered. Mail from Yuva's own addresses is dropped. |
| DMARC | Used only with `YUVA_INGRESS_AUTHSERV_ID`, from the topmost `Authentication-Results` when it carries that authserv-id. Several headers carrying it mean the sender added one: a `fail` among them counts, a `pass` does not. A failing new conversation is marked spam; a failing reply starts a new one. |

### Limits

| Limit | Value |
|---|---|
| Message size | 25 MiB |
| Attachments | The channel's [attachment limits](configuration.md#attachments-and-storage); a part over them is skipped, the message is kept and the part stays in the raw e-mail |
| New conversations per sender and channel | 20 per hour; more mail joins their latest conversation |
| Inbound mails per sender and workspace | `YUVA_EMAIL_SENDER_HOURLY_CAP`, 500 per hour; more are refused |
| Messages processed at once | `YUVA_INGRESS_MAX_CONCURRENT`, 8; more get `503` |
| Time to send the message body | 30 seconds; slower uploads get `503` |
| Worker wait for Yuva | 20 seconds |

### `ingest-email` exit codes

| Code | Meaning |
|---|---|
| `0` | Accepted (`stored`, `duplicate`, `bounce` or `dropped`) |
| `64` | Wrong arguments |
| `65` | Bad or too large message |
| `67` | Unknown recipient |
| `75` | Temporary failure; try again |
| `77` | Refused, e.g. blocked sender |
