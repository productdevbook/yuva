package api

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func TestParseSearchOperators(t *testing.T) {
	t.Parallel()
	op := func(key, value string, negated bool) searchOperator {
		return searchOperator{Key: key, Value: value, Negated: negated}
	}
	for _, tc := range []struct {
		q    string
		text string
		ops  []searchOperator
	}{
		{"invoice", "invoice", nil},
		{"from:ada@example.com", "", []searchOperator{op("from", "ada@example.com", false)}},
		{`FROM:"Ada" invoice`, "invoice", []searchOperator{op("from", "Ada", false)}},
		{`from:"Ada Lovelace" to:a,b to:"x y@z.com" cc:ab from:ab`, `from:"Ada Lovelace" to:a,b to:"x y@z.com" cc:ab from:ab`, nil},
		{"after:0000-01-01 before:0000-12-31", "after:0000-01-01 before:0000-12-31", nil},
		{"after:0001-01-01", "", []searchOperator{op("after", "0001-01-01", false)}},
		{"-to:me paid", "paid", []searchOperator{op("to", "me", true)}},
		{"has:Attachment is:starred", "", []searchOperator{op("has", "Attachment", false), op("is", "starred", false)}},
		{"in:Trash after:2026-01-02 before:2026-02-03", "", []searchOperator{op("in", "Trash", false), op("after", "2026-01-02", false), op("before", "2026-02-03", false)}},
		{"ratio:3:1 https://example.com/a", "ratio:3:1 https://example.com/a", nil},
		{"before:yesterday-ish in:nowhere has:video is:read from:", "before:yesterday-ish in:nowhere has:video is:read from:", nil},
		{"after:2026-13-01 before:2026-02-30", "after:2026-13-01 before:2026-02-30", nil},
		{`"from:ada" -"to:bob"`, `"from:ada" -"to:bob"`, nil},
		{`subject:"unterminated`, `subject:"unterminated`, nil},
		{`refund -subject:late cc:x@y.z`, "refund", []searchOperator{op("subject", "late", true), op("cc", "x@y.z", false)}},
		{"-refund -from:me", "-refund", []searchOperator{op("from", "me", true)}},
		{"a from:bob c", "a c", []searchOperator{op("from", "bob", false)}},
	} {
		text, ops, err := parseSearchOperators(tc.q, time.UTC)
		if err != nil {
			t.Fatalf("%q: %v", tc.q, err)
		}
		if strings.Join(strings.Fields(text), " ") != strings.Join(strings.Fields(tc.text), " ") || !slices.Equal(ops, tc.ops) {
			t.Fatalf("%q: got %q %v, want %q %v", tc.q, text, ops, tc.text, tc.ops)
		}
	}
}

func TestParseSearchOperatorsLimit(t *testing.T) {
	t.Parallel()
	if _, ops, err := parseSearchOperators(strings.Repeat("is:starred ", maxSearchOperators), time.UTC); err != nil || len(ops) != maxSearchOperators {
		t.Fatalf("%d operators: %v", len(ops), err)
	}
	if _, _, err := parseSearchOperators(strings.Repeat("is:starred ", maxSearchOperators+1), time.UTC); err == nil {
		t.Fatal("21 operators accepted")
	}
}

func TestLikeContainsEscapes(t *testing.T) {
	t.Parallel()
	if got := likeContains(`50%_a\b`); got != `%50\%\_a\\b%` {
		t.Fatalf("got %q", got)
	}
}

func TestParseSearchOperatorsDatesStayInRangeInTheTimeZone(t *testing.T) {
	t.Parallel()
	kiritimati := time.FixedZone("UTC+14", 14*3600)
	if _, ops, _ := parseSearchOperators("after:0001-01-01", kiritimati); len(ops) != 0 {
		t.Fatalf("a date that is year 0 in UTC was accepted: %v", ops)
	}
	if _, ops, _ := parseSearchOperators("before:9999-12-31", kiritimati); len(ops) != 1 {
		t.Fatalf("a valid date was refused: %v", ops)
	}
}
