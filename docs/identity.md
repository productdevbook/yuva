# Identity tokens

Your users are signed in to your app, not to Yuva. To let them write as themselves, your backend
signs a short-lived identity token for the current user; the [widget](widget.md) or the
[mobile SDK](mobile.md) exchanges it at `POST /client/v1/session` for a contact session. Yuva never
sees your user database or passwords.

```
app ─► your backend: "token for the current user"
your backend ─► app: JWT signed with the inbox identity secret
app ─► Yuva POST /client/v1/session {channel_key, identity_token} ─► contact session
```

## In short

- Your backend signs a short-lived token for the signed-in user with the inbox's
  [identity secret](#the-identity-secret).
- [In Go](#signing-in-go) it is one call; [any JWT library](#signing-in-any-language) works.
- The widget or the SDK exchanges it for a contact session; Yuva never sees your user database.

## The identity secret

Each inbox has one identity secret (`yuva_is_…`). It is shown once when the inbox is created in the
panel or through `POST /v1/inboxes`, and again whenever it is rotated: in the panel, or with
`POST /v1/inboxes/{inboxId}/identity-secret` (owners and admins, not API keys). An inbox created with `yuva inbox create` does not
print it; rotate it once when an app needs one. Keep it on your server only, never in a web page or
an app binary. Rotating it makes tokens signed with the old secret fail at once.

## Signing in Go

```sh
go get github.com/productdevbook/yuva/sdk/go
```

```go
import "github.com/productdevbook/yuva/sdk/go/identity"

func yuvaToken(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	token, err := identity.Sign(os.Getenv("YUVA_IDENTITY_SECRET"), identity.Claims{
		Subject:       user.ID,
		Email:         user.Email,
		EmailVerified: user.EmailConfirmed,
		Name:          user.Name,
		Locale:        "en",
		Attrs:         map[string]any{"plan": user.Plan},
	})
	if err != nil {
		http.Error(w, "token", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"token": token})
}
```

Tokens are valid for 5 minutes by default (`Claims.TTL`, at most 10). Sign a new one each time the
page or app starts a session; the widget and SDKs ask for one when they need it.

## Signing in any language

The token is a JWT (JWS compact form) signed with **HS256**. The key is the identity secret's UTF-8
bytes, the whole string including `yuva_is_`.

Header:

```json
{"alg": "HS256", "typ": "JWT"}
```

Claims:

| Claim | Required | Meaning |
|---|---|---|
| `sub` | yes | Your user id, 1 to 200 characters. The same `sub` always finds the same contact in the inbox. |
| `exp` | yes | Expiry, Unix seconds. At most 10 minutes after now. |
| `iat`, `nbf` | no | Unix seconds; must not be in the future. |
| `jti` | no | A unique token id, 1 to 200 characters. A token with a `jti` starts one session only; without it, a token can be used again until it expires. |
| `email` | no | The user's address. |
| `email_verified` | no | `true` when your app has verified that the user owns `email`; default `false`. Only a verified `email` is used to find an existing contact. |
| `name` | no | Display name, up to 200 characters. |
| `locale` | no | Language tag such as `en` or `tr`. |
| `attrs` | no | A JSON object shown to members next to the conversation (plan, app version, account id, …), up to 16 KiB. |

The token is `base64url(header) "." base64url(claims) "." base64url(HMAC-SHA256(key, base64url(header) "." base64url(claims)))`,
base64url without padding. Any JWT library that signs HS256 produces it. In Node.js with no
dependencies:

```js
import { createHmac } from "node:crypto";

function yuvaToken(secret, user) {
  const now = Math.floor(Date.now() / 1000);
  const b64 = (value) => Buffer.from(JSON.stringify(value)).toString("base64url");
  const body = `${b64({ alg: "HS256", typ: "JWT" })}.${b64({
    sub: user.id, email: user.email, email_verified: user.emailConfirmed, name: user.name,
    iat: now, exp: now + 300,
  })}`;
  return `${body}.${createHmac("sha256", secret).update(body).digest("base64url")}`;
}
```

## What Yuva checks

- `alg` must be `HS256`; `none` and every other algorithm are refused.
- The signature must match the inbox's identity secret (the inbox of the channel key used).
- `exp` must be present, not past and at most 10 minutes ahead; `iat` and `nbf`, when present, not
  in the future. Clocks may differ by 30 seconds.
- `sub` must be a non-empty string.

A failing token answers `401 invalid_identity_token` with the reason.

## What happens to the contact

The contact is found by `sub` among the inbox's external ids, then by `email` when the token says
`email_verified: true` and the contact with that address has no other external id in this inbox,
or created. `name`, `locale` and `attrs` are saved on it (a contact found by e-mail that is known in
another inbox only gets what it is missing). A verified `email` becomes one of the contact's
addresses; an unverified one is stored for replies but never used to link a contact. When the same browser or app was writing as an
anonymous visitor of this inbox before, the visitor's conversations move to the signed-in contact.
The contact session lasts 7 days after its last use; end it when the user signs out of your app
(`signOut()` in the widget and the SDKs, or `DELETE /client/v1/session`).

When a user deletes their account in your app, delete them in Yuva too with an API key:

```sh
curl -X DELETE -H "Authorization: Bearer $YUVA_API_KEY" \
  "https://support.example.com/v1/contacts/by-external-id?inbox_id=<inbox id>&external_id=<your user id>"
```

This removes the contact with all their conversations, messages and files, and sends
`contact.deleted` to your [webhooks](webhooks.md).
