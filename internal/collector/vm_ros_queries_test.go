//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import "testing"

func TestRosVMQueriesDefined(t *testing.T) {
	if rosVMQueries == nil {
		t.Fatal("rosVMQueries is nil")
	}
	expected := []string{
		"ros:vm_cpu_usage_mc",
		"ros:vm_cpu_request_mc",
		"ros:vm_cpu_limit_mc",
		"ros:vm_memory_usage_kib",
		"ros:vm_memory_request_kib",
		"ros:vm_memory_available_kib",
		"ros:vm_disk_allocated_bytes",
		"ros:vm_filesystem_used_bytes",
		"ros:vm_filesystem_capacity_bytes",
		"ros:vm_disk_read_iops",
		"ros:vm_disk_write_iops",
		"ros:vm_disk_read_bytes_per_sec",
		"ros:vm_disk_write_bytes_per_sec",
		"ros:vm_info",
		"ros:vm_restart_count",
	}
	for _, key := range expected {
		if _, ok := QueryMap[key]; !ok {
			t.Errorf("QueryMap missing %q", key)
		}
	}
	if len(*rosVMQueries) != len(expected) {
		t.Fatalf("expected %d ros VM queries, got %d", len(expected), len(*rosVMQueries))
	}
}
