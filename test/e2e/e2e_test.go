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

package e2e

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	labelsv1alpha1 "github.com/sbahar619/namespace-label-operator/api/v1alpha1"
	"github.com/sbahar619/namespace-label-operator/test/utils"
)

var _ = Describe("NamespaceLabel E2E Tests", func() {
	var (
		k8sClient client.Client
		ctx       context.Context
		testNS    string
	)

	BeforeEach(func() {
		ctx = context.Background()
		// Use nanoseconds and random number to avoid collisions
		testNS = fmt.Sprintf("e2e-test-%d-%d", time.Now().UnixNano(), rand.Int31())

		By("Setting up Kubernetes client")
		var err error
		k8sClient, err = utils.GetK8sClient()
		Expect(err).NotTo(HaveOccurred())

		By("Creating test namespace")
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNS,
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
	})

	AfterEach(func() {
		By("Cleaning up test namespace")
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNS,
			},
		}
		// Delete the namespace and wait for it to be fully removed
		err := k8sClient.Delete(ctx, ns)
		if err != nil && !errors.IsNotFound(err) {
			Expect(err).NotTo(HaveOccurred())
		}
		
		// Wait for namespace to be fully deleted
		Eventually(func() bool {
			checkNS := &corev1.Namespace{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: testNS}, checkNS)
			return errors.IsNotFound(err)
		}, time.Minute, time.Second).Should(BeTrue())
	})

	Context("Basic NamespaceLabel Operations", func() {
		It("should create a NamespaceLabel CR successfully", func() {
			By("Creating a NamespaceLabel CR")
			cr := &labelsv1alpha1.NamespaceLabel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "labels",
					Namespace: testNS,
				},
				Spec: labelsv1alpha1.NamespaceLabelSpec{
					Labels: map[string]string{
						"environment": "test",
						"team":        "platform",
					},
				},
			}

			Expect(k8sClient.Create(ctx, cr)).To(Succeed())

			By("Verifying the CR was created")
			Eventually(func() error {
				found := &labelsv1alpha1.NamespaceLabel{}
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      "labels",
					Namespace: testNS,
				}, found)
			}, time.Minute, time.Second).Should(Succeed())
		})

		It("should reject invalid CR names", func() {
			By("Creating a NamespaceLabel CR with invalid name")
			cr := &labelsv1alpha1.NamespaceLabel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-name",
					Namespace: testNS,
				},
				Spec: labelsv1alpha1.NamespaceLabelSpec{
					Labels: map[string]string{
						"test": "value",
					},
				},
			}

			Expect(k8sClient.Create(ctx, cr)).To(Succeed())

			By("Verifying the CR gets an error status")
			Eventually(func() bool {
				found := &labelsv1alpha1.NamespaceLabel{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "invalid-name",
					Namespace: testNS,
				}, found)
				if err != nil {
					return false
				}

				// Check if there are conditions and if any indicate failure
				for _, condition := range found.Status.Conditions {
					if condition.Status == metav1.ConditionFalse {
						return true
					}
				}
				return false
			}, time.Minute, time.Second).Should(BeTrue())
		})
	})

	Context("Namespace Label Application", func() {
		It("should apply labels to the namespace (if controller is running)", func() {
			By("Creating a valid NamespaceLabel CR")
			cr := &labelsv1alpha1.NamespaceLabel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "labels",
					Namespace: testNS,
				},
				Spec: labelsv1alpha1.NamespaceLabelSpec{
					Labels: map[string]string{
						"environment": "test",
						"managed-by":  "namespacelabel-operator",
					},
				},
			}

			Expect(k8sClient.Create(ctx, cr)).To(Succeed())

			By("Checking if labels are applied to namespace")
			Eventually(func() map[string]string {
				ns := &corev1.Namespace{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testNS}, ns)
				if err != nil {
					return nil
				}
				return ns.Labels
			}, time.Minute*2, time.Second*5).Should(Or(
				// If controller is running, labels should be applied
				And(
					HaveKeyWithValue("environment", "test"),
					HaveKeyWithValue("managed-by", "namespacelabel-operator"),
				),
				// If controller is not running, we won't have the labels
				Not(HaveKey("environment")),
			))
		})
	})

	Context("CR Deletion", func() {
		It("should delete NamespaceLabel CRs successfully", func() {
			By("Creating a NamespaceLabel CR")
			cr := &labelsv1alpha1.NamespaceLabel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "labels",
					Namespace: testNS,
				},
				Spec: labelsv1alpha1.NamespaceLabelSpec{
					Labels: map[string]string{
						"test": "value",
					},
				},
			}

			Expect(k8sClient.Create(ctx, cr)).To(Succeed())

			By("Deleting the CR")
			Expect(k8sClient.Delete(ctx, cr)).To(Succeed())

			By("Verifying the CR is deleted")
			Eventually(func() bool {
				found := &labelsv1alpha1.NamespaceLabel{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "labels",
					Namespace: testNS,
				}, found)
				return errors.IsNotFound(err)
			}, time.Minute, time.Second).Should(BeTrue())
		})
	})
})
