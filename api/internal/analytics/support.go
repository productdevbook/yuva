package analytics

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
)

// DatasetSupport is a query of what a property's linked inbox received from its pages.
const DatasetSupport = "support"

// SupportMetrics are the support dataset's own metrics; it also offers visitors and pageviews.
var SupportMetrics = []string{"conversations", "feedback", "ratings_up", "ratings_down", "helpful_rate", "discussions", "support_rate"}

// SupportDims are the dimensions and filters the support dataset takes besides time grains.
var SupportDims = []string{"page", "hostname"}

// SupportRow is what an inbox received from one page (host and path) on one local day.
type SupportRow struct {
	Day  time.Time
	Host string
	Path string
	Sums Sums
}

// SupportSource says where support comes from: the linked inbox and the property's path rules.
type SupportSource struct {
	WorkspaceID uuid.UUID
	PropertyID  uuid.UUID
	InboxID     uuid.UUID
	Timezone    string
	Rules       []PathRule
}

type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// fromInbox reads the inbox's conversations, page feedback, page ratings and published discussion
// threads of the local days from to, by page and day. Page ratings are counted on the day they
// were given, which is a UTC day.
func (src SupportSource) fromInbox(ctx context.Context, q querier, from, to time.Time) ([]SupportRow, error) {
	loc, err := time.LoadLocation(src.Timezone)
	if err != nil {
		loc = time.UTC
	}
	start := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, loc)
	end := DayEnd(to, loc)
	rows, err := q.Query(ctx, `SELECT page, day, sum(c)::bigint, sum(f)::bigint, sum(up)::bigint, sum(down)::bigint, sum(d)::bigint FROM (
		SELECT page_url AS page, (created_at AT TIME ZONE $3)::date AS day, count(*) FILTER (WHERE kind <> 'feedback') AS c,
			count(*) FILTER (WHERE kind = 'feedback') AS f, 0::bigint AS up, 0::bigint AS down, 0::bigint AS d
		FROM conversations
		WHERE workspace_id = $1 AND inbox_id = $2 AND page_url IS NOT NULL AND NOT spam AND trashed_at IS NULL AND created_at >= $6 AND created_at < $7
		GROUP BY 1, 2
		UNION ALL
		SELECT page, day, 0, 0, sum(up), sum(down), 0 FROM page_ratings
		WHERE workspace_id = $1 AND inbox_id = $2 AND day BETWEEN $4 AND $5
		GROUP BY 1, 2
		UNION ALL
		SELECT page, (created_at AT TIME ZONE $3)::date, 0, 0, 0, 0, count(*) FROM discussions
		WHERE workspace_id = $1 AND inbox_id = $2 AND kind = 'page' AND state = 'published' AND created_at >= $6 AND created_at < $7
		GROUP BY 1, 2
	) AS t GROUP BY 1, 2`, src.WorkspaceID, src.InboxID, loc.String(), from, to, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byKey := map[string]int{}
	var out []SupportRow
	for rows.Next() {
		var page string
		var day time.Time
		var s Sums
		if err := rows.Scan(&page, &day, &s.Conversations, &s.Feedback, &s.RatingsUp, &s.RatingsDown, &s.Discussions); err != nil {
			return nil, err
		}
		pg, ok := ParsePage(page, false)
		if !ok {
			continue
		}
		path := ApplyRules(src.Rules, pg.Path)
		day = day.UTC()
		k := day.Format(time.DateOnly) + valueSep + pg.Host + valueSep + path
		if i, ok := byKey[k]; ok {
			out[i].Sums.Add(s)
			continue
		}
		byKey[k] = len(out)
		out = append(out, SupportRow{Day: day, Host: pg.Host, Path: path, Sums: s})
	}
	return out, rows.Err()
}

// RollupSupportDay replaces a property's support rollup of one local day with what its inbox holds
// now, and records the inbox it was built from.
func (s *PGStore) RollupSupportDay(ctx context.Context, src SupportSource, day time.Time) error {
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		rows, err := src.fromInbox(ctx, tx, day, day)
		if err != nil {
			return fmt.Errorf("support rollup: %w", err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM analytics_daily_support WHERE workspace_id = $1 AND property_id = $2 AND day = $3`,
			src.WorkspaceID, src.PropertyID, day); err != nil {
			return err
		}
		copyRows := make([][]any, len(rows))
		for i, r := range rows {
			copyRows[i] = []any{src.WorkspaceID, src.PropertyID, day, r.Host, r.Path, r.Sums.Conversations, r.Sums.Feedback,
				r.Sums.RatingsUp, r.Sums.RatingsDown, r.Sums.Discussions}
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"analytics_daily_support"}, []string{"workspace_id", "property_id", "day", "host", "path",
			"conversations", "feedback", "ratings_up", "ratings_down", "discussions"}, pgx.CopyFromRows(copyRows)); err != nil {
			return fmt.Errorf("support rollup: %w", err)
		}
		_, err = tx.Exec(ctx, `INSERT INTO analytics_support_days (workspace_id, property_id, day, inbox_id, built_at) VALUES ($1, $2, $3, $4, now())
			ON CONFLICT (property_id, day) DO UPDATE SET inbox_id = excluded.inbox_id, built_at = excluded.built_at`,
			src.WorkspaceID, src.PropertyID, day, src.InboxID)
		return err
	})
}

// Support reads a property's support of the local days from to: built days from the rollup, the
// others from the inbox.
func (s *PGStore) Support(ctx context.Context, src SupportSource, from, to time.Time) (rows []SupportRow, servedFrom string, err error) {
	err = pgx.BeginTxFunc(ctx, s.Pool, pgx.TxOptions{AccessMode: pgx.ReadOnly, IsoLevel: pgx.RepeatableRead}, func(tx pgx.Tx) error {
		if err := queryLimits(ctx, tx); err != nil {
			return err
		}
		r, err := tx.Query(ctx, `SELECT day FROM analytics_support_days WHERE workspace_id = $1 AND property_id = $2 AND day BETWEEN $3 AND $4 AND inbox_id = $5`,
			src.WorkspaceID, src.PropertyID, from, to, src.InboxID)
		if err != nil {
			return err
		}
		built, err := pgx.CollectRows(r, pgx.RowTo[time.Time])
		if err != nil {
			return err
		}
		isBuilt := map[time.Time]bool{}
		for _, d := range built {
			isBuilt[d.UTC()] = true
		}
		var open []time.Time
		for _, d := range days(from, to) {
			if !isBuilt[d] {
				open = append(open, d)
			}
		}
		if len(built) > 0 {
			r, err := tx.Query(ctx, `SELECT day, host, path, conversations, feedback, ratings_up, ratings_down, discussions FROM analytics_daily_support
				WHERE workspace_id = $1 AND property_id = $2 AND day = ANY($3::date[])`, src.WorkspaceID, src.PropertyID, built)
			if err != nil {
				return err
			}
			for r.Next() {
				var x SupportRow
				if err := r.Scan(&x.Day, &x.Host, &x.Path, &x.Sums.Conversations, &x.Sums.Feedback, &x.Sums.RatingsUp, &x.Sums.RatingsDown,
					&x.Sums.Discussions); err != nil {
					r.Close()
					return err
				}
				x.Day = x.Day.UTC()
				rows = append(rows, x)
			}
			r.Close()
			if err := r.Err(); err != nil {
				return err
			}
		}
		if len(open) > 0 {
			live, err := src.fromInbox(ctx, tx, open[0], open[len(open)-1])
			if err != nil {
				return err
			}
			for _, x := range live {
				if !isBuilt[x.Day] {
					rows = append(rows, x)
				}
			}
		}
		switch {
		case len(open) == 0:
			servedFrom = ServedRollups
		case len(built) == 0:
			servedFrom = ServedRaw
		default:
			servedFrom = ServedMixed
		}
		return nil
	})
	return rows, servedFrom, err
}

// MatchValue applies a filter to one value in Go, as the SQL filters do.
func MatchValue(f Filter, value string, path bool) bool {
	switch f.Op {
	case "is":
		return slices.Contains(f.Values, value)
	case "is_not":
		return !slices.Contains(f.Values, value)
	case "contains", "not_contains":
		found := slices.ContainsFunc(f.Values, func(v string) bool { return strings.Contains(strings.ToLower(value), strings.ToLower(v)) })
		return found == (f.Op == "contains")
	case "matches":
		return slices.ContainsFunc(f.Values, func(v string) bool {
			if path && !strings.Contains(v, "*") {
				return value == v || value == v+"/"
			}
			ok, _ := regexp.MatchString(patternRegex(v, path), value)
			return ok
		})
	}
	return false
}

// Bucket is the start of the time bucket of a grain (day, week or month) a day falls in.
func Bucket(grain string, day time.Time) string {
	p := plan{grain: grain}
	return p.bucket(day, 0)
}

// Days lists the days from to.
func Days(from, to time.Time) []time.Time { return days(from, to) }
