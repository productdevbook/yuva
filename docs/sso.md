# Single sign-on

Members of your workspace can sign in to Yuva through your company's identity provider with
OpenID Connect or SAML 2.0: Google Workspace, Microsoft Entra ID, Okta, Keycloak or any provider that
publishes an OpenID Connect discovery document or SAML metadata. A workspace uses one of the two.
Yuva learns who someone is from the provider; their e-mail address decides which provider they use.

Only owners change these settings, in Settings › Single sign-on or through the API
(`GET /v1/sso` and the endpoints below, with a member session).

## How it works

1. You register Yuva as an application at your provider and give Yuva the issuer URL, client id and
   client secret (OpenID Connect) or the provider's metadata (SAML, see
   [SAML 2.0 instead of OpenID Connect](#saml-20-instead-of-openid-connect)).
2. You claim your e-mail domains (`example.com`) with a DNS TXT record. A domain belongs to one
   workspace on a Yuva server.
3. On the sign-in page a person types their e-mail address. When its domain is claimed, the browser
   goes to your provider, and comes back signed in.

Yuva asks for `openid email profile` (plus any scopes you add), uses the authorization code flow
with PKCE, `state` and `nonce`, and checks the `id_token`'s signature against the provider's keys
(new keys are fetched when the provider rotates them), its issuer, audience, expiry and nonce. The
token must carry the person's e-mail address with `email_verified: true`, and the address must be in
one of your verified domains.

A sign-in through your provider works only in your workspace (and on discussion pages as a reader).
Someone who is also a member of other workspaces signs in there with an e-mailed code or a passkey;
when they open another workspace, the panel asks them to. Such a sign-in cannot add or remove
passkeys, create a workspace or delete the account (`sso_session_scope`), and it accepts only your
workspace's invitations. Until Yuva 0.1.0 a sign-in through a provider counted for every workspace of
the person; it no longer does, so that whoever runs one workspace's identity provider cannot act as
someone in their other workspaces.

Yuva e-mails the person the first time they sign in to a workspace through its provider, and every
time a passkey is added to or removed from their account.

## 1. Register Yuva at your provider

Use this redirect URI (Settings › Single sign-on shows it; it is `redirect_uri` in `GET /v1/sso`):

```
https://<your Yuva address>/auth/sso/callback
```

### Google Workspace

1. In the Google Cloud console, open APIs & Services › OAuth consent screen and choose **Internal**,
   so only your organization's accounts can sign in.
2. Credentials › Create credentials › OAuth client ID, type **Web application**, with the redirect
   URI above.
3. Issuer: `https://accounts.google.com`. Google's tokens carry no groups, so leave the allowed
   groups empty.

### Microsoft Entra ID

1. Entra admin center › App registrations › New registration, "Accounts in this organizational
   directory only", platform **Web** with the redirect URI above.
2. Certificates & secrets › New client secret.
3. Token configuration › Add optional claim › ID: add `email` and `xms_edov`. Entra does not send
   `email_verified`; Yuva accepts `xms_edov: true` (the domain of the address is verified in your
   tenant) in its place.
4. Issuer: `https://login.microsoftonline.com/<tenant id>/v2.0`. The shared `common` and
   `organizations` issuers are not accepted.
5. For groups, add the `groups` claim under Token configuration; it lists group object ids, so
   allow those ids.

### Okta

1. Applications › Create App Integration › OIDC, **Web Application**, with the redirect URI above
   as the sign-in redirect URI. Assign the people or groups who may use it.
2. Issuer: `https://<your org>.okta.com` (the org authorization server) or your custom
   authorization server's issuer, such as `https://<your org>.okta.com/oauth2/default`.
3. For groups, add a `groups` claim to the ID token (Sign On › OpenID Connect ID Token › Groups
   claim filter) and add the `groups` scope in Yuva.

### Keycloak

1. In your realm, Clients › Create client, type **OpenID Connect**, client authentication on, with
   the redirect URI above as a valid redirect URI.
2. Issuer: `https://<keycloak host>/realms/<realm>`.
3. For groups, add a **Group Membership** mapper to the client's dedicated scope with the token
   claim name `groups` and "Add to ID token" on.

## 2. Connect the provider

Enter the issuer, client id and client secret (`PUT /v1/sso/oidc`). Yuva reads
`<issuer>/.well-known/openid-configuration` before saving; the issuer must match the document's
`issuer` exactly. The secret is stored encrypted and never shown again; leave it empty later to keep
it.

Optionally:

- **Scopes**: extra scopes, such as `groups` for Okta.
- **Groups claim** (`groups` by default) and **allowed groups**: when you list groups, only people
  whose `id_token` names one of them sign in.

The issuer must be on a public `https` address. A server whose operator set
`YUVA_WEBHOOK_ALLOW_PRIVATE=true` also reaches providers on private addresses, for example a
Keycloak on the same network; link-local and cloud metadata addresses are always refused.

## SAML 2.0 instead of OpenID Connect

If your provider offers only SAML, or you prefer it, do this instead of steps 1 and 2. Saving a SAML
provider replaces an OpenID Connect one, and the other way round; domains and rules stay.

SAML needs Yuva on `https` (`YUVA_PUBLIC_URL`): the provider posts its answer to Yuva from its own
site, and browsers send the cookie that ties that answer to the sign-in only as a Secure cookie. On
plain http this works only on `localhost`; elsewhere saving a SAML provider answers
`saml_requires_https`.

Sign-in always starts on Yuva's sign-in page (service-provider initiated). A response the provider
sends without Yuva asking, for example from a tile on the provider's app dashboard, is refused: hide
the app from the dashboard or point the tile at Yuva's sign-in page.

Register these values at the provider (Settings › Single sign-on shows them; `saml_sp` in
`GET /v1/sso`):

| At the provider | Value |
|---|---|
| Entity ID, Audience URI, Identifier | `https://<your Yuva address>/auth/saml/metadata/<workspace id>` |
| ACS URL, Reply URL, single sign-on URL | `https://<your Yuva address>/auth/saml/acs` (HTTP-POST) |
| NameID format | `urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress`, the person's e-mail address |

The entity ID is also the address of Yuva's metadata for your workspace, for providers that can read
it. Yuva does not sign its requests and does not take encrypted assertions: leave request signing
and assertion encryption off.

Yuva checks every response: the response or the assertion must be signed by one of the
certificates in your provider's metadata (unsigned responses, other keys and changed content are
refused), it must hold exactly one assertion, answer a request this browser started in the last 10
minutes and be used once, name Yuva's ACS URL as destination and recipient and Yuva's entity ID as
audience, and be within its validity (3 minutes of clock difference allowed, issued at most 90
seconds before). The e-mail address comes from the NameID, or from an attribute you name, and must
be in one of your verified domains.

### Okta

1. Applications › Create App Integration › **SAML 2.0**.
2. Single sign-on URL: the ACS URL, with "Use this for Recipient URL and Destination URL" on.
   Audience URI: the entity ID. Name ID format **EmailAddress**, application username **Email**.
3. For groups, add a group attribute statement named `groups` with a filter (for example "Matches
   regex" `.*`).
4. Assign the people or groups who may use it, and copy the **Metadata URL** from the Sign On tab.

### Microsoft Entra ID

1. Entra admin center › Enterprise applications › New application › Create your own application,
   "Integrate any other application you don't find in the gallery".
2. Single sign-on › **SAML**. Basic SAML Configuration: Identifier (Entity ID) the entity ID, Reply
   URL the ACS URL.
3. Attributes & Claims: set the Unique User Identifier (Name ID) to `user.mail` with the format
   **Email address**.
4. For groups, add a group claim; Entra names it
   `http://schemas.microsoft.com/ws/2008/06/identity/claims/groups` and lists group object ids, so
   use that name as the groups attribute in Yuva and allow those ids.
5. Copy the **App Federation Metadata Url** from SAML Certificates.

### Google Workspace

1. Admin console › Apps › Web and mobile apps › Add app › **Add custom SAML app**.
2. On the Google identity provider details page, download the metadata. Google has no metadata URL,
   so paste the file into Yuva, and paste the new one when Google's certificate is renewed.
3. Service provider details: ACS URL and Entity ID from above, Name ID format **EMAIL**, Name ID
   **Basic Information › Primary email**.
4. For groups, map Group membership to an app attribute named `groups`.
5. Turn the app on for the organizational units or groups who may use it.

### Keycloak

1. In your realm, Clients › Create client, type **SAML**, Client ID the entity ID.
2. Valid redirect URIs: the ACS URL. Name ID format **email**, with "Force name ID format" on, and
   "Sign documents" or "Sign assertions" on.
3. Keys: turn "Client signature required" off; Yuva does not sign its requests.
4. For groups, add a **Group list** mapper to the client's dedicated scope with the attribute name
   `groups` and "Full group path" off.
5. The metadata URL is `https://<keycloak host>/realms/<realm>/protocol/saml/descriptor`.

### Connect the SAML provider

Give Yuva the metadata (`PUT /v1/sso/saml`): its URL (`metadata_url`, preferred) or the document
itself (`metadata_xml`). Yuva takes the provider's entity ID, its single sign-on URL for the
HTTP-Redirect binding and its signing certificates; the settings show each certificate's subject,
validity and SHA-256 fingerprint. Expired certificates are left out and named in the answer;
metadata without a certificate that is valid now is refused. Metadata from a URL is read again when
a sign-in starts and the copy is more than an hour old, so a certificate your provider rotates is
used without you doing anything; when the URL cannot be read, Yuva keeps the stored copy and tries
again 15 minutes later. Pasted metadata changes only when you paste it again. Save again without metadata to
change only the options below (and to read the URL again at once).

Optionally:

- **E-mail attribute**: the attribute (by name or friendly name) holding the address, when the
  NameID is not the address.
- **Groups attribute** (`groups` by default) and **allowed groups**: when you list groups, only people
  whose assertion names one of them sign in.

The same address rules apply as for the issuer: public `https` addresses, private ones only with
`YUVA_WEBHOOK_ALLOW_PRIVATE=true`.

## 3. Claim your domains

Add each e-mail domain (`POST /v1/sso/domains`) and publish the TXT record it shows:

```
_yuva-sso.example.com.  TXT  "yuva-verify-…"
```

Then press Verify (`POST /v1/sso/domains/{id}/verify`). Only the exact domain is claimed, not its
subdomains; add `eu.example.com` on its own if people have such addresses. A domain another
workspace on the server has verified cannot be claimed (`domain_taken`). Keep the record in place: Yuva looks it up again every day. When it has been
missing for 7 days, the domain goes back to pending, the owners get an e-mail, single sign-on stops
being required if it was your last verified domain, `sso_domain.lapsed` is recorded, and another
workspace may claim the domain. While the record is missing the domain shows what the check found;
publish the record again and press Verify to clear it.

## 4. Choose the sign-in rules

`PATCH /v1/sso`:

- **Require single sign-on** (`sso_required`): people of your domains work in this workspace only
  after signing in through your provider. A session made with an e-mailed code or a passkey still
  signs them in, to their other workspaces and as readers of discussions, but this workspace
  answers `sso_required` and the panel sends them to your provider. Sessions that exist when you
  turn it on are refused from the next request. Owners are exempt with any session, so a broken
  provider never locks you out. Connected apps (OAuth, MCP) of these people stop working here until
  they approve them again after signing in through the provider; API keys are not affected. Needs a
  provider and a verified domain; removing the provider or the last verified domain turns it off.
- **Just-in-time membership** (`jit`, off by default): a person of your domains who signs in
  through the provider and is not a member joins with the default role (`jit_role`: `agent`, or
  `admin`; never `owner`). They see what that role and the usual inbox access rules give them.
  With it off, people are invited as before; an invited person who signs in through the provider
  accepts the invite.

People of your domains who are not members, have no invitation to your workspace and are not let
in by just-in-time membership are refused (`not_member`), even when they have a Yuva account.

## Audit log

Every change and every sign-in is in the workspace's audit log (Settings › Audit log):
`sso.provider_created`, `sso.provider_updated`, `sso.provider_deleted`, `sso.settings_changed`,
`sso_domain.added`, `sso_domain.verified`, `sso_domain.deleted`, `sso_domain.lapsed`, `sso.signed_in`
and `sso.sign_in_refused` with its reason, and `member.joined` with `via: sso` for just-in-time members.
When Yuva reads SAML metadata again and finds new certificates, `sso.provider_updated` is recorded
by the system with `via: metadata_refresh`.

## When sign-in fails

The sign-in page shows what went wrong (`/sign-in?sso_error=…`):

| Code | Meaning |
|---|---|
| `expired` | The sign-in took longer than 10 minutes, was used already, or started in another browser; for SAML also a response Yuva did not ask for, or one that is not a SAML response. |
| `denied` | The provider refused or the person cancelled. |
| `failed` | Yuva could not reach the provider or exchange the code; check the client secret. |
| `invalid_token` | The `id_token` failed a check (signature, issuer, audience, expiry, nonce), or the SAML response did (signature, certificate, destination, recipient, audience, validity, an assertion used before); the audit log names the check. |
| `email_unverified` | The token has no address or does not say it is verified; for SAML, the NameID or the e-mail attribute holds no address. |
| `domain_not_claimed` | The address is not in one of the workspace's verified domains. |
| `group_not_allowed` | The person is in none of the allowed groups. |
| `not_member` | The person is not a member of the workspace, has no invitation to it, and just-in-time membership is off. |
