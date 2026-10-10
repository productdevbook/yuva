# Mobil SDK'lar

iOS (SwiftUI) ve Android (Jetpack Compose) için yerel SDK'lar uygulamanıza bir konuşma listesi, ek
gönderilebilen bir konuşma akışı ve bir geri bildirim formu ekler.

## 1. Uygulama kanalı oluşturun

Panelde: Ayarlar → gelen kutusu → Müşteriler nereden yazıyor → Mobil uygulama. Kanalın genel
anahtarını (`yuva_pk_…`) kopyalayın; uygulamanızın içinde dağıtılır ve gizli değildir. Oturum açmış
kullanıcılar için backend'iniz bir [kimlik token'ı](identity.md) döndürür.

## 2. iOS

iOS 17 ya da macOS 14 gerekir. Xcode'da File → Add Package Dependencies… →
`https://github.com/productdevbook/yuva.git`, ürün `YuvaKit`.

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

minSdk 26 gerekir. Repository'lerinize `maven("https://jitpack.io")` ekleyin ve sürüm olarak
release etiketini kullanın (JitPack'in yeni bir etiketi derlemesi birkaç dakika sürer):

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

Import'lar: istemci için `dev.yuva`, görünümler için `dev.yuva.ui`.

İki platformda da kullanıcı giriş yaptıktan sonra `identify()`, çıkış yaptıktan sonra `signOut()`
çağırın.

## 4. Anlık bildirimler

Push'ları backend'iniz gönderir; ne zaman göndereceğini Yuva söyler.

1. `message.created` için bir [webhook](webhooks.md) ekleyin.
2. `data.message.direction` `out`, `data.message.kind` `message` ve `data.contact.online` `false`
   olduğunda, `data.contact.external_ids` içindeki kullanıcıya push gönderin.
3. `data.message.conversation_id` değerini push'a `yuva_conversation_id` olarak koyun (APNs: `aps`'in
   yanına; FCM: `data` içine).

Kullanıcı bildirime dokunduğunda `Yuva.handleNotification(...)` konuşma id'sini döndürür; bunu
`YuvaConversationsView`'a `openConversationId` olarak verin.

```swift
let id = Yuva.handleNotification(response.notification.request.content.userInfo)
```

## Başvuru

### Görünümler

| Görünüm | Gösterdiği |
|---|---|
| `YuvaConversationsView` | Kullanıcının konuşmaları; akışları açar ve yenisini başlatır. |
| `YuvaThreadView` (iOS) | Tek bir konuşma akışı. |
| `YuvaFeedbackView` | Kategori, isteğe bağlı ekran görüntüleri ve cihaz bilgileriyle geri bildirim. `screen` ve `onDone` alır. |

Gelen kutusu değerlendirme istiyorsa kapalı konuşmalarda bir değerlendirme kartı görünür.

### Yuva istemcisi

| Üye | Anlamı |
|---|---|
| `serverURL` (Android'de `serverUrl`), `channelKey` | Sunucunuz ve uygulama kanalının anahtarı. |
| `identityToken` | Bir token döndürür, anonim için `nil` / `null`. |
| `maxAttachmentSize` | En büyük yükleme, varsayılan 25 MiB. |
| `start()`, `identify()`, `signOut()` | Oturum. |
| `conversations()`, `messages(...)` | Sayfalı okuma. |
| `startConversation(...)`, `sendMessage(...)` | İsteğe bağlı eklerle yazma. |
| `sendFeedback(YuvaFeedback(...))` | Geri bildirim. |
| `rate(conversationId, rating, comment)` | `canRate` true olduğunda. |
| `events()` | Canlı güncellemeler (`AsyncStream` / `SharedFlow`). |

SDK olmadan kendi backend'inizden geri bildirim: `feedback:write` yetkisi olan bir API anahtarıyla
`POST /v1/feedback`.
