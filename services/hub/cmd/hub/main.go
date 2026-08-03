package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/DominikPinsel/ainsel/services/hub/internal/api"
	_ "github.com/DominikPinsel/ainsel/services/hub/internal/metrics"
	"github.com/DominikPinsel/ainsel/services/hub/internal/invocations"
	"github.com/DominikPinsel/ainsel/services/hub/internal/tasklogs"
	"github.com/DominikPinsel/ainsel/services/hub/pkg/version"
	ainselv1alpha1 "github.com/DominikPinsel/ainsel/shared/api/api/v1alpha1"

	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(ainselv1alpha1.AddToScheme(scheme))
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	slog.Info("starting hub", "version", version.Version, "commit", version.Commit)

	cfg := containerConfig{
		dbURL:                  envOrDefault("HUB_DB_URL", ""),
		namespace:              envOrDefault("HUB_NAMESPACE", "ainsel"),
		hubPort:                envOrDefault("HUB_PORT", "8080"),
		metricsPort:            envOrDefault("HUB_METRICS_PORT", "9090"),
		promURL:                envOrDefault("HUB_PROMETHEUS_URL", ""),
		invocationCapacity:     envIntOrDefault("HUB_INVOCATION_BUFFER_SIZE", invocations.DefaultCapacity),
		claimTimeoutSecs:       envIntOrDefault("TASK_CLAIM_TIMEOUT_SECONDS", 1800),
		connectorCfg:           api.LoadConnectorConfig(),
		hubAllowInsecureNoAuth: os.Getenv("HUB_ALLOW_INSECURE_NO_AUTH") == "true",
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	c, err := newContainer(ctx, cfg, containerDeps{})
	if err != nil {
		slog.Error("startup failed", "error", err)
		os.Exit(1)
	}
	defer c.Close()

	g, ctx := errgroup.WithContext(ctx)

	// Trigger sync loop (initial sync + 30s ticker).
	g.Go(runUntilCanceled(ctx, func(ctx context.Context) error {
		return runSyncLoop(ctx, c.triggerStore, c.idx, c.cronEmitter)
	}))

	// API HTTP server.
	g.Go(func() error {
		slog.Info("API server starting", "port", cfg.hubPort)
		if err := c.apiHTTPServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("API server: %w", err)
		}
		return nil
	})

	// Metrics HTTP server.
	g.Go(func() error {
		slog.Info("metrics server starting", "port", cfg.metricsPort)
		if err := c.metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("metrics server: %w", err)
		}
		return nil
	})

	// Webhook-driven router.
	g.Go(runUntilCanceled(ctx, func(ctx context.Context) error {
		return c.rtr.Run(ctx)
	}))

	// Cron emitter.
	g.Go(runUntilCanceled(ctx, func(ctx context.Context) error {
		return c.cronEmitter.Run(ctx)
	}))

	// K8s manager (cache + informers).
	g.Go(runUntilCanceled(ctx, func(ctx context.Context) error {
		slog.Info("starting manager for cache and informers")
		return c.mgr.Start(ctx)
	}))

	// Ownership backfill (one-shot after cache sync; non-fatal).
	g.Go(func() error {
		if err := runBackfill(ctx, c); err != nil {
			slog.Error("backfill", "error", err)
		}
		return nil
	})

	// Periodic task log pruning (hourly, non-fatal).
	g.Go(runUntilCanceled(ctx, func(ctx context.Context) error {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
				n, err := c.taskLogStore.Prune(ctx, tasklogs.DefaultRetention)
				if err != nil {
					slog.Error("task log prune failed", "error", err)
				} else if n > 0 {
					slog.Info("pruned old task log entries", "count", n)
				}
				cn, err := c.taskLogStore.PruneConversations(ctx, tasklogs.ConversationRetention)
				if err != nil {
					slog.Error("conversation prune failed", "error", err)
				} else if cn > 0 {
					slog.Info("pruned old conversation messages", "count", cn)
				}
			}
		}
	}))

	// Stale claim reaper (every 5m, non-fatal).
	g.Go(runUntilCanceled(ctx, func(ctx context.Context) error {
		claimTimeout := time.Duration(cfg.claimTimeoutSecs) * time.Second
		slog.Info("stale claim reaper started", "timeout", claimTimeout, "interval", 5*time.Minute)
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
				reaped, err := c.eventQueue.ReapStaleClaims(ctx, claimTimeout)
				if err != nil {
					slog.Error("stale claim reap failed", "error", err)
					continue
				}
				// Deduplicate agent names so we send at most one NOTIFY per agent.
				notified := make(map[string]bool)
				for _, rt := range reaped {
					slog.Warn("reaped stale claim", "task_id", rt.ID, "agent_name", rt.AgentName, "timeout", claimTimeout)
					if !notified[rt.AgentName] {
						notified[rt.AgentName] = true
						if err := c.eventQueue.NotifyAgent(ctx, rt.AgentName); err != nil {
							slog.Error("notify agent after reap failed", "agent_name", rt.AgentName, "error", err)
						}
					}
				}
			}
		}
	}))

	// Graceful shutdown: on ctx cancellation, shut down HTTP servers.
	// Returns the first non-nil shutdown error so g.Wait() surfaces it.
	g.Go(func() error {
		<-ctx.Done()
		slog.Info("shutting down")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := c.apiHTTPServer.Shutdown(shutdownCtx); err != nil {
			slog.Error("API server shutdown error", "error", err)
			return fmt.Errorf("API server shutdown: %w", err)
		}
		if err := c.metricsServer.Shutdown(shutdownCtx); err != nil {
			slog.Error("metrics server shutdown error", "error", err)
			return fmt.Errorf("metrics server shutdown: %w", err)
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		slog.Error("hub stopped with error", "error", err)
		os.Exit(1)
	}
	slog.Info("hub stopped")
}
