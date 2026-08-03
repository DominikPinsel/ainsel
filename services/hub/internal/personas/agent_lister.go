package personas

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// kubeAgentLister implements AgentLister by reading Agent CRs from
// Kubernetes via the controller-runtime client. Reads spec.persona.id
// from each CR and matches against the requested persona id.
//
// At A-prime-1's merge, no Agent CR uses spec.persona.id yet (that's
// A-prime-2). The lister returns an empty slice for every call until
// then. The code is written now so A-prime-2 doesn't have to add it —
// the Service depends on AgentLister to enforce delete-with-referrers.
type kubeAgentLister struct {
	client    ctrlclient.Client
	namespace string
}

// NewAgentLister wires a production AgentLister against the hub's K8s
// client and namespace.
func NewAgentLister(c ctrlclient.Client, namespace string) AgentLister {
	return &kubeAgentLister{client: c, namespace: namespace}
}

func (l *kubeAgentLister) ListReferrers(ctx context.Context, personaID string) ([]Referrer, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "ainsel.dev", Version: "v1alpha1", Kind: "AgentList",
	})
	if err := l.client.List(ctx, list, ctrlclient.InNamespace(l.namespace)); err != nil {
		return nil, err
	}
	var refs []Referrer
	for _, item := range list.Items {
		id, found, err := unstructured.NestedString(item.Object, "spec", "persona", "id")
		if err != nil || !found {
			continue
		}
		if id == personaID {
			refs = append(refs, Referrer{AgentName: item.GetName()})
		}
	}
	return refs, nil
}
