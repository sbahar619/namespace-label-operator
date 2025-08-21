package controller

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

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
