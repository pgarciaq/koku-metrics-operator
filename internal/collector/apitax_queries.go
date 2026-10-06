//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import (
	"github.com/prometheus/common/model"
)

// API tax webhook queries (thin W5, #393).
//
// Three instant queries evaluated at window end (cumulative snapshots, SLO
// pattern): per-webhook latency buckets plus totals and rejected totals.
// Label names verified live against SNO thanos (2026-10-05): buckets carry
// name/le (+type/operation, deliberately ungrouped to stay thin); totals
// carry name/code/rejected. SNO exposes 3 webhooks — bounded by webhook
// registration count, never by user/request cardinality (ADR-0335 NO-GOs:
// no per-user Prom export, no audit shipping).
//
// Grouping includes job (recorded as source): the SLO source-split amendment
// taught that histogram schemas can differ per job, and per-job row sets
// keep each progression coherent. The backend buckets hourly per
// (webhook, source) and derives p99 at read.
func init() {
	QueryMap["ros:apitax_webhook_latency"] = `sum by (name, le, job) (apiserver_admission_webhook_admission_duration_seconds_bucket)`
	QueryMap["ros:apitax_webhook_total"] = `sum by (name, job) (apiserver_admission_webhook_request_total)`
	QueryMap["ros:apitax_webhook_rejected"] = `sum by (name, job) (apiserver_admission_webhook_request_total{rejected="true"})`

	apitaxQueries = &querys{
		query{
			Name:        "apitax-webhook-latency",
			QueryString: QueryMap["ros:apitax_webhook_latency"],
			MetricKey:   staticFields{"le": "le", "webhook": "name", "source": "job"},
			QueryValue:  &saveQueryValue{ValName: "apitax_latency_count"},
			RowKey:      []model.LabelName{"le", "name", "job"},
		},
		query{
			Name:        "apitax-webhook-total",
			QueryString: QueryMap["ros:apitax_webhook_total"],
			MetricKey:   staticFields{"webhook": "name", "source": "job"},
			QueryValue:  &saveQueryValue{ValName: "apitax_total_count"},
			RowKey:      []model.LabelName{"name", "job"},
		},
		query{
			Name:        "apitax-webhook-rejected",
			QueryString: QueryMap["ros:apitax_webhook_rejected"],
			MetricKey:   staticFields{"webhook": "name", "source": "job"},
			QueryValue:  &saveQueryValue{ValName: "apitax_rejected_count"},
			RowKey:      []model.LabelName{"name", "job"},
		},
	}
}
