package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DominikPinsel/ainsel/shared/api"
)

type TriggerSpec struct {
	DisplayName  string                   `json:"displayName"`
	AgentRef     string                   `json:"agentRef"`
	ConnectorRef string                   `json:"connectorRef"`
	Filters      []ainselapishared.Filter `json:"filters,omitempty"`
}

type TriggerStatus struct {
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
}

const (
	TriggerConditionAgentRefValid     = "AgentRefValid"
	TriggerConditionConnectorRefValid = "ConnectorRefValid"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Agent",type=string,JSONPath=`.spec.agentRef`
// +kubebuilder:printcolumn:name="Connector",type=string,JSONPath=`.spec.connectorRef`
// +kubebuilder:printcolumn:name="Agent Valid",type=string,JSONPath=`.status.conditions[?(@.type=="AgentRefValid")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

type Trigger struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TriggerSpec   `json:"spec,omitempty"`
	Status TriggerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

type TriggerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Trigger `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Trigger{}, &TriggerList{})
}
