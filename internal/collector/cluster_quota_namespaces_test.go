//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import (
	"testing"

	"github.com/prometheus/common/model"
)

func TestClusterQuotaNamespaceMembershipByCRQ(t *testing.T) {
	t.Parallel()

	vector := model.Vector{
		&model.Sample{
			Metric: model.Metric{
				"name":      "team-a",
				"namespace": "payments",
			},
			Value: 1,
		},
		&model.Sample{
			Metric: model.Metric{
				"name":      "team-a",
				"namespace": "billing",
			},
			Value: 1,
		},
	}

	mapResults := mappedMockPromResult{
		QueryMap["ros:cluster_quota_namespace_members"]: &mockPromResult{value: vector},
	}

	c := &PrometheusCollector{
		PromConn:   mockPrometheusConnection{mappedResults: &mapResults, t: t},
		TimeSeries: &fakeTimeRange,
	}

	got, err := c.clusterQuotaNamespaceMembershipByCRQ(fakeTimeRange.End)
	if err != nil {
		t.Fatalf("clusterQuotaNamespaceMembershipByCRQ: %v", err)
	}
	if got["team-a"] != "billing,payments" {
		t.Fatalf("got namespaces %q, want billing,payments", got["team-a"])
	}
}

func TestApplyClusterQuotaNamespaces(t *testing.T) {
	t.Parallel()

	rows := mappedCSVStruct{
		"team-a": &rosClusterQuotaRow{ClusterQuotaName: "team-a"},
	}
	applyClusterQuotaNamespaces(rows, map[string]string{"team-a": "ns1,ns2"})
	if rows["team-a"].(*rosClusterQuotaRow).Namespaces != "ns1,ns2" {
		t.Fatalf("namespaces = %q", rows["team-a"].(*rosClusterQuotaRow).Namespaces)
	}
}
