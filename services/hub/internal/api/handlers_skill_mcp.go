package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/DominikPinsel/ainsel/services/hub/internal/skills"
)

// skillMCPPath serves a read-only MCP view of the whole skill catalogue.
//
// It lives under /api/internal/ on purpose. The hub publishes only
// /api/v1/* through its ingress (chart/templates/hub-backend/ingress.yaml
// restricts the capture group to `v1` for exactly that reason), and
// serveHTTPInner runs the user-auth middleware only for /api/v1/* paths.
// So this endpoint is unreachable from outside the cluster and is not
// reachable by a user session either: the only credential that opens it is
// the dedicated catalogue token HUB_SKILLS_MCP_TOKEN, checked in
// requireCatalogueToken below. It is deliberately not
// HUB_INTERNAL_VALIDATE_SECRET -- see that function for why the two must
// stay separate.
const skillMCPPath = "/api/internal/skills/mcp"

// SkillDiscovery is the read surface the skill MCP needs. It is separate
// from SkillService on purpose: the catalogue reader must not acquire
// Create/Update/Delete just because it is a skills endpoint, and keeping
// the interfaces apart means adding a discovery method does not silently
// break every SkillService fake in the API tests.
type SkillDiscovery interface {
	SearchForDiscovery(ctx context.Context, query string, tags []string) ([]skills.SkillSummary, error)
	Get(ctx context.Context, id string) (*skills.Skill, error)
}

// Limits on one search response. Catalogue entries are metadata only, so
// a page is cheap; the bound exists so a tool call cannot drag thousands
// of rows into a model's context in one turn.
const (
	skillSearchDefaultLimit = 20
	skillSearchMaxLimit     = 50
)

// SetSkillDiscovery wires the catalogue and builds its MCP handler. Called
// from wiring before the server starts serving, so the handler is never
// mutated while requests are in flight.
func (s *Server) SetSkillDiscovery(d SkillDiscovery) {
	if d == nil {
		return
	}
	s.skillMCPHandler = s.requireCatalogueToken(skillMCPServer(d).ServeHTTP)
}

// skillMCPServer builds the MCP server for the catalogue. Two tools: find
// something, then read it. Nothing here writes.
func skillMCPServer(d SkillDiscovery) *server.StreamableHTTPServer {
	srv := server.NewMCPServer(
		"ainsel-skill-catalogue",
		"0.1.0",
		server.WithToolCapabilities(false),
		server.WithRecovery(),
	)

	srv.AddTool(
		mcp.NewTool("search_skills",
			mcp.WithDescription("Search the full hub skill catalogue by keyword and/or tags. Returns metadata only (id, name, description, tags) — never the body, so this stays cheap to call. Use it to find something, then load it with get_skill. An empty query with no tags lists the newest skills first (the catalogue is read in descending creation order)."),
			mcp.WithString("query", mcp.Description("Keyword matched case-insensitively against id, name and description")),
			mcp.WithArray("tags", mcp.Description("Tag names to filter by; a skill matches if it carries any of them"), mcp.WithStringItems()),
			mcp.WithNumber("limit", mcp.Description("Max results (default 20, hard max 50). Check truncated in the response before assuming you saw everything.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return searchSkills(ctx, d, req)
		},
	)

	srv.AddTool(
		mcp.NewTool("get_skill",
			mcp.WithDescription("Load one skill by id. Returns the complete SKILL.md — YAML frontmatter plus body — exactly as a file-mounted skill would read. Follow the instructions in it for the current task. Not limited by which skills your agent image has enabled."),
			mcp.WithString("id", mcp.Required(), mcp.Description("Skill id (slug) from search_skills")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getSkill(ctx, d, req)
		},
	)

	// Stateless: every request is its own session, so no session id is
	// issued and nothing has to be shared between hub replicas. A stateful
	// manager here would pin an agent's session to one replica and break
	// it on the next rollout, which for a long task means a mid-run
	// failure; sticky sessions would only paper over the rollout case.
	return server.NewStreamableHTTPServer(srv, server.WithStateLess(true))
}

// searchSkills answers a discovery query with metadata only. The body is
// deliberately absent: a model that reads 50 full skill files to pick one
// has paid the exact context cost this endpoint exists to avoid.
func searchSkills(ctx context.Context, d SkillDiscovery, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query := strings.TrimSpace(req.GetString("query", ""))
	tags := req.GetStringSlice("tags", nil)
	limit := req.GetInt("limit", skillSearchDefaultLimit)
	if limit <= 0 {
		limit = skillSearchDefaultLimit
	}
	if limit > skillSearchMaxLimit {
		limit = skillSearchMaxLimit
	}

	found, err := d.SearchForDiscovery(ctx, query, tags)
	if err != nil {
		return mcp.NewToolResultError("skill search failed: " + err.Error()), nil
	}

	// Matched is the true size of the result set, so it must be taken
	// before slicing. Reporting the sliced length instead would make a page
	// of 20 out of 300 read as "20 skills match", and truncated=true would
	// then contradict the one number a caller uses to decide whether to
	// narrow the query.
	matched := len(found)
	truncated := matched > limit
	if truncated {
		found = found[:limit]
	}

	items := make([]skillCatalogueEntry, 0, len(found))
	for _, sum := range found {
		items = append(items, skillCatalogueEntry{
			ID:          sum.ID,
			Name:        sum.Name,
			Description: sum.Description,
			Tags:        sum.Tags,
		})
	}

	payload, err := json.Marshal(struct {
		Items     []skillCatalogueEntry `json:"skills"`
		Matched   int                   `json:"matched"`
		Truncated bool                  `json:"truncated"`
	}{Items: items, Matched: matched, Truncated: truncated})
	if err != nil {
		return mcp.NewToolResultError("failed to encode results: " + err.Error()), nil
	}
	return mcp.NewToolResultText(string(payload)), nil
}

// skillCatalogueEntry is one search hit. It has no body field by
// construction, which is the point: the type is what keeps a large body
// from reaching the response by accident.
type skillCatalogueEntry struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

func getSkill(ctx context.Context, d SkillDiscovery, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := strings.TrimSpace(req.GetString("id", ""))
	if id == "" {
		return mcp.NewToolResultError("id is required"), nil
	}

	sk, err := d.Get(ctx, id)
	if err != nil {
		// The store's not-found sentinel is what the REST handlers map to
		// 404; a tool call has no status code, so say the same thing in
		// words the model can act on rather than echoing an opaque error.
		if errors.Is(err, skills.ErrNotFound) {
			return mcp.NewToolResultError("no skill with id " + id + " — run search_skills to find the right id"), nil
		}
		return mcp.NewToolResultError("failed to load skill " + id + ": " + err.Error()), nil
	}

	// Rendered through the same function that writes the mounted file, so
	// a skill an agent loads on demand is byte-identical to one an
	// operator enabled. Two renderers would drift, and the drift would
	// show up as "works when enabled, subtly broken when fetched".
	return mcp.NewToolResultText(skills.RenderSkillMD(sk)), nil
}

// requireCatalogueToken guards the catalogue MCP.
//
// It checks a dedicated token rather than the cluster-wide internal
// secret, for two reasons. The ingress comment on the hub spells out the
// first: that secret "must never be exposed to internet traffic where it
// can be replayed or brute-forced", and turning it into a bearer
// credential on a second endpoint widens exactly what it protects. The
// second is mechanical: the operator treats HUB_INTERNAL_VALIDATE_SECRET
// as a reserved name and injects its own copy, so an MCP server cannot
// reference it through spec.mcpServers[].tokenFromEnv at all.
//
// Both spellings of the credential are accepted because two callers
// present it differently: the agent runtime sends MCP server tokens as
// `Authorization: Bearer <token>` (pi/pi-extensions/ainsel-mcp/catalog.ts),
// while internal HTTP callers and curl use a header. Only one applies per
// request; both are compared against the same value.
//
// The header keeps its repo-wide name for caller ergonomics only. Everywhere
// else in the hub `X-Internal-Token` carries HUB_INTERNAL_VALIDATE_SECRET;
// here it carries HUB_SKILLS_MCP_TOKEN, and the two are not interchangeable
// in either direction (TestCatalogueDoesNotAcceptTheInternalSecret).
//
// A missing token means the endpoint is disabled, never open: a hub that
// did not configure one answers 503 rather than serving the catalogue to
// anything in the namespace.
func (s *Server) requireCatalogueToken(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.skillsMCPToken == "" {
			writeError(w, http.StatusServiceUnavailable, "skill catalogue token not configured")
			return
		}
		provided := r.Header.Get("X-Internal-Token")
		if auth := r.Header.Get("Authorization"); provided == "" && auth != "" {
			if strings.HasPrefix(auth, "Bearer ") {
				provided = strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
			}
		}
		if subtle.ConstantTimeCompare([]byte(provided), []byte(s.skillsMCPToken)) != 1 {
			writeError(w, http.StatusUnauthorized, "invalid skill catalogue token")
			return
		}
		next(w, r)
	}
}

// handleSkillMCP dispatches to the catalogue MCP. The route is registered
// by the constructor whether or not a catalogue is wired, so a hub running
// without one answers 503 rather than 404: the path exists and the feature
// does not, which are different things to an operator debugging an agent
// that cannot connect.
func (s *Server) handleSkillMCP(w http.ResponseWriter, r *http.Request) {
	if s.skillMCPHandler == nil {
		writeError(w, http.StatusServiceUnavailable, "skill catalogue not configured")
		return
	}
	s.skillMCPHandler.ServeHTTP(w, r)
}
