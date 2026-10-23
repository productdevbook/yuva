# Headless

Panelin ve widget'ın yaptığı her şey kendi kodunuza da açıktır: `/v1` üzerinde kendi gelen kutunuz,
kişi API'si üzerinde kendi sohbetiniz ya da taslaklarla yanıt veren bir bot. Çalışan kod
[`examples/`](../../examples) dizininde, MIT lisanslı.

## API anahtarları

Kodunuz `/v1` ile bir API anahtarıyla konuşur. Sahipler ve yöneticiler anahtarları Ayarlar → API
anahtarları altında ya da sunucuda oluşturur:

```sh
yuva api-key create --workspace <id|name> --name "Support bot" --scope conversations:read --scope messages:write
```

Bir anahtara yalnızca gereken [yetkileri](#yetkiler) verin (`--scope` yoksa hepsi verilir);
`--inbox <id>` onu bazı gelen kutularıyla sınırlar. Her anahtar, adı yazdığı her şeyde görünen bir
bottur. Anahtarları sunucularınızda tutun.

## Kendi gelen kutusu arayüzünüz

Konuşmaları tipli istemciyle okuyup yazın (`npm install useyuva`):

```ts
import { createYuvaApi } from "useyuva/api";

const api = createYuvaApi({ server: "https://support.example.com", apiKey: process.env.YUVA_API_KEY! });
const { data } = await api.GET("/v1/conversations", { params: { query: { status: ["open"] } } });
```

Go ile: `github.com/productdevbook/yuva/sdk/go/client` ([README](../../sdk/go/README.md)). Diğer
diller: [OpenAPI sözleşmesi](../../openapi/openapi.yaml). Ekibiniz için bir arayüz,
[OAuth](mcp.md#oauth) ile her üyenin yerine hareket edebilir.

Değişiklikleri izlemenin iki yolu var:

- **Olay akışını yoklayın.** `GET /v1/events/latest` ile başlayın, sonra
  `GET /v1/events?after=<position>&limit=100` çağırın, `events`'i işleyin, `next`'i saklayın ve
  `has_more` true olduğu sürece hemen tekrarlayın. Bkz. [`examples/inbox-feed`](../../examples/inbox-feed).
- **Bir WebSocket açık tutun:** `Authorization: Bearer <key>` ile `GET /v1/realtime`. Kaçırdıklarınızı
  almak için `?last_event_id=<last id handled>` ile yeniden bağlanın.

## Kendi sohbet arayüzünüz

Kişiler, API anahtarıyla değil kendi oturumlarıyla kişi API'sini (`/client/v1`) kullanır.
`createYuvaClient()` bunu DOM olmadan sarar; tarayıcılar, Node 22+, Bun, Deno ve worker'lar için:

```ts
import { createYuvaClient } from "useyuva";

const yuva = createYuvaClient({
  server: "https://support.example.com",
  channel: "yuva_pk_…",
  identityToken: () => fetch("/my/yuva-token").then((r) => r.text()),
});

yuva.on("event", (event) => console.log(event.type));
await yuva.connect();
const { conversation } = await yuva.startConversation({ body: "Hello" });
```

Bir üyenin sildiği konuşma `/client/v1` içinde görünmez olur ve gerçek zamanlı akış bir `conversation.removed`
çerçevesi (`data: { id }`) gönderir: konuşmayı listenizden çıkarın, açıksa kapatın. Bu kimliğe gönderilen
mesaj yeni bir konuşma başlatır; yanıttaki `conversation_id` onu belirtir. Konuşma geri alınırsa yeniden
`conversation.created` olarak gelir. `createYuvaClient()` onu
`conversationRemoved` olayı (`{ id }`) olarak yayar, React kancaları konuşmayı listeden çıkarır; Swift ve Kotlin
SDK'ları `YuvaEvent.conversationRemoved(id:)` / `YuvaEvent.ConversationRemoved(id)` gönderir.

Bir üye [bir mesajı kaldırdığında](#bir-mesajı-kaldırma) akış bir `message.redacted` çerçevesi gönderir; `data`,
`redacted: true`, boş bir `body` ve eksiz mesajdır: mesajı yerinde değiştirin ve "Bu mesaj kaldırıldı" gösterin.
Aynı mesaj sonraki bir tekrar oynatmada veya listede de `redacted: true` taşır.
`createYuvaClient()` çerçeveyi `event` olayında iletir, React kancaları mesajı günceller; Swift ve Kotlin SDK'ları
`YuvaEvent.messageRedacted(_:)` / `YuvaEvent.MessageRedacted(message)` gönderir.

Tarayıcı dışında bir `app` kanalının anahtarını kullanın; `chat` kanalları yalnızca izin verilen
origin'leri kabul eder. React hook'ları `useyuva/react` içinde; tüm seçenekler
[SDK README](../../sdk/js/README.md)'sinde.

## Taslakla yanıtlayan bot

1. Botunuzu gösteren, `message.created` için bir [webhook](webhooks.md) ekleyin.
2. [İmzayı doğrulayın](webhooks.md#webhook-imzasını-doğrulayın) ve yalnızca `direction: "in"` ve
   `kind: "message"` olan `data.message` üzerinde işlem yapın.
3. Yanıtı taslak olarak gönderin. Gelen mesajdan türetilen bir `Idempotency-Key`, yeniden denenen bir
   webhook'un ikinci bir taslak oluşturmasını engeller:

```sh
curl -X POST https://support.example.com/v1/conversations/$CONVERSATION/messages \
  -H "Authorization: Bearer $YUVA_API_KEY" -H "Content-Type: application/json" \
  -H "Idempotency-Key: draft-bot:$INCOMING_MESSAGE_ID" \
  -d '{"kind": "message", "direction": "out", "draft": true, "body": "Thanks, we are on it."}'
```

Bir üye taslağı botun adıyla görür ve gönderir, düzenler ya da atar. Bkz.
[`examples/draft-bot`](../../examples/draft-bot).

### Botlar gönderebilir

**Botlar gönderebilir** çalışma alanı ayarı (Ayarlar → API anahtarları) varsayılan olarak kapalıdır:
bu durumda bir anahtarın giden mesajı taslak olmak zorundadır (yoksa `403 bot_sending_disabled`).
Açıkken `messages:write` yetkili bir anahtar doğrudan yanıt verir, `drafts:send` ile taslakları da
gönderir. Bunu yalnızca panelde oturum açmış bir sahip ya da yönetici değiştirebilir
(`PATCH /v1/workspace` üzerinde `bots_may_send`), anahtarlar asla.

## Başvuru

### Yetkiler

Gereken yetki olmadan yapılan çağrı `403 insufficient_scope` döner.

| Yetki | İzin verdiği |
|---|---|
| `conversations:read` | Konuşmalar, mesajlar, ekler, etiketler, hazır yanıtlar, olay akışı ve gerçek zamanlı bağlantı |
| `conversations:write` | Durum, atanan kişi, erteleme, öncelik, etiketler; konuşmaları taşıma ve toplu güncelleme |
| `conversations:delete` | Konuşmaları Çöp kutusuna taşıma, geri alma, Çöp kutusunu listeleme. Başka hiçbir yetkiye dahil değildir: adıyla verin. Yalnızca sahiplerin ve yöneticilerin anahtarlarına ve bağlı uygulamalarına sunulur. |
| `messages:write` | Mesajlar ve taslaklar: oluşturma, düzenleme, atma |
| `messages:redact` | [Bir mesajın içeriğini kaldırma](#bir-mesajı-kaldırma). Başka hiçbir yetkiye dahil değildir: adıyla verin. |
| `drafts:send` | `messages:write` ile birlikte taslak gönderme |
| `notes:write` | Notlar |
| `contacts:read` | Kişiler, aramalar, çevrimiçi durumu |
| `contacts:write` | Kişi oluşturma, güncelleme, silme ve birleştirme |
| `inboxes:read` | Çalışma alanı, üyeler, gelen kutuları, kanallar |
| `inboxes:manage` | Gelen kutularını ve kanalları oluşturma, değiştirme ve silme, gelen kutusu erişimi, gizli anahtarlar |
| `labels:write` | Etiketler |
| `canned_replies:write` | Hazır yanıtlar |
| `webhooks:manage` | Webhook'lar, teslimatları ve denemeleri |
| `workspace:manage` | Çalışma alanı ayarları ve kullanım |
| `migrations:manage` | [Bir posta kutusunu Yuva'ya taşıma](migrate.md): tespit, oluşturma, bağlama, tarama, başlatma, bitirme, aktarılan geçmişi kaldırma. Asistan hesaplarına ve gelen kutularıyla sınırlı anahtarlara asla verilmez. |
| `analytics:read` | [Analitik](analytics.md#api): mülkler, rakamlar, canlı, durum |
| `analytics:write` | [Sunucu tarafı olayları](analytics-server.md) gönderme (`POST /v1/analytics/properties/{propertyId}/events`). Yalnızca API anahtarları; OAuth istemcilerine asla sunulmaz. |
| `analytics:manage` | Mülk oluşturma, değiştirme ve silme, anahtarlarını yenileme, verilerini silme. Asistan hesaplarına ve gelen kutularıyla sınırlı anahtarlara asla verilmez. |
| `status:read` | [Durum sayfaları](status-pages.md): sayfalar, bileşenler, olaylar, bildirimler, monitörler ve kontrolleri |
| `status:manage` | Durum sayfalarını değiştirme, durum ayarlama, olay güncellemesi gönderme, monitörleri yönetme, konuşmaları olaylara bağlama ve onlara yanıt verme. `status:read` içermez. |
| `help:read` | [Yardım merkezleri](help-center.md): merkezler, koleksiyonlar, taslakları ve revizyonlarıyla makaleler, arama, rapor |
| `help:write` | Koleksiyon ve makale oluşturma ve değiştirme, taslak kaydetme, görsel yükleme |
| `help:publish` | Yardım makalelerini yayınlama, yayından kaldırma ve yayımlanmış olanları silme. Başka hiçbir yetkiye dahil değildir: adıyla verin. |
| `roadmap:read` | [Yol haritası panoları](roadmap-board.md): panolar, aşama geçmişi, bağlı geri bildirimlere yanıtın sonucu |
| `roadmap:write` | Pano oluşturma, değiştirme ve silme, geri bildirimleri fikirlere bağlama, bağlı geri bildirimlere yanıt. Fikirleri okumak ve bağlamak ayrıca `conversations:read` ister. |
| `roadmap:stage` | Bir fikrin aşamasını taşıma; bağlı geri bildirimlerinin ilerlemesini atar. Başka hiçbir yetkiye dahil değildir: adıyla verin. |
| `changelog:read` | [Yenilikler](whats-new.md): değişiklik günlükleri, taslaklı kayıtlar, önizleme |
| `changelog:write` | Kayıt ve çeviri yazma, görsel yükleme |
| `changelog:publish` | Değişiklik günlüğü oluşturma, değiştirme ve silme; yayındaki kayıtları yayınlama, zamanlama, yayından kaldırma ve silme. Başka hiçbir yetkiye dahil değildir: adıyla verin. |
| `audit:read` | `GET /v1/audit-log`, [denetim günlüğü](operations.md#denetim-günlüğü). Başka hiçbir kapsama dahil değildir: adıyla verin. Yalnızca bir sahip anahtara verebilir, gelen kutularına sınırlı bir anahtar bunu tutamaz. Yalnızca anahtarı oluşturan üye hâlâ sahip olduğu sürece çalışır. `yuva api-key create` adını vermezseniz bunu eklemez. |
| `feedback:write` | `POST /v1/feedback` |

### Anahtarlar

| Ayar | Ayrıntı |
|---|---|
| Yetkiler, gelen kutuları | Oluştururken sabitlenir. Gelen kutusuyla sınırlı bir anahtar `inboxes:manage`, `webhooks:manage` ya da `workspace:manage` alamaz. |
| Son kullanma | İsteğe bağlı, sabit. Sonrasında `401 api_key_expired`. |
| Ad, bot adı, avatar | Anahtarın yazdıklarında görünür. `PATCH /v1/api-keys/{id}` ile değiştirilir. |

### Taslaklar

| Çağrı | Yaptığı |
|---|---|
| `PATCH /v1/messages/{id}` | Bir taslağı düzenler. |
| `DELETE /v1/messages/{id}` | Taslağı atar. |
| `POST /v1/messages/{id}/send` | Taslağı gönderir; `sent_by` gönderenin kim olduğunu söyler. |

Taslak olmayan bir mesajda: `409 not_a_draft`. Taslaklar `draft.created`, `draft.updated` ve
`draft.deleted` olaylarını üretir.

### Olay akışı ve gerçek zamanlı bağlantı

| Kural | Ayrıntı |
|---|---|
| Saklama | 7 gün. Daha eski bir konum `410 cursor_expired` döner; gerçek zamanlı bağlantı `resync_required` gönderir. Yeniden yükleyin ve `GET /v1/events/latest`'ten tekrar başlayın. |
| `after=0` | Saklanan en eski olaydan başlar. |
| Sayfalar | `has_more` true iken `limit`'ten az olay içerebilir. |
| Yetkiler | `conversations:read`; kişi olayları için ayrıca `contacts:read`. |
| Yazıyor | `typing` çerçevelerinin `id`'si yoktur ve tekrar oynatılmaz. |

### Konuşmaları silme

`DELETE /v1/conversations/{conversationId}` bir konuşmayı Çöp kutusuna taşır. Bunu yalnızca sahipler ve
yöneticiler ile `conversations:delete` yetkisi olan anahtarlar ve bağlı uygulamalar yapabilir; temsilciler
`403 forbidden` alır. Gelen kutularıyla sınırlı bir anahtar yalnızca o gelen kutularında silebilir.
`yuva api-key create` yetkiyi siz adlandırmadıkça vermez.

| Kural | Ayrıntı |
|---|---|
| Çöp kutusu | Konuşma ekip, kişi, uygulamalar ve asistanlar için tüm listelerden, sayımlardan, aramalardan ve rakamlardan kaybolur; kimse onu değiştiremez. İki kez yapmak bir şey değiştirmez. |
| Çöp kutusunu görme | `GET /v1/conversations?trashed=true`; sahipler, yöneticiler ve `conversations:delete` içindir. Konuşmada `trashed_at` ve `deletes_at` olur. |
| Geri alma | 30 gün içinde `POST /v1/conversations/{conversationId}/restore`; durumu, atanan kişisi ve etiketleriyle geri gelir. Sonrasında `409 trash_expired` yanıtı alınır. |
| 30 gün sonra | Saatlik temizlik onu mesajları, ekleri ve saklanan dosyalarıyla kalıcı olarak siler. |
| Birçoğu birden | `POST /v1/conversations/bulk` ile yalnızca `trashed: true` veya `false`, en fazla 100. |
| Kişi yeniden yazarsa | E-posta, sohbet veya uygulamadan yeni bir konuşma başlar; Çöp kutusundaki konuşma asla yeniden açılmaz. |

Panelde sahipler ve yöneticiler Sil'i konuşmanın menüsünde ve bir seçim için bulur, kısayol `#`'dir; geri almak
için Çöp kutusu sekmesi vardır.

Webhook'lar `conversation.trashed` ve `conversation.restored` (konuşma ve kişi) olaylarını, 30 gün sonra da
yalnızca `id`, `inbox_id` ve `contact_id` taşıyan `conversation.deleted` olayını gönderir.
[Denetim günlüğü](operations.md#denetim-günlüğü) her konuşmayı kimin Çöp kutusuna taşıdığını veya geri aldığını,
son silmeyi ise sistemin yaptığını kaydeder.

### Bir mesajı kaldırma

`POST /v1/messages/{messageId}/redact` tek bir mesajın veya notun içeriğini kaldırır ve konuşmada bir satır bırakır:
zamanıyla birlikte "<ad> tarafından kaldırıldı". Yanıt, mesajla birlikte `200` döner; mesajda `redacted_at`,
`redacted_by`, boş bir `body` olur, `html`, ek, kart ve bahsetme olmaz. Sahipler ve yöneticiler her mesajı
kaldırabilir, diğer üyeler yalnızca kendi yazdıkları mesaj ve notları (`403 forbidden`). Anahtarın `messages:redact`
yetkisi olmalıdır ve görebildiği her mesajı kaldırabilir; `yuva api-key create` yetkiyi siz adlandırmadıkça vermez.

| Kural | Ayrıntı |
|---|---|
| Kaldırılan | Metin, HTML, ekler ve saklanan dosyaları; e-postada ayrıca konu, başlıklar ve özgün ileti; hepsi tek adımda. Dosyaları hemen ardından bir iş siler. |
| Kalan | Yazar, zaman, konuşmadaki yer ve e-postada gönderim durumu. |
| Diğer kopyalar | Önceki olaylar, saklanan webhook teslimatları ve önizlemeler içeriği kaybeder; mesaj aramadan çıkar. |
| Kişinin alıntısı | Kişinin sonraki bir e-postası kaldırılan bir e-postayı yanıtlayıp tamamını alıntılıyorsa, alıntı ve o e-postanın özgün hali de kaldırılır; yeni yazdıkları kalır. Kısmi alıntı kalır. |
| İlk mesaj | Bir geri bildirimin ilk mesajı kaldırılırsa `feedback.text` temizlenir; bir konuşmanın konusunu veren e-posta kaldırılırsa konu temizlenir. İkisi de `conversation.updated` gönderir. |
| Henüz gönderilmemiş e-posta | Sıradaki e-posta gönderilmez ve `failed` görünür. |
| Gönderilmiş e-posta | Alıcının posta kutusunda kalır: Yuva onu geri alamaz. |
| Yeniden | Kaldırılmış bir mesajı yeniden kaldırmak onu değişmeden döndürür. |
| Kaldırılamayanlar | Taslak (`DELETE` ile atın) ve zaman çizelgesi olayı `409 not_redactable` döner. Çöp kutusundaki bir konuşma `404` döner. |

Webhook'lar eski içerik olmadan [`message.redacted`](webhooks.md#olaylar) gönderir; elinizdeki kopyaları silin.
[Denetim günlüğü](operations.md#denetim-günlüğü) her kaldırmayı kaydeder, metni asla.
Panelde **Kaldır**, sahipler, yöneticiler ve yazarı için her mesaj ve notta bulunur. Onay penceresi sonucu söyler;
gönderilmiş bir e-postanın alıcının posta kutusunda kaldığını da belirtir. Odaktaki bir mesajda `Delete` tuşu da aynı
pencereyi açar. Konuşmayı açık tutan herkes, zamanıyla birlikte "<üye> tarafından kaldırıldı" satırını canlı görür.


### Idempotency

`/v1` ve `/client/v1` üzerindeki her kimlik doğrulamalı `POST`, `Idempotency-Key` kabul eder (1 ile
255 arası yazdırılabilir ASCII karakter); çağıran başına 24 saat saklanır. `5xx` yanıtları
saklanmaz. Giriş, kişi oturumu isteği ve `/v1/me/…` bunu yok sayar.

| Yeniden deneme | Yanıt |
|---|---|
| Aynı anahtar, aynı istek | İlk yanıt, `Idempotent-Replayed: true` ile |
| Aynı anahtar, farklı istek | `409 idempotency_key_reused` |
| Aynı anahtar, ilki hâlâ çalışıyor | `409 idempotency_key_in_use` |

### Yalnızca API sunan sunucular

| Değişken | Etkisi |
|---|---|
| `YUVA_PANEL=off` | Panel ve OAuth onay sayfası yok; API anahtarları çalışmaya devam eder. |
| `YUVA_WIDGET=off` | `/yuva.js` ya da `/yuva-chat.js` yok; kişi API'si kalır. |

Bkz. [Yapılandırma](configuration.md#sunucu).
