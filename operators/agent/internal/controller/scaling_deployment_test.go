package controller

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ainselv1alpha1 "github.com/DominikPinsel/ainsel/shared/api/api/v1alpha1"
)

// These tests drive reconcileDeployment rather than the pure resolver, so the
// wiring between a published queue signal and the pod count a Deployment actually
// asks for is covered. The resolver being right proves nothing if the controller
// keeps writing its own number.
//
// The image reference is digest-pinned on purpose: a mutable tag makes
// resolveImageDigest reach for a registry, which has no business in a unit test.

const pinnedImage = "local.test/ainsel-pi@sha256:0000000000000000000000000000000000000000000000000000000000000000"

func scalingFixtures() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(s))
	utilruntime.Must(appsv1.AddToScheme(s))
	utilruntime.Must(ainselv1alpha1.AddToScheme(s))
	return s
}

func imageObject() *ainselv1alpha1.AgentImage {
	img := &ainselv1alpha1.AgentImage{}
	img.Name = "img"
	img.Namespace = "ns"
	img.Spec = ainselv1alpha1.AgentImageSpec{
		DisplayName: "Test",
		ImageURL:    pinnedImage,
		Tools:       []ainselv1alpha1.AgentImageTool{{Name: "git"}},
	}
	return img
}

func scalingAgent(scaling *ainselv1alpha1.AgentScaling) *ainselv1alpha1.Agent {
	a := &ainselv1alpha1.Agent{}
	a.Name = "worker"
	a.Namespace = "ns"
	a.Spec = ainselv1alpha1.AgentSpec{
		DisplayName:  "Worker",
		ImageRef:     ainselv1alpha1.AgentImageRef{Name: "img"},
		Runtime:      ainselv1alpha1.AgentRuntime{},
		LLM:          ainselv1alpha1.AgentLLM{Model: "glm-5.1:cloud"},
		Persona:      ainselv1alpha1.AgentPersona{ID: "01hxtestpersona00000000000"},
		EnabledTools: []string{"git"},
		Scaling:      scaling,
	}
	return a
}

func queueStatus(a *ainselv1alpha1.Agent, pending, active int32, observed, invoked *time.Time) *ainselv1alpha1.Agent {
	if observed != nil {
		a.Status.QueueObservedAt = &metav1.Time{Time: *observed}
	}
	a.Status.PendingTasks = pending
	a.Status.ActiveTasks = active
	if invoked != nil {
		a.Status.LastInvocation = &metav1.Time{Time: *invoked}
	}
	return a
}

// deploymentFor runs the real Deployment reconciliation and returns the object it
// produced, so assertions are about what would be written to the cluster.
func deploymentFor(t *testing.T, agent *ainselv1alpha1.Agent, existing *appsv1.Deployment) *appsv1.Deployment {
	t.Helper()
	scheme := scalingFixtures()

	objs := []runtime.Object{imageObject(), agent}
	if existing != nil {
		objs = append(objs, existing)
	}
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(objs...).
		WithStatusSubresource(&ainselv1alpha1.Agent{}).
		Build()

	r := &AgentReconciler{Client: c, Scheme: scheme}
	img := &ainselv1alpha1.AgentImage{}
	if err := r.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "img"}, img); err != nil {
		t.Fatalf("load agent image: %v", err)
	}

	deploy, scale, _, err := r.reconcileDeployment(context.Background(), agent, "agent-worker", img, img.Spec.ImageURL)
	if err != nil {
		t.Fatalf("reconcileDeployment: %v", err)
	}
	if deploy == nil {
		t.Fatal("reconcileDeployment returned no Deployment")
	}
	t.Logf("decision: replicas=%d reason=%s message=%q", scale.Replicas, scale.Reason, scale.Message)
	return deploy
}

func existingDeployment(replicas int32) *appsv1.Deployment {
	d := &appsv1.Deployment{}
	d.Name = "agent-worker"
	d.Namespace = "ns"
	d.Spec = appsv1.DeploymentSpec{
		Replicas: ptr.To(replicas),
		Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app.kubernetes.io/name": "agent-worker"}},
		Template: corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "agent", Image: pinnedImage}}},
		},
	}
	return d
}

func ptrTo(t time.Time) *time.Time { return &t }

// The headline behaviour: an opted-in agent with an empty, freshly measured queue
// and no pods wanted ends up with a Deployment that asks for zero pods.
func TestReconcileDeploymentScalesToZeroWhenQuiet(t *testing.T) {
	now := time.Now()
	quiet := now.Add(-3 * defaultScaleDownWindow)
	a := queueStatus(scalingAgent(&ainselv1alpha1.AgentScaling{
		Replicas:    ptr.To(int32(3)),
		MinReplicas: ptr.To(int32(0)),
	}), 0, 0, ptrTo(now.Add(-time.Second)), ptrTo(quiet))

	deploy := deploymentFor(t, a, existingDeployment(2))
	if deploy.Spec.Replicas == nil || *deploy.Spec.Replicas != 0 {
		t.Errorf("Deployment replicas = %v, want 0 for a drained agent that opted in",
			deploy.Spec.Replicas)
	}
}

// The same agent with work waiting must be scaled up instead, one pod per task.
func TestReconcileDeploymentScalesUpForQueuedWork(t *testing.T) {
	now := time.Now()
	a := queueStatus(scalingAgent(&ainselv1alpha1.AgentScaling{
		Replicas:    ptr.To(int32(4)),
		MinReplicas: ptr.To(int32(0)),
	}), 3, 1, ptrTo(now.Add(-time.Second)), ptrTo(now.Add(-time.Minute)))

	deploy := deploymentFor(t, a, existingDeployment(1))
	if deploy.Spec.Replicas == nil || *deploy.Spec.Replicas != 4 {
		t.Errorf("Deployment replicas = %v, want 4 (3 waiting + 1 in flight)", deploy.Spec.Replicas)
	}
}

func TestReconcileDeploymentCapsAtTheCeiling(t *testing.T) {
	now := time.Now()
	a := queueStatus(scalingAgent(&ainselv1alpha1.AgentScaling{
		Replicas:    ptr.To(int32(2)),
		MinReplicas: ptr.To(int32(0)),
	}), 40, 0, ptrTo(now.Add(-time.Second)), ptrTo(now.Add(-time.Minute)))

	deploy := deploymentFor(t, a, existingDeployment(2))
	if deploy.Spec.Replicas == nil || *deploy.Spec.Replicas != 2 {
		t.Errorf("Deployment replicas = %v, want the ceiling 2", deploy.Spec.Replicas)
	}
}

// The compatibility contract, at the layer that would actually break it: an agent
// with no minReplicas keeps its standing count even with a queue full of work and
// an empty one.
func TestReconcileDeploymentLeavesStaticAgentsAlone(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name         string
		pending      int32
		wantReplicas int32
	}{
		{"busy queue", 25, 3},
		{"empty queue", 0, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := queueStatus(scalingAgent(&ainselv1alpha1.AgentScaling{
				Replicas: ptr.To(int32(3)),
			}), tc.pending, 0, ptrTo(now.Add(-time.Second)), ptrTo(now.Add(-time.Hour)))

			deploy := deploymentFor(t, a, existingDeployment(3))
			if deploy.Spec.Replicas == nil || *deploy.Spec.Replicas != tc.wantReplicas {
				t.Errorf("Deployment replicas = %v, want the pinned %d", deploy.Spec.Replicas, tc.wantReplicas)
			}
		})
	}
}

// The safety rule, end to end: with no usable measurement the operator keeps the
// pods it has rather than acting on an unproven empty queue.
func TestReconcileDeploymentHoldsOnStaleSignal(t *testing.T) {
	now := time.Now()
	stale := now.Add(-2 * defaultQueueSignalTTL)
	a := queueStatus(scalingAgent(&ainselv1alpha1.AgentScaling{
		Replicas:    ptr.To(int32(5)),
		MinReplicas: ptr.To(int32(0)),
	}), 0, 0, ptrTo(stale), ptrTo(now.Add(-time.Hour)))

	deploy := deploymentFor(t, a, existingDeployment(3))
	if deploy.Spec.Replicas == nil || *deploy.Spec.Replicas != 3 {
		t.Errorf("Deployment replicas = %v, want 3 held; an outdated signal is not permission to sleep",
			deploy.Spec.Replicas)
	}
}

// A fresh agent that opted into dormancy has no Deployment yet and no signal from
// the hub. It must come up at zero rather than the default one pod, otherwise the
// feature only ever halves the cost.
func TestReconcileDeploymentCreatesDormantAgentAtZero(t *testing.T) {
	now := time.Now()
	a := queueStatus(scalingAgent(&ainselv1alpha1.AgentScaling{
		Replicas:    ptr.To(int32(2)),
		MinReplicas: ptr.To(int32(0)),
	}), 0, 0, ptrTo(now), ptrTo(now.Add(-defaultScaleDownWindow*2)))

	deploy := deploymentFor(t, a, nil)
	if deploy.Spec.Replicas == nil || *deploy.Spec.Replicas != 0 {
		t.Errorf("new dormant agent Deployment replicas = %v, want 0", deploy.Spec.Replicas)
	}
}
