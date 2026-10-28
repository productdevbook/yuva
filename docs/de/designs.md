# E-Mail-Designs und Vorlagen

Ein Design ist das Aussehen einer Mail: Blöcke wie Überschriften, Text, Bilder und Buttons sowie ein
Theme mit Farben und Schriften. Sie bauen es einmal im Panel, starten mit einer der Vorlagen, die
Yuva mitbringt, und nutzen es für Ihre Broadcasts. Der Server macht daraus Mail-HTML, das in den
gängigen Mail-Apps funktioniert, mit Nur-Text-Teil und Dark Mode.

Designs erstellen Owner und Admins. API-Keys und OAuth-Tokens brauchen den Scope `broadcasts:read`
oder `broadcasts:write`. Assistenten (Agents) verwenden nur
Designs, die in Unterhaltungen angeboten werden ([Abschnitt 8](#8-ein-design-in-einer-unterhaltung-verwenden)).

## 1. Mit einer Vorlage starten

Im Panel: Broadcasts, dann Designs in der Seitenleiste. Designs hat zwei Tabs: Meine, Ihre eigenen
Designs, und Galerie, die Vorlagen. Die Galerie bietet sechs Startvorlagen: Newsletter, Product
update, Announcement, Welcome, Event und Digest, jeweils auf Englisch, Türkisch und Deutsch. Die
Chips darüber (Alle, Newsletter, Produkt, Ankündigung, Onboarding, Ereignis, Zusammenfassung) zeigen
jeweils eine Art, der Schalter Vorlagensprache wählt die Sprache; eine Vorlage ohne Text in Ihrer
Sprache zeigt „Text auf Englisch“.

Wählen Sie eine Vorlagenkarte oder Neues Design (Kürzel `C`), um den Dialog Neues Design zu öffnen.
Er zeigt Leere Seite und die Vorlagen als Kacheln. Wählen Sie eine, geben Sie dem Design einen Namen,
wählen Sie das Postfach (oder Gesamter Workspace) und wählen Sie Erstellen und bearbeiten. Die Bilder
einer Vorlage werden in die Bilder Ihres Workspace kopiert, danach können Sie alles ändern. Leere
Seite beginnt mit einer leeren Mail in schlichten Farben.

Unter Meine suchen Sie Designs mit `/`. Archivierte Designs anzeigen (`A`) nimmt archivierte Designs
in die Liste auf, markiert als Archiviert; Wiederherstellen bei einem holt es zurück. Meine und
Galerie sind `1` und `2`.

Ein Design gehört zu einem Posteingang oder zum ganzen Workspace. Die
[Absenderangaben](lists.md#1-absenderangaben-eintragen) des Posteingangs füllen die Fußzeile.

Per API: `POST /v1/email-designs` mit `name` und `template: { "slug": "newsletter", "locale": "de" }`.
`GET /v1/email-templates` listet die Vorlagen.

## 2. Das Design bearbeiten

Der Designer öffnet sich im Vollbild. Ziehen Sie Blöcke aus der Palette (oder der Ebenenübersicht)
oder drücken Sie Enter darauf, um sie nach dem gewählten Block einzufügen. Klicken Sie auf einen
Block, um ihn rechts unter Blockeinstellungen zu ändern.

| Block | Verwendung |
|---|---|
| Section | Eine Gruppe von Blöcken auf eigenem Hintergrund. Sections lassen sich nicht verschachteln. |
| Columns | Zwei oder drei Spalten, die sich auf Handys untereinander stapeln. Die Spaltenbreiten ergeben 100. Columns lassen sich nicht verschachteln. |
| Heading | Drei Größen. |
| Text | Absätze, Listen und Links. |
| Image | Ein hochgeladenes Bild, optional mit Link und Alt-Text. |
| Button | Ein Link, der wie ein Button aussieht. |
| Divider, Spacer | Eine dünne Linie, Leerraum. |
| Social links | Links zu Ihren Profilen, als Icons oder Text. |
| Footer text | Eigene Zeilen über der vorgeschriebenen Fußzeile. |
| Message | Die Stelle für den Text, den ein Mitglied schreibt, wenn das Design in einer Unterhaltung verwendet wird ([Abschnitt 8](#8-ein-design-in-einer-unterhaltung-verwenden)). Höchstens einer, nie in Spalten. |

Überschriften, Text, Fußzeilentext und Button-Beschriftungen bearbeiten Sie direkt an Ort und Stelle.
Text behält nur, was Mail-Apps gut darstellen: fett, kursiv, unterstrichen, durchgestrichen, Links,
Listen, Zeilenumbrüche und Code. Links beginnen mit `https://`, `http://`, `mailto:` oder `tel:`.

Jede Mail endet mit einer Fußzeile, die Sie nicht entfernen können: der juristische Name und die
Postanschrift des Posteingangs, der Grund, warum die Person die Mail erhält, sowie die Links zum
Abmelden und zu den Einstellungen. Eine Warnung weist darauf hin, wenn dem Posteingang noch
juristischer Name oder Postanschrift fehlen.

Ein Design enthält höchstens 200 Blöcke.

### Merge-Felder

Merge-Felder setzen die Angaben der jeweiligen Person in die Mail. Fügen Sie eines im Text über das
Menü Insert ein oder tippen Sie es. Nach `|` steht ein Ersatzwert für Personen, bei denen der Wert
leer ist:

```
Hallo {{ contact.first_name | "liebe Leserin, lieber Leser" }},
```

| Feld | Wert |
|---|---|
| `contact.name` | Der Name des Kontakts. |
| `contact.first_name` | Das erste Wort des Namens. |
| `contact.email` | Die E-Mail-Adresse. |
| `contact.attributes.<Schlüssel>` | Ein Attribut des Kontakts, über seinen Schlüssel. |
| `inbox.name` | Der Name des Posteingangs. |
| `list.name` | Die Liste, an die die Mail geht. |
| `unsubscribe_url`, `preferences_url` | Die eigenen Links der Person. In Links dürfen Merge-Felder nur nach dem `?` stehen. |

Ein Feld, das Yuva nicht kennt, wird abgelehnt. Ein Ersatzwert hat höchstens 100 Zeichen. Werte werden
nie zu Markup. Ohne Ersatzwert kann das Feld eine Lücke wie „Hallo ," lassen; der Designer warnt davor.

## 3. Theme und Dark Mode

Unter Design-Einstellungen (Kürzel `T`) stellen Sie Postfach, Vorschautext, Farben, Schriften, Inhaltsbreite (480 bis 720 Pixel),
Eckenradius und Richtung ein (links nach rechts, rechts nach links oder automatisch je Text). Die
Schriften sind Systemstapel: system, humanist, geometric, serif und monospace; Mails laden keine
Webfonts.

Leser, deren Mail-App im Dark Mode läuft, bekommen dunklere Farben, die aus Ihren abgeleitet werden,
mit Text bei 4,5:1 Kontrast. Unter Dark mode können Sie jede dieser Farben selbst festlegen. Ein
dunkles Logo auf transparentem Hintergrund kann im Dark Mode eine helle Platte dahinter bekommen.

## 4. Bilder

Öffnen Sie die Bildbibliothek an einem Image-Block (Kürzel `I`) oder legen Sie Dateien darauf ab.

| | |
|---|---|
| Typen | PNG, JPEG, GIF, WebP. SVG wird abgelehnt, weil es Skripte enthalten kann. |
| Upload-Größe | Höchstens 10 MiB; 40 Megapixel und 8.000 Pixel je Seite. |
| Gespeichert als | Dekodiert und ohne EXIF-, GPS- und Farbprofildaten neu kodiert. Breiter als 1.200 Pixel wird auf 1.200 verkleinert. WebP wird als PNG gespeichert, ohne Transparenz als JPEG. |
| GIF | Bleibt unverändert, mit Animation: höchstens 1.200 Pixel breit und 5 MiB. |
| Dieselbe Datei zweimal | Antwortet mit dem Bild, das Sie schon haben. |
| Adresse | Jedes Bild hat auf Ihrem Server eine öffentliche URL `/i/{token}`: keine Cookies, ein Jahr im Cache, nichts wird pro Aufruf aufgezeichnet. |

Geben Sie jedem Bild eine Beschreibung für Screenreader oder markieren Sie es als dekorativ. Die
Bilder eines Workspace dürfen zusammen höchstens `YUVA_EMAIL_IMAGES_MAX_BYTES` belegen
([Konfiguration](configuration.md)).

Ein Bild, das ein nicht archiviertes Design verwendet, lässt sich nicht löschen; das Panel nennt die
Designs. Bei einem Bild, das in einem Broadcast verschickt wurde, fragt es nach, weil bereits
zugestellte Mails dann ein defektes Bild zeigen. Bilder, die kein Design verwendet und kein Broadcast
versendet hat, werden 7 Tage nach dem Upload gelöscht.

Ein [KI-Assistent](mcp.md#listen-designs-und-einrichtung-über-einen-assistenten) kann mit `upload_email_image` ein Bild aus einer öffentlichen `https`-URL oder als Base64 (über MCP etwa 5 MB) hinzufügen und die Designs mit `list_designs` und `get_design` lesen.

## 5. Vorschauen und Prüfungen

Die Vorschau kommt vom Server, zeigt also, was Empfänger erhalten. Oben wechseln Sie zwischen
Bearbeiten und Vorschau; in der Vorschau wählen Sie Desktop oder Smartphone und schalten Dunkel an
oder aus. Die Tasten: `1` Bearbeiten, `2` Desktop, `3` Smartphone, `4` Dunkel.

Die Schaltfläche Prüfungen zeigt in Grün, Gelb oder Rot, wie viele Probleme es gibt. Sie öffnet über
dem Design den Streifen Prüfungen (Kürzel `C`): eine Anzeige der Mailgröße gegen 102 KB und je Problem
eine Zeile, markiert als Beheben oder Prüfen, mit einer Schaltfläche Anzeigen, die den Block auswählt.
„Keine Probleme gefunden.“ heißt, dass die Liste leer ist. Das wird markiert:

| Warnung | Bedeutung |
|---|---|
| Hard to read | Text unter 4,5:1 Kontrast zum Hintergrund, in hellen oder dunklen Farben. |
| Über 90 KB | Gmail schneidet Mails bei etwa 102 KB HTML ab und blendet den Rest aus, auch den Abmeldelink. Der Designer warnt ab 90 KB und erneut über 102 KB. |
| Bild fehlt, Beschreibung fehlt | Ein Image-Block ohne Bild oder ohne Alt-Text. |
| Kein Ersatzwert | Ein Merge-Feld, das für manche Personen leer sein kann. |
| Absenderangaben fehlen | Dem Posteingang fehlen juristischer Name oder Postanschrift. |

Ein Design mit mehr als 500 KB HTML wird abgelehnt.

## 6. Speichern, Versionen und Konflikte

Der Designer speichert 1,5 Sekunden nach Ihrer letzten Änderung; `Strg`/`Cmd` + `S` speichert sofort
und behält eine Version. Rückgängig und Wiederholen (`Strg`/`Cmd` + `Z`, mit `Umschalt` zum
Wiederholen) gehen 100 Schritte zurück. Offline bleiben Änderungen auf Ihrem Gerät und werden
gespeichert, sobald Sie wieder online sind.

Der Tab Versionen rechts (Kürzel `V`) behält die neuesten 50: eine höchstens alle 10 Minuten beim Bearbeiten, eine je
manuellem Speichern und eine je Wiederherstellung. Eine Wiederherstellung speichert diese Version als
neueste.

Hat jemand anderes (oder ein anderer Tab von Ihnen) zuerst gespeichert, bietet ein Dialog drei Wege:
meine behalten, meine als Kopie speichern oder ihre laden. Ihre Änderung geht nie verloren, und die
andere Version bleibt in den Versionen. Was Sie auch wählen, die Version der Gegenseite wird dort als
„Aus einem Konflikt behalten“ aufbewahrt; keine Änderung geht verloren.

JSON herunterladen, Kopie erstellen und Archivieren stehen im Menü Weitere Aktionen. Archivieren
blendet ein Design aus der Liste aus und behält seine Versionen; Wiederherstellen beim archivierten
Design holt es zurück. Ein Dokument, das ein neueres Yuva erstellt hat, lässt sich
ansehen, aber nicht ändern.

Kürzel: `A` Block hinzufügen, `T` Design-Einstellungen, `I` Bild, `V` Versionen, `C` Prüfungen, `Strg`/`Cmd` + `D` Block
duplizieren, `Backspace` löschen, `Alt` + `↑`/`↓` verschieben, `Esc` verlässt erst den Text, dann den
Block.

## 7. Die API

Alles, was das Panel tut, steht im [API-Vertrag](../../openapi/openapi.yaml). Dieser Aufruf rendert
ein Design, ohne es zu speichern:

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
        { "id": "h", "type": "heading", "level": 1, "text": "Hallo {{ contact.first_name | \"liebe Leserin, lieber Leser\" }}" },
        { "id": "b", "type": "button", "label": "Weiterlesen", "url": "https://example.com/news" }
      ]
    }
  }'
```

Die Antwort enthält `html`, `text`, `size_bytes` und `warnings`. Senden Sie `"dark": true` für die
dunkle Palette. Ein Speichern sendet die `version`, von der es ausging, als `base_version`; hat sich das
Design inzwischen geändert, lautet die Antwort `409 version_conflict` mit dem aktuellen Design in
`design`. Ein ungültiges Dokument ergibt `422 invalid_design` mit einem Eintrag je Problem in `errors`.

## 8. Ein Design in einer Unterhaltung verwenden

Eine Antwort oder neue Mail aus einem Posteingang kann statt als reiner Text in einem Ihrer Designs
rausgehen. Das Design braucht genau einen Message-Block, auf oberster Ebene oder in einem Abschnitt
(nicht in Spalten). Der Block hat keinen eigenen Text; der Text des Mitglieds wird hineingesetzt,
danach folgen die Karten der Hilfeartikel und die Signatur des Mitglieds. Ein Design mit Message-Block
lässt sich nicht als Broadcast senden: Prüfung und Testversand lehnen es ab.

Inhaber oder Admins bieten das Design in Unterhaltungen an (`for_conversations` bei
`PUT /v1/email-designs/{designId}`). Jedes Mitglied, das den Posteingang des Designs sieht, sowie
Schlüssel und Tokens mit `conversations:read` können die angebotenen Designs auflisten
(`GET /v1/email-designs?for_conversations=true`) und lesen; Bearbeiten bleibt bei Inhabern, Admins und
`broadcasts:write`.

Im Panel hat der Editor einer per E-Mail geführten Unterhaltung und einer neuen Mail eine
Design-Auswahl. Wählen Sie ein Design, geht die Nachricht darin raus. Per API kommt `design_id` (optional
`design_locale`) an `POST /v1/conversations/{id}/messages` (eine Antwort oder die erste Nachricht einer
von Ihnen eröffneten Unterhaltung) und an `POST /v1/conversations/mail`; `PATCH /v1/messages/{id}`
ändert das Design eines Entwurfs.

So funktioniert es:

- Es gilt die Sprachfassung des Kontakts, gewählt wie bei Broadcasts; `design_locale` nennt eine andere.
  Der Text wird escaped in den Message-Block gesetzt, eine Leerzeile beginnt einen Absatz.
- Der Textteil der Mail ist die ganze Mail als Text. Die Unterhaltung zeigt weiter, was geschrieben
  wurde, nicht das Design; `GET /v1/messages/{id}/email` liefert die Mail, wie sie gesendet wurde.
- Merge-Felder sind die des Kontakts. `list.name`, `unsubscribe_url` und `preferences_url` werden
  abgelehnt, und ein Feld ohne Fallback, das für den Kontakt leer ist, lässt die Nachricht scheitern
  (`422 invalid_design`).
- Header, Empfänger, Threading und SMTP-Konto sind die einer normalen Antwort.
- Designs gelten nur für per E-Mail gesendete Nachrichten: nicht für Notizen, Chat oder App-Unterhaltungen
  (`400 design_not_emailed`).
- Ein archiviertes oder nicht mehr angebotenes Design antwortet mit `409 design_unavailable`.
- Eine Mail über 500 KB HTML antwortet mit `422 html_too_large`.
- Neue Mail mit Design geht an genau eine `to`-Adresse und ohne `cc` (`400 design_one_recipient`).

Was ein Kontakt bekäme, zeigt `POST /v1/conversations/{id}/design-preview`: seine Fassung und
Merge-Werte, Ihre Signatur, die Fußzeile und Ihr Text (oder der Platzhalter des Designs). Die Antwort
enthält `html`, `text`, `subject` und `footer`; nichts wird gespeichert oder gesendet, und die 120
Renderings pro Minute teilt sie sich mit `POST /v1/email-designs/render`.

### Fußzeile: Antwort oder Erstkontakt

| Fußzeile | Wann | Inhalt |
|---|---|---|
| `reply` | Der Kontakt hat in der Unterhaltung geschrieben. | Die Zeile mit den Absenderangaben des Posteingangs. |
| `outreach` | Der Kontakt hat nicht geschrieben: erste Nachricht einer neuen Unterhaltung oder neue Mail. | Die Absenderzeile, ein Abmeldelink und der Header `List-Unsubscribe` mit One-Click. |

Abmeldelink und One-Click beenden den Erstkontakt mit Designs von diesem Posteingang an den Kontakt;
gewöhnliche Antworten bleiben unberührt. Ein Erstkontakt mit Design wird abgelehnt, wenn:

| Ablehnung | Grund |
|---|---|
| `409 email_opted_out` | Die Adresse hat eine Sperre beliebigen Umfangs, einen Opt-out des Workspace oder hat den Erstkontakt dieses Posteingangs beendet. |
| `409 sender_identity_missing` | Die [Absenderangaben](lists.md#1-absenderangaben-eintragen) des Posteingangs haben keinen rechtlichen Namen oder keine Postadresse. |
| `429 rate_limited` | Mehr als 20 pro Stunde je Mitglied oder Schlüssel oder 200 pro Tag (UTC) je Workspace. |
| `403 broadcasts_approval_required`, `403 broadcasts_suspended` | Der Server verlangt Freigabe für Broadcasts und Ihr Workspace ist nicht freigegeben oder wurde gesperrt. |

Antworten mit Design haben keine zusätzliche Grenze; ein Erstkontakt ohne Design folgt den Regeln einer
normalen Nachricht. Designs werden nie für Notizen, Chat- oder App-Unterhaltungen, Weiterleitungen,
Regelantworten, Begrüßungen oder Bewertungsanfragen verwendet.

## Referenz

### Endpunkte

| Endpunkt | Verwendung |
|---|---|
| `GET`, `POST /v1/email-designs` | Auflisten (`inbox_id`, `q`, `archived=true`, `for_conversations`) und aus einem `document`, einem `template` oder leer erstellen. |
| `GET`, `PUT`, `DELETE /v1/email-designs/{designId}` | Lesen, speichern (mit `base_version`), archivieren. |
| `POST /v1/email-designs/{designId}/duplicate` | Ein Design kopieren. |
| `POST /v1/email-designs/render` | Ein Dokument rendern: HTML, Text, Größe und Warnungen. Höchstens 120 pro Minute. |
| `POST /v1/conversations/{id}/design-preview` | Eine Nachricht in einem in Unterhaltungen angebotenen Design in der Vorschau zeigen. Scope `conversations:read`; teilt sich die 120 pro Minute. |
| `GET /v1/email-designs/{designId}/versions` | Die gespeicherten Versionen. |
| `GET /v1/email-designs/{designId}/versions/{version}` | Eine Version mit ihrem Dokument. |
| `POST /v1/email-designs/{designId}/versions/{version}/restore` | Eine Version als neueste speichern. |
| `GET`, `POST /v1/email-images` | Bilder auflisten und hochladen (multipart, eine `file`). |
| `DELETE /v1/email-images/{imageId}` | Ein Bild löschen (`force=true` für ein versendetes). |
| `GET /v1/email-templates`, `GET /v1/email-templates/{slug}` | Die Galerie (`locale=en`, `tr` oder `de`). |
| `GET /i/{token}` | Ein Bild, öffentlich. |
| `GET /email-templates/{slug}/{locale}.html`, `.webp` | Die Vorschau einer Vorlage, öffentlich. |

### Scopes

| Scope | Bedeutung |
|---|---|
| `broadcasts:read` | Designs, Versionen, Bilder und Vorlagen lesen; rendern. |
| `conversations:read` | In Unterhaltungen angebotene Designs auflisten, lesen und in der Vorschau zeigen. |
| `broadcasts:write` | Designs erstellen, speichern, kopieren, archivieren und wiederherstellen; Bilder hochladen und löschen. |

Ein auf Posteingänge beschränkter Key liest die Designs seiner Posteingänge und die workspaceweiten
und ändert nur Designs seiner Posteingänge. Keys, die vor diesen Scopes angelegt wurden, erhalten sie
nicht.

### Grenzen

| Grenze | Wert |
|---|---|
| Blöcke in einem Design | 200 |
| Design-Dokument | 300 KiB JSON |
| Gerendertes HTML | 90 KB Warnung, 102 KB abgeschnitten, 500 KB abgelehnt |
| Gespeicherte Versionen | 50 |
| Bild-Upload | 10 MiB, 40 Megapixel, 8.000 Pixel je Seite |
| Bildbreite nach dem Upload | 1.200 Pixel |
| Ersatzwert eines Merge-Felds | 100 Zeichen |
| Link | 2.048 Zeichen |
| Renderings | 120 pro Minute und Aufrufer |
| Nachrichten mit Design an Personen, die nicht geschrieben haben | 20 pro Stunde je Mitglied oder Schlüssel, 200 pro Tag je Workspace |
| Ungenutzte Bilder | Werden 7 Tage nach dem Upload gelöscht |
