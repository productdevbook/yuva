package api

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

// cleanClientInfo trims and checks what a client sent; nil when nothing is left.
func cleanClientInfo(in *oas.ClientInfo) ([]byte, error) {
	if in == nil {
		return nil, nil
	}
	out := oas.ClientInfo{}
	set := false
	for _, f := range []struct {
		name string
		max  int
		src  *string
		dst  **string
	}{
		{"client_info.app_version", 50, in.AppVersion, &out.AppVersion},
		{"client_info.build", 50, in.Build, &out.Build},
		{"client_info.sdk_name", 50, in.SdkName, &out.SdkName},
		{"client_info.sdk_version", 50, in.SdkVersion, &out.SdkVersion},
		{"client_info.os", 50, in.Os, &out.Os},
		{"client_info.os_version", 50, in.OsVersion, &out.OsVersion},
		{"client_info.device_manufacturer", 100, in.DeviceManufacturer, &out.DeviceManufacturer},
		{"client_info.device_model", 100, in.DeviceModel, &out.DeviceModel},
		{"client_info.app_locale", 35, in.AppLocale, &out.AppLocale},
		{"client_info.device_locale", 35, in.DeviceLocale, &out.DeviceLocale},
		{"client_info.timezone", 64, in.Timezone, &out.Timezone},
		{"client_info.screen", 50, in.Screen, &out.Screen},
	} {
		if f.src == nil {
			continue
		}
		if strings.ContainsRune(*f.src, 0) {
			return nil, errValidation(f.name + " must not contain NUL characters")
		}
		v, err := trimmed(*f.src, 0, f.max, f.name)
		if err != nil {
			return nil, err
		}
		if v != "" {
			*f.dst, set = &v, true
		}
	}
	if in.Platform != nil && *in.Platform != "" {
		switch *in.Platform {
		case oas.ClientPlatformIos, oas.ClientPlatformAndroid, oas.ClientPlatformWeb:
			out.Platform, set = in.Platform, true
		default:
			return nil, errValidation("client_info.platform must be ios, android or web")
		}
	}
	if in.FreeStorageMb != nil {
		if *in.FreeStorageMb < 0 {
			return nil, errValidation("client_info.free_storage_mb must be at least 0")
		}
		out.FreeStorageMb, set = in.FreeStorageMb, true
	}
	if !set {
		return nil, nil
	}
	return mustJSON(out), nil
}

func parseClientInfoField(v string) ([]byte, error) {
	var in oas.ClientInfo
	if err := json.Unmarshal([]byte(v), &in); err != nil {
		return nil, errValidation("client_info must be a JSON object of app and device details")
	}
	return cleanClientInfo(&in)
}

func decodeClientInfo(raw []byte, at *time.Time) (*oas.ClientInfo, *time.Time) {
	if raw == nil {
		return nil, nil
	}
	var out oas.ClientInfo
	if json.Unmarshal(raw, &out) != nil {
		return nil, nil
	}
	return &out, at
}

// useClientInfo makes a request's own client_info the session's from now on.
func (s *Server) useClientInfo(ctx context.Context, cp *contactPrincipal, info []byte) error {
	if info == nil {
		return nil
	}
	if err := s.st.SetContactSessionClientInfo(ctx, store.SetContactSessionClientInfoParams{WorkspaceID: cp.workspaceID, ID: cp.sessionID, ClientInfo: info}); err != nil {
		return err
	}
	cp.clientInfo, cp.ownInfo = info, true
	return nil
}

// writeClientInfo keeps the session's client_info on the conversation and, when newer, on its
// contact. A conversation that existed before hears conversation.updated when the value changed.
// Without a value of its own the request reads the session's now, since another request may have
// refreshed it after this one was authenticated.
func (s *Server) writeClientInfo(ctx context.Context, q *store.Queries, events *eventBatch, cp contactPrincipal, c store.Conversation, isNew bool) (store.Conversation, error) {
	info := cp.clientInfo
	if !cp.ownInfo {
		var err error
		info, err = q.GetContactSessionClientInfo(ctx, store.GetContactSessionClientInfoParams{WorkspaceID: cp.workspaceID, ID: cp.sessionID})
		if store.IsNotFound(err) {
			return c, nil
		}
		if err != nil {
			return c, err
		}
	}
	if info == nil {
		return c, nil
	}
	updated, err := q.SetConversationClientInfo(ctx, store.SetConversationClientInfoParams{
		WorkspaceID: cp.workspaceID, ID: c.ID, ClientInfo: info, Now: s.now(),
	})
	if store.IsNotFound(err) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	u := store.Conversation(updated)
	if isNew || bytes.Equal(c.ClientInfo, u.ClientInfo) {
		return u, nil
	}
	body, err := oneConversation(ctx, q, u)
	if err != nil {
		return u, err
	}
	events.conversation(realtime.ConversationUpdated, u, body)
	return u, nil
}
