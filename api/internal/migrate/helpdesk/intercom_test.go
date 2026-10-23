package helpdesk_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/productdevbook/yuva/api/internal/migrate/helpdesk"
	"github.com/productdevbook/yuva/api/internal/migrate/helpdesk/helpdesktest"
)

func TestIntercomReaderAgainstFixtures(t *testing.T) {
	srv, cfg, _ := fixtureConfig(t)
	ctx := context.Background()
	budget := &helpdesk.Budget{}
	ic := helpdesk.NewIntercom(cfg, "us", func() string { return helpdesktest.IntercomToken }, budget)
	me, err := ic.Me(ctx)
	if err != nil || me.ID != "x7t2wq" || me.Email != "deniz@ic.example.com" {
		t.Fatalf("me: %+v %v", me, err)
	}
	folders, err := ic.Folders(ctx)
	if err != nil || len(folders) != 3 || folders[0].ID != "0" || folders[1].Name != "Billing" {
		t.Fatalf("folders: %+v %v", folders, err)
	}
	var tickets []helpdesk.Ticket
	c := helpdesk.Cursor{}
	for {
		page, err := ic.Tickets(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
		tickets = append(tickets, page.Tickets...)
		c = page.Next
		if page.End {
			break
		}
	}
	if len(tickets) != 6 {
		t.Fatalf("%d tickets, cursor %+v", len(tickets), c)
	}
	first, lead, renewal := tickets[0], tickets[1], tickets[2]
	if first.Status != helpdesk.StatusOpen || first.Folder != "3001" || first.Subject != "Charged twice" || first.Requester.Email != "ayse@ic-customer.example.org" ||
		first.Assignee == nil || first.Assignee.Email != "ece@ic.example.com" || !slices.Equal(first.Tags, []string{"billing", "vip"}) {
		t.Errorf("conversation 5001: %+v", first)
	}
	if lead.Status != helpdesk.StatusPending || lead.Folder != "0" || lead.Assignee != nil || lead.Requester.ID != "c-lead" || lead.Requester.Email != "" {
		t.Errorf("conversation 5002: %+v", lead)
	}
	if renewal.Status != helpdesk.StatusClosed || renewal.Folder != "3002" || renewal.Subject != "Your renewal" || renewal.Requester.ID != "u-103" || renewal.Assignee == nil {
		t.Errorf("conversation 5003: %+v", renewal)
	}

	msgs, err := ic.Messages(ctx, first)
	if err != nil || len(msgs) != 6 {
		t.Fatalf("messages: %d %v", len(msgs), err)
	}
	if msgs[0].Direction != "in" || msgs[0].Author.ID != "u-100" || len(msgs[0].Attachments) != 1 {
		t.Errorf("first message: %+v", msgs[0])
	}
	if msgs[1].Direction != "out" || msgs[1].Author.Email != "ece@ic.example.com" || len(msgs[1].Attachments) != 2 || !msgs[2].Note {
		t.Errorf("admin comment and note: %+v %+v", msgs[1], msgs[2])
	}
	if msgs[3].Direction != "in" || msgs[3].Author.ID != "u-101" || msgs[3].Author.Email != "bora@ic-customer.example.org" {
		t.Errorf("a second contact's comment keeps its own sender: %+v", msgs[3].Author)
	}
	if msgs[4].Direction != "out" || msgs[4].Author.Name != "Fin" {
		t.Errorf("bot comment: %+v", msgs[4])
	}
	if msgs[5].Direction != "in" || msgs[5].Author.ID != "u-100" || msgs[5].Author.Email != "ayse@ic-customer.example.org" {
		t.Errorf("a comment without a sender falls back to the requester: %+v", msgs[5].Author)
	}
	data, err := ic.Attachment(ctx, first, msgs[1].Attachments[0])
	if err != nil || len(data) == 0 {
		t.Errorf("attachment: %v", err)
	}
	var refused *helpdesk.RefusedError
	if _, err := ic.Attachment(ctx, first, msgs[1].Attachments[1]); !errors.As(err, &refused) {
		t.Errorf("an attachment on the API host: %v, want a refusal", err)
	}

	customer, err := ic.Customer(ctx, first.Requester)
	if err != nil || customer.Phone == "" || customer.ExternalID != "crm-100" || customer.Email != "ayse@ic-customer.example.org" {
		t.Errorf("customer: %+v %v", customer, err)
	}
	visitor, err := ic.Customer(ctx, lead.Requester)
	if err != nil || visitor.Name != "Visitor 77" || visitor.Email != "" {
		t.Errorf("a contact without e-mail: %+v %v", visitor, err)
	}

	cut, err := ic.Messages(ctx, tickets[4])
	var truncated *helpdesk.TruncatedError
	if !errors.As(err, &truncated) || truncated.Have != 2 || truncated.Total != 503 || len(cut) != 3 {
		t.Errorf("a conversation cut at the parts cap: %d messages, %v", len(cut), err)
	}

	reqs := srv.Requests()
	for _, r := range reqs {
		switch {
		case r.Method != http.MethodGet:
			t.Errorf("%s %s%s was sent", r.Method, r.Host, r.Path)
		case r.Host == "api.intercom.io" && (r.Auth != "Bearer "+helpdesktest.IntercomToken || r.Version != "2.17"):
			t.Errorf("%s carried Authorization %q and Intercom-Version %q", r.Path, r.Auth, r.Version)
		case r.Host != "api.intercom.io" && (r.Auth != "" || r.Version != ""):
			t.Errorf("%s%s received the token or the version header", r.Host, r.Path)
		case r.Host == helpdesktest.OutsideHost || r.Path == "/steal":
			t.Errorf("a link or an API-host attachment was fetched: %s%s", r.Host, r.Path)
		case r.Path == "/conversations" && !strings.Contains(r.Query, "per_page=150"):
			t.Errorf("listing without per_page=150: %s", r.Query)
		}
	}
	if !slices.ContainsFunc(reqs, func(r helpdesktest.Request) bool { return r.Host == helpdesktest.IntercomCDN }) {
		t.Error("the attachment was not fetched from the CDN")
	}
	for i, r := range reqs {
		n := 0
		for _, o := range reqs[:i+1] {
			if o.Host == "api.intercom.io" && o.At.After(r.At.Add(-helpdesk.IntercomWindow)) {
				n++
			}
		}
		if n > helpdesktest.IntercomLimit/2 {
			t.Errorf("the 10 seconds up to %s saw %d requests, want at most %d", r.At.Format(time.TimeOnly), n, helpdesktest.IntercomLimit/2)
		}
	}
	if budget.Limit != helpdesktest.IntercomLimit {
		t.Errorf("budget limit %d, want the X-RateLimit-Limit %d", budget.Limit, helpdesktest.IntercomLimit)
	}
}

func TestIntercomSyncReadsTheWholeListing(t *testing.T) {
	srv, cfg, _ := fixtureConfig(t)
	ctx := context.Background()
	last, err := helpdesktest.Fixture("intercom/conversations_3.json")
	if err != nil {
		t.Fatal(err)
	}
	srv.Override("intercom/conversations_3.json", []byte(strings.Replace(string(last), "1759020000", "1759999000", 1)))
	ic := helpdesk.NewIntercom(cfg, "eu", func() string { return helpdesktest.IntercomToken }, &helpdesk.Budget{})
	c := helpdesk.Cursor{ModifiedSince: time.Unix(1760000000-500000, 0)}
	var tickets []helpdesk.Ticket
	for {
		page, err := ic.Tickets(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
		tickets = append(tickets, page.Tickets...)
		c = page.Next
		if page.End {
			break
		}
	}
	if len(tickets) != 4 || tickets[3].ID != "5006" {
		t.Errorf("%d conversations changed, want 4 with the reopened 5006 from the last page: %+v", len(tickets), tickets)
	}
	for _, r := range srv.Requests() {
		if r.Host != "api.eu.intercom.io" {
			t.Errorf("a request reached %s", r.Host)
		}
	}
	if !slices.ContainsFunc(srv.Requests(), func(r helpdesktest.Request) bool { return strings.Contains(r.Query, "starting_after=ic-c2") }) {
		t.Error("the listing stopped before its last page")
	}
}

func TestIntercomTruncatesAtTheParts500(t *testing.T) {
	srv, cfg, _ := fixtureConfig(t)
	var parts []string
	for i := 0; i < 500; i++ {
		parts = append(parts, fmt.Sprintf(`{"id":"p%d","part_type":"comment","body":"<p>x</p>","created_at":1760000000,"author":{"type":"admin","id":"7001","name":"Deniz","email":"deniz@ic.example.com"}}`, i))
	}
	srv.Override("intercom/conversation_5004.json", []byte(`{"id":"5004","source":{"id":"1","body":"<p>hi</p>","author":{"type":"user","id":"u-100"}},"conversation_parts":{"total_count":500,"conversation_parts":[`+strings.Join(parts, ",")+`]}}`))
	ic := helpdesk.NewIntercom(cfg, "us", func() string { return helpdesktest.IntercomToken }, &helpdesk.Budget{})
	msgs, err := ic.Messages(context.Background(), helpdesk.Ticket{ID: "5004"})
	var cut *helpdesk.TruncatedError
	if !errors.As(err, &cut) || cut.Have != 500 || len(msgs) != 501 {
		t.Errorf("500 parts with total_count 500: %d messages, %v", len(msgs), err)
	}
}

func TestIntercomHonoursTheRateLimitReset(t *testing.T) {
	srv, cfg, _ := fixtureConfig(t)
	ic := helpdesk.NewIntercom(cfg, "us", func() string { return helpdesktest.IntercomToken }, &helpdesk.Budget{})
	srv.IntercomLimited = 1
	_, err := ic.Me(context.Background())
	var limit *helpdesk.LimitError
	if !errors.As(err, &limit) || limit.Until.Sub(cfg.Now()) != 4*time.Second {
		t.Fatalf("429: %v", err)
	}
	if _, err := ic.Me(context.Background()); err != nil {
		t.Fatal(err)
	}
	bad := helpdesk.NewIntercom(cfg, "us", func() string { return "wrong" }, &helpdesk.Budget{})
	var auth *helpdesk.AuthError
	if _, err := bad.Me(context.Background()); !errors.As(err, &auth) {
		t.Errorf("a wrong token: %v", err)
	}
}
