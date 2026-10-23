# Single Sign-on

Mitglieder Ihres Workspaces melden sich über den Identitätsanbieter Ihres Unternehmens per OpenID
Connect bei Yuva an: Google Workspace, Microsoft Entra ID, Okta, Keycloak oder jeder Anbieter, der ein
Discovery-Dokument veröffentlicht. Yuva erfährt vom Anbieter, wer jemand ist; welcher Anbieter gilt,
entscheidet die E-Mail-Adresse der Person.

Nur Eigentümer ändern diese Einstellungen, unter Einstellungen › Single Sign-on oder über die API
(`GET /v1/sso` und die Endpunkte unten, mit einer Mitgliedssitzung).

## So funktioniert es

1. Sie registrieren Yuva als Anwendung bei Ihrem Anbieter und geben Yuva Issuer-URL, Client-ID und
   Client-Secret.
2. Sie beanspruchen Ihre E-Mail-Domains (`example.com`) mit einem DNS-TXT-Eintrag. Eine Domain gehört
   auf einem Yuva-Server genau einem Workspace.
3. Auf der Anmeldeseite gibt die Person ihre E-Mail-Adresse ein. Ist ihre Domain beansprucht, geht der
   Browser zu Ihrem Anbieter und kommt angemeldet zurück.

Yuva fragt `openid email profile` an (plus die Scopes, die Sie ergänzen), nutzt den
Authorization-Code-Flow mit PKCE, `state` und `nonce` und prüft die Signatur des `id_token` gegen die
Schlüssel des Anbieters (wechselt der Anbieter seine Schlüssel, werden die neuen geholt), dazu Issuer,
Audience, Ablauf und Nonce. Das Token muss die E-Mail-Adresse der Person mit `email_verified: true`
tragen, und die Adresse muss in einer Ihrer verifizierten Domains liegen.

Sitzungen gehören der Person, nicht dem Workspace: Wer Mitglied mehrerer Workspaces ist, meldet sich
einmal an, über den Anbieter der eigenen E-Mail-Domain.

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

## 3. Ihre Domains beanspruchen

Fügen Sie jede E-Mail-Domain hinzu (`POST /v1/sso/domains`) und veröffentlichen Sie den angezeigten
TXT-Eintrag:

```
_yuva-sso.example.com.  TXT  "yuva-verify-…"
```

Dann auf Prüfen klicken (`POST /v1/sso/domains/{id}/verify`). Beansprucht wird nur genau diese Domain,
nicht ihre Subdomains; fügen Sie `eu.example.com` eigens hinzu, wenn Personen solche Adressen haben.
Eine Domain, die ein anderer Workspace auf dem Server verifiziert hat, lässt sich nicht beanspruchen
(`domain_taken`). Eine verifizierte Domain bleibt beansprucht, bis Sie sie entfernen.

## 4. Die Anmelderegeln wählen

`PATCH /v1/sso`:

- **Single Sign-on erzwingen** (`sso_required`): Personen Ihrer Domains arbeiten in diesem Workspace
  nur, nachdem sie sich über Ihren Anbieter angemeldet haben. Eine Sitzung per E-Mail-Code oder
  Passkey meldet sie weiterhin an, für ihre anderen Workspaces und als Leser von Diskussionen, aber
  dieser Workspace antwortet mit `sso_required` und das Panel schickt sie zu Ihrem Anbieter.
  Sitzungen, die beim Einschalten bestehen, werden ab der nächsten Anfrage abgelehnt. Eigentümer sind
  mit jeder Sitzung ausgenommen, damit ein defekter Anbieter Sie nie aussperrt. Verbundene Apps
  (OAuth, MCP) dieser Personen funktionieren hier erst wieder, wenn sie sie nach einer Anmeldung über
  den Anbieter erneut genehmigen; API-Schlüssel sind nicht betroffen. Braucht einen Anbieter und eine
  verifizierte Domain; das Entfernen des Anbieters oder der letzten verifizierten Domain schaltet es
  aus.
- **Just-in-time-Mitgliedschaft** (`jit`, standardmäßig aus): Eine Person Ihrer Domains, die sich über
  den Anbieter anmeldet und kein Mitglied ist, tritt mit der Standardrolle bei (`jit_role`: `agent`
  oder `admin`; nie `owner`). Sie sieht, was diese Rolle und die üblichen Posteingangsrechte ergeben.
  Ist es aus, werden Personen wie bisher eingeladen; eine eingeladene Person, die sich über den
  Anbieter anmeldet, nimmt die Einladung an.

Personen Ihrer Domains, die schon ein Yuva-Konto haben (etwa als Mitglied eines anderen Workspaces),
melden sich über Ihren Anbieter an, auch wenn sie nicht Mitglied Ihres Workspaces sind.

## Audit-Log

Jede Änderung und jede Anmeldung steht im Audit-Log des Workspaces (Einstellungen › Audit-Log):
`sso.provider_created`, `sso.provider_updated`, `sso.provider_deleted`, `sso.settings_changed`,
`sso_domain.added`, `sso_domain.verified`, `sso_domain.deleted`, `sso.signed_in` und
`sso.sign_in_refused` mit Grund, dazu `member.joined` mit `via: sso` für Just-in-time-Mitglieder.

## Wenn die Anmeldung scheitert

Die Anmeldeseite zeigt, was schiefging (`/sign-in?sso_error=…`):

| Code | Bedeutung |
|---|---|
| `expired` | Die Anmeldung dauerte länger als 10 Minuten, wurde schon benutzt oder begann in einem anderen Browser. |
| `denied` | Der Anbieter lehnte ab oder die Person brach ab. |
| `failed` | Yuva erreichte den Anbieter nicht oder konnte den Code nicht einlösen; prüfen Sie das Client-Secret. |
| `invalid_token` | Das `id_token` bestand eine Prüfung nicht (Signatur, Issuer, Audience, Ablauf, Nonce). |
| `email_unverified` | Das Token enthält keine Adresse oder sagt nicht, dass sie verifiziert ist. |
| `domain_not_claimed` | Die Adresse liegt in keiner verifizierten Domain des Workspaces. |
| `group_not_allowed` | Die Person ist in keiner der erlaubten Gruppen. |
| `not_member` | Die Person hat hier kein Konto und die Just-in-time-Mitgliedschaft ist aus. |
