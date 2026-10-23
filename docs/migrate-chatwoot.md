# Move from Chatwoot

Copy the conversations of a Chatwoot account into Yuva and send its support address to Yuva. Yuva only
reads Chatwoot. The overview is in [Move to Yuva](migrate.md). This works with Chatwoot Cloud and with a
Chatwoot you host yourself.

## Before you start

- Use an administrator. Chatwoot's API shows an agent only the inboxes the agent belongs to, so Yuva needs
  the access token of a user who is an **administrator** of the account. Yuva refuses a token of an agent.
- Copy the access token. In Chatwoot, open **Profile Settings** (your avatar, bottom left), scroll to
  **Access Token** and copy it. Chatwoot's page:
  [API](https://developers.chatwoot.com/api-reference/introduction).
- Find the account number. It is the number after `/app/accounts/` in the address bar while you are in
  Chatwoot, for example `1` in `https://app.chatwoot.com/app/accounts/1/dashboard`.
- On the Connect step, enter the **address** of your Chatwoot (`https://app.chatwoot.com`, or the HTTPS
  address of your own install), the **account number** and the **access token**, and press **Test
  connection**. A self-hosted address must be reachable from the internet; a server on a private address
  is refused unless the operator of Yuva allowed it.

Chatwoot has no read-only token, so the token could change data. Yuva's connection is what keeps it
read-only: it sends Chatwoot nothing but read requests, and sends the token only to the address you
entered.

## What is copied

| Chatwoot | Yuva |
|---|---|
| Conversations | Conversations, with all their messages |
| Incoming messages | Messages in |
| Outgoing messages | Messages out |
| Private notes | Notes, which only your team sees |
| Activity lines (assigned, resolved, label added) | Nothing |
| Status: open | Open |
| Status: pending and snoozed | Pending |
| Status: resolved | Closed |
| Conversation labels | Labels, as you map them (or none) |
| Agents | Team members, as you map them; others are kept by name |
| Assignee | The assignee, when the agent is mapped |
| Contacts | Contacts, with name, e-mail address and phone |
| Inboxes | Inboxes, one per Chatwoot inbox as you map them |
| Attachments | Files on the messages |

Conversations take their original times and arrive [quietly](migrate.md#how-history-stays-quiet).

## What is not copied

Canned responses, automation rules, macros, teams, custom attributes, CSAT ratings, the Help Center and
reports. Yuva copies only what the table above lists.

## Attachments

Chatwoot gives every attachment a link on its own storage (your install, or the storage service behind
it). Yuva fetches the file from that link without the token. A file larger than the server's attachment
limit stays behind.

## Limits

Chatwoot reports no limit. It throttles requests per IP address, with a figure the operator of an install
sets and Chatwoot Cloud does not publish. Yuva sends at most 60 requests a minute, waits as long as
Chatwoot asks when it answers `429 Too Many Requests` with a `Retry-After` header, and a minute without
one. A large account therefore takes a while: each conversation takes at least one request for its
messages, and one more for each twenty messages.

## The cutover

Today your support address reaches Chatwoot, through an e-mail inbox there or a forwarding rule at the
mail provider that hosts it. On the Switch step, copy the channel's
[forwarding address](email.md#forwarding-address) and change that so the support address reaches Yuva
instead. Until you do, Yuva reads Chatwoot every two minutes, so no conversation is lost. Replies sent
from Yuva go out from the channel's address.

Then press **Send a test message**: it works when the message reaches Yuva by the new route. After a quiet
period, or when you press **Finish now**, the migration finishes
([how](migrate.md#the-test-message-the-quiet-period-and-finishing)).

## After it finishes

Reset your access token: in Chatwoot, **Profile Settings → Access Token → Reset**. Yuva deletes its copy
when the migration ends but cannot revoke it in Chatwoot, so resetting is what ends its access. Keep the
forwarding as it is, and keep Chatwoot as an archive if you like.
