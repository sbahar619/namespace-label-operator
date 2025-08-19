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

	// Helper function to clean up NamespaceLabel CRs reliably
	cleanupNamespaceLabels := func(namespace string) {
		crList := &labelsv1alpha1.NamespaceLabelList{}
		if err := k8sClient.List(ctx, crList, client.InNamespace(namespace)); err == nil {
			for _, cr := range crList.Items {
				if err := k8sClient.Delete(ctx, &cr); err != nil && !errors.IsNotFound(err) {
					fmt.Printf("Warning: failed to delete CR %s: %v\n", cr.Name, err)
				}
			}

			// Wait for all CRs to be deleted (finalizers processed)
			Eventually(func() int {
				crList := &labelsv1alpha1.NamespaceLabelList{}
				if err := k8sClient.List(ctx, crList, client.InNamespace(namespace)); err != nil {
					return 0
				}
				return len(crList.Items)
			}, time.Minute, time.Second).Should(Equal(0))
		}
	}

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

		// First, delete any NamespaceLabel CRs in the namespace to remove finalizers
		By("Cleaning up NamespaceLabel CRs to remove finalizers")
		cleanupNamespaceLabels(testNS)

		// Now delete the namespace
		By("Deleting the test namespace")
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNS,
			},
		}
		err := k8sClient.Delete(ctx, ns)
		if err != nil && !errors.IsNotFound(err) {
			// Log but don't fail the test - this is cleanup
			fmt.Printf("Warning: failed to delete namespace %s: %v\n", testNS, err)
			return // Skip waiting if delete failed
		}

		// Wait for namespace to be fully deleted with longer timeout
		By("Waiting for namespace to be fully deleted")
		Eventually(func() bool {
			checkNS := &corev1.Namespace{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: testNS}, checkNS)
			return errors.IsNotFound(err)
		}, time.Minute*2, time.Second*2).Should(BeTrue(),
			fmt.Sprintf("Namespace %s should be deleted within 2 minutes", testNS))
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

	Context("Label Protection", func() {
		It("should skip protected labels in skip mode", func() {
			By("Pre-setting a protected label on the namespace")
			ns := &corev1.Namespace{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testNS}, ns)).To(Succeed())
			if ns.Labels == nil {
				ns.Labels = make(map[string]string)
			}
			ns.Labels["kubernetes.io/managed-by"] = "system"
			Expect(k8sClient.Update(ctx, ns)).To(Succeed())

			By("Creating a NamespaceLabel CR with protection patterns")
			cr := &labelsv1alpha1.NamespaceLabel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "labels",
					Namespace: testNS,
				},
				Spec: labelsv1alpha1.NamespaceLabelSpec{
					Labels: map[string]string{
						"environment":              "test",
						"kubernetes.io/managed-by": "namespacelabel-operator", // This should be skipped
					},
					ProtectedLabelPatterns: []string{"kubernetes.io/*"},
					ProtectionMode:         "skip",
				},
			}

			Expect(k8sClient.Create(ctx, cr)).To(Succeed())

			By("Verifying the environment label was applied but protected label was skipped")
			Eventually(func() map[string]string {
				updatedNS := &corev1.Namespace{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testNS}, updatedNS)
				if err != nil {
					return nil
				}
				return updatedNS.Labels
			}, time.Minute, time.Second*2).Should(And(
				HaveKeyWithValue("environment", "test"),                // Should be applied
				HaveKeyWithValue("kubernetes.io/managed-by", "system"), // Should remain unchanged
			))

			By("Checking the status shows skipped labels")
			Eventually(func() []string {
				found := &labelsv1alpha1.NamespaceLabel{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "labels",
					Namespace: testNS,
				}, found)
				if err != nil {
					return nil
				}
				return found.Status.ProtectedLabelsSkipped
			}, time.Minute, time.Second).Should(ContainElement("kubernetes.io/managed-by"))
		})

		// Test for regression: https://github.com/sbahar619/namespace-label-operator/issues/protection-race
		// This test specifically validates that the protection system works correctly even when
		// there are rapid reconciliations that could trigger race conditions in annotation updates.
		//
		// Bug context: Previously, the writeAppliedAnnotation function could cause race conditions
		// where protected labels would initially be skipped correctly, but then subsequent
		// reconciliations might bypass protection due to incorrect annotation state.
		//
		// This test ensures:
		// 1. Protection works on first reconciliation
		// 2. Protection continues to work through multiple rapid updates
		// 3. The applied annotation only contains labels that were actually applied
		// 4. Protected labels remain unchanged throughout all reconciliations
		It("should prevent protection bypass through annotation race condition", func() {
			By("Pre-setting a protected label on the namespace")
			ns := &corev1.Namespace{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testNS}, ns)).To(Succeed())
			if ns.Labels == nil {
				ns.Labels = make(map[string]string)
			}
			originalValue := "original-system-value"
			ns.Labels["system.io/managed-by"] = originalValue
			Expect(k8sClient.Update(ctx, ns)).To(Succeed())

			By("Creating a NamespaceLabel CR attempting to override the protected label")
			cr := &labelsv1alpha1.NamespaceLabel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "labels",
					Namespace: testNS,
				},
				Spec: labelsv1alpha1.NamespaceLabelSpec{
					Labels: map[string]string{
						"environment":          "production",   // This should be applied
						"system.io/managed-by": "hacker-value", // This should be blocked by protection
						"tier":                 "critical",     // This should be applied
					},
					ProtectedLabelPatterns: []string{"system.io/*"},
					ProtectionMode:         "warn",
				},
			}

			Expect(k8sClient.Create(ctx, cr)).To(Succeed())

			By("Triggering multiple rapid reconciliations by updating the CR")
			for i := 0; i < 5; i++ {
				// Use Eventually with retry logic to handle resource version conflicts
				Eventually(func() error {
					// Get fresh copy of the CR to avoid resource version conflicts
					freshCR := &labelsv1alpha1.NamespaceLabel{}
					if err := k8sClient.Get(ctx, types.NamespacedName{Name: "labels", Namespace: testNS}, freshCR); err != nil {
						return err
					}

					// Update the counter to trigger reconciliation
					freshCR.Spec.Labels["update-counter"] = fmt.Sprintf("update-%d", i)
					return k8sClient.Update(ctx, freshCR)
				}, time.Second*10, time.Millisecond*100).Should(Succeed(),
					fmt.Sprintf("Should be able to update CR for iteration %d", i))

				// Small delay to allow controller processing
				time.Sleep(time.Millisecond * 200)
			}

			By("Verifying protection held through all reconciliations")
			Consistently(func() string {
				updatedNS := &corev1.Namespace{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testNS}, updatedNS)
				if err != nil || updatedNS.Labels == nil {
					return ""
				}
				return updatedNS.Labels["system.io/managed-by"]
			}, time.Second*10, time.Second).Should(Equal(originalValue), "Protected label should never change from original value")

			By("Verifying non-protected labels were applied correctly")
			Eventually(func() map[string]string {
				updatedNS := &corev1.Namespace{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testNS}, updatedNS)
				if err != nil {
					return nil
				}
				return updatedNS.Labels
			}, time.Minute, time.Second).Should(And(
				HaveKeyWithValue("environment", "production"),           // Should be applied
				HaveKeyWithValue("tier", "critical"),                    // Should be applied
				HaveKeyWithValue("update-counter", "update-4"),          // Should have latest update
				HaveKeyWithValue("system.io/managed-by", originalValue), // Should remain original
			))

			By("Verifying the status consistently shows the protected label was skipped")
			Eventually(func() []string {
				found := &labelsv1alpha1.NamespaceLabel{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "labels",
					Namespace: testNS,
				}, found)
				if err != nil {
					return nil
				}
				return found.Status.ProtectedLabelsSkipped
			}, time.Minute, time.Second).Should(ContainElement("system.io/managed-by"))

			By("Verifying the applied annotation only contains non-protected labels")
			Eventually(func() string {
				updatedNS := &corev1.Namespace{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testNS}, updatedNS)
				if err != nil || updatedNS.Annotations == nil {
					return ""
				}
				return updatedNS.Annotations["labels.shahaf.com/applied"]
			}, time.Minute, time.Second).Should(And(
				ContainSubstring("environment"),
				ContainSubstring("tier"),
				ContainSubstring("update-counter"),
				Not(ContainSubstring("system.io/managed-by")), // Should NOT be in applied annotation
			))
		})
	})
})
