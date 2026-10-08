package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
	"uuid"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
)

const (
	mcpURIPrefix       = "yuva://"
	mcpInboxPrefix     = mcpURIPrefix + "inbox/"
	mcpConvPrefix      = mcpURIPrefix + "conversation/"
	mcpContactPrefix   = mcpURIPrefix + "contact/"
	mcpRecheckInterval = 30 * time.Second
	mcpJSON            = "application/json"
)

type mcpInboxResource struct {
	Inbox             mcpInbox          `json:"inbox"`
	OpenConversations []mcpConversation `json:"open_conversations,omitempty"`
}

func addMCPResources(srv *mcp.Server, c *mcpCall) {
	if keyMayCall(c.p, "GetInbox") == nil {
		srv.AddResourceTemplate(&mcp.ResourceTemplate{
			Name: "inbox", Title: "Inbox", URITemplate: mcpInboxPrefix + "{id}", MIMEType: mcpJSON,
			Description: "An inbox with its language and settings, and its latest open conversations when you may read conversations." + customerContentRule,
		}, c.readResource)
	}
	if keyMayCall(c.p, "GetConversation") == nil && keyMayCall(c.p, "ListMessages") == nil {
		srv.AddResourceTemplate(&mcp.ResourceTemplate{
			Name: "conversation", Title: "Conversation", URITemplate: mcpConvPrefix + "{id}", MIMEType: mcpJSON,
			Description: "A conversation with its latest messages, notes, drafts and timeline events, as get_conversation returns it." + customerContentRule,
		}, c.readResource)
	}
	if keyMayCall(c.p, "GetContact") == nil {
		srv.AddResourceTemplate(&mcp.ResourceTemplate{
			Name: "contact", Title: "Contact", URITemplate: mcpContactPrefix + "{id}", MIMEType: mcpJSON,
			Description: "A contact, as get_contact returns it." + customerContentRule,
		}, c.readResource)
	}
}

func (c *mcpCall) inboxResources(ctx context.Context) []*mcp.Resource {
	if keyMayCall(c.p, "ListInboxes") != nil || keyMayCall(c.p, "GetInbox") != nil {
		return nil
	}
	res, err := c.s.ListInboxes(c.as(ctx), oas.ListInboxesRequestObject{})
	if err != nil {
		c.s.log.WarnContext(ctx, "mcp resources", slog.Any("error", err))
		return nil
	}
	var out []*mcp.Resource
	for _, in := range res.(oas.ListInboxes200JSONResponse).Items {
		out = append(out, &mcp.Resource{
			URI: mcpInboxPrefix + in.Id.String(), Name: "inbox-" + in.Slug, Title: in.Name, MIMEType: mcpJSON,
			Description: "Inbox " + in.Name + " (" + in.DefaultLocale + ")",
		})
	}
	return out
}

func parseResourceURI(uri string) (string, uuid.UUID, bool) {
	for _, prefix := range []string{mcpInboxPrefix, mcpConvPrefix, mcpContactPrefix} {
		if rest, ok := strings.CutPrefix(uri, prefix); ok {
			id, err := uuid.Parse(rest)
			return prefix, id, err == nil
		}
	}
	return "", uuid.UUID{}, false
}

func (c *mcpCall) resource(ctx context.Context, uri string) (any, error) {
	kind, id, ok := parseResourceURI(uri)
	if !ok {
		return nil, mcp.ResourceNotFoundError(uri)
	}
	ctx = c.as(ctx)
	switch kind {
	case mcpInboxPrefix:
		if err := keyMayCall(c.p, "GetInbox"); err != nil {
			return nil, err
		}
		res, err := c.s.GetInbox(ctx, oas.GetInboxRequestObject{InboxId: id})
		if err != nil {
			return nil, err
		}
		out := mcpInboxResource{Inbox: inboxOut(oas.Inbox(res.(oas.GetInbox200JSONResponse)))}
		if keyMayCall(c.p, "ListConversations") == nil {
			open := oas.ConversationStatusOpen
			page, err := c.listConversations(ctx, oas.ListConversationsParams{InboxId: &id, Status: &open})
			if err != nil {
				return nil, err
			}
			out.OpenConversations = page.Items
		}
		return out, nil
	case mcpConvPrefix:
		if err := keyMayCall(c.p, "GetConversation"); err != nil {
			return nil, err
		}
		if err := keyMayCall(c.p, "ListMessages"); err != nil {
			return nil, err
		}
		return c.conversation(ctx, id, "", 0)
	default:
		if err := keyMayCall(c.p, "GetContact"); err != nil {
			return nil, err
		}
		res, err := c.s.GetContact(ctx, oas.GetContactRequestObject{ContactId: id})
		if err != nil {
			return nil, err
		}
		return contactOut(oas.Contact(res.(oas.GetContact200JSONResponse))), nil
	}
}

func (c *mcpCall) resourceError(ctx context.Context, uri string, err error) error {
	var e *apiError
	if errors.As(err, &e) {
		if e.Status == http.StatusNotFound {
			return mcp.ResourceNotFoundError(uri)
		}
		return e
	}
	var je *jsonrpc.Error
	if errors.As(err, &je) {
		return je
	}
	return c.toolError(ctx, err)
}

func (c *mcpCall) readResource(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	uri := req.Params.URI
	v, err := c.resource(ctx, uri)
	if err != nil {
		return nil, c.resourceError(ctx, uri, err)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: uri, MIMEType: mcpJSON, Text: string(b)}}}, nil
}

// mcpWatcher feeds subscriptions/listen from the realtime hub: one hub subscription per request,
// filtered by what the caller may see, and a resources/updated notification per matching event.
type mcpWatcher struct {
	c    *mcpCall
	srv  *mcp.Server
	mu   sync.Mutex
	uris map[string]bool
	once sync.Once
}

func (w *mcpWatcher) subscribe(ctx context.Context, req *mcp.SubscribeRequest) error {
	uri := req.Params.URI
	if _, err := w.c.resource(ctx, uri); err != nil {
		return w.c.resourceError(ctx, uri, err)
	}
	w.mu.Lock()
	w.uris[uri] = true
	w.mu.Unlock()
	w.once.Do(func() { go w.run(w.c.r.Context()) })
	return nil
}

func (w *mcpWatcher) unsubscribe(_ context.Context, req *mcp.UnsubscribeRequest) error {
	w.mu.Lock()
	delete(w.uris, req.Params.URI)
	w.mu.Unlock()
	return nil
}

func (w *mcpWatcher) run(ctx context.Context) {
	s, p := w.c.s, w.c.p
	sub := s.hub.Subscribe(p.workspaceID)
	defer s.hub.Unsubscribe(sub)
	f := &eventFilter{p: p}
	if err := f.reload(ctx, s.st.Queries); err != nil {
		s.log.WarnContext(ctx, "mcp subscription", slog.Any("error", err))
		w.c.cancel()
		return
	}
	tick := time.NewTicker(mcpRecheckInterval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-sub.Done():
			w.c.cancel()
			return
		case <-tick.C:
			next, err := s.resolveBearer(ctx, w.c.token, accessMemberOrKey, nil, true)
			if err != nil {
				w.c.cancel()
				return
			}
			f.p = next
			if err := f.reload(ctx, s.st.Queries); err != nil {
				s.log.WarnContext(ctx, "mcp subscription", slog.Any("error", err))
			}
		case e := <-sub.Events():
			if e.Ephemeral() {
				continue
			}
			ok, err := f.allows(ctx, s.st.Queries, e)
			if err != nil {
				s.log.WarnContext(ctx, "mcp subscription", slog.Any("error", err))
				continue
			}
			if !ok {
				continue
			}
			for _, uri := range w.matching(e) {
				_ = w.srv.ResourceUpdated(ctx, &mcp.ResourceUpdatedNotificationParams{URI: uri})
			}
		}
	}
}

func (w *mcpWatcher) matching(e realtime.Event) []string {
	var uris []string
	if e.ConversationID != nil {
		uris = append(uris, mcpConvPrefix+e.ConversationID.String())
	}
	if e.InboxID != nil {
		switch e.Type {
		case realtime.InboxUpdated, realtime.InboxDeleted, realtime.ConversationCreated, realtime.ConversationUpdated, realtime.ConversationMoved:
			uris = append(uris, mcpInboxPrefix+e.InboxID.String())
		}
	}
	if e.Type == realtime.ContactUpdated || e.Type == realtime.ContactDeleted {
		var ct struct {
			ID uuid.UUID `json:"id"`
		}
		if json.Unmarshal(e.Data, &ct) == nil {
			uris = append(uris, mcpContactPrefix+ct.ID.String())
		}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	out := uris[:0]
	for _, u := range uris {
		if w.uris[u] {
			out = append(out, u)
		}
	}
	return out
}
