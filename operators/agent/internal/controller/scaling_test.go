package controller

import (
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	ainselv1alpha1 "github.com/DominikPinsel/ainsel/shared/api/api/v1alpha1"
)

func agentWith(min, max *int32) *ainselv1alpha1.Agent {
	a := &ainselv1alpha1.Agent{}
	a.Name = "agent"
	if max != nil || min != nil {
		a.Spec.Scaling = &ainselv1alpha1.AgentScaling{Replicas: max, MinReplicas: min}
	}
	return a
}

// observed stamps a fresh queue signal onto the agent.
func observed(a *ainselv1alpha1.Agent, pending, active int32, at time.Time) *ainselv1alpha1.Agent {
	a.Status.PendingTasks = pending
	a.Status.ActiveTasks = active
	a.Status.QueueObservedAt = &metav1.Time{Time: at}
	return a
}

func invoked(a *ainselv1alpha1.Agent, at time.Time) *ainselv1alpha1.Agent {
	a.Status.LastInvocation = &metav1.Time{Time: at}
	return a
}

// Fixed clock so the idle-window cases are exact rather than timing-dependent.
var clock = time.Date(2026, 4, 5, 12, 0, 0, 0, time.UTC)

func fresh(pending, active int32) *ainselv1alpha1.Agent {
	return observed(agentWith(ptr.To(int32(0)), ptr.To(int32(5))), pending, active, clock.Add(-time.Second))
}

func TestResolveReplicas(t *testing.T) {
	r := &AgentReconciler{}

	tests := []struct {
		name        string
		agent       *ainselv1alpha1.Agent
		current     int32
		wantReplica int32
		wantReason  string
		// wantMessageIs is an optional substring the message must carry.
		wantMessageIs string
		wantRequeue   time.Duration
	}{
		// --- static mode: the compatibility contract ---
		{
			name:        "no scaling block keeps one pod and ignores a full queue",
			agent:       observed(agentWith(nil, nil), 40, 3, clock.Add(-time.Second)),
			wantReplica: 1,
			wantReason:  ScaleReasonStatic,
		},
		{
			name:        "replicas without minReplicas is still static",
			agent:       observed(agentWith(nil, ptr.To(int32(3))), 50, 0, clock.Add(-time.Second)),
			wantReplica: 3,
			wantReason:  ScaleReasonStatic,
		},
		{
			name:        "static agent stays at its count with no work at all",
			agent:       observed(agentWith(nil, ptr.To(int32(2))), 0, 0, clock.Add(-time.Second)),
			wantReplica: 2,
			wantReason:  ScaleReasonStatic,
		},

		// --- opted-in, work present ---
		{
			name:        "queue depth drives pod count",
			agent:       fresh(3, 1),
			wantReplica: 4,
			wantReason:  ScaleReasonQueued,
		},
		{
			name:        "one task wakes a dormant agent",
			agent:       fresh(1, 0),
			current:     0,
			wantReplica: 1,
			wantReason:  ScaleReasonQueued,
		},
		{
			name:          "backlog is capped at the ceiling",
			agent:         fresh(9, 1),
			wantReplica:   5,
			wantReason:    ScaleReasonQueued,
			wantMessageIs: "capped at 5",
		},
		{
			name:        "the floor outranks a shallow queue",
			agent:       observed(agentWith(ptr.To(int32(2)), ptr.To(int32(5))), 1, 0, clock.Add(-time.Second)),
			wantReplica: 2,
			wantReason:  ScaleReasonQueued,
		},
		{
			name:        "in-flight tasks are never scaled away mid-work",
			agent:       observed(agentWith(ptr.To(int32(0)), ptr.To(int32(5))), 0, 3, clock.Add(-time.Second)),
			current:     3,
			wantReplica: 3,
			wantReason:  ScaleReasonQueued,
		},
		{
			name:        "more in-flight work than the ceiling still cannot exceed it",
			agent:       observed(agentWith(ptr.To(int32(0)), ptr.To(int32(2))), 0, 4, clock.Add(-time.Second)),
			wantReplica: 2,
			wantReason:  ScaleReasonQueued,
		},

		// --- opted-in, quiet ---
		{
			name:        "quiet inside the grace window keeps one warm",
			agent:       invoked(observed(agentWith(ptr.To(int32(0)), ptr.To(int32(5))), 0, 0, clock.Add(-time.Second)), clock.Add(-30*time.Second)),
			current:     4,
			wantReplica: 1,
			wantReason:  ScaleReasonIdleGrace,
			wantRequeue: defaultScaleDownWindow - 30*time.Second,
		},
		{
			name:        "quiet past the grace window sleeps",
			agent:       invoked(observed(agentWith(ptr.To(int32(0)), ptr.To(int32(5))), 0, 0, clock.Add(-time.Second)), clock.Add(-defaultScaleDownWindow)),
			current:     1,
			wantReplica: 0,
			wantReason:  ScaleReasonScaledDown,
		},
		{
			name:        "a nonzero floor never sleeps",
			agent:       invoked(observed(agentWith(ptr.To(int32(2)), ptr.To(int32(5))), 0, 0, clock.Add(-time.Second)), clock.Add(-time.Hour)),
			current:     2,
			wantReplica: 2,
			wantReason:  ScaleReasonDormant,
		},
		{
			name:        "an agent that has never been handed work sleeps",
			agent:       observed(agentWith(ptr.To(int32(0)), ptr.To(int32(5))), 0, 0, clock.Add(-time.Second)),
			current:     2,
			wantReplica: 0,
			wantReason:  ScaleReasonDormant,
		},
		{
			// The clock only moves when work arrives, so a burst that has since
			// drained must still wait out the window from its last task.
			name:        "quiet is measured from the last task, not the last pod",
			agent:       invoked(observed(agentWith(ptr.To(int32(0)), ptr.To(int32(5))), 0, 0, clock.Add(-time.Second)), clock.Add(-91*time.Second)),
			current:     1,
			wantReplica: 1,
			wantReason:  ScaleReasonIdleGrace,
			wantRequeue: 29 * time.Second, // the rest of the window
		},

		{
			// An agent with no pods that just went quiet must stay at zero. Raising
			// it to the grace-period "keep one warm" would create a pod only to
			// delete it a minute later, on every quiet spell.
			name:        "grace period does not wake an agent that has no pods",
			agent:       invoked(observed(agentWith(ptr.To(int32(0)), ptr.To(int32(3))), 0, 0, clock.Add(-time.Second)), clock.Add(-10*time.Second)),
			current:     0,
			wantReplica: 0,
			wantReason:  ScaleReasonIdleGrace,
			wantRequeue: defaultScaleDownWindow - 10*time.Second,
		},

		// --- untrusted signal: the fail-open contract ---
		{
			name:        "a stale signal holds the running pods instead of sleeping",
			agent:       invoked(observed(agentWith(ptr.To(int32(0)), ptr.To(int32(5))), 0, 0, clock.Add(-10*time.Minute)), clock.Add(-10*time.Minute)),
			current:     3,
			wantReplica: 3,
			wantReason:  ScaleReasonQueueStale,
			wantRequeue: queueStaleRequeue,
		},
		{
			name:        "no signal at all cannot mean empty",
			agent:       agentWith(ptr.To(int32(0)), ptr.To(int32(5))),
			current:     2,
			wantReplica: 2,
			wantReason:  ScaleReasonQueueStale,
			wantRequeue: queueStaleRequeue,
		},
		{
			name:        "a fresh agent with a floor starts without waiting for a signal",
			agent:       agentWith(ptr.To(int32(2)), ptr.To(int32(5))),
			current:     0,
			wantReplica: 2,
			wantReason:  ScaleReasonQueueStale,
			wantRequeue: queueStaleRequeue,
		},
		{
			// A brand-new opted-in agent has no signal and no pods: staying at
			// zero is right, since the hub will publish the moment work arrives.
			name:        "a brand new dormant agent waits for its first signal",
			agent:       agentWith(ptr.To(int32(0)), ptr.To(int32(3))),
			current:     0,
			wantReplica: 0,
			wantReason:  ScaleReasonQueueStale,
			wantRequeue: queueStaleRequeue,
		},
		{
			name:        "a future-dated signal is not trusted",
			agent:       observed(agentWith(ptr.To(int32(0)), ptr.To(int32(5))), 0, 0, clock.Add(time.Hour)),
			current:     1,
			wantReplica: 1,
			wantReason:  ScaleReasonQueueStale,
			wantRequeue: queueStaleRequeue,
		},

		// --- degenerate specs ---
		{
			name:        "an explicit zero ceiling stays at zero",
			agent:       observed(agentWith(ptr.To(int32(0)), ptr.To(int32(0))), 5, 2, clock.Add(-time.Second)),
			wantReplica: 0,
			wantReason:  ScaleReasonDisabled,
		},
		{
			name:        "a floor above the ceiling is clamped down to it",
			agent:       observed(agentWith(ptr.To(int32(6)), ptr.To(int32(2))), 0, 0, clock.Add(-time.Second)),
			current:     2,
			wantReplica: 2,
			wantReason:  ScaleReasonDormant,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := r.resolveReplicas(tc.agent, tc.current, clock)
			if got.Replicas != tc.wantReplica {
				t.Errorf("replicas = %d, want %d (reason %q, message %q)",
					got.Replicas, tc.wantReplica, got.Reason, got.Message)
			}
			if got.Reason != tc.wantReason {
				t.Errorf("reason = %q, want %q", got.Reason, tc.wantReason)
			}
			if tc.wantMessageIs != "" && !strings.Contains(got.Message, tc.wantMessageIs) {
				t.Errorf("message = %q, want it to mention %q", got.Message, tc.wantMessageIs)
			}
			if tc.wantRequeue != got.RequeueAfter {
				t.Errorf("requeue = %v, want %v", got.RequeueAfter, tc.wantRequeue)
			}
		})
	}
}

// The idle grace window is configurable, and the decision must actually use it.
func TestResolveReplicasHonoursConfiguredWindows(t *testing.T) {
	r := &AgentReconciler{ScaleDownWindow: 10 * time.Second}
	a := invoked(observed(agentWith(ptr.To(int32(0)), ptr.To(int32(3))), 0, 0, clock.Add(-time.Second)), clock.Add(-30*time.Second))
	if got := r.resolveReplicas(a, 1, clock); got.Replicas != 0 {
		t.Errorf("with a 10s window and 30s of quiet: replicas = %d, want 0", got.Replicas)
	}

	r2 := &AgentReconciler{QueueSignalTTL: time.Minute}
	stale := observed(agentWith(ptr.To(int32(0)), ptr.To(int32(3))), 0, 0, clock.Add(-2*time.Minute))
	if got := r2.resolveReplicas(stale, 2, clock); got.Reason != ScaleReasonQueueStale {
		t.Errorf("with a 1m TTL and a 2m-old signal: reason = %q, want %q", got.Reason, ScaleReasonQueueStale)
	}
}

// Defaults must be applied, not assumed: a zero-valued reconciler is what the
// chart produces when no flags are set.
func TestReconcilerScalingDefaults(t *testing.T) {
	r := &AgentReconciler{}
	if got := r.scaleDownWindow(); got != defaultScaleDownWindow {
		t.Errorf("scaleDownWindow() = %v, want %v", got, defaultScaleDownWindow)
	}
	if got := r.queueSignalTTL(); got != defaultQueueSignalTTL {
		t.Errorf("queueSignalTTL() = %v, want %v", got, defaultQueueSignalTTL)
	}
	if defaultQueueSignalTTL <= 60*time.Second {
		t.Errorf("queue signal TTL %v must exceed the hub's 60s publish sweep", defaultQueueSignalTTL)
	}
}

// quietFor must not hand the scaler a negative idle period when the hub's clock
// runs ahead of the operator's.
func TestQuietForClockSkew(t *testing.T) {
	a := invoked(agentWith(ptr.To(int32(0)), ptr.To(int32(3))), clock.Add(time.Minute))
	idle, known := quietFor(a, clock)
	if !known {
		t.Fatal("quietFor() reported no idle clock for a future-dated invocation")
	}
	if idle < 0 {
		t.Errorf("idle = %v, want no negative idle period", idle)
	}
}
