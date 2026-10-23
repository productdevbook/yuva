# Single Sign-on

Mitglieder Ihres Workspaces melden sich über den Identitätsanbieter Ihres Unternehmens per OpenID
Connect oder SAML 2.0 bei Yuva an: Google Workspace, Microsoft Entra ID, Okta, Keycloak oder jeder
Anbieter, der ein OpenID-Connect-Discovery-Dokument oder SAML-Metadaten veröffentlicht. Ein Workspace
nutzt eines von beiden. Yuva erfährt vom Anbieter, wer jemand ist; welcher Anbieter gilt, entscheidet
die E-Mail-Adresse der Person.

Nur Eigentümer ändern diese Einstellungen, unter Einstellungen › Single Sign-on oder über die API
(`GET /v1/sso` und die Endpunkte unten, mit einer Mitgliedssitzung).

## So funktioniert es

1. Sie registrieren Yuva als Anwendung bei Ihrem Anbieter und geben Yuva Issuer-URL, Client-ID und
   Client-Secret (OpenID Connect) oder die Metadaten des Anbieters (SAML, siehe
   [SAML 2.0 statt OpenID Connect](#saml-20-statt-openid-connect)).
2. Sie beanspruchen Ihre E-Mail-Domains (`example.com`) mit einem DNS-TXT-Eintrag. Eine Domain gehört
   auf einem Yuva-Server genau einem Workspace.
3. Auf der Anmeldeseite gibt die Person ihre E-Mail-Adresse ein. Ist ihre Domain beansprucht, geht der
   Browser zu Ihrem Anbieter und kommt angemeldet zurück.

Yuva fragt `openid email profile` an (plus die Scopes, die Sie ergänzen), nutzt den
Authorization-Code-Flow mit PKCE, `state` und `nonce` und prüft die Signatur des `id_token` gegen die
Schlüssel des Anbieters (wechselt der Anbieter seine Schlüssel, werden die neuen geholt), dazu Issuer,
Audience, Ablauf und Nonce. Das Token muss die E-Mail-Adresse der Person mit `email_verified: true`
tragen, und die Adresse muss in einer Ihrer verifizierten Domains liegen.

Eine Anmeldung über Ihren Anbieter gilt nur in Ihrem Workspace (und auf Diskussionsseiten als Leser).
Wer auch Mitglied anderer Workspaces ist, meldet sich dort mit einem per E-Mail gesendeten Code oder
einem Passkey an; öffnet die Person einen anderen Workspace, fordert das Panel sie dazu auf. Eine
solche Anmeldung kann keine Passkeys hinzufügen oder entfernen, keinen Workspace anlegen, das Konto
nicht löschen, Name, Sprache oder Verfügbarkeit der Person nicht ändern, ihre anderen Sitzungen
nicht beenden und die Benachrichtigungen ihrer anderen Geräte nicht anfassen (`sso_session_scope`),
und sie nimmt nur Einladungen Ihres Workspaces an. Wer Ihren Anbieter ersetzt (anderes Protokoll,
anderer Issuer oder Client-ID, andere SAML-Entität, Anmelde-URL, Zertifikate oder Metadaten-URL) oder
entfernt, meldet alle ab, die sich darüber angemeldet haben, und widerruft die so genehmigten
verbundenen Apps. Das Löschen des Workspaces gibt seine Domains sofort frei. Bis Yuva 0.1.0
galt eine Anmeldung über einen Anbieter für alle Workspaces der Person; das ist nicht mehr so, damit
niemand, der den Identitätsanbieter eines Workspaces betreibt, in den anderen Workspaces einer Person
als diese handeln kann.

Yuva schreibt der Person eine E-Mail, wenn sie sich zum ersten Mal über den Anbieter eines Workspaces
anmeldet und jedes Mal, wenn ihrem Konto ein Passkey hinzugefügt oder daraus entfernt wird.

## 1. Yuva bei Ihrem Anbieter registrieren

Verwenden Sie diese Redirect-URI (Einstellungen › Single Sign-on zeigt sie; in `GET /v1/sso` ist es
`redirect_uri`):

```
https://<Ihre Yuva-Adresse>/auth/sso/callback
```

### Google Workspace

1. In der Google Cloud Console APIs & Services › OAuth consent screen öffnen und **Internal** wählen,
   damit sich nur Konten Ihrer Organisation anmelden.
2. Credentials › Create credentials › OAuth client ID, Typ **Web application**, mit der Redirect-URI
   oben.
3. Issuer: `https://accounts.google.com`. Googles Tokens enthalten keine Gruppen; lassen Sie die
   erlaubten Gruppen leer.

### Microsoft Entra ID

1. Entra Admin Center › App registrations › New registration, "Accounts in this organizational
   directory only", Plattform **Web** mit der Redirect-URI oben.
2. Certificates & secrets › New client secret.
3. Token configuration › Add optional claim › ID: `email` und `xms_edov` hinzufügen. Entra sendet kein
   `email_verified`; Yuva akzeptiert stattdessen `xms_edov: true` (die Domain der Adresse ist in Ihrem
   Tenant verifiziert).
4. Issuer: `https://login.microsoftonline.com/<Tenant-ID>/v2.0`. Die gemeinsamen Issuer `common` und
   `organizations` werden nicht angenommen.
5. Für Gruppen den Claim `groups` unter Token configuration hinzufügen; er nennt Objekt-IDs von
   Gruppen, erlauben Sie also diese IDs.

### Okta

1. Applications › Create App Integration › OIDC, **Web Application**, mit der Redirect-URI oben als
   Sign-in Redirect URI. Weisen Sie die Personen oder Gruppen zu, die sie nutzen dürfen.
2. Issuer: `https://<Ihre Org>.okta.com` (der Org-Autorisierungsserver) oder der Issuer Ihres eigenen
   Autorisierungsservers, etwa `https://<Ihre Org>.okta.com/oauth2/default`.
3. Für Gruppen dem ID-Token einen Claim `groups` hinzufügen (Sign On › OpenID Connect ID Token › Groups
   claim filter) und in Yuva den Scope `groups` ergänzen.

### Keycloak

1. Im Realm Clients › Create client, Typ **OpenID Connect**, Client-Authentifizierung an, mit der
   Redirect-URI oben als gültiger Redirect-URI.
2. Issuer: `https://<Keycloak-Host>/realms/<Realm>`.
3. Für Gruppen im eigenen Scope des Clients einen **Group Membership**-Mapper mit dem Claim-Namen
   `groups` und "Add to ID token" anlegen.

## 2. Den Anbieter verbinden

Geben Sie Issuer, Client-ID und Client-Secret ein (`PUT /v1/sso/oidc`). Yuva liest vor dem Speichern
`<issuer>/.well-known/openid-configuration`; der Issuer muss genau dem `issuer` des Dokuments
entsprechen. Das Secret wird verschlüsselt gespeichert und nie wieder angezeigt; lassen Sie es später
leer, bleibt das gespeicherte.

Optional:

- **Scopes**: zusätzliche Scopes, etwa `groups` für Okta.
- **Gruppen-Claim** (standardmäßig `groups`) und **erlaubte Gruppen**: Nennen Sie Gruppen, melden sich
  nur Personen an, deren `id_token` eine davon enthält.

Der Issuer muss unter einer öffentlichen `https`-Adresse liegen. Ein Server, dessen Betreiber
`YUVA_WEBHOOK_ALLOW_PRIVATE=true` gesetzt hat, erreicht auch Anbieter unter privaten Adressen, etwa ein
Keycloak im selben Netz; Link-local- und Cloud-Metadaten-Adressen werden immer abgelehnt.

## SAML 2.0 statt OpenID Connect

Bietet Ihr Anbieter nur SAML an oder ziehen Sie es vor, gehen Sie so vor statt der Schritte 1 und 2.
Ein gespeicherter SAML-Anbieter ersetzt einen OpenID-Connect-Anbieter und umgekehrt; Domains und
Regeln bleiben.

SAML braucht Yuva unter `https` (`YUVA_PUBLIC_URL`): Der Anbieter schickt seine Antwort von seiner
eigenen Website an Yuva, und Browser senden das Cookie, das diese Antwort mit der Anmeldung verbindet,
nur als Secure-Cookie. Über reines http klappt das nur auf `localhost`; anderswo antwortet das
Speichern eines SAML-Anbieters mit `saml_requires_https`.

Die Anmeldung beginnt immer auf Yuvas Anmeldeseite (vom Service Provider ausgelöst). Eine Antwort,
die der Anbieter schickt, ohne dass Yuva gefragt hat, etwa von einer Kachel im App-Dashboard des
Anbieters, wird abgelehnt: Blenden Sie die App im Dashboard aus oder lassen Sie die Kachel auf Yuvas
Anmeldeseite zeigen.

Tragen Sie beim Anbieter diese Werte ein (Einstellungen › Single Sign-on zeigt sie; `saml_sp` in
`GET /v1/sso`):

| Beim Anbieter | Wert |
|---|---|
| Entity ID, Audience URI, Identifier | `https://<Ihre Yuva-Adresse>/auth/saml/metadata/<Workspace-ID>` |
| ACS URL, Reply URL, Single-Sign-on-URL | `https://<Ihre Yuva-Adresse>/auth/saml/acs` (HTTP-POST) |
| NameID-Format | `urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress`, die E-Mail-Adresse der Person |

Die Entity ID ist zugleich die Adresse von Yuvas Metadaten für Ihren Workspace, für Anbieter, die sie
lesen können. Yuva signiert seine Anfragen nicht und nimmt keine verschlüsselten Assertions an: Lassen
Sie Anfrage-Signatur und Assertion-Verschlüsselung aus.

Yuva prüft jede Antwort: Die Antwort oder die Assertion muss mit einem der Zertifikate aus den
Metadaten Ihres Anbieters signiert sein (unsignierte Antworten, andere Schlüssel und veränderter
Inhalt werden abgelehnt), genau eine Assertion enthalten, eine Anfrage beantworten, die dieser Browser
in den letzten 10 Minuten begann, und nur einmal benutzt werden, Yuvas ACS-URL als Ziel und Empfänger
und Yuvas Entity ID als Audience nennen und innerhalb ihrer Gültigkeit liegen (3 Minuten Uhrabweichung
erlaubt, höchstens 90 Sekunden vorher ausgestellt). Die E-Mail-Adresse kommt aus der NameID oder aus
einem Attribut, das Sie nennen, und muss in einer Ihrer verifizierten Domains liegen.

### Okta

1. Applications › Create App Integration › **SAML 2.0**.
2. Single sign-on URL: die ACS-URL, mit "Use this for Recipient URL and Destination URL" an. Audience
   URI: die Entity ID. Name ID format **EmailAddress**, Application username **Email**.
3. Für Gruppen ein Group Attribute Statement namens `groups` mit einem Filter anlegen (etwa "Matches
   regex" `.*`).
4. Die Personen oder Gruppen zuweisen, die es nutzen dürfen, und auf dem Tab Sign On die
   **Metadata URL** kopieren.

### Microsoft Entra ID

1. Entra Admin Center › Enterprise applications › New application › Create your own application,
   "Integrate any other application you don't find in the gallery".
2. Single sign-on › **SAML**. Basic SAML Configuration: Identifier (Entity ID) die Entity ID, Reply URL
   die ACS-URL.
3. Attributes & Claims: Unique User Identifier (Name ID) auf `user.mail` mit dem Format **Email
   address** setzen.
4. Für Gruppen einen Group Claim hinzufügen; Entra nennt ihn
   `http://schemas.microsoft.com/ws/2008/06/identity/claims/groups` und listet Gruppen-Objekt-IDs auf;
   tragen Sie diesen Namen in Yuva als Gruppen-Attribut ein und erlauben Sie diese IDs.
5. Unter SAML Certificates die **App Federation Metadata Url** kopieren.

### Google Workspace

1. Admin-Konsole › Apps › Web and mobile apps › Add app › **Add custom SAML app**.
2. Auf der Seite Google identity provider details die Metadaten herunterladen. Google hat keine
   Metadaten-URL; fügen Sie die Datei in Yuva ein und fügen Sie die neue ein, wenn Google das
   Zertifikat erneuert.
3. Service provider details: ACS-URL und Entity ID von oben, Name ID format **EMAIL**, Name ID
   **Basic Information › Primary email**.
4. Für Gruppen Group membership einem App-Attribut namens `groups` zuordnen.
5. Die App für die Organisationseinheiten oder Gruppen einschalten, die sie nutzen dürfen.

### Keycloak

1. In Ihrem Realm Clients › Create client, Typ **SAML**, Client ID die Entity ID.
2. Valid redirect URIs: die ACS-URL. Name ID format **email** mit "Force name ID format" an, dazu "Sign
   documents" oder "Sign assertions" an.
3. Keys: "Client signature required" ausschalten; Yuva signiert seine Anfragen nicht.
4. Für Gruppen dem Dedicated Scope des Clients einen **Group list**-Mapper mit dem Attributnamen
   `groups` und "Full group path" aus hinzufügen.
5. Die Metadaten-URL ist `https://<Keycloak-Host>/realms/<Realm>/protocol/saml/descriptor`.

### Den SAML-Anbieter verbinden

Geben Sie Yuva die Metadaten (`PUT /v1/sso/saml`): ihre URL (`metadata_url`, empfohlen) oder das
Dokument selbst (`metadata_xml`). Yuva übernimmt daraus die Entity ID des Anbieters, seine
Single-Sign-on-URL für das HTTP-Redirect-Binding und seine Signaturzertifikate; die Einstellungen
zeigen für jedes Zertifikat Subject, Gültigkeit und SHA-256-Fingerabdruck. Abgelaufene Zertifikate
werden weggelassen und in der Antwort genannt; Metadaten ohne ein jetzt gültiges Zertifikat werden
abgelehnt. Metadaten von einer URL werden neu gelesen, wenn eine Anmeldung beginnt und die Kopie
älter als eine Stunde ist; ein Zertifikat, das Ihr Anbieter wechselt, wird so ohne Ihr Zutun genutzt.
Ist die URL nicht lesbar, nutzt Yuva die gespeicherte Kopie und versucht es 15 Minuten später erneut.
Eingefügte Metadaten ändern sich
nur, wenn Sie sie neu einfügen. Speichern Sie ohne Metadaten erneut, um nur die Optionen unten zu
ändern (und die URL sofort neu zu lesen).

Optional:

- **E-Mail-Attribut**: das Attribut (nach Name oder Friendly Name), das die Adresse enthält, wenn die
  NameID nicht die Adresse ist.
- **Gruppen-Attribut** (standardmäßig `groups`) und **erlaubte Gruppen**: Nennen Sie Gruppen, melden
  sich nur Personen an, deren Assertion eine davon enthält.

Es gelten dieselben Adressregeln wie für den Issuer: öffentliche `https`-Adressen, private nur mit
`YUVA_WEBHOOK_ALLOW_PRIVATE=true`.

## 3. Ihre Domains beanspruchen

Fügen Sie jede E-Mail-Domain hinzu (`POST /v1/sso/domains`) und veröffentlichen Sie den angezeigten
TXT-Eintrag:

```
_yuva-sso.example.com.  TXT  "yuva-verify-…"
```

Dann auf Prüfen klicken (`POST /v1/sso/domains/{id}/verify`). Beansprucht wird nur genau diese Domain,
nicht ihre Subdomains; fügen Sie `eu.example.com` eigens hinzu, wenn Personen solche Adressen haben.
Eine Domain, die ein anderer Workspace auf dem Server verifiziert hat, lässt sich nicht beanspruchen
(`domain_taken`). Lassen Sie den Eintrag bestehen: Yuva fragt ihn jeden Tag erneut ab. Fehlt er 7
Tage lang, fällt die Domain auf ausstehend zurück, die Eigentümer erhalten eine E-Mail, Single Sign-on
ist nicht mehr erzwungen, wenn es Ihre letzte verifizierte Domain war, `sso_domain.lapsed` wird
verzeichnet und ein anderer Workspace kann die Domain beanspruchen. Solange der Eintrag fehlt, zeigt
die Domain, was die Prüfung gefunden hat; veröffentlichen Sie den Eintrag wieder und klicken Sie auf
Prüfen, um das zu beheben.

## 4. Die Anmelderegeln wählen

`PATCH /v1/sso`:

- **Single Sign-on erzwingen** (`sso_required`): Personen Ihrer Domains arbeiten in diesem Workspace
  nur, nachdem sie sich über Ihren Anbieter angemeldet haben. Eine Sitzung per E-Mail-Code oder
  Passkey meldet sie weiterhin an, für ihre anderen Workspaces und als Leser von Diskussionen, aber
  dieser Workspace antwortet mit `sso_required` und das Panel schickt sie zu Ihrem Anbieter.
  Beim Einschalten werden diese Personen aus allen Sitzungen abgemeldet, die nicht über Ihren Anbieter entstanden sind, auch für ihre anderen Workspaces, wo sie sich neu anmelden. Eigentümer sind
  mit jeder Sitzung ausgenommen, damit ein defekter Anbieter Sie nie aussperrt. Verbundene Apps
  (OAuth, MCP) dieser Personen funktionieren hier erst wieder, wenn sie sie nach einer Anmeldung über
  den Anbieter erneut genehmigen; API-Schlüssel sind nicht betroffen. Ihre Antworten in
  Slack-Threads werden abgelehnt, und Aktionen, die sie ohne Anmeldung über den Anbieter in die
  Warteschlange gestellt haben (Antworten an verknüpfte Unterhaltungen, Fortschritt aus Issue-Trackern
  und der Roadmap), werden nicht ausgeführt. Braucht einen Anbieter und eine
  verifizierte Domain; das Entfernen des Anbieters oder der letzten verifizierten Domain schaltet es
  aus.
- **Just-in-time-Mitgliedschaft** (`jit`, standardmäßig aus): Eine Person Ihrer Domains, die sich über
  den Anbieter anmeldet und kein Mitglied ist, tritt mit der Standardrolle bei (`jit_role`: `agent`
  oder `admin`; nie `owner`). Sie sieht, was diese Rolle und die üblichen Posteingangsrechte ergeben.
  Ist es aus, werden Personen wie bisher eingeladen; eine eingeladene Person, die sich über den
  Anbieter anmeldet, nimmt die Einladung an.

Personen Ihrer Domains, die kein Mitglied sind, keine Einladung in Ihren Workspace haben und nicht per
Just-in-time-Mitgliedschaft beitreten, werden abgewiesen (`not_member`), auch wenn sie ein Yuva-Konto
haben.

## Audit-Log

Jede Änderung und jede Anmeldung steht im Audit-Log des Workspaces (Einstellungen › Audit-Log):
`sso.provider_created`, `sso.provider_updated`, `sso.provider_deleted`, `sso.settings_changed`,
`sso_domain.added`, `sso_domain.verified`, `sso_domain.deleted`, `sso_domain.lapsed`, `sso.signed_in` und
`sso.sign_in_refused` mit Grund, dazu `member.joined` mit `via: sso` für Just-in-time-Mitglieder.
Liest Yuva SAML-Metadaten neu und findet neue Zertifikate, verzeichnet das System
`sso.provider_updated` mit `via: metadata_refresh`.

## Wenn die Anmeldung scheitert

Die Anmeldeseite zeigt, was schiefging (`/sign-in?sso_error=…`):

| Code | Bedeutung |
|---|---|
| `expired` | Die Anmeldung dauerte länger als 10 Minuten, wurde schon benutzt oder begann in einem anderen Browser; bei SAML auch eine Antwort, um die Yuva nicht gebeten hat, oder keine SAML-Antwort. |
| `denied` | Der Anbieter lehnte ab oder die Person brach ab. |
| `failed` | Yuva erreichte den Anbieter nicht oder konnte den Code nicht einlösen; prüfen Sie das Client-Secret. |
| `invalid_token` | Das `id_token` bestand eine Prüfung nicht (Signatur, Issuer, Audience, Ablauf, Nonce) oder die SAML-Antwort (Signatur, Zertifikat, Ziel, Empfänger, Audience, Gültigkeit, schon benutzte Assertion); das Audit-Log nennt die Prüfung. |
| `email_unverified` | Das Token enthält keine Adresse oder sagt nicht, dass sie verifiziert ist; bei SAML enthält die NameID oder das E-Mail-Attribut keine Adresse. |
| `domain_not_claimed` | Die Adresse liegt in keiner verifizierten Domain des Workspaces. |
| `group_not_allowed` | Die Person ist in keiner der erlaubten Gruppen. |
| `not_member` | Die Person ist kein Mitglied des Workspaces, hat keine Einladung und die Just-in-time-Mitgliedschaft ist aus. |
