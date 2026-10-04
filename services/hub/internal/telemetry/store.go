// Package telemetry derives the hub's event metrics from its own PostgreSQL
// records. It is the backend the console's KPI cards and throughput charts read
// by default, on every install: the four counters those panels otherwise ask
// Prometheus about are incremented while the hub routes, and every routing
// decision that increments one leaves a row behind in events, agent_tasks or
// task_logs. This package is the single place that says what those four metrics
// mean in those tables.
//
// Reading the rows rather than the counters is the more truthful of the two, not
// merely the more available one: a counter lives in a process and restarts at zero
// with its pod, while events and agent_tasks are never pruned. The exception
// callers must know about is task_logs, which cmd/hub prunes after 7 days — a
// window at or beyond that reports the errors still retained, not every error the
// window produced. Prometheus has the same weakness in miniature (retention) and
// the same strength (a counter that predates a prune still remembers), which is why
// the panels remain pinnable to the counters rather than welded to this package.
package telemetry

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Metric identifiers. These are the same tokens the observability API accepts
// on ?metric=, so a caller can pass what it received without a translation step.
const (
	MetricEventsConsumed  = "events_consumed"
	MetricTriggersMatched = "triggers_matched"
	MetricEventsRouted    = "events_routed"
	MetricRoutingErrors   = "routing_errors"
)

// Errors reported by a query this store cannot answer.
var (
	// ErrNoDatabase is returned when the store was built without a pool.
	ErrNoDatabase = errors.New("hub database not configured")
	// ErrUnknownMetric is returned for a metric with no records behind it.
	ErrUnknownMetric = errors.New("unknown metric")
)

// Totals holds one count per metric over a window.
//
// The field names mirror the metric identifiers, not the Prometheus counter
// names: RoutingErrors answers "what went wrong in this window" from the error
// log the console's Errors page lists, which is why it does not track the
// router-internal hub_routing_errors_total counter.
type Totals struct {
	// EventsConsumed counts events the hub received in the window.
	EventsConsumed int
	// TriggersMatched counts routing decisions — one per agent an event was
	// matched to, which is what hub_triggers_matched_total adds up.
	TriggersMatched int
	// EventsRouted counts distinct events that reached at least one agent. It
	// is therefore lower than TriggersMatched when one event went to several
	// agents.
	EventsRouted int
	// RoutingErrors counts error-level task logs in the window.
	RoutingErrors int
}

// Bucket is one step-wide slice of a query window and the number of matching
// records in it.
type Bucket struct {
	// Start is the bucket's inclusive lower bound, aligned to the window start.
	Start time.Time
	Count int
}

// metricSpec binds a metric to the records that answer it. Every field is a
// compile-time constant: the SQL this package builds interpolates these values
// and positional parameters only, never a string that arrived in a request.
type metricSpec struct {
	table   string
	timeCol string
	// countExpr is the aggregate for the metric.
	countExpr string
	// filter narrows the table to the rows this metric counts, or "" for all.
	filter string
}

// totalsOrder fixes the metric order used by Totals, so the statement it builds
// and the struct it fills cannot drift apart between metrics being added.
var totalsOrder = []string{
	MetricEventsConsumed,
	MetricTriggersMatched,
	MetricEventsRouted,
	MetricRoutingErrors,
}

var metricSpecs = map[string]metricSpec{
	MetricEventsConsumed: {
		table:     "events",
		timeCol:   "received_at",
		countExpr: "count(*)",
	},
	MetricTriggersMatched: {
		table:     "agent_tasks",
		timeCol:   "created_at",
		countExpr: "count(*)",
	},
	MetricEventsRouted: {
		table:     "agent_tasks",
		timeCol:   "created_at",
		countExpr: "count(DISTINCT event_id)",
	},
	MetricRoutingErrors: {
		table:     "task_logs",
		timeCol:   "created_at",
		countExpr: "count(*)",
		filter:    "level = 'error'",
	},
}

// windowPredicate renders the `WHERE` fragment restricting a metric's table to
// [since, until). $1 is the window start, $2 its end.
func (spec metricSpec) windowPredicate() string {
	pred := fmt.Sprintf("%s.%s >= $1 AND %s.%s < $2", spec.table, spec.timeCol, spec.table, spec.timeCol)
	if spec.filter != "" {
		pred += " AND " + spec.filter
	}
	return pred
}

// subcount renders the metric's count as a scalar subquery of the Totals
// statement.
func (spec metricSpec) subcount() string {
	return fmt.Sprintf("(SELECT %s FROM %s WHERE %s)", spec.countExpr, spec.table, spec.windowPredicate())
}

// Store answers metric queries from the hub's PostgreSQL records.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore returns a telemetry store over an existing pool. The pool is owned
// by the caller. A nil pool yields a store that reports ErrNoDatabase instead of
// panicking, so wiring never has to special-case a hub without a database.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Ready reports whether the store can answer anything. NewStore tolerates a nil
// pool so wiring never has to special-case a hub without a database, but a store
// over a nil pool is not a metrics backend — every query it takes fails with
// ErrNoDatabase. Callers choosing between backends ask this instead of asking
// whether a pointer was set, so such a hub reports that no metrics source is
// configured rather than advertising "postgres" and then erroring.
func (s *Store) Ready() bool {
	return s != nil && s.pool != nil
}

// SupportsMetric reports whether this store can answer queries for name.
func SupportsMetric(name string) bool {
	_, ok := metricSpecs[name]
	return ok
}

// Totals counts every metric over [since, until) in a single round trip. The
// dashboard polls this on every range change, and four sequential queries —
// one per metric, as the Prometheus path does — would be four chances for one
// to fail and blank the whole card row.
func (s *Store) Totals(ctx context.Context, since, until time.Time) (Totals, error) {
	if s.pool == nil {
		return Totals{}, ErrNoDatabase
	}

	parts := make([]string, 0, len(totalsOrder))
	for _, name := range totalsOrder {
		parts = append(parts, metricSpecs[name].subcount())
	}
	query := "SELECT " + strings.Join(parts, ", ")

	vals := make([]int, len(totalsOrder))
	scanArgs := make([]any, len(vals))
	for i := range vals {
		scanArgs[i] = &vals[i]
	}
	if err := s.pool.QueryRow(ctx, query, since, until).Scan(scanArgs...); err != nil {
		return Totals{}, fmt.Errorf("telemetry.Totals: %w", err)
	}

	return Totals{
		EventsConsumed:  vals[0],
		TriggersMatched: vals[1],
		EventsRouted:    vals[2],
		RoutingErrors:   vals[3],
	}, nil
}

// Series counts a metric's records into fixed-width buckets across
// [since, until). Buckets align to `since`, and only buckets holding records
// come back: the caller zero-fills, so an absence of rows stays an absence the
// caller can render, rather than a count this package invented.
//
// The alignment on `since` is load-bearing, not cosmetic: the metrics handler
// files each bucket on its own grid by rounding (api.denseBuckets), which is
// only unambiguous while the distance from `since` is a whole number of steps.
// A bucket derived from any other origin — an epoch-aligned date_bin, say — is
// refused there rather than silently mis-dating the chart.
func (s *Store) Series(ctx context.Context, metric string, since, until time.Time, step time.Duration) ([]Bucket, error) {
	if s.pool == nil {
		return nil, ErrNoDatabase
	}
	spec, ok := metricSpecs[metric]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownMetric, metric)
	}
	if step <= 0 {
		return nil, fmt.Errorf("telemetry.Series: step must be positive, got %s", step)
	}

	// Bucket start = window start + (elapsed seconds since the window start,
	// floored to a whole number of steps). $1/$2 are the window bounds, $3 the
	// step width in seconds.
	query := fmt.Sprintf(
		`SELECT ($1::timestamptz
		           + (floor(extract(epoch FROM (%s.%s - $1::timestamptz)) / $3::float8) * $3::float8)
		           * interval '1 second')::timestamptz AS bucket,
		        %s AS count
		   FROM %s
		  WHERE %s
		  GROUP BY 1
		  ORDER BY 1`,
		spec.table, spec.timeCol, spec.countExpr, spec.table, spec.windowPredicate(),
	)

	rows, err := s.pool.Query(ctx, query, since, until, step.Seconds())
	if err != nil {
		return nil, fmt.Errorf("telemetry.Series %s: %w", metric, err)
	}
	defer rows.Close()

	var out []Bucket
	for rows.Next() {
		var b Bucket
		if err := rows.Scan(&b.Start, &b.Count); err != nil {
			return nil, fmt.Errorf("telemetry.Series %s scan: %w", metric, err)
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("telemetry.Series %s rows: %w", metric, err)
	}
	return out, nil
}
