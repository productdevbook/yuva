package helpdesk_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/productdevbook/yuva/api/internal/migrate/helpdesk"
	"github.com/productdevbook/yuva/api/internal/migrate/helpdesk/helpdesktest"
)

func freshdeskReader(t *testing.T, key string) (*helpdesktest.Server, *helpdesk.Freshdesk, *helpdesk.Budget, helpdesk.Config) {
	srv, cfg, _ := fixtureConfig(t)
	budget := &helpdesk.Budget{}
	return srv, helpdesk.NewFreshdesk(cfg, helpdesktest.FreshdeskSubdomain, func() string { return key }, budget), budget, cfg
}

func listFreshdesk(t *testing.T, f *helpdesk.Freshdesk, c helpdesk.Cursor) ([]helpdesk.Ticket, helpdesk.Cursor, int) {
	t.Helper()
	var out []helpdesk.Ticket
	pages := 0
	for {
		page, err := f.Tickets(context.Background(), c)
		if err != nil {
			t.Fatal(err)
		}
		pages++
		out = append(out, page.Tickets...)
		c = page.Next
		if page.End {
			return out, c, pages
		}
	}
}

func TestFreshdeskReaderAgainstFixtures(t *testing.T) {
	srv, f, budget, _ := freshdeskReader(t, helpdesktest.FreshdeskKey)
	ctx := context.Background()
	me, err := f.Me(ctx)
	if err != nil || !me.Admin || me.Email != "deniz@acme.example.com" || me.ID != "acme.freshdesk.com" {
		t.Fatalf("me: %+v %v", me, err)
	}
	folders, err := f.Folders(ctx)
	if err != nil || len(folders) != 3 {
		t.Fatalf("folders: %+v %v", folders, err)
	}
	if folders[0].ID != "default" || folders[0].Address != "support@acme.example.com" || folders[1].ID != "9001" || folders[1].Address != "pay@acme.example.com" ||
		folders[2].Name != "Acme Cloud" || folders[2].Address != "cloud@acme.example.com" {
		t.Errorf("folders: %+v", folders)
	}
	agents, err := f.Agents(ctx)
	if err != nil || len(agents) != 2 || agents[1].Email != "ece@acme.example.com" || !agents[0].Agent {
		t.Fatalf("agents: %+v %v", agents, err)
	}

	tickets, cursor, _ := listFreshdesk(t, f, helpdesk.Cursor{})
	if len(tickets) != 7 || cursor.ModifiedSince.IsZero() {
		t.Fatalf("%d tickets, cursor %+v", len(tickets), cursor)
	}
	first := tickets[0]
	if first.ID != "101" || first.Folder != "9001" || first.Requester.Email != "ayse@example.org" || first.Requester.ID != "3001" ||
		first.Requester.Phone != "+90 555 0100" || first.Assignee == nil || first.Assignee.Email != "ece@acme.example.com" ||
		first.Status != helpdesk.StatusOpen || !slices.Equal(first.Tags, []string{"login", "vip"}) {
		t.Errorf("ticket 101: %+v", first)
	}
	if tickets[1].Folder != "default" || tickets[1].Assignee == nil || tickets[1].Assignee.Email != "deniz@acme.example.com" {
		t.Errorf("ticket 102: %+v", tickets[1])
	}
	if tickets[2].Requester.Email != "" || tickets[2].Requester.Phone != "+90 555 0103" || tickets[2].Assignee != nil {
		t.Errorf("ticket 103: %+v", tickets[2])
	}
	want := map[string]string{
		"101": helpdesk.StatusOpen, "102": helpdesk.StatusClosed, "103": helpdesk.StatusPending, "104": helpdesk.StatusClosed,
		"105": helpdesk.StatusOpen, "106": helpdesk.StatusClosed, "107": helpdesk.StatusSpam,
	}
	for _, tk := range tickets {
		if tk.Status != want[tk.ID] {
			t.Errorf("ticket %s is %s, want %s", tk.ID, tk.Status, want[tk.ID])
		}
	}

	msgs, err := f.Messages(ctx, first)
	if err != nil || len(msgs) != 6 {
		t.Fatalf("messages: %d %v", len(msgs), err)
	}
	if m := msgs[0]; m.ID != "description" || m.Direction != "in" || m.Note || m.Author.Email != "ayse@example.org" || m.HTML == "" ||
		!strings.Contains(m.Text, "cannot log in") || len(m.Attachments) != 1 {
		t.Errorf("description: %+v", m)
	}
	if m := msgs[1]; m.ID != "2001" || m.Direction != "out" || m.Note || m.Author.Email != "ece@acme.example.com" || len(m.Attachments) != 1 {
		t.Errorf("reply: %+v", m)
	}
	if m := msgs[2]; m.Direction != "in" || m.Author.ID != "3001" || m.Author.Email != "ayse@example.org" {
		t.Errorf("customer reply: %+v", m)
	}
	if m := msgs[3]; !m.Note || m.Author.Email != "ece@acme.example.com" {
		t.Errorf("private note: %+v", m)
	}
	if m := msgs[4]; !m.Note || m.Author.Email != "deniz@acme.example.com" {
		t.Errorf("public note: %+v", m)
	}
	if m := msgs[5]; m.Note || m.Direction != "in" || m.Author.ID != "3009" || m.Author.Email != "can@example.net" || m.Author.Name != "Can Demir" {
		t.Errorf("a copied contact's reply is the customer's side: %+v", m)
	}
	outbound, err := f.Messages(ctx, tickets[1])
	if err != nil || len(outbound) != 1 || outbound[0].Direction != "out" || outbound[0].Author.Email != "deniz@acme.example.com" {
		t.Errorf("outbound ticket: %+v %v", outbound, err)
	}
	chat, err := f.Messages(ctx, tickets[2])
	if err != nil || len(chat) != 2 || chat[0].Text == "" || chat[1].Author.ID != "3003" {
		t.Errorf("chat ticket: %+v %v", chat, err)
	}
	var se *helpdesk.StatusError
	if _, err := f.Messages(ctx, helpdesk.Ticket{ID: "999"}); !errors.As(err, &se) || se.Status != http.StatusNotFound {
		t.Errorf("a missing ticket: %v", err)
	}

	for _, a := range []helpdesk.Attachment{msgs[0].Attachments[0], msgs[1].Attachments[0]} {
		data, err := f.Attachment(ctx, first, a)
		if err != nil || len(data) == 0 {
			t.Errorf("attachment %s: %v", a.Name, err)
		}
	}
	if _, err := f.Attachment(ctx, tickets[2], chat[1].Attachments[0]); !errors.As(err, &se) || se.Status != http.StatusForbidden {
		t.Errorf("an attachment that stays refused: %v", err)
	}

	reqs := srv.Requests()
	basic := "Basic " + base64.StdEncoding.EncodeToString([]byte(helpdesktest.FreshdeskKey+":X"))
	var expired, files int
	for _, r := range reqs {
		if r.Method != http.MethodGet {
			t.Errorf("%s %s%s was sent", r.Method, r.Host, r.Path)
		}
		switch r.Host {
		case "acme.freshdesk.com":
			if r.Auth != basic {
				t.Errorf("%s was sent with %q", r.Path, r.Auth)
			}
		case helpdesktest.FreshdeskFilesHost:
			files++
			if r.Auth != "" {
				t.Errorf("the attachment %s received %q", r.Path, r.Auth)
			}
			if strings.Contains(r.Query, "stale") {
				expired++
			}
		default:
			t.Errorf("a request reached %s", r.Host)
		}
	}
	if expired != 2 || files != 5 {
		t.Errorf("%d expired of %d attachment requests, want 2 of 5", expired, files)
	}
	withinBudget(t, reqs, "acme.freshdesk.com", helpdesktest.FreshdeskLimit/2, 0)
	if budget.Limit != helpdesktest.FreshdeskLimit {
		t.Errorf("budget limit %d, want the X-Ratelimit-Total %d", budget.Limit, helpdesktest.FreshdeskLimit)
	}
}

func TestFreshdeskMeRefusesAnAgentWhoIsNotAnAdmin(t *testing.T) {
	_, f, _, _ := freshdeskReader(t, helpdesktest.FreshdeskAgentKey)
	me, err := f.Me(context.Background())
	if err != nil || me.Admin || me.Email != "ece@acme.example.com" {
		t.Errorf("me: %+v %v", me, err)
	}
	_, f, _, _ = freshdeskReader(t, "wrong-key")
	var auth *helpdesk.AuthError
	if _, err := f.Me(context.Background()); !errors.As(err, &auth) {
		t.Errorf("a wrong key: %v", err)
	}
}

func TestFreshdeskListingStartsOverAfterTheCut(t *testing.T) {
	srv, f, _, _ := freshdeskReader(t, helpdesktest.FreshdeskKey)
	f.SetPaging(2, 2)
	srv.CapFreshdeskPages(2)
	tickets, cursor, pages := listFreshdesk(t, f, helpdesk.Cursor{})
	var ids []string
	for _, tk := range tickets {
		ids = append(ids, tk.ID)
	}
	if !slices.Equal(ids, []string{"101", "102", "103", "104", "105", "106", "107"}) {
		t.Errorf("tickets %v, each once", ids)
	}
	if pages != 6 {
		t.Errorf("%d pages, want 6", pages)
	}
	if !cursor.ModifiedSince.Equal(time.Date(2026, 9, 7, 8, 29, 59, 0, time.UTC)) {
		t.Errorf("cursor %+v", cursor)
	}
	var starts []string
	for _, r := range srv.Requests() {
		if r.Path == "/api/v2/tickets" {
			if strings.Contains(r.Query, "page=3") {
				t.Errorf("page 3 was requested: %s", r.Query)
			}
			if strings.Contains(r.Query, "page=1") {
				starts = append(starts, r.Query)
			}
		}
	}
	if len(starts) != 5 || !strings.Contains(starts[3], "updated_since=2026-09-05T07%3A59%3A59Z") || !strings.Contains(starts[0], "order_by=updated_at") ||
		!strings.Contains(starts[0], "order_type=asc") {
		t.Errorf("listings: %v", starts)
	}

	changed, _, _ := listFreshdesk(t, f, helpdesk.Cursor{ModifiedSince: time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC)})
	if len(changed) != 2 || changed[0].ID != "106" {
		t.Errorf("changes since 6 September: %+v", changed)
	}
}

func TestFreshdeskWaitsOutTheLimit(t *testing.T) {
	srv, f, _, cfg := freshdeskReader(t, helpdesktest.FreshdeskKey)
	srv.ThrottleFreshdesk(1)
	_, err := f.Folders(context.Background())
	var limit *helpdesk.LimitError
	if !errors.As(err, &limit) || !limit.Until.Equal(cfg.Now().Add(7*time.Second)) {
		t.Fatalf("a 429 with Retry-After 7: %v", err)
	}
	if _, err := f.Folders(context.Background()); err != nil {
		t.Errorf("after the wait: %v", err)
	}
}

func TestFreshdeskBudgetCountsListingsApart(t *testing.T) {
	b := helpdesk.FreshdeskBudget(&helpdesk.Budget{})
	if b.PerWindow() != 50 {
		t.Errorf("%d requests a minute before Freshdesk reports its limit, want 50", b.PerWindow())
	}
	h := http.Header{}
	h.Set("X-Ratelimit-Total", "400")
	b.Saw(h)
	if b.PerWindow() != 200 {
		t.Errorf("%d requests a minute under a limit of 400, want 200", b.PerWindow())
	}
	now := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	var listings int
	for i := 0; i < 400; i++ {
		err := b.Take(helpdesk.ClassIncremental, now)
		var wait *helpdesk.WaitError
		if errors.As(err, &wait) {
			break
		}
		listings++
	}
	if listings != 20 {
		t.Errorf("%d listing requests in a minute, want 20", listings)
	}
}

func TestFreshdeskSenderAndAttachmentHosts(t *testing.T) {
	srv, f, _, _ := freshdeskReader(t, helpdesktest.FreshdeskKey)
	ctx := context.Background()
	ticket := helpdesk.Ticket{ID: "108", Requester: helpdesk.Person{ID: "3002", Name: "Kerem Aslan", Email: "kerem@example.net"}}
	msgs, err := f.Messages(ctx, ticket)
	if err != nil || len(msgs) != 5 {
		t.Fatalf("messages: %+v %v", msgs, err)
	}
	if m := msgs[1]; !m.Note || m.Author.Email != "zeynep@example.org" || m.Author.Name != "Zeynep Arslan" || m.Author.ID == ticket.Requester.ID {
		t.Errorf("a second contact's message was written by %+v", m)
	}
	if m := msgs[2]; m.Note || m.Direction != "in" || m.Author.ID != "3002" || m.Author.Email != "kerem@example.net" {
		t.Errorf("a message without a sender is the requester's: %+v", m)
	}
	if m := msgs[3]; !m.Note || m.Author.Email != "ece@acme.example.com" {
		t.Errorf("a forward to a third party must be a note: %+v", m)
	}
	if m := msgs[4]; !m.Note || m.Direction != "" || m.Author.Email != "legal@example.com" {
		t.Errorf("a third party's reply to a forward must be a note: %+v", m)
	}
	if strings.Contains(msgs[0].HTML, "typed.txt") && len(msgs[0].Attachments) != 0 {
		t.Errorf("a link in the text became an attachment: %+v", msgs[0].Attachments)
	}

	before := len(srv.Requests())
	var refused *helpdesk.RefusedError
	if _, err := f.Attachment(ctx, ticket, msgs[1].Attachments[0]); !errors.As(err, &refused) {
		t.Errorf("an attachment on the account's own host: %v", err)
	}
	typed := helpdesk.Attachment{Name: "typed.txt", Ref: "description|1|https://acme.freshdesk.com/data/attachments/9/typed.txt"}
	if _, err := f.Attachment(ctx, ticket, typed); !errors.As(err, &refused) {
		t.Errorf("a link on the API host: %v", err)
	}
	if n := len(srv.Requests()); n != before {
		t.Errorf("%d requests were sent for attachments on the API host", n-before)
	}
}

func TestFreshdeskAttachmentsOnlyFromItsHosts(t *testing.T) {
	srv, f, _, _ := freshdeskReader(t, helpdesktest.FreshdeskKey)
	var refused *helpdesk.RefusedError
	a := helpdesk.Attachment{Name: "x.txt", Ref: "description|1|https://" + helpdesktest.OutsideHost + "/x.txt"}
	if _, err := f.Attachment(context.Background(), helpdesk.Ticket{ID: "101"}, a); !errors.As(err, &refused) {
		t.Errorf("an attachment on another host: %v", err)
	}
	for _, bad := range []string{"https://" + helpdesktest.FreshdeskS3Host + ".evil.example/x.txt", "http://" + helpdesktest.FreshdeskS3Host + "/x.txt"} {
		if _, err := f.Attachment(context.Background(), helpdesk.Ticket{ID: "101"}, helpdesk.Attachment{Ref: "description|1|" + bad}); !errors.As(err, &refused) {
			t.Errorf("%s: %v", bad, err)
		}
	}
	regional := helpdesk.Attachment{Ref: "description|1|https://" + helpdesktest.FreshdeskS3Host + "/cdn.freshdesk.com/x.txt?sig=fresh"}
	if data, err := f.Attachment(context.Background(), helpdesk.Ticket{ID: "101"}, regional); err != nil || len(data) == 0 {
		t.Errorf("a regional S3 attachment: %v", err)
	}
	var reached bool
	for _, r := range srv.Requests() {
		if r.Host == helpdesktest.OutsideHost || strings.HasSuffix(r.Host, "evil.example") {
			t.Errorf("%s was requested", r.Host)
		}
		if r.Host == helpdesktest.FreshdeskS3Host {
			reached = true
			if r.Auth != "" {
				t.Errorf("the key went to S3: %q", r.Auth)
			}
		}
	}
	if !reached {
		t.Error("the regional host was not reached")
	}
}

func TestFreshdeskListingSurvivesChangesBetweenPages(t *testing.T) {
	srv, f, _, _ := freshdeskReader(t, helpdesktest.FreshdeskKey)
	f.SetPaging(2, 300)
	ctx := context.Background()
	first, err := f.Tickets(ctx, helpdesk.Cursor{})
	if err != nil || len(first.Tickets) != 2 || first.End {
		t.Fatalf("first page: %+v %v", first, err)
	}
	raw, err := helpdesktest.Fixture("freshdesk/tickets.json")
	if err != nil {
		t.Fatal(err)
	}
	var all []map[string]any
	if err := json.Unmarshal(raw, &all); err != nil {
		t.Fatal(err)
	}
	all[0]["updated_at"] = "2026-09-09T10:00:00Z"
	changed := append(append([]map[string]any{}, all[1:]...), all[0])
	body, _ := json.Marshal(changed)
	srv.Override("freshdesk/tickets.json", body)

	ids := []string{"101", "102"}
	c := first.Next
	for {
		page, err := f.Tickets(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
		for _, tk := range page.Tickets {
			ids = append(ids, tk.ID)
		}
		c = page.Next
		if page.End {
			break
		}
	}
	for _, want := range []string{"101", "102", "103", "104", "105", "106", "107"} {
		if !slices.Contains(ids, want) {
			t.Errorf("ticket %s was lost: %v", want, ids)
		}
	}
	if n := strings.Count(strings.Join(ids, " "), "101"); n != 2 {
		t.Errorf("the changed ticket was read %d times, want its old and its new version: %v", n, ids)
	}
}
