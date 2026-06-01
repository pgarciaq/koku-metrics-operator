//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import "github.com/prometheus/common/model"

// rosVMQueries collects OpenShift Virtualization metrics at 15-minute resolution for ROS.
// There are 21 sequential Prometheus queries per 15-minute window. If any query still fails
// after getQueryResults retries, the entire window is discarded and retried on the next
// reconcile (see docs/design/partial-prometheus-failure-handling.md).
var rosVMQueries = &querys{
	query{
		Name:        "vm-ros-cpu-usage-mc",
		QueryString: QueryMap["ros:vm_cpu_usage_mc"],
		MetricKey: staticFields{
			"name":      "name",
			"namespace": "namespace",
			"node":      "node",
		},
		QueryValue: &saveQueryValue{ValName: "cpu_usage_mc"},
		RowKey:     []model.LabelName{"name", "namespace", "node"},
	},
	query{
		Name:        "vm-ros-cpu-request-mc",
		QueryString: QueryMap["ros:vm_cpu_request_mc"],
		MetricKey: staticFields{
			"name":      "name",
			"namespace": "namespace",
			"node":      "node",
		},
		QueryValue: &saveQueryValue{ValName: "cpu_request_mc"},
		RowKey:     []model.LabelName{"name", "namespace", "node"},
	},
	query{
		Name:        "vm-ros-cpu-limit-mc",
		QueryString: QueryMap["ros:vm_cpu_limit_mc"],
		MetricKey: staticFields{
			"name":      "name",
			"namespace": "namespace",
			"node":      "node",
		},
		QueryValue: &saveQueryValue{ValName: "cpu_limit_mc"},
		RowKey:     []model.LabelName{"name", "namespace", "node"},
	},
	query{
		Name:        "vm-ros-memory-usage-kib",
		QueryString: QueryMap["ros:vm_memory_usage_kib"],
		MetricKey: staticFields{
			"name":      "name",
			"namespace": "namespace",
			"node":      "node",
		},
		QueryValue: &saveQueryValue{ValName: "memory_usage_kib"},
		RowKey:     []model.LabelName{"name", "namespace", "node"},
	},
	query{
		Name:        "vm-ros-memory-request-kib",
		QueryString: QueryMap["ros:vm_memory_request_kib"],
		MetricKey: staticFields{
			"name":      "name",
			"namespace": "namespace",
			"node":      "node",
		},
		QueryValue: &saveQueryValue{ValName: "memory_request_kib"},
		RowKey:     []model.LabelName{"name", "namespace", "node"},
	},
	query{
		Name:        "vm-ros-memory-available-kib",
		QueryString: QueryMap["ros:vm_memory_available_kib"],
		MetricKey: staticFields{
			"name":      "name",
			"namespace": "namespace",
			"node":      "node",
		},
		QueryValue: &saveQueryValue{ValName: "memory_available_kib"},
		RowKey:     []model.LabelName{"name", "namespace", "node"},
	},
	query{
		Name:        "vm-ros-disk-allocated-bytes",
		QueryString: QueryMap["ros:vm_disk_allocated_bytes"],
		MetricKey: staticFields{
			"name":      "name",
			"namespace": "namespace",
			"node":      "node",
		},
		QueryValue: &saveQueryValue{ValName: "disk_allocated_bytes"},
		RowKey:     []model.LabelName{"name", "namespace", "node"},
	},
	query{
		Name:        "vm-ros-filesystem-used-bytes",
		QueryString: QueryMap["ros:vm_filesystem_used_bytes"],
		MetricKey: staticFields{
			"name":      "name",
			"namespace": "namespace",
			"node":      "node",
		},
		QueryValue: &saveQueryValue{ValName: "filesystem_used_bytes"},
		RowKey:     []model.LabelName{"name", "namespace", "node"},
	},
	query{
		Name:        "vm-ros-filesystem-capacity-bytes",
		QueryString: QueryMap["ros:vm_filesystem_capacity_bytes"],
		MetricKey: staticFields{
			"name":      "name",
			"namespace": "namespace",
			"node":      "node",
		},
		QueryValue: &saveQueryValue{ValName: "filesystem_capacity_bytes"},
		RowKey:     []model.LabelName{"name", "namespace", "node"},
	},
	query{
		Name:        "vm-ros-disk-read-iops",
		QueryString: QueryMap["ros:vm_disk_read_iops"],
		MetricKey: staticFields{
			"name":      "name",
			"namespace": "namespace",
			"node":      "node",
		},
		QueryValue: &saveQueryValue{ValName: "disk_read_iops"},
		RowKey:     []model.LabelName{"name", "namespace", "node"},
	},
	query{
		Name:        "vm-ros-disk-write-iops",
		QueryString: QueryMap["ros:vm_disk_write_iops"],
		MetricKey: staticFields{
			"name":      "name",
			"namespace": "namespace",
			"node":      "node",
		},
		QueryValue: &saveQueryValue{ValName: "disk_write_iops"},
		RowKey:     []model.LabelName{"name", "namespace", "node"},
	},
	query{
		Name:        "vm-ros-disk-read-bytes-per-sec",
		QueryString: QueryMap["ros:vm_disk_read_bytes_per_sec"],
		MetricKey: staticFields{
			"name":      "name",
			"namespace": "namespace",
			"node":      "node",
		},
		QueryValue: &saveQueryValue{ValName: "disk_read_bytes_per_sec"},
		RowKey:     []model.LabelName{"name", "namespace", "node"},
	},
	query{
		Name:        "vm-ros-disk-write-bytes-per-sec",
		QueryString: QueryMap["ros:vm_disk_write_bytes_per_sec"],
		MetricKey: staticFields{
			"name":      "name",
			"namespace": "namespace",
			"node":      "node",
		},
		QueryValue: &saveQueryValue{ValName: "disk_write_bytes_per_sec"},
		RowKey:     []model.LabelName{"name", "namespace", "node"},
	},
	query{
		Name:        "vm-ros-info",
		QueryString: QueryMap["ros:vm_info"],
		MetricKey: staticFields{
			"name":      "name",
			"namespace": "namespace",
			"node":      "node",
			"guest_os":  "os",
		},
		RowKey: []model.LabelName{"name", "namespace", "node"},
	},
	// vm-ros-restart-count (query 15 of 15): counts Running phase transitions per 15-minute
	// window from kubevirt_vmi_phase_transition_time_seconds. Written to restart_count in
	// ros-openshift-vm-usage CSV; ROS sums daily restart_count_sum for crash-loop notification 48.
	query{
		Name:        "vm-ros-restart-count",
		QueryString: QueryMap["ros:vm_restart_count"],
		MetricKey: staticFields{
			"name":      "name",
			"namespace": "namespace",
			"node":      "node",
		},
		QueryValue: &saveQueryValue{ValName: "restart_count"},
		RowKey:     []model.LabelName{"name", "namespace", "node"},
	},
	query{
		Name:        "vm-ros-net-rx-bytes-per-sec",
		QueryString: QueryMap["ros:vm_net_rx_bytes_per_sec"],
		MetricKey: staticFields{
			"name":      "name",
			"namespace": "namespace",
			"node":      "node",
		},
		QueryValue: &saveQueryValue{ValName: "net_rx_bytes_per_sec"},
		RowKey:     []model.LabelName{"name", "namespace", "node"},
	},
	query{
		Name:        "vm-ros-net-tx-bytes-per-sec",
		QueryString: QueryMap["ros:vm_net_tx_bytes_per_sec"],
		MetricKey: staticFields{
			"name":      "name",
			"namespace": "namespace",
			"node":      "node",
		},
		QueryValue: &saveQueryValue{ValName: "net_tx_bytes_per_sec"},
		RowKey:     []model.LabelName{"name", "namespace", "node"},
	},
	query{
		Name:        "vm-ros-net-rx-packets-per-sec",
		QueryString: QueryMap["ros:vm_net_rx_packets_per_sec"],
		MetricKey: staticFields{
			"name":      "name",
			"namespace": "namespace",
			"node":      "node",
		},
		QueryValue: &saveQueryValue{ValName: "net_rx_packets_per_sec"},
		RowKey:     []model.LabelName{"name", "namespace", "node"},
	},
	query{
		Name:        "vm-ros-net-tx-packets-per-sec",
		QueryString: QueryMap["ros:vm_net_tx_packets_per_sec"],
		MetricKey: staticFields{
			"name":      "name",
			"namespace": "namespace",
			"node":      "node",
		},
		QueryValue: &saveQueryValue{ValName: "net_tx_packets_per_sec"},
		RowKey:     []model.LabelName{"name", "namespace", "node"},
	},
	query{
		Name:        "vm-ros-net-rx-drops-per-sec",
		QueryString: QueryMap["ros:vm_net_rx_drops_per_sec"],
		MetricKey: staticFields{
			"name":      "name",
			"namespace": "namespace",
			"node":      "node",
		},
		QueryValue: &saveQueryValue{ValName: "net_rx_drops_per_sec"},
		RowKey:     []model.LabelName{"name", "namespace", "node"},
	},
	query{
		Name:        "vm-ros-net-tx-drops-per-sec",
		QueryString: QueryMap["ros:vm_net_tx_drops_per_sec"],
		MetricKey: staticFields{
			"name":      "name",
			"namespace": "namespace",
			"node":      "node",
		},
		QueryValue: &saveQueryValue{ValName: "net_tx_drops_per_sec"},
		RowKey:     []model.LabelName{"name", "namespace", "node"},
	},
}
