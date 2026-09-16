package mcpservers_test

import (
	"testing"

	"github.com/DominikPinsel/ainsel/operators/agent/internal/controller/mcpservers"
	ainselv1alpha1 "github.com/DominikPinsel/ainsel/shared/api/api/v1alpha1"
)

func TestEnvValueJoinsWithCommas(t *testing.T) {
	v := mcpservers.EnvValue([]string{"a=u1", "b=u2"})
	if v != "a=u1,b=u2" {
		t.Errorf("EnvValue: %q", v)
	}
	if mcpservers.EnvValue(nil) != "" {
		t.Errorf("EnvValue(nil) should be empty")
	}
}

func TestDedupeEntriesKeepsFirstOccurrence(t *testing.T) {
	in := []string{
		"mem0=http://mcp-mem0.ainsel.svc.cluster.local:8080/mcp",
		"forgejo=http://forgejo.workloads.svc.cluster.local:8080/mcp",
		"mem0=http://mcp-mem0.ainsel.svc.cluster.local:8080/mcp",
		"chat=http://localhost:8081/mcp",
	}
	got := mcpservers.DedupeEntries(in)
	want := []string{
		"mem0=http://mcp-mem0.ainsel.svc.cluster.local:8080/mcp",
		"forgejo=http://forgejo.workloads.svc.cluster.local:8080/mcp",
		"chat=http://localhost:8081/mcp",
	}
	if len(got) != 3 {
		t.Fatalf("got %d entries: %+v", len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestDedupeEmptyEntries(t *testing.T) {
	if got := mcpservers.DedupeEntries(nil); len(got) != 0 {
		t.Errorf("expected empty result, got %+v", got)
	}
}

func TestDedupeEntriesWithoutEquals(t *testing.T) {
	// Entries without '=' are deduped on the whole string.
	got := mcpservers.DedupeEntries([]string{"weird", "weird", "a=u"})
	if len(got) != 2 || got[0] != "weird" || got[1] != "a=u" {
		t.Errorf("got %+v", got)
	}
}

func TestTokenEnvValueEmitsKubernetesVarRefs(t *testing.T) {
	servers := []ainselv1alpha1.AgentImageMCPServer{
		{Name: "forgejo-mcp-server", URL: "http://x", TokenFromEnv: "FORGEJO_PAT"},
		{Name: "example-mcp", URL: "http://y" /* no token */},
		{Name: "github", URL: "http://z", TokenFromEnv: "GITHUB_TOKEN"}, // #nosec G101 -- test data, env var name not a credential
	}
	envNames := map[string]bool{"FORGEJO_PAT": true, "GITHUB_TOKEN": true}
	got, missing := mcpservers.TokenEnvValue(servers, envNames)
	want := "forgejo-mcp-server=$(FORGEJO_PAT),github=$(GITHUB_TOKEN)"
	if got != want {
		t.Errorf("value = %q want %q", got, want)
	}
	if len(missing) != 0 {
		t.Errorf("expected no missing, got %v", missing)
	}
}

func TestTokenEnvValueReportsMissingEnv(t *testing.T) {
	servers := []ainselv1alpha1.AgentImageMCPServer{
		{Name: "forgejo-mcp-server", URL: "http://x", TokenFromEnv: "FORGEJO_PAT"},
	}
	got, missing := mcpservers.TokenEnvValue(servers, map[string]bool{})
	if got != "" {
		t.Errorf("value should be empty when env is missing, got %q", got)
	}
	if len(missing) != 1 {
		t.Fatalf("expected 1 missing entry, got %d: %v", len(missing), missing)
	}
	if missing[0].ServerName != "forgejo-mcp-server" || missing[0].EnvVarName != "FORGEJO_PAT" {
		t.Errorf("missing entry: %+v", missing[0])
	}
}

func TestTokenEnvValueEmptyTokensYieldEmpty(t *testing.T) {
	got, missing := mcpservers.TokenEnvValue(nil, map[string]bool{})
	if got != "" || len(missing) != 0 {
		t.Errorf("expected empty result, got value=%q missing=%v", got, missing)
	}
}
