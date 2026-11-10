# #829 KMO Split-Part Recovery — Implementation Plan

> **For agentic workers:** superpowers:test-driven-development governs every step.
> Owner approved the mechanism 2026-11-10: **root-based classification** —
> a staged part is stale iff its ROOT original (base with the entire
> trailing digit run stripped) is present in the same uid group; root
> absent ⟹ the split completed (the splitter removes the original only
> after the full contiguous part run) ⟹ the parts are the report's only
> representation and are ALL recovered. No archive-member naming change →
> zero consumer impact (the rename-parts alternative was rejected: a
> cross-repo archive-contract change for no need).

**Repo:** koku-metrics-operator, branch `pgarciaq-rosocp-superpowers-phase17`,
HEAD `fad195d9` (the issue's pinned commit; #766/#767 recovery code in).
Tracked centrally as ros-ocp-backend #829. No overlap with the parked
#764/#768 side branches (verified: zero diff in internal/packaging/).

**Recon facts the design rests on (all verified in code):**
- `PackageReports` (packaging.go:706) generates a FRESH uid per call; all
  reports staged in one pass share it. Old-uid leftovers are stranded
  lineages.
- `supersedeStagedArtifacts(sourceName)` (per fresh source, upstream of
  recovery) deletes staged entries of ANY uid whose base is the source or
  a part of it — old-uid artifacts die when the source is re-collected.
- `writePart` names parts `<stem><n>.csv`, n ≥ 1 sequential, NO padding —
  part 10 of "report" is "report10.csv", which is ALSO "report1"+"0":
  the intrinsic ambiguity the current pairwise loop resolves wrongly.
- `splitFiles` removes the original only AFTER the full part run
  (packaging.go:512-527): original-present ⟹ interrupted mid-split
  (parts stale); original-absent ⟹ completed split (parts are the data).
- On retry, `p.maxBytes` resets from the CR (100MB in fixtures) —
  recovered small parts are NOT re-split (the executed proof confirms).
  Re-split only occurs if MaxSize shrank between passes; nested part
  names remain classifiable by the same root-strip rule. Non-issue.
- The #766 test family (8 tests in packaging_recovery_test.go) is the
  regression base: StrandedOriginalSupersedesItsParts (same-uid
  original+part), SplitArtifactsSuperseded (old-uid parts + fresh
  source), DuplicateInventorySuperseded (json), etc.

**Mechanism (corrected after the fresh-context review — see below):**
1. `splitPartRoot(stagedBase)`: month-anchored. Every collector filename
   is `<type-prefix><YYYYMM>.csv` (18 construction sites, collector.go) —
   a trailing digit run of exactly six, a valid year-month. Parts append
   the part number to the FULL stem, so a part's run is always
   YYYYMM∥partnumber (≥ 7 digits) and the original's month is the run's
   first six digits. Root = stem with the run replaced by run[:6]. Run 6
   = original (never a part); run 0–5 or invalid month = nothing the
   product writes — left alone, never deleted.
2. The supersession loop in `moveOrCopyFiles`: uid-scoped present-bases
   set; delete f iff its root is present in the SAME uid group.
   `supersedeStagedArtifacts` and `isSplitPartOf` stay unchanged.
3. `splitFiles` NEVER re-splits a part (run ≥ 7): a part's re-split
   outputs are stem(part)+n — the deterministic names of its own
   siblings — and os.Create would truncate them (review C2: silent row
   loss). An oversized part (single huge row, or a shrunken MaxSize) is
   packaged as-is with a loud log — fail-visible over fail-silent
   (owner-approved 2026-11-10). NOTE: the first draft of this plan
   claimed re-splits were data-preserving; the review proved otherwise
   and this section corrects it.

**Review outcome (gpt-6.1-terra, all findings verified before action):**
- C1 (Critical): the first draft's full-run root-strip broke original+
  parts deletion for REAL month-suffixed names (root "…-.csv" never
  exists) → corrected to the month anchor above; #766 fixtures moved to
  real names; a month-boundary coexistence pin added ("…-202611" +
  "…-202612" both originals). Correction of my own claim: the OLD
  pairwise code did NOT delete month pairs (isSplitPartOf is a string
  prefix test; "202612" is not "202611"+"2") — the month-boundary test
  is a regression pin, not a pre-existing-hazard fix.
- C2 (Critical): re-split collision truncates siblings → the split-skip
  + the oversized-part trade-off (owner-approved).
- I3 (Important): the uid-scoping control didn't test uid scoping →
  replaced with a root-under-uid1 / part-under-uid2 control.
- I4 (Important, pre-existing): partial multi-tarball failure can
  duplicate data on retry → filed as #836 (separate issue, own recon);
  the multi-archive test helper added here is the tooling step.

**Tests:**
- RED: `TestPackageReports_CompletedSplitRetryRecoversAllParts` — the
  issue's Appendix D shape as a named test: 16 REAL parts via
  `p.splitFiles` (maxBytes=120), original removed by the splitter,
  `PackageReports` retry must succeed with every marker row exactly
  once across the archive and all parts recovered. Baseline: 9 of 16,
  row-30 absent (executed proof).
- Controls: multi-digit same-uid original+parts (all parts deleted,
  original packaged once); parts-only old uid + a different fresh
  report in the same pass (parts recovered, no cross-talk); the
  existing #766 family stays green.
- Mutations: classification reverted to pairwise (RED fails);
  root-strip only one digit (RED fails — part 10 read as part of part
  1); never-delete (the stranded-original control fails). Stash-rerun.

**Docs:** KMO has no CHANGELOG (verify); the recovery design is
documented in code comments — amend them. Design notes go to the issue
as comments per the tracking convention.

**Non-goals:** no part-naming change (cross-repo contract); no
`supersedeStagedArtifacts` change; no cluster E2E; I4 (partial
multi-tarball duplication) is #836's, not this issue's.
