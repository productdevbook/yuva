# Team mail

Team mail is Yuva as the mailbox of a team: shared addresses such as sales@ or billing@, and people's
own mailboxes, with the same conversations, labels, assignees and notes as support, but in words and
folders of their own. Your domain's mail is routed to Yuva, you read and write it in the panel, and
the history of your old mailbox is moved in.

## What it is and what it is not

An inbox has a purpose: **Support** (customers on chat, e-mail and in-app), **Team mail** (a `mail`
inbox) or **Both**, which means one workspace with inboxes of either kind. New workspaces are asked
which fits. A team mail inbox has e-mail channels only. The support features stay off for it: ratings,
SLA policies, automatic assignment, greetings, status, help, roadmap, What's new and support reports.

- Yuva is the mailbox. Mail arrives through your domain's [routing to Yuva](email.md#inbound) and is
  read in the panel, the mobile app and by [assistants](mcp.md).
- Yuva is not an IMAP, POP3 or SMTP server for mail apps. You cannot add it to Apple Mail,
  Thunderbird or Outlook, and it does not keep a mailbox at Gmail or Outlook in sync: [moving
  history](#moving-history) reads the old mailbox once and never writes to it.
- A reply goes out through the e-mail channel's own SMTP account, so sending needs the same setup as
  support e-mail.

## Shared and personal mailboxes

| | Shared mailbox | Personal mailbox |
|---|---|---|
| For | An address the team answers together: sales@, billing@ | One person's own mail |
| Who reads it | Members with access to the inbox, as for any inbox | Its owner, and the members the owner [delegates](#delegation) to |
| Who sets it up | Owners and admins | Any member, for themself; an owner or admin can create one for another member |
| Filters | Owners and admins | Its owner only |

A personal mailbox is private, even from owners and admins: see [Privacy](#privacy).

1. Open **Settings → Products → Add an inbox**, or in **Mail** the mailbox picker → **New mailbox**.
2. Under **Team mail**, answer **Who reads it?** with **Shared with the team** or **Personal (only you)**.
   Owners and admins also get **Personal for a member**: pick the member.

In **Mail** the mailbox picker groups mailboxes as **Your mailboxes**, **Shared with you** (marked
**Read** or **Read and send**) and **Team mail**. A personal mailbox can also be created through the API:
`POST /v1/inboxes` with `owner_member_id`.

## Route a domain's mail to Yuva

Each mailbox address is an [e-mail channel](email.md) on the inbox: **Settings → the inbox → Channels
→ E-mail**. Then:

1. [Prove the address](email.md#prove-an-address), with a code mailed to it or a DNS record.
2. Route its mail to Yuva with the [Cloudflare Email Worker or any MTA](email.md#inbound), or forward the
   old mailbox to the channel's [forwarding address](email.md#forwarding-address).
3. Give the channel an SMTP account to send replies and set up the [DNS records](email.md#dns-checklist).

A catch-all channel (`*@example.com`) takes the mail of every other address of the domain.

## Moving history

[Move to Yuva](migrate.md) copies the messages of your old mailbox (Gmail, Outlook, Yandex, iCloud and
any IMAP server, or an export file) into an inbox. Yuva only reads the old mailbox; nothing there is
changed. Pick your provider's guide from the [list](migrate.md). Owners and admins start a migration:
**Settings → Move to Yuva → New migration**. A migration into a personal mailbox is created, seen and run only by the mailbox's owner, and only when that owner
is an owner or admin of the workspace: migrations are a manager feature, so an agent cannot migrate into their own
mailbox. Other owners, admins, delegates and API keys do not see it. Deleting the mailbox deletes its migrations;
the deletion is refused while one is scanning, copying, syncing or removing history.

## The Mail section

**Mail** in the sidebar lists the mail of every team mail inbox you can read (**All mail**) or of one
mailbox. Its folders:

| Folder | What is in it |
|---|---|
| Inbox | Open conversations |
| Starred | Conversations you starred; a star is yours alone |
| Snoozed | Conversations hidden until their time |
| Sent | Conversations where you sent mail |
| Drafts | Your own unsent mail and replies |
| Scheduled | Your mail waiting for its send time |
| Archive | Conversations you archived (closed ones) |
| Trash | Deleted conversations, emptied after 30 days; listed for owners and admins |

Open a mail to reply, star, archive, snooze or move it to the Trash. **Filters** in the Mail header
opens the mailbox's [filters](#filters).

## Write new mail

Choose **New mail**. Under **From**, pick the mailbox it comes from, then fill in:

- **To**: up to 50 addresses. The first becomes the conversation's contact.
- **Cc** and **Bcc**: up to 50 each. Bcc recipients get the mail but are hidden from everyone else.
- **Subject** and the message, with attachments.

A mail is sent as written, without `Re:`. The other To and Cc addresses become participants when it
is sent, so their answers join the conversation. A conversation you start waits under **Sent** and the
first reply reopens it into the Inbox. You can write 60 new mails a minute and reach 300 recipients an
hour. The API call is `POST /v1/conversations/mail`.

### Signatures

Each member has a signature per inbox, and an inbox has a default that owners and admins set. Open
**Settings → You → Signature**, pick the inbox and write your own, use the default or none. In the
default, `{{member.name}}` and `{{member.title}}` become your name and title. The signature ends your
e-mailed replies and new mail; **Leave it out** drops it from one message. Notes and chat replies carry
none.

## Send as yourself

A mail you send from a team mail inbox goes from the channel's address with your name and the
channel's: `Ada Lovelace (Sales) <sales@example.com>`. Replies come back to the channel.

To send from your own address instead (`ada@example.com`):

1. Open **Settings → You → Your addresses** and choose **Add an address**. You can keep 10.
2. Yuva mails a 6-digit code to it, valid for 10 minutes. Enter it. If a channel of the workspace catches
   all mail of that domain, no code is mailed: the address waits for an owner or admin to approve it
   under **Addresses to send from** on your member entry in **Settings → Team → Members**.
3. In New mail or a reply, choose it in the address picker next to **From**.

An address works with a channel only when its domain is the channel's sending domain and the channel's
latest [deliverability check](deliverability.md) of that domain passed with aligned DKIM and DMARC.
Otherwise Yuva refuses the send and says why. An address already used by a channel or verified by another
member is refused.

## Drafts, Send later and undo

- **Drafts** save as you type. Close the window and the mail waits under **Drafts**; only you see it.
  Its recipients become contacts as soon as it is saved, like any address you write to.
- **Send later** (next to Send) schedules a mail for a time between a minute and 366 days ahead. It
  waits under **Scheduled**, where you can change its time (**Cancel schedule** returns it to a draft). If
  it cannot be sent when the time comes (the address was removed, you lost access), it becomes a draft with the
  reason and you are notified.
- **Undo**: a reply in a conversation waits 6 seconds before it goes out, and the notice after you
  schedule a mail has an **Undo** that cancels the schedule. A new mail is sent at once.

## Filters

A filter looks at every mail that arrives in a mailbox and acts on it. Open **Mail → Filters** and add one
with a name, conditions and actions. Filters run in the order of the list.

| Conditions | Actions |
|---|---|
| From, To, Cc, the mailing list (`List-Id`), subject, has an attachment | Add labels, star, archive, move to the Trash, snooze, forward to a member, change status or priority, assign |

Text values are 3 to 200 characters. A filter that stars, assigns or forwards to a member only works
for members who can read the mailbox. Drafts, sent mail, imported history and delivery reports never
run a filter. A mailbox keeps at most 100.

To start from a mail, open it and choose **Filter messages like this**: Yuva fills in the sender and
the mailing list, or the subject, and you pick the actions.

Owners and admins manage the filters of a shared mailbox. The filters of a personal mailbox are its
owner's: nobody else sees or edits them, and they act on that mailbox only.

## Search

The search box of Mail takes plain words and operators. Write `key:value`, `key:"two words"`, or
`-key:value` to leave out what matches. Every operator must hold; a term with another key is searched as
text.

| Operator | Matches | Example |
|---|---|---|
| `from:` | sender address or name contains the value; `from:me` is you | `from:ada@example.com` |
| `to:`, `cc:` | a recipient address contains the value; `to:me` is you | `to:me cc:@example.com` |
| `subject:` | the subject | `subject:"quarterly invoice"` |
| `has:attachment` | a message with an attachment | `has:attachment invoice` |
| `is:starred` | conversations you starred | `-is:starred` |
| `in:` | a folder: `inbox`, `starred`, `snoozed`, `sent`, `archive`, `trash`, `drafts`, `scheduled` | `in:archive from:ada` |
| `after:`, `before:` | a message sent on or after, or before, a `YYYY-MM-DD` day | `after:2026-01-01 before:2026-02-01` |

Details such as the length limits are in [Searching mail](email.md#searching-mail).

## Privacy

Owners and admins find someone else's mailbox in **Settings → Products** as `Personal mailbox of <owner>`, with a lock. The page shows **Addresses**, the conversation count and **Size**, and offers only
**Rename the mailbox** and **Delete mailbox**.

A personal mailbox is read only by its owner and the members the owner delegates to.

- Owners and admins see that a personal mailbox exists: its name, its addresses, its number of
  conversations and its size. Never its mail. They can rename or delete it.
- API keys and webhooks never reach a personal mailbox, nor do workspace automation rules, Slack or
  team and member grants of inboxes. An assistant reaches it only through its owner's own OAuth
  connection. Its events are not sent to anyone else.
- A personal mailbox's mailing lists, broadcasts, e-mail designs and analytics properties are its owner's alone:
  delegates, owners and admins neither see nor change them, also on Home.
- Contacts stay workspace-wide: there is one contact per address. A shared inbox that writes to an
  address a personal mailbox knows reuses that contact, and adding such an address by hand answers that
  it is taken. Only the mail in the personal mailbox is private, not the contact record. A contact is hidden only
  when every conversation of it is in personal mailboxes you cannot read and it has no external id.
- Retention, contact deletion and workspace deletion still cover a personal mailbox, and every
  delegation and hand-over is in the [audit log](operations.md#audit-log).

## Delegation

1. The owner opens **Settings → Products**, the mailbox, then **Delegates**.
2. Choose **Add a member**, pick the member and choose **Read** or **Read and send**.
3. Change or remove a delegate in the same list: they stop reading this mailbox at once.

A delegate with **Read** sees `You can read this mailbox but not change it.` With **Read and send** the
composer says `Sending on behalf of <owner>` and the thread shows `<delegate> on behalf of <owner>`.
Delegates get no Settings link for the mailbox.

The owner of a personal mailbox lets other members in, one by one, with one of two rights. Only the
owner can do this; owners, admins and delegates cannot.

| Right | Can | Cannot |
|---|---|---|
| Read | Read everything the owner reads: conversations, notes, attachments, search, counts, notifications | Reply, write notes, or change a status, star, label, snooze or assignee |
| Send | All of Read, plus reply, write new mail and forward, write notes, and change status, star, snooze, assignee and labels | Move mail out of the mailbox (owner only, for both rights), change its settings, channels, filters or delegates, run migrations, or manage its lists and broadcasts |

Neither right shows the owner's own drafts. Mail a delegate sends goes out "on behalf of" the owner:
`From` is the owner's name and address, with the delegate named in the `Sender` header. The message
shows both people. Removing a delegate takes the access away at once. API: `GET|PUT|DELETE
/v1/inboxes/{id}/delegates[/{memberId}]`.

## When a member leaves

1. Open **Team**, choose the member, then **Remove**. The dialog `Remove <name>?` lists each of their
   personal mailboxes.
2. For each one choose **Delete this mailbox**, or **Hand over** and pick the member in **Hand over to**.

An orphaned mailbox shows **No owner** and **Needs an owner** in **Settings → Products**. Choose **Hand
over** and pick the **New owner**, or **Delete mailbox**.

A personal mailbox does not become shared when its owner goes.

- **An owner or admin removes the member.** For every personal mailbox the member owns, the removal asks
  you to either **delete** it with its mail, or **hand it over** to a named member, who becomes its owner.
  The removal is refused until every personal mailbox of the member has a choice, and the member you hand over to must be active and not the one leaving. The delegates of a handed-over mailbox lose their access. Nobody can read the mailbox before you choose.
- **A member deletes their own account.** Their personal mailboxes are not deleted. Each loses its owner
  and its delegates and is orphaned: nobody reads it. Owners and admins still see it by name and size and,
  until they decide, can hand it over to a member (`POST /v1/inboxes/{id}/hand-over`) or delete it. Handing over works only on an orphaned mailbox and needs an owner or admin signed in to the panel, not an API key.

Both choices are in the audit log.
