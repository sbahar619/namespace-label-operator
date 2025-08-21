package controller

import (
	"fmt"
	"path/filepath"

	labelsv1alpha1 "github.com/sbahar619/namespace-label-operator/api/v1alpha1"
)

// isLabelProtected checks if a label key matches any of the protection patterns
func isLabelProtected(labelKey string, protectionPatterns []string) bool {
	for _, pattern := range protectionPatterns {
		// Skip empty patterns
		if pattern == "" {
			continue
		}

		// Use filepath.Match for glob pattern matching
		if matched, err := filepath.Match(pattern, labelKey); err == nil && matched {
			return true
		}
		// If there's an error in pattern matching, log it but continue
		// This prevents malformed patterns from breaking protection
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

			// Protected label with no conflict - log for debugging
			if !hasExisting {
				// This is fine - setting a new protected label is allowed
			} else if existingValue == value {
				// This is fine - no change needed
			}
		}

		// Label is either not protected or safe to apply
		result.AllowedLabels[key] = value
	}

	return result
}
