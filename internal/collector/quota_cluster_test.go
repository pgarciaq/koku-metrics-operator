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

var clusterQuotaQueryMapKeys = []string{
	"ros:cluster_quota_cpu_request_hard",
	"ros:cluster_quota_cpu_request_used",
	"ros:cluster_quota_cpu_limit_hard",
	"ros:cluster_quota_cpu_limit_used",
	"ros:cluster_quota_memory_request_hard",
	"ros:cluster_quota_memory_request_used",
	"ros:cluster_quota_memory_limit_hard",
	"ros:cluster_quota_memory_limit_used",
	"ros:cluster_quota_storage_request_hard",
	"ros:cluster_quota_storage_request_used",
	"ros:cluster_quota_pods_hard",
	"ros:cluster_quota_pods_used",
	"ros:cluster_quota_object_count_hard",
	"ros:cluster_quota_object_count_used",
	"ros:cluster_quota_namespace_members",
}

var clusterQuotaQueryNames = []string{
	"cluster-quota-cpu-request-hard",
	"cluster-quota-cpu-request-used",
	"cluster-quota-cpu-limit-hard",
	"cluster-quota-cpu-limit-used",
	"cluster-quota-memory-request-hard",
	"cluster-quota-memory-request-used",
	"cluster-quota-memory-limit-hard",
	"cluster-quota-memory-limit-used",
	"cluster-quota-storage-request-hard",
	"cluster-quota-storage-request-used",
	"cluster-quota-pods-hard",
	"cluster-quota-pods-used",
	"cluster-quota-object-count-hard",
	"cluster-quota-object-count-used",
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
		if !strings.Contains(q, "sum by (name)") {
			t.Errorf("QueryMap[%q] should group by name, got: %s", key, q)
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
