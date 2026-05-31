//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import "github.com/prometheus/common/model"

// rosVMGpuQueries collects DCGM GPU metrics for OpenShift Virtualization workloads.
// Metrics are scoped to virt-launcher pods and merged into ros-openshift-vm-usage CSV rows
// by VMI name (see mergeVMGPUIntoResults).
var rosVMGpuQueries = &querys{
	query{
		Name:        "vm-ros-gpu-utilization-avg",
		QueryString: QueryMap["ros:vm_gpu_utilization_avg"],
		MetricKey: staticFields{
			"exported_pod":       "exported_pod",
			"namespace":          "exported_namespace",
			"gpu_model":          "modelName",
			"gpu_mig_profile":    "GPU_I_PROFILE",
			"gpu_uuid":           "UUID",
		},
		QueryValue: &saveQueryValue{ValName: "gpu_utilization_avg"},
		RowKey:     []model.LabelName{"exported_pod", "exported_namespace", "UUID"},
	},
	query{
		Name:        "vm-ros-gpu-utilization-max",
		QueryString: QueryMap["ros:vm_gpu_utilization_max"],
		MetricKey: staticFields{
			"exported_pod":    "exported_pod",
			"namespace":       "exported_namespace",
			"gpu_mig_profile": "GPU_I_PROFILE",
			"gpu_uuid":        "UUID",
		},
		QueryValue: &saveQueryValue{ValName: "gpu_utilization_max"},
		RowKey:     []model.LabelName{"exported_pod", "exported_namespace", "UUID"},
	},
	query{
		Name:        "vm-ros-gpu-fb-used-avg",
		QueryString: QueryMap["ros:vm_gpu_fb_used_avg"],
		MetricKey: staticFields{
			"exported_pod":    "exported_pod",
			"namespace":       "exported_namespace",
			"gpu_mig_profile": "GPU_I_PROFILE",
			"gpu_uuid":        "UUID",
		},
		QueryValue: &saveQueryValue{ValName: "gpu_fb_used_avg_mib"},
		RowKey:     []model.LabelName{"exported_pod", "exported_namespace", "UUID"},
	},
	query{
		Name:        "vm-ros-gpu-fb-used-max",
		QueryString: QueryMap["ros:vm_gpu_fb_used_max"],
		MetricKey: staticFields{
			"exported_pod":    "exported_pod",
			"namespace":       "exported_namespace",
			"gpu_mig_profile": "GPU_I_PROFILE",
			"gpu_uuid":        "UUID",
		},
		QueryValue: &saveQueryValue{ValName: "gpu_fb_used_max_mib"},
		RowKey:     []model.LabelName{"exported_pod", "exported_namespace", "UUID"},
	},
	query{
		Name:        "vm-ros-gpu-sm-active-avg",
		QueryString: QueryMap["ros:vm_gpu_sm_active_avg"],
		MetricKey: staticFields{
			"exported_pod":    "exported_pod",
			"namespace":       "exported_namespace",
			"gpu_mig_profile": "GPU_I_PROFILE",
			"gpu_uuid":        "UUID",
		},
		QueryValue: &saveQueryValue{ValName: "gpu_sm_active_avg"},
		RowKey:     []model.LabelName{"exported_pod", "exported_namespace", "UUID"},
	},
	query{
		Name:        "vm-ros-gpu-tensor-active-avg",
		QueryString: QueryMap["ros:vm_gpu_tensor_active_avg"],
		MetricKey: staticFields{
			"exported_pod":    "exported_pod",
			"namespace":       "exported_namespace",
			"gpu_mig_profile": "GPU_I_PROFILE",
			"gpu_uuid":        "UUID",
		},
		QueryValue: &saveQueryValue{ValName: "gpu_tensor_active_avg"},
		RowKey:     []model.LabelName{"exported_pod", "exported_namespace", "UUID"},
	},
	query{
		Name:        "vm-ros-gpu-dram-active-avg",
		QueryString: QueryMap["ros:vm_gpu_dram_active_avg"],
		MetricKey: staticFields{
			"exported_pod":    "exported_pod",
			"namespace":       "exported_namespace",
			"gpu_mig_profile": "GPU_I_PROFILE",
			"gpu_uuid":        "UUID",
		},
		QueryValue: &saveQueryValue{ValName: "gpu_dram_active_avg"},
		RowKey:     []model.LabelName{"exported_pod", "exported_namespace", "UUID"},
	},
	query{
		Name:        "vm-ros-gpu-max-slices",
		QueryString: QueryMap["ros:vm_gpu_max_slices"],
		MetricKey: staticFields{
			"exported_pod":    "exported_pod",
			"namespace":       "exported_namespace",
			"gpu_mig_profile": "GPU_I_PROFILE",
			"gpu_uuid":        "UUID",
		},
		QueryValue: &saveQueryValue{ValName: "gpu_max_slices"},
		RowKey:     []model.LabelName{"exported_pod", "exported_namespace", "UUID"},
	},
}
