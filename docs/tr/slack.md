# Slack

Yuva bir gelen kutusunun yeni konuşmalarını ve üyeleri anan notları Slack kanallarına
gönderebilir. Üyeler gönderinin başlığında (thread) yanıt verir: yanıt konuşmaya not olarak
eklenir, `!reply` ile başlayan yanıt ise müşteriye gider.

## Slack uygulamasını oluşturun

Bunu sunucunun işletmecisi bir kez yapar; sunucudaki her çalışma alanı aynı uygulamayla bağlanır.

1. [api.slack.com/apps](https://api.slack.com/apps) adresinde **Create New App** › **From a
   manifest** seçin, uygulamayı geliştireceğiniz Slack çalışma alanını seçin ve
   `support.example.com` yerine sunucunuzun adresini yazarak şu manifesti yapıştırın:

   ```yaml
   display_information:
     name: Yuva
   features:
     bot_user:
       display_name: Yuva
       always_online: true
   oauth_config:
     redirect_urls:
       - https://support.example.com/slack/oauth/callback
     scopes:
       bot:
         - chat:write
         - channels:read
         - groups:read
         - channels:history
         - groups:history
         - im:write
         - users:read
         - users:read.email
   settings:
     event_subscriptions:
       request_url: https://support.example.com/ingress/slack
       bot_events:
         - message.channels
         - message.groups
         - app_uninstalled
         - tokens_revoked
     org_deploy_enabled: false
     socket_mode_enabled: false
     token_rotation_enabled: false
   ```

2. **Basic Information** altında Client ID, Client Secret ve Signing Secret değerlerini sunucunun
   yapılandırmasına kopyalayın ve sunucuyu yeniden başlatın:

   ```sh
   YUVA_SLACK_CLIENT_ID=1234567890.1234567890
   YUVA_SLACK_CLIENT_SECRET=...
   YUVA_SLACK_SIGNING_SECRET=...
   ```

3. Slack istek adresini, sunucu orada yanıt verdiğinde bir kez denetler; adres sunucu imzalama
   sırrını almadan önce kaydedildiyse uygulamanın **Event Subscriptions** sayfasında denetimi
   yineleyin.
4. Uygulamayı geliştirdiğiniz çalışma alanı dışındaki çalışma alanlarının da bağlanabilmesi için
   **Manage Distribution** › **Public Distribution** seçeneğini açın.

Kapsamlar ne için: `chat:write` gönderir ve yanıtlar, `channels:read` ve `groups:read` botun
bulunduğu kanalları listeler, `channels:history` ve `groups:history` başlıklardaki yanıtları
iletir, `im:write` bağlama kodunu gönderir, `users:read` ve `users:read.email` bir üyenin Slack
kullanıcısını e-posta adresinden bulur. Yuva kanal geçmişi okumaz; yalnızca Slack'in olay olarak
gönderdiği mesajları alır ve yalnızca kendi gönderdiği başlıklardaki yanıtlara göre davranır.

Üç değişken atanmamışsa Slack kapalıdır: panel onu sunmaz ve API `409 slack_not_configured` döner.

## Bir çalışma alanını bağlayın

Bir sahip ya da yönetici Ayarlar › Slack'i açar, **Connect Slack** seçer ve uygulamaya Slack'te
izin verir. Bir Slack çalışma alanı aynı anda tek bir Yuva çalışma alanına bağlıdır. Bot jetonu
sunucunun ana anahtarıyla şifrelenmiş olarak saklanır.

Botu, gönderi yapacağı her kanala davet edin: o kanalda `/invite @Yuva`. Özel kanallar da aynı
şekilde çalışır.

## Gelen kutusu başına kanalları seçin

Bir gelen kutusunun ayarlarında şunları seçin:

- **yeni konuşmalar** için bir kanal (bir kişinin ilk mesajı) ve
- **anmalar** için bir kanal (üyeleri anan bir not; bağlı bir üye Slack'te de anılır) ve
- **SLA** için bir kanal (bir konuşma SLA hedefini aşar; hedef uyarıya ulaştığında da göndermek
  için **SLA warnings** seçeneğini açın). Gönderi hedefi (ilk yanıt, sonraki yanıt ya da çözüm),
  ne kadar geciktiğini (ya da ne kadar kaldığını), atanan kişiyi ya da ekibi, konuyu, kişiyi ve bir
  bağlantıyı içerir.

Gönderi konuyu, kişinin adını, gelen kutusunu ve konuşmanın bağlantısını taşır. Gelen kutusu
**Include message text** seçeneğini açmadıkça mesajın kendisi Yuva'da kalır; açıksa gönderi ilk
300 karakterini de alıntılar. Kanalda konuşmanın bir başlığı zaten varsa anma ve SLA gönderisi o başlığa gider; oradaki yanıt
diğer başlıklardaki gibi çalışır.

API ile: `new_conversations_channel_id`, `mentions_channel_id`, `sla_channel_id`, `sla_warnings` ve `include_content` ile
`PUT /v1/inboxes/{id}/slack`; `GET /v1/slack/channels` botun bulunduğu kanalları listeler.

## Slack kullanıcınızı bağlayın

Her üye kendi Slack kullanıcısını profilinden bağlar: Yuva üyenin e-posta adresiyle (ya da
girdiğiniz başka bir adresle) Slack kullanıcısını bulur ve Yuva botu ona Slack'te 10 dakika
geçerli, 6 haneli bir kod gönderir. Kodu girmek ikisini bağlar. Bir Slack kullanıcısı aynı anda
tek bir üyeye aittir.

## Slack'ten yanıtlayın

Bir gönderinin başlığında:

- her yanıt, bağlı üye tarafından yazılmış bir **not** olarak konuşmaya eklenir;
- `!reply` ile başlayan yanıt, o üyenin yanıtı olarak **müşteriye** her zamanki yoldan (e-posta,
  sohbet ya da uygulama) gider. Bir sahip ya da yönetici `!send` gibi başka bir komut seçebilir;
  komut `/` ile başlayamaz, çünkü Slack onu kendi komutlarına ayırır.

İkisi de Yuva'da "via Slack" olarak görünür. Bağlantılar adreslerini korur; bağlı Slack
kullanıcılarının anılması üyelerin adlarına dönüşür.

Bir üyeye bağlanmamış ya da üyesi konuşmanın gelen kutusunu göremeyen bir Slack kullanıcısı için
hiçbir şey değişmez: nedenini söyleyen, yalnızca kendisinin gördüğü bir mesaj alır. Slack'teki
düzenlemeler ve silmeler aktarılmaz.

## Düzeltme, çöp kutusu ve silme

Slack, Yuva'nın gönderdiğini saklar; bu yüzden Yuva gönderiyi sonradan düzenler:

- bir mesajı düzeltmek (redact), metnini taşıyan gönderileri "Message removed." ile değiştirir;
- bir görüşmeyi çöp kutusuna taşımak, görüşmenin tüm gönderilerini "This conversation is in the
  trash." ile değiştirir; geri yüklemek metni geri getirmez;
- bir görüşmeyi kalıcı olarak silmek ya da kişisini silmek, gönderilerini "This conversation was
  deleted." ile değiştirir.

Yerine konan satırda ad, konu ya da metin yoktur; ilk ikisi görüşmeye giden bağlantıyı korur.
İnsanların Slack'te yazdığı yanıtlar değişmez; Slack'in artık botun düzenlemesine izin vermediği
bir gönderi (silinmiş, çalışma alanının düzenleme kurallarına göre çok eski ya da arşivlenmiş bir
kanalda) olduğu gibi kalır. Slack bağlı değilken hiçbir şey düzenlenmez.

## Bağlantıyı kesin

Ayarlar › Slack › **Disconnect**, bot jetonunu Slack'te iptal eder ve gelen kutularının
kanallarını, başlıkları ve üyelerin bağlantılarını kaldırır. Uygulamayı Slack'te kaldırmak da aynı
şeyi yapar. Bağlama ve bağlantıyı kesme ile üyelerin bağlaması ve bağlantıyı kaldırması çalışma
alanının denetim kaydında yer alır.
