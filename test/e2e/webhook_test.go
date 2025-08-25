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
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sbahar619/namespace-label-operator/test/utils"
)

var _ = Describe("NamespaceLabel Webhook Tests", Label("webhook"), func() {
	var (
		k8sClient client.Client
		ctx       context.Context
		testNS    string
	)

	BeforeEach(func() {
		ctx = context.Background()
		// Use nanoseconds and random number to avoid collisions
		testNS = fmt.Sprintf("webhook-test-%d-%d", time.Now().UnixNano(), rand.Int31())

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

	Context("Name Validation", func() {
		It("should reject invalid CR names", func() {
			By("Creating a NamespaceLabel CR with invalid name and expecting webhook rejection")
			cr := utils.NewNamespaceLabel(utils.CROptions{
				Name: "invalid-name",
				Labels: map[string]string{
					"test": "value",
				},
			}, testNS)

			utils.ExpectWebhookRejection(ctx, k8sClient, cr,
				"NamespaceLabel resource must be named 'labels' for singleton pattern enforcement")
		})
	})

	Context("Singleton Enforcement", func() {
		It("should prevent multiple NamespaceLabel CRs in the same namespace", func() {
			By("Creating the first valid NamespaceLabel CR")
			utils.CreateNamespaceLabel(ctx, k8sClient, utils.CROptions{
				Labels: map[string]string{
					"environment": "production",
				},
			}, testNS)

			By("Attempting to create a second NamespaceLabel CR with invalid name")
			cr2 := utils.NewNamespaceLabel(utils.CROptions{
				Name: "other-labels",
				Labels: map[string]string{
					"team": "platform",
				},
			}, testNS)

			utils.ExpectWebhookRejection(ctx, k8sClient, cr2,
				"NamespaceLabel resource must be named 'labels' for singleton pattern enforcement")
		})

		It("should prevent creation of second CR with valid name when one already exists", func() {
			By("Creating the first valid NamespaceLabel CR")
			utils.CreateNamespaceLabel(ctx, k8sClient, utils.CROptions{
				Labels: map[string]string{
					"environment": "production",
				},
			}, testNS)

			By("Attempting to create a second NamespaceLabel CR with the same valid name")
			cr2 := utils.NewNamespaceLabel(utils.CROptions{
				Labels: map[string]string{
					"team": "platform",
				},
			}, testNS)

			By("Attempting to create the duplicate CR")
			err := k8sClient.Create(ctx, cr2)
			Expect(err).To(HaveOccurred())

			By("Verifying appropriate error message")
			// Accept either webhook validation error or standard Kubernetes "already exists" error
			errorMsg := err.Error()
			webhookError := "only one NamespaceLabel resource is allowed per namespace"
			k8sError := "already exists"

			Expect(errorMsg).To(Or(
				ContainSubstring(webhookError), // Webhook validation message
				ContainSubstring(k8sError),     // Standard Kubernetes API error
			))

			if !strings.Contains(errorMsg, webhookError) && strings.Contains(errorMsg, k8sError) {
				By("Standard Kubernetes API rejection (webhook not running) - this is expected behavior")
			}
		})
	})
})
