# E-posta

E-posta kanalı bir gelen kutusuna destek adresi verir. Bu adrese gelen e-posta bir konuşma başlatır
ya da mevcut konuşmaya eklenir; yanıtlar aynı adresten, aynı zincirde gider.

## Kanalı oluşturun

Panelde **Ayarlar → gelen kutusu → Müşteriler nereden yazıyor → E-posta** yolunu izleyin. Ya da
sunucuda:

```sh
yuva channel create-email --workspace Example --inbox support --name "Support" \
  --address support@example.com --display-name "Example Support" \
  --smtp-host email-smtp.eu-west-1.amazonaws.com --smtp-username <user> --smtp-password-file -
```

Yanıtları SMTP hesabı gönderir ([Giden SMTP](#giden-smtp)). Sonra gelen e-postayı aşağıdaki üç
yoldan biriyle Yuva'ya yönlendirin ve [DNS](#dns-kontrol-listesi) kayıtlarını ayarlayın.

## Gelen e-posta

Yuva SMTP dinlemez. E-postayı içeri almak için şu üç yoldan birini seçin.

### Cloudflare Email Worker

`edge/`, e-postayı Yuva'ya ileten bir Cloudflare Email Worker'dır. Cloudflare Email Routing alan
adının MX'i olur; bu yüzden başka bir e-posta sunucusunun kullanmadığı bir alan adı ya da alt alan
adı seçin.

1. Sunucuda şunları atayın ve yeniden başlatın:

   ```sh
   YUVA_INGRESS_SECRET=<openssl rand -hex 32>
   YUVA_INGRESS_AUTHSERV_ID=mx.cloudflare.net
   ```

2. Worker'ı Cloudflare hesabınızla deploy edin:

   ```sh
   cd edge
   bun install
   bunx wrangler login
   bunx wrangler secret put INGRESS_SECRET          # YUVA_INGRESS_SECRET ile aynı değer
   bunx wrangler deploy --var YUVA_URL:https://support.example.com
   ```

3. Cloudflare panelinde alan adını açın → **Email → Email Routing**, etkinleştirin ve MX ile SPF
   kayıtlarını eklemesine izin verin.
4. **Routing rules** altında her destek adresini `yuva-edge` Worker'ına gönderin.
   [Catch-all kanal](#catch-all-kanallar) için catch-all kuralını Worker'a yönlendirin.
5. Bir deneme e-postası gönderin ve konuşmanın panelde belirdiğini görün.

Yuva kapalı ya da meşgulse gönderen sunucu daha sonra yeniden dener. Bu e-postaları kaybetmemek için
`--var FALLBACK_FORWARD:you@example.org` ile deploy edin (doğrulanmış bir Email Routing adresi):
e-posta oraya gider.

Eski bir Email Worker `v1` ile imzalar ve sunucu bunu reddeder. Worker'ı yeniden deploy edin ya da o
zamana kadar `YUVA_INGRESS_ACCEPT_V1=true` atayın.

Email Worker `v3` ile imzalar; sunucu bu imzayı e-postayı okumadan önce denetler. Bundan önce deploy
edilen Worker'lar `v2` ile imzalar; `YUVA_INGRESS_ACCEPT_V2` açık olduğu sürece (şimdilik varsayılan)
bu imza da kabul edilir. `v2` ile e-posta, gerçek olduğu anlaşılmadan önce okunur; bu yüzden herkes
sunucuya imzasız yüklemeler biriktirtebilir (GHSA-jfvx-vc82-56f7 duyurusu). Worker'ı yeniden deploy
edin, ardından `YUVA_INGRESS_ACCEPT_V2=false` atayın. API'nin önündeki bir proxy `/ingress/email`
isteklerinde `Content-Length` başlığını korumalıdır: `v3`, parça parça (chunked) gönderilen gövdeyi
reddeder.

### Herhangi bir MTA: `yuva ingest-email`

Bir komuta teslim edebilen MTA, ham mesajı pipe ile verir:

```sh
yuva ingest-email --to <envelope recipient> [--from <envelope sender>] < message.eml
```

Komutun sunucuyla aynı `YUVA_DATABASE_URL`, `YUVA_MASTER_KEY` ve depolama ayarlarına ihtiyacı var.
Ayrı bir e-posta sunucusunda S3 depolama kullanın ki ikisi de aynı yere yazsın. Programı imajdan
kopyalayın:

```sh
docker create --name yuva-bin ghcr.io/productdevbook/yuva:0.0.32
docker cp yuva-bin:/yuva /usr/local/bin/yuva && docker rm yuva-bin
```

Postfix için ayarları yükleyen bir `/usr/local/bin/yuva-ingest` sarmalayıcısı yazın:

```sh
#!/bin/sh
set -a
. /etc/yuva/ingest.env
exec /usr/local/bin/yuva ingest-email "$@"
```

Bunu `master.cf` dosyasına ekleyin ve destek adreslerini `transport_maps` içinde ona yönlendirin
(`support@example.com yuva:`):

```
yuva      unix  -       n       n       -       -       pipe
  flags=q user=yuva argv=/usr/local/bin/yuva-ingest --to ${recipient} --from ${sender}
```

### Herhangi bir MTA: HTTP

Ham mesajı `YUVA_INGRESS_SECRET` ile imzalayarak `/ingress/email` adresine POST edin. Biçim
[edge/README.md](../../edge/README.md#ingress-contract) dosyasında. `YUVA_INGRESS_AUTHSERV_ID`
değerini MTA'nızın `Authentication-Results` başlığına yazdığı authserv-id olarak atayın; yoksa DMARC
kullanılmaz. MTA'nız ilettiği her mesajın en üstüne kendi `Authentication-Results` başlığını
eklemelidir: Yuva en üsttekini okur, bu yüzden MTA'nın başlık eklemediği bir mesaj, göndericinin
yazdığı bir başlıkla değerlendirilir.

## Yönlendirme adresi

Sunucuda bir [yönlendirme alan adı](configuration.md#yuvaya-taşıma) varsa her e-posta kanalı ikinci bir
adresten de e-posta alır: `<belirteç>@<yönlendirme alan adı>`. Bir posta kutusunu oraya yönlendirin; örneğin
[Yuva'ya taşırken](migrate.md) başka bir sağlayıcıdan. Bir posta kutusunu kendi adresine yönlendirmek
döngü yaratırdı; bu adres bu yüzden vardır. Ona gönderilen e-posta, kanalın kendi adresine gelen gibi işlenir.

Adresi **Ayarlar → gelen kutusu → Müşteriler nereden yazıyor → E-posta** altında, bir kopyalama düğmesiyle
bulursunuz. "Yenile" kanala yeni bir adres verir (`POST /v1/channels/{channelId}/forwarding-address/rotate`,
`inboxes:manage`): eski adrese gelen e-posta o andan sonra reddedilir; bu yüzden sağlayıcıdaki yönlendirmeyi
hemen değiştirin.

İşletmeci yönlendirme alan adının e-postalarını Yuva'ya bir kez yöneltir; kanalların kendi alan adlarıyla
aynı yoldan ([Gelen e-posta](#gelen-e-posta) altındaki üç yol). Yönlendirme alan adı yoksa kanalların
yönlendirme adresi de yoktur.

## Giden SMTP

Her e-posta kanalı kendi SMTP hesabıyla gönderir (SES, Postmark, kendi relay'iniz). Bunu kanal
ayarlarında girin. SMTP hesabı olmayan kanal e-posta alır ama yanıtlar reddedilir
(`email_not_configured`).

Kişinin okumadığı sohbet ve geri bildirim yanıtları, gelen kutusunun e-posta kanalı üzerinden
e-postayla gönderilir (`YUVA_CHAT_EMAIL_DELAY` sonra, bkz. [Yapılandırma](configuration.md#sohbet)).
Sohbet ya da uygulama kanalı olan her gelen kutusuna bir de e-posta kanalı ekleyin.

Geliştirmedeki Mailpit gibi özel ağ ve loopback SMTP sunucuları `YUVA_SMTP_ALLOW_PRIVATE=true`
ister.

### Amazon SES tenant'ları

SES itibarı ve gönderimin durdurulmasını tenant başına tutar. Sunucunun operatörü
[tenant yönetimini](operations.md#barındırılan-sunucuda-amazon-ses-tenantları) kurmuşsa ve bir kanal operatörün SES
hesabı üzerinden gönderiyorsa, her gelen kutusu kendi tenant'ında gönderir: bir gelen kutusunun geri dönmeleri
ya da şikâyetleri yalnızca o gelen kutusunu duraklatır, başka bir gelen kutusunu ya da çalışma alanını değil.
Ekibinizin yapacağı bir şey yok. Kanalın gönderim sayfası ve gelen kutusu ayarları durumu sahiplere ve
yöneticilere gösterir:

| Durum | Anlamı |
|---|---|
| Yalıtılmış | E-postalar gelen kutusunun tenant'ını taşır. |
| Kuruluyor | Sunucu tenant'ı oluşturuyor. Saniyeler sürer; bülten bekler ve kendiliğinden başlar. |
| Kimlik eksik | Gönderen adres ya da alan adı SES hesabında doğrulanmış bir kimlik değil. Bkz. [Teslim edilebilirlik](deliverability.md#amazon-ses). E-postalar tenant'sız gider ve bültenler engellenir. |
| Duraklatıldı | Amazon SES gelen kutusunun tenant'ını duraklattı. Yanıtlar SES'in cevabıyla başarısız olur; sunucunun operatörü yeniden etkinleştirir. |
| Hata | Bir adım başarısız oldu; sayfa nedenini gösterir. |
| Kayıp | Tenant artık Amazon SES'te yok. E-postalar tenant'sız gider ve sunucu onu yeniden oluşturana kadar bültenler engellenir. |

Yönetilen hesap üzerinden giden e-postalar her zaman `X-SES-CONFIGURATION-SET` taşır; `X-SES-TENANT` ise tenant
hazır olunca eklenir. İki başlık da yalnızca Amazon SES üzerinden giden e-postalara eklenir.

Sunucunun yönetmediği kendi SES hesabınızla tenant'ı kendiniz yazın: kanalda `ses.tenant` ve kullanıyorsanız
`ses.configuration_set` ayarlayın (harf, rakam, `-` ve `_`, en fazla 64). Yuva bunları SES üzerinden giden
e-postalara başlık olarak ekler ve AWS'de hiçbir şeyi denetlemez. Sunucu gelen kutusunun tenant'ını yönettiği
sürece yok sayılırlar.

## Geri dönen e-postalar

Bir geri dönme (bounce) ya da şikâyet, mesajı başarısız ve adresi teslim edilemez olarak işaretler;
bir üye kişi üzerindeki işareti temizleyene kadar bu adrese yanıtlar reddedilir. Kanal adresine
gelen geri dönme raporları kurulum gerektirmez. Amazon SES için SNS bağlayın:

1. SES kimliğinizin bölgesinde bir SNS topic oluşturun, ör. `yuva-ses`.
2. Kimliğin (ya da configuration set'in) bounce ve complaint bildirimlerini bu topic'e gönderin.
3. `YUVA_SES_TOPIC_ARNS` değerini topic ARN'si olarak atayın ve Yuva'yı yeniden başlatın.
4. `https://support.example.com/ingress/ses` adresini topic'e HTTPS ile, raw message delivery
   kapalı olarak abone edin. Yuva aboneliği kendisi onaylar.

## DNS kontrol listesi

Bir kanalın gönderim yaptığı her alan adı için:

| Kayıt | Değer |
|---|---|
| SPF | SMTP sağlayıcınızı içeren TXT, ör. `v=spf1 include:amazonses.com ~all`. Bir ad için tek SPF kaydı. |
| DKIM | SMTP sağlayıcınızın verdiği CNAME ya da TXT kayıtları. |
| DMARC | `_dmarc.example.com TXT "v=DMARC1; p=none; rua=mailto:dmarc@example.com"`; raporlar düzgün görününce `p=quarantine`'e geçin. |
| MX | Gelen e-posta için: Email Routing'in eklediği kayıtlar ya da MTA'nızınkiler. |
| Custom MAIL FROM | Sağlayıcınızın bounce alt alan adı için istediği MX ve SPF kayıtları (SES ve diğerleri). |

Kontrol ettiğiniz bir posta kutusuna deneme yanıtı gönderin ve `spf=pass`, `dkim=pass` ve
`dmarc=pass` olduğuna bakın.

## Başvuru

### Catch-all kanallar

Adresi `*@example.com` olan bir kanal, alan adının diğer tüm adreslerine gelen e-postaları alır.
Yanıtlar kişinin yazdığı adresten gider; bu yüzden SMTP hesabının tüm alan adı adına gönderebilmesi
gerekir. E-postayla devam eden sohbetler için ona bir `--from-address` verin.

```sh
yuva channel create-email --workspace Example --inbox support --name "Everything else" \
  --address '*@example.com' --from-address support@example.com \
  --smtp-host smtp.example.com --smtp-username support@example.com --smtp-password-file -
```

### Gelen e-posta nasıl işlenir

| Konu | Davranış |
|---|---|
| Kanal | Zarf alıcısının kanalı; `local+tag@domain` bulunamazsa `local@domain`'e düşer. Bilinmeyen adresler reddedilir. |
| Zincirleme | `In-Reply-To` ve `References` ile; diğer her şey yeni konuşma başlatır. Bir yanıt konuşmayı yeniden açar. |
| Cc ve katılımcılar | Bir e-postanın Yuva'ya ait olmayan `To` ve `Cc` adresleri konuşmanın katılımcıları olur (en çok 20). Yalnızca e-posta DMARC'tan geçtiyse (bunun için `YUVA_INGRESS_AUTHSERV_ID` gerekir) etkin olurlar; aksi hâlde önerilir, üyelere gösterilir ama asla kopyalanmaz. Üyeler katılımcı ekleyip çıkarabilir. Etkin bir katılımcının DMARC'tan geçen yanıtı konuşmaya onun mesajı olarak eklenir; zinciri yanıtlayan başka biri, bu konuşmaya bağlı yeni bir konuşma başlatır. DMARC'tan geçemeyen veya otomatik e-posta kimseyi eklemez. |
| Yanıtlar | `To` kişiye, `Cc` etkin katılımcılara gider; otomatik e-postalar (karşılama, kurallar) kimseyi kopyalamaz. Her yanıt `Cc`'yi değiştirebilir ve `Bcc` ekleyebilir (her biri 20 adres); `Bcc` alıcıları e-postayı alır ama e-postada hiç görünmez ve onları yalnızca üyeler görür. Geri dönen veya şikâyet edilen bir adres reddedilir ya da katılımcıysa atlanır; toplu gönderimlerden abonelikten çıkmak yanıtları durdurmaz. |
| İletme | Bir üye tek bir mesajı veya tüm konuşmayı (mesajlarını; notları ve taslakları asla, dosyalarını 20 MiB'a kadar isteğe bağlı olarak) üstüne kendi notunu ekleyerek en çok 10 dış adrese iletebilir. İleti gelen kutusunun e-posta kanalından üyenin adıyla, `Reply-To` kanal adresi ve `Fwd: …` konusuyla gider; yalnızca üyelere gösterilir, kişiye asla. Kişinin adresi, konuşmada zaten kopyada olan adresler, Yuva'nın kendi adresleri ve geri dönen veya şikâyet edilen adresler reddedilir; bir üye saatte en çok 20 kez iletir (çalışma alanı başına 200). İletiye DMARC'tan geçen bir yanıt konuşmaya yalnızca üyelerin göreceği şekilde eklenir ve göndereni yanıtlarda kopyalanmayan iletilmiş bir katılımcı yapar; ondan gelen diğer e-postalar yeni konuşma başlatır. Bir yanıt yalnızca iletiyi gönderen üyeye ve atanan kişiye bildirilir. Asistanlar yalnızca göndermelerine izin verilen yerde iletir; aksi hâlde taslak bırakır. |
| Otomatik e-posta | Otomatik yanıtlar, toplu e-postalar ve teslim raporları saklanır, asla yanıtlanmaz. Yuva'nın kendi adreslerinden gelen e-posta atılır. |
| DMARC | Yalnızca `YUVA_INGRESS_AUTHSERV_ID` ile, en üstteki `Authentication-Results` bu authserv-id'yi taşıyorsa ondan kullanılır. Bu kimliği taşıyan birden çok başlık, göndericinin birini eklediğini gösterir: aralarındaki bir `fail` sayılır, bir `pass` sayılmaz. DMARC'tan geçemeyen yeni bir konuşma spam olarak işaretlenir; geçemeyen bir yanıt yeni konuşma başlatır. |

### Sınırlar

| Sınır | Değer |
|---|---|
| Mesaj boyutu | 25 MiB |
| Ekler | Kanalın [ek sınırları](configuration.md#ekler-ve-depolama); sınırı aşan bir parça atlanır, mesaj korunur ve parça ham e-postada kalır |
| Gönderici ve kanal başına yeni konuşma | Saatte 20; fazlası göndericinin son konuşmasına eklenir |
| Gönderici ve çalışma alanı başına gelen e-posta | `YUVA_EMAIL_SENDER_HOURLY_CAP`, saatte 500; fazlası reddedilir |
| Aynı anda işlenen mesaj | `YUVA_INGRESS_MAX_CONCURRENT`, 8; fazlası `503` alır |
| Mesaj gövdesini gönderme süresi | 30 saniye; daha yavaş gönderimler `503` alır |
| Worker'ın Yuva'yı bekleme süresi | 20 saniye |

### `ingest-email` çıkış kodları

| Kod | Anlamı |
|---|---|
| `0` | Kabul edildi (`stored`, `duplicate`, `bounce` ya da `dropped`) |
| `64` | Hatalı argümanlar |
| `65` | Bozuk ya da çok büyük mesaj |
| `67` | Bilinmeyen alıcı |
| `75` | Geçici hata; yeniden deneyin |
| `77` | Reddedildi, ör. engellenmiş gönderici |
