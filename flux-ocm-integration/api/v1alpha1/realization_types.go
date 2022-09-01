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

// RealizationSpec defines the desired state of Realization
type RealizationSpec struct {
	// +required
	Interval metav1.Duration `json:"interval,omitempty"`

	// +required
	ComponentRef *meta.NamespacedObjectReference `json:"componentRef,omitempty"`

	// +required
	PackageResource *meta.LocalObjectReference `json:"packageResource,omitempty"`

	//TODO: this needs to use unstructed yaml type (look at tf controller)
	// +optional
	Config map[string]string `json:"config,omitempty"`
}

// RealizationStatus defines the observed state of Realization
type RealizationStatus struct {
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

// Realization is the Schema for the realizations API
type Realization struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RealizationSpec   `json:"spec,omitempty"`
	Status RealizationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RealizationList contains a list of Realization
type RealizationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Realization `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Realization{}, &RealizationList{})
}
