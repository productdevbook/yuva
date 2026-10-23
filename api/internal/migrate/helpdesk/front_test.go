package helpdesk_test

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/productdevbook/yuva/api/internal/migrate/helpdesk"
	"github.com/productdevbook/yuva/api/internal/migrate/helpdesk/helpdesktest"
)

func readFront(t *testing.T, f *helpdesk.Front, c helpdesk.Cursor) []helpdesk.Ticket {
	t.Helper()
	var out []helpdesk.Ticket
	for range 20 {
		page, err := f.Tickets(context.Background(), c)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, page.Tickets...)
		c = page.Next
		if page.End {
			return out
		}
	}
	t.Fatal("the listing did not end")
	return nil
}

func TestFrontReaderAgainstFixtures(t *testing.T) {
	srv, cfg, _ := fixtureConfig(t)
	ctx := context.Background()
	budget := &helpdesk.Budget{}
	f := helpdesk.NewFront(cfg, func() string { return helpdesktest.FrontToken }, budget)

	me, err := f.Me(ctx)
	if err != nil || me.ID != "cmp_fx1" {
		t.Fatalf("me: %+v %v", me, err)
	}
	folders, err := f.Folders(ctx)
	if err != nil || len(folders) != 2 || folders[0].ID != "inb_a1" || folders[0].Address != "support@front.example.com" || folders[1].Address != "billing@front.example.com" {
		t.Fatalf("inboxes: %+v %v", folders, err)
	}
	agents, err := f.Agents(ctx)
	if err != nil || len(agents) != 2 || agents[0].Email != "ece@front.example.com" || !agents[0].Admin {
		t.Fatalf("teammates: %+v %v", agents, err)
	}

	tickets := readFront(t, f, helpdesk.Cursor{})
	if len(tickets) != 5 {
		t.Fatalf("%d conversations, want 5: %+v", len(tickets), tickets)
	}
	got := map[string]helpdesk.Ticket{}
	for _, tk := range tickets {
		got[tk.ID] = tk
	}
	first := got["cnv_1a"]
	if first.Folder != "inb_a1" || first.Status != helpdesk.StatusOpen || first.Requester.Email != "ayse@example.org" || first.Requester.Name != "Ayse Kaya" ||
		first.Assignee == nil || first.Assignee.Email != "ece@front.example.com" || !slices.Equal(first.Tags, []string{"vip", "login"}) ||
		!first.CreatedAt.Equal(time.Unix(1760000000, 500000000)) || !first.UpdatedAt.After(first.CreatedAt) {
		t.Errorf("cnv_1a: %+v", first)
	}
	if got["cnv_1b"].Status != helpdesk.StatusClosed || got["cnv_1c"].Status != helpdesk.StatusPending || got["cnv_2a"].Status != helpdesk.StatusClosed || got["cnv_2b"].Status != helpdesk.StatusDeleted {
		t.Errorf("statuses: %s %s %s %s", got["cnv_1b"].Status, got["cnv_1c"].Status, got["cnv_2a"].Status, got["cnv_2b"].Status)
	}
	if got["cnv_1c"].Folder != "inb_a1" || got["cnv_2a"].Folder != "inb_b2" || got["cnv_2a"].Requester.Email != "" || got["cnv_2a"].Requester.Name != "Dan Roe" {
		t.Errorf("folders and requesters: %+v %+v", got["cnv_1c"], got["cnv_2a"])
	}

	msgs, err := f.Messages(ctx, first)
	if err != nil || len(msgs) != 3 {
		t.Fatalf("messages: %+v %v", msgs, err)
	}
	if msgs[0].Direction != "in" || msgs[0].Author.Email != "ayse@example.org" || msgs[0].Note || len(msgs[0].Attachments) != 1 ||
		!msgs[1].Note || msgs[1].Author.Email != "can@front.example.com" || msgs[1].Text != "Looks like the SSO outage." ||
		msgs[2].Direction != "out" || msgs[2].Author.Email != "ece@front.example.com" || !strings.Contains(msgs[2].Text, "reset your password") {
		t.Errorf("cnv_1a messages: %+v", msgs)
	}
	data, err := f.Attachment(ctx, first, msgs[0].Attachments[0])
	if err != nil || len(data) == 0 {
		t.Errorf("attachment: %v", err)
	}
	paged, err := f.Messages(ctx, got["cnv_1b"])
	if err != nil || len(paged) != 2 || paged[1].Direction != "out" {
		t.Errorf("paged messages: %+v %v", paged, err)
	}
	internal, err := f.Messages(ctx, got["cnv_2a"])
	if err != nil || len(internal) != 2 || internal[0].Direction != "in" || !internal[1].Note {
		t.Errorf("cnv_2a messages: %+v %v", internal, err)
	}
	contact, err := f.Customer(ctx, first.Requester)
	if err != nil || contact.Phone != "+905550100200" {
		t.Errorf("contact: %+v %v", contact, err)
	}
	if unknown, err := f.Customer(ctx, helpdesk.Person{ID: "x", Email: "nobody@example.org"}); err != nil || unknown.Phone != "" {
		t.Errorf("unknown contact: %+v %v", unknown, err)
	}

	reqs := srv.Requests()
	for _, r := range reqs {
		if r.Method != http.MethodGet || r.Host != helpdesk.FrontAPIHost || r.Auth != "Bearer "+helpdesktest.FrontToken {
			t.Errorf("%s %s%s with %q", r.Method, r.Host, r.Path, r.Auth)
		}
	}
	withinBudget(t, reqs, helpdesk.FrontAPIHost, helpdesktest.FrontLimit/2, 0)
	if budget.Limit != helpdesktest.FrontLimit {
		t.Errorf("budget limit %d, want the x-ratelimit-limit %d", budget.Limit, helpdesktest.FrontLimit)
	}
}

// The listing resumes from the stored cursor, the inbox and Front's page token, without reading
// a conversation twice or skipping one.
func TestFrontListingResumes(t *testing.T) {
	srv, cfg, _ := fixtureConfig(t)
	newFront := func() *helpdesk.Front {
		return helpdesk.NewFront(cfg, func() string { return helpdesktest.FrontToken }, &helpdesk.Budget{})
	}
	page, err := newFront().Tickets(context.Background(), helpdesk.Cursor{})
	if err != nil || page.End || page.Next.Page != 1 || page.Next.After != "pg2" || len(page.Tickets) != 2 {
		t.Fatalf("first page: %+v %v", page, err)
	}
	rest := readFront(t, newFront(), page.Next)
	var ids []string
	for _, tk := range append(page.Tickets, rest...) {
		ids = append(ids, tk.ID)
	}
	if !slices.Equal(ids, []string{"cnv_1a", "cnv_1b", "cnv_1c", "cnv_2a", "cnv_2b"}) {
		t.Errorf("conversations across the restart: %v", ids)
	}
	for _, r := range srv.Requests() {
		if strings.HasPrefix(r.Path, "/inboxes/") && strings.HasSuffix(r.Path, "/conversations") && !strings.Contains(r.Query, "q%5Bstatuses%5D") {
			t.Errorf("a listing without statuses: %s?%s", r.Path, r.Query)
		}
	}
}

// A stored page token belongs to the inbox that issued it: when the inbox list shrinks before the
// listing resumes, the token is never sent to the inbox that took its place.
func TestFrontResumeAfterInboxListChanges(t *testing.T) {
	srv, cfg, _ := fixtureConfig(t)
	newFront := func() *helpdesk.Front {
		return helpdesk.NewFront(cfg, func() string { return helpdesktest.FrontToken }, &helpdesk.Budget{})
	}
	page, err := newFront().Tickets(context.Background(), helpdesk.Cursor{})
	if err != nil || page.Next.Inbox != "inb_a1" || page.Next.After != "pg2" {
		t.Fatalf("first page: %+v %v", page, err)
	}
	srv.Override("front/inboxes.json", []byte(`{"_pagination": {"next": null}, "_results": [{"id": "inb_b2", "name": "Billing"}]}`))
	rest := readFront(t, newFront(), page.Next)
	var ids []string
	for _, tk := range rest {
		ids = append(ids, tk.ID)
	}
	if !slices.Equal(ids, []string{"cnv_2a", "cnv_2b"}) {
		t.Errorf("after inb_a1 was removed: %v", ids)
	}
	for _, r := range srv.Requests() {
		if strings.Contains(r.Query, "page_token=pg2") && r.Path != "/inboxes/inb_a1/conversations" {
			t.Errorf("the page token of inb_a1 was sent to %s", r.Path)
		}
	}

	srv.Override("front/inboxes.json", []byte(`{"_pagination": {"next": null}, "_results": [{"id": "inb_new", "name": "New"}, {"id": "inb_a1", "name": "Support"}, {"id": "inb_b2", "name": "Billing"}]}`))
	srv.Override("front/conversations_inb_new.json", []byte(`{"_pagination": {"next": null}, "_results": []}`))
	srv.Override("front/channels_inb_new.json", []byte(`{"_results": []}`))
	again := readFront(t, newFront(), page.Next)
	ids = nil
	for _, tk := range again {
		ids = append(ids, tk.ID)
	}
	if !slices.Equal(ids, []string{"cnv_1c", "cnv_2a", "cnv_2b"}) {
		t.Errorf("after an inbox was added before it: %v", ids)
	}
}

func TestFrontStatuses(t *testing.T) {
	srv, cfg, _ := fixtureConfig(t)
	srv.Override("front/conversations_inb_b2.json", []byte(`{"_pagination": {"next": null}, "_results": [{"id": "cnv_s", "subject": "Buy now", "status": "spam", "created_at": 1760300000}]}`))
	f := helpdesk.NewFront(cfg, func() string { return helpdesktest.FrontToken }, &helpdesk.Budget{})
	for _, tk := range readFront(t, f, helpdesk.Cursor{Inbox: "inb_b2", Page: 2}) {
		if tk.ID == "cnv_s" && tk.Status != helpdesk.StatusSpam {
			t.Errorf("a spam conversation is %s", tk.Status)
		}
	}
}

// Sync searches each inbox for conversations with activity after the cursor's time, under the
// search guard.
func TestFrontSyncSearchesPerInbox(t *testing.T) {
	srv, cfg, _ := fixtureConfig(t)
	f := helpdesk.NewFront(cfg, func() string { return helpdesktest.FrontToken }, &helpdesk.Budget{})
	since := time.Unix(1760104000, 0)
	got := readFront(t, f, helpdesk.Cursor{ModifiedSince: since})
	var ids []string
	for _, tk := range got {
		ids = append(ids, tk.ID)
	}
	if !slices.Equal(ids, []string{"cnv_1b", "cnv_1c", "cnv_2a"}) {
		t.Errorf("changed since %s: %v", since, ids)
	}
	searches := 0
	for _, r := range srv.Requests() {
		if strings.HasPrefix(r.Path, "/conversations/search/") {
			searches++
			if !strings.Contains(r.Path, "after:1760104000") {
				t.Errorf("search without the time: %s", r.Path)
			}
		}
	}
	if searches != 2 {
		t.Errorf("%d searches, want one per inbox", searches)
	}
}

func TestFrontRateLimitAndToken(t *testing.T) {
	srv, cfg, clock := fixtureConfig(t)
	ctx := context.Background()
	f := helpdesk.NewFront(cfg, func() string { return helpdesktest.FrontToken }, &helpdesk.Budget{})
	srv.FrontThrottle = 1
	_, err := f.Me(ctx)
	var limit *helpdesk.LimitError
	if !errors.As(err, &limit) || !limit.Until.Equal(clock.Now().Add(helpdesktest.FrontRetryAfter*time.Second)) {
		t.Fatalf("429: %v, want a limit error with Retry-After %ds", err, helpdesktest.FrontRetryAfter)
	}
	if _, err := f.Me(ctx); err != nil {
		t.Fatalf("after the limit: %v", err)
	}

	wrong := helpdesk.NewFront(cfg, func() string { return "wrong" }, &helpdesk.Budget{})
	var auth *helpdesk.AuthError
	if _, err := wrong.Me(ctx); !errors.As(err, &auth) {
		t.Errorf("a wrong token: %v, want an auth error", err)
	}
}

// An incoming message is by whoever sent it, a second contact included; the requester stands in
// only when the message names no sender. Only files Front stores are fetched, with the token, from
// the API host: addresses a customer typed are skipped.
func TestFrontMessageAuthorsAndAttachments(t *testing.T) {
	srv, cfg, _ := fixtureConfig(t)
	srv.Override("front/messages_cnv_1c.json", []byte(`{"_pagination": {"next": null}, "_results": [
	 {"id": "msg_a", "type": "email", "is_inbound": true, "created_at": 1760200000, "draft_mode": null, "author": null,
	  "recipients": [{"name": "Dave Poe", "handle": "Dave@example.net", "role": "from"}, {"name": "Carol Ng", "handle": "carol@example.com", "role": "cc"}],
	  "body": "<p>Adding my colleague.</p>", "text": "Adding my colleague.", "attachments": [
	  {"id": "a1", "url": "https://acme.api.frontapp.com/download/fil_ok", "filename": "ok.txt", "content_type": "text/plain", "size": 5, "metadata": {"is_inline": false}},
	  {"id": "a2", "url": "https://files.example.org/download/fil_typed", "filename": "typed.txt", "content_type": "text/plain", "size": 5, "metadata": {"is_inline": false}},
	  {"id": "a3", "url": "https://api2.frontapp.com:8443/download/fil_port", "filename": "port.txt", "content_type": "text/plain", "size": 5, "metadata": {"is_inline": false}},
	  {"id": "a4", "url": "https://api2.frontapp.com/conversations/cnv_1c", "filename": "api.txt", "content_type": "text/plain", "size": 5, "metadata": {"is_inline": false}},
	  {"id": "a5", "url": "http://api2.frontapp.com/download/fil_plain", "filename": "plain.txt", "content_type": "text/plain", "size": 5, "metadata": {"is_inline": false}}]},
	 {"id": "msg_b", "type": "email", "is_inbound": true, "created_at": 1760200100, "draft_mode": null, "author": null,
	  "recipients": [{"name": null, "handle": "carol@example.com", "role": "to"}], "body": "<p>No sender.</p>", "text": "No sender.", "attachments": []}]}`))
	f := helpdesk.NewFront(cfg, func() string { return helpdesktest.FrontToken }, &helpdesk.Budget{})
	tk := helpdesk.Ticket{ID: "cnv_1c", Requester: helpdesk.Person{ID: "carol@example.com", Email: "carol@example.com", Name: "Carol Ng"}}
	msgs, err := f.Messages(context.Background(), tk)
	if err != nil || len(msgs) != 2 {
		t.Fatalf("messages: %+v %v", msgs, err)
	}
	if msgs[0].Author.Email != "dave@example.net" || msgs[0].Author.Name != "Dave Poe" {
		t.Errorf("the sender of an incoming message: %+v", msgs[0].Author)
	}
	if msgs[1].Author.Email != "carol@example.com" {
		t.Errorf("a message without a sender: %+v", msgs[1].Author)
	}
	if len(msgs[0].Attachments) != 1 || msgs[0].Attachments[0].Ref != "fil_ok" {
		t.Fatalf("attachments: %+v", msgs[0].Attachments)
	}
	if _, err := f.Attachment(context.Background(), tk, msgs[0].Attachments[0]); err != nil {
		t.Fatal(err)
	}
	reqs := srv.Requests()
	for _, r := range reqs {
		if r.Host != helpdesk.FrontAPIHost || r.Auth != "Bearer "+helpdesktest.FrontToken {
			t.Errorf("%s%s was fetched with %q", r.Host, r.Path, r.Auth)
		}
	}
	if last := reqs[len(reqs)-1]; last.Path != "/download/fil_ok" {
		t.Errorf("last request: %s", last.Path)
	}
}

// Search requests stay at 40% of the reported limit, whatever else is read.
func TestFrontSearchGuard(t *testing.T) {
	b := helpdesk.FrontBudget(&helpdesk.Budget{})
	h := http.Header{}
	h.Set("X-Ratelimit-Limit", "100")
	b.Saw(h)
	now := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	var searches []time.Time
	for len(searches) < 120 {
		err := b.Take(helpdesk.ClassIncremental, now)
		var wait *helpdesk.WaitError
		if errors.As(err, &wait) {
			now = wait.Until
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		searches = append(searches, now)
	}
	for i, at := range searches {
		n := 0
		for _, o := range searches[:i+1] {
			if o.After(at.Add(-time.Minute)) {
				n++
			}
		}
		if n > 40 {
			t.Fatalf("%d searches in the minute up to %s, want at most 40", n, at)
		}
	}
}
