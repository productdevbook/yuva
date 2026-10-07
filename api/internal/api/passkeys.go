package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"uuid"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	ceremonyTTL          = 5 * time.Minute
	ceremonyRegistration = "registration"
	ceremonyLogin        = "login"
)

var (
	errCeremony      = problem(http.StatusBadRequest, "invalid_ceremony", "the passkey ceremony is unknown or expired; start again")
	errPasskeyExists = problem(http.StatusConflict, "passkey_exists", "this passkey is already registered")
)

func errPasskey(err error) *apiError {
	detail := "the passkey response was not accepted"
	var pe *protocol.Error
	if errors.As(err, &pe) && pe.Details != "" {
		detail += ": " + pe.Details
	}
	return problem(http.StatusBadRequest, "invalid_passkey", detail)
}

type passkeyUser struct {
	person store.Person
	creds  []webauthn.Credential
	rows   []store.Passkey
}

func (u *passkeyUser) WebAuthnID() []byte                         { return u.person.WebauthnHandle }
func (u *passkeyUser) WebAuthnName() string                       { return u.person.Email }
func (u *passkeyUser) WebAuthnCredentials() []webauthn.Credential { return u.creds }
func (u *passkeyUser) WebAuthnDisplayName() string {
	if u.person.Name != "" {
		return u.person.Name
	}
	return u.person.Email
}

func (s *Server) loadPasskeyUser(ctx context.Context, person store.Person) (*passkeyUser, error) {
	rows, err := s.st.ListPasskeys(ctx, store.ListPasskeysParams{PersonID: person.ID, RpID: s.webauthn.Config.RPID})
	if err != nil {
		return nil, err
	}
	u := &passkeyUser{person: person, rows: rows}
	for _, r := range rows {
		var c webauthn.Credential
		if err := json.Unmarshal(r.Credential, &c); err != nil {
			return nil, err
		}
		u.creds = append(u.creds, c)
	}
	return u, nil
}

func (s *Server) startCeremony(ctx context.Context, kind string, personID *uuid.UUID, options any, session *webauthn.SessionData) (oas.PasskeyCeremony, error) {
	now := s.now()
	if err := s.st.DeleteExpiredWebauthnCeremonies(ctx, now); err != nil {
		return oas.PasskeyCeremony{}, err
	}
	data, err := json.Marshal(session)
	if err != nil {
		return oas.PasskeyCeremony{}, err
	}
	id := randomToken(32)
	if err := s.st.CreateWebauthnCeremony(ctx, store.CreateWebauthnCeremonyParams{
		IDHash: hashSecret(id), Kind: kind, PersonID: personID, SessionData: data, ExpiresAt: now.Add(ceremonyTTL),
	}); err != nil {
		return oas.PasskeyCeremony{}, err
	}
	raw, err := json.Marshal(options)
	if err != nil {
		return oas.PasskeyCeremony{}, err
	}
	var opts map[string]any
	if err := json.Unmarshal(raw, &opts); err != nil {
		return oas.PasskeyCeremony{}, err
	}
	return oas.PasskeyCeremony{CeremonyId: id, Options: opts}, nil
}

func (s *Server) takeCeremony(ctx context.Context, id, kind string) (store.WebauthnCeremony, webauthn.SessionData, error) {
	var sd webauthn.SessionData
	c, err := s.st.TakeWebauthnCeremony(ctx, store.TakeWebauthnCeremonyParams{IDHash: hashSecret(id), Kind: kind})
	if store.IsNotFound(err) {
		return c, sd, errCeremony
	}
	if err != nil {
		return c, sd, err
	}
	if !c.ExpiresAt.After(s.now()) {
		return c, sd, errCeremony
	}
	if err := json.Unmarshal(c.SessionData, &sd); err != nil {
		return c, sd, err
	}
	return c, sd, nil
}

func (s *Server) BeginPasskeyRegistration(ctx context.Context, _ oas.BeginPasskeyRegistrationRequestObject) (oas.BeginPasskeyRegistrationResponseObject, error) {
	personID := principalFrom(ctx).personID
	person, err := s.st.GetPerson(ctx, personID)
	if err != nil {
		return nil, err
	}
	user, err := s.loadPasskeyUser(ctx, person)
	if err != nil {
		return nil, err
	}
	creation, session, err := s.webauthn.BeginRegistration(user,
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired),
		webauthn.WithExclusions(webauthn.Credentials(user.creds).CredentialDescriptors()),
	)
	if err != nil {
		return nil, err
	}
	out, err := s.startCeremony(ctx, ceremonyRegistration, &personID, creation, session)
	if err != nil {
		return nil, err
	}
	return oas.BeginPasskeyRegistration200JSONResponse(out), nil
}

func (s *Server) FinishPasskeyRegistration(ctx context.Context, req oas.FinishPasskeyRegistrationRequestObject) (oas.FinishPasskeyRegistrationResponseObject, error) {
	personID := principalFrom(ctx).personID
	name := "Passkey"
	if req.Body.Name != nil {
		if n := strings.TrimSpace(*req.Body.Name); n != "" {
			name = n
		}
	}
	if len([]rune(name)) > 200 {
		return nil, errValidation("name is longer than 200 characters")
	}
	c, session, err := s.takeCeremony(ctx, req.Body.CeremonyId, ceremonyRegistration)
	if err != nil {
		return nil, err
	}
	if c.PersonID == nil || *c.PersonID != personID {
		return nil, errCeremony
	}
	raw, err := json.Marshal(req.Body.Credential)
	if err != nil {
		return nil, err
	}
	parsed, err := protocol.ParseCredentialCreationResponseBytes(raw)
	if err != nil {
		return nil, errPasskey(err)
	}
	person, err := s.st.GetPerson(ctx, personID)
	if err != nil {
		return nil, err
	}
	user, err := s.loadPasskeyUser(ctx, person)
	if err != nil {
		return nil, err
	}
	cred, err := s.webauthn.CreateCredential(user, session, parsed)
	if err != nil {
		return nil, errPasskey(err)
	}
	data, err := json.Marshal(cred)
	if err != nil {
		return nil, err
	}
	pk, err := s.st.CreatePasskey(ctx, store.CreatePasskeyParams{
		ID: uuid.New(), PersonID: personID, RpID: s.webauthn.Config.RPID, CredentialID: cred.ID, Name: name, Credential: data,
	})
	if store.IsUniqueViolation(err) {
		return nil, errPasskeyExists
	}
	if err != nil {
		return nil, err
	}
	return oas.FinishPasskeyRegistration201JSONResponse(passkeyBody(pk)), nil
}

func passkeyBody(p store.Passkey) oas.Passkey {
	return oas.Passkey{Id: p.ID, Name: p.Name, CreatedAt: p.CreatedAt, LastUsedAt: p.LastUsedAt}
}

func (s *Server) ListPasskeys(ctx context.Context, _ oas.ListPasskeysRequestObject) (oas.ListPasskeysResponseObject, error) {
	rows, err := s.st.ListPasskeys(ctx, store.ListPasskeysParams{PersonID: principalFrom(ctx).personID, RpID: s.webauthn.Config.RPID})
	if err != nil {
		return nil, err
	}
	out := oas.ListPasskeys200JSONResponse{Items: make([]oas.Passkey, 0, len(rows))}
	for _, r := range rows {
		out.Items = append(out.Items, passkeyBody(r))
	}
	return out, nil
}

func (s *Server) DeletePasskey(ctx context.Context, req oas.DeletePasskeyRequestObject) (oas.DeletePasskeyResponseObject, error) {
	n, err := s.st.DeletePasskey(ctx, store.DeletePasskeyParams{PersonID: principalFrom(ctx).personID, ID: req.PasskeyId})
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, errNotFound
	}
	return oas.DeletePasskey204Response{}, nil
}

func (s *Server) BeginPasskeySignIn(ctx context.Context, _ oas.BeginPasskeySignInRequestObject) (oas.BeginPasskeySignInResponseObject, error) {
	assertion, session, err := s.webauthn.BeginDiscoverableLogin()
	if err != nil {
		return nil, err
	}
	out, err := s.startCeremony(ctx, ceremonyLogin, nil, assertion, session)
	if err != nil {
		return nil, err
	}
	return oas.BeginPasskeySignIn200JSONResponse(out), nil
}

func (s *Server) FinishPasskeySignIn(ctx context.Context, req oas.FinishPasskeySignInRequestObject) (oas.FinishPasskeySignInResponseObject, error) {
	_, session, err := s.takeCeremony(ctx, req.Body.CeremonyId, ceremonyLogin)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(req.Body.Credential)
	if err != nil {
		return nil, err
	}
	parsed, err := protocol.ParseCredentialRequestResponseBytes(raw)
	if err != nil {
		return nil, errPasskey(err)
	}
	var lookupErr error
	handler := func(_, userHandle []byte) (webauthn.User, error) {
		person, err := s.st.GetPersonByWebauthnHandle(ctx, userHandle)
		if err != nil {
			if !store.IsNotFound(err) {
				lookupErr = err
			}
			return nil, err
		}
		return s.loadPasskeyUser(ctx, person)
	}
	validated, cred, err := s.webauthn.ValidatePasskeyLogin(handler, session, parsed)
	if lookupErr != nil {
		return nil, lookupErr
	}
	if err != nil {
		return nil, errPasskey(err)
	}
	user := validated.(*passkeyUser)
	var row *store.Passkey
	for i := range user.rows {
		if bytes.Equal(user.rows[i].CredentialID, cred.ID) {
			row = &user.rows[i]
		}
	}
	if row == nil {
		return nil, errPasskey(errors.New("unknown credential"))
	}
	data, err := json.Marshal(cred)
	if err != nil {
		return nil, err
	}
	now := s.now()
	var (
		cookie string
		me     oas.Me
	)
	err = s.st.InTx(ctx, func(q *store.Queries) error {
		if err := q.UpdatePasskeyUse(ctx, store.UpdatePasskeyUseParams{PersonID: user.person.ID, ID: row.ID, Credential: data, LastUsedAt: &now}); err != nil {
			return err
		}
		if me, err = buildMe(ctx, q, user.person.ID); err != nil {
			return err
		}
		cookie, err = s.startSession(ctx, q, user.person.ID, methodPasskey)
		return err
	})
	if err != nil {
		return nil, err
	}
	return oas.FinishPasskeySignIn200JSONResponse{SignedInJSONResponse: oas.SignedInJSONResponse{
		Body: me, Headers: oas.SignedInResponseHeaders{SetCookie: &cookie},
	}}, nil
}
