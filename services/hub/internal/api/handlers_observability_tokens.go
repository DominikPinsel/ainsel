package api

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/DominikPinsel/ainsel/services/hub/internal/invocations"
	"github.com/DominikPinsel/ainsel/services/hub/internal/prometheus"
	"github.com/DominikPinsel/ainsel/services/hub/internal/tasklogs"
)

// Agents publish token usage as agent_tokens_used_total{agent,repo,org,event_type,token_type,model}.
// The handlers in this file expose those metrics to the observability dashboard.
const (
	metricTokensUsed = "agent_tokens_used_total"

	// defaultSummaryRange is the range used when the caller does not supply one.
	// Kept at 24h for backward compatibility with the original "Tokens last 24h" tile.
	defaultSummaryRange = "24h"

	// summaryWindow is the duration form of defaultSummaryRange, used by the
	// tokens timeseries endpoint which still renders a fixed 24h sparkline.
	summaryWindow = 24 * time.Hour

	// timeseriesStep determines how many sparkline points the 24h tile renders.
	// 30 minutes → ~48 points across 24h. Matches the spec.
	timeseriesStep = 30 * time.Minute
)

// Values of the token_type label the agent runtime publishes. Pi reports
// prompt-cache traffic separately from billable input: usage.input excludes
// cacheRead/cacheWrite, and usage.totalTokens is the sum of all four. An agent
// loop with prompt caching enabled sends the large majority of its prompt as
// cache reads, so counting only input + output under-reports consumption by
// roughly an order of magnitude.
const (
	tokenTypeInput      = "input"
	tokenTypeOutput     = "output"
	tokenTypeCacheRead  = "cache_read"
	tokenTypeCacheWrite = "cache_write"
)

// tokenTotal sums every token component the runtime publishes. This is the one
// place the accounting rule lives — callers must not re-derive it, because
// omitting the cache components is exactly the bug this guards against.
func tokenTotal(input, output, cacheRead, cacheWrite float64) float64 {
	return input + output + cacheRead + cacheWrite
}

// tokenTotalsByType returns per-token_type totals for a window in a single
// grouped query. A component the runtime never published is absent from the
// map and reads as zero, which keeps "no data yet" and "zero so far"
// indistinguishable — the desired UX for a freshly-started hub.
func tokenTotalsByType(ctx context.Context, p *prometheus.Client, rangeKey string) (map[string]float64, error) {
	result, err := p.Query(ctx,
		fmt.Sprintf(`sum by (token_type) (increase(%s[%s]))`, metricTokensUsed, rangeKey))
	if err != nil {
		return nil, err
	}
	totals := make(map[string]float64, len(result.Data))
	for _, m := range result.Data {
		totals[m.Labels["token_type"]] += m.Value
	}
	return totals, nil
}

// TokensSummary powers the "Tokens last 24h" tile.
type TokensSummary struct {
	InputTokens         float64   `json:"inputTokens"`
	OutputTokens        float64   `json:"outputTokens"`
	CacheReadTokens     float64   `json:"cacheReadTokens"`
	CacheWriteTokens    float64   `json:"cacheWriteTokens"`
	TotalTokens         float64   `json:"totalTokens"`
	PreviousTotalTokens float64   `json:"previousTotalTokens"`
	UpdatedAt           time.Time `json:"updatedAt"`
}

// TokensTimeseries powers the sparkline embedded in the tile.
type TokensTimeseries struct {
	Range  string            `json:"range"`
	Step   string            `json:"step"`
	Points []TimeseriesPoint `json:"points"`
}

// TokenSubjectRow is one row in the Agent activity table — a unique
// (agent, repo, eventType, model) tuple with its tokens.
type TokenSubjectRow struct {
	Agent            string  `json:"agent"`
	AgentName        string  `json:"agentName"`
	Repo             string  `json:"repo"`
	EventType        string  `json:"eventType"`
	Model            string  `json:"model"`
	InputTokens      float64 `json:"inputTokens"`
	OutputTokens     float64 `json:"outputTokens"`
	CacheReadTokens  float64 `json:"cacheReadTokens"`
	CacheWriteTokens float64 `json:"cacheWriteTokens"`
	TotalTokens      float64 `json:"totalTokens"`
}

// TokensBySubject is the response for the /metrics/tokens/by-subject endpoint.
type TokensBySubject struct {
	Range     string            `json:"range"`
	Rows      []TokenSubjectRow `json:"rows"`
	UpdatedAt time.Time         `json:"updatedAt"`
}

// getTokensSummary aggregates token totals over a configurable window.
// Accepts an optional ?range= query parameter (1h|6h|24h|7d); defaults to 24h
// when omitted for backward compatibility.
func (s *Server) getTokensSummary(w http.ResponseWriter, r *http.Request) {
	rangeKey := r.URL.Query().Get("range")
	if rangeKey == "" {
		rangeKey = defaultSummaryRange
	}
	if _, ok := supportedRanges[rangeKey]; !ok {
		writeError(w, http.StatusBadRequest, "unsupported range; allowed values: 1h, 6h, 24h, 7d")
		return
	}

	cacheKey := "tokens-summary:" + rangeKey
	if cached, ok := s.observabilityCache.get(cacheKey); ok {
		writeJSON(w, http.StatusOK, cached)
		return
	}

	ctx := r.Context()

	totals, err := tokenTotalsByType(ctx, s.prom, rangeKey)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to query token metrics: "+err.Error())
		return
	}
	// The prior window is compared as a single total, so it needs no breakdown.
	prior, err := singleScalar(ctx, s.prom,
		fmt.Sprintf(`sum(increase(%s[%s] offset %s))`, metricTokensUsed, rangeKey, rangeKey))
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to query token metrics: "+err.Error())
		return
	}

	input := totals[tokenTypeInput]
	output := totals[tokenTypeOutput]
	cacheRead := totals[tokenTypeCacheRead]
	cacheWrite := totals[tokenTypeCacheWrite]

	resp := TokensSummary{
		InputTokens:         input,
		OutputTokens:        output,
		CacheReadTokens:     cacheRead,
		CacheWriteTokens:    cacheWrite,
		TotalTokens:         tokenTotal(input, output, cacheRead, cacheWrite),
		PreviousTotalTokens: prior,
		UpdatedAt:           time.Now().UTC(),
	}
	s.observabilityCache.set(cacheKey, resp)
	writeJSON(w, http.StatusOK, resp)
}

// getTokensTimeseries returns a 24h sparkline of total tokens consumed,
// stepped at 30-minute intervals.
func (s *Server) getTokensTimeseries(w http.ResponseWriter, r *http.Request) {
	rangeKey := r.URL.Query().Get("range")
	if rangeKey == "" {
		rangeKey = defaultSummaryRange
	}
	// Only 24h is supported initially — the tile is hardcoded to 24h on the UI side.
	// Accepting a parameter keeps the endpoint shape consistent with the rest of
	// /metrics/* and leaves room for 7d later without an API break.
	if rangeKey != defaultSummaryRange {
		writeError(w, http.StatusBadRequest, "unsupported range; only 24h is supported")
		return
	}

	cacheKey := "tokens-timeseries-" + rangeKey
	if cached, ok := s.observabilityCache.get(cacheKey); ok {
		writeJSON(w, http.StatusOK, cached)
		return
	}

	end := time.Now().UTC()
	start := end.Add(-summaryWindow)
	query := fmt.Sprintf(`sum(increase(%s[%s]))`, metricTokensUsed, "30m")

	result, err := s.prom.QueryRange(r.Context(), query, start, end, timeseriesStep)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to query token timeseries: "+err.Error())
		return
	}

	points := make([]TimeseriesPoint, 0)
	if len(result.Series) > 0 {
		for _, sample := range result.Series[0].Samples {
			points = append(points, TimeseriesPoint{
				Timestamp: sample.Timestamp.UTC(),
				Value:     sample.Value,
			})
		}
	}

	resp := TokensTimeseries{
		Range:  rangeKey,
		Step:   timeseriesStep.String(),
		Points: points,
	}
	s.observabilityCache.set(cacheKey, resp)
	writeJSON(w, http.StatusOK, resp)
}

// getTokensBySubject returns one row per (agent, repo, event_type, model) with
// input/output token totals. Honors the page-level range selector via ?range=.
func (s *Server) getTokensBySubject(w http.ResponseWriter, r *http.Request) {
	rangeKey := r.URL.Query().Get("range")
	if rangeKey == "" {
		rangeKey = defaultSummaryRange
	}
	if _, ok := supportedRanges[rangeKey]; !ok {
		writeError(w, http.StatusBadRequest, "unsupported range; allowed values: 1h, 6h, 24h, 7d")
		return
	}

	cacheKey := "tokens-by-subject-" + rangeKey
	if cached, ok := s.observabilityCache.get(cacheKey); ok {
		writeJSON(w, http.StatusOK, cached)
		return
	}

	ctx := r.Context()
	tokensQ := fmt.Sprintf(`sum by (agent, repo, event_type, model, token_type) (increase(%s[%s]))`, metricTokensUsed, rangeKey)
	tokensResult, err := s.prom.Query(ctx, tokensQ)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to query token metrics: "+err.Error())
		return
	}

	rows := map[subjectKey]*TokenSubjectRow{}
	for _, m := range tokensResult.Data {
		key := subjectKey{
			Agent:     m.Labels["agent"],
			Repo:      m.Labels["repo"],
			EventType: m.Labels["event_type"],
			Model:     m.Labels["model"],
		}
		// Skip rows where the agent label is missing — these are usually a
		// stale series or a malformed exporter and would render as a blank row.
		if key.Agent == "" {
			continue
		}
		row, ok := rows[key]
		if !ok {
			row = &TokenSubjectRow{Agent: key.Agent, Repo: key.Repo, EventType: key.EventType, Model: key.Model}
			rows[key] = row
		}
		switch m.Labels["token_type"] {
		case tokenTypeInput:
			row.InputTokens += m.Value
		case tokenTypeOutput:
			row.OutputTokens += m.Value
		case tokenTypeCacheRead:
			row.CacheReadTokens += m.Value
		case tokenTypeCacheWrite:
			row.CacheWriteTokens += m.Value
		}
	}

	nameMap := s.agentNameMap(ctx)
	out := make([]TokenSubjectRow, 0, len(rows))
	for _, row := range rows {
		row.TotalTokens = tokenTotal(row.InputTokens, row.OutputTokens, row.CacheReadTokens, row.CacheWriteTokens)
		if name, ok := nameMap[row.Agent]; ok && name != "" {
			row.AgentName = name
		} else {
			row.AgentName = row.Agent
		}
		out = append(out, *row)
	}
	// Stable order so polling on the frontend doesn't shuffle the table.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Repo != out[j].Repo {
			return out[i].Repo < out[j].Repo
		}
		if out[i].EventType != out[j].EventType {
			return out[i].EventType < out[j].EventType
		}
		return out[i].Agent < out[j].Agent
	})

	resp := TokensBySubject{
		Range:     rangeKey,
		Rows:      out,
		UpdatedAt: time.Now().UTC(),
	}
	s.observabilityCache.set(cacheKey, resp)
	writeJSON(w, http.StatusOK, resp)
}

// subjectKey identifies a row in the by-subject response.
type subjectKey struct {
	Agent     string
	Repo      string
	EventType string
	Model     string
}

// TokenEventRow is one row in the tokens-by-event response — a unique event
// with its aggregated token totals.
//
// Unlike the summary and by-subject rows, this view is backed by the
// task_conversations table rather than Prometheus, and that table only persists
// input/output. Its TotalTokens therefore still excludes prompt-cache traffic
// until cache columns are added by migration; see the token cost-tracking
// issue. Do not compare it against the summary tile.
type TokenEventRow struct {
	Event        string  `json:"event"`
	InputTokens  float64 `json:"inputTokens"`
	OutputTokens float64 `json:"outputTokens"`
	TotalTokens  float64 `json:"totalTokens"`
}

// TokensByEvent is the response for the /metrics/tokens/by-event endpoint.
type TokensByEvent struct {
	Range     string          `json:"range"`
	Rows      []TokenEventRow `json:"rows"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

// getTokensByEvent returns token totals grouped by event ID. It queries
// task_conversations grouped by invocation_id, then maps each invocation to
// its originating event via the in-memory invocations store.
func (s *Server) getTokensByEvent(w http.ResponseWriter, r *http.Request) {
	rangeKey := r.URL.Query().Get("range")
	if rangeKey == "" {
		rangeKey = defaultSummaryRange
	}
	if _, ok := supportedRanges[rangeKey]; !ok {
		writeError(w, http.StatusBadRequest, "unsupported range; allowed values: 1h, 6h, 24h, 7d")
		return
	}

	cacheKey := "tokens-by-event-" + rangeKey
	if cached, ok := s.observabilityCache.get(cacheKey); ok {
		writeJSON(w, http.StatusOK, cached)
		return
	}

	if s.taskLogs == nil {
		writeError(w, http.StatusServiceUnavailable, logStoreRequiredMessage)
		return
	}

	rng := supportedRanges[rangeKey]
	since := time.Now().UTC().Add(-rng.Duration)

	invRows, err := s.taskLogs.TokensByInvocationSince(r.Context(), since)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to query token data: "+err.Error())
		return
	}

	out := aggregateTokensByEvent(invRows, s.invocations)

	resp := TokensByEvent{
		Range:     rangeKey,
		Rows:      out,
		UpdatedAt: time.Now().UTC(),
	}
	s.observabilityCache.set(cacheKey, resp)
	writeJSON(w, http.StatusOK, resp)
}

// invocationGetter is the subset of invocations.Store used by
// aggregateTokensByEvent, allowing tests to supply a fake.
type invocationGetter interface {
	Get(id string) (invocations.Invocation, bool)
}

// aggregateTokensByEvent maps invocation-level token rows to event-level
// totals via the invocations store, then returns a stable-sorted slice.
func aggregateTokensByEvent(invRows []tasklogs.InvocationTokenRow, invStore invocationGetter) []TokenEventRow {
	eventTokens := map[string]*TokenEventRow{}
	for _, row := range invRows {
		inv, ok := invStore.Get(row.InvocationID)
		if !ok || inv.EventID == "" {
			continue
		}
		er, ok := eventTokens[inv.EventID]
		if !ok {
			er = &TokenEventRow{Event: inv.EventID}
			eventTokens[inv.EventID] = er
		}
		er.InputTokens += float64(row.InputTokens)
		er.OutputTokens += float64(row.OutputTokens)
	}

	out := make([]TokenEventRow, 0, len(eventTokens))
	for _, row := range eventTokens {
		row.TotalTokens = row.InputTokens + row.OutputTokens
		out = append(out, *row)
	}
	// Stable order by event ID so polling doesn't shuffle the table.
	sort.Slice(out, func(i, j int) bool { return out[i].Event < out[j].Event })
	return out
}
