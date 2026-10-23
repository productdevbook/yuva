# Tek oturum açma

Çalışma alanınızın üyeleri Yuva'ya şirketinizin kimlik sağlayıcısı üzerinden OpenID Connect ya da
SAML 2.0 ile girebilir: Google Workspace, Microsoft Entra ID, Okta, Keycloak ya da OpenID Connect keşif
belgesi veya SAML metadata'sı yayımlayan her sağlayıcı. Bir çalışma alanı ikisinden birini kullanır.
Yuva kişinin kim olduğunu sağlayıcıdan öğrenir; hangi sağlayıcıyı kullanacağına kişinin e-posta
adresi karar verir.

Bu ayarları yalnızca sahipler değiştirir: Ayarlar › Tek oturum açma'da ya da API üzerinden
(`GET /v1/sso` ve aşağıdaki uç noktalar, üye oturumuyla).

## Nasıl çalışır

1. Yuva'yı sağlayıcınızda bir uygulama olarak kaydeder, Yuva'ya issuer URL'sini, istemci kimliğini
   ve istemci gizli anahtarını (OpenID Connect) ya da sağlayıcının metadata'sını (SAML, bkz.
   [OpenID Connect yerine SAML 2.0](#openid-connect-yerine-saml-20)) verirsiniz.
2. E-posta alan adlarınızı (`example.com`) bir DNS TXT kaydıyla sahiplenirsiniz. Bir alan adı bir Yuva
   sunucusunda tek bir çalışma alanına aittir.
3. Giriş sayfasında kişi e-posta adresini yazar. Alan adı sahiplenilmişse tarayıcı sağlayıcınıza
   gider ve oturum açmış olarak döner.

Yuva `openid email profile` (ve eklediğiniz kapsamları) ister, PKCE, `state` ve `nonce` ile
yetkilendirme kodu akışını kullanır ve `id_token`'ın imzasını sağlayıcının anahtarlarıyla (sağlayıcı
anahtarlarını değiştirdiğinde yenileri alınır), issuer'ını, hedef kitlesini, süresini ve nonce'unu
denetler. Token kişinin e-posta adresini `email_verified: true` ile taşımalı ve adres doğrulanmış
alan adlarınızdan birinde olmalıdır.

Sağlayıcınız üzerinden yapılan giriş yalnızca sizin çalışma alanınızda (ve tartışma sayfalarında okur
olarak) geçerlidir. Başka çalışma alanlarının da üyesi olan biri oralara e-postayla gelen bir kodla ya
da geçiş anahtarıyla girer; başka bir çalışma alanını açtığında panel bunu ister. Böyle bir giriş
geçiş anahtarı ekleyemez ya da kaldıramaz, çalışma alanı oluşturamaz, hesabı silemez
(`sso_session_scope`) ve yalnızca sizin çalışma alanınızın davetlerini kabul eder. Yuva 0.1.0'a kadar
bir sağlayıcı üzerinden giriş kişinin bütün çalışma alanlarında geçerliydi; artık değil, böylece bir
çalışma alanının kimlik sağlayıcısını yöneten kişi birinin adına onun diğer çalışma alanlarında iş
yapamaz.

Yuva, kişi bir çalışma alanına onun sağlayıcısı üzerinden ilk kez giriş yaptığında ve hesabına her
geçiş anahtarı eklendiğinde ya da kaldırıldığında kişiye e-posta gönderir.

## 1. Yuva'yı sağlayıcınızda kaydedin

Şu yönlendirme URI'sini kullanın (Ayarlar › Tek oturum açma gösterir; `GET /v1/sso` içinde
`redirect_uri`):

```
https://<Yuva adresiniz>/auth/sso/callback
```

### Google Workspace

1. Google Cloud konsolunda APIs & Services › OAuth consent screen'i açın ve **Internal** seçin;
   böylece yalnızca kuruluşunuzun hesapları giriş yapabilir.
2. Credentials › Create credentials › OAuth client ID, tür **Web application**, yukarıdaki
   yönlendirme URI'siyle.
3. Issuer: `https://accounts.google.com`. Google'ın token'ları grup taşımaz; izin verilen grupları
   boş bırakın.

### Microsoft Entra ID

1. Entra yönetim merkezi › App registrations › New registration, "Accounts in this organizational
   directory only", yukarıdaki yönlendirme URI'siyle **Web** platformu.
2. Certificates & secrets › New client secret.
3. Token configuration › Add optional claim › ID: `email` ve `xms_edov` ekleyin. Entra
   `email_verified` göndermez; Yuva onun yerine `xms_edov: true`'yu (adresin alan adı kiracınızda
   doğrulanmış) kabul eder.
4. Issuer: `https://login.microsoftonline.com/<kiracı kimliği>/v2.0`. Ortak `common` ve
   `organizations` issuer'ları kabul edilmez.
5. Gruplar için Token configuration'da `groups` claim'ini ekleyin; grup nesne kimliklerini listeler,
   bu yüzden o kimliklere izin verin.

### Okta

1. Applications › Create App Integration › OIDC, **Web Application**, oturum açma yönlendirme
   URI'si olarak yukarıdaki URI. Kullanabilecek kişileri ya da grupları atayın.
2. Issuer: `https://<kuruluşunuz>.okta.com` (kuruluş yetkilendirme sunucusu) ya da özel
   yetkilendirme sunucunuzun issuer'ı, örneğin `https://<kuruluşunuz>.okta.com/oauth2/default`.
3. Gruplar için ID token'a bir `groups` claim'i ekleyin (Sign On › OpenID Connect ID Token › Groups
   claim filter) ve Yuva'da `groups` kapsamını ekleyin.

### Keycloak

1. Realm'inizde Clients › Create client, tür **OpenID Connect**, istemci kimlik doğrulaması açık,
   geçerli yönlendirme URI'si olarak yukarıdaki URI.
2. Issuer: `https://<keycloak sunucusu>/realms/<realm>`.
3. Gruplar için istemcinin kendi kapsamına token claim adı `groups` olan ve "Add to ID token" açık
   bir **Group Membership** eşleyicisi ekleyin.

## 2. Sağlayıcıyı bağlayın

Issuer'ı, istemci kimliğini ve istemci gizli anahtarını girin (`PUT /v1/sso/oidc`). Yuva kaydetmeden
önce `<issuer>/.well-known/openid-configuration` belgesini okur; issuer belgedeki `issuer` ile birebir
aynı olmalıdır. Gizli anahtar şifreli saklanır ve bir daha gösterilmez; sonradan boş bırakırsanız
saklanan kalır.

İsteğe bağlı olarak:

- **Kapsamlar**: Okta için `groups` gibi ek kapsamlar.
- **Grup claim'i** (varsayılan `groups`) ve **izin verilen gruplar**: grup listelediğinizde yalnızca
  `id_token`'ı bunlardan birini adlandıran kişiler giriş yapar.

Issuer herkese açık bir `https` adresinde olmalıdır. İşletmecisi `YUVA_WEBHOOK_ALLOW_PRIVATE=true`
ayarlamış bir sunucu, aynı ağdaki bir Keycloak gibi özel adreslerdeki sağlayıcılara da ulaşır;
link-local ve bulut metadata adresleri her zaman reddedilir.

## OpenID Connect yerine SAML 2.0

Sağlayıcınız yalnızca SAML sunuyorsa ya da onu tercih ediyorsanız 1. ve 2. adımlar yerine bunu
yapın. Bir SAML sağlayıcısını kaydetmek OpenID Connect sağlayıcısının yerini alır, tersi de geçerlidir;
alan adları ve kurallar kalır.

SAML için Yuva'nın `https` üzerinde olması gerekir (`YUVA_PUBLIC_URL`): sağlayıcı yanıtını kendi
sitesinden Yuva'ya gönderir ve tarayıcılar bu yanıtı girişe bağlayan çerezi yalnızca Secure bir çerez
olarak gönderir. Düz http'de bu yalnızca `localhost`'ta çalışır; başka yerde SAML sağlayıcısını
kaydetmek `saml_requires_https` yanıtını verir.

Giriş her zaman Yuva'nın giriş sayfasında başlar (hizmet sağlayıcı başlatır). Sağlayıcının Yuva
istemeden gönderdiği bir yanıt, örneğin sağlayıcının uygulama panosundaki bir kutucuktan gelen,
reddedilir: uygulamayı panodan gizleyin ya da kutucuğu Yuva'nın giriş sayfasına yönlendirin.

Sağlayıcıda şu değerleri kaydedin (Ayarlar › Tek oturum açma gösterir; `GET /v1/sso` içinde
`saml_sp`):

| Sağlayıcıda | Değer |
|---|---|
| Entity ID, Audience URI, Identifier | `https://<Yuva adresiniz>/auth/saml/metadata/<çalışma alanı kimliği>` |
| ACS URL, Reply URL, single sign-on URL | `https://<Yuva adresiniz>/auth/saml/acs` (HTTP-POST) |
| NameID biçimi | `urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress`, kişinin e-posta adresi |

Entity ID aynı zamanda Yuva'nın çalışma alanınız için metadata'sının adresidir; onu okuyabilen
sağlayıcılar için. Yuva isteklerini imzalamaz ve şifreli assertion kabul etmez: istek imzalamayı ve
assertion şifrelemeyi kapalı bırakın.

Yuva her yanıtı denetler: yanıt ya da assertion, sağlayıcınızın metadata'sındaki sertifikalardan
biriyle imzalanmış olmalıdır (imzasız yanıtlar, başka anahtarlar ve değiştirilmiş içerik reddedilir);
tam olarak bir assertion taşımalı, bu tarayıcının son 10 dakikada başlattığı bir isteğe cevap olmalı
ve bir kez kullanılmalı, hedef ve alıcı olarak Yuva'nın ACS URL'sini, hedef kitle olarak Yuva'nın
entity ID'sini adlandırmalı ve geçerlilik süresi içinde olmalıdır (3 dakikalık saat farkına izin
verilir, en fazla 90 saniye önce düzenlenmiş). E-posta adresi NameID'den ya da adını verdiğiniz bir
özellikten (attribute) alınır ve doğrulanmış alan adlarınızdan birinde olmalıdır.

### Okta

1. Applications › Create App Integration › **SAML 2.0**.
2. Single sign-on URL: ACS URL, "Use this for Recipient URL and Destination URL" açık. Audience URI:
   entity ID. Name ID format **EmailAddress**, application username **Email**.
3. Gruplar için `groups` adlı bir group attribute statement ekleyin ve bir filtre verin (örneğin
   "Matches regex" `.*`).
4. Kullanabilecek kişileri ya da grupları atayın ve Sign On sekmesinden **Metadata URL**'yi kopyalayın.

### Microsoft Entra ID

1. Entra yönetim merkezi › Enterprise applications › New application › Create your own
   application, "Integrate any other application you don't find in the gallery".
2. Single sign-on › **SAML**. Basic SAML Configuration: Identifier (Entity ID) olarak entity ID, Reply
   URL olarak ACS URL.
3. Attributes & Claims: Unique User Identifier (Name ID) değerini `user.mail`, biçimini **Email
   address** yapın.
4. Gruplar için bir group claim ekleyin; Entra bunu
   `http://schemas.microsoft.com/ws/2008/06/identity/claims/groups` diye adlandırır ve grup nesne
   kimliklerini listeler; Yuva'da grup özelliği olarak bu adı kullanın ve bu kimliklere izin verin.
5. SAML Certificates bölümünden **App Federation Metadata Url**'yi kopyalayın.

### Google Workspace

1. Yönetici konsolu › Apps › Web and mobile apps › Add app › **Add custom SAML app**.
2. Google identity provider details sayfasında metadata'yı indirin. Google'ın metadata URL'si yoktur;
   dosyayı Yuva'ya yapıştırın ve Google'ın sertifikası yenilendiğinde yenisini yapıştırın.
3. Service provider details: yukarıdaki ACS URL ve Entity ID, Name ID format **EMAIL**, Name ID
   **Basic Information › Primary email**.
4. Gruplar için Group membership'i `groups` adlı bir uygulama özelliğine eşleyin.
5. Uygulamayı kullanabilecek kuruluş birimleri ya da gruplar için açın.

### Keycloak

1. Realm'inizde Clients › Create client, tür **SAML**, Client ID olarak entity ID.
2. Valid redirect URIs: ACS URL. Name ID format **email**, "Force name ID format" açık ve "Sign
   documents" ya da "Sign assertions" açık.
3. Keys: "Client signature required" seçeneğini kapatın; Yuva isteklerini imzalamaz.
4. Gruplar için istemcinin dedicated scope'una özellik adı `groups` olan ve "Full group path"
   kapalı bir **Group list** mapper ekleyin.
5. Metadata URL'si `https://<keycloak sunucusu>/realms/<realm>/protocol/saml/descriptor` olur.

### SAML sağlayıcısını bağlayın

Yuva'ya metadata'yı verin (`PUT /v1/sso/saml`): URL'sini (`metadata_url`, önerilir) ya da belgenin
kendisini (`metadata_xml`). Yuva buradan sağlayıcının entity ID'sini, HTTP-Redirect bağlaması için tek
oturum açma URL'sini ve imzalama sertifikalarını alır; ayarlar her sertifikanın konusunu, geçerlilik
süresini ve SHA-256 parmak izini gösterir. Süresi dolmuş sertifikalar alınmaz ve yanıtta adlandırılır;
şu an geçerli bir sertifikası olmayan metadata reddedilir. URL'den gelen metadata, bir giriş
başladığında kopya bir saatten eskiyse yeniden okunur; böylece sağlayıcınızın değiştirdiği bir
sertifika siz bir şey yapmadan kullanılır. URL okunamazsa Yuva saklanan kopyayı kullanır ve 15 dakika
sonra yeniden dener. Yapıştırılan metadata yalnızca yeniden yapıştırdığınızda değişir. Yalnızca aşağıdaki
seçenekleri değiştirmek (ve URL'yi hemen yeniden okutmak) için metadata vermeden yeniden kaydedin.

İsteğe bağlı olarak:

- **E-posta özelliği**: NameID adres değilse, adresi taşıyan özellik (adıyla ya da friendly name'iyle).
- **Grup özelliği** (varsayılan `groups`) ve **izin verilen gruplar**: grup listelediğinizde yalnızca
  assertion'ı bunlardan birini adlandıran kişiler giriş yapar.

Issuer için geçerli adres kuralları burada da geçerlidir: herkese açık `https` adresleri, özel
adresler yalnızca `YUVA_WEBHOOK_ALLOW_PRIVATE=true` ile.

## 3. Alan adlarınızı sahiplenin

Her e-posta alan adını ekleyin (`POST /v1/sso/domains`) ve gösterdiği TXT kaydını yayımlayın:

```
_yuva-sso.example.com.  TXT  "yuva-verify-…"
```

Sonra Doğrula'ya basın (`POST /v1/sso/domains/{id}/verify`). Yalnızca tam alan adı sahiplenilir,
alt alan adları değil; kişilerin böyle adresleri varsa `eu.example.com`'u ayrıca ekleyin. Sunucudaki
başka bir çalışma alanının doğruladığı bir alan adı sahiplenilemez (`domain_taken`). Kaydı yerinde
bırakın: Yuva onu her gün yeniden sorgular. Kayıt 7 gün boyunca bulunamazsa alan adı beklemeye döner,
sahiplere e-posta gider, son doğrulanmış alan adınızsa tek oturum açma artık zorunlu olmaz,
`sso_domain.lapsed` kaydedilir ve başka bir çalışma alanı alan adını sahiplenebilir. Kayıt
bulunamadığı sürece alan adı denetimin ne bulduğunu gösterir; kaydı yeniden yayımlayıp Doğrula'ya
basarak bunu temizleyin.

## 4. Giriş kurallarını seçin

`PATCH /v1/sso`:

- **Tek oturum açma zorunlu** (`sso_required`): alan adlarınızdaki kişiler bu çalışma alanında
  yalnızca sağlayıcınız üzerinden giriş yaptıktan sonra çalışır. E-postayla gelen kod ya da geçiş
  anahtarıyla açılan oturum onları yine içeri alır, diğer çalışma alanlarına ve tartışmalarda okur
  olarak; ama bu çalışma alanı `sso_required` yanıtını verir ve panel onları sağlayıcınıza gönderir.
  Açtığınızda bu kişilerin sağlayıcınız üzerinden açmadığı bütün oturumları kapanır; diğer çalışma alanları için de, oralara yeniden giriş yaparlar. Sahipler her oturumla
  muaftır; böylece bozulan bir sağlayıcı sizi asla dışarıda bırakmaz. Bu kişilerin bağlı uygulamaları
  (OAuth, MCP) sağlayıcı üzerinden giriş yapıp onları yeniden onaylayana kadar burada çalışmaz; API
  anahtarları etkilenmez. Slack başlıklarındaki yanıtları reddedilir; sağlayıcı üzerinden giriş
  yapmadan sıraya koydukları işler (bağlı konuşmalara yanıtlar, iş takibi ve yol haritası ilerlemesi)
  durur. Bir sağlayıcı ve doğrulanmış bir alan adı gerekir; sağlayıcıyı ya da son
  doğrulanmış alan adını kaldırmak onu kapatır.
- **Anında üyelik** (`jit`, varsayılan kapalı): alan adlarınızdan biri sağlayıcı üzerinden giriş
  yapar ve üye değilse varsayılan rolle katılır (`jit_role`: `agent` ya da `admin`; asla `owner`
  değil). O rolün ve olağan gelen kutusu erişim kurallarının verdiğini görür. Kapalıyken kişiler
  eskisi gibi davet edilir; sağlayıcıyla giriş yapan davetli kişi daveti kabul etmiş olur.

Alan adlarınızdan üye olmayan, çalışma alanınıza daveti bulunmayan ve anında üyelikle de katılmayan
kişiler, bir Yuva hesapları olsa bile reddedilir (`not_member`).

## Denetim kaydı

Her değişiklik ve her giriş çalışma alanının denetim kaydındadır (Ayarlar › Denetim kaydı):
`sso.provider_created`, `sso.provider_updated`, `sso.provider_deleted`, `sso.settings_changed`,
`sso_domain.added`, `sso_domain.verified`, `sso_domain.deleted`, `sso_domain.lapsed`, `sso.signed_in`, nedeniyle
`sso.sign_in_refused` ve anında üyeler için `via: sso` ile `member.joined`. Yuva SAML metadata'sını
yeniden okuyup yeni sertifikalar bulduğunda sistem `via: metadata_refresh` ile
`sso.provider_updated` kaydeder.

## Giriş başarısız olduğunda

Giriş sayfası neyin yanlış gittiğini gösterir (`/sign-in?sso_error=…`):

| Kod | Anlamı |
|---|---|
| `expired` | Giriş 10 dakikadan uzun sürdü, zaten kullanıldı ya da başka bir tarayıcıda başladı; SAML'de ayrıca Yuva'nın istemediği ya da SAML yanıtı olmayan bir yanıt. |
| `denied` | Sağlayıcı reddetti ya da kişi vazgeçti. |
| `failed` | Yuva sağlayıcıya ulaşamadı ya da kodu değiştiremedi; istemci gizli anahtarını denetleyin. |
| `invalid_token` | `id_token` bir denetimden geçemedi (imza, issuer, hedef kitle, süre, nonce) ya da SAML yanıtı geçemedi (imza, sertifika, hedef, alıcı, hedef kitle, geçerlilik, daha önce kullanılmış assertion); denetim kaydı hangi denetim olduğunu yazar. |
| `email_unverified` | Token'da adres yok ya da doğrulandığını söylemiyor; SAML'de NameID ya da e-posta özelliği adres taşımıyor. |
| `domain_not_claimed` | Adres çalışma alanının doğrulanmış alan adlarından birinde değil. |
| `group_not_allowed` | Kişi izin verilen grupların hiçbirinde değil. |
| `not_member` | Kişi çalışma alanının üyesi değil, daveti yok ve anında üyelik kapalı. |
