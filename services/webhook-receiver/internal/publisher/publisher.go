package publisher

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	ainselapishared "github.com/DominikPinsel/ainsel/shared/api"
)

// HTTPPublisher publishes webhook events to the hub's ingest API.
type HTTPPublisher struct {
	hubURL string
	secret string
	client *http.Client
}

// New creates an HTTPPublisher that POSTs events to the hub.
func New(hubURL, internalSecret string) *HTTPPublisher {
	return &HTTPPublisher{
		hubURL: strings.TrimRight(hubURL, "/"),
		secret: internalSecret,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Publish builds an Event and POSTs it to the hub's ingest endpoint.
func (p *HTTPPublisher) Publish(logger *slog.Logger, connector string, headers map[string]string, body []byte) error {
	evt := ainselapishared.Event{
		ID:        fmt.Sprintf("evt_%s_%d", connector, time.Now().UnixNano()),
		Version:   "1",
		Connector: connector,
		Timestamp: time.Now().UTC(),
		Headers:   headers,
		Data:      ainselapishared.RawJSON(body),
		Raw:       string(body),
	}

	data, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	url := p.hubURL + "/api/internal/events"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if p.secret != "" {
		req.Header.Set("X-Internal-Token", p.secret)
	}

	logger.Debug("publishing to hub", "url", url, "event_id", evt.ID)
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("hub returned %d (expected 202)", resp.StatusCode)
	}

	return nil
}

// Close is a no-op for the HTTP publisher (kept for interface compatibility).
func (p *HTTPPublisher) Close() {}
