package controller

import (
	labelsv1alpha1 "github.com/sbahar619/namespace-label-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func updateStatus(cr *labelsv1alpha1.NamespaceLabel, ok bool, reason, msg string, protectedSkipped, labelsApplied []string) {
	cr.Status.Applied = ok
	cr.Status.ProtectedLabelsSkipped = protectedSkipped
	cr.Status.LabelsApplied = labelsApplied

	// Update condition
	cond := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             reason,
		Message:            msg,
		ObservedGeneration: cr.Generation,
		LastTransitionTime: metav1.Now(),
	}
	if !ok {
		cond.Status = metav1.ConditionFalse
	}

	// Replace existing Ready condition or add new one
	for i := range cr.Status.Conditions {
		if cr.Status.Conditions[i].Type == "Ready" {
			cr.Status.Conditions[i] = cond
			return
		}
	}
	cr.Status.Conditions = append(cr.Status.Conditions, cond)
}

// updateStatusWithProtection is deprecated - use updateStatus instead
func updateStatusWithProtection(cr *labelsv1alpha1.NamespaceLabel, ok bool, reason, msg string, protectedSkipped, labelsApplied []string) {
	updateStatus(cr, ok, reason, msg, protectedSkipped, labelsApplied)
}
