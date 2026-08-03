package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CronTriggerSpec defines the desired state of a CronTrigger: a recurring
// schedule that delivers a fixed prompt to an agent. Unlike a webhook-driven
// Trigger, a CronTrigger has no connector — the hub emits a synthetic event
// on the schedule and enqueues a task for the agent
// (agent.<agentRef>), where one of the agent's replicas pulls and processes it.
type CronTriggerSpec struct {
	// DisplayName is the user-facing name of the cron trigger.
	DisplayName string `json:"displayName"`

	// AgentRef is the name of the Agent that should receive the scheduled prompt.
	AgentRef string `json:"agentRef"`

	// Schedule is a standard 5-field cron expression
	// (minute hour day-of-month month day-of-week, in the hub's local time).
	// Example: "0 9 * * 1-5" fires at 09:00 on weekdays.
	Schedule string `json:"schedule"`

	// Prompt is the text delivered to the agent as the user message on each
	// fire. It is sent verbatim — the agent runtime renders it without the
	// forgejo event template used for webhook events.
	Prompt string `json:"prompt"`

	// Enabled controls whether the schedule is active. When false (or unset,
	// which defaults to enabled), the hub does not fire the trigger. This
	// allows pausing a schedule without deleting it.
	// +kubebuilder:default:=true
	Enabled *bool `json:"enabled,omitempty"`
}

// CronTriggerStatus defines the observed state of a CronTrigger.
type CronTriggerStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastRun is the last time the cron trigger fired and published an event.
	// +optional
	LastRun *metav1.Time `json:"lastRun,omitempty"`

	// NextRun is the next scheduled fire time computed by the hub.
	// +optional
	NextRun *metav1.Time `json:"nextRun,omitempty"`

	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// Condition types for CronTrigger.
const (
	CronTriggerConditionAgentRefValid  = "AgentRefValid"
	CronTriggerConditionScheduleValid  = "ScheduleValid"
	CronTriggerConditionReady          = "Ready"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Agent",type=string,JSONPath=`.spec.agentRef`
// +kubebuilder:printcolumn:name="Schedule",type=string,JSONPath=`.spec.schedule`
// +kubebuilder:printcolumn:name="Enabled",type=boolean,JSONPath=`.spec.enabled`
// +kubebuilder:printcolumn:name="Agent Valid",type=string,JSONPath=`.status.conditions[?(@.type=="AgentRefValid")].status`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Next Run",type=date,JSONPath=`.status.nextRun`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// CronTrigger is the Schema for the crontriggers API. It schedules a prompt
// to be delivered to an agent on a recurring cron schedule.
type CronTrigger struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CronTriggerSpec   `json:"spec,omitempty"`
	Status CronTriggerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CronTriggerList contains a list of CronTrigger.
type CronTriggerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CronTrigger `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CronTrigger{}, &CronTriggerList{})
}
