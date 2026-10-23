# Hilfecenter

Ein Hilfecenter beantwortet häufige Fragen schriftlich, dort, wo Kunden zuerst suchen: öffentliche Seiten
für Ihr Produkt mit Sammlungen und Artikeln in mehreren Sprachen. Sie sind durchsuchbar auf der Seite
selbst, auf der Startseite des [Web-Widgets](widget.md) und im Antwortfeld Ihres Teams. Ein Hilfecenter
gehört zu einem Posteingang: Ein Kunde, der trotzdem Hilfe braucht, schreibt an denselben Posteingang,
und die Suchen ohne Treffer zeigen, welchen Artikel Sie als Nächstes schreiben sollten.

Die öffentlichen Seiten sind schlichtes HTML ohne Panel und ohne zwingendes JavaScript, erzeugt von Ihrem
Yuva-Server. Jeder Posteingang hat höchstens ein Hilfecenter. Die Produktdokumentation bleibt auf Ihrer
eigenen Website; das Hilfecenter enthält Support-Artikel, die in Yuva geschrieben werden.

## 1. Center anlegen und veröffentlichen

Im Panel: Hilfe in der Seitenleiste, dann am Posteingang ein Hilfecenter erstellen. Center legen Inhaber
und Admins an und ändern sie; wer Artikel veröffentlichen darf, steht unter
[Entwürfe und Veröffentlichung](#entwürfe-und-veröffentlichung).

| Einstellung (API-Feld) | Bedeutung |
|---|---|
| Name (`name`) | Bis zu 100 Zeichen. |
| Beschreibung (`description`) | Bis zu 1.000 Zeichen, auf der Startseite angezeigt. |
| Adresse (`slug`) | 3 bis 48 Zeichen: `a-z`, `0-9` und innere Bindestriche. Auf dem Server eindeutig; einige Wörter sind reserviert. |
| Sprachen (`locales`), Standard (`default_locale`) | Bis zu 20 [Sprachen](widget.md#sprachen), die das Center veröffentlicht; der Standard ist eine davon. |
| Logo (`logo_url`), Akzentfarbe (`accent_color`) | Ein `https`-Bild und eine Farbe wie `#2563eb`. |
| Website-Link (`site_url`) | Ihr Produkt, ein `http(s)`-Link. |
| Chat (`chat_channel_id`) | Ein [Chat-Kanal](widget.md), dessen Chat die Seiten unter „Brauchen Sie noch Hilfe?“ anbieten. Eine dort begonnene Unterhaltung trägt den Artikel, den der Besucher gelesen hat. |
| Analytics (`analytics_property_id`) | Eine Website von [Analytics](analytics.md), die cookielose Seitenaufrufe der öffentlichen Seiten zählt. |
| Agents dürfen veröffentlichen (`agents_may_publish`) | Standardmäßig aus. Siehe unten. |
| Veröffentlicht (`published`) | Standardmäßig aus. Ein unveröffentlichtes Center antwortet allen außer Ihrem Team mit `404`; das Team sieht es im Panel in der Vorschau. |

Ändern Sie die Adresse, bleibt die alte für das Center reserviert, und jede öffentliche Adresse darunter
leitet auf die neue um.

Per API: `POST /v1/help-centers` mit `inbox_id`, `name` und `slug`, dann
`PATCH /v1/help-centers/{centerId}` mit `{"published": true}`.
`GET /v1/help-centers/{centerId}/preview` liefert eine Seite so, wie Besucher sie sehen würden, auch
solange das Center unveröffentlicht ist.

## 2. Sammlungen und Artikel

Eine *Sammlung* fasst Artikel zusammen, etwa „Abrechnung“ oder „Erste Schritte“, hat ein Symbol aus einer
festen Auswahl und höchstens eine Ebene Untersammlungen. Ein *Artikel* liegt in einer Sammlung oder in
keiner und hat eine *Übersetzung* pro Sprache. Sammlungen haben Name und Beschreibung pro Sprache; eine
ohne veröffentlichten Artikel in einer Sprache wird in dieser Sprache nicht angezeigt.

Ein Artikel wird in Markdown geschrieben: Überschriften, Listen, Tabellen, Code, Links und Bilder. Rohes
HTML wird entfernt, und Bilder bleiben nur, wenn Sie sie in Ihren Workspace hochgeladen haben. Laden Sie
im Editor hoch oder mit `POST /v1/help-centers/{centerId}/images` (dieselben Typen, Größen und dieselbe
Neukodierung wie bei [E-Mail-Bildern](designs.md#4-bilder); die Antwort enthält das einzufügende
Markdown). Ein Bild, das kein Entwurf nutzt und das kein veröffentlichter Artikel gezeigt hat, wird eine
Woche nach dem Hochladen gelöscht.

| Feld | Grenze |
|---|---|
| Titel (`title`) | 3 bis 200 Zeichen. |
| Text (`body`) | Markdown, bis zu 100.000 Zeichen. |
| Schlüsselwörter (`keywords`) | Bis zu 500 Zeichen zusätzliche Suchwörter, etwa Synonyme. |
| Adresse (`slug`) | Aus dem Titel erzeugt, änderbar. Ein falscher Slug in einer URL leitet auf den richtigen um; einen Artikel umzubenennen bricht also keinen Link. |
| Seiten (`paths`) | Bis zu 20 Seitenpfade wie `/billing/*`. Das Widget schlägt den Artikel auf diesen Seiten als besten Treffer vor. |

Ein Workspace hat höchstens `YUVA_HELP_MAX_ARTICLES` Artikel ([Konfiguration](configuration.md#hilfecenter));
darüber antwortet das Anlegen eines Artikels mit `422 hosted_limit`.

Per API, unter `/v1/help-centers/{centerId}`: `/collections`, `/articles` und `/articles/{articleId}`.
`GET /articles` filtert nach `collection_id`, `locale` und `state`.

## Entwürfe und Veröffentlichung

Eine Übersetzung hat einen Arbeitsentwurf und, sobald veröffentlicht, eine eingefrorene veröffentlichte
Kopie. Speichern ändert nur den Entwurf; die öffentliche Seite ändert sich, wenn Sie veröffentlichen.

- **Speichern.** Jedes Speichern trägt die `base_version`, die der Editor geladen hat. Hat dazwischen
  jemand gespeichert, wird das Speichern mit `409 version_conflict` abgelehnt, und die Antwort enthält die
  Übersetzung im aktuellen Stand; niemand überschreibt also die Arbeit eines anderen. Andere geöffnete
  Editoren erfahren, dass sich der Artikel geändert hat.
- **Veröffentlichen.** Veröffentlichen friert den Entwurf ein, rendert ihn und stellt ihn ins Web;
  Zurückziehen nimmt die Übersetzung aus dem Web und behält ihren Entwurf. Eine Übersetzung zeigt
  „nicht veröffentlichte Änderungen“, solange ihr Entwurf von der veröffentlichten Kopie abweicht.
- **Wer veröffentlichen darf.** Agents mit Zugriff auf den Posteingang schreiben Entwürfe, Sammlungen und
  Übersetzungen. Inhaber und Admins veröffentlichen außerdem, ziehen zurück, löschen veröffentlichte
  Artikel und ändern die Einstellungen des Centers. Schalten Sie Agents dürfen veröffentlichen
  (`agents_may_publish`) ein, damit auch Agents mit Zugriff auf den Posteingang veröffentlichen dürfen.
- **Assistenten und OAuth.** [MCP](mcp.md#hilfecenter-über-einen-assistenten)-Tools und OAuth-Tokens
  veröffentlichen nie, egal wie die Einstellung steht: Ein Assistent schreibt einen Entwurf, ein Mitglied
  veröffentlicht. API-Schlüssel können mit der Berechtigung `help:publish` veröffentlichen.

Per API: `PUT /v1/help-centers/{centerId}/articles/{articleId}/translations/{locale}` speichert einen
Entwurf, und `POST` auf denselben Pfad mit `/publish` oder `/unpublish` veröffentlicht oder zieht zurück.
[Webhooks](webhooks.md) senden `help.article.published`, `help.article.unpublished` und
`help.article.deleted`.

## Sprachen und Übersetzungen

Ein Center veröffentlicht die Sprachen in `locales`; Artikel und Sammlungen werden nur in ihnen
geschrieben (sonst `400 locale_not_offered`). Yuva betreibt kein Übersetzungsmodell: Eine Übersetzung ist
Text, den Sie oder Ihr Team schreiben. Der Editor zeigt den Text der Standardsprache neben dem, den Sie
übersetzen, markiert Übersetzungen, die älter als der Standard sind, und listet die fehlenden auf.

Die öffentlichen Seiten folgen der Sprache in ihrer Adresse (`/en/…`, `/de/…`); Yuva leitet nicht nach der
Sprache des Browsers um. Jede Seite führt für Suchmaschinen ihre weiteren veröffentlichten Sprachen auf.
Die Wörter der Seite selbst (Suche, „War das hilfreich?“) gibt es auf Englisch, Türkisch und Deutsch; andere
Sprachen zeigen Englisch. Das Widget wählt die Sprache des Besuchers aus der Liste des Centers (erst
exakt, dann die Basissprache, sonst den Standard) und zeigt einen Artikel ohne Übersetzung in der
Standardsprache.

## Revisionen

Jede Übersetzung behält ihre letzten 50 Revisionen: einen gespeicherten Entwurf (höchstens alle
10 Minuten pro Autor), jede Veröffentlichung und jede Wiederherstellung. Öffnen Sie den Verlauf einer
Übersetzung und stellen Sie eine Revision im Entwurf wieder her; veröffentlicht wird sie erst, wenn Sie
veröffentlichen. Per API: `GET …/translations/{locale}/revisions` und
`POST …/revisions/{revisionId}/restore`.

## Öffentliche Seiten

Jedes veröffentlichte Center liegt auf Ihrem Server unter `/help/{slug}` oder unter
`<YUVA_HELP_URL>/{slug}`. Darunter:

| Adresse | Seite |
|---|---|
| `/{locale}/` | Startseite: Suche, Sammlungen und beliebte Artikel. |
| `/{locale}/collections/{number}-{slug}` | Eine Sammlung mit ihren Untersammlungen und Artikeln. |
| `/{locale}/articles/{number}-{slug}` | Ein Artikel mit Inhaltsverzeichnis, verwandten Artikeln und „War das hilfreich?“. |
| `/{locale}/search?q=` | Suchergebnisse. |
| `/sitemap.xml` | Alle veröffentlichten Seiten, für Suchmaschinen. |

„War das hilfreich?“ funktioniert ohne JavaScript: Die Schaltflächen Ja und Nein senden ein Formular an
`/{locale}/articles/{number}-{slug}/helpful`, und die Seite kommt mit `?thanks=up` oder `?thanks=down`
zurück und zeigt eine Dankeszeile und eine Schaltfläche, um die Antwort zu ändern. Ein Skript sendet das
Formular wenn möglich an Ort und Stelle. Seiten werden eine Minute zwischengespeichert und tragen einen
kanonischen Link, Alternativen für jede Sprache, Open-Graph-Tags und strukturierte Daten. Öffentliche
Seiten beantworten 120 Anfragen pro Minute und Adresse.

`YUVA_HELP_URL` ist ein Ursprung, den Sie auf den Server zeigen lassen, etwa `https://help.example.com`.
Setzen Sie ihn in der [Konfiguration](configuration.md#hilfecenter); die Center liegen dann unter
`<Ursprung>/{slug}`, und der Ursprung muss sich von `YUVA_PUBLIC_URL` unterscheiden. Ein Center kann
auch eine eigene Domain haben, etwa `help.example.com`: Fügen Sie sie in den Einstellungen des Centers hinzu,
legen Sie die dort gezeigten CNAME- und TXT-Einträge an, und sobald sie bestätigt sind, antwortet das Center
unter `/` auf dieser Domain (die Servereinstellung ist `YUVA_STATUS_DOMAINS` in der
[Konfiguration](configuration.md)). `YUVA_HELP=off` entfernt Hilfecenter ganz: Ihre Pfade antworten mit
`404`.

## Suche und Bericht

Panel, Widget, öffentliche Seiten und Assistenten teilen sich eine Volltextsuche pro Sprache. Alle Wörter
müssen passen, und das letzte passt als Präfix, sodass Ergebnisse schon beim Tippen erscheinen; hat kein
Artikel alle Wörter, werden Artikel mit einigen davon gezeigt. Der Titel wiegt am meisten, dann die
Schlüsselwörter, dann der Text. Suchen von Mitgliedern im Panel werden nicht gezählt.

Der Bericht (Hilfe → das Center → Bericht oder `GET /v1/help-centers/{centerId}/report?days=7|30|90`)
zeigt für die letzten 7, 30 oder 90 Tage:

- die meistgesehenen Artikel, den Anteil von Ja gegenüber Nein, Unterhaltungen, die von einem Artikel
  ausgingen, und wie oft er als Karte gesendet wurde;
- Suchen und die Suchen ohne Treffer, häufigste zuerst: die nächsten Artikel, die zu schreiben sind;
- die Artikel mit den meisten Nein-Antworten: die, die zu verbessern sind.

Die Zähler speichern keine Besucherdaten. Aufrufe sind ungefähr: einer pro Adresse, Artikel und Stunde,
und Seiten, die ein CDN auslieferte, werden nicht gesehen. Suchen, die wie eine E-Mail-Adresse, eine URL
oder eine Zahl aussehen, werden als `(scrubbed)` gezählt.

## Im Web-Widget

Hat der Posteingang ein veröffentlichtes Center, bekommt die Startseite des
[Widgets](widget.md#was-besucher-sehen) ein Suchfeld und eine Hilfecenter-App. Sie zeigt die besten
Treffer für die Seite, auf der der Besucher ist (aus den `paths` der Artikel), die beliebten Artikel und
die Sammlungen, öffnet Artikel im Panel und fragt „War das hilfreich?“. Eine Schaltfläche am Ende beginnt
eine Unterhaltung. Schreibt der Besucher nach dem Lesen eines Artikels, hält die Unterhaltung ihn als
`help_article_id` fest, und Ihr Team sieht, welchen Artikel der Kunde zuerst gelesen hat. Die ID wird nur
für einen veröffentlichten Artikel des Centers des Posteingangs behalten.

Für eine eigene Oberfläche nutzen die [shadcn-Komponenten](https://useyuva.com/shadcn) `yuva-help`,
`yuva-help-search`, `yuva-help-article` und die Hooks in `use-yuva-help` dieselben Endpunkte
`/client/v1/channels/{channel_key}/help` wie der [Headless-Client](headless.md).

## Artikelkarten in Unterhaltungen

In einer Unterhaltung finden die Hilfe-Schaltfläche des Antwortfelds und die Suche mit `/` veröffentlichte
Artikel. Wählen Sie bis zu drei und senden Sie: Der Kunde bekommt eine Karte mit Titel und Auszug in seiner
Sprache (die des Kontakts, sonst die des Posteingangs, sonst die Standardsprache des Centers). Im Chat
öffnet die Karte den Artikel im Widget; per E-Mail steht im Textteil „Titel - URL“ und im HTML-Teil ein
Block mit Link, ohne Tracking. Per API nimmt `POST /v1/conversations/{id}/messages` mit `article_ids` bis
zu 3 veröffentlichte Artikel des Centers des Posteingangs. Ziehen Sie einen Artikel später zurück, bleiben
alte Nachrichten lesbar, und der Link zeigt die Nicht-gefunden-Seite des Centers.

## Import aus Zendesk oder Help Scout

Holen Sie ein vorhandenes Help-Center als Entwürfe in dieses. Nichts wird veröffentlicht, die Quelle
wird nur gelesen.

- **Zendesk Guide.** Nutzen Sie eine angemeldete Zendesk-Migration (Zu Yuva umziehen); ihre
  Nur-Lese-Anmeldung wird wiederverwendet, es wird keine neue Berechtigung verlangt. Kategorien
  werden Sammlungen, Abschnitte deren Untersammlungen (ein Abschnitt in einem Abschnitt wird
  `Eltern / Kind`). Jeder Artikel kommt mit allen Übersetzungen in Sprachen, die das Center anbietet;
  andere Sprachen werden als übersprungen aufgeführt – ergänzen Sie die Sprache im Center und
  importieren Sie erneut.
- **Help Scout Docs.** Legen Sie in Help Scout einen Docs-API-Schlüssel an (Your Profile, API Keys)
  und geben Sie ihn dem Import; Yuva hält ihn versiegelt, bis der Import endet. Sammlungen werden
  Sammlungen, Kategorien Untersammlungen. Docs-Sites haben keine Sprachen: Wählen Sie die Sprache der
  Artikel.
- **Was unterwegs geschieht.** Artikel-HTML wird Markdown. Bilder werden geladen, als Bilder des
  Centers gespeichert und umgeschrieben; ein nicht ladbares bleibt ein Link. Links zwischen
  importierten Artikeln zeigen auf die neuen Seiten.
- **Erneut ausführen.** Ein früher importierter Artikel aktualisiert seinen Entwurf und wird nie
  verdoppelt. Ein Entwurf, den Sie seitdem in Yuva bearbeitet haben, bleibt erhalten, außer Sie
  wählen Überschreiben.
- **Weiterleitungen.** Der Import listet jede alte URL mit der neuen (als CSV herunterladbar). Die
  Weiterleitung unter der alten Adresse einzurichten ist Ihre Sache; die neue URL antwortet `404`,
  bis Sie den Artikel veröffentlichen.

Starten Sie ihn mit `POST /v1/help-centers/{id}/imports` (Berechtigung `help:write`); ein Assistent
kann einen Zendesk-Import starten und mit den Tools `import_help_articles` und `get_help_import`
verfolgen.

## API

Alles liegt unter `/v1/help-centers` und trägt in der [API-Referenz](headless.md) das Tag `help`.

| Berechtigung | Erlaubt |
|---|---|
| `help:read` | Center, Sammlungen, Artikel mit Entwürfen und Revisionen, Suche, Bericht. Wird Schlüsseln gegeben, die ohne benannte Berechtigungen angelegt werden. |
| `help:write` | Sammlungen und Artikel anlegen und ändern, Entwürfe speichern, Bilder hochladen. Wird Schlüsseln gegeben, die ohne benannte Berechtigungen angelegt werden. |
| `help:publish` | Veröffentlichen, zurückziehen und (mit `help:write`) veröffentlichte Artikel löschen. In keiner Berechtigung enthalten: Nennen Sie sie ausdrücklich. |

Ein auf Posteingänge beschränkter Schlüssel sieht nur die Center dieser Posteingänge; jedes andere Center
antwortet mit `404`. Bestehende Schlüssel erhalten keine dieser Berechtigungen.
