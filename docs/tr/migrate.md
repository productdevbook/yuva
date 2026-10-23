# Yuva'ya taşı

Bugün yanıt verdiğiniz posta kutusunun geçmişini bir Yuva gelen kutusuna getirin, sonra e-postalarını
Yuva'ya yönlendirin. Yuva eski posta kutusunu yalnızca okur: içindeki hiçbir şey değiştirilmez ya da
silinmez, ekibiniz bu sırada çalışmaya devam eder.

Posta kutusunu sahipler ve yöneticiler taşıyabilir: **Ayarlar → Yuva'ya taşı → Yeni taşıma**. Aynı adımlar
[asistanlar](#claude-code-ya-da-başka-bir-asistanla) ve [API](#arka-uçtan) için de geçerlidir.
Sağlayıcınızın rehberini seçin: [Gmail](migrate-gmail.md), [Google Workspace](migrate-google-workspace.md),
[Outlook.com](migrate-outlook.md), [Microsoft 365](migrate-microsoft365.md), [Yandex](migrate-yandex.md), [iCloud](migrate-icloud.md), [Zoho Mail](migrate-zoho.md),
[Yahoo Mail](migrate-yahoo.md), [Fastmail](migrate-fastmail.md), [GMX ve mail.com](migrate-gmx.md),
[Zendesk](migrate-zendesk.md), [Help Scout](migrate-helpscout.md), [Chatwoot](migrate-chatwoot.md), [Front](migrate-front.md), [Freshdesk](migrate-freshdesk.md), [Intercom](migrate-intercom.md),
[herhangi bir IMAP sunucusu](migrate-imap.md) ya da [bir dışa aktarım dosyası](migrate-files.md) (Google Takeout, mbox, zip içinde .eml veya Maildir). Kendi sunucunuzu mu işletiyorsunuz? [İşletmeciler için Yuva'ya taşıma](migrate-operators.md) sayfasına bakın.
Zendesk, Help Scout, Outlook.com ve Microsoft 365 şifreyle değil, oturum açarak, Front ise bir API belirteciyle, Freshdesk bir API anahtarı ve alt alan adınızla bağlanır; Intercom erişim belirteciyle.

## Yuva posta kutunuzla neler yapar, neler yapmaz

Yuva eski posta kutunuzda hiçbir şeyi silmez, taşımaz, arşivlemez ya da okundu olarak işaretlemez, orada
hiçbir ayarı değiştirmez. Bu üç kez güvence altına alınmıştır; böylece tek bir hata e-postalarınızı
değiştiremez:

- Yuva posta kutusuna yalnızca klasörleri salt okunur açan (`EXAMINE`) ve mesajları işaretlemeden getiren
  (`BODY.PEEK`) kodla ulaşır.
- Bu kodun altında her komut sunucudan çıkmadan önce denetlenir. Yalnızca `CAPABILITY`, `NOOP`, `LOGOUT`,
  `STARTTLS`, `LOGIN`, `AUTHENTICATE`, `LIST`, `STATUS`, `EXAMINE`, `UNSELECT`, `SEARCH` ve mesajı
  işaretlemeyen öğelerin `FETCH` komutuna izin verilir. Başka her şey gönderilmeden bağlantıyı kapatır.
- Her değişiklikte çalışan testler, yazan bir komut gönderilebilirse derlemeyi başarısız kılar; ayrıca bir
  deneme posta kutusunun taşımadan sonra bayt bayt aynı olduğu doğrulanır.

Bir uygulama şifresiyle e-postalar değiştirilebilir; Yuva'nın bağlantısı bunu yapan her komutu reddeder.
Taşıma sayfasında "{sağlayıcı} içinde yapılan değişiklik: yok" yazar (sağlayıcının adıyla).

## Neler kopyalanır

| Kopyalanır | Kopyalanmaz |
|---|---|
| Seçtiğiniz klasörlerdeki mesajlar, iki yönde de, özgün zamanlarıyla | Spam, çöp kutusu ve taslaklar: hiç okunmaz |
| Ekler (sunucunun boyut sınırını aşanlar ya da saklamadığı türde olanlar özgün iletide kalır) | Sunucunun tek mesaj sınırından büyük mesajlar ([`YUVA_MIGRATION_MAX_MESSAGE_BYTES`](configuration.md#yuvaya-taşıma), 50 MiB) |
| Yazışmalar konuşma olarak, `References` ve `In-Reply-To` ile birleştirilir, konuya göre asla | Yalnızca otomatik e-postalardan oluşan yazışmalar (otomatik yanıtlar, bültenler, teslim raporları); seçeneği kaldırmazsanız |
| Müşteriler kişi olarak; ekibinizin kendi e-postaları giden mesaj olarak | Yalnızca ekibinizin e-postalarından oluşan yazışmalar |
| Klasörler ve etiketler, eşleştirdiğiniz gibi Yuva etiketi olarak | Seçtiğiniz tarih aralığının dışındaki mesajlar |
| "Özgün mesajları sakla" işaretliyse her mesajın özgün kaynağı (daha fazla depolama kullanır) | |

Son mesajı seçtiğiniz gün sayısından (varsayılan 14) eski olan konuşmalar kapatılır; daha yeni olanlar,
son yazan müşteriyse açık, ekibinizse beklemede kalır.

## Geçmiş nasıl sessiz kalır

Aktarılan konuşmalar özgün zamanlarını korur, gelen kutusunda **Aktarıldı** ile işaretlenir ve son aktarılan
mesajdan sonra bir ayraç görünür. Ekip adreslerinizden gelen e-postalar, eşleştirdiğiniz üyenin ya da üye
yoksa özgün adın ve adresin gönderdiği olarak görünür.

- Aktarılan e-postalar için bildirim, e-posta, push, webhook, otomatik yanıt ya da puanlama isteği gönderilmez.
- Aktarılan hiçbir şey okunmamış sayılmaz.
- Aktarılan e-postalar istatistiklerin (yanıtlar, ilk yanıtlar, kapatmalar) dışında kalır; faturalanan
  sayaçlar yerine kullanımın aktarım sütunlarında sayılır.
- Çalışma alanının [saklama süresi](operations.md#veri-saklama) aktarım gününden başlar.
- Kopyalama sürerken gelen kutusunda bir şerit görünür; gelen kutusunu gören herkes onu görür.

## Adımlar

1. **Kaynak.** Posta kutusunun adresini girin. Yuva sağlayıcısını adresten ve alan adının e-posta
   sunucularından bulur; henüz hiçbir şey okunmaz. Ya da sağlayıcıyı kendiniz seçin.
2. **Bağla.** Kullanıcı adını ve bir uygulama şifresini girin (sağlayıcınızın rehberi alanın yanındadır) ve
   "Bağlantıyı dene"ye basın. Deneme hangi adımın başarısız olduğunu ve nasıl düzeltileceğini gösterir.
   Zendesk, Help Scout, Outlook.com ve Microsoft 365 bunun yerine oturum açma kullanır.
3. **Seç.** Klasörleri ve ne kadar geriye gidileceğini seçin. Yuva yalnızca başlıkları tarar; böylece neyin
   aktarılacağını tam olarak söyleyebilir.
4. **Eşleştir.** Adres için gelen kutusunu ve e-posta kanalını (Yuva bunları oluşturmayı önerir), her ekip
   adresinin hangi üyeye ait olduğunu, klasörlerin nasıl etikete dönüşeceğini, eski konuşmaların ne zaman
   kapatılacağını ve otomatik e-postaların atlanıp atlanmayacağını ya da özgün mesajların saklanıp
   saklanmayacağını seçin.
5. **Gözden geçir.** Deneme sonucu klasör başına sayıları (toplam, seçilen klasörlerdeki tüm iletilerdir; yinelenenler ve atlananlar sayılır), nelerin neden atlandığını, kişileri, etiketleri,
   depolamayı ve süreyi listeler, en yeni konuşmaların nasıl görüneceğini gösterir. "Taşımayı başlat"a
   basana kadar hiçbir şey aktarılmaz. Buradaki bir seçimi değiştirmek kopyalamayı yeniden planlar.
6. **Kopyala.** Önce en yeni konuşmalar, canlı ilerlemeyle. Sayfayı kapatabilir, duraklatabilir, sürdürebilir
   ya da iptal edebilirsiniz; duran bir kopyalama (iptal edilen uygulama şifresi, sağlayıcının sınırı)
   düzeltilince kaldığı yerden devam eder; "Başarısız olanları yeniden dene" başarısız mesajları yeniden dener.
7. **Geçiş.** Geçmiş aktarıldı; siz e-postalarını Yuva'ya yönlendirirken hiçbir şey kaybolmasın diye Yuva eski
   posta kutusunu iki dakikada bir okumaya devam eder (aşağıda).
8. **Bitti.** Taşıma tamamlanır, erişim bilgileri silinir ve sayfa uygulama şifresini nereden kaldıracağınızı
   söyler.

## E-postaları yönlendirme

Kopyalama bitmeden önce eski posta kutusuna gelen e-postalar geçmiştir. Sonra gelenler yeni e-postadır:
doğrudan Yuva'ya ulaşmış gibi bildirimlerle görünür, ama gelen kutusunun karşılama yanıtı olmadan. İki yoldan
da Yuva'ya ulaşan e-posta bir kez saklanır.

Posta kutusunun e-postalarını Yuva'ya üç yoldan biriyle gönderin:

| Yol | Kim | Eski posta kutusu kopya tutar |
|---|---|---|
| Posta kutusunu kanalın [yönlendirme adresine](email.md#yönlendirme-adresi) yönlendirmek | posta kutusunun sahibi | evet |
| Aynı adrese sunucuda, yönetici olarak yönlendirmek | Google Workspace ya da e-posta sunucusu yöneticisi | evet |
| Yalnızca destek için kullanılan bir alan adının ya da alt alan adının MX kayıtlarını değiştirmek ([e-posta rehberi](email.md)) | DNS'i yöneten kişi | hayır |

Yönlendirme alan adı ([`YUVA_FORWARDING_DOMAIN`](configuration.md#yuvaya-taşıma)) olmayan bir sunucuda
yönlendirme adresi yoktur ve yalnızca MX yolu sunulur. Her sağlayıcı rehberi yönlendirme ayarlarını gösterir.
Google yönlendirme adresine bir onay gönderirse kod ve bağlantı bu adımda görünür.

## Deneme e-postası, sessiz dönem ve bitirme

"Deneme e-postası gönder"e basın. Yuva posta kutusuna kısa bir ileti gönderir; yeni yoldan Yuva'ya ulaşırsa
geçiş çalışıyor demektir ve Yuva bunun yönlendirmeyle mi doğrudan mı geldiğini söyler. Birkaç dakika sonra
yalnızca eski posta kutusunda bulunursa e-postalar hâlâ eski sağlayıcıya ulaşıyor ama henüz yönlendirilmiyor
demektir.

Bir deneme e-postası ulaştıktan sonra Yuva eski posta kutusunu bir sessiz dönem boyunca (varsayılan 24 saat)
okumaya devam eder. Bu sürede yalnızca orada ortaya çıkan bir şey olmazsa taşıma kendiliğinden biter.
"Şimdi bitir" ile daha erken, "Kontrol etmeden bitir" ile deneme e-postası olmadan bitirebilirsiniz; bundan
sonra yalnızca eski posta kutusuna ulaşan e-postalar kopyalanmaz.
[`YUVA_MIGRATION_SYNC_MAX_DAYS`](configuration.md#yuvaya-taşıma) (30 gün) sonunda bitirilmemiş bir taşıma
kendiliğinden durur.

Bitirdikten sonra yönlendirmeyi ya da MX kayıtlarını olduğu gibi bırakın: e-postalar artık Yuva'ya bu yolla
ulaşır. Eski posta kutusuna dokunulmadı; dilerseniz arşiv olarak saklayın.

## Şifre

Yuva şifreyi girdiğiniz anda sunucunun ana anahtarıyla mühürler. Bir daha gösterilmez; günlüklerde,
olaylarda, API yanıtlarında ve asistan araçlarında asla görünmez. Yuva onu taşıma tamamlandığında ya da
iptal edildiğinde, eşitleme sınırı dolduğunda, siz değiştirdiğinizde ve çalışma alanıyla birlikte siler.
Sayfa o zaman "Erişim bilgileri silindi" der. Yuva sağlayıcınızdaki bir uygulama şifresini iptal edemez:
Yuva için oluşturduğunuzu orada kaldırın (her sağlayıcı rehberi sayfaya bağlanır).

## Aktarılan geçmişi kaldırma

Ayarlar → Yuva'ya taşı → taşıma → "Aktarılan geçmişi kaldır"; taşıma tamamlandıktan ya da iptal edildikten
sonra. Yuva, o zamandan beri kimsenin yazmadığı aktarılmış konuşmaları mesajları ve dosyalarıyla, sonra
taşımanın oluşturduğu ve konuşmasız kalan kişileri siler. Ekibinizin yazdığı konuşmalar ve etiketler kalır.
Önce sayıyı onaylarsınız. Eski posta kutusunda hiçbir şey değişmez.

## Claude Code ya da başka bir asistanla

Asistanı `migrations:manage` yetkisiyle [Yuva'nın MCP sunucusuna](mcp.md) bağlayın ve `move_to_yuva`
prompt'unu posta kutusunun adresiyle (isteğe bağlı olarak bir gelen kutusuyla) çalıştırın. Asistan
sağlayıcıyı bulur, taşımayı oluşturur, tarar, deneme sonucunu ve en yeni konuşmaları gösterir ve kopyalamayı
yalnızca siz sayıları onayladıktan sonra başlatır. E-postaların nasıl yönlendirileceğini anlatır, deneme
e-postasını gönderir ve taşımayı bitirir.

Asistan şifreyi asla sormaz. Size bir Yuva sayfası verir; uygulama şifresini orada girersiniz ve hiçbir araç
şifre almaz. Asistanlar şifreyi ya da girileni göremez.

Bir gelen kutusu belirtip kanal belirtmezseniz asistan, adres için gelen kutusunun e-posta kanalını kullanır
ya da yeni bir kanal oluşturur. Yeni kanalın SMTP hesabı yoktur: Yuva bu kanaldan yanıt gönderebilmeden önce
asistan size panelde kanalın ayarlarında nelerin girileceğini (posta kutusunun SMTP sunucusu, kullanıcı adı ve
şifresi) söyler.

## Verileriniz

- Aktarılan e-postalar, her konuşma gibi çalışma alanınızın verisidir. Çalışma alanının veritabanında ve
  depolamasında durur; kişi dışa aktarımı ve silme, saklama süresi (aktarımdan başlayarak) ve çalışma alanı
  silme bunları kapsar.
- Yuva yalnızca bağladığınız posta kutusuyla, sizin erişim bilgilerinizle ve yalnızca okumak için
  iletişim kurar. Sağlayıcı tespiti alan adının DNS kayıtlarına ve kendi e-posta ayar dosyasına bakar;
  başka kimseye bir şey gönderilmez.
- Neyin aktarılacağına (klasörler, tarihler) siz karar verirsiniz ve içindeki kişisel verilerin sorumlusu
  sizsiniz. Barındıran işletmeci bunları çalışma alanınız adına, yalnızca istediğiniz aktarımı sağlamak için
  işler: reklam ya da model eğitimi için asla, güvenlik, isteğiniz üzerine destek ya da yasa dışında kimse
  tarafından okunmaz.
- Sahipler ve yöneticiler her işlemi taşımanın etkinliğinde görür; etkinlik taşıma bittikten sonra
  [`YUVA_MIGRATION_AUDIT_DAYS`](configuration.md#yuvaya-taşıma) (365) gün saklanır.

## Arka uçtan

Sahipler, yöneticiler ve `migrations:manage` yetkili, gelen kutusu sınırı olmayan API anahtarları (bu yetki
asistan hesaplarına verilmez; onlar `403` alır). Tam şekiller [API sözleşmesindedir](../../openapi/openapi.yaml).

| Uç nokta | Kullanım |
|---|---|
| `GET /v1/migrations/presets` | Bu sunucunun sunduğu sağlayıcılar, sunucuları ve klasörleri. |
| `POST /v1/migrations/detect` | Bir adresin sağlayıcısını bulur. |
| `GET`, `POST /v1/migrations` | Taşımaları listeler; birini başlatır (`draft`). |
| `GET`, `PATCH`, `DELETE /v1/migrations/{migrationId}` | Okur; posta kutusunu, gelen kutusunu, kanalı ve seçenekleri değiştirir; siler. |
| `PUT /v1/migrations/{migrationId}/credentials` | Posta kutusunu kullanıcı adı ve şifreyle bağlar. |
| `GET /v1/migrations/{migrationId}/upload` | Sunucudaki [dışa aktarım dosyası](migrate-files.md) parçaları; yarıda kalan yüklemeye devam etmek için. |
| `POST /v1/migrations/{migrationId}/upload/parts/{part}` | Dışa aktarım dosyasının bir parçasını (en çok 8 MiB) SHA-256 değeriyle yükler. |
| `POST /v1/migrations/{migrationId}/upload/complete` | Yüklemeyi bitirir; Yuva dosyayı okur ve taşıma `connected` olur. |
| `POST /v1/migrations/{migrationId}/test` | Bağlantıyı dener. |
| `GET /v1/migrations/{migrationId}/folders` | Posta kutusunun klasörleri. |
| `POST /v1/migrations/{migrationId}/scan` | Deneme sonucu için başlıkları okur. |
| `GET /v1/migrations/{migrationId}/preview` | En yeni konuşmalar nasıl görünecek. |
| `POST /v1/migrations/{migrationId}/start` | Kopyalamayı başlatır. |
| `POST /v1/migrations/{migrationId}/pause`, `resume`, `cancel` | Duraklatır, sürdürür ya da iptal eder. |
| `POST /v1/migrations/{migrationId}/retry` | Başarısız mesajları yeniden dener. |
| `GET /v1/migrations/{migrationId}/errors` | Neler ters gitti. |
| `POST /v1/migrations/{migrationId}/probe` | Deneme e-postasını gönderir. |
| `POST /v1/migrations/{migrationId}/finish` | Bitirir (`force` kontrolü atlar). |
| `POST /v1/migrations/{migrationId}/remove-history` | Aktarılan geçmişi kaldırır (`confirm_count`). |
| `POST /v1/channels/{channelId}/forwarding-address/rotate` | Bir e-posta kanalına yeni [yönlendirme adresi](email.md#yönlendirme-adresi) verir. |

Durumlar: `draft`, `connected`, `scanning`, `ready`, `copying`, `paused`, `needs_attention`, `syncing`,
`completed`, `cancelled`, `removing`, `removed`; her taşıma şu anda izin verilen `actions` değerlerini
listeler. Ele alınmaya değer hatalar: `409 migration_state`, `409 provider_not_configured`,
`409 confirm_count_mismatch`, `422 hosted_limit` (ayar `limit` içinde adlandırılır) ve deneme e-postası ile
bağlantı denemeleri için `429`. İlerleme sahiplere ve yöneticilere `/v1/realtime` üzerinden
`migration.updated`, `migration.progress` ve `migration.forwarding_confirmation` olarak ulaşır; taşımalar
hiç webhook göndermez.
