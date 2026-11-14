//
// Copyright 2021 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0
//

package packaging

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logr "sigs.k8s.io/controller-runtime/pkg/log"

	metricscfgv1beta1 "github.com/project-koku/koku-metrics-operator/api/v1beta1"
	"github.com/project-koku/koku-metrics-operator/internal/dirconfig"
	"github.com/project-koku/koku-metrics-operator/internal/strset"
)

// FilePackager struct for defining the packaging vars
type FilePackager struct {
	DirCfg           *dirconfig.DirectoryConfig
	FilesAction      FilesAction
	manifest         manifestInfo
	uid              string
	createdTimestamp string
	maxBytes         int64
	start            time.Time
	end              time.Time
}

type FilesAction func(src, dst string) error

const (
	timestampFormat = "20060102T150405.999999"

	megaByte int64 = 1024 * 1024

	// the csv module does not expose the bytes-offset of the
	// underlying file object.
	// instead, the script estimates the size of the data as VARIANCE percent larger than a
	// naïve string concatenation of the CSV fields to cover the overhead of quoting
	// and delimiters. This gets close enough for now.
	// VARIANCE := 0.03
	variance float64 = 0.03
)

var (
	MoveFiles FilesAction = os.Rename
	CopyFiles FilesAction = copyFile

	// if we're creating more than 1k files, something is probably wrong.
	maxSplits int64 = 1000

	BUFFERSIZE int64 = 10 * megaByte

	// ErrNoReports a "no reports" Error type
	ErrNoReports = errors.New("reports not found")

	// Set boolean on whether community or certified
	isCertified bool = false

	log = logr.Log.WithName("packaging")
)

// Manifest interface
type Manifest interface{}

// manifest template
type manifest struct {
	UUID         string                                `json:"uuid"`
	ClusterID    string                                `json:"cluster_id"`
	Version      string                                `json:"version"`
	Date         time.Time                             `json:"date"`
	Files        []string                              `json:"files"`
	ROSFiles     []string                              `json:"resource_optimization_files"`
	Start        time.Time                             `json:"start"`
	End          time.Time                             `json:"end"`
	CRStatus     metricscfgv1beta1.MetricsConfigStatus `json:"cr_status"`
	Certified    bool                                  `json:"certified"`
	DailyReports bool                                  `json:"daily_reports"`
}

type FileInfoManifest manifest
type manifestInfo struct {
	manifest Manifest
	filename string
}
type fileTracker struct {
	costfiles map[int]string
	rosfiles  map[int]string
	allfiles  map[int]string
}

func newFileTracker() fileTracker {
	return fileTracker{
		costfiles: make(map[int]string, 4),
		rosfiles:  make(map[int]string, 1),
		allfiles:  make(map[int]string, 5),
	}
}

// renderManifest writes the manifest
func (m *manifestInfo) renderManifest() error {
	// write the manifest file
	file, err := json.MarshalIndent(m.manifest, "", " ")
	if err != nil {
		return fmt.Errorf("renderManifest: failed to marshal manifest: %v", err)
	}
	if err := os.WriteFile(m.filename, file, 0644); err != nil {
		return fmt.Errorf("renderManifest: failed to write manifest: %v", err)
	}
	return nil
}

func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	_, err = os.Stat(dst)
	if err == nil {
		return fmt.Errorf("file %s already exists", dst)
	}

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	buf := make([]byte, BUFFERSIZE)
	for {
		n, err := source.Read(buf)
		if err != nil && err != io.EOF {
			return err
		}
		if n == 0 {
			break
		}

		if _, err := destination.Write(buf[:n]); err != nil {
			return err
		}
	}
	return err
}

const clusterInstanceTypesUploadName = "cluster_instance_types.json"

// buildLocalCSVFileList gets the list of files in the staging directory.
func (p *FilePackager) buildLocalCSVFileList(fileList []os.FileInfo, stagingDirectory string) fileTracker {
	tracker := newFileTracker()
	for idx, file := range fileList {
		filePath := filepath.Join(stagingDirectory, file.Name())
		if strings.Contains(file.Name(), "cluster_instance_types") && strings.HasSuffix(file.Name(), ".json") {
			tracker.rosfiles[idx] = filePath
			tracker.allfiles[idx] = filePath
			continue
		}
		if !strings.HasSuffix(file.Name(), ".csv") {
			continue
		}
		if strings.Contains(file.Name(), "ros-openshift") {
			tracker.rosfiles[idx] = filePath
		} else {
			tracker.costfiles[idx] = filePath
		}
		if strings.Contains(file.Name(), "storage-usage") {
			tracker.rosfiles[idx] = filePath
		}
		tracker.allfiles[idx] = filePath
	}
	return tracker
}

func (p *FilePackager) getManifest(archiveFiles fileTracker, filePath string, cr *metricscfgv1beta1.MetricsConfig) {
	// setup the manifest
	manifestDate := metav1.Now()
	var costFiles []string
	for idx, costFile := range archiveFiles.costfiles {
		uploadName := p.createCSVUploadName(costFile, idx)
		costFiles = append(costFiles, uploadName)
	}
	var rosFiles []string
	for idx, rosFile := range archiveFiles.rosfiles {
		if strings.Contains(rosFile, "cluster_instance_types") {
			rosFiles = append(rosFiles, clusterInstanceTypesUploadName)
			continue
		}
		uploadName := p.createCSVUploadName(rosFile, idx)
		rosFiles = append(rosFiles, uploadName)
	}
	p.manifest = manifestInfo{
		manifest: manifest{
			UUID:         p.uid,
			ClusterID:    cr.Status.ClusterID,
			Version:      cr.Status.OperatorCommit,
			Date:         manifestDate.UTC(),
			Files:        costFiles,
			ROSFiles:     rosFiles,
			Start:        p.start.UTC(),
			End:          p.end.UTC(),
			Certified:    isCertified,
			CRStatus:     cr.Status,
			DailyReports: true,
		},
		filename: filepath.Join(filePath, "manifest.json"),
	}
}

func (p *FilePackager) addFileToTarWriter(uploadName, filePath string, tarWriter *tar.Writer) error {
	log := log.WithName("addFileToTarWriter")
	log.Info("adding file to tar.gz", "file", filePath)
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return err
	}

	header := &tar.Header{
		Name:    uploadName,
		Size:    stat.Size(),
		Mode:    int64(stat.Mode()),
		ModTime: stat.ModTime(),
	}

	if err := tarWriter.WriteHeader(header); err != nil {
		return err
	}

	if _, err := io.Copy(tarWriter, file); err != nil {
		return err
	}

	return nil
}

// writeSplitTarballSet writes the split archive set: one tarball per
// staged file, each named <filenameBase>-<idx>.tar.gz, all sharing the
// one manifest. #836: the set must finalize ALL-OR-NOTHING — a failure
// partway through the set must never leave already-finalized tarballs
// in the upload directory. The uploader uploads everything present
// (no prune/dedup; removal only on a 202), and the retry packages
// under a FRESH timestamp — a lingering partial set from a failed pass
// would upload alongside the new full set, duplicating the reports
// they share server-side.
func (p *FilePackager) writeSplitTarballSet(filenameBase string, tracker fileTracker) error {
	// Deterministic part order (sorted idx) — map iteration is random,
	// and the set's semantics must not depend on which part happens to
	// write first.
	idxs := make([]int, 0, len(tracker.allfiles))
	for idx := range tracker.allfiles {
		idxs = append(idxs, idx)
	}
	sort.Ints(idxs)

	// #836: all-or-nothing finalize. Parts first write under PENDING
	// names ("<final>.part" — deliberately avoiding the "tar.gz"
	// substring, which the uploader selects by, so a lingering pending
	// file can never upload), and the set renames into the final names
	// only after every part finalized. A failure partway removes the
	// pendings and any already-renamed finals (rename-loop compensation;
	// a failed compensation is SURFACED — the set marker below is never
	// written, so the parts stay unpublishable and the next run's sweep
	// (sweepOrphanedSplitArtifacts) removes them). The SET MARKER
	// ("<base>.set") is written only after every rename succeeded — the
	// commit point. The uploader refuses part-shaped tarballs without
	// their marker, so a crash between renames (compensation never runs)
	// still cannot publish a partial set. Stale pendings, orphaned
	// unmarked part-shaped finals, and fully-uploaded markers are swept
	// at every packaging pass (sweepOrphanedSplitArtifacts — both the
	// split and non-split paths, #836 review P3).
	if err := sweepOrphanedSplitArtifacts(p.DirCfg.Upload.Path); err != nil {
		return fmt.Errorf("writeSplitTarballSet: sweep: %v", err)
	}
	type partName struct{ pending, final string }
	written := make([]partName, 0, len(idxs))
	var cleanupErr error
	cleanup := func() {
		for _, w := range written {
			if err := os.Remove(w.pending); err != nil && cleanupErr == nil {
				cleanupErr = fmt.Errorf("writeSplitTarballSet: cleanup: removing %s: %v", w.pending, err)
			}
			if err := os.Remove(w.final); err != nil && cleanupErr == nil {
				cleanupErr = fmt.Errorf("writeSplitTarballSet: cleanup: removing %s: %v", w.final, err)
			}
		}
	}
	for _, idx := range idxs {
		fileMap := map[int]string{idx: tracker.allfiles[idx]}
		finalName := filenameBase + "-" + strconv.Itoa(idx) + ".tar.gz"
		pendingName := strings.TrimSuffix(finalName, ".tar.gz") + ".part"
		if err := p.writeTarball(pendingName, p.manifest.filename, fileMap); err != nil {
			cleanup()
			if cleanupErr != nil {
				return fmt.Errorf("%v (set left unmarked — unpublishable; the next pass sweeps it)", cleanupErr)
			}
			return err
		}
		written = append(written, partName{pending: filepath.Join(p.DirCfg.Upload.Path, pendingName),
			final: filepath.Join(p.DirCfg.Upload.Path, finalName)})
	}
	for _, w := range written {
		if err := os.Rename(w.pending, w.final); err != nil {
			cleanup()
			if cleanupErr != nil {
				return fmt.Errorf("%v (set left unmarked — unpublishable; the next pass sweeps it)", cleanupErr)
			}
			return fmt.Errorf("writeSplitTarballSet: finalizing part set: %v", err)
		}
	}
	// The commit point: the marker makes the set publishable.
	markerPath := filepath.Join(p.DirCfg.Upload.Path, filenameBase+".set")
	if err := os.WriteFile(markerPath, []byte(strconv.Itoa(len(written))), 0644); err != nil {
		return fmt.Errorf("writeSplitTarballSet: writing set marker: %v", err)
	}
	return nil
}

// sweepOrphanedSplitArtifacts cleans the upload directory of split-set
// residue (#836): stale pending files (".part" — a crashed attempt),
// part-shaped finalized tarballs ("-<n>.tar.gz") WITHOUT their set
// marker (a crash between renames, or a failed compensation —
// unpublishable by the uploader's marker gate, removed here), and set
// markers whose finals are all gone (a fully-uploaded set). Runs on
// every packaging pass, both the split and non-split paths.
//
// Version-skew note: part-shaped tarballs written by operator versions
// predating the marker protocol also lack markers and are swept —
// deploy the marker-aware operator when no old-format split sets are
// stuck in upload (stuck-upload residue is the only shape affected).
func sweepOrphanedSplitArtifacts(uploadPath string) error {
	entries, err := os.ReadDir(uploadPath)
	if err != nil {
		return err
	}
	marked := make(map[string]bool, len(entries))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".set") {
			marked[strings.TrimSuffix(e.Name(), ".set")] = true
		}
	}
	// Consistency rule: a committed marker (written only after every
	// rename succeeded) never coexists with its own base's PENDING
	// files. Marker + same-base pending = an inconsistently committed
	// set (a marker-before-renames bug shape, or FS-level weirdness) —
	// clean the whole set: marker, finals, and pendings.
	for base := range marked {
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".part") && strings.HasPrefix(e.Name(), base+"-") {
				if err := removeSplitSet(uploadPath, entries, base); err != nil {
					return err
				}
				delete(marked, base)
				break
			}
		}
	}
	// removeIfGone: the consistency rule above may already have removed
	// entries — the stale snapshot must treat "no such file" as done.
	removeOrGone := func(full, kind string) error {
		if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("sweep: removing %s %s: %v", kind, filepath.Base(full), err)
		}
		return nil
	}
	for _, e := range entries {
		name := e.Name()
		full := filepath.Join(uploadPath, name)
		switch {
		case strings.HasSuffix(name, ".part"):
			if err := removeOrGone(full, "pending"); err != nil {
				return err
			}
		case IsPartShapedTarball(name) && !marked[SplitSetBase(name)]:
			if err := removeOrGone(full, "orphaned part"); err != nil {
				return err
			}
		case strings.HasSuffix(name, ".set"):
			base := strings.TrimSuffix(name, ".set")
			if !marked[base] {
				continue // removed by the consistency rule above
			}
			if !anyFinalForBase(entries, base) {
				if err := removeOrGone(full, "spent marker"); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// removeSplitSet removes a set's marker, its part-shaped finals, and
// its pendings — the inconsistently-committed-set cleanup.
func removeSplitSet(uploadPath string, entries []os.DirEntry, base string) error {
	for _, e := range entries {
		name := e.Name()
		if name == base+".set" ||
			(strings.HasPrefix(name, base+"-") && (IsPartShapedTarball(name) || strings.HasSuffix(name, ".part"))) {
			if err := os.Remove(filepath.Join(uploadPath, name)); err != nil {
				return fmt.Errorf("sweep: removing inconsistent set member %s: %v", name, err)
			}
		}
	}
	return nil
}

// isPartShapedTarball reports whether name is a split part's finalized
// name: "<base>-<n>.tar.gz" with a numeric part suffix.
func IsPartShapedTarball(name string) bool {
	if !strings.HasSuffix(name, ".tar.gz") {
		return false
	}
	stem := strings.TrimSuffix(name, ".tar.gz")
	dash := strings.LastIndex(stem, "-")
	if dash < 0 || dash == len(stem)-1 {
		return false
	}
	for _, c := range stem[dash+1:] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// splitSetBase returns the set base of a part-shaped name (the name
// minus "-<n>.tar.gz").
func SplitSetBase(name string) string {
	stem := strings.TrimSuffix(name, ".tar.gz")
	dash := strings.LastIndex(stem, "-")
	return stem[:dash]
}

// anyFinalForBase reports whether any entry is a part-shaped tarball of
// the given set base.
func anyFinalForBase(entries []os.DirEntry, base string) bool {
	for _, e := range entries {
		if IsPartShapedTarball(e.Name()) && SplitSetBase(e.Name()) == base {
			return true
		}
	}
	return false
}

// writeTarball packages the files into tar balls
func (p *FilePackager) writeTarball(tarFileName, manifestFileName string, archiveFiles map[int]string) error {
	tarFilePath := filepath.Join(p.DirCfg.Upload.Path, tarFileName)
	log.Info("generating tar.gz", "tarFile", tarFilePath)

	// #766: write to a temp name and rename on completion — a failed or
	// interrupted archive attempt must never leave a partial file under the
	// final name in the upload directory. The temp name deliberately avoids
	// the "tar.gz" substring: the uploader selects upload candidates by
	// substring match, so an interrupted attempt's temp file must never be
	// uploadable.
	tmpFilePath := strings.TrimSuffix(tarFilePath, ".tar.gz") + ".tar.tmp"
	tarFile, err := os.Create(tmpFilePath)
	if err != nil {
		return fmt.Errorf("writeTarball: error creating tar file: %v", err)
	}
	// Close on every exit path — error returns below must not leak the
	// descriptor (double-close on the success path is a harmless no-op
	// error).
	defer func() { _ = tarFile.Close() }()
	tmpOK := false
	defer func() {
		if !tmpOK {
			_ = os.Remove(tmpFilePath)
		}
	}()

	gzipWriter := gzip.NewWriter(tarFile)
	tw := tar.NewWriter(gzipWriter)

	// add the files to the tarFile
	for idx, filePath := range archiveFiles {
		switch {
		case strings.Contains(filePath, "cluster_instance_types") && strings.HasSuffix(filePath, ".json"):
			if err := p.addFileToTarWriter(clusterInstanceTypesUploadName, filePath, tw); err != nil {
				return fmt.Errorf("writeTarball: failed to create tar file: %v", err)
			}
		case strings.HasSuffix(filePath, ".csv"):
			uploadName := p.createCSVUploadName(filePath, idx)
			if err := p.addFileToTarWriter(uploadName, filePath, tw); err != nil {
				return fmt.Errorf("writeTarball: failed to create tar file: %v", err)
			}
		}
	}
	if err := p.addFileToTarWriter("manifest.json", manifestFileName, tw); err != nil {
		return fmt.Errorf("writeTarball: failed to create tar file: %v", err)
	}

	// Close the writers in order (tar → gzip → file) before the rename so
	// the flushed bytes hit disk, then finalize atomically (#766).
	if err := tw.Close(); err != nil {
		return fmt.Errorf("writeTarball: closing tar writer: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		return fmt.Errorf("writeTarball: closing gzip writer: %v", err)
	}
	if err := tarFile.Sync(); err != nil {
		return fmt.Errorf("writeTarball: syncing tar file: %v", err)
	}
	if err := tarFile.Close(); err != nil {
		return fmt.Errorf("writeTarball: closing tar file: %v", err)
	}
	if err := os.Rename(tmpFilePath, tarFilePath); err != nil {
		return fmt.Errorf("writeTarball: finalizing tar file: %v", err)
	}
	tmpOK = true
	return nil
}

// stagedBaseName strips the "<uuid>-" prefix the packager adds when moving
// files to staging, recovering the original report name.
func stagedBaseName(stagedName string) string {
	// uuid v4 with dashes is 36 chars; staged names are "<uuid>-<original>".
	if len(stagedName) > 37 {
		return stagedName[37:]
	}
	return stagedName
}

// isSplitPartOf reports whether stagedBase is a split part of the source
// report base (writePart names parts "<base><n>.csv", n >= 1).
func isSplitPartOf(stagedBase, sourceBase string) bool {
	if !strings.HasSuffix(sourceBase, ".csv") || !strings.HasSuffix(stagedBase, ".csv") {
		return false
	}
	prefix := strings.TrimSuffix(sourceBase, ".csv")
	rest := strings.TrimSuffix(stagedBase, ".csv")
	if !strings.HasPrefix(rest, prefix) {
		return false
	}
	num := rest[len(prefix):]
	if num == "" {
		return false
	}
	for _, r := range num {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// splitPartRoot returns the base name of the original that stagedBase was
// split from, anchored on the collector's naming scheme (#829 review C1):
// every collector filename is "<type-prefix><YYYYMM>.csv" — a trailing
// digit run of exactly six, a valid year-month (collector.go builds all
// eighteen reports as prefix + yearMonth). Parts append the part number
// to the FULL original stem (writePart: "<stem><n>.csv", n ≥ 1, no
// padding), so a part's trailing run is always YYYYMM∥partnumber — seven
// digits or more, and the original's month is the run's first six digits,
// an unambiguous concatenation. Part 10 of "…-202611" ("…-20261110")
// roots to "…-202611", never to part 1 ("…-2026111").
//
// Returns "" when the base is not part-shaped under this scheme: a
// six-run original (never a part — part numbers make the run ≥ 7), a
// shorter or invalid-month run (nothing in the product writes it; the
// recovery leaves such files alone rather than guess), or a non-CSV.
func splitPartRoot(stagedBase string) string {
	if !strings.HasSuffix(stagedBase, ".csv") {
		return ""
	}
	stem := strings.TrimSuffix(stagedBase, ".csv")
	i := len(stem)
	for i > 0 && stem[i-1] >= '0' && stem[i-1] <= '9' {
		i--
	}
	run := stem[i:]
	if len(run) < 7 || !validYearMonth(run[:6]) {
		return ""
	}
	return stem[:i] + run[:6] + ".csv"
}

// validYearMonth reports whether ym is six digits forming a plausible
// year-month (year 2000+, month 01–12) — the collector's YYYYMM suffix.
func validYearMonth(ym string) bool {
	if len(ym) != 6 {
		return false
	}
	year, month := 0, 0
	for _, r := range ym {
		if r < '0' || r > '9' {
			return false
		}
		year = year*10 + int(r-'0')
		month = month*10 + int(r-'0')
	}
	// year accumulated all six digits; recompute month from the last two.
	month = year % 100
	year = year / 100
	return year >= 2000 && month >= 1 && month <= 12
}

// supersedeStagedArtifacts deletes staged entries derived from the source
// report (#766 review findings): a prior pass may have left an old-uid copy
// (complete or truncated — copy mode can be interrupted mid-write) or split
// parts under the OLD uid. When the source is (re-)staged fresh, those
// artifacts are stale: the fresh move supersedes them, and keeping them
// would duplicate the report in the archive (or package a truncated copy).
func (p *FilePackager) supersedeStagedArtifacts(sourceName string) error {
	stagedList, err := os.ReadDir(p.DirCfg.Staging.Path)
	if err != nil {
		return fmt.Errorf("supersedeStagedArtifacts: could not read staging directory: %v", err)
	}
	for _, entry := range stagedList {
		base := stagedBaseName(entry.Name())
		if base == sourceName || isSplitPartOf(base, sourceName) {
			if err := os.Remove(filepath.Join(p.DirCfg.Staging.Path, entry.Name())); err != nil {
				return fmt.Errorf("supersedeStagedArtifacts: could not remove superseded %s: %v", entry.Name(), err)
			}
			log.WithName("supersedeStagedArtifacts").Info("removed superseded staged artifact",
				"staged", entry.Name(), "source", sourceName)
		}
	}
	return nil
}

// writePart writes a portion of a split file into a new file
func (p *FilePackager) writePart(fileName string, csvReader *csv.Reader, csvHeader []string, num int64) (*os.File, bool, error) {
	log := log.WithName("writePart")
	fileNamePart := strings.TrimSuffix(fileName, ".csv")
	sizeEstimate := 0
	splitFileName := fileNamePart + strconv.FormatInt(num, 10) + ".csv"
	log.Info("creating file ", "file", splitFileName)
	splitFile, err := os.Create(splitFileName)
	if err != nil {
		return nil, false, fmt.Errorf("writePart: error creating file: %v", err)
	}
	// Create the csv writer
	writer := csv.NewWriter(splitFile)
	// Preserve the header
	if err := writer.Write(csvHeader); err != nil {
		return nil, false, err
	}
	for {
		row, err := csvReader.Read()
		if errors.Is(err, io.EOF) {
			writer.Flush()
			return splitFile, true, nil
		} else if err != nil {
			return nil, false, err
		}
		if err := writer.Write(row); err != nil {
			return nil, false, err
		}
		rowLen := len(strings.Join(row, ","))
		rowSize := rowLen + int(float64(rowLen)*variance)
		sizeEstimate += rowSize
		if sizeEstimate >= int(p.maxBytes) {
			writer.Flush()
			return splitFile, false, nil
		}
	}
}

// getIndex returns the index of an element in an array
func getIndex(array []string, val string) (int, error) {
	for index, value := range array {
		if value == val {
			return index, nil
		}
	}
	err := errors.New("could not index the interval time")
	return -1, err
}

// getStartEnd grabs the start and end interval from the csvFile
func (p *FilePackager) getStartEnd(filePath string) error {
	csvFile, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("getStartEnd: error opening file: %v", err)
	}
	csvReader := csv.NewReader(csvFile)
	csvHeader, err := csvReader.Read()
	if err != nil {
		return fmt.Errorf("getStartEnd: error reading file: %v", err)
	}
	startIndex, err := getIndex(csvHeader, "interval_start")
	if err != nil {
		return fmt.Errorf("getStartEnd: error getting the start index: %v", err)
	}
	endIndex, err := getIndex(csvHeader, "interval_end")
	if err != nil {
		return fmt.Errorf("getStartEnd: error getting the end index: %v", err)
	}
	// grab the first line to get the initial interval start
	firstLine, err := csvReader.Read()
	if err != nil {
		return fmt.Errorf("getStartEnd: error reading file: %v", err)
	}
	startInterval := firstLine[startIndex]
	p.start, _ = time.Parse("2006-01-02 15:04:05.999999999 -0700 MST", startInterval)
	// need to grab the last line in the file to get the last interval end
	lastLine, err := getLastRow(csvReader, firstLine)
	if err != nil {
		return fmt.Errorf("getStartEnd: getLastRow: %v", err)
	}
	endInterval := lastLine[endIndex]
	p.end, _ = time.Parse("2006-01-02 15:04:05.999999999 -0700 MST", endInterval)
	return nil
}

func getLastRow(csvReader *csv.Reader, firstRow []string) ([]string, error) {
	lastRow := firstRow
	for {
		record, err := csvReader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		lastRow = record
	}
	return lastRow, nil
}

// splitFiles breaks larger files into smaller ones
func (p *FilePackager) splitFiles(filePath string, fileList []os.FileInfo) ([]os.FileInfo, bool, error) {
	log := log.WithName("splitFiles")
	if !p.needSplit(fileList) {
		log.Info("files do not require splitting")
		return fileList, false, nil
	}
	log.Info("files require splitting")
	var splitFiles []os.FileInfo
	for _, file := range fileList {
		absPath := filepath.Join(filePath, file.Name())
		fileSize := file.Size()
		if fileSize >= p.maxBytes {
			// #829 review C2: never re-split a part. A part's re-split
			// outputs are named stem(part)+n — the deterministic names of
			// the part's own siblings — and os.Create would truncate
			// them, silently destroying their rows. A part can exceed
			// maxBytes only through a single oversized row or a shrunken
			// MaxSize between passes; it is packaged as-is so the
			// anomaly stays visible (a possibly-oversized tarball member
			// and a loud log line) instead of silently corrupting the
			// archive — fail-visible over fail-silent (owner-approved
			// 2026-11-10).
			if splitPartRoot(stagedBaseName(file.Name())) != "" {
				log.Info("staged split part exceeds max bytes; packaging as-is instead of re-splitting (re-split would overwrite its siblings)",
					"file", file.Name(), "size", fileSize, "maxBytes", p.maxBytes)
				splitFiles = append(splitFiles, file)
				continue
			}
			// open the file
			csvFile, err := os.Open(absPath)
			if err != nil {
				return nil, false, fmt.Errorf("splitFiles: error opening file: %v", err)
			}
			csvReader := csv.NewReader(csvFile)
			csvHeader, err := csvReader.Read()
			if err != nil {
				return nil, false, fmt.Errorf("splitFiles: error reading file: %v", err)
			}
			var part int64 = 1
			for {
				newFile, eof, err := p.writePart(absPath, csvReader, csvHeader, part)
				if err != nil {
					return nil, false, fmt.Errorf("splitFiles: error writing part: %v", err)
				}
				info, err := newFile.Stat()
				if err != nil {
					return nil, false, fmt.Errorf("splitFiles: error getting file stats: %v", err)
				}
				splitFiles = append(splitFiles, info)
				part++
				if eof || part >= maxSplits {
					break
				}
			}
			os.Remove(absPath)
		} else {
			splitFiles = append(splitFiles, file)
		}
	}
	return splitFiles, true, nil
}

// needSplit determines if any of the files to be packaged need to be split.
func (p *FilePackager) needSplit(fileList []os.FileInfo) bool {
	var totalSize int64 = 0
	for _, file := range fileList {
		fileSize := file.Size()
		totalSize += fileSize
		if fileSize >= p.maxBytes || totalSize >= p.maxBytes {
			return true
		}
	}
	return false
}

// moveOrCopyFiles moves files from reportsDirectory to stagingDirectory.
// #766 (V15-16): staging is NOT cleared at entry — a prior failed pass may
// have stranded already-moved reports there (their only copy), and the
// retry must include them as pending input. Sources stage FIRST (each
// fresh staging superseding the same report's old-uid artifacts: truncated
// or complete prior copies, and parts from an interrupted split); the
// recovery read of staging happens AFTER the moves so superseded artifacts
// are never collected. Staging is removed only after successful finalized
// packaging (see PackageReports).
func (p *FilePackager) moveOrCopyFiles(cr *metricscfgv1beta1.MetricsConfig) ([]os.FileInfo, error) {
	log := log.WithName("moveOrCopyFiles")

	fileList, err := os.ReadDir(p.DirCfg.Reports.Path)
	if err != nil {
		return nil, fmt.Errorf("moveOrCopyFiles: could not read reports directory: %v", err)
	}

	log.Info("moving or copying report files to staging directory")
	for _, file := range fileList {
		isCSV := strings.HasSuffix(file.Name(), ".csv")
		isClusterInstanceTypesJSON := strings.Contains(file.Name(), "cluster_instance_types") && strings.HasSuffix(file.Name(), ".json")
		if !isCSV && !isClusterInstanceTypesJSON {
			continue
		}

		from := filepath.Join(p.DirCfg.Reports.Path, file.Name())
		to := filepath.Join(p.DirCfg.Staging.Path, p.uid+"-"+file.Name())

		// #766 review finding: the fresh staging of a source supersedes any
		// old-uid artifacts of the SAME report (a truncated or complete
		// prior copy, or parts from an interrupted split) — keeping them
		// would duplicate the report or package a partial copy.
		if err := p.supersedeStagedArtifacts(file.Name()); err != nil {
			return nil, fmt.Errorf("moveOrCopyFiles: %v", err)
		}
		if err := p.FilesAction(from, to); err != nil {
			return nil, fmt.Errorf("moveOrCopyFiles: failed to copy/move files: %v", err)
		}
	}

	// Recovery read AFTER the moves: staging now holds the fresh moves plus
	// reports stranded by prior failed passes (their sources are gone) —
	// and nothing that a fresh move just superseded.
	stagedList, err := os.ReadDir(p.DirCfg.Staging.Path)
	if err != nil {
		return nil, fmt.Errorf("moveOrCopyFiles: could not read staging directory: %v", err)
	}
	var files []os.FileInfo
	for _, file := range stagedList {
		if filepath.Base(file.Name()) == "manifest.json" {
			continue
		}
		isCSV := strings.HasSuffix(file.Name(), ".csv")
		isClusterInstanceTypesJSON := strings.Contains(file.Name(), "cluster_instance_types") && strings.HasSuffix(file.Name(), ".json")
		if !isCSV && !isClusterInstanceTypesJSON {
			continue
		}
		info, err := file.Info()
		if err != nil {
			return nil, fmt.Errorf("moveOrCopyFiles: could not stat staged file %s: %v", file.Name(), err)
		}
		files = append(files, info)
	}

	// An original supersedes its own split parts, BY ROOT (#766 review
	// finding, corrected by #829/V16-05): a staged part is stale iff its
	// ROOT original — the base with the entire trailing digit run
	// stripped — is present in the same uid group. The splitter removes
	// the original only AFTER writing the full contiguous part run, so
	// original-present ⟹ interrupted mid-split ⟹ the parts are stale
	// (this pass decides afresh whether to split, overwriting the same
	// deterministic part names). Root ABSENT ⟹ the split completed and
	// the splitter removed the original: the parts are the report's ONLY
	// representation and are all recovered. The prior pairwise form ("a
	// part of ANY present file") misclassified part 10+ of a ≥10-part
	// split as parts of parts 1–9 ("report10" = "report1"+"0") and
	// deleted them, returning success over a truncated archive. Parts
	// share their original's staged uid prefix, which is what
	// distinguishes a part of X from an independently-collected report
	// literally named like a part.
	uidOf := func(name string) string {
		if len(name) >= 37 {
			return name[:36]
		}
		return ""
	}
	presentBases := map[string]map[string]bool{}
	for _, f := range files {
		fu := uidOf(f.Name())
		if presentBases[fu] == nil {
			presentBases[fu] = map[string]bool{}
		}
		presentBases[fu][stagedBaseName(f.Name())] = true
	}
	var recovered []os.FileInfo
	for _, f := range files {
		fb, fu := stagedBaseName(f.Name()), uidOf(f.Name())
		if root := splitPartRoot(fb); root != "" && presentBases[fu][root] {
			if err := os.Remove(filepath.Join(p.DirCfg.Staging.Path, f.Name())); err != nil {
				return nil, fmt.Errorf("moveOrCopyFiles: could not remove superseded split part %s: %v", f.Name(), err)
			}
			continue
		}
		recovered = append(recovered, f)
	}
	files = recovered

	if len(fileList) <= 0 && len(files) <= 0 {
		return nil, ErrNoReports
	}
	if stranded := len(files); stranded > 0 {
		log.Info("packaging staged reports", "fresh-and-recovered", stranded)
	}
	return files, nil
}

func (p *FilePackager) TrimPackages(cr *metricscfgv1beta1.MetricsConfig) error {
	log := log.WithName("trimPackages")

	packages, err := p.DirCfg.Upload.GetFiles()
	if err != nil {
		return fmt.Errorf("failed to read upload dir: %v", err)
	}

	datetimesSet := strset.NewSet()
	for _, f := range packages {
		if strings.HasSuffix(f, "tar.gz") {
			datetimesSet.Add(strings.Split(f, "-")[0])
		}
	}

	reportCount := int64(datetimesSet.Len())

	if reportCount <= cr.Spec.Packaging.MaxReports {
		log.Info("number of stored reports within limit")
		cr.Status.Packaging.ReportCount = &reportCount
		return nil
	}

	log.Info("max report count reached: removing oldest reports")

	datetimes := []string{}
	for d := range datetimesSet.Range() {
		datetimes = append(datetimes, d)
	}

	sort.Strings(datetimes)
	ind := len(datetimes) - int(cr.Spec.Packaging.MaxReports)
	filesToExclude := datetimes[0:ind]

	for _, pre := range filesToExclude {
		for _, file := range packages {
			if !strings.HasPrefix(file, pre) {
				continue
			}
			log.Info(fmt.Sprintf("removing report: %s", file))
			if err := os.Remove(filepath.Join(p.DirCfg.Upload.Path, file)); err != nil {
				return fmt.Errorf("failed to remove %s: %v", file, err)
			}
		}
	}

	cr.Status.Packaging.ReportCount = &cr.Spec.Packaging.MaxReports
	return nil
}

// PackageReports is responsible for packing report files for upload
func (p *FilePackager) PackageReports(cr *metricscfgv1beta1.MetricsConfig) error {
	log := log.WithName("PackageReports")
	p.maxBytes = *cr.Status.Packaging.MaxSize * megaByte
	p.uid = uuid.New().String()
	p.createdTimestamp = strings.Replace(time.Now().Format(timestampFormat), ".", "_", 1)

	// create reports/staging/upload directories if they do not exist
	if err := dirconfig.CheckExistsOrRecreate(p.DirCfg.Reports, p.DirCfg.Staging, p.DirCfg.Upload); err != nil {
		return fmt.Errorf("PackageReports: could not check directory: %v", err)
	}

	// move CSV reports from data directory to staging directory
	filesToPackage, err := p.moveOrCopyFiles(cr)
	if errors.Is(err, ErrNoReports) {
		log.Info("no payload to generate")
		return nil
	} else if err != nil {
		return fmt.Errorf("PackageReports: %v", err)
	}
	// get the start and end dates from the report
	log.Info("getting the start and end intervals for the manifest")
	for _, file := range filesToPackage {
		absPath := filepath.Join(p.DirCfg.Staging.Path, file.Name())
		if strings.Contains(file.Name(), "cm-openshift-pod") || (p.start.IsZero() && strings.Contains(file.Name(), "ros-openshift")) {
			if err := p.getStartEnd(absPath); err != nil {
				return fmt.Errorf("PackageReports: %v", err)
			}
		}
	}

	// check if the files need to be split
	log.Info("checking to see if the report files need to be split")
	filesToPackage, split, err := p.splitFiles(p.DirCfg.Staging.Path, filesToPackage)
	if err != nil {
		return fmt.Errorf("PackageReports: %v", err)
	}
	tracker := p.buildLocalCSVFileList(filesToPackage, p.DirCfg.Staging.Path)
	p.getManifest(tracker, p.DirCfg.Staging.Path, cr)
	log.Info("rendering manifest", "manifest", p.manifest.filename)
	if err := p.manifest.renderManifest(); err != nil {
		return fmt.Errorf("PackageReports: %v", err)
	}

	filenameBase := p.createdTimestamp + "-cost-mgmt"

	if split {
		if err := p.writeSplitTarballSet(filenameBase, tracker); err != nil {
			return fmt.Errorf("PackageReports: %v", err)
		}
	} else {
		// #836 review P3: the sweep runs on every packaging pass — a
		// non-split retry must also clear stale split-set residue.
		if err := sweepOrphanedSplitArtifacts(p.DirCfg.Upload.Path); err != nil {
			return fmt.Errorf("PackageReports: sweep: %v", err)
		}
		tarFileName := filenameBase + ".tar.gz"
		if err := p.writeTarball(tarFileName, p.manifest.filename, tracker.allfiles); err != nil {
			return fmt.Errorf("PackageReports: %v", err)
		}
	}

	log.Info("file packaging was successful")
	// #766: staging is removed only after successful finalized packaging —
	// every report now exists inside the finalized archive(s) in upload.
	if err := p.DirCfg.Staging.RemoveContents(); err != nil {
		return fmt.Errorf("PackageReports: could not clear staging after successful packaging: %v", err)
	}
	cr.Status.Packaging.LastSuccessfulPackagingTime = metav1.Now()
	return nil
}

func (p *FilePackager) GetFileInfo(file string) (FileInfoManifest, error) {
	log := log.WithName("getFileInfo")
	fileInfo := FileInfoManifest{}
	openFile, err := os.Open(file)
	if err != nil {
		log.Info("Could not open tar.gz")
		return fileInfo, err
	}
	archive, err := gzip.NewReader(openFile)
	if err != nil {
		log.Info("Could not read the tar.gz")
		return fileInfo, err
	}
	tr := tar.NewReader(archive)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fileInfo, err
		}
		if hdr.Name != "manifest.json" {
			continue
		}
		//Using a bytes buffer is an important part to print the values as a string
		buff := new(bytes.Buffer)
		_, err = buff.ReadFrom(tr)
		if err != nil {
			log.Info("Could not read from the buffer.")
			return fileInfo, err
		}
		s := buff.String()
		if err := json.Unmarshal([]byte(s), &fileInfo); err != nil {
			log.Info("Could not load the manifest json.")
			return fileInfo, err
		}
		break // exit `for` loop after processing the manifest.json
	}
	return fileInfo, nil
}

// createCSVUploadName creates a csv filename from the base file name that reflects file contents.
func (p *FilePackager) createCSVUploadName(filePath string, idx int) string {
	baseFileName := filepath.Base(filePath)
	fileNameWithoutExt := strings.TrimSuffix(baseFileName, filepath.Ext(baseFileName))
	uploadName := fileNameWithoutExt + "." + strconv.Itoa(idx) + ".csv"
	return uploadName
}
