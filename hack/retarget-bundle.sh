#!/usr/bin/env bash

# Copyright 2026 Red Hat Inc.
# SPDX-License-Identifier: Apache-2.0
#
# retarget-bundle.sh - point a bundle's CSV at one operator image, in all three
# places OLM reads it, and prove the write landed.
#
# Why this exists
# ---------------
# The CSV carries the operator image three times:
#
#   metadata.annotations.containerImage
#   spec.relatedImages[0].image
#   spec.install.spec.deployments[<deployment>].spec.template.spec.containers[<c>].image
#
# OLM deploys from the THIRD. Retargeting only the first two (or the first and
# third) installs the upstream quay.io/project-koku image while the catalog, the
# annotation and relatedImages all name the branch digest: the CSV reports
# Succeeded, the wrong binary runs, and nothing complains. That cost a full
# rebuild/deploy cycle on 2026-10-31.
#
# So this script writes all three, re-reads the file, and fails if they disagree.
#
# Usage
# -----
#   hack/retarget-bundle.sh <image-ref> [bundle-dir]     # copy to a temp dir
#   hack/retarget-bundle.sh <image-ref> --out DIR        # copy to DIR
#   hack/retarget-bundle.sh <image-ref> --in-place       # edit bundle/ in place
#   hack/retarget-bundle.sh --check [<image-ref>]        # write nothing, verify
#
#   --bundle-dir DIR      source bundle (default: <repo>/bundle)
#   --container NAME      operator container name (default: manager)
#   --allow-upstream      permit a quay.io/project-koku/... image
#
# Out-of-tree is the default on purpose: bundle/ is committed, so an in-place
# retarget is one `git add -A` away from shipping a lab digest into the artifact.
# The default copy is a fresh temp dir, printed on success.
#
# Env: YQ overrides the yq binary (falls back to bin/yq, then PATH).
#
# Exit: 0 retargeted (or --check passed), 1 verification failed, 2 usage/tooling.

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

CSV_GLOB='*.clusterserviceversion.yaml'
DEFAULT_BUNDLE_DIR="${SCRIPT_ROOT}/bundle"
CONTAINER_NAME=manager
ALLOW_UPSTREAM=false
CHECK=false
IN_PLACE=false
OUT_DIR=""
BUNDLE_DIR="${DEFAULT_BUNDLE_DIR}"
IMAGE=""

usage() {
	# Test the intact line: sub() rewrites $0, so a later /^#/ test would see
	# the stripped line and stop after the first line.
	awk '/^# Usage$/ { printing = 1 } printing { if ($0 !~ /^#/) exit; sub(/^# ?/, ""); print }' \
		"${BASH_SOURCE[0]}" >&2
}

fail_usage() {
	echo "retarget-bundle: $*" >&2
	echo "run 'hack/retarget-bundle.sh --help' for usage" >&2
	exit 2
}

while [[ $# -gt 0 ]]; do
	case "$1" in
	--help | -h)
		usage
		exit 0
		;;
	--check)
		CHECK=true
		shift
		;;
	--in-place)
		IN_PLACE=true
		shift
		;;
	--allow-upstream)
		ALLOW_UPSTREAM=true
		shift
		;;
	--out)
		[[ $# -ge 2 ]] || fail_usage "--out requires a directory"
		OUT_DIR="$2"
		shift 2
		;;
	--bundle-dir)
		[[ $# -ge 2 ]] || fail_usage "--bundle-dir requires a directory"
		BUNDLE_DIR="$2"
		shift 2
		;;
	--container)
		[[ $# -ge 2 ]] || fail_usage "--container requires a name"
		CONTAINER_NAME="$2"
		shift 2
		;;
	-*)
		fail_usage "unknown flag '$1'"
		;;
	*)
		if [[ -z "${IMAGE}" ]]; then
			IMAGE="$1"
		elif [[ "${BUNDLE_DIR}" == "${DEFAULT_BUNDLE_DIR}" ]]; then
			# The issue documents `<image-ref> [bundle-dir]`; the flag is the
			# explicit form of the same thing.
			BUNDLE_DIR="$1"
		else
			fail_usage "unexpected extra argument '$1'"
		fi
		shift
		;;
	esac
done

if ${CHECK}; then
	[[ -z "${OUT_DIR}" && "${IN_PLACE}" == false ]] || fail_usage "--check writes nothing; drop --out/--in-place"
else
	[[ -n "${IMAGE}" ]] || fail_usage "an image reference is required"
	[[ -z "${OUT_DIR}" ]] || [[ "${IN_PLACE}" == false ]] || fail_usage "--out and --in-place are mutually exclusive"
fi

resolve_yq() {
	if [[ -n "${YQ:-}" ]]; then
		printf '%s' "${YQ}"
	elif [[ -x "${SCRIPT_ROOT}/bin/yq" ]]; then
		printf '%s' "${SCRIPT_ROOT}/bin/yq"
	elif command -v yq >/dev/null 2>&1; then
		command -v yq
	else
		return 1
	fi
}

YQ_BIN=$(resolve_yq) || {
	echo "retarget-bundle: yq is required (run 'make yq' to download the pinned version)" >&2
	exit 2
}

validate_image_ref() {
	local image="$1"
	[[ -n "${image}" ]] || fail_usage "image reference is empty"
	[[ "${image}" != -* ]] || fail_usage "image reference must not start with '-'"
	[[ "${image}" =~ ^[A-Za-z0-9._:/-]+(@sha256:[a-f0-9]+)?$ ]] ||
		fail_usage "image reference '${image}' is not a valid name[@sha256:digest][:tag]"

	if [[ "${image}" == quay.io/project-koku/* && "${ALLOW_UPSTREAM}" == false ]]; then
		echo "retarget-bundle: refusing the released upstream image '${image}'." >&2
		echo "retarget-bundle: a fork/branch deploy must name its own digest; pass --allow-upstream only to reinstall upstream on purpose." >&2
		exit 2
	fi

	if [[ "${image}" != *:* && "${image}" != *@sha256:* ]]; then
		echo "retarget-bundle: warning: '${image}' has no tag or digest, so it resolves to :latest." >&2
	fi
}

[[ -d "${BUNDLE_DIR}" ]] || fail_usage "bundle directory '${BUNDLE_DIR}' does not exist"

CSV_ABS=$(find "${BUNDLE_DIR}/manifests" -maxdepth 1 -name "${CSV_GLOB}" -print -quit 2>/dev/null || true)
[[ -n "${CSV_ABS}" ]] || fail_usage "no ${CSV_GLOB} under ${BUNDLE_DIR}/manifests"
CSV_REL="${CSV_ABS#"${BUNDLE_DIR}"/}"

read_fields() {
	local csv="$1"
	"${YQ_BIN}" -r '.metadata.annotations.containerImage // ""' "${csv}"
	"${YQ_BIN}" -r '.spec.relatedImages[0].image // ""' "${csv}"
	"${YQ_BIN}" -r \
		".spec.install.spec.deployments[].spec.template.spec.containers[] | select(.name == \"${CONTAINER_NAME}\") | .image" \
		"${csv}"
}

# container_names lists what the deployment actually declares, so a failed
# lookup names the alternatives instead of just saying "not found".
container_names() {
	"${YQ_BIN}" -r '.spec.install.spec.deployments[].spec.template.spec.containers[].name' "$1" | tr '\n' ' '
}

verify_fields() {
	local csv="$1" want="$2"
	local annotation related deploy
	annotation=$("${YQ_BIN}" -r '.metadata.annotations.containerImage // ""' "${csv}")
	related=$("${YQ_BIN}" -r '.spec.relatedImages[0].image // ""' "${csv}")
	deploy=$(read_fields "${csv}" | sed -n '3p')

	[[ -n "${deploy}" ]] || {
		echo "retarget-bundle: no container named '${CONTAINER_NAME}' in the CSV deployment (found: $(container_names "${csv}"))" >&2
		return 1
	}

	local failed=false
	if [[ -n "${want}" && "${annotation}" != "${want}" ]]; then
		echo "retarget-bundle: metadata.annotations.containerImage is '${annotation}', expected '${want}'" >&2
		failed=true
	fi
	if [[ -n "${want}" && "${related}" != "${want}" ]]; then
		echo "retarget-bundle: spec.relatedImages[0].image is '${related}', expected '${want}'" >&2
		failed=true
	fi
	if [[ -n "${want}" && "${deploy}" != "${want}" ]]; then
		echo "retarget-bundle: deployment container '${CONTAINER_NAME}' image is '${deploy}', expected '${want}'" >&2
		failed=true
	fi
	if [[ "${annotation}" != "${related}" || "${annotation}" != "${deploy}" ]]; then
		echo "retarget-bundle: the three image fields disagree; OLM deploys from the deployment, so the catalog would name a different image than the one that runs" >&2
		failed=true
	fi
	${failed} && return 1

	printf 'metadata.annotations.containerImage = %s\n' "${annotation}"
	printf 'spec.relatedImages[0].image          = %s\n' "${related}"
	printf 'deployment/%s image                  = %s\n' "${CONTAINER_NAME}" "${deploy}"
	return 0
}

if ${CHECK}; then
	[[ -z "${IMAGE}" ]] || validate_image_ref "${IMAGE}"
	verify_fields "${BUNDLE_DIR}/${CSV_REL}" "${IMAGE}"
	echo "retarget-bundle: ${BUNDLE_DIR}/${CSV_REL} is consistent"
	exit 0
fi

validate_image_ref "${IMAGE}"

if ${IN_PLACE}; then
	OUT_DIR="${BUNDLE_DIR}"
else
	if [[ -z "${OUT_DIR}" ]]; then
		OUT_DIR=$(mktemp -d "${TMPDIR:-/tmp}/kmo-bundle.XXXXXX")
	fi
	[[ ! -e "${OUT_DIR}" ]] || [[ -d "${OUT_DIR}" && -z "$(ls -A "${OUT_DIR}")" ]] ||
		fail_usage "--out '${OUT_DIR}' exists and is not empty"
	mkdir -p "${OUT_DIR}"
	# Copy the CONTENTS, not the directory: `cp -a bundle DIR` nests it as
	# DIR/bundle whenever DIR already exists, and the mktemp default always does.
	cp -a "${BUNDLE_DIR}/." "${OUT_DIR}/"
fi

TARGET_CSV="${OUT_DIR}/${CSV_REL}"
[[ -f "${TARGET_CSV}" ]] || fail_usage "'${TARGET_CSV}' does not exist after the copy"

OLD_IMAGE=$("${YQ_BIN}" -r '.metadata.annotations.containerImage // ""' "${TARGET_CSV}")

IMG="${IMAGE}" "${YQ_BIN}" -i \
	'.metadata.annotations.containerImage = strenv(IMG) |
	 .spec.relatedImages[0].image = strenv(IMG)' "${TARGET_CSV}"

# The deployment holds the image OLM actually runs. Target it by container name:
# index 0 happens to be "manager" today, but a sidecar inserted before it would
# silently retarget the wrong container.
IMG="${IMAGE}" CONTAINER="${CONTAINER_NAME}" "${YQ_BIN}" -i \
	'(.spec.install.spec.deployments[].spec.template.spec.containers[]
	  | select(.name == strenv(CONTAINER))
	  | .image) = strenv(IMG)' "${TARGET_CSV}"

if ! verify_fields "${TARGET_CSV}" "${IMAGE}"; then
	echo "retarget-bundle: retarget did not land consistently in ${TARGET_CSV}" >&2
	exit 1
fi

if [[ -n "${OLD_IMAGE}" && "${OLD_IMAGE}" != "${IMAGE}" ]]; then
	LEFTOVER=$(OLD="${OLD_IMAGE}" "${YQ_BIN}" -r \
		'.spec.install.spec.deployments[].spec.template.spec.containers[]
		 | select(.image == strenv(OLD)) | .name' "${TARGET_CSV}")
	if [[ -n "${LEFTOVER}" ]]; then
		echo "retarget-bundle: containers still on the old image '${OLD_IMAGE}': ${LEFTOVER}" >&2
		echo "retarget-bundle: OLM may deploy one of those; update them or pass --container" >&2
		exit 1
	fi
fi

echo "retarget-bundle: wrote ${TARGET_CSV}"
