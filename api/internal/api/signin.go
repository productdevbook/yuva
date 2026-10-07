package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"math/big"
	"net/http"
	"net/mail"
	"strings"
	"time"
	"uuid"

	yuvamail "github.com/productdevbook/yuva/api/internal/mail"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	codeTTL           = 10 * time.Minute
	codeAttempts      = 5
	codeRateWindow    = 15 * time.Minute
	codesPerEmail     = 5
	codesPerIP        = 30
	webauthnHandleLen = 32
)

var (
	errRateLimited     = problem(http.StatusTooManyRequests, "rate_limited", "too many sign-in codes requested; try again later")
	errInvalidCode     = problem(http.StatusBadRequest, "invalid_code", "the code is not valid")
	errCodeExpired     = problem(http.StatusBadRequest, "code_expired", "the code has expired; request a new one")
	errTooManyAttempts = problem(http.StatusBadRequest, "code_attempts_exceeded", "too many wrong attempts; request a new code")
)

func normalizeEmail(e oas.Email) (string, error) {
	a, err := mail.ParseAddress(string(e))
	if err != nil || len(a.Address) > 320 {
		return "", errValidation("email is not a valid address")
	}
	return strings.ToLower(a.Address), nil
}

func hashCode(id uuid.UUID, code string) []byte {
	h := sha256.New()
	h.Write(id[:])
	h.Write([]byte(code))
	return h.Sum(nil)
}

func newCode() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("%06d", n.Int64())
}

func (s *Server) RequestSignInCode(ctx context.Context, req oas.RequestSignInCodeRequestObject) (oas.RequestSignInCodeResponseObject, error) {
	email, err := normalizeEmail(req.Body.Email)
	if err != nil {
		return nil, err
	}
	now := s.now()
	ip := s.clientIP(requestFrom(ctx))
	since := now.Add(-codeRateWindow)
	byEmail, err := s.st.CountLoginCodesByEmail(ctx, store.CountLoginCodesByEmailParams{Email: email, CreatedAt: since})
	if err != nil {
		return nil, err
	}
	byIP, err := s.st.CountLoginCodesByIP(ctx, store.CountLoginCodesByIPParams{Ip: ip, CreatedAt: since})
	if err != nil {
		return nil, err
	}
	if byEmail >= codesPerEmail || byIP >= codesPerIP {
		return nil, errRateLimited
	}
	known, err := s.st.SignInTarget(ctx, store.SignInTargetParams{Email: email, Now: now})
	if err != nil {
		return nil, err
	}
	code := newCode()
	id := uuid.New()
	if err := s.st.CreateLoginCode(ctx, store.CreateLoginCodeParams{
		ID: id, Email: email, CodeHash: hashCode(id, code), Ip: ip, CreatedAt: now, ExpiresAt: now.Add(codeTTL),
	}); err != nil {
		return nil, err
	}
	if known == nil || !*known {
		return oas.RequestSignInCode202Response{}, nil
	}
	locale, err := s.localeFor(ctx, email, now)
	if err != nil {
		return nil, err
	}
	msg, err := yuvamail.Render("sign_in_code", locale, map[string]any{"Code": code, "Minutes": int(codeTTL / time.Minute)})
	if err != nil {
		return nil, err
	}
	msg.To = email
	if err := s.mailer.Send(ctx, msg); err != nil {
		return nil, err
	}
	return oas.RequestSignInCode202Response{}, nil
}

func (s *Server) localeFor(ctx context.Context, email string, now time.Time) (string, error) {
	p, err := s.st.GetPersonByEmail(ctx, email)
	if err == nil {
		return p.Locale, nil
	}
	if !store.IsNotFound(err) {
		return "", err
	}
	inv, err := s.st.LatestPendingInvite(ctx, store.LatestPendingInviteParams{Email: email, Now: now})
	if store.IsNotFound(err) {
		return "en", nil
	}
	if err != nil {
		return "", err
	}
	return inv.Locale, nil
}

func (s *Server) VerifySignInCode(ctx context.Context, req oas.VerifySignInCodeRequestObject) (oas.VerifySignInCodeResponseObject, error) {
	email, err := normalizeEmail(req.Body.Email)
	if err != nil {
		return nil, err
	}
	now := s.now()
	var (
		fail   *apiError
		cookie string
		me     oas.Me
	)
	err = s.st.InTx(ctx, func(q *store.Queries) error {
		lc, err := q.LatestLoginCode(ctx, email)
		if store.IsNotFound(err) {
			fail = errInvalidCode
			return nil
		}
		if err != nil {
			return err
		}
		if !lc.ExpiresAt.After(now) {
			fail = errCodeExpired
			return nil
		}
		if lc.Attempts >= codeAttempts {
			fail = errTooManyAttempts
			return nil
		}
		attempts, err := q.AddLoginCodeAttempt(ctx, lc.ID)
		if err != nil {
			return err
		}
		if subtle.ConstantTimeCompare(hashCode(lc.ID, req.Body.Code), lc.CodeHash) != 1 {
			fail = errInvalidCode
			if attempts >= codeAttempts {
				fail = errTooManyAttempts
			}
			return nil
		}
		if err := q.ConsumeLoginCode(ctx, store.ConsumeLoginCodeParams{ID: lc.ID, ConsumedAt: &now}); err != nil {
			return err
		}
		personID, ok, err := acceptInvites(ctx, q, email, now)
		if err != nil {
			return err
		}
		if !ok {
			fail = errInvalidCode
			return nil
		}
		if me, err = buildMe(ctx, q, personID); err != nil {
			return err
		}
		if len(me.Memberships) == 0 {
			fail = errInvalidCode
			return nil
		}
		cookie, err = s.startSession(ctx, q, personID, methodCode)
		return err
	})
	if err != nil {
		return nil, err
	}
	if fail != nil {
		return nil, fail
	}
	return oas.VerifySignInCode200JSONResponse{SignedInJSONResponse: oas.SignedInJSONResponse{
		Body: me, Headers: oas.SignedInResponseHeaders{SetCookie: &cookie},
	}}, nil
}

func acceptInvites(ctx context.Context, q *store.Queries, email string, now time.Time) (uuid.UUID, bool, error) {
	person, err := q.GetPersonByEmail(ctx, email)
	found := err == nil
	if err != nil && !store.IsNotFound(err) {
		return uuid.Nil(), false, err
	}
	invites, err := q.TakePendingInvites(ctx, store.TakePendingInvitesParams{Email: email, Now: now})
	if err != nil {
		return uuid.Nil(), false, err
	}
	if !found {
		if len(invites) == 0 {
			return uuid.Nil(), false, nil
		}
		if person, err = createPerson(ctx, q, email, "", invites[0].Locale); err != nil {
			return uuid.Nil(), false, err
		}
	}
	for _, inv := range invites {
		_, err := q.CreateMember(ctx, store.CreateMemberParams{
			ID: uuid.New(), WorkspaceID: inv.WorkspaceID, PersonID: person.ID, Role: inv.Role,
		})
		if err != nil && !store.IsNotFound(err) {
			return uuid.Nil(), false, err
		}
	}
	return person.ID, true, nil
}

func createPerson(ctx context.Context, q *store.Queries, email, name, locale string) (store.Person, error) {
	handle := make([]byte, webauthnHandleLen)
	_, _ = rand.Read(handle)
	return q.CreatePerson(ctx, store.CreatePersonParams{
		ID: uuid.New(), Email: email, Name: name, Locale: locale, WebauthnHandle: handle,
	})
}

func (s *Server) SignOut(ctx context.Context, _ oas.SignOutRequestObject) (oas.SignOutResponseObject, error) {
	if err := s.st.DeleteSession(ctx, principalFrom(ctx).sessionID); err != nil {
		return nil, err
	}
	cookie := s.sessionCookie("", -1)
	return oas.SignOut204Response{Headers: oas.SignOut204ResponseHeaders{SetCookie: &cookie}}, nil
}
