package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	labelsv1alpha1 "github.com/sbahar619/namespace-label-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// RBAC: access our CRD + update Namespaces.
// +kubebuilder:rbac:groups=labels.shahaf.com,resources=namespacelabels,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=labels.shahaf.com,resources=namespacelabels/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=labels.shahaf.com,resources=namespacelabels/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;update;patch

// NamespaceLabelReconciler reconciles a NamespaceLabel object
type NamespaceLabelReconciler struct {
	client.Client
	Scheme *runtime.Scheme
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
	if exists && !controllerutil.ContainsFinalizer(&current, FinalizerName) {
		controllerutil.AddFinalizer(&current, FinalizerName)
		if err := r.Update(ctx, &current); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Target namespace is always the same as the CR's namespace for multi-tenant security
	targetNS := req.Namespace
	if targetNS == "" {
		// Should never happen for namespaced resources, but be defensive
		return ctrl.Result{}, nil
	}

	// Get the target Namespace object to modify its labels
	var ns corev1.Namespace
	if err := r.Get(ctx, types.NamespacedName{Name: targetNS}, &ns); err != nil {
		return ctrl.Result{}, err
	}

	// Since we enforce singleton pattern, use the current CR directly
	desired := current.Spec.Labels

	// Load what we previously applied (from annotation) to compute removals safely.
	prevApplied := readAppliedAnnotation(&ns)

	// Get protection configuration from the current CR
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
		if exists {
			message := fmt.Sprintf("Protected label conflicts: %s", strings.Join(protectionResult.Warnings, "; "))
			updateStatus(&current, false, "ProtectedLabelConflict", message, protectionResult.ProtectedSkipped, nil)
			if err := r.Status().Update(ctx, &current); err != nil {
				l.Error(err, "failed to update status for protection conflict")
			}
		}
		return ctrl.Result{RequeueAfter: time.Minute * 5}, fmt.Errorf("protected label conflict: %s", strings.Join(protectionResult.Warnings, "; "))
	}

	// Use filtered labels from protection logic
	actuallyDesired := protectionResult.AllowedLabels
	changed := false

	// Remove stale operator-managed keys
	for k, prevVal := range prevApplied {
		if _, stillWanted := actuallyDesired[k]; !stillWanted {
			if cur, ok := ns.Labels[k]; ok && cur == prevVal {
				delete(ns.Labels, k)
				changed = true
			}
		}
	}

	// Apply/overwrite allowed keys only
	for k, v := range actuallyDesired {
		if cur, ok := ns.Labels[k]; !ok || cur != v {
			ns.Labels[k] = v
			changed = true
		}
	}

	if changed {
		if err := r.Update(ctx, &ns); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Persist new applied set to the annotation (truth of what we own)
	if err := writeAppliedAnnotation(ctx, r.Client, &ns, actuallyDesired); err != nil {
		l.Error(err, "failed to write applied annotation")
		if exists {
			message := fmt.Sprintf("Failed to update tracking annotation: %v", err)
			updateStatus(&current, false, "AnnotationError", message, nil, nil)
			if err := r.Status().Update(ctx, &current); err != nil {
				l.Error(err, "failed to update status after annotation error")
			}
		}
		// Continue despite annotation failure - labels were applied successfully
	}

	// Update status if the CR still exists
	if exists {
		labelCount := len(desired)
		appliedCount := len(actuallyDesired)
		skippedCount := len(protectionResult.ProtectedSkipped)

		var msg string
		if skippedCount > 0 {
			msg = fmt.Sprintf("Applied %d labels to namespace '%s', skipped %d protected labels (%v). This is the only NamespaceLabel CR in this namespace (singleton pattern enforced).",
				appliedCount, targetNS, skippedCount, protectionResult.ProtectedSkipped)
		} else {
			msg = fmt.Sprintf("Applied %d labels to namespace '%s'. This is the only NamespaceLabel CR in this namespace (singleton pattern enforced).",
				appliedCount, targetNS)
		}

		appliedKeys := make([]string, 0, len(actuallyDesired))
		for k := range actuallyDesired {
			appliedKeys = append(appliedKeys, k)
		}

		l.Info("NamespaceLabel successfully processed",
			"namespace", current.Namespace, "labelsApplied", appliedCount, "labelsRequested", labelCount, "protectedSkipped", skippedCount)
		updateStatus(&current, true, "Synced", msg, protectionResult.ProtectedSkipped, appliedKeys)
		if err := r.Status().Update(ctx, &current); err != nil {
			l.Error(err, "failed to update CR status")
		}
	}

	return ctrl.Result{}, nil
}

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

	// Get all remaining CRs in this namespace (excluding the one being deleted)
	// Since we enforce singleton pattern, there will be NO remaining CRs
	// So we should remove ALL labels that were applied by this operator
	desiredAfterDeletion := map[string]string{} // Empty - no remaining CRs

	// Get currently applied labels
	prevApplied := readAppliedAnnotation(&ns)

	// Remove labels that were set by this CR and won't be replaced by others
	if ns.Labels == nil {
		ns.Labels = map[string]string{}
	}

	changed := false
	for k := range cr.Spec.Labels {
		if _, shouldRemain := desiredAfterDeletion[k]; !shouldRemain {
			if prevVal, wasPrevApplied := prevApplied[k]; wasPrevApplied {
				if cur, exists := ns.Labels[k]; exists && cur == prevVal {
					delete(ns.Labels, k)
					changed = true
				}
			}
		}
	}

	if changed {
		if err := r.Update(ctx, &ns); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Update applied annotation to reflect the new state
	if err := writeAppliedAnnotation(ctx, r.Client, &ns, desiredAfterDeletion); err != nil {
		l.Error(err, "failed to update applied annotation during deletion")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(cr, FinalizerName)
	return ctrl.Result{}, r.Update(ctx, cr)
}

func (r *NamespaceLabelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Create the controller without unnecessary namespace watch
	return ctrl.NewControllerManagedBy(mgr).
		For(&labelsv1alpha1.NamespaceLabel{}).
		Complete(r)
}
