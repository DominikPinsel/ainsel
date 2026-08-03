package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/DominikPinsel/ainsel/services/webhook-receiver/internal/publisher"
	"github.com/DominikPinsel/ainsel/services/webhook-receiver/internal/webhook"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	connectorName := requireEnv("CONNECTOR_NAME")
	secret := requireEnv("WEBHOOK_SECRET")
	signatureHeader := requireEnv("SIGNATURE_HEADER")
	hubURL := requireEnv("HUB_URL")
	internalSecret := os.Getenv("HUB_INTERNAL_SECRET")
	port := envOrDefault("CONNECTOR_PORT", "8080")

	slog.Info("starting webhook-receiver",
		"connector", connectorName,
		"signatureHeader", signatureHeader,
		"hubURL", hubURL,
		"port", port,
	)

	pub := publisher.New(hubURL, internalSecret)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/", webhook.New(connectorName, secret, signatureHeader, pub))

	slog.Info("listening", "port", port)
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		slog.Error("required env var not set", "key", key)
		os.Exit(1)
	}
	return v
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
