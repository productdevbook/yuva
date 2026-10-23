package helpdesk

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	zendeskTicketsPage  = 1000
	zendeskUsersPerCall = 100
	zendeskPage         = 100
)

// ZendeskBudget is a Zendesk account's budget: half of its X-Rate-Limit, 5 incremental requests a
// minute of the 10 Zendesk allows.
func ZendeskBudget(b *Budget) *Budget { return zendeskDesk.Budget(b) }

// Zendesk reads one Zendesk account with an OAuth token of scope read.
type Zendesk struct {
	c    *client
	host string
}

// NewZendesk reads subdomain.zendesk.com. The token goes only to the account's host and to
// *.zdusercontent.com, where Zendesk keeps attachments; attachments elsewhere are fetched
// without it.
func NewZendesk(cfg Config, subdomain string, token func() string, budget *Budget) *Zendesk {
	a := Account{Subdomain: subdomain}
	return &Zendesk{host: ZendeskHost(subdomain), c: zendeskDesk.client(cfg, a, Credentials{Token: token}, budget)}
}

func (z *Zendesk) url(path string, q url.Values) string {
	return withQuery("https://"+z.host+path, q)
}

type zdUser struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Email      string `json:"email"`
	Phone      string `json:"phone"`
	ExternalID string `json:"external_id"`
	Role       string `json:"role"`
}

func (u zdUser) person() Person {
	return Person{
		ID: idString(u.ID), Name: strings.TrimSpace(u.Name), Email: strings.ToLower(strings.TrimSpace(u.Email)), Phone: u.Phone,
		ExternalID: u.ExternalID, Agent: u.Role == "agent" || u.Role == "admin", Admin: u.Role == "admin",
	}
}

type zdPaging struct {
	Meta struct {
		HasMore     bool   `json:"has_more"`
		AfterCursor string `json:"after_cursor"`
	} `json:"meta"`
}

func (z *Zendesk) Me(ctx context.Context) (Person, error) {
	var out struct {
		User zdUser `json:"user"`
	}
	if err := z.c.getJSON(ctx, ClassAPI, z.url("/api/v2/users/me", nil), &out); err != nil {
		return Person{}, err
	}
	return out.User.person(), nil
}

// Folders are the account's brands, with the support address Zendesk receives each one's mail at.
func (z *Zendesk) Folders(ctx context.Context) ([]Folder, error) {
	var brands struct {
		Brands []struct {
			ID     int64  `json:"id"`
			Name   string `json:"name"`
			Active bool   `json:"active"`
		} `json:"brands"`
	}
	if err := z.c.getJSON(ctx, ClassAPI, z.url("/api/v2/brands", nil), &brands); err != nil {
		return nil, err
	}
	addresses := map[int64]string{}
	q := url.Values{"page[size]": {strconv.Itoa(zendeskPage)}}
	for {
		var page struct {
			zdPaging
			Addresses []struct {
				BrandID int64  `json:"brand_id"`
				Email   string `json:"email"`
				Default bool   `json:"default"`
			} `json:"recipient_addresses"`
		}
		if err := z.c.getJSON(ctx, ClassAPI, z.url("/api/v2/recipient_addresses", q), &page); err != nil {
			return nil, err
		}
		for _, a := range page.Addresses {
			if _, ok := addresses[a.BrandID]; !ok || a.Default {
				addresses[a.BrandID] = strings.ToLower(a.Email)
			}
		}
		if !page.Meta.HasMore || page.Meta.AfterCursor == "" {
			break
		}
		q.Set("page[after]", page.Meta.AfterCursor)
	}
	out := make([]Folder, 0, len(brands.Brands))
	for _, b := range brands.Brands {
		out = append(out, Folder{ID: idString(b.ID), Name: b.Name, Address: addresses[b.ID]})
	}
	return out, nil
}

func (z *Zendesk) Agents(ctx context.Context) ([]Person, error) {
	q := url.Values{"role[]": {"agent", "admin"}, "page[size]": {strconv.Itoa(zendeskPage)}}
	var out []Person
	for {
		var page struct {
			zdPaging
			Users []zdUser `json:"users"`
		}
		if err := z.c.getJSON(ctx, ClassAPI, z.url("/api/v2/users", q), &page); err != nil {
			return nil, err
		}
		for _, u := range page.Users {
			out = append(out, u.person())
		}
		if !page.Meta.HasMore || page.Meta.AfterCursor == "" {
			return out, nil
		}
		q.Set("page[after]", page.Meta.AfterCursor)
	}
}

func (z *Zendesk) users(ctx context.Context, ids []int64) (map[int64]zdUser, error) {
	out := map[int64]zdUser{}
	for len(ids) > 0 {
		part := ids[:min(zendeskUsersPerCall, len(ids))]
		ids = ids[len(part):]
		list := make([]string, len(part))
		for i, id := range part {
			list[i] = idString(id)
		}
		var page struct {
			Users []zdUser `json:"users"`
		}
		if err := z.c.getJSON(ctx, ClassAPI, z.url("/api/v2/users/show_many", url.Values{"ids": {strings.Join(list, ",")}}), &page); err != nil {
			return nil, err
		}
		for _, u := range page.Users {
			out[u.ID] = u
		}
	}
	return out, nil
}

type zdTicket struct {
	ID         int64     `json:"id"`
	Subject    string    `json:"subject"`
	Status     string    `json:"status"`
	BrandID    int64     `json:"brand_id"`
	Requester  int64     `json:"requester_id"`
	AssigneeID *int64    `json:"assignee_id"`
	Tags       []string  `json:"tags"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	Via        struct {
		Channel string `json:"channel"`
	} `json:"via"`
}

func zendeskStatus(s string) string {
	switch s {
	case "new", "open":
		return StatusOpen
	case "pending", "hold":
		return StatusPending
	case "solved", "closed":
		return StatusClosed
	}
	return StatusDeleted
}

// Tickets reads a page of the incremental ticket export: from c.Start on the first request, then
// from the cursor. The last page's cursor is where changes are read from later.
func (z *Zendesk) Tickets(ctx context.Context, c Cursor) (TicketPage, error) {
	q := url.Values{"per_page": {strconv.Itoa(zendeskTicketsPage)}}
	if c.After != "" {
		q.Set("cursor", c.After)
	} else {
		q.Set("start_time", strconv.FormatInt(c.Start, 10))
	}
	var page struct {
		Tickets     []zdTicket `json:"tickets"`
		AfterCursor string     `json:"after_cursor"`
		EndOfStream bool       `json:"end_of_stream"`
	}
	if err := z.c.getJSON(ctx, ClassIncremental, z.url("/api/v2/incremental/tickets/cursor", q), &page); err != nil {
		return TicketPage{}, err
	}
	seen := map[int64]bool{}
	var ids []int64
	for _, t := range page.Tickets {
		for _, id := range []int64{t.Requester, valueOr(t.AssigneeID)} {
			if id != 0 && !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	users, err := z.users(ctx, ids)
	if err != nil {
		return TicketPage{}, err
	}
	out := TicketPage{Next: c, End: page.EndOfStream || page.AfterCursor == ""}
	if page.AfterCursor != "" {
		out.Next.After = page.AfterCursor
	}
	for _, t := range page.Tickets {
		req := users[t.Requester].person()
		req.ID = idString(t.Requester)
		tk := Ticket{
			ID: idString(t.ID), Folder: idString(t.BrandID), Subject: t.Subject, Status: zendeskStatus(t.Status), Requester: req,
			Tags: t.Tags, Via: t.Via.Channel, CreatedAt: t.CreatedAt.UTC(), UpdatedAt: t.UpdatedAt.UTC(),
		}
		if t.AssigneeID != nil {
			if u, ok := users[*t.AssigneeID]; ok {
				tk.Assignee = new(u.person())
			}
		}
		out.Tickets = append(out.Tickets, tk)
	}
	return out, nil
}

func valueOr(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

type zdComment struct {
	ID          int64     `json:"id"`
	AuthorID    int64     `json:"author_id"`
	Body        string    `json:"body"`
	HTMLBody    string    `json:"html_body"`
	PlainBody   string    `json:"plain_body"`
	Public      bool      `json:"public"`
	CreatedAt   time.Time `json:"created_at"`
	Attachments []struct {
		FileName    string `json:"file_name"`
		ContentURL  string `json:"content_url"`
		ContentType string `json:"content_type"`
		Size        int64  `json:"size"`
		Inline      bool   `json:"inline"`
	} `json:"attachments"`
}

// Messages reads a ticket's comments with their authors: public comments are messages, from an
// end user in and from an agent out; private ones are notes.
func (z *Zendesk) Messages(ctx context.Context, t Ticket) ([]Message, error) {
	q := url.Values{"include": {"users"}, "page[size]": {strconv.Itoa(zendeskPage)}}
	var out []Message
	for {
		var page struct {
			zdPaging
			Comments []zdComment `json:"comments"`
			Users    []zdUser    `json:"users"`
		}
		if err := z.c.getJSON(ctx, ClassAPI, z.url("/api/v2/tickets/"+url.PathEscape(t.ID)+"/comments", q), &page); err != nil {
			return nil, err
		}
		users := map[int64]zdUser{}
		for _, u := range page.Users {
			users[u.ID] = u
		}
		for _, c := range page.Comments {
			author := users[c.AuthorID].person()
			author.ID = idString(c.AuthorID)
			m := Message{ID: idString(c.ID), Note: !c.Public, Direction: "in", Author: author, At: c.CreatedAt.UTC(), Text: c.PlainBody, HTML: c.HTMLBody}
			if author.Agent {
				m.Direction = "out"
			}
			if m.Text == "" {
				m.Text = c.Body
			}
			for _, a := range c.Attachments {
				m.Attachments = append(m.Attachments, Attachment{Name: a.FileName, Type: a.ContentType, Size: a.Size, Inline: a.Inline, Ref: a.ContentURL})
			}
			out = append(out, m)
		}
		if !page.Meta.HasMore || page.Meta.AfterCursor == "" {
			return out, nil
		}
		q.Set("page[after]", page.Meta.AfterCursor)
	}
}

// Attachment fetches a file from its content URL: with the token from the account's host or
// *.zdusercontent.com, without it from anywhere else.
func (z *Zendesk) Attachment(ctx context.Context, _ Ticket, a Attachment) ([]byte, error) {
	u, err := url.Parse(a.Ref)
	if err != nil || u.Scheme != "https" {
		return nil, &RefusedError{Method: "GET", Host: a.Ref}
	}
	return z.c.get(ctx, z.c.files, ClassAPI, u.String(), "*/*")
}

// Customer: Zendesk's tickets already name the requester in full.
func (z *Zendesk) Customer(_ context.Context, p Person) (Person, error) { return p, nil }
