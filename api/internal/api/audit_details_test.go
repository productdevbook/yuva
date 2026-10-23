package api

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestAuditDetailsStayBounded(t *testing.T) {
	ids := make([]string, 300)
	for i := range ids {
		ids[i] = fmt.Sprintf("01a16881-ca6d-7dae-8737-%012d", i)
	}
	var got map[string]any
	if err := json.Unmarshal(boundedDetails(auditDetails{"name": "Ops", "events": ids}), &got); err != nil {
		t.Fatal(err)
	}
	if got["truncated"] != true || got["events_count"] != float64(300) || got["name"] != "Ops" || got["events"] != nil {
		t.Fatalf("bounded %v", got)
	}
	long := make([]byte, 3000)
	for i := range long {
		long[i] = 'x'
	}
	if b := string(boundedDetails(auditDetails{"name": string(long)})); b != `{"truncated": true}` {
		t.Fatalf("a long value kept: %d bytes", len(b))
	}
	if b := string(boundedDetails(auditDetails{"name": "Ops"})); b != `{"name":"Ops"}` {
		t.Fatalf("small details changed: %s", b)
	}
}
