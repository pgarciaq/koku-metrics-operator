//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	gologr "github.com/go-logr/logr"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	metricscfgv1beta1 "github.com/project-koku/koku-metrics-operator/api/v1beta1"
	"github.com/project-koku/koku-metrics-operator/internal/dirconfig"
)

// W4 fleet-capacity snapshot (#392, ADR-0334 amendment).
//
// One bounded row per current report window: raw management-cluster
// inputs the backend turns into published-sizing scenarios via shared
// librobne/hcp math. Kubernetes API supplies nodes (allocatable, labels,
// taints), HC/HCP inventory, and pressure — the nodepool/topology
// precedent, and how MCE itself computes; Prometheus supplies only the
// per-HCP QPS rates and the five MCE reference gauges.
//
// Emission gating (C1): clusters without the HyperShift APIs emit no file
// at all — absent is "not an HCP cluster", not "incomplete". Window
// semantics (C2): only the current window; observed_at is the window end;
// backfilled windows are skipped because Kubernetes inventory has no
// past. Freshness (C3): the row carries both cadence fields so serving
// never guesses. Best-effort: never fails the run.
const (
	// rosHCPCapacityFilePrefix routes the file into ROSFiles via the
	// existing ros-openshift substring rule (packaging.go).
	rosHCPCapacityFilePrefix = "ros-openshift-hcp-capacity-"

	// hcpCapacityModelVersion must equal librobne/hcp.ModelVersion; the
	// backend validates it at ingest so the two cannot silently drift.
	hcpCapacityModelVersion = "hcp-published-sizing-v1"

	// hcpCapacityCaptureInterval is the pinned hourly capture cadence
	// (owner decision 2026-10-25; SLO/APITax/Nodepool precedent).
	hcpCapacityCaptureInterval = time.Hour

	// hcpCapacityDefaultUploadCycleMinutes mirrors the CR default.
	hcpCapacityDefaultUploadCycleMinutes = int64(360)

	workerRoleLabel            = "node-role.kubernetes.io/worker"
	hypershiftControlPlaneNode = "hypershift.openshift.io/control-plane"
)

// hcpCapacityToleratedTaints are the taint keys HyperShift CP pods
// tolerate; any other taint removes a node from the eligible pool.
var hcpCapacityToleratedTaints = map[string]bool{
	"hypershift.openshift.io/control-plane": true,
	"hypershift.openshift.io/cluster":       true,
}

var (
	hostedControlPlaneGVR = schema.GroupVersionResource{
		Group:    "hypershift.openshift.io",
		Version:  "v1beta1",
		Resource: "hostedcontrolplanes",
	}
	nodeGVR = schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "nodes",
	}
	clusterVersionGVR = schema.GroupVersionResource{
		Group:    "config.openshift.io",
		Version:  "v1",
		Resource: "clusterversions",
	}
)

// The five MCE capacity gauges (live-verified 2026-10-23, MCE 2.17.3):
// optional, separately labeled references — never blended with native
// results. Exactly one series each; zero or duplicate series invalidate
// that gauge only.
const (
	mceRequestGaugeQuery    = "mce_hs_addon_request_based_hcp_capacity_gauge"
	mceLowQPSGaugeQuery     = "mce_hs_addon_low_qps_based_hcp_capacity_gauge"
	mceAverageQPSGaugeQuery = "mce_hs_addon_average_qps_based_hcp_capacity_gauge"
	mceMediumQPSGaugeQuery  = "mce_hs_addon_medium_qps_based_hcp_capacity_gauge"
	mceHighQPSGaugeQuery    = "mce_hs_addon_high_qps_based_hcp_capacity_gauge"
)

// hcpCapacityQuerier is the instant-query surface the W4 collector needs;
// PrometheusConnection satisfies it.
type hcpCapacityQuerier interface {
	Query(ctx context.Context, query string, ts time.Time, opts ...promv1.Option) (model.Value, promv1.Warnings, error)
}

// hcpWorkerCapacity is the aggregate of the eligible worker pool.
type hcpWorkerCapacity struct {
	CPUMillicores int64
	MemoryBytes   int64
	MaxPods       int64
}

// hcpK8sFacts is the Kubernetes-API half of one observation. A nil facts
// from the gather means "HyperShift APIs absent" (emit nothing); list
// errors keep the facts with the completeness flags set false — the row
// still emits, marked incomplete, so the backend suppresses headroom.
// NodesListFailed (zero value = healthy) forces placement unknown.
type hcpK8sFacts struct {
	HCs               []unstructured.Unstructured
	HCPs              []unstructured.Unstructured
	Nodes             []unstructured.Unstructured
	OCPVersion        string
	InventoryComplete bool
	NodesListFailed   bool
}

// hcpQPSQueryResult is one Ready-HCP namespace's [2m] rate query.
type hcpQPSQueryResult struct {
	Value  float64
	Failed bool
}

// hcpPromFacts is the Prometheus half: the average Ready-HCP QPS (plain
// Σ/N mean; any failed or empty per-HCP query makes it unknown — never
// the 50 basis, which is the no-ready-HCPs case decided downstream) and
// the five optional MCE references.
type hcpPromFacts struct {
	AverageReadyHCPQPS  float64
	ReadyHCPQPSKnown    bool
	MCERequestMaxHCs    *int64
	MCELowQPSMaxHCs     *int64
	MCEAverageQPSMaxHCs *int64
	MCEMediumQPSMaxHCs  *int64
	MCEHighQPSMaxHCs    *int64
	MCESampledAt        string
}

// hcpCapacityRow is the one-row CSV contract the backend ingests (#392
// plan Task 4). cluster_uuid is deliberately absent: it stays trusted
// Kafka/report metadata, never CSV-supplied.
type hcpCapacityRow struct {
	ObservedAt                time.Time
	MCESampledAt              string
	OperatorVersion           string
	OCPVersion                string
	MCEVersion                string
	NativeModelVersion        string
	CollectionIntervalMinutes int64
	UploadCycleMinutes        int64
	InventoryComplete         bool
	CurrentHCCount            int64
	ReadyHCPCount             int64
	AverageReadyHCPQPS        float64
	ReadyHCPQPSKnown          bool
	HostedAvailabilityBasis   string
	EligibleCPUMillicores     int64
	EligibleMemoryBytes       int64
	EligibleMaxPods           int64
	WorkerPlacementKnown      bool
	PressureState             string
	SharedInfrastructure      bool
	MCERequestMaxHCs          *int64
	MCELowQPSMaxHCs           *int64
	MCEAverageQPSMaxHCs       *int64
	MCEMediumQPSMaxHCs        *int64
	MCEHighQPSMaxHCs          *int64
}

func (hcpCapacityRow) csvHeader() []string {
	return []string{
		"observed_at", "mce_sampled_at", "operator_version", "ocp_version", "mce_version",
		"native_model_version", "collection_interval_minutes", "upload_cycle_minutes",
		"inventory_complete", "current_hc_count", "ready_hcp_count",
		"average_ready_hcp_qps", "ready_hcp_qps_known", "hosted_availability_basis",
		"eligible_worker_cpu_allocatable_millicores", "eligible_worker_memory_allocatable_bytes",
		"eligible_worker_max_pods", "worker_placement_known", "pressure_state",
		"mce_request_max_hcs", "mce_low_qps_max_hcs", "mce_average_qps_max_hcs",
		"mce_medium_qps_max_hcs", "mce_high_qps_max_hcs",
		// Appended (contract position 25): compact/shared-infrastructure
		// topology — management control-plane nodes doubling as workers
		// (ADR-0334 §3.2.5 caveat).
		"shared_infrastructure",
	}
}

func formatCapacityInt64(v *int64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatInt(*v, 10)
}

func (r hcpCapacityRow) csvRow() []string {
	qps := ""
	if r.ReadyHCPQPSKnown {
		qps = strconv.FormatFloat(r.AverageReadyHCPQPS, 'f', -1, 64)
	}
	return []string{
		r.ObservedAt.UTC().Format(time.RFC3339),
		r.MCESampledAt,
		r.OperatorVersion,
		r.OCPVersion,
		r.MCEVersion,
		r.NativeModelVersion,
		strconv.FormatInt(r.CollectionIntervalMinutes, 10),
		strconv.FormatInt(r.UploadCycleMinutes, 10),
		strconv.FormatBool(r.InventoryComplete),
		strconv.FormatInt(r.CurrentHCCount, 10),
		strconv.FormatInt(r.ReadyHCPCount, 10),
		qps,
		strconv.FormatBool(r.ReadyHCPQPSKnown),
		r.HostedAvailabilityBasis,
		strconv.FormatInt(r.EligibleCPUMillicores, 10),
		strconv.FormatInt(r.EligibleMemoryBytes, 10),
		strconv.FormatInt(r.EligibleMaxPods, 10),
		strconv.FormatBool(r.WorkerPlacementKnown),
		r.PressureState,
		formatCapacityInt64(r.MCERequestMaxHCs),
		formatCapacityInt64(r.MCELowQPSMaxHCs),
		formatCapacityInt64(r.MCEAverageQPSMaxHCs),
		formatCapacityInt64(r.MCEMediumQPSMaxHCs),
		formatCapacityInt64(r.MCEHighQPSMaxHCs),
		strconv.FormatBool(r.SharedInfrastructure),
	}
}

func (r hcpCapacityRow) string() string { return strings.Join(r.csvRow(), ",") }

// eligibleWorkers computes the eligible worker pool. Rules (owner
// decision 2026-10-25): any HC nodeSelector → unknown (v1 has no
// scheduler semantics); a dedicated hypershift control-plane pool, when
// labeled nodes exist, is the eligible set (counting shared workers too
// would overestimate — pods only spill under pressure); otherwise every
// worker-role node is eligible. Nodes carrying taints CP pods do not
// tolerate drop out. Unparseable quantities contribute zero (biases the
// estimate down, never up).
func eligibleWorkers(nodes, hcs []unstructured.Unstructured) (hcpWorkerCapacity, bool) {
	for i := range hcs {
		if sel, found, _ := unstructured.NestedMap(hcs[i].Object, "spec", "nodeSelector"); found && len(sel) > 0 {
			return hcpWorkerCapacity{}, false
		}
	}
	var workers []unstructured.Unstructured
	for i := range nodes {
		if _, found, _ := unstructured.NestedString(nodes[i].Object, "metadata", "labels", workerRoleLabel); found {
			workers = append(workers, nodes[i])
		}
	}
	pool := workers
	dedicated := make([]unstructured.Unstructured, 0, len(workers))
	for i := range workers {
		if _, found, _ := unstructured.NestedString(workers[i].Object, "metadata", "labels", hypershiftControlPlaneNode); found {
			dedicated = append(dedicated, workers[i])
		}
	}
	if len(dedicated) > 0 {
		pool = dedicated
	}
	var out hcpWorkerCapacity
	for i := range pool {
		if nodeHasUntoleratedTaint(pool[i]) {
			continue
		}
		cpu, mem, pods := nodeAllocatable(pool[i])
		out.CPUMillicores += cpu
		out.MemoryBytes += mem
		out.MaxPods += pods
	}
	return out, true
}

// sharedInfrastructure reports compact topology: any node carrying both a
// control-plane role label and the worker role label (OCP sizing-guide
// §3.2.5; ADR-0334 amendment). On such clusters the eligible worker pool
// includes the management control plane's own capacity, so the
// published-sizing estimate may overstate dedicated capacity — the row
// flags it and the API renders the caveat.
func sharedInfrastructure(nodes []unstructured.Unstructured) bool {
	for i := range nodes {
		_, worker, _ := unstructured.NestedString(nodes[i].Object, "metadata", "labels", workerRoleLabel)
		if !worker {
			continue
		}
		if _, master, _ := unstructured.NestedString(nodes[i].Object, "metadata", "labels", "node-role.kubernetes.io/master"); master {
			return true
		}
		if _, cp, _ := unstructured.NestedString(nodes[i].Object, "metadata", "labels", "node-role.kubernetes.io/control-plane"); cp {
			return true
		}
	}
	return false
}

func nodeHasUntoleratedTaint(node unstructured.Unstructured) bool {
	taints, found, _ := unstructured.NestedSlice(node.Object, "spec", "taints")
	if !found {
		return false
	}
	for _, raw := range taints {
		taint, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		key, _, _ := unstructured.NestedString(taint, "key")
		if key != "" && !hcpCapacityToleratedTaints[key] {
			return true
		}
	}
	return false
}

// nodeAllocatable parses status.allocatable quantities; parse failures
// contribute zero for that leg.
func nodeAllocatable(node unstructured.Unstructured) (cpuMC, memBytes, pods int64) {
	parse := func(name string) *resource.Quantity {
		raw, found, _ := unstructured.NestedString(node.Object, "status", "allocatable", name)
		if !found || raw == "" {
			return &resource.Quantity{}
		}
		q, err := resource.ParseQuantity(raw)
		if err != nil {
			return &resource.Quantity{}
		}
		return &q
	}
	cpuMC = parse("cpu").MilliValue()
	memBytes = parse("memory").Value()
	pods = parse("pods").Value()
	return
}

// hostedAvailabilityBasis aggregates the hosted control planes'
// ControllerAvailabilityPolicy (ADR-0334 rule 2 — never the management
// cluster's controlPlaneTopology). An empty policy reads as the API
// default HighlyAvailable. Zero HCs read unknown.
func hostedAvailabilityBasis(hcs []unstructured.Unstructured) string {
	if len(hcs) == 0 {
		return "unknown"
	}
	distinct := map[string]bool{}
	for i := range hcs {
		policy, _, _ := unstructured.NestedString(hcs[i].Object, "spec", "controllerAvailabilityPolicy")
		if policy == "" {
			policy = "HighlyAvailable"
		}
		distinct[policy] = true
	}
	if len(distinct) != 1 {
		return "mixed"
	}
	for policy := range distinct {
		switch policy {
		case "HighlyAvailable":
			return "highly_available"
		case "SingleReplica":
			return "single_replica"
		}
	}
	return "unknown"
}

// nodePressureState reports "pressured" when ANY node in the cluster —
// management control-plane nodes included, not just the eligible set —
// carries Memory/Disk/PID pressure; otherwise "unknown" (v1 never claims
// clear: absence of evidence is not health).
func nodePressureState(nodes []unstructured.Unstructured) string {
	for i := range nodes {
		conditions, found, _ := unstructured.NestedSlice(nodes[i].Object, "status", "conditions")
		if !found {
			continue
		}
		for _, raw := range conditions {
			cond, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			t, _, _ := unstructured.NestedString(cond, "type")
			if t != "MemoryPressure" && t != "DiskPressure" && t != "PIDPressure" {
				continue
			}
			if s, _, _ := unstructured.NestedString(cond, "status"); s == "True" {
				return "pressured"
			}
		}
	}
	return "unknown"
}

// readyHCPNamespaces lists namespaces of Ready HostedControlPlanes — the
// traffic basis MCE's average gauge uses.
func readyHCPNamespaces(hcps []unstructured.Unstructured) []string {
	var out []string
	for i := range hcps {
		if ready, found, _ := unstructured.NestedBool(hcps[i].Object, "status", "ready"); found && ready {
			out = append(out, hcps[i].GetNamespace())
		}
	}
	return out
}

// aggregateReadyQPS is the plain mean Σ/N; any failure or an empty input
// makes it unknown. The no-ready-HCPs 50-basis fallback is decided
// downstream (librobne/hcp), not here.
func aggregateReadyQPS(results []hcpQPSQueryResult) (float64, bool) {
	if len(results) == 0 {
		return 0, false
	}
	sum := 0.0
	for _, r := range results {
		if r.Failed {
			return 0, false
		}
		sum += r.Value
	}
	return sum / float64(len(results)), true
}

// mceGaugeFromVector accepts exactly one series; zero (missing) or
// multiple (duplicate) invalidate the reference — never selected or
// zero-filled.
func mceGaugeFromVector(vec model.Vector) (*int64, time.Time, bool) {
	if len(vec) != 1 {
		return nil, time.Time{}, false
	}
	// Non-finite samples are measurement error (e.g. a division-by-zero in
	// the addon's own computation) — int64(NaN) is platform-dependent
	// garbage. Treat as an invalidated reference, never a number.
	if math.IsNaN(float64(vec[0].Value)) || math.IsInf(float64(vec[0].Value), 0) {
		return nil, time.Time{}, false
	}
	v := int64(vec[0].Value)
	return &v, vec[0].Timestamp.Time(), true
}

// gatherHCPCapacityK8s lists HC/HCP/node inventory and the ClusterVersion
// via the Kubernetes API. nil facts + nil error means the HyperShift APIs
// are absent (emit nothing); list errors keep the facts with the
// completeness flags false (emit, marked incomplete).
var gatherHCPCapacityK8s = func(restConfig *rest.Config) (*hcpK8sFacts, error) {
	if restConfig == nil || !isHCPCapacityAPIAvailable(restConfig) {
		return nil, nil
	}
	dyn, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("dynamic client for HCP capacity: %w", err)
	}
	ctx := context.Background()
	facts := &hcpK8sFacts{InventoryComplete: true}

	hcs, err := listClusterResources(ctx, dyn, hostedClusterGVR)
	if err != nil {
		if meta.IsNoMatchError(err) {
			return nil, nil
		}
		facts.InventoryComplete = false
	} else {
		facts.HCs = hcs
	}

	hcps, err := listClusterResources(ctx, dyn, hostedControlPlaneGVR)
	if err != nil {
		if meta.IsNoMatchError(err) {
			return nil, nil
		}
		facts.InventoryComplete = false
	} else {
		facts.HCPs = hcps
	}

	nodes, err := listClusterResources(ctx, dyn, nodeGVR)
	if err != nil {
		facts.NodesListFailed = true
		facts.Nodes = nil
	} else {
		facts.Nodes = nodes
	}

	if cv, err := dyn.Resource(clusterVersionGVR).Get(ctx, "version", metav1.GetOptions{}); err == nil {
		facts.OCPVersion, _, _ = unstructured.NestedString(cv.Object, "status", "desired", "version")
	}
	return facts, nil
}

// isHCPCapacityAPIAvailable discovery-gates the HyperShift read (absent
// API reads as "not an HCP cluster", never as error).
func isHCPCapacityAPIAvailable(config *rest.Config) bool {
	if config == nil {
		return false
	}
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return false
	}
	resources, err := discoveryClient.ServerResourcesForGroupVersion("hypershift.openshift.io/v1beta1")
	if err != nil {
		return false
	}
	for _, r := range resources.APIResources {
		if r.Name == "hostedclusters" {
			return true
		}
	}
	return false
}

// gatherHCPCapacityProm queries the per-HCP QPS rates and the five MCE
// gauges at the window-end instant. Every instant query is evaluated at
// ts — one consistent observation timestamp.
var gatherHCPCapacityProm = func(q hcpCapacityQuerier, ts time.Time, readyNamespaces []string) hcpPromFacts {
	facts := hcpPromFacts{}
	ctx := context.Background()

	results := make([]hcpQPSQueryResult, 0, len(readyNamespaces))
	for _, ns := range readyNamespaces {
		query := fmt.Sprintf(`sum(rate(apiserver_request_total{namespace=%q}[2m]))`, ns)
		val, _, err := q.Query(ctx, query, ts)
		if err != nil {
			results = append(results, hcpQPSQueryResult{Failed: true})
			continue
		}
		vec, ok := val.(model.Vector)
		if !ok || len(vec) == 0 {
			results = append(results, hcpQPSQueryResult{Failed: true})
			continue
		}
		results = append(results, hcpQPSQueryResult{Value: float64(vec[0].Value)})
	}
	facts.AverageReadyHCPQPS, facts.ReadyHCPQPSKnown = aggregateReadyQPS(results)

	mceSampled := time.Time{}
	gauges := []struct {
		query string
		dst   **int64
	}{
		{mceRequestGaugeQuery, &facts.MCERequestMaxHCs},
		{mceLowQPSGaugeQuery, &facts.MCELowQPSMaxHCs},
		{mceAverageQPSGaugeQuery, &facts.MCEAverageQPSMaxHCs},
		{mceMediumQPSGaugeQuery, &facts.MCEMediumQPSMaxHCs},
		{mceHighQPSGaugeQuery, &facts.MCEHighQPSMaxHCs},
	}
	for _, g := range gauges {
		val, _, err := q.Query(ctx, g.query, ts)
		if err != nil {
			continue
		}
		vec, ok := val.(model.Vector)
		if !ok {
			continue
		}
		v, at, ok := mceGaugeFromVector(vec)
		if !ok {
			continue
		}
		*g.dst = v
		if at.After(mceSampled) {
			mceSampled = at
		}
	}
	if !mceSampled.IsZero() {
		facts.MCESampledAt = mceSampled.UTC().Format(time.RFC3339)
	}
	return facts
}

// buildHCPCapacityRow assembles the one-row observation.
func buildHCPCapacityRow(facts *hcpK8sFacts, prom hcpPromFacts, operatorVersion string, uploadCycleMinutes int64, windowEnd time.Time) hcpCapacityRow {
	wc, placementKnown := eligibleWorkers(facts.Nodes, facts.HCs)
	if facts.NodesListFailed {
		placementKnown = false
	}
	// mce_version stays empty in v1: reading the MultiClusterEngine CR is
	// out of RBAC scope; unknown is accepted and capability detection
	// governs the reference.
	return hcpCapacityRow{
		ObservedAt:                windowEnd,
		MCESampledAt:              prom.MCESampledAt,
		OperatorVersion:           operatorVersion,
		OCPVersion:                facts.OCPVersion,
		MCEVersion:                "",
		NativeModelVersion:        hcpCapacityModelVersion,
		CollectionIntervalMinutes: int64(hcpCapacityCaptureInterval / time.Minute),
		UploadCycleMinutes:        uploadCycleMinutes,
		InventoryComplete:         facts.InventoryComplete,
		CurrentHCCount:            int64(len(facts.HCs)),
		ReadyHCPCount:             int64(len(readyHCPNamespaces(facts.HCPs))),
		AverageReadyHCPQPS:        prom.AverageReadyHCPQPS,
		ReadyHCPQPSKnown:          prom.ReadyHCPQPSKnown,
		HostedAvailabilityBasis:   hostedAvailabilityBasis(facts.HCs),
		EligibleCPUMillicores:     wc.CPUMillicores,
		EligibleMemoryBytes:       wc.MemoryBytes,
		EligibleMaxPods:           wc.MaxPods,
		WorkerPlacementKnown:      placementKnown,
		PressureState:             nodePressureState(facts.Nodes),
		SharedInfrastructure:      sharedInfrastructure(facts.Nodes),
		MCERequestMaxHCs:          prom.MCERequestMaxHCs,
		MCELowQPSMaxHCs:           prom.MCELowQPSMaxHCs,
		MCEAverageQPSMaxHCs:       prom.MCEAverageQPSMaxHCs,
		MCEMediumQPSMaxHCs:        prom.MCEMediumQPSMaxHCs,
		MCEHighQPSMaxHCs:          prom.MCEHighQPSMaxHCs,
	}
}

// generateHCPCapacityReport writes the one-row CSV. The observed_at value
// is both the mappedCSVStruct key and the dedup prefix: re-collecting the
// same window is idempotent, other windows' rows are left untouched.
func generateHCPCapacityReport(log gologr.Logger, row hcpCapacityRow, dirCfg *dirconfig.DirectoryConfig, yearMonth string) error {
	observedAt := row.ObservedAt.UTC().Format(time.RFC3339)
	capacityReport := report{
		file: &file{
			name: rosHCPCapacityFilePrefix + yearMonth + ".csv",
			path: dirCfg.Reports.Path,
		},
		data: &data{
			queryData: mappedCSVStruct{observedAt: row},
			headers:   row.csvHeader(),
			prefix:    observedAt,
		},
	}
	log.WithName("writeResults").Info("writing HCP capacity snapshot to file", "filename", capacityReport.file.getName())
	if err := capacityReport.writeReport(); err != nil {
		return fmt.Errorf("failed to write HCP capacity report: %w", err)
	}
	return nil
}

// collectHCPCapacityHourly is the GenerateReports hook: hourly capture,
// current window only. PromConn may be nil — the native inputs still
// emit; QPS reads unknown and MCE references stay absent.
func collectHCPCapacityHourly(log gologr.Logger, cr *metricscfgv1beta1.MetricsConfig, c *PrometheusCollector, dirCfg *dirconfig.DirectoryConfig, yearMonth string, hourStart, hourEnd time.Time) error {
	if c == nil || c.RestConfig == nil {
		return nil
	}
	// Backfill guard (C2): windows older than one capture interval are
	// catch-up history — Kubernetes inventory cannot be reconstructed
	// for the past, so a misdated row would corrupt history.
	if time.Since(hourEnd) > hcpCapacityCaptureInterval {
		return nil
	}
	facts, err := gatherHCPCapacityK8s(c.RestConfig)
	if err != nil {
		return err
	}
	if facts == nil {
		// HyperShift APIs absent: not an HCP cluster — no file (C1).
		return nil
	}
	var prom hcpPromFacts
	if c.PromConn != nil {
		prom = gatherHCPCapacityProm(c.PromConn, hourEnd, readyHCPNamespaces(facts.HCPs))
	}
	uploadCycle := hcpCapacityDefaultUploadCycleMinutes
	operatorVersion := ""
	if cr != nil {
		if cr.Status.Upload.UploadCycle != nil {
			uploadCycle = *cr.Status.Upload.UploadCycle
		}
		operatorVersion = cr.Status.OperatorCommit
	}
	row := buildHCPCapacityRow(facts, prom, operatorVersion, uploadCycle, hourEnd)
	return generateHCPCapacityReport(log, row, dirCfg, yearMonth)
}
