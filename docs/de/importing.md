# Abonnenten importieren

Holen Sie Personen, die schon zugestimmt haben, von Ihnen zu hören, per CSV-Datei auf eine
[Mailingliste](lists.md). Yuva prüft jede Adresse, bevor etwas eingetragen wird, und hält fest, wo die
Personen zugestimmt haben.

Importieren können Owner und Admins. Alles auf dieser Seite geht auch über die [API](#aus-dem-backend).

## 1. Die Datei vorbereiten

Speichern Sie Ihre Tabelle oder Ihren Export als CSV, mit einer Kopfzeile.

| Grenze | Wert |
|---|---|
| Größe | 20 MiB |
| Zeilen | 100.000 Datenzeilen (ein Server kann ein niedrigeres Limit setzen, [`YUVA_IMPORT_MAX_ROWS`](configuration.md#broadcasts)) |
| Kodierung | UTF-8 (mit oder ohne BOM), UTF-16 mit BOM, Windows-1254 (türkisches Excel), Windows-1252 (westliches Excel) |
| Trennzeichen | Komma, Semikolon oder Tabulator, erkannt an der Kopfzeile |

Sie müssen nichts umwandeln: Eine türkische Excel-Datei mit Semikolons wird mit richtigen Buchstaben
importiert. Eine größere Datei müssen Sie teilen.

## 2. Hochladen

Im Panel: Broadcasts, dann Lists, die Liste öffnen, dann der Tab Import. Der Assistent hat vier Schritte:
File and consent, Columns, Review, Import. Sie können die Seite in jedem Schritt verlassen.

**File and consent.** Wählen Sie die Datei, dann:

- Setzen Sie den Haken bei der Erklärung, dass alle in der Datei zugestimmt haben, diese Liste von Ihrem
  Posteingang zu erhalten, und dass Sie zeigen können, wie und wann.
- Schreiben Sie, wo und wann sie zugestimmt haben, etwa „Newsletter-Kästchen auf der Bezahlseite unseres
  Shops, 2023 bis 2026“.

Beides ist Pflicht; ohne sie wird der Upload abgelehnt. Yuva speichert es beim Import und im
Einwilligungsprotokoll jedes Abonnements, das dabei entsteht, sodass es im
[Einwilligungsexport](lists.md) der Liste erscheint. Es ist Ihr Nachweis, und das, was Sie jemandem zeigen,
der fragt, woher Sie seine Adresse haben. Schreiben Sie die wahre Quelle: Wenn Sie nicht sicher sind, dass
die Personen zugestimmt haben, importieren Sie sie nicht.

**Ask everyone to confirm again.** Schalten Sie das ein, wenn die Einwilligung alt oder unsicher ist.
Niemand wird sofort Abonnent: Jede neue Adresse wird `pending` und erhält die Bestätigungsmail der Liste und
tritt erst nach einem Klick auf deren Link bei. Die Mails verteilen sich über Tage nach dem täglichen
Sendelimit des Workspace, eine große Datei geht also nicht auf einmal hinaus. Dafür braucht der Posteingang
einen E-Mail-Kanal, der senden kann.

Klicken Sie „Upload and check“. Die Datei wird gelesen und jede Adresse geprüft, noch wird nichts eingetragen.

## 3. Spalten

Yuva rät an der Kopfzeile, was jede Spalte enthält. Prüfen Sie es: Genau eine Spalte ist die E-Mail-Adresse;
die anderen können Name (oder Vor- und Nachname), Sprache, eine externe ID, ein von Ihnen benanntes Attribut
(Buchstaben, Ziffern, Punkte, Bindestriche und Unterstriche, höchstens 64) sein oder übersprungen werden.
Attribute füllen nur Schlüssel, die ein Kontakt noch nicht hat; ein Import überschreibt also nie, was Sie
schon wissen. Ein Attribut, das wie ein [benutzerdefiniertes Kontaktfeld](custom-fields.md) heißt, wird als Typ des Felds gelesen und weggelassen, wenn der Wert nicht passt. Ändern Sie eine Spalte und klicken Sie „Check again with these columns“.

## 4. Review

Vor dem Start sagt die Seite, was passiert: wie viele neue Abonnenten, wie viele neue Kontakte entstehen,
wie viele schon auf der Liste sind (bleiben unverändert) und was ausgelassen wird:

| Ausgelassen | Grund |
|---|---|
| Not a valid e-mail address | Die Adresse ist fehlerhaft. |
| The domain takes no mail | Die Domain hat keinen Mailserver (kein MX-, A- oder AAAA-Eintrag) oder veröffentlicht einen Null-MX. |
| Role addresses | Adressen wie `postmaster@`, `abuse@` oder `noreply@`, die keine Person erreichen. |
| Already earlier in the file | Eine Wiederholung. Die erste wird verwendet. |
| Bounced or reported as spam before | Die Adresse ist für alle Mail des Workspace unterdrückt. |
| Unsubscribed from everything or asked to be forgotten | Die Person hat allen Ihren Mails widersprochen. |
| Unsubscribed from this list | Die Person hat diese Liste verlassen. Nur sie selbst kann zurückkehren, über eine Bestätigung. |

Ein Import setzt niemanden zurück, der sich abgemeldet, widersprochen hat oder gebounct ist, egal was in
der Datei steht.

Klicken Sie „Import N addresses“, um zu starten.

## 5. Während er läuft

Die Seite zeigt erledigte Zeilen und Zähler live. Zeilen werden zu je 1.000 eingetragen. Sie können gehen;
der Import läuft weiter, und Imports im selben Tab listet frühere auf. „Cancel import“ stoppt ihn
umgehend: Was schon eingetragen ist, bleibt mit seinem Einwilligungsprotokoll, die hochgeladene Datei wird
gelöscht.

- Pro Workspace läuft **nur ein Import gleichzeitig**. Ein zweiter Upload wartet, bis der erste endet oder
  abgebrochen wird.
- **Es geht keine Willkommensmail** hinaus, und keine Bestätigung, außer Sie haben gewählt, alle erneut
  bestätigen zu lassen. Ein Import sendet keinen [Webhook](webhooks.md) und kein Ereignis pro Zeile.
- Am Ende wird die hochgeladene Datei gelöscht.

### Die ausgelassenen Zeilen

„Download the rows left out“ liefert eine CSV mit der Zeilennummer in Ihrer Datei, der Adresse wie
geschrieben und dem Grund. Die Datei bleibt 7 Tage nach dem Ende des Imports. Ein Import, den Sie
hochgeladen, aber nie gestartet haben, wird nach 7 Tagen abgebrochen.

| Grund | Bedeutung |
|---|---|
| `invalid_email` | Keine gültige E-Mail-Adresse. |
| `no_mail_domain` | Die Domain nimmt keine Mail an. |
| `role_address` | Eine Rollenadresse. |
| `duplicate` | Steht weiter oben in der Datei schon. |
| `suppressed` | Früher gebounct oder als Spam gemeldet. |
| `opted_out` | Von allem abgemeldet oder um Löschung gebeten. |
| `unsubscribed` | Von dieser Liste abgemeldet. |

Eine Datei, die kein Text ist, bekommt `415 not_csv`, eine über dem Größenlimit `413 file_too_large`. Eine
Datei über dem Zeilenlimit schlägt fehl, und nichts wird eingetragen.

## Aus dem Backend

Für ein paar Personen auf einmal oder Anmeldungen, wie sie passieren, nehmen Sie
[`POST /v1/lists/{listId}/subscriptions`](lists.md#5-abonnenten-aus-dem-backend-eintragen). Für eine Datei hat
der Import dieselben Schritte über die API. Der Schlüssel braucht `lists:write`.

```sh
curl -X POST https://support.example.com/v1/lists/LIST-ID/imports \
  -H "Authorization: Bearer $YUVA_API_KEY" \
  -F file=@subscribers.csv \
  -F 'attestation=Everyone in this file agreed to receive Newsletter from Example, and I can show how and when.' \
  -F 'consent_source=Newsletter checkbox on the checkout page, 2023 to 2026' \
  -F reconfirm=false
```

Die Antwort ist der Import im Zustand `validating`. Fragen Sie `GET /v1/lists/{listId}/imports/{importId}` ab,
bis `state` `ready` ist: `counts` und `columns` enthalten, was die Review zeigt, `preview` die ersten Zeilen.
Ändern Sie die Spalten mit `PATCH` (`{"columns": [{"index": 0, "field": "email"}, …]}`), dann starten Sie:

```sh
curl -X POST https://support.example.com/v1/lists/LIST-ID/imports/IMPORT-ID/start \
  -H "Authorization: Bearer $YUVA_API_KEY"
```

Fragen Sie weiter ab, bis `done`; `rows_done` wächst, und `/v1/realtime` sendet `list_import.progress` und
`list_import.updated`.

## Referenz

| Endpoint | Zweck |
|---|---|
| `GET`, `POST /v1/lists/{listId}/imports` | Imports auflisten; eine Datei hochladen (`file`, `attestation`, `consent_source`, `reconfirm`, `mapping`). |
| `GET`, `PATCH`, `DELETE /v1/lists/{listId}/imports/{importId}` | Lesen; Spalten ändern (nur solange `ready`); abbrechen. |
| `POST /v1/lists/{listId}/imports/{importId}/start` | Einen `ready`-Import starten. |
| `GET /v1/lists/{listId}/imports/{importId}/errors` | Die ausgelassenen Zeilen als CSV. |

Zustände: `validating`, `ready`, `importing`, `done`, `failed` (siehe `error`), `cancelled`. Fehler, die
Sie behandeln sollten: `422 attestation_required`, `409 import_running`, `409 email_channel_required`
(erneute Bestätigung ohne sendefähigen Kanal), `409 not_ready`, `422 email_column_required`, `413`, `415`.
Lesen braucht `lists:read`; Hochladen, Ändern, Starten und Abbrechen brauchen `lists:write`. Die genauen
Formen stehen im [API-Vertrag](../../openapi/openapi.yaml).
