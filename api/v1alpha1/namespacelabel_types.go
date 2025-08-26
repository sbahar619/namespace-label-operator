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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ProtectionMode defines how the operator handles attempts to modify protected labels
// +kubebuilder:validation:Enum=skip;warn;fail
type ProtectionMode string

const (
	// ProtectionModeSkip silently skips protected labels without error
	ProtectionModeSkip ProtectionMode = "skip"
	// ProtectionModeWarn skips protected labels but logs warnings and updates status
	ProtectionModeWarn ProtectionMode = "warn"
	// ProtectionModeFail fails the entire reconciliation if any protected labels are attempted
	ProtectionModeFail ProtectionMode = "fail"
)

// NamespaceLabelSpec defines the desired state of NamespaceLabel
type NamespaceLabelSpec struct {
	// Labels is a map of key-value pairs to apply to the namespace where this CR is created.
	// The target namespace is always the same as the CR's metadata.namespace for security.
	Labels map[string]string `json:"labels,omitempty"`

	// ProtectedLabelPatterns is a list of glob patterns for label keys that should not be overwritten.
	// If a label in the spec matches any of these patterns and the label already exists on the namespace
	// with a different value, the behavior is controlled by protectionMode.
	// Common patterns: "kubernetes.io/*", "*.k8s.io/*", "istio.io/*", "pod-security.kubernetes.io/*"
	// +optional
	ProtectedLabelPatterns []string `json:"protectedLabelPatterns,omitempty"`

	// ProtectionMode controls behavior when attempting to modify protected labels.
	// - skip: Silently skip protected labels (default)
	// - warn: Skip protected labels but log warnings and update status
	// - fail: Fail the entire reconciliation if any protected labels are attempted
	// +kubebuilder:default=skip
	// +optional
	ProtectionMode ProtectionMode `json:"protectionMode,omitempty"`
}

// NamespaceLabelStatus defines the observed state of NamespaceLabel
type NamespaceLabelStatus struct {
	// Applied indicates whether the labels were successfully applied
	Applied bool `json:"applied,omitempty"`

	// Conditions represent the latest available observations of the resource's state
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ProtectedLabelsSkipped lists label keys that were skipped due to protection
	// +optional
	ProtectedLabelsSkipped []string `json:"protectedLabelsSkipped,omitempty"`

	// LabelsApplied lists the label keys that were successfully applied
	// +optional
	LabelsApplied []string `json:"labelsApplied,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:path=namespacelabels,scope=Namespaced,shortName=nsl

// NamespaceLabel is the Schema for the namespacelabels API
type NamespaceLabel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NamespaceLabelSpec   `json:"spec,omitempty"`
	Status NamespaceLabelStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NamespaceLabelList contains a list of NamespaceLabel
type NamespaceLabelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NamespaceLabel `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NamespaceLabel{}, &NamespaceLabelList{})
}
