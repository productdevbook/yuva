---
title: "Yuva 0.1.0: Teams, Berichte, Single Sign-on und fünf weitere Help Desks"
description: "Yuva 0.1.0 bringt Teams mit Zuweisung und Reaktionszielen, Berichte, Slack- und Issue-Tracker-Verknüpfungen, von Yuva selbst gesendete Push-Nachrichten, Single Sign-on für Mitglieder und Verlaufsimporte aus fünf weiteren Help Desks. Vier Änderungen müssen Sie beim Update beachten."
draft: true
author: productdevbook
---

Yuva ist ein quelloffener Posteingang für jedes Produkt, das Sie betreiben. E-Mail, Live-Chat auf einer Website und In-App-Unterhaltungen aus iOS und Android landen an einem Ort, daneben Feedback, ein Hilfe-Center, Roadmap- und Statusseiten, Newsletter und cookielose Analytics. Sie können unseren gehosteten Dienst nutzen oder denselben Code selbst mit Postgres und einem Container betreiben. Nach den [mobilen Oberflächen](/blog/yuva-mobile-sdks/) und mehr als dreißig kleinen Versionen ist [0.1.0](https://github.com/productdevbook/yuva/releases/tag/v0.1.0) die erste, die wir als Meilenstein nummerieren.

Es ist auch eine Version, die man nicht blind einspielt. Beim Start ändern sich vier Dinge; sie stehen unter [Vor dem Update](#vor-dem-update).

## Was neu ist

### Team-Workflow

- **Teams** haben Zugriff auf Posteingänge und bekommen Zuweisungen. Siehe [Teams](/de/docs/headless/#teams).
- **Automatische Zuweisung** gibt eine neue Unterhaltung an ein Mitglied des Posteingangs oder Teams, das online ist, reihum oder nach den wenigsten offenen Unterhaltungen, nie über ein Limit hinaus, das Sie pro Mitglied setzen. Siehe [Zuweisung](/de/docs/assignment/).
- **SLA-Richtlinien** setzen Ziele für die erste Antwort, weitere Antworten und die Lösung nach Geschäftszeiten. Die Liste zeigt das Ziel, eine Verletzung benachrichtigt die zugewiesene Person. Siehe [SLA](/de/docs/sla/).
- **Automatisierungsregeln** vergeben Labels, weisen zu, antworten oder rufen einen Webhook auf, wenn eine Unterhaltung passt; eine Regel löst sich nicht durch ihre eigene Änderung erneut aus. Siehe [Automatisierung](/de/docs/automation/).
- **Makros** wenden Antwort, Labels und Status in einem Schritt an, mit Rückgängig. **Gespeicherte Ansichten** sind geteilte Filter, die jedem Mitglied nur zeigen, was es sehen darf. **Benutzerdefinierte Felder** können vor dem Schließen einer Unterhaltung erforderlich sein. Siehe [Makros](/de/docs/headless/#makros), [gespeicherte Ansichten](/de/docs/headless/#gespeicherte-ansichten) und [benutzerdefinierte Felder](/de/docs/custom-fields/).
- **Audit-Log**: Inhaber lesen, wer Zugriff, Zugangsdaten und Daten geändert hat, ohne Nachrichteninhalt und ohne Geheimnisse. Siehe [Audit-Log](/de/docs/operations/#audit-log).
- **Papierkorb und Schwärzung**: Eine gelöschte Unterhaltung kommt 30 Tage in einen Papierkorb und lässt sich wiederherstellen; eine einzelne Nachricht lässt sich samt Anhang entfernen und erscheint dann als entfernt. Siehe [Unterhaltungen löschen](/de/docs/headless/#unterhaltungen-löschen).

### E-Mail

- **Cc und Bcc** bei Antworten. Personen in Cc bleiben an der Unterhaltung beteiligt, ihre Antworten gehören dazu, wenn die Mail DMARC bestanden hat.
- **Unterhaltung weiterleiten** an eine externe Adresse; die Antwort kommt zurück in die Unterhaltung, sichtbar nur für Ihr Team.
- **Adressnachweis**: Ein E-Mail-Kanal empfängt Mail an seiner eigenen Adresse erst, wenn der Arbeitsbereich die Adresse nachgewiesen hat (siehe unten).

Einzelheiten stehen unter [E-Mail](/de/docs/email/).

### Berichte

Berichte zählen Unterhaltungen, Antworten, Zeiten bis zur ersten Antwort und bis zur Lösung, SLA-Ergebnisse und Bewertungen je Tag, Woche oder Monat, nach Posteingang, Kanal, Label, Team oder Mitglied, mit CSV-Export. Ein Agent sieht nur seine Posteingänge. Dieselben Zahlen liefern die API und der MCP-Server. Siehe [Berichte](/de/docs/reports/).

### Integrationen

- **Slack**: Neue Unterhaltungen erscheinen in einem Kanal, eine Antwort im Thread wird zur Notiz des verknüpften Mitglieds. Siehe [Slack](/de/docs/slack/).
- **GitHub und Linear**: Verknüpfen Sie Feedback mit einem Issue, sein Status bestimmt den Fortschritt des Feedbacks. Siehe [Issue-Tracker](/de/docs/issue-trackers/).
- **Von Yuva gesendeter Push**: Mit APNs- und FCM-Schlüsseln am App-Kanal sendet Yuva den Antwort-Push selbst, ohne Webhook. Siehe [Mobile](/de/docs/mobile/#von-yuva-gesendeter-push).
- Hilfe-Center, Roadmap-Boards und Neuigkeiten-Seiten können unter einer eigenen Domain antworten, und Hilfeartikel lassen sich aus Zendesk Guide und Help Scout importieren. Siehe [Hilfe-Center](/de/docs/help-center/#import-aus-zendesk-oder-help-scout).

### Umzug aus einem anderen Help Desk

Verlaufsimporte gibt es jetzt auch für [Front](/de/docs/migrate-front/), [Freshdesk](/de/docs/migrate-freshdesk/), [Intercom](/de/docs/migrate-intercom/), [Crisp](/de/docs/migrate-crisp/) und [Chatwoot](/de/docs/migrate-chatwoot/), neben Zendesk, Help Scout und jedem Postfach über IMAP. Yuva liest das alte System nur. Der Verlauf behält seine ursprünglichen Daten, kommt als gelesen an und löst keine Benachrichtigung, keinen Webhook und keinen Push aus. Siehe [Zu Yuva umziehen](/de/docs/migrate/).

### Anmeldung und Sicherheit

- **Single Sign-on für Mitglieder** mit OpenID Connect oder SAML 2.0, für die E-Mail-Domains, die ein Arbeitsbereich beansprucht hat. Mitglieder können bei der ersten Anmeldung angelegt werden, und Inhaber können Single Sign-on vorschreiben. Siehe [Single Sign-on](/de/docs/sso/).
- Ratenlimits werden in Postgres gezählt, sodass mehrere Serverprozesse sie teilen. Anhangsgrenzen gelten pro Kanal. Der Server exportiert OpenTelemetry-Traces, sobald Sie ihn auf einen Collector zeigen lassen.

## Was 0.1 bedeutet

Das ganze Repository hat eine Version. Server, API-Vertrag, die Bibliotheken für JavaScript, Swift, Kotlin und Go, das Helm-Chart und die MCP-Manifeste melden `0.1.0`. `/v1` und `/client/v1` in URLs sind die Version des Vertrags und bleiben, wie sie sind.

Yuva ist weiter in der Beta. Bis 1.0 können sich API und Datenbankschema in einer Version ändern. Ein Update folgt ab jetzt dem [Betriebshandbuch](/de/docs/operations/#updates): Release Notes aller Versionen dazwischen lesen, sichern, das Image-Tag ändern, starten. Datenbankmigrationen laufen beim Start. Was mehr verlangt, steht vor dem Release im Handbuch, wie jetzt für 0.1.0.

## Vor dem Update

Die vollständigen Schritte stehen unter [Update auf 0.1.0](/de/docs/operations/#update-auf-010). Kurz:

1. **Jede E-Mail-Adresse nachweisen.** Nach dem Update ist jeder E-Mail-Kanal ungeprüft. Ein ungeprüfter Kanal routet keine Mail mehr über seine eigene Adresse; Mail an die Weiterleitungsadresse und das Senden funktionieren weiter, ein Catch-all empfängt nichts. Jeder Arbeitsbereich weist einen Kanal per DNS-TXT-Eintrag nach, mit einem an die Adresse gesendeten Code, oder Sie bürgen dafür:

   ```sh
   docker compose exec yuva /yuva channel prove --all-unproven --workspace <ws>
   ```

   Ohne `--yes` listet der Befehl nur auf, was er nachweisen würde. Führen Sie ihn mit `--yes --plan <code>` erneut aus, mit dem ausgegebenen Code.

2. **Den Email Worker neu ausrollen.** `YUVA_INGRESS_ACCEPT_V2` ist jetzt aus, der Server akzeptiert nur noch `v3`-Signaturen. Rollen Sie den Worker in `edge/` vor dem Update neu aus oder setzen Sie bis dahin `YUVA_INGRESS_ACCEPT_V2=true`.

3. **`YUVA_INGRESS_AUTHSERV_ID` setzen.** Begrüßungen, Regelantworten und Bewertungsanfragen gehen jetzt nur noch an Absender, deren Mail DMARC bestanden hat. Ohne die Einstellung wird nichts davon gesendet, und der Server warnt beim Start.

4. **Ein Server ohne SMTP protokolliert Mail.** Ist `YUVA_SMTP_HOST` nicht gesetzt, schreibt der Server Mails geschwärzt ins Log, statt sie zu verwerfen. Ein Anmeldecode erscheint als eine Zeile, die Adresse auf `*@domain` gekürzt.

Zwei weitere Änderungen. Hintergrundarbeit läuft jetzt auf mehr Job-Queues, sodass ein langsamer Kundenendpunkt Zuweisung und Benachrichtigungen nicht aufhält; Sie müssen nichts einstellen. Und Single-Sign-on-Sitzungen sind enger gefasst: Eine Sitzung über den Anbieter eines Arbeitsbereichs gilt nur in diesem Arbeitsbereich und nur als Leser. Wer sich anderswo so angemeldet hat, meldet sich dort mit Code oder Passkey neu an. Schaltet ein Inhaber „Single Sign-on vorschreiben“ ein, werden die betroffenen Mitglieder aus jeder Sitzung abgemeldet, die nicht über Ihren Anbieter entstand, in allen ihren Arbeitsbereichen. Inhaber sind ausgenommen.

## Installation

- **Gehostet:** Legen Sie auf der [Startseite](/de/) ein Konto an.
- **Selbst betrieben:** Die [Installationsanleitung](/de/docs/install/) startet Yuva und Postgres mit Docker Compose in wenigen Minuten und beschreibt auch Helm für Kubernetes. Das Image heißt `ghcr.io/productdevbook/yuva:0.1.0`.

## Was wir nicht ausprobiert haben

Die genannten Funktionen haben wir auf einem laufenden Server mit aufgezeichneten oder nachgebauten Anbietern geprüft: Mailpit für Mail, Keycloak für Single Sign-on, nachgebaute Slack-, GitHub-, Linear-, APNs- und FCM-Endpunkte und aufgezeichnete Antworten für die Importe. Echtes DNS, echte DMARC-Ergebnisse bei einem Mail-Anbieter, eine echte Slack-App, Push auf echten Telefonen und Importe aus echten Help-Desk-Konten gehören nicht dazu. Wenn Sie es versuchen, melden Sie uns in einem [Issue](https://github.com/productdevbook/yuva/issues), was nicht funktioniert.
