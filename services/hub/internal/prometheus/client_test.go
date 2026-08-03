package prometheus

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestQuery_ParsesVectorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		q := r.URL.Query().Get("query")
		if q == "" {
			t.Fatal("expected query parameter")
		}
		resp := map[string]interface{}{
			"status": "success",
			"data": map[string]interface{}{
				"resultType": "vector",
				"result": []map[string]interface{}{
					{
						"metric": map[string]string{
							"agent":        "dev-tester",
							"repository":   "dpinsel/aic",
							"issue_number": "42",
							"model":        "glm-5.1:cloud",
							"token_type":   "input",
						},
						"value": []interface{}{1715100000.0, "15000"},
					},
					{
						"metric": map[string]string{
							"agent":        "dev-tester",
							"repository":   "dpinsel/aic",
							"issue_number": "42",
							"model":        "glm-5.1:cloud",
							"token_type":   "output",
						},
						"value": []interface{}{1715100000.0, "3200"},
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewClient(server.URL, nil)
	result, err := c.Query(context.Background(), `sum by (agent) (agent_tokens_used_total)`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Data) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result.Data))
	}
	if result.Data[0].Labels["agent"] != "dev-tester" {
		t.Fatalf("expected agent=dev-tester, got %v", result.Data[0].Labels["agent"])
	}
	if result.Data[0].Value != 15000 {
		t.Fatalf("expected value=15000, got %f", result.Data[0].Value)
	}
}

func TestQuery_ReturnsEmptyOnNoResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"status": "success",
			"data": map[string]interface{}{
				"resultType": "vector",
				"result":     []interface{}{},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewClient(server.URL, nil)
	result, err := c.Query(context.Background(), `agent_tokens_used_total`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Data) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(result.Data))
	}
}

func TestQuery_ReturnsErrorOnNon200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	c := NewClient(server.URL, nil)
	_, err := c.Query(context.Background(), `invalid query`)
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
}

func TestQueryRange_ParsesMatrixResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query_range" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("query") == "" {
			t.Fatal("expected query parameter")
		}
		if q.Get("start") == "" || q.Get("end") == "" || q.Get("step") == "" {
			t.Fatal("expected start, end, step parameters")
		}
		resp := map[string]interface{}{
			"status": "success",
			"data": map[string]interface{}{
				"resultType": "matrix",
				"result": []map[string]interface{}{
					{
						"metric": map[string]string{"__name__": "hub_events_consumed_total"},
						"values": [][]interface{}{
							{1715100000.0, "5"},
							{1715100060.0, "12"},
							{1715100120.0, "20"},
						},
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewClient(server.URL, nil)
	end := time.Unix(1715100120, 0)
	start := end.Add(-2 * time.Minute)
	result, err := c.QueryRange(context.Background(), "hub_events_consumed_total", start, end, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Series) != 1 {
		t.Fatalf("expected 1 series, got %d", len(result.Series))
	}
	if len(result.Series[0].Samples) != 3 {
		t.Fatalf("expected 3 samples, got %d", len(result.Series[0].Samples))
	}
	if result.Series[0].Samples[2].Value != 20 {
		t.Errorf("expected last sample value=20, got %f", result.Series[0].Samples[2].Value)
	}
	if !result.Series[0].Samples[0].Timestamp.Equal(time.Unix(1715100000, 0)) {
		t.Errorf("unexpected first timestamp: %v", result.Series[0].Samples[0].Timestamp)
	}
}

func TestQueryRange_RejectsInvalidArgs(t *testing.T) {
	c := NewClient("http://example.invalid", nil)
	end := time.Now()
	if _, err := c.QueryRange(context.Background(), "x", end, end.Add(-time.Minute), time.Minute); err == nil {
		t.Error("expected error when end is before start")
	}
	if _, err := c.QueryRange(context.Background(), "x", end.Add(-time.Minute), end, 0); err == nil {
		t.Error("expected error for zero step")
	}
}
