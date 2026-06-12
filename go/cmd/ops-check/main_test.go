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

	nestedPath := filepath.Join(dir, "nested.json")
	if err := os.WriteFile(nestedPath, []byte(`{"articles":{"count":9},"search_index":{"expected":8}}`), 0o644); err != nil {
		t.Fatalf("write nested baseline: %v", err)
	}
	nestedCounts, err := readBaselineCounts(nestedPath)
	if err != nil {
		t.Fatalf("read nested baseline: %v", err)
	}
	if nestedCounts["articles"] != 9 || nestedCounts["search_index"] != 8 {
		t.Fatalf("unexpected nested baseline counts: %+v", nestedCounts)
	}

	csvPath := filepath.Join(dir, "baseline.csv")
	if err := os.WriteFile(csvPath, []byte("metric,label,expected\nitems,文章,3\nreports,报告,4\n"), 0o644); err != nil {
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
	nlpCapabilities := int64(6)
	out := &report{
		Status: "ok",
		Checks: []checkResult{
			{Name: "items", Status: "ok", Count: &items},
			{Name: "reports", Status: "ok", Count: &reports},
			{Name: "nlp_capabilities", Status: "ok", Count: &nlpCapabilities},
		},
	}
	compareBaseline(out, map[string]int64{"articles": 3, "projects": 1, "nlp_status": 6})
	if out.Status != "failed" {
		t.Fatalf("expected failed status, got %s", out.Status)
	}
	if len(out.Diff) != 1 || out.Diff[0].Name != "articles" || out.Diff[0].ActualName != "items" {
		t.Fatalf("expected articles/items diff, got %+v", out.Diff)
	}
	if len(out.Missing) != 1 || out.Missing[0] != "projects" {
		t.Fatalf("expected projects missing, got %+v", out.Missing)
	}
	if len(out.Extra) != 1 || out.Extra[0] != "reports" {
		t.Fatalf("expected reports extra, got %+v", out.Extra)
	}
	if !containsString(out.Success, "baseline:nlp_status") {
		t.Fatalf("expected nlp_status baseline success, got %+v", out.Success)
	}
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
