#!/usr/bin/env bash

# Copyright 2026 Red Hat Inc.
# SPDX-License-Identifier: Apache-2.0
#
# verify-bundle-drift.sh - detect (and refresh) drift between config/ and bundle/.
#
# Why this exists
# ---------------
# `bundle/` is regenerated at release time only, from a release branch, with
# `make bundle`. Branch builds of the operator are therefore installed under a
# bundle that can LACK permissions and CRD fields the branch code requires. The
# result is silent: the CSV reports Succeeded, the pod runs, and the install
# merely produces `forbidden` log lines and an empty CR status.
#
# CI already guards `config/` (make verify-manifests). Nothing guarded `bundle/`
# until this script.
#
# What is compared (subset, not equality)
# ---------------------------------------
#   * config/rbac/role.yaml         -> CSV spec.install.spec.clusterPermissions
#                                      CSV spec.install.spec.permissions
#   * config/crd/bases/*.yaml       -> bundle/manifests/<same file name>
#
# A subset check, deliberately:
#   * the CSV's namespaced `permissions` is the UNION of role.yaml's Role plus
#     leader-election and auth-proxy Roles, so equality is impossible;
#   * the bundle is edited by hand at release time (CSV description, "what's
#     new" text, IMAGE_SHA, VERSION, com.redhat.openshift.versions), so a
#     `make bundle && git diff --exit-code` gate would be red permanently and
#     would be muted. What must never happen is the bundle GRANTING LESS THAN
#     config/ requires, or LACKING a field config/ defines.
#
# Usage
# -----
#   hack/verify-bundle-drift.sh              # same as --check
#   hack/verify-bundle-drift.sh --check      # exit 1 on drift, listing it
#   hack/verify-bundle-drift.sh --write      # regenerate the affected bundle
#                                            # subtrees from config/, then
#                                            # re-check; leaves unrelated
#                                            # release-time text untouched
#   hack/verify-bundle-drift.sh --verbose    # also list what matched
#
# Env: BUNDLE_DRIFT_ADVISORY=true turns GitHub annotations into warnings
#      (used by CI when the change cannot block a PR).
#
# Exit: 0 clean, 1 drift (check mode), 2 tooling/usage error.

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

if ! command -v python3 >/dev/null 2>&1; then
	echo "verify-bundle-drift: python3 is required" >&2
	exit 2
fi

python3 - "${SCRIPT_ROOT}" "$@" <<'PY'
import json
import os
import re
import sys

try:
    import yaml
except ImportError:
    sys.stderr.write(
        "verify-bundle-drift: PyYAML is required "
        "(python3 -m pip install --user pyyaml)\n"
    )
    sys.exit(2)

REPO = sys.argv[1]
ARGV = sys.argv[2:]

MODE = "check"
VERBOSE = False

for arg in ARGV:
    if arg in ("--check",):
        MODE = "check"
    elif arg in ("--write",):
        MODE = "write"
    elif arg in ("-v", "--verbose"):
        VERBOSE = True
    elif arg in ("-h", "--help"):
        print(__doc__ or "usage: verify-bundle-drift.sh [--check|--write] [--verbose]")
        sys.exit(0)
    else:
        sys.stderr.write("verify-bundle-drift: unknown argument: %s\n" % arg)
        sys.exit(2)

ADVISORY = os.environ.get("BUNDLE_DRIFT_ADVISORY", "false").lower() == "true"

RBAC_PATH = os.path.join(REPO, "config/rbac/role.yaml")
CRD_DIR = os.path.join(REPO, "config/crd/bases")
BUNDLE_DIR = os.path.join(REPO, "bundle/manifests")


# --------------------------------------------------------------------------
# loading
# --------------------------------------------------------------------------
def load_docs(path):
    with open(path, encoding="utf-8") as handle:
        return [doc for doc in yaml.safe_load_all(handle) if doc]


def read_lines(path):
    with open(path, encoding="utf-8") as handle:
        return handle.read().splitlines(True)


def bundle_paths():
    csvs = []
    crds = []
    if not os.path.isdir(BUNDLE_DIR):
        return None, []
    for name in sorted(os.listdir(BUNDLE_DIR)):
        if not name.endswith((".yaml", ".yml")):
            continue
        full = os.path.join(BUNDLE_DIR, name)
        with open(full, encoding="utf-8") as handle:
            head = handle.read(4096)
        if "kind: ClusterServiceVersion" in head:
            csvs.append(full)
        elif "kind: CustomResourceDefinition" in head:
            crds.append(full)
    return (csvs[0] if csvs else None), crds


def config_crds():
    if not os.path.isdir(CRD_DIR):
        return []
    return [
        os.path.join(CRD_DIR, name)
        for name in sorted(os.listdir(CRD_DIR))
        if name.endswith((".yaml", ".yml"))
    ]


# --------------------------------------------------------------------------
# RBAC comparison
# --------------------------------------------------------------------------
def triples(rules):
    """Flatten rules to {(apiGroup, resource, verb, resourceNames)}."""
    out = set()
    for rule in rules or []:
        names = tuple(sorted(rule.get("resourceNames") or ()))
        for group in rule.get("apiGroups") or [""]:
            for resource in rule.get("resources") or []:
                for verb in rule.get("verbs") or []:
                    out.add((group, resource, verb, names))
    return out


def uncovered(required, granted):
    unrestricted = {(g, r, v) for (g, r, v, names) in granted if not names}
    missing = []
    for (g, r, v, names) in sorted(required):
        if (g, r, v) in unrestricted:
            continue
        if (g, r, v, names) in granted:
            continue
        missing.append((g, r, v, names))
    return missing


def format_missing(missing, scope):
    grouped = {}
    for (group, resource, verb, names) in missing:
        key = (group, resource, names)
        grouped.setdefault(key, set()).add(verb)
    lines = []
    for (group, resource, names) in sorted(grouped):
        label = "%s/%s" % (group or "core", resource)
        if names:
            label += " [resourceNames=%s]" % ",".join(names)
        lines.append(
            "%s: %s grants no %s"
            % (scope, label, ",".join(sorted(grouped[(group, resource, names)])))
        )
    return lines


def check_rbac(csv_doc, verbose):
    problems = []
    config_docs = load_docs(RBAC_PATH)
    install = ((csv_doc.get("spec") or {}).get("install") or {}).get("spec") or {}

    cluster_required = set()
    namespaced_required = set()
    for doc in config_docs:
        kind = doc.get("kind")
        if kind == "ClusterRole":
            cluster_required |= triples(doc.get("rules"))
        elif kind == "Role":
            namespaced_required |= triples(doc.get("rules"))

    cluster_granted = set()
    for entry in install.get("clusterPermissions") or []:
        cluster_granted |= triples(entry.get("rules"))
    namespaced_granted = set()
    for entry in install.get("permissions") or []:
        namespaced_granted |= triples(entry.get("rules"))

    problems += format_missing(
        uncovered(cluster_required, cluster_granted), "clusterPermissions"
    )
    problems += format_missing(
        uncovered(namespaced_required, namespaced_granted), "permissions"
    )

    if verbose and not problems:
        print(
            "  rbac: %d cluster rule(s) and %d namespaced rule(s) covered"
            % (len(cluster_required), len(namespaced_required))
        )
    return problems


# --------------------------------------------------------------------------
# CRD comparison
# --------------------------------------------------------------------------
def schema_props(node, path=""):
    out = {}
    if not isinstance(node, dict):
        return out
    for name, sub in (node.get("properties") or {}).items():
        child = "%s.%s" % (path, name) if path else name
        out[child] = sub
        out.update(schema_props(sub, child))
    items = node.get("items")
    if isinstance(items, dict):
        out.update(schema_props(items, "%s[]" % path))
    return out


def versions_by_name(doc):
    out = {}
    for version in (doc.get("spec") or {}).get("versions") or []:
        out[version.get("name")] = version
    return out


def check_crd(config_path, bundle_path, verbose):
    problems = []
    label = os.path.basename(config_path)
    config_doc = load_docs(config_path)[0]
    bundle_doc = load_docs(bundle_path)[0]

    for field in ("scope",):
        want = (config_doc.get("spec") or {}).get(field)
        have = (bundle_doc.get("spec") or {}).get(field)
        if want != have:
            problems.append("%s: spec.%s is %r, config/ has %r" % (label, field, have, want))

    config_versions = versions_by_name(config_doc)
    bundle_versions = versions_by_name(bundle_doc)

    for name in sorted(config_versions):
        if name not in bundle_versions:
            problems.append("%s: version %s is missing from the bundle" % (label, name))
            continue

        config_schema = (
            (config_versions[name].get("schema") or {}).get("openAPIV3Schema") or {}
        )
        bundle_schema = (
            (bundle_versions[name].get("schema") or {}).get("openAPIV3Schema") or {}
        )
        config_props = schema_props(config_schema)
        bundle_props = schema_props(bundle_schema)

        missing = [path for path in sorted(config_props) if path not in bundle_props]
        for path in missing:
            problems.append(
                "%s: field %s (%s) is in config/ but not in the bundle"
                % (label, path, name)
            )

        for path in sorted(config_props):
            if path not in bundle_props:
                continue
            want_type = (config_props[path] or {}).get("type")
            have_type = (bundle_props[path] or {}).get("type")
            if want_type and have_type and want_type != have_type:
                problems.append(
                    "%s: field %s has type %s in the bundle, config/ says %s"
                    % (label, path, have_type, want_type)
                )
            want_required = set((config_props[path] or {}).get("required") or [])
            have_required = set((bundle_props[path] or {}).get("required") or [])
            for dropped in sorted(want_required - have_required):
                problems.append(
                    "%s: field %s no longer marks %s as required"
                    % (label, path or "<root>", dropped)
                )

        if verbose and not problems:
            print("  crd %s (%s): %d field(s) covered" % (label, name, len(config_props)))

    return problems


def check_all(verbose=False):
    csv_path, crd_paths = bundle_paths()
    problems = []

    if csv_path is None:
        problems.append("bundle/manifests: no ClusterServiceVersion found")
    else:
        problems += check_rbac(load_docs(csv_path)[0], verbose)

    bundle_crd_names = {os.path.basename(path) for path in crd_paths}
    for config_path in config_crds():
        name = os.path.basename(config_path)
        if name not in bundle_crd_names:
            problems.append("bundle/manifests: CRD %s is missing" % name)
            continue
        problems += check_crd(config_path, os.path.join(BUNDLE_DIR, name), verbose)

    return problems


# --------------------------------------------------------------------------
# --write: splice regenerated subtrees into the existing bundle files
# --------------------------------------------------------------------------
def toplevel_span(lines, key):
    pattern = re.compile(r"^%s:\s*$" % re.escape(key))
    start = None
    for index, line in enumerate(lines):
        if pattern.match(line):
            start = index
            break
    if start is None:
        return None
    end = len(lines)
    for index in range(start + 1, len(lines)):
        if re.match(r"^[A-Za-z_]", lines[index]):
            end = index
            break
    return start, end


def yscalar(value):
    text = str(value)
    if text == "":
        return '""'
    reserved = {"true", "false", "null", "yes", "no", "on", "off", "~"}
    if re.fullmatch(r"[A-Za-z0-9_./-]+", text) and text.lower() not in reserved:
        return text
    return json.dumps(text)


def render_rules(rules, base):
    lines = []
    for rule in rules or []:
        keys = [k for k in ("apiGroups", "resources", "verbs", "resourceNames") if k in rule]
        for position, key in enumerate(keys):
            prefix = "%s- " % base if position == 0 else "%s  " % base
            lines.append("%s%s:\n" % (prefix, key))
            for value in rule[key]:
                lines.append("%s    - %s\n" % (base, yscalar(value)))
    return lines


def splice_rules_blocks(lines, key, extra_rules):
    """Append `extra_rules` to the rules list of the single entry under `key:`.

    Refuses to guess when there is more than one entry (different service
    accounts would get the wrong grants); that case needs a release cut.
    """
    key_line = re.compile(r"^      %s:\s*$" % re.escape(key))
    start = None
    for index, line in enumerate(lines):
        if key_line.match(line):
            start = index
            break
    if start is None:
        return lines, False

    end = len(lines)
    for index in range(start + 1, len(lines)):
        if re.match(r"^ {0,6}[A-Za-z_]", lines[index]):
            end = index
            break

    entries = [
        index
        for index in range(start + 1, end)
        if re.match(r"^ {8}- ", lines[index])
    ]
    if len(entries) != 1:
        raise SystemExit(
            "verify-bundle-drift: %s: expected exactly one entry under %r, found %d; "
            "refusing to guess which service account to grant"
            % (rel(BUNDLE_CSV_PATH), key, len(entries))
        )

    # operator-sdk emits the rules list inline on the entry's first line:
    #         - rules:
    #             - apiGroups:
    entry_start = entries[0]
    if not re.match(r"^ {8}- rules:\s*$", lines[entry_start]):
        raise SystemExit(
            "verify-bundle-drift: %s: entry under %r does not start with 'rules:'; "
            "refusing to rewrite" % (rel(BUNDLE_CSV_PATH), key)
        )

    content_end = end
    for index in range(entry_start + 1, end):
        if re.match(r"^ {10}[A-Za-z_]", lines[index]):
            content_end = index
            break

    rendered = render_rules(extra_rules, " " * 12)
    return lines[:content_end] + rendered + lines[content_end:], True


CONFIG_RULES = {}
BUNDLE_CSV_PATH = ""


def write_csv(csv_path):
    global CONFIG_RULES, BUNDLE_CSV_PATH
    BUNDLE_CSV_PATH = csv_path

    cluster_rules = []
    namespaced_rules = []
    for doc in load_docs(RBAC_PATH):
        if doc.get("kind") == "ClusterRole":
            cluster_rules += doc.get("rules") or []
        elif doc.get("kind") == "Role":
            namespaced_rules += doc.get("rules") or []
    CONFIG_RULES = {"clusterPermissions": cluster_rules, "permissions": namespaced_rules}

    lines = read_lines(csv_path)
    before = load_docs_text("".join(lines))[0]
    install = ((before.get("spec") or {}).get("install") or {}).get("spec") or {}

    added = 0
    for key in ("clusterPermissions", "permissions"):
        entries = install.get(key) or []
        existing = set()
        for entry in entries:
            existing |= triples(entry.get("rules"))
        extra = appended_rules(triples(CONFIG_RULES[key]), existing)
        if not extra:
            continue
        lines, changed = splice_rules_blocks(lines, key, extra)
        if not changed:
            raise SystemExit(
                "verify-bundle-drift: %s: could not locate rules under %r; "
                "refusing to rewrite" % (csv_path, key)
            )
        added += len(extra)

    if not added:
        return

    text = "".join(lines)
    after = load_docs_text(text)[0]

    if mask_csv(before) != mask_csv(after):
        raise SystemExit(
            "verify-bundle-drift: %s: refusing to write, splice would change "
            "unrelated content" % csv_path
        )

    with open(csv_path, "w", encoding="utf-8") as handle:
        handle.write(text)
    print("added %d rule(s) to %s (clusterPermissions, permissions)" % (added, rel(csv_path)))


def appended_rules(required, existing):
    """Rules to ADD so that `required` is covered; never proposes removals.

    Dropping a grant is a release-cut decision (`make bundle`), not something a
    drift fix should do behind the reviewer's back.
    """
    per_resource = {}
    for (group, resource, verb, names) in uncovered(required, existing):
        per_resource.setdefault((group, resource, names), set()).add(verb)

    buckets = {}
    for (group, resource, names), verbs in per_resource.items():
        buckets.setdefault((group, names, frozenset(verbs)), []).append(resource)

    rules = []
    for (group, names, verbs) in sorted(
        buckets, key=lambda k: (k[0], tuple(sorted(k[2])), k[1])
    ):
        rule = {
            "apiGroups": [group],
            "resources": sorted(buckets[(group, names, verbs)]),
            "verbs": sorted(verbs),
        }
        if names:
            rule["resourceNames"] = list(names)
        rules.append(rule)
    return rules


def write_crd(config_path, bundle_path):
    config_lines = read_lines(config_path)
    bundle_lines = read_lines(bundle_path)

    config_span = toplevel_span(config_lines, "spec")
    bundle_span = toplevel_span(bundle_lines, "spec")
    if config_span is None or bundle_span is None:
        raise SystemExit(
            "verify-bundle-drift: %s: no top-level spec: block to splice" % bundle_path
        )

    config_block = config_lines[config_span[0] : config_span[1]]
    if config_block and not config_block[-1].endswith("\n"):
        config_block[-1] += "\n"

    lines = bundle_lines[: bundle_span[0]] + config_block + bundle_lines[bundle_span[1] :]
    text = "".join(lines)

    before = load_docs_text("".join(bundle_lines))[0]
    after = load_docs_text(text)[0]
    masked_before = {k: v for k, v in before.items() if k != "spec"}
    masked_after = {k: v for k, v in after.items() if k != "spec"}
    if masked_before != masked_after:
        raise SystemExit(
            "verify-bundle-drift: %s: refusing to write, splice would change "
            "metadata/status" % bundle_path
        )

    with open(bundle_path, "w", encoding="utf-8") as handle:
        handle.write(text)
    print("rewrote %s (spec)" % rel(bundle_path))


def mask_csv(doc):
    masked = json.loads(json.dumps(doc))
    install = ((masked.get("spec") or {}).get("install") or {}).get("spec") or {}
    for key in ("clusterPermissions", "permissions"):
        for entry in install.get(key) or []:
            entry["rules"] = []
    return masked


def load_docs_text(text):
    return [doc for doc in yaml.safe_load_all(text) if doc]


def rel(path):
    return os.path.relpath(path, REPO)


# --------------------------------------------------------------------------
# main
# --------------------------------------------------------------------------
def main():
    if not os.path.isfile(RBAC_PATH):
        sys.stderr.write("verify-bundle-drift: missing %s\n" % rel(RBAC_PATH))
        return 2

    if MODE == "write":
        csv_path, crd_paths = bundle_paths()
        if csv_path is None:
            sys.stderr.write("verify-bundle-drift: no ClusterServiceVersion in bundle/manifests\n")
            return 2
        write_csv(csv_path)
        bundle_crd_names = {os.path.basename(path) for path in crd_paths}
        for config_path in config_crds():
            name = os.path.basename(config_path)
            if name not in bundle_crd_names:
                sys.stderr.write(
                    "verify-bundle-drift: CRD %s missing from bundle/manifests; "
                    "copy it there from config/crd/bases and re-run --write\n" % name
                )
                return 2
            write_crd(config_path, os.path.join(BUNDLE_DIR, name))

    problems = check_all(verbose=VERBOSE)
    if not problems:
        print("bundle/manifests is not missing anything config/ requires.")
        return 0

    print("bundle/manifests lacks %d thing(s) that config/ requires:" % len(problems))
    for problem in problems:
        print("  - %s" % problem)

    if os.environ.get("GITHUB_ACTIONS") == "true":
        level = "warning" if ADVISORY else "error"
        for problem in problems:
            print("::%s title=Bundle drift::%s" % (level, problem.replace("%", "%25").replace("\n", "%0A")))

    print("")
    print("Fix: hack/verify-bundle-drift.sh --write, then review with git diff")
    print("     (a release cut regenerates the whole bundle with `make bundle`)")
    return 1


sys.exit(main())
PY
