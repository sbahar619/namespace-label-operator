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

package v1alpha1

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	labelsv1alpha1 "github.com/sbahar619/namespace-label-operator/api/v1alpha1"
)

// nolint:unused
var namespacelabellog = logf.Log.WithName("namespacelabel-resource")

const (
	// StandardCRName is the required name for NamespaceLabel CRs (singleton pattern)
	StandardCRName = "labels"
)

func SetupNamespaceLabelWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&labelsv1alpha1.NamespaceLabel{}).
		WithValidator(&NamespaceLabelCustomValidator{
			Client: mgr.GetClient(),
		}).
		Complete()
}

// NOTE: Webhook validates create and update operations only. Deletion cleanup is handled by the controller's finalizer.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-labels-shahaf-com-v1alpha1-namespacelabel,mutating=false,failurePolicy=fail,sideEffects=None,groups=labels.shahaf.com,resources=namespacelabels,verbs=create;update,versions=v1alpha1,name=vnamespacelabel-v1alpha1.kb.io,admissionReviewVersions=v1

// NamespaceLabelCustomValidator struct is responsible for validating the NamespaceLabel resource
// when it is created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type NamespaceLabelCustomValidator struct {
	Client client.Client
}

var _ webhook.CustomValidator = &NamespaceLabelCustomValidator{}

func (v *NamespaceLabelCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	namespacelabel, ok := obj.(*labelsv1alpha1.NamespaceLabel)
	if !ok {
		return nil, fmt.Errorf("expected a NamespaceLabel object but got %T", obj)
	}
	namespacelabellog.Info("Validation for NamespaceLabel upon creation", "name", namespacelabel.GetName(), "namespace", namespacelabel.GetNamespace())

	// Validate name (singleton pattern)
	if err := v.validateName(namespacelabel); err != nil {
		return nil, err
	}

	// Validate singleton (only one NamespaceLabel per namespace)
	if err := v.validateSingleton(ctx, namespacelabel, nil); err != nil {
		return nil, err
	}

	return nil, nil
}

func (v *NamespaceLabelCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	namespacelabel, ok := newObj.(*labelsv1alpha1.NamespaceLabel)
	if !ok {
		return nil, fmt.Errorf("expected a NamespaceLabel object for the newObj but got %T", newObj)
	}

	oldNamespacelabel, ok := oldObj.(*labelsv1alpha1.NamespaceLabel)
	if !ok {
		return nil, fmt.Errorf("expected a NamespaceLabel object for the oldObj but got %T", oldObj)
	}

	namespacelabellog.Info("Validation for NamespaceLabel upon update", "name", namespacelabel.GetName(), "namespace", namespacelabel.GetNamespace())

	// Validate name (singleton pattern)
	if err := v.validateName(namespacelabel); err != nil {
		return nil, err
	}

	// Validate singleton (only one NamespaceLabel per namespace)
	if err := v.validateSingleton(ctx, namespacelabel, oldNamespacelabel); err != nil {
		return nil, err
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator interface but performs no validation.
// Deletion cleanup is handled by the controller's finalizer logic.
func (v *NamespaceLabelCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	_, ok := obj.(*labelsv1alpha1.NamespaceLabel)
	if !ok {
		return nil, fmt.Errorf("expected a NamespaceLabel object but got %T", obj)
	}
	return nil, nil
}
