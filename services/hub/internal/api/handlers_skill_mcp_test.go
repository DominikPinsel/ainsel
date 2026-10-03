package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"gopkg.in/yaml.v3"

	"github.com/DominikPinsel/ainsel/services/hub/internal/skills"
	"github.com/DominikPinsel/ainsel/shared/auth/oidc"
)

// stubDiscovery is the read-only catalogue the MCP is given. Deliberately
// no create/update/delete: if the endpoint could write, this stub would
// have to implement it.
type stubDiscovery struct {
	all     []skills.SkillSummary
	byID    map[string]*skills.Skill
	searchE error
	calls   int
	lastQ   string
	lastTag []string
}

func (s *stubDiscovery) SearchForDiscovery(_ context.Context, query string, tags []string) ([]skills.SkillSummary, error) {
	s.calls++
	s.lastQ, s.lastTag = query, tags
	if s.searchE != nil {
		return nil, s.searchE
	}
	// Mirror the store's ILIKE semantics loosely: the filter itself is
	// tested at the store level, this only has to prove the tool forwards
	// its arguments instead of ignoring them.
	if query == "" && len(tags) == 0 {
		return s.all, nil
	}
	var out []skills.SkillSummary
	for _, sum := range s.all {
		if query != "" && !strings.Contains(strings.ToLower(sum.ID+sum.Name+sum.Description), strings.ToLower(query)) {
			continue
		}
		if len(tags) > 0 && !hasAnyTag(sum.Tags, tags) {
			continue
		}
		out = append(out, sum)
	}
	return out, nil
}

func (s *stubDiscovery) Get(_ context.Context, id string) (*skills.Skill, error) {
	sk, ok := s.byID[id]
	if !ok {
		return nil, skills.ErrNotFound
	}
	return sk, nil
}

func hasAnyTag(have, want []string) bool {
	for _, w := range want {
		for _, h := range have {
			if h == w {
				return true
			}
		}
	}
	return false
}

const (
	bodySentinel   = "SECRET-BODY-TEXT-DO-NOT-LEAK"
	internalSecret = "test-catalogue-token"
)

// newCatalogueServer builds a Server wired like production for this route:
// an auth middleware that rejects any request without an authenticated
// user (the OIDC/local behaviour), plus the handler-level internal-secret
// check. If the middleware ever started applying to /api/internal/*, the
// catalogue would stop being reachable by agents and these tests fail —
// which is the point of asserting through ServeHTTP rather than the handler.
func newCatalogueServer(t *testing.T, d SkillDiscovery, token string) *Server {
	t.Helper()
	s := &Server{mux: http.NewServeMux()}
	s.SetSkillsMCPToken(token)
	s.mux.HandleFunc(skillMCPPath, s.handleSkillMCP)
	s.SetAuthMiddleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := oidc.FromContext(r.Context()); !ok {
				http.Error(w, "missing bearer token", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	if d != nil {
		s.SetSkillDiscovery(d)
	}
	return s
}

func catalogueFixture() *stubDiscovery {
	return &stubDiscovery{
		all: []skills.SkillSummary{
			{ID: "rubric-designer", Name: "Rubric Designer", Description: "Designs rubrics: criteria and levels", Tags: []string{"education", "assessment"}},
			{ID: "hint-ladder", Name: "Hint Ladder", Description: "Graduated hint sequences", Tags: []string{"education"}},
		},
		byID: map[string]*skills.Skill{
			"rubric-designer": {ID: "rubric-designer", Name: "Rubric Designer", Description: "Designs rubrics: criteria and levels", Body: bodySentinel, Tags: []string{"education"}},
			"hint-ladder":     {ID: "hint-ladder", Name: "Hint Ladder", Description: "Graduated hint sequences", Body: "second body", Tags: []string{"education"}},
		},
	}
}

func doCatalogue(s *Server, method, path, header, value string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if header != "" {
		req.Header.Set(header, value)
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

// TestCatalogueBypassesUserAuthAndRequiresToken pins both halves of the
// security design: the route is reachable without a user session (so
// agents can use it) and unreachable without the catalogue token (so
// nothing in the cluster can read the catalogue anonymously).
func TestCatalogueBypassesUserAuthAndRequiresToken(t *testing.T) {
	s := newCatalogueServer(t, catalogueFixture(), internalSecret)

	cases := []struct {
		name   string
		header string
		value  string
		want   int
		// wantBody distinguishes the two 401s. The user-auth middleware
		// says "missing bearer token"; the catalogue gate says "invalid
		// skill catalogue token". A 401 from the wrong layer means the
		// path classification changed.
		wantBody string
	}{
		{"anonymous", "", "", http.StatusUnauthorized, "invalid skill catalogue token"},
		{"wrong catalogue token", "X-Internal-Token", "nope", http.StatusUnauthorized, "invalid skill catalogue token"},
		{"wrong bearer", "Authorization", "Bearer nope", http.StatusUnauthorized, "invalid skill catalogue token"},
		{"user session token is not a catalogue token", "Authorization", "Bearer ainsel-some-user-token", http.StatusUnauthorized, "invalid skill catalogue token"},
		{"catalogue token as bearer", "Authorization", "Bearer " + internalSecret, http.StatusOK, ""},
		{"catalogue token as header", "X-Internal-Token", internalSecret, http.StatusOK, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doCatalogue(s, http.MethodPost, skillMCPPath, tc.header, tc.value)
			if tc.want == http.StatusOK {
				// A bare POST with no MCP body is not a valid protocol
				// exchange, so 200 is not expected; what matters is that
				// the gate let it through to the MCP layer, which rejects
				// the payload rather than the caller.
				if rec.Code == http.StatusUnauthorized {
					t.Fatalf("request was rejected at the secret gate: %s", rec.Body.String())
				}
				return
			}
			if rec.Code != tc.want {
				t.Fatalf("got %d, want %d (%s)", rec.Code, tc.want, rec.Body.String())
			}
			if tc.wantBody != "" && !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Fatalf("401 came from the wrong layer: want %q, got %q", tc.wantBody, rec.Body.String())
			}
		})
	}
}

func TestCatalogueRefusesToOpenWithoutConfiguredSecret(t *testing.T) {
	// An empty token must not mean "no auth". The hub can start without
	// HUB_SKILLS_MCP_TOKEN, and in that state the catalogue is
	// unavailable, not world-readable inside the cluster.
	s := newCatalogueServer(t, catalogueFixture(), "")
	rec := doCatalogue(s, http.MethodPost, skillMCPPath, "", "")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want 503 (%s)", rec.Code, rec.Body.String())
	}
}

func TestCatalogueWithoutDiscoveryServiceIs503(t *testing.T) {
	s := newCatalogueServer(t, nil, internalSecret)
	rec := doCatalogue(s, http.MethodPost, skillMCPPath, "X-Internal-Token", internalSecret)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want 503 (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "not configured") {
		t.Errorf("want a 503 that says the catalogue is not configured, got %q", rec.Body.String())
	}
}

// --- end-to-end over the real MCP protocol --------------------------------
//
// These drive the endpoint with the same mcp-go client the agent runtime
// extension uses, so tool names, schemas, and result shapes are checked as
// an agent would see them rather than as the handler produces them.

type catalogueClient struct {
	c *client.Client
}

func connectCatalogue(t *testing.T, d SkillDiscovery, secret string) *catalogueClient {
	t.Helper()
	s := newCatalogueServer(t, d, secret)
	ts := httptest.NewServer(s)
	t.Cleanup(ts.Close)

	trans, err := transport.NewStreamableHTTP(ts.URL+skillMCPPath,
		transport.WithHTTPHeaders(map[string]string{"Authorization": "Bearer " + secret}))
	if err != nil {
		t.Fatalf("transport: %v", err)
	}
	c := client.NewClient(trans)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	if err := c.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, err := c.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ClientInfo:      mcp.Implementation{Name: "ainsel-catalogue-test", Version: "0"},
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
		},
	}); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	return &catalogueClient{c: c}
}

func (cc *catalogueClient) call(t *testing.T, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := cc.c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: name, Arguments: args},
	})
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	return res
}

func textOf(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	for _, item := range res.Content {
		if tc, ok := item.(mcp.TextContent); ok {
			return tc.Text
		}
	}
	t.Fatalf("result has no text content: %+v", res.Content)
	return ""
}

func TestCatalogueServesExactlyTwoReadOnlyTools(t *testing.T) {
	cc := connectCatalogue(t, catalogueFixture(), internalSecret)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tools, err := cc.c.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}

	got := map[string]bool{}
	for _, tool := range tools.Tools {
		got[tool.Name] = true
	}
	for _, want := range []string{"search_skills", "get_skill"} {
		if !got[want] {
			t.Errorf("missing tool %q; got %v", want, got)
		}
	}
	// The admin gateway's write tools must not appear here. An agent given
	// the catalogue must not be able to edit or delete shared skills.
	for _, forbidden := range []string{"create_skill", "update_skill", "delete_skill", "assign_skill", "list_skills"} {
		if got[forbidden] {
			t.Errorf("catalogue exposes %q: it is meant to be read-only", forbidden)
		}
	}
	if len(got) != 2 {
		t.Errorf("catalogue advertises %d tools, want exactly 2: %v", len(got), got)
	}
}

func TestCatalogueSearchReturnsMetadataAndNeverBodies(t *testing.T) {
	cc := connectCatalogue(t, catalogueFixture(), internalSecret)
	res := cc.call(t, "search_skills", map[string]any{"query": "rubric"})
	if res.IsError {
		t.Fatalf("search returned an error: %s", textOf(t, res))
	}
	text := textOf(t, res)

	if strings.Contains(text, bodySentinel) {
		t.Fatalf("search leaked the skill body into the response: %s", text)
	}
	var payload struct {
		Skills []struct {
			ID          string   `json:"id"`
			Name        string   `json:"name"`
			Description string   `json:"description"`
			Tags        []string `json:"tags"`
		} `json:"skills"`
		Matched   int  `json:"matched"`
		Truncated bool `json:"truncated"`
	}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("search result is not JSON: %v\n%s", err, text)
	}
	if len(payload.Skills) != 1 || payload.Skills[0].ID != "rubric-designer" {
		t.Fatalf("want the single rubric hit, got %s", text)
	}
	if payload.Skills[0].Description == "" {
		t.Error("search must carry the description: it is how an agent decides whether to load a skill")
	}
	if payload.Truncated || payload.Matched != 1 {
		t.Errorf("want matched=1 truncated=false, got matched=%d truncated=%v", payload.Matched, payload.Truncated)
	}
}

// TestCatalogueSearchReportsTruncationTruthfully is the regression for the
// bug found while writing these tests: matched was taken after slicing, so
// a page of 2 out of 6 reported matched=2 and read as "only 2 exist".
func TestCatalogueSearchReportsTruncationTruthfully(t *testing.T) {
	d := &stubDiscovery{byID: map[string]*skills.Skill{}}
	for i := 0; i < 6; i++ {
		d.all = append(d.all, skills.SkillSummary{
			ID:          "skill-" + string(rune('a'+i)),
			Name:        "Skill",
			Description: "description",
		})
	}
	cc := connectCatalogue(t, d, internalSecret)
	res := cc.call(t, "search_skills", map[string]any{"limit": 2})
	var payload struct {
		Skills    []map[string]any `json:"skills"`
		Matched   int              `json:"matched"`
		Truncated bool             `json:"truncated"`
	}
	if err := json.Unmarshal([]byte(textOf(t, res)), &payload); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, textOf(t, res))
	}
	if len(payload.Skills) != 2 {
		t.Fatalf("want 2 returned, got %d", len(payload.Skills))
	}
	if payload.Matched != 6 {
		t.Errorf("matched = %d, want 6 (the true result size, not the page size)", payload.Matched)
	}
	if !payload.Truncated {
		t.Error("truncated must be true when 4 of 6 were left out")
	}
}

func TestCatalogueSearchClampsLimit(t *testing.T) {
	d := &stubDiscovery{byID: map[string]*skills.Skill{}}
	for i := 0; i < 60; i++ {
		d.all = append(d.all, skills.SkillSummary{ID: "s"})
	}
	cc := connectCatalogue(t, d, internalSecret)
	res := cc.call(t, "search_skills", map[string]any{"limit": 100000})
	var payload struct {
		Skills []map[string]any `json:"skills"`
	}
	if err := json.Unmarshal([]byte(textOf(t, res)), &payload); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if len(payload.Skills) != skillSearchMaxLimit {
		t.Errorf("got %d results, want the hard max %d", len(payload.Skills), skillSearchMaxLimit)
	}
}

func TestCatalogueSearchForwardsTags(t *testing.T) {
	d := catalogueFixture()
	cc := connectCatalogue(t, d, internalSecret)
	cc.call(t, "search_skills", map[string]any{"query": "designer", "tags": []any{"assessment"}})
	if d.lastQ != "designer" {
		t.Errorf("query forwarded as %q, want %q", d.lastQ, "designer")
	}
	if !hasAnyTag(d.lastTag, []string{"assessment"}) {
		t.Errorf("tags not forwarded, got %v", d.lastTag)
	}
}

func TestCatalogueSearchFailureBecomesAToolError(t *testing.T) {
	d := catalogueFixture()
	d.searchE = errors.New("db gone")
	cc := connectCatalogue(t, d, internalSecret)
	res := cc.call(t, "search_skills", map[string]any{"query": "x"})
	if !res.IsError {
		t.Fatal("a failed search must surface as a tool error, not an empty result")
	}
	if !strings.Contains(textOf(t, res), "db gone") {
		t.Errorf("want the cause reported, got %q", textOf(t, res))
	}
}

func TestCatalogueGetSkillReturnsTheMountedFile(t *testing.T) {
	cc := connectCatalogue(t, catalogueFixture(), internalSecret)
	res := cc.call(t, "get_skill", map[string]any{"id": "rubric-designer"})
	if res.IsError {
		t.Fatalf("get_skill errored: %s", textOf(t, res))
	}
	got := textOf(t, res)

	// The same renderer the ConfigMap uses, so a fetched skill is
	// byte-identical to an enabled one.
	want := skills.RenderSkillMD(&skills.Skill{
		ID: "rubric-designer", Name: "Rubric Designer",
		Description: "Designs rubrics: criteria and levels", Body: bodySentinel,
	})
	if got != want {
		t.Errorf("rendered skill differs from the mounted form:\ngot:  %q\nwant: %q", got, want)
	}
	if !strings.Contains(got, bodySentinel) {
		t.Error("get_skill must carry the body — that is the point of loading a skill")
	}
}

// TestCatalogueGetSkillFrontmatterParses checks the delivered SKILL.md is
// readable: this fixture description carries ": ", the exact shape that
// made six live hub skills render frontmatter no YAML parser accepts.
func TestCatalogueGetSkillFrontmatterParses(t *testing.T) {
	cc := connectCatalogue(t, catalogueFixture(), internalSecret)
	got := textOf(t, cc.call(t, "get_skill", map[string]any{"id": "rubric-designer"}))

	fm := strings.TrimPrefix(got, "---\n")
	idx := strings.Index(fm, "\n---\n")
	if idx < 0 {
		t.Fatalf("no closing frontmatter delimiter: %q", got)
	}
	var meta map[string]any
	if err := yaml.Unmarshal([]byte(fm[:idx]), &meta); err != nil {
		t.Fatalf("frontmatter is not valid YAML: %v\n%s", err, fm[:idx])
	}
	if meta["description"] != "Designs rubrics: criteria and levels" {
		t.Errorf("description round trip = %v, want the original with its colon intact", meta["description"])
	}
	if meta["name"] != "rubric-designer" {
		t.Errorf("name = %v, want rubric-designer", meta["name"])
	}
}

func TestCatalogueGetSkillUnknownIDNamesTheID(t *testing.T) {
	cc := connectCatalogue(t, catalogueFixture(), internalSecret)
	res := cc.call(t, "get_skill", map[string]any{"id": "does-not-exist"})
	if !res.IsError {
		t.Fatal("want a tool error for an unknown id")
	}
	text := textOf(t, res)
	if !strings.Contains(text, "does-not-exist") {
		t.Errorf("error should name the id the agent asked for, got %q", text)
	}
	if !strings.Contains(text, "search_skills") {
		t.Errorf("error should point at how to find the right id, got %q", text)
	}
}

func TestCatalogueGetSkillRequiresID(t *testing.T) {
	cc := connectCatalogue(t, catalogueFixture(), internalSecret)
	res := cc.call(t, "get_skill", map[string]any{})
	if !res.IsError || !strings.Contains(textOf(t, res), "id is required") {
		t.Errorf("want a clear missing-id error, got %q", textOf(t, res))
	}
}

// TestCatalogueDoesNotAcceptTheInternalSecret keeps the two credentials
// separate. The catalogue has its own token precisely so the cluster-wide
// internal secret does not become a bearer credential for a second
// endpoint -- the hub ingress comment is explicit that that secret must
// not travel in traffic where it can be replayed. If someone "simplifies"
// the gate back to the shared secret, this fails.
func TestCatalogueDoesNotAcceptTheInternalSecret(t *testing.T) {
	s := newCatalogueServer(t, catalogueFixture(), internalSecret)
	s.SetInternalValidateSecret("a-different-cluster-wide-secret")

	rec := doCatalogue(s, http.MethodPost, skillMCPPath, "X-Internal-Token", "a-different-cluster-wide-secret")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("the internal secret must not open the catalogue: got %d (%s)", rec.Code, rec.Body.String())
	}
	// ...and the catalogue token must not be accepted as an internal token
	// on the endpoints that do use it, so neither leaks into the other.
	rec = doCatalogue(s, http.MethodPost, skillMCPPath, "X-Internal-Token", internalSecret)
	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("the catalogue token itself was rejected: %s", rec.Body.String())
	}
}
