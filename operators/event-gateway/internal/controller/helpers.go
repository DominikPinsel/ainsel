package controller

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ptrString returns a pointer to the given string value.
func ptrString(s string) *string { return &s }

// findCondition returns the first condition of the given type from the slice,
// or nil if no matching condition is found.
func findCondition(conditions []metav1.Condition, condType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == condType {
			return &conditions[i]
		}
	}
	return nil
}
