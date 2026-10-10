# Kimlik token'ları

Kimlik token'ı, uygulamanızda oturum açmış bir kullanıcının [widget'ta](widget.md) ya da
[mobil SDK'larda](mobile.md) kendi kimliğiyle sohbet etmesini sağlar. Token'ı backend'iniz imzalar;
Yuva kullanıcılarınızı ya da parolalarını hiç görmez.

## 1. Kimlik anahtarını alın

Her gelen kutusunun bir kimlik anahtarı (`yuva_is_…`) vardır. Panel onu yalnızca bir kez, gelen
kutusu oluşturulduğunda ya da anahtar yenilendiğinde gösterir. Yalnızca sunucunuzda tutun.

## 2. O anki kullanıcı için token imzalayın

Token döndüren bir uç nokta ekleyin. Go ile:

```go
token, err := identity.Sign(os.Getenv("YUVA_IDENTITY_SECRET"), identity.Claims{
	Subject: user.ID, Email: user.Email, EmailVerified: user.EmailConfirmed, Name: user.Name,
})
```

`identity`, `github.com/productdevbook/yuva/sdk/go/identity` paketidir. Diğer dillerde herhangi bir
JWT kütüphanesi işinizi görür: [claim'leri](#claimler) HS256 ile, anahtar olarak gizli değerin
tamamını kullanarak imzalayın.

## 3. Widget'a ya da SDK'ya verin

Onlara token'ı getiren bir fonksiyon verin: [widget'ta](widget.md#3-oturum-açmış-kullanıcılar)
`setIdentityToken`, [mobil SDK'larda](mobile.md) `identityToken`. Kullanıcı uygulamanızdan çıkış
yaptığında `signOut()` çağırın.

Aynı `sub` her zaman aynı kişiyi bulur. Kullanıcının o tarayıcıda ya da cihazda anonim olarak
başlattığı konuşmalar ona geçer.

## Kullanıcıyı silme

Bir kullanıcı hesabını sildiğinde, kişisini, konuşmalarını ve dosyalarını bir API anahtarıyla
silin:

```sh
curl -X DELETE -H "Authorization: Bearer $YUVA_API_KEY" \
  "https://support.example.com/v1/contacts/by-external-id?inbox_id=<inbox id>&external_id=<your user id>"
```

## Başvuru

### Claim'ler

| Claim | Zorunlu | Anlamı |
|---|---|---|
| `sub` | evet | Sizin kullanıcı id'niz, 1 ile 200 karakter arası. |
| `exp` | evet | Bitiş zamanı, Unix saniyesi, en fazla 10 dakika sonrası. Go yardımcısı 5 dakika kullanır (`Claims.TTL`). |
| `iat`, `nbf` | hayır | Unix saniyesi, gelecekte olamaz. |
| `jti` | hayır | Token id'si, 1 ile 200 karakter arası; token o zaman yalnızca bir oturum başlatır. |
| `email` | hayır | Kullanıcının adresi. |
| `email_verified` | hayır | `email`'i doğruladıysanız `true`. Yalnızca doğrulanmış bir adres mevcut bir kişiye bağlanır. |
| `name` | hayır | Görünen ad, en fazla 200 karakter. |
| `locale` | hayır | Dil etiketi, ör. `en`. |
| `attrs` | hayır | Üyelere gösterilen JSON nesnesi (plan, uygulama sürümü, …), en fazla 16 KiB. |

### Kurallar

| Konu | Ayrıntı |
|---|---|
| İmza | Yalnızca HS256, kanalın gelen kutusunun kimlik anahtarıyla. 30 saniyelik saat farkına izin verilir. |
| Hata | Nedeniyle birlikte `401 invalid_identity_token`. |
| Değişim | `channel_key` ve `identity_token` ile `POST /client/v1/session`; widget ve SDK'lar bunu yapar. |
| Oturum | Son kullanımdan sonra 7 gün sürer; `DELETE /client/v1/session` oturumu bitirir. |
| Yenileme | Panelden ya da `POST /v1/inboxes/{inboxId}/identity-secret` ile (sahipler ve yöneticiler). Eski token'lar hemen geçersiz olur. `yuva inbox create` ile oluşturulan bir gelen kutusunun anahtar alması için bir kez yenilenmesi gerekir. |
| Silme | [Webhook'larınıza](webhooks.md) `contact.deleted` gönderir. |
