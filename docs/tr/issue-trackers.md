# GitHub ve Linear kayıtları

Bir çalışma alanına GitHub ya da Linear bağlayın; bir geri bildirim ya da bir [yol haritası
fikri](roadmap-board.md), ekibinizin işini planladığı yerde bir kayda (issue) dönüşebilir. Ardından
kaydın durumu, kimse elle ayarlamadan, müşteriye geri bildiriminin nerede olduğunu söyler.

## 1. Bir izleyici bağlayın

İzleyicileri sahipler ve yöneticiler bağlar; hangilerinin bağlı olduğunu her üye görür. Kimsenin bir
uygulama kaydetmesi gerekmez.

**GitHub.** Kayıtların açılacağı depolarla sınırlı, depo izni Issues: Read and write olan bir
[ince ayarlı kişisel erişim belirteci](https://github.com/settings/personal-access-tokens/new)
oluşturun. GitHub Enterprise Server'da API adresini de verin: `https://<host>/api/v3`; herkese açık bir
adresten erişilebilir olmalıdır.

**Linear.** Settings › Account › Security & access altında kişisel bir API anahtarı oluşturun.

Sonra API ile bağlayın (panel ekranı ardından gelecek):

```sh
curl -X POST "$YUVA_URL/v1/issue-integrations" -H "Authorization: Bearer $YUVA_KEY" \
  -H "Content-Type: application/json" \
  -d '{"provider": "github", "token": "github_pat_…", "default_target": "acme/app"}'
```

| Alan | Anlamı |
|---|---|
| `provider` | `github` ya da `linear`. |
| `token` | Belirteç ya da anahtar. Yuva önce izleyiciye sorar (reddedilirse `422 tracker_auth_failed`), sunucunun ana anahtarıyla şifreli saklar ve bir daha göstermez. |
| `name` | Ekibinize gösterilir; varsayılanı sağlayıcı ve hesaptır. |
| `api_url` | Yalnızca GitHub Enterprise Server için. |
| `default_target` | Hedef verilmediğinde yeni kayıtların gideceği yer: `owner/repo` ya da `ENG` gibi bir Linear ekip anahtarı. |
| `webhook_secret` | Webhook'ların imzalandığı gizli değer. GitHub için boş bırakın: Yuva bir tane üretir ve bir kez gösterir. |

Yanıtta `webhook_url` bulunur. Belirteci değiştirmek (`PATCH /v1/issue-integrations/{id}`) önce yenisini
denetler; `POST /v1/issue-integrations/{id}/webhook-secret` yeni bir gizli değer üretir. Bağlantıyı
kesmek (`DELETE`) belirteci ve o izleyicinin kayıtlarına olan tüm bağlantıları siler; ayarlanmış ilerleme
kalır. Bağlama, değiştirme ve bağlantıyı kesme [denetim kaydına](operations.md#denetim-günlüğü) yazılır.

## 2. Webhook'u ekleyin

Yuva bir değişikliği hemen duysun diye:

- **GitHub:** depoda (ya da kuruluşta) Settings › Webhooks › Add webhook: Payload URL `webhook_url`,
  içerik türü `application/json`, Secret size gösterilen `webhook_secret`, ve "Let me select individual
  events" › Issues.
- **Linear:** Settings › API › Webhooks › New webhook: URL `webhook_url`, Issues için veri değişikliği
  olayları. Linear'ın gösterdiği imzalama değerini kopyalayıp gönderin:
  `{"webhook_secret": "lin_wh_…"}` ile `PATCH /v1/issue-integrations/{id}`.

Doğru imzası olmayan istek reddedilir. Yuva webhook'un söylediğine güvenmez: kaydı belirtecinizle yeniden
okur. Webhook olmadan da, varken yedek olarak da Yuva bağlı her kaydı yaklaşık saatte bir yeniden okur; başarısız
bir okumadan sonra her seferinde daha uzun, en fazla bir gün bekler.

## 3. Kayıt oluşturun ya da bağlayın

Bir geri bildirim konuşmasında:

```sh
# oluştur
curl -X POST "$YUVA_URL/v1/conversations/$ID/issues" -H "Authorization: Bearer $YUVA_KEY" \
  -H "Content-Type: application/json" \
  -d '{"integration_id": "…", "title": "Dark mode", "body": "Several customers ask for it."}'
# var olanı bağla
curl -X POST "$YUVA_URL/v1/conversations/$ID/issues" -H "Authorization: Bearer $YUVA_KEY" \
  -H "Content-Type: application/json" \
  -d '{"integration_id": "…", "url": "https://github.com/acme/app/issues/12"}'
```

`url`, `owner/repo#12`, bir Linear kayıt adresi ya da `ENG-123` de alır. Oluşturulan kaydın başlığı ve
gövdesi sizin yazdığınızdır; ardından paneldeki konuşmaya geri bağlantı veren bir satır gelir.
Konuşmadan başka hiçbir şey izleyiciye gitmez: müşterinin adı, adresi ve mesajları, siz gövdeye
yazmadıkça Yuva'da kalır.

Fikirler de `/v1/discussions/{id}/issues` üzerinde aynı biçimde kayıt alır, ama bir fikir kayıtlarını ve
durumlarını yalnızca gösterir: bir kayıt fikrin aşamasını hiçbir zaman değiştirmez, bu ekibinizin kararı
olarak kalır. Gelen kutusuna erişimi olan
temsilciler oluşturur, bağlar ve bağlantıyı kaldırır (`DELETE …/issues/{linkId}`). Bir geri bildirim ya
da fikir en fazla 10 kayıt alır.

## 4. Durum ilerlemeyi nasıl ayarlar

| GitHub | Linear | Geri bildirim ilerlemesi |
|---|---|---|
| open | Triage, Backlog, Todo (unstarted) | Planlandı |
| – | In Progress (started) | Sürüyor |
| completed olarak kapatıldı | Done (completed) | Tamamlandı |
| not planned ya da duplicate olarak kapatıldı | Canceled | Yapılmayacak |

İlerleme, kayıt bağlandığında ve kaydın durumu her değiştiğinde ayarlanır. Bir geri bildirimin
ilerlemesini elle ayarlarsanız, kayıt yeniden değişene kadar öyle kalır. Değişiklik, kaydı bağlayan
kişi ya da API anahtarı adına yapılır.

Müşteri ilerlemeyi konuşmasında bir satır olarak görür. Ekibiniz onu hangi kaydın ayarladığını görür.

## 5. API anahtarları, webhook'lar ve asistanlar

- Yetkiler: `integrations:read` izleyicileri listeler, `integrations:manage` bağlar (gelen kutularıyla
  sınırlı anahtarlar için değil), `issues:write` ile `conversations:read` kayıt oluşturur ve bağlar.
- Bir konuşmanın ve bir fikrin `issues` alanı bağlantılarını listeler. [Webhook'lar](webhooks.md) ve
  `/v1/realtime` `issue_link.created`, `issue_link.updated` ve `issue_link.deleted` gönderir; ilerlemenin
  kendisi her zamanki `conversation.updated` olayını gönderir.
- [MCP](mcp.md): bir geri bildirim üzerinde `list_issue_integrations`, `link_issue` ve `create_issue`.
