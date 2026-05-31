//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import "github.com/prometheus/common/model"

// rosVMQueries collects OpenShift Virtualization metrics at 15-minute resolution for ROS.
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
}
