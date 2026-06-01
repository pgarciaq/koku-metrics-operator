//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

var rosClusterQuotaNamespaceMembershipQuery = query{
	Name:        "cluster-quota-namespace-members",
	QueryString: QueryMap["ros:cluster_quota_namespace_members"],
}

// clusterQuotaNamespaceMembershipByCRQ returns sorted comma-separated namespace lists per CRQ name.
func (c *PrometheusCollector) clusterQuotaNamespaceMembershipByCRQ(ts time.Time) (map[string]string, error) {
	vector, err := c.getVectorQuerySimple(rosClusterQuotaNamespaceMembershipQuery, ts)
	if err != nil {
		return nil, fmt.Errorf("cluster quota namespace membership: %w", err)
	}

	byName := make(map[string]map[string]struct{})
	for _, sample := range vector {
		name := string(sample.Metric["name"])
		ns := string(sample.Metric["namespace"])
		if name == "" || ns == "" {
			continue
		}
		if byName[name] == nil {
			byName[name] = make(map[string]struct{})
		}
		byName[name][ns] = struct{}{}
	}

	out := make(map[string]string, len(byName))
	for name, nss := range byName {
		list := make([]string, 0, len(nss))
		for ns := range nss {
			list = append(list, ns)
		}
		sort.Strings(list)
		out[name] = strings.Join(list, ",")
	}
	return out, nil
}

func applyClusterQuotaNamespaces(rows mappedCSVStruct, membership map[string]string) {
	for key, row := range rows {
		crqRow, ok := row.(*rosClusterQuotaRow)
		if !ok {
			continue
		}
		crqRow.Namespaces = membership[key]
	}
}
