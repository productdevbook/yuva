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
- `get_broadcast`, bülten sayfasındaki kontrolün aynısını gösterir: her kontrol tamam, uyarı ya da engel
  olarak gelir. Asistan tasarım sorunlarını `update_design` ile, taslaktaki diğer her şeyi (konu, ön
  metin, kanal, listeler, tıklama takibi, UTM etiketleri, hız) `update_broadcast` ile düzeltir; bu araç
  kontrolü yeniden döndürür. Onay ve kanalın alan adı kontrolleri sizde kalır.
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

## Yardım merkezini bir asistanla kullanın

`help:read` ve `help:write` yetkileriyle bir asistan [yardım merkezinde](help-center.md) arama yapabilir,
makaleleri okuyabilir ve taslak yazabilir. Şunu deneyin: "Son 30 günde müşterilerin aradığı ama
yanıt bulamadığı şeyleri bul ve en önemli üç makalenin taslağını yaz." `help_gaps_review` prompt'u
bunu yapar; `write_help_article` bir konuşmadan ya da konudan tek bir makale taslağı yazar, önce
arama yaparak aynı şeyin iki kez yazılmasını önler. Asistan asla yayınlamaz: taslak kaydeder, bir
üye onu Yuva'da okuyup yayınlar. `draft_reply` içindeki `article_ids`, yanıta en fazla üç yayımlanmış
makaleyi kart olarak ekler.

## Yol haritasını bir asistanla kullanın

`roadmap:read`, `roadmap:write` ve `conversations:read` yetkileriyle bir asistan bir [yol haritası panosunun](roadmap-board.md)
fikirlerini listeleyebilir ve geri bildirimleri onlara bağlayabilir. `link_feedback_to_ideas` prompt'u henüz
bağlanmamış geri bildirimleri gözden geçirir, panoda arar ve bağlantılar önerir; siz onaylayana kadar hiçbir şeyi
değiştirmez ve asla bir aşamayı taşımaz. Aşamayı taşımak müşterilere bildirilir: `move_idea_stage`, yalnızca
`roadmap:stage` yetkisini adıyla veren bir anahtara ya da izne listelenir; başka hiçbir yetki bunu içermez.
Ayrıca `confirm_linked` ister; bu, `get_roadmap_idea`'nın o aşama için gösterdiği (`would_change`) ve sizin
gördüğünüz bağlı geri bildirim sayısıdır. Başka bir sayı `409 confirm_mismatch` verir ve hiçbir şey taşınmaz.
Hiçbir araç fikir oluşturmaz, düzenlemez, denetlemez ya da bağlı geri bildirimlere yanıt göndermez.

## Yenilikleri bir asistanla kullanın

`changelog:read` ve `changelog:write` yetkileriyle bir asistan bir [Yenilikler](whats-new.md) değişiklik günlüğünü okuyabilir ve kayıtları taslak olarak yazabilir. `draft_release_notes` prompt'una commit başlıklarınızı veya sürüm notlarınızı verin (Yuva kendisi hiçbir şey getirmez); müşterinin fark edeceği her değişiklik için yeni, iyileştirildi ya da düzeltildi etiketli bir kayıt taslağı hazırlar. Bir asistan bir kaydı asla yayınlamaz, zamanlamaz, yayından kaldırmaz ya da e-postayla göndermez ve zamanlanmış bir kaydı değiştiremez: taslakları bir üye okur ve Yuva'da yayınlar.

## Listeleri, tasarımları ve kurulumu bir asistanla yapın

Taslak yazmanın ötesinde, bir asistan bülten ya da yeni bir ürün sayfasından önce gelen olağan adımları yapabilir; her aracın rolü ve yetkisi [aşağıdaki tabloda](#araçlar) yazılıdır. Kullanamayacağınız araçlar listelenmez.

- **Gönderen ve kanal.** `update_inbox_sender`, her bültenin alt bilgisinde görünen yasal adı, posta adresini, sicil bilgisini ve iletişimi ayarlar. `create_email_channel` SMTP hesabı olmadan bir adres ekler: yanıt, postanın yönlendirileceği adresi ve SMTP'yi kendinizin gireceği bir bağlantıyı verir. `create_dkim_key` ve `get_deliverability`, eklenecek DNS kaydını ve kontrollerin ne bulduğunu verir.
- **Listeler ve aboneler.** `create_mailing_list`, `update_mailing_list`, `list_subscriptions`, `add_subscription` ve `unsubscribe`. `add_subscription` bir onay e-postası gönderir ve kişiyi beklemede bırakır. Kişiyi yalnızca, onun zaten kabul ettiğini, nerede ve ne zaman kabul ettiğini kendi sözlerinizle belirtirseniz abone yapar; asistan sizin sözlerinizi iletir, kendisi yazmaz. Bastırılmış ve çıkmış adresler reddedilir.
- **CSV içe aktarma.** `create_list_import` dosyanın metnini alır (bir asistan üzerinden yaklaşık 5 MB'a kadar; daha büyük dosyalar Yuva'da yüklenir, bkz. [Abone içe aktarma](importing.md)); `attestation` ve `consent_source` rıza kanıtı olarak saklanır ve sizin sözleriniz olmalıdır. Yalnızca doğrular. `get_list_import` sayıları gösterdikten sonra `start_list_import`, size gösterilen geçerli satır sayısı olan `confirm_count` ister; başka bir sayı `409 count_mismatch` döner ve hiçbir şey eklenmez.
- **Görseller ve tasarımlar.** `upload_email_image` herkese açık bir `https` URL'si (10 saniye içinde getirilir; özel ve yerel adresler reddedilir) ya da `data_base64` alır, bir asistan üzerinden en fazla yaklaşık 5 MB görsel, ve `update_design`'ın görsel bloğunun aldığı kimliği döndürür. `list_designs`, `get_design`, `list_email_images` ve `duplicate_design` takımı tamamlar. Görsel kuralları için bkz. [E-posta tasarımları](designs.md).
- **Bülten yönetimi.** `pause_broadcast`, `resume_broadcast`, `unschedule_broadcast` ve `cancel_broadcast`, `schedule_broadcast` ile aynı kuralla listelenir. `cancel_broadcast`, onayladığınız bülten adı olan `confirm_name` ister.
- **Kişiler ve konuşmalar.** `list_members`, `assign` için kimlikleri verir; kişiler, notları ve abonelikleri aranabilir, oluşturulabilir ve güncellenebilir; `start_conversation` çalışma alanı asistanların teslim etmesine izin vermedikçe ilk mesajı taslak yazar; `list_drafts` incelemeyi bekleyen taslakları gösterir, `update_draft` birini yeniden yazar, `discard_draft` siler; etiketler ve hazır yanıtlar oluşturulabilir. `list_discussions` ve `get_discussion` tartışmaları okur; `reply_to_discussion` ve `mark_discussion_answer` sizin adınıza yanıt verir ve cevabı işaretler; yanıt hemen herkese açık olduğundan yalnızca OAuth ile ve çalışma alanı asistanların teslim etmesine izin verdiğinde. `request_broadcast_approval` operatörden yayın izni ister; yalnızca OAuth ile bağlanan sahibe sunulur ve web sitesi, gönderen ve onay açıklamaları asistanın değil sizindir.
- **Diğer ürünler.** `create_status_page`, `create_status_component`, `create_status_monitor`, `update_incident`, `create_help_center`, `create_help_collection`, `create_changelog`, `create_roadmap_board`, `create_funnel` ve `analytics_retention`. Oluşturulan hiçbir şey yayınlanmaz: bunu bir üye Yuva'da yapar. `create_status_monitor` push belirtecini yalnızca bir kez gösterir; izleme işinize koymanız içindir.

Bir asistanın bilerek yapamadıkları: kimlik bilgilerini ve erişimi yönetmek (API anahtarları, OAuth izinleri, roller, davetler, geçiş anahtarları, push abonelikleri, webhook'lar, bir sırrı döndürmek), çalışma alanı ayarlarını değiştirmek ya da çalışma alanını silmek, bir şeyi silmek (`remove_migration_history` hariç), rıza kayıtlarını ya da analitiği dışa aktarmak veya analitik verisini silmek, taşıma dosyaları yüklemek, ham mesajları ve ekleri okumak, SMTP ya da başka bir şifre almak, yardım makalelerini, Yenilikler kayıtlarını ve olayları yayınlamak. Abonelikten çıkarmak bir durum değişikliğidir, silme değildir.

## Başvuru

### Araçlar

Araçlar `/v1` ile aynı kontrollerden geçer. Kullanamayacağınız araçlar listelenmez; yine de
çağrılırsa verilmesi gereken yetkiyle birlikte `insufficient_scope` döner.

| Araçlar | Yetki |
|---|---|
| `get_setup_state`, `report_yuva_issue` | yok; her bağlantı |
| `search_conversations`, `get_conversation`, `list_labels`, `list_canned_replies`, `get_counts` | `conversations:read` |
| `get_report` ([Raporlar](reports.md)) | `reports:read` |
| `list_feedback` (kişi verdiyse `feedback_score`, 1 ile 5, ile; `without_idea` bir fikre bağlı olmayan geri bildirimleri tutar) | `conversations:read` |
| `list_inboxes` | `inboxes:read` |
| `get_contact`, `lookup_contact` | `contacts:read` |
| `draft_reply` | `messages:write` |
| `send_reply` | `messages:write`; yalnızca botlar gönderebiliyorsa |
| `send_draft` (bir `draft_reply` taslağını gönderir) | `messages:write` ve `drafts:send`; yalnızca botlar gönderebiliyorsa |
| `update_draft`, `discard_draft` (yalnızca gönderilmemiş taslaklar; bir üyenin kendi taslağını yalnızca o üye) | `messages:write` |
| `add_note` | `notes:write` |
| `assign`, `set_status`, `snooze`, `add_labels`, `remove_labels`, `move_conversation`, `bulk_update` | `conversations:write` |
| `merge_contacts` (sahipler, yöneticiler, gelen kutusu sınırı olmayan anahtarlar) | `contacts:write` |
| `list_channels`, `get_install_snippet` (`html`, `react`, `nextjs`, `nuxt`, `vue`, `astro`, `shadcn`, `docs-` varyantlarıyla dokümantasyon siteleri ve `list_ids` alan `subscribe-html`, `subscribe-react`, `subscribe-nuxt` ve `subscribe-shadcn` ile abonelik formları için kod) | `inboxes:read` |
| `create_inbox`, `create_chat_channel`, `update_chat_channel`, `create_app_channel`, `update_app_channel` (sahipler ve yöneticiler) | `inboxes:manage` |
| `list_properties`, `analytics_overview`, `analytics_query` (`dataset: support` ve `insights` dahil), `analytics_realtime`, `analytics_support_report`, `list_goals`, `funnel_report`, `get_analytics_snippet`, `add_annotation` | `analytics:read` |
| `create_property`, `create_goal` (sahipler, yöneticiler, gelen kutusu sınırı olmayan anahtarlar) | `analytics:manage` |
| `list_status_pages`, `get_status_page`, `list_monitors` | `status:read` |
| `set_component_state`, `create_incident` (her zaman taslak), `post_incident_update`, `resolve_incident` | `status:manage` |
| `link_conversation_to_incident` | `status:manage` ve `conversations:read` |
| `list_roadmap_ideas`, `get_roadmap_idea` | `conversations:read` ve `roadmap:read` |
| `link_feedback_to_idea`, `unlink_feedback_from_idea` | `roadmap:write` ve `conversations:read` |
| `move_idea_stage` (yalnızca `roadmap:stage` adıyla verildiğinde; `confirm_linked` ister) | `roadmap:stage` |
| `list_help_centers`, `search_help_articles`, `get_help_article`, `help_search_gaps` | `help:read` |
| `draft_help_article`, `update_help_article_draft` (yalnızca taslak, `base_version` ile; `locale` bir çeviri yazar) | `help:write` |
| `import_help_articles` (oturumu açık bir Zendesk taşımasından merkeze, taslak olarak), `get_help_import` | `help:write` |
| `list_whats_new_entries`, `get_whats_new_entry` | `changelog:read` |
| `draft_whats_new_entry`, `update_whats_new_draft` (yalnızca taslak, `base_version` ile; `locale` bir çeviri yazar) | `changelog:write` |
| `list_mailing_lists` | `lists:read` |
| `list_broadcasts`, `get_broadcast` (gözden geçirme ve taslak için kitle sayısıyla), `get_broadcast_report` | `broadcasts:read` |
| `list_email_templates`, `preview_design` (sahipler ve yöneticiler) | `broadcasts:read` |
| `create_design`, `update_design`, `create_broadcast_draft`, `update_broadcast`, `send_test_broadcast` (sahipler ve yöneticiler) | `broadcasts:write` |
| `schedule_broadcast` (yalnızca çalışma alanı anahtarların ve asistanların bülten göndermesine izin veriyorsa) | `broadcasts:send` |
| `list_members` | `inboxes:read` |
| `list_teams` | `inboxes:read` |
| `list_macros` | `conversations:read` |
| `list_custom_fields` | `conversations:read` veya `contacts:read` |
| `set_conversation_fields` | `conversations:write` |
| `search_contacts`, `list_contact_notes` | `contacts:read` |
| `list_contact_subscriptions` | `contacts:read` ve `lists:read` |
| `create_contact`, `update_contact`, `add_contact_note` | `contacts:write` |
| `start_conversation` (çağıran teslim edebiliyor ve `send: true` veriyorsa ilk mesaj gönderilir, yoksa taslak olur) | `conversations:write` ve `messages:write` |
| `list_drafts`, `list_discussions`, `get_discussion` | `conversations:read` |
| `reply_to_discussion`, `mark_discussion_answer` (yalnızca OAuth; yalnızca botlar gönderebiliyorsa) | `messages:write` |
| `request_broadcast_approval` (yalnızca OAuth ile sahipler) | `workspace:manage` |
| `create_label` (sahipler, yöneticiler, anahtarlar) | `labels:write` |
| `create_canned_reply`, `update_canned_reply` (sahipler, yöneticiler, anahtarlar) | `canned_replies:write` |
| `update_inbox_sender`, `create_email_channel`, `create_dkim_key` (sahipler, yöneticiler, anahtarlar) | `inboxes:manage` |
| `get_deliverability` (`check: true` ile bir test iletisi gönderir ve `inboxes:manage` ister) | `inboxes:read` |
| `list_subscriptions`, `get_list_import`, `list_list_imports`, `get_list_import_errors` | `lists:read` |
| `create_mailing_list`, `update_mailing_list`, `add_subscription`, `unsubscribe`, `create_list_import`, `update_list_import_mapping`, `cancel_list_import`, `start_list_import` (sahipler, yöneticiler, anahtarlar) | `lists:write` |
| `list_designs`, `get_design`, `list_email_images` (sahipler, yöneticiler, anahtarlar) | `broadcasts:read` |
| `duplicate_design`, `upload_email_image` (sahipler, yöneticiler, anahtarlar) | `broadcasts:write` |
| `pause_broadcast`, `resume_broadcast`, `unschedule_broadcast`, `cancel_broadcast` (yalnızca çalışma alanı anahtarların ve asistanların bülten göndermesine izin veriyorsa) | `broadcasts:send` |
| `create_status_page`, `create_status_component`, `create_status_monitor`, `update_incident` (sahipler, yöneticiler, anahtarlar) | `status:manage` |
| `create_help_collection` | `help:write` |
| `create_help_center` (sahipler, yöneticiler, anahtarlar; OAuth yok) | `help:publish` |
| `create_changelog` (sahipler, yöneticiler, anahtarlar; OAuth yok) | `changelog:publish` |
| `create_roadmap_board` (sahipler, yöneticiler, anahtarlar; OAuth yok) | `roadmap:write` |
| `create_funnel` (sahipler, yöneticiler, anahtarlar) | `analytics:manage` |
| `analytics_retention` | `analytics:read` |
| `detect_mail_source`, `list_migrations`, `get_migration`, `create_migration`, `update_migration`, `test_migration_connection`, `scan_migration`, `preview_migration`, `start_migration`, `pause_migration`, `resume_migration`, `cancel_migration`, `finish_migration`, `retry_migration_items`, `list_migration_errors`, `send_migration_probe`, `remove_migration_history` (sahipler ve yöneticiler) | `migrations:manage` |

Bir konuşmada açık taslağınız varken oradaki `send_reply` `draft_pending` döner.

Hiçbir taşıma aracı şifre almaz: `create_migration`, uygulama şifresini girdiğiniz bir Yuva sayfası (`connect_url`) döndürür. `remove_migration_history` yıkıcı olarak işaretlidir ve `confirm_count` ister; bu, onayladığınız konuşma sayısıdır.

`create_migration`, `inbox_id` ile ve `channel_id` olmadan çağrıldığında, posta kutusu adresi için gelen kutusunun e-posta kanalını kullanır ya da yeni bir kanal oluşturur (ayrıca `inboxes:manage` gerekir). Yeni kanalın SMTP hesabı yoktur; bu yüzden yanıtta `channel_created`, `channel_setup` (yanıtlar gönderilebilmeden önce panelde nelerin ayarlanacağı) ve `channel_settings_url` bulunur; `move_to_yuva` prompt'u bunları size iletir.

`assign`, `team_id` (`list_teams` içinden, konuşmanın gelen kutusuna yetkisi olan bir ekip) veya `remove_team: true` alır; ikisinden biri gönderilip `assignee_id` verilmediğinde üye atanan olduğu gibi kalır. `search_conversations` `team_id` alır ve konuşmalarda `team_id` bulunur.

`list_macros`, çalışma alanının paylaşılan makrolarını ve sizinkileri `id`, `name`, `shortcut`, `visibility`, `body` ve `actions` (`add_labels`, `remove_labels`, `assignee_id`, `unassign`, `status`, `snooze_minutes`, `priority`) ile döndürür. Eylemler yalnızca bilgi içindir; hiçbir araç makro uygulamaz.

`list_custom_fields`, çalışma alanının [özel alanlarını](custom-fields.md) döndürür (`object`, `key`, `name`, `type`, `options`, `required_on_close`); isteğe bağlı `object` olarak `conversation` veya `contact` verilebilir. `set_conversation_fields`, `conversation_id` ve `fields` alır, yani anahtara göre değerler (`null` birini temizler); uymayan bir değer `field_invalid` döner. `search_conversations`, anahtar ve değerden oluşan bir nesne olan `fields` alır ve konuşmalar `fields` taşır. Zorunlu bir alan boşken `set_status` veya `bulk_update` ile kapatmak `field_required` döner. Hiçbir araç alan tanımlamaz.

`search_conversations`, `sla_state` (`on_track`, `due_soon`, `breached`, `met` veya `paused`) ve `sort` alır (`last_activity` ya da en erken [SLA](sla.md) son tarihi önce gelsin diye `sla_due`); bir politika uygulandığında konuşmalar `sla` taşır. Politikaları yöneten bir araç yoktur.

### Kaynaklar ve prompt'lar

| Tür | Öğeler |
|---|---|
| Kaynaklar | `yuva://inbox/{id}` (gelen kutusu ve en son açık konuşmaları), `yuva://conversation/{id}`, `yuva://contact/{id}`, `yuva://broadcast/{id}`, `yuva://migration/{id}` (`migrations:manage` ile), `yuva://property/{id}` (bir web sitesi, ayarları, son yedi günü ve hedefleriyle; `analytics:read` ile), `yuva://help-center/{id}` (`help:read` ile), `yuva://roadmap/{board_id}` (bir pano ve en çok oy alan fikirleri; `roadmap:read` ve `conversations:read` ile), `yuva://changelog/{id}` (bir değişiklik günlüğü ve en yeni kayıtları; `changelog:read` ile); abonelikler değişiklikleri izler. |
| Prompt'lar | `init` (isteğe bağlı hedef; her bağlantı), `triage_inbox`, `draft_reply`, `summarize_conversation`, `weekly_report`, `setup_live_chat`, `draft_newsletter`, `broadcast_report`, `move_to_yuva` (adres, isteğe bağlı gelen kutusu; `migrations:manage` ile), `analytics_review` (isteğe bağlı web sitesi ve aralık; `analytics:read` ile), `setup_analytics` (site adresi, isteğe bağlı framework; `analytics:manage` ile), `write_help_article` (merkez, isteğe bağlı konuşma ya da konu, dil; `help:write` ile), `help_gaps_review` (merkez, isteğe bağlı gün sayısı; `help:read` ile), `link_feedback_to_ideas` (isteğe bağlı gelen kutusu ve pano; `roadmap:write`, `roadmap:read` ve `conversations:read` ile), `draft_release_notes` (yazılacak notlar, isteğe bağlı değişiklik günlüğü ve dil; `changelog:write` ile). |

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
