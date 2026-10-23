# Özel alanlar

Özel alanlar, bir kez tanımladığınız ve her konuşmada ya da kişide tuttuğunuz türlü değerlerdir:
sipariş numarası, plan, ürün alanı. API'de, webhook'larda ve MCP'de geri gelirler; bir konuşma alanı
da konuşma kapanmadan önce zorunlu kılınabilir.

## 1. Alan tanımlayın

Alanları sahipler ve yöneticiler ya da `fields:manage` yetkisi olan bir anahtar tanımlar:

```sh
curl -X POST https://support.example.com/v1/custom-fields \
  -H "Authorization: Bearer $YUVA_API_KEY" -H "Content-Type: application/json" \
  -d '{"object": "conversation", "key": "product_area", "name": "Product area",
       "type": "select", "options": ["Billing", "Login", "Other"], "required_on_close": true}'
```

Yanıt alandır, `201`. `object`, `key` ve `type` sonradan değişmez.

| Tür | Değer |
|---|---|
| `text` | 1 ile 1000 karakter arasında bir dize. |
| `number` | Sonlu bir JSON sayısı. |
| `date` | `YYYY-MM-DD` biçiminde bir dize. |
| `select` | Alanın `options` değerlerinden biri. |
| `checkbox` | `true` veya `false`. |

## 2. Konuşmada değer ayarlayın

`PATCH /v1/conversations/{conversationId}` ve `POST /v1/conversations/bulk`, anahtara göre değerlerden
oluşan bir nesne olan `fields` alır. Adını verdiğiniz anahtarlar ayarlanır, diğerleri kalır ve `null`
birini temizler:

```sh
curl -X PATCH https://support.example.com/v1/conversations/$CONVERSATION \
  -H "Authorization: Bearer $YUVA_API_KEY" -H "Content-Type: application/json" \
  -d '{"fields": {"product_area": "Billing", "order_id": null}}'
```

Her anahtar bir konuşma alanı olmalı ve her değer türüne uymalıdır; aksi halde çağrı
`400 field_invalid` döner ve `field` anahtarı belirtir. Toplu istek, hiçbir şey değişmeden önce bütün
olarak denetlenir. Bir konuşmayı okumak, konuşmaları listelemek, webhook'lar ve olay akışı `fields`
gösterir (hiçbiri ayarlı değilse `{}`). Yalnızca `fields` değişirse `conversation.updated` gönderilir.
Anahtarın `conversations:write` yetkisi olmalı.

Listeyi `field.<key>=<value>` ile filtreleyin:

```sh
curl -G https://support.example.com/v1/conversations \
  -H "Authorization: Bearer $YUVA_API_KEY" \
  --data-urlencode "field.product_area=Billing" --data-urlencode "field.seats=5"
```

Sayı sayı olarak karşılaştırılır, onay kutusu `true` veya `false` alır, geri kalanı metinle tam
eşleşir. Birden çok anahtarın hepsi eşleşmelidir. Konuşma alanı olmayan bir anahtar, tekrarlanan bir
anahtar ya da türe uymayan bir değer `400 field_invalid` döner.

## 3. Kişiler

Bir kişi değerlerini `attributes` içinde tutar. Kişi alanı olan bir anahtar alana göre denetlenir;
diğer anahtarlar serbest biçimli kalır.

| Nerede | Uymayan bir değer |
|---|---|
| `/v1` (kişi oluşturma veya güncelleme, abonelik ekleme) | `400 field_invalid`, `field` anahtarı belirtir. `null` serbesttir ve değeri ayarsız bırakır. |
| [Kimlik belirteci](identity.md) `attrs` | Atılır; belirtecin geri kalanı kullanılır. |
| [Liste içe aktarma](importing.md) sütunları | CSV metni alanın türüne göre okunur (`12`, `true`); uymuyorsa atılır. |

## 4. Kapatmak için alanı zorunlu kılın

`required_on_close` ile (yalnızca konuşma alanları), bir konuşmayı alanın değeri olmadan kapatmak
`409 field_required` döner ve `field`, `position` sırasına göre eksik olan ilkini belirtir. Aynı
istekte ayarlanan değerler sayılır. Bu, `PATCH` ile kapatmada, toplu güncellemenin her konuşmasında
(`failed` içinde `field` ile listelenir), [makrolarda](headless.md#makrolar) ve bir mesajın `then`
değerinde, [MCP](mcp.md)'de `set_status` ve `bulk_update` için ve kapalı bir konuşmayı spam'den
çıkarırken geçerlidir. Spam bu denetim olmadan kapatılır, zaten kapalı bir konuşma yeniden
denetlenmez. Onay kutusu `true` veya `false` ile ayarlı sayılır.

## 5. Alanı değiştirin ya da silin

`PATCH /v1/custom-fields/{customFieldId}` `name`, `options` (liste değiştirilir), `required_on_close`
ve `position` değerlerini değiştirir. `DELETE` alanı kaldırır.

Bir konuşma alanını silmek ya da bir `select` seçeneğini kaldırmak, bu değerleri konuşmalardan arka
planda temizler ve değişen her konuşma `conversation.updated` gönderir. Temizlik bitene kadar aynı
anahtarla alan oluşturmak `409 field_clearing` döner; biraz sonra yeniden deneyin. Anahtarı ya da
seçeneği yeniden tanımlamak temizliği durdurur. Bir kişi alanı silindiğinde kişi öznitelikleri
korunur.

## Başvuru

| Uç nokta | Kullanım |
|---|---|
| `GET /v1/custom-fields` | Alanları listeler, isteğe bağlı `?object=conversation` veya `contact`. `{"items": [...]}`; nesneye, `position` değerine ve oluşturma zamanına göre sıralı. |
| `POST /v1/custom-fields` | Bir alan oluşturur. |
| `GET`, `PATCH`, `DELETE /v1/custom-fields/{customFieldId}` | Bir alanı okur, değiştirir ya da siler. |

Bir alanda `id`, `object`, `key`, `name`, `type`, `options`, `required_on_close`, `position`,
`created_at` ve `updated_at` bulunur.

| Öğe | Sınır |
|---|---|
| `key` | `^[a-z][a-z0-9_]{0,39}$`, nesne başına benzersiz (`409 field_taken`). |
| `name` | 1 ile 100 karakter. |
| `options` | Yalnızca `select`: 1 ile 100 karakterlik 1 ile 100 farklı dize. |
| `position` | 0 ile 10000. |
| Nesne başına alan | 50 (`403 field_limit`). |
| Bir `fields` içindeki anahtar | 50. |

| Yetki | İzin verir |
|---|---|
| `conversations:read` veya `contacts:read` | Alanları listeler ve okur. |
| `fields:manage` | Alanları oluşturur, değiştirir ve siler. Yalnızca sahipler ve yöneticiler; gelen kutularıyla sınırlı anahtarlar bunu taşıyamaz ve başka yetkiye dahil değildir: adıyla verin. Temsilciler `403 forbidden` alır. |
| `conversations:write`, `contacts:write` | Değer yazar. |
