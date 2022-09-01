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

// TransformationSpec defines the desired state of Transformation
type TransformationSpec struct {
	// +required
	ResourceRef *meta.LocalObjectReference `json:"resourceRef,omitempty"`

	// +required
	TransformStorageRef *meta.LocalObjectReference `json:"transformStorageRef,omitempty"`

	// +required
	Transform Transform `json:"transform,omitempty"`
}

type Transform struct {
	Operation string `json:"operation,omitempty"`
}

// TransformationStatus defines the observed state of Transformation
type TransformationStatus struct {
	// +optional
	Artifact string `json:"artifact,omitempty"`

	// +optional
	LatestVersion string `json:"latestVersion,omitempty"`

	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the last reconciled generation.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Transformation is the Schema for the transformations API
type Transformation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TransformationSpec   `json:"spec,omitempty"`
	Status TransformationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// TransformationList contains a list of Transformation
type TransformationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Transformation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Transformation{}, &TransformationList{})
}
