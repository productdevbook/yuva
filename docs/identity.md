# Identity tokens

An identity token lets a user who is signed in to your app chat as themselves in the
[widget](widget.md) or the [mobile SDKs](mobile.md). Your backend signs it; Yuva never sees your
users or passwords.

## 1. Get the identity secret

Each inbox has an identity secret (`yuva_is_…`). The panel shows it once, when the inbox is created
or the secret is rotated. Keep it on your server only.

## 2. Sign a token for the current user

Add an endpoint that returns a token. In Go:

```go
token, err := identity.Sign(os.Getenv("YUVA_IDENTITY_SECRET"), identity.Claims{
	Subject: user.ID, Email: user.Email, EmailVerified: user.EmailConfirmed, Name: user.Name,
})
```

`identity` is `github.com/productdevbook/yuva/sdk/go/identity`. In other languages, any JWT
library works: sign the [claims](#claims) with HS256, using the whole secret string as the key.

## 3. Pass it to the widget or SDK

Give them a function that fetches the token: `setIdentityToken` in the
[widget](widget.md#3-signed-in-users), `identityToken` in the [mobile SDKs](mobile.md). Call
`signOut()` when the user signs out of your app.

The same `sub` always finds the same contact. Conversations the user started anonymously on that
browser or device move to them.

## Delete a user

When a user deletes their account, delete their contact, conversations and files with an API key:

```sh
curl -X DELETE -H "Authorization: Bearer $YUVA_API_KEY" \
  "https://support.example.com/v1/contacts/by-external-id?inbox_id=<inbox id>&external_id=<your user id>"
```

## Reference

### Claims

| Claim | Required | Meaning |
|---|---|---|
| `sub` | yes | Your user id, 1 to 200 characters. |
| `exp` | yes | Expiry, Unix seconds, at most 10 minutes ahead. The Go helper uses 5 minutes (`Claims.TTL`). |
| `iat`, `nbf` | no | Unix seconds, not in the future. |
| `jti` | no | Token id, 1 to 200 characters; the token then starts one session only. |
| `email` | no | The user's address. |
| `email_verified` | no | `true` if you verified `email`. Only a verified address links to an existing contact. |
| `name` | no | Display name, up to 200 characters. |
| `locale` | no | Language tag, such as `en`. |
| `attrs` | no | JSON object shown to members (plan, app version, …), up to 16 KiB. |

### Rules

| Item | Detail |
|---|---|
| Signature | HS256 only, with the identity secret of the channel's inbox. 30 seconds of clock skew allowed. |
| Failure | `401 invalid_identity_token`, with the reason. |
| Exchange | `POST /client/v1/session` with `channel_key` and `identity_token`; the widget and SDKs do it. |
| Session | Lasts 7 days after last use; `DELETE /client/v1/session` ends it. |
| Rotation | Panel, or `POST /v1/inboxes/{inboxId}/identity-secret` (owners and admins). Old tokens fail at once. An inbox from `yuva inbox create` needs one rotation to get a secret. |
| Deletion | Sends `contact.deleted` to your [webhooks](webhooks.md). |
