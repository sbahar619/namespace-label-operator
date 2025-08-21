package controller

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	labelsv1alpha1 "github.com/sbahar619/namespace-label-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

// applyLabelsToNamespace applies desired labels and removes stale ones
func (r *NamespaceLabelReconciler) applyLabelsToNamespace(ns *corev1.Namespace, actuallyDesired, prevApplied map[string]string) (bool, error) {
	changed := false

	// Remove stale operator-managed keys
	for k, prevVal := range prevApplied {
		if _, stillWanted := actuallyDesired[k]; !stillWanted {
			if cur, ok := ns.Labels[k]; ok && cur == prevVal {
				delete(ns.Labels, k)
				changed = true
			}
		}
	}

	// Apply/overwrite allowed keys only
	for k, v := range actuallyDesired {
		if cur, ok := ns.Labels[k]; !ok || cur != v {
			ns.Labels[k] = v
			changed = true
		}
	}

	return changed, nil
}

// updateTrackingAnnotation updates the annotation that tracks what we've applied
func (r *NamespaceLabelReconciler) updateTrackingAnnotation(ctx context.Context, l logr.Logger, cr *labelsv1alpha1.NamespaceLabel, ns *corev1.Namespace, actuallyDesired map[string]string) error {
	if err := writeAppliedAnnotation(ctx, r.Client, ns, actuallyDesired); err != nil {
		message := fmt.Sprintf("Failed to update tracking annotation: %v", err)
		updateStatus(cr, false, "AnnotationError", message, nil, nil)
		if statusErr := r.Status().Update(ctx, cr); statusErr != nil {
			l.Error(statusErr, "failed to update status after annotation error")
		}
		return err
	}
	return nil
}
