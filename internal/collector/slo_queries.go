//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import (
	"github.com/prometheus/common/model"
)

// SLO rollup queries (#644 canonical contract, source-split amendment).
//
// Three instant queries, one per verb group, with verb regexes baked in.
// WATCH/CONNECT/PROXY are excluded at source in every query (WATCH-exclusion
// halves volume and is load-bearing). Results are summed over resources —
// never split by resource/route — grouping by (le, job): the metric name is
// exposed by several apiserver jobs (kubernetes, metrics-server, metrics)
// with different compiled bucket schemas, and per-job row sets keep each
// progression a coherent cumulative histogram. The backend buckets hourly and
// computes reset-aware deltas at read; the operator emits per-cycle
// cumulative snapshots verbatim.
//
// API verbs (not HTTP methods): apiserver_request_duration_seconds carries
// CREATE/UPDATE/PATCH/DELETE/DELETECOLLECTION (mutating), GET/LIST (read),
// plus WATCH/CONNECT/PROXY (excluded) and any residual verbs (other).
func init() {
	QueryMap["ros:slo_api_buckets_mutating"] = `sum by (le, job) (apiserver_request_duration_seconds_bucket{verb=~"CREATE|UPDATE|PATCH|DELETE|DELETECOLLECTION"})`
	QueryMap["ros:slo_api_buckets_read"] = `sum by (le, job) (apiserver_request_duration_seconds_bucket{verb=~"GET|LIST"})`
	QueryMap["ros:slo_api_buckets_other"] = `sum by (le, job) (apiserver_request_duration_seconds_bucket{verb!~"CREATE|UPDATE|PATCH|DELETE|DELETECOLLECTION|GET|LIST|WATCH|CONNECT|PROXY"})`

	sloQueries = &querys{
		query{
			Name:        "slo-api-buckets-mutating",
			QueryString: QueryMap["ros:slo_api_buckets_mutating"],
			MetricKey:   staticFields{"le": "le", "source": "job"},
			QueryValue:  &saveQueryValue{ValName: "slo_mutating_count"},
			RowKey:      []model.LabelName{"le", "job"},
		},
		query{
			Name:        "slo-api-buckets-read",
			QueryString: QueryMap["ros:slo_api_buckets_read"],
			MetricKey:   staticFields{"le": "le", "source": "job"},
			QueryValue:  &saveQueryValue{ValName: "slo_read_count"},
			RowKey:      []model.LabelName{"le", "job"},
		},
		query{
			Name:        "slo-api-buckets-other",
			QueryString: QueryMap["ros:slo_api_buckets_other"],
			MetricKey:   staticFields{"le": "le", "source": "job"},
			QueryValue:  &saveQueryValue{ValName: "slo_other_count"},
			RowKey:      []model.LabelName{"le", "job"},
		},
	}
}
