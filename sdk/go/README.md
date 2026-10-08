# Yuva Go SDK

Helpers for a host application's backend. MIT licensed.

```sh
go get github.com/productdevbook/yuva/sdk/go
```

## API client

`client` is a typed client for `/v1`, generated from the OpenAPI contract with
[oapi-codegen](https://github.com/oapi-codegen/oapi-codegen). Authenticate with an API key (or an
OAuth access token) as a bearer token:

```go
import "github.com/productdevbook/yuva/sdk/go/client"

c, err := client.NewClientWithResponses("https://yuva.example.com",
	client.WithRequestEditorFn(func(ctx context.Context, req *http.Request) error {
		req.Header.Set("Authorization", "Bearer "+os.Getenv("YUVA_API_KEY"))
		return nil
	}))
if err != nil {
	return err
}
res, err := c.ListInboxesWithResponse(ctx, nil)
if err != nil {
	return err
}
if res.JSON200 == nil {
	return fmt.Errorf("yuva: %s: %s", res.Status(), res.Body)
}
for _, inbox := range res.JSON200.Items {
	fmt.Println(inbox.Id, inbox.Name)
}
```

Every operation has a `…WithResponse` method whose result holds the decoded body for each documented
status (`JSON200`, `ApplicationproblemJSON403`, …) next to the raw `Body` and `HTTPResponse`.
`go generate ./...` in `sdk/go` regenerates it after the contract changes.

## Identity tokens

Signed-in users of your app chat as themselves: your backend signs a short-lived identity token
with the inbox's identity secret, and the widget or mobile SDK exchanges it for a contact session at
`POST /client/v1/session`. The secret is shown once when the inbox is created or its secret is
rotated; keep it on the server.

```go
import "github.com/productdevbook/yuva/sdk/go/identity"

token, err := identity.Sign(os.Getenv("YUVA_IDENTITY_SECRET"), identity.Claims{
	Subject:       user.ID, // required: your user id
	Email:         user.Email,
	EmailVerified: user.EmailConfirmed, // only then does Yuva match the address to a contact
	Name:          user.Name,
	Locale:        "en",
	Attrs:         map[string]any{"plan": user.Plan},
})
// Hand the token to the widget or the mobile SDK.
```

Tokens are valid for 5 minutes by default (`TTL`, at most 10). Sign a new one whenever the page or
app starts a session.

## Webhooks

Yuva signs webhooks per [Standard Webhooks](https://www.standardwebhooks.com). Verify the raw body
with the endpoint's secret (shown once as `whsec_…` when the endpoint is created or its secret is
rotated) before you parse it:

```go
import "github.com/productdevbook/yuva/sdk/go/webhook"

func handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err := webhook.Verify(os.Getenv("YUVA_WEBHOOK_SECRET"), r.Header, body, 5*time.Minute); err != nil {
		http.Error(w, "bad signature", http.StatusUnauthorized)
		return
	}
	// Ignore a webhook-id you have already handled; Yuva retries until it gets a 2xx.
	var event struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	_ = json.Unmarshal(body, &event)
	// message.created with data.contact.online == false: send your push notification.
	w.WriteHeader(http.StatusNoContent)
}
```

During a secret rotation Yuva signs with both the new and the old secret for 24 hours, so either
secret verifies.
