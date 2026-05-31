# Partial Prometheus Query Failure Handling

**Audience:** koku-metrics-operator maintainers (David and team)  
**Status:** Design proposal — not implemented  
**Priority:** P3 (resilience optimization; not blocking VM recommendations)

## 1. Problem Statement

The operator collects metrics via multiple sequential Prometheus queries per time window. Today, if **any** query in a batch still fails after built-in retries (timeout, 503, network error), the **entire** collection pass for that window is treated as failed: no success timestamp is recorded, and the window is attempted again on a later reconciliation (5-minute requeue).

This behavior applies to **all** metric types (node, pod, storage, namespace, GPU, VM)—not only VM metrics.

## 2. How It Works Today

### Reconciliation and time windows

- The controller reconciles every **5 minutes** (`RequeueAfter: 5 * time.Minute` in `costmanagementmetricsconfig_controller.go`).
- For each reconciliation, `collectPromStats` processes one or more **hourly** Prometheus ranges (`promv1.Range`), stepping backward from the current hour as needed.
- VM metrics for ROS/cost use **four 15-minute sub-windows** per hour (`vmQuarterHours = 4` in `vm_collector.go`).

### Query execution

Two shared helpers execute queries sequentially:

| Helper | API | Used for |
|--------|-----|----------|
| `getQueryRangeResults` | `QueryRange` | Hourly cost metrics (node, pod, storage, namespace, VM range, GPU) |
| `getQueryResults` | `Query` (instant) | ROS container/namespace/quota and **15-minute ROS VM** samples |

Both live in `internal/collector/prometheus.go`.

**Per-query retry (already exists):** When a query fails and `retries > 0`, the query is queued for a later retry pass with exponential backoff (1s, 2s, 4s, …). Other queries in the same pass continue. `MaxRetries` defaults to **5** (`prometheus.go`).

**Window-level failure:** After retries are exhausted, if **any** query still fails, the helper returns an error. The caller does not write reports for that metric group (and `GenerateReports` aborts the overall hour).

**No cross-reconciliation cache:** Successful query results are held only in memory for the current attempt. On the next reconciliation, **all** queries for that window run again—even those that succeeded before the batch failed.

### Call chain (hourly window)

```
Reconcile (5 min)
  └── collectPromStats (controller/prometheus.go)
        └── collector.GenerateReports (collector/collector.go)
              ├── getQueryRangeResults(nodeQueries)     → 6 queries
              ├── generateCostManagementReports
              │     ├── getQueryRangeResults(podQueries)      → 7 queries
              │     ├── getQueryRangeResults(volQueries)      → 6 queries
              │     ├── getQueryRangeResults(namespaceQueries)→ 1 query
              │     └── getQueryRangeResults(gpu …)           → multiple batches
              └── VM path (when KubeVirt detected)
                    └── collectVMQuarterHour × 4
                          └── getQueryResults(rosVMQueries)   → 14 queries
```

### Controller retry for a failed window

`collectPromStats` increments `retryTracker[timeRange.Start]` on failure. `isQueryNeeded` allows up to **5** attempts per window start time across reconciliations; after that, the window is skipped (logged as "query retry limit exceeded"). `LastQuerySuccessTime` is only advanced on full success.

### Query counts (current branch)

| Query set | File | Count | Notes |
|-----------|------|-------|-------|
| `nodeQueries` | `queries.go` ~210–267 | 6 | Gate for entire hour (queried first) |
| `podQueries` | `queries.go` ~321–395 | 7 | |
| `volQueries` | `queries.go` ~268–320 | 6 | |
| `namespaceQueries` | `queries.go` ~558–566 | 1 | |
| `vmQueries` | `queries.go` ~396–557 | 11 | Legacy hourly range path (`generateCostVMMetricsReport`) |
| `rosVMQueries` | `vm_ros_queries.go` ~11–166 | **14** | 15-minute ROS + cost aggregation path |

**ROS VM 14 queries** include guest-agent style metrics (queries 6, 8, 9 in the logical ordering):

- `vm-ros-memory-available-kib` → `kubevirt_vmi_memory_available_bytes`
- `vm-ros-filesystem-used-bytes` → `kubevirt_vmi_filesystem_used_bytes`
- `vm-ros-filesystem-capacity-bytes` → `kubevirt_vmi_filesystem_capacity_bytes`

### Empty results vs errors

**Empty results are not failures.** A query that returns no series (e.g., no VMs with guest agent) is handled normally; fields stay empty/nil in CSV output. This design applies only to **Prometheus HTTP/client errors** and non-matrix/vector type errors.

## 3. Impact

### Normal operation

Minimal impact. Prometheus is usually reliable; retrying on the next 5-minute reconcile almost always succeeds.

### Under load or cluster stress

During OpenShift upgrades, Prometheus overload, or network blips:

- A single slow or failing query causes the **whole batch** for that window to fail after retries, discarding in-memory results from sibling queries that already succeeded.
- The operator may retry the same window up to five times across reconciliations, re-executing **all** queries each time.
- **VM path is heavier:** 14 instant queries × 4 quarter-hours per hour, plus 11 range queries if the legacy cost VM path runs. One timeout on `vm-ros-filesystem-used-bytes` fails that 15-minute slice and blocks marking the hour successful—even if CPU/memory queries succeeded.
- **Pod path:** 7 queries vs VM’s 14; same all-or-nothing semantics.

### Distinction from “partial data”

The operator already supports **partial rows** when metrics are missing from Prometheus (nil/empty columns). The gap is **partial query success** when Prometheus returns errors for some queries only.

## 4. Proposed Solution: Partial-Success Semantics

### Option A: Required vs optional queries (recommended for VM)

Categorize each query:

- **Required:** CPU usage, CPU request, memory usage, memory request—without these, the row is not useful for ROS/cost.
- **Optional:** Guest-agent metrics (`memory_available_kib`, filesystem used/capacity), disk I/O rates, etc.—failure logs a warning; row is written with empty fields.

```go
type query struct {
    Name     string
    // ... existing fields ...
    Required bool // if false, Prometheus errors do not fail the window
}
```

- Optional failure: log warning, continue, write partial row.
- Required failure: abort window (current behavior).

`ros-ocp-backend` already tolerates nil/empty optional fields.

### Option B: Per-query retry with backoff (partially exists)

`getQueryRangeResults` / `getQueryResults` already retry failed queries up to `MaxRetries` with exponential backoff within a single collection attempt.

**Enhancement:** Tune backoff, add jitter, or increase retries for heavy queries—helps all metric types with minimal structural change. Does not help if Prometheus is down for longer than the retry window.

### Option C: Cache successful query results per window

Persist which queries succeeded for `(windowStart, queryName)` in memory (or on disk). On retry, skip completed queries.

- **Pros:** Avoids re-running expensive queries.
- **Cons:** Stateful collector; must handle process restarts and memory growth; more complex than Option A.

### Combined approach

Retry as today (Option B), then apply Option A for queries still failing—especially optional VM guest-agent metrics.

## 5. Recommendation

1. **Option A** for `rosVMQueries` first—optional metrics align with product semantics (guest agent not installed everywhere).
2. **Option B** tuning globally if Prometheus latency is the main issue.
3. **Option C** only if profiling shows retry storms dominate cost.

## 6. Affected Code Paths

| Location | Role | Lines (approx.) |
|----------|------|-----------------|
| `internal/collector/prometheus.go` | `getQueryRangeResults` — sequential `QueryRange`, per-query retry, final error | 214–251 |
| `internal/collector/prometheus.go` | `getQueryResults` — sequential `Query`, same pattern | 253–293 |
| `internal/collector/prometheus.go` | `MaxRetries` default | 32 |
| `internal/collector/collector.go` | `GenerateReports` — orchestrates hourly collection | 197–295 |
| `internal/collector/collector.go` | `generateCostPodMetricsReport`, `generateCostStorageMetricsReport`, etc. — abort on `getQueryRangeResults` error | 408–556 |
| `internal/collector/vm_collector.go` | `collectVMQuarterHour` — `getQueryResults(rosVMQueries, …)` | 47–95 |
| `internal/collector/vm_ros_queries.go` | `rosVMQueries` definition (14 queries) | 11–166 |
| `internal/collector/queries.go` | `vmQueries` (11 range queries), `podQueries`, `nodeQueries`, … | 210–557 |
| `internal/controller/prometheus.go` | `collectPromStats` — calls `GenerateReports`, sets status | 162–212 |
| `internal/controller/prometheus.go` | `isQueryNeeded` — cross-reconcile retry limit (5) | 143–159 |
| `internal/controller/costmanagementmetricsconfig_controller.go` | `RequeueAfter: 5 * time.Minute` | ~891–893 |

**Tests today:**

- `internal/collector/prometheus_test.go` — `TestGetQueryRangeResultsSuccess`, `TestGetQueryRangeResultsError`
- `internal/collector/collector_test.go` — `TestGenerateReportsQueryErrors` (expects full failure per metric group)

## 7. Testing Considerations

1. **Unit:** Mock `PrometheusConnection`; fail one optional ROS VM query; assert CSV written with empty guest-agent columns and no error from `collectVMQuarterHour`.
2. **Unit:** Fail a required query (e.g., `vm-ros-cpu-usage-mc`); assert error returned and no partial hour success.
3. **Unit:** Extend `TestGetQueryRangeResultsError` for optional-query semantics once `Required` exists.
4. **Integration (optional):** Artificial delay or error injection on selected Prometheus rules—validate operator progress under degraded Prometheus.

## 8. Scope and Priority

- **Not blocking** VM recommendations or cost ingestion—the current all-or-retry behavior is **correct** (no silent data loss).
- **Benefit:** Less wasted Prometheus load and faster recovery under degradation; applies to pods, nodes, storage, GPU, and VMs.
- **Suggested priority:** **P3** — nice-to-have for a future quarter.

## References

- Architecture overview: `docs/architecture.md`
- VM report fields: `docs/report-fields-description.md`
- ROS VM query list test: `internal/collector/vm_ros_queries_test.go`
