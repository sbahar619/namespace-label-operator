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

package utils

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:golint,revive
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	labelsv1alpha1 "github.com/sbahar619/namespace-label-operator/api/v1alpha1"
)

// Run executes the provided command within this context
func Run(cmd *exec.Cmd) ([]byte, error) {
	dir, _ := GetProjectDir()
	cmd.Dir = dir

	if err := os.Chdir(cmd.Dir); err != nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "chdir dir: %s\n", err)
	}

	cmd.Env = append(os.Environ(), "GO111MODULE=on")
	command := strings.Join(cmd.Args, " ")
	_, _ = fmt.Fprintf(GinkgoWriter, "running: %s\n", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("%s failed with error: (%v) %s", command, err, string(output))
	}

	return output, nil
}

// GetProjectDir will return the directory where the project is
func GetProjectDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return wd, err
	}
	wd = strings.Replace(wd, "/test/e2e", "", -1)
	return wd, nil
}

// GetK8sClient returns a controller-runtime client configured for the current kubeconfig
func GetK8sClient() (client.Client, error) {
	// Create the scheme and add our custom types
	s := runtime.NewScheme()
	if err := scheme.AddToScheme(s); err != nil {
		return nil, fmt.Errorf("failed to add core types to scheme: %v", err)
	}
	if err := labelsv1alpha1.AddToScheme(s); err != nil {
		return nil, fmt.Errorf("failed to add NamespaceLabel types to scheme: %v", err)
	}

	// Get the kubeconfig
	config, err := getKubeConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig: %v", err)
	}

	// Create the client
	k8sClient, err := client.New(config, client.Options{Scheme: s})
	if err != nil {
		return nil, fmt.Errorf("failed to create k8s client: %v", err)
	}

	return k8sClient, nil
}

// getKubeConfig returns the kubeconfig for the current environment
func getKubeConfig() (*rest.Config, error) {
	// Try in-cluster config first
	config, err := rest.InClusterConfig()
	if err == nil {
		return config, nil
	}

	// Fall back to kubeconfig file
	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath == "" {
		kubeconfigPath = clientcmd.RecommendedHomeFile
	}

	config, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build kubeconfig: %v", err)
	}

	return config, nil
}

// CleanupNamespaceLabels cleans up NamespaceLabel CRs reliably
// This function handles finalizers that are managed by the controller
func CleanupNamespaceLabels(ctx context.Context, k8sClient client.Client, namespace string) {
	crList := &labelsv1alpha1.NamespaceLabelList{}
	if err := k8sClient.List(ctx, crList, client.InNamespace(namespace)); err == nil {
		// First attempt: normal deletion
		for _, cr := range crList.Items {
			if err := k8sClient.Delete(ctx, &cr); err != nil && !errors.IsNotFound(err) {
				fmt.Printf("Warning: failed to delete CR %s: %v\n", cr.Name, err)
			}
		}

		// Give the controller a chance to process finalizers (if running)
		time.Sleep(time.Second * 2)

		// Check if CRs still exist after initial deletion attempt
		crList = &labelsv1alpha1.NamespaceLabelList{}
		remainingCRs := 0
		if err := k8sClient.List(ctx, crList, client.InNamespace(namespace)); err == nil {
			remainingCRs = len(crList.Items)
		}

		if remainingCRs > 0 {
			fmt.Printf("Found %d remaining CRs, manually removing finalizers for namespace %s\n", remainingCRs, namespace)
			// If CRs still exist, manually remove finalizers (controller might not be running)
			for _, cr := range crList.Items {
				// Get fresh copy and remove finalizers
				freshCR := &labelsv1alpha1.NamespaceLabel{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: cr.Namespace}, freshCR); err == nil {
					if len(freshCR.Finalizers) > 0 {
						freshCR.Finalizers = nil
						if err := k8sClient.Update(ctx, freshCR); err != nil && !errors.IsNotFound(err) {
							fmt.Printf("Warning: failed to remove finalizers from CR %s: %v\n", cr.Name, err)
						}
					}
				}
			}
		}

		// Final wait for all CRs to be deleted
		Eventually(func() int {
			crList := &labelsv1alpha1.NamespaceLabelList{}
			if err := k8sClient.List(ctx, crList, client.InNamespace(namespace)); err != nil {
				return 0
			}
			return len(crList.Items)
		}, time.Second*30, time.Second).Should(Equal(0))
	}
}
