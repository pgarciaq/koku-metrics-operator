package collector

import (
	"testing"
)

func TestRosContainerRow_OOMCountInHeader(t *testing.T) {
	header := rosContainerRow{}.csvHeader()

	oomIdx := -1
	for i, col := range header {
		if col == "oom_count" {
			oomIdx = i
			break
		}
	}
	if oomIdx < 0 {
		t.Fatal("oom_count must be present in csvHeader()")
	}
	if oomIdx > 0 && header[oomIdx-1] != "memory_rss_usage_container_sum" {
		t.Errorf("oom_count should follow memory_rss_usage_container_sum, got %q", header[oomIdx-1])
	}
	if oomIdx < len(header)-1 && header[oomIdx+1] != "accelerator_model_name" {
		t.Errorf("oom_count should precede accelerator_model_name, got %q", header[oomIdx+1])
	}
}

func TestRosContainerRow_OOMCountInRow(t *testing.T) {
	row := rosContainerRow{
		dateTimes: &dateTimes{
			ReportPeriodStart: "2026-03-01",
			ReportPeriodEnd:   "2026-04-01",
			IntervalStart:     "2026-03-15 10:00:00",
			IntervalEnd:       "2026-03-15 10:15:00",
		},
		OOMCount: "3",
	}

	csvRow := row.csvRow()
	header := row.csvHeader()

	if len(header) != len(csvRow) {
		t.Fatalf("csvRow length %d != csvHeader length %d", len(csvRow), len(header))
	}

	oomIdx := -1
	for i, col := range header {
		if col == "oom_count" {
			oomIdx = i
			break
		}
	}
	if oomIdx < 0 {
		t.Fatal("oom_count must be in header")
	}
	if csvRow[oomIdx] != "3" {
		t.Errorf("csvRow OOM count = %q, want %q", csvRow[oomIdx], "3")
	}
}

func TestRosContainerRow_OOMCountZero(t *testing.T) {
	row := rosContainerRow{
		dateTimes: &dateTimes{},
		OOMCount:  "0",
	}

	csvRow := row.csvRow()
	header := row.csvHeader()

	oomIdx := -1
	for i, col := range header {
		if col == "oom_count" {
			oomIdx = i
			break
		}
	}
	if oomIdx < 0 {
		t.Fatal("oom_count must be in header")
	}
	if csvRow[oomIdx] != "0" {
		t.Errorf("zero OOM count = %q, want %q", csvRow[oomIdx], "0")
	}
}
