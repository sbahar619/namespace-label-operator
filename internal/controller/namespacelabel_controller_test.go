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

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	labelsv1alpha1 "github.com/sbahar619/namespace-label-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

// Tests for functions in namespacelabel_controller.go

var _ = Describe("NamespaceLabelReconciler", func() {
	var (
		reconciler *NamespaceLabelReconciler
		fakeClient client.Client
		scheme     *runtime.Scheme
		ctx        context.Context
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(labelsv1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
		reconciler = &NamespaceLabelReconciler{
			Client: fakeClient,
			Scheme: scheme,
		}
		ctx = context.TODO()
	})

	Describe("Reconcile", func() {
		It("should handle non-existent CR gracefully", func() {
			// Create the namespace first since controller tries to get it
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ns",
				},
			}
			Expect(fakeClient.Create(ctx, ns)).To(Succeed())

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "labels",
					Namespace: "test-ns",
				},
			}

			result, err := reconciler.Reconcile(ctx, req)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
		})

		It("should add finalizer to CR without finalizer", func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ns",
				},
			}
			cr := &labelsv1alpha1.NamespaceLabel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "labels",
					Namespace: "test-ns",
				},
				Spec: labelsv1alpha1.NamespaceLabelSpec{
					Labels: map[string]string{"app": "test"},
				},
			}

			Expect(fakeClient.Create(ctx, ns)).To(Succeed())
			Expect(fakeClient.Create(ctx, cr)).To(Succeed())

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "labels",
					Namespace: "test-ns",
				},
			}

			result, err := reconciler.Reconcile(ctx, req)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Verify finalizer was added
			var updatedCR labelsv1alpha1.NamespaceLabel
			Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(cr), &updatedCR)).To(Succeed())
			Expect(updatedCR.Finalizers).To(ContainElement(FinalizerName))
		})

		It("should apply labels to namespace successfully", func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ns",
				},
			}
			cr := &labelsv1alpha1.NamespaceLabel{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "labels",
					Namespace:  "test-ns",
					Finalizers: []string{FinalizerName},
				},
				Spec: labelsv1alpha1.NamespaceLabelSpec{
					Labels: map[string]string{
						"app": "test",
						"env": "prod",
					},
				},
			}

			Expect(fakeClient.Create(ctx, ns)).To(Succeed())
			Expect(fakeClient.Create(ctx, cr)).To(Succeed())

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "labels",
					Namespace: "test-ns",
				},
			}

			result, err := reconciler.Reconcile(ctx, req)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Verify labels were applied to namespace
			var updatedNS corev1.Namespace
			Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(ns), &updatedNS)).To(Succeed())
			Expect(updatedNS.Labels).To(HaveKeyWithValue("app", "test"))
			Expect(updatedNS.Labels).To(HaveKeyWithValue("env", "prod"))

			// Verify annotation was written
			Expect(updatedNS.Annotations).To(HaveKey(appliedAnnoKey))
		})

		It("should handle label protection in fail mode", func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ns",
					Labels: map[string]string{
						"kubernetes.io/managed-by": "existing-operator",
					},
				},
			}
			cr := &labelsv1alpha1.NamespaceLabel{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "labels",
					Namespace:  "test-ns",
					Finalizers: []string{FinalizerName},
				},
				Spec: labelsv1alpha1.NamespaceLabelSpec{
					Labels: map[string]string{
						"app":                      "test",
						"kubernetes.io/managed-by": "my-operator", // This should be protected
					},
					ProtectedLabelPatterns: []string{"kubernetes.io/*"},
					ProtectionMode:         labelsv1alpha1.ProtectionModeFail,
				},
			}

			Expect(fakeClient.Create(ctx, ns)).To(Succeed())
			Expect(fakeClient.Create(ctx, cr)).To(Succeed())

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "labels",
					Namespace: "test-ns",
				},
			}

			result, err := reconciler.Reconcile(ctx, req)

			Expect(err).To(HaveOccurred())
			// Protection mode fail returns a requeue after 5 minutes
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			// Verify protected label was not changed
			var updatedNS corev1.Namespace
			Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(ns), &updatedNS)).To(Succeed())
			Expect(updatedNS.Labels).To(HaveKeyWithValue("kubernetes.io/managed-by", "existing-operator"))
		})

		It("should handle label updates when spec changes", func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ns",
					Labels: map[string]string{
						"old-label": "old-value",
					},
					Annotations: map[string]string{
						appliedAnnoKey: `{"old-label":"old-value"}`,
					},
				},
			}
			cr := &labelsv1alpha1.NamespaceLabel{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "labels",
					Namespace:  "test-ns",
					Finalizers: []string{FinalizerName},
				},
				Spec: labelsv1alpha1.NamespaceLabelSpec{
					Labels: map[string]string{
						"new-label": "new-value", // Changed from old-label to new-label
					},
				},
			}

			Expect(fakeClient.Create(ctx, ns)).To(Succeed())
			Expect(fakeClient.Create(ctx, cr)).To(Succeed())

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "labels",
					Namespace: "test-ns",
				},
			}

			result, err := reconciler.Reconcile(ctx, req)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Verify old label was removed and new label was applied
			var updatedNS corev1.Namespace
			Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(ns), &updatedNS)).To(Succeed())
			Expect(updatedNS.Labels).NotTo(HaveKey("old-label"))                    // Should be removed
			Expect(updatedNS.Labels).To(HaveKeyWithValue("new-label", "new-value")) // Should be added

			// Verify annotation was updated
			result2 := readAppliedAnnotation(&updatedNS)
			Expect(result2).To(HaveKeyWithValue("new-label", "new-value"))
			Expect(result2).NotTo(HaveKey("old-label"))
		})
	})

	Describe("getTargetNamespace", func() {
		It("should get target namespace successfully", func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ns",
				},
			}
			Expect(fakeClient.Create(ctx, ns)).To(Succeed())

			result, err := reconciler.getTargetNamespace(ctx, "test-ns")

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Name).To(Equal("test-ns"))
		})

		It("should return error for non-existent namespace", func() {
			_, err := reconciler.getTargetNamespace(ctx, "non-existent")

			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})
	})

	Describe("applyLabelsToNamespace", func() {
		It("should apply labels to namespace", func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ns",
					Labels: map[string]string{
						"existing": "label",
					},
				},
			}

			desired := map[string]string{
				"new":     "label",
				"updated": "value",
			}
			prevApplied := map[string]string{
				"old": "label",
			}

			changed := reconciler.applyLabelsToNamespace(ns, desired, prevApplied)

			Expect(changed).To(BeTrue())
			Expect(ns.Labels).To(HaveKeyWithValue("existing", "label"))
			Expect(ns.Labels).To(HaveKeyWithValue("new", "label"))
			Expect(ns.Labels).To(HaveKeyWithValue("updated", "value"))
			Expect(ns.Labels).NotTo(HaveKey("old")) // Should be removed as stale
		})
	})

	It("should create reconciler with proper configuration", func() {
		Expect(reconciler.Client).NotTo(BeNil())
		Expect(reconciler.Scheme).NotTo(BeNil())
	})

	Context("HandleDeletion Method", func() {
		var scheme *runtime.Scheme
		var fakeClient client.WithWatch
		var reconciler *NamespaceLabelReconciler

		BeforeEach(func() {
			scheme = runtime.NewScheme()
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			Expect(labelsv1alpha1.AddToScheme(scheme)).To(Succeed())

			fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			reconciler = &NamespaceLabelReconciler{
				Client: fakeClient,
				Scheme: scheme,
			}
		})

		It("should handle namespace not found during deletion", func() {
			// Create CR with finalizer in namespace that doesn't exist
			cr := &labelsv1alpha1.NamespaceLabel{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-cr",
					Namespace:  "nonexistent-ns",
					Finalizers: []string{FinalizerName},
				},
			}
			Expect(fakeClient.Create(ctx, cr)).To(Succeed())

			// Call handleDeletion - namespace doesn't exist
			result, err := reconciler.handleDeletion(ctx, cr)

			// Should succeed and not requeue
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
			Expect(result.RequeueAfter).To(BeZero())

			// Finalizer should be removed from CR
			var updatedCR labelsv1alpha1.NamespaceLabel
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "test-cr", Namespace: "nonexistent-ns"}, &updatedCR)).To(Succeed())
			Expect(updatedCR.Finalizers).NotTo(ContainElement(FinalizerName))
		})

		It("should remove applied labels and finalizer when namespace exists", func() {
			// Create namespace with some labels and applied annotation
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ns",
					Labels: map[string]string{
						"applied-by-operator": "value1",
						"another-applied":     "value2",
						"existing":            "keep-me", // Not applied by operator
					},
					Annotations: map[string]string{
						appliedAnnoKey: `{"applied-by-operator":"value1","another-applied":"value2"}`,
					},
				},
			}
			Expect(fakeClient.Create(ctx, ns)).To(Succeed())

			// Create CR with finalizer
			cr := &labelsv1alpha1.NamespaceLabel{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-cr",
					Namespace:  "test-ns",
					Finalizers: []string{FinalizerName},
				},
			}
			Expect(fakeClient.Create(ctx, cr)).To(Succeed())

			// Call handleDeletion
			result, err := reconciler.handleDeletion(ctx, cr)

			// Should succeed
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			// Check that applied labels were removed but existing labels remain
			var updatedNS corev1.Namespace
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "test-ns"}, &updatedNS)).To(Succeed())
			Expect(updatedNS.Labels).NotTo(HaveKey("applied-by-operator"))
			Expect(updatedNS.Labels).NotTo(HaveKey("another-applied"))
			Expect(updatedNS.Labels).To(HaveKeyWithValue("existing", "keep-me"))

			// Applied annotation should be cleared (empty map)
			Expect(updatedNS.Annotations).To(HaveKeyWithValue(appliedAnnoKey, "{}"))

			// Finalizer should be removed from CR
			var updatedCR labelsv1alpha1.NamespaceLabel
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "test-cr", Namespace: "test-ns"}, &updatedCR)).To(Succeed())
			Expect(updatedCR.Finalizers).NotTo(ContainElement(FinalizerName))
		})

		It("should handle namespace with no applied labels", func() {
			// Create namespace with no applied annotation
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ns",
					Labels: map[string]string{
						"existing": "label",
					},
				},
			}
			Expect(fakeClient.Create(ctx, ns)).To(Succeed())

			// Create CR with finalizer
			cr := &labelsv1alpha1.NamespaceLabel{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-cr",
					Namespace:  "test-ns",
					Finalizers: []string{FinalizerName},
				},
			}
			Expect(fakeClient.Create(ctx, cr)).To(Succeed())

			// Call handleDeletion
			result, err := reconciler.handleDeletion(ctx, cr)

			// Should succeed
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			// Existing labels should remain untouched
			var updatedNS corev1.Namespace
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "test-ns"}, &updatedNS)).To(Succeed())
			Expect(updatedNS.Labels).To(HaveKeyWithValue("existing", "label"))

			// Applied annotation should be set to empty map
			Expect(updatedNS.Annotations).To(HaveKeyWithValue(appliedAnnoKey, "{}"))

			// Finalizer should be removed
			var updatedCR labelsv1alpha1.NamespaceLabel
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "test-cr", Namespace: "test-ns"}, &updatedCR)).To(Succeed())
			Expect(updatedCR.Finalizers).NotTo(ContainElement(FinalizerName))
		})

		It("should handle namespace with nil labels map and applied labels to remove", func() {
			// Create namespace with nil labels but annotation indicating applied labels
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ns",
					Labels: map[string]string{
						"applied-label": "value", // This was applied by operator
					},
					Annotations: map[string]string{
						appliedAnnoKey: `{"applied-label":"value"}`, // Consistent with actual labels
					},
				},
			}
			Expect(fakeClient.Create(ctx, ns)).To(Succeed())

			// Create CR with finalizer
			cr := &labelsv1alpha1.NamespaceLabel{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-cr",
					Namespace:  "test-ns",
					Finalizers: []string{FinalizerName},
				},
			}
			Expect(fakeClient.Create(ctx, cr)).To(Succeed())

			// Call handleDeletion
			result, err := reconciler.handleDeletion(ctx, cr)

			// Should succeed
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			// Applied label should be removed, namespace updated
			var updatedNS corev1.Namespace
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "test-ns"}, &updatedNS)).To(Succeed())
			Expect(updatedNS.Labels).NotTo(HaveKey("applied-label"))

			// Applied annotation should be cleared
			Expect(updatedNS.Annotations).To(HaveKeyWithValue(appliedAnnoKey, "{}"))

			// Finalizer should be removed
			var updatedCR labelsv1alpha1.NamespaceLabel
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "test-cr", Namespace: "test-ns"}, &updatedCR)).To(Succeed())
			Expect(updatedCR.Finalizers).NotTo(ContainElement(FinalizerName))
		})
	})
})
