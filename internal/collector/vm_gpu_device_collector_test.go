//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"

	"github.com/project-koku/koku-metrics-operator/internal/dirconfig"
	"github.com/project-koku/koku-metrics-operator/internal/testutils"
)

func TestVMGPUDeviceRow_CSVHeader(t *testing.T) {
	want := []string{
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
	got := rosVMGPUDeviceRow{}.csvHeader()
	if len(got) != 14 {
		t.Fatalf("expected 14 columns, got %d: %v", len(got), got)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("csvHeader = %v, want %v", got, want)
	}
}

func TestVMGPUDeviceRow_CSVValues(t *testing.T) {
	row := rosVMGPUDeviceRow{
		IntervalStart:   "2026-05-01 12:00:00",
		Namespace:       "ml-training",
		VMName:          "multi-gpu-vm",
		GPUUUID:         "GPU-aaa-111",
		GPUModel:        "NVIDIA A100-SXM4-80GB",
		UtilizationAvg:  "0.08",
		UtilizationMax:  "0.12",
		FBUsedAvgMiB:    "4096",
		FBUsedMaxMiB:    "8192",
		SMActiveAvg:     "0.10",
		TensorActiveAvg: "0.08",
		DRAMActiveAvg:   "0.06",
		MIGProfile:      "3g.20gb",
		MaxSlices:       "7",
	}
	got := row.csvRow()
	if len(got) != 14 {
		t.Fatalf("csvRow length = %d, want 14", len(got))
	}
	if got[3] != "GPU-aaa-111" || got[4] != "NVIDIA A100-SXM4-80GB" || got[12] != "3g.20gb" {
		t.Fatalf("unexpected csvRow: %v", got)
	}
	if row.string() != strings.Join(got, ",") {
		t.Fatalf("string() mismatch with csvRow join")
	}
}

func TestVMGPUDeviceFilenamePrefix(t *testing.T) {
	if !strings.HasPrefix(rosVMGPUDeviceFilePrefix, "ros-openshift-vm-gpu-device-") {
		t.Fatalf("prefix = %q", rosVMGPUDeviceFilePrefix)
	}
	yearMonth := "202605"
	name := rosVMGPUDeviceFilePrefix + yearMonth + ".csv"
	if !strings.HasPrefix(name, "ros-openshift-vm-gpu-device-") {
		t.Fatalf("report name = %q", name)
	}
	if !strings.HasSuffix(name, ".csv") {
		t.Fatalf("report name = %q", name)
	}
}

func TestWriteVMGPUDeviceReport_EmptyData(t *testing.T) {
	log := testutils.ZapLogger(true)
	dir := t.TempDir()
	dirCfg := &dirconfig.DirectoryConfig{Reports: dirconfig.Directory{Path: dir}}
	ts := &promv1.Range{Start: time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)}
	c := &PrometheusCollector{TimeSeries: ts}

	if err := writeVMGPUDeviceReport(log, c, dirCfg, "202605", nil, nil); err != nil {
		t.Fatalf("writeVMGPUDeviceReport: %v", err)
	}
	path := filepath.Join(dir, rosVMGPUDeviceFilePrefix+"202605.csv")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected no device CSV for empty gpuResults, stat err=%v", err)
	}

	// Rows without gpu_uuid are skipped; file must not be created.
	gpuResults := mappedResults{
		"k1": mappedValues{
			"namespace":    "ml",
			"exported_pod": "virt-launcher-no-uuid-abc12",
			"gpu_model":    "NVIDIA T4",
		},
	}
	if err := writeVMGPUDeviceReport(log, c, dirCfg, "202605", gpuResults, nil); err != nil {
		t.Fatalf("writeVMGPUDeviceReport: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected no device CSV when gpu_uuid missing, stat err=%v", err)
	}
}

func TestWriteVMGPUDeviceReport_ValidData(t *testing.T) {
	log := testutils.ZapLogger(true)
	dir := t.TempDir()
	dirCfg := &dirconfig.DirectoryConfig{Reports: dirconfig.Directory{Path: dir}}
	start := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	c := &PrometheusCollector{TimeSeries: &promv1.Range{Start: start}}

	gpuResults := mappedResults{
		"g1": mappedValues{
			"namespace":             "ml-training",
			"exported_pod":          "virt-launcher-multi-gpu-vm-x9y8z",
			"gpu_uuid":              "GPU-aaa-111",
			"gpu_model":             "NVIDIA A100-SXM4-80GB",
			"gpu_utilization_avg":   "0.08",
			"gpu_utilization_max":   "0.12",
			"gpu_fb_used_avg_mib":   "4096",
			"gpu_fb_used_max_mib":   "8192",
			"gpu_sm_active_avg":     "0.10",
			"gpu_tensor_active_avg": "0.08",
			"gpu_dram_active_avg":   "0.06",
			"gpu_mig_profile":       "3g.20gb",
			"gpu_max_slices":        "7",
		},
	}
	podVMI := map[string]string{
		"ml-training\x00virt-launcher-multi-gpu-vm-x9y8z": "multi-gpu-vm",
	}

	if err := writeVMGPUDeviceReport(log, c, dirCfg, "202605", gpuResults, podVMI); err != nil {
		t.Fatalf("writeVMGPUDeviceReport: %v", err)
	}

	path := filepath.Join(dir, rosVMGPUDeviceFilePrefix+"202605.csv")
	records, err := readGPUDeviceCSV(path)
	if err != nil {
		t.Fatalf("read device CSV: %v", err)
	}
	if len(records) < 2 {
		t.Fatalf("expected header + data rows, got %d records", len(records))
	}
	header := records[0]
	if len(header) != 14 {
		t.Fatalf("header columns = %d, want 14: %v", len(header), header)
	}
	if strings.Join(header, ",") != strings.Join(rosVMGPUDeviceRow{}.csvHeader(), ",") {
		t.Fatalf("header = %v", header)
	}
	data := records[1]
	if data[1] != "ml-training" || data[2] != "multi-gpu-vm" || data[3] != "GPU-aaa-111" {
		t.Fatalf("unexpected data row: %v", data)
	}
	if data[0] != start.Format("2006-01-02 15:04:05") {
		t.Fatalf("interval_start = %q, want %q", data[0], start.Format("2006-01-02 15:04:05"))
	}
}

func readGPUDeviceCSV(path string) ([][]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return csv.NewReader(f).ReadAll()
}
