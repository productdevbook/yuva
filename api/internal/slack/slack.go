// Package slack talks to Slack's Web API at slack.com and checks the signatures of its event
// requests.
package slack

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// APIBase is where every call goes; only tests point a Client elsewhere.
	APIBase      = "https://slack.com/api/"
	AuthorizeURL = "https://slack.com/oauth/v2/authorize"
	// ReplayWindow is how far an event's timestamp may be from the server's clock.
	ReplayWindow = 5 * time.Minute
	maxAnswer    = 1 << 20
	attempts     = 3
	// maxWait is the longest Retry-After a call sleeps through; a longer one is returned.
	maxWait = 10 * time.Second
)

// Scopes are the bot scopes the install asks for and the connection needs.
var Scopes = []string{
	"chat:write", "channels:read", "groups:read", "channels:history", "groups:history",
	"im:write", "users:read", "users:read.email",
}

// Error is Slack's refusal of a call: its error code, or the HTTP status when there was none.
// RetryAfter is set for a rate limit.
type Error struct {
	Code       string
	Status     int
	RetryAfter time.Duration
}

func (e *Error) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("slack: %s (retry after %s)", e.Code, e.RetryAfter)
	}
	return "slack: " + e.Code
}

// Temporary reports a refusal that may pass on a later try.
func (e *Error) Temporary() bool {
	return e.RetryAfter > 0 || e.Status >= 500 || e.Code == "internal_error" || e.Code == "service_unavailable" || e.Code == "fatal_error" || e.Code == "request_timeout"
}

// Gone reports a refusal that means the token no longer works.
func (e *Error) Gone() bool {
	switch e.Code {
	case "invalid_auth", "account_inactive", "token_revoked", "token_expired", "not_authed", "team_access_not_granted":
		return true
	}
	return false
}

// Client calls the Web API. Sleep waits out short rate limits and server errors between tries.
type Client struct {
	HTTP  *http.Client
	Base  string
	Sleep func(ctx context.Context, d time.Duration) error
}

func (c *Client) base() string {
	if c.Base != "" {
		return strings.TrimRight(c.Base, "/") + "/"
	}
	return APIBase
}

func (c *Client) sleep(ctx context.Context, d time.Duration) error {
	if c.Sleep != nil {
		return c.Sleep(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

type envelope struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
}

// call posts a form to a Web API method with up to three tries: a rate limit is waited out when
// Slack asks for at most maxWait, a server error or a failed connection after 1 and 2 seconds.
func (c *Client) call(ctx context.Context, method, token string, form url.Values, basic [2]string, out any) error {
	var last error
	for try := range attempts {
		err := c.once(ctx, method, token, form, basic, out)
		if err == nil {
			return nil
		}
		last = err
		var wait time.Duration
		var se *Error
		switch {
		case errors.As(err, &se) && se.RetryAfter > 0:
			if se.RetryAfter > maxWait || try == attempts-1 {
				return err
			}
			wait = se.RetryAfter
		case errors.As(err, &se) && !se.Temporary():
			return err
		case try == attempts-1:
			return err
		default:
			wait = time.Duration(try+1) * time.Second
		}
		if err := c.sleep(ctx, wait); err != nil {
			return err
		}
	}
	return last
}

func (c *Client) once(ctx context.Context, method, token string, form url.Values, basic [2]string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base()+method, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	switch {
	case token != "":
		req.Header.Set("Authorization", "Bearer "+token)
	case basic[0] != "":
		req.SetBasicAuth(basic[0], basic[1])
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	res, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("slack did not answer %s: %w", method, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxAnswer))
	if err != nil {
		return err
	}
	if res.StatusCode == http.StatusTooManyRequests {
		secs, _ := strconv.Atoi(res.Header.Get("Retry-After"))
		return &Error{Code: "ratelimited", Status: res.StatusCode, RetryAfter: time.Duration(max(secs, 1)) * time.Second}
	}
	if res.StatusCode != http.StatusOK {
		return &Error{Code: "http_" + strconv.Itoa(res.StatusCode), Status: res.StatusCode}
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return &Error{Code: "invalid_answer", Status: res.StatusCode}
	}
	if !env.OK {
		code := env.Error
		if code == "" {
			code = "unknown_error"
		}
		return &Error{Code: code, Status: res.StatusCode}
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return &Error{Code: "invalid_answer", Status: res.StatusCode}
		}
	}
	return nil
}

// Install is what an OAuth v2 install grants.
type Install struct {
	BotToken   string
	TokenType  string
	Scopes     []string
	BotUserID  string
	AppID      string
	TeamID     string
	TeamName   string
	Enterprise bool
}

// Exchange trades an install's code for the bot token at oauth.v2.access.
func (c *Client) Exchange(ctx context.Context, clientID, clientSecret, code, redirect string) (Install, error) {
	var a struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		Scope       string `json:"scope"`
		BotUserID   string `json:"bot_user_id"`
		AppID       string `json:"app_id"`
		Team        struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"team"`
		Enterprise bool `json:"is_enterprise_install"`
	}
	err := c.call(ctx, "oauth.v2.access", "", url.Values{"code": {code}, "redirect_uri": {redirect}}, [2]string{clientID, clientSecret}, &a)
	if err != nil {
		return Install{}, err
	}
	scopes := strings.FieldsFunc(a.Scope, func(r rune) bool { return r == ',' || r == ' ' })
	return Install{
		BotToken: a.AccessToken, TokenType: a.TokenType, Scopes: scopes, BotUserID: a.BotUserID, AppID: a.AppID,
		TeamID: a.Team.ID, TeamName: a.Team.Name, Enterprise: a.Enterprise,
	}, nil
}

// MissingScopes lists the Scopes a grant lacks.
func MissingScopes(granted []string) []string {
	var out []string
	for _, s := range Scopes {
		found := false
		for _, g := range granted {
			if g == s {
				found = true
				break
			}
		}
		if !found {
			out = append(out, s)
		}
	}
	return out
}

// Revoke ends the token at auth.revoke.
func (c *Client) Revoke(ctx context.Context, token string) error {
	return c.call(ctx, "auth.revoke", token, url.Values{}, [2]string{}, nil)
}

// Message is a post; ThreadTS makes it a reply in that thread.
type Message struct {
	Channel  string
	Text     string
	ThreadTS string
}

// Post sends a message with chat.postMessage and returns its timestamp.
func (c *Client) Post(ctx context.Context, token string, m Message) (string, error) {
	form := url.Values{"channel": {m.Channel}, "text": {m.Text}, "unfurl_links": {"false"}, "unfurl_media": {"false"}}
	if m.ThreadTS != "" {
		form.Set("thread_ts", m.ThreadTS)
	}
	var a struct {
		TS string `json:"ts"`
	}
	if err := c.call(ctx, "chat.postMessage", token, form, [2]string{}, &a); err != nil {
		return "", err
	}
	return a.TS, nil
}

// PostEphemeral shows a message to one user in a channel (and thread) only.
func (c *Client) PostEphemeral(ctx context.Context, token string, m Message, user string) error {
	form := url.Values{"channel": {m.Channel}, "text": {m.Text}, "user": {user}}
	if m.ThreadTS != "" {
		form.Set("thread_ts", m.ThreadTS)
	}
	return c.call(ctx, "chat.postEphemeral", token, form, [2]string{}, nil)
}

// Channel is a channel the bot is a member of.
type Channel struct {
	ID      string
	Name    string
	Private bool
}

// Channels lists the bot's channels, public and private and not archived, at most limit of them.
func (c *Client) Channels(ctx context.Context, token string, limit int) ([]Channel, error) {
	var out []Channel
	cursor := ""
	for len(out) < limit {
		form := url.Values{"types": {"public_channel,private_channel"}, "exclude_archived": {"true"}, "limit": {"200"}}
		if cursor != "" {
			form.Set("cursor", cursor)
		}
		var a struct {
			Channels []struct {
				ID        string `json:"id"`
				Name      string `json:"name"`
				IsPrivate bool   `json:"is_private"`
			} `json:"channels"`
			Meta struct {
				NextCursor string `json:"next_cursor"`
			} `json:"response_metadata"`
		}
		if err := c.call(ctx, "users.conversations", token, form, [2]string{}, &a); err != nil {
			return nil, err
		}
		for _, ch := range a.Channels {
			if len(out) < limit {
				out = append(out, Channel{ID: ch.ID, Name: ch.Name, Private: ch.IsPrivate})
			}
		}
		if cursor = a.Meta.NextCursor; cursor == "" {
			break
		}
	}
	return out, nil
}

// User is a Slack user.
type User struct {
	ID   string
	Name string
}

// UserByEmail finds the workspace's user with an e-mail address.
func (c *Client) UserByEmail(ctx context.Context, token, email string) (User, error) {
	var a struct {
		User struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Deleted bool   `json:"deleted"`
			IsBot   bool   `json:"is_bot"`
			Profile struct {
				RealName    string `json:"real_name"`
				DisplayName string `json:"display_name"`
			} `json:"profile"`
		} `json:"user"`
	}
	if err := c.call(ctx, "users.lookupByEmail", token, url.Values{"email": {email}}, [2]string{}, &a); err != nil {
		return User{}, err
	}
	if a.User.Deleted || a.User.IsBot {
		return User{}, &Error{Code: "users_not_found", Status: http.StatusOK}
	}
	name := a.User.Profile.DisplayName
	if name == "" {
		name = a.User.Profile.RealName
	}
	if name == "" {
		name = a.User.Name
	}
	return User{ID: a.User.ID, Name: name}, nil
}

// DirectMessage sends a user a direct message from the bot.
func (c *Client) DirectMessage(ctx context.Context, token, user, text string) error {
	var a struct {
		Channel struct {
			ID string `json:"id"`
		} `json:"channel"`
	}
	if err := c.call(ctx, "conversations.open", token, url.Values{"users": {user}}, [2]string{}, &a); err != nil {
		return err
	}
	_, err := c.Post(ctx, token, Message{Channel: a.Channel.ID, Text: text})
	return err
}

// Verify checks an event request's X-Slack-Signature and X-Slack-Request-Timestamp against the
// signing secret and the replay window.
func Verify(secret, timestamp, signature string, body []byte, now time.Time) error {
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return errors.New("no request timestamp")
	}
	if d := now.Sub(time.Unix(ts, 0)); d > ReplayWindow || d < -ReplayWindow {
		return errors.New("request timestamp outside the replay window")
	}
	got, ok := strings.CutPrefix(signature, "v0=")
	if !ok {
		return errors.New("no v0 signature")
	}
	sig, err := hex.DecodeString(got)
	if err != nil {
		return errors.New("signature is not hex")
	}
	if !hmac.Equal(sig, Sign(secret, timestamp, body)) {
		return errors.New("signature does not match")
	}
	return nil
}

// Sign is the HMAC-SHA256 Slack sends as v0=<hex>.
func Sign(secret, timestamp string, body []byte) []byte {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte("v0:" + timestamp + ":"))
	m.Write(body)
	return m.Sum(nil)
}

var escaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

// Escape makes text plain in Slack's mrkdwn: no links, mentions or @channel from what it holds.
func Escape(text string) string { return escaper.Replace(text) }

var (
	linkRe    = regexp.MustCompile(`<((?:https?|mailto):[^|>]+)(?:\|([^>]*))?>`)
	userRe    = regexp.MustCompile(`<@([UW][A-Z0-9]+)(?:\|[^>]*)?>`)
	specialRe = regexp.MustCompile(`<!(?:subteam\^[A-Z0-9]+|[a-z]+)(?:\|([^>]*))?>`)
	channelRe = regexp.MustCompile(`<#[CG][A-Z0-9]+(?:\|([^>]*))?>`)
	unescaper = strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">")
)

// PlainText turns a message's mrkdwn into text: links become "label (url)", user mentions go
// through name (which returns "" for an unknown user), and entities are decoded.
func PlainText(text string, name func(user string) string) string {
	text = userRe.ReplaceAllStringFunc(text, func(m string) string {
		id := userRe.FindStringSubmatch(m)[1]
		if n := name(id); n != "" {
			return "@" + n
		}
		return "@" + id
	})
	text = linkRe.ReplaceAllStringFunc(text, func(m string) string {
		p := linkRe.FindStringSubmatch(m)
		if p[2] == "" || p[2] == p[1] {
			return p[1]
		}
		return p[2] + " (" + p[1] + ")"
	})
	text = channelRe.ReplaceAllString(text, "#$1")
	text = specialRe.ReplaceAllStringFunc(text, func(m string) string {
		if p := specialRe.FindStringSubmatch(m); p[1] != "" {
			return p[1]
		}
		return "@" + strings.TrimSuffix(strings.TrimPrefix(m, "<!"), ">")
	})
	return unescaper.Replace(text)
}

// MentionedUsers lists the user ids a message mentions.
func MentionedUsers(text string) []string {
	var out []string
	for _, m := range userRe.FindAllStringSubmatch(text, -1) {
		out = append(out, m[1])
	}
	return out
}
