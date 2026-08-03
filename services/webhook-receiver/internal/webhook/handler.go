package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
)

const maxBodySize = 1 << 20 // 1 MiB

const sigTruncLen = 16

// WebhookReceivedTotal counts webhook deliveries by connector and outcome.
// outcome ∈ {ok, invalid_sig, publish_error}.
var WebhookReceivedTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "webhook_received_total",
		Help: "Total webhooks received by connector and outcome.",
	},
	[]string{"connector", "outcome"},
)

// WebhookPublishDuration observes the publish step duration in seconds.
var WebhookPublishDuration = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "webhook_publish_duration_seconds",
		Help:    "Time spent publishing a webhook event to the message bus.",
		Buckets: prometheus.DefBuckets,
	},
	[]string{"connector"},
)

func init() {
	prometheus.MustRegister(WebhookReceivedTotal, WebhookPublishDuration)
}

// Publisher publishes a received webhook payload.
type Publisher interface {
	Publish(logger *slog.Logger, connector string, headers map[string]string, body []byte) error
}

// Handler is the HTTP handler for generic webhook events.
type Handler struct {
	connectorName   string
	secret          string
	signatureHeader string
	publisher       Publisher
}

// New creates a webhook handler.
func New(connectorName, secret, signatureHeader string, pub Publisher) *Handler {
	return &Handler{
		connectorName:   connectorName,
		secret:          secret,
		signatureHeader: signatureHeader,
		publisher:       pub,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/healthz":
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	case "/publish":
		h.handlePublish(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *Handler) handlePublish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Determine request ID: honour upstream headers, otherwise mint a UUID v4.
	requestID := r.Header.Get("X-Request-Id")
	if requestID == "" {
		requestID = r.Header.Get("X-GitHub-Delivery")
	}
	if requestID == "" {
		requestID = uuid.NewString()
	}
	// Stamp the request ID back on the response.
	w.Header().Set("X-Request-Id", requestID)

	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodySize))
	if err != nil {
		WebhookReceivedTotal.WithLabelValues(h.connectorName, "publish_error").Inc()
		http.Error(w, "failed to read body", http.StatusInternalServerError)
		return
	}

	sig := r.Header.Get(h.signatureHeader)
	sigTrunc := TruncateSig(sig)

	logger := slog.With(
		"request_id", requestID,
		"connector", h.connectorName,
		"body_size", len(body),
		"sig_trunc", sigTrunc,
	)

	if !h.validSignature(body, sig) {
		logger.Warn("webhook signature invalid", "header", h.signatureHeader)
		WebhookReceivedTotal.WithLabelValues(h.connectorName, "invalid_sig").Inc()
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// Some providers (e.g. GitHub) deliver with
	// Content-Type: application/x-www-form-urlencoded, wrapping the JSON in a
	// "payload" form field. Unwrap it to JSON so downstream trigger matching
	// sees top-level fields. Signature validation above already ran against the
	// original raw body, which is what the provider signs.
	body, err = unwrapFormPayload(r, body, logger)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Collect all request headers for downstream filter evaluation.
	headers := make(map[string]string, len(r.Header))
	for k := range r.Header {
		headers[k] = r.Header.Get(k)
	}

	start := time.Now()
	if err := h.publisher.Publish(logger, h.connectorName, headers, body); err != nil {
		elapsed := time.Since(start)
		logger.Error("publish failed",
			"log_type", "error_event",
			"severity", "error",
			"source", "connector",
			"error_message", err.Error(),
			"dur_ms", elapsed.Milliseconds(),
		)
		WebhookReceivedTotal.WithLabelValues(h.connectorName, "publish_error").Inc()
		WebhookPublishDuration.WithLabelValues(h.connectorName).Observe(elapsed.Seconds())
		http.Error(w, "publish failed", http.StatusInternalServerError)
		return
	}

	elapsed := time.Since(start)
	WebhookPublishDuration.WithLabelValues(h.connectorName).Observe(elapsed.Seconds())
	WebhookReceivedTotal.WithLabelValues(h.connectorName, "ok").Inc()
	logger.Info("webhook published", "dur_ms", elapsed.Milliseconds())
	w.WriteHeader(http.StatusOK)
}

// unwrapFormPayload converts an application/x-www-form-urlencoded body into the
// JSON carried by its "payload" field. For any other content type (including
// unset) the body is returned unchanged. The request body has already been
// drained by the caller, so the form is parsed from the read bytes via
// url.ParseQuery rather than r.ParseForm.
func unwrapFormPayload(r *http.Request, body []byte, logger *slog.Logger) ([]byte, error) {
	mediaType := ""
	if ct := r.Header.Get("Content-Type"); ct != "" {
		var err error
		mediaType, _, err = mime.ParseMediaType(ct)
		if err != nil {
			// Unparseable content type: fall through to treating the body as JSON.
			mediaType = ""
		}
	}
	if mediaType != "application/x-www-form-urlencoded" {
		return body, nil
	}

	values, err := url.ParseQuery(string(body))
	if err != nil {
		logger.Warn("failed to parse form body", "error_message", err.Error())
		return nil, errors.New("invalid form body")
	}
	payload := values.Get("payload")
	if payload == "" || !json.Valid([]byte(payload)) {
		logger.Warn("form body missing valid JSON payload")
		return nil, errors.New("missing or invalid payload field")
	}
	return []byte(payload), nil
}

// TruncateSig returns the first sigTruncLen characters of the signature header
// value (or the full string if shorter). This is enough to spot prefix/format
// mismatches without leaking the full HMAC.
func TruncateSig(sig string) string {
	if len(sig) <= sigTruncLen {
		return sig
	}
	return sig[:sigTruncLen]
}

// validSignature checks HMAC-SHA256. Accepts both raw hex and "sha256=<hex>" formats.
func (h *Handler) validSignature(body []byte, signature string) bool {
	mac := hmac.New(sha256.New, []byte(h.secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))

	// Strip "sha256=" prefix if present (GitHub format).
	actual := strings.TrimPrefix(signature, "sha256=")
	return hmac.Equal([]byte(expected), []byte(actual))
}
