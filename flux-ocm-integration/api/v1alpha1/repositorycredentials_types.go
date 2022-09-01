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

package v1alpha1

import (
	"github.com/fluxcd/pkg/apis/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RepositoryCredentialsSpec defines the desired state of RepositoryCredentials
type RepositoryCredentialsSpec struct {
	// +required
	Credentials []RepositoryCredential `json:"credentials,omitempty"`
}

// RepositoryCredential defines a single repository credential item
type RepositoryCredential struct {
	// +required
	Kind string `json:"kind,omitempty"`

	// +required
	Name string `json:"name,omitempty"`

	// +required
	URL string `json:"url,omitempty"`

	// +required
	SecretRef *meta.LocalObjectReference `json:"secretRef,omitempty"`
}

// RepositoryCredentialsStatus defines the observed state of RepositoryCredentials
type RepositoryCredentialsStatus struct {
	// ObservedGeneration is the last reconciled generation.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// RepositoryCredentials is the Schema for the repositorycredentials API
type RepositoryCredentials struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RepositoryCredentialsSpec   `json:"spec,omitempty"`
	Status RepositoryCredentialsStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RepositoryCredentialsList contains a list of RepositoryCredentials
type RepositoryCredentialsList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RepositoryCredentials `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RepositoryCredentials{}, &RepositoryCredentialsList{})
}
