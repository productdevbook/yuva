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
| `conversation.trashed`, `conversation.restored` | Bir konuşma [Çöp kutusuna](headless.md#konuşmaları-silme) taşındı veya geri alındı | `conversation`, `contact` |
| `conversation.deleted` | Çöp kutusu onu, taşındıktan 30 gün sonra kalıcı olarak sildi | yalnızca `id`, `inbox_id`, `contact_id` içeren `conversation` |
| `message.created` | Kişiden gelen ya da kişiye giden bir mesaj; açıksa notlar da | `message`, `conversation`, `contact` |
| `message.redacted` | Bir üye veya API anahtarı [bir mesajın içeriğini kaldırdı](headless.md#bir-mesajı-kaldırma); açıksa notlar da | `message`, `conversation`, `contact`; mesajda `redacted_at` ve `redacted_by` vardır, `body` boştur, `html` ve ek yoktur. Kopyanızı silin. |
| `feedback.created` | Bir uygulamadan ya da `POST /v1/feedback` ile gelen geri bildirim; uygulamadan gelende `feedback.score` (1 ile 5) olabilir | `message`, `conversation`, `contact` |
| `contact.updated` | Bir kişi değişti | `contact` |
| `contact.deleted` | Bir kişi silindi ya da birleştirildi (`merged_into_id`) | `contact` |
| `draft.created`, `draft.updated`, `draft.deleted` | Bir [taslak](headless.md#taslakla-yanıtlayan-bot) değişti; gönderilmesi `message.created` üretir | `message`, `conversation`, `contact` |
| `discussion.created` | Bir [tartışma](documentation-pages.md) başlığı açıldı; bir üyeyi beklerken de (`state: pending`) | `discussion` |
| `discussion.post_created` | Bir başlığa yanıt yazıldı; bir üyeyi beklerken de | `discussion`, `post` |
| `discussion.answered` | Bir yanıt cevap olarak kabul edildi | `discussion`, `post` |
| `discussion.updated` | Bir başlık ya da gönderi onaylandı, gizlendi, spam işaretlendi, düzenlendi, silindi, kilitlendi, kilidi açıldı ya da cevap kaldırıldı | `discussion`, bir gönderi değiştiyse `post` |
| `subscription.created` | Biri uç noktanın gelen kutusundaki bir [listeye](lists.md) abone oldu, her durumda (onay beklerken `pending`) | `subscription`, `list`, `contact` |
| `subscription.updated` | Bir abonelik onaylandı, iptal edildi (`unsubscribe_reason` ile) ya da yeniden abone olundu | `subscription`, `list`, `contact`, `previous_state` |
| `analytics.alert` | Bir [Analytics](analytics.md) web sitesi [uyarı](analytics-support.md#uyarılar) tetikledi: ani artış, düşüş, hedef düşüşü, destek yükü ya da yeni yönlendiren | `alert`, `property` |
| `broadcast.updated` | Bir [bülten](broadcasts.md) durum değiştirdi: zamanlandı, gönderiliyor, duraklatıldı, gönderildi, iptal edildi, başarısız oldu taslağa döndü ya da hızı değişti | `broadcast`, `previous_state` |
| `status.component.updated` | Bir [durum sayfası](status-pages.md) bileşeni oluşturuldu, değişti ya da yeni bir durum aldı | `component`, `page`, `previous_state` |
| `status.incident.created`, `status.incident.updated`, `status.incident.resolved` | Yayınlanmış bir olay ya da bakım yayınlandı, değişti veya çözüldü ya da tamamlandı | `incident`, `page`, `change`, `update` |
| `status.monitor.down`, `status.monitor.up` | Bir monitör düştü ya da düzeldi | `monitor`, `page`, `incident_id` |
| `help.article.published`, `help.article.unpublished`, `help.article.deleted` | Bir [yardım merkezi](help-center.md) makale çevirisi yayınlandı, yayından kaldırıldı ya da silindi (`locale` yoksa makalenin tamamı) | `center_id`, `article_id`, `number`, `locale`; yayınlandığında `title` ve `url` |
| `changelog.entry.published`, `changelog.entry.unpublished`, `changelog.entry.deleted` | Bir [Yenilikler](whats-new.md) kaydı yayınlandı (zamanlamayla ya da yayındaki bir çeviri değiştirildiğinde de), yayından kaldırıldı ya da yayındaki bir kayıt silindi | `changelog` (`id`, `name`, `url`), `entry` (`id`, `number`, `labels`, `published_at`, `translations`: `locale`, `title`, `url`) |
| `discussion.stage_changed` | Bir [yol haritası](roadmap-board.md) fikri başka bir aşamaya taşındı | `discussion`, `from`, `to`, `linked` (fikre bağlı geri bildirim sayısı) |

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
