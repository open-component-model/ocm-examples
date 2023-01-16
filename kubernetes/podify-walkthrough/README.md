## Open Component Model Podify Tutorial

This tutorial will walk you through the steps involved in deploying a microservices application on Kubernetes using the Open Component Model.

We recommend following our getting started guide as a pre-requisite to this walkthrough.

## Table of contents
- [Requirements](#requirements)
- [Architecture](#architecture)
- [Building the component](#building-the-component)
- [Preparing a cluster](#preparing-a-cluster)
- [Examining the application](#examining-the-application)
- [Diffusion](#diffusion)

## Requirements

- [OCM command line tool](https://github.com/open-component-model/ocm)
- [kubectl](https://kubernetes.io/docs/reference/kubectl/)
- [git](https://git-scm.com/downloads)
- [kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation)
- [flux](https://fluxcd.io/flux/installation/#install-the-flux-cli)

### Architecture

We'll be building a microservices application composed of frontend, backend and cache services. We'll use the `podinfo` container to provide the functionality.

### Building the component

The component we'll use is available under the `./components` directory. We won't go into the details of component authoring in this guide, but please consult our website for detailed guides on the component authoring process.

The makefile allows you to easily build, push and sign the component:

```shell
make all OCI_REPO=ghcr.io/${GITHUB_USER}
```

### Preparing a cluster

First, we create a kind cluster:

```shell
$ kind create cluster

Creating cluster "kind" ...
 ✓ Ensuring node image (kindest/node:v1.25.3) 🖼
 ✓ Preparing nodes 📦
 ✓ Writing configuration 📜
 ✓ Starting control-plane 🕹️
 ✓ Installing CNI 🔌
 ✓ Installing StorageClass 💾
Set kubectl context to "kind-kind"
You can now use your cluster with:

kubectl cluster-info --context kind-kind

Thanks for using kind! 😊
```

The controller requires a few secrets to retrieve components from our OCI registry, let's set those up first (please update the arguments with the appropriate values for your GitHub user):

```shell
./scripts/setup-secrets.sh ${GITHUB_USER} ${GITHUB_TOKEN} ${GITHUB_USER_EMAIL}
```

Now we can bootstrap flux:

`flux bootstrap github --owner ${GITHUB_USER} --repository ${TARGET_REPO} --personal --path=./clusters`

Once flux is bootstrapped it will automatically deploy the OCM controllers, let's wait for those to roll out:

```shell
kubectl get deploy -n ocm-system

NAME                   READY   UP-TO-DATE   AVAILABLE   AGE
diffusion-controller   1/1     1            1           1m0s
ocm-controller         1/1     1            1           1m3s
```

### Examining the application

Let's look at how our microservices application is structured using the Custom Resources provided by the Open Component Model controllers:

```shell
tree ./app/podify

./apps/podify
├── componentversion.yaml
└── diffusion.yaml
```

The custom resource defined in `./apps/podify/componentversion.yaml` is responsible for retrieving our Component from an OCM repository:

```yaml
# ./apps/podify/componentversion.yaml
apiVersion: delivery.ocm.software/v1alpha1
kind: ComponentVersion
metadata:
  name: podify
  namespace: ocm-system
spec:
  interval: 10m0s
  component: github.com/weaveworks/podify
  version:
    semver: v1.0.2
  repository:
    url: ghcr.io/phoban01
    secretRef:
      name: creds
  references:
    expand: true
  verify:
  - name: default
    publicKey:
      secretRef:
        name: publickey
```

The custom resource defined in `./apps/podify/diffusion.yaml` is responsible for unpacking the resources references in the Component version and applying them to the cluster:

```yaml
# ./apps/podify/diffusion.yaml
apiVersion: delivery.ocm.software/v1alpha1
kind: Diffusion
metadata:
  name: podify
  namespace: ocm-system
spec:
  interval: 10m0s
  prune: true
  componentVersionRef:
    name: podify
    namespace: ocm-system
  resourceSelector:
    skipRoot: true
    followReferences: true
    matchSelector:
    - field: "type"
      values:
      - kustomize.ocm.fluxcd.io
  pipelineTemplateRef:
    kind: GitRepository
    name: flux-system
    namespace: flux-system
    path: ./templates/podify.yaml
  values:
    frontend:
      manifests:
        message: "Welcome to podify"
        color: "red"
```

### Diffusion

The Diffusion custom resource allows us to automate the process of extracting a resource from a component and executing localization or configuration. With many resources, these tasks could become laborious. To reduce the toil involved we provide a pipeline template in `spec.pipelineTemplateRef` which defines a set of Kuberenetes resources to be created for each item selected by the Diffusion's resource selector (`spec.resourceSelector`).

The pipeline template is a go-template that contains a resource, localization, configuration, Flux OCI Repository and a Flux Kustomization:

<details>
  <summary>Expand to view template...</summary>

```yaml
# ./templates/podify.yaml
apiVersion: config.ocm.software/v1alpha1
kind: PipelineTemplate
metadata:
  name: podify-pipeline-template
steps:
- name: resource
  template:
    apiVersion: delivery.ocm.software/v1alpha1
    kind: Resource
    metadata:
      name: {{ .Parameters.Name }}
      namespace: {{ .Component.Namespace }}
    spec:
      interval: 1m0s
      componentVersionRef:
        name: {{ .Component.Name }}
        namespace: {{ .Component.Namespace }}
      resource:
        name: {{ .Resource }}
        {{ with .Component.Reference  }}
        referencePath:
          - name: {{ . }}
        {{ end }}
      snapshotTemplate:
        name: {{ .Parameters.Name }}
        tag: latest
- name: localize
  template:
    apiVersion: delivery.ocm.software/v1alpha1
    kind: Localization
    metadata:
      name: {{ .Parameters.Name }}
      namespace: {{ .Component.Namespace }}
    spec:
      interval: 1m0s
      componentVersionRef:
        name: {{ .Component.Name }}
        namespace: {{ .Component.Namespace }}
      sourceRef:
        kind: Snapshot
        name: {{ .Parameters.Name }}
        namespace: {{ .Component.Namespace }}
      configRef:
        resource:
          name: config
          {{ with .Component.Reference  }}
          referencePath:
            - name: {{ . }}
          {{ end }}
      snapshotTemplate:
        name: {{ .Parameters.Name }}-localized
        tag: latest
- name: config
  template:
    apiVersion: delivery.ocm.software/v1alpha1
    kind: Configuration
    metadata:
      name: {{ .Parameters.Name }}
      namespace: {{ .Component.Namespace }}
    spec:
      interval: 1m0s
      componentVersionRef:
        name: {{ .Component.Name }}
        namespace: {{ .Component.Namespace }}
      sourceRef:
        kind: Snapshot
        name: {{ .Parameters.Name }}-localized
        namespace: {{ .Component.Namespace }}
      configRef:
        resource:
          name: config
          {{ with .Component.Reference  }}
          referencePath:
            - name: {{ . }}
          {{ end }}
      values: {{ .Values | toYaml | nindent 8 }}
      snapshotTemplate:
        name: {{ .Parameters.Name }}-configured
        tag: latest
- name: flux-source
  template:
    apiVersion: source.toolkit.fluxcd.io/v1beta2
    kind: OCIRepository
    metadata:
      name: {{ .Parameters.Name }}
      namespace: {{ .Component.Namespace }}
    spec:
      interval: 1m0s
      url: oci://{{ .Registry }}/snapshots/{{ .Parameters.Name }}-configured
      insecure: true
      ref:
        tag: latest
- name: flux-kustomization
  template:
    apiVersion: kustomize.toolkit.fluxcd.io/v1beta2
    kind: Kustomization
    metadata:
      name: {{ .Parameters.Name }}
      namespace: {{ .Component.Namespace }}
    spec:
      interval: 1m0s
      prune: true
      targetNamespace: default
      sourceRef:
        kind: OCIRepository
        name: {{ .Parameters.Name }}
      path: ./
```
</details>

If you have followed our earlier tutorial on getting started with Flux and OCM, you will recognize the resources we have used here.

### Deploy the component custom resources

Now that we have an understanding of what our Custom Resources do and how to configure them, we can deploy by creating a flux Kustomization linking to our `./app/podify` directory:

```shell
flux create kustomization podify \
    --path ./apps/podify \
    --source GitRepository/flux-system \
    --depends-on=flux-system/ocm  \
    --prune=true \
    --export > ./clusters/podify_kustomization.yaml
```

Commit this change and push it to the remote repository.

Once Flux has reconciled the Kustomization we should see the OCM resources in the cluster:

```shell
kubectl get componentversions, diffusions -n ocm-system

NAME                                            AGE
componentversion.delivery.ocm.software/podify   83s

NAME                                     AGE
diffusion.delivery.ocm.software/podify   83s
```

Momentarily, our Components will reconcile:

```shell
kubectl get deploy
```
