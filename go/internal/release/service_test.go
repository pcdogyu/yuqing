package release

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/config"
)

func TestHealthz(t *testing.T) {
	svc := NewService(config.Config{ReleaseDir: t.TempDir(), ReleaseURL: "http://release.example.com"})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()

	svc.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"service":"release-service"`) {
		t.Fatalf("unexpected health response status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestReleaseListShowsFiles(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "yuqing-20260625-120000-abcdef12-release.apk", "apk")
	writeTestFile(t, dir, "notes.txt", "notes")
	svc := NewService(config.Config{ReleaseDir: dir, ReleaseURL: "http://release.example.com"})
	req := httptest.NewRequest(http.MethodGet, "/release/", nil)
	rr := httptest.NewRecorder()

	svc.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if body := rr.Body.String(); !strings.Contains(body, "yuqing-20260625-120000-abcdef12-release.apk") || !strings.Contains(body, "notes.txt") {
		t.Fatalf("expected release files in listing, got %s", body)
	}
}

func TestLatestAPKUsesNewestAPKAndBaseURL(t *testing.T) {
	dir := t.TempDir()
	oldPath := writeTestFile(t, dir, "yuqing-20260624-120000-aaaaaaaa-release.apk", "old")
	newPath := writeTestFile(t, dir, "yuqing-20260625-120000-bbbbbbbb-release.apk", "new")
	writeTestFile(t, dir, "readme.txt", "ignore")
	setModTime(t, oldPath, time.Date(2026, 6, 24, 4, 0, 0, 0, time.UTC))
	setModTime(t, newPath, time.Date(2026, 6, 25, 4, 0, 0, 0, time.UTC))
	svc := NewService(config.Config{ReleaseDir: dir, ReleaseURL: "http://release.example.com:8099"})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/release/latest", nil)
	rr := httptest.NewRecorder()

	svc.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var envelope struct {
		Data APKMetadata `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode latest response: %v", err)
	}
	if envelope.Data.FileName != "yuqing-20260625-120000-bbbbbbbb-release.apk" {
		t.Fatalf("unexpected latest apk: %+v", envelope.Data)
	}
	if envelope.Data.DownloadURL != "http://release.example.com:8099/release/yuqing-20260625-120000-bbbbbbbb-release.apk" {
		t.Fatalf("unexpected download url: %s", envelope.Data.DownloadURL)
	}
	if envelope.Data.SHA256 == "" || envelope.Data.SizeBytes != 3 {
		t.Fatalf("expected checksum and size, got %+v", envelope.Data)
	}
}

func TestLatestAPKReturns404WhenEmpty(t *testing.T) {
	svc := NewService(config.Config{ReleaseDir: t.TempDir(), ReleaseURL: "http://release.example.com"})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/release/latest", nil)
	rr := httptest.NewRecorder()

	svc.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound || !strings.Contains(rr.Body.String(), "no release apk found") {
		t.Fatalf("expected empty release 404, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func writeTestFile(t *testing.T, dir string, name string, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func setModTime(t *testing.T, path string, value time.Time) {
	t.Helper()
	if err := os.Chtimes(path, value, value); err != nil {
		t.Fatalf("chtimes %s: %v", path, err)
	}
}
