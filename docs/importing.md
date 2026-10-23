# Importing subscribers

Bring people who already agreed to hear from you onto a [mailing list](lists.md) by uploading a CSV
file. Yuva checks every address before it adds anything, and keeps a record of where the people agreed.

Owners and admins can import. Everything on this page can also be done through the [API](#from-your-backend).

## 1. Prepare the file

Save your spreadsheet or export as CSV, with a header row.

| Limit | Value |
|---|---|
| Size | 20 MiB |
| Rows | 100,000 data rows (a server can set a lower limit, [`YUVA_IMPORT_MAX_ROWS`](configuration.md#broadcasts)) |
| Encoding | UTF-8 (with or without BOM), UTF-16 with BOM, Windows-1254 (Turkish Excel), Windows-1252 (Western Excel) |
| Separator | Comma, semicolon or tab, found from the header row |

You do not need to convert anything: a Turkish Excel file saved with semicolons imports with its
letters intact. A bigger file has to be split.

## 2. Upload it

In the panel: Broadcasts, then Lists, open the list, then the Import tab. The wizard has four steps:
file and consent, columns, review, import. You can leave the page at any step.

**File and consent.** Pick the file, then:

- Tick the statement that everyone in the file agreed to receive this list from your inbox and that you
  can show how and when.
- Write where and when they agreed, for example "Newsletter checkbox on the checkout page of our shop,
  2023 to 2026".

Both are required; without them the upload is refused. Yuva keeps them with the import and on the
consent record of every subscription it creates, so they appear in the list's
[consent export](lists.md#6-export-consent-records). They are your proof, and what you would show
someone who asks how you got their address. Write the real source: if you are not sure the people
agreed, do not import them.

**Ask everyone to confirm again.** Turn this on when the consent is old or you are not sure of it.
Nobody is subscribed at once: each new address becomes `pending` and gets the list's confirmation mail,
and joins only after clicking its link. The mails are spread over days by the workspace's daily sending
limit, so a big file does not go out in one burst. This needs an e-mail channel on the inbox that can
send.

Press "Upload and check". The file is read and every address checked, and nothing is added yet.

## 3. Columns

Yuva guesses what each column holds from its header. Check it: exactly one column is the e-mail
address; the others can be the name (or first and last name), language, an external id, an attribute
you name (letters, digits, dots, dashes and underscores, up to 64), or skipped. Attributes only fill
keys a contact does not have yet, so an import never overwrites what you already know. An attribute named like a contact [custom field](custom-fields.md) is read as the field's type, and left out when the value does not fit. Change a column
and press "Check again with these columns".

## 4. Review

Before you start, the page tells you what will happen: how many new subscribers, how many new contacts
are created, how many were already on the list (left as they are), and what is left out:

| Left out | Why |
|---|---|
| Not a valid e-mail address | The address is malformed. |
| The domain takes no mail | The domain has no mail server (no MX, A or AAAA record) or publishes a null MX. |
| Role addresses | Addresses such as `postmaster@`, `abuse@` or `noreply@`, which reach no person. |
| Already earlier in the file | A repeat. The first one is used. |
| Bounced or reported as spam before | The address is suppressed for all mail from the workspace. |
| Unsubscribed from everything or asked to be forgotten | The person objected to all your mail. |
| Unsubscribed from this list | The person left this list. Only they can come back, through a confirmation. |

An import never puts back anyone who unsubscribed, objected or bounced, whatever the file says.

Press "Import N addresses" to start.

## 5. While it runs

The page shows the rows done and the counts live. Rows are added 1,000 at a time. You can leave; the
import goes on, and Imports on the same tab lists the earlier ones. "Cancel import" stops it within a
moment: what it already added stays, with its consent record, and the uploaded file is deleted.

- **Only one import at a time** runs in a workspace. A second upload waits for the first to end or be
  cancelled.
- **No welcome mail** is sent, and no confirmation unless you chose to ask everyone again. Imports do
  not send a [webhook](webhooks.md) or event for every row.
- When it ends, the uploaded file is deleted.

### The rows that were left out

"Download the rows left out" gives a CSV with the line in your file, the address as you wrote it, and
the reason. The file is there for 7 days after the import ends. An import you uploaded but never
started is cancelled after 7 days.

| Reason | Meaning |
|---|---|
| `invalid_email` | Not a valid e-mail address. |
| `no_mail_domain` | The domain takes no mail. |
| `role_address` | A role address. |
| `duplicate` | Already earlier in the file. |
| `suppressed` | Bounced or reported as spam before. |
| `opted_out` | Unsubscribed from everything or asked to be forgotten. |
| `unsubscribed` | Unsubscribed from this list. |

A file that is not text answers `415 not_csv`, one over the size limit `413 file_too_large`. A file
over the row limit fails and nothing is added.

## From your backend

For a few people at a time, or sign-ups as they happen, use
[`POST /v1/lists/{listId}/subscriptions`](lists.md#5-add-subscribers-from-your-backend). For a file,
the import has the same steps over the API. The key needs `lists:write`.

```sh
curl -X POST https://support.example.com/v1/lists/LIST-ID/imports \
  -H "Authorization: Bearer $YUVA_API_KEY" \
  -F file=@subscribers.csv \
  -F 'attestation=Everyone in this file agreed to receive Newsletter from Example, and I can show how and when.' \
  -F 'consent_source=Newsletter checkbox on the checkout page, 2023 to 2026' \
  -F reconfirm=false
```

The answer is the import, `validating`. Poll `GET /v1/lists/{listId}/imports/{importId}` until `state`
is `ready`: `counts` and `columns` hold what the review shows, and `preview` the first rows. Change the
columns with `PATCH` (`{"columns": [{"index": 0, "field": "email"}, …]}`), then start it:

```sh
curl -X POST https://support.example.com/v1/lists/LIST-ID/imports/IMPORT-ID/start \
  -H "Authorization: Bearer $YUVA_API_KEY"
```

Poll again until `done`; `rows_done` grows as it goes, and `/v1/realtime` sends `list_import.progress`
and `list_import.updated`.

## Reference

| Endpoint | Use |
|---|---|
| `GET`, `POST /v1/lists/{listId}/imports` | List imports; upload a file (`file`, `attestation`, `consent_source`, `reconfirm`, `mapping`). |
| `GET`, `PATCH`, `DELETE /v1/lists/{listId}/imports/{importId}` | Read; change the columns (only while `ready`); cancel. |
| `POST /v1/lists/{listId}/imports/{importId}/start` | Start a `ready` import. |
| `GET /v1/lists/{listId}/imports/{importId}/errors` | The rows left out, as CSV. |

States: `validating`, `ready`, `importing`, `done`, `failed` (see `error`), `cancelled`. Errors worth
handling: `422 attestation_required`, `409 import_running`, `409 email_channel_required` (re-confirm
without a channel that can send), `409 not_ready`, `422 email_column_required`, `413`, `415`. Reading
needs `lists:read`; uploading, changing, starting and cancelling need `lists:write`. Full shapes are in
the [API contract](../openapi/openapi.yaml).
