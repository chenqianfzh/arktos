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

package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	"gopkg.in/yaml.v2"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
)

const ShellToUse = "bash"

type Connector struct {
	queue         workqueue.RateLimitingInterface
	upperCluster  *ClusterConfig
	lowerCluster  *ClusterConfig
	missionLog    string
	missionLogPos int
}

const (
	COMMAND_TIMEOUT_SEC = 10

	CRD_FILE = "data/crd.yaml"

	MISSION_WATCH_LOG = "mission_watch.log"
)

func New(
	upperCluster *ClusterConfig,
	lowerCluster *ClusterConfig,
) *Connector {
	basedir, _ := filepath.Abs(filepath.Dir(os.Args[0]))
	crdPath := filepath.Join(basedir, CRD_FILE)

	if equalClusterConfig(upperCluster, lowerCluster) == false {
		check_master_cmd := fmt.Sprintf("%s get mission %s", upperCluster.kubectl, upperCluster.kubeconfig)
		if exitCode, _, err := ExecCommandLine(check_master_cmd, COMMAND_TIMEOUT_SEC); exitCode != 0 || err != nil {
			klog.Fatalf("Master cluster does not have the CRD installed!")
		}
	}

	crd_apply_cmd := fmt.Sprintf("%s apply -f %s %s", lowerCluster.kubectl, crdPath, lowerCluster.kubeconfig)
	if exitCode, _, err := ExecCommandLine(crd_apply_cmd, COMMAND_TIMEOUT_SEC); exitCode != 0 || err != nil {
		klog.Fatalf("Failed to apply the CRD in local cluster!")
	}

	missionLog := filepath.Join(os.TempDir(), MISSION_WATCH_LOG)
	os.Create(missionLog)

	return &Connector{
		queue:         workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
		upperCluster:  upperCluster,
		lowerCluster:  lowerCluster,
		missionLog:    missionLog,
		missionLogPos: 0,
	}
}

// Run starts the control loop with workers processing the items
func (c *Connector) Run(workers int, stopCh <-chan struct{}) {

	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	klog.Info("starting edge cluster connector")

	get_mission_cmd := fmt.Sprintf("%s get mission -o json %s  | jq -r '.items[] | .metadata.name '", c.upperCluster.kubectl, c.upperCluster.kubeconfig)
	exitCode, ouputMissions, err := ExecCommandLine(get_mission_cmd, COMMAND_TIMEOUT_SEC)
	if exitCode != 0 || err != nil {
		klog.Fatalf("Failed to list the missions in the upper cluster!")
	}

	missionNameList := strings.Split(ouputMissions, "\n")

	for _, missionName := range missionNameList {
		c.applyMissionByName(missionName)
	}

	c.startMissionWatcher()
}

func (c *Connector) startMissionWatcher() {
	if c.upperCluster.clusterType == ArktosCluster {
		watch_mission_command := fmt.Sprintf("%s get mission --watch-only --output json %s | tee %s", c.upperCluster.kubectl, c.upperCluster.kubeconfig, c.missionLog)
		go ExecCommandLine(watch_mission_command, 0)
	} else {
		watch_mission_command := fmt.Sprintf("%s get mission --watch-only --output json %s | tee %s &", c.upperCluster.kubectl, c.upperCluster.kubeconfig, c.missionLog)
		ExecCommandLine(watch_mission_command, COMMAND_TIMEOUT_SEC)
	}


	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		klog.Fatalf("error in creating mission log watcher : %v", err)
	}
	defer watcher.Close()

	done := make(chan bool)
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&fsnotify.Write == fsnotify.Write {
					klog.Infof("Modifed %#v", event)

					newMission, err := c.captureChangedMission()
					if err != nil {
						klog.Errorf("Error in reading mission log : %v", err)
						continue
					}

					if c.missionExists(newMission.Name) {
						c.applyMission(newMission)
					} else {
						c.deleteMission(newMission)
					}
				} else {
					klog.Infof("file %v event %v. ignore.", c.missionLog, event)
				}

			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				klog.Errorf("error: %v", err)
			}
		}
	}()

	err = watcher.Add(c.missionLog)
	if err != nil {
		klog.Fatalf("error in starting mission log watcher : %v", err)
	}
	<-done
}

func (c *Connector) applyMissionByName(missionName string) {
	missionName = strings.TrimSpace(missionName)
	if len(missionName) == 0 {
		return
	}

	tmpFile := filepath.Join(os.TempDir(), "mission_extract"+".tmp")
	get_mission_cmd := fmt.Sprintf("%s get mission %s -o yaml %s > %s ", c.upperCluster.kubectl, missionName, c.upperCluster.kubeconfig, tmpFile)

	exitCode, output, err := ExecCommandLine(get_mission_cmd, COMMAND_TIMEOUT_SEC)
	if exitCode != 0 || err != nil {
		klog.Errorf("Failed to get the content of mission %v: exitcode: %v, output: (%s), err: %v", missionName, exitCode, output, err)
		return
	}

	var mission Mission

	data, err := ioutil.ReadFile(tmpFile)
	if err != nil {
		klog.Errorf("error in reading file %v, %v", tmpFile, err)
		return
	}

	err = yaml.Unmarshal(data, &mission)
	if err != nil {
		klog.Errorf("error in unmarshall file %v, %v", tmpFile, err)
		return
	}

	c.applyMission(&mission)
}

func (c *Connector) applyMission(mission *Mission) {
	if equalClusterConfig(c.upperCluster, c.lowerCluster) == false {
		pass_through_cmd := fmt.Sprintf("%s get mission %s -o json %s| %s apply %s -f - ", c.upperCluster.kubectl, mission.Name, c.upperCluster.kubeconfig, c.lowerCluster.kubectl, c.lowerCluster.kubeconfig)
		ExecCommandLine(pass_through_cmd, COMMAND_TIMEOUT_SEC)
	}

	if matchMission(mission) {
		deploy_mission_cmd := fmt.Sprintf("printf \"%s\" | %s apply %s -f - ", mission.Spec.Content, c.lowerCluster.kubectl, c.lowerCluster.kubeconfig)
		ExecCommandLine(deploy_mission_cmd, COMMAND_TIMEOUT_SEC)
	}
}

func (c *Connector) deleteMission(mission *Mission) {
	if equalClusterConfig(c.upperCluster, c.lowerCluster) == false  {
		pass_through_cmd := fmt.Sprintf("%s delete mission %v %s", c.lowerCluster.kubectl, mission.Name, c.lowerCluster.kubeconfig)
		ExecCommandLine(pass_through_cmd, COMMAND_TIMEOUT_SEC)
	}

	if matchMission(mission) {
		deploy_mission_cmd := fmt.Sprintf("printf \"%s\" | %s delete %s -f - ", mission.Spec.Content, c.lowerCluster.kubectl, c.lowerCluster.kubeconfig)
		ExecCommandLine(deploy_mission_cmd, COMMAND_TIMEOUT_SEC)
	}
}

func (c *Connector) missionExists(mission string) bool {
	deploy_mission_cmd := fmt.Sprintf("%s get mission %s %s ", c.upperCluster.kubectl, mission, c.upperCluster.kubeconfig)
	exitcode, _, _ := ExecCommandLine(deploy_mission_cmd, COMMAND_TIMEOUT_SEC)

	return exitcode == 0
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

	if exitCode != 0 || err != nil {
		klog.Errorf("Command (%v) failed: exitcode: %v, output (%v), error: %v", commandline, exitCode, string(output), err)
	} else {
		klog.V(3).Infof("Running Command (%v) succeeded", commandline)
	}

	return exitCode, string(output), err
}

func matchMission(mission *Mission) bool {
	if len(mission.Spec.Placement.Clusters) == 0 && len(mission.Spec.Placement.MatchLabels) == 0 {
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

	if len(mission.Spec.Placement.MatchLabels) == 0 {
		return false
	}

	labels := getClusterLabels()
	for k, v := range mission.Spec.Placement.MatchLabels {
		if val, ok := labels[k]; ok && val == v {
			return true
		}
	}

	return false
}

func (c *Connector) captureChangedMission() (*Mission, error) {

	tmpFile := filepath.Join(os.TempDir(), MISSION_WATCH_LOG+".tmp")
	cp_cmd := fmt.Sprintf("yes | cp -rf %v %v", c.missionLog, tmpFile)
	ExecCommandLine(cp_cmd, COMMAND_TIMEOUT_SEC)

	get_line_count_cmd := fmt.Sprintf("wc -l < %s", c.missionLog)
	_, wc_ouptut, _ := ExecCommandLine(get_line_count_cmd, COMMAND_TIMEOUT_SEC)

	line_count, err := strconv.Atoi(strings.TrimSpace(wc_ouptut))
	if err != nil {
		return nil, err
	}

	line_diff := line_count - c.missionLogPos
	if line_diff <= 0 {
		return nil, fmt.Errorf("empty mission.")
	}
	c.missionLogPos = line_count

	cmd_line := fmt.Sprintf("tail -n %v %v", line_diff, tmpFile)
	_, missionString, _ := ExecCommandLine(cmd_line, COMMAND_TIMEOUT_SEC)

	var mission Mission

	err = yaml.Unmarshal([]byte(missionString), &mission)
	if err != nil {
		return nil, fmt.Errorf("error in unmarshall file %v, %v", missionString, err)
	}

	klog.V(3).Infof("succeeded capturing mission object %#v", mission)

	return &mission, nil
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
