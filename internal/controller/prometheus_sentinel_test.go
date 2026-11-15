//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package controller

import (
	"errors"
	"strings"
	"testing"
	"time"

	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"

	metricscfgv1beta1 "github.com/project-koku/koku-metrics-operator/api/v1beta1"
	"github.com/project-koku/koku-metrics-operator/internal/collector"
	"github.com/project-koku/koku-metrics-operator/internal/dirconfig"
)

// #768: the no-opted-in-namespace sentinel is a successful hour, not a
// failure to retry, and its message must not claim nothing was collected —
// cluster-level deliverables did run.
func TestCollectPromStatsROSNoEnabledNamespaces(t *testing.T) {
	origGenerate := generateReports
	origTracker := retryTracker
	t.Cleanup(func() {
		generateReports = origGenerate
		retryTracker = origTracker
	})
	retryTracker = make(map[time.Time]int)

	generateReports = func(*metricscfgv1beta1.MetricsConfig, *dirconfig.DirectoryConfig, *collector.PrometheusCollector) error {
		return collector.ErrROSNoEnabledNamespaces
	}

	r := &MetricsConfigReconciler{promCollector: &collector.PrometheusCollector{}}
	cr := &metricscfgv1beta1.MetricsConfig{}
	start := time.Date(2026, 11, 5, 20, 0, 0, 0, time.UTC)
	timeRange := promv1.Range{Start: start, End: start.Add(time.Hour - time.Second), Step: time.Minute}

	err := collectPromStats(r, cr, &dirconfig.DirectoryConfig{}, timeRange)
	if !errors.Is(err, collector.ErrROSNoEnabledNamespaces) {
		t.Fatalf("collectPromStats error = %v, want ErrROSNoEnabledNamespaces", err)
	}
	if !cr.Status.Prometheus.LastQuerySuccessTime.Time.Equal(start) {
		t.Errorf("LastQuerySuccessTime = %v, want %v", cr.Status.Prometheus.LastQuerySuccessTime.Time, start)
	}
	if !cr.Status.Reports.DataCollected {
		t.Error("DataCollected = false, want true: cluster-level data was collected without container/namespace metrics (#768)")
	}

	msg := cr.Status.Reports.DataCollectionMessage
	if !strings.Contains(msg, "Cluster-level metrics were collected") {
		t.Errorf("DataCollectionMessage = %q, want it to say cluster-level metrics were collected", msg)
	}
	if !strings.Contains(msg, "no container or namespace metrics were collected") {
		t.Errorf("DataCollectionMessage = %q, want it to name the skipped container/namespace metrics", msg)
	}

	if got := retryTracker[start]; got != 0 {
		t.Errorf("retryTracker[start] = %d, want 0: the hour is a success, not a retry", got)
	}
}

// #811: an hour with no node metrics but with node-independent deliverables
// collected is still a successful hour — no retry, the hour advances — and,
// unlike a plain no-data hour, data was collected.
func TestCollectPromStatsNoDataPartial(t *testing.T) {
	origGenerate := generateReports
	origTracker := retryTracker
	t.Cleanup(func() {
		generateReports = origGenerate
		retryTracker = origTracker
	})
	retryTracker = make(map[time.Time]int)

	generateReports = func(*metricscfgv1beta1.MetricsConfig, *dirconfig.DirectoryConfig, *collector.PrometheusCollector) error {
		return collector.ErrNoDataPartial
	}

	r := &MetricsConfigReconciler{promCollector: &collector.PrometheusCollector{}}
	cr := &metricscfgv1beta1.MetricsConfig{}
	start := time.Date(2026, 11, 5, 20, 0, 0, 0, time.UTC)
	timeRange := promv1.Range{Start: start, End: start.Add(time.Hour - time.Second), Step: time.Minute}

	err := collectPromStats(r, cr, &dirconfig.DirectoryConfig{}, timeRange)
	if !errors.Is(err, collector.ErrNoDataPartial) {
		t.Fatalf("collectPromStats error = %v, want ErrNoDataPartial", err)
	}
	if !cr.Status.Prometheus.LastQuerySuccessTime.Time.Equal(start) {
		t.Errorf("LastQuerySuccessTime = %v, want %v: the hour must not be retried (#811)",
			cr.Status.Prometheus.LastQuerySuccessTime.Time, start)
	}
	if !cr.Status.Reports.DataCollected {
		t.Error("DataCollected = false, want true: node-independent deliverables were collected (#811)")
	}

	msg := cr.Status.Reports.DataCollectionMessage
	if !strings.Contains(msg, "No node metrics for the hour queried") {
		t.Errorf("DataCollectionMessage = %q, want it to name the missing node metrics", msg)
	}
	if !strings.Contains(msg, "cluster-level and VM deliverables were collected") {
		t.Errorf("DataCollectionMessage = %q, want it to name what was collected", msg)
	}
	if strings.Contains(msg, "No data to report") {
		t.Errorf("DataCollectionMessage = %q, must not reuse the no-data wording: something was collected", msg)
	}

	if got := retryTracker[start]; got != 0 {
		t.Errorf("retryTracker[start] = %d, want 0: the hour is a success, not a retry", got)
	}
}

// #811 control: a plain ErrNoData hour still reports that nothing was collected.
// Without this, "partial" could quietly become the answer for every empty hour.
func TestCollectPromStatsNoDataKeepsDataCollectedFalse(t *testing.T) {
	origGenerate := generateReports
	origTracker := retryTracker
	t.Cleanup(func() {
		generateReports = origGenerate
		retryTracker = origTracker
	})
	retryTracker = make(map[time.Time]int)

	generateReports = func(*metricscfgv1beta1.MetricsConfig, *dirconfig.DirectoryConfig, *collector.PrometheusCollector) error {
		return collector.ErrNoData
	}

	r := &MetricsConfigReconciler{promCollector: &collector.PrometheusCollector{}}
	cr := &metricscfgv1beta1.MetricsConfig{}
	start := time.Date(2026, 11, 5, 20, 0, 0, 0, time.UTC)
	timeRange := promv1.Range{Start: start, End: start.Add(time.Hour - time.Second), Step: time.Minute}

	err := collectPromStats(r, cr, &dirconfig.DirectoryConfig{}, timeRange)
	if !errors.Is(err, collector.ErrNoData) {
		t.Fatalf("collectPromStats error = %v, want ErrNoData", err)
	}
	if cr.Status.Reports.DataCollected {
		t.Error("DataCollected = true, want false: nothing was collected")
	}
	if msg := cr.Status.Reports.DataCollectionMessage; !strings.Contains(msg, "No data to report") {
		t.Errorf("DataCollectionMessage = %q, want the existing no-data wording", msg)
	}
	if got := retryTracker[start]; got != 0 {
		t.Errorf("retryTracker[start] = %d, want 0: no-data hours are not retried", got)
	}
}

// #811: a partial hour must NOT abandon the rest of the day during initial
// ingest. It already carries real rows (VM family, SLO, API tax, Kubernetes
// inventory) and a partial day beats no day at all — the owner decision of
// 2026-11-15. Only an hour that collected nothing still skips the day.
func TestShouldSkipDay(t *testing.T) {
	endTime := time.Date(2026, 11, 5, 23, 0, 0, 0, time.UTC)
	midnight := time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)
	oneAM := midnight.Add(time.Hour)
	sameDayMidnight := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		err       error
		hourStart time.Time
		end       time.Time
		initial   bool
		want      bool
	}{
		{"no data at midnight, initial ingest", collector.ErrNoData, midnight, endTime, true, true},
		{"partial at midnight, initial ingest", collector.ErrNoDataPartial, midnight, endTime, true, false},
		{"partial at 1am", collector.ErrNoDataPartial, oneAM, endTime, true, false},
		{"partial at midnight, not initial ingest", collector.ErrNoDataPartial, midnight, endTime, false, false},
		{"partial at midnight of the last day", collector.ErrNoDataPartial, sameDayMidnight, endTime, true, false},
		{"no error", nil, midnight, endTime, true, false},
		{"other error", errors.New("boom"), midnight, endTime, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldSkipDay(tt.err, tt.hourStart, tt.end, tt.initial); got != tt.want {
				t.Errorf("shouldSkipDay(%v, %v) = %v, want %v", tt.err, tt.hourStart, got, tt.want)
			}
		})
	}
}
