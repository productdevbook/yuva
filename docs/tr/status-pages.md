# Durum sayfaları

Durum sayfası, müşterilerinize ürününüzün çalışıp çalışmadığını söyler: durumu olan bileşenler, olaylar,
planlı bakımlar ve geçmiş. Bir gelen kutusuna aittir; bu yüzden o gelen kutusunun canlı sohbeti ve
uygulamaları, müşteri size yazmadan önce açık bir olayı gösterir ve bir kesintinin getirdiği konuşmalar
toplu yanıtlanabilir.

Herkese açık sayfa, panelsiz ve zorunlu JavaScript olmadan, Yuva sunucunuz tarafından düz HTML olarak
üretilir. Her gelen kutusunun en fazla bir durum sayfası olur.

## 1. Sayfayı oluşturun ve yayınlayın

Panelde: Durum (kısayol `g s`), sonra gelen kutusunda Durum sayfası oluştur. Sayfaları sahipler ve
yöneticiler oluşturur ve değiştirir; gelen kutusuna erişimi olan temsilciler bileşenler ve olaylarla
çalışır.

| Ayar (API alanı) | Anlamı |
|---|---|
| Ad (`name`) | En fazla 100 karakter. |
| Açıklama (`description`) | En fazla 1.000 karakter, adın altında gösterilir. |
| Adres (`slug`) | 3 ile 48 karakter: `a-z`, `0-9` ve araya giren tireler. Sunucuda benzersiz; bazı sözcükler ayrılmıştır. |
| Logo (`logo_url`), vurgu rengi (`accent_color`) | Bir `https` görseli ve `#2563eb` gibi bir renk. |
| Koyu mod logosu (`logo_dark_url`) | İsteğe bağlı. Ziyaretçinin sistemi koyu moddayken logonun yerine gösterilen bir `https` görseli, örneğin koyu bir logonun açık renkli sürümü. Yalnızca bir logoyla birlikte kullanılır. |
| Site bağlantısı (`site_url`), destek bağlantısı (`support_url`) | Ürününüz ve müşterilerin yardım aldığı bir `http(s)` ya da `mailto:` bağlantısı. |
| Dil (`locale`) | `en`, `tr` ya da `de`; sayfanın sözcükleri buna uyar. Diğer diller İngilizce gösterir. |
| Saat dilimi (`timezone`) | Gün çubuklarının dilimi. Ziyaretçiler saatleri kendi dilimlerinde görür. |
| Geçmiş günü (`history_days`) | 7 ile 90, varsayılan 90. |
| Çalışma süresini göster (`show_uptime`) | Her bileşenin yanında bir yüzde. Varsayılan olarak açık. |
| Yanıt sürelerini göster (`show_response_times`) | `http` veya `tcp` izleyicisi olan bileşenlerin son 24 saatteki yanıt süreleri, medyan ve p95 ile. Varsayılan olarak kapalı. |
| Adı logonun yanında göster (`show_name`) | Varsayılan olarak açık. Logo adı zaten içeriyorsa kapatın: başlıkta yalnızca logo görünür ve ad, logonun alternatif metni olur. Logo yoksa ad her zaman görünür. |
| Akışı, rozeti ve yerleştirmeyi göster (`show_follow`) | Varsayılan olarak açık. Kapalıyken Takip et ve yerleştir bölümü, başlıktaki bağlantısı, sayfa altındaki `feed.atom` ve `badge.svg` bağlantıları ve olay sayfalarındaki akış bağlantısı gösterilmez. Akış, rozet ve JSON adresleri yanıt vermeye devam eder. |
| Yayında (`published`) | Varsayılan olarak kapalı. Yayınlanmamış bir sayfa ekibiniz dışında herkese `404` verir; ekibiniz onu panelde önizler. |

Adresi değiştirirseniz eskisi sayfaya ayrılmış kalır ve altındaki her herkese açık adres yenisine
yönlendirir.

API ile: `inbox_id`, `name` ve `slug` ile `POST /v1/status-pages`, sonra
`{"published": true}` ile `PATCH /v1/status-pages/{pageId}`.
`GET /v1/status-pages/{pageId}/preview` sayfayı, yayınlanmamışken de, ziyaretçilerin göreceği gibi verir.

## 2. Bileşenler, gruplar ve durumlar

Bileşenler, müşterilerin ürününüzde bel bağladığı parçalardır: "Web sitesi", "API", "Ödeme".
Bileşenler sekmesinde ekleyin, isterseniz gruplara koyun ve sıralayın. Bir sayfada en fazla 100 bileşen
olur. Gizli bir bileşen sayfada görünmez ve genel durumuna katılmaz.

| Durum | Anlamı |
|---|---|
| `operational` | Çalışıyor. |
| `degraded` | Yavaş ya da kısmen bozuk. |
| `partial_outage` | Kullanıcıların ya da özelliklerin bir kısmı çalışmıyor. |
| `major_outage` | Çalışmıyor. |
| `maintenance` | Planlı çalışma. |

Sayfanın genel durumu görünen bileşenlerin en kötüsüdür; `maintenance` yalnızca daha kötü bir durum
yokken sayılır. Bir günün çalışma süresi, günün `major_outage` dışında geçen payıdır; `partial_outage`
yarım sayılır, `maintenance` hesaba katılmaz. Gün çubuğu o günün en kötü durumunu gösterir.

Durumu panelde elle ya da `POST /v1/status-pages/{pageId}/components/{componentId}/state` ile
(`{"state": "degraded"}`) ayarlayın. Bir bileşeni açık bir olay, çalışmayan bir
[monitör](#5-heartbeat-ve-push-monitörleri) ya da siz tutabilirsiniz; panel kimin tuttuğunu söyler
("Bir olay tarafından tutuluyor", "Bir monitör ayarladı").

API ile: `/v1/status-pages/{pageId}/components` ve `/v1/status-pages/{pageId}/groups`.

## 3. Olaylar ve bakım

Olaylar sekmesinde Olay aç bir sorunu bildirir, Bakım planla bir çalışmayı duyurur. İkisinde de başlık,
dokundukları bileşenler (açıkken her birinin alacağı durumla) ve ilk güncelleme vardır.

| Tür | Etki | Durumlar |
|---|---|---|
| Olay (`incident`) | `none`, `minor`, `major`, `critical` | `investigating`, `identified`, `monitoring`, `resolved` |
| Bakım (`maintenance`) | aynı, varsayılan `none` | `scheduled`, `in_progress`, `completed` |

- **Taslak ya da yayında.** Taslak olarak kaydet, olayı yalnızca ekibe gösterir ve hiçbir bileşeni
  değiştirmez. Yayınla, olayı sayfada gösterir, bileşenleri ayarlar ve Abonelere bildir işaretini
  kaldırmadıysanız bir [bildirimi](#8-e-posta-aboneleri) sıraya alır. Yalnızca taslak silinebilir.
- **Güncellemeler.** Güncellemeyi gönder metin ekler ve durumu ilerletebilir. Metin düz yazıdır;
  bağlantılar ve satır sonları olabilir, en fazla 5.000 karakter. Yayınlanmış bir güncelleme kaldırılamaz;
  yeni bir güncellemeyle düzeltin ya da düzenleyin, "edited" işareti görünür.
- **Çözme.** Çözüldü say (ya da bakımı tamamlamak), dokunulan her bileşeni `operational` yapar; yalnızca
  başka bir açık olay ya da çalışmayan bir monitör hâlâ tutuyorsa yapmaz.
- **Bakım pencereleri.** Bakımın başlangıcı ve bitişi gerekir. Siz önce yapmadıysanız Yuva bakımı
  zamanında başlatır ve tamamlar; sayfanın dilinde "The maintenance has started." ve "The maintenance is
  complete." gibi bir metin ekler.

API ile: `POST /v1/status-pages/{pageId}/incidents` (taslağı atlamak için `"publish": true`),
`PATCH /v1/status-pages/{pageId}/incidents/{incidentId}`, `POST …/incidents/{incidentId}/publish`,
`POST …/incidents/{incidentId}/updates` ve `PATCH …/updates/{updateId}`. Çözülmüş bir olay başka
değişiklik almaz (`409 incident_closed`).

```sh
curl -X POST https://support.example.com/v1/status-pages/$PAGE/incidents \
  -H "Authorization: Bearer $YUVA_API_KEY" -H "Content-Type: application/json" \
  -d '{"title": "Checkout is failing", "message": "We are looking into it.", "impact": "major",
       "components": [{"component_id": "'$COMPONENT'", "state": "major_outage"}], "publish": true}'
```

## 4. Herkese açık sayfa

Yayınlanmış bir sayfa `<durum URL'si>/{slug}` adresindedir: genel durum, açık olaylar ve bakımlar, gün
çubuklu bileşenler ve son 14 günün olayları.

| Adres | Ne olduğu |
|---|---|
| `/status/{slug}` | Sayfa. |
| `/status/{slug}/history` | Geçmiş olaylar, aya göre. |
| `/status/{slug}/incidents/{id}` | Güncellemeleriyle tek bir olay ya da bakım. |
| `/status/{slug}/feed.atom` | Olayların Atom akışı. Abonelik gerektirmez. |
| `/status/{slug}/badge.svg` | Genel durum, README için rozet olarak. |
| `GET /client/v1/status/{slug}` | Aynısı JSON olarak, kendi siteniz ya da uygulamanız için. Her kaynağa açık, kimlik bilgisi yok. |
| `/status/{slug}/subscribe` | [E-posta abonelik formu](#8-e-posta-aboneleri). |

Durum URL'si `YUVA_PUBLIC_URL` artı `/status` olur, örneğin `https://support.example.com/status/acme`.
Sayfaları kendi alan adından sunmak için `YUVA_STATUS_URL` değerini `https://status.example.com` gibi bir
kaynağa ayarlayın; yollar o zaman `/status` öneki olmadan `https://status.example.com/acme`,
`/acme/history` ve benzeri olur, eski adresler oraya yönlenir. Alan adı Yuva sunucunuza ulaşmalı ve
`YUVA_PUBLIC_URL` alan adından farklı olmalıdır. Bkz. [Yapılandırma](configuration.md#durum-sayfaları).

Sayfa dakikada bir kendini yeniler ve JavaScript olmadan da eksiksizdir. Yanıtlar 30 saniye önbelleğe
alınır; böylece Yuva'nın önündeki bir CDN, ürününüz kapalıyken ziyaretçilere hizmet verebilir. Adres başına
dakikada 120 istek sınırı vardır.

Sayfayı gömmek için bir `iframe` kullanın; durumu başka yerde göstermek için rozeti ya da JSON'u:

```html
<iframe src="https://support.example.com/status/acme" title="Status" height="600"></iframe>
<img src="https://support.example.com/status/acme/badge.svg" alt="Status">
```

```sh
curl -s https://support.example.com/client/v1/status/acme
```

Bir widget ya da uygulamanın yüklediği gelen kutusu ayarları, bağlı sayfanın slug değerini `status.slug` olarak taşır; istemci bu JSON'u, özel alan adında da, slug'ı önceden bilmeden alabilir.

## 5. Heartbeat ve push monitörleri

Monitör, bir bileşenin durumunu kimse elini sürmeden ayarlar. Monitörler sekmesinde ekleyin (sahipler ve
yöneticiler): bileşeni ve türü seçin. Yuva monitörün adresini kopyalanabilir örneklerle bir kez gösterir.
Adresi bilen herkes monitör adına bildirim yapabilir; gizli tutun. Yeni adres oluştur adresi değiştirir ve
eskisi hemen çalışmaz olur.

### Heartbeat: işiniz Yuva'yı çağırır

Yuva'nın ulaşamadığı sunucular ve cron işleri için. İşiniz adresi en az her `interval` (30 saniye ile
30 gün, varsayılan 5 dakika) çağırır. `interval` artı `grace` (en çok bir gün, varsayılan 1 dakika)
boyunca gelmeyen bir sinyal monitörü başarısız yapar.

```sh
curl -fsS -m 10 --retry 3 -o /dev/null https://support.example.com/ingress/heartbeat/TOKEN
```

Crontab'ta yalnızca iş başarılı olduysa sinyal gönderin:

```sh
*/5 * * * * /path/to/job.sh && curl -fsS -m 10 --retry 3 -o /dev/null https://support.example.com/ingress/heartbeat/TOKEN
```

Başarısız bir çalışmayı, grace beklemeden hemen bildirin:

```sh
curl -fsS -m 10 -o /dev/null "https://support.example.com/ingress/heartbeat/TOKEN?fail=1"
```

`GET` ve `POST` aynı işi yapar; `/ingress/heartbeat/TOKEN/fail` ise `?fail=1` ile aynıdır.

### Push: izleme sisteminiz durumu gönderir

Düzelmek için `operational`, başarısız olmak için `degraded`, `partial_outage` ya da `major_outage`
gönderin. Gönderdiğiniz durum, bileşenin alacağı durumdur; `message` (en fazla 1.000 karakter) kontrolle
birlikte saklanır.

```sh
curl -fsS -X POST -H "Content-Type: application/json" \
  -d '{"state":"major_outage","message":"Checkout is failing"}' \
  https://support.example.com/ingress/status/TOKEN

curl -fsS -X POST -H "Content-Type: application/json" \
  -d '{"state":"operational"}' https://support.example.com/ingress/status/TOKEN
```

Alertmanager ve Grafana uyarıları olduğu gibi kabul edilir: firing monitörü başarısız yapar, resolved
düzeltir.

```yaml
receivers:
  - name: yuva-status
    webhook_configs:
      - url: https://support.example.com/ingress/status/TOKEN
        send_resolved: true
```

Grafana'da Webhook türünde bir contact point ekleyin, adresi yapıştırın, yöntemi `POST` bırakın ve contact
point'i bir notification policy'de kullanın.

### Ayarlar, eşikler ve elle yapılan değişiklikler

| Ayar (API alanı) | Anlamı |
|---|---|
| Düştüğündeki durum (`failure_state`) | `degraded`, `partial_outage` ya da `major_outage` (varsayılan). Kendi durumunu bildiren bir push o durumu korur. |
| Düştüğünde (`on_failure`) | Taslak olay açar (`draft`, varsayılan), yayınlanmış olay açar (`publish`) ya da açmaz (`none`). |
| Düşmek için hata sayısı (`failure_threshold`) | Monitör düşmeden önce üst üste başarısızlık sayısı, 1 ile 10, varsayılan 1. |
| Düzelmek için başarı sayısı (`recovery_threshold`) | Yeniden ayağa kalkmadan önce üst üste başarı sayısı, 1 ile 10, varsayılan 1. |
| Duraklatıldı (`paused`) | Sonuçları yok sayar ve bileşeni serbest bırakır. Devam ettirince `pending` durumundan başlar. |

Neyin başarısızlık olduğuna işiniz ya da izleme sisteminiz zaten karar verdiği için eşikler 1'den başlar.

Monitör sizin elinizi ezmez: bir üye ya da API anahtarı bileşenin durumunu monitörün son değişikliğinden
sonra ayarladıysa, monitör kendi durumu yeniden değişene kadar ona dokunmaz. Monitör düşükken olayı
bildirdiği durumu izler; düzelince olayı çözer ve başka bir şey tutmuyorsa bileşeni eski durumuna döndürür.
Monitörü duraklatmak ya da silmek de aynısını yapar. Panel, bir üyenin ayarladığı bileşeni tutulu olarak
gösterir.

Bir monitör düştüğünde ya da düzeldiğinde gelen kutusunun üyelerine yeni bir konuşma gibi bildirim gider ve
[webhook'lara](webhooks.md) `status.monitor.down` ile `status.monitor.up` gönderilir. Monitörler
sekmesi son kontrolleri listeler; bunlar 30 gün saklanır. Bir çalışma alanında en fazla
`YUVA_STATUS_MONITORS` monitör olur (varsayılan 20, `403 status_monitor_limit`). Bir token dakikada 60
çağrı alır.

API ile: `/v1/status-pages/{pageId}/monitors`, yeni adres için `POST …/monitors/{monitorId}/token` ve
`GET …/monitors/{monitorId}/checks`.

## 6. Widget'ta ve uygulamalarda durum

Sayfa yayındayken ve bir olay ya da bakım açıkken [web widget'ı](widget.md) ile iOS ve Android
[SDK'ları](mobile.md), konuşma listesinin ve mesaj kutusunun üstünde, durumu sözcüklerle ve olaya bir
bağlantıyla gösteren bir satır çıkarır. Önce en kötü açık olay gösterilir. Müşteri satırı kapatabilir;
olay yeni bir güncelleme alınca satır geri gelir. Kurulacak bir şey yok.

## 7. Konuşmaları bir olaya bağlayın

Bir kesinti çok konuşma getirir. Panelde bir konuşmayı açın ve bir olaya bağlayın (ya da
`PUT /v1/status-pages/{pageId}/incidents/{incidentId}/conversations/{conversationId}` kullanın). Olayın
sayfası bağlı konuşmaları ve durumlarını listeler; bir olaya en fazla 5.000 konuşma bağlanır.

Bağlılara yanıt ver, tek bir yanıt yazar ve bunu hâlâ açık olan her bağlı konuşmaya, sizin adınıza, her
konuşmanın kendi kanalından (e-posta, sohbet ya da uygulama) gönderir. Yanıttan sonra her konuşmayı
kapatmak için kutuyu işaretleyin. İncele, kaç konuşmanın yanıt alacağını gösterir. Gönderilen yanıt geri
alınamaz. Panel her konuşma için gönderildi, atlandı (kapalı, bağı kaldırılmış, silinmiş ya da erişiminiz
bitmiş) ya da başarısız oldu bilgisini verir.

API ile: `POST …/incidents/{incidentId}/reply` bir kimlikle `202` döner,
`GET …/incidents/{incidentId}/replies/{replyId}` sonucu bildirir. Anahtarın `status:manage`,
`conversations:read` ve `messages:write` kapsamları gerekir (kapatmak için ayrıca `conversations:write`);
çalışma alanı botların göndermesine izin vermelidir ([Botlar gönderebilir](headless.md#botlar-gönderebilir)).

## 8. E-posta aboneleri

Ziyaretçiler olay ve bakım bildirimlerine abone olabilir. Sayfanın Ayarlar sekmesinde açın: Bildirimleri e-postayla gönder
ve göndermeyi yapacak e-posta kanalı. Kanalın gönderebiliyor olması gerekir (aksi hâlde
`409 sender_not_ready`). Gönderen alan adı teslim edilebilirlik denetimini geçmediyse, gelen kutusunda
posta adresi yoksa ya da posta gönderenin kendi alan adıyla DKIM imzalı değilse sayfa engellemeden
uyarır; bkz. [Teslim edilebilirlik](deliverability.md).

- **Abone olma.** Sayfa abonelik formuna bağlanır: bir adres, bileşen başına bir kutu (hepsi işaretli;
  yalnızca bazılarını duymak için işareti kaldırın) ve ziyaretçinin kabul ettiği cümle. Her zaman çift
  onay kullanılır: yanıt her adres için aynıdır ve ardından bir onay e-postası gelir. Bileşen seçmeyen
  abone, sonradan eklediğiniz bileşenler dahil her şeyi alır. Aboneler seçimlerini değiştirir ya da
  ayrılır; bunun için her e-postadaki bağlantıyı kullanır.
- **Bildirimde ne var.** Sözcüklerle durum (Investigating, Identified, Monitoring, Resolved, Scheduled,
  In progress, Completed), etki, durumlarıyla etkilenen bileşenler, son bildirimden beri gelen
  güncellemeler (sayfanın saat diliminde saatlerle), olaya bir bağlantı ve bileşenleri seçmek ya da
  abonelikten çıkmak için bir bağlantı. Abonenin dilindedir. Konu `[Sayfa] Başlık` ve
  `[Sayfa] Maintenance: Başlık` biçimindedir; bir olayın bildirimleri aynı konuyu ve aynı iş parçacığını
  paylaşır. Görsel, izleme ya da tanıtım içermez. Bildirimler gelen kutusunun e-posta kanalından, onun
  adresiyle gider.
- **Ne zaman gider.** Yayınlanan yeni bir olay varsayılan olarak bildirir; bir güncelleme durumu ya da
  pencereyi değiştirirse veya olayı çözerse bildirir, yalnızca metin eklerse bildirmez. Bunu olaydaki ya da
  güncellemedeki Abonelere bildir kutusuyla (kaç kişiye ulaşacağını gösterir) ya da API'de `notify` ile
  değiştirin. Yalnızca bileşen değişiklikleri ve taslaklar hiçbir şey göndermez.
- **Bekletme ve geri alma.** Bildirim 60 saniye bekler; yazım hatasını düzeltebilirsiniz ve bu sürede
  gönderdiğiniz güncellemeler ona katılır. Olayın zaman çizelgesindeki Geri al (ya da
  `POST /v1/status-pages/{pageId}/notices/{noticeId}/cancel`) bildirimi gitmeden durdurur (sonrasında
  `409 not_pending`).
- **Aralık ve saatlik sınır.** Bir olayın bildirimleri arasında en az 15 dakika olur ve bir sayfa saatte
  en fazla 10 bildirim gönderir; fazlası atlanır ve zaman çizelgesi bunu söyler.
- **Son bildirimler.** Olayın çözüldüğünü ya da bakımın başladığını veya tamamlandığını bildiren bildirim,
  aralığı ve saatlik sınırı yok sayar. Yalnızca o olayın daha önceki bir bildirimini almış kişilere gider;
  öncesinde hiçbiri gönderilmediyse atlanır (`nothing_earlier`), çünkü olayı kimse duymamıştır.
- **Sınırlar.** Sunucuda `YUVA_STATUS_NOTICES`, `YUVA_STATUS_NOTICES_DAILY` ve
  `YUVA_STATUS_MAX_SUBSCRIBERS`; bkz. [Yapılandırma](configuration.md#durum-sayfaları). Abone olma,
  ziyaretçi adresi başına saatte 10 ve sayfa başına saatte 300 ile sınırlıdır. Onaylanmamış abonelikler
  30 gün sonra silinir.

Aboneler sekmesi adresleri durumlarıyla ve seçtikleri bileşenlerle listeler, bir adres ekler (onay e-postası
gönderir), bir adresi kaldırır ve izin kayıtlarını dışa aktarır. Aboneler, gelen kutusunun `status` türünde
bir e-posta listesine aittir; bu liste formlarda, bültenlerde ya da CSV içe aktarmalarında asla sunulmaz ve
ondan ayrılmak kişiyi başka hiçbir şeyden çıkarmaz. Sayfanın bildirimleri
`GET /v1/status-pages/{pageId}/notices` adresindedir.

## API, webhook'lar ve MCP

| Kapsam | İzin verdiği |
|---|---|
| `status:read` | Sayfalar, bileşenler, olaylar, bildirimler, monitörler ve kontrolleri. |
| `status:manage` | Bunların hepsini oluşturma ve değiştirme, güncelleme gönderme, durum ayarlama, konuşma bağlama. `status:read` içermez: bir anahtara ikisini de verin. |

Sayfalar, gruplar, bileşenler ve monitörler sahipler ve yöneticiler içindir; kapsamı olan temsilciler ve
anahtarlar, görebildikleri gelen kutularının durumlarıyla, olaylarıyla ve bildirimleriyle de çalışır. Tüm
yüzey [openapi.yaml](../../openapi/openapi.yaml) içindeki `status` bölümündedir.

[Webhook'lar](webhooks.md) `status.component.updated`, `status.incident.created`,
`status.incident.updated`, `status.incident.resolved`, `status.monitor.down` ve `status.monitor.up`
olaylarına abone olabilir. Taslaklar webhook'lara gönderilmez.

[Yapay zekâ asistanları (MCP)](mcp.md) `list_status_pages`, `get_status_page`, `list_monitors`,
`set_component_state`, `create_incident` (her zaman bir üyenin yayınladığı taslak), `post_incident_update`,
`resolve_incident` ve `link_conversation_to_incident` araçlarını alır.
