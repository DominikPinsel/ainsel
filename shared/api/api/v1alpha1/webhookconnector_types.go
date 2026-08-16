package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SecretKeyRef references a key in a Secret.
type SecretKeyRef struct {
	SecretRef SecretRef `json:"secretRef"`
}

// SecretRef identifies a Secret and key.
type SecretRef struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

// ConnectorImage specifies the container image.
type ConnectorImage struct {
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
}

const (
	ConnectorConditionReady    = "Ready"
	ConnectorConditionDisabled = "Disabled"
)

// WebhookConnectorSpec defines the desired state of a WebhookConnector.
type WebhookConnectorSpec struct {
	// DisplayName is the user-facing name.
	DisplayName string `json:"displayName,omitempty"`
	// WebhookEndpoint is the URL to paste into GitHub/Forgejo webhook settings.
	// Set by the hub when creating the connector; read-only thereafter.
	WebhookEndpoint string `json:"webhookEndpoint"`
	// SignatureHeader is the HTTP header the sender puts the HMAC-SHA256
	// signature in. E.g. "X-Hub-Signature-256" for GitHub,
	// "X-Forgejo-Signature" for Forgejo.
	SignatureHeader string `json:"signatureHeader"`
	// WebhookSecret references the K8s Secret holding the HMAC key (key: "secret").
	WebhookSecret SecretKeyRef `json:"webhookSecret"`
	// Image for the webhook-receiver container.
	Image ConnectorImage `json:"image"`
	// Disabled scales the Deployment to zero when true.
	// +optional
	Disabled bool `json:"disabled,omitempty"`
}

// WebhookConnectorStatus defines the observed state of a WebhookConnector.
type WebhookConnectorStatus struct {
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Endpoint",type=string,JSONPath=`.spec.webhookEndpoint`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Disabled",type=boolean,JSONPath=`.spec.disabled`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// WebhookConnector is the Schema for the webhookconnectors API.
type WebhookConnector struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WebhookConnectorSpec   `json:"spec,omitempty"`
	Status WebhookConnectorStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// WebhookConnectorList contains a list of WebhookConnector.
type WebhookConnectorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WebhookConnector `json:"items"`
}

