/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"

	labelsv1alpha1 "github.com/sbahar619/namespace-label-operator/api/v1alpha1"
)

// validateName ensures the NamespaceLabel CR follows the singleton naming pattern
func (v *NamespaceLabelCustomValidator) validateName(nl *labelsv1alpha1.NamespaceLabel) error {
	if nl.Name != StandardCRName {
		return fmt.Errorf("NamespaceLabel resource must be named '%s' for singleton pattern enforcement. Found name: '%s'", StandardCRName, nl.Name)
	}
	return nil
}

// validateSingleton ensures only one NamespaceLabel CR exists per namespace
func (v *NamespaceLabelCustomValidator) validateSingleton(ctx context.Context, nl *labelsv1alpha1.NamespaceLabel, oldNL *labelsv1alpha1.NamespaceLabel) error {
	// For updates, if the name hasn't changed, we're updating the same resource
	if oldNL != nil && oldNL.Name == nl.Name && oldNL.Namespace == nl.Namespace {
		return nil
	}

	// Check if another NamespaceLabel already exists in this namespace
	var existingList labelsv1alpha1.NamespaceLabelList
	err := v.Client.List(ctx, &existingList, client.InNamespace(nl.Namespace))
	if err != nil {
		return fmt.Errorf("failed to check for existing NamespaceLabel resources: %w", err)
	}

	// Count existing resources (excluding the one being updated if this is an update)
	existingCount := 0
	for _, existing := range existingList.Items {
		// Skip the resource being updated
		if oldNL != nil && existing.Name == oldNL.Name {
			continue
		}
		existingCount++
	}

	if existingCount > 0 {
		return fmt.Errorf("only one NamespaceLabel resource is allowed per namespace. Found %d existing NamespaceLabel resource(s) in namespace '%s'", existingCount, nl.Namespace)
	}

	return nil
}

// validateSpec validates the NamespaceLabel specification
func (v *NamespaceLabelCustomValidator) validateSpec(nl *labelsv1alpha1.NamespaceLabel) error {
	// Validate label keys and values
	if err := v.validateLabels(nl.Spec.Labels); err != nil {
		return err
	}

	// Validate protection patterns
	if err := v.validateProtectionPatterns(nl.Spec.ProtectedLabelPatterns); err != nil {
		return err
	}

	// Validate protection mode
	if err := v.validateProtectionMode(nl.Spec.ProtectionMode); err != nil {
		return err
	}

	return nil
}

// validateLabels validates that all label keys and values are valid Kubernetes labels
func (v *NamespaceLabelCustomValidator) validateLabels(labels map[string]string) error {
	for key, value := range labels {
		// Validate label key
		if errs := validation.IsQualifiedName(key); len(errs) > 0 {
			return fmt.Errorf("invalid label key '%s': %s", key, strings.Join(errs, ", "))
		}

		// Validate label value
		if errs := validation.IsValidLabelValue(value); len(errs) > 0 {
			return fmt.Errorf("invalid label value '%s' for key '%s': %s", value, key, strings.Join(errs, ", "))
		}
	}
	return nil
}

// validateProtectionPatterns validates that protection patterns are valid glob patterns
func (v *NamespaceLabelCustomValidator) validateProtectionPatterns(patterns []string) error {
	for _, pattern := range patterns {
		if pattern == "" {
			return fmt.Errorf("protection pattern cannot be empty")
		}

		// Test if the pattern is a valid glob pattern by trying to match against a test string
		_, err := filepath.Match(pattern, "test.example.com/key")
		if err != nil {
			return fmt.Errorf("invalid protection pattern '%s': %w", pattern, err)
		}
	}
	return nil
}

// validateProtectionMode validates that the protection mode is valid
func (v *NamespaceLabelCustomValidator) validateProtectionMode(mode labelsv1alpha1.ProtectionMode) error {
	switch mode {
	case labelsv1alpha1.ProtectionModeSkip, labelsv1alpha1.ProtectionModeWarn, labelsv1alpha1.ProtectionModeFail, "":
		return nil
	default:
		return fmt.Errorf("invalid protection mode '%s': must be one of 'skip', 'warn', or 'fail'", mode)
	}
}
