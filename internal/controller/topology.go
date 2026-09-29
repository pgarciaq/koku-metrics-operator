//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package controller

import (
	"context"
	"fmt"
	"sort"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	metricscfgv1beta1 "github.com/project-koku/koku-metrics-operator/api/v1beta1"
)

const (
	hypershiftManagedLabel = "hypershift.openshift.io/managed"
	hcpNamespaceLabel      = "hypershift.openshift.io/hosted-control-plane"
	labelTrueValue         = "true"

	masterRoleLabel       = "node-role.kubernetes.io/master"
	controlPlaneRoleLabel = "node-role.kubernetes.io/control-plane"
	workerRoleLabel       = "node-role.kubernetes.io/worker"

	infrastructureSingletonName = "cluster"
)

// hostedClusterListGVK identifies HostedCluster objects without vendoring the
// HyperShift API. Clusters without those CRDs fail the list with a no-match
// error, which callers treat as "zero hosted clusters".
var hostedClusterListGVK = schema.GroupVersionKind{
	Group:   "hypershift.openshift.io",
	Version: "v1beta1",
	Kind:    "HostedClusterList",
}

// hostedControlPlaneListGVK identifies HostedControlPlane objects without
// vendoring the HyperShift API (same shapeless pattern as HostedClusters).
var hostedControlPlaneListGVK = schema.GroupVersionKind{
	Group:   "hypershift.openshift.io",
	Version: "v1beta1",
	Kind:    "HostedControlPlaneList",
}

// buildTopologyStatus folds already-fetched cluster facts into status.
// It is pure (no API calls): nil infrastructure yields zero topology facts,
// dual-labeled (compact) nodes increment both role counters by design.
func buildTopologyStatus(infra *configv1.Infrastructure, nodes []corev1.Node, hostedClusterCount int, hcpNamespaces []string) metricscfgv1beta1.ClusterTopologyStatus {
	status := metricscfgv1beta1.ClusterTopologyStatus{
		HostedClusterCount:           int32(hostedClusterCount),
		HostedControlPlaneNamespaces: append([]string(nil), hcpNamespaces...),
	}
	if infra != nil {
		status.ControlPlaneTopology = string(infra.Status.ControlPlaneTopology)
		status.ManagedByHypershift = infra.Labels[hypershiftManagedLabel] == labelTrueValue
	}
	for i := range nodes {
		labels := nodes[i].Labels
		if _, ok := labels[masterRoleLabel]; ok {
			status.MasterNodes++
		} else if _, ok := labels[controlPlaneRoleLabel]; ok {
			status.MasterNodes++
		}
		if _, ok := labels[workerRoleLabel]; ok {
			status.WorkerNodes++
		}
	}
	return status
}

// isMissingAPIError reports whether err means the API itself is absent
// (unknown kind or missing resource) as opposed to a real failure.
func isMissingAPIError(err error) bool {
	return apimeta.IsNoMatchError(err) || apierrors.IsNotFound(err)
}

// hcpObserved, hcObserved, and nsObserved are client-free snapshots of the
// objects buildHCPSnapshot reasons over: plain scalars so the mapping rule
// stays unit-testable without a fake API server.
type hcpObserved struct {
	Namespace string
	Name      string
	UID       string
	ClusterID string
}

type hcObserved struct {
	Namespace string
	Name      string
	UID       string
	ClusterID string
}

type nsObserved struct {
	UID       string
	CreatedAt string
}

// buildHCPSnapshot maps each labeled HCP namespace to its hosted incarnation
// (#633). Output is sorted by namespace for deterministic manifests. Every
// ambiguity fails closed to an incomplete entry naming the cause: the
// namespace count or HCP object count differing from one, an empty
// spec.clusterID, or a live HostedCluster count for that ID differing from
// one. Name patterns, infraID, and infrastructureName are never identity.
func buildHCPSnapshot(hcps []hcpObserved, hcs []hcObserved, nsInfos map[string]nsObserved, labeledNS []string, observedAt string) []metricscfgv1beta1.HCPSnapshotEntry {
	byNS := make(map[string][]hcpObserved)
	for _, h := range hcps {
		byNS[h.Namespace] = append(byNS[h.Namespace], h)
	}
	byClusterID := make(map[string][]hcObserved)
	for _, h := range hcs {
		if h.ClusterID == "" {
			continue
		}
		byClusterID[h.ClusterID] = append(byClusterID[h.ClusterID], h)
	}
	sorted := append([]string(nil), labeledNS...)
	sort.Strings(sorted)
	out := make([]metricscfgv1beta1.HCPSnapshotEntry, 0, len(sorted))
	for _, ns := range sorted {
		entry := metricscfgv1beta1.HCPSnapshotEntry{HCPNamespace: ns, ObservedAt: observedAt}
		info, ok := nsInfos[ns]
		if !ok {
			entry.Diagnostics = "namespace vanished between list and snapshot"
			out = append(out, entry)
			continue
		}
		entry.NamespaceUID = info.UID
		entry.NamespaceCreatedAt = info.CreatedAt
		inNS := byNS[ns]
		if len(inNS) != 1 {
			entry.Diagnostics = fmt.Sprintf("expected 1 HostedControlPlane in namespace, found %d", len(inNS))
			out = append(out, entry)
			continue
		}
		if inNS[0].ClusterID == "" {
			entry.Diagnostics = fmt.Sprintf("HostedControlPlane %s has empty spec.clusterID", inNS[0].Name)
			out = append(out, entry)
			continue
		}
		matches := byClusterID[inNS[0].ClusterID]
		if len(matches) != 1 {
			entry.Diagnostics = fmt.Sprintf("expected 1 live HostedCluster for clusterID, found %d", len(matches))
			out = append(out, entry)
			continue
		}
		entry.HostedClusterID = inNS[0].ClusterID
		entry.HcUID = matches[0].UID
		entry.HcpUID = inNS[0].UID
		entry.Complete = true
		out = append(out, entry)
	}
	return out
}

// listHCPObserved lists HostedControlPlane objects (namespace, name, UID,
// spec.clusterID). Missing APIs yield empty + nil (pre-HyperShift); any other
// error propagates for the caller to degrade.
func listHCPObserved(ctx context.Context, c client.Client) ([]hcpObserved, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(hostedControlPlaneListGVK)
	if err := c.List(ctx, list); err != nil {
		if isMissingAPIError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list hostedcontrolplanes: %w", err)
	}
	out := make([]hcpObserved, 0, len(list.Items))
	for i := range list.Items {
		obj := &list.Items[i]
		clusterID, _, _ := unstructured.NestedString(obj.Object, "spec", "clusterID")
		out = append(out, hcpObserved{
			Namespace: obj.GetNamespace(),
			Name:      obj.GetName(),
			UID:       string(obj.GetUID()),
			ClusterID: clusterID,
		})
	}
	return out, nil
}

// listHCObserved lists HostedCluster objects (namespace, name, UID,
// spec.clusterID) with the same missing-API posture as listHCPObserved.
func listHCObserved(ctx context.Context, c client.Client) ([]hcObserved, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(hostedClusterListGVK)
	if err := c.List(ctx, list); err != nil {
		if isMissingAPIError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list hostedclusters: %w", err)
	}
	out := make([]hcObserved, 0, len(list.Items))
	for i := range list.Items {
		obj := &list.Items[i]
		clusterID, _, _ := unstructured.NestedString(obj.Object, "spec", "clusterID")
		out = append(out, hcObserved{
			Namespace: obj.GetNamespace(),
			Name:      obj.GetName(),
			UID:       string(obj.GetUID()),
			ClusterID: clusterID,
		})
	}
	return out, nil
}

// collectClusterTopology reads cluster topology facts with single LISTs (no
// per-object GETs). It never fails: unreadable APIs degrade to empty facts
// plus CollectionError so packaging is never blocked on topology.
func collectClusterTopology(ctx context.Context, c client.Client) metricscfgv1beta1.ClusterTopologyStatus {
	var collectionError string

	infra := &configv1.Infrastructure{}
	if err := c.Get(ctx, client.ObjectKey{Name: infrastructureSingletonName}, infra); err != nil {
		collectionError = fmt.Sprintf("infrastructures.config.openshift.io: %v", err)
		infra = nil
	}

	nodeList := &corev1.NodeList{}
	if err := c.List(ctx, nodeList); err != nil && collectionError == "" {
		collectionError = fmt.Sprintf("nodes: %v", err)
	}

	hcObjs, err := listHCObserved(ctx, c)
	if err != nil && collectionError == "" {
		collectionError = fmt.Sprintf("hostedclusters.hypershift.openshift.io: %v", err)
	}

	var hcpNamespaces []string
	nsInfos := make(map[string]nsObserved)
	nsList := &corev1.NamespaceList{}
	if err := c.List(ctx, nsList, client.MatchingLabels{hcpNamespaceLabel: labelTrueValue}); err != nil {
		if collectionError == "" {
			collectionError = fmt.Sprintf("namespaces: %v", err)
		}
	} else {
		for i := range nsList.Items {
			hcpNamespaces = append(hcpNamespaces, nsList.Items[i].Name)
			nsInfos[nsList.Items[i].Name] = nsObserved{
				UID:       string(nsList.Items[i].UID),
				CreatedAt: nsList.Items[i].CreationTimestamp.UTC().Format(time.RFC3339),
			}
		}
		sort.Strings(hcpNamespaces)
	}

	// Snapshot emission (#633): same never-fail posture. A missing HCP API
	// (pre-HyperShift) means no snapshot; any other LIST error degrades each
	// labeled namespace to an incomplete entry naming the cause.
	var snapshot []metricscfgv1beta1.HCPSnapshotEntry
	if len(hcpNamespaces) > 0 {
		hcpObjs, hcpErr := listHCPObserved(ctx, c)
		if hcpErr != nil {
			if isMissingAPIError(hcpErr) {
				hcpObjs = nil
			} else {
				if collectionError == "" {
					collectionError = fmt.Sprintf("hostedcontrolplanes.hypershift.openshift.io: %v", hcpErr)
				}
				for _, ns := range hcpNamespaces {
					info := nsInfos[ns]
					snapshot = append(snapshot, metricscfgv1beta1.HCPSnapshotEntry{
						HCPNamespace:       ns,
						NamespaceUID:       info.UID,
						NamespaceCreatedAt: info.CreatedAt,
						ObservedAt:         time.Now().UTC().Format(time.RFC3339),
						Diagnostics:        fmt.Sprintf("HostedControlPlane list failed: %v", hcpErr),
					})
				}
			}
		}
		if snapshot == nil && hcpErr == nil {
			snapshot = buildHCPSnapshot(hcpObjs, hcObjs, nsInfos, hcpNamespaces, time.Now().UTC().Format(time.RFC3339))
		}
	}

	status := buildTopologyStatus(infra, nodeList.Items, len(hcObjs), hcpNamespaces)
	status.HCPSnapshot = snapshot
	status.CollectionError = collectionError
	return status
}

// setClusterTopology records observed cluster topology facts on the CR status
// for HCP/fleet classification (W0, #406). Best-effort by design.
func setClusterTopology(ctx context.Context, c client.Client, cr *metricscfgv1beta1.MetricsConfig) {
	cr.Status.Topology = collectClusterTopology(ctx, c)
	if cr.Status.Topology.CollectionError != "" {
		log.WithName("setClusterTopology").Info("topology collection degraded", "error", cr.Status.Topology.CollectionError)
	}
}
