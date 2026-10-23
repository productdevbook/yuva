package api

import (
	"context"
	"slices"
	"sync"
	"time"
	"uuid"
)

const (
	clientSocketsPerSession = 5
	clientSocketsPerAddress = 50
	clientReplayMax         = 1000
	contactPresenceEvery    = 2 * time.Second
	presenceGatesMax        = 10000
	reasonTooManySockets    = "too_many_connections"
)

var (
	limitClientRealtimePerSession = limit{30, time.Minute}
	limitClientRealtimePerIP      = limit{120, time.Minute}
)

// clientSocket is one open /client/v1/realtime connection of this process. kicked closes when a
// newer connection of the same session or address pushed it over its cap.
type clientSocket struct {
	session, contact uuid.UUID
	addr             string
	kicked           chan struct{}
	gone             bool
}

type presenceGate struct {
	last    time.Time
	pending bool
}

// clientSockets holds the open client connections of this process by session and address, the
// open count per contact, and when each contact's presence was last announced.
type clientSockets struct {
	mu        sync.Mutex
	bySession map[uuid.UUID][]*clientSocket
	byAddr    map[string][]*clientSocket
	contacts  map[uuid.UUID]int
	presence  map[uuid.UUID]*presenceGate
}

func newClientSockets() *clientSockets {
	return &clientSockets{
		bySession: map[uuid.UUID][]*clientSocket{}, byAddr: map[string][]*clientSocket{},
		contacts: map[uuid.UUID]int{}, presence: map[uuid.UUID]*presenceGate{},
	}
}

// open registers a connection, closes the oldest of its session or address beyond their caps, and
// says whether it is the contact's first open connection in this process.
func (c *clientSockets) open(session, contact uuid.UUID, addr string) (*clientSocket, bool) {
	sock := &clientSocket{session: session, contact: contact, addr: addr, kicked: make(chan struct{})}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bySession[session] = append(c.bySession[session], sock)
	c.byAddr[addr] = append(c.byAddr[addr], sock)
	for len(c.bySession[session]) > clientSocketsPerSession {
		c.kick(c.bySession[session][0])
	}
	for len(c.byAddr[addr]) > clientSocketsPerAddress {
		c.kick(c.byAddr[addr][0])
	}
	c.contacts[contact]++
	return sock, c.contacts[contact] == 1
}

func (c *clientSockets) kick(sock *clientSocket) {
	c.drop(sock)
	close(sock.kicked)
}

func (c *clientSockets) drop(sock *clientSocket) {
	if sock.gone {
		return
	}
	sock.gone = true
	without := func(list []*clientSocket) []*clientSocket {
		return slices.DeleteFunc(list, func(x *clientSocket) bool { return x == sock })
	}
	if c.bySession[sock.session] = without(c.bySession[sock.session]); len(c.bySession[sock.session]) == 0 {
		delete(c.bySession, sock.session)
	}
	if c.byAddr[sock.addr] = without(c.byAddr[sock.addr]); len(c.byAddr[sock.addr]) == 0 {
		delete(c.byAddr, sock.addr)
	}
}

// close unregisters a connection and says whether it was the contact's last open connection in
// this process.
func (c *clientSockets) close(sock *clientSocket) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.drop(sock)
	if c.contacts[sock.contact]--; c.contacts[sock.contact] > 0 {
		return false
	}
	delete(c.contacts, sock.contact)
	return true
}

// presenceDue says whether a contact's presence may be announced now; otherwise it marks one
// announcement due after the wait it returns, or returns false and no wait when one already is.
func (c *clientSockets) presenceDue(contact uuid.UUID, now time.Time) (bool, time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	g := c.presence[contact]
	if g == nil {
		if len(c.presence) >= presenceGatesMax {
			for id, old := range c.presence {
				if !old.pending && now.Sub(old.last) >= contactPresenceEvery {
					delete(c.presence, id)
				}
			}
		}
		g = &presenceGate{}
		c.presence[contact] = g
	}
	if g.pending {
		return false, 0
	}
	if wait := g.last.Add(contactPresenceEvery).Sub(now); wait > 0 {
		g.pending = true
		return false, wait
	}
	g.last = now
	return true, 0
}

func (c *clientSockets) presenceSent(contact uuid.UUID, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if g := c.presence[contact]; g != nil {
		g.pending, g.last = false, now
	}
}

// contactPresenceChanged announces a contact's presence at most once per contactPresenceEvery;
// a change inside that time is announced once when it ends, with the state it ends in.
func (s *Server) contactPresenceChanged(ctx context.Context, workspaceID, contactID uuid.UUID) {
	ok, wait := s.sockets.presenceDue(contactID, time.Now())
	if ok {
		s.announceContactPresence(ctx, workspaceID, contactID)
		return
	}
	if wait == 0 {
		return
	}
	bg := context.WithoutCancel(ctx)
	time.AfterFunc(wait, func() {
		s.sockets.presenceSent(contactID, time.Now())
		s.announceContactPresence(bg, workspaceID, contactID)
	})
}
