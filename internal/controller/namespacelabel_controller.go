package controller

import (
	"context"
	"fmt"
	"strings"
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

func (r *NamespaceLabelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Create the controller without unnecessary namespace watch
	return ctrl.NewControllerManagedBy(mgr).
		For(&labelsv1alpha1.NamespaceLabel{}).
		Complete(r)
}

func (r *NamespaceLabelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	var current labelsv1alpha1.NamespaceLabel
	err := r.Get(ctx, req.NamespacedName, &current)
	exists := err == nil
	if err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	// Handle deletion
	if exists && current.DeletionTimestamp != nil {
		return r.finalize(ctx, &current)
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

	ns, err := r.getTargetNamespace(ctx, targetNS)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Process namespace labels with protection logic
	desired := current.Spec.Labels
	prevApplied := readAppliedAnnotation(ns)

	allProtectionPatterns := current.Spec.ProtectedLabelPatterns
	protectionMode := current.Spec.ProtectionMode

	// Apply protection logic
	if ns.Labels == nil {
		ns.Labels = map[string]string{}
	}

	protectionResult := applyProtectionLogic(
		desired,
		ns.Labels,
		allProtectionPatterns,
		protectionMode,
	)

	// If protection mode is "fail" and we hit protected labels, fail the reconciliation
	if protectionResult.ShouldFail {
		message := fmt.Sprintf("Protected label conflicts: %s", strings.Join(protectionResult.Warnings, "; "))
		updateStatus(&current, false, "ProtectedLabelConflict", message, protectionResult.ProtectedSkipped, nil)
		if err := r.Status().Update(ctx, &current); err != nil {
			l.Error(err, "failed to update status for protection conflict")
		}
		return ctrl.Result{RequeueAfter: time.Minute * 5}, fmt.Errorf("protected label conflict: %s", strings.Join(protectionResult.Warnings, "; "))
	}

	changed := r.applyLabelsToNamespace(ns, protectionResult.AllowedLabels, prevApplied)

	if changed {
		if err := r.Update(ctx, ns); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := writeAppliedAnnotation(ctx, r.Client, ns, protectionResult.AllowedLabels); err != nil {
		// Log error but don't fail reconciliation since labels were applied successfully
		l.Error(err, "failed to write applied annotation")
	}

	if exists {
		labelCount := len(desired)
		appliedCount := len(protectionResult.AllowedLabels)
		skippedCount := len(protectionResult.ProtectedSkipped)

		var message string
		if skippedCount > 0 {
			message = fmt.Sprintf("Applied %d labels to namespace '%s', skipped %d protected labels (%v)",
				appliedCount, targetNS, skippedCount, protectionResult.ProtectedSkipped)
		} else {
			message = fmt.Sprintf("Applied %d labels to namespace '%s'",
				appliedCount, targetNS)
		}

		appliedKeys := make([]string, 0, len(protectionResult.AllowedLabels))
		for k := range protectionResult.AllowedLabels {
			appliedKeys = append(appliedKeys, k)
		}

		l.Info("NamespaceLabel successfully processed",
			"namespace", current.Namespace, "labelsApplied", appliedCount, "labelsRequested", labelCount, "protectedSkipped", skippedCount)

		updateStatus(&current, true, "Synced", message, protectionResult.ProtectedSkipped, appliedKeys)
		if err := r.Status().Update(ctx, &current); err != nil {
			l.Error(err, "failed to update CR status")
		}
	}

	return ctrl.Result{}, nil
}

// finalize cleans up namespace labels and removes the finalizer
func (r *NamespaceLabelReconciler) finalize(ctx context.Context, cr *labelsv1alpha1.NamespaceLabel) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	ns, err := r.getTargetNamespace(ctx, cr.Namespace)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Namespace is gone - just remove finalizer
			controllerutil.RemoveFinalizer(cr, FinalizerName)
			return ctrl.Result{}, r.Update(ctx, cr)
		}
		return ctrl.Result{}, err
	}

	prevApplied := readAppliedAnnotation(ns)
	changed := r.applyLabelsToNamespace(ns, map[string]string{}, prevApplied)
	if changed {
		if err := r.Update(ctx, ns); err != nil {
			l.Error(err, "failed to remove applied labels")
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
	}

	if err := writeAppliedAnnotation(ctx, r.Client, ns, map[string]string{}); err != nil {
		l.Error(err, "failed to clear applied annotation")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	controllerutil.RemoveFinalizer(cr, FinalizerName)
	return ctrl.Result{}, r.Update(ctx, cr)
}

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

// applyLabelsToNamespace applies desired labels and removes stale ones
func (r *NamespaceLabelReconciler) applyLabelsToNamespace(ns *corev1.Namespace, desired, prevApplied map[string]string) bool {
	if ns.Labels == nil {
		ns.Labels = make(map[string]string)
	}

	changed := removeStaleLabels(ns.Labels, desired, prevApplied)
	changed = applyDesiredLabels(ns.Labels, desired) || changed
	return changed
}
