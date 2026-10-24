# Kurulum

Yuva'yı tek bir sunucuda Docker Compose ile ya da Kubernetes'te [Helm chart](#helm-ile-kubernetes) ile, HTTPS sunan bir ters vekil sunucunun (reverse proxy)
arkasında çalıştırın. Gerekenler: Docker (ya da bir küme), TLS sertifikalı bir alan adı ve giriş kodları için bir
SMTP hesabı.

> [!WARNING]
> Yuva beta aşamasında. Gerçek konuşmaları taşımadan önce [README](../../README.md)'deki
> uyarıyı okuyun.

## Compose ile hızlı başlangıç

### 1. `compose.yaml` dosyasını yazın

Sunucuda bir dizin açın, örneğin `/opt/yuva`, ve bu dosyayı içine koyun:

```yaml
name: yuva

services:
  db:
    image: postgres:18
    environment:
      POSTGRES_USER: yuva
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:?set POSTGRES_PASSWORD in .env}
      POSTGRES_DB: yuva
    volumes:
      - db:/var/lib/postgresql
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U yuva -d yuva"]
      interval: 5s
      timeout: 3s
      retries: 30
    restart: unless-stopped

  yuva:
    image: ghcr.io/productdevbook/yuva:0.2.0
    env_file: .env
    environment:
      YUVA_DATABASE_URL: postgres://yuva:${POSTGRES_PASSWORD}@db:5432/yuva?sslmode=disable
      YUVA_STORAGE: local
      YUVA_STORAGE_DIR: /data/attachments
    ports:
      - "127.0.0.1:8080:8080"
    volumes:
      - attachments:/data
    depends_on:
      db:
        condition: service_healthy
    restart: unless-stopped

volumes:
  db:
  attachments:
```

### 2. `.env` dosyasını yazın

`compose.yaml` dosyasının yanına koyun ve `chmod 600 .env` çalıştırın. Her `<openssl rand …>`
yerine o komutun çıktısını yazın:

```sh
POSTGRES_PASSWORD=<openssl rand -hex 24>
YUVA_PUBLIC_URL=https://support.example.com
YUVA_MASTER_KEY=<openssl rand -base64 32>
YUVA_INGRESS_SECRET=<openssl rand -hex 32>
YUVA_CLIENT_IP_HEADER=X-Real-IP
YUVA_TRUSTED_PROXIES=172.16.0.0/12

YUVA_SMTP_HOST=smtp.example.com
YUVA_SMTP_USERNAME=yuva@example.com
YUVA_SMTP_PASSWORD=<password>
YUVA_SMTP_FROM=Yuva <yuva@example.com>
```

`YUVA_MASTER_KEY` değerinin bir kopyasını sunucunun dışında saklayın ([Ana anahtar](#ana-anahtar)).

### 3. Başlatın

```sh
docker run --rm ghcr.io/productdevbook/yuva:0.2.0 vapid-keys >> .env
docker compose up -d
curl -s http://127.0.0.1:8080/readyz     # {"status":"ok"}
```

İlk satır Web Push anahtarlarını ekler. Sunucu başlamadan önce veritabanı migration'larını
çalıştırır.

### 4. Sahip hesabını oluşturun

`YUVA_SIGNUP=open` ayarlamadıkça kayıt yalnızca davetle olur ([yapılandırma](configuration.md)); ilk sahibi oluşturun:

```sh
docker compose exec yuva /yuva bootstrap --email you@example.com --workspace "Example" --name "Your Name"
```

### 5. HTTPS arkasına alın

Ters vekil sunucunuzu `127.0.0.1:8080` adresine yönlendirin. Caddy ile:

```
support.example.com {
	reverse_proxy 127.0.0.1:8080 {
		header_up X-Real-IP {remote_host}
	}
}
```

`YUVA_PUBLIC_URL` adresini açın, e-posta adresinizi girin ve gelen e-postadaki kodla giriş yapın.
SMTP yoksa kod `docker compose logs yuva` çıktısındadır.

### 6. İhtiyacınız olanı kurun

İlk sahip **Ana sayfa**'ya (`/`) düşer; orada henüz kurulmayanlar listelenir. Bu listeden
**Yuva'yı kur** sayfasını (`/setup`) açın. Altı kart vardır; istediklerinizi seçin, her biri birkaç
dakika sürer ve istediğiniz zaman geri dönebilirsiniz:

| Kart | Ne yapar | Rehber |
|---|---|---|
| Web sitesi sohbeti | Web sitenizde sohbet | [Web widget'ı](widget.md) |
| E-posta | Konuşmaya dönüşen bir destek adresi | [E-posta](email.md) |
| Mobil uygulama | Uygulamanızın içinde sohbet | [Mobil SDK'lar](mobile.md) |
| Dokümantasyon sitesi | Dokümantasyonunuzda puanlama, geri bildirim ve tartışma | [Dokümantasyon sayfaları](documentation-pages.md) |
| Analitik | Bir web sitesinin ya da uygulamanın ziyaretçileri, çerezsiz | [Analitik](analytics.md) |
| Bülten | Abonelik formlu bir posta listesi | [Listeler](lists.md) |

Kurulmuş bir kart kaç tane olduğunu gösterir ve değiştireceğiniz yeri açar. Yuva kurulumu kendiliğinden
açmaz; **Sonra yaparım** sizi bulunduğunuz yere döndürür. Sonra ekibinizi davet edin.

Sahipler ve yöneticiler Ana sayfa'yı `/` adresinde açar (kısayol `g h`): bugünün gelen kutusu, her
web sitesi ve uygulama için bugünkü, son 30 dakikadaki ve yedi günlük ziyaretçiler, son bülten ve kurulum
listesi. Temsilciler `/` adresinde gelen kutusunu açar; gelen kutusu `/conversations` adresindedir de.
Analitik kurulumunun kısayolu `/analytics/new` hâlâ çalışır.

## Helm ile Kubernetes

Yuva'nın bir Helm chart'ı var; her sürümle sunucunun sürüm numarasıyla `oci://ghcr.io/productdevbook/charts/yuva` adresinde yayımlanır. Veritabanı getirmez: yönetilen bir Postgres 16 veya üstünü ya da [CloudNativePG](https://cloudnative-pg.io) gibi bir operatörün çalıştırdığı birini kullanın.

### 1. Secret oluşturun

Veritabanı adresini ve ana anahtarı ([Ana anahtar](#ana-anahtar)) ingress sırrı ve SMTP girişiyle birlikte bir Secret içine koyun. S3 anahtarları ve `yuva vapid-keys` çıktısındaki Web Push anahtarları aynı Secret içinde `YUVA_S3_ACCESS_KEY_ID`, `YUVA_S3_SECRET_ACCESS_KEY`, `YUVA_VAPID_PUBLIC_KEY` ve `YUVA_VAPID_PRIVATE_KEY` adlarıyla yer alır:

```sh
kubectl create namespace yuva
kubectl -n yuva create secret generic yuva \
  --from-literal=YUVA_DATABASE_URL='postgres://yuva:<password>@<host>:5432/yuva?sslmode=require' \
  --from-literal=YUVA_MASTER_KEY="$(openssl rand -base64 32)" \
  --from-literal=YUVA_INGRESS_SECRET="$(openssl rand -hex 32)" \
  --from-literal=YUVA_SMTP_USERNAME='yuva@example.com' \
  --from-literal=YUVA_SMTP_PASSWORD='<password>'
```

### 2. `values.yaml` dosyasını yazın

`publicUrl` değerini verin, `secrets.existingSecret` ile Secret'ı gösterin, diğer [ayarları](configuration.md) `env` altına koyun. Aşağıdaki ingress sınıfı ve açıklamaları ingress-nginx içindir; kullandığınız ingress en az 26 MiB gövdeli istekleri kabul etmeli ve WebSocket'leri geçirmelidir ([Ters vekil sunucu ve TLS](#ters-vekil-sunucu-ve-tls)):

```yaml
publicUrl: https://support.example.com
secrets:
  existingSecret: yuva
env:
  YUVA_CLIENT_IP_HEADER: X-Real-IP
  YUVA_TRUSTED_PROXIES: 10.0.0.0/8
  YUVA_SMTP_HOST: smtp.example.com
  YUVA_SMTP_FROM: Yuva <yuva@example.com>
ingress:
  enabled: true
  className: nginx
  annotations:
    nginx.ingress.kubernetes.io/proxy-body-size: 30m
    nginx.ingress.kubernetes.io/proxy-read-timeout: "120"
    nginx.ingress.kubernetes.io/proxy-send-timeout: "120"
  tls:
    - secretName: yuva-tls
      hosts: [support.example.com]
```

### 3. Kurun

```sh
helm install yuva oci://ghcr.io/productdevbook/charts/yuva --version 0.2.0 -n yuva -f values.yaml
```

### 4. Sahip hesabını oluşturun

```sh
kubectl -n yuva exec deploy/yuva -c yuva -- \
  /yuva bootstrap --email you@example.com --workspace "Example" --name "Your Name"
```

Veritabanı geçişleri her pod başlamadan önce bir init container içinde çalışır. Ekler varsayılan olarak bir birimde tutulur; bu yalnızca tek kopyaya izin verir. Birden çok kopya için S3 kullanın (`storage.driver: s3` ve `YUVA_S3_*` [ayarları](configuration.md)). Yük dengeleyiciniz yapışkan oturum (sticky session) çerezi koyuyorsa `service.analytics.separateService: true` yapın; böylece analitik betiği ve uç noktası hiç çerez almaz. Tüm değerler chart'ın [README](https://github.com/productdevbook/yuva/blob/main/deploy/helm/yuva/README.md) dosyasındadır.

## Başvuru

### Gereksinimler

| Ne | Not |
|---|---|
| Postgres 16 veya üstü | Her şeyi tutar. Redis yok. Veritabanı, e-posta aramasının kullandığı `pg_trgm` eklentisine izin vermelidir: eklenti PostgreSQL 13'ten beri güvenilirdir, bu yüzden Yuva veritabanı kullanıcısının veritabanında yalnızca `CREATE` yetkisi olması yeter; yönetilen Postgres hizmetleri eklentiyi hazır sunar. |
| SMTP hesabı | Giriş kodları ve bildirimler için. Yoksa yalnızca giriş kodları loga yazılır, başka e-posta gönderilmez. |
| Ek depolama | Bir Docker volume'u ya da `YUVA_STORAGE=s3` ve `YUVA_S3_*` [ayarlarıyla](configuration.md#ekler-ve-depolama) S3. |
| TLS'li alan adı | Geçiş anahtarları ve Web Push HTTPS ister. |

### Ters vekil sunucu ve TLS

Yuva 8080 portunda düz HTTP konuşur ve her şeyi tek bir alan adının kökünden sunar. Metrik portu
9090'ı dışarı açmayın. Vekil sunucu şunları yapmalı:

- `/v1/realtime` ve `/client/v1/realtime` üzerindeki WebSocket upgrade'lerini geçirmeli;
- boşta bekleyen bağlantıları 60 saniyeden uzun açık tutmalı;
- en az 26 MiB'lık istek gövdelerini kabul etmeli;
- `YUVA_CLIENT_IP_HEADER` ile adı verilen başlığın üzerine istemci adresini yazmalı;
- `Host` ve `Origin` başlıklarını korumalı.

Vekil sunucunuzun adreslerini `YUVA_TRUSTED_PROXIES` içinde de listeleyin. Bu olmadan başlık,
gönderen herkesten okunur; sunucuya doğrudan ulaşan biri kendi hız sınırı kimliğini kendi seçer.

Yukarıdaki Caddy yapılandırması bunların hepsini yapar. Cloudflare proxy'si arkasında
`YUVA_CLIENT_IP_HEADER=CF-Connecting-IP` kullanın. nginx:

```nginx
server {
    listen 443 ssl;
    http2 on;
    server_name support.example.com;
    ssl_certificate     /etc/ssl/support.example.com/fullchain.pem;
    ssl_certificate_key /etc/ssl/support.example.com/privkey.pem;

    client_max_body_size 30m;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
        proxy_read_timeout 120s;
        proxy_buffering off;
    }
}

map $http_upgrade $connection_upgrade {
    default upgrade;
    ''      close;
}
```

### Genel adres

`YUVA_PUBLIC_URL` panelin, widget'ın, API'nin ve e-postalardaki bağlantıların adresidir. Ekibiniz
geçiş anahtarı eklemeden önce kararlaştırın; geçiş anahtarları bu adresin alan adına bağlıdır
([Yapılandırma](configuration.md#giriş-ve-geçiş-anahtarları)).

### Ana anahtar

`YUVA_MASTER_KEY`, Yuva'nın sakladığı gizli bilgileri şifreler: e-posta kanallarının SMTP
parolaları, gelen kutularının kimlik anahtarları ve webhook imzalama anahtarları. Bir kez
`openssl rand -base64 32` ile üretin ve veritabanından ayrı yedekleyin, örneğin bir parola
yöneticisinde.

Anahtar olmadan geri yüklenen bir veritabanı bu bilgileri kaybeder: e-posta durur, uygulamalar
kullanıcıları giriş yaptıramaz ve webhook'lar her biri yeniden girilene kadar imzasız gider.
Anahtarı değiştirmek de aynı sonucu doğurur; [Ana anahtarı döndürme](operations.md#ana-anahtarı-döndürme)
bölümündeki gibi döndürmediğiniz sürece.

### VAPID anahtarları

Üyelere anlık bildirim gönderebilmek için `yuva vapid-keys` çıktısındaki anahtar çifti `.env`
dosyasında olmalı. Yoksa üyeler yalnızca e-posta bildirimi alır. Çifti saklayın: değişirse üyelerin
bildirimleri yeniden açması gerekir.

### Docker imajı

`ghcr.io/productdevbook/yuva:<version>`, `linux/amd64` ve `linux/arm64` üzerinde root olmayan bir
kullanıcıyla, `8080` (HTTP) ve `9090` (metrikler) portlarıyla çalışır. Giriş noktası `yuva`
programıdır; [operatör komutlarını](operations.md#operatör-komutları) da o çalıştırır. İmajı kendiniz
derlemek için sürüm etiketini checkout edip şunu çalıştırın:

```sh
docker build -f deploy/Dockerfile --build-arg VERSION=0.2.0 -t ghcr.io/productdevbook/yuva:0.2.0 .
```

Depodaki `deploy/compose.yaml` yalnızca geliştirme içindir. Production'da çalıştırmayın. Tüm portları
yalnızca `127.0.0.1` üzerinde yayınlar; `YUVA_PUBLIC_URL` `localhost` ya da bir loopback adresi değilse
sunucu onun sabit, herkese açık sırlarını (ana anahtar, ingress sırrı, VAPID özel anahtarı) reddeder.
