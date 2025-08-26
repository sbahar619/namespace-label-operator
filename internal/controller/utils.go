package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	labelsv1alpha1 "github.com/sbahar619/namespace-label-operator/api/v1alpha1"
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

// removeStaleLabels removes labels that were previously applied by this operator but are no longer desired
func removeStaleLabels(current, desired, prevApplied map[string]string) bool {
	changed := false
	for key, prevVal := range prevApplied {
		if _, stillWanted := desired[key]; !stillWanted {
			if cur, exists := current[key]; exists && cur == prevVal {
				delete(current, key)
				changed = true
			}
		}
	}
	return changed
}

// applyDesiredLabels sets or updates labels to their desired values
func applyDesiredLabels(current, desired map[string]string) bool {
	changed := false
	for key, val := range desired {
		if current[key] != val {
			current[key] = val
			changed = true
		}
	}
	return changed
}

// isLabelProtected checks if a label key matches any of the protection patterns
func isLabelProtected(labelKey string, protectionPatterns []string) bool {
	for _, pattern := range protectionPatterns {
		// Skip empty patterns
		if pattern == "" {
			continue
		}

		// Handle problematic patterns that might cause issues
		// Patterns like "**/*" are not valid filepath patterns
		if strings.Contains(pattern, "**") {
			// Convert double wildcards to single wildcards for this context
			pattern = strings.ReplaceAll(pattern, "**", "*")
		}

		// Use filepath.Match for glob pattern matching
		matched, err := filepath.Match(pattern, labelKey)
		if err != nil {
			// If there's an error in pattern matching, log it but continue
			// This prevents malformed patterns from breaking protection
			continue
		}
		if matched {
			return true
		}
	}
	return false
}

// applyProtectionLogic processes desired labels against protection rules
func applyProtectionLogic(
	desired map[string]string,
	existing map[string]string,
	protectionPatterns []string,
	protectionMode labelsv1alpha1.ProtectionMode,
) ProtectionResult {
	result := ProtectionResult{
		AllowedLabels:    make(map[string]string),
		ProtectedSkipped: []string{},
		Warnings:         []string{},
		ShouldFail:       false,
	}

	for key, value := range desired {
		// Check if this label is protected
		if isLabelProtected(key, protectionPatterns) {
			existingValue, hasExisting := existing[key]

			// If the label exists with a different value, apply protection
			if hasExisting && existingValue != value {
				msg := fmt.Sprintf("Label '%s' is protected by pattern and has existing value '%s' (attempting to set '%s')",
					key, existingValue, value)

				switch protectionMode {
				case labelsv1alpha1.ProtectionModeFail:
					result.ShouldFail = true
					result.Warnings = append(result.Warnings, msg)
					return result
				case labelsv1alpha1.ProtectionModeWarn:
					result.Warnings = append(result.Warnings, msg)
					result.ProtectedSkipped = append(result.ProtectedSkipped, key)
					continue
				default: // ProtectionModeSkip
					result.ProtectedSkipped = append(result.ProtectedSkipped, key)
					continue
				}
			}

			// Protected label with no conflict - allow it
			// Either setting a new protected label (!hasExisting) or no change needed (existingValue == value)
		}

		// Label is either not protected or safe to apply
		result.AllowedLabels[key] = value
	}

	return result
}

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
