// Command migrate-triggers is a one-time operational tool that reads existing
// Trigger and CronTrigger Custom Resources from the Kubernetes cluster and
// inserts them into the hub's Postgres database tables.
//
// This bridges the gap when upgrading from the CRD-based storage model to the
// database-based storage model.
//
// Usage:
//
//	HUB_DB_URL=postgres://user:pass@host:5432/dbname?sslmode=disable \
//	KUBECONFIG=/path/to/kubeconfig \
//	go run ./services/hub/cmd/migrate-triggers
//
// The tool is idempotent: if a trigger with the same ID already exists in the
// database (e.g. from a previous partial run), it is skipped.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/DominikPinsel/ainsel/services/hub/internal/triggers"
	ainselv1alpha1 "github.com/DominikPinsel/ainsel/shared/api/api/v1alpha1"
	"github.com/jackc/pgx/v5/pgxpool"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(ainselv1alpha1.AddToScheme(scheme))
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	dbURL := os.Getenv("HUB_DB_URL")
	if dbURL == "" {
		slog.Error("HUB_DB_URL is required")
		os.Exit(1)
	}

	namespace := os.Getenv("HUB_NAMESPACE")
	if namespace == "" {
		namespace = "ainsel"
	}

	// Connect to Postgres.
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	store := triggers.NewStore(pool)

	// Connect to Kubernetes.
	k8sClient, err := client.New(ctrl.GetConfigOrDie(), client.Options{Scheme: scheme})
	if err != nil {
		slog.Error("failed to create k8s client", "error", err)
		os.Exit(1)
	}

	migrated, skipped, errors := 0, 0, 0

	// --- Migrate Triggers ---
	var triggerList ainselv1alpha1.TriggerList
	if err := k8sClient.List(ctx, &triggerList, client.InNamespace(namespace)); err != nil {
		slog.Error("failed to list Trigger CRs", "namespace", namespace, "error", err)
		os.Exit(1)
	}

	slog.Info("found Trigger CRs", "count", len(triggerList.Items), "namespace", namespace)

	for i := range triggerList.Items {
		cr := &triggerList.Items[i]
		id := cr.Name

		// Check if already migrated (idempotent).
		if _, err := store.GetTrigger(ctx, id); err == nil {
			slog.Info("trigger already exists in DB, skipping", "id", id)
			skipped++
			continue
		}

		agentValid, connectorValid := extractTriggerValidity(cr.Status.Conditions)

		t := &triggers.Trigger{
			ID:             id,
			DisplayName:    cr.Spec.DisplayName,
			AgentRef:       cr.Spec.AgentRef,
			ConnectorRef:   cr.Spec.ConnectorRef,
			Filters:        cr.Spec.Filters,
			AgentValid:     agentValid,
			ConnectorValid: connectorValid,
		}

		if err := store.CreateTrigger(ctx, t); err != nil {
			slog.Error("failed to migrate trigger", "id", id, "error", err)
			errors++
			continue
		}
		slog.Info("migrated trigger", "id", id, "agentRef", t.AgentRef, "connectorRef", t.ConnectorRef)
		migrated++
	}

	// --- Migrate CronTriggers ---
	var cronList ainselv1alpha1.CronTriggerList
	if err := k8sClient.List(ctx, &cronList, client.InNamespace(namespace)); err != nil {
		slog.Error("failed to list CronTrigger CRs", "namespace", namespace, "error", err)
		os.Exit(1)
	}

	slog.Info("found CronTrigger CRs", "count", len(cronList.Items), "namespace", namespace)

	for i := range cronList.Items {
		cr := &cronList.Items[i]
		id := cr.Name

		// Check if already migrated (idempotent).
		if _, err := store.GetCronTrigger(ctx, id); err == nil {
			slog.Info("cron trigger already exists in DB, skipping", "id", id)
			skipped++
			continue
		}

		agentValid, scheduleValid := extractCronTriggerValidity(cr.Status.Conditions)
		enabled := true
		if cr.Spec.Enabled != nil {
			enabled = *cr.Spec.Enabled
		}

		var lastRun, nextRun *time.Time
		if cr.Status.LastRun != nil {
			t := cr.Status.LastRun.Time
			lastRun = &t
		}
		if cr.Status.NextRun != nil {
			t := cr.Status.NextRun.Time
			nextRun = &t
		}

		ct := &triggers.CronTrigger{
			ID:            id,
			DisplayName:   cr.Spec.DisplayName,
			AgentRef:      cr.Spec.AgentRef,
			Schedule:      cr.Spec.Schedule,
			Prompt:        cr.Spec.Prompt,
			Enabled:       enabled,
			AgentValid:    agentValid,
			ScheduleValid: scheduleValid,
			LastRun:       lastRun,
			NextRun:       nextRun,
		}

		if err := store.CreateCronTrigger(ctx, ct); err != nil {
			slog.Error("failed to migrate cron trigger", "id", id, "error", err)
			errors++
			continue
		}
		slog.Info("migrated cron trigger", "id", id, "agentRef", ct.AgentRef, "schedule", ct.Schedule)
		migrated++
	}

	slog.Info("migration complete", "migrated", migrated, "skipped", skipped, "errors", errors)
	if errors > 0 {
		os.Exit(1)
	}
}

// extractTriggerValidity reads the AgentRefValid and ConnectorRefValid
// conditions from the CRD status and returns the corresponding bools.
func extractTriggerValidity(conditions []metav1.Condition) (agentValid, connectorValid bool) {
	for _, c := range conditions {
		if c.Status == metav1.ConditionTrue {
			switch c.Type {
			case ainselv1alpha1.TriggerConditionAgentRefValid:
				agentValid = true
			case ainselv1alpha1.TriggerConditionConnectorRefValid:
				connectorValid = true
			}
		}
	}
	return
}

// extractCronTriggerValidity reads the AgentRefValid and ScheduleValid
// conditions from the CRD status and returns the corresponding bools.
func extractCronTriggerValidity(conditions []metav1.Condition) (agentValid, scheduleValid bool) {
	for _, c := range conditions {
		if c.Status == metav1.ConditionTrue {
			switch c.Type {
			case ainselv1alpha1.CronTriggerConditionAgentRefValid:
				agentValid = true
			case ainselv1alpha1.CronTriggerConditionScheduleValid:
				scheduleValid = true
			}
		}
	}
	return
}

// debugDump prints the JSON representation of a CR for troubleshooting.
// Unused in production but kept for reference during manual migration.
var _ = debugDump

func debugDump(v any) {
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Fprintln(os.Stderr, string(b))
}