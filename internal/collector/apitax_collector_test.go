//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import (
	"strings"
	"testing"

	"github.com/prometheus/common/model"
)

var apitaxQueryMapKeys = []string{
	"ros:apitax_webhook_latency",
	"ros:apitax_webhook_total",
	"ros:apitax_webhook_rejected",
}

// TestQueryMap_APITaxQueries pins the thin-W5 contract: per-webhook latency
// buckets plus totals, grouped by webhook identity only — never split by
// user, resource, or verb. No username label anywhere (ADR-0335 NO-GO).
func TestQueryMap_APITaxQueries(t *testing.T) {
	t.Parallel()

	latency, ok := QueryMap["ros:apitax_webhook_latency"]
	if !ok {
		t.Fatal("QueryMap missing ros:apitax_webhook_latency")
	}
	if !strings.Contains(latency, "apiserver_admission_webhook_admission_duration_seconds_bucket") {
		t.Errorf("latency query must read admission_duration_seconds_bucket, got: %s", latency)
	}
	if !strings.Contains(latency, "sum by (name, le, job)") {
		t.Errorf("latency query must group by (name, le, job), got: %s", latency)
	}
	for _, key := range apitaxQueryMapKeys {
		q, ok := QueryMap[key]
		if !ok {
			t.Fatalf("QueryMap missing %q", key)
		}
		for _, banned := range []string{"username", "user=", "resource"} {
			if strings.Contains(q, banned) {
				t.Errorf("QueryMap[%q] must not select %q (cardinality/privacy), got: %s", key, banned, q)
			}
		}
	}
	total := QueryMap["ros:apitax_webhook_total"]
	if !strings.Contains(total, "apiserver_admission_webhook_request_total") {
		t.Errorf("total query must read admission_webhook_request_total, got: %s", total)
	}
	rejected := QueryMap["ros:apitax_webhook_rejected"]
	if !strings.Contains(rejected, `rejected="true"`) {
		t.Errorf("rejected query must filter rejected=true, got: %s", rejected)
	}
}

func TestAPITaxRow_CSVHeader(t *testing.T) {
	t.Parallel()

	want := []string{"webhook_name", "window_start", "window_end", "le", "bucket_count", "total_count", "rejected_count", "collected_at", "source"}
	got := apiTaxRow{}.csvHeader()
	if len(got) != len(want) {
		t.Fatalf("header has %d columns, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("header[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestBuildAPITaxRows_MergesAndJoins proves the pivot: buckets merge across
// float-equal le labels, totals/rejected join by webhook+source onto every
// bucket row, and bucket-less carriers never emit.
func TestBuildAPITaxRows_MergesAndJoins(t *testing.T) {
	t.Parallel()

	results := mappedResults{
		"a": mappedValues{"webhook": "pod.network-node-identity.openshift.io", "source": "kubernetes", "le": "0.4", "apitax_latency_count": "100.000000"},
		"b": mappedValues{"webhook": "pod.network-node-identity.openshift.io", "source": "kubernetes", "le": "0.40", "apitax_latency_count": "23.000000"},
		"c": mappedValues{"webhook": "pod.network-node-identity.openshift.io", "source": "kubernetes", "le": "+Inf", "apitax_latency_count": "500.000000"},
		"d": mappedValues{"webhook": "pod.network-node-identity.openshift.io", "source": "kubernetes", "apitax_total_count": "500.000000", "apitax_rejected_count": "5.000000"},
		"e": mappedValues{"webhook": "lonely.example.io", "source": "kubernetes", "apitax_total_count": "42.000000"},
	}
	rows := buildAPITaxRows(results, "ws", "we", "ca")
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2 (0.4+0.40 merged, +Inf; lonely totals carrier omitted)", len(rows))
	}
	byLe := map[string]apiTaxRow{}
	for _, r := range rows {
		byLe[r.Le] = r
	}
	merged, ok := byLe["0.4"]
	if !ok {
		t.Fatalf("merged 0.4 row missing: %+v", rows)
	}
	if merged.BucketCount != "123" {
		t.Errorf("merged bucket = %q, want 123 (100+23 float-equal merge)", merged.BucketCount)
	}
	if merged.TotalCount != "500" || merged.RejectedCount != "5" {
		t.Errorf("totals not joined: total=%q rejected=%q", merged.TotalCount, merged.RejectedCount)
	}
	if merged.Source != "kubernetes" || merged.CollectedAt != "ca" {
		t.Errorf("source/collector not carried: %+v", merged)
	}
	inf := byLe["+Inf"]
	if inf.BucketCount != "500" || inf.TotalCount != "500" {
		t.Errorf("+Inf row wrong: %+v", inf)
	}
}

func apitaxSample(job, le, webhook string, v float64, ts model.Time) *model.Sample {
	metric := model.Metric{"job": model.LabelValue(job)}
	if le != "" {
		metric["le"] = model.LabelValue(le)
	}
	if webhook != "" {
		metric["name"] = model.LabelValue(webhook)
	}
	return &model.Sample{Metric: metric, Value: model.SampleValue(v), Timestamp: ts}
}

func addAPITaxMockResults(mapResults mappedMockPromResult, withData bool) {
	for _, q := range *apitaxQueries {
		vec := model.Vector{}
		if withData {
			ts := model.Time(0)
			switch q.Name {
			case "apitax-webhook-latency":
				vec = model.Vector{
					apitaxSample("kubernetes", "0.5", "pod.network-node-identity.openshift.io", 87, ts),
					apitaxSample("kubernetes", "+Inf", "pod.network-node-identity.openshift.io", 102, ts),
				}
			case "apitax-webhook-total":
				vec = model.Vector{apitaxSample("kubernetes", "", "pod.network-node-identity.openshift.io", 102, ts)}
			case "apitax-webhook-rejected":
				vec = model.Vector{apitaxSample("kubernetes", "", "pod.network-node-identity.openshift.io", 2, ts)}
			}
		}
		mapResults[q.QueryString] = &mockPromResult{value: vec}
	}
}
