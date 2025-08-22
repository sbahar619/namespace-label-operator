package controller

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	labelsv1alpha1 "github.com/sbahar619/namespace-label-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

// applyLabelsToNamespace applies desired labels and removes stale ones
func (r *NamespaceLabelReconciler) applyLabelsToNamespace(ns *corev1.Namespace, desired, prevApplied map[string]string) bool {
	if ns.Labels == nil {
		ns.Labels = make(map[string]string)
	}

	changed := removeStaleLabels(ns.Labels, desired, prevApplied)
	changed = applyDesiredLabels(ns.Labels, desired) || changed
	return changed
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
