package helpdesk_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/productdevbook/yuva/api/internal/migrate/helpdesk"
	"github.com/productdevbook/yuva/api/internal/migrate/helpdesk/helpdesktest"
)

const chatwootBase = "https://" + helpdesktest.ChatwootHost

func chatwootReader(cfg helpdesk.Config, account, token string) *helpdesk.Chatwoot {
	return helpdesk.NewChatwoot(cfg, chatwootBase, account, func() string { return token }, &helpdesk.Budget{})
}

func TestChatwootReaderAgainstFixtures(t *testing.T) {
	srv, cfg, _ := fixtureConfig(t)
	ctx := context.Background()
	w := chatwootReader(cfg, "1", helpdesktest.ChatwootToken)

	me, err := w.Me(ctx)
	if err != nil || !me.Admin || me.ID != "1" || me.Email != "deniz@acme.example.com" {
		t.Fatalf("me: %+v %v", me, err)
	}
	other, err := chatwootReader(cfg, "2", helpdesktest.ChatwootToken).Me(ctx)
	if err != nil || other.Admin {
		t.Errorf("an agent of account 2 is an admin: %+v %v", other, err)
	}
	var auth *helpdesk.AuthError
	_, err = chatwootReader(cfg, "3", helpdesktest.ChatwootToken).Me(ctx)
	if !errors.As(err, &auth) || !strings.Contains(err.Error(), "1 Acme Support, 2 Acme Labs") {
		t.Errorf("an account the user is not in: %v", err)
	}
	if _, err = chatwootReader(cfg, "1", "wrong").Me(ctx); !errors.As(err, &auth) {
		t.Errorf("a wrong token: %v", err)
	}

	folders, err := w.Folders(ctx)
	if err != nil || len(folders) != 2 || folders[0].ID != "1" || folders[0].Address != "support@acme.example.com" || folders[1].Address != "" {
		t.Fatalf("inboxes: %+v %v", folders, err)
	}
	agents, err := w.Agents(ctx)
	if err != nil || len(agents) != 2 || !agents[0].Admin || agents[1].Admin || !agents[1].Agent {
		t.Fatalf("agents: %+v %v", agents, err)
	}

	var tickets []helpdesk.Ticket
	c := helpdesk.Cursor{}
	for {
		page, err := w.Tickets(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
		tickets = append(tickets, page.Tickets...)
		c = page.Next
		if page.End {
			break
		}
	}
	if len(tickets) != 27 || c.Page != 3 {
		t.Fatalf("%d conversations, cursor %+v", len(tickets), c)
	}
	first := tickets[0]
	if first.ID != "105" || first.Folder != "1" || first.Subject != "Invoice question" || first.Status != helpdesk.StatusOpen ||
		first.Requester.Email != "ayse@example.org" || first.Requester.ExternalID != "crm-101" || first.Requester.Phone != "+905550000011" ||
		first.Assignee == nil || first.Assignee.Email != "ece@acme.example.com" || !slices.Equal(first.Tags, []string{"billing", "vip"}) ||
		first.UpdatedAt.Unix() != 1760005000 || first.CreatedAt.Unix() != 1760004000 {
		t.Errorf("conversation 105: %+v", first)
	}
	if tickets[1].Status != helpdesk.StatusPending || tickets[2].Status != helpdesk.StatusPending || tickets[2].Requester.Email != "" ||
		tickets[2].Requester.Phone != "+905550000013" || tickets[26].Status != helpdesk.StatusOpen || tickets[25].Status != helpdesk.StatusClosed {
		t.Errorf("statuses: %+v", tickets[1:3])
	}

	var export helpdesk.Ticket
	for _, tk := range tickets {
		if tk.ID == "101" {
			export = tk
		}
	}
	msgs, err := w.Messages(ctx, export)
	if err != nil || len(msgs) != 22 {
		t.Fatalf("messages: %d %v", len(msgs), err)
	}
	for i := 1; i < len(msgs); i++ {
		if msgs[i-1].At.After(msgs[i].At) {
			t.Fatalf("messages are not oldest first at %d", i)
		}
	}
	if msgs[0].ID != "1001" || msgs[0].Direction != "in" || msgs[0].Author.Email != "carol@example.net" || len(msgs[0].Attachments) != 1 ||
		msgs[0].Attachments[0].Name != "screenshot.png" || msgs[0].Attachments[0].Type != "image/png" {
		t.Errorf("message 1001: %+v", msgs[0])
	}
	if msgs[1].ID != "1002" || msgs[1].Direction != "out" || !msgs[1].Author.Agent || msgs[1].Note {
		t.Errorf("message 1002: %+v", msgs[1])
	}
	if msgs[2].ID != "1004" || !msgs[2].Note || !msgs[2].Author.Agent {
		t.Errorf("the private note: %+v", msgs[2])
	}
	if slices.ContainsFunc(msgs, func(m helpdesk.Message) bool { return m.ID == "1003" }) {
		t.Error("an activity line became a message")
	}
	invoice, err := w.Messages(ctx, first)
	if err != nil || len(invoice) != 2 || !strings.Contains(invoice[0].HTML, "<b>wrong</b>") || invoice[0].Text != "Invoice 7 is wrong" {
		t.Errorf("e-mail message: %+v %v", invoice, err)
	}
	refund, err := w.Messages(ctx, helpdesk.Ticket{ID: "102", Requester: helpdesk.Person{ID: "11", Email: "ayse@example.org"}})
	if err != nil || len(refund) != 3 || len(refund[0].Attachments) != 0 || refund[0].Author.ID != "11" ||
		refund[2].Direction != "in" || refund[2].Author.ID != "14" || refund[2].Author.Email != "cem@example.net" {
		t.Errorf("a cc'd contact's reply and a fallback attachment: %+v %v", refund, err)
	}
	filler, err := w.Messages(ctx, helpdesk.Ticket{ID: "301"})
	if err != nil || len(filler) != 1 {
		t.Errorf("a conversation of one message: %+v %v", filler, err)
	}

	var outside, inside helpdesk.Attachment
	for _, m := range msgs {
		for _, a := range m.Attachments {
			if strings.Contains(a.Ref, helpdesktest.OutsideHost) {
				outside = a
			} else {
				inside = a
			}
		}
	}
	for _, a := range []helpdesk.Attachment{outside, inside} {
		if data, err := w.Attachment(ctx, export, a); err != nil || len(data) == 0 {
			t.Errorf("attachment %s: %v", a.Name, err)
		}
	}
	if inside.Name != "export-log.txt" || inside.Type != "text/plain; charset=utf-8" {
		t.Errorf("attachment on the install: %+v", inside)
	}

	synced, err := w.Tickets(ctx, helpdesk.Cursor{ModifiedSince: time.Unix(1760002500, 0)})
	if err != nil || !synced.End || len(synced.Tickets) != 3 || synced.Tickets[2].ID != "103" {
		t.Errorf("sync reads the listing until the cursor's time: %+v %v", synced, err)
	}

	reqs := srv.Requests()
	onlyGETs(t, reqs, "")
	withinBudget(t, reqs, helpdesktest.ChatwootHost, 60, 0)
	var before, outsideSeen bool
	for _, r := range reqs {
		switch {
		case r.Host == helpdesktest.OutsideHost:
			outsideSeen = true
			if r.Token != "" || r.Auth != "" {
				t.Errorf("the attachment on %s carried a token: %+v", r.Host, r)
			}
		case r.Host == helpdesktest.ChatwootHost:
			if strings.HasPrefix(r.Path, "/rails/") {
				if r.Token != "" || r.Auth != "" {
					t.Errorf("the attachment %s carried the token", r.Path)
				}
				continue
			}
			if r.Token == "" || r.Auth != "" || (r.Token != helpdesktest.ChatwootToken && r.Token != "wrong") {
				t.Errorf("%s carried token %q and Authorization %q", r.Path, r.Token, r.Auth)
			}
			before = before || r.Query == "before=1004"
		default:
			t.Errorf("a request reached %s", r.Host)
		}
	}
	if !before || !outsideSeen {
		t.Errorf("messages paged with before=1004: %v; attachment on its host: %v", before, outsideSeen)
	}
}

func TestChatwootThrottle(t *testing.T) {
	for _, tc := range []struct {
		name, retry string
		wait        time.Duration
	}{{"no Retry-After", "", time.Minute}, {"Retry-After", "7", 7 * time.Second}} {
		t.Run(tc.name, func(t *testing.T) {
			srv, cfg, clock := fixtureConfig(t)
			srv.ThrottleChatwoot(1, tc.retry)
			w := chatwootReader(cfg, "1", helpdesktest.ChatwootToken)
			start := clock.Now()
			_, err := w.Tickets(context.Background(), helpdesk.Cursor{})
			var limit *helpdesk.LimitError
			if !errors.As(err, &limit) || !limit.Until.Equal(start.Add(tc.wait)) {
				t.Fatalf("429: %v, want a wait of %s from %s", err, tc.wait, start)
			}
			if _, err := w.Tickets(context.Background(), helpdesk.Cursor{}); err != nil {
				t.Errorf("after the wait: %v", err)
			}
			for _, r := range srv.Requests() {
				if r.Method != http.MethodGet {
					t.Errorf("%s was sent", r.Method)
				}
			}
		})
	}
}

func TestTokenHostMatchesPort(t *testing.T) {
	h := helpdesk.BaseURLHosts(helpdesk.Account{BaseURL: "https://chat.example.com"})
	other := helpdesk.BaseURLHosts(helpdesk.Account{BaseURL: "https://chat.example.com:8443/support"})
	for _, tc := range []struct {
		hosts helpdesk.Hosts
		url   string
		want  bool
	}{
		{h, "https://chat.example.com/a", true}, {h, "https://chat.example.com:443/a", true}, {h, "https://chat.example.com:8443/a", false},
		{h, "https://other.example.com/a", false}, {other, "https://chat.example.com:8443/a", true}, {other, "https://chat.example.com/a", false},
	} {
		u, _ := url.Parse(tc.url)
		if got := tc.hosts.AllowsURL(u); got != tc.want {
			t.Errorf("%+v %s: %v, want %v", tc.hosts, tc.url, got, tc.want)
		}
	}
}
