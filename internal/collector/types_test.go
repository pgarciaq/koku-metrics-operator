package collector

import (
	"testing"
)

func TestRosContainerRow_OOMCountInHeader(t *testing.T) {
	header := rosContainerRow{}.csvHeader()

	oomIdx := -1
	for i, col := range header {
		if col == "oom_count" {
			oomIdx = i
			break
		}
	}
	if oomIdx < 0 {
		t.Fatal("oom_count must be present in csvHeader()")
	}
	if oomIdx > 0 && header[oomIdx-1] != "memory_rss_usage_container_sum" {
		t.Errorf("oom_count should follow memory_rss_usage_container_sum, got %q", header[oomIdx-1])
	}
	if oomIdx < len(header)-1 && header[oomIdx+1] != "workload_pod_count" {
		t.Errorf("oom_count should precede workload_pod_count, got %q", header[oomIdx+1])
	}
}

func TestRosContainerRow_OOMCountInRow(t *testing.T) {
	row := rosContainerRow{
		dateTimes: &dateTimes{
			ReportPeriodStart: "2026-03-01",
			ReportPeriodEnd:   "2026-04-01",
			IntervalStart:     "2026-03-15 10:00:00",
			IntervalEnd:       "2026-03-15 10:15:00",
		},
		OOMCount: "3",
	}

	csvRow := row.csvRow()
	header := row.csvHeader()

	if len(header) != len(csvRow) {
		t.Fatalf("csvRow length %d != csvHeader length %d", len(csvRow), len(header))
	}

	oomIdx := -1
	for i, col := range header {
		if col == "oom_count" {
			oomIdx = i
			break
		}
	}
	if oomIdx < 0 {
		t.Fatal("oom_count must be in header")
	}
	if csvRow[oomIdx] != "3" {
		t.Errorf("csvRow OOM count = %q, want %q", csvRow[oomIdx], "3")
	}
}

func TestRosContainerRow_OOMCountZero(t *testing.T) {
	row := rosContainerRow{
		dateTimes: &dateTimes{},
		OOMCount:  "0",
	}

	csvRow := row.csvRow()
	header := row.csvHeader()

	oomIdx := -1
	for i, col := range header {
		if col == "oom_count" {
			oomIdx = i
			break
		}
	}
	if oomIdx < 0 {
		t.Fatal("oom_count must be in header")
	}
	if csvRow[oomIdx] != "0" {
		t.Errorf("zero OOM count = %q, want %q", csvRow[oomIdx], "0")
	}
}

func TestRosContainerRow_ReplicaColumns(t *testing.T) {
	row := rosContainerRow{
		dateTimes: &dateTimes{
			ReportPeriodStart: "2026-03-01",
			ReportPeriodEnd:   "2026-04-01",
			IntervalStart:     "2026-03-15 10:00:00",
			IntervalEnd:       "2026-03-15 10:15:00",
		},
		DesiredReplicas:   "3",
		AvailableReplicas: "2",
	}

	csvRow := row.csvRow()
	header := row.csvHeader()

	if len(header) != len(csvRow) {
		t.Fatalf("csvRow length %d != csvHeader length %d", len(csvRow), len(header))
	}

	desiredIdx := -1
	availIdx := -1
	podCountIdx := -1
	for i, col := range header {
		switch col {
		case "desired_replicas":
			desiredIdx = i
		case "available_replicas":
			availIdx = i
		case "workload_pod_count":
			podCountIdx = i
		}
	}
	if desiredIdx < 0 {
		t.Fatal("desired_replicas must be in csvHeader()")
	}
	if availIdx < 0 {
		t.Fatal("available_replicas must be in csvHeader()")
	}
	if desiredIdx != podCountIdx+1 {
		t.Errorf("desired_replicas should follow workload_pod_count, got index %d vs %d", desiredIdx, podCountIdx)
	}
	if availIdx != desiredIdx+1 {
		t.Errorf("available_replicas should follow desired_replicas, got index %d vs %d", availIdx, desiredIdx)
	}
	if csvRow[desiredIdx] != "3" {
		t.Errorf("csvRow desired_replicas = %q, want %q", csvRow[desiredIdx], "3")
	}
	if csvRow[availIdx] != "2" {
		t.Errorf("csvRow available_replicas = %q, want %q", csvRow[availIdx], "2")
	}
}

func TestRosContainerRow_NodeCapacityColumns(t *testing.T) {
	row := rosContainerRow{
		dateTimes: &dateTimes{
			ReportPeriodStart: "2026-03-01",
			ReportPeriodEnd:   "2026-04-01",
			IntervalStart:     "2026-03-15 10:00:00",
			IntervalEnd:       "2026-03-15 10:15:00",
		},
		nodeRow: nodeRow{
			NodeCapacityCPUCores:       "8.000000",
			NodeCapacityMemoryBytes:    "33554432000.000000",
			NodeAllocatableCPUCores:    "7.500000",
			NodeAllocatableMemoryBytes: "32212254720.000000",
		},
	}

	csvRow := row.csvRow()
	header := row.csvHeader()

	if len(header) != len(csvRow) {
		t.Fatalf("csvRow length %d != csvHeader length %d", len(csvRow), len(header))
	}

	cpuCapIdx := -1
	memCapIdx := -1
	cpuAllocIdx := -1
	memAllocIdx := -1
	for i, col := range header {
		switch col {
		case "node_capacity_cpu_cores":
			cpuCapIdx = i
		case "node_capacity_memory_bytes":
			memCapIdx = i
		case "node_allocatable_cpu_cores":
			cpuAllocIdx = i
		case "node_allocatable_memory_bytes":
			memAllocIdx = i
		}
	}
	if cpuCapIdx < 0 {
		t.Fatal("node_capacity_cpu_cores must be in csvHeader()")
	}
	if memCapIdx < 0 {
		t.Fatal("node_capacity_memory_bytes must be in csvHeader()")
	}
	if cpuAllocIdx < 0 {
		t.Fatal("node_allocatable_cpu_cores must be in csvHeader()")
	}
	if memAllocIdx < 0 {
		t.Fatal("node_allocatable_memory_bytes must be in csvHeader()")
	}
	if memCapIdx != cpuCapIdx+1 {
		t.Errorf("node_capacity_memory_bytes should follow node_capacity_cpu_cores")
	}
	if cpuAllocIdx != memCapIdx+1 {
		t.Errorf("node_allocatable_cpu_cores should follow node_capacity_memory_bytes")
	}
	if memAllocIdx != cpuAllocIdx+1 {
		t.Errorf("node_allocatable_memory_bytes should follow node_allocatable_cpu_cores")
	}
	if csvRow[cpuCapIdx] != "8.000000" {
		t.Errorf("csvRow node_capacity_cpu_cores = %q, want %q", csvRow[cpuCapIdx], "8.000000")
	}
	if csvRow[memCapIdx] != "33554432000.000000" {
		t.Errorf("csvRow node_capacity_memory_bytes = %q, want %q", csvRow[memCapIdx], "33554432000.000000")
	}
	if csvRow[cpuAllocIdx] != "7.500000" {
		t.Errorf("csvRow node_allocatable_cpu_cores = %q, want %q", csvRow[cpuAllocIdx], "7.500000")
	}
	if csvRow[memAllocIdx] != "32212254720.000000" {
		t.Errorf("csvRow node_allocatable_memory_bytes = %q, want %q", csvRow[memAllocIdx], "32212254720.000000")
	}
}

func TestRosContainerRow_InstanceTypeColumn(t *testing.T) {
	row := rosContainerRow{
		dateTimes: &dateTimes{
			ReportPeriodStart: "2026-03-01",
			ReportPeriodEnd:   "2026-04-01",
			IntervalStart:     "2026-03-15 10:00:00",
			IntervalEnd:       "2026-03-15 10:15:00",
		},
		nodeRow: nodeRow{
			NodeCapacityCPUCores:       "8.000000",
			NodeCapacityMemoryBytes:    "33554432000.000000",
			NodeAllocatableCPUCores:    "7.500000",
			NodeAllocatableMemoryBytes: "32212254720.000000",
			InstanceType:               "m5.2xlarge",
		},
	}

	csvRow := row.csvRow()
	header := row.csvHeader()

	if len(header) != len(csvRow) {
		t.Fatalf("csvRow length %d != csvHeader length %d", len(csvRow), len(header))
	}

	instanceTypeIdx := -1
	memAllocIdx := -1
	for i, col := range header {
		switch col {
		case "instance_type":
			instanceTypeIdx = i
		case "node_allocatable_memory_bytes":
			memAllocIdx = i
		}
	}
	if instanceTypeIdx < 0 {
		t.Fatal("instance_type must be in csvHeader()")
	}
	if memAllocIdx < 0 {
		t.Fatal("node_allocatable_memory_bytes must be in csvHeader()")
	}
	if instanceTypeIdx != memAllocIdx+1 {
		t.Errorf("instance_type should follow node_allocatable_memory_bytes, got indices %d and %d", instanceTypeIdx, memAllocIdx)
	}
	if csvRow[instanceTypeIdx] != "m5.2xlarge" {
		t.Errorf("csvRow instance_type = %q, want %q", csvRow[instanceTypeIdx], "m5.2xlarge")
	}
}
