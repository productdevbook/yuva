# Slack

Yuva kann die neuen Unterhaltungen eines Posteingangs und die Notizen, die Mitglieder erwähnen, in
Slack-Kanäle posten. Mitglieder antworten im Thread des Posts: Eine Antwort wird zur Notiz in der
Unterhaltung, und eine Antwort, die mit `!reply` beginnt, geht an den Kunden.

## Die Slack-App anlegen

Das erledigt der Betreiber des Servers einmal; jeder Workspace auf dem Server verbindet sich über
dieselbe App.

1. Wählen Sie unter [api.slack.com/apps](https://api.slack.com/apps) **Create New App** › **From a
   manifest**, wählen Sie einen Slack-Workspace zum Entwickeln und fügen Sie dieses Manifest ein,
   mit der Adresse Ihres Servers statt `support.example.com`:

   ```yaml
   display_information:
     name: Yuva
   features:
     bot_user:
       display_name: Yuva
       always_online: true
   oauth_config:
     redirect_urls:
       - https://support.example.com/slack/oauth/callback
     scopes:
       bot:
         - chat:write
         - channels:read
         - groups:read
         - channels:history
         - groups:history
         - im:write
         - users:read
         - users:read.email
   settings:
     event_subscriptions:
       request_url: https://support.example.com/ingress/slack
       bot_events:
         - message.channels
         - message.groups
         - app_uninstalled
         - tokens_revoked
     org_deploy_enabled: false
     socket_mode_enabled: false
     token_rotation_enabled: false
   ```

2. Kopieren Sie unter **Basic Information** die Client ID, das Client Secret und das Signing
   Secret in die Konfiguration des Servers und starten Sie ihn neu:

   ```sh
   YUVA_SLACK_CLIENT_ID=1234567890.1234567890
   YUVA_SLACK_CLIENT_SECRET=...
   YUVA_SLACK_SIGNING_SECRET=...
   ```

3. Slack prüft die Request-URL, sobald der Server dort antwortet; wurde sie gespeichert, bevor der
   Server das Signing Secret hatte, wiederholen Sie die Prüfung auf der Seite **Event
   Subscriptions** der App.
4. Damit sich auch Workspaces außerhalb des Entwicklungs-Workspace verbinden können, schalten Sie
   **Manage Distribution** › **Public Distribution** ein.

Wofür die Scopes da sind: `chat:write` postet und antwortet, `channels:read` und `groups:read`
listen die Kanäle, in denen der Bot ist, `channels:history` und `groups:history` liefern die
Antworten in Threads, `im:write` sendet den Verknüpfungscode, und `users:read` und
`users:read.email` finden den Slack-Benutzer eines Mitglieds über die E-Mail-Adresse. Yuva liest
keinen Kanalverlauf; es erhält nur die Nachrichten, die Slack als Ereignisse sendet, und reagiert
nur auf Antworten in Threads, die es selbst gepostet hat.

Ohne die drei Variablen ist Slack aus: Das Panel bietet es nicht an, und die API antwortet
`409 slack_not_configured`.

## Einen Workspace verbinden

Ein Owner oder Admin öffnet Einstellungen › Slack, wählt **Connect Slack** und erlaubt die App in
Slack. Ein Slack-Workspace ist jeweils mit einem Yuva-Workspace verbunden. Das Bot-Token wird mit
dem Master-Key des Servers verschlüsselt gespeichert.

Laden Sie den Bot in jeden Kanal ein, in dem er posten soll: `/invite @Yuva` in diesem Kanal.
Private Kanäle funktionieren genauso.

## Kanäle pro Posteingang wählen

Wählen Sie in den Einstellungen eines Posteingangs:

- einen Kanal für **neue Unterhaltungen** (die erste Nachricht eines Kontakts) und
- einen Kanal für **Erwähnungen** (eine Notiz, die Mitglieder erwähnt; ein verknüpftes Mitglied
  wird auch in Slack erwähnt) und
- einen Kanal für **SLA** (eine Unterhaltung verletzt ein SLA-Ziel; schalten Sie **SLA warnings**
  ein, um auch beim Erreichen der Warnung zu posten). Der Post nennt das Ziel (erste Antwort,
  nächste Antwort oder Lösung), wie spät es ist (oder wie viel Zeit bleibt), die zugewiesene Person
  oder das Team, den Betreff, den Kontakt und einen Link.

Ein Post enthält den Betreff, den Namen des Kontakts, den Posteingang und einen Link zur
Unterhaltung. Die Nachricht selbst bleibt in Yuva, außer der Posteingang schaltet **Include message
text** ein; dann zitiert der Post auch ihre ersten 300 Zeichen. Eine Erwähnung oder ein SLA-Post geht in den Thread der
Unterhaltung, wenn der Kanal schon einen hat; eine Antwort dort funktioniert wie in jedem anderen Thread.

Über die API: `PUT /v1/inboxes/{id}/slack` mit `new_conversations_channel_id`,
`mentions_channel_id`, `sla_channel_id`, `sla_warnings` und `include_content`; `GET /v1/slack/channels` listet die Kanäle, in denen
der Bot ist.

## Ihren Slack-Benutzer verknüpfen

Jedes Mitglied verknüpft seinen eigenen Slack-Benutzer im Profil: Yuva findet den Slack-Benutzer
mit der E-Mail-Adresse des Mitglieds (oder einer anderen, die Sie eingeben), und der Yuva-Bot
schickt ihm in Slack einen sechsstelligen Code, der 10 Minuten gilt. Mit dem Code werden beide
verknüpft. Ein Slack-Benutzer gehört jeweils zu einem Mitglied.

## Aus Slack antworten

Im Thread eines Posts:

- jede Antwort wird zu einer **Notiz** in der Unterhaltung, geschrieben vom verknüpften Mitglied;
- eine Antwort, die mit `!reply` beginnt, geht als Antwort dieses Mitglieds an den **Kunden**, auf
  dem üblichen Weg (E-Mail, Chat oder App). Ein Owner oder Admin kann einen anderen Befehl wählen,
  etwa `!send`; er darf nicht mit `/` beginnen, das Slack für eigene Befehle nutzt.

Beides erscheint in Yuva als „via Slack“. Links behalten ihre Adresse; Erwähnungen verknüpfter
Slack-Benutzer werden zu den Namen der Mitglieder.

Für einen Slack-Benutzer, der kein Mitglied verknüpft hat oder dessen Mitglied den Posteingang der
Unterhaltung nicht sehen kann, ändert sich nichts: Er erhält eine Nachricht, die nur er sieht und
die den Grund nennt. Bearbeitungen und Löschungen in Slack werden nicht übernommen.

## Schwärzen, Papierkorb und Löschen

Slack behält, was Yuva gepostet hat; deshalb bearbeitet Yuva den Beitrag nachträglich:

- Wird eine Nachricht geschwärzt (Redaction), ersetzt Yuva die Beiträge, die ihren Text
  enthielten, durch "Message removed.";
- wird eine Unterhaltung in den Papierkorb verschoben, ersetzt Yuva alle ihre Beiträge durch "This
  conversation is in the trash."; Wiederherstellen bringt den Text nicht zurück;
- wird eine Unterhaltung endgültig gelöscht oder ihr Kontakt gelöscht, ersetzt Yuva ihre Beiträge
  durch "This conversation was deleted."

Der Ersatz enthält weder Namen noch Betreff noch Text; die ersten beiden behalten den Link zur
Unterhaltung. Antworten, die Personen in Slack geschrieben haben, bleiben unverändert, ebenso ein
Beitrag, den Slack den Bot nicht mehr bearbeiten lässt (gelöscht, nach den Bearbeitungsregeln des
Workspace zu alt oder in einem archivierten Kanal). Solange Slack getrennt ist, wird nichts
bearbeitet.

## Trennen

Einstellungen › Slack › **Disconnect** widerruft das Bot-Token bei Slack und entfernt die Kanäle
der Posteingänge, die Threads und die Verknüpfungen der Mitglieder. Die App in Slack zu entfernen
bewirkt dasselbe. Verbinden und Trennen sowie das Verknüpfen und Lösen durch Mitglieder stehen im
Audit-Log des Workspace.
