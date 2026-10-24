package api

import (
	"context"
	"strings"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	maxSearchOperators = 20
	minAddressValue    = 3
)

var addressMatchCap atomic.Int64

func init() { addressMatchCap.Store(5000) }

var errTooManyOperators = errValidation("q takes at most 20 operators")

var searchFolders = []string{"inbox", "starred", "snoozed", "sent", "archive", "trash", "drafts", "scheduled"}

type searchOperator struct {
	Key     string
	Value   string
	Negated bool
}

func (o searchOperator) me() bool {
	return (o.Key == "from" || o.Key == "to") && strings.EqualFold(o.Value, "me")
}

func (o searchOperator) folder() string {
	if o.Key == "in" {
		return strings.ToLower(o.Value)
	}
	return ""
}

func validOperator(key, value string, loc *time.Location) bool {
	switch key {
	case "from", "to":
		if strings.EqualFold(value, "me") {
			return true
		}
		return validAddressValue(value)
	case "cc":
		return validAddressValue(value)
	case "subject":
		return value != ""
	case "has":
		return strings.EqualFold(value, "attachment")
	case "is":
		return strings.EqualFold(value, "starred")
	case "in":
		for _, f := range searchFolders {
			if strings.EqualFold(value, f) {
				return true
			}
		}
		return false
	case "before", "after":
		_, ok := localMidnight(value, loc)
		return ok
	}
	return false
}

// to: and cc: search the addresses joined by spaces, so a value with a space or a comma could only match across two.
func validAddressValue(v string) bool {
	return utf8.RuneCountInString(v) >= minAddressValue && !strings.ContainsAny(v, " ,\t\n")
}

func localMidnight(date string, loc *time.Location) (time.Time, bool) {
	d, err := time.Parse(time.DateOnly, date)
	if err != nil || d.Year() < 1 {
		return time.Time{}, false
	}
	at := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, loc).UTC()
	return at, at.Year() >= 1 && at.Year() <= 9999
}

func parseSearchOperators(q string, loc *time.Location) (string, []searchOperator, error) {
	rs := []rune(q)
	text := slicesClone(rs)
	var ops []searchOperator
	for i := 0; i < len(rs); {
		if unicode.IsSpace(rs[i]) {
			i++
			continue
		}
		start := i
		negated := rs[i] == '-'
		j := i
		if negated {
			j++
		}
		if j < len(rs) && rs[j] == '"' {
			j++
			for j < len(rs) && rs[j] != '"' {
				j++
			}
			i = min(j+1, len(rs))
			continue
		}
		k := j
		for k < len(rs) && (rs[k] >= 'a' && rs[k] <= 'z' || rs[k] >= 'A' && rs[k] <= 'Z') {
			k++
		}
		if k == j || k >= len(rs) || rs[k] != ':' {
			for i < len(rs) && !unicode.IsSpace(rs[i]) && rs[i] != '"' {
				i++
			}
			if i == start {
				i++
			}
			continue
		}
		key := strings.ToLower(string(rs[j:k]))
		k++
		var value string
		end := k
		if k < len(rs) && rs[k] == '"' {
			c := k + 1
			for c < len(rs) && rs[c] != '"' {
				c++
			}
			if c < len(rs) {
				value = strings.TrimSpace(string(rs[k+1 : c]))
				end = c + 1
			}
		} else {
			for end < len(rs) && !unicode.IsSpace(rs[end]) {
				end++
			}
			value = string(rs[k:end])
		}
		i = max(end, k)
		if !validOperator(key, value, loc) {
			for i < len(rs) && !unicode.IsSpace(rs[i]) {
				i++
			}
			continue
		}
		ops = append(ops, searchOperator{Key: key, Value: value, Negated: negated})
		for x := start; x < i; x++ {
			text[x] = ' '
		}
	}
	if len(ops) > maxSearchOperators {
		return "", nil, errTooManyOperators
	}
	return strings.TrimSpace(string(text)), ops, nil
}

func slicesClone(rs []rune) []rune { return append([]rune(nil), rs...) }

func likeContains(v string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(v) + "%"
}

func searchLocation(timezone *string) (*time.Location, error) {
	if timezone == nil || strings.TrimSpace(*timezone) == "" {
		return time.UTC, nil
	}
	loc, err := time.LoadLocation(strings.TrimSpace(*timezone))
	if err != nil {
		e := errValidation("timezone must be an IANA time zone name")
		e.Field = "timezone"
		return nil, e
	}
	return loc, nil
}

func applySearchOperators(arg *store.ListConversationsParams, p principal, ops []searchOperator, loc *time.Location) ([]searchOperator, error) {
	var addressOps []searchOperator
	flag := func(dst **bool, o searchOperator) {
		want := !o.Negated
		if *dst != nil && **dst != want {
			arg.OpImpossible = true
		}
		*dst = &want
	}
	for _, o := range ops {
		switch f := o.folder(); {
		case o.me(), o.Key == "is", f == "starred", f == "sent", f == "drafts", f == "scheduled":
			if p.memberRef() == nil {
				return nil, errValidation("the operator " + o.Key + ":" + o.Value + " needs a member session")
			}
		case f == "trash" && !o.Negated:
			arg.Trashed = true
		}
		switch o.Key {
		case "from", "to":
			switch {
			case o.me() && o.Key == "from":
				flag(&arg.OpFromMe, o)
			case o.me():
				flag(&arg.OpToMe, o)
			default:
				addressOps = append(addressOps, o)
			}
		case "cc":
			addressOps = append(addressOps, o)
		case "subject":
			arg.OpSubjectVals, arg.OpSubjectNeg = append(arg.OpSubjectVals, o.Value), append(arg.OpSubjectNeg, o.Negated)
		case "has":
			flag(&arg.OpHasAttachment, o)
		case "is":
			flag(&arg.OpIsStarred, o)
		case "in":
			switch o.folder() {
			case "inbox":
				flag(&arg.OpInInbox, o)
			case "starred":
				flag(&arg.OpInStarred, o)
			case "snoozed":
				flag(&arg.OpInSnoozed, o)
			case "archive":
				flag(&arg.OpInArchive, o)
			case "trash":
				flag(&arg.OpInTrash, o)
			case "sent":
				flag(&arg.OpInSent, o)
			case "drafts":
				flag(&arg.OpInDrafts, o)
			case "scheduled":
				flag(&arg.OpInScheduled, o)
			}
		case "before", "after":
			at, _ := localMidnight(o.Value, loc)
			bound(arg, o, at)
		}
	}
	return addressOps, nil
}

// bound folds the date operators into four instants: the positive ones must hold for one message
// together, the negated ones each exclude conversations on their own.
func bound(arg *store.ListConversationsParams, o searchOperator, at time.Time) {
	pick := func(dst **time.Time, later bool) {
		if *dst == nil || at.After(**dst) == later {
			*dst = &at
		}
	}
	switch {
	case o.Key == "after" && !o.Negated:
		pick(&arg.OpAfter, true)
	case o.Key == "before" && !o.Negated:
		pick(&arg.OpBefore, false)
	case o.Key == "after":
		pick(&arg.OpNotAfter, false)
	default:
		pick(&arg.OpNotBefore, true)
	}
}

func resolveAddresses(ctx context.Context, q *store.Queries, arg *store.ListConversationsParams, ops []searchOperator) error {
	limit := int32(addressMatchCap.Load())
	hits, misses := map[uuid.UUID]int{}, map[uuid.UUID]struct{}{}
	positive := 0
	for _, o := range ops {
		ids, err := addressMatches(ctx, q, arg.WorkspaceID, o, limit+1)
		if err != nil {
			return err
		}
		if len(ids) > int(limit) || o.Negated && len(misses)+len(ids) > int(limit) {
			inlineAddress(arg, o)
			continue
		}
		seen := map[uuid.UUID]struct{}{}
		for _, id := range ids {
			if _, dup := seen[id]; dup {
				continue
			}
			seen[id] = struct{}{}
			if o.Negated {
				misses[id] = struct{}{}
			} else {
				hits[id]++
			}
		}
		if !o.Negated {
			positive++
		}
	}
	arg.OpHitIds, arg.OpMissIds = []uuid.UUID{}, make([]uuid.UUID, 0, len(misses))
	arg.OpHasHits = positive > 0
	for id, n := range hits {
		if n == positive {
			arg.OpHitIds = append(arg.OpHitIds, id)
		}
	}
	for id := range misses {
		arg.OpMissIds = append(arg.OpMissIds, id)
	}
	return nil
}

func inlineAddress(arg *store.ListConversationsParams, o searchOperator) {
	v := likeContains(o.Value)
	switch o.Key {
	case "from":
		arg.OpFromVals, arg.OpFromNeg = append(arg.OpFromVals, v), append(arg.OpFromNeg, o.Negated)
	case "to":
		arg.OpToVals, arg.OpToNeg = append(arg.OpToVals, v), append(arg.OpToNeg, o.Negated)
	default:
		arg.OpCcVals, arg.OpCcNeg = append(arg.OpCcVals, v), append(arg.OpCcNeg, o.Negated)
	}
}

func addressMatches(ctx context.Context, q *store.Queries, workspace uuid.UUID, o searchOperator, limit int32) ([]uuid.UUID, error) {
	pattern := likeContains(o.Value)
	switch o.Key {
	case "from":
		return q.ListFromMatchConversations(ctx, store.ListFromMatchConversationsParams{WorkspaceID: workspace, Pattern: pattern, MaxRows: limit})
	case "to":
		return q.ListToMatchConversations(ctx, store.ListToMatchConversationsParams{WorkspaceID: workspace, Pattern: pattern, MaxRows: limit})
	default:
		return q.ListCcMatchConversations(ctx, store.ListCcMatchConversationsParams{WorkspaceID: workspace, Pattern: pattern, MaxRows: limit})
	}
}
