package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	labelsv1alpha1 "github.com/sbahar619/namespace-label-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// isLabelProtected checks if a label key matches any of the protection patterns
func isLabelProtected(labelKey string, protectionPatterns []string) bool {
	for _, pattern := range protectionPatterns {
		// Skip empty patterns
		if pattern == "" {
			continue
		}

		// Use filepath.Match for glob pattern matching
		if matched, err := filepath.Match(pattern, labelKey); err == nil && matched {
			return true
		}
		// If there's an error in pattern matching, log it but continue
		// This prevents malformed patterns from breaking protection
	}
	return false
}

// applyProtectionLogic processes desired labels against protection rules
func applyProtectionLogic(
	desired map[string]string,
	existing map[string]string,
	prevApplied map[string]string,
	protectionPatterns []string,
	protectionMode labelsv1alpha1.ProtectionMode,
	ignoreExisting bool,
) ProtectionResult {
	result := ProtectionResult{
		AllowedLabels:    make(map[string]string),
		ProtectedSkipped: []string{},
		Warnings:         []string{},
		ShouldFail:       false,
	}

	for key, value := range desired {
		// Check if this label is protected
		if isLabelProtected(key, protectionPatterns) {
			existingValue, hasExisting := existing[key]
			_, wasPrevApplied := prevApplied[key]

			// If ignoreExisting is true and we previously applied this label, allow it
			if ignoreExisting && wasPrevApplied {
				result.AllowedLabels[key] = value
				continue
			}

			// If the label exists with a different value, apply protection
			if hasExisting && existingValue != value {
				msg := fmt.Sprintf("Label '%s' is protected by pattern and has existing value '%s' (attempting to set '%s')",
					key, existingValue, value)

				switch protectionMode {
				case labelsv1alpha1.ProtectionModeFail:
					result.ShouldFail = true
					result.Warnings = append(result.Warnings, msg)
					return result
				case labelsv1alpha1.ProtectionModeWarn:
					result.Warnings = append(result.Warnings, msg)
					result.ProtectedSkipped = append(result.ProtectedSkipped, key)
					continue
				default: // ProtectionModeSkip
					result.ProtectedSkipped = append(result.ProtectedSkipped, key)
					continue
				}
			}

			// Protected label with no conflict - log for debugging
			if !hasExisting {
				// This is fine - setting a new protected label is allowed
			} else if existingValue == value {
				// This is fine - no change needed
			}
		}

		// Label is either not protected or safe to apply
		result.AllowedLabels[key] = value
	}

	return result
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

	// Enforce singleton pattern: only allow CR named "labels"
	if exists && current.Name != StandardCRName {
		// Check if a valid "labels" CR already exists to provide context-aware message
		var labelsInstance labelsv1alpha1.NamespaceLabel
		labelsExists := r.Get(ctx, types.NamespacedName{Name: StandardCRName, Namespace: current.Namespace}, &labelsInstance) == nil

		var message string
		if labelsExists {
			message = fmt.Sprintf("NamespaceLabel CR must be named '%s'. A valid '%s' CR already exists in this namespace. Please delete this CR and update the existing '%s' CR instead.",
				StandardCRName, StandardCRName, StandardCRName)
		} else {
			message = fmt.Sprintf("NamespaceLabel CR must be named '%s' for platform consistency. Please delete this CR and create a new one named '%s'.",
				StandardCRName, StandardCRName)
		}

		l.Error(nil, "NamespaceLabel CR has invalid name",
			"namespace", current.Namespace, "actualName", current.Name, "requiredName", StandardCRName, "labelsInstanceExists", labelsExists)
		updateStatus(&current, false, "InvalidName", message)
		if err := r.Status().Update(ctx, &current); err != nil {
			l.Error(err, "failed to update status for invalid name")
		}
		return ctrl.Result{}, nil // Don't requeue - user needs to fix the name
	}

	// Enforce singleton pattern: ensure only one NamespaceLabel per namespace
	if exists {
		var allCRs labelsv1alpha1.NamespaceLabelList
		if err := r.List(ctx, &allCRs, client.InNamespace(current.Namespace)); err != nil {
			return ctrl.Result{}, err
		}

		// Check for multiple CRs (excluding the current one if it's being deleted)
		activeCRs := 0
		for _, cr := range allCRs.Items {
			if cr.DeletionTimestamp == nil {
				activeCRs++
			}
		}

		if activeCRs > 1 {
			// Build list of CR names for helpful error message
			var crNames []string
			for _, cr := range allCRs.Items {
				if cr.DeletionTimestamp == nil {
					crNames = append(crNames, cr.Name)
				}
			}

			// Create context-aware message with specific guidance
			var message string
			if current.Name == StandardCRName {
				// This is the valid "labels" CR, but others exist
				message = fmt.Sprintf("Multiple NamespaceLabel CRs detected (%d active: %v). Only one CR named '%s' is allowed per namespace. Please delete the other CRs: %v",
					activeCRs, crNames, StandardCRName, removeFromSlice(crNames, StandardCRName))
			} else {
				// This is an invalid CR name and others exist
				message = fmt.Sprintf("Multiple NamespaceLabel CRs detected (%d active: %v). Only one CR named '%s' is allowed per namespace. Please delete this CR and keep only the '%s' CR.",
					activeCRs, crNames, StandardCRName, StandardCRName)
			}

			l.Error(nil, "Multiple NamespaceLabel CRs found in namespace",
				"namespace", current.Namespace, "count", activeCRs, "crNames", crNames)
			updateStatus(&current, false, "MultipleInstances", message)
			if err := r.Status().Update(ctx, &current); err != nil {
				l.Error(err, "failed to update status for multiple instances")
			}
			return ctrl.Result{}, nil // Don't requeue - user needs to fix by deleting others
		}
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

	// Target namespace is always the same as the CR's namespace for multi-tenant security
	targetNS := req.Namespace
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
				message := fmt.Sprintf("Target namespace '%s' does not exist. The NamespaceLabel CR will remain inactive until the namespace is created. Namespace creation is typically handled by platform administrators.",
					targetNS)
				updateStatus(&current, false, "NamespaceNotFound", message)
				if err := r.Status().Update(ctx, &current); err != nil {
					l.Error(err, "failed to update status for missing namespace")
				}
			}
			// Requeue to check again later
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
		return ctrl.Result{}, err
	}

	// List ALL NamespaceLabel CRs in this namespace and merge labels.
	var list labelsv1alpha1.NamespaceLabelList
	if err := r.List(ctx, &list, client.InNamespace(targetNS)); err != nil {
		return ctrl.Result{}, err
	}

	desired, _ := mergeDesiredLabels(list.Items)

	// Load what we previously applied (from annotation) to compute removals safely.
	prevApplied := readAppliedAnnotation(&ns)

	// Gather protection configuration from all CRs
	var allProtectionPatterns []string
	var protectionMode labelsv1alpha1.ProtectionMode = labelsv1alpha1.ProtectionModeSkip
	var ignoreExisting bool = false

	for _, cr := range list.Items {
		allProtectionPatterns = append(allProtectionPatterns, cr.Spec.ProtectedLabelPatterns...)
		// Use the most restrictive protection mode from all CRs
		if cr.Spec.ProtectionMode == labelsv1alpha1.ProtectionModeFail {
			protectionMode = labelsv1alpha1.ProtectionModeFail
		} else if cr.Spec.ProtectionMode == labelsv1alpha1.ProtectionModeWarn && protectionMode != labelsv1alpha1.ProtectionModeFail {
			protectionMode = labelsv1alpha1.ProtectionModeWarn
		}
		// If any CR allows ignoring existing, apply that policy
		if cr.Spec.IgnoreExistingProtectedLabels {
			ignoreExisting = true
		}
	}

	// Apply protection logic
	if ns.Labels == nil {
		ns.Labels = map[string]string{}
	}

	protectionResult := applyProtectionLogic(
		desired,
		ns.Labels,
		prevApplied,
		allProtectionPatterns,
		protectionMode,
		ignoreExisting,
	)

	// If protection mode is "fail" and we hit protected labels, fail the reconciliation
	if protectionResult.ShouldFail {
		if exists {
			message := fmt.Sprintf("Reconciliation failed due to protected label conflicts: %s",
				strings.Join(protectionResult.Warnings, "; "))
			updateStatus(&current, false, "ProtectedLabelConflict", message)
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
			message := fmt.Sprintf("Labels were applied to namespace '%s' but failed to update tracking annotation. This may cause issues during cleanup. The controller will retry automatically. Error: %v",
				targetNS, err)
			updateStatus(&current, false, "AnnotationError", message)
			if err := r.Status().Update(ctx, &current); err != nil {
				l.Error(err, "failed to update status after annotation error")
			}
		}
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// If the CR from this request still exists, set its status (best-effort).
	if exists {
		// Log protection warnings if any
		for _, warning := range protectionResult.Warnings {
			l.Info("Label protection warning", "warning", warning)
		}

		labelCount := len(current.Spec.Labels)
		appliedCount := len(actuallyDesired)
		skippedCount := len(protectionResult.ProtectedSkipped)

		var msg string
		if skippedCount > 0 {
			msg = fmt.Sprintf("Applied %d labels to namespace '%s', skipped %d protected labels (%v). This is the only NamespaceLabel CR in this namespace (singleton pattern enforced).",
				appliedCount, current.Namespace, skippedCount, protectionResult.ProtectedSkipped)
		} else {
			msg = fmt.Sprintf("Successfully applied %d labels to namespace '%s'. This is the only NamespaceLabel CR in this namespace (singleton pattern enforced).",
				appliedCount, current.Namespace)
		}

		// Additional context if there are no labels defined
		if labelCount == 0 {
			msg = fmt.Sprintf("NamespaceLabel CR is active but no labels are defined. Add labels to the spec to apply them to namespace '%s'.",
				current.Namespace)
		}

		// Create list of applied label keys
		appliedKeys := make([]string, 0, len(actuallyDesired))
		for key := range actuallyDesired {
			appliedKeys = append(appliedKeys, key)
		}
		sort.Strings(appliedKeys)

		l.Info("NamespaceLabel successfully processed",
			"namespace", current.Namespace, "labelsApplied", appliedCount, "labelsRequested", labelCount, "protectedSkipped", skippedCount)
		updateStatusWithProtection(&current, true, "Synced", msg, protectionResult.ProtectedSkipped, appliedKeys)
		if err := r.Status().Update(ctx, &current); err != nil {
			l.Error(err, "failed to update CR status")
			// Don't return error; namespace labels were applied successfully
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
	var list labelsv1alpha1.NamespaceLabelList
	if err := r.List(ctx, &list, client.InNamespace(targetNS)); err != nil {
		return ctrl.Result{}, err
	}

	var remainingCRs []labelsv1alpha1.NamespaceLabel
	for _, otherCR := range list.Items {
		if otherCR.Name == cr.Name {
			continue // Skip the one being deleted
		}
		remainingCRs = append(remainingCRs, otherCR)
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
	// Fetch a fresh copy of the namespace to avoid conflicts with the previously updated object
	var freshNS corev1.Namespace
	if err := c.Get(ctx, types.NamespacedName{Name: ns.Name}, &freshNS); err != nil {
		return fmt.Errorf("failed to fetch namespace for annotation update: %w", err)
	}

	if freshNS.Annotations == nil {
		freshNS.Annotations = map[string]string{}
	}

	b, err := json.Marshal(applied)
	if err != nil {
		return fmt.Errorf("marshal applied: %w", err)
	}

	// Check if annotation already has the correct value
	if cur, ok := freshNS.Annotations[appliedAnnoKey]; ok && cur == string(b) {
		return nil // no change needed
	}

	freshNS.Annotations[appliedAnnoKey] = string(b)
	return c.Update(ctx, &freshNS)
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

func updateStatusWithProtection(cr *labelsv1alpha1.NamespaceLabel, ok bool, reason, msg string, protectedSkipped, labelsApplied []string) {
	cr.Status.Applied = ok
	cr.Status.Message = msg
	cr.Status.ProtectedLabelsSkipped = protectedSkipped
	cr.Status.LabelsApplied = labelsApplied

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

// removeFromSlice removes a specific value from a slice and returns a new slice
func removeFromSlice(slice []string, item string) []string {
	var result []string
	for _, s := range slice {
		if s != item {
			result = append(result, s)
		}
	}
	return result
}

func (r *NamespaceLabelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Create the controller without unnecessary namespace watch
	return ctrl.NewControllerManagedBy(mgr).
		For(&labelsv1alpha1.NamespaceLabel{}).
		Complete(r)
}
