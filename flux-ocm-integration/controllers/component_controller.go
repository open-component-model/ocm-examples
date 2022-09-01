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
	"time"

	"github.com/fluxcd/pkg/apis/meta"
	sourcev1beta2 "github.com/fluxcd/source-controller/api/v1beta2"
	"github.com/minio/minio-go/v7"
	miniocredentials "github.com/minio/minio-go/v7/pkg/credentials"
	transferv1alpha1 "github.com/phoban01/ocm-flux/api/v1alpha1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// ComponentReconciler reconciles a Component object
type ComponentReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	MinioURL string
}

//+kubebuilder:rbac:groups=transfer.phoban.io,resources=components,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=transfer.phoban.io,resources=components/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=transfer.phoban.io,resources=components/finalizers,verbs=update

//+kubebuilder:rbac:groups=source.toolkit.fluxcd.io,resources=buckets,verbs=get;list;watch;create;update;patch;delete

func (r *ComponentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	var obj transferv1alpha1.Component
	if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Info("reconciling component", "name", obj.GetName())

	res, reconcileErr := r.reconcile(ctx, obj)
	if err := r.patchStatus(ctx, req, res.Status); err != nil {
		return ctrl.Result{Requeue: true}, err
	}

	if reconcileErr != nil {
		log.Error(reconcileErr, fmt.Sprintf("reconciliation failed"))
	}

	return ctrl.Result{}, nil
}

// TODO: needs finalizer to delete bucket
func (r *ComponentReconciler) reconcile(ctx context.Context, obj transferv1alpha1.Component) (transferv1alpha1.Component, error) {
	log := ctrl.LoggerFrom(ctx)
	name := obj.GetName()
	namespace := obj.GetNamespace()

	bucketName := fmt.Sprintf("%s.%s.fluxcd.io", name, namespace)
	// TODO: endpoint should be take from r
	endpoint := "localhost:9000"

	// Initialize minio client object.
	mc, err := minio.New(endpoint, &minio.Options{
		Creds:  miniocredentials.NewStaticV4("minioadmin", "minioadmin", ""),
		Secure: false,
	})
	if err != nil {
		return obj, err
	}

	obj.Status.Bucket = bucketName

	opts := minio.MakeBucketOptions{}

	if err := mc.MakeBucket(ctx, bucketName, opts); err != nil {
		exists, errBucketExists := mc.BucketExists(ctx, bucketName)
		if errBucketExists != nil && !exists {
			return obj, fmt.Errorf("error creating bucket: %w", err)
		}
	}

	log.Info("minio bucket created")

	// create a source pointing at the bucket
	bucket := &sourcev1beta2.Bucket{
		ObjectMeta: v1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: sourcev1beta2.BucketSpec{
			Provider:   "generic",
			BucketName: bucketName,
			Endpoint:   r.MinioURL,
			Interval:   metav1.Duration{Duration: time.Minute},
			Insecure:   true,
			SecretRef: &meta.LocalObjectReference{
				Name: "minio-default-credentials",
			},
		},
	}

	if err := ctrlutil.SetOwnerReference(&obj, bucket, r.Scheme); err != nil {
		return obj, err
	}

	if err := r.Create(ctx, bucket); err != nil && !apierrs.IsAlreadyExists(err) {
		return obj, err
	}

	log.Info("bucket source created")

	return obj, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ComponentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&transferv1alpha1.Component{}).
		Complete(r)
}

func (r *ComponentReconciler) patchStatus(ctx context.Context, req ctrl.Request, newStatus transferv1alpha1.ComponentStatus) error {
	var obj transferv1alpha1.Component

	if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
		return err
	}

	patch := client.MergeFrom(obj.DeepCopy())

	obj.Status = newStatus

	return r.Status().Patch(ctx, &obj, patch)
}
