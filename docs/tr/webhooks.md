# Webhook'lar

Bir şey olduğunda Yuva backend'inize imzalı bir HTTP isteği gönderir; böylece push gönderebilir,
kişileri eşitleyebilir ya da otomasyon kurabilirsiniz. İstekler
[Standard Webhooks](https://www.standardwebhooks.com) biçimindedir.

## Uç nokta ekleyin

Panelde: Ayarlar → Webhook'lar. Ya da `webhooks:manage` yetkisi olan bir API anahtarıyla:

```sh
curl -X POST https://support.example.com/v1/webhooks \
  -H "Authorization: Bearer $YUVA_API_KEY" -H "Content-Type: application/json" \
  -d '{"url": "https://api.example.com/yuva/webhook", "events": ["message.created"]}'
```

Yanıttaki imzalama anahtarını (`whsec_…`) kaydedin; yalnızca bir kez gösterilir.

## Webhook imzasını doğrulayın

Her isteği ayrıştırmadan önce **ham** gövdesi üzerinden kontrol edin. Go ile:

```go
import "github.com/productdevbook/yuva/sdk/go/webhook"

err := webhook.Verify(os.Getenv("YUVA_WEBHOOK_SECRET"), r.Header, body, 5*time.Minute)
```

Herhangi bir Standard Webhooks kütüphanesi de çalışır.

## Hızlı yanıt verin

10 saniye içinde `2xx` döndürün. Başarısız istekler yaklaşık 24 saat boyunca yeniden denenir. Bir
teslimat iki kez ya da sırasız gelebilir: işlediğiniz bir `webhook-id`'yi atlayın ve `timestamp`'e
göre sıralayın.

## Başvuru

### Olaylar

| Tür | Ne zaman | `data` |
|---|---|---|
| `conversation.created` | Bir konuşma başladı, geri bildirimler dahil | `conversation`, `contact` |
| `conversation.updated` | Durum, atanan kişi, öncelik, etiketler, konu ya da spam işareti değişti veya konuşma taşındı | `conversation`, `contact` |
| `conversation.rated` | Kişi konuşmayı değerlendirdi (`conversation.rating`) | `conversation`, `contact` |
| `message.created` | Kişiden gelen ya da kişiye giden bir mesaj; açıksa notlar da | `message`, `conversation`, `contact` |
| `feedback.created` | Bir uygulamadan ya da `POST /v1/feedback` ile gelen geri bildirim | `message`, `conversation`, `contact` |
| `contact.updated` | Bir kişi değişti | `contact` |
| `contact.deleted` | Bir kişi silindi ya da birleştirildi (`merged_into_id`) | `contact` |
| `draft.created`, `draft.updated`, `draft.deleted` | Bir [taslak](headless.md#taslakla-yanıtlayan-bot) değişti; gönderilmesi `message.created` üretir | `message`, `conversation`, `contact` |

Her gövdede `type`, `timestamp` (olayın zamanı), `workspace_id`, `inbox_id` (kişi olaylarında yok)
ve `data` bulunur. Kişinin canlı bağlantısı yoksa `data.contact.online` `false` olur:
[push](mobile.md#4-anlık-bildirimler) göndermenin zamanı. Tam şemalar
[openapi.yaml](../../openapi/openapi.yaml) içindeki `webhooks` bölümündedir.

### Uç nokta alanları

| Alan | Anlamı |
|---|---|
| `url` | Herkese açık `http` ya da `https`. `YUVA_WEBHOOK_ALLOW_PRIVATE` atanmadıkça özel ve ayrılmış adresler reddedilir ([Yapılandırma](configuration.md#webhooklar)). |
| `events` | Gönderilecek olay türleri. |
| `inbox_id` | Yalnızca bu gelen kutusu; verilmezse tüm çalışma alanı. |
| `include_notes` | Üyelerin notlarını da gönderir. Varsayılan olarak kapalı. |

### Webhook imzası

| Başlık | Anlamı |
|---|---|
| `webhook-id` | Her yeniden denemede aynıdır. |
| `webhook-timestamp` | Unix saniyesi. Birkaç dakikadan fazla sapan değerleri reddedin. |
| `webhook-signature` | `v1,` + `base64(HMAC-SHA256(key, "<id>.<timestamp>.<raw body>"))`; `key`, `whsec_` sonrasının base64 çözülmüş hâlidir. Anahtar yenilenirken boşlukla ayrılmış iki imza gelir. |

### Teslimat

| Kural | Ayrıntı |
|---|---|
| Yeniden deneme | 5 sn, 1 dk, 5 dk, 30 dk, 1 sa, 2 sa, 4 sa, 8 sa, 9 sa sonra, %10'a kadar sapmayla. Yönlendirmeler başarısız sayılır. |
| Devre dışı | 24 saatlik başarısızlıktan ya da bir `410 Gone` yanıtından sonra. Panelden yeniden etkinleştirin. |
| Anahtarı yenileme | `POST /v1/webhooks/{webhookId}/secret`; eski anahtar 24 saat daha imzalamaya devam eder. |
| Kayıt | Son 100 deneme: `GET /v1/webhooks/{webhookId}/deliveries`, `GET /v1/webhooks/{webhookId}/attempts`. |
| Yeniden teslim | `POST /v1/webhooks/{webhookId}/deliveries/{deliveryId}/redeliver`. |
