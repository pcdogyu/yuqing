package release

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/pcdogyu/yuqing/go/internal/apiutil"
	"github.com/pcdogyu/yuqing/go/internal/config"
)

const serviceName = "release-service"

var errNoReleaseAPK = errors.New("no release apk found")

type Service struct {
	dir     string
	baseURL string
}

type APKMetadata struct {
	VersionName string    `json:"version_name"`
	VersionCode int       `json:"version_code"`
	FileName    string    `json:"file_name"`
	DownloadURL string    `json:"download_url"`
	SizeBytes   int64     `json:"size_bytes"`
	SHA256      string    `json:"sha256"`
	ModifiedAt  time.Time `json:"modified_at"`
}

type apkFile struct {
	name    string
	path    string
	size    int64
	modTime time.Time
}

func NewService(cfg config.Config) *Service {
	return &Service{
		dir:     strings.TrimSpace(cfg.ReleaseDir),
		baseURL: strings.TrimRight(strings.TrimSpace(cfg.ReleaseURL), "/"),
	}
}

func (s *Service) Router() http.Handler {
	r := chi.NewRouter()
	r.Get("/healthz", s.handleHealthz)
	r.Get("/healthy", s.handleHealthz)
	r.Get("/api/v1/release/latest", s.handleLatest)
	r.Get("/release", s.redirectReleaseRoot)
	r.Get("/release/", s.handleReleaseList)
	r.Get("/release/{filename}", s.handleReleaseFile)
	return r
}

func (s *Service) handleHealthz(w http.ResponseWriter, r *http.Request) {
	apiutil.WriteHealth(w, serviceName, "ok", "ok")
}

func (s *Service) redirectReleaseRoot(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/release/", http.StatusMovedPermanently)
}

func (s *Service) handleReleaseList(w http.ResponseWriter, r *http.Request) {
	files, err := os.ReadDir(s.releaseDir())
	if err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Name() < files[j].Name()
	})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, "<!doctype html><html><head><meta charset=\"utf-8\"><title>Yuqing Releases</title></head><body><h1>Yuqing Releases</h1><ul>")
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		name := file.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		escaped := url.PathEscape(name)
		_, _ = fmt.Fprintf(w, "<li><a href=\"/release/%s\">%s</a></li>", escaped, htmlEscape(name))
	}
	_, _ = io.WriteString(w, "</ul></body></html>")
}

func (s *Service) handleReleaseFile(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(chi.URLParam(r, "filename"))
	if name == "" || name != filepath.Base(name) {
		apiutil.WriteJSON(w, http.StatusBadRequest, "invalid filename", nil)
		return
	}
	path := filepath.Join(s.releaseDir(), name)
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		apiutil.WriteJSON(w, http.StatusNotFound, "not found", nil)
		return
	}
	http.ServeFile(w, r, path)
}

func (s *Service) handleLatest(w http.ResponseWriter, r *http.Request) {
	metadata, err := s.LatestAPK()
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, errNoReleaseAPK) {
			status = http.StatusNotFound
		}
		apiutil.WriteJSON(w, status, err.Error(), nil)
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, "ok", metadata)
}

func (s *Service) LatestAPK() (APKMetadata, error) {
	files, err := s.apkFiles()
	if err != nil {
		return APKMetadata{}, err
	}
	if len(files) == 0 {
		return APKMetadata{}, errNoReleaseAPK
	}
	file := files[0]
	sum, err := fileSHA256(file.path)
	if err != nil {
		return APKMetadata{}, err
	}
	versionName, versionCode := parseAPKVersion(file.name)
	return APKMetadata{
		VersionName: versionName,
		VersionCode: versionCode,
		FileName:    file.name,
		DownloadURL: s.downloadURL(file.name),
		SizeBytes:   file.size,
		SHA256:      sum,
		ModifiedAt:  file.modTime.UTC(),
	}, nil
}

func (s *Service) apkFiles() ([]apkFile, error) {
	entries, err := os.ReadDir(s.releaseDir())
	if err != nil {
		return nil, err
	}
	files := make([]apkFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".apk") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		files = append(files, apkFile{
			name:    entry.Name(),
			path:    filepath.Join(s.releaseDir(), entry.Name()),
			size:    info.Size(),
			modTime: info.ModTime(),
		})
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].modTime.Equal(files[j].modTime) {
			return files[i].name > files[j].name
		}
		return files[i].modTime.After(files[j].modTime)
	})
	return files, nil
}

func (s *Service) releaseDir() string {
	if strings.TrimSpace(s.dir) == "" {
		return "release"
	}
	return s.dir
}

func (s *Service) downloadURL(fileName string) string {
	base := s.baseURL
	if base == "" {
		base = "http://127.0.0.1:8099"
	}
	return base + "/release/" + url.PathEscape(fileName)
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func parseAPKVersion(fileName string) (string, int) {
	name := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	name = strings.TrimPrefix(name, "yuqing-")
	name = strings.TrimSuffix(name, "-release")
	name = strings.TrimSuffix(name, "-debug")
	return name, 0
}

func htmlEscape(value string) string {
	value = strings.ReplaceAll(value, "&", "&amp;")
	value = strings.ReplaceAll(value, "<", "&lt;")
	value = strings.ReplaceAll(value, ">", "&gt;")
	value = strings.ReplaceAll(value, "\"", "&quot;")
	return value
}
