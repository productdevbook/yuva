package api

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	contactSessionTTL    = 7 * 24 * time.Hour
	contactTokenPrefix   = "yuva_cs_"
	clientPrefix         = "/client/v1/"
	wsProtocol           = "yuva"
	wsTokenProtocolStart = "yuva.token."
)

var (
	errContactUnauthenticated = problem(http.StatusUnauthorized, "unauthenticated", "start a contact session and send its token")
	errContactBlocked         = problem(http.StatusForbidden, "contact_blocked", "this contact may not chat")
)

type contactPrincipal struct {
	workspaceID uuid.UUID
	inboxID     uuid.UUID
	channelID   uuid.UUID
	contactID   uuid.UUID
	sessionID   uuid.UUID
	identified  bool
	expiresAt   time.Time
	chat        store.ChatChannel
}

const (
	contactKey ctxKey = iota + 100
	originKey
)

func contactFrom(ctx context.Context) contactPrincipal {
	c, _ := ctx.Value(contactKey).(contactPrincipal)
	return c
}

func originFrom(ctx context.Context) string {
	o, _ := ctx.Value(originKey).(string)
	return o
}

// clientCORS answers preflights for /client/v1 and refuses browsers from origins that no chat
// channel allows. Whether the origin is allowed for the channel in question is checked once the
// channel is known (from the public key or the session).
func (s *Server) clientCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, clientPrefix) {
			next.ServeHTTP(w, r)
			return
		}
		raw := r.Header.Get("Origin")
		if raw == "" {
			next.ServeHTTP(w, r)
			return
		}
		origin, ok := normalizeOrigin(raw)
		known := false
		if ok {
			var err error
			if known, err = s.st.AnyChatChannelAllowsOrigin(r.Context(), origin); err != nil {
				s.log.ErrorContext(r.Context(), "origin check", "error", err)
				writeProblem(w, errInternal)
				return
			}
		}
		w.Header().Add("Vary", "Origin")
		if !known {
			writeProblem(w, errOriginRefused)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", raw)
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), originKey, origin)))
	})
}

func originAllowedFor(origin string, allowed []string) bool {
	return origin == "" || slices.Contains(allowed, origin)
}

func (s *Server) resolveContact(ctx context.Context, token string) (contactPrincipal, error) {
	if token == "" {
		return contactPrincipal{}, errContactUnauthenticated
	}
	now := s.now()
	sess, err := s.st.GetContactSessionByTokenHash(ctx, store.GetContactSessionByTokenHashParams{TokenHash: hashSecret(token), ExpiresAt: now})
	if store.IsNotFound(err) {
		return contactPrincipal{}, errContactUnauthenticated
	}
	if err != nil {
		return contactPrincipal{}, err
	}
	chat, err := s.st.GetChatChannel(ctx, store.GetChatChannelParams{WorkspaceID: sess.WorkspaceID, ChannelID: sess.ChannelID})
	if store.IsNotFound(err) {
		return contactPrincipal{}, errContactUnauthenticated
	}
	if err != nil {
		return contactPrincipal{}, err
	}
	if !originAllowedFor(originFrom(ctx), chat.AllowedOrigins) {
		return contactPrincipal{}, errOriginRefused
	}
	if sess.ContactBlocked {
		return contactPrincipal{}, errContactBlocked
	}
	expires := sess.ExpiresAt
	if sess.LastSeenAt.Before(now.Add(-touchEvery)) {
		expires = now.Add(contactSessionTTL)
		if err := s.st.TouchContactSession(ctx, store.TouchContactSessionParams{
			Now: now, ExpiresAt: expires, WorkspaceID: sess.WorkspaceID, ID: sess.ID, StaleBefore: now.Add(-touchEvery),
		}); err != nil {
			return contactPrincipal{}, err
		}
	}
	return contactPrincipal{
		workspaceID: sess.WorkspaceID, inboxID: sess.InboxID, channelID: sess.ChannelID, contactID: sess.ContactID,
		sessionID: sess.ID, identified: sess.Identified, expiresAt: expires, chat: chat,
	}, nil
}

// websocketToken reads the session token from the `token` query parameter or from a
// `yuva.token.<token>` subprotocol, since browsers cannot set headers on a WebSocket.
func websocketToken(r *http.Request) string {
	if t := r.URL.Query().Get("token"); t != "" {
		return t
	}
	for _, h := range r.Header.Values("Sec-WebSocket-Protocol") {
		for _, p := range strings.Split(h, ",") {
			if t, ok := strings.CutPrefix(strings.TrimSpace(p), wsTokenProtocolStart); ok {
				return t
			}
		}
	}
	return ""
}
