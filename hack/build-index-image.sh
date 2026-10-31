#!/usr/bin/env bash

# Copyright 2026 Red Hat Inc.
# SPDX-License-Identifier: Apache-2.0
#
# build-index-image.sh - build a file-based-catalog index image that provably
# serves, before anyone pushes it.
#
# Why this exists
# ---------------
# An index image built from
#
#   FROM quay.io/operator-framework/opm:latest
#   COPY configs /configs
#
# inherits the base's Entrypoint (/bin/opm) and no CMD, so the catalog pod runs
# `opm` with no arguments and exits 0 in under a second: CrashLoopBackOff with
# empty logs. `opm validate` passes on the same configs, so every offline check
# says yes. That cost a full deploy cycle on 2026-10-31.
#
# `opm generate dockerfile` emits the right shape (ENTRYPOINT + CMD serve, and a
# cache-prepopulating builder stage), but it defaults BOTH stages to
# `quay.io/operator-framework/opm:latest`: -i pins only the final stage, so the
# builder stage must be pinned separately with -b. Both are passed here, and the
# generated Dockerfile is then checked line by line rather than trusted.
#
# The image is never pushed. The last line prints the command.
#
# Usage
# -----
#   hack/build-index-image.sh <configs-dir> <image-ref> [flags]
#
#   --platform ARCH       build for ARCH (e.g. linux/arm64); also asserted on the
#                         built image. Pass it explicitly: a podman whose default
#                         platform differs from the cluster's builds a catalog the
#                         cluster cannot run, and nothing else notices.
#   --dockerfile FILE     use this Dockerfile instead of generating one (the same
#                         checks apply).
#   --check-dockerfile    run the Dockerfile checks and stop; no build, no podman
#                         needed. What a long build should be preceded by.
#   --smoke-timeout SEC   how long to wait for "serving registry" (default 20).
#
# Env: OPM_IMAGE overrides the pinned opm image (must be a digest reference).
#      CONTAINER_TOOL overrides podman.
#
# Exit: 0 built and serving, 1 a check failed, 2 usage/tooling.

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

WORK_DIR=""
SMOKE_NAME=""
CONTAINER=""

cleanup() {
	if [[ -n "${SMOKE_NAME}" && -n "${CONTAINER}" ]]; then
		"${CONTAINER}" rm -f "${SMOKE_NAME}" >/dev/null 2>&1 || true
	fi
	if [[ -n "${WORK_DIR}" ]]; then
		rm -rf "${WORK_DIR}"
	fi
}
trap cleanup EXIT

# Pinned, and pinned by digest on purpose: opm:latest is what produced the
# 2026-10-31 image, and a tag can move under a reproducible build. v1.74.0, the
# multi-arch manifest list (amd64/arm64/ppc64le/s390x), so one pin builds for
# every cluster we run.
OPM_IMAGE="${OPM_IMAGE:-quay.io/operator-framework/opm@sha256:b32d3891616662620da08d7f0ec42c2e69fa2de43427dc975d35b12f7a969a0f}"

CONFIGS=""
IMAGE_REF=""
PLATFORM=""
DOCKERFILE=""
CHECK_DOCKERFILE=false
SMOKE_TIMEOUT=20

usage() {
	# Test the intact line: sub() rewrites $0, so a later /^#/ test would see
	# the stripped line and stop after the first line.
	awk '/^# Usage$/ { printing = 1 } printing { if ($0 !~ /^#/) exit; sub(/^# ?/, ""); print }' \
		"${BASH_SOURCE[0]}" >&2
}

fail_usage() {
	echo "build-index-image: $*" >&2
	echo "run 'hack/build-index-image.sh --help' for usage" >&2
	exit 2
}

while [[ $# -gt 0 ]]; do
	case "$1" in
	--help | -h)
		usage
		exit 0
		;;
	--check-dockerfile)
		CHECK_DOCKERFILE=true
		shift
		;;
	--platform)
		[[ $# -ge 2 ]] || fail_usage "--platform requires a value"
		PLATFORM="$2"
		shift 2
		;;
	--dockerfile)
		[[ $# -ge 2 ]] || fail_usage "--dockerfile requires a path"
		DOCKERFILE="$2"
		shift 2
		;;
	--smoke-timeout)
		[[ $# -ge 2 ]] || fail_usage "--smoke-timeout requires seconds"
		SMOKE_TIMEOUT="$2"
		shift 2
		;;
	-*)
		fail_usage "unknown flag '$1'"
		;;
	*)
		if [[ -z "${CONFIGS}" ]]; then
			CONFIGS="$1"
		elif [[ -z "${IMAGE_REF}" ]]; then
			IMAGE_REF="$1"
		else
			fail_usage "unexpected extra argument '$1'"
		fi
		shift
		;;
	esac
done

[[ -n "${CONFIGS}" ]] || fail_usage "a file-based catalog directory is required"
[[ -d "${CONFIGS}" ]] || fail_usage "'${CONFIGS}' is not a directory"
CONFIGS_ABS=$(cd "${CONFIGS}" && pwd)
CONTEXT=$(dirname "${CONFIGS_ABS}")

if ! ${CHECK_DOCKERFILE}; then
	[[ -n "${IMAGE_REF}" ]] || fail_usage "an index image reference is required"
	[[ "${IMAGE_REF}" != -* ]] || fail_usage "image reference must not start with '-'"
fi

resolve_container_tool() {
	if [[ -n "${CONTAINER_TOOL:-}" ]]; then
		printf '%s' "${CONTAINER_TOOL}"
	elif command -v podman >/dev/null 2>&1; then
		command -v podman
	elif command -v docker >/dev/null 2>&1; then
		command -v docker
	else
		return 1
	fi
}

# check_dockerfile is the whole point: every stage pinned, an entrypoint, and a
# command that serves. A Dockerfile can look right and still leave the catalog
# pod with nothing to run, so each requirement fails with what to add.
check_dockerfile() {
	local file="$1"
	local failed=false
	local line
	local -a from_lines=()

	[[ -f "${file}" ]] || fail_usage "'${file}' does not exist"

	while IFS= read -r line; do
		from_lines+=("${line}")
	done < <(grep -Ei '^[[:space:]]*FROM[[:space:]]' "${file}" || true)

	if [[ ${#from_lines[@]} -eq 0 ]]; then
		echo "build-index-image: ${file} has no FROM line" >&2
		return 1
	fi

	for line in "${from_lines[@]}"; do
		if [[ "${line}" != *@sha256:* ]]; then
			echo "build-index-image: ${file}: '${line#FROM }' is not pinned by digest." >&2
			echo "build-index-image: opm generate dockerfile defaults BOTH the builder and the final stage to opm:latest; pass -i and -b with the same digest (OPM_IMAGE=${OPM_IMAGE})." >&2
			failed=true
		fi
		if [[ "${line}" == *:latest* ]]; then
			echo "build-index-image: ${file}: '${line#FROM }' uses :latest, which can move under a rebuilt image." >&2
			failed=true
		fi
	done

	if ! grep -Eq '^[[:space:]]*ENTRYPOINT[[:space:]]+\[.*"/bin/opm"' "${file}"; then
		echo "build-index-image: ${file} has no ENTRYPOINT [\"/bin/opm\"]." >&2
		echo "build-index-image: without it the catalog binary is whatever the base image happens to run." >&2
		failed=true
	fi

	if ! grep -Eq '^[[:space:]]*CMD[[:space:]]+\[.*serve' "${file}"; then
		echo "build-index-image: ${file} has no CMD serving the catalog." >&2
		echo "build-index-image: opm's base image has Entrypoint /bin/opm and no Cmd, so this image runs 'opm' with no arguments, exits 0, and the catalog pod CrashLoopBackOffs with empty logs." >&2
		echo "build-index-image: add CMD [\"serve\", \"/configs\"], or generate the Dockerfile with opm generate dockerfile." >&2
		failed=true
	fi

	if ${failed}; then
		return 1
	fi
	return 0
}

if [[ -n "${DOCKERFILE}" ]]; then
	[[ -f "${DOCKERFILE}" ]] || fail_usage "'${DOCKERFILE}' does not exist"
	if ! check_dockerfile "${DOCKERFILE}"; then
		exit 1
	fi
	if ${CHECK_DOCKERFILE}; then
		echo "build-index-image: ${DOCKERFILE} passes the Dockerfile checks"
		exit 0
	fi
	BUILD_DOCKERFILE="${DOCKERFILE}"
	BUILD_CONTEXT="${CONTEXT}"
else
	if ${CHECK_DOCKERFILE}; then
		fail_usage "--check-dockerfile with no --dockerfile needs opm to generate one; pass --dockerfile, or drop the flag to generate and check"
	fi

	CONTAINER=$(resolve_container_tool) || fail_usage "podman or docker is required"

	WORK_DIR=$(mktemp -d "${TMPDIR:-/tmp}/kmo-index.XXXXXX")
	cp -a "${CONFIGS_ABS}" "${WORK_DIR}/configs"
	# The opm image runs as a non-root user; it has to write configs.Dockerfile
	# into WORK_DIR.
	chmod -R a+rwX "${WORK_DIR}"

	echo "build-index-image: generating a Dockerfile with ${OPM_IMAGE}"
	"${CONTAINER}" run --rm -v "${WORK_DIR}:/work:z" -w /work \
		"${OPM_IMAGE}" generate dockerfile -i "${OPM_IMAGE}" -b "${OPM_IMAGE}" configs >/dev/null

	BUILD_DOCKERFILE="${WORK_DIR}/configs.Dockerfile"
	BUILD_CONTEXT="${WORK_DIR}"
	if ! check_dockerfile "${BUILD_DOCKERFILE}"; then
		echo "build-index-image: the generated Dockerfile failed its own checks; refusing to build" >&2
		exit 1
	fi
fi

# Check the pin before reaching for the container tool: an OPM_IMAGE override
# with a tag would unpin every generated Dockerfile, and that must fail whether
# or not podman happens to be installed.
[[ "${OPM_IMAGE}" == *@sha256:* ]] ||
	fail_usage "OPM_IMAGE must be a digest reference, got '${OPM_IMAGE}'"
CONTAINER=$(resolve_container_tool) || fail_usage "podman or docker is required"

echo "build-index-image: validating ${CONFIGS_ABS}"
"${CONTAINER}" run --rm -v "${CONFIGS_ABS}:/configs:z,ro" "${OPM_IMAGE}" validate /configs >/dev/null ||
	{ echo "build-index-image: opm validate rejected ${CONFIGS_ABS}" >&2; exit 1; }

build_args=(build -t "${IMAGE_REF}" -f "${BUILD_DOCKERFILE}")
if [[ -n "${PLATFORM}" ]]; then
	build_args+=(--platform "${PLATFORM}")
fi
echo "build-index-image: building ${IMAGE_REF} from ${BUILD_DOCKERFILE} (context ${BUILD_CONTEXT})"
"${CONTAINER}" "${build_args[@]}" "${BUILD_CONTEXT}"

if [[ -n "${PLATFORM}" ]]; then
	BUILT_ARCH=$("${CONTAINER}" image inspect --format '{{.Architecture}}' "${IMAGE_REF}")
	if [[ "${BUILT_ARCH}" != "${PLATFORM#linux/}" ]]; then
		echo "build-index-image: built ${IMAGE_REF} for '${BUILT_ARCH}', but --platform asked for '${PLATFORM}'" >&2
		exit 1
	fi
fi

# Smoke: start it and read the log. A `timeout N podman run` check cannot tell a
# healthy image from one that served for a second and exited, and returns 124 on
# success, so the exit code carries no signal.
SMOKE_NAME="kmo-index-smoke-$$"

echo "build-index-image: starting ${IMAGE_REF} to confirm it serves"
"${CONTAINER}" run -d --name "${SMOKE_NAME}" "${IMAGE_REF}" >/dev/null

served=false
elapsed=0
SMOKE_LOG=""
while [[ ${elapsed} -lt ${SMOKE_TIMEOUT} ]]; do
	SMOKE_LOG=$("${CONTAINER}" logs "${SMOKE_NAME}" 2>&1 || true)
	if grep -q 'serving registry' <<<"${SMOKE_LOG}"; then
		served=true
		break
	fi
	running=$("${CONTAINER}" inspect --format '{{.State.Running}}' "${SMOKE_NAME}" 2>/dev/null || echo false)
	[[ "${running}" == "true" ]] || break
	sleep 1
	elapsed=$((elapsed + 1))
done

if ! ${served}; then
	SMOKE_LOG=$("${CONTAINER}" logs "${SMOKE_NAME}" 2>&1 || true)
	STATE=$("${CONTAINER}" inspect --format '{{.State.Status}} exit={{.State.ExitCode}}' "${SMOKE_NAME}" 2>/dev/null || echo "unknown")
	echo "build-index-image: ${IMAGE_REF} never logged 'serving registry' (${STATE} after ${elapsed}s)" >&2
	echo "build-index-image: last log lines:" >&2
	printf '%s\n' "${SMOKE_LOG:-<empty>}" | tail -n 20 >&2
	if [[ "${STATE}" == *"exit=0"* ]]; then
		echo "build-index-image: it exited 0 with no catalog served, which is what an image with no CMD does: opm runs with no arguments, exits, and the catalog pod CrashLoopBackOffs with empty logs." >&2
	else
		echo "build-index-image: it exited non-zero, so the catalog under /configs is not servable even though it validated offline." >&2
	fi
	echo "build-index-image: do not push this image." >&2
	exit 1
fi

echo "build-index-image: serving: $(grep -m1 'serving registry' <<<"${SMOKE_LOG}")"
echo "build-index-image: ${IMAGE_REF} built and serving; nothing was pushed"
echo "build-index-image: next: ${CONTAINER} push ${IMAGE_REF}"
