package main

import (
	"context"
	"log/slog"
	"time"
)

// Skill delivery is retried on a loop rather than once at startup because
// the thing that makes it fail is size: the shared ConfigMap holds every
// enabled skill and the apiserver rejects the object past 1 MiB. A skill
// enabled while the object is full stays undelivered until something
// frees space, and nothing else re-writes those entries -- skill CRUD
// only touches the one skill being changed.
const (
	// skillsDeliveryIdle is the steady-state period. It keeps the
	// ConfigMap honest about skills enabled by an edited AgentImage CR,
	// which is a path the hub does not observe as a write.
	skillsDeliveryIdle = 10 * time.Minute
	// skillsDeliveryRetry is used while something is undelivered, so a
	// skill that starts fitting gets mounted without waiting out the
	// idle period.
	skillsDeliveryRetry = 30 * time.Second
)

// runSkillsDelivery converges the shared skills ConfigMap to the set of
// skills that at least one AgentImage enables, then keeps doing so.
//
// It runs once the informer cache is synced (the enabled-set comes from
// AgentImage CRs) and is non-fatal throughout: an agent whose skill
// cannot be delivered yet is a degraded agent, not a broken hub.
func runSkillsDelivery(ctx context.Context, c *container) error {
	if !c.mgr.GetCache().WaitForCacheSync(ctx) {
		return context.Canceled
	}

	// Every branch of the switch below sets this before it is read; the
	// first pass therefore runs unconditionally.
	var period time.Duration
	for {
		report, err := c.skillSvc.Converge(ctx)
		switch {
		case err != nil:
			slog.Error("skills: delivery reconcile failed", "error", err)
			period = skillsDeliveryRetry
		case len(report.Undelivered) > 0:
			slog.Warn("skills: delivery incomplete, will retry",
				"undelivered", len(report.Undelivered), "enabled", report.Enabled,
				"retry_in", skillsDeliveryRetry)
			period = skillsDeliveryRetry
		default:
			period = skillsDeliveryIdle
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(period):
		}
	}
}
