//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import (
	"sort"
)

// rosDCGMContainerMetricKey maps DCGM series labels onto rosContainerRow fields.
// container_name must come from exported_container so mapstructure fills ContainerName.
var rosDCGMContainerMetricKey = staticFields{
	"accelerator_model_name":   "modelName",
	"gpu_uuid":                 "UUID",
	"container_name":           "exported_container",
	"namespace":                "exported_namespace",
	"pod":                      "exported_pod",
	"node":                     "Hostname",
	"accelerator_profile_name": "GPU_I_PROFILE",
}

// rosDCGMOverlayKeys are GPU fields copied from a DCGM row onto a cloned kube row.
// node is intentionally omitted so kube node identity wins over DCGM Hostname.
var rosDCGMOverlayKeys = []string{
	"accelerator_model_name",
	"accelerator_profile_name",
	"gpu_uuid",
	"accelerator-frame-buffer-usage-min",
	"accelerator-frame-buffer-usage-max",
	"accelerator-frame-buffer-usage-avg",
	"tensor-pipe-active-min",
	"tensor-pipe-active-max",
	"tensor-pipe-active-avg",
	"dram-active-min",
	"dram-active-max",
	"dram-active-avg",
	"sm-active-min",
	"sm-active-max",
	"sm-active-avg",
}

func rosContainerJoinKey(val mappedValues) string {
	ns := stringValue(val, "namespace")
	pod := stringValue(val, "pod")
	container := stringValue(val, "container_name")
	if container == "" {
		container = stringValue(val, "container")
	}
	if ns == "" || pod == "" || container == "" {
		return ""
	}
	return ns + "\x00" + pod + "\x00" + container
}

func cloneMappedValues(src mappedValues) mappedValues {
	dst := make(mappedValues, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func overlayRosGPUFields(dst, gpu mappedValues) {
	for _, key := range rosDCGMOverlayKeys {
		if v, ok := gpu[key]; ok {
			dst[key] = v
		}
	}
}

// attachRosContainerGPUs clones each kube ROS container row once per matching
// DCGM UUID. Unmatched DCGM rows (no kube CPU/mem identity) are dropped.
func attachRosContainerGPUs(results mappedResults) mappedResults {
	kube := mappedResults{}
	dcgmByIdentity := map[string][]mappedValues{}

	for key, val := range results {
		if stringValue(val, "gpu_uuid") != "" {
			if id := rosContainerJoinKey(val); id != "" {
				dcgmByIdentity[id] = append(dcgmByIdentity[id], val)
			}
			continue
		}
		kube[key] = val
	}

	out := mappedResults{}
	for key, val := range kube {
		gpus := dcgmByIdentity[rosContainerJoinKey(val)]
		if len(gpus) == 0 {
			out[key] = val
			continue
		}
		sort.Slice(gpus, func(i, j int) bool {
			return stringValue(gpus[i], "gpu_uuid") < stringValue(gpus[j], "gpu_uuid")
		})
		for _, gpu := range gpus {
			cloned := cloneMappedValues(val)
			overlayRosGPUFields(cloned, gpu)
			uuid := stringValue(gpu, "gpu_uuid")
			out[key+"\x00"+uuid] = cloned
		}
	}
	return out
}

func countRosGPURows(results mappedResults) int {
	n := 0
	for _, val := range results {
		if stringValue(val, "gpu_uuid") != "" {
			n++
		}
	}
	return n
}
