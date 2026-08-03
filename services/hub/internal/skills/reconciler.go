package skills

import (
	"context"
	"fmt"

	sharedskills "github.com/DominikPinsel/ainsel/shared/api/skills"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Reconciler renders the single shared skills ConfigMap in the hub's
// namespace. Each skill is one data key (the skill ID) whose value is
// the full SKILL.md (frontmatter + body).
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

// Ensure creates or updates the skills ConfigMap so that the skill's
// data key reflects the given skill. The ConfigMap is shared across all
// skills; this method reads-modifies-writes it.
func (r *Reconciler) Ensure(ctx context.Context, sk *Skill) error {
	name := sharedskills.ConfigMapName
	var cm corev1.ConfigMap
	err := r.client.Get(ctx, types.NamespacedName{Name: name, Namespace: r.namespace}, &cm)
	if apierrors.IsNotFound(err) {
		cm = corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: r.namespace,
				Labels: map[string]string{
					"ainsel.dev/managed-by": "hub",
					"ainsel.dev/resource":   "skills",
				},
			},
			Data: map[string]string{
				sk.ID: assembleSKILLMD(sk),
			},
		}
		createErr := r.client.Create(ctx, &cm)
		if createErr == nil {
			return nil
		}
		// A concurrent Ensure may have created the ConfigMap between our
		// Get and Create. Fall through to the update path by re-reading
		// the now-existing object.
		if !apierrors.IsAlreadyExists(createErr) {
			return fmt.Errorf("create configmap %s: %w", name, createErr)
		}
		if err := r.client.Get(ctx, types.NamespacedName{Name: name, Namespace: r.namespace}, &cm); err != nil {
			return fmt.Errorf("get configmap %s after AlreadyExists: %w", name, err)
		}
	} else if err != nil {
		return fmt.Errorf("get configmap %s: %w", name, err)
	}

	if cm.Labels == nil {
		cm.Labels = map[string]string{}
	}
	cm.Labels["ainsel.dev/managed-by"] = "hub"
	cm.Labels["ainsel.dev/resource"] = "skills"
	if cm.Data == nil {
		cm.Data = map[string]string{}
	}
	cm.Data[sk.ID] = assembleSKILLMD(sk)
	if err := r.client.Update(ctx, &cm); err != nil {
		return fmt.Errorf("update configmap %s: %w", name, err)
	}
	return nil
}

// Delete removes the skill's data key from the shared ConfigMap.
// No error if the ConfigMap or key doesn't exist.
func (r *Reconciler) Delete(ctx context.Context, skillID string) error {
	name := sharedskills.ConfigMapName
	var cm corev1.ConfigMap
	err := r.client.Get(ctx, types.NamespacedName{Name: name, Namespace: r.namespace}, &cm)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("get configmap %s: %w", name, err)
	}
	if cm.Data == nil {
		return nil
	}
	if _, exists := cm.Data[skillID]; !exists {
		return nil
	}
	delete(cm.Data, skillID)
	if err := r.client.Update(ctx, &cm); err != nil {
		return fmt.Errorf("update configmap %s: %w", name, err)
	}
	return nil
}
