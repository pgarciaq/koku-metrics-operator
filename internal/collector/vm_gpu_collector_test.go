//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import (
	"strings"
	"testing"
)

func TestVmiNameForGPURow_PrefersPodLabels(t *testing.T) {
	val := mappedValues{
		"exported_pod": "virt-launcher-my-vm-abc12",
		"namespace":    "ml",
	}
	podVMI := map[string]string{"ml\x00virt-launcher-my-vm-abc12": "exact-vmi-name"}
	if got := vmiNameForGPURow(val, podVMI); got != "exact-vmi-name" {
		t.Fatalf("vmiNameForGPURow = %q, want exact-vmi-name", got)
	}
}

func TestVmiNameFromVirtLauncherPod(t *testing.T) {
	tests := []struct {
		pod  string
		want string
	}{
		{"virt-launcher-my-vm-abc12", "my-vm"},
		{"virt-launcher-gpu-idle-vm-x9y8z", "gpu-idle-vm"},
		{"virt-launcher-complex-name-here-12345", "complex-name-here"},
		{"not-virt-launcher", "not-virt-launcher"},
	}
	for _, tt := range tests {
		if got := vmiNameFromVirtLauncherPod(tt.pod); got != tt.want {
			t.Errorf("vmiNameFromVirtLauncherPod(%q) = %q, want %q", tt.pod, got, tt.want)
		}
	}
}

func TestRosVMGpuQueriesUseVirtLauncherFilter(t *testing.T) {
	for _, q := range *rosVMGpuQueries {
		if !strings.Contains(q.QueryString, `virt-launcher-`) {
			t.Errorf("query %q missing virt-launcher pod filter", q.Name)
		}
	}
}

func TestMergeVMGPUIntoResults(t *testing.T) {
	vmResults := mappedResults{
		"k1": mappedValues{
			"name":      "my-vm",
			"namespace": "ml",
		},
	}
	gpuResults := mappedResults{
		"g1": mappedValues{
			"exported_pod":         "virt-launcher-my-vm-abc12",
			"namespace":            "ml",
			"gpu_uuid":             "GPU-1",
			"gpu_model":            "NVIDIA A100",
			"gpu_utilization_avg":  "0.05",
			"gpu_utilization_max":  "0.10",
			"gpu_fb_used_avg_mib":  "1024",
			"gpu_fb_used_max_mib":  "2048",
			"gpu_sm_active_avg":    "0.04",
			"gpu_tensor_active_avg": "0.02",
			"gpu_dram_active_avg":  "0.01",
			"gpu_mig_profile":      "3g.20gb",
			"gpu_max_slices":       "7",
		},
		"g2": mappedValues{
			"exported_pod":        "virt-launcher-my-vm-abc12",
			"namespace":           "ml",
			"gpu_uuid":            "GPU-2",
			"gpu_utilization_avg": "0.15",
		},
	}
	mergeVMGPUIntoResults(vmResults, gpuResults, nil)

	val := vmResults["k1"]
	if stringValue(val, "gpu_count") != "2" {
		t.Fatalf("gpu_count = %q, want 2", stringValue(val, "gpu_count"))
	}
	if stringValue(val, "gpu_model") != "NVIDIA A100" {
		t.Fatalf("gpu_model = %q", stringValue(val, "gpu_model"))
	}
	if stringValue(val, "gpu_mig_profile") != "3g.20gb" {
		t.Fatalf("gpu_mig_profile = %q", stringValue(val, "gpu_mig_profile"))
	}
}

func TestROSVMRowCSVIncludesGPUColumns(t *testing.T) {
	header := rosVMRow{}.csvHeader()
	if len(header) != 37 {
		t.Fatalf("expected 31 columns, got %d", len(header))
	}
	if header[20] != "gpu_count" || header[30] != "gpu_max_slices" {
		t.Fatalf("unexpected GPU column positions: %v", header[20:])
	}
	row := rosVMRow{GPUCount: "1", GPUModel: "NVIDIA T4"}
	csv := row.csvRow()
	if csv[20] != "1" || csv[21] != "NVIDIA T4" {
		t.Fatalf("GPU columns not mapped in csv row: %v", csv[20:22])
	}
}
