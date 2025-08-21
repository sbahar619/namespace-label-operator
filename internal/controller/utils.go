package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	labelsv1alpha1 "github.com/sbahar619/namespace-label-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// mergeDesiredLabels merges labels from many CRs in the same namespace.
// Conflict policy: if multiple CRs set the same key with different values,
// the CR with the lexicographically smallest name wins for that key.
func mergeDesiredLabels(items []labelsv1alpha1.NamespaceLabel) (map[string]string, map[string]string) {
	desired := make(map[string]string)
	perKeyWinner := make(map[string]string) // key -> CR name that won

	// Sort CRs by name ascending for deterministic results
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })

	for _, cr := range items {
		for k, v := range cr.Spec.Labels {
			// If key not yet set, or set by a CR with a "larger" name, this CR wins.
			if winner, exists := perKeyWinner[k]; !exists || cr.Name < winner {
				desired[k] = v
				perKeyWinner[k] = cr.Name
			}
		}
	}
	return desired, perKeyWinner
}

func readAppliedAnnotation(ns *corev1.Namespace) map[string]string {
	out := map[string]string{}
	if ns.Annotations == nil {
		return out
	}
	raw, ok := ns.Annotations[appliedAnnoKey]
	if !ok || raw == "" {
		return out
	}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

func writeAppliedAnnotation(ctx context.Context, c client.Client, ns *corev1.Namespace, applied map[string]string) error {
	// Fetch a fresh copy of the namespace to avoid conflicts with the previously updated object
	var freshNS corev1.Namespace
	if err := c.Get(ctx, types.NamespacedName{Name: ns.Name}, &freshNS); err != nil {
		return fmt.Errorf("failed to fetch namespace for annotation update: %w", err)
	}

	if freshNS.Annotations == nil {
		freshNS.Annotations = map[string]string{}
	}

	b, err := json.Marshal(applied)
	if err != nil {
		return fmt.Errorf("marshal applied: %w", err)
	}

	// Check if annotation already has the correct value
	if cur, ok := freshNS.Annotations[appliedAnnoKey]; ok && cur == string(b) {
		return nil // no change needed
	}

	freshNS.Annotations[appliedAnnoKey] = string(b)
	return c.Update(ctx, &freshNS)
}

func boolToCond(b bool) metav1.ConditionStatus {
	if b {
		return metav1.ConditionTrue
	}
	return metav1.ConditionFalse
}
