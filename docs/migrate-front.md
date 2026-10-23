# Move from Front

Copy the conversations of a Front company into Yuva and send its support address to Yuva. Yuva only reads
Front. The overview is in [Move to Yuva](migrate.md).

## Before you start

- Create an API token for Yuva. In Front, open **Settings → Developers → API tokens** and click **Create API
  token**. Name it "Yuva". Choose the feature **Access resources**, the namespaces **Global resources** and
  **Shared resources** (add **Private resources** only if personal inboxes should move; each teammate must
  allow API access in their own settings for that), and the permission **Read only**: leave Write, Delete
  and Send off. Create it and copy the token, which Front shows only once. Front's page:
  [Create and revoke API tokens](https://dev.frontapp.com/docs/create-and-revoke-api-tokens).
- On the Connect step, paste the token and press **Test connection**. Create the token as an admin who sees
  every inbox you want to move: Yuva reads only what the token can see.

Front shows a token's permissions to its admins but does not let a program read them, so Yuva cannot check
that the token is read-only. Yuva's connection is what keeps it read-only: it sends Front nothing but read
requests, and only to Front's API address, so no conversation, contact or setting changes.

## What is copied

| Front | Yuva |
|---|---|
| Conversations | Conversations, with all their messages |
| Received messages | Messages in |
| Messages your teammates sent | Messages out |
| Comments and internal messages | Notes, which only your team sees |
| Archived or resolved | Closed |
| Waiting for a reply | Pending |
| Other open conversations | Open |
| Tags | Labels, as you map them (or none) |
| Teammates | Team members, as you map them; others are kept by name |
| Assignee | The assignee, when the teammate is mapped |
| Contacts | Contacts, with name, e-mail address and phone |
| Inboxes | Inboxes, one per Front inbox as you map them |
| Attachments | Files on the messages |

Messages of other channels (SMS, chat, WhatsApp) arrive as plain messages with their text. Drafts and
deleted conversations are never imported. Conversations take their original times and arrive
[quietly](migrate.md#how-history-stays-quiet).

## What is not copied

Rules, macros, snippets, custom fields, links, reminders, discussions between teammates that are not part of
a conversation, analytics, and the original source of an e-mail: Front gives Yuva each message as formatted
text, so headers and original formatting are not kept. Yuva copies only what the table above lists. A
conversation whose requester has no e-mail address (SMS, for example) is kept as history without a channel.

## Limits

Front limits requests per minute for the whole company by plan (Starter 50, Professional 100, Enterprise
200), and every token shares the limit. Yuva stays at half of the limit Front reports, keeps its searches
for changes under the 40% Front allows them, and waits as long as Front's `retry-after` says when Front
asks it to slow down. A large account therefore takes a while. Front's page:
[Rate limiting](https://dev.frontapp.com/docs/rate-limiting).

## The cutover

Today your support address reaches Front, forwarded by the mail provider that hosts it or by a rule there.
On the Switch step, copy the channel's [forwarding address](email.md#forwarding-address) and change that
forwarding so the support address reaches Yuva instead. Until you do, Yuva asks Front every two minutes for
conversations with new activity, so no conversation is lost. Replies sent from Yuva go out from the
channel's address.

Then press **Send a test message**: it works when the message reaches Yuva by the new route. After a quiet
period, or when you press **Finish now**, the migration finishes
([how](migrate.md#the-test-message-the-quiet-period-and-finishing)).

## After it finishes

Delete the token you made for Yuva: in Front, **Settings → Developers → API tokens**, open the token and
click **Delete**. Yuva deletes its copy when the migration ends but cannot revoke the token in Front, so
deleting it is what ends Yuva's access. Keep the forwarding as it is, and keep Front as an archive if you
like.
