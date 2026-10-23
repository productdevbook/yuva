# Move to Yuva

Bring the history of the mailbox you answer from today into a Yuva inbox, then switch its mail over.
Yuva only reads the old mailbox: nothing in it is changed or deleted, and your team keeps working
meanwhile.

Owners and admins can move a mailbox: **Settings → Move to Yuva → New migration**. The same steps are
available to [assistants](#from-claude-code-or-another-assistant) and the [API](#from-your-backend).
Pick your provider's guide: [Gmail](migrate-gmail.md), [Google Workspace](migrate-google-workspace.md),
[Outlook.com](migrate-outlook.md), [Microsoft 365](migrate-microsoft365.md), [Yandex](migrate-yandex.md), [iCloud](migrate-icloud.md), [Zoho Mail](migrate-zoho.md),
[Yahoo Mail](migrate-yahoo.md), [Fastmail](migrate-fastmail.md), [GMX and mail.com](migrate-gmx.md),
[Zendesk](migrate-zendesk.md), [Help Scout](migrate-helpscout.md), [Chatwoot](migrate-chatwoot.md), [Front](migrate-front.md), [Freshdesk](migrate-freshdesk.md), [Intercom](migrate-intercom.md),
[any other IMAP server](migrate-imap.md) or [an export file](migrate-files.md) (Google Takeout, mbox, .eml or Maildir in a zip). Running your own server? See [Move to Yuva for operators](migrate-operators.md).
Zendesk, Help Scout, Outlook.com and Microsoft 365 connect with a sign-in instead of a password, Front with an API token, Freshdesk with an API key and your subdomain, Intercom with an access token.

## What Yuva does and does not do with your mailbox

Yuva never deletes, moves, archives or marks anything as read in your old mailbox, and never changes a
setting there. This is enforced three times, so no single mistake can change your mail:

- Yuva's only way to reach the mailbox is code that opens folders read-only (`EXAMINE`) and fetches
  messages without marking them (`BODY.PEEK`).
- Under that code, every command is checked before it leaves the server. Only `CAPABILITY`, `NOOP`,
  `LOGOUT`, `STARTTLS`, `LOGIN`, `AUTHENTICATE`, `LIST`, `STATUS`, `EXAMINE`, `UNSELECT`, `SEARCH` and
  `FETCH` of items that do not mark a message are allowed. Anything else closes the connection before it
  is sent.
- Tests that run on every change fail the build if a command that writes could be sent, and a test
  mailbox is checked to be byte for byte the same after a migration.

An app password could be used to change mail; Yuva's connection refuses every command that does. The
migration page shows "Changes made to {provider}: none".

## What is copied

| Copied | Not copied |
|---|---|
| Messages in the folders you choose, in both directions, with their original times | Spam, trash and drafts: never read |
| Attachments (those over the server's size limit or of a type it does not keep stay in the original message) | Messages larger than the server's limit for one message ([`YUVA_MIGRATION_MAX_MESSAGE_BYTES`](configuration.md#moving-to-yuva), 50 MiB) |
| Threads as conversations, joined by `References` and `In-Reply-To`, never by subject | Threads of only automatic mail (auto-replies, newsletters, delivery reports), unless you untick the option |
| Customers as contacts; your team's own mail as outgoing messages | Threads of only your team's mail |
| Folders and labels as Yuva labels, as you map them | Messages outside the date range you pick |
| The original source of each message, if you tick "Keep original messages" (uses more storage) | |

Old conversations are closed when their last message is older than the number of days you choose
(14 by default); newer ones stay open when the customer wrote last and pending when your team did.

## How history stays quiet

Imported conversations keep their original times and are marked **Imported** in the inbox, with a
divider after the last imported message. Mail from your team's addresses shows as sent by the member you
map it to or, without one, by its original name and address.

- No notification, e-mail, push, webhook, auto-reply or rating request is sent for imported mail.
- Nothing imported is unread.
- Imported mail is left out of the statistics (replies, first replies, closes) and counts in the
  imported columns of usage instead of the billed ones.
- A workspace's [retention](operations.md#retention) counts from the day of the import.
- The inbox shows a banner while the copy runs; everyone who sees the inbox sees it.

## The steps

1. **Source.** Enter the mailbox address. Yuva finds its provider from the address and the domain's
   mail servers; nothing is read yet. Or pick the provider yourself.
2. **Connect.** Enter the user name and an app password (the guide for your provider is next to the
   field) and press "Test connection". The test shows which step failed and how to fix it. Zendesk, Help
   Scout, Outlook.com and Microsoft 365 use a sign-in instead.
3. **Choose.** Pick the folders and how far back to go. Yuva scans the headers only, so it can tell
   you exactly what will be imported.
4. **Map.** Pick the inbox and its e-mail channel for the address (Yuva offers to create them), which
   member each team address belongs to, how folders become labels, when to close old conversations and
   whether to skip automatic mail or keep originals.
5. **Review.** The dry run lists the counts per folder (the total is every message in the chosen folders, with duplicates and skipped messages counted), what is skipped and why, contacts, labels, storage
   and time, and shows how the newest conversations will look. Nothing is imported until you press
   "Start migration". Changing a choice here plans the copy again.
6. **Copy.** Newest conversations first, with live progress. You can close the page, pause, resume or
   cancel; a copy that stops (a revoked app password, the provider's limit) continues where it stopped
   once fixed, and "Retry failed items" tries messages that failed again.
7. **Switch.** The history is in, and Yuva keeps reading the old mailbox every two minutes so nothing is
   lost while you send its mail to Yuva (below).
8. **Done.** The migration completes, its credentials are deleted and the page says where to remove the
   app password.

## Switching delivery

Mail the old mailbox received before the copy ended is history. Mail it receives afterwards is new mail:
it appears with notifications as if it had reached Yuva directly, without the inbox's greeting. Mail
that reaches Yuva both ways is stored once.

Send the mailbox's mail to Yuva in one of three ways:

| Way | Who | The old mailbox keeps a copy |
|---|---|---|
| Forward the mailbox to the channel's [forwarding address](email.md#forwarding-address) | the mailbox owner | yes |
| Route it on the server, as an admin, to the same address | a Google Workspace or mail server admin | yes |
| Change the MX records of a domain or subdomain used only for support ([the e-mail guide](email.md)) | whoever controls DNS | no |

A server without a forwarding domain ([`YUVA_FORWARDING_DOMAIN`](configuration.md#moving-to-yuva)) has no
forwarding address and offers only the MX way. Each provider guide shows the forwarding settings. If
Google mails a confirmation to the forwarding address, the code and link appear on this step.

## The test message, the quiet period and finishing

Press "Send a test message". Yuva mails the mailbox a short message; when it reaches Yuva by the new
route, the switch works and Yuva says whether it came by forwarding or directly. If it is found only in
the old mailbox after a few minutes, mail still reaches the old provider but is not forwarded yet.

Once a test message has arrived, Yuva keeps reading the old mailbox for a quiet period (24 hours by
default). If nothing arrives only there in that time, the migration finishes on its own. You can finish
earlier with "Finish now", or without a test message with "Finish without checking"; mail that then still
reaches only the old mailbox is not copied. A migration not finished after
[`YUVA_MIGRATION_SYNC_MAX_DAYS`](configuration.md#moving-to-yuva) (30 days) stops on its own.

Keep the forwarding or MX as it is after finishing: that is how mail reaches Yuva now. The old mailbox is
untouched; keep it as an archive if you like.

## The password

Yuva seals the password under the server's master key as soon as you enter it. It is never shown again,
and never appears in logs, events, API answers or assistant tools. Yuva deletes it when the migration
completes or is cancelled, when the sync limit passes, when you replace it and with the workspace. The
page then says "Credentials deleted". Yuva cannot revoke an app password at your provider: remove the one
you made for Yuva there (each provider guide links the page).

## Remove imported history

Settings → Move to Yuva → the migration → "Remove imported history", once it is completed or cancelled.
Yuva deletes the imported conversations that nobody wrote in since, with their messages and files, then
the contacts the migration created that are left without a conversation. Conversations your team wrote in
stay, and so do labels. You confirm the number first. Nothing changes in the old mailbox.

## From Claude Code or another assistant

Connect the assistant to [Yuva's MCP server](mcp.md) with the scope `migrations:manage` and run the
`move_to_yuva` prompt with the mailbox address (and optionally an inbox). The assistant finds the
provider, creates the migration, scans, shows the dry run and the newest conversations and starts the copy
only after you confirm the numbers. It explains how to switch the mail, sends the test message and
finishes the migration.

The assistant never asks for the password. It gives you a Yuva page; you enter the app password there,
and no tool takes one. Assistants cannot see the password or what was entered.

If you name an inbox but no channel, the assistant uses the inbox's e-mail channel for the address or creates
one. A new channel has no SMTP account: the assistant tells you what to set in the channel's settings in the
panel (the mailbox's SMTP server, user name and password) before Yuva can send replies from it.

## Your data

- Imported mail is your workspace's data like any conversation. It stays in the workspace's database and
  storage, and contact export and deletion, retention (counted from the import) and workspace deletion
  cover it.
- Yuva contacts only the mailbox you connect, with your credentials, and only to read. Detection looks up
  the domain's DNS records and its own mail settings file; nothing is sent to anyone else.
- You decide what to import (folders, dates) and are the controller of the personal data in it. A hosted
  operator processes it on your workspace's behalf, only to provide the import you asked for: never for
  ads or model training, and never read by people except for security, support at your request, or the law.
- Owners and admins see every action in the migration's activity; it is kept
  [`YUVA_MIGRATION_AUDIT_DAYS`](configuration.md#moving-to-yuva) (365) after the migration ends.

## From your backend

Owners, admins and API keys without an inbox limit that hold `migrations:manage` (never given to agents;
they get `403`). Full shapes are in the [API contract](../openapi/openapi.yaml).

| Endpoint | Use |
|---|---|
| `GET /v1/migrations/presets` | The providers this server offers, their servers and folders. |
| `POST /v1/migrations/detect` | Find the provider of an address. |
| `GET`, `POST /v1/migrations` | List migrations; start one (`draft`). |
| `GET`, `PATCH`, `DELETE /v1/migrations/{migrationId}` | Read; change the mailbox, inbox, channel and options; delete. |
| `PUT /v1/migrations/{migrationId}/credentials` | Connect the mailbox with a user name and password. |
| `GET /v1/migrations/{migrationId}/upload` | The parts of an [export file](migrate-files.md) the server has, to resume a cut upload. |
| `POST /v1/migrations/{migrationId}/upload/parts/{part}` | Upload one part (8 MiB at most) of the export file with its SHA-256. |
| `POST /v1/migrations/{migrationId}/upload/complete` | Finish the upload; Yuva reads the file and the migration becomes `connected`. |
| `POST /v1/migrations/{migrationId}/test` | Test the connection. |
| `GET /v1/migrations/{migrationId}/folders` | The mailbox's folders. |
| `POST /v1/migrations/{migrationId}/scan` | Read the headers for the dry run. |
| `GET /v1/migrations/{migrationId}/preview` | The newest conversations as they will look. |
| `POST /v1/migrations/{migrationId}/start` | Start copying. |
| `POST /v1/migrations/{migrationId}/pause`, `resume`, `cancel` | Pause, resume or cancel. |
| `POST /v1/migrations/{migrationId}/retry` | Retry failed messages. |
| `GET /v1/migrations/{migrationId}/errors` | What went wrong. |
| `POST /v1/migrations/{migrationId}/probe` | Send the test message. |
| `POST /v1/migrations/{migrationId}/finish` | Finish (`force` skips the check). |
| `POST /v1/migrations/{migrationId}/remove-history` | Remove imported history (`confirm_count`). |
| `POST /v1/channels/{channelId}/forwarding-address/rotate` | A new [forwarding address](email.md#forwarding-address) for an e-mail channel. |

States: `draft`, `connected`, `scanning`, `ready`, `copying`, `paused`, `needs_attention`, `syncing`,
`completed`, `cancelled`, `removing`, `removed`; each migration lists the `actions` allowed now. Errors
worth handling: `409 migration_state`, `409 provider_not_configured`, `409 confirm_count_mismatch`,
`422 hosted_limit` (the setting is named in `limit`) and `429` for the test message and connection tests.
Progress reaches owners and admins on `/v1/realtime` as `migration.updated`, `migration.progress` and
`migration.forwarding_confirmation`; migrations never send webhooks.
