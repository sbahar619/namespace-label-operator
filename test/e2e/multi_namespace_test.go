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
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sbahar619/namespace-label-operator/test/utils"
)

var _ = Describe("Multi-Namespace Tests", Label("multi-namespace"), func() {
	var (
		k8sClient client.Client
		ctx       context.Context
		testNSs   []string
	)

	BeforeEach(func() {
		ctx = context.Background()
		testNSs = make([]string, 0)

		By("Setting up Kubernetes client")
		var err error
		k8sClient, err = utils.GetK8sClient()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		By("Cleaning up all test namespaces")
		for _, ns := range testNSs {
			utils.CleanupNamespaceLabels(ctx, k8sClient, ns)

			nsObj := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: ns,
				},
			}
			err := k8sClient.Delete(ctx, nsObj)
			if err != nil && !errors.IsNotFound(err) {
				fmt.Printf("Warning: failed to delete namespace %s: %v\n", ns, err)
			}
		}
	})

	createTestNamespace := func(suffix string) string {
		nsName := fmt.Sprintf("multi-test-%s-%d-%d", suffix, time.Now().UnixNano(), rand.Int31())
		testNSs = append(testNSs, nsName)

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: nsName,
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		return nsName
	}

	Context("Isolation Between Namespaces", func() {
		It("should manage labels independently across namespaces", func() {
			ns1 := createTestNamespace("isolation-1")
			ns2 := createTestNamespace("isolation-2")
			ns3 := createTestNamespace("isolation-3")

			By("Creating different NamespaceLabel CRs in each namespace")
			utils.CreateNamespaceLabel(ctx, k8sClient, utils.CROptions{
				Labels: map[string]string{
					"environment": "development",
					"team":        "backend",
					"app":         "service-a",
				},
			}, ns1)

			utils.CreateNamespaceLabel(ctx, k8sClient, utils.CROptions{
				Labels: map[string]string{
					"environment": "staging",
					"team":        "frontend",
					"app":         "service-b",
				},
			}, ns2)

			utils.CreateNamespaceLabel(ctx, k8sClient, utils.CROptions{
				Labels: map[string]string{
					"environment": "production",
					"team":        "platform",
					"app":         "service-c",
				},
			}, ns3)

			By("Verifying each namespace has only its own labels")
			Eventually(utils.GetNamespaceLabels(ctx, k8sClient, ns1), time.Minute, time.Second).Should(And(
				HaveKeyWithValue("environment", "development"),
				HaveKeyWithValue("team", "backend"),
				HaveKeyWithValue("app", "service-a"),
			))

			Eventually(utils.GetNamespaceLabels(ctx, k8sClient, ns2), time.Minute, time.Second).Should(And(
				HaveKeyWithValue("environment", "staging"),
				HaveKeyWithValue("team", "frontend"),
				HaveKeyWithValue("app", "service-b"),
			))

			Eventually(utils.GetNamespaceLabels(ctx, k8sClient, ns3), time.Minute, time.Second).Should(And(
				HaveKeyWithValue("environment", "production"),
				HaveKeyWithValue("team", "platform"),
				HaveKeyWithValue("app", "service-c"),
			))

			By("Verifying namespaces don't have each other's labels")
			Consistently(utils.GetNamespaceLabels(ctx, k8sClient, ns1), time.Second*10, time.Second).ShouldNot(And(
				HaveKeyWithValue("team", "frontend"),
				HaveKeyWithValue("app", "service-b"),
			))
		})

		It("should handle concurrent operations across multiple namespaces", func() {
			const numNamespaces = 5
			namespaces := make([]string, numNamespaces)

			By("Creating multiple namespaces")
			for i := 0; i < numNamespaces; i++ {
				namespaces[i] = createTestNamespace(fmt.Sprintf("concurrent-%d", i))
			}

			By("Creating NamespaceLabel CRs concurrently")
			var wg sync.WaitGroup
			wg.Add(numNamespaces)

			for i := 0; i < numNamespaces; i++ {
				go func(index int) {
					defer wg.Done()
					defer GinkgoRecover()

					utils.CreateNamespaceLabel(ctx, k8sClient, utils.CROptions{
						Labels: map[string]string{
							"namespace-id": fmt.Sprintf("ns-%d", index),
							"batch":        "concurrent-test",
							"index":        fmt.Sprintf("%d", index),
						},
					}, namespaces[index])
				}(i)
			}

			wg.Wait()

			By("Verifying all namespaces have their correct labels")
			for i := 0; i < numNamespaces; i++ {
				nsIndex := i // Capture for closure
				Eventually(utils.GetNamespaceLabels(ctx, k8sClient, namespaces[nsIndex]), time.Minute*2, time.Second).Should(And(
					HaveKeyWithValue("namespace-id", fmt.Sprintf("ns-%d", nsIndex)),
					HaveKeyWithValue("batch", "concurrent-test"),
					HaveKeyWithValue("index", fmt.Sprintf("%d", nsIndex)),
				))
			}
		})
	})

	Context("Namespace Lifecycle", func() {
		It("should handle namespace deletion while CR exists", func() {
			ns := createTestNamespace("lifecycle")

			By("Creating NamespaceLabel CR")
			utils.CreateNamespaceLabel(ctx, k8sClient, utils.CROptions{
				Labels: map[string]string{
					"lifecycle-test": "true",
					"environment":    "test",
				},
			}, ns)

			By("Verifying labels are applied")
			Eventually(utils.GetNamespaceLabels(ctx, k8sClient, ns), time.Minute, time.Second).Should(
				HaveKeyWithValue("lifecycle-test", "true"),
			)

			By("Deleting the namespace while CR exists")
			nsObj := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: ns,
				},
			}
			Expect(k8sClient.Delete(ctx, nsObj)).To(Succeed())

			By("Verifying namespace is eventually deleted")
			Eventually(func() bool {
				checkNS := &corev1.Namespace{}
				err := k8sClient.Get(ctx, client.ObjectKey{Name: ns}, checkNS)
				return errors.IsNotFound(err)
			}, time.Minute*3, time.Second*2).Should(BeTrue())

			// Remove from cleanup list since we manually deleted it
			for i, name := range testNSs {
				if name == ns {
					testNSs = append(testNSs[:i], testNSs[i+1:]...)
					break
				}
			}
		})

	})

	Context("Cross-Namespace Protection Scenarios", func() {
		It("should handle same protection patterns across different namespaces", func() {
			ns1 := createTestNamespace("protection-1")
			ns2 := createTestNamespace("protection-2")

			By("Pre-setting different protected labels in each namespace")
			utils.SetNamespaceLabel(ctx, k8sClient, ns1, "kubernetes.io/managed-by", "system-a")
			utils.SetNamespaceLabel(ctx, k8sClient, ns2, "kubernetes.io/managed-by", "system-b")

			By("Creating CRs with same protection patterns but different modes")
			utils.CreateNamespaceLabel(ctx, k8sClient, utils.CROptions{
				Labels: map[string]string{
					"app":                      "service-1",
					"kubernetes.io/managed-by": "operator", // Should be skipped
				},
				ProtectedLabelPatterns: []string{"kubernetes.io/*"},
				ProtectionMode:         "skip",
			}, ns1)

			utils.CreateNamespaceLabel(ctx, k8sClient, utils.CROptions{
				Labels: map[string]string{
					"app":                      "service-2",
					"kubernetes.io/managed-by": "operator", // Should cause failure
				},
				ProtectedLabelPatterns: []string{"kubernetes.io/*"},
				ProtectionMode:         "fail",
			}, ns2)

			By("Verifying different protection behaviors")
			Eventually(utils.GetNamespaceLabels(ctx, k8sClient, ns1), time.Minute, time.Second).Should(And(
				HaveKeyWithValue("app", "service-1"),
				HaveKeyWithValue("kubernetes.io/managed-by", "system-a"), // Protected
			))

			// For ns2, the protected label should remain unchanged due to fail mode
			Eventually(utils.GetNamespaceLabels(ctx, k8sClient, ns2), time.Minute, time.Second).Should(
				HaveKeyWithValue("kubernetes.io/managed-by", "system-b"), // Should remain unchanged
			)

			By("Verifying protection status is reported correctly for each namespace")
			Eventually(func() []string {
				status := utils.GetCRStatus(ctx, k8sClient, "labels", ns1)()
				if status == nil {
					return nil
				}
				return status.ProtectedLabelsSkipped
			}, time.Minute, time.Second).Should(ContainElement("kubernetes.io/managed-by"))
		})
	})
})
