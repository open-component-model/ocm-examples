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
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	securejoin "github.com/cyphar/filepath-securejoin"
	"github.com/fluxcd/pkg/untar"
	sourcev1beta2 "github.com/fluxcd/source-controller/api/v1beta2"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	transferv1alpha1 "github.com/phoban01/ocm-flux/api/v1alpha1"
)

// TransformationReconciler reconciles a Transformation object
type TransformationReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=transfer.phoban.io,resources=transformations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=transfer.phoban.io,resources=transformations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=transfer.phoban.io,resources=transformations/finalizers,verbs=update

func (r *TransformationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	var obj transferv1alpha1.Transformation
	if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Info("reconciling transformation", "name", obj.GetName())

	res, reconcileErr := r.reconcile(ctx, obj)
	if err := r.patchStatus(ctx, req, res.Status); err != nil {
		return ctrl.Result{Requeue: true}, err
	}

	if reconcileErr != nil {
		log.Error(reconcileErr, fmt.Sprintf("reconciliation failed"))
		return ctrl.Result{Requeue: true}, reconcileErr
	}

	return ctrl.Result{}, nil
}

func (r *TransformationReconciler) reconcile(ctx context.Context, obj transferv1alpha1.Transformation) (transferv1alpha1.Transformation, error) {
	log := ctrl.LoggerFrom(ctx)
	name := obj.GetName()
	namespace := obj.GetNamespace()

	// read the artifact from the resource source
	var repo sourcev1beta2.OCIRepository
	if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: obj.Spec.ResourceRef.Name}, &repo); err != nil {
		return obj, err
	}

	data, err := r.downloadAsBytes(repo.GetArtifact())
	if err != nil {
		return obj, err
	}

	tmpDir, err := securejoin.SecureJoin(os.TempDir(), fmt.Sprintf("%s-%s", namespace, name))
	if err != nil {
		return obj, err
	}

	if _, err = untar.Untar(data, tmpDir); err != nil {
		return obj, err
	}

	bucketName := "flux-ocm"
	endpoint := fmt.Sprintf("%s.%s:9000", obj.Spec.TransformStorageRef.Name, namespace)
	// endpoint := "localhost:9000"
	accessKeyID := "minioadmin"
	secretAccessKey := "minioadmin"
	useSSL := false

	// Initialize minio client object.
	mc, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return obj, err
	}

	if err := filepath.WalkDir(tmpDir, func(path string, d fs.DirEntry, err error) error {
		if d.IsDir() {
			return nil
		}

		filename := filepath.Base(path)

		contentType := "text/yaml"

		log.Info("copying file to bucket", "path", path, "name", filename)

		if _, err := mc.FPutObject(ctx, bucketName, filename, path, minio.PutObjectOptions{ContentType: contentType}); err != nil {
			fmt.Println(err)
			return err
		}
		return nil
	}); err != nil {
		return obj, err
	}
	// use the minio client to copy the artifact contents to the bucket
	return obj, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *TransformationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&transferv1alpha1.Transformation{}).
		Complete(r)
}

func (r *TransformationReconciler) patchStatus(ctx context.Context, req ctrl.Request, newStatus transferv1alpha1.TransformationStatus) error {
	var obj transferv1alpha1.Transformation

	if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
		return err
	}

	patch := client.MergeFrom(obj.DeepCopy())
	obj.Status = newStatus

	return r.Status().Patch(ctx, &obj, patch)
}

func (r *TransformationReconciler) downloadAsBytes(artifact *sourcev1beta2.Artifact) (*bytes.Buffer, error) {
	artifactURL := artifact.URL
	if hostname := os.Getenv("SOURCE_CONTROLLER_LOCALHOST"); hostname != "" {
		u, err := url.Parse(artifactURL)
		if err != nil {
			return nil, err
		}
		u.Host = hostname
		artifactURL = u.String()
	}

	resp, err := http.Get(artifactURL)
	if err != nil {
		return nil, fmt.Errorf("failed to download artifact, error: %w", err)
	}
	defer resp.Body.Close()

	// check response
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download artifact from %s, status: %s", artifactURL, resp.Status)
	}

	var buf bytes.Buffer

	// verify checksum matches origin
	if err := r.verifyArtifact(artifact, &buf, resp.Body); err != nil {
		return nil, err
	}

	return &buf, nil
}

func (r *TransformationReconciler) verifyArtifact(artifact *sourcev1beta2.Artifact, buf *bytes.Buffer, reader io.Reader) error {
	hasher := sha256.New()

	// for backwards compatibility with source-controller v0.17.2 and older
	if len(artifact.Checksum) == 40 {
		hasher = sha1.New()
	}

	// compute checksum
	mw := io.MultiWriter(hasher, buf)
	if _, err := io.Copy(mw, reader); err != nil {
		return err
	}

	if checksum := fmt.Sprintf("%x", hasher.Sum(nil)); checksum != artifact.Checksum {
		return fmt.Errorf("failed to verify artifact: computed checksum '%s' doesn't match advertised '%s'",
			checksum, artifact.Checksum)
	}

	return nil
}
