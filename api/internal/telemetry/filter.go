package telemetry

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

var allowed = map[attribute.Key]bool{
	WorkspaceKey:                true,
	InboxKey:                    true,
	"http.request.method":       true,
	"http.route":                true,
	"http.response.status_code": true,
	"db.system.name":            true,
	"messaging.system":          true,
	"messaging.operation.name":  true,
	"kind":                      true,
	"queue":                     true,
	"attempt":                   true,
	"priority":                  true,
	"status":                    true,
	"id":                        true,
	"snooze":                    true,
	"cancel":                    true,
	"yuva.recipients":           true,
	"yuva.message_bytes":        true,
	"exception.type":            true,
	"pgx.sql_state":             true,
	"db.response.status_code":   true,
	"db.operation.batch.size":   true,
	"pgx.operation.type":        true,
	"error.type":                true,
}

type filter struct{ sdktrace.SpanExporter }

// Filter returns an exporter that passes only allow-listed attributes, strips error messages and
// event attributes, and drops database spans that belong to no trace (the job queue's polling).
func Filter(next sdktrace.SpanExporter) sdktrace.SpanExporter { return filter{next} }

func (f filter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	out := make([]sdktrace.ReadOnlySpan, 0, len(spans))
	for _, s := range spans {
		if cleaned, ok := clean(s); ok {
			out = append(out, cleaned)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return f.SpanExporter.ExportSpans(ctx, out)
}

func clean(s sdktrace.ReadOnlySpan) (sdktrace.ReadOnlySpan, bool) {
	stub := tracetest.SpanStubFromReadOnlySpan(s)
	if !stub.Parent.IsValid() && stub.SpanKind == trace.SpanKindClient && hasKey(stub.Attributes, "db.system.name") {
		return nil, false
	}
	stub.Attributes = keep(stub.Attributes)
	events := stub.Events[:0:0]
	for _, e := range stub.Events {
		e.Attributes = keep(e.Attributes)
		if e.Name != "exception" {
			e.Name = "event"
		}
		events = append(events, e)
	}
	stub.Events = events
	links := stub.Links[:0:0]
	for _, l := range stub.Links {
		l.Attributes = nil
		links = append(links, l)
	}
	stub.Links = links
	stub.Status.Description = ""
	return stub.Snapshot(), true
}

func keep(in []attribute.KeyValue) []attribute.KeyValue {
	out := make([]attribute.KeyValue, 0, len(in))
	for _, kv := range in {
		if allowed[kv.Key] && !strings.ContainsAny(kv.Value.Emit(), "@") {
			out = append(out, kv)
		}
	}
	return out
}

func hasKey(in []attribute.KeyValue, k attribute.Key) bool {
	for _, kv := range in {
		if kv.Key == k {
			return true
		}
	}
	return false
}
