//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package controller

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"strings"

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

func hcpTestNSInfos(names ...string) map[string]nsObserved {
	out := make(map[string]nsObserved, len(names))
	for _, n := range names {
		out[n] = nsObserved{UID: "uid-ns-" + n, CreatedAt: "2026-09-26T19:10:57Z"}
	}
	return out
}

// TestBuildHCPSnapshot_Matrix pins the mapping rule with adversarial values:
// distinct IDs/UIDs/namespaces per case so agreement-by-coincidence cannot
// green it. Mirrors the backend temporal matrix (ros-ocp-backend #621): the
// operator proves one incarnation per namespace, the backend proves it covers
// the window.
func TestBuildHCPSnapshot_Matrix(t *testing.T) {
	const ts = "2026-09-28T12:00:00Z"
	hcp := func(ns, name, uid, id string) hcpObserved {
		return hcpObserved{Namespace: ns, Name: name, UID: uid, ClusterID: id}
	}
	hc := func(ns, name, uid, id string) hcObserved {
		return hcObserved{Namespace: ns, Name: name, UID: uid, ClusterID: id}
	}
	complete := func(ns, id, hcUID, hcpUID string) metricscfgv1beta1.HCPSnapshotEntry {
		return metricscfgv1beta1.HCPSnapshotEntry{
			HCPNamespace: ns, HostedClusterID: id, HcUID: hcUID, HcpUID: hcpUID,
			NamespaceUID: "uid-ns-" + ns, NamespaceCreatedAt: "2026-09-26T19:10:57Z",
			ObservedAt: ts, Complete: true,
		}
	}
	incomplete := func(ns, diag string) metricscfgv1beta1.HCPSnapshotEntry {
		return metricscfgv1beta1.HCPSnapshotEntry{
			HCPNamespace: ns, NamespaceUID: "uid-ns-" + ns,
			NamespaceCreatedAt: "2026-09-26T19:10:57Z", ObservedAt: ts,
			Diagnostics: diag,
		}
	}

	cases := []struct {
		name      string
		hcps      []hcpObserved
		hcs       []hcObserved
		labeledNS []string
		nsInfos   map[string]nsObserved
		want      []metricscfgv1beta1.HCPSnapshotEntry
	}{
		{
			name:      "single HCP cross-checked to one HC",
			hcps:      []hcpObserved{hcp("hc01-infra-hc01", "hc01", "uid-hcp-1", "d5d3-1")},
			hcs:       []hcObserved{hc("hc01-infra", "hc01", "uid-hc-1", "d5d3-1")},
			labeledNS: []string{"hc01-infra-hc01"},
			nsInfos:   hcpTestNSInfos("hc01-infra-hc01"),
			want:      []metricscfgv1beta1.HCPSnapshotEntry{complete("hc01-infra-hc01", "d5d3-1", "uid-hc-1", "uid-hcp-1")},
		},
		{
			name: "two hosted clusters map independently",
			hcps: []hcpObserved{
				hcp("mgmt-hc1", "hc1", "uid-hcp-1", "id-1"),
				hcp("mgmt-hc2", "hc2", "uid-hcp-2", "id-2"),
			},
			hcs: []hcObserved{
				hc("infra1", "hc1", "uid-hc-1", "id-1"),
				hc("infra2", "hc2", "uid-hc-2", "id-2"),
			},
			labeledNS: []string{"mgmt-hc2", "mgmt-hc1"},
			nsInfos:   hcpTestNSInfos("mgmt-hc1", "mgmt-hc2"),
			want: []metricscfgv1beta1.HCPSnapshotEntry{
				complete("mgmt-hc1", "id-1", "uid-hc-1", "uid-hcp-1"),
				complete("mgmt-hc2", "id-2", "uid-hc-2", "uid-hcp-2"),
			},
		},
		{
			name:      "no HCP object in namespace fails closed",
			hcps:      nil,
			hcs:       []hcObserved{hc("infra", "hc1", "uid-hc-1", "id-1")},
			labeledNS: []string{"mgmt-hc1"},
			nsInfos:   hcpTestNSInfos("mgmt-hc1"),
			want:      []metricscfgv1beta1.HCPSnapshotEntry{incomplete("mgmt-hc1", "expected 1 HostedControlPlane in namespace, found 0")},
		},
		{
			name: "two HCP objects in one namespace fail closed",
			hcps: []hcpObserved{
				hcp("mgmt-hc1", "a", "uid-hcp-a", "id-1"),
				hcp("mgmt-hc1", "b", "uid-hcp-b", "id-1"),
			},
			hcs:       []hcObserved{hc("infra", "hc1", "uid-hc-1", "id-1")},
			labeledNS: []string{"mgmt-hc1"},
			nsInfos:   hcpTestNSInfos("mgmt-hc1"),
			want:      []metricscfgv1beta1.HCPSnapshotEntry{incomplete("mgmt-hc1", "expected 1 HostedControlPlane in namespace, found 2")},
		},
		{
			name:      "empty clusterID fails closed",
			hcps:      []hcpObserved{hcp("mgmt-hc1", "hc1", "uid-hcp-1", "")},
			hcs:       []hcObserved{hc("infra", "hc1", "uid-hc-1", "id-1")},
			labeledNS: []string{"mgmt-hc1"},
			nsInfos:   hcpTestNSInfos("mgmt-hc1"),
			want:      []metricscfgv1beta1.HCPSnapshotEntry{incomplete("mgmt-hc1", "HostedControlPlane hc1 has empty spec.clusterID")},
		},
		{
			name:      "no live HostedCluster fails closed",
			hcps:      []hcpObserved{hcp("mgmt-hc1", "hc1", "uid-hcp-1", "id-9")},
			hcs:       []hcObserved{hc("infra", "other", "uid-hc-9", "id-8")},
			labeledNS: []string{"mgmt-hc1"},
			nsInfos:   hcpTestNSInfos("mgmt-hc1"),
			want:      []metricscfgv1beta1.HCPSnapshotEntry{incomplete("mgmt-hc1", "expected 1 live HostedCluster for clusterID, found 0")},
		},
		{
			name: "duplicate live IDs fail closed",
			hcps: []hcpObserved{hcp("mgmt-hc1", "hc1", "uid-hcp-1", "id-1")},
			hcs: []hcObserved{
				hc("infra-a", "hc1a", "uid-hc-a", "id-1"),
				hc("infra-b", "hc1b", "uid-hc-b", "id-1"),
			},
			labeledNS: []string{"mgmt-hc1"},
			nsInfos:   hcpTestNSInfos("mgmt-hc1"),
			want:      []metricscfgv1beta1.HCPSnapshotEntry{incomplete("mgmt-hc1", "expected 1 live HostedCluster for clusterID, found 2")},
		},
		{
			name:      "vanished namespace fails closed",
			hcps:      []hcpObserved{hcp("mgmt-gone", "hc1", "uid-hcp-1", "id-1")},
			hcs:       []hcObserved{hc("infra", "hc1", "uid-hc-1", "id-1")},
			labeledNS: []string{"mgmt-gone"},
			nsInfos:   map[string]nsObserved{},
			want: []metricscfgv1beta1.HCPSnapshotEntry{{
				HCPNamespace: "mgmt-gone", ObservedAt: ts,
				Diagnostics: "namespace vanished between list and snapshot",
			}},
		},
		{
			name:      "no labeled namespaces yields empty",
			hcps:      []hcpObserved{hcp("mgmt-hc1", "hc1", "uid-hcp-1", "id-1")},
			hcs:       []hcObserved{hc("infra", "hc1", "uid-hc-1", "id-1")},
			labeledNS: nil,
			nsInfos:   map[string]nsObserved{},
			want:      []metricscfgv1beta1.HCPSnapshotEntry{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildHCPSnapshot(tc.hcps, tc.hcs, tc.nsInfos, tc.labeledNS, ts)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("buildHCPSnapshot = %#v, want %#v", got, tc.want)
			}
		})
	}
}

// TestBuildHCPSnapshot_FleetSizeBound proves the 100-HC fleet snapshot stays
// far under manifest/Kafka limits (#633 scope). Guards against accidentally
// embedding full objects instead of scalar fields.
func TestBuildHCPSnapshot_FleetSizeBound(t *testing.T) {
	const n = 100
	hcps := make([]hcpObserved, 0, n)
	hcs := make([]hcObserved, 0, n)
	infos := make(map[string]nsObserved, n)
	labeled := make([]string, 0, n)
	for i := 0; i < n; i++ {
		ns := "mgmt-hc-" + string(rune('a'+i/26)) + string(rune('a'+i%26))
		id := "cluster-id-" + string(rune('a'+i/26)) + string(rune('a'+i%26))
		hcps = append(hcps, hcpObserved{Namespace: ns, Name: "hc", UID: "uid-hcp", ClusterID: id})
		hcs = append(hcs, hcObserved{Namespace: "infra", Name: "hc", UID: "uid-hc", ClusterID: id})
		infos[ns] = nsObserved{UID: "uid-ns", CreatedAt: "2026-09-26T19:10:57Z"}
		labeled = append(labeled, ns)
	}
	got := buildHCPSnapshot(hcps, hcs, infos, labeled, "2026-09-28T12:00:00Z")
	if len(got) != n {
		t.Fatalf("snapshot length = %d, want %d", len(got), n)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal fleet snapshot: %v", err)
	}
	t.Logf("100-entry snapshot = %d bytes", len(raw))
	if len(raw) > 65536 {
		t.Errorf("100-entry snapshot = %d bytes, exceeds 64KiB bound (full objects embedded?)", len(raw))
	}
}

// (ros-ocp-backend librobne/topology HCPSnapshotEntry + its contract test):
// same values must marshal here and unmarshal there. Renames update both
// sides plus the shared golden fixture together.
func TestHCPSnapshot_JSONHandshake(t *testing.T) {
	entry := metricscfgv1beta1.HCPSnapshotEntry{
		HCPNamespace: "hc01-infra-hc01", HostedClusterID: "d5d31999-89ed-4c13-b5e8-c9193f62e630",
		HcUID: "uid-hc-1", HcpUID: "uid-hcp-1", NamespaceUID: "uid-ns-1",
		NamespaceCreatedAt: "2026-09-26T19:10:57Z", ObservedAt: "2026-09-28T12:00:00Z",
		Complete: true,
	}
	raw, err := json.Marshal(metricscfgv1beta1.ClusterTopologyStatus{
		HostedControlPlaneNamespaces: []string{"hc01-infra-hc01"},
		HCPSnapshot:                  []metricscfgv1beta1.HCPSnapshotEntry{entry},
	})
	if err != nil {
		t.Fatalf("marshal snapshot status: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal snapshot status: %v", err)
	}
	snap, ok := decoded["hcpSnapshot"].([]any)
	if !ok || len(snap) != 1 {
		t.Fatalf("hcpSnapshot missing or wrong length in %s", raw)
	}
	fields, ok := snap[0].(map[string]any)
	if !ok {
		t.Fatalf("snapshot entry not an object in %s", raw)
	}
	for _, key := range []string{"hcpNamespace", "hostedClusterID", "hcUID", "hcpUID", "namespaceUID", "namespaceCreatedAt", "observedAt", "complete"} {
		if _, ok := fields[key]; !ok {
			t.Errorf("wire key %q missing in %s", key, raw)
		}
	}
	if fields["hostedClusterID"] != "d5d31999-89ed-4c13-b5e8-c9193f62e630" {
		t.Errorf("hostedClusterID mismatch in %s", raw)
	}
}

func hcpTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("scheme corev1: %v", err)
	}
	if err := configv1.Install(scheme); err != nil {
		t.Fatalf("scheme configv1: %v", err)
	}
	return scheme
}

func hcpUnstructured(t *testing.T, kind, ns, name, uid, clusterID string) *unstructured.Unstructured {
	t.Helper()
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(schema.GroupVersionKind{Group: "hypershift.openshift.io", Version: "v1beta1", Kind: kind})
	u.SetNamespace(ns)
	u.SetName(name)
	u.SetUID(types.UID(uid))
	if clusterID != "" {
		if err := unstructured.SetNestedField(u.Object, clusterID, "spec", "clusterID"); err != nil {
			t.Fatalf("set clusterID: %v", err)
		}
	}
	return u
}

func hcpLabeledNS(name string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			UID:               types.UID("uid-ns-" + name),
			CreationTimestamp: metav1.NewTime(time.Date(2026, 9, 26, 19, 10, 57, 0, time.UTC)),
			Labels:            map[string]string{"hypershift.openshift.io/hosted-control-plane": "true"},
		},
	}
}

// hcpCollectFixture builds a fake client with management-like topology plus
// optional HCP-list interception. errKind selects the injected HCP LIST
// failure: "" (none), "forbidden", or "nomatch".
func hcpCollectFixture(t *testing.T, errKind string) client.Client {
	t.Helper()
	infra := testInfrastructure(configv1.HighlyAvailableTopologyMode, nil)
	node := nodeWithRoles("worker-0", "worker")
	ns := hcpLabeledNS("hc01-infra-hc01")
	hc := hcpUnstructured(t, "HostedCluster", "hc01-infra", "hc01", "uid-hc-1", "d5d3-1")
	hcp := hcpUnstructured(t, "HostedControlPlane", "hc01-infra-hc01", "hc01", "uid-hcp-1", "d5d3-1")

	b := fake.NewClientBuilder().WithScheme(hcpTestScheme(t)).
		WithObjects(infra, &node, ns, hc, hcp)
	if errKind != "" {
		var injected error
		switch errKind {
		case "forbidden":
			injected = apierrors.NewForbidden(
				schema.GroupResource{Group: "hypershift.openshift.io", Resource: "hostedcontrolplanes"}, "", errors.New("denied"))
		case "nomatch":
			injected = &apimeta.NoKindMatchError{
				GroupKind: schema.GroupKind{Group: "hypershift.openshift.io", Kind: "HostedControlPlane"},
			}
		default:
			t.Fatalf("unknown errKind %q", errKind)
		}
		b = b.WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, cl client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				if ul, ok := list.(*unstructured.UnstructuredList); ok && ul.GetKind() == "HostedControlPlaneList" {
					return injected
				}
				return cl.List(ctx, list, opts...)
			},
		})
	}
	return b.Build()
}

// TestCollectClusterTopology_HCPSnapshotHappyPath proves the full client path:
// unstructured parsing, ns UID/creationTime capture, and cross-check wiring.
func TestCollectClusterTopology_HCPSnapshotHappyPath(t *testing.T) {
	got := collectClusterTopology(context.Background(), hcpCollectFixture(t, ""))
	if len(got.HCPSnapshot) != 1 {
		t.Fatalf("HCPSnapshot length = %d, want 1", len(got.HCPSnapshot))
	}
	assert_HCPSnapshotComplete(t, got.HCPSnapshot[0], "hc01-infra-hc01", "d5d3-1", "uid-hc-1", "uid-hcp-1")
	if got.CollectionError != "" {
		t.Errorf("CollectionError = %q, want empty", got.CollectionError)
	}
	if !reflect.DeepEqual(got.HostedControlPlaneNamespaces, []string{"hc01-infra-hc01"}) {
		t.Errorf("HostedControlPlaneNamespaces = %v", got.HostedControlPlaneNamespaces)
	}
	if got.HostedClusterCount != 1 {
		t.Errorf("HostedClusterCount = %d, want 1", got.HostedClusterCount)
	}
}

func assert_HCPSnapshotComplete(t *testing.T, e metricscfgv1beta1.HCPSnapshotEntry, ns, id, hcUID, hcpUID string) {
	t.Helper()
	if e.HCPNamespace != ns || e.HostedClusterID != id || e.HcUID != hcUID || e.HcpUID != hcpUID {
		t.Errorf("identity = (%q,%q,%q,%q), want (%q,%q,%q,%q)", e.HCPNamespace, e.HostedClusterID, e.HcUID, e.HcpUID, ns, id, hcUID, hcpUID)
	}
	if e.NamespaceUID != "uid-ns-"+ns || e.NamespaceCreatedAt != "2026-09-26T19:10:57Z" {
		t.Errorf("ns identity = (%q,%q)", e.NamespaceUID, e.NamespaceCreatedAt)
	}
	if e.ObservedAt == "" || !e.Complete || e.Diagnostics != "" {
		t.Errorf("expected complete entry without diagnostics, got observed=%q complete=%v diag=%q", e.ObservedAt, e.Complete, e.Diagnostics)
	}
}

// TestCollectClusterTopology_HCPListForbidden proves RBAC denial degrades to
// an incomplete entry naming the cause while every other fact survives.
func TestCollectClusterTopology_HCPListForbidden(t *testing.T) {
	got := collectClusterTopology(context.Background(), hcpCollectFixture(t, "forbidden"))
	if !reflect.DeepEqual(got.HostedControlPlaneNamespaces, []string{"hc01-infra-hc01"}) {
		t.Errorf("namespace list survives HCP denial: %v", got.HostedControlPlaneNamespaces)
	}
	if got.HostedClusterCount != 1 {
		t.Errorf("HC count survives HCP denial: %d", got.HostedClusterCount)
	}
	if len(got.HCPSnapshot) != 1 {
		t.Fatalf("HCPSnapshot length = %d, want 1", len(got.HCPSnapshot))
	}
	e := got.HCPSnapshot[0]
	if e.HCPNamespace != "hc01-infra-hc01" || e.Complete || e.HostedClusterID != "" {
		t.Errorf("denied entry = %+v, want incomplete without ID", e)
	}
	if !strings.Contains(e.Diagnostics, "hostedcontrolplanes") {
		t.Errorf("diagnostic %q must name the denied read", e.Diagnostics)
	}
	if e.NamespaceUID != "uid-ns-hc01-infra-hc01" {
		t.Errorf("ns identity lost on denial: %+v", e)
	}
	if !strings.Contains(got.CollectionError, "hostedcontrolplanes") {
		t.Errorf("CollectionError %q must name the denied read", got.CollectionError)
	}
}

// TestCollectClusterTopology_HCPAPIAbsent proves pre-HyperShift clusters get
// fail-closed incomplete entries (not silent emptiness) when labeled
// namespaces exist without the HCP API.
func TestCollectClusterTopology_HCPAPIAbsent(t *testing.T) {
	got := collectClusterTopology(context.Background(), hcpCollectFixture(t, "nomatch"))
	if len(got.HCPSnapshot) != 1 || got.HCPSnapshot[0].Complete {
		t.Errorf("absent API must yield one incomplete entry, got %+v", got.HCPSnapshot)
	}
	if !strings.Contains(got.HCPSnapshot[0].Diagnostics, "found 0") {
		t.Errorf("diagnostic %q must report zero objects", got.HCPSnapshot[0].Diagnostics)
	}
	if got.CollectionError != "" {
		t.Errorf("missing API is absence, not an error: %q", got.CollectionError)
	}
}
