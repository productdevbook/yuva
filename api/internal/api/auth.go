package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	sessionCookie    = "yuva_session"
	workspaceHeader  = "Yuva-Workspace"
	sessionTTL       = 30 * 24 * time.Hour
	touchEvery       = time.Minute
	roleOwner        = string(oas.Owner)
	roleAdmin        = string(oas.Admin)
	roleAgent        = string(oas.Agent)
	methodCode       = "code"
	methodPasskey    = "passkey"
	apiKeyPrefixHead = "yuva_"
)

type access int

const (
	accessMemberOrKey access = iota
	accessPublic
	accessPerson
	accessMember
	accessContact
)

var operationAccess = map[string]access{
	"GetHealthz":          accessPublic,
	"GetReadyz":           accessPublic,
	"GetVersion":          accessPublic,
	"RequestSignInCode":   accessPublic,
	"VerifySignInCode":    accessPublic,
	"BeginPasskeySignIn":  accessPublic,
	"FinishPasskeySignIn": accessPublic,

	"SignOut":                   accessPerson,
	"GetMe":                     accessPerson,
	"UpdateMe":                  accessPerson,
	"ListPasskeys":              accessPerson,
	"BeginPasskeyRegistration":  accessPerson,
	"FinishPasskeyRegistration": accessPerson,
	"DeletePasskey":             accessPerson,
	"DeleteMe":                  accessPerson,

	"MarkConversationRead": accessMember,
	"SetMemberTyping":      accessMember,

	"GetVapidPublicKey":          accessPerson,
	"ListPushSubscriptions":      accessPerson,
	"CreatePushSubscription":     accessPerson,
	"DeletePushSubscription":     accessPerson,
	"TestPushSubscription":       accessPerson,
	"GetNotificationSettings":    accessMember,
	"UpdateNotificationSettings": accessMember,
	"SetInboxNotifications":      accessMember,
	"DeleteInboxNotifications":   accessMember,

	"CreateClientSession":        accessPublic,
	"GetClientChannel":           accessPublic,
	"GetClientSession":           accessContact,
	"DeleteClientSession":        accessContact,
	"ListClientConversations":    accessContact,
	"CreateClientConversation":   accessContact,
	"GetClientConversation":      accessContact,
	"ListClientMessages":         accessContact,
	"CreateClientMessage":        accessContact,
	"MarkClientConversationRead": accessContact,
	"SetClientTyping":            accessContact,
	"SetClientContactEmail":      accessContact,
	"CreateClientFeedback":       accessContact,
	"DownloadClientAttachment":   accessContact,

	"UpdateMember": accessMember,
	"RemoveMember": accessMember,
	"CreateInvite": accessMember,
	"DeleteInvite": accessMember,
	"ListApiKeys":  accessMember,
	"CreateApiKey": accessMember,
	"RevokeApiKey": accessMember,

	"DeleteWorkspace": accessMember,
}

type principal struct {
	personID    uuid.UUID
	sessionID   uuid.UUID
	keyID       uuid.UUID
	workspaceID uuid.UUID
	memberID    uuid.UUID
	role        string
}

func (p principal) isKey() bool { return p.keyID != uuid.Nil() }

type ctxKey int

const (
	principalKey ctxKey = iota
	requestKey
)

var (
	errUnauthenticated       = problem(http.StatusUnauthorized, "unauthenticated", "sign in or send a valid API key")
	errMemberSessionRequired = problem(http.StatusForbidden, "member_session_required", "this endpoint needs a member session, not an API key")
	errNotAMember            = problem(http.StatusForbidden, "not_a_member", "you are not a member of this workspace")
	errWorkspaceRequired     = problem(http.StatusBadRequest, "workspace_required", "you belong to several workspaces; name one in the Yuva-Workspace header")
	errWorkspaceMismatch     = problem(http.StatusForbidden, "workspace_mismatch", "the API key belongs to another workspace")
	errForbidden             = problem(http.StatusForbidden, "forbidden", "your role does not allow this")
)

var workspaceQueryOperations = map[string]bool{"DownloadAttachment": true}

// workspaceFromQuery lets clients that cannot set headers (WebSocket, links) name the workspace
// with ?workspace_id=.
func workspaceFromQuery(r *http.Request) error {
	v := r.URL.Query().Get("workspace_id")
	if v == "" {
		return nil
	}
	if h := r.Header.Get(workspaceHeader); h != "" && h != v {
		return errValidation("workspace_id and Yuva-Workspace name different workspaces")
	}
	r.Header.Set(workspaceHeader, v)
	return nil
}

func (s *Server) authenticate(next oas.StrictHandlerFunc, operationID string) oas.StrictHandlerFunc {
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request, request any) (any, error) {
		ctx = context.WithValue(ctx, requestKey, r)
		kind := operationAccess[operationID]
		if kind == accessPublic {
			return next(ctx, w, r, request)
		}
		if kind == accessContact {
			token, _ := bearerToken(r)
			c, err := s.resolveContact(ctx, token)
			if err != nil {
				return nil, err
			}
			return next(context.WithValue(ctx, contactKey, c), w, r, request)
		}
		if workspaceQueryOperations[operationID] {
			if err := workspaceFromQuery(r); err != nil {
				return nil, err
			}
		}
		p, err := s.resolvePrincipal(ctx, r, kind)
		if err != nil {
			return nil, err
		}
		return next(context.WithValue(ctx, principalKey, p), w, r, request)
	}
}

func (s *Server) resolvePrincipal(ctx context.Context, r *http.Request, kind access) (principal, error) {
	now := s.now()
	var selected *uuid.UUID
	if h := strings.TrimSpace(r.Header.Get(workspaceHeader)); h != "" {
		id, err := uuid.Parse(h)
		if err != nil {
			return principal{}, errValidation("Yuva-Workspace must be a workspace id")
		}
		selected = &id
	}
	if token, ok := bearerToken(r); ok {
		key, err := s.st.GetActiveAPIKeyByHash(ctx, hashSecret(token))
		if store.IsNotFound(err) {
			return principal{}, errUnauthenticated
		}
		if err != nil {
			return principal{}, err
		}
		if kind != accessMemberOrKey {
			return principal{}, errMemberSessionRequired
		}
		if selected != nil && *selected != key.WorkspaceID {
			return principal{}, errWorkspaceMismatch
		}
		if err := s.st.TouchAPIKey(ctx, store.TouchAPIKeyParams{
			Now: now, WorkspaceID: key.WorkspaceID, ID: key.ID, StaleBefore: now.Add(-touchEvery),
		}); err != nil {
			return principal{}, err
		}
		return principal{keyID: key.ID, workspaceID: key.WorkspaceID}, nil
	}
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return principal{}, errUnauthenticated
	}
	sess, err := s.st.GetSessionByTokenHash(ctx, store.GetSessionByTokenHashParams{TokenHash: hashSecret(c.Value), ExpiresAt: now})
	if store.IsNotFound(err) {
		return principal{}, errUnauthenticated
	}
	if err != nil {
		return principal{}, err
	}
	if err := s.st.TouchSession(ctx, store.TouchSessionParams{Now: now, ID: sess.ID, StaleBefore: now.Add(-touchEvery)}); err != nil {
		return principal{}, err
	}
	p := principal{personID: sess.PersonID, sessionID: sess.ID}
	if kind == accessPerson {
		return p, nil
	}
	if selected == nil {
		memberships, err := s.st.ListMemberships(ctx, sess.PersonID)
		if err != nil {
			return principal{}, err
		}
		switch len(memberships) {
		case 0:
			return principal{}, errNotAMember
		case 1:
			selected = &memberships[0].WorkspaceID
		default:
			return principal{}, errWorkspaceRequired
		}
	}
	m, err := s.st.GetMemberByPerson(ctx, store.GetMemberByPersonParams{WorkspaceID: *selected, PersonID: sess.PersonID})
	if store.IsNotFound(err) {
		return principal{}, errNotAMember
	}
	if err != nil {
		return principal{}, err
	}
	p.workspaceID, p.memberID, p.role = m.WorkspaceID, m.ID, m.Role
	return p, nil
}

func principalFrom(ctx context.Context) principal {
	p, _ := ctx.Value(principalKey).(principal)
	return p
}

func requestFrom(ctx context.Context) *http.Request {
	r, _ := ctx.Value(requestKey).(*http.Request)
	return r
}

func requireManager(p principal) error {
	if p.isKey() {
		return errMemberSessionRequired
	}
	if p.role != roleOwner && p.role != roleAdmin {
		return errForbidden
	}
	return nil
}

func requireOwner(p principal) error {
	if p.isKey() {
		return errMemberSessionRequired
	}
	if p.role != roleOwner {
		return errForbidden
	}
	return nil
}

func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	scheme, token, ok := strings.Cut(h, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	token = strings.TrimSpace(token)
	return token, token != ""
}

var (
	errCrossSite       = problem(http.StatusForbidden, "origin_not_allowed", "requests with the session cookie must come from the panel's origin")
	errUnsupportedType = problem(http.StatusUnsupportedMediaType, "unsupported_media_type", "send the body as application/json or multipart/form-data")
)

// guardCookieWrites refuses state-changing /v1 requests that carry the session cookie unless the
// browser says they come from the panel's origin and the body has a type a plain form cannot send.
func (s *Server) guardCookieWrites(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v1/") || r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		if _, bearer := bearerToken(r); bearer {
			next.ServeHTTP(w, r)
			return
		}
		if c, err := r.Cookie(sessionCookie); err != nil || c.Value == "" {
			next.ServeHTTP(w, r)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			if !s.originAllowed(origin) {
				writeProblem(w, errCrossSite)
				return
			}
		} else if r.Header.Get("Sec-Fetch-Site") != "same-origin" {
			writeProblem(w, errCrossSite)
			return
		}
		if r.ContentLength != 0 {
			mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if mt != "application/json" && mt != "multipart/form-data" {
				writeProblem(w, errUnsupportedType)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) trusted(v string) bool {
	a, err := netip.ParseAddr(v)
	if err != nil {
		return false
	}
	a = a.Unmap()
	for _, p := range s.auth.TrustedProxies {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// clientIP is the TCP peer, or with a client IP header the rightmost address in it that is not
// one of the trusted proxies: proxies append to X-Forwarded-For, so everything left of the hop
// our own proxy added is whatever the client sent. With trusted proxies configured, the header
// counts only when the peer is one of them.
func (s *Server) clientIP(r *http.Request) string {
	peer, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		peer = r.RemoteAddr
	}
	if s.auth.ClientIPHeader == "" || (len(s.auth.TrustedProxies) > 0 && !s.trusted(peer)) {
		return peer
	}
	var hops []string
	for _, v := range r.Header.Values(s.auth.ClientIPHeader) {
		for h := range strings.SplitSeq(v, ",") {
			if h = strings.TrimSpace(h); h != "" {
				hops = append(hops, h)
			}
		}
	}
	if len(hops) == 0 {
		return peer
	}
	for i := len(hops) - 1; i > 0; i-- {
		if !s.trusted(hops[i]) {
			return hops[i]
		}
	}
	return hops[0]
}

func randomToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func hashSecret(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}

func (s *Server) sessionCookie(value string, maxAge int) string {
	c := http.Cookie{
		Name:     sessionCookie,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   s.auth.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	}
	return c.String()
}

func (s *Server) startSession(ctx context.Context, q *store.Queries, personID uuid.UUID, method string) (string, error) {
	now := s.now()
	token := randomToken(32)
	if err := q.CreateSession(ctx, store.CreateSessionParams{
		ID:        uuid.New(),
		PersonID:  personID,
		TokenHash: hashSecret(token),
		Method:    method,
		CreatedAt: now,
		ExpiresAt: now.Add(sessionTTL),
	}); err != nil {
		return "", err
	}
	return s.sessionCookie(token, int(sessionTTL/time.Second)), nil
}
