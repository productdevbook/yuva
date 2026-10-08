# Mobile SDKs

Native SDKs for iOS (`YuvaKit`, Swift and SwiftUI) and Android (Kotlin and Jetpack Compose) give
your app a conversation list, a thread with attachments, and a feedback form, or a headless client
for your own UI. Both talk to `/client/v1` with the public key of an `app` channel.

## 1. Create an app channel

In the panel: Settings → the inbox → Channels → App. Choose the platforms and whether people who
are not signed in may write (anonymous use is off by default). Copy the channel's **public key**
(`yuva_pk_…`); it ships inside the app and is not a secret. App keys are not tied to web origins.

To let signed-in users write as themselves, give your backend the inbox's identity secret and an
endpoint that returns an [identity token](identity.md) for the current user.

## 2. iOS: YuvaKit

Requirements: iOS 17 or macOS 14, Swift 6.

Add the package in Xcode (File → Add Package Dependencies…) with the repository URL, or in
`Package.swift`:

```swift
.package(url: "https://github.com/productdevbook/yuva.git", exact: "0.0.1"),
// target dependencies:
.product(name: "YuvaKit", package: "yuva"),
```

Create one client for the app's lifetime:

```swift
import YuvaKit

let yuva = YuvaClient(configuration: YuvaConfiguration(
    serverURL: URL(string: "https://support.example.com")!,
    channelKey: "yuva_pk_xxxxxxxxxxxxxxxx",
    identityToken: { try await MyAPI.yuvaIdentityToken() }   // nil: anonymous
))
```

Show the screens:

```swift
@State private var openConversationId: String?

YuvaConversationsView(client: yuva, openConversationId: $openConversationId)

YuvaFeedbackView(client: yuva, screenshot: nil, screen: "Settings") {
    dismiss()
}
```

`YuvaConversationsView` lists the user's conversations, opens threads and starts new ones;
`YuvaThreadView(client:conversationId:)` shows one thread. `YuvaFeedbackView` sends feedback with a
category (bug, idea, praise, other), optional screenshots and the device details (app version,
build, OS, device model, locale).

When the user signs in to your app, call `try await yuva.identify()`; when they sign out,
`await yuva.signOut()`, which ends the session on the server too.

Headless use, for your own UI:

```swift
let session = try await yuva.start()
let page = try await yuva.conversations()
let started = try await yuva.startConversation(body: "Hello")
try await yuva.sendMessage(conversationId: id, body: "Thanks")
try await yuva.sendFeedback(YuvaFeedback(category: .bug, body: "The export button does nothing"))
for await event in yuva.events() { … }   // live updates while subscribed
```

## 3. Android

Requirements: minSdk 26, Jetpack Compose, the `INTERNET` permission (the library declares it).

The library is published through [JitPack](https://jitpack.io/#productdevbook/yuva), built from
the release tags (`v0.0.1`, `v0.0.2`, …). Add the repository and the dependency, with the release
tag as the version:

```kotlin
// settings.gradle.kts
dependencyResolutionManagement {
    repositories {
        google()
        mavenCentral()
        maven("https://jitpack.io")
    }
}
// app/build.gradle.kts
dependencies {
    implementation("com.github.productdevbook:yuva:v0.0.x")
}
```

The library brings OkHttp, kotlinx.coroutines, kotlinx.serialization and Compose (aligned by the
Compose BOM) with it. The first request for a new tag starts its JitPack build, which takes a few
minutes.

Or add `sdk/kotlin/yuva` from the release tag to your build as a module, together with the
libraries it lists in `sdk/kotlin/gradle/libs.versions.toml`, and depend on it:

```kotlin
// settings.gradle.kts
include(":yuva")
// app/build.gradle.kts
dependencies { implementation(project(":yuva")) }
```

Create one client for the app's lifetime:

```kotlin
import dev.yuva.YuvaClient
import dev.yuva.YuvaConfiguration

val yuva = YuvaClient(
    context,
    YuvaConfiguration(
        serverUrl = "https://support.example.com",
        channelKey = "yuva_pk_xxxxxxxxxxxxxxxx",
        identityToken = { myApi.yuvaIdentityToken() },   // null: anonymous
    ),
)
```

Show the screens:

```kotlin
YuvaConversationsView(client = yuva, openConversationId = conversationId, onClose = { finish() })

YuvaFeedbackView(client = yuva, screen = "Settings", onDone = { navController.popBackStack() })
```

`yuva.identify()` after sign-in, `yuva.signOut()` on sign-out (both `suspend`). The headless
client has the same calls as on iOS: `start()`, `conversations()`, `messages(...)`,
`startConversation(...)`, `sendMessage(...)`, `sendFeedback(YuvaFeedback(...))`, and `events()`, a
`SharedFlow` of live updates while collected.

## 4. Push notifications

Yuva does not send push notifications to your app; your backend does, with the device tokens and
APNs or FCM keys it already has.

1. Add a [webhook](webhooks.md) for `message.created`.
2. When a webhook arrives with `data.message.kind` `message`, `data.message.direction` `out` (to
   the contact) and `data.contact.online` `false` (the app is closed or in the background), find
   the user by `data.contact.external_ids` and send them a push.
3. Put the conversation id (`data.message.conversation_id`) in the payload as
   **`yuva_conversation_id`**.

APNs: a top-level key next to `aps`:

```json
{
  "aps": { "alert": { "title": "Example Support", "body": "We have shipped a fix." }, "sound": "default" },
  "yuva_conversation_id": "0192f0c4-6f1e-7b2a-9c1d-3e4f5a6b7c8d"
}
```

FCM: a `data` entry:

```json
{
  "message": {
    "token": "<device registration token>",
    "notification": { "title": "Example Support", "body": "We have shipped a fix." },
    "data": { "yuva_conversation_id": "0192f0c4-6f1e-7b2a-9c1d-3e4f5a6b7c8d" }
  }
}
```

When the user taps it, `Yuva.handleNotification` returns the conversation to open:

```swift
func userNotificationCenter(_ center: UNUserNotificationCenter, didReceive response: UNNotificationResponse) async {
    if let id = Yuva.handleNotification(response.notification.request.content.userInfo) {
        openConversationId = id
    }
}
```

```kotlin
val extras = intent.extras?.keySet()?.associateWith { intent.extras?.getString(it).orEmpty() }.orEmpty()
val conversationId = Yuva.handleNotification(extras)   // or Yuva.handleNotification(message.data) in onMessageReceived
```

Passing the id to `YuvaConversationsView` opens that thread.

## Feedback from your server

A web form rendered by your own backend can send feedback without the SDK, with a workspace API
key: `POST /v1/feedback`, naming the user by your user id. It lands in the inbox you name, on its
`api` channel, which the first such request creates.
