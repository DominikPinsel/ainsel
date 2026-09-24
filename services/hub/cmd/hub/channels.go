package main

import (
	"context"
	"log/slog"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/DominikPinsel/ainsel/services/hub/internal/channels"
)

// channelSyncInterval is how often the channel registry is re-derived from the
// connector and agent registries. Channels are provisioned by those registries,
// so this is a repair loop, not the primary path: an event that arrives before
// the first sync still gets its channel, because ingest provisions on demand.
const channelSyncInterval = 30 * time.Second

// runChannelSyncLoop reconciles the channel registry from the Kubernetes
// registries: it waits for the informer cache, runs one pass, then repeats on a
// ticker. Errors are logged and retried rather than returned — a hub whose
// channel registry is briefly behind must keep routing events.
func runChannelSyncLoop(ctx context.Context, rec *channels.Reconciler, mgr manager.Manager) error {
	if rec == nil {
		return nil
	}
	if mgr != nil && !mgr.GetCache().WaitForCacheSync(ctx) {
		slog.Warn("channel sync: informer cache did not sync, continuing anyway")
	}

	syncChannels(ctx, rec)

	ticker := time.NewTicker(channelSyncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			syncChannels(ctx, rec)
		}
	}
}

// syncChannels runs one reconcile pass. The reconciler reports what it changed;
// a failure is logged and picked up on the next tick.
func syncChannels(ctx context.Context, rec *channels.Reconciler) {
	if _, err := rec.Sync(ctx); err != nil {
		slog.Error("channel sync failed", "error", err)
	}
}
