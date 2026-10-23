package helpdesk

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/inbucket/html2text"
)

// HelpScoutBudget is a Help Scout account's budget: half of its X-RateLimit-Limit-Minute, which
// all of the account's apps share.
func HelpScoutBudget(b *Budget) *Budget { return helpScoutDesk.Budget(b) }

// HelpScout reads one Help Scout account through the Mailbox API 2.0. Help Scout has no read-only
// permission: the GET-only transport is what keeps this reader from changing anything.
type HelpScout struct {
	c *client
}

func NewHelpScout(cfg Config, token func() string, budget *Budget) *HelpScout {
	return &HelpScout{c: helpScoutDesk.client(cfg, Account{}, Credentials{Token: token}, budget)}
}

func (h *HelpScout) url(path string, q url.Values) string {
	return withQuery("https://"+HelpScoutAPIHost+path, q)
}

type hsPage struct {
	Page struct {
		Number     int `json:"number"`
		TotalPages int `json:"totalPages"`
	} `json:"page"`
}

func (p hsPage) more() bool { return p.Page.Number < p.Page.TotalPages }

type hsUser struct {
	ID        int64  `json:"id"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Email     string `json:"email"`
	Role      string `json:"role"`
}

func (u hsUser) person() Person {
	return Person{
		ID: idString(u.ID), Name: strings.TrimSpace(u.FirstName + " " + u.LastName), Email: strings.ToLower(strings.TrimSpace(u.Email)),
		Agent: true, Admin: u.Role == "owner" || u.Role == "admin",
	}
}

// hsRef is how a conversation or thread names a person.
type hsRef struct {
	ID    int64  `json:"id"`
	Type  string `json:"type"`
	First string `json:"first"`
	Last  string `json:"last"`
	Email string `json:"email"`
}

func (r hsRef) person(agent bool) Person {
	return Person{ID: idString(r.ID), Name: strings.TrimSpace(r.First + " " + r.Last), Email: strings.ToLower(strings.TrimSpace(r.Email)), Agent: agent}
}

func (h *HelpScout) Me(ctx context.Context) (Person, error) {
	var u hsUser
	if err := h.c.getJSON(ctx, ClassAPI, h.url("/v2/users/me", nil), &u); err != nil {
		return Person{}, err
	}
	return u.person(), nil
}

// Folders are the account's mailboxes.
func (h *HelpScout) Folders(ctx context.Context) ([]Folder, error) {
	var out []Folder
	for n := 1; ; n++ {
		var page struct {
			hsPage
			Embedded struct {
				Mailboxes []struct {
					ID    int64  `json:"id"`
					Name  string `json:"name"`
					Email string `json:"email"`
				} `json:"mailboxes"`
			} `json:"_embedded"`
		}
		if err := h.c.getJSON(ctx, ClassAPI, h.url("/v2/mailboxes", url.Values{"page": {strconv.Itoa(n)}}), &page); err != nil {
			return nil, err
		}
		for _, m := range page.Embedded.Mailboxes {
			out = append(out, Folder{ID: idString(m.ID), Name: m.Name, Address: strings.ToLower(m.Email)})
		}
		if !page.more() {
			return out, nil
		}
	}
}

func (h *HelpScout) Agents(ctx context.Context) ([]Person, error) {
	var out []Person
	for n := 1; ; n++ {
		var page struct {
			hsPage
			Embedded struct {
				Users []hsUser `json:"users"`
			} `json:"_embedded"`
		}
		if err := h.c.getJSON(ctx, ClassAPI, h.url("/v2/users", url.Values{"page": {strconv.Itoa(n)}}), &page); err != nil {
			return nil, err
		}
		for _, u := range page.Embedded.Users {
			out = append(out, u.person())
		}
		if !page.more() {
			return out, nil
		}
	}
}

type hsConversation struct {
	ID              int64      `json:"id"`
	Threads         int        `json:"threads"`
	Status          string     `json:"status"`
	State           string     `json:"state"`
	Subject         string     `json:"subject"`
	MailboxID       int64      `json:"mailboxId"`
	Assignee        *hsRef     `json:"assignee"`
	PrimaryCustomer hsRef      `json:"primaryCustomer"`
	CreatedAt       time.Time  `json:"createdAt"`
	ClosedAt        *time.Time `json:"closedAt"`
	UserUpdatedAt   *time.Time `json:"userUpdatedAt"`
	Waiting         *struct {
		Time *time.Time `json:"time"`
	} `json:"customerWaitingSince"`
	Source struct {
		Type string `json:"type"`
	} `json:"source"`
	Tags []struct {
		Tag string `json:"tag"`
	} `json:"tags"`
}

func helpScoutStatus(c hsConversation) string {
	if c.State == "deleted" || c.State == "draft" {
		return StatusDeleted
	}
	switch c.Status {
	case "active":
		return StatusOpen
	case "pending":
		return StatusPending
	case "closed":
		return StatusClosed
	case "spam":
		return StatusSpam
	}
	return StatusOpen
}

// Tickets reads a page of conversations of every status, oldest change first, changed since
// c.ModifiedSince when it is set.
func (h *HelpScout) Tickets(ctx context.Context, c Cursor) (TicketPage, error) {
	page := max(c.Page, 1)
	q := url.Values{"status": {"all"}, "sortField": {"modifiedAt"}, "sortOrder": {"asc"}, "page": {strconv.Itoa(page)}}
	if !c.ModifiedSince.IsZero() {
		q.Set("modifiedSince", c.ModifiedSince.UTC().Format(time.RFC3339))
	}
	var res struct {
		hsPage
		Embedded struct {
			Conversations []hsConversation `json:"conversations"`
		} `json:"_embedded"`
	}
	if err := h.c.getJSON(ctx, ClassAPI, h.url("/v2/conversations", q), &res); err != nil {
		return TicketPage{}, err
	}
	out := TicketPage{Next: c, End: !res.more()}
	out.Next.Page = page + 1
	for _, cv := range res.Embedded.Conversations {
		t := Ticket{
			ID: idString(cv.ID), Folder: idString(cv.MailboxID), Subject: cv.Subject, Status: helpScoutStatus(cv),
			Requester: cv.PrimaryCustomer.person(false), Via: cv.Source.Type, Messages: cv.Threads, CreatedAt: cv.CreatedAt.UTC(),
		}
		t.UpdatedAt = t.CreatedAt
		for _, at := range []*time.Time{cv.UserUpdatedAt, cv.ClosedAt} {
			if at != nil && at.After(t.UpdatedAt) {
				t.UpdatedAt = at.UTC()
			}
		}
		if cv.Waiting != nil && cv.Waiting.Time != nil && cv.Waiting.Time.After(t.UpdatedAt) {
			t.UpdatedAt = cv.Waiting.Time.UTC()
		}
		if cv.Assignee != nil && cv.Assignee.ID != 0 {
			t.Assignee = new(cv.Assignee.person(true))
		}
		for _, tag := range cv.Tags {
			t.Tags = append(t.Tags, tag.Tag)
		}
		out.Tickets = append(out.Tickets, t)
	}
	return out, nil
}

type hsThread struct {
	ID        int64     `json:"id"`
	Type      string    `json:"type"`
	State     string    `json:"state"`
	Body      string    `json:"body"`
	Customer  *hsRef    `json:"customer"`
	CreatedBy *hsRef    `json:"createdBy"`
	CreatedAt time.Time `json:"createdAt"`
	Source    struct {
		Type string `json:"type"`
		Via  string `json:"via"`
	} `json:"source"`
	Embedded struct {
		Attachments []struct {
			ID       int64  `json:"id"`
			Filename string `json:"filename"`
			MimeType string `json:"mimeType"`
			Size     int64  `json:"size"`
		} `json:"attachments"`
	} `json:"_embedded"`
}

// Messages reads a conversation's threads: customer threads in, replies out, notes as notes,
// chats and calls by who wrote them; workflow line items, drafts and forwards' copies are left
// out. An e-mail thread is read from its original source while Help Scout keeps it.
func (h *HelpScout) Messages(ctx context.Context, t Ticket) ([]Message, error) {
	var out []Message
	for n := 1; ; n++ {
		var page struct {
			hsPage
			Embedded struct {
				Threads []hsThread `json:"threads"`
			} `json:"_embedded"`
		}
		path := "/v2/conversations/" + url.PathEscape(t.ID) + "/threads"
		if err := h.c.getJSON(ctx, ClassAPI, h.url(path, url.Values{"page": {strconv.Itoa(n)}}), &page); err != nil {
			return nil, err
		}
		for _, th := range page.Embedded.Threads {
			m, ok := threadMessage(th)
			if !ok {
				continue
			}
			if th.Source.Type == "email" && !m.Note {
				raw, err := h.original(ctx, t.ID, th.ID)
				if err != nil {
					return nil, err
				}
				m.Raw = raw
			}
			out = append(out, m)
		}
		if !page.more() {
			return out, nil
		}
	}
}

func threadMessage(th hsThread) (Message, bool) {
	if th.State != "" && th.State != "published" {
		return Message{}, false
	}
	by := hsRef{}
	if th.CreatedBy != nil {
		by = *th.CreatedBy
	}
	customer := by.Type == "customer"
	m := Message{ID: idString(th.ID), At: th.CreatedAt.UTC(), HTML: th.Body}
	switch th.Type {
	case "customer":
		m.Direction = "in"
		if th.Customer != nil && th.Customer.Email != "" {
			by = *th.Customer
		}
		m.Author = by.person(false)
	case "message":
		m.Direction, m.Author = "out", by.person(true)
	case "note", "forwardparent":
		m.Note, m.Author = true, by.person(true)
	case "chat", "phone", "beaconchat":
		m.Direction, m.Author = "out", by.person(!customer)
		if customer {
			m.Direction = "in"
		}
	default:
		return Message{}, false
	}
	if text, err := html2text.FromString(th.Body, html2text.Options{TextOnly: true}); err == nil {
		m.Text = text
	}
	for _, a := range th.Embedded.Attachments {
		m.Attachments = append(m.Attachments, Attachment{Name: a.Filename, Type: a.MimeType, Size: a.Size, Ref: idString(a.ID)})
	}
	return m, true
}

// original is an e-mail thread's original message, or nil once Help Scout no longer keeps it.
func (h *HelpScout) original(ctx context.Context, conversation string, thread int64) ([]byte, error) {
	path := "/v2/conversations/" + url.PathEscape(conversation) + "/threads/" + idString(thread) + "/original-source"
	raw, err := h.c.get(ctx, h.c.api, ClassAPI, h.url(path, nil), "message/rfc822")
	var se *StatusError
	if errors.As(err, &se) && (se.Status == http.StatusNotFound || se.Status == http.StatusGone) {
		return nil, nil
	}
	if errors.Is(err, ErrTooLarge) {
		return nil, nil
	}
	return raw, err
}

func (h *HelpScout) Attachment(ctx context.Context, t Ticket, a Attachment) ([]byte, error) {
	path := "/v2/conversations/" + url.PathEscape(t.ID) + "/attachments/" + url.PathEscape(a.Ref) + "/data"
	var out struct {
		Data string `json:"data"`
	}
	if err := h.c.getJSON(ctx, ClassAPI, h.url(path, nil), &out); err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(out.Data)
}

// Customer reads a customer's name and phone.
func (h *HelpScout) Customer(ctx context.Context, p Person) (Person, error) {
	var c struct {
		FirstName string `json:"firstName"`
		LastName  string `json:"lastName"`
		Embedded  struct {
			Phones []struct {
				Value string `json:"value"`
			} `json:"phones"`
		} `json:"_embedded"`
	}
	if err := h.c.getJSON(ctx, ClassAPI, h.url("/v2/customers/"+url.PathEscape(p.ID), nil), &c); err != nil {
		var se *StatusError
		if errors.As(err, &se) && se.Status == http.StatusNotFound {
			return p, nil
		}
		return p, err
	}
	if name := strings.TrimSpace(c.FirstName + " " + c.LastName); name != "" {
		p.Name = name
	}
	if len(c.Embedded.Phones) > 0 {
		p.Phone = c.Embedded.Phones[0].Value
	}
	return p, nil
}
