package model

import (
	"encoding/json"
	"testing"
)

func TestItemOmitsEmptyProjectIDs(t *testing.T) {
	item := Item{ID: 1, Title: "hello"}

	body, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("failed to marshal item: %v", err)
	}
	if string(body) == "" {
		t.Fatal("expected non-empty json")
	}
	if containsProjectIDs(body) {
		t.Fatalf("expected project_ids to be omitted, got %s", body)
	}
}

func TestItemIncludesProjectIDsWhenPresent(t *testing.T) {
	item := Item{ID: 1, Title: "hello", ProjectIDs: []int64{3, 4}}

	body, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("failed to marshal item: %v", err)
	}
	if !containsProjectIDs(body) {
		t.Fatalf("expected project_ids in json, got %s", body)
	}
}

func containsProjectIDs(body []byte) bool {
	return string(body) != "" && jsonContains(string(body), `"project_ids"`)
}

func jsonContains(body, target string) bool {
	return len(body) >= len(target) && (body == target || len(body) > len(target) && (containsAt(body, target)))
}

func containsAt(body, target string) bool {
	for idx := 0; idx+len(target) <= len(body); idx++ {
		if body[idx:idx+len(target)] == target {
			return true
		}
	}
	return false
}
