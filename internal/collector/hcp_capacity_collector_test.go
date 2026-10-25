//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import (
	"context"
	"encoding/csv"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	gologr "github.com/go-logr/logr"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/rest"

	metricscfgv1beta1 "github.com/project-koku/koku-metrics-operator/api/v1beta1"
	"github.com/project-koku/koku-metrics-operator/internal/dirconfig"
)

// ---------------------------------------------------------------------------
// fixtures
// ---------------------------------------------------------------------------

func capacityNodeObj(name string, labels map[string]interface{}, taints []interface{}, cpu, mem, pods string, conditions ...interface{}) unstructured.Unstructured {
	node := map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":   name,
			"labels": labels,
		},
		"status": map[string]interface{}{
			"allocatable": map[string]interface{}{
				"cpu":    cpu,
				"memory": mem,
				"pods":   pods,
			},
		},
	}
	if len(taints) > 0 {
		node["spec"] = map[string]interface{}{"taints": taints}
	}
	if len(conditions) > 0 {
		node["status"].(map[string]interface{})["conditions"] = conditions
	}
	return unstructured.Unstructured{Object: node}
}

func capacityHCObj(name, namespace, policy string, deleting, paused bool, nodeSelector map[string]interface{}) unstructured.Unstructured {
	md := map[string]interface{}{"name": name, "namespace": namespace}
	if deleting {
		md["deletionTimestamp"] = "2026-10-25T00:00:00Z"
	}
	spec := map[string]interface{}{}
	if policy != "" {
		spec["controllerAvailabilityPolicy"] = policy
	}
	if paused {
		spec["pausedUntil"] = "2030-01-01T00:00:00Z"
	}
	if nodeSelector != nil {
		spec["nodeSelector"] = nodeSelector
	}
	return unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": md,
		"spec":     spec,
	}}
}

func capacityHCPObj(namespace string, ready bool) unstructured.Unstructured {
	return unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "hcp-" + namespace, "namespace": namespace},
		"status":   map[string]interface{}{"ready": ready},
	}}
}

func capacityTaint(key, effect string) map[string]interface{} {
	return map[string]interface{}{"key": key, "effect": effect}
}

func capacityCondition(condType, status string) map[string]interface{} {
	return map[string]interface{}{"type": condType, "status": status}
}

// fakeCapacityQuerier fakes the instant-query surface the W4 collector uses.
type fakeCapacityQuerier struct {
	vectors map[string]model.Vector
	errs    map[string]error
}

func (f fakeCapacityQuerier) Query(_ context.Context, query string, _ time.Time, _ ...promv1.Option) (model.Value, promv1.Warnings, error) {
	if err, ok := f.errs[query]; ok {
		return nil, nil, err
	}
	if vec, ok := f.vectors[query]; ok {
		return vec, nil, nil
	}
	return model.Vector{}, nil, nil
}

// QueryRange satisfies PrometheusConnection so the fake can be embedded
// in a PrometheusCollector; the W4 path only uses instant queries.
func (f fakeCapacityQuerier) QueryRange(_ context.Context, _ string, _ promv1.Range, _ ...promv1.Option) (model.Value, promv1.Warnings, error) {
	return nil, nil, nil
}

func capacitySample(v float64, at time.Time) *model.Sample {
	return &model.Sample{
		Metric:    model.Metric{"__name__": "gauge"},
		Value:     model.SampleValue(v),
		Timestamp: model.TimeFromUnix(at.Unix()),
	}
}

// ---------------------------------------------------------------------------
// row contract
// ---------------------------------------------------------------------------

// The header is the backend ingest contract (#392 plan Task 4); column
// changes coordinate both sides.
func TestHCPCapacityRow_Contract(t *testing.T) {
	want := []string{
		"observed_at", "mce_sampled_at", "operator_version", "ocp_version", "mce_version",
		"native_model_version", "collection_interval_minutes", "upload_cycle_minutes",
		"inventory_complete", "current_hc_count", "ready_hcp_count",
		"average_ready_hcp_qps", "ready_hcp_qps_known", "hosted_availability_basis",
		"eligible_worker_cpu_allocatable_millicores", "eligible_worker_memory_allocatable_bytes",
		"eligible_worker_max_pods", "worker_placement_known", "pressure_state",
		"mce_request_max_hcs", "mce_low_qps_max_hcs", "mce_average_qps_max_hcs",
		"mce_medium_qps_max_hcs", "mce_high_qps_max_hcs",
	}
	if got := (hcpCapacityRow{}).csvHeader(); !reflect.DeepEqual(got, want) {
		t.Fatalf("header is the backend contract — changes coordinate: %v", got)
	}
}

// ---------------------------------------------------------------------------
// eligible workers
// ---------------------------------------------------------------------------

// Default placement (no control-plane labels/taints, no HC nodeSelector):
// every worker-role node is eligible and placement is KNOWN. The master
// node (no worker label) must not count.
func TestEligibleWorkers_DefaultPlacement(t *testing.T) {
	nodes := []unstructured.Unstructured{
		capacityNodeObj("master-0", map[string]interface{}{"node-role.kubernetes.io/master": ""}, nil, "4", "16Gi", "250"),
		capacityNodeObj("worker-0", map[string]interface{}{"node-role.kubernetes.io/worker": ""}, nil, "4", "16Gi", "250"),
		capacityNodeObj("worker-1", map[string]interface{}{"node-role.kubernetes.io/worker": ""}, nil, "8", "32Gi", "500"),
		capacityNodeObj("worker-2", map[string]interface{}{"node-role.kubernetes.io/worker": ""}, nil, "52", "128Gi", "750"),
	}
	wc, known := eligibleWorkers(nodes, nil)
	if !known {
		t.Fatal("default placement must be known — fail-closed false is for ambiguous constraint sets only")
	}
	if wc.CPUMillicores != 64000 {
		t.Errorf("cpu = %d, want 64000 (4+8+52 vCPU)", wc.CPUMillicores)
	}
	if wc.MemoryBytes != 176<<30 {
		t.Errorf("memory = %d, want 176GiB", wc.MemoryBytes)
	}
	if wc.MaxPods != 1500 {
		t.Errorf("pods = %d, want 1500", wc.MaxPods)
	}
}

// Dedicated control-plane pool: when hypershift control-plane labeled
// workers exist, only they are eligible (the dedicated pool — counting
// shared workers too would overestimate; pods only spill under pressure).
func TestEligibleWorkers_DedicatedPool(t *testing.T) {
	nodes := []unstructured.Unstructured{
		capacityNodeObj("worker-shared", map[string]interface{}{"node-role.kubernetes.io/worker": ""}, nil, "8", "32Gi", "500"),
		capacityNodeObj("worker-cp-0", map[string]interface{}{
			"node-role.kubernetes.io/worker":        "",
			"hypershift.openshift.io/control-plane": "",
		}, nil, "64", "128Gi", "750"),
		capacityNodeObj("worker-cp-1", map[string]interface{}{
			"node-role.kubernetes.io/worker":        "",
			"hypershift.openshift.io/control-plane": "",
		}, nil, "64", "128Gi", "750"),
	}
	wc, known := eligibleWorkers(nodes, nil)
	if !known {
		t.Fatal("dedicated pool is computable — must be known")
	}
	if wc.CPUMillicores != 128000 {
		t.Errorf("cpu = %d, want 128000 (dedicated pool only)", wc.CPUMillicores)
	}
	if wc.MaxPods != 1500 {
		t.Errorf("pods = %d, want 1500", wc.MaxPods)
	}
}

// Any HostedCluster nodeSelector → placement unknown (v1 does not
// implement scheduler selector semantics); zero capacity, never a guess.
func TestEligibleWorkers_NodeSelectorUnknown(t *testing.T) {
	nodes := []unstructured.Unstructured{
		capacityNodeObj("worker-0", map[string]interface{}{"node-role.kubernetes.io/worker": ""}, nil, "8", "32Gi", "500"),
	}
	hcs := []unstructured.Unstructured{
		capacityHCObj("hc-a", "clusters-hc-a", "", false, false, map[string]interface{}{"node-role.kubernetes.io/infra": ""}),
	}
	wc, known := eligibleWorkers(nodes, hcs)
	if known {
		t.Fatal("HC nodeSelector must make placement unknown — v1 has no scheduler semantics")
	}
	if wc.CPUMillicores != 0 || wc.MemoryBytes != 0 || wc.MaxPods != 0 {
		t.Fatalf("unknown placement must yield zero capacity, got %+v", wc)
	}
}

// Custom taints CP pods do not tolerate drop the node from the eligible
// pool; hypershift's own control-plane taint is tolerated and keeps the
// node eligible.
func TestElelibleWorkers_CustomTaintDropped(t *testing.T) {
	nodes := []unstructured.Unstructured{
		capacityNodeObj("worker-tainted", map[string]interface{}{"node-role.kubernetes.io/worker": ""},
			[]interface{}{capacityTaint("custom.example.com/spot", "NoSchedule")}, "8", "32Gi", "500"),
		capacityNodeObj("worker-hstaint", map[string]interface{}{"node-role.kubernetes.io/worker": ""},
			[]interface{}{capacityTaint("hypershift.openshift.io/control-plane", "NoSchedule")}, "8", "32Gi", "500"),
		capacityNodeObj("worker-clean", map[string]interface{}{"node-role.kubernetes.io/worker": ""}, nil, "8", "32Gi", "500"),
	}
	wc, known := eligibleWorkers(nodes, nil)
	if !known {
		t.Fatal("no HC nodeSelector — placement known")
	}
	if wc.CPUMillicores != 16000 {
		t.Errorf("cpu = %d, want 16000 (custom-tainted node dropped, hypershift taint tolerated)", wc.CPUMillicores)
	}
}

// ---------------------------------------------------------------------------
// availability basis + pressure
// ---------------------------------------------------------------------------

func TestHostedAvailabilityBasis(t *testing.T) {
	cases := []struct {
		name string
		hcs  []unstructured.Unstructured
		want string
	}{
		{"all HA", []unstructured.Unstructured{
			capacityHCObj("a", "ns", "HighlyAvailable", false, false, nil),
			capacityHCObj("b", "ns", "HighlyAvailable", false, false, nil),
		}, "highly_available"},
		{"empty policy reads as the API default HA", []unstructured.Unstructured{
			capacityHCObj("a", "ns", "", false, false, nil),
		}, "highly_available"},
		{"all single replica", []unstructured.Unstructured{
			capacityHCObj("a", "ns", "SingleReplica", false, false, nil),
		}, "single_replica"},
		{"mixed", []unstructured.Unstructured{
			capacityHCObj("a", "ns", "HighlyAvailable", false, false, nil),
			capacityHCObj("b", "ns", "SingleReplica", false, false, nil),
		}, "mixed"},
		{"no hosted clusters", nil, "unknown"},
	}
	for _, tc := range cases {
		if got := hostedAvailabilityBasis(tc.hcs); got != tc.want {
			t.Errorf("%s: basis = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestNodePressureState(t *testing.T) {
	cases := []struct {
		name  string
		nodes []unstructured.Unstructured
		want  string
	}{
		{"memory pressure", []unstructured.Unstructured{
			capacityNodeObj("n", nil, nil, "4", "16Gi", "250", capacityCondition("MemoryPressure", "True")),
		}, "pressured"},
		{"pid pressure on any node", []unstructured.Unstructured{
			capacityNodeObj("a", nil, nil, "4", "16Gi", "250", capacityCondition("Ready", "True")),
			capacityNodeObj("b", nil, nil, "4", "16Gi", "250", capacityCondition("PIDPressure", "True")),
		}, "pressured"},
		{"conditions false → unknown, never clear", []unstructured.Unstructured{
			capacityNodeObj("n", nil, nil, "4", "16Gi", "250",
				capacityCondition("MemoryPressure", "False"), capacityCondition("DiskPressure", "False")),
		}, "unknown"},
		{"no conditions → unknown", []unstructured.Unstructured{
			capacityNodeObj("n", nil, nil, "4", "16Gi", "250"),
		}, "unknown"},
	}
	for _, tc := range cases {
		if got := nodePressureState(tc.nodes); got != tc.want {
			t.Errorf("%s: pressure = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// QPS aggregation + MCE gauge parsing
// ---------------------------------------------------------------------------

// The mean is the plain Σ/N over Ready-HCP queries; ANY failed or empty
// query makes the whole average unknown — never a partial mean and never
// the 50 basis (that is the no-ready-HCPs case, decided downstream).
func TestAggregateReadyQPS(t *testing.T) {
	mean, known := aggregateReadyQPS([]hcpQPSQueryResult{{Value: 10}, {Value: 30}})
	if !known || mean != 20 {
		t.Fatalf("mean = %v known = %v, want 20 true", mean, known)
	}
	if _, known := aggregateReadyQPS([]hcpQPSQueryResult{{Value: 10}, {Failed: true}}); known {
		t.Fatal("any failed query must make the average unknown")
	}
	if _, known := aggregateReadyQPS(nil); known {
		t.Fatal("no ready HCPs must not claim a known average here (fallback decided downstream)")
	}
}

// Exactly one series → the gauge value and its scrape timestamp; zero or
// duplicate series → nil (the reference is unavailable, never selected or
// zero-filled).
func TestMCEGaugeFromVector(t *testing.T) {
	at := time.Unix(1792800000, 0)
	v, ts, ok := mceGaugeFromVector(model.Vector{capacitySample(8, at)})
	if !ok || v == nil || *v != 8 || !ts.Equal(at) {
		t.Fatalf("single series: v=%v ts=%v ok=%v, want 8/%v/true", v, ts, ok, at)
	}
	if _, _, ok := mceGaugeFromVector(model.Vector{}); ok {
		t.Fatal("missing series must not be ok")
	}
	if _, _, ok := mceGaugeFromVector(model.Vector{capacitySample(8, at), capacitySample(4, at)}); ok {
		t.Fatal("duplicate series must not be ok")
	}
}

// ---------------------------------------------------------------------------
// row assembly
// ---------------------------------------------------------------------------

func TestBuildHCPCapacityRow(t *testing.T) {
	facts := &hcpK8sFacts{
		HCs: []unstructured.Unstructured{
			capacityHCObj("hc-live", "clusters-hc-live", "HighlyAvailable", false, false, nil),
			capacityHCObj("hc-deleting", "clusters-hc-deleting", "HighlyAvailable", true, false, nil),
			capacityHCObj("hc-paused", "clusters-hc-paused", "SingleReplica", false, true, nil),
		},
		HCPs: []unstructured.Unstructured{
			capacityHCPObj("clusters-hc-live", true),
			capacityHCPObj("clusters-hc-deleting", false),
			capacityHCPObj("clusters-hc-paused", true),
		},
		Nodes: []unstructured.Unstructured{
			capacityNodeObj("worker-0", map[string]interface{}{"node-role.kubernetes.io/worker": ""}, nil, "8", "32Gi", "500"),
		},
		OCPVersion:        "4.22.17",
		InventoryComplete: true,
	}
	mceEight, mceFour := int64(8), int64(4)
	prom := hcpPromFacts{
		AverageReadyHCPQPS: 42.5,
		ReadyHCPQPSKnown:   true,
		MCERequestMaxHCs:   &mceEight,
		MCEMediumQPSMaxHCs: &mceFour,
		MCESampledAt:       time.Unix(1792800000, 0).UTC().Format(time.RFC3339),
	}
	windowEnd := time.Date(2026, 10, 25, 14, 59, 59, 0, time.UTC)

	row := buildHCPCapacityRow(facts, prom, "deadbeef", 360, windowEnd)

	// Occupancy counts every visible HC — deleting and paused included.
	if row.CurrentHCCount != 3 {
		t.Errorf("current_hc_count = %d, want 3 (deleting+paused counted)", row.CurrentHCCount)
	}
	if row.ReadyHCPCount != 2 {
		t.Errorf("ready_hcp_count = %d, want 2", row.ReadyHCPCount)
	}
	if !row.InventoryComplete {
		t.Error("inventory_complete must pass through true")
	}
	// Mixed availability (HA + SingleReplica) and single worker.
	if row.HostedAvailabilityBasis != "mixed" {
		t.Errorf("basis = %q, want mixed", row.HostedAvailabilityBasis)
	}
	if row.EligibleCPUMillicores != 8000 || row.EligibleMaxPods != 500 {
		t.Errorf("eligible workers = %+v", row.EligibleCPUMillicores)
	}
	if !row.WorkerPlacementKnown {
		t.Error("default placement must be known")
	}
	if row.PressureState != "unknown" {
		t.Errorf("pressure = %q, want unknown (v1 never claims clear)", row.PressureState)
	}
	// Versions + cadence.
	if row.OCPVersion != "4.22.17" || row.OperatorVersion != "deadbeef" {
		t.Errorf("versions = %q/%q", row.OCPVersion, row.OperatorVersion)
	}
	if row.MCEVersion != "" {
		t.Errorf("mce_version = %q, want empty (best-effort, v1 has no source)", row.MCEVersion)
	}
	if row.CollectionIntervalMinutes != 60 {
		t.Errorf("collection_interval = %d, want 60 (hourly)", row.CollectionIntervalMinutes)
	}
	if row.UploadCycleMinutes != 360 {
		t.Errorf("upload_cycle = %d, want 360", row.UploadCycleMinutes)
	}
	if !row.ObservedAt.Equal(windowEnd) {
		t.Errorf("observed_at = %v, want window end %v", row.ObservedAt, windowEnd)
	}
	// MCE references: present ones pass, absent ones nil.
	if row.MCERequestMaxHCs == nil || *row.MCERequestMaxHCs != 8 {
		t.Error("request gauge reference must pass through")
	}
	if row.MCEMediumQPSMaxHCs == nil || *row.MCEMediumQPSMaxHCs != 4 {
		t.Error("medium gauge reference must pass through")
	}
	if row.MCELowQPSMaxHCs != nil || row.MCEAverageQPSMaxHCs != nil || row.MCEHighQPSMaxHCs != nil {
		t.Error("absent gauge references must be nil, never zero")
	}
	if !row.ReadyHCPQPSKnown || row.AverageReadyHCPQPS != 42.5 {
		t.Errorf("qps = %v known = %v", row.AverageReadyHCPQPS, row.ReadyHCPQPSKnown)
	}

	// CSV rendering: nil MCE refs render empty; booleans render true/false.
	csvr := row.csvRow()
	if csvr[19] != "8" || csvr[20] != "" || csvr[22] != "4" {
		t.Errorf("mce columns = %v..%v, want 8,\"\",...,4", csvr[19], csvr[22])
	}
	if csvr[8] != "true" || csvr[17] != "true" {
		t.Errorf("boolean columns must render true: inventory=%q placement=%q", csvr[8], csvr[17])
	}
}

// ---------------------------------------------------------------------------
// emission (hook) via stub points
// ---------------------------------------------------------------------------

func capacityTestCR() *metricscfgv1beta1.MetricsConfig {
	cr := &metricscfgv1beta1.MetricsConfig{}
	cr.Status.OperatorCommit = "deadbeef"
	cycle := int64(360)
	cr.Status.Upload.UploadCycle = &cycle
	return cr
}

func capacityTestCollector(querier PrometheusConnection) *PrometheusCollector {
	return &PrometheusCollector{RestConfig: &rest.Config{}, PromConn: querier}
}

func readCapacityCSV(t *testing.T, dirCfg *dirconfig.DirectoryConfig, yearMonth string) [][]string {
	t.Helper()
	path := filepath.Join(dirCfg.Reports.Path, rosHCPCapacityFilePrefix+yearMonth+".csv")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open capacity csv: %v", err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatalf("read capacity csv: %v", err)
	}
	return rows
}

func stubCapacityGathers(t *testing.T, facts *hcpK8sFacts, k8sErr error, prom hcpPromFacts) {
	t.Helper()
	origK8s, origProm := gatherHCPCapacityK8s, gatherHCPCapacityProm
	gatherHCPCapacityK8s = func(*rest.Config) (*hcpK8sFacts, error) { return facts, k8sErr }
	gatherHCPCapacityProm = func(hcpCapacityQuerier, time.Time, []string) hcpPromFacts { return prom }
	t.Cleanup(func() { gatherHCPCapacityK8s, gatherHCPCapacityProm = origK8s, origProm })
}

func capacityTempDirCfg(t *testing.T) *dirconfig.DirectoryConfig {
	t.Helper()
	cfg := &dirconfig.DirectoryConfig{}
	cfg.Reports = dirconfig.Directory{Path: t.TempDir()}
	return cfg
}

func TestCollectHCPCapacityHourly_EmitsCurrentWindow(t *testing.T) {
	facts := &hcpK8sFacts{
		HCs:               []unstructured.Unstructured{capacityHCObj("hc-a", "ns", "HighlyAvailable", false, false, nil)},
		HCPs:              []unstructured.Unstructured{capacityHCPObj("ns", true)},
		Nodes:             []unstructured.Unstructured{capacityNodeObj("w", map[string]interface{}{"node-role.kubernetes.io/worker": ""}, nil, "8", "32Gi", "500")},
		OCPVersion:        "4.22.17",
		InventoryComplete: true,
	}
	stubCapacityGathers(t, facts, nil, hcpPromFacts{})
	dirCfg := capacityTempDirCfg(t)
	hourEnd := time.Now().UTC().Truncate(time.Minute)

	if err := collectHCPCapacityHourly(gologr.Discard(), capacityTestCR(), capacityTestCollector(fakeCapacityQuerier{}), dirCfg, "202610", hourEnd.Add(-time.Hour), hourEnd); err != nil {
		t.Fatalf("collect: %v", err)
	}

	rows := readCapacityCSV(t, dirCfg, "202610")
	if len(rows) != 2 {
		t.Fatalf("want header + 1 row, got %d rows", len(rows))
	}
	if rows[1][0] != hourEnd.Format(time.RFC3339) {
		t.Errorf("observed_at = %q, want window end %q", rows[1][0], hourEnd.Format(time.RFC3339))
	}
	if rows[1][9] != "1" {
		t.Errorf("current_hc_count = %q, want 1", rows[1][9])
	}

	// Idempotency: re-collecting the same window must not duplicate the row.
	if err := collectHCPCapacityHourly(gologr.Discard(), capacityTestCR(), capacityTestCollector(fakeCapacityQuerier{}), dirCfg, "202610", hourEnd.Add(-time.Hour), hourEnd); err != nil {
		t.Fatalf("re-collect: %v", err)
	}
	rows = readCapacityCSV(t, dirCfg, "202610")
	if len(rows) != 2 {
		t.Fatalf("same-window recollect must dedup: want 2 rows, got %d", len(rows))
	}
}

// Backfilled windows (older than one capture interval) are skipped —
// Kubernetes inventory has no past, and a misdated row would corrupt
// history (C2).
func TestCollectHCPCapacityHourly_BackfillSkipped(t *testing.T) {
	stubCapacityGathers(t, &hcpK8sFacts{InventoryComplete: true}, nil, hcpPromFacts{})
	dirCfg := capacityTempDirCfg(t)
	oldEnd := time.Now().UTC().Add(-2 * time.Hour)

	if err := collectHCPCapacityHourly(gologr.Discard(), capacityTestCR(), capacityTestCollector(fakeCapacityQuerier{}), dirCfg, "202610", oldEnd.Add(-time.Hour), oldEnd); err != nil {
		t.Fatalf("collect: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dirCfg.Reports.Path, rosHCPCapacityFilePrefix+"202610.csv")); !os.IsNotExist(err) {
		t.Fatal("backfilled window must emit no file")
	}
}

// HyperShift APIs absent → no file at all (not an HCP cluster — this is
// emission gating, not "incomplete").
func TestCollectHCPCapacityHourly_APIAbsent(t *testing.T) {
	stubCapacityGathers(t, nil, nil, hcpPromFacts{}) // nil facts = API absent
	dirCfg := capacityTempDirCfg(t)
	now := time.Now().UTC()

	if err := collectHCPCapacityHourly(gologr.Discard(), capacityTestCR(), capacityTestCollector(fakeCapacityQuerier{}), dirCfg, "202610", now.Add(-time.Hour), now); err != nil {
		t.Fatalf("collect: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dirCfg.Reports.Path, rosHCPCapacityFilePrefix+"202610.csv")); !os.IsNotExist(err) {
		t.Fatal("HyperShift API absence must emit no file")
	}
}

// A real list error (APIs present) still emits — with
// inventory_complete=false so the backend suppresses headroom.
func TestCollectHCPCapacityHourly_ListErrorIncomplete(t *testing.T) {
	stubCapacityGathers(t, &hcpK8sFacts{InventoryComplete: false}, nil, hcpPromFacts{})
	dirCfg := capacityTempDirCfg(t)
	now := time.Now().UTC()

	if err := collectHCPCapacityHourly(gologr.Discard(), capacityTestCR(), capacityTestCollector(fakeCapacityQuerier{}), dirCfg, "202610", now.Add(-time.Hour), now); err != nil {
		t.Fatalf("collect: %v", err)
	}
	rows := readCapacityCSV(t, dirCfg, "202610")
	if len(rows) != 2 {
		t.Fatalf("want header + 1 row, got %d", len(rows))
	}
	if rows[1][8] != "false" {
		t.Errorf("inventory_complete = %q, want false on list error", rows[1][8])
	}
}

// ---------------------------------------------------------------------------
// prom gathering through the fake querier
// ---------------------------------------------------------------------------

// Per-HCP QPS: each Ready-HCP namespace gets one [2m] rate query; a failed
// OR EMPTY query makes the average unknown; the mean is Σ/N.
func TestGatherHCPCapacityProm_QPS(t *testing.T) {
	q1 := `sum(rate(apiserver_request_total{namespace="ns-a"}[2m]))`
	q2 := `sum(rate(apiserver_request_total{namespace="ns-b"}[2m]))`
	ts := time.Now().UTC()

	both := fakeCapacityQuerier{vectors: map[string]model.Vector{
		q1: {capacitySample(10, ts)},
		q2: {capacitySample(30, ts)},
	}}
	f := gatherHCPCapacityProm(both, ts, []string{"ns-a", "ns-b"})
	if !f.ReadyHCPQPSKnown || f.AverageReadyHCPQPS != 20 {
		t.Fatalf("mean = %v known = %v, want 20 true", f.AverageReadyHCPQPS, f.ReadyHCPQPSKnown)
	}

	empty := fakeCapacityQuerier{vectors: map[string]model.Vector{
		q1: {capacitySample(10, ts)},
		// q2 absent → empty vector
	}}
	f = gatherHCPCapacityProm(empty, ts, []string{"ns-a", "ns-b"})
	if f.ReadyHCPQPSKnown {
		t.Fatal("empty per-HCP query must make the average unknown")
	}

	errd := fakeCapacityQuerier{errs: map[string]error{q2: io.EOF}}
	f = gatherHCPCapacityProm(errd, ts, []string{"ns-a", "ns-b"})
	if f.ReadyHCPQPSKnown {
		t.Fatal("failed per-HCP query must make the average unknown")
	}
}

// MCE references: exactly-one-series gauges pass with the scrape
// timestamp; duplicates invalidate only that gauge.
func TestGatherHCPCapacityProm_MCEGauges(t *testing.T) {
	ts := time.Now().UTC()
	req := `mce_hs_addon_request_based_hcp_capacity_gauge`
	low := `mce_hs_addon_low_qps_based_hcp_capacity_gauge`
	dup := `mce_hs_addon_high_qps_based_hcp_capacity_gauge`
	querier := fakeCapacityQuerier{vectors: map[string]model.Vector{
		req: {capacitySample(8, ts)},
		low: {capacitySample(8, ts)},
		dup: {capacitySample(2, ts), capacitySample(3, ts)},
	}}
	f := gatherHCPCapacityProm(querier, ts, nil)
	if f.MCERequestMaxHCs == nil || *f.MCERequestMaxHCs != 8 {
		t.Error("single-series request gauge must pass through")
	}
	if f.MCEHighQPSMaxHCs != nil {
		t.Error("duplicate-series gauge must be invalidated")
	}
	if f.MCEAverageQPSMaxHCs != nil {
		t.Error("absent gauge must be nil")
	}
	if f.MCESampledAt != ts.Format(time.RFC3339) {
		t.Errorf("mce_sampled_at = %q, want %q", f.MCESampledAt, ts.Format(time.RFC3339))
	}
}

var _ hcpCapacityQuerier = fakeCapacityQuerier{}
var _ csvStruct = hcpCapacityRow{}
var _ = metav1.Now // keep the import if fixtures evolve
