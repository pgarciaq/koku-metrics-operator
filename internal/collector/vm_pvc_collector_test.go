//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"

	"github.com/project-koku/koku-metrics-operator/internal/dirconfig"
	"github.com/project-koku/koku-metrics-operator/internal/testutils"
)

func TestVMPVCRow_CSVHeader(t *testing.T) {
	want := []string{
		"interval_start",
		"interval_end",
		"vm_name",
		"namespace",
		"node_name",
		"pvc_name",
		"disk_capacity_bytes",
		"volume_mode",
	}
	got := rosVMPVCRow{}.csvHeader()
	if len(got) != 8 {
		t.Fatalf("expected 8 columns, got %d: %v", len(got), got)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("csvHeader = %v, want %v", got, want)
	}
}

func TestVMPVCRow_CSVValues(t *testing.T) {
	row := rosVMPVCRow{
		IntervalStart:     "2026-05-01 12:00:00",
		IntervalEnd:       "2026-05-01 12:15:00",
		VMName:            "db-vm-01",
		Namespace:         "vm-ns",
		NodeName:          "worker-1",
		PVCName:           "data-pvc-shared",
		DiskCapacityBytes: "107374182400",
		VolumeMode:        "Filesystem",
	}
	got := row.csvRow()
	if len(got) != 8 {
		t.Fatalf("csvRow length = %d, want 8", len(got))
	}
	if got[2] != "db-vm-01" || got[5] != "data-pvc-shared" || got[7] != "Filesystem" {
		t.Fatalf("unexpected csvRow: %v", got)
	}
	if row.string() != strings.Join(got, ",") {
		t.Fatalf("string() mismatch with csvRow join")
	}
}

func TestVMPVCFilenamePrefix(t *testing.T) {
	if !strings.HasPrefix(rosVMPVCFilePrefix, "ros-openshift-vm-pvc-") {
		t.Fatalf("prefix = %q", rosVMPVCFilePrefix)
	}
	yearMonth := "202605"
	name := rosVMPVCFilePrefix + yearMonth + ".csv"
	if !strings.HasPrefix(name, "ros-openshift-vm-pvc-") {
		t.Fatalf("report name = %q", name)
	}
	if !strings.HasSuffix(name, ".csv") {
		t.Fatalf("report name = %q", name)
	}
}

func TestWriteVMPVCReport_EmptyData(t *testing.T) {
	log := testutils.ZapLogger(true)
	dir := t.TempDir()
	dirCfg := &dirconfig.DirectoryConfig{Reports: dirconfig.Directory{Path: dir}}
	start := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 1, 12, 15, 0, 0, time.UTC)
	c := &PrometheusCollector{TimeSeries: &promv1.Range{Start: start, End: end}}

	if err := writeVMPVCReport(log, c, dirCfg, "202605", nil); err != nil {
		t.Fatalf("writeVMPVCReport: %v", err)
	}
	path := filepath.Join(dir, rosVMPVCFilePrefix+"202605.csv")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected no PVC CSV for empty results, stat err=%v", err)
	}

	// Rows without persistentvolumeclaim are skipped; file must not be created.
	pvcResults := mappedResults{
		"k1": mappedValues{
			"name":      "some-vm",
			"namespace": "vm-ns",
			"node":      "worker-1",
		},
	}
	if err := writeVMPVCReport(log, c, dirCfg, "202605", pvcResults); err != nil {
		t.Fatalf("writeVMPVCReport: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected no PVC CSV when pvc_name missing, stat err=%v", err)
	}
}

func TestWriteVMPVCReport_ValidData(t *testing.T) {
	log := testutils.ZapLogger(true)
	dir := t.TempDir()
	dirCfg := &dirconfig.DirectoryConfig{Reports: dirconfig.Directory{Path: dir}}
	start := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 1, 12, 15, 0, 0, time.UTC)
	c := &PrometheusCollector{TimeSeries: &promv1.Range{Start: start, End: end}}

	pvcResults := mappedResults{
		"p1": mappedValues{
			"name":                  "db-vm-01",
			"namespace":             "vm-ns",
			"node":                  "worker-1",
			"persistentvolumeclaim": "data-pvc-shared",
			"pvc_disk_bytes":        "107374182400",
			"volume_mode":           "Filesystem",
		},
		"p2": mappedValues{
			"name":                  "db-vm-01",
			"namespace":             "vm-ns",
			"node":                  "worker-1",
			"persistentvolumeclaim": "logs-pvc",
			"pvc_disk_bytes":        "53687091200",
			"volume_mode":           "Filesystem",
		},
	}

	if err := writeVMPVCReport(log, c, dirCfg, "202605", pvcResults); err != nil {
		t.Fatalf("writeVMPVCReport: %v", err)
	}

	path := filepath.Join(dir, rosVMPVCFilePrefix+"202605.csv")
	records, err := readPVCCSV(path)
	if err != nil {
		t.Fatalf("read PVC CSV: %v", err)
	}
	if len(records) < 3 {
		t.Fatalf("expected header + 2 data rows, got %d records", len(records))
	}
	header := records[0]
	if len(header) != 8 {
		t.Fatalf("header columns = %d, want 8: %v", len(header), header)
	}
	if strings.Join(header, ",") != strings.Join(rosVMPVCRow{}.csvHeader(), ",") {
		t.Fatalf("header = %v", header)
	}

	foundShared := false
	for _, row := range records[1:] {
		if row[5] == "data-pvc-shared" {
			foundShared = true
			if row[2] != "db-vm-01" {
				t.Fatalf("vm_name = %q, want db-vm-01", row[2])
			}
			if row[3] != "vm-ns" {
				t.Fatalf("namespace = %q, want vm-ns", row[3])
			}
			if row[0] != start.Format("2006-01-02 15:04:05") {
				t.Fatalf("interval_start = %q, want %q", row[0], start.Format("2006-01-02 15:04:05"))
			}
		}
	}
	if !foundShared {
		t.Fatal("data-pvc-shared row not found in CSV output")
	}
}

func TestWriteVMPVCReport_DefaultVolumeMode(t *testing.T) {
	log := testutils.ZapLogger(true)
	dir := t.TempDir()
	dirCfg := &dirconfig.DirectoryConfig{Reports: dirconfig.Directory{Path: dir}}
	start := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 1, 12, 15, 0, 0, time.UTC)
	c := &PrometheusCollector{TimeSeries: &promv1.Range{Start: start, End: end}}

	pvcResults := mappedResults{
		"p1": mappedValues{
			"name":                  "vm-01",
			"namespace":             "ns",
			"node":                  "w1",
			"persistentvolumeclaim": "pvc-1",
			"pvc_disk_bytes":        "100",
		},
	}

	if err := writeVMPVCReport(log, c, dirCfg, "202605", pvcResults); err != nil {
		t.Fatalf("writeVMPVCReport: %v", err)
	}

	path := filepath.Join(dir, rosVMPVCFilePrefix+"202605.csv")
	records, err := readPVCCSV(path)
	if err != nil {
		t.Fatalf("read PVC CSV: %v", err)
	}
	if len(records) < 2 {
		t.Fatalf("expected at least header + 1 data row")
	}
	dataRow := records[1]
	if dataRow[7] != "Filesystem" {
		t.Fatalf("volume_mode = %q, want Filesystem (default)", dataRow[7])
	}
}

func readPVCCSV(path string) ([][]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return csv.NewReader(f).ReadAll()
}
