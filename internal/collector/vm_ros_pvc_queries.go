//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import "github.com/prometheus/common/model"

// rosVMPVCQueries collects per-PVC disk allocation for each running VM.
// The results are written to a companion CSV (ros-openshift-vm-pvc-YYYYMM.csv)
// consumed by ros-ocp-backend for accurate shared-storage detection.
var rosVMPVCQueries = &querys{
	query{
		Name:        "vm-ros-pvc-disk-bytes",
		QueryString: QueryMap["ros:vm_pvc_disk_bytes"],
		MetricKey: staticFields{
			"name":                   "name",
			"namespace":              "namespace",
			"node":                   "node",
			"persistentvolumeclaim":  "persistentvolumeclaim",
			"volume_mode":            "volume_mode",
		},
		QueryValue: &saveQueryValue{ValName: "pvc_disk_bytes"},
		RowKey:     []model.LabelName{"name", "namespace", "node", "persistentvolumeclaim"},
	},
}
