package api

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const maxChannelAttachmentTypes = 100

type attachLimits struct {
	maxBytes     int64
	types        []string
	channelBytes bool
	channelTypes bool
}

func (l attachLimits) allows(ct string) bool {
	return typeIn(l.types, ct)
}

func (l attachLimits) tooLarge() error {
	if l.channelBytes {
		return errChannelAttachmentTooLarge
	}
	return errAttachmentTooLarge
}

func (l attachLimits) typeRefused() error {
	if l.channelTypes {
		return errChannelAttachmentType
	}
	return errAttachmentType
}

func typeIn(list []string, ct string) bool {
	for _, t := range list {
		if t == ct || (strings.HasSuffix(t, "/*") && strings.HasPrefix(ct, strings.TrimSuffix(t, "*"))) {
			return true
		}
	}
	return false
}

func intersectTypes(server, channel []string) []string {
	out := []string{}
	add := func(t string) {
		if !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	for _, c := range channel {
		if typeIn(server, c) {
			add(c)
			continue
		}
		if strings.HasSuffix(c, "/*") {
			for _, t := range server {
				if typeIn([]string{c}, t) {
					add(t)
				}
			}
		}
	}
	return out
}

func (s *Server) serverLimits() attachLimits {
	return attachLimits{maxBytes: s.attach.MaxBytes, types: s.attach.Types}
}

func (s *Server) effectiveLimits(maxBytes *int64, types []string) attachLimits {
	l := s.serverLimits()
	if maxBytes != nil && *maxBytes < l.maxBytes {
		l.maxBytes, l.channelBytes = *maxBytes, true
	}
	if types != nil {
		l.types, l.channelTypes = intersectTypes(s.attach.Types, types), true
	}
	return l
}

// channelLimits answers the server's limits for a conversation without a channel, and accepts no
// file when the channel is gone, since its narrower limits can no longer be read.
func (s *Server) channelLimits(ctx context.Context, q *store.Queries, workspaceID uuid.UUID, channelID *uuid.UUID) (attachLimits, error) {
	if channelID == nil {
		return s.serverLimits(), nil
	}
	r, err := q.GetChannelAttachmentLimits(ctx, store.GetChannelAttachmentLimitsParams{WorkspaceID: workspaceID, ID: *channelID})
	if store.IsNotFound(err) {
		return attachLimits{types: []string{}, channelBytes: true, channelTypes: true}, nil
	}
	if err != nil {
		return attachLimits{}, err
	}
	return s.effectiveLimits(r.AttachmentMaxBytes, r.AttachmentTypes), nil
}

func (s *Server) channelAttachmentsBody(c store.Channel) oas.ChannelAttachments {
	l := s.effectiveLimits(c.AttachmentMaxBytes, c.AttachmentTypes)
	out := oas.ChannelAttachments{MaxBytes: c.AttachmentMaxBytes, EffectiveMaxBytes: l.maxBytes, EffectiveTypes: slices.Clone(l.types)}
	if out.EffectiveTypes == nil {
		out.EffectiveTypes = []string{}
	}
	if c.AttachmentTypes != nil {
		types := slices.Clone(c.AttachmentTypes)
		out.Types = &types
	}
	return out
}

func clientAttachmentLimits(l attachLimits) oas.ClientAttachmentLimits {
	types := slices.Clone(l.types)
	if types == nil {
		types = []string{}
	}
	return oas.ClientAttachmentLimits{MaxBytes: l.maxBytes, Types: types}
}

type channelAttachmentsChange struct {
	setMaxBytes bool
	maxBytes    *int64
	setTypes    bool
	types       []string
}

// channelAttachmentsInput reads a create or update body, refusing a size above the server's and a
// type that allows nothing the server allows.
func (s *Server) channelAttachmentsInput(in *oas.ChannelAttachmentsInput) (channelAttachmentsChange, error) {
	var out channelAttachmentsChange
	if in == nil {
		return out, nil
	}
	if in.MaxBytes.IsSpecified() {
		out.setMaxBytes = true
		if !in.MaxBytes.IsNull() {
			v := in.MaxBytes.MustGet()
			if v < 1 {
				return out, errValidation("attachments.max_bytes must be at least 1")
			}
			if v > s.attach.MaxBytes {
				return out, errValidation(fmt.Sprintf("attachments.max_bytes must be at most %d, the server's limit", s.attach.MaxBytes))
			}
			out.maxBytes = &v
		}
	}
	if in.Types.IsSpecified() {
		out.setTypes = true
		if !in.Types.IsNull() {
			list := in.Types.MustGet()
			if len(list) == 0 || len(list) > maxChannelAttachmentTypes {
				return out, errValidation("attachments.types must have 1 to 100 entries")
			}
			out.types = []string{}
			for _, raw := range list {
				t := strings.ToLower(strings.TrimSpace(raw))
				if !validTypePattern(t) {
					return out, errValidation("attachments.types: " + raw + " is not a content type such as application/pdf or image/*")
				}
				if len(intersectTypes(s.attach.Types, []string{t})) == 0 {
					return out, errValidation("attachments.types: " + t + " allows nothing the server allows")
				}
				if !slices.Contains(out.types, t) {
					out.types = append(out.types, t)
				}
			}
		}
	}
	return out, nil
}

func validTypePattern(t string) bool {
	if len(t) < 3 || len(t) > 255 {
		return false
	}
	major, minor, ok := strings.Cut(t, "/")
	if !ok || major == "" || minor == "" || strings.Contains(minor, "/") || strings.Contains(major, "*") {
		return false
	}
	if strings.Contains(minor, "*") && minor != "*" {
		return false
	}
	for _, r := range major + minor {
		if r <= ' ' || r > '~' || strings.ContainsRune(`()<>@,;:\"[]?=`, r) {
			return false
		}
	}
	return true
}
