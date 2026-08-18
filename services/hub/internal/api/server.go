package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/DominikPinsel/ainsel/services/hub/internal/authz"
	"github.com/DominikPinsel/ainsel/services/hub/internal/chat"
	"github.com/DominikPinsel/ainsel/services/hub/internal/eventqueue"
	"github.com/DominikPinsel/ainsel/services/hub/internal/invocations"
	"github.com/DominikPinsel/ainsel/services/hub/internal/mcpservers"
	"github.com/DominikPinsel/ainsel/services/hub/internal/personas"
	"github.com/DominikPinsel/ainsel/services/hub/internal/prometheus"
	"github.com/DominikPinsel/ainsel/services/hub/internal/skills"
	"github.com/DominikPinsel/ainsel/services/hub/internal/tasklogs"
	"github.com/DominikPinsel/ainsel/services/hub/internal/triggers"
	"github.com/DominikPinsel/ainsel/services/hub/internal/usertokens"
	"github.com/DominikPinsel/ainsel/shared/auth/oidc"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// authzStore is the interface satisfied by *authz.Store and test doubles.
type authzStore interface {
	UpsertUser(ctx context.Context, sub, email, username string) (*authz.User, error)
	GetUser(ctx context.Context, id string) (*authz.User, error)
	ListUsers(ctx context.Context) ([]authz.User, error)
	SetAdmin(ctx context.Context, userID string, isAdmin bool) error
	ClearUsername(ctx context.Context, id string) error
	CreateLocalUser(ctx context.Context, id, email, username, passwordHash string, isAdmin bool) (*authz.User, error)
	UserPasswordHash(ctx context.Context, id string) (string, error)
	SetPassword(ctx context.Context, id, hash string) error
	DeleteUser(ctx context.Context, id string) error
	UserGroupIDs(ctx context.Context, userID string) ([]string, error)
	UserGroupRoles(ctx context.Context, userID string) (map[string]authz.GroupRole, error)
	CreateGroup(ctx context.Context, name, description string) (*authz.Group, error)
	GetGroup(ctx context.Context, id string) (*authz.Group, error)
	ListGroups(ctx context.Context) ([]authz.Group, error)
	UpdateGroup(ctx context.Context, id, name, description string) (*authz.Group, error)
	DeleteGroup(ctx context.Context, id string) error
	AddGroupMember(ctx context.Context, groupID, userID string, role authz.GroupRole) error
	RemoveGroupMember(ctx context.Context, groupID, userID string) error
	ListGroupMembers(ctx context.Context, groupID string) ([]authz.MemberWithUser, error)
	SetResourceGroup(ctx context.Context, resourceType, resourceName, groupID string, public bool) error
	GetResourceGroup(ctx context.Context, resourceType, resourceName string) (*authz.ResourceGroup, error)
	DeleteResourceGroup(ctx context.Context, resourceType, resourceName string) error
	SetResourcePublic(ctx context.Context, resourceType, resourceName string, public bool) error
	ListResourcesByGroups(ctx context.Context, resourceType string, groupIDs []string, includePublic bool) ([]string, error)
}

// Server provides a REST API for managing Ainsel CRDs.
type Server struct {
	client                 client.Client
	mux                    *http.ServeMux
	ns                     string
	connectorCfg           ConnectorConfig
	prom                   *prometheus.Client
	wsHub                  *wsHub
	observabilityCache     *promCache
	invocations            *invocations.Store
	mcp                    *mcpservers.Service
	personas               *personas.Service
	skills                 *skills.Service
	triggerStore           *triggers.Store
	chat                   *chat.Store
	taskLogs               *tasklogs.Store
	authzStore             authzStore
	authzChecker           *authz.Checker
	userTokens             *usertokens.Store
	internalValidateSecret string

	// localAuthSecret is the HS256 signing key for local session JWTs.
	// Non-empty enables /api/v1/auth/login (auth.mode=local).
	localAuthSecret []byte
	// loginThrottle guards /api/v1/auth/login against per-username brute
	// force; initialized together with localAuthSecret.
	loginThrottle *loginThrottle

	// authMW, when non-nil, is applied to every /api/v1/* request before the
	// mux dispatches to a handler. Wired from main once OIDC env is present;
	// nil leaves the API open (local-dev escape hatch).
	authMW func(http.Handler) http.Handler
	// rateLimitMW, when non-nil, is applied to every request before auth.
	rateLimitMW func(http.Handler) http.Handler
	// userInfoURL is the OIDC userinfo endpoint URL, used by handleUserSync.
	userInfoURL    string
	userInfoClient *http.Client
	// eventQueue is the PostgreSQL event queue store.
	eventQueue *eventqueue.Store

	// identityTracker guards automatic identity persistence so it does not
	// fire on every authenticated request. See identity_persist.go.
	identityTracker *identityPersistTracker
}

// SetAuthZ wires the authorization store and checker.
func (s *Server) SetAuthZ(store authzStore, checker *authz.Checker) {
	s.authzStore = store
	s.authzChecker = checker
}

// SetUserTokenStore wires the user token store.
func (s *Server) SetUserTokenStore(store *usertokens.Store) {
	s.userTokens = store
}

// SetInternalValidateSecret sets the shared secret for /api/internal/user-tokens/validate.
func (s *Server) SetInternalValidateSecret(secret string) {
	s.internalValidateSecret = secret
}

// SetTriggerStore wires the trigger DB store for the /api/v1/triggers/* and /api/v1/cron-triggers/* endpoints.
func (s *Server) SetTriggerStore(store *triggers.Store) {
	s.triggerStore = store
}

// SetChatStore wires the chat session store for the /api/v1/chat/* endpoints.
func (s *Server) SetChatStore(store *chat.Store) {
	s.chat = store
}

// SetTaskLogStore wires the task log store for the observability logs and
// errors endpoints. When set, these endpoints serve structured log entries
// from the hub's own database instead of requiring an external log backend.
func (s *Server) SetTaskLogStore(store *tasklogs.Store) {
	s.taskLogs = store
}

// SetAuthMiddleware installs an HTTP middleware that wraps every /api/v1/*
// request. Pass nil (the default) to leave the API open. This follows the
// same pattern as other optional wiring: keep the constructor signature stable.
func (s *Server) SetAuthMiddleware(mw func(http.Handler) http.Handler) {
	s.authMW = mw
}

// SetUserInfoURL configures the OIDC userinfo endpoint used by the username
// sync handler. Must be called before the server starts serving.
func (s *Server) SetUserInfoURL(endpoint string) {
	s.userInfoURL = endpoint
	s.userInfoClient = &http.Client{Timeout: 5 * time.Second}
}

// ErrInsecureAuthConfig is returned by ValidateAuthConfig when the server
// has no auth middleware wired and the operator has not explicitly opted into
// running without auth.
var ErrInsecureAuthConfig = errors.New("api auth middleware is not configured")

// ValidateAuthConfig fails fast when the API would be served without any auth
// middleware and the operator has not explicitly opted into running without auth.
func (s *Server) ValidateAuthConfig(allowInsecureNoAuth bool) error {
	if s.authMW != nil {
		return nil
	}
	if allowInsecureNoAuth {
		return nil
	}
	return fmt.Errorf("%w: auth middleware is not configured; set HUB_ALLOW_INSECURE_NO_AUTH=true to override", ErrInsecureAuthConfig)
}

// AuthMiddlewareConfigured reports whether SetAuthMiddleware has been called
// with a non-nil middleware.
func (s *Server) AuthMiddlewareConfigured() bool {
	return s.authMW != nil
}

// Config carries optional dependencies for the API server.
type Config struct {
}

// New creates a new API server with routes registered.
func New(c client.Client, namespace string, connectorCfg ConnectorConfig, promClient *prometheus.Client, invStore *invocations.Store, mcp *mcpservers.Service, personaSvc *personas.Service, skillSvc *skills.Service, cfg *Config) *Server {
	s := &Server{client: c, mux: http.NewServeMux(), ns: namespace, connectorCfg: connectorCfg, prom: promClient, invocations: invStore, mcp: mcp, personas: personaSvc, skills: skillSvc}
	s.wsHub = newWsHub()
	s.observabilityCache = newPromCache(observabilityCacheTTL)
	s.identityTracker = newIdentityPersistTracker()
	s.mux.HandleFunc("/health", s.health)
	s.mux.HandleFunc("/api/v1/agents", s.handleAgents)
	s.mux.HandleFunc("/api/v1/agents/", s.handleAgent)
	s.mux.HandleFunc("/api/v1/agent-images", s.handleAgentImages)
	s.mux.HandleFunc("/api/v1/agent-images/", s.handleAgentImagePath)
	s.mux.HandleFunc("/api/v1/connectors", s.handleConnectors)
	s.mux.HandleFunc("/api/v1/connectors/", s.handleConnector)
	s.mux.HandleFunc("/api/v1/triggers", s.handleTriggers)
	s.mux.HandleFunc("/api/v1/triggers/", s.handleTrigger)
	s.mux.HandleFunc("/api/v1/cron-triggers", s.handleCronTriggers)
	s.mux.HandleFunc("/api/v1/cron-triggers/", s.handleCronTrigger)
	s.mux.HandleFunc("/api/v1/errors", s.handleErrors)
	s.mux.HandleFunc("/api/v1/events", s.handleEvents)
	s.mux.HandleFunc("/api/v1/events/", s.handleEvent)
	s.mux.HandleFunc("/api/v1/invocations", s.handleInvocations)
	s.mux.HandleFunc("/api/v1/invocations/", s.handleInvocation)
	s.mux.HandleFunc("/api/v1/mcp-servers", s.handleMCPServers)
	s.mux.HandleFunc("/api/v1/mcp-servers/", s.handleMCPServer)
	s.mux.HandleFunc("/api/v1/tokens", s.handleTokens)
	s.mux.HandleFunc("/api/v1/stats", s.handleStats)
	s.mux.HandleFunc("/api/v1/ws", s.handleWs)
	s.mux.HandleFunc("/api/v1/me", s.handleMe)
	s.mux.Handle("/api/v1/auth/me", oidc.MeHandler())
	s.mux.HandleFunc("/api/v1/observability/metrics/summary", s.handleObservability)
	s.mux.HandleFunc("/api/v1/observability/metrics/timeseries", s.handleObservability)
	s.mux.HandleFunc("/api/v1/observability/metrics/agents", s.handleObservability)
	s.mux.HandleFunc("/api/v1/observability/metrics/tokens/summary", s.handleObservability)
	s.mux.HandleFunc("/api/v1/observability/metrics/tokens/timeseries", s.handleObservability)
	s.mux.HandleFunc("/api/v1/observability/metrics/tokens/by-subject", s.handleObservability)
	s.mux.HandleFunc("/api/v1/observability/metrics/tokens/by-event", s.handleObservability)
	s.mux.HandleFunc("/api/v1/observability/conversations", s.handleConversations)
	s.mux.HandleFunc("/api/v1/metrics/summary", s.handleObservability)
	s.mux.HandleFunc("/api/v1/metrics/timeseries", s.handleObservability)
	s.mux.HandleFunc("/api/v1/metrics/agents", s.handleObservability)
	s.mux.HandleFunc("/api/v1/observability/logs", s.handleObservabilityLogs)
	s.mux.HandleFunc("/api/v1/observability/metrics/query", s.handleObservabilityMetricsQuery)
	s.mux.HandleFunc("/api/internal/events", s.handleIngestEvent)
	s.mux.HandleFunc("/api/internal/agents/", s.handleInternalAgent)
	s.mux.HandleFunc("/api/v1/queue/info", s.handleQueueInfo)
	s.mux.HandleFunc("/api/v1/queue/recent", s.handleQueueRecent)
	s.mux.HandleFunc("/api/internal/task-logs", s.handleIngestTaskLog)
	s.mux.HandleFunc("/api/internal/task-messages", s.handleIngestTaskMessage)
	s.mux.HandleFunc("/api/v1/platform/health", s.handlePlatformHealth)
	s.mux.HandleFunc("/api/v1/users", s.handleUsers)
	s.mux.HandleFunc("/api/v1/users/", s.handleUser)
	s.mux.HandleFunc("/api/v1/auth/login", s.handleLogin)
	s.mux.HandleFunc("/api/v1/auth/logout", s.handleLogout)
	s.mux.HandleFunc("/api/v1/auth/password", s.handleChangePassword)
	s.mux.HandleFunc("/api/v1/user-tokens", s.handleUserTokens)
	s.mux.HandleFunc("/api/v1/user-tokens/", s.handleUserTokenDelete)
	s.mux.HandleFunc("/api/internal/user-tokens/validate", s.handleUserTokenValidate)
	s.mux.HandleFunc("/api/internal/chat/sessions", s.handleInternalChatSessions)
	s.mux.HandleFunc("/api/internal/chat/sessions/", s.handleInternalChatSession)
	s.mux.HandleFunc("/api/v1/groups", s.handleGroups)
	s.mux.HandleFunc("/api/v1/groups/", s.handleGroup)
	s.mux.HandleFunc("/api/v1/me/resources", s.handleMyResources)
	s.mux.HandleFunc("/api/v1/chat/sessions", s.handleChatSessions)
	s.mux.HandleFunc("/api/v1/chat/sessions/", s.handleChatSession)
	if personaSvc != nil {
		RegisterPersonaRoutes(s.mux, personaSvc, &s.authzStore, &s.authzChecker)
	}
	if skillSvc != nil {
		RegisterSkillRoutes(s.mux, skillSvc, &s.authzStore, &s.authzChecker)
	}
	return s
}

// ServeHTTP implements http.Handler.
// SetRateLimiter installs a per-IP rate limiting middleware applied to all
// requests. Pass rps=0 to disable (the default).
func (s *Server) SetRateLimiter(rps float64, burst int) {
	if rps > 0 {
		s.rateLimitMW = RateLimitMiddleware(rps, burst)
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.rateLimitMW != nil {
		s.rateLimitMW(http.HandlerFunc(s.serveHTTPInner)).ServeHTTP(w, r)
		return
	}
	s.serveHTTPInner(w, r)
}

func (s *Server) serveHTTPInner(w http.ResponseWriter, r *http.Request) {
	if s.authMW != nil && strings.HasPrefix(r.URL.Path, "/api/v1/") && !isAuthExempt(r.URL.Path) {
		s.authMW(s.mux).ServeHTTP(w, r)
		return
	}
	s.mux.ServeHTTP(w, r)
}

// isAuthExempt lists /api/v1/* paths that must stay reachable without a
// session. Only the login endpoint qualifies: it validates credentials
// itself and is still protected by the global per-IP rate limiter plus a
// per-username throttle.
func isAuthExempt(path string) bool {
	return path == "/api/v1/auth/login"
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to encode JSON response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func extractName(path, prefix string) string {
	return strings.TrimPrefix(path, prefix)
}

func sanitizeLabelValue(v string) string {
	return strings.ReplaceAll(v, `"`, "")
}

// --- Authorization helpers ---

// requireRead checks that the authenticated user can read the resource.
// Returns true if access is granted (caller should proceed).
func (s *Server) requireRead(w http.ResponseWriter, r *http.Request, resourceType, resourceName string) bool {
	if s.authzChecker == nil {
		return true // authz not configured (dev mode)
	}
	u, ok := oidc.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return false
	}
	ok, err := s.authzChecker.CanRead(r.Context(), u.Sub, resourceType, resourceName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return false
	}
	if !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return false
	}
	return true
}

// requireWrite checks that the authenticated user can modify the resource.
func (s *Server) requireWrite(w http.ResponseWriter, r *http.Request, resourceType, resourceName string) bool {
	if s.authzChecker == nil {
		return true
	}
	u, ok := oidc.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return false
	}
	ok, err := s.authzChecker.CanWrite(r.Context(), u.Sub, resourceType, resourceName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return false
	}
	if !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return false
	}
	return true
}

// requireGroupOwner checks that the authenticated user is an owner of the group.
func (s *Server) requireGroupOwner(w http.ResponseWriter, r *http.Request, groupID string) bool {
	if s.authzChecker == nil {
		return true
	}
	u, ok := oidc.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return false
	}
	ok, err := s.authzChecker.CanManageGroup(r.Context(), u.Sub, groupID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return false
	}
	if !ok {
		writeError(w, http.StatusForbidden, "group owner or admin required")
		return false
	}
	return true
}

// requireGroupWrite checks that the authenticated user can create resources
// in the given group (writer or owner, or admin).
func (s *Server) requireGroupWrite(w http.ResponseWriter, r *http.Request, groupID string) bool {
	if s.authzChecker == nil {
		return true
	}
	u, ok := oidc.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return false
	}
	ok, err := s.authzChecker.CanWriteGroup(r.Context(), u.Sub, groupID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return false
	}
	if !ok {
		writeError(w, http.StatusForbidden, "writer or owner of the group required")
		return false
	}
	return true
}

// callerGroupIDs returns the group IDs for the authenticated user.
func (s *Server) callerGroupIDs(r *http.Request) []string {
	if s.authzStore == nil {
		return nil
	}
	u, ok := oidc.FromContext(r.Context())
	if !ok {
		return nil
	}
	ids, _ := s.authzStore.UserGroupIDs(r.Context(), u.Sub)
	return ids
}

// callerIsAdmin returns true if the authenticated user is an admin.
func (s *Server) callerIsAdmin(r *http.Request) bool {
	if s.authzChecker == nil {
		return false
	}
	u, ok := oidc.FromContext(r.Context())
	if !ok {
		return false
	}
	admin, _ := s.authzChecker.IsAdmin(r.Context(), u.Sub)
	return admin
}

// filterByAccess filters a list of resource names to those accessible by the caller.
func (s *Server) filterByAccess(r *http.Request, resourceType string, names []string) []string {
	if s.authzStore == nil || s.callerIsAdmin(r) {
		return names
	}
	groupIDs := s.callerGroupIDs(r)
	includePublic := r.URL.Query().Get("includePublic") == "true"
	accessible, err := s.authzStore.ListResourcesByGroups(r.Context(), resourceType, groupIDs, includePublic)
	if err != nil {
		return nil
	}
	set := make(map[string]bool, len(accessible))
	for _, n := range accessible {
		set[n] = true
	}
	var out []string
	for _, n := range names {
		if set[n] {
			out = append(out, n)
		}
	}
	return out
}

// toAccessSet converts a slice of names to a lookup set.
func toAccessSet(names []string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}
	return set
}
