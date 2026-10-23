package helpdesk

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// CrispAPIHost is Crisp's REST API, the only host the token is sent to.
const CrispAPIHost = "api.crisp.chat"

// crispStorageHost keeps the files visitors and operators upload; the token never goes there.
const crispStorageHost = "storage.crisp.chat"

const crispPerPage = 50

// CrispBudget is a Crisp website's budget: half of the website token's 10,000 requests a day, and
// a pace of its own, halved whenever Crisp answers 429 or 420.
func CrispBudget(b *Budget) *Budget { return crispDesk.Budget(b) }

// Crisp reads one Crisp website with its website token. Crisp has no read-only scope for a
// website token: the GET-only transport is what keeps this reader from changing anything.
type Crisp struct {
	c       *client
	website string

	mu  sync.Mutex
	ops map[string]Person
}

// NewCrisp reads the website with its token's identifier and key.
func NewCrisp(cfg Config, website, identifier, key string, budget *Budget) *Crisp {
	creds := Credentials{Token: func() string { return identifier }, Secret: key}
	a := Account{Website: website}
	return &Crisp{website: website, c: crispDesk.client(cfg, a, creds, budget)}
}

func (r *Crisp) url(path string, q url.Values) string {
	return withQuery("https://"+CrispAPIHost+"/v1/website/"+url.PathEscape(r.website)+path, q)
}

func (r *Crisp) get(ctx context.Context, path string, q url.Values, data any) error {
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := r.c.getJSON(ctx, ClassAPI, r.url(path, q), &env); err != nil {
		return err
	}
	if len(env.Data) == 0 || string(env.Data) == "null" {
		return nil
	}
	return json.Unmarshal(env.Data, data)
}

func (r *Crisp) Me(ctx context.Context) (Person, error) {
	var w struct {
		Name string `json:"name"`
	}
	if err := r.get(ctx, "", nil, &w); err != nil {
		return Person{}, err
	}
	return Person{ID: r.website, Name: w.Name}, nil
}

// Folders is the website itself.
func (r *Crisp) Folders(ctx context.Context) ([]Folder, error) {
	var w struct {
		Name   string `json:"name"`
		Domain string `json:"domain"`
	}
	if err := r.get(ctx, "", nil, &w); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(w.Name)
	if name == "" {
		name = w.Domain
	}
	return []Folder{{ID: r.website, Name: name}}, nil
}

type crispOperator struct {
	Details struct {
		UserID    string `json:"user_id"`
		Name      string `json:"name"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
		Email     string `json:"email"`
		Role      string `json:"role"`
	} `json:"details"`
}

func (o crispOperator) person() Person {
	d := o.Details
	name := strings.TrimSpace(d.Name)
	if name == "" {
		name = strings.TrimSpace(d.FirstName + " " + d.LastName)
	}
	return Person{ID: d.UserID, Name: name, Email: strings.ToLower(strings.TrimSpace(d.Email)), Agent: true, Admin: d.Role == "owner"}
}

func (r *Crisp) operators(ctx context.Context) (map[string]Person, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ops != nil {
		return r.ops, nil
	}
	var list []crispOperator
	err := r.get(ctx, "/operators/list", nil, &list)
	var se *StatusError
	if errors.As(err, &se) && (se.Status == http.StatusForbidden || se.Status == http.StatusNotFound) {
		err = nil
	}
	if err != nil {
		return nil, err
	}
	r.ops = map[string]Person{}
	for _, o := range list {
		if o.Details.UserID != "" {
			r.ops[o.Details.UserID] = o.person()
		}
	}
	return r.ops, nil
}

func (r *Crisp) Agents(ctx context.Context) ([]Person, error) {
	ops, err := r.operators(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Person, 0, len(ops))
	for _, p := range ops {
		out = append(out, p)
	}
	return out, nil
}

type crispConversation struct {
	SessionID string `json:"session_id"`
	PeopleID  string `json:"people_id"`
	State     string `json:"state"`
	Topic     string `json:"topic"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
	Assigned  *struct {
		UserID string `json:"user_id"`
	} `json:"assigned"`
	Segments []string `json:"segments"`
	Meta     struct {
		Nickname string   `json:"nickname"`
		Email    string   `json:"email"`
		Phone    string   `json:"phone"`
		Subject  string   `json:"subject"`
		Segments []string `json:"segments"`
	} `json:"meta"`
}

func crispStatus(state string) string {
	if state == "resolved" {
		return StatusClosed
	}
	return StatusOpen
}

func crispTime(ms int64) time.Time { return time.UnixMilli(ms).UTC() }

const (
	crispDesc = "desc"
	crispAsc  = "asc"
)

var errCrispOrder = errors.New("Crisp's list of conversations changed order or stopped making progress; the listing cannot be read reliably")

// Tickets reads a page of conversations ordered by last update, changed since c.ModifiedSince when
// it is set. The walk depends neither on list positions nor on which end Crisp starts from: the
// first page shows whether the list runs newest first or oldest first (c.Order), and from then on
// each request asks for page 1 of what lies beyond the edge read so far (c.Start, milliseconds),
// 1 ms inside it, dropping the ids already read at that instant. A deletion or an update between
// two requests, or a resume, therefore loses nothing. A page whose conversations were all read
// goes on to the next page of the same window, and the listing ends only at an empty page; a full
// page that adds nothing new, or a page that contradicts the order, is an error. Until the order
// is known (a single conversation, or equal times) the walk goes by page number, which is safe
// for that few. Next.After is the newest update seen so far.
func (r *Crisp) Tickets(ctx context.Context, c Cursor) (TicketPage, error) {
	page := max(c.Page, 1)
	q := url.Values{"per_page": {strconv.Itoa(crispPerPage)}, "order_date_updated": {"true"}}
	if !c.ModifiedSince.IsZero() {
		q.Set("filter_date_start", c.ModifiedSince.UTC().Format(time.RFC3339))
	}
	const iso = "2006-01-02T15:04:05.000Z"
	switch {
	case c.Order == crispDesc && c.Start > 0:
		q.Set("filter_date_end", crispTime(c.Start+1).Format(iso))
	case c.Order == crispAsc && c.Start > 0:
		q.Set("filter_date_start", crispTime(c.Start-1).Format(iso))
	}
	var list []crispConversation
	if err := r.get(ctx, "/conversations/"+strconv.Itoa(page), q, &list); err != nil {
		return TicketPage{}, err
	}
	order := c.Order
	if len(list) > 1 {
		first, last := list[0].UpdatedAt, list[len(list)-1].UpdatedAt
		switch {
		case order == "" && first > last:
			order = crispDesc
		case order == "" && first < last:
			order = crispAsc
		case order == crispDesc && first < last, order == crispAsc && first > last:
			return TicketPage{}, errCrispOrder
		}
	}
	read := map[string]bool{}
	for _, id := range c.Seen {
		read[id] = true
	}
	var fresh []crispConversation
	for _, cv := range list {
		if !read[cv.SessionID] {
			fresh = append(fresh, cv)
		}
	}
	if len(list) >= crispPerPage && len(fresh) == 0 {
		return TicketPage{}, errCrispOrder
	}
	out := TicketPage{Next: c, End: len(list) == 0}
	out.Next.Page, out.Next.Order, out.Next.Seen = 1, order, slices.Clone(c.Seen)
	edge := c.Start
	if order != "" {
		for _, cv := range list {
			if edge == 0 || (order == crispDesc && cv.UpdatedAt < edge) || (order == crispAsc && cv.UpdatedAt > edge) {
				edge = cv.UpdatedAt
			}
		}
	}
	switch {
	case order == "":
		out.Next.Page = page + 1
		for _, cv := range fresh {
			out.Next.Seen = append(out.Next.Seen, cv.SessionID)
		}
	case edge != c.Start:
		out.Next.Start, out.Next.Seen = edge, nil
		for _, cv := range list {
			if cv.UpdatedAt == edge {
				out.Next.Seen = append(out.Next.Seen, cv.SessionID)
			}
		}
	default:
		if len(list) > 0 && len(fresh) == 0 {
			out.Next.Page = page + 1
		}
		for _, cv := range fresh {
			if cv.UpdatedAt == edge {
				out.Next.Seen = append(out.Next.Seen, cv.SessionID)
			}
		}
	}
	newest, _ := time.Parse(time.RFC3339, c.After)
	var ops map[string]Person
	for _, cv := range fresh {
		requesterID := cv.PeopleID
		if requesterID == "" {
			requesterID = cv.SessionID
		}
		subject := strings.TrimSpace(cv.Meta.Subject)
		if subject == "" {
			subject = strings.TrimSpace(cv.Topic)
		}
		t := Ticket{
			ID: cv.SessionID, Folder: r.website, Subject: subject, Status: crispStatus(cv.State),
			Requester: Person{
				ID: requesterID, Name: strings.TrimSpace(cv.Meta.Nickname), Email: strings.ToLower(strings.TrimSpace(cv.Meta.Email)), Phone: cv.Meta.Phone,
			},
			CreatedAt: crispTime(cv.CreatedAt), UpdatedAt: crispTime(cv.UpdatedAt),
		}
		seen := map[string]bool{}
		for _, s := range append(append([]string{}, cv.Segments...), cv.Meta.Segments...) {
			if s = strings.TrimSpace(s); s != "" && !seen[s] {
				seen[s] = true
				t.Tags = append(t.Tags, s)
			}
		}
		if cv.Assigned != nil && cv.Assigned.UserID != "" {
			if ops == nil {
				var err error
				if ops, err = r.operators(ctx); err != nil {
					return TicketPage{}, err
				}
			}
			if p, ok := ops[cv.Assigned.UserID]; ok {
				t.Assignee = new(p)
			}
		}
		if t.UpdatedAt.After(newest) {
			newest = t.UpdatedAt
		}
		out.Tickets = append(out.Tickets, t)
	}
	if !newest.IsZero() {
		out.Next.After = newest.Format(time.RFC3339)
	}
	return out, nil
}

type crispMessage struct {
	Type        string          `json:"type"`
	From        string          `json:"from"`
	Content     json.RawMessage `json:"content"`
	Fingerprint json.Number     `json:"fingerprint"`
	Timestamp   int64           `json:"timestamp"`
	User        *struct {
		Type     string `json:"type"`
		UserID   string `json:"user_id"`
		Nickname string `json:"nickname"`
	} `json:"user"`
}

// Messages reads a conversation's messages backwards in batches, with timestamp_before one
// millisecond past the oldest message read, until a batch has none that is new, so messages that
// share a millisecond across a batch boundary are read whether Crisp's bound is inclusive or not. Texts and notes are kept, and files and animations as
// attachments; events, pickers and the other types are not messages.
func (r *Crisp) Messages(ctx context.Context, t Ticket) ([]Message, error) {
	ops, err := r.operators(ctx)
	if err != nil {
		return nil, err
	}
	var out []Message
	seen := map[string]bool{}
	var before int64
	for {
		q := url.Values{}
		if before > 0 {
			q.Set("timestamp_before", strconv.FormatInt(before, 10))
		}
		var batch []crispMessage
		if err := r.get(ctx, "/conversation/"+url.PathEscape(t.ID)+"/messages", q, &batch); err != nil {
			return nil, err
		}
		oldest, added := int64(0), 0
		for _, m := range batch {
			if m.Timestamp > 0 && (oldest == 0 || m.Timestamp < oldest) {
				oldest = m.Timestamp
			}
			id := m.Fingerprint.String()
			if id == "" {
				id = strconv.FormatInt(m.Timestamp, 10) + m.Type
			}
			if seen[id] {
				continue
			}
			seen[id] = true
			added++
			if msg, ok := r.message(m, id, t, ops); ok {
				out = append(out, msg)
			}
		}
		if added == 0 || oldest == 0 {
			return out, nil
		}
		before = oldest + 1
	}
}

func (r *Crisp) message(m crispMessage, id string, t Ticket, ops map[string]Person) (Message, bool) {
	msg := Message{ID: id, At: crispTime(m.Timestamp), Direction: "in"}
	switch m.Type {
	case "text", "note":
		var text string
		if json.Unmarshal(m.Content, &text) != nil {
			return msg, false
		}
		msg.Text, msg.Note = text, m.Type == "note"
	case "file", "animation":
		var f struct {
			Name string `json:"name"`
			URL  string `json:"url"`
			Type string `json:"type"`
		}
		if json.Unmarshal(m.Content, &f) != nil || f.URL == "" {
			return msg, false
		}
		name := f.Name
		if name == "" {
			if u, err := url.Parse(f.URL); err == nil {
				name = path.Base(u.Path)
			}
		}
		msg.Attachments = []Attachment{{Name: name, Type: f.Type, Ref: f.URL}}
	default:
		return msg, false
	}
	if m.From == "operator" {
		msg.Direction = "out"
		msg.Author = Person{Agent: true}
		if m.User != nil {
			msg.Author.ID, msg.Author.Name = m.User.UserID, strings.TrimSpace(m.User.Nickname)
			if op, ok := ops[m.User.UserID]; ok {
				msg.Author = op
			}
		}
		return msg, true
	}
	msg.Author = t.Requester
	if m.User != nil && m.User.Type == "participant" && m.User.UserID != "" {
		msg.Author = Person{ID: m.User.UserID, Name: strings.TrimSpace(m.User.Nickname)}
	}
	return msg, true
}

// Attachment fetches a file Crisp keeps in its storage, without the token. A URL anywhere else
// (a link a customer typed or an animation from another service) is not fetched.
func (r *Crisp) Attachment(ctx context.Context, _ Ticket, a Attachment) ([]byte, error) {
	u, err := url.Parse(a.Ref)
	if err != nil || u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") || !strings.EqualFold(u.Hostname(), crispStorageHost) {
		return nil, &RefusedError{Method: "GET", Host: a.Ref}
	}
	return r.c.get(ctx, r.c.files, ClassAPI, u.String(), "*/*")
}

// Customer: the conversation list already carries the visitor's e-mail, nickname and phone.
func (r *Crisp) Customer(_ context.Context, p Person) (Person, error) { return p, nil }

var crispDesk = Desk{
	Source: "crisp", Name: "Crisp", Guide: "migrate-crisp", Auth: AuthToken, Account: AccountWebsite, Secret: true,
	APIHosts:  func(Account) Hosts { return Hosts{Exact: []string{CrispAPIHost}} },
	FileHosts: func(Account) Hosts { return Hosts{Exact: []string{crispStorageHost}} },
	Send:      Send{Scheme: SendBasic, Headers: map[string]string{"X-Crisp-Tier": "website"}},
	Limits: []Limit{
		{Window: time.Minute, Default: 60},
		{Window: Day, Default: 5000},
	},
	HalveOnLimit: true, AttributePrefix: "crisp", Sync: SyncModifiedSince,
	read: func(c *client, a Account) Source { return &Crisp{c: c, website: a.Website} },
}
