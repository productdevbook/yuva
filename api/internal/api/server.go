package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/productdevbook/yuva/api/internal/metrics"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
	"github.com/productdevbook/yuva/api/internal/ui"
)

const maxBodyBytes = 8 << 20

var apiPrefixes = []string{"/v1/", "/client/v1/", "/ingress/"}

var apiPaths = []string{"/v1", "/client/v1", "/healthz", "/readyz"}

type Server struct {
	log     *slog.Logger
	st      *store.Store
	version string
}

type Deps struct {
	Log     *slog.Logger
	Store   *store.Store
	Version string
}

func New(d Deps) *Server {
	return &Server{log: d.Log, st: d.Store, version: d.Version}
}

var _ oas.StrictServerInterface = (*Server)(nil)

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	strict := oas.NewStrictHandlerWithOptions(s, nil, oas.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  s.writeRequestError,
		ResponseErrorHandlerFunc: s.writeError,
	})
	oas.HandlerWithOptions(strict, oas.StdHTTPServerOptions{
		BaseRouter: mux,
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			writeProblem(w, errValidation(err.Error()))
		},
	})
	panel := ui.Handler()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if isAPIPath(r.URL.Path) {
			writeProblem(w, errNotFound)
			return
		}
		panel.ServeHTTP(w, r)
	})
	return s.recoverer(s.logRequests(limitBody(mux)))
}

func isAPIPath(p string) bool {
	for _, prefix := range apiPrefixes {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	for _, exact := range apiPaths {
		if p == exact {
			return true
		}
	}
	return false
}

func (s *Server) writeRequestError(w http.ResponseWriter, r *http.Request, err error) {
	writeProblem(w, errValidation(err.Error()))
}

func (s *Server) writeError(w http.ResponseWriter, r *http.Request, err error) {
	var e *apiError
	if errors.As(err, &e) {
		writeProblem(w, e)
		return
	}
	s.log.ErrorContext(r.Context(), "handler failed", slog.String("path", r.URL.Path), slog.Any("error", err))
	writeProblem(w, errInternal)
}

func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		took := time.Since(start)
		metrics.Requests.WithLabelValues(r.Method, strconv.Itoa(rec.status)).Inc()
		metrics.RequestSeconds.WithLabelValues(r.Method).Observe(took.Seconds())
		s.log.Info("request", slog.String("method", r.Method), slog.String("path", r.URL.Path),
			slog.Int("status", rec.status), slog.Duration("duration", took))
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				s.log.ErrorContext(r.Context(), "panic", slog.Any("value", v), slog.String("path", r.URL.Path))
				writeProblem(w, errInternal)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
