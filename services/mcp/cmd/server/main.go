package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/DominikPinsel/ainsel/services/mcp/internal/auth"
	"github.com/DominikPinsel/ainsel/services/mcp/internal/config"
	mcpserver "github.com/DominikPinsel/ainsel/services/mcp/internal/mcp"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Load()

	if err := cfg.Validate(); err != nil {
		log.Error("config invalid", "error", err)
		os.Exit(1)
	}

	oidcMW, err := auth.NewMiddleware(auth.Config{
		Issuer:    cfg.OIDCIssuer,
		ProjectID: cfg.OIDCProjectID,
	})
	if err != nil {
		log.Error("auth middleware init", "error", err)
		os.Exit(1)
	}
	log.Info("OIDC auth enabled", "issuer", cfg.OIDCIssuer, "projectID", cfg.OIDCProjectID)

	mw := auth.UserTokenMiddleware(cfg.HubURL, cfg.InternalValidateSecret, oidcMW)
	if cfg.InternalValidateSecret != "" {
		log.Info("user token auth enabled")
	} else {
		log.Warn("INTERNAL_VALIDATE_SECRET not set, user token auth disabled")
	}

	s := mcpserver.New(log, mcpserver.Backends{
		HubURL: cfg.HubURL,
	})
	handler := mcpserver.StreamableHTTPHandler(s)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintln(w, `{"status":"ok"}`)
	})
	mux.Handle("/mcp", mw(handler))

	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Info("starting ainsel-mcp", "addr", addr)

	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Info("shutting down")
		_ = srv.Close()
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Error("server error", "err", err)
		os.Exit(1)
	}
}
