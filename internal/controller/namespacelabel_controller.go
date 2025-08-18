package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	labelsv1alpha1 "github.com/sbahar619/namespace-label-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

const (
	appliedAnnoKey = "labels.shahaf.com/applied" // JSON of map[string]string
	FinalizerName  = "labels.shahaf.com/finalizer"
)

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

	// Determine target namespace: use spec.Namespace if specified, otherwise use CR's namespace
	targetNS := req.Namespace
	if exists && current.Spec.Namespace != "" {
		targetNS = current.Spec.Namespace
	}
	if targetNS == "" {
		// Should never happen for namespaced resources, but be defensive
		return ctrl.Result{}, nil
	}

	// Get the target Namespace object (cluster-scoped by name)
	var ns corev1.Namespace
	if err := r.Get(ctx, types.NamespacedName{Name: targetNS}, &ns); err != nil {
		if apierrors.IsNotFound(err) {
			// Namespace missing; if CR exists, update its status
			if exists {
				updateStatus(&current, false, "NamespaceNotFound", "Target Namespace does not exist")
				if err := r.Status().Update(ctx, &current); err != nil {
					l.Error(err, "failed to update status for missing namespace")
				}
			}
			// Requeue to check again later
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
		return ctrl.Result{}, err
	}

	// List ALL NamespaceLabel CRs targeting this namespace and merge labels.
	var list labelsv1alpha1.NamespaceLabelList
	if err := r.List(ctx, &list); err != nil {
		return ctrl.Result{}, err
	}

	// Filter CRs that target this namespace
	var relevantCRs []labelsv1alpha1.NamespaceLabel
	for _, cr := range list.Items {
		crTargetNS := cr.Namespace
		if cr.Spec.Namespace != "" {
			crTargetNS = cr.Spec.Namespace
		}
		if crTargetNS == targetNS {
			relevantCRs = append(relevantCRs, cr)
		}
	}

	desired, perKeyWinners := mergeDesiredLabels(relevantCRs)

	// Load what we previously applied (from annotation) to compute removals safely.
	prevApplied := readAppliedAnnotation(&ns)

	// Compute patch to Namespace labels: remove keys we previously applied that are no longer desired
	// (only if the current value still equals what we set), and set/overwrite desired keys.
	if ns.Labels == nil {
		ns.Labels = map[string]string{}
	}

	changed := false

	// Remove stale operator-managed keys
	for k, prevVal := range prevApplied {
		if _, stillWanted := desired[k]; !stillWanted {
			if cur, ok := ns.Labels[k]; ok && cur == prevVal {
				delete(ns.Labels, k)
				changed = true
			}
		}
	}

	// Apply/overwrite desired keys
	for k, v := range desired {
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
	if err := writeAppliedAnnotation(ctx, r.Client, &ns, desired); err != nil {
		l.Error(err, "failed to write applied annotation")
		if exists {
			updateStatus(&current, false, "AnnotationError", "Failed to update applied annotation")
			if err := r.Status().Update(ctx, &current); err != nil {
				l.Error(err, "failed to update status after annotation error")
			}
		}
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// If the CR from this request still exists, set its status (best-effort).
	if exists {
		// If this CR "won" on all of its keys, mark Applied=true; else Applied=true but add message on conflicts.
		msg := ""
		conflicted := false
		for k := range current.Spec.Labels {
			if winner, ok := perKeyWinners[k]; ok && winner != current.Name {
				conflicted = true
			}
		}
		if conflicted {
			msg = "Some keys were overridden by another NamespaceLabel in this namespace (name tie-breaker applied)."
		} else {
			msg = "Labels merged and applied."
		}
		updateStatus(&current, true, "Synced", msg)
		if err := r.Status().Update(ctx, &current); err != nil {
			l.Error(err, "failed to update CR status")
			// Don't return error; namespace labels were applied successfully
		}
	}

	return ctrl.Result{}, nil
}

func (r *NamespaceLabelReconciler) handleDeletion(ctx context.Context, cr *labelsv1alpha1.NamespaceLabel) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	// Determine target namespace
	targetNS := cr.Namespace
	if cr.Spec.Namespace != "" {
		targetNS = cr.Spec.Namespace
	}

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

	// Get all remaining CRs targeting this namespace (excluding the one being deleted)
	var list labelsv1alpha1.NamespaceLabelList
	if err := r.List(ctx, &list); err != nil {
		return ctrl.Result{}, err
	}

	var remainingCRs []labelsv1alpha1.NamespaceLabel
	for _, otherCR := range list.Items {
		if otherCR.Name == cr.Name && otherCR.Namespace == cr.Namespace {
			continue // Skip the one being deleted
		}
		crTargetNS := otherCR.Namespace
		if otherCR.Spec.Namespace != "" {
			crTargetNS = otherCR.Spec.Namespace
		}
		if crTargetNS == targetNS {
			remainingCRs = append(remainingCRs, otherCR)
		}
	}

	// Calculate what labels should remain after this CR is deleted
	desiredAfterDeletion, _ := mergeDesiredLabels(remainingCRs)

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

// mergeDesiredLabels merges labels from many CRs in the same namespace.
// Conflict policy: if multiple CRs set the same key with different values,
// the CR with the lexicographically smallest name wins for that key.
func mergeDesiredLabels(items []labelsv1alpha1.NamespaceLabel) (map[string]string, map[string]string) {
	desired := make(map[string]string)
	perKeyWinner := make(map[string]string) // key -> CR name that won

	// Sort CRs by name ascending for deterministic results
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })

	for _, cr := range items {
		for k, v := range cr.Spec.Labels {
			// If key not yet set, or set by a CR with a "larger" name, this CR wins.
			if winner, exists := perKeyWinner[k]; !exists || cr.Name < winner {
				desired[k] = v
				perKeyWinner[k] = cr.Name
			}
		}
	}
	return desired, perKeyWinner
}

func readAppliedAnnotation(ns *corev1.Namespace) map[string]string {
	out := map[string]string{}
	if ns.Annotations == nil {
		return out
	}
	raw, ok := ns.Annotations[appliedAnnoKey]
	if !ok || raw == "" {
		return out
	}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

func writeAppliedAnnotation(ctx context.Context, c client.Client, ns *corev1.Namespace, applied map[string]string) error {
	if ns.Annotations == nil {
		ns.Annotations = map[string]string{}
	}
	b, err := json.Marshal(applied)
	if err != nil {
		return fmt.Errorf("marshal applied: %w", err)
	}
	if cur, ok := ns.Annotations[appliedAnnoKey]; ok && cur == string(b) {
		return nil // no change
	}
	ns.Annotations[appliedAnnoKey] = string(b)
	return c.Update(ctx, ns)
}

func updateStatus(cr *labelsv1alpha1.NamespaceLabel, ok bool, reason, msg string) {
	cr.Status.Applied = ok
	cr.Status.Message = msg

	now := metav1.Now()
	cond := metav1.Condition{
		Type:               "Ready",
		Status:             boolToCond(ok),
		Reason:             reason,
		Message:            msg,
		ObservedGeneration: cr.Generation,
		LastTransitionTime: now,
	}
	// Replace/ensure single Ready condition
	found := false
	for i := range cr.Status.Conditions {
		if cr.Status.Conditions[i].Type == "Ready" {
			cr.Status.Conditions[i] = cond
			found = true
			break
		}
	}
	if !found {
		cr.Status.Conditions = append(cr.Status.Conditions, cond)
	}
}

func boolToCond(b bool) metav1.ConditionStatus {
	if b {
		return metav1.ConditionTrue
	}
	return metav1.ConditionFalse
}

func (r *NamespaceLabelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Create the controller without unnecessary namespace watch
	return ctrl.NewControllerManagedBy(mgr).
		For(&labelsv1alpha1.NamespaceLabel{}).
		Complete(r)
}
