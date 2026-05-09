//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/project-koku/koku-metrics-operator/internal/dirconfig"
)

const (
	snapshotFilePrefix = "ros-openshift-snapshot-inventory-"
	snapshotAPIGroup   = "snapshot.storage.k8s.io"
	snapshotAPIVersion = "v1"
	snapshotResource   = "volumesnapshots"
)

var snapshotGVR = schema.GroupVersionResource{
	Group:    snapshotAPIGroup,
	Version:  snapshotAPIVersion,
	Resource: snapshotResource,
}

// SnapshotCollectorConfig holds the configuration for the snapshot collector.
type SnapshotCollectorConfig struct {
	RestConfig *rest.Config
}

// GenerateSnapshotReport collects VolumeSnapshot objects from the Kubernetes
// API and writes a snapshot inventory CSV. It gracefully skips collection
// if the snapshot.storage.k8s.io CRD is not installed on the cluster.
func GenerateSnapshotReport(cfg *SnapshotCollectorConfig, dirCfg *dirconfig.DirectoryConfig, yearMonth string) error {
	snapshotLog := log.WithName("GenerateSnapshotReport")

	if !IsSnapshotCRDAvailable(cfg.RestConfig) {
		snapshotLog.Info("snapshot.storage.k8s.io CRD not available, skipping snapshot inventory collection")
		return nil
	}

	dynClient, err := dynamic.NewForConfig(cfg.RestConfig)
	if err != nil {
		return fmt.Errorf("failed to create dynamic client: %w", err)
	}

	k8sClient, err := kubernetes.NewForConfig(cfg.RestConfig)
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	snapshots, err := listAllSnapshots(ctx, dynClient)
	if err != nil {
		return fmt.Errorf("failed to list VolumeSnapshots: %w", err)
	}

	if len(snapshots) == 0 {
		snapshotLog.Info("no VolumeSnapshots found, skipping report generation")
		return nil
	}

	now := time.Now().UTC()
	intervalStart := now.Format("2006-01-02 15:04:05 +0000 UTC")
	intervalEnd := now.Format("2006-01-02 15:04:05 +0000 UTC")

	rows := make(mappedCSVStruct)
	for _, snap := range snapshots {
		row := buildSnapshotRow(ctx, k8sClient, &snap, intervalStart, intervalEnd)
		ns, _ := snap.GetNamespace(), snap.GetName()
		key := ns + "/" + snap.GetName()
		rows[key] = row
	}

	emptyRow := snapshotRow{}
	snapshotReport := report{
		file: &file{
			name: snapshotFilePrefix + yearMonth + ".csv",
			path: dirCfg.Reports.Path,
		},
		data: &data{
			queryData: rows,
			headers:   emptyRow.csvHeader(),
			prefix:    intervalStart,
		},
	}

	snapshotLog.Info("writing snapshot inventory to file", "filename", snapshotReport.file.getName(), "count", len(rows))
	if err := snapshotReport.writeReport(); err != nil {
		return fmt.Errorf("failed to write snapshot report: %w", err)
	}

	return nil
}

// IsSnapshotCRDAvailable checks if the snapshot.storage.k8s.io API group is
// registered on the cluster via API discovery.
func IsSnapshotCRDAvailable(config *rest.Config) bool {
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return false
	}

	resources, err := discoveryClient.ServerResourcesForGroupVersion(snapshotAPIGroup + "/" + snapshotAPIVersion)
	if err != nil {
		return false
	}

	for _, r := range resources.APIResources {
		if r.Name == snapshotResource {
			return true
		}
	}
	return false
}

func listAllSnapshots(ctx context.Context, dynClient dynamic.Interface) ([]unstructured.Unstructured, error) {
	var result []unstructured.Unstructured
	continueToken := ""

	for {
		list, err := dynClient.Resource(snapshotGVR).Namespace("").List(ctx, metav1.ListOptions{
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

func buildSnapshotRow(ctx context.Context, k8sClient kubernetes.Interface, snap *unstructured.Unstructured, intervalStart, intervalEnd string) snapshotRow {
	namespace := snap.GetNamespace()
	snapshotName := snap.GetName()

	spec, _ := snap.Object["spec"].(map[string]interface{})
	status, _ := snap.Object["status"].(map[string]interface{})

	sourcePVCName := ""
	if spec != nil {
		if source, ok := spec["source"].(map[string]interface{}); ok {
			if pvcName, ok := source["persistentVolumeClaimName"].(string); ok {
				sourcePVCName = pvcName
			}
		}
	}

	volumeSnapshotClass := ""
	if spec != nil {
		if className, ok := spec["volumeSnapshotClassName"].(string); ok {
			volumeSnapshotClass = className
		}
	}

	creationTimestamp := snap.GetCreationTimestamp().UTC().Format("2006-01-02T15:04:05Z")

	restoreSizeBytes := int64(0)
	if status != nil {
		if restoreSize, ok := status["restoreSize"].(string); ok {
			if q, err := parseQuantity(restoreSize); err == nil {
				restoreSizeBytes = q
			}
		}
	}

	readyToUse := false
	if status != nil {
		if ready, ok := status["readyToUse"].(bool); ok {
			readyToUse = ready
		}
	}

	sourcePVCExists := false
	if sourcePVCName != "" {
		sourcePVCExists = pvcExists(ctx, k8sClient, namespace, sourcePVCName)
	}

	restoredPVCCount := countRestoredPVCs(ctx, k8sClient, namespace, snapshotName)

	storageClass := resolveStorageClass(ctx, k8sClient, namespace, sourcePVCName)

	labels := snap.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labelsJSON, _ := json.Marshal(labels)

	return snapshotRow{
		IntervalStart:       intervalStart,
		IntervalEnd:         intervalEnd,
		Namespace:           namespace,
		SnapshotName:        snapshotName,
		SourcePVCName:       sourcePVCName,
		VolumeSnapshotClass: volumeSnapshotClass,
		StorageClass:        storageClass,
		CreationTimestamp:   creationTimestamp,
		RestoreSizeBytes:    strconv.FormatInt(restoreSizeBytes, 10),
		ReadyToUse:          strconv.FormatBool(readyToUse),
		SourcePVCExists:     strconv.FormatBool(sourcePVCExists),
		RestoredPVCCount:    strconv.FormatInt(int64(restoredPVCCount), 10),
		Labels:              string(labelsJSON),
	}
}

func pvcExists(ctx context.Context, client kubernetes.Interface, namespace, name string) bool {
	_, err := client.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
	return err == nil
}

func countRestoredPVCs(ctx context.Context, client kubernetes.Interface, namespace, snapshotName string) int {
	pvcs, err := client.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return 0
	}

	count := 0
	for _, pvc := range pvcs.Items {
		if pvc.Spec.DataSource != nil &&
			pvc.Spec.DataSource.Kind == "VolumeSnapshot" &&
			pvc.Spec.DataSource.Name == snapshotName {
			count++
		}
		if pvc.Spec.DataSourceRef != nil &&
			pvc.Spec.DataSourceRef.Kind == "VolumeSnapshot" &&
			pvc.Spec.DataSourceRef.Name == snapshotName {
			count++
		}
	}
	return count
}

func resolveStorageClass(ctx context.Context, client kubernetes.Interface, namespace, pvcName string) string {
	if pvcName == "" {
		return ""
	}
	pvc, err := client.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, pvcName, metav1.GetOptions{})
	if err != nil {
		return ""
	}
	if pvc.Spec.StorageClassName != nil {
		return *pvc.Spec.StorageClassName
	}
	return ""
}

// parseQuantity parses a Kubernetes resource quantity string (e.g. "10Gi") to bytes.
func parseQuantity(s string) (int64, error) {
	// Try parsing as plain integer first (bytes)
	if v, err := strconv.ParseInt(s, 10, 64); err == nil {
		return v, nil
	}

	// Handle common suffixes
	multipliers := map[string]int64{
		"Ki": 1024,
		"Mi": 1024 * 1024,
		"Gi": 1024 * 1024 * 1024,
		"Ti": 1024 * 1024 * 1024 * 1024,
	}
	for suffix, mult := range multipliers {
		if len(s) > len(suffix) && s[len(s)-len(suffix):] == suffix {
			num := s[:len(s)-len(suffix)]
			if v, err := strconv.ParseInt(num, 10, 64); err == nil {
				return v * mult, nil
			}
		}
	}
	return 0, fmt.Errorf("cannot parse quantity: %s", s)
}
