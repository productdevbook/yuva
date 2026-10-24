# Ekip postası

Ekip postası, Yuva'nın bir ekibin posta kutusu olarak kullanılmasıdır: sales@ ya da billing@ gibi ortak adresler
ve kişilerin kendi posta kutuları. Konuşmalar, etiketler, atananlar ve notlar destekteki gibidir; yalnızca
kelimeleri ve klasörleri kendine aittir. Alan adınızın postası Yuva'ya yönlendirilir, panelde okuyup yazarsınız
ve eski posta kutunuzun geçmişi içeri taşınır.

## Ne olduğu ve ne olmadığı

Bir gelen kutusunun bir amacı vardır: **Destek** (sohbet, e-posta ve uygulama içi müşteriler), **Ekip postası**
(bir `mail` gelen kutusu) ya da **İkisi de**; yani iki türden gelen kutuları olan tek bir çalışma alanı. Yeni
çalışma alanlarına hangisinin uyduğu sorulur. Ekip postası gelen kutusunda yalnızca e-posta kanalları bulunur.
Destek özellikleri onda kapalı kalır: puanlar, SLA politikaları, otomatik atama, karşılama mesajları, durum,
yardım, yol haritası, Yenilikler ve destek raporları.

- Posta kutusu Yuva'dır. Posta, alan adınızın [Yuva'ya yönlendirilmesiyle](email.md#gelen-e-posta) gelir; panelde,
  mobil uygulamada ve [asistanlarla](mcp.md) okunur.
- Yuva, posta uygulamaları için bir IMAP, POP3 ya da SMTP sunucusu değildir. Apple Mail, Thunderbird ya da
  Outlook'a eklenemez ve Gmail ya da Outlook'ta kalan bir posta kutusuyla eşitlenmez: [geçmişi taşımak](#geçmişi-taşımak)
  eski posta kutusunu bir kez okur ve ona hiçbir şey yazmaz.
- Yanıt, e-posta kanalının kendi SMTP hesabıyla gider; yani gönderme için destek e-postasıyla aynı kurulum gerekir.

## Ortak ve kişisel posta kutuları

| | Ortak posta kutusu | Kişisel posta kutusu |
|---|---|---|
| Ne için | Ekibin birlikte yanıtladığı adres: sales@, billing@ | Bir kişinin kendi postası |
| Kim okur | Her gelen kutusunda olduğu gibi, gelen kutusuna erişimi olan üyeler | Sahibi ve sahibinin [yetki verdiği](#yetki-devri) üyeler |
| Kim kurar | Sahipler ve yöneticiler | Her üye kendisi için; sahip ya da yönetici başka bir üye için de kurabilir |
| Filtreler | Sahipler ve yöneticiler | Yalnızca sahibi |

Kişisel posta kutusu sahiplerden ve yöneticilerden bile gizlidir: bkz. [Gizlilik](#gizlilik).

<!-- panel steps: Y-170 -->
Ortak posta kutusu kurmak için sahipler ve yöneticiler **Ayarlar → Ürünler → Gelen kutusu ekle** yolunu açar
(ya da boş **Posta** sayfasında **Gelen kutusu ekle**'yi seçer) ve gelen kutusunun amacı olarak **Ekip postası**'nı
seçer. Kişisel posta kutusu, gelen kutusunun sahibi bir üye olarak ayarlanarak oluşturulur (`POST /v1/inboxes`,
`owner_member_id`).

## Alan adının postasını Yuva'ya yönlendirme

Her posta kutusu adresi, gelen kutusunda bir [e-posta kanalıdır](email.md): **Ayarlar → gelen kutusu → Kanallar →
E-posta**. Sonra:

1. [Adresi kanıtlayın](email.md#adresi-kanıtlama); ona gönderilen bir kodla ya da bir DNS kaydıyla.
2. Postasını [Cloudflare Email Worker ya da herhangi bir MTA](email.md#gelen-e-posta) ile Yuva'ya yönlendirin ya
   da eski posta kutusunu kanalın [yönlendirme adresine](email.md#yönlendirme-adresi) iletin.
3. Yanıtları göndermesi için kanala bir SMTP hesabı verin ve [DNS kayıtlarını](email.md#dns-kontrol-listesi) ayarlayın.

Catch-all kanal (`*@example.com`), alan adının diğer tüm adreslerinin postasını alır.

## Geçmişi taşımak

[Yuva'ya taşı](migrate.md), eski posta kutunuzdaki iletileri (Gmail, Outlook, Yandex, iCloud ve herhangi bir IMAP
sunucusu ya da bir dışa aktarma dosyası) bir gelen kutusuna kopyalar. Yuva eski posta kutusunu yalnızca okur; orada
hiçbir şey değişmez. Sağlayıcınızın kılavuzunu [listeden](migrate.md) seçin. Taşımayı sahipler ve yöneticiler başlatır:
**Ayarlar → Yuva'ya taşı → Yeni taşıma**. Kişisel bir posta kutusuna yapılan taşımayı yalnızca posta kutusunun sahibi oluşturur, görür ve çalıştırır; o da yalnızca
sahip çalışma alanının sahibi ya da yöneticisiyse: taşıma bir yönetici özelliğidir, bu yüzden bir ajan kendi posta kutusuna
taşıma yapamaz. Diğer sahipler, yöneticiler, yetki verilenler ve API anahtarları onu görmez. Posta kutusunu silmek
taşımalarını da siler; biri tarıyor, kopyalıyor, eşitliyor ya da geçmişi kaldırıyorken silme reddedilir.

## Posta bölümü

Kenar çubuğundaki **Posta**, okuyabildiğiniz tüm ekip postası gelen kutularının postasını (**Tüm postalar**) ya da
tek bir posta kutusununkini listeler. Klasörleri:

| Klasör | İçindekiler |
|---|---|
| Gelen kutusu | Açık konuşmalar |
| Yıldız eklendi | Yıldızladığınız konuşmalar; yıldız yalnızca sizindir |
| Ertelendi | Zamanı gelene kadar gizlenen konuşmalar |
| Gönderildi | Posta gönderdiğiniz konuşmalar |
| Taslaklar | Gönderilmemiş kendi postalarınız ve yanıtlarınız |
| Zamanlandı | Gönderim zamanını bekleyen postalarınız |
| Arşiv | Arşivlediğiniz (kapalı) konuşmalar |
| Çöp kutusu | Silinen konuşmalar, 30 gün sonra boşaltılır; sahiplere ve yöneticilere listelenir |

Bir postayı açıp yanıtlayabilir, yıldızlayabilir, arşivleyebilir, erteleyebilir ya da çöp kutusuna taşıyabilirsiniz.
Posta başlığındaki **Filtreler**, posta kutusunun [filtrelerini](#filtreler) açar.

## Yeni posta yazma

**Yeni posta**'yı seçin. **Kimden** altında postanın hangi posta kutusundan çıkacağını seçin, sonra şunları doldurun:

- **Kime**: en çok 50 adres. İlki konuşmanın kişisi olur.
- **Cc** ve **Bcc**: her biri en çok 50. Bcc alıcıları postayı alır ama başka herkesten gizlenir.
- **Konu** ve ileti, ekleriyle birlikte.

Posta yazıldığı gibi, `Re:` olmadan gider. Kime ve Cc'deki diğer adresler gönderildiğinde katılımcı olur; böylece
yanıtları konuşmaya eklenir. Başlattığınız konuşma **Gönderildi** altında bekler ve ilk yanıt onu Gelen kutusuna
geri açar. Dakikada 60 yeni posta yazabilir, saatte 300 alıcıya ulaşabilirsiniz. API çağrısı
`POST /v1/conversations/mail`'dir.

### İmzalar

Her üyenin gelen kutusu başına bir imzası vardır; gelen kutusunun da sahiplerin ve yöneticilerin belirlediği bir
varsayılanı. **Ayarlar → Sen → İmza**'yı açın, gelen kutusunu seçin ve kendi imzanızı yazın, varsayılanı kullanın
ya da hiç kullanmayın. Varsayılanda `{{member.name}}` ve `{{member.title}}` adınıza ve unvanınıza dönüşür. İmza,
e-postayla giden yanıtlarınızı ve yeni postalarınızı bitirir; **Çıkar** onu tek bir iletiden kaldırır. Notlar ve
sohbet yanıtlarında imza olmaz.

## Kendiniz olarak gönderme

Bir ekip postası gelen kutusundan gönderdiğiniz posta, kanalın adresinden sizin adınızla ve kanalın adıyla gider:
`Ada Lovelace (Sales) <sales@example.com>`. Yanıtlar kanala döner.

Bunun yerine kendi adresinizden (`ada@example.com`) göndermek için:

1. **Ayarlar → Sen → Adreslerin**'i açıp **Adres ekle**'yi seçin. 10 adres tutabilirsiniz.
2. Yuva adrese 10 dakika geçerli 6 haneli bir kod gönderir; girin. Çalışma alanının bir kanalı o alan adının
   tüm postasını yakalıyorsa kod gönderilmez: adres, **Ayarlar → Ekip → Üyeler** altındaki üye kaydınızda
   **Gönderilecek adresler** bölümünde bir sahibin ya da yöneticinin onayını bekler.
3. Yeni postada ya da yanıtta, **Kimden** yanındaki adres seçicide onu seçin.

Bir adres bir kanalla yalnızca alan adı kanalın gönderim alan adıysa ve kanalın o alan adı için son
[teslim edilebilirlik denetimi](deliverability.md) hizalı DKIM ve DMARC ile geçtiyse çalışır. Aksi halde Yuva
göndermeyi nedenini söyleyerek reddeder. Bir kanalın kullandığı ya da başka bir üyenin doğruladığı adres kabul
edilmez.

## Taslaklar, Sonra gönder ve geri alma

- **Taslaklar** siz yazdıkça kaydedilir. Pencereyi kapatırsanız posta **Taslaklar** altında bekler; yalnızca siz
  görürsünüz. Alıcıları, yazdığınız her adres gibi, taslak kaydedilince kişi olur.
- **Sonra gönder** (Gönder'in yanında) postayı bir dakika ile 366 gün arasında bir zamana planlar. Posta
  **Zamanlandı** altında bekler; oradan zamanını değiştirebilirsiniz (**Zamanlamayı iptal et** onu taslağa
  döndürür). Zamanı geldiğinde gönderilemezse (adres kaldırıldı, erişiminizi kaybettiniz) nedeniyle birlikte
  taslak olur ve size bildirilir.
- **Geri al**: bir konuşmadaki yanıt gitmeden önce 6 saniye bekler; bir postayı zamanladıktan sonraki bildirimde
  zamanlamayı iptal eden bir **Geri al** vardır. Yeni posta hemen gönderilir.

## Filtreler

Filtre, bir posta kutusuna gelen her postaya bakar ve ona göre işlem yapar. **Posta → Filtreler**'i açıp ad,
koşullar ve işlemlerle bir tane ekleyin. Filtreler listedeki sırayla çalışır.

| Koşullar | İşlemler |
|---|---|
| Kimden, Kime, Cc, posta listesi (`List-Id`), konu, eki var | Etiket ekle, yıldızla, arşivle, çöp kutusuna taşı, ertele, bir üyeye ilet, durumu ya da önceliği değiştir, ata |

Metin değerleri 3 ile 200 karakter arasındadır. Bir üyeyi yıldızlayan, atayan ya da ona ileten filtre yalnızca
posta kutusunu okuyabilen üyeler için çalışır. Taslaklar, gönderilen posta, içeri aktarılan geçmiş ve teslim
raporları filtre çalıştırmaz. Bir posta kutusu en çok 100 filtre tutar.

Bir postadan yola çıkmak için onu açıp **Buna benzeyen iletileri filtrele**'yi seçin: Yuva göndereni ve posta
listesini, yoksa konuyu doldurur; işlemleri siz seçersiniz.

Ortak posta kutusunun filtrelerini sahipler ve yöneticiler yönetir. Kişisel posta kutusunun filtreleri sahibinindir:
başkası görmez ya da düzenlemez ve yalnızca o posta kutusunda çalışırlar.

## Arama

Posta aramasına düz kelimeler ve işleçler yazılır. `anahtar:değer`, `anahtar:"iki kelime"` ya da eşleşeni dışarıda
bırakmak için `-anahtar:değer` yazın. Her işleç sağlanmalıdır; başka bir anahtarlı terim metin olarak aranır.

| İşleç | Eşleşen | Örnek |
|---|---|---|
| `from:` | gönderen adresi ya da adı değeri içerir; `from:me` sizsiniz | `from:ada@example.com` |
| `to:`, `cc:` | bir alıcı adresi değeri içerir; `to:me` sizsiniz | `to:me cc:@example.com` |
| `subject:` | konu | `subject:"quarterly invoice"` |
| `has:attachment` | eki olan bir ileti | `has:attachment invoice` |
| `is:starred` | yıldızladığınız konuşmalar | `-is:starred` |
| `in:` | bir klasör: `inbox`, `starred`, `snoozed`, `sent`, `archive`, `trash`, `drafts`, `scheduled` | `in:archive from:ada` |
| `after:`, `before:` | bir `YYYY-MM-DD` gününde ya da sonrasında, ya da öncesinde gönderilmiş ileti | `after:2026-01-01 before:2026-02-01` |

Uzunluk sınırları gibi ayrıntılar [E-postada arama](email.md#e-postada-arama) bölümündedir.

## Gizlilik

<!-- panel steps: Y-170 -->
Kişisel posta kutusunu yalnızca sahibi ve sahibinin yetki verdiği üyeler okur.

- Sahipler ve yöneticiler kişisel bir posta kutusunun var olduğunu görür: adını, adreslerini, konuşma sayısını ve
  boyutunu. Postasını asla. Adını değiştirebilir ya da silebilirler.
- API anahtarları ve webhook'lar kişisel posta kutusuna asla ulaşamaz; çalışma alanı otomasyon kuralları, Slack ve
  gelen kutularının ekip ve üye yetkileri de ulaşamaz. Bir asistan ona yalnızca sahibinin kendi OAuth bağlantısıyla
  ulaşır. Olayları başkasına gönderilmez.
- Kişisel posta kutusunun posta listeleri, toplu gönderimleri, e-posta tasarımları ve analitik özellikleri yalnızca
  sahibinindir: yetki verilenler, sahipler ve yöneticiler onları görmez ya da değiştiremez; Ana sayfada da.
- Kişiler çalışma alanı genelindedir: adres başına tek kişi kaydı vardır. Bir kişisel posta kutusunun bildiği bir
  adrese yazan ortak gelen kutusu aynı kişiyi kullanır; böyle bir adresi elle eklemek adresin alınmış olduğu
  yanıtını verir. Kişisel posta kutusunda yalnızca posta gizlidir, kişi kaydı değil. Bir kişi yalnızca, tüm konuşmaları okuyamadığınız kişisel
  posta kutularındaysa ve harici kimliği yoksa gizlidir.
- Saklama süresi, kişi silme ve çalışma alanı silme kişisel posta kutusunu da kapsar; her yetki verme ve devir
  [denetim günlüğündedir](operations.md#denetim-günlüğü).

## Yetki devri

<!-- panel steps: Y-170 -->
Kişisel posta kutusunun sahibi diğer üyeleri tek tek, iki haktan biriyle içeri alır. Bunu yalnızca sahibi yapabilir;
sahipler, yöneticiler ve yetki verilenler yapamaz.

| Hak | Yapabilir | Yapamaz |
|---|---|---|
| Okuma | Sahibinin okuduğu her şeyi okur: konuşmalar, notlar, ekler, arama, sayaçlar, bildirimler | Yanıtlayamaz, not yazamaz; durum, yıldız, etiket, erteleme ya da atananı değiştiremez |
| Gönderme | Okumanın tümü, ayrıca yanıtlama, yeni posta yazma ve iletme, not yazma ve durum, yıldız, erteleme, atanan ve etiketleri değiştirme | Postayı posta kutusundan taşıyamaz (iki hak için de yalnızca sahibi); ayarlarını, kanallarını, filtrelerini ya da yetkililerini değiştiremez, taşıma çalıştıramaz, listelerini ve toplu gönderimlerini yönetemez |

Hiçbir hak sahibinin kendi taslaklarını göstermez. Yetki verilenin gönderdiği posta sahibi "adına" gider: `From`
sahibinin adı ve adresidir, yetki verilen `Sender` başlığında adlandırılır. İleti iki kişiyi de gösterir. Yetkiyi
kaldırmak erişimi hemen alır. API: `GET|PUT|DELETE /v1/inboxes/{id}/delegates[/{memberId}]`.

## Bir üye ayrıldığında

<!-- panel steps: Y-170 -->
Kişisel posta kutusu, sahibi gidince ortak olmaz.

- **Bir sahip ya da yönetici üyeyi kaldırır.** Üyenin sahibi olduğu her kişisel posta kutusu için kaldırma sizden
  ya postayla birlikte **silmenizi** ya da adı verilen bir üyeye **devretmenizi** ister; o üye yeni sahip olur.
  Devredilen posta kutusunun yetkilileri erişimini kaybeder. Üyenin her kişisel posta kutusu için bir seçim yapılana kadar kaldırma reddedilir; devredilen üye etkin olmalı ve ayrılan üye olmamalıdır. Siz seçene kadar kimse posta kutusunu okuyamaz.
- **Bir üye kendi hesabını siler.** Kişisel posta kutuları silinmez. Her biri sahibini ve yetkililerini kaybeder ve
  sahipsiz kalır: kimse okumaz. Sahipler ve yöneticiler onu yine adı ve boyutuyla görür ve karar verene kadar bir
  üyeye devredebilir (`POST /v1/inboxes/{id}/hand-over`) ya da silebilir. Devir yalnızca sahipsiz bir posta kutusunda çalışır ve panelde oturum açmış bir sahip ya da yönetici gerektirir, API anahtarı yetmez.

Her iki seçim de denetim günlüğündedir.
