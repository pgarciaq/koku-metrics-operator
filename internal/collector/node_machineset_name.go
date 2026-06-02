//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	"github.com/go-logr/logr"
)

const (
	machineAPIGroup    = "machine.openshift.io"
	machineResource    = "machines"
	machineSetKind     = "MachineSet"
	machineAPIVersions = "v1beta1"
)

// BuildNodeMachineSetNameMap lists Machine resources and maps node name to owning MachineSet name.
// Best-effort: returns an empty map when the Machine API is unavailable or listing fails.
func BuildNodeMachineSetNameMap(restConfig *rest.Config, log logr.Logger) map[string]string {
	result := make(map[string]string)
	if restConfig == nil {
		return result
	}
	gvr, ok := machineGVR(restConfig)
	if !ok {
		log.Info("Machine API not available; machineset_name will be empty in ROS reports")
		return result
	}
	dynClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		log.Error(err, "failed to create dynamic client for Machine API")
		return result
	}
	ctx := context.Background()
	items, err := listClusterResources(ctx, dynClient, gvr)
	if err != nil {
		log.Error(err, "failed to list Machines for machineset_name mapping")
		return result
	}
	for i := range items {
		nodeName, machineSetName := machineSetNameFromMachine(&items[i])
		if nodeName == "" || machineSetName == "" {
			continue
		}
		result[nodeName] = machineSetName
	}
	return result
}

func machineGVR(config *rest.Config) (schema.GroupVersionResource, bool) {
	if !isMachineAPIGroupAvailable(config) {
		return schema.GroupVersionResource{}, false
	}
	return schema.GroupVersionResource{
		Group:    machineAPIGroup,
		Version:  machineAPIVersions,
		Resource: machineResource,
	}, true
}

func isMachineAPIGroupAvailable(config *rest.Config) bool {
	if config == nil {
		return false
	}
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return false
	}
	resources, err := discoveryClient.ServerResourcesForGroupVersion(machineAPIGroup + "/" + machineAPIVersions)
	if err != nil {
		return false
	}
	for _, r := range resources.APIResources {
		if r.Name == machineResource {
			return true
		}
	}
	return false
}

func machineSetNameFromMachine(item *unstructured.Unstructured) (nodeName, machineSetName string) {
	if item == nil {
		return "", ""
	}
	nodeName = machineNodeName(item)
	if nodeName == "" {
		return "", ""
	}
	for _, ref := range item.GetOwnerReferences() {
		if ref.Kind == machineSetKind && ref.Name != "" {
			return nodeName, ref.Name
		}
	}
	return nodeName, ""
}

func machineNodeName(item *unstructured.Unstructured) string {
	nodeRef, found, _ := unstructured.NestedMap(item.Object, "status", "nodeRef")
	if !found || len(nodeRef) == 0 {
		return ""
	}
	name, _, _ := unstructured.NestedString(nodeRef, "name")
	return name
}

// applyMachineSetNamesToNodeRows sets MachineSetName on each nodeRow from the node→MachineSet map.
func applyMachineSetNamesToNodeRows(nodeRows mappedCSVStruct, machineSetByNode map[string]string) {
	if len(machineSetByNode) == 0 {
		return
	}
	for key, row := range nodeRows {
		nr, ok := row.(*nodeRow)
		if !ok {
			continue
		}
		node := nr.Node
		if node == "" {
			node = key
		}
		if ms, ok := machineSetByNode[node]; ok {
			nr.MachineSetName = ms
		}
	}
}

// collectNodeMachineSetNames is a test seam for BuildNodeMachineSetNameMap.
var collectNodeMachineSetNames = BuildNodeMachineSetNameMap

func collectNodeMachineSetNamesForCollector(c *PrometheusCollector, log logr.Logger) map[string]string {
	if c == nil || c.RestConfig == nil {
		return map[string]string{}
	}
	return collectNodeMachineSetNames(c.RestConfig, log)
}
