//
// Copyright 2026 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package collector

import (
	"strings"
	"time"

	"github.com/go-logr/logr"
)

// vmNameForStoragePod resolves the KubeVirt VM name for a storage CSV row from the mounting pod.
// Primary: kube_pod_labels label vm.kubevirt.io/name (podVMINames from Prometheus).
// Fallback: parse virt-launcher-<vmi>-<hash> when kube-state-metrics labels are unavailable.
// Returns empty for non-virt-launcher pods.
func vmNameForStoragePod(pod, namespace string, podVMINames map[string]string) string {
	if pod == "" || !strings.HasPrefix(pod, "virt-launcher-") {
		return ""
	}
	if podVMINames != nil && namespace != "" {
		if name := podVMINames[namespace+"\x00"+pod]; name != "" {
			return name
		}
	}
	return vmiNameFromVirtLauncherPod(pod)
}

// applyVMNameToStorageRows sets VMName on storage rows when the mounting pod is a virt-launcher.
func applyVMNameToStorageRows(volRows mappedCSVStruct, podVMINames map[string]string) {
	for _, row := range volRows {
		sr, ok := row.(*storageRow)
		if !ok {
			continue
		}
		sr.VMName = vmNameForStoragePod(sr.Pod, sr.Namespace, podVMINames)
	}
}

func collectPodVMINamesForStorage(c *PrometheusCollector, log logr.Logger) map[string]string {
	at := c.TimeSeries.End
	if at.IsZero() {
		at = time.Now().UTC()
	}
	podVMINames, err := fetchPodVMINameMap(c, at)
	if err != nil {
		log.Error(err, "failed to fetch virt-launcher pod to VMI name map for storage report")
		return nil
	}
	return podVMINames
}
