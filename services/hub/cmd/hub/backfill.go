package main

import (
	"context"
	"fmt"
	"log/slog"

	ainselv1alpha1 "github.com/DominikPinsel/ainsel/shared/api/api/v1alpha1"

	"github.com/jackc/pgx/v5/pgxpool"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// resourceRef identifies a resource for group assignment backfill.
type resourceRef struct {
	Type string
	Name string
}

// runBackfill waits for cache sync, collects resource refs, and ensures
// every resource has a resource_groups entry in the "legacy" group.
// It is a one-shot operation; failures are logged but non-fatal.
func runBackfill(ctx context.Context, c *container) error {
	if !c.mgr.GetCache().WaitForCacheSync(ctx) {
		return fmt.Errorf("cache sync failed, skipping group backfill")
	}
	refs, err := collectResourceRefs(ctx, c.apiClient, c.namespace, c.pool)
	if err != nil {
		return fmt.Errorf("collect resource refs for backfill: %w", err)
	}

	created := 0
	skipped := 0
	for _, r := range refs {
		_, err := c.authzStore.GetResourceGroup(ctx, r.Type, r.Name)
		if err == nil {
			skipped++
			continue // Already has a group
		}
		if err := c.authzStore.SetResourceGroup(ctx, r.Type, r.Name, "legacy", false); err != nil {
			return fmt.Errorf("set resource group for %s/%s: %w", r.Type, r.Name, err)
		}
		created++
	}

	slog.Info("group backfill complete", "created", created, "skipped", skipped)
	return nil
}

// collectResourceRefs gathers all resource references from Kubernetes and the
// database for group backfill.
func collectResourceRefs(ctx context.Context, c client.Client, ns string, pool *pgxpool.Pool) ([]resourceRef, error) {
	var refs []resourceRef

	var agents ainselv1alpha1.AgentList
	if err := c.List(ctx, &agents, client.InNamespace(ns)); err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	for _, a := range agents.Items {
		refs = append(refs, resourceRef{Type: "agent", Name: a.Name})
	}

	// DB-based resources: triggers
	triggerRows, err := pool.Query(ctx, `SELECT id FROM triggers`)
	if err != nil {
		return nil, fmt.Errorf("list triggers: %w", err)
	}
	defer triggerRows.Close()
	for triggerRows.Next() {
		var id string
		if err := triggerRows.Scan(&id); err != nil {
			return nil, err
		}
		refs = append(refs, resourceRef{Type: "trigger", Name: id})
	}
	if err := triggerRows.Err(); err != nil {
		return nil, err
	}

	// DB-based resources: cron triggers
	cronTriggerRows, err := pool.Query(ctx, `SELECT id FROM cron_triggers`)
	if err != nil {
		return nil, fmt.Errorf("list cron triggers: %w", err)
	}
	defer cronTriggerRows.Close()
	for cronTriggerRows.Next() {
		var id string
		if err := cronTriggerRows.Scan(&id); err != nil {
			return nil, err
		}
		refs = append(refs, resourceRef{Type: "cron-trigger", Name: id})
	}
	if err := cronTriggerRows.Err(); err != nil {
		return nil, err
	}

	var wConnectors ainselv1alpha1.WebhookConnectorList
	if err := c.List(ctx, &wConnectors, client.InNamespace(ns)); err != nil {
		return nil, fmt.Errorf("list webhook connectors: %w", err)
	}
	for _, wc := range wConnectors.Items {
		refs = append(refs, resourceRef{Type: "connector", Name: wc.Name})
	}

	var images ainselv1alpha1.AgentImageList
	if err := c.List(ctx, &images, client.InNamespace(ns)); err != nil {
		return nil, fmt.Errorf("list agent images: %w", err)
	}
	for _, img := range images.Items {
		refs = append(refs, resourceRef{Type: "agent-image", Name: img.Name})
	}

	// DB-based resources: personas
	rows, err := pool.Query(ctx, `SELECT id FROM personas`)
	if err != nil {
		return nil, fmt.Errorf("list personas: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		refs = append(refs, resourceRef{Type: "persona", Name: id})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// DB-based resources: skills
	skillRows, err := pool.Query(ctx, `SELECT id FROM skills`)
	if err != nil {
		return nil, fmt.Errorf("list skills: %w", err)
	}
	defer skillRows.Close()
	for skillRows.Next() {
		var id string
		if err := skillRows.Scan(&id); err != nil {
			return nil, err
		}
		refs = append(refs, resourceRef{Type: "skill", Name: id})
	}

	// MCP servers
	mcpRows, err := pool.Query(ctx, `SELECT name FROM mcp_servers`)
	if err != nil {
		return nil, fmt.Errorf("list mcp servers: %w", err)
	}
	defer mcpRows.Close()
	for mcpRows.Next() {
		var name string
		if err := mcpRows.Scan(&name); err != nil {
			return nil, err
		}
		refs = append(refs, resourceRef{Type: "mcp-server", Name: name})
	}

	return refs, mcpRows.Err()
}
