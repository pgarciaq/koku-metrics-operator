//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import (
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
)

const (
	kubevirtAPIGroup   = "kubevirt.io"
	kubevirtAPIVersion = "v1"
	kubevirtVMIResource = "virtualmachineinstances"
)

// kubeVirtCRDChecker gates VM metrics collection. Tests may replace this variable.
var kubeVirtCRDChecker = IsKubeVirtCRDAvailable

func shouldCollectVMMetrics(c *PrometheusCollector) bool {
	if c == nil {
		return false
	}
	if c.RestConfig == nil {
		return false
	}
	return kubeVirtCRDChecker(c.RestConfig)
}

// IsKubeVirtCRDAvailable reports whether the KubeVirt API is registered on the cluster.
func IsKubeVirtCRDAvailable(config *rest.Config) bool {
	if config == nil {
		return false
	}
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return false
	}

	resources, err := discoveryClient.ServerResourcesForGroupVersion(kubevirtAPIGroup + "/" + kubevirtAPIVersion)
	if err != nil {
		return false
	}

	for _, r := range resources.APIResources {
		if r.Name == kubevirtVMIResource {
			return true
		}
	}
	return false
}
