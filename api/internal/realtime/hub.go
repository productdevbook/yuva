package realtime

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/store"
)

const Channel = "yuva_events"

// SignalChannel carries short-lived notices (typing, presence) as the NOTIFY payload itself:
// they are not stored, have no id and are never replayed.
const SignalChannel = "yuva_signals"

const (
	ConversationCreated = "conversation.created"
	ConversationUpdated = "conversation.updated"
	ConversationMoved   = "conversation.moved"
	MessageCreated      = "message.created"
	MessageUpdated      = "message.updated"
	DraftCreated        = "draft.created"
	DraftUpdated        = "draft.updated"
	DraftDeleted        = "draft.deleted"
	ContactUpdated      = "contact.updated"
	ContactDeleted      = "contact.deleted"
	InboxCreated        = "inbox.created"
	InboxUpdated        = "inbox.updated"
	InboxDeleted        = "inbox.deleted"
	InboxAccessChanged  = "inbox_access.changed"
	ConversationRead    = "conversation.read"
	ContactRead         = "contact.read"
	ConversationPin     = "conversation.pin"
	Typing              = "typing"
	PresenceHint        = "presence.hint"
	MemberPresence      = "member.presence"
	ContactPresence     = "contact.presence"
	Viewing             = "viewing"
	ChannelUpdated      = "channel.updated"
)

type Event struct {
	ID             int64           `json:"id,omitempty"`
	Type           string          `json:"type"`
	WorkspaceID    uuid.UUID       `json:"workspace_id"`
	InboxID        *uuid.UUID      `json:"inbox_id,omitempty"`
	ConversationID *uuid.UUID      `json:"conversation_id,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	Data           json.RawMessage `json:"data"`
}

func FromRow(e store.Event) Event {
	return Event{
		ID: e.ID, Type: e.Type, WorkspaceID: e.WorkspaceID, InboxID: e.InboxID, ConversationID: e.ConversationID,
		CreatedAt: e.CreatedAt, Data: e.Payload,
	}
}

func (e Event) Ephemeral() bool { return e.ID == 0 }

type signal struct {
	Type           string          `json:"t"`
	WorkspaceID    uuid.UUID       `json:"w"`
	InboxID        *uuid.UUID      `json:"i,omitempty"`
	ConversationID *uuid.UUID      `json:"c,omitempty"`
	CreatedAt      time.Time       `json:"at"`
	Data           json.RawMessage `json:"d,omitempty"`
}

func SignalPayload(e Event) (string, error) {
	b, err := json.Marshal(signal{Type: e.Type, WorkspaceID: e.WorkspaceID, InboxID: e.InboxID, ConversationID: e.ConversationID, CreatedAt: e.CreatedAt, Data: e.Data})
	return string(b), err
}

func parseSignal(p string) (Event, error) {
	var s signal
	if err := json.Unmarshal([]byte(p), &s); err != nil {
		return Event{}, err
	}
	return Event{Type: s.Type, WorkspaceID: s.WorkspaceID, InboxID: s.InboxID, ConversationID: s.ConversationID, CreatedAt: s.CreatedAt, Data: s.Data}, nil
}

type Subscription struct {
	workspaceID uuid.UUID
	events      chan Event
	done        chan struct{}
	once        sync.Once
	reason      string
}

func (s *Subscription) Events() <-chan Event { return s.events }

func (s *Subscription) Done() <-chan struct{} { return s.done }

func (s *Subscription) Reason() string {
	<-s.done
	return s.reason
}

func (s *Subscription) stop(reason string) {
	s.once.Do(func() {
		s.reason = reason
		close(s.done)
	})
}

const (
	ReasonSlowConsumer = "slow_consumer"
	ReasonRestart      = "restart"
)

type Hub struct {
	buffer    int
	listening atomic.Bool
	mu        sync.Mutex
	subs      map[uuid.UUID]map[*Subscription]struct{}
}

func (h *Hub) Listening() bool { return h.listening.Load() }

func NewHub(buffer int) *Hub {
	return &Hub{buffer: buffer, subs: map[uuid.UUID]map[*Subscription]struct{}{}}
}

func (h *Hub) Subscribe(workspaceID uuid.UUID) *Subscription {
	s := &Subscription{workspaceID: workspaceID, events: make(chan Event, h.buffer), done: make(chan struct{})}
	h.mu.Lock()
	defer h.mu.Unlock()
	set := h.subs[workspaceID]
	if set == nil {
		set = map[*Subscription]struct{}{}
		h.subs[workspaceID] = set
	}
	set[s] = struct{}{}
	return s
}

func (h *Hub) Unsubscribe(s *Subscription) {
	s.stop("")
	h.mu.Lock()
	defer h.mu.Unlock()
	if set := h.subs[s.workspaceID]; set != nil {
		delete(set, s)
		if len(set) == 0 {
			delete(h.subs, s.workspaceID)
		}
	}
}

func (h *Hub) Publish(e Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subs[e.WorkspaceID] {
		select {
		case <-s.done:
		case s.events <- e:
		default:
			s.stop(ReasonSlowConsumer)
		}
	}
}

func (h *Hub) StopAll(reason string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, set := range h.subs {
		for s := range set {
			s.stop(reason)
		}
	}
}

func (h *Hub) Count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, set := range h.subs {
		n += len(set)
	}
	return n
}
