//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import (
	"strconv"
	"strings"

	"github.com/prometheus/common/model"
)

func init() {
	initClusterQuotaQueryMap()
	initRosClusterQuotaQueries()
}

// rosClusterQuotaMetricKeys are QueryMap entries for unified ClusterResourceQuota hard/used metrics.
var rosClusterQuotaMetricKeys = []string{
	"ros:cluster_quota_hard",
	"ros:cluster_quota_used",
}

func initClusterQuotaQueryMap() {
	QueryMap["ros:cluster_quota_hard"] = "sum by (name, resource) (openshift_clusterresourcequota_usage{type='hard'})"
	QueryMap["ros:cluster_quota_used"] = "sum by (name, resource) (openshift_clusterresourcequota_usage{type='used'})"
}

func initRosClusterQuotaQueries() {
	rosClusterQuotaQueries = &querys{
		query{
			Name:        "cluster-quota-hard",
			QueryString: QueryMap["ros:cluster_quota_hard"],
			MetricKey:   staticFields{"cluster_quota_name": "name", "resource_name": "resource"},
			QueryValue:  &saveQueryValue{ValName: "hard_value"},
			RowKey:      []model.LabelName{"name", "resource"},
		},
		query{
			Name:        "cluster-quota-used",
			QueryString: QueryMap["ros:cluster_quota_used"],
			MetricKey:   staticFields{"cluster_quota_name": "name", "resource_name": "resource"},
			QueryValue:  &saveQueryValue{ValName: "used_value"},
			RowKey:      []model.LabelName{"name", "resource"},
		},
	}
}

// clusterQuotaResourceFieldMap maps Prometheus resource label values to the
// mapstructure field names used by rosClusterQuotaRow. Index 0 = hard field, index 1 = used field.
var clusterQuotaResourceFieldMap = map[string][2]string{
	"requests.cpu":     {"cpu-request-hard", "cpu-request-used"},
	"limits.cpu":       {"cpu-limit-hard", "cpu-limit-used"},
	"requests.memory":  {"memory-request-hard", "memory-request-used"},
	"limits.memory":    {"memory-limit-hard", "memory-limit-used"},
	"requests.storage": {"storage-request-hard", "storage-request-used"},
	"pods":             {"pods-hard", "pods-used"},
}

// pivotClusterQuotaResults transforms the raw unified query results (keyed by
// name,resource with "hard_value"/"used_value" fields) into the format expected
// by generateROSClusterQuotaReport (keyed by CRQ name with individually named
// value fields matching rosClusterQuotaRow mapstructure tags).
func pivotClusterQuotaResults(raw mappedResults) mappedResults {
	out := make(mappedResults)
	objectCountHardAccum := make(map[string]float64)
	objectCountUsedAccum := make(map[string]float64)

	for _, val := range raw {
		crqName, _ := val["cluster_quota_name"].(string)
		resource, _ := val["resource_name"].(string)
		hardStr, _ := val["hard_value"].(string)
		usedStr, _ := val["used_value"].(string)
		if crqName == "" || resource == "" {
			continue
		}

		if out[crqName] == nil {
			out[crqName] = mappedValues{
				"cluster_quota_name": crqName,
			}
		}

		if strings.HasPrefix(resource, "count/") {
			if hardStr != "" {
				if v, err := strconv.ParseFloat(hardStr, 64); err == nil {
					objectCountHardAccum[crqName] += v
				}
			}
			if usedStr != "" {
				if v, err := strconv.ParseFloat(usedStr, 64); err == nil {
					objectCountUsedAccum[crqName] += v
				}
			}
			continue
		}

		if fields, ok := clusterQuotaResourceFieldMap[resource]; ok {
			if hardStr != "" {
				out[crqName][fields[0]] = hardStr
			}
			if usedStr != "" {
				out[crqName][fields[1]] = usedStr
			}
		}
	}

	for key, sum := range objectCountHardAccum {
		if out[key] != nil {
			out[key]["object-count-hard"] = floatToString(sum)
		}
	}
	for key, sum := range objectCountUsedAccum {
		if out[key] != nil {
			out[key]["object-count-used"] = floatToString(sum)
		}
	}

	return out
}
