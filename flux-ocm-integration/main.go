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

package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	imagev1_reflect "github.com/fluxcd/image-reflector-controller/api/v1beta1"
	kustomizev1beta2 "github.com/fluxcd/kustomize-controller/api/v1beta2"
	sourcev1beta2 "github.com/fluxcd/source-controller/api/v1beta2"
	transferv1alpha1 "github.com/phoban01/ocm-flux/api/v1alpha1"
	"github.com/phoban01/ocm-flux/controllers"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	//+kubebuilder:scaffold:imports
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(imagev1_reflect.AddToScheme(scheme))
	utilruntime.Must(transferv1alpha1.AddToScheme(scheme))
	utilruntime.Must(sourcev1beta2.AddToScheme(scheme))
	utilruntime.Must(kustomizev1beta2.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "18aee1df.phoban.io",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	ctx := ctrl.SetupSignalHandler()

	minioServerURL, err := makeMinioServer(ctx, mgr.GetClient(), "ocm-flux-system", "minio", "250Mi")
	if err != nil {
		setupLog.Error(err, "unable to start minio")
		os.Exit(1)
	}

	fmt.Println(minioServerURL)

	if err = (&controllers.ResourceReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Resource")
		os.Exit(1)
	}
	if err = (&controllers.TransformationReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Transformation")
		os.Exit(1)
	}
	if err = (&controllers.OCMKustomizationReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "OCMKustomization")
		os.Exit(1)
	}
	if err = (&controllers.RealizationReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		MinioURL: minioServerURL,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Realization")
		os.Exit(1)
	}
	if err = (&controllers.ComponentReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		MinioURL: minioServerURL,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Component")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func makeMinioServer(ctx context.Context, kubeClient client.Client, namespace, name, capacity string) (string, error) {
	diskSize, err := resource.ParseQuantity(capacity)
	if err != nil {
		return "", err
	}
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "server",
							Image: "quay.io/minio/minio",
							Args: []string{
								"server",
								"/data",
								"--console-address=:9001",
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "workdir",
									MountPath: "/data",
								},
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "s3",
									ContainerPort: 9000,
								},
								{
									Name:          "console",
									ContainerPort: 9001,
								},
							},
							Env: []corev1.EnvVar{},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "workdir",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{
									Medium:    "",
									SizeLimit: &diskSize,
								},
							},
						},
					},
				},
			},
		},
	}

	if err := kubeClient.Create(ctx, deployment); err != nil && !apierrs.IsAlreadyExists(err) {
		return "", err
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name: "s3",
					Port: 9000,
					TargetPort: intstr.IntOrString{
						Type:   1,
						StrVal: "s3",
					},
				},
				{
					Name: "console",
					Port: 9001,
					TargetPort: intstr.IntOrString{
						Type:   1,
						StrVal: "console",
					},
				},
			},
			Selector: map[string]string{
				"app": name,
			},
		},
		Status: corev1.ServiceStatus{
			LoadBalancer: corev1.LoadBalancerStatus{
				Ingress: []corev1.LoadBalancerIngress{},
			},
			Conditions: []metav1.Condition{},
		},
	}

	if err := kubeClient.Create(ctx, svc); err != nil && !apierrs.IsAlreadyExists(err) {
		return "", err
	}

	return fmt.Sprintf("%s.%s:9000", name, namespace), nil
}
