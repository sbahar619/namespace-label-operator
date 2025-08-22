package controller

import (
	"context"
	"fmt"
	"time"

	labelsv1alpha1 "github.com/sbahar619/namespace-label-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// RBAC: access our CRD + update Namespaces.
// +kubebuilder:rbac:groups=labels.shahaf.com,resources=namespacelabels,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=labels.shahaf.com,resources=namespacelabels/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=labels.shahaf.com,resources=namespacelabels/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;update;patch

// getTargetNamespace retrieves the namespace that should be modified
func (r *NamespaceLabelReconciler) getTargetNamespace(ctx context.Context, targetNS string) (*corev1.Namespace, error) {
	if targetNS == "" {
		return nil, fmt.Errorf("empty namespace name")
	}

	var ns corev1.Namespace
	if err := r.Get(ctx, types.NamespacedName{Name: targetNS}, &ns); err != nil {
		return nil, err
	}
	return &ns, nil
}

func (r *NamespaceLabelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	// Fetch the CR if it still exists
	var current labelsv1alpha1.NamespaceLabel
	err := r.Get(ctx, req.NamespacedName, &current)
	exists := err == nil
	if err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	// Note: Singleton pattern validation is now handled by the admission webhook
	// No need to validate name or check for multiple CRs here

	// Handle deletion
	if exists && current.DeletionTimestamp != nil {
		return r.handleDeletion(ctx, &current)
	}

	// Add finalizer if it doesn't exist and CR exists
	if exists {
		if !controllerutil.ContainsFinalizer(&current, FinalizerName) {
			controllerutil.AddFinalizer(&current, FinalizerName)
			if err := r.Update(ctx, &current); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil // Stop reconciliation after adding finalizer
		}
	}

	// Target namespace is always the same as the CR's namespace for multi-tenant security
	targetNS := req.Namespace

	// Get the target Namespace object to modify its labels
	ns, err := r.getTargetNamespace(ctx, targetNS)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Process namespace labels with protection logic
	protectionResult, desired, err := r.processNamespaceLabels(ctx, &current, ns)
	if err != nil {
		if protectionResult != nil {
			// This means we had a protection conflict - requeue with delay
			return ctrl.Result{RequeueAfter: time.Minute * 5}, err
		}
		return ctrl.Result{}, err
	}

	// Update status if the CR still exists
	if exists {
		if err := r.updateSuccessStatus(ctx, &current, desired, protectionResult.AllowedLabels, *protectionResult, targetNS); err != nil {
			l.Error(err, "failed to update CR status")
		}
	}

	return ctrl.Result{}, nil
}

func (r *NamespaceLabelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Create the controller without unnecessary namespace watch
	return ctrl.NewControllerManagedBy(mgr).
		For(&labelsv1alpha1.NamespaceLabel{}).
		Complete(r)
}
