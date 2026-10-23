# Betrieb

So aktualisieren, sichern, wiederherstellen und überwachen Sie einen Yuva-Server. Die Beispiele
verwenden das Compose-Setup aus der [Installationsanleitung](install.md) und laufen in dessen
Verzeichnis.

## Updates

1. Lesen Sie die Release Notes jeder Version zwischen Ihrer und der neuen.
2. [Sichern](#backups) Sie die Datenbank und die Anhänge.
3. Ändern Sie den Image-Tag in `compose.yaml` und starten Sie die neue Version:

   ```sh
   docker compose up -d
   ```

Migrationen laufen beim Start; der Server ist bereit, wenn `/readyz` mit `200` antwortet. Lassen Sie
während des Updates nur einen Serverprozess laufen. Um zurückzugehen, spielen Sie das Backup aus
Schritt 2 wieder ein.

## Backups

Ein vollständiges Backup hat drei Teile: die Datenbank, die Anhänge und den Master-Key.

1. Sichern Sie die Datenbank. `pg_dump` erstellt einen konsistenten Snapshot, während Yuva läuft:

   ```sh
   docker compose exec -T db pg_dump -U yuva -d yuva -Fc > yuva-$(date +%F).dump
   ```

2. Archivieren Sie danach die Anhänge. Mit `YUVA_STORAGE=local`:

   ```sh
   docker run --rm -v yuva_attachments:/data:ro -v "$PWD":/backup alpine \
     tar czf /backup/yuva-attachments-$(date +%F).tgz -C /data .
   ```

   Mit S3 nutzen Sie die Versionierung oder Replikation des Anbieters, oder kopieren Sie den Bucket
   mit `rclone sync`.

3. Bewahren Sie `YUVA_MASTER_KEY` getrennt von beidem auf ([Master-Key](install.md#master-key)).

Führen Sie die Schritte 1 und 2 per Cron in dieser Reihenfolge aus und kopieren Sie die Dateien vom
Host weg.

## Wiederherstellen

Verwenden Sie denselben `YUVA_MASTER_KEY`, mit dem das Backup erstellt wurde:

```sh
docker compose stop yuva
docker compose exec -T db sh -c 'dropdb -U yuva --force yuva && createdb -U yuva yuva'
docker compose exec -T db pg_restore -U yuva -d yuva --no-owner < yuva-2026-10-07.dump
docker run --rm -v yuva_attachments:/data -v "$PWD":/backup alpine \
  sh -c 'rm -rf /data/* && tar xzf /backup/yuva-attachments-2026-10-07.tgz -C /data'
docker compose start yuva
```

Ein Backup einer älteren Version wird beim Serverstart auf den aktuellen Stand migriert.

## Operator-Befehle

Sie laufen mit `docker compose exec yuva /yuva <command>` (`-T`, wenn Sie etwas hineinleiten).
`<ws>` ist eine Workspace-ID oder der exakte Name.

```sh
docker compose exec -T yuva /yuva api-key create --workspace Example --name provisioning
```

Befehle, die eine ID oder ein Secret ausgeben, schreiben nur das auf stdout.

| Befehl | Was er tut |
|---|---|
| `serve [--migrate=false]` | Startet den Server (der Standard), nach den Migrationen, außer mit `--migrate=false`. |
| `migrate up\|down\|status` | Spielt offene Migrationen ein, nimmt die letzte zurück oder listet sie auf. |
| `bootstrap --email <address> --workspace <name> [--name <name>] [--locale en\|tr] [--allow-existing]` | Legt einen Workspace und seinen ersten Inhaber an. `--allow-existing` fügt einen weiteren Workspace hinzu. |
| `api-key create --workspace <ws> --name <name> [--scope <scope>]... [--inbox <id>]...` | Legt einen API-Schlüssel an und gibt das Secret einmal aus. Standard: alle Scopes, alle Posteingänge. |
| `api-key list --workspace <ws>` | Listet Schlüssel auf, nie ihre Secrets. |
| `api-key revoke <id>` | Widerruft einen Schlüssel. |
| `inbox create --workspace <ws> --name <name> [--slug <slug>] [--locale <tag>] [--timezone <zone>] [--mode live\|async] [--expected-reply-minutes <n>]` | Legt einen Posteingang an und gibt seine ID aus. Standards: Slug aus dem Namen, `en`, `UTC`, `async`. |
| `inbox list --workspace <ws>` | Listet Posteingänge auf. |
| `inbox ses-tenant show --workspace <ws> [--inbox <id\|slug>]` | Gibt für jeden Posteingang den gespeicherten Amazon-SES-Tenant-Zustand, Sendestatus, Grund, zugeordnete Ressourcen und Kanäle aus. Ruft kein AWS auf. |
| `inbox ses-tenant sync (--workspace <ws> [--inbox <id\|slug>] \| --all) [--dry-run]` | Legt Tenants in AWS an und aktualisiert sie, liest danach ihren Status neu, eine Zeile pro Posteingang. `--all` ist der Nachtrag; `--dry-run` zeigt nur, was sich ändern würde. |
| `inbox ses-tenant reinstate --workspace <ws> --inbox <id\|slug>` | Hebt eine Pause auf, die von der Reputationsrichtlinie kam. |
| `channel create-email --workspace <ws> --inbox <id\|slug> --name <name> --address <address> [--display-name <name>] [--from-address <address>] [--smtp-host <host> [--smtp-port <n>] [--tls starttls\|tls\|none] [--smtp-username <name>] [--smtp-password-file <path\|->]]` | Legt einen E-Mail-Kanal an und gibt seine ID aus. Passwort aus einer Datei, oder mit `-` von stdin. |
| `channel create-chat --workspace <ws> --inbox <id\|slug> --name <name> --origin <origin>... [--allow-anonymous] [--ask-email-offline=false] [--greeting <text>]` | Legt einen Chat-Kanal (Web-Widget) an und gibt seine ID und seinen öffentlichen Schlüssel (`yuva_pk_…`) durch einen Tabulator getrennt aus. `--origin` für jede Website wiederholen. |
| `channel list --workspace <ws> --inbox <id\|slug>` | Listet die Kanäle eines Posteingangs auf, bei Chat-Kanälen mit öffentlichem Schlüssel und Origins. |
| `workspace delete --workspace <ws> --yes` | Löscht einen Workspace ([unten](#workspace-oder-konto-entfernen)). Ohne `--yes` ein Probelauf. |
| `workspace broadcasts approve\|deny\|suspend --workspace <ws> [--reason <text>]` | Beantwortet die Broadcast-Freigabe eines Workspaces oder sperrt ihn ([unten](#broadcasts-auf-einem-gehosteten-server)). |
| `workspace broadcasts limit --workspace <ws> --daily <n>` | Setzt die Empfänger pro UTC-Tag des Workspaces; `0` hebt das Limit auf. |
| `workspace broadcasts show --workspace <ws>` | Gibt Status, Limit, 30-Tage-Raten und die letzten Broadcasts aus. |
| `analytics property create\|list`, `analytics geoip-update`, `workspace analytics-limit` | Websites für [Analytics](analytics.md), seine Standortdatenbank und das monatliche Event-Limit eines Workspaces ([Konfiguration](configuration.md#analytics)). |
| `person delete --email <address> --yes` | Löscht das Konto einer Person. Abgelehnt, solange sie einziger Inhaber eines Workspaces ist. |
| `ingest-email --to <address> [--from <address>]` | Stellt eine Rohnachricht von stdin zu ([E-Mail](email.md#beliebiger-mta-yuva-ingest-email)). |
| `vapid-keys` | Gibt ein neues Web-Push-Schlüsselpaar aus. Braucht keine Datenbank. |

Alles andere (Chat- und App-Kanäle, Mitglieder, Webhooks, Identitäts-Secrets) verwalten Sie im Panel
oder über `/v1`.

## Broadcasts auf einem gehosteten Server

Wie [Broadcasts](broadcasts.md) begrenzt werden, bestimmt `YUVA_BROADCASTS` ([Konfiguration](configuration.md#broadcasts)).
Bei `on` gibt es nichts zu tun. Bei `approval` (der Standard bei `YUVA_SIGNUP=open`) kann ein über die
Anmeldung angelegter Workspace erst Broadcasts planen, wenn Sie ihn freigeben, und jeder Workspace
wärmt sich ab 500 Empfängern am Tag auf, die sich nach jedem sauberen Tag verdoppeln. Workspaces, die
Sie vorher oder nachher mit `yuva bootstrap` anlegen, sind `allowed`, wärmen sich aber ebenfalls auf.

Setzen Sie `YUVA_OPERATOR_EMAIL`, damit Sie es erfahren. Der Digest (höchstens eine E-Mail alle 5
Minuten) listet, ohne Mailinhalt und ohne Adressen der Empfänger:

| Zeile | Wann |
|---|---|
| Broadcast approval requested | Ein Owner hat gefragt: Workspace-Name, Adresse der Person, Website, wer was sendet, wie die Abonnenten zugestimmt haben. |
| Workspace suspended from broadcasts | Der Server hat ihn wegen seiner 30-Tage-Beschwerde- oder Bounce-Rate gesperrt, mit den Raten. |
| Broadcast paused | Ein Broadcast hat sich selbst pausiert (Bounce-Rate, Beschwerderate oder Absenderfehler), mit Broadcast-ID und Grund. |
| SES tenant paused | Amazon SES hat den Tenant eines Posteingangs pausiert: Namen von Workspace und Posteingang, wer pausiert hat und der Grund von SES. |

Ein Workspace wird gesperrt, wenn in den letzten 30 Tagen mindestens 0,3 % von mindestens 1.000
Broadcast-Mails als Spam gemeldet wurden oder mindestens 8 % von mindestens 2.000 gebounct sind. Seine
laufenden Broadcasts pausieren.

Antworten Sie mit der CLI. Ein Workspace ist ein Name oder eine ID, und `--reason` (bis zu 500 Zeichen)
wird den Ownern des Workspaces angezeigt:

```sh
docker compose exec -T yuva /yuva workspace broadcasts approve --workspace Example
docker compose exec -T yuva /yuva workspace broadcasts deny --workspace Example --reason "No subscribe form on the site"
docker compose exec -T yuva /yuva workspace broadcasts suspend --workspace Example --reason "Complaints"
docker compose exec -T yuva /yuva workspace broadcasts limit --workspace Example --daily 2000
docker compose exec -T yuva /yuva workspace broadcasts show --workspace Example
```

| Befehl | Wirkung |
|---|---|
| `approve` | Der Workspace darf senden. Hebt auch eine Sperre auf; Broadcasts, die pausiert haben, bleiben pausiert, bis ein Mitglied sie fortsetzt. |
| `deny` | Die Anfrage wird mit Nein beantwortet; der Workspace kann erneut anfragen. |
| `suspend` | Lehnt Planen und Fortsetzen ab und pausiert die laufenden Broadcasts (Grund `workspace_suspended`). |
| `limit --daily <n>` | Empfänger pro UTC-Tag des Workspaces über alle seine Broadcasts. Das Kleinste aus diesem Wert, `YUVA_BROADCAST_DAILY_RECIPIENTS` und dem Warm-up gilt. `0` hebt nur dieses Limit auf. |
| `show` | Status und Grund, Tageslimit und heutige Zahl, die letzten 30 Tage (gesendet, Bounce- und Beschwerderate), die genutzten Absender-Domains und die letzten 10 Broadcasts. |

Eine Sperre hebt nur der Operator auf. Schauen Sie vor einer Freigabe auf die Website und darauf, wie
die Abonnenten zugestimmt haben.

## Amazon-SES-Tenants auf einem gehosteten Server

Mehrere Workspaces, die über ein Amazon-SES-Konto senden, teilen sich dessen Reputation. SES-Tenants trennen sie:
Yuva führt **einen Tenant pro Posteingang**, SES verfolgt Reputation und Sendestopp pro Tenant, sodass schlechter
Versand eines Posteingangs nur diesen Posteingang pausiert. Einmal einrichten.

1. Verifizieren Sie Ihre Absenderdomains als Identitäten im SES-Konto (siehe [Zustellbarkeit](deliverability.md#amazon-ses)).
2. Legen Sie ein Configuration Set an und, um Bounces und Beschwerden zu erfahren, ein Event Destination auf einem
   SNS-Topic aus `YUVA_SES_TOPIC_ARNS` (siehe [E-Mail](email.md#bounces)). Geben Sie dem Set keine eigenen
   Suppression-Optionen, damit die des Tenants gelten.

   ```sh
   aws sesv2 create-configuration-set --configuration-set-name yuva --region REGION
   aws sesv2 create-configuration-set-event-destination --configuration-set-name yuva \
     --event-destination-name yuva-feedback --region REGION \
     --event-destination '{"Enabled":true,"MatchingEventTypes":["BOUNCE","COMPLAINT"],"SnsDestination":{"TopicArn":"arn:aws:sns:REGION:ACCOUNT:yuva-ses"}}'
   ```

3. Legen Sie für Yuva einen IAM-Benutzer mit dieser Richtlinie und einen Zugriffsschlüssel an. Ersetzen Sie
   `REGION`, `ACCOUNT`, `PREFIX` (der Wert von `YUVA_SES_TENANT_PREFIX`) und `CONFIG_SET`.

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "YuvaOwnTenants",
      "Effect": "Allow",
      "Action": [
        "ses:CreateTenant",
        "ses:GetTenant",
        "ses:DeleteTenant",
        "ses:TagResource",
        "ses:ListTenantResources",
        "ses:PutTenantSuppressionAttributes",
        "ses:UpdateReputationEntityCustomerManagedStatus"
      ],
      "Resource": "arn:aws:ses:REGION:ACCOUNT:tenant/PREFIX*"
    },
    {
      "Sid": "YuvaTenantResources",
      "Effect": "Allow",
      "Action": [
        "ses:CreateTenantResourceAssociation",
        "ses:DeleteTenantResourceAssociation"
      ],
      "Resource": [
        "arn:aws:ses:REGION:ACCOUNT:tenant/PREFIX*",
        "arn:aws:ses:REGION:ACCOUNT:identity/*",
        "arn:aws:ses:REGION:ACCOUNT:configuration-set/CONFIG_SET"
      ]
    },
    {
      "Sid": "YuvaReadAccount",
      "Effect": "Allow",
      "Action": [
        "ses:ListReputationEntities",
        "ses:ListEmailIdentities",
        "sts:GetAccessKeyInfo"
      ],
      "Resource": "*"
    }
  ]
}
```

   Yuva sendet nie mit diesem Schlüssel: Das tun die SMTP-Benutzer der Kanäle. `ses:TagResource` wird gebraucht,
   weil Tenants mit Tags angelegt werden. `ses:ListReputationEntities` lässt sich nur auf `*` erlauben; es ist
   schreibgeschützt, und Yuva fragt nur nach Tenants mit seinem Präfix. Die Richtlinie erreicht keine Tenants
   außerhalb des Präfixes.
4. Setzen Sie `YUVA_SES_REGION`, `YUVA_SES_ACCESS_KEY_ID`, `YUVA_SES_SECRET_ACCESS_KEY` und
   `YUVA_SES_CONFIGURATION_SET` ([Konfiguration](configuration.md#eingehende-e-mail)) und starten Sie neu. Alle vier oder keine.

Ein Kanal wird verwaltet, wenn sein SMTP-Host der Amazon-SES-Endpunkt von `YUVA_SES_REGION` ist und sein
Benutzername ein Zugriffsschlüssel desselben AWS-Kontos wie der Schlüssel des Servers. Kanäle über ein anderes
Konto oder eine andere Region bleiben unberührt. Für einen verwalteten Kanal legt der Server den Tenant des
Posteingangs an (mit Suppression von Bounces und Beschwerden auf Tenant-Ebene), ordnet das Configuration Set und
die Absenderidentität zu und fügt jeder Mail über den Kanal die Header hinzu. Das geschieht beim Speichern eines
Kanals und bei `workspace broadcasts approve`; beim Start ruft er AWS nie schreibend auf. Für bereits vorhandene
Kanäle führen Sie den Nachtrag einmal aus:

```sh
docker compose exec -T yuva /yuva inbox ses-tenant sync --all --dry-run
docker compose exec -T yuva /yuva inbox ses-tenant sync --all
```

Alle fünf Minuten liest der Server den Status all seiner Tenants. Wird einer deaktiviert, pausieren die laufenden
und geplanten Broadcasts des Posteingangs über verwaltete Kanäle (Grund `ses_tenant`; die anderen Posteingänge
senden weiter), Planen und Fortsetzen werden mit `409 ses_tenant_blocked` abgelehnt, Support-Antworten werden
weiter versucht und scheitern mit der Antwort von SES, und der Digest bekommt eine Zeile. Nichts setzt sich von
selbst fort. Wer einen Posteingang oder Workspace löscht, löscht auch seine Tenants.

`show` gibt aus, wer einen Tenant pausiert hat. Kam die Pause von der Richtlinie, setzt `reinstate` den
Reinstated-Status von SES, und ein Mitglied setzt die Broadcasts fort. Hat Amazon SES selbst pausiert, hebt nur
ein AWS-Support-Fall die Pause auf; `show` sagt das.

Optional, für eine Reaktion in Sekunden statt in fünf Minuten: Leiten Sie die Tenant-Ereignisse von SES an das
SNS-Topic. Das Ereignis lässt Yuva nur den Status neu lesen; sein Inhalt wird nicht vertraut.

```sh
aws events put-rule --name yuva-ses-tenants --region REGION \
  --event-pattern '{"source":["aws.ses"],"detail-type":["Sending Status Enabled","Sending Status Disabled","Advisor Recommendation Status Open","Advisor Recommendation Status Resolved"]}'
aws events put-targets --rule yuva-ses-tenants --region REGION --targets Id=yuva,Arn=arn:aws:sns:REGION:ACCOUNT:yuva-ses
```

Erlauben Sie EventBridge das Veröffentlichen im Topic mit dieser Anweisung in der Zugriffsrichtlinie des Topics:

```json
{
  "Sid": "AllowEventBridge",
  "Effect": "Allow",
  "Principal": { "Service": "events.amazonaws.com" },
  "Action": "sns:Publish",
  "Resource": "arn:aws:sns:REGION:ACCOUNT:yuva-ses"
}
```

## Workspace oder Konto entfernen

**Einen Workspace** löscht ein Inhaber unter **Settings → Workspace → Danger zone**, mit
`DELETE /v1/workspace` und `{"name": "<exact name>"}` oder mit `yuva workspace delete`. Er hört
sofort auf zu funktionieren; ein Hintergrundjob löscht danach seine Daten und schreibt
`workspace deletion`-Zeilen ins Log. Personen behalten ihre Konten.

**Ein Konto** löscht die Person selbst unter **Settings → My profile**, mit `DELETE /v1/me` und
`{"email": "<their address>"}` oder mit `yuva person delete`. Ihre Nachrichten bleiben ohne Autor
erhalten. Der einzige Inhaber eines Workspaces muss zuerst die Inhaberschaft übergeben oder den
Workspace löschen.

## Audit-Log

Das Audit-Log hält fest, wer Zugriff, Zugangsdaten und Daten eines Workspace geändert hat: wer beigetreten
ist, gegangen ist oder die Rolle gewechselt hat; Einladungen; erstellte oder widerrufene API-Schlüssel und
verbundene Apps; Webhooks und ihre Geheimnisse; Zugangsdaten und Schlüssel von E-Mail-Kanälen;
Postfachzugriff und Löschungen; gelöschte Kontakte; Exporte von Einwilligungs- und Analytics-Daten;
Änderungen der Aufbewahrung; und die Antworten des Betreibers zu Broadcasts. Jeder Eintrag hat den Akteur
(ein Mitglied, einen API-Schlüssel, ein Mitglied über eine verbundene App oder die Befehlszeile des
Servers), die Aktion wie `api_key.created`, ihr Ziel, einige Details und die Zeit. Ein Eintrag wird im
selben Schritt wie die Änderung geschrieben.

Nie enthalten sind Nachrichteninhalte, Geheimnisse, Passwörter, die URL eines Webhooks (nur sein Host) oder
Name und Adresse eines Kontakts. Das Löschen eines Workspace steht nicht in seinem Log, das mit dem
Workspace verschwindet; der Digest des Betreibers nennt es, wenn [`YUVA_OPERATOR_EMAIL`](configuration.md)
gesetzt ist.

Nur Inhaber lesen es: im Panel unter **Settings → Audit log** oder mit `GET /v1/audit-log` (Admins und
Agents erhalten `403 forbidden`). Ein API-Schlüssel liest es mit dem [Scope](headless.md#scopes)
`audit:read`, solange das Mitglied, das den Schlüssel erstellt hat, noch Inhaber ist.

```sh
curl -H "Authorization: Bearer $YUVA_KEY" \
  "$YUVA_URL/v1/audit-log?action=api_key.created&action=api_key.revoked&since=2026-01-01T00:00:00Z"
```

Neueste zuerst, 25 Einträge pro Seite (bis 100 mit `limit`), `next_cursor` für die nächste Seite.

| Filter | Zeigt |
|---|---|
| `actor_id` | Einträge dieses Mitglieds, Schlüssels oder dieser verbundenen App. Die ID eines Mitglieds passt auch auf das, was es über verbundene Apps getan hat. |
| `actor_kind` | `member`, `api_key`, `oauth_grant` oder `operator`. |
| `action` | Diese Aktionen; den Parameter wiederholen oder mit Kommas trennen. |
| `since`, `until` | Zeitraum: ab `since`, vor `until`. |

Einträge werden [`YUVA_AUDIT_DAYS`](configuration.md) Tage aufbewahrt (standardmäßig 365).

## Aufbewahrung

Unterhaltungen, Nachrichten und Dateien bleiben, bis ihr Kontakt gelöscht wird (im Panel,
`DELETE /v1/contacts/{contactId}` oder `DELETE /v1/contacts/by-external-id`). Ein Inhaber kann unter
**Settings → Workspace** oder mit `PATCH /v1/workspace` und `{"retention_days": 90}` eine
Aufbewahrungsfrist setzen; `null` behält alles (der Standard).

Yuva räumt einmal pro Stunde auf:

| Daten | Aufbewahrt |
|---|---|
| Ereignisse (Realtime-Replay und `GET /v1/events`) | 7 Tage |
| Kontaktsitzungen | bis sie ablaufen, 7 Tage nach der letzten Nutzung |
| Abgeschlossene Webhook-Zustellungen | 7 Tage; die neuesten 100 pro Endpunkt bleiben |
| Veraltete Einträge von Realtime-Verbindungen | 1 Stunde |
| Geschlossene Unterhaltungen mit Nachrichten und Dateien (wenn `retention_days` gesetzt ist) | so viele Tage seit ihrer letzten Änderung |
| Empfänger eines beendeten Broadcasts (seine Zahlen bleiben) | 400 Tage nach seinem Ende |
| Tägliche Versandsummen hinter den Limits | 90 Tage |
| [Audit-Log](#audit-log)-Einträge | `YUVA_AUDIT_DAYS` Tage (standardmäßig 365) |
| Original-`.eml` eingehender E-Mails (wenn `retention_days` gesetzt ist) | so viele Tage; die Nachricht selbst bleibt |

## Überwachung

| Endpunkt | Port | Antwortet |
|---|---|---|
| `GET /healthz` | 8080 | `200`, solange der Prozess läuft. Liveness-Check. |
| `GET /readyz` | 8080 | `200`, wenn die Datenbank erreichbar ist, sonst `503`. Readiness- und Uptime-Check. |
| `GET /v1/version` | 8080 | Die laufende Version. |
| `GET /metrics` | 9090 | Prometheus: `yuva_http_requests_total`, `yuva_http_request_seconds`, Go- und Prozessmetriken. |

Logs sind JSON-Zeilen auf stderr. Zeilen, die einen Alarm wert sind:

| Log-Meldung | Bedeutung |
|---|---|
| `mail not sent` | Das SMTP-Konto des Servers schlägt fehl; Anmeldecodes kommen nicht an. |
| `email send failed, retrying` | Das SMTP-Konto eines Kanals schlägt fehl. |
| `YUVA_SMTP_HOST is not set` (ebenso für `YUVA_INGRESS_SECRET`, `YUVA_VAPID_PUBLIC_KEY`) | Eine Funktion ist per Konfiguration abgeschaltet. |
| `not ready` | Die Datenbank ist nicht erreichbar. |

## Tracing

Yuva exportiert OpenTelemetry-Traces per OTLP an jeden Collector oder Dienst, der sie annimmt (Jaeger, Tempo,
Grafana Cloud, Honeycomb und andere). Tracing ist aus und kostet nichts, bis du `OTEL_EXPORTER_OTLP_ENDPOINT`
setzt; die Variablen stehen unter [Konfiguration](configuration.md#tracing). Beim Start schreibt der Server
`OpenTelemetry traces are on`, beim Stoppen sendet er die letzten Spans.

Was getraced wird:

| Span | Name | Umfasst |
|---|---|---|
| HTTP-Anfrage | das Routenmuster, zum Beispiel `GET /v1/conversations/{id}` | Jede Anfrage mit Methode, Route und Statuscode. Ein eingehender `traceparent`-Header wird fortgesetzt. |
| Hintergrundjob | `river.work/<kind>` | Jeder Lauf eines Jobs, verknüpft mit der Anfrage, die ihn eingereiht hat. |
| Datenbankabfrage | der Abfragename, zum Beispiel `GetConversation` | Abfragen während einer Anfrage oder eines Jobs. Das Polling der Job-Queue selbst wird nicht exportiert. |
| Mailversand | `mail.send`, `smtp.send` | Eine ausgehende Nachricht und ihre SMTP-Transaktion. |
| Webhook-Zustellung | `webhook.deliver` | Ein Zustellversuch. |

Nie aufgezeichnet werden: Nachrichtentexte, E-Mail-Adressen, Header-Werte, Tokens, Geheimnisse, URLs mit
Query-String, Client-Adressen, SQL-Text und Abfrageargumente sowie Fehlermeldungen. Ein Span trägt nur Attribute
aus einer festen Liste, die vor dem Export durchgesetzt wird; eine spätere Änderung in einer Bibliothek kann also
nichts hinzufügen:

- Anfrage: `http.request.method`, `http.route` (das Muster, nie der Pfad), `http.response.status_code`
- Workspace- und Posteingangs-IDs: `yuva.workspace_id`, `yuva.inbox_id`
- Jobs: `kind`, `queue`, `attempt`, `priority`, `id`, `status`, `messaging.system`
- Mail: `yuva.recipients` (eine Anzahl) und `yuva.message_bytes`
- Datenbank und Fehler: `db.system.name`, der SQL-Statuscode und der Ausnahmetyp

Logzeilen, die während einer getraceten Anfrage oder eines Jobs geschrieben werden, tragen `trace_id` und
`span_id`. Kopiere die ID aus einer Logzeile in dein Tracing-Backend, um den Trace zu öffnen.

Sampling: Standardmäßig wird jede Anfrage getraced. Um nur einen Teil zu behalten, setze den Sampler, zum Beispiel
`OTEL_TRACES_SAMPLER=parentbased_traceidratio` und `OTEL_TRACES_SAMPLER_ARG=0.1`.

Beispiel mit einem lokalen Collector, der die empfangenen Spans ausgibt:

```yaml
# otel-collector.yaml
receivers:
  otlp:
    protocols:
      http:
        endpoint: 0.0.0.0:4318
exporters:
  debug:
    verbosity: detailed
service:
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [debug]
```

```sh
docker run -d --name otel-collector -p 4318:4318 \
  -v "$PWD/otel-collector.yaml:/etc/otelcol/config.yaml" \
  otel/opentelemetry-collector:latest
```

Füge Folgendes zur `.env` hinzu und starte den Server neu:

```sh
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
```

Öffne eine beliebige Seite des Panels und lies das Log des Collectors (`docker logs otel-collector`): ein Span pro
Anfrage, benannt nach der Route. Läuft Yuva ebenfalls in Docker, nimm statt `localhost` den Dienstnamen des Collectors.
