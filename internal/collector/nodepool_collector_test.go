package collector

import (
	"reflect"
	"testing"

	gologr "github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/rest"
)

// stubNodepoolCollectorForGolden injects two fixture pools (live +
// scaled-to-zero) shaping the golden file. Restored via Cleanup.
// NoROS/NoEnabledROS tests keep the unstubbed empty path (no file).
func stubNodepoolCollectorForGolden(t *testing.T) {
	t.Helper()
	orig := collectNodepoolRowsForCollector
	collectNodepoolRowsForCollector = func(_ *rest.Config, _ gologr.Logger, windowStart, windowEnd, collectedAt string) ([]nodepoolRow, int, error) {
		return []nodepoolRow{
			{HCClusterID: "hc-golden-a", PoolName: "pool-live", WindowStart: windowStart, WindowEnd: windowEnd, SpecReplicas: 2, StatusReplicas: 2, CollectedAt: collectedAt, Source: "hypershift"},
			{HCClusterID: "hc-golden-a", PoolName: "pool-zero", WindowStart: windowStart, WindowEnd: windowEnd, CollectedAt: collectedAt, Source: "hypershift"},
		}, 0, nil
	}
	t.Cleanup(func() { collectNodepoolRowsForCollector = orig })
}

func nodepoolObj(ns, name, cluster string, specRep, statusRep int64, autoMin, autoMax *int64, omitStatus bool) unstructured.Unstructured {
	u := unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"namespace": ns, "name": name},
		"spec": map[string]interface{}{
			"clusterName": cluster,
			"replicas":    specRep,
		},
	}}
	if autoMin != nil {
		u.Object["spec"].(map[string]interface{})["autoScaling"] = map[string]interface{}{
			"min": *autoMin, "max": *autoMax,
		}
	}
	if !omitStatus {
		u.Object["status"] = map[string]interface{}{"replicas": statusRep}
	}
	return u
}

func TestBuildNodepoolRows(t *testing.T) {
	i64 := func(v int64) *int64 { return &v }
	hcs := map[string]string{"ns-a/hc-a": "hc-uuid-a"}
	pools := []unstructured.Unstructured{
		nodepoolObj("ns-a", "pool-live", "hc-a", 2, 2, nil, nil, false),
		nodepoolObj("ns-a", "pool-zero", "hc-a", 0, 0, nil, nil, false),
		nodepoolObj("ns-a", "pool-auto", "hc-a", 1, 1, i64(0), i64(3), false),
		nodepoolObj("ns-a", "pool-orphan", "hc-missing", 2, 2, nil, nil, false),
		nodepoolObj("ns-a", "pool-nostatus", "hc-a", 2, 0, nil, nil, true),
	}
	rows, skipped := buildNodepoolRows(pools, hcs, "ws", "we", "ca")
	if len(rows) != 3 {
		t.Fatalf("want 3 rows (live+zero+auto), got %d", len(rows))
	}
	if skipped != 2 {
		t.Fatalf("want 2 skipped (orphan/nostatus), got %d", skipped)
	}
	byName := map[string]nodepoolRow{}
	for _, r := range rows {
		byName[r.PoolName] = r
	}
	if byName["pool-live"].SpecReplicas != 2 {
		t.Errorf("pool-live spec = %d, want 2", byName["pool-live"].SpecReplicas)
	}
	if byName["pool-live"].Autoscaling != "" {
		t.Errorf("pool-live autoscaling = %q, want empty", byName["pool-live"].Autoscaling)
	}
	if byName["pool-zero"].SpecReplicas != 0 || byName["pool-zero"].StatusReplicas != 0 {
		t.Errorf("zero-zero must land verbatim (rule interprets, never pre-filtered): %+v", byName["pool-zero"])
	}
	if byName["pool-auto"].Autoscaling != "0-3" {
		t.Errorf("pool-auto autoscaling = %q, want 0-3", byName["pool-auto"].Autoscaling)
	}
	if byName["pool-auto"].HCClusterID != "hc-uuid-a" || byName["pool-auto"].Source != "hypershift" {
		t.Errorf("bad identity/provenance: %+v", byName["pool-auto"])
	}
}

func TestBuildNodepoolRows_NegativeReplicasSkip(t *testing.T) {
	hcs := map[string]string{"ns-a/hc-a": "hc-uuid-a"}
	pools := []unstructured.Unstructured{
		nodepoolObj("ns-a", "pool-neg", "hc-a", -1, 2, nil, nil, false),
	}
	rows, skipped := buildNodepoolRows(pools, hcs, "ws", "we", "ca")
	if len(rows) != 0 || skipped != 1 {
		t.Fatalf("negative counts must never fabricate: rows=%d skipped=%d", len(rows), skipped)
	}
}

func TestIsNodepoolAPIAvailable_NilConfig(t *testing.T) {
	if isNodepoolAPIAvailable(nil) {
		t.Fatal("nil RestConfig must read as absent API")
	}
}

func TestNodepoolRow_Contract(t *testing.T) {
	want := []string{
		"hc_cluster_id", "pool_name", "window_start", "window_end",
		"spec_replicas", "status_replicas", "autoscaling",
		"collected_at", "source",
	}
	if got := (nodepoolRow{}).csvHeader(); !reflect.DeepEqual(got, want) {
		t.Fatalf("header is the backend contract — changes coordinate: %v", got)
	}
}
