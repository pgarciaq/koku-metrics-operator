#!/usr/bin/env bash

# Copyright 2026 Red Hat Inc.
# SPDX-License-Identifier: Apache-2.0
#
# build-index-image.selftest.sh - regression test for the Dockerfile checks in
# hack/build-index-image.sh.
#
# Why this exists
# ---------------
# An index image is only correct if it does two things: both build stages come
# from a pinned opm, and the finished image actually serves the catalog. Every
# way of getting that wrong is silent until a cluster runs it:
#
#   * no CMD -> the base's Entrypoint (/bin/opm) runs with no arguments, exits 0
#     in under a second, and the catalog pod CrashLoopBackOffs with empty logs;
#   * a floating base -> the image that worked yesterday builds differently
#     today, and `opm validate` is still green on the same configs;
#   * an unpinned BUILDER stage -> `opm generate dockerfile` pins only the final
#     stage with -i, so the trap is a half-pinned file that looks pinned.
#
# The 2026-10-31 index hit the first two at once, and only the cluster noticed.
# These cases pin the guard against that, and assert the message names the cause
# so a future edit cannot fail for the wrong reason and pass review.
#
# Usage: hack/build-index-image.selftest.sh   (no podman, no network)
# Exit:  0 all cases pass, 1 a case regressed, 2 the script is missing.

set -o errexit
set -o nounset
set -o pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="${HERE}/build-index-image.sh"
[[ -x "${SCRIPT}" ]] || {
	echo "selftest: ${SCRIPT} is missing or not executable" >&2
	exit 2
}

TMP="$(mktemp -d)"
trap 'rm -rf "${TMP}"' EXIT

mkdir -p "${TMP}/configs"
DIGEST="sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
BASE="quay.io/operator-framework/opm@${DIGEST}"
CONFIGS="${TMP}/configs"

CASE_OUT=""
CASE_RC=0

expect_dockerfile() {
	local name="$1" want_rc="$2" file="$3"
	CASE_RC=0
	CASE_OUT=$("${SCRIPT}" "${CONFIGS}" --check-dockerfile --dockerfile "${file}" 2>&1) || CASE_RC=$?
	if [[ "${CASE_RC}" != "${want_rc}" ]]; then
		echo "FAIL: ${name}: exit ${CASE_RC}, want ${want_rc}" >&2
		echo "  dockerfile: ${file}" >&2
		echo "  output:     ${CASE_OUT}" >&2
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

write_df() {
	local file="$1" body="$2"
	printf '%s\n' "${body}" >"${file}"
	printf '%s' "${file}"
}

# 1. The shape `opm generate dockerfile -i <pin> -b <pin>` emits: both stages
#    pinned, an entrypoint, and a CMD that serves. This must be accepted, or the
#    guard blocks the only correct image.
GOOD="${TMP}/good.Dockerfile"
write_df "${GOOD}" "FROM ${BASE} as builder
ADD configs /configs
RUN [\"/bin/opm\", \"serve\", \"/configs\", \"--cache-dir=/tmp/cache\", \"--cache-only\"]

FROM ${BASE}
ENTRYPOINT [\"/bin/opm\"]
CMD [\"serve\", \"/configs\", \"--cache-dir=/tmp/cache\"]
COPY --from=builder /configs /configs" >/dev/null
expect_dockerfile "fully pinned catalog Dockerfile is accepted" 0 "${GOOD}"

# 2. The 2026-10-31 failure: pinned base, no CMD. The catalog pod runs `opm`
#    with no arguments and exits 0 with empty logs.
NO_CMD="${TMP}/no-cmd.Dockerfile"
write_df "${NO_CMD}" "FROM ${BASE}
ENTRYPOINT [\"/bin/opm\"]
COPY configs /configs" >/dev/null
expect_dockerfile "missing CMD is rejected" 1 "${NO_CMD}"
expect_says "missing CMD names the failure mode" "CrashLoopBackOffs with empty logs"
expect_says "missing CMD says how to fix it" 'add CMD ["serve", "/configs"]'

# 3. A CMD that runs something other than serve leaves no catalog on :50051.
NOT_SERVING="${TMP}/not-serving.Dockerfile"
write_df "${NOT_SERVING}" "FROM ${BASE}
ENTRYPOINT [\"/bin/opm\"]
CMD [\"--help\"]
COPY configs /configs" >/dev/null
expect_dockerfile "CMD without serve is rejected" 1 "${NOT_SERVING}"

# 4. No ENTRYPOINT: whatever the base happens to run is what runs.
NO_ENTRYPOINT="${TMP}/no-entrypoint.Dockerfile"
write_df "${NO_ENTRYPOINT}" "FROM ${BASE}
CMD [\"serve\", \"/configs\"]
COPY configs /configs" >/dev/null
expect_dockerfile "missing ENTRYPOINT is rejected" 1 "${NO_ENTRYPOINT}"

# 5. The half-pinned file: -i pinned the final stage, the builder is still
#    :latest. It looks pinned when read quickly, and it is not.
LATEST_BUILDER="${TMP}/latest-builder.Dockerfile"
write_df "${LATEST_BUILDER}" "FROM quay.io/operator-framework/opm:latest as builder
FROM ${BASE}
ENTRYPOINT [\"/bin/opm\"]
CMD [\"serve\", \"/configs\"]
COPY --from=builder /configs /configs" >/dev/null
expect_dockerfile "unpinned builder stage is rejected" 1 "${LATEST_BUILDER}"
expect_says "unpinned builder names the stage" "'quay.io/operator-framework/opm:latest as builder'"
expect_says "unpinned builder explains -i and -b" "pass -i and -b with the same digest"

# 6. A tag instead of a digest: opm:latest can move under a rebuilt image.
LATEST_FINAL="${TMP}/latest-final.Dockerfile"
write_df "${LATEST_FINAL}" "FROM quay.io/operator-framework/opm:latest
ENTRYPOINT [\"/bin/opm\"]
CMD [\"serve\", \"/configs\"]
COPY configs /configs" >/dev/null
expect_dockerfile "unpinned final stage is rejected" 1 "${LATEST_FINAL}"
expect_says "unpinned final stage says why" "can move under a rebuilt image"

# 7. No FROM at all is a malformed Dockerfile, not a passing one.
NO_FROM="${TMP}/no-from.Dockerfile"
write_df "${NO_FROM}" "ENTRYPOINT [\"/bin/opm\"]
CMD [\"serve\", \"/configs\"]" >/dev/null
expect_dockerfile "Dockerfile without FROM is rejected" 1 "${NO_FROM}"
expect_says "missing FROM is named" "has no FROM line"

# 8. --check-dockerfile with nothing to check is a usage error, not a pass. It
#    must not reach for podman to generate one.
CASE_RC=0
CASE_OUT=$("${SCRIPT}" "${CONFIGS}" --check-dockerfile 2>&1) || CASE_RC=$?
[[ "${CASE_RC}" == 2 ]] || {
	echo "FAIL: --check-dockerfile with no --dockerfile: exit ${CASE_RC}, want 2" >&2
	echo "  output: ${CASE_OUT}" >&2
	exit 1
}
echo "ok: --check-dockerfile without --dockerfile is a usage error (exit 2)"

# 9. A missing Dockerfile is a usage error too, and names the path.
CASE_RC=0
CASE_OUT=$("${SCRIPT}" "${CONFIGS}" --check-dockerfile --dockerfile "${TMP}/absent.Dockerfile" 2>&1) || CASE_RC=$?
[[ "${CASE_RC}" == 2 ]] || {
	echo "FAIL: missing --dockerfile: exit ${CASE_RC}, want 2" >&2
	echo "  output: ${CASE_OUT}" >&2
	exit 1
}
echo "ok: missing Dockerfile is a usage error (exit 2)"

# 10. The pinned image is itself a digest reference. A tag or a floating ref
#     there would unpin every build the script makes.
CASE_RC=0
CASE_OUT=$(OPM_IMAGE="quay.io/operator-framework/opm:latest" "${SCRIPT}" "${CONFIGS}" --dockerfile "${GOOD}" "localhost/selftest:1" 2>&1) || CASE_RC=$?
[[ "${CASE_RC}" == 2 ]] || {
	echo "FAIL: floating OPM_IMAGE: exit ${CASE_RC}, want 2" >&2
	echo "  output: ${CASE_OUT}" >&2
	exit 1
}
echo "ok: a floating OPM_IMAGE is refused (exit 2)"

# 11. The checks must not need a container runtime: they are the cheap step a
#     long build is preceded by, and the CI job that pins them has no podman.
CASE_RC=0
CASE_OUT=$(CONTAINER_TOOL="${TMP}/no-such-runtime" "${SCRIPT}" "${CONFIGS}" --check-dockerfile --dockerfile "${GOOD}" 2>&1) || CASE_RC=$?
[[ "${CASE_RC}" == 0 ]] || {
	echo "FAIL: the Dockerfile checks reached for a container runtime: exit ${CASE_RC}" >&2
	echo "  output: ${CASE_OUT}" >&2
	exit 1
}
echo "ok: the Dockerfile checks need no container runtime"

CASE_RC=0
CASE_OUT=$(CONTAINER_TOOL="${TMP}/no-such-runtime" "${SCRIPT}" "${CONFIGS}" --check-dockerfile --dockerfile "${NO_CMD}" 2>&1) || CASE_RC=$?
[[ "${CASE_RC}" == 1 ]] || {
	echo "FAIL: a bad Dockerfile without a runtime: exit ${CASE_RC}, want 1" >&2
	echo "  output: ${CASE_OUT}" >&2
	exit 1
}
echo "ok: a bad Dockerfile is rejected without a container runtime"

# 12. --help must print the whole usage block, not just its first line.
CASE_RC=0
CASE_OUT=$("${SCRIPT}" --help 2>&1) || CASE_RC=$?
[[ "${CASE_RC}" == 0 ]] || {
	echo "FAIL: --help: exit ${CASE_RC}, want 0" >&2
	exit 1
}
[[ "${CASE_OUT}" == *"--check-dockerfile"* ]] || {
	echo "FAIL: --help does not list the flags: ${CASE_OUT}" >&2
	exit 1
}
echo "ok: --help prints the usage block"

echo "build-index-image selftest: all cases pass"
