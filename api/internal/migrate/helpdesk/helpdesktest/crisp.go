package helpdesktest

import (
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	CrispIdentifier = "crisp-token-id"
	CrispKey        = "crisp-token-key"
	CrispWebsite    = "b2f4a9d0-3c1e-4e7a-9a55-0c6d2f1e8a01"
	CrispStorage    = "storage.crisp.chat"
)

func (s *Server) crisp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	user, key, ok := r.BasicAuth()
	if !ok || user != CrispIdentifier || key != CrispKey || r.Header.Get("X-Crisp-Tier") != "website" {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":true,"reason":"invalid_session","data":{}}`))
		return
	}
	s.mu.Lock()
	throttled := s.CrispThrottle > 0
	if throttled {
		s.CrispThrottle--
	}
	s.mu.Unlock()
	if throttled {
		w.WriteHeader(420)
		_, _ = w.Write([]byte(`{"error":true,"reason":"rate_limited","data":{}}`))
		return
	}
	rest, found := strings.CutPrefix(r.URL.Path, "/v1/website/"+CrispWebsite)
	if !found {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	q := r.URL.Query()
	switch {
	case rest == "":
		s.json(w, r, "crisp/website.json")
	case rest == "/operators/list":
		s.json(w, r, "crisp/operators.json")
	case len(parts) == 2 && parts[0] == "conversations":
		s.crispConversations(w, q, parts[1])
	case len(parts) == 3 && parts[0] == "conversation" && parts[2] == "messages":
		name := "crisp/messages_" + parts[1]
		if before := q.Get("timestamp_before"); before != "" {
			name += "_" + before
		}
		if _, ok := s.file(name + ".json"); !ok {
			crispEmpty(w)
			return
		}
		s.json(w, r, name+".json")
	default:
		http.NotFound(w, r)
	}
}

func crispEmpty(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"error":false,"reason":"listed","data":[]}`))
}

// crispConversations lists the conversations of both fixture pages as one list, newest update
// first, within filter_date_start and filter_date_end, at CrispPageSize (else per_page) a page.
func (s *Server) crispConversations(w http.ResponseWriter, q url.Values, page string) {
	var all []map[string]any
	for _, name := range []string{"crisp/conversations_1.json", "crisp/conversations_2.json"} {
		b, ok := s.file(name)
		if !ok {
			continue
		}
		var env struct {
			Data []map[string]any `json:"data"`
		}
		_ = json.Unmarshal(b, &env)
		all = append(all, env.Data...)
	}
	updated := func(c map[string]any) int64 { ms, _ := c["updated_at"].(float64); return int64(ms) }
	sort.SliceStable(all, func(i, j int) bool { return updated(all[i]) > updated(all[j]) })
	if s.CrispAscending {
		slices.Reverse(all)
	}
	var start, end int64
	if v := q.Get("filter_date_start"); v != "" {
		at, err := time.Parse(time.RFC3339, v)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		start = at.UnixMilli()
	}
	if v := q.Get("filter_date_end"); v != "" {
		at, err := time.Parse(time.RFC3339, v)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		end = at.UnixMilli()
	}
	kept := []map[string]any{}
	for _, c := range all {
		if (start == 0 || updated(c) > start) && (end == 0 || updated(c) <= end) {
			kept = append(kept, c)
		}
	}
	size, _ := strconv.Atoi(q.Get("per_page"))
	if s.CrispPageSize > 0 {
		size = s.CrispPageSize
	}
	if size <= 0 {
		size = 20
	}
	n, _ := strconv.Atoi(page)
	from := min((max(n, 1)-1)*size, len(kept))
	to := min(from+size, len(kept))
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"error": false, "reason": "listed", "data": kept[from:to]})
}

func (s *Server) crispStorage(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	throttled := s.CrispFilesThrottle > 0
	if throttled {
		s.CrispFilesThrottle--
	}
	s.mu.Unlock()
	if throttled {
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}
	s.attachment(w, r.URL.Path)
}
