# Raporlar

Raporlar ekibinizin işini sayar: açılan, kapanan ve yeniden açılan konuşmalar, yanıtlar, ne kadar hızlı
yanıtladığınız ve çözdüğünüz, [SLA](sla.md) hedeflerini tutturup tutturmadığınız ve müşterilerin verdiği
puanlar. 13 aya kadar herhangi bir aralığı gün, hafta veya ay olarak isteyin; gelen kutusu, kanal, etiket,
ekip veya üyeye göre kırın ve CSV olarak indirin. Panelin Raporlar bölümündeki Eğilimler sekmesi bunun içindir; Genel bakış, Puanlar ve Ekip sekmeleri bugünü, son 7 günü ve son 30 günü gösterir. Aşağıda anlatılan API ve MCP sunucusu aynı rakamları kendi araçlarınıza verir.

## Eğilimler sekmesi

Raporlar › Eğilimler'i açın. Bir dönem seçin (12 aya kadar hazır aralıklar veya 13 aya kadar özel tarihler), gün, hafta veya ay seçin ve rakamları gelen kutusu, kanal, etiket, ekip veya üyeye göre kırın. Süzgeçler raporu tek bir gelen kutusuna, kanala, etikete, ekibe veya üyeye daraltır. KPI kutuları her rakamı önceki dönemle karşılaştırarak gösterir, grafikler hacmi ve yanıt sürelerini gösterir, sıralanabilir bir tablo grupları listeler. "Verilerin güncel olduğu an" notu sayımların nereye kadar ulaştığını söyler. **CSV olarak dışa aktar** gördüğünüzü indirir.

## 1. Ne sayılır

| Rakam | Neyi sayar |
|---|---|
| `opened` | Dönemde başlayan konuşmalar. Spam hariçtir. |
| `closed` | Her kapatma; birden çok kez kapatılan konuşma da dahildir. |
| `reopened` | Kapalı durumdan başka bir duruma geçiş. |
| `replies` | Üyelerin gönderdiği yanıtlar; bir üyenin gönderdiği bot taslağı da dahildir. |
| `first_replies` | İlk üye yanıtı dönemde olan konuşmalar; medyan ve p90 süre saniye olarak (`first_reply_median_seconds`, `first_reply_p90_seconds`). |
| `resolutions` | Müşteriden gelen bir iletiyi izleyen kapatmalar; medyan ve p90 süre saniye olarak. Saat, önceki kapatmadan sonraki ilk müşteri iletisiyle başlar. |
| `sla` | Tutturulan ve kaçırılan hedefler: `first_reply`, `next_reply` ve `resolution`. |
| `ratings` | `good` ve `bad`; müşterinin puan verdiği güne sayılır. Geri bildirim hiçbir zaman puanlanmaz. |

Botların, [otomasyon kurallarının](automation.md) ve otomatik yanıtların gönderdiği yanıtlar asla sayılmaz;
ne yanıt ne de ilk yanıt olarak. Çöp kutusundaki konuşmalar ve içe aktarılan geçmiş hariçtir. Ölçülecek bir
şey olmayan medyan veya p90 yanıtta yer almaz.

Medyan ve p90 gerçek değerin %2,5'i içindedir. Bugünün rakamları panelin özetiyle uyuşur.

## 2. Saat dilimleri ve dönemler

`from` ve `to`, `timezone` içindeki yerel günlerdir (`Europe/Istanbul` gibi bir IANA adı; varsayılan
`UTC`) ve `to` dahildir. Aralarında en fazla 13 ay olabilir. Bugünden sonraki günler boştur. Hafta
Pazartesi'den Pazar'a sürer; `from` ile kesilen bir hafta veya ay orada başlar. Rapor, boş olanlar dahil
her dönemi listeler.

## 3. Gruplama ve süzgeçler

`group_by`, her rakamı `inbox`, `channel`, `label`, `team` veya `member` olarak kırar. Her grubun kendi
toplamları ve serisi vardır. Ekibi, üyesi veya kanalı olmayan rakamlar `key`'siz bir gruba gider. Bir
konuşma her etiketinin altında sayılır; etiketi yoksa `label` gruplamasında hiç yer almaz.

Bir rakam, işi yapan üyeye aittir: yanıtın yazarı, kapatan veya yeniden açan, ilk yanıtı gönderen,
çözüm için kapatan. Açılan konuşmalar ve puanlar atanan kişiye, SLA sonuçları ise konuşmanın sonuç
anındaki atananına ve ekibine aittir.

`inbox_id`, `channel_kind` (`email`, `chat`, `app` veya `api`), `label_id`, `team_id` ve `member_id`
raporu her birinden birine daraltır. `metrics` raporu `opened`, `closed`, `reopened`, `replies`,
`first_reply`, `resolution`, `sla` ve `ratings` içinden bazılarıyla sınırlar; vermezseniz hepsini alırsınız.

Etiketler ve ekip, bir konuşmanın dönemi en son sayıldığında sahip olduklarıdır.

## 4. Kim neyi görür

Rapor, çağıranın görebildiği gelen kutularını kapsar. Sahipler ve yöneticiler her gelen kutusunu görür,
temsilciler yalnızca kendilerinin veya ekiplerinin erişimi olan kutuları, gelen kutularıyla sınırlı bir
API anahtarı ise yalnızca onları. Çağıranın göremediği bir gelen kutusunu istemek `404` döner.

## 5. Rakamlar ne kadar güncel

Yuva sayıları çeyrek saat başına tutar; böylece yerel bir gün her saat diliminde kesindir. Bugün birkaç
dakikada bir yeniden sayılır, son 31 gün her gece yeniden sayılır ve bu, etiket ve ekip değişikliklerini
yakalar. Son sayımdan yeni olan her şey canlı eklenir, yani bugünün toplamları günceldir. Yeni
yükseltilmiş bir sunucuda geçmiş arka planda sayılır: yanıttaki `built_until` sayımların nereye kadar
ulaştığını söyler ve sonraki dönemler yetişene kadar boştur.

Sayımlar yalnızca sayı ve kimlik tutar: ileti metni, müşteri adı veya adresi yok. Çalışma alanıyla birlikte
silinirler. [Veri saklama](operations.md#veri-saklama) onları silmez: konuşmaları saklama süresiyle silinen
dönemler sayılarını korur.

## 6. API

Bir [API anahtarı](headless.md#yetkiler) veya `reports:read` yetkisi olan bir OAuth bağlantısı kullanın.
Her üye bu yetkiyi taşıyabilir; yetki adı verilmeden yapılan anahtarlarda vardır, eski anahtarlarda yoktur.

```sh
curl -G https://support.example.com/v1/reports \
  -H "Authorization: Bearer $YUVA_API_KEY" \
  -d from=2026-09-01 -d to=2026-09-30 -d timezone=Europe/Istanbul \
  -d granularity=week -d group_by=inbox -d metrics=opened,closed,first_reply
```

Yanıtta `totals`, her dönem için bir girişi olan `series` ve `group_by` ile her biri kendi `totals` ve
`series`'ine sahip `groups`, ayrıca `built_until` bulunur. Gelen kutuları, etiketler, ekipler ve üyeler
kimlik olarak görünür; adlarını kendi uç noktalarından okuyun.

`GET /v1/reports/export` aynı parametreleri alır ve bir CSV dosyası döner: bir başlık satırı, sonra her
dönem için bir satır; `group_by` ile her grup ve dönem için bir satır. Sütunlar `period` (dönemin ilk
günü), `group_by` ile ayrıca `group_id` ve `group_name`, sonra `opened`, `closed`, `reopened`, `replies`,
`first_replies`, `first_reply_median_seconds`, `first_reply_p90_seconds`, `resolutions`,
`resolution_median_seconds`, `resolution_p90_seconds`, `sla_first_reply_met`, `sla_first_reply_breached`,
`sla_next_reply_met`, `sla_next_reply_breached`, `sla_resolution_met`, `sla_resolution_breached`,
`ratings_good` ve `ratings_bad`; seçtiğiniz metriklerle sınırlıdır. Ölçülecek bir şey olmayan süre boş
hücredir. `=`, `+`, `-`, `@`, sekme veya satır başı ile başlayan hücrelerin başına `'` eklenir; böylece
bir hesap tablosu onları çalıştırmaz.

| Parametre | |
|---|---|
| `from`, `to` | Zorunlu. İlk ve son yerel gün. |
| `timezone` | IANA adı; varsayılan `UTC`. |
| `granularity` | `day` (varsayılan), `week` veya `month`. |
| `metrics` | Tekrarlayın veya virgülle ayırın; varsayılan hepsi. |
| `group_by` | `inbox`, `channel`, `label`, `team` veya `member`. |
| `inbox_id`, `channel_kind`, `label_id`, `team_id`, `member_id` | Süzgeçler. |

## 7. MCP

`reports:read` yetkisiyle bir [asistan](mcp.md) `get_report` aracını çağırabilir; araç aynı parametreleri
alır (`from` ve `to` `YYYY-MM-DD` olarak) ve aynı rakamlarla yanıt verir. Örneğin sorun: "Geçen ay gelen
kutusu bazında ne kadar hızlı yanıtladık ve en çok kim kapattı?"
