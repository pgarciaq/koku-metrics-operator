# GPU and OpenShift Virtualization Metrics Architecture

This document describes how the koku-metrics-operator collects NVIDIA GPU and KubeVirt VM metrics, how reports are split between Cost Management (Koku) and Resource Optimization (ros-ocp-backend), and how MIG vs non-MIG workloads are handled.

Implementation references:

- Cost GPU queries and CSV shape: [`internal/collector/queries.go`](../internal/collector/queries.go), [`internal/collector/types.go`](../internal/collector/types.go) (`nvidiaGpuRow`)
- ROS container GPU columns: [`internal/collector/types.go`](../internal/collector/types.go) (`rosContainerRow`)
- ROS VM usage + GPU rollup: [`internal/collector/vm_gpu_collector.go`](../internal/collector/vm_gpu_collector.go), [`internal/collector/vm_ros_gpu_queries.go`](../internal/collector/vm_ros_gpu_queries.go)
- ROS VM per-GPU device report: [`internal/collector/vm_gpu_device_collector.go`](../internal/collector/vm_gpu_device_collector.go)

## Data path overview

```text
Prometheus / Thanos (DCGM + kube-state-metrics + KubeVirt)
        |
        v
koku-metrics-operator (hourly range + 15m ROS instant queries)
        |
        +-- Cost path  --> cm-openshift-nvidia-gpu-usage-*.csv --> Koku (gpu_usage)
        +-- ROS path   --> ros-openshift-container-*.csv       --> ros-ocp-backend (container plugin)
        +-- ROS VM     --> ros-openshift-vm-usage-*.csv         --> ros-ocp-backend (VM plugin)
        +-- ROS VM GPU --> ros-openshift-vm-gpu-device-*.csv  --> ros-ocp-backend (VM GPU device plugin)
```

## 1. ROS vs Cost GPU data split

### Cost Management (Koku)

| Aspect | Detail |
|--------|--------|
| Query prefix | `cost:nvidia_gpu_*` in [`QueryMap`](../internal/collector/queries.go) |
| Output file | `cm-openshift-nvidia-gpu-usage-{YYYYMM}.csv` |
| Downstream | Koku `gpu_usage` report type → `openshift_gpu_usage_line_items` (and `_daily`) |
| Granularity | Pod + physical GPU UUID (+ MIG instance when `GPU_I_ID` is present) |
| Primary metrics | Memory capacity, pod uptime (wall-clock presence), utilization sum, MIG max slices |

Prometheus queries include:

- `cost:nvidia_gpu_capacity_memory_mib_mig` / `_non_mig` — memory capacity (MIG uses DCGM + node labels; non-MIG uses `nvidia.com/gpu` requests)
- `cost:nvidia_gpu_utilization` — `DCGM_FI_PROF_GR_ENGINE_ACTIVE` (summed per interval)
- `cost:nvidia_gpu_pod_uptime` — clamped presence of profiling metric (wall-clock uptime proxy)
- `cost:nvidia_gpu_max_slices` — `DCGM_FI_DEV_MIG_MAX_SLICES` when MIG is enabled

CSV columns are defined by [`nvidiaGpuRow.csvHeader()`](../internal/collector/types.go): `gpu_uuid`, `gpu_model_name`, `gpu_vendor_name`, `gpu_memory_capacity_mib`, `gpu_pod_uptime`, `gpu_pod_utilization`, `gpu_max_slices`, `mig_instance_id`, `mig_profile`, `mig_strategy`.

### ROS — containers

| Aspect | Detail |
|--------|--------|
| Query prefix | `ros:accelerator_*`, `ros:tensor_pipe_*`, `ros:dram_*`, `ros:sm_active_*` |
| Output file | `ros-openshift-container-{timestamp}.csv` |
| Downstream | ros-ocp-backend container recommendations (GPU-aware when DCGM profiling is available) |
| Namespace filter | `cost_management_optimizations` or legacy `insights_cost_management_optimizations` label |

GPU-related columns on [`rosContainerRow`](../internal/collector/types.go): `accelerator_model_name`, `accelerator_profile_name`, frame-buffer min/max/avg, `tensor_pipe_active_*`, `dram_active_*`, `sm_active_*`.

### ROS — OpenShift Virtualization (VM)

Two complementary outputs:

1. **Aggregated VM row** — `ros-openshift-vm-usage-*.csv` ([`rosVMRow`](../internal/collector/types.go)): one row per VMI per 15-minute window with `gpu_count`, `gpu_model`, utilization/FB/profiling averages, `gpu_mig_profile`, `gpu_max_slices`.
2. **Per-device digest** — `ros-openshift-vm-gpu-device-*.csv` ([`rosVMGPUDeviceRow`](../internal/collector/vm_gpu_device_collector.go)): one row per `(namespace, vm_name, gpu_uuid)` with utilization, FB, SM/tensor/DRAM, MIG profile, max slices.

VM GPU queries use prefix `ros:vm_gpu_*` and only match `virt-launcher-*` pods ([`QueryMap`](../internal/collector/queries.go)).

## 2. MIG vs non-MIG query branches

MIG is detected when DCGM exposes **`GPU_I_PROFILE`** (mapped to `mig_profile` / `accelerator_profile_name`) and **`GPU_I_ID`** (mapped to `mig_instance_id`).

| Mode | Indicators | Cost capacity query | Row key |
|------|------------|---------------------|---------|
| MIG | `GPU_I_ID` non-empty, `GPU_I_PROFILE` set | `cost:nvidia_gpu_capacity_memory_mib_mig` | pod, namespace, node/Hostname, UUID, GPU_I_ID |
| Non-MIG | No MIG instance id | `cost:nvidia_gpu_capacity_memory_mib_non_mig` | pod, namespace, node + `nvidia.com/gpu` request × node memory label |

Collector logic in [`collector.go`](../internal/collector/collector.go) (`writeCostNvidiaGpuReport`): utilization rows drive the report; MIG rows merge capacity and max-slices on the same key; non-MIG merges capacity by sorted `(pod, namespace, node)` key.

Koku derives `mig_slice_count` and `mig_memory_capacity_mib` from `mig_profile` in post-processing when the operator does not supply them (`koku/masu/util/ocp/ocp_post_processor.py`).

### `honor_labels` and DCGM

Clusters that set **`honor_labels=true`** on the DCGM exporter may expose pod/namespace as native `pod`/`namespace` labels instead of `exported_pod`/`exported_namespace`. Cost GPU PromQL uses an **`or`** branch: prefer `exported_*` joins with `kube_pod_status_phase`, and fall back to `label_replace` from native labels when `exported_namespace` is empty. This was fixed in operator **v4.4.1** (see [`docs/csv-description.md`](csv-description.md)).

## 3. KubeVirt VM GPU correlation

Virt-launcher pods run the VM workload; GPU metrics attach to those pods, not to the VMI object directly.

**Primary correlation** — query `ros:vm_pod_vmi_name`:

```promql
max by (pod, namespace, label_vm_kubevirt_io_name) (
  kube_pod_labels{pod=~"virt-launcher-.*", label_vm_kubevirt_io_name!=""}
)
```

Implemented in [`vmiNameForGPURow`](../internal/collector/vm_gpu_collector.go): lookup `podVMINames[namespace + pod]` → `label_vm_kubevirt_io/name`.

**Fallback** — parse pod name `virt-launcher-<vmi-name>-<5-char-hash>` via [`vmiNameFromVirtLauncherPod`](../internal/collector/vm_gpu_collector.go) when kube-state-metrics labels are unavailable.

Per-GPU device rows require a resolved VMI name and non-empty `gpu_uuid` ([`writeVMGPUDeviceReport`](../internal/collector/vm_gpu_device_collector.go)).

## 4. DCGM exporter prerequisites

| Capability | DCGM metrics | ROS / cost usage |
|------------|--------------|------------------|
| Frame buffer | `DCGM_FI_DEV_FB_USED` | Always (V100/Pascal+); works without profiling |
| Profiling | `DCGM_FI_PROF_GR_ENGINE_ACTIVE`, `DCGM_FI_PROF_SM_ACTIVE`, `DCGM_FI_PROF_PIPE_TENSOR_ACTIVE`, `DCGM_FI_PROF_DRAM_ACTIVE` | Cost utilization/uptime; ROS SM/tensor/DRAM; VM profiling columns |

**Profiling requirement:** SM active, tensor pipe, and DRAM metrics need a DCGM build and GPU generation that supports profiling counters (Ampere/Hopper class). On **V100/Pascal** and similar, exporters often run with profiling disabled (`no_profiling` or equivalent): only frame-buffer and engine-active style metrics may appear. In that case:

- Cost path still gets utilization/uptime when `DCGM_FI_PROF_GR_ENGINE_ACTIVE` exists; otherwise GPU rows may be sparse.
- ROS container/VM **tensor/dram/sm** columns may be empty; frame-buffer columns may still populate.

Ensure the namespace monitor CR and operator pod selectors match your DCGM service monitor labels.

## 5. Multi-GPU approaches

| Workload | Strategy | Notes |
|----------|----------|-------|
| **VM (KubeVirt)** | Per-device CSV | `ros-openshift-vm-gpu-device-*.csv` keyed by `gpu_uuid`; VM usage CSV aggregates `gpu_count` across UUIDs |
| **Container (ROS)** | Single-stream | Queries group by one accelerator profile per container row; multi-GPU per pod is not split into separate rows (deferred) |
| **Cost (pod)** | One row per GPU UUID (+ MIG instance) | Multiple GPUs on a pod produce multiple CSV rows |

## 6. Operator version matrix (CSV columns)

| Operator version | Cost GPU CSV | ROS container GPU columns | ROS VM GPU columns | VM GPU device CSV |
|------------------|--------------|---------------------------|--------------------|-------------------|
| **< 4.3.0** | Not emitted | — | — | — |
| **4.3.0** | Base GPU cost columns (no MIG) | — | — | — |
| **4.4.0** | + `mig_instance_id`, `mig_profile`, `mig_strategy`, `gpu_max_slices` | (existing FB from 4.2.0) | — | — |
| **4.4.1** | + `gpu_pod_utilization`; `honor_labels` fix; uptime fix | Unchanged | — | — |
| **4.2.0** (ROS) | — | + `accelerator_*`, `tensor_pipe_*`, `dram_*`, `sm_active_*` | — | — |
| **Phase 12 / HEAD** | Same as 4.4.1+ | Unchanged | + `gpu_*` on `ros-openshift-vm-usage` | + `ros-openshift-vm-gpu-device` |

Koku on-prem (`OCPGPUUsageLineItem`) aligns with operator **4.4.0+** MIG columns and **4.4.1+** `gpu_pod_utilization`. Older operator bundles without MIG columns still ingest; missing columns are added as null in the masu pipeline.

## 7. Roadmap (items 1–3, future work)

The following are **not** implemented in the current MVP; they are tracked as follow-up work:

1. **GPUs per node** — node-level GPU inventory/capacity reporting for cost and ROS (beyond pod/VM-attached usage).
2. **Multi-GPU containers** — separate ROS rows or digests per GPU UUID for non-virt-launcher pods (today: single accelerator stream per container).
3. **Broader profiling degradation** — explicit operator signaling when only FB metrics are available, surfaced in manifest or report metadata for downstream quality flags.

For general operator architecture, see [`architecture.md`](architecture.md). For field-level CSV descriptions, see [`report-fields-description.md`](report-fields-description.md).
