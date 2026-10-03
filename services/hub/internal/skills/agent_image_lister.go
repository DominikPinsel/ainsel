package skills

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// kubeAgentImageLister implements AgentImageLister by reading AgentImage
// CRs from Kubernetes via the controller-runtime client. Reads
// spec.enabledSkills from each CR and matches against the requested skill ID.
type kubeAgentImageLister struct {
	client    ctrlclient.Client
	namespace string
}

// NewAgentImageLister wires a production AgentImageLister against the
// hub's K8s client and namespace.
func NewAgentImageLister(c ctrlclient.Client, namespace string) AgentImageLister {
	return &kubeAgentImageLister{client: c, namespace: namespace}
}

func (l *kubeAgentImageLister) ListReferrers(ctx context.Context, skillID string) ([]Referrer, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "ainsel.dev", Version: "v1alpha1", Kind: "AgentImageList",
	})
	if err := l.client.List(ctx, list, ctrlclient.InNamespace(l.namespace)); err != nil {
		return nil, err
	}
	var refs []Referrer
	for _, item := range list.Items {
		skills, found, err := unstructured.NestedStringSlice(item.Object, "spec", "enabledSkills")
		if err != nil || !found {
			continue
		}
		for _, s := range skills {
			if s == skillID {
				refs = append(refs, Referrer{AgentImageName: item.GetName()})
				break
			}
		}
	}
	return refs, nil
}

// Assign adds skillID to the AgentImage CR's spec.enabledSkills list.
// Idempotent: no-op if the skill is already present.
func (l *kubeAgentImageLister) Assign(ctx context.Context, skillID, agentImageName string) error {
	return l.mutateEnabledSkills(ctx, agentImageName, func(skills []string) []string {
		for _, s := range skills {
			if s == skillID {
				return skills // already present
			}
		}
		return append(skills, skillID)
	})
}

// Unassign removes skillID from the AgentImage CR's spec.enabledSkills list.
// Idempotent: no-op if the skill is not present.
func (l *kubeAgentImageLister) Unassign(ctx context.Context, skillID, agentImageName string) error {
	return l.mutateEnabledSkills(ctx, agentImageName, func(skills []string) []string {
		out := skills[:0]
		for _, s := range skills {
			if s != skillID {
				out = append(out, s)
			}
		}
		return out
	})
}

// mutateEnabledSkills fetches the named AgentImage CR, applies the mutation
// function to spec.enabledSkills, and updates the CR if changed. The
// get-mutate-update cycle is wrapped in retry.RetryOnConflict so that
// concurrent assignments to the same CR are retried automatically.
func (l *kubeAgentImageLister) mutateEnabledSkills(ctx context.Context, agentImageName string, mutate func([]string) []string) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(schema.GroupVersionKind{
			Group: "ainsel.dev", Version: "v1alpha1", Kind: "AgentImage",
		})
		if err := l.client.Get(ctx, types.NamespacedName{Name: agentImageName, Namespace: l.namespace}, obj); err != nil {
			return fmt.Errorf("get agent image %q: %w", agentImageName, err)
		}

		existing, _, _ := unstructured.NestedStringSlice(obj.Object, "spec", "enabledSkills")
		updated := mutate(existing)

		// Compare lengths as a quick check; if same length and same content, skip update.
		if len(updated) == len(existing) {
			same := true
			for i := range updated {
				if updated[i] != existing[i] {
					same = false
					break
				}
			}
			if same {
				return nil // no change needed
			}
		}

		if err := unstructured.SetNestedStringSlice(obj.Object, updated, "spec", "enabledSkills"); err != nil {
			return fmt.Errorf("set enabledSkills: %w", err)
		}
		if err := l.client.Update(ctx, obj); err != nil {
			return fmt.Errorf("update agent image %q: %w", agentImageName, err)
		}
		return nil
	})
}

// UsageCounts lists all AgentImage CRs in the namespace once and
// tallies, per skill ID, how many CRs reference it via
// spec.enabledSkills. A CR that lists the same skill ID more than
// once counts once for that skill.
func (l *kubeAgentImageLister) UsageCounts(ctx context.Context) (map[string]int, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "ainsel.dev", Version: "v1alpha1", Kind: "AgentImageList",
	})
	if err := l.client.List(ctx, list, ctrlclient.InNamespace(l.namespace)); err != nil {
		return nil, err
	}
	counts := make(map[string]int)
	for _, item := range list.Items {
		skills, found, err := unstructured.NestedStringSlice(item.Object, "spec", "enabledSkills")
		if err != nil || !found {
			continue
		}
		tallyUnique(counts, skills)
	}
	return counts, nil
}

// EnabledSkillIDs tallies every skill ID that some pod is meant to
// project, across both levels of the skill selection.
//
// UsageCounts alone is not enough: Agent.spec.skills.items *replaces* the
// referenced image's enabledSkills rather than adding to it, so a skill
// chosen only on the Agent would look unclaimed and get pruned out of
// the ConfigMap the running agent mounts from.
//
// The result is a union, which can over-count — an Agent that narrows to
// a subset still sits on an image whose other skills other agents use.
// Over-delivering is the safe direction here; under-delivering silently
// costs an agent a skill.
func (l *kubeAgentImageLister) EnabledSkillIDs(ctx context.Context) (map[string]int, error) {
	counts, err := l.UsageCounts(ctx)
	if err != nil {
		return nil, err
	}
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "ainsel.dev", Version: "v1alpha1", Kind: "AgentList",
	})
	if err := l.client.List(ctx, list, ctrlclient.InNamespace(l.namespace)); err != nil {
		return nil, err
	}
	for _, item := range list.Items {
		// An absent spec.skills means "inherit the image", which
		// UsageCounts already covered.
		skills, found, err := unstructured.NestedStringSlice(item.Object, "spec", "skills", "items")
		if err != nil || !found {
			continue
		}
		tallyUnique(counts, skills)
	}
	return counts, nil
}

// tallyUnique adds each distinct id in ids to counts once.
func tallyUnique(counts map[string]int, ids []string) {
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		counts[id]++
	}
}
