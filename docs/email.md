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

## Prove an address

A channel receives mail at its own address (an MX record that points at Yuva, `/ingress/email` or
`yuva ingest-email --to`) only after its workspace has proven the address. Until then it still receives
mail at its [forwarding address](#forwarding-address) and sends replies through its SMTP account. A
channel's `email.verification` holds its `state` (`unverified` or `verified`), the `method` that proved
it (`code`, `dns` or `operator`), the TXT record to add and, while a code is open, when it runs out.

| Way | For | How |
|---|---|---|
| Code | An exact address, not one on the forwarding domain | `POST /v1/channels/{channelId}/verification-code` mails an 8-digit code to the address. Send it back with `POST /v1/channels/{channelId}/verify` and `{"code": "…"}`. A code is valid for an hour and 5 tries, and a new one replaces it. If the mailbox forwards to the channel's forwarding address, the code mail proves the channel by itself. |
| DNS | Any address; the only way for a [catch-all](#catch-all-channels) | Add the TXT record `email.verification.txt_name` (`_yuva-mail.<domain>`) with the value `txt_value` (`yuva-mail-verify=…`), then `POST /v1/channels/{channelId}/verify` with no body. One record proves every channel of the workspace on that domain. |
| Operator | An address the operator trusts the workspace with | The server's operator runs `yuva channel prove` ([Operations](operations.md#e-mail-addresses-must-be-proven)). |

Owners, admins and API keys with `inboxes:manage` do the first two. A wrong or expired code answers
`400 verification_code_invalid`; a DNS check that finds nothing leaves the channel `unverified` with a
`problem`. Code mails are limited to one a minute per channel, 20 a day per workspace and 5 a day to one
address across the server (`429 rate_limited`), and DNS checks to 30 an hour per workspace.

- A catch-all on the server's forwarding domain or on a public mail provider (gmail.com, outlook.com, …)
  is refused (`400 validation_failed`): it would take other people's mail.
- Several workspaces may claim the same address, but only the one that proved it last receives its mail.
  When another workspace proves an address you had proven, your channel goes back to `unverified` with the
  problem `taken` and your owners get an e-mail (at most one a day, listing the addresses). Prove it again
  to take it back. A claim that is not proven blocks nobody.
- Proving an exact address also moves mail that a catch-all or `local@` channel of another workspace
  received for it until then; that workspace is told the same way.
- Changing a channel's address starts the proof over.
- Each proof and loss is in the [audit log](operations.md#audit-log).

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

The Email Worker signs with `v3`, which the server checks before it reads the message. A Worker
deployed before it signs `v2`, which is accepted only while `YUVA_INGRESS_ACCEPT_V2=true` (off by
default). With `v2` the message is read before it is known to be genuine, so anyone can make the
server buffer unsigned uploads (advisory GHSA-jfvx-vc82-56f7): redeploy the Worker instead of turning
it on. A proxy in front of the API must pass `Content-Length` on to
`/ingress/email`: `v3` refuses a chunked body.

### Any MTA: `yuva ingest-email`

An MTA that delivers to a command pipes the raw message in:

```sh
yuva ingest-email --to <envelope recipient> [--from <envelope sender>] < message.eml
```

It needs the server's `YUVA_DATABASE_URL`, `YUVA_MASTER_KEY` and storage settings. On a separate
mail host, use S3 storage so both write to the same place. Copy the binary out of the image:

```sh
docker create --name yuva-bin ghcr.io/productdevbook/yuva:0.3.0
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
authserv-id your MTA writes into `Authentication-Results`, or DMARC is not used and no automatic reply
is sent to a sender. Your MTA must put
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

## Writing new mail

`POST /v1/conversations/mail` (MCP `compose_mail`) writes a new e-mail from an inbox's e-mail channel
and opens its conversation in one step. It goes `To` up to 50 addresses, with up to 50 in `Cc` and 50
in `Bcc`, under the subject you give (no `Re:`). The first `To` address becomes the conversation's
contact; the other `To` and the `Cc` addresses become participants when the mail is sent, so their
answers join the conversation. `Bcc` recipients get the mail but never appear in it. With
`draft: true` the mail is kept as a draft and sent as written with `POST /v1/messages/{id}/send`.
A member or key writes at most 60 new mails a minute and reaches at most 300 recipients an hour. API
keys need `conversations:write`, `messages:write` and `contacts:write`.

Later replies choose who is copied: a reply without `cc` goes to the contact and copies every active
participant (Reply all); `cc: []` goes to the contact alone (Reply); a `cc` list copies exactly
those addresses. A bounce for one of several `To` addresses marks only that address undeliverable.

## Searching mail

The search box of the conversation list (`q` of `GET /v1/conversations`, `query` of MCP `search_conversations`)
takes operators on top of plain words. Write `key:value`, `key:"two words"`, or `-key:value` to leave
out what matches. Every operator and every other filter must hold. Keys and values are case-insensitive;
a term that is not one of these operators, or whose value does not fit, is searched as text, so
`ratio:3:1` and links still work. A `from:`, `to:` or `cc:` value needs at least 3 characters and no space or comma, and dates run from year 1 to 9999. Drafts of other members are never searched.

| Operator | Matches | Example |
|---|---|---|
| `from:` | a sent message whose sender address or name contains the value; `from:me` is you | `from:ada@example.com` |
| `to:`, `cc:` | a message with a recipient address that contains the value; `to:me` is you | `to:me cc:@example.com` |
| `subject:` | the subject, as the plain words match it | `subject:"quarterly invoice"` |
| `has:attachment` | a message with an attachment | `has:attachment invoice` |
| `is:starred` | conversations you starred | `-is:starred` |
| `in:` | a mail folder: `inbox`, `starred`, `snoozed`, `sent`, `archive`, `trash`, `drafts` or `scheduled` | `in:archive from:ada` |
| `after:`, `before:` | a message sent at or after, or before, midnight of a `YYYY-MM-DD` day | `after:2026-01-01 before:2026-02-01` |

`after:` and `before:` use the time zone the client sends as `timezone` (UTC without it). `in:trash`
needs the same rights as `trashed=true`. A query holds at most 200 characters and 20 operators.

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
for the whole domain. Give it a `--from-address` for chats that continue by e-mail. A catch-all receives mail
only after its domain is [proven with the TXT record](#prove-an-address), and not on a shared domain.

```sh
yuva channel create-email --workspace Example --inbox support --name "Everything else" \
  --address '*@example.com' --from-address support@example.com \
  --smtp-host smtp.example.com --smtp-username support@example.com --smtp-password-file -
```

### How inbound mail is handled

| Topic | Behaviour |
|---|---|
| Channel | Chosen by the envelope recipient, only through a proven address: the channel's forwarding address, then the proven exact address (`local+tag@domain` falls back to a proven `local@domain`), then the domain's proven catch-all. Unknown and unproven addresses are rejected. |
| Threading | By `In-Reply-To` and `References`; anything else starts a new conversation. A reply reopens the conversation. |
| Cc and participants | The `To` and `Cc` addresses of a mail that are not the workspace's own become the conversation's participants (at most 100). They are active only when the mail passed DMARC (which needs `YUVA_INGRESS_AUTHSERV_ID`); otherwise they are suggested, shown to members but never copied. A member can add or remove participants. An active participant's reply that passes DMARC joins the conversation as their message; anyone else who answers the thread starts a new conversation that links to it. Mail that fails DMARC or is automatic adds no one. The workspace's own addresses are the contact's addresses, the sender, every channel address of the workspace (proven or not), its catch-all domains, forwarding addresses and the server's sender; another workspace's address is an ordinary address. |
| Replies | Go `To` the contact and `Cc` the active participants; automatic mail (greetings, rules) copies no one. Each reply can replace the `Cc` and add `Bcc` (50 addresses each); `Bcc` recipients get the mail but never appear in it, and only members see them. An address that bounced or complained is refused or, as a participant, left out; an unsubscribe from broadcasts does not stop replies. |
| Forwarding | A member can forward one message or the whole conversation (its messages, never notes or drafts, with or without their files up to 20 MiB) to up to 10 outside addresses with a note on top. It goes from the inbox's e-mail channel with the member's name, `Reply-To` the channel address and the subject `Fwd: …`, and is shown to members only, never to the contact. The contact's address, addresses already copied on the conversation, Yuva's own addresses and addresses that bounced or complained are refused, and a member forwards at most 20 times an hour (200 per workspace). An answer to the forward that passes DMARC joins the conversation for members only and makes its sender a forwarded participant, who is not copied on replies; any other mail from them starts a new conversation. Only the member who forwarded and the assignee are notified of an answer. Assistants forward only where they may send; otherwise they leave a draft. |
| Automatic mail | Auto-replies, bulk mail and delivery reports are stored, never answered. Mail from the receiving channel's own address, from the server's sender or from any channel address of the same workspace (proven or not) is dropped; another workspace's addresses never drop mail. |
| DMARC | Used only with `YUVA_INGRESS_AUTHSERV_ID`, from the topmost `Authentication-Results` when it carries that authserv-id. Several headers carrying it mean the sender added one: a `fail` among them counts, a `pass` does not. The header is read as RFC 8601 says: only the result each method states counts, never text inside an address or a reason, and a header that does not parse is `unknown`. A `pass` counts only for a message with one From address. A failing new conversation is marked spam; a failing reply starts a new one. Mail read from your old mailbox while you [move to Yuva](migrate.md) is always `unknown`. |
| Automatic replies | A greeting, a rule's `send_reply` and a rating request go to an e-mail sender only when their mail passed DMARC. Without `YUVA_INGRESS_AUTHSERV_ID` none is sent. A rule reply that is held back fails the run with `auto_reply`. Chat and API conversations are not affected. |

### Limits

| Limit | Value |
|---|---|
| Message size | 25 MiB |
| Attachments | The channel's [attachment limits](configuration.md#attachments-and-storage); a part over them is skipped, the message is kept and the part stays in the raw e-mail |
| New conversations per sender and channel | 20 per hour; more mail joins their latest conversation |
| Inbound mails per sender and workspace | `YUVA_EMAIL_SENDER_HOURLY_CAP`, 500 per hour; more are refused. Mail that passed DMARC and other mail are counted apart. |
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
