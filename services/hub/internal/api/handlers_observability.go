package api

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	agentv1alpha1 "github.com/DominikPinsel/ainsel/shared/api/api/v1alpha1"
	"github.com/DominikPinsel/ainsel/services/hub/internal/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// observabilityCacheTTL is the duration cached Prometheus query results stay fresh.
// 30s matches the Prometheus default scrape interval, so the dashboard only
// pays for one query per metric per scrape window even under heavy fan-in.
const observabilityCacheTTL = 30 * time.Second

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
type MetricsSummary struct {
	EventsConsumed  float64   `json:"eventsConsumed"`
	TriggersMatched float64   `json:"triggersMatched"`
	EventsRouted    float64   `json:"eventsRouted"`
	RoutingErrors   float64   `json:"routingErrors"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

// TimeseriesPoint is one (timestamp, value) pair.
type TimeseriesPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

// MetricsTimeseries is the response for the timeseries endpoint.
type MetricsTimeseries struct {
	Metric string            `json:"metric"`
	Range  string            `json:"range"`
	Step   string            `json:"step"`
	Points []TimeseriesPoint `json:"points"`
}

// AgentMetric is per-agent token consumption + invocation counts.
type AgentMetric struct {
	Agent        string  `json:"agent"`
	AgentName    string  `json:"agentName"`
	InputTokens  float64 `json:"inputTokens"`
	OutputTokens float64 `json:"outputTokens"`
	TotalTokens  float64 `json:"totalTokens"`
	Invocations  float64 `json:"invocations"`
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

// handleObservability dispatches to the three observability sub-handlers.
//
// The canonical paths live under /api/v1/observability/metrics/*. We also
// accept the legacy /api/v1/metrics/* prefix because the deployed frontend
// in ainsel-dev (pre-PR-50) calls those paths. Legacy responses are tagged
// with a Deprecation/Link header pair (RFC 8594 + RFC 8288) so frontends can
// detect the alias and migrate without us having to break them mid-flight.
func (s *Server) handleObservability(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/v1/observability/metrics/summary":
		s.observabilityMethodGate(w, r, s.getMetricsSummary)
	case "/api/v1/observability/metrics/timeseries":
		s.observabilityMethodGate(w, r, s.getMetricsTimeseries)
	case "/api/v1/observability/metrics/agents":
		s.observabilityMethodGate(w, r, s.getAgentsMetrics)
	case "/api/v1/observability/metrics/tokens/summary":
		s.observabilityMethodGate(w, r, s.getTokensSummary)
	case "/api/v1/observability/metrics/tokens/timeseries":
		s.observabilityMethodGate(w, r, s.getTokensTimeseries)
	case "/api/v1/observability/metrics/tokens/by-subject":
		s.observabilityMethodGate(w, r, s.getTokensBySubject)
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
		s.observabilityMethodGate(w, r, s.getAgentsMetrics)
	default:
		writeError(w, http.StatusNotFound, "not found")
	}
}

// handleObservabilityMetricsQuery serves GET /api/v1/observability/metrics/query.
// It is a thin raw proxy: it accepts ?query=<promql>&time=<optional rfc3339/unix>
// and forwards the request directly to Prometheus, returning the unmodified JSON
// response. This allows MCP tools and power users to run freeform PromQL without
// needing a direct Prometheus connection.
func (s *Server) handleObservabilityMetricsQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.prom == nil {
		writeError(w, http.StatusServiceUnavailable, "metrics backend not configured")
		return
	}
	query := r.URL.Query().Get("query")
	if query == "" {
		writeError(w, http.StatusBadRequest, "query is required")
		return
	}
	// Enforce namespace scoping: the query must contain a namespace label
	// matcher for the AInsel namespace to prevent cross-namespace metric
	// disclosure.
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
	if s.prom == nil {
		writeError(w, http.StatusServiceUnavailable, "metrics backend not configured")
		return
	}
	h(w, r)
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

	cacheKey := "summary"
	if rng != nil {
		cacheKey = "summary:" + rangeKey
	}
	if cached, ok := s.observabilityCache.get(cacheKey); ok {
		writeJSON(w, http.StatusOK, cached)
		return
	}

	summary := MetricsSummary{UpdatedAt: time.Now().UTC()}
	for _, m := range hubMetrics {
		query := m.PromQL
		if rng != nil {
			// Use increase() over the requested range so the card shows
			// the count within the window, not the all-time total.
			// m.PromQL is "sum(<counter>)" — extract the counter name.
			counter := strings.TrimSuffix(strings.TrimPrefix(m.PromQL, "sum("), ")")
			query = fmt.Sprintf("sum(increase(%s[%s]))", counter, rangeKey)
		}
		val, err := singleScalar(r.Context(), s.prom, query)
		if err != nil {
			writeError(w, http.StatusBadGateway, fmt.Sprintf("failed to query %s: %s", m.Name, err.Error()))
			return
		}
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

	s.observabilityCache.set(cacheKey, summary)
	writeJSON(w, http.StatusOK, summary)
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

	cacheKey := fmt.Sprintf("ts:%s:%s", metric.Name, rangeKey)
	if cached, ok := s.observabilityCache.get(cacheKey); ok {
		writeJSON(w, http.StatusOK, cached)
		return
	}

	end := time.Now().UTC()
	start := end.Add(-rng.Duration)

	// Prefer a rate query for counters so charts show throughput rather than the
	// monotonically-increasing raw counter.
	query := metric.PromQL
	if metric.RatePromQL != "" {
		query = fmt.Sprintf(metric.RatePromQL, rng.PromRange)
	}

	result, err := s.prom.QueryRange(r.Context(), query, start, end, rng.Step)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to query metrics: "+err.Error())
		return
	}

	points := make([]TimeseriesPoint, 0)
	if len(result.Series) > 0 {
		// We use sum(...) queries which collapse to a single series; if multiple
		// come back (e.g. caller passes a custom metric with labels) we just take
		// the first one to keep the wire format stable.
		for _, s := range result.Series[0].Samples {
			points = append(points, TimeseriesPoint{
				Timestamp: s.Timestamp.UTC(),
				Value:     s.Value,
			})
		}
	}

	resp := MetricsTimeseries{
		Metric: metric.Name,
		Range:  rangeKey,
		Step:   rng.Step.String(),
		Points: points,
	}
	s.observabilityCache.set(cacheKey, resp)
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) getAgentsMetrics(w http.ResponseWriter, r *http.Request) {
	const cacheKey = "agents"
	if cached, ok := s.observabilityCache.get(cacheKey); ok {
		writeJSON(w, http.StatusOK, cached)
		return
	}

	agents := map[string]*AgentMetric{}

	// Token consumption (input + output)
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
		case "input":
			entry.InputTokens = m.Value
		case "output":
			entry.OutputTokens = m.Value
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
		a.TotalTokens = a.InputTokens + a.OutputTokens
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
