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
| `channel create-email --workspace <ws> --inbox <id\|slug> --name <name> --address <address> [--display-name <name>] [--from-address <address>] [--smtp-host <host> [--smtp-port <n>] [--tls starttls\|tls\|none] [--smtp-username <name>] [--smtp-password-file <path\|->]]` | Bir e-posta kanalı oluşturur ve id'sini yazdırır. Parola bir dosyadan ya da `-` ile stdin'den okunur. |
| `channel list --workspace <ws> --inbox <id\|slug>` | Bir gelen kutusunun kanallarını listeler. |
| `workspace delete --workspace <ws> --yes` | Bir çalışma alanını siler ([aşağıda](#çalışma-alanını-veya-hesabı-silme)). `--yes` olmadan yalnızca deneme yapar. |
| `person delete --email <address> --yes` | Bir kullanıcının hesabını siler. Bir çalışma alanının tek sahibiyse reddedilir. |
| `ingest-email --to <address> [--from <address>]` | stdin'den gelen ham bir mesajı teslim eder ([E-posta](email.md#herhangi-bir-mta-yuva-ingest-email)). |
| `vapid-keys` | Yeni bir Web Push anahtar çifti yazdırır. Veritabanı gerektirmez. |

Geri kalan her şey (sohbet ve uygulama kanalları, üyeler, webhook'lar, kimlik anahtarları) panelden
ya da `/v1` üzerinden yönetilir.

## Çalışma alanını veya hesabı silme

**Bir çalışma alanını** bir sahip **Ayarlar → Çalışma alanı → Tehlikeli bölge** altından,
`{"name": "<exact name>"}` ile `DELETE /v1/workspace` çağrısıyla ya da `yuva workspace delete` ile
siler. Çalışma alanı hemen kullanılamaz hâle gelir; ardından bir arka plan işi verilerini siler ve
`workspace deletion` log satırları yazar. Üyelerin hesapları kalır.

**Bir hesabı** sahibi **Ayarlar → Profilim** altından, `{"email": "<their address>"}` ile
`DELETE /v1/me` çağrısıyla ya da `yuva person delete` ile siler. Mesajları yazarsız olarak kalır. Bir
çalışma alanının tek sahibi önce sahipliği devretmeli ya da çalışma alanını silmelidir.

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
| Kapalı konuşmalar, mesajları ve dosyalarıyla (`retention_days` atanmışsa) | son değişiklikten itibaren o kadar gün |
| Gelen e-postaların özgün `.eml` dosyası (`retention_days` atanmışsa) | o kadar gün; mesajın kendisi kalır |

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
