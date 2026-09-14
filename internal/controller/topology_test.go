//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package controller

import (
	"errors"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	configv1 "github.com/openshift/api/config/v1"

	metricscfgv1beta1 "github.com/project-koku/koku-metrics-operator/api/v1beta1"
)

func nodeWithRoles(name string, roles ...string) corev1.Node {
	labels := map[string]string{}
	for _, r := range roles {
		labels["node-role.kubernetes.io/"+r] = ""
	}
	return corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels}}
}

func testInfrastructure(topology configv1.TopologyMode, labels map[string]string) *configv1.Infrastructure {
	return &configv1.Infrastructure{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster", Labels: labels},
		Status:     configv1.InfrastructureStatus{ControlPlaneTopology: topology},
	}
}

func TestBuildTopologyStatus(t *testing.T) {
	cases := []struct {
		name       string
		infra      *configv1.Infrastructure
		nodes      []corev1.Node
		hcCount    int
		hcpNS      []string
		wantTopo   string
		wantMgmt   bool
		wantMaster int32
		wantWorker int32
	}{
		{
			name:       "external management with HCP namespace and compact nodes",
			infra:      testInfrastructure(configv1.ExternalTopologyMode, map[string]string{"hypershift.openshift.io/managed": "true"}),
			nodes:      []corev1.Node{nodeWithRoles("n0", "control-plane", "worker"), nodeWithRoles("n1", "control-plane", "worker"), nodeWithRoles("n2", "control-plane", "worker")},
			hcCount:    1,
			hcpNS:      []string{"hc01-infra-hc01"},
			wantTopo:   "External",
			wantMgmt:   true,
			wantMaster: 3,
			wantWorker: 3,
		},
		{
			name:       "standalone HA without hypershift",
			infra:      testInfrastructure(configv1.HighlyAvailableTopologyMode, nil),
			nodes:      []corev1.Node{nodeWithRoles("m0", "master"), nodeWithRoles("m1", "master"), nodeWithRoles("m2", "master"), nodeWithRoles("w0", "worker"), nodeWithRoles("w1", "worker")},
			hcCount:    0,
			hcpNS:      nil,
			wantTopo:   "HighlyAvailable",
			wantMgmt:   false,
			wantMaster: 3,
			wantWorker: 2,
		},
		{
			name:       "managed label false is not managed",
			infra:      testInfrastructure(configv1.ExternalTopologyMode, map[string]string{"hypershift.openshift.io/managed": "false"}),
			nodes:      []corev1.Node{nodeWithRoles("n0", "control-plane")},
			hcCount:    0,
			hcpNS:      nil,
			wantTopo:   "External",
			wantMgmt:   false,
			wantMaster: 1,
			wantWorker: 0,
		},
		{
			name:       "nil infrastructure yields zero facts",
			infra:      nil,
			nodes:      []corev1.Node{nodeWithRoles("n0", "worker")},
			hcCount:    0,
			hcpNS:      nil,
			wantTopo:   "",
			wantMgmt:   false,
			wantMaster: 0,
			wantWorker: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Assign through the CRD status struct: fails to compile if the
			// Topology field is missing or misnamed (API wiring guard).
			var st metricscfgv1beta1.CostManagementMetricsConfigStatus
			st.Topology = buildTopologyStatus(tc.infra, tc.nodes, tc.hcCount, tc.hcpNS)
			got := st.Topology
			if got.ControlPlaneTopology != tc.wantTopo {
				t.Errorf("ControlPlaneTopology = %q, want %q", got.ControlPlaneTopology, tc.wantTopo)
			}
			if got.ManagedByHypershift != tc.wantMgmt {
				t.Errorf("ManagedByHypershift = %v, want %v", got.ManagedByHypershift, tc.wantMgmt)
			}
			if got.MasterNodes != tc.wantMaster {
				t.Errorf("MasterNodes = %d, want %d", got.MasterNodes, tc.wantMaster)
			}
			if got.WorkerNodes != tc.wantWorker {
				t.Errorf("WorkerNodes = %d, want %d", got.WorkerNodes, tc.wantWorker)
			}
			if int(got.HostedClusterCount) != tc.hcCount {
				t.Errorf("HostedClusterCount = %d, want %d", got.HostedClusterCount, tc.hcCount)
			}
			if !reflect.DeepEqual(got.HostedControlPlaneNamespaces, tc.hcpNS) && (len(got.HostedControlPlaneNamespaces) != 0 || len(tc.hcpNS) != 0) {
				t.Errorf("HostedControlPlaneNamespaces = %v, want %v", got.HostedControlPlaneNamespaces, tc.hcpNS)
			}
			if got.CollectionError != "" {
				t.Errorf("CollectionError = %q, want empty (pure builder never errors)", got.CollectionError)
			}
		})
	}
}

func TestIsMissingAPIError(t *testing.T) {
	noMatch := &apimeta.NoKindMatchError{
		GroupKind: schema.GroupKind{Group: "hypershift.openshift.io", Kind: "HostedCluster"},
	}
	notFound := apierrors.NewNotFound(schema.GroupResource{Group: "hypershift.openshift.io", Resource: "hostedclusters"}, "")
	forbidden := apierrors.NewForbidden(schema.GroupResource{Group: "hypershift.openshift.io", Resource: "hostedclusters"}, "", errors.New("denied"))
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"no-kind-match means absent API", noMatch, true},
		{"not-found means absent API", notFound, true},
		{"nil means present", nil, false},
		{"forbidden is a real failure, not absence", forbidden, false},
		{"generic errors propagate", errors.New("boom"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isMissingAPIError(tc.err); got != tc.want {
				t.Errorf("isMissingAPIError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
