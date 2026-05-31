//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import (
	"regexp"
	"strconv"
	"strings"
)

var virtLauncherPodHashSuffix = regexp.MustCompile(`-[a-z0-9]{5}$`)

// vmiNameFromVirtLauncherPod extracts the KubeVirt VMI name from a virt-launcher pod name.
// Pod names follow virt-launcher-<vmi-name>-<5char-hash>.
func vmiNameFromVirtLauncherPod(pod string) string {
	pod = strings.TrimPrefix(pod, "virt-launcher-")
	if loc := virtLauncherPodHashSuffix.FindStringIndex(pod); loc != nil && loc[0] > 0 {
		return pod[:loc[0]]
	}
	return pod
}

type vmGPUAggregate struct {
	uuids           map[string]struct{}
	gpuModel        string
	utilizationAvg  float64
	utilAvgN        int
	utilizationMax  float64
	fbUsedAvgMiB    float64
	fbAvgN          int
	fbUsedMaxMiB    float64
	smActiveAvg     float64
	smAvgN          int
	tensorActiveAvg float64
	tensorAvgN      int
	dramActiveAvg   float64
	dramAvgN        int
	migProfile      string
	maxSlices       float64
}

func mergeVMGPUIntoResults(vmResults mappedResults, gpuResults mappedResults) {
	if len(gpuResults) == 0 {
		return
	}

	byVM := make(map[string]*vmGPUAggregate)

	for _, val := range gpuResults {
		pod := stringValue(val, "exported_pod")
		if pod == "" {
			continue
		}
		ns := stringValue(val, "namespace")
		vmiName := vmiNameFromVirtLauncherPod(pod)
		if vmiName == "" || ns == "" {
			continue
		}
		key := ns + "\x00" + vmiName

		agg, ok := byVM[key]
		if !ok {
			agg = &vmGPUAggregate{uuids: make(map[string]struct{})}
			byVM[key] = agg
		}

		uuid := stringValue(val, "gpu_uuid")
		if uuid != "" {
			agg.uuids[uuid] = struct{}{}
		}

		if model := stringValue(val, "gpu_model"); model != "" && agg.gpuModel == "" {
			agg.gpuModel = model
		}

		if v := avgFloat(val, "gpu_utilization_avg"); v > 0 {
			agg.utilizationAvg += v
			agg.utilAvgN++
		}
		agg.utilizationMax = maxFloat64(agg.utilizationMax, avgFloat(val, "gpu_utilization_max"))
		if v := avgFloat(val, "gpu_fb_used_avg_mib"); v > 0 {
			agg.fbUsedAvgMiB += v
			agg.fbAvgN++
		}
		agg.fbUsedMaxMiB = maxFloat64(agg.fbUsedMaxMiB, avgFloat(val, "gpu_fb_used_max_mib"))
		if v := avgFloat(val, "gpu_sm_active_avg"); v > 0 {
			agg.smActiveAvg += v
			agg.smAvgN++
		}
		if v := avgFloat(val, "gpu_tensor_active_avg"); v > 0 {
			agg.tensorActiveAvg += v
			agg.tensorAvgN++
		}
		if v := avgFloat(val, "gpu_dram_active_avg"); v > 0 {
			agg.dramActiveAvg += v
			agg.dramAvgN++
		}

		profile := stringValue(val, "gpu_mig_profile")
		if profile != "" && profile != "None" && agg.migProfile == "" {
			agg.migProfile = profile
		}
		slices := avgFloat(val, "gpu_max_slices")
		if slices > agg.maxSlices {
			agg.maxSlices = slices
		}
	}

	for _, val := range vmResults {
		name := stringValue(val, "name")
		ns := stringValue(val, "namespace")
		if name == "" || ns == "" {
			continue
		}
		agg, ok := byVM[ns+"\x00"+name]
		if !ok || len(agg.uuids) == 0 {
			continue
		}
		applyVMGPUAggregate(val, agg)
	}
}

func applyVMGPUAggregate(val mappedValues, agg *vmGPUAggregate) {
	val["gpu_count"] = strconv.Itoa(len(agg.uuids))
	if agg.utilAvgN > 0 {
		agg.utilizationAvg /= float64(agg.utilAvgN)
	}
	if agg.fbAvgN > 0 {
		agg.fbUsedAvgMiB /= float64(agg.fbAvgN)
	}
	if agg.smAvgN > 0 {
		agg.smActiveAvg /= float64(agg.smAvgN)
	}
	if agg.tensorAvgN > 0 {
		agg.tensorActiveAvg /= float64(agg.tensorAvgN)
	}
	if agg.dramAvgN > 0 {
		agg.dramActiveAvg /= float64(agg.dramAvgN)
	}
	val["gpu_model"] = agg.gpuModel
	val["gpu_utilization_avg"] = floatToString(agg.utilizationAvg)
	val["gpu_utilization_max"] = floatToString(agg.utilizationMax)
	val["gpu_fb_used_avg_mib"] = floatToString(agg.fbUsedAvgMiB)
	val["gpu_fb_used_max_mib"] = floatToString(agg.fbUsedMaxMiB)
	val["gpu_sm_active_avg"] = floatToString(agg.smActiveAvg)
	val["gpu_tensor_active_avg"] = floatToString(agg.tensorActiveAvg)
	val["gpu_dram_active_avg"] = floatToString(agg.dramActiveAvg)
	val["gpu_mig_profile"] = agg.migProfile
	val["gpu_max_slices"] = floatToString(agg.maxSlices)
}

func maxFloat64(a, b float64) float64 {
	if b > a {
		return b
	}
	return a
}
