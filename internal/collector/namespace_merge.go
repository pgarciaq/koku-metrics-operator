//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import (
	"github.com/mitchellh/mapstructure"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
)

// mergeRosNamespaceCSVRows combines namespace usage metrics (keyed by namespace) with
// per-ResourceQuota hard/used metrics (keyed by namespace,resourcequota).
//
// Row identity comes from each quota row's own namespace/quota_name values, never
// from the map key: the keys are opaque identities, and splitting them would tie
// this function to the encoder's component order. Composite and raw namespace keys
// share the output map; they cannot collide because a namespace name is a DNS-1123
// label and never contains ':'.
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

	for _, val := range quotaResults {
		ns := stringValue(val, "namespace")
		quotaName := stringValue(val, "quota_name")
		if ns == "" || quotaName == "" {
			continue
		}
		namespacesWithQuota[ns] = struct{}{}

		row := newROSNamespaceRow(ts)
		if base, ok := usageRows[ns]; ok {
			// getStruct stores rows through a pointer, so the assertion must be
			// *rosNamespaceRow; asserting the value type silently discarded the
			// namespace usage fields of every quota row.
			if baseRow, ok := base.(*rosNamespaceRow); ok {
				row = *baseRow
			}
		}
		row.Namespace = ns
		row.QuotaName = quotaName
		if err := mapstructure.Decode(val, &row); err != nil {
			continue
		}
		out[keyFromValues(ns, quotaName)] = &row
	}

	for ns, row := range usageRows {
		if _, hasQuota := namespacesWithQuota[ns]; hasQuota {
			continue
		}
		out[ns] = row
	}
	return out
}
