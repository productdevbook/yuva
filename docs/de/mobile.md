# Mobile-SDKs

Native SDKs für iOS (SwiftUI) und Android (Jetpack Compose) geben Ihrer App eine Unterhaltungsliste,
einen Verlauf mit Anhängen und ein Feedback-Formular.

## 1. Einen App-Kanal anlegen

Im Panel: Settings → den Posteingang → Channels → App. Kopieren Sie den öffentlichen Schlüssel des
Kanals (`yuva_pk_…`); er wird mit Ihrer App ausgeliefert und ist kein Geheimnis. Für angemeldete
Nutzer liefert Ihr Backend ein [Identitäts-Token](identity.md).

## 2. iOS

Voraussetzung ist iOS 17 oder macOS 14. In Xcode: File → Add Package Dependencies… →
`https://github.com/productdevbook/yuva.git`, Produkt `YuvaKit`.

```swift
import YuvaKit

let yuva = YuvaClient(configuration: YuvaConfiguration(
    serverURL: URL(string: "https://support.example.com")!,
    channelKey: "yuva_pk_xxxxxxxxxxxxxxxx",
    identityToken: { try await MyAPI.yuvaIdentityToken() }
))

YuvaConversationsView(client: yuva, openConversationId: $openConversationId)
```

## 3. Android

Voraussetzung ist minSdk 26. Fügen Sie `maven("https://jitpack.io")` zu Ihren Repositories hinzu
und nehmen Sie den Release-Tag als Version (JitPack braucht für einen neuen Tag ein paar Minuten zum
Bauen):

```kotlin
implementation("com.github.productdevbook:yuva:v0.4.0")
```

```kotlin
val yuva = YuvaClient(context, YuvaConfiguration(
    serverUrl = "https://support.example.com",
    channelKey = "yuva_pk_xxxxxxxxxxxxxxxx",
    identityToken = { myApi.yuvaIdentityToken() },
))

YuvaConversationsView(client = yuva, openConversationId = conversationId, onClose = { finish() })
```

Imports: `dev.yuva` für den Client, `dev.yuva.ui` für die Views.

Rufen Sie auf beiden Plattformen nach der Anmeldung des Nutzers `identify()` auf und nach der
Abmeldung `signOut()`.

## 4. Push-Benachrichtigungen

Ihr Backend versendet die Pushes; Yuva sagt ihm, wann.

1. Legen Sie einen [Webhook](webhooks.md) für `message.created` an.
2. Wenn `data.message.direction` `out` ist, `data.message.kind` `message` und
   `data.contact.online` `false`, senden Sie einen Push an den Nutzer in `data.contact.external_ids`.
3. Legen Sie `data.message.conversation_id` als `yuva_conversation_id` in den Push (APNs: neben
   `aps`; FCM: in `data`).

Tippt der Nutzer darauf, gibt `Yuva.handleNotification(...)` die Unterhaltungs-ID zurück; übergeben
Sie sie als `openConversationId` an `YuvaConversationsView`.

```swift
let id = Yuva.handleNotification(response.notification.request.content.userInfo)
```

### Von Yuva gesendeter Push

Statt des Webhooks kann Yuva den Antwort-Push selbst senden, wenn der `app`-Kanal APNs- und
FCM-Zugangsdaten hat und die Push-Einstellung aktiv ist. Die App übergibt dem SDK nur ihr
Geräte-Token; das SDK hat keine Firebase-Abhängigkeit.

```swift
func application(_ application: UIApplication, didRegisterForRemoteNotificationsWithDeviceToken deviceToken: Data) {
    Yuva.setPushToken(deviceToken)
}
```

```kotlin
class AppMessagingService : FirebaseMessagingService() {
    override fun onNewToken(token: String) = Yuva.setPushToken(token)
}
```

Das SDK registriert das Token beim Start, bei einer Token-Änderung und nach `identify()` für die
aktuelle Sitzung und versucht es bei Netzwerkfehlern still erneut. `signOut()` entfernt das Gerät;
danach kommt kein Push an, bis die App `setPushToken` erneut aufruft. Dieselben Aufrufe gibt es am
Client (`yuva.setPushToken(...)`). Eine App, die `setPushToken` nie aufruft, sendet ihre Pushes
weiter selbst. Der Push trägt `yuva_conversation_id` wie ein selbst gesendeter;
`Yuva.handleNotification(...)` öffnet die Unterhaltung.

## 5. Newsletter-Abonnements

Angemeldete Nutzer können in Ihrer App Abonnements von [Mailinglisten](lists.md) verwalten. Die
Sitzung muss identifiziert sein (Identitäts-Token); anonyme Sitzungen erhalten `identity_required`.
Weitere Fehler: `no_email` (der Nutzer hat keine E-Mail-Adresse) und `not_found` (unbekannte Liste).

`subscribe` gibt `subscribed` zurück, oder `pending`, wenn eine Bestätigungs-E-Mail gesendet wurde
(Double-Opt-in). `consentText` ist der Text, dem die Person zugestimmt hat, etwa die Beschriftung
neben Ihrem Schalter; er wird mit dem Einwilligungseintrag gespeichert.

```swift
let lists = try await yuva.subscriptions()
let state = try await yuva.subscribe(listId: list.id, consentText: "Produktneuigkeiten senden")
try await yuva.unsubscribe(listId: list.id)
```

```kotlin
val lists = yuva.subscriptions()
val state = yuva.subscribe(list.id, "Produktneuigkeiten senden")
yuva.unsubscribe(list.id)
```

## Referenz

### Views

| View | Zeigt |
|---|---|
| `YuvaConversationsView` | Die Unterhaltungen des Nutzers; öffnet und beginnt Verläufe. |
| `YuvaThreadView` (iOS) | Einen Verlauf. |
| `YuvaFeedbackView` | Feedback mit Kategorie, optionalen Screenshots und Gerätedaten. Nimmt `screen` und `onDone`. |

Geschlossene Unterhaltungen zeigen eine Bewertungskarte, wenn der Posteingang um Bewertungen bittet.

Eine Nachricht, die ein Mitglied entfernt hat, erscheint als „Diese Nachricht wurde entfernt“ in der Unterhaltung und
in der Listenvorschau. Ihr `redacted` ist true, der Text leer, und `events()` sendet `YuvaEvent.messageRedacted(message)`
(Kotlin `YuvaEvent.MessageRedacted(message)`).

### Schütteln, um Feedback zu senden

Optional. Ein Aufruf schaltet es ein; beim Schütteln des Geräts nimmt das SDK einen Screenshot des
aktuellen Bildschirms auf, fragt „Feedback senden?“ und öffnet das Feedback-Formular mit dem
Screenshot und einem `screen`-Wert.

```swift
Yuva.enableShakeToFeedback(client: client, screen: { currentRouteName }, isEnabled: { settings.shakeFeedback })
Yuva.disableShakeToFeedback()
```

```kotlin
Yuva.enableShakeToFeedback(application, client, screen = { currentRoute }, isEnabled = { prefs.shake })
Yuva.disableShakeToFeedback()
```

- `screen` liefert den Wert, der mit dem Feedback gespeichert wird, zum Beispiel Ihren aktuellen
  Routennamen. Ohne ihn nimmt das SDK den Klassennamen des obersten View Controllers (iOS) oder den
  Namen der Activity (Android).
- `isEnabled` wird bei jedem Schütteln abgefragt; geben Sie `false` zurück, um es zu ignorieren,
  zum Beispiel aus einer Einstellung.
- Nichts passiert, solange ein Textfeld den Fokus hat (iOS nutzt Schütteln dort für Widerrufen; unter
  Android solange die Tastatur offen ist), solange `YuvaFeedbackView` oder `YuvaConversationsView`
  angezeigt wird oder wenn `isEnabled` `false` liefert.
- iOS greift `motionShake` an den Fenstern der App ab. Android lauscht auf den Beschleunigungssensor,
  solange eine Activity Ihrer App im Vordergrund ist, und zeigt Bestätigung und Formular in einer
  SDK-Activity; übergeben Sie `theme` (ein Composable, das den Inhalt umschließt, zum Beispiel Ihr
  App-Theme), um dort Ihr eigenes Material-Theme zu verwenden.
- Rufen Sie es nach `signOut()` oder beim Austauschen des Clients erneut mit dem neuen Client auf.

### Client

| Member | Bedeutung |
|---|---|
| `serverURL` (Android `serverUrl`), `channelKey` | Ihr Server und der Schlüssel des App-Kanals. |
| `identityToken` | Gibt ein Token zurück, oder `nil` / `null` für anonym. |
| `maxAttachmentSize` | Größter Upload, Standard 25 MiB. Ein App-Kanal kann niedrigere [Datei-Grenzen](configuration.md#dateien-und-speicher) setzen; die SDKs lesen sie (`YuvaInbox.attachments`) und lehnen eine zu große Datei vor dem Upload mit demselben `413`- oder `415`-Fehler ab wie der Server. |
| `start()`, `identify()`, `signOut()` | Sitzung. |
| `conversations()`, `messages(...)` | Lesen, seitenweise. |
| `startConversation(...)`, `sendMessage(...)` | Schreiben, mit optionalen Anhängen. |
| `sendFeedback(YuvaFeedback(...))` | Feedback. |
| `rate(conversationId, rating, comment)` | Wenn `canRate` true ist. |
| `events()` | Live-Updates (`AsyncStream` / `SharedFlow`). |
| `subscriptions()`, `subscribe(listId, consentText)`, `unsubscribe(listId)` | Newsletter-Abonnements des angemeldeten Nutzers. |

Feedback aus Ihrem eigenen Backend, ohne SDK: `POST /v1/feedback` mit einem API-Schlüssel, der
`feedback:write` hat.
