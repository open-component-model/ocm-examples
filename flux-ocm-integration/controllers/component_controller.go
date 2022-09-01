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
	"errors"
	"fmt"

	"github.com/fluxcd/pkg/apis/meta"
	sourcev1beta2 "github.com/fluxcd/source-controller/api/v1beta2"
	"github.com/minio/minio-go/v7"
	miniocredentials "github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/open-component-model/ocm/pkg/common"
	"github.com/open-component-model/ocm/pkg/contexts/credentials"
	"github.com/open-component-model/ocm/pkg/contexts/oci/repositories/ocireg"
	"github.com/open-component-model/ocm/pkg/contexts/ocm"
	"github.com/open-component-model/ocm/pkg/contexts/ocm/attrs/signingattr"
	"github.com/open-component-model/ocm/pkg/contexts/ocm/repositories/genericocireg"
	"github.com/open-component-model/ocm/pkg/contexts/ocm/signing"
	transferv1alpha1 "github.com/phoban01/ocm-flux/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
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

	if obj.Status.Bucket == "" {
		mc, err := minio.New(r.MinioURL, &minio.Options{
			Creds:  miniocredentials.NewStaticV4("minioadmin", "minioadmin", ""),
			Secure: false,
		})
		if err != nil {
			return obj, err
		}

		if err := r.reconcileBucket(ctx, mc, obj, bucketName); err != nil {
			return obj, err
		}

		obj.Status.Bucket = bucketName

		log.Info("storage reconciled successfully")
	}

	obj.Status.IsVerified = false
	obj.Status.FailedVerificationReason = ""

	if !obj.ShouldVerify() {
		return obj, nil
	}

	res, err := r.verifyComponent(ctx, obj)
	if err != nil {
		obj.Status.FailedVerificationReason = fmt.Sprintf("%s", err)
		return obj, nil
	}

	obj.Status.IsVerified = res

	obj.Status.ObservedGeneration = obj.ObjectMeta.Generation

	return obj, nil
}

func (r *ComponentReconciler) verifyComponent(ctx context.Context, obj transferv1alpha1.Component) (bool, error) {
	session := ocm.NewSession(nil)
	defer session.Close()

	ocmCtx := ocm.ForContext(ctx)

	if err := r.configureCredentials(ctx, ocmCtx, obj); err != nil {
		return false, err
	}

	repoSpec := genericocireg.NewRepositorySpec(ocireg.NewRepositorySpec(obj.Spec.Repository.URL), nil)
	repo, err := session.LookupRepository(ocmCtx, repoSpec)
	if err != nil {
		return false, fmt.Errorf("repo error: %w", err)
	}

	resolver := ocm.NewCompoundResolver(repo)

	cv, err := session.LookupComponentVersion(repo, obj.Spec.Name, obj.Spec.Version)
	if err != nil {
		return false, fmt.Errorf("component error: %w", err)
	}

	cert, err := r.getPublicKey(ctx, obj)
	if err != nil {
		return false, fmt.Errorf("verify error: %w", err)
	}

	opts := signing.NewOptions(
		signing.VerifySignature(obj.Spec.Verify.Signature),
		signing.Resolver(resolver),
		signing.VerifyDigests(),
		signing.PublicKey(obj.Spec.Verify.Signature, cert),
	)

	if err := opts.Complete(signingattr.Get(ocmCtx)); err != nil {
		return false, fmt.Errorf("verify error: %w", err)
	}

	dig, err := signing.Apply(nil, nil, cv, opts)
	if err != nil {
		return false, err
	}

	return dig.Value == cv.GetDescriptor().Signatures[0].Digest.Value, nil
}

func (r *ComponentReconciler) configureCredentials(ctx context.Context, ocmCtx ocm.Context, component transferv1alpha1.Component) error {
	// create the consumer id for credentials
	consumerID, err := getConsumerIdentityForRepository(component.Spec.Repository)
	if err != nil {
		return err
	}

	// fetch the credentials for the component storage
	creds, err := r.getCredentialsForRepository(ctx, component.GetNamespace(), component.Spec.Repository)
	if err != nil {
		return err
	}

	// TODO: set credentials should return an error
	ocmCtx.CredentialsContext().SetCredentialsForConsumer(consumerID, creds)

	return nil
}

func (r *ComponentReconciler) getCredentialsForRepository(ctx context.Context, namespace string, repo transferv1alpha1.Repository) (credentials.Credentials, error) {
	var secret corev1.Secret
	secretKey := client.ObjectKey{
		Namespace: namespace,
		Name:      repo.SecretRef.Name,
	}
	if err := r.Get(ctx, secretKey, &secret); err != nil {
		return nil, err
	}

	props := make(common.Properties)
	for key, value := range secret.Data {
		props.SetNonEmptyValue(key, string(value))
	}

	return credentials.NewCredentials(props), nil
}

func (r *ComponentReconciler) getPublicKey(ctx context.Context, obj transferv1alpha1.Component) ([]byte, error) {
	var secret corev1.Secret
	secretKey := client.ObjectKey{
		Namespace: obj.GetNamespace(),
		Name:      obj.Spec.Verify.PublicKey.Name,
	}
	if err := r.Get(ctx, secretKey, &secret); err != nil {
		return nil, err
	}

	for key, value := range secret.Data {
		if key == obj.Spec.Verify.Signature {
			return value, nil
		}
	}

	return nil, errors.New("public key not found")
}

func (r *ComponentReconciler) reconcileBucket(ctx context.Context, mc *minio.Client, obj transferv1alpha1.Component, bucketName string) error {
	// create the bucket
	opts := minio.MakeBucketOptions{}
	if err := mc.MakeBucket(ctx, bucketName, opts); err != nil {
		exists, errBucketExists := mc.BucketExists(ctx, bucketName)
		if errBucketExists != nil && !exists {
			return fmt.Errorf("error creating bucket: %w", err)
		}
	}

	// create a source pointing at the bucket
	bucket := &sourcev1beta2.Bucket{
		ObjectMeta: v1.ObjectMeta{
			Name:      obj.GetName(),
			Namespace: obj.GetNamespace(),
		},
		Spec: sourcev1beta2.BucketSpec{
			Provider:   "generic",
			BucketName: bucketName,
			Endpoint:   r.MinioURL,
			Interval:   obj.Spec.Interval,
			Insecure:   true,
			SecretRef: &meta.LocalObjectReference{
				Name: "minio-default-credentials",
			},
		},
	}

	if err := ctrlutil.SetOwnerReference(&obj, bucket, r.Scheme); err != nil {
		return err
	}

	if err := r.Create(ctx, bucket); err != nil && !apierrs.IsAlreadyExists(err) {
		return err
	}

	return nil
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
