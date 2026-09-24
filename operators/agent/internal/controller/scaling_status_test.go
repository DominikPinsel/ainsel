package controller

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	ainselv1alpha1 "github.com/DominikPinsel/ainsel/shared/api/api/v1alpha1"
)

// The hub and the operator both write Agent status, and the operator's pod count
// depends on the hub's drain signal staying intact. A whole-object update from the
// operator would collide with a hub write that landed after its read, so status is
// written as a merge patch over just the operator's fields. This pins that
// contract: a stale operator write must not resurrect the numbers the hub replaced.
func TestPatchStatusLeavesHubFieldsAlone(t *testing.T) {
	scheme := scalingFixtures()
	a := scalingAgent(&ainselv1alpha1.AgentScaling{Replicas: ptr.To(int32(2)), MinReplicas: ptr.To(int32(0))})
	a.Status.PendingTasks = 5
	a.Status.ActiveTasks = 2
	observed := metav1.NewTime(time.Now().Add(-time.Second))
	a.Status.QueueObservedAt = &observed

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(a).
		WithStatusSubresource(&ainselv1alpha1.Agent{}).
		Build()
	ctx := context.Background()
	key := types.NamespacedName{Namespace: "ns", Name: "worker"}

	// The operator reads the agent, as a reconcile would.
	operatorCopy := &ainselv1alpha1.Agent{}
	if err := c.Get(ctx, key, operatorCopy); err != nil {
		t.Fatalf("operator read: %v", err)
	}

	// The hub then republishes: the work finished, so the counts drop to zero.
	hubCopy := &ainselv1alpha1.Agent{}
	if err := c.Get(ctx, key, hubCopy); err != nil {
		t.Fatalf("hub read: %v", err)
	}
	hubCopy.Status.PendingTasks = 0
	hubCopy.Status.ActiveTasks = 0
	hubCopy.Status.QueueObservedAt = &metav1.Time{Time: time.Now()}
	if err := c.Status().Update(ctx, hubCopy); err != nil {
		t.Fatalf("hub status update: %v", err)
	}

	// The operator writes its own fields from the stale copy.
	operatorCopy.Status.Replicas = 7
	meta.SetStatusCondition(&operatorCopy.Status.Conditions, metav1.Condition{
		Type:               ainselv1alpha1.AgentConditionReady,
		Status:             metav1.ConditionTrue,
		Reason:             "AllReady",
		Message:            "ok",
		LastTransitionTime: metav1.Now(),
	})
	operatorCopy.Status.Scaling = &ainselv1alpha1.AgentScalingStatus{Mode: "queue", Desired: 2, Reason: "QueueDepth"}

	r := &AgentReconciler{Client: c, Scheme: scheme}
	if err := r.patchStatus(ctx, operatorCopy); err != nil {
		t.Fatalf("patchStatus with a concurrent hub write: %v", err)
	}

	var got ainselv1alpha1.Agent
	if err := c.Get(ctx, key, &got); err != nil {
		t.Fatalf("read back: %v", err)
	}
	// Operator-owned fields landed.
	if got.Status.Replicas != 7 {
		t.Errorf("status.replicas = %d, want the operator's 7", got.Status.Replicas)
	}
	if got.Status.Scaling == nil || got.Status.Scaling.Reason != "QueueDepth" {
		t.Errorf("status.scaling = %+v, want the operator's decision recorded", got.Status.Scaling)
	}
	if cond := meta.FindStatusCondition(got.Status.Conditions, ainselv1alpha1.AgentConditionReady); cond == nil {
		t.Error("operator condition was not written")
	}
	// Hub-owned fields survived the stale write.
	if got.Status.PendingTasks != 0 || got.Status.ActiveTasks != 0 {
		t.Errorf("queue counts = %d/%d, want the hub's 0/0; the operator resurrected values it read before the hub updated them",
			got.Status.PendingTasks, got.Status.ActiveTasks)
	}
}

// The same discipline must apply to the early status write in Reconcile, which
// fires when the referenced AgentImage is missing. Here the hub's write lands
// *after* the operator read, so the object the operator holds carries queue
// numbers that no longer exist in the cluster: a whole-object update would publish
// those stale counts back, and the operator would then scale pods on work that has
// already finished.
func TestReconcileMissingImageKeepsHubFields(t *testing.T) {
	// Consumed by the Get interceptor below. Reconcile runs inline here, so a
	// plain flag is enough to make the staleness happen once.
	staleOnce := true
	scheme := scalingFixtures()
	a := scalingAgent(&ainselv1alpha1.AgentScaling{Replicas: ptr.To(int32(1))})
	a.Spec.ImageRef.Name = "does-not-exist"
	a.Status.PendingTasks = 4
	a.Status.ActiveTasks = 1

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(a).
		WithStatusSubresource(&ainselv1alpha1.Agent{}).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, cl client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if err := cl.Get(ctx, key, obj, opts...); err != nil {
					return err
				}
				// Stale on the reconcile's first read only, so the assertions below
				// still see what the cluster actually holds.
				if ag, ok := obj.(*ainselv1alpha1.Agent); ok && ag.Name == "worker" && staleOnce {
					staleOnce = false
					ag.Status.PendingTasks = 99
					ag.Status.ActiveTasks = 99
				}
				return nil
			},
		}).
		Build()
	r := &AgentReconciler{Client: c, Scheme: scheme}

	res, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "ns", Name: "worker"},
	})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if res.RequeueAfter == 0 {
		t.Error("a missing image should be retried, not dropped")
	}

	var got ainselv1alpha1.Agent
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(a), &got); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.Status.PendingTasks != 4 || got.Status.ActiveTasks != 1 {
		t.Errorf("queue counts = %d/%d after a failed reconcile, want the cluster's 4/1 untouched; "+
			"the operator wrote back the counts it read before the hub updated them",
			got.Status.PendingTasks, got.Status.ActiveTasks)
	}
	cond := meta.FindStatusCondition(got.Status.Conditions, ainselv1alpha1.AgentConditionReady)
	if cond == nil || cond.Reason != "ImageNotFound" {
		t.Errorf("Ready condition = %+v, want reason ImageNotFound", cond)
	}
}

// updateStatus is the hot path: it runs on every reconcile, including the ones
// that scale pods. Writing it as a whole-object update would fail whenever the hub
// publishes between the operator's read and write, and a retry loop on a
// continuously busy agent would keep colliding.
func TestUpdateStatusLeavesHubFieldsAlone(t *testing.T) {
	scheme := scalingFixtures()
	a := scalingAgent(&ainselv1alpha1.AgentScaling{Replicas: ptr.To(int32(3)), MinReplicas: ptr.To(int32(0))})
	a.Status.PendingTasks = 6
	a.Status.ActiveTasks = 3
	a.Status.QueueObservedAt = &metav1.Time{Time: time.Now().Add(-time.Second)}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(a).
		WithStatusSubresource(&ainselv1alpha1.Agent{}).
		Build()
	ctx := context.Background()
	key := types.NamespacedName{Namespace: "ns", Name: "worker"}

	stale := &ainselv1alpha1.Agent{}
	if err := c.Get(ctx, key, stale); err != nil {
		t.Fatalf("operator read: %v", err)
	}

	hub := &ainselv1alpha1.Agent{}
	if err := c.Get(ctx, key, hub); err != nil {
		t.Fatalf("hub read: %v", err)
	}
	hub.Status.PendingTasks = 1
	hub.Status.ActiveTasks = 0
	if err := c.Status().Update(ctx, hub); err != nil {
		t.Fatalf("hub status update: %v", err)
	}

	deploy := existingDeployment(3)
	deploy.Status.ReadyReplicas = 3
	r := &AgentReconciler{Client: c, Scheme: scheme}
	decision := r.resolveReplicas(stale, 3, time.Now())
	if err := r.updateStatus(ctx, stale, deploy, decision); err != nil {
		t.Fatalf("updateStatus with a concurrent hub write: %v", err)
	}

	var got ainselv1alpha1.Agent
	if err := c.Get(ctx, key, &got); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.Status.PendingTasks != 1 || got.Status.ActiveTasks != 0 {
		t.Errorf("queue counts = %d/%d, want the hub's 1/0 to survive the operator's write",
			got.Status.PendingTasks, got.Status.ActiveTasks)
	}
	if got.Status.Scaling == nil || got.Status.Scaling.Desired != decision.Replicas {
		t.Errorf("status.scaling = %+v, want the operator's decision (desired %d)",
			got.Status.Scaling, decision.Replicas)
	}
	if got.Status.Replicas != 3 {
		t.Errorf("status.replicas = %d, want the ready count 3", got.Status.Replicas)
	}
	if cond := meta.FindStatusCondition(got.Status.Conditions, ainselv1alpha1.AgentConditionDeploymentReady); cond == nil || cond.Status != metav1.ConditionTrue {
		t.Errorf("DeploymentReady condition = %+v, want True with 3/3 ready", cond)
	}
}
