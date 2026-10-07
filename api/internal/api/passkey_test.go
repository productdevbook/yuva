package api_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/fxamacker/cbor/v2"
)

var b64 = base64.RawURLEncoding

type softAuthenticator struct {
	t          *testing.T
	key        *ecdsa.PrivateKey
	credID     []byte
	userHandle []byte
	signCount  uint32
}

func newSoftAuthenticator(t *testing.T) *softAuthenticator {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	id := make([]byte, 16)
	_, _ = rand.Read(id)
	return &softAuthenticator{t: t, key: key, credID: id}
}

func (a *softAuthenticator) clientData(typ string, options map[string]any) []byte {
	pk := options["publicKey"].(map[string]any)
	b, _ := json.Marshal(map[string]any{
		"type": typ, "challenge": pk["challenge"], "origin": testOrigin, "crossOrigin": false,
	})
	return b
}

func (a *softAuthenticator) authData(flags byte, attested []byte) []byte {
	rp := sha256.Sum256([]byte("localhost"))
	out := append([]byte{}, rp[:]...)
	out = append(out, flags)
	out = binary.BigEndian.AppendUint32(out, a.signCount)
	return append(out, attested...)
}

func (a *softAuthenticator) create(options map[string]any) map[string]any {
	a.t.Helper()
	user := options["publicKey"].(map[string]any)["user"].(map[string]any)
	handle, err := b64.DecodeString(user["id"].(string))
	if err != nil {
		a.t.Fatal(err)
	}
	a.userHandle = handle
	pub, err := a.key.PublicKey.Bytes()
	if err != nil {
		a.t.Fatal(err)
	}
	cose, err := cbor.Marshal(map[int]any{1: 2, 3: -7, -1: 1, -2: pub[1:33], -3: pub[33:65]})
	if err != nil {
		a.t.Fatal(err)
	}
	attested := make([]byte, 16)
	attested = binary.BigEndian.AppendUint16(attested, uint16(len(a.credID)))
	attested = append(attested, a.credID...)
	attested = append(attested, cose...)
	attObj, err := cbor.Marshal(map[string]any{
		"fmt": "none", "attStmt": map[string]any{}, "authData": a.authData(0x45, attested),
	})
	if err != nil {
		a.t.Fatal(err)
	}
	return map[string]any{
		"id": b64.EncodeToString(a.credID), "rawId": b64.EncodeToString(a.credID), "type": "public-key",
		"authenticatorAttachment": "platform", "clientExtensionResults": map[string]any{},
		"response": map[string]any{
			"clientDataJSON":    b64.EncodeToString(a.clientData("webauthn.create", options)),
			"attestationObject": b64.EncodeToString(attObj),
			"transports":        []string{"internal"},
		},
	}
}

func (a *softAuthenticator) get(options map[string]any) map[string]any {
	a.t.Helper()
	a.signCount++
	cd := a.clientData("webauthn.get", options)
	ad := a.authData(0x05, nil)
	cdHash := sha256.Sum256(cd)
	digest := sha256.Sum256(append(append([]byte{}, ad...), cdHash[:]...))
	sig, err := ecdsa.SignASN1(rand.Reader, a.key, digest[:])
	if err != nil {
		a.t.Fatal(err)
	}
	return map[string]any{
		"id": b64.EncodeToString(a.credID), "rawId": b64.EncodeToString(a.credID), "type": "public-key",
		"authenticatorAttachment": "platform", "clientExtensionResults": map[string]any{},
		"response": map[string]any{
			"clientDataJSON":    b64.EncodeToString(cd),
			"authenticatorData": b64.EncodeToString(ad),
			"signature":         b64.EncodeToString(sig),
			"userHandle":        b64.EncodeToString(a.userHandle),
		},
	}
}

func TestPasskeyRegistrationAndSignIn(t *testing.T) {
	h := newHarness(t)
	email := unique("owner") + "@example.com"
	ws := h.bootstrap(email, unique("ws"))
	owner := h.client()
	owner.signIn(email)
	auth := newSoftAuthenticator(t)

	h.client().expectProblem(http.StatusUnauthorized, "unauthenticated", "POST", "/v1/me/passkeys/options", nil)

	begin := owner.expect(http.StatusOK, "POST", "/v1/me/passkeys/options", nil)
	options := begin.body["options"].(map[string]any)
	sel := options["publicKey"].(map[string]any)["authenticatorSelection"].(map[string]any)
	if sel["residentKey"] != "required" {
		t.Fatalf("registration options: %s", begin.raw)
	}
	credential := auth.create(options)
	other := h.client()
	otherEmail := unique("other") + "@example.com"
	h.bootstrap(otherEmail, unique("ws"))
	other.signIn(otherEmail)
	other.expectProblem(http.StatusBadRequest, "invalid_ceremony", "POST", "/v1/me/passkeys",
		map[string]any{"ceremony_id": begin.str("ceremony_id"), "credential": credential})

	begin = owner.expect(http.StatusOK, "POST", "/v1/me/passkeys/options", nil)
	credential = auth.create(begin.body["options"].(map[string]any))
	created := owner.expect(http.StatusCreated, "POST", "/v1/me/passkeys",
		map[string]any{"ceremony_id": begin.str("ceremony_id"), "name": "Laptop", "credential": credential})
	if created.str("name") != "Laptop" {
		t.Fatalf("passkey: %s", created.raw)
	}
	owner.expectProblem(http.StatusBadRequest, "invalid_ceremony", "POST", "/v1/me/passkeys",
		map[string]any{"ceremony_id": begin.str("ceremony_id"), "credential": credential})

	browser := h.client()
	login := browser.expect(http.StatusOK, "POST", "/v1/auth/passkey/options", nil)
	assertion := auth.get(login.body["options"].(map[string]any))
	signedIn := browser.expect(http.StatusOK, "POST", "/v1/auth/passkey",
		map[string]any{"ceremony_id": login.str("ceremony_id"), "credential": assertion})
	if signedIn.header.Get("Set-Cookie") == "" {
		t.Fatal("no session cookie after passkey sign-in")
	}
	me := browser.expect(http.StatusOK, "GET", "/v1/me", nil)
	if me.body["memberships"].([]any)[0].(map[string]any)["member_id"] != ws.MemberID.String() {
		t.Fatalf("me after passkey sign-in: %s", me.raw)
	}

	login = browser.expect(http.StatusOK, "POST", "/v1/auth/passkey/options", nil)
	forged := auth.get(login.body["options"].(map[string]any))
	forged["response"].(map[string]any)["signature"] = b64.EncodeToString([]byte("not a signature"))
	browser.expectProblem(http.StatusBadRequest, "invalid_passkey", "POST", "/v1/auth/passkey",
		map[string]any{"ceremony_id": login.str("ceremony_id"), "credential": forged})

	list := owner.expect(http.StatusOK, "GET", "/v1/me/passkeys", nil)
	items := list.body["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["last_used_at"] == nil {
		t.Fatalf("passkeys: %s", list.raw)
	}
	other.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/v1/me/passkeys/"+created.str("id"), nil)
	owner.expect(http.StatusNoContent, "DELETE", "/v1/me/passkeys/"+created.str("id"), nil)

	login = browser.expect(http.StatusOK, "POST", "/v1/auth/passkey/options", nil)
	browser.expectProblem(http.StatusBadRequest, "invalid_passkey", "POST", "/v1/auth/passkey",
		map[string]any{"ceremony_id": login.str("ceremony_id"), "credential": auth.get(login.body["options"].(map[string]any))})
}
