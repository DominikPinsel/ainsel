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
// the full SKILL.md (frontmatter + body). The object is a cache of the
// skills agents have enabled, not a mirror of the registry: see
// Converge.
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

// Converge makes the shared ConfigMap hold exactly the data keys in
// keep, each carrying the rendered SKILL.md for that skill. Any other key
// in the object is pruned, which is what releases space for skills that
// are actually enabled -- the object is hub-owned, so nothing else is
// expected to live in it.
//
// Ordering matters: non-target keys are dropped before any target key is
// written, so a ConfigMap that filled up with catalogue entries can
// recover in a single pass rather than failing every write.
//
// keep must come from a successful live read of the enabling CRs. The
// prune is unconditional, so a wrong-empty keep would strip every key out
// of the object and agents would come up without their skills until the
// next pass. Callers therefore propagate a list error instead of an empty
// set, and the pass aborts before pruning; do not source keep from a
// cache that can answer "empty" for "not loaded yet".
//
// Partial delivery is a normal outcome, not an error. A skill whose
// rendered body would push the shared object past the apiserver's 1 MiB
// ceiling cannot be delivered no matter how the pass is ordered, so the
// rest are delivered, the remainder is reported, and the next pass
// retries. Returning an error would make every caller treat a healthy
// pass as a failed one.
func (r *Reconciler) Converge(ctx context.Context, keep map[string]*Skill) (delivered []string, undelivered map[string]error, err error) {
	undelivered = map[string]error{}
	name := sharedskills.ConfigMapName
	var cm corev1.ConfigMap
	getErr := r.client.Get(ctx, types.NamespacedName{Name: name, Namespace: r.namespace}, &cm)
	if apierrors.IsNotFound(getErr) {
		if len(keep) == 0 {
			return nil, undelivered, nil
		}
		// Seed an empty object and let the update path below fill it; a
		// single Create carrying every skill would hit the same ceiling
		// this pass exists to work around.
		if err := r.client.Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: r.namespace,
				Labels: map[string]string{
					"ainsel.dev/managed-by": "hub",
					"ainsel.dev/resource":   "skills",
				},
			},
			Data: map[string]string{},
		}); err != nil && !apierrors.IsAlreadyExists(err) {
			return nil, undelivered, fmt.Errorf("create configmap %s: %w", name, err)
		}
		if err := r.client.Get(ctx, types.NamespacedName{Name: name, Namespace: r.namespace}, &cm); err != nil {
			return nil, undelivered, fmt.Errorf("get configmap %s after create: %w", name, err)
		}
	} else if getErr != nil {
		return nil, undelivered, fmt.Errorf("get configmap %s: %w", name, getErr)
	}

	changed := false
	for key := range cm.Data {
		if _, want := keep[key]; !want {
			delete(cm.Data, key)
			changed = true
		}
	}
	if changed {
		if err := r.client.Update(ctx, &cm); err != nil {
			return nil, undelivered, fmt.Errorf("prune configmap %s: %w", name, err)
		}
	}

	for id, sk := range keep {
		// Skip what is already correct. Every write to the shared object
		// changes it, the operator hashes it, and agents restart on a new
		// hash — so a pass that rewrote unchanged skills would cycle every
		// skill-bearing pod for no reason. A steady-state pass must be a
		// no-op on the object.
		if existing, ok := cm.Data[id]; ok && existing == assembleSKILLMD(sk) {
			delivered = append(delivered, id)
			continue
		}
		// Ensure re-reads the object, so a stale resourceVersion from the
		// prune above costs a retry rather than a lost write.
		if err := r.Ensure(ctx, sk); err != nil {
			undelivered[id] = err
			continue
		}
		delivered = append(delivered, id)
	}
	return delivered, undelivered, nil
}
