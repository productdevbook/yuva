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
implementation("com.github.productdevbook:yuva:v0.0.32")
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

## 5. Newsletter subscriptions

Signed-in users can manage [mailing list](lists.md) subscriptions in your app. The session must be
identified (an identity token); anonymous sessions get `identity_required`. Other errors:
`no_email` (the user has no e-mail address) and `not_found` (unknown list).

`subscribe` returns `subscribed`, or `pending` when a confirmation e-mail was sent (double opt-in).
`consentText` is the label the person agreed to, for example the text next to your switch; it is
stored with the consent record.

```swift
let lists = try await yuva.subscriptions()
let state = try await yuva.subscribe(listId: list.id, consentText: "Send me product news")
try await yuva.unsubscribe(listId: list.id)
```

```kotlin
val lists = yuva.subscriptions()
val state = yuva.subscribe(list.id, "Send me product news")
yuva.unsubscribe(list.id)
```

## Reference

### Views

| View | Shows |
|---|---|
| `YuvaConversationsView` | The user's conversations; opens and starts threads. Messages and Feedback as tabs, or one of them alone with `section` (`.messages` / `.feedback`, Kotlin `YuvaConversationsSection.MESSAGES` / `FEEDBACK`); the Feedback section's + opens the feedback form. |
| `YuvaThreadView` (iOS) | One thread. |
| `YuvaFeedbackView` | Feedback with a category, optional screenshots and device details. Takes `screen` and `onDone`. |

Closed conversations show a rating card when the inbox asks for ratings.

### Shake to send feedback

Opt-in. One call turns it on; shaking the device then captures a screenshot of the current screen,
asks "Send feedback?" and opens the feedback form with the screenshot and a `screen` value.

```swift
Yuva.enableShakeToFeedback(client: client, screen: { currentRouteName }, isEnabled: { settings.shakeFeedback })
Yuva.disableShakeToFeedback()
```

```kotlin
Yuva.enableShakeToFeedback(application, client, screen = { currentRoute }, isEnabled = { prefs.shake })
Yuva.disableShakeToFeedback()
```

- `screen` returns the value stored with the feedback, for example your current route name. Without
  it the SDK uses the top view controller's class name (iOS) or the activity name (Android).
- `isEnabled` is asked at every shake; return `false` to ignore it, for example from a setting.
- Nothing happens while a text field has focus (iOS uses shake for Undo there; on Android while the
  keyboard is up), while `YuvaFeedbackView` or `YuvaConversationsView` is showing, or when
  `isEnabled` returns `false`.
- iOS hooks `motionShake` on the app's windows. Android listens to the accelerometer while an
  activity of your app is in the foreground and shows the confirmation and form in an SDK activity;
  pass `theme` (a composable wrapping its content, for example your app theme) to use your own
  Material theme there.
- Call it again with a new client after `signOut()` or when you replace the client.

### Client

| Member | Meaning |
|---|---|
| `serverURL` (Android `serverUrl`), `channelKey` | Your server and the app channel's key. |
| `identityToken` | Returns a token, or `nil` / `null` for anonymous. |
| `maxAttachmentSize` | Largest upload, default 25 MiB. An app channel can set lower [attachment limits](configuration.md#attachments-and-storage); the SDKs do not read them yet, so the server refuses a file over them (`413` or `415`). |
| `start()`, `identify()`, `signOut()` | Session. |
| `conversations()`, `messages(...)` | Read, paged. |
| `startConversation(...)`, `sendMessage(...)` | Write, with optional attachments. |
| `sendFeedback(YuvaFeedback(...))` | Feedback. |
| `rate(conversationId, rating, comment)` | When `canRate` is true. |
| `events()` | Live updates (`AsyncStream` / `SharedFlow`). |
| `subscriptions()`, `subscribe(listId, consentText)`, `unsubscribe(listId)` | Newsletter subscriptions of the signed-in user. |

Feedback from your own backend, without the SDK: `POST /v1/feedback` with an API key holding
`feedback:write`.
