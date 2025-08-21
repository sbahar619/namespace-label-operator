package controller

import (
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	appliedAnnoKey = "labels.shahaf.com/applied" // JSON of map[string]string
	FinalizerName  = "labels.shahaf.com/finalizer"
	StandardCRName = "labels" // Standard name for NamespaceLabel CRs (singleton pattern)
)

// NamespaceLabelReconciler reconciles a NamespaceLabel object
type NamespaceLabelReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// ProtectionResult represents the result of applying protection logic
type ProtectionResult struct {
	AllowedLabels    map[string]string
	ProtectedSkipped []string
	Warnings         []string
	ShouldFail       bool
}
