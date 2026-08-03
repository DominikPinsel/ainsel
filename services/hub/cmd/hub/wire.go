package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/DominikPinsel/ainsel/services/hub/internal/api"
	"github.com/DominikPinsel/ainsel/services/hub/internal/authz"
	"github.com/DominikPinsel/ainsel/services/hub/internal/db"
	"github.com/DominikPinsel/ainsel/services/hub/internal/mcpservers"
	"github.com/DominikPinsel/ainsel/services/hub/internal/personas"
	"github.com/DominikPinsel/ainsel/services/hub/internal/prometheus"
	"github.com/DominikPinsel/ainsel/services/hub/internal/skills"
	"github.com/DominikPinsel/ainsel/services/hub/internal/usertokens"
	"github.com/DominikPinsel/ainsel/shared/auth/oidc"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	ctrl "sigs.k8s.io/controller-runtime"
	crcache "sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

// wireDB runs migrations and opens the database pool.
func wireDB(ctx context.Context, dbURL string) (*pgxpool.Pool, error) {
	if dbURL == "" {
		return nil, fmt.Errorf("HUB_DB_URL is required")
	}
	if err := db.Migrate(ctx, dbURL); err != nil {
		return nil, fmt.Errorf("db migrate: %w", err)
	}
	pool, err := db.Open(ctx, dbURL)
	if err != nil {
		return nil, fmt.Errorf("db open: %w", err)
	}
	return pool, nil
}

// wireAuthZ creates the authz store and checker.
func wireAuthZ(ctx context.Context, pool *pgxpool.Pool) (*authz.Store, *authz.Checker) {
	store := authz.NewStore(pool)
	groupCache := authz.NewGroupCache(func(uid string) (map[string]authz.GroupRole, error) {
		return store.UserGroupRoles(ctx, uid)
	}, 30*time.Second)
	checker := authz.NewChecker(store, groupCache)
	return store, checker
}

// wireK8sManager creates the controller-runtime manager.
func wireK8sManager(namespace string) (ctrl.Manager, error) {
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:  scheme,
		Metrics: metricsserver.Options{BindAddress: "0"},
		Cache: crcache.Options{
			DefaultNamespaces: map[string]crcache.Config{namespace: {}},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create manager: %w", err)
	}
	return mgr, nil
}

// wireAPIClient creates the Kubernetes API client from the manager.
func wireAPIClient(mgr ctrl.Manager) (client.Client, error) {
	ac, err := client.New(mgr.GetConfig(), client.Options{
		Scheme:     mgr.GetScheme(),
		Mapper:     mgr.GetRESTMapper(),
		HTTPClient: mgr.GetHTTPClient(),
	})
	if err != nil {
		return nil, fmt.Errorf("create API client: %w", err)
	}
	return ac, nil
}


// wirePrometheus creates the Prometheus client if URL is set.
func wirePrometheus(promURL string) *prometheus.Client {
	if promURL == "" {
		slog.Warn("HUB_PROMETHEUS_URL not set, token queries will fail")
		return nil
	}
	slog.Info("prometheus client configured", "url", promURL)
	return prometheus.NewClient(promURL, nil)
}

// wireMCP creates the MCP servers service.
func wireMCP(pool *pgxpool.Pool) *mcpservers.Service {
	store := mcpservers.NewStore(pool)
	return mcpservers.NewService(store)
}

// wirePersonas creates the personas service.
func wirePersonas(pool *pgxpool.Pool, apiClient client.Client, namespace string) *personas.Service {
	store := personas.NewStore(pool)
	rec := personas.NewReconciler(apiClient, namespace)
	lister := personas.NewAgentLister(apiClient, namespace)
	return personas.NewService(store, rec, lister)
}

// wireSkills creates the skills service.
func wireSkills(pool *pgxpool.Pool, apiClient client.Client, namespace string) *skills.Service {
	store := skills.NewStore(pool)
	rec := skills.NewReconciler(apiClient, namespace)
	lister := skills.NewAgentImageLister(apiClient, namespace)
	return skills.NewService(store, rec, lister)
}

// wireAPIServer creates the API server and wires basic dependencies.
func wireAPIServer(c *container, cfg containerConfig) *api.Server {
	srv := api.New(c.apiClient, cfg.namespace, cfg.connectorCfg, c.promClient, c.invStore, c.mcpSvc, c.personaSvc, c.skillSvc, &api.Config{})
	srv.SetAuthZ(c.authzStore, c.authzChecker)
	srv.SetEventQueue(c.eventQueue)
	srv.SetChatStore(c.chatStore)
	srv.SetTriggerStore(c.triggerStore)
	srv.SetUserTokenStore(c.userTokenStore)
	srv.SetTaskLogStore(c.taskLogStore)

	if secret := os.Getenv("HUB_INTERNAL_VALIDATE_SECRET"); secret != "" {
		srv.SetInternalValidateSecret(secret)
		slog.Info("user token validate endpoint enabled")
	} else {
		slog.Warn("HUB_INTERNAL_VALIDATE_SECRET not set, user token validate endpoint disabled")
	}

	return srv
}

// wireAuthMiddleware sets up OIDC and authz middleware on the API server.
func wireAuthMiddleware(c *container, cfg containerConfig) error {
	issuer := os.Getenv("ZITADEL_ISSUER")
	projectID := os.Getenv("OIDC_PROJECT_ID")
	if issuer == "" || projectID == "" {
		slog.Warn("OIDC auth disabled: ZITADEL_ISSUER and OIDC_PROJECT_ID must both be set")
	} else {
		slog.Info("OIDC auth enabled", "issuer", issuer, "projectID", projectID)
		userInfoURL := strings.TrimRight(issuer, "/") + "/oauth/v2/userinfo"
		oidcMW, err := oidc.NewMiddleware(oidc.Config{
			Issuer:      issuer,
			Audience:    projectID,
			JWKSURL:     strings.TrimRight(issuer, "/") + "/oauth/v2/keys",
			UserInfoURL: userInfoURL,
		})
		if err != nil {
			return fmt.Errorf("auth middleware setup: %w", err)
		}
		userTokenMW := usertokens.NewMiddleware(c.userTokenStore, func(ctx context.Context, userID string) (string, error) {
			u, err := c.authzStore.GetUser(ctx, userID)
			if err != nil {
				return "", err
			}
			return u.Username, nil
		})
		// Authorization is enforced in handlers via requireRead/requireWrite/
		// requireManage helpers. The middleware chain only handles authentication.
		// The identity persistence middleware is the innermost handler so it
		// runs after userTokenMW and oidcMW have populated the request context
		// with the authenticated user's identity.
		c.apiServer.SetAuthMiddleware(func(next http.Handler) http.Handler {
			return userTokenMW(oidcMW(c.apiServer.IdentityPersistMiddleware(next)))
		})
		c.apiServer.SetUserInfoURL(userInfoURL)
	}

	if err := c.apiServer.ValidateAuthConfig(cfg.hubAllowInsecureNoAuth); err != nil {
		return fmt.Errorf("API auth middleware not configured (override: HUB_ALLOW_INSECURE_NO_AUTH): %w", err)
	}
	if !c.apiServer.AuthMiddlewareConfigured() {
		slog.Error("========================================")
		slog.Error("API IS RUNNING WITHOUT AUTHENTICATION")
		slog.Error("All /api/v1/* endpoints are open to any caller.")
		slog.Error("Set OIDC environment variables to enable auth.")
		slog.Error("This mode is intended for local development ONLY.")
		slog.Error("========================================",
			"override", "HUB_ALLOW_INSECURE_NO_AUTH=true",
		)
	}

	// Wire per-IP rate limiting. Defaults: 30 rps sustained, burst 60.
	// Override via HUB_RATE_LIMIT_RPS and HUB_RATE_LIMIT_BURST.
	rateRPS := 30.0
	rateBurst := 60
	if v := os.Getenv("HUB_RATE_LIMIT_RPS"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			rateRPS = f
		}
	}
	if v := os.Getenv("HUB_RATE_LIMIT_BURST"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			rateBurst = n
		}
	}
	c.apiServer.SetRateLimiter(rateRPS, rateBurst)
	slog.Info("Rate limiting enabled", "rps", rateRPS, "burst", rateBurst)

	return nil
}

// wireMetricsServer creates the Prometheus metrics HTTP server.
func wireMetricsServer(metricsPort string) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	return &http.Server{
		Addr:              ":" + metricsPort,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
}

// wireHTTPServer creates an HTTP server for the given port and handler.
func wireHTTPServer(port string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
}

// runUntilCanceled wraps a blocking function so that context.Canceled
// is converted to nil. Use for components expected to end via cancellation.
func runUntilCanceled(ctx context.Context, fn func(context.Context) error) func() error {
	return func() error {
		err := fn(ctx)
		if err == context.Canceled {
			return nil
		}
		return err
	}
}

// envOrDefault returns the env var value or a default.
func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// envIntOrDefault returns the env var value as a positive int or a default.
func envIntOrDefault(key string, defaultVal int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		slog.Warn("invalid integer env var, using default", "key", key, "value", v, "default", defaultVal)
		return defaultVal
	}
	return n
}
