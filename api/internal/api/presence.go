package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"
	"uuid"

	"github.com/riverqueue/river"

	"github.com/productdevbook/yuva/api/internal/jobs"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	// presenceFresh is how recently a connection must have been seen to count as live; connections
	// are seen on every heartbeat.
	presenceFresh      = 2*realtimeHeartbeat + 15*time.Second
	maxPresenceMembers = 5
)

var weekdayNames = [...]oas.Weekday{oas.Sun, oas.Mon, oas.Tue, oas.Wed, oas.Thu, oas.Fri, oas.Sat}

func inboxOpen(in store.Inbox, now time.Time) bool {
	var h oas.BusinessHours
	if json.Unmarshal(in.BusinessHours, &h) != nil || !h.Enabled {
		return true
	}
	loc, err := time.LoadLocation(in.Timezone)
	if err != nil {
		loc = time.UTC
	}
	t := now.In(loc)
	day, clock := weekdayNames[t.Weekday()], t.Format("15:04")
	for _, iv := range h.Intervals {
		if iv.Day == day && iv.Start <= clock && clock < iv.End {
			return true
		}
	}
	return false
}

func initials(name string) string {
	var out []rune
	for _, w := range strings.Fields(name) {
		for _, r := range w {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				out = append(out, unicode.ToUpper(r))
				break
			}
		}
		if len(out) == 2 {
			break
		}
	}
	return string(out)
}

func clientMember(name string) oas.ClientMember {
	return oas.ClientMember{Name: name, Initials: initials(name)}
}

// presence is nil for `async` inboxes, which never show who is available.
func (s *Server) presence(ctx context.Context, q *store.Queries, in store.Inbox) (*oas.ClientPresence, error) {
	if in.Mode != string(oas.Live) {
		return nil, nil
	}
	now := s.now()
	out := &oas.ClientPresence{Members: []oas.ClientMember{}}
	if !inboxOpen(in, now) {
		return out, nil
	}
	rows, err := q.ListAvailableMembers(ctx, store.ListAvailableMembersParams{
		WorkspaceID: in.WorkspaceID, InboxID: in.ID, FreshAfter: now.Add(-presenceFresh),
	})
	if err != nil {
		return nil, err
	}
	out.Available = len(rows) > 0
	for _, r := range rows {
		if len(out.Members) == maxPresenceMembers {
			break
		}
		out.Members = append(out.Members, clientMember(r.Name))
	}
	return out, nil
}

// signal sends a short-lived notice to every process's realtime connections without storing it.
func (s *Server) signal(ctx context.Context, e realtime.Event) {
	e.CreatedAt = s.now()
	payload, err := realtime.SignalPayload(e)
	if err == nil {
		err = s.st.NotifySignal(context.WithoutCancel(ctx), store.NotifySignalParams{Channel: realtime.SignalChannel, Payload: payload})
	}
	if err != nil {
		s.log.WarnContext(ctx, "realtime signal", "type", e.Type, "error", err)
	}
}

func (s *Server) presenceHint(ctx context.Context, workspaceID uuid.UUID) {
	s.signal(ctx, realtime.Event{Type: realtime.PresenceHint, WorkspaceID: workspaceID, Data: json.RawMessage("{}")})
}

// memberPresence tells the workspace's members a member's current availability and whether they
// are online.
func (s *Server) memberPresence(ctx context.Context, workspaceID, memberID uuid.UUID) {
	rows, err := s.st.ListMemberPresence(context.WithoutCancel(ctx), store.ListMemberPresenceParams{
		WorkspaceID: workspaceID, Ids: []uuid.UUID{memberID}, FreshAfter: s.now().Add(-presenceFresh),
	})
	if err != nil {
		s.log.WarnContext(ctx, "member presence", "error", err)
		return
	}
	if len(rows) == 0 {
		return
	}
	s.signal(ctx, realtime.Event{Type: realtime.MemberPresence, WorkspaceID: workspaceID, Data: mustJSON(oas.MemberPresence{
		MemberId: memberID, Availability: oas.Availability(rows[0].Availability), Online: rows[0].Online,
	})})
}

// announceContactPresence tells the workspace's members whether a contact is online now.
func (s *Server) announceContactPresence(ctx context.Context, workspaceID, contactID uuid.UUID) {
	online, last, err := s.contactPresence(context.WithoutCancel(ctx), s.st.Queries, workspaceID, contactID)
	if err != nil {
		s.log.WarnContext(ctx, "contact presence", "error", err)
		return
	}
	s.signal(ctx, realtime.Event{Type: realtime.ContactPresence, WorkspaceID: workspaceID, Data: mustJSON(oas.ContactPresence{
		ContactId: contactID, Online: online, LastSeenAt: last,
	})})
}

type presenceSweepWorker struct {
	river.WorkerDefaults[jobs.PresenceSweepArgs]
	s *Server
}

func (w *presenceSweepWorker) Work(ctx context.Context, _ *river.Job[jobs.PresenceSweepArgs]) error {
	return w.s.AnnounceLapsedPresence(ctx)
}

// AnnounceLapsedPresence tells teammates about members and contacts whose last connection stopped
// being seen without closing, as when a server process dies. The window covers two sweeps, so a
// late run still finds them; one may be announced twice.
func (s *Server) AnnounceLapsedPresence(ctx context.Context) error {
	ids, err := s.st.ListWorkspaceIDs(ctx)
	if err != nil {
		return err
	}
	fresh := s.now().Add(-presenceFresh)
	for _, ws := range ids {
		members, err := s.st.ListLapsedMembers(ctx, store.ListLapsedMembersParams{
			WorkspaceID: ws, FreshAfter: fresh, LapsedAfter: fresh.Add(-2 * jobs.PresenceSweepInterval),
		})
		if err != nil {
			return fmt.Errorf("workspace %s: %w", ws, err)
		}
		if len(members) > 0 {
			s.presenceHint(ctx, ws)
		}
		for _, id := range members {
			s.memberPresence(ctx, ws, id)
		}
		contacts, err := s.st.ListLapsedContacts(ctx, store.ListLapsedContactsParams{
			WorkspaceID: ws, FreshAfter: fresh, LapsedAfter: fresh.Add(-2 * jobs.PresenceSweepInterval),
		})
		if err != nil {
			return fmt.Errorf("workspace %s: %w", ws, err)
		}
		for _, id := range contacts {
			s.announceContactPresence(ctx, ws, id)
		}
	}
	return nil
}
