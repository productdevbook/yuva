package api

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	chatKeyPrefix     = "yuva_pk_"
	maxAllowedOrigins = 20
)

var (
	errChatRequired   = errValidation("a chat channel needs chat settings with at least one allowed origin")
	errChatNotAllowed = errValidation("chat settings are only for chat channels")
	errAppNotAllowed  = errValidation("app settings are only for app channels")
	errNotChat        = errValidation("only chat and app channels have a public key")
)

var allPlatforms = []string{string(oas.Ios), string(oas.Android)}

func hasPublicKey(kind string) bool {
	return kind == string(oas.ChannelKindChat) || kind == string(oas.ChannelKindApp)
}

func newChatKey() string { return chatKeyPrefix + randomToken(18) }

// normalizeOrigin returns the origin as browsers send it: lowercase scheme and host, no default
// port, no path.
func normalizeOrigin(v string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(v))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return "", false
	}
	scheme, host, port := strings.ToLower(u.Scheme), strings.ToLower(u.Hostname()), u.Port()
	if (scheme == "https" && port == "443") || (scheme == "http" && port == "80") {
		port = ""
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	out := scheme + "://" + host
	if port != "" {
		out += ":" + port
	}
	return out, len(out) <= 300
}

func chatChannelBody(c store.ChatChannel) *oas.ChatChannel {
	out := &oas.ChatChannel{
		PublicKey: c.PublicKey, AllowAnonymous: c.AllowAnonymous, AskEmailOffline: c.AskEmailOffline,
		Greeting: c.Greeting, AllowedOrigins: make([]oas.Origin, len(c.AllowedOrigins)),
		Launcher: oas.ChatLauncher{Color: c.LauncherColor},
	}
	for i, o := range c.AllowedOrigins {
		out.AllowedOrigins[i] = oas.Origin(o)
	}
	if c.LauncherPosition != nil {
		p := oas.ChatLauncherPosition(*c.LauncherPosition)
		out.Launcher.Position = &p
	}
	return out
}

func appChannelBody(c store.ChatChannel) *oas.AppChannel {
	out := &oas.AppChannel{PublicKey: c.PublicKey, AllowAnonymous: c.AllowAnonymous, Platforms: make([]oas.AppPlatform, len(c.Platforms))}
	for i, p := range c.Platforms {
		out.Platforms[i] = oas.AppPlatform(p)
	}
	return out
}

func appChannelParams(workspaceID, channelID uuid.UUID, in *oas.AppChannelInput, cur *store.ChatChannel) (store.SaveChatChannelParams, error) {
	out := store.SaveChatChannelParams{
		WorkspaceID: workspaceID, ChannelID: channelID, AskEmailOffline: true, AllowedOrigins: []string{}, Platforms: allPlatforms,
	}
	if cur != nil {
		out.PublicKey = cur.PublicKey
	} else {
		out.PublicKey = newChatKey()
	}
	if in == nil {
		return out, nil
	}
	if in.AllowAnonymous != nil {
		out.AllowAnonymous = *in.AllowAnonymous
	}
	if in.Platforms != nil {
		if len(*in.Platforms) == 0 {
			return out, errValidation("app.platforms must name ios, android or both")
		}
		out.Platforms = nil
		for _, p := range allPlatforms {
			for _, q := range *in.Platforms {
				if !q.Valid() {
					return out, errValidation("app.platforms must name ios, android or both")
				}
				if string(q) == p {
					out.Platforms = append(out.Platforms, p)
					break
				}
			}
		}
	}
	return out, nil
}

func chatChannelParams(workspaceID, channelID uuid.UUID, in *oas.ChatChannelInput, cur *store.ChatChannel) (store.SaveChatChannelParams, error) {
	out := store.SaveChatChannelParams{WorkspaceID: workspaceID, ChannelID: channelID, AskEmailOffline: true, Platforms: []string{}}
	if cur != nil {
		out.PublicKey = cur.PublicKey
	} else {
		out.PublicKey = newChatKey()
	}
	if len(in.AllowedOrigins) == 0 || len(in.AllowedOrigins) > maxAllowedOrigins {
		return out, errValidation("chat.allowed_origins must list 1 to 20 origins")
	}
	seen := map[string]bool{}
	for _, o := range in.AllowedOrigins {
		n, ok := normalizeOrigin(string(o))
		if !ok {
			return out, errValidation("chat.allowed_origins must be origins such as https://www.example.com, without a path")
		}
		if !seen[n] {
			seen[n] = true
			out.AllowedOrigins = append(out.AllowedOrigins, n)
		}
	}
	if in.AllowAnonymous != nil {
		out.AllowAnonymous = *in.AllowAnonymous
	}
	if in.AskEmailOffline != nil {
		out.AskEmailOffline = *in.AskEmailOffline
	}
	if in.Greeting != nil {
		g, err := trimmed(*in.Greeting, 0, 500, "chat.greeting")
		if err != nil {
			return out, err
		}
		out.Greeting = g
	}
	if l := in.Launcher; l != nil {
		if l.Position != nil {
			if !l.Position.Valid() {
				return out, errValidation("chat.launcher.position must be right or left")
			}
			p := string(*l.Position)
			out.LauncherPosition = &p
		}
		if l.Color != nil {
			if !colorPattern.MatchString(*l.Color) {
				return out, errValidation("chat.launcher.color must be #rrggbb")
			}
			c := strings.ToLower(*l.Color)
			out.LauncherColor = &c
		}
	}
	return out, nil
}

func chatChannelsByID(ctx context.Context, q *store.Queries, workspaceID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]store.ChatChannel, error) {
	rows, err := q.ListChatChannels(ctx, store.ListChatChannelsParams{WorkspaceID: workspaceID, ChannelIds: ids})
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]store.ChatChannel, len(rows))
	for _, r := range rows {
		out[r.ChannelID] = r
	}
	return out, nil
}

func (s *Server) RotateChannelPublicKey(ctx context.Context, req oas.RotateChannelPublicKeyRequestObject) (oas.RotateChannelPublicKeyResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManager(p); err != nil {
		return nil, err
	}
	c, err := s.visibleChannel(ctx, p, req.ChannelId)
	if err != nil {
		return nil, err
	}
	if !hasPublicKey(c.Kind) {
		return nil, errNotChat
	}
	chat, err := s.st.SetChatChannelKey(ctx, store.SetChatChannelKeyParams{WorkspaceID: p.workspaceID, ChannelID: c.ID, PublicKey: newChatKey()})
	if store.IsNotFound(err) {
		return nil, errNotChat
	}
	if err != nil {
		return nil, err
	}
	return oas.RotateChannelPublicKey200JSONResponse(channelBody(c, nil, &chat)), nil
}

var errOriginRefused = problem(http.StatusForbidden, "origin_not_allowed", "this origin may not use the channel")
