package api

import (
	"log/slog"
	"net/http"
	"slices"
	"sort"

	"github.com/DominikPinsel/ainsel/services/hub/internal/eventqueue"
	connectorv1alpha1 "github.com/DominikPinsel/ainsel/shared/api/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// telemetryScope limits what a caller may see on the activity and
// observability plane: events, invocations, agent logs and conversations.
//
// Those endpoints are cross-resource by nature — an activity feed or a log
// stream spans every agent — so the group checks that guard the resource CRUD
// routes have no single resource to attach to, and without a scope they
// returned every tenant's records to any authenticated caller. That included
// raw webhook payloads and the full agent/user dialogue.
//
// The rule mirrors the resource lists: a caller sees a record when they can
// read a resource the record belongs to. Admins, and deployments with no authz
// store wired, are unrestricted.
//
// Not yet scoped, deliberately: the Prometheus-backed aggregates
// (/api/v1/tokens, /api/v1/stats, /api/v1/observability/metrics/{summary,
// timeseries,agents,tokens/*}). Those return counts rather than records, so
// they disclose volume but no payload, message or conversation content.
// Scoping them means adding an agent label matcher to their PromQL, and the
// `agent` label on agent_tokens_used_total is consumed verbatim
// (getOrCreateAgent) without ever being resolved against agentNameMap. The
// examples in docs/api-reference.md show CR-style values such as "a-3f9a2b",
// which points at the CR name, but nothing in this repository verifies that
// against whatever emits the metric. Guessing wrong fails closed and silently
// blanks a dashboard, so confirm the label semantics before scoping these.
type telemetryScope struct {
	// unrestricted reports whether the caller may see every record. The
	// agents and connectors slices are meaningless when this is set.
	unrestricted bool
	// agents holds the agent CR names the caller may read.
	agents []string
	// connectors holds the connector CR names the caller may read.
	connectors []string
}

// telemetryScopeFor computes the caller's telemetry scope. Enumerating the
// resources costs a Kubernetes list per request (the agent map is cached), so
// callers should compute this once per request rather than per record.
//
// A failure to enumerate fails closed: an empty accessible set means the
// caller sees nothing rather than everything.
func (s *Server) telemetryScopeFor(r *http.Request) telemetryScope {
	if s.authzStore == nil || s.callerIsAdmin(r) {
		return telemetryScope{unrestricted: true}
	}

	agentNames := make([]string, 0, len(s.agentNameMap(r.Context())))
	for name := range s.agentNameMap(r.Context()) {
		agentNames = append(agentNames, name)
	}
	sort.Strings(agentNames)

	// nil (rather than empty) when the list fails, which filterByAccess maps
	// to "nothing accessible".
	var connectorNames []string
	if s.client != nil {
		var list connectorv1alpha1.WebhookConnectorList
		if err := s.client.List(r.Context(), &list, client.InNamespace(s.ns)); err == nil {
			connectorNames = make([]string, 0, len(list.Items))
			for _, c := range list.Items {
				connectorNames = append(connectorNames, c.Name)
			}
			sort.Strings(connectorNames)
		}
	}

	return telemetryScope{
		agents:     s.filterByAccess(r, "agent", agentNames),
		connectors: s.filterByAccess(r, "connector", connectorNames),
	}
}

// allowsAgent reports whether records belonging to the named agent are visible.
func (sc telemetryScope) allowsAgent(name string) bool {
	return sc.unrestricted || slices.Contains(sc.agents, name)
}

// allowsConnector reports whether records belonging to the named connector are
// visible.
func (sc telemetryScope) allowsConnector(name string) bool {
	return sc.unrestricted || slices.Contains(sc.connectors, name)
}

// isEmpty reports whether the scope admits no records at all, letting handlers
// short-circuit to an empty page instead of issuing a query that can only
// return nothing.
func (sc telemetryScope) isEmpty() bool {
	return !sc.unrestricted && len(sc.agents) == 0 && len(sc.connectors) == 0
}

// filterEventsByScope drops events the caller may not see, applying the same
// connector-or-routed-agent rule listEvents pushes into SQL.
//
// It exists for endpoints that read events through a path other than
// QueryEvents and so cannot scope in SQL. Prefer QueryEvents where the endpoint
// paginates: SQL-side scoping keeps the row query and the count query in
// agreement, which post-filtering cannot.
func (s *Server) filterEventsByScope(r *http.Request, scope telemetryScope, events []eventqueue.Event) []eventqueue.Event {
	if len(events) == 0 || scope.unrestricted {
		return events
	}

	// One batched lookup for the whole page rather than one per event.
	agentsByEvent := map[string][]string{}
	ids := make([]string, len(events))
	for i, e := range events {
		ids[i] = e.ID
	}
	if tasks, err := s.eventQueue.TasksForEvents(r.Context(), ids); err != nil {
		// Without routing information only the connector rule can be applied.
		// That under-reports rather than leaks, which is the right way to fail.
		slog.Warn("telemetry scope: could not resolve event routing, scoping by connector only",
			"error", err)
	} else {
		for _, t := range tasks {
			agentsByEvent[t.EventID] = append(agentsByEvent[t.EventID], t.AgentName)
		}
	}

	out := make([]eventqueue.Event, 0, len(events))
	for _, e := range events {
		if scope.allowsConnector(e.Connector) {
			out = append(out, e)
			continue
		}
		for _, agent := range agentsByEvent[e.ID] {
			if scope.allowsAgent(agent) {
				out = append(out, e)
				break
			}
		}
	}
	return out
}
