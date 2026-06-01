//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import (
	"fmt"
	"strconv"
	"strings"

	gologr "github.com/go-logr/logr"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"

	"github.com/project-koku/koku-metrics-operator/internal/dirconfig"
)

const (
	rosVMFilePrefix = "ros-openshift-vm-usage-"
	vmQuarterHours  = 4
	secondsPerHour  = 3600.0
)

type vmHourlyAggregator struct {
	hourRange *promv1.Range
	samples   map[string][]mappedValues
}

func newVMHourlyAggregator(hourRange *promv1.Range) *vmHourlyAggregator {
	return &vmHourlyAggregator{
		hourRange: hourRange,
		samples:   make(map[string][]mappedValues),
	}
}

func (a *vmHourlyAggregator) addSample(results mappedResults) {
	for key, val := range results {
		a.samples[key] = append(a.samples[key], val)
	}
}

func (a *vmHourlyAggregator) hasData() bool {
	return len(a.samples) > 0
}

func collectVMQuarterHour(
	log gologr.Logger,
	c *PrometheusCollector,
	dirCfg *dirconfig.DirectoryConfig,
	yearMonth string,
	aggregator *vmHourlyAggregator,
) error {
	log.Info("querying for OpenShift Virtualization VM metrics", "interval_start", c.TimeSeries.Start, "interval_end", c.TimeSeries.End)

	vmResults := mappedResults{}
	if err := c.getQueryResults(c.TimeSeries.End, rosVMQueries, &vmResults, MaxRetries); err != nil {
		return err
	}

	gpuResults := mappedResults{}
	if err := c.getQueryResults(c.TimeSeries.End, rosVMGpuQueries, &gpuResults, MaxRetries); err != nil {
		return err
	}
	podVMINames, _ := fetchPodVMINameMap(c, c.TimeSeries.End)
	mergeVMGPUIntoResults(vmResults, gpuResults, podVMINames)

	if len(vmResults) == 0 {
		log.Info("no running VM metrics returned for interval")
		return nil
	}

	aggregator.addSample(vmResults)

	rosRows := make(mappedCSVStruct)
	for vmKey, val := range vmResults {
		usage := newROSVMRow(c.TimeSeries)
		if err := getStruct(val, &usage, rosRows, vmKey); err != nil {
			return err
		}
	}

	emptyRow := newROSVMRow(c.TimeSeries)
	rosReport := report{
		file: &file{
			name: rosVMFilePrefix + yearMonth + ".csv",
			path: dirCfg.Reports.Path,
		},
		data: &data{
			queryData: rosRows,
			headers:   emptyRow.csvHeader(),
			prefix:    emptyRow.reportPrefix(),
		},
	}

	log.WithName("writeResults").Info("writing ROS VM results to file", "filename", rosReport.file.getName())
	if err := rosReport.writeReport(); err != nil {
		return fmt.Errorf("failed to write ROS VM report: %v", err)
	}

	if err := writeVMGPUDeviceReport(log, c, dirCfg, yearMonth, gpuResults, podVMINames); err != nil {
		return err
	}

	return nil
}

func generateCostVMMetricsReportFromAggregator(
	log gologr.Logger,
	c *PrometheusCollector,
	dirCfg *dirconfig.DirectoryConfig,
	yearMonth string,
	aggregator *vmHourlyAggregator,
) error {
	if aggregator == nil || !aggregator.hasData() {
		log.Info("no VM hourly samples collected, skipping cost VM report")
		return nil
	}

	log.Info("writing hourly cost VM report from 15-minute samples")
	virtualMachineRows := make(mappedCSVStruct)
	for vmKey, sampleSet := range aggregator.samples {
		usage := buildHourlyVMRow(c.TimeSeries, sampleSet)
		virtualMachineRows[vmKey] = usage
	}

	emptyVmRow := newVMRow(c.TimeSeries)
	virtualMachineReport := report{
		file: &file{
			name: vmFilePrefix + yearMonth + ".csv",
			path: dirCfg.Reports.Path,
		},
		data: &data{
			queryData: virtualMachineRows,
			headers:   emptyVmRow.csvHeader(),
			prefix:    emptyVmRow.dateTimes.string(),
		},
	}

	log.WithName("writeResults").Info("writing cost vm results to file", "filename", virtualMachineReport.file.getName())
	if err := virtualMachineReport.writeReport(); err != nil {
		return fmt.Errorf("failed to write cost vm report: %v", err)
	}

	return nil
}

func buildHourlyVMRow(hourRange *promv1.Range, samples []mappedValues) *vmRow {
	row := newVMRow(hourRange)
	if len(samples) == 0 {
		return &row
	}

	avgSample := averageVMSamples(samples)

	row.VMName = stringValue(avgSample, "name")
	row.Namespace = stringValue(avgSample, "namespace")
	row.Node = stringValue(avgSample, "node")
	row.OS = stringValue(avgSample, "guest_os")
	if row.OS == "" {
		row.OS = stringValue(avgSample, "os")
	}

	resourceID := getResourceID(avgSample["provider_id"])
	row.ResourceID = resourceID

	cpuUsageRate := avgFloat(avgSample, "cpu_usage_mc") / 1000.0
	cpuRequestCores := avgFloat(avgSample, "cpu_request_mc") / 1000.0
	cpuLimitCores := avgFloat(avgSample, "cpu_limit_mc") / 1000.0
	memUsageBytes := avgFloat(avgSample, "memory_usage_kib") * 1024.0
	memRequestBytes := avgFloat(avgSample, "memory_request_kib") * 1024.0
	memLimitBytes := avgFloat(avgSample, "memory_limit_kib") * 1024.0
	diskAllocated := avgFloat(avgSample, "disk_allocated_bytes")

	row.CPUUsageSeconds = floatToString(cpuUsageRate * secondsPerHour)
	row.CPURequestCores = floatToString(cpuRequestCores)
	row.CPURequestCoreSeconds = floatToString(cpuRequestCores * secondsPerHour)
	row.CPULimitCores = floatToString(cpuLimitCores)
	row.CPULimitCoreSeconds = floatToString(cpuLimitCores * secondsPerHour)
	row.MemoryUsageBytes = floatToString(memUsageBytes * secondsPerHour)
	row.MemoryRequestBytes = floatToString(memRequestBytes)
	row.MemoryRequestByteSeconds = floatToString(memRequestBytes * secondsPerHour)
	row.MemoryLimitBytes = floatToString(memLimitBytes)
	row.MemoryLimitByteSeconds = floatToString(memLimitBytes * secondsPerHour)
	row.DiskAllocatedSizeBytes = floatToString(diskAllocated * secondsPerHour)
	row.UptimeSeconds = floatToString(secondsPerHour)

	return &row
}

func averageVMSamples(samples []mappedValues) mappedValues {
	if len(samples) == 1 {
		return samples[0]
	}

	merged := mappedValues{}
	numericKeys := []string{
		"cpu_usage_mc", "cpu_request_mc", "cpu_limit_mc",
		"memory_usage_kib", "memory_request_kib", "memory_available_kib", "memory_limit_kib",
		"disk_allocated_bytes", "filesystem_used_bytes", "filesystem_capacity_bytes",
		"disk_read_iops", "disk_write_iops", "disk_read_bytes_per_sec", "disk_write_bytes_per_sec",
		"restart_count",
	}

	for key, val := range samples[0] {
		merged[key] = val
	}

	for _, numKey := range numericKeys {
		var sum float64
		var count int
		for _, sample := range samples {
			if v, ok := sample[numKey]; ok {
				if f, err := parseFloat(v); err == nil {
					sum += f
					count++
				}
			}
		}
		if count > 0 {
			merged[numKey] = floatToString(sum / float64(count))
		}
	}

	return merged
}

func stringValue(values mappedValues, key string) string {
	if v, ok := values[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func avgFloat(values mappedValues, key string) float64 {
	if v, ok := values[key]; ok {
		if f, err := parseFloat(v); err == nil {
			return f
		}
	}
	return 0
}

func parseFloat(value interface{}) (float64, error) {
	switch v := value.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return 0, nil
		}
		return strconv.ParseFloat(v, 64)
	case float64:
		return v, nil
	default:
		return 0, fmt.Errorf("unsupported type %T", value)
	}
}
