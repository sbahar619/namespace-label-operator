package controller

import (
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
