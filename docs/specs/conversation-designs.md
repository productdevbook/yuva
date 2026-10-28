# Designed messages in conversations (#209)

Status: planned, milestone M13.10. Owner of the design: yuva-broadcasts. Builders: yuva-api
(`openapi/`, `api/`), yuva-panel (`web/`), yuva-widget (`sdk/js` schema), yuva-docs (`docs/`).

An outgoing e-mailed message in a conversation (a reply, the first message after
`start_conversation`, a new mail from the panel's Mail section) may name an e-mail design. The
design's version in the contact's language becomes the mail's HTML part, with the message text set
into the design where the design says; the plain-text part is the same mail as text. Threading,
recipients, Cc/Bcc, attachments, signatures and delivery states stay as they are. Without a design
nothing changes.

## 1. Decisions

1. **The body goes into a `message` block.** A new block type `message` marks where the text a
   member writes appears. A design used in conversations has exactly one, at the top level or inside
   a section (never inside columns). The block holds no text of its own and takes no translation
   entry. It renders the message body escaped, a blank line starting a new paragraph and a single
   line break a `<br>`, in the theme's text colour and font (16 px / 26 px), followed by the help
   article cards and then the signature (`-- ` and the signature HTML), so these sit inside the
   design rather than after the footer. A design without a `message` block cannot be offered in
   conversations; a broadcast's review blocks a design that has one (`message_block`), so broadcast
   mail never shows an empty slot.
2. **The text part is the whole mail as text, not the body alone.** The design's text rendering with
   the `message` block replaced by the body, the cards' lines and the signature text. The two
   `multipart/alternative` parts then carry the same content, as RFC 2046 means them to, and spam
   filters that score differing parts (SpamAssassin's `MPART_ALT_DIFF`) see none. `messages.body`
   stays exactly what was written: the panel thread, the widget, the client API, events and webhooks
   show the body, never the design.
3. **Rendered once, when the message is handed to the outbox.** Creating a message that is sent at
   once, `sendMessage` for a draft and the scheduled send each render the design's current version
   for the contact and store the result on the message's e-mail row (`full_html`, `full_text`, plus
   the design id, version and language). The send job sends those parts as they are; retries never
   render again. A draft only stores which design it uses; creating or changing it renders once to
   validate and discards the result.
4. **One version per contact,** chosen as broadcasts choose (exact tag, shorter tag, same language
   in another region, fallback; no locale → fallback) from `contacts.locale`; the caller may name a
   version (`design_locale`, one of the design's versions). The subject stays the conversation's
   (`Re: …` on replies, the written subject on new mail and on the first message); designs carry no
   subject. The preheader is the version's.
5. **Merge fields are the contact's.** `contact.*` and `inbox.name` resolve to the conversation's
   contact; `list.name`, `unsubscribe_url` and `preferences_url` do not exist in conversation mail
   and are refused (`422 invalid_design`, issue `merge_field_unavailable`). A field without fallback
   that is empty for this contact refuses the message (`merge_field_empty`), since there is no
   review step to warn in.
6. **Footer by kind of mail.** The renderer gets a footer mode:
   - `list` (today's, broadcasts and welcome mail): identity, why, unsubscribe and preferences.
   - `reply`: a designed message in a conversation where the contact has written at least one
     message. The inbox's identity line (legal name · postal address · registration · contact) when
     the inbox has a legal name, nothing else: no "why", no links, no `sender_identity_missing`
     warning, no list headers. This is one-to-one mail the person is part of.
   - `outreach`: a designed message in a conversation where the contact has not written (the first
     message after `start_conversation` or new mail). The identity line plus one "Unsubscribe" link
     (existing footer word, all 44 footer languages) and `List-Unsubscribe` with
     `List-Unsubscribe-Post: List-Unsubscribe=One-Click`. Google requires one-click only for
     marketing and promotional mail and excludes transactional mail
     (support.google.com/a/answer/14229414), but the FTC reads "transactional or relationship"
     narrowly and an existing customer relationship alone does not make a mail transactional
     (ftc.gov CAN-SPAM compliance guide), so unsolicited designed mail carries the address and an
     opt-out as commercial mail must.
7. **Outreach respects objections and has its own limits** (a design with a logo must not become a
   way around lists and consent):
   - refused for an address with any suppression (`scope` `all` or `broadcast`), an `email_optouts`
     row, or an `inbox_mail_optouts` row for this inbox and contact: `409 email_opted_out`. Plain
     outreach without a design keeps today's rule (only `scope` `all` stops it).
   - at most 20 outreach designed messages per member or key per hour and 200 per workspace per UTC
     day (`429 rate_limited`, counted when the mail is handed to the outbox).
   - on a server in `YUVA_BROADCASTS=approval` mode, only a workspace whose broadcasts are approved
     sends outreach with a design (`403 broadcasts_approval_required`, `403 broadcasts_suspended`).
     Replies with a design are never gated by broadcast approval.
   - its one-click unsubscribe writes `inbox_mail_optouts (workspace, inbox, contact)`: no more
     outreach designed mail and no triggered service mail (#205) from that inbox; lists are untouched
     (the `/m/` page also offers "unsubscribe from everything", which works as today). Replies still
     go: the person can always write and be answered.
8. **Headers and sending path unchanged otherwise.** `From`, `Reply-To`, `To`, `Cc`, envelope `Bcc`,
   `Message-ID` with the conversation token, `In-Reply-To`, `References`, the SES tenant headers and
   the provider's DKIM are exactly a reply's. Outreach adds only the two list-unsubscribe headers; no
   `List-Id`, no `Feedback-ID`, no `Precedence`. Mail goes through the conversation's e-mail channel
   and its support SMTP account (never `broadcast_smtp_*`, never `YUVA_SMTP_*`), so replies thread
   as today.
9. **Who may pick a design.** Owners and admins mark a design `for_conversations` (only with a
   `message` block). Every member who can see the design's inbox (workspace-wide designs: every
   member) can list those designs, read them and preview them for a conversation; keys and tokens
   need `conversations:read` for that and `messages:write` to send, nothing from `broadcasts:*`.
   Editing stays with owners, admins and `broadcasts:write`. Assistants may name a design wherever
   they may write the message.
10. **Where designs are refused.** Notes, chat and app conversations, chat continuity mail,
    forwards, automation rule replies, greetings and rating requests (`400 design_not_emailed` for a
    message that is not e-mailed); a message with both `html` and `design_id` (`400
    validation_failed`); new mail with more than one `to` or any `cc` (`400 design_one_recipient`,
    since merge fields would show the first person's details to the others).

## 2. Data model

One migration, the next free number when it is written (`00143_conversation_designs.sql` today).

```sql
ALTER TABLE email_designs ADD COLUMN for_conversations boolean NOT NULL DEFAULT false;
CREATE INDEX email_designs_conversations_idx ON email_designs (workspace_id, updated_at DESC, id DESC)
    WHERE for_conversations AND archived_at IS NULL;

ALTER TABLE messages
    ADD COLUMN email_design_id uuid,
    ADD COLUMN email_design_locale text CHECK (length(email_design_locale) BETWEEN 2 AND 35),
    ADD CONSTRAINT messages_email_design_fkey FOREIGN KEY (workspace_id, email_design_id)
        REFERENCES email_designs (workspace_id, id) ON DELETE SET NULL (email_design_id);

ALTER TABLE message_emails
    ADD COLUMN design_id uuid,
    ADD COLUMN design_version integer,
    ADD COLUMN design_locale text CHECK (length(design_locale) <= 35),
    ADD COLUMN outreach boolean NOT NULL DEFAULT false,
    ADD CONSTRAINT message_emails_design_fkey FOREIGN KEY (workspace_id, design_id)
        REFERENCES email_designs (workspace_id, id) ON DELETE SET NULL (design_id);

CREATE TABLE inbox_mail_optouts (
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    inbox_id     uuid NOT NULL,
    contact_id   uuid NOT NULL,
    source       text NOT NULL CHECK (source IN ('outreach', 'triggered')),
    created_at   timestamptz NOT NULL,
    PRIMARY KEY (workspace_id, inbox_id, contact_id),
    FOREIGN KEY (workspace_id, inbox_id) REFERENCES inboxes (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, contact_id) REFERENCES contacts (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX inbox_mail_optouts_contact_idx ON inbox_mail_optouts (workspace_id, contact_id);
```

- `messages.email_design_id` / `email_design_locale`: what the author asked for (drafts included);
  `email_design_locale` NULL means "the contact's".
- `message_emails.design_version IS NOT NULL` marks a row whose `full_html`/`full_text` are the
  complete mail as sent (signature and cards inside); `design_locale` is the version used, `''` for
  a design without versions; `design_id` is NULL after the design is gone, version and parts stay;
  `outreach` says the mail carries the outreach footer and list-unsubscribe headers.
- No new index on `messages`: the design is read with the message.
- Redaction: `RedactMessageEmail` already clears `full_text` and `full_html`; it also clears
  `design_locale` (the version can hint at the person's language). Trash, contact and workspace
  deletion follow the message rows as today.
- Deleting a contact that has an `inbox_mail_optouts` row leaves an `email_optouts` row (reason
  `erasure_after_optout`), as for an objecting list subscriber, so the objection survives erasure.
- An image used by a design counts as used while the design does; a sent designed message keeps
  its HTML pointing at `/i/{token}`, so the "went out in a broadcast" confirmation before deleting
  an image also counts images a designed message carried (look up `message_emails.design_id`).

## 3. Renderer (`api/internal/mail/design`)

- `Block.Type` `message`; `parseBlocks` and `Validate` accept it with only the base fields
  (`id`, `padding`, `background`, `align` ignored, `hide`). Issues: `message_block_repeated` (more
  than one), `nesting` (inside columns), `translation_block` (a translation entry for it).
- `Input.Message *MessageSlot{Text string; CardsHTML, CardsText string; SignatureHTML,
  SignatureText string}`. With `Message == nil` the block renders as a dashed placeholder "Your
  message appears here" in previews and as nothing in sent mail (broadcasts are blocked before
  sending anyway).
- `Input.Footer` `FooterList` (zero value, today's) | `FooterReply` | `FooterOutreach`;
  `FooterOutreach` needs `Values.UnsubscribeURL`.
- `Document.HasMessageBlock() (count int, nested bool)` for the API's checks.
- Merge-field checking takes the allowed set: `design.ConversationFields` (all but `list.name`,
  `unsubscribe_url`, `preferences_url`).
- Golden files: one `message` design per footer mode, light and dark, `ltr` and `rtl`, in
  `api/internal/mail/testdata`.

## 4. Contract (`openapi/openapi.yaml`)

Schemas:

- `EmailMessageBlock` (`type: message`, `EmailBlockBase` only), added to `EmailBlock.oneOf` and the
  discriminator; `EmailDesignDocument.description` and the root description's block list name it.
- `EmailDesignSummary.for_conversations: boolean` (required in responses); settable on
  `EmailDesignCreate` and `EmailDesignUpdate` (`422 invalid_design` with `message_block_missing` when
  set on a document without exactly one top-level or section `message` block, and on a save that
  removes the block while the flag is on).
- `MessageCreate`, `MessageUpdate` and `ComposeMail` gain
  `design_id` (uuid; `MessageUpdate`: `[string, "null"]`, `null` removes it) and `design_locale`
  (`LanguageTag`; `MessageUpdate`: nullable). Multipart forms take the same two fields.
- `Message.design` (`MessageDesign`, absent when none): `{id: uuid | absent when the design is
  gone, name, version: int (absent on a draft), locale: LanguageTag (absent for a design without
  versions or on a draft)}`.
- `MessageEmailDetail.original_html` (`GET /v1/messages/{id}/email`) returns the sent design HTML for
  a designed outgoing message, so the panel's existing "as sent" view shows it.
- `ConversationDesignPreview` request `{design_id, design_locale?, body?: string (≤ 65536)}`;
  response `EmailDesignRender` plus `subject`.

Operations:

| Method and path | Change | Who, scopes | Errors |
|---|---|---|---|
| `GET /v1/email-designs` | `for_conversations=true` filter; with it, any member and `conversations:read` keys list the designs of inboxes they see and the workspace-wide ones (summaries only) | members, keys | as today |
| `GET /v1/email-designs/{id}` | a `for_conversations` design is readable by the same callers | members, keys | `404 not_found` |
| `POST /v1/conversations/{id}/design-preview` | new: renders what the contact would get, with the caller's signature, the footer mode this message would have and the body given (sample text without one); 120 a minute per caller, shared with `renderEmailDesign` | members who see the conversation; `conversations:read` | `404`, `400 design_not_emailed`, `409 design_unavailable` (archived or not offered), `422 invalid_design` (`merge_field_unavailable`, `merge_field_empty`, `message_block_missing`), `422 html_too_large`, `429` |
| `POST /v1/conversations/{id}/messages` | `design_id`, `design_locale` | as today | the above plus `400 design_locale_unknown`, `400 validation_failed` (`html` with a design), `409 email_opted_out`, `403 broadcasts_approval_required`, `403 broadcasts_suspended`, `429 rate_limited` |
| `PATCH /v1/messages/{id}` | draft's design | as today | same validation as create |
| `POST /v1/messages/{id}/send` | renders the draft's design | as today | same; plus `409 design_unavailable` |
| `POST /v1/conversations/mail` | `design_id`, `design_locale` | as today | plus `400 design_one_recipient` |

A scheduled draft whose design cannot be rendered when its time comes fails like any scheduled send
(`send_error` with the code). `message.created` and `message.updated` carry `design` (id, name,
version, locale) and never the HTML.

## 5. API (`api/`)

- `messageInput` gains `designID`, `designLocale`; `validateMessage` refuses them on notes and with
  `html`.
- In `CreateMessage` (and `sendMessage`, the scheduled-send worker and `composeMail`), after
  `planEmail` and `signMessage`, when a design is named:
  1. load it with a new `usableDesign(ctx, q, p, id, inboxID)`: not archived, `for_conversations`,
     its inbox the conversation's or none, visible to the principal under rule 9;
  2. decide outreach: `NOT EXISTS` a message of direction `in` from the contact in the conversation;
     for outreach run the checks of decision 7 (suppression of either scope, `email_optouts` by
     HMAC, `inbox_mail_optouts`, approval mode) and take the two limits
     (`designed_outreach:<actor>` 20/h, `designed_outreach_ws:<workspace>` 200/UTC day) through
     `s.limits.takeAll`;
  3. resolve the version (`design.Resolve` against `contacts.locale`, or `design_locale` checked
     against `Document.Versions()`);
  4. render with `renderDesign`, extended with `message`, `footer` and `values` from
     `contactValues`; for outreach the unsubscribe URL is a manage token sealing workspace, inbox,
     contact, conversation and purpose `outreach`;
  5. write `full_html`, `full_text`, `design_id`, `design_version`, `design_locale`, `outreach` in
     `recordOutboundEmail` (and set `messages.email_design_id` / `_locale`).
- `sendQueued`: when `me.DesignVersion != nil` use `FullText`/`FullHtml` as they are, skip
  `batchSignature`, `batchCards` and `TextToHTML`; when `me.Outreach`, seal a fresh outreach manage
  token for the header (tokens never expire, so the footer's and the header's both work) and add
  `List-Unsubscribe` and `List-Unsubscribe-Post`. Nothing about the token is stored, so members
  never see it in `GET /v1/messages/{id}/email`.
- `/m/{token}` with purpose `outreach`: the page names the inbox and offers "Stop mail like this from
  <inbox>" (writes `inbox_mail_optouts`, source `outreach`) and "Unsubscribe from everything" (today's);
  the one-click POST does the first. Opening changes nothing.
- `designBody`/summaries: `for_conversations`; `ListEmailDesigns` and `visibleDesign` relax
  `requireManagerOrKey` only for `for_conversations` designs and only for reading.
- MCP (`mcp_contacts.go`, `mcp_tools.go`, `mcp_designs.go`):
  - `start_conversation`, `send_reply`, `compose_mail`: optional `design_id` ("an e-mail design
    offered in conversations, from list_designs with for_conversations; the body is set into its
    message block, in the contact's language") and `design_locale`.
  - `list_designs`: `for_conversations` filter; listed for every caller with `conversations:read`.
  - new `preview_message_design` (read-only; maps to `POST /v1/conversations/{id}/design-preview`):
    returns `text`, `size_bytes`, `warnings`, `locale` and the subject, and the HTML only when
    `include_html` is true.
  - `mcpMessage` gains `design {id, name, version, locale}`.
- Audit: `email_design.updated` records `for_conversations` changes.

## 6. Panel (`web/`)

- **Designer:** palette item "Message" (`message` block, icon `MessageSquareText`), shown as a
  dashed slot with "The message you write appears here"; at most one (the palette item disables
  itself, with a tooltip); not offered inside columns. Design settings get "Offer in conversations"
  (switch, owners and admins), disabled with a reason until the design has a message block. The
  Checks strip shows `message_block` as Check when the design is offered in conversations but
  a broadcast uses it.
- **Composer (`ReplyBox`, `features/mail/Compose.tsx`):** for e-mailed replies and new mail, a
  "Design" button next to Help article (shortcut `D`) opens a picker of offered designs for the
  inbox: search, name, a small preview thumbnail (rendered on demand), "No design". Chosen, a chip
  above the text shows the design name and the language version the contact gets ("Turkish" or
  "Fallback: English"), with a menu to pick another version and "Remove". "Preview" opens a sheet
  with the server's `design-preview` at desktop, phone and dark, refreshed 800 ms after typing
  stops. States: loading (skeleton), empty ("No designs are offered in conversations yet. Owners
  and admins offer one in the designer." with a link for them), error (the server's message, send
  disabled), outreach notice when the contact has not written ("This first mail carries an
  unsubscribe link"). The signature preview moves into the chip's preview, since the signature sits
  inside the design.
- **Thread:** a designed outgoing message shows its body as today plus a small "Design: <name> ·
  <language>" line; "As sent" shows the stored HTML through the existing `EmailHtml` view.
- Drafts keep the chosen design (local draft store and server drafts); autosave stays on
  (`plainText` no longer turns off for a design).
- Lingui strings in `en`, with the `tr` and `de` catalogs and the other panel catalogs extracted as
  usual.

## 7. SDK and docs

- `sdk/js`: regenerate `src/schema.gen.ts` (`EmailBlock`, `Message.design`); no widget UI change.
  Swift and Kotlin SDKs: nothing (they do not carry design or member message schemas).
- `docs/designs.md`: the Message block, "Offer in conversations", the conversation footer modes;
  `docs/email.md` (Writing new mail, Outbound): designed messages, outreach rules and limits;
  `docs/mcp.md`: the new parameters and `preview_message_design`. Each with `docs/tr/` and
  `docs/de/` in the same commit.

## 8. Order of work and acceptance

1. **yuva-api — contract:** schemas and operations of section 4; Redocly lint clean; regenerate
   `api/internal/oas`. Accept: lint passes; `git diff` shows only the listed schemas and operations.
2. **yuva-api — renderer:** section 3 with golden files. Accept: `go test ./internal/mail/...`
   passes; the three footer modes render; a document with two message blocks or one in columns is
   refused; a broadcast review of a design with a message block is blocking.
3. **yuva-api — server:** migration, section 5, MCP. Accept (tests, Postgres): a reply with a design
   to a contact with `locale` `es-MX` in a design with `es` sends the `es` version, Mailpit shows the
   HTML with the body inside the design and the text part with the body and the design's texts,
   `In-Reply-To`/`References` equal a plain reply's; outreach to a broadcast-suppressed address
   answers `409 email_opted_out`, a plain outreach still goes; the 21st outreach designed message in
   an hour answers `429`; a draft with a design sent later uses the design's version at send time;
   redaction clears the stored parts; an agent lists only offered designs of their inboxes;
   `start_conversation` with `design_id` from an assistant stores a draft with the design;
   the outreach one-click writes `inbox_mail_optouts` and a second designed outreach is refused.
   `go vet`, `go test`, govulncheck and the generated-code diff pass.
4. **yuva-widget — schema:** regenerate `sdk/js`. Accept: `bun run check && bun run build` passes.
5. **yuva-panel — UI:** section 6. Accept: the `web/` check passes; device-tester screenshots
   (WebP): the designer with a Message block, the composer with a chosen design and its preview in
   desktop, phone and dark, the empty picker for an agent, the thread line and "As sent".
6. **yuva-docs:** section 7. Accept: links resolve; every documented endpoint exists in `openapi/`.
7. **Verification:** in Mailpit the designed reply passes Mailpit's html-check with no error-level
   finding, renders in WebKit and Chromium at desktop, phone and dark like the preview; a reply to
   it from Mailpit's sender threads into the same conversation.

Out of scope: designs in automation rule replies, greetings and rating requests; designs for chat
or app conversations; a gallery template made for conversations; per-message subjects in designs.

## 9. Proposed text for `docs/architecture.md` and `docs/roadmap.md`

### `docs/architecture.md`, in "Broadcasts and e-mail designs (M13)", after the "Languages (M13.9)" bullet

```markdown
- **Designs in conversations (M13.10).** A design with exactly one `message` block (top level or in
  a section, never in columns; no text, no translation entry) can be offered in conversations
  (`email_designs.for_conversations`, set by owners and admins). An e-mailed outgoing message (a
  reply, the first message after `start_conversation`, new mail with one `to` and no `cc`) may name
  one (`design_id`, optional `design_locale`): the contact's version, chosen as for broadcasts, is
  rendered when the message is handed to the outbox, with the body escaped into the `message` block
  followed by the help cards and the signature, and stored on `message_emails` (`full_html`,
  `full_text`, `design_id`, `design_version`, `design_locale`); the send job sends those parts
  unchanged. The text part is the whole mail as text, so both alternative parts carry the same
  content; `messages.body` stays what was written and is all the panel thread, widget, events and
  webhooks show. Merge fields are the contact's; `list.name`, `unsubscribe_url` and
  `preferences_url` are refused, and a field without fallback that is empty refuses the message.
  Headers, recipients, threading, the SMTP account (the channel's support account) and the
  provider's DKIM are a reply's. The footer has three modes: `list` (broadcasts), `reply` (the
  contact has written in the conversation: the identity line only) and `outreach` (the contact has
  not: the identity line, an unsubscribe link and `List-Unsubscribe` with one-click, whose POST
  writes `inbox_mail_optouts` for the inbox and contact). Outreach with a design is refused for an
  address with a suppression of either scope or an opt-out (`409 email_opted_out`), limited to 20
  per member or key an hour and 200 per workspace a UTC day, and needs approved broadcasts on a
  server in `approval` mode; replies with a design have no extra gate. Members who see the
  design's inbox, and keys with `conversations:read`, list, read and preview offered designs
  (`POST /v1/conversations/{id}/design-preview`); editing stays with owners, admins and
  `broadcasts:write`. A broadcast's review blocks a design with a `message` block. Designs never go
  into notes, chat or app conversations, continuity mail, forwards, rule replies, greetings or
  rating requests.
```

### `docs/roadmap.md`, in "M13 — Broadcasts and e-mail designs"

Replace the Status line's end "M13.9 (#203) in progress." with "M13.9 (#203) in progress; M13.10
(#209) planned, spec `docs/specs/conversation-designs.md`." and add after the M13.9 bullet:

```markdown
- **M13.10 Designs in conversations:** a `message` block that holds the text a member writes;
  designs offered in conversations; a reply, the first message of `start_conversation` and new mail
  sent with a design in the contact's language, body and signature inside it, the text part the
  same mail as text, threading unchanged; footers for replies (identity only) and outreach
  (identity and one-click unsubscribe), outreach limited and refused to people who objected; the
  composer's design picker and preview; API and MCP.
  Accept: a reply with a design to a contact in `es-MX` sends the Spanish version through the
  conversation's channel, Mailpit shows the body inside the design in HTML and text, and the
  contact's answer threads into the same conversation; a first message with a design carries
  one-click unsubscribe and is refused to an address that unsubscribed from broadcasts; the
  designer, the composer and the as-sent view match the preview in WebKit and Chromium at desktop,
  phone and dark.
```

## 10. Issue body for #209 (to replace or follow the report)

```markdown
Plan: `docs/specs/conversation-designs.md` (M13.10). Decisions in `architecture.md` › Broadcasts and
e-mail designs › Designs in conversations.

Steps (in order):
1. yuva-api: contract (`EmailMessageBlock`, `for_conversations`, `design_id`/`design_locale` on
   `MessageCreate`, `MessageUpdate`, `ComposeMail`, `Message.design`, `POST /v1/conversations/{id}/design-preview`).
2. yuva-api: renderer (`message` block, footer modes `list|reply|outreach`) with golden files.
3. yuva-api: migration, sending, outreach rules and limits, `/m/` outreach page, MCP parameters and
   `preview_message_design`.
4. yuva-widget: regenerate the `sdk/js` schema.
5. yuva-panel: Message block in the designer, "Offer in conversations", the composer's design picker
   and preview, the thread line and "As sent".
6. yuva-docs: designs, e-mail and MCP guides with tr and de.

Acceptance: section 8 of the spec.
```
