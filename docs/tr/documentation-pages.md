# Dokümantasyon sayfaları

Dokümantasyon sitenize "Bu sayfa işinize yaradı mı?" sorusunu ve bir soru kutusunu ekleyin,
yanıtları Yuva'nın gelen kutusunda okuyun. Sayfalarınız olduğu yerde kalır; iki web bileşeni
değerlendirmeleri, geri bildirimleri ve soruları Yuva'ya gönderir.

## 1. Bir sohbet kanalı

[Web widget'ı](widget.md#1-sohbet-kanalı-oluşturun) için olduğu gibi kurulmuş bir sohbet kanalı
kullanın; tek bir kanal ikisine de hizmet edebilir.

- Dokümantasyon sitesinin origin'ini kanalın **izin verilen kaynaklarına** ekleyin
  (`https://docs.example.com`, ayrıca `http://localhost:3000` gibi geliştirme sunucunuz). Başka bir
  origin'de öğeler boş kalır.
- Gelen kutusuna bir [e-posta kanalı](email.md) ekleyin: yanıtlar okuyuculara e-postayla ulaşır.

## 2. Öğeleri ekleyin

### Düz HTML

```html
<script src="https://support.example.com/yuva-docs.js" defer></script>

<yuva-page-questions channel="yuva_pk_xxxxxxxxxxxxxxxx"></yuva-page-questions>
<yuva-page-feedback channel="yuva_pk_xxxxxxxxxxxxxxxx"></yuva-page-feedback>
```

Bunları her doküman sayfasının paylaştığı şablona, makalenin arkasına koyun. `/yuva-docs.js`
dosyasını Yuva sunucunuz sunar.

npm'de `useyuva` 0.0.6'dan sonraki sürümden itibaren `useyuva/docs` ve `useyuva/react` olarak
bulunurlar. O zamana kadar sunucudaki betiği kullanın ya da örneklerdeki gibi `sdk/js`'e yol
üzerinden bağımlı olun.

### Nuxt Content

Çalıştırılabilir örnek: [`examples/nuxt-content`](../../examples/nuxt-content)
([nasıl çalıştırılır](../../examples/README.md#nuxt-content)). Eklenecek satırlar:

```ts
// app/plugins/yuva.client.ts
import "useyuva/docs";
export default defineNuxtPlugin(() => {});
```

```ts
// nuxt.config.ts
export default defineNuxtConfig({
  vue: { compilerOptions: { isCustomElement: (tag) => tag.startsWith("yuva-") } },
  runtimeConfig: { public: { yuvaChannel: "", yuvaServer: "" } },
});
```

```vue
<!-- app/pages/[...slug].vue, <ContentRenderer> sonrasına -->
<yuva-page-questions :channel="config.yuvaChannel" :server="config.yuvaServer" :page-title="page.title" />
<yuva-page-feedback :channel="config.yuvaChannel" :server="config.yuvaServer" :page-title="page.title" />
```

`config`, `useRuntimeConfig().public` değeridir. Yeni bir sitede `nuxt build` "Nuxt Content
requires better-sqlite3" hatasıyla durur: paketi kurun ya da örnekteki gibi
`content.experimental.sqliteConnector: "native"` atayın. Sayfa bir `content` koleksiyonunu sorgular;
onu örneğin [`content.config.ts`](../../examples/nuxt-content/content.config.ts) dosyasındaki gibi
tanımlayın.

### Fumadocs

Çalıştırılabilir örnek: [`examples/fumadocs`](../../examples/fumadocs)
([nasıl çalıştırılır](../../examples/README.md#fumadocs)). Doküman sayfası bir server component
olduğu için React bileşenlerini bir client component içine sarın:

```tsx
// components/yuva.tsx
"use client";
import { YuvaPageFeedback, YuvaPageQuestions } from "useyuva/react";

const channel = process.env.NEXT_PUBLIC_YUVA_CHANNEL!;
const server = process.env.NEXT_PUBLIC_YUVA_SERVER;

export const PageQuestions = ({ title }: { title: string }) =>
  <YuvaPageQuestions channel={channel} server={server} pageTitle={title} />;
export const PageFeedback = ({ title }: { title: string }) =>
  <YuvaPageFeedback channel={channel} server={server} pageTitle={title} />;
```

```tsx
// app/docs/[[...slug]]/page.tsx, <DocsBody> sonrasına
<PageQuestions title={page.data.title} />
<PageFeedback title={page.data.title} />
```

Fumadocs'un kendi `<Feedback>` bileşenini eklediyseniz onu `<PageFeedback />` ile değiştirin ya da
tutup yalnızca `<PageQuestions />` ekleyin. `useyuva` `sdk/js`'ten yol üzerinden bağlıysa Next,
React'in ikinci bir kopyasını yükleyebilir; örnekteki `next.config.mjs` gibi `turbopack.root`
atayın.

### Diğer site üreticileri

Betiği (ya da `useyuva/docs`'u) yükleyin ve öğeleri makalenin altına koyun:

- **Starlight**: öğeleri override edilmiş bir `Footer`'a ekleyin.
- **VitePress**: `doc-after` slot'unu kullanın; `isCustomElement`'ı Nuxt'taki gibi atayın.
- **Docusaurus**: `DocItem/Footer`'ı swizzle edin (wrap).
- **MkDocs, Hugo, Jekyll ve diğerleri**: [düz HTML](#düz-html) gibi.

## Okuyucular ve ekibiniz ne görür

- **Okuyucular** bir sayfayı Evet ya da Hayır ile değerlendirir (anonim bir sayaç), ardından geri
  bildirim yazabilir; herkes soru sorabilir. İkisi de yanıt için isteğe bağlı bir e-posta adresi
  alır ve gizli kalır.
- **Ekibiniz** geri bildirimleri ve soruları gelen kutusunda `feedback` ve `question` konuşmaları
  olarak alır. **Dokümanlar** görünümü sayfaları 7, 30 ya da 90 gündeki değerlendirmeleri, geri
  bildirimleri ve sorularıyla listeler. Yanıtladığınız bir soruda **Sayfada yayımla**, düzenlenmiş
  soruyu ve yanıtı bir dakika içinde o sayfadaki "Sorular ve cevaplar" bölümüne koyar.

## Bilmekte fayda var

- **Bir sayfa, sorgu ve fragment olmadan URL'sidir.** Dile göre ayrılan URL'ler
  (`/en/docs/install`, `/de/docs/install`) kendi değerlendirmeleri olan ayrı sayfalardır; birlikte
  saymak için `page` olarak tek bir URL verin ve `locale`'i sayfanın diline atayın.
- **Oturum açmış okuyucular**: öğelere `setIdentityToken` ile (React'te `identityToken` prop'u) bir
  [kimlik token'ı](identity.md) verin. E-posta alanını görmezler ve yanıtlar token'daki adrese gider.
  Kanalda **Anonim ziyaretçiler** açık değilse geri bildirim ve sorular token ister;
  değerlendirmeler asla istemez.
- **Content Security Policy**: Yuva sunucusuna `script-src` (`/yuva-docs.js` için) ve `connect-src`
  içinde izin verin.
- **Öğeler boş kalıyor**: origin'e izin verilmemiş ya da anahtar yanlış; tarayıcı konsoluna bakın.

## Başvuru

### Nitelikler

| Nitelik | Anlamı |
|---|---|
| `channel` | Sohbet kanalının genel anahtarı (`yuva_pk_…`). Zorunlu. |
| `server` | Yuva sunucusunun adresi. Varsayılan, `yuva-docs.js`'nin yüklendiği origin. |
| `locale` | `en` ya da `tr`. Varsayılan tarayıcının dili, o da yoksa İngilizce. |
| `dir` | `ltr` ya da `rtl`. Varsayılan, dilin yazım yönü. |
| `identity-token` | İmzalı bir kimlik token'ı. Genellikle `setIdentityToken` daha iyidir. |
| `page` | Sayfanın URL'si. Varsayılan o anki URL; istemci tarafı gezinme izlenir. |
| `page-title` | Sayfanın başlığı. Varsayılan `document.title`. |

React sarmalayıcıları aynılarını prop olarak alır (`channel`, `server`, `locale`, `dir`, `page`,
`pageTitle`, `identityToken`), ayrıca `className` ve `style`.

### Özellikler ve metotlar

| Öğe | Üye | Anlamı |
|---|---|---|
| ikisi de | `channel`, `server`, `locale`, `page`, `pageTitle` | Özellik olarak nitelikler. |
| ikisi de | `setIdentityToken(fn)` | Token, oturum açmamış okuyucu için `null` döndüren bir fonksiyon. |
| `yuva-page-feedback` | `rating` | Bu tarayıcıda bu sayfa için `"up"`, `"down"` ya da `null`. |
| `yuva-page-feedback` | `rate("up" \| "down")` | Evet ya da Hayır'a basmakla aynı. |
| `yuva-page-questions` | `answers` | Gösterilen yayımlanmış yanıtlar. |
| `yuva-page-questions` | `reload()` | Yanıtları yeniden getirir. |

### Olaylar

Shadow DOM'dan dışarı kabarcıklanırlar (bubble), bu yüzden `document` üzerinde dinleyebilirsiniz.

| Olay | Öğe | `detail` | React prop'u |
|---|---|---|---|
| `yuva-rating` | `yuva-page-feedback` | `{ page, rating, previous }` | `onRating` |
| `yuva-feedback` | `yuva-page-feedback` | `{ page, rating, conversation_id }` | `onFeedback` |
| `yuva-question` | `yuva-page-questions` | `{ page, conversation_id }` | `onQuestion` |

### CSS değişkenleri

Öğeler `color` ve `font` değerlerini sayfadan alır ve koyu temaya uyar. Vurgu rengi kanalın düğme
renginden gelir; kendi renginizi kullanmak için `!important` ile atayın.

| Değişken | Kullanıldığı yer |
|---|---|
| `--yuva-accent` | Düğmeler, seçili değerlendirme, odak halkaları, yanıtlardaki bağlantılar. |
| `--yuva-on-accent` | Vurgu rengi üzerindeki metin. |
| `--yuva-border` | Düğmelerin, alanların ve yanıtların kenarlıkları. |
| `--yuva-soft` | Seçili değerlendirmenin arka planı, yanıtlardaki kod. |
| `--yuva-danger` | Hata mesajları. |

### Uç noktalar

| Uç nokta | Kullanıldığı yer |
|---|---|
| `POST /client/v1/channels/{channel_key}/page-ratings` | Bir değerlendirmeyi sayar. |
| `POST /client/v1/feedback` | `page_url`, `page_title`, `rating` ile geri bildirim gönderir. |
| `POST /client/v1/questions` | Soru sorar. |
| `GET /client/v1/channels/{channel_key}/page-answers?page=` | Bir sayfanın yayımlanmış yanıtları (bir dakika önbelleğe alınır). |
| `POST /v1/conversations/{conversationId}/publish` | Bir soruyu ve yanıtını yayımlar. |
| `GET /v1/page-answers`, `GET`, `PATCH`, `DELETE /v1/page-answers/{pageAnswerId}` | Yanıtları listeler, düzenler ve yayından kaldırır. |
| `GET /v1/docs/pages`, `GET /v1/docs/summary`, `GET /v1/docs/page` | Dokümanlar görünümü. |
| `GET /v1/conversations?page=&kind=` | Bir sayfanın geri bildirimleri ya da soruları. |

Ayrıntılar: [API sözleşmesi](../../openapi/openapi.yaml).
