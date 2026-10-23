# Otomatik atama

Otomatik atama, üyesi olmayan bir konuşmayı müsait birine verir; ekip çevrimiçiyken hiçbir şey atanmamış
listede beklemez. Gelen kutusu ya da [ekip](headless.md#ekipler) başına çalışır; sırayla (round robin) ya da
en az açık konuşması olana göre dağıtır, isteğe bağlı olarak üye başına üst sınır koyabilirsiniz.

## 1. Açın

Bir gelen kutusunda ya da ekipte `assignment_mode` ayarlayın. İkisi de `assignment_cap` alır.

```sh
curl -X PATCH https://support.example.com/v1/inboxes/$INBOX \
  -H "Authorization: Bearer $YUVA_API_KEY" -H "Content-Type: application/json" \
  -d '{"assignment_mode": "least_open", "assignment_cap": 20}'
```

| Alan | Değerler |
|---|---|
| `assignment_mode` | `off` (varsayılan), `round_robin` ya da `least_open`. |
| `assignment_cap` | 1 ile 500 arası ya da sınır yoksa alan yok. `PATCH` ile `null` sınırı kaldırır. |

Alanlar `Inbox`, `InboxCreate`, `InboxUpdate`, `Team`, `TeamCreate` ve `TeamUpdate` içindedir
(`PATCH /v1/inboxes/{inboxId}`, `PATCH /v1/teams/{teamId}` ve ilgili `POST` çağrıları). Yazmak için gelen
kutusunun ya da ekibin diğer alanlarıyla aynı yetki gerekir (`inboxes:manage`); bilinmeyen bir mod ya da 1
ile 500 dışında bir sınır `400 validation_failed` verir.

- `round_robin`, konuşmayı uygun üyeler arasında en uzun süredir konuşma almamış olana verir; hiç almamış
  olan önce gelir.
- `least_open`, en az açık konuşması olan uygun üyeye verir; eşitlikte en uzun süredir konuşma almamış olan
  seçilir.

Sınır, bir üyenin tutabileceği en fazla açık konuşma sayısıdır. Üyeye atanmış, `open` olan, spam olmayan ve
çöp kutusunda bulunmayan konuşmaları tüm çalışma alanı genelinde sayar. Elle atama sınıra bakmaz.

## 2. Kim uygundur

Bir üye, şu koşulların hepsi sağlanıyorsa konuşma için uygundur:

- konuşmanın gelen kutusuna kendi yetkisiyle ya da bir ekibin yetkisiyle erişebilir; sahipler ve
  yöneticiler için de yetki gerekir
- paneli ya da bir uygulamayı bağlıdır
- `availability` değeri `auto`dur (`PATCH /v1/me` ile `auto` ya da `away` ayarlanır)
- gelen kutusu mesai saatleri içindedir
- havuzun sınırının altındadır
- ekip havuzunda ekibin üyesidir

## 3. Hangi havuz

Ekibi olup üyesi olmayan bir konuşma ekibin ayarlarını ve üyelerini kullanır. İkisi de olmayan konuşma gelen
kutusununkini kullanır. Ekibin modu `off` ise hiçbir şey atanmaz; gelen kutusunun ayarları devreye girmez.
Zaten üyesi olan bir konuşmaya, yeniden açılma dışında (aşağıda) hiç dokunulmaz.

Konuşmalar oluşturulduklarında ve bir kişi onları yeniden açtığında ele alınır. Elle atamadan çıkarılan
konuşma atanmamış kalır.

## 4. Kimse müsait değilse

Konuşma bekler. `assign_waiting_since` alanını alır (bir zaman; aksi halde yok) ve bir `conversation.updated`
gönderilir. Biri uygun olur olmaz, en eskiden başlayarak atanır: bir üye bağlanır ya da `availability`
değerini `auto` yapar, bir üye konuşma kapatarak ya da devrederek boşalır, sınır yükseltilir, bir havuz
açılır ya da gelen kutusunun mesaisi başlar. Yuva ayrıca biri çevrimiçi olduğu sürece her dakika denetler.
Konuşma üye alınca, `open` olmaktan çıkınca, spam işaretlenince ya da çöpe atılınca alan temizlenir.

## 5. Yeniden açılan konuşmalar

Bir kişi kapalı, bekleyen ya da ertelenmiş bir konuşmaya yazdığında ya da erteleme dolduğunda konuşma yeniden
açılır. Üye, gelen kutusuna erişimi sürdüğü ve `away` olmadığı sürece konuşmayı tutar; çevrimdışı olmak ya da
sınırı aşmak konuşmayı elinden almaz. Aksi halde atama kalkar ve konuşma havuza gider, kimse uygun değilse
bekler. Modu `off` olan gelen kutuları ve ekipler eski davranışı korur.

## 6. Olaylar

Atama, yazarı `system` olan olağan `assigned` ya da `unassigned` zaman çizelgesi olayı olarak yazılır.
Olayda `auto: true` bulunur; önceki üye varsa `previous_assignee_id` de bulunur. Atanan kişi
`assigned_to_me` bildirimini alır; `conversation.updated` ve [webhook'lar](webhooks.md#olaylar) elle atamadaki
gibi izler.

## 7. Otomasyon kuralları

[Otomasyon kuralları](automation.md) önce çalışır. `assign_pool` eylemi konuşmanın üyesini temizler ve
tetikleyicinin tüm kuralları çalıştıktan sonra konuşmayı otomatik atamaya verir. Bir üye atayan,
atamayı kaldıran (`unassign`) ya da `assign_pool` kullanan kural, yeniden açılan konuşmanın eski üyesini
korumasını da engeller.
