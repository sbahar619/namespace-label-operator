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

package utils

import (
	"context"
	"time"

	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	labelsv1alpha1 "github.com/sbahar619/namespace-label-operator/api/v1alpha1"
)

// CROptions provides options for creating NamespaceLabel CRs
type CROptions struct {
	Name                   string
	Labels                 map[string]string
	ProtectedLabelPatterns []string
	ProtectionMode         string
}

// NewNamespaceLabel builds a NamespaceLabel CR object with the given options
// Returns the CR object without creating it in Kubernetes
func NewNamespaceLabel(opts CROptions, namespace string) *labelsv1alpha1.NamespaceLabel {
	// Default name to "labels" if not specified
	name := opts.Name
	if name == "" {
		name = "labels"
	}

	// Create the CR
	cr := &labelsv1alpha1.NamespaceLabel{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: labelsv1alpha1.NamespaceLabelSpec{
			Labels: opts.Labels,
		},
	}

	// Add protection settings if specified
	if len(opts.ProtectedLabelPatterns) > 0 {
		cr.Spec.ProtectedLabelPatterns = opts.ProtectedLabelPatterns
	}
	if opts.ProtectionMode != "" {
		cr.Spec.ProtectionMode = labelsv1alpha1.ProtectionMode(opts.ProtectionMode)
	}

	return cr
}

// CreateNamespaceLabel creates a NamespaceLabel CR in Kubernetes and expects it to succeed
// Returns the created CR object for further operations (like deletion)
func CreateNamespaceLabel(
	ctx context.Context,
	k8sClient client.Client,
	opts CROptions,
	namespace string,
) *labelsv1alpha1.NamespaceLabel {
	cr := NewNamespaceLabel(opts, namespace)
	Expect(k8sClient.Create(ctx, cr)).To(Succeed())
	return cr
}

// WaitForCRToExist waits for a CR to be created and accessible
func WaitForCRToExist(ctx context.Context, k8sClient client.Client, name, namespace string) {
	Eventually(func() error {
		found := &labelsv1alpha1.NamespaceLabel{}
		return k8sClient.Get(ctx, types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		}, found)
	}, time.Minute, time.Second).Should(Succeed())
}

// WaitForCRToBeDeleted waits for a CR to be deleted (IsNotFound)
func WaitForCRToBeDeleted(ctx context.Context, k8sClient client.Client, name, namespace string) {
	Eventually(func() bool {
		found := &labelsv1alpha1.NamespaceLabel{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, found)
		return errors.IsNotFound(err)
	}, time.Minute, time.Second).Should(BeTrue())
}

// GetCRStatus returns a function that gets the CR status
func GetCRStatus(
	ctx context.Context,
	k8sClient client.Client,
	name, namespace string,
) func() *labelsv1alpha1.NamespaceLabelStatus {
	return func() *labelsv1alpha1.NamespaceLabelStatus {
		found := &labelsv1alpha1.NamespaceLabel{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, found)
		if err != nil {
			return nil
		}
		return &found.Status
	}
}

// GetNamespaceLabels returns a function that gets the current labels on the specified namespace
func GetNamespaceLabels(ctx context.Context, k8sClient client.Client, namespace string) func() map[string]string {
	return func() map[string]string {
		ns := &corev1.Namespace{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: namespace}, ns)
		if err != nil {
			return nil
		}
		return ns.Labels
	}
}

// SetNamespaceLabel sets a label on the specified namespace (handles label map initialization)
func SetNamespaceLabel(ctx context.Context, k8sClient client.Client, namespace, key, value string) {
	ns := &corev1.Namespace{}
	Expect(k8sClient.Get(ctx, types.NamespacedName{Name: namespace}, ns)).To(Succeed())
	if ns.Labels == nil {
		ns.Labels = make(map[string]string)
	}
	ns.Labels[key] = value
	Expect(k8sClient.Update(ctx, ns)).To(Succeed())
}

// ExpectWebhookRejection expects webhook to reject CR creation with specific error
func ExpectWebhookRejection(
	ctx context.Context,
	k8sClient client.Client,
	cr *labelsv1alpha1.NamespaceLabel,
	expectedErrorSubstring string,
) {
	err := k8sClient.Create(ctx, cr)
	Expect(err).To(HaveOccurred(), "Expected webhook to reject the CR")
	Expect(err.Error()).To(ContainSubstring(expectedErrorSubstring), "Expected specific validation error message")
}
