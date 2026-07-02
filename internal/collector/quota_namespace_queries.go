//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import (
	"sort"
	"strconv"
	"strings"

	"github.com/prometheus/common/model"
)

func init() {
	initNamespaceQuotaQueryMap()
	initRosNamespaceQuotaQueries()
}

func unifiedNamespaceQuotaQuery(quotaType string) string {
	return `(sum by (namespace, resourcequota, resource) (kube_resourcequota{type='` + quotaType + `'})` +
		` * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'}` +
		` or sum by (namespace, resourcequota, resource) (kube_resourcequota{type='` + quotaType + `'})` +
		` * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})`
}

// rosNamespaceQuotaMetricKeys are QueryMap entries for unified ResourceQuota hard/used metrics.
var rosNamespaceQuotaMetricKeys = []string{
	"ros:namespace_quota_hard",
	"ros:namespace_quota_used",
}

func initNamespaceQuotaQueryMap() {
	QueryMap["ros:namespace_quota_hard"] = unifiedNamespaceQuotaQuery("hard")
	QueryMap["ros:namespace_quota_used"] = unifiedNamespaceQuotaQuery("used")
}

// rosNamespaceQuotaQueries collects kube_resourcequota metrics per (namespace, resourcequota, resource).
var rosNamespaceQuotaQueries *querys

func initRosNamespaceQuotaQueries() {
	rosNamespaceQuotaQueries = &querys{
		query{
			Name:        "namespace-quota-hard",
			QueryString: QueryMap["ros:namespace_quota_hard"],
			MetricKey:   staticFields{"namespace": "namespace", "quota_name": "resourcequota", "resource_name": "resource"},
			QueryValue:  &saveQueryValue{ValName: "hard_value"},
			RowKey:      []model.LabelName{"namespace", "resource", "resourcequota"},
		},
		query{
			Name:        "namespace-quota-used",
			QueryString: QueryMap["ros:namespace_quota_used"],
			MetricKey:   staticFields{"namespace": "namespace", "quota_name": "resourcequota", "resource_name": "resource"},
			QueryValue:  &saveQueryValue{ValName: "used_value"},
			RowKey:      []model.LabelName{"namespace", "resource", "resourcequota"},
		},
	}
}

// namespaceQuotaResourceFieldMap maps Prometheus resource label values to the
// mapstructure field names used by rosNamespaceRow. Index 0 = hard field, index 1 = used field.
var namespaceQuotaResourceFieldMap = map[string][2]string{
	"requests.cpu":     {"cpu-request-namespace-sum", "cpu-request-namespace-used"},
	"limits.cpu":       {"cpu-limit-namespace-sum", "cpu-limit-namespace-used"},
	"requests.memory":  {"memory-request-namespace-sum", "memory-request-namespace-used"},
	"limits.memory":    {"memory-limit-namespace-sum", "memory-limit-namespace-used"},
	"requests.storage": {"storage-request-namespace-hard", "storage-request-namespace-used"},
	"pods":             {"pods-namespace-hard", "pods-namespace-used"},
}

// pivotNamespaceQuotaResults transforms the raw unified query results (keyed by
// namespace,resource,resourcequota with "hard_value"/"used_value" fields) into
// the format expected by mergeRosNamespaceCSVRows (keyed by namespace,resourcequota
// with individually named value fields matching rosNamespaceRow mapstructure tags).
func pivotNamespaceQuotaResults(raw mappedResults) mappedResults {
	out := make(mappedResults)
	// accumulators for count/* resources (summed into object_count)
	objectCountHardAccum := make(map[string]float64)
	objectCountUsedAccum := make(map[string]float64)

	for _, val := range raw {
		ns, _ := val["namespace"].(string)
		quotaName, _ := val["quota_name"].(string)
		resource, _ := val["resource_name"].(string)
		hardStr, _ := val["hard_value"].(string)
		usedStr, _ := val["used_value"].(string)
		if ns == "" || quotaName == "" || resource == "" {
			continue
		}

		outKey := makeQuotaPivotKey(ns, quotaName)
		if out[outKey] == nil {
			out[outKey] = mappedValues{
				"namespace":  ns,
				"quota_name": quotaName,
			}
		}

		if strings.HasPrefix(resource, "count/") {
			if hardStr != "" {
				if v, err := strconv.ParseFloat(hardStr, 64); err == nil {
					objectCountHardAccum[outKey] += v
				}
			}
			if usedStr != "" {
				if v, err := strconv.ParseFloat(usedStr, 64); err == nil {
					objectCountUsedAccum[outKey] += v
				}
			}
			continue
		}

		if fields, ok := namespaceQuotaResourceFieldMap[resource]; ok {
			if hardStr != "" {
				out[outKey][fields[0]] = hardStr
			}
			if usedStr != "" {
				out[outKey][fields[1]] = usedStr
			}
		}
	}

	for key, sum := range objectCountHardAccum {
		if out[key] != nil {
			out[key]["object-count-namespace-hard"] = floatToString(sum)
		}
	}
	for key, sum := range objectCountUsedAccum {
		if out[key] != nil {
			out[key]["object-count-namespace-used"] = floatToString(sum)
		}
	}

	return out
}

// makeQuotaPivotKey produces the same key format that generateKey produces
// for RowKey []model.LabelName{"namespace", "resourcequota"} — sorted values joined by comma.
func makeQuotaPivotKey(namespace, resourcequota string) string {
	parts := []string{namespace, resourcequota}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}
