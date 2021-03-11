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

package app

import (
	"fmt"
	"strings"
	"time"
	"os"
	"path/filepath"
	"os/exec"
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	v1 "k8s.io/arktos-ext/pkg/apis/arktosedgeextensions/v1"
	arktos "k8s.io/arktos-ext/pkg/generated/clientset/versioned"
	arktoscheme "k8s.io/arktos-ext/pkg/generated/clientset/versioned/scheme"
	arktosinformer "k8s.io/arktos-ext/pkg/generated/informers/externalversions/arktosedgeextensions/v1"
	arktosv1 "k8s.io/arktos-ext/pkg/generated/listers/arktosedgeextensions/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
//	"k8s.io/apimachinery/pkg/api/errors"
)

const ShellToUse = "bash"

// Controller represents the edge workload controller
type Controller struct {
	cacheSynced        cache.InformerSynced
	store              arktosv1.WorkloadLister
	queue              workqueue.RateLimitingInterface
	masterArktosClient *arktos.Clientset
	edgeArktosClient   *arktos.Clientset
	edgeKubeClient     *kubernetes.Clientset
	recorder           record.EventRecorder
	masterkubeconfig string
	edgekubeconfig string
}

type EventType string

const (
	EventType_Create EventType = "Create"
	EventType_Update EventType = "Update"
	EventType_Delete EventType = "Delete"
)

type WorkloadEvent struct {
	EventType       EventType
	Workload        *v1.Workload
}

// New creates the controller object
func New(
	masterArktosClient *arktos.Clientset,
	edgeArktosClient *arktos.Clientset,
	edgeKubeClient *kubernetes.Clientset,
	informer arktosinformer.WorkloadInformer,
	masterkubeconfig string, 
	edgekubeconfig string,
) *Controller {
	utilruntime.Must(arktoscheme.AddToScheme(scheme.Scheme))
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: edgeKubeClient.CoreV1().EventsWithMultiTenancy(metav1.NamespaceAll, metav1.TenantAll)})

	return &Controller{
		store:              informer.Lister(),
		queue:              workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
		cacheSynced:        informer.Informer().HasSynced,
		masterArktosClient: masterArktosClient,
		edgeArktosClient:   edgeArktosClient,
		edgeKubeClient:     edgeKubeClient,
		recorder:           eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "edge-workload-controller"}),
		masterkubeconfig: masterkubeconfig,
		edgekubeconfig: edgekubeconfig,

	}
}

// Run starts the control loop with workers processing the items
func (c *Controller) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	klog.Info("starting edge workload controller")
	klog.V(5).Info("waiting for informer caches to sync")
	if !cache.WaitForCacheSync(stopCh, c.cacheSynced) {
		klog.Error("failed to wait for cache to sync")
		return
	}

	klog.V(5).Info("staring workers of edge workload controller")
	for i := 0; i < workers; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	klog.V(5).Infof("%d workers started", workers)
	<-stopCh
	klog.Info("shutting down edge workload controller")
}

func (c *Controller) runWorker() {
	for {
		item, queueIsEmpty := c.queue.Get()
		if queueIsEmpty {
			break
		}

		c.process(item)
	}
}

// process will read a single work item off the work queue and attempt to process it
func (c *Controller) process(item interface{}) {
	defer c.queue.Done(item)

	wlevent, ok := item.(WorkloadEvent)
	if !ok {
		klog.Fatalf("got unknown object %v", item)
	}

	wl := wlevent.Workload
	eventType := wlevent.EventType

	basedir, _ := filepath.Abs(filepath.Dir(os.Args[0]))
	kubectlpath := filepath.Join(basedir, "kubectl")

	deploy_wl_cmd := ""
	pass_through_cmd := ""
	if eventType == EventType_Create || eventType == EventType_Update {
		deploy_wl_cmd = fmt.Sprintf("printf \"%s\" | %s apply --kubeconfig=%s -f - ", wl.Spec.Content, kubectlpath, c.edgekubeconfig)
		pass_through_cmd = fmt.Sprintf("%s get workload %s -o json --kubeconfig=%s| %s apply --kubeconfig=%s -f - ", kubectlpath, wl.Name, c.masterkubeconfig, kubectlpath, c.edgekubeconfig)
	} 

	if eventType == EventType_Delete {
		deploy_wl_cmd = fmt.Sprintf("printf \"%s\" | %s delete --kubeconfig=%s -f - ", wl.Spec.Content, kubectlpath, c.edgekubeconfig)
        pass_through_cmd = fmt.Sprintf("%s delete workload %v --kubeconfig=%s", kubectlpath, wl.Name, c.edgekubeconfig)
	}

	//fmt.Printf("\n Running command ------------ %v ", deploy_wl_cmd)
	exitCode, output, err := ExecCommandLine(deploy_wl_cmd, 0)

	fmt.Printf("\n  deployment command %v returned ------------ %v \n %v \n %v", exitCode, output, err)

	exitCode, output, err = ExecCommandLine(pass_through_cmd, 0)

	fmt.Printf("\n  pass throught command %v returned ------------ %v \n %v \n %v", exitCode, output, err)

	/*wl, err := c.store.WorkloadsWithMultiTenancy(tenant).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {

		}
		klog.Warningf("failed to retrieve workload in local cache by tenant %s, name %s: %v", tenant, name, err)
		c.queue.Forget(item)
		return
	}

	klog.V(5).Infof("processing workload %s/%s", wl.Tenant, wl.Name)

	fmt.Printf("\n workload track --------------- : %v", wl)

	if err != nil {
		c.queue.AddRateLimited(key)
		c.recorder.Eventf(wl, corev1.EventTypeWarning, "FailedProvision", "failed to provision workload %s/%s: %v", wl.Tenant, wl.Name, err)
		return
	} */

	//c.recorder.Eventf(wl, corev1.EventTypeNormal, "SuccessfulProvision", "successfully provision workload %v", wl)
	c.queue.Forget(item)
}

// Add puts key of the workload object in the work queue
func (c *Controller) Add(obj interface{}) {
	wl, ok := obj.(*v1.Workload)
	if !ok {
		klog.Fatalf("got unknown object %v", obj)
	}

	c.queue.Add(WorkloadEvent{Workload: wl, EventType: EventType_Create})
}

func (c *Controller) Update(_, newObj interface{}) {
	newWorkload, ok := newObj.(*v1.Workload)
	if !ok {
		klog.Fatalf("got unknown new object %v", newObj)
	}

	c.queue.Add(WorkloadEvent{Workload: newWorkload, EventType: EventType_Update})
}

// Add puts key of the workload object in the work queue
func (c *Controller) Delete(obj interface{}) {
	wl, ok := obj.(*v1.Workload)
	if !ok {
		klog.Fatalf("got unknown object %v", obj)
	}

	c.queue.Add(WorkloadEvent{Workload: wl, EventType: EventType_Delete})
}

func genKey(wl *v1.Workload) string {
	return fmt.Sprintf("%s/%s", wl.Tenant, wl.Name)
}

func parseKey(key string) (tenant, name string, err error) {
	segs := strings.Split(key, "/")
	switch len(segs) {
	case 1:
		tenant = metav1.TenantSystem
		name = segs[0]
		return
	case 2:
		tenant = segs[0]
		name = segs[1]
		return
	default:
		err = fmt.Errorf("invalid key format")
		return
	}
}

func ExecCommandLine(commandline string, timeout int) (int, string, error) {
	var cmd *exec.Cmd
	if timeout > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
		defer cancel()

		cmd = exec.CommandContext(ctx, ShellToUse, "-c", commandline)
	} else {
		cmd = exec.Command(ShellToUse, "-c", commandline)
	}

	exitCode := 0
	var output []byte
	var err error

	if output, err = cmd.CombinedOutput(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		}
	}

	return exitCode, string(output), nil
}
