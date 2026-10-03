package api

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DominikPinsel/ainsel/services/hub/internal/prometheus"
	"github.com/DominikPinsel/ainsel/services/hub/internal/tasklogs"
	"github.com/DominikPinsel/ainsel/services/hub/internal/telemetry"
	agentv1alpha1 "github.com/DominikPinsel/ainsel/shared/api/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// observabilityCacheTTL is the duration cached Prometheus query results stay fresh.
// 30s matches the Prometheus default scrape interval, so the dashboard only
// pays for one query per metric per scrape window even under heavy fan-in.
const observabilityCacheTTL = 30 * time.Second

// Values of the "source" field on metrics responses. It names the backend that
// answered, so an operator reading the JSON over curl — or a support report
// quoting it — can tell Prometheus figures from the hub's own records instead of
// guessing from which panels have data.
const (
	metricsSourcePrometheus = "prometheus"
	metricsSourcePostgres   = "postgres"
)

// Reasons a panel cannot be answered. Each one names the dependency that is
// missing and where to set it, because the console shows this text to the
// operator verbatim.
const (
	// metricsRequiredMessage is reported when neither Prometheus nor the hub's
	// database can serve an event metric.
	metricsRequiredMessage = "no metrics backend: set observability.prometheus.url, or give the hub a database to read its own records from"
	// promRequiredMessage is reported for the panels only Prometheus can answer.
	promRequiredMessage = "prometheus not configured: set observability.prometheus.url"
	// logStoreRequiredMessage is reported for the panels that read the hub's log
	// and conversation tables.
	logStoreRequiredMessage = "hub database not configured: these panels read the hub's own task log table"
)

// allTimeStart is the lower bound of a "no window" query. The range-less summary
// path has always meant "everything so far"; read from the hub's records that
// becomes everything still retained, which is the closest equivalent.
var allTimeStart = time.Unix(0, 0).UTC()

// hubMetric describes one of the hub-internal Prometheus counters surfaced via
// the summary endpoint and queryable by name in the timeseries endpoint.
type hubMetric struct {
	// Name is the user-facing identifier (also the JSON field on the summary).
	Name string
	// PromQL is the instant query for the current value.
	PromQL string
	// RatePromQL is the per-second rate query used by the timeseries endpoint;
	// if empty, the raw counter is returned.
	RatePromQL string
}

// hubMetrics is the registry of hub-internal counters exposed by the API.
// Adding a metric here automatically makes it appear in the summary and
// queryable by name in the timeseries endpoint.
var hubMetrics = []hubMetric{
	{
		Name:       "events_consumed",
		PromQL:     "sum(hub_events_consumed_total)",
		RatePromQL: "sum(rate(hub_events_consumed_total[%s]))",
	},
	{
		Name:       "triggers_matched",
		PromQL:     "sum(hub_triggers_matched_total)",
		RatePromQL: "sum(rate(hub_triggers_matched_total[%s]))",
	},
	{
		Name:       "events_routed",
		PromQL:     "sum(hub_events_routed_total)",
		RatePromQL: "sum(rate(hub_events_routed_total[%s]))",
	},
	{
		Name:       "routing_errors",
		PromQL:     "sum(hub_routing_errors_total)",
		RatePromQL: "sum(rate(hub_routing_errors_total[%s]))",
	},
}

// MetricsSummary holds the current values of the hub-internal counters.
//
// Source names the backend the figures came from: prometheus, or postgres when
// the hub answered from its own event records.
//
// RoutingErrors is named for its original source (hub_routing_errors_total),
// but with a range set it reports error-level task logs within the window —
// the same entries the errors page lists. The routing-errors counter only
// tracks router failures and was effectively always zero, which left the
// Errors KPI card stuck at 0 even when events were failing.
type MetricsSummary struct {
	EventsConsumed  float64   `json:"eventsConsumed"`
	TriggersMatched float64   `json:"triggersMatched"`
	EventsRouted    float64   `json:"eventsRouted"`
	RoutingErrors   float64   `json:"routingErrors"`
	UpdatedAt       time.Time `json:"updatedAt"`
	Source          string    `json:"source,omitempty"`
}

// TimeseriesPoint is one (timestamp, value) pair.
type TimeseriesPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

// MetricsTimeseries is the response for the timeseries endpoint.
//
// Step is the bucket width, and the unit of Value depends on Source: the
// Prometheus path reports a per-second rate, the hub's own records report a
// count per bucket. Point counts are dense on both paths.
type MetricsTimeseries struct {
	Metric string            `json:"metric"`
	Range  string            `json:"range"`
	Step   string            `json:"step"`
	Points []TimeseriesPoint `json:"points"`
	Source string            `json:"source,omitempty"`
}

// AgentMetric is per-agent token consumption + invocation counts.
type AgentMetric struct {
	Agent            string  `json:"agent"`
	AgentName        string  `json:"agentName"`
	InputTokens      float64 `json:"inputTokens"`
	OutputTokens     float64 `json:"outputTokens"`
	CacheReadTokens  float64 `json:"cacheReadTokens"`
	CacheWriteTokens float64 `json:"cacheWriteTokens"`
	TotalTokens      float64 `json:"totalTokens"`
	Invocations      float64 `json:"invocations"`
}

// AgentsMetricsResponse wraps the per-agent metrics.
type AgentsMetricsResponse struct {
	Agents    []AgentMetric `json:"agents"`
	UpdatedAt time.Time     `json:"updatedAt"`
}

// rangeOption maps a user-facing range token to a Prometheus duration + step.
// The step is chosen to give roughly 60-180 points per chart so the frontend
// can render smoothly without exploding the response size.
type rangeOption struct {
	Duration time.Duration
	Step     time.Duration
	// PromRange is the Prometheus range vector duration string, e.g. "1m".
	PromRange string
}

var supportedRanges = map[string]rangeOption{
	"1h":  {Duration: 1 * time.Hour, Step: 30 * time.Second, PromRange: "1m"},
	"6h":  {Duration: 6 * time.Hour, Step: 3 * time.Minute, PromRange: "5m"},
	"24h": {Duration: 24 * time.Hour, Step: 10 * time.Minute, PromRange: "15m"},
	"7d":  {Duration: 7 * 24 * time.Hour, Step: 1 * time.Hour, PromRange: "1h"},
}

// promCache is a tiny TTL cache keyed by a query identifier. It is concurrency-safe
// and intentionally minimal — entries are never evicted on size, only on TTL.
type promCache struct {
	mu      sync.Mutex
	entries map[string]promCacheEntry
	ttl     time.Duration
}

type promCacheEntry struct {
	value    interface{}
	expireAt time.Time
}

func newPromCache(ttl time.Duration) *promCache {
	return &promCache{
		entries: make(map[string]promCacheEntry),
		ttl:     ttl,
	}
}

// get returns (value, true) if a fresh entry exists.
func (c *promCache) get(key string) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(e.expireAt) {
		delete(c.entries, key)
		return nil, false
	}
	return e.value, true
}

// set stores a value with the cache's configured TTL.
func (c *promCache) set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = promCacheEntry{
		value:    value,
		expireAt: time.Now().Add(c.ttl),
	}
}

// SetTelemetryStore wires the store that reads the hub's own records for the
// event metric panels. It is the fallback used when no Prometheus is configured,
// which is the common case: the chart ships no Prometheus of its own and the
// value defaults to empty.
func (s *Server) SetTelemetryStore(store *telemetry.Store) {
	s.telemetry = store
}

// handleObservability dispatches to the observability sub-handlers.
//
// The canonical paths live under /api/v1/observability/metrics/*. We also
// accept the legacy /api/v1/metrics/* prefix because the deployed frontend
// in ainsel-dev (pre-PR-50) calls those paths. Legacy responses are tagged
// with a Deprecation/Link header pair (RFC 8594 + RFC 8288) so frontends can
// detect the alias and migrate without us having to break them mid-flight.
//
// Which gate a path gets is the part worth reading: these panels need different
// backends, so each is gated on what it actually queries. They all used to
// require Prometheus, which blanked panels that never ask it anything.
func (s *Server) handleObservability(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/v1/observability/metrics/summary":
		s.observabilityMethodGate(w, r, s.getMetricsSummary)
	case "/api/v1/observability/metrics/timeseries":
		s.observabilityMethodGate(w, r, s.getMetricsTimeseries)
	case "/api/v1/observability/metrics/agents":
		s.prometheusMethodGate(w, r, s.getAgentsMetrics)
	case "/api/v1/observability/metrics/tokens/summary":
		s.prometheusMethodGate(w, r, s.getTokensSummary)
	case "/api/v1/observability/metrics/tokens/timeseries":
		s.prometheusMethodGate(w, r, s.getTokensTimeseries)
	case "/api/v1/observability/metrics/tokens/by-subject":
		s.prometheusMethodGate(w, r, s.getTokensBySubject)
	case "/api/v1/observability/metrics/tokens/by-event":
		s.observabilityMethodGate(w, r, s.getTokensByEvent)
	case "/api/v1/metrics/summary":
		setDeprecationHeaders(w, "/api/v1/observability/metrics/summary")
		s.observabilityMethodGate(w, r, s.getMetricsSummary)
	case "/api/v1/metrics/timeseries":
		setDeprecationHeaders(w, "/api/v1/observability/metrics/timeseries")
		s.observabilityMethodGate(w, r, s.getMetricsTimeseries)
	case "/api/v1/metrics/agents":
		setDeprecationHeaders(w, "/api/v1/observability/metrics/agents")
		s.prometheusMethodGate(w, r, s.getAgentsMetrics)
	default:
		writeError(w, http.StatusNotFound, "not found")
	}
}

// handleObservabilityMetricsQuery serves GET /api/v1/observability/metrics/query.
// It is a thin raw proxy: it accepts ?query=<promql>&time=<optional rfc3339/unix>
// and forwards the request directly to Prometheus, returning the unmodified JSON
// response. This allows MCP tools and power users to run freeform PromQL without
// needing a direct Prometheus connection.
//
// Restricted to admins: this is an operator escape hatch, not a dashboard
// input — the UI reads the structured summary/timeseries/agents endpoints,
// which build their own namespace-scoped PromQL. Handing every authenticated
// user a raw Prometheus proxy let them read metrics from outside the AInsel
// namespace and submit arbitrarily expensive expressions.
func (s *Server) handleObservabilityMetricsQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.prom == nil {
		writeError(w, http.StatusServiceUnavailable, promRequiredMessage)
		return
	}
	// With no authz wired there is no notion of admin and the whole API is open
	// by design, so the check passes through exactly as requireRead does.
	// requireAdmin alone would fail closed here and break local development.
	if s.authzChecker != nil && !s.requireAdmin(w, r) {
		return
	}
	query := r.URL.Query().Get("query")
	if query == "" {
		writeError(w, http.StatusBadRequest, "query is required")
		return
	}
	// Namespace scoping, kept as defence in depth now that the endpoint is
	// admin-only. Note this is a substring heuristic, not a PromQL parse: it
	// confirms the query mentions the namespace somewhere, which an expression
	// like `up{namespace="ainsel"} or up` satisfies while still selecting
	// unscoped series. It is adequate as a backstop against an admin typo, and
	// must not be relied on as the access control.
	ns := s.ns
	if !containsNamespaceMatcher(query, ns) {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("query must be scoped to namespace %q (e.g. {namespace=%q})", ns, ns))
		return
	}
	body, err := s.prom.QueryRaw(r.Context(), query, r.URL.Query().Get("time"))
	if err != nil {
		writeError(w, http.StatusBadGateway, "prometheus query failed: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// containsNamespaceMatcher checks whether a PromQL query contains a namespace
// label matcher for the given namespace. It looks for both namespace="<ns>"
// and namespace=~"<ns>" patterns.
//
// This is a substring heuristic: it does not parse the expression, so a query
// that mentions the matcher anywhere passes even when other parts of it select
// unscoped series. See handleObservabilityMetricsQuery, which no longer relies
// on it as the sole control.
func containsNamespaceMatcher(query, ns string) bool {
	sanitized := sanitizeLabelValue(ns)
	return strings.Contains(query, fmt.Sprintf("namespace=%q", sanitized)) ||
		strings.Contains(query, fmt.Sprintf("namespace=~%q", sanitized))
}

// setDeprecationHeaders marks a response as a deprecated alias and points
// callers at the canonical successor URL. Headers follow RFC 8594 (Deprecation)
// and RFC 8288 (Link). We intentionally don't set a Sunset header — the alias
// will be removed once the frontend dashboard PR (ainsel-hub-frontend#50)
// lands and the deployed bundle is updated; that timeline is decided by the
// frontend team, not by us.
func setDeprecationHeaders(w http.ResponseWriter, successor string) {
	w.Header().Set("Deprecation", "true")
	w.Header().Set("Link", fmt.Sprintf(`<%s>; rel="successor-version"`, successor))
}

func (s *Server) observabilityMethodGate(w http.ResponseWriter, r *http.Request, h func(http.ResponseWriter, *http.Request)) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	h(w, r)
}

// prometheusMethodGate serves the panels only Prometheus can answer. Agent token
// and invocation series are published by the runtime to metrics this hub does not
// keep rows for, so a hub with no Prometheus has nothing to offer them — the 503
// says as much, naming the setting that would fix it.
func (s *Server) prometheusMethodGate(w http.ResponseWriter, r *http.Request, h func(http.ResponseWriter, *http.Request)) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.prom == nil {
		writeError(w, http.StatusServiceUnavailable, promRequiredMessage)
		return
	}
	h(w, r)
}

// metricsBackend chooses the source that answers an event-metric query.
//
// Prometheus wins when configured, because counters cover everything the process
// has done while the hub's rows are bounded by retention. With no Prometheus the
// hub answers from the records it wrote while routing, which is what lets a
// standalone install — the common shape, as the chart ships no Prometheus of its
// own — show real charts instead of an empty panel.
//
// The records are only a backend if the store can actually read them. A store
// built over a nil pool exists but answers nothing, and naming it here would
// turn "no metrics source configured" into a query failure.
func (s *Server) metricsBackend() (string, bool) {
	switch {
	case s.prom != nil:
		return metricsSourcePrometheus, true
	case s.telemetry.Ready():
		return metricsSourcePostgres, true
	default:
		return "", false
	}
}

func (s *Server) getMetricsSummary(w http.ResponseWriter, r *http.Request) {
	rangeKey := r.URL.Query().Get("range")

	// When a range is supplied, validate it against the supported set and
	// switch to increase()-based queries so the KPI cards show windowed
	// counts instead of all-time cumulative counters. When omitted, the
	// endpoint preserves its legacy all-time semantics for backward
	// compatibility with legacy aliases and MCP consumers.
	var rng *rangeOption
	if rangeKey != "" {
		opt, ok := supportedRanges[rangeKey]
		if !ok {
			writeError(w, http.StatusBadRequest, "unsupported range; allowed values: 1h, 6h, 24h, 7d")
			return
		}
		rng = &opt
	}

	source, ok := s.metricsBackend()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, metricsRequiredMessage)
		return
	}

	// The cache is keyed by source so a hub that gains Prometheus after running
	// without it cannot serve records-shaped figures under a prometheus label, or
	// the other way round, for the length of the TTL.
	cacheKey := "summary:" + source
	if rng != nil {
		cacheKey += ":" + rangeKey
	}
	if cached, ok := s.observabilityCache.get(cacheKey); ok {
		writeJSON(w, http.StatusOK, cached)
		return
	}

	var summary MetricsSummary
	var err error
	if source == metricsSourcePostgres {
		summary, err = s.summaryFromRecords(r.Context(), rng)
	} else {
		summary, err = s.summaryFromPrometheus(r.Context(), rng, rangeKey)
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	s.observabilityCache.set(cacheKey, summary)
	writeJSON(w, http.StatusOK, summary)
}

// summaryFromPrometheus reads the four hub counters. With a range it uses
// increase() so the card shows the count inside the window rather than the
// all-time total.
func (s *Server) summaryFromPrometheus(ctx context.Context, rng *rangeOption, rangeKey string) (MetricsSummary, error) {
	summary := MetricsSummary{UpdatedAt: time.Now().UTC(), Source: metricsSourcePrometheus}
	for _, m := range hubMetrics {
		// With a range set, the Errors card counts error-level task logs in
		// the window (what the errors page lists) instead of the
		// hub_routing_errors_total counter. The counter remains the source on
		// the legacy range-less path and when no log store is configured.
		if m.Name == "routing_errors" && rng != nil && s.taskLogs != nil {
			count, err := s.taskLogs.CountByLevelSince(ctx, tasklogs.LevelError, time.Now().UTC().Add(-rng.Duration))
			if err != nil {
				return MetricsSummary{}, fmt.Errorf("failed to count error task logs: %s", err.Error())
			}
			summary.RoutingErrors = float64(count)
			continue
		}
		query := m.PromQL
		if rng != nil {
			// Use increase() over the requested range so the card shows
			// the count within the window, not the all-time total.
			// m.PromQL is "sum(<counter>)" — extract the counter name.
			counter := strings.TrimSuffix(strings.TrimPrefix(m.PromQL, "sum("), ")")
			query = fmt.Sprintf("sum(increase(%s[%s]))", counter, rangeKey)
		}
		val, err := singleScalar(ctx, s.prom, query)
		if err != nil {
			return MetricsSummary{}, fmt.Errorf("failed to query %s: %s", m.Name, err.Error())
		}
		// These metrics are event counts. increase() extrapolates fractional
		// values (e.g. 64.7826), which surfaced on the dashboard KPI cards as
		// long decimals; round to the nearest whole count before returning.
		val = math.Round(val)
		switch m.Name {
		case "events_consumed":
			summary.EventsConsumed = val
		case "triggers_matched":
			summary.TriggersMatched = val
		case "events_routed":
			summary.EventsRouted = val
		case "routing_errors":
			summary.RoutingErrors = val
		}
	}
	return summary, nil
}

// summaryFromRecords answers the same four figures from the hub's own tables.
// It is the path a hub with no Prometheus takes, and it counts what actually
// happened rather than what a counter remembered: a window older than the hub's
// retained rows reports zero, because the rows it would have counted are gone.
func (s *Server) summaryFromRecords(ctx context.Context, rng *rangeOption) (MetricsSummary, error) {
	end := time.Now().UTC()
	start := allTimeStart
	if rng != nil {
		start = end.Add(-rng.Duration)
	}

	totals, err := s.telemetry.Totals(ctx, start, end)
	if err != nil {
		return MetricsSummary{}, fmt.Errorf("failed to read metrics from the hub database: %s", err.Error())
	}
	return MetricsSummary{
		EventsConsumed:  float64(totals.EventsConsumed),
		TriggersMatched: float64(totals.TriggersMatched),
		EventsRouted:    float64(totals.EventsRouted),
		RoutingErrors:   float64(totals.RoutingErrors),
		UpdatedAt:       end,
		Source:          metricsSourcePostgres,
	}, nil
}

func (s *Server) getMetricsTimeseries(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	metricName := q.Get("metric")
	rangeKey := q.Get("range")
	if metricName == "" {
		// Dashboards load this endpoint with just a range when the user hasn't
		// picked a metric yet; default to events_consumed so the chart has
		// something to show.
		metricName = "events_consumed"
	}
	if rangeKey == "" {
		rangeKey = "1h"
	}

	rng, ok := supportedRanges[rangeKey]
	if !ok {
		writeError(w, http.StatusBadRequest, "unsupported range; allowed values: 1h, 6h, 24h, 7d")
		return
	}

	metric, ok := findHubMetric(metricName)
	if !ok {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unknown metric %q", metricName))
		return
	}

	source, ok := s.metricsBackend()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, metricsRequiredMessage)
		return
	}
	// Two registries, deliberately kept separate: hubMetrics names what can be
	// scraped, telemetry names what the hub has rows for. A metric in one but not
	// the other must be refused, not drawn as an empty chart.
	if source == metricsSourcePostgres && !telemetry.SupportsMetric(metric.Name) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("metric %q is not readable from the hub's own records", metric.Name))
		return
	}

	cacheKey := fmt.Sprintf("ts:%s:%s:%s", source, metric.Name, rangeKey)
	if cached, ok := s.observabilityCache.get(cacheKey); ok {
		writeJSON(w, http.StatusOK, cached)
		return
	}

	end := time.Now().UTC()
	start := end.Add(-rng.Duration)

	var points []TimeseriesPoint
	var err error
	if source == metricsSourcePostgres {
		points, err = s.pointsFromRecords(r.Context(), metric.Name, start, end, rng.Step)
	} else {
		points, err = s.pointsFromPrometheus(r.Context(), metric, rng, start, end)
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	resp := MetricsTimeseries{
		Metric: metric.Name,
		Range:  rangeKey,
		Step:   rng.Step.String(),
		Points: points,
		Source: source,
	}
	s.observabilityCache.set(cacheKey, resp)
	writeJSON(w, http.StatusOK, resp)
}

// pointsFromPrometheus samples a counter's rate across the window, so the chart
// shows throughput rather than the monotonically-increasing raw counter.
func (s *Server) pointsFromPrometheus(ctx context.Context, metric hubMetric, rng rangeOption, start, end time.Time) ([]TimeseriesPoint, error) {
	query := metric.PromQL
	if metric.RatePromQL != "" {
		query = fmt.Sprintf(metric.RatePromQL, rng.PromRange)
	}

	result, err := s.prom.QueryRange(ctx, query, start, end, rng.Step)
	if err != nil {
		return nil, errors.New("failed to query metrics: " + err.Error())
	}

	points := make([]TimeseriesPoint, 0)
	if len(result.Series) > 0 {
		// We use sum(...) queries which collapse to a single series; if multiple
		// come back (e.g. caller passes a custom metric with labels) we just take
		// the first one to keep the wire format stable.
		for _, sample := range result.Series[0].Samples {
			points = append(points, TimeseriesPoint{
				Timestamp: sample.Timestamp.UTC(),
				Value:     sample.Value,
			})
		}
	}
	return points, nil
}

// pointsFromRecords counts the hub's own rows into one point per bucket. Values
// are counts per bucket, not the per-second rate the Prometheus path reports;
// the response's source field tells the reader which of the two it is looking at.
func (s *Server) pointsFromRecords(ctx context.Context, metric string, start, end time.Time, step time.Duration) ([]TimeseriesPoint, error) {
	rows, err := s.telemetry.Series(ctx, metric, start, end, step)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s from the hub database: %s", metric, err.Error())
	}
	return denseBuckets(rows, start, end, step)
}

// maxBucketDrift bounds how far a bucket the records store reports may sit from
// the grid point it is filed to. See denseBuckets.
const maxBucketDrift = time.Millisecond

// maxSeriesPoints bounds the zero-filled response. The supported ranges step to
// at most 120-180 points; the cap exists so a future fine-grained step cannot
// turn one dashboard poll into a megabyte of JSON.
const maxSeriesPoints = 2000

// denseBuckets lays the store's buckets onto a fixed grid spanning
// [start, end), zero-filling the gaps. The chart places bars by index, so
// passing the sparse result straight through would spread a handful of events
// across the whole window and misreport when they happened.
//
// The grid spans [start, end): a window that divides evenly by step yields one
// fewer point than Prometheus' QueryRange, which samples its end bound
// inclusively. The chart lays bars out by index, so both backends render the
// same shape; only a response diff sees the difference.
//
// Bucket bounds are matched to the grid by rounding, not truncating: see the
// comment in the loop below. A bucket that is not on this grid at all is an
// error rather than a mis-dated chart, so the function reports one.
func denseBuckets(rows []telemetry.Bucket, start, end time.Time, step time.Duration) ([]TimeseriesPoint, error) {
	count := int(end.Sub(start) / step)
	if count < 0 {
		count = 0
	}
	if count > maxSeriesPoints {
		count = maxSeriesPoints
	}

	points := make([]TimeseriesPoint, count)
	for i := range points {
		points[i] = TimeseriesPoint{Timestamp: start.Add(time.Duration(i) * step).UTC()}
	}
	for _, row := range rows {
		// Bucket bounds were derived in Postgres, which keeps timestamps to the
		// microsecond. The window start it worked from can therefore differ from
		// this Go-side `start` by up to 1µs, and every bucket it returns carries
		// that same sliver of error. Truncating the division would file the whole
		// series one bucket early — a silently mis-dated chart — so round to the
		// nearest bucket instead. The drift is orders of magnitude smaller than
		// any step this endpoint uses, so rounding is never ambiguous.
		idx := int(math.Round(float64(row.Start.Sub(start)) / float64(step)))
		if idx < 0 || idx >= count {
			continue
		}
		// Rounding is only safe because telemetry.Series anchors its buckets on
		// the `start` passed here, so the residual is that sub-microsecond Postgres
		// sliver and nothing else. The invariant is owned by another package and
		// rounding cannot express it — rounding a bucket a quarter-step off lands it
		// on a neighbour just as quietly as truncation did. Fail loudly instead: a
		// store that stopped aligning to the window is a broken fallback, not a chart
		// whose x-axis is a guess.
		drift := row.Start.Sub(start) - time.Duration(idx)*step
		if drift > maxBucketDrift || drift < -maxBucketDrift {
			return nil, fmt.Errorf("metrics bucket %s sits %s from grid point %d of the %s window starting %s: the records store must bucket from the window start",
				row.Start.UTC().Format(time.RFC3339Nano), drift, idx, step, start.UTC().Format(time.RFC3339Nano))
		}
		points[idx].Value += float64(row.Count)
	}
	return points, nil
}

func (s *Server) getAgentsMetrics(w http.ResponseWriter, r *http.Request) {
	const cacheKey = "agents"
	if cached, ok := s.observabilityCache.get(cacheKey); ok {
		writeJSON(w, http.StatusOK, cached)
		return
	}

	agents := map[string]*AgentMetric{}

	// Token consumption, broken down by component. Cache reads/writes are
	// counted separately by the runtime and must be included in the total.
	tokenResult, err := s.prom.Query(r.Context(), `sum by (agent, token_type) (agent_tokens_used_total)`)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to query token metrics: "+err.Error())
		return
	}
	for _, m := range tokenResult.Data {
		agent := m.Labels["agent"]
		if agent == "" {
			continue
		}
		entry := getOrCreateAgent(agents, agent)
		switch m.Labels["token_type"] {
		case tokenTypeInput:
			entry.InputTokens += m.Value
		case tokenTypeOutput:
			entry.OutputTokens += m.Value
		case tokenTypeCacheRead:
			entry.CacheReadTokens += m.Value
		case tokenTypeCacheWrite:
			entry.CacheWriteTokens += m.Value
		}
	}

	// Invocations
	invocationResult, err := s.prom.Query(r.Context(), `sum by (agent) (agent_invocations_total)`)
	if err == nil {
		for _, m := range invocationResult.Data {
			agent := m.Labels["agent"]
			if agent == "" {
				continue
			}
			entry := getOrCreateAgent(agents, agent)
			entry.Invocations = m.Value
		}
	}

	nameMap := s.agentNameMap(r.Context())
	out := make([]AgentMetric, 0, len(agents))
	for _, a := range agents {
		a.TotalTokens = tokenTotal(a.InputTokens, a.OutputTokens, a.CacheReadTokens, a.CacheWriteTokens)
		if name, ok := nameMap[a.Agent]; ok && name != "" {
			a.AgentName = name
		} else {
			a.AgentName = a.Agent
		}
		out = append(out, *a)
	}
	// Stable order so the frontend can diff cleanly between polls.
	sort.Slice(out, func(i, j int) bool { return out[i].Agent < out[j].Agent })

	resp := AgentsMetricsResponse{
		Agents:    out,
		UpdatedAt: time.Now().UTC(),
	}
	s.observabilityCache.set(cacheKey, resp)
	writeJSON(w, http.StatusOK, resp)
}

// agentNameMap returns a cached map from agent resource name to display name.
// It returns nil when the K8s client is unavailable so callers can skip
// enrichment gracefully.
func (s *Server) agentNameMap(ctx context.Context) map[string]string {
	const cacheKey = "agent-names"
	if cached, ok := s.observabilityCache.get(cacheKey); ok {
		if m, ok := cached.(map[string]string); ok {
			return m
		}
	}
	if s.client == nil {
		return nil
	}
	var list agentv1alpha1.AgentList
	if err := s.client.List(ctx, &list, client.InNamespace(s.ns)); err != nil {
		return nil
	}
	m := make(map[string]string, len(list.Items))
	for _, a := range list.Items {
		m[a.Name] = a.Spec.DisplayName
	}
	s.observabilityCache.set(cacheKey, m)
	return m
}

// findHubMetric returns the hubMetric matching name, or false.
func findHubMetric(name string) (hubMetric, bool) {
	for _, m := range hubMetrics {
		if m.Name == name {
			return m, true
		}
	}
	return hubMetric{}, false
}

// getOrCreateAgent returns the existing AgentMetric for agent or creates one.
func getOrCreateAgent(agents map[string]*AgentMetric, agent string) *AgentMetric {
	if e, ok := agents[agent]; ok {
		return e
	}
	e := &AgentMetric{Agent: agent}
	agents[agent] = e
	return e
}

// singleScalar runs an instant query expected to return a single scalar value
// and returns 0 (not an error) when the series is empty — this makes "no data
// yet" indistinguishable from "zero so far," which is the desired UX for a
// freshly-started hub with no events processed yet.
func singleScalar(ctx context.Context, p *prometheus.Client, promql string) (float64, error) {
	result, err := p.Query(ctx, promql)
	if err != nil {
		return 0, err
	}
	if len(result.Data) == 0 {
		return 0, nil
	}
	return result.Data[0].Value, nil
}
