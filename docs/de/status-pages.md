# Statusseiten

Eine Statusseite sagt Ihren Kunden, ob Ihr Produkt funktioniert: Komponenten mit einem Zustand, Vorfälle,
geplante Wartungen und ein Verlauf. Sie gehört zu einem Posteingang. Deshalb zeigen der Live-Chat und die
Apps dieses Posteingangs einen offenen Vorfall, bevor ein Kunde schreibt, und die Konversationen, die ein
Ausfall auslöst, lassen sich gemeinsam beantworten.

Die öffentliche Seite ist schlichtes HTML ohne Panel und ohne zwingendes JavaScript, erzeugt von Ihrem
Yuva-Server. Jeder Posteingang hat höchstens eine Statusseite.

## 1. Seite anlegen und veröffentlichen

Im Panel: Status (Kürzel `g s`), dann am Posteingang Statusseite erstellen. Seiten legen Inhaber und
Admins an und ändern sie; Agents mit Zugriff auf den Posteingang arbeiten mit Komponenten und Vorfällen.

| Einstellung (API-Feld) | Bedeutung |
|---|---|
| Name (`name`) | Bis zu 100 Zeichen. |
| Beschreibung (`description`) | Bis zu 1.000 Zeichen, unter dem Namen angezeigt. |
| Adresse (`slug`) | 3 bis 48 Zeichen: `a-z`, `0-9` und innere Bindestriche. Auf dem Server eindeutig; einige Wörter sind reserviert. |
| Logo (`logo_url`), Akzentfarbe (`accent_color`) | Ein `https`-Bild und eine Farbe wie `#2563eb`. |
| Logo für den dunklen Modus (`logo_dark_url`) | Optional. Ein `https`-Bild, das statt des Logos erscheint, wenn das System der Besucher im dunklen Modus ist, etwa eine helle Fassung eines dunklen Logos. Wird nur zusammen mit einem Logo verwendet. |
| Website-Link (`site_url`), Support-Link (`support_url`) | Ihr Produkt und ein `http(s)`- oder `mailto:`-Link, unter dem Kunden Hilfe bekommen. |
| Sprache (`locale`) | `en`, `tr` oder `de`; die Wörter der Seite folgen ihr. Andere Sprachen zeigen Englisch. |
| Zeitzone (`timezone`) | Die Zone der Tagesbalken. Besucher sehen Zeiten in ihrer eigenen Zone. |
| Tage Verlauf (`history_days`) | 7 bis 90, Standard 90. |
| Verfügbarkeit anzeigen (`show_uptime`) | Ein Prozentwert neben jeder Komponente. Standardmäßig an. |
| Antwortzeiten anzeigen (`show_response_times`) | Die Antwortzeiten der letzten 24 Stunden für Komponenten mit einem `http`- oder `tcp`-Monitor oder einem Push-Monitor, der in dieser Zeit eine `duration_ms` gesendet hat, mit Median und p95. Standardmäßig aus. |
| Namen neben dem Logo anzeigen (`show_name`) | Standardmäßig an. Schalten Sie es aus, wenn das Logo den Namen schon enthält: Die Kopfzeile zeigt dann nur das Logo, mit dem Namen als Alternativtext. Ohne Logo erscheint der Name immer. |
| Feed, Badge und Einbettung anzeigen (`show_follow`) | Standardmäßig an. Aus blendet den Abschnitt „Folgen und einbetten“, seinen Link in der Kopfzeile, die Links `feed.atom` und `badge.svg` in der Fußzeile und den Feed-Link auf Vorfallseiten aus. Feed, Badge und JSON bleiben unter ihren Adressen erreichbar. |
| Veröffentlicht (`published`) | Standardmäßig aus. Eine unveröffentlichte Seite antwortet allen außer Ihrem Team mit `404`; das Team sieht sie im Panel in der Vorschau. |

Ändern Sie die Adresse, bleibt die alte für die Seite reserviert, und jede öffentliche Adresse darunter
leitet auf die neue um.

Per API: `POST /v1/status-pages` mit `inbox_id`, `name` und `slug`, dann `PATCH
/v1/status-pages/{pageId}` mit `{"published": true}`. `GET /v1/status-pages/{pageId}/preview` liefert die
Seite so, wie Besucher sie sähen, auch solange sie unveröffentlicht ist.

## 2. Komponenten, Gruppen und Zustände

Komponenten sind die Teile Ihres Produkts, auf die sich Kunden verlassen: „Website“, „API“, „Checkout“.
Fügen Sie sie im Tab Komponenten hinzu, auf Wunsch in Gruppen, und ordnen Sie sie. Eine Seite hat höchstens
100. Eine versteckte Komponente fehlt auf der Seite und im Gesamtzustand.

| Zustand | Bedeutung |
|---|---|
| `operational` | Läuft. |
| `degraded` | Langsam oder teilweise beeinträchtigt. |
| `partial_outage` | Ein Teil der Nutzer oder Funktionen ist ausgefallen. |
| `major_outage` | Ausgefallen. |
| `maintenance` | Geplante Arbeiten. |

Der Gesamtzustand der Seite ist der schlechteste sichtbare Komponentenzustand; `maintenance` zählt nur,
wenn nichts Schlechteres vorliegt. Die Verfügbarkeit eines Tages ist der Anteil des Tages, der nicht in
`major_outage` lag; `partial_outage` zählt zur Hälfte, `maintenance` bleibt außen vor. Ein Tagesbalken zeigt
den schlechtesten Zustand seines Tages.

Setzen Sie einen Zustand im Panel von Hand oder mit
`POST /v1/status-pages/{pageId}/components/{componentId}/state` (`{"state": "degraded"}`). Eine Komponente
kann ein offener Vorfall, ein ausgefallener [Monitor](#5-monitore-heartbeat-und-push) oder Sie halten; das
Panel sagt, wer („Von einem Vorfall gehalten“, „Von einem Monitor gesetzt“).

Per API: `/v1/status-pages/{pageId}/components` und `/v1/status-pages/{pageId}/groups`.

## 3. Vorfälle und Wartung

Im Tab Vorfälle meldet Vorfall eröffnen ein Problem und Wartung planen kündigt Arbeiten an. Beide haben
einen Titel, die Komponenten, die sie berühren (mit dem Zustand, den jede erhält, solange sie offen sind),
und ein erstes Update.

| Art | Auswirkung | Status |
|---|---|---|
| Vorfall (`incident`) | `none`, `minor`, `major`, `critical` | `investigating`, `identified`, `monitoring`, `resolved` |
| Wartung (`maintenance`) | dieselben, Standard `none` | `scheduled`, `in_progress`, `completed` |

- **Entwurf oder veröffentlicht.** Als Entwurf speichern zeigt ihn nur Ihrem Team und ändert keine
  Komponente. Veröffentlichen zeigt ihn auf der Seite, setzt die Komponenten und stellt, sofern Sie Abonnenten
  benachrichtigen nicht abwählen, eine [Benachrichtigung](#8-e-mail-abonnenten) in die Warteschlange. Nur
  ein Entwurf lässt sich löschen.
- **Updates.** Update senden fügt Text hinzu und kann den Status weiterschalten. Der Text ist einfacher
  Text mit Links und Zeilenumbrüchen, höchstens 5.000 Zeichen. Ein veröffentlichtes Update lässt sich nicht
  entfernen; korrigieren Sie es mit einem neuen Update oder bearbeiten Sie es, dann erscheint „edited“.
- **Lösen.** Lösen (oder eine Wartung abschließen) setzt jede berührte Komponente auf `operational`, es sei
  denn, ein anderer offener Vorfall oder ein ausgefallener Monitor hält sie noch.
- **Wartungsfenster.** Eine Wartung braucht Beginn und Ende. Yuva startet und beendet sie pünktlich, sofern
  Sie es nicht vorher tun, und schreibt „The maintenance has started.“ bzw. „The maintenance is complete.“ in
  der Sprache der Seite dazu.

Per API: `POST /v1/status-pages/{pageId}/incidents` (mit `"publish": true` ohne Entwurf),
`PATCH /v1/status-pages/{pageId}/incidents/{incidentId}`, `POST …/incidents/{incidentId}/publish`,
`POST …/incidents/{incidentId}/updates` und `PATCH …/updates/{updateId}`. Ein gelöster Vorfall nimmt keine
Änderungen mehr an (`409 incident_closed`).

```sh
curl -X POST https://support.example.com/v1/status-pages/$PAGE/incidents \
  -H "Authorization: Bearer $YUVA_API_KEY" -H "Content-Type: application/json" \
  -d '{"title": "Checkout is failing", "message": "We are looking into it.", "impact": "major",
       "components": [{"component_id": "'$COMPONENT'", "state": "major_outage"}], "publish": true}'
```

## 4. Die öffentliche Seite

Eine veröffentlichte Seite liegt unter `<Status-URL>/{slug}`, mit Gesamtzustand, offenen Vorfällen und
Wartungen, den Komponenten mit Tagesbalken und den Vorfällen der letzten 14 Tage.

| Adresse | Was es ist |
|---|---|
| `/status/{slug}` | Die Seite. |
| `/status/{slug}/history` | Vergangene Vorfälle, nach Monat. |
| `/status/{slug}/incidents/{id}` | Ein Vorfall oder eine Wartung mit ihren Updates. |
| `/status/{slug}/feed.atom` | Ein Atom-Feed der Vorfälle. Braucht kein Abonnement. |
| `/status/{slug}/badge.svg` | Der Gesamtzustand als Badge, etwa für ein README. |
| `GET /client/v1/status/{slug}` | Dasselbe als JSON, für Ihre eigene Website oder App. Für jede Herkunft offen, ohne Zugangsdaten. |
| `/status/{slug}/subscribe` | Das [E-Mail-Anmeldeformular](#8-e-mail-abonnenten). |

Die Status-URL ist `YUVA_PUBLIC_URL` plus `/status`, etwa `https://support.example.com/status/acme`. Um die
Seiten von einem eigenen Host auszuliefern, setzen Sie `YUVA_STATUS_URL` auf einen Ursprung wie
`https://status.example.com`; die Pfade lauten dann ohne Präfix `/status` `https://status.example.com/acme`,
`/acme/history` und so weiter, und die alten Adressen leiten dorthin um. Der Host muss Ihren Yuva-Server
erreichen und sich von `YUVA_PUBLIC_URL` unterscheiden. Siehe [Konfiguration](configuration.md#statusseiten).

Die Seite lädt sich jede Minute neu und ist auch ohne JavaScript vollständig. Antworten werden 30 Sekunden
zwischengespeichert, sodass ein CDN vor Yuva Besucher bedienen kann, während Ihr Produkt ausfällt. Es gelten
120 Anfragen pro Minute je Adresse.

Zum Einbetten der Seite nehmen Sie einen `iframe`; um den Zustand anderswo zu zeigen, das Badge oder das JSON:

```html
<iframe src="https://support.example.com/status/acme" title="Status" height="600"></iframe>
<img src="https://support.example.com/status/acme/badge.svg" alt="Status">
```

```sh
curl -s https://support.example.com/client/v1/status/acme
```

Die Inbox-Einstellungen, die ein Widget oder eine App lädt, tragen den Slug der verknüpften Seite als `status.slug`; so kann ein Client dieses JSON, auch auf einer eigenen Domain, abrufen, ohne den Slug vorher zu kennen.

## 5. Monitore: Heartbeat und Push

Ein Monitor setzt den Zustand einer Komponente, ohne dass jemand Hand anlegt. Fügen Sie einen im Tab Monitore
hinzu (Inhaber und Admins): Komponente und Art wählen. Yuva zeigt die Adresse des Monitors einmal, mit
kopierbaren Beispielen. Wer die Adresse hat, kann für den Monitor melden; halten Sie sie geheim. Neue Adresse
erzeugen ersetzt sie, und die alte funktioniert sofort nicht mehr.

### Heartbeat: Ihr Job ruft Yuva auf

Für Server und Cron-Jobs, die Yuva nicht erreichen kann. Der Job ruft die Adresse mindestens alle `interval`
(30 Sekunden bis 30 Tage, Standard 5 Minuten) auf. Ein Signal, das `interval` plus `grace` (höchstens ein
Tag, Standard 1 Minute) ausbleibt, lässt den Monitor ausfallen.

```sh
curl -fsS -m 10 --retry 3 -o /dev/null https://support.example.com/ingress/heartbeat/TOKEN
```

In einer Crontab nur senden, wenn der Job erfolgreich war:

```sh
*/5 * * * * /path/to/job.sh && curl -fsS -m 10 --retry 3 -o /dev/null https://support.example.com/ingress/heartbeat/TOKEN
```

Einen fehlgeschlagenen Lauf sofort melden, ohne die Karenzzeit abzuwarten:

```sh
curl -fsS -m 10 -o /dev/null "https://support.example.com/ingress/heartbeat/TOKEN?fail=1"
```

`GET` und `POST` tun dasselbe, und `/ingress/heartbeat/TOKEN/fail` entspricht `?fail=1`.

### Push: Ihr Monitoring sendet den Zustand

Senden Sie `operational`, um den Monitor zu erholen, oder `degraded`, `partial_outage` bzw. `major_outage`,
um ihn ausfallen zu lassen. Der gesendete Zustand ist der, den die Komponente erhält; `message` (bis zu 1.000
Zeichen) wird mit der Prüfung gespeichert. Misst Ihr eigener Prober die Antwortzeit, senden Sie sie als
`duration_ms` (eine ganze Zahl von 0 bis 120000): Die Antwortzeiten der Seite zählen sie bei erfolgreichen
Prüfungen.

```sh
curl -fsS -X POST -H "Content-Type: application/json" \
  -d '{"state":"major_outage","message":"Checkout is failing"}' \
  https://support.example.com/ingress/status/TOKEN

curl -fsS -X POST -H "Content-Type: application/json" \
  -d '{"state":"operational","duration_ms":182}' https://support.example.com/ingress/status/TOKEN
```

Alertmanager- und Grafana-Alarme werden unverändert angenommen: firing lässt den Monitor ausfallen, resolved
erholt ihn.

```yaml
receivers:
  - name: yuva-status
    webhook_configs:
      - url: https://support.example.com/ingress/status/TOKEN
        send_resolved: true
```

Legen Sie in Grafana einen Contact Point vom Typ Webhook an, fügen Sie die Adresse ein, lassen Sie die
Methode `POST` und verwenden Sie den Contact Point in einer Notification Policy.

### Einstellungen, Schwellen und manuelle Eingriffe

| Einstellung (API-Feld) | Bedeutung |
|---|---|
| Zustand bei Ausfall (`failure_state`) | `degraded`, `partial_outage` oder `major_outage` (Standard). Ein Push, der seinen eigenen Zustand nennt, behält ihn. |
| Bei Ausfall (`on_failure`) | Öffnet einen Entwurf-Vorfall (`draft`, Standard), einen veröffentlichten (`publish`) oder keinen (`none`). |
| Fehler bis zum Ausfall (`failure_threshold`) | Fehler in Folge, bevor der Monitor ausfällt, 1 bis 10, Standard 1. |
| Erfolge bis zur Wiederherstellung (`recovery_threshold`) | Erfolge in Folge, bevor er wieder läuft, 1 bis 10, Standard 1. |
| Pausiert (`paused`) | Ignoriert Ergebnisse und gibt die Komponente frei. Beim Fortsetzen beginnt er wieder bei `pending`. |

Ihr Job oder Ihr Monitoring hat schon entschieden, was ein Fehler ist; deshalb beginnen die Schwellen bei 1.

Ein Monitor überstimmt Ihre Hand nie: Hat ein Mitglied oder ein API-Key den Zustand der Komponente nach der
letzten Änderung des Monitors gesetzt, lässt der Monitor ihn in Ruhe, bis sich sein eigener Zustand wieder
ändert. Solange ein Monitor ausgefallen ist, folgt sein Vorfall dem gemeldeten Zustand; bei der
Wiederherstellung löst er den Vorfall und setzt die Komponente zurück, sofern nichts anderes sie hält.
Pausieren oder Löschen eines Monitors bewirkt dasselbe. Das Panel zeigt eine von einem Mitglied gesetzte
Komponente als haltend.

Fällt ein Monitor aus oder erholt er sich, werden die Mitglieder des Posteingangs wie bei einer neuen
Konversation benachrichtigt, und `status.monitor.down` und `status.monitor.up` gehen an
[Webhooks](webhooks.md). Der Tab Monitore listet die letzten Prüfungen, die 30 Tage aufbewahrt werden. Ein
Workspace hat höchstens `YUVA_STATUS_MONITORS` Monitore (Standard 20, `403 status_monitor_limit`). Ein Token
nimmt 60 Aufrufe pro Minute an.

Per API: `/v1/status-pages/{pageId}/monitors`, `POST …/monitors/{monitorId}/token` für eine neue Adresse und
`GET …/monitors/{monitorId}/checks`.

## 6. Status im Widget und in den Apps

Solange die Seite veröffentlicht ist und ein Vorfall oder eine Wartung offen ist, zeigen das
[Web-Widget](widget.md) und die iOS- und Android-[SDKs](mobile.md) über der Konversationsliste und dem
Eingabefeld eine Zeile mit dem Zustand in Worten und einem Link zum Vorfall. Der schlimmste offene wird zuerst
gezeigt. Kunden können sie schließen; sie kommt wieder, wenn der Vorfall ein neues Update bekommt. Es ist
nichts einzurichten.

## 7. Konversationen mit einem Vorfall verknüpfen

Ein Ausfall bringt viele Konversationen. Öffnen Sie im Panel eine Konversation und verknüpfen Sie sie mit
einem Vorfall (oder nutzen Sie
`PUT /v1/status-pages/{pageId}/incidents/{incidentId}/conversations/{conversationId}`). Die Seite des Vorfalls
listet seine verknüpften Konversationen und deren Status; ein Vorfall nimmt höchstens 5.000 auf.

An Verknüpfte antworten schreibt eine Antwort und sendet sie an jede verknüpfte Konversation, die noch offen
ist, als Sie, über den eigenen Kanal jeder Konversation (E-Mail, Chat oder App). Mit dem Häkchen schließen Sie
jede Konversation danach. Prüfen zeigt, wie viele Konversationen sie erhalten. Eine gesendete Antwort lässt
sich nicht zurückholen. Das Panel meldet je Konversation, ob sie gesendet, übersprungen (geschlossen,
entknüpft, gelöscht oder Ihr Zugriff endete) oder fehlgeschlagen ist.

Per API: `POST …/incidents/{incidentId}/reply` antwortet mit `202` und einer ID,
`GET …/incidents/{incidentId}/replies/{replyId}` meldet das Ergebnis. Ein Key braucht `status:manage`,
`conversations:read` und `messages:write` (und `conversations:write` zum Schließen), und der Workspace muss
Bots das Senden erlauben ([Bots may send](headless.md#bots-may-send)).

## 8. E-Mail-Abonnenten

Besucher können Benachrichtigungen über Vorfälle und Wartungen abonnieren. Schalten Sie das in den
Einstellungen der Seite ein: Benachrichtigungen per E-Mail senden und der E-Mail-Kanal, der sendet. Der Kanal
muss senden können (sonst `409 sender_not_ready`). Die Seite warnt, ohne zu blockieren, wenn die
Absenderdomain keine Zustellbarkeitsprüfung bestanden hat, der Posteingang keine Postanschrift hat oder die
Mail nicht mit der eigenen Domain des Absenders per DKIM signiert ist; siehe
[Zustellbarkeit](deliverability.md).

- **Anmelden.** Die Seite verlinkt ihr Anmeldeformular: eine Adresse, ein Kästchen je Komponente (alle
  angehakt; abwählen, um nur von einigen zu hören) und der Satz, dem der Besucher zustimmt. Es gilt immer
  Double-Opt-in: Die Antwort ist für jede Adresse gleich, danach folgt eine Bestätigungsmail. Wer keine
  Komponenten gewählt hat, erhält alles, auch Komponenten, die Sie später hinzufügen. Abonnenten ändern ihre
  Auswahl oder melden sich über den Link in jeder Mail ab.
- **Was eine Benachrichtigung enthält.** Den Zustand in Worten (Investigating, Identified, Monitoring,
  Resolved, Scheduled, In progress, Completed), die Auswirkung, die betroffenen Komponenten mit ihren
  Zuständen, die Updates seit der letzten Benachrichtigung mit Zeiten in der Zone der Seite, einen Link zum
  Vorfall und einen Link, um Komponenten zu wählen oder sich abzumelden. Sie ist in der Sprache des
  Abonnenten. Betreffzeilen lauten `[Seite] Titel` und `[Seite] Maintenance: Titel`; Benachrichtigungen
  eines Vorfalls teilen sich Betreff und Thread. Sie enthält keine Bilder, kein Tracking und keine Werbung.
  Sie geht über den E-Mail-Kanal des Posteingangs, von dessen Adresse.
- **Wann sie gesendet wird.** Ein neuer veröffentlichter Vorfall benachrichtigt standardmäßig; ein Update
  benachrichtigt, wenn es Status oder Fenster ändert oder den Vorfall löst, nicht, wenn es nur Text
  hinzufügt. Ändern Sie das mit Abonnenten benachrichtigen am Vorfall oder Update (es zeigt, wie viele es
  erreichen würde) oder mit `notify` per API. Reine Komponentenänderungen und Entwürfe senden nichts.
- **Wartezeit und Rückgängig.** Eine Benachrichtigung wartet 60 Sekunden, damit Sie einen Tippfehler beheben
  können; Updates, die Sie währenddessen senden, schließen sich ihr an. Rückgängig im Verlauf des Vorfalls
  (oder `POST /v1/status-pages/{pageId}/notices/{noticeId}/cancel`) stoppt sie vor dem Versand (danach
  `409 not_pending`).
- **Abstand und Stundenlimit.** Zwischen Benachrichtigungen zu einem Vorfall liegen mindestens 15 Minuten,
  und eine Seite sendet höchstens 10 pro Stunde; weitere werden übersprungen, und der Verlauf sagt es.
- **Abschließende Benachrichtigungen.** Die Benachrichtigung, dass ein Vorfall gelöst oder eine Wartung
  begonnen oder abgeschlossen ist, ignoriert Abstand und Stundenlimit. Sie geht nur an Personen, die eine
  frühere Benachrichtigung zu diesem Vorfall erhalten haben; wurde vorher keine gesendet, wird sie
  übersprungen (`nothing_earlier`), weil niemand von dem Vorfall wusste.
- **Grenzen.** `YUVA_STATUS_NOTICES`, `YUVA_STATUS_NOTICES_DAILY` und `YUVA_STATUS_MAX_SUBSCRIBERS` am
  Server; siehe [Konfiguration](configuration.md#statusseiten). Das Anmelden ist auf 10 pro Stunde je
  Besucheradresse und 300 pro Stunde je Seite begrenzt. Unbestätigte Anmeldungen werden nach 30 Tagen
  entfernt.

Der Tab Abonnenten listet Adressen mit Zustand und gewählten Komponenten, fügt eine Adresse hinzu (sie
bekommt die Bestätigung), entfernt eine und exportiert die Einwilligungsnachweise. Abonnenten gehören zu einer
Mailingliste des Posteingangs der Art `status`; sie wird nie in Formularen, Broadcasts oder CSV-Importen
angeboten, und wer sie verlässt, ist dadurch nirgends sonst abgemeldet. Die Benachrichtigungen der Seite
stehen unter `GET /v1/status-pages/{pageId}/notices`.

## API, Webhooks und MCP

| Scope | Erlaubt |
|---|---|
| `status:read` | Seiten, Komponenten, Vorfälle, Benachrichtigungen, Monitore und ihre Prüfungen. |
| `status:manage` | All das anlegen und ändern, Updates senden, Zustände setzen, Konversationen verknüpfen. Enthält `status:read` nicht: Geben Sie einem Key beide. |

Seiten, Gruppen, Komponenten und Monitore sind Sache von Inhabern und Admins; Agents und Keys mit dem Scope
arbeiten außerdem mit Zuständen, Vorfällen und Benachrichtigungen der Posteingänge, die sie sehen können. Die
ganze Oberfläche steht im Abschnitt `status` von [openapi.yaml](../../openapi/openapi.yaml).

[Webhooks](webhooks.md) können `status.component.updated`, `status.incident.created`,
`status.incident.updated`, `status.incident.resolved`, `status.monitor.down` und `status.monitor.up`
abonnieren. Entwürfe gehen nicht an Webhooks.

[KI-Assistenten (MCP)](mcp.md) erhalten `list_status_pages`, `get_status_page`, `list_monitors`,
`set_component_state`, `create_incident` (immer ein Entwurf, den ein Mitglied veröffentlicht),
`post_incident_update`, `resolve_incident` und `link_conversation_to_incident`.
