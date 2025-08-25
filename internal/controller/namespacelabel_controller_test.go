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

var _ = Describe("NamespaceLabelReconciler", Label("controller"), func() {
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

	// Helper functions to reduce code duplication
	createNamespace := func(name string, labels map[string]string, annotations map[string]string) *corev1.Namespace {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name,
				Labels:      labels,
				Annotations: annotations,
			},
		}
		Expect(fakeClient.Create(ctx, ns)).To(Succeed())
		return ns
	}

	createCR := func(name, namespace string, labels map[string]string, finalizers []string, spec labelsv1alpha1.NamespaceLabelSpec) *labelsv1alpha1.NamespaceLabel {
		cr := &labelsv1alpha1.NamespaceLabel{
			ObjectMeta: metav1.ObjectMeta{
				Name:       name,
				Namespace:  namespace,
				Labels:     labels,
				Finalizers: finalizers,
			},
			Spec: spec,
		}
		Expect(fakeClient.Create(ctx, cr)).To(Succeed())
		return cr
	}

	reconcileRequest := func(name, namespace string) reconcile.Request {
		return reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      name,
				Namespace: namespace,
			},
		}
	}

	expectFinalizerRemoved := func(cr *labelsv1alpha1.NamespaceLabel) {
		var updatedCR labelsv1alpha1.NamespaceLabel
		Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(cr), &updatedCR)).To(Succeed())
		Expect(updatedCR.Finalizers).NotTo(ContainElement(FinalizerName))
	}

	Describe("Reconcile", func() {
		It("should handle non-existent CR gracefully", func() {
			createNamespace("test-ns", nil, nil)

			result, err := reconciler.Reconcile(ctx, reconcileRequest("labels", "test-ns"))

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
		})

		It("should add finalizer to CR without finalizer", func() {
			createNamespace("test-ns", nil, nil)
			cr := createCR("labels", "test-ns", nil, nil, labelsv1alpha1.NamespaceLabelSpec{
				Labels: map[string]string{"app": "test"},
			})

			result, err := reconciler.Reconcile(ctx, reconcileRequest("labels", "test-ns"))

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Verify finalizer was added
			var updatedCR labelsv1alpha1.NamespaceLabel
			Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(cr), &updatedCR)).To(Succeed())
			Expect(updatedCR.Finalizers).To(ContainElement(FinalizerName))
		})

		It("should apply labels to namespace successfully", func() {
			ns := createNamespace("test-ns", nil, nil)
			createCR("labels", "test-ns", nil, []string{FinalizerName}, labelsv1alpha1.NamespaceLabelSpec{
				Labels: map[string]string{
					"app": "test",
					"env": "prod",
				},
			})

			result, err := reconciler.Reconcile(ctx, reconcileRequest("labels", "test-ns"))

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Verify labels were applied to namespace
			var updatedNS corev1.Namespace
			Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(ns), &updatedNS)).To(Succeed())
			Expect(updatedNS.Labels).To(HaveKeyWithValue("app", "test"))
			Expect(updatedNS.Labels).To(HaveKeyWithValue("env", "prod"))
			Expect(updatedNS.Annotations).To(HaveKey(appliedAnnoKey))
		})

		It("should handle label protection in fail mode", func() {
			ns := createNamespace("test-ns", map[string]string{
				"kubernetes.io/managed-by": "existing-operator",
			}, nil)
			createCR("labels", "test-ns", nil, []string{FinalizerName}, labelsv1alpha1.NamespaceLabelSpec{
				Labels: map[string]string{
					"app":                      "test",
					"kubernetes.io/managed-by": "my-operator", // This should be protected
				},
				ProtectedLabelPatterns: []string{"kubernetes.io/*"},
				ProtectionMode:         labelsv1alpha1.ProtectionModeFail,
			})

			result, err := reconciler.Reconcile(ctx, reconcileRequest("labels", "test-ns"))

			Expect(err).To(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			// Verify protected label was not changed
			var updatedNS corev1.Namespace
			Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(ns), &updatedNS)).To(Succeed())
			Expect(updatedNS.Labels).To(HaveKeyWithValue("kubernetes.io/managed-by", "existing-operator"))
		})

		It("should handle label updates when spec changes", func() {
			ns := createNamespace("test-ns", map[string]string{
				"old-label": "old-value",
			}, map[string]string{
				appliedAnnoKey: `{"old-label":"old-value"}`,
			})
			createCR("labels", "test-ns", nil, []string{FinalizerName}, labelsv1alpha1.NamespaceLabelSpec{
				Labels: map[string]string{
					"new-label": "new-value", // Changed from old-label to new-label
				},
			})

			result, err := reconciler.Reconcile(ctx, reconcileRequest("labels", "test-ns"))

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Verify old label was removed and new label was applied
			var updatedNS corev1.Namespace
			Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(ns), &updatedNS)).To(Succeed())
			Expect(updatedNS.Labels).NotTo(HaveKey("old-label"))
			Expect(updatedNS.Labels).To(HaveKeyWithValue("new-label", "new-value"))

			// Verify annotation was updated
			appliedLabels := readAppliedAnnotation(&updatedNS)
			Expect(appliedLabels).To(HaveKeyWithValue("new-label", "new-value"))
			Expect(appliedLabels).NotTo(HaveKey("old-label"))
		})
	})

	Describe("finalize", func() {
		// Test data for table-driven approach
		DescribeTable("should handle different deletion scenarios",
			func(setupNamespace func() *corev1.Namespace, crNamespace string, shouldFindNS bool, expectedLabelsAfter map[string]string) {
				// Setup namespace if provided
				var ns *corev1.Namespace
				if setupNamespace != nil {
					ns = setupNamespace()
				}

				// Create CR with finalizer
				cr := createCR("test-cr", crNamespace, nil, []string{FinalizerName}, labelsv1alpha1.NamespaceLabelSpec{})

				// Call finalize
				result, err := reconciler.finalize(ctx, cr)

				// Should always succeed and not requeue
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeFalse())
				Expect(result.RequeueAfter).To(BeZero())

				// Verify finalizer is removed
				expectFinalizerRemoved(cr)

				// Verify namespace state if it should exist
				if shouldFindNS && ns != nil {
					var updatedNS corev1.Namespace
					Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(ns), &updatedNS)).To(Succeed())

					// Check expected labels
					for k, v := range expectedLabelsAfter {
						Expect(updatedNS.Labels).To(HaveKeyWithValue(k, v))
					}

					// Applied annotation should be cleared
					Expect(updatedNS.Annotations).To(HaveKeyWithValue(appliedAnnoKey, "{}"))
				}
			},
			Entry("namespace not found", nil, "nonexistent-ns", false, nil),
			Entry("namespace with no applied labels",
				func() *corev1.Namespace {
					return createNamespace("test-ns", map[string]string{"existing": "label"}, nil)
				}, "test-ns", true, map[string]string{"existing": "label"}),
			Entry("namespace with applied labels to remove",
				func() *corev1.Namespace {
					return createNamespace("test-ns",
						map[string]string{
							"applied-by-operator": "value1",
							"another-applied":     "value2",
							"existing":            "keep-me",
						},
						map[string]string{
							appliedAnnoKey: `{"applied-by-operator":"value1","another-applied":"value2"}`,
						})
				}, "test-ns", true, map[string]string{"existing": "keep-me"}),
		)
	})

	Describe("getTargetNamespace", func() {
		It("should get target namespace successfully", func() {
			createNamespace("test-ns", nil, nil)

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
})
