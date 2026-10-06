//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	gologr "github.com/go-logr/logr"

	"github.com/project-koku/koku-metrics-operator/internal/dirconfig"
)

// rosAPITaxFilePrefix routes ros-openshift-apitax-YYYYMM.csv into the
// packaged reports (globbed by directory, no manifest registration).
const rosAPITaxFilePrefix = "ros-openshift-apitax-"

// apiTaxRow is one cumulative webhook snapshot. Column order is the thin-W5
// contract (locked here for the backend step): webhook_name | window_start |
// window_end | le | bucket_count | total_count | rejected_count |
// collected_at | source. Totals are denormalized onto every bucket row of
// that webhook (CSVs stay single-shape; the backend reads totals off any
// row). Empty totals mean the series was absent — never zero-filled.
type apiTaxRow struct {
	WebhookName   string
	WindowStart   string
	WindowEnd     string
	Le            string
	BucketCount   string
	TotalCount    string
	RejectedCount string
	CollectedAt   string
	Source        string
}

func (r apiTaxRow) csvHeader() []string {
	return []string{
		"webhook_name",
		"window_start",
		"window_end",
		"le",
		"bucket_count",
		"total_count",
		"rejected_count",
		"collected_at",
		"source",
	}
}

func (r apiTaxRow) csvRow() []string {
	return []string{
		r.WebhookName,
		r.WindowStart,
		r.WindowEnd,
		r.Le,
		r.BucketCount,
		r.TotalCount,
		r.RejectedCount,
		r.CollectedAt,
		r.Source,
	}
}

func (r apiTaxRow) string() string { return strings.Join(r.csvRow(), ",") }

// buildAPITaxRows pivots per-webhook query results into contract rows, one
// per (webhook, le, source) with a present bucket count; totals and rejected
// totals join by (webhook, source). Groups with no bucket series are omitted.
// Float-equal le boundaries merge (same lesson as SLO: per-job schemas
// differ). collectedAt is the per-cycle snapshot time.
func buildAPITaxRows(results mappedResults, windowStart, windowEnd, collectedAt string) []apiTaxRow {
	type key struct{ webhook, le, source string }
	type accum struct {
		le, source                string
		bucket, total, rejected   int64
		hasBucket, hasTotal, hasR bool
	}
	merged := map[key]*accum{}
	var order []key
	acc := func(webhook, le, source string) *accum {
		k := key{webhook, le, source}
		a, dup := merged[k]
		if !dup {
			a = &accum{le: le, source: source}
			merged[k] = a
			order = append(order, k)
		}
		return a
	}
	for _, val := range results {
		webhook, _ := val["webhook"].(string)
		if webhook == "" {
			continue
		}
		source, _ := val["source"].(string)
		if source == "" {
			continue
		}
		if raw, ok := val["apitax_latency_count"]; ok {
			leRaw, _ := val["le"].(string)
			canon, ok := canonicalSLOLe(leRaw)
			if !ok {
				continue
			}
			countStr, ok := formatSLOCount(raw)
			if !ok {
				continue
			}
			count, err := strconv.ParseInt(countStr, 10, 64)
			if err != nil {
				continue
			}
			a := acc(webhook, canon, source)
			a.bucket += count
			a.hasBucket = true
		}
		if raw, ok := val["apitax_total_count"]; ok {
			countStr, ok := formatSLOCount(raw)
			if !ok {
				continue
			}
			count, err := strconv.ParseInt(countStr, 10, 64)
			if err != nil {
				continue
			}
			// Totals attach to every bucket row of this webhook+source;
			// create a carrier on the empty le if no buckets exist yet —
			// the row filter below drops le-less carriers without buckets.
			a := acc(webhook, "", source)
			a.total += count
			a.hasTotal = true
		}
		if raw, ok := val["apitax_rejected_count"]; ok {
			countStr, ok := formatSLOCount(raw)
			if !ok {
				continue
			}
			count, err := strconv.ParseInt(countStr, 10, 64)
			if err != nil {
				continue
			}
			a := acc(webhook, "", source)
			a.rejected += count
			a.hasR = true
		}
	}
	// Spread totals onto bucket rows; drop carriers that never got buckets.
	totals := map[[2]string][2]int64{}
	hasTotals := map[[2]string][2]bool{}
	for k, a := range merged {
		if k.le == "" {
			totals[[2]string{k.webhook, k.source}] = [2]int64{a.total, a.rejected}
			hasTotals[[2]string{k.webhook, k.source}] = [2]bool{a.hasTotal, a.hasR}
		}
	}
	rows := make([]apiTaxRow, 0, len(order))
	for _, k := range order {
		if k.le == "" {
			continue
		}
		a := merged[k]
		if !a.hasBucket {
			continue
		}
		t := totals[[2]string{k.webhook, k.source}]
		ht := hasTotals[[2]string{k.webhook, k.source}]
		row := apiTaxRow{
			WebhookName: k.webhook,
			WindowStart: windowStart,
			WindowEnd:   windowEnd,
			Le:          k.le,
			BucketCount: strconv.FormatInt(a.bucket, 10),
			CollectedAt: collectedAt,
			Source:      k.source,
		}
		if ht[0] {
			row.TotalCount = strconv.FormatInt(t[0], 10)
		}
		if ht[1] {
			row.RejectedCount = strconv.FormatInt(t[1], 10)
		}
		rows = append(rows, row)
	}
	return rows
}

// generateAPITaxReport queries admission-webhook rollups and appends one
// per-cycle cumulative snapshot to ros-openshift-apitax-YYYYMM.csv.
// Best-effort like SLO: missing metrics or empty results skip the file with
// an info log — collection never fails the run. Runs on every topology
// (ADR-0335: useful on any OpenShift, both planes).
func generateAPITaxReport(log gologr.Logger, c *PrometheusCollector, dirCfg *dirconfig.DirectoryConfig, yearMonth, windowStart, windowEnd string, ts time.Time) error {
	log.Info(fmt.Sprintf("querying for API tax webhook rollups for ts: %+v", ts))
	taxResults := mappedResults{}
	if err := c.getQueryResults(ts, apitaxQueries, &taxResults, MaxRetries); err != nil {
		return err
	}
	if len(taxResults) == 0 {
		log.Info("no admission webhook metrics found, skipping API tax report generation")
		return nil
	}

	windowStartStr := windowStart
	windowEndStr := windowEnd
	// collected_at == window_end: deterministic per-cycle snapshot, same as SLO.
	collectedAt := ts.String()
	rows := buildAPITaxRows(taxResults, windowStartStr, windowEndStr, collectedAt)
	if len(rows) == 0 {
		log.Info("no complete webhook windows, skipping API tax report generation")
		return nil
	}

	taxCSVRows := make(mappedCSVStruct, len(rows))
	for _, r := range rows {
		r := r
		taxCSVRows[r.WebhookName+"|"+r.WindowStart+"|"+r.WindowEnd+"|"+r.Le+"|"+r.Source] = r
	}
	emptyTaxRow := apiTaxRow{}
	taxReport := report{
		file: &file{
			name: rosAPITaxFilePrefix + yearMonth + ".csv",
			path: dirCfg.Reports.Path,
		},
		data: &data{
			queryData: taxCSVRows,
			headers:   emptyTaxRow.csvHeader(),
			prefix:    windowStartStr + "," + windowEndStr,
		},
	}
	log.WithName("writeResults").Info("writing API tax results to file", "filename", taxReport.file.getName())
	if err := taxReport.writeReport(); err != nil {
		return fmt.Errorf("failed to write API tax report: %v", err)
	}
	return nil
}

// collectAPITaxHourly is the GenerateReports hook: ungated hourly snapshot.
// hourStart/hourEnd are captured before the quarterly loop mutates the shared
// TimeSeries. Independent of namespace opt-in and topology; never fails the run.
func collectAPITaxHourly(log gologr.Logger, c *PrometheusCollector, dirCfg *dirconfig.DirectoryConfig, yearMonth string, hourStart, hourEnd time.Time) error {
	return generateAPITaxReport(log, c, dirCfg, yearMonth, hourStart.String(), hourEnd.String(), hourEnd)
}
