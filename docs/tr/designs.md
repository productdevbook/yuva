# E-posta tasarımları ve şablonlar

Tasarım, bir e-postanın görünümüdür: başlık, metin, görsel ve düğme gibi bloklar ile renkleri ve
yazı tiplerini taşıyan bir tema. Bir kez panelde hazırlarsınız, Yuva ile gelen şablonlardan birinden
başlayabilirsiniz ve bültenlerinizde kullanırsınız. Sunucu tasarımı yaygın e-posta uygulamalarında
çalışan e-posta HTML'ine, düz metin kısmına ve koyu moda çevirir.

Tasarımları sahipler ve yöneticiler hazırlar. API anahtarları ve OAuth belirteçleri
`broadcasts:read` ya da `broadcasts:write` kapsamını ister. Asistanlar (agent) tasarımları
kullanamaz.

## 1. Şablonla başlayın

Panelde: Bültenler, sonra kenar çubuğunda Tasarımlar. Tasarımlar iki sekmeden oluşur: kendi
tasarımlarınız olan Benimkiler ve şablonların bulunduğu Galeri. Galeri altı başlangıç şablonu sunar:
Bülten, Ürün güncellemesi, Duyuru, Hoş geldiniz, Etkinlik ve Özet; her biri İngilizce, Türkçe ve
Almanca. Üstündeki çipler (Tümü, Bülten, Ürün, Duyuru, Başlangıç, Etkinlik, Özet) her seferinde tek
bir türü gösterir, Şablon dili anahtarı dili seçer; seçtiğiniz dilde metni olmayan şablonda "Metni
İngilizce" yazar.

Bir şablon kartını seçin ya da Yeni tasarım'ı (kısayol `C`) seçerek Yeni tasarım penceresini açın.
Pencerede Boş sayfa ve şablonlar kutucuk olarak görünür. Birini seçin, tasarıma bir Ad verin,
Gelen kutusunu (ya da Tüm çalışma alanı) seçin ve Oluştur ve düzenle'yi seçin. Şablonun görselleri
çalışma alanınızın görsellerine kopyalanır, sonra her şeyi değiştirebilirsiniz. Boş sayfa, sade
renklerle boş bir e-postayla başlar.

Benimkiler'de tasarımlarda `/` ile arayın. Arşivlenmiş tasarımları göster (`A`), arşivlenmiş
tasarımları Arşivlendi etiketiyle listeye ekler; birindeki Geri al düğmesi onu geri getirir.
Benimkiler ve Galeri `1` ve `2` tuşlarıdır.

Bir tasarım bir gelen kutusuna ya da tüm çalışma alanına aittir. Gelen kutusunun
[gönderen bilgileri](lists.md#1-gönderen-bilgilerini-girin) alt bilgiyi doldurur.

API ile: `name` ve `template: { "slug": "newsletter", "locale": "tr" }` ile `POST /v1/email-designs`.
`GET /v1/email-templates` şablonları listeler.

## 2. Tasarımı düzenleyin

Tasarımcı tam ekran açılır. Blokları paletten (ya da Katmanlar özetinden) sürükleyin ya da seçili
bloktan sonra eklemek için üzerinde Enter'a basın. Sağda Blok ayarları altında değiştirmek için bir bloğa
tıklayın.

| Blok | Kullanım |
|---|---|
| Bölüm | Kendi arka planı olan bir blok grubu. Bölümler iç içe geçmez. |
| Sütunlar | Telefonlarda alt alta dizilen iki ya da üç sütun. Sütun genişlikleri 100 eder. Sütunlar iç içe geçmez. |
| Başlık | Üç boyut. |
| Metin | Paragraflar, listeler ve bağlantılar. |
| Görsel | Yüklenmiş bir resim; isteğe bağlı bağlantı ve alt metin. |
| Düğme | Düğme gibi görünen bir bağlantı. |
| Ayırıcı, Boşluk | İnce bir çizgi, boş alan. |
| Sosyal bağlantılar | Profillerinize bağlantılar, simge ya da metin olarak. |
| Alt bilgi metni | Zorunlu alt bilginin üstüne kendi satırlarınız. |
| Mesaj | Tasarım bir konuşmada kullanıldığında bir üyenin yazdığı metnin yeri ([bölüm 8](#8-konuşmada-tasarım-kullanma)). En çok bir tane, sütunlarda olmaz. |

Başlıklar, metinler, alt bilgi metni ve düğme etiketleri yerinde düzenlenir. Metin yalnızca e-posta
uygulamalarının iyi işlediklerini korur: kalın, italik, altı çizili, üstü çizili, bağlantı, liste,
satır sonu ve kod. Bağlantılar `https://`, `http://`, `mailto:` ya da `tel:` ile başlar.

Her e-posta, kaldıramayacağınız bir alt bilgiyle biter: gelen kutusunun yasal unvanı ve posta
adresi, kişinin e-postayı neden aldığı, abonelikten çıkma ve tercih bağlantıları. Gelen kutusunda
henüz yasal unvan ya da posta adresi yoksa bir uyarı bunu söyler.

Bir tasarım en çok 200 blok taşır.

### Birleştirme alanları

Birleştirme alanları her kişinin kendi bilgilerini e-postaya koyar. Bir metinde Ekle menüsünden
ekleyin ya da yazın. Değeri boş olan kişiler için `|` sonrasına yedek değer yazın:

```
Merhaba {{ contact.first_name | "değerli okurumuz" }},
```

| Alan | Değer |
|---|---|
| `contact.name` | Kişinin adı. |
| `contact.first_name` | Adın ilk sözcüğü. |
| `contact.email` | E-posta adresi. |
| `contact.attributes.<anahtar>` | Kişinin bir özniteliği, anahtarıyla. |
| `inbox.name` | Gelen kutusunun adı. |
| `list.name` | E-postanın gittiği liste. |
| `unsubscribe_url`, `preferences_url` | Kişinin kendi bağlantıları. Bağlantılarda birleştirme alanları yalnızca `?` sonrasında olabilir. |

Yuva'nın tanımadığı bir alan reddedilir. Yedek değer en çok 100 karakterdir. Değerler asla işaretlemeye
dönüşmez. Yedek değeri olmayan alan "Merhaba ," gibi bir boşluk bırakabilir; tasarımcı bunu uyarır.

## 3. Tema ve koyu mod

Renkler, yazı tipleri, içerik genişliği (480 ile 720 piksel), köşe yuvarlaklığı ve yön (soldan
sağa, sağdan sola ya da metne göre otomatik) için Tasarım ayarlarını açın (kısayol `T`). Yazı tipleri sistem
yığınlarıdır: system, humanist, geometric, serif ve monospace; e-postalar web yazı tipi yüklemez.

E-posta uygulaması koyu modda olan okurlar, sizin renklerinizden türetilmiş daha koyu renkleri alır;
metin 4,5:1 kontrastta tutulur. Koyu mod bölümünde bu renklerden istediğinizi kendiniz belirleyebilirsiniz.
Saydam arka planlı koyu bir logo, koyu modda arkasına açık bir plaka alabilir.

## 4. Görseller

Görsel kitaplığını bir görsel bloğundan açın (kısayol `I`) ya da dosyaları üzerine bırakın.

| | |
|---|---|
| Türler | PNG, JPEG, GIF, WebP. SVG betik taşıyabildiği için reddedilir. |
| Yükleme boyutu | En çok 10 MiB; 40 megapiksel ve kenar başına 8.000 piksel. |
| Saklanan | Çözülüp EXIF, GPS ve renk profili bilgisi olmadan yeniden kodlanır. 1.200 pikselden genişse 1.200'e küçültülür. WebP, PNG olarak saklanır; saydamlığı yoksa JPEG olarak. |
| GIF | Olduğu gibi, animasyonuyla saklanır: en çok 1.200 piksel genişlik ve 5 MiB. |
| Aynı dosya iki kez | Elinizdeki görselle yanıtlanır. |
| Adres | Her görselin sunucunuzda herkese açık bir `/i/{token}` adresi vardır: çerez yok, bir yıl önbellek, istek başına hiçbir şey kaydedilmez. |

Her görsele ekran okuyucular için bir açıklama yazın ya da süs amaçlı olarak işaretleyin. Bir çalışma
alanının görselleri birlikte en çok `YUVA_EMAIL_IMAGES_MAX_BYTES` kadar yer tutabilir
([Yapılandırma](configuration.md)).

Arşivlenmemiş bir tasarımın kullandığı görsel silinemez; panel tasarımları adıyla söyler. Bir bültende
gönderilmiş görsel onay ister, çünkü teslim edilmiş e-postalarda kırık görsel görünür. Hiçbir tasarımın
kullanmadığı ve hiçbir bültende gönderilmemiş görseller yüklemeden 7 gün sonra silinir.

Bir [yapay zekâ asistanı](mcp.md#listeleri-tasarımları-ve-kurulumu-bir-asistanla-yapın) `upload_email_image` ile herkese açık bir `https` URL'sinden ya da base64 olarak (MCP üzerinden yaklaşık 5 MB) görsel ekleyebilir ve tasarımları `list_designs` ve `get_design` ile okuyabilir.

## 5. Önizlemeler ve denetimler

Önizleme sunucudan gelir, yani alıcıların göreceği şeydir. Üstte Düzenle ile Önizleme arasında
geçin; Önizleme'de Masaüstü ya da Telefon'u seçin, Koyu'yu açıp kapatın. Tuşlar: `1` Düzenle,
`2` Masaüstü, `3` Telefon, `4` Koyu.

Denetimler düğmesi kaç sorun olduğunu yeşil, sarı ya da kırmızı gösterir. Tasarımın üstünde Denetimler
şeridini açar (kısayol `C`): e-postanın boyutunu 102 KB'a göre gösteren bir ölçer ve her sorun için,
Düzelt ya da Kontrol et olarak işaretlenmiş, bloğu seçen bir Göster düğmesi olan bir satır. "Sorun
bulunmadı." listenin boş olduğunu söyler. İşaretlenenler şunlardır:

| Uyarı | Anlamı |
|---|---|
| Zor okunuyor | Metin arka planına göre 4,5:1 kontrastın altında, açık ya da koyu renklerde. |
| 90 KB üstü | Gmail e-postaları yaklaşık 102 KB HTML'de keser ve sonrasını, abonelikten çıkma bağlantısı dahil, gizler. Tasarımcı 90 KB'ta ve 102 KB üstünde uyarır. |
| Görsel yok, açıklama yok | Görseli olmayan ya da alt metni olmayan görsel bloğu. |
| Yedek değer yok | Bazı kişiler için boş kalabilecek bir birleştirme alanı. |
| Gönderen bilgileri eksik | Gelen kutusunda yasal unvan ya da posta adresi yok. |

500 KB'ı aşan HTML'li tasarım reddedilir.

## 6. Kaydetme, sürümler ve çakışmalar

Tasarımcı son değişikliğinizden 1,5 saniye sonra kaydeder; `Ctrl`/`Cmd` + `S` hemen kaydeder ve bir
sürüm saklar. Geri al ve yinele (`Ctrl`/`Cmd` + `Z`, yinelemek için `Shift` ile) 100 adım geri
gider. Çevrimdışıyken değişiklikler cihazınızda kalır ve geri geldiğinizde kaydedilir.

Sağdaki Sürümler sekmesi (kısayol `V`) en yeni 50'yi tutar: düzenlerken en çok 10 dakikada bir, her elle kayıtta ve
her geri yüklemede bir. Geri yükleme, o sürümü en yeni sürüm olarak kaydeder.

Başka biri (ya da sizin başka bir sekmeniz) önce kaydettiyse bir pencere üç yol sunar: benimkini
koru, benimkini kopya olarak kaydet ya da onlarınkini yükle. Düzenlemeniz hiç kaybolmaz, diğer sürüm de
sürümlerde kalır. Hangisini seçerseniz seçin, karşı tarafın sürümü orada "Çakışmada saklandı" adıyla
tutulur; hiçbir düzenleme kaybolmaz.

JSON indir, Kopyasını oluştur ve Arşivle Diğer işlemler menüsündedir. Arşivle, tasarımı listeden
gizler ve sürümlerini tutar; arşivlenmiş tasarımdaki Geri al onu geri getirir. Yuva'nın daha yeni bir sürümünün yaptığı belge görüntülenebilir ama
değiştirilemez.

Kısayollar: `A` blok ekle, `T` tasarım ayarları, `I` görsel, `V` sürümler, `C` denetimler, `Ctrl`/`Cmd` + `D` bloğu çoğalt,
`Backspace` sil, `Alt` + `↑`/`↓` taşı, `Esc` önce metinden sonra bloktan çıkar.

## 7. API

Panelin yaptığı her şey [API sözleşmesinde](../../openapi/openapi.yaml) var. Şu istek bir tasarımı
kaydetmeden işler:

```sh
curl -X POST https://support.example.com/v1/email-designs/render \
  -H "Authorization: Bearer $YUVA_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "inbox_id": "INBOX-ID",
    "document": {
      "schema": 1,
      "theme": {
        "width": 600,
        "font": "system",
        "colors": {
          "page": "#f4f4f5", "card": "#ffffff", "text": "#18181b", "muted": "#52525b",
          "accent": "#1d4ed8", "link": "#1d4ed8", "border": "#e4e4e7"
        }
      },
      "blocks": [
        { "id": "h", "type": "heading", "level": 1, "text": "Merhaba {{ contact.first_name | \"değerli okurumuz\" }}" },
        { "id": "b", "type": "button", "label": "Devamını oku", "url": "https://example.com/news" }
      ]
    }
  }'
```

Yanıtta `html`, `text`, `size_bytes` ve `warnings` bulunur. Koyu palet için `"dark": true` gönderin.
Kayıt, başladığı `version` değerini `base_version` olarak gönderir; tasarım bu arada değiştiyse yanıt,
güncel tasarımı `design` içinde taşıyan `409 version_conflict` olur. Geçersiz belge, her sorun için bir
kayıtla `errors` taşıyan `422 invalid_design` döner.

## 8. Konuşmada tasarım kullanma

Bir gelen kutusundan giden yanıt ya da yeni e-posta, düz metin yerine tasarımlarınızdan biriyle
gidebilir. Tasarımda tam olarak bir Mesaj bloğu olmalıdır; üst düzeyde ya da bir bölümün içinde
(sütunlarda olmaz). Blok kendi metnini taşımaz; üyenin yazdığı metin buraya yerleştirilir, ardından
yardım makalesi kartları ve üyenin imzası gelir. Mesaj bloğu olan bir tasarım bülten olarak
gönderilemez: incelemesi ve deneme gönderimleri onu reddeder.

Sahip ya da yönetici tasarımı konuşmalarda sunar (`PUT /v1/email-designs/{designId}` üzerinde
`for_conversations`). Tasarımın gelen kutusunu gören her üye ile `conversations:read` yetkili anahtar ve
belirteçler, sunulan tasarımları listeleyebilir (`GET /v1/email-designs?for_conversations=true`) ve
okuyabilir; düzenleme sahiplerde, yöneticilerde ve `broadcasts:write` yetkisinde kalır.

Panelde, e-postayla yazışılan bir konuşmanın ve yeni e-postanın yazma alanında bir tasarım seçici
bulunur. Bir tasarım seçin, mesaj onunla gider. API'de `design_id` (isteğe bağlı `design_locale`)
`POST /v1/conversations/{id}/messages` (bir yanıt ya da açtığınız konuşmanın ilk mesajı) ve
`POST /v1/conversations/mail` isteğine girer; `PATCH /v1/messages/{id}` bir taslağın tasarımını değiştirir.

Nasıl çalışır:

- Kişinin dil sürümü kullanılır, bülteklerdeki gibi seçilir; `design_locale` başka birini seçer. Metin
  kaçışlanarak Mesaj bloğuna konur, boş satır yeni paragraf başlatır.
- E-postanın metin kısmı, e-postanın tamamının metin halidir. Konuşma yine yazılanı gösterir, tasarımı
  değil; `GET /v1/messages/{id}/email` e-postayı gönderildiği gibi döndürür.
- Birleştirme alanları kişinindir. `list.name`, `unsubscribe_url` ve `preferences_url` reddedilir; kişi
  için boş olan ve yedeği bulunmayan bir alan mesajı reddettirir (`422 invalid_design`).
- Başlıklar, alıcılar, ileti dizisi ve SMTP hesabı düz bir yanıtınkiyle aynıdır.
- Tasarımlar yalnızca e-postayla giden mesajlarda geçerlidir: notlarda, sohbette ve uygulama
  konuşmalarında değil (`400 design_not_emailed`).
- Arşivlenmiş ya da artık sunulmayan bir tasarım `409 design_unavailable` döndürür.
- HTML'i 500 KB'ı aşan e-posta `422 html_too_large` döndürür.
- Tasarımlı yeni e-posta tek bir `to` adresine ve `cc` olmadan gider (`400 design_one_recipient`).

Bir kişinin alacağını `POST /v1/conversations/{id}/design-preview` ile önizleyin: kişinin sürümü ve
birleştirme değerleri, imzanız, alt bilgi ve metniniz (ya da tasarımın yer tutucusu). `html`, `text`,
`subject` ve `footer` döner, hiçbir şey kaydedilmez ya da gönderilmez; dakikada 120 işleme hakkı
`POST /v1/email-designs/render` ile ortaktır.

### Alt bilgi: yanıt ya da ilk iletişim

| Alt bilgi | Ne zaman | İçeriği |
|---|---|---|
| `reply` | Kişi konuşmada yazmış. | Gelen kutusunun gönderen bilgileri satırı. |
| `outreach` | Kişi yazmamış: yeni konuşmanın ilk mesajı ya da yeni e-posta. | Gönderen bilgileri satırı, bir abonelikten çıkma bağlantısı ve tek tıklamalı `List-Unsubscribe` başlığı. |

Abonelikten çıkma bağlantısı ve tek tıklama, o gelen kutusundan kişiye tasarımla giden ilk iletişimi
durdurur; olağan yanıtlar etkilenmez. Tasarımlı ilk iletişim şu durumlarda reddedilir:

| Ret | Neden |
|---|---|
| `409 email_opted_out` | Adreste herhangi bir kapsamda engelleme ya da çalışma alanı çıkışı var, ya da kişi o gelen kutusundan gelen iletişimi durdurmuş. |
| `409 sender_identity_missing` | Gelen kutusunun [gönderen bilgilerinde](lists.md#1-gönderen-bilgilerini-girin) yasal ad ya da posta adresi yok. |
| `429 rate_limited` | Üye ya da anahtar başına saatte 20'den, çalışma alanı başına günde (UTC) 200'den fazla. |
| `403 broadcasts_approval_required`, `403 broadcasts_suspended` | Sunucu bültenler için onay istiyor ve çalışma alanınıza izin verilmemiş ya da askıya alınmış. |

Tasarımlı yanıtlarda ek sınır yoktur, tasarımsız ilk iletişim düz mesajın kurallarını izler.
Tasarımlar notlarda, sohbet ve uygulama konuşmalarında, yönlendirmelerde, kural yanıtlarında,
karşılama mesajlarında ve puan isteklerinde hiç kullanılmaz.

## Başvuru

### Uç noktalar

| Uç nokta | Kullanım |
|---|---|
| `GET`, `POST /v1/email-designs` | Listele (`inbox_id`, `q`, `archived=true`, `for_conversations`) ve bir `document`, bir `template` ya da boş olarak oluştur. |
| `GET`, `PUT`, `DELETE /v1/email-designs/{designId}` | Oku, kaydet (`base_version` ile), arşivle. |
| `POST /v1/email-designs/{designId}/duplicate` | Tasarımı kopyala. |
| `POST /v1/email-designs/render` | Bir belgeyi işle: HTML, metin, boyut ve uyarılar. Dakikada en çok 120. |
| `POST /v1/conversations/{id}/design-preview` | Konuşmalarda sunulan bir tasarımla mesaj önizle. Kapsam `conversations:read`; dakikada 120 hakkı paylaşır. |
| `GET /v1/email-designs/{designId}/versions` | Saklanan sürümler. |
| `GET /v1/email-designs/{designId}/versions/{version}` | Belgesiyle bir sürüm. |
| `POST /v1/email-designs/{designId}/versions/{version}/restore` | Bir sürümü en yeni olarak kaydet. |
| `GET`, `POST /v1/email-images` | Görselleri listele ve yükle (multipart, tek `file`). |
| `DELETE /v1/email-images/{imageId}` | Görseli sil (gönderilmiş olan için `force=true`). |
| `GET /v1/email-templates`, `GET /v1/email-templates/{slug}` | Galeri (`locale=en`, `tr` ya da `de`). |
| `GET /i/{token}` | Bir görsel, herkese açık. |
| `GET /email-templates/{slug}/{locale}.html`, `.webp` | Bir şablonun önizlemesi, herkese açık. |

### Kapsamlar

| Kapsam | Anlamı |
|---|---|
| `broadcasts:read` | Tasarımları, sürümleri, görselleri ve şablonları oku; işle. |
| `conversations:read` | Konuşmalarda sunulan tasarımları listele, oku ve önizle. |
| `broadcasts:write` | Tasarım oluştur, kaydet, kopyala, arşivle ve geri yükle; görsel yükle ve sil. |

Gelen kutularıyla sınırlı bir anahtar, kendi gelen kutularının ve tüm çalışma alanına ait tasarımları
okur, yalnızca kendi gelen kutularının tasarımlarını değiştirir. Bu kapsamlar var olmadan önce
oluşturulan anahtarlara verilmez.

### Sınırlar

| Sınır | Değer |
|---|---|
| Bir tasarımdaki blok | 200 |
| Tasarım belgesi | 300 KiB JSON |
| İşlenmiş HTML | 90 KB uyarı, 102 KB kesilir, 500 KB reddedilir |
| Saklanan sürüm | 50 |
| Görsel yükleme | 10 MiB, 40 megapiksel, kenar başına 8.000 piksel |
| Yüklemeden sonra görsel genişliği | 1.200 piksel |
| Birleştirme alanı yedek değeri | 100 karakter |
| Bağlantı | 2.048 karakter |
| İşleme | Çağıran başına dakikada 120 |
| Henüz yazmamış kişilere tasarımlı mesaj | Üye ya da anahtar başına saatte 20, çalışma alanı başına günde 200 |
| Kullanılmayan görseller | Yüklemeden 7 gün sonra silinir |
