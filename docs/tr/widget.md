# Web widget'ı

`<yuva-chat>` web sitenize canlı sohbet, ya da kendi ürününüzün içine bir konuşma akışı ekler. Her
sayfada ve her framework'te çalışır.

## 1. Sohbet kanalı oluşturun

Panelde: Ayarlar → gelen kutusu → Müşteriler nereden yazıyor → Web sitesi sohbeti. Widget'ı
gösterebilecek her origin'i ekleyin, örneğin `https://www.example.com`, sonra kanalın genel
anahtarını (`yuva_pk_…`) kopyalayın. Anahtar gizli değildir.

## 2. Sayfaya ekleyin

```html
<script src="https://support.example.com/yuva.js" defer></script>
<yuva-chat channel="yuva_pk_xxxxxxxxxxxxxxxx"></yuva-chat>
```

Bu, köşede yüzen bir düğme gösterir. Bundler ile: `npm install useyuva`, `import "useyuva/chat"` ve
`server` niteliğini atayın. Nuxt'ta: `nuxt.config` içinde `modules: ["useyuva/nuxt"]`,
`yuva: { server, channel }` ve `<YuvaChat />`. Siteniz Content Security Policy gönderiyorsa Yuva
sunucunuza `script-src` ve `connect-src` içinde izin verin (`https://` ve `wss://`).

## 3. Oturum açmış kullanıcılar

Backend'iniz o anki kullanıcı için bir [kimlik token'ı](identity.md) imzalar; widget onu ister:

```js
document.querySelector("yuva-chat").setIdentityToken(async () => {
  const response = await fetch("/api/yuva-identity-token");
  return response.ok ? (await response.json()).token : null;
});
```

Anonim ziyaretçiler için `null` döndürün. Giriş yapıldıktan sonra yeniden çağırın, çıkış yapıldıktan
sonra `signOut()` çağırın.

## Ziyaretçi neler görür

Panel bir ana ekranla açılır: kimlerin çevrimiçi olduğu, devam edilecek son konuşma ve yeni mesaj
yazma düğmesi. Konuşmada gün ayırıcıları, "katıldı" ve "kapatıldı" satırları, ek kartları
(yükleniyor, başarısız ve gönderildi), yazıyor göstergesi, ekibin okuduğu son mesajın altında "Görüldü"
ve ziyaretçi yukarı kaydırdığında okunmamış sayısını gösteren "en sona git" düğmesi bulunur. Mesaj
kutusu dosyaları, yapıştırılan görselleri ve tarayıcı izin veriyorsa sayfanın ekran görüntüsünü kabul
eder.

Ana ekran ayrıca aynı panelin içinde uygulamalar açar; her biri yalnızca kanalda ihtiyaç duyduğu
şey varsa görünür ve ilk kullanımda yüklenir. **Geri bildirim** bir kategori, metin ve dosyaları
ziyaretçinin mesajlarında bulacağı bir geri bildirim konuşması olarak gönderir. **Durum**, gelen
kutusunun yayımlanmış durum sayfasını gösterir: genel durum, son 60 günüyle bileşenler, açık ve
yakın zamanlı olaylar. **Güncellemeleri al** ziyaretçiyi gelen kutusunun herkese açık posta
listelerine abone eder ve onay metnini gösterildiği haliyle gönderir. **Yardım merkezi**, gelen
kutusunun yayımlanmış bir [yardım merkezi](help-center.md) olduğunda görünür ve Ana ekranda bir
arama alanı da belirir: ziyaretçinin bulunduğu sayfaya uyan makaleleri, popüler olanları ve
koleksiyonları gösterir, makaleleri panelin içinde "Bu yararlı oldu mu?" sorusuyla açar ve bir
"Bize yazın" düğmesiyle biter. Bir makaleyi okuduktan sonra başlayan konuşma onu kaydeder; ekibinizin
yanıtta gönderdiği makale de aynı uygulamada açılır. **Yol haritası**, gelen kutusunun herkese açık bir [yol haritası panosu](roadmap-board.md) olduğunda görünür: aşamaya ve oya göre fikirler, yanıtlarıyla bir fikir ve Fikir öner. Oy vermek ve öneri yazmak giriş ister; ziyaretçinin kimlik token'ı yoksa okuyucu penceresi açılır. **Yenilikler**, gelen kutusunun kaydı olan yayınlanmış bir [Yenilikler](whats-new.md) değişiklik günlüğü olduğunda görünür: okunmamış sayılı bir kutucuk, sonra kayıtlar yeniden eskiye etiketleri, tarihi, görseli ve metniyle.

Bir üyenin kaldırdığı mesaj, konuşmada ve konuşma listesindeki önizlemesinde "Bu mesaj kaldırıldı" olarak görünür;
panel açıkken canlı güncellenir.

Kimse çevrimiçi değilse ve kanal e-posta adresi istiyorsa konuşma bir adres ister. Ziyaretçi adresi
kaydedince konuşma onay bağlantısının gönderildiğini söyler (Adresi değiştir ve Tekrar gönder ile),
adres geri döndüğünde ya da bağlantı 24 saat sonra geçersiz olduğunda yeniden adres ister, onaylanınca
sessiz bir satır gösterir. Panel `prefers-color-scheme` değerini izler, vurgu rengi olarak kanalın
başlatıcı rengini kullanır; sayfada Geist yazı tipi varsa onu, yoksa sistem yazı tipini kullanır.

## Widget neleri saklar

Widget oturumunu sayfanızın `localStorage` alanında tutar; böylece ziyaretçinin konuşması sayfa
yenilenince kaybolmaz. Her şey sayfanızın origin'ine bağlıdır.

| Anahtar | İçerik |
|---|---|
| `yuva:<channel>` | Oturum token'ı ve süresi, anonim ziyaretçi kimliği ve düğmenin durumu. |
| `yuva:reader:<server> <channel>` | Bir okuyucu dokümantasyon sayfasında oturum açtıktan sonra okuyucu token'ı. |

Token'lar rastgele metinlerdir; Yuva onları yalnızca özetlenmiş olarak saklar ve istediği an
sonlandırabilir. Depolama sizin origin'inize ait olduğundan sayfanıza enjekte edilen bir betik (XSS)
onları okuyabilir ve token'ın süresi dolana ya da sonlandırılana kadar o ziyaretçi gibi davranabilir.
Yapabilecekleriniz:

- **Oturumu kapatın.** `signOut()` oturumu sunucuda sonlandırır ve depolamadan siler; ziyaretçi kimliği
  ile düğme durumu kalır. `readerSignOut()` okuyucu token'ını sonlandırır ve anahtarını temizler.
  Kullanıcınız uygulamanızdan çıkarken ikisini de çağırın.
- **Oturum açmış kullanıcılar için kimlik token'ı kullanın.** Backend'inizin imzaladığı
  [kısa ömürlü bir token](identity.md) kullanıcının kim olduğunu kanıtlar; saklanan oturum tek güvence
  olmaz.
- **Content Security Policy gönderin** ve `script-src` değerini güvendiğiniz kaynaklarla sınırlayın;
  enjekte edilen betiklere karşı asıl korumadır.
- **Kendi arayüzünüzü yazıyorsanız token'ı `localStorage` dışında tutun.** `createYuvaClient()` bir
  `storage` seçeneği alır: `getItem` ve `setItem` olan herhangi bir nesne, örneğin `sessionStorage`'ı
  saran ya da değerleri bellekte tutan biri. `<yuva-chat>` öğesinde böyle bir seçenek yoktur ve her
  zaman `localStorage` kullanır; `localStorage` yoksa bellek kullanılır.

## Sorun giderme

- **Hiçbir şey görünmüyor**: sayfanın origin'ine izin verilmemiş (konsol adını yazar) ya da anahtar
  yanlış.
- **"Sign in to chat with us."**: kanal anonim ziyaretçilere kapalı ve token verilmemiş.

## Başvuru

### Nitelikler

| Nitelik | Anlamı |
|---|---|
| `channel` | Sohbet kanalının genel anahtarı. Zorunlu. |
| `server` | Yuva sunucusunun adresi. Varsayılan, `yuva.js`'nin yüklendiği yer. |
| `layout` | `launcher` (varsayılan) ya da sayfanızın içinde bir akış için `embedded`; ona bir yükseklik verin. |
| `locale` | [47 dilden](#diller) biri; örneğin `en`, `tr`, `de`, `pt-BR` ya da `ar`. Varsayılan tarayıcının dili; Yuva'da olmayan bir dil İngilizceye döner. |
| `dir` | `ltr` ya da `rtl`. Varsayılan, dilin yazım yönü. |
| `identity-token` | Bir token metni; genellikle `setIdentityToken` daha iyidir. |
| `open` | Varsa panel açıktır. |

### Diller

Pencere 47 dil konuşur: `en`, `tr`, `de`, `fr`, `fr-CA`, `es`, `es-MX`, `pt-BR`, `pt-PT`, `it`, `nl`, `sv`, `da`, `nb`, `fi`, `pl`, `cs`, `sk`, `sl`, `hr`, `hu`, `ro`, `ca`, `ja`, `ko`, `zh-Hans`, `zh-Hant`, `ru`, `uk`, `el`, `id`, `ms`, `ar`, `he`, `ur`, `hi`, `bn`, `mr`, `pa`, `gu`, `or`, `kn`, `ml`, `ta`, `te`, `th`, `vi`. Arapça, İbranice ve Urduca sağdan sola dizilir. `pt-BR`, `zh-TW` ya da `es-AR` gibi bir etiket bunlardan en yakınını seçer; `no` ve `nn` için `nb` kullanılır. Listede olmayan bir dil İngilizceye döner.

### Metotlar ve olaylar

| Üye | Anlamı |
|---|---|
| `open()`, `close()`, `toggle()`, `isOpen` | Düğmenin açtığı panel. |
| `unread`, `yuva-unread` olayı | Henüz görülmemiş yanıtlar; olayda `detail.count` bulunur. |
| `setIdentityToken(fn)`, `signOut()` | Oturum açmış kullanıcılar, yukarıdaki gibi. |
| `rate(conversationId, "good" \| "bad", comment?)` | Kapalı bir konuşmayı değerlendirir; yapamadığında `409 already_rated` ya da `rating_unavailable`. |
| `openApp(kind, params?)` | Paneli bir uygulamada açar: `"help"` (`{ article }` ya da `{ query }`), `"feedback"`, `"status"`, `"subscribe"` `"roadmap"` (`{ idea }` o fikri açar) ya da `"whats-new"` (`{ entry }` o kaydı açar). |

### Kanal ayarları

| Ayar (API alanı) | Anlamı |
|---|---|
| İzin verilen kaynaklar (`allowed_origins`) | 1 ile 20 arası origin, birebir karşılaştırılır. |
| Anonim ziyaretçiler (`allow_anonymous`) | Kimlik token'ı olmayan ziyaretçiler sohbet edebilir. |
| E-posta adresi isteme (`ask_email_offline`) | Kimse müsait değilken adres ister. Okunmamış yanıtlar `YUVA_CHAT_EMAIL_DELAY` (varsayılan 5 dakika) sonra e-postayla gönderilir. Gelen kutusunda bir e-posta kanalı gerekir. |
| Karşılama mesajı (`greeting`) | İlk mesajdan önce gösterilir. |
| Düğme (`launcher`) | `position` ve `color` (`#rrggbb`). |
| Güncellemeleri duyur (`announce_updates`) | Varsayılan olarak kapalı. Okunmamış mesaj yokken başlatıcı, okunmamış [Yenilikler](whats-new.md#4-widget-içinde) kayıtlarının sayısını gösterir. |

API üzerinden: sahip ya da yönetici olarak, `kind: "chat"` ve bir `chat` nesnesiyle
`POST /v1/inboxes/{inboxId}/channels`.

Bir sohbet kanalı, `attachments` ile sunucunun [ek sınırlarını](configuration.md#ekler-ve-depolama) da daraltabilir.
Widget bunları okur: çok büyük ya da başka türde bir dosyayı yüklemeden önce bir notla reddeder, dosya seçiciyi
izinli türlerle sınırlar ve hiçbir tür izinli değilse ek düğmesini ile ekran görüntüsü seçiciyi gizler. Sahipler
ve yöneticiler bunları panelde kanalın ayarlarındaki Ekler bölümünden belirler.
