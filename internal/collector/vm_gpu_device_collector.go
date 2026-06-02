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

const rosVMGPUDeviceFilePrefix = "ros-openshift-vm-gpu-device-"

// rosVMGPUDeviceRow is one GPU device sample for ros-ocp-backend vm_gpu_device_digests.
type rosVMGPUDeviceRow struct {
	IntervalStart   string `mapstructure:"interval_start"`
	Namespace       string `mapstructure:"namespace"`
	VMName          string `mapstructure:"name"`
	GPUUUID         string `mapstructure:"gpu_uuid"`
	GPUModel        string `mapstructure:"gpu_model"`
	UtilizationAvg  string `mapstructure:"gpu_utilization_avg"`
	UtilizationMax  string `mapstructure:"gpu_utilization_max"`
	FBUsedAvgMiB    string `mapstructure:"gpu_fb_used_avg_mib"`
	FBUsedMaxMiB    string `mapstructure:"gpu_fb_used_max_mib"`
	SMActiveAvg     string `mapstructure:"gpu_sm_active_avg"`
	TensorActiveAvg string `mapstructure:"gpu_tensor_active_avg"`
	DRAMActiveAvg   string `mapstructure:"gpu_dram_active_avg"`
	MIGProfile      string `mapstructure:"gpu_mig_profile"`
	MaxSlices       string `mapstructure:"gpu_max_slices"`
}

func (rosVMGPUDeviceRow) csvHeader() []string {
	return []string{
		"interval_start",
		"namespace",
		"vm_name",
		"gpu_uuid",
		"gpu_model",
		"utilization_avg",
		"utilization_max",
		"fb_used_avg_mib",
		"fb_used_max_mib",
		"sm_active_avg",
		"tensor_active_avg",
		"dram_active_avg",
		"mig_profile",
		"max_slices",
	}
}

func (row rosVMGPUDeviceRow) csvRow() []string {
	return []string{
		row.IntervalStart,
		row.Namespace,
		row.VMName,
		row.GPUUUID,
		row.GPUModel,
		row.UtilizationAvg,
		row.UtilizationMax,
		row.FBUsedAvgMiB,
		row.FBUsedMaxMiB,
		row.SMActiveAvg,
		row.TensorActiveAvg,
		row.DRAMActiveAvg,
		row.MIGProfile,
		row.MaxSlices,
	}
}

func (row rosVMGPUDeviceRow) string() string { return strings.Join(row.csvRow(), ",") }

func (row rosVMGPUDeviceRow) reportPrefix() string {
	return row.IntervalStart
}

func writeVMGPUDeviceReport(
	log gologr.Logger,
	c *PrometheusCollector,
	dirCfg *dirconfig.DirectoryConfig,
	yearMonth string,
	gpuResults mappedResults,
	podVMINames map[string]string,
) error {
	if len(gpuResults) == 0 {
		return nil
	}

	deviceRows := make(mappedCSVStruct)
	for key, val := range gpuResults {
		ns := stringValue(val, "namespace")
		vmiName := vmiNameForGPURow(val, podVMINames)
		if vmiName == "" || ns == "" {
			continue
		}
		uuid := stringValue(val, "gpu_uuid")
		if uuid == "" {
			continue
		}
		row := newROSVMGPUDeviceRow(c.TimeSeries)
		row.Namespace = ns
		row.VMName = vmiName
		row.GPUUUID = uuid
		row.GPUModel = stringValue(val, "gpu_model")
		row.UtilizationAvg = floatToString(avgFloat(val, "gpu_utilization_avg"))
		row.UtilizationMax = floatToString(avgFloat(val, "gpu_utilization_max"))
		row.FBUsedAvgMiB = floatToString(avgFloat(val, "gpu_fb_used_avg_mib"))
		row.FBUsedMaxMiB = floatToString(avgFloat(val, "gpu_fb_used_max_mib"))
		row.SMActiveAvg = floatToString(avgFloat(val, "gpu_sm_active_avg"))
		row.TensorActiveAvg = floatToString(avgFloat(val, "gpu_tensor_active_avg"))
		row.DRAMActiveAvg = floatToString(avgFloat(val, "gpu_dram_active_avg"))
		row.MIGProfile = stringValue(val, "gpu_mig_profile")
		row.MaxSlices = floatToString(avgFloat(val, "gpu_max_slices"))
		deviceRows[key] = row
	}

	if len(deviceRows) == 0 {
		return nil
	}

	emptyRow := newROSVMGPUDeviceRow(c.TimeSeries)
	deviceReport := report{
		file: &file{
			name: rosVMGPUDeviceFilePrefix + yearMonth + ".csv",
			path: dirCfg.Reports.Path,
		},
		data: &data{
			queryData: deviceRows,
			headers:   emptyRow.csvHeader(),
			prefix:    emptyRow.reportPrefix(),
		},
	}

	log.WithName("writeResults").Info("writing ROS VM GPU device results to file", "filename", deviceReport.file.getName())
	if err := deviceReport.writeReport(); err != nil {
		return fmt.Errorf("failed to write ROS VM GPU device report: %v", err)
	}
	return nil
}

func newROSVMGPUDeviceRow(ts *promv1.Range) rosVMGPUDeviceRow {
	return rosVMGPUDeviceRow{
		IntervalStart: ts.Start.Format("2006-01-02 15:04:05"),
	}
}
