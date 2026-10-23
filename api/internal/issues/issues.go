// Package issues talks to GitHub (REST, with a fine-grained personal access token) and Linear
// (GraphQL, with a personal API key): it checks a token, reads, finds and creates issues, maps an
// issue's state to a feedback progress and checks the signatures of their webhooks.
package issues

import (
	"bytes"
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
	GitHub = "github"
	Linear = "linear"

	GitHubAPI = "https://api.github.com"
	LinearAPI = "https://api.linear.app"

	ProgressPlanned    = "planned"
	ProgressInProgress = "in_progress"
	ProgressDone       = "done"
	ProgressWontDo     = "wont_do"

	maxAnswerBytes = 1 << 20
	linearSkew     = time.Minute
)

// Kind tells what went wrong with a call, so the API answers it the right way.
type Kind int

const (
	KindUnavailable Kind = iota
	KindAuth
	KindNotFound
	KindRejected
	KindInvalid
)

type Error struct {
	Kind Kind
	Msg  string
}

func (e *Error) Error() string { return e.Msg }

func fail(k Kind, format string, a ...any) error {
	return &Error{Kind: k, Msg: fmt.Sprintf(format, a...)}
}

// KindOf is KindUnavailable for any error that is not an *Error.
func KindOf(err error) Kind {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return KindUnavailable
}

type Issue struct {
	ExternalID string
	Key        string
	URL        string
	Title      string
	State      string
	Progress   string
}

type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// Client calls one tracker with one token. Base is the API's root: api.github.com, a GitHub
// Enterprise Server's /api/v3, or api.linear.app.
type Client struct {
	HTTP     Doer
	Provider string
	Base     string
	Token    string
	// WebHost is where people see GitHub issues: github.com, or the Enterprise Server's host.
	WebHost string
}

func (c *Client) Verify(ctx context.Context) (string, error) {
	if c.Provider == Linear {
		var out struct {
			Organization struct {
				Name string `json:"name"`
			} `json:"organization"`
		}
		if err := c.graphql(ctx, `query { viewer { id } organization { name } }`, nil, &out); err != nil {
			return "", err
		}
		return out.Organization.Name, nil
	}
	var u struct {
		Login string `json:"login"`
	}
	if err := c.rest(ctx, http.MethodGet, "/user", nil, &u); err != nil {
		return "", err
	}
	return u.Login, nil
}

// Find reads the issue a person named by its address or short form.
func (c *Client) Find(ctx context.Context, ref string) (Issue, error) {
	ref = strings.TrimSpace(ref)
	if c.Provider == Linear {
		id, ok := linearRef(ref)
		if !ok {
			return Issue{}, fail(KindInvalid, "not a Linear issue address or identifier")
		}
		return c.Get(ctx, id)
	}
	id, ok := c.githubRef(ref)
	if !ok {
		return Issue{}, fail(KindInvalid, "not a GitHub issue address or owner/repo#number")
	}
	return c.Get(ctx, id)
}

// Get reads an issue by the external id Find or Create returned.
func (c *Client) Get(ctx context.Context, externalID string) (Issue, error) {
	if c.Provider == Linear {
		var out struct {
			Issue *linearIssue `json:"issue"`
		}
		if err := c.graphql(ctx, `query($id: String!) { issue(id: $id) { id identifier title url state { name type } } }`, map[string]any{"id": externalID}, &out); err != nil {
			return Issue{}, err
		}
		if out.Issue == nil {
			return Issue{}, fail(KindNotFound, "Linear has no such issue")
		}
		return out.Issue.issue(), nil
	}
	repo, num, ok := splitGitHubID(externalID)
	if !ok {
		return Issue{}, fail(KindRejected, "not a GitHub issue")
	}
	var gi githubIssue
	if err := c.rest(ctx, http.MethodGet, "/repos/"+repo+"/issues/"+num, nil, &gi); err != nil {
		return Issue{}, err
	}
	if gi.PullRequest != nil {
		return Issue{}, fail(KindRejected, "a pull request, not an issue")
	}
	return gi.issue(repo), nil
}

// Create opens an issue in target: owner/repo on GitHub, a team key or id on Linear.
func (c *Client) Create(ctx context.Context, target, title, body string) (Issue, error) {
	if c.Provider == Linear {
		team, err := c.linearTeam(ctx, target)
		if err != nil {
			return Issue{}, err
		}
		var out struct {
			IssueCreate struct {
				Success bool         `json:"success"`
				Issue   *linearIssue `json:"issue"`
			} `json:"issueCreate"`
		}
		q := `mutation($input: IssueCreateInput!) { issueCreate(input: $input) { success issue { id identifier title url state { name type } } } }`
		if err := c.graphql(ctx, q, map[string]any{"input": map[string]any{"teamId": team, "title": title, "description": body}}, &out); err != nil {
			return Issue{}, err
		}
		if !out.IssueCreate.Success || out.IssueCreate.Issue == nil {
			return Issue{}, fail(KindRejected, "Linear did not create the issue")
		}
		return out.IssueCreate.Issue.issue(), nil
	}
	if !githubRepo.MatchString(target) {
		return Issue{}, fail(KindInvalid, "target must be owner/repo")
	}
	var gi githubIssue
	if err := c.rest(ctx, http.MethodPost, "/repos/"+target+"/issues", map[string]string{"title": title, "body": body}, &gi); err != nil {
		return Issue{}, err
	}
	return gi.issue(target), nil
}

// ValidTarget tells whether target can name where an issue is created.
func ValidTarget(provider, target string) bool {
	if provider == Linear {
		return linearTeamKey.MatchString(target) || uuidPattern.MatchString(target)
	}
	return githubRepo.MatchString(target)
}

var (
	githubRepo     = regexp.MustCompile(`^[A-Za-z0-9-]{1,39}/[A-Za-z0-9._-]{1,100}$`)
	githubShort    = regexp.MustCompile(`^([A-Za-z0-9-]{1,39}/[A-Za-z0-9._-]{1,100})#([0-9]{1,10})$`)
	githubPath     = regexp.MustCompile(`^/([A-Za-z0-9-]{1,39}/[A-Za-z0-9._-]{1,100})/issues/([0-9]{1,10})/?$`)
	linearKey      = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,9}-[0-9]{1,9}$`)
	linearTeamKey  = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,9}$`)
	linearPath     = regexp.MustCompile(`^/[^/]+/issue/([A-Za-z][A-Za-z0-9_]{0,9}-[0-9]{1,9})(/.*)?$`)
	uuidPattern    = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	githubNumberID = regexp.MustCompile(`^([a-z0-9-]{1,39}/[a-z0-9._-]{1,100})#([0-9]{1,10})$`)
)

// GitHubID is the external id of a GitHub issue: the repository in lower case, # and the number.
func GitHubID(repo string, number int) string {
	return strings.ToLower(repo) + "#" + strconv.Itoa(number)
}

func splitGitHubID(id string) (string, string, bool) {
	m := githubNumberID.FindStringSubmatch(id)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

func (c *Client) webHost() string {
	if c.WebHost == "" {
		return "github.com"
	}
	return strings.ToLower(c.WebHost)
}

func (c *Client) githubRef(ref string) (string, bool) {
	if m := githubShort.FindStringSubmatch(ref); m != nil {
		n, _ := strconv.Atoi(m[2])
		return GitHubID(m[1], n), true
	}
	u, err := url.Parse(ref)
	if err != nil || u.Scheme != "https" {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	if host != c.webHost() && !(c.webHost() == "github.com" && host == "www.github.com") {
		return "", false
	}
	m := githubPath.FindStringSubmatch(u.Path)
	if m == nil {
		return "", false
	}
	n, _ := strconv.Atoi(m[2])
	return GitHubID(m[1], n), true
}

func linearRef(ref string) (string, bool) {
	if linearKey.MatchString(ref) {
		return strings.ToUpper(ref), true
	}
	u, err := url.Parse(ref)
	if err != nil || u.Scheme != "https" || !strings.EqualFold(u.Hostname(), "linear.app") {
		return "", false
	}
	m := linearPath.FindStringSubmatch(u.Path)
	if m == nil {
		return "", false
	}
	return strings.ToUpper(m[1]), true
}

type githubIssue struct {
	Number      int             `json:"number"`
	HTMLURL     string          `json:"html_url"`
	Title       string          `json:"title"`
	State       string          `json:"state"`
	StateReason *string         `json:"state_reason"`
	PullRequest json.RawMessage `json:"pull_request"`
}

func (g githubIssue) issue(repo string) Issue {
	return Issue{
		ExternalID: GitHubID(repo, g.Number), Key: repo + "#" + strconv.Itoa(g.Number), URL: g.HTMLURL, Title: g.Title,
		State: g.State, Progress: GitHubProgress(g.State, g.StateReason),
	}
}

// GitHubProgress: open is planned; closed is done, or won't do when closed as not planned or as a
// duplicate.
func GitHubProgress(state string, reason *string) string {
	if state != "closed" {
		return ProgressPlanned
	}
	if reason != nil && (*reason == "not_planned" || *reason == "duplicate") {
		return ProgressWontDo
	}
	return ProgressDone
}

type linearIssue struct {
	ID         string `json:"id"`
	Identifier string `json:"identifier"`
	Title      string `json:"title"`
	URL        string `json:"url"`
	State      struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"state"`
}

func (l linearIssue) issue() Issue {
	return Issue{ExternalID: l.ID, Key: l.Identifier, URL: l.URL, Title: l.Title, State: l.State.Name, Progress: LinearProgress(l.State.Type)}
}

// LinearProgress maps a workflow state's type: started is in progress, completed done, canceled
// (and duplicate) won't do, and triage, backlog and unstarted planned.
func LinearProgress(stateType string) string {
	switch stateType {
	case "started":
		return ProgressInProgress
	case "completed":
		return ProgressDone
	case "canceled", "duplicate":
		return ProgressWontDo
	}
	return ProgressPlanned
}

func (c *Client) linearTeam(ctx context.Context, target string) (string, error) {
	if uuidPattern.MatchString(target) {
		return target, nil
	}
	if !linearTeamKey.MatchString(target) {
		return "", fail(KindInvalid, "target must be a Linear team key or id")
	}
	var out struct {
		Teams struct {
			Nodes []struct {
				ID string `json:"id"`
			} `json:"nodes"`
		} `json:"teams"`
	}
	if err := c.graphql(ctx, `query($key: String!) { teams(filter: { key: { eq: $key } }) { nodes { id } } }`, map[string]any{"key": strings.ToUpper(target)}, &out); err != nil {
		return "", err
	}
	if len(out.Teams.Nodes) == 0 {
		return "", fail(KindNotFound, "Linear has no team %s the key can see", strings.ToUpper(target))
	}
	return out.Teams.Nodes[0].ID, nil
}

func (c *Client) rest(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimSuffix(c.Base, "/")+path, body)
	if err != nil {
		return fail(KindRejected, "bad GitHub API URL")
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	raw, status, hdr, err := c.send(req)
	if err != nil {
		return err
	}
	switch {
	case status == http.StatusUnauthorized:
		return fail(KindAuth, "GitHub refused the token")
	case status == http.StatusForbidden || status == http.StatusTooManyRequests:
		if hdr.Get("X-RateLimit-Remaining") == "0" || hdr.Get("Retry-After") != "" {
			return fail(KindUnavailable, "GitHub's rate limit for the token is used up")
		}
		return fail(KindAuth, "the token may not do this: %s", githubMessage(raw))
	case status == http.StatusNotFound || status == http.StatusGone || (status >= 300 && status < 400):
		return fail(KindNotFound, "GitHub has no such issue or repository, or the token cannot see it")
	case status == http.StatusUnprocessableEntity || status == http.StatusBadRequest:
		return fail(KindRejected, "GitHub refused it: %s", githubMessage(raw))
	case status < 200 || status >= 300:
		return fail(KindUnavailable, "GitHub answered %d", status)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fail(KindUnavailable, "GitHub answered something that is not JSON")
	}
	return nil
}

func githubMessage(raw []byte) string {
	var e struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(raw, &e)
	if len(e.Message) > 200 {
		e.Message = e.Message[:200]
	}
	return e.Message
}

func (c *Client) graphql(ctx context.Context, query string, vars map[string]any, out any) error {
	b, err := json.Marshal(map[string]any{"query": query, "variables": vars})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(c.Base, "/")+"/graphql", bytes.NewReader(b))
	if err != nil {
		return fail(KindRejected, "bad Linear API URL")
	}
	req.Header.Set("Authorization", c.Token)
	req.Header.Set("Content-Type", "application/json")
	raw, status, _, err := c.send(req)
	if err != nil {
		return err
	}
	var res struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message    string `json:"message"`
			Extensions struct {
				Code string `json:"code"`
				Type string `json:"type"`
			} `json:"extensions"`
		} `json:"errors"`
	}
	_ = json.Unmarshal(raw, &res)
	for _, e := range res.Errors {
		code := strings.ToLower(e.Extensions.Code + " " + e.Extensions.Type)
		switch {
		case strings.Contains(code, "authentication") || strings.Contains(code, "forbidden"):
			return fail(KindAuth, "Linear refused the key")
		case strings.Contains(code, "ratelimited") || strings.Contains(code, "rate limited"):
			return fail(KindUnavailable, "Linear's rate limit for the key is used up")
		case strings.Contains(strings.ToLower(e.Message), "not found") || strings.Contains(code, "not_found"):
			return fail(KindNotFound, "Linear has no such issue or team, or the key cannot see it")
		}
	}
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return fail(KindAuth, "Linear refused the key")
	case status == http.StatusTooManyRequests || status >= 500:
		return fail(KindUnavailable, "Linear answered %d", status)
	case len(res.Errors) > 0:
		msg := res.Errors[0].Message
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return fail(KindRejected, "Linear refused it: %s", msg)
	case status < 200 || status >= 300 || len(res.Data) == 0:
		return fail(KindUnavailable, "Linear answered %d", status)
	}
	if err := json.Unmarshal(res.Data, out); err != nil {
		return fail(KindUnavailable, "Linear answered something unexpected")
	}
	return nil
}

func (c *Client) send(req *http.Request) ([]byte, int, http.Header, error) {
	req.Header.Set("User-Agent", "Yuva")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, nil, fail(KindUnavailable, "the tracker could not be reached")
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxAnswerBytes))
	if err != nil {
		return nil, 0, nil, fail(KindUnavailable, "the tracker's answer was cut off")
	}
	return raw, res.StatusCode, res.Header, nil
}

// VerifyGitHub checks X-Hub-Signature-256: sha256= and the hex HMAC-SHA256 of the body.
func VerifyGitHub(secret, body []byte, header string) bool {
	sig, ok := strings.CutPrefix(header, "sha256=")
	if !ok {
		return false
	}
	return equalMAC(secret, body, sig)
}

// VerifyLinear checks Linear-Signature, the hex HMAC-SHA256 of the body, and that the body's
// webhookTimestamp (milliseconds) is within a minute of now.
func VerifyLinear(secret, body []byte, header string, now time.Time) bool {
	if !equalMAC(secret, body, header) {
		return false
	}
	var b struct {
		WebhookTimestamp int64 `json:"webhookTimestamp"`
	}
	if json.Unmarshal(body, &b) != nil || b.WebhookTimestamp == 0 {
		return false
	}
	d := now.Sub(time.UnixMilli(b.WebhookTimestamp))
	return d < linearSkew && d > -linearSkew
}

func equalMAC(secret, body []byte, sig string) bool {
	got, err := hex.DecodeString(strings.TrimSpace(sig))
	if err != nil || len(got) != sha256.Size || len(secret) == 0 {
		return false
	}
	m := hmac.New(sha256.New, secret)
	m.Write(body)
	return hmac.Equal(got, m.Sum(nil))
}

var githubStateActions = map[string]bool{"opened": true, "closed": true, "reopened": true, "edited": true, "deleted": true, "transferred": true}

// GitHubEventIssues names the issues a GitHub webhook may have changed the state of.
func GitHubEventIssues(event string, body []byte) []string {
	if event != "issues" {
		return nil
	}
	var b struct {
		Action string `json:"action"`
		Issue  struct {
			Number int `json:"number"`
		} `json:"issue"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
	}
	if json.Unmarshal(body, &b) != nil || !githubStateActions[b.Action] || b.Issue.Number == 0 || b.Repository.FullName == "" {
		return nil
	}
	return []string{GitHubID(b.Repository.FullName, b.Issue.Number)}
}

// LinearEventIssues names the issue of a Linear Issue webhook.
func LinearEventIssues(body []byte) []string {
	var b struct {
		Type string `json:"type"`
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &b) != nil || b.Type != "Issue" || b.Data.ID == "" {
		return nil
	}
	return []string{b.Data.ID}
}
