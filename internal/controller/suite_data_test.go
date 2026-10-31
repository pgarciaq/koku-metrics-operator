package controller

import (
	"encoding/json"

	"github.com/prometheus/common/model"
)

// the data in this file is used to mock Prometheus queries in controller tests

var metricjson string = `
{
	"label_topology_kubernetes_io_zone": "us-east-2a",
	"namespace": "openshift-monitoring",
	"node": "ip-10-0-146-115.us-east-2.compute.internal",
	"provider_id": "aws:///us-east-2a/i-0eb3a4cb7807fb144",
	"role": "worker",
	"pod": "hive-server-0"
}
`
var nodeallocatablecpucores string = `
[
	[
		1604685600,
		"7.5"
	]
]
`
var noderole string = `
[
	[
		1604685600,
		"1"
	]
]
`
var nodelabels string = `
[
	[
		1604685600,
		"1"
	]
]`
var podlimitcpucores string = `
[
	[
		1604685600,
		"1"
	]
]
`

// nodeAllocMatrixJSON and nodeCapMatrixJSON are the responses to the unified
// node allocatable/capacity queries (`cost:node_allocatable`, `cost:node_capacity`).
// Unlike metricjson they carry the `resource` label, because
// collector.pivotNodeMatrix drops any stream without one; an hour whose streams
// all lack it reads as ErrNoData ("No data to report for the hour queried."),
// even though Prometheus returned samples.
var nodeAllocMatrixJSON string = `
[
	{
		"metric": {
			"node": "ip-10-0-146-115.us-east-2.compute.internal",
			"provider_id": "aws:///us-east-2a/i-0eb3a4cb7807fb144",
			"resource": "cpu"
		},
		"values": [[1604685600, "7.5"]]
	},
	{
		"metric": {
			"node": "ip-10-0-146-115.us-east-2.compute.internal",
			"provider_id": "aws:///us-east-2a/i-0eb3a4cb7807fb144",
			"resource": "memory"
		},
		"values": [[1604685600, "32058703872"]]
	},
	{
		"metric": {
			"node": "ip-10-0-146-115.us-east-2.compute.internal",
			"provider_id": "aws:///us-east-2a/i-0eb3a4cb7807fb144",
			"resource": "nvidia.com/gpu"
		},
		"values": [[1604685600, "0"]]
	}
]
`
var nodeCapMatrixJSON string = `
[
	{
		"metric": {
			"node": "ip-10-0-146-115.us-east-2.compute.internal",
			"provider_id": "aws:///us-east-2a/i-0eb3a4cb7807fb144",
			"resource": "cpu"
		},
		"values": [[1604685600, "8"]]
	},
	{
		"metric": {
			"node": "ip-10-0-146-115.us-east-2.compute.internal",
			"provider_id": "aws:///us-east-2a/i-0eb3a4cb7807fb144",
			"resource": "memory"
		},
		"values": [[1604685600, "33237303296"]]
	},
	{
		"metric": {
			"node": "ip-10-0-146-115.us-east-2.compute.internal",
			"provider_id": "aws:///us-east-2a/i-0eb3a4cb7807fb144",
			"resource": "pods"
		},
		"values": [[1604685600, "250"]]
	}
]
`

// asMatrix parses a whole matrix fixture, the shape a range query returns.
func asMatrix(matrix string) model.Matrix {
	var m model.Matrix
	if err := json.Unmarshal([]byte(matrix), &m); err != nil {
		panic(err)
	}
	return m
}

func asModelMatrix(metric, value string) model.Matrix {
	var m model.Metric
	var v []model.SamplePair

	if err := json.Unmarshal([]byte(metric), &m); err != nil {
		panic(err)
	}
	if err := json.Unmarshal([]byte(value), &v); err != nil {
		panic(err)
	}
	return model.Matrix{&model.SampleStream{Metric: m, Values: v}}
}
