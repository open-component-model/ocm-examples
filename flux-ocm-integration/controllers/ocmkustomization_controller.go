/*
Copyright 2022.

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

package controllers

import (
	"context"
	"fmt"
	"path/filepath"

	apierrs "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	kustomizev1beta2 "github.com/fluxcd/kustomize-controller/api/v1beta2"
	transferv1alpha1 "github.com/phoban01/ocm-flux/api/v1alpha1"
)

// OCMKustomizationReconciler reconciles a OCMKustomization object
type OCMKustomizationReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=transfer.phoban.io,resources=ocmkustomizations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=transfer.phoban.io,resources=ocmkustomizations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=transfer.phoban.io,resources=ocmkustomizations/finalizers,verbs=update

//+kubebuilder:rbac:groups=kustomize.toolkit.fluxcd.io,resources=kustomizations,verbs=get;list;watch;create;update;patch;delete

func (r *OCMKustomizationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	var obj transferv1alpha1.OCMKustomization
	if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Info("reconciling OCM kustomization", "name", obj.GetName())

	res, reconcileErr := r.reconcile(ctx, obj)
	if err := r.patchStatus(ctx, req, res.Status); err != nil {
		return ctrl.Result{Requeue: true}, err
	}

	if reconcileErr != nil {
		log.Error(reconcileErr, fmt.Sprintf("reconciliation failed"))
	}

	return ctrl.Result{}, nil
}

func (r *OCMKustomizationReconciler) reconcile(ctx context.Context, obj transferv1alpha1.OCMKustomization) (transferv1alpha1.OCMKustomization, error) {
	// create a  kustomization
	name := obj.GetName()
	namespace := obj.GetNamespace()

	var sourceKind string
	if obj.Spec.SourceRef.Kind == "Resource" {
		sourceKind = "OCIRepository"
	} else {
		sourceKind = "Bucket"
	}

	kustomization := &kustomizev1beta2.Kustomization{
		ObjectMeta: v1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: kustomizev1beta2.KustomizationSpec{
			SourceRef: kustomizev1beta2.CrossNamespaceSourceReference{
				Kind: sourceKind,
				Name: obj.Spec.SourceRef.Name,
			},
			Interval:        obj.Spec.KustomizeTemplate.Interval,
			Path:            filepath.Join("", obj.Spec.KustomizeTemplate.Path),
			Prune:           obj.Spec.KustomizeTemplate.Prune,
			TargetNamespace: namespace,
		},
	}

	if err := ctrlutil.SetOwnerReference(&obj, kustomization, r.Scheme); err != nil {
		return obj, err
	}

	// TODO: should be create or update
	if err := r.Create(ctx, kustomization); err != nil && !apierrs.IsAlreadyExists(err) {
		return obj, err
	}

	return obj, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *OCMKustomizationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&transferv1alpha1.OCMKustomization{}).
		Complete(r)
}

func (r *OCMKustomizationReconciler) patchStatus(ctx context.Context, req ctrl.Request, newStatus transferv1alpha1.OCMKustomizationStatus) error {
	var obj transferv1alpha1.OCMKustomization

	if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
		return err
	}

	patch := client.MergeFrom(obj.DeepCopy())
	obj.Status = newStatus

	return r.Status().Patch(ctx, &obj, patch)
}
