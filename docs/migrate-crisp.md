# Move from Crisp

Copy the conversations of a Crisp website into Yuva. Yuva only reads Crisp. The overview is in
[Move to Yuva](migrate.md).

## Before you start

- Generate a website token. In Crisp, open **Settings → Workspace Settings → Advanced configuration → API
  Token** and generate a token for the workspace. Crisp shows an **identifier** and a **key**; copy both. Crisp's
  page: [Authentication](https://docs.crisp.chat/guides/rest-api/authentication/). Which Crisp plans offer the
  website token is up to Crisp; if the page is missing in your workspace, your plan may not include it.
- Find your **website ID**: it is the UUID in the address of your Crisp dashboard, after `/website/`.
- On the Connect step, enter the **identifier** as the API token, the **key** as the Key and the website ID as the Website,
  then press **Test connection**.

Crisp has no read-only scope for a website token, so the token could change data. Yuva's connection is what keeps
it read-only: it sends Crisp nothing but read requests, and the token only to `api.crisp.chat`. Plugin tokens
(Marketplace) are not used.

## What is copied

| Crisp | Yuva |
|---|---|
| The website | One inbox, as you map it |
| Conversations | Conversations, with all their messages |
| Visitor messages | Messages in |
| Operator messages | Messages out |
| Notes | Notes, which only your team sees |
| File and animation messages | Files on the messages |
| State: pending, unresolved | Open |
| State: resolved | Closed |
| Segments | Labels, as you map them (or none) |
| Operators | Team members, as you map them; others are kept by name |
| Assigned operator | The assignee, when the operator is mapped |
| The conversation's person (e-mail, nickname, phone) | A contact |

Events (for example a state change), pickers and fields are not messages and are not copied. A second person
who writes in a conversation keeps their own name on their messages. Files are fetched from Crisp's own storage
without the token; a file or animation that sits anywhere else, and links people typed into a message, are never
fetched. Conversations take their original times and arrive [quietly](migrate.md#how-history-stays-quiet).

## What is not copied

Contact profiles beyond the conversation's person, campaigns, the knowledge base, shortcuts, audio messages and
reports. Yuva copies only what the table above lists.

## Limits

A website token may make 10,000 requests a day. Yuva uses at most 5,000 of them, about two per conversation, and
stops for the rest of the day when it has used them, so a large website takes several days; the dry run states
how many. Crisp sends no limit headers: when it answers with too many requests, Yuva waits a minute and slows
down from then on. Crisp's page: [Rate limits](https://docs.crisp.chat/guides/rest-api/rate-limits/).

## The cutover

Crisp keeps chatting on your website until you replace its chat box with Yuva's. Mail from your support address
reaches Yuva once you point that address's forwarding at the channel's
[forwarding address](email.md#forwarding-address). Until the chat box is replaced, Yuva reads Crisp every two
minutes, so no conversation is lost.

Then press **Send a test message**: it works when the message reaches Yuva by the new route. After a quiet
period, or when you press **Finish now**, the migration finishes
([how](migrate.md#the-test-message-the-quiet-period-and-finishing)).

## After it finishes

Roll the token: in Crisp, open **Settings → Workspace Settings → Advanced configuration** and regenerate it. Yuva
deletes its token when the migration ends but cannot revoke it in Crisp, so regenerating it is what ends its
access. Keep Crisp as an archive if you like.
