//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import (
	"strings"
	"testing"
)

// TestRosContainerAndNamespaceQueriesCarryOptInGate is #808's structural
// regression: EVERY query in the container, namespace, and namespace-quota
// CSV query lists must gate on the namespace opt-in label — a row for a
// namespace the customer never opted into is the #808 defect (the
// node-pressure trio escaped precisely because no test asserted this;
// TestRosQueriesIncludeHypershiftNamespaces deliberately skips ungated
// queries). New queries added to these lists without a gate fail here.
//
// Out of scope, by design: the VM query lists (cluster-wide collection,
// #405's rationale) and the cluster-quota namespace-membership query
// (cluster-scoped rows — see cluster_quota_namespaces.go).
func TestRosContainerAndNamespaceQueriesCarryOptInGate(t *testing.T) {
	// Prefix match (no closing brace): the quota queries carry the same
	// gate arms with additional label selectors inside the braces
	// (namespace!~'kube-.*|openshift-.*').
	const gateArm1 = "kube_namespace_labels{label_insights_cost_management_optimizations='true'"
	const gateArm2 = "kube_namespace_labels{label_cost_management_optimizations='true'"

	lists := map[string]*querys{
		"rosContainerQueries":      rosContainerQueries,
		"rosNamespaceQueries":      rosNamespaceQueries,
		"rosNamespaceQuotaQueries": rosNamespaceQuotaQueries,
	}
	checked := 0
	for listName, list := range lists {
		if list == nil {
			t.Fatalf("%s is nil", listName)
		}
		for _, q := range *list {
			if strings.TrimSpace(q.QueryString) == "" {
				continue
			}
			checked++
			if !strings.Contains(q.QueryString, gateArm1) && !strings.Contains(q.QueryString, gateArm2) {
				t.Errorf("%s entry %q: no opt-in namespace gate in query — "+
					"every container/namespace-row query must carry one (#808)",
					listName, q.Name)
			}
		}
	}
	if checked == 0 {
		t.Fatal("expected container/namespace queries to inspect")
	}
}

// TestNodePressureQueriesFilterOnConditionValue pins the value-blind fix:
// PromQL `and` matches series EXISTENCE, and the
// kube_node_status_condition{...,status='true'} series always exists
// (value 0 when the node is healthy), so an unfiltered `and on(node)`
// stamps node_pressure on every container on every node, every interval.
// The filter must compare the VALUE (== 1), and all three queries must
// carry ALL THREE opt-in gate arms (insights, legacy cost, hypershift) —
// a dropped arm silently strips node-pressure from customers using only
// that label form (the review's M3: arm 2's inner `or` was unpinned).
//
// Cell-value semantics, for the next reader: the value is
// clamp_min(cpu_request, 1) — max(request, 1), NOT always 1 — and the
// backend treats it as > 0 = pressured (librobne PressureOrdinal).
func TestNodePressureQueriesFilterOnConditionValue(t *testing.T) {
	arms := [3]string{
		"kube_namespace_labels{label_insights_cost_management_optimizations='true'}",
		"kube_namespace_labels{label_cost_management_optimizations='true'}",
		"kube_namespace_labels{label_hypershift_openshift_io_hosted_control_plane='true'}",
	}
	for _, key := range []string{"ros:node_pressure_memory", "ros:node_pressure_disk", "ros:node_pressure_pid"} {
		q, ok := QueryMap[key]
		if !ok {
			t.Fatalf("QueryMap missing %s", key)
		}
		if !strings.Contains(q, "status='true'} == 1") {
			t.Errorf("%s: the condition filter must compare the VALUE (== 1), not the series' existence — "+
				"the status='true' series always exists (value 0 when healthy), so an unfiltered and-on(node) "+
				"marks every container as pressured (#808)", key)
		}
		for i, arm := range arms {
			if !strings.Contains(q, arm) {
				t.Errorf("%s: missing the opt-in gate (arm %d: %q)", key, i+1, arm)
			}
		}
	}
}
