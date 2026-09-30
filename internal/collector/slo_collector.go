//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	gologr "github.com/go-logr/logr"

	metricscfgv1beta1 "github.com/project-koku/koku-metrics-operator/api/v1beta1"
	"github.com/project-koku/koku-metrics-operator/internal/dirconfig"
)

// rosSLOFilePrefix routes ros-openshift-slo-YYYYMM.csv into ROSFiles via the
// existing ros-openshift substring rule (packaging.go); no manifest change.
const rosSLOFilePrefix = "ros-openshift-slo-"

// sloVerbGroups maps backend verb_group values to the mappedValues field
// written by sloQueries. Order is stable for deterministic output.
var sloVerbGroups = []struct {
	group   string
	valName string
}{
	{"mutating", "slo_mutating_count"},
	{"read", "slo_read_count"},
	{"other", "slo_other_count"},
}

// sloRow is one cumulative bucket snapshot. Column order matches the #644
// canonical contract: hc_cluster_id | window_start | window_end | verb_group
// | le | bucket_count | collected_at. Incomplete windows are omitted by the
// emitter (absence reads as missing downstream).
type sloRow struct {
	HCClusterID string
	WindowStart string
	WindowEnd   string
	VerbGroup   string
	Le          string
	BucketCount string
	CollectedAt string
}

func (s sloRow) csvHeader() []string {
	return []string{
		"hc_cluster_id",
		"window_start",
		"window_end",
		"verb_group",
		"le",
		"bucket_count",
		"collected_at",
	}
}

func (s sloRow) csvRow() []string {
	return []string{
		s.HCClusterID,
		s.WindowStart,
		s.WindowEnd,
		s.VerbGroup,
		s.Le,
		s.BucketCount,
		s.CollectedAt,
	}
}

func (s sloRow) string() string { return strings.Join(s.csvRow(), ",") }

// shouldCollectSLO gates SLO emission on External control-plane topology
// (#644: hosted-gated on External). Standalone clusters skip quietly; the
// backend join (HostedCluster.spec.clusterID == hosted ClusterVersion ==
// hc_cluster_id) can never match there, so emitting would be pure noise.
// An empty topology (not yet collected) also skips — never fails the run.
func shouldCollectSLO(cr *metricscfgv1beta1.MetricsConfig) bool {
	if cr == nil {
		return false
	}
	return cr.Status.Topology.ControlPlaneTopology == "External"
}

// formatSLOCount renders a cumulative bucket count as a non-negative integer
// string. floatToString's 6-decimal form ("102.000000") would fail backend
// ParseInt and poison the row into skip-counters, so reformat here. Returns
// ok=false for unparseable or negative values (row omitted, never zero-filled:
// absence-is-not-healthy, #624).
func formatSLOCount(raw interface{}) (string, bool) {
	s, _ := raw.(string)
	if s == "" {
		return "", false
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) || f < 0 {
		return "", false
	}
	return strconv.FormatInt(int64(math.Round(f)), 10), true
}

// buildSLORows pivots per-le query results into contract rows, one per
// (verb_group, le) with a present count. Groups with no series for an le are
// omitted (incomplete windows omitted). collectedAt is the per-cycle snapshot
// time; the backend buckets hourly and upserts idempotently.
func buildSLORows(results mappedResults, hcClusterID string, windowStart, windowEnd, collectedAt string) []sloRow {
	var rows []sloRow
	for leKey, val := range results {
		le, _ := val["le"].(string)
		if le == "" {
			le = leKey
		}
		if le == "" {
			continue
		}
		for _, g := range sloVerbGroups {
			count, ok := formatSLOCount(val[g.valName])
			if !ok {
				continue
			}
			rows = append(rows, sloRow{
				HCClusterID: hcClusterID,
				WindowStart: windowStart,
				WindowEnd:   windowEnd,
				VerbGroup:   g.group,
				Le:          le,
				BucketCount: count,
				CollectedAt: collectedAt,
			})
		}
	}
	return rows
}

// generateSLOReport queries hosted apiserver bucket rollups and appends one
// per-cycle cumulative snapshot to ros-openshift-slo-YYYYMM.csv. Best-effort:
// missing metrics, empty results, or unknown cluster ID skip the file with an
// info log — SLO collection never fails the run (partial-prometheus-failure
// handling: other reports proceed).
//
// windowStart/windowEnd are the hourly window (captured by the caller before
// the 15-minute quarterly loop mutates the shared TimeSeries); ts is the
// instant queried (window end) and doubles as collected_at.
func generateSLOReport(log gologr.Logger, c *PrometheusCollector, dirCfg *dirconfig.DirectoryConfig, yearMonth, hcClusterID string, windowStart, windowEnd, ts time.Time) error {
	if hcClusterID == "" {
		log.Info("skipping SLO rollup report: hosted cluster ID unknown")
		return nil
	}
	log.Info(fmt.Sprintf("querying for SLO API bucket rollups for ts: %+v", ts))
	sloResults := mappedResults{}
	if err := c.getQueryResults(ts, sloQueries, &sloResults, MaxRetries); err != nil {
		return err
	}
	if len(sloResults) == 0 {
		log.Info("no apiserver bucket metrics found, skipping SLO rollup report generation")
		return nil
	}

	windowStartStr := windowStart.String()
	windowEndStr := windowEnd.String()
	// collected_at is the per-cycle snapshot time. The snapshot is taken at
	// window end (instant query at ts=End), so collected_at == window_end:
	// deterministic for golden fixtures; the backend buckets hourly and
	// upserts idempotently on the full window key.
	collectedAt := ts.String()
	rows := buildSLORows(sloResults, hcClusterID, windowStartStr, windowEndStr, collectedAt)
	if len(rows) == 0 {
		log.Info("no complete SLO bucket windows, skipping SLO rollup report generation")
		return nil
	}

	sloCSVRows := make(mappedCSVStruct, len(rows))
	for _, r := range rows {
		r := r
		sloCSVRows[r.WindowStart+"|"+r.WindowEnd+"|"+r.VerbGroup+"|"+r.Le] = r
	}
	emptySLORow := sloRow{}
	sloReport := report{
		file: &file{
			name: rosSLOFilePrefix + yearMonth + ".csv",
			path: dirCfg.Reports.Path,
		},
		data: &data{
			queryData: sloCSVRows,
			headers:   emptySLORow.csvHeader(),
			prefix:    hcClusterID + "," + windowStartStr + "," + windowEndStr,
		},
	}
	log.WithName("writeResults").Info("writing SLO rollup results to file", "filename", sloReport.file.getName())
	if err := sloReport.writeReport(); err != nil {
		return fmt.Errorf("failed to write SLO rollup report: %v", err)
	}
	return nil
}

// collectSLOHourly is the GenerateReports hook: External-gated, hourly-window
// snapshot. hourStart/hourEnd are captured before the 15-minute quarterly
// loop mutates the shared TimeSeries. Independent of namespace opt-in:
// apiserver pain evidence must flow even with no opted-in namespaces.
func collectSLOHourly(log gologr.Logger, cr *metricscfgv1beta1.MetricsConfig, c *PrometheusCollector, dirCfg *dirconfig.DirectoryConfig, yearMonth string, hourStart, hourEnd time.Time) error {
	if !shouldCollectSLO(cr) {
		return nil
	}
	return generateSLOReport(log, c, dirCfg, yearMonth, cr.Status.ClusterID, hourStart, hourEnd, hourEnd)
}
