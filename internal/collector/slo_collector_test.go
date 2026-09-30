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

	metricscfgv1beta1 "github.com/project-koku/koku-metrics-operator/api/v1beta1"
)

const sloTestHCID = "d5d31999-1111-4444-8888-aaaaaaaaaaaa"

var sloQueryMapKeys = []string{
	"ros:slo_api_buckets_mutating",
	"ros:slo_api_buckets_read",
	"ros:slo_api_buckets_other",
}

var sloQueryNames = []string{
	"slo-api-buckets-mutating",
	"slo-api-buckets-read",
	"slo-api-buckets-other",
}

// TestQueryMap_SLOQueries pins the #644 contract: histogram buckets summed by
// le only (never split by resource), verb regexes baked in, WATCH/CONNECT/
// PROXY excluded at source in every query — by positive match (mutating/read
// can never select them) or explicit exclusion (other).
func TestQueryMap_SLOQueries(t *testing.T) {
	t.Parallel()

	excluded := []string{"WATCH", "CONNECT", "PROXY"}
	for _, key := range sloQueryMapKeys {
		q, ok := QueryMap[key]
		if !ok {
			t.Fatalf("QueryMap missing %q", key)
		}
		if !strings.Contains(q, "apiserver_request_duration_seconds_bucket") {
			t.Errorf("QueryMap[%q] should query apiserver_request_duration_seconds_bucket, got: %s", key, q)
		}
		if !strings.Contains(q, "sum by (le, job)") {
			t.Errorf("QueryMap[%q] should group by (le, job) — per-job coherent progressions, never split by resource — got: %s", key, q)
		}
		if strings.Contains(q, "=~") {
			// Positive match: none of the excluded verbs may appear as an alternative.
			for _, verb := range excluded {
				for _, alt := range strings.Split(extractVerbAlternatives(q), "|") {
					if strings.TrimSpace(alt) == verb {
						t.Errorf("QueryMap[%q] positive match selects excluded verb %s, got: %s", key, verb, q)
					}
				}
			}
		}
		if strings.Contains(q, "!~") {
			// Negative match: all excluded verbs must be listed.
			for _, verb := range excluded {
				if !strings.Contains(q, verb) {
					t.Errorf("QueryMap[%q] should exclude %s at source, got: %s", key, verb, q)
				}
			}
		}
	}
	if !strings.Contains(QueryMap["ros:slo_api_buckets_mutating"], "CREATE") {
		t.Errorf("mutating query should match CREATE/UPDATE/PATCH/DELETE verbs, got: %s", QueryMap["ros:slo_api_buckets_mutating"])
	}
	if !strings.Contains(QueryMap["ros:slo_api_buckets_read"], "GET") || !strings.Contains(QueryMap["ros:slo_api_buckets_read"], "LIST") {
		t.Errorf("read query should match GET/LIST verbs, got: %s", QueryMap["ros:slo_api_buckets_read"])
	}
}

// extractVerbAlternatives returns the first =~ "..." alternative list in q.
func extractVerbAlternatives(q string) string {
	start := strings.Index(q, "=~\"")
	if start < 0 {
		return ""
	}
	rest := q[start+3:]
	end := strings.Index(rest, "\"")
	if end < 0 {
		return ""
	}
	return rest[:end]
}

func TestSLOQueries_Registered(t *testing.T) {
	t.Parallel()

	if sloQueries == nil {
		t.Fatal("sloQueries is nil")
	}
	names := make(map[string]struct{}, len(*sloQueries))
	for _, q := range *sloQueries {
		names[q.Name] = struct{}{}
		if q.QueryString == "" {
			t.Errorf("query %q has empty QueryString", q.Name)
		}
	}
	for _, want := range sloQueryNames {
		if _, ok := names[want]; !ok {
			t.Errorf("sloQueries missing query %q", want)
		}
	}
}

func TestSLORow_CSVHeader(t *testing.T) {
	t.Parallel()

	header := sloRow{}.csvHeader()
	want := []string{"hc_cluster_id", "window_start", "window_end", "verb_group", "le", "bucket_count", "collected_at", "source"}
	if len(header) != len(want) {
		t.Fatalf("csvHeader() has %d columns, want %d: %v", len(header), len(want), header)
	}
	for i, col := range want {
		if header[i] != col {
			t.Errorf("csvHeader()[%d] = %q, want %q (contract column order)", i, header[i], col)
		}
	}
}

func TestShouldCollectSLO(t *testing.T) {
	t.Parallel()

	externalCR := &metricscfgv1beta1.MetricsConfig{}
	externalCR.Status.Topology.ControlPlaneTopology = "External"
	if !shouldCollectSLO(externalCR) {
		t.Errorf("shouldCollectSLO = false for External topology, want true")
	}
	standaloneCR := &metricscfgv1beta1.MetricsConfig{}
	standaloneCR.Status.Topology.ControlPlaneTopology = "HighlyAvailable"
	if shouldCollectSLO(standaloneCR) {
		t.Errorf("shouldCollectSLO = true for HighlyAvailable topology, want false")
	}
	if shouldCollectSLO(&metricscfgv1beta1.MetricsConfig{}) {
		t.Errorf("shouldCollectSLO = true for empty topology, want false")
	}
	if shouldCollectSLO(nil) {
		t.Errorf("shouldCollectSLO = true for nil CR, want false")
	}
}

func TestFormatSLOCount(t *testing.T) {
	t.Parallel()

	// floatToString's 6-decimal form must render as integer for backend ParseInt.
	got, ok := formatSLOCount("102.000000")
	if !ok || got != "102" {
		t.Errorf("formatSLOCount(102.000000) = %q,%v want 102,true", got, ok)
	}
	for _, bad := range []interface{}{"", "abc", "-3", "-0.5"} {
		if _, ok := formatSLOCount(bad); ok {
			t.Errorf("formatSLOCount(%v) = ok, want omitted", bad)
		}
	}
}

func TestBuildSLORows_OmitsIncomplete(t *testing.T) {
	t.Parallel()

	results := mappedResults{
		"0.1":  mappedValues{"le": "0.1", "source": "kubernetes", "slo_mutating_count": "12.000000", "slo_read_count": "45.000000"},
		"+Inf": mappedValues{"le": "+Inf", "source": "kubernetes", "slo_mutating_count": "102.000000", "slo_read_count": "310.000000", "slo_other_count": "18.000000"},
		"0.5":  mappedValues{"le": "0.5", "source": "kubernetes", "slo_mutating_count": "87.000000"},
	}
	rows := buildSLORows(results, sloTestHCID, "ws", "we", "ca")
	// 0.1 → mutating+read (other absent → omitted); +Inf → all three; 0.5 → mutating only.
	if len(rows) != 6 {
		t.Fatalf("buildSLORows returned %d rows, want 6 (incomplete groups omitted)", len(rows))
	}
	seen := map[string]string{}
	for _, r := range rows {
		seen[r.VerbGroup+"|"+r.Le] = r.BucketCount
		if r.Source != "kubernetes" {
			t.Errorf("row source = %q, want kubernetes", r.Source)
		}
	}
	if seen["other|0.1"] != "" || seen["read|0.5"] != "" || seen["other|0.5"] != "" {
		t.Errorf("incomplete groups should be omitted, got: %v", seen)
	}
	if seen["mutating|+Inf"] != "102" || seen["read|+Inf"] != "310" || seen["other|+Inf"] != "18" {
		t.Errorf("+Inf rows wrong: %v", seen)
	}
}

func sloSample(job, le string, v float64, ts model.Time) *model.Sample {
	return &model.Sample{
		Metric:    model.Metric{"le": model.LabelValue(le), "job": model.LabelValue(job)},
		Value:     model.SampleValue(v),
		Timestamp: ts,
	}
}

func addSLOMockResults(mapResults mappedMockPromResult, withData bool) {
	for _, q := range *sloQueries {
		vec := model.Vector{}
		if withData {
			ts := model.Time(0)
			switch q.Name {
			case "slo-api-buckets-mutating":
				vec = model.Vector{sloSample("kubernetes", "0.1", 12, ts), sloSample("kubernetes", "0.5", 87, ts), sloSample("kubernetes", "+Inf", 102, ts)}
			case "slo-api-buckets-read":
				vec = model.Vector{sloSample("kubernetes", "0.1", 45, ts), sloSample("kubernetes", "+Inf", 310, ts)}
			case "slo-api-buckets-other":
				vec = model.Vector{sloSample("kubernetes", "+Inf", 18, ts)}
			}
		}
		mapResults[q.QueryString] = &mockPromResult{value: vec}
	}
}

func TestGenerateSLOReport_WithData(t *testing.T) {
	mapResults := make(mappedMockPromResult)
	addSLOMockResults(mapResults, true)

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
	reportPath := filepath.Join(tempReportsDir, rosSLOFilePrefix+yearMonth+".csv")
	_ = os.Remove(reportPath)

	if err := generateSLOReport(log, fakeCollector, fakeDirCfg, yearMonth, sloTestHCID, copyfakeTimeRange.Start, copyfakeTimeRange.End, copyfakeTimeRange.End); err != nil {
		t.Fatalf("generateSLOReport returned error: %v", err)
	}

	expectedPath := filepath.Join("test_files", "expected_reports", rosSLOFilePrefix+yearMonth+".csv")
	expectedInfo, err := os.Open(expectedPath)
	if err != nil {
		t.Fatalf("failed to open expected report %s: %v", expectedPath, err)
	}
	defer expectedInfo.Close()

	generatedInfo, err := os.Open(reportPath)
	if err != nil {
		t.Fatalf("failed to open generated report %s: %v", reportPath, err)
	}
	defer generatedInfo.Close()

	if err := compareFiles(expectedInfo, generatedInfo); err != nil {
		t.Fatalf("generated SLO report differs from golden fixture: %v", err)
	}
}

func TestGenerateSLOReport_EmptyResultsNoFile(t *testing.T) {
	mapResults := make(mappedMockPromResult)
	addSLOMockResults(mapResults, false)

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
	reportPath := filepath.Join(tempReportsDir, rosSLOFilePrefix+yearMonth+".csv")
	_ = os.Remove(reportPath)

	if err := generateSLOReport(log, fakeCollector, fakeDirCfg, yearMonth, sloTestHCID, copyfakeTimeRange.Start, copyfakeTimeRange.End, copyfakeTimeRange.End); err != nil {
		t.Fatalf("generateSLOReport returned error: %v", err)
	}
	if _, err := os.Stat(reportPath); !os.IsNotExist(err) {
		t.Errorf("expected no SLO report file when PromQL is empty, but file exists at %s", reportPath)
	}
}

func TestGenerateSLOReport_UnknownHCNoFile(t *testing.T) {
	mapResults := make(mappedMockPromResult)
	addSLOMockResults(mapResults, true)

	copyfakeTimeRange := fakeTimeRange
	fakeCollector := &PrometheusCollector{
		PromConn: mockPrometheusConnection{
			mappedResults: &mapResults,
			t:             t,
		},
		TimeSeries: &copyfakeTimeRange,
	}

	tempReportsDir := filepath.Join("test_files", "test_reports")
	if err := os.MkdirAll(tempReportsDir, os.ModePerm); err != nil {
		t.Fatalf("failed to create test reports dir: %v", err)
	}
	yearMonth := copyfakeTimeRange.Start.Format("200601")
	reportPath := filepath.Join(tempReportsDir, rosSLOFilePrefix+yearMonth+".csv")
	_ = os.Remove(reportPath)
	defer func() {
		_ = os.Remove(reportPath)
	}()

	if err := generateSLOReport(log, fakeCollector, fakeDirCfg, yearMonth, "", copyfakeTimeRange.Start, copyfakeTimeRange.End, copyfakeTimeRange.End); err != nil {
		t.Fatalf("generateSLOReport with unknown HC ID returned error (must never fail): %v", err)
	}
	if _, err := os.Stat(reportPath); !os.IsNotExist(err) {
		t.Errorf("expected no SLO report file with unknown HC ID, but file exists at %s", reportPath)
	}
}

func TestCanonicalSLOLe(t *testing.T) {
	t.Parallel()

	for in, want := range map[string]string{
		"0.1": "0.1", "8": "8", "8.0": "8", "+Inf": "+Inf", "inf": "+Inf",
	} {
		got, ok := canonicalSLOLe(in)
		if !ok || got != want {
			t.Errorf("canonicalSLOLe(%q) = %q,%v want %q,true", in, got, ok, want)
		}
	}
	for _, bad := range []string{"", "abc", "-1", "NaN"} {
		if _, ok := canonicalSLOLe(bad); ok {
			t.Errorf("canonicalSLOLe(%q) = ok, want rejected", bad)
		}
	}
}

// TestBuildSLORows_MergesFloatEqualBoundaries pins the live-lab 2026-09-30
// finding: different apiserver jobs emit string-distinct but float-equal le
// boundaries ("8" vs "8.0"). They must merge by summing (disjoint cumulative
// progressions add) instead of colliding in the backend float PK.
func TestBuildSLORows_MergesFloatEqualBoundaries(t *testing.T) {
	t.Parallel()

	results := mappedResults{
		"8":   mappedValues{"le": "8", "source": "kubernetes", "slo_read_count": "100.000000"},
		"8.0": mappedValues{"le": "8.0", "source": "kubernetes", "slo_read_count": "50.000000"},
	}
	rows := buildSLORows(results, sloTestHCID, "ws", "we", "ca")
	if len(rows) != 1 {
		t.Fatalf("buildSLORows returned %d rows, want 1 merged row", len(rows))
	}
	if rows[0].Le != "8" || rows[0].BucketCount != "150" {
		t.Errorf("merged row = %v, want le=8 count=150", rows[0])
	}
}

// TestBuildSLORows_SplitsBySource pins the amendment: same (verb_group, le)
// from different jobs must stay separate rows so each progression is a
// coherent cumulative histogram (live lab 2026-09-30: kubernetes vs
// metrics-server boundary schemas).
func TestBuildSLORows_SplitsBySource(t *testing.T) {
	t.Parallel()

	results := mappedResults{
		"a": mappedValues{"le": "0.4", "source": "metrics-server", "slo_read_count": "94577.000000"},
		"b": mappedValues{"le": "0.1", "source": "kubernetes", "slo_read_count": "3357110.000000"},
	}
	rows := buildSLORows(results, sloTestHCID, "ws", "we", "ca")
	if len(rows) != 2 {
		t.Fatalf("buildSLORows returned %d rows, want 2 (one per source)", len(rows))
	}
	bySource := map[string]sloRow{}
	for _, r := range rows {
		bySource[r.Source] = r
	}
	if bySource["kubernetes"].BucketCount != "3357110" || bySource["metrics-server"].BucketCount != "94577" {
		t.Errorf("per-source rows wrong: %v", bySource)
	}
}
