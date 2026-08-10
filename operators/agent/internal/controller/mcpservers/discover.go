// Package mcpservers contains discovery helpers used by the agent
// controller to resolve enabled MCP names into runtime URLs.
package mcpservers

import (
	"context"
	"fmt"
	"strings"

	ainselv1alpha1 "github.com/DominikPinsel/ainsel/shared/api/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// servicePath is the streamable-HTTP MCP mount path. The hub backend creates
// services with port "http"; this package assumes the standard MCP mount
// path of /mcp. If a future MCP needs a different path, surface it via the
// MCPServer record and pass it through here (out of scope for v1).
const servicePath = "/mcp"

// MissingEnvEntry pairs an MCP server name with the env-var name it
// references via tokenFromEnv when that env var is not defined on the
// AgentImage. The controller uses this to emit a Warning Event and set
// a Degraded condition on the Agent status.
type MissingEnvEntry struct {
	ServerName string
	EnvVarName string
}

// Discover resolves the supplied MCP names to "name=url" entries. Names whose
// Service does not (yet) exist are reported in `missing` and skipped — the
// caller may log a warning and continue rolling out the agent. Returned slices
// preserve the input order.
func Discover(ctx context.Context, c ctrlclient.Client, namespace string, names []string) (entries []string, missing []string, err error) {
	for _, name := range names {
		svc := &corev1.Service{}
		err := c.Get(ctx, types.NamespacedName{Name: "mcp-" + name, Namespace: namespace}, svc)
		if apierrors.IsNotFound(err) {
			missing = append(missing, name)
			continue
		}
		if err != nil {
			return nil, nil, fmt.Errorf("get service mcp-%s: %w", name, err)
		}
		port := int32(8080)
		for _, p := range svc.Spec.Ports {
			if p.Name == "http" {
				port = p.Port
				break
			}
		}
		url := fmt.Sprintf("http://%s.%s.svc.cluster.local:%d%s", svc.Name, namespace, port, servicePath)
		entries = append(entries, fmt.Sprintf("%s=%s", name, url))
	}
	return entries, missing, nil
}

// EnvValue formats the discovered entries into the MCP_SERVERS env value
// format the agent runtime expects: comma-separated "name=url".
func EnvValue(entries []string) string {
	return strings.Join(entries, ",")
}

// DedupeEntries removes duplicate "name=url" entries, keeping the first
// occurrence of each server name. The controller builds MCP_SERVERS from
// several sources (Agent.spec.enabledMCPs discovery, AgentImage MCP
// servers, sidecar declarations, and the injected chat sidecar); an MCP
// declared on both the Agent and its AgentImage would otherwise reach the
// runtime twice and be connected/registered twice.
func DedupeEntries(entries []string) []string {
	seen := make(map[string]bool, len(entries))
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		name := e
		if i := strings.Index(e, "="); i > 0 {
			name = e[:i]
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, e)
	}
	return out
}

// TokenEnvValue builds the MCP_SERVER_TOKENS env-var value as a
// comma-separated "name=$(VAR)" string. Each server whose TokenFromEnv is
// set yields an entry that Kubernetes resolves at container start by
// substituting the env var named TokenFromEnv. Callers must guarantee
// that any referenced env var is defined earlier in the same container's
// env: list (envFrom-loaded vars are not eligible for $(VAR) substitution).
// envNames is the set of env-var names already present on the container; any
// server whose TokenFromEnv is not in that set is skipped and its details
// are returned in `missingEnv` so the caller can surface a Degraded
// condition and emit a Warning Event.
func TokenEnvValue(servers []ainselv1alpha1.AgentImageMCPServer, envNames map[string]bool) (value string, missingEnv []MissingEnvEntry) {
	var parts []string
	for _, s := range servers {
		if s.TokenFromEnv == "" {
			continue
		}
		if !envNames[s.TokenFromEnv] {
			missingEnv = append(missingEnv, MissingEnvEntry{
				ServerName: s.Name,
				EnvVarName: s.TokenFromEnv,
			})
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=$(%s)", s.Name, s.TokenFromEnv))
	}
	return strings.Join(parts, ","), missingEnv
}
