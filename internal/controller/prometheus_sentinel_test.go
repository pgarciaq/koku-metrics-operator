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
