package controller

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestAgentImageCRDHasTokenFromEnv ensures the checked-in CRD base matches the
// Go types. This catches the class of bug where a developer updates the Go
// struct (e.g. renaming a JSON tag) but forgets to regenerate the CRD YAML.
//
// The specific failure mode: the live cluster CRD had `token` instead of
// `tokenFromEnv` in the mcpServers schema, causing the API server to silently
// drop the tokenFromEnv field from AgentImage specs. This meant
// MCP_SERVER_TOKENS was never built for forgejo-mcp-server, and all Forgejo
// API calls failed with "token is required".
func TestAgentImageCRDHasTokenFromEnv(t *testing.T) {
	b, err := os.ReadFile("../../config/crd/bases/ainsel.dev_agentimages.yaml")
	if err != nil {
		t.Fatalf("read CRD: %v", err)
	}

	var doc map[string]any
	if err := yaml.Unmarshal(b, &doc); err != nil {
		t.Fatalf("unmarshal CRD: %v", err)
	}

	// Navigate: spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.mcpServers.items.properties
	spec := mustMap(t, doc, "spec")
	versions := mustSlice(t, spec, "versions")
	if len(versions) == 0 {
		t.Fatal("no versions in CRD")
	}
	v0 := mustMap(t, versions[0], "")
	schema := mustMap(t, v0, "schema")
	oapi := mustMap(t, schema, "openAPIV3Schema")
	props := mustMap(t, oapi, "properties")
	specProps := mustMap(t, props, "spec")
	specPropsProps := mustMap(t, specProps, "properties")
	mcpServers := mustMap(t, specPropsProps, "mcpServers")
	items := mustMap(t, mcpServers, "items")
	itemProps := mustMap(t, items, "properties")

	if _, ok := itemProps["tokenFromEnv"]; !ok {
		t.Errorf("AgentImage CRD mcpServers schema missing 'tokenFromEnv' field; found keys: %v", keys(itemProps))
	}
	if _, ok := itemProps["token"]; ok {
		t.Errorf("AgentImage CRD mcpServers schema still has old 'token' field; should be 'tokenFromEnv'")
	}

	// Also verify tokenFromEnv is NOT required (it's optional)
	required := mustSlice(t, items, "required")
	for _, r := range required {
		if s, ok := r.(string); ok && s == "token" {
			t.Errorf("AgentImage CRD mcpServers schema incorrectly requires 'token' field")
		}
	}
}

func mustMap(t *testing.T, parent any, key string) map[string]any {
	t.Helper()
	var m map[string]any
	if key == "" {
		// parent is already the map
		m, _ = parent.(map[string]any)
	} else {
		p, _ := parent.(map[string]any)
		if p == nil {
			t.Fatalf("expected map for parent, got %T", parent)
		}
		v, ok := p[key]
		if !ok {
			t.Fatalf("key %q not found in map", key)
		}
		m, _ = v.(map[string]any)
	}
	if m == nil {
		t.Fatalf("expected map[string]any for key %q, got %T", key, parent)
	}
	return m
}

func mustSlice(t *testing.T, parent any, key string) []any {
	t.Helper()
	p, _ := parent.(map[string]any)
	if p == nil {
		t.Fatalf("expected map for parent, got %T", parent)
	}
	v, ok := p[key]
	if !ok {
		t.Fatalf("key %q not found in map", key)
	}
	s, ok := v.([]any)
	if !ok {
		t.Fatalf("expected []any for key %q, got %T", key, v)
	}
	return s
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
