# E-mail designs and templates

A design is the look of a mail: blocks such as headings, text, images and buttons, and a theme with
colours and fonts. You build it once in the panel, start from one of the templates that ship with
Yuva, and use it for your broadcasts. The server turns it into mail HTML that works in the common
mail apps, with a plain-text part and a dark mode.

Owners and admins make designs. API keys and OAuth tokens need the `broadcasts:read` or
`broadcasts:write` scope. Agents use only the designs offered in conversations
(see [section 8](#8-use-a-design-in-a-conversation)).

## 1. Start from a template

In the panel: Broadcasts, then Designs in the sidebar. Designs has two tabs: Mine, your own designs,
and Gallery, the templates. The gallery offers six starters: Newsletter, Product update, Announcement,
Welcome, Event and Digest, each in English, Turkish and German. The chips above it (All, Newsletter,
Product, Announcement, Onboarding, Event, Digest) show one kind at a time, and the Template language
switch picks the language; a template without text in your language shows "Text in English".

Pick a template card, or choose New design (shortcut `C`), to open the New design dialog. It shows
Empty page and the templates as tiles. Pick one, give the design a Name, pick the Inbox (or Whole
workspace) and choose Create and edit. The images of a template are copied into your workspace's
images, and you can change everything afterwards. Empty page starts from an empty mail with simple
colours.

On Mine, search designs with `/`. Show archived designs (`A`) adds archived designs to the list, marked
Archived; Restore on one brings it back. Mine and Gallery are `1` and `2`.

A design belongs to an inbox or to the whole workspace. The inbox's [sender identity](lists.md#1-set-the-sender-identity)
fills the footer.

Over the API: `POST /v1/email-designs` with `name` and `template: { "slug": "newsletter", "locale": "tr" }`.
`GET /v1/email-templates` lists the templates.

## 2. Edit the design

The designer opens full screen. Drag blocks from the palette (or the Layers outline), or press
Enter on one to add it after the selected block. Click a block to change it under Block settings on the right.

| Block | Use |
|---|---|
| Section | A group of blocks on its own background. Sections do not nest. |
| Columns | Two or three columns that stack on phones. Column widths add up to 100. Columns do not nest. |
| Heading | Three sizes. |
| Text | Paragraphs, lists and links. |
| Image | An uploaded picture, with an optional link and alt text. |
| Button | A link that looks like a button. |
| Divider, Spacer | A thin line, empty room. |
| Social | Links to your profiles, as icons or text. |
| Footer text | Your own lines above the required footer. |
| Message | Where the text a member writes goes when the design is used in a conversation ([section 8](#8-use-a-design-in-a-conversation)). At most one, never in columns. |

Headings, text, footer text and button labels are edited in place. Text keeps only what mail apps
handle well: bold, italic, underline, strikethrough, links, lists, line breaks and code. Links
start with `https://`, `http://`, `mailto:` or `tel:`.

Every mail ends with a footer you cannot remove: the inbox's legal name and postal address, why the
person gets the mail, and the unsubscribe and preference links. A warning says when the inbox has no
legal name or postal address yet.

A design holds up to 200 blocks.

### Merge fields

Merge fields put each person's own details into the mail. Insert one from the Insert menu in a text,
or type it. Add a fallback after `|` for people whose value is empty:

```
Hi {{ contact.first_name | "there" }},
```

| Field | Value |
|---|---|
| `contact.name` | The contact's name. |
| `contact.first_name` | The first word of the name. |
| `contact.email` | The e-mail address. |
| `contact.attributes.<key>` | An attribute of the contact, by its key. |
| `inbox.name` | The inbox's name. |
| `list.name` | The list the mail goes to. |
| `unsubscribe_url`, `preferences_url` | The person's own links. In links, merge fields may appear only after the `?`. |

A field Yuva does not know is refused. A fallback is at most 100 characters. Values never turn into
markup. Without a fallback the field can leave a gap such as "Hi ,", so the designer warns you.

## 3. Theme and dark mode

Open Design settings (shortcut `T`) for the inbox, preview text, colours, fonts, content width (480 to
720 pixels), corner radius and direction (left to right, right to left, or automatic per text). Fonts are system stacks: system,
humanist, geometric, serif and monospace; mails load no web fonts.

Readers whose mail app is in dark mode get darker colours made from yours, with text kept at
4.5:1 contrast. Under Dark mode you can set any of those colours yourself. An image of a dark logo on
a transparent background can get a light plate behind it in dark mode.

## 4. Images

Open the image library from an image block (shortcut `I`), or drop files on it.

| | |
|---|---|
| Types | PNG, JPEG, GIF, WebP. SVG is refused because it can carry scripts. |
| Upload size | At most 10 MiB; 40 megapixels and 8,000 pixels on a side. |
| Stored as | Decoded and encoded again without EXIF, GPS or colour-profile data. Wider than 1,200 pixels it is scaled down to 1,200. WebP is stored as PNG, or JPEG when it has no transparency. |
| GIF | Kept as it is, animation included: at most 1,200 pixels wide and 5 MiB. |
| Same file twice | Answers with the image you already have. |
| Address | Every image has a public `/i/{token}` URL on your server: no cookies, cached for a year, nothing recorded per request. |

Give every image a description for screen readers, or mark it decorative. Together the images of a
workspace may take `YUVA_EMAIL_IMAGES_MAX_BYTES` ([Configuration](configuration.md)).

An image used by a design that is not archived cannot be deleted; the panel names the designs. An
image that went out in a broadcast asks for confirmation, because mail already delivered then shows
a broken image. Images that no design uses and no broadcast sent are deleted 7 days after upload.

An [AI assistant](mcp.md#lists-designs-and-setup-from-an-assistant) can add an image with `upload_email_image`, from a public `https` URL or as base64 (about 5 MB through MCP), and read the designs with `list_designs` and `get_design`.

## 5. Previews and checks

The preview comes from the server, so it is what recipients get. Switch between Edit and Preview at the
top; in Preview choose Desktop or Phone, and turn Dark on or off. The keys are `1` Edit, `2` Desktop,
`3` Phone and `4` Dark.

The Checks button shows how many problems there are, in green, amber or red. It opens the Checks strip
(shortcut `C`) above the design: a meter of the mail's size against 102 KB, and one line for each
problem, marked Fix or Check, with a Show button that selects the block. "No problems found." means
the list is empty. This is what it flags:

| Warning | Meaning |
|---|---|
| Hard to read | Text under 4.5:1 contrast against its background, in light or dark colours. |
| Over 90 KB | Gmail cuts a mail off at about 102 KB of HTML and hides what follows, the unsubscribe link included. The designer warns at 90 KB and again over 102 KB. |
| Image missing, no description | An image block without an image, or without alt text. |
| No fallback | A merge field that can be empty for some people. |
| Sender details missing | The inbox has no legal name or postal address. |

A design over 500 KB of HTML is refused.

## 6. Saving, versions and conflicts

The designer saves 1.5 seconds after your last change; `Ctrl`/`Cmd` + `S` saves at once and keeps a
version. Undo and redo (`Ctrl`/`Cmd` + `Z`, with `Shift` for redo) go back 100 steps. Changes stay on
your device while you are offline and are saved when you are back.

The Versions tab on the right (shortcut `V`) keeps the newest 50: one at most every 10 minutes while you edit, one for each
manual save and one for each restore. Restoring saves that version as the newest one.

If someone else (or another tab of yours) saved first, a dialog offers three ways out: keep mine,
save mine as a copy, or load theirs. Your edit is never dropped, and the other version stays in the
versions. Whatever you pick, the other side's version is kept there as "Kept from a conflict", so no
edit is lost.

Download JSON, Make a copy and Archive are in the More actions menu. Archive hides a design from the
list and keeps its versions; Restore on the archived design brings it back. A document made by a newer Yuva can be viewed but not changed.

Shortcuts: `A` add a block, `T` design settings, `I` image, `V` versions, `C` checks, `Ctrl`/`Cmd` + `D` duplicate the
block, `Backspace` delete it, `Alt` + `↑`/`↓` move it, `Esc` leaves the text and then the block.

## 7. The API

Everything the panel does is in the [API contract](../openapi/openapi.yaml). This renders a design
without saving it:

```sh
curl -X POST https://support.example.com/v1/email-designs/render \
  -H "Authorization: Bearer $YUVA_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "inbox_id": "INBOX-ID",
    "document": {
      "schema": 1,
      "theme": {
        "width": 600,
        "font": "system",
        "colors": {
          "page": "#f4f4f5", "card": "#ffffff", "text": "#18181b", "muted": "#52525b",
          "accent": "#1d4ed8", "link": "#1d4ed8", "border": "#e4e4e7"
        }
      },
      "blocks": [
        { "id": "h", "type": "heading", "level": 1, "text": "Hello {{ contact.first_name | \"there\" }}" },
        { "id": "b", "type": "button", "label": "Read more", "url": "https://example.com/news" }
      ]
    }
  }'
```

The answer has `html`, `text`, `size_bytes` and `warnings`. Pass `"dark": true` for the dark palette.
A save sends the `version` it started from as `base_version`; if the design moved on, the answer is
`409 version_conflict` with the current design in `design`. An invalid document answers
`422 invalid_design` with one entry per problem in `errors`.

## 8. Use a design in a conversation

A reply or a new mail from an inbox can go out in one of your designs instead of as plain text. The
design needs exactly one Message block, at the top level or inside a section (not in columns). The
block holds no text of its own; the text a member writes is set into it, followed by the help article
cards and the member's signature. A design with a Message block cannot be sent as a broadcast: its
review and test sends refuse it.

An owner or admin makes the design available by offering it in conversations (`for_conversations`
on `PUT /v1/email-designs/{designId}`). Every member who sees the design's inbox, and keys and tokens
with `conversations:read`, can then list the offered designs (`GET /v1/email-designs?for_conversations=true`)
and read them; editing stays with owners, admins and `broadcasts:write`.

In the panel, the composer of an e-mailed conversation and of new mail has a design picker. Pick a
design and the message is sent in it. Over the API, `design_id` (and optionally `design_locale`)
goes on `POST /v1/conversations/{id}/messages` (a reply, or the first message of a conversation you
opened) and on `POST /v1/conversations/mail`; `PATCH /v1/messages/{id}` changes the design of a draft.

How it works:

- The contact's language version of the design is used, chosen as for broadcasts; `design_locale`
  names another one. The text is escaped into the Message block, and a blank line starts a paragraph.
- The text part of the mail is the whole mail as text. The conversation still shows what was written,
  not the design; `GET /v1/messages/{id}/email` returns the mail as sent.
- Merge fields are the contact's. `list.name`, `unsubscribe_url` and `preferences_url` are refused, and a
  field without a fallback that is empty for the contact refuses the message (`422 invalid_design`).
- Headers, recipients, threading and the SMTP account are those of a plain reply.
- Designs apply to e-mailed messages only: not notes, chat or app conversations (`400 design_not_emailed`).
- A design that is archived or no longer offered answers `409 design_unavailable`.
- A mail over 500 KB of HTML answers `422 html_too_large`.
- New mail with a design goes to one `to` address and no `cc` (`400 design_one_recipient`).

Preview what a contact would get with `POST /v1/conversations/{id}/design-preview`: the contact's
version and merge values, your signature, the footer and your text (or the design's placeholder). It
returns `html`, `text`, `subject` and `footer`, stores and sends nothing, and shares the 120 renders a
minute with `POST /v1/email-designs/render`.

### Footer: reply or outreach

| Footer | When | Contains |
|---|---|---|
| `reply` | The contact has written in the conversation. | The inbox's sender identity line. |
| `outreach` | The contact has not written: the first message of a new conversation or new mail. | The identity line, an unsubscribe link, and the `List-Unsubscribe` header with one-click. |

The unsubscribe link and one-click stop outreach in designs from that inbox to the contact; ordinary
replies are not affected. Outreach with a design is refused when:

| Refusal | Reason |
|---|---|
| `409 email_opted_out` | The address has a suppression of any scope, a workspace opt-out, or stopped outreach from the inbox. |
| `409 sender_identity_missing` | The inbox's [sender identity](lists.md#1-set-the-sender-identity) has no legal name or postal address. |
| `429 rate_limited` | More than 20 an hour per member or key, or 200 a day (UTC) per workspace. |
| `403 broadcasts_approval_required`, `403 broadcasts_suspended` | The server requires approval for broadcasts and your workspace has not been allowed, or was suspended. |

Replies with a design have no extra limit, and outreach without a design follows the rules of a plain
message. Designs are never used for notes, chat or app conversations, forwards, rule replies,
greetings or rating requests.

## Reference

### Endpoints

| Endpoint | Use |
|---|---|
| `GET`, `POST /v1/email-designs` | List (`inbox_id`, `q`, `archived=true`, `for_conversations`) and create from a `document`, a `template` or nothing. |
| `GET`, `PUT`, `DELETE /v1/email-designs/{designId}` | Read, save (with `base_version`), archive. |
| `POST /v1/email-designs/{designId}/duplicate` | Copy a design. |
| `POST /v1/email-designs/render` | Render a document: HTML, text, size and warnings. At most 120 a minute. |
| `POST /v1/conversations/{id}/design-preview` | Preview a message in a design offered in conversations. Scope `conversations:read`; shares the 120 a minute. |
| `GET /v1/email-designs/{designId}/versions` | The kept versions. |
| `GET /v1/email-designs/{designId}/versions/{version}` | One version with its document. |
| `POST /v1/email-designs/{designId}/versions/{version}/restore` | Save a version as the newest. |
| `GET`, `POST /v1/email-images` | List and upload images (multipart, one `file`). |
| `DELETE /v1/email-images/{imageId}` | Delete an image (`force=true` for one that was sent). |
| `GET /v1/email-templates`, `GET /v1/email-templates/{slug}` | The gallery (`locale=en`, `tr` or `de`). |
| `GET /i/{token}` | An image, public. |
| `GET /email-templates/{slug}/{locale}.html`, `.webp` | A template's preview, public. |

### Scopes

| Scope | Meaning |
|---|---|
| `broadcasts:read` | Read designs, versions, images and templates; render. |
| `conversations:read` | List, read and preview the designs offered in conversations. |
| `broadcasts:write` | Create, save, copy, archive and restore designs; upload and delete images. |

A key limited to inboxes reads the designs of its inboxes and the workspace-wide ones, and changes
only designs of its inboxes. Keys made before these scopes existed do not get them.

### Limits

| Limit | Value |
|---|---|
| Blocks in a design | 200 |
| Design document | 300 KiB of JSON |
| Rendered HTML | 90 KB warning, 102 KB clipped, 500 KB refused |
| Versions kept | 50 |
| Image upload | 10 MiB, 40 megapixels, 8,000 pixels a side |
| Image width after upload | 1,200 pixels |
| Merge field fallback | 100 characters |
| Link | 2,048 characters |
| Renders | 120 a minute per caller |
| Messages with a design to people who have not written | 20 an hour per member or key, 200 a day per workspace |
| Unused images | Deleted 7 days after upload |
