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
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	sourcev1beta2 "github.com/fluxcd/source-controller/api/v1beta2"
	"github.com/google/go-containerregistry/pkg/authn/k8schain"
	"github.com/google/go-containerregistry/pkg/crane"
	transferv1alpha1 "github.com/phoban01/ocm-flux/api/v1alpha1"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type ComponentDescriptor struct {
	Component Component `yaml:"component"`
}

type Component struct {
	Name      string     `yaml:"name"`
	Version   string     `yaml:"version"`
	Provider  string     `yaml:"provider"`
	Resources []Resource `yaml:"resources"`
}

type Resource struct {
	Access   ResourceAccess `yaml:"access"`
	Name     string         `yaml:"name"`
	Relation string         `yaml:"relation"`
	Type     string         `yaml:"type"`
	Version  string         `yaml:"version"`
}

type ResourceAccess struct {
	LocalReference string `yaml:"localReference"`
	MediaType      string `yaml:"mediaType"`
	Type           string `yaml:"type"`
}

// ResourceReconciler reconciles a Resource object
type ResourceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=transfer.phoban.io,resources=resources,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=transfer.phoban.io,resources=resources/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=transfer.phoban.io,resources=resources/finalizers,verbs=update

//+kubebuilder:rbac:groups=transfer.phoban.io,resources=components,verbs=get;list;watch

//+kubebuilder:rbac:groups=source.toolkit.fluxcd.io,resources=ocirepositories,verbs=get;list;watch;create;update;patch;delete

func (r *ResourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	var obj transferv1alpha1.Resource
	if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var component transferv1alpha1.Component
	cKey := types.NamespacedName{
		Name:      obj.Spec.ComponentRef.Name,
		Namespace: obj.Spec.ComponentRef.Namespace,
	}
	if err := r.Get(ctx, cKey, &component); err != nil {
		return ctrl.Result{}, fmt.Errorf("could not find specified component: %w", err)
	}

	log.Info("reconciling OCM resource", "name", obj.GetName())

	res, reconcileErr := r.reconcile(ctx, obj, component)
	if err := r.patchStatus(ctx, req, res.Status); err != nil {
		return ctrl.Result{Requeue: true}, err
	}

	if reconcileErr != nil {
		log.Error(reconcileErr, fmt.Sprintf("reconciliation failed"))
	}

	return ctrl.Result{}, nil
}

func (r *ResourceReconciler) reconcile(ctx context.Context, obj transferv1alpha1.Resource, component transferv1alpha1.Component) (transferv1alpha1.Resource, error) {
	// fetch the ocm and extract the resource media type
	mediaType, err := r.fetchResourceMediaType(ctx, obj, component)
	if err != nil {
		return obj, err
	}

	// create an oci source for the component resource
	name := obj.GetName()
	namespace := obj.GetNamespace()

	repo := &sourcev1beta2.OCIRepository{
		ObjectMeta: v1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: sourcev1beta2.OCIRepositorySpec{
			URL: fmt.Sprintf("oci://%s/%s", component.Spec.Repository.URL, component.Spec.Name),
			Reference: &sourcev1beta2.OCIRepositoryRef{
				Tag: component.Spec.Version,
			},
			LayerSelector: &sourcev1beta2.OCILayerSelector{
				MediaType: mediaType,
			},
			SecretRef: component.Spec.Repository.SecretRef,
			Interval:  component.Spec.Interval,
		},
	}

	if err := ctrlutil.SetOwnerReference(&obj, repo, r.Scheme); err != nil {
		return obj, err
	}

	if err := r.Create(ctx, repo); err != nil && !apierrs.IsAlreadyExists(err) {
		return obj, err
	}

	return obj, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ResourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&transferv1alpha1.Resource{}).
		Complete(r)
}

func (r *ResourceReconciler) patchStatus(ctx context.Context, req ctrl.Request, newStatus transferv1alpha1.ResourceStatus) error {
	var obj transferv1alpha1.Resource

	if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
		return err
	}

	patch := client.MergeFrom(obj.DeepCopy())
	obj.Status = newStatus

	return r.Status().Patch(ctx, &obj, patch)
}

// we should implement code similar to this in a controller for the installation resource
// but that uses the ocm libraries to fetch the deploy package resource
// can possibly use the flux libs to auth
// should then execute the transformations and output to storage
//
func (r *ResourceReconciler) fetchResourceMediaType(ctx context.Context, obj transferv1alpha1.Resource, component transferv1alpha1.Component) (string, error) {
	log := ctrl.LoggerFrom(ctx)

	var regSecret corev1.Secret
	regSecretKey := client.ObjectKey{
		Namespace: component.GetNamespace(),
		Name:      component.Spec.Repository.SecretRef.Name,
	}
	if err := r.Get(ctx, regSecretKey, &regSecret); err != nil {
		return "", client.IgnoreNotFound(err)
	}

	keychain, err := k8schain.NewFromPullSecrets(ctx, []corev1.Secret{regSecret})
	if err != nil {
		return "", err
	}

	src := fmt.Sprintf("%s/%s:%s", component.Spec.Repository.URL, component.Spec.Name, component.Spec.Version)

	log.Info("pulling oci image", "url", src)

	img, err := crane.Pull(src, crane.WithAuthFromKeychain(keychain))
	if err != nil {
		return "", err
	}

	digest, err := img.Digest()
	if err != nil {
		return "", err
	}

	log.Info("pulled image", "digest", digest)

	layers, err := img.Layers()
	if err != nil {
		return "", err
	}

	blob, err := layers[0].Uncompressed()
	if err != nil {
		return "", fmt.Errorf("error fetching blog: %w", err)
	}

	log.Info("extracting image layer")

	tmpDir, err := os.MkdirTemp("/tmp", "ocm"+obj.GetNamespace()+obj.GetName())
	if err != nil {
		return "", err
	}

	// download ocm manifest for latest image
	if err = myuntar(tmpDir, blob); err != nil {
		return "", fmt.Errorf("error untarring blob: %w", err)
	}

	componentDescriptorBytes, err := os.ReadFile(fmt.Sprintf("%s/component-descriptor.yaml", tmpDir))
	if err != nil {
		return "", err
	}

	componentDescriptor := &ComponentDescriptor{}
	if err := yaml.Unmarshal(componentDescriptorBytes, componentDescriptor); err != nil {
		return "", err
	}

	var mediaType string
	for _, r := range componentDescriptor.Component.Resources {
		if r.Name == obj.Spec.Name {
			mediaType = r.Access.MediaType
			break
		}
	}

	if mediaType == "" {
		return "", errors.New("resource not found")
	}

	return mediaType, nil
}

// Untar takes a destination path and a reader; a tar reader loops over the tarfile
// creating the file structure at 'dst' along the way, and writing any files
func myuntar(dst string, r io.Reader) error {
	tr := tar.NewReader(r)

	for {
		header, err := tr.Next()

		switch {

		// if no more files are found return
		case err == io.EOF:
			return nil

		// return any other error
		case err != nil:
			return err

		// if the header is nil, just skip it (not sure how this happens)
		case header == nil:
			continue
		}

		// the target location where the dir/file should be created
		target := filepath.Join(dst, header.Name)

		// the following switch could also be done using fi.Mode(), not sure if there
		// a benefit of using one vs. the other.
		// fi := header.FileInfo()

		// check the file type
		switch header.Typeflag {

		// if its a dir and it doesn't exist create it
		case tar.TypeDir:
			if _, err := os.Stat(target); err != nil {
				if err := os.MkdirAll(target, 0755); err != nil {
					return err
				}
			}

		// if it's a file create it
		case tar.TypeReg:
			f, err := os.Create(target)
			if err != nil {
				return err
			}

			// copy over contents
			if _, err := io.Copy(f, tr); err != nil {
				return err
			}

			// manually close here after each file operation; defering would cause each file close
			// to wait until all operations have completed.
			f.Close()
		}
	}
}
