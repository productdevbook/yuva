package helpdesk

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/inbucket/html2text"
)

const (
	intercomVersion   = "2.17"
	intercomPerPage   = 150
	intercomPartsCap  = 500
	intercomNoTeam    = "0"
	intercomRegionUS  = "us"
	intercomRegionEU  = "eu"
	intercomRegionAU  = "au"
	IntercomWindow    = 10 * time.Second
	intercomPerWindow = 833
)

// IntercomHost is the API host of a workspace's data region.
func IntercomHost(region string) string {
	switch region {
	case intercomRegionEU:
		return "api.eu.intercom.io"
	case intercomRegionAU:
		return "api.au.intercom.io"
	}
	return "api.intercom.io"
}

// IntercomRegion tells a region the connect form offers.
func IntercomRegion(region string) bool {
	return region == intercomRegionUS || region == intercomRegionEU || region == intercomRegionAU
}

// IntercomBudget is an Intercom workspace's budget: half of its X-RateLimit-Limit, which counts
// 10-second windows and is shared by all of the workspace's private apps.
func IntercomBudget(b *Budget) *Budget { return intercomDesk.Budget(b) }

// TruncatedError says that Intercom returned only the most recent parts of a very long
// conversation; the messages come back with it, and the caller records the cut.
type TruncatedError struct{ Have, Total int }

func (e *TruncatedError) Error() string {
	return fmt.Sprintf("Intercom returned the %d most recent of %d parts", e.Have, e.Total)
}

// Intercom reads one Intercom workspace with the access token of a private app. Intercom's search
// endpoints are POSTs, which the transport refuses, so conversations are listed and read one by
// one. Attachments are fetched from Intercom's CDN without the token.
type Intercom struct {
	c    *client
	host string

	mu     sync.Mutex
	admins map[string]Person
}

func NewIntercom(cfg Config, region string, token func() string, budget *Budget) *Intercom {
	a := Account{Region: region}
	return &Intercom{host: IntercomHost(region), c: intercomDesk.client(cfg, a, Credentials{Token: token}, budget)}
}

func (i *Intercom) url(path string, q url.Values) string {
	return withQuery("https://"+i.host+path, q)
}

type icAdmin struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

func (a icAdmin) person() Person {
	return Person{ID: a.ID, Name: strings.TrimSpace(a.Name), Email: strings.ToLower(strings.TrimSpace(a.Email)), Agent: true}
}

type icAuthor struct {
	Type  string `json:"type"`
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

func (a icAuthor) person() Person {
	return Person{
		ID: a.ID, Name: strings.TrimSpace(a.Name), Email: strings.ToLower(strings.TrimSpace(a.Email)),
		Agent: a.Type == "admin" || a.Type == "bot" || a.Type == "team",
	}
}

func (a icAuthor) customer() bool { return a.Type == "user" || a.Type == "lead" || a.Type == "contact" }

// Me is the admin the token belongs to, named by the workspace it reads, which a reconnect must
// reach again.
func (i *Intercom) Me(ctx context.Context) (Person, error) {
	var out struct {
		icAdmin
		App struct {
			IDCode string `json:"id_code"`
		} `json:"app"`
	}
	if err := i.c.getJSON(ctx, ClassAPI, i.url("/me", nil), &out); err != nil {
		return Person{}, err
	}
	p := out.person()
	if out.App.IDCode != "" {
		p.ID = out.App.IDCode
	}
	return p, nil
}

// Folders are the teams and Intercom's own "unassigned": a conversation without a team goes to
// the folder with id 0.
func (i *Intercom) Folders(ctx context.Context) ([]Folder, error) {
	var out struct {
		Teams []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"teams"`
	}
	if err := i.c.getJSON(ctx, ClassAPI, i.url("/teams", nil), &out); err != nil {
		return nil, err
	}
	folders := []Folder{{ID: intercomNoTeam, Name: "Unassigned"}}
	for _, t := range out.Teams {
		folders = append(folders, Folder{ID: t.ID, Name: t.Name})
	}
	return folders, nil
}

func (i *Intercom) adminList(ctx context.Context) (map[string]Person, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.admins != nil {
		return i.admins, nil
	}
	var out struct {
		Admins []icAdmin `json:"admins"`
	}
	if err := i.c.getJSON(ctx, ClassAPI, i.url("/admins", nil), &out); err != nil {
		return nil, err
	}
	m := map[string]Person{}
	for _, a := range out.Admins {
		m[a.ID] = a.person()
	}
	i.admins = m
	return m, nil
}

func (i *Intercom) Agents(ctx context.Context) ([]Person, error) {
	m, err := i.adminList(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Person, 0, len(m))
	for _, p := range m {
		out = append(out, p)
	}
	return out, nil
}

type icConversation struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	State     string `json:"state"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
	Admin     any    `json:"admin_assignee_id"`
	Team      any    `json:"team_assignee_id"`
	Source    struct {
		Type    string   `json:"type"`
		Subject string   `json:"subject"`
		Author  icAuthor `json:"author"`
	} `json:"source"`
	Contacts struct {
		Contacts []icAuthor `json:"contacts"`
	} `json:"contacts"`
	Tags struct {
		Tags []struct {
			Name string `json:"name"`
		} `json:"tags"`
	} `json:"tags"`
}

// intercomRef reads an id Intercom sends as a number or a string; 0 and empty are none.
func intercomRef(v any) string {
	switch x := v.(type) {
	case string:
		if x == "0" {
			return ""
		}
		return x
	case float64:
		if x == 0 {
			return ""
		}
		return strconv.FormatInt(int64(x), 10)
	}
	return ""
}

func intercomStatus(state string) string {
	switch state {
	case "snoozed":
		return StatusPending
	case "closed":
		return StatusClosed
	}
	return StatusOpen
}

var tagRe = regexp.MustCompile(`<[^>]*>`)

func plainText(html string) string {
	text, err := html2text.FromString(html, html2text.Options{TextOnly: true})
	if err != nil {
		return ""
	}
	return strings.TrimSpace(text)
}

// Tickets reads a page of GET /conversations. Intercom documents no order, so the listing is always
// read to its end and only the conversations updated at or after c.ModifiedSince are kept.
func (i *Intercom) Tickets(ctx context.Context, c Cursor) (TicketPage, error) {
	admins, err := i.adminList(ctx)
	if err != nil {
		return TicketPage{}, err
	}
	q := url.Values{"per_page": {strconv.Itoa(intercomPerPage)}}
	if c.After != "" {
		q.Set("starting_after", c.After)
	}
	var res struct {
		Conversations []icConversation `json:"conversations"`
		Pages         struct {
			Next *struct {
				StartingAfter string `json:"starting_after"`
			} `json:"next"`
		} `json:"pages"`
	}
	if err := i.c.getJSON(ctx, ClassAPI, i.url("/conversations", q), &res); err != nil {
		return TicketPage{}, err
	}
	out := TicketPage{Next: c, End: res.Pages.Next == nil || res.Pages.Next.StartingAfter == ""}
	if !out.End {
		out.Next.After = res.Pages.Next.StartingAfter
	}
	for _, cv := range res.Conversations {
		updated := time.Unix(cv.UpdatedAt, 0).UTC()
		if !c.ModifiedSince.IsZero() && updated.Before(c.ModifiedSince) {
			continue
		}
		out.Tickets = append(out.Tickets, intercomTicket(cv, admins))
	}
	return out, nil
}

func intercomTicket(cv icConversation, admins map[string]Person) Ticket {
	t := Ticket{
		ID: cv.ID, Folder: intercomNoTeam, Subject: strings.TrimSpace(cv.Title), Status: intercomStatus(cv.State), Via: cv.Source.Type,
		CreatedAt: time.Unix(cv.CreatedAt, 0).UTC(), UpdatedAt: time.Unix(cv.UpdatedAt, 0).UTC(),
	}
	if t.Subject == "" {
		t.Subject = strings.TrimSpace(html.UnescapeString(tagRe.ReplaceAllString(cv.Source.Subject, "")))
	}
	if team := intercomRef(cv.Team); team != "" {
		t.Folder = team
	}
	switch {
	case cv.Source.Author.customer():
		t.Requester = cv.Source.Author.person()
	case len(cv.Contacts.Contacts) > 0:
		t.Requester = cv.Contacts.Contacts[0].person()
	}
	if id := intercomRef(cv.Admin); id != "" {
		if a, ok := admins[id]; ok {
			t.Assignee = &a
		}
	}
	for _, tag := range cv.Tags.Tags {
		t.Tags = append(t.Tags, tag.Name)
	}
	return t
}

type icAttachment struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	ContentType string `json:"content_type"`
	Filesize    int64  `json:"filesize"`
}

type icPart struct {
	ID          string         `json:"id"`
	PartType    string         `json:"part_type"`
	Body        string         `json:"body"`
	CreatedAt   int64          `json:"created_at"`
	Author      icAuthor       `json:"author"`
	Attachments []icAttachment `json:"attachments"`
}

// Messages reads a conversation's first message and its parts: comments from a user or lead in,
// from an admin or bot out, notes as notes; assignments, state changes and attribute updates are
// left out. Intercom returns at most the 500 most recent parts; a longer conversation comes back
// with a *TruncatedError next to what was returned.
func (i *Intercom) Messages(ctx context.Context, t Ticket) ([]Message, error) {
	var cv struct {
		Source struct {
			ID          string         `json:"id"`
			Body        string         `json:"body"`
			Author      icAuthor       `json:"author"`
			Attachments []icAttachment `json:"attachments"`
		} `json:"source"`
		Parts struct {
			Total int      `json:"total_count"`
			Parts []icPart `json:"conversation_parts"`
		} `json:"conversation_parts"`
	}
	if err := i.c.getJSON(ctx, ClassAPI, i.url("/conversations/"+url.PathEscape(t.ID), nil), &cv); err != nil {
		return nil, err
	}
	var out []Message
	first := icPart{ID: "source-" + cv.Source.ID, PartType: "comment", Body: cv.Source.Body, Author: cv.Source.Author, Attachments: cv.Source.Attachments}
	if m, ok := intercomMessage(first, t); ok {
		m.At = t.CreatedAt
		out = append(out, m)
	}
	for _, p := range cv.Parts.Parts {
		if m, ok := intercomMessage(p, t); ok {
			out = append(out, m)
		}
	}
	if n := len(cv.Parts.Parts); cv.Parts.Total > n || n >= intercomPartsCap {
		return out, &TruncatedError{Have: n, Total: max(cv.Parts.Total, n)}
	}
	return out, nil
}

func intercomMessage(p icPart, t Ticket) (Message, bool) {
	m := Message{ID: p.ID, At: time.Unix(p.CreatedAt, 0).UTC(), HTML: p.Body, Text: plainText(p.Body)}
	switch p.PartType {
	case "comment", "quick_reply":
	case "note":
		m.Note = true
	default:
		return Message{}, false
	}
	if m.Note || !p.Author.customer() {
		m.Direction, m.Author = "out", p.Author.person()
		m.Author.Agent = true
	} else {
		m.Direction, m.Author = "in", p.Author.person()
		if m.Author.ID == "" {
			m.Author = t.Requester
		}
	}
	for _, a := range p.Attachments {
		if (a.Type != "" && a.Type != "upload") || a.URL == "" {
			continue
		}
		m.Attachments = append(m.Attachments, Attachment{Name: a.Name, Type: a.ContentType, Size: a.Filesize, Ref: a.URL})
	}
	if m.Text == "" && len(m.Attachments) == 0 {
		return Message{}, false
	}
	return m, true
}

// Attachment fetches a file from the address Intercom gave for an upload, without the token. Only
// those addresses are fetched, never a link written in a message, and never one on an API host.
func (i *Intercom) Attachment(ctx context.Context, _ Ticket, a Attachment) ([]byte, error) {
	u, err := url.Parse(a.Ref)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || intercomAPIHost(u.Hostname()) {
		return nil, &RefusedError{Method: http.MethodGet, Host: a.Ref}
	}
	return i.c.get(ctx, i.c.files, ClassAPI, u.String(), "*/*")
}

func intercomAPIHost(host string) bool {
	for _, r := range []string{intercomRegionUS, intercomRegionEU, intercomRegionAU} {
		if strings.EqualFold(host, IntercomHost(r)) {
			return true
		}
	}
	return false
}

// Customer reads a contact's name, e-mail and phone; a contact Intercom no longer has stays as
// the conversation named it.
func (i *Intercom) Customer(ctx context.Context, p Person) (Person, error) {
	if p.ID == "" {
		return p, nil
	}
	var c struct {
		Name       string `json:"name"`
		Email      string `json:"email"`
		Phone      string `json:"phone"`
		ExternalID string `json:"external_id"`
	}
	if err := i.c.getJSON(ctx, ClassAPI, i.url("/contacts/"+url.PathEscape(p.ID), nil), &c); err != nil {
		var se *StatusError
		if errors.As(err, &se) && se.Status == http.StatusNotFound {
			return p, nil
		}
		return p, err
	}
	if name := strings.TrimSpace(c.Name); name != "" {
		p.Name = name
	}
	if email := strings.ToLower(strings.TrimSpace(c.Email)); email != "" {
		p.Email = email
	}
	p.Phone, p.ExternalID = c.Phone, c.ExternalID
	return p, nil
}
