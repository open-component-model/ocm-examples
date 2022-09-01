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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OCMKustomizationSpec defines the desired state of OCMKustomization
type OCMKustomizationSpec struct {
	// +requried
	SourceRef OCMSourceRef `json:"sourceRef,omitempty"`

	// +required
	KustomizeTemplate KustomizationTemplate `json:"kustomizeTemplate,omitempty"`
}

type OCMSourceRef struct {
	// +kubebuilder:validation:Enum:=Resource;Transformation;Realization
	// +required
	Kind string `json:"kind,omitempty"`

	// +required
	Name string `json:"name,omitempty"`
}

type KustomizationTemplate struct {
	// +required
	Interval metav1.Duration `json:"interval,omitempty"`

	// +required
	Path string `json:"path,omitempty"`

	// +required
	Prune bool `json:"prune,omitempty"`
}

// OCMKustomizationStatus defines the observed state of OCMKustomization
type OCMKustomizationStatus struct {
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the last reconciled generation.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// OCMKustomization is the Schema for the ocmkustomizations API
type OCMKustomization struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OCMKustomizationSpec   `json:"spec,omitempty"`
	Status OCMKustomizationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// OCMKustomizationList contains a list of OCMKustomization
type OCMKustomizationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OCMKustomization `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OCMKustomization{}, &OCMKustomizationList{})
}
