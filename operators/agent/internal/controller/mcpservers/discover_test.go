package mcpservers_test

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/DominikPinsel/ainsel/operators/agent/internal/controller/mcpservers"
	ainselv1alpha1 "github.com/DominikPinsel/ainsel/shared/api/api/v1alpha1"
)

func newFake(t *testing.T, services ...*corev1.Service) *fake.ClientBuilder {
	t.Helper()
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	b := fake.NewClientBuilder().WithScheme(scheme)
	for _, s := range services {
		b = b.WithObjects(s)
	}
	return b
}

func mcpService(name, ns string, port int32) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "mcp-" + name, Namespace: ns},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{Name: "http", Port: port}},
		},
	}
}

func TestDiscoverReturnsURLs(t *testing.T) {
	c := newFake(t, mcpService("example-mcp", "ainsel", 8080)).Build()
	got, missing, err := mcpservers.Discover(context.Background(), c, "ainsel", []string{"example-mcp"})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d entries: %+v", len(got), got)
	}
	want := "example-mcp=http://mcp-example-mcp.ainsel.svc.cluster.local:8080/mcp"
	if got[0] != want {
		t.Errorf("got %q want %q", got[0], want)
	}
	if len(missing) != 0 {
		t.Errorf("expected no missing, got %v", missing)
	}
}

func TestDiscoverPreservesOrder(t *testing.T) {
	c := newFake(t,
		mcpService("example-mcp", "ainsel", 8080),
		mcpService("github", "ainsel", 8080),
	).Build()
	got, _, err := mcpservers.Discover(context.Background(), c, "ainsel", []string{"github", "example-mcp"})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if got[0] != "github=http://mcp-github.ainsel.svc.cluster.local:8080/mcp" {
		t.Errorf("order broken: %+v", got)
	}
}

func TestDiscoverMissingServiceIsSkippedAndReported(t *testing.T) {
	c := newFake(t, mcpService("example-mcp", "ainsel", 8080)).Build()
	got, missing, err := mcpservers.Discover(context.Background(), c, "ainsel", []string{"example-mcp", "ghost"})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 url, got %+v", got)
	}
	if len(missing) != 1 || missing[0] != "ghost" {
		t.Errorf("missing: %+v", missing)
	}
}

func TestDiscoverEmptyListReturnsEmpty(t *testing.T) {
	c := newFake(t).Build()
	got, missing, err := mcpservers.Discover(context.Background(), c, "ainsel", nil)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 0 || len(missing) != 0 {
		t.Errorf("expected empty, got got=%v missing=%v", got, missing)
	}
}

func TestEnvValueJoinsWithCommas(t *testing.T) {
	v := mcpservers.EnvValue([]string{"a=u1", "b=u2"})
	if v != "a=u1,b=u2" {
		t.Errorf("EnvValue: %q", v)
	}
	if mcpservers.EnvValue(nil) != "" {
		t.Errorf("EnvValue(nil) should be empty")
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
