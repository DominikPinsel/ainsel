package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

type MetricsTools struct {
	HubURL     string
	HTTPClient *http.Client
}

func NewMetricsTools(hubURL string) *MetricsTools {
	return &MetricsTools{
		HubURL:     hubURL,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (m *MetricsTools) GetAgentMetricsTool() mcp.Tool {
	return mcp.NewTool("get_agent_metrics",
		mcp.WithDescription("Get key metrics for a specific agent: event processing times, event counts, error rates. Returns extracted scalar values by default; pass raw=true for full Prometheus envelopes."),
		mcp.WithString("agent", mcp.Required(), mcp.Description("Agent name")),
		mcp.WithBoolean("raw", mcp.Description("Return raw Prometheus response envelopes (default: false, extracted values)")),
	)
}

func (m *MetricsTools) QueryMetricsTool() mcp.Tool {
	return mcp.NewTool("query_metrics",
		mcp.WithDescription("Run a freeform PromQL query against Prometheus. Result capped at 20 series by default."),
		mcp.WithString("query", mcp.Required(), mcp.Description("PromQL query")),
		mcp.WithString("time", mcp.Description("Evaluation timestamp (RFC3339 or Unix). Default: now")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of result series to return (default: 20)")),
	)
}

func (m *MetricsTools) GetAgentMetrics(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, _ := req.Params.Arguments.(map[string]any)
	agent, ok := args["agent"].(string)
	if !ok || agent == "" {
		return mcp.NewToolResultError("agent name is required"), nil
	}
	raw, _ := args["raw"].(bool)

	queries := map[string]string{
		"events_total":           fmt.Sprintf(`sum(ainsel_agent_events_total{agent=%q})`, agent),
		"errors_total":           fmt.Sprintf(`sum(ainsel_agent_errors_total{agent=%q})`, agent),
		"processing_seconds_p99": fmt.Sprintf(`histogram_quantile(0.99, sum(rate(ainsel_agent_processing_seconds_bucket{agent=%q}[5m])) by (le))`, agent),
		"processing_seconds_avg": fmt.Sprintf(`sum(rate(ainsel_agent_processing_seconds_sum{agent=%q}[5m])) / sum(rate(ainsel_agent_processing_seconds_count{agent=%q}[5m]))`, agent, agent),
	}

	if raw {
		results := make(map[string]string)
		for name, query := range queries {
			body, err := m.promQueryRaw(ctx, query, "")
			if err != nil {
				results[name] = fmt.Sprintf("error: %v", err)
				continue
			}
			results[name] = string(body)
		}
		data, _ := json.MarshalIndent(results, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}

	// Extract scalar values from Prometheus responses.
	results := make(map[string]any)
	for name, query := range queries {
		body, err := m.promQueryRaw(ctx, query, "")
		if err != nil {
			results[name] = fmt.Sprintf("error: %v", err)
			continue
		}
		val := extractPromScalar(body)
		if val != "" {
			results[name] = val
		} else {
			results[name] = "no data"
		}
	}

	data, _ := json.MarshalIndent(results, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func (m *MetricsTools) QueryMetrics(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, _ := req.Params.Arguments.(map[string]any)
	query, ok := args["query"].(string)
	if !ok || query == "" {
		return mcp.NewToolResultError("query is required"), nil
	}
	evalTime, _ := args["time"].(string)
	limit := defaultArgInt(args, "limit", 20)

	body, err := m.promQueryRaw(ctx, query, evalTime)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("metrics query failed: %v", err)), nil
	}

	// Cap result series to limit.
	body = capPromResults(body, limit)
	return mcp.NewToolResultText(string(body)), nil
}

func (m *MetricsTools) promQueryRaw(ctx context.Context, query, evalTime string) ([]byte, error) {
	params := url.Values{"query": {query}}
	if evalTime != "" {
		params.Set("time", evalTime)
	}
	return hubGet(ctx, m.HTTPClient, m.HubURL, "/api/v1/observability/metrics/query?"+params.Encode())
}

// extractPromScalar attempts to extract a scalar value from a Prometheus
// JSON response. Returns the value as a string, or empty string if the
// response shape is unexpected.
func extractPromScalar(body []byte) string {
	var resp struct {
		Data struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Value []any `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return ""
	}
	if len(resp.Data.Result) > 0 && len(resp.Data.Result[0].Value) >= 2 {
		if s, ok := resp.Data.Result[0].Value[1].(string); ok {
			return s
		}
		// Could be a float64 directly.
		if f, ok := resp.Data.Result[0].Value[1].(float64); ok {
			return fmt.Sprintf("%g", f)
		}
	}
	return ""
}

// capPromResults caps the number of result series in a Prometheus response
// to maxSeries and adds truncated/hint metadata.
func capPromResults(body []byte, maxSeries int) []byte {
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		return body
	}
	data, ok := resp["data"].(map[string]any)
	if !ok {
		return body
	}
	results, ok := data["result"].([]any)
	if !ok {
		return body
	}
	total := len(results)
	if total > maxSeries {
		data["result"] = results[:maxSeries]
		data["truncated"] = true
		data["returned"] = maxSeries
		data["total"] = total
		data["hint"] = fmt.Sprintf("showing %d of %d series — pass limit=N for more", maxSeries, total)
	} else {
		data["truncated"] = false
		data["returned"] = total
	}
	out, err := json.Marshal(resp)
	if err != nil {
		return body
	}
	return out
}
