# Yapılandırma

Yuva ayarlarını başlarken ortam değişkenlerinden okur. Hatalı bir değer, değişkenin adını veren bir
mesajla sunucuyu durdurur.

Çalışma alanı, gelen kutusu ya da kanal ayarları burada değil; panelde, API'de ve
[operatör komutlarında](operations.md#operatör-komutları) yapılır.

## Değer atama

1. Değişkeni `compose.yaml` dosyasının yanındaki `.env` dosyasına ekleyin:

   ```sh
   YUVA_CHAT_EMAIL_DELAY=10m
   ```

2. Sunucuyu yeniden başlatın:

   ```sh
   docker compose up -d
   ```

Boş bir değişken, `YUVA_METRICS_ADDR` dışında, atanmamış sayılır ve varsayılanı alır. Listeler
virgülle ayrılır. Boolean değerler `true`/`false`, `1`/`0` ve `t`/`f` kabul eder.

## Zorunlu

| Değişken | Anlamı |
|---|---|
| `YUVA_DATABASE_URL` | Postgres 16+ adresi, ör. `postgres://yuva:secret@db:5432/yuva?sslmode=disable`. |
| `YUVA_MASTER_KEY` | base64 ile 32 rastgele bayt (`openssl rand -base64 32`). Saklanan gizli bilgileri şifreler. Bkz. [Ana anahtar](install.md#ana-anahtar). |
| `YUVA_PUBLIC_URL` | Kişilerin ve uygulamaların kullandığı adres, ör. `https://support.example.com`, alan adının kökünde. Varsayılan `http://localhost:8080`, yalnızca yerel denemeler için. |

## Sunucu

| Değişken | Varsayılan | Anlamı |
|---|---|---|
| `YUVA_LISTEN_ADDR` | `:8080` | Metrikler dışındaki her şey için HTTP adresi. |
| `YUVA_METRICS_ADDR` | `:9090` | Prometheus adresi (`GET /metrics`). Dışarı açmayın. `off` ya da boş değer kapatır. |
| `YUVA_CLIENT_IP_HEADER` | atanmamış | Vekil sunucunuzun istemci IP'sini yazdığı başlık, ör. `X-Real-IP`; hız sınırları için. `X-Forwarded-For` gibi bir listede, `YUVA_TRUSTED_PROXIES` içinde olmayan en sağdaki adres sayılır. |
| `YUVA_TRUSTED_PROXIES` | atanmamış | Vekil sunucularınızın adresleri ya da CIDR aralıkları, ör. `10.0.0.0/8`. Atanırsa istemci IP başlığı yalnızca onlardan okunur. |
| `YUVA_COOKIE_SECURE` | `YUVA_PUBLIC_URL` `https` ise `true` | Oturum çerezini `Secure` olarak işaretler. |
| `YUVA_MCP` | `on` | `off`, MCP uç noktası `/mcp`'yi kapatır (`404` döner). OAuth açık kalır. |
| `YUVA_PANEL` | `on` | `off`, panelin ve OAuth onay sayfasının sunulmasını durdurur; API anahtarları çalışmaya devam eder. |
| `YUVA_WIDGET` | `on` | `off`, `/yuva.js`, `/yuva-chat.js` ve `/yuva-docs.js` dosyalarının sunulmasını durdurur; `/client/v1` kalır. |

## Giriş ve geçiş anahtarları

| Değişken | Varsayılan | Anlamı |
|---|---|---|
| `YUVA_WEBAUTHN_RP_ID` | `YUVA_PUBLIC_URL` alan adı | Geçiş anahtarı relying party id'si. Değiştirmek mevcut geçiş anahtarlarını kullanılamaz yapar. |
| `YUVA_WEBAUTHN_RP_NAME` | `Yuva` | Tarayıcıların yeni geçiş anahtarı için gösterdiği ad. |
| `YUVA_WEBAUTHN_ORIGINS` | `YUVA_PUBLIC_URL` origin'i | Geçiş anahtarları ve panelin gerçek zamanlı soketi için izin verilen origin'ler. |

## Sunucu e-postası

Giriş kodları, davetler ve bildirimler. E-posta kanallarının [kendi SMTP'si](email.md#giden-smtp)
vardır.

| Değişken | Varsayılan | Anlamı |
|---|---|---|
| `YUVA_SMTP_HOST` | atanmamış | SMTP sunucusu. Atanmamışsa e-postalar loga yazılır. |
| `YUVA_SMTP_PORT` | `tls` ile `465`, yoksa `587` | SMTP portu. |
| `YUVA_SMTP_TLS` | `starttls` | `starttls`, `tls` (doğrudan TLS) ya da `none`. |
| `YUVA_SMTP_USERNAME` | atanmamış | SMTP kullanıcı adı. |
| `YUVA_SMTP_PASSWORD` | atanmamış | SMTP parolası. |
| `YUVA_SMTP_FROM` | atanmamış | Gönderen, ör. `Yuva <yuva@example.com>`. `YUVA_SMTP_HOST` ile birlikte zorunlu. |
| `YUVA_SMTP_ALLOW_PRIVATE` | `false` | E-posta kanallarının loopback ve özel ağ adreslerindeki SMTP sunucularını (ör. Mailpit) kullanmasına izin verir. |

## Ekler ve depolama

| Değişken | Varsayılan | Anlamı |
|---|---|---|
| `YUVA_STORAGE` | `local` | `local` (bir dizin) ya da `s3` (S3, R2, MinIO, …). |
| `YUVA_STORAGE_DIR` | `data/attachments` | `local` için dizin; Docker imajında `/data/attachments`. |
| `YUVA_S3_ENDPOINT` | atanmamış | Uç nokta adresi, ör. `https://s3.eu-central-1.amazonaws.com`. |
| `YUVA_S3_REGION` | `auto` | Bölge. |
| `YUVA_S3_BUCKET` | atanmamış | Bucket. |
| `YUVA_S3_ACCESS_KEY_ID` | atanmamış | Erişim anahtarı id'si. |
| `YUVA_S3_SECRET_ACCESS_KEY` | atanmamış | Gizli erişim anahtarı. |
| `YUVA_S3_PATH_STYLE` | `false` | Path-style adresler; MinIO genelde bunu ister. |
| `YUVA_ATTACHMENT_MAX_BYTES` | `26214400` (25 MiB) | Bayt cinsinden en büyük ek boyutu. |
| `YUVA_ATTACHMENT_TYPES` | aşağıda | İzin verilen içerik türleri; `image/*` gibi joker karakterler çalışır. |

Varsayılan türler: `image/png,image/jpeg,image/gif,image/webp,image/heic,application/pdf,text/plain,text/csv,application/zip,application/json,video/mp4,video/quicktime,audio/mpeg,audio/mp4`.

## Gelen e-posta

| Değişken | Varsayılan | Anlamı |
|---|---|---|
| `YUVA_INGRESS_SECRET` | atanmamış | `/ingress/email` isteklerini imzalar; Email Worker aynı değeri kullanır. Atanmamışsa bu uç nokta tüm e-postaları reddeder. |
| `YUVA_INGRESS_ACCEPT_V1` | `false` | Eski bir Email Worker'ı yeniden deploy edene kadar eski `v1` imzasını da kabul eder. |
| `YUVA_INGRESS_AUTHSERV_ID` | atanmamış | E-postayı alan sunucunuzun authserv-id'si, ör. `mx.cloudflare.net`. DMARC sonuçlarının kullanılması için gerekir. |
| `YUVA_INGRESS_MAX_CONCURRENT` | `8` | `/ingress/email`'in süreç başına aynı anda işlediği mesaj sayısı; fazlası `503` alır. |
| `YUVA_SES_TOPIC_ARNS` | atanmamış | `/ingress/ses`'in SES geri dönme (bounce) ve şikâyet bildirimlerini kabul ettiği SNS topic ARN'leri. |
| `YUVA_EMAIL_SENDER_HOURLY_CAP` | `500` | Bir göndericinin bir çalışma alanına saatte gönderebileceği e-posta sayısı; fazlası reddedilir (`429`). |

## Sohbet

| Değişken | Varsayılan | Anlamı |
|---|---|---|
| `YUVA_CHAT_EMAIL_DELAY` | `5m` | Sohbetten ayrılmış bir kişiye, okunmamış bir yanıtın ne kadar süre sonra e-postayla gönderileceği. En az `1s`. |
| `YUVA_ANONYMOUS_CONTACTS_PER_HOUR` | `20` | IP adresi (IPv6 /64), kanal ve saat başına yeni anonim ziyaretçi sayısı; fazlası `429` alır. |

## Webhook'lar

| Değişken | Varsayılan | Anlamı |
|---|---|---|
| `YUVA_WEBHOOK_ALLOW_PRIVATE` | `false` | Webhook'ların loopback ve özel ağ adreslerine ulaşmasına izin verir. Yalnızca geliştirme için. |

## Web Push

| Değişken | Varsayılan | Anlamı |
|---|---|---|
| `YUVA_VAPID_PUBLIC_KEY` | atanmamış | VAPID genel anahtarı. İki anahtarı birlikte atayın ya da hiçbirini; `yuva vapid-keys` bir çift üretir. |
| `YUVA_VAPID_PRIVATE_KEY` | atanmamış | VAPID özel anahtarı. |
| `YUVA_VAPID_SUBJECT` | `mailto:` + `YUVA_SMTP_FROM` adresi, yoksa `https` ise `YUVA_PUBLIC_URL` | Push servisleri için iletişim adresi: `mailto:…` ya da `https://…`. |

## Derleme

Sunucunun bildirdiği sürüm (`GET /v1/version`), derleme sırasında Docker build argümanı `VERSION`
ile belirlenir. Ortam değişkeni değildir.
