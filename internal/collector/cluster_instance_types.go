//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	"github.com/project-koku/koku-metrics-operator/internal/dirconfig"
)

const (
	clusterInstanceTypesFileName = "cluster_instance_types.json"
	instanceTypeAPIGroup         = "instancetype.kubevirt.io"
	instanceTypeResource         = "virtualmachineclusterinstancetypes"
	instanceTypeClassLabel       = "instancetype.kubevirt.io/class"
)

var instanceTypeAPIVersions = []string{"v1beta1", "v1alpha2"}

// ClusterInstanceTypeEntry is one VirtualMachineClusterInstancetype exported for ROS.
type ClusterInstanceTypeEntry struct {
	Name      string `json:"name"`
	Series    string `json:"series"`
	VCPU      int32  `json:"vcpu"`
	MemoryGiB int32  `json:"memory_gib"`
	GPUs      int32  `json:"gpus"`
}

// ClusterInstanceTypesDocument is written to cluster_instance_types.json in the upload tarball.
type ClusterInstanceTypesDocument struct {
	ClusterUUID   string                     `json:"cluster_uuid"`
	CollectedAt   time.Time                  `json:"collected_at"`
	InstanceTypes []ClusterInstanceTypeEntry `json:"instance_types"`
}

// ClusterInstanceTypesCollectionResult holds the outcome of cluster instance type collection.
type ClusterInstanceTypesCollectionResult struct {
	CRDAvailable bool
	TypeCount    int
	FileWritten  bool
	Error        error
}

// GenerateClusterInstanceTypes lists VirtualMachineClusterInstancetype objects and writes
// cluster_instance_types.json when the instancetype.kubevirt.io API is available.
func GenerateClusterInstanceTypes(restConfig *rest.Config, dirCfg *dirconfig.DirectoryConfig, clusterUUID string) ClusterInstanceTypesCollectionResult {
	log := log.WithName("GenerateClusterInstanceTypes")

	if restConfig == nil || dirCfg == nil {
		return ClusterInstanceTypesCollectionResult{}
	}
	if !shouldCollectVMMetrics(&PrometheusCollector{RestConfig: restConfig}) {
		log.Info("KubeVirt not available, skipping cluster instance type collection")
		return ClusterInstanceTypesCollectionResult{}
	}

	gvr, available := instanceTypeGVR(restConfig)
	if !available {
		log.Info("instancetype.kubevirt.io CRD not available, skipping cluster instance type collection")
		return ClusterInstanceTypesCollectionResult{}
	}

	dynClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return ClusterInstanceTypesCollectionResult{CRDAvailable: true, Error: fmt.Errorf("create dynamic client: %w", err)}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	items, err := listClusterInstanceTypes(ctx, dynClient, gvr)
	if err != nil {
		return ClusterInstanceTypesCollectionResult{CRDAvailable: true, Error: fmt.Errorf("list cluster instance types: %w", err)}
	}

	entries := make([]ClusterInstanceTypeEntry, 0, len(items))
	for _, item := range items {
		entry, ok := clusterInstanceTypeFromUnstructured(&item)
		if !ok {
			continue
		}
		entries = append(entries, entry)
	}

	doc := ClusterInstanceTypesDocument{
		ClusterUUID:   clusterUUID,
		CollectedAt:   time.Now().UTC(),
		InstanceTypes: entries,
	}
	payload, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return ClusterInstanceTypesCollectionResult{CRDAvailable: true, TypeCount: len(entries), Error: fmt.Errorf("marshal cluster instance types: %w", err)}
	}

	outPath := filepath.Join(dirCfg.Reports.Path, clusterInstanceTypesFileName)
	if err := os.WriteFile(outPath, payload, 0644); err != nil {
		return ClusterInstanceTypesCollectionResult{
			CRDAvailable: true,
			TypeCount:    len(entries),
			Error:        fmt.Errorf("write cluster instance types file: %w", err),
		}
	}

	log.Info("wrote cluster instance types", "path", outPath, "count", len(entries))
	return ClusterInstanceTypesCollectionResult{CRDAvailable: true, TypeCount: len(entries), FileWritten: true}
}

func instanceTypeGVR(config *rest.Config) (schema.GroupVersionResource, bool) {
	for _, version := range instanceTypeAPIVersions {
		if !isInstanceTypeCRDAvailable(config, version) {
			continue
		}
		return schema.GroupVersionResource{
			Group:    instanceTypeAPIGroup,
			Version:  version,
			Resource: instanceTypeResource,
		}, true
	}
	return schema.GroupVersionResource{}, false
}

func isInstanceTypeCRDAvailable(config *rest.Config, version string) bool {
	if config == nil {
		return false
	}
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return false
	}
	resources, err := discoveryClient.ServerResourcesForGroupVersion(instanceTypeAPIGroup + "/" + version)
	if err != nil {
		return false
	}
	for _, r := range resources.APIResources {
		if r.Name == instanceTypeResource {
			return true
		}
	}
	return false
}

func listClusterInstanceTypes(ctx context.Context, dynClient dynamic.Interface, gvr schema.GroupVersionResource) ([]unstructured.Unstructured, error) {
	var result []unstructured.Unstructured
	continueToken := ""
	for {
		list, err := dynClient.Resource(gvr).List(ctx, metav1.ListOptions{
			Limit:    500,
			Continue: continueToken,
		})
		if err != nil {
			return nil, err
		}
		result = append(result, list.Items...)
		continueToken = list.GetContinue()
		if continueToken == "" {
			break
		}
	}
	return result, nil
}

func clusterInstanceTypeFromUnstructured(item *unstructured.Unstructured) (ClusterInstanceTypeEntry, bool) {
	if item == nil {
		return ClusterInstanceTypeEntry{}, false
	}
	name := item.GetName()
	if name == "" {
		return ClusterInstanceTypeEntry{}, false
	}

	series := ""
	if labels := item.GetLabels(); labels != nil {
		series = labels[instanceTypeClassLabel]
	}

	spec, _ := item.Object["spec"].(map[string]interface{})
	vcpu := int32(0)
	if spec != nil {
		if cpu, ok := spec["cpu"].(map[string]interface{}); ok {
			if guest, ok := cpu["guest"].(int64); ok {
				vcpu = int32(guest)
			} else if guest, ok := cpu["guest"].(float64); ok {
				vcpu = int32(guest)
			}
		}
	}
	if vcpu < 1 {
		vcpu = 1
	}

	memGiB := int32(0)
	if spec != nil {
		if mem, ok := spec["memory"].(map[string]interface{}); ok {
			if guest, ok := mem["guest"].(string); ok {
				if q, err := parseQuantity(guest); err == nil {
					memGiB = int32((q + (1024 * 1024 * 1024) - 1) / (1024 * 1024 * 1024))
				}
			}
		}
	}
	if memGiB < 1 {
		memGiB = 1
	}

	gpuCount := int32(0)
	if spec != nil {
		if gpus, ok := spec["gpus"].([]interface{}); ok {
			gpuCount = int32(len(gpus))
		}
	}

	return ClusterInstanceTypeEntry{
		Name:      name,
		Series:    series,
		VCPU:      vcpu,
		MemoryGiB: memGiB,
		GPUs:      gpuCount,
	}, true
}

// IsClusterInstanceTypesFile reports whether name is the ROS cluster instance types payload.
func IsClusterInstanceTypesFile(name string) bool {
	base := filepath.Base(name)
	return base == clusterInstanceTypesFileName ||
		(strings.Contains(base, "cluster_instance_types") && strings.HasSuffix(base, ".json"))
}

// ParseClusterInstanceTypesFile unmarshals cluster_instance_types.json from disk.
func ParseClusterInstanceTypesFile(path string) (ClusterInstanceTypesDocument, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ClusterInstanceTypesDocument{}, err
	}
	var doc ClusterInstanceTypesDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		return ClusterInstanceTypesDocument{}, err
	}
	return doc, nil
}
