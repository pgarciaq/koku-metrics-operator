package packaging

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	metricscfgv1beta1 "github.com/project-koku/koku-metrics-operator/api/v1beta1"
	"github.com/project-koku/koku-metrics-operator/internal/dirconfig"
)

// #766 (V15-16) regression suite. The baseline cleared the staging directory
// whenever PackagingError was empty — but the controller wrapper resets that
// field before every retry, so a retry of a failed pass deleted the only
// copy of reports the failed pass had already moved to staging, and
// discovery (reports/ only) could never see them again. The audit's proof:
// reportA + reportB, fault on the second move, retry → A absent everywhere,
// archive contains only B, success reported.
//
// The controller wrapper's retry sequence (reset PackagingError, then
// PackageReports) is replicated faithfully here — the wrapper itself is
// package-private to internal/controller whose suite needs envtest.

// newRecoveryPackager builds an isolated packager with injectable moves.
func newRecoveryPackager(t *testing.T, filesAction func(string, string) error) (*FilePackager, *metricscfgv1beta1.MetricsConfig) {
	t.Helper()
	dir := t.TempDir()
	cfg := &dirconfig.DirectoryConfig{
		Parent:  dirconfig.Directory{Path: dir},
		Upload:  dirconfig.Directory{Path: filepath.Join(dir, "upload")},
		Staging: dirconfig.Directory{Path: filepath.Join(dir, "staging")},
		Reports: dirconfig.Directory{Path: filepath.Join(dir, "data")},
	}
	if err := dirconfig.CheckExistsOrRecreate(cfg.Upload, cfg.Staging, cfg.Reports); err != nil {
		t.Fatalf("newRecoveryPackager: could not create dirs: %v", err)
	}
	maxSize := int64(100)
	cr := &metricscfgv1beta1.MetricsConfig{}
	cr.Status.Packaging.MaxSize = &maxSize
	return &FilePackager{DirCfg: cfg, FilesAction: filesAction}, cr
}

// writeReport writes a report CSV with a DISTINCT marker row so archive
// assertions can verify content, not just names (#766 review finding: the
// original fixtures were byte-identical, so name checks could hide loss).
func writeReport(t *testing.T, reportsDir, name string) {
	t.Helper()
	writeReportRows(t, reportsDir, name, "marker-"+name)
}

func writeReportRows(t *testing.T, reportsDir, name, marker string) {
	t.Helper()
	content := "report_period_start,report_period_end,interval_start,interval_end,node,node_labels\n" +
		"2020-10-01 00:00:00 +0000 UTC,2020-11-01 00:00:00 +0000 UTC,2020-10-01 00:00:00 +0000 UTC,2020-10-01 01:00:00 +0000 UTC," + marker + ",\n"
	if err := os.WriteFile(filepath.Join(reportsDir, name), []byte(content), 0644); err != nil {
		t.Fatalf("writeReport: writing %s: %v", name, err)
	}
}

// archiveMembersWithContent returns the tar member names, the manifest.json
// bytes, and the concatenated content of every csv member.
func archiveMembersWithContent(t *testing.T, uploadDir string) (names []string, manifest string, csvContent string) {
	t.Helper()
	entries, err := os.ReadDir(uploadDir)
	if err != nil {
		t.Fatalf("archiveMembers: reading upload dir: %v", err)
	}
	var archives []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tar.gz") {
			archives = append(archives, filepath.Join(uploadDir, e.Name()))
		}
	}
	if len(archives) != 1 {
		t.Fatalf("archiveMembers: exactly one finalized archive expected, got %d: %v", len(archives), archives)
	}
	f, err := os.Open(archives[0])
	if err != nil {
		t.Fatalf("archiveMembers: opening archive: %v", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("archiveMembers: gzip reader: %v", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("archiveMembers: tar iteration: %v", err)
		}
		names = append(names, hdr.Name)
		if hdr.Name == "manifest.json" {
			data, err := io.ReadAll(tr)
			if err != nil {
				t.Fatalf("archiveMembers: reading manifest: %v", err)
			}
			manifest = string(data)
			continue
		}
		if strings.HasSuffix(hdr.Name, ".csv") {
			data, err := io.ReadAll(tr)
			if err != nil {
				t.Fatalf("archiveMembers: reading csv member: %v", err)
			}
			csvContent += string(data)
		}
	}
	return names, manifest, csvContent
}

func countCSVNames(names []string) int {
	n := 0
	for _, name := range names {
		if strings.HasSuffix(name, ".csv") {
			n++
		}
	}
	return n
}

// listReportsIn lists the report files (csv + instance-types json) in dir.
func listReportsIn(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("listReportsIn: reading %s: %v", dir, err)
	}
	var names []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".csv") ||
			(strings.Contains(e.Name(), "cluster_instance_types") && strings.HasSuffix(e.Name(), ".json")) {
			names = append(names, e.Name())
		}
	}
	return names
}

func containsMarker(names []string, marker string) bool {
	for _, n := range names {
		if strings.Contains(n, marker) {
			return true
		}
	}
	return false
}

// RED on baseline (the audit's exact scenario): a mid-sequence move failure
// strands reportA in staging; the controller-wrapper-equivalent retry must
// RETAIN it — A's content and manifest membership must survive into the
// recovered archive alongside B.
func TestPackageReports_MoveFailureRetryRetainsStagedReports(t *testing.T) {
	var bMoves atomic.Int32
	failOnB := func(from, to string) error {
		if strings.Contains(from, "reportB") && bMoves.Add(1) == 1 {
			return errors.New("injected transient move failure")
		}
		return MoveFiles(from, to)
	}
	p, cr := newRecoveryPackager(t, failOnB)
	writeReport(t, p.DirCfg.Reports.Path, "ros-openshift-reportA.csv")
	writeReport(t, p.DirCfg.Reports.Path, "ros-openshift-reportB.csv")

	// Pass 1: A moves to staging, B's move fails.
	err := p.PackageReports(cr)
	if err == nil {
		t.Fatal("pass 1 must fail on the second move")
	}
	cr.Status.Packaging.PackagingError = err.Error()

	// The stranded state (the audit's observed intermediate): A lives only
	// in staging; B remains in reports.
	if !containsMarker(listReportsIn(t, p.DirCfg.Staging.Path), "reportA") {
		t.Error("the failed pass must strand A in staging (observed intermediate)")
	}
	if got := len(listReportsIn(t, p.DirCfg.Reports.Path)); got != 1 {
		t.Errorf("B must remain in reports; got %d reports", got)
	}

	// The controller wrapper's retry: reset the error, retry with normal moves.
	p.FilesAction = MoveFiles
	cr.Status.Packaging.PackagingError = ""
	err = p.PackageReports(cr)
	if err != nil {
		t.Fatalf("the retry must succeed: %v", err)
	}

	// THE #766 ASSERTIONS: both reports' content and manifest membership
	// survive recovery. Baseline: staging was cleared (A destroyed) and the
	// archive contained only B.
	names, manifest, csvContent := archiveMembersWithContent(t, p.DirCfg.Upload.Path)
	if got := countCSVNames(names); got != 2 {
		t.Errorf("the recovered archive must contain BOTH reports (#766); got %d csv members: %v", got, names)
	}
	if !strings.Contains(manifest, "reportA") {
		t.Error("manifest membership must include the stranded report (#766)")
	}
	if !strings.Contains(manifest, "reportB") {
		t.Error("manifest membership must include the retried report (#766)")
	}
	if !strings.Contains(csvContent, "marker-ros-openshift-reportA.csv") {
		t.Error("the stranded report's CONTENT must survive recovery (#766)")
	}
	if !strings.Contains(csvContent, "marker-ros-openshift-reportB.csv") {
		t.Error("the retried report's content must be present (#766)")
	}

	// And staging is clean after the successful finalized packaging.
	if got := listReportsIn(t, p.DirCfg.Staging.Path); len(got) != 0 {
		t.Errorf("successful finalized packaging removes staging contents; got %v", got)
	}
}

// Fault AFTER all moves (at archive creation): every report staged, the
// archive step fails; the retry must package the full set.
func TestPackageReports_ArchiveFailureRetryRecoversAll(t *testing.T) {
	p, cr := newRecoveryPackager(t, MoveFiles)
	writeReport(t, p.DirCfg.Reports.Path, "ros-openshift-reportA.csv")
	writeReport(t, p.DirCfg.Reports.Path, "ros-openshift-reportB.csv")

	// Make archive creation fail: the upload directory rejects writes.
	// (Root bypasses DAC overrides, where this injection cannot fire.)
	if os.Geteuid() == 0 {
		t.Skip("the 0500 permission injection cannot fail under root (DAC override)")
	}
	if err := os.Chmod(p.DirCfg.Upload.Path, 0500); err != nil {
		t.Fatalf("chmod upload 0500: %v", err)
	}
	err := p.PackageReports(cr)
	if err == nil {
		t.Fatal("pass 1 must fail at archive creation")
	}
	cr.Status.Packaging.PackagingError = err.Error()

	// Everything is staged; nothing finalized.
	if got := len(listReportsIn(t, p.DirCfg.Staging.Path)); got != 2 {
		t.Errorf("both reports must be staged; got %d", got)
	}

	// Retry: restore the upload directory.
	if err := os.Chmod(p.DirCfg.Upload.Path, 0755); err != nil {
		t.Fatalf("chmod upload 0755: %v", err)
	}
	cr.Status.Packaging.PackagingError = ""
	err = p.PackageReports(cr)
	if err != nil {
		t.Fatalf("the retry must succeed: %v", err)
	}

	names, manifest, csvContent := archiveMembersWithContent(t, p.DirCfg.Upload.Path)
	if got := countCSVNames(names); got != 2 {
		t.Errorf("the recovered archive must contain both reports (#766); got %d csv members", got)
	}
	if !strings.Contains(manifest, "reportA") || !strings.Contains(manifest, "reportB") {
		t.Error("manifest membership must include both reports after archive-failure recovery (#766)")
	}
	if !strings.Contains(csvContent, "marker-ros-openshift-reportA.csv") || !strings.Contains(csvContent, "marker-ros-openshift-reportB.csv") {
		t.Error("both reports' content must survive archive-failure recovery (#766)")
	}
}

// Retry with a NEW source report: the recovered archive must include the
// stranded report, the retried report, AND the new one.
func TestPackageReports_RetryIncludesNewSourceReports(t *testing.T) {
	var bMoves atomic.Int32
	failOnB := func(from, to string) error {
		if strings.Contains(from, "reportB") && bMoves.Add(1) == 1 {
			return errors.New("injected transient move failure")
		}
		return MoveFiles(from, to)
	}
	p, cr := newRecoveryPackager(t, failOnB)
	writeReport(t, p.DirCfg.Reports.Path, "ros-openshift-reportA.csv")
	writeReport(t, p.DirCfg.Reports.Path, "ros-openshift-reportB.csv")

	err := p.PackageReports(cr)
	if err == nil {
		t.Fatal("pass 1 must fail")
	}
	cr.Status.Packaging.PackagingError = err.Error()

	// A new report arrives between passes.
	writeReport(t, p.DirCfg.Reports.Path, "ros-openshift-reportC.csv")

	p.FilesAction = MoveFiles
	cr.Status.Packaging.PackagingError = ""
	err = p.PackageReports(cr)
	if err != nil {
		t.Fatalf("the retry must succeed: %v", err)
	}

	names, manifest, csvContent := archiveMembersWithContent(t, p.DirCfg.Upload.Path)
	if got := countCSVNames(names); got != 3 {
		t.Errorf("the retry must package stranded + retried + NEW reports (#766); got %d csv members", got)
	}
	for _, marker := range []string{"reportA", "reportB", "reportC"} {
		if !strings.Contains(manifest, marker) {
			t.Errorf("manifest membership must include %s (#766)", marker)
		}
		if !strings.Contains(csvContent, "marker-ros-openshift-"+marker+".csv") {
			t.Errorf("%s's content must be present (#766)", marker)
		}
	}
}

// Order-reversal control: fault the FIRST move instead — nothing strands,
// and the retry packages both. A surviving-report assertion cannot hide the
// loss; only full membership can.
func TestPackageReports_FirstMoveFailureRetryRecoversAll(t *testing.T) {
	var aMoves atomic.Int32
	failOnA := func(from, to string) error {
		if strings.Contains(from, "reportA") && aMoves.Add(1) == 1 {
			return errors.New("injected transient move failure")
		}
		return MoveFiles(from, to)
	}
	p, cr := newRecoveryPackager(t, failOnA)
	writeReport(t, p.DirCfg.Reports.Path, "ros-openshift-reportA.csv")
	writeReport(t, p.DirCfg.Reports.Path, "ros-openshift-reportB.csv")

	err := p.PackageReports(cr)
	if err == nil {
		t.Fatal("pass 1 must fail")
	}
	cr.Status.Packaging.PackagingError = err.Error()

	p.FilesAction = MoveFiles
	cr.Status.Packaging.PackagingError = ""
	err = p.PackageReports(cr)
	if err != nil {
		t.Fatalf("the retry must succeed: %v", err)
	}

	names, manifest, csvContent := archiveMembersWithContent(t, p.DirCfg.Upload.Path)
	if got := countCSVNames(names); got != 2 {
		t.Errorf("the retry must package both reports (#766); got %d csv members", got)
	}
	if !strings.Contains(manifest, "reportA") || !strings.Contains(manifest, "reportB") {
		t.Error("manifest membership must include both reports (#766)")
	}
	if !strings.Contains(csvContent, "marker-ros-openshift-reportA.csv") || !strings.Contains(csvContent, "marker-ros-openshift-reportB.csv") {
		t.Error("both reports' content must survive the order-reversed retry (#766)")
	}
}

// F1 (review finding): an interrupted COPY leaves a truncated staged file
// while the source remains; a re-collected source's fresh staging must
// supersede the truncated artifact — the archive carries the complete
// report exactly once.
func TestPackageReports_TruncatedCopySupersededByFreshSource(t *testing.T) {
	truncated := false
	copyTruncatingA := func(from, to string) error {
		if strings.Contains(from, "reportA") && !truncated {
			truncated = true
			// An interrupted copy: only the header lands in staging.
			if err := os.WriteFile(to, []byte("report_period_start,report_period_end,interval_start,interval_end,node,node_labels\n"), 0644); err != nil {
				return err
			}
			return errors.New("injected interrupted copy")
		}
		return CopyFiles(from, to)
	}
	p, cr := newRecoveryPackager(t, copyTruncatingA)
	writeReport(t, p.DirCfg.Reports.Path, "ros-openshift-reportA.csv")

	// Pass 1: the copy of A is interrupted (source intact — copy mode).
	err := p.PackageReports(cr)
	if err == nil {
		t.Fatal("pass 1 must fail on the interrupted copy")
	}
	cr.Status.Packaging.PackagingError = err.Error()

	// Retry with normal copies (the controller wrapper's reset).
	p.FilesAction = CopyFiles
	cr.Status.Packaging.PackagingError = ""
	err = p.PackageReports(cr)
	if err != nil {
		t.Fatalf("the retry must succeed: %v", err)
	}

	names, manifest, csvContent := archiveMembersWithContent(t, p.DirCfg.Upload.Path)
	if got := countCSVNames(names); got != 1 {
		t.Errorf("the archive must contain the report exactly once; got %d csv members: %v", got, names)
	}
	if !strings.Contains(manifest, "reportA") {
		t.Error("manifest membership must include the report")
	}
	if !strings.Contains(csvContent, "marker-ros-openshift-reportA.csv") {
		t.Error("the COMPLETE content must be packaged, not the truncated copy (#766 review finding)")
	}
	if strings.Count(csvContent, "report_period_start") != 1 {
		t.Errorf("exactly one header row expected (no truncated duplicate); got %d", strings.Count(csvContent, "report_period_start"))
	}
}

// F2 (review finding): an interrupted split can leave the original AND its
// parts in staging. When the source is re-collected, the fresh staging
// supersedes BOTH old artifacts; when only the stranded set exists, the
// original supersedes its own parts — either way the archive carries the
// report's data exactly once.
func TestPackageReports_SplitArtifactsSuperseded(t *testing.T) {
	p, cr := newRecoveryPackager(t, MoveFiles)

	// Simulate the stranded state of an interrupted split under an OLD uid:
	// the original plus one completed part, and the source re-collected.
	oldUID := "00000000-0000-0000-0000-000000000000"
	stagedOriginal := oldUID + "-ros-openshift-reportA.csv"
	stagedPart := oldUID + "-ros-openshift-reportA1.csv"
	writeReportRows(t, p.DirCfg.Staging.Path, stagedOriginal, "marker-STALE-original")
	writeReportRows(t, p.DirCfg.Staging.Path, stagedPart, "marker-STALE-part")
	writeReport(t, p.DirCfg.Reports.Path, "ros-openshift-reportA.csv")
	writeReport(t, p.DirCfg.Reports.Path, "ros-openshift-reportB.csv")

	cr.Status.Packaging.PackagingError = "prior failed pass"
	if err := p.PackageReports(cr); err != nil {
		t.Fatalf("the retry must succeed: %v", err)
	}

	names, manifest, csvContent := archiveMembersWithContent(t, p.DirCfg.Upload.Path)
	if strings.Contains(csvContent, "marker-STALE-original") || strings.Contains(csvContent, "marker-STALE-part") {
		t.Error("stale split artifacts must be superseded, never packaged (#766 review finding)")
	}
	if !strings.Contains(csvContent, "marker-ros-openshift-reportA.csv") || !strings.Contains(csvContent, "marker-ros-openshift-reportB.csv") {
		t.Error("the fresh reports' content must be packaged")
	}
	if got := countCSVNames(names); got != 2 {
		t.Errorf("the archive must contain exactly the two reports; got %d csv members: %v", got, names)
	}
	_ = manifest
}

// F3 (review finding): a staged inventory plus a newly collected
// cluster_instance_types.json must not produce two tar members under the
// same upload name — the fresh staging supersedes the old copy.
func TestPackageReports_DuplicateInventorySuperseded(t *testing.T) {
	p, cr := newRecoveryPackager(t, MoveFiles)

	// A stranded old-uid inventory from a failed pass...
	oldUID := "00000000-0000-0000-0000-000000000000"
	if err := os.WriteFile(filepath.Join(p.DirCfg.Staging.Path, oldUID+"-cluster_instance_types.json"), []byte("{\"stale\":true}"), 0644); err != nil {
		t.Fatalf("writing staged inventory: %v", err)
	}
	// ...and a freshly collected one.
	if err := os.WriteFile(filepath.Join(p.DirCfg.Reports.Path, "cluster_instance_types.json"), []byte("{\"fresh\":true}"), 0644); err != nil {
		t.Fatalf("writing fresh inventory: %v", err)
	}
	writeReport(t, p.DirCfg.Reports.Path, "ros-openshift-reportA.csv")

	cr.Status.Packaging.PackagingError = "prior failed pass"
	if err := p.PackageReports(cr); err != nil {
		t.Fatalf("the retry must succeed: %v", err)
	}

	entries, err := os.ReadDir(p.DirCfg.Upload.Path)
	if err != nil {
		t.Fatalf("reading upload dir: %v", err)
	}
	var archive string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tar.gz") {
			archive = filepath.Join(p.DirCfg.Upload.Path, e.Name())
		}
	}
	if archive == "" {
		t.Fatal("no finalized archive")
	}
	f, err := os.Open(archive)
	if err != nil {
		t.Fatalf("opening archive: %v", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip: %v", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	inventoryMembers := 0
	inventoryContent := ""
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar: %v", err)
		}
		if strings.Contains(hdr.Name, "cluster_instance_types") {
			inventoryMembers++
			data, _ := io.ReadAll(tr)
			inventoryContent = string(data)
		}
	}
	if inventoryMembers != 1 {
		t.Errorf("exactly one inventory member expected (duplicates overwrite nondeterministically at extraction); got %d (#766 review finding)", inventoryMembers)
	}
	if inventoryContent != "{\"fresh\":true}" {
		t.Errorf("the FRESH inventory must win; got %q", inventoryContent)
	}
}

// The stranded-split case (rule 2 of the supersede logic): the original and
// its parts are BOTH staged (interrupted split) with NO source in reports —
// the original supersedes its own parts, and this pass re-splits afresh.
func TestPackageReports_StrandedOriginalSupersedesItsParts(t *testing.T) {
	p, cr := newRecoveryPackager(t, MoveFiles)

	// Stranded under one uid: the original plus a completed part.
	strandedUID := "11111111-0000-0000-0000-000000000000"
	writeReportRows(t, p.DirCfg.Staging.Path, strandedUID+"-ros-openshift-reportA.csv", "marker-ros-openshift-reportA.csv")
	writeReportRows(t, p.DirCfg.Staging.Path, strandedUID+"-ros-openshift-reportA1.csv", "marker-STALE-part")

	cr.Status.Packaging.PackagingError = "prior failed pass"
	if err := p.PackageReports(cr); err != nil {
		t.Fatalf("the recovery must succeed: %v", err)
	}

	_, manifest, csvContent := archiveMembersWithContent(t, p.DirCfg.Upload.Path)
	if strings.Contains(csvContent, "marker-STALE-part") {
		t.Error("a stranded original's stale parts must not be packaged alongside it (#766 review finding)")
	}
	if !strings.Contains(csvContent, "marker-ros-openshift-reportA.csv") {
		t.Error("the stranded report's content must be packaged")
	}
	if !strings.Contains(manifest, "reportA") {
		t.Error("manifest membership must include the stranded report")
	}
}
