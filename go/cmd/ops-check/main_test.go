package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadBaselineCounts(t *testing.T) {
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "baseline.json")
	if err := os.WriteFile(jsonPath, []byte(`{"counts":{"items":2,"projects":"1"}}`), 0o644); err != nil {
		t.Fatalf("write json baseline: %v", err)
	}
	jsonCounts, err := readBaselineCounts(jsonPath)
	if err != nil {
		t.Fatalf("read json baseline: %v", err)
	}
	if jsonCounts["items"] != 2 || jsonCounts["projects"] != 1 {
		t.Fatalf("unexpected json baseline counts: %+v", jsonCounts)
	}

	csvPath := filepath.Join(dir, "baseline.csv")
	if err := os.WriteFile(csvPath, []byte("name,count\nitems,3\nreports,4\n"), 0o644); err != nil {
		t.Fatalf("write csv baseline: %v", err)
	}
	csvCounts, err := readBaselineCounts(csvPath)
	if err != nil {
		t.Fatalf("read csv baseline: %v", err)
	}
	if csvCounts["items"] != 3 || csvCounts["reports"] != 4 {
		t.Fatalf("unexpected csv baseline counts: %+v", csvCounts)
	}
}

func TestCompareBaselineReportsDiffMissingAndExtra(t *testing.T) {
	items := int64(2)
	reports := int64(1)
	out := &report{
		Status: "ok",
		Checks: []checkResult{
			{Name: "items", Status: "ok", Count: &items},
			{Name: "reports", Status: "ok", Count: &reports},
		},
	}
	compareBaseline(out, map[string]int64{"items": 3, "projects": 1})
	if out.Status != "failed" {
		t.Fatalf("expected failed status, got %s", out.Status)
	}
	if len(out.Diff) != 1 || out.Diff[0].Name != "items" {
		t.Fatalf("expected items diff, got %+v", out.Diff)
	}
	if len(out.Missing) != 1 || out.Missing[0] != "projects" {
		t.Fatalf("expected projects missing, got %+v", out.Missing)
	}
	if len(out.Extra) != 1 || out.Extra[0] != "reports" {
		t.Fatalf("expected reports extra, got %+v", out.Extra)
	}
}
