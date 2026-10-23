package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/productdevbook/yuva/api/internal/oas"
)

type apiError struct {
	Status int
	Code   string
	Detail string
	Scope  string
	Field  string
	Limit  oas.HostedLimit
	// RetryAfter, in seconds, is sent as the Retry-After header when set.
	RetryAfter int
	// Extra members of the problem document, such as a design's validation errors.
	Extra map[string]any
}

func (e *apiError) Error() string { return fmt.Sprintf("%d %s: %s", e.Status, e.Code, e.Detail) }

func problem(status int, code, detail string) *apiError {
	return &apiError{Status: status, Code: code, Detail: detail}
}

var (
	errNotFound   = problem(http.StatusNotFound, "not_found", "no such resource")
	errInternal   = problem(http.StatusInternalServerError, "internal", "unexpected server error")
	errNotReady   = problem(http.StatusServiceUnavailable, "not_ready", "the database is not reachable")
	errValidation = func(detail string) *apiError { return problem(http.StatusBadRequest, "validation_failed", detail) }
)

func (e *apiError) body() oas.Problem {
	p := oas.Problem{
		Type:   "about:blank",
		Title:  http.StatusText(e.Status),
		Status: int32(e.Status),
		Code:   e.Code,
	}
	if e.Detail != "" {
		p.Detail = &e.Detail
	}
	if e.Scope != "" {
		p.Scope = &e.Scope
	}
	if e.Field != "" {
		p.Field = &e.Field
	}
	if e.Limit != "" {
		p.Limit = &e.Limit
	}
	return p
}

func writeProblem(w http.ResponseWriter, e *apiError) {
	w.Header().Set("Content-Type", "application/problem+json")
	if e.RetryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(e.RetryAfter))
	}
	w.WriteHeader(e.Status)
	if len(e.Extra) == 0 {
		_ = json.NewEncoder(w).Encode(e.body())
		return
	}
	doc := map[string]any{}
	b, _ := json.Marshal(e.body())
	_ = json.Unmarshal(b, &doc)
	for k, v := range e.Extra {
		doc[k] = v
	}
	_ = json.NewEncoder(w).Encode(doc)
}
