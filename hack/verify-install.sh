#!/usr/bin/env bash

# Copyright 2026 Red Hat Inc.
# SPDX-License-Identifier: Apache-2.0
#
# verify-install.sh - assert that a Koku Metrics Operator install actually WORKS.
#
# Why this exists
# ---------------
# A broken install of this operator is indistinguishable from a healthy one by
# the signals people normally check: the CSV reports Succeeded, the pod is
# Running with zero restarts, and nothing crash-loops. What actually happens is
# that every reconcile is denied a cluster-scoped read, so the CR status stays
# EMPTY and the only trace is `forbidden` in the operator log.
#
# That was observed twice (2026-10-09 and 2026-10-31) with branch-built images
# installed under a checked-in bundle that lagged config/ (see
# hack/verify-bundle-drift.sh). Both times the install was reported healthy and
# produced no data.
#
# Hard assertions (satisfied by the first successful reconcile)
#   * OLM-managed: the CSV is Succeeded (skipped with --no-olm / non-OLM install)
#   * the controller Deployment has a ready replica
#   * no authorization failure in the reconcile path (see "Best-effort denials")
#   * status.prometheus.prometheus_configured and .prometheus_connected are true
#   * status.source.source_defined is true
#   * status.topology.controlPlaneTopology is non-empty
#
# Best-effort denials (reported as a note, never a failure)
#   Denials logged by a collector.* component are enrichment, not reconcile: the
#   collector degrades to an empty result and the cycle completes. The
#   machine.openshift.io/machines lookup is the live example - without that
#   grant the CSV's machineset_name column is empty on every cycle, which is a
#   data gap to fix on its own merits, not evidence of a broken install. Counting
#   those as failures made this script red on healthy clusters for as long as the
#   log window covered the last cycle, pointing at the wrong remedy.
#
# Soft assertions (reported; hard only with --require-upload)
#   * status.upload.last_successful_upload_time is set
#   * status.packaging.number_reports_stored > 0
#   * status.authentication.credentials_found is true
#
# The upload assertion is soft by default on purpose: it needs a completed
# metrics query + packaging + upload cycle, which a fresh install can reach well
# after the first reconcile. Two installs that both eventually upload are not
# told apart by waiting longer; an install that cannot read the cluster never
# uploads at all, and the hard assertions above already catch that.
#
# Usage
#   hack/verify-install.sh [options]
#
# Options
#   -n, --namespace NS     operator namespace              (default: koku-metrics-operator)
#   -c, --cr NAME          MetricsConfig CR name           (default: costmanagementmetricscfg)
#   -t, --timeout SECS     wait budget                     (default: 360)
#   -i, --interval SECS    poll interval                   (default: 15)
#   -s, --since WINDOW     log window for the forbidden scan (default: 30m)
#       --require-upload   make a successful upload a hard assertion
#       --no-olm           skip the CSV check (non-OLM / `make deploy` installs)
#       --no-wait          evaluate once, do not poll
#       --context NAME     kubectl/oc context
#       --classify-log FILE
#                          internal: classify the lines in FILE and exit
#                          (used by hack/verify-install.selftest.sh, needs no cluster)
#   -v, --verbose          print every poll
#   -h, --help
#
# Env: KMO_NAMESPACE, KMO_CR_NAME, KMO_TIMEOUT, KMO_INTERVAL, KMO_SINCE
#
# Exit: 0 healthy, 1 assertion failed, 2 preflight/usage error.

set -o errexit
set -o nounset
set -o pipefail

NS="${KMO_NAMESPACE:-koku-metrics-operator}"
CR_NAME="${KMO_CR_NAME:-costmanagementmetricscfg}"
TIMEOUT="${KMO_TIMEOUT:-360}"
INTERVAL="${KMO_INTERVAL:-15}"
SINCE="${KMO_SINCE:-30m}"
REQUIRE_UPLOAD=false
CHECK_OLM=true
WAIT=true
VERBOSE=false
CONTEXT=""
CLASSIFY_LOG=""

# Prints this file's header comment block. Bounded by the first statement rather
# than by line numbers, so editing the header cannot silently truncate --help.
usage() {
	awk 'NR > 1 && /^set -o /{exit} NR > 1 {sub(/^# ?/, ""); print}' "${BASH_SOURCE[0]}"
}

while [[ $# -gt 0 ]]; do
	case "$1" in
	-n | --namespace)
		NS="$2"
		shift 2
		;;
	-c | --cr)
		CR_NAME="$2"
		shift 2
		;;
	-t | --timeout)
		TIMEOUT="$2"
		shift 2
		;;
	-i | --interval)
		INTERVAL="$2"
		shift 2
		;;
	-s | --since)
		SINCE="$2"
		shift 2
		;;
	--require-upload)
		REQUIRE_UPLOAD=true
		shift
		;;
	--no-olm)
		CHECK_OLM=false
		shift
		;;
	--no-wait)
		WAIT=false
		shift
		;;
	--context)
		CONTEXT="$2"
		shift 2
		;;
	--classify-log)
		CLASSIFY_LOG="$2"
		shift 2
		;;
	-v | --verbose)
		VERBOSE=true
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		echo "verify-install: unknown argument: $1" >&2
		exit 2
		;;
	esac
done

OC=(oc)
if [[ -n "$CONTEXT" ]]; then
	OC+=(--context "$CONTEXT")
fi

fail() {
	echo "FAIL: $*" >&2
	exit 1
}

preflight_fail() {
	echo "verify-install: $*" >&2
	exit 2
}

info() {
	echo "  $*"
}

# --------------------------------------------------------------------------
# authorization failures: stall vs best-effort
# --------------------------------------------------------------------------
# A denial is logged as "<ts>\t<LEVEL>\t<logger>\t<msg>...". The logger field is
# what separates the two kinds:
#
#   * collector.*               best-effort enrichment - degrade to empty, cycle
#                              completes (machineset_name, for example)
#   * anything else             the reconcile/status path - the operator is
#                              stalled, which is what leaves .status empty and
#                              uploads nothing
DENIAL_RE='is forbidden: User "system:serviceaccount:|cannot (list|get|watch) resource'
BESTEFFORT_LOGGER_RE='^collector[.]'

# classify_denials LOG_TEXT -> STALL_DENIAL_COUNT, BESTEFFORT_DENIAL_COUNT (+ samples)
classify_denials() {
	local denials
	denials=$(printf '%s\n' "$1" | grep -E "$DENIAL_RE" || true)
	STALL_DENIALS=$(printf '%s\n' "$denials" | awk -F'\t' -v re="$BESTEFFORT_LOGGER_RE" '$3 !~ re' || true)
	BESTEFFORT_DENIALS=$(printf '%s\n' "$denials" | awk -F'\t' -v re="$BESTEFFORT_LOGGER_RE" '$3 ~ re' || true)
	STALL_DENIAL_COUNT=$(printf '%s\n' "$STALL_DENIALS" | grep -c . || true)
	BESTEFFORT_DENIAL_COUNT=$(printf '%s\n' "$BESTEFFORT_DENIALS" | grep -c . || true)
	STALL_DENIAL_COUNT="${STALL_DENIAL_COUNT:-0}"
	BESTEFFORT_DENIAL_COUNT="${BESTEFFORT_DENIAL_COUNT:-0}"
	STALL_DENIAL_SAMPLE=$(printf '%s\n' "$STALL_DENIALS" | head -3 || true)
	BESTEFFORT_DENIAL_SAMPLE=$(printf '%s\n' "$BESTEFFORT_DENIALS" | head -3 || true)
}

# Test hook: classify a captured log file without touching a cluster.
if [[ -n "$CLASSIFY_LOG" ]]; then
	[[ -r "$CLASSIFY_LOG" ]] || preflight_fail "cannot read $CLASSIFY_LOG"
	classify_denials "$(cat "$CLASSIFY_LOG")"
	echo "stall=${STALL_DENIAL_COUNT} besteffort=${BESTEFFORT_DENIAL_COUNT}"
	exit 0
fi

# --------------------------------------------------------------------------
# preflight
# --------------------------------------------------------------------------
for tool in oc jq; do
	command -v "$tool" >/dev/null 2>&1 || preflight_fail "$tool is required"
done

if ! "${OC[@]}" whoami >/dev/null 2>&1; then
	preflight_fail "not logged in (oc whoami failed). Log in, or attach SSH/VPN to the cluster first."
fi

"${OC[@]}" get namespace "$NS" >/dev/null 2>&1 ||
	preflight_fail "namespace $NS does not exist in $("${OC[@]}" config current-context 2>/dev/null)"

echo "verify-install: namespace=$NS cr=$CR_NAME context=$("${OC[@]}" config current-context 2>/dev/null)"

# --------------------------------------------------------------------------
# OLM: CSV must be Succeeded
# --------------------------------------------------------------------------
CSV_PHASE=""
if [[ "$CHECK_OLM" == true ]]; then
	csv_json=$("${OC[@]}" get csv -n "$NS" -o json 2>/dev/null || echo '{"items":[]}')
	csv_name=$(echo "$csv_json" | jq -r '
		.items[] | select(.metadata.name | startswith("koku-metrics-operator"))
		| .metadata.name' | head -1)

	if [[ -z "$csv_name" ]]; then
		info "CSV: none found in $NS (non-OLM install?) - continuing, use --no-olm to silence"
	else
		CSV_PHASE=$(echo "$csv_json" | jq -r --arg n "$csv_name" '
			.items[] | select(.metadata.name == $n) | .status.phase // "None"')
		if [[ "$CSV_PHASE" != "Succeeded" ]]; then
			echo "$csv_json" | jq -r --arg n "$csv_name" '
				.items[] | select(.metadata.name == $n)
				| "  reason: \(.status.reason // "-")", "  message: \(.status.message // "-")"' || true
			fail "CSV $csv_name is $CSV_PHASE, not Succeeded"
		fi
		info "CSV: $csv_name Succeeded"
	fi
fi

# --------------------------------------------------------------------------
# controller Deployment
# --------------------------------------------------------------------------
deploy_json=$("${OC[@]}" get deploy -n "$NS" -l control-plane=controller-manager -o json 2>/dev/null || echo '{"items":[]}')
DEPLOY=$(echo "$deploy_json" | jq -r '.items[0].metadata.name // ""')
if [[ -z "$DEPLOY" ]]; then
	deploy_json=$("${OC[@]}" get deploy -n "$NS" -o json)
	DEPLOY=$(echo "$deploy_json" | jq -r '
		.items[] | select(.metadata.name | test("koku-metrics|controller-manager"))
		| .metadata.name' | head -1)
fi
[[ -n "$DEPLOY" ]] || fail "no controller Deployment found in $NS"

DEPLOY_IMAGE=$(echo "$deploy_json" | jq -r --arg d "$DEPLOY" '
	.items[] | select(.metadata.name == $d) | .spec.template.spec.containers[0].image // "-"')
DEPLOY_READY=$(echo "$deploy_json" | jq -r --arg d "$DEPLOY" '
	.items[] | select(.metadata.name == $d) | .status.readyReplicas // 0')
info "deployment: $DEPLOY ready=$DEPLOY_READY"
info "image: $DEPLOY_IMAGE"
[[ "$DEPLOY_READY" -ge 1 ]] || fail "deployment $DEPLOY has no ready replica"

# --------------------------------------------------------------------------
# authorization failures: scan the operator log
# --------------------------------------------------------------------------
scan_forbidden() {
	local logs
	logs=$("${OC[@]}" logs -n "$NS" deploy/"$DEPLOY" --since="$SINCE" 2>/dev/null || true)
	classify_denials "$logs"
}

STALL_DENIAL_COUNT=0
BESTEFFORT_DENIAL_COUNT=0
STALL_DENIAL_SAMPLE=""
BESTEFFORT_DENIAL_SAMPLE=""
scan_forbidden

# --------------------------------------------------------------------------
# CR status assertions
# --------------------------------------------------------------------------
cr_kind_ok=$("${OC[@]}" get costmanagementmetricsconfig -n "$NS" >/dev/null 2>&1 && echo yes || echo no)
[[ "$cr_kind_ok" == yes ]] || fail "CRD costmanagementmetricsconfig is not served; the operator's CRD is not installed"

if ! "${OC[@]}" get costmanagementmetricsconfig -n "$NS" "$CR_NAME" >/dev/null 2>&1; then
	echo "available CRs in $NS:" >&2
	"${OC[@]}" get costmanagementmetricsconfig -n "$NS" --no-headers 2>/dev/null | sed 's/^/  /' >&2
	fail "MetricsConfig CR $CR_NAME not found in $NS"
fi

# status_json prints the CR status, or {} when it is empty.
status_json() {
	"${OC[@]}" get costmanagementmetricsconfig -n "$NS" "$CR_NAME" -o json 2>/dev/null |
		jq -c '.status // {}'
}

# jq path -> "true" when the value is boolean true, non-empty otherwise.
assert_true() {
	local status="$1" path="$2"
	echo "$status" | jq -e --arg p "$path" '
		getpath($p | split(".")) as $v
		| ($v == true) or (($v | type) == "string" and ($v | length) > 0)' >/dev/null 2>&1
}

report_status() {
	local status="$1"
	echo "$status" | jq -r '
		"prometheus: configured=\(.prometheus.prometheus_configured // "-") connected=\(.prometheus.prometheus_connected // "-")",
		"credentials_found: \(.authentication.credentials_found // "-")",
		"source_defined: \(.source.source_defined // "-")",
		"topology: controlPlaneTopology=\(.topology.controlPlaneTopology // "-") masterNodes=\(.topology.masterNodes // "-") workerNodes=\(.topology.workerNodes // "-")",
		"packaging: number_reports_stored=\(.packaging.number_reports_stored // "-")",
		"upload: last_upload_status=\(.upload.last_upload_status // "-") last_successful_upload_time=\(.upload.last_successful_upload_time // "-")"' 2>/dev/null |
		while IFS= read -r line; do info "$line"; done
}

# Best-effort denials are tolerated, not hidden: they explain a column that is
# empty in the CSVs, which is a data gap worth knowing about while you are
# already looking at this install.
report_besteffort() {
	[[ "${BESTEFFORT_DENIAL_COUNT:-0}" -gt 0 ]] || return 0
	echo
	echo "note: ${BESTEFFORT_DENIAL_COUNT} best-effort denial(s) in the last ${SINCE} (reconcile unaffected):"
	printf '%s\n' "$BESTEFFORT_DENIAL_SAMPLE" | sed 's/^/  /'
	echo "  a collector.* component degrades to an empty result, so the columns it"
	echo "  fills stay empty (machineset_name is the current example)."
}

STATUS=""
hard_failures=""
deadline=$((SECONDS + TIMEOUT))

while true; do
	STATUS=$(status_json)
	hard_failures=""
	[[ "$STALL_DENIAL_COUNT" -eq 0 ]] || hard_failures="authorization failures in the reconcile path ($STALL_DENIAL_COUNT)"
	assert_true "$STATUS" "prometheus.prometheus_configured" || hard_failures+="${hard_failures:+, }status.prometheus.prometheus_configured is not true"
	assert_true "$STATUS" "prometheus.prometheus_connected" || hard_failures+="${hard_failures:+, }status.prometheus.prometheus_connected is not true"
	assert_true "$STATUS" "source.source_defined" || hard_failures+="${hard_failures:+, }status.source.source_defined is not true"
	assert_true "$STATUS" "topology.controlPlaneTopology" || hard_failures+="${hard_failures:+, }status.topology.controlPlaneTopology is empty"
	if [[ "$REQUIRE_UPLOAD" == true ]]; then
		assert_true "$STATUS" "upload.last_successful_upload_time" || hard_failures+="${hard_failures:+, }status.upload.last_successful_upload_time is empty"
	fi

	if [[ -z "$hard_failures" ]]; then
		break
	fi
	if [[ "$WAIT" != true ]] || [[ $SECONDS -ge $deadline ]]; then
		break
	fi
	[[ "$VERBOSE" == true ]] && info "waiting ($SECONDS/${TIMEOUT}s): $hard_failures"
	sleep "$INTERVAL"
	scan_forbidden
done

# --------------------------------------------------------------------------
# result
# --------------------------------------------------------------------------
echo
if [[ -z "$hard_failures" ]]; then
	echo "PASS: $NS/$CR_NAME is installed and reconciling against this cluster."
	report_status "$STATUS"
	[[ "$CSV_PHASE" == "Succeeded" ]] && info "csv: Succeeded"
	if ! assert_true "$STATUS" "upload.last_successful_upload_time"; then
		info "note: no successful upload recorded yet (data path unproven; re-run with --require-upload to gate on it)"
	fi
	report_besteffort
	exit 0
fi

echo "FAIL: $hard_failures"
echo
report_status "$STATUS"
echo
if [[ "$STALL_DENIAL_COUNT" -gt 0 ]]; then
	echo "authorization failures in the reconcile path (last $SINCE):"
	printf '%s\n' "$STALL_DENIAL_SAMPLE" | sed 's/^/  /'
	echo
	echo "  next: hack/verify-bundle-drift.sh --check"
	echo "        the installed bundle grants less than the code requires"
	echo "        (a branch image installed under a stale bundle)"
fi
report_besteffort
if [[ "$STATUS" == "{}" ]]; then
	echo "the CR status is entirely empty: reconcile is not reaching its status write."
	echo "  next: check the operator log for a stall ('Failed to watch', errors before"
	echo "        the first status write). A denied cached read can block reconcile"
	echo "        instead of degrading - see pgarciaq/ros-ocp-backend#729."
fi
topology_empty=$(assert_true "$STATUS" "topology.controlPlaneTopology" && echo no || echo yes)
if [[ "$topology_empty" == yes && "$STATUS" != "{}" ]]; then
	echo "topology is unset while other status fields are set:"
	echo "  next: the operator cannot read infrastructures.config.openshift.io and/or nodes;"
	echo "        hack/verify-bundle-drift.sh --check"
fi
if ! assert_true "$STATUS" "prometheus.prometheus_connected"; then
	echo "prometheus is not connected:"
	echo "$STATUS" | jq -r '  .prometheus.prometheus_connection_error // "  (no connection error recorded)"'
fi
if ! assert_true "$STATUS" "source.source_defined"; then
	echo "source is not defined:"
	echo "$STATUS" | jq -r '  .source.error // "  (no source error recorded)"'
fi
exit 1
