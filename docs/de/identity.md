# Identitäts-Tokens

Mit einem Identitäts-Token chattet ein Nutzer, der in Ihrer App angemeldet ist, unter eigenem Namen
im [Widget](widget.md) oder in den [Mobile-SDKs](mobile.md). Ihr Backend signiert es; Yuva sieht nie
Ihre Nutzer oder Passwörter.

## 1. Das Identitäts-Secret holen

Jeder Posteingang hat ein Identitäts-Secret (`yuva_is_…`). Das Panel zeigt es einmal, wenn der
Posteingang angelegt oder das Secret rotiert wird. Bewahren Sie es nur auf Ihrem Server auf.

## 2. Ein Token für den aktuellen Nutzer signieren

Legen Sie einen Endpunkt an, der ein Token zurückgibt. In Go:

```go
token, err := identity.Sign(os.Getenv("YUVA_IDENTITY_SECRET"), identity.Claims{
	Subject: user.ID, Email: user.Email, EmailVerified: user.EmailConfirmed, Name: user.Name,
})
```

`identity` ist `github.com/productdevbook/yuva/sdk/go/identity`. In anderen Sprachen funktioniert
jede JWT-Bibliothek: Signieren Sie die [Claims](#claims) mit HS256 und dem ganzen Secret-String als
Schlüssel.

## 3. An Widget oder SDK übergeben

Geben Sie ihnen eine Funktion, die das Token abruft: `setIdentityToken` im
[Widget](widget.md#3-angemeldete-nutzer), `identityToken` in den [Mobile-SDKs](mobile.md). Rufen
Sie `signOut()` auf, wenn sich der Nutzer von Ihrer App abmeldet.

Dasselbe `sub` findet immer denselben Kontakt. Unterhaltungen, die der Nutzer in diesem Browser oder
auf diesem Gerät anonym begonnen hat, gehen auf ihn über.

## Einen Nutzer löschen

Löscht ein Nutzer sein Konto, löschen Sie seinen Kontakt, seine Unterhaltungen und Dateien mit einem
API-Schlüssel:

```sh
curl -X DELETE -H "Authorization: Bearer $YUVA_API_KEY" \
  "https://support.example.com/v1/contacts/by-external-id?inbox_id=<inbox id>&external_id=<your user id>"
```

## Referenz

### Claims

| Claim | Pflicht | Bedeutung |
|---|---|---|
| `sub` | ja | Ihre Nutzer-ID, 1 bis 200 Zeichen. |
| `exp` | ja | Ablauf in Unix-Sekunden, höchstens 10 Minuten in der Zukunft. Der Go-Helfer nimmt 5 Minuten (`Claims.TTL`). |
| `iat`, `nbf` | nein | Unix-Sekunden, nicht in der Zukunft. |
| `jti` | nein | Token-ID, 1 bis 200 Zeichen; das Token startet dann nur eine Sitzung. |
| `email` | nein | Die Adresse des Nutzers. |
| `email_verified` | nein | `true`, wenn Sie `email` verifiziert haben. Nur eine verifizierte Adresse wird mit einem bestehenden Kontakt verknüpft. |
| `name` | nein | Anzeigename, bis zu 200 Zeichen. |
| `locale` | nein | Sprach-Tag, etwa `en`. |
| `attrs` | nein | JSON-Objekt, das Mitgliedern angezeigt wird (Tarif, App-Version, …), bis zu 16 KiB. |

### Regeln

| Punkt | Detail |
|---|---|
| Signatur | Nur HS256, mit dem Identitäts-Secret des Posteingangs, zu dem der Kanal gehört. 30 Sekunden Uhrabweichung sind erlaubt. |
| Fehler | `401 invalid_identity_token`, mit dem Grund. |
| Austausch | `POST /client/v1/session` mit `channel_key` und `identity_token`; Widget und SDKs erledigen das. |
| Sitzung | Gilt 7 Tage nach der letzten Nutzung; `DELETE /client/v1/session` beendet sie. |
| Rotation | Im Panel oder mit `POST /v1/inboxes/{inboxId}/identity-secret` (Inhaber und Admins). Alte Tokens schlagen sofort fehl. Ein Posteingang aus `yuva inbox create` braucht eine Rotation, um ein Secret zu bekommen. |
| Löschen | Sendet `contact.deleted` an Ihre [Webhooks](webhooks.md). |
