//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prometheus/common/model"
)

func TestQueryMap_UnifiedNamespaceQuotaQueries(t *testing.T) {
	t.Parallel()

	for _, key := range rosNamespaceQuotaMetricKeys {
		q, ok := QueryMap[key]
		if !ok {
			t.Fatalf("QueryMap missing %q", key)
		}
		if strings.TrimSpace(q) == "" {
			t.Fatalf("QueryMap[%q] is empty", key)
		}
		if !strings.Contains(q, "kube_resourcequota") {
			t.Errorf("QueryMap[%q] should query kube_resourcequota, got: %s", key, q)
		}
		if !strings.Contains(q, "resource") {
			t.Errorf("QueryMap[%q] should group by resource, got: %s", key, q)
		}
		if !strings.Contains(q, "resourcequota") {
			t.Errorf("QueryMap[%q] should group by resourcequota, got: %s", key, q)
		}
	}

	if !strings.Contains(QueryMap["ros:namespace_quota_hard"], "type='hard'") {
		t.Error("hard query should filter type='hard'")
	}
	if !strings.Contains(QueryMap["ros:namespace_quota_used"], "type='used'") {
		t.Error("used query should filter type='used'")
	}
}

func TestRosNamespaceQuotaQueries_Structure(t *testing.T) {
	t.Parallel()

	if rosNamespaceQuotaQueries == nil {
		t.Fatal("rosNamespaceQuotaQueries is nil")
	}

	if len(*rosNamespaceQuotaQueries) != 2 {
		t.Fatalf("expected 2 unified queries, got %d", len(*rosNamespaceQuotaQueries))
	}

	for _, q := range *rosNamespaceQuotaQueries {
		if q.QueryString == "" {
			t.Errorf("query %q has empty QueryString", q.Name)
		}
		if len(q.RowKey) != 3 {
			t.Errorf("query %q should have RowKey with 3 labels (namespace, resource, resourcequota), got %v", q.Name, q.RowKey)
		}
		if q.MetricKey["resource_name"] != model.LabelName("resource") {
			t.Errorf("query %q should have MetricKey resource_name -> resource", q.Name)
		}
	}
}

func TestRosNamespaceRow_CSVHeader_IncludesQuotaColumns(t *testing.T) {
	t.Parallel()

	header := rosNamespaceRow{}.csvHeader()
	requiredCols := []string{
		"cpu_request_namespace_sum",
		"cpu_request_namespace_used",
		"cpu_limit_namespace_sum",
		"cpu_limit_namespace_used",
		"memory_request_namespace_sum",
		"memory_request_namespace_used",
		"memory_limit_namespace_sum",
		"memory_limit_namespace_used",
		"storage_request_namespace_hard",
		"storage_request_namespace_used",
		"pods_namespace_hard",
		"pods_namespace_used",
		"object_count_namespace_hard",
		"object_count_namespace_used",
	}

	for _, col := range requiredCols {
		found := false
		for _, h := range header {
			if h == col {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("csvHeader() missing column %q", col)
		}
	}

	cpuReqUsedIdx := -1
	cpuReqSumIdx := -1
	for i, h := range header {
		switch h {
		case "cpu_request_namespace_used":
			cpuReqUsedIdx = i
		case "cpu_request_namespace_sum":
			cpuReqSumIdx = i
		}
	}
	if cpuReqUsedIdx < 0 || cpuReqSumIdx < 0 {
		t.Fatal("expected cpu_request_namespace_sum and cpu_request_namespace_used in header")
	}
	if cpuReqUsedIdx != cpuReqSumIdx+1 {
		t.Errorf("cpu_request_namespace_used should follow cpu_request_namespace_sum, got indices %d and %d",
			cpuReqSumIdx, cpuReqUsedIdx)
	}
}

func TestPivotNamespaceQuotaResults(t *testing.T) {
	t.Parallel()

	raw := mappedResults{
		"compute-resources,costmanagement-metrics-operator,requests.cpu": mappedValues{
			"namespace":     "costmanagement-metrics-operator",
			"quota_name":    "compute-resources",
			"resource_name": "requests.cpu",
			"hard_value":    "0.100000",
			"used_value":    "0.050000",
		},
		"compute-resources,costmanagement-metrics-operator,limits.cpu": mappedValues{
			"namespace":     "costmanagement-metrics-operator",
			"quota_name":    "compute-resources",
			"resource_name": "limits.cpu",
			"hard_value":    "0.500000",
			"used_value":    "0.250000",
		},
		"compute-resources,costmanagement-metrics-operator,count/pods": mappedValues{
			"namespace":     "costmanagement-metrics-operator",
			"quota_name":    "compute-resources",
			"resource_name": "count/pods",
			"hard_value":    "50.000000",
			"used_value":    "25.000000",
		},
		"compute-resources,costmanagement-metrics-operator,count/configmaps": mappedValues{
			"namespace":     "costmanagement-metrics-operator",
			"quota_name":    "compute-resources",
			"resource_name": "count/configmaps",
			"hard_value":    "50.000000",
			"used_value":    "25.000000",
		},
	}

	pivoted := pivotNamespaceQuotaResults(raw)

	expectedKey := makeQuotaPivotKey("costmanagement-metrics-operator", "compute-resources")
	val, ok := pivoted[expectedKey]
	if !ok {
		t.Fatalf("pivoted results missing key %q, got keys: %v", expectedKey, keys(pivoted))
	}

	if val["cpu-request-namespace-sum"] != "0.100000" {
		t.Errorf("cpu-request-namespace-sum = %v, want 0.100000", val["cpu-request-namespace-sum"])
	}
	if val["cpu-request-namespace-used"] != "0.050000" {
		t.Errorf("cpu-request-namespace-used = %v, want 0.050000", val["cpu-request-namespace-used"])
	}
	if val["cpu-limit-namespace-sum"] != "0.500000" {
		t.Errorf("cpu-limit-namespace-sum = %v, want 0.500000", val["cpu-limit-namespace-sum"])
	}
	if val["cpu-limit-namespace-used"] != "0.250000" {
		t.Errorf("cpu-limit-namespace-used = %v, want 0.250000", val["cpu-limit-namespace-used"])
	}

	// count/* resources should be summed: 50 + 50 = 100
	if val["object-count-namespace-hard"] != "100.000000" {
		t.Errorf("object-count-namespace-hard = %v, want 100.000000", val["object-count-namespace-hard"])
	}
	if val["object-count-namespace-used"] != "50.000000" {
		t.Errorf("object-count-namespace-used = %v, want 50.000000", val["object-count-namespace-used"])
	}

	if val["namespace"] != "costmanagement-metrics-operator" {
		t.Errorf("namespace = %v, want costmanagement-metrics-operator", val["namespace"])
	}
	if val["quota_name"] != "compute-resources" {
		t.Errorf("quota_name = %v, want compute-resources", val["quota_name"])
	}
}

func TestPivotNamespaceQuotaResults_EmptyInput(t *testing.T) {
	t.Parallel()

	pivoted := pivotNamespaceQuotaResults(mappedResults{})
	if len(pivoted) != 0 {
		t.Errorf("expected empty result for empty input, got %d entries", len(pivoted))
	}
}

func keys(m mappedResults) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func addNamespaceQuotaMockResults(mapResults mappedMockPromResult, t *testing.T, withData bool) {
	t.Helper()
	for _, query := range *rosNamespaceQuotaQueries {
		res := &model.Vector{}
		if withData {
			dataPath := filepath.Join("test_files", "test_data", query.Name)
			if _, err := os.Stat(dataPath); err == nil {
				Load(dataPath, res, t)
			}
		}
		mapResults[query.QueryString] = &mockPromResult{value: *res}
	}
}
