//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"k8s.io/client-go/rest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/project-koku/koku-metrics-operator/internal/dirconfig"
	"github.com/project-koku/koku-metrics-operator/internal/testutils"
)

func TestVMQuarterHoursConstant(t *testing.T) {
	if vmQuarterHours != 4 {
		t.Fatalf("vmQuarterHours = %d, want 4 (four 15-minute windows per hour)", vmQuarterHours)
	}
}

func TestRosVMQueriesMetricFieldNames(t *testing.T) {
	expectedValNames := map[string]string{
		"vm-ros-cpu-usage-mc":              "cpu_usage_mc",
		"vm-ros-cpu-request-mc":            "cpu_request_mc",
		"vm-ros-cpu-limit-mc":              "cpu_limit_mc",
		"vm-ros-memory-usage-kib":          "memory_usage_kib",
		"vm-ros-memory-request-kib":        "memory_request_kib",
		"vm-ros-memory-available-kib":      "memory_available_kib",
		"vm-ros-disk-allocated-bytes":      "disk_allocated_bytes",
		"vm-ros-filesystem-used-bytes":     "filesystem_used_bytes",
		"vm-ros-filesystem-capacity-bytes": "filesystem_capacity_bytes",
		"vm-ros-disk-read-iops":            "disk_read_iops",
		"vm-ros-disk-write-iops":           "disk_write_iops",
		"vm-ros-disk-read-bytes-per-sec":   "disk_read_bytes_per_sec",
		"vm-ros-disk-write-bytes-per-sec":  "disk_write_bytes_per_sec",
		"vm-ros-restart-count":             "restart_count",
		"vm-ros-net-rx-bytes-per-sec":      "net_rx_bytes_per_sec",
		"vm-ros-net-tx-bytes-per-sec":      "net_tx_bytes_per_sec",
		"vm-ros-net-rx-packets-per-sec":    "net_rx_packets_per_sec",
		"vm-ros-net-tx-packets-per-sec":    "net_tx_packets_per_sec",
		"vm-ros-net-rx-drops-per-sec":      "net_rx_drops_per_sec",
		"vm-ros-net-tx-drops-per-sec":      "net_tx_drops_per_sec",
	}
	for _, q := range *rosVMQueries {
		if q.Name == "vm-ros-info" {
			if q.QueryValue != nil {
				t.Errorf("vm-ros-info should not have QueryValue")
			}
			if q.MetricKey["guest_os"] != "os" {
				t.Errorf("vm-ros-info guest_os maps from %q, want os", q.MetricKey["guest_os"])
			}
			continue
		}
		want, ok := expectedValNames[q.Name]
		if !ok {
			t.Errorf("unexpected query name %q", q.Name)
			continue
		}
		if q.QueryValue == nil || q.QueryValue.ValName != want {
			t.Errorf("query %q ValName = %v, want %q", q.Name, q.QueryValue, want)
		}
	}
}

func TestQuarterHourTimeRangeDuration(t *testing.T) {
	start := time.Date(2020, 11, 6, 18, 0, 1, 0, time.UTC)
	end := start.Add(14*time.Minute + 59*time.Second)
	if end.Sub(start) != 14*time.Minute+59*time.Second {
		t.Fatalf("unexpected quarter-hour span: %v", end.Sub(start))
	}
	nextStart := start.Add(15 * time.Minute)
	if nextStart.Hour() != 18 || nextStart.Minute() != 15 {
		t.Fatalf("next quarter start = %v, want 18:15", nextStart)
	}
}

func TestROSVMRowCSVFormat(t *testing.T) {
	row := rosVMRow{
		IntervalStart:           "2020-11-06 18:00:01 +0000 UTC",
		IntervalEnd:             "2020-11-06 18:14:59 +0000 UTC",
		VMName:                  "test-vm",
		Namespace:               "default",
		NodeName:                "worker-1",
		GuestOS:                 "linux",
		CPUUsageMC:              "100.5",
		CPURequestMC:            "2000",
		CPULimitMC:              "4000",
		MemoryUsageKiB:          "512000",
		MemoryRequestKiB:        "1048576",
		MemoryAvailableKiB:      "",
		DiskAllocatedBytes:      "10737418240",
		FilesystemUsedBytes:     "",
		FilesystemCapacityBytes: "",
		DiskReadIOPS:            "1.2",
		DiskWriteIOPS:           "0.5",
		DiskReadBytesPerSec:     "1024",
		DiskWriteBytesPerSec:    "512",
	}

	header := rosVMRow{}.csvHeader()
	if len(header) != 37 {
		t.Fatalf("expected 37 columns, got %d", len(header))
	}

	csv := row.csvRow()
	if len(csv) != len(header) {
		t.Fatalf("header/row length mismatch: %d vs %d", len(header), len(csv))
	}
	if csv[11] != "" {
		t.Fatalf("expected empty memory_available_kib for missing guest agent, got %q", csv[11])
	}
	if csv[13] != "" || csv[14] != "" {
		t.Fatalf("expected empty guest-agent filesystem fields, got used=%q capacity=%q", csv[13], csv[14])
	}
}

func TestAverageVMSamples(t *testing.T) {
	samples := []mappedValues{
		{"cpu_usage_mc": "100", "memory_usage_kib": "1024"},
		{"cpu_usage_mc": "200", "memory_usage_kib": "2048"},
	}
	avg := averageVMSamples(samples)
	if avg["cpu_usage_mc"] != "150.000000" {
		t.Fatalf("expected averaged cpu_usage_mc, got %v", avg["cpu_usage_mc"])
	}
	if avg["memory_usage_kib"] != "1536.000000" {
		t.Fatalf("expected averaged memory_usage_kib, got %v", avg["memory_usage_kib"])
	}
}

func TestAverageVMSamplesSingleSample(t *testing.T) {
	samples := []mappedValues{{"cpu_usage_mc": "42"}}
	avg := averageVMSamples(samples)
	if avg["cpu_usage_mc"] != "42" {
		t.Fatalf("single sample should pass through unchanged, got %v", avg["cpu_usage_mc"])
	}
}

func TestBuildHourlyVMRowFromSamples(t *testing.T) {
	localTime, _ := time.Parse(time.RFC3339, "2020-11-06T19:43:23Z")
	tm := localTime.UTC()
	hourRange := promv1.Range{
		Start: time.Date(tm.Year(), tm.Month(), tm.Day(), tm.Hour()-1, 0, 0, 0, tm.Location()),
		End:   time.Date(tm.Year(), tm.Month(), tm.Day(), tm.Hour()-1, 59, 59, 0, tm.Location()),
	}

	samples := []mappedValues{
		{
			"name":                 "test-vm",
			"namespace":            "default",
			"node":                 "worker-1",
			"guest_os":             "linux",
			"cpu_usage_mc":         "100",
			"cpu_request_mc":       "2000",
			"cpu_limit_mc":         "4000",
			"memory_usage_kib":     "1048576",
			"disk_allocated_bytes": "10737418240",
		},
	}

	row := buildHourlyVMRow(&hourRange, samples)
	if row.VMName != "test-vm" {
		t.Fatalf("expected vm name test-vm, got %q", row.VMName)
	}
	if row.CPUUsageSeconds != "360.000000" {
		t.Fatalf("expected hourly cpu usage seconds 360, got %q", row.CPUUsageSeconds)
	}
	if row.UptimeSeconds != "3600.000000" {
		t.Fatalf("expected 3600 uptime seconds, got %q", row.UptimeSeconds)
	}
}

func TestBuildHourlyVMRowGuestAgentAbsent(t *testing.T) {
	samples := []mappedValues{
		{
			"name":                 "legacy-vm",
			"namespace":            "legacy",
			"memory_available_kib": "",
		},
	}
	row := buildHourlyVMRow(&fakeTimeRange, samples)
	if row.VMName != "legacy-vm" {
		t.Fatalf("unexpected vm name %q", row.VMName)
	}
}

func TestBuildHourlyVMRowEmptySamples(t *testing.T) {
	row := buildHourlyVMRow(&fakeTimeRange, nil)
	if row.VMName != "" {
		t.Errorf("expected empty row for nil samples, got VMName=%q", row.VMName)
	}
}

func TestVMHourlyAggregator(t *testing.T) {
	agg := newVMHourlyAggregator(&fakeTimeRange)
	if agg.hasData() {
		t.Fatal("new aggregator should have no data")
	}
	agg.addSample(mappedResults{"default,test-vm,worker-1": {"name": "test-vm"}})
	if !agg.hasData() {
		t.Fatal("aggregator should have data after addSample")
	}
}

func TestCollectVMQuarterHour_EmptyPrometheusResults(t *testing.T) {
	log := testutils.ZapLogger(true)
	logf.SetLogger(log)

	dir := t.TempDir()
	dirCfg := &dirconfig.DirectoryConfig{Reports: dirconfig.Directory{Path: dir}}
	yearMonth := fakeTimeRange.Start.Format("200601")

	mockResults := make(mappedMockPromResult)
	for _, q := range *rosVMQueries {
		mockResults[q.QueryString] = &mockPromResult{value: model.Vector{}}
	}
	addVMGpuMockResults(mockResults)
	addVMPodVMIMockResults(mockResults)

	collector := &PrometheusCollector{
		PromConn:       mockPrometheusConnection{mappedResults: &mockResults, t: t},
		TimeSeries:     &fakeTimeRange,
		ContextTimeout: defaultContextTimeout,
	}
	agg := newVMHourlyAggregator(&fakeTimeRange)

	if err := collectVMQuarterHour(log, collector, dirCfg, yearMonth, agg); err != nil {
		t.Fatalf("collectVMQuarterHour: %v", err)
	}
	if agg.hasData() {
		t.Error("aggregator should remain empty when Prometheus returns no VM metrics")
	}
	rosPath := filepath.Join(dir, rosVMFilePrefix+yearMonth+".csv")
	if _, err := os.Stat(rosPath); !os.IsNotExist(err) {
		t.Errorf("expected no ROS VM file for empty results, stat err=%v", err)
	}
}

func TestCollectVMQuarterHour_SkipsWhenKubeVirtUnavailable(t *testing.T) {
	kubeVirtCRDChecker = func(*rest.Config) bool { return false }
	defer func() { kubeVirtCRDChecker = IsKubeVirtCRDAvailable }()

	// VM collection is gated in GenerateReports; verify the gate directly.
	if shouldCollectVMMetrics(&PrometheusCollector{RestConfig: &rest.Config{}}) {
		t.Fatal("expected VM collection disabled when KubeVirt CRD checker returns false")
	}
}

func TestCollectVMQuarterHour_WritesROSAndHourlyCostCSV(t *testing.T) {
	log := testutils.ZapLogger(true)
	logf.SetLogger(log)

	dir := t.TempDir()
	dirCfg := &dirconfig.DirectoryConfig{Reports: dirconfig.Directory{Path: dir}}
	yearMonth := fakeTimeRange.Start.Format("200601")

	mockResults := buildVMRosMockPromResults(t)
	collector := &PrometheusCollector{
		PromConn:       mockPrometheusConnection{mappedResults: &mockResults, t: t},
		TimeSeries:     quarterHourRangeFromFake(),
		ContextTimeout: defaultContextTimeout,
	}
	agg := newVMHourlyAggregator(&fakeTimeRange)

	if err := collectVMQuarterHour(log, collector, dirCfg, yearMonth, agg); err != nil {
		t.Fatalf("first collectVMQuarterHour: %v", err)
	}

	// Second quarter-hour sample for hourly aggregation.
	secondRange := *quarterHourRangeFromFake()
	secondRange.Start = secondRange.Start.Add(15 * time.Minute)
	secondRange.End = secondRange.End.Add(15 * time.Minute)
	collector.TimeSeries = &secondRange
	if err := collectVMQuarterHour(log, collector, dirCfg, yearMonth, agg); err != nil {
		t.Fatalf("second collectVMQuarterHour: %v", err)
	}

	rosPath := filepath.Join(dir, rosVMFilePrefix+yearMonth+".csv")
	if _, err := os.Stat(rosPath); err != nil {
		t.Fatalf("ROS VM report missing: %v", err)
	}
	rosRows, err := readCSVDataRows(rosPath)
	if err != nil {
		t.Fatalf("read ROS VM CSV: %v", err)
	}
	if len(rosRows) < 1 {
		t.Fatal("expected at least one ROS VM data row")
	}
	if !strings.Contains(rosRows[0], "test-vm") {
		t.Errorf("ROS row should contain test-vm, got %q", rosRows[0])
	}

	hourCollector := &PrometheusCollector{TimeSeries: &fakeTimeRange}
	if err := generateCostVMMetricsReportFromAggregator(log, hourCollector, dirCfg, yearMonth, agg); err != nil {
		t.Fatalf("generateCostVMMetricsReportFromAggregator: %v", err)
	}

	costPath := filepath.Join(dir, vmFilePrefix+yearMonth+".csv")
	if _, err := os.Stat(costPath); err != nil {
		t.Fatalf("cost VM report missing: %v", err)
	}
	costRows, err := readCSVDataRows(costPath)
	if err != nil {
		t.Fatalf("read cost VM CSV: %v", err)
	}
	if len(costRows) != 1 {
		t.Fatalf("expected 1 hourly VM row, got %d", len(costRows))
	}
	if !strings.Contains(costRows[0], "test-vm") {
		t.Errorf("cost row should contain test-vm, got %q", costRows[0])
	}
}

func TestGenerateCostVMMetricsReportFromAggregator_NilOrEmpty(t *testing.T) {
	log := testutils.ZapLogger(true)
	dir := t.TempDir()
	dirCfg := &dirconfig.DirectoryConfig{Reports: dirconfig.Directory{Path: dir}}
	yearMonth := fakeTimeRange.Start.Format("200601")
	collector := &PrometheusCollector{TimeSeries: &fakeTimeRange}

	if err := generateCostVMMetricsReportFromAggregator(log, collector, dirCfg, yearMonth, nil); err != nil {
		t.Fatalf("nil aggregator: %v", err)
	}
	emptyAgg := newVMHourlyAggregator(&fakeTimeRange)
	if err := generateCostVMMetricsReportFromAggregator(log, collector, dirCfg, yearMonth, emptyAgg); err != nil {
		t.Fatalf("empty aggregator: %v", err)
	}
	costPath := filepath.Join(dir, vmFilePrefix+yearMonth+".csv")
	if _, err := os.Stat(costPath); !os.IsNotExist(err) {
		t.Error("expected no cost VM file when aggregator has no samples")
	}
}

func TestIterateVector_MergesRosVMMetrics(t *testing.T) {
	results := mappedResults{}
	labels := model.Metric{
		"name":      "test-vm",
		"namespace": "default",
		"node":      "worker-1",
		"os":        "linux",
	}
	ts := model.Time(1604685600)

	cpuQuery := (*rosVMQueries)[0]
	results.iterateVector(model.Vector{{
		Metric: labels, Value: 100, Timestamp: ts,
	}}, cpuQuery)

	memQuery := (*rosVMQueries)[3]
	results.iterateVector(model.Vector{{
		Metric: labels, Value: 2048, Timestamp: ts,
	}}, memQuery)

	infoQuery := (*rosVMQueries)[13]
	results.iterateVector(model.Vector{{
		Metric: labels, Value: 1, Timestamp: ts,
	}}, infoQuery)

	key := generateKey(labels, cpuQuery.RowKey)
	if results[key]["cpu_usage_mc"] != "100.000000" {
		t.Errorf("cpu_usage_mc = %v", results[key]["cpu_usage_mc"])
	}
	if results[key]["memory_usage_kib"] != "2048.000000" {
		t.Errorf("memory_usage_kib = %v", results[key]["memory_usage_kib"])
	}
	if results[key]["guest_os"] != "linux" {
		t.Errorf("guest_os = %v", results[key]["guest_os"])
	}

	row := newROSVMRow(&fakeTimeRange)
	if err := getStruct(results[key], &row, make(mappedCSVStruct), key); err != nil {
		t.Fatalf("getStruct: %v", err)
	}
	if row.MemoryAvailableKiB != "" {
		t.Errorf("MemoryAvailableKiB should be empty without guest agent, got %q", row.MemoryAvailableKiB)
	}
}

func buildVMRosMockPromResults(t *testing.T) mappedMockPromResult {
	t.Helper()
	m := make(mappedMockPromResult)
	labels := model.Metric{
		"name":      "test-vm",
		"namespace": "default",
		"node":      "worker-1",
		"os":        "linux",
	}
	ts := model.Time(1604685600)
	sampleValues := map[string]float64{
		"cpu_usage_mc":              100,
		"cpu_request_mc":            2000,
		"cpu_limit_mc":              4000,
		"memory_usage_kib":          1024,
		"memory_request_kib":        2048,
		"memory_available_kib":      4096,
		"disk_allocated_bytes":      1e10,
		"filesystem_used_bytes":     1e9,
		"filesystem_capacity_bytes": 2e9,
		"disk_read_iops":            1.5,
		"disk_write_iops":           0.5,
		"disk_read_bytes_per_sec":   1024,
		"disk_write_bytes_per_sec":  512,
	}
	for _, q := range *rosVMQueries {
		if q.QueryValue == nil {
			m[q.QueryString] = &mockPromResult{
				value: model.Vector{{Metric: labels, Value: 1, Timestamp: ts}},
			}
			continue
		}
		val := sampleValues[q.QueryValue.ValName]
		m[q.QueryString] = &mockPromResult{
			value: model.Vector{{Metric: labels, Value: model.SampleValue(val), Timestamp: ts}},
		}
	}
	addVMGpuMockResults(m)
	addVMPodVMIMockResults(m)
	return m
}

func addVMGpuMockResults(m mappedMockPromResult) {
	for _, q := range *rosVMGpuQueries {
		m[q.QueryString] = &mockPromResult{value: model.Vector{}}
	}
}

func addVMPodVMIMockResults(m mappedMockPromResult) {
	for _, q := range *rosVMPodVMINameQueries {
		m[q.QueryString] = &mockPromResult{value: model.Vector{}}
	}
}

func quarterHourRangeFromFake() *promv1.Range {
	start := fakeTimeRange.Start.Add(1 * time.Second)
	end := start.Add(14*time.Minute + 59*time.Second)
	r := fakeTimeRange
	r.Start = start
	r.End = end
	return &r
}

func readCSVDataRows(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	records, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) < 2 {
		return nil, nil
	}
	var rows []string
	for _, rec := range records[1:] {
		rows = append(rows, strings.Join(rec, ","))
	}
	return rows, nil
}
