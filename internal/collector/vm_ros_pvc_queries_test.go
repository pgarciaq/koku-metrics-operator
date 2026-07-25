//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import "testing"

func TestRosVMPVCQueriesDefined(t *testing.T) {
	if rosVMPVCQueries == nil {
		t.Fatal("rosVMPVCQueries is nil")
	}
	expected := []string{
		"ros:vm_pvc_disk_bytes",
	}
	for _, key := range expected {
		if _, ok := QueryMap[key]; !ok {
			t.Errorf("QueryMap missing %q", key)
		}
	}
	if len(*rosVMPVCQueries) != len(expected) {
		t.Fatalf("expected %d PVC queries, got %d", len(expected), len(*rosVMPVCQueries))
	}
}

func TestRosVMPVCQueryRowKey(t *testing.T) {
	q := (*rosVMPVCQueries)[0]
	if len(q.RowKey) != 4 {
		t.Fatalf("expected 4 RowKey labels, got %d: %v", len(q.RowKey), q.RowKey)
	}
	expectedKeys := []string{"name", "namespace", "node", "persistentvolumeclaim"}
	for i, k := range expectedKeys {
		if string(q.RowKey[i]) != k {
			t.Errorf("RowKey[%d] = %q, want %q", i, q.RowKey[i], k)
		}
	}
}
