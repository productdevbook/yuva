# Front'tan taşıyın

Bir Front şirketinin konuşmalarını Yuva'ya kopyalayın ve destek adresini Yuva'ya yönlendirin. Yuva Front'u
yalnızca okur. Genel bakış [Yuva'ya taşı](migrate.md) sayfasındadır.

## Başlamadan önce

- Yuva için bir API belirteci oluşturun. Front'ta **Settings → Developers → API tokens** yolunu açın ve
  **Create API token**'a tıklayın. Adını "Yuva" koyun. **Access resources** özelliğini, **Global resources**
  ve **Shared resources** ad alanlarını (**Private resources**'ı yalnızca kişisel gelen kutuları da
  taşınacaksa ekleyin; bunun için her ekip üyesi kendi ayarlarında API erişimine izin vermelidir) ve
  **Read only** iznini seçin: Write, Delete ve Send kapalı kalsın. Oluşturun ve Front'un yalnızca bir kez
  gösterdiği belirteci kopyalayın. Front'un sayfası:
  [Create and revoke API tokens](https://dev.frontapp.com/docs/create-and-revoke-api-tokens).
- Bağlan adımında belirteci yapıştırın ve **Bağlantıyı sına**'ya basın. Belirteci, taşımak istediğiniz her
  gelen kutusunu gören bir yönetici olarak oluşturun: Yuva yalnızca belirtecin gördüğünü okur.

Front bir belirtecin izinlerini yöneticilere gösterir ama bir programın okumasına izin vermez; bu yüzden Yuva
belirtecin salt okunur olduğunu denetleyemez. Onu salt okunur tutan Yuva'nın bağlantısıdır: Front'a yalnızca
okuma istekleri gönderir, o da yalnızca Front'un API adresine; bu yüzden hiçbir konuşma, kişi ya da ayar
değişmez.

## Neler kopyalanır

| Front | Yuva |
|---|---|
| Konuşmalar | Konuşmalar, tüm mesajlarıyla |
| Alınan mesajlar | Gelen mesajlar |
| Ekip üyelerinin gönderdiği mesajlar | Giden mesajlar |
| Yorumlar ve dahili mesajlar | Yalnızca ekibinizin gördüğü notlar |
| Arşivlenmiş ya da çözülmüş | Kapalı |
| Yanıt bekleyen | Beklemede |
| Diğer açık konuşmalar | Açık |
| Etiketler | Etiketler, eşlediğiniz gibi (ya da hiçbiri) |
| Ekip üyeleri | Ekip üyeleri, eşlediğiniz gibi; diğerleri adıyla saklanır |
| Atanan kişi | Atanan kişi, ekip üyesi eşlenmişse |
| Kişiler | Kişiler, adı, e-posta adresi ve telefonuyla |
| Gelen kutuları | Gelen kutuları, her Front gelen kutusu eşlediğiniz gibi bir gelen kutusuna |
| Ekler | Mesajlardaki dosyalar |

Diğer kanalların (SMS, sohbet, WhatsApp) mesajları metinleriyle düz mesaj olarak gelir. Taslaklar ve silinmiş
konuşmalar hiç içe aktarılmaz. Konuşmalar özgün zamanlarını korur ve
[sessizce](migrate.md#geçmiş-nasıl-sessiz-kalır) gelir.

## Neler kopyalanmaz

Kurallar, makrolar, hazır metinler, özel alanlar, bağlantılar, hatırlatıcılar, bir konuşmanın parçası olmayan
ekip içi tartışmalar, analizler ve bir e-postanın özgün kaynağı: Front her mesajı Yuva'ya biçimlendirilmiş
metin olarak verir; bu yüzden başlıklar ve özgün biçim korunmaz. Yuva yalnızca yukarıdaki tabloda yazanı
kopyalar. Talep edeninin e-posta adresi olmayan bir konuşma (örneğin SMS) kanalsız geçmiş olarak saklanır.

## Sınırlar

Front istekleri plana göre şirketin tamamı için dakikada sınırlar (Starter 50, Professional 100, Enterprise
200) ve tüm belirteçler sınırı paylaşır. Yuva Front'un bildirdiği sınırın yarısında kalır, değişiklik
aramalarını Front'un onlara tanıdığı yüzde 40'ın altında tutar ve Front yavaşlamasını istediğinde Front'un
`retry-after` süresi kadar bekler. Bu yüzden büyük bir hesap biraz sürer. Front'un sayfası:
[Rate limiting](https://dev.frontapp.com/docs/rate-limiting).

## Geçiş

Bugün destek adresiniz Front'a ulaşıyor; bunu onu barındıran e-posta sağlayıcısı ya da orada bir kural
yönlendiriyor. Geçiş adımında kanalın [yönlendirme adresi](email.md#yönlendirme-adresi)ni kopyalayın ve bu
yönlendirmeyi destek adresi Yuva'ya ulaşacak şekilde değiştirin. Bunu yapana kadar Yuva iki dakikada bir
Front'tan yeni hareket olan konuşmaları ister, böylece hiçbir konuşma kaybolmaz. Yuva'dan gönderilen yanıtlar
kanalın adresinden çıkar.

Sonra **Deneme e-postası gönder**'e basın: ileti yeni yoldan Yuva'ya ulaştığında çalışıyor demektir. Sessiz
bir dönemden sonra ya da **Şimdi bitir**'e bastığınızda taşıma biter
([nasıl çalışır](migrate.md#deneme-e-postası-sessiz-dönem-ve-bitirme)).

## Bittikten sonra

Yuva için oluşturduğunuz belirteci silin: Front'ta **Settings → Developers → API tokens** yolunda belirteci
açın ve **Delete**'e tıklayın. Yuva taşıma bitince kendi kopyasını siler ama belirteci Front'ta iptal edemez;
bu yüzden erişimini belirteci silmek sonlandırır. Yönlendirmeyi olduğu gibi bırakın; isterseniz Front'u arşiv
olarak tutun.
