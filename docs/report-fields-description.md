# Report Fields For Collected Metrics

This document provides an outline of the fields included in the collected usage metrics. These metrics relate to containers, persistent volumes, nodes, pods, and namespaces.


**NOTE:**

* The [Prometheus queries](https://github.com/project-koku/koku-metrics-operator/blob/main/internal/collector/queries.go) that the operator uses to collect metrics are detailed in the linked file.

* To enable the collection ROS (Resource Optimization) metrics, ensure that the namespaces are labeled with `cost_management_optimizations='true'`. Note in operator versions below 4.1.0 you must use the label `insights_cost_management_optimizations='true'`.

    * Queries responsible for collecting ROS metrics are identified in the `QueryMap` with the prefix `ros:` and include a specific filter to target the appropriately labeled namespaces:
        ```
        kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'}
        ```


### Common Fields

* `report_period_start`: The start timestamp of the reporting period.
* `report_period_end`: The end timestamp of the reporting period.
* `interval_start`: The start timestamp of the reporting interval.
* `interval_end`: The end timestamp of the reporting interval.

These common fields are included in all the reports and provide temporal information about the reporting period and interval.


## Cost Management Reports:


### 1. Node Metrics

Fields for metrics related to nodes:

* `node`: The name of the node.
* `node_labels`: The labels associated with the node.


### 2. Namespace Metrics

Fields for metrics related to namespaces:

* `namespace`: The namespace.
* `namespace_labels`: The labels associated with the namespace.


### 3. Pod Metrics

Fields for metrics related to pods:

* `node`: The name of the node.
* `namespace`: The namespace of the pod.
* `pod`: The name of the pod.
* `pod_usage_cpu_core_seconds`: The CPU core seconds used by the pod.
* `pod_request_cpu_core_seconds`: The CPU core seconds requested by the pod.
* `pod_limit_cpu_core_seconds`: The CPU core seconds limited for the pod.
* `pod_usage_memory_byte_seconds`: The memory byte seconds used by the pod.
* `pod_request_memory_byte_seconds`: The memory byte seconds requested by the pod.
* `pod_limit_memory_byte_seconds`: The memory byte seconds limited for the pod.
* `node_capacity_cpu_cores`: The CPU cores capacity of the node.
* `node_capacity_cpu_core_seconds`: The CPU core seconds capacity of the node.
* `node_capacity_memory_bytes`: The memory bytes capacity of the node.
* `node_capacity_memory_byte_seconds`: The memory byte seconds capacity of the node.
* `node_role`: The role of the node.
* `resource_id`: The unique identifier of the resource.
* `pod_labels`: The labels associated with the pod.


### 4. Virtual Machine Metrics

Fields for metrics related to running virtual machines (VMs) and their associated resources:

* `node`: The name of the node where the virtual machine instance (VMI) is currently running.
* `namespace`: The namespace where the virtual machine (VM) is defined.
* `resource_id`: The unique identifier of the VM on the node. This is derived from the `provider_id` of the node, specifically the segment after the last `/` character. For example, if the `provider_id` is `aws:///us-east-1a/i-0abcdef1234567890`, the `resource_id` is `i-0abcdef1234567890`.
* `vm_name`: The name of the VM.
* `vm_instance_type`: The instance type associated with the VM, if defined.
* `vm_os`: The operating system reported by the VM's info.
* `vm_guest_os_arch`: The guest operating system architecture reported by the VM. For example x86_64.
* `vm_guest_os_name`: The guest operating system name reported by the VM. For example RHEL or Fedora.
* `vm_guest_os_version`: The guest operating system version number reported by the VM. For example 8.6.
* `vm_uptime_total_seconds`: The total uptime of the VMI in seconds since it started.
* `vm_cpu_limit_cores`: The CPU core limit configured, representing the maximum number of cores the VM can use.
* `vm_cpu_limit_core_seconds`: The total CPU core seconds limited for the VM over the reporting period.
* `vm_cpu_request_cores`: The CPU core request configured for the VM, representing the guaranteed number of cores the VM will receive.
* `vm_cpu_request_core_seconds`: The total CPU core seconds requested for the VM over the reporting period.
* `vm_cpu_request_sockets`: The number of CPU sockets requested for the VM.
* `vm_cpu_request_socket_seconds`: The total CPU socket seconds requested for the VM over the reporting period.
* `vm_cpu_request_threads`: The number of CPU threads requested per core for the VM.
* `vm_cpu_request_thread_seconds`: The total CPU thread seconds requested for the VM over the reporting period.
* `vm_cpu_usage_total_seconds`: The total CPU usage of the VM in seconds over the reporting period.
* `vm_memory_limit_bytes`: The memory limit configured for the VM in bytes, representing the maximum memory the VM can use.
* `vm_memory_limit_byte_seconds`: The total memory byte seconds limited for the VM over the reporting period.
* `vm_memory_request_bytes`: The memory request configured for the VM in bytes, representing the guaranteed memory the VM will receive.
* `vm_memory_request_byte_seconds`: The total memory byte seconds requested for the VM over the reporting period.
* `vm_memory_usage_byte_seconds`: The total memory usage of the VM in byte seconds over the reporting period.
* `vm_device`: The name of the virtual device attached to the VM, typically referring to a disk.
* `vm_volume_mode`: The volume mode of the attached disk. For example Block or Filesystem.
* `vm_persistentvolumeclaim_name`: The name of the `PersistentVolumeClaim` backing the VM's disk.
* `vm_disk_allocated_size_byte_seconds`: The total allocated disk size for the VM's storage in byte seconds over the reporting period.
* `vm_labels`: A JSON string representing key-value pairs of labels applied to the VM.


### 5. Persistent Volume Metrics

Fields for metrics related to Persistent Volumes (PVs):

* `namespace`: The namespace associated with the persistent volume claim (PVC).
* `pod`: The name of the pod associated with the persistent volume claim.
* `persistentvolumeclaim`: The name of the persistent volume claim.
* `persistentvolume`: The name of the persistent volume.
* `storageclass`: The storage class of the persistent volume claim.
* `persistentvolumeclaim_capacity_bytes`: The capacity of the persistent volume claim in bytes.
* `persistentvolumeclaim_capacity_byte_seconds`: The capacity of the persistent volume claim in byte seconds.
* `volume_request_storage_byte_seconds`: The storage byte seconds requested by the volume.
* `persistentvolumeclaim_usage_byte_seconds`: The usage byte seconds of the persistent volume claim.
* `persistentvolume_labels`: The labels associated with the persistent volume.
* `persistentvolumeclaim_labels`: The labels associated with the persistent volume claim.

### 6. NVIDIA GPU Metrics

Fields for metrics related to NVIDIA GPUs:

* `node`: The name of the node where the pod is running.
* `namespace`: The namespace where the pod is running.
* `pod`: The name of the pod that is consuming GPU resources.
* `gpu_uuid`: The unique identifier for the physical NVIDIA GPU card.
* `gpu_model_name`: The model name of the GPU (e.g., Tesla T4).
* `gpu_vendor_name`: The vendor of the GPU reported by telemetry (for example, `nvidia0`).
* `gpu_memory_capacity_mib`: The total memory capacity of the GPU in Mebibytes (MiB).
* `gpu_pod_uptime`: The cumulative number of seconds the pod has been running on the specific GPU during the report period.
* `gpu_pod_utilization`: The sum of GPU compute engine utilization samples (from DCGM_FI_PROF_GR_ENGINE_ACTIVE) for the pod during the report period.
* `gpu_max_slices`: The raw maximum number of MIG slices available for the GPU profile. This field is optional and may be empty if the metric is not exposed by DCGM exporter configuration.
* `mig_instance_id`: The MIG instance ID used by the workload. Empty for non-MIG workloads.
* `mig_profile`: The MIG profile (for example, `1g.5gb`) associated with the workload. Empty for non-MIG workloads.
* `mig_strategy`: The node MIG strategy label (for example, `single`, `mixed`, or `none`) when available.

## Resource Optimization (ROS) Reports:

### 1. Container Metrics

Fields for metrics related to containers:

* `container_name`: The name of the container.
* `pod`: The name of the pod that associated with the container.
* `owner_name`: The name of the owner entity that is associated with the container. For example Deployment or StatefulSet.
* `owner_kind`: The kind of the owner entity that is associated with the container. For example Deployment or StatefulSet.
* `workload`: The workload associated with the container.
* `workload_type`: The type of the workload.
* `namespace`: The namespace of the container.
* `image_name`: The name of the container's image.
* `node`: The node on which the container is running.
* `instance_type`: Node instance type from `kube_node_labels` (e.g. `label_node_kubernetes_io_instance_type`); used by ros-ocp-backend for Level 3 node consolidation grouping.
* `node_allocatable_cpu_cores`: Node allocatable CPU cores from `kube_node_status_allocatable{resource='cpu'}` (preferred over capacity for utilization ratios).
* `node_allocatable_memory_bytes`: Node allocatable memory bytes from `kube_node_status_allocatable{resource='memory'}`.
* `resource_id`: The unique identifier of the resource.
* `cpu_request_container_avg`: The average CPU request for the container.
* `cpu_request_container_sum`: The total CPU request for the container.
* `cpu_limit_container_avg`: The average CPU limit for the container.
* `cpu_limit_container_sum`: The total CPU limit for the container.
* `cpu_usage_container_avg`: The average CPU usage for the container.
* `cpu_usage_container_min`: The minimum CPU usage for the container.
* `cpu_usage_container_max`: The maximum CPU usage for the container.
* `cpu_usage_container_sum`: The total CPU usage for the container.
* `cpu_throttle_container_avg`: The average CPU throttle for the container.
* `cpu_throttle_container_max`: The maximum CPU throttle for the container.
* `cpu_throttle_container_min`: The minimum CPU throttle for the container.
* `cpu_throttle_container_sum`: The total CPU throttle for the container.
* `memory_request_container_avg`: The average memory request for the container.
* `memory_request_container_sum`: The total memory request for the container.
* `memory_limit_container_avg`: The average memory limit for the container.
* `memory_limit_container_sum`: The total memory limit for the container.
* `memory_usage_container_avg`: The average memory usage for the container.
* `memory_usage_container_min`: The minimum memory usage for the container.
* `memory_usage_container_max`: The maximum memory usage for the container.
* `memory_usage_container_sum`: The total memory usage for the container.
* `memory_rss_usage_container_avg`: The average RSS memory usage for the container.
* `memory_rss_usage_container_min`: The minimum RSS memory usage for the container.
* `memory_rss_usage_container_max`: The maximum RSS memory usage for the container.
* `memory_rss_usage_container_sum`: The total RSS memory usage for the container.
* `accelerator_model_name`: The GPU Model which the workload is utilising.
* `accelerator_profile_name`: The GPU partition which the workload is utilising.
* `tensor_pipe_active_min`: The minimum tensor core pipe activity ratio (0.0-1.0) for a container. Requires Turing+ datacenter GPU with DCGM profiling metrics.
* `tensor_pipe_active_max`: The maximum tensor core pipe activity ratio (0.0-1.0) for a container.
* `tensor_pipe_active_avg`: The average tensor core pipe activity ratio (0.0-1.0) for a container.
* `dram_active_min`: The minimum DRAM bandwidth activity ratio (0.0-1.0) for a container.
* `dram_active_max`: The maximum DRAM bandwidth activity ratio (0.0-1.0) for a container.
* `dram_active_avg`: The average DRAM bandwidth activity ratio (0.0-1.0) for a container.
* `sm_active_min`: The minimum streaming multiprocessor activity ratio (0.0-1.0) for a container.
* `sm_active_max`: The maximum streaming multiprocessor activity ratio (0.0-1.0) for a container.
* `sm_active_avg`: The average streaming multiprocessor activity ratio (0.0-1.0) for a container.
* `accelerator_frame_buffer_usage_min`: The minimum GPU frame buffer usage in bytes for a container.
* `accelerator_frame_buffer_usage_max`: The maximum GPU frame buffer usage in bytes for a container.
* `accelerator_frame_buffer_usage_avg`: The average GPU frame buffer usage in bytes for a container.



### 2. Namespace Metrics

Fields for metrics related to namespaces:

* `namespace`: The name of the namespace.
* `cpu_request_namespace_sum`: The total CPU request hard limit (millicores) summed across all `ResourceQuota` objects in the namespace, from `kube_resourcequota{resource='requests.cpu', type='hard'}`.
* `cpu_request_namespace_used`: The total CPU request **used** (millicores) summed across all `ResourceQuota` objects in the namespace, from `kube_resourcequota{resource='requests.cpu', type='used'}`. Emitted when the operator collects ROS namespace quota metrics; may be empty on older operator builds.
* `cpu_limit_namespace_sum`: The total CPU limit hard limit (millicores) summed across all `ResourceQuota` objects in the namespace, from `kube_resourcequota{resource='limits.cpu', type='hard'}`.
* `cpu_limit_namespace_used`: The total CPU limit **used** (millicores) summed across all `ResourceQuota` objects in the namespace, from `kube_resourcequota{resource='limits.cpu', type='used'}`.
* `cpu_usage_namespace_avg`: The average CPU usage rate across all containers in the namespace over a 15 minute window.
* `cpu_usage_namespace_max`: The maximum CPU usage rate observed across all containers in the namespace over a 15 minute window.
* `cpu_usage_namespace_min`: The minimum CPU usage rate observed across all containers in the namespace over a 15 minute window.
* `cpu_throttle_namespace_avg`: The average CPU throttling rate for all containers in the namespace over a 15 minute window, indicating how often containers hit their CPU limits.
* `cpu_throttle_namespace_max`: The maximum CPU throttling rate observed for all containers in the namespace over a 15 minute window.
* `cpu_throttle_namespace_min`: The minimum CPU throttling rate observed for all containers in the namespace over a 15 minute window.
* `memory_request_namespace_sum`: The total memory request hard limit (bytes) summed across all `ResourceQuota` objects in the namespace, from `kube_resourcequota{resource='requests.memory', type='hard'}`.
* `memory_request_namespace_used`: The total memory request **used** (bytes) summed across all `ResourceQuota` objects in the namespace, from `kube_resourcequota{resource='requests.memory', type='used'}`.
* `memory_limit_namespace_sum`: The total memory limit hard limit (bytes) summed across all `ResourceQuota` objects in the namespace, from `kube_resourcequota{resource='limits.memory', type='hard'}`.
* `memory_limit_namespace_used`: The total memory limit **used** (bytes) summed across all `ResourceQuota` objects in the namespace, from `kube_resourcequota{resource='limits.memory', type='used'}`.
* `memory_usage_namespace_avg`: The average working set memory usage across all containers in the namespace over a 15 minute window.
* `memory_usage_namespace_max`: The maximum working set memory usage observed across all containers in the namespace over a 15 minute window.
* `memory_usage_namespace_min`: The minimum working set memory usage observed across all containers in the namespace over a 15 minute window.
* `memory_rss_usage_namespace_avg`: The average RSS (Resident Set Size) memory usage across all containers in the namespace over a 15 minute window.
* `memory_rss_usage_namespace_max`: The maximum RSS memory usage observed across all containers in the namespace over a 15 minute window.
* `memory_rss_usage_namespace_min`: The minimum RSS memory usage observed across all containers in the namespace over a 15 minute window.
* `namespace_running_pods_max`: The maximum number of pods in a running state observed in the namespace over a 15 minute window.
* `namespace_running_pods_avg`: The average number of pods in a running state in the namespace over a 15 minute window.
* `namespace_total_pods_max`: The maximum total number of pods (all phases) observed in the namespace over a 15 minute window.
* `namespace_total_pods_avg`: The average total number of pods (all phases) in the namespace over a 15 minute window.

### 3. ClusterResourceQuota Metrics (ROS)

Monthly roll-up file: **`ros-openshift-cluster-quota-YYYYMM.csv`** (15-minute interval rows during collection). Collected from `openshift_clusterresourcequota_usage` via [`rosClusterQuotaQueries`](https://github.com/project-koku/koku-metrics-operator/blob/main/internal/collector/queries.go). One row per ClusterResourceQuota name per interval.

| Field | Description |
|-------|-------------|
| `cluster_quota_name` | OpenShift `ClusterResourceQuota` object name (Prometheus label `name`) |
| `cpu_request_hard` | Cluster-wide CPU request hard limit (millicores), `resource='requests.cpu', type='hard'` |
| `cpu_request_used` | Cluster-wide CPU request used (millicores), `type='used'` |
| `cpu_limit_hard` | Cluster-wide CPU limit hard limit (millicores), `resource='limits.cpu', type='hard'` |
| `cpu_limit_used` | Cluster-wide CPU limit used (millicores), `type='used'` |
| `memory_request_hard` | Cluster-wide memory request hard limit (bytes), `resource='requests.memory', type='hard'` |
| `memory_request_used` | Cluster-wide memory request used (bytes), `type='used'` |
| `memory_limit_hard` | Cluster-wide memory limit hard limit (bytes), `resource='limits.memory', type='hard'` |
| `memory_limit_used` | Cluster-wide memory limit used (bytes), `type='used'` |
| `storage_request_hard` | Cluster-wide storage request hard limit (bytes), `resource='requests.storage', type='hard'` |
| `storage_request_used` | Cluster-wide storage request used (bytes), `type='used'` |
| `pods_hard` | Cluster-wide pod count hard limit, `resource='pods', type='hard'` |
| `pods_used` | Cluster-wide pod count used, `type='used'` |
| `object_count_hard` | Sum of `count/*` hard limits across object types |
| `object_count_used` | Sum of `count/*` used values across object types |
| `namespaces` | Comma-separated namespaces with non-zero `type=used` for this CRQ |

### 4. OpenShift Virtualization VM Metrics (ROS)

15-minute **`ros-openshift-vm-usage-*.csv`** for VM recommendations (ros-ocp-backend). Collected by
[`rosVMQueries`](https://github.com/project-koku/koku-metrics-operator/blob/main/internal/collector/vm_ros_queries.go)
(15 Prometheus queries per window). See [`ros:vm_restart_count`](https://github.com/project-koku/koku-metrics-operator/blob/main/internal/collector/queries.go) for the crash-loop query.

Key fields (in addition to common interval timestamps):

* `vm_name`, `namespace`, `node_name`: VM identity.
* `guest_os`: Guest OS label from `kubevirt_vmi_info` (may be empty).
* `cpu_usage_mc`, `cpu_request_mc`, `cpu_limit_mc`: CPU usage and allocation (millicores).
* `mem_usage_kib`, `mem_request_kib`, `mem_limit_kib`: Memory usage and allocation.
* `memory_available_kib`: Guest-agent metric when QEMU guest agent is installed (optional).
* `disk_allocated_bytes`, `filesystem_used_bytes`, `filesystem_capacity_bytes`: Disk metrics.
* `disk_read_iops`, `disk_write_iops`, `disk_read_bytes_per_sec`, `disk_write_bytes_per_sec`: I/O.
* **`restart_count`**: Count of transitions into the `Running` phase during the 15-minute interval,
  derived from `kubevirt_vmi_phase_transition_time_seconds`. Summed daily by ROS for crash-loop
  detection (notification **48** when the term-window total ≥ `stability.crash_loop_restart_threshold`).

#### Per-GPU device metrics (`ros-openshift-vm-gpu-device-*.csv`)

Separate from the aggregate GPU columns on **`ros-openshift-vm-usage-*.csv`**, the operator can emit
**`ros-openshift-vm-gpu-device-YYYYMMDD-YYYYMMDD-HHMMSS.csv`** (monthly roll-up files use
`ros-openshift-vm-gpu-device-YYYYMM.csv` during collection). Collected by
[`writeVMGPUDeviceReport`](https://github.com/project-koku/koku-metrics-operator/blob/main/internal/collector/vm_gpu_device_collector.go)
when DCGM exporter metrics are available on **virt-launcher** pods.

| Aspect | Detail |
|--------|--------|
| **Purpose** | Per-GPU-device metrics for VMs — one row per GPU UUID per 15-minute interval |
| **Relationship to VM usage CSV** | Main VM CSV has aggregate GPU summary (`gpu_count`, blended utilization); device CSV has per-UUID detail for multi-GPU analysis and notification **54** |
| **When generated** | Only when Prometheus returns DCGM-style GPU metrics for virt-launcher pods; omitted entirely if no GPU data |

**Columns (14):**

* `interval_start` — Start of the 15-minute sampling window
* `namespace` — VM namespace
* `vm_name` — KubeVirt VM name (from pod/VMI labels, not the virt-launcher pod name)
* `gpu_uuid` — Device UUID from DCGM
* `gpu_model` — GPU product name
* `utilization_avg`, `utilization_max` — GPU utilization (0–1)
* `fb_used_avg_mib`, `fb_used_max_mib` — Frame buffer used (MiB)
* `sm_active_avg`, `tensor_active_avg`, `dram_active_avg` — SM/tensor/DRAM activity ratios
* `mig_profile` — MIG profile label when applicable
* `max_slices` — Maximum MIG slices for the profile
