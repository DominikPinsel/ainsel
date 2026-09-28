package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/DominikPinsel/ainsel/services/hub/internal/authz"
	"github.com/DominikPinsel/ainsel/services/hub/internal/db"
	"github.com/DominikPinsel/ainsel/services/hub/internal/eventqueue"
	"github.com/DominikPinsel/ainsel/services/hub/internal/invocations"
	"github.com/DominikPinsel/ainsel/services/hub/internal/prometheus"
	"github.com/DominikPinsel/ainsel/services/hub/internal/tasklogs"
	connectorv1alpha1 "github.com/DominikPinsel/ainsel/shared/api/api/v1alpha1"
	"github.com/DominikPinsel/ainsel/shared/auth/oidc"
	pgcontainer "github.com/testcontainers/testcontainers-go/modules/postgres"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// The activity and observability plane used to be open to every authenticated
// caller: no route in it consulted the authz graph, because these endpoints are
// cross-resource and the group checks that guard the CRUD routes have no single
// resource to attach to. That exposed raw webhook payloads, agent task logs, the
// agent/user dialogue and invocation history across tenants.
//
// These tests pin down the scope that replaced it: a caller sees a record when
// they can read a resource it belongs to, admins see everything, and an
// explicit filter naming a resource the caller cannot read yields nothing
// rather than someone else's data.
//
// The authz graph is faked so the tests state the intended permissions directly;
// Postgres is real because the scoping is pushed into SQL (so that query and
// count agree and pagination stays truthful), and only a real database can show
// that the generated WHERE clauses work.

// telemetryAuthzFake models a small authz graph. It implements only what
// telemetryScopeFor and requireRead reach; the embedded nil interface makes any
// other call panic rather than silently returning a zero value that a test could
// mistake for "denied".
type telemetryAuthzFake struct {
	authzStore

	// admins maps a subject to its admin flag.
	admins map[string]bool
	// roles maps subject -> groupID -> role.
	roles map[string]map[string]authz.GroupRole
	// resources maps "resourceType/resourceName" -> groupID.
	resources map[string]string
	// public marks "resourceType/resourceName" as publicly readable.
	public map[string]bool
}

func resourceKey(resourceType, resourceName string) string {
	return resourceType + "/" + resourceName
}

func (f *telemetryAuthzFake) GetUser(_ context.Context, id string) (*authz.User, error) {
	return &authz.User{ID: id, Username: id, IsAdmin: f.admins[id]}, nil
}

func (f *telemetryAuthzFake) GetResourceGroup(_ context.Context, resourceType, resourceName string) (*authz.ResourceGroup, error) {
	key := resourceKey(resourceType, resourceName)
	groupID, ok := f.resources[key]
	if !ok {
		return nil, authz.ErrNotFound
	}
	return &authz.ResourceGroup{
		ResourceType: resourceType,
		ResourceName: resourceName,
		GroupID:      groupID,
		Public:       f.public[key],
	}, nil
}

func (f *telemetryAuthzFake) UserGroupIDs(_ context.Context, sub string) ([]string, error) {
	var out []string
	for groupID := range f.roles[sub] {
		out = append(out, groupID)
	}
	sort.Strings(out)
	return out, nil
}

func (f *telemetryAuthzFake) UserGroupRoles(_ context.Context, sub string) (map[string]authz.GroupRole, error) {
	return f.roles[sub], nil
}

func (f *telemetryAuthzFake) ListResourcesByGroups(_ context.Context, resourceType string, groupIDs []string, includePublic bool) ([]string, error) {
	wanted := make(map[string]bool, len(groupIDs))
	for _, g := range groupIDs {
		wanted[g] = true
	}
	var out []string
	for key, groupID := range f.resources {
		rt, rn, found := strings.Cut(key, "/")
		if !found || rt != resourceType {
			continue
		}
		if wanted[groupID] || (includePublic && f.public[key]) {
			out = append(out, rn)
		}
	}
	sort.Strings(out)
	return out, nil
}

// newTelemetryAuthzFake builds the graph used throughout: alice may read
// a-alice and c-alice, bob may read a-bob and c-bob, and neither may read the
// other's resources.
func newTelemetryAuthzFake(admins ...string) *telemetryAuthzFake {
	f := &telemetryAuthzFake{
		admins:    map[string]bool{},
		roles:     map[string]map[string]authz.GroupRole{},
		resources: map[string]string{},
		public:    map[string]bool{},
	}
	for _, a := range admins {
		f.admins[a] = true
	}
	for _, who := range []string{"alice", "bob"} {
		f.roles[who] = map[string]authz.GroupRole{"g-" + who: authz.RoleReader}
		f.resources[resourceKey("agent", "a-"+who)] = "g-" + who
		f.resources[resourceKey("connector", "c-"+who)] = "g-" + who
	}
	return f
}

type telemetryEnv struct {
	s           *Server
	taskLogs    *tasklogs.Store
	events      *eventqueue.Store
	invocations invocations.Store
	fake        *telemetryAuthzFake
	promHits    *int
}

// newTelemetryEnv boots Postgres, seeds two tenants' worth of telemetry, and
// wires a Server whose authz graph is the fake above. Skips when Docker is
// unavailable.
func newTelemetryEnv(t *testing.T, admins ...string) *telemetryEnv {
	t.Helper()
	ctx := context.Background()

	c, err := pgcontainer.Run(ctx, "postgres:17-alpine",
		pgcontainer.WithDatabase("ainsel_test"),
		pgcontainer.WithUsername("test"),
		pgcontainer.WithPassword("test"),
		pgcontainer.BasicWaitStrategies(),
	)
	if err != nil {
		t.Skipf("docker unavailable: %v", err)
	}
	dsn, err := c.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = c.Terminate(ctx)
		t.Fatalf("connection string: %v", err)
	}
	if err := db.Migrate(ctx, dsn); err != nil {
		_ = c.Terminate(ctx)
		t.Fatalf("migrate: %v", err)
	}
	pool, err := db.Open(ctx, dsn)
	if err != nil {
		_ = c.Terminate(ctx)
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		_ = c.Terminate(context.Background())
	})

	agents := []runtime.Object{}
	for _, name := range []string{"a-alice", "a-bob"} {
		agents = append(agents, &connectorv1alpha1.Agent{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "test-ns"},
			Spec:       connectorv1alpha1.AgentSpec{DisplayName: strings.TrimPrefix(name, "a-")},
		})
	}
	for _, name := range []string{"c-alice", "c-bob"} {
		agents = append(agents, &connectorv1alpha1.WebhookConnector{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "test-ns"},
		})
	}

	fake := newTelemetryAuthzFake(admins...)
	s := testServer(t, agents...)
	s.authzStore = fake
	checker := authz.NewChecker(fake, authz.NewGroupCache(
		func(userID string) (map[string]authz.GroupRole, error) {
			return fake.UserGroupRoles(context.Background(), userID)
		}, time.Minute))
	s.authzChecker = checker
	s.SetAuthMiddleware(func(next http.Handler) http.Handler { return next })

	taskLogStore := tasklogs.NewStore(pool)
	eventStore := eventqueue.NewStore(pool)
	invStore := invocations.NewMemoryStore(100)
	s.taskLogs = taskLogStore
	s.eventQueue = eventStore
	s.invocations = invStore

	s.mux.HandleFunc("/api/v1/observability/logs", s.handleObservabilityLogs)
	s.mux.HandleFunc("/api/v1/observability/conversations", s.handleConversations)
	s.mux.HandleFunc("/api/v1/events", s.handleEvents)
	s.mux.HandleFunc("/api/v1/events/", s.handleEvent)
	s.mux.HandleFunc("/api/v1/invocations", s.handleInvocations)
	s.mux.HandleFunc("/api/v1/invocations/", s.handleInvocation)
	s.mux.HandleFunc("/api/v1/errors", s.handleErrors)
	s.mux.HandleFunc("/api/v1/queue/recent", s.handleQueueRecent)

	// A stand-in Prometheus that records whether it was reached at all, so the
	// admin gate on the raw query proxy can be asserted independently of
	// whether a query would have succeeded.
	hits := 0
	promSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	}))
	t.Cleanup(promSrv.Close)
	s.prom = prometheus.NewClient(promSrv.URL, nil)
	s.mux.HandleFunc("/api/v1/observability/metrics/query", s.handleObservabilityMetricsQuery)

	env := &telemetryEnv{
		s:           s,
		taskLogs:    taskLogStore,
		events:      eventStore,
		invocations: invStore,
		fake:        fake,
		promHits:    &hits,
	}
	env.seed(t)
	return env
}

// seed writes one log entry, one conversation message, one event and one
// invocation per tenant, each carrying text that identifies its owner so a leak
// is unmistakable in a failure message.
func (e *telemetryEnv) seed(t *testing.T) {
	t.Helper()
	ctx := context.Background()

	for _, who := range []string{"alice", "bob"} {
		agent, connector := "a-"+who, "c-"+who

		if err := e.taskLogs.Insert(ctx, &tasklogs.Entry{
			AgentName: agent,
			Level:     "info",
			Message:   who + " secret log line",
		}); err != nil {
			t.Fatalf("insert log for %s: %v", who, err)
		}
		// An error-level entry too: /api/v1/errors reads the same table and its
		// messages routinely quote the payload that failed.
		if err := e.taskLogs.Insert(ctx, &tasklogs.Entry{
			AgentName: agent,
			Level:     tasklogs.LevelError,
			Message:   who + " secret error detail",
		}); err != nil {
			t.Fatalf("insert error log for %s: %v", who, err)
		}
		if err := e.taskLogs.InsertConversation(ctx, &tasklogs.ConversationMessage{
			AgentName: agent,
			Role:      "user",
			Content:   who + " private conversation",
		}); err != nil {
			t.Fatalf("insert conversation for %s: %v", who, err)
		}

		eventID := "evt-" + who
		payload, _ := json.Marshal(map[string]string{"secret": who + " webhook payload"})
		if err := e.events.InsertEvent(ctx, eventqueue.Event{
			ID:         eventID,
			Connector:  connector,
			Headers:    json.RawMessage(`{"type":"push"}`),
			Data:       payload,
			Raw:        string(payload),
			ReceivedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatalf("insert event for %s: %v", who, err)
		}
		if err := e.events.EnqueueTask(ctx, eventqueue.Task{
			EventID:     eventID,
			AgentName:   agent,
			TriggerName: "t-" + who,
			Headers:     json.RawMessage(`{"type":"push"}`),
			Payload:     payload,
			Status:      "queued",
			CreatedAt:   time.Now().UTC(),
		}); err != nil {
			t.Fatalf("enqueue task for %s: %v", who, err)
		}

		e.invocations.Record(invocations.Invocation{
			AgentName:   agent,
			TriggerName: "t-" + who,
			EventID:     eventID,
		})
	}
}

// get issues an authenticated GET as sub ("" for no identity).
func (e *telemetryEnv) get(path, sub string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if sub != "" {
		req = req.WithContext(oidc.ContextWithUser(req.Context(), &oidc.User{Sub: sub, Username: sub}))
	}
	rec := httptest.NewRecorder()
	e.s.mux.ServeHTTP(rec, req)
	return rec
}

// assertNoLeak fails when the response body mentions the other tenant's marker.
func assertNoLeak(t *testing.T, rec *httptest.ResponseRecorder, path, sub string) {
	t.Helper()
	other := "bob"
	if sub == "bob" {
		other = "alice"
	}
	if body := rec.Body.String(); strings.Contains(body, other) {
		t.Errorf("%s as %q leaked %s's data: %s", path, sub, other, body)
	}
}

func TestTelemetryLogsScopedToAccessibleAgents(t *testing.T) {
	env := newTelemetryEnv(t)

	t.Run("unfiltered list is restricted", func(t *testing.T) {
		rec := env.get("/api/v1/observability/logs", "alice")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d (%s), want 200", rec.Code, rec.Body.String())
		}
		var resp LogsResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		// Two entries per agent were seeded (one info, one error).
		if len(resp.Logs) != 2 {
			t.Errorf("got %d log lines, want 2 (alice's only): %s", len(resp.Logs), rec.Body.String())
		}
		assertNoLeak(t, rec, "/observability/logs", "alice")
	})

	t.Run("explicit app requires read access", func(t *testing.T) {
		if rec := env.get("/api/v1/observability/logs?app=a-alice", "alice"); rec.Code != http.StatusOK {
			t.Errorf("own agent: status = %d (%s), want 200", rec.Code, rec.Body.String())
		}
		rec := env.get("/api/v1/observability/logs?app=a-bob", "alice")
		if rec.Code != http.StatusForbidden {
			t.Errorf("foreign agent: status = %d (%s), want 403", rec.Code, rec.Body.String())
		}
		assertNoLeak(t, rec, "/observability/logs?app=a-bob", "alice")
	})

	t.Run("admin sees both tenants", func(t *testing.T) {
		adminEnv := newTelemetryEnv(t, "root")
		rec := adminEnv.get("/api/v1/observability/logs", "root")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d (%s), want 200", rec.Code, rec.Body.String())
		}
		var resp LogsResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(resp.Logs) != 4 {
			t.Errorf("admin got %d log lines, want 4", len(resp.Logs))
		}
	})
}

// TestTelemetryErrorsScoped covers /api/v1/errors, which reads the same
// task_logs table as the log stream.
func TestTelemetryErrorsScoped(t *testing.T) {
	env := newTelemetryEnv(t)

	rec := env.get("/api/v1/errors", "alice")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s), want 200", rec.Code, rec.Body.String())
	}
	var resp struct {
		Errors []map[string]any `json:"errors"`
		Total  int              `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Errors) != 1 {
		t.Errorf("got %d error entries, want 1 (alice's only): %s", len(resp.Errors), rec.Body.String())
	}
	assertNoLeak(t, rec, "/errors", "alice")

	if rec := env.get("/api/v1/errors?agent=a-bob", "alice"); rec.Code != http.StatusForbidden {
		t.Errorf("foreign agent: status = %d (%s), want 403", rec.Code, rec.Body.String())
	}
}

// TestTelemetryQueueRecentScoped covers /api/v1/queue/recent, which returns raw
// eventqueue.Event rows — headers, data and raw body included.
func TestTelemetryQueueRecentScoped(t *testing.T) {
	env := newTelemetryEnv(t)

	t.Run("unfiltered list is restricted", func(t *testing.T) {
		rec := env.get("/api/v1/queue/recent", "alice")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d (%s), want 200", rec.Code, rec.Body.String())
		}
		var events []eventqueue.Event
		if err := json.Unmarshal(rec.Body.Bytes(), &events); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(events) != 1 {
			t.Errorf("got %d events, want 1 (alice's only): %s", len(events), rec.Body.String())
		}
		assertNoLeak(t, rec, "/queue/recent", "alice")
	})

	t.Run("explicit connector cannot widen", func(t *testing.T) {
		rec := env.get("/api/v1/queue/recent?connector=c-bob", "alice")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d (%s), want 200", rec.Code, rec.Body.String())
		}
		var events []eventqueue.Event
		if err := json.Unmarshal(rec.Body.Bytes(), &events); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(events) != 0 {
			t.Errorf("got %d events for a foreign connector, want 0", len(events))
		}
		assertNoLeak(t, rec, "/queue/recent?connector=c-bob", "alice")
	})

	t.Run("admin sees both tenants", func(t *testing.T) {
		adminEnv := newTelemetryEnv(t, "root")
		rec := adminEnv.get("/api/v1/queue/recent", "root")
		var events []eventqueue.Event
		if err := json.Unmarshal(rec.Body.Bytes(), &events); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(events) != 2 {
			t.Errorf("admin got %d events, want 2", len(events))
		}
	})
}

func TestTelemetryConversationsScopedToAccessibleAgents(t *testing.T) {
	env := newTelemetryEnv(t)

	rec := env.get("/api/v1/observability/conversations", "alice")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s), want 200", rec.Code, rec.Body.String())
	}
	var resp struct {
		Messages []tasklogs.ConversationMessage `json:"messages"`
		Total    int                            `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Messages) != 1 {
		t.Errorf("got %d messages, want 1 (alice's only): %s", len(resp.Messages), rec.Body.String())
	}
	assertNoLeak(t, rec, "/observability/conversations", "alice")

	if rec := env.get("/api/v1/observability/conversations?agent=a-bob", "alice"); rec.Code != http.StatusForbidden {
		t.Errorf("foreign agent: status = %d (%s), want 403", rec.Code, rec.Body.String())
	}
}

func TestTelemetryEventsScopedToAccessibleResources(t *testing.T) {
	env := newTelemetryEnv(t)

	t.Run("unfiltered list is restricted", func(t *testing.T) {
		rec := env.get("/api/v1/events", "alice")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d (%s), want 200", rec.Code, rec.Body.String())
		}
		var resp eventsEnvelope
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(resp.Events) != 1 {
			t.Errorf("got %d events, want 1 (alice's only): %s", len(resp.Events), rec.Body.String())
		}
		// total must agree with the filtered result, or the UI paginates past
		// the end and re-discovers the hidden rows.
		if resp.Total != len(resp.Events) {
			t.Errorf("total = %d but %d events returned — count and query disagree", resp.Total, len(resp.Events))
		}
		assertNoLeak(t, rec, "/events", "alice")
	})

	t.Run("explicit filters cannot widen", func(t *testing.T) {
		for _, path := range []string{"/api/v1/events?connector=c-bob", "/api/v1/events?agent=a-bob"} {
			rec := env.get(path, "alice")
			if rec.Code != http.StatusOK {
				t.Fatalf("%s: status = %d (%s), want 200", path, rec.Code, rec.Body.String())
			}
			var resp eventsEnvelope
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("%s: decode: %v", path, err)
			}
			if len(resp.Events) != 0 {
				t.Errorf("%s returned %d events, want 0", path, len(resp.Events))
			}
			assertNoLeak(t, rec, path, "alice")
		}
	})

	t.Run("own filter still works", func(t *testing.T) {
		rec := env.get("/api/v1/events?connector=c-alice", "alice")
		var resp eventsEnvelope
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(resp.Events) != 1 {
			t.Errorf("got %d events for own connector, want 1", len(resp.Events))
		}
	})

	t.Run("single event is hidden, not merely forbidden", func(t *testing.T) {
		if rec := env.get("/api/v1/events/evt-bob", "alice"); rec.Code != http.StatusNotFound {
			t.Errorf("foreign event: status = %d (%s), want 404", rec.Code, rec.Body.String())
		}
		if rec := env.get("/api/v1/events/evt-alice", "alice"); rec.Code != http.StatusOK {
			t.Errorf("own event: status = %d (%s), want 200", rec.Code, rec.Body.String())
		}
	})
}

func TestTelemetryInvocationsScopedToAccessibleAgents(t *testing.T) {
	env := newTelemetryEnv(t)

	t.Run("unfiltered list is restricted", func(t *testing.T) {
		rec := env.get("/api/v1/invocations", "alice")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d (%s), want 200", rec.Code, rec.Body.String())
		}
		var resp struct {
			Invocations []invocations.Invocation `json:"invocations"`
			Total       int                      `json:"total"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(resp.Invocations) != 1 {
			t.Errorf("got %d invocations, want 1 (alice's only): %s", len(resp.Invocations), rec.Body.String())
		}
		assertNoLeak(t, rec, "/invocations", "alice")
	})

	t.Run("event enrichment does not reintroduce foreign agents", func(t *testing.T) {
		// enrichWithTasks synthesizes rows straight from agent_tasks, bypassing
		// the scoped store query, so it needs its own check.
		rec := env.get("/api/v1/invocations?event=evt-bob", "alice")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d (%s), want 200", rec.Code, rec.Body.String())
		}
		var resp struct {
			Invocations []invocations.Invocation `json:"invocations"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(resp.Invocations) != 0 {
			t.Errorf("got %d invocations for a foreign event, want 0: %s", len(resp.Invocations), rec.Body.String())
		}
		assertNoLeak(t, rec, "/invocations?event=evt-bob", "alice")
	})

	t.Run("single invocation requires read access", func(t *testing.T) {
		var listResp struct {
			Invocations []invocations.Invocation `json:"invocations"`
		}
		rec := env.get("/api/v1/invocations", "alice")
		if err := json.Unmarshal(rec.Body.Bytes(), &listResp); err != nil {
			t.Fatalf("decode list: %v", err)
		}
		if len(listResp.Invocations) != 1 {
			t.Fatalf("expected 1 invocation for alice, got %d", len(listResp.Invocations))
		}
		own := listResp.Invocations[0].ID
		if rec := env.get("/api/v1/invocations/"+own, "alice"); rec.Code != http.StatusOK {
			t.Errorf("own invocation: status = %d (%s), want 200", rec.Code, rec.Body.String())
		}

		// bob's invocation id, discovered by asking as bob.
		bobRec := env.get("/api/v1/invocations", "bob")
		var bobResp struct {
			Invocations []invocations.Invocation `json:"invocations"`
		}
		if err := json.Unmarshal(bobRec.Body.Bytes(), &bobResp); err != nil {
			t.Fatalf("decode bob list: %v", err)
		}
		if len(bobResp.Invocations) != 1 {
			t.Fatalf("expected 1 invocation for bob, got %d", len(bobResp.Invocations))
		}
		foreign := bobResp.Invocations[0].ID
		if rec := env.get("/api/v1/invocations/"+foreign, "alice"); rec.Code != http.StatusForbidden {
			t.Errorf("foreign invocation: status = %d (%s), want 403", rec.Code, foreign)
		}
	})
}

// TestMetricsRawQueryRequiresAdmin covers the freeform PromQL proxy. The
// namespace check it relied on is a substring test, so an expression such as
// `up{namespace="ainsel"} or up` satisfies it while still selecting unscoped
// series; the gate is now who may call it at all.
func TestMetricsRawQueryRequiresAdmin(t *testing.T) {
	const q = `/api/v1/observability/metrics/query?query=up{namespace="test-ns"}`

	t.Run("non-admin is refused and Prometheus is never reached", func(t *testing.T) {
		env := newTelemetryEnv(t)
		before := *env.promHits
		if rec := env.get(q, "alice"); rec.Code != http.StatusForbidden {
			t.Errorf("status = %d (%s), want 403", rec.Code, rec.Body.String())
		}
		if *env.promHits != before {
			t.Errorf("Prometheus was queried %d time(s) for a non-admin", *env.promHits-before)
		}
	})

	t.Run("admin is allowed through", func(t *testing.T) {
		env := newTelemetryEnv(t, "root")
		before := *env.promHits
		if rec := env.get(q, "root"); rec.Code != http.StatusOK {
			t.Errorf("status = %d (%s), want 200", rec.Code, rec.Body.String())
		}
		if *env.promHits == before {
			t.Error("Prometheus was not queried for an admin")
		}
	})

	t.Run("unauthenticated is refused", func(t *testing.T) {
		env := newTelemetryEnv(t)
		if rec := env.get(q, ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d (%s), want 401", rec.Code, rec.Body.String())
		}
	})

	t.Run("dev mode without authz still works", func(t *testing.T) {
		// No authz wired means no notion of admin and an open API by design;
		// requireAdmin alone would fail closed here and break local runs.
		s := testServer(t)
		hits := 0
		promSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits++
			_, _ = w.Write([]byte(`{"status":"success"}`))
		}))
		defer promSrv.Close()
		s.prom = prometheus.NewClient(promSrv.URL, nil)
		s.mux.HandleFunc("/api/v1/observability/metrics/query", s.handleObservabilityMetricsQuery)

		req := httptest.NewRequest(http.MethodGet, q, nil)
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d (%s), want 200 in dev mode", rec.Code, rec.Body.String())
		}
		if hits == 0 {
			t.Error("Prometheus was not queried in dev mode")
		}
	})
}

// TestContainsNamespaceMatcherIsAHeuristic documents the limit of the substring
// check. It is kept as defence in depth behind the admin gate, not as an access
// control, and this test exists so nobody mistakes it for one.
func TestContainsNamespaceMatcherIsAHeuristic(t *testing.T) {
	const ns = "ainsel"

	if !containsNamespaceMatcher(`up{namespace="ainsel"}`, ns) {
		t.Error("scoped query rejected")
	}
	// Passes the check while still selecting every series in the cluster.
	if !containsNamespaceMatcher(`up{namespace="ainsel"} or up`, ns) {
		t.Error("expected the heuristic to accept this bypass — if it now rejects it, the check was strengthened and this test should assert the stronger behaviour")
	}
	if containsNamespaceMatcher(`up`, ns) {
		t.Error("unscoped query accepted")
	}
}

// TestTelemetryScope is the unit-level contract for the scope value itself,
// independent of any handler or database.
func TestTelemetryScope(t *testing.T) {
	unrestricted := telemetryScope{unrestricted: true}
	if !unrestricted.allowsAgent("anything") || !unrestricted.allowsConnector("anything") {
		t.Error("unrestricted scope must allow everything")
	}
	if unrestricted.isEmpty() {
		t.Error("unrestricted scope must not report itself empty")
	}

	scoped := telemetryScope{agents: []string{"a-alice"}, connectors: []string{"c-alice"}}
	if !scoped.allowsAgent("a-alice") || scoped.allowsAgent("a-bob") {
		t.Error("agent scope wrong")
	}
	if !scoped.allowsConnector("c-alice") || scoped.allowsConnector("c-bob") {
		t.Error("connector scope wrong")
	}
	if scoped.isEmpty() {
		t.Error("scope with resources must not be empty")
	}

	// A caller with no readable resources must see nothing, and handlers
	// short-circuit on this rather than querying with an empty ANY() list —
	// which in SQL matches everything.
	empty := telemetryScope{}
	if !empty.isEmpty() {
		t.Error("scope with no resources must report empty")
	}
	if empty.allowsAgent("a-alice") {
		t.Error("empty scope must allow nothing")
	}
}
