package content

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/apiutil"
	"github.com/pcdogyu/yuqing/go/internal/model"
)

type auditResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *auditResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *auditResponseWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(body)
}

func (s *Service) auditMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if skipAuditPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		started := time.Now()
		recorder := &auditResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)

		detail, _ := json.Marshal(map[string]any{
			"method":      r.Method,
			"path":        r.URL.Path,
			"query":       sanitizeQuery(r.URL.RawQuery),
			"status":      recorder.status,
			"duration_ms": time.Since(started).Milliseconds(),
			"ip":          clientIP(r),
			"user_agent":  r.UserAgent(),
			"module":      auditModule(r.URL.Path),
			"operation":   auditOperation(r.Method, r.URL.Path),
		})
		_, _ = s.store.CreateAuditLog(r.Context(), model.AuditLog{
			UserID:     auditUserID(r),
			Username:   strings.TrimSpace(r.Header.Get("X-User-Name")),
			Action:     "http." + strings.ToLower(r.Method),
			Resource:   r.URL.Path,
			DetailJSON: string(detail),
		})
	})
}

func skipAuditPath(path string) bool {
	return path == "/healthz" ||
		strings.HasPrefix(path, "/api/v1/system/audit-logs") ||
		strings.HasPrefix(path, "/api/v1/system/task-runs")
}

func auditUserID(r *http.Request) int64 {
	if userID := apiutil.UserIDFromContext(r.Context()); userID > 0 {
		return userID
	}
	userID, _ := strconv.ParseInt(strings.TrimSpace(r.Header.Get("X-User-ID")), 10, 64)
	return userID
}

func auditModule(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 3 && parts[0] == "api" && parts[1] == "v1" {
		return parts[2]
	}
	if len(parts) > 0 && parts[0] != "" {
		return parts[0]
	}
	return "system"
}

func auditOperation(method, path string) string {
	module := auditModule(path)
	switch method {
	case http.MethodPost:
		return module + ".create_or_run"
	case http.MethodPut, http.MethodPatch:
		return module + ".update"
	case http.MethodDelete:
		return module + ".delete"
	default:
		return module + ".read"
	}
}

func sanitizeQuery(raw string) string {
	if raw == "" {
		return ""
	}
	values := strings.Split(raw, "&")
	for i, value := range values {
		lower := strings.ToLower(value)
		if strings.Contains(lower, "password=") ||
			strings.Contains(lower, "token=") ||
			strings.Contains(lower, "secret") ||
			strings.Contains(lower, "key=") {
			key := strings.SplitN(value, "=", 2)[0]
			values[i] = key + "=<redacted>"
		}
	}
	return strings.Join(values, "&")
}

func clientIP(r *http.Request) string {
	for _, header := range []string{"X-Forwarded-For", "X-Real-IP"} {
		raw := strings.TrimSpace(r.Header.Get(header))
		if raw == "" {
			continue
		}
		first := strings.TrimSpace(strings.Split(raw, ",")[0])
		if first != "" {
			return first
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
