package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/DominikPinsel/ainsel/services/hub/internal/cron"
	"github.com/DominikPinsel/ainsel/services/hub/internal/trigger"
	"github.com/DominikPinsel/ainsel/services/hub/internal/triggers"
)

// runSyncLoop periodically syncs triggers and cron triggers from the database
// into the in-memory index and cron emitter. It performs an initial sync
// immediately, then every 30 seconds until the context is cancelled.
func runSyncLoop(ctx context.Context, store *triggers.Store, idx *trigger.Index, emitter *cron.Emitter) error {
	syncTriggersFromDB(ctx, store, idx, emitter)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			syncTriggersFromDB(ctx, store, idx, emitter)
		}
	}
}

// syncTriggersFromDB loads all triggers and cron triggers from the database
// and syncs them into the in-memory trigger index and cron emitter. This
// replaces the former Kubernetes informer-based approach.
func syncTriggersFromDB(ctx context.Context, store *triggers.Store, idx *trigger.Index, emitter *cron.Emitter) {
	allTriggers, err := store.ListTriggers(ctx, "", "")
	if err != nil {
		slog.Error("sync triggers from DB", "error", err)
	} else {
		seen := make(map[string]bool, len(allTriggers))
		for i := range allTriggers {
			t := &allTriggers[i]
			idx.Update(t)
			seen[t.ID] = true
		}
		// Remove triggers that no longer exist in DB.
		for _, k := range idx.Keys() {
			if !seen[k] {
				idx.Delete(k)
			}
		}
	}

	allCronTriggers, err := store.ListCronTriggers(ctx, "")
	if err != nil {
		slog.Error("sync cron triggers from DB", "error", err)
	} else {
		seen := make(map[string]bool, len(allCronTriggers))
		for i := range allCronTriggers {
			ct := &allCronTriggers[i]
			emitter.Upsert(ct)
			seen[ct.ID] = true
		}
		// Remove cron triggers that no longer exist in DB.
		for _, k := range emitter.Keys() {
			if !seen[k] {
				emitter.Delete(k)
			}
		}
	}
}