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

// ComponentSpec defines the desired state of Component
// simply a bucket for metadata related to accessing the component
type ComponentSpec struct {
	// +optional
	Interval metav1.Duration `json:"interval,omitempty"`

	// +optional
	Repository Repository `json:"repository,omitempty"`

	// +optional
	Name string `json:"name,omitempty"`

	// +optional
	Version string `json:"version,omitempty"`

	// +optional
	CredentialsChain RepositoryCredentials `json:"credentialsChain,omitempty"`

	// +optional
	Verify Verification `json:"verify,omitempty"`
}

type Verification struct {
	// +required
	Signature string `json:"signature,omitempty"`

	// +required
	PublicKey *meta.LocalObjectReference `json:"publicKey,omitempty"`
}

type Repository struct {
	URL       string                     `json:"url"`
	SecretRef *meta.LocalObjectReference `json:"secretRef"`
}

// ComponentStatus defines the observed state of Component
type ComponentStatus struct {
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the last reconciled generation.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +optional
	Bucket string `json:"bucket,omitempty"`

	// +optional
	LatestComponentVersion string `json:"latestComponentVersion,omitempty"`

	// +optional
	Digest string `json:"digest,omitempty"`

	// +optional
	IsVerified bool `json:"isVerified"`

	// +optional
	FailedVerificationReason string `json:"failedVerificationReason,omitempty"`
}

// ShouldVerify checks whether a component should be verified or not
func (c *Component) ShouldVerify() bool {
	return c.Spec.Verify.Signature != "" && c.Spec.Verify.PublicKey.Name != ""
}

// IsVerified returns whether a component is verified or not
func (c *Component) IsVerified() bool {
	return c.Status.IsVerified
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Component is the Schema for the components API
type Component struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ComponentSpec   `json:"spec,omitempty"`
	Status ComponentStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ComponentList contains a list of Component
type ComponentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Component `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Component{}, &ComponentList{})
}
