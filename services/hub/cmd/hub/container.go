package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/DominikPinsel/ainsel/services/hub/internal/api"
	"github.com/DominikPinsel/ainsel/services/hub/internal/authz"
	"github.com/DominikPinsel/ainsel/services/hub/internal/chat"
	"github.com/DominikPinsel/ainsel/services/hub/internal/cron"
	"github.com/DominikPinsel/ainsel/services/hub/internal/eventqueue"
	"github.com/DominikPinsel/ainsel/services/hub/internal/invocations"
	"github.com/DominikPinsel/ainsel/services/hub/internal/mcpservers"
	"github.com/DominikPinsel/ainsel/services/hub/internal/personas"
	"github.com/DominikPinsel/ainsel/services/hub/internal/prometheus"
	"github.com/DominikPinsel/ainsel/services/hub/internal/router"
	"github.com/DominikPinsel/ainsel/services/hub/internal/skills"
	"github.com/DominikPinsel/ainsel/services/hub/internal/tasklogs"
	"github.com/DominikPinsel/ainsel/services/hub/internal/trigger"
	"github.com/DominikPinsel/ainsel/services/hub/internal/triggers"
	"github.com/DominikPinsel/ainsel/services/hub/internal/usertokens"

	"github.com/jackc/pgx/v5/pgxpool"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// container owns all constructed resources for the hub process.
// It is unexported and lives in cmd/hub to keep the boot sequence
// explicit and easy to step through in a debugger.
type container struct {
	pool           *pgxpool.Pool
	mgr            ctrl.Manager
	apiClient      client.Client
	namespace      string
	authzStore     *authz.Store
	authzChecker   *authz.Checker
	idx            *trigger.Index
	triggerStore   *triggers.Store
	cronEmitter    *cron.Emitter
	eventQueue     *eventqueue.Store
	invStore       *invocations.Store
	mcpSvc         *mcpservers.Service
	personaSvc     *personas.Service
	skillSvc       *skills.Service
	chatStore      *chat.Store
	apiServer      *api.Server
	userTokenStore *usertokens.Store
	rtr            *router.Router
	apiHTTPServer  *http.Server
	metricsServer  *http.Server
	promClient     *prometheus.Client
	taskLogStore   *tasklogs.Store
}

// containerDeps holds optional injectable dependencies for testing.
// When a field is nil, newContainer constructs the real resource.
type containerDeps struct {
	mgr       ctrl.Manager
	apiClient client.Client
}

// containerConfig holds env-derived configuration for newContainer.
type containerConfig struct {
	dbURL                  string
	namespace              string
	hubPort                string
	metricsPort            string
	promURL                string
	invocationCapacity     int
	claimTimeoutSecs       int
	connectorCfg           api.ConnectorConfig
	hubAllowInsecureNoAuth bool
}

// newContainer constructs all hub resources in dependency order.
// On error it cleans up partially constructed resources.
func newContainer(ctx context.Context, cfg containerConfig, deps containerDeps) (*container, error) {
	c := &container{namespace: cfg.namespace}

	// --- Database ---
	pool, err := wireDB(ctx, cfg.dbURL)
	if err != nil {
		return nil, err
	}
	c.pool = pool

	// --- Event Queue ---
	c.eventQueue = eventqueue.NewStore(pool)

	// --- AuthZ ---
	c.authzStore, c.authzChecker = wireAuthZ(ctx, pool)

	// --- Kubernetes manager + client ---
	if deps.mgr != nil {
		c.mgr = deps.mgr
	} else {
		mgr, err := wireK8sManager(cfg.namespace)
		if err != nil {
			c.Close()
			return nil, err
		}
		c.mgr = mgr
	}

	if deps.apiClient != nil {
		c.apiClient = deps.apiClient
	} else {
		ac, err := wireAPIClient(c.mgr)
		if err != nil {
			c.Close()
			return nil, err
		}
		c.apiClient = ac
	}

	// --- Triggers ---
	c.idx = trigger.NewIndex()
	c.triggerStore = triggers.NewStore(pool)

	// --- Observability clients ---
	c.promClient = wirePrometheus(cfg.promURL)

	// --- Invocation store + cron emitter ---
	c.invStore = invocations.NewStore(cfg.invocationCapacity)
	slog.Info("invocation history store ready", "capacity", c.invStore.Capacity())
	c.cronEmitter = cron.New(c.eventQueue, c.invStore)

	// --- Service layer ---
	c.mcpSvc = wireMCP(pool)
	c.personaSvc = wirePersonas(pool, c.apiClient, cfg.namespace)
	c.skillSvc = wireSkills(pool, c.apiClient, cfg.namespace)
	c.chatStore = chat.NewStore(pool)
	c.taskLogStore = tasklogs.NewStore(pool)

	// --- API server + auth middleware ---
	c.userTokenStore = usertokens.NewStore(pool)
	c.apiServer = wireAPIServer(c, cfg)
	if err := wireAuthMiddleware(ctx, c, cfg); err != nil {
		c.Close()
		return nil, err
	}

	// --- Router ---
	c.rtr = router.New(c.eventQueue, c.idx, c.apiServer, c.invStore)
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("create router: %w", err)
	}

	// --- HTTP servers ---
	c.metricsServer = wireMetricsServer(cfg.metricsPort)
	c.apiHTTPServer = wireHTTPServer(cfg.hubPort, c.apiServer)

	return c, nil
}

// Close releases all resources held by the container.
// The controller-runtime manager is not stopped here because it is
// stopped by cancelling the context passed to mgr.Start in main().
// The JetStream context (js) does not have its own close method;
// it is cleaned up when the underlying NATS connection (nc) is closed.
func (c *container) Close() {
	if c.pool != nil {
		c.pool.Close()
	}
}