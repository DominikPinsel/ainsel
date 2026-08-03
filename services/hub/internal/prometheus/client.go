package prometheus

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client queries a Prometheus instance.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// MetricEntry is a single result from a Prometheus vector query.
type MetricEntry struct {
	Labels map[string]string
	Value  float64
}

// QueryResult holds the entries returned by a Prometheus query.
type QueryResult struct {
	Data []MetricEntry
}

// SamplePoint is a single (timestamp, value) pair from a Prometheus matrix query.
type SamplePoint struct {
	Timestamp time.Time
	Value     float64
}

// MatrixSeries is a single labelled time series returned by a range query.
type MatrixSeries struct {
	Labels  map[string]string
	Samples []SamplePoint
}

// RangeResult holds the series returned by a Prometheus range query.
type RangeResult struct {
	Series []MatrixSeries
}

// QueryRaw executes an instant PromQL query and returns the raw Prometheus JSON
// response body unchanged. evalTime is an optional RFC3339 or Unix timestamp;
// empty means now.
func (c *Client) QueryRaw(ctx context.Context, promql, evalTime string) ([]byte, error) {
	params := url.Values{"query": {promql}}
	if evalTime != "" {
		params.Set("time", evalTime)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/api/v1/query?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("prometheus returned %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

// NewClient creates a Prometheus client. If httpClient is nil, a default with 10s timeout is used.
func NewClient(baseURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{baseURL: baseURL, httpClient: httpClient}
}

// Query executes an instant PromQL query and returns parsed vector results.
func (c *Client) Query(ctx context.Context, promql string) (QueryResult, error) {
	params := url.Values{}
	params.Set("query", promql)

	reqURL := fmt.Sprintf("%s/api/v1/query?%s", c.baseURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return QueryResult{}, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return QueryResult{}, fmt.Errorf("prometheus query: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return QueryResult{}, fmt.Errorf("prometheus returned status %d", resp.StatusCode)
	}

	var result queryResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return QueryResult{}, fmt.Errorf("decode response: %w", err)
	}

	entries := make([]MetricEntry, 0, len(result.Data.Result))
	for _, r := range result.Data.Result {
		if len(r.Value) < 2 {
			continue
		}
		valStr, ok := r.Value[1].(string)
		if !ok {
			continue
		}
		val, err := strconv.ParseFloat(valStr, 64)
		if err != nil {
			continue
		}
		entries = append(entries, MetricEntry{
			Labels: r.Metric,
			Value:  val,
		})
	}

	return QueryResult{Data: entries}, nil
}

// QueryRange executes a PromQL range query and returns parsed matrix results.
func (c *Client) QueryRange(ctx context.Context, promql string, start, end time.Time, step time.Duration) (RangeResult, error) {
	if step <= 0 {
		return RangeResult{}, fmt.Errorf("step must be positive")
	}
	if !end.After(start) {
		return RangeResult{}, fmt.Errorf("end must be after start")
	}

	params := url.Values{}
	params.Set("query", promql)
	params.Set("start", formatTime(start))
	params.Set("end", formatTime(end))
	params.Set("step", strconv.FormatFloat(step.Seconds(), 'f', -1, 64))

	reqURL := fmt.Sprintf("%s/api/v1/query_range?%s", c.baseURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return RangeResult{}, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return RangeResult{}, fmt.Errorf("prometheus query_range: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return RangeResult{}, fmt.Errorf("prometheus returned status %d", resp.StatusCode)
	}

	var result rangeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return RangeResult{}, fmt.Errorf("decode response: %w", err)
	}

	series := make([]MatrixSeries, 0, len(result.Data.Result))
	for _, r := range result.Data.Result {
		samples := make([]SamplePoint, 0, len(r.Values))
		for _, v := range r.Values {
			if len(v) < 2 {
				continue
			}
			tsFloat, ok := v[0].(float64)
			if !ok {
				continue
			}
			valStr, ok := v[1].(string)
			if !ok {
				continue
			}
			val, err := strconv.ParseFloat(valStr, 64)
			if err != nil {
				continue
			}
			samples = append(samples, SamplePoint{
				Timestamp: time.Unix(int64(tsFloat), int64((tsFloat-float64(int64(tsFloat)))*1e9)),
				Value:     val,
			})
		}
		series = append(series, MatrixSeries{
			Labels:  r.Metric,
			Samples: samples,
		})
	}

	return RangeResult{Series: series}, nil
}

// SanitizeLabel strips characters that could break PromQL label matchers.
func SanitizeLabel(v string) string {
	v = strings.ReplaceAll(v, `"`, "")
	v = strings.ReplaceAll(v, `}`, "")
	v = strings.ReplaceAll(v, `{`, "")
	return v
}

// formatTime renders a time as a Prometheus-friendly RFC3339 timestamp.
func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

// Prometheus API response types

type queryResponse struct {
	Status string    `json:"status"`
	Data   queryData `json:"data"`
}

type queryData struct {
	ResultType string         `json:"resultType"`
	Result     []vectorResult `json:"result"`
}

type vectorResult struct {
	Metric map[string]string `json:"metric"`
	Value  []interface{}     `json:"value"`
}

type rangeResponse struct {
	Status string    `json:"status"`
	Data   rangeData `json:"data"`
}

type rangeData struct {
	ResultType string         `json:"resultType"`
	Result     []matrixResult `json:"result"`
}

type matrixResult struct {
	Metric map[string]string `json:"metric"`
	Values [][]interface{}   `json:"values"`
}
