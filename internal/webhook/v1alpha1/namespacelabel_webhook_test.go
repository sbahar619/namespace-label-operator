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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	labelsv1alpha1 "github.com/sbahar619/namespace-label-operator/api/v1alpha1"
)

var _ = Describe("NamespaceLabel Webhook", Label("webhook"), func() {
	var (
		ctx       context.Context
		validator *NamespaceLabelCustomValidator
		scheme    *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(labelsv1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
	})

	Describe("ValidateCreate", func() {
		Context("When validating name", func() {
			It("should allow creation with correct name 'labels'", func() {
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
				validator = &NamespaceLabelCustomValidator{Client: fakeClient}

				obj := &labelsv1alpha1.NamespaceLabel{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "labels",
						Namespace: "test-ns",
					},
					Spec: labelsv1alpha1.NamespaceLabelSpec{
						Labels: map[string]string{"env": "test"},
					},
				}

				warnings, err := validator.ValidateCreate(ctx, obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(warnings).To(BeEmpty())
			})

			It("should reject creation with incorrect name", func() {
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
				validator = &NamespaceLabelCustomValidator{Client: fakeClient}

				obj := &labelsv1alpha1.NamespaceLabel{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "invalid-name",
						Namespace: "test-ns",
					},
					Spec: labelsv1alpha1.NamespaceLabelSpec{
						Labels: map[string]string{"env": "test"},
					},
				}

				warnings, err := validator.ValidateCreate(ctx, obj)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("NamespaceLabel resource must be named 'labels'"))
				Expect(warnings).To(BeEmpty())
			})
		})

		Context("When validating singleton pattern", func() {
			It("should allow creation when no other NamespaceLabel exists", func() {
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
				validator = &NamespaceLabelCustomValidator{Client: fakeClient}

				obj := &labelsv1alpha1.NamespaceLabel{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "labels",
						Namespace: "test-ns",
					},
					Spec: labelsv1alpha1.NamespaceLabelSpec{
						Labels: map[string]string{"env": "test"},
					},
				}

				warnings, err := validator.ValidateCreate(ctx, obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(warnings).To(BeEmpty())
			})

			It("should reject creation when another NamespaceLabel already exists", func() {
				existing := &labelsv1alpha1.NamespaceLabel{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "labels",
						Namespace: "test-ns",
					},
					Spec: labelsv1alpha1.NamespaceLabelSpec{
						Labels: map[string]string{"existing": "label"},
					},
				}

				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(existing).
					Build()
				validator = &NamespaceLabelCustomValidator{Client: fakeClient}

				obj := &labelsv1alpha1.NamespaceLabel{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "labels",
						Namespace: "test-ns",
					},
					Spec: labelsv1alpha1.NamespaceLabelSpec{
						Labels: map[string]string{"env": "test"},
					},
				}

				warnings, err := validator.ValidateCreate(ctx, obj)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("only one NamespaceLabel resource is allowed per namespace"))
				Expect(warnings).To(BeEmpty())
			})
		})
	})

	Describe("ValidateUpdate", func() {
		It("should allow valid updates", func() {
			existing := &labelsv1alpha1.NamespaceLabel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "labels",
					Namespace: "test-ns",
				},
				Spec: labelsv1alpha1.NamespaceLabelSpec{
					Labels: map[string]string{"env": "test"},
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(existing).
				Build()
			validator = &NamespaceLabelCustomValidator{Client: fakeClient}

			newObj := &labelsv1alpha1.NamespaceLabel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "labels",
					Namespace: "test-ns",
				},
				Spec: labelsv1alpha1.NamespaceLabelSpec{
					Labels: map[string]string{
						"env":  "production",
						"tier": "backend",
					},
				},
			}

			warnings, err := validator.ValidateUpdate(ctx, existing, newObj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should reject name changes", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			validator = &NamespaceLabelCustomValidator{Client: fakeClient}

			oldObj := &labelsv1alpha1.NamespaceLabel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "labels",
					Namespace: "test-ns",
				},
				Spec: labelsv1alpha1.NamespaceLabelSpec{
					Labels: map[string]string{"env": "test"},
				},
			}

			newObj := &labelsv1alpha1.NamespaceLabel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "different-name",
					Namespace: "test-ns",
				},
				Spec: labelsv1alpha1.NamespaceLabelSpec{
					Labels: map[string]string{"env": "test"},
				},
			}

			warnings, err := validator.ValidateUpdate(ctx, oldObj, newObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("NamespaceLabel resource must be named 'labels'"))
			Expect(warnings).To(BeEmpty())
		})
	})

	Describe("ValidateDelete", func() {
		It("should always allow deletion", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			validator = &NamespaceLabelCustomValidator{Client: fakeClient}

			obj := &labelsv1alpha1.NamespaceLabel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "labels",
					Namespace: "test-ns",
				},
			}

			warnings, err := validator.ValidateDelete(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})
	})

	Describe("Type validation", func() {
		It("should reject non-NamespaceLabel objects", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			validator = &NamespaceLabelCustomValidator{Client: fakeClient}

			// Use a ConfigMap as a different runtime.Object type
			obj := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "not-a-namespacelabel",
					Namespace: "test-ns",
				},
			}

			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected a NamespaceLabel object"))
			Expect(warnings).To(BeEmpty())

			warnings, err = validator.ValidateUpdate(ctx, obj, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected a NamespaceLabel object"))
			Expect(warnings).To(BeEmpty())

			warnings, err = validator.ValidateDelete(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected a NamespaceLabel object"))
			Expect(warnings).To(BeEmpty())
		})
	})
})
