package helpdesktest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	FreshdeskSubdomain = "acme"
	FreshdeskKey       = "fd-admin-key-1234"
	FreshdeskAgentKey  = "fd-agent-key-5678"
	FreshdeskLimit     = 40
	FreshdeskFilesHost = "cdn.freshdesk.com"
	FreshdeskS3Host    = "s3.eu-west-1.amazonaws.com"
	freshdeskCap       = 300
)

type freshdeskState struct {
	throttle int
	cap      int
	served   map[string]int
}

// ThrottleFreshdesk answers the next n Freshdesk API requests with 429 and Retry-After: 7.
func (s *Server) ThrottleFreshdesk(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fresh.throttle = n
}

// CapFreshdeskPages makes Freshdesk refuse a ticket page beyond n, as it does beyond page 300.
func (s *Server) CapFreshdeskPages(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fresh.cap = n
}

func (s *Server) freshdeskFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Query().Get("sig") == "fresh" {
		s.attachment(w, r.URL.Path)
		return
	}
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(`<Error><Code>AccessDenied</Code><Message>Request has expired</Message></Error>`))
}

func freshdeskJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) freshdesk(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Ratelimit-Total", fmt.Sprint(FreshdeskLimit))
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	user, password, ok := r.BasicAuth()
	if !ok || password != "X" || (user != FreshdeskKey && user != FreshdeskAgentKey) {
		freshdeskJSON(w, http.StatusUnauthorized, map[string]string{"code": "invalid_credentials", "message": "You have to be logged in to perform this action."})
		return
	}
	s.mu.Lock()
	throttled := s.fresh.throttle > 0
	if throttled {
		s.fresh.throttle--
	}
	s.mu.Unlock()
	if throttled {
		w.Header().Set("Retry-After", "7")
		freshdeskJSON(w, http.StatusTooManyRequests, map[string]string{"code": "rate_limit", "message": "You have exceeded the limit of requests per hour"})
		return
	}
	q := r.URL.Query()
	page, perPage := max(atoi(q.Get("page"), 1), 1), atoi(q.Get("per_page"), 30)
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v2"), "/"), "/")
	switch {
	case len(parts) == 2 && parts[0] == "agents" && parts[1] == "me":
		name := "freshdesk/agents_me.json"
		if user == FreshdeskAgentKey {
			name = "freshdesk/agents_me_agent.json"
		}
		s.json(w, r, name)
	case len(parts) == 1 && parts[0] == "roles":
		if user == FreshdeskAgentKey {
			freshdeskJSON(w, http.StatusForbidden, map[string]string{"code": "access_denied", "message": "You are not authorized to perform this action."})
			return
		}
		s.json(w, r, "freshdesk/roles.json")
	case len(parts) == 1 && (parts[0] == "agents" || parts[0] == "products" || parts[0] == "email_configs"):
		s.freshdeskPage(w, r, "freshdesk/"+parts[0]+".json", page, perPage, nil)
	case len(parts) == 1 && parts[0] == "tickets":
		s.mu.Lock()
		limit := s.fresh.cap
		s.mu.Unlock()
		if limit == 0 {
			limit = freshdeskCap
		}
		if page > limit {
			freshdeskJSON(w, http.StatusBadRequest, map[string]any{"description": "Validation failed", "errors": []map[string]string{{"field": "page", "message": "Page number is beyond the limit", "code": "invalid_value"}}})
			return
		}
		since, _ := time.Parse(time.RFC3339, q.Get("updated_since"))
		s.freshdeskPage(w, r, "freshdesk/tickets.json", page, perPage, func(t map[string]any) bool {
			at, _ := time.Parse(time.RFC3339, fmt.Sprint(t["updated_at"]))
			return !at.Before(since)
		})
	case len(parts) == 2 && parts[0] == "tickets":
		s.freshdeskTemplated(w, r, "freshdesk/ticket_"+parts[1]+".json", "")
	case len(parts) == 3 && parts[0] == "tickets" && parts[2] == "conversations":
		s.freshdeskPage(w, r, "freshdesk/conversations_"+parts[1]+".json", page, perPage, nil)
	default:
		http.NotFound(w, r)
	}
}

func atoi(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}

// freshdeskTemplated answers a fixture whose {{SIG}} is the signature of its attachment URLs:
// stale the first time the fixture is served under key, fresh after that.
func (s *Server) freshdeskTemplated(w http.ResponseWriter, r *http.Request, name, key string) {
	b, ok := s.file(name)
	if !ok {
		freshdeskJSON(w, http.StatusNotFound, map[string]string{"code": "resource_not_found", "message": "The requested resource is not found."})
		return
	}
	s.mu.Lock()
	if s.fresh.served == nil {
		s.fresh.served = map[string]int{}
	}
	n := s.fresh.served[name+key]
	s.fresh.served[name+key]++
	s.mu.Unlock()
	sig := "fresh"
	if n == 0 {
		sig = "stale"
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(strings.ReplaceAll(string(b), "{{SIG}}", sig)))
}

// freshdeskPage answers the page of a fixture's array, filtered by keep when it is set. A page's
// attachment signatures are stale the first time that page is served.
func (s *Server) freshdeskPage(w http.ResponseWriter, r *http.Request, name string, page, perPage int, keep func(map[string]any) bool) {
	b, ok := s.file(name)
	if !ok {
		freshdeskJSON(w, http.StatusNotFound, map[string]string{"code": "resource_not_found", "message": "The requested resource is not found."})
		return
	}
	s.mu.Lock()
	if s.fresh.served == nil {
		s.fresh.served = map[string]int{}
	}
	key := name + "#" + strconv.Itoa(page)
	n := s.fresh.served[key]
	s.fresh.served[key]++
	s.mu.Unlock()
	sig := "fresh"
	if n == 0 {
		sig = "stale"
	}
	var all []map[string]any
	if err := json.Unmarshal([]byte(strings.ReplaceAll(string(b), "{{SIG}}", sig)), &all); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := []map[string]any{}
	for _, item := range all {
		if keep == nil || keep(item) {
			out = append(out, item)
		}
	}
	from := min((page-1)*perPage, len(out))
	freshdeskJSON(w, http.StatusOK, out[from:min(from+perPage, len(out))])
}
