package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/DominikPinsel/ainsel/services/chat-mcp/internal/config"
	mcpserver "github.com/DominikPinsel/ainsel/services/chat-mcp/internal/mcp"
)

// main starts the chat MCP sidecar. The server exposes MCP tools that let
// the agent runtime interact with chat sessions by proxying to the hub
// backend's chat REST API.
//
// The sidecar is designed to run as a container in the agent pod, accessible
// via localhost. No authentication is required — the MCP endpoint is only
// reachable from within the pod.
func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Load()

	if err := cfg.Validate(); err != nil {
		log.Error("config invalid", "error", err)
		os.Exit(1)
	}

	s := mcpserver.New(log, cfg.HubURL, cfg.HubInternalToken, cfg.AgentName)
	handler := mcpserver.StreamableHTTPHandler(s)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintln(w, `{"status":"ok"}`)
	})
	mux.Handle("/mcp", handler)

	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Info("starting ainsel-chat-mcp",
		"addr", addr,
		"agent", cfg.AgentName,
		"hub_url", cfg.HubURL,
		"has_internal_token", cfg.HubInternalToken != "",
	)

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