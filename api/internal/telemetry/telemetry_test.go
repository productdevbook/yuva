package telemetry_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/productdevbook/yuva/api/internal/telemetry"
)

func install(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	mem := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(telemetry.Filter(mem)))
	telemetry.Install(tp)
	t.Cleanup(telemetry.Uninstall)
	return mem
}

func dump(spans tracetest.SpanStubs) string {
	var b strings.Builder
	for _, s := range spans {
		fmt.Fprintf(&b, "%s|%s|", s.Name, s.Status.Description)
		for _, kv := range s.Attributes {
			fmt.Fprintf(&b, "%s=%s|", kv.Key, kv.Value.Emit())
		}
		for _, e := range s.Events {
			fmt.Fprintf(&b, "%s|", e.Name)
			for _, kv := range e.Attributes {
				fmt.Fprintf(&b, "%s=%s|", kv.Key, kv.Value.Emit())
			}
		}
		for _, l := range s.Links {
			fmt.Fprintf(&b, "%v|", l.Attributes)
		}
	}
	return b.String()
}

func TestFilterKeepsOnlyAllowedAttributes(t *testing.T) {
	mem := install(t)
	ctx, parent := telemetry.Tracer().Start(context.Background(), "parent")
	_, span := telemetry.Tracer().Start(ctx, "child", trace.WithAttributes(
		telemetry.WorkspaceKey.String("ws-1"),
		attribute.String("db.query.text", "SELECT * FROM contacts WHERE email = 'jane@example.com'"),
		attribute.String("http.request.header.authorization", "Bearer secret-token"),
		attribute.String("url.full", "https://hooks.example.com/secret"),
		attribute.String("email", "jane@example.com"),
		attribute.String("http.route", "GET /v1/conversations/{id}"),
	))
	span.RecordError(fmt.Errorf("duplicate key value (jane@example.com)"))
	span.AddEvent("note", trace.WithAttributes(attribute.String("body", "hello jane@example.com")))
	span.SetStatus(codes.Error, "550 jane@example.com not found")
	span.End()
	parent.End()

	spans := mem.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("got %d spans", len(spans))
	}
	out := dump(spans)
	for _, bad := range []string{"jane", "secret", "example.com", "contacts", "Bearer"} {
		if strings.Contains(out, bad) {
			t.Errorf("exported span contains %q: %s", bad, out)
		}
	}
	if !strings.Contains(out, "yuva.workspace_id=ws-1") || !strings.Contains(out, "http.route=GET /v1/conversations/{id}") {
		t.Errorf("allowed attributes missing: %s", out)
	}
}

func TestFilterDropsRootDatabaseSpans(t *testing.T) {
	mem := install(t)
	opts := trace.WithAttributes(attribute.String("db.system.name", "postgresql"))
	_, orphan := telemetry.Tracer().Start(context.Background(), "poll", trace.WithSpanKind(trace.SpanKindClient), opts)
	orphan.End()
	ctx, parent := telemetry.Tracer().Start(context.Background(), "request")
	_, child := telemetry.Tracer().Start(ctx, "query", trace.WithSpanKind(trace.SpanKindClient), opts)
	child.End()
	parent.End()
	var names []string
	for _, s := range mem.GetSpans() {
		names = append(names, s.Name)
	}
	if strings.Join(names, ",") != "query,request" {
		t.Fatalf("spans = %v", names)
	}
}

func TestMiddlewareRecordsRouteNotPath(t *testing.T) {
	mem := install(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /unsubscribe/{token}", func(w http.ResponseWriter, r *http.Request) {
		telemetry.Workspace(r.Context(), "ws-9")
		w.WriteHeader(http.StatusTeapot)
	})
	h := telemetry.Middleware(telemetry.Route(mux))
	req := httptest.NewRequest("GET", "/unsubscribe/tok-123?email=jane@example.com", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	req.Header.Set("User-Agent", "agent-x")
	req.RemoteAddr = "203.0.113.9:4444"
	h.ServeHTTP(httptest.NewRecorder(), req)

	spans := mem.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("got %d spans", len(spans))
	}
	out := dump(spans)
	for _, bad := range []string{"tok-123", "jane", "secret", "agent-x", "203.0.113.9"} {
		if strings.Contains(out, bad) {
			t.Errorf("span contains %q: %s", bad, out)
		}
	}
	for _, want := range []string{"GET /unsubscribe/{token}", "http.response.status_code=418", "yuva.workspace_id=ws-9"} {
		if !strings.Contains(out, want) {
			t.Errorf("span lacks %q: %s", want, out)
		}
	}
}

func TestMiddlewareContinuesRemoteTrace(t *testing.T) {
	mem := install(t)
	h := telemetry.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	h.ServeHTTP(httptest.NewRecorder(), req)
	spans := mem.GetSpans()
	if len(spans) != 1 || spans[0].SpanContext.TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("trace not continued: %+v", spans)
	}
}

func TestLogLinesCarryTraceID(t *testing.T) {
	install(t)
	var buf strings.Builder
	log := slog.New(telemetry.Handler{Handler: slog.NewJSONHandler(&buf, nil)})
	ctx, span := telemetry.Start(context.Background(), "x")
	log.InfoContext(ctx, "inside")
	span.End()
	log.InfoContext(context.Background(), "outside")
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if !strings.Contains(lines[0], `"trace_id":"`+span.SpanContext().TraceID().String()+`"`) {
		t.Errorf("no trace_id: %s", lines[0])
	}
	if strings.Contains(lines[1], "trace_id") {
		t.Errorf("trace_id without a span: %s", lines[1])
	}
}

func TestOffByDefault(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	stop, err := telemetry.Setup(context.Background(), "test")
	if err != nil || telemetry.Enabled() {
		t.Fatalf("enabled=%v err=%v", telemetry.Enabled(), err)
	}
	_ = stop(context.Background())
	called := false
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })
	telemetry.Middleware(next).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	ctx, span := telemetry.Start(context.Background(), "x")
	telemetry.End(span, nil)
	if !called || span.SpanContext().IsValid() || trace.SpanContextFromContext(ctx).IsValid() {
		t.Fatal("tracing active while off")
	}
}

func TestSetupOnWithEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:1")
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "http/protobuf")
	stop, err := telemetry.Setup(context.Background(), "test")
	if err != nil || !telemetry.Enabled() {
		t.Fatalf("enabled=%v err=%v", telemetry.Enabled(), err)
	}
	t.Cleanup(telemetry.Uninstall)
	_ = stop
}
