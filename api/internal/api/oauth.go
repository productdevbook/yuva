package api

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
	"github.com/productdevbook/yuva/api/internal/webhook"
)

const (
	oauthAccessPrefix  = "yuva_at_"
	oauthRefreshPrefix = "yuva_rt_"
	oauthClientPrefix  = "yuva_client_"
	oauthAccessTTL     = time.Hour
	oauthRefreshTTL    = 30 * 24 * time.Hour
	oauthCodeTTL       = time.Minute
	oauthRequestTTL    = 10 * time.Minute
	oauthMetadataTTL   = 24 * time.Hour
	oauthConsentPath   = "/oauth/consent"
	oauthMaxDocBytes   = 64 << 10
	oauthMaxRedirects  = 20
	oauthMaxURIBytes   = 2000
	oauthMaxStateBytes = 2000
	tokenKindAccess    = "access"
	tokenKindRefresh   = "refresh"
	clientKindRegister = "registered"
)

var (
	errTokenExpired       = problem(http.StatusUnauthorized, "token_expired", "the access token has expired; refresh it")
	errTokenWrongResource = problem(http.StatusUnauthorized, "token_wrong_resource", "this token was issued for /mcp only; request one for the server's public URL to call /v1")
	errTokenWorkspace     = problem(http.StatusForbidden, "workspace_mismatch", "the token belongs to another workspace")
	errOAuthRequestGone   = problem(http.StatusNotFound, "not_found", "no such authorization request, or it has expired")
	errOAuthGrantGone     = problem(http.StatusNotFound, "not_found", "no such connected app")
)

// managerScopes need an owner or admin; tokens of agents are never offered them. feedback:write
// is for API keys only and offered to nobody.
var (
	managerScopes  = []oas.ApiKeyScope{oas.InboxesManage, oas.LabelsWrite, oas.CannedRepliesWrite, oas.WebhooksManage, oas.WorkspaceManage}
	keyOnlyScopes  = []oas.ApiKeyScope{oas.FeedbackWrite}
	privateSchemeR = regexp.MustCompile(`^[a-z][a-z0-9+.-]*$`)
	refusedSchemes = []string{"javascript", "data", "file", "vbscript", "about", "blob", "ftp", "ws", "wss", "mailto", "tel", "http", "https"}
)

func (s *Server) apiResource() string { return s.auth.PublicURL }
func (s *Server) mcpResource() string { return s.auth.PublicURL + "/mcp" }

func normalizeResource(r string) string { return strings.TrimSuffix(strings.TrimSpace(r), "/") }

func (s *Server) resourceKind(resource string) oas.OAuthResource {
	if resource == s.mcpResource() {
		return oas.OAuthResourceMcp
	}
	return oas.OAuthResourceApi
}

func grantableScopes() []string {
	var out []string
	for _, sc := range allScopes {
		if !slices.Contains(keyOnlyScopes, sc) {
			out = append(out, string(sc))
		}
	}
	return out
}

// offeredScopes are the requested scopes (every scope when none were requested) that a member
// with role can use.
func offeredScopes(requested []string, role string) []oas.ApiKeyScope {
	out := []oas.ApiKeyScope{}
	for _, sc := range allScopes {
		if slices.Contains(keyOnlyScopes, sc) || (requested != nil && !slices.Contains(requested, string(sc))) {
			continue
		}
		if role == roleAgent && slices.Contains(managerScopes, sc) {
			continue
		}
		out = append(out, sc)
	}
	return out
}

// resolveOAuthAccess turns an OAuth access token into its member, narrowed by the grant.
func (s *Server) resolveOAuthAccess(ctx context.Context, token string, kind access, selected *uuid.UUID, forMCP bool) (principal, error) {
	a, err := s.st.GetOAuthAccess(ctx, hashSecret(token))
	if store.IsNotFound(err) {
		return principal{}, errUnauthenticated
	}
	if err != nil {
		return principal{}, err
	}
	if !s.now().Before(a.ExpiresAt) {
		return principal{}, errTokenExpired
	}
	mcpOnly := a.Resource == s.mcpResource()
	if !mcpOnly && a.Resource != s.apiResource() {
		return principal{}, errUnauthenticated
	}
	if mcpOnly && !forMCP {
		return principal{}, errTokenWrongResource
	}
	if kind != accessMemberOrKey {
		return principal{}, errMemberSessionRequired
	}
	if selected != nil && *selected != a.WorkspaceID {
		return principal{}, errTokenWorkspace
	}
	return principal{
		grantID: a.GrantID, workspaceID: a.WorkspaceID, memberID: a.MemberID, role: a.Role, scopes: a.Scopes,
		via: a.ClientName, mcpOnly: mcpOnly, botsMaySend: a.BotsMaySend,
	}, nil
}

// meterBearer applies the per-token rate limit to an API key or OAuth grant and counts a grant's
// request for Connected apps.
func (s *Server) meterBearer(ctx context.Context, p principal) error {
	key := "bearer:" + p.keyID.String()
	if p.isGrant() {
		key = "bearer:" + p.grantID.String()
	}
	now := s.now()
	if ok, wait := s.limits.allowWait(key, limitBearer, now); !ok {
		e := *errClientRateLimited
		e.RetryAfter = int(math.Ceil(wait.Seconds()))
		return &e
	}
	if !p.isGrant() {
		return nil
	}
	return s.st.CountOAuthGrantRequest(ctx, store.CountOAuthGrantRequestParams{
		Now: now, Month: monthOf(now), WorkspaceID: p.workspaceID, ID: p.grantID,
	})
}

func (s *Server) serveOAuthServerMetadata(w http.ResponseWriter, r *http.Request) {
	yes := true
	base := s.auth.PublicURL
	register, revoke := base+"/oauth/register", base+"/oauth/revoke"
	scopes := grantableScopes()
	writeOAuthJSON(w, http.StatusOK, oas.OAuthServerMetadata{
		Issuer:                                     base,
		AuthorizationEndpoint:                      base + "/oauth/authorize",
		TokenEndpoint:                              base + "/oauth/token",
		RegistrationEndpoint:                       &register,
		RevocationEndpoint:                         &revoke,
		ScopesSupported:                            &scopes,
		ResponseTypesSupported:                     []string{"code"},
		GrantTypesSupported:                        &[]string{"authorization_code", "refresh_token"},
		TokenEndpointAuthMethodsSupported:          &[]string{"none"},
		RevocationEndpointAuthMethodsSupported:     &[]string{"none"},
		CodeChallengeMethodsSupported:              &[]string{"S256"},
		ClientIdMetadataDocumentSupported:          &yes,
		AuthorizationResponseIssParameterSupported: &yes,
	})
}

func (s *Server) serveResourceMetadata(resource, name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scopes := grantableScopes()
		writeOAuthJSON(w, http.StatusOK, oas.OAuthResourceMetadata{
			Resource: resource, AuthorizationServers: &[]string{s.auth.PublicURL}, ScopesSupported: &scopes,
			BearerMethodsSupported: &[]string{"header"}, ResourceName: &name,
		})
	}
}

func writeOAuthJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type oauthError struct {
	status      int
	code        string
	description string
}

func (e *oauthError) Error() string { return e.code + ": " + e.description }

func oauthErr(code, description string) *oauthError {
	return &oauthError{status: http.StatusBadRequest, code: code, description: description}
}

func (s *Server) writeOAuthError(w http.ResponseWriter, r *http.Request, err error) {
	var oe *oauthError
	if errors.As(err, &oe) {
		d := oe.description
		writeOAuthJSON(w, oe.status, oas.OAuthError{Error: oe.code, ErrorDescription: &d})
		return
	}
	var ae *apiError
	if errors.As(err, &ae) && ae.Status == http.StatusTooManyRequests {
		writeProblem(w, ae)
		return
	}
	s.log.ErrorContext(r.Context(), "oauth", slog.String("path", r.URL.Path), slog.Any("error", err))
	writeOAuthJSON(w, http.StatusInternalServerError, oas.OAuthError{Error: "server_error"})
}

// validRedirectURI accepts https, http on a loopback host, and private-use schemes of native apps.
func validRedirectURI(raw string) bool {
	if raw == "" || len(raw) > oauthMaxURIBytes || strings.Contains(raw, "#") {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil {
		return false
	}
	switch u.Scheme {
	case "https":
		return u.Host != ""
	case "http":
		return loopbackHost(u.Hostname())
	}
	if slices.Contains(refusedSchemes, u.Scheme) || !privateSchemeR.MatchString(u.Scheme) {
		return false
	}
	return u.Opaque != "" || u.Path != "" || u.Host != ""
}

// redirectMatches compares exactly, except that an http redirect on a loopback host may use any
// port (RFC 8252 section 7.3). localhost counts as loopback because Claude Code registers it.
func redirectMatches(registered, requested string) bool {
	if registered == requested {
		return true
	}
	r, err := url.Parse(registered)
	if err != nil || r.Scheme != "http" || !loopbackHost(r.Hostname()) {
		return false
	}
	q, err := url.Parse(requested)
	if err != nil || !validRedirectURI(requested) {
		return false
	}
	return q.Scheme == r.Scheme && q.Hostname() == r.Hostname() && q.User == nil &&
		q.EscapedPath() == r.EscapedPath() && q.RawQuery == r.RawQuery && q.ForceQuery == r.ForceQuery
}

func redirectHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if u.Scheme == "http" || u.Scheme == "https" {
		return u.Host
	}
	return u.Scheme + ":"
}

func withQuery(raw string, params map[string]string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	for k, v := range params {
		if v != "" {
			q.Set(k, v)
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

type clientMetadata struct {
	ClientID                string   `json:"client_id"`
	RedirectURIs            []string `json:"redirect_uris"`
	ClientName              string   `json:"client_name"`
	ClientURI               string   `json:"client_uri"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
}

// check validates what both registration and metadata documents describe, and fills the name.
func (m *clientMetadata) check() error {
	if len(m.RedirectURIs) == 0 || len(m.RedirectURIs) > oauthMaxRedirects {
		return oauthErr("invalid_redirect_uri", "redirect_uris must list 1 to 20 URIs")
	}
	for _, u := range m.RedirectURIs {
		if !validRedirectURI(u) {
			return oauthErr("invalid_redirect_uri", "redirect URIs must be https, http on a loopback host, or a private-use scheme, without a fragment: "+u)
		}
	}
	for _, g := range m.GrantTypes {
		if g != "authorization_code" && g != "refresh_token" {
			return oauthErr("invalid_client_metadata", "grant_types may hold authorization_code and refresh_token")
		}
	}
	for _, t := range m.ResponseTypes {
		if t != "code" {
			return oauthErr("invalid_client_metadata", "response_types may hold only code")
		}
	}
	m.ClientName = strings.TrimSpace(m.ClientName)
	if n := []rune(m.ClientName); len(n) > 200 {
		m.ClientName = string(n[:200])
	}
	if m.ClientName == "" {
		m.ClientName = redirectHost(m.RedirectURIs[0])
	}
	if u, err := url.Parse(m.ClientURI); m.ClientURI != "" && (err != nil || u.Scheme != "https" || u.Host == "" || len(m.ClientURI) > oauthMaxURIBytes) {
		m.ClientURI = ""
	}
	return nil
}

func optional(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func (s *Server) serveOAuthRegister(w http.ResponseWriter, r *http.Request) {
	if err := s.rateLimit(rateCheck{"oauth-register:" + rateIP(s.clientIP(r)), limitRegisterPerIP}); err != nil {
		s.writeOAuthError(w, r, &oauthError{status: http.StatusTooManyRequests, code: "rate_limited", description: "too many registrations from this address; try again later"})
		return
	}
	var m clientMetadata
	if err := json.NewDecoder(io.LimitReader(r.Body, oauthMaxDocBytes)).Decode(&m); err != nil {
		s.writeOAuthError(w, r, oauthErr("invalid_client_metadata", "the body must be a JSON object of client metadata"))
		return
	}
	if err := m.check(); err != nil {
		s.writeOAuthError(w, r, err)
		return
	}
	now := s.now()
	c, err := s.st.CreateOAuthClient(r.Context(), store.CreateOAuthClientParams{
		ID: newID(), ClientID: oauthClientPrefix + randomToken(18), Kind: clientKindRegister, Name: m.ClientName,
		ClientUri: optional(m.ClientURI), RedirectUris: m.RedirectURIs, CreatedAt: now,
	})
	if err != nil {
		s.writeOAuthError(w, r, err)
		return
	}
	grants := []oas.OAuthClientRegistrationGrantTypes{"authorization_code", "refresh_token"}
	responses := []oas.OAuthClientRegistrationResponseTypes{"code"}
	none := "none"
	writeOAuthJSON(w, http.StatusCreated, oas.OAuthClientRegistration{
		ClientId: c.ClientID, ClientIdIssuedAt: now.Unix(), RedirectUris: c.RedirectUris, ClientName: &c.Name,
		ClientUri: c.ClientUri, TokenEndpointAuthMethod: &none, GrantTypes: &grants, ResponseTypes: &responses,
	})
}

var errUnknownClient = oauthErr("invalid_client", "unknown client_id")

// oauthClient finds a registered client, or fetches and caches a Client ID Metadata Document.
func (s *Server) oauthClient(ctx context.Context, clientID string) (store.OauthClient, error) {
	c, err := s.st.GetOAuthClientByClientID(ctx, clientID)
	if err != nil && !store.IsNotFound(err) {
		return c, err
	}
	if !strings.HasPrefix(clientID, "https://") {
		if err != nil || c.Kind != clientKindRegister {
			return c, errUnknownClient
		}
		return c, nil
	}
	if err == nil && c.FetchedAt != nil && s.now().Sub(*c.FetchedAt) < oauthMetadataTTL {
		return c, nil
	}
	m, err := s.fetchClientMetadata(ctx, clientID)
	if err != nil {
		return c, err
	}
	return s.st.SaveMetadataClient(ctx, store.SaveMetadataClientParams{
		ID: newID(), ClientID: clientID, Name: m.ClientName, ClientUri: optional(m.ClientURI), RedirectUris: m.RedirectURIs, Now: s.now(),
	})
}

func (s *Server) fetchClientMetadata(ctx context.Context, clientID string) (clientMetadata, error) {
	var m clientMetadata
	u, err := url.Parse(clientID)
	if err != nil || len(clientID) > oauthMaxURIBytes || u.Scheme != "https" || u.User != nil || u.Fragment != "" ||
		u.Path == "" || u.Path == "/" || slices.Contains(strings.Split(u.Path, "/"), ".") || slices.Contains(strings.Split(u.Path, "/"), "..") {
		return m, oauthErr("invalid_client", "a client_id URL must be https with a path, without credentials, fragment or dot segments")
	}
	if _, err := webhook.CheckURL(clientID, s.webhooks.AllowPrivate); err != nil {
		return m, oauthErr("invalid_client", "the client_id URL points to an address the server does not fetch")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, clientID, nil)
	if err != nil {
		return m, oauthErr("invalid_client", "the client_id URL is not valid")
	}
	req.Header.Set("Accept", "application/json")
	res, err := s.hooks.Do(req)
	if err != nil {
		return m, oauthErr("invalid_client", "the client metadata document could not be fetched")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return m, oauthErr("invalid_client", "the client metadata document answered "+res.Status)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, oauthMaxDocBytes+1))
	if err != nil || len(body) > oauthMaxDocBytes || json.Unmarshal(body, &m) != nil {
		return m, oauthErr("invalid_client", "the client metadata document is not a JSON object of at most 64 KiB")
	}
	if m.ClientID != clientID {
		return m, oauthErr("invalid_client", "the client metadata document's client_id must equal its URL")
	}
	if m.TokenEndpointAuthMethod != "" && m.TokenEndpointAuthMethod != "none" {
		return m, oauthErr("invalid_client", "only public clients (token_endpoint_auth_method none) are supported")
	}
	if err := m.check(); err != nil {
		return m, oauthErr("invalid_client", err.(*oauthError).description)
	}
	if m.ClientName == redirectHost(m.RedirectURIs[0]) {
		m.ClientName = u.Host
	}
	return m, nil
}

func validChallenge(c string) bool {
	if len(c) != 43 {
		return false
	}
	_, err := base64.RawURLEncoding.DecodeString(c)
	return err == nil
}

func (s *Server) serveOAuthAuthorize(w http.ResponseWriter, r *http.Request) {
	plain := func(status int, msg string) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, msg+"\n")
	}
	if err := s.rateLimit(rateCheck{"oauth-authorize:" + rateIP(s.clientIP(r)), limitAuthorizePerIP}); err != nil {
		plain(http.StatusTooManyRequests, "rate_limited: too many authorization requests; try again in a minute")
		return
	}
	q := r.URL.Query()
	ctx := r.Context()
	client, err := s.oauthClient(ctx, q.Get("client_id"))
	if err != nil {
		var oe *oauthError
		if errors.As(err, &oe) {
			plain(http.StatusBadRequest, oe.code+": "+oe.description)
			return
		}
		s.log.ErrorContext(ctx, "oauth authorize", slog.Any("error", err))
		plain(http.StatusInternalServerError, "server_error")
		return
	}
	redirect := q.Get("redirect_uri")
	switch {
	case redirect == "" && len(client.RedirectUris) == 1:
		redirect = client.RedirectUris[0]
	case redirect == "":
		plain(http.StatusBadRequest, "invalid_request: redirect_uri is required for a client with several redirect URIs")
		return
	case !slices.ContainsFunc(client.RedirectUris, func(registered string) bool { return redirectMatches(registered, redirect) }):
		plain(http.StatusBadRequest, "invalid_request: redirect_uri does not match any of the client's redirect URIs")
		return
	}
	state := q.Get("state")
	fail := func(code, description string) {
		http.Redirect(w, r, withQuery(redirect, map[string]string{
			"error": code, "error_description": description, "state": state, "iss": s.auth.PublicURL,
		}), http.StatusFound)
	}
	if s.noPanel {
		fail("temporarily_unavailable", "this server does not serve the panel, so it cannot ask for consent; use an API key")
		return
	}
	if q.Get("response_type") != "code" {
		fail("unsupported_response_type", "only response_type=code is supported")
		return
	}
	challenge := q.Get("code_challenge")
	if q.Get("code_challenge_method") != "S256" || !validChallenge(challenge) {
		fail("invalid_request", "PKCE is required: code_challenge_method=S256 and a 43-character code_challenge")
		return
	}
	if len(state) > oauthMaxStateBytes {
		fail("invalid_request", "state is too long")
		return
	}
	resources := q["resource"]
	if len(resources) != 1 {
		fail("invalid_target", "send exactly one resource: "+s.mcpResource()+" or "+s.apiResource())
		return
	}
	resource := normalizeResource(resources[0])
	if resource != s.mcpResource() && resource != s.apiResource() {
		fail("invalid_target", "resource must be "+s.mcpResource()+" or "+s.apiResource())
		return
	}
	var scopes []string
	if raw := strings.Fields(q.Get("scope")); len(raw) > 0 {
		scopes = []string{}
		for _, sc := range raw {
			if !slices.Contains(allScopes, oas.ApiKeyScope(sc)) {
				fail("invalid_scope", "unknown scope "+sc)
				return
			}
			if !slices.Contains(scopes, sc) {
				scopes = append(scopes, sc)
			}
		}
	}
	now := s.now()
	id := uuid.New()
	if err := s.st.CreateOAuthRequest(ctx, store.CreateOAuthRequestParams{
		ID: id, ClientID: client.ID, RedirectUri: redirect, State: optional(state), CodeChallenge: challenge,
		Scopes: scopes, Resource: resource, CreatedAt: now, ExpiresAt: now.Add(oauthRequestTTL),
	}); err != nil {
		s.log.ErrorContext(ctx, "oauth authorize", slog.Any("error", err))
		plain(http.StatusInternalServerError, "server_error")
		return
	}
	http.Redirect(w, r, s.auth.PublicURL+oauthConsentPath+"?request="+id.String(), http.StatusFound)
}

type tokenPair struct {
	access, refresh string
}

func (s *Server) issueTokens(ctx context.Context, q *store.Queries, workspaceID, grantID uuid.UUID, now time.Time) (tokenPair, error) {
	t := tokenPair{access: oauthAccessPrefix + randomToken(32), refresh: oauthRefreshPrefix + randomToken(32)}
	for _, tk := range []struct {
		kind, secret string
		ttl          time.Duration
	}{{tokenKindAccess, t.access, oauthAccessTTL}, {tokenKindRefresh, t.refresh, oauthRefreshTTL}} {
		if err := q.CreateOAuthToken(ctx, store.CreateOAuthTokenParams{
			WorkspaceID: workspaceID, ID: newID(), GrantID: grantID, Kind: tk.kind, TokenHash: hashSecret(tk.secret),
			CreatedAt: now, ExpiresAt: now.Add(tk.ttl),
		}); err != nil {
			return t, err
		}
	}
	return t, nil
}

func revokeGrant(ctx context.Context, q *store.Queries, workspaceID, grantID uuid.UUID, now time.Time) (bool, error) {
	n, err := q.RevokeOAuthGrant(ctx, store.RevokeOAuthGrantParams{Now: now, WorkspaceID: workspaceID, ID: grantID})
	if err != nil || n == 0 {
		return false, err
	}
	if err := q.DeleteOAuthGrantTokens(ctx, store.DeleteOAuthGrantTokensParams{WorkspaceID: workspaceID, GrantID: grantID}); err != nil {
		return false, err
	}
	return true, q.DeleteOAuthGrantCodes(ctx, store.DeleteOAuthGrantCodesParams{WorkspaceID: workspaceID, GrantID: grantID})
}

func formClientID(r *http.Request) string {
	if id := r.PostForm.Get("client_id"); id != "" {
		return id
	}
	if u, _, ok := r.BasicAuth(); ok {
		if id, err := url.QueryUnescape(u); err == nil {
			return id
		}
	}
	return ""
}

func (s *Server) serveOAuthToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || r.PostForm == nil {
		s.writeOAuthError(w, r, oauthErr("invalid_request", "send the parameters as application/x-www-form-urlencoded"))
		return
	}
	f := r.PostForm
	clientID := formClientID(r)
	if clientID == "" {
		s.writeOAuthError(w, r, oauthErr("invalid_request", "client_id is required"))
		return
	}
	var resource string
	if v, ok := f["resource"]; ok {
		if len(v) != 1 {
			s.writeOAuthError(w, r, oauthErr("invalid_target", "send at most one resource"))
			return
		}
		resource = normalizeResource(v[0])
	}
	now := s.now()
	var (
		out    oas.OAuthTokenResponse
		failed *oauthError
	)
	finish := func(q *store.Queries, workspaceID, grantID uuid.UUID, scopes []string, granted string) error {
		if resource != "" && resource != granted {
			failed = oauthErr("invalid_target", "resource does not match the authorization")
			return nil
		}
		t, err := s.issueTokens(r.Context(), q, workspaceID, grantID, now)
		if err != nil {
			return err
		}
		out = oas.OAuthTokenResponse{
			AccessToken: t.access, RefreshToken: t.refresh, TokenType: oas.Bearer, ExpiresIn: int(oauthAccessTTL / time.Second),
			Scope: strings.Join(scopes, " "),
		}
		return nil
	}
	var err error
	switch f.Get("grant_type") {
	case "authorization_code":
		code, verifier := f.Get("code"), f.Get("code_verifier")
		if code == "" || verifier == "" {
			s.writeOAuthError(w, r, oauthErr("invalid_request", "code and code_verifier are required"))
			return
		}
		err = s.st.InTx(r.Context(), func(q *store.Queries) error {
			c, err := q.LockOAuthCode(r.Context(), hashSecret(code))
			if store.IsNotFound(err) {
				failed = oauthErr("invalid_grant", "the code is not valid")
				return nil
			}
			if err != nil {
				return err
			}
			switch {
			case c.ClientKey != clientID:
				failed = oauthErr("invalid_grant", "the code was issued to another client")
			case c.UsedAt != nil:
				failed = oauthErr("invalid_grant", "the code was already used; the grant is revoked")
				_, err = revokeGrant(r.Context(), q, c.WorkspaceID, c.GrantID, now)
				return err
			case !now.Before(c.ExpiresAt):
				failed = oauthErr("invalid_grant", "the code has expired")
			case f.Get("redirect_uri") != "" && f.Get("redirect_uri") != c.RedirectUri:
				failed = oauthErr("invalid_grant", "redirect_uri does not match the authorization request")
			case !pkceMatches(verifier, c.CodeChallenge):
				failed = oauthErr("invalid_grant", "code_verifier does not match the code_challenge")
			}
			if failed != nil {
				return nil
			}
			if err := q.MarkOAuthCodeUsed(r.Context(), store.MarkOAuthCodeUsedParams{Now: now, WorkspaceID: c.WorkspaceID, ID: c.ID}); err != nil {
				return err
			}
			return finish(q, c.WorkspaceID, c.GrantID, c.Scopes, c.Resource)
		})
	case "refresh_token":
		token := f.Get("refresh_token")
		if token == "" {
			s.writeOAuthError(w, r, oauthErr("invalid_request", "refresh_token is required"))
			return
		}
		err = s.st.InTx(r.Context(), func(q *store.Queries) error {
			t, err := q.LockOAuthRefreshToken(r.Context(), hashSecret(token))
			if store.IsNotFound(err) {
				failed = oauthErr("invalid_grant", "the refresh token is not valid")
				return nil
			}
			if err != nil {
				return err
			}
			switch {
			case t.ClientKey != clientID:
				failed = oauthErr("invalid_grant", "the refresh token was issued to another client")
			case t.UsedAt != nil:
				failed = oauthErr("invalid_grant", "the refresh token was already used; the grant is revoked")
				_, err = revokeGrant(r.Context(), q, t.WorkspaceID, t.GrantID, now)
				return err
			case !now.Before(t.ExpiresAt):
				failed = oauthErr("invalid_grant", "the refresh token has expired")
			}
			if failed != nil {
				return nil
			}
			if err := q.MarkOAuthTokenUsed(r.Context(), store.MarkOAuthTokenUsedParams{Now: now, WorkspaceID: t.WorkspaceID, ID: t.ID}); err != nil {
				return err
			}
			return finish(q, t.WorkspaceID, t.GrantID, t.Scopes, t.Resource)
		})
	case "":
		failed = oauthErr("invalid_request", "grant_type is required")
	default:
		failed = oauthErr("unsupported_grant_type", "grant_type must be authorization_code or refresh_token")
	}
	switch {
	case err != nil:
		s.writeOAuthError(w, r, err)
	case failed != nil:
		s.writeOAuthError(w, r, failed)
	default:
		writeOAuthJSON(w, http.StatusOK, out)
	}
}

func pkceMatches(verifier, challenge string) bool {
	if len(verifier) < 43 || len(verifier) > 128 {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	return subtle.ConstantTimeCompare([]byte(base64.RawURLEncoding.EncodeToString(sum[:])), []byte(challenge)) == 1
}

func (s *Server) serveOAuthRevoke(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || r.PostForm.Get("token") == "" {
		s.writeOAuthError(w, r, oauthErr("invalid_request", "token is required"))
		return
	}
	ctx := r.Context()
	t, err := s.st.GetOAuthTokenByHash(ctx, hashSecret(r.PostForm.Get("token")))
	if store.IsNotFound(err) {
		w.WriteHeader(http.StatusOK)
		return
	}
	if err != nil {
		s.writeOAuthError(w, r, err)
		return
	}
	if id := formClientID(r); id != "" && id != t.ClientKey {
		w.WriteHeader(http.StatusOK)
		return
	}
	if t.Kind == tokenKindRefresh {
		err = s.st.InTx(ctx, func(q *store.Queries) error {
			_, err := revokeGrant(ctx, q, t.WorkspaceID, t.GrantID, s.now())
			return err
		})
	} else {
		err = s.st.DeleteOAuthToken(ctx, store.DeleteOAuthTokenParams{WorkspaceID: t.WorkspaceID, ID: t.ID})
	}
	if err != nil {
		s.writeOAuthError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

var oauthCORSPaths = []string{"/oauth/register", "/oauth/token", "/oauth/revoke"}

// oauthCORS lets browser-based clients read metadata and call the token endpoints; none of them
// uses cookies.
func oauthCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/.well-known/oauth-") && !slices.Contains(oauthCORSPaths, r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, MCP-Protocol-Version")
			w.Header().Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
