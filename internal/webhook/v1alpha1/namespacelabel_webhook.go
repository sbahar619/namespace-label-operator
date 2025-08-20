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

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	labelsv1alpha1 "github.com/sbahar619/namespace-label-operator/api/v1alpha1"
)

// nolint:unused
// log is for logging in this package.
var namespacelabellog = logf.Log.WithName("namespacelabel-resource")

const (
	// StandardCRName is the required name for NamespaceLabel CRs (singleton pattern)
	StandardCRName = "labels"
)

// SetupNamespaceLabelWebhookWithManager registers the webhook for NamespaceLabel in the manager.
func SetupNamespaceLabelWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&labelsv1alpha1.NamespaceLabel{}).
		WithValidator(&NamespaceLabelCustomValidator{
			Client: mgr.GetClient(),
		}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-labels-shahaf-com-v1alpha1-namespacelabel,mutating=false,failurePolicy=fail,sideEffects=None,groups=labels.shahaf.com,resources=namespacelabels,verbs=create;update,versions=v1alpha1,name=vnamespacelabel-v1alpha1.kb.io,admissionReviewVersions=v1

// NamespaceLabelCustomValidator struct is responsible for validating the NamespaceLabel resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type NamespaceLabelCustomValidator struct {
	Client client.Client
}

var _ webhook.CustomValidator = &NamespaceLabelCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type NamespaceLabel.
func (v *NamespaceLabelCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	namespacelabel, ok := obj.(*labelsv1alpha1.NamespaceLabel)
	if !ok {
		return nil, fmt.Errorf("expected a NamespaceLabel object but got %T", obj)
	}
	namespacelabellog.Info("Validation for NamespaceLabel upon creation", "name", namespacelabel.GetName(), "namespace", namespacelabel.GetNamespace())

	// Validate name (singleton pattern)
	if err := v.validateName(namespacelabel); err != nil {
		return nil, err
	}

	// Validate singleton (only one NamespaceLabel per namespace)
	if err := v.validateSingleton(ctx, namespacelabel, nil); err != nil {
		return nil, err
	}

	// Validate spec content
	if err := v.validateSpec(namespacelabel); err != nil {
		return nil, err
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type NamespaceLabel.
func (v *NamespaceLabelCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	namespacelabel, ok := newObj.(*labelsv1alpha1.NamespaceLabel)
	if !ok {
		return nil, fmt.Errorf("expected a NamespaceLabel object for the newObj but got %T", newObj)
	}

	oldNamespacelabel, ok := oldObj.(*labelsv1alpha1.NamespaceLabel)
	if !ok {
		return nil, fmt.Errorf("expected a NamespaceLabel object for the oldObj but got %T", oldObj)
	}

	namespacelabellog.Info("Validation for NamespaceLabel upon update", "name", namespacelabel.GetName(), "namespace", namespacelabel.GetNamespace())

	// Validate name (singleton pattern)
	if err := v.validateName(namespacelabel); err != nil {
		return nil, err
	}

	// Validate singleton (only one NamespaceLabel per namespace)
	if err := v.validateSingleton(ctx, namespacelabel, oldNamespacelabel); err != nil {
		return nil, err
	}

	// Validate spec content
	if err := v.validateSpec(namespacelabel); err != nil {
		return nil, err
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type NamespaceLabel.
func (v *NamespaceLabelCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	namespacelabel, ok := obj.(*labelsv1alpha1.NamespaceLabel)
	if !ok {
		return nil, fmt.Errorf("expected a NamespaceLabel object but got %T", obj)
	}
	namespacelabellog.Info("Validation for NamespaceLabel upon deletion", "name", namespacelabel.GetName(), "namespace", namespacelabel.GetNamespace())

	// No validation needed for deletion - let the controller handle cleanup
	return nil, nil
}

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
