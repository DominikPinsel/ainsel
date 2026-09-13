// Package mcpservers contains helpers used by the agent controller to turn
// MCP server definitions into the runtime env values the agent expects, plus
// the one-shot resolution of legacy in-cluster server names.
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

// servicePath is the streamable-HTTP MCP mount path used when resolving a
// legacy server name to its in-cluster Service. The hub backend creates
// services with port "http"; this package assumes the standard MCP mount
// path of /mcp.
const servicePath = "/mcp"

// MissingEnvEntry pairs an MCP server name with the env-var name it
// references via tokenFromEnv when that env var is not defined on the
// AgentImage. The controller uses this to emit a Warning Event and set
// a Degraded condition on the Agent status.
type MissingEnvEntry struct {
	ServerName string
	EnvVarName string
}

// Resolve turns legacy MCP server names into agent-scoped server definitions
// by looking each name up as the in-cluster Service "mcp-<name>" in the given
// namespace. Names whose Service does not exist are reported in `missing` and
// skipped; the resolved definitions carry no TokenFromEnv, matching what the
// legacy discovery path injected into MCP_SERVERS. Returned slices preserve
// the input order.
//
// This exists only to migrate the deprecated Agent.spec.enabledMCPs field into
// spec.mcp — agent-scoped definitions carry their URL already, so nothing else
// needs in-cluster lookup.
func Resolve(ctx context.Context, c ctrlclient.Client, namespace string, names []string) (servers []ainselv1alpha1.AgentMCPServer, missing []string, err error) {
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
		servers = append(servers, ainselv1alpha1.AgentMCPServer{
			Name: name,
			URL:  fmt.Sprintf("http://%s.%s.svc.cluster.local:%d%s", svc.Name, namespace, port, servicePath),
		})
	}
	return servers, missing, nil
}

// EnvValue formats the discovered entries into the MCP_SERVERS env value
// format the agent runtime expects: comma-separated "name=url".
func EnvValue(entries []string) string {
	return strings.Join(entries, ",")
}

// DedupeEntries removes duplicate "name=url" entries, keeping the first
// occurrence of each server name. The controller builds MCP_SERVERS from
// several sources (the agent's or its profile's MCP servers, sidecar
// declarations, and the injected chat sidecar); an MCP declared twice would
// otherwise reach the runtime twice and be connected/registered twice.
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
