#!/usr/bin/env bash

# Copyright 2026 Red Hat Inc.
# SPDX-License-Identifier: Apache-2.0
#
# retarget-bundle.selftest.sh - regression test for hack/retarget-bundle.sh.
#
# Why this exists
# ---------------
# The CSV names the operator image three times and OLM deploys from the third
# (the deployment container). Retargeting that third site is the whole point of
# the script, and it is also the site a hand edit misses: the 2026-10-31 deploy
# shipped upstream's image while the annotation and relatedImages named the
# branch digest, and the CSV still reported Succeeded under the installed hash,
# which is exactly what the script rewrites.
#
# A silent miss here is expensive (a full image build, push, bundle build,
# install and rollout) and invisible in review, so each behaviour is pinned
# against fixtures here. The shipping script is executed; nothing is
# re-implemented.
#
# Usage: hack/retarget-bundle.selftest.sh   (needs yq, no cluster, no podman)
# Exit:  0 all cases pass, 1 a case regressed, 2 the script or yq is missing.

set -o errexit
set -o nounset
set -o pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="${HERE}/retarget-bundle.sh"
[[ -x "${SCRIPT}" ]] || {
	echo "selftest: ${SCRIPT} is missing or not executable" >&2
	exit 2
}

resolve_yq() {
	if [[ -n "${YQ:-}" ]]; then
		printf '%s' "${YQ}"
	elif [[ -x "${HERE}/../bin/yq" ]]; then
		printf '%s' "${HERE}/../bin/yq"
	elif command -v yq >/dev/null 2>&1; then
		command -v yq
	else
		return 1
	fi
}
YQ_BIN=$(resolve_yq) || {
	echo "selftest: yq is required (run 'make yq' to download the pinned version)" >&2
	exit 2
}
export YQ="${YQ_BIN}"

TMP="$(mktemp -d)"
trap 'rm -rf "${TMP}"' EXIT

OLD="quay.io/example/test-operator@sha256:1111111111111111111111111111111111111111111111111111111111111111"
NEW="quay.io/pgarciaq/test-operator@sha256:2222222222222222222222222222222222222222222222222222222222222222"
SIDECAR="quay.io/openshift/origin-oauth-proxy@sha256:3333333333333333333333333333333333333333333333333333333333333333"

# make_bundle <dir> [sidecar] writes a bundle whose CSV carries the image in all
# three places OLM reads. With "sidecar", an unrelated container is placed at
# index 0 so an index-based retarget would hit the wrong container.
make_bundle() {
	local dir="$1" sidecar="${2:-}"
	mkdir -p "${dir}/manifests" "${dir}/metadata"
	local csv="${dir}/manifests/test-operator.clusterserviceversion.yaml"
	{
		cat <<-YAML
			apiVersion: operators.coreos.com/v1alpha1
			kind: ClusterServiceVersion
			metadata:
			  name: test-operator.v0.0.1
			  annotations:
			    containerImage: ${OLD}
			spec:
			  install:
			    spec:
			      deployments:
			        - name: test-operator-controller-manager
			          spec:
			            template:
			              spec:
			                containers:
		YAML
		if [[ -n "${sidecar}" ]]; then
			cat <<-YAML
			                  - name: oauth-proxy
			                    image: ${SIDECAR}
			YAML
		fi
		cat <<-YAML
			                  - name: manager
			                    image: ${OLD}
			  relatedImages:
			    - name: test-operator
			      image: ${OLD}
		YAML
	} >"${csv}"
}

CASE_OUT=""
CASE_RC=0

# expect <name> <want-rc> <command...> runs the shipping script and records both
# its exit code and its combined output for the substring assertions below.
expect() {
	local name="$1" want_rc="$2"
	shift 2
	CASE_RC=0
	CASE_OUT=$("$@" 2>&1) || CASE_RC=$?
	if [[ "${CASE_RC}" != "${want_rc}" ]]; then
		echo "FAIL: ${name}: exit ${CASE_RC}, want ${want_rc}" >&2
		echo "  command: $*" >&2
		echo "  output:  ${CASE_OUT}" >&2
		exit 1
	fi
	echo "ok: ${name} (exit ${CASE_RC})"
}

expect_says() {
	local name="$1" needle="$2"
	if [[ "${CASE_OUT}" != *"${needle}"* ]]; then
		echo "FAIL: ${name}: output does not contain '${needle}'" >&2
		echo "  output: ${CASE_OUT}" >&2
		exit 1
	fi
}

field() {
	local csv="$1" expr="$2"
	"${YQ_BIN}" -r "${expr}" "${csv}"
}

# 1. The default is out-of-tree: bundle/ is committed, so a retarget must not
#    edit it unless --in-place is explicit.
make_bundle "${TMP}/src"
expect "retarget writes to --out" 0 "${SCRIPT}" "${NEW}" --out "${TMP}/out" --bundle-dir "${TMP}/src"
DEST_CSV="${TMP}/out/manifests/test-operator.clusterserviceversion.yaml"
[[ "$(field "${DEST_CSV}" '.spec.install.spec.deployments[0].spec.template.spec.containers[0].image')" == "${NEW}" ]] || {
	echo "FAIL: deployment container image was not retargeted" >&2
	exit 1
}
[[ "$(field "${TMP}/src/manifests/test-operator.clusterserviceversion.yaml" '.metadata.annotations.containerImage')" == "${OLD}" ]] || {
	echo "FAIL: the source bundle was modified; the default must be out-of-tree" >&2
	exit 1
}
echo "ok: source bundle untouched"

# 2. All three sites, not just the two obvious ones.
for expr in '.metadata.annotations.containerImage' '.spec.relatedImages[0].image'; do
	got="$(field "${DEST_CSV}" "${expr}")"
	[[ "${got}" == "${NEW}" ]] || {
		echo "FAIL: ${expr} is '${got}', want '${NEW}'" >&2
		exit 1
	}
done
echo "ok: all three image sites retargeted"

# 3. --check passes on the retargeted bundle.
expect "--check accepts a consistent bundle" 0 "${SCRIPT}" --check --bundle-dir "${TMP}/out"
# 4. --check accepts it when the expected image is named too.
expect "--check accepts an explicit image" 0 "${SCRIPT}" --check "${NEW}" --bundle-dir "${TMP}/out"

# 5. The 2026-10-31 bug, precisely: annotation and relatedImages agree with each
#    other but the deployment still runs the old image. That bundle must fail,
#    because OLM deploys the deployment's image.
cp -a "${TMP}/out" "${TMP}/diverged-deploy"
DIVERGED_CSV="${TMP}/diverged-deploy/manifests/test-operator.clusterserviceversion.yaml"
OLD="${OLD}" "${YQ_BIN}" -i \
	'.spec.install.spec.deployments[0].spec.template.spec.containers[0].image = strenv(OLD)' \
	"${DIVERGED_CSV}"
expect "--check rejects a deployment left on the old image" 1 "${SCRIPT}" --check "${NEW}" --bundle-dir "${TMP}/diverged-deploy"
expect_says "diverged deployment names the field" "deployment container 'manager' image is"
expect_says "diverged deployment names the disagreement" "the three image fields disagree"

# 6. Same for a stale relatedImages entry.
cp -a "${TMP}/out" "${TMP}/diverged-related"
OLD="${OLD}" "${YQ_BIN}" -i \
	'.spec.relatedImages[0].image = strenv(OLD)' \
	"${TMP}/diverged-related/manifests/test-operator.clusterserviceversion.yaml"
expect "--check rejects a stale relatedImages" 1 "${SCRIPT}" --check "${NEW}" --bundle-dir "${TMP}/diverged-related"
expect_says "stale relatedImages is named" "spec.relatedImages[0].image is"

# 7. The container is found by name, not by index: a sidecar at index 0 must not
#    receive the image.
make_bundle "${TMP}/sidecar" sidecar
expect "retarget handles a sidecar before the operator" 0 "${SCRIPT}" "${NEW}" --out "${TMP}/out-sidecar" --bundle-dir "${TMP}/sidecar"
SIDECAR_CSV="${TMP}/out-sidecar/manifests/test-operator.clusterserviceversion.yaml"
[[ "$(field "${SIDECAR_CSV}" '.spec.install.spec.deployments[0].spec.template.spec.containers[0].image')" == "${SIDECAR}" ]] || {
	echo "FAIL: the sidecar at index 0 was retargeted" >&2
	exit 1
}
[[ "$(field "${SIDECAR_CSV}" '.spec.install.spec.deployments[0].spec.template.spec.containers[1].image')" == "${NEW}" ]] || {
	echo "FAIL: the manager container was not retargeted" >&2
	exit 1
}
echo "ok: retarget is by container name, not index"

# 8. A wrong --container names what the deployment actually has.
expect "unknown container fails" 1 "${SCRIPT}" "${NEW}" --out "${TMP}/out-absent" --bundle-dir "${TMP}/src" --container nope
expect_says "unknown container lists the alternatives" "found: manager"

# 9. Upstream is refused: a fork/branch deploy that silently reinstalls the
#    released image looks installed and is not.
expect "upstream image is refused" 2 "${SCRIPT}" "quay.io/project-koku/test-operator@sha256:4444444444444444444444444444444444444444444444444444444444444444" --out "${TMP}/out-upstream" --bundle-dir "${TMP}/src"
expect_says "upstream refusal explains itself" "refusing the released upstream image"
[[ ! -e "${TMP}/out-upstream" ]] || {
	echo "FAIL: the refused retarget still created ${TMP}/out-upstream" >&2
	exit 1
}
echo "ok: refusal wrote nothing"

# 10. A floating reference is allowed but warned about: :latest is what made the
#     2026-10-31 image un-rerunnable from the CSV.
expect "floating reference warns but writes" 0 "${SCRIPT}" "quay.io/pgarciaq/test-operator" --out "${TMP}/out-floating" --bundle-dir "${TMP}/src"
expect_says "floating reference names the consequence" "has no tag or digest"

# 11. A non-empty --out is refused rather than merged into.
mkdir -p "${TMP}/occupied" && touch "${TMP}/occupied/keep"
expect "non-empty --out is refused" 2 "${SCRIPT}" "${NEW}" --out "${TMP}/occupied" --bundle-dir "${TMP}/src"

# 12. The default output is a fresh temp dir that already exists when the copy
#     runs, so the copy must place manifests/ directly under it. A regression
#     here is invisible: the nested copy passes every field check on the source
#     path and only the final `re-read` notices.
make_bundle "${TMP}/defaultsrc"
CASE_RC=0
CASE_OUT=$("${SCRIPT}" "${NEW}" --bundle-dir "${TMP}/defaultsrc" 2>&1) || CASE_RC=$?
[[ "${CASE_RC}" == 0 ]] || {
	echo "FAIL: default output: exit ${CASE_RC}, want 0" >&2
	echo "  output: ${CASE_OUT}" >&2
	exit 1
}
DEFAULT_DIR="${CASE_OUT##*retarget-bundle: wrote }"
DEFAULT_DIR="${DEFAULT_DIR%/manifests/*}"
[[ -f "${DEFAULT_DIR}/manifests/test-operator.clusterserviceversion.yaml" ]] || {
	echo "FAIL: default output is not a bundle root: ${DEFAULT_DIR}" >&2
	echo "  output: ${CASE_OUT}" >&2
	exit 1
}
[[ ! -d "${DEFAULT_DIR}/bundle" ]] || {
	echo "FAIL: default output nested the bundle at ${DEFAULT_DIR}/bundle" >&2
	exit 1
}
echo "ok: default output is a flat bundle copy"
rm -rf "${DEFAULT_DIR}"

# 13. An --out that already exists and is empty (what mktemp -d gives you, and
#     what a re-run after `rm -rf DIR/manifests` gives you) must also be flat.
make_bundle "${TMP}/emptysrc"
mkdir -p "${TMP}/empty-out"
expect "--out accepts an existing empty directory" 0 "${SCRIPT}" "${NEW}" --out "${TMP}/empty-out" --bundle-dir "${TMP}/emptysrc"
[[ -f "${TMP}/empty-out/manifests/test-operator.clusterserviceversion.yaml" ]] || {
	echo "FAIL: existing empty --out was not filled flat: $(ls -A "${TMP}/empty-out")" >&2
	exit 1
}

# 14. --check writes nothing, so it cannot be combined with a write mode.
expect "--check rejects --in-place" 2 "${SCRIPT}" --check --in-place --bundle-dir "${TMP}/src"

# 15. --help must print the whole usage block. The extractor stops at the first
#     non-comment line, so an off-by-one leaves a one-line usage screen.
expect "--help prints the usage block" 0 "${SCRIPT}" --help
expect_says "--help lists the flags" "--bundle-dir DIR"

echo "retarget-bundle selftest: all cases pass"
