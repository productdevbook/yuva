# SLA politikaları

Bir SLA politikası ekibinizin ne kadar hızlı yanıt vereceğini belirler: ilk yanıt, sonraki her yanıt ve
konuşmanın çözülmesi için bir süre. Yuva süreyi gelen kutusunun mesai saatlerine göre sayar, her
konuşmanın son tarihini ve durumunu gösterir, hedef kaçmadan önce uyarır ve kaçtığında haber verir.

## 1. Politika oluşturun

Politikaları sahipler ve yöneticiler ya da gelen kutularıyla sınırlı olmayan, `inboxes:manage` yetkili
bir anahtar oluşturur:

```sh
curl -X POST https://support.example.com/v1/sla-policies \
  -H "Authorization: Bearer $YUVA_API_KEY" -H "Content-Type: application/json" \
  -d '{"name": "Urgent", "priorities": ["urgent", "high"],
       "first_reply_minutes": 30, "next_reply_minutes": 120, "resolution_minutes": 1440,
       "warn_minutes": 15}'
```

Yanıt politikadır, `201`. Üç hedeften en az birini verin; dakikası olmayan hedef izlenmez. Dakikalar 1 ile
525600 arasındadır. `warn_minutes` 0 ile 1440 arasındadır (varsayılan 15; `0` uyarı göndermez).

| Alan | Anlamı |
|---|---|
| `name` | 1 ile 100 karakter. |
| `enabled` | Varsayılan `true`. Kapalı politika hiç uygulanmaz. |
| `inbox_ids`, `channel_kinds`, `label_ids`, `priorities` | Koşullar. Hepsi sağlanmalı; bir listede herhangi bir öğe eşleşir; boş liste her şeyle eşleşir. `channel_kinds` değerleri `email`, `chat`, `app` ve `api`. |
| `first_reply_minutes`, `next_reply_minutes`, `resolution_minutes` | Hedefler, mesai dakikası olarak. |
| `warn_minutes` | Uyarının son tarihten ne kadar önce gönderileceği. |

`PATCH /v1/sla-policies/{slaPolicyId}` gönderdiğinizi değiştirir; `null` bir hedefin izlenmesini
bırakır ve biri kalmak zorundadır. Bir çalışma alanında en fazla 100 politika olur (`403 sla_policy_limit`).

| Çağrı | Ne yapar |
|---|---|
| `GET /v1/sla-policies` | Politikaları sırayla listeler: `{"items": [...]}`. |
| `POST /v1/sla-policies` | Birini oluşturur; sona eklenir. |
| `GET`, `PATCH`, `DELETE /v1/sla-policies/{slaPolicyId}` | Birini okur, değiştirir veya siler. |
| `PUT /v1/sla-policies/order` | Sırayı belirler: `{"ids": [...]}` her politikayı bir kez adlandırır. |

Okumak `inboxes:read`, yazmak ve sıralamak `inboxes:manage` ister. Temsilciler ve bazı gelen kutularıyla
sınırlı anahtarlar okuyabilir, ancak yalnızca `inbox_ids` içermeyen politikaları ve kendi
gelen kutularından birini adlandıranları görür (diğerleri `404`; `inbox_ids` yalnızca gördüklerini
listeler). Yazamaz ya da sıralayamaz: `403 forbidden`.

## 2. Hangi politika uygulanır

Koşulları konuşmayla eşleşen, sıradaki ilk etkin politika uygulanır. Öncelik, etiket veya gelen kutusu
değişince politika yeniden seçilir. Zaten karşılanmış ya da kaçırılmış bir hedef sonucunu korur.

Yeni bir politika, SLA'sı olmayan konuşmalara bir sonraki değişiklikte ya da mesajda uygulanır. Politika
oluşturulmadan önce hiçbir hedef başlatmaz.

## 3. Hedefler

| Hedef | Başlar | Karşılanır |
|---|---|---|
| `first_reply` | Kişinin ilk mesajı. | Bir üyenin ya da botun herkese açık yanıtı. |
| `next_reply` | Bir yanıttan sonra gelen kişi mesajı. | Sonraki herkese açık yanıt. |
| `resolution` | Konuşma oluşturulur. | İlk kapatma. |

Notlar, taslaklar ve otomatik yanıtlar hiçbir hedefi karşılamaz. Konuşmayı kapatmak, hâlâ süren bir yanıt
hedefini sonuç üretmeden durdurur. Yeniden açılan konuşma yeni çözüm hedefi almaz; ilk kapatma geçerlidir.
Kapatmadan sonra gelen kişi mesajı bir sonraki yanıt hedefi başlatır.

## 4. Mesai saatleri ve saat dilimleri

Her hedef yalnızca gelen kutusunun açık olduğu süreyi, gelen kutusunun saat diliminde sayar (gelen
kutusunun `timezone` ve `business_hours` alanları). Mesai saatleri kapalıysa ya da aralıksız açıksa her saat
sayılır. Saat değişiklikleri işlenir: var olmayan bir duvar saati atlamanın sonuna kayar.

## 5. Duraklatma

`pending` ve `snoozed` çalışan her hedefi duraklatır ve kalan süreyi saklar; `open` bu süreyle sürdürür.
Ertelenmiş bir konuşma `snooze_until` anında kendiliğinden yeniden açılır, `conversation.updated` gönderir
ve saati sürdürür. Spam ya da çöp kutusundaki konuşma ertelenmiş kalır.

## 6. Politika uygulanmayı bırakınca

Spam, çöp kutusu, kapatılan, silinen ya da artık eşleşmeyen bir politika ve koşullarını daraltan bir
düzenleme SLA'yı düşürür: `sla` konuşmadan kalkar ve `conversation.updated` gönderilir. Başka bir politika
eşleşirse devralır ve kullanılan mesai süresini korur. Bir politika sonra yeniden uygulanırsa, hiçbirinin
uygulanmadığı süre sayılmaz ve hiçbir hedef ikinci bir sonuç almaz.

Yeni dakikalar sonradan başlayan hedeflere uygulanır; çalışan hedefler son tarihlerini korur. Bir etiketi ya
da gelen kutusunu silmek onu adlandıran politikalardan çıkarır ve listesi boşalan politikayı kapatır; böylece
politika her şeyle eşleşmeye başlamaz.

## 7. Uyarılar ve ihlaller

- Son tarihten `warn_minutes` önce `conversation.sla_warning` gönderilir; hedef başladığında ya da
  sürdürüldüğünde o an geçmişse gönderilmez.
- Son tarihte `conversation.sla_breached` gönderilir. Kaçırılan hedef karşılanana (`met_at` geç yanıtı
  kaydeder), konuşma duraklatılana ya da kapanana kadar kaçırılmış kalır.
- Her biri hedef başına bir kez gönderilir, ardından yeni `sla` ile `conversation.updated` gelir.

İkisi de [olay akışına ve gerçek zamanlı bağlantıya](headless.md#olay-akışı-ve-gerçek-zamanlı-bağlantı) ve
[webhook'lara](webhooks.md#olaylar) gider. `data`, `{target, due_at, conversation}` içerir; webhook ayrıca
`contact` ekler.

Üyeler de bildirim alır, [bildirim ayarlarındaki](#üye-bildirimleri) iki tür aracılığıyla.

### Üye bildirimleri

| Tür | Varsayılan | Ne zaman gönderilir |
|---|---|---|
| `sla_warning` | Push | Bir hedefin süresi dolmak üzere. |
| `sla_breached` | Push ve hemen e-posta | Bir hedef kaçırıldı. |

Atanan üyeye bildirilir; yoksa konuşmanın ekibinin üyelerine; ikisi de yoksa kimseye. Üyenin gelen
kutusuna hâlâ erişimi olmalıdır. `away` durumundaki üye kendi konuşmaları için yine bildirim alır, ekibinin
konuşmaları için almaz. Üyeler türleri `PATCH /v1/me/notifications` ile değiştirir (yalnızca üye
oturumları).

## 8. Durumu okuyun

Bir politika uygulandığında konuşma `sla` taşır:

```json
{"policy_id": "…", "state": "due_soon", "due_at": "2026-03-04T10:30:00Z",
 "targets": [{"target": "first_reply", "state": "due_soon", "started_at": "2026-03-04T09:45:00Z",
              "due_at": "2026-03-04T10:30:00Z", "warned_at": "2026-03-04T10:15:00Z"}]}
```

| Alan | Anlamı |
|---|---|
| `state` | `on_track`, `due_soon` (en erken çalışan hedefin süresi `warn_minutes` içinde doluyor), `breached`, `met` veya `paused`. |
| `due_at` | Çalışan bir hedefin en erken son tarihi; hiçbiri çalışmıyorsa yok. |
| `targets[].target` | `first_reply`, `next_reply` veya `resolution`. Yalnızca en son sonraki yanıt hedefi tutulur. |
| `targets[].state` | `on_track`, `due_soon`, `breached`, `met`, `paused` veya `stopped` (yanıttan önce kapandı). |
| `targets[]` | Ayrıca `started_at`, çalışırken `due_at`, duraklatılmışken `remaining_seconds`, `met_at`, `breached_at` ve `warned_at`. |

`GET /v1/conversations` `sort=sla_due` alır: çalışan hedefi olan konuşmalar önce, en erken `sla.due_at` en
başta (kaçırılanlar öne geçer), sonra kalanlar son etkinliğe göre. İmleç yalnızca geldiği sıralamayı sürdürür.
`sla_state` konuşmaları bir ya da daha çok durumla sınırlar (tekrarlayın ya da virgülle ayırın); SLA'sı
olmayan konuşmalar hiç eşleşmez.

```sh
curl -G https://support.example.com/v1/conversations \
  -H "Authorization: Bearer $YUVA_API_KEY" \
  --data-urlencode "sort=sla_due" --data-urlencode "sla_state=breached,due_soon"
```

`GET /v1/stats` içindeki `sla`, penceredeki karşılanan (`met`) ve kaçırılan (`breached`) hedefleri toplam,
gelen kutusu (`inboxes`) ve üye (`members`) başına verir; üye, sonucun olduğu andaki atanan kişidir. Atanmamış
sonuçlar yalnızca toplamda ve gelen kutusu başına sayılır.

## Asistanlar

[MCP](mcp.md) `search_conversations`, `sla_state` ve `sort` alır; o ve `get_conversation` `sla` döndürür.
Politikaları yöneten bir araç yoktur.
