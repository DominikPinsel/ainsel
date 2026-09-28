package channels

import (
	"context"
	"fmt"

	ainselv1alpha1 "github.com/DominikPinsel/ainsel/shared/api/api/v1alpha1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// kubeRegistry reads the entity registries from the hub's namespace. Agents and
// webhook connectors are CRDs, so a channel's existence follows what the API
// server reports — the same source the console lists.
type kubeRegistry struct {
	client    ctrlclient.Client
	namespace string
}

// NewKubeRegistry wires a Registry against the given controller-runtime client.
func NewKubeRegistry(c ctrlclient.Client, namespace string) Registry {
	return &kubeRegistry{client: c, namespace: namespace}
}

// ListAgents returns one entity per Agent CR, keyed by its CR name.
func (r *kubeRegistry) ListAgents(ctx context.Context) ([]Entity, error) {
	list := &ainselv1alpha1.AgentList{}
	if err := r.client.List(ctx, list, ctrlclient.InNamespace(r.namespace)); err != nil {
		return nil, fmt.Errorf("channels: list agents: %w", err)
	}
	out := make([]Entity, 0, len(list.Items))
	for _, a := range list.Items {
		out = append(out, Entity{
			Ref:         a.Name,
			Name:        firstNonEmpty(a.Spec.DisplayName, a.Name),
			Description: a.Spec.Description,
		})
	}
	return out, nil
}

// ListConnectors returns one entity per WebhookConnector CR. The CR name is
// also the label the receiver stamps on its events, which is what makes a
// connector channel resolvable from an event alone.
func (r *kubeRegistry) ListConnectors(ctx context.Context) ([]Entity, error) {
	list := &ainselv1alpha1.WebhookConnectorList{}
	if err := r.client.List(ctx, list, ctrlclient.InNamespace(r.namespace)); err != nil {
		return nil, fmt.Errorf("channels: list connectors: %w", err)
	}
	out := make([]Entity, 0, len(list.Items))
	for _, c := range list.Items {
		out = append(out, Entity{Ref: c.Name, Name: firstNonEmpty(c.Spec.DisplayName, c.Name)})
	}
	return out, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
