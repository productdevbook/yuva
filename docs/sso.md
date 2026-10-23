# Single sign-on

Members of your workspace can sign in to Yuva through your company's identity provider with
OpenID Connect: Google Workspace, Microsoft Entra ID, Okta, Keycloak or any provider that publishes
a discovery document. Yuva learns who someone is from the provider; their e-mail address decides
which provider they use.

Only owners change these settings, in Settings › Single sign-on or through the API
(`GET /v1/sso` and the endpoints below, with a member session).

## How it works

1. You register Yuva as an application at your provider and give Yuva the issuer URL, client id and
   client secret.
2. You claim your e-mail domains (`example.com`) with a DNS TXT record. A domain belongs to one
   workspace on a Yuva server.
3. On the sign-in page a person types their e-mail address. When its domain is claimed, the browser
   goes to your provider, and comes back signed in.

Yuva asks for `openid email profile` (plus any scopes you add), uses the authorization code flow
with PKCE, `state` and `nonce`, and checks the `id_token`'s signature against the provider's keys
(new keys are fetched when the provider rotates them), its issuer, audience, expiry and nonce. The
token must carry the person's e-mail address with `email_verified: true`, and the address must be in
one of your verified domains.

Sessions belong to the person, not the workspace: someone who is a member of several workspaces
signs in once, through the provider of their own e-mail domain.

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

## 3. Claim your domains

Add each e-mail domain (`POST /v1/sso/domains`) and publish the TXT record it shows:

```
_yuva-sso.example.com.  TXT  "yuva-verify-…"
```

Then press Verify (`POST /v1/sso/domains/{id}/verify`). Only the exact domain is claimed, not its
subdomains; add `eu.example.com` on its own if people have such addresses. A domain another
workspace on the server has verified cannot be claimed (`domain_taken`). Once verified, a domain
stays claimed until you remove it.

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

People of your domains who already have a Yuva account (for example as a member of another
workspace) sign in through your provider even when they are not members of your workspace.

## Audit log

Every change and every sign-in is in the workspace's audit log (Settings › Audit log):
`sso.provider_created`, `sso.provider_updated`, `sso.provider_deleted`, `sso.settings_changed`,
`sso_domain.added`, `sso_domain.verified`, `sso_domain.deleted`, `sso.signed_in` and
`sso.sign_in_refused` with its reason, and `member.joined` with `via: sso` for just-in-time members.

## When sign-in fails

The sign-in page shows what went wrong (`/sign-in?sso_error=…`):

| Code | Meaning |
|---|---|
| `expired` | The sign-in took longer than 10 minutes, was used already, or started in another browser. |
| `denied` | The provider refused or the person cancelled. |
| `failed` | Yuva could not reach the provider or exchange the code; check the client secret. |
| `invalid_token` | The `id_token` failed a check (signature, issuer, audience, expiry, nonce). |
| `email_unverified` | The token has no address or does not say it is verified. |
| `domain_not_claimed` | The address is not in one of the workspace's verified domains. |
| `group_not_allowed` | The person is in none of the allowed groups. |
| `not_member` | The person has no account here and just-in-time membership is off. |
