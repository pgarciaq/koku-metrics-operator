//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import (
	"strings"

	"github.com/mitchellh/mapstructure"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
)

// mergeRosNamespaceCSVRows combines namespace usage metrics (keyed by namespace) with
// per-ResourceQuota hard/used metrics (keyed by namespace,resourcequota).
func mergeRosNamespaceCSVRows(usageResults, quotaResults mappedResults, ts *promv1.Range) mappedCSVStruct {
	usageRows := make(mappedCSVStruct)
	for ns, val := range usageResults {
		usage := newROSNamespaceRow(ts)
		if err := getStruct(val, &usage, usageRows, ns); err != nil {
			continue
		}
	}

	if len(quotaResults) == 0 {
		return usageRows
	}

	out := make(mappedCSVStruct)
	namespacesWithQuota := make(map[string]struct{})

	for quotaKey, val := range quotaResults {
		parts := strings.SplitN(quotaKey, ",", 2)
		if len(parts) != 2 {
			continue
		}
		ns, quotaName := parts[0], parts[1]
		namespacesWithQuota[ns] = struct{}{}

		row := newROSNamespaceRow(ts)
		if base, ok := usageRows[ns]; ok {
			if baseRow, ok := base.(rosNamespaceRow); ok {
				row = baseRow
			}
		}
		row.Namespace = ns
		row.QuotaName = quotaName
		if err := mapstructure.Decode(val, &row); err != nil {
			continue
		}
		out[quotaKey] = row
	}

	for ns, row := range usageRows {
		if _, hasQuota := namespacesWithQuota[ns]; hasQuota {
			continue
		}
		out[ns] = row
	}
	return out
}
