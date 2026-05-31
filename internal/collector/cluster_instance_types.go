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
	preferenceResource           = "virtualmachineclusterpreferences"
	instanceTypeClassLabel       = "instancetype.kubevirt.io/class"
	vmAPIGroup                   = "kubevirt.io"
	vmResource                   = "virtualmachines"
)

var (
	instanceTypeAPIVersions = []string{"v1beta1", "v1alpha2"}
	preferenceAPIVersions   = []string{"v1beta1", "v1alpha2"}
	preferenceGVRFunc       = preferenceGVR
	vmGVRFunc               = vmGVR
)

// ClusterInstanceTypeEntry is one VirtualMachineClusterInstancetype exported for ROS.
type ClusterInstanceTypeEntry struct {
	Name      string `json:"name"`
	Series    string `json:"series"`
	VCPU      int32  `json:"vcpu"`
	MemoryGiB int32  `json:"memory_gib"`
	GPUs      int32  `json:"gpus"`
}

// ClusterPreferenceEntry is one VirtualMachineClusterPreference exported for ROS.
type ClusterPreferenceEntry struct {
	Name  string `json:"name"`
	Class string `json:"class"`
}

// ClusterInstanceTypesDocument is written to cluster_instance_types.json in the upload tarball.
type ClusterInstanceTypesDocument struct {
	ClusterUUID   string                     `json:"cluster_uuid"`
	CollectedAt   time.Time                  `json:"collected_at"`
	InstanceTypes []ClusterInstanceTypeEntry `json:"instance_types"`
	Preferences   []ClusterPreferenceEntry   `json:"preferences,omitempty"`
	VMPreferences map[string]string          `json:"vm_preferences,omitempty"`
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

	doc, err := buildClusterInstanceTypesDocument(ctx, restConfig, dynClient, gvr, clusterUUID, items)
	if err != nil {
		return ClusterInstanceTypesCollectionResult{CRDAvailable: true, Error: err}
	}
	entries := doc.InstanceTypes
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

func buildClusterInstanceTypesDocument(
	ctx context.Context,
	restConfig *rest.Config,
	dynClient dynamic.Interface,
	instanceTypeGVR schema.GroupVersionResource,
	clusterUUID string,
	instanceTypeItems []unstructured.Unstructured,
) (ClusterInstanceTypesDocument, error) {
	entries := make([]ClusterInstanceTypeEntry, 0, len(instanceTypeItems))
	for _, item := range instanceTypeItems {
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

	if prefGVR, ok := preferenceGVRFunc(restConfig); ok {
		prefItems, err := listClusterResources(ctx, dynClient, prefGVR)
		if err != nil {
			return ClusterInstanceTypesDocument{}, fmt.Errorf("list cluster preferences: %w", err)
		}
		doc.Preferences = clusterPreferencesFromItems(prefItems)
	}

	if vmResGVR, ok := vmGVRFunc(restConfig); ok {
		vmItems, err := listClusterResources(ctx, dynClient, vmResGVR)
		if err != nil {
			return ClusterInstanceTypesDocument{}, fmt.Errorf("list virtual machines: %w", err)
		}
		doc.VMPreferences = vmPreferencesFromItems(vmItems)
	}

	return doc, nil
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
	return listClusterResources(ctx, dynClient, gvr)
}

func listClusterResources(ctx context.Context, dynClient dynamic.Interface, gvr schema.GroupVersionResource) ([]unstructured.Unstructured, error) {
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

func preferenceGVR(config *rest.Config) (schema.GroupVersionResource, bool) {
	for _, version := range preferenceAPIVersions {
		if !isInstanceTypeCRDAvailable(config, version) {
			continue
		}
		if !isPreferenceCRDAvailable(config, version) {
			continue
		}
		return schema.GroupVersionResource{
			Group:    instanceTypeAPIGroup,
			Version:  version,
			Resource: preferenceResource,
		}, true
	}
	return schema.GroupVersionResource{}, false
}

func isPreferenceCRDAvailable(config *rest.Config, version string) bool {
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
		if r.Name == preferenceResource {
			return true
		}
	}
	return false
}

func vmGVR(config *rest.Config) (schema.GroupVersionResource, bool) {
	if config == nil {
		return schema.GroupVersionResource{}, false
	}
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return schema.GroupVersionResource{}, false
	}
	resources, err := discoveryClient.ServerResourcesForGroupVersion(vmAPIGroup + "/v1")
	if err != nil {
		return schema.GroupVersionResource{}, false
	}
	for _, r := range resources.APIResources {
		if r.Name == vmResource {
			return schema.GroupVersionResource{
				Group:    vmAPIGroup,
				Version:  "v1",
				Resource: vmResource,
			}, true
		}
	}
	return schema.GroupVersionResource{}, false
}

func clusterPreferencesFromItems(items []unstructured.Unstructured) []ClusterPreferenceEntry {
	out := make([]ClusterPreferenceEntry, 0, len(items))
	for i := range items {
		entry, ok := clusterPreferenceFromUnstructured(&items[i])
		if !ok {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func clusterPreferenceFromUnstructured(item *unstructured.Unstructured) (ClusterPreferenceEntry, bool) {
	if item == nil {
		return ClusterPreferenceEntry{}, false
	}
	name := item.GetName()
	if name == "" {
		return ClusterPreferenceEntry{}, false
	}
	class := preferenceClassFromObject(item)
	return ClusterPreferenceEntry{Name: name, Class: class}, true
}

func preferenceClassFromObject(item *unstructured.Unstructured) string {
	if labels := item.GetLabels(); labels != nil {
		if class := labels[instanceTypeClassLabel]; class != "" {
			return class
		}
	}
	if annotations := item.GetAnnotations(); annotations != nil {
		if class := annotations[instanceTypeClassLabel]; class != "" {
			return class
		}
	}
	return ""
}

func vmPreferencesFromItems(items []unstructured.Unstructured) map[string]string {
	out := make(map[string]string)
	for i := range items {
		namespace := items[i].GetNamespace()
		name := items[i].GetName()
		if namespace == "" || name == "" {
			continue
		}
		prefName := vmPreferenceNameFromUnstructured(&items[i])
		if prefName == "" {
			continue
		}
		out[namespace+"/"+name] = prefName
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func vmPreferenceNameFromUnstructured(item *unstructured.Unstructured) string {
	if item == nil {
		return ""
	}
	spec, _ := item.Object["spec"].(map[string]interface{})
	if spec == nil {
		return ""
	}
	pref, _ := spec["preference"].(map[string]interface{})
	if pref == nil {
		return ""
	}
	name, _ := pref["name"].(string)
	return strings.TrimSpace(name)
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
