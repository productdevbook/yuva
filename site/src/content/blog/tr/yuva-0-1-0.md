---
title: "Yuva 0.1.0: ekipler, raporlar, tek oturum açma ve beş yeni destek sisteminden aktarım"
description: "Yuva 0.1.0 atama ve yanıt süresi hedefleri olan ekipler, Raporlar, Slack ve sorun takipçisi bağlantıları, Yuva'nın kendisinin gönderdiği push, üyeler için tek oturum açma ve beş destek sisteminden daha geçmiş aktarımı getiriyor. Yükseltirken ilgilenmeniz gereken dört değişiklik de var."
draft: true
author: productdevbook
---

Yuva, çalıştırdığınız her ürün için açık kaynaklı bir gelen kutusudur. E-posta, bir sitedeki canlı sohbet ve iOS ile Android uygulamalarındaki konuşmalar tek yerde toplanır; yanında geri bildirim, yardım merkezi, yol haritası ve durum sayfaları, bültenler ve çerezsiz analitik bulunur. Barındırdığımız hizmeti kullanabilir ya da aynı kodu Postgres ve tek bir konteynerle kendiniz çalıştırabilirsiniz. [Mobil ekranlardan](/blog/yuva-mobile-sdks/) ve otuzdan fazla küçük sürümden sonra [0.1.0](https://github.com/productdevbook/yuva/releases/tag/v0.1.0), bir kilometre taşı olarak numaralandırdığımız ilk sürüm.

Gözü kapalı kurulacak bir sürüm de değil. Başlarken dört şey değişiyor; bunlar [Yükseltmeden önce](#yükseltmeden-önce) bölümünde.

## Yeni olanlar

### Ekip iş akışı

- **Ekipler** gelen kutularına erişir ve atama alır. Bkz. [ekipler](/tr/docs/headless/#ekipler).
- **Otomatik atama**, yeni bir konuşmayı gelen kutusunun ya da ekibin çevrimiçi bir üyesine verir; sırayla ya da en az açık konuşması olana göre, üye başına koyduğunuz sınırı aşmadan. Bkz. [atama](/tr/docs/assignment/).
- **SLA politikaları**, çalışma saatlerine göre ilk yanıt, sonraki yanıt ve çözüm hedefleri koyar. Liste hedefi gösterir, ihlal atanan kişiye bildirilir. Bkz. [SLA](/tr/docs/sla/).
- **Otomasyon kuralları**, bir konuşma eşleşince etiket ekler, atar, yanıtlar ya da webhook çağırır; bir kural kendi yaptığı değişiklikle yeniden tetiklenmez. Bkz. [otomasyon](/tr/docs/automation/).
- **Makrolar** bir yanıtı, etiketleri ve durumu tek adımda, geri alma ile uygular. **Kayıtlı görünümler** her üyeye yalnızca görmeye hakkı olanı gösteren paylaşılan filtrelerdir. **Özel alanlar**, konuşma kapanmadan önce zorunlu kılınabilir. Bkz. [makrolar](/tr/docs/headless/#makrolar), [kayıtlı görünümler](/tr/docs/headless/#kayıtlı-görünümler) ve [özel alanlar](/tr/docs/custom-fields/).
- **Denetim günlüğü**: sahipler erişimi, kimlik bilgilerini ve verileri kimin değiştirdiğini okur; günlükte ileti içeriği ya da gizli anahtar yoktur. Bkz. [denetim günlüğü](/tr/docs/operations/#denetim-günlüğü).
- **Çöp kutusu ve silme**: bir konuşmayı silmek onu geri alınabilir, 30 günlük bir çöp kutusuna taşır; tek bir ileti, eki ile birlikte silinip "kaldırıldı" olarak gösterilebilir. Bkz. [konuşmaları silme](/tr/docs/headless/#konuşmaları-silme).

### E-posta

- Yanıtlarda **Cc ve Bcc**. Cc'deki kişiler konuşmada kalır; DMARC'ı geçen yanıtları konuşmaya eklenir.
- **Konuşmayı yönlendirme**: konuşmayı dışarıdaki bir adrese yönlendirirsiniz, gelen yanıt yalnızca ekibinizin göreceği biçimde konuşmaya döner.
- **Adres kanıtı**: bir e-posta kanalı kendi adresine gelen postayı ancak çalışma alanı adresi kanıtladıktan sonra alır (aşağıya bakın).

Ayrıntılar [E-posta](/tr/docs/email/) sayfasında.

### Raporlar

Raporlar; konuşmaları, yanıtları, ilk yanıt ve çözüm sürelerini, SLA sonuçlarını ve puanlamaları gün, hafta ya da ay bazında, gelen kutusu, kanal, etiket, ekip veya üyeye göre döker ve CSV olarak indirmenizi sağlar. Bir temsilci yalnızca kendi gelen kutularını görür. Aynı rakamlar API'de ve MCP sunucusunda da var. Bkz. [Raporlar](/tr/docs/reports/).

### Entegrasyonlar

- **Slack**: yeni konuşmalar bir kanala düşer, ileti dizisine yazılan yanıt bağlı üyeden bir not olur. Bkz. [Slack](/tr/docs/slack/).
- **GitHub ve Linear**: bir geri bildirimi bir sorunla bağlarsınız, sorunun durumu geri bildirimin ilerlemesini belirler. Bkz. [sorun takipçileri](/tr/docs/issue-trackers/).
- **Yuva'nın gönderdiği push**: bir uygulama kanalında APNs ve FCM anahtarları varsa yanıt push'unu webhook olmadan Yuva kendisi gönderir. Bkz. [mobil](/tr/docs/mobile/#yuvanın-gönderdiği-push).
- Yardım merkezleri, yol haritası panoları ve Yenilikler sayfaları kendi alan adında yayın yapabilir; yardım makaleleri Zendesk Guide ve Help Scout'tan içe aktarılır. Bkz. [yardım merkezi](/tr/docs/help-center/#zendesk-veya-help-scouttan-içe-aktarma).

### Başka bir destek sisteminden geçiş

Geçmiş aktarımı artık [Front](/tr/docs/migrate-front/), [Freshdesk](/tr/docs/migrate-freshdesk/), [Intercom](/tr/docs/migrate-intercom/), [Crisp](/tr/docs/migrate-crisp/) ve [Chatwoot](/tr/docs/migrate-chatwoot/) için de var; Zendesk, Help Scout ve IMAP'li her posta kutusunun yanında. Yuva eski sistemi yalnızca okur. Geçmiş özgün tarihleriyle, okunmuş olarak gelir; bildirim, webhook ya da push göndermez. Bkz. [Yuva'ya geçiş](/tr/docs/migrate/).

### Oturum açma ve güvenlik

- Üyeler için **tek oturum açma**: OpenID Connect ya da SAML 2.0, çalışma alanının sahiplendiği e-posta alan adları için. İlk girişte üye oluşturulabilir, sahip tek oturum açmayı zorunlu kılabilir. Bkz. [tek oturum açma](/tr/docs/sso/).
- Hız sınırları Postgres'te sayılır, bu yüzden birden çok sunucu işlemi aynı sınırı paylaşır. Ek boyutu sınırları kanal başına ayarlanır. Sunucu, bir toplayıcıya yönlendirirseniz OpenTelemetry izleri gönderir.

## 0.1 ne demek

Tüm depo için tek bir sürüm var. Sunucu, API sözleşmesi, JavaScript, Swift, Kotlin ve Go kitaplıkları, Helm şeması ve MCP bildirimleri `0.1.0` bildirir. URL'lerdeki `/v1` ve `/client/v1` sözleşmenin sürümüdür ve olduğu gibi kalır.

Yuva hâlâ beta. 1.0'a kadar API ve veritabanı şeması bir sürümde değişebilir. Bundan sonra yükseltme [işletim kılavuzunu](/tr/docs/operations/#güncelleme) izler: aradaki her sürümün sürüm notlarını okuyun, yedek alın, imaj etiketini değiştirin, başlatın. Veritabanı geçişleri başlangıçta çalışır. Bundan fazlasını gerektiren her şey, 0.1.0'da olduğu gibi, sürümden önce kılavuzda yazılır.

## Yükseltmeden önce

Tam adımlar [0.1.0 sürümüne yükseltme](/tr/docs/operations/#010-sürümüne-yükseltme) bölümünde. Özetle:

1. **Her e-posta adresini kanıtlayın.** Yükseltmeden sonra her e-posta kanalı kanıtlanmamıştır. Kanıtlanmamış kanal kendi adresine gelen postayı yönlendirmeyi bırakır; yönlendirme adresine gelen posta ve gönderim çalışmaya devam eder, her şeyi yakalayan (catch-all) kanal hiçbir şey almaz. Her çalışma alanı bir kanalı DNS TXT kaydıyla, adrese postalanan bir kodla kanıtlar ya da siz kefil olursunuz:

   ```sh
   docker compose exec yuva /yuva channel prove --all-unproven --workspace <ws>
   ```

   `--yes` olmadan komut yalnızca neyi kanıtlayacağını listeler. Yazdırdığı kodla `--yes --plan <kod>` ekleyerek yeniden çalıştırın.

2. **Email Worker'ı yeniden dağıtın.** `YUVA_INGRESS_ACCEPT_V2` artık kapalı; sunucu yalnızca `v3` imzalarını kabul eder. `edge/` içindeki Worker'ı yükseltmeden önce yeniden dağıtın ya da bunu yapana kadar `YUVA_INGRESS_ACCEPT_V2=true` verin.

3. **`YUVA_INGRESS_AUTHSERV_ID` ayarlayın.** Karşılama, kural yanıtları ve puanlama istekleri artık yalnızca postası DMARC'ı geçen gönderenlere e-postalanır. Ayar yoksa hiçbiri gönderilmez ve sunucu başlangıçta uyarı verir.

4. **SMTP'si olmayan sunucu postayı günlüğe yazar.** `YUVA_SMTP_HOST` ayarlı değilse sunucu postayı atmak yerine, gizlenmiş olarak günlüğe yazar. Oturum açma kodu, adresi `*@alanadi` olarak kısaltılmış tek bir satır olarak görünür.

Bilmeniz gereken iki değişiklik daha var. Arka plan işleri artık daha çok kuyrukta çalışıyor; yavaş bir müşteri uç noktası atamayı ya da bildirimleri geciktirmez, bir şey ayarlamanız gerekmez. Ayrıca tek oturum açma oturumları daraldı: bir çalışma alanının sağlayıcısıyla açılan oturum yalnızca o çalışma alanında ve yalnızca okuyucu olarak çalışır; başka yerde bu şekilde giriş yapmış biri kodla ya da geçiş anahtarıyla yeniden giriş yapar. "Tek oturum açmayı zorunla" açıldığında kapsanan üyeler, sağlayıcınız üzerinden açmadıkları her oturumdan, bütün çalışma alanlarında çıkarılır. Sahipler bunun dışındadır.

## Kurulum

- **Barındırılan hizmet:** [ana sayfadan](/tr/) hesap açın.
- **Kendi sunucunuzda:** [kurulum kılavuzu](/tr/docs/install/) Yuva'yı ve Postgres'i Docker Compose ile birkaç dakikada başlatır; Kubernetes için Helm da anlatılır. İmaj `ghcr.io/productdevbook/yuva:0.1.0`.

## Denemediklerimiz

Yukarıdaki özellikleri çalışan bir sunucuda, kaydedilmiş ya da sahte sağlayıcılarla denedik: posta için Mailpit, tek oturum açma için Keycloak, sahte Slack, GitHub, Linear, APNs ve FCM uç noktaları ve aktarımlar için kaydedilmiş yanıtlar. Gerçek DNS, bir posta sağlayıcısındaki gerçek DMARC sonuçları, gerçek bir Slack uygulaması, gerçek telefonlara push ve gerçek destek sistemi hesaplarından aktarım bunun dışında. Denerseniz neyin bozulduğunu bir [issue](https://github.com/productdevbook/yuva/issues) ile bildirin.
