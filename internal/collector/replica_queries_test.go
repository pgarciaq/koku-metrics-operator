//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import (
	"strings"
	"testing"
)

func TestDesiredReplicasQueryContainsDeploymentConfig(t *testing.T) {
	q, ok := QueryMap["ros:desired_replicas"]
	if !ok {
		t.Fatal("QueryMap missing ros:desired_replicas")
	}
	expected := "openshift_apps_deploymentconfig_replicas"
	count := strings.Count(q, expected)
	if count != 2 {
		t.Errorf("ros:desired_replicas should contain %q exactly 2 times (one per label variant), got %d", expected, count)
	}
}

func TestAvailableReplicasQueryContainsDeploymentConfig(t *testing.T) {
	q, ok := QueryMap["ros:available_replicas"]
	if !ok {
		t.Fatal("QueryMap missing ros:available_replicas")
	}
	expected := "openshift_apps_deploymentconfig_status_available_replicas"
	count := strings.Count(q, expected)
	if count != 2 {
		t.Errorf("ros:available_replicas should contain %q exactly 2 times (one per label variant), got %d", expected, count)
	}
}

func TestReplicaQueriesContainAllWorkloadTypes(t *testing.T) {
	desiredMetrics := []string{
		"kube_deployment_spec_replicas",
		"kube_statefulset_replicas",
		"kube_daemonset_status_desired_number_scheduled",
		"openshift_apps_deploymentconfig_replicas",
	}
	availableMetrics := []string{
		"kube_deployment_status_replicas_available",
		"kube_statefulset_status_replicas_ready",
		"kube_daemonset_status_number_available",
		"openshift_apps_deploymentconfig_status_available_replicas",
	}

	desiredQuery := QueryMap["ros:desired_replicas"]
	for _, metric := range desiredMetrics {
		if !strings.Contains(desiredQuery, metric) {
			t.Errorf("ros:desired_replicas missing metric %q", metric)
		}
	}

	availableQuery := QueryMap["ros:available_replicas"]
	for _, metric := range availableMetrics {
		if !strings.Contains(availableQuery, metric) {
			t.Errorf("ros:available_replicas missing metric %q", metric)
		}
	}
}
