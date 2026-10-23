// Package telemetry exports OpenTelemetry traces over OTLP. It is off, with no exporter and
// no provider, unless OTEL_EXPORTER_OTLP_ENDPOINT or OTEL_EXPORTER_OTLP_TRACES_ENDPOINT is set.
// Every span passes an allow-list before it leaves the process, so no instrumentation can export
// a message body, address, header value, token or query argument.
package telemetry

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync/atomic"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.opentelemetry.io/otel/trace"
)

const scope = "github.com/productdevbook/yuva/api"

const (
	WorkspaceKey = attribute.Key("yuva.workspace_id")
	InboxKey     = attribute.Key("yuva.inbox_id")
)

var enabled atomic.Bool

func Enabled() bool { return enabled.Load() }

func Configured() bool {
	if strings.EqualFold(os.Getenv("OTEL_SDK_DISABLED"), "true") || strings.EqualFold(os.Getenv("OTEL_TRACES_EXPORTER"), "none") {
		return false
	}
	return os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" || os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") != ""
}

// Setup installs the global tracer provider when tracing is configured and returns the function
// that flushes and stops it. Otherwise it does nothing and the global provider stays a no-op.
func Setup(ctx context.Context, version string) (func(context.Context) error, error) {
	if !Configured() {
		return func(context.Context) error { return nil }, nil
	}
	var client otlptrace.Client
	proto := firstEnv("OTEL_EXPORTER_OTLP_TRACES_PROTOCOL", "OTEL_EXPORTER_OTLP_PROTOCOL")
	if strings.HasPrefix(proto, "http") {
		client = otlptracehttp.NewClient()
	} else {
		client = otlptracegrpc.NewClient()
	}
	exp, err := otlptrace.New(ctx, client)
	if err != nil {
		return nil, err
	}
	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName("yuva"), semconv.ServiceVersion(version)),
		resource.WithFromEnv(),
	)
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(Filter(exp)),
		sdktrace.WithResource(res),
	)
	Install(tp)
	return tp.Shutdown, nil
}

func firstEnv(names ...string) string {
	for _, n := range names {
		if v := os.Getenv(n); v != "" {
			return v
		}
	}
	return ""
}

// Install makes tp the global tracer provider and turns instrumentation on.
func Install(tp trace.TracerProvider) {
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	enabled.Store(true)
}

func Uninstall() {
	enabled.Store(false)
	otel.SetTracerProvider(trace.NewNoopTracerProvider())
}

func Tracer() trace.Tracer { return otel.Tracer(scope) }

// Start opens a span, or returns ctx unchanged and a no-op span when tracing is off.
func Start(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	if !Enabled() {
		return ctx, trace.SpanFromContext(ctx)
	}
	return Tracer().Start(ctx, name, trace.WithAttributes(attrs...))
}

func End(span trace.Span, err error) {
	if !Enabled() {
		return
	}
	if err != nil {
		span.SetStatus(codes.Error, "")
	}
	span.End()
}

func Workspace(ctx context.Context, id string) {
	if Enabled() {
		trace.SpanFromContext(ctx).SetAttributes(WorkspaceKey.String(id))
	}
}

func Inbox(ctx context.Context, workspaceID, inboxID string) {
	if Enabled() {
		trace.SpanFromContext(ctx).SetAttributes(WorkspaceKey.String(workspaceID), InboxKey.String(inboxID))
	}
}

// Handler adds trace_id and span_id to every record logged with a context that carries a span.
type Handler struct{ slog.Handler }

func (h Handler) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.AddAttrs(slog.String("trace_id", sc.TraceID().String()), slog.String("span_id", sc.SpanID().String()))
	}
	return h.Handler.Handle(ctx, r)
}

func (h Handler) WithAttrs(a []slog.Attr) slog.Handler { return Handler{h.Handler.WithAttrs(a)} }
func (h Handler) WithGroup(n string) slog.Handler      { return Handler{h.Handler.WithGroup(n)} }

// Middleware traces each request as one server span named after the matched route pattern. It
// records the method, the route pattern and the status; the path, query, headers and client
// address are never recorded because they carry tokens and addresses.
func Middleware(next http.Handler) http.Handler {
	if !Enabled() {
		return next
	}
	tracer := Tracer()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		ctx, span := tracer.Start(ctx, r.Method, trace.WithSpanKind(trace.SpanKindServer))
		rec := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			span.SetAttributes(
				attribute.String("http.request.method", r.Method),
				attribute.Int("http.response.status_code", rec.status),
			)
			if rec.status >= 500 {
				span.SetStatus(codes.Error, "")
			}
			span.End()
		}()
		next.ServeHTTP(rec, r.WithContext(ctx))
	})
}

// Route names the server span after the pattern the mux matched. Wrap the mux in it: only the
// mux knows the pattern, and middleware between may have copied the request.
func Route(mux *http.ServeMux) http.Handler {
	if !Enabled() {
		return mux
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r)
		route := r.Pattern
		if route == "" {
			route = "unmatched"
		}
		span := trace.SpanFromContext(r.Context())
		span.SetName(routeName(r.Method, route))
		span.SetAttributes(attribute.String("http.route", route))
	})
}

func routeName(method, route string) string {
	if strings.HasPrefix(route, method+" ") {
		return route
	}
	return method + " " + route
}

type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (s *statusWriter) WriteHeader(code int) {
	if !s.wrote {
		s.status, s.wrote = code, true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusWriter) Write(b []byte) (int, error) {
	s.wrote = true
	return s.ResponseWriter.Write(b)
}

func (s *statusWriter) Unwrap() http.ResponseWriter { return s.ResponseWriter }
