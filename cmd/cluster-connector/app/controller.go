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
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	v1 "k8s.io/arktos-ext/pkg/apis/arktosedgeextensions/v1"
	arktoscheme "k8s.io/arktos-ext/pkg/generated/clientset/versioned/scheme"
	arktosinformer "k8s.io/arktos-ext/pkg/generated/informers/externalversions/arktosedgeextensions/v1"
	arktosv1 "k8s.io/arktos-ext/pkg/generated/listers/arktosedgeextensions/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
)

const ShellToUse = "bash"

// Controller represents the edge cluster connector
type Controller struct {
	cacheSynced      cache.InformerSynced
	store            arktosv1.MissionLister
	queue            workqueue.RateLimitingInterface
	masterkubeconfig string
	edgekubeconfig   string
	kubectlpath      string
}

type EventType string

const (
	EventType_Create EventType = "Create"
	EventType_Update EventType = "Update"
	EventType_Delete EventType = "Delete"

	COMMAND_TIMEOUT_SEC = 10

	CRD_FILE = "../../cmd/cluster-connector/data/crd.yaml"
)

type MissionEvent struct {
	EventType EventType
	Mission   *v1.Mission
}

// New creates the controller object
func New(
	informer arktosinformer.MissionInformer,
	masterkubeconfig string,
	edgekubeconfig string,
) *Controller {
	utilruntime.Must(arktoscheme.AddToScheme(scheme.Scheme))

	basedir, _ := filepath.Abs(filepath.Dir(os.Args[0]))
	kubectlpath := filepath.Join(basedir, "kubectl")
	crdPath := filepath.Join(basedir, CRD_FILE)

	crd_apply_cmd := fmt.Sprintf("%s apply -f %s --kubeconfig=%s", kubectlpath, crdPath, edgekubeconfig)
	if ExecCommandLine(crd_apply_cmd, COMMAND_TIMEOUT_SEC) == false {
		klog.Fatalf("Failed to apply the CRD in local cluster!")
	}

	check_master_cmd := fmt.Sprintf("%s get mission --kubeconfig=%s", kubectlpath, masterkubeconfig)
	if ExecCommandLine(check_master_cmd, COMMAND_TIMEOUT_SEC) == false {
		klog.Fatalf("Master cluster does not have the CRD installed!")
	}

	return &Controller{
		store:            informer.Lister(),
		queue:            workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
		cacheSynced:      informer.Informer().HasSynced,
		masterkubeconfig: masterkubeconfig,
		edgekubeconfig:   edgekubeconfig,
		kubectlpath:      kubectlpath,
	}
}

// Run starts the control loop with workers processing the items
func (c *Controller) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	klog.Info("starting edge cluster connector")
	klog.V(5).Info("waiting for informer caches to sync")
	if !cache.WaitForCacheSync(stopCh, c.cacheSynced) {
		klog.Error("failed to wait for cache to sync")
		return
	}

	missions, _ := c.store.Missions().List(labels.Everything())
	for _, mission := range missions {
		c.applyMission(mission)
	}

	klog.V(5).Info("staring workers of edge cluster connector")
	for i := 0; i < workers; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	klog.V(5).Infof("%d workers started", workers)
	<-stopCh
	klog.Info("shutting down edge cluster connector")
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

	missionevent, ok := item.(MissionEvent)
	if !ok {
		klog.Fatalf("got unknown object %v", item)
	}

	mission := missionevent.Mission
	eventType := missionevent.EventType

	if eventType == EventType_Create || eventType == EventType_Update {
		c.applyMission(mission)
	}

	if eventType == EventType_Delete {
		c.deleteMission(mission)
	}

	c.queue.Forget(item)
}

func (c *Controller) applyMission(mission *v1.Mission) {
	if c.masterkubeconfig != c.edgekubeconfig {
		pass_through_cmd := fmt.Sprintf("%s get mission %s -o json --kubeconfig=%s| %s apply --kubeconfig=%s -f - ", c.kubectlpath, mission.Name, c.masterkubeconfig, c.kubectlpath, c.edgekubeconfig)
		ExecCommandLine(pass_through_cmd, COMMAND_TIMEOUT_SEC)
	}

	if matchMission(mission) {
		deploy_mission_cmd := fmt.Sprintf("printf \"%s\" | %s apply --kubeconfig=%s -f - ", mission.Spec.Content, c.kubectlpath, c.edgekubeconfig)
		ExecCommandLine(deploy_mission_cmd, COMMAND_TIMEOUT_SEC)
	}
}

func (c *Controller) deleteMission(mission *v1.Mission) {
	if c.masterkubeconfig != c.edgekubeconfig {
		pass_through_cmd := fmt.Sprintf("%s delete mission %v --kubeconfig=%s", c.kubectlpath, mission.Name, c.edgekubeconfig)
		ExecCommandLine(pass_through_cmd, COMMAND_TIMEOUT_SEC)
	}

	if matchMission(mission) {
		deploy_mission_cmd := fmt.Sprintf("printf \"%s\" | %s delete --kubeconfig=%s -f - ", mission.Spec.Content, c.kubectlpath, c.edgekubeconfig)
		ExecCommandLine(deploy_mission_cmd, COMMAND_TIMEOUT_SEC)
	}

}

// Add puts key of the mission object in the work queue
func (c *Controller) Add(obj interface{}) {
	mission, ok := obj.(*v1.Mission)
	if !ok {
		klog.Fatalf("got unknown object %v", obj)
	}

	c.queue.Add(MissionEvent{Mission: mission, EventType: EventType_Create})
}

func (c *Controller) Update(_, newObj interface{}) {
	newMission, ok := newObj.(*v1.Mission)
	if !ok {
		klog.Fatalf("got unknown new object %v", newObj)
	}

	c.queue.Add(MissionEvent{Mission: newMission, EventType: EventType_Update})
}

// Add puts key of the mission object in the work queue
func (c *Controller) Delete(obj interface{}) {
	mission, ok := obj.(*v1.Mission)
	if !ok {
		klog.Fatalf("got unknown object %v", obj)
	}

	c.queue.Add(MissionEvent{Mission: mission, EventType: EventType_Delete})
}

func ExecCommandLine(commandline string, timeout int) bool {
	fmt.Printf("\n Running ____________ %v\n ", commandline)
	klog.V(3).Infof("Running Command (%v)", commandline)
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

	klog.V(3).Infof("")
	if exitCode != 0 || err != nil {
		klog.Errorf("Command (%v) failed: exitcode: %v, output (%v), error: %v", commandline, exitCode, string(output), err)
		return false
	}

	return true
}

func getClusterNames() []string {
	names := []string{}
	for _, name := range strings.Split(os.Getenv("ARKTOS_CLUSTER_NAME"), ",") {
		if len(strings.TrimSpace(name)) > 0 {
			names = append(names, strings.TrimSpace(name))
		}
	}
	return names
}

func getClusterLabels() map[string]string {
	labels := map[string]string{}
	for _, label := range strings.Split(os.Getenv("ARKTOS_CLUSTER_LABEL"), ",") {
		label = strings.TrimSpace(label)
		if strings.Count(label, ":") == 1 {
			parts := strings.Split(label, ":")
			if len(parts[0]) > 0 && len(parts[1]) > 0 {
				labels[parts[0]] = parts[1]
			}
		}
	}
	return labels
}

func matchMission(mission *v1.Mission) bool {
	fmt.Printf("\n Match (%v) (%v)", mission.Spec.Placement.Clusters, mission.Spec.Placement.ClusterSelector)
	if len(mission.Spec.Placement.Clusters) == 0 && len(mission.Spec.Placement.ClusterSelector.MatchLabels) == 0 && len(mission.Spec.Placement.ClusterSelector.MatchExpressions) == 0 {
		return true
	}

	names := getClusterNames()
	for _, cluster := range mission.Spec.Placement.Clusters {
		for _, name := range names {
			if name == cluster.Name {
				return true
			}
		}
	}

	selector, _ := metav1.LabelSelectorAsSelector(&(mission.Spec.Placement.ClusterSelector))
	return selector.Matches(labels.Set(getClusterLabels()))
}
