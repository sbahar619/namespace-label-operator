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
