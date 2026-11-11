//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import (
	"testing"

	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

func TestGenerateKeyRowOrderIsIdentity(t *testing.T) {
	t.Parallel()

	metric := model.Metric{"name": "alpha", "namespace": "beta"}

	nameFirst := generateKey(metric, []model.LabelName{"name", "namespace"})
	namespaceFirst := generateKey(metric, []model.LabelName{"namespace", "name"})

	if nameFirst == namespaceFirst {
		t.Errorf(
			"RowKey{name,namespace} and RowKey{namespace,name} both produced key %q: "+
				"the component order of RowKey is part of the series identity",
			nameFirst,
		)
	}

	// The key must also differ from the swapped *values*, which the old sorted
	// join conflated with the original identity.
	swapped := generateKey(model.Metric{"name": "beta", "namespace": "alpha"}, []model.LabelName{"name", "namespace"})
	if nameFirst == swapped {
		t.Errorf(
			"name=alpha namespace=beta and name=beta namespace=alpha share key %q: "+
				"sorting components destroys the label roles",
			nameFirst,
		)
	}
}

func TestGenerateKeyIsInjective(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a    model.Metric
		b    model.Metric
	}{
		{
			name: "comma inside a value",
			a:    model.Metric{"pod": "a,b", "namespace": "c"},
			b:    model.Metric{"pod": "a", "namespace": "b,c"},
		},
		{
			name: "digits that can be read as a length prefix",
			a:    model.Metric{"pod": "12", "namespace": "3"},
			b:    model.Metric{"pod": "1", "namespace": "23"},
		},
		{
			name: "empty component",
			a:    model.Metric{"pod": "", "namespace": "abc"},
			b:    model.Metric{"pod": "abc", "namespace": ""},
		},
		{
			name: "pipe inside a value",
			a:    model.Metric{"pod": "a|b", "namespace": "c"},
			b:    model.Metric{"pod": "a", "namespace": "b|c"},
		},
	}

	keys := []model.LabelName{"pod", "namespace"}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			aKey := generateKey(tt.a, keys)
			bKey := generateKey(tt.b, keys)
			if aKey == bKey {
				t.Errorf(
					"distinct label tuples %v and %v share key %q: "+
						"a label value can impersonate a component boundary",
					tt.a, tt.b, aKey,
				)
			}
		})
	}
}

// TestGenerateKeyFormat pins the encoding. Keys never leave the process, but a
// change here changes every results-map identity, so it must be deliberate.
func TestGenerateKeyFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		metric model.Metric
		keys   []model.LabelName
		want   string
	}{
		{
			name:   "single component is the raw value",
			metric: model.Metric{"namespace": "prod"},
			keys:   []model.LabelName{"namespace"},
			want:   "prod",
		},
		{
			name:   "three components are length prefixed in RowKey order",
			metric: model.Metric{"container": "api", "pod": "web-0", "namespace": "prod"},
			keys:   []model.LabelName{"container", "pod", "namespace"},
			want:   "3:api5:web-04:prod",
		},
		{
			name:   "empty component keeps its slot",
			metric: model.Metric{"container": "", "pod": "web-0", "namespace": "prod"},
			keys:   []model.LabelName{"container", "pod", "namespace"},
			want:   "0:5:web-04:prod",
		},
		{
			name:   "no components",
			metric: model.Metric{},
			keys:   nil,
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := generateKey(tt.metric, tt.keys); got != tt.want {
				t.Errorf("generateKey(%v, %v) = %q, want %q", tt.metric, tt.keys, got, tt.want)
			}
		})
	}
}

// TestKeyFromValuesMatchesGenerateKey covers the call sites that rebuild the key
// of a map populated by generateKey (non-MIG GPU capacity lookup): they must pass
// the values in that query's RowKey order.
func TestKeyFromValuesMatchesGenerateKey(t *testing.T) {
	t.Parallel()

	metric := model.Metric{"pod": "web-0", "namespace": "prod", "node": "node-a"}

	capacityKey := generateKey(metric, []model.LabelName{"pod", "namespace", "node"})
	if rebuilt := keyFromValues("web-0", "prod", "node-a"); rebuilt != capacityKey {
		t.Errorf("keyFromValues in RowKey order = %q, want %q", rebuilt, capacityKey)
	}
	if reordered := keyFromValues("prod", "web-0", "node-a"); reordered == capacityKey {
		t.Errorf("keyFromValues out of RowKey order = %q, want a different identity", reordered)
	}

	single := generateKey(model.Metric{"node": "node-a"}, []model.LabelName{"node"})
	if single != "node-a" || keyFromValues("node-a") != single {
		t.Errorf("single-component keys must stay the raw value, got %q and %q", single, keyFromValues("node-a"))
	}
}

// TestIterateMatrixKeepsPermutedIdentitiesApart is the VM reproducer: two virtual
// machines whose name and namespace values are swapped collapsed into a single
// row, and the ROS parser accepted the result.
func TestIterateMatrixKeepsPermutedIdentitiesApart(t *testing.T) {
	t.Parallel()

	q := query{
		Name:       "vm-ros-cpu-usage-mc",
		MetricKey:  staticFields{"name": "name", "namespace": "namespace", "node": "node"},
		QueryValue: &saveQueryValue{ValName: "cpu_usage_mc", Method: "sum"},
		RowKey:     []model.LabelName{"name", "namespace", "node"},
	}
	matrix := model.Matrix{
		{
			Metric: model.Metric{"name": "alpha", "namespace": "beta", "node": "node-a"},
			Values: []model.SamplePair{{Timestamp: 1604691780, Value: 900}},
		},
		{
			Metric: model.Metric{"name": "beta", "namespace": "alpha", "node": "node-a"},
			Values: []model.SamplePair{{Timestamp: 1604691780, Value: 100}},
		},
	}

	results := mappedResults{}
	results.iterateMatrix(matrix, q)

	if len(results) != 2 {
		t.Fatalf(
			"iterateMatrix produced %d rows for 2 distinct VMs: swapping name and namespace "+
				"collapsed them into one identity (results: %v)",
			len(results), results,
		)
	}

	byName := map[string]mappedValues{}
	for _, val := range results {
		byName[stringValue(val, "name")] = val
	}
	for name, want := range map[string]string{"alpha": "900.000000", "beta": "100.000000"} {
		val, ok := byName[name]
		if !ok {
			t.Fatalf("no row for VM name %q, got rows: %v", name, results)
		}
		if got := stringValue(val, "cpu_usage_mc"); got != want {
			t.Errorf("VM %q cpu_usage_mc = %q, want %q (values were misattributed across VMs)", name, got, want)
		}
		if got := stringValue(val, "namespace"); got != map[string]string{"alpha": "beta", "beta": "alpha"}[name] {
			t.Errorf("VM %q namespace = %q, want the namespace that owns it", name, got)
		}
	}
}

// TestVMHourlyAggregatorKeepsPermutedIdentitiesApart covers the four-interval
// aggregation path: samples from different VMs must not land under one key, or
// averageVMSamples averages two virtual machines together.
func TestVMHourlyAggregatorKeepsPermutedIdentitiesApart(t *testing.T) {
	t.Parallel()

	hourRange := promv1.Range{}
	agg := newVMHourlyAggregator(&hourRange)

	q := query{
		Name:       "vm-ros-cpu-usage-mc",
		MetricKey:  staticFields{"name": "name", "namespace": "namespace", "node": "node"},
		QueryValue: &saveQueryValue{ValName: "cpu_usage_mc"},
		RowKey:     []model.LabelName{"name", "namespace", "node"},
	}
	interval := func(value float64) mappedResults {
		results := mappedResults{}
		results.iterateVector(model.Vector{{
			Metric: model.Metric{"name": "alpha", "namespace": "beta", "node": "node-a"},
			Value:  model.SampleValue(value),
		}, {
			Metric: model.Metric{"name": "beta", "namespace": "alpha", "node": "node-a"},
			Value:  model.SampleValue(value * 2),
		}}, q)
		return results
	}

	agg.addSample(interval(1))
	agg.addSample(interval(2))

	if len(agg.samples) != 2 {
		t.Fatalf(
			"aggregator holds %d sample groups for 2 distinct VMs: permuted identities were merged "+
				"across intervals (samples: %v)",
			len(agg.samples), agg.samples,
		)
	}
	for key, samples := range agg.samples {
		if len(samples) != 2 {
			t.Errorf("identity %q holds %d samples, want one per interval", key, len(samples))
		}
		if got := stringValue(samples[0], "name"); got != stringValue(samples[1], "name") {
			t.Errorf("identity %q mixes VM names %q and %q", key, got, stringValue(samples[1], "name"))
		}
	}
}

// TestReversedQueryOrderKeepsRequestAndUsageTogether is #764's order criterion:
// the request and usage queries return the same VMs in opposite order, so a
// shared identity would pair one VM's request with the other VM's usage.
func TestReversedQueryOrderKeepsRequestAndUsageTogether(t *testing.T) {
	t.Parallel()

	rowKey := []model.LabelName{"name", "namespace", "node"}
	metricKey := staticFields{"name": "name", "namespace": "namespace", "node": "node"}
	requestQuery := query{
		Name:       "vm-ros-cpu-request-mc",
		MetricKey:  metricKey,
		QueryValue: &saveQueryValue{ValName: "cpu_request_mc"},
		RowKey:     rowKey,
	}
	usageQuery := query{
		Name:       "vm-ros-cpu-usage-mc",
		MetricKey:  metricKey,
		QueryValue: &saveQueryValue{ValName: "cpu_usage_mc"},
		RowKey:     rowKey,
	}

	vmAlpha := model.Metric{"name": "alpha", "namespace": "beta", "node": "node-a"}
	vmBeta := model.Metric{"name": "beta", "namespace": "alpha", "node": "node-a"}
	requestVector := model.Vector{
		{Metric: vmAlpha, Value: 200},
		{Metric: vmBeta, Value: 400},
	}
	usageVector := model.Vector{
		{Metric: vmBeta, Value: 100},
		{Metric: vmAlpha, Value: 900},
	}

	results := mappedResults{}
	results.iterateVector(requestVector, requestQuery)
	results.iterateVector(usageVector, usageQuery)

	if len(results) != 2 {
		t.Fatalf(
			"two queries nested %d identities for 2 distinct VMs: a shared identity pairs one VM's "+
				"request with the other VM's usage (results: %v)",
			len(results), results,
		)
	}

	want := map[string]struct{ namespace, request, usage string }{
		"alpha": {"beta", "200.000000", "900.000000"},
		"beta":  {"alpha", "400.000000", "100.000000"},
	}
	seen := map[string]bool{}
	for _, val := range results {
		name := stringValue(val, "name")
		expected, ok := want[name]
		if !ok {
			t.Fatalf("unexpected VM name %q in results: %v", name, results)
		}
		seen[name] = true
		if got := stringValue(val, "namespace"); got != expected.namespace {
			t.Errorf("VM %q namespace = %q, want %q: the label roles were lost", name, got, expected.namespace)
		}
		if got := stringValue(val, "cpu_request_mc"); got != expected.request {
			t.Errorf("VM %q cpu_request_mc = %q, want %q: request and usage mixed across identities", name, got, expected.request)
		}
		if got := stringValue(val, "cpu_usage_mc"); got != expected.usage {
			t.Errorf("VM %q cpu_usage_mc = %q, want %q: request and usage mixed across identities", name, got, expected.usage)
		}
	}
	for name := range want {
		if !seen[name] {
			t.Errorf("no row for VM %q: its fields were attributed to another identity", name)
		}
	}
}

func TestMergeRosNamespaceCSVRows(t *testing.T) {
	t.Parallel()

	ts := &promv1.Range{}
	tests := []struct {
		name      string
		namespace string
		quota     string
	}{
		{
			name:      "quota name sorts before the namespace",
			namespace: "costmanagement-metrics-operator",
			quota:     "compute-resources",
		},
		{
			name:      "quota name sorts after the namespace",
			namespace: "alpha-tmp",
			quota:     "z-quota",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			usageResults := mappedResults{
				tt.namespace: {
					"namespace":                  tt.namespace,
					"cpu-usage-namespace-avg":    "0.500000",
					"memory-usage-namespace-avg": "1024.000000",
				},
				"no-quota-ns": {
					"namespace":               "no-quota-ns",
					"cpu-usage-namespace-avg": "0.250000",
				},
			}
			quotaResults := mappedResults{
				keyFromValues(tt.namespace, tt.quota): {
					"namespace":                 tt.namespace,
					"quota_name":                tt.quota,
					"cpu-request-namespace-sum": "0.100000",
					"pods-namespace-hard":       "50.000000",
				},
			}

			rows := mergeRosNamespaceCSVRows(usageResults, quotaResults, ts)

			if len(rows) != 2 {
				t.Fatalf(
					"merge produced %d rows, want one per namespace (2): a namespace with a quota "+
						"must not also appear as a quota-less usage row (rows: %v)",
					len(rows), rows,
				)
			}

			var quotaRow *rosNamespaceRow
			for _, row := range rows {
				if r, ok := row.(*rosNamespaceRow); ok && r.QuotaName != "" {
					quotaRow = r
				}
			}
			if quotaRow == nil {
				t.Fatalf("merge dropped the quota row for namespace %q: %v", tt.namespace, rows)
			}
			if quotaRow.Namespace != tt.namespace {
				t.Errorf("quota row namespace = %q, want %q", quotaRow.Namespace, tt.namespace)
			}
			if quotaRow.QuotaName != tt.quota {
				t.Errorf("quota row quota_name = %q, want %q", quotaRow.QuotaName, tt.quota)
			}

			// The quota row must carry the namespace usage fields, otherwise the
			// ROS parser skips it and the quota never reaches the service.
			if quotaRow.CPUUsageAvg != "0.500000" {
				t.Errorf(
					"quota row cpu_usage_namespace_avg = %q, want 0.500000: the merge discarded the "+
						"namespace usage fields of the quota row",
					quotaRow.CPUUsageAvg,
				)
			}
			if quotaRow.MemoryUsageAvg != "1024.000000" {
				t.Errorf(
					"quota row memory_usage_namespace_avg = %q, want 1024.000000: the merge discarded "+
						"the namespace usage fields of the quota row",
					quotaRow.MemoryUsageAvg,
				)
			}
			if quotaRow.CPURequestSum != "0.100000" || quotaRow.PodsHard != "50.000000" {
				t.Errorf(
					"quota row lost its quota fields: cpu_request_namespace_sum=%q pods_namespace_hard=%q",
					quotaRow.CPURequestSum, quotaRow.PodsHard,
				)
			}
		})
	}
}

func TestMergeRosNamespaceCSVRowsQuotaWithoutUsage(t *testing.T) {
	t.Parallel()

	ts := &promv1.Range{}
	rows := mergeRosNamespaceCSVRows(
		mappedResults{},
		mappedResults{
			keyFromValues("quiet-ns", "compute-resources"): {
				"namespace":                 "quiet-ns",
				"quota_name":                "compute-resources",
				"cpu-request-namespace-sum": "2.000000",
			},
		},
		ts,
	)

	if len(rows) != 1 {
		t.Fatalf("merge produced %d rows for a quota on a namespace without usage, want 1: %v", len(rows), rows)
	}
	row, ok := rows[keyFromValues("quiet-ns", "compute-resources")].(*rosNamespaceRow)
	if !ok {
		t.Fatalf("row is not a *rosNamespaceRow: %T", rows[keyFromValues("quiet-ns", "compute-resources")])
	}
	if row.Namespace != "quiet-ns" || row.CPURequestSum != "2.000000" {
		t.Errorf("quota-only row = namespace %q cpu_request_namespace_sum %q, want quiet-ns and 2.000000",
			row.Namespace, row.CPURequestSum)
	}
}

// TestPivotAndMergeNamespaceQuotaKeepsUsage drives the production path end to end:
// raw quota query results -> pivot -> merge. It fails when either the pivot key
// encoding loses the (namespace, quota) roles or the merge parses the key instead
// of reading the row values.
func TestPivotAndMergeNamespaceQuotaKeepsUsage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		namespace string
		quota     string
	}{
		{
			name:      "quota name sorts before the namespace",
			namespace: "costmanagement-metrics-operator",
			quota:     "compute-resources",
		},
		{
			name:      "quota name sorts after the namespace",
			namespace: "alpha-tmp",
			quota:     "z-quota",
		},
	}

	hardQuery := namespaceQuotaQueryByName(t, "namespace-quota-hard")
	usedQuery := namespaceQuotaQueryByName(t, "namespace-quota-used")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			raw := mappedResults{}
			raw.iterateVector(model.Vector{{
				Metric: model.Metric{
					"namespace":     model.LabelValue(tt.namespace),
					"resource":      "requests.cpu",
					"resourcequota": model.LabelValue(tt.quota),
				},
				Value: 0.1,
			}}, hardQuery)
			raw.iterateVector(model.Vector{{
				Metric: model.Metric{
					"namespace":     model.LabelValue(tt.namespace),
					"resource":      "requests.cpu",
					"resourcequota": model.LabelValue(tt.quota),
				},
				Value: 0.05,
			}}, usedQuery)

			usageResults := mappedResults{
				tt.namespace: {
					"namespace":               tt.namespace,
					"cpu-usage-namespace-avg": "0.500000",
				},
			}

			rows := mergeRosNamespaceCSVRows(usageResults, pivotNamespaceQuotaResults(raw), &promv1.Range{})

			if len(rows) != 1 {
				t.Fatalf(
					"pivot+merge produced %d rows for one namespace with one quota, want 1: "+
						"the quota row and the usage row were not merged (rows: %v)",
					len(rows), rows,
				)
			}

			var row *rosNamespaceRow
			for _, got := range rows {
				if r, ok := got.(*rosNamespaceRow); ok {
					row = r
				}
			}
			if row == nil {
				t.Fatalf("merged row is not a *rosNamespaceRow: %v", rows)
			}
			if row.Namespace != tt.namespace || row.QuotaName != tt.quota {
				t.Errorf(
					"merged row is namespace=%q quota_name=%q, want namespace=%q quota_name=%q: "+
						"the (namespace, quota) roles were not preserved",
					row.Namespace, row.QuotaName, tt.namespace, tt.quota,
				)
			}
			if row.CPUUsageAvg != "0.500000" {
				t.Errorf(
					"merged row cpu_usage_namespace_avg = %q, want 0.500000: the quota row lost the "+
						"namespace usage fields and the ROS parser would skip it",
					row.CPUUsageAvg,
				)
			}
			if row.CPURequestSum != "0.100000" || row.CPURequestUsed != "0.050000" {
				t.Errorf(
					"merged row lost its quota values: cpu_request_namespace_sum=%q cpu_request_namespace_used=%q",
					row.CPURequestSum, row.CPURequestUsed,
				)
			}
		})
	}
}

// namespaceQuotaQueryByName returns the unified quota query with the given name.
func namespaceQuotaQueryByName(t *testing.T, name string) query {
	t.Helper()

	for _, q := range *rosNamespaceQuotaQueries {
		if q.Name == name {
			return q
		}
	}
	t.Fatalf("rosNamespaceQuotaQueries has no query named %q", name)
	return query{}
}

// TestMakeQuotaPivotKeyIsInjective guards the pivot map against the collision the
// sorted join allowed, where a namespace and a quota name could trade places.
func TestMakeQuotaPivotKeyIsInjective(t *testing.T) {
	t.Parallel()

	if makeQuotaPivotKey("a,b", "c") == makeQuotaPivotKey("a", "b,c") {
		t.Errorf(
			"makeQuotaPivotKey(%q,%q) collides with makeQuotaPivotKey(%q,%q): "+
				"distinct (namespace, quota) pairs share a pivot key",
			"a,b", "c", "a", "b,c",
		)
	}
	if makeQuotaPivotKey("ns", "quota") == makeQuotaPivotKey("quota", "ns") {
		t.Errorf("makeQuotaPivotKey does not preserve the namespace/quota roles")
	}
}
