package helpdesk

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	errBodyBytes = 300
	// statusEnhanceYourCalm is the 420 some help desks answer instead of 429.
	statusEnhanceYourCalm = 420
)

// ErrTooLarge is a body over Config.MaxBytes.
var ErrTooLarge = errors.New("the answer is larger than the limit")

// Person is a requester, customer or agent.
type Person struct {
	ID         string `json:"id"`
	Name       string `json:"name,omitempty"`
	Email      string `json:"email,omitempty"`
	Phone      string `json:"phone,omitempty"`
	ExternalID string `json:"external_id,omitempty"`
	Agent      bool   `json:"agent,omitempty"`
	Admin      bool   `json:"admin,omitempty"`
}

// Folder is a Zendesk brand or a Help Scout mailbox.
type Folder struct {
	ID      string
	Name    string
	Address string
	Count   int64
}

// Ticket statuses, as Yuva reads them.
const (
	StatusOpen    = "open"
	StatusPending = "pending"
	StatusClosed  = "closed"
	StatusSpam    = "spam"
	StatusDeleted = "deleted"
)

// Ticket is a Zendesk ticket or a Help Scout conversation, kept on its migration item.
type Ticket struct {
	ID        string    `json:"id"`
	Folder    string    `json:"folder"`
	Subject   string    `json:"subject,omitempty"`
	Status    string    `json:"status"`
	Requester Person    `json:"requester"`
	Assignee  *Person   `json:"assignee,omitempty"`
	Tags      []string  `json:"tags,omitempty"`
	Via       string    `json:"via,omitempty"`
	Messages  int       `json:"messages,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Cursor is where a listing of tickets stands: Zendesk's incremental cursor (Start on the first
// request), Help Scout's page of conversations modified since a time.
type Cursor struct {
	Start         int64     `json:"start,omitempty"`
	After         string    `json:"after,omitempty"`
	Page          int       `json:"page,omitempty"`
	Inbox         string    `json:"inbox,omitempty"`
	ModifiedSince time.Time `json:"modified_since,omitzero"`
	StartedAt     time.Time `json:"started_at,omitzero"`
}

type TicketPage struct {
	Tickets []Ticket
	Next    Cursor
	End     bool
}

// Message is a comment or thread of a ticket: a message in or out, or a note.
type Message struct {
	ID          string
	Note        bool
	Direction   string
	Author      Person
	At          time.Time
	Text        string
	HTML        string
	Raw         []byte
	Attachments []Attachment
}

// Attachment is a file of a message; Ref is how the reader fetches it.
type Attachment struct {
	Name   string
	Type   string
	Size   int64
	Inline bool
	Ref    string
}

// Source is a help desk account a migration reads.
type Source interface {
	Me(ctx context.Context) (Person, error)
	Folders(ctx context.Context) ([]Folder, error)
	Agents(ctx context.Context) ([]Person, error)
	Tickets(ctx context.Context, c Cursor) (TicketPage, error)
	Messages(ctx context.Context, t Ticket) ([]Message, error)
	Attachment(ctx context.Context, t Ticket, a Attachment) ([]byte, error)
	Customer(ctx context.Context, p Person) (Person, error)
}

// client sends the reader's GETs within the budget.
type client struct {
	cfg          Config
	api          *http.Client
	files        *http.Client
	budget       *Budget
	retryHeaders []string
}

// inlineWait is the longest wait for the incremental exports a reader sleeps through; a longer
// one ends the caller's slice. The pace between other requests is always slept through, so a read
// that takes several requests gets them.
const inlineWait = 2 * time.Second

func (c *client) get(ctx context.Context, hc *http.Client, class Class, rawURL, accept string) ([]byte, error) {
	for {
		now := c.cfg.now()
		err := c.budget.Take(class, now)
		var wait *WaitError
		if errors.As(err, &wait) && (!wait.Incremental && !wait.Daily || wait.Until.Sub(now) <= inlineWait) {
			if err := c.cfg.sleep(ctx, wait.Until.Sub(now)); err != nil {
				return nil, err
			}
			continue
		}
		if err != nil {
			return nil, err
		}
		break
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	if accept == "" {
		accept = "application/json"
	}
	req.Header.Set("Accept", accept)
	res, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	c.budget.Saw(res.Header)
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return nil, c.failure(res, req.URL.Path)
	}
	limit := c.cfg.MaxBytes
	if limit <= 0 {
		limit = 64 << 20
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, ErrTooLarge
	}
	return b, nil
}

func (c *client) getJSON(ctx context.Context, class Class, rawURL string, out any) error {
	b, err := c.get(ctx, c.api, class, rawURL, "")
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

func (c *client) failure(res *http.Response, path string) error {
	b, _ := io.ReadAll(io.LimitReader(res.Body, errBodyBytes))
	se := StatusError{Status: res.StatusCode, Path: path, Body: strings.TrimSpace(string(b))}
	switch res.StatusCode {
	case http.StatusUnauthorized:
		return &AuthError{se}
	case http.StatusTooManyRequests, statusEnhanceYourCalm:
		wait := time.Minute
		for _, h := range c.retryHeaders {
			if v, err := strconv.Atoi(strings.TrimSpace(res.Header.Get(h))); err == nil && v >= 0 {
				wait = time.Duration(v) * time.Second
				break
			}
		}
		return &LimitError{Until: c.cfg.now().Add(max(wait, time.Second))}
	}
	return &se
}

func withQuery(base string, q url.Values) string {
	if len(q) == 0 {
		return base
	}
	return base + "?" + q.Encode()
}

func idString(v int64) string { return strconv.FormatInt(v, 10) }
