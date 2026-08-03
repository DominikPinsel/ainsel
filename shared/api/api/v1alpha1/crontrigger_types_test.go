package v1alpha1

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestCronTriggerDeepCopy(t *testing.T) {
	enabled := false
	src := &CronTrigger{
		ObjectMeta: metav1.ObjectMeta{Name: "ct", Namespace: "ns"},
		Spec: CronTriggerSpec{
			DisplayName: "Daily",
			AgentRef:    "bot",
			Schedule:    "0 9 * * 1-5",
			Prompt:      "do the thing",
			Enabled:     &enabled,
		},
		Status: CronTriggerStatus{
			Conditions: []metav1.Condition{
				{Type: CronTriggerConditionAgentRefValid, Status: metav1.ConditionTrue, Reason: "AgentExists"},
				{Type: CronTriggerConditionScheduleValid, Status: metav1.ConditionTrue, Reason: "ScheduleValid"},
				{Type: CronTriggerConditionReady, Status: metav1.ConditionTrue, Reason: "Ready"},
			},
			ObservedGeneration: 1,
		},
	}

	dst := src.DeepCopy()
	if dst == src {
		t.Fatal("DeepCopy returned same pointer")
	}
	if dst.Spec.AgentRef != "bot" || dst.Spec.Schedule != "0 9 * * 1-5" || dst.Spec.Prompt != "do the thing" {
		t.Fatalf("spec not copied: %+v", dst.Spec)
	}
	if dst.Spec.Enabled == nil || *dst.Spec.Enabled != false {
		t.Fatalf("Enabled pointer not copied: %+v", dst.Spec.Enabled)
	}
	// Mutating the copy's Enabled must not affect the source.
	*dst.Spec.Enabled = true
	if enabled != false {
		t.Fatal("Enabled pointer was aliased, not deep-copied")
	}
	if len(dst.Status.Conditions) != 3 {
		t.Fatalf("conditions not copied: %d", len(dst.Status.Conditions))
	}
	dst.Status.Conditions[0].Reason = "mutated"
	if src.Status.Conditions[0].Reason != "AgentExists" {
		t.Fatal("conditions slice was aliased, not deep-copied")
	}
}

func TestCronTriggerListDeepCopy(t *testing.T) {
	src := &CronTriggerList{
		Items: []CronTrigger{
			{ObjectMeta: metav1.ObjectMeta{Name: "a"}},
			{ObjectMeta: metav1.ObjectMeta{Name: "b"}},
		},
	}
	dst := src.DeepCopy()
	if len(dst.Items) != 2 || dst.Items[0].Name != "a" {
		t.Fatalf("list items not copied: %+v", dst.Items)
	}
	// Mutate copy and ensure independence.
	dst.Items[0].Spec.AgentRef = "x"
	if src.Items[0].Spec.AgentRef == "x" {
		t.Fatal("list items were aliased, not deep-copied")
	}
}

func TestCronTriggerImplementsRuntimeObject(t *testing.T) {
	var _ runtime.Object = &CronTrigger{}
	var _ runtime.Object = &CronTriggerList{}
}
