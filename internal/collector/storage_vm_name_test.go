//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import (
	"testing"
)

func TestVMNameForStoragePod(t *testing.T) {
	t.Parallel()

	podVMINames := map[string]string{
		"kubevirt\x00virt-launcher-fedora-vm-x9y8z": "fedora-vm",
	}

	tests := []struct {
		pod      string
		ns       string
		want     string
	}{
		{"virt-launcher-fedora-vm-x9y8z", "kubevirt", "fedora-vm"},
		{"virt-launcher-my-vm-abc12", "prod", "my-vm"},
		{"app-pod-1", "prod", ""},
		{"", "prod", ""},
		{"virt-launcher-only-hash-abc12", "prod", "only-hash"},
	}
	for _, tt := range tests {
		got := vmNameForStoragePod(tt.pod, tt.ns, podVMINames)
		if got != tt.want {
			t.Errorf("vmNameForStoragePod(%q, %q) = %q, want %q", tt.pod, tt.ns, got, tt.want)
		}
	}
}

func TestApplyVMNameToStorageRows(t *testing.T) {
	t.Parallel()

	rows := mappedCSVStruct{
		"pv-1": &storageRow{
			Namespace: "kubevirt",
			Pod:       "virt-launcher-fedora-vm-x9y8z",
		},
		"pv-2": &storageRow{
			Namespace: "apps",
			Pod:       "data-pvc-app",
		},
	}
	applyVMNameToStorageRows(rows, map[string]string{
		"kubevirt\x00virt-launcher-fedora-vm-x9y8z": "fedora-vm",
	})
	if rows["pv-1"].(*storageRow).VMName != "fedora-vm" {
		t.Fatalf("VMName = %q, want fedora-vm", rows["pv-1"].(*storageRow).VMName)
	}
	if rows["pv-2"].(*storageRow).VMName != "" {
		t.Fatalf("VMName = %q, want empty for non-virt-launcher pod", rows["pv-2"].(*storageRow).VMName)
	}
}

func TestStorageRow_VMNameColumn(t *testing.T) {
	t.Parallel()

	row := storageRow{dateTimes: &dateTimes{}, VMName: "my-vm", Pod: "virt-launcher-my-vm-abc12"}
	header := row.csvHeader()
	csvRow := row.csvRow()
	if len(header) != len(csvRow) {
		t.Fatalf("csvRow length %d != csvHeader length %d", len(csvRow), len(header))
	}
	idx := -1
	for i, col := range header {
		if col == "vm_name" {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatal("csvHeader missing vm_name")
	}
	if csvRow[idx] != "my-vm" {
		t.Fatalf("vm_name column = %q, want my-vm", csvRow[idx])
	}
}
