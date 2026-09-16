//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import (
	"strings"
	"testing"
)

// TestRosQueriesIncludeHypershiftNamespaces pins #405: every ROS query that
// gates on the manual opt-in labels must also match namespaces labeled
// hypershift.openshift.io/hosted-control-plane, so HyperShift control planes
// are collected without manual labeling. Queries with no namespace-label
// gate at all (VM collection is cluster-wide by design; HCP namespaces hold
// no VMs) are out of scope and explicitly skipped. Additions to any gated
// ros query list automatically fall under this assertion.
func TestRosQueriesIncludeHypershiftNamespaces(t *testing.T) {
	const hypershiftLabel = "label_hypershift_openshift_io_hosted_control_plane"

	var queries []string
	for key, q := range QueryMap {
		if strings.HasPrefix(key, "ros:") {
			queries = append(queries, q)
		}
	}
	lists := []*querys{
		rosContainerQueries, rosNamespaceQueries, rosVMQueries,
		rosVMGpuQueries, rosVMPodVMINameQueries, rosVMPVCQueries,
	}
	for _, list := range lists {
		if list == nil {
			t.Fatal("ros query list is nil")
		}
		for _, q := range *list {
			queries = append(queries, q.QueryString)
		}
	}
	if len(queries) == 0 {
		t.Fatal("expected ROS queries to inspect")
	}

	missing := 0
	scoped := 0
	for _, q := range queries {
		if !strings.Contains(q, "label_insights_cost_management_optimizations") &&
			!strings.Contains(q, "label_cost_management_optimizations") {
			continue // ungated query (e.g. cluster-wide VM collection): out of scope
		}
		scoped++
		if !strings.Contains(q, hypershiftLabel) {
			missing++
		}
	}
	if scoped == 0 {
		t.Fatal("expected gated ROS queries to inspect")
	}
	if missing != 0 {
		t.Errorf("%d gated ROS queries lack the hypershift namespace selector", missing)
	}
}
