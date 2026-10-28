package api

import (
	"context"
	"log/slog"
	"net/http"
	"slices"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/store"
)

type outreachTarget struct {
	manage  manageTarget
	stopped bool
}

func (s *Server) outreachTarget(ctx context.Context, q *store.Queries, ids []uuid.UUID, lock bool) (*outreachTarget, error) {
	ws := ids[0]
	if live, err := s.workspaceLive(ctx, ws); err != nil || !live {
		return nil, err
	}
	in, err := q.GetInbox(ctx, store.GetInboxParams{WorkspaceID: ws, ID: ids[1]})
	if store.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c, err := q.GetContact(ctx, store.GetContactParams{WorkspaceID: ws, ID: ids[2]})
	if store.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if lock {
		if _, err := q.LockContact(ctx, store.LockContactParams{WorkspaceID: ws, ID: c.ID}); err != nil {
			return nil, err
		}
	}
	t := outreachTarget{manage: manageTarget{ws: ws, inbox: in, contact: store.Contact{ID: c.ID, Locale: c.Locale}}}
	m := &t.manage
	if m.lists, err = q.ListInboxMailingLists(ctx, store.ListInboxMailingListsParams{WorkspaceID: ws, InboxID: in.ID}); err != nil {
		return nil, err
	}
	m.lists = slices.DeleteFunc(m.lists, func(l store.MailingList) bool { return l.Kind == listKindStatus })
	all, err := q.ListContactSubscriptions(ctx, store.ListContactSubscriptionsParams{WorkspaceID: ws, ContactID: c.ID})
	if err != nil {
		return nil, err
	}
	for _, sub := range all {
		if slices.ContainsFunc(m.lists, func(l store.MailingList) bool { return l.ID == sub.ListID }) {
			m.subs = append(m.subs, sub)
		}
	}
	if t.stopped, err = q.IsInboxMailOptedOut(ctx, store.IsInboxMailOptedOutParams{WorkspaceID: ws, InboxID: in.ID, ContactID: c.ID}); err != nil {
		return nil, err
	}
	return &t, nil
}

// Opening the page changes nothing, since mail scanners follow links; RFC 8058 one-click is a POST.
func (s *Server) outreachPage(w http.ResponseWriter, r *http.Request, token string) bool {
	source := "outreach"
	ids, _, ok := s.openToken(token, outreachTokenContext, 4, false)
	if !ok {
		if ids, _, ok = s.openToken(token, triggeredTokenContext, 4, false); !ok {
			return false
		}
		source = "triggered"
	}
	ctx := r.Context()
	if r.Method != http.MethodPost {
		t, err := s.outreachTarget(ctx, s.st.Queries, ids, false)
		switch {
		case err != nil:
			s.listPageError(w, r, err)
		case t == nil:
			goneListPage(w)
		default:
			s.outreachView(w, ctx, t, token, "", nil, false, source == "triggered")
		}
		return true
	}
	action := r.PostFormValue("action")
	oneClick := action == "" && r.PostFormValue("List-Unsubscribe") == "One-Click"
	var (
		t       *outreachTarget
		heading string
		undo    []uuid.UUID
		stopped bool
	)
	err := s.inTx(ctx, ids[0], func(q *store.Queries, events *eventBatch) error {
		var err error
		if t, err = s.outreachTarget(ctx, q, ids, true); err != nil || t == nil {
			return err
		}
		m := &t.manage
		words := s.listPageView(m.inbox, &m.contact).W
		stop := func() error {
			if err := q.AddInboxMailOptOut(ctx, store.AddInboxMailOptOutParams{WorkspaceID: m.ws, InboxID: m.inbox.ID, ContactID: m.contact.ID, Source: source, Now: s.now()}); err != nil {
				return err
			}
			if source != "triggered" {
				return nil
			}
			return s.unsubscribedThrough(ctx, q, m.ws, ids[3], m.contact.ID)
		}
		switch {
		case oneClick, action == "stop":
			heading = fill(words.OutreachStopped, "", m.inbox.Name, "")
			return stop()
		case action == "save" && source == "triggered":
			heading = words.Saved
			undo, err = s.saveChoices(ctx, q, events, r, m, formLists(r.PostForm["lists"]))
			return err
		case action == "all":
			heading = fill(words.AllGone, "", m.inbox.Name, "")
			if undo, err = s.leaveAll(ctx, q, events, r, m); err != nil || t.stopped {
				return err
			}
			stopped = true
			return stop()
		case action == "undo":
			heading = words.Restored
			if r.PostFormValue("stop") == "undo" {
				if err := q.RemoveInboxMailOptOut(ctx, store.RemoveInboxMailOptOutParams{
					WorkspaceID: m.ws, InboxID: m.inbox.ID, ContactID: m.contact.ID, Since: s.now().Add(-undoWindow),
				}); err != nil {
					return err
				}
			}
			return s.undoLeave(ctx, q, events, r, m, formLists(r.PostForm["lists"]))
		}
		return nil
	})
	switch {
	case err != nil && oneClick:
		s.log.ErrorContext(ctx, "one-click outreach stop", slog.Any("error", err))
		w.WriteHeader(http.StatusInternalServerError)
	case err != nil:
		s.listPageError(w, r, err)
	case t == nil && oneClick:
		w.WriteHeader(http.StatusNotFound)
	case t == nil:
		goneListPage(w)
	case oneClick:
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
	default:
		if fresh, ferr := s.outreachTarget(ctx, s.st.Queries, ids, false); ferr == nil && fresh != nil {
			t = fresh
		}
		s.outreachView(w, ctx, t, token, heading, undo, stopped, source == "triggered")
	}
	return true
}

func (s *Server) outreachView(w http.ResponseWriter, ctx context.Context, t *outreachTarget, token, heading string, undo []uuid.UUID, stopped, lists bool) {
	m := &t.manage
	v := s.listPageView(m.inbox, &m.contact)
	in := m.inbox.Name
	v.Target = managePathPrefix + token
	if heading != "" {
		v.Heading = heading
		if len(undo) > 0 || stopped {
			f := pageForm{Action: "undo", Button: v.W.Undo, Secondary: true}
			if stopped {
				f.Hidden = map[string]string{"stop": "undo"}
			}
			for _, id := range undo {
				f.Lists = append(f.Lists, id.String())
			}
			v.Forms = append(v.Forms, f)
		}
		writeListPage(w, http.StatusOK, v)
		return
	}
	v.Heading = fill(v.W.OutreachTitle, "", in, "")
	if addr, err := m.address(ctx, s.st.Queries); err == nil && addr != "" {
		v.Texts = []string{fill(v.W.OutreachText, "", in, maskAddress(addr))}
	}
	if t.stopped {
		v.Texts = append(v.Texts, fill(v.W.OutreachStopped, "", in, ""))
	} else {
		v.Forms = append(v.Forms, pageForm{Action: "stop", Button: fill(v.W.OutreachStop, "", in, ""), Note: v.W.OutreachStopNote})
	}
	if offered := m.offered(); lists && len(offered) > 0 {
		choices := pageForm{Action: "save", Button: v.W.Save, Legend: fill(v.W.Lists, "", in, ""), Secondary: true}
		for _, l := range offered {
			choices.Choices = append(choices.Choices, pageChoice{ID: l.ID.String(), Name: l.Name, Description: l.Description, Checked: activeState(m.subsOf(l.ID))})
		}
		v.Forms = append(v.Forms, choices)
	}
	v.Forms = append(v.Forms, pageForm{Action: "all", Button: fill(v.W.UnsubscribeAll, "", in, ""), Note: fill(v.W.UnsubscribeAllText, "", in, ""), Secondary: true})
	writeListPage(w, http.StatusOK, v)
}
