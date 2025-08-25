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

var _ = Describe("NamespaceLabel Controller Tests", Label("controller"), func() {
	var (
		k8sClient client.Client
		ctx       context.Context
		testNS    string
	)

	BeforeEach(func() {
		ctx = context.Background()
		// Use nanoseconds and random number to avoid collisions
		testNS = fmt.Sprintf("controller-test-%d-%d", time.Now().UnixNano(), rand.Int31())

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
		utils.CleanupNamespaceLabels(ctx, k8sClient, testNS)

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
			utils.CreateNamespaceLabel(ctx, k8sClient, utils.CROptions{
				Labels: map[string]string{
					"environment": "test",
					"team":        "platform",
				},
			}, testNS)

			By("Verifying the CR was created")
			utils.WaitForCRToExist(ctx, k8sClient, "labels", testNS)
		})

		It("should delete NamespaceLabel CRs successfully", func() {
			By("Creating a NamespaceLabel CR")
			cr := utils.CreateNamespaceLabel(ctx, k8sClient, utils.CROptions{
				Labels: map[string]string{
					"test": "value",
				},
			}, testNS)

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

	Context("Namespace Label Application", func() {
		It("should apply labels to the namespace", func() {
			By("Creating a valid NamespaceLabel CR")
			utils.CreateNamespaceLabel(ctx, k8sClient, utils.CROptions{
				Labels: map[string]string{
					"environment": "test",
					"managed-by":  "namespacelabel-operator",
				},
			}, testNS)

			By("Checking if labels are applied to namespace")
			Eventually(utils.GetNamespaceLabels(ctx, k8sClient, testNS), time.Minute*2, time.Second*5).Should(And(
				HaveKeyWithValue("environment", "test"),
				HaveKeyWithValue("managed-by", "namespacelabel-operator"),
			))
		})
	})

	Context("Label Protection", func() {
		It("should skip protected labels in skip mode", func() {
			By("Pre-setting a protected label on the namespace")
			utils.SetNamespaceLabel(ctx, k8sClient, testNS, "kubernetes.io/managed-by", "system")

			By("Creating a NamespaceLabel CR with protection patterns")
			utils.CreateNamespaceLabel(ctx, k8sClient, utils.CROptions{
				Labels: map[string]string{
					"environment":              "test",
					"kubernetes.io/managed-by": "namespacelabel-operator", // This should be skipped
				},
				ProtectedLabelPatterns: []string{"kubernetes.io/*"},
				ProtectionMode:         "skip",
			}, testNS)

			By("Verifying the environment label was applied but protected label was skipped")
			Eventually(utils.GetNamespaceLabels(ctx, k8sClient, testNS), time.Minute, time.Second*2).Should(And(
				HaveKeyWithValue("environment", "test"),                // Should be applied
				HaveKeyWithValue("kubernetes.io/managed-by", "system"), // Should remain unchanged
			))

			By("Checking the status shows skipped labels")
			Eventually(func() []string {
				status := utils.GetCRStatus(ctx, k8sClient, "labels", testNS)()
				if status == nil {
					return nil
				}
				return status.ProtectedLabelsSkipped
			}, time.Minute, time.Second).Should(ContainElement("kubernetes.io/managed-by"))
		})

		It("should prevent protection bypass through annotation race condition", func() {
			By("Pre-setting a protected label on the namespace")
			originalValue := "original-system-value"
			utils.SetNamespaceLabel(ctx, k8sClient, testNS, "system.io/managed-by", originalValue)

			By("Creating a NamespaceLabel CR attempting to override the protected label")
			utils.CreateNamespaceLabel(ctx, k8sClient, utils.CROptions{
				Labels: map[string]string{
					"environment":          "production",   // This should be applied
					"system.io/managed-by": "hacker-value", // This should be blocked by protection
					"tier":                 "critical",     // This should be applied
				},
				ProtectedLabelPatterns: []string{"system.io/*"},
				ProtectionMode:         "warn",
			}, testNS)

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
				labels := utils.GetNamespaceLabels(ctx, k8sClient, testNS)()
				if labels == nil {
					return ""
				}
				return labels["system.io/managed-by"]
			}, time.Second*10, time.Second).Should(Equal(originalValue),
				"Protected label should never change from original value")

			By("Verifying non-protected labels were applied correctly")
			Eventually(utils.GetNamespaceLabels(ctx, k8sClient, testNS), time.Minute, time.Second).Should(And(
				HaveKeyWithValue("environment", "production"),           // Should be applied
				HaveKeyWithValue("tier", "critical"),                    // Should be applied
				HaveKeyWithValue("update-counter", "update-4"),          // Should have latest update
				HaveKeyWithValue("system.io/managed-by", originalValue), // Should remain original
			))

			By("Verifying the status consistently shows the protected label was skipped")
			Eventually(func() []string {
				status := utils.GetCRStatus(ctx, k8sClient, "labels", testNS)()
				if status == nil {
					return nil
				}
				return status.ProtectedLabelsSkipped
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

	Context("Protection Modes", func() {
		It("should fail reconciliation in fail mode when protected labels conflict", func() {
			By("Pre-setting a protected label on the namespace")
			utils.SetNamespaceLabel(ctx, k8sClient, testNS, "kubernetes.io/managed-by", "existing-system")

			By("Creating a NamespaceLabel CR with fail protection mode")
			utils.CreateNamespaceLabel(ctx, k8sClient, utils.CROptions{
				Labels: map[string]string{
					"environment":              "test",
					"kubernetes.io/managed-by": "operator", // This should cause failure
				},
				ProtectedLabelPatterns: []string{"kubernetes.io/*"},
				ProtectionMode:         "fail",
			}, testNS)

			By("Verifying the CR gets a failure status")
			Eventually(func() bool {
				found := &labelsv1alpha1.NamespaceLabel{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "labels",
					Namespace: testNS,
				}, found)
				if err != nil {
					return false
				}

				// Check for failure condition
				for _, condition := range found.Status.Conditions {
					if condition.Type == "Ready" && condition.Status == metav1.ConditionFalse {
						return true
					}
				}
				return false
			}, time.Minute, time.Second).Should(BeTrue())

			By("Verifying protected label remains unchanged")
			Consistently(func() string {
				updatedNS := &corev1.Namespace{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testNS}, updatedNS)
				if err != nil || updatedNS.Labels == nil {
					return ""
				}
				return updatedNS.Labels["kubernetes.io/managed-by"]
			}, time.Second*10, time.Second).Should(Equal("existing-system"))
		})

		It("should warn about protected labels in warn mode", func() {
			By("Pre-setting a protected label on the namespace")
			utils.SetNamespaceLabel(ctx, k8sClient, testNS, "istio.io/injection", "enabled")

			By("Creating a NamespaceLabel CR with warn protection mode")
			utils.CreateNamespaceLabel(ctx, k8sClient, utils.CROptions{
				Labels: map[string]string{
					"environment":        "production",
					"istio.io/injection": "disabled", // This should be skipped with warning
				},
				ProtectedLabelPatterns: []string{"istio.io/*"},
				ProtectionMode:         "warn",
			}, testNS)

			By("Verifying non-protected labels are applied")
			Eventually(func() string {
				updatedNS := &corev1.Namespace{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testNS}, updatedNS)
				if err != nil || updatedNS.Labels == nil {
					return ""
				}
				return updatedNS.Labels["environment"]
			}, time.Minute, time.Second).Should(Equal("production"))

			By("Verifying protected label remains unchanged")
			Consistently(func() string {
				updatedNS := &corev1.Namespace{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testNS}, updatedNS)
				if err != nil || updatedNS.Labels == nil {
					return ""
				}
				return updatedNS.Labels["istio.io/injection"]
			}, time.Second*5, time.Second).Should(Equal("enabled"))

			By("Verifying the status shows skipped labels")
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
			}, time.Minute, time.Second).Should(ContainElement("istio.io/injection"))
		})
	})

	Context("Label Updates and Removal", func() {
		It("should update namespace labels when CR is modified", func() {
			By("Creating a NamespaceLabel CR")
			utils.CreateNamespaceLabel(ctx, k8sClient, utils.CROptions{
				Labels: map[string]string{
					"environment": "dev",
					"version":     "v1",
				},
			}, testNS)

			By("Waiting for labels to be applied")
			Eventually(utils.GetNamespaceLabels(ctx, k8sClient, testNS), time.Minute, time.Second).Should(And(
				HaveKeyWithValue("environment", "dev"),
				HaveKeyWithValue("version", "v1"),
			))

			By("Updating the CR to change and remove labels")
			Eventually(func() error {
				freshCR := &labelsv1alpha1.NamespaceLabel{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: "labels", Namespace: testNS}, freshCR); err != nil {
					return err
				}
				freshCR.Spec.Labels = map[string]string{
					"environment": "production", // Changed value
					"tier":        "critical",   // New label
					// "version" removed
				}
				return k8sClient.Update(ctx, freshCR)
			}, time.Second*10, time.Millisecond*100).Should(Succeed())

			By("Verifying namespace labels are updated accordingly")
			Eventually(utils.GetNamespaceLabels(ctx, k8sClient, testNS), time.Minute, time.Second).Should(And(
				HaveKeyWithValue("environment", "production"), // Updated
				HaveKeyWithValue("tier", "critical"),          // Added
				Not(HaveKey("version")),                       // Removed
			))
		})

		It("should clean up all labels when CR is deleted", func() {
			By("Creating a NamespaceLabel CR")
			cr := utils.CreateNamespaceLabel(ctx, k8sClient, utils.CROptions{
				Labels: map[string]string{
					"cleanup-test":     "true",
					"operator-managed": "namespacelabel",
				},
			}, testNS)

			By("Waiting for labels to be applied")
			Eventually(utils.GetNamespaceLabels(ctx, k8sClient, testNS), time.Minute, time.Second).Should(And(
				HaveKeyWithValue("cleanup-test", "true"),
				HaveKeyWithValue("operator-managed", "namespacelabel"),
			))

			By("Deleting the CR")
			Expect(k8sClient.Delete(ctx, cr)).To(Succeed())

			By("Verifying operator-managed labels are removed from namespace")
			Eventually(utils.GetNamespaceLabels(ctx, k8sClient, testNS), time.Minute, time.Second).Should(And(
				Not(HaveKey("cleanup-test")),
				Not(HaveKey("operator-managed")),
			))
		})
	})

	Context("Status Reporting", func() {
		It("should report correct status with applied and skipped labels", func() {
			By("Pre-setting a protected label on the namespace")
			utils.SetNamespaceLabel(ctx, k8sClient, testNS, "app.kubernetes.io/managed-by", "helm")

			By("Creating a NamespaceLabel CR with mixed protected and non-protected labels")
			utils.CreateNamespaceLabel(ctx, k8sClient, utils.CROptions{
				Labels: map[string]string{
					"environment":                  "test",     // Should be applied
					"team":                         "platform", // Should be applied
					"app.kubernetes.io/managed-by": "operator", // Should be skipped
				},
				ProtectedLabelPatterns: []string{"app.kubernetes.io/*"},
				ProtectionMode:         "skip",
			}, testNS)

			By("Verifying the status accurately reflects applied and skipped labels")
			Eventually(func() bool {
				found := &labelsv1alpha1.NamespaceLabel{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "labels",
					Namespace: testNS,
				}, found)
				if err != nil {
					return false
				}

				// Check applied labels
				expectedApplied := []string{"environment", "team"}
				if len(found.Status.LabelsApplied) != len(expectedApplied) {
					return false
				}
				for _, label := range expectedApplied {
					labelFound := false
					for _, applied := range found.Status.LabelsApplied {
						if applied == label {
							labelFound = true
							break
						}
					}
					if !labelFound {
						return false
					}
				}

				// Check skipped labels
				expectedSkipped := []string{"app.kubernetes.io/managed-by"}
				if len(found.Status.ProtectedLabelsSkipped) != len(expectedSkipped) {
					return false
				}
				for _, label := range expectedSkipped {
					labelFound := false
					for _, skipped := range found.Status.ProtectedLabelsSkipped {
						if skipped == label {
							labelFound = true
							break
						}
					}
					if !labelFound {
						return false
					}
				}

				// Check overall status
				return found.Status.Applied == true
			}, time.Minute, time.Second).Should(BeTrue())

			By("Verifying the Ready condition contains appropriate message")
			Eventually(func() bool {
				found := &labelsv1alpha1.NamespaceLabel{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "labels",
					Namespace: testNS,
				}, found)
				if err != nil {
					return false
				}

				for _, condition := range found.Status.Conditions {
					if condition.Type == "Ready" && condition.Status == metav1.ConditionTrue {
						// Should mention both applied and skipped counts
						return condition.Message != "" &&
							condition.Reason == "Synced"
					}
				}
				return false
			}, time.Minute, time.Second).Should(BeTrue())
		})
	})
})
