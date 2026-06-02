//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import "github.com/prometheus/common/model"

func init() {
	initNamespaceQuotaQueryMap()
	initRosNamespaceQuotaQueries()
}

func namespaceResourceQuotaQuery(resource, quotaType string) string {
	hardUsed := quotaType
	return `(sum by (namespace, resourcequota) (kube_resourcequota{resource='` + resource + `', type='` + hardUsed + `'})` +
		` * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'}` +
		` or sum by (namespace, resourcequota) (kube_resourcequota{resource='` + resource + `', type='` + hardUsed + `'})` +
		` * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})`
}

// rosNamespaceQuotaMetricKeys are QueryMap entries for per-ResourceQuota hard/used metrics.
var rosNamespaceQuotaMetricKeys = []string{
	"ros:cpu_request_namespace_sum",
	"ros:cpu_request_namespace_used",
	"ros:cpu_limit_namespace_sum",
	"ros:cpu_limit_namespace_used",
	"ros:memory_request_namespace_sum",
	"ros:memory_request_namespace_used",
	"ros:memory_limit_namespace_sum",
	"ros:memory_limit_namespace_used",
	"ros:storage_request_namespace_hard",
	"ros:storage_request_namespace_used",
	"ros:pods_namespace_hard",
	"ros:pods_namespace_used",
	"ros:object_count_namespace_hard",
	"ros:object_count_namespace_used",
}

func initNamespaceQuotaQueryMap() {
	QueryMap["ros:cpu_request_namespace_sum"] = namespaceResourceQuotaQuery("requests.cpu", "hard")
	QueryMap["ros:cpu_request_namespace_used"] = namespaceResourceQuotaQuery("requests.cpu", "used")
	QueryMap["ros:cpu_limit_namespace_sum"] = namespaceResourceQuotaQuery("limits.cpu", "hard")
	QueryMap["ros:cpu_limit_namespace_used"] = namespaceResourceQuotaQuery("limits.cpu", "used")
	QueryMap["ros:memory_request_namespace_sum"] = namespaceResourceQuotaQuery("requests.memory", "hard")
	QueryMap["ros:memory_request_namespace_used"] = namespaceResourceQuotaQuery("requests.memory", "used")
	QueryMap["ros:memory_limit_namespace_sum"] = namespaceResourceQuotaQuery("limits.memory", "hard")
	QueryMap["ros:memory_limit_namespace_used"] = namespaceResourceQuotaQuery("limits.memory", "used")
	QueryMap["ros:storage_request_namespace_hard"] = namespaceResourceQuotaQuery("requests.storage", "hard")
	QueryMap["ros:storage_request_namespace_used"] = namespaceResourceQuotaQuery("requests.storage", "used")
	QueryMap["ros:pods_namespace_hard"] = namespaceResourceQuotaQuery("pods", "hard")
	QueryMap["ros:pods_namespace_used"] = namespaceResourceQuotaQuery("pods", "used")
	QueryMap["ros:object_count_namespace_hard"] = namespaceResourceQuotaCountQuery("hard")
	QueryMap["ros:object_count_namespace_used"] = namespaceResourceQuotaCountQuery("used")
}

func namespaceResourceQuotaCountQuery(quotaType string) string {
	return `(sum by (namespace, resourcequota) (kube_resourcequota{resource=~'count/.+', type='` + quotaType + `'})` +
		` * on(namespace) group_left kube_namespace_labels{label_insights_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'}` +
		` or sum by (namespace, resourcequota) (kube_resourcequota{resource=~'count/.+', type='` + quotaType + `'})` +
		` * on(namespace) group_left kube_namespace_labels{label_cost_management_optimizations='true', namespace!~'kube-.*|openshift|openshift-.*'})`
}

// rosNamespaceQuotaQueries collects kube_resourcequota metrics per (namespace, resourcequota).
var rosNamespaceQuotaQueries *querys

func initRosNamespaceQuotaQueries() {
	rosNamespaceQuotaQueries = &querys{
	query{
		Name:        "cpu-request-namespace-sum",
		QueryString: QueryMap["ros:cpu_request_namespace_sum"],
		MetricKey:   staticFields{"namespace": "namespace", "quota_name": "resourcequota"},
		QueryValue:  &saveQueryValue{ValName: "cpu-request-namespace-sum"},
		RowKey:      []model.LabelName{"namespace", "resourcequota"},
	},
	query{
		Name:        "cpu-request-namespace-used",
		QueryString: QueryMap["ros:cpu_request_namespace_used"],
		MetricKey:   staticFields{"namespace": "namespace", "quota_name": "resourcequota"},
		QueryValue:  &saveQueryValue{ValName: "cpu-request-namespace-used"},
		RowKey:      []model.LabelName{"namespace", "resourcequota"},
	},
	query{
		Name:        "cpu-limit-namespace-sum",
		QueryString: QueryMap["ros:cpu_limit_namespace_sum"],
		MetricKey:   staticFields{"namespace": "namespace", "quota_name": "resourcequota"},
		QueryValue:  &saveQueryValue{ValName: "cpu-limit-namespace-sum"},
		RowKey:      []model.LabelName{"namespace", "resourcequota"},
	},
	query{
		Name:        "cpu-limit-namespace-used",
		QueryString: QueryMap["ros:cpu_limit_namespace_used"],
		MetricKey:   staticFields{"namespace": "namespace", "quota_name": "resourcequota"},
		QueryValue:  &saveQueryValue{ValName: "cpu-limit-namespace-used"},
		RowKey:      []model.LabelName{"namespace", "resourcequota"},
	},
	query{
		Name:        "memory-request-namespace-sum",
		QueryString: QueryMap["ros:memory_request_namespace_sum"],
		MetricKey:   staticFields{"namespace": "namespace", "quota_name": "resourcequota"},
		QueryValue:  &saveQueryValue{ValName: "memory-request-namespace-sum"},
		RowKey:      []model.LabelName{"namespace", "resourcequota"},
	},
	query{
		Name:        "memory-request-namespace-used",
		QueryString: QueryMap["ros:memory_request_namespace_used"],
		MetricKey:   staticFields{"namespace": "namespace", "quota_name": "resourcequota"},
		QueryValue:  &saveQueryValue{ValName: "memory-request-namespace-used"},
		RowKey:      []model.LabelName{"namespace", "resourcequota"},
	},
	query{
		Name:        "memory-limit-namespace-sum",
		QueryString: QueryMap["ros:memory_limit_namespace_sum"],
		MetricKey:   staticFields{"namespace": "namespace", "quota_name": "resourcequota"},
		QueryValue:  &saveQueryValue{ValName: "memory-limit-namespace-sum"},
		RowKey:      []model.LabelName{"namespace", "resourcequota"},
	},
	query{
		Name:        "memory-limit-namespace-used",
		QueryString: QueryMap["ros:memory_limit_namespace_used"],
		MetricKey:   staticFields{"namespace": "namespace", "quota_name": "resourcequota"},
		QueryValue:  &saveQueryValue{ValName: "memory-limit-namespace-used"},
		RowKey:      []model.LabelName{"namespace", "resourcequota"},
	},
	query{
		Name:        "storage-request-namespace-hard",
		QueryString: QueryMap["ros:storage_request_namespace_hard"],
		MetricKey:   staticFields{"namespace": "namespace", "quota_name": "resourcequota"},
		QueryValue:  &saveQueryValue{ValName: "storage-request-namespace-hard"},
		RowKey:      []model.LabelName{"namespace", "resourcequota"},
	},
	query{
		Name:        "storage-request-namespace-used",
		QueryString: QueryMap["ros:storage_request_namespace_used"],
		MetricKey:   staticFields{"namespace": "namespace", "quota_name": "resourcequota"},
		QueryValue:  &saveQueryValue{ValName: "storage-request-namespace-used"},
		RowKey:      []model.LabelName{"namespace", "resourcequota"},
	},
	query{
		Name:        "pods-namespace-hard",
		QueryString: QueryMap["ros:pods_namespace_hard"],
		MetricKey:   staticFields{"namespace": "namespace", "quota_name": "resourcequota"},
		QueryValue:  &saveQueryValue{ValName: "pods-namespace-hard"},
		RowKey:      []model.LabelName{"namespace", "resourcequota"},
	},
	query{
		Name:        "pods-namespace-used",
		QueryString: QueryMap["ros:pods_namespace_used"],
		MetricKey:   staticFields{"namespace": "namespace", "quota_name": "resourcequota"},
		QueryValue:  &saveQueryValue{ValName: "pods-namespace-used"},
		RowKey:      []model.LabelName{"namespace", "resourcequota"},
	},
	query{
		Name:        "object-count-namespace-hard",
		QueryString: QueryMap["ros:object_count_namespace_hard"],
		MetricKey:   staticFields{"namespace": "namespace", "quota_name": "resourcequota"},
		QueryValue:  &saveQueryValue{ValName: "object-count-namespace-hard"},
		RowKey:      []model.LabelName{"namespace", "resourcequota"},
	},
	query{
		Name:        "object-count-namespace-used",
		QueryString: QueryMap["ros:object_count_namespace_used"],
		MetricKey:   staticFields{"namespace": "namespace", "quota_name": "resourcequota"},
		QueryValue:  &saveQueryValue{ValName: "object-count-namespace-used"},
		RowKey:      []model.LabelName{"namespace", "resourcequota"},
	},
	}
}
