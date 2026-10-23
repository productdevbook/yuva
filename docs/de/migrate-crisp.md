# Von Crisp umziehen

Kopieren Sie die Unterhaltungen einer Crisp-Website nach Yuva. Yuva liest Crisp nur. Die Übersicht steht unter
[Zu Yuva umziehen](migrate.md).

## Bevor Sie anfangen

- Erzeugen Sie ein Website-Token. Öffnen Sie in Crisp **Settings → Workspace Settings → Advanced configuration →
  API Token** und erzeugen Sie ein Token für den Workspace. Crisp zeigt eine **Kennung** und einen **Schlüssel**;
  kopieren Sie beide. Crisps Seite: [Authentication](https://docs.crisp.chat/guides/rest-api/authentication/).
  Welche Crisp-Tarife das Website-Token anbieten, bestimmt Crisp; fehlt die Seite in Ihrem Workspace, enthält Ihr
  Tarif es möglicherweise nicht.
- Suchen Sie die **Website-ID**: Es ist die UUID in der Adresse Ihres Crisp-Dashboards, hinter `/website/`.
- Geben Sie im Schritt Connect die **Kennung** als API-Token, den **Schlüssel** als Schlüssel und die
  Website-ID als Website ein und drücken Sie dann **Verbindung testen**.

Crisp kennt für ein Website-Token keine reine Leseberechtigung, das Token könnte also Daten ändern. Schreibgeschützt
hält es Yuvas Verbindung: Sie sendet Crisp nur Leseanfragen und das Token nur an `api.crisp.chat`. Plugin-Tokens
(Marketplace) werden nicht verwendet.

## Was kopiert wird

| Crisp | Yuva |
|---|---|
| Die Website | Ein Posteingang, wie Sie ihn zuordnen |
| Unterhaltungen | Unterhaltungen mit allen Nachrichten |
| Nachrichten von Besuchern | Eingehende Nachrichten |
| Nachrichten von Operatoren | Ausgehende Nachrichten |
| Notizen | Notizen, die nur Ihr Team sieht |
| Datei- und Animationsnachrichten | Dateien an den Nachrichten |
| Status: pending, unresolved | Offen |
| Status: resolved | Geschlossen |
| Segmente | Labels, wie Sie sie zuordnen (oder keine) |
| Operatoren | Teammitglieder, wie Sie sie zuordnen; andere bleiben mit Namen erhalten |
| Zugewiesener Operator | Der Bearbeiter, wenn der Operator zugeordnet ist |
| Die Person der Unterhaltung (E-Mail, Nickname, Telefon) | Ein Kontakt |

Ereignisse (zum Beispiel ein Statuswechsel), Auswahlfelder und Felder sind keine Nachrichten und werden nicht
kopiert. Eine zweite Person, die in einer Unterhaltung schreibt, behält ihren eigenen Namen an ihren Nachrichten.
Dateien werden ohne das Token aus Crisps eigenem Speicher geholt; eine Datei oder Animation an einem anderen Ort und
Links, die jemand in eine Nachricht getippt hat, werden nie abgerufen. Unterhaltungen behalten ihre ursprünglichen
Zeiten und kommen [still](migrate.md#how-history-stays-quiet) an.

## Was nicht kopiert wird

Kontaktprofile über die Person der Unterhaltung hinaus, Kampagnen, die Wissensdatenbank, Kurzbefehle,
Sprachnachrichten und Berichte. Yuva kopiert nur, was die Tabelle oben aufführt.

## Limits

Ein Website-Token darf 10.000 Anfragen am Tag stellen. Yuva nutzt höchstens 5.000 davon, etwa zwei pro
Unterhaltung, und pausiert für den Rest des Tages, wenn sie verbraucht sind; eine große Website dauert deshalb
mehrere Tage, der Probelauf nennt die Zahl. Crisp sendet keine Limit-Header: Antwortet es mit zu vielen Anfragen,
wartet Yuva eine Minute und arbeitet danach langsamer. Crisps Seite:
[Rate limits](https://docs.crisp.chat/guides/rest-api/rate-limits/).

## Die Umstellung

Crisp chattet auf Ihrer Website weiter, bis Sie dessen Chatfenster durch das von Yuva ersetzen. E-Mails an Ihre
Support-Adresse erreichen Yuva, sobald Sie die Weiterleitung dieser Adresse auf die
[Weiterleitungsadresse](email.md#forwarding-address) des Kanals umstellen. Bis das Chatfenster ersetzt ist, liest
Yuva Crisp alle zwei Minuten, sodass keine Unterhaltung verloren geht.

Drücken Sie dann **Testnachricht senden**: Es funktioniert, wenn die Nachricht auf dem neuen Weg bei Yuva ankommt.
Nach der ruhigen Phase oder mit **Jetzt beenden** endet der Umzug
([wie](migrate.md#the-test-message-the-quiet-period-and-finishing)).

## Nach dem Umzug

Erneuern Sie das Token: Öffnen Sie in Crisp **Settings → Workspace Settings → Advanced configuration** und erzeugen
Sie es neu. Yuva löscht sein Token, wenn der Umzug endet, kann es in Crisp aber nicht widerrufen; erst die
Neuerzeugung beendet seinen Zugriff. Behalten Sie Crisp gern als Archiv.
