# Betrieb

So aktualisieren, sichern, wiederherstellen und überwachen Sie einen Yuva-Server. Die Beispiele
verwenden das Compose-Setup aus der [Installationsanleitung](install.md) und laufen in dessen
Verzeichnis.

## Update auf 0.1.0

Beim Start von 0.1.0 ändern sich vier Dinge. Lesen Sie sie, und folgen Sie dann den Schritten unter [Updates](#updates).

### E-Mail-Adressen müssen nachgewiesen werden

Ab 0.1.0 empfängt ein E-Mail-Kanal E-Mails an seiner eigenen Adresse (ein MX-Eintrag, der auf Yuva zeigt,
`/ingress/email` oder `yuva ingest-email --to`) erst, nachdem sein Workspace die Adresse nachgewiesen hat.
Nichts wird übernommen: Jeder vorhandene E-Mail-Kanal beginnt nicht nachgewiesen, denn gespeicherte
E-Mails können die Zustellung per MX nicht von der Zustellung über eine Weiterleitungsadresse unterscheiden,
an die sich jeder selbst senden kann.

| Ein nicht nachgewiesener Kanal | |
|---|---|
| Hört auf | E-Mails nach seiner eigenen Adresse zu routen, bei Kanälen, deren Domain-MX auf Yuva zeigt. Ein Catch-all (`*@domain`) routet nichts. |
| Funktioniert weiter | E-Mails an seine [Weiterleitungsadresse](email.md#weiterleitungsadresse) (`<token>@YUVA_FORWARDING_DOMAIN`), die Weiterleitung von Gmail oder Outlook läuft also weiter. Senden über sein SMTP-Konto. |
| Wird abgelehnt | Ein Catch-all auf `YUVA_FORWARDING_DOMAIN` oder bei einem öffentlichen Mail-Anbieter wie gmail.com. Er empfängt nichts; ändern Sie seine Adresse. |

Der Server meldet Ihnen jeden Kanal einmal: im Digest des Operators unter „E-mail address to prove“, wenn
[`YUVA_OPERATOR_EMAIL`](configuration.md#anmeldung-und-passkeys) gesetzt ist, sonst als Warnung im Log
(`e-mail channel address must be proven`). Jede nennt den Workspace und den Kanal.

Weisen Sie einen Kanal auf einem von drei Wegen nach:

- **DNS.** Der Workspace legt einen TXT-Eintrag `_yuva-mail.<domain>` mit einem Wert an, der mit
  `yuva-mail-verify=` beginnt (beides steht in `email.verification` des Kanals), und lässt Yuva ihn prüfen.
  Ein Eintrag weist alle Kanäle dieses Workspace auf der Domain nach. Nur so lässt sich ein Catch-all
  nachweisen.
- **Code.** Der Workspace lässt einen Code an die Adresse senden und sendet ihn zurück. Leitet das Postfach
  an die Weiterleitungsadresse des Kanals weiter, weist die Code-E-Mail den Kanal von selbst nach.
- **Sie.** Wenn Sie wissen, dass der Kanal zu seinem Workspace gehört, bürgen Sie dafür. Ohne `--yes` listet
  der Befehl nur auf, was er nachweisen würde:

  ```sh
  docker compose exec yuva /yuva channel prove --all-unproven --workspace <ws>
  docker compose exec yuva /yuva channel prove --all-unproven --workspace <ws> --yes --plan <code>
  docker compose exec yuva /yuva channel prove --channel <id> --yes --address support@example.com
  docker compose exec yuva /yuva channel prove --channel support@example.com --workspace <ws> --yes
  ```

  Die Liste zeigt jeden Kanal, Workspace, jede Adresse und jeden Namen mit einem Ergebnis: `would prove`,
  `already proven` oder `refused: …`. Solange etwas nachgewiesen würde, endet der Befehl mit einem Fehler und
  gibt den Plan-Code aus. `--yes` muss wiederholen, was Ihnen gezeigt wurde: `--plan <code>` bei
  `--all-unproven` (ein Code der aufgelisteten Kanäle und Adressen, eine geänderte Liste wird also
  abgelehnt) und `--address` bei einem Kanal, der per ID angegeben ist. `--workspace` ist bei
  `--all-unproven` nötig und wenn mehrere Workspaces dieselbe Adresse beanspruchen. Der Nachweis steht im
  [Audit-Log](#audit-log) des Workspace als `channel.address_proven` durch `operator`, und der Kanal zeigt
  die Methode `operator`.

  Abgelehnt wird er bei einer Adresse, die ein anderer Workspace nachgewiesen hat, bei einem Catch-all auf
  einer gemeinsam genutzten Domain und wenn der Nachweis einem nachgewiesenen Kanal eines anderen Workspace
  E-Mails wegnehmen würde („would take from workspace …“). Der Operator nimmt niemandem eine Adresse weg.

Um eine Lücke zu vermeiden, weisen Sie die Kanäle, die per MX empfangen, gleich nach dem Update nach, oder
bitten Sie die Workspaces, vorher ihre TXT-Einträge anzulegen (der Wert erscheint am Kanal, sobald 0.1.0
läuft). Workspaces weisen Adressen über die API nach, siehe [E-Mail](email.md#adresse-nachweisen).

### Signaturen und automatische Antworten

- **`YUVA_INGRESS_ACCEPT_V2` ist jetzt standardmäßig aus.** Der Server akzeptiert nur `v3`-Signaturen. Der
  Email Worker in `edge/` signiert `v3`; läuft bei Ihnen noch ein älterer, deployen Sie ihn vor dem Update
  neu oder setzen Sie bis dahin `YUVA_INGRESS_ACCEPT_V2=true`. Mit `v2` signierte E-Mails werden sonst mit
  `401` abgelehnt, und der Worker leitet sie an `FALLBACK_FORWARD` weiter, oder der sendende Server versucht
  es erneut.
- **Automatische Antworten brauchen DMARC.** Begrüßungen, Regelantworten und Bewertungsanfragen gehen nur
  an Absender, deren E-Mail laut [`YUVA_INGRESS_AUTHSERV_ID`](configuration.md#eingehende-e-mail) DMARC
  bestanden hat. Ein Server ohne diese Einstellung sendet keine davon und warnt beim Start. Setzen Sie sie,
  bevor Sie sich darauf verlassen.

### E-Mails ohne SMTP

Ohne `YUVA_SMTP_HOST` sendet der Server keine E-Mails und schreibt sie geschwärzt ins Log. Ein Anmeldecode
ist eine Zeile, `sign-in code (log mailer, not sent)`, mit der Adresse als `*@domain` und dem `code`. Jede
andere E-Mail protokolliert nur die Domain des Empfängers und die Größen. `YUVA_MAIL_LOG=full` schreibt ganze
E-Mails (Empfänger, Betreff, Text) und wird beim Start abgelehnt, wenn `YUVA_PUBLIC_URL` keine Localhost-
oder Loopback-Adresse ist; nutzen Sie es nur auf Ihrem eigenen Rechner. Logzeilen und wiederholte
Sendefehler behalten nur die Domain einer Adresse (`*@example.com`); der Zustellfehler, den das Panel bei
einer fehlgeschlagenen Nachricht zeigt, enthält dagegen die volle Antwort des Mailservers.

### Job-Queues

Hintergrundarbeit läuft jetzt auf mehr Queues, damit ein langsamer Endpunkt eines Kunden Zuweisung oder
Benachrichtigungen nicht aufhalten kann. Ein Job, der einen von einem Workspace gewählten Server oder einen
anderen Dienst aufruft, läuft außerhalb der Queue `default`. Jeder Prozess führt alle aus; die Größen sind
fest, und es gibt nichts einzustellen.

| Queue | Worker pro Prozess | Führt aus |
|---|---|---|
| `default` | 10 | Zuweisung, Automatisierung, SLA-Prüfungen, Benachrichtigungen, Snooze-Aufwecken, Webhook-Fan-out, Aufbewahrung und alles, was nur die Datenbank liest und schreibt. |
| `webhooks` | 10 | Webhook-Zustellungen. |
| `outbound` | 10 | Web Push, App-Push (APNs und FCM), Slack-Beiträge und Issue-Tracker-Sync. |
| `email` | 10 | E-Mails über das SMTP-Konto eines Workspace: Antworten, Listen-E-Mails und Zustellbarkeitsprüfungen. |
| `imports` | 2 | Importe von Help Centern. |

Die Queues `broadcast`, `migrations` und `analytics` sind unverändert ([Konfiguration](configuration.md)).
Jobs, die vor dem Update in `default` eingereiht wurden, werden dort weiter abgearbeitet. So sehen Sie den
Rückstau pro Queue:

```sql
SELECT queue, state, count(*) FROM river_job
WHERE state IN ('available','running','retryable') GROUP BY 1, 2;
```

Ein wachsender Rückstau in `webhooks` bedeutet langsame oder fehlerhafte Kunden-Endpunkte; er hält nichts anderes mehr auf.

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
| `channel prove --channel <id\|address> [--workspace <ws>] [--yes [--address <address>]]` | Bürgt als Operator für die Adresse eines E-Mail-Kanals ([Update auf 0.1.0](#e-mail-adressen-müssen-nachgewiesen-werden)). Ohne `--yes` listet er auf, was er nachweisen würde. Abgelehnt bei einer Adresse, die ein anderer Workspace nachgewiesen hat, und bei Catch-alls auf gemeinsam genutzten Domains. |
| `channel prove --all-unproven --workspace <ws> [--yes --plan <code>]` | Dasselbe für jeden nicht nachgewiesenen E-Mail-Kanal des Workspace. `--plan` ist der Code, den die Liste ausgibt. |
| `workspace delete --workspace <ws> --yes` | Löscht einen Workspace ([unten](#workspace-oder-konto-entfernen)). Ohne `--yes` ein Probelauf. |
| `workspace broadcasts approve\|deny\|suspend --workspace <ws> [--reason <text>]` | Beantwortet die Broadcast-Freigabe eines Workspaces oder sperrt ihn ([unten](#broadcasts-auf-einem-gehosteten-server)). |
| `workspace broadcasts limit --workspace <ws> --daily <n>` | Setzt die Empfänger pro UTC-Tag des Workspaces; `0` hebt das Limit auf. |
| `workspace broadcasts show --workspace <ws>` | Gibt Status, Limit, 30-Tage-Raten und die letzten Broadcasts aus. |
| `analytics property create\|list`, `analytics geoip-update`, `workspace analytics-limit` | Websites für [Analytics](analytics.md), seine Standortdatenbank und das monatliche Event-Limit eines Workspaces ([Konfiguration](configuration.md#analytics)). |
| `person delete --email <address> --yes` | Löscht das Konto einer Person. Abgelehnt, solange sie einziger Inhaber eines Workspaces ist. |
| `ingest-email --to <address> [--from <address>]` | Stellt eine Rohnachricht von stdin zu ([E-Mail](email.md#beliebiger-mta-yuva-ingest-email)). |
| `master-key rotate\|check [--batch <n>]` | Versiegelt gespeicherte Geheimnisse unter dem neuen Master-Key neu oder meldet, was noch nicht darunter liegt ([Master-Key rotieren](#master-key-rotieren)). |
| `vapid-keys` | Gibt ein neues Web-Push-Schlüsselpaar aus. Braucht keine Datenbank. |

Alles andere (Chat- und App-Kanäle, Mitglieder, Webhooks, Identitäts-Secrets) verwalten Sie im Panel
oder über `/v1`.

## Master-Key rotieren

`YUVA_MASTER_KEY` versiegelt die Geheimnisse, die Yuva speichert (SMTP-Passwörter, DKIM-Schlüssel,
Webhook- und Integrations-Secrets, Slack-, GitHub- und Linear-Tokens, Client-Secrets für Single
Sign-on, Push-Zugangsdaten, Import- und Migrations-Zugangsdaten, Identitäts-Secrets, Analytics-
Besucherschlüssel) und die Links in den versendeten E-Mails (Abmelde-, Einstellungs-, Bestätigungs-,
Bewertungs- und Klick-Links). Rotieren Sie ihn in dem Rhythmus, den Ihre Richtlinie vorgibt. Währenddessen
läuft die E-Mail weiter und niemand wird abgemeldet. Wenn der Schlüssel durchgesickert sein könnte,
gehen Sie stattdessen nach [Wenn der Master-Key durchgesickert ist](#wenn-der-master-key-durchgesickert-ist) vor.

1. Starten Sie den Server einmal mit dieser Version nur mit dem aktuellen Schlüssel (jedes normale
   Update tut das). Er merkt sich den Opt-out-Schlüssel jedes Workspaces (siehe unten). Solange
   `YUVA_MASTER_KEY_PREVIOUS` gesetzt ist, verweigern der Server und `yuva master-key rotate` den
   Start, wenn ein Workspace Opt-outs, aber keinen gemerkten Schlüssel hat, und sagen das.
2. Erzeugen Sie einen neuen Schlüssel: `openssl rand -base64 32`.
3. Setzen Sie in jedem Prozess, der den Schlüssel hat (der Server, `yuva ingest-email` auf einem
   Mailhost, der Rechner, von dem Sie `yuva`-Befehle ausführen), den neuen Schlüssel als
   `YUVA_MASTER_KEY` und den alten als `YUVA_MASTER_KEY_PREVIOUS` und starten Sie neu. Neue Geheimnisse
   werden mit dem neuen Schlüssel versiegelt, alles unter dem alten bleibt lesbar. Solange
   `YUVA_MASTER_KEY_PREVIOUS` gesetzt ist, schreibt der Server beim Start eine Warnung.
4. Versiegeln Sie das Gespeicherte neu:

   ```sh
   docker compose exec yuva /yuva master-key rotate
   ```

   Der Befehl geht Spalte für Spalte und Workspace für Workspace in Stapeln vor (`--batch`, Standard
   500), schreibt pro Spalte eine Zeile (neu versiegelt, schon unter dem neuen Schlüssel, nicht
   lesbar) und kann jederzeit gestoppt und neu gestartet werden: Ein zweiter Lauf berührt nur den
   Rest. Ein Geheimnis, das jemand zwischenzeitlich im Panel ändert, wird nicht überschrieben. Am
   Ende prüft er alles noch einmal und endet mit einem Fehler samt Zeilenliste, wenn noch etwas unter
   dem alten Schlüssel liegt (etwa weil ein Prozess mit dem alten Schlüssel als `YUVA_MASTER_KEY`
   geschrieben hat): Beheben Sie diesen Prozess und führen Sie den Befehl erneut aus.
5. Bestätigen Sie:

   ```sh
   docker compose exec yuva /yuva master-key check
   ```

   Er schreibt nichts und endet mit einem Fehler, solange ein gespeicherter Wert noch unter dem alten
   Schlüssel liegt (er listet die Zeilen; führen Sie `rotate` erneut aus) oder mit keinem der beiden
   Schlüssel lesbar ist.
6. Behalten Sie `YUVA_MASTER_KEY_PREVIOUS`, solange Links in vor dem Wechsel versendeten E-Mails
   weiter funktionieren sollen: für Abmelde-Links in versendeten Broadcasts, solange Empfänger sie
   noch nutzen können, für
   Bewertungs- und Klick-Links so lange, wie sie zählen sollen. Entfernen Sie ihn dann, starten Sie
   neu und führen Sie `master-key check` ein letztes Mal aus.

Was sich ändert und was nicht:

- Opt-outs bleiben erhalten. Adressen, die sich abgemeldet oder beschwert haben, werden nur als
  geschlüsselter Hash gespeichert, der sich unter einem neuen Schlüssel nicht neu bilden lässt. Der
  Hash-Schlüssel jedes Workspaces wird deshalb versiegelt gespeichert und wie die anderen Geheimnisse
  neu versiegelt; die Widerspruchsliste übersteht jede Rotation. Wird der alte Schlüssel entfernt,
  bevor `rotate` gelaufen ist, verweigert der Server den Start, statt Menschen anzuschreiben, die
  widersprochen haben: Setzen Sie `YUVA_MASTER_KEY_PREVIOUS` wieder ein. Ein Zurückrollen (alter
  Schlüssel als `YUVA_MASTER_KEY`, neuer als `YUVA_MASTER_KEY_PREVIOUS`) funktioniert in jedem
  Stadium.
- Der Verifizierungs-TXT-Wert für E-Mail-Adressen (`_yuva-mail.<domain>`) ändert sich: Das Panel
  zeigt sofort den neuen Wert. Eine bereits nachgewiesene Adresse bleibt nachgewiesen, und ein
  Eintrag mit dem alten Wert weist eine Adresse weiter nach, solange `YUVA_MASTER_KEY_PREVIOUS`
  gesetzt ist.
- Rate Limits beginnen von vorn: Ihre Schlüssel werden aus dem Master-Key abgeleitet.
- Bereits versendete Codes (Adress-Codes, Slack-Link-Codes), bereits eingegebene
  Analytics-Freigabepasswörter und laufende Migrations-Testnachrichten funktionieren weiter, solange
  der alte Schlüssel gesetzt ist.
- Ein Wert, der mit keinem der beiden Schlüssel lesbar ist, wird mit Tabelle, Workspace und Zeile
  aufgeführt. Nichts kann ihn lesen: Tragen Sie das Geheimnis im Panel neu ein oder löschen Sie die
  Integration.
- Ein Backup von vor dem Wechsel braucht den alten Schlüssel. Bewahren Sie ihn so lange auf wie
  diese Backups ([Backups](#backups)).
- Es wird nur ein alter Schlüssel akzeptiert. Beenden Sie eine Rotation (`check` besteht, alter
  Schlüssel entfernt), bevor Sie die nächste beginnen.

## Wenn der Master-Key durchgesickert ist

Wer den Schlüssel und eine Kopie der Datenbank hat, kann jedes gespeicherte Geheimnis lesen und die
Links in Ihren E-Mails fälschen. Gehen Sie der Reihe nach vor.

1. Rotieren Sie: Schritte 1 bis 5 unter [Master-Key rotieren](#master-key-rotieren). Führen Sie
   `master-key check` aus, bis er besteht.
2. Entfernen Sie `YUVA_MASTER_KEY_PREVIOUS` sofort und starten Sie neu. Abmelde-, Einstellungs-,
   Bestätigungs-, Bewertungs- und Klick-Links in vor dem Wechsel versendeten E-Mails funktionieren
   dann nicht mehr, weil jeder mit dem alten Schlüssel sie hätte fälschen können. Abmelden können
   sich Menschen weiterhin über die nächste E-Mail.
3. Ändern Sie die Geheimnisse selbst an ihrer Quelle, denn der durchgesickerte Schlüssel konnte sie
   öffnen: SMTP-Passwörter (auch das des Broadcast-SMTP-Kontos), Slack-, GitHub- und Linear-Tokens,
   Zugangsdaten für Help-Desk- und Postfach-Migrationen, Client-Secrets für Single Sign-on,
   APNs-Schlüssel und FCM-Dienstkonten. Rotieren Sie Webhook-Signatur-Secrets und
   Identitäts-Secrets der Posteingänge in Yuva und informieren Sie die Empfänger und die Apps, die
   damit signieren. Erzeugen Sie für jede Absenderdomain einen neuen DKIM-Schlüssel und
   veröffentlichen Sie seinen Eintrag.
4. Opt-out-Hashes behalten ihren Schlüssel über eine Rotation hinweg (so gewollt), der alte
   Schlüssel leitet ihn also weiterhin ab: Wer den alten Schlüssel und die Datenbank hat, kann
   prüfen, ob eine bestimmte Adresse den E-Mails eines Workspaces widersprochen hat. Die Adressen
   auflisten kann er nicht. Betrachten Sie das zusammen mit der Datenbank als offengelegt.
5. Sitzungen, API-Schlüssel und Kontakt-Sitzungen werden als ungeschlüsselte Hashes gespeichert und
   hängen nicht vom Master-Key ab. Widerrufen Sie sie nur dann gesondert, wenn die Datenbank selbst
   durchgesickert ist.

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
verbundene Apps; Webhooks und ihre Geheimnisse; Zugangsdaten und Schlüssel von E-Mail-Kanälen; nachgewiesene oder verlorene E-Mail-Adressen;
Postfachzugriff und Löschungen; in den Papierkorb gelegte, wiederhergestellte oder gelöschte Unterhaltungen; entfernte Nachrichten (`message.redacted`: die Unterhaltung, ob Nachricht oder Notiz, wie viele Dateien, nie der Text); gelöschte Kontakte; Exporte von Einwilligungs- und Analytics-Daten;
Änderungen der Aufbewahrung; ob API-Schlüssel und Assistenten direkt an Kunden senden oder Broadcasts planen dürfen; und die Antworten des Betreibers zu Broadcasts. Jeder Eintrag hat den Akteur
(ein Mitglied, einen API-Schlüssel, ein Mitglied über eine verbundene App oder die Befehlszeile des
Servers), die Aktion wie `api_key.created`, ihr Ziel, einige Details und die Zeit. Ein Eintrag wird im
selben Schritt wie die Änderung geschrieben.

Nie enthalten sind Nachrichteninhalte, Geheimnisse, Passwörter, die URL eines Webhooks (nur sein Host) oder
Name und Adresse eines Kontakts. Das Löschen eines Workspace steht nicht in seinem Log, das mit dem
Workspace verschwindet; der Digest des Betreibers nennt es, wenn [`YUVA_OPERATOR_EMAIL`](configuration.md)
gesetzt ist.

Team-Änderungen werden als `team.created`, `team.deleted`, `team.member_added`, `team.member_removed`,
`team.inbox_access_granted` und `team.inbox_access_revoked` erfasst.

Nachweise von E-Mail-Adressen werden als `channel.address_verified` (`address`, `method`), `channel.address_lost`
(`address`; durch das System, wenn ein anderer Workspace die Adresse später nachwies) und
`channel.address_proven` (durch den Operator, [oben](#e-mail-adressen-müssen-nachgewiesen-werden)) erfasst. Die
beiden Einstellungen werden als `workspace.bots_may_send_changed` und `workspace.broadcasts_by_keys_changed` mit
`from` und `to` erfasst, nur wenn sich der Wert ändert.

Änderungen an Automatisierungsregeln werden als `automation_rule.created`, `automation_rule.updated` und
`automation_rule.deleted` erfasst ([Automatisierungsregeln](automation.md#10-zugriff)).

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
| Unterhaltungen im [Papierkorb](headless.md#unterhaltungen-löschen) mit Nachrichten und Dateien | 30 Tage |
| Geschlossene Unterhaltungen mit Nachrichten und Dateien (wenn `retention_days` gesetzt ist) | so viele Tage seit ihrer letzten Änderung |
| Empfänger eines beendeten Broadcasts (seine Zahlen bleiben) | 400 Tage nach seinem Ende |
| Tägliche Versandsummen hinter den Limits | 90 Tage |
| [Audit-Log](#audit-log)-Einträge | `YUVA_AUDIT_DAYS` Tage (standardmäßig 365) |
| Original-`.eml` eingehender E-Mails (wenn `retention_days` gesetzt ist) | so viele Tage; die Nachricht selbst bleibt |

Wann immer Unterhaltungen endgültig gelöscht werden (ein Kontakt, ein Posteingang, Aufbewahrung, die
Papierkorb-Bereinigung oder ein Workspace), löscht derselbe Schritt auch ihre gespeicherten Ereignisse und jede
Webhook-Zustellung, ob ausstehend oder aufbewahrt, die sie enthält: `/v1/events`, Realtime-Replay und das
Zustellprotokoll zeigen den Inhalt sofort nicht mehr, und eine ausstehende Zustellung wird nicht wiederholt. Die
gespeicherten Dateien (Anhänge, rohe E-Mails, E-Mail-Bilder, Import-Uploads) löscht ein Job, der es wiederholt,
bis der Speicher antwortet; ein fehlerhafter Speicher hinterlässt also keine Dateien, auf die nichts mehr zeigt.

## Mehrere Serverprozesse

Beliebig viele Serverprozesse können sich eine Datenbank teilen (mehr `replicaCount` im Helm-Chart
oder mehrere Container hinter einem Proxy). Sie koordinieren sich über Postgres:

- **Rate Limits gelten über Prozesse hinweg.** Jedes Limit (Sitzungsstarts, Widget-Schreibzugriffe,
  OAuth-Freigaben und MCP-Tokens, öffentliche Seiten, Aufrufe von Status-Monitoren, Analytics-Erfassung,
  ...) zählt in der Tabelle `rate_limits`, zehn Prozesse erlauben also so viel wie einer, egal welchen
  Prozess eine Anfrage erreicht. Die 4 Analytics-Abfragen, die ein Workspace gleichzeitig ausführen darf,
  sind Leases in `rate_slots` und werden ebenso geteilt.
- Beide Tabellen sind `UNLOGGED`: Ein Postgres-Absturz oder ein Failover auf einen Standby leert sie,
  wodurch nur das Fenster jedes Limits neu beginnt. Backups müssen sie nicht enthalten
  (`pg_dump --no-unlogged-table-data` lässt ihre Zeilen weg).
- Schlüssel sind Hashes mit Schlüssel (HMAC unter dem Master-Key), nie Client- oder E-Mail-Adressen.
- Der Job `rate_limit_sweep` löscht abgelaufene Fenster und Leases jede Minute.
- Fenster und Leases werden mit der Uhr der Datenbank gemessen, die Uhren der Prozesse spielen für die
  Limits keine Rolle.
- Limits, die alle Besucher eines Chat-Kanals, Help Centers oder einer Seite teilen, werden pro Netz
  aufgeteilt. Nach dem Limit pro Adresse (IPv4 /32, IPv6 /64) darf ein Netz (IPv4 /24, IPv6 /48) nur einen
  Teil des gemeinsamen Budgets nutzen, sodass eine Flut aus wenigen Quellen niemanden sonst aussperrt:

  | Gemeinsames Budget | Ein Netz | Alle Besucher |
  |---|---|---|
  | Sitzungsstarts eines Kanals | 120 pro Minute | 6.000 pro Minute |
  | Schreibzugriffe der Besucher eines Kanals | 240 pro Minute | 12.000 pro Minute |
  | Help-Center-Suche | 120 pro Minute | 12.000 pro Minute |
  | Listenanmeldungen eines Kanals | 40 pro Stunde | 6.000 pro Stunde |
  | Statusseiten-Anmeldungen | 40 pro Stunde | 3.000 pro Stunde |
  | Lesezugriffe auf Neuigkeiten | 480 pro Minute | 24.000 pro Minute |

- Schlägt eine Prüfung in der Datenbank fehl (Verbindungsverlust, Timeout), sperrt das nie ein angemeldetes
  Mitglied, einen API-Schlüssel oder eine verbundene App aus: Diese Anfragen gehen durch. Jede andere Anfrage
  (Besucher, Kontakte) wird mit denselben Limits im Speicher des Prozesses gezählt, bis die Datenbank
  antwortet; der Server protokolliert dann höchstens einmal pro Minute
  `rate limit check failed; counting anonymous requests in this process`.
- [`YUVA_CLIENT_IP_HEADER` und `YUVA_TRUSTED_PROXIES`](configuration.md) müssen in jedem Prozess gleich
  sein, sonst zählen die Limits pro Adresse unterschiedliche Adressen.

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
| `e-mail channel address must be proven` | Ein E-Mail-Kanal ist nicht nachgewiesen, und `YUVA_OPERATOR_EMAIL` ist nicht gesetzt, um davon zu erfahren ([Update auf 0.1.0](#e-mail-adressen-müssen-nachgewiesen-werden)). |
| `rate limit check failed` | Die Datenbank konnte Rate Limits nicht zählen ([Mehrere Serverprozesse](#mehrere-serverprozesse)). |
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
