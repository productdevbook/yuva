# Yardım merkezi

Yardım merkezi sık sorulan soruları müşterilerin ilk baktığı yerde yazıyla yanıtlar: ürününüz için
koleksiyonlar ve birden çok dilde makaleler içeren herkese açık sayfalar. Sayfadan, [web widget'ının](widget.md)
ana ekranından ve ekibinizin yanıt kutusundan aranabilir. Bir gelen kutusuna aittir; bu yüzden hâlâ
yardıma ihtiyacı olan müşteri aynı gelen kutusuna yazar ve sonuç bulamayan aramalar bir sonraki
hangi makaleyi yazmanız gerektiğini söyler.

Herkese açık sayfalar, panelsiz ve zorunlu JavaScript olmadan, Yuva sunucunuz tarafından düz HTML olarak
üretilir. Her gelen kutusunun en fazla bir yardım merkezi olur. Ürün dokümantasyonu kendi sitenizde kalır;
yardım merkezi Yuva'da yazılan destek makalelerini barındırır.

## 1. Merkezi oluşturun ve yayınlayın

Panelde: kenar çubuğunda Yardım, sonra gelen kutusunda yardım merkezi oluştur. Merkezleri sahipler ve
yöneticiler oluşturur ve değiştirir; makaleleri kimin yayınlayabileceği için bkz.
[Taslaklar ve yayınlama](#taslaklar-ve-yayınlama).

| Ayar (API alanı) | Anlamı |
|---|---|
| Ad (`name`) | En fazla 100 karakter. |
| Açıklama (`description`) | En fazla 1.000 karakter, ana sayfada gösterilir. |
| Adres (`slug`) | 3 ile 48 karakter: `a-z`, `0-9` ve araya giren tireler. Sunucuda benzersiz; bazı sözcükler ayrılmıştır. |
| Diller (`locales`), varsayılan (`default_locale`) | Merkezin yayınladığı en fazla 20 [dil](widget.md#diller); varsayılan bunlardan biridir. |
| Logo (`logo_url`), vurgu rengi (`accent_color`) | Bir `https` görseli ve `#2563eb` gibi bir renk. |
| Site bağlantısı (`site_url`) | Ürününüz, bir `http(s)` bağlantısı. |
| Sohbet (`chat_channel_id`) | Sayfaların "Hâlâ yardıma mı ihtiyacınız var?" altında sunduğu sohbetin [sohbet kanalı](widget.md). Orada başlayan bir konuşma, ziyaretçinin okuduğu makaleyi taşır. |
| Analitik (`analytics_property_id`) | Herkese açık sayfaların çerezsiz sayfa görüntülemelerini sayan bir [Analitik](analytics.md) web sitesi. |
| Temsilciler yayınlayabilir (`agents_may_publish`) | Varsayılan olarak kapalı. Aşağıya bakın. |
| Yayında (`published`) | Varsayılan olarak kapalı. Yayınlanmamış bir merkez ekibiniz dışında herkese `404` verir; ekibiniz onu panelde önizler. |

Adresi değiştirirseniz eskisi merkeze ayrılmış kalır ve altındaki her herkese açık adres yenisine
yönlendirir.

API ile: `inbox_id`, `name` ve `slug` ile `POST /v1/help-centers`, sonra
`{"published": true}` ile `PATCH /v1/help-centers/{centerId}`.
`GET /v1/help-centers/{centerId}/preview`, merkez yayında olmasa bile bir sayfayı ziyaretçilerin
göreceği gibi döndürür.

## 2. Koleksiyonlar ve makaleler

*Koleksiyon*, "Faturalama" ya da "Başlarken" gibi makaleleri gruplar; sabit bir kümeden bir simgesi vardır
ve en fazla bir seviye alt koleksiyon alabilir. *Makale* bir koleksiyonda ya da hiçbirinde olur ve her
dil için bir *çevirisi* vardır. Koleksiyonların adı ve açıklaması dil başınadır; bir dilde yayımlanmış
makalesi olmayan koleksiyon o dilde gösterilmez.

Makale Markdown ile yazılır: başlıklar, listeler, tablolar, kod, bağlantılar ve görseller. Ham HTML
kaldırılır ve görseller yalnızca çalışma alanınıza yüklediyseniz kalır. Düzenleyiciden ya da
`POST /v1/help-centers/{centerId}/images` ile yükleyin ([e-posta görselleriyle](designs.md#4-görseller)
aynı türler, boyutlar ve yeniden kodlama; yanıt yapıştırılacak Markdown'ı taşır). Hiçbir taslağın
kullanmadığı ve hiçbir yayımlanmış makalenin göstermediği görsel, yüklemeden bir hafta sonra silinir.

| Alan | Sınır |
|---|---|
| Başlık (`title`) | 3 ile 200 karakter. |
| Gövde (`body`) | Markdown, en fazla 100.000 karakter. |
| Anahtar sözcükler (`keywords`) | Eş anlamlılar gibi ek arama sözcükleri için en fazla 500 karakter. |
| Adres (`slug`) | Başlıktan üretilir, düzenlenebilir. Bir URL'deki yanlış slug doğrusuna yönlendirir; bu yüzden makaleyi yeniden adlandırmak hiçbir bağlantıyı bozmaz. |
| Sayfalar (`paths`) | `/billing/*` gibi en fazla 20 sayfa yolu. Widget makaleyi bu sayfalarda en iyi eşleşme olarak önerir. |

Bir çalışma alanının en fazla `YUVA_HELP_MAX_ARTICLES` makalesi olabilir
([Yapılandırma](configuration.md#yardım-merkezi)); aşılırsa makale oluşturmak `422 hosted_limit` verir.

API ile, `/v1/help-centers/{centerId}` altında: `/collections`, `/articles` ve
`/articles/{articleId}`. `GET /articles`, `collection_id`, `locale` ve `state` ile süzer.

## Taslaklar ve yayınlama

Bir çevirinin çalışma taslağı vardır ve yayınlandığında dondurulmuş bir yayımlanmış kopyası olur.
Kaydetmek yalnızca taslağı değiştirir; herkese açık sayfa siz yayınlayınca değişir.

- **Kaydetme.** Her kayıt, düzenleyicinin yüklediği `base_version` değerini taşır. Arada biri kaydetmişse
  kayıt `409 version_conflict` ile reddedilir ve yanıt çeviriyi şimdiki haliyle içerir; böylece kimse
  başkasının işinin üzerine yazmaz. Açık diğer düzenleyicilere makalenin değiştiği bildirilir.
- **Yayınlama.** Yayınla, taslağı dondurur, işler ve web'e koyar; yayından kaldır, çeviriyi web'den alır
  ve taslağını korur. Bir çeviri, taslağı yayımlanmış kopyadan farklı olduğu sürece "yayınlanmamış
  değişiklikler" gösterir.
- **Kim yayınlayabilir.** Gelen kutusuna erişimi olan temsilciler taslak, koleksiyon ve çeviri yazar.
  Sahipler ve yöneticiler ayrıca yayınlar, yayından kaldırır, yayımlanmış makaleleri siler ve merkezin
  ayarlarını değiştirir. Gelen kutusuna erişimi olan temsilcilerin de yayınlamasına izin vermek için
  Temsilciler yayınlayabilir (`agents_may_publish`) ayarını açın.
- **Asistanlar ve OAuth.** [MCP](mcp.md#yardım-merkezini-bir-asistanla-kullanın) araçları ve OAuth
  belirteçleri ayar ne olursa olsun asla yayınlamaz: asistan taslak yazar, bir üye yayınlar. API
  anahtarları `help:publish` yetkisiyle yayınlayabilir.

API ile: `PUT /v1/help-centers/{centerId}/articles/{articleId}/translations/{locale}` taslağı kaydeder;
aynı yola `/publish` ya da `/unpublish` eklenmiş `POST` yayınlar ya da yayından kaldırır.
[Webhook'lar](webhooks.md) `help.article.published`, `help.article.unpublished` ve
`help.article.deleted` olaylarını gönderir.

## Diller ve çeviriler

Merkez `locales` içindeki dilleri yayınlar; makaleler ve koleksiyonlar yalnızca bu dillerde yazılır
(aksi halde `400 locale_not_offered`). Yuva çeviri modeli çalıştırmaz: çeviri sizin ya da ekibinizin
yazdığı metindir. Düzenleyici, çevirdiğiniz metnin yanında varsayılan dildeki metni gösterir, varsayılandan
eski çevirileri işaretler ve eksik olanları listeler.

Herkese açık sayfalar adreslerindeki dile uyar (`/en/…`, `/tr/…`); Yuva tarayıcının diline göre
yönlendirmez. Her sayfa, arama motorları için yayımlanmış diğer dillerini listeler. Sayfanın kendi
sözcükleri (arama, "Bu yararlı oldu mu?") İngilizce, Türkçe ve Almanca vardır; diğer diller İngilizce
gösterir. Widget ziyaretçinin dilini merkezin listesinden seçer (önce tam eşleşme, sonra ana dil, yoksa
varsayılan) ve çevirisi olmayan makaleyi varsayılan dilde gösterir.

## Revizyonlar

Her çeviri son 50 revizyonunu saklar: kaydedilen taslak (yazar başına en fazla 10 dakikada bir), her
yayınlama ve her geri yükleme. Bir çevirinin geçmişini açın ve bir revizyonu taslağa geri yükleyin;
yalnızca siz yayınlayınca yayınlanır. API ile: `GET …/translations/{locale}/revisions` ve
`POST …/revisions/{revisionId}/restore`.

## Herkese açık sayfalar

Yayımlanmış her merkez sunucunuzda `/help/{slug}` adresinde ya da `<YUVA_HELP_URL>/{slug}` adresindedir.
Altında:

| Adres | Sayfa |
|---|---|
| `/{locale}/` | Ana sayfa: arama, koleksiyonlar ve popüler makaleler. |
| `/{locale}/collections/{number}-{slug}` | Alt koleksiyonları ve makaleleriyle bir koleksiyon. |
| `/{locale}/articles/{number}-{slug}` | İçindekiler, ilgili makaleler ve "Bu yararlı oldu mu?" ile bir makale. |
| `/{locale}/search?q=` | Arama sonuçları. |
| `/sitemap.xml` | Arama motorları için yayımlanmış tüm sayfalar. |

"Bu yararlı oldu mu?" JavaScript olmadan çalışır: Evet ve Hayır düğmeleri bir formu
`/{locale}/articles/{number}-{slug}/helpful` adresine gönderir ve sayfa `?thanks=up` ya da
`?thanks=down` ile geri gelir; bir teşekkür satırı ve yanıtı değiştirmek için bir düğme gösterir. Bir betik
mümkün olduğunda formu sayfadan ayrılmadan gönderir. Sayfalar bir dakika önbelleğe alınır; kurallı bir
bağlantı, her dil için alternatifler, Open Graph etiketleri ve yapılandırılmış veri içerir. Herkese açık
sayfalar adres başına dakikada 120 isteğe yanıt verir.

`YUVA_HELP_URL`, sunucuya yönlendirdiğiniz bir origin'dir, örneğin `https://help.example.com`.
[Yapılandırma](configuration.md#yardım-merkezi) bölümünde atayın; merkezler bu durumda `<origin>/{slug}`
adresindedir ve origin `YUVA_PUBLIC_URL`'den farklı olmalıdır. Merkez başına kendi alan adı henüz
desteklenmez. `YUVA_HELP=off` yardım merkezlerini tümden kaldırır: yolları `404` verir.

## Arama ve rapor

Panel, widget, herkese açık sayfalar ve asistanlar dil başına tek bir tam metin aramayı paylaşır.
Tüm sözcükler eşleşmelidir ve sonuncusu önek olarak eşleşir; böylece sonuçlar yazarken görünür. Hiçbir
makalede tüm sözcükler yoksa bazılarını içeren makaleler gösterilir. Başlık en çok, sonra anahtar
sözcükler, sonra gövde ağırlık taşır. Panelde üyelerin aramaları sayılmaz.

Rapor (Yardım → merkez → Rapor ya da `GET /v1/help-centers/{centerId}/report?days=7|30|90`) son 7, 30
ya da 90 gün için şunları gösterir:

- en çok görüntülenen makaleler, Evet'in Hayır'a oranı, bir makaleden başlayan konuşmalar ve kart olarak
  gönderilme sayısı;
- aramalar ve sonuç bulamayanlar, en sık olandan başlayarak: sıradaki yazılacak makaleler;
- en çok Hayır alan makaleler: geliştirilecek olanlar.

Sayaçlar ziyaretçi verisi saklamaz. Görüntülemeler yaklaşıktır: adres, makale ve saat başına bir tane
sayılır ve bir CDN'in sunduğu sayfalar görülmez. E-posta adresine, URL'ye ya da sayıya benzeyen aramalar
`(scrubbed)` olarak sayılır.

## Web widget'ında

Gelen kutusunun yayımlanmış bir merkezi olduğunda [widget](widget.md#ziyaretçi-neler-görür)'ın ana
ekranında bir arama alanı ve bir Yardım merkezi uygulaması bulunur. Ziyaretçinin bulunduğu sayfa için
en iyi eşleşmeleri (makalelerin `paths` değerinden), popüler makaleleri ve koleksiyonları gösterir,
makaleleri panelin içinde açar ve "Bu yararlı oldu mu?" diye sorar. Alttaki bir düğme konuşma başlatır.
Ziyaretçi bir makaleyi okuduktan sonra yazarsa konuşma bunu `help_article_id` olarak kaydeder ve
ekibiniz müşterinin önce hangi makaleyi okuduğunu görür. Kimlik yalnızca gelen kutusunun merkezindeki
yayımlanmış bir makale için saklanır.

Kendi arayüzünüz için `yuva-help`, `yuva-help-search`, `yuva-help-article` [shadcn bileşenleri](https://useyuva.com/shadcn)
ve `use-yuva-help` hook'ları, [headless istemcinin](headless.md) aynı
`/client/v1/channels/{channel_key}/help` uç noktalarını kullanır.

## Konuşmalarda makale kartları

Bir konuşmada, yanıt kutusunun Yardım düğmesi ve `/` araması yayımlanmış makaleleri bulur. En fazla
üçünü seçip gönderin: müşteri kendi dilinde (kişinin, yoksa gelen kutusunun, yoksa merkezin varsayılan
dili) başlık ve özet içeren bir kart alır. Sohbette kart makaleyi widget'ın içinde açar; e-postada metin
bölümünde "Başlık - URL", HTML bölümünde izlemesiz bir bağlantı bloğu bulunur. API ile
`POST /v1/conversations/{id}/messages` gelen kutusunun merkezindeki en fazla 3 yayımlanmış makaleyi
`article_ids` ile alır. Bir makaleyi sonradan yayından kaldırırsanız eski mesajlar okunabilir kalır ve
bağlantı merkezin bulunamadı sayfasını gösterir.

## Zendesk veya Help Scout'tan içe aktarma

Var olan bir yardım merkezini bunun içine taslak olarak alın. Hiçbir şey yayımlanmaz, kaynak yalnızca
okunur.

- **Zendesk Guide.** Oturumu açık bir Zendesk taşımasını (Yuva'ya taşın) kullanın; salt okunur
  oturumu yeniden kullanılır, yeni izin istenmez. Kategoriler koleksiyon, bölümler alt koleksiyon olur
  (bölüm içindeki bölüm `Üst / Alt` adlı alt koleksiyon olur). Her makale, merkezin sunduğu dillerdeki
  çevirileriyle gelir; diğer diller atlananlar olarak listelenir, dili merkezin dillerine ekleyip
  yeniden içe aktarın.
- **Help Scout Docs.** Help Scout'ta bir Docs API anahtarı oluşturun (Your Profile, API Keys) ve içe
  aktarmaya verin; Yuva anahtarı içe aktarma bitene kadar mühürlü saklar. Koleksiyonlar koleksiyon,
  kategoriler alt koleksiyon olur. Docs sitelerinin dili yoktur: makalelerin yazıldığı dili seçin.
- **Yolda neler değişir.** Makale HTML'i Markdown olur. Görseller indirilir, merkezin görselleri olarak
  saklanır ve yeniden yazılır; alınamayan görsel bağlantı olarak kalır. İçe aktarılan makaleler arası
  bağlantılar yeni sayfalarına yönelir.
- **Yeniden çalıştırma.** Daha önce alınan makale taslağını günceller, çoğalmaz. Yuva'da sonradan
  düzenlediğiniz taslak, üzerine yazmayı istemedikçe korunur.
- **Yönlendirmeler.** İçe aktarma her eski adresi yeni adresiyle listeler (CSV olarak indirin).
  Eski adreste yönlendirmeyi kurmak size aittir; yeni adres, makaleyi yayımlayana kadar `404` verir.

`POST /v1/help-centers/{id}/imports` ile başlatın (yetki `help:write`); bir asistan Zendesk içe
aktarmasını başlatıp `import_help_articles` ve `get_help_import` araçlarıyla izleyebilir.

## API

Hepsi `/v1/help-centers` altındadır ve [API başvurusunda](headless.md) `help` etiketini taşır.

| Yetki | İzin verdiği |
|---|---|
| `help:read` | Merkezler, koleksiyonlar, taslakları ve revizyonlarıyla makaleler, arama, rapor. Yetkisi adlandırılmadan oluşturulan anahtarlara verilir. |
| `help:write` | Koleksiyon ve makale oluşturma ve değiştirme, taslak kaydetme, görsel yükleme. Yetkisi adlandırılmadan oluşturulan anahtarlara verilir. |
| `help:publish` | Yayınlama, yayından kaldırma ve (`help:write` ile) yayımlanmış makaleleri silme. Hiçbir yetkiye dahil değildir: adıyla verin. |

Gelen kutularıyla sınırlı bir anahtar yalnızca o gelen kutularının merkezlerini görür; diğer her merkez
`404` verir. Mevcut anahtarlar bu yetkilerden hiçbirini kazanmaz.
