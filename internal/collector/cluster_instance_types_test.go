//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/rest"

	"github.com/project-koku/koku-metrics-operator/internal/dirconfig"
)

func TestGenerateClusterInstanceTypes_CRDNotAvailable(t *testing.T) {
	kubeVirtCRDChecker = func(*rest.Config) bool { return true }
	defer func() { kubeVirtCRDChecker = IsKubeVirtCRDAvailable }()

	result := GenerateClusterInstanceTypes(nil, nil, "cluster-1")
	if result.CRDAvailable {
		t.Error("expected CRDAvailable=false with nil config")
	}
	if result.FileWritten {
		t.Error("expected no file written")
	}
}

func TestGenerateClusterInstanceTypes_KubeVirtNotAvailable(t *testing.T) {
	kubeVirtCRDChecker = func(*rest.Config) bool { return false }
	defer func() { kubeVirtCRDChecker = IsKubeVirtCRDAvailable }()

	dir := t.TempDir()
	dirCfg := &dirconfig.DirectoryConfig{
		Reports: dirconfig.Directory{Path: dir},
	}
	result := GenerateClusterInstanceTypes(&rest.Config{}, dirCfg, "cluster-1")
	if result.FileWritten {
		t.Error("expected no file when KubeVirt unavailable")
	}
}

func TestGenerateClusterInstanceTypes_EmptyListWritesFile(t *testing.T) {
	gvr := schema.GroupVersionResource{
		Group:    instanceTypeAPIGroup,
		Version:  "v1beta1",
		Resource: instanceTypeResource,
	}
	scheme := runtime.NewScheme()
	listKinds := map[schema.GroupVersionResource]string{
		gvr: "VirtualMachineClusterInstancetypeList",
	}
	dynClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds)

	kubeVirtCRDChecker = func(*rest.Config) bool { return true }
	instanceTypeGVRFunc = func(*rest.Config) (schema.GroupVersionResource, bool) {
		return gvr, true
	}
	defer func() {
		kubeVirtCRDChecker = IsKubeVirtCRDAvailable
		instanceTypeGVRFunc = instanceTypeGVR
	}()

	dir := t.TempDir()
	dirCfg := &dirconfig.DirectoryConfig{
		Reports: dirconfig.Directory{Path: dir},
	}

	result := generateClusterInstanceTypesWithClient(&rest.Config{}, dirCfg, "abc-123", dynClient)
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if !result.FileWritten {
		t.Fatal("expected file written for empty instance type list")
	}

	doc, err := ParseClusterInstanceTypesFile(filepath.Join(dir, clusterInstanceTypesFileName))
	if err != nil {
		t.Fatalf("parse written file: %v", err)
	}
	if doc.ClusterUUID != "abc-123" {
		t.Errorf("cluster_uuid = %q, want abc-123", doc.ClusterUUID)
	}
	if len(doc.InstanceTypes) != 0 {
		t.Errorf("expected empty instance_types, got %d", len(doc.InstanceTypes))
	}
}

func TestGenerateClusterInstanceTypes_ParsesResources(t *testing.T) {
	gvr := schema.GroupVersionResource{
		Group:    instanceTypeAPIGroup,
		Version:  "v1beta1",
		Resource: instanceTypeResource,
	}
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "instancetype.kubevirt.io/v1beta1",
			"kind":       "VirtualMachineClusterInstancetype",
			"metadata": map[string]interface{}{
				"name": "u1.large",
				"labels": map[string]interface{}{
					instanceTypeClassLabel: "general-purpose",
				},
			},
			"spec": map[string]interface{}{
				"cpu": map[string]interface{}{
					"guest": int64(2),
				},
				"memory": map[string]interface{}{
					"guest": "8Gi",
				},
			},
		},
	}
	scheme := runtime.NewScheme()
	listKinds := map[schema.GroupVersionResource]string{
		gvr: "VirtualMachineClusterInstancetypeList",
	}
	dynClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, obj)

	kubeVirtCRDChecker = func(*rest.Config) bool { return true }
	instanceTypeGVRFunc = func(*rest.Config) (schema.GroupVersionResource, bool) {
		return gvr, true
	}
	defer func() {
		kubeVirtCRDChecker = IsKubeVirtCRDAvailable
		instanceTypeGVRFunc = instanceTypeGVR
	}()

	dir := t.TempDir()
	dirCfg := &dirconfig.DirectoryConfig{
		Reports: dirconfig.Directory{Path: dir},
	}

	result := generateClusterInstanceTypesWithClient(&rest.Config{}, dirCfg, "abc-123", dynClient)
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if result.TypeCount != 1 {
		t.Fatalf("TypeCount = %d, want 1", result.TypeCount)
	}

	doc, err := ParseClusterInstanceTypesFile(filepath.Join(dir, clusterInstanceTypesFileName))
	if err != nil {
		t.Fatalf("parse written file: %v", err)
	}
	if len(doc.InstanceTypes) != 1 {
		t.Fatalf("len(instance_types) = %d, want 1", len(doc.InstanceTypes))
	}
	got := doc.InstanceTypes[0]
	if got.Name != "u1.large" || got.VCPU != 2 || got.MemoryGiB != 8 || got.Series != "general-purpose" {
		t.Errorf("unexpected entry: %+v", got)
	}
}

func TestClusterInstanceTypeFromUnstructured_GPUs(t *testing.T) {
	item := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name": "gn1.xlarge",
			},
			"spec": map[string]interface{}{
				"cpu":    map[string]interface{}{"guest": int64(4)},
				"memory": map[string]interface{}{"guest": "16Gi"},
				"gpus": []interface{}{
					map[string]interface{}{"name": "gpu1"},
				},
			},
		},
	}
	entry, ok := clusterInstanceTypeFromUnstructured(item)
	if !ok {
		t.Fatal("expected ok")
	}
	if entry.GPUs != 1 {
		t.Errorf("GPUs = %d, want 1", entry.GPUs)
	}
}

func TestClusterInstanceTypesDocument_JSONFormat(t *testing.T) {
	doc := ClusterInstanceTypesDocument{
		ClusterUUID: "abc-123",
		InstanceTypes: []ClusterInstanceTypeEntry{
			{Name: "u1.large", Series: "general-purpose", VCPU: 2, MemoryGiB: 8},
		},
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	var decoded ClusterInstanceTypesDocument
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.InstanceTypes[0].Name != "u1.large" {
		t.Errorf("decoded name = %q", decoded.InstanceTypes[0].Name)
	}
}

func TestIsClusterInstanceTypesFile(t *testing.T) {
	if !IsClusterInstanceTypesFile("cluster_instance_types.json") {
		t.Error("expected match for bare filename")
	}
	if !IsClusterInstanceTypesFile("/tmp/uuid-cluster_instance_types.json") {
		t.Error("expected match for path suffix")
	}
	if IsClusterInstanceTypesFile("ros-openshift-vm-usage.csv") {
		t.Error("unexpected match for CSV")
	}
}

// instanceTypeGVRFunc allows tests to inject GVR without API discovery.
var instanceTypeGVRFunc = instanceTypeGVR

func generateClusterInstanceTypesWithClient(
	restConfig *rest.Config,
	dirCfg *dirconfig.DirectoryConfig,
	clusterUUID string,
	dynClient dynamic.Interface,
) ClusterInstanceTypesCollectionResult {
	if restConfig == nil || dirCfg == nil {
		return ClusterInstanceTypesCollectionResult{}
	}
	if !shouldCollectVMMetrics(&PrometheusCollector{RestConfig: restConfig}) {
		return ClusterInstanceTypesCollectionResult{}
	}
	gvr, available := instanceTypeGVRFunc(restConfig)
	if !available {
		return ClusterInstanceTypesCollectionResult{}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	items, err := listClusterInstanceTypes(ctx, dynClient, gvr)
	if err != nil {
		return ClusterInstanceTypesCollectionResult{CRDAvailable: true, Error: err}
	}

	doc, err := buildClusterInstanceTypesDocument(ctx, restConfig, dynClient, gvr, clusterUUID, items)
	if err != nil {
		return ClusterInstanceTypesCollectionResult{CRDAvailable: true, Error: err}
	}

	payload, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return ClusterInstanceTypesCollectionResult{CRDAvailable: true, TypeCount: len(doc.InstanceTypes), Error: err}
	}

	outPath := filepath.Join(dirCfg.Reports.Path, clusterInstanceTypesFileName)
	if err := os.WriteFile(outPath, payload, 0644); err != nil {
		return ClusterInstanceTypesCollectionResult{CRDAvailable: true, TypeCount: len(doc.InstanceTypes), Error: err}
	}
	return ClusterInstanceTypesCollectionResult{CRDAvailable: true, TypeCount: len(doc.InstanceTypes), FileWritten: true}
}

func TestGenerateClusterInstanceTypes_PreferencesAndVMMappings(t *testing.T) {
	instanceGVR := schema.GroupVersionResource{
		Group: instanceTypeAPIGroup, Version: "v1beta1", Resource: instanceTypeResource,
	}
	prefGVR := schema.GroupVersionResource{
		Group: instanceTypeAPIGroup, Version: "v1beta1", Resource: preferenceResource,
	}
	vmListGVR := schema.GroupVersionResource{
		Group: vmAPIGroup, Version: "v1", Resource: vmResource,
	}

	prefObj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "instancetype.kubevirt.io/v1beta1",
		"kind":       "VirtualMachineClusterPreference",
		"metadata": map[string]interface{}{
			"name": "database",
			"labels": map[string]interface{}{
				instanceTypeClassLabel: "memory-intensive",
			},
		},
	}}
	vmWithPref := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "kubevirt.io/v1",
		"kind":       "VirtualMachine",
		"metadata": map[string]interface{}{
			"name":      "db-server-01",
			"namespace": "production",
		},
		"spec": map[string]interface{}{
			"preference": map[string]interface{}{"name": "database"},
		},
	}}
	vmWithoutPref := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "kubevirt.io/v1",
		"kind":       "VirtualMachine",
		"metadata": map[string]interface{}{
			"name":      "plain-vm",
			"namespace": "production",
		},
		"spec": map[string]interface{}{},
	}}

	scheme := runtime.NewScheme()
	listKinds := map[schema.GroupVersionResource]string{
		instanceGVR: "VirtualMachineClusterInstancetypeList",
		prefGVR:     "VirtualMachineClusterPreferenceList",
		vmListGVR:   "VirtualMachineList",
	}
	dynClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme, listKinds, prefObj, vmWithPref, vmWithoutPref,
	)

	kubeVirtCRDChecker = func(*rest.Config) bool { return true }
	instanceTypeGVRFunc = func(*rest.Config) (schema.GroupVersionResource, bool) {
		return instanceGVR, true
	}
	preferenceGVRFunc = func(*rest.Config) (schema.GroupVersionResource, bool) {
		return prefGVR, true
	}
	vmGVRFunc = func(*rest.Config) (schema.GroupVersionResource, bool) {
		return vmListGVR, true
	}
	defer func() {
		kubeVirtCRDChecker = IsKubeVirtCRDAvailable
		instanceTypeGVRFunc = instanceTypeGVR
		preferenceGVRFunc = preferenceGVR
		vmGVRFunc = vmGVR
	}()

	dir := t.TempDir()
	dirCfg := &dirconfig.DirectoryConfig{Reports: dirconfig.Directory{Path: dir}}

	result := generateClusterInstanceTypesWithClient(&rest.Config{}, dirCfg, "abc-123", dynClient)
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if !result.FileWritten {
		t.Fatal("expected file written")
	}

	doc, err := ParseClusterInstanceTypesFile(filepath.Join(dir, clusterInstanceTypesFileName))
	if err != nil {
		t.Fatalf("parse written file: %v", err)
	}
	if len(doc.Preferences) != 1 {
		t.Fatalf("len(preferences) = %d, want 1", len(doc.Preferences))
	}
	if doc.Preferences[0].Name != "database" || doc.Preferences[0].Class != "memory-intensive" {
		t.Errorf("unexpected preference: %+v", doc.Preferences[0])
	}
	if len(doc.VMPreferences) != 1 {
		t.Fatalf("len(vm_preferences) = %d, want 1", len(doc.VMPreferences))
	}
	if doc.VMPreferences["production/db-server-01"] != "database" {
		t.Errorf("vm_preferences = %#v", doc.VMPreferences)
	}
	if _, ok := doc.VMPreferences["production/plain-vm"]; ok {
		t.Error("VM without preference should not appear in vm_preferences")
	}
}

func TestGenerateClusterInstanceTypes_InstanceTypeCRDNotAvailable(t *testing.T) {
	kubeVirtCRDChecker = func(*rest.Config) bool { return true }
	instanceTypeGVRFunc = func(*rest.Config) (schema.GroupVersionResource, bool) {
		return schema.GroupVersionResource{}, false
	}
	defer func() {
		kubeVirtCRDChecker = IsKubeVirtCRDAvailable
		instanceTypeGVRFunc = instanceTypeGVR
	}()

	dir := t.TempDir()
	dirCfg := &dirconfig.DirectoryConfig{Reports: dirconfig.Directory{Path: dir}}
	result := GenerateClusterInstanceTypes(&rest.Config{}, dirCfg, "cluster-1")
	if result.FileWritten {
		t.Error("expected no file when instance type CRD is unavailable")
	}
	if result.CRDAvailable {
		t.Error("expected CRDAvailable=false when instance type GVR is missing")
	}
}

func TestClusterInstanceTypeFromUnstructured_Defaults(t *testing.T) {
	item := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "minimal"},
	}}
	entry, ok := clusterInstanceTypeFromUnstructured(item)
	if !ok {
		t.Fatal("expected ok for minimal spec")
	}
	if entry.VCPU != 1 || entry.MemoryGiB != 1 {
		t.Errorf("expected minimum vcpu/memory of 1, got vcpu=%d mem=%d", entry.VCPU, entry.MemoryGiB)
	}
	if entry.GPUs != 0 {
		t.Errorf("GPUs = %d, want 0", entry.GPUs)
	}
}

func TestClusterInstanceTypeFromUnstructured_Invalid(t *testing.T) {
	if _, ok := clusterInstanceTypeFromUnstructured(nil); ok {
		t.Error("nil item should not parse")
	}
	if _, ok := clusterInstanceTypeFromUnstructured(&unstructured.Unstructured{}); ok {
		t.Error("empty metadata should not parse")
	}
}

func TestVmPreferencesFromItems_EmptyReturnsNil(t *testing.T) {
	if got := vmPreferencesFromItems(nil); got != nil {
		t.Errorf("nil items should return nil map, got %#v", got)
	}
	vmNoPref := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{
			"name": "vm1", "namespace": "ns1",
		},
		"spec": map[string]interface{}{},
	}}
	if got := vmPreferencesFromItems([]unstructured.Unstructured{*vmNoPref}); got != nil {
		t.Errorf("VMs without preference should yield nil map, got %#v", got)
	}
}

func TestClusterInstanceTypesDocument_AllSections(t *testing.T) {
	doc := ClusterInstanceTypesDocument{
		ClusterUUID: "cluster-uuid",
		CollectedAt: time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC),
		InstanceTypes: []ClusterInstanceTypeEntry{
			{Name: "u1.large", Series: "general", VCPU: 2, MemoryGiB: 8, GPUs: 0},
		},
		Preferences: []ClusterPreferenceEntry{
			{Name: "database", Class: "memory-intensive"},
		},
		VMPreferences: map[string]string{
			"production/db-vm": "database",
		},
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"cluster_uuid", "collected_at", "instance_types", "preferences", "vm_preferences"} {
		if _, ok := decoded[key]; !ok {
			t.Errorf("JSON missing key %q", key)
		}
	}
	types, ok := decoded["instance_types"].([]interface{})
	if !ok || len(types) != 1 {
		t.Fatalf("instance_types = %#v", decoded["instance_types"])
	}
	prefs, ok := decoded["preferences"].([]interface{})
	if !ok || len(prefs) != 1 {
		t.Fatalf("preferences = %#v", decoded["preferences"])
	}
	vmPrefs, ok := decoded["vm_preferences"].(map[string]interface{})
	if !ok || vmPrefs["production/db-vm"] != "database" {
		t.Fatalf("vm_preferences = %#v", decoded["vm_preferences"])
	}
}

func TestClusterPreferenceFromUnstructured_ClassFromAnnotation(t *testing.T) {
	item := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{
			"name": "highperformance",
			"annotations": map[string]interface{}{
				instanceTypeClassLabel: "compute-intensive",
			},
		},
	}}
	entry, ok := clusterPreferenceFromUnstructured(item)
	if !ok {
		t.Fatal("expected ok")
	}
	if entry.Class != "compute-intensive" {
		t.Errorf("class = %q, want compute-intensive", entry.Class)
	}
}
