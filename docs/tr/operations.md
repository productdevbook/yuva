# İşletim

Bir Yuva sunucusunu güncelleme, yedekleme, geri yükleme ve izleme. Örnekler
[kurulum rehberindeki](install.md) Compose kurulumunu kullanır ve onun dizininde çalıştırılır.

## Güncelleme

1. Kendi sürümünüzle yeni sürüm arasındaki her sürümün sürüm notlarını okuyun.
2. Veritabanını ve ekleri [yedekleyin](#yedekleme).
3. `compose.yaml` içindeki imaj etiketini değiştirin ve yeni sürümü başlatın:

   ```sh
   docker compose up -d
   ```

Migration'lar başlangıçta çalışır; `/readyz` `200` döndüğünde sunucu hazırdır. Güncelleme sırasında
tek bir sunucu süreci çalıştırın. Geri dönmek için 2. adımdaki yedeği geri yükleyin.

## Yedekleme

Tam bir yedeğin üç parçası vardır: veritabanı, ekler ve ana anahtar.

1. Veritabanını dökün. `pg_dump`, Yuva çalışırken tutarlı bir anlık görüntü alır:

   ```sh
   docker compose exec -T db pg_dump -U yuva -d yuva -Fc > yuva-$(date +%F).dump
   ```

2. Ardından ekleri arşivleyin. `YUVA_STORAGE=local` ile:

   ```sh
   docker run --rm -v yuva_attachments:/data:ro -v "$PWD":/backup alpine \
     tar czf /backup/yuva-attachments-$(date +%F).tgz -C /data .
   ```

   S3 ile sağlayıcının sürümleme ya da çoğaltma özelliğini kullanın veya bucket'ı
   `rclone sync` ile kopyalayın.

3. `YUVA_MASTER_KEY` değerini ikisinden de ayrı saklayın ([Ana anahtar](install.md#ana-anahtar)).

1. ve 2. adımları bu sırayla cron'dan çalıştırın ve dosyaları sunucunun dışına kopyalayın.

## Geri yükleme

Yedeğin alındığı `YUVA_MASTER_KEY` değerini kullanın:

```sh
docker compose stop yuva
docker compose exec -T db sh -c 'dropdb -U yuva --force yuva && createdb -U yuva yuva'
docker compose exec -T db pg_restore -U yuva -d yuva --no-owner < yuva-2026-10-07.dump
docker run --rm -v yuva_attachments:/data -v "$PWD":/backup alpine \
  sh -c 'rm -rf /data/* && tar xzf /backup/yuva-attachments-2026-10-07.tgz -C /data'
docker compose start yuva
```

Eski bir sürümden alınmış yedek, sunucu başlarken yeni sürüme taşınır.

## Operatör komutları

`docker compose exec yuva /yuva <command>` ile çalıştırın (pipe kullanırken `-T` ekleyin). `<ws>` bir
çalışma alanı id'si ya da tam adıdır.

```sh
docker compose exec -T yuva /yuva api-key create --workspace Example --name provisioning
```

Bir id ya da gizli anahtar yazdıran komutlar stdout'a yalnızca onu yazar.

| Komut | Ne yapar |
|---|---|
| `serve [--migrate=false]` | Sunucuyu çalıştırır (varsayılan); `--migrate=false` verilmedikçe önce migration'ları uygular. |
| `migrate up\|down\|status` | Bekleyen migration'ları uygular, sonuncuyu geri alır ya da listeler. |
| `bootstrap --email <address> --workspace <name> [--name <name>] [--locale en\|tr] [--allow-existing]` | Bir çalışma alanı ve ilk sahibini oluşturur. `--allow-existing` bir çalışma alanı daha ekler. |
| `api-key create --workspace <ws> --name <name> [--scope <scope>]... [--inbox <id>]...` | Bir API anahtarı oluşturur ve gizli değeri bir kez yazdırır. Varsayılan: tüm yetkiler, tüm gelen kutuları. |
| `api-key list --workspace <ws>` | Anahtarları listeler, gizli değerlerini asla göstermez. |
| `api-key revoke <id>` | Bir anahtarı iptal eder. |
| `inbox create --workspace <ws> --name <name> [--slug <slug>] [--locale <tag>] [--timezone <zone>] [--mode live\|async] [--expected-reply-minutes <n>]` | Bir gelen kutusu oluşturur ve id'sini yazdırır. Varsayılanlar: addan türetilen slug, `en`, `UTC`, `async`. |
| `inbox list --workspace <ws>` | Gelen kutularını listeler. |
| `inbox ses-tenant show --workspace <ws> [--inbox <id\|slug>]` | Her gelen kutusunun kayıtlı Amazon SES tenant durumunu, gönderim durumunu, nedenini, ilişkili kaynakları ve kanalları yazdırır. AWS'ye çağrı yapmaz. |
| `inbox ses-tenant sync (--workspace <ws> [--inbox <id\|slug>] \| --all) [--dry-run]` | Tenant'ları AWS'de oluşturur ve günceller, sonra durumlarını yeniler; gelen kutusu başına bir satır. `--all` geriye dönük doldurmadır; `--dry-run` yalnızca neyi değiştireceğini yazdırır. |
| `inbox ses-tenant reinstate --workspace <ws> --inbox <id\|slug>` | İtibar politikasından gelen bir duraklatmayı kaldırır. |
| `channel create-email --workspace <ws> --inbox <id\|slug> --name <name> --address <address> [--display-name <name>] [--from-address <address>] [--smtp-host <host> [--smtp-port <n>] [--tls starttls\|tls\|none] [--smtp-username <name>] [--smtp-password-file <path\|->]]` | Bir e-posta kanalı oluşturur ve id'sini yazdırır. Parola bir dosyadan ya da `-` ile stdin'den okunur. |
| `channel create-chat --workspace <ws> --inbox <id\|slug> --name <name> --origin <origin>... [--allow-anonymous] [--ask-email-offline=false] [--greeting <text>]` | Bir sohbet (web widget) kanalı oluşturur; id'sini ve genel anahtarını (`yuva_pk_…`) aralarında bir sekmeyle yazdırır. Her site için `--origin` yinelenir. |
| `channel list --workspace <ws> --inbox <id\|slug>` | Bir gelen kutusunun kanallarını listeler; sohbet kanallarında genel anahtarı ve origin'leri de gösterir. |
| `workspace delete --workspace <ws> --yes` | Bir çalışma alanını siler ([aşağıda](#çalışma-alanını-veya-hesabı-silme)). `--yes` olmadan yalnızca deneme yapar. |
| `workspace broadcasts approve\|deny\|suspend --workspace <ws> [--reason <text>]` | Bir çalışma alanının bülten onayını yanıtlar ya da onu askıya alır ([aşağıda](#barındırılan-sunucuda-bültenler)). |
| `workspace broadcasts limit --workspace <ws> --daily <n>` | Çalışma alanının UTC günü başına alıcı sayısını belirler; `0` sınırı kaldırır. |
| `workspace broadcasts show --workspace <ws>` | Durumunu, sınırını, 30 günlük oranlarını ve son bültenlerini yazdırır. |
| `analytics property create\|list`, `analytics geoip-update`, `workspace analytics-limit` | [Analitik](analytics.md) için web siteleri, konum veritabanı ve bir çalışma alanının aylık olay sınırı ([Yapılandırma](configuration.md#analitik)). |
| `person delete --email <address> --yes` | Bir kullanıcının hesabını siler. Bir çalışma alanının tek sahibiyse reddedilir. |
| `ingest-email --to <address> [--from <address>]` | stdin'den gelen ham bir mesajı teslim eder ([E-posta](email.md#herhangi-bir-mta-yuva-ingest-email)). |
| `vapid-keys` | Yeni bir Web Push anahtar çifti yazdırır. Veritabanı gerektirmez. |

Geri kalan her şey (sohbet ve uygulama kanalları, üyeler, webhook'lar, kimlik anahtarları) panelden
ya da `/v1` üzerinden yönetilir.

## Barındırılan sunucuda bültenler

[Bültenlerin](broadcasts.md) nasıl sınırlandığını `YUVA_BROADCASTS` belirler ([Yapılandırma](configuration.md#bültenler)).
`on` ile yapacak bir şey yoktur. `approval` ile (`YUVA_SIGNUP=open` iken varsayılan), kayıt yoluyla
oluşturulan bir çalışma alanı siz onaylayana kadar bülten zamanlayamaz ve her çalışma alanı günde 500
alıcıdan başlayarak ısınır, her temiz günden sonra bu sayı ikiye katlanır. `yuva bootstrap` ile önce ya
da sonra yapılan çalışma alanları `allowed` durumundadır ama yine ısınır.

Haberiniz olsun diye `YUVA_OPERATOR_EMAIL` atayın. Özet e-postası (5 dakikada en fazla bir) e-posta
içeriğini ve alıcıların adreslerini içermeden şunları listeler:

| Satır | Ne zaman |
|---|---|
| Bülten onayı istendi | Bir sahip istedi: çalışma alanı adı, kişinin adresi, web sitesi, kimin neyi gönderdiği, abonelerin nasıl izin verdiği. |
| Çalışma alanı bültenlerden askıya alındı | Sunucu, 30 günlük şikâyet ya da geri dönme oranı nedeniyle askıya aldı; oranlarla birlikte. |
| Bülten duraklatıldı | Bir bülten kendiliğinden duraklatıldı (geri dönme oranı, şikâyet oranı ya da gönderici hatası); bülten id'si ve nedeniyle. |
| SES tenant duraklatıldı | Amazon SES bir gelen kutusunun tenant'ını duraklattı: çalışma alanı ve gelen kutusu adları, kimin duraklattığı ve SES'in nedeni. |

Son 30 günde en az 1.000 bülten e-postasının en az %0,3'ü spam bildirildiğinde ya da en az 2.000
e-postanın en az %8'i geri döndüğünde bir çalışma alanı askıya alınır. Çalışan bültenleri duraklar.

CLI ile yanıtlayın. Çalışma alanı bir ad ya da id'dir; `--reason` (en fazla 500 karakter) çalışma
alanının sahiplerine gösterilir:

```sh
docker compose exec -T yuva /yuva workspace broadcasts approve --workspace Example
docker compose exec -T yuva /yuva workspace broadcasts deny --workspace Example --reason "No subscribe form on the site"
docker compose exec -T yuva /yuva workspace broadcasts suspend --workspace Example --reason "Complaints"
docker compose exec -T yuva /yuva workspace broadcasts limit --workspace Example --daily 2000
docker compose exec -T yuva /yuva workspace broadcasts show --workspace Example
```

| Komut | Etkisi |
|---|---|
| `approve` | Çalışma alanı gönderebilir. Askıyı da kaldırır; duraklayan bültenler bir üye sürdürene kadar duraklamış kalır. |
| `deny` | İstek hayır olarak yanıtlanır; çalışma alanı yeniden isteyebilir. |
| `suspend` | Zamanlamayı ve sürdürmeyi reddeder, çalışan bültenleri duraklatır (neden `workspace_suspended`). |
| `limit --daily <n>` | Çalışma alanının bültenleri toplamında UTC günü başına alıcı sayısı. Bunun, `YUVA_BROADCAST_DAILY_RECIPIENTS` ve ısınmanın en küçüğü geçerlidir. `0` yalnızca bu sınırı kaldırır. |
| `show` | Durum ve nedeni, günlük sınır ve bugünkü sayı, son 30 gün (gönderilen, geri dönme ve şikâyet oranları), kullandığı gönderen alan adları ve son 10 bülteni. |

Askıyı yalnızca operatör kaldırır. Onaylamadan önce web sitesine ve abonelerin nasıl izin verdiğine bakın.

## Barındırılan sunucuda Amazon SES tenant'ları

Aynı Amazon SES hesabı üzerinden gönderen birkaç çalışma alanı itibarı paylaşır. SES tenant'ları onları ayırır:
Yuva **gelen kutusu başına bir tenant** tutar, SES itibarı ve gönderimin durdurulmasını tenant başına izler; böylece
bir gelen kutusunun kötü gönderimi yalnızca o gelen kutusunu duraklatır. Bir kez kurulur.

1. Gönderen alan adlarınızı SES hesabında kimlik olarak doğrulayın (bkz. [Teslim edilebilirlik](deliverability.md#amazon-ses)).
2. Bir configuration set oluşturun ve geri dönmeleri ile şikâyetleri öğrenmek için `YUVA_SES_TOPIC_ARNS` içinde
   listelenen bir SNS topic'ine event destination ekleyin (bkz. [E-posta](email.md#geri-dönen-e-postalar)). Set'e kendi
   bastırma seçeneklerini vermeyin; böylece tenant'ınkiler geçerli olur.

   ```sh
   aws sesv2 create-configuration-set --configuration-set-name yuva --region REGION
   aws sesv2 create-configuration-set-event-destination --configuration-set-name yuva \
     --event-destination-name yuva-feedback --region REGION \
     --event-destination '{"Enabled":true,"MatchingEventTypes":["BOUNCE","COMPLAINT"],"SnsDestination":{"TopicArn":"arn:aws:sns:REGION:ACCOUNT:yuva-ses"}}'
   ```

3. Yuva için bu politikayla bir IAM kullanıcısı ve bir erişim anahtarı oluşturun. `REGION`, `ACCOUNT`, `PREFIX`
   (`YUVA_SES_TENANT_PREFIX` değeri) ve `CONFIG_SET` değerlerini değiştirin.

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "YuvaOwnTenants",
      "Effect": "Allow",
      "Action": [
        "ses:CreateTenant",
        "ses:GetTenant",
        "ses:DeleteTenant",
        "ses:TagResource",
        "ses:ListTenantResources",
        "ses:PutTenantSuppressionAttributes",
        "ses:UpdateReputationEntityCustomerManagedStatus"
      ],
      "Resource": "arn:aws:ses:REGION:ACCOUNT:tenant/PREFIX*"
    },
    {
      "Sid": "YuvaTenantResources",
      "Effect": "Allow",
      "Action": [
        "ses:CreateTenantResourceAssociation",
        "ses:DeleteTenantResourceAssociation"
      ],
      "Resource": [
        "arn:aws:ses:REGION:ACCOUNT:tenant/PREFIX*",
        "arn:aws:ses:REGION:ACCOUNT:identity/*",
        "arn:aws:ses:REGION:ACCOUNT:configuration-set/CONFIG_SET"
      ]
    },
    {
      "Sid": "YuvaReadAccount",
      "Effect": "Allow",
      "Action": [
        "ses:ListReputationEntities",
        "ses:ListEmailIdentities",
        "sts:GetAccessKeyInfo"
      ],
      "Resource": "*"
    }
  ]
}
```

   Yuva bu anahtarla hiç göndermez: kanalların SMTP kullanıcıları gönderir. Tenant'lar etiketle oluşturulduğu için
   `ses:TagResource` gerekir. `ses:ListReputationEntities` yalnızca `*` üzerinde verilebilir; salt okunurdur ve Yuva
   yalnızca kendi önekli tenant'ları ister. Politika önek dışındaki tenant'lara erişemez.
4. `YUVA_SES_REGION`, `YUVA_SES_ACCESS_KEY_ID`, `YUVA_SES_SECRET_ACCESS_KEY` ve `YUVA_SES_CONFIGURATION_SET`
   değerlerini ayarlayın ([Yapılandırma](configuration.md#gelen-e-posta)) ve yeniden başlatın. Dördü birden ya da hiçbiri.

Bir kanal, SMTP sunucusu `YUVA_SES_REGION` bölgesinin Amazon SES uç noktası ve kullanıcı adı sunucunun anahtarıyla
aynı AWS hesabının bir erişim anahtarı olduğunda yönetilir. Başka hesap ya da bölge üzerinden giden kanallara
dokunulmaz. Yönetilen bir kanal için sunucu gelen kutusunun tenant'ını oluşturur (geri dönmelerin ve şikâyetlerin
tenant düzeyinde bastırılmasıyla), configuration set'i ve gönderen kimliği ilişkilendirir ve kanaldan giden her
e-postaya başlıkları ekler. Bunu bir kanal kaydedildiğinde ve `workspace broadcasts approve` sırasında yapar;
başlangıçta AWS'ye yazmak için hiç çağrı yapmaz. Önceden var olan kanallar için geriye dönük doldurmayı bir kez çalıştırın:

```sh
docker compose exec -T yuva /yuva inbox ses-tenant sync --all --dry-run
docker compose exec -T yuva /yuva inbox ses-tenant sync --all
```

Sunucu beş dakikada bir tüm tenant'larının durumunu okur. Biri devre dışı kaldığında gelen kutusunun yönetilen
kanallardan giden çalışan ve zamanlanmış bültenleri duraklar (neden `ses_tenant`; diğer gelen kutuları göndermeye
devam eder), zamanlama ve sürdürme `409 ses_tenant_blocked` ile reddedilir, destek yanıtları yine denenir ve SES'in
cevabıyla başarısız olur, özete bir satır eklenir. Hiçbir şey kendiliğinden sürdürülmez. Bir gelen kutusunu ya da
çalışma alanını silmek tenant'larını da siler.

`show` bir tenant'ı kimin duraklattığını yazar. Duraklatma politikadan geldiyse `reinstate` SES'in yeniden
etkinleştirilmiş durumunu ayarlar ve bir üye bültenleri sürdürür. Amazon SES'in kendisi duraklattıysa yalnızca bir
AWS Support talebi kaldırır; `show` bunu söyler.

İsteğe bağlı, beş dakika yerine saniyeler içinde tepki için: SES'in tenant olaylarını SNS topic'ine gönderin. Olay
yalnızca Yuva'nın durumu yeniden okumasını sağlar; içeriğine güvenilmez.

```sh
aws events put-rule --name yuva-ses-tenants --region REGION \
  --event-pattern '{"source":["aws.ses"],"detail-type":["Sending Status Enabled","Sending Status Disabled","Advisor Recommendation Status Open","Advisor Recommendation Status Resolved"]}'
aws events put-targets --rule yuva-ses-tenants --region REGION --targets Id=yuva,Arn=arn:aws:sns:REGION:ACCOUNT:yuva-ses
```

Topic'in erişim politikasına şu ifadeyi ekleyerek EventBridge'in topic'e yayımlamasına izin verin:

```json
{
  "Sid": "AllowEventBridge",
  "Effect": "Allow",
  "Principal": { "Service": "events.amazonaws.com" },
  "Action": "sns:Publish",
  "Resource": "arn:aws:sns:REGION:ACCOUNT:yuva-ses"
}
```

## Çalışma alanını veya hesabı silme

**Bir çalışma alanını** bir sahip **Ayarlar → Çalışma alanı → Tehlikeli bölge** altından,
`{"name": "<exact name>"}` ile `DELETE /v1/workspace` çağrısıyla ya da `yuva workspace delete` ile
siler. Çalışma alanı hemen kullanılamaz hâle gelir; ardından bir arka plan işi verilerini siler ve
`workspace deletion` log satırları yazar. Üyelerin hesapları kalır.

**Bir hesabı** sahibi **Ayarlar → Profilim** altından, `{"email": "<their address>"}` ile
`DELETE /v1/me` çağrısıyla ya da `yuva person delete` ile siler. Mesajları yazarsız olarak kalır. Bir
çalışma alanının tek sahibi önce sahipliği devretmeli ya da çalışma alanını silmelidir.

## Denetim günlüğü

Denetim günlüğü, bir çalışma alanının erişimini, kimlik bilgilerini ve verilerini kimin değiştirdiğini
kaydeder: kimin katıldığı, ayrıldığı veya rol değiştirdiği; davetler; oluşturulan veya iptal edilen API
anahtarları ve bağlı uygulamalar; webhook'lar ve gizli anahtarları; e-posta kanalı kimlik bilgileri ve
anahtarları; gelen kutusu erişimi ve silinmeleri; Çöp kutusuna taşınan, geri alınan veya silinen konuşmalar; kişi silmeleri; onay ve analitik verisi dışa
aktarmaları; saklama değişiklikleri; ve operatörün bültenlerle ilgili yanıtları. Her kayıtta işlemi yapan
(bir üye, bir API anahtarı, bağlı bir uygulama üzerinden bir üye veya sunucunun komut satırı),
`api_key.created` gibi eylem, hedefi, birkaç ayrıntı ve zaman bulunur. Kayıt, değişiklikle aynı adımda
yazılır.

Mesaj içeriği, gizli anahtarlar, parolalar, webhook'un adresi (yalnızca ana makinesi) veya bir kişinin adı
ya da e-posta adresi kayda girmez. Çalışma alanının silinmesi kendi günlüğünde yoktur, çünkü günlük
çalışma alanıyla birlikte gider; [`YUVA_OPERATOR_EMAIL`](configuration.md) ayarlıysa operatörün özeti
bunu bildirir.

Yalnızca sahipler okuyabilir: panelde **Ayarlar → Denetim günlüğü** altında veya `GET /v1/audit-log` ile
(yöneticiler ve temsilciler `403 forbidden` alır). Bir API anahtarı, onu oluşturan üye hâlâ sahip olduğu
sürece `audit:read` [kapsamıyla](headless.md#yetkiler) okur.

```sh
curl -H "Authorization: Bearer $YUVA_KEY" \
  "$YUVA_URL/v1/audit-log?action=api_key.created&action=api_key.revoked&since=2026-01-01T00:00:00Z"
```

En yeniden eskiye, sayfada 25 kayıt (`limit` ile 100'e kadar), sonraki sayfa için `next_cursor`.

| Filtre | Gösterir |
|---|---|
| `actor_id` | Bu üyenin, anahtarın veya bağlı uygulamanın kayıtları. Üyenin kimliği, bağlı uygulamalar üzerinden yaptıklarıyla da eşleşir. |
| `actor_kind` | `member`, `api_key`, `oauth_grant` veya `operator`. |
| `action` | Bu eylemler; parametreyi tekrarlayın veya virgülle ayırın. |
| `since`, `until` | Zaman aralığı: `since` ve sonrası, `until` öncesi. |

Kayıtlar [`YUVA_AUDIT_DAYS`](configuration.md) gün saklanır (varsayılan 365).

## Veri saklama

Konuşmalar, mesajlar ve dosyalar, ait oldukları kişi silinene kadar kalır (panelden,
`DELETE /v1/contacts/{contactId}` ya da `DELETE /v1/contacts/by-external-id` ile). Bir sahip
**Ayarlar → Çalışma alanı** altından ya da `{"retention_days": 90}` ile `PATCH /v1/workspace`
çağrısıyla bir saklama süresi belirleyebilir; `null` her şeyi saklar (varsayılan).

Yuva saatte bir temizlik yapar:

| Veri | Saklama süresi |
|---|---|
| Olaylar (gerçek zamanlı tekrar oynatma ve `GET /v1/events`) | 7 gün |
| Kişi oturumları | süreleri dolana kadar, son kullanımdan 7 gün sonra |
| Tamamlanmış webhook teslimatları | 7 gün; uç nokta başına en yeni 100 tanesi kalır |
| Bayat gerçek zamanlı bağlantı kayıtları | 1 saat |
| [Çöp kutusundaki](headless.md#konuşmaları-silme) konuşmalar, mesajları ve dosyalarıyla | 30 gün |
| Kapalı konuşmalar, mesajları ve dosyalarıyla (`retention_days` atanmışsa) | son değişiklikten itibaren o kadar gün |
| Biten bir bültenin alıcıları (sayıları kalır) | bittikten 400 gün sonra |
| Sınırların dayandığı günlük gönderim toplamları | 90 gün |
| [Denetim günlüğü](#denetim-günlüğü) kayıtları | `YUVA_AUDIT_DAYS` gün (varsayılan 365) |
| Gelen e-postaların özgün `.eml` dosyası (`retention_days` atanmışsa) | o kadar gün; mesajın kendisi kalır |

## Birden fazla sunucu süreci

Her sayıda sunucu süreci tek bir veritabanını paylaşabilir (Helm chart'ında daha fazla `replicaCount`
ya da tek bir proxy arkasında birden çok konteyner). Süreçler Postgres üzerinden eşgüdüm sağlar:

- **Hız sınırları süreçler arasında geçerlidir.** Her sınır (oturum başlatma, widget yazmaları, OAuth
  izinleri ve MCP token'ları, herkese açık sayfalar, durum izleyici çağrıları, analitik toplama, ...)
  `rate_limits` tablosunda sayılır; on süreç, bir sürecin izin verdiğini verir, istek hangi sürece
  ulaşırsa ulaşsın. Bir çalışma alanının aynı anda çalıştırabileceği 4 analitik sorgu, `rate_slots`
  içindeki kiralardır ve aynı şekilde paylaşılır.
- İki tablo da `UNLOGGED`'dır: Postgres çökmesi ya da bir yedek sunucuya geçiş onları boşaltır; bu
  yalnızca her sınırın penceresini yeniden başlatır. Yedeklerin onları tutması gerekmez
  (`pg_dump --no-unlogged-table-data` satırlarını dışarıda bırakır).
- Anahtarlar anahtarlı özetlerdir (ana anahtar altında HMAC); istemci adresi ya da e-posta adresi asla
  değildir.
- `rate_limit_sweep` işi, süresi dolmuş pencereleri ve kiraları dakikada bir siler.
- Pencereler ve kiralar veritabanının saatiyle ölçülür; süreçlerin saatleri sınırları etkilemez.
- Veritabanında başarısız olan bir kontrol (bağlantı kaybı, zaman aşımı) isteği reddetmek yerine
  geçirir; sunucu bu sürdüğü sürece en fazla dakikada bir
  `rate limit check failed; letting requests through` kaydı yazar.
- [`YUVA_CLIENT_IP_HEADER` ve `YUVA_TRUSTED_PROXIES`](configuration.md) her süreçte aynı olmalıdır;
  yoksa adres başına sınırlar farklı adresleri sayar.

## Sağlık kontrolleri ve loglar

| Uç nokta | Port | Yanıt |
|---|---|---|
| `GET /healthz` | 8080 | Süreç çalıştığı sürece `200`. Canlılık kontrolü. |
| `GET /readyz` | 8080 | Veritabanına ulaşılabiliyorsa `200`, yoksa `503`. Hazırlık ve erişilebilirlik kontrolü. |
| `GET /v1/version` | 8080 | Çalışan sürüm. |
| `GET /metrics` | 9090 | Prometheus: `yuva_http_requests_total`, `yuva_http_request_seconds`, Go ve süreç metrikleri. |

Loglar stderr'e JSON satırları olarak yazılır. Alarm kurmaya değer satırlar:

| Log mesajı | Anlamı |
|---|---|
| `mail not sent` | Sunucunun SMTP hesabı çalışmıyor; giriş kodları ulaşmıyor. |
| `email send failed, retrying` | Bir kanalın SMTP hesabı çalışmıyor. |
| `YUVA_SMTP_HOST is not set` (`YUVA_INGRESS_SECRET` ve `YUVA_VAPID_PUBLIC_KEY` için de aynısı) | Bir özellik yapılandırma nedeniyle kapalı. |
| `not ready` | Veritabanına ulaşılamıyor. |

## Dağıtık izleme

Yuva, OpenTelemetry izlerini OTLP ile bunu kabul eden her toplayıcıya ya da servise (Jaeger, Tempo, Grafana
Cloud, Honeycomb ve diğerleri) gönderir. `OTEL_EXPORTER_OTLP_ENDPOINT` atanana kadar izleme kapalıdır ve ek yük
getirmez; değişkenler [Yapılandırma](configuration.md#dağıtık-izleme) sayfasında. Sunucu başlarken
`OpenTelemetry traces are on` yazar ve durduğunda son span'ları gönderir.

Neler izlenir:

| Span | Adı | Kapsamı |
|---|---|---|
| HTTP isteği | rota deseni, örneğin `GET /v1/conversations/{id}` | Her istek; yöntemi, rotası ve durum koduyla. Gelen `traceparent` başlığı sürdürülür. |
| Arka plan işi | `river.work/<kind>` | Bir işin her çalışması; onu kuyruğa alan isteğe bağlanır. |
| Veritabanı sorgusu | sorgu adı, örneğin `GetConversation` | Bir istek ya da iş sırasında yapılan sorgular. İş kuyruğunun kendi yoklaması gönderilmez. |
| Posta gönderimi | `mail.send`, `smtp.send` | Giden bir ileti ve SMTP işlemi. |
| Webhook teslimi | `webhook.deliver` | Tek bir teslim denemesi. |

Hiçbir zaman kaydedilmeyenler: ileti gövdeleri, e-posta adresleri, başlık değerleri, belirteçler, sırlar, sorgu
dizesi içeren URL'ler, istemci adresleri, SQL metni ve sorgu argümanları ile hata mesajları. Bir span yalnızca
sabit bir listedeki öznitelikleri taşır; liste dışa gönderimden önce uygulanır, bu yüzden bir kütüphanedeki
sonraki bir değişiklik fazlasını ekleyemez:

- istek: `http.request.method`, `http.route` (desen, asla yol değil), `http.response.status_code`
- çalışma alanı ve gelen kutusu kimlikleri: `yuva.workspace_id`, `yuva.inbox_id`
- işler: `kind`, `queue`, `attempt`, `priority`, `id`, `status`, `messaging.system`
- posta: `yuva.recipients` (bir sayı) ve `yuva.message_bytes`
- veritabanı ve hatalar: `db.system.name`, SQL durum kodu ve istisna türü

İzlenen bir istek ya da iş sırasında yazılan log satırları `trace_id` ve `span_id` taşır. Bir log satırındaki
kimliği izleme servisinize yapıştırarak izi açın.

Örnekleme: varsayılan olarak her istek izlenir. Bir kısmını tutmak için örnekleyiciyi atayın, örneğin
`OTEL_TRACES_SAMPLER=parentbased_traceidratio` ve `OTEL_TRACES_SAMPLER_ARG=0.1`.

Aldığı span'ları yazdıran yerel bir toplayıcı örneği:

```yaml
# otel-collector.yaml
receivers:
  otlp:
    protocols:
      http:
        endpoint: 0.0.0.0:4318
exporters:
  debug:
    verbosity: detailed
service:
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [debug]
```

```sh
docker run -d --name otel-collector -p 4318:4318 \
  -v "$PWD/otel-collector.yaml:/etc/otelcol/config.yaml" \
  otel/opentelemetry-collector:latest
```

`.env` dosyasına ekleyin, sonra sunucuyu yeniden başlatın:

```sh
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
```

Panelde herhangi bir sayfayı açın ve toplayıcının logunu okuyun (`docker logs otel-collector`): her istek için adı
rota olan bir span görünür. Yuva da Docker'da çalışıyorsa `localhost` yerine toplayıcının servis adını kullanın.
