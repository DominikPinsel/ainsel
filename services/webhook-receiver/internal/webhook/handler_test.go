package webhook_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DominikPinsel/ainsel/services/webhook-receiver/internal/webhook"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
)

type fakePublisher struct {
	published []map[string]any
	err       error
}

func (f *fakePublisher) Publish(logger *slog.Logger, connector string, headers map[string]string, body []byte) error {
	var data map[string]any
	_ = json.Unmarshal(body, &data)
	f.published = append(f.published, map[string]any{
		"connector": connector,
		"headers":   headers,
		"data":      data,
	})
	return f.err
}

func sign(secret, sigHeader string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	_ = sigHeader
	return sig
}

func TestHandlerAcceptsValidSignature(t *testing.T) {
	secret := "test-secret"
	sigHeader := "X-Hub-Signature-256"
	pub := &fakePublisher{}
	h := webhook.New("my-connector", secret, sigHeader, pub)

	body := []byte(`{"action":"opened"}`)
	sig := sign(secret, sigHeader, body)

	req := httptest.NewRequest(http.MethodPost, "/publish", bytes.NewReader(body))
	req.Header.Set(sigHeader, sig)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if len(pub.published) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(pub.published))
	}
	if pub.published[0]["connector"] != "my-connector" {
		t.Fatalf("expected connector 'my-connector', got %v", pub.published[0]["connector"])
	}
}

func TestHandlerRejectsInvalidSignature(t *testing.T) {
	pub := &fakePublisher{}
	h := webhook.New("my-connector", "secret", "X-Hub-Signature-256", pub)

	body := []byte(`{"action":"opened"}`)
	req := httptest.NewRequest(http.MethodPost, "/publish", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", "sha256=badhash")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
	if len(pub.published) != 0 {
		t.Fatal("expected no published events on bad signature")
	}
}

func TestHandlerHealthz(t *testing.T) {
	pub := &fakePublisher{}
	h := webhook.New("conn", "secret", "X-Sig", pub)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHandlerPassesHeadersToPublisher(t *testing.T) {
	secret := "sec"
	sigHeader := "X-Forgejo-Signature"
	pub := &fakePublisher{}
	h := webhook.New("forgejo-conn", secret, sigHeader, pub)

	body := []byte(`{"issue":{"number":1}}`)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil)) // Forgejo sends without "sha256=" prefix

	req := httptest.NewRequest(http.MethodPost, "/publish", bytes.NewReader(body))
	req.Header.Set(sigHeader, sig)
	req.Header.Set("X-Forgejo-Event", "issues")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if len(pub.published) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(pub.published))
	}
	headers := pub.published[0]["headers"].(map[string]string)
	if headers["X-Forgejo-Event"] != "issues" {
		t.Fatalf("expected X-Forgejo-Event=issues in headers, got %v", headers)
	}
}

func TestHandlerEchoesUpstreamRequestID(t *testing.T) {
	secret := "test-secret"
	sigHeader := "X-Hub-Signature-256"
	pub := &fakePublisher{}
	h := webhook.New("my-connector", secret, sigHeader, pub)

	body := []byte(`{"action":"opened"}`)
	sig := sign(secret, sigHeader, body)

	req := httptest.NewRequest(http.MethodPost, "/publish", bytes.NewReader(body))
	req.Header.Set(sigHeader, sig)
	req.Header.Set("X-Request-Id", "upstream-req-123")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if got := rr.Header().Get("X-Request-Id"); got != "upstream-req-123" {
		t.Fatalf("expected X-Request-Id=upstream-req-123, got %q", got)
	}
}

func TestHandlerEchoesGitHubDeliveryID(t *testing.T) {
	secret := "test-secret"
	sigHeader := "X-Hub-Signature-256"
	pub := &fakePublisher{}
	h := webhook.New("my-connector", secret, sigHeader, pub)

	body := []byte(`{"action":"opened"}`)
	sig := sign(secret, sigHeader, body)

	req := httptest.NewRequest(http.MethodPost, "/publish", bytes.NewReader(body))
	req.Header.Set(sigHeader, sig)
	req.Header.Set("X-GitHub-Delivery", "gh-delivery-456")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if got := rr.Header().Get("X-Request-Id"); got != "gh-delivery-456" {
		t.Fatalf("expected X-Request-Id=gh-delivery-456, got %q", got)
	}
}

func TestHandlerMintsUUIDv4WhenNoUpstreamID(t *testing.T) {
	secret := "test-secret"
	sigHeader := "X-Hub-Signature-256"
	pub := &fakePublisher{}
	h := webhook.New("my-connector", secret, sigHeader, pub)

	body := []byte(`{"action":"opened"}`)
	sig := sign(secret, sigHeader, body)

	req := httptest.NewRequest(http.MethodPost, "/publish", bytes.NewReader(body))
	req.Header.Set(sigHeader, sig)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	got := rr.Header().Get("X-Request-Id")
	if got == "" {
		t.Fatal("expected non-empty X-Request-Id")
	}
	if _, err := uuid.Parse(got); err != nil {
		t.Fatalf("expected valid UUID v4, got %q: %v", got, err)
	}
}

func TestHandlerIncrementsOkCounter(t *testing.T) {
	secret := "test-secret"
	sigHeader := "X-Hub-Signature-256"
	pub := &fakePublisher{}
	h := webhook.New("ok-conn", secret, sigHeader, pub)

	body := []byte(`{"action":"opened"}`)
	sig := sign(secret, sigHeader, body)

	req := httptest.NewRequest(http.MethodPost, "/publish", bytes.NewReader(body))
	req.Header.Set(sigHeader, sig)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	count := testutil.ToFloat64(webhook.WebhookReceivedTotal.WithLabelValues("ok-conn", "ok"))
	if count != 1 {
		t.Fatalf("expected ok counter=1, got %v", count)
	}
}

func TestHandlerIncrementsInvalidSigCounter(t *testing.T) {
	pub := &fakePublisher{}
	h := webhook.New("badsig-conn", "secret", "X-Hub-Signature-256", pub)

	body := []byte(`{"action":"opened"}`)
	req := httptest.NewRequest(http.MethodPost, "/publish", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", "sha256=badhash")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	count := testutil.ToFloat64(webhook.WebhookReceivedTotal.WithLabelValues("badsig-conn", "invalid_sig"))
	if count != 1 {
		t.Fatalf("expected invalid_sig counter=1, got %v", count)
	}
}

func TestHandlerIncrementsPublishErrorCounter(t *testing.T) {
	secret := "test-secret"
	sigHeader := "X-Hub-Signature-256"
	pub := &fakePublisher{err: http.ErrAbortHandler}
	h := webhook.New("err-conn", secret, sigHeader, pub)

	body := []byte(`{"action":"opened"}`)
	sig := sign(secret, sigHeader, body)

	req := httptest.NewRequest(http.MethodPost, "/publish", bytes.NewReader(body))
	req.Header.Set(sigHeader, sig)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
	count := testutil.ToFloat64(webhook.WebhookReceivedTotal.WithLabelValues("err-conn", "publish_error"))
	if count != 1 {
		t.Fatalf("expected publish_error counter=1, got %v", count)
	}
}

func TestHandlerHistogramObservesPublishDuration(t *testing.T) {
	secret := "test-secret"
	sigHeader := "X-Hub-Signature-256"
	pub := &fakePublisher{}
	h := webhook.New("hist-conn", secret, sigHeader, pub)

	body := []byte(`{"action":"opened"}`)
	sig := sign(secret, sigHeader, body)

	req := httptest.NewRequest(http.MethodPost, "/publish", bytes.NewReader(body))
	req.Header.Set(sigHeader, sig)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	// Assert on the specific histogram child's SampleCount, not CollectAndCount
	// (which counts label combinations, not observations).
	hist, ok := webhook.WebhookPublishDuration.WithLabelValues("hist-conn").(prometheus.Histogram)
	if !ok {
		t.Fatal("expected prometheus.Histogram type")
	}
	var m dto.Metric
	if err := hist.Write(&m); err != nil {
		t.Fatalf("failed to read histogram metric: %v", err)
	}
	if m.Histogram == nil || m.Histogram.SampleCount == nil {
		t.Fatal("expected histogram with SampleCount")
	}
	if *m.Histogram.SampleCount == 0 {
		t.Fatal("expected histogram to have at least one observation")
	}
}

func TestTruncateSigNeverLeaksFullHMAC(t *testing.T) {
	// A full GitHub-style HMAC is "sha256=" + 64 hex chars = 71 chars.
	fullSig := "sha256=" + strings.Repeat("a", 64)
	trunc := webhook.TruncateSig(fullSig)
	if len(trunc) > 16 {
		t.Fatalf("truncated sig is %d chars, expected ≤ 16", len(trunc))
	}
	if trunc == fullSig {
		t.Fatal("truncated sig must not equal full signature")
	}
}

func TestTruncateSigShortString(t *testing.T) {
	short := "sha256=abc"
	trunc := webhook.TruncateSig(short)
	if trunc != short {
		t.Fatalf("expected short sig unchanged, got %q", trunc)
	}
}

func TestHandlerUnwrapsFormURLEncodedPayload(t *testing.T) {
	secret := "test-secret"
	sigHeader := "X-Hub-Signature-256"
	pub := &fakePublisher{}
	h := webhook.New("gh-conn", secret, sigHeader, pub)

	// GitHub-style form body: the JSON is URL-encoded in the "payload" field.
	formBody := []byte(`payload=%7B%22action%22%3A%22opened%22%2C%22issue%22%3A%7B%22number%22%3A42%7D%7D`)
	// Signature is computed over the raw form-encoded body.
	sig := sign(secret, sigHeader, formBody)

	req := httptest.NewRequest(http.MethodPost, "/publish", bytes.NewReader(formBody))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set(sigHeader, sig)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if len(pub.published) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(pub.published))
	}
	data, ok := pub.published[0]["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data to be a JSON object, got %T", pub.published[0]["data"])
	}
	if data["action"] != "opened" {
		t.Fatalf("expected data.action=opened, got %v", data["action"])
	}
	issue, ok := data["issue"].(map[string]any)
	if !ok {
		t.Fatalf("expected data.issue to be an object, got %T", data["issue"])
	}
	if issue["number"] != float64(42) {
		t.Fatalf("expected data.issue.number=42, got %v", issue["number"])
	}
}

func TestHandlerUnwrapsFormWithCharsetParam(t *testing.T) {
	secret := "test-secret"
	sigHeader := "X-Hub-Signature-256"
	pub := &fakePublisher{}
	h := webhook.New("gh-conn", secret, sigHeader, pub)

	formBody := []byte(`payload=%7B%22action%22%3A%22opened%22%7D`)
	sig := sign(secret, sigHeader, formBody)

	req := httptest.NewRequest(http.MethodPost, "/publish", bytes.NewReader(formBody))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=utf-8")
	req.Header.Set(sigHeader, sig)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if len(pub.published) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(pub.published))
	}
	data := pub.published[0]["data"].(map[string]any)
	if data["action"] != "opened" {
		t.Fatalf("expected data.action=opened, got %v", data["action"])
	}
}

func TestHandlerFormInvalidSignatureRejected(t *testing.T) {
	pub := &fakePublisher{}
	h := webhook.New("gh-conn", "secret", "X-Hub-Signature-256", pub)

	formBody := []byte(`payload=%7B%22action%22%3A%22opened%22%7D`)
	req := httptest.NewRequest(http.MethodPost, "/publish", bytes.NewReader(formBody))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Hub-Signature-256", "sha256=badhash")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
	if len(pub.published) != 0 {
		t.Fatal("expected no published events on bad signature")
	}
}

func TestHandlerFormMissingPayloadRejected(t *testing.T) {
	secret := "test-secret"
	sigHeader := "X-Hub-Signature-256"
	pub := &fakePublisher{}
	h := webhook.New("gh-conn", secret, sigHeader, pub)

	formBody := []byte(`foo=bar`)
	sig := sign(secret, sigHeader, formBody)

	req := httptest.NewRequest(http.MethodPost, "/publish", bytes.NewReader(formBody))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set(sigHeader, sig)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	if len(pub.published) != 0 {
		t.Fatal("expected no published events when payload missing")
	}
}

func TestHandlerFormNonJSONPayloadRejected(t *testing.T) {
	secret := "test-secret"
	sigHeader := "X-Hub-Signature-256"
	pub := &fakePublisher{}
	h := webhook.New("gh-conn", secret, sigHeader, pub)

	formBody := []byte(`payload=not-json`)
	sig := sign(secret, sigHeader, formBody)

	req := httptest.NewRequest(http.MethodPost, "/publish", bytes.NewReader(formBody))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set(sigHeader, sig)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	if len(pub.published) != 0 {
		t.Fatal("expected no published events when payload is not JSON")
	}
}
