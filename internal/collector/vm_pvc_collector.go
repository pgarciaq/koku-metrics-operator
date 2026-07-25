//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import (
	"fmt"
	"strings"

	gologr "github.com/go-logr/logr"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"

	"github.com/project-koku/koku-metrics-operator/internal/dirconfig"
)

const rosVMPVCFilePrefix = "ros-openshift-vm-pvc-"

// rosVMPVCRow represents one PVC attachment for a VM in a single 15-minute interval.
type rosVMPVCRow struct {
	IntervalStart     string `mapstructure:"interval_start"`
	IntervalEnd       string `mapstructure:"interval_end"`
	VMName            string `mapstructure:"name"`
	Namespace         string `mapstructure:"namespace"`
	NodeName          string `mapstructure:"node"`
	PVCName           string `mapstructure:"persistentvolumeclaim"`
	DiskCapacityBytes string `mapstructure:"pvc_disk_bytes"`
	VolumeMode        string `mapstructure:"volume_mode"`
}

func (rosVMPVCRow) csvHeader() []string {
	return []string{
		"interval_start",
		"interval_end",
		"vm_name",
		"namespace",
		"node_name",
		"pvc_name",
		"disk_capacity_bytes",
		"volume_mode",
	}
}

func (row rosVMPVCRow) csvRow() []string {
	return []string{
		row.IntervalStart,
		row.IntervalEnd,
		row.VMName,
		row.Namespace,
		row.NodeName,
		row.PVCName,
		row.DiskCapacityBytes,
		row.VolumeMode,
	}
}

func (row rosVMPVCRow) string() string { return strings.Join(row.csvRow(), ",") }

func (row rosVMPVCRow) reportPrefix() string {
	return row.IntervalStart
}

func writeVMPVCReport(
	log gologr.Logger,
	c *PrometheusCollector,
	dirCfg *dirconfig.DirectoryConfig,
	yearMonth string,
	pvcResults mappedResults,
) error {
	if len(pvcResults) == 0 {
		return nil
	}

	pvcRows := make(mappedCSVStruct)
	for key, val := range pvcResults {
		pvcName := stringValue(val, "persistentvolumeclaim")
		if pvcName == "" {
			continue
		}
		row := newROSVMPVCRow(c.TimeSeries)
		row.VMName = stringValue(val, "name")
		row.Namespace = stringValue(val, "namespace")
		row.NodeName = stringValue(val, "node")
		row.PVCName = pvcName
		row.DiskCapacityBytes = floatToString(avgFloat(val, "pvc_disk_bytes"))
		row.VolumeMode = stringValue(val, "volume_mode")
		if row.VolumeMode == "" {
			row.VolumeMode = "Filesystem"
		}
		pvcRows[key] = row
	}

	if len(pvcRows) == 0 {
		return nil
	}

	emptyRow := newROSVMPVCRow(c.TimeSeries)
	pvcReport := report{
		file: &file{
			name: rosVMPVCFilePrefix + yearMonth + ".csv",
			path: dirCfg.Reports.Path,
		},
		data: &data{
			queryData: pvcRows,
			headers:   emptyRow.csvHeader(),
			prefix:    emptyRow.reportPrefix(),
		},
	}

	log.WithName("writeResults").Info("writing ROS VM PVC results to file", "filename", pvcReport.file.getName())
	if err := pvcReport.writeReport(); err != nil {
		return fmt.Errorf("failed to write ROS VM PVC report: %v", err)
	}
	return nil
}

func newROSVMPVCRow(ts *promv1.Range) rosVMPVCRow {
	return rosVMPVCRow{
		IntervalStart: ts.Start.Format("2006-01-02 15:04:05"),
		IntervalEnd:   ts.End.Format("2006-01-02 15:04:05"),
	}
}
