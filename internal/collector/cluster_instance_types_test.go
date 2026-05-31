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
		return ClusterInstanceTypesCollectionResult{CRDAvailable: true, TypeCount: len(entries), Error: err}
	}

	outPath := filepath.Join(dirCfg.Reports.Path, clusterInstanceTypesFileName)
	if err := os.WriteFile(outPath, payload, 0644); err != nil {
		return ClusterInstanceTypesCollectionResult{CRDAvailable: true, TypeCount: len(entries), Error: err}
	}
	return ClusterInstanceTypesCollectionResult{CRDAvailable: true, TypeCount: len(entries), FileWritten: true}
}
