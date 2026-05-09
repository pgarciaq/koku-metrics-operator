//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import "strings"

// snapshotRow represents a single row in the snapshot inventory CSV.
// Unlike other collectors, snapshot data comes from the Kubernetes API
// rather than Prometheus queries.
type snapshotRow struct {
	IntervalStart       string
	IntervalEnd         string
	Namespace           string
	SnapshotName        string
	SourcePVCName       string
	VolumeSnapshotClass string
	StorageClass        string
	CreationTimestamp   string
	RestoreSizeBytes    string
	ReadyToUse          string
	SourcePVCExists     string
	RestoredPVCCount    string
	Labels              string
}

func (snapshotRow) csvHeader() []string {
	return []string{
		"interval_start",
		"interval_end",
		"namespace",
		"snapshot_name",
		"source_pvc_name",
		"volume_snapshot_class",
		"storageclass",
		"creation_timestamp",
		"restore_size_bytes",
		"ready_to_use",
		"source_pvc_exists",
		"restored_pvc_count",
		"labels",
	}
}

func (row snapshotRow) csvRow() []string {
	return []string{
		row.IntervalStart,
		row.IntervalEnd,
		row.Namespace,
		row.SnapshotName,
		row.SourcePVCName,
		row.VolumeSnapshotClass,
		row.StorageClass,
		row.CreationTimestamp,
		row.RestoreSizeBytes,
		row.ReadyToUse,
		row.SourcePVCExists,
		row.RestoredPVCCount,
		row.Labels,
	}
}

func (row snapshotRow) string() string { return strings.Join(row.csvRow(), ",") }
