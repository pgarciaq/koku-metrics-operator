#!/usr/bin/env bash

# Copyright 2026 Red Hat Inc.
# SPDX-License-Identifier: Apache-2.0
#
# verify-install.selftest.sh - regression test for the denial classifier in
# hack/verify-install.sh.
#
# Why this exists
# ---------------
# The classifier decides whether a `forbidden` line means "this install is
# stalled" (hard failure) or "a collector degraded, the reconcile is fine"
# (note). Getting it backwards has two equally silent failure modes, both of
# which have been observed:
#
#   * every denial treated as fatal -> the check is red on healthy clusters for
#     as long as the log window covers the last packaging cycle, and it points
#     at hack/verify-bundle-drift.sh, which is green;
#   * every denial tolerated -> a stalled install (empty .status, nothing
#     uploaded) passes the very check that exists to catch it.
#
# Neither shows up in review, so the split is pinned here. Each case runs the
# shipping script through --classify-log; nothing is re-implemented.
#
# Usage: hack/verify-install.selftest.sh   (no cluster, no oc, no jq needed)
# Exit:  0 all cases pass, 1 a case regressed, 2 the script is missing.

set -o errexit
set -o nounset
set -o pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VI="${HERE}/verify-install.sh"
[[ -x "$VI" ]] || {
	echo "selftest: $VI is missing or not executable" >&2
	exit 2
}

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

NS="koku-metrics-operator"
SA="system:serviceaccount:${NS}:koku-metrics-controller-manager"

# Real log format: <RFC3339Nano>\t<LEVEL>\t<logger>\t<message>
emit() {
	local level="$1" logger="$2" message="$3"
	printf '2026-10-31T19:03:44.078611163Z\t%s\t%s\t%s\n' "$level" "$logger" "$message"
}

stall_infra="failed to list *v1.Infrastructure: infrastructures.config.openshift.io is forbidden: User \"${SA}\" cannot list resource \"infrastructures\" in API group \"config.openshift.io\" at the cluster scope"
stall_nodes="nodes is forbidden: User \"${SA}\" cannot list resource \"nodes\" in API group \"\" at the cluster scope"
besteffort_machines="failed to list Machines for machineset_name mapping\t{\"error\": \"machines.machine.openshift.io is forbidden: User \\\"${SA}\\\" cannot list resource \\\"machines\\\" in API group \\\"machine.openshift.io\\\" at the cluster scope\"}"
stall_named_collector="failed to start collector for metric collection: is forbidden: User \"${SA}\" cannot list resource \"pods\" in API group \"\" at the cluster scope"

run_case() {
	local name="$1" want="$2" file="$3" got
	if ! got=$("$VI" --classify-log "$file" 2>&1); then
		echo "FAIL: ${name}: classifier exited non-zero" >&2
		echo "  output: ${got}" >&2
		exit 1
	fi
	if [[ "$got" != "$want" ]]; then
		echo "FAIL: ${name}" >&2
		echo "  want: ${want}" >&2
		echo "  got:  ${got}" >&2
		echo "  (a denial from collector.* is best-effort; anything else is a reconcile stall)" >&2
		exit 1
	fi
	echo "ok: ${name} -> ${got}"
}

# 1. Reconcile-path denial: the stall that leaves .status empty.
{
	emit ERROR controller.metricsconfig_controller "$stall_infra"
} >"$TMP/stall-infra.log"
run_case "reconcile-path denial (infrastructures)" "stall=1 besteffort=0" "$TMP/stall-infra.log"

# 2. The live tolerated case: machines.machine.openshift.io, once per cycle.
{
	emit ERROR collector.GenerateReports "$besteffort_machines"
} >"$TMP/besteffort-machines.log"
run_case "collector denial (machines, best-effort)" "stall=0 besteffort=1" "$TMP/besteffort-machines.log"

# 3. Both kinds in one window: still a stall.
{
	emit ERROR collector.GenerateReports "$besteffort_machines"
	emit ERROR controller.metricsconfig_controller "$stall_infra"
} >"$TMP/mixed.log"
run_case "mixed window" "stall=1 besteffort=1" "$TMP/mixed.log"

# 4. A healthy log with no denials must not be read as anything.
{
	emit INFO metricsconfig_controller.checkCycle "not time to execute the upload"
	emit INFO collector.GenerateReports "generated 4 reports"
} >"$TMP/clean.log"
run_case "no denials" "stall=0 besteffort=0" "$TMP/clean.log"

# 5. The second regex branch: a denial without the service-account phrasing.
{
	emit ERROR controller.metricsconfig_controller "$stall_nodes"
} >"$TMP/stall-nodes.log"
run_case "reconcile-path denial (bare 'cannot list resource')" "stall=1 besteffort=0" "$TMP/stall-nodes.log"

# 6. Repeated cycles: the count has to keep up, or a repeated denial looks new.
{
	emit ERROR collector.GenerateReports "$besteffort_machines"
	emit ERROR collector.GenerateReports "$besteffort_machines"
} >"$TMP/besteffort-twice.log"
run_case "collector denial repeated" "stall=0 besteffort=2" "$TMP/besteffort-twice.log"

# 7. The word "collector" in the message must not buy tolerance: only the
#    logger field decides. A substring match would silently tolerate this.
{
	emit ERROR controller.metricsconfig_controller "$stall_named_collector"
} >"$TMP/stall-mentions-collector.log"
run_case "reconcile denial whose message says 'collector'" "stall=1 besteffort=0" "$TMP/stall-mentions-collector.log"

echo
echo "selftest PASS: denial classification (stall vs best-effort) behaves as documented."
