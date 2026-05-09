//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
)

func TestSnapshotRowCSVHeader(t *testing.T) {
	row := snapshotRow{}
	headers := row.csvHeader()
	expected := []string{
		"interval_start", "interval_end", "namespace", "snapshot_name",
		"source_pvc_name", "volume_snapshot_class", "storageclass",
		"creation_timestamp", "restore_size_bytes", "ready_to_use",
		"source_pvc_exists", "restored_pvc_count", "labels",
	}
	if len(headers) != len(expected) {
		t.Fatalf("expected %d headers, got %d", len(expected), len(headers))
	}
	for i, h := range headers {
		if h != expected[i] {
			t.Errorf("header[%d] = %q, want %q", i, h, expected[i])
		}
	}
}

func TestSnapshotRowCSVRow(t *testing.T) {
	row := snapshotRow{
		IntervalStart:       "2026-03-01 00:00:00 +0000 UTC",
		IntervalEnd:         "2026-03-01 00:00:00 +0000 UTC",
		Namespace:           "default",
		SnapshotName:        "my-snap",
		SourcePVCName:       "data-pvc",
		VolumeSnapshotClass: "csi-snap-class",
		StorageClass:        "gp3-csi",
		CreationTimestamp:   "2026-01-15T10:00:00Z",
		RestoreSizeBytes:    "10737418240",
		ReadyToUse:          "true",
		SourcePVCExists:     "true",
		RestoredPVCCount:    "1",
		Labels:              `{"velero.io/backup-name":"daily"}`,
	}
	csvRow := row.csvRow()
	if len(csvRow) != 13 {
		t.Fatalf("expected 13 columns, got %d", len(csvRow))
	}
	if csvRow[3] != "my-snap" {
		t.Errorf("snapshot_name = %q, want %q", csvRow[3], "my-snap")
	}
	if csvRow[8] != "10737418240" {
		t.Errorf("restore_size_bytes = %q, want %q", csvRow[8], "10737418240")
	}
}

func TestPVCExists(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "existing-pvc", Namespace: "default"},
		},
	)
	ctx := context.Background()

	if !pvcExists(ctx, client, "default", "existing-pvc") {
		t.Error("expected pvcExists to return true for existing PVC")
	}
	if pvcExists(ctx, client, "default", "nonexistent-pvc") {
		t.Error("expected pvcExists to return false for nonexistent PVC")
	}
}

func TestCountRestoredPVCs(t *testing.T) {
	snapshotName := "db-snapshot"
	apiGroup := "snapshot.storage.k8s.io"

	client := fake.NewSimpleClientset(
		&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "restored-pvc-1", Namespace: "default"},
			Spec: corev1.PersistentVolumeClaimSpec{
				DataSource: &corev1.TypedLocalObjectReference{
					Kind:     "VolumeSnapshot",
					Name:     snapshotName,
					APIGroup: &apiGroup,
				},
			},
		},
		&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "unrelated-pvc", Namespace: "default"},
			Spec: corev1.PersistentVolumeClaimSpec{
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("10Gi"),
					},
				},
			},
		},
	)

	ctx := context.Background()
	count := countRestoredPVCs(ctx, client, "default", snapshotName)
	if count != 1 {
		t.Errorf("expected 1 restored PVC, got %d", count)
	}
}

func TestResolveStorageClass(t *testing.T) {
	sc := "gp3-csi"
	client := fake.NewSimpleClientset(
		&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "my-pvc", Namespace: "default"},
			Spec: corev1.PersistentVolumeClaimSpec{
				StorageClassName: &sc,
			},
		},
	)
	ctx := context.Background()

	result := resolveStorageClass(ctx, client, "default", "my-pvc")
	if result != "gp3-csi" {
		t.Errorf("expected 'gp3-csi', got %q", result)
	}

	result = resolveStorageClass(ctx, client, "default", "nonexistent")
	if result != "" {
		t.Errorf("expected empty string for nonexistent PVC, got %q", result)
	}

	result = resolveStorageClass(ctx, client, "default", "")
	if result != "" {
		t.Errorf("expected empty string for empty PVC name, got %q", result)
	}
}

func TestBuildSnapshotRow(t *testing.T) {
	sc := "gp3-csi"
	client := fake.NewSimpleClientset(
		&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "source-pvc", Namespace: "prod"},
			Spec: corev1.PersistentVolumeClaimSpec{
				StorageClassName: &sc,
			},
		},
	)

	snap := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "snapshot.storage.k8s.io/v1",
			"kind":       "VolumeSnapshot",
			"metadata": map[string]interface{}{
				"name":              "daily-backup-20260301",
				"namespace":         "prod",
				"creationTimestamp": "2026-03-01T06:00:00Z",
				"labels": map[string]interface{}{
					"velero.io/backup-name": "daily-20260301",
				},
			},
			"spec": map[string]interface{}{
				"source": map[string]interface{}{
					"persistentVolumeClaimName": "source-pvc",
				},
				"volumeSnapshotClassName": "csi-rbdplugin-snapclass",
			},
			"status": map[string]interface{}{
				"readyToUse":  true,
				"restoreSize": "10737418240",
			},
		},
	}

	ctx := context.Background()
	row := buildSnapshotRow(ctx, client, snap, "2026-03-01 00:00:00 +0000 UTC", "2026-03-01 00:00:00 +0000 UTC")

	if row.Namespace != "prod" {
		t.Errorf("Namespace = %q, want %q", row.Namespace, "prod")
	}
	if row.SnapshotName != "daily-backup-20260301" {
		t.Errorf("SnapshotName = %q, want %q", row.SnapshotName, "daily-backup-20260301")
	}
	if row.SourcePVCName != "source-pvc" {
		t.Errorf("SourcePVCName = %q, want %q", row.SourcePVCName, "source-pvc")
	}
	if row.VolumeSnapshotClass != "csi-rbdplugin-snapclass" {
		t.Errorf("VolumeSnapshotClass = %q, want %q", row.VolumeSnapshotClass, "csi-rbdplugin-snapclass")
	}
	if row.StorageClass != "gp3-csi" {
		t.Errorf("StorageClass = %q, want %q", row.StorageClass, "gp3-csi")
	}
	if row.ReadyToUse != "true" {
		t.Errorf("ReadyToUse = %q, want %q", row.ReadyToUse, "true")
	}
	if row.SourcePVCExists != "true" {
		t.Errorf("SourcePVCExists = %q, want %q", row.SourcePVCExists, "true")
	}
	if row.RestoreSizeBytes != "10737418240" {
		t.Errorf("RestoreSizeBytes = %q, want %q", row.RestoreSizeBytes, "10737418240")
	}
}

func TestParseQuantity(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"1073741824", 1073741824},
		{"10Gi", 10 * 1024 * 1024 * 1024},
		{"512Mi", 512 * 1024 * 1024},
		{"1Ti", 1024 * 1024 * 1024 * 1024},
		{"100Ki", 100 * 1024},
	}

	for _, tt := range tests {
		result, err := parseQuantity(tt.input)
		if err != nil {
			t.Errorf("parseQuantity(%q) error: %v", tt.input, err)
			continue
		}
		if result != tt.expected {
			t.Errorf("parseQuantity(%q) = %d, want %d", tt.input, result, tt.expected)
		}
	}
}

func TestListAllSnapshots(t *testing.T) {
	scheme := runtime.NewScheme()
	snap1 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "snapshot.storage.k8s.io/v1",
			"kind":       "VolumeSnapshot",
			"metadata": map[string]interface{}{
				"name":      "snap-1",
				"namespace": "default",
			},
		},
	}
	snap2 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "snapshot.storage.k8s.io/v1",
			"kind":       "VolumeSnapshot",
			"metadata": map[string]interface{}{
				"name":      "snap-2",
				"namespace": "prod",
			},
		},
	}

	dynClient := dynamicfake.NewSimpleDynamicClient(scheme, snap1, snap2)

	ctx := context.Background()
	snapshots, err := listAllSnapshots(ctx, dynClient)
	if err != nil {
		t.Fatalf("listAllSnapshots error: %v", err)
	}
	if len(snapshots) != 2 {
		t.Errorf("expected 2 snapshots, got %d", len(snapshots))
	}
}
