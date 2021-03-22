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

// This controller implementation is based on design doc docs/design-proposals/multi-tenancy/multi-tenancy-network.md

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"k8s.io/klog"
)

const (
	defaultWorkers = 1
)

var (
	upper_kubeconfig_string string
	lower_kubeconfig_string string
	workers                 int
)

func main() {
	klog.InitFlags(nil)
	flag.Parse()
	if workers <= 0 {
		workers = defaultWorkers
	}

	upper_cluster, err := parseClusterConfig(upper_kubeconfig_string)
	if err != nil {
		klog.Fatalf("Error in parsing upper cluster config: %v", err)
	}

	lower_cluster, err := parseClusterConfig(lower_kubeconfig_string)
	if err != nil {
		klog.Fatalf("Error in parsing lower cluster config: %v", err)
	}

	defer klog.Flush()

	stopCh := make(chan struct{})
	defer close(stopCh)

	connector := New(upper_cluster, lower_cluster)
	connector.Run(workers, stopCh)

	klog.Infof("Cluster Connector Exited.")
}

func init() {
	flag.StringVar(&upper_kubeconfig_string, "upper-config", "", "the cluster type and the path to the kubeconfig to connect to the upper cluster.")
	flag.StringVar(&lower_kubeconfig_string, "lower-config", "", "the cluster type and the path to the kubeconfig to connect to the lower cluster.")
	flag.IntVar(&workers, "concurrent-workers", defaultWorkers, "The number of workers that are allowed to process concurrently.")
}

func parseClusterConfig(input string) (*ClusterConfig, error) {
	basedir, _ := filepath.Abs(filepath.Dir(os.Args[0]))

	parts := strings.SplitN(input, ":", 2)

	switch strings.ToLower(parts[0]) {
	case "arktos":
		kubeconfigFile := strings.TrimSpace(parts[1])
		if FileExists(kubeconfigFile) == false {
			return nil, fmt.Errorf("Failed to access kube-config file (%s)", kubeconfigFile)
		}
		return &ClusterConfig{
			clusterType: ArktosCluster,
			kubeconfig:  fmt.Sprintf("--kubeconfig=%s", kubeconfigFile),
			kubectl:     filepath.Join(basedir, "arktos/kubectl"),
		}, nil

	case "kubernetes":
		kubeconfigFile := strings.TrimSpace(parts[1])
		if FileExists(kubeconfigFile) == false {
			return nil, fmt.Errorf("Failed to access kube-config file (%s)", kubeconfigFile)
		}
		return &ClusterConfig{
			clusterType: K8sCluster,
			kubeconfig:  fmt.Sprintf("--kubeconfig=%s", kubeconfigFile),
			kubectl:     filepath.Join(basedir, "kubernetes/kubectl"),
		}, nil

	case "k3s":
		return &ClusterConfig{
			clusterType: K3sCluster,
			kubeconfig:  "",
			kubectl:     "sudo k3s kubectl",
		}, nil

	default:
		return nil, fmt.Errorf("Unrecognized Cluster Type")
	}
}

func equalClusterConfig(cc1 *ClusterConfig, cc2 *ClusterConfig) bool {
	return cc1.clusterType == cc2.clusterType && cc1.kubectl == cc2.kubectl
}
