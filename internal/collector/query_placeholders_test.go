//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import (
	"regexp"
	"testing"
)

// unresolvedPlaceholder matches bracketed all-caps literals like [STEP]:
// range-vector durations ([15m], [1h]) start with digits or lowercase and
// never match. No PromQL parser dependency is needed — an unsubstituted
// placeholder is a parse error on every Prometheus, so its absence is the
// property under test.
var unresolvedPlaceholder = regexp.MustCompile(`\[[A-Z][A-Z_]*\]`)

// TestRosQueriesHaveNoUnresolvedPlaceholders pins #582: every query string
// the collector can execute must contain no unsubstituted placeholders.
// Additions to QueryMap automatically fall under this assertion.
func TestRosQueriesHaveNoUnresolvedPlaceholders(t *testing.T) {
	for name, query := range QueryMap {
		for _, hit := range unresolvedPlaceholder.FindAllString(query, -1) {
			t.Errorf("query %q contains unresolved placeholder %s", name, hit)
		}
	}
}
