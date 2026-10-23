package helpdesk

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/inbucket/html2text"
)

// FrontAPIHost is where every Front request goes, the token with it. Message attachments name the
// company's own subdomain of api.frontapp.com; they are fetched from this host by id instead.
const FrontAPIHost = "api2.frontapp.com"

const frontPageSize = "100"

// FrontBudget is a Front company's budget: half of its x-ratelimit-limit, which every token of
// the company shares, and at most 40% of that limit on the search guard.
func FrontBudget(b *Budget) *Budget { return frontDesk.Budget(b) }

// Front reads one Front company through the Core API. A token can be limited to Read only when it
// is created, but a caller cannot read the permissions of its own token (GET /me answers the
// company only), so the GET-only transport is what keeps this reader from changing anything.
//
// Tickets lists inbox by inbox: Cursor.Inbox is the id of the inbox being read, Cursor.After Front's
// page token inside it and Cursor.Page the inbox's 1-based position when it was stored, used only
// to go on when that inbox is gone. A page token is never sent to another inbox than the one that
// issued it. With Cursor.ModifiedSince set it searches each inbox for
// conversations with activity after that time instead.
type Front struct {
	c       *client
	inboxes []Folder
	listed  bool
}

func NewFront(cfg Config, token func() string, budget *Budget) *Front {
	return &Front{c: frontDesk.client(cfg, Account{}, Credentials{Token: token}, budget)}
}

func (f *Front) url(path string, q url.Values) string {
	return withQuery("https://"+FrontAPIHost+path, q)
}

type frontPagination struct {
	Pagination struct {
		Next string `json:"next"`
	} `json:"_pagination"`
}

// token is the page token of the next page, or "" on the last.
func (p frontPagination) token() string {
	if p.Pagination.Next == "" {
		return ""
	}
	u, err := url.Parse(p.Pagination.Next)
	if err != nil {
		return ""
	}
	return u.Query().Get("page_token")
}

func frontTime(v float64) time.Time {
	if v <= 0 {
		return time.Time{}
	}
	sec, frac := math.Modf(v)
	return time.Unix(int64(sec), int64(frac*1e9)).UTC()
}

type frontTeammate struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	IsAdmin   bool   `json:"is_admin"`
	Type      string `json:"type"`
}

func (t frontTeammate) person() Person {
	return Person{
		ID: t.ID, Name: strings.TrimSpace(t.FirstName + " " + t.LastName), Email: strings.ToLower(strings.TrimSpace(t.Email)),
		Agent: true, Admin: t.IsAdmin,
	}
}

type frontRecipient struct {
	Name   string `json:"name"`
	Handle string `json:"handle"`
	Role   string `json:"role"`
}

func (r frontRecipient) person() Person {
	handle := strings.ToLower(strings.TrimSpace(r.Handle))
	p := Person{ID: handle, Name: strings.TrimSpace(r.Name)}
	if strings.Contains(handle, "@") {
		p.Email = handle
	} else if p.Name == "" {
		p.Name = handle
	}
	return p
}

// Me is the company the token belongs to.
func (f *Front) Me(ctx context.Context) (Person, error) {
	var me struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := f.c.getJSON(ctx, ClassAPI, f.url("/me", nil), &me); err != nil {
		return Person{}, err
	}
	return Person{ID: me.ID, Name: me.Name, Admin: true}, nil
}

func (f *Front) listInboxes(ctx context.Context) ([]Folder, error) {
	var out []Folder
	token := ""
	for {
		var page struct {
			frontPagination
			Results []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"_results"`
		}
		q := url.Values{"limit": {frontPageSize}}
		if token != "" {
			q.Set("page_token", token)
		}
		if err := f.c.getJSON(ctx, ClassAPI, f.url("/inboxes", q), &page); err != nil {
			return nil, err
		}
		for _, in := range page.Results {
			out = append(out, Folder{ID: in.ID, Name: in.Name})
		}
		if token = page.token(); token == "" {
			f.inboxes, f.listed = out, true
			return out, nil
		}
	}
}

// Folders are the inboxes the token can see, each with the address of its first e-mail channel.
func (f *Front) Folders(ctx context.Context) ([]Folder, error) {
	out, err := f.listInboxes(ctx)
	if err != nil {
		return nil, err
	}
	for i := range out {
		var ch struct {
			Results []struct {
				Address string `json:"address"`
			} `json:"_results"`
		}
		err := f.c.getJSON(ctx, ClassAPI, f.url("/inboxes/"+url.PathEscape(out[i].ID)+"/channels", nil), &ch)
		var se *StatusError
		if errors.As(err, &se) && (se.Status == http.StatusForbidden || se.Status == http.StatusNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, c := range ch.Results {
			if a := strings.ToLower(strings.TrimSpace(c.Address)); strings.Contains(a, "@") {
				out[i].Address = a
				break
			}
		}
	}
	return out, nil
}

func (f *Front) Agents(ctx context.Context) ([]Person, error) {
	var out []Person
	token := ""
	for {
		var page struct {
			frontPagination
			Results []frontTeammate `json:"_results"`
		}
		q := url.Values{}
		if token != "" {
			q.Set("page_token", token)
		}
		if err := f.c.getJSON(ctx, ClassAPI, f.url("/teammates", q), &page); err != nil {
			return nil, err
		}
		for _, t := range page.Results {
			if t.Type == "" || t.Type == "user" {
				out = append(out, t.person())
			}
		}
		if token = page.token(); token == "" {
			return out, nil
		}
	}
}

type frontConversation struct {
	ID             string          `json:"id"`
	Subject        string          `json:"subject"`
	Status         string          `json:"status"`
	StatusCategory string          `json:"status_category"`
	IsClosed       bool            `json:"is_closed"`
	Assignee       *frontTeammate  `json:"assignee"`
	Recipient      *frontRecipient `json:"recipient"`
	Tags           []struct {
		Name string `json:"name"`
	} `json:"tags"`
	CreatedAt    float64 `json:"created_at"`
	UpdatedAt    float64 `json:"updated_at"`
	WaitingSince float64 `json:"waiting_since"`
}

// frontStatus: archived and resolved conversations are closed, waiting ones pending, the rest
// open. Front's deleted status is never copied.
func frontStatus(c frontConversation) string {
	switch {
	case c.Status == "deleted":
		return StatusDeleted
	case c.Status == "spam":
		return StatusSpam
	case c.Status == "archived", c.StatusCategory == "resolved", c.IsClosed:
		return StatusClosed
	case c.StatusCategory == "waiting":
		return StatusPending
	}
	return StatusOpen
}

func (f *Front) inboxList(ctx context.Context) ([]Folder, error) {
	if f.listed {
		return f.inboxes, nil
	}
	return f.listInboxes(ctx)
}

// Tickets reads a page of one inbox's conversations; End once the last inbox's last page is read.
func (f *Front) Tickets(ctx context.Context, c Cursor) (TicketPage, error) {
	inboxes, err := f.inboxList(ctx)
	if err != nil {
		return TicketPage{}, err
	}
	idx := max(c.Page, 1) - 1
	if c.Inbox != "" {
		if i := slices.IndexFunc(inboxes, func(f Folder) bool { return f.ID == c.Inbox }); i >= 0 {
			idx = i
		} else {
			c.After = ""
		}
	}
	if idx >= len(inboxes) {
		return TicketPage{Next: c, End: true}, nil
	}
	inbox := inboxes[idx]
	q := url.Values{"limit": {frontPageSize}}
	if c.After != "" {
		q.Set("page_token", c.After)
	}
	class := ClassAPI
	var path string
	if c.ModifiedSince.IsZero() {
		path = "/inboxes/" + url.PathEscape(inbox.ID) + "/conversations"
		q["q[statuses][]"] = []string{"unassigned", "assigned", "archived"}
	} else {
		class = ClassIncremental
		query := "inbox:" + inbox.ID + " after:" + strconv.FormatInt(c.ModifiedSince.Unix(), 10)
		path = "/conversations/search/" + url.PathEscape(query)
	}
	var res struct {
		frontPagination
		Results []frontConversation `json:"_results"`
	}
	if err := f.c.getJSON(ctx, class, f.url(path, q), &res); err != nil {
		return TicketPage{}, err
	}
	out := TicketPage{Next: c}
	if token := res.token(); token != "" {
		out.Next.Inbox, out.Next.Page, out.Next.After = inbox.ID, idx+1, token
	} else {
		out.Next.Inbox, out.Next.Page, out.Next.After = "", idx+2, ""
		if idx+1 < len(inboxes) {
			out.Next.Inbox = inboxes[idx+1].ID
		}
		out.End = idx+1 >= len(inboxes)
	}
	for _, cv := range res.Results {
		t := Ticket{ID: cv.ID, Folder: inbox.ID, Subject: cv.Subject, Status: frontStatus(cv), CreatedAt: frontTime(cv.CreatedAt)}
		if cv.Recipient != nil {
			t.Requester = cv.Recipient.person()
		}
		t.UpdatedAt = t.CreatedAt
		for _, at := range []float64{cv.UpdatedAt, cv.WaitingSince} {
			if u := frontTime(at); u.After(t.UpdatedAt) {
				t.UpdatedAt = u
			}
		}
		if cv.Assignee != nil && cv.Assignee.ID != "" {
			t.Assignee = new(cv.Assignee.person())
		}
		for _, tag := range cv.Tags {
			t.Tags = append(t.Tags, tag.Name)
		}
		out.Tickets = append(out.Tickets, t)
	}
	return out, nil
}

type frontAttachment struct {
	URL      string `json:"url"`
	Filename string `json:"filename"`
	Type     string `json:"content_type"`
	Size     int64  `json:"size"`
	Metadata struct {
		Inline bool `json:"is_inline"`
	} `json:"metadata"`
}

// frontAttachments keeps the files Front itself stores, whose URL is /download/{id} on the API host
// or the company's subdomain of it; a link a customer typed, a preview or a shared link names any
// other address and is never fetched.
func frontAttachments(in []frontAttachment) []Attachment {
	var out []Attachment
	for _, a := range in {
		u, err := url.Parse(a.URL)
		if err != nil || u.Scheme != "https" || u.Port() != "" {
			continue
		}
		host := strings.ToLower(u.Hostname())
		if host != FrontAPIHost && !strings.HasSuffix(host, ".api.frontapp.com") {
			continue
		}
		id, ok := strings.CutPrefix(u.Path, "/download/")
		if !ok || id == "" || strings.Contains(id, "/") {
			continue
		}
		out = append(out, Attachment{Name: a.Filename, Type: a.Type, Size: a.Size, Inline: a.Metadata.Inline, Ref: id})
	}
	return out
}

// Messages reads a conversation's messages (inbound ones in, teammates' replies out, internal
// discussion messages as notes; drafts are left out) and its comments as notes, oldest first.
func (f *Front) Messages(ctx context.Context, t Ticket) ([]Message, error) {
	var out []Message
	base := "/conversations/" + url.PathEscape(t.ID)
	token := ""
	for {
		var page struct {
			frontPagination
			Results []struct {
				ID          string            `json:"id"`
				Type        string            `json:"type"`
				IsInbound   bool              `json:"is_inbound"`
				CreatedAt   float64           `json:"created_at"`
				DraftMode   *string           `json:"draft_mode"`
				Author      *frontTeammate    `json:"author"`
				Recipients  []frontRecipient  `json:"recipients"`
				Body        string            `json:"body"`
				Text        string            `json:"text"`
				Attachments []frontAttachment `json:"attachments"`
			} `json:"_results"`
		}
		q := url.Values{"limit": {frontPageSize}, "sort_by": {"created_at"}, "sort_order": {"asc"}}
		if token != "" {
			q.Set("page_token", token)
		}
		if err := f.c.getJSON(ctx, ClassAPI, f.url(base+"/messages", q), &page); err != nil {
			return nil, err
		}
		for _, m := range page.Results {
			if m.DraftMode != nil && *m.DraftMode != "" {
				continue
			}
			msg := Message{ID: m.ID, At: frontTime(m.CreatedAt), HTML: m.Body, Text: m.Text, Attachments: frontAttachments(m.Attachments)}
			switch {
			case m.Type == "internal":
				msg.Note = true
				if m.Author != nil {
					msg.Author = m.Author.person()
				}
			case m.IsInbound:
				msg.Direction, msg.Author = "in", t.Requester
				for _, r := range m.Recipients {
					if r.Role == "from" {
						msg.Author = r.person()
						break
					}
				}
			default:
				msg.Direction = "out"
				if m.Author != nil {
					msg.Author = m.Author.person()
				}
			}
			if msg.Text == "" {
				if text, err := html2text.FromString(m.Body, html2text.Options{TextOnly: true}); err == nil {
					msg.Text = text
				}
			}
			out = append(out, msg)
		}
		if token = page.token(); token == "" {
			break
		}
	}
	var comments struct {
		Results []struct {
			ID          string            `json:"id"`
			Author      *frontTeammate    `json:"author"`
			Body        string            `json:"body"`
			PostedAt    float64           `json:"posted_at"`
			Attachments []frontAttachment `json:"attachments"`
		} `json:"_results"`
	}
	if err := f.c.getJSON(ctx, ClassAPI, f.url(base+"/comments", nil), &comments); err != nil {
		return nil, err
	}
	for _, c := range comments.Results {
		msg := Message{ID: c.ID, Note: true, At: frontTime(c.PostedAt), Text: c.Body, Attachments: frontAttachments(c.Attachments)}
		if c.Author != nil {
			msg.Author = c.Author.person()
		}
		out = append(out, msg)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out, nil
}

func (f *Front) Attachment(ctx context.Context, _ Ticket, a Attachment) ([]byte, error) {
	return f.c.get(ctx, f.c.api, ClassAPI, f.url("/download/"+url.PathEscape(a.Ref), nil), "*/*")
}

// Customer reads a contact's name and phone by e-mail address.
func (f *Front) Customer(ctx context.Context, p Person) (Person, error) {
	if p.Email == "" {
		return p, nil
	}
	var c struct {
		Name    string `json:"name"`
		Handles []struct {
			Handle string `json:"handle"`
			Source string `json:"source"`
		} `json:"handles"`
	}
	if err := f.c.getJSON(ctx, ClassAPI, f.url("/contacts/alt:email:"+url.PathEscape(p.Email), nil), &c); err != nil {
		var se *StatusError
		if errors.As(err, &se) && se.Status == http.StatusNotFound {
			return p, nil
		}
		return p, err
	}
	if name := strings.TrimSpace(c.Name); name != "" {
		p.Name = name
	}
	for _, h := range c.Handles {
		if h.Source == "phone" && h.Handle != "" {
			p.Phone = h.Handle
			break
		}
	}
	return p, nil
}
