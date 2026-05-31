//
// Copyright 2021 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import "github.com/prometheus/common/model"

var (
	QueryMap = map[string]string{
		"cost:node_allocatable_cpu_cores":    "kube_node_status_allocatable{resource='cpu'} * on(node) group_left(provider_id) max by (node, provider_id) (kube_node_info) ",
		"cost:node_allocatable_memory_bytes": "kube_node_status_allocatable{resource='memory'} * on(node) group_left(provider_id) max by (node, provider_id) (kube_node_info)",
		"cost:node_capacity_cpu_cores":       "kube_node_status_capacity{resource='cpu'} * on(node) group_left(provider_id) max by (node, provider_id) (kube_node_info)",
		"cost:node_capacity_memory_bytes":    "kube_node_status_capacity{resource='memory'} * on(node) group_left(provider_id) max by (node, provider_id) (kube_node_info)",

		"cost:persistentvolume_pod_info":            "kube_pod_spec_volumes_persistentvolumeclaims_info * on(persistentvolumeclaim, namespace) group_left(volumename) max by(namespace, persistentvolumeclaim, volumename) (kube_persistentvolumeclaim_info{volumename != ''})",
		"cost:persistentvolumeclaim_capacity_bytes": "kube_persistentvolume_capacity_bytes{persistentvolume != ''}",
		"cost:persistentvolumeclaim_request_bytes":  "kube_persistentvolumeclaim_resource_requests_storage_bytes * on(persistentvolumeclaim, namespace) group_left(volumename) max by(namespace, persistentvolumeclaim, volumename) (kube_persistentvolumeclaim_info{volumename != ''})",
		"cost:persistentvolumeclaim_usage_bytes":    "kubelet_volume_stats_used_bytes * on(persistentvolumeclaim, namespace) group_left(volumename) max by(namespace, persistentvolumeclaim, volumename) (kube_persistentvolumeclaim_info{volumename != ''})",
		"cost:persistentvolume_labels":              "kube_persistentvolume_labels * on(persistentvolume, namespace) group_left(storageclass, csi_driver, csi_volume_handle) max by(namespace, persistentvolume, storageclass, csi_driver, csi_volume_handle) (kube_persistentvolume_info)",
		"cost:persistentvolumeclaim_labels":         "kube_persistentvolumeclaim_labels * on(persistentvolumeclaim, namespace) group_left(volumename) max by(namespace, persistentvolumeclaim, volumename) (kube_persistentvolumeclaim_info{volumename != ''})",

		"cost:pod_limit_cpu_cores":      "sum by (pod, namespace, node) (kube_pod_container_resource_limits{pod!='', namespace!='', node!='', resource='cpu'} * on(pod, namespace) group_left max by (pod, namespace) (kube_pod_status_phase{phase='Running'}))",
		"cost:pod_request_cpu_cores":    "sum by (pod, namespace, node) (kube_pod_container_resource_requests{pod!='', namespace!='', node!='', resource='cpu'} * on(pod, namespace) group_left max by (pod, namespace) (kube_pod_status_phase{phase='Running'}))",
		"cost:pod_usage_cpu_cores":      "sum by (pod, namespace, node) (rate(container_cpu_usage_seconds_total{container!='', container!='POD', pod!='', namespace!='', node!=''}[5m]))",
		"cost:pod_limit_memory_bytes":   "sum by (pod, namespace, node) (kube_pod_container_resource_limits{pod!='', namespace!='', node!='', resource='memory'} * on(pod, namespace) group_left max by (pod, namespace) (kube_pod_status_phase{phase='Running'}))",
		"cost:pod_request_memory_bytes": "sum by (pod, namespace, node) (kube_pod_container_resource_requests{pod!='', namespace!='', node!='', resource='memory'} * on(pod, namespace) group_left max by (pod, namespace) (kube_pod_status_phase{phase='Running'}))",
		"cost:pod_usage_memory_bytes":   "sum by (pod, namespace, node) (container_memory_usage_bytes{container!='', container!='POD', pod!='', namespace!='', node!=''})",
		"cost:pod_labels":               "kube_pod_labels{namespace!='',pod!=''} * on(pod, namespace) group_left max by (pod, namespace) (kube_pod_status_phase{phase='Running'})",

		// virtual machine metrics queries
		"cost:vm_cpu_limit_cores":           "sum by (name, namespace) (kubevirt_vm_resource_limits{name!='', namespace!='', resource='cpu'}) * on (name, namespace) group_left max by (name, namespace) (kubevirt_vmi_info{phase='running'})",
		"cost:vm_cpu_request_cores":         "sum by (name, namespace) (kubevirt_vm_resource_requests{name!='', namespace!='', resource='cpu', unit='cores'}) * on (name, namespace) group_left max by (name, namespace) (kubevirt_vmi_info{phase='running'})",
		"cost:vm_cpu_request_sockets":       "sum by (name, namespace) (kubevirt_vm_resource_requests{name!='', namespace!='', resource='cpu', unit='sockets'}) * on (name, namespace) group_left max by (name, namespace) (kubevirt_vmi_info{phase='running'})",
		"cost:vm_cpu_request_threads":       "sum by (name, namespace) (kubevirt_vm_resource_requests{name!='', namespace!='', resource='cpu', unit='threads'}) * on (name, namespace) group_left max by (name, namespace) (kubevirt_vmi_info{phase='running'})",
		"cost:vm_cpu_usage":                 "sum by (name, namespace) (rate(kubevirt_vmi_cpu_usage_seconds_total{name!='', namespace!=''}[5m])) * on (name, namespace) group_left max by (name, namespace) (kubevirt_vmi_info{phase='running'})",
		"cost:vm_memory_limit_bytes":        "sum by (name, namespace) (kubevirt_vm_resource_limits{name!='', namespace!='', resource='memory'}) * on (name, namespace) group_left max by (name, namespace) (kubevirt_vmi_info{phase='running'})",
		"cost:vm_memory_request_bytes":      "sum by (name, namespace) (kubevirt_vm_resource_requests{name!='', namespace!='', resource='memory'}) * on (name, namespace) group_left max by (name, namespace) (kubevirt_vmi_info{phase='running'})",
		"cost:vm_memory_usage_bytes":        "sum by (name, namespace) (sum_over_time(kubevirt_vmi_memory_used_bytes{name!='', namespace!=''}[5m])) * on (name, namespace) group_left max by (name, namespace) (kubevirt_vmi_info{phase='running'})",
		"cost:vm_info":                      "sum by (name, namespace, node, os, instance_type, guest_os_name, guest_os_version_id, guest_os_arch) (kubevirt_vmi_info{phase='running'}) * on(node) group_left(provider_id) max by (node, provider_id) (kube_node_info)",
		"cost:vm_disk_allocated_size_bytes": "sum by (name, namespace, device, persistentvolumeclaim, volume_mode) (kubevirt_vm_disk_allocated_size_bytes{name!='', namespace!=''}) * on (name, namespace) group_left max by (name, namespace) (kubevirt_vmi_info{phase='running'})",
		"cost:vm_labels":                    "kubevirt_vm_labels{name!='', namespace!=''}",

		// OpenShift Virtualization ROS metrics (15-minute instant queries; running VMIs only)
		"ros:vm_cpu_usage_mc":            "sum by (name, namespace, node) (rate(kubevirt_vmi_cpu_usage_seconds_total{name!='', namespace!=''}[5m]) * 1000) * on (name, namespace, node) group_left() max by (name, namespace, node) (kubevirt_vmi_info{phase='running'})",
		"ros:vm_cpu_request_mc":          "sum by (name, namespace, node) (kubevirt_vmi_resource_requests{resource='cpu', name!='', namespace!=''} * 1000) * on (name, namespace, node) group_left() max by (name, namespace, node) (kubevirt_vmi_info{phase='running'})",
		"ros:vm_cpu_limit_mc":            "sum by (name, namespace, node) (kubevirt_vmi_resource_limits{resource='cpu', name!='', namespace!=''} * 1000) * on (name, namespace, node) group_left() max by (name, namespace, node) (kubevirt_vmi_info{phase='running'})",
		"ros:vm_memory_usage_kib":        "sum by (name, namespace, node) (kubevirt_vmi_memory_resident_bytes{name!='', namespace!=''} / 1024) * on (name, namespace, node) group_left() max by (name, namespace, node) (kubevirt_vmi_info{phase='running'})",
		"ros:vm_memory_request_kib":      "sum by (name, namespace, node) (kubevirt_vmi_resource_requests{resource='memory', name!='', namespace!=''} / 1024) * on (name, namespace, node) group_left() max by (name, namespace, node) (kubevirt_vmi_info{phase='running'})",
		"ros:vm_memory_available_kib":    "sum by (name, namespace, node) (kubevirt_vmi_memory_available_bytes{name!='', namespace!=''} / 1024) * on (name, namespace, node) group_left() max by (name, namespace, node) (kubevirt_vmi_info{phase='running'})",
		"ros:vm_disk_allocated_bytes":    "sum by (name, namespace, node) (kubevirt_vmi_storage_disk_capacity_bytes{name!='', namespace!=''}) * on (name, namespace, node) group_left() max by (name, namespace, node) (kubevirt_vmi_info{phase='running'})",
		"ros:vm_filesystem_used_bytes":   "sum by (name, namespace, node) (kubevirt_vmi_filesystem_used_bytes{name!='', namespace!=''}) * on (name, namespace, node) group_left() max by (name, namespace, node) (kubevirt_vmi_info{phase='running'})",
		"ros:vm_filesystem_capacity_bytes": "sum by (name, namespace, node) (kubevirt_vmi_filesystem_capacity_bytes{name!='', namespace!=''}) * on (name, namespace, node) group_left() max by (name, namespace, node) (kubevirt_vmi_info{phase='running'})",
		"ros:vm_disk_read_iops":          "sum by (name, namespace, node) (rate(kubevirt_vmi_storage_read_times_total{name!='', namespace!=''}[5m])) * on (name, namespace, node) group_left() max by (name, namespace, node) (kubevirt_vmi_info{phase='running'})",
		"ros:vm_disk_write_iops":         "sum by (name, namespace, node) (rate(kubevirt_vmi_storage_write_times_total{name!='', namespace!=''}[5m])) * on (name, namespace, node) group_left() max by (name, namespace, node) (kubevirt_vmi_info{phase='running'})",
		"ros:vm_disk_read_bytes_per_sec": "sum by (name, namespace, node) (rate(kubevirt_vmi_storage_read_traffic_bytes_total{name!='', namespace!=''}[5m])) * on (name, namespace, node) group_left() max by (name, namespace, node) (kubevirt_vmi_info{phase='running'})",
		"ros:vm_disk_write_bytes_per_sec": "sum by (name, namespace, node) (rate(kubevirt_vmi_storage_write_traffic_bytes_total{name!='', namespace!=''}[5m])) * on (name, namespace, node) group_left() max by (name, namespace, node) (kubevirt_vmi_info{phase='running'})",
		"ros:vm_info":                    "max by (name, namespace, node, os) (kubevirt_vmi_info{phase='running', name!='', namespace!=''})",

		// cost NVIDIA GPU metrics queries, including MIG
		"cost:nvidia_gpu_capacity_memory_mib_mig":     "(DCGM_FI_PROF_GR_ENGINE_ACTIVE{UUID!='', GPU_I_ID!=''} * on(exported_pod, exported_namespace) group_left(pod, namespace) max by (exported_pod, exported_namespace) (label_replace(label_replace(kube_pod_status_phase{phase='Running'}, 'exported_pod', '$1', 'pod', '(.*)'), 'exported_namespace', '$1', 'namespace', '(.*)')) * on(Hostname) group_left(label_nvidia_com_gpu_memory, label_nvidia_com_mig_strategy) label_replace(kube_node_labels{label_nvidia_com_gpu_memory!=''}, 'Hostname', '$1', 'node', '(.+)')) or (label_replace(label_replace(DCGM_FI_PROF_GR_ENGINE_ACTIVE{UUID!='', GPU_I_ID!='', exported_namespace=''} * on(pod, namespace) group_left max by (pod, namespace) (kube_pod_status_phase{phase='Running'}) * on(Hostname) group_left(label_nvidia_com_gpu_memory, label_nvidia_com_mig_strategy) label_replace(kube_node_labels{label_nvidia_com_gpu_memory!=''}, 'Hostname', '$1', 'node', '(.+)'), 'exported_pod', '$1', 'pod', '(.*)'), 'exported_namespace', '$1', 'namespace', '(.*)'))",
		"cost:nvidia_gpu_capacity_memory_mib_non_mig": "sum by (pod, namespace, node, label_nvidia_com_gpu_memory) ((kube_pod_container_resource_requests{pod!='', namespace!='', node!='', resource='nvidia_com_gpu'} * on(pod, namespace) group_left max by (pod, namespace) (kube_pod_status_phase{phase='Running'})) * on(node) group_left(label_nvidia_com_gpu_memory) (max by (node, label_nvidia_com_gpu_memory) (kube_node_labels)))",
		"cost:nvidia_gpu_utilization":                 "(sum by (exported_pod, exported_namespace, Hostname, UUID, modelName, GPU_I_ID, GPU_I_PROFILE, device) (DCGM_FI_PROF_GR_ENGINE_ACTIVE{UUID!=''}) * on(exported_pod, exported_namespace) group_left(pod, namespace) max by (exported_pod, exported_namespace) (label_replace(label_replace(kube_pod_status_phase{phase='Running'}, 'exported_pod', '$1', 'pod', '(.*)'), 'exported_namespace', '$1', 'namespace', '(.*)'))) or (label_replace(label_replace(sum by (pod, namespace, Hostname, UUID, modelName, GPU_I_ID, GPU_I_PROFILE, device) (DCGM_FI_PROF_GR_ENGINE_ACTIVE{UUID!='', exported_namespace=''}) * on(pod, namespace) group_left max by (pod, namespace) (kube_pod_status_phase{phase='Running'}), 'exported_pod', '$1', 'pod', '(.*)'), 'exported_namespace', '$1', 'namespace', '(.*)'))",
		"cost:nvidia_gpu_pod_uptime":                  "(sum by (exported_pod, exported_namespace, Hostname, UUID, modelName, GPU_I_ID, GPU_I_PROFILE, device) (clamp_max(DCGM_FI_PROF_GR_ENGINE_ACTIVE{UUID!=''} + 1, 1)) * on(exported_pod, exported_namespace) group_left(pod, namespace) max by (exported_pod, exported_namespace) (label_replace(label_replace(kube_pod_status_phase{phase='Running'}, 'exported_pod', '$1', 'pod', '(.*)'), 'exported_namespace', '$1', 'namespace', '(.*)'))) or (label_replace(label_replace(sum by (pod, namespace, Hostname, UUID, modelName, GPU_I_ID, GPU_I_PROFILE, device) (clamp_max(DCGM_FI_PROF_GR_ENGINE_ACTIVE{UUID!='', exported_namespace=''} + 1, 1)) * on(pod, namespace) group_left max by (pod, namespace) (kube_pod_status_phase{phase='Running'}), 'exported_pod', '$1', 'pod', '(.*)'), 'exported_namespace', '$1', 'namespace', '(.*)'))",
		"cost:nvidia_gpu_max_slices":                  "(sum by (exported_pod, exported_namespace, Hostname, UUID, modelName, GPU_I_ID, GPU_I_PROFILE) (DCGM_FI_DEV_MIG_MAX_SLICES{UUID!=''}) * on(exported_pod, exported_namespace) group_left(pod, namespace) max by (exported_pod, exported_namespace) (label_replace(label_replace(kube_pod_status_phase{phase='Running'}, 'exported_pod', '$1', 'pod', '(.*)'), 'exported_namespace', '$1', 'namespace', '(.*)'))) or (label_replace(label_replace(sum by (pod, namespace, Hostname, UUID, modelName, GPU_I_ID, GPU_I_PROFILE) (DCGM_FI_DEV_MIG_MAX_SLICES{UUID!='', exported_namespace=''}) * on(pod, namespace) group_left max by (pod, namespace) (kube_pod_status_phase{phase='Running'}), 'exported_pod', '$1', 'pod', '(.*)'), 'exported_namespace', '$1', 'namespace', '(.*)'))",

		// resource optimization container metrics queries
		"ros:namespace_filter":               "kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'} or kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'}",
		"ros:image_owners":                   "((max_over_time(kube_pod_container_info{container!='', container!='POD'}[15m]) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'}) or (max_over_time(kube_pod_container_info{container!='', container!='POD'}[15m]) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})) * on(pod, namespace) group_left(owner_kind, owner_name) max by(pod, namespace, owner_kind, owner_name) (max_over_time(kube_pod_owner{container!='', container!='POD', pod!=''}[15m]))",
		"ros:image_workloads":                "((max_over_time(kube_pod_container_info{container!='', container!='POD'}[15m]) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'}) or (max_over_time(kube_pod_container_info{container!='', container!='POD'}[15m]) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})) * on(pod, namespace) group_left(workload, workload_type) max by(pod, namespace, workload, workload_type) (max_over_time(namespace_workload_pod:kube_pod_owner:relabel{pod!='', workload_type!~'(?i)deploymentconfig'}[15m]))",
		"ros:cpu_request_container_avg":      "((avg by(container, pod, namespace, node) (kube_pod_container_resource_requests{container!='', container!='POD', pod!='', resource='cpu', unit='core'} * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})) or (avg by(container, pod, namespace, node) (kube_pod_container_resource_requests{container!='', container!='POD', pod!='', resource='cpu', unit='core'} * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'}))) * on(pod, namespace) group_left max by (container, pod, namespace) (kube_pod_status_phase{phase='Running'})",
		"ros:cpu_request_container_sum":      "((sum by(container, pod, namespace, node) (kube_pod_container_resource_requests{container!='', container!='POD', pod!='', resource='cpu', unit='core'} * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})) or (sum by(container, pod, namespace, node) (kube_pod_container_resource_requests{container!='', container!='POD', pod!='', resource='cpu', unit='core'} * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'}))) * on(pod, namespace) group_left max by (container, pod, namespace) (kube_pod_status_phase{phase='Running'})",
		"ros:cpu_limit_container_avg":        "((avg by(container, pod, namespace, node) (kube_pod_container_resource_limits{container!='', container!='POD', pod!='', resource='cpu', unit='core'} * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})) or (avg by(container, pod, namespace, node) (kube_pod_container_resource_limits{container!='', container!='POD', pod!='', resource='cpu', unit='core'} * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'}))) * on(pod, namespace) group_left max by (container, pod, namespace) (kube_pod_status_phase{phase='Running'})",
		"ros:cpu_limit_container_sum":        "((sum by(container, pod, namespace, node) (kube_pod_container_resource_limits{container!='', container!='POD', pod!='', resource='cpu', unit='core'} * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})) or (sum by(container, pod, namespace, node) (kube_pod_container_resource_limits{container!='', container!='POD', pod!='', resource='cpu', unit='core'} * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'}))) * on(pod, namespace) group_left max by (container, pod, namespace) (kube_pod_status_phase{phase='Running'})",
		"ros:cpu_usage_container_avg":        "(avg by(container, pod, namespace, node) (avg_over_time(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{container!='', container!='POD', pod!=''}[15m]) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})) or (avg by(container, pod, namespace, node) (avg_over_time(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{container!='', container!='POD', pod!=''}[15m]) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'}))",
		"ros:cpu_usage_container_min":        "(min by(container, pod, namespace, node) (min_over_time(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{container!='', container!='POD', pod!=''}[15m]) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})) or (min by(container, pod, namespace, node) (min_over_time(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{container!='', container!='POD', pod!=''}[15m]) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'}))",
		"ros:cpu_usage_container_max":        "(max by(container, pod, namespace, node) (max_over_time(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{container!='', container!='POD', pod!=''}[15m]) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})) or (max by(container, pod, namespace, node) (max_over_time(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{container!='', container!='POD', pod!=''}[15m]) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'}))",
		"ros:cpu_usage_container_sum":        "(sum by(container, pod, namespace, node) (avg_over_time(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{container!='', container!='POD', pod!=''}[15m]) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})) or (sum by(container, pod, namespace, node) (avg_over_time(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{container!='', container!='POD', pod!=''}[15m]) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'}))",
		"ros:cpu_throttle_container_avg":     "(avg by(container, pod, namespace, node) (rate(container_cpu_cfs_throttled_seconds_total{container!='', container!='POD', pod!=''}[15m]) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})) or (avg by(container, pod, namespace, node) (rate(container_cpu_cfs_throttled_seconds_total{container!='', container!='POD', pod!=''}[15m]) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'}))",
		"ros:cpu_throttle_container_max":     "(max by(container, pod, namespace, node) (rate(container_cpu_cfs_throttled_seconds_total{container!='', container!='POD', pod!=''}[15m]) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})) or (max by(container, pod, namespace, node) (rate(container_cpu_cfs_throttled_seconds_total{container!='', container!='POD', pod!=''}[15m]) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'}))",
		"ros:cpu_throttle_container_min":     "(min by(container, pod, namespace, node) (rate(container_cpu_cfs_throttled_seconds_total{container!='', container!='POD', pod!=''}[15m]) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})) or (min by(container, pod, namespace, node) (rate(container_cpu_cfs_throttled_seconds_total{container!='', container!='POD', pod!=''}[15m]) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'}))",
		"ros:cpu_throttle_container_sum":     "(sum by(container, pod, namespace, node) (rate(container_cpu_cfs_throttled_seconds_total{container!='', container!='POD', pod!=''}[15m]) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})) or (sum by(container, pod, namespace, node) (rate(container_cpu_cfs_throttled_seconds_total{container!='', container!='POD', pod!=''}[15m]) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'}))",
		"ros:memory_request_container_avg":   "((avg by(container, pod, namespace, node) (kube_pod_container_resource_requests{container!='', container!='POD', pod!='', resource='memory', unit='byte'} * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})) or (avg by(container, pod, namespace, node) (kube_pod_container_resource_requests{container!='', container!='POD', pod!='', resource='memory', unit='byte'} * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'}))) * on(pod, namespace) group_left max by (container, pod, namespace) (kube_pod_status_phase{phase='Running'})",
		"ros:memory_request_container_sum":   "((sum by(container, pod, namespace, node) (kube_pod_container_resource_requests{container!='', container!='POD', pod!='', resource='memory', unit='byte'} * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})) or (sum by(container, pod, namespace, node) (kube_pod_container_resource_requests{container!='', container!='POD', pod!='', resource='memory', unit='byte'} * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'}))) * on(pod, namespace) group_left max by (container, pod, namespace) (kube_pod_status_phase{phase='Running'})",
		"ros:memory_limit_container_avg":     "((avg by(container, pod, namespace, node) (kube_pod_container_resource_limits{container!='', container!='POD', pod!='', resource='memory', unit='byte'} * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})) or (avg by(container, pod, namespace, node) (kube_pod_container_resource_limits{container!='', container!='POD', pod!='', resource='memory', unit='byte'} * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'}))) * on(pod, namespace) group_left max by (container, pod, namespace) (kube_pod_status_phase{phase='Running'})",
		"ros:memory_limit_container_sum":     "((sum by(container, pod, namespace, node) (kube_pod_container_resource_limits{container!='', container!='POD', pod!='', resource='memory', unit='byte'} * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})) or (sum by(container, pod, namespace, node) (kube_pod_container_resource_limits{container!='', container!='POD', pod!='', resource='memory', unit='byte'} * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'}))) * on(pod, namespace) group_left max by (container, pod, namespace) (kube_pod_status_phase{phase='Running'})",
		"ros:memory_usage_container_avg":     "(avg by(container, pod, namespace, node) (avg_over_time(container_memory_working_set_bytes{container!='', container!='POD', pod!=''}[15m]) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})) or (avg by(container, pod, namespace, node) (avg_over_time(container_memory_working_set_bytes{container!='', container!='POD', pod!=''}[15m]) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'}))",
		"ros:memory_usage_container_min":     "(min by(container, pod, namespace, node) (min_over_time(container_memory_working_set_bytes{container!='', container!='POD', pod!=''}[15m]) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})) or (min by(container, pod, namespace, node) (min_over_time(container_memory_working_set_bytes{container!='', container!='POD', pod!=''}[15m]) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'}))",
		"ros:memory_usage_container_max":     "(max by(container, pod, namespace, node) (max_over_time(container_memory_working_set_bytes{container!='', container!='POD', pod!=''}[15m]) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})) or (max by(container, pod, namespace, node) (max_over_time(container_memory_working_set_bytes{container!='', container!='POD', pod!=''}[15m]) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'}))",
		"ros:memory_usage_container_sum":     "(sum by(container, pod, namespace, node) (avg_over_time(container_memory_working_set_bytes{container!='', container!='POD', pod!=''}[15m]) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})) or (sum by(container, pod, namespace, node) (avg_over_time(container_memory_working_set_bytes{container!='', container!='POD', pod!=''}[15m]) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'}))",
		"ros:memory_rss_usage_container_avg": "(avg by(container, pod, namespace, node) (avg_over_time(container_memory_rss{container!='', container!='POD', pod!=''}[15m]) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})) or (avg by(container, pod, namespace, node) (avg_over_time(container_memory_rss{container!='', container!='POD', pod!=''}[15m]) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'}))",
		"ros:memory_rss_usage_container_min": "(min by(container, pod, namespace, node) (min_over_time(container_memory_rss{container!='', container!='POD', pod!=''}[15m]) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})) or (min by(container, pod, namespace, node) (min_over_time(container_memory_rss{container!='', container!='POD', pod!=''}[15m]) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'}))",
		"ros:memory_rss_usage_container_max": "(max by(container, pod, namespace, node) (max_over_time(container_memory_rss{container!='', container!='POD', pod!=''}[15m]) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})) or (max by(container, pod, namespace, node) (max_over_time(container_memory_rss{container!='', container!='POD', pod!=''}[15m]) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'}))",
		"ros:memory_rss_usage_container_sum": "(sum by(container, pod, namespace, node) (avg_over_time(container_memory_rss{container!='', container!='POD', pod!=''}[15m]) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})) or (sum by(container, pod, namespace, node) (avg_over_time(container_memory_rss{container!='', container!='POD', pod!=''}[15m]) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'}))",
		"ros:oom_count_container_sum":        "(sum by(container, pod, namespace) (increase(kube_pod_container_status_restarts_total{container!='', container!='POD', pod!=''}[15m]) * on(pod, namespace, container) group_left (kube_pod_container_status_last_terminated_reason{reason='OOMKilled'} > 0)) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'}) or (sum by(container, pod, namespace) (increase(kube_pod_container_status_restarts_total{container!='', container!='POD', pod!=''}[15m]) * on(pod, namespace, container) group_left (kube_pod_container_status_last_terminated_reason{reason='OOMKilled'} > 0)) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})",

		// Workload pod count: counts ready pods per (container, namespace, workload, workload_type)
		// and broadcasts the count to each per-pod row via a many-to-one join.
		// Each per-pod series gets value = count of ready pods in its workload.
		"ros:workload_pod_count": "(" +
			"((max_over_time(kube_pod_container_status_ready{container!='', container!='POD', pod!=''}[15m]) == 1) * on(pod, namespace) group_left(workload, workload_type) max by(pod, namespace, workload, workload_type) (max_over_time(namespace_workload_pod:kube_pod_owner:relabel{pod!=''}[15m])) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'}) " +
			"or " +
			"((max_over_time(kube_pod_container_status_ready{container!='', container!='POD', pod!=''}[15m]) == 1) * on(pod, namespace) group_left(workload, workload_type) max by(pod, namespace, workload, workload_type) (max_over_time(namespace_workload_pod:kube_pod_owner:relabel{pod!=''}[15m])) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})" +
			") * on(container, namespace, workload, workload_type) group_left() count by(container, namespace, workload, workload_type) (" +
			"((max_over_time(kube_pod_container_status_ready{container!='', container!='POD', pod!=''}[15m]) == 1) * on(pod, namespace) group_left(workload, workload_type) max by(pod, namespace, workload, workload_type) (max_over_time(namespace_workload_pod:kube_pod_owner:relabel{pod!=''}[15m])) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'}) " +
			"or " +
			"((max_over_time(kube_pod_container_status_ready{container!='', container!='POD', pod!=''}[15m]) == 1) * on(pod, namespace) group_left(workload, workload_type) max by(pod, namespace, workload, workload_type) (max_over_time(namespace_workload_pod:kube_pod_owner:relabel{pod!=''}[15m])) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})" +
			")",

		// Desired replicas: the spec-level replica count for deployments, statefulsets, and daemonsets,
		// broadcast to each per-pod container row via workload join.
		// Pod info (many per workload) is LEFT; replica count (one per workload) is RIGHT.
		"ros:desired_replicas": "max by(container, pod, namespace, workload) (max_over_time(kube_pod_container_info{container!='', container!='POD', pod!=''}[15m]) * on(pod, namespace) group_left(workload) max by(pod, namespace, workload) (max_over_time(namespace_workload_pod:kube_pod_owner:relabel{pod!=''}[15m])))" +
			" * on(namespace, workload) group_left() (" +
			"label_replace(" +
			"(max by(namespace, workload, workload_type) (" +
			"label_replace(kube_deployment_spec_replicas, 'workload', '$1', 'deployment', '(.+)') " +
			"or label_replace(kube_statefulset_replicas, 'workload', '$1', 'statefulset', '(.+)') " +
			"or label_replace(kube_daemonset_status_desired_number_scheduled, 'workload', '$1', 'daemonset', '(.+)')" +
			") * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})" +
			", 'workload_type', 'deployment', 'workload_type', '')" +
			" or label_replace(" +
			"(max by(namespace, workload, workload_type) (" +
			"label_replace(kube_deployment_spec_replicas, 'workload', '$1', 'deployment', '(.+)') " +
			"or label_replace(kube_statefulset_replicas, 'workload', '$1', 'statefulset', '(.+)') " +
			"or label_replace(kube_daemonset_status_desired_number_scheduled, 'workload', '$1', 'daemonset', '(.+)')" +
			") * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})" +
			", 'workload_type', 'deployment', 'workload_type', '')" +
			")",

		// Available replicas: deployment available, statefulset ready, or daemonset available replicas,
		// broadcast to each per-pod container row via workload join.
		// Pod info (many per workload) is LEFT; replica count (one per workload) is RIGHT.
		"ros:available_replicas": "max by(container, pod, namespace, workload) (max_over_time(kube_pod_container_info{container!='', container!='POD', pod!=''}[15m]) * on(pod, namespace) group_left(workload) max by(pod, namespace, workload) (max_over_time(namespace_workload_pod:kube_pod_owner:relabel{pod!=''}[15m])))" +
			" * on(namespace, workload) group_left() (" +
			"label_replace(" +
			"(max by(namespace, workload, workload_type) (" +
			"label_replace(kube_deployment_status_replicas_available, 'workload', '$1', 'deployment', '(.+)') " +
			"or label_replace(kube_statefulset_status_replicas_ready, 'workload', '$1', 'statefulset', '(.+)') " +
			"or label_replace(kube_daemonset_status_number_available, 'workload', '$1', 'daemonset', '(.+)')" +
			") * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})" +
			", 'workload_type', 'deployment', 'workload_type', '')" +
			" or label_replace(" +
			"(max by(namespace, workload, workload_type) (" +
			"label_replace(kube_deployment_status_replicas_available, 'workload', '$1', 'deployment', '(.+)') " +
			"or label_replace(kube_statefulset_status_replicas_ready, 'workload', '$1', 'statefulset', '(.+)') " +
			"or label_replace(kube_daemonset_status_number_available, 'workload', '$1', 'daemonset', '(.+)')" +
			") * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})" +
			", 'workload_type', 'deployment', 'workload_type', '')" +
			")",

		// resource optimization NVIDIA GPU container level metrics queries
		"ros:accelerator_frame_buffer_usage_min": "(min by (modelName, GPU_I_PROFILE, exported_container, exported_namespace, exported_pod, Hostname) (min_over_time(DCGM_FI_DEV_FB_USED{exported_namespace != '', exported_container != '', exported_pod != ''}[15m])) * on(exported_namespace) group_left(namespace) label_replace(kube_namespace_labels{label_insights_cost_management_optimizations='true'}, 'exported_namespace', '$1', 'namespace', '(.*)') or min by (modelName, GPU_I_PROFILE, exported_container, exported_namespace, exported_pod, Hostname) (min_over_time(DCGM_FI_DEV_FB_USED{exported_namespace != '', exported_container != '', exported_pod != ''}[15m])) * on(exported_namespace) group_left(namespace) label_replace(kube_namespace_labels{label_cost_management_optimizations='true'}, 'exported_namespace', '$1', 'namespace', '(.*)') or label_replace(label_replace(label_replace(min by (modelName, GPU_I_PROFILE, container, namespace, pod, Hostname) (min_over_time(DCGM_FI_DEV_FB_USED{namespace != '', container != '', pod != '', exported_namespace=''}[15m])) * on(namespace) group_left() kube_namespace_labels{label_insights_cost_management_optimizations='true'}, 'exported_container', '$1', 'container', '(.*)'), 'exported_namespace', '$1', 'namespace', '(.*)'), 'exported_pod', '$1', 'pod', '(.*)') or label_replace(label_replace(label_replace(min by (modelName, GPU_I_PROFILE, container, namespace, pod, Hostname) (min_over_time(DCGM_FI_DEV_FB_USED{namespace != '', container != '', pod != '', exported_namespace=''}[15m])) * on(namespace) group_left() kube_namespace_labels{label_cost_management_optimizations='true'}, 'exported_container', '$1', 'container', '(.*)'), 'exported_namespace', '$1', 'namespace', '(.*)'), 'exported_pod', '$1', 'pod', '(.*)'))",
		"ros:accelerator_frame_buffer_usage_max": "(max by (modelName, GPU_I_PROFILE, exported_container, exported_namespace, exported_pod, Hostname) (max_over_time(DCGM_FI_DEV_FB_USED{exported_namespace != '', exported_container != '', exported_pod != ''}[15m])) * on(exported_namespace) group_left(namespace) label_replace(kube_namespace_labels{label_insights_cost_management_optimizations='true'}, 'exported_namespace', '$1', 'namespace', '(.*)') or max by (modelName, GPU_I_PROFILE, exported_container, exported_namespace, exported_pod, Hostname) (max_over_time(DCGM_FI_DEV_FB_USED{exported_namespace != '', exported_container != '', exported_pod != ''}[15m])) * on(exported_namespace) group_left(namespace) label_replace(kube_namespace_labels{label_cost_management_optimizations='true'}, 'exported_namespace', '$1', 'namespace', '(.*)') or label_replace(label_replace(label_replace(max by (modelName, GPU_I_PROFILE, container, namespace, pod, Hostname) (max_over_time(DCGM_FI_DEV_FB_USED{namespace != '', container != '', pod != '', exported_namespace=''}[15m])) * on(namespace) group_left() kube_namespace_labels{label_insights_cost_management_optimizations='true'}, 'exported_container', '$1', 'container', '(.*)'), 'exported_namespace', '$1', 'namespace', '(.*)'), 'exported_pod', '$1', 'pod', '(.*)') or label_replace(label_replace(label_replace(max by (modelName, GPU_I_PROFILE, container, namespace, pod, Hostname) (max_over_time(DCGM_FI_DEV_FB_USED{namespace != '', container != '', pod != '', exported_namespace=''}[15m])) * on(namespace) group_left() kube_namespace_labels{label_cost_management_optimizations='true'}, 'exported_container', '$1', 'container', '(.*)'), 'exported_namespace', '$1', 'namespace', '(.*)'), 'exported_pod', '$1', 'pod', '(.*)'))",
		"ros:accelerator_frame_buffer_usage_avg": "(avg by (modelName, GPU_I_PROFILE, exported_container, exported_namespace, exported_pod, Hostname) (avg_over_time(DCGM_FI_DEV_FB_USED{exported_namespace != '', exported_container != '', exported_pod != ''}[15m])) * on(exported_namespace) group_left(namespace) label_replace(kube_namespace_labels{label_insights_cost_management_optimizations='true'}, 'exported_namespace', '$1', 'namespace', '(.*)') or avg by (modelName, GPU_I_PROFILE, exported_container, exported_namespace, exported_pod, Hostname) (avg_over_time(DCGM_FI_DEV_FB_USED{exported_namespace != '', exported_container != '', exported_pod != ''}[15m])) * on(exported_namespace) group_left(namespace) label_replace(kube_namespace_labels{label_cost_management_optimizations='true'}, 'exported_namespace', '$1', 'namespace', '(.*)') or label_replace(label_replace(label_replace(avg by (modelName, GPU_I_PROFILE, container, namespace, pod, Hostname) (avg_over_time(DCGM_FI_DEV_FB_USED{namespace != '', container != '', pod != '', exported_namespace=''}[15m])) * on(namespace) group_left() kube_namespace_labels{label_insights_cost_management_optimizations='true'}, 'exported_container', '$1', 'container', '(.*)'), 'exported_namespace', '$1', 'namespace', '(.*)'), 'exported_pod', '$1', 'pod', '(.*)') or label_replace(label_replace(label_replace(avg by (modelName, GPU_I_PROFILE, container, namespace, pod, Hostname) (avg_over_time(DCGM_FI_DEV_FB_USED{namespace != '', container != '', pod != '', exported_namespace=''}[15m])) * on(namespace) group_left() kube_namespace_labels{label_cost_management_optimizations='true'}, 'exported_container', '$1', 'container', '(.*)'), 'exported_namespace', '$1', 'namespace', '(.*)'), 'exported_pod', '$1', 'pod', '(.*)'))",
		"ros:tensor_pipe_active_min":             "(min by (modelName, GPU_I_PROFILE, exported_container, exported_namespace, exported_pod, Hostname) (min_over_time(DCGM_FI_PROF_PIPE_TENSOR_ACTIVE{exported_namespace != '', exported_container != '', exported_pod != ''}[15m])) * on(exported_namespace) group_left(namespace) label_replace(kube_namespace_labels{label_insights_cost_management_optimizations='true'}, 'exported_namespace', '$1', 'namespace', '(.*)') or min by (modelName, GPU_I_PROFILE, exported_container, exported_namespace, exported_pod, Hostname) (min_over_time(DCGM_FI_PROF_PIPE_TENSOR_ACTIVE{exported_namespace != '', exported_container != '', exported_pod != ''}[15m])) * on(exported_namespace) group_left(namespace) label_replace(kube_namespace_labels{label_cost_management_optimizations='true'}, 'exported_namespace', '$1', 'namespace', '(.*)'))",
		"ros:tensor_pipe_active_max":             "(max by (modelName, GPU_I_PROFILE, exported_container, exported_namespace, exported_pod, Hostname) (max_over_time(DCGM_FI_PROF_PIPE_TENSOR_ACTIVE{exported_namespace != '', exported_container != '', exported_pod != ''}[15m])) * on(exported_namespace) group_left(namespace) label_replace(kube_namespace_labels{label_insights_cost_management_optimizations='true'}, 'exported_namespace', '$1', 'namespace', '(.*)') or max by (modelName, GPU_I_PROFILE, exported_container, exported_namespace, exported_pod, Hostname) (max_over_time(DCGM_FI_PROF_PIPE_TENSOR_ACTIVE{exported_namespace != '', exported_container != '', exported_pod != ''}[15m])) * on(exported_namespace) group_left(namespace) label_replace(kube_namespace_labels{label_cost_management_optimizations='true'}, 'exported_namespace', '$1', 'namespace', '(.*)'))",
		"ros:tensor_pipe_active_avg":             "(avg by (modelName, GPU_I_PROFILE, exported_container, exported_namespace, exported_pod, Hostname) (avg_over_time(DCGM_FI_PROF_PIPE_TENSOR_ACTIVE{exported_namespace != '', exported_container != '', exported_pod != ''}[15m])) * on(exported_namespace) group_left(namespace) label_replace(kube_namespace_labels{label_insights_cost_management_optimizations='true'}, 'exported_namespace', '$1', 'namespace', '(.*)') or avg by (modelName, GPU_I_PROFILE, exported_container, exported_namespace, exported_pod, Hostname) (avg_over_time(DCGM_FI_PROF_PIPE_TENSOR_ACTIVE{exported_namespace != '', exported_container != '', exported_pod != ''}[15m])) * on(exported_namespace) group_left(namespace) label_replace(kube_namespace_labels{label_cost_management_optimizations='true'}, 'exported_namespace', '$1', 'namespace', '(.*)'))",
		"ros:dram_active_min":                    "(min by (modelName, GPU_I_PROFILE, exported_container, exported_namespace, exported_pod, Hostname) (min_over_time(DCGM_FI_PROF_DRAM_ACTIVE{exported_namespace != '', exported_container != '', exported_pod != ''}[15m])) * on(exported_namespace) group_left(namespace) label_replace(kube_namespace_labels{label_insights_cost_management_optimizations='true'}, 'exported_namespace', '$1', 'namespace', '(.*)') or min by (modelName, GPU_I_PROFILE, exported_container, exported_namespace, exported_pod, Hostname) (min_over_time(DCGM_FI_PROF_DRAM_ACTIVE{exported_namespace != '', exported_container != '', exported_pod != ''}[15m])) * on(exported_namespace) group_left(namespace) label_replace(kube_namespace_labels{label_cost_management_optimizations='true'}, 'exported_namespace', '$1', 'namespace', '(.*)'))",
		"ros:dram_active_max":                    "(max by (modelName, GPU_I_PROFILE, exported_container, exported_namespace, exported_pod, Hostname) (max_over_time(DCGM_FI_PROF_DRAM_ACTIVE{exported_namespace != '', exported_container != '', exported_pod != ''}[15m])) * on(exported_namespace) group_left(namespace) label_replace(kube_namespace_labels{label_insights_cost_management_optimizations='true'}, 'exported_namespace', '$1', 'namespace', '(.*)') or max by (modelName, GPU_I_PROFILE, exported_container, exported_namespace, exported_pod, Hostname) (max_over_time(DCGM_FI_PROF_DRAM_ACTIVE{exported_namespace != '', exported_container != '', exported_pod != ''}[15m])) * on(exported_namespace) group_left(namespace) label_replace(kube_namespace_labels{label_cost_management_optimizations='true'}, 'exported_namespace', '$1', 'namespace', '(.*)'))",
		"ros:dram_active_avg":                    "(avg by (modelName, GPU_I_PROFILE, exported_container, exported_namespace, exported_pod, Hostname) (avg_over_time(DCGM_FI_PROF_DRAM_ACTIVE{exported_namespace != '', exported_container != '', exported_pod != ''}[15m])) * on(exported_namespace) group_left(namespace) label_replace(kube_namespace_labels{label_insights_cost_management_optimizations='true'}, 'exported_namespace', '$1', 'namespace', '(.*)') or avg by (modelName, GPU_I_PROFILE, exported_container, exported_namespace, exported_pod, Hostname) (avg_over_time(DCGM_FI_PROF_DRAM_ACTIVE{exported_namespace != '', exported_container != '', exported_pod != ''}[15m])) * on(exported_namespace) group_left(namespace) label_replace(kube_namespace_labels{label_cost_management_optimizations='true'}, 'exported_namespace', '$1', 'namespace', '(.*)'))",
		"ros:sm_active_min":                      "(min by (modelName, GPU_I_PROFILE, exported_container, exported_namespace, exported_pod, Hostname) (min_over_time(DCGM_FI_PROF_SM_ACTIVE{exported_namespace != '', exported_container != '', exported_pod != ''}[15m])) * on(exported_namespace) group_left(namespace) label_replace(kube_namespace_labels{label_insights_cost_management_optimizations='true'}, 'exported_namespace', '$1', 'namespace', '(.*)') or min by (modelName, GPU_I_PROFILE, exported_container, exported_namespace, exported_pod, Hostname) (min_over_time(DCGM_FI_PROF_SM_ACTIVE{exported_namespace != '', exported_container != '', exported_pod != ''}[15m])) * on(exported_namespace) group_left(namespace) label_replace(kube_namespace_labels{label_cost_management_optimizations='true'}, 'exported_namespace', '$1', 'namespace', '(.*)'))",
		"ros:sm_active_max":                      "(max by (modelName, GPU_I_PROFILE, exported_container, exported_namespace, exported_pod, Hostname) (max_over_time(DCGM_FI_PROF_SM_ACTIVE{exported_namespace != '', exported_container != '', exported_pod != ''}[15m])) * on(exported_namespace) group_left(namespace) label_replace(kube_namespace_labels{label_insights_cost_management_optimizations='true'}, 'exported_namespace', '$1', 'namespace', '(.*)') or max by (modelName, GPU_I_PROFILE, exported_container, exported_namespace, exported_pod, Hostname) (max_over_time(DCGM_FI_PROF_SM_ACTIVE{exported_namespace != '', exported_container != '', exported_pod != ''}[15m])) * on(exported_namespace) group_left(namespace) label_replace(kube_namespace_labels{label_cost_management_optimizations='true'}, 'exported_namespace', '$1', 'namespace', '(.*)'))",
		"ros:sm_active_avg":                      "(avg by (modelName, GPU_I_PROFILE, exported_container, exported_namespace, exported_pod, Hostname) (avg_over_time(DCGM_FI_PROF_SM_ACTIVE{exported_namespace != '', exported_container != '', exported_pod != ''}[15m])) * on(exported_namespace) group_left(namespace) label_replace(kube_namespace_labels{label_insights_cost_management_optimizations='true'}, 'exported_namespace', '$1', 'namespace', '(.*)') or avg by (modelName, GPU_I_PROFILE, exported_container, exported_namespace, exported_pod, Hostname) (avg_over_time(DCGM_FI_PROF_SM_ACTIVE{exported_namespace != '', exported_container != '', exported_pod != ''}[15m])) * on(exported_namespace) group_left(namespace) label_replace(kube_namespace_labels{label_cost_management_optimizations='true'}, 'exported_namespace', '$1', 'namespace', '(.*)'))",

		// resource optimization namespace metrics queries
		"ros:cpu_request_namespace_sum":      "(sum by (namespace) (kube_resourcequota{resource='requests.cpu', type='hard'}) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'} or sum by (namespace) (kube_resourcequota{resource='requests.cpu', type='hard'}) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})",
		"ros:cpu_request_namespace_used":     "(sum by (namespace) (kube_resourcequota{resource='requests.cpu', type='used'}) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'} or sum by (namespace) (kube_resourcequota{resource='requests.cpu', type='used'}) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})",
		"ros:cpu_limit_namespace_sum":        "(sum by (namespace) (kube_resourcequota{ resource='limits.cpu', type='hard'}) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'} or sum by (namespace) (kube_resourcequota{ resource='limits.cpu', type='hard'}) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})",
		"ros:cpu_limit_namespace_used":       "(sum by (namespace) (kube_resourcequota{ resource='limits.cpu', type='used'}) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'} or sum by (namespace) (kube_resourcequota{ resource='limits.cpu', type='used'}) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})",
		"ros:cpu_usage_namespace_avg":        "(avg_over_time(sum by(namespace) (node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{container!='', container!='POD', pod!=''})[15m:]) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'} or avg_over_time(sum by(namespace) (node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{container!='', container!='POD', pod!=''})[15m:]) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})",
		"ros:cpu_usage_namespace_max":        "(max_over_time(sum by(namespace) (node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{container!='', container!='POD', pod!=''})[15m:]) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'} or max_over_time(sum by(namespace) (node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{container!='', container!='POD', pod!=''})[15m:]) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})",
		"ros:cpu_usage_namespace_min":        "(min_over_time(sum by(namespace) (node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{container!='', container!='POD', pod!=''})[15m:]) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'} or min_over_time(sum by(namespace) (node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{container!='', container!='POD', pod!=''})[15m:]) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})",
		"ros:cpu_throttle_namespace_avg":     "(avg_over_time(sum by(namespace) (rate(container_cpu_cfs_throttled_seconds_total{container!='', container!='POD', pod!=''}[5m]))[15m:]) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'} or avg_over_time(sum by(namespace) (rate(container_cpu_cfs_throttled_seconds_total{container!='', container!='POD', pod!=''}[5m]))[15m:]) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})",
		"ros:cpu_throttle_namespace_max":     "(max_over_time(sum by(namespace) (rate(container_cpu_cfs_throttled_seconds_total{container!='', container!='POD', pod!=''}[5m]))[15m:]) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'} or max_over_time(sum by(namespace) (rate(container_cpu_cfs_throttled_seconds_total{container!='', container!='POD', pod!=''}[5m]))[15m:]) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})",
		"ros:cpu_throttle_namespace_min":     "(min_over_time(sum by(namespace) (rate(container_cpu_cfs_throttled_seconds_total{container!='', container!='POD', pod!=''}[5m]))[15m:]) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'} or min_over_time(sum by(namespace) (rate(container_cpu_cfs_throttled_seconds_total{container!='', container!='POD', pod!=''}[5m]))[15m:]) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})",
		"ros:memory_request_namespace_sum":   "(sum by (namespace) (kube_resourcequota{ resource='requests.memory', type='hard'}) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'} or sum by (namespace) (kube_resourcequota{ resource='requests.memory', type='hard'}) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})",
		"ros:memory_request_namespace_used":  "(sum by (namespace) (kube_resourcequota{ resource='requests.memory', type='used'}) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'} or sum by (namespace) (kube_resourcequota{ resource='requests.memory', type='used'}) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})",
		"ros:memory_limit_namespace_sum":     "(sum by (namespace) (kube_resourcequota{ resource='limits.memory', type='hard'}) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'} or sum by (namespace) (kube_resourcequota{ resource='limits.memory', type='hard'}) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})",
		"ros:memory_limit_namespace_used":    "(sum by (namespace) (kube_resourcequota{ resource='limits.memory', type='used'}) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'} or sum by (namespace) (kube_resourcequota{ resource='limits.memory', type='used'}) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})",
		"ros:memory_usage_namespace_avg":     "(avg_over_time(sum by(namespace) (container_memory_working_set_bytes{container!='', container!='POD', pod!=''})[15m:]) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'} or avg_over_time(sum by(namespace) (container_memory_working_set_bytes{container!='', container!='POD', pod!=''})[15m:]) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})",
		"ros:memory_usage_namespace_max":     "(max_over_time(sum by(namespace) (container_memory_working_set_bytes{container!='', container!='POD', pod!=''})[15m:]) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'} or max_over_time(sum by(namespace) (container_memory_working_set_bytes{container!='', container!='POD', pod!=''})[15m:]) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})",
		"ros:memory_usage_namespace_min":     "(min_over_time(sum by(namespace) (container_memory_working_set_bytes{container!='', container!='POD', pod!=''})[15m:]) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'} or min_over_time(sum by(namespace) (container_memory_working_set_bytes{container!='', container!='POD', pod!=''})[15m:]) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})",
		"ros:memory_rss_usage_namespace_avg": "(avg_over_time(sum by(namespace) (container_memory_rss{container!='', container!='POD', pod!=''})[15m:]) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'} or avg_over_time(sum by(namespace) (container_memory_rss{container!='', container!='POD', pod!=''})[15m:]) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})",
		"ros:memory_rss_usage_namespace_max": "(max_over_time(sum by(namespace) (container_memory_rss{container!='', container!='POD', pod!=''})[15m:]) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'} or max_over_time(sum by(namespace) (container_memory_rss{container!='', container!='POD', pod!=''})[15m:]) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})",
		"ros:memory_rss_usage_namespace_min": "(min_over_time(sum by(namespace) (container_memory_rss{container!='', container!='POD', pod!=''})[15m:]) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'} or min_over_time(sum by(namespace) (container_memory_rss{container!='', container!='POD', pod!=''})[15m:]) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})",
		"ros:namespace_running_pods_max":     "(max_over_time(sum by(namespace) (kube_pod_status_phase{phase='Running'})[15m:]) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'} or max_over_time(sum by(namespace) (kube_pod_status_phase{phase='Running'})[15m:]) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})",
		"ros:namespace_running_pods_avg":     "(avg_over_time(sum by(namespace) (kube_pod_status_phase{phase='Running'})[15m:]) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'} or avg_over_time(sum by(namespace) (kube_pod_status_phase{phase='Running'})[15m:]) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})",
		"ros:namespace_total_pods_max":       "(max_over_time(sum by(namespace) (kube_pod_info)[15m:]) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'} or max_over_time(sum by(namespace) (kube_pod_info)[15m:]) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})",
		"ros:namespace_total_pods_avg":       "(avg_over_time(sum by(namespace) (kube_pod_info)[15m:]) * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'} or avg_over_time(sum by(namespace) (kube_pod_info)[15m:]) * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})",

		// cluster-scoped ClusterResourceQuota metrics (openshift-state-metrics)
		"ros:cluster_quota_cpu_request_hard":   "sum by (name) (openshift_clusterresourcequota_usage{resource='requests.cpu', type='hard'})",
		"ros:cluster_quota_cpu_request_used":   "sum by (name) (openshift_clusterresourcequota_usage{resource='requests.cpu', type='used'})",
		"ros:cluster_quota_cpu_limit_hard":     "sum by (name) (openshift_clusterresourcequota_usage{resource='limits.cpu', type='hard'})",
		"ros:cluster_quota_cpu_limit_used":     "sum by (name) (openshift_clusterresourcequota_usage{resource='limits.cpu', type='used'})",
		"ros:cluster_quota_memory_request_hard": "sum by (name) (openshift_clusterresourcequota_usage{resource='requests.memory', type='hard'})",
		"ros:cluster_quota_memory_request_used": "sum by (name) (openshift_clusterresourcequota_usage{resource='requests.memory', type='used'})",
		"ros:cluster_quota_memory_limit_hard":   "sum by (name) (openshift_clusterresourcequota_usage{resource='limits.memory', type='hard'})",
		"ros:cluster_quota_memory_limit_used":   "sum by (name) (openshift_clusterresourcequota_usage{resource='limits.memory', type='used'})",
	}

	rosNamespaceFilter = query{
		Name:        "ros-namespace-filter",
		QueryString: QueryMap["ros:namespace_filter"],
		MetricKey:   staticFields{"namespace": "namespace"},
	}

	nodeQueries = &querys{
		query{
			Name:        "node-allocatable-cpu-cores",
			QueryString: QueryMap["cost:node_allocatable_cpu_cores"],
			MetricKey:   staticFields{"node": "node", "provider_id": "provider_id"},
			QueryValue: &saveQueryValue{
				ValName:         "node-allocatable-cpu-cores",
				Method:          "max",
				TransformedName: "node-allocatable-cpu-core-seconds",
			},
			RowKey: []model.LabelName{"node"},
		},
		query{
			Name:        "node-allocatable-memory-bytes",
			QueryString: QueryMap["cost:node_allocatable_memory_bytes"],
			MetricKey:   staticFields{"node": "node", "provider_id": "provider_id"},
			QueryValue: &saveQueryValue{
				ValName:         "node-allocatable-memory-bytes",
				Method:          "max",
				TransformedName: "node-allocatable-memory-byte-seconds",
			},
			RowKey: []model.LabelName{"node"},
		},
		query{
			Name:        "node-capacity-cpu-cores",
			QueryString: QueryMap["cost:node_capacity_cpu_cores"],
			MetricKey:   staticFields{"node": "node", "provider_id": "provider_id"},
			QueryValue: &saveQueryValue{
				ValName:         "node-capacity-cpu-cores",
				Method:          "max",
				TransformedName: "node-capacity-cpu-core-seconds",
			},
			RowKey: []model.LabelName{"node"},
		},
		query{
			Name:        "node-capacity-memory-bytes",
			QueryString: QueryMap["cost:node_capacity_memory_bytes"],
			MetricKey:   staticFields{"node": "node", "provider_id": "provider_id"},
			QueryValue: &saveQueryValue{
				ValName:         "node-capacity-memory-bytes",
				Method:          "max",
				TransformedName: "node-capacity-memory-byte-seconds",
			},
			RowKey: []model.LabelName{"node"},
		},
		query{
			Name:        "node-role",
			QueryString: "kube_node_role",
			MetricKey:   staticFields{"node": "node", "node-role": "role"},
			RowKey:      []model.LabelName{"node"},
		},
		query{
			Name:           "node-labels",
			QueryString:    "kube_node_labels",
			MetricKeyRegex: regexFields{"node_labels": "label_*"},
			RowKey:         []model.LabelName{"node"},
		},
	}
	volQueries = &querys{
		query{
			Name:        "persistentvolume-pod-info",
			QueryString: QueryMap["cost:persistentvolume_pod_info"],
			MetricKey:   staticFields{"namespace": "namespace", "pod": "pod"},
			RowKey:      []model.LabelName{"volumename"},
		},
		query{
			Name:        "persistentvolumeclaim-capacity-bytes",
			QueryString: QueryMap["cost:persistentvolumeclaim_capacity_bytes"],
			QueryValue: &saveQueryValue{
				ValName:         "persistentvolumeclaim-capacity-bytes",
				Method:          "max",
				TransformedName: "persistentvolumeclaim-capacity-byte-seconds",
			},
			RowKey: []model.LabelName{"persistentvolume"},
		},
		query{
			Name:        "persistentvolumeclaim-request-bytes",
			QueryString: QueryMap["cost:persistentvolumeclaim_request_bytes"],
			QueryValue: &saveQueryValue{
				ValName:         "persistentvolumeclaim-request-bytes",
				Method:          "max",
				TransformedName: "persistentvolumeclaim-request-byte-seconds",
			},
			RowKey: []model.LabelName{"volumename"},
		},
		query{
			Name:        "persistentvolumeclaim-usage-bytes",
			QueryString: QueryMap["cost:persistentvolumeclaim_usage_bytes"],
			MetricKey:   staticFields{"node": "node"},
			QueryValue: &saveQueryValue{
				ValName:         "persistentvolumeclaim-usage-bytes",
				Method:          "sum",
				TransformedName: "persistentvolumeclaim-usage-byte-seconds",
			},
			RowKey: []model.LabelName{"volumename"},
		},
		query{
			Name:           "persistentvolume-labels",
			QueryString:    QueryMap["cost:persistentvolume_labels"],
			MetricKey:      staticFields{"storageclass": "storageclass", "persistentvolume": "persistentvolume", "csi_driver": "csi_driver", "csi_volume_handle": "csi_volume_handle"},
			MetricKeyRegex: regexFields{"persistentvolume_labels": "label_*"},
			RowKey:         []model.LabelName{"persistentvolume"},
		},
		query{
			Name:           "persistentvolumeclaim-labels",
			QueryString:    QueryMap["cost:persistentvolumeclaim_labels"],
			MetricKey:      staticFields{"namespace": "namespace", "persistentvolumeclaim": "persistentvolumeclaim"},
			MetricKeyRegex: regexFields{"persistentvolumeclaim_labels": "label_"},
			RowKey:         []model.LabelName{"volumename"},
		},
	}
	podQueries = &querys{
		query{
			Name:        "pod-limit-cpu-cores",
			QueryString: QueryMap["cost:pod_limit_cpu_cores"],
			MetricKey:   staticFields{"pod": "pod", "namespace": "namespace", "node": "node"},
			QueryValue: &saveQueryValue{
				ValName:         "pod-limit-cpu-cores",
				Method:          "sum",
				TransformedName: "pod-limit-cpu-core-seconds",
			},
			RowKey: []model.LabelName{"pod", "namespace"},
		},
		query{
			Name:        "pod-limit-memory-bytes",
			QueryString: QueryMap["cost:pod_limit_memory_bytes"],
			MetricKey:   staticFields{"pod": "pod", "namespace": "namespace", "node": "node"},
			QueryValue: &saveQueryValue{
				ValName:         "pod-limit-memory-bytes",
				Method:          "sum",
				TransformedName: "pod-limit-memory-byte-seconds",
			},
			RowKey: []model.LabelName{"pod", "namespace"},
		},
		query{
			Name:        "pod-request-cpu-cores",
			QueryString: QueryMap["cost:pod_request_cpu_cores"],
			MetricKey:   staticFields{"pod": "pod", "namespace": "namespace", "node": "node"},
			QueryValue: &saveQueryValue{
				ValName:         "pod-request-cpu-cores",
				Method:          "sum",
				TransformedName: "pod-request-cpu-core-seconds",
			},
			RowKey: []model.LabelName{"pod", "namespace"},
		},
		query{
			Name:        "pod-request-memory-bytes",
			QueryString: QueryMap["cost:pod_request_memory_bytes"],
			MetricKey:   staticFields{"pod": "pod", "namespace": "namespace", "node": "node"},
			QueryValue: &saveQueryValue{
				ValName:         "pod-request-memory-bytes",
				Method:          "sum",
				TransformedName: "pod-request-memory-byte-seconds",
			},
			RowKey: []model.LabelName{"pod", "namespace"},
		},
		query{
			Name:        "pod-usage-cpu-cores",
			QueryString: QueryMap["cost:pod_usage_cpu_cores"],
			MetricKey:   staticFields{"pod": "pod", "namespace": "namespace", "node": "node"},
			QueryValue: &saveQueryValue{
				ValName:         "pod-usage-cpu-cores",
				Method:          "sum",
				TransformedName: "pod-usage-cpu-core-seconds",
			},
			RowKey: []model.LabelName{"pod", "namespace"},
		},
		query{
			Name:        "pod-usage-memory-bytes",
			QueryString: QueryMap["cost:pod_usage_memory_bytes"],
			MetricKey:   staticFields{"pod": "pod", "namespace": "namespace", "node": "node"},
			QueryValue: &saveQueryValue{
				ValName:         "pod-usage-memory-bytes",
				Method:          "sum",
				TransformedName: "pod-usage-memory-byte-seconds",
			},
			RowKey: []model.LabelName{"pod", "namespace"},
		},
		query{
			Name:           "pod-labels",
			QueryString:    QueryMap["cost:pod_labels"],
			MetricKey:      staticFields{"pod": "pod", "namespace": "namespace"},
			MetricKeyRegex: regexFields{"pod_labels": "label_*"},
			RowKey:         []model.LabelName{"pod", "namespace"},
		},
	}
	vmQueries = &querys{
		query{
			Name:        "vm_cpu_limit_cores",
			QueryString: QueryMap["cost:vm_cpu_limit_cores"],
			MetricKey: staticFields{
				"name":      "name",
				"namespace": "namespace",
			},
			QueryValue: &saveQueryValue{
				ValName:         "vm_cpu_limit_cores",
				Method:          "max",
				TransformedName: "vm_cpu_limit_core_seconds",
			},
			RowKey: []model.LabelName{"name", "namespace"},
		},
		query{
			Name:        "vm_cpu_request_cores",
			QueryString: QueryMap["cost:vm_cpu_request_cores"],
			MetricKey: staticFields{
				"name":      "name",
				"namespace": "namespace",
			},
			QueryValue: &saveQueryValue{
				ValName:         "vm_cpu_request_cores",
				Method:          "max",
				TransformedName: "vm_cpu_request_core_seconds",
			},
			RowKey: []model.LabelName{"name", "namespace"},
		},
		query{
			Name:        "vm_cpu_request_sockets",
			QueryString: QueryMap["cost:vm_cpu_request_sockets"],
			MetricKey: staticFields{
				"name":      "name",
				"namespace": "namespace",
			},
			QueryValue: &saveQueryValue{
				ValName:         "vm_cpu_request_sockets",
				Method:          "max",
				TransformedName: "vm_cpu_request_socket_seconds",
			},
			RowKey: []model.LabelName{"name", "namespace"},
		},
		query{
			Name:        "vm_cpu_request_threads",
			QueryString: QueryMap["cost:vm_cpu_request_threads"],
			MetricKey: staticFields{
				"name":      "name",
				"namespace": "namespace",
			},
			QueryValue: &saveQueryValue{
				ValName:         "vm_cpu_request_threads",
				Method:          "max",
				TransformedName: "vm_cpu_request_thread_seconds",
			},
			RowKey: []model.LabelName{"name", "namespace"},
		},
		query{
			Name:        "vm_cpu_usage",
			QueryString: QueryMap["cost:vm_cpu_usage"],
			MetricKey: staticFields{
				"name":      "name",
				"namespace": "namespace",
			},
			QueryValue: &saveQueryValue{
				ValName:         "vm_cpu_usage",
				Method:          "sum",
				TransformedName: "vm_cpu_usage_total_seconds",
			},
			RowKey: []model.LabelName{"name", "namespace"},
		},
		query{
			Name:        "vm_memory_limit_bytes",
			QueryString: QueryMap["cost:vm_memory_limit_bytes"],
			MetricKey: staticFields{
				"name":      "name",
				"namespace": "namespace",
			},
			QueryValue: &saveQueryValue{
				ValName:         "vm_memory_limit_bytes",
				Method:          "max",
				TransformedName: "vm_memory_limit_byte_seconds",
			},
			RowKey: []model.LabelName{"name", "namespace"},
		},
		query{
			Name:        "vm_memory_request_bytes",
			QueryString: QueryMap["cost:vm_memory_request_bytes"],
			MetricKey: staticFields{
				"name":      "name",
				"namespace": "namespace",
				"resource":  "resource",
			},
			QueryValue: &saveQueryValue{
				ValName:         "vm_memory_request_bytes",
				Method:          "max",
				TransformedName: "vm_memory_request_byte_seconds",
			},
			RowKey: []model.LabelName{"name", "namespace"},
		},
		query{
			Name:        "vm_memory_usage",
			QueryString: QueryMap["cost:vm_memory_usage_bytes"],
			MetricKey: staticFields{
				"name":      "name",
				"namespace": "namespace",
			},
			QueryValue: &saveQueryValue{
				ValName:         "vm_memory_usage_bytes",
				Method:          "sum",
				TransformedName: "vm_memory_usage_byte_seconds",
			},
			RowKey: []model.LabelName{"name", "namespace"},
		},
		query{
			Name:        "vm_info",
			QueryString: QueryMap["cost:vm_info"],
			MetricKey: staticFields{
				"node":                "node",
				"provider_id":         "provider_id",
				"name":                "name",
				"namespace":           "namespace",
				"instance_type":       "instance_type",
				"os":                  "os",
				"guest_os_arch":       "guest_os_arch",
				"guest_os_name":       "guest_os_name",
				"guest_os_version_id": "guest_os_version_id",
			},
			QueryValue: &saveQueryValue{
				Method:          "sum",
				TransformedName: "vm_uptime_total_seconds",
			},
			RowKey: []model.LabelName{"name", "namespace"},
		},
		query{
			Name:        "vm_disk_allocated_size",
			QueryString: QueryMap["cost:vm_disk_allocated_size_bytes"],
			MetricKey: staticFields{
				"name":                       "name",
				"namespace":                  "namespace",
				"device":                     "device",
				"volume_mode":                "volume_mode",
				"persistentvolumeclaim_name": "persistentvolumeclaim",
			},
			QueryValue: &saveQueryValue{
				ValName:         "vm_disk_allocated_size_bytes",
				Method:          "max",
				TransformedName: "vm_disk_allocated_size_byte_seconds",
			},
			RowKey: []model.LabelName{"name", "namespace"},
		},
		query{
			Name:        "vm_labels",
			QueryString: QueryMap["cost:vm_labels"],
			MetricKey: staticFields{
				"name":      "name",
				"namespace": "namespace",
			},
			MetricKeyRegex: regexFields{"vm_labels": "label_*"},
			RowKey:         []model.LabelName{"name", "namespace"},
		},
	}
	namespaceQueries = &querys{
		query{
			Name:           "namespace-labels",
			QueryString:    "kube_namespace_labels",
			MetricKey:      staticFields{"namespace": "namespace"},
			MetricKeyRegex: regexFields{"namespace_labels": "label_*"},
			RowKey:         []model.LabelName{"namespace"},
		},
	}
	costNvidiaGpuMemoryCapacityNonMIGQueries = &querys{
		query{
			Name:        "nvidia-gpu-memory-capacity-mib-non-mig",
			QueryString: QueryMap["cost:nvidia_gpu_capacity_memory_mib_non_mig"],
			MetricKey: staticFields{
				"pod":                     "pod",
				"namespace":               "namespace",
				"node":                    "node",
				"gpu_memory_capacity_mib": "label_nvidia_com_gpu_memory",
			},
			RowKey: []model.LabelName{"pod", "namespace", "node"},
		},
	}
	costNvidiaGpuMemoryCapacityMIGQueries = &querys{
		query{
			Name:        "nvidia-gpu-memory-capacity-mib-mig",
			QueryString: QueryMap["cost:nvidia_gpu_capacity_memory_mib_mig"],
			MetricKey: staticFields{
				"pod":                     "exported_pod",
				"namespace":               "exported_namespace",
				"node":                    "Hostname",
				"gpu_memory_capacity_mib": "label_nvidia_com_gpu_memory",
				"mig_strategy":            "label_nvidia_com_mig_strategy",
			},
			RowKey: []model.LabelName{"exported_pod", "exported_namespace", "Hostname", "UUID", "GPU_I_ID"},
		},
	}
	costNvidiaGpuUtilizationQueries = &querys{
		query{
			Name:        "nvidia-gpu-utilization",
			QueryString: QueryMap["cost:nvidia_gpu_utilization"],
			MetricKey: staticFields{
				"node":            "Hostname",
				"namespace":       "exported_namespace",
				"pod":             "exported_pod",
				"gpu_uuid":        "UUID",
				"model_name":      "modelName",
				"mig_instance_id": "GPU_I_ID",
				"mig_profile":     "GPU_I_PROFILE",
				"vendor_name":     "device",
			},
			QueryValue: &saveQueryValue{
				ValName: "nvidia-gpu-pod-utilization",
				Method:  "sum",
			},
			RowKey: []model.LabelName{"exported_pod", "exported_namespace", "Hostname", "UUID", "GPU_I_ID"},
		},
		query{
			Name:        "nvidia-gpu-pod-uptime",
			QueryString: QueryMap["cost:nvidia_gpu_pod_uptime"],
			MetricKey: staticFields{
				"node":            "Hostname",
				"namespace":       "exported_namespace",
				"pod":             "exported_pod",
				"gpu_uuid":        "UUID",
				"model_name":      "modelName",
				"mig_instance_id": "GPU_I_ID",
				"mig_profile":     "GPU_I_PROFILE",
				"vendor_name":     "device",
			},
			QueryValue: &saveQueryValue{
				Method:          "sum",
				TransformedName: "nvidia-gpu-pod-uptime-seconds",
			},
			RowKey: []model.LabelName{"exported_pod", "exported_namespace", "Hostname", "UUID", "GPU_I_ID"},
		},
	}
	costNvidiaGpuMaxSlicesQueries = &querys{
		query{
			Name:        "nvidia-gpu-max-slices",
			QueryString: QueryMap["cost:nvidia_gpu_max_slices"],
			MetricKey: staticFields{
				"node":      "Hostname",
				"namespace": "exported_namespace",
				"pod":       "exported_pod",
			},
			QueryValue: &saveQueryValue{
				Method:  "max",
				ValName: "nvidia-gpu-max-slices",
			},
			RowKey: []model.LabelName{"exported_pod", "exported_namespace", "Hostname", "UUID", "GPU_I_ID"},
		},
	}
	rosContainerQueries = &querys{
		query{
			Name:        "container-image-owner",
			QueryString: QueryMap["ros:image_owners"],
			MetricKey:   staticFields{"image_name": "image", "owner_name": "owner_name", "owner_kind": "owner_kind", "container_name": "container", "pod": "pod", "namespace": "namespace"},
			RowKey:      []model.LabelName{"container", "pod", "namespace"},
		},
		query{
			Name:        "container-image-workload",
			QueryString: QueryMap["ros:image_workloads"],
			MetricKey:   staticFields{"image_name": "image", "workload": "workload", "workload_type": "workload_type", "container_name": "container", "pod": "pod", "namespace": "namespace"},
			RowKey:      []model.LabelName{"container", "pod", "namespace"},
		},
		query{
			Name:        "cpu-request-container-avg",
			QueryString: QueryMap["ros:cpu_request_container_avg"],
			MetricKey:   staticFields{"container_name": "container", "pod": "pod", "namespace": "namespace", "node": "node"},
			QueryValue: &saveQueryValue{
				ValName: "cpu-request-container-avg",
			},
			RowKey: []model.LabelName{"container", "pod", "namespace"},
		},
		query{
			Name:        "cpu-request-container-sum",
			QueryString: QueryMap["ros:cpu_request_container_sum"],
			MetricKey:   staticFields{"container_name": "container", "pod": "pod", "namespace": "namespace", "node": "node"},
			QueryValue: &saveQueryValue{
				ValName: "cpu-request-container-sum",
			},
			RowKey: []model.LabelName{"container", "pod", "namespace"},
		},
		query{
			Name:        "cpu-limit-container-avg",
			QueryString: QueryMap["ros:cpu_limit_container_avg"],
			MetricKey:   staticFields{"container_name": "container", "pod": "pod", "namespace": "namespace", "node": "node"},
			QueryValue: &saveQueryValue{
				ValName: "cpu-limit-container-avg",
			},
			RowKey: []model.LabelName{"container", "pod", "namespace"},
		},
		query{
			Name:        "cpu-limit-container-sum",
			QueryString: QueryMap["ros:cpu_limit_container_sum"],
			MetricKey:   staticFields{"container_name": "container", "pod": "pod", "namespace": "namespace", "node": "node"},
			QueryValue: &saveQueryValue{
				ValName: "cpu-limit-container-sum",
			},
			RowKey: []model.LabelName{"container", "pod", "namespace"},
		},
		query{
			Name:        "cpu-usage-container-avg",
			QueryString: QueryMap["ros:cpu_usage_container_avg"],
			MetricKey:   staticFields{"container_name": "container", "pod": "pod", "namespace": "namespace", "node": "node"},
			QueryValue: &saveQueryValue{
				ValName: "cpu-usage-container-avg",
				Method:  "sum",
			},
			RowKey: []model.LabelName{"container", "pod", "namespace"},
		},
		query{
			Name:        "cpu-usage-container-min",
			QueryString: QueryMap["ros:cpu_usage_container_min"],
			MetricKey:   staticFields{"container_name": "container", "pod": "pod", "namespace": "namespace", "node": "node"},
			QueryValue: &saveQueryValue{
				ValName: "cpu-usage-container-min",
			},
			RowKey: []model.LabelName{"container", "pod", "namespace"},
		},
		query{
			Name:        "cpu-usage-container-max",
			QueryString: QueryMap["ros:cpu_usage_container_max"],
			MetricKey:   staticFields{"container_name": "container", "pod": "pod", "namespace": "namespace", "node": "node"},
			QueryValue: &saveQueryValue{
				ValName: "cpu-usage-container-max",
			},
			RowKey: []model.LabelName{"container", "pod", "namespace"},
		},
		query{
			Name:        "cpu-usage-container-sum",
			QueryString: QueryMap["ros:cpu_usage_container_sum"],
			MetricKey:   staticFields{"container_name": "container", "pod": "pod", "namespace": "namespace", "node": "node"},
			QueryValue: &saveQueryValue{
				ValName: "cpu-usage-container-sum",
			},
			RowKey: []model.LabelName{"container", "pod", "namespace"},
		},
		query{
			Name:        "cpu-throttle-container-avg",
			QueryString: QueryMap["ros:cpu_throttle_container_avg"],
			MetricKey:   staticFields{"container_name": "container", "pod": "pod", "namespace": "namespace", "node": "node"},
			QueryValue: &saveQueryValue{
				ValName: "cpu-throttle-container-avg",
			},
			RowKey: []model.LabelName{"container", "pod", "namespace"},
		},
		query{
			Name:        "cpu-throttle-container-max",
			QueryString: QueryMap["ros:cpu_throttle_container_max"],
			MetricKey:   staticFields{"container_name": "container", "pod": "pod", "namespace": "namespace", "node": "node"},
			QueryValue: &saveQueryValue{
				ValName: "cpu-throttle-container-max",
			},
			RowKey: []model.LabelName{"container", "pod", "namespace"},
		},
		query{
			Name:        "cpu-throttle-container-min",
			QueryString: QueryMap["ros:cpu_throttle_container_min"],
			MetricKey:   staticFields{"container_name": "container", "pod": "pod", "namespace": "namespace", "node": "node"},
			QueryValue: &saveQueryValue{
				ValName: "cpu-throttle-container-min",
			},
			RowKey: []model.LabelName{"container", "pod", "namespace"},
		},
		query{
			Name:        "cpu-throttle-container-sum",
			QueryString: QueryMap["ros:cpu_throttle_container_sum"],
			MetricKey:   staticFields{"container_name": "container", "pod": "pod", "namespace": "namespace", "node": "node"},
			QueryValue: &saveQueryValue{
				ValName: "cpu-throttle-container-sum",
			},
			RowKey: []model.LabelName{"container", "pod", "namespace"},
		},
		query{
			Name:        "memory-request-container-avg",
			QueryString: QueryMap["ros:memory_request_container_avg"],
			MetricKey:   staticFields{"container_name": "container", "pod": "pod", "namespace": "namespace", "node": "node"},
			QueryValue: &saveQueryValue{
				ValName: "memory-request-container-avg",
			},
			RowKey: []model.LabelName{"container", "pod", "namespace"},
		},
		query{
			Name:        "memory-request-container-sum",
			QueryString: QueryMap["ros:memory_request_container_sum"],
			MetricKey:   staticFields{"container_name": "container", "pod": "pod", "namespace": "namespace", "node": "node"},
			QueryValue: &saveQueryValue{
				ValName: "memory-request-container-sum",
			},
			RowKey: []model.LabelName{"container", "pod", "namespace"},
		},
		query{
			Name:        "memory-limit-container-avg",
			QueryString: QueryMap["ros:memory_limit_container_avg"],
			MetricKey:   staticFields{"container_name": "container", "pod": "pod", "namespace": "namespace", "node": "node"},
			QueryValue: &saveQueryValue{
				ValName: "memory-limit-container-avg",
			},
			RowKey: []model.LabelName{"container", "pod", "namespace"},
		},
		query{
			Name:        "memory-limit-container-sum",
			QueryString: QueryMap["ros:memory_limit_container_sum"],
			MetricKey:   staticFields{"container_name": "container", "pod": "pod", "namespace": "namespace", "node": "node"},
			QueryValue: &saveQueryValue{
				ValName: "memory-limit-container-sum",
			},
			RowKey: []model.LabelName{"container", "pod", "namespace"},
		},
		query{
			Name:        "memory-usage-container-avg",
			QueryString: QueryMap["ros:memory_usage_container_avg"],
			MetricKey:   staticFields{"container_name": "container", "pod": "pod", "namespace": "namespace", "node": "node"},
			QueryValue: &saveQueryValue{
				ValName: "memory-usage-container-avg",
			},
			RowKey: []model.LabelName{"container", "pod", "namespace"},
		},
		query{
			Name:        "memory-usage-container-min",
			QueryString: QueryMap["ros:memory_usage_container_min"],
			MetricKey:   staticFields{"container_name": "container", "pod": "pod", "namespace": "namespace", "node": "node"},
			QueryValue: &saveQueryValue{
				ValName: "memory-usage-container-min",
			},
			RowKey: []model.LabelName{"container", "pod", "namespace"},
		},
		query{
			Name:        "memory-usage-container-max",
			QueryString: QueryMap["ros:memory_usage_container_max"],
			MetricKey:   staticFields{"container_name": "container", "pod": "pod", "namespace": "namespace", "node": "node"},
			QueryValue: &saveQueryValue{
				ValName: "memory-usage-container-max",
			},
			RowKey: []model.LabelName{"container", "pod", "namespace"},
		},
		query{
			Name:        "memory-usage-container-sum",
			QueryString: QueryMap["ros:memory_usage_container_sum"],
			MetricKey:   staticFields{"container_name": "container", "pod": "pod", "namespace": "namespace", "node": "node"},
			QueryValue: &saveQueryValue{
				ValName: "memory-usage-container-sum",
			},
			RowKey: []model.LabelName{"container", "pod", "namespace"},
		},
		query{
			Name:        "memory-rss-usage-container-avg",
			QueryString: QueryMap["ros:memory_rss_usage_container_avg"],
			MetricKey:   staticFields{"container_name": "container", "pod": "pod", "namespace": "namespace", "node": "node"},
			QueryValue: &saveQueryValue{
				ValName: "memory-rss-usage-container-avg",
			},
			RowKey: []model.LabelName{"container", "pod", "namespace"},
		},
		query{
			Name:        "memory-rss-usage-container-min",
			QueryString: QueryMap["ros:memory_rss_usage_container_min"],
			MetricKey:   staticFields{"container_name": "container", "pod": "pod", "namespace": "namespace", "node": "node"},
			QueryValue: &saveQueryValue{
				ValName: "memory-rss-usage-container-min",
			},
			RowKey: []model.LabelName{"container", "pod", "namespace"},
		},
		query{
			Name:        "memory-rss-usage-container-max",
			QueryString: QueryMap["ros:memory_rss_usage_container_max"],
			MetricKey:   staticFields{"container_name": "container", "pod": "pod", "namespace": "namespace", "node": "node"},
			QueryValue: &saveQueryValue{
				ValName: "memory-rss-usage-container-max",
			},
			RowKey: []model.LabelName{"container", "pod", "namespace"},
		},
		query{
			Name:        "memory-rss-usage-container-sum",
			QueryString: QueryMap["ros:memory_rss_usage_container_sum"],
			MetricKey:   staticFields{"container_name": "container", "pod": "pod", "namespace": "namespace", "node": "node"},
			QueryValue: &saveQueryValue{
				ValName: "memory-rss-usage-container-sum",
			},
			RowKey: []model.LabelName{"container", "pod", "namespace"},
		},
		query{
			Name:        "oom-count-container-sum",
			QueryString: QueryMap["ros:oom_count_container_sum"],
			MetricKey:   staticFields{"container_name": "container", "pod": "pod", "namespace": "namespace"},
			QueryValue: &saveQueryValue{
				ValName: "oom-count",
			},
			RowKey: []model.LabelName{"container", "pod", "namespace"},
		},
		query{
			Name:        "workload-pod-count",
			QueryString: QueryMap["ros:workload_pod_count"],
			MetricKey:   staticFields{"container_name": "container", "pod": "pod", "namespace": "namespace"},
			QueryValue: &saveQueryValue{
				ValName: "workload-pod-count",
			},
			RowKey: []model.LabelName{"container", "pod", "namespace"},
		},
		query{
			Name:        "desired-replicas",
			QueryString: QueryMap["ros:desired_replicas"],
			MetricKey:   staticFields{"container_name": "container", "pod": "pod", "namespace": "namespace"},
			QueryValue: &saveQueryValue{
				ValName: "desired-replicas",
			},
			RowKey: []model.LabelName{"container", "pod", "namespace"},
		},
		query{
			Name:        "available-replicas",
			QueryString: QueryMap["ros:available_replicas"],
			MetricKey:   staticFields{"container_name": "container", "pod": "pod", "namespace": "namespace"},
			QueryValue: &saveQueryValue{
				ValName: "available-replicas",
			},
			RowKey: []model.LabelName{"container", "pod", "namespace"},
		},
		query{
			Name:        "accelerator-frame-buffer-usage-min",
			QueryString: QueryMap["ros:accelerator_frame_buffer_usage_min"],
			MetricKey: staticFields{
				"accelerator_model_name":   "modelName",
				"container":                "exported_container",
				"namespace":                "exported_namespace",
				"pod":                      "exported_pod",
				"node":                     "Hostname",
				"accelerator_profile_name": "GPU_I_PROFILE",
			},
			QueryValue: &saveQueryValue{
				ValName: "accelerator-frame-buffer-usage-min",
			},
			RowKey: []model.LabelName{"exported_container", "exported_pod", "exported_namespace"},
		},
		query{
			Name:        "accelerator-frame-buffer-usage-max",
			QueryString: QueryMap["ros:accelerator_frame_buffer_usage_max"],
			MetricKey: staticFields{
				"accelerator_model_name":   "modelName",
				"container":                "exported_container",
				"namespace":                "exported_namespace",
				"pod":                      "exported_pod",
				"node":                     "Hostname",
				"accelerator_profile_name": "GPU_I_PROFILE",
			},
			QueryValue: &saveQueryValue{
				ValName: "accelerator-frame-buffer-usage-max",
			},
			RowKey: []model.LabelName{"exported_container", "exported_pod", "exported_namespace"},
		},
		query{
			Name:        "accelerator-frame-buffer-usage-avg",
			QueryString: QueryMap["ros:accelerator_frame_buffer_usage_avg"],
			MetricKey: staticFields{
				"accelerator_model_name":   "modelName",
				"container":                "exported_container",
				"namespace":                "exported_namespace",
				"pod":                      "exported_pod",
				"node":                     "Hostname",
				"accelerator_profile_name": "GPU_I_PROFILE",
			},
			QueryValue: &saveQueryValue{
				ValName: "accelerator-frame-buffer-usage-avg",
			},
			RowKey: []model.LabelName{"exported_container", "exported_pod", "exported_namespace"},
		},
		query{
			Name:        "tensor-pipe-active-min",
			QueryString: QueryMap["ros:tensor_pipe_active_min"],
			MetricKey: staticFields{
				"accelerator_model_name":   "modelName",
				"container":                "exported_container",
				"namespace":                "exported_namespace",
				"pod":                      "exported_pod",
				"node":                     "Hostname",
				"accelerator_profile_name": "GPU_I_PROFILE",
			},
			QueryValue: &saveQueryValue{
				ValName: "tensor-pipe-active-min",
			},
			RowKey: []model.LabelName{"exported_container", "exported_pod", "exported_namespace"},
		},
		query{
			Name:        "tensor-pipe-active-max",
			QueryString: QueryMap["ros:tensor_pipe_active_max"],
			MetricKey: staticFields{
				"accelerator_model_name":   "modelName",
				"container":                "exported_container",
				"namespace":                "exported_namespace",
				"pod":                      "exported_pod",
				"node":                     "Hostname",
				"accelerator_profile_name": "GPU_I_PROFILE",
			},
			QueryValue: &saveQueryValue{
				ValName: "tensor-pipe-active-max",
			},
			RowKey: []model.LabelName{"exported_container", "exported_pod", "exported_namespace"},
		},
		query{
			Name:        "tensor-pipe-active-avg",
			QueryString: QueryMap["ros:tensor_pipe_active_avg"],
			MetricKey: staticFields{
				"accelerator_model_name":   "modelName",
				"container":                "exported_container",
				"namespace":                "exported_namespace",
				"pod":                      "exported_pod",
				"node":                     "Hostname",
				"accelerator_profile_name": "GPU_I_PROFILE",
			},
			QueryValue: &saveQueryValue{
				ValName: "tensor-pipe-active-avg",
			},
			RowKey: []model.LabelName{"exported_container", "exported_pod", "exported_namespace"},
		},
		query{
			Name:        "dram-active-min",
			QueryString: QueryMap["ros:dram_active_min"],
			MetricKey: staticFields{
				"accelerator_model_name":   "modelName",
				"container":                "exported_container",
				"namespace":                "exported_namespace",
				"pod":                      "exported_pod",
				"node":                     "Hostname",
				"accelerator_profile_name": "GPU_I_PROFILE",
			},
			QueryValue: &saveQueryValue{
				ValName: "dram-active-min",
			},
			RowKey: []model.LabelName{"exported_container", "exported_pod", "exported_namespace"},
		},
		query{
			Name:        "dram-active-max",
			QueryString: QueryMap["ros:dram_active_max"],
			MetricKey: staticFields{
				"accelerator_model_name":   "modelName",
				"container":                "exported_container",
				"namespace":                "exported_namespace",
				"pod":                      "exported_pod",
				"node":                     "Hostname",
				"accelerator_profile_name": "GPU_I_PROFILE",
			},
			QueryValue: &saveQueryValue{
				ValName: "dram-active-max",
			},
			RowKey: []model.LabelName{"exported_container", "exported_pod", "exported_namespace"},
		},
		query{
			Name:        "dram-active-avg",
			QueryString: QueryMap["ros:dram_active_avg"],
			MetricKey: staticFields{
				"accelerator_model_name":   "modelName",
				"container":                "exported_container",
				"namespace":                "exported_namespace",
				"pod":                      "exported_pod",
				"node":                     "Hostname",
				"accelerator_profile_name": "GPU_I_PROFILE",
			},
			QueryValue: &saveQueryValue{
				ValName: "dram-active-avg",
			},
			RowKey: []model.LabelName{"exported_container", "exported_pod", "exported_namespace"},
		},
		query{
			Name:        "sm-active-min",
			QueryString: QueryMap["ros:sm_active_min"],
			MetricKey: staticFields{
				"accelerator_model_name":   "modelName",
				"container":                "exported_container",
				"namespace":                "exported_namespace",
				"pod":                      "exported_pod",
				"node":                     "Hostname",
				"accelerator_profile_name": "GPU_I_PROFILE",
			},
			QueryValue: &saveQueryValue{
				ValName: "sm-active-min",
			},
			RowKey: []model.LabelName{"exported_container", "exported_pod", "exported_namespace"},
		},
		query{
			Name:        "sm-active-max",
			QueryString: QueryMap["ros:sm_active_max"],
			MetricKey: staticFields{
				"accelerator_model_name":   "modelName",
				"container":                "exported_container",
				"namespace":                "exported_namespace",
				"pod":                      "exported_pod",
				"node":                     "Hostname",
				"accelerator_profile_name": "GPU_I_PROFILE",
			},
			QueryValue: &saveQueryValue{
				ValName: "sm-active-max",
			},
			RowKey: []model.LabelName{"exported_container", "exported_pod", "exported_namespace"},
		},
		query{
			Name:        "sm-active-avg",
			QueryString: QueryMap["ros:sm_active_avg"],
			MetricKey: staticFields{
				"accelerator_model_name":   "modelName",
				"container":                "exported_container",
				"namespace":                "exported_namespace",
				"pod":                      "exported_pod",
				"node":                     "Hostname",
				"accelerator_profile_name": "GPU_I_PROFILE",
			},
			QueryValue: &saveQueryValue{
				ValName: "sm-active-avg",
			},
			RowKey: []model.LabelName{"exported_container", "exported_pod", "exported_namespace"},
		},
	}

	rosNamespaceQueries = &querys{
		query{
			Name:        "cpu-request-namespace-sum",
			QueryString: QueryMap["ros:cpu_request_namespace_sum"],
			MetricKey:   staticFields{"namespace": "namespace"},
			QueryValue: &saveQueryValue{
				ValName: "cpu-request-namespace-sum",
			},
			RowKey: []model.LabelName{"namespace"},
		},
		query{
			Name:        "cpu-request-namespace-used",
			QueryString: QueryMap["ros:cpu_request_namespace_used"],
			MetricKey:   staticFields{"namespace": "namespace"},
			QueryValue: &saveQueryValue{
				ValName: "cpu-request-namespace-used",
			},
			RowKey: []model.LabelName{"namespace"},
		},
		query{
			Name:        "cpu-limit-namespace-sum",
			QueryString: QueryMap["ros:cpu_limit_namespace_sum"],
			MetricKey:   staticFields{"namespace": "namespace"},
			QueryValue: &saveQueryValue{
				ValName: "cpu-limit-namespace-sum",
			},
			RowKey: []model.LabelName{"namespace"},
		},
		query{
			Name:        "cpu-limit-namespace-used",
			QueryString: QueryMap["ros:cpu_limit_namespace_used"],
			MetricKey:   staticFields{"namespace": "namespace"},
			QueryValue: &saveQueryValue{
				ValName: "cpu-limit-namespace-used",
			},
			RowKey: []model.LabelName{"namespace"},
		},
		query{
			Name:        "cpu-usage-namespace-avg",
			QueryString: QueryMap["ros:cpu_usage_namespace_avg"],
			MetricKey:   staticFields{"namespace": "namespace"},
			QueryValue: &saveQueryValue{
				ValName: "cpu-usage-namespace-avg",
			},
			RowKey: []model.LabelName{"namespace"},
		},
		query{
			Name:        "cpu-usage-namespace-max",
			QueryString: QueryMap["ros:cpu_usage_namespace_max"],
			MetricKey:   staticFields{"namespace": "namespace"},
			QueryValue: &saveQueryValue{
				ValName: "cpu-usage-namespace-max",
			},
			RowKey: []model.LabelName{"namespace"},
		},
		query{
			Name:        "cpu-usage-namespace-min",
			QueryString: QueryMap["ros:cpu_usage_namespace_min"],
			MetricKey:   staticFields{"namespace": "namespace"},
			QueryValue: &saveQueryValue{
				ValName: "cpu-usage-namespace-min",
			},
			RowKey: []model.LabelName{"namespace"},
		},
		query{
			Name:        "cpu-throttle-namespace-avg",
			QueryString: QueryMap["ros:cpu_throttle_namespace_avg"],
			MetricKey:   staticFields{"namespace": "namespace"},
			QueryValue: &saveQueryValue{
				ValName: "cpu-throttle-namespace-avg",
			},
			RowKey: []model.LabelName{"namespace"},
		},
		query{
			Name:        "cpu-throttle-namespace-max",
			QueryString: QueryMap["ros:cpu_throttle_namespace_max"],
			MetricKey:   staticFields{"namespace": "namespace"},
			QueryValue: &saveQueryValue{
				ValName: "cpu-throttle-namespace-max",
			},
			RowKey: []model.LabelName{"namespace"},
		},
		query{
			Name:        "cpu-throttle-namespace-min",
			QueryString: QueryMap["ros:cpu_throttle_namespace_min"],
			MetricKey:   staticFields{"namespace": "namespace"},
			QueryValue: &saveQueryValue{
				ValName: "cpu-throttle-namespace-min",
			},
			RowKey: []model.LabelName{"namespace"},
		},
		query{
			Name:        "memory-request-namespace-sum",
			QueryString: QueryMap["ros:memory_request_namespace_sum"],
			MetricKey:   staticFields{"namespace": "namespace"},
			QueryValue: &saveQueryValue{
				ValName: "memory-request-namespace-sum",
			},
			RowKey: []model.LabelName{"namespace"},
		},
		query{
			Name:        "memory-request-namespace-used",
			QueryString: QueryMap["ros:memory_request_namespace_used"],
			MetricKey:   staticFields{"namespace": "namespace"},
			QueryValue: &saveQueryValue{
				ValName: "memory-request-namespace-used",
			},
			RowKey: []model.LabelName{"namespace"},
		},
		query{
			Name:        "memory-limit-namespace-sum",
			QueryString: QueryMap["ros:memory_limit_namespace_sum"],
			MetricKey:   staticFields{"namespace": "namespace"},
			QueryValue: &saveQueryValue{
				ValName: "memory-limit-namespace-sum",
			},
			RowKey: []model.LabelName{"namespace"},
		},
		query{
			Name:        "memory-limit-namespace-used",
			QueryString: QueryMap["ros:memory_limit_namespace_used"],
			MetricKey:   staticFields{"namespace": "namespace"},
			QueryValue: &saveQueryValue{
				ValName: "memory-limit-namespace-used",
			},
			RowKey: []model.LabelName{"namespace"},
		},
		query{
			Name:        "memory-usage-namespace-avg",
			QueryString: QueryMap["ros:memory_usage_namespace_avg"],
			MetricKey:   staticFields{"namespace": "namespace"},
			QueryValue: &saveQueryValue{
				ValName: "memory-usage-namespace-avg",
			},
			RowKey: []model.LabelName{"namespace"},
		},
		query{
			Name:        "memory-usage-namespace-max",
			QueryString: QueryMap["ros:memory_usage_namespace_max"],
			MetricKey:   staticFields{"namespace": "namespace"},
			QueryValue: &saveQueryValue{
				ValName: "memory-usage-namespace-max",
			},
			RowKey: []model.LabelName{"namespace"},
		},
		query{
			Name:        "memory-usage-namespace-min",
			QueryString: QueryMap["ros:memory_usage_namespace_min"],
			MetricKey:   staticFields{"namespace": "namespace"},
			QueryValue: &saveQueryValue{
				ValName: "memory-usage-namespace-min",
			},
			RowKey: []model.LabelName{"namespace"},
		},
		query{
			Name:        "memory-rss-usage-namespace-avg",
			QueryString: QueryMap["ros:memory_rss_usage_namespace_avg"],
			MetricKey:   staticFields{"namespace": "namespace"},
			QueryValue: &saveQueryValue{
				ValName: "memory-rss-usage-namespace-avg",
			},
			RowKey: []model.LabelName{"namespace"},
		},
		query{
			Name:        "memory-rss-usage-namespace-max",
			QueryString: QueryMap["ros:memory_rss_usage_namespace_max"],
			MetricKey:   staticFields{"namespace": "namespace"},
			QueryValue: &saveQueryValue{
				ValName: "memory-rss-usage-namespace-max",
			},
			RowKey: []model.LabelName{"namespace"},
		},
		query{
			Name:        "memory-rss-usage-namespace-min",
			QueryString: QueryMap["ros:memory_rss_usage_namespace_min"],
			MetricKey:   staticFields{"namespace": "namespace"},
			QueryValue: &saveQueryValue{
				ValName: "memory-rss-usage-namespace-min",
			},
			RowKey: []model.LabelName{"namespace"},
		},
		query{
			Name:        "pods-running-namespace-max",
			QueryString: QueryMap["ros:namespace_running_pods_max"],
			MetricKey:   staticFields{"namespace": "namespace"},
			QueryValue: &saveQueryValue{
				ValName: "pods-running-namespace-max",
			},
			RowKey: []model.LabelName{"namespace"},
		},
		query{
			Name:        "pods-running-namespace-avg",
			QueryString: QueryMap["ros:namespace_running_pods_avg"],
			MetricKey:   staticFields{"namespace": "namespace"},
			QueryValue: &saveQueryValue{
				ValName: "pods-running-namespace-avg",
			},
			RowKey: []model.LabelName{"namespace"},
		},
		query{
			Name:        "pods-total-namespace-max",
			QueryString: QueryMap["ros:namespace_total_pods_max"],
			MetricKey:   staticFields{"namespace": "namespace"},
			QueryValue: &saveQueryValue{
				ValName: "pods-total-namespace-max",
			},
			RowKey: []model.LabelName{"namespace"},
		},
		query{
			Name:        "pods-total-namespace-avg",
			QueryString: QueryMap["ros:namespace_total_pods_avg"],
			MetricKey:   staticFields{"namespace": "namespace"},
			QueryValue: &saveQueryValue{
				ValName: "pods-total-namespace-avg",
			},
			RowKey: []model.LabelName{"namespace"},
		},
	}

	rosClusterQuotaQueries = &querys{
		query{
			Name:        "cluster-quota-cpu-request-hard",
			QueryString: QueryMap["ros:cluster_quota_cpu_request_hard"],
			MetricKey:   staticFields{"cluster_quota_name": "name"},
			QueryValue: &saveQueryValue{
				ValName: "cpu-request-hard",
			},
			RowKey: []model.LabelName{"name"},
		},
		query{
			Name:        "cluster-quota-cpu-request-used",
			QueryString: QueryMap["ros:cluster_quota_cpu_request_used"],
			MetricKey:   staticFields{"cluster_quota_name": "name"},
			QueryValue: &saveQueryValue{
				ValName: "cpu-request-used",
			},
			RowKey: []model.LabelName{"name"},
		},
		query{
			Name:        "cluster-quota-cpu-limit-hard",
			QueryString: QueryMap["ros:cluster_quota_cpu_limit_hard"],
			MetricKey:   staticFields{"cluster_quota_name": "name"},
			QueryValue: &saveQueryValue{
				ValName: "cpu-limit-hard",
			},
			RowKey: []model.LabelName{"name"},
		},
		query{
			Name:        "cluster-quota-cpu-limit-used",
			QueryString: QueryMap["ros:cluster_quota_cpu_limit_used"],
			MetricKey:   staticFields{"cluster_quota_name": "name"},
			QueryValue: &saveQueryValue{
				ValName: "cpu-limit-used",
			},
			RowKey: []model.LabelName{"name"},
		},
		query{
			Name:        "cluster-quota-memory-request-hard",
			QueryString: QueryMap["ros:cluster_quota_memory_request_hard"],
			MetricKey:   staticFields{"cluster_quota_name": "name"},
			QueryValue: &saveQueryValue{
				ValName: "memory-request-hard",
			},
			RowKey: []model.LabelName{"name"},
		},
		query{
			Name:        "cluster-quota-memory-request-used",
			QueryString: QueryMap["ros:cluster_quota_memory_request_used"],
			MetricKey:   staticFields{"cluster_quota_name": "name"},
			QueryValue: &saveQueryValue{
				ValName: "memory-request-used",
			},
			RowKey: []model.LabelName{"name"},
		},
		query{
			Name:        "cluster-quota-memory-limit-hard",
			QueryString: QueryMap["ros:cluster_quota_memory_limit_hard"],
			MetricKey:   staticFields{"cluster_quota_name": "name"},
			QueryValue: &saveQueryValue{
				ValName: "memory-limit-hard",
			},
			RowKey: []model.LabelName{"name"},
		},
		query{
			Name:        "cluster-quota-memory-limit-used",
			QueryString: QueryMap["ros:cluster_quota_memory_limit_used"],
			MetricKey:   staticFields{"cluster_quota_name": "name"},
			QueryValue: &saveQueryValue{
				ValName: "memory-limit-used",
			},
			RowKey: []model.LabelName{"name"},
		},
	}
)

type querys []query

type query struct {
	Name           string
	QueryString    string
	MetricKey      staticFields
	MetricKeyRegex regexFields
	QueryValue     *saveQueryValue
	RowKey         []model.LabelName
}

type staticFields map[string]model.LabelName

type regexFields map[string]string

type saveQueryValue struct {
	ValName         string
	Method          string
	TransformedName string
}
