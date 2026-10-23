# Custom fields

Custom fields are typed values you define once and keep on every conversation or contact: an order
number, a plan, a product area. They come back in the API, in webhooks and in MCP, and a conversation
field can be required before a conversation closes.

## 1. Define a field

Owners and admins define fields, or a key with the `fields:manage` scope:

```sh
curl -X POST https://support.example.com/v1/custom-fields \
  -H "Authorization: Bearer $YUVA_API_KEY" -H "Content-Type: application/json" \
  -d '{"object": "conversation", "key": "product_area", "name": "Product area",
       "type": "select", "options": ["Billing", "Login", "Other"], "required_on_close": true}'
```

The answer is the field, `201`. `object`, `key` and `type` cannot change afterwards.

| Type | Value |
|---|---|
| `text` | A string of 1 to 1000 characters. |
| `number` | A finite JSON number. |
| `date` | A `YYYY-MM-DD` string. |
| `select` | One of the field's `options`. |
| `checkbox` | `true` or `false`. |

## 2. Set values on a conversation

`PATCH /v1/conversations/{conversationId}` and `POST /v1/conversations/bulk` take `fields`, an object
of values by key. The keys you name are set, the others stay, and `null` clears one:

```sh
curl -X PATCH https://support.example.com/v1/conversations/$CONVERSATION \
  -H "Authorization: Bearer $YUVA_API_KEY" -H "Content-Type: application/json" \
  -d '{"fields": {"product_area": "Billing", "order_id": null}}'
```

Every key must be a conversation field and every value must fit its type, or the call answers
`400 field_invalid` and `field` names the key. A bulk request is checked as a whole before anything
changes. Reading a conversation, listing conversations, webhooks and the event feed show `fields`
(`{}` when none is set). Changing only `fields` sends `conversation.updated`. The key needs
`conversations:write`.

Filter the list with `field.<key>=<value>`:

```sh
curl -G https://support.example.com/v1/conversations \
  -H "Authorization: Bearer $YUVA_API_KEY" \
  --data-urlencode "field.product_area=Billing" --data-urlencode "field.seats=5"
```

A number is compared as a number, a checkbox takes `true` or `false`, everything else matches the
text exactly. Several keys must all match. A key that is not a conversation field, a repeated key or a
value that does not fit the type answers `400 field_invalid`.

## 3. Contacts

A contact keeps its values in `attributes`. A key that has a contact field is checked against the
field; every other key stays free-form.

| Where | A value that does not fit |
|---|---|
| `/v1` (create or update a contact, add a subscription) | `400 field_invalid`, `field` names the key. `null` is allowed and leaves it unset. |
| [Identity token](identity.md) `attrs` | Dropped; the rest of the token is used. |
| [List import](importing.md) columns | The CSV text is read as the field's type (`12`, `true`) and dropped when it does not fit. |

## 4. Require a field to close

With `required_on_close` (conversation fields only), moving a conversation to closed without a value
for the field answers `409 field_required`, and `field` names the first missing one by `position`.
Values set in the same request count. This holds for closing with `PATCH`, for each conversation of a
bulk update (listed in `failed` with `field`), for [macros](headless.md#macros) and a message's `then`,
for `set_status` and `bulk_update` in [MCP](mcp.md), and for taking a closed conversation out of
spam. Spam is closed without the check, and a conversation that is already closed is not checked
again. A checkbox counts as set with `true` or `false`.

## 5. Change or delete a field

`PATCH /v1/custom-fields/{customFieldId}` changes `name`, `options` (the list is replaced),
`required_on_close` and `position`. `DELETE` removes the field.

Deleting a conversation field, or removing an option from a `select`, clears those values from the
conversations in the background, and each changed conversation sends `conversation.updated`. Until the
clearing is done, creating a field with the same key answers `409 field_clearing`; try again shortly.
Defining the key or option again stops the clearing. Contact attributes are kept when a contact field
is deleted.

## Reference

| Endpoint | Use |
|---|---|
| `GET /v1/custom-fields` | List the fields, optionally `?object=conversation` or `contact`. `{"items": [...]}`, ordered by object, `position`, creation. |
| `POST /v1/custom-fields` | Create one. |
| `GET`, `PATCH`, `DELETE /v1/custom-fields/{customFieldId}` | Read, change or delete one. |

A field has `id`, `object`, `key`, `name`, `type`, `options`, `required_on_close`, `position`,
`created_at` and `updated_at`.

| Item | Limit |
|---|---|
| `key` | `^[a-z][a-z0-9_]{0,39}$`, unique per object (`409 field_taken`). |
| `name` | 1 to 100 characters. |
| `options` | `select` only: 1 to 100 distinct strings of 1 to 100 characters. |
| `position` | 0 to 10000. |
| Fields per object | 50 (`403 field_limit`). |
| Keys in one `fields` | 50. |

| Scope | Allows |
|---|---|
| `conversations:read` or `contacts:read` | List and read fields. |
| `fields:manage` | Create, change and delete fields. Owners and admins only; keys limited to inboxes cannot hold it, and it is not implied: name it. Agents get `403 forbidden`. |
| `conversations:write`, `contacts:write` | Write values. |
