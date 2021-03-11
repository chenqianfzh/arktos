/*
Copyright 2020 Authors of Arktos.

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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:nonNamespaced
// +genclient:onlyVerbs=create,get,list,watch,updateStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Workload specifies a network boundary
type Workload struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines desired state of network
	// +optional
	Spec WorkloadSpec `json:"spec"`
}

// WorkloadSpec is a description of Workload
type WorkloadSpec struct {
	// Type is the workload type, supported: deployment, job, cronjob 
	Type string `json:"type"`
	// VPCID is vpc identifier specific to network provider
	// +optional
	Content string `json:"content,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// WorkloadList is a list of Workload objects.
type WorkloadList struct {
	metav1.TypeMeta
	// +optional
	metav1.ListMeta

	// Items is the list of Workload objects in the list
	Items []Workload
}
