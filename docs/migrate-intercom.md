# Move from Intercom

Copy the conversations of an Intercom workspace into Yuva and send its support address to Yuva. Yuva only
reads Intercom. The overview is in [Move to Yuva](migrate.md).

## Before you start

- Create a private app for Yuva in your own workspace. In Intercom, open **Settings → Integrations →
  Developer Hub**, choose **New app** and name it "Yuva". Under **Configure → Authentication**, turn on only
  the read permissions: read conversations, read users and companies, read admins and read tags. Intercom's
  page: [Authentication](https://developers.intercom.com/docs/build-an-integration/learn-more/authentication).
- Copy the app's **access token** from the same page.
- On the Connect step, choose the **region** your workspace's data is hosted in (US, Europe or Australia) and
  paste the token. Yuva sends it as a Bearer token to that region's API host only, and shows it to nobody.

Yuva does not use OAuth: an app you create in your own workspace needs none. Yuva cannot tell which
permissions a token has, so the connection is what keeps it read-only: besides the token it sends Intercom
nothing but read requests (`GET`), so no conversation, contact or setting changes. Yuva never calls Intercom's
search, which needs `POST`.

## What is copied

| Intercom | Yuva |
|---|---|
| Conversations | Conversations, with all their messages |
| Comments by a user or lead | Messages in, from the person who wrote them |
| Comments by an admin or a bot | Messages out |
| Notes | Notes, which only your team sees |
| State: open | Open |
| State: snoozed | Pending |
| State: closed | Closed |
| Assignment and state changes | Nothing |
| Tags | Labels, as you map them (or none) |
| Admins | Team members, as you map them; others are kept by name |
| Assigned admin | The assignee, when the admin is mapped |
| Teams | Inboxes, one per team as you map them; conversations without a team are the folder "Unassigned" |
| Users and leads | Contacts, with name, e-mail and phone (a contact without an e-mail address is kept by name) |
| Attachments | Files on the messages |

Conversations take their original times and arrive [quietly](migrate.md#how-history-stays-quiet).

## What is not copied

Tickets (Intercom's separate ticket types), companies, custom attributes, Help Center articles, workflows,
Fin's configuration, ratings and reports. Yuva copies only what the table above lists.

## Very long conversations

Intercom returns at most the 500 most recent parts of a conversation. Yuva imports those and records the
conversation as an error on the migration (`truncated`), so you can see which ones lack their oldest
messages. Links people typed into messages are never opened; only the files Intercom attached are fetched,
from Intercom's file servers and without the token.

## Limits

Intercom allows 10,000 requests a minute per workspace for private apps, shared by all of them, counted in
10-second windows. Yuva stays at half of the limit Intercom reports (`X-RateLimit-Limit`) and waits when
Intercom answers 429. Intercom's page: [Rate limiting](https://developers.intercom.com/docs/references/rest-api/errors/rate-limiting).
A large workspace therefore takes a while: reading a conversation takes two requests.

## The cutover

Today your support address reaches Intercom, forwarded by the mail provider that hosts it or by a rule there.
On the Switch step, copy the channel's [forwarding address](email.md#forwarding-address) and change that
forwarding so the support address reaches Yuva instead. Until you do, Yuva reads Intercom every two minutes,
so no conversation is lost. Replies sent from Yuva go out from the channel's address.

Then press **Send a test message**: it works when the message reaches Yuva by the new route. After a quiet
period, or when you press **Finish now**, the migration finishes
([how](migrate.md#the-test-message-the-quiet-period-and-finishing)).

## After it finishes

Delete the app you made for Yuva: in Intercom, **Settings → Integrations → Developer Hub**. Yuva deletes its
token when the migration ends but cannot revoke it in Intercom, so deleting the app is what ends its access.
Keep the forwarding as it is, and keep Intercom as an archive if you like.
