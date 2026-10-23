package api

import (
	"context"
	"strings"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

func (s *Server) ListSessions(ctx context.Context, _ oas.ListSessionsRequestObject) (oas.ListSessionsResponseObject, error) {
	p := principalFrom(ctx)
	rows, err := s.st.ListSessions(ctx, store.ListSessionsParams{PersonID: p.personID, Now: s.now()})
	if err != nil {
		return nil, err
	}
	out := oas.ListSessions200JSONResponse{Items: make([]oas.Session, 0, len(rows))}
	for _, r := range rows {
		item := oas.Session{
			Id: r.ID, Method: oas.SessionMethod(r.Method), Current: r.ID == p.sessionID, UserAgent: r.UserAgent,
			CreatedAt: r.CreatedAt, LastSeenAt: r.LastSeenAt, ExpiresAt: r.ExpiresAt,
		}
		if r.UserAgent != nil {
			item.Device = optional(deviceName(*r.UserAgent))
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}

func (s *Server) DeleteOtherSessions(ctx context.Context, _ oas.DeleteOtherSessionsRequestObject) (oas.DeleteOtherSessionsResponseObject, error) {
	p := principalFrom(ctx)
	n, err := s.st.DeleteOtherSessions(ctx, store.DeleteOtherSessionsParams{PersonID: p.personID, Keep: p.sessionID})
	if err != nil {
		return nil, err
	}
	return oas.DeleteOtherSessions200JSONResponse{Revoked: int(n)}, nil
}

// deviceName reads a browser and system from a user agent, such as "Firefox on Linux".
func deviceName(ua string) string {
	has := func(parts ...string) bool {
		for _, p := range parts {
			if strings.Contains(ua, p) {
				return true
			}
		}
		return false
	}
	var browser, system string
	switch {
	case has("Edg/", "EdgA/", "EdgiOS/"):
		browser = "Edge"
	case has("OPR/"):
		browser = "Opera"
	case has("Firefox/", "FxiOS/"):
		browser = "Firefox"
	case has("Chrome/", "CriOS/"):
		browser = "Chrome"
	case has("Safari/"):
		browser = "Safari"
	}
	switch {
	case has("iPhone", "iPad", "iPod"):
		system = "iOS"
	case has("Android"):
		system = "Android"
	case has("Windows"):
		system = "Windows"
	case has("CrOS"):
		system = "ChromeOS"
	case has("Macintosh", "Mac OS X"):
		system = "macOS"
	case has("Linux"):
		system = "Linux"
	}
	switch {
	case browser != "" && system != "":
		return browser + " on " + system
	case browser != "":
		return browser
	}
	return system
}
