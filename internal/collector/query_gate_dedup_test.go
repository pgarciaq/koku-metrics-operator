//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import (
	"regexp"
	"testing"
)

// #875: every gated ros: query joins kube_namespace_labels as the opt-in
// gate's right side. If that right side can yield MORE THAN ONE series for
// one namespace (label churn through the staleness window, duplicate KSM
// scrape targets), on(namespace) group_left fails the whole query with
// "many-to-many matching not allowed" — and getQueryResults then empties the
// container CSV for the interval. Every join operand is therefore wrapped in
// max by (namespace) (…): one series per namespace, value 1, gate semantics
// unchanged. These regexes match the UN-wrapped shapes; each must find
// nothing. (The standalone ros:namespace_filter or-of-selectors is not a
// join operand — duplicates there are duplicate rows, not errors.)
var unwrappedGateJoins = []*regexp.Regexp{
	// on(namespace) group_left <bare selector> — the legacy-label arm.
	regexp.MustCompile(`on\(namespace\) group_left kube_namespace_labels`),
	// on(namespace) group_left() <bare selector> — the GPU label_replace chains.
	regexp.MustCompile(`on\(namespace\) group_left\(\) kube_namespace_labels`),
	// on(namespace) group_left() (<selector> or …) — the current+hypershift arm.
	regexp.MustCompile(`on\(namespace\) group_left\(\) \(kube_namespace_labels`),
	// label_replace(<bare selector or or-pair>, …) — the exported_namespace arms.
	regexp.MustCompile(`label_replace\(\(?kube_namespace_labels`),
}

// TestRosQueriesGateJoinsAreDeduplicated pins #875: no gated query may join
// kube_namespace_labels without the max by (namespace) dedup wrap.
// Additions to QueryMap automatically fall under these assertions.
func TestRosQueriesGateJoinsAreDeduplicated(t *testing.T) {
	for name, query := range QueryMap {
		for _, bad := range unwrappedGateJoins {
			if bad.MatchString(query) {
				t.Errorf("query %q joins kube_namespace_labels without the max by (namespace) dedup wrap (#875): matched %q",
					name, bad.String())
			}
		}
	}
}
