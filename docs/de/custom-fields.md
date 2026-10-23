# Benutzerdefinierte Felder

Benutzerdefinierte Felder sind typisierte Werte, die Sie einmal definieren und an jeder Unterhaltung
oder jedem Kontakt halten: eine Bestellnummer, ein Tarif, ein Produktbereich. Sie erscheinen in der
API, in Webhooks und in MCP, und ein Unterhaltungsfeld kann zur Pflicht werden, bevor eine
Unterhaltung geschlossen wird.

## 1. Ein Feld definieren

Felder definieren Inhaber und Admins oder ein Schlüssel mit dem Scope `fields:manage`:

```sh
curl -X POST https://support.example.com/v1/custom-fields \
  -H "Authorization: Bearer $YUVA_API_KEY" -H "Content-Type: application/json" \
  -d '{"object": "conversation", "key": "product_area", "name": "Product area",
       "type": "select", "options": ["Billing", "Login", "Other"], "required_on_close": true}'
```

Die Antwort ist das Feld, `201`. `object`, `key` und `type` lassen sich danach nicht mehr ändern.

| Typ | Wert |
|---|---|
| `text` | Eine Zeichenkette von 1 bis 1000 Zeichen. |
| `number` | Eine endliche JSON-Zahl. |
| `date` | Eine Zeichenkette `YYYY-MM-DD`. |
| `select` | Einer der `options` des Felds. |
| `checkbox` | `true` oder `false`. |

## 2. Werte an einer Unterhaltung setzen

`PATCH /v1/conversations/{conversationId}` und `POST /v1/conversations/bulk` nehmen `fields`, ein
Objekt mit Werten nach Schlüssel. Die genannten Schlüssel werden gesetzt, die anderen bleiben, und
`null` leert einen:

```sh
curl -X PATCH https://support.example.com/v1/conversations/$CONVERSATION \
  -H "Authorization: Bearer $YUVA_API_KEY" -H "Content-Type: application/json" \
  -d '{"fields": {"product_area": "Billing", "order_id": null}}'
```

Jeder Schlüssel muss ein Unterhaltungsfeld sein und jeder Wert zu seinem Typ passen, sonst antwortet
der Aufruf mit `400 field_invalid`, und `field` nennt den Schlüssel. Eine Sammelanfrage wird als
Ganzes geprüft, bevor sich etwas ändert. Eine Unterhaltung lesen, Unterhaltungen auflisten, Webhooks
und der Ereignis-Feed zeigen `fields` (`{}`, wenn keines gesetzt ist). Ändert sich nur `fields`, wird
`conversation.updated` gesendet. Der Schlüssel braucht `conversations:write`.

Die Liste filtern Sie mit `field.<key>=<value>`:

```sh
curl -G https://support.example.com/v1/conversations \
  -H "Authorization: Bearer $YUVA_API_KEY" \
  --data-urlencode "field.product_area=Billing" --data-urlencode "field.seats=5"
```

Eine Zahl wird als Zahl verglichen, ein Kontrollkästchen nimmt `true` oder `false`, alles andere muss
dem Text genau entsprechen. Mehrere Schlüssel müssen alle passen. Ein Schlüssel, der kein
Unterhaltungsfeld ist, ein wiederholter Schlüssel oder ein Wert, der nicht zum Typ passt, antwortet
mit `400 field_invalid`.

## 3. Kontakte

Ein Kontakt hält seine Werte in `attributes`. Ein Schlüssel mit einem Kontaktfeld wird gegen das Feld
geprüft; alle anderen Schlüssel bleiben frei.

| Wo | Ein Wert, der nicht passt |
|---|---|
| `/v1` (Kontakt anlegen oder aktualisieren, Abonnement hinzufügen) | `400 field_invalid`, `field` nennt den Schlüssel. `null` ist erlaubt und lässt den Wert ungesetzt. |
| [Identitäts-Token](identity.md) `attrs` | Wird verworfen; der Rest des Tokens wird verwendet. |
| Spalten eines [Listenimports](importing.md) | Der CSV-Text wird als Typ des Felds gelesen (`12`, `true`) und verworfen, wenn er nicht passt. |

## 4. Ein Feld zum Schließen verlangen

Mit `required_on_close` (nur Unterhaltungsfelder) antwortet das Schließen einer Unterhaltung ohne
Wert für das Feld mit `409 field_required`, und `field` nennt das erste fehlende nach `position`.
Werte, die in derselben Anfrage gesetzt werden, zählen. Das gilt für das Schließen mit `PATCH`, für
jede Unterhaltung einer Sammelaktualisierung (in `failed` mit `field` aufgeführt), für
[Makros](headless.md#makros) und das `then` einer Nachricht, für `set_status` und `bulk_update` in
[MCP](mcp.md) und für das Herausnehmen einer geschlossenen Unterhaltung aus dem Spam. Spam wird ohne
diese Prüfung geschlossen, und eine bereits geschlossene Unterhaltung wird nicht erneut geprüft. Ein
Kontrollkästchen gilt mit `true` oder `false` als gesetzt.

## 5. Ein Feld ändern oder löschen

`PATCH /v1/custom-fields/{customFieldId}` ändert `name`, `options` (die Liste wird ersetzt),
`required_on_close` und `position`. `DELETE` entfernt das Feld.

Ein Unterhaltungsfeld zu löschen oder eine Option eines `select` zu entfernen, leert diese Werte im
Hintergrund aus den Unterhaltungen, und jede geänderte Unterhaltung sendet `conversation.updated`.
Bis das Leeren fertig ist, antwortet das Anlegen eines Felds mit demselben Schlüssel mit
`409 field_clearing`; versuchen Sie es kurz darauf erneut. Den Schlüssel oder die Option neu zu
definieren, beendet das Leeren. Kontaktattribute bleiben erhalten, wenn ein Kontaktfeld gelöscht wird.

## Referenz

| Endpunkt | Zweck |
|---|---|
| `GET /v1/custom-fields` | Listet die Felder, optional `?object=conversation` oder `contact`. `{"items": [...]}`, sortiert nach Objekt, `position` und Erstellung. |
| `POST /v1/custom-fields` | Legt ein Feld an. |
| `GET`, `PATCH`, `DELETE /v1/custom-fields/{customFieldId}` | Liest, ändert oder löscht ein Feld. |

Ein Feld hat `id`, `object`, `key`, `name`, `type`, `options`, `required_on_close`, `position`,
`created_at` und `updated_at`.

| Punkt | Grenze |
|---|---|
| `key` | `^[a-z][a-z0-9_]{0,39}$`, pro Objekt eindeutig (`409 field_taken`). |
| `name` | 1 bis 100 Zeichen. |
| `options` | Nur `select`: 1 bis 100 verschiedene Zeichenketten mit 1 bis 100 Zeichen. |
| `position` | 0 bis 10000. |
| Felder pro Objekt | 50 (`403 field_limit`). |
| Schlüssel in einem `fields` | 50. |

| Scope | Erlaubt |
|---|---|
| `conversations:read` oder `contacts:read` | Felder auflisten und lesen. |
| `fields:manage` | Felder anlegen, ändern und löschen. Nur Inhaber und Admins; auf Posteingänge beschränkte Schlüssel können ihn nicht besitzen, und er ist in keinem anderen Scope enthalten: nennen Sie ihn ausdrücklich. Agents erhalten `403 forbidden`. |
| `conversations:write`, `contacts:write` | Werte schreiben. |
