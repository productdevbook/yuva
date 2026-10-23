# Intercom'dan taşıyın

Bir Intercom çalışma alanının konuşmalarını Yuva'ya kopyalayın ve destek adresini Yuva'ya yönlendirin. Yuva
Intercom'u yalnızca okur. Genel bakış [Yuva'ya taşı](migrate.md) sayfasındadır.

## Başlamadan önce

- Kendi çalışma alanınızda Yuva için özel bir uygulama oluşturun. Intercom'da **Settings → Integrations →
  Developer Hub** yolunu açın, **New app**'i seçin ve adını "Yuva" koyun. **Configure → Authentication**
  altında yalnızca okuma izinlerini açın: konuşmaları okuma, kullanıcıları ve şirketleri okuma, yöneticileri
  okuma ve etiketleri okuma. Intercom'un sayfası:
  [Authentication](https://developers.intercom.com/docs/build-an-integration/learn-more/authentication).
- Aynı sayfadan uygulamanın **erişim belirtecini** (access token) kopyalayın.
- Bağlan adımında çalışma alanınızın verilerinin barındırıldığı **bölgeyi** (ABD, Avrupa veya Avustralya)
  seçin ve belirteci yapıştırın. Yuva belirteci yalnızca o bölgenin API sunucusuna Bearer belirteci olarak
  gönderir ve kimseye göstermez.

Yuva OAuth kullanmaz: kendi çalışma alanınızda oluşturduğunuz uygulamanın buna ihtiyacı yoktur. Yuva bir
belirtecin hangi izinlere sahip olduğunu öğrenemez; salt okunur kalmasını bağlantı sağlar: belirteçten başka
Intercom'a yalnızca okuma istekleri (`GET`) gönderir, dolayısıyla hiçbir konuşma, kişi veya ayar değişmez.
Yuva, `POST` gerektiren Intercom aramasını hiç kullanmaz.

## Neler kopyalanır

| Intercom | Yuva |
|---|---|
| Konuşmalar | Konuşmalar, tüm mesajlarıyla |
| Kullanıcı veya potansiyel müşterinin yorumları | Gelen mesajlar, yazan kişi adına |
| Yönetici veya botun yorumları | Giden mesajlar |
| Notlar | Yalnızca ekibinizin gördüğü notlar |
| Durum: open | Açık |
| Durum: snoozed | Beklemede |
| Durum: closed | Kapalı |
| Atama ve durum değişiklikleri | Hiçbir şey |
| Etiketler | Eşlediğiniz şekilde etiketler (veya hiç) |
| Yöneticiler | Eşlediğiniz şekilde ekip üyeleri; diğerleri adıyla saklanır |
| Atanan yönetici | Yönetici eşlenmişse atanan kişi |
| Ekipler | Eşlediğiniz şekilde gelen kutuları, ekip başına bir; ekibi olmayan konuşmalar "Unassigned" klasörüdür |
| Kullanıcılar ve potansiyel müşteriler | Kişiler; ad, e-posta ve telefonla (e-posta adresi olmayan kişi adıyla saklanır) |
| Ekler | Mesajlardaki dosyalar |

Konuşmalar özgün zamanlarını korur ve [sessizce](migrate.md#geçmiş-nasıl-sessiz-kalır) gelir.

## Neler kopyalanmaz

Biletler (Intercom'un ayrı bilet türleri), şirketler, özel öznitelikler, Yardım Merkezi makaleleri, iş
akışları, Fin yapılandırması, puanlamalar ve raporlar. Yuva yalnızca yukarıdaki tabloda yazanı kopyalar.

## Çok uzun konuşmalar

Intercom bir konuşmanın en fazla en yeni 500 parçasını döndürür. Yuva bunları içe aktarır ve konuşmayı
taşımada bir hata olarak (`truncated`) kaydeder; böylece hangilerinin en eski mesajlarının eksik olduğunu
görebilirsiniz. Mesajlara yazılan bağlantılar hiçbir zaman açılmaz; yalnızca Intercom'un eklediği dosyalar,
Intercom'un dosya sunucularından ve belirteç olmadan alınır.

## Sınırlar

Intercom özel uygulamalar için çalışma alanı başına dakikada 10.000 isteğe izin verir; bunu tüm uygulamalar
paylaşır ve 10 saniyelik pencerelerle sayılır. Yuva, Intercom'un bildirdiği sınırın (`X-RateLimit-Limit`)
yarısında kalır ve Intercom 429 yanıtladığında bekler. Intercom'un sayfası:
[Rate limiting](https://developers.intercom.com/docs/references/rest-api/errors/rate-limiting). Bu yüzden
büyük bir çalışma alanı biraz zaman alır: bir konuşmayı okumak iki istek tutar.

## Geçiş

Bugün destek adresiniz, onu barındıran posta sağlayıcısının yönlendirmesiyle ya da orada bir kuralla
Intercom'a ulaşıyor. Geçiş adımında kanalın [yönlendirme adresini](email.md#yönlendirme-adresi) kopyalayın
ve yönlendirmeyi destek adresi artık Yuva'ya ulaşacak şekilde değiştirin. Bunu yapana kadar Yuva, hiçbir
konuşma kaybolmasın diye Intercom'u iki dakikada bir okur. Yuva'dan gönderilen yanıtlar kanalın adresinden
çıkar.

Sonra **Deneme e-postası gönder**'e basın: mesaj yeni yoldan Yuva'ya ulaştığında çalışmış demektir. Sessiz
dönemden sonra ya da **Şimdi bitir**'e bastığınızda taşıma biter
([nasıl](migrate.md#deneme-e-postası-sessiz-dönem-ve-bitirme)).

## Bittikten sonra

Yuva için oluşturduğunuz uygulamayı silin: Intercom'da **Settings → Integrations → Developer Hub**. Yuva
taşıma bittiğinde belirtecini siler ama Intercom'da iptal edemez; erişimini bitiren, uygulamayı silmenizdir.
Yönlendirmeyi olduğu gibi bırakın; isterseniz Intercom'u arşiv olarak tutun.
