//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import "testing"

func TestIsKubeVirtCRDAvailableNilConfig(t *testing.T) {
	if IsKubeVirtCRDAvailable(nil) {
		t.Fatal("expected false for nil rest config")
	}
}

func TestShouldCollectVMMetrics(t *testing.T) {
	if shouldCollectVMMetrics(nil) {
		t.Fatal("expected false for nil collector")
	}
	if shouldCollectVMMetrics(&PrometheusCollector{}) {
		t.Fatal("expected false without rest config")
	}
}
