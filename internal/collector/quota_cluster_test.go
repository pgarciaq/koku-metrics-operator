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

var clusterQuotaQueryMapKeys = append(rosClusterQuotaMetricKeys,
	"ros:cluster_quota_namespace_members",
)

var clusterQuotaQueryNames = []string{
	"cluster-quota-hard",
	"cluster-quota-used",
}

var clusterQuotaCSVColumns = []string{
	"cluster_quota_name",
	"cpu_request_hard",
	"cpu_request_used",
	"cpu_limit_hard",
	"cpu_limit_used",
	"memory_request_hard",
	"memory_request_used",
	"memory_limit_hard",
	"memory_limit_used",
	"storage_request_hard",
	"storage_request_used",
	"pods_hard",
	"pods_used",
	"object_count_hard",
	"object_count_used",
	"namespaces",
}

func TestQueryMap_ClusterQuotaQueries(t *testing.T) {
	t.Parallel()

	for _, key := range clusterQuotaQueryMapKeys {
		q, ok := QueryMap[key]
		if !ok {
			t.Fatalf("QueryMap missing %q", key)
		}
		if strings.TrimSpace(q) == "" {
			t.Fatalf("QueryMap[%q] is empty", key)
		}
		if !strings.Contains(q, "openshift_clusterresourcequota_usage") {
			t.Errorf("QueryMap[%q] should query openshift_clusterresourcequota_usage, got: %s", key, q)
		}
		if key == "ros:cluster_quota_namespace_members" {
			if !strings.Contains(q, "namespace") {
				t.Errorf("QueryMap[%q] should include namespace label, got: %s", key, q)
			}
			continue
		}
		if !strings.Contains(q, "sum by (name, resource)") {
			t.Errorf("QueryMap[%q] should group by (name, resource), got: %s", key, q)
		}
	}
}

func TestRosClusterQuotaQueries_Registered(t *testing.T) {
	t.Parallel()

	if rosClusterQuotaQueries == nil {
		t.Fatal("rosClusterQuotaQueries is nil")
	}

	names := make(map[string]struct{}, len(*rosClusterQuotaQueries))
	for _, q := range *rosClusterQuotaQueries {
		names[q.Name] = struct{}{}
		if q.QueryString == "" {
			t.Errorf("query %q has empty QueryString", q.Name)
		}
	}

	for _, want := range clusterQuotaQueryNames {
		if _, ok := names[want]; !ok {
			t.Errorf("rosClusterQuotaQueries missing query %q", want)
		}
	}
}

func TestRosClusterQuotaRow_CSVHeader(t *testing.T) {
	t.Parallel()

	header := rosClusterQuotaRow{}.csvHeader()
	for _, col := range clusterQuotaCSVColumns {
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
}

func addClusterQuotaMockResults(mapResults mappedMockPromResult, t *testing.T, withData bool) {
	t.Helper()
	for _, query := range *rosClusterQuotaQueries {
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

func loadClusterQuotaMockResults(t *testing.T) mappedMockPromResult {
	t.Helper()
	mapResults := make(mappedMockPromResult)
	addClusterQuotaMockResults(mapResults, t, true)
	mapResults[QueryMap["ros:cluster_quota_namespace_members"]] = &mockPromResult{value: model.Vector{}}
	return mapResults
}

func TestGenerateROSClusterQuotaReport_EmptyResultsNoFile(t *testing.T) {
	mapResults := make(mappedMockPromResult)
	for _, query := range *rosClusterQuotaQueries {
		mapResults[query.QueryString] = &mockPromResult{value: model.Vector{}}
	}

	tempReportsDir := filepath.Join("test_files", "test_reports")
	if err := os.MkdirAll(tempReportsDir, os.ModePerm); err != nil {
		t.Fatalf("failed to create test reports dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempReportsDir); err != nil {
			t.Fatalf("failed to cleanup test reports dir: %v", err)
		}
	}()

	copyfakeTimeRange := fakeTimeRange
	fakeCollector := &PrometheusCollector{
		PromConn: mockPrometheusConnection{
			mappedResults: &mapResults,
			t:             t,
		},
		TimeSeries: &copyfakeTimeRange,
	}

	yearMonth := copyfakeTimeRange.Start.Format("200601")
	reportPath := filepath.Join(tempReportsDir, rosClusterQuotaFilePrefix+yearMonth+".csv")
	_ = os.Remove(reportPath)

	if err := generateROSClusterQuotaReport(log, fakeCollector, fakeDirCfg, yearMonth, copyfakeTimeRange.End); err != nil {
		t.Fatalf("generateROSClusterQuotaReport returned error: %v", err)
	}

	if _, err := os.Stat(reportPath); !os.IsNotExist(err) {
		t.Errorf("expected no cluster quota report file when PromQL is empty, but file exists at %s", reportPath)
	}
}

func TestGenerateROSClusterQuotaReport_WithData(t *testing.T) {
	mapResults := loadClusterQuotaMockResults(t)

	tempReportsDir := filepath.Join("test_files", "test_reports")
	if err := os.MkdirAll(tempReportsDir, os.ModePerm); err != nil {
		t.Fatalf("failed to create test reports dir: %v", err)
	}
	defer func() {
		if err := fakeDirCfg.Reports.RemoveContents(); err != nil {
			t.Fatalf("failed to cleanup reports directory: %v", err)
		}
	}()

	copyfakeTimeRange := fakeTimeRange
	fakeCollector := &PrometheusCollector{
		PromConn: mockPrometheusConnection{
			mappedResults: &mapResults,
			t:             t,
		},
		TimeSeries: &copyfakeTimeRange,
	}

	yearMonth := copyfakeTimeRange.Start.Format("200601")
	if err := generateROSClusterQuotaReport(log, fakeCollector, fakeDirCfg, yearMonth, copyfakeTimeRange.End); err != nil {
		t.Fatalf("generateROSClusterQuotaReport returned error: %v", err)
	}

	expectedPath := filepath.Join("test_files", "expected_cluster_quota_reports", rosClusterQuotaFilePrefix+yearMonth+".csv")
	generatedPath := filepath.Join(tempReportsDir, rosClusterQuotaFilePrefix+yearMonth+".csv")

	expectedInfo, err := os.Open(expectedPath)
	if err != nil {
		t.Fatalf("failed to open expected report %s: %v", expectedPath, err)
	}
	defer expectedInfo.Close()

	generatedInfo, err := os.Open(generatedPath)
	if err != nil {
		t.Fatalf("failed to open generated report %s: %v", generatedPath, err)
	}
	defer generatedInfo.Close()

	if err := compareFiles(expectedInfo, generatedInfo); err != nil {
		t.Errorf("cluster quota report does not match golden file: %v", err)
	}
}

func TestPivotClusterQuotaResults(t *testing.T) {
	t.Parallel()

	raw := mappedResults{
		"requests.cpu,team-a": mappedValues{
			"cluster_quota_name": "team-a",
			"resource_name":      "requests.cpu",
			"hard_value":         "10",
			"used_value":         "3",
		},
		"limits.cpu,team-a": mappedValues{
			"cluster_quota_name": "team-a",
			"resource_name":      "limits.cpu",
			"hard_value":         "20",
			"used_value":         "5",
		},
		"requests.memory,team-a": mappedValues{
			"cluster_quota_name": "team-a",
			"resource_name":      "requests.memory",
			"hard_value":         "1073741824",
			"used_value":         "536870912",
		},
		"limits.memory,team-a": mappedValues{
			"cluster_quota_name": "team-a",
			"resource_name":      "limits.memory",
			"hard_value":         "2147483648",
			"used_value":         "1073741824",
		},
	}

	result := pivotClusterQuotaResults(raw)

	if len(result) != 1 {
		t.Fatalf("expected 1 CRQ in pivot output, got %d", len(result))
	}

	row, ok := result["team-a"]
	if !ok {
		t.Fatal("pivot output missing key 'team-a'")
	}

	checks := map[string]string{
		"cluster_quota_name":  "team-a",
		"cpu-request-hard":    "10",
		"cpu-request-used":    "3",
		"cpu-limit-hard":      "20",
		"cpu-limit-used":      "5",
		"memory-request-hard": "1073741824",
		"memory-request-used": "536870912",
		"memory-limit-hard":   "2147483648",
		"memory-limit-used":   "1073741824",
	}

	for field, want := range checks {
		got, _ := row[field].(string)
		if got != want {
			t.Errorf("field %q: got %q, want %q", field, got, want)
		}
	}
}

func TestPivotClusterQuotaResults_ObjectCount(t *testing.T) {
	t.Parallel()

	raw := mappedResults{
		"count/pods,team-b": mappedValues{
			"cluster_quota_name": "team-b",
			"resource_name":      "count/pods",
			"hard_value":         "50",
			"used_value":         "10",
		},
		"count/configmaps,team-b": mappedValues{
			"cluster_quota_name": "team-b",
			"resource_name":      "count/configmaps",
			"hard_value":         "30",
			"used_value":         "5",
		},
		"pods,team-b": mappedValues{
			"cluster_quota_name": "team-b",
			"resource_name":      "pods",
			"hard_value":         "100",
			"used_value":         "20",
		},
	}

	result := pivotClusterQuotaResults(raw)

	if len(result) != 1 {
		t.Fatalf("expected 1 CRQ in pivot output, got %d", len(result))
	}

	row := result["team-b"]

	if got, want := row["pods-hard"].(string), "100"; got != want {
		t.Errorf("pods-hard: got %q, want %q", got, want)
	}
	if got, want := row["pods-used"].(string), "20"; got != want {
		t.Errorf("pods-used: got %q, want %q", got, want)
	}
	if got, want := row["object-count-hard"].(string), floatToString(80); got != want {
		t.Errorf("object-count-hard: got %q, want %q", got, want)
	}
	if got, want := row["object-count-used"].(string), floatToString(15); got != want {
		t.Errorf("object-count-used: got %q, want %q", got, want)
	}
}
