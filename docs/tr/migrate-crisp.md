# Crisp'ten taşıyın

Bir Crisp web sitesinin konuşmalarını Yuva'ya kopyalayın. Yuva Crisp'i yalnızca okur. Genel bakış
[Yuva'ya taşı](migrate.md) sayfasındadır.

## Başlamadan önce

- Bir web sitesi tokenı oluşturun. Crisp'te **Settings → Workspace Settings → Advanced configuration → API
  Token** sayfasını açın ve çalışma alanı için bir token oluşturun. Crisp bir **kimlik** ve bir **anahtar** gösterir;
  ikisini de kopyalayın. Crisp'in sayfası: [Authentication](https://docs.crisp.chat/guides/rest-api/authentication/).
  Web sitesi tokenını hangi Crisp planlarının sunduğunu Crisp belirler; sayfa çalışma alanınızda yoksa planınız
  bunu içermiyor olabilir.
- **Web sitesi kimliğini** bulun: Crisp panelinizin adresinde `/website/` kısmından sonra gelen UUID'dir.
- Bağlan adımında **kimliği** API tokenı olarak, **anahtarı** Anahtar olarak ve web sitesi kimliğini Web sitesi olarak girin,
  sonra **Bağlantıyı sına**'ya basın.

Crisp'in bir web sitesi tokenı için salt okunur kapsamı yoktur; token veriyi değiştirebilirdi. Onu salt okunur
tutan Yuva'nın bağlantısıdır: Crisp'e okuma isteklerinden başka bir şey göndermez ve tokenı yalnızca
`api.crisp.chat`'e gönderir. Eklenti tokenları (Marketplace) kullanılmaz.

## Neler kopyalanır

| Crisp | Yuva |
|---|---|
| Web sitesi | Eşlediğiniz bir gelen kutusu |
| Konuşmalar | Konuşmalar, tüm mesajlarıyla |
| Ziyaretçi mesajları | Gelen mesajlar |
| Operatör mesajları | Giden mesajlar |
| Notlar | Yalnızca ekibinizin gördüğü notlar |
| Dosya ve animasyon mesajları | Mesajlardaki dosyalar |
| Durum: pending, unresolved | Açık |
| Durum: resolved | Kapalı |
| Segmentler | Eşlediğiniz etiketler (ya da hiçbiri) |
| Operatörler | Eşlediğiniz ekip üyeleri; diğerleri adıyla saklanır |
| Atanan operatör | Operatör eşlenmişse atanan kişi |
| Konuşmadaki kişi (e-posta, takma ad, telefon) | Bir kişi |

Olaylar (örneğin durum değişikliği), seçiciler ve alanlar mesaj değildir ve kopyalanmaz. Bir konuşmaya yazan ikinci
bir kişi mesajlarında kendi adını korur. Dosyalar tokensız olarak Crisp'in kendi depolama alanından alınır; başka
yerde duran bir dosya ya da animasyon ve insanların mesaja yazdığı bağlantılar hiçbir zaman alınmaz. Konuşmalar
özgün zamanlarını taşır ve [sessizce](migrate.md#geçmiş-nasıl-sessiz-kalır) gelir.

## Neler kopyalanmaz

Konuşmadaki kişinin ötesindeki kişi profilleri, kampanyalar, bilgi bankası, kısayollar, ses mesajları ve raporlar.
Yuva yalnızca yukarıdaki tabloda yazanı kopyalar.

## Sınırlar

Bir web sitesi tokenı günde 10.000 istek yapabilir. Yuva en çok 5.000'ini, konuşma başına yaklaşık ikisini kullanır
ve kullandığında günün kalanında durur; bu yüzden büyük bir web sitesi birkaç gün sürer, kuru çalıştırma kaç gün
süreceğini söyler. Crisp sınır üstbilgisi göndermez: çok fazla istek yanıtı verdiğinde Yuva bir dakika bekler ve
bundan sonra hızını yarıya indirir. Crisp'in sayfası: [Rate limits](https://docs.crisp.chat/guides/rest-api/rate-limits/).

## Geçiş

Web sitenizdeki sohbet kutusunu Yuva'nınkiyle değiştirene kadar Crisp sohbete devam eder. Destek adresinize gelen
e-posta, o adresin yönlendirmesini kanalın [yönlendirme adresine](email.md#yönlendirme-adresi) çevirdiğinizde
Yuva'ya ulaşır. Sohbet kutusu değişene kadar Yuva Crisp'i iki dakikada bir okur, böylece hiçbir konuşma kaybolmaz.

Sonra **Deneme iletisi gönder**'e basın: ileti yeni yoldan Yuva'ya ulaştığında çalışıyordur. Sessiz dönemden sonra
ya da **Şimdi bitir**'e bastığınızda taşıma biter
([nasıl](migrate.md#deneme-e-postası-sessiz-dönem-ve-bitirme)).

## Bittikten sonra

Tokenı yenileyin: Crisp'te **Settings → Workspace Settings → Advanced configuration** sayfasını açın ve yeniden
oluşturun. Yuva taşıma bitince tokenını siler ama Crisp'te iptal edemez; erişimini yeniden oluşturmak bitirir.
İsterseniz Crisp'i arşiv olarak tutun.
