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

var quotaNamespaceUsedQueryKeys = []string{
	"ros:cpu_request_namespace_used",
	"ros:cpu_limit_namespace_used",
	"ros:memory_request_namespace_used",
	"ros:memory_limit_namespace_used",
}

var quotaNamespaceUsedCSVColumns = []string{
	"cpu_request_namespace_used",
	"cpu_limit_namespace_used",
	"memory_request_namespace_used",
	"memory_limit_namespace_used",
}

func TestQueryMap_ResourceQuotaNamespaceUsedQueries(t *testing.T) {
	t.Parallel()

	for _, key := range quotaNamespaceUsedQueryKeys {
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
		if !strings.Contains(q, "type='used'") {
			t.Errorf("QueryMap[%q] should filter type='used', got: %s", key, q)
		}
		if !strings.Contains(q, "resourcequota") {
			t.Errorf("QueryMap[%q] should group by resourcequota, got: %s", key, q)
		}
	}
}

func TestRosNamespaceQueries_IncludeResourceQuotaUsed(t *testing.T) {
	t.Parallel()

	if rosNamespaceQuotaQueries == nil {
		t.Fatal("rosNamespaceQuotaQueries is nil")
	}

	names := make(map[string]struct{}, len(*rosNamespaceQuotaQueries))
	for _, q := range *rosNamespaceQuotaQueries {
		names[q.Name] = struct{}{}
		if q.QueryString == "" {
			t.Errorf("query %q has empty QueryString", q.Name)
		}
		if len(q.RowKey) != 2 {
			t.Errorf("query %q should RowKey namespace+resourcequota, got %v", q.Name, q.RowKey)
		}
	}

	for _, want := range []string{
		"cpu-request-namespace-used",
		"cpu-limit-namespace-used",
		"memory-request-namespace-used",
		"memory-limit-namespace-used",
		"storage-request-namespace-hard",
		"pods-namespace-hard",
	} {
		if _, ok := names[want]; !ok {
			t.Errorf("rosNamespaceQuotaQueries missing query %q", want)
		}
	}
}

func TestRosNamespaceRow_CSVHeader_IncludesNamespaceUsedColumns(t *testing.T) {
	t.Parallel()

	header := rosNamespaceRow{}.csvHeader()
	for _, col := range quotaNamespaceUsedCSVColumns {
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
