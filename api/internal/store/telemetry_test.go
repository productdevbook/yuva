package store

import (
	"context"
	"os"
	"strings"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/productdevbook/yuva/api/internal/telemetry"
)

func TestSpanName(t *testing.T) {
	for stmt, want := range map[string]string{
		"-- name: GetContact :one\nSELECT 1": "GetContact",
		"select 1":                           "SELECT",
		"-- plain comment\nselect 1":         "QUERY",
		"":                                   "QUERY",
	} {
		if got := spanName(stmt); got != want {
			t.Errorf("spanName(%q) = %q, want %q", stmt, got, want)
		}
	}
}

func TestQuerySpansCarryNoArguments(t *testing.T) {
	url := os.Getenv("YUVA_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("YUVA_TEST_DATABASE_URL is not set")
	}
	mem := tracetest.NewInMemoryExporter()
	telemetry.Install(sdktrace.NewTracerProvider(sdktrace.WithSyncer(telemetry.Filter(mem))))
	t.Cleanup(telemetry.Uninstall)
	ctx, parent := telemetry.Tracer().Start(context.Background(), "request")
	st, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var out string
	if err := st.Pool.QueryRow(ctx, "SELECT $1::text, 'literal-secret'", "jane@example.com").Scan(&out, new(string)); err != nil {
		t.Fatal(err)
	}
	parent.End()
	var all strings.Builder
	n := 0
	for _, s := range mem.GetSpans() {
		all.WriteString(s.Name + "|" + s.Status.Description)
		for _, kv := range s.Attributes {
			all.WriteString("|" + string(kv.Key) + "=" + kv.Value.Emit())
		}
		if s.Name == "SELECT" {
			n++
		}
	}
	if n == 0 {
		t.Fatalf("no query span: %s", all.String())
	}
	for _, bad := range []string{"jane", "literal-secret", "example.com"} {
		if strings.Contains(all.String(), bad) {
			t.Errorf("spans contain %q: %s", bad, all.String())
		}
	}
}
