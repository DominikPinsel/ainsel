package personas

import (
	"context"
	"fmt"
	"strconv"

	sharedpersonas "github.com/DominikPinsel/ainsel/shared/api/personas"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Reconciler renders the persona ConfigMap into the hub's namespace.
// Mirrors the mcpservers.Reconciler shape: namespace is injected via the
// constructor (rather than read from env at call time) so the API server
// constructor controls scope.
type Reconciler struct {
	client    ctrlclient.Client
	namespace string
}

// NewReconciler wires a Reconciler against the given client and namespace.
func NewReconciler(c ctrlclient.Client, namespace string) *Reconciler {
	return &Reconciler{client: c, namespace: namespace}
}

// Client exposes the underlying K8s client; primarily for tests.
func (r *Reconciler) Client() ctrlclient.Client {
	return r.client
}

// Namespace returns the namespace the Reconciler writes into.
func (r *Reconciler) Namespace() string {
	return r.namespace
}

// ConfigMapName returns the deterministic ConfigMap name for a persona id.
// Thin wrapper around the shared helper so the format only lives in
// shared/api/personas; both hub (producer) and agent operator (consumer)
// import it from there.
func ConfigMapName(personaID string) string {
	return sharedpersonas.PersonaConfigMapName(personaID)
}

// Ensure creates or updates the persona ConfigMap so its data reflects p.
// Idempotent: repeated Ensure calls with the same Persona overwrite the
// same fields, leaving the ConfigMap in the desired state.
func (r *Reconciler) Ensure(ctx context.Context, p *Persona) error {
	name := ConfigMapName(p.ID)
	var cm corev1.ConfigMap
	err := r.client.Get(ctx, types.NamespacedName{Name: name, Namespace: r.namespace}, &cm)
	if apierrors.IsNotFound(err) {
		cm = corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: r.namespace,
				Labels: map[string]string{
					"ainsel.dev/managed-by": "hub",
					"ainsel.dev/resource":   "persona",
				},
				Annotations: map[string]string{
					"ainsel.dev/persona-name":    p.Name,
					"ainsel.dev/persona-version": strconv.Itoa(p.CurrentVersion),
				},
			},
			Data: map[string]string{
				"persona.md": p.Text,
			},
		}
		if err := r.client.Create(ctx, &cm); err != nil {
			return fmt.Errorf("create configmap %s: %w", name, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("get configmap %s: %w", name, err)
	}

	// Update existing
	if cm.Labels == nil {
		cm.Labels = map[string]string{}
	}
	cm.Labels["ainsel.dev/managed-by"] = "hub"
	cm.Labels["ainsel.dev/resource"] = "persona"
	if cm.Annotations == nil {
		cm.Annotations = map[string]string{}
	}
	cm.Annotations["ainsel.dev/persona-name"] = p.Name
	cm.Annotations["ainsel.dev/persona-version"] = strconv.Itoa(p.CurrentVersion)
	if cm.Data == nil {
		cm.Data = map[string]string{}
	}
	cm.Data["persona.md"] = p.Text
	if err := r.client.Update(ctx, &cm); err != nil {
		return fmt.Errorf("update configmap %s: %w", name, err)
	}
	return nil
}

// Delete removes the ConfigMap. No error if it doesn't exist.
func (r *Reconciler) Delete(ctx context.Context, personaID string) error {
	name := ConfigMapName(personaID)
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: r.namespace},
	}
	if err := r.client.Delete(ctx, cm); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete configmap %s: %w", name, err)
	}
	return nil
}
