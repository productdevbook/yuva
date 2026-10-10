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
implementation("com.github.productdevbook:yuva:v0.0.6")
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

## Referenz

### Views

| View | Zeigt |
|---|---|
| `YuvaConversationsView` | Die Unterhaltungen des Nutzers; öffnet und beginnt Verläufe. |
| `YuvaThreadView` (iOS) | Einen Verlauf. |
| `YuvaFeedbackView` | Feedback mit Kategorie, optionalen Screenshots und Gerätedaten. Nimmt `screen` und `onDone`. |

Geschlossene Unterhaltungen zeigen eine Bewertungskarte, wenn der Posteingang um Bewertungen bittet.

### Client

| Member | Bedeutung |
|---|---|
| `serverURL` (Android `serverUrl`), `channelKey` | Ihr Server und der Schlüssel des App-Kanals. |
| `identityToken` | Gibt ein Token zurück, oder `nil` / `null` für anonym. |
| `maxAttachmentSize` | Größter Upload, Standard 25 MiB. |
| `start()`, `identify()`, `signOut()` | Sitzung. |
| `conversations()`, `messages(...)` | Lesen, seitenweise. |
| `startConversation(...)`, `sendMessage(...)` | Schreiben, mit optionalen Anhängen. |
| `sendFeedback(YuvaFeedback(...))` | Feedback. |
| `rate(conversationId, rating, comment)` | Wenn `canRate` true ist. |
| `events()` | Live-Updates (`AsyncStream` / `SharedFlow`). |

Feedback aus Ihrem eigenen Backend, ohne SDK: `POST /v1/feedback` mit einem API-Schlüssel, der
`feedback:write` hat.
