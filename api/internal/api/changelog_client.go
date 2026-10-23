package api

import (
	"context"
	"net/http"
	"strings"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	clientWhatsNewDefaultLimit = 20
	clientWhatsNewMaxLimit     = 50
)

var (
	limitWhatsNewPerIP        = limit{120, time.Minute}
	limitWhatsNewPerChangelog = limit{2400, time.Minute}
)

type (
	clientWhatsNewAnswer      struct{ helpJSON }
	clientWhatsNewEntryAnswer struct{ helpJSON }
)

func (a clientWhatsNewAnswer) VisitGetClientWhatsNewResponse(w http.ResponseWriter) error {
	return a.write(w)
}

func (a clientWhatsNewEntryAnswer) VisitGetClientWhatsNewEntryResponse(w http.ResponseWriter) error {
	return a.write(w)
}

func (s *Server) whatsNewClientChangelog(ctx context.Context, key string) (store.Changelog, error) {
	var none store.Changelog
	if err := s.whatsNewOn(); err != nil {
		return none, err
	}
	key = strings.TrimSpace(key)
	if key == "" || len(key) > 200 {
		return none, errUnknownChannelKey
	}
	ip := rateIP(s.clientIP(requestFrom(ctx)))
	if err := s.rateLimit(rateCheck{"whats-new-ip:" + ip, limitChannelPerIP}); err != nil {
		return none, err
	}
	ch, err := s.st.FindChatChannelByKey(ctx, key)
	if store.IsNotFound(err) {
		return none, errUnknownChannelKey
	}
	if err != nil {
		return none, err
	}
	if !s.originAllowedFor(ctx, ch.WorkspaceID, originFrom(ctx), ch.Kind, ch.AllowedOrigins) {
		return none, errOriginRefused
	}
	c, err := s.st.GetInboxChangelog(ctx, store.GetInboxChangelogParams{WorkspaceID: ch.WorkspaceID, InboxID: ch.InboxID})
	if store.IsNotFound(err) || (err == nil && !c.Published) {
		return none, errWhatsNewNoChangelog
	}
	if err != nil {
		return none, err
	}
	err = s.rateLimit(
		rateCheck{"whats-new:" + ip + ":" + ch.ChannelID.String(), limitWhatsNewPerIP},
		rateCheck{"whats-new:changelog:" + c.ID.String(), limitWhatsNewPerChangelog},
	)
	return c, err
}

func (s *Server) clientWhatsNewInbox(ctx context.Context, q *store.Queries, in store.Inbox) (*oas.ClientInboxWhatsNew, error) {
	if s.changelog.Off {
		return nil, nil
	}
	c, err := q.GetInboxChangelog(ctx, store.GetInboxChangelogParams{WorkspaceID: in.WorkspaceID, InboxID: in.ID})
	if store.IsNotFound(err) || (err == nil && !c.Published) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	latest, err := q.LatestChangelogPublishedAt(ctx, store.LatestChangelogPublishedAtParams{WorkspaceID: c.WorkspaceID, ChangelogID: c.ID})
	if store.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &oas.ClientInboxWhatsNew{Url: s.changelogURL(c, ""), Locales: c.Locales, DefaultLocale: c.DefaultLocale, LatestPublishedAt: latest}, nil
}

type publicChangelogEntry struct {
	ID          uuid.UUID
	Number      int32
	Labels      []string
	PublishedAt time.Time
	Locale      string
	Title       string
	Slug        string
	Excerpt     string
	Html        string
	ImageToken  *string
	ImageWidth  *int32
	ImageHeight *int32
}

func (s *Server) clientWhatsNewEntry(c store.Changelog, served string, r publicChangelogEntry) oas.ClientWhatsNewEntry {
	out := oas.ClientWhatsNewEntry{
		Id: r.ID, Number: r.Number, Locale: r.Locale, Fallback: r.Locale != served, Title: r.Title, Excerpt: r.Excerpt, Html: r.Html,
		Labels: changelogLabels(r.Labels), PublishedAt: r.PublishedAt, Url: s.changelogEntryURL(c, r.Number, r.Locale, r.Slug),
	}
	if r.ImageToken != nil && r.ImageWidth != nil && r.ImageHeight != nil {
		out.Image = &oas.ClientWhatsNewImage{Url: s.imageURL(*r.ImageToken), Width: *r.ImageWidth, Height: *r.ImageHeight}
	}
	return out
}

func (s *Server) GetClientWhatsNew(ctx context.Context, req oas.GetClientWhatsNewRequestObject) (oas.GetClientWhatsNewResponseObject, error) {
	prm := req.Params
	lim := int32(clientWhatsNewDefaultLimit)
	if prm.Limit != nil {
		if *prm.Limit < 1 || *prm.Limit > clientWhatsNewMaxLimit {
			return nil, errValidation("limit must be 1 to 50")
		}
		lim = *prm.Limit
	}
	at, id, err := decodeCursor(prm.Cursor)
	if err != nil {
		return nil, err
	}
	c, err := s.whatsNewClientChangelog(ctx, req.ChannelKey)
	if err != nil {
		return nil, err
	}
	q := s.st.Queries
	locale := servedLocale(c.Locales, c.DefaultLocale, deref(prm.Locale))
	rows, err := q.ListPublicChangelogEntries(ctx, store.ListPublicChangelogEntriesParams{
		WorkspaceID: c.WorkspaceID, ChangelogID: c.ID, Locale: locale, DefaultLocale: c.DefaultLocale, CursorAt: at, CursorID: id, Lim: lim + 1,
	})
	if err != nil {
		return nil, err
	}
	out := oas.ClientWhatsNew{
		Changelog: oas.ClientChangelog{
			Name: c.Name, Description: c.Description, Locale: locale, Locales: c.Locales, DefaultLocale: c.DefaultLocale,
			Url: s.changelogURL(c, "/"+locale+"/"),
		},
		Items: []oas.ClientWhatsNewEntry{},
	}
	if len(rows) > int(lim) {
		rows = rows[:lim]
		last := rows[len(rows)-1]
		out.NextCursor = new(encodeCursor(last.PublishedAt, last.ID))
	}
	for _, r := range rows {
		out.Items = append(out.Items, s.clientWhatsNewEntry(c, locale, publicChangelogEntry(r)))
	}
	latest, err := q.LatestChangelogPublishedAt(ctx, store.LatestChangelogPublishedAtParams{WorkspaceID: c.WorkspaceID, ChangelogID: c.ID})
	if err != nil && !store.IsNotFound(err) {
		return nil, err
	}
	if err == nil {
		out.Changelog.LatestPublishedAt = &latest
	}
	return clientWhatsNewAnswer{helpJSON{out}}, nil
}

func (s *Server) GetClientWhatsNewEntry(ctx context.Context, req oas.GetClientWhatsNewEntryRequestObject) (oas.GetClientWhatsNewEntryResponseObject, error) {
	c, err := s.whatsNewClientChangelog(ctx, req.ChannelKey)
	if err != nil {
		return nil, err
	}
	locale := servedLocale(c.Locales, c.DefaultLocale, deref(req.Params.Locale))
	r, err := s.st.GetPublicChangelogEntry(ctx, store.GetPublicChangelogEntryParams{
		WorkspaceID: c.WorkspaceID, ChangelogID: c.ID, ID: req.EntryId, Locale: locale, DefaultLocale: c.DefaultLocale,
	})
	if store.IsNotFound(err) {
		return nil, errChangelogEntryGone
	}
	if err != nil {
		return nil, err
	}
	return clientWhatsNewEntryAnswer{helpJSON{s.clientWhatsNewEntry(c, locale, publicChangelogEntry(r))}}, nil
}
