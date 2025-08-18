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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	labelsv1alpha1 "github.com/sbahar619/namespace-label-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

// Unit tests are integrated into the main controller suite in suite_test.go

var _ = Describe("NamespaceLabel Controller Unit Tests", func() {
	var (
		reconciler *NamespaceLabelReconciler
		fakeClient client.Client
		scheme     *runtime.Scheme
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
	})

	Context("JSON Annotation Processing", func() {
		It("should parse valid JSON annotation correctly", func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"labels.shahaf.com/applied": `{"app":"web","environment":"prod"}`,
					},
				},
			}

			result := readAppliedAnnotation(ns)

			Expect(result).To(HaveLen(2))
			Expect(result).To(HaveKeyWithValue("app", "web"))
			Expect(result).To(HaveKeyWithValue("environment", "prod"))
		})

		It("should handle empty annotation gracefully", func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"labels.shahaf.com/applied": "",
					},
				},
			}

			result := readAppliedAnnotation(ns)

			Expect(result).To(BeEmpty())
		})

		It("should handle missing annotation gracefully", func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			}

			result := readAppliedAnnotation(ns)

			Expect(result).To(BeEmpty())
		})

		It("should handle nil annotations gracefully", func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: nil,
				},
			}

			result := readAppliedAnnotation(ns)

			Expect(result).To(BeEmpty())
		})

		It("should handle invalid JSON gracefully", func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"labels.shahaf.com/applied": `{invalid-json}`,
					},
				},
			}

			result := readAppliedAnnotation(ns)

			Expect(result).To(BeEmpty())
		})
	})

	Context("Boolean to Condition Conversion", func() {
		It("should convert true to ConditionTrue", func() {
			result := boolToCond(true)
			Expect(result).To(Equal(metav1.ConditionTrue))
		})

		It("should convert false to ConditionFalse", func() {
			result := boolToCond(false)
			Expect(result).To(Equal(metav1.ConditionFalse))
		})
	})

	Context("Slice Utility Functions", func() {
		It("should remove existing item from slice", func() {
			input := []string{"apple", "banana", "cherry"}
			result := removeFromSlice(input, "banana")

			Expect(result).To(Equal([]string{"apple", "cherry"}))
			Expect(result).To(HaveLen(2))
		})

		It("should handle removing non-existing item", func() {
			input := []string{"apple", "banana", "cherry"}
			result := removeFromSlice(input, "orange")

			Expect(result).To(Equal([]string{"apple", "banana", "cherry"}))
			Expect(result).To(HaveLen(3))
		})

		It("should handle empty slice", func() {
			input := []string{}
			result := removeFromSlice(input, "anything")

			Expect(result).To(BeEmpty())
		})

		It("should handle removing duplicate items", func() {
			input := []string{"apple", "banana", "apple", "cherry"}
			result := removeFromSlice(input, "apple")

			Expect(result).To(Equal([]string{"banana", "cherry"}))
			Expect(result).To(HaveLen(2))
		})
	})

	Context("Label Merging Logic", func() {
		It("should handle single CR correctly", func() {
			crs := []labelsv1alpha1.NamespaceLabel{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "labels"},
					Spec: labelsv1alpha1.NamespaceLabelSpec{
						Labels: map[string]string{
							"app":         "web",
							"environment": "prod",
						},
					},
				},
			}

			merged, perKeyWinner := mergeDesiredLabels(crs)

			Expect(merged).To(HaveLen(2))
			Expect(merged).To(HaveKeyWithValue("app", "web"))
			Expect(merged).To(HaveKeyWithValue("environment", "prod"))
			Expect(perKeyWinner).To(HaveKeyWithValue("app", "labels"))
			Expect(perKeyWinner).To(HaveKeyWithValue("environment", "labels"))
		})

		It("should handle empty CR list", func() {
			crs := []labelsv1alpha1.NamespaceLabel{}

			merged, perKeyWinner := mergeDesiredLabels(crs)

			Expect(merged).To(BeEmpty())
			Expect(perKeyWinner).To(BeEmpty())
		})

		It("should handle CR with empty labels", func() {
			crs := []labelsv1alpha1.NamespaceLabel{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "labels"},
					Spec: labelsv1alpha1.NamespaceLabelSpec{
						Labels: map[string]string{},
					},
				},
			}

			merged, perKeyWinner := mergeDesiredLabels(crs)

			Expect(merged).To(BeEmpty())
			Expect(perKeyWinner).To(BeEmpty())
		})

		It("should handle multiple CRs with name-based ordering", func() {
			// Even though singleton pattern discourages this, the function should work correctly
			crs := []labelsv1alpha1.NamespaceLabel{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "zzz-last"},
					Spec: labelsv1alpha1.NamespaceLabelSpec{
						Labels: map[string]string{"env": "staging"},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "aaa-first"},
					Spec: labelsv1alpha1.NamespaceLabelSpec{
						Labels: map[string]string{"env": "production"},
					},
				},
			}

			merged, perKeyWinner := mergeDesiredLabels(crs)

			// "aaa-first" should win due to alphabetical ordering
			Expect(merged).To(HaveKeyWithValue("env", "production"))
			Expect(perKeyWinner).To(HaveKeyWithValue("env", "aaa-first"))
		})
	})

	Context("Status Update Logic", func() {
		It("should update status fields correctly", func() {
			cr := &labelsv1alpha1.NamespaceLabel{
				Status: labelsv1alpha1.NamespaceLabelStatus{},
			}

			updateStatus(cr, true, "Synced", "Labels applied successfully")

			Expect(cr.Status.Applied).To(BeTrue())
			Expect(cr.Status.Message).To(Equal("Labels applied successfully"))
			Expect(cr.Status.Conditions).To(HaveLen(1))

			condition := cr.Status.Conditions[0]
			Expect(condition.Type).To(Equal("Ready"))
			Expect(condition.Status).To(Equal(metav1.ConditionTrue))
			Expect(condition.Reason).To(Equal("Synced"))
			Expect(condition.Message).To(Equal("Labels applied successfully"))
		})

		It("should handle failure status correctly", func() {
			cr := &labelsv1alpha1.NamespaceLabel{
				Status: labelsv1alpha1.NamespaceLabelStatus{},
			}

			updateStatus(cr, false, "InvalidName", "CR must be named 'labels'")

			Expect(cr.Status.Applied).To(BeFalse())
			Expect(cr.Status.Message).To(Equal("CR must be named 'labels'"))
			Expect(cr.Status.Conditions).To(HaveLen(1))

			condition := cr.Status.Conditions[0]
			Expect(condition.Type).To(Equal("Ready"))
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal("InvalidName"))
			Expect(condition.Message).To(Equal("CR must be named 'labels'"))
		})
	})

	Context("Constants and Configuration", func() {
		It("should have correct singleton CR name", func() {
			Expect(StandardCRName).To(Equal("labels"))
		})

		It("should have correct finalizer name", func() {
			Expect(FinalizerName).To(Equal("labels.shahaf.com/finalizer"))
		})

		It("should have properly configured reconciler", func() {
			Expect(reconciler.Client).NotTo(BeNil())
			Expect(reconciler.Scheme).NotTo(BeNil())
		})
	})
})
