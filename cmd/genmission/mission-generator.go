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
	"io/ioutil"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
	// this tool is based on the cluster-connetor tool
	base "k8s.io/kubernetes/cmd/cluster-connector"
)

const (
	defaultWorkers = 1
)

type MissionForYaml struct {
	metav1.TypeMeta `yaml:",inline"`
	Metadata        MetadataForYaml  `yaml:"metadata,omitempty"`
	Spec            base.MissionSpec `yaml:"spec"`
}

type MetadataForYaml struct {
	Name string `yaml:"name,omitempty"`
}

var (
	input_file           string
	output_file          string
	destination_clusters string
	destination_labels   string
	mission_name         string
	print                bool

	mission MissionForYaml
)

func main() {
	klog.InitFlags(nil)
	flag.Parse()

	completeFlags()

	completeMission()

	data, err := yaml.Marshal(&mission)
	if err != nil {
		klog.Fatalf("Error in marshalling mission object: %v", err)
	}

	err = ioutil.WriteFile(output_file, data, 0644)
	if err != nil {
		klog.Fatalf("Error in writing to file (%s): %v", output_file, err)
	}

	if print {
		fmt.Printf("%s", data)
	}
}

func init() {
	flag.StringVar(&input_file, "input", "", "the path to the resource yaml file to convert.")
	flag.StringVar(&output_file, "output", "", "the path to save the convereted mission yaml file.")
	flag.StringVar(&mission_name, "mission", "", "the name of the mission. If not specified, will be [input-file-name]-mission")
	flag.StringVar(&destination_clusters, "clusters", "", "the names of the clusters that the mission will be deployed, speparatated by \";\". When empty, the mission will go to all the clusters.")
	flag.StringVar(&destination_labels, "labels", "", "the labels used to specify the clusters to deploy the mission, speparated by \";\" if there are multiple.")
	flag.BoolVar(&print, "print", false, "if true, will print the generated mission yam file.")
}

func completeFlags() {
	input_file = strings.TrimSpace(input_file)
	if base.FileExists(input_file) == false {
		klog.Fatalf("Cannot access input file (%s)", input_file)
	}

	output_file = strings.TrimSpace(output_file)
	if len(output_file) == 0 {
		output_file = input_file + ".mission"
	}

	if base.FileExists(output_file) {
		klog.Infof("Output file (%s) already exists, will overwrite it.", output_file)
	}
}

func completeMission() {
	mission.APIVersion = "arktosedge.futurewei.com/v1"
	mission.Kind = "Mission"
	mission.Metadata.Name = getMissionName()

	data, err := ioutil.ReadFile(input_file)
	if err != nil {
		klog.Fatalf("Error in reading file (%s), error: %v", input_file, err)
	}
	mission.Spec.Content = string(data)
	mission.Spec.Placement.Clusters = getClusterNames()
	mission.Spec.Placement.MatchLabels = getClusterLabels()
}

func getMissionName() string {
	if len(mission_name) > 0 {
		return mission_name
	}

	filename := filepath.Base(input_file)
	ext := filepath.Ext(filename)

	name := strings.TrimSuffix(filename, ext)
	if len(name) == 0 {
		name = "generated"
	}

	return name + "-mission"
}

func getClusterNames() []base.GenericClusterReference {
	names := []base.GenericClusterReference{}
	for _, name := range strings.Split(destination_clusters, ",") {
		if len(strings.TrimSpace(name)) > 0 {
			names = append(names, base.GenericClusterReference{Name: strings.TrimSpace(name)})
		}
	}
	return names
}

func getClusterLabels() map[string]string {
	labels := map[string]string{}
	for _, label := range strings.Split(destination_labels, ",") {
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
