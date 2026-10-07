//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	gologr "github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	"github.com/project-koku/koku-metrics-operator/internal/dirconfig"
)

// NodePool inventory snapshots (W3 slice, #660 Child A → #673).
//
// Per-HC NodePool state (spec/status replicas, autoscaling) read from the
// Kubernetes API — not Prometheus — once per collection, then emitted as
// hourly-windowed rows the backend upserts idempotently (SLO-handshake
// pattern: strict parser, skip-with-counters, hourly PK). Scaled-to-0 :=
// spec AND status zero (mid-transition reads unknown, never idle-proof);
// the backend rule (Child B) interprets, this file only observes.
//
// Best-effort like the machineset mapping: missing HyperShift APIs
// degrade to an empty row set (skip the file, never fail the run);
// other errors return for the caller to log-and-continue.
const rosNodepoolFilePrefix = "ros-openshift-nodepool-"

var (
	nodePoolGVR = schema.GroupVersionResource{
		Group:    "hypershift.openshift.io",
		Version:  "v1beta1",
		Resource: "nodepools",
	}
	hostedClusterGVR = schema.GroupVersionResource{
		Group:    "hypershift.openshift.io",
		Version:  "v1beta1",
		Resource: "hostedclusters",
	}
)

type nodepoolRow struct {
	HCClusterID    string
	PoolName       string
	WindowStart    string
	WindowEnd      string
	SpecReplicas   int64
	StatusReplicas int64
	Autoscaling    string
	CollectedAt    string
	Source         string
}

func (r nodepoolRow) csvHeader() []string {
	return []string{
		"hc_cluster_id", "pool_name", "window_start", "window_end",
		"spec_replicas", "status_replicas", "autoscaling",
		"collected_at", "source",
	}
}

func (r nodepoolRow) csvRow() []string {
	return []string{
		r.HCClusterID, r.PoolName, r.WindowStart, r.WindowEnd,
		strconv.FormatInt(r.SpecReplicas, 10), strconv.FormatInt(r.StatusReplicas, 10),
		r.Autoscaling, r.CollectedAt, r.Source,
	}
}

func (r nodepoolRow) string() string { return strings.Join(r.csvRow(), ",") }

// buildNodepoolRows joins NodePool objects to HostedCluster IDs
// (spec.clusterName → HostedCluster name → spec.clusterID). Pools whose
// HC cannot be resolved, or whose replica counts are unreadable, are
// skipped (counted by the caller): never fabricate zeros.
func buildNodepoolRows(pools []unstructured.Unstructured, hcByName map[string]string, windowStart, windowEnd, collectedAt string) (rows []nodepoolRow, skipped int) {
	for i := range pools {
		obj := &pools[i]
		ns := obj.GetNamespace()
		name := obj.GetName()
		clusterName, _, _ := unstructured.NestedString(obj.Object, "spec", "clusterName")
		hcID := hcByName[ns+"/"+clusterName]
		if hcID == "" {
			skipped++
			continue
		}
		specRep, found, err := unstructured.NestedInt64(obj.Object, "spec", "replicas")
		if err != nil || !found || specRep < 0 {
			skipped++
			continue
		}
		statusRep, found, err := unstructured.NestedInt64(obj.Object, "status", "replicas")
		if err != nil || !found || statusRep < 0 {
			skipped++
			continue
		}
		auto := ""
		if min, found, _ := unstructured.NestedInt64(obj.Object, "spec", "autoScaling", "min"); found {
			max, _, _ := unstructured.NestedInt64(obj.Object, "spec", "autoScaling", "max")
			auto = strconv.FormatInt(min, 10) + "-" + strconv.FormatInt(max, 10)
		}
		rows = append(rows, nodepoolRow{
			HCClusterID: hcID, PoolName: name,
			WindowStart: windowStart, WindowEnd: windowEnd,
			SpecReplicas: specRep, StatusReplicas: statusRep,
			Autoscaling: auto, CollectedAt: collectedAt, Source: "hypershift",
		})
	}
	return rows, skipped
}

// isNodepoolAPIAvailable discovery-gates the HyperShift NodePool read
// (mirrors the machineset mapping precedent: absent API reads as empty,
// never as error).
func isNodepoolAPIAvailable(config *rest.Config) bool {
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
		if r.Name == "nodepools" {
			return true
		}
	}
	return false
}

// collectNodepoolRows lists NodePools + HostedClusters and builds rows.
// Nil RestConfig or missing APIs yield empty (skip); other errors
// propagate for log-and-continue.
func collectNodepoolRows(restConfig *rest.Config, log gologr.Logger, windowStart, windowEnd, collectedAt string) ([]nodepoolRow, int, error) {
	if restConfig == nil || !isNodepoolAPIAvailable(restConfig) {
		return nil, 0, nil
	}
	dynClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, 0, fmt.Errorf("dynamic client for NodePool inventory: %w", err)
	}
	ctx := context.Background()
	pools, err := listClusterResources(ctx, dynClient, nodePoolGVR)
	if err != nil {
		if meta.IsNoMatchError(err) {
			return nil, 0, nil
		}
		return nil, 0, fmt.Errorf("list nodepools: %w", err)
	}
	hcs, err := listClusterResources(ctx, dynClient, hostedClusterGVR)
	if err != nil {
		if meta.IsNoMatchError(err) {
			return nil, 0, nil
		}
		return nil, 0, fmt.Errorf("list hostedclusters: %w", err)
	}
	hcByName := make(map[string]string, len(hcs))
	for i := range hcs {
		name := hcs[i].GetNamespace() + "/" + hcs[i].GetName()
		if id, _, _ := unstructured.NestedString(hcs[i].Object, "spec", "clusterID"); id != "" {
			hcByName[name] = id
		}
	}
	rows, skipped := buildNodepoolRows(pools, hcByName, windowStart, windowEnd, collectedAt)
	if skipped > 0 {
		log.Info(fmt.Sprintf("NodePool inventory skipped %d unresolvable pools", skipped))
	}
	return rows, skipped, nil
}

// collectNodepoolRowsForCollector is the stub point for coarse-suite
// fixtures (mirrors collectNodeMachineSetNamesForCollector).
var collectNodepoolRowsForCollector = collectNodepoolRows

// generateNodepoolReport writes one hourly NodePool snapshot file.
// Empty row sets skip the file (no empty-file noise in packaging).
func generateNodepoolReport(log gologr.Logger, restConfig *rest.Config, dirCfg *dirconfig.DirectoryConfig, yearMonth, windowStart, windowEnd string, ts time.Time) error {
	rows, _, err := collectNodepoolRowsForCollector(restConfig, log, windowStart, windowEnd, ts.String())
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		log.Info("no NodePool inventory found, skipping nodepool report generation")
		return nil
	}
	poolCSVRows := make(mappedCSVStruct, len(rows))
	for _, r := range rows {
		r := r
		poolCSVRows[r.HCClusterID+"|"+r.PoolName+"|"+r.WindowStart] = r
	}
	emptyRow := nodepoolRow{}
	poolReport := report{
		file: &file{
			name: rosNodepoolFilePrefix + yearMonth + ".csv",
			path: dirCfg.Reports.Path,
		},
		data: &data{
			queryData: poolCSVRows,
			headers:   emptyRow.csvHeader(),
			prefix:    windowStart + "," + windowEnd,
		},
	}
	log.WithName("writeResults").Info("writing NodePool inventory results to file", "filename", poolReport.file.getName())
	if err := poolReport.writeReport(); err != nil {
		return fmt.Errorf("failed to write NodePool inventory report: %w", err)
	}
	return nil
}

// collectNodepoolHourly is the GenerateReports hook: ungated hourly
// snapshot, best-effort. Independent of namespace opt-in, topology, and
// VM collection; never fails the run.
func collectNodepoolHourly(log gologr.Logger, restConfig *rest.Config, dirCfg *dirconfig.DirectoryConfig, yearMonth string, hourStart, hourEnd time.Time) error {
	return generateNodepoolReport(log, restConfig, dirCfg, yearMonth, hourStart.String(), hourEnd.String(), hourEnd)
}
