package helpdesk

import (
	"context"
	"errors"
	"net/http"
	"net/mail"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/inbucket/html2text"
)

const (
	freshdeskPerPage   = 100
	freshdeskMaxPages  = 300
	freshdeskDefault   = "default"
	freshdeskDescribed = "description"
	freshdeskWindow    = time.Second
	freshdeskForward   = 8
)

var freshdeskS3 = regexp.MustCompile(`^(?:[a-z0-9][a-z0-9.-]*\.)?s3[.-][a-z0-9-]+\.amazonaws\.com$`)

var freshdeskEpoch = time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC)

var freshdeskDesk = Desk{
	Source: "freshdesk", Name: "Freshdesk", Guide: "migrate-freshdesk", Auth: AuthToken, Account: AccountSubdomain, Admin: true,
	APIHosts: func(a Account) Hosts { return Hosts{Exact: []string{FreshdeskHost(a.Subdomain)}} },
	// The attachment_url examples in Freshdesk's API reference are on cdn.freshdesk.com, and its signed
	// downloads on s3.amazonaws.com; bucket-style S3 hosts are allowed too. The key is never sent there.
	FileHosts: func(Account) Hosts {
		return Hosts{Exact: []string{"cdn.freshdesk.com", "s3.amazonaws.com"}, Suffixes: []string{"s3.amazonaws.com"}, Pattern: freshdeskS3}
	},
	Send:   Send{Scheme: SendBasic, Password: "X"},
	Limits: []Limit{{Window: time.Minute, Default: 50, Headers: []string{"X-Ratelimit-Total"}}},
	// The lowest ticket-listing limit of any plan is 40 a minute.
	IncrementalMax: 20, RetryHeaders: []string{"Retry-After"}, AttributePrefix: "freshdesk", Sync: SyncModifiedSince,
	read: func(c *client, a Account) Source { return newFreshdesk(c, a.Subdomain) },
}

func FreshdeskHost(subdomain string) string { return subdomain + ".freshdesk.com" }

// FreshdeskBudget is a Freshdesk account's budget: half of its X-RateLimit-Total, and at most 20
// ticket-listing requests a minute.
func FreshdeskBudget(b *Budget) *Budget { return freshdeskDesk.Budget(b) }

// Freshdesk reads one Freshdesk account with an admin's API key. Freshdesk has no scopes and no
// read-only key: the GET-only transport is what keeps this reader from changing anything.
type Freshdesk struct {
	c        *client
	host     string
	perPage  int
	maxPages int
	agents   map[int64]Person
}

// NewFreshdesk reads subdomain.freshdesk.com. The key goes only to the account's host; attachments
// live on pre-signed URLs elsewhere and are fetched without it.
func NewFreshdesk(cfg Config, subdomain string, key func() string, budget *Budget) *Freshdesk {
	a := Account{Subdomain: subdomain}
	return newFreshdesk(freshdeskDesk.client(cfg, a, Credentials{Token: key}, budget), subdomain)
}

func newFreshdesk(c *client, subdomain string) *Freshdesk {
	return &Freshdesk{c: c, host: FreshdeskHost(subdomain), perPage: freshdeskPerPage, maxPages: freshdeskMaxPages}
}

// SetPaging changes the page size and the number of pages after which a listing starts over; for
// tests, which serve Freshdesk's 300-page cut in small pages.
func (f *Freshdesk) SetPaging(perPage, maxPages int) { f.perPage, f.maxPages = perPage, maxPages }

func (f *Freshdesk) url(path string, q url.Values) string {
	return withQuery("https://"+f.host+path, q)
}

func (f *Freshdesk) paged(page int, q url.Values) url.Values {
	if q == nil {
		q = url.Values{}
	}
	q.Set("per_page", strconv.Itoa(f.perPage))
	q.Set("page", strconv.Itoa(page))
	return q
}

type fdContact struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Email  string `json:"email"`
	Phone  string `json:"phone"`
	Mobile string `json:"mobile"`
}

func (c fdContact) person() Person {
	phone := c.Phone
	if phone == "" {
		phone = c.Mobile
	}
	return Person{ID: idString(c.ID), Name: strings.TrimSpace(c.Name), Email: strings.ToLower(strings.TrimSpace(c.Email)), Phone: phone}
}

type fdAgent struct {
	ID      int64     `json:"id"`
	RoleIDs []int64   `json:"role_ids"`
	Contact fdContact `json:"contact"`
}

func (a fdAgent) person() Person {
	p := a.Contact.person()
	p.ID, p.Agent = idString(a.ID), true
	return p
}

// Me is the agent the key belongs to; Admin when one of its roles is an administrator's. Only an
// admin may list roles, so a refused list means an agent who is not one. The ID is the account,
// so a reconnect with another admin's key still reaches the same account.
func (f *Freshdesk) Me(ctx context.Context) (Person, error) {
	var a fdAgent
	if err := f.c.getJSON(ctx, ClassAPI, f.url("/api/v2/agents/me", nil), &a); err != nil {
		return Person{}, err
	}
	p := a.person()
	p.ID = f.host
	var roles []struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	err := f.c.getJSON(ctx, ClassAPI, f.url("/api/v2/roles", nil), &roles)
	var se *StatusError
	if errors.As(err, &se) && se.Status == http.StatusForbidden {
		return p, nil
	}
	if err != nil {
		return Person{}, err
	}
	for _, r := range roles {
		if (r.Name == "Account Administrator" || r.Name == "Administrator") && slices.Contains(a.RoleIDs, r.ID) {
			p.Admin = true
		}
	}
	return p, nil
}

// Folders are the account's products and, first, the default for tickets that have none, with the
// support address Freshdesk receives each one's mail at.
func (f *Freshdesk) Folders(ctx context.Context) ([]Folder, error) {
	addresses := map[int64]string{}
	primary := map[int64]bool{}
	for page := 1; ; page++ {
		var configs []struct {
			ProductID *int64 `json:"product_id"`
			ReplyMail string `json:"reply_email"`
			Primary   bool   `json:"primary_role"`
			Active    bool   `json:"active"`
		}
		err := f.c.getJSON(ctx, ClassAPI, f.url("/api/v2/email_configs", f.paged(page, nil)), &configs)
		var se *StatusError
		if errors.As(err, &se) && se.Status == http.StatusNotFound {
			break
		}
		if err != nil {
			return nil, err
		}
		for _, c := range configs {
			id := valueOr(c.ProductID)
			if c.ReplyMail == "" || !c.Active || (addresses[id] != "" && (primary[id] || !c.Primary)) {
				continue
			}
			addresses[id], primary[id] = strings.ToLower(c.ReplyMail), c.Primary
		}
		if len(configs) < f.perPage {
			break
		}
	}
	out := []Folder{{ID: freshdeskDefault, Name: "No product", Address: addresses[0]}}
	for page := 1; ; page++ {
		var products []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		}
		if err := f.c.getJSON(ctx, ClassAPI, f.url("/api/v2/products", f.paged(page, nil)), &products); err != nil {
			return nil, err
		}
		for _, p := range products {
			out = append(out, Folder{ID: idString(p.ID), Name: p.Name, Address: addresses[p.ID]})
		}
		if len(products) < f.perPage {
			return out, nil
		}
	}
}

func (f *Freshdesk) Agents(ctx context.Context) ([]Person, error) {
	var out []Person
	for page := 1; ; page++ {
		var agents []fdAgent
		if err := f.c.getJSON(ctx, ClassAPI, f.url("/api/v2/agents", f.paged(page, nil)), &agents); err != nil {
			return nil, err
		}
		for _, a := range agents {
			out = append(out, a.person())
		}
		if len(agents) < f.perPage {
			return out, nil
		}
	}
}

func (f *Freshdesk) isAgent(ctx context.Context, id int64) (bool, error) {
	if _, err := f.agent(ctx, id); err != nil {
		return false, err
	}
	_, ok := f.agents[id]
	return ok, nil
}

func (f *Freshdesk) agent(ctx context.Context, id int64) (Person, error) {
	if f.agents == nil {
		list, err := f.Agents(ctx)
		if err != nil {
			return Person{}, err
		}
		f.agents = map[int64]Person{}
		for _, p := range list {
			n, _ := strconv.ParseInt(p.ID, 10, 64)
			f.agents[n] = p
		}
	}
	if p, ok := f.agents[id]; ok {
		return p, nil
	}
	return Person{ID: idString(id), Agent: true}, nil
}

type fdTicket struct {
	ID          int64      `json:"id"`
	Subject     string     `json:"subject"`
	Status      int        `json:"status"`
	Source      int        `json:"source"`
	ProductID   *int64     `json:"product_id"`
	ResponderID *int64     `json:"responder_id"`
	Tags        []string   `json:"tags"`
	Spam        bool       `json:"spam"`
	Deleted     bool       `json:"deleted"`
	Requester   *fdContact `json:"requester"`
	RequesterID int64      `json:"requester_id"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

var freshdeskVia = map[int]string{1: "email", 2: "portal", 3: "phone", 7: "chat", 9: "feedback_widget", 10: "outbound_email"}

func freshdeskStatus(t fdTicket) string {
	switch {
	case t.Deleted:
		return StatusDeleted
	case t.Spam:
		return StatusSpam
	}
	switch t.Status {
	case 3:
		return StatusPending
	case 4, 5:
		return StatusClosed
	}
	return StatusOpen
}

// freshdeskSeen is the tickets read within a second of the newest change seen, which a listing
// that starts over at that change can return again.
type freshdeskSeen map[string]int64

func parseFreshdeskSeen(s string) freshdeskSeen {
	out := freshdeskSeen{}
	for _, part := range strings.Split(s, ",") {
		id, at, ok := strings.Cut(part, "@")
		if n, err := strconv.ParseInt(at, 10, 64); ok && err == nil {
			out[id] = n
		}
	}
	return out
}

func (s freshdeskSeen) String() string {
	ids := make([]string, 0, len(s))
	for id := range s {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = id + "@" + strconv.FormatInt(s[id], 10)
	}
	return strings.Join(parts, ",")
}

// Tickets reads a page of tickets of every status oldest change first, changed since
// c.ModifiedSince. A ticket changed while the scan runs moves to the end of the listing and shifts
// the rest left, so a page number is not a place: every page starts the next listing at the newest
// change seen, and c.After names the tickets of that second to leave out. Only a page that lies
// wholly within one second goes on to the next page of the same listing, which Freshdesk ends at 300.
func (f *Freshdesk) Tickets(ctx context.Context, c Cursor) (TicketPage, error) {
	since := c.ModifiedSince
	if since.IsZero() {
		since = freshdeskEpoch
	}
	page := max(c.Page, 1)
	q := f.paged(page, url.Values{
		"updated_since": {since.UTC().Format(time.RFC3339)}, "order_by": {"updated_at"}, "order_type": {"asc"}, "include": {"requester"},
	})
	var list []fdTicket
	if err := f.c.getJSON(ctx, ClassIncremental, f.url("/api/v2/tickets", q), &list); err != nil {
		return TicketPage{}, err
	}
	seen := parseFreshdeskSeen(c.After)
	var tip int64
	for _, at := range seen {
		tip = max(tip, at)
	}
	out := TicketPage{Next: c, End: len(list) < f.perPage}
	out.Next.ModifiedSince, out.Next.Page = since, page+1
	for _, t := range list {
		id, at := idString(t.ID), t.UpdatedAt.UTC().Unix()
		_, dup := seen[id]
		if at > tip {
			tip = at
		}
		seen[id] = at
		if dup {
			continue
		}
		tk := Ticket{
			ID: idString(t.ID), Folder: freshdeskDefault, Subject: t.Subject, Status: freshdeskStatus(t), Tags: t.Tags, Via: freshdeskVia[t.Source],
			Requester: Person{ID: idString(t.RequesterID)}, CreatedAt: t.CreatedAt.UTC(), UpdatedAt: t.UpdatedAt.UTC(),
		}
		if t.ProductID != nil {
			tk.Folder = idString(*t.ProductID)
		}
		if t.Requester != nil {
			tk.Requester = t.Requester.person()
		}
		if t.ResponderID != nil {
			a, err := f.agent(ctx, *t.ResponderID)
			if err != nil {
				return TicketPage{}, err
			}
			tk.Assignee = &a
		}
		out.Tickets = append(out.Tickets, tk)
	}
	for id, at := range seen {
		if at < tip-int64(freshdeskWindow/time.Second) {
			delete(seen, id)
		}
	}
	out.Next.After = seen.String()
	if out.End {
		return out, nil
	}
	windowStart := tip - int64(freshdeskWindow/time.Second)
	inside := true
	for _, t := range list {
		if t.UpdatedAt.UTC().Unix() < windowStart {
			inside = false
		}
	}
	switch {
	case !inside:
		out.Next.ModifiedSince, out.Next.Page = time.Unix(windowStart, 0).UTC(), 1
	case page >= f.maxPages:
		out.Next.ModifiedSince, out.Next.Page = since.Add(freshdeskWindow), 1
	}
	return out, nil
}

type fdAttachment struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	FileSize    int64  `json:"file_size"`
	URL         string `json:"attachment_url"`
}

func freshdeskAttachments(message string, list []fdAttachment) []Attachment {
	var out []Attachment
	for _, a := range list {
		out = append(out, Attachment{
			Name: a.Name, Type: a.ContentType, Size: max(a.Size, a.FileSize), Ref: message + "|" + idString(a.ID) + "|" + a.URL,
		})
	}
	return out
}

func htmlText(body string) string {
	text, err := html2text.FromString(body, html2text.Options{TextOnly: true})
	if err != nil {
		return ""
	}
	return text
}

type fdConversation struct {
	ID          int64          `json:"id"`
	Body        string         `json:"body"`
	BodyText    string         `json:"body_text"`
	Incoming    bool           `json:"incoming"`
	Private     bool           `json:"private"`
	Source      int            `json:"source"`
	UserID      int64          `json:"user_id"`
	FromEmail   string         `json:"from_email"`
	ToEmails    []string       `json:"to_emails"`
	CcEmails    []string       `json:"cc_emails"`
	CreatedAt   time.Time      `json:"created_at"`
	Attachments []fdAttachment `json:"attachments"`
}

// Messages reads a ticket's description as its first message, then its conversations: customer
// replies in, agent replies out, private notes and an agent's public notes (which Freshdesk does
// not mail to the requester) as notes. A customer's public note is a message in.
func (f *Freshdesk) Messages(ctx context.Context, t Ticket) ([]Message, error) {
	first, thread, err := f.description(ctx, t)
	if err != nil {
		return nil, err
	}
	var all []fdConversation
	for page := 1; page <= f.maxPages; page++ {
		var list []fdConversation
		path := "/api/v2/tickets/" + url.PathEscape(t.ID) + "/conversations"
		if err := f.c.getJSON(ctx, ClassAPI, f.url(path, f.paged(page, nil)), &list); err != nil {
			return nil, err
		}
		all = append(all, list...)
		if len(list) < f.perPage {
			break
		}
	}
	for _, c := range all {
		if c.Source != freshdeskForward {
			for _, a := range append(slices.Clone(c.ToEmails), c.CcEmails...) {
				thread[strings.ToLower(strings.TrimSpace(a))] = true
			}
		}
	}
	out := []Message{first}
	for _, c := range all {
		m, err := f.conversation(ctx, t, c, thread)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

// description reads the ticket's first message and the addresses on the customer's side of the
// thread: its requester and what the ticket copies (not what an agent forwarded it to).
func (f *Freshdesk) description(ctx context.Context, t Ticket) (Message, map[string]bool, error) {
	var d struct {
		CcEmails        []string       `json:"cc_emails"`
		ReplyCcEmails   []string       `json:"reply_cc_emails"`
		Description     string         `json:"description"`
		DescriptionText string         `json:"description_text"`
		Source          int            `json:"source"`
		CreatedAt       time.Time      `json:"created_at"`
		Attachments     []fdAttachment `json:"attachments"`
	}
	if err := f.c.getJSON(ctx, ClassAPI, f.url("/api/v2/tickets/"+url.PathEscape(t.ID), nil), &d); err != nil {
		return Message{}, nil, err
	}
	thread := map[string]bool{}
	for _, a := range append(append([]string{t.Requester.Email}, d.CcEmails...), d.ReplyCcEmails...) {
		if a = strings.ToLower(strings.TrimSpace(a)); a != "" {
			thread[a] = true
		}
	}
	m := Message{
		ID: freshdeskDescribed, Direction: "in", Author: t.Requester, At: d.CreatedAt.UTC(), Text: d.DescriptionText, HTML: d.Description,
		Attachments: freshdeskAttachments(freshdeskDescribed, d.Attachments),
	}
	if m.At.IsZero() {
		m.At = t.CreatedAt
	}
	if m.Text == "" {
		m.Text = htmlText(d.Description)
	}
	if d.Source == 10 {
		m.Direction, m.Author = "out", Person{Agent: true}
		if t.Assignee != nil {
			m.Author = *t.Assignee
		}
	}
	return m, thread, nil
}

func (f *Freshdesk) conversation(ctx context.Context, t Ticket, c fdConversation, thread map[string]bool) (Message, error) {
	m := Message{
		ID: idString(c.ID), At: c.CreatedAt.UTC(), Text: c.BodyText, HTML: c.Body, Attachments: freshdeskAttachments(idString(c.ID), c.Attachments),
	}
	if m.Text == "" {
		m.Text = htmlText(c.Body)
	}
	agent, err := f.isAgent(ctx, c.UserID)
	if err != nil {
		return Message{}, err
	}
	if c.Incoming && !c.Private && !agent && c.Source != freshdeskForward {
		sender, serr := mail.ParseAddress(c.FromEmail)
		fromRequester := (c.UserID != 0 && idString(c.UserID) == t.Requester.ID) ||
			(c.UserID == 0 && (serr != nil || strings.EqualFold(sender.Address, t.Requester.Email)))
		if fromRequester {
			m.Direction, m.Author = "in", t.Requester
			return m, nil
		}
		m.Author = Person{ID: idString(c.UserID)}
		if serr == nil {
			m.Author.Name, m.Author.Email = sender.Name, strings.ToLower(sender.Address)
		}
		if thread[m.Author.Email] {
			m.Direction = "in"
		} else {
			m.Note = true
		}
		return m, nil
	}
	author, err := f.agent(ctx, c.UserID)
	if err != nil {
		return Message{}, err
	}
	m.Author = author
	if c.Private || c.Source == 2 || c.Source == freshdeskForward {
		m.Note = true
		return m, nil
	}
	m.Direction = "out"
	return m, nil
}

// Attachment fetches a pre-signed file URL without the key. Freshdesk's URLs expire, so a refused
// one is read again from the ticket or conversation that holds it, once, for a fresh URL.
func (f *Freshdesk) Attachment(ctx context.Context, t Ticket, a Attachment) ([]byte, error) {
	message, rest, _ := strings.Cut(a.Ref, "|")
	id, link, _ := strings.Cut(rest, "|")
	u, err := url.Parse(link)
	if err != nil || u.Scheme != "https" || strings.EqualFold(u.Hostname(), f.host) {
		return nil, &RefusedError{Method: "GET", Host: link}
	}
	data, err := f.c.get(ctx, f.c.files, ClassAPI, u.String(), "*/*")
	var se *StatusError
	if !errors.As(err, &se) || (se.Status != http.StatusForbidden && se.Status != http.StatusNotFound && se.Status != http.StatusGone) {
		return data, err
	}
	fresh, ferr := f.freshURL(ctx, t, message, id)
	if ferr != nil {
		return nil, ferr
	}
	if fresh == "" || fresh == link {
		return nil, err
	}
	next, perr := url.Parse(fresh)
	if perr != nil || next.Scheme != "https" || strings.EqualFold(next.Hostname(), f.host) {
		return nil, err
	}
	return f.c.get(ctx, f.c.files, ClassAPI, next.String(), "*/*")
}

func (f *Freshdesk) freshURL(ctx context.Context, t Ticket, message, attachment string) (string, error) {
	find := func(list []fdAttachment) string {
		for _, a := range list {
			if idString(a.ID) == attachment {
				return a.URL
			}
		}
		return ""
	}
	if message == freshdeskDescribed {
		var d struct {
			Attachments []fdAttachment `json:"attachments"`
		}
		if err := f.c.getJSON(ctx, ClassAPI, f.url("/api/v2/tickets/"+url.PathEscape(t.ID), nil), &d); err != nil {
			return "", err
		}
		return find(d.Attachments), nil
	}
	for page := 1; page <= f.maxPages; page++ {
		var list []fdConversation
		path := "/api/v2/tickets/" + url.PathEscape(t.ID) + "/conversations"
		if err := f.c.getJSON(ctx, ClassAPI, f.url(path, f.paged(page, nil)), &list); err != nil {
			return "", err
		}
		for _, c := range list {
			if idString(c.ID) == message {
				return find(c.Attachments), nil
			}
		}
		if len(list) < f.perPage {
			break
		}
	}
	return "", nil
}

// Customer: the ticket listing already names the requester in full.
func (f *Freshdesk) Customer(_ context.Context, p Person) (Person, error) { return p, nil }
