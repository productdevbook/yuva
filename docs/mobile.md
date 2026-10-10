# Mobile SDKs

Native SDKs for iOS (SwiftUI) and Android (Jetpack Compose) give your app a conversation list, a
thread with attachments and a feedback form.

## 1. Create an app channel

In the panel: Settings → the inbox → Channels → App. Copy the channel's public key (`yuva_pk_…`);
it ships inside your app and is not a secret. For signed-in users, your backend returns an
[identity token](identity.md).

## 2. iOS

Requires iOS 17 or macOS 14. In Xcode, File → Add Package Dependencies… →
`https://github.com/productdevbook/yuva.git`, product `YuvaKit`.

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

Requires minSdk 26. Add `maven("https://jitpack.io")` to your repositories and the release tag as
the version (a new tag takes JitPack a few minutes to build):

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

Imports: `dev.yuva` for the client, `dev.yuva.ui` for the views.

On both platforms, call `identify()` after the user signs in and `signOut()` after they sign out.

## 4. Push notifications

Your backend sends pushes; Yuva tells it when.

1. Add a [webhook](webhooks.md) for `message.created`.
2. When `data.message.direction` is `out`, `data.message.kind` is `message` and
   `data.contact.online` is `false`, push to the user in `data.contact.external_ids`.
3. Put `data.message.conversation_id` in the push as `yuva_conversation_id` (APNs: next to `aps`;
   FCM: in `data`).

When the user taps it, `Yuva.handleNotification(...)` returns the conversation id; pass it to
`YuvaConversationsView` as `openConversationId`.

```swift
let id = Yuva.handleNotification(response.notification.request.content.userInfo)
```

## Reference

### Views

| View | Shows |
|---|---|
| `YuvaConversationsView` | The user's conversations; opens and starts threads. |
| `YuvaThreadView` (iOS) | One thread. |
| `YuvaFeedbackView` | Feedback with a category, optional screenshots and device details. Takes `screen` and `onDone`. |

Closed conversations show a rating card when the inbox asks for ratings.

### Client

| Member | Meaning |
|---|---|
| `serverURL` (Android `serverUrl`), `channelKey` | Your server and the app channel's key. |
| `identityToken` | Returns a token, or `nil` / `null` for anonymous. |
| `maxAttachmentSize` | Largest upload, default 25 MiB. |
| `start()`, `identify()`, `signOut()` | Session. |
| `conversations()`, `messages(...)` | Read, paged. |
| `startConversation(...)`, `sendMessage(...)` | Write, with optional attachments. |
| `sendFeedback(YuvaFeedback(...))` | Feedback. |
| `rate(conversationId, rating, comment)` | When `canRate` is true. |
| `events()` | Live updates (`AsyncStream` / `SharedFlow`). |

Feedback from your own backend, without the SDK: `POST /v1/feedback` with an API key holding
`feedback:write`.
