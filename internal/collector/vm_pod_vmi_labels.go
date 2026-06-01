//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import (
	"time"

	"github.com/prometheus/common/model"
)

// rosVMPodVMINameQueries maps virt-launcher pods to KubeVirt VMI names via kube_pod_labels.
var rosVMPodVMINameQueries = &querys{
	query{
		Name:        "vm-ros-pod-vmi-name",
		QueryString: QueryMap["ros:vm_pod_vmi_name"],
		MetricKey: staticFields{
			"exported_pod": "pod",
			"namespace":    "namespace",
			"vmi_name":     "label_vm_kubevirt_io_name",
		},
		QueryValue: &saveQueryValue{ValName: "vmi_name"},
		RowKey:     []model.LabelName{"pod", "namespace"},
	},
}

// fetchPodVMINameMap loads pod -> VMI name from kube_pod_labels (vm.kubevirt.io/name).
// Returns nil when the metric is unavailable; merge falls back to pod name parsing.
func fetchPodVMINameMap(c *PrometheusCollector, at time.Time) (map[string]string, error) {
	results := mappedResults{}
	if err := c.getQueryResults(at, rosVMPodVMINameQueries, &results, MaxRetries); err != nil {
		// kube-state-metrics / kube_pod_labels may be absent; caller falls back to pod name parsing.
		return nil, nil
	}
	if len(results) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(results))
	for _, val := range results {
		pod := stringValue(val, "exported_pod")
		ns := stringValue(val, "namespace")
		name := stringValue(val, "vmi_name")
		if pod == "" || ns == "" || name == "" {
			continue
		}
		out[ns+"\x00"+pod] = name
	}
	return out, nil
}
