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
const { data } = await api.GET("/v1/conversations", { params: { query: { status: "open" } } });
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
| `messages:write` | Mesajlar ve taslaklar: oluşturma, düzenleme, atma |
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
