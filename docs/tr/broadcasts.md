# Bülten gönderme

Bülten, [e-posta listelerinizin](lists.md) abonelerine giden tek bir e-postadır: haber bülteni, ürün
güncellemesi, duyuru. Bir [e-posta tasarımı](designs.md) kullanır, gelen kutunuzun e-posta
kanallarından biri üzerinden gider ve ona verilen yanıt o gelen kutusunda bir konuşma olur. Yuva sonra
ne olduğunu gösterir: kime ulaştı, kim geri döndü, kim abonelikten çıktı ya da yanıtladı.

Akış şöyledir: liste, sonra tasarım, sonra bülten. Bültenleri sahipler ve yöneticiler gönderir;
temsilciler, bir yanıtı anlayabilmek için gelen kutularındaki bültenleri okur.

## 1. İlk bültenden önce

1. Bir [liste](lists.md#2-liste-oluşturun) oluşturun ve aboneleri bir formla, arka uçtan ya da
   [CSV dosyası içe aktararak](importing.md) ekleyin. Bülten yalnızca bir listeye `subscribed` olan kişilere gider.
2. Gelen kutusunun [gönderen bilgilerini](lists.md#1-gönderen-bilgilerini-girin) doldurun. Her bülten
   bunlarla biter; yasal unvan ve posta adresi yoksa gözden geçirme göndermeyi engeller.
3. Bir [tasarım](designs.md) yapın ya da bir şablonla başlayın.
4. Gelen kutusunda SMTP hesabı ve gönderilecek adresi olan bir [e-posta kanalı](email.md) bulunduğundan
   emin olun. Catch-all kanalın bir From adresi olmalıdır. O adresin alan adı için SPF, DKIM ve DMARC
   kurun ve [teslim edilebilirlik denetimini](deliverability.md) çalıştırın.
5. İsteğe bağlı ama akıllıca: kanala bültenler için ayrı bir hesap verin ([aşağıda](#gönderen)).

Başkasının işlettiği bir sunucuda yeni bir çalışma alanının göndermeden önce
[onaylanması](#barındırılan-sunucularda-onay) da gerekebilir.

## 2. Bülteni yazın

Panelde: Bültenler, sonra Yeni bülten. Yalnızca ekibinizin göreceği bir ad verin ve gelen kutusunu
seçin. Siz gönderene ya da zamanlayana kadar taslak olarak kalır ve her değişiklik yaptıkça kaydedilir.

### Gönderen

Bülteni gönderecek e-posta kanalını seçin. Oluşturucu, kanalın hazır olup olmadığını, hangi hesabı
kullanacağını ve ne hızla göndereceğini söyler.

Bülten e-postaları sunucunun kendi posta göndericisinden hiçbir zaman geçmez. Kanalın ayrı bülten
hesabı varsa o, yoksa destek hesabı kullanılır. Ayrı hesap, pazarlama e-postalarına gelen şikâyetleri
destek yanıtlarınızı taşıyan hesaptan uzak tutar; yavaş bir gönderim de bir destek yanıtını geciktirmez.

Ayarlamak için: Ayarlar → gelen kutusu → Müşteriler nereden yazıyor → E-posta, sonra Bülten gönderimi
bölümü. "Bültenler için ayrı bir hesap kullan" seçeneğini açın; sunucu, port, şifreleme, kullanıcı adı ve
parolayı girin. Seçenek kapalıyken kaydetmek hesabı kaldırır. Hız, kanalın saniyede gönderdiği e-posta
sayısıdır: 1 ile 100 arası (yeni kanalda 5); sağlayıcınızın izin verdiğinin altında tutun.

### Alıcılar

Gelen kutusunun en fazla 10 listesini seçin. Birden çok listede olan kişi tek e-posta alır; bu e-posta,
sizin sıraladığınız listelerden ilkinin altında gider.

İsterseniz daraltın:

- **Koşullar** (en fazla 10; hepsi uymalı) bir kişi özniteliğini karşılaştırır: eşittir, eşit değildir,
  içerir (büyük/küçük harf fark etmez), şundan büyüktür, şundan küçüktür, şunlardan biridir, vardır,
  yoktur. Özniteliği olmayan kişi yalnızca "eşit değildir" ve "yoktur" koşullarına uyar.
- **Diller** (en fazla 20) yalnızca dili bunlardan biri olan kişileri tutar; `tr` aynı zamanda `tr-TR`
  kişilerini de tutar. Boş bırakırsanız herkes kalır.

Oluşturucu, alıcılar değiştikçe alıcı sayısını ve "Dışarıda kalanlar" altında diğerlerinin neden e-posta
almayacağını gösterir:

| Neden | Kim |
|---|---|
| Ayrıldı | Listelerin hepsinden ayrılmış. |
| Henüz onaylamadı | Listelerde yalnızca `pending` (çift onay tamamlanmamış). |
| Geri döndü ya da spam bildirildi | Adres engelli listesinde. |
| Tüm e-postalara itiraz etti | Kişi çalışma alanınızdan gelen e-postalara itiraz etti. |
| Engellenmiş kişiler | Kişiyi siz engellediniz. |
| Bir koşul ya da dil nedeniyle dışarıda | Bir koşul ya da dil uymuyor. |
| Birden çok listede, her birine tek e-posta | Fazladan abonelikler. |

### İçerik

Tasarımı seçin. Konuyu yazın (en fazla 300 karakter; gelen kutuları yaklaşık ilk 80 karakteri gösterir)
ve isterseniz çoğu gelen kutusunda konudan sonra görünen önizleme metnini yazın. Boş bırakırsanız
tasarımın kendi metni kullanılır. `{{ contact.first_name | "there" }}` gibi birleştirme alanları konuda
da çalışır; yedek değeri olmayan bir alan birinde boş kalacaksa gözden geçirme sizi durdurur.

### Seçenekler

"Tıklamaları say" kapalıdır. Bkz. [Tıklama takibi](#tıklama-takibi). Bağlantılar ayrıca [Analytics](#kampanya-takibi-utm) için
kampanya parametreleri taşır; siz kapatmadıkça açıktır.

### Gönderim hızı

Bir bültenin kendi hızı olabilir: saatte, günde ve ayda (30 gün) en fazla şu kadar e-posta; üçü de isteğe
bağlıdır. Hazırlama ekranındaki "Hızı sınırla" bunu açar. Yuva e-postaları üçünün en sıkısına göre eşit
aralıklarla dağıtır; yani günde 100, gece yarısı 100 e-posta değil, yaklaşık her 14 dakikada bir e-posta
demektir ve hiçbir 60 dakika, 24 saat ya da 30 gün sizin verdiğinizden fazlasını almaz. Kanalın hızı ve
çalışma alanının [günlük sınırı](#günlük-sınır) yine geçerlidir; bültenin hızı onu yalnızca yavaşlatabilir.
Her deneme sayılır, yeniden deneme de; bu arada abonelikten çıktığı için atlananlar sayılmaz. Bir bülten
kendi hızıyla en fazla 365 gün sürebilir; kontrol listesi bundan önce sizi durdurur. Zamanlanmış,
gönderilen ya da duraklatılmış bir bültenin hızını sayfasından değiştirebilirsiniz; bir sonraki e-postaya
uygulanır. Duraklatma e-posta biriktirmez: devam ettirdikten sonra bir sonraki e-posta sırasını bekler.

### Deneme gönderimleri

Her gönderimden önce kendinize ya da en fazla beş üyeye deneme gönderin. Gerçek e-postayla aynı görünür;
konunun başında `[Test]` yazar. Denemeler yalnızca çalışma alanının üyelerine gider, başka hiçbir adrese
gitmez, abonelikten çıkma bağlantısı da bunun bir deneme olduğunu söyler. Birleştirme alanlarını örnek
değerlerle ya da bir kişinin değerleriyle doldurabilirsiniz (deneme yine yalnızca seçtiğiniz üyelere
gider). Çalışma alanı başına saatte en fazla 20 deneme.

## 3. Gözden geçirin ve gönderin

"Gözden geçir ve gönder", zamanladığınızda sunucunun çalıştırdığı kontrol listesini gösterir. Engelleyen
maddeler göndermeyi durdurur; uyarılar durdurmaz.

| Madde | Engeller, eğer |
|---|---|
| Alt bilgi için gönderen bilgileri | Gelen kutusunda yasal unvan ya da posta adresi yok. |
| Gönderen kanal | Kanal seçilmemiş ya da kanalda SMTP hesabı veya From adresi yok. |
| Gönderim izni | Sunucuda bültenler kapalı, çalışma alanı [onay](#barındırılan-sunucularda-onay) bekliyor ya da askıda. |
| Tasarım | Seçilmemiş ya da 500 KB'tan fazla HTML'ye dönüşüyor. |
| Alıcılar | Liste seçilmemiş ya da hiç kimse e-posta almayacak. |
| Birleştirme alanları | Yedek değeri olmayan bir alan bir alıcıda boş kalacak. |
| E-posta boyutu | 102 KB'tan fazla HTML; Gmail e-postayı keser (alt bilgi ve abonelikten çıkma bağlantısıyla birlikte). 90 KB üstü uyarıdır. |
| Görsel açıklamaları | Uyarı: bir görselde alternatif metin yok. |
| Bağlantılar | Uyarı: bir bağlantı `http:` kullanıyor. |
| Konu | Boş ya da 300 karakterden uzun. 80 üstü uyarıdır. |
| Gönderim hızı | Bültenin hızı gönderimi 365 günden uzun sürdürür. |
| Günlük sınır | Uyarı: [günlük sınır](#günlük-sınır) gönderimi bir günden uzun sürdürür. |
| Deneme e-postası | Uyarı: son değişiklikten beri deneme gitmedi. |
| Tıklama sayımı | Açıksa uyarı: bağlantılar sunucunun bağlantı alan adına işaret eder. |
| Amazon SES tenant | Gelen kutusunun SES tenant'ı duraklatılmış ya da yok, kurulamamış ya da kanalın kimliği eksikse engeller. Yüksek önemde bir itibar bulgusunda ya da sunucu tenant'ı hâlâ kurarken (gönderim saniyelerce bekler) uyarı verir. |

Sonra Şimdi gönder'i seçin ya da Zamanla'yı seçip saat diliminizde en fazla 90 gün sonrası için bir
zaman belirleyin. Panel alıcı sayısını onaylamanızı ister. Siz bakarken alıcılar değiştiyse yeniden
gözden geçirmenizi ister.

Zamanladığınızda olanlar:

- Tasarım o anki sürümünde dondurulur. Tasarımda sonradan yapılan düzenlemeler bu bülteni değiştirmez.
- Gönderim başlarken alıcılar bir kez belirlenir. Sonradan abone olanlar dahil edilmez. E-postası
  gitmeden abonelikten çıkan, geri dönen ya da şikâyet eden herkes atlanır; her adres, e-postasından
  hemen önce yeniden denetlenir.
- Başlayana kadar, "Taslağa dön" zamanlanmış bir bülteni değişiklik için geri alır.

## 4. Gönderilirken

Bülten sayfası ilerlemeyi canlı izler: belirlenen alıcılar, gönderilenler, başarısızlar, geri dönenler ve
yaklaşık ne zaman biteceği. Hız varsa sayfa bir sonraki e-postanın ne zaman çıkacağını gösterir; Hızı değiştir onu ayarlar.

| Durum | Anlamı |
|---|---|
| Taslak | Yazılıyor. |
| Zamanlandı | Vakti bekliyor. |
| Hazırlanıyor | Alıcılar belirleniyor. |
| Gönderiliyor | E-postalar çıkıyor. |
| Duraklatıldı | Biri sürdürene kadar durdu. |
| Gönderildi | Bitti. |
| İptal edildi | Kalıcı olarak durdu. |
| Başarısız | Devam edemiyor, örneğin kanalı kaldırıldı. |

**Duraklat**, sürmekte olan toplu işlemin içinde (en fazla 50 e-posta) gönderimi durdurur. **Sürdür**
kaldığı yerden devam eder. **İptal**, bülteni kalıcı olarak bitirir; henüz gitmemiş e-postalar atlanır.
Bir taslak ya da bitmiş bülten alıcı listesiyle birlikte silinebilir; yanıtların açtığı konuşmalar kalır.

Geçici SMTP hataları 5 dakika, 30 dakika, 2 saat ve 6 saat sonra yeniden denenir, sonra e-posta başarısız
sayılır. Adresin kendisini reddeden bir SMTP yanıtı (5.1.x ve 5.2.1) geri dönme sayılır.

### Günlük sınır

Bir çalışma alanının, tüm bültenleri toplamında günde (UTC) en fazla kaç alıcıya gönderebileceği
sınırlanabilir. Sınır sunucunun [`YUVA_BROADCAST_DAILY_RECIPIENTS`](configuration.md#bültenler) ayarından,
operatörün çalışma alanı için koyduğu sınırdan ve barındırılan sunucuda ısınma sınırından
([aşağıda](#barındırılan-sunucularda-onay)) gelir; en küçüğü geçerlidir. Gönderim sınıra ulaşınca bülten
bekler ve panel, 00:00 UTC'de ne zaman devam edeceğini gösterir. Günde 100 sınırında 250 alıcılı bir
bülten üç gün sürer. Gözden geçirme başlamadan önce uyarır; Ayarlar → Çalışma alanı → Bültenler,
bugünkü sayıyı sınırla birlikte gösterir.

### Yuva'nın yaptığı duraklatmalar

Bir bülten kendiliğinden duraklar ve nedeni sayfada görünür, eğer:

| Neden | Tetikleyici |
|---|---|
| Geri dönme oranı | Başladığından ya da son sürdürülmesinden beri gönderilen e-postaların %5 veya fazlası geri döndü (en az 200 e-postadan sonra). |
| Şikâyet oranı | %0,3 veya fazlası spam bildirildi (en az 1.000 e-postadan sonra). |
| Gönderici hatası | SMTP sunucusu hesabı ya da TLS'i reddediyor, art arda 20 e-postayı reddediyor ya da 30 dakika boyunca erişilemiyor. Sunucunun yanıtı gösterilir. |
| Çalışma alanı askıya alındı | Aşağıya bakın. |
| Amazon SES tenant | Amazon SES gelen kutusunun [tenant](email.md#amazon-ses-tenantları)'ını duraklattı ya da kanal tenant yalıtımını kaybetti. Sayfa hangisi olduğunu söyler. Diğer gelen kutuları göndermeye devam eder. |
| Üye, operatör | Biri duraklattı. |

Nedeni giderin, sonra sürdürün. Sürdürdükten sonra oranlar o andan itibaren sayılır.

Tenant duraklatılmışken sürdürme `409 ses_tenant_blocked` ile reddedilir; sunucunun operatörü yeniden etkinleştirir.

Bir çalışma alanı, son 30 günde en az 1.000 bülten e-postasının en az %0,3'ü spam bildirildiğinde ya da
en az 2.000 e-postanın en az %8'i geri döndüğünde gönderimden **askıya alınır**. Çalışan bültenleri
duraklar; zamanlama ve sürdürme reddedilir. Askıyı yalnızca sunucunun operatörü kaldırır: sahip nedeni
Ayarlar → Çalışma alanı → Bültenler altında görür.

### Geri dönenler ve şikâyetler

Yuva geri dönen e-postaları Amazon SES'ten (SNS ile, bkz. [Geri dönen e-postalar](email.md#geri-dönen-e-postalar)),
kanala gelen teslimat raporlarından, gönderim sırasındaki SMTP reddlerinden ve kanala gelen kötüye
kullanım raporlarından (ARF) okur; ARF raporları yalnızca DMARC sonucunu alıcı sunucunun doğruladığı bir
göndericiden gelirse sayılır.

- Kalıcı bir geri dönme, adresi çalışma alanının tüm e-postaları için engeller.
- Bir bülten hakkındaki spam şikâyeti, o adrese bülten gitmesini durdurur, adresi çalışma alanının tüm
  listelerinden çıkarır ve itirazı hatırlar. Destek yanıtları yine de ulaşabilir.

## 5. Yanıtlar

Bir bültene verilen yanıt, o kişiyle bültenin gelen kutusunda "{bülten} bültenine yanıt" diye işaretli,
bültene bağlı bir konuşma açar. Sonraki yanıtlar konuşmaya eklenir. Gelen kutusunda tür süzgecinde
"Bülten yanıtları" bunları gösterir; bültenin raporu ilk 50'sini listeler.

Otomatik yanıtlar (ofis dışında) konuşma olarak saklanmaz: yalnızca `auto_replied` olarak sayılır; böylece
bir gönderim gelen kutusunu doldurmaz. Alıcıdan başkasından gelen e-posta, örneğin bir yönlendirme, kendi
konuşmasını açar.

Üyeler, bir bültenin yeni yanıtları için 10 dakikada bir gruplanmış tek bir anlık bildirim alır; bunlar
için e-posta gitmez.

## 6. Raporlar

Bülten sayfası, gönderim başlar başlamaz raporunu gösterir.

| Sayı | Anlamı |
|---|---|
| Alıcılar | Gönderim başlarken belirlenen alıcılar. |
| Gönderildi | SMTP sunucusunun kabul ettiği e-postalar. |
| Başarısız | Kötü bir adres belirtilmeden kalıcı olarak reddedilenler. |
| Geri döndü | E-postası geri dönen alıcılar. |
| Spam bildirildi | Şikâyet eden alıcılar. |
| Ayrıldı | Bu bültenin bağlantılarıyla bir listeden ayrılan alıcılar. |
| Yanıtladı | Yanıt veren alıcılar; her biri bir konuşmadır. |
| Otomatik yanıtlar | Ofis dışı yanıtlar, yalnızca sayılır. |
| Tıkladı | Bir bağlantı açan alıcılar; yalnızca tıklama takibiyle. |
| Atlandı | Engelli, itiraz etmiş, e-postasından önce abonelikten çıkmış olanlar ya da bülten iptal edildi. |

Oranlar, gönderilen e-postaların payıdır. Zaman çizelgesi, gönderim başladıktan sonraki ilk 72 saat için
saatlik, sonra günlüktür. Alıcı listesi her kişinin durumunu ve başarısızlıklar için SMTP yanıtını
gösterir; duruma göre süzebilir, adres ya da adla arayabilir ya da yalnızca yanıtlayanları
gösterebilirsiniz. Alıcı listesi, bülten bittikten 400 gün sonra silinir; sayılar kalır.

### Tıklama takibi

Varsayılan olarak kapalıdır. Açılırsa e-postadaki bağlantılar sunucunun bağlantı alan adında `/c/…`
üzerinden geçer ve rapor bağlantı başına tekil tıklamaları gösterir. Yalnızca her alıcı için aynı olan
(birleştirme alanı içermeyen) web bağlantıları izlenir; bunlar bülten zamanlanırken bulunur. Tıklamalar
yaklaşık değerdir: bir `HEAD` isteği ve güvenlik tarayıcılarının yaptığı gibi bir e-postanın üç
bağlantısının iki saniye içinde açılması sayılmaz.

### Kampanya takibi (UTM)

Varsayılan olarak açıktır. E-postadaki web bağlantıları `utm_source=newsletter`, `utm_medium=email` ve
`utm_campaign=<kampanya>` alır; böylece [Yuva Analytics](analytics.md), e-postanın web sitenize getirdiği ziyaretleri
E-posta kanalı altında sayar. Kampanya bültenin adından yapılır; taslakta değiştirin (küçük harf, rakam, `-`, `_` ve
`.`; 1–100 karakter). Kendi `utm_` parametresi olan bağlantılar, `http` ya da `https` olmayan bağlantılar ve Yuva'nın
kendi bağlantıları (abonelikten çıkma, tercihler, tıklamalar) olduğu gibi kalır. Seçeneklerden ya da taslakta
`utm: false` ile kapatın. Bu özellik gelmeden önce taslaktan çıkmış bültenler bağlantılarını gönderildiği gibi korur.

Rapor bundan sonra **bu kampanyadan gelen ziyaretleri** gösterir: görmeye yetkili olduğunuz web sitesi başına,
`utm_campaign` değeri bültenin kampanyası olan ziyaretçiler, ziyaretler ve sayfa görüntülemeleri; gönderimin başladığı
günden itibaren (`analytics:read` ile; API'de `campaign_visits`). Ayrıntılar
[Destek ve trafik](analytics-support.md#bülten-kampanyalarını-izleme) sayfasında.

Açılma takibi ve izleme pikseli yoktur. E-posta uygulamaları görselleri önceden yüklediği için açılma
oranları güvenilir değildir ve bir piksel, e-postada üçüncü bir tarafa haber veren tek şey olurdu.

## 7. Anahtarlar ve asistanlar

Göndermek, API anahtarlarının ve OAuth belirteçlerinin varsayılan olarak alamadığı tek şeydir:

- `broadcasts:send` kapsamı hiçbir zaman örtük verilmez; anahtarı oluştururken adıyla yazın.
- O zaman bile bir anahtar ya da belirteç, bir sahip veya yönetici Ayarlar → Çalışma alanı → Bültenler
  altında "Anahtarlar ve asistanlar bülten gönderebilir" seçeneğini açtığı sürece bülten zamanlayabilir ve
  sürdürebilir. Varsayılan olarak kapalıdır ve değiştirmek bir üye oturumu gerektirir. Aksi halde
  zamanlama `403 broadcasts_by_keys_disabled` yanıtı verir.
- Taslak oluşturmak, değiştirmek ve denemek `broadcasts:write`, okumak `broadcasts:read` ister.
  Duraklatmak ve iptal etmek `broadcasts:send` ister ve bu anahtar tarafından engellenmez.
- Bir [asistan](mcp.md#listeleri-tasarımları-ve-kurulumu-bir-asistanla-yapın) üzerinden `pause_broadcast`, `resume_broadcast`, `unschedule_broadcast` ve `cancel_broadcast`, `schedule_broadcast` gibi yalnızca anahtar açıkken listelenir.

## 8. Tetiklenen e-postalar

Tetiklenen e-posta, bir gelen kutusundaki her kişiye bir [tasarımı](designs.md) yalnızca bir kez gönderir; kişi orada
ilk kez tanındıktan belirli bir süre sonra. Tanınma anı, bir uygulamanın ya da arka ucunuzun kişiye ilk harici
kimliğini verdiği andır ([kimlik token'ı](identity.md), `POST /v1/contacts` ya da kimlik taşıyan bir form ile).
Kayıttan bir gün sonra giden hoş geldin e-postası ya da bir hafta sonra gelen "nasıl gidiyor" notu için kullanın.
Bu, `kind` değeri `triggered` olan bir bültendir; tasarım, dile göre konular, birleştirme alanları, gözden geçirme,
deneme gönderimleri, tıklama takibi, hız, gönderim saatleri ve günlük sınır yukarıda anlatıldığı gibi çalışır.
Hiçbir şey bir listeye bir anda gitmez: kişiler tanındıkça tek tek kaydedilir.

### Kurulum

Panelde gelen kutusunda tek seferlik bülten yerine tetiklenen e-posta oluşturun. API ile `kind: "triggered"` ve
bir `trigger` verin:

```sh
curl -X POST https://support.example.com/v1/broadcasts \
  -H "Authorization: Bearer $YUVA_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "inbox_id": "INBOX-ID",
    "channel_id": "CHANNEL-ID",
    "name": "Welcome",
    "subject": "Welcome, {{ contact.first_name | \"there\" }}",
    "design_id": "DESIGN-ID",
    "kind": "triggered",
    "trigger": { "event": "contact_identified", "delay_minutes": 1440, "basis": "service" }
  }'
```

- **Olay.** `contact_identified` (yukarıda), `subscribed` ya da `attribute_changed`; bkz. [Olaylar](#olaylar).
- **Gecikme.** Tetikten 0 dakika ile 365 gün sonrası. E-posta o kadar bekler, sonra bültenlerin hızı, gönderim
  saatleri ve günlük sınırı altında gider; o an saatler kapalıysa açılmalarını bekler. Tetiklenen e-postalar,
  aynı kanalın tek seferlik bültenlerinden önce gider.
- **Gönderim saatleri.** `timezone` ve `send_window` [bültenlerdeki](#gönderim-hızı) gibi çalışır ve e-posta
  çalışırken değiştirilebilir.
- **Alıcılar.** Koşullar ve diller (en fazla 10 ve 20), kişinin tanındığı anda değil, gönderim zamanı geldiğinde
  denetlenir. Artık uymayan kişi atlanır.

### Olaylar

| Olay | Gerekir | Şu durumda tetiklenir |
|---|---|---|
| `contact_identified` | Hiçbir şey. | Kişi gelen kutusunda ilk harici kimliğini alır. |
| `subscribed` | `trigger.list_id`: gelen kutusunun bir listesi (durum sayfasınınki değil). | O listeye bir abonelik onaylanır. Tetikleme zamanı onaylandığı andır; çift onayda kişi onayladığında sayılır, beklerken sayılmaz. İçe aktarmayla oluşan abonelikler bunu asla tetiklemez. |
| `attribute_changed` | `trigger.condition`: bir [bülten koşulu](#2-bülteni-yazın). | Bir yazma, kişiyi daha önce eşleşmediği koşulla eşleştirir; koşula uyan yeni bir kişi de sayılır. Yazmalar: kimlik belirteci, `POST` ve `PATCH /v1/contacts` ve kişisini oluşturan bir abonelik. |

```json
{ "event": "subscribed", "list_id": "LIST-ID", "delay_minutes": 60, "basis": "marketing" }
```

`attribute_changed` geriye dönük kayıt yapmaz: yalnızca `backfill_days` 0 alır ve başladığında zaten eşleşen
kişiler hiçbir şey almaz. Bir yazma kişiyi yalnızca ait olduğu gelen kutularına kaydeder: harici kimliği ya da
görüşmesi olan, listelerinden birine abone olan ya da yazmanın geldiği gelen kutusu. Eksik, yanlış yere konmuş
ya da uygun olmayan alanlar `400 trigger_invalid` verir. `skip_triggers` ve içe aktarmalar üç olayı da atlar.

Zamanlanmış, etkin ya da duraklatılmış bir tetiklenen e-postanın adını verdiği liste silinemez
(`DELETE /v1/lists/{listId}` `409 list_in_use` döner); önce e-postayı durdurun.

### Pazarlama mı hizmet mi

İzin dayanağı oluştururken seçilir ve sonra değişmez.

| Dayanak | Kime gider | Alt bilgi | Gerekir |
|---|---|---|---|
| `marketing` | Yalnızca 1 ile 10 `audience.list_ids` listesine abone olan kişilere. | Listeden tek tıkla çıkış içeren olağan bülten alt bilgisi. | Listeler. |
| `service` | Uyan her kişiye, liste yok. | Kişinin gelen kutusunun ürününü kullandığını belirten, posta adresinizi ve tek tıkla durdurmayı içeren alt bilgi. | Başlatırken `confirm_service: true`. |

`service` dayanağını yalnızca ürünün kullanımıyla ilgili e-postalar için seçin: başlangıç, hesap, geri bildirim
isteği. Reklam ve kampanyalar her zaman `marketing`'dir. `service` e-postada `list.name` birleştirme alanı
kullanılamaz. `service` e-postayı `confirm_service: true` olmadan başlatmak `400 service_basis_unconfirmed` döndürür;
Yuva onaylayanı ve zamanı saklar, denetim günlüğü başlatmayı kaydeder.

Hizmet alt bilgisindeki tek tıkla durdurma, o kişiye gelen kutusunun hizmet amaçlı tetiklenen e-postalarını ve
[tasarımlı iletileri](designs.md#8-konuşmada-tasarım-kullanma) durdurur; kişiyi hiçbir listeden çıkarmaz. Her iki dayanak da
geri dönen ya da şikâyet edilen adresleri ve çalışma alanından gelen tüm e-postaya itiraz edenleri atlar.

### Başlatma, geriye dönük kayıt ve durdurma

Başlatma zamanlamaya benzer (`POST /v1/broadcasts/{broadcastId}/schedule`): `send_at` anında (o zamana kadar
`scheduled`) ya da hemen `active` olur. Önce gözden geçirme geçmelidir. Bundan sonra tetiği gerçekleşen her kişi bir
kez kaydedilir; hiç kimse iki kez kaydedilmez.

Başlatmadan önce tanınan kişiler hiçbir şey almaz; **geriye dönük kayıt** yapmadıkça: `backfill_days` (0 ile 30,
başlatırken ya da `trigger` içinde) başlatmadan önceki o kadar gün içinde tetiği gerçekleşen kişileri de kaydeder.
Gecikme tetikten sayılır, ama kimse başlatmadan önce gönderilmez. Gözden geçirme bunları sayar ve
`confirm_recipients` bu sayıya eşit olmalıdır (geriye dönük kayıt yoksa 0).

Kendi arka ucunuzdan kopyaladığınız kişiler de bir tetiktir: ürünün zaten sahip olduğu bir kullanıcı "hoş geldin"
e-postası alırdı. `POST /v1/contacts` ya da `PATCH /v1/contacts/{contactId}` ile `"skip_triggers": true` gönderin;
yazma kimseyi kaydetmez. İçe aktarmalar, destek masası taşımaları ve birleştirmeler asla tetiklemez.

| İşlem | Ne yapar |
|---|---|
| Duraklat (`POST .../pause`) | Göndermeyi durdurur. Yeni tetikler yine kaydedilir ve bekler. |
| Sürdür (`POST .../resume`) | Gözden geçirmeyi yeniden çalıştırır ve tasarımı şimdi kayıtlı haliyle dondurur. Zamanı 7 günden fazla önce geçen kişiler `late` ile atlanır. |
| Durdur (`POST .../cancel`) | Kalıcı olarak durdurur. Artık kayıt yapmaz ve bekleyenleri bırakır. Durdurulan e-posta yeniden başlatılamaz; yenisini oluşturun. |

**Etkinken** hızı, saat dilimini, gönderim saatlerini, gecikmeyi (bekleyen herkesin zamanını kaydırır) ve tıklama
takibini değiştirebilirsiniz. **Duraklatılmışken** ayrıca tasarımı, konuları, önizleme metnini, çevirileri, koşulları
ve dilleri değiştirebilirsiniz; sürdürmek yeniden gözden geçirir. Olay, listesi ya da koşulu, dayanak, listeler ve kanal taslaktan sonra
asla değişmez; bunun için yeni e-posta oluşturun. Duraklatma, sürdürme ve durdurma `broadcasts:send` ister.

### Kim bekliyor

Bülten sayfası kaydettiği kişileri listeler: gönderim zamanıyla bekleyenler, e-posta gönderilenler ve nedeniyle
atlananlar. API'de `GET /v1/broadcasts/{broadcastId}/enrollments` bunları listeler (`state`: `waiting`, `queued`,
`skipped`, `cancelled`); `waiting` için zamanı önce gelen ilk sırada. Bekleyen bir kişinin e-postayı almamasını
istiyorsanız `DELETE /v1/broadcasts/{broadcastId}/enrollments/{contactId}` onu listeden çıkarır: `removed` ile
atlanır ve bir daha kaydedilmez. Yalnızca bekleyen kişi çıkarılabilir (aksi halde `409 invalid_state`).

| Atlama nedeni | Anlamı |
|---|---|
| `no_address` | Kişinin e-posta adresi yok. |
| `blocked` | Kişiyi engellediniz. |
| `suppressed` | Adres geri döndü ya da şikâyet edildi. |
| `opted_out` | Kişi çalışma alanından gelen tüm e-postaya itiraz etti. |
| `stopped` | `service`: kişi gelen kutusunun bu tür e-postalarını durdurdu. |
| `not_subscribed` | `marketing`: listelerinden hiçbirine abone değil. |
| `not_matching` | Bir koşul ya da dil uymuyor. |
| `late` | Zamanı, uzun bir duraklatma ya da günlük sınır nedeniyle, işlenebilmesinden 7 günden fazla önce geçti. |
| `removed` | Bekleme listesinden çıkarıldı. |

Rapor kaydedilen ve bekleyen kişileri ve atlananları nedenine göre ekler. Sayılar 5 dakika içinde güncellenir.
Kaydedilen kişi başına olay ya da webhook gönderilmez.

Bir gelen kutusunda en fazla 20 tetiklenen e-posta aynı anda zamanlanmış, etkin ya da duraklatılmış olabilir
(`409 too_many_triggered`). Yedeği olmayan bir birleştirme alanı gözden geçirmede yalnızca uyarıdır, çünkü kişiler
henüz bilinmez.

## Barındırılan sunucularda onay

Herkese kayıt açan bir sunucu (`YUVA_SIGNUP=open`) bültenleri varsayılan olarak `approval` kipinde
çalıştırır; böylece tek bir özensiz çalışma alanı bütün sunucunun gönderim itibarına zarar veremez.
Yalnızca davetle çalışan kendi sunucunuz için bu geçerli değildir.

`approval` kipinde, kayıt yoluyla oluşturulan bir çalışma alanı, operatör onaylayana kadar bülten
zamanlayamaz. Bu sırada taslak yazabilir ve deneme gönderebilirsiniz.

1. Ayarlar → Çalışma alanı → Bültenler'i açın. Sahip durumu görür: izinli, operatörün onayı gerekiyor,
   operatör bekleniyor ya da askıda.
2. Onay iste'yi seçin; web sitenizi, kimin gönderdiğini ve e-postaların ne hakkında olduğunu, abonelerinizin
   nasıl izin verdiğini yazın. Operatör tam olarak bunu okur.
3. Durum "Sunucu yöneticisi bekleniyor" olur. Operatör onaylar ya da sizin göreceğiniz bir gerekçeyle
   reddeder; reddedildikten sonra yeniden isteyebilirsiniz. O zamana kadar gözden geçirmede "Gönderim izni"
   engelleyici görünür.

Onaylı çalışma alanları ve bir `approval` sunucusundaki her çalışma alanı ısınır: ilk gün 500 alıcıya
gönderebilir, her temiz günden sonra sınır ikiye katlanır (en az 100 e-posta gönderilmiş, geri dönme %3'ün,
şikâyet %0,1'in altında). Kötü bir gün bir katlamayı geri alır. Böyle bir sunucuda bir çalışma alanının
en fazla 1.000 bülteni olabilir; eskileri silin.

Bültenlerin `off` olduğu bir sunucuda listeler ve formlar çalışmaya devam eder, ama bülten oluşturmak,
değiştirmek ve göndermek `403 broadcasts_disabled` yanıtı verir.

## Teslim edilebilirlik

Yeni bir göndericinin e-postası, temeller doğru değilse spam'e düşer. [Teslim edilebilirlik kılavuzu](deliverability.md)
bunları anlatır: bir kanalın SPF, DKIM ve DMARC'ını sınayan denetim, Amazon SES, Postmark ya da başka bir
aktarıcıyla alan adı kurulumu, Yuva DKIM, Gmail ve Yahoo kuralları, Google Postmaster Tools ve ısınma.

- **Yalnızca izin verenlere gönderin.** İzin kayıtlarından kurulmuş bir liste en iyi korumanızdır.
- **Küçük başlayın.** Önce birkaç yüz ilgili aboneye gönderin, sonra büyütün.
- Bültenler için **ayrı bir hesap** kullanın; isterseniz `news.example.com` gibi bir alt alan adından.

Yuva her bülten e-postasına tek tıkla abonelikten çıkmalı (RFC 8058) `List-Unsubscribe`, `List-Id`,
`Feedback-ID` ve `X-Auto-Response-Suppress` ekler; böylece e-posta programları kendi abonelikten çık
düğmesini gösterir.

## API

Panel, diğer her şeyle aynı API'yi kullanır. Bir betikten bülten:

```sh
curl -X POST https://support.example.com/v1/broadcasts \
  -H "Authorization: Bearer $YUVA_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "inbox_id": "INBOX-ID",
    "channel_id": "CHANNEL-ID",
    "name": "October newsletter",
    "subject": "What is new in October",
    "design_id": "DESIGN-ID",
    "audience": { "list_ids": ["LIST-ID"] },
    "pace": { "per_hour": 10, "per_day": 100 }
  }'
```

`pace` tek başına `PATCH` edilirse zamanlanmış ya da çalışan bir bültenin hızı değişir; `{}` hızı kaldırır.

Sonra kontrol listesi için `GET /v1/broadcasts/{broadcastId}/review`, alıcı sayısı için
`POST .../audience` ve bir üyeye deneme göndermek için `POST .../test` (API anahtarı üyeleri
`member_ids` içinde belirtir). Anahtarlar [aşağıdaki kapsamları](#kapsamlar) ister. Gördüğünüz sayıyı
`confirm_recipients` olarak vererek zamanlayın:

```sh
curl -X POST https://support.example.com/v1/broadcasts/BROADCAST-ID/schedule \
  -H "Authorization: Bearer $YUVA_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"send_at": "2026-10-20T08:00:00Z", "confirm_recipients": 1280}'
```

## Başvuru

### Uç noktalar

| Uç nokta | Kullanım |
|---|---|
| `GET`, `POST /v1/broadcasts` | Taslakları listeler (`inbox_id`, `state`, `kind`) ve taslak ya da [tetiklenen e-posta](#8-tetiklenen-e-postalar) oluşturur (`kind: "triggered"`, `trigger`). |
| `GET`, `PATCH`, `DELETE /v1/broadcasts/{broadcastId}` | Okur, taslağı (ya da zamanlanmış veya çalışan bir bültenin yalnızca hızını) değiştirir, taslağı ya da bitmiş bülteni siler. |
| `POST /v1/broadcasts/{broadcastId}/audience` | Alıcıları sayar: alıcılar, kimin neden dışarıda kaldığı, boş kalacak birleştirme alanları. |
| `GET /v1/broadcasts/{broadcastId}/review` | Kontrol listesi. |
| `POST /v1/broadcasts/{broadcastId}/test` | Üyelere deneme gönderir (`member_ids`, `contact_id`). |
| `POST /v1/broadcasts/{broadcastId}/schedule` | Zamanlar ya da hemen gönderir (`send_at`, `confirm_recipients`). |
| `POST /v1/broadcasts/{broadcastId}/unschedule` | Başlamadan önce taslağa döndürür. |
| `POST /v1/broadcasts/{broadcastId}/pause`, `resume`, `cancel` | Çalışan bülteni yönetir. |
| `GET /v1/broadcasts/{broadcastId}/deliveries` | Alıcılar (`state`, `q`, `replied`). |
| `GET /v1/broadcasts/{broadcastId}/enrollments` | Tetiklenen e-postanın kişileri (`state`): kim ne zaman bekliyor, kime gönderildi, kim atlandı. |
| `DELETE /v1/broadcasts/{broadcastId}/enrollments/{contactId}` | Bekleyen bir kişiyi tetiklenen e-postadan çıkarır. |
| `GET /v1/broadcasts/{broadcastId}/report` | Sayılar, oranlar, zaman çizelgesi, bağlantılar ve yanıtlar. |
| `GET /v1/conversations?broadcast_id=` | Bir bültene verilen yanıtların açtığı konuşmalar. |
| `PATCH /v1/workspace` | `broadcasts_by_keys`, anahtarlar ve asistanlar için anahtar. |
| `POST /v1/workspace/broadcast-approval` | Operatörden onay ister (sahipler, üye oturumu). |

`GET /v1/workspace`, sunucunun `mode` değerini, çalışma alanının `status`, `daily_limit`, `sent_today` ve
`by_keys` değerlerini içeren bir `broadcasts` nesnesi döndürür. Bir kanalın `email.broadcast_smtp` ve
`email.broadcast_rate` alanları `PATCH /v1/channels/{channelId}` ile ayarlanır (`broadcast_smtp_remove: true`
hesabı kaldırır). Tam şekiller [API sözleşmesinde](../../openapi/openapi.yaml). İşlemeye değer hatalar:
`409 review_failed` (engelleyen `items` ile), `409 audience_changed`, `409 not_draft`, `409 invalid_state`, `409 pace_too_slow`,
`403 broadcasts_approval_required`, `403 broadcasts_suspended`, `403 broadcasts_by_keys_disabled`,
`403 broadcasts_disabled` ve `429 test_limit`.

### Kapsamlar

| Kapsam | Anlamı |
|---|---|
| `broadcasts:read` | Bültenleri, alıcı sayılarını, gözden geçirmeleri ve raporları okur; alıcı listesi ve tetiklenen e-postanın kayıtlı kişileri ayrıca `contacts:read` ister. |
| `broadcasts:write` | Taslak oluşturur, değiştirir ve siler; deneme gönderir; bir kişiyi tetiklenen e-postadan çıkarır. |
| `broadcasts:send` | Zamanlar, zamanlamayı geri alır, duraklatır, sürdürür ve iptal eder. Hiçbir zaman örtük verilmez. |

Bu kapsamlar var olmadan önce yapılmış anahtarlar bunları almaz.

### Olaylar

| Ad | Anlamı |
|---|---|
| `broadcast.updated` | [Webhook](webhooks.md) ve olay akışı olayı: bülten durum ya da hız değiştirdi. `data.broadcast` bültendir, `data.previous_state` eski durumdur. |
| `broadcast.progress` | Yalnızca gerçek zamanlı (`/v1/realtime`): hazırlanan ya da gönderilen bir bültenin sayıları, en fazla 5 saniyede bir. Saklanmaz, yeniden oynatılmaz; yeniden bağlandıktan sonra bülteni tekrar okuyun. |

İkisi de bültenin gelen kutusunu görebilen, `broadcasts:read` sahibi çağıranlara ulaşır. Yanıtlar,
konuşmanın zaman çizelgesine bir `broadcast_reply` girdisi ekler.

### Sınırlar

| Sınır | Değer |
|---|---|
| Bülten başına liste | 10 |
| Koşul, dil | 10, 20 |
| Ad, konu, önizleme metni | 200, 300, 300 karakter |
| İleri zamanlama | 90 gün |
| Deneme gönderimi | Çalışma alanı başına saatte 20; her biri 1 ile 5 üyeye |
| Duraklatma ve iptal şu sürede etki eder | Bir toplu işlem, en fazla 50 e-posta |
| Hız | Kanal başına saniyede 1 ile 100 e-posta; varsayılan 5 |
| Bülten hızı | Saatte, günde, 30 günlük ayda 1 ile 10.000.000; gönderim başına en fazla 365 gün |
| Geçici hatanın yeniden denemeleri | 5 dakika, 30 dakika, 2 saat, 6 saat sonra |
| HTML | 90 KB uyarı, 102 KB engeller, 500 KB reddedilir |
| Çalışma alanı başına bülten | `approval` sunucusunda 1.000 |
| Alıcı listesinin saklanması | Bülten bittikten 400 gün |
| Gelen kutusu başına tetiklenen e-posta | 20 zamanlanmış, etkin ya da duraklatılmış |
| Tetiklenen e-postanın gecikmesi | 0 dakika ile 365 gün |
| Geriye dönük kayıt | 0 ile 30 gün |
