package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
	"uuid"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	yuvamail "github.com/productdevbook/yuva/api/internal/mail"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
	"github.com/productdevbook/yuva/api/internal/webhook"
)

const (
	ssoCookie        = "yuva_sso"
	ssoCookiePath    = "/auth/sso"
	ssoCallbackPath  = "/auth/sso/callback"
	ssoLoginTTL      = 10 * time.Minute
	ssoDiscoveryTTL  = time.Hour
	ssoHTTPTimeout   = 10 * time.Second
	ssoMaxBody       = 1 << 20
	ssoTXT           = "_yuva-sso."
	ssoDomainLimit   = 50
	ssoDefaultGroups = "groups"
	protocolOIDC     = string(oas.SsoProtocolOidc)
	methodSSO        = "sso"

	ssoFailExpired      = "expired"
	ssoFailDenied       = "denied"
	ssoFailFailed       = "failed"
	ssoFailInvalidToken = "invalid_token"
	ssoFailUnverified   = "email_unverified"
	ssoFailUnclaimed    = "domain_not_claimed"
	ssoFailGroup        = "group_not_allowed"
	ssoFailNotMember    = "not_member"
)

var (
	errSSORequired     = problem(http.StatusForbidden, "sso_required", "this workspace requires signing in through its identity provider; send sso_email to POST /v1/auth/sso")
	errSSOSessionScope = problem(http.StatusForbidden, "sso_session_scope", "this session was made through the single sign-on of the workspace sso_workspace_id and acts only there; sign in with a code or a passkey for this")
	errSSONotReady     = problem(http.StatusConflict, "sso_not_ready", "single sign-on can be required only with an identity provider and a verified domain")
	errSSOUnavailable  = problem(http.StatusBadGateway, "sso_unavailable", "the identity provider could not be reached; try again later")
	errSSODomainExists = problem(http.StatusConflict, "domain_exists", "this workspace already claims this domain")
	errSSODomainTaken  = problem(http.StatusConflict, "domain_taken", "another workspace on this server has claimed this domain")
	errSSODomainLimit  = problem(http.StatusConflict, "sso_domain_limit", "a workspace claims at most 50 domains")

	limitSSOStartPerIP = limit{30, time.Minute}

	scopeTokenPattern = regexp.MustCompile(`^[\x21\x23-\x5B\x5D-\x7E]{1,100}$`)
)

func errSSODiscovery(detail string) *apiError {
	return problem(http.StatusBadRequest, "sso_discovery_failed", "the provider's discovery document could not be used: "+truncateRunes(detail, 300))
}

type ssoState struct {
	http *http.Client
	mu   sync.Mutex
	prov map[string]ssoProvider
}

type ssoProvider struct {
	p  *oidc.Provider
	at time.Time
}

type limitedBodies struct{ next http.RoundTripper }

func (l limitedBodies) RoundTrip(r *http.Request) (*http.Response, error) {
	res, err := l.next.RoundTrip(r)
	if err == nil {
		res.Body = struct {
			io.Reader
			io.Closer
		}{io.LimitReader(res.Body, ssoMaxBody), res.Body}
	}
	return res, err
}

// newSSOState reaches identity providers through the webhook address checks: no link-local or
// metadata address ever, no private one unless the operator allows them.
func newSSOState(w WebhookSettings) *ssoState {
	dial := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return webhook.Dial(ctx, w.Resolver, w.AllowPrivate, ssoHTTPTimeout, network, addr)
	}
	return &ssoState{prov: map[string]ssoProvider{}, http: &http.Client{
		Timeout: ssoHTTPTimeout,
		Transport: limitedBodies{&http.Transport{
			Proxy: nil, DialContext: dial, TLSHandshakeTimeout: ssoHTTPTimeout, ResponseHeaderTimeout: ssoHTTPTimeout,
			MaxResponseHeaderBytes: 64 << 10, IdleConnTimeout: time.Minute,
		}},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

// provider returns the issuer's discovered configuration, read again after an hour or when fresh;
// its key set is kept with it and fetched again when a token names a key it does not hold.
func (s *Server) oidcProvider(ctx context.Context, issuer string, fresh bool) (*oidc.Provider, error) {
	now := s.now()
	s.sso.mu.Lock()
	c, ok := s.sso.prov[issuer]
	s.sso.mu.Unlock()
	if ok && !fresh && now.Sub(c.at) < ssoDiscoveryTTL {
		return c.p, nil
	}
	if _, err := webhook.CheckURL(issuer, s.webhooks.AllowPrivate); err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, ssoHTTPTimeout)
	defer cancel()
	p, err := oidc.NewProvider(oidc.ClientContext(cctx, s.sso.http), issuer)
	if err != nil {
		return nil, err
	}
	var meta struct {
		JWKS string `json:"jwks_uri"`
	}
	if err := p.Claims(&meta); err != nil || meta.JWKS == "" || p.Endpoint().AuthURL == "" || p.Endpoint().TokenURL == "" {
		return nil, errors.New("the document lacks authorization_endpoint, token_endpoint or jwks_uri")
	}
	s.sso.mu.Lock()
	if len(s.sso.prov) > 1000 {
		clear(s.sso.prov)
	}
	s.sso.prov[issuer] = ssoProvider{p: p, at: now}
	s.sso.mu.Unlock()
	return p, nil
}

func ssoSecretContext(ws, id uuid.UUID) []byte {
	return []byte("sso-client-secret:" + ws.String() + ":" + id.String())
}

func (s *Server) ssoRedirectURI() string {
	return strings.TrimRight(s.auth.PublicURL, "/") + ssoCallbackPath
}

func (s *Server) ssoOAuth(set store.SsoSetting, p *oidc.Provider, clientSecret string) *oauth2.Config {
	return &oauth2.Config{
		ClientID: deref(set.OidcClientID), ClientSecret: clientSecret, Endpoint: p.Endpoint(), RedirectURL: s.ssoRedirectURI(),
		Scopes: append([]string{oidc.ScopeOpenID, "email", "profile"}, set.OidcScopes...),
	}
}

func emailDomain(email string) string {
	at := strings.LastIndexByte(email, '@')
	if at < 0 {
		return ""
	}
	return strings.TrimSuffix(strings.ToLower(email[at+1:]), ".")
}

func (s *Server) ssoFor(ctx context.Context, q *store.Queries, email string) (*store.SsoSetting, error) {
	set, err := q.FindSsoByDomain(ctx, emailDomain(email))
	if store.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if set.Protocol == nil {
		return nil, nil
	}
	return &set, nil
}

// ssoGate refuses a member of a workspace that requires single sign-on, whose address is in one of
// its verified domains, unless the session or grant came through that workspace's provider. Owners
// are exempt, so a broken provider never locks the workspace out.
func (s *Server) ssoGate(ctx context.Context, ws, personID uuid.UUID, role string, via *uuid.UUID) error {
	if role == roleOwner {
		return nil
	}
	row, err := s.st.SsoRequiredFor(ctx, store.SsoRequiredForParams{WorkspaceID: ws, PersonID: personID})
	if store.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if via != nil && *via == row.ID {
		return nil
	}
	e := *errSSORequired
	e.Extra = map[string]any{"sso_email": row.Email}
	return &e
}

// ssoScope refuses a session made through a workspace's single sign-on outside that workspace;
// a nil ws is an action for the whole person (passkeys, new workspaces, the account), which such a
// session never takes.
func (p principal) ssoScope(ws *uuid.UUID) error {
	if p.ssoWorkspace == nil || (ws != nil && *ws == *p.ssoWorkspace) {
		return nil
	}
	e := *errSSOSessionScope
	e.Extra = map[string]any{"sso_workspace_id": *p.ssoWorkspace}
	return &e
}

func (s *Server) auditSSO(ctx context.Context, q *store.Queries, set store.SsoSetting, email string, action oas.AuditAction, details auditDetails) error {
	now := s.now()
	if email != "" {
		if person, err := q.GetPersonByEmail(ctx, email); err == nil {
			m, err := q.GetMemberByPerson(ctx, store.GetMemberByPersonParams{WorkspaceID: set.WorkspaceID, PersonID: person.ID})
			if err == nil {
				return writeAudit(ctx, q, principal{workspaceID: set.WorkspaceID, memberID: m.ID}, now, action, oas.AuditTargetSso, set.ID, details)
			}
			if !store.IsNotFound(err) {
				return err
			}
		} else if !store.IsNotFound(err) {
			return err
		}
	}
	return writeSystemAudit(ctx, q, set.WorkspaceID, now, action, oas.AuditTargetSso, set.ID, details)
}

func (s *Server) auditSSORefusal(ctx context.Context, q *store.Queries, set store.SsoSetting, email, reason, detail string) error {
	d := auditDetails{"reason": reason, "protocol": deref(set.Protocol)}
	if email != "" {
		d["email"] = email
	}
	if detail != "" {
		d["detail"] = truncateRunes(detail, 300)
	}
	return s.auditSSO(ctx, q, set, email, oas.AuditSsoSignInRefused, d)
}

func safeReturnTo(v *string) string {
	if v == nil {
		return "/"
	}
	r := *v
	if !strings.HasPrefix(r, "/") || strings.HasPrefix(r, "//") || strings.HasPrefix(r, "/\\") || len(r) > 2000 ||
		strings.ContainsFunc(r, func(c rune) bool { return unicode.IsControl(c) || c == '\\' }) {
		return "/"
	}
	return r
}

func (s *Server) StartSsoSignIn(ctx context.Context, req oas.StartSsoSignInRequestObject) (oas.StartSsoSignInResponseObject, error) {
	if err := s.rateLimit(ctx, rateCheck{"sso-start:" + rateIP(s.clientIP(requestFrom(ctx))), limitSSOStartPerIP}); err != nil {
		return nil, err
	}
	email, err := normalizeEmail(req.Body.Email)
	if err != nil {
		return nil, err
	}
	set, err := s.ssoFor(ctx, s.st.Queries, email)
	if err != nil {
		return nil, err
	}
	if set == nil {
		return oas.StartSsoSignIn200JSONResponse{Body: oas.SsoStart{Sso: false}}, nil
	}
	if deref(set.Protocol) == protocolSAML {
		target, c, err := s.startSAML(ctx, *set, safeReturnTo(req.Body.ReturnTo))
		if err != nil {
			return nil, err
		}
		return oas.StartSsoSignIn200JSONResponse{
			Body:    oas.SsoStart{Sso: true, AuthorizationUrl: &target, Required: new(set.SsoRequired)},
			Headers: oas.StartSsoSignIn200ResponseHeaders{SetCookie: &c},
		}, nil
	}
	p, err := s.oidcProvider(ctx, deref(set.OidcIssuer), false)
	if err != nil {
		s.log.WarnContext(ctx, "sso discovery", "workspace", set.WorkspaceID, "error", err)
		return nil, errSSOUnavailable
	}
	now := s.now()
	if err := s.st.DeleteExpiredSsoLogins(ctx, now); err != nil {
		return nil, err
	}
	state, nonce, verifier := randomToken(32), randomToken(32), oauth2.GenerateVerifier()
	if err := s.st.CreateSsoLogin(ctx, store.CreateSsoLoginParams{
		StateHash: hashSecret(state), WorkspaceID: set.WorkspaceID, SettingsID: set.ID, Nonce: nonce, Verifier: verifier,
		ReturnTo: safeReturnTo(req.Body.ReturnTo), Now: now, ExpiresAt: now.Add(ssoLoginTTL),
	}); err != nil {
		return nil, err
	}
	target := s.ssoOAuth(*set, p, "").AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier),
		oauth2.SetAuthURLParam("login_hint", email))
	c := (&http.Cookie{
		Name: ssoCookie, Value: state, Path: ssoCookiePath, MaxAge: int(ssoLoginTTL / time.Second), HttpOnly: true,
		Secure: s.auth.CookieSecure, SameSite: http.SameSiteLaxMode,
	}).String()
	return oas.StartSsoSignIn200JSONResponse{
		Body:    oas.SsoStart{Sso: true, AuthorizationUrl: &target, Required: new(set.SsoRequired)},
		Headers: oas.StartSsoSignIn200ResponseHeaders{SetCookie: &c},
	}, nil
}

func (s *Server) serveSSOCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := r.URL.Query()
	panel := strings.TrimRight(s.auth.PublicURL, "/")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Add("Set-Cookie", (&http.Cookie{Name: ssoCookie, Path: ssoCookiePath, MaxAge: -1, HttpOnly: true, Secure: s.auth.CookieSecure, SameSite: http.SameSiteLaxMode}).String())
	fail := func(code string) {
		http.Redirect(w, r, panel+"/sign-in?"+url.Values{"sso_error": {code}}.Encode(), http.StatusSeeOther)
	}
	if err := s.rateLimit(ctx, rateCheck{"sso-callback:" + rateIP(s.clientIP(r)), limitSSOStartPerIP}); err != nil {
		fail(ssoFailFailed)
		return
	}
	state := query.Get("state")
	c, err := r.Cookie(ssoCookie)
	if state == "" || err != nil || subtle.ConstantTimeCompare([]byte(c.Value), []byte(state)) != 1 {
		fail(ssoFailExpired)
		return
	}
	login, err := s.st.TakeSsoLogin(ctx, hashSecret(state))
	if err != nil || !login.ExpiresAt.After(s.now()) {
		if err != nil && !store.IsNotFound(err) {
			s.log.ErrorContext(ctx, "sso callback", "error", err)
		}
		fail(ssoFailExpired)
		return
	}
	if query.Get("error") != "" {
		fail(ssoFailDenied)
		return
	}
	if query.Get("code") == "" {
		fail(ssoFailExpired)
		return
	}
	cookie, code, err := s.finishSSO(ctx, login, query.Get("code"))
	if err != nil {
		s.log.ErrorContext(ctx, "sso callback", "workspace", login.WorkspaceID, "error", err)
		fail(ssoFailFailed)
		return
	}
	if code != "" {
		fail(code)
		return
	}
	w.Header().Add("Set-Cookie", cookie)
	http.Redirect(w, r, panel+login.ReturnTo, http.StatusSeeOther)
}

type ssoClaims struct {
	Email         string `json:"email"`
	EmailVerified any    `json:"email_verified"`
	EntraVerified any    `json:"xms_edov"`
	Name          string `json:"name"`
	Locale        string `json:"locale"`
}

func claimTrue(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(t, "true")
	}
	return false
}

func claimStrings(v any) []string {
	switch t := v.(type) {
	case string:
		return []string{t}
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func (s *Server) finishSSO(ctx context.Context, login store.SsoLogin, authCode string) (string, string, error) {
	set, err := s.st.GetSsoSettings(ctx, login.WorkspaceID)
	if store.IsNotFound(err) || (err == nil && (set.ID != login.SettingsID || deref(set.Protocol) != protocolOIDC)) {
		return "", ssoFailExpired, nil
	}
	if err != nil {
		return "", "", err
	}
	refuse := func(email, reason, detail string) (string, string, error) {
		return s.refuseSSO(ctx, set, email, reason, detail)
	}
	clientSecret, err := s.secrets.Open(set.OidcClientSecret, ssoSecretContext(set.WorkspaceID, set.ID))
	if err != nil {
		return "", "", err
	}
	p, err := s.oidcProvider(ctx, deref(set.OidcIssuer), false)
	if err != nil {
		s.log.WarnContext(ctx, "sso discovery", "workspace", set.WorkspaceID, "error", err)
		return "", ssoFailFailed, nil
	}
	cctx, cancel := context.WithTimeout(oidc.ClientContext(ctx, s.sso.http), ssoHTTPTimeout)
	defer cancel()
	tok, err := s.ssoOAuth(set, p, string(clientSecret)).Exchange(cctx, authCode, oauth2.VerifierOption(login.Verifier))
	if err != nil {
		detail := "the token exchange failed"
		var re *oauth2.RetrieveError
		if errors.As(err, &re) && re.ErrorCode != "" {
			detail += ": " + re.ErrorCode
		}
		return refuse("", ssoFailFailed, detail)
	}
	raw, _ := tok.Extra("id_token").(string)
	if raw == "" {
		return refuse("", ssoFailInvalidToken, "the token response holds no id_token")
	}
	idt, err := p.Verifier(&oidc.Config{ClientID: deref(set.OidcClientID), Now: s.now}).Verify(cctx, raw)
	if err != nil {
		return refuse("", ssoFailInvalidToken, err.Error())
	}
	if subtle.ConstantTimeCompare([]byte(idt.Nonce), []byte(login.Nonce)) != 1 {
		return refuse("", ssoFailInvalidToken, "the id_token's nonce does not match this sign-in")
	}
	var claims ssoClaims
	var all map[string]any
	if err := idt.Claims(&claims); err != nil {
		return refuse("", ssoFailInvalidToken, err.Error())
	}
	if err := idt.Claims(&all); err != nil {
		return refuse("", ssoFailInvalidToken, err.Error())
	}
	email, err := normalizeEmail(oas.Email(claims.Email))
	if err != nil {
		return refuse("", ssoFailUnverified, "the id_token holds no valid email")
	}
	if !claimTrue(claims.EmailVerified) && !claimTrue(claims.EntraVerified) {
		return refuse(email, ssoFailUnverified, "")
	}
	return s.completeSSO(ctx, set, ssoIdentity{email: email, name: claims.Name, locale: claims.Locale, groups: claimStrings(all[set.GroupsClaim])})
}

func (s *Server) refuseSSO(ctx context.Context, set store.SsoSetting, email, reason, detail string) (string, string, error) {
	if err := s.st.InTx(ctx, func(q *store.Queries) error {
		return s.auditSSORefusal(ctx, q, set, email, reason, detail)
	}); err != nil {
		return "", "", err
	}
	return "", reason, nil
}

type ssoIdentity struct {
	email, name, locale string
	groups              []string
}

// completeSSO is the sign-in after either protocol verified who the person is: the address must be
// in a domain this workspace verified, then invites, JIT and the session.
func (s *Server) completeSSO(ctx context.Context, set store.SsoSetting, id ssoIdentity) (string, string, error) {
	email, protocol := id.email, deref(set.Protocol)
	ok, err := s.st.SsoDomainVerifiedIn(ctx, store.SsoDomainVerifiedInParams{WorkspaceID: set.WorkspaceID, Domain: emailDomain(email)})
	if err != nil {
		return "", "", err
	}
	if !ok {
		return s.refuseSSO(ctx, set, email, ssoFailUnclaimed, "")
	}
	if len(set.AllowedGroups) > 0 && !slices.ContainsFunc(id.groups, func(g string) bool { return slices.Contains(set.AllowedGroups, g) }) {
		return s.refuseSSO(ctx, set, email, ssoFailGroup, "")
	}
	now := s.now()
	var cookie string
	refused, first := false, false
	err = s.st.InTx(ctx, func(q *store.Queries) error {
		wasMember := false
		if person, err := q.GetPersonByEmail(ctx, email); err == nil {
			_, err := q.GetMemberByPerson(ctx, store.GetMemberByPersonParams{WorkspaceID: set.WorkspaceID, PersonID: person.ID})
			if err != nil && !store.IsNotFound(err) {
				return err
			}
			wasMember = err == nil
		} else if !store.IsNotFound(err) {
			return err
		}
		personID, found, err := acceptInvites(ctx, q, email, &set.WorkspaceID, now)
		if err != nil {
			return err
		}
		details := auditDetails{"protocol": protocol, "email": email}
		isMember := wasMember
		if found && !wasMember {
			_, err := q.GetMemberByPerson(ctx, store.GetMemberByPersonParams{WorkspaceID: set.WorkspaceID, PersonID: personID})
			if err != nil && !store.IsNotFound(err) {
				return err
			}
			if err == nil {
				isMember, details["joined"] = true, "invite"
			}
		}
		if !isMember && set.Jit {
			if !found {
				locale := string(oas.En)
				if validLocale(oas.Locale(id.locale)) {
					locale = id.locale
				}
				pe, err := createPerson(ctx, q, email, truncateRunes(strings.TrimSpace(id.name), 200), locale)
				if err != nil {
					return err
				}
				personID = pe.ID
			}
			m, err := q.CreateMember(ctx, store.CreateMemberParams{ID: newID(), WorkspaceID: set.WorkspaceID, PersonID: personID, Role: set.JitRole})
			switch {
			case store.IsNotFound(err):
			case err != nil:
				return err
			default:
				if err := writeAudit(ctx, q, principal{workspaceID: m.WorkspaceID, memberID: m.ID}, now, oas.AuditMemberJoined, oas.AuditTargetMember, m.ID,
					auditDetails{"role": m.Role, "via": methodSSO}); err != nil {
					return err
				}
				details["joined"] = "jit"
			}
			isMember = true
		}
		if !isMember {
			refused = true
			return s.auditSSORefusal(ctx, q, set, email, ssoFailNotMember, "")
		}
		if err := s.auditSSO(ctx, q, set, email, oas.AuditSsoSignedIn, details); err != nil {
			return err
		}
		n, err := q.RecordSsoSignIn(ctx, store.RecordSsoSignInParams{WorkspaceID: set.WorkspaceID, PersonID: personID, Now: now})
		if err != nil {
			return err
		}
		first = n > 0
		cookie, err = s.startSession(ctx, q, personID, methodSSO, &set)
		return err
	})
	if err != nil {
		return "", "", err
	}
	if refused {
		return "", ssoFailNotMember, nil
	}
	if first {
		s.mailFirstSSOSignIn(ctx, set.WorkspaceID, email)
	}
	return cookie, "", nil
}

// mailFirstSSOSignIn tells the person that a workspace's identity provider signed them in, so a
// provider that signs in someone it should not is noticed.
func (s *Server) mailFirstSSOSignIn(ctx context.Context, ws uuid.UUID, email string) {
	w, err := s.st.GetWorkspace(ctx, ws)
	if err != nil {
		s.log.WarnContext(ctx, "sso sign-in mail", "workspace", ws, "error", err)
		return
	}
	locale, err := s.localeFor(ctx, email, s.now(), string(oas.En))
	if err != nil {
		s.log.WarnContext(ctx, "sso sign-in mail", "workspace", ws, "error", err)
		return
	}
	msg, err := yuvamail.Render("sso_first_sign_in", locale, map[string]any{"Workspace": w.Name, "Email": email, "URL": s.auth.PublicURL})
	if err != nil {
		s.log.WarnContext(ctx, "sso sign-in mail", "workspace", ws, "error", err)
		return
	}
	msg.To = email
	s.sendMail(ctx, msg)
}
