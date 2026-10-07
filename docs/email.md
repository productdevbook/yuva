# E-mail

An e-mail channel gives an inbox a support address. Mail to the address opens or continues a
conversation; members' replies go out from the address through the channel's own SMTP account, in
the same thread.

```
sender ─► MX ─► Cloudflare Email Worker (edge/) ─► POST /ingress/email ─┐
sender ─► your MTA ─► yuva ingest-email, or POST /ingress/email ───────┼─► Yuva ─► channel SMTP ─► contact
                                         SES bounces ─► SNS ─► POST /ingress/ses ─┘
```

## Create the channel

In the panel: Settings → the inbox → Channels → E-mail. Or on the server:

```sh
yuva channel create-email --workspace Example --inbox support --name "Support" \
  --address support@example.com --display-name "Example Support" \
  --smtp-host email-smtp.eu-west-1.amazonaws.com --smtp-username <user> --smtp-password-file -
```

An address belongs to one channel on the whole server. Mail to `local+tag@domain` reaches the
channel of `local@domain` when no channel has the tagged address. `--from-address` sends from
another address than the one mail arrives at (replies still go to the channel address through
`Reply-To`).

### Catch-all channels

A channel whose address is `*@example.com` is the catch-all of that domain (one per domain): it
receives mail to every address of the domain that no other channel has exactly, after the
`local+tag` fallback. Each conversation remembers the address the contact wrote to, and replies go
out `From` and `Reply-To` that address, so the channel's SMTP account must be allowed to send from
the whole domain. The channel's `from_address` (which cannot itself be a catch-all) is used only
for conversations without such an address, such as a chat that continues by e-mail; without
either, the reply is refused (`email_no_sender`). When an inbox has both, chat and feedback replies
go out through the exact-address channel.

```sh
yuva channel create-email --workspace Example --inbox support --name "Everything else" \
  --address '*@example.com' --from-address support@example.com \
  --smtp-host smtp.example.com --smtp-username support@example.com --smtp-password-file -
```

## Inbound

Yuva does not listen for SMTP. Mail reaches it in one of three ways; all of them pass the raw
message and the envelope recipient, and the recipient picks the channel.

### Cloudflare Email Worker

`edge/` is a Cloudflare Email Worker that forwards every message it receives to `/ingress/email`,
signed with the ingress secret, and rejects the message with Yuva's reason when Yuva refuses it.
Cloudflare Email Routing becomes the MX of the domain you enable it on, so use a domain (or
subdomain) whose mail is not handled by another mail server.

1. Set `YUVA_INGRESS_SECRET` on the server (`openssl rand -hex 32`) and
   `YUVA_INGRESS_AUTHSERV_ID=mx.cloudflare.net`, the authserv-id Cloudflare Email Routing stamps on
   the `Authentication-Results` it adds, so its DMARC verdict is used; restart the server.
2. Deploy the Worker with your own Cloudflare account:

   ```sh
   cd edge
   bun install
   bunx wrangler login
   bunx wrangler secret put INGRESS_SECRET          # paste the same value as YUVA_INGRESS_SECRET
   bunx wrangler deploy --var YUVA_URL:https://support.example.com
   ```

   `wrangler.toml` contains nothing account-specific; the Worker is named `yuva-edge`.
3. In the Cloudflare dashboard, open the domain → Email → Email Routing, enable it and let it add
   its MX and SPF records.
4. Under Routing rules, add each support address with the action **Send to a Worker** →
   `yuva-edge`. For a [catch-all channel](#catch-all-channels), set the catch-all rule of Email
   Routing to the Worker. Mail to an address that no channel receives is rejected by Yuva with
   "No such recipient".
5. Send a test mail to the address and watch the conversation appear in the panel.

Cloudflare accepts messages up to 25 MiB. A message Yuva refuses (unknown recipient, blocked
sender, a sender over `YUVA_EMAIL_SENDER_HOURLY_CAP`) is rejected permanently with Yuva's reason.
When Yuva does not answer (5xx, network error, no answer within 20 seconds) the delivery fails and
the sending server may retry. To keep such mail during an outage, deploy with
`--var FALLBACK_FORWARD:you@example.org`, a verified destination address in Email Routing: the
message is then forwarded there as a plain e-mail and does not appear in Yuva. The same happens
when the server is busy: it processes at most `YUVA_INGRESS_MAX_CONCURRENT` messages at once and
answers further ones `503`.

The Worker signs with the `v2` signature, which covers the envelope sender. A Worker deployed
before it signs with `v1`, which the server refuses unless `YUVA_INGRESS_ACCEPT_V1=true`; redeploy
the Worker, or set that variable only for the time between upgrading the server and redeploying.

### Any MTA: `yuva ingest-email`

An MTA that delivers to a command pipes the raw message into `yuva ingest-email`:

```sh
yuva ingest-email --to <envelope recipient> [--from <envelope sender>] < message.eml
```

It prints what happened (`stored`, `duplicate`, `bounce` or `dropped`) and exits with a code from `sysexits.h`, which MTAs
map to bounces and retries: `0` accepted, `65` bad or too large message, `67` unknown recipient,
`77` refused (e.g. blocked sender), `75` temporary failure, try again.

The command needs the same `YUVA_DATABASE_URL`, `YUVA_MASTER_KEY` and storage settings as the
server, and the same storage: with `YUVA_STORAGE=local` it must write into the server's attachment
directory, so on a separate mail host use S3-compatible storage. The binary is static; copy it out
of the image with `docker create --name yuva-bin yuva:0.0.1 && docker cp yuva-bin:/yuva /usr/local/bin/yuva && docker rm yuva-bin`.

Postfix example. A wrapper that loads the settings, `/usr/local/bin/yuva-ingest`:

```sh
#!/bin/sh
set -a
. /etc/yuva/ingest.env
exec /usr/local/bin/yuva ingest-email "$@"
```

`master.cf`:

```
yuva      unix  -       n       n       -       -       pipe
  flags=q user=yuva argv=/usr/local/bin/yuva-ingest --to ${recipient} --from ${sender}
```

and route the support addresses to the `yuva` transport (for example
`support@example.com yuva:` in a `transport_maps` table).

### Any MTA: HTTP

Anything that can send an HTTP request can post the raw message to `/ingress/email` with the
envelope headers and an HMAC-SHA256 signature under `YUVA_INGRESS_SECRET` (version `v2`, over the
timestamp, the envelope recipient, the envelope sender and the body). The request format and the
answers are in [edge/README.md](../edge/README.md#ingress-contract). Set `YUVA_INGRESS_AUTHSERV_ID`
to the authserv-id your MTA writes into `Authentication-Results`; without it DMARC is not used.

### What happens to inbound mail

- Threads are matched by `In-Reply-To` and `References`; anything else starts a new conversation
  with the mail's subject. A reply reopens a closed, pending or snoozed conversation.
- The visible text has quotes and signatures stripped; the full text, sanitized HTML, attachments
  and the original message are kept. Remote images are blocked in the panel until a member loads
  them.
- Automatic mail (auto-replies, bulk and list mail, delivery reports, mail from our own domain) is
  stored but never answered automatically. Mail from an address Yuva sends from (any channel's
  address or sending address, and `YUVA_SMTP_FROM`) is dropped, so notifications and replies that
  reach a catch-all never loop.
- Volume does not bounce mail: a sender opens at most 20 new conversations per channel per hour,
  and further mail in that hour is added to their latest conversation on the channel. Only a sender
  over `YUVA_EMAIL_SENDER_HOURLY_CAP` inbound mails per hour (500 by default) is refused.
- DMARC comes only from the topmost `Authentication-Results`, and only when its authserv-id is
  `YUVA_INGRESS_AUTHSERV_ID`; otherwise the header is shown but DMARC counts as unknown. A new
  conversation whose first mail fails DMARC is marked spam, and a reply that fails DMARC never joins
  the thread it names (its `From` may be forged): it opens a new spam conversation that points to
  it. Contacts can be blocked; their mail is refused.
- A reply to a chat continuity e-mail from the address a visitor typed in the widget, before they
  confirmed it, joins the visitor's conversation and is marked as coming from an unverified
  address; the address is not added to the contact. Once the visitor confirms it, it becomes one of
  their addresses and the mark goes away.

## Outbound SMTP

Each e-mail channel sends through its own SMTP account: SES, Postmark, your own relay or any other
SMTP service. Set host, port, TLS mode (`starttls`, `tls` or `none`), user name and password in the
channel settings. The password is stored encrypted under the master key and never shown again.
Credentials are only sent over an encrypted connection; use `none` only for a relay that needs no
login. A channel without an SMTP account receives mail, but replies to its conversations are
refused (`email_not_configured`).

A reply goes from the channel address (or its sending address) with the channel's display name, to
the address the contact last wrote from, with `Re: <subject>` and the thread headers. Notes are
never sent. Each message shows its delivery state in the panel (`queued`, `sent`, `failed` with the
error); temporary SMTP errors are retried, permanent ones fail at once. An e-mail channel can also
send an automatic greeting to new conversations, at most once per contact within a set interval.

Chat and feedback replies that a contact has not read are e-mailed through the inbox's e-mail
channel (see `YUVA_CHAT_EMAIL_DELAY` in [Configuration](configuration.md#chat)), so give an inbox
that has chat or app channels an e-mail channel too.

The SMTP host is checked when the channel is saved: a loopback, private, link-local or metadata
address, or `localhost`, is refused (loopback and private ones are allowed with
`YUVA_SMTP_ALLOW_PRIVATE`). A host name is resolved and checked again on every send.

## Bounces

**Delivery reports by mail.** A bounce sent to the channel address (a standard delivery status
report from an empty envelope sender) for a message Yuva sent marks the message failed and the
recipient undeliverable. Nothing to set up.

**Amazon SES through SNS.** When a channel sends through SES:

1. Create an SNS topic, e.g. `yuva-ses`, in the region of your SES identity.
2. Send the identity's bounce and complaint notifications to it (SES → identity → Notifications),
   or add the topic as an event destination of the configuration set you send with, for bounce and
   complaint events.
3. Set `YUVA_SES_TOPIC_ARNS` to the topic ARN (comma-separate several) and restart Yuva.
4. Subscribe `https://support.example.com/ingress/ses` to the topic with protocol HTTPS and raw
   message delivery off. Yuva confirms the subscription itself.

Yuva checks the SNS signature and the topic, and acts only on notifications about messages it sent
and their recipients. A permanent bounce or a complaint fails the message and marks the address
undeliverable on the contact; replies to it are refused until a member clears it on the contact.

## DNS checklist

For every domain a channel sends from:

- **SPF**: a TXT record on the domain (or on the MAIL FROM domain your provider uses) that includes
  the SMTP provider, e.g. `v=spf1 include:amazonses.com ~all`. One SPF record per name; merge
  includes.
- **DKIM**: the records your SMTP provider gives you (CNAME or TXT), so mail is signed for your
  domain.
- **DMARC**: `_dmarc.example.com TXT "v=DMARC1; p=quarantine; rua=mailto:dmarc@example.com"`.
  Start with `p=none` while you check the reports, then tighten it.
- **MX**: for inbound, the records Cloudflare Email Routing adds, or your MTA's.
- **Custom MAIL FROM** (SES and some others): an MX and SPF record on the bounce subdomain your
  provider asks for, so SPF aligns with your domain.

Send a test reply to a mailbox you control and check that the headers show `spf=pass`,
`dkim=pass` and `dmarc=pass`.
