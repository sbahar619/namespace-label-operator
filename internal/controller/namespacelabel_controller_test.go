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
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	labelsv1alpha1 "github.com/sbahar619/namespace-label-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

func TestControllerUnit(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Unit Tests")
}

var _ = Describe("NamespaceLabel Controller Unit Tests", func() {
	var (
		reconciler *NamespaceLabelReconciler
		ctx        context.Context
		fakeClient client.Client
		scheme     *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(labelsv1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
		reconciler = &NamespaceLabelReconciler{
			Client: fakeClient,
			Scheme: scheme,
		}
	})

	Context("Label Merging Logic", func() {
		It("should merge labels from multiple CRs correctly", func() {
			crs := []labelsv1alpha1.NamespaceLabel{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "cr1"},
					Spec: labelsv1alpha1.NamespaceLabelSpec{
						Labels: map[string]string{
							"app":         "web",
							"environment": "prod",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "cr2"},
					Spec: labelsv1alpha1.NamespaceLabelSpec{
						Labels: map[string]string{
							"team":    "backend",
							"version": "v1.0",
						},
					},
				},
			}

			mergedLabels, conflicts := mergeDesiredLabels(crs)

			Expect(mergedLabels).To(HaveKeyWithValue("app", "web"))
			Expect(mergedLabels).To(HaveKeyWithValue("environment", "prod"))
			Expect(mergedLabels).To(HaveKeyWithValue("team", "backend"))
			Expect(mergedLabels).To(HaveKeyWithValue("version", "v1.0"))
			Expect(conflicts).To(BeEmpty())
		})

		It("should handle label conflicts with name-based tie-breaking", func() {
			crs := []labelsv1alpha1.NamespaceLabel{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "zz-last"},
					Spec: labelsv1alpha1.NamespaceLabelSpec{
						Labels: map[string]string{
							"environment": "staging",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "aa-first"},
					Spec: labelsv1alpha1.NamespaceLabelSpec{
						Labels: map[string]string{
							"environment": "production",
						},
					},
				},
			}

			mergedLabels, conflicts := mergeDesiredLabels(crs)

			// "aa-first" should win due to alphabetical ordering
			Expect(mergedLabels).To(HaveKeyWithValue("environment", "production"))
			Expect(conflicts).To(HaveKey("environment"))
			Expect(conflicts["environment"]).To(Equal("aa-first"))
		})

		It("should handle empty label specs", func() {
			crs := []labelsv1alpha1.NamespaceLabel{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "empty"},
					Spec: labelsv1alpha1.NamespaceLabelSpec{
						Labels: map[string]string{},
					},
				},
			}

			mergedLabels, conflicts := mergeDesiredLabels(crs)

			Expect(mergedLabels).To(BeEmpty())
			Expect(conflicts).To(BeEmpty())
		})
	})

	Context("Singleton Pattern Validation", func() {
		It("should validate CR name is 'labels'", func() {
			By("Creating a CR with invalid name")
			cr := &labelsv1alpha1.NamespaceLabel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-name",
					Namespace: "test-ns",
				},
				Spec: labelsv1alpha1.NamespaceLabelSpec{
					Labels: map[string]string{"test": "value"},
				},
			}

			// Create the CR in fake client
			Expect(fakeClient.Create(ctx, cr)).To(Succeed())

			// Create a namespace for the test
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "test-ns"},
			}
			Expect(fakeClient.Create(ctx, ns)).To(Succeed())

			// Run reconcile
			req := ctrl.Request{NamespacedName: types.NamespacedName{
				Name:      "invalid-name",
				Namespace: "test-ns",
			}}

			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Check that CR status indicates error
			updated := &labelsv1alpha1.NamespaceLabel{}
			Expect(fakeClient.Get(ctx, req.NamespacedName, updated)).To(Succeed())

			foundError := false
			for _, condition := range updated.Status.Conditions {
				if condition.Type == "Ready" && condition.Status == metav1.ConditionFalse {
					foundError = true
					break
				}
			}
			Expect(foundError).To(BeTrue())
		})

		It("should allow CR named 'labels'", func() {
			By("Creating a CR with valid name")
			cr := &labelsv1alpha1.NamespaceLabel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "labels",
					Namespace: "test-ns",
				},
				Spec: labelsv1alpha1.NamespaceLabelSpec{
					Labels: map[string]string{"test": "value"},
				},
			}

			// Create the CR in fake client
			Expect(fakeClient.Create(ctx, cr)).To(Succeed())

			// Create a namespace for the test
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "test-ns"},
			}
			Expect(fakeClient.Create(ctx, ns)).To(Succeed())

			// Run reconcile
			req := ctrl.Request{NamespacedName: types.NamespacedName{
				Name:      "labels",
				Namespace: "test-ns",
			}}

			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Check that CR status indicates success
			updated := &labelsv1alpha1.NamespaceLabel{}
			Expect(fakeClient.Get(ctx, req.NamespacedName, updated)).To(Succeed())

			foundSuccess := false
			for _, condition := range updated.Status.Conditions {
				if condition.Type == "Ready" && condition.Status == metav1.ConditionTrue {
					foundSuccess = true
					break
				}
			}
			Expect(foundSuccess).To(BeTrue())
		})
	})

	Context("Namespace Label Application", func() {
		It("should apply labels to target namespace", func() {
			By("Creating a valid CR")
			cr := &labelsv1alpha1.NamespaceLabel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "labels",
					Namespace: "test-ns",
				},
				Spec: labelsv1alpha1.NamespaceLabelSpec{
					Labels: map[string]string{
						"environment": "production",
						"team":        "platform",
					},
				},
			}
			Expect(fakeClient.Create(ctx, cr)).To(Succeed())

			By("Creating target namespace")
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "test-ns"},
			}
			Expect(fakeClient.Create(ctx, ns)).To(Succeed())

			By("Running reconcile")
			req := ctrl.Request{NamespacedName: types.NamespacedName{
				Name:      "labels",
				Namespace: "test-ns",
			}}

			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying labels were applied")
			updatedNS := &corev1.Namespace{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "test-ns"}, updatedNS)).To(Succeed())

			Expect(updatedNS.Labels).To(HaveKeyWithValue("environment", "production"))
			Expect(updatedNS.Labels).To(HaveKeyWithValue("team", "platform"))
		})
	})

	Context("Error Handling", func() {
		It("should handle missing namespace gracefully", func() {
			By("Creating a CR for non-existent namespace")
			cr := &labelsv1alpha1.NamespaceLabel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "labels",
					Namespace: "missing-ns",
				},
				Spec: labelsv1alpha1.NamespaceLabelSpec{
					Labels: map[string]string{"test": "value"},
				},
			}
			Expect(fakeClient.Create(ctx, cr)).To(Succeed())

			By("Running reconcile")
			req := ctrl.Request{NamespacedName: types.NamespacedName{
				Name:      "labels",
				Namespace: "missing-ns",
			}}

			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			By("Verifying error status is set")
			updated := &labelsv1alpha1.NamespaceLabel{}
			Expect(fakeClient.Get(ctx, req.NamespacedName, updated)).To(Succeed())

			foundError := false
			for _, condition := range updated.Status.Conditions {
				if condition.Type == "Ready" && condition.Status == metav1.ConditionFalse {
					foundError = true
					break
				}
			}
			Expect(foundError).To(BeTrue())
		})
	})
})
