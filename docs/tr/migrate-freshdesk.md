# Freshdesk'ten taşıyın

Bir Freshdesk hesabının destek kayıtlarını Yuva'ya kopyalayın ve destek adresini Yuva'ya yönlendirin. Yuva
Freshdesk'i yalnızca okur. Genel bakış [Yuva'ya taşı](migrate.md) sayfasındadır.

## Başlamadan önce

- API anahtarınızı bulun. Freshdesk'te yönetici olarak oturum açın, sağ üstteki profil resminize tıklayın ve
  **Profile settings**'i seçin. Anahtar sağdaki **Your API Key** başlığının altındadır. Freshdesk'in
  sayfası: [Authentication](https://developers.freshdesk.com/api/#authentication).
- Bağlan adımında **API anahtarını** ve **alt alan adınızı** girin; `acme.freshdesk.com` içindeki `acme`
  budur. Sonra **Bağlantıyı dene**'ye basın. Bir yöneticinin anahtarını kullanın: bir temsilcinin anahtarı
  reddedilir, çünkü hesabın ürünlerini ve rollerini yalnızca yöneticiler okuyabilir.

Freshdesk'te kapsam ve salt okunur anahtar yoktur: anahtar, yöneticinin yapabildiği her şeyi yapabilir. Onu
salt okunur tutan Yuva'nın bağlantısıdır: Freshdesk'e okuma istekleri dışında hiçbir şey göndermez, onları da
yalnızca hesabınızın `freshdesk.com` adresine gönderir; bu yüzden hiçbir kayıt, kişi ya da ayar değişmez.

## Neler kopyalanır

| Freshdesk | Yuva |
|---|---|
| Destek kayıtları (ticket) | Konuşmalar, tüm mesajlarıyla |
| Kaydın açıklaması | İlk mesaj: gelen; bir temsilcinin giden e-postayla açtığı kayıtta giden |
| Müşteri yanıtları | Gelen mesajlar |
| Temsilci yanıtları | Giden mesajlar |
| Özel notlar | Yalnızca ekibinizin gördüğü notlar |
| Bir temsilcinin herkese açık notları | Yalnızca ekibinizin gördüğü notlar |
| Bir müşterinin herkese açık notu | Gelen mesaj |
| Durum: open ve özel durumlar | Açık |
| Durum: pending | Beklemede |
| Durum: resolved ve closed | Kapalı |
| Etiketler (tags) | Etiketler, eşlediğiniz gibi (ya da hiçbiri) |
| Temsilciler | Ekip üyeleri, eşlediğiniz gibi; diğerleri adıyla saklanır |
| Sorumlu (responder) | Atanan kişi, temsilci eşlenmişse |
| Talep sahipleri | Kişiler, adı ve telefonuyla |
| Ürünler | Gelen kutuları, her ürün eşlediğiniz gibi bir gelen kutusuna |
| Ürünü olmayan kayıtlar | Varsayılan gelen kutusu, "No product" klasörü olarak |
| Ekler | Mesajlardaki dosyalar |

Spam ve silinmiş kayıtlar hiç okunmaz: Freshdesk bunları listesinden çıkarır ve Yuva da bunları istemez.
Kayıtlar özgün zamanlarını korur ve [sessizce](migrate.md#geçmiş-nasıl-sessiz-kalır) gelir; bu yüzden içe
aktarılan hiçbir not ya da yanıt kimseye gönderilmez.

## Neler kopyalanmaz

Otomasyonlar, hazır yanıtlar, memnuniyet puanları, özel kayıt alanları, zaman girdileri, şirketler,
gruplar, roller, çözüm makaleleri ve raporlar. Yuva yalnızca yukarıdaki tabloda yazanı kopyalar.

## Ekler

Freshdesk ekleri başka bir sunucuda, süreli ve imzalı adreslerde tutar. Yuva onları API anahtarınız olmadan
oradan getirir. Bir adresin süresi dolmuşsa Yuva yenisi için kaydı bir kez yeniden okur; yine reddedilen ya
da sunucunun ek sınırından büyük bir dosya geride kalır.

## Sınırlar

Freshdesk istekleri plana göre dakikada sınırlar ve geçersiz istekleri de sayar. Yuva Freshdesk'in bildirdiği
sınırın yarısında kalır (bir sınır bildirene kadar dakikada 50), kayıt listesini dakikada en çok 20 kez ister
ve Freshdesk 429 yanıtı verince istediği kadar bekler. Freshdesk'in kayıt eksikliği dışında bir nedenle
reddettiği bir istek tekrarlanmaz, kopyalamayı bir süre durdurur. Bu yüzden büyük bir hesap biraz sürer.
Freshdesk'in sayfası: [Rate limit](https://developers.freshdesk.com/api/#ratelimit).

Freshdesk bir kayıt listesini 300 sayfada, yani 30.000 kayıtta keser. Yuva sonraki listeyi okuduğu son kaydın
değişiklik zamanından başlatır; böylece hiçbir kayıt iki kez okunmaz ya da atlanmaz.

## Geçiş

Bugün destek adresiniz Freshdesk'e ulaşıyor; bunu onu barındıran e-posta sağlayıcısı ya da orada bir kural
yönlendiriyor. Geçiş adımında kanalın [yönlendirme adresi](email.md#yönlendirme-adresi)ni kopyalayın ve bu
yönlendirmeyi destek adresi Yuva'ya ulaşacak şekilde değiştirin. Bunu yapana kadar Yuva Freshdesk'i iki
dakikada bir okur, böylece hiçbir kayıt kaybolmaz. Yuva'dan gönderilen yanıtlar kanalın adresinden çıkar.

Sonra **Deneme e-postası gönder**'e basın: ileti yeni yoldan Yuva'ya ulaştığında çalışıyor demektir. Sessiz
bir dönemden sonra ya da **Şimdi bitir**'e bastığınızda taşıma biter
([nasıl çalışır](migrate.md#deneme-e-postası-sessiz-dönem-ve-bitirme)).

## Bittikten sonra

API anahtarınızı sıfırlayın: Freshdesk'te **Profile settings**, ardından **Reset API key**. Yuva taşıma
bitince anahtarın kendi kopyasını siler ama Freshdesk'te sıfırlayamaz; bu yüzden erişimini anahtarı
sıfırlamak sonlandırır. Yönlendirmeyi olduğu gibi bırakın; isterseniz Freshdesk'i arşiv olarak tutun.
