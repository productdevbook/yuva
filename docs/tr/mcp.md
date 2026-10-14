# Yapay zekâ asistanları (MCP)

Yuva'nın yerleşik bir [MCP](https://modelcontextprotocol.io) sunucusu var. Claude, ChatGPT, Cursor,
VS Code ve Gemini CLI gibi asistanlar, verdiğiniz erişimle gelen kutunuzda arama yapabilir,
konuşmaları okuyup düzenleyebilir ve yanıt taslakları yazabilir.

## 1. Asistanın nasıl giriş yapacağını seçin

Uç nokta sunucunuzdaki `/mcp`, örneğin `https://support.example.com/mcp`.

- **OAuth**: asistan sizin adınıza, sizden fazla erişimi olmadan çalışır.
- **API anahtarı**: kimsenin başında olmadığı otomasyonlar için. Ayarlar → API anahtarları altında,
  gereken [yetkilerle](#araçlar) ve "Claude" gibi bir bot adıyla bir anahtar oluşturun
  ([API anahtarları](headless.md#api-anahtarları)).

## 2. Sunucuyu asistanınıza ekleyin

Claude Code'da OAuth ile:

```sh
claude mcp add --transport http yuva https://support.example.com/mcp
claude mcp login yuva
```

API anahtarıyla kullanmak için ilk komuta `--header "Authorization: Bearer $YUVA_API_KEY"` ekleyin.
Diğer asistanlar:

| Asistan | Nasıl |
|---|---|
| claude.ai, Claude Desktop | Customize → Connectors → **Add custom connector**, `/mcp` adresini girin, giriş yapın. Herkese açık bir `https` adresi gerekir ([rehber](https://claude.com/docs/connectors/custom/add-unlisted)). |
| Claude Desktop, özel sunucu | Bir release'teki `yuva.mcpb` dosyasını açın; sunucu adresini ve bir API anahtarı ister ([paket](#yerel-istemciler)). |
| ChatGPT | [Plugins](https://chatgpt.com/plugins) → artı → **Add custom MCP server**, yalnızca OAuth. Herkese açık bir `https` adresi gerekir ([rehber](https://developers.openai.com/plugins/deploy/connect-chatgpt)). |
| Cursor | `.cursor/mcp.json`: `{"mcpServers": {"yuva": {"url": "https://support.example.com/mcp"}}}`; anahtar için `headers` ekleyin ([belgeler](https://cursor.com/docs/context/mcp)). |
| VS Code | **MCP: Add Server**, ya da `"type": "http"` ve `url` ile `.vscode/mcp.json`; anahtar için `headers` ekleyin ([belgeler](https://code.visualstudio.com/docs/copilot/customization/mcp-servers)). |
| Gemini CLI | `gemini mcp add --transport http yuva https://support.example.com/mcp`, sonra `/mcp auth yuva`; ya da anahtar için `--header` ([belgeler](https://geminicli.com/docs/tools/mcp-server/)). |
| MCP Inspector | Her aracı, kaynağı ve prompt'u görmek için `npx @modelcontextprotocol/inspector`. |

## 3. Sorun

Örneğin: "Yuva gelen kutularımı listele ve Türkçe gelen kutusundaki açık konuşmaya Türkçe bir yanıt
taslağı yaz."

Yanıtlar taslaktır. Çalışma alanı botların göndermesine izin vermedikçe
([Botlar gönderebilir](headless.md#botlar-gönderebilir)) bir üye her birini konuşmada inceler ve
gönderir, düzenler ya da atar. Asistanın OAuth ile yazdıkları "Ayşe via Claude" olarak, API
anahtarıyla yazdıkları anahtarın botu olarak görünür.

## init ile Yuva'yı bir projeye ekleyin

Asistanınızı projenin klasöründe açın ve `init` prompt'unu çalıştırın (Claude Code'da
`/mcp__yuva__init`) ya da şöyle sorun: "Bu projeye bak ve Yuva'yı nasıl ekleyeceğimi söyle."
Asistan projeyi kendi makinenizde okur (framework, oturum açma, dokümantasyon sitesi, mobil
uygulamalar, bugün kullandığınız destek ve analitik araçları), çalışma alanınızda neyin zaten
olduğunu görmek için `get_setup_state` aracını çağırır ve kısa bir planla yanıt verir: her adımın
projeye ne kattığı, koddaki dayanağı ve onu yapan prompt ya da araçlar. Siz adımları seçene kadar
hiçbir şey değişmez.

Kodunuz ve dosyalarınız asistanda kalır: `init` projeden Yuva'ya hiçbir şey göndermez. Gelen kutusu,
kanal ya da analitik mülkü oluşturan adımlar adını verdikleri yetkiye ihtiyaç duyar;
`get_setup_state` bağlantının okuyamadığı kısımları ve verilmesi gereken yetkiyi listeler.

## Yuva ile ilgili bir sorunu bildirin

Bir asistan Yuva ile çalışırken Yuva'nın kendisi hatalı davranırsa (bir sunucu hatası, rehberinde
yazdığı gibi çalışmayan bir SDK, yanlış bir rehber), asistan `report_yuva_issue` aracını
çağırabilir. Sunucu [productdevbook/yuva](https://github.com/productdevbook/yuva/issues) için bir
GitHub issue'su yazar; çalışma alanınızın adlarını, sitelerini ve adreslerini, e-posta adreslerini,
diğer URL ve sunucu adlarını, IP adreslerini, anahtarları, token'ları, kimlikleri ve ev dizini
yollarını çıkarır ve metni, önceden doldurulmuş bir bağlantı ve bir `gh issue create` komutuyla
döndürür. Kendisi hiçbir şey açmaz: asistan raporu size gösterir, siz de kendi GitHub hesabınızla
açarsınız. Güvenlik sorunları bunun yerine
[özel bildirime](https://github.com/productdevbook/yuva/security/advisories/new) gider.

## Canlı sohbeti bir asistanla kurun

Sahipler ve yöneticiler kurulumu bir asistana bırakabilir. Bağlanırken `inboxes:manage` yetkisini
verin (API anahtarında da gerekir), sonra örneğin şunu isteyin: "Yuva canlı sohbetini
https://www.example.com sitesine bağla. Acme'nin sitesi, Next.js ile yapıldı."

`setup_live_chat` prompt'u da aynısını yapar. Asistan gelen kutusunu bulur ya da oluşturur, sitenin
origin'i için sohbet kanalını oluşturur (ya da origin'i mevcut kanala ekler) ve sunucunuzun
adresiyle kanalın açık anahtarı doldurulmuş kurulum kodunu alır. Kodunuza erişimi varsa kodu oraya
ekler, yoksa gösterir. `inboxes:manage` olmayan bir bağlantı yetkinin adını veren bir hata alır:
yeniden bağlanıp bu yetkiyi verin. [Giriş yapmış kullanıcılar](identity.md) için kimlik sırrı
asistanlara verilmez; gerektiğinde gelen kutusu ayarlarında yenileyin.

## Bir asistanla bülten taslağı hazırlayın

Bir asistan bir bülteni tasarıma ve taslak bir bültene yazabilir, size deneme gönderebilir ve sonra raporu
okuyabilir. İzin vermedikçe abonelerinize gönderemez: zamanlama ayrı bir adımdır ve onu siz yönetirsiniz.

- Bağlanırken `lists:read`, `broadcasts:read` ve `broadcasts:write` yetkilerini verin (API anahtarının da
  bunlara ihtiyacı var). `draft_newsletter` prompt'u bir özetle başlar ve asla zamanlamaz.
- Bağlı bir asistanla gönderilen deneme yalnızca size gider. API anahtarı, denemeyi alacak üyeleri adıyla belirtir.
- `schedule_broadcast` yalnızca anahtar ya da belirteçte `broadcasts:send` varsa **ve** bir sahip veya
  yönetici "Anahtarlar ve asistanlar bülten gönderebilir" seçeneğini açtıysa listelenir ([Anahtarlar ve
  asistanlar](broadcasts.md#7-anahtarlar-ve-asistanlar)). `send_at` ve `confirm_recipients` ister;
  ikincisi `get_broadcast`'in gösterdiği ve sizin onayladığınız alıcı sayısıdır. Kitle bu arada değiştiyse
  hiçbir şey zamanlanmaz.

Bir örnek:

> **Siz:** "Ürün haberleri" listesine bülten şablonundan bir bülten taslağı hazırla: 2.1 sürümü çıktı,
> karanlık mod ve daha hızlı arama var. Değişiklik günlüğüne bağlantı ver.
>
> **Asistan:** Bülten şablonundan "Sürüm 2.1" tasarımını ve "Karanlık mod ve daha hızlı arama geldi"
> konulu "Sürüm 2.1" taslak bültenini oluşturdum. Gözden geçirme geçiyor. Kitle 1.248 kişi. Taslağı
> Yuva'da açın: *bağlantı*.
>
> **Siz:** Bana deneme gönder.
>
> **Asistan:** Size gönderildi. E-posta uygulamanızda nasıl göründüğüne bakın.
>
> **Siz:** İyi. Yarın 9:00'a zamanla.
>
> **Asistan:** Taslakta yazdığı gibi 1.248 kişiye gidecek. Yarın 9:00'a zamanlayayım mı?
>
> **Siz:** Evet.
>
> **Asistan:** Yarın 9:00'a zamanlandı.

Asistan yalnızca sayıyı onayladıktan sonra zamanlar; göstermezse sayıyı sorun.

## Analitiği bir asistanla kullanın

`analytics:read` ile bir asistan web sitelerinizin ve uygulamalarınızın rakamlarını ([Analitik](analytics.md)) ve bir
gelen kutusuna bağlı web siteleri için sayfalarının ne kadar destek doğurduğunu ([Destek ve trafik](analytics-support.md))
okur. Örneğin sorun: "Bu ay en çok konuşmaya yol açan sayfalar hangileri?" Asistan web sitelerinizi listeler,
`analytics_support_report` aracını `period: month` ve `order_by: conversations` ile çağırır ve her sayfanın
konuşmalarını, sayfa görüntülemelerini ve 1.000 görüntüleme başına konuşma sayısını yanıtlar.

- `analytics_review` prompt'u bir web sitesini bir aralıkta gözden geçirir: önceki aralığa göre başlıca rakamlar,
  kaynaklar, sayfalar, hedefler ve gelen kutusu varsa sayfa başına destek; öneriyle biter. Asistan analitiği okuyabiliyorsa
  `weekly_report` prompt'u bir destek-ve-trafik bölümü ekler.
- `setup_analytics` prompt'u (`analytics:manage` ister) bir web sitesi ya da uygulama oluşturur, kurulum kodunu alır ve
  projenize ekler; `setup_live_chat` sohbet için neyse odur.
- Sayfa yolları, yönlendirenler, kampanyalar, olay adları ve özellik değerleri ziyaretçilerin tarayıcılarından gelir ve
  müşteri metni gibi yalnızca `customer_content` içinde döner. Analitik araçları yalnızca sunucuda analitik açıkken
  listelenir.

## Bir posta kutusunu asistanla taşıyın

`migrations:manage` yetkisiyle `move_to_yuva` prompt'unu posta kutusunun adresiyle çalıştırın. Asistan
sağlayıcıyı bulur, taşımayı oluşturur ve uygulama şifresini girmeniz için size bir Yuva sayfası verir;
sohbette asla şifre istemez. Kopyalamayı yalnızca deneme sonucunu onayladıktan sonra başlatır, sonra
e-postaların nasıl yönlendirileceğini anlatır ve taşımayı bitirir. Adımlar [Yuva'ya taşı](migrate.md)
sayfasındadır.

## Başvuru

### Araçlar

Araçlar `/v1` ile aynı kontrollerden geçer. Kullanamayacağınız araçlar listelenmez; yine de
çağrılırsa verilmesi gereken yetkiyle birlikte `insufficient_scope` döner.

| Araçlar | Yetki |
|---|---|
| `get_setup_state`, `report_yuva_issue` | yok; her bağlantı |
| `search_conversations`, `get_conversation`, `list_labels`, `list_canned_replies`, `get_counts`, `list_feedback` | `conversations:read` |
| `list_inboxes` | `inboxes:read` |
| `get_contact`, `lookup_contact` | `contacts:read` |
| `draft_reply` | `messages:write` |
| `send_reply` | `messages:write`; yalnızca botlar gönderebiliyorsa |
| `send_draft` (bir `draft_reply` taslağını gönderir) | `messages:write` ve `drafts:send`; yalnızca botlar gönderebiliyorsa |
| `add_note` | `notes:write` |
| `assign`, `set_status`, `snooze`, `add_labels`, `remove_labels`, `move_conversation`, `bulk_update` | `conversations:write` |
| `merge_contacts` (sahipler, yöneticiler, gelen kutusu sınırı olmayan anahtarlar) | `contacts:write` |
| `list_channels`, `get_install_snippet` (`html`, `react`, `nextjs`, `nuxt`, `vue`, `astro`, `shadcn`, `docs-` varyantlarıyla dokümantasyon siteleri ve `list_ids` alan `subscribe-html`, `subscribe-react`, `subscribe-nuxt` ve `subscribe-shadcn` ile abonelik formları için kod) | `inboxes:read` |
| `create_inbox`, `create_chat_channel`, `update_chat_channel` (sahipler ve yöneticiler) | `inboxes:manage` |
| `list_properties`, `analytics_overview`, `analytics_query` (`dataset: support` ve `insights` dahil), `analytics_realtime`, `analytics_support_report`, `list_goals`, `funnel_report`, `get_analytics_snippet`, `add_annotation` | `analytics:read` |
| `create_property`, `create_goal` (sahipler, yöneticiler, gelen kutusu sınırı olmayan anahtarlar) | `analytics:manage` |
| `list_mailing_lists` | `lists:read` |
| `list_broadcasts`, `get_broadcast` (gözden geçirme ve taslak için kitle sayısıyla), `get_broadcast_report` | `broadcasts:read` |
| `list_email_templates`, `preview_design` (sahipler ve yöneticiler) | `broadcasts:read` |
| `create_design`, `update_design`, `create_broadcast_draft`, `send_test_broadcast` (sahipler ve yöneticiler) | `broadcasts:write` |
| `schedule_broadcast` (yalnızca çalışma alanı anahtarların ve asistanların bülten göndermesine izin veriyorsa) | `broadcasts:send` |
| `detect_mail_source`, `list_migrations`, `get_migration`, `create_migration`, `update_migration`, `test_migration_connection`, `scan_migration`, `preview_migration`, `start_migration`, `pause_migration`, `resume_migration`, `cancel_migration`, `finish_migration`, `retry_migration_items`, `list_migration_errors`, `send_migration_probe`, `remove_migration_history` (sahipler ve yöneticiler) | `migrations:manage` |

Bir konuşmada açık taslağınız varken oradaki `send_reply` `draft_pending` döner.

Hiçbir taşıma aracı şifre almaz: `create_migration`, uygulama şifresini girdiğiniz bir Yuva sayfası (`connect_url`) döndürür. `remove_migration_history` yıkıcı olarak işaretlidir ve `confirm_count` ister; bu, onayladığınız konuşma sayısıdır.

`create_migration`, `inbox_id` ile ve `channel_id` olmadan çağrıldığında, posta kutusu adresi için gelen kutusunun e-posta kanalını kullanır ya da yeni bir kanal oluşturur (ayrıca `inboxes:manage` gerekir). Yeni kanalın SMTP hesabı yoktur; bu yüzden yanıtta `channel_created`, `channel_setup` (yanıtlar gönderilebilmeden önce panelde nelerin ayarlanacağı) ve `channel_settings_url` bulunur; `move_to_yuva` prompt'u bunları size iletir.

### Kaynaklar ve prompt'lar

| Tür | Öğeler |
|---|---|
| Kaynaklar | `yuva://inbox/{id}` (gelen kutusu ve en son açık konuşmaları), `yuva://conversation/{id}`, `yuva://contact/{id}`, `yuva://broadcast/{id}`, `yuva://migration/{id}` (`migrations:manage` ile), `yuva://property/{id}` (bir web sitesi, ayarları, son yedi günü ve hedefleriyle; `analytics:read` ile); abonelikler değişiklikleri izler. |
| Prompt'lar | `init` (isteğe bağlı hedef; her bağlantı), `triage_inbox`, `draft_reply`, `summarize_conversation`, `weekly_report`, `setup_live_chat`, `draft_newsletter`, `broadcast_report`, `move_to_yuva` (adres, isteğe bağlı gelen kutusu; `migrations:manage` ile), `analytics_review` (isteğe bağlı web sitesi ve aralık; `analytics:read` ile), `setup_analytics` (site adresi, isteğe bağlı framework; `analytics:manage` ile). |

### Güvenlik ve sınırlar

| Konu | Ayrıntı |
|---|---|
| Müşteri metni | Yalnızca `customer_content` alanlarının içinde döner; talimat değil veri olarak işaretlidir. |
| İpuçları | Okuma araçları `readOnlyHint`, tekrarlanabilir yazma araçları `idempotentHint`, `merge_contacts` `destructiveHint` taşır. Her aracın bir çıktı şeması vardır. |
| Hız sınırı | Token ya da anahtar başına dakikada 600 istek; üstü `Retry-After` ile `429`. |
| Kapatma | `YUVA_MCP=off`, `/mcp`'nin `404` dönmesini sağlar ([Yapılandırma](configuration.md#sunucu)). OAuth uç noktaları kalır. |

### OAuth

Yuva kendi OAuth 2.1 sunucusudur (authorization code, PKCE `S256`, resource indicators).

| Konu | Ayrıntı |
|---|---|
| Keşif | `WWW-Authenticate` ile `401` → `/.well-known/oauth-protected-resource/mcp` → `/.well-known/oauth-authorization-server`. |
| Kayıt | Dinamik (`POST /oauth/register`) ya da bir Client ID Metadata Document (`https` adresi olan bir `client_id`). |
| Onay | Panelde: çalışma alanını ve yetkileri seçin. `YUVA_PANEL=off` ile istemciler `temporarily_unavailable` alır. |
| Token'lar | Sizin adınıza, izin verilen yetkilerle daraltılmış olarak çalışır. Erişim token'ları bir saat geçerlidir ve 30 güne kadar yenilenir. |
| Yönlendirme adresleri | `https`, loopback bir host üzerinde `http` ya da özel kullanımlı bir şema; birebir eşleşir. Loopback `http` adresinin portu farklı olabilir. |
| İzinler | Ayarlar → Bağlı uygulamalar bunları listeler ve iptal eder. |

### Yerel istemciler

`yuva mcp stdio`, yalnızca yerel program başlatabilen istemciler için stdio'yu `/mcp`'ye bağlar.

```sh
yuva mcp stdio --url https://support.example.com --key "$YUVA_API_KEY"
```

| Konu | Ayrıntı |
|---|---|
| Seçenekler | `--url` (yoksa sonuna `/mcp` eklenir) ve `--key` (bir API anahtarı ya da OAuth erişim token'ı), ya da `YUVA_URL` ve `YUVA_API_KEY`. |
| Program | `yuva.mcpb` içinde (bir zip: `server/<os>-<arch>/yuva`, Windows'ta `server/yuva.exe`) ya da `docker run --rm -i -e YUVA_URL -e YUVA_API_KEY ghcr.io/productdevbook/yuva:<version> mcp stdio`. |
| Claude Desktop paketi | 0.0.4'ten itibaren release'ler macOS, Linux ve Windows için `yuva.mcpb` ekler. Paket olmadan `claude_desktop_config.json` dosyasına `"command": "/usr/local/bin/yuva"`, `"args": ["mcp", "stdio"]` ve `env` içinde iki değişkeni ekleyin. |
| Hatalar | Başarısız bir çağrı JSON-RPC hatası döner; `401` alınca 1 durum koduyla çıkar. |
| MCP Registry | Uzak uç nokta ve paketle birlikte `io.github.productdevbook/yuva` olarak listelenir. |
