//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import (
	"testing"
	"time"

	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
)

func TestROSVMRowCSVFormat(t *testing.T) {
	row := rosVMRow{
		IntervalStart:           "2020-11-06 18:00:01 +0000 UTC",
		IntervalEnd:             "2020-11-06 18:14:59 +0000 UTC",
		VMName:                  "test-vm",
		Namespace:               "default",
		NodeName:                "worker-1",
		GuestOS:                 "linux",
		CPUUsageMC:              "100.5",
		CPURequestMC:            "2000",
		CPULimitMC:              "4000",
		MemoryUsageKiB:          "512000",
		MemoryRequestKiB:        "1048576",
		MemoryAvailableKiB:      "",
		DiskAllocatedBytes:      "10737418240",
		FilesystemUsedBytes:     "",
		FilesystemCapacityBytes: "",
		DiskReadIOPS:            "1.2",
		DiskWriteIOPS:           "0.5",
		DiskReadBytesPerSec:     "1024",
		DiskWriteBytesPerSec:    "512",
	}

	header := rosVMRow{}.csvHeader()
	if len(header) != 19 {
		t.Fatalf("expected 19 columns, got %d", len(header))
	}

	csv := row.csvRow()
	if len(csv) != len(header) {
		t.Fatalf("header/row length mismatch: %d vs %d", len(header), len(csv))
	}
	if csv[11] != "" {
		t.Fatalf("expected empty memory_available_kib for missing guest agent, got %q", csv[11])
	}
}

func TestAverageVMSamples(t *testing.T) {
	samples := []mappedValues{
		{"cpu_usage_mc": "100", "memory_usage_kib": "1024"},
		{"cpu_usage_mc": "200", "memory_usage_kib": "2048"},
	}
	avg := averageVMSamples(samples)
	if avg["cpu_usage_mc"] != "150.000000" {
		t.Fatalf("expected averaged cpu_usage_mc, got %v", avg["cpu_usage_mc"])
	}
	if avg["memory_usage_kib"] != "1536.000000" {
		t.Fatalf("expected averaged memory_usage_kib, got %v", avg["memory_usage_kib"])
	}
}

func TestBuildHourlyVMRowFromSamples(t *testing.T) {
	localTime, _ := time.Parse(time.RFC3339, "2020-11-06T19:43:23Z")
	tm := localTime.UTC()
	hourRange := promv1.Range{
		Start: time.Date(tm.Year(), tm.Month(), tm.Day(), tm.Hour()-1, 0, 0, 0, tm.Location()),
		End:   time.Date(tm.Year(), tm.Month(), tm.Day(), tm.Hour()-1, 59, 59, 0, tm.Location()),
	}

	samples := []mappedValues{
		{
			"name":             "test-vm",
			"namespace":        "default",
			"node":             "worker-1",
			"guest_os":         "linux",
			"cpu_usage_mc":     "100",
			"cpu_request_mc":   "2000",
			"cpu_limit_mc":     "4000",
			"memory_usage_kib": "1048576",
			"disk_allocated_bytes": "10737418240",
		},
	}

	row := buildHourlyVMRow(&hourRange, samples)
	if row.VMName != "test-vm" {
		t.Fatalf("expected vm name test-vm, got %q", row.VMName)
	}
	if row.CPUUsageSeconds != "360.000000" {
		t.Fatalf("expected hourly cpu usage seconds 360, got %q", row.CPUUsageSeconds)
	}
	if row.UptimeSeconds != "3600.000000" {
		t.Fatalf("expected 3600 uptime seconds, got %q", row.UptimeSeconds)
	}
}

func TestBuildHourlyVMRowGuestAgentAbsent(t *testing.T) {
	samples := []mappedValues{
		{
			"name":              "legacy-vm",
			"namespace":         "legacy",
			"memory_available_kib": "",
		},
	}
	row := buildHourlyVMRow(&fakeTimeRange, samples)
	if row.VMName != "legacy-vm" {
		t.Fatalf("unexpected vm name %q", row.VMName)
	}
}
