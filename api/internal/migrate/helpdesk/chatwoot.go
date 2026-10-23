package helpdesk

import (
	"context"
	"mime"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

const (
	chatwootConversationsPage = 25
	chatwootMessagesPage      = 20
	chatwootMaxMessagePages   = 5000
)

// ChatwootBudget is a Chatwoot install's budget: it sends no limit headers, so a fixed 60 requests
// a minute.
func ChatwootBudget(b *Budget) *Budget { return chatwootDesk.Budget(b) }

// Chatwoot reads one account of a Chatwoot install with the access token of an administrator.
// Chatwoot has no read-only token: the GET-only transport is what keeps this reader from
// changing anything.
type Chatwoot struct {
	c       *client
	base    string
	account string
}

// NewChatwoot reads account on the install at baseURL. The token goes only to that host;
// attachments are fetched without it from anywhere else.
func NewChatwoot(cfg Config, baseURL, account string, token func() string, budget *Budget) *Chatwoot {
	a := Account{BaseURL: baseURL, ID: account}
	return &Chatwoot{base: baseURL, account: account, c: chatwootDesk.client(cfg, a, Credentials{Token: token}, budget)}
}

func (w *Chatwoot) url(path string, q url.Values) string {
	return withQuery(w.base+path, q)
}

func (w *Chatwoot) accountURL(path string, q url.Values) string {
	return w.url("/api/v1/accounts/"+url.PathEscape(w.account)+path, q)
}

type cwContact struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Email       string `json:"email"`
	PhoneNumber string `json:"phone_number"`
	Identifier  string `json:"identifier"`
}

func (c cwContact) person() Person {
	return Person{
		ID: idString(c.ID), Name: strings.TrimSpace(c.Name), Email: strings.ToLower(strings.TrimSpace(c.Email)),
		Phone: c.PhoneNumber, ExternalID: c.Identifier,
	}
}

type cwAgent struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

func (a cwAgent) person() Person {
	return Person{
		ID: idString(a.ID), Name: strings.TrimSpace(a.Name), Email: strings.ToLower(strings.TrimSpace(a.Email)),
		Agent: true, Admin: a.Role == "administrator",
	}
}

// Me reads the profile of the token's user, who must belong to the account; Admin is the user's
// role in that account.
func (w *Chatwoot) Me(ctx context.Context) (Person, error) {
	var out struct {
		ID       int64  `json:"id"`
		Name     string `json:"name"`
		Email    string `json:"email"`
		Accounts []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
			Role string `json:"role"`
		} `json:"accounts"`
	}
	if err := w.c.getJSON(ctx, ClassAPI, w.url("/api/v1/profile", nil), &out); err != nil {
		return Person{}, err
	}
	var listed []string
	for _, a := range out.Accounts {
		if idString(a.ID) == w.account {
			return cwAgent{ID: out.ID, Name: out.Name, Email: out.Email, Role: a.Role}.person(), nil
		}
		listed = append(listed, idString(a.ID)+" "+strings.TrimSpace(a.Name))
	}
	return Person{}, &AuthError{StatusError{Status: 403, Path: "/api/v1/profile", Body: "the token's user is not in account " + w.account + "; its accounts: " + strings.Join(listed, ", ")}}
}

// Folders are the account's inboxes.
func (w *Chatwoot) Folders(ctx context.Context) ([]Folder, error) {
	var out struct {
		Payload []struct {
			ID    int64  `json:"id"`
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"payload"`
	}
	if err := w.c.getJSON(ctx, ClassAPI, w.accountURL("/inboxes", nil), &out); err != nil {
		return nil, err
	}
	folders := make([]Folder, 0, len(out.Payload))
	for _, i := range out.Payload {
		folders = append(folders, Folder{ID: idString(i.ID), Name: i.Name, Address: strings.ToLower(strings.TrimSpace(i.Email))})
	}
	return folders, nil
}

func (w *Chatwoot) Agents(ctx context.Context) ([]Person, error) {
	var agents []cwAgent
	if err := w.c.getJSON(ctx, ClassAPI, w.accountURL("/agents", nil), &agents); err != nil {
		return nil, err
	}
	out := make([]Person, 0, len(agents))
	for _, a := range agents {
		out = append(out, a.person())
	}
	return out, nil
}

func chatwootStatus(s string) string {
	switch s {
	case "pending", "snoozed":
		return StatusPending
	case "resolved":
		return StatusClosed
	}
	return StatusOpen
}

func unixTime(v int64) time.Time {
	if v <= 0 {
		return time.Time{}
	}
	return time.Unix(v, 0).UTC()
}

// Tickets reads a page of conversations of every status, the most recently active first. With
// c.ModifiedSince set the listing ends at the first conversation last active before it.
func (w *Chatwoot) Tickets(ctx context.Context, c Cursor) (TicketPage, error) {
	page := max(c.Page, 1)
	q := url.Values{"status": {"all"}, "assignee_type": {"all"}, "sort_by": {"last_activity_at_desc"}, "page": {strconv.Itoa(page)}}
	var res struct {
		Data struct {
			Payload []struct {
				ID             int64    `json:"id"`
				Status         string   `json:"status"`
				InboxID        int64    `json:"inbox_id"`
				Labels         []string `json:"labels"`
				CreatedAt      int64    `json:"created_at"`
				LastActivityAt int64    `json:"last_activity_at"`
				Timestamp      int64    `json:"timestamp"`
				Meta           struct {
					Sender   cwContact `json:"sender"`
					Channel  string    `json:"channel"`
					Assignee *cwAgent  `json:"assignee"`
				} `json:"meta"`
				AdditionalAttributes struct {
					MailSubject string `json:"mail_subject"`
				} `json:"additional_attributes"`
			} `json:"payload"`
		} `json:"data"`
	}
	if err := w.c.getJSON(ctx, ClassAPI, w.accountURL("/conversations", q), &res); err != nil {
		return TicketPage{}, err
	}
	out := TicketPage{Next: c, End: len(res.Data.Payload) < chatwootConversationsPage}
	out.Next.Page = page + 1
	for _, cv := range res.Data.Payload {
		t := Ticket{
			ID: idString(cv.ID), Folder: idString(cv.InboxID), Subject: strings.TrimSpace(cv.AdditionalAttributes.MailSubject),
			Status: chatwootStatus(cv.Status), Requester: cv.Meta.Sender.person(), Tags: cv.Labels, Via: cv.Meta.Channel,
			CreatedAt: unixTime(cv.CreatedAt),
		}
		t.UpdatedAt = unixTime(max(cv.LastActivityAt, cv.Timestamp))
		if t.UpdatedAt.Before(t.CreatedAt) {
			t.UpdatedAt = t.CreatedAt
		}
		if !c.ModifiedSince.IsZero() && t.UpdatedAt.Before(c.ModifiedSince) {
			out.End = true
			break
		}
		if cv.Meta.Assignee != nil && cv.Meta.Assignee.ID != 0 {
			t.Assignee = new(cv.Meta.Assignee.person())
		}
		out.Tickets = append(out.Tickets, t)
	}
	return out, nil
}

type cwMessage struct {
	ID          int64  `json:"id"`
	Content     string `json:"content"`
	MessageType int    `json:"message_type"`
	Private     bool   `json:"private"`
	CreatedAt   int64  `json:"created_at"`
	SenderType  string `json:"sender_type"`
	Sender      *struct {
		ID    int64  `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"sender"`
	ContentAttributes struct {
		Email struct {
			HTML struct {
				Full string `json:"full"`
			} `json:"html_content"`
			Text struct {
				Full string `json:"full"`
			} `json:"text_content"`
		} `json:"email"`
	} `json:"content_attributes"`
	Attachments []struct {
		FileType  string `json:"file_type"`
		Extension string `json:"extension"`
		DataURL   string `json:"data_url"`
		FileSize  int64  `json:"file_size"`
	} `json:"attachments"`
}

// Messages reads a conversation's messages backwards, twenty at a time with before=<oldest id>,
// and gives them oldest first: incoming and outgoing messages, private ones as notes; activity
// lines are left out.
func (w *Chatwoot) Messages(ctx context.Context, t Ticket) ([]Message, error) {
	var newestFirst [][]Message
	var before int64
	for range chatwootMaxMessagePages {
		q := url.Values{}
		if before != 0 {
			q.Set("before", idString(before))
		}
		var page struct {
			Payload []cwMessage `json:"payload"`
		}
		if err := w.c.getJSON(ctx, ClassAPI, w.accountURL("/conversations/"+url.PathEscape(t.ID)+"/messages", q), &page); err != nil {
			return nil, err
		}
		if len(page.Payload) == 0 {
			break
		}
		oldest := page.Payload[0].ID
		var msgs []Message
		for _, cm := range page.Payload {
			oldest = min(oldest, cm.ID)
			if m, ok := chatwootMessage(cm, t); ok {
				msgs = append(msgs, m)
			}
		}
		newestFirst = append(newestFirst, msgs)
		if len(page.Payload) < chatwootMessagesPage || before != 0 && oldest >= before {
			break
		}
		before = oldest
	}
	var out []Message
	for i := len(newestFirst) - 1; i >= 0; i-- {
		out = append(out, newestFirst[i]...)
	}
	return out, nil
}

func chatwootMessage(cm cwMessage, t Ticket) (Message, bool) {
	m := Message{ID: idString(cm.ID), At: unixTime(cm.CreatedAt), Text: cm.Content, Note: cm.Private}
	switch cm.MessageType {
	case 0:
		m.Direction = "in"
	case 1, 3:
		m.Direction = "out"
	default:
		return Message{}, false
	}
	agent := cm.SenderType == "User" || cm.SenderType == "AgentBot"
	if cm.Sender != nil && cm.Sender.ID != 0 {
		m.Author = Person{ID: idString(cm.Sender.ID), Name: strings.TrimSpace(cm.Sender.Name), Email: strings.ToLower(strings.TrimSpace(cm.Sender.Email)), Agent: agent}
	}
	if m.Direction == "in" && !agent && (cm.SenderType != "Contact" || m.Author.ID == "") {
		m.Author = t.Requester
	}
	if m.Note {
		m.Author.Agent = true
	}
	if h := cm.ContentAttributes.Email.HTML.Full; h != "" && !m.Note {
		m.HTML = h
		if txt := cm.ContentAttributes.Email.Text.Full; txt != "" {
			m.Text = txt
		}
	}
	for _, a := range cm.Attachments {
		if a.DataURL == "" || a.FileType == "fallback" || a.FileType == "location" {
			continue
		}
		name := ""
		if u, err := url.Parse(a.DataURL); err == nil {
			name = path.Base(u.Path)
		}
		if name == "" || name == "." || name == "/" {
			name = "attachment-" + m.ID
			if ext := strings.TrimPrefix(a.Extension, "."); ext != "" {
				name += "." + ext
			}
		}
		typ := mime.TypeByExtension(path.Ext(name))
		if typ == "" {
			typ = "application/octet-stream"
		}
		m.Attachments = append(m.Attachments, Attachment{Name: name, Type: typ, Size: a.FileSize, Ref: a.DataURL})
	}
	return m, true
}

// Attachment fetches a file from the instance's storage URL: with the token from the install's
// own host, without it from anywhere else.
func (w *Chatwoot) Attachment(ctx context.Context, _ Ticket, a Attachment) ([]byte, error) {
	u, err := url.Parse(a.Ref)
	if err != nil || u.Scheme != "https" {
		return nil, &RefusedError{Method: "GET", Host: a.Ref}
	}
	return w.c.get(ctx, w.c.files, ClassAPI, u.String(), "*/*")
}

// Customer: Chatwoot's conversations already name the contact in full.
func (w *Chatwoot) Customer(_ context.Context, p Person) (Person, error) { return p, nil }
