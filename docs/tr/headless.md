# Headless

Panelin ve widget'ın yaptığı her şey kendi kodunuza da açıktır: `/v1` üzerinde kendi gelen kutunuz,
kişi API'si üzerinde kendi sohbetiniz ya da taslaklarla yanıt veren bir bot. Çalışan kod
[`examples/`](../../examples) dizininde, MIT lisanslı.

## API anahtarları

Kodunuz `/v1` ile bir API anahtarıyla konuşur. Sahipler ve yöneticiler anahtarları Ayarlar → API
anahtarları altında ya da sunucuda oluşturur:

```sh
yuva api-key create --workspace <id|name> --name "Support bot" --scope conversations:read --scope messages:write
```

Bir anahtara yalnızca gereken [yetkileri](#yetkiler) verin (`--scope` yoksa hepsi verilir);
`--inbox <id>` onu bazı gelen kutularıyla sınırlar. Her anahtar, adı yazdığı her şeyde görünen bir
bottur. Anahtarları sunucularınızda tutun.

## Kendi gelen kutusu arayüzünüz

Konuşmaları tipli istemciyle okuyup yazın (`npm install useyuva`):

```ts
import { createYuvaApi } from "useyuva/api";

const api = createYuvaApi({ server: "https://support.example.com", apiKey: process.env.YUVA_API_KEY! });
const { data } = await api.GET("/v1/conversations", { params: { query: { status: ["open"] } } });
```

Go ile: `github.com/productdevbook/yuva/sdk/go/client` ([README](../../sdk/go/README.md)). Diğer
diller: [OpenAPI sözleşmesi](../../openapi/openapi.yaml). Ekibiniz için bir arayüz,
[OAuth](mcp.md#oauth) ile her üyenin yerine hareket edebilir.

Değişiklikleri izlemenin iki yolu var:

- **Olay akışını yoklayın.** `GET /v1/events/latest` ile başlayın, sonra
  `GET /v1/events?after=<position>&limit=100` çağırın, `events`'i işleyin, `next`'i saklayın ve
  `has_more` true olduğu sürece hemen tekrarlayın. Bkz. [`examples/inbox-feed`](../../examples/inbox-feed).
- **Bir WebSocket açık tutun:** `Authorization: Bearer <key>` ile `GET /v1/realtime`. Kaçırdıklarınızı
  almak için `?last_event_id=<last id handled>` ile yeniden bağlanın.

## Kendi sohbet arayüzünüz

Kişiler, API anahtarıyla değil kendi oturumlarıyla kişi API'sini (`/client/v1`) kullanır.
`createYuvaClient()` bunu DOM olmadan sarar; tarayıcılar, Node 22+, Bun, Deno ve worker'lar için:

```ts
import { createYuvaClient } from "useyuva";

const yuva = createYuvaClient({
  server: "https://support.example.com",
  channel: "yuva_pk_…",
  identityToken: () => fetch("/my/yuva-token").then((r) => r.text()),
});

yuva.on("event", (event) => console.log(event.type));
await yuva.connect();
const { conversation } = await yuva.startConversation({ body: "Hello" });
```

Bir üyenin sildiği konuşma `/client/v1` içinde görünmez olur ve gerçek zamanlı akış bir `conversation.removed`
çerçevesi (`data: { id }`) gönderir: konuşmayı listenizden çıkarın, açıksa kapatın. Bu kimliğe gönderilen
mesaj yeni bir konuşma başlatır; yanıttaki `conversation_id` onu belirtir. Konuşma geri alınırsa yeniden
`conversation.created` olarak gelir. `createYuvaClient()` onu
`conversationRemoved` olayı (`{ id }`) olarak yayar, React kancaları konuşmayı listeden çıkarır; Swift ve Kotlin
SDK'ları `YuvaEvent.conversationRemoved(id:)` / `YuvaEvent.ConversationRemoved(id)` gönderir.

Bir üye [bir mesajı kaldırdığında](#bir-mesajı-kaldırma) akış bir `message.redacted` çerçevesi gönderir; `data`,
`redacted: true`, boş bir `body` ve eksiz mesajdır: mesajı yerinde değiştirin ve "Bu mesaj kaldırıldı" gösterin.
Aynı mesaj sonraki bir tekrar oynatmada veya listede de `redacted: true` taşır.
`createYuvaClient()` çerçeveyi `event` olayında iletir, React kancaları mesajı günceller; Swift ve Kotlin SDK'ları
`YuvaEvent.messageRedacted(_:)` / `YuvaEvent.MessageRedacted(message)` gönderir.

Tarayıcı dışında bir `app` kanalının anahtarını kullanın; `chat` kanalları yalnızca izin verilen
origin'leri kabul eder. React hook'ları `useyuva/react` içinde; tüm seçenekler
[SDK README](../../sdk/js/README.md)'sinde.

## Taslakla yanıtlayan bot

1. Botunuzu gösteren, `message.created` için bir [webhook](webhooks.md) ekleyin.
2. [İmzayı doğrulayın](webhooks.md#webhook-imzasını-doğrulayın) ve yalnızca `direction: "in"` ve
   `kind: "message"` olan `data.message` üzerinde işlem yapın.
3. Yanıtı taslak olarak gönderin. Gelen mesajdan türetilen bir `Idempotency-Key`, yeniden denenen bir
   webhook'un ikinci bir taslak oluşturmasını engeller:

```sh
curl -X POST https://support.example.com/v1/conversations/$CONVERSATION/messages \
  -H "Authorization: Bearer $YUVA_API_KEY" -H "Content-Type: application/json" \
  -H "Idempotency-Key: draft-bot:$INCOMING_MESSAGE_ID" \
  -d '{"kind": "message", "direction": "out", "draft": true, "body": "Thanks, we are on it."}'
```

Bir üye taslağı botun adıyla görür ve gönderir, düzenler ya da atar. Bkz.
[`examples/draft-bot`](../../examples/draft-bot).

### Botlar gönderebilir

**Botlar gönderebilir** çalışma alanı ayarı (Ayarlar → API anahtarları) varsayılan olarak kapalıdır:
bu durumda bir anahtarın giden mesajı taslak olmak zorundadır (yoksa `403 bot_sending_disabled`).
Açıkken `messages:write` yetkili bir anahtar doğrudan yanıt verir, `drafts:send` ile taslakları da
gönderir. Bunu yalnızca panelde oturum açmış bir sahip ya da yönetici değiştirebilir
(`PATCH /v1/workspace` üzerinde `bots_may_send`), anahtarlar asla.

## Ekipler

Ekip, gelen kutularına erişim verilen, adı olan bir üye grubudur. Bir üye bir gelen kutusuna kendi
yetkisiyle, ait olduğu herhangi bir ekiple ya da ikisiyle ulaşır: erişim bunların birleşimidir; yani
başka bir yetki gelen kutusunu vermeye devam ederken bir yetkiyi kaldırmak hiçbir şeyi değiştirmez.
Bir konuşma bir üyeye olduğu gibi bir ekibe de atanabilir.

```sh
curl -X POST https://support.example.com/v1/teams \
  -H "Authorization: Bearer $YUVA_API_KEY" -H "Content-Type: application/json" \
  -d '{"name": "Billing", "description": "Invoices and refunds"}'
curl -X PUT https://support.example.com/v1/teams/$TEAM/inboxes/$INBOX -H "Authorization: Bearer $YUVA_API_KEY"
curl -X PUT https://support.example.com/v1/teams/$TEAM/members/$MEMBER -H "Authorization: Bearer $YUVA_API_KEY"
```

| Çağrı | Ne yapar |
|---|---|
| `GET /v1/teams` | Ekipleri ada göre listeler: `{"items": [...]}`. |
| `POST /v1/teams` | Ekip oluşturur (`name` 1 ila 64 karakter ve benzersiz: `409 team_taken`; isteğe bağlı `description`, en fazla 500). `201`. |
| `GET`, `PATCH`, `DELETE /v1/teams/{teamId}` | Ekibi okur, adını veya açıklamasını değiştirir (`PATCH` en az bir alan ister) ya da siler. |
| `PUT`, `DELETE /v1/teams/{teamId}/members/{memberId}` | Üye ekler veya çıkarır. `204`; tekrarı hiçbir şeyi değiştirmez. |
| `PUT`, `DELETE /v1/teams/{teamId}/inboxes/{inboxId}` | Gelen kutusu yetkisi verir veya kaldırır. `204`; tekrarı hiçbir şeyi değiştirmez. |
| `GET /v1/inboxes/{inboxId}/members` | Bir gelen kutusuna kimlerin erişimi var. Her üyede `via` (`direct`, ya da erişimi yalnızca ekipler veriyorsa `team`) ve gelen kutusunu veren ekipler olan `team_ids` bulunur. |

Bir ekipte `id`, `name`, `description`, `member_ids`, `inbox_ids` (ikisi de eklenme sırasıyla),
`created_at` ve `updated_at` bulunur. Ekip başına en fazla 500 üye ve 100 gelen kutusu (`403 team_limit`);
bilinmeyen bir ekip, üye veya gelen kutusu `404` verir.

Okumak için `inboxes:read`, yazmak için `inboxes:manage` ve bir sahip, yönetici ya da gelen kutularıyla
sınırlı olmayan bir anahtar gerekir (temsilciler `403 forbidden` alır). Sahipler, yöneticiler ve
sınırsız anahtarlar her ekibi görür; temsilci üyesi olduğu ekipleri ve gördüğü bir gelen kutusuna yetkili
ekipleri görür; gelen kutularıyla sınırlı bir anahtar, o gelen kutularından birine yetkili ekipleri görür.
Göremediğiniz ekip `404` verir ve aldığınız `inbox_ids` yalnızca görebildiğiniz gelen kutularını listeler.

Bir üyenin doğrudan yetkisini kaldırmak (`DELETE /v1/inboxes/{inboxId}/members/{memberId}`) bir ekibin ona
verdiğini kaldırmaz; bunun yerine üyeyi ekipten çıkarın ya da ekibin gelen kutusu yetkisini kaldırın.

### Bir ekibe atama

`PATCH /v1/conversations/{conversationId}`, `assignee_id` yanında `team_id` alır (bir ekip kimliği ya da
temizlemek için `null`). Ekibin konuşmanın gelen kutusuna yetkisi olmalıdır; yoksa çağrı
`422 team_access` verir; çağrı sürerken silinen ya da yetkisi alınan ekip de aynısını verir.
`POST /v1/conversations/{conversationId}/move` ile taşınan bir konuşma, ekibini yalnızca ekibin hedef
gelen kutusuna yetkisi varsa korur. Bir ekibi silmek ya da gelen kutusu yetkisini kaldırmak, ilgili
konuşmalarda `team_id` değerini temizler; her biri için bir `conversation.updated` gönderilir.
Konuşmada ekip varsa `team_id` bulunur; bu ayrı bir alandır ve `assignee=unassigned` ekip ne olursa olsun
hâlâ "üye yok" demektir. Toplu güncelleme ve `POST /v1/conversations` ekip almaz.

| Liste filtresi | Gösterir |
|---|---|
| `GET /v1/conversations?team_id=<id>` | Bu ekibe atanmış konuşmalar. |
| `GET /v1/conversations?team=mine` | İstek yapan üyenin bir ekibine atanmış konuşmalar. Yalnızca üye oturumları; anahtar `400` alır. |

`GET /v1/conversations/counts` içinde `teams` (spam olmayan açık konuşmalar, ekip kimliği başına) ve
`mine_teams` (isteği yapanın herhangi bir ekibine atanmış açık konuşmalar; anahtarlar için `0`) bulunur.

### Olaylar

- `team.created`, `team.updated` ve `team.deleted`, [olay akışına ve gerçek zamanlı bağlantıya](#olay-akışı-ve-gerçek-zamanlı-bağlantı)
  ve [webhook'lara](webhooks.md#olaylar) gider. `team.updated`; ad, açıklama ve her üye ya da gelen kutusu
  değişikliğinden sonra gelir; ekipten çıkarılan üyeler `removed_member_ids` içindedir. `inboxes:read`
  gerekir ve her gelen kutusunu gören kullanıcılara, ekibin üyelerine ve gelen kutularından birini gören
  kullanıcılara ulaşır.
- `inbox_access.changed`, erişimi değişen her üye ve gelen kutusu için bir kez gönderilir; neden doğrudan
  bir yetki, bir ekip ya da ekibin kaldırılması olabilir. Erişimi aynı bırakan bir değişiklik olay göndermez.
- Bir ekibe atama, yükünde `team_id` taşıyan `conversation.updated` içinde görünür.
- [Denetim günlüğü](operations.md#denetim-günlüğü) `team.created`, `team.deleted`, `team.member_added`,
  `team.member_removed`, `team.inbox_access_granted` ve `team.inbox_access_revoked` kayıtlarını tutar.

[MCP](mcp.md) asistanları `list_teams`, `team_id` veya `remove_team` ile `assign` ve `team_id` ile
`search_conversations` kullanır.

## SLA politikaları

Mesai saatlerine göre yanıt süresi hedefleri, uyarılar ve ihlaller: [SLA politikaları](sla.md). Bunları
`GET`, `POST /v1/sla-policies`, `GET`, `PATCH`, `DELETE /v1/sla-policies/{slaPolicyId}` ve
`PUT /v1/sla-policies/order` ile yönetin (okumak için `inboxes:read`, yazmak için `inboxes:manage`; sahipler,
yöneticiler ve gelen kutularıyla sınırlı olmayan anahtarlar). Konuşmalar `sla` taşır, liste `sort=sla_due` ve
`sla_state` alır, `conversation.sla_warning` ile `conversation.sla_breached` olay akışına ve webhook'lara ulaşır.

## Kayıtlı görünümler

Kayıtlı görünüm, konuşma listesi filtrelerinin adlandırılmış bir kümesidir. Kişisel görünümler tek bir üyeye aittir;
çalışma alanı görünümleri herkesle, ekip görünümleri bir ekiple paylaşılır.

```sh
curl -X POST https://support.example.com/v1/views \
  -H "Authorization: Bearer $YUVA_API_KEY" -H "Content-Type: application/json" \
  -d '{"visibility": "workspace", "name": "Open Pro refunds",
       "filters": {"status": "open,pending", "q": "refund -spam", "field.plan": "pro"}}'
```

| Çağrı | Ne yapar |
|---|---|
| `GET /v1/views` | Kişisel görünümleriniz, çalışma alanının görünümleri ve ekiplerinizin görünümleri; her biri `position` ve ada göre: `{"items": [...]}`. `?team_id=` ile yalnızca o ekibin görünümleri; ekibin üyeleri, sahipler, yöneticiler ve gelen kutularıyla sınırlı olmayan anahtarlar görür (diğerleri `404` alır). |
| `POST /v1/views` | Bir görünüm kaydeder: `visibility` (`personal`, `workspace` veya `team`), `name`, `filters`, ekip görünümü için `team_id`, isteğe bağlı `position`. `201`. |
| `GET`, `PATCH`, `DELETE /v1/views/{viewId}` | Görünümü okur, değiştirir veya siler. `PATCH` `name`, `filters` (tüm kümeyi değiştirir) ve `position` alır; görünürlük ve ekip değişmez. |
| `PUT /v1/views/order` | `{"ids": [...]}`, 1 ile 50 görünüm: her biri dizin sırasını konum olarak alır. Her görünümü görebiliyor (`404`) ve değiştirebiliyor (`403`) olmalısınız; aksi halde hiçbir şey taşınmaz. `204`. |

Bir görünümde `id`, `visibility`, `owner_member_id` (yalnızca kişisel), `team_id` (yalnızca ekip), `name`, `filters`,
`position`, `count`, `count_capped`, `count_unavailable`, `created_at` ve `updated_at` bulunur.

### Filtreler

`filters`, `GET /v1/conversations` sorgu parametrelerinin ve değerlerinin (metin olarak) bir nesnesidir; örneğin
`status`, `label_id`, `assignee`, `team`, `q` ve `field.<key>`. `cursor` ve `limit` filtre değildir. Filtre, listenin
denetlediği gibi ve kaydeden kişi için denetlenir: bilinmeyen bir ad, listenin reddettiği bir değer veya var olmayan
bir etiket ya da ekip, parametreyi `field` ile gösteren `400 validation_failed` döndürür. Bir görünümde en fazla 30
filtre bulunur, her biri en çok 2000 karakter. `assignee=me`, `team=mine` ve `pinned`, görünümü okuyan kişi için
çözülür.

### Sayaçlar

Her görünümde `count` bulunur: filtrelerine uyan açık konuşmalar, okuyanın kendi gelen kutusu erişimiyle sayılır;
paylaşılan bir görünüm okuyanın göremediğini asla saymaz. `open` içermeyen bir durum filtresi 0 sayar.

| Alan | Anlamı |
|---|---|
| `count` | En fazla 1000. Filtreler okuyana uygulanmıyorsa (göremediği bir gelen kutusu, bir API anahtarı için `assignee=me`) ve sayım başarısız olduysa yoktur. |
| `count_capped` | 1000'den fazlası eşleştiğinde `true`. |
| `count_unavailable` | Sayım bir saniyeden uzun sürdüğünde `true`. |

### Kim ne yapabilir

| Kim | Yapabilir |
|---|---|
| Oturum açmış her üye | Kendi kişisel görünümlerini yönetir; çalışma alanı ve kendi ekiplerinin görünümlerini okur. |
| Sahipler ve yöneticiler | Ayrıca çalışma alanı ve ekip görünümlerini yönetir. Temsilciler `403 forbidden` alır. |
| Bağlı uygulamalar (OAuth) | `views:write` ile kişisel görünümler; sahipler ve yöneticiler için `views:write` ile çalışma alanı ve ekip görünümleri. |
| API anahtarları | `views:write` ile çalışma alanı görünümleri, gelen kutularıyla sınırlı değilse ekip görünümleri. Kişisel görünümler: `403 member_session_required`. |

Ekip üyeleri ekiplerinin görünümlerini görür ama değiştiremez. Başka bir üyenin kişisel görünümü `404` döndürür.
Yalnızca `views:write` taşıyan bir anahtar her görünüm uç noktasında `403 insufficient_scope` alır:
`conversations:read` ekleyin.

Bir üyenin en fazla 50 kişisel görünümü, çalışma alanının 50 çalışma alanı görünümü ve her ekibin 50 ekip görünümü
olabilir (`403 view_limit`).

### Bir şey silindiğinde

Bir etiketi, ekibi veya özel alanı silmek, onu kullanan her görünümden `label_id`, `team_id` veya `field.<key>`
filtresini kaldırır; görünümün geri kalanı kalır. Yeniden adlandırma hiçbir şeyi değiştirmez. Bir ekibi silmek ekip
görünümlerini, bir üyeyi kaldırmak kişisel görünümlerini siler. Silinmiş bir gelen kutusunu, kişiyi veya fikri
kullanan görünüm kalır ve 0 sayar ya da `count` içermez.

## Özel alanlar

Konuşmalarda ve kişilerde türlü değerler, listede filtreler ve bir değer olmadan kapatmayı engelleyen bir kural: [Özel alanlar](custom-fields.md). Tanımlamak için anahtarın `fields:manage` yetkisi olmalı.

## Makrolar

Makro, kayıtlı bir yanıt ile eylemlerdir: eklenecek ya da kaldırılacak etiketler, atanan kişi, durum,
erteleme veya öncelik. Bir mesajı makroyla göndermek, mesajı yazar ve eylemleri tek adımda uygular.
Makro `personal` (yalnızca sizin) ya da `shared` (tüm çalışma alanı) olur.

```sh
curl -X POST https://support.example.com/v1/macros \
  -H "Authorization: Bearer $YUVA_API_KEY" -H "Content-Type: application/json" \
  -d '{"visibility": "shared", "name": "Refund done", "shortcut": "refund-done",
       "body": "Your refund is on its way.",
       "actions": {"add_labels": ["'$LABEL'"], "status": "closed"}}'

curl -X POST https://support.example.com/v1/conversations/$CONVERSATION/messages \
  -H "Authorization: Bearer $YUVA_API_KEY" -H "Content-Type: application/json" \
  -d '{"kind": "message", "direction": "out", "body": "Your refund is on its way.", "macro_id": "'$MACRO'"}'
```

| Çağrı | Ne yapar |
|---|---|
| `GET /v1/macros` | Önce kişisel makrolarınız, sonra paylaşılanlar; her biri `position` ve ada göre: `{"items": [...]}`. Anahtar yalnızca paylaşılanları görür. |
| `POST /v1/macros` | Makro oluşturur: `visibility`, `name`, `shortcut` ve bir `body`, `actions` ya da ikisi; isteğe bağlı `position` (0 ila 10000). `201`. |
| `GET`, `PATCH`, `DELETE /v1/macros/{macroId}` | Makroyu okur, değiştirir veya siler. `PATCH` `name`, `shortcut`, `body` (`null` metni kaldırır), `actions` (tüm kümeyi değiştirir) ve `position` alır; görünürlük değişmez. |

Bir makroda `id`, `visibility`, `owner_member_id` (yalnızca kişisel), `name`, `shortcut`, `body`, `actions`,
`position`, `created_at` ve `updated_at` bulunur. `actions` şunları içerebilir:

| Eylem | Etki |
|---|---|
| `add_labels`, `remove_labels` | Etiket kimlikleri, her biri en fazla 50; bir etiket ikisinde birden olamaz. Silinen etiket makrodan düşer. |
| `assignee_id` | Atanacak üye; `null` atamayı kaldırır. Çıkarılan üye makrodan düşer. |
| `status` | `open`, `pending`, `snoozed` veya `closed`. `snoozed` için `snooze_minutes` gerekir. |
| `snooze_minutes` | 1 ila 525600; mesajın gönderildiği andan itibaren erteler ve `status: snoozed` anlamına gelir. |
| `priority` | Konuşmanın önceliği. |

Kısayol, paylaşılan makrolar arasında ve her üyenin kişisel makroları arasında benzersizdir
(`409 shortcut_taken`); bir hazır yanıtın kısayoluyla aynı olabilir. Bilinmeyen etiketler, üye olmayan bir
atanan, aynı etiketin hem eklenip hem kaldırılması, `snooze_minutes` olmadan `snoozed` ya da ne metni ne
eylemi olan bir makro `400 validation_failed` verir.

| Kim | Ne yapabilir |
|---|---|
| Oturum açmış her üye | Kendi kişisel makrolarını okur, oluşturur, değiştirir ve siler; paylaşılanları okur. |
| Sahipler ve yöneticiler | Ayrıca paylaşılan makroları oluşturur, değiştirir ve siler. Temsilciler `403 forbidden` alır. |
| Bağlı uygulamalar (OAuth) | Kişisel makrolar için `conversations:write`; paylaşılanlar için `canned_replies:write`. |
| API anahtarları | Paylaşılanları `conversations:read` ile okur; `canned_replies:write` ile yönetir (olmadan `403 insufficient_scope`). Kişisel makrolar: `403 member_session_required`. |

Başka bir üyenin kişisel makrosu ve başka bir çalışma alanının makrosu `404` verir.

### Makroyla göndermek

`POST /v1/conversations/{conversationId}/messages` iki isteğe bağlı alan alır (multipart isteğinde `then`
bir JSON dizesi olarak):

| Alan | Anlamı |
|---|---|
| `macro_id` | Bu makronun eylemlerini mesajla birlikte uygular. Paylaşılan makrolar anahtarlarla çalışır. |
| `then` | Makrodan sonra alan alan uygulanan bir `ConversationUpdate`: `then.status` makronun durumunun ve ertelemesinin yerine geçer, `then.labels` etiketlerin yerine geçer (makronun eklemeleri ve kaldırmaları dahil). |

Mesaj ve değişiklikler tek bir işlemdir. Mesaj reddedilirse (geçersiz anma, teslimat reddi) hiçbir şey
uygulanmaz; bir eylem reddedilirse (`then` içinde gelen kutusuna erişimi olmayan bir atanan, geçmişte bir
erteleme) mesaj da yazılmaz. İstisna, makronun kendi atananıdır: o üye çıkarılmışsa veya gelen kutusuna
erişimini kaybetmişse yanıt ve diğer eylemler geçer, atanan atlanır. Her iki alan da anahtarlar ve
token'lar için `conversations:write` gerektirir, taslakta reddedilir (`400 validation_failed`) ve tekrarlanan
bir `client_id` kayıtlı mesajı döndürdüğünde yeniden uygulanmaz. Erteleme, gönderimin sunucu
zamanından başlar.

Konuşmanın zaman çizelgesinde mesajdan hemen sonra `macro` (`id`, `name`) taşıyan ve atanan atlandıysa
`skipped: ["assignee"]` içeren bir `macro_applied` olayı, ardından olağan `assigned`, `status_changed` ve
`labels_changed` olayları görünür. Öncelik değişikliklerinin zaman çizelgesi olayı yoktur.

[MCP](mcp.md) asistanları makroları `list_macros` ile okur; araçlar hiçbir zaman makro uygulamaz. Bir makronun
`body` metnini `draft_reply` ile kullanın.

## Başvuru

### Yetkiler

Gereken yetki olmadan yapılan çağrı `403 insufficient_scope` döner.

| Yetki | İzin verdiği |
|---|---|
| `conversations:read` | Konuşmalar, mesajlar, ekler, etiketler, hazır yanıtlar, olay akışı ve gerçek zamanlı bağlantı |
| `conversations:write` | Durum, atanan kişi, erteleme, öncelik, etiketler; konuşmaları taşıma ve toplu güncelleme |
| `conversations:delete` | Konuşmaları Çöp kutusuna taşıma, geri alma, Çöp kutusunu listeleme. Başka hiçbir yetkiye dahil değildir: adıyla verin. Yalnızca sahiplerin ve yöneticilerin anahtarlarına ve bağlı uygulamalarına sunulur. |
| `messages:write` | Mesajlar ve taslaklar: oluşturma, düzenleme, atma |
| `messages:redact` | [Bir mesajın içeriğini kaldırma](#bir-mesajı-kaldırma). Başka hiçbir yetkiye dahil değildir: adıyla verin. |
| `drafts:send` | `messages:write` ile birlikte taslak gönderme |
| `notes:write` | Notlar |
| `contacts:read` | Kişiler, aramalar, çevrimiçi durumu |
| `contacts:write` | Kişi oluşturma, güncelleme, silme ve birleştirme |
| `inboxes:read` | Çalışma alanı, üyeler, gelen kutuları, kanallar |
| `inboxes:manage` | Gelen kutularını ve kanalları oluşturma, değiştirme ve silme, gelen kutusu erişimi, gizli anahtarlar |
| `labels:write` | Etiketler |
| `canned_replies:write` | Hazır yanıtlar |
| `webhooks:manage` | Webhook'lar, teslimatları ve denemeleri |
| `workspace:manage` | Çalışma alanı ayarları ve kullanım |
| `migrations:manage` | [Bir posta kutusunu Yuva'ya taşıma](migrate.md): tespit, oluşturma, bağlama, tarama, başlatma, bitirme, aktarılan geçmişi kaldırma. Asistan hesaplarına ve gelen kutularıyla sınırlı anahtarlara asla verilmez. |
| `analytics:read` | [Analitik](analytics.md#api): mülkler, rakamlar, canlı, durum |
| `analytics:write` | [Sunucu tarafı olayları](analytics-server.md) gönderme (`POST /v1/analytics/properties/{propertyId}/events`). Yalnızca API anahtarları; OAuth istemcilerine asla sunulmaz. |
| `analytics:manage` | Mülk oluşturma, değiştirme ve silme, anahtarlarını yenileme, verilerini silme. Asistan hesaplarına ve gelen kutularıyla sınırlı anahtarlara asla verilmez. |
| `status:read` | [Durum sayfaları](status-pages.md): sayfalar, bileşenler, olaylar, bildirimler, monitörler ve kontrolleri |
| `status:manage` | Durum sayfalarını değiştirme, durum ayarlama, olay güncellemesi gönderme, monitörleri yönetme, konuşmaları olaylara bağlama ve onlara yanıt verme. `status:read` içermez. |
| `help:read` | [Yardım merkezleri](help-center.md): merkezler, koleksiyonlar, taslakları ve revizyonlarıyla makaleler, arama, rapor |
| `help:write` | Koleksiyon ve makale oluşturma ve değiştirme, taslak kaydetme, görsel yükleme |
| `help:publish` | Yardım makalelerini yayınlama, yayından kaldırma ve yayımlanmış olanları silme. Başka hiçbir yetkiye dahil değildir: adıyla verin. |
| `roadmap:read` | [Yol haritası panoları](roadmap-board.md): panolar, aşama geçmişi, bağlı geri bildirimlere yanıtın sonucu |
| `roadmap:write` | Pano oluşturma, değiştirme ve silme, geri bildirimleri fikirlere bağlama, bağlı geri bildirimlere yanıt. Fikirleri okumak ve bağlamak ayrıca `conversations:read` ister. |
| `roadmap:stage` | Bir fikrin aşamasını taşıma; bağlı geri bildirimlerinin ilerlemesini atar. Başka hiçbir yetkiye dahil değildir: adıyla verin. |
| `changelog:read` | [Yenilikler](whats-new.md): değişiklik günlükleri, taslaklı kayıtlar, önizleme |
| `changelog:write` | Kayıt ve çeviri yazma, görsel yükleme |
| `changelog:publish` | Değişiklik günlüğü oluşturma, değiştirme ve silme; yayındaki kayıtları yayınlama, zamanlama, yayından kaldırma ve silme. Başka hiçbir yetkiye dahil değildir: adıyla verin. |
| `audit:read` | `GET /v1/audit-log`, [denetim günlüğü](operations.md#denetim-günlüğü). Başka hiçbir kapsama dahil değildir: adıyla verin. Yalnızca bir sahip anahtara verebilir, gelen kutularına sınırlı bir anahtar bunu tutamaz. Yalnızca anahtarı oluşturan üye hâlâ sahip olduğu sürece çalışır. `yuva api-key create` adını vermezseniz bunu eklemez. |
| `fields:manage` | [Özel alan](custom-fields.md) tanımlarını oluşturma, değiştirme ve silme. Yalnızca sahipler ve yöneticiler; gelen kutularıyla sınırlı bir anahtar bunu taşıyamaz. Başka hiçbir yetkiye dahil değildir: adıyla verin. Değer yazmak için `conversations:write` veya `contacts:write` gerekir. |
| `views:write` | [Kayıtlı görünümleri](#kayıtlı-görünümler) kaydetme, değiştirme, sıralama ve silme. Okumalar dahil her görünüm uç noktası ayrıca `conversations:read` ister. |
| `integrations:read` | `GET /v1/issue-integrations` ve tek bir entegrasyon: [GitHub ve Linear](issue-trackers.md) bağlantıları, hiçbir zaman bir belirteç değil. |
| `integrations:manage` | GitHub ve Linear bağlamak, değiştirmek, bağlantıyı kesmek, yeni webhook gizli değeri üretmek. Gelen kutularına sınırlı bir anahtar bunu tutamaz. |
| `issues:write` | Geri bildirim ve fikirlerde GitHub ve Linear kayıtları oluşturmak, bağlamak ve bağlantıyı kaldırmak (`/v1/conversations/{id}/issues`, `/v1/discussions/{id}/issues`), `conversations:read` ile birlikte. |
| `feedback:write` | `POST /v1/feedback` |

### Anahtarlar

| Ayar | Ayrıntı |
|---|---|
| Yetkiler, gelen kutuları | Oluştururken sabitlenir. Gelen kutusuyla sınırlı bir anahtar `inboxes:manage`, `webhooks:manage` ya da `workspace:manage` alamaz. |
| Son kullanma | İsteğe bağlı, sabit. Sonrasında `401 api_key_expired`. |
| Ad, bot adı, avatar | Anahtarın yazdıklarında görünür. `PATCH /v1/api-keys/{id}` ile değiştirilir. |

### Taslaklar

| Çağrı | Yaptığı |
|---|---|
| `PATCH /v1/messages/{id}` | Bir taslağı düzenler. |
| `DELETE /v1/messages/{id}` | Taslağı atar. |
| `POST /v1/messages/{id}/send` | Taslağı gönderir; `sent_by` gönderenin kim olduğunu söyler. |

Taslak olmayan bir mesajda: `409 not_a_draft`. Taslaklar `draft.created`, `draft.updated` ve
`draft.deleted` olaylarını üretir.

### Olay akışı ve gerçek zamanlı bağlantı

| Kural | Ayrıntı |
|---|---|
| Saklama | 7 gün. Daha eski bir konum `410 cursor_expired` döner; gerçek zamanlı bağlantı `resync_required` gönderir. Yeniden yükleyin ve `GET /v1/events/latest`'ten tekrar başlayın. |
| `after=0` | Saklanan en eski olaydan başlar. |
| Sayfalar | `has_more` true iken `limit`'ten az olay içerebilir. |
| Yetkiler | `conversations:read`; kişi olayları için ayrıca `contacts:read`. |
| Yazıyor | `typing` çerçevelerinin `id`'si yoktur ve tekrar oynatılmaz. |

### Konuşmaları silme

`DELETE /v1/conversations/{conversationId}` bir konuşmayı Çöp kutusuna taşır. Bunu yalnızca sahipler ve
yöneticiler ile `conversations:delete` yetkisi olan anahtarlar ve bağlı uygulamalar yapabilir; temsilciler
`403 forbidden` alır. Gelen kutularıyla sınırlı bir anahtar yalnızca o gelen kutularında silebilir.
`yuva api-key create` yetkiyi siz adlandırmadıkça vermez.

| Kural | Ayrıntı |
|---|---|
| Çöp kutusu | Konuşma ekip, kişi, uygulamalar ve asistanlar için tüm listelerden, sayımlardan, aramalardan ve rakamlardan kaybolur; kimse onu değiştiremez. İki kez yapmak bir şey değiştirmez. |
| Çöp kutusunu görme | `GET /v1/conversations?trashed=true`; sahipler, yöneticiler ve `conversations:delete` içindir. Konuşmada `trashed_at` ve `deletes_at` olur. |
| Geri alma | 30 gün içinde `POST /v1/conversations/{conversationId}/restore`; durumu, atanan kişisi ve etiketleriyle geri gelir. Sonrasında `409 trash_expired` yanıtı alınır. |
| 30 gün sonra | Saatlik temizlik onu mesajları, ekleri ve saklanan dosyalarıyla kalıcı olarak siler. |
| Birçoğu birden | `POST /v1/conversations/bulk` ile yalnızca `trashed: true` veya `false`, en fazla 100. |
| Kişi yeniden yazarsa | E-posta, sohbet veya uygulamadan yeni bir konuşma başlar; Çöp kutusundaki konuşma asla yeniden açılmaz. |

Panelde sahipler ve yöneticiler Sil'i konuşmanın menüsünde ve bir seçim için bulur, kısayol `#`'dir; geri almak
için Çöp kutusu sekmesi vardır.

Webhook'lar `conversation.trashed` ve `conversation.restored` (konuşma ve kişi) olaylarını, 30 gün sonra da
yalnızca `id`, `inbox_id` ve `contact_id` taşıyan `conversation.deleted` olayını gönderir.
[Denetim günlüğü](operations.md#denetim-günlüğü) her konuşmayı kimin Çöp kutusuna taşıdığını veya geri aldığını,
son silmeyi ise sistemin yaptığını kaydeder.

### Bir mesajı kaldırma

`POST /v1/messages/{messageId}/redact` tek bir mesajın veya notun içeriğini kaldırır ve konuşmada bir satır bırakır:
zamanıyla birlikte "<ad> tarafından kaldırıldı". Yanıt, mesajla birlikte `200` döner; mesajda `redacted_at`,
`redacted_by`, boş bir `body` olur, `html`, ek, kart ve bahsetme olmaz. Sahipler ve yöneticiler her mesajı
kaldırabilir, diğer üyeler yalnızca kendi yazdıkları mesaj ve notları (`403 forbidden`). Anahtarın `messages:redact`
yetkisi olmalıdır ve görebildiği her mesajı kaldırabilir; `yuva api-key create` yetkiyi siz adlandırmadıkça vermez.

| Kural | Ayrıntı |
|---|---|
| Kaldırılan | Metin, HTML, ekler ve saklanan dosyaları; e-postada ayrıca konu, başlıklar ve özgün ileti; hepsi tek adımda. Dosyaları hemen ardından bir iş siler. |
| Kalan | Yazar, zaman, konuşmadaki yer ve e-postada gönderim durumu. |
| Diğer kopyalar | Önceki olaylar, saklanan webhook teslimatları ve önizlemeler içeriği kaybeder; mesaj aramadan çıkar. |
| Kişinin alıntısı | Kişinin sonraki bir e-postası kaldırılan bir e-postayı yanıtlayıp tamamını alıntılıyorsa, alıntı ve o e-postanın özgün hali de kaldırılır; yeni yazdıkları kalır. Kısmi alıntı kalır. |
| İlk mesaj | Bir geri bildirimin ilk mesajı kaldırılırsa `feedback.text` temizlenir; bir konuşmanın konusunu veren e-posta kaldırılırsa konu temizlenir. İkisi de `conversation.updated` gönderir. |
| Henüz gönderilmemiş e-posta | Sıradaki e-posta gönderilmez ve `failed` görünür. |
| Gönderilmiş e-posta | Alıcının posta kutusunda kalır: Yuva onu geri alamaz. |
| Yeniden | Kaldırılmış bir mesajı yeniden kaldırmak onu değişmeden döndürür. |
| Kaldırılamayanlar | Taslak (`DELETE` ile atın) ve zaman çizelgesi olayı `409 not_redactable` döner. Çöp kutusundaki bir konuşma `404` döner. |

Webhook'lar eski içerik olmadan [`message.redacted`](webhooks.md#olaylar) gönderir; elinizdeki kopyaları silin.
[Denetim günlüğü](operations.md#denetim-günlüğü) her kaldırmayı kaydeder, metni asla.
Panelde **Kaldır**, sahipler, yöneticiler ve yazarı için her mesaj ve notta bulunur. Onay penceresi sonucu söyler;
gönderilmiş bir e-postanın alıcının posta kutusunda kaldığını da belirtir. Odaktaki bir mesajda `Delete` tuşu da aynı
pencereyi açar. Konuşmayı açık tutan herkes, zamanıyla birlikte "<üye> tarafından kaldırıldı" satırını canlı görür.


### Idempotency

`/v1` ve `/client/v1` üzerindeki her kimlik doğrulamalı `POST`, `Idempotency-Key` kabul eder (1 ile
255 arası yazdırılabilir ASCII karakter); çağıran başına 24 saat saklanır. `5xx` yanıtları
saklanmaz. Giriş, kişi oturumu isteği ve `/v1/me/…` bunu yok sayar.

| Yeniden deneme | Yanıt |
|---|---|
| Aynı anahtar, aynı istek | İlk yanıt, `Idempotent-Replayed: true` ile |
| Aynı anahtar, farklı istek | `409 idempotency_key_reused` |
| Aynı anahtar, ilki hâlâ çalışıyor | `409 idempotency_key_in_use` |

### Yalnızca API sunan sunucular

| Değişken | Etkisi |
|---|---|
| `YUVA_PANEL=off` | Panel ve OAuth onay sayfası yok; API anahtarları çalışmaya devam eder. |
| `YUVA_WIDGET=off` | `/yuva.js` ya da `/yuva-chat.js` yok; kişi API'si kalır. |

Bkz. [Yapılandırma](configuration.md#sunucu).
