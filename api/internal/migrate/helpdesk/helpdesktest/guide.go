package helpdesktest

import (
	"net/http"
	"net/url"
	"strings"
)

// zendeskGuide answers the Help Center API of the acme account and the article attachments it
// serves. The article listing is cursor paged: the first page has no page[after], the second has
// the cursor "a1". An attachment named missing is a 404.
func (s *Server) zendeskGuide(w http.ResponseWriter, r *http.Request, p string, q url.Values) {
	if strings.HasPrefix(p, "/hc/article_attachments/") {
		if strings.Contains(p, "missing") {
			http.NotFound(w, r)
			return
		}
		s.attachment(w, r.URL.Path)
		return
	}
	rest := strings.TrimPrefix(p, "/api/v2/help_center/")
	if rest == "locales" {
		s.json(w, r, "zendesk/help_locales.json")
		return
	}
	locale, kind, ok := strings.Cut(rest, "/")
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch kind {
	case "categories", "sections":
		s.json(w, r, "zendesk/help_"+kind+"_"+locale+".json")
	case "articles":
		page := "1"
		if q.Get("page[after]") != "" {
			page = "2"
		}
		s.json(w, r, "zendesk/help_articles_"+locale+"_"+page+".json")
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) helpscoutDocs(w http.ResponseWriter, r *http.Request) {
	user, pass, ok := r.BasicAuth()
	if !ok || user != DocsKey || pass != "X" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	page := r.URL.Query().Get("page")
	switch {
	case r.URL.Path == "/v1/sites":
		s.json(w, r, "helpscout_docs/sites.json")
	case r.URL.Path == "/v1/collections":
		s.json(w, r, "helpscout_docs/collections.json")
	case len(parts) == 4 && parts[1] == "collections" && parts[3] == "categories":
		s.json(w, r, "helpscout_docs/categories_"+parts[2]+".json")
	case len(parts) == 4 && parts[1] == "collections" && parts[3] == "articles":
		s.json(w, r, "helpscout_docs/articles_"+parts[2]+"_"+page+".json")
	case len(parts) == 3 && parts[1] == "articles":
		s.json(w, r, "helpscout_docs/article_"+parts[2]+".json")
	default:
		http.NotFound(w, r)
	}
}
