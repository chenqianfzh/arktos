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
	"time"

	arktosext "k8s.io/arktos-ext/pkg/generated/clientset/versioned"
	"k8s.io/arktos-ext/pkg/generated/informers/externalversions"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
	"k8s.io/kubernetes/cmd/edgecluster/app"
)

const (
	defaultWorkers = 1
)

var (
	masterkubeconfig string
	edgekubeconfig   string
	workers          int
)

func main() {
	klog.InitFlags(nil)
	flag.Parse()
	if workers <= 0 {
		workers = defaultWorkers
	}

	if !FileExists(masterkubeconfig) {
		klog.Fatalf("Failed to access master-config file (%s).", masterkubeconfig)
	}

	if !FileExists(edgekubeconfig) {
		klog.Fatalf("Failed to access edge-config file (%s).", edgekubeconfig)
	}

	defer klog.Flush()

	masterArktosClient, err := createArktosExtClientFromFile(masterkubeconfig, "edge controller")
	if err != nil {
		klog.Fatalf("error building Arktos extension client: %s", err.Error())
	}

	edgeArktosClient, err := createArktosExtClientFromFile(edgekubeconfig, "edge controller")
	if err != nil {
		klog.Fatalf("error building Arktos extension client: %s", err.Error())
	}

	edgeKubeClient, err := createKubeClientFromFile(edgekubeconfig, "edge controller")
	if err != nil {
		klog.Fatalf("error building Kubernetes client: %s", err.Error())
	}

	informerFactory := externalversions.NewSharedInformerFactory(masterArktosClient, 10*time.Minute)
	stopCh := make(chan struct{})
	defer close(stopCh)

	workloadInformer := informerFactory.Arktosedge().V1().Workloads()
	controller := app.New(masterArktosClient, edgeArktosClient, edgeKubeClient, workloadInformer, masterkubeconfig, edgekubeconfig)
	workloadInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			controller.Add(obj)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			controller.Update(oldObj, newObj)
		},
		DeleteFunc: func(obj interface{}) {
			controller.Delete(obj)
		},
	})

	informerFactory.Start(stopCh)
	controller.Run(workers, stopCh)

	klog.Infof("arktos network controller exited")
}

func init() {
	flag.StringVar(&masterkubeconfig, "master-config", "", "Path to the kubeconfig to connect to the master cluster.")
	flag.StringVar(&edgekubeconfig, "edge-config", "", "Path to the kubeconfig to connect to the client cluster.")
	flag.IntVar(&workers, "concurrent-workers", defaultWorkers, "The number of workers that are allowed to process concurrently.")
}

func createKubeClientFromFile(kubeconfigPath string, componentName string) (*kubernetes.Clientset, error) {
	klog.V(4).Infof("Create kubeclient from config file: %s", kubeconfigPath)
	clientCfg, err := createClientConfigFromFile(kubeconfigPath)
	if err != nil {
		return nil, err
	}

	client, err := kubernetes.NewForConfig(restclient.AddUserAgent(clientCfg, componentName))
	if err != nil {
		return nil, fmt.Errorf("error while creating kube clientset with %s, error %v", kubeconfigPath, err.Error())
	}

	return client, nil
}

func createArktosExtClientFromFile(kubeconfigPath string, componentName string) (*arktosext.Clientset, error) {
	klog.V(4).Infof("Create kubeclient from config file: %s", kubeconfigPath)
	clientCfg, err := createClientConfigFromFile(kubeconfigPath)
	if err != nil {
		return nil, err
	}

	client, err := arktosext.NewForConfig(restclient.AddUserAgent(clientCfg, componentName))
	if err != nil {
		return nil, fmt.Errorf("error while creating arktosext clientset with %s, error %v", kubeconfigPath, err.Error())
	}

	return client, nil
}

func createClientConfigFromFile(kubeconfigPath string) (*restclient.Config, error) {
	clientConfigs, err := clientcmd.LoadFromFile(kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("error while loading kubeconfig from file %v: %v", kubeconfigPath, err)
	}
	configs, err := clientcmd.NewDefaultClientConfig(*clientConfigs, &clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("error while creating kubeconfig: %v", err)
	}

	return configs, nil
}

func FileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}
