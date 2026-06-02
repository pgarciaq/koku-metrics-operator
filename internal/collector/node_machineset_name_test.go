//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"github.com/go-logr/logr"
)

func TestMachineSetNameFromMachine(t *testing.T) {
	t.Parallel()

	machine := &unstructured.Unstructured{Object: map[string]interface{}{
		"status": map[string]interface{}{
			"nodeRef": map[string]interface{}{
				"name": "worker-1",
			},
		},
	}}
	machine.SetOwnerReferences([]metav1.OwnerReference{{
		APIVersion: "machine.openshift.io/v1beta1",
		Kind:       machineSetKind,
		Name:       "worker-us-east-1a",
		Controller: boolPtr(true),
	}})

	node, ms := machineSetNameFromMachine(machine)
	if node != "worker-1" {
		t.Fatalf("node = %q, want worker-1", node)
	}
	if ms != "worker-us-east-1a" {
		t.Fatalf("machineset = %q, want worker-us-east-1a", ms)
	}
}

func TestMachineSetNameFromMachine_NoMachineSetOwner(t *testing.T) {
	t.Parallel()

	machine := &unstructured.Unstructured{Object: map[string]interface{}{
		"status": map[string]interface{}{
			"nodeRef": map[string]interface{}{
				"name": "bare-metal-1",
			},
		},
	}}
	node, ms := machineSetNameFromMachine(machine)
	if node != "bare-metal-1" {
		t.Fatalf("node = %q, want bare-metal-1", node)
	}
	if ms != "" {
		t.Fatalf("machineset = %q, want empty", ms)
	}
}

func TestBuildNodeMachineSetNameMap_NilConfig(t *testing.T) {
	t.Parallel()

	got := BuildNodeMachineSetNameMap(nil, logr.Discard())
	if len(got) != 0 {
		t.Fatalf("expected empty map, got %v", got)
	}
}

func TestApplyMachineSetNamesToNodeRows(t *testing.T) {
	t.Parallel()

	rows := mappedCSVStruct{
		"node-1": &nodeRow{Node: "node-1"},
	}
	applyMachineSetNamesToNodeRows(rows, map[string]string{"node-1": "worker-a"})
	if rows["node-1"].(*nodeRow).MachineSetName != "worker-a" {
		t.Fatalf("MachineSetName = %q, want worker-a", rows["node-1"].(*nodeRow).MachineSetName)
	}
}

func TestRosContainerRow_MachineSetNameColumn(t *testing.T) {
	t.Parallel()

	row := rosContainerRow{
		dateTimes: &dateTimes{},
		nodeRow: nodeRow{
			InstanceType:   "m5.xlarge",
			MachineSetName: "worker-us-east-1a",
		},
	}
	header := row.csvHeader()
	csvRow := row.csvRow()
	if len(header) != len(csvRow) {
		t.Fatalf("csvRow length %d != csvHeader length %d", len(csvRow), len(header))
	}

	instanceTypeIdx, machinesetIdx := -1, -1
	for i, col := range header {
		switch col {
		case "instance_type":
			instanceTypeIdx = i
		case "machineset_name":
			machinesetIdx = i
		}
	}
	if instanceTypeIdx < 0 || machinesetIdx < 0 {
		t.Fatal("instance_type and machineset_name must be in csvHeader()")
	}
	if machinesetIdx != instanceTypeIdx+1 {
		t.Fatalf("machineset_name index %d should follow instance_type %d", machinesetIdx, instanceTypeIdx)
	}
	if csvRow[machinesetIdx] != "worker-us-east-1a" {
		t.Fatalf("csvRow machineset_name = %q", csvRow[machinesetIdx])
	}
}

func boolPtr(b bool) *bool { return &b }
