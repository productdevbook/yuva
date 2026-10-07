# Yuva Go SDK

Helpers for a host application's backend. MIT licensed.

```sh
go get github.com/productdevbook/yuva/sdk/go
```

## Identity tokens

Signed-in users of your app chat as themselves: your backend signs a short-lived identity token
with the inbox's identity secret, and the widget or mobile SDK exchanges it for a contact session at
`POST /client/v1/session`. The secret is shown once when the inbox is created or its secret is
rotated; keep it on the server.

```go
import "github.com/productdevbook/yuva/sdk/go/identity"

token, err := identity.Sign(os.Getenv("YUVA_IDENTITY_SECRET"), identity.Claims{
	Subject: user.ID,         // required: your user id
	Email:   user.Email,      // trusted by Yuva
	Name:    user.Name,
	Locale:  "en",
	Attrs:   map[string]any{"plan": user.Plan},
})
// Hand the token to the widget or the mobile SDK.
```

Tokens are valid for 5 minutes by default (`TTL`, at most 10). Sign a new one whenever the page or
app starts a session.
