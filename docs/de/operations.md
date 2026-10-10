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
| `channel create-email --workspace <ws> --inbox <id\|slug> --name <name> --address <address> [--display-name <name>] [--from-address <address>] [--smtp-host <host> [--smtp-port <n>] [--tls starttls\|tls\|none] [--smtp-username <name>] [--smtp-password-file <path\|->]]` | Legt einen E-Mail-Kanal an und gibt seine ID aus. Passwort aus einer Datei, oder mit `-` von stdin. |
| `channel list --workspace <ws> --inbox <id\|slug>` | Listet die Kanäle eines Posteingangs auf. |
| `workspace delete --workspace <ws> --yes` | Löscht einen Workspace ([unten](#workspace-oder-konto-entfernen)). Ohne `--yes` ein Probelauf. |
| `person delete --email <address> --yes` | Löscht das Konto einer Person. Abgelehnt, solange sie einziger Inhaber eines Workspaces ist. |
| `ingest-email --to <address> [--from <address>]` | Stellt eine Rohnachricht von stdin zu ([E-Mail](email.md#beliebiger-mta-yuva-ingest-email)). |
| `vapid-keys` | Gibt ein neues Web-Push-Schlüsselpaar aus. Braucht keine Datenbank. |

Alles andere (Chat- und App-Kanäle, Mitglieder, Webhooks, Identitäts-Secrets) verwalten Sie im Panel
oder über `/v1`.

## Workspace oder Konto entfernen

**Einen Workspace** löscht ein Inhaber unter **Settings → Workspace → Danger zone**, mit
`DELETE /v1/workspace` und `{"name": "<exact name>"}` oder mit `yuva workspace delete`. Er hört
sofort auf zu funktionieren; ein Hintergrundjob löscht danach seine Daten und schreibt
`workspace deletion`-Zeilen ins Log. Personen behalten ihre Konten.

**Ein Konto** löscht die Person selbst unter **Settings → My profile**, mit `DELETE /v1/me` und
`{"email": "<their address>"}` oder mit `yuva person delete`. Ihre Nachrichten bleiben ohne Autor
erhalten. Der einzige Inhaber eines Workspaces muss zuerst die Inhaberschaft übergeben oder den
Workspace löschen.

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
