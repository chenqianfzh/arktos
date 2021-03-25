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

package connector

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ClusterType string

const (
	ArktosCluster ClusterType = "arktos"
	K8sCluster    ClusterType = "k8s"
	K3sCluster    ClusterType = "k3s"
)

type ClusterConfig struct {
	clusterType ClusterType
	kubeconfig  string
	kubectl     string
}

// Mission specifies a mission across the clusters
type Mission struct {
	metav1.TypeMeta `yaml:",inline"`
	// +optional
	metav1.ObjectMeta `yaml:"metadata,omitempty"`

	// +optional
	Spec MissionSpec `yaml:"spec"`
}

// MissionSpec is a description of Mission
type MissionSpec struct {
	Content string `yaml:"content,omitempty"`

	Placement GenericPlacementFields `yaml:"placement,omitempty"`
}

type GenericClusterReference struct {
	Name string `yaml:"name"`
}

type GenericPlacementFields struct {
	Clusters    []GenericClusterReference `yaml:"clusters,omitempty"`
	MatchLabels map[string]string         `yaml:"matchLabels,omitempty"`
}
