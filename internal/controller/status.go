package controller

import (
	"context"
	"fmt"

	labelsv1alpha1 "github.com/sbahar619/namespace-label-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
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

// updateSuccessStatus updates the CR status after successful reconciliation
func (r *NamespaceLabelReconciler) updateSuccessStatus(ctx context.Context, cr *labelsv1alpha1.NamespaceLabel, desired, actuallyDesired map[string]string, protectionResult ProtectionResult, targetNS string) error {
	l := log.FromContext(ctx)

	labelCount := len(desired)
	appliedCount := len(actuallyDesired)
	skippedCount := len(protectionResult.ProtectedSkipped)

	var msg string
	if skippedCount > 0 {
		msg = fmt.Sprintf("Applied %d labels to namespace '%s', skipped %d protected labels (%v)",
			appliedCount, targetNS, skippedCount, protectionResult.ProtectedSkipped)
	} else {
		msg = fmt.Sprintf("Applied %d labels to namespace '%s'",
			appliedCount, targetNS)
	}

	appliedKeys := make([]string, 0, len(actuallyDesired))
	for k := range actuallyDesired {
		appliedKeys = append(appliedKeys, k)
	}

	l.Info("NamespaceLabel successfully processed",
		"namespace", cr.Namespace, "labelsApplied", appliedCount, "labelsRequested", labelCount, "protectedSkipped", skippedCount)

	updateStatus(cr, true, "Synced", msg, protectionResult.ProtectedSkipped, appliedKeys)
	return r.Status().Update(ctx, cr)
}
