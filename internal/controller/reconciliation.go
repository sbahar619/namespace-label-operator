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

// processNamespaceLabels handles the core label processing logic
func (r *NamespaceLabelReconciler) processNamespaceLabels(ctx context.Context, cr *labelsv1alpha1.NamespaceLabel, ns *corev1.Namespace) (*ProtectionResult, map[string]string, error) {
	l := log.FromContext(ctx)

	// Since we enforce singleton pattern, use the current CR directly
	desired := cr.Spec.Labels

	// Load what we previously applied (from annotation) to compute removals safely
	prevApplied := readAppliedAnnotation(ns)

	// Get protection configuration from the current CR
	allProtectionPatterns := cr.Spec.ProtectedLabelPatterns
	protectionMode := cr.Spec.ProtectionMode

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
		updateStatus(cr, false, "ProtectedLabelConflict", message, protectionResult.ProtectedSkipped, nil)
		if err := r.Status().Update(ctx, cr); err != nil {
			l.Error(err, "failed to update status for protection conflict")
		}
		return &protectionResult, desired, fmt.Errorf("protected label conflict: %s", strings.Join(protectionResult.Warnings, "; "))
	}

	// Apply labels to namespace
	changed := r.applyLabelsToNamespace(ns, protectionResult.AllowedLabels, prevApplied)

	if changed {
		if err := r.Update(ctx, ns); err != nil {
			return nil, nil, err
		}
	}

	// Update tracking annotation
	if err := writeAppliedAnnotation(ctx, r.Client, ns, protectionResult.AllowedLabels); err != nil {
		// Log error but don't fail reconciliation since labels were applied successfully
		l.Error(err, "failed to write applied annotation")
	}

	return &protectionResult, desired, nil
}

// handleDeletion handles the deletion of a NamespaceLabel CR
func (r *NamespaceLabelReconciler) handleDeletion(ctx context.Context, cr *labelsv1alpha1.NamespaceLabel) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	// Target namespace is always the same as the CR's namespace
	targetNS := cr.Namespace

	// Get the target namespace
	var ns corev1.Namespace
	if err := r.Get(ctx, types.NamespacedName{Name: targetNS}, &ns); err != nil {
		if apierrors.IsNotFound(err) {
			// Namespace is gone, nothing to clean up
			controllerutil.RemoveFinalizer(cr, FinalizerName)
			return ctrl.Result{}, r.Update(ctx, cr)
		}
		return ctrl.Result{}, err
	}

	// Remove all labels that were applied by this operator
	prevApplied := readAppliedAnnotation(&ns)

	if ns.Labels == nil {
		ns.Labels = map[string]string{}
	}

	changed := false
	for k := range cr.Spec.Labels {
		if prevVal, wasApplied := prevApplied[k]; wasApplied {
			if cur, exists := ns.Labels[k]; exists && cur == prevVal {
				delete(ns.Labels, k)
				changed = true
			}
		}
	}

	if changed {
		if err := r.Update(ctx, &ns); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Clear applied annotation since no labels remain
	if err := writeAppliedAnnotation(ctx, r.Client, &ns, map[string]string{}); err != nil {
		l.Error(err, "failed to update applied annotation during deletion")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(cr, FinalizerName)
	return ctrl.Result{}, r.Update(ctx, cr)
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
