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

## Başvuru

### Araçlar

Araçlar `/v1` ile aynı kontrollerden geçer. Kullanamayacağınız araçlar listelenmez.

| Araçlar | Yetki |
|---|---|
| `search_conversations`, `get_conversation`, `list_labels`, `list_canned_replies`, `get_counts`, `list_feedback` | `conversations:read` |
| `list_inboxes` | `inboxes:read` |
| `get_contact`, `lookup_contact` | `contacts:read` |
| `draft_reply` | `messages:write` |
| `send_reply` | `messages:write`; yalnızca botlar gönderebiliyorsa |
| `send_draft` (bir `draft_reply` taslağını gönderir) | `messages:write` ve `drafts:send`; yalnızca botlar gönderebiliyorsa |
| `add_note` | `notes:write` |
| `assign`, `set_status`, `snooze`, `add_labels`, `remove_labels`, `move_conversation`, `bulk_update` | `conversations:write` |
| `merge_contacts` (sahipler, yöneticiler, gelen kutusu sınırı olmayan anahtarlar) | `contacts:write` |

Bir konuşmada açık taslağınız varken oradaki `send_reply` `draft_pending` döner.

### Kaynaklar ve prompt'lar

| Tür | Öğeler |
|---|---|
| Kaynaklar | `yuva://inbox/{id}` (gelen kutusu ve en son açık konuşmaları), `yuva://conversation/{id}`, `yuva://contact/{id}`; abonelikler değişiklikleri izler. |
| Prompt'lar | `triage_inbox`, `draft_reply`, `summarize_conversation`, `weekly_report`. |

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
