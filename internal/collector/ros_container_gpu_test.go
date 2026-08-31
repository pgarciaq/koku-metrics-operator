//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import (
	"testing"

	"github.com/prometheus/common/model"
)

func TestAttachRosContainerGPUs_TwoUUIDsOnOneContainer(t *testing.T) {
	kubeKey := "deploy-abc,main,production"
	results := mappedResults{
		kubeKey: mappedValues{
			"container_name":             "main",
			"pod":                        "deploy-abc",
			"namespace":                  "production",
			"node":                       "node-1",
			"workload":                   "trainer",
			"cpu-usage-container-avg":    "0.500000",
			"memory-usage-container-avg": "1024.000000",
		},
		"GPU-aaa,gpu-workload,gpu-pod,gpu-test": mappedValues{
			"container":                          "main",
			"pod":                                "deploy-abc",
			"namespace":                          "production",
			"gpu_uuid":                           "GPU-aaa",
			"accelerator_model_name":             "Tesla V100",
			"accelerator-frame-buffer-usage-avg": "100.000000",
			"node":                               "gpu-host-should-not-win",
		},
		"GPU-bbb,gpu-workload,gpu-pod,gpu-test": mappedValues{
			"container_name":                     "main",
			"pod":                                "deploy-abc",
			"namespace":                          "production",
			"gpu_uuid":                           "GPU-bbb",
			"accelerator_model_name":             "Tesla V100",
			"accelerator-frame-buffer-usage-avg": "200.000000",
			"node":                               "gpu-host-should-not-win",
		},
	}

	out := attachRosContainerGPUs(results)
	if len(out) != 2 {
		t.Fatalf("got %d rows, want 2 (one per UUID)", len(out))
	}

	byUUID := map[string]mappedValues{}
	for _, val := range out {
		uuid := stringValue(val, "gpu_uuid")
		if uuid == "" {
			t.Fatalf("expected gpu_uuid on every exploded row, got %#v", val)
		}
		byUUID[uuid] = val
	}
	if _, ok := byUUID["GPU-aaa"]; !ok {
		t.Fatal("missing GPU-aaa row")
	}
	if _, ok := byUUID["GPU-bbb"]; !ok {
		t.Fatal("missing GPU-bbb row")
	}

	for uuid, val := range byUUID {
		if stringValue(val, "container_name") != "main" {
			t.Errorf("%s container_name = %q, want main", uuid, stringValue(val, "container_name"))
		}
		if stringValue(val, "pod") != "deploy-abc" {
			t.Errorf("%s pod = %q, want deploy-abc", uuid, stringValue(val, "pod"))
		}
		if stringValue(val, "namespace") != "production" {
			t.Errorf("%s namespace = %q, want production", uuid, stringValue(val, "namespace"))
		}
		if stringValue(val, "workload") != "trainer" {
			t.Errorf("%s workload = %q, want trainer", uuid, stringValue(val, "workload"))
		}
		if stringValue(val, "cpu-usage-container-avg") != "0.500000" {
			t.Errorf("%s cpu-usage-container-avg = %q, want 0.500000", uuid, stringValue(val, "cpu-usage-container-avg"))
		}
		if stringValue(val, "node") != "node-1" {
			t.Errorf("%s node = %q, want kube node-1 (do not overlay DCGM Hostname)", uuid, stringValue(val, "node"))
		}
	}
	if stringValue(byUUID["GPU-aaa"], "accelerator-frame-buffer-usage-avg") != "100.000000" {
		t.Errorf("GPU-aaa FB avg = %q, want 100.000000", stringValue(byUUID["GPU-aaa"], "accelerator-frame-buffer-usage-avg"))
	}
	if stringValue(byUUID["GPU-bbb"], "accelerator-frame-buffer-usage-avg") != "200.000000" {
		t.Errorf("GPU-bbb FB avg = %q, want 200.000000", stringValue(byUUID["GPU-bbb"], "accelerator-frame-buffer-usage-avg"))
	}
}

func TestAttachRosContainerGPUs_NoGPUUnchanged(t *testing.T) {
	results := mappedResults{
		"cpu-only": mappedValues{
			"container_name":          "web",
			"pod":                     "web-1",
			"namespace":               "default",
			"cpu-usage-container-avg": "0.100000",
		},
	}
	out := attachRosContainerGPUs(results)
	if len(out) != 1 {
		t.Fatalf("got %d rows, want 1", len(out))
	}
	var row mappedValues
	for _, val := range out {
		row = val
	}
	if stringValue(row, "gpu_uuid") != "" {
		t.Errorf("gpu_uuid = %q, want empty", stringValue(row, "gpu_uuid"))
	}
	if stringValue(row, "cpu-usage-container-avg") != "0.100000" {
		t.Errorf("cpu-usage-container-avg = %q, want 0.100000", stringValue(row, "cpu-usage-container-avg"))
	}
}

func TestAttachRosContainerGPUs_UnmatchedDCGMDropped(t *testing.T) {
	results := mappedResults{
		"cpu-only": mappedValues{
			"container_name": "web",
			"pod":            "web-1",
			"namespace":      "default",
		},
		"orphan-dcgm": mappedValues{
			"container_name": "gpu-workload",
			"pod":            "gpu-pod-123",
			"namespace":      "gpu-test",
			"gpu_uuid":       "GPU-orphan",
		},
	}
	out := attachRosContainerGPUs(results)
	if len(out) != 1 {
		t.Fatalf("got %d rows, want 1 (orphan DCGM dropped)", len(out))
	}
	for _, val := range out {
		if stringValue(val, "gpu_uuid") != "" {
			t.Errorf("unexpected gpu_uuid %q on remaining row", stringValue(val, "gpu_uuid"))
		}
		if stringValue(val, "container_name") != "web" {
			t.Errorf("remaining row container_name = %q, want web", stringValue(val, "container_name"))
		}
	}
}

func TestAttachRosContainerGPUs_MatchesExportedContainerFallback(t *testing.T) {
	results := mappedResults{
		"kube": mappedValues{
			"container_name":          "gpu-workload",
			"pod":                     "gpu-pod-123",
			"namespace":               "gpu-test",
			"cpu-usage-container-avg": "0.250000",
		},
		"dcgm": mappedValues{
			"container": "gpu-workload",
			"pod":       "gpu-pod-123",
			"namespace": "gpu-test",
			"gpu_uuid":  "GPU-1a2b",
		},
	}
	out := attachRosContainerGPUs(results)
	if len(out) != 1 {
		t.Fatalf("got %d rows, want 1 joined row", len(out))
	}
	for _, val := range out {
		if stringValue(val, "gpu_uuid") != "GPU-1a2b" {
			t.Errorf("gpu_uuid = %q, want GPU-1a2b", stringValue(val, "gpu_uuid"))
		}
		if stringValue(val, "container_name") != "gpu-workload" {
			t.Errorf("container_name = %q, want gpu-workload", stringValue(val, "container_name"))
		}
		if stringValue(val, "cpu-usage-container-avg") != "0.250000" {
			t.Errorf("cpu-usage-container-avg = %q, want 0.250000", stringValue(val, "cpu-usage-container-avg"))
		}
	}
}

func TestIterateVectorThenAttachRosContainerGPUs(t *testing.T) {
	cpuQuery := query{
		Name:      "cpu-usage-container-avg",
		MetricKey: staticFields{"container_name": "container", "pod": "pod", "namespace": "namespace", "node": "node"},
		QueryValue: &saveQueryValue{
			ValName: "cpu-usage-container-avg",
		},
		RowKey: []model.LabelName{"container", "pod", "namespace"},
	}
	dcgmQuery := query{
		Name: "accelerator-frame-buffer-usage-avg",
		MetricKey: staticFields{
			"accelerator_model_name": "modelName",
			"gpu_uuid":               "UUID",
			"container_name":         "exported_container",
			"namespace":              "exported_namespace",
			"pod":                    "exported_pod",
			"node":                   "Hostname",
		},
		QueryValue: &saveQueryValue{
			ValName: "accelerator-frame-buffer-usage-avg",
		},
		RowKey: []model.LabelName{"exported_container", "exported_pod", "exported_namespace", "UUID"},
	}

	results := mappedResults{}
	results.iterateVector(model.Vector{
		{
			Metric: model.Metric{
				"container": "gpu-workload",
				"pod":       "gpu-pod-123",
				"namespace": "gpu-test",
				"node":      "node-1",
			},
			Value: model.SampleValue(0.25),
		},
	}, cpuQuery)
	results.iterateVector(model.Vector{
		{
			Metric: model.Metric{
				"exported_container": "gpu-workload",
				"exported_pod":       "gpu-pod-123",
				"exported_namespace": "gpu-test",
				"UUID":               "GPU-aaa",
				"modelName":          "Tesla V100",
				"Hostname":           "gpu-host",
			},
			Value: model.SampleValue(100),
		},
		{
			Metric: model.Metric{
				"exported_container": "gpu-workload",
				"exported_pod":       "gpu-pod-123",
				"exported_namespace": "gpu-test",
				"UUID":               "GPU-bbb",
				"modelName":          "Tesla V100",
				"Hostname":           "gpu-host",
			},
			Value: model.SampleValue(200),
		},
	}, dcgmQuery)

	if len(results) != 3 {
		t.Fatalf("before attach: got %d mapped keys, want 1 kube + 2 DCGM", len(results))
	}

	out := attachRosContainerGPUs(results)
	if len(out) != 2 {
		t.Fatalf("after attach: got %d rows, want 2", len(out))
	}

	byUUID := map[string]mappedValues{}
	for _, val := range out {
		byUUID[stringValue(val, "gpu_uuid")] = val
	}
	if stringValue(byUUID["GPU-aaa"], "cpu-usage-container-avg") != "0.250000" {
		t.Errorf("GPU-aaa cpu = %q", stringValue(byUUID["GPU-aaa"], "cpu-usage-container-avg"))
	}
	if stringValue(byUUID["GPU-bbb"], "accelerator-frame-buffer-usage-avg") != "200.000000" {
		t.Errorf("GPU-bbb FB = %q", stringValue(byUUID["GPU-bbb"], "accelerator-frame-buffer-usage-avg"))
	}
	if stringValue(byUUID["GPU-aaa"], "node") != "node-1" {
		t.Errorf("GPU-aaa node = %q, want node-1", stringValue(byUUID["GPU-aaa"], "node"))
	}
	if stringValue(byUUID["GPU-aaa"], "container_name") != "gpu-workload" {
		t.Errorf("GPU-aaa container_name = %q", stringValue(byUUID["GPU-aaa"], "container_name"))
	}
}
