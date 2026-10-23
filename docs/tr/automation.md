# Otomasyon kuralları

Otomasyon kuralı, bir konuşmada bir şey olduğunda kendiliğinden çalışır: koşulları denetler, ardından sizin
yerinize etiketler, atar, yanıtlar ya da bir webhook çağırır. Kurallar `/v1/automation-rules` altındaki API ile
yönetilir; tüm örnekler bir [API anahtarı](headless.md#yetkiler) kullanır.

## 1. Kural oluşturun

Kuralları sahipler, yöneticiler ve gelen kutularıyla sınırlı olmayan anahtarlar oluşturur. Bu kural, mesai
saatleri dışında açılan her yeni e-posta konuşmasına etiket ekler ve hazır yanıt gönderir:

```sh
curl -X POST https://support.example.com/v1/automation-rules \
  -H "Authorization: Bearer $YUVA_API_KEY" -H "Content-Type: application/json" \
  -d '{"name": "After hours",
       "trigger": "conversation_created",
       "conditions": [
         {"field": "channel_kind", "operator": "in", "values": ["email"]},
         {"field": "business_hours", "operator": "equals", "value": "outside"}
       ],
       "actions": [
         {"type": "add_labels", "label_ids": ["<label id>"]},
         {"type": "send_reply", "canned_reply_id": "<canned reply id>"}
       ]}'
```

Yanıt kuralın kendisidir, `201`. Sıranın sonuna girer ve kaydedildikten sonra olanlara tepki verir. Bir
kuralın adını verdiği her etiket, üye, ekip, hazır yanıt, webhook uç noktası, özel alan ve gelen kutusu var olmalıdır
(`400 validation_failed`). Bir çalışma alanında en fazla 100 kural olur (`403 automation_rule_limit`).

| Alan | Anlamı |
|---|---|
| `name` | 1 ile 100 karakter. |
| `enabled` | Varsayılan `true`. |
| `inbox_id` | Her gelen kutusundaki kural için boş bırakın; yalnızca o gelen kutusunun konuşmalarında çalışması için verin. |
| `trigger` | Kuralın ne zaman değerlendirileceği, bkz. [tetikleyiciler](#2-tetikleyiciler). |
| `idle_minutes` | 5 ile 43200. `idle` için zorunlu, diğer tetikleyicilerde reddedilir. |
| `conditions` | En fazla 20; hepsi eşleşmeli. Boş liste her konuşmayla eşleşir. |
| `actions` | 1 ile 10. |

`PATCH /v1/automation-rules/{automationRuleId}` yalnızca gönderdiklerinizi değiştirir; `conditions` ve `actions`
bütünüyle değiştirilir, `null` verilen `inbox_id` ve `idle_minutes` temizlenir.

| Çağrı | Ne yapar |
|---|---|
| `GET /v1/automation-rules` | Kuralları sırayla listeler: `{"items": [...]}`. |
| `POST /v1/automation-rules` | Bir kural oluşturur. |
| `GET`, `PATCH`, `DELETE /v1/automation-rules/{automationRuleId}` | Birini okur, değiştirir ya da siler. Silmek günlüğünü de siler; kuralın yaptıkları kalır. |
| `PUT /v1/automation-rules/order` | Sırayı belirler: `{"ids": [...]}` her kuralı bir kez içerir. |
| `GET /v1/automation-rules/{automationRuleId}/runs` | Kuralın [günlüğü](#7-çalıştırma-günlüğü). |

## 2. Tetikleyiciler

| Tetikleyici | Ne zaman çalışır |
|---|---|
| `conversation_created` | Bir konuşma oluşturuldu: bir kişi, üye ya da anahtar tarafından. |
| `message_received` | Kişi bir mesaj yazdı. İçe aktarılan mesajlar ve taslaklar sayılmaz. |
| `status_changed` | Durum değişti; bir kişinin mesajı ya da erteleme süresinin dolması konuşmayı yeniden açtığında da. |
| `idle` | Açık ya da bekleyen bir konuşmada `idle_minutes` boyunca hiçbir şey yazılmadı. |

`idle` her 5 dakikada bir denetlenir; kural en fazla birkaç dakika geç çalışır ve sessiz dönem başına bir kez:
bir sonraki dönem ancak biri yazdıktan sonra başlar. Bir kuralın kendi notu ya da yanıtı yeni bir sessiz dönem
başlatmaz. Sunucu 15 dakikadan uzun süre kapalıyken sessizleşen bir konuşma yakalanmaz. Spam, çöp kutusundaki ve
kapalı konuşmalar hiçbir zaman boşta sayılmaz; kural çalışmadan önce yazılan ya da kapatılan konuşmaya dokunulmaz.

## 3. Koşullar

Bir koşulun `field`, `operator` ve karşılaştırılacak değeri vardır: `in` ve `not_in` için `values`, `equals`,
`not_equals`, `contains` ve `not_contains` için `value`, `is_set` ve `is_not_set` için hiçbiri.

| Alan | İşleçler | Neyi karşılaştırır |
|---|---|---|
| `channel_kind` | `in`, `not_in` | `email`, `chat`, `app` ya da `api`. |
| `kind` | `in`, `not_in` | Konuşma türü. |
| `feedback_category` | `in`, `not_in` | Geri bildirim kategori anahtarları. |
| `priority`, `status` | `in`, `not_in` | Konuşmanın önceliği ya da durumu. |
| `subject`, `body` | `contains`, `not_contains` | Büyük/küçük harf duyarsız. `body` kuralı tetikleyen mesajdır; diğer tetikleyicilerde kişinin son mesajı. |
| `contact_attribute` | `equals`, `not_equals`, `contains`, `not_contains`, `is_set`, `is_not_set` | `key` ile adı verilen kişi özniteliği. |
| `custom_field` | aynı | O `key` ile konuşmanın [özel alanı](custom-fields.md). Sayılar, onay kutuları ve tarihler JSON metni olarak karşılaştırılır (`42`, `true`, `2026-01-31`). |
| `label` | `in`, `not_in` | `in`: etiketlerden herhangi birine sahip. `not_in`: hiçbirine sahip değil. |
| `team` | `in`, `not_in`, `is_set`, `is_not_set` | Ekip kimlikleri. |
| `assignee` | `is_set`, `is_not_set` | Atanmış bir üye olup olmadığı. |
| `business_hours` | `equals` | Gelen kutusunun mesai saatleri `inside` (içinde) ya da `outside` (dışında). Mesai saati olmayan gelen kutusu her zaman içindedir. |

`equals` birebir eşleşir; `contains` büyük/küçük harfe bakmaz. Koşullar konuşmanın kural çalıştığı andaki hâline
göre denetlenir; yani bir kural, öncekilerin değiştirdiğini görür.

## 4. Eylemler

| Eylem | Alanlar | Ne yapar |
|---|---|---|
| `add_labels`, `remove_labels` | `label_ids` (en fazla 50) | Etiket ekler ya da kaldırır. Aynı etiketi paylaşamazlar. |
| `assign_member` | `member_id` | Gelen kutusuna erişimi olan bir üyeyi atar. |
| `assign_team` | `team_id` | Gelen kutusuna yetkili bir ekibi atar. |
| `unassign` | yok | Atanmış üyeyi kaldırır. |
| `assign_pool` | yok | Kabul edilir ama henüz bir şey yapmaz: çalıştırma onu `skipped` olarak listeler. |
| `set_priority` | `priority` | Önceliği ayarlar. |
| `set_status` | `status` | `open`, `pending` ya da `closed`. Kapatmak için konuşmanın zorunlu özel alanları gerekir. |
| `snooze` | `snooze_minutes` (1 ile 525600) | Konuşmayı erteler. |
| `add_note` | `body` (en fazla 10000 karakter) | İç not ekler. |
| `send_reply` | `canned_reply_id` | Hazır yanıtın metnini kişiye gönderir, bkz. [yanıtlar](#6-yanıtlar). |
| `send_webhook` | `webhook_id` | O uç noktaya [`automation.triggered`](#8-webhook-eylemi) gönderir. |

Her tür bir kuralda en fazla bir kez, yalnızca kendi alanlarıyla yer alır (aksi hâlde `400 validation_failed`).
`set_status` ile `snooze` birbirini dışlar; `assign_member`, `assign_pool` ve `unassign` da öyle.

Bir kuralın içinde eylemler, siz hangi sırayla yazarsanız yazın sabit bir sırayla çalışır: not, yanıt, ardından
etiketler, atanan, ekip, öncelik ve durum ya da erteleme birlikte, en sonda webhook. Bir kuralın bir konuşmada yaptığı
her şey zaman çizelgesine `system` yazarıyla yazılır ve her mesaj o andaki kuralın `id` ve `name` değerlerini
taşıyan `rule` alanını içerir. Bir çalıştırma ayrıca eylemleri ve sonuçlarını listeleyen bir `rule_applied` olayı ekler.

## 5. Sıra ve döngüler

Kurallar `position` ile sıralanmış tek bir listededir; çalışma alanı ve gelen kutusu kuralları aynı listeyi paylaşır.
Bir tetikleyici için uygulanan etkin kurallar bu sırayla çalışır ve her kural tetikleyici başına en fazla bir kez,
sunucu yeniden denese bile: bir kez çalışmış kural, eşleşmiş olsun olmasın, o tetikleyici için bir daha çalışmaz.

Bir kuralın kendi değişiklikleri hiçbir kuralı tetiklemez; yani kurallar döngüye giremez: eklediği etiket, ayarladığı
durum ya da yazdığı mesaj hiçbir şey başlatmaz. Bir kuralın kurduğu erteleme için de aynıdır: erteleme bitip konuşma
yeniden açıldığında uyanma için hiçbir kural çalışmaz.

## 6. Yanıtlar

`send_reply`, hazır yanıtın metnini otomatik yanıt olarak gönderir (e-postada `Auto-Submitted: auto-replied`
ile). Aşağıdaki durumlarda reddedilir ve çalıştırma `error: auto_reply` ile başarısız olur:

- kuralı tetikleyen mesaj bir otomatik yanıt ya da geri dönen e-postadır (bounce);
- kişi engellenmiştir;
- konuşma spamdır;
- bir kural bu konuşmada son bir saat içinde ya da son 24 saatte üç kez zaten yanıt verdi.

Bir kuralın müşterilere e-posta gönderip gönderemeyeceği ayrıca `bots_may_send` çalışma alanı ayarına bağlıdır
([Botlar gönderebilir](headless.md#botlar-gönderebilir)). Kapalıyken yanıt yalnızca hem kural hem de hazır yanıt,
kural çalıştığında hâlâ sahip ya da yönetici olan bir üye tarafından en son kaydedilmişse gider; aksi hâlde çalıştırma
`bot_sending_disabled` ile başarısız olur ve `detail` hangisinin eksik olduğunu söyler.

İkisinde de `saved_by` vardır (`type` ve `member_id` ya da `api_key_id`):

- Bir kuralın `saved_by` değeri, onu oluşturan ya da `trigger`, `idle_minutes`, `inbox_id`, `conditions`, `actions`
  veya `enabled` alanlarını en son değiştirendir. Yeniden adlandırma ve yeniden sıralama onu değiştirmez.
- Bir hazır yanıtın `saved_by` değeri, onu oluşturan ya da metnini en son değiştirendir. Kaydedildiği zaman
  henüz tutulmadığı için eski yanıtlarda yoktur; `bots_may_send` kapalıyken bir kuralın bunu gönderebilmesi için
  bir sahip ya da yöneticinin yanıtı yeniden kaydetmesi gerekir.
- API anahtarları ve bağlı uygulamalar, bir sahip yapmış olsa bile bot sayılır.

## 7. Çalıştırma günlüğü

Bir kural bir tetikleyici için her değerlendirildiğinde bir çalıştırma bırakır; 30 gün saklanır:

```sh
curl -H "Authorization: Bearer $YUVA_API_KEY" \
  "https://support.example.com/v1/automation-rules/$RULE_ID/runs?outcome=failed"
```

En yeni önce; sayfalama için `limit` ve `next_cursor`. `outcome` tek bir değere göre süzer.

| Sonuç | Anlamı |
|---|---|
| `applied` | Koşullar eşleşti ve eylemler uygulandı. |
| `failed` | Bir eylem yapılamadı. O kuralın hiçbir şeyi uygulanmadı. `error` sorun kodunu, `detail` bir cümle içerir. |
| `not_matched` | Koşullar eşleşmedi. Eylem yok. |

Her çalıştırmada `conversation_id`, `trigger` ve `actions` vardır: eylem başına bir `type` ve `result`. Sonuç
`applied`, `unchanged` (konuşma zaten öyleydi), `skipped` (`assign_pool`) ya da `not_run` (çalıştırma başarısız oldu) olur.

Sık görülen `error` değerleri:

| Hata | Nedeni |
|---|---|
| `auto_reply` | Müşteriyi korumak için yanıt reddedildi, bkz. [yanıtlar](#6-yanıtlar). |
| `bot_sending_disabled` | `bots_may_send` kapalı ve kural ya da hazır yanıt en son bir sahip veya yönetici tarafından kaydedilmemiş. |
| `field_required` | Kapatmak için bir özel alan değeri gerekir. |
| `team_access`, `validation_failed` | Ekibin gelen kutusuna erişimi yok ya da üyenin yok. |
| `email_undeliverable`, `email_not_configured` | Yanıt e-postayla gönderilemedi. |
| `label_gone`, `canned_reply_gone`, `webhook_unavailable` | Eylemin adını verdiği şey silindi ya da uç nokta kapalı veya başka bir gelen kutusuyla sınırlı. |

Silinmiş bir etiketi, üyeyi, ekibi, hazır yanıtı ya da uç noktayı anan bir kural değiştirilmez: onu gerektiren
eylemleri başarısız olur. Silinmiş bir etiket veya ekip üzerindeki koşul hiç eşleşmez (`in`) ya da her zaman eşleşir
(`not_in`). Bir gelen kutusunu silmek onun kurallarını da siler.

## 8. Webhook eylemi

`send_webhook`, adını verdiği [webhook uç noktasına](webhooks.md) diğer teslimatlar gibi imzalanmış bir
`automation.triggered` olayı gönderir. Uç nokta bunu `events` içinde listeleyemez (orada reddedilir); kapalı ya da
başka bir gelen kutusuyla sınırlı olmadıkça `events` değerinden bağımsız olarak alır. Kurallar URL veya sır saklamaz.
`data` şunları içerir: `rule` (`id`, `name`), `trigger`, kuralın diğer eylemlerinden sonraki hâliyle `conversation` ve `contact`.

## 9. Hız sınırı

Bir çalışma alanının kuralları dakikada en fazla 600 eylem uygular. Bir kural sınırı aşacaksa başarısız olmaz,
sonraki dakikayı bekler; sonraki kurallar da onunla bekler. Koşulları eşleşmeyen kurallar hiçbir şeye mal olmaz.

## 10. Erişim

| Kapsam | İzin verir |
|---|---|
| `automations:read` | Kuralları ve günlüklerini okur. |
| `automations:manage` | Kural oluşturur, değiştirir, sıralar ve siler. Yalnızca sahipler ve yöneticiler; gelen kutularıyla sınırlı anahtarlarda reddedilir; başka hiçbir kapsama dahil değildir. |

Aracılar ve bazı gelen kutularıyla sınırlı anahtarlar çalışma alanı kurallarını ve kendi gelen kutularının kurallarını
okuyabilir (diğerleri `404`), yalnızca kendi gelen kutularındaki konuşmaların çalıştırmalarını görür. Yazamazlar:
`403 forbidden`. Bir kural kapsadığı her gelen kutusunda çalışma alanının yetkisiyle davrandığı için
`automations:manage` yetkisini dikkatle verin.

Bir kuralı oluşturmak, değiştirmek ve silmek [denetim günlüğüne](operations.md#denetim-günlüğü)
`automation_rule.created`, `automation_rule.updated` ve `automation_rule.deleted` olarak, kuralın adı,
tetikleyicisi, gelen kutusu, eylem türleri ve webhook uç noktalarının ana makineleriyle (URL'leriyle asla) yazılır.

Otomasyon kuralları [MCP](mcp.md) asistanlarına bilerek sunulmaz.
