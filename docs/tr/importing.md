# Abone içe aktarma

Sizden haber almayı zaten kabul etmiş kişileri bir CSV dosyası yükleyerek bir [e-posta listesine](lists.md)
getirin. Yuva bir şey eklemeden önce her adresi denetler ve kişilerin nerede kabul ettiğinin kaydını tutar.

İçe aktarmayı sahipler ve yöneticiler yapabilir. Bu sayfadaki her şey [API](#arka-uçtan) ile de yapılabilir.

## 1. Dosyayı hazırlayın

Tablonuzu ya da dışa aktarımınızı bir başlık satırıyla CSV olarak kaydedin.

| Sınır | Değer |
|---|---|
| Boyut | 20 MiB |
| Satır | 100.000 veri satırı (sunucu daha düşük bir sınır koyabilir, [`YUVA_IMPORT_MAX_ROWS`](configuration.md#bültenler)) |
| Kodlama | UTF-8 (BOM'lu ya da BOM'suz), BOM'lu UTF-16, Windows-1254 (Türkçe Excel), Windows-1252 (Batı dilleri Excel) |
| Ayırıcı | Virgül, noktalı virgül ya da sekme; başlık satırından bulunur |

Hiçbir şeyi dönüştürmeniz gerekmez: noktalı virgülle kaydedilmiş bir Türkçe Excel dosyası harfleri bozulmadan
içe aktarılır. Daha büyük bir dosyayı bölmeniz gerekir.

## 2. Yükleyin

Panelde: Bültenler, sonra Listeler; listeyi açın, sonra İçe aktarma sekmesi. Sihirbazın dört adımı vardır:
dosya ve onay, sütunlar, gözden geçirme, içe aktarma. Her adımda sayfadan çıkabilirsiniz.

**Dosya ve onay.** Dosyayı seçin, sonra:

- Dosyadaki herkesin bu listeyi gelen kutunuzdan almayı kabul ettiğini ve bunun nasıl ve ne zaman olduğunu
  gösterebileceğinizi belirten ifadeyi işaretleyin.
- Nerede ve ne zaman kabul ettiklerini yazın, örneğin "Mağazamızın ödeme sayfasındaki bülten kutucuğu,
  2023–2026".

İkisi de zorunludur; olmadan yükleme reddedilir. Yuva bunları içe aktarmayla ve oluşturduğu her aboneliğin
izin kaydıyla birlikte saklar; böylece listenin [izin kayıtları dışa aktarımında](lists.md)
görünürler. Bunlar sizin kanıtınızdır ve adresini nereden aldığınızı soran birine göstereceğiniz şeydir.
Gerçek kaynağı yazın: kişilerin kabul ettiğinden emin değilseniz onları içe aktarmayın.

**Herkesten yeniden onay iste.** İzin eskiyse ya da emin değilseniz açın. Kimse hemen abone olmaz: her yeni
adres `pending` olur ve listenin onay e-postasını alır, ancak bağlantıya tıklayınca katılır. E-postalar
çalışma alanının günlük gönderim sınırına göre günlere yayılır; büyük bir dosya tek seferde gitmez. Bunun için
gelen kutusunda gönderebilen bir e-posta kanalı gerekir.

"Yükle ve denetle"ye basın. Dosya okunur ve her adres denetlenir; henüz hiçbir şey eklenmez.

## 3. Sütunlar

Yuva her sütunda ne olduğunu başlığından tahmin eder. Denetleyin: tam olarak bir sütun e-posta adresi olmalı;
diğerleri ad (ya da ad ve soyad), dil, dış kimlik, adlandırdığınız bir özellik (harf, rakam, nokta, tire ve
alt çizgi; en fazla 64 karakter) olabilir ya da atlanır. Özellikler yalnızca kişide henüz olmayan anahtarları
doldurur; yani içe aktarma, bildiğiniz bir şeyin üzerine yazmaz. Bir kişi [özel alanıyla](custom-fields.md) aynı adı taşıyan özellik alanın türüne göre okunur, değer uymuyorsa dışarıda bırakılır. Bir sütunu değiştirin ve "Bu sütunlarla yeniden
denetle"ye basın.

## 4. Gözden geçirme

Başlamadan önce sayfa ne olacağını söyler: kaç yeni abone, kaç yeni kişi oluşturulacak, kaçı zaten listede
(olduğu gibi bırakılır) ve neler dışarıda kalıyor:

| Dışarıda kalan | Neden |
|---|---|
| Geçerli bir e-posta adresi değil | Adres bozuk. |
| Alan adı e-posta almıyor | Alan adının posta sunucusu (MX, A ya da AAAA kaydı) yok ya da null MX yayımlıyor. |
| Rol adresleri | `postmaster@`, `abuse@` ya da `noreply@` gibi, hiçbir kişiye ulaşmayan adresler. |
| Dosyada daha önce geçiyor | Tekrar. İlki kullanılır. |
| Daha önce geri döndü ya da spam olarak bildirildi | Adres, çalışma alanının tüm e-postaları için bastırılmış. |
| Her şeyden ayrıldı ya da silinmeyi istedi | Kişi tüm e-postalarınıza itiraz etti. |
| Bu listeden ayrıldı | Kişi bu listeden ayrıldı. Yalnızca kendisi, bir onayla geri dönebilir. |

Dosyada ne yazarsa yazsın, bir içe aktarma aboneliğinden ayrılmış, itiraz etmiş ya da geri dönmüş kimseyi geri
koymaz.

Başlatmak için "# adresi içe aktar"a basın.

## 5. Çalışırken

Sayfa biten satırları ve sayıları canlı gösterir. Satırlar 1.000'er 1.000'er eklenir. Çıkabilirsiniz; içe aktarma
sürer, aynı sekmedeki İçe aktarmalar öncekileri listeler. "İçe aktarmayı iptal et" kısa sürede durdurur:
şimdiye kadar eklenenler izin kaydıyla kalır ve yüklenen dosya silinir.

- Bir çalışma alanında **aynı anda yalnızca bir içe aktarma** çalışır. İkinci yükleme, birincinin bitmesini ya da
  iptal edilmesini bekler.
- **Hoş geldin e-postası gitmez**; yeniden onay istemeyi seçmediyseniz onay e-postası da gitmez. İçe aktarma
  her satır için bir [webhook](webhooks.md) ya da olay göndermez.
- Bittiğinde yüklenen dosya silinir.

### Dışarıda kalan satırlar

"Dışarıda kalan satırları indir", dosyanızdaki satır numarasını, adresi yazdığınız gibi ve nedeni içeren bir
CSV verir. Dosya içe aktarma bittikten sonra 7 gün kalır. Yükleyip hiç başlatmadığınız içe aktarma 7 gün sonra
iptal edilir.

| Neden | Anlamı |
|---|---|
| `invalid_email` | Geçerli bir e-posta adresi değil. |
| `no_mail_domain` | Alan adı e-posta almıyor. |
| `role_address` | Rol adresi. |
| `duplicate` | Dosyada daha önce geçiyor. |
| `suppressed` | Daha önce geri döndü ya da spam olarak bildirildi. |
| `opted_out` | Her şeyden ayrıldı ya da silinmeyi istedi. |
| `unsubscribed` | Bu listeden ayrıldı. |

Metin olmayan bir dosya `415 not_csv`, boyut sınırını aşan `413 file_too_large` yanıtı alır. Satır sınırını
aşan dosya başarısız olur ve hiçbir şey eklenmez.

## Arka uçtan

Bir seferde birkaç kişi ya da kayıt olurken eklenenler için
[`POST /v1/lists/{listId}/subscriptions`](lists.md#5-arka-uçtan-abone-ekleyin) kullanın. Bir dosya için içe
aktarmanın aynı adımları API ile de vardır. Anahtarın `lists:write` kapsamı olmalıdır.

```sh
curl -X POST https://support.example.com/v1/lists/LIST-ID/imports \
  -H "Authorization: Bearer $YUVA_API_KEY" \
  -F file=@subscribers.csv \
  -F 'attestation=Everyone in this file agreed to receive Newsletter from Example, and I can show how and when.' \
  -F 'consent_source=Newsletter checkbox on the checkout page, 2023 to 2026' \
  -F reconfirm=false
```

Yanıt, `validating` durumundaki içe aktarmadır. `state` `ready` olana kadar
`GET /v1/lists/{listId}/imports/{importId}` isteğini yoklayın: `counts` ve `columns` gözden geçirmenin
gösterdiğini, `preview` ilk satırları taşır. Sütunları `PATCH` ile değiştirin
(`{"columns": [{"index": 0, "field": "email"}, …]}`), sonra başlatın:

```sh
curl -X POST https://support.example.com/v1/lists/LIST-ID/imports/IMPORT-ID/start \
  -H "Authorization: Bearer $YUVA_API_KEY"
```

`done` olana kadar yeniden yoklayın; `rows_done` ilerledikçe artar ve `/v1/realtime`
`list_import.progress` ile `list_import.updated` gönderir.

## Başvuru

| Uç nokta | Kullanım |
|---|---|
| `GET`, `POST /v1/lists/{listId}/imports` | İçe aktarmaları sıralar; dosya yükler (`file`, `attestation`, `consent_source`, `reconfirm`, `mapping`). |
| `GET`, `PATCH`, `DELETE /v1/lists/{listId}/imports/{importId}` | Okur; sütunları değiştirir (yalnızca `ready` iken); iptal eder. |
| `POST /v1/lists/{listId}/imports/{importId}/start` | `ready` bir içe aktarmayı başlatır. |
| `GET /v1/lists/{listId}/imports/{importId}/errors` | Dışarıda kalan satırlar, CSV olarak. |

Durumlar: `validating`, `ready`, `importing`, `done`, `failed` (`error`'a bakın), `cancelled`. Ele almaya değer
hatalar: `422 attestation_required`, `409 import_running`, `409 email_channel_required` (gönderebilen kanal
olmadan yeniden onay), `409 not_ready`, `422 email_column_required`, `413`, `415`. Okumak `lists:read`;
yüklemek, değiştirmek, başlatmak ve iptal etmek `lists:write` ister. Tam şekiller
[API sözleşmesinde](../../openapi/openapi.yaml).
