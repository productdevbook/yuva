# Move from Freshdesk

Copy the tickets of a Freshdesk account into Yuva and send its support address to Yuva. Yuva only reads
Freshdesk. The overview is in [Move to Yuva](migrate.md).

## Before you start

- Find your API key. Sign in to Freshdesk as an admin, click your profile picture at the top right and
  choose **Profile settings**. The key is under **Your API Key** on the right. Freshdesk's page:
  [Authentication](https://developers.freshdesk.com/api/#authentication).
- On the Connect step, enter the **API key** and your **subdomain**, the `acme` of `acme.freshdesk.com`, and
  press **Test connection**. Use the key of an admin: an agent's key is refused, because only admins can
  read the account's products and roles.

Freshdesk has no scopes and no read-only key: the key can do whatever the admin can. Yuva's connection is
what keeps it read-only: it sends Freshdesk nothing but read requests, and only to your account's
`freshdesk.com` address, so no ticket, contact or setting changes.

## What is copied

| Freshdesk | Yuva |
|---|---|
| Tickets | Conversations, with all their messages |
| A ticket's description | The first message: in, or out for a ticket an agent created by outbound e-mail |
| Customer replies | Messages in |
| Agent replies | Messages out |
| Private notes | Notes, which only your team sees |
| An agent's public notes | Notes, which only your team sees |
| A customer's public note | A message in |
| Status: open, and custom statuses | Open |
| Status: pending | Pending |
| Status: resolved and closed | Closed |
| Tags | Labels, as you map them (or none) |
| Agents | Team members, as you map them; others are kept by name |
| Responder | The assignee, when the agent is mapped |
| Requesters | Contacts, with name and phone |
| Products | Inboxes, one per product as you map them |
| Tickets without a product | The default inbox, as the folder "No product" |
| Attachments | Files on the messages |

Spam and deleted tickets are never read: Freshdesk leaves them out of its listing and Yuva never asks for
them. Tickets take their original times and arrive [quietly](migrate.md#how-history-stays-quiet), so an
imported note or reply is never sent to anyone.

## What is not copied

Automations, canned responses, satisfaction ratings, custom ticket fields, time entries, companies, groups,
roles, solution articles and reports. Yuva copies only what the table above lists.

## Attachments

Freshdesk keeps attachments at temporary, signed addresses on another host. Yuva fetches them from there
without your API key. When an address has expired, Yuva reads the ticket again once for a fresh one; a file
that is still refused, or larger than the server's attachment limit, stays behind.

## Limits

Freshdesk limits requests per minute by plan and counts invalid requests too. Yuva stays at half of the
limit Freshdesk reports (50 a minute until it has reported one), asks for the ticket list at most 20 times a
minute, and waits as long as Freshdesk asks when it answers 429. A request Freshdesk refuses for any other
reason than a missing ticket stops the copy for a while instead of being repeated. A large account
therefore takes a while. Freshdesk's page: [Rate limit](https://developers.freshdesk.com/api/#ratelimit).

Freshdesk ends a ticket list after 300 pages, which is 30,000 tickets. Yuva starts the next list at the
change time of the last ticket it read, so no ticket is read twice or missed.

## The cutover

Today your support address reaches Freshdesk, forwarded by the mail provider that hosts it or by a rule
there. On the Switch step, copy the channel's [forwarding address](email.md#forwarding-address) and change
that forwarding so the support address reaches Yuva instead. Until you do, Yuva reads Freshdesk every two
minutes, so no ticket is lost. Replies sent from Yuva go out from the channel's address.

Then press **Send a test message**: it works when the message reaches Yuva by the new route. After a quiet
period, or when you press **Finish now**, the migration finishes
([how](migrate.md#the-test-message-the-quiet-period-and-finishing)).

## After it finishes

Reset your API key: in Freshdesk, **Profile settings**, then **Reset API key**. Yuva deletes its copy of the
key when the migration ends but cannot reset it in Freshdesk, so resetting it is what ends its access. Keep
the forwarding as it is, and keep Freshdesk as an archive if you like.
