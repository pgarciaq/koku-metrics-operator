//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/prometheus/common/model"
)

func init() {
	initNodeAllocCapQueryMap()
}

func initNodeAllocCapQueryMap() {
	QueryMap["cost:node_allocatable"] = "kube_node_status_allocatable * on(node) group_left(provider_id) max by (node, provider_id) (kube_node_info)"
	QueryMap["cost:node_capacity"] = "kube_node_status_capacity * on(node) group_left(provider_id) max by (node, provider_id) (kube_node_info)"
}

// nodeAllocCapMetricKeys lists the QueryMap keys used by the unified node alloc/cap queries.
var nodeAllocCapMetricKeys = []string{
	"cost:node_allocatable",
	"cost:node_capacity",
}

// nodeAllocResourceFieldMap maps Prometheus resource label values from kube_node_status_allocatable
// to the mapstructure field names used by nodeRow. The second element (if non-empty) is the
// -seconds transformed field name.
var nodeAllocResourceFieldMap = map[string][2]string{
	"cpu":            {"node-allocatable-cpu-cores", ""},
	"memory":         {"node-allocatable-memory-bytes", ""},
	"nvidia.com/gpu": {"node-allocatable-gpu-count", ""},
}

// nodeCapResourceFieldMap maps Prometheus resource label values from kube_node_status_capacity
// to the mapstructure field names used by nodeRow. The second element (if non-empty) is the
// -seconds transformed field name.
var nodeCapResourceFieldMap = map[string][2]string{
	"cpu":    {"node-capacity-cpu-cores", "node-capacity-cpu-core-seconds"},
	"memory": {"node-capacity-memory-bytes", "node-capacity-memory-byte-seconds"},
	"pods":   {"node-capacity-pods", ""},
}

// getNodeAllocCapRangeResults executes the 2 unified allocatable/capacity range queries,
// pivots the matrix results by resource label, and returns a mappedResults keyed by node
// with all the named fields that nodeRow expects (including -seconds transformations).
func (c *PrometheusCollector) getNodeAllocCapRangeResults(retries int) (mappedResults, error) {
	log := log.WithName("getNodeAllocCapRangeResults")

	type querySpec struct {
		name     string
		qs       string
		fieldMap map[string][2]string
	}
	specs := []querySpec{
		{"node-allocatable", QueryMap["cost:node_allocatable"], nodeAllocResourceFieldMap},
		{"node-capacity", QueryMap["cost:node_capacity"], nodeCapResourceFieldMap},
	}

	out := make(mappedResults)

	for _, spec := range specs {
		matrix, err := c.queryRangeWithRetries(spec.name, spec.qs, retries)
		if err != nil {
			return nil, err
		}
		pivotNodeMatrix(matrix, spec.fieldMap, out)
	}

	log.Info(fmt.Sprintf("pivoted allocatable/capacity data for %d nodes", len(out)))
	return out, nil
}

// queryRangeWithRetries executes a single range query with retry logic.
func (c *PrometheusCollector) queryRangeWithRetries(name, queryString string, retries int) (model.Matrix, error) {
	log := log.WithName("queryRangeWithRetries")
	for attempt := 0; ; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), c.ContextTimeout)
		defer cancel()
		result, warnings, err := c.PromConn.QueryRange(ctx, queryString, *c.TimeSeries)
		if err != nil {
			if attempt < retries {
				sleep := math.Max(math.Pow(2, float64(attempt+1)), 1)
				waitTime := time.Duration(sleep) * time.Second
				log.Info(fmt.Sprintf("query `%s` failed (attempt %d/%d), retrying after %s", name, attempt+1, retries+1, waitTime))
				time.Sleep(waitTime)
				continue
			}
			return nil, fmt.Errorf("query: %s: error querying prometheus: %v", queryString, err)
		}
		if len(warnings) > 0 {
			log.Info("query warnings", "Warnings", warnings)
		}
		matrix, ok := result.(model.Matrix)
		if !ok {
			return nil, fmt.Errorf("expected a matrix in response to query %s, got a %v", name, result.Type())
		}
		return matrix, nil
	}
}

// pivotNodeMatrix processes a matrix result from a unified allocatable or capacity query,
// pivoting by resource label into the correct nodeRow field names.
func pivotNodeMatrix(matrix model.Matrix, fieldMap map[string][2]string, out mappedResults) {
	for _, stream := range matrix {
		node := string(stream.Metric["node"])
		resource := string(stream.Metric["resource"])
		if node == "" || resource == "" {
			continue
		}

		fields, ok := fieldMap[resource]
		if !ok {
			continue
		}

		if out[node] == nil {
			out[node] = mappedValues{
				"node":        node,
				"provider_id": string(stream.Metric["provider_id"]),
			}
		}

		value := maxSlice(stream.Values)
		out[node][fields[0]] = floatToString(value)

		if fields[1] != "" {
			factor := float64(60) * float64(len(stream.Values))
			out[node][fields[1]] = floatToString(value * factor)
		}
	}
}
