# Chatwoot'tan taşıyın

Bir Chatwoot hesabının konuşmalarını Yuva'ya kopyalayın ve destek adresinizi Yuva'ya yönlendirin. Yuva
Chatwoot'u yalnızca okur. Genel bakış [Yuva'ya taşıyın](migrate.md) sayfasındadır. Chatwoot Cloud ile ve
kendi sunucunuzda çalışan Chatwoot ile çalışır.

## Başlamadan önce

- Bir yönetici kullanın. Chatwoot'un API'si bir temsilciye yalnızca üyesi olduğu gelen kutularını
  gösterir; bu yüzden Yuva'nın hesabın **yöneticisi** olan bir kullanıcının erişim jetonuna ihtiyacı var.
  Yuva bir temsilcinin jetonunu reddeder.
- Erişim jetonunu kopyalayın. Chatwoot'ta **Profile Settings**'i (sol altta avatarınız) açın, **Access
  Token** bölümüne inin ve jetonu kopyalayın. Chatwoot'un sayfası:
  [API](https://developers.chatwoot.com/api-reference/introduction).
- Hesap numarasını bulun. Chatwoot'tayken adres çubuğunda `/app/accounts/` sonrasındaki sayıdır; örneğin
  `https://app.chatwoot.com/app/accounts/1/dashboard` adresinde `1`.
- Bağlan adımında Chatwoot'unuzun **adresini** (`https://app.chatwoot.com` ya da kendi kurulumunuzun HTTPS
  adresi), **hesap numarasını** ve **erişim jetonunu** girin ve **Bağlantıyı dene**'ye basın. Kendi
  sunucunuzun adresine internetten ulaşılabilmelidir; özel bir adresteki sunucu, Yuva'yı işleten izin
  vermediği sürece reddedilir.

Chatwoot'un salt okunur jetonu yoktur, bu yüzden jeton veriyi değiştirebilirdi. Yuva'nın bağlantısı onu
salt okunur tutar: Chatwoot'a okuma istekleri dışında hiçbir şey göndermez ve jetonu yalnızca girdiğiniz
adrese gönderir.

## Nelerin kopyalandığı

| Chatwoot | Yuva |
|---|---|
| Konuşmalar | Konuşmalar, tüm mesajlarıyla |
| Gelen mesajlar | Gelen mesajlar |
| Giden mesajlar | Giden mesajlar |
| Özel notlar | Yalnızca ekibinizin gördüğü notlar |
| Etkinlik satırları (atandı, çözüldü, etiket eklendi) | Hiçbir şey |
| Durum: open | Açık |
| Durum: pending ve snoozed | Beklemede |
| Durum: resolved | Kapalı |
| Konuşma etiketleri | Etiketler, eşlediğiniz gibi (ya da hiç) |
| Temsilciler | Ekip üyeleri, eşlediğiniz gibi; diğerleri adıyla saklanır |
| Atanan kişi | Atanan kişi, temsilci eşlenmişse |
| Kişiler | Kişiler; ad, e-posta adresi ve telefonla |
| Gelen kutuları | Gelen kutuları, eşlediğiniz gibi her Chatwoot kutusu için bir tane |
| Ekler | Mesajlardaki dosyalar |

Konuşmalar özgün zamanlarını korur ve [sessizce](migrate.md#geçmiş-nasıl-sessiz-kalır) gelir.

## Nelerin kopyalanmadığı

Hazır yanıtlar, otomasyon kuralları, makrolar, ekipler, özel nitelikler, memnuniyet puanları, Yardım
Merkezi ve raporlar. Yuva yalnızca yukarıdaki tablodakileri kopyalar.

## Ekler

Chatwoot her eke kendi depolamasında (kurulumunuzda ya da arkasındaki depolama hizmetinde) bir bağlantı
verir. Yuva dosyayı bu bağlantıdan jeton olmadan alır. Sunucunun ek sınırından büyük bir dosya geride
kalır.

## Sınırlar

Chatwoot bir sınır bildirmez. İstekleri IP adresi başına kısar; bu değeri bir kurulumu işleten belirler,
Chatwoot Cloud ise yayımlamaz. Yuva dakikada en fazla 60 istek gönderir; Chatwoot `Retry-After` başlığıyla
`429 Too Many Requests` yanıtlarsa o kadar, başlıksız yanıtlarsa bir dakika bekler. Bu yüzden büyük bir
hesap zaman alır: her konuşma mesajları için en az bir istek, her yirmi mesaj için bir istek daha ister.

## Geçiş anı

Bugün destek adresiniz Chatwoot'a ulaşıyor: oradaki bir e-posta gelen kutusu üzerinden ya da onu barındıran
posta sağlayıcısındaki bir yönlendirme kuralıyla. Geçiş adımında kanalın
[yönlendirme adresi](email.md#yönlendirme-adresi)ni kopyalayın ve destek adresi artık Yuva'ya ulaşsın diye
yönlendirmeyi değiştirin. Bunu yapana dek Yuva Chatwoot'u iki dakikada bir okur, böylece hiçbir konuşma
kaybolmaz. Yuva'dan gönderdiğiniz yanıtlar kanalın adresinden çıkar.

Sonra **Deneme e-postası gönder**'e basın: ileti yeni yoldan Yuva'ya ulaştığında çalışıyor demektir. Sessiz
bir dönemden sonra ya da **Şimdi bitir**'e bastığınızda taşıma biter
([nasıl çalışır](migrate.md#deneme-e-postası-sessiz-dönem-ve-bitirme)).

## Bittikten sonra

Erişim jetonunuzu sıfırlayın: Chatwoot'ta **Profile Settings → Access Token → Reset**. Yuva taşıma
bitince kendi kopyasını siler ama Chatwoot'ta iptal edemez; erişimini sıfırlamanız bitirir. Yönlendirmeyi
olduğu gibi bırakın; isterseniz Chatwoot'u arşiv olarak tutun.
