package helpdesk

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Class is the limit a request counts against: Zendesk's incremental exports have their own.
type Class int

const (
	ClassAPI Class = iota
	ClassIncremental
)

const (
	minute = time.Minute
	// Day is the window of a help desk's daily limit.
	Day = 24 * time.Hour
)

// Budget is a migration's use of the help desk's rate limits, kept on the migration row so a
// restart does not reset it. Requests come from a bucket that holds a tenth of the window's
// budget and refills with the rest over the window (a minute, or the ten seconds some help desks
// count), so no window ever sees more than half of the limit the help desk last reported (Default
// before it reports one); incremental requests are also at most IncrementalMax in any minute.
// A help desk with a daily limit also has a day's budget next to it: at most half of the daily
// limit it last reported (DayDefault before), counted from the day's first request.
type Budget struct {
	Limit       int         `json:"limit,omitempty"`
	Tokens      float64     `json:"tokens"`
	At          time.Time   `json:"at,omitzero"`
	Incremental []time.Time `json:"incremental,omitempty"`
	DayLimit    int         `json:"day_limit,omitempty"`
	DayStart    time.Time   `json:"day_start,omitzero"`
	DayUsed     int         `json:"day_used,omitempty"`

	Default        int           `json:"-"`
	Window         time.Duration `json:"-"`
	IncrementalMax int           `json:"-"`
	// IncrementalShare, when set, makes the cap on incremental requests that share of the reported
	// limit, IncrementalMax until the help desk reports one.
	IncrementalShare float64  `json:"-"`
	LimitHeaders     []string `json:"-"`
	DayDefault       int      `json:"-"`
	DayHeaders       []string `json:"-"`
}

func (b *Budget) window() time.Duration {
	if b.Window <= 0 {
		return minute
	}
	return b.Window
}

// PerWindow is the most requests any window may see.
func (b *Budget) PerWindow() int {
	limit := b.Limit
	if limit <= 0 {
		limit = b.Default * 2
	}
	return max(limit/2, 2)
}

// PerDay is the most requests a day may see; 0 when the help desk has no daily limit.
func (b *Budget) PerDay() int {
	if b.DayLimit > 0 {
		return max(b.DayLimit/2, 1)
	}
	return b.DayDefault
}

func (b *Budget) capacity() float64 { return max(1, float64(b.PerWindow())/10) }

func (b *Budget) perSecond() float64 {
	return (float64(b.PerWindow()) - b.capacity()) / b.window().Seconds()
}

// Take reserves a request at now, or tells when one may be sent.
func (b *Budget) Take(class Class, now time.Time) error {
	perDay := b.PerDay()
	if perDay > 0 {
		if b.DayStart.IsZero() || !now.Before(b.DayStart.Add(Day)) {
			b.DayStart, b.DayUsed = now, 0
		}
		if b.DayUsed >= perDay {
			return &WaitError{Until: b.DayStart.Add(Day), Daily: true}
		}
	}
	if b.At.IsZero() {
		b.Tokens, b.At = b.capacity(), now
	}
	if now.After(b.At) {
		b.Tokens += now.Sub(b.At).Seconds() * b.perSecond()
		b.At = now
	}
	b.Tokens = min(b.Tokens, b.capacity())
	if class == ClassIncremental && b.incrementalMax() > 0 {
		kept := b.Incremental[:0]
		for _, t := range b.Incremental {
			if now.Before(t.Add(minute)) {
				kept = append(kept, t)
			}
		}
		b.Incremental = kept
		if len(kept) >= b.incrementalMax() {
			return &WaitError{Until: kept[0].Add(minute), Incremental: true}
		}
	}
	if b.Tokens < 1 {
		wait := time.Duration((1 - b.Tokens) / b.perSecond() * float64(time.Second))
		return &WaitError{Until: now.Add(max(wait, time.Millisecond))}
	}
	b.Tokens--
	if class == ClassIncremental && b.incrementalMax() > 0 {
		b.Incremental = append(b.Incremental, now)
	}
	if perDay > 0 {
		b.DayUsed++
	}
	return nil
}

// incrementalMax is the most incremental requests a minute may see.
func (b *Budget) incrementalMax() int {
	if b.IncrementalShare > 0 && b.Limit > 0 {
		return max(1, int(float64(b.Limit)*b.IncrementalShare))
	}
	return b.IncrementalMax
}

// Saw learns the window's and the day's limits from an answer's headers.
func (b *Budget) Saw(h http.Header) {
	if v, ok := headerInt(h, b.LimitHeaders); ok {
		b.Limit = v
	}
	if v, ok := headerInt(h, b.DayHeaders); ok {
		b.DayLimit = v
	}
}

func headerInt(h http.Header, names []string) (int, bool) {
	for _, name := range names {
		if v, err := strconv.Atoi(strings.TrimSpace(h.Get(name))); err == nil && v > 0 {
			return v, true
		}
	}
	return 0, false
}
