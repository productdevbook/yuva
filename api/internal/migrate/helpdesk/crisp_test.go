package helpdesk_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/productdevbook/yuva/api/internal/migrate/helpdesk"
	"github.com/productdevbook/yuva/api/internal/migrate/helpdesk/helpdesktest"
)

func crispAccount() helpdesk.Account { return helpdesk.Account{Website: helpdesktest.CrispWebsite} }

func crispCreds() helpdesk.Credentials {
	return helpdesk.Credentials{Token: func() string { return helpdesktest.CrispIdentifier }, Secret: helpdesktest.CrispKey}
}

func crispTickets(t *testing.T, ctx context.Context, cr helpdesk.Source) ([]helpdesk.Ticket, helpdesk.Cursor) {
	t.Helper()
	var out []helpdesk.Ticket
	c := helpdesk.Cursor{}
	for {
		page, err := cr.Tickets(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, page.Tickets...)
		c = page.Next
		if page.End {
			return out, c
		}
	}
}

func TestCrispReaderAgainstFixtures(t *testing.T) {
	srv, cfg, _ := fixtureConfig(t)
	ctx := context.Background()
	srv.CrispPageSize = 3
	budget := &helpdesk.Budget{}
	cr := helpdesk.NewCrisp(cfg, helpdesktest.CrispWebsite, helpdesktest.CrispIdentifier, helpdesktest.CrispKey, budget)

	me, err := cr.Me(ctx)
	if err != nil || me.ID != helpdesktest.CrispWebsite {
		t.Fatalf("me: %+v %v", me, err)
	}
	folders, err := cr.Folders(ctx)
	if err != nil || len(folders) != 1 || folders[0].Name != "Acme Shop" || folders[0].ID != helpdesktest.CrispWebsite {
		t.Fatalf("folders: %+v %v", folders, err)
	}
	agents, err := cr.Agents(ctx)
	if err != nil || len(agents) != 2 {
		t.Fatalf("agents: %+v %v", agents, err)
	}

	tickets, cursor := crispTickets(t, ctx, cr)
	if len(tickets) != 5 || cursor.Start != tickets[4].UpdatedAt.UnixMilli() || !slices.Equal(cursor.Seen, []string{"session_cr005"}) || cursor.Order != "desc" {
		t.Fatalf("%d tickets, cursor %+v", len(tickets), cursor)
	}
	if want := tickets[0].UpdatedAt.Format(time.RFC3339); cursor.After != want {
		t.Errorf("newest update seen %q, want %q", cursor.After, want)
	}
	ada, bob, visitor := tickets[0], tickets[1], tickets[2]
	if ada.Status != helpdesk.StatusOpen || bob.Status != helpdesk.StatusClosed || visitor.Status != helpdesk.StatusOpen {
		t.Errorf("statuses: %s %s %s", ada.Status, bob.Status, visitor.Status)
	}
	if ada.Requester.Email != "ada@example.org" || ada.Requester.Name != "Ada Example" || ada.Requester.Phone != "+900000000001" || ada.Requester.ID != "p-100" ||
		ada.Assignee == nil || ada.Assignee.Email != "deniz@acme.example.com" || !slices.Equal(ada.Tags, []string{"vip", "login"}) ||
		ada.Subject != "Cannot log in" || ada.Folder != helpdesktest.CrispWebsite {
		t.Errorf("conversation 1: %+v", ada)
	}
	if bob.Assignee == nil || bob.Assignee.Name != "Ece Sample" {
		t.Errorf("conversation 2 assignee: %+v", bob.Assignee)
	}
	if visitor.Requester.Email != "" || visitor.Requester.ID != "session_cr003" || visitor.Requester.Name != "visitor 4821" || visitor.Assignee != nil {
		t.Errorf("anonymous visitor: %+v", visitor)
	}

	msgs, err := cr.Messages(ctx, ada)
	if err != nil || len(msgs) != 6 {
		t.Fatalf("messages: %d %v", len(msgs), err)
	}
	slices.SortFunc(msgs, func(a, b helpdesk.Message) int { return a.At.Compare(b.At) })
	if msgs[0].Text != "Hello" || msgs[1].Text != "I cannot log in." || msgs[0].Direction != "in" || msgs[0].Author.ID != "p-100" {
		t.Errorf("older batch: %+v", msgs[:2])
	}
	if !strings.Contains(msgs[2].Text, "https://files.example.org/leak.png") || len(msgs[3].Attachments) != 1 || msgs[3].Attachments[0].Name != "login.png" {
		t.Errorf("newer batch: %+v", msgs[2:4])
	}
	if !msgs[4].Note || msgs[4].Direction != "out" || msgs[4].Author.Email != "deniz@acme.example.com" || msgs[5].Note || msgs[5].Direction != "out" {
		t.Errorf("operator messages: %+v", msgs[4:])
	}
	for _, a := range msgs[3].Attachments {
		if data, err := cr.Attachment(ctx, ada, a); err != nil || len(data) == 0 {
			t.Errorf("attachment %s: %v", a.Name, err)
		}
	}

	colleagues, err := cr.Messages(ctx, bob)
	if err != nil || len(colleagues) != 5 {
		t.Fatalf("messages of 2: %d %v", len(colleagues), err)
	}
	if colleagues[0].Author.ID != "p-101" || colleagues[0].Author.Email != "bob@example.net" {
		t.Errorf("the conversation's person is the author of their own message: %+v", colleagues[0].Author)
	}
	if a := colleagues[1].Author; a.ID != "part-77" || a.Name != "Carol Third" || a.Email != "" {
		t.Errorf("a second participant is its own author: %+v", a)
	}
	for _, m := range colleagues[2:4] {
		for _, a := range m.Attachments {
			if _, err := cr.Attachment(ctx, bob, a); err == nil {
				t.Errorf("%s was fetched from outside Crisp's storage", a.Ref)
			} else if r := new(helpdesk.RefusedError); !errors.As(err, &r) {
				t.Errorf("%s: %v, want a refusal", a.Ref, err)
			}
		}
	}

	anon, err := cr.Messages(ctx, visitor)
	if err != nil || len(anon) != 2 {
		t.Fatalf("messages of 3: %d %v", len(anon), err)
	}
	for _, m := range anon {
		if m.Author.ID != "session_cr003" || m.Author.Name != "visitor 4821" {
			t.Errorf("a message without its sender falls back to the conversation's person: %+v", m.Author)
		}
	}

	reqs := srv.Requests()
	onlyGETs(t, reqs, "")
	withinBudget(t, reqs, "api.crisp.chat", 60, 0)
	basic := basicHeader(helpdesktest.CrispIdentifier, helpdesktest.CrispKey)
	var before []string
	var storage int
	for _, r := range reqs {
		switch r.Host {
		case "api.crisp.chat":
			if r.Auth != basic {
				t.Errorf("%s was fetched with %q", r.Path, r.Auth)
			}
			if strings.HasSuffix(r.Path, "/session_cr001/messages") {
				before = append(before, r.Query)
			}
		case helpdesktest.CrispStorage:
			storage++
			if r.Auth != "" {
				t.Errorf("the file on %s was fetched with %q", r.Host, r.Auth)
			}
		default:
			t.Errorf("a request reached %s%s", r.Host, r.Path)
		}
	}
	if storage != 1 {
		t.Errorf("%d files fetched from Crisp's storage, want 1", storage)
	}
	if !slices.Equal(before, []string{"", "timestamp_before=1791198000001", "timestamp_before=1790845200001"}) {
		t.Errorf("message batches were requested with %q", before)
	}
	if budget.Limit != 0 {
		t.Errorf("Crisp sends no limit headers, but the budget limit is %d", budget.Limit)
	}
}

func basicHeader(user, password string) string {
	r, _ := http.NewRequest(http.MethodGet, "https://x.example", nil)
	r.SetBasicAuth(user, password)
	return r.Header.Get("Authorization")
}

func TestCrispSyncListsChangesSince(t *testing.T) {
	srv, cfg, _ := fixtureConfig(t)
	cr := helpdesk.NewCrisp(cfg, helpdesktest.CrispWebsite, helpdesktest.CrispIdentifier, helpdesktest.CrispKey, &helpdesk.Budget{})
	since := time.UnixMilli(1791198000000).Add(-time.Minute)
	page, err := cr.Tickets(context.Background(), helpdesk.Cursor{ModifiedSince: since})
	if err != nil || len(page.Tickets) != 1 || page.Tickets[0].ID != "session_cr001" || page.End {
		t.Fatalf("changes: %+v %v", page, err)
	}
	reqs := srv.Requests()
	if want := "filter_date_start=" + url.QueryEscape(since.UTC().Format(time.RFC3339)); len(reqs) == 0 || !strings.Contains(reqs[0].Query, want) {
		t.Errorf("requests %+v, want the query to hold %s", reqs, want)
	}
}

func TestCrispWrongTokenIsRefused(t *testing.T) {
	_, cfg, _ := fixtureConfig(t)
	cr := helpdesk.NewCrisp(cfg, helpdesktest.CrispWebsite, helpdesktest.CrispIdentifier, "stale", &helpdesk.Budget{})
	_, err := cr.Me(context.Background())
	var auth *helpdesk.AuthError
	if !errors.As(err, &auth) {
		t.Fatalf("%v, want an AuthError", err)
	}
}

// Crisp sends no limit headers: a 420 waits a minute and halves the pace from then on.
func TestCrispThrottleHalvesThePace(t *testing.T) {
	srv, cfg, clock := fixtureConfig(t)
	budget := helpdesk.CrispBudget(&helpdesk.Budget{})
	cr := helpdesk.NewCrisp(cfg, helpdesktest.CrispWebsite, helpdesktest.CrispIdentifier, helpdesktest.CrispKey, budget)
	srv.CrispThrottle = 1
	start := clock.Now()
	_, err := cr.Me(context.Background())
	var limit *helpdesk.LimitError
	if !errors.As(err, &limit) || limit.Until.Sub(start) != time.Minute {
		t.Fatalf("%v, want a wait of a minute", err)
	}
	if before, after := 60, budget.PerWindow(); after != before/2 {
		t.Errorf("pace %d a minute after the 420, want %d", after, before/2)
	}
	srv.CrispThrottle = 1
	_, _ = cr.Me(context.Background())
	if got := budget.PerWindow(); got != 15 {
		t.Errorf("pace %d after a second 420, want 15", got)
	}
	_ = cfg.Sleep(context.Background(), 59*time.Minute)
	if _, err := cr.Me(context.Background()); err != nil || budget.PerWindow() != 15 {
		t.Errorf("pace %d, %v: the pace came back before an hour without a limit answer", budget.PerWindow(), err)
	}
	_ = cfg.Sleep(context.Background(), 2*time.Minute)
	if _, err := cr.Me(context.Background()); err != nil || budget.PerWindow() != 60 {
		t.Errorf("pace %d, %v after an hour without a limit answer, want 60", budget.PerWindow(), err)
	}
}

// A 429 from Crisp's storage host asks for a wait but does not slow the API pace.
func TestCrispFileLimitDoesNotHalveThePace(t *testing.T) {
	srv, cfg, _ := fixtureConfig(t)
	ctx := context.Background()
	budget := helpdesk.CrispBudget(&helpdesk.Budget{})
	cr := helpdesk.NewCrisp(cfg, helpdesktest.CrispWebsite, helpdesktest.CrispIdentifier, helpdesktest.CrispKey, budget)
	tickets, _ := crispTickets(t, ctx, cr)
	msgs, err := cr.Messages(ctx, tickets[0])
	if err != nil {
		t.Fatal(err)
	}
	var file helpdesk.Attachment
	for _, m := range msgs {
		if len(m.Attachments) > 0 {
			file = m.Attachments[0]
		}
	}
	srv.CrispFilesThrottle = 1
	var limit *helpdesk.LimitError
	if _, err := cr.Attachment(ctx, tickets[0], file); !errors.As(err, &limit) {
		t.Fatalf("%v, want a wait", err)
	}
	if got := budget.PerWindow(); got != 60 {
		t.Errorf("pace %d after a storage 429, want 60", got)
	}
}

// A conversation deleted on a page already read shifts the list; the walk does not skip one, and
// a reader started again from the stored cursor goes on from the same place, whichever end of the
// list Crisp starts from.
func TestCrispListingSurvivesDeletionAndResume(t *testing.T) {
	for _, tc := range []struct {
		name      string
		ascending bool
		file      string
		drop      string
	}{
		{"newest first", false, "crisp/conversations_1.json", "session_cr001"},
		{"oldest first", true, "crisp/conversations_2.json", "session_cr005"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, cfg, _ := fixtureConfig(t)
			ctx := context.Background()
			srv.CrispPageSize, srv.CrispAscending = 2, tc.ascending
			open := func() *helpdesk.Crisp {
				return helpdesk.NewCrisp(cfg, helpdesktest.CrispWebsite, helpdesktest.CrispIdentifier, helpdesktest.CrispKey, &helpdesk.Budget{})
			}
			page, err := open().Tickets(ctx, helpdesk.Cursor{})
			if err != nil || len(page.Tickets) != 2 {
				t.Fatalf("first page: %+v %v", page, err)
			}
			read := []string{page.Tickets[0].ID, page.Tickets[1].ID}
			if !slices.Contains(read, tc.drop) {
				t.Fatalf("the first page %v does not hold the conversation to delete, %s", read, tc.drop)
			}

			raw, err := helpdesktest.Fixture(tc.file)
			if err != nil {
				t.Fatal(err)
			}
			var env struct {
				Data []map[string]any `json:"data"`
			}
			if err := json.Unmarshal(raw, &env); err != nil {
				t.Fatal(err)
			}
			env.Data = slices.DeleteFunc(env.Data, func(c map[string]any) bool { return c["session_id"] == tc.drop })
			kept, _ := json.Marshal(env)
			srv.Override(tc.file, kept)

			stored, _ := json.Marshal(page.Next)
			var c helpdesk.Cursor
			if err := json.Unmarshal(stored, &c); err != nil {
				t.Fatal(err)
			}
			for {
				page, err := open().Tickets(ctx, c)
				if err != nil {
					t.Fatal(err)
				}
				for _, tk := range page.Tickets {
					read = append(read, tk.ID)
				}
				c = page.Next
				if page.End {
					break
				}
			}
			want := []string{"session_cr001", "session_cr002", "session_cr003", "session_cr004", "session_cr005"}
			slices.Sort(read)
			if !slices.Equal(read, want) {
				t.Errorf("read %v, want each conversation once: %v", read, want)
			}
		})
	}
}

// A page that contradicts the order the walk found, or a full page that adds nothing, is an error
// and not the end of the listing.
func TestCrispListingNeverEndsSilently(t *testing.T) {
	_, cfg, _ := fixtureConfig(t)
	cr := helpdesk.NewCrisp(cfg, helpdesktest.CrispWebsite, helpdesktest.CrispIdentifier, helpdesktest.CrispKey, &helpdesk.Budget{})
	if _, err := cr.Tickets(context.Background(), helpdesk.Cursor{Order: "asc"}); err == nil {
		t.Error("a newest-first page was taken for an oldest-first walk")
	}
}

// Messages that share a millisecond across a batch boundary are all read, and the loop ends.
func TestCrispMessagesSharingATimestamp(t *testing.T) {
	srv, cfg, _ := fixtureConfig(t)
	ctx := context.Background()
	t1 := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC).UnixMilli()
	t2 := time.Date(2026, 10, 1, 8, 0, 1, 0, time.UTC).UnixMilli()
	t3 := time.Date(2026, 10, 1, 8, 0, 2, 0, time.UTC).UnixMilli()
	batch := func(ms ...[2]int64) []byte {
		var list []map[string]any
		for _, m := range ms {
			list = append(list, map[string]any{"type": "text", "from": "user", "content": "message " + strconv.FormatInt(m[0], 10), "fingerprint": m[0], "timestamp": m[1]})
		}
		b, _ := json.Marshal(map[string]any{"error": false, "reason": "listed", "data": list})
		return b
	}
	srv.Override("crisp/messages_session_cr004.json", batch([2]int64{3, t2}, [2]int64{4, t2}, [2]int64{5, t3}))
	srv.Override("crisp/messages_session_cr004_"+strconv.FormatInt(t2+1, 10)+".json", batch([2]int64{1, t1}, [2]int64{2, t2}, [2]int64{3, t2}, [2]int64{4, t2}))
	cr := helpdesk.NewCrisp(cfg, helpdesktest.CrispWebsite, helpdesktest.CrispIdentifier, helpdesktest.CrispKey, &helpdesk.Budget{})
	msgs, err := cr.Messages(ctx, helpdesk.Ticket{ID: "session_cr004", Requester: helpdesk.Person{ID: "p-103"}})
	if err != nil || len(msgs) != 5 {
		t.Fatalf("%d messages, %v, want 5", len(msgs), err)
	}
}

// The daily class stops the reader at its share of the day and the next day goes on, on the test
// clock.
func TestCrispDailyBudgetResumesNextDay(t *testing.T) {
	srv, cfg, clock := fixtureConfig(t)
	desk, ok := helpdesk.Lookup("crisp")
	if !ok {
		t.Fatal("no crisp desk")
	}
	const perDay = 6
	desk.Limits = []helpdesk.Limit{{Window: time.Minute, Default: 1000}, {Window: helpdesk.Day, Default: perDay}}
	budget := &helpdesk.Budget{}
	cr := desk.Open(cfg, crispAccount(), crispCreds(), budget)
	ctx := context.Background()
	start := clock.Now()
	dayTwo := start.Add(helpdesk.Day)

	waits := 0
	run := func(op func() error) {
		t.Helper()
		for {
			err := op()
			var wait *helpdesk.WaitError
			if errors.As(err, &wait) {
				if !wait.Daily {
					t.Fatalf("waited for a window: %v", err)
				}
				waits++
				_ = cfg.Sleep(ctx, wait.Until.Sub(cfg.Now()))
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	var tickets []helpdesk.Ticket
	c := helpdesk.Cursor{}
	for done := false; !done; {
		run(func() error {
			page, err := cr.Tickets(ctx, c)
			if err != nil {
				return err
			}
			tickets, c, done = append(tickets, page.Tickets...), page.Next, page.End
			return nil
		})
	}
	for _, tk := range tickets {
		run(func() error { _, err := cr.Messages(ctx, tk); return err })
	}
	if waits == 0 {
		t.Fatal("the daily budget never ran out")
	}
	var first, second int
	for _, r := range srv.Requests() {
		if r.Method != http.MethodGet {
			t.Errorf("%s %s was sent", r.Method, r.Path)
		}
		if r.At.Before(dayTwo.Add(-time.Second)) && r.At.Sub(start) < helpdesk.Day {
			first++
		} else {
			second++
		}
	}
	if first > perDay || first == 0 || second == 0 {
		t.Errorf("%d requests on the first day (at most %d), %d after it", first, perDay, second)
	}
	if len(tickets) != 5 {
		t.Errorf("%d conversations read", len(tickets))
	}
}
