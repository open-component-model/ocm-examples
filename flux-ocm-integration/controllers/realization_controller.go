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
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/minio/minio-go/v7"
	ocmruntime "github.com/open-component-model/ocm/pkg/runtime"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/containers/image/v5/pkg/compression"
	"github.com/mandelsoft/vfs/pkg/osfs"
	"github.com/mandelsoft/vfs/pkg/vfs"
	miniocredentials "github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/open-component-model/ocm/pkg/common"
	"github.com/open-component-model/ocm/pkg/contexts/credentials"
	"github.com/open-component-model/ocm/pkg/contexts/oci/repositories/ocireg"
	"github.com/open-component-model/ocm/pkg/contexts/ocm"
	"github.com/open-component-model/ocm/pkg/contexts/ocm/accessmethods/ociartefact"
	ocmmeta "github.com/open-component-model/ocm/pkg/contexts/ocm/compdesc/meta/v1"
	"github.com/open-component-model/ocm/pkg/contexts/ocm/cpi"
	"github.com/open-component-model/ocm/pkg/contexts/ocm/repositories/genericocireg"
	ocmutils "github.com/open-component-model/ocm/pkg/contexts/ocm/utils"
	"github.com/open-component-model/ocm/pkg/contexts/ocm/utils/localize"
	"github.com/open-component-model/ocm/pkg/utils"
	transferv1alpha1 "github.com/phoban01/ocm-flux/api/v1alpha1"
	"github.com/phoban01/ocm-flux/internal/substitutions"
)

type Package struct {
	TemplateResource  template                `json:"templateResource"`
	ConfigRules       []configRule            `json:"configRules"`
	ConfigScheme      map[string]interface{}  `json:"configScheme"`
	LocalizationRules []localize.Localization `json:"localizationRules"`
}

type template struct {
	Name string `json:"name"`
}

type configRule struct {
	Value string `json:"value"`
	File  string `json:"file"`
	Path  string `json:"path"`
}

// RealizationReconciler reconciles a Realization object
type RealizationReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	MinioURL string
}

//+kubebuilder:rbac:groups=transfer.phoban.io,resources=realizations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=transfer.phoban.io,resources=realizations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=transfer.phoban.io,resources=realizations/finalizers,verbs=update

//+kubebuilder:rbac:groups=transfer.phoban.io,resources=components,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

func (r *RealizationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	var obj transferv1alpha1.Realization
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

	if component.Status.Bucket == "" {
		log.Info("bucket status is empty")
		return ctrl.Result{RequeueAfter: time.Minute}, nil

	}
	log.Info("reconciling realization", "name", obj.GetName())

	res, reconcileErr := r.reconcile(ctx, obj, component)
	if err := r.patchStatus(ctx, req, res.Status); err != nil {
		return ctrl.Result{Requeue: true}, err
	}

	if reconcileErr != nil {
		log.Error(reconcileErr, fmt.Sprintf("reconciliation failed"))
	}

	return ctrl.Result{}, nil
}

func (r *RealizationReconciler) reconcile(ctx context.Context, obj transferv1alpha1.Realization, component transferv1alpha1.Component) (transferv1alpha1.Realization, error) {
	session := ocm.NewSession(nil)
	defer session.Close()

	ocmCtx := ocm.ForContext(ctx)

	// create the consumer id for credentials
	consumerID, err := getConsumerIdentityForRepository(component.Spec.Repository)
	if err != nil {
		return obj, err
	}

	// fetch the credentials for the component storage
	creds, err := r.getCredentialsForRepository(ctx, component.GetNamespace(), component.Spec.Repository)
	if err != nil {
		return obj, err
	}

	// TODO: set credentials should return an error
	ocmCtx.CredentialsContext().SetCredentialsForConsumer(consumerID, creds)

	// configure the repository access
	repoSpec := genericocireg.NewRepositorySpec(ocireg.NewRepositorySpec(component.Spec.Repository.URL), nil)
	repo, err := session.LookupRepository(ocmCtx, repoSpec)
	if err != nil {
		return obj, fmt.Errorf("repo error: %w", err)
	}

	// get the component version
	cv, err := session.LookupComponentVersion(repo, component.Spec.Name, component.Spec.Version)
	if err != nil {
		return obj, fmt.Errorf("component error: %w", err)
	}

	// get the resource
	_, packageData, err := r.getResourceForComponentVersion(cv, obj.Spec.Resource)
	if err != nil {
		return obj, fmt.Errorf("resource error: %w", err)
	}

	// get the blob containing the delivery package
	pkg := new(Package)
	if err := ocmruntime.DefaultYAMLEncoding.Unmarshal(packageData.Bytes(), &pkg); err != nil {
		return obj, fmt.Errorf("package unmarshal error: %w", err)
	}

	// get the template resource
	_, templateBytes, err := r.getResourceForComponentVersion(cv, pkg.TemplateResource.Name)
	if err != nil {
		return obj, fmt.Errorf("template error: %w", err)
	}

	// setup virtual filesystem
	virtualFS, err := osfs.NewTempFileSystem()
	if err != nil {
		return obj, fmt.Errorf("fs error: %w", err)
	}
	defer vfs.Cleanup(virtualFS)

	// extract the template
	if err := utils.ExtractTarToFs(virtualFS, templateBytes); err != nil {
		return obj, fmt.Errorf("extract tar error: %w", err)
	}

	// perform localization
	// generate subs
	subst, err := localize.Localize(pkg.LocalizationRules, cv, nil)
	if err != nil {
		return obj, fmt.Errorf("localize error: %w", err)
	}

	// apply subs
	if err := substitutions.Substitute(subst, virtualFS); err != nil {
		return obj, fmt.Errorf("subst error: %w", err)
	}

	// TODO: merge config
	// move storage setup to component
	// when the controller starts it should create the minio pod
	// then each component creates a bucket
	//

	// write transformed resources to local storage
	bucketName := component.Status.Bucket
	// endpoint := fmt.Sprintf("%s.%s:9000", obj.Spec.TransformStorageRef.Name, namespace)
	endpoint := "localhost:9000"

	// Initialize minio client object.
	mc, err := minio.New(endpoint, &minio.Options{
		Creds:  miniocredentials.NewStaticV4("minioadmin", "minioadmin", ""),
		Secure: false,
	})
	if err != nil {
		return obj, err
	}

	rootDir := "/"

	fi, err := virtualFS.Stat(rootDir)
	if err != nil {
		return obj, err
	}

	sourceDir := filepath.Join(os.TempDir(), fi.Name())

	if err := vfs.Walk(virtualFS, rootDir, func(path string, fi fs.FileInfo, err error) error {
		if m := fi.Mode(); !(m.IsRegular() || m.IsDir()) {
			return nil
		}

		if fi.IsDir() {
			return nil
		}

		abspath := filepath.Join(sourceDir, path)

		opts := minio.PutObjectOptions{ContentType: "text/yaml"}

		if _, err := mc.FPutObject(ctx, bucketName, path, abspath, opts); err != nil {
			return err
		}

		return nil
	}); err != nil {
		return obj, fmt.Errorf("copy files error: %w", err)
	}

	return obj, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RealizationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&transferv1alpha1.Realization{}).
		Complete(r)
}

func (r *RealizationReconciler) patchStatus(ctx context.Context, req ctrl.Request, newStatus transferv1alpha1.RealizationStatus) error {
	var obj transferv1alpha1.Realization

	if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
		return err
	}

	patch := client.MergeFrom(obj.DeepCopy())

	obj.Status = newStatus

	return r.Status().Patch(ctx, &obj, patch)
}

func getConsumerIdentityForRepository(repo transferv1alpha1.Repository) (credentials.ConsumerIdentity, error) {
	regURL, err := url.Parse(repo.URL)
	if err != nil {
		return nil, err
	}

	if regURL.Scheme == "" {
		regURL, err = url.Parse(fmt.Sprintf("oci://%s", repo.URL))
		if err != nil {
			return nil, err
		}
	}

	return credentials.ConsumerIdentity{
		"type":     "OCIRegistry",
		"hostname": regURL.Host,
	}, nil
}

func (r *RealizationReconciler) getCredentialsForRepository(ctx context.Context, namespace string, repo transferv1alpha1.Repository) (credentials.Credentials, error) {
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

func (r *RealizationReconciler) applyLocalization(ctx context.Context, pkg *Package) error {
	return nil
}

func (r *RealizationReconciler) getResourceForComponentVersion(cv ocm.ComponentVersionAccess, resourceName string) (ocm.ResourceAccess, *bytes.Buffer, error) {
	resource, err := cv.GetResource(ocmmeta.NewIdentity(resourceName))
	if err != nil {
		return nil, nil, err
	}

	rd, err := cpi.ResourceReader(resource)
	if err != nil {
		return nil, nil, err
	}
	defer rd.Close()

	decompress, _, err := compression.AutoDecompress(rd)
	if err != nil {
		return nil, nil, err
	}

	data := new(bytes.Buffer)
	if _, err := data.ReadFrom(decompress); err != nil {
		return nil, nil, err
	}

	return resource, data, nil
}

func (r *RealizationReconciler) getResourceReference(cv ocm.ComponentVersionAccess, resourceName string) (string, error) {
	res, _, err := ocmutils.ResolveResourceReference(cv, ocmmeta.NewResourceRef(ocmmeta.NewIdentity(resourceName)), nil)
	if err != nil {
		return "", err
	}

	acc, err := res.Access()
	if err != nil {
		return "", err
	}

	if acc.GetKind() != "ociArtefact" {
		return "", errors.New("localized resource must be an OCI Artifact")
	}

	return acc.(*ociartefact.AccessSpec).ImageReference, nil
}
