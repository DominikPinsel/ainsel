package controller

import (
	"strconv"
	"time"

	ainselv1alpha1 "github.com/DominikPinsel/ainsel/shared/api/api/v1alpha1"
)

// Scale reasons double as the diagnostics the UI and operators read, so they are
// worth keeping distinct even when two of them produce the same pod count.
const (
	ScaleReasonStatic     = "Static"
	ScaleReasonQueued     = "QueueDepth"
	ScaleReasonDormant    = "Dormant"
	ScaleReasonIdleGrace  = "IdleGrace"
	ScaleReasonQueueStale = "QueueSignalStale"
	ScaleReasonScaledDown = "ScaledToZero"
	ScaleReasonDisabled   = "Disabled"
)

// Defaults for the queue-driven mode. IdleGrace exists because the cost of
// scaling to zero too eagerly is not a spare pod, it is the next request paying a
// cold boot: a chat message that arrives right after an agent finishes its work
// would otherwise wait for the pod to be created, scheduled and started again.
// QueueSignalTTL must comfortably exceed the hub's publish sweep (60s), or a
// healthy hub looks stale under normal scheduling jitter.
const (
	defaultScaleDownWindow = 2 * time.Minute
	defaultQueueSignalTTL  = 5 * time.Minute
	// queueStaleRequeue is how soon the operator looks again when it has no
	// usable signal, so a hub that comes back to life is picked up without
	// waiting out the full reconcile period.
	queueStaleRequeue = 15 * time.Second
	// clockSkewTolerance is how far ahead of this clock the hub may appear to be
	// and still be believed. A measurement stamped in the future would otherwise
	// look fresh forever, since its age never exceeds the TTL.
	clockSkewTolerance = 30 * time.Second
)

// ScaleDecision is the operator's answer to "how many pods should this agent
// have right now", plus the reasoning that made it so.
type ScaleDecision struct {
	Replicas int32
	Reason   string
	Message  string
	// RequeueAfter asks to be reconsidered sooner than the normal period. Zero
	// means the normal period is fine.
	RequeueAfter time.Duration
}

// scaleDownWindow returns the configured idle grace or the default.
func (r *AgentReconciler) scaleDownWindow() time.Duration {
	if r.ScaleDownWindow > 0 {
		return r.ScaleDownWindow
	}
	return defaultScaleDownWindow
}

// queueSignalTTL returns how long a published queue signal stays trustworthy.
func (r *AgentReconciler) queueSignalTTL() time.Duration {
	if r.QueueSignalTTL > 0 {
		return r.QueueSignalTTL
	}
	return defaultQueueSignalTTL
}

// maxReplicas is the ceiling for an agent: spec.scaling.replicas, defaulting to
// one. Its meaning predates scale-to-zero as the pinned count; for agents that
// opt into queue-driven scaling it becomes the upper bound.
func maxReplicas(agent *ainselv1alpha1.Agent) int32 {
	if agent.Spec.Scaling != nil && agent.Spec.Scaling.Replicas != nil {
		return *agent.Spec.Scaling.Replicas
	}
	return 1
}

// minReplicas resolves the scaling floor. The second return reports whether the
// agent opted into queue-driven scaling at all: an unset minReplicas means keep
// exactly maxReplicas pods, which is how every agent behaved before this existed.
func minReplicas(agent *ainselv1alpha1.Agent) (int32, bool) {
	if agent.Spec.Scaling == nil || agent.Spec.Scaling.MinReplicas == nil {
		return 0, false
	}
	m := *agent.Spec.Scaling.MinReplicas
	if m < 0 {
		m = 0
	}
	return m, true
}

// resolveReplicas decides the pod count for one agent.
//
// current is the replica count the Deployment has now. It matters for exactly one
// case: when the queue signal cannot be trusted, the safe move is to leave the
// agent as it is rather than act on a number nobody is standing behind.
//
// The modes, in the order they are decided:
//
//  1. No minReplicas: static. Nothing about queue depth is consulted, so agents
//     that did not opt in cannot change behaviour.
//  2. maxReplicas 0: disabled. An explicit zero is a user saying "none", and no
//     amount of queued work should overrule that.
//  3. Stale or missing signal: hold at whatever is running, clamped up to the
//     floor. Never scale to zero on an unproven claim of emptiness.
//  4. Work waiting or in flight: one pod per task, bounded by the ceiling. A
//     runtime claims exactly one task at a time, so depth is also the concurrency
//     need; and pods holding claimed tasks are included, because losing one does
//     not lose the task but parks it until the reaper gives up on the claim.
//  5. Quiet, but not quiet long enough: shed the extra pods down to one warm
//     container, keep it, and come back for the last one when the window closes.
//  6. Quiet past the window: the floor, which is zero for a dormant agent.
func (r *AgentReconciler) resolveReplicas(agent *ainselv1alpha1.Agent, current int32, now time.Time) ScaleDecision {
	max := maxReplicas(agent)
	minP, optedIn := minReplicas(agent)

	if !optedIn {
		return ScaleDecision{
			Replicas: max,
			Reason:   ScaleReasonStatic,
			Message:  "static replica count",
		}
	}
	if minP > max {
		// Rejected by the API and the validating webhook; clamped here so a
		// slipped-through object cannot ask for more pods than its own ceiling.
		minP = max
	}
	if max == 0 {
		return ScaleDecision{
			Replicas: 0,
			Reason:   ScaleReasonDisabled,
			Message:  "replicas set to 0",
		}
	}

	pending, active, fresh := queueSignal(agent, now, r.queueSignalTTL())
	if !fresh {
		target := current
		if target < minP {
			target = minP
		}
		if target > max {
			target = max
		}
		return ScaleDecision{
			Replicas:     target,
			Reason:       ScaleReasonQueueStale,
			Message:      "no recent queue signal from the hub; holding instead of scaling to zero",
			RequeueAfter: queueStaleRequeue,
		}
	}

	work := pending + active
	if work > 0 {
		// No floor bump for the single-pod case: work >= 1 here, so clamping into
		// [minP, max] already puts at least one pod on.
		target := clampInt32(work, minP, max)
		return ScaleDecision{
			Replicas: target,
			Reason:   ScaleReasonQueued,
			Message:  queuedMessage(target, pending, active, max),
		}
	}

	// Nothing waiting and nothing in flight: how long has it been like that?
	window := r.scaleDownWindow()
	idle, known := quietFor(agent, now)
	if !known {
		return ScaleDecision{
			Replicas: minP,
			Reason:   ScaleReasonDormant,
			Message:  "no work has ever been queued for this agent",
		}
	}
	if idle >= window {
		reason := ScaleReasonDormant
		if minP == 0 {
			reason = ScaleReasonScaledDown
		}
		return ScaleDecision{
			Replicas: minP,
			Reason:   reason,
			Message:  "quiet for " + idle.Round(time.Second).String(),
		}
	}

	// Inside the grace window: drop the burst capacity immediately, keep one
	// container warm for the tail of the window, and recheck when it closes.
	hold := int32(1)
	if minP > hold {
		hold = minP
	}
	if current < hold {
		hold = current
	}
	return ScaleDecision{
		Replicas:     hold,
		Reason:       ScaleReasonIdleGrace,
		Message:      "quiet, holding one pod for " + (window - idle).Round(time.Second).String(),
		RequeueAfter: window - idle,
	}
}

// queuedMessage explains the pod count a busy agent got, in terms of what the
// ceiling did.
func queuedMessage(target, pending, active, max int32) string {
	msg := activeLabel(active) + ", " + pendingLabel(pending)
	if target >= max && pending+active > max {
		msg += "; capped at " + strconv.FormatInt(int64(max), 10) + " pods"
	}
	return msg
}

// queueSignal reads the hub's published measurement, reporting whether it is
// recent enough to act on. A nil timestamp means the hub has never published for
// this agent, which is treated the same as stale: absence of a signal is not
// evidence of an empty queue.
func queueSignal(agent *ainselv1alpha1.Agent, now time.Time, ttl time.Duration) (pending, active int32, fresh bool) {
	if agent.Status.QueueObservedAt == nil {
		return 0, 0, false
	}
	observed := agent.Status.QueueObservedAt.Time
	age := now.Sub(observed)
	if observed.IsZero() || age > ttl || age < -clockSkewTolerance {
		return 0, 0, false
	}
	return agent.Status.PendingTasks, agent.Status.ActiveTasks, true
}

// quietFor measures how long the agent has had nothing to do. The idle clock only
// moves when work arrives, so quiet is measured from the last task handed over.
// The second result is false when the agent has never been given work.
func quietFor(agent *ainselv1alpha1.Agent, now time.Time) (time.Duration, bool) {
	if agent.Status.LastInvocation == nil {
		return 0, false
	}
	stamp := agent.Status.LastInvocation.Time
	if stamp.IsZero() || stamp.After(now) {
		return 0, stamp.After(now)
	}
	return now.Sub(stamp), true
}

func clampInt32(v, low, high int32) int32 {
	if v < low {
		return low
	}
	if v > high {
		return high
	}
	return v
}

func activeLabel(n int32) string {
	switch n {
	case 0:
		return "0 tasks in flight"
	case 1:
		return "1 task in flight"
	default:
		return strconv.FormatInt(int64(n), 10) + " tasks in flight"
	}
}

func pendingLabel(n int32) string {
	switch n {
	case 0:
		return "none waiting"
	case 1:
		return "1 waiting"
	default:
		return strconv.FormatInt(int64(n), 10) + " waiting"
	}
}
