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

## 5. Bülten abonelikleri

Oturum açmış kullanıcılar uygulamanızda [e-posta listesi](lists.md) aboneliklerini yönetebilir.
Oturum kimliklendirilmiş olmalı (kimlik belirteci); anonim oturum `identity_required` alır. Diğer
hatalar: `no_email` (kullanıcının e-posta adresi yok) ve `not_found` (bilinmeyen liste).

`subscribe`, `subscribed` döndürür; onay e-postası gönderildiyse (çift onay) `pending` döndürür.
`consentText`, kişinin kabul ettiği metindir, örneğin anahtarın yanındaki yazı; onay kaydıyla birlikte
saklanır.

```swift
let lists = try await yuva.subscriptions()
let state = try await yuva.subscribe(listId: list.id, consentText: "Ürün haberlerini gönderin")
try await yuva.unsubscribe(listId: list.id)
```

```kotlin
val lists = yuva.subscriptions()
val state = yuva.subscribe(list.id, "Ürün haberlerini gönderin")
yuva.unsubscribe(list.id)
```

## Başvuru

### Görünümler

| Görünüm | Gösterdiği |
|---|---|
| `YuvaConversationsView` | Kullanıcının konuşmaları; akışları açar ve yenisini başlatır. |
| `YuvaThreadView` (iOS) | Tek bir konuşma akışı. |
| `YuvaFeedbackView` | Kategori, isteğe bağlı ekran görüntüleri ve cihaz bilgileriyle geri bildirim. `screen` ve `onDone` alır. |

Gelen kutusu değerlendirme istiyorsa kapalı konuşmalarda bir değerlendirme kartı görünür.

### Sallayarak geri bildirim gönderme

İsteğe bağlıdır. Tek bir çağrı açar; cihaz sallandığında SDK o anki ekranın ekran görüntüsünü alır,
"Geri bildirim gönderilsin mi?" diye sorar ve geri bildirim formunu ekran görüntüsü ile bir `screen`
değeriyle açar.

```swift
Yuva.enableShakeToFeedback(client: client, screen: { currentRouteName }, isEnabled: { settings.shakeFeedback })
Yuva.disableShakeToFeedback()
```

```kotlin
Yuva.enableShakeToFeedback(application, client, screen = { currentRoute }, isEnabled = { prefs.shake })
Yuva.disableShakeToFeedback()
```

- `screen`, geri bildirimle birlikte saklanan değeri döndürür; örneğin mevcut rota adınız. Verilmezse
  SDK en üstteki view controller'ın sınıf adını (iOS) ya da activity adını (Android) kullanır.
- `isEnabled` her sallamada sorulur; yok saymak için `false` döndürün, örneğin bir ayardan.
- Şu durumlarda hiçbir şey olmaz: bir metin alanı odaktayken (iOS bunun için sallamayı Geri Al olarak
  kullanır; Android'de klavye açıkken), `YuvaFeedbackView` ya da `YuvaConversationsView` görünürken
  veya `isEnabled` `false` döndürürken.
- iOS, uygulamanın pencerelerinde `motionShake` olayını dinler. Android, uygulamanızın bir activity'si
  ön plandayken ivmeölçeri dinler; onay ve form bir SDK activity'sinde gösterilir. Kendi Material
  temanızı kullanmak için `theme` verin (içeriği saran bir composable, örneğin uygulama temanız).
- `signOut()` sonrasında ya da istemciyi değiştirdiğinizde yeni istemciyle tekrar çağırın.

### Yuva istemcisi

| Üye | Anlamı |
|---|---|
| `serverURL` (Android'de `serverUrl`), `channelKey` | Sunucunuz ve uygulama kanalının anahtarı. |
| `identityToken` | Bir token döndürür, anonim için `nil` / `null`. |
| `maxAttachmentSize` | En büyük yükleme, varsayılan 25 MiB. Bir uygulama kanalı daha düşük [ek sınırları](configuration.md#ekler-ve-depolama) belirleyebilir; SDK'lar bunları henüz okumaz, bu yüzden sunucu aşan dosyayı reddeder (`413` veya `415`). |
| `start()`, `identify()`, `signOut()` | Oturum. |
| `conversations()`, `messages(...)` | Sayfalı okuma. |
| `startConversation(...)`, `sendMessage(...)` | İsteğe bağlı eklerle yazma. |
| `sendFeedback(YuvaFeedback(...))` | Geri bildirim. |
| `rate(conversationId, rating, comment)` | `canRate` true olduğunda. |
| `events()` | Canlı güncellemeler (`AsyncStream` / `SharedFlow`). |
| `subscriptions()`, `subscribe(listId, consentText)`, `unsubscribe(listId)` | Oturum açmış kullanıcının bülten abonelikleri. |

SDK olmadan kendi backend'inizden geri bildirim: `feedback:write` yetkisi olan bir API anahtarıyla
`POST /v1/feedback`.
