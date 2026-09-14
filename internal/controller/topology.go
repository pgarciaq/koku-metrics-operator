//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package controller

import (
	"context"
	"fmt"
	"sort"

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

// listHostedClusterNames returns HostedCluster names across all namespaces.
// Clusters without the HyperShift CRDs yield an empty list and no error.
func listHostedClusterNames(ctx context.Context, c client.Client) ([]string, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(hostedClusterListGVK)
	if err := c.List(ctx, list); err != nil {
		if isMissingAPIError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list hostedclusters: %w", err)
	}
	names := make([]string, 0, len(list.Items))
	for i := range list.Items {
		names = append(names, list.Items[i].GetName())
	}
	return names, nil
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

	hcNames, err := listHostedClusterNames(ctx, c)
	if err != nil && collectionError == "" {
		collectionError = fmt.Sprintf("hostedclusters.hypershift.openshift.io: %v", err)
	}

	var hcpNamespaces []string
	nsList := &corev1.NamespaceList{}
	if err := c.List(ctx, nsList, client.MatchingLabels{hcpNamespaceLabel: labelTrueValue}); err != nil {
		if collectionError == "" {
			collectionError = fmt.Sprintf("namespaces: %v", err)
		}
	} else {
		for i := range nsList.Items {
			hcpNamespaces = append(hcpNamespaces, nsList.Items[i].Name)
		}
		sort.Strings(hcpNamespaces)
	}

	status := buildTopologyStatus(infra, nodeList.Items, len(hcNames), hcpNamespaces)
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
