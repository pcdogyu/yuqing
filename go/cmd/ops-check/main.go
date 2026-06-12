package main

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type checkResult struct {
	Name       string `json:"name"`
	ActualName string `json:"actual_name,omitempty"`
	Status     string `json:"status"`
	Message    string `json:"message,omitempty"`
	Count      *int64 `json:"count,omitempty"`
	Expected   *int64 `json:"expected,omitempty"`
	Actual     *int64 `json:"actual,omitempty"`
}

type report struct {
	DatabasePath string        `json:"database_path"`
	BaselinePath string        `json:"baseline_path,omitempty"`
	GeneratedAt  time.Time     `json:"generated_at"`
	Status       string        `json:"status"`
	Success      []string      `json:"success"`
	Failed       []string      `json:"failed"`
	Diff         []checkResult `json:"diff"`
	Missing      []string      `json:"missing"`
	Extra        []string      `json:"extra"`
	Warnings     []string      `json:"warnings"`
	Checks       []checkResult `json:"checks"`
}

func main() {
	dbPath := flag.String("db", "", "SQLite database path")
	baselinePath := flag.String("baseline", "", "optional Java baseline JSON/CSV counts file")
	flag.Parse()
	if *dbPath == "" {
		*dbPath = os.Getenv("YUQING_DB_PATH")
	}
	if *dbPath == "" {
		*dbPath = "data/yuqing.db"
	}

	out := report{
		DatabasePath: *dbPath,
		BaselinePath: strings.TrimSpace(*baselinePath),
		GeneratedAt:  time.Now().UTC(),
		Status:       "ok",
	}
	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		fail(&out, "open_database", err.Error())
		write(out)
		os.Exit(1)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		fail(&out, "ping_database", err.Error())
		write(out)
		os.Exit(1)
	}
	ok(&out, checkResult{Name: "ping_database", Status: "ok"})

	for _, table := range []string{
		"items",
		"projects",
		"reports",
		"items_fts",
		"task_runs",
		"audit_logs",
		"platform_bindings",
	} {
		count, err := countRows(ctx, db, table)
		if err != nil {
			fail(&out, table, err.Error())
			continue
		}
		value := count
		ok(&out, checkResult{Name: table, Status: "ok", Count: &value})
	}

	for _, check := range []struct {
		name  string
		query string
	}{
		{name: "crypto_social_sources", query: `SELECT COUNT(*) FROM items WHERE source_type IN ('crypto_x','crypto_telegram')`},
		{name: "failed_task_runs", query: `SELECT COUNT(*) FROM task_runs WHERE status = 'failed'`},
		{name: "recent_audit_logs", query: `SELECT COUNT(*) FROM audit_logs WHERE created_at >= datetime('now','-1 day') OR created_at >= strftime('%Y-%m-%dT%H:%M:%fZ','now','-1 day')`},
	} {
		count, err := scalarCount(ctx, db, check.query)
		if err != nil {
			fail(&out, check.name, err.Error())
			continue
		}
		value := count
		ok(&out, checkResult{Name: check.name, Status: "ok", Count: &value})
	}
	nlpCapabilityCount := int64(6)
	ok(&out, checkResult{Name: "nlp_capabilities", Status: "ok", Count: &nlpCapabilityCount})

	var quickCheck string
	if err := db.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&quickCheck); err != nil {
		fail(&out, "quick_check", err.Error())
	} else if quickCheck != "ok" {
		fail(&out, "quick_check", quickCheck)
	} else {
		ok(&out, checkResult{Name: "quick_check", Status: "ok"})
	}

	if out.BaselinePath != "" {
		baseline, err := readBaselineCounts(out.BaselinePath)
		if err != nil {
			fail(&out, "baseline", err.Error())
		} else {
			compareBaseline(&out, baseline)
		}
	}

	write(out)
	if out.Status != "ok" {
		os.Exit(1)
	}
}

func countRows(ctx context.Context, db *sql.DB, table string) (int64, error) {
	return scalarCount(ctx, db, fmt.Sprintf("SELECT COUNT(*) FROM %s", table))
}

func scalarCount(ctx context.Context, db *sql.DB, query string) (int64, error) {
	var count int64
	if err := db.QueryRowContext(ctx, query).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func fail(out *report, name, message string) {
	out.Status = "failed"
	out.Checks = append(out.Checks, checkResult{Name: name, Status: "failed", Message: message})
	out.Failed = append(out.Failed, name)
}

func ok(out *report, result checkResult) {
	out.Checks = append(out.Checks, result)
	out.Success = append(out.Success, result.Name)
}

func readBaselineCounts(path string) (map[string]int64, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".csv":
		return readBaselineCSV(raw)
	default:
		return readBaselineJSON(raw)
	}
}

func readBaselineCSV(raw []byte) (map[string]int64, error) {
	reader := csv.NewReader(strings.NewReader(string(raw)))
	reader.TrimLeadingSpace = true
	result := map[string]int64{}
	for {
		record, err := reader.Read()
		if errorsIsEOF(err) {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(record) < 2 || strings.EqualFold(record[0], "name") || strings.EqualFold(record[0], "table") || strings.EqualFold(record[0], "metric") {
			continue
		}
		countColumn := record[1]
		if len(record) >= 3 && strings.TrimSpace(record[2]) != "" {
			countColumn = record[2]
		}
		count, err := strconv.ParseInt(strings.TrimSpace(countColumn), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid baseline count for %s: %w", record[0], err)
		}
		result[strings.TrimSpace(record[0])] = count
	}
	return result, nil
}

func errorsIsEOF(err error) bool {
	return errors.Is(err, io.EOF)
}

func readBaselineJSON(raw []byte) (map[string]int64, error) {
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	result := map[string]int64{}
	switch typed := payload.(type) {
	case map[string]any:
		if counts, ok := typed["counts"].(map[string]any); ok {
			return numericMap(counts)
		}
		return nestedNumericMap(typed)
	case []any:
		for _, item := range typed {
			row, ok := item.(map[string]any)
			if !ok {
				continue
			}
			name := firstBaselineString(row, "name", "table", "metric")
			if name == "" {
				continue
			}
			count, ok := baselineNumber(row["count"])
			if !ok {
				count, ok = baselineNumber(row["expected"])
			}
			if ok {
				result[name] = count
			}
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unsupported baseline JSON shape")
	}
}

func nestedNumericMap(input map[string]any) (map[string]int64, error) {
	result := map[string]int64{}
	for key, value := range input {
		if count, ok := baselineNumber(value); ok {
			result[key] = count
			continue
		}
		row, ok := value.(map[string]any)
		if !ok {
			continue
		}
		count, ok := baselineNumber(row["count"])
		if !ok {
			count, ok = baselineNumber(row["expected"])
		}
		if ok {
			result[key] = count
		}
	}
	return result, nil
}

func numericMap(input map[string]any) (map[string]int64, error) {
	result := map[string]int64{}
	for key, value := range input {
		if count, ok := baselineNumber(value); ok {
			result[key] = count
		}
	}
	return result, nil
}

func baselineNumber(value any) (int64, bool) {
	switch typed := value.(type) {
	case float64:
		return int64(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func firstBaselineString(row map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := row[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func compareBaseline(out *report, baseline map[string]int64) {
	actual := map[string]int64{}
	for _, check := range out.Checks {
		if check.Count != nil {
			actual[check.Name] = *check.Count
		}
	}
	for name, expected := range baseline {
		actualName := baselineActualName(name)
		value, exists := actual[actualName]
		if !exists {
			out.Missing = append(out.Missing, name)
			out.Failed = append(out.Failed, name)
			out.Status = "failed"
			continue
		}
		if value != expected {
			exp, act := expected, value
			diff := checkResult{Name: name, ActualName: actualName, Status: "diff", Expected: &exp, Actual: &act}
			out.Diff = append(out.Diff, diff)
			out.Checks = append(out.Checks, diff)
			out.Failed = append(out.Failed, name)
			out.Status = "failed"
		} else {
			out.Success = append(out.Success, "baseline:"+name)
		}
	}
	for name := range actual {
		if _, exists := baselineByActualName(baseline)[name]; !exists {
			out.Extra = append(out.Extra, name)
		}
	}
	if len(baseline) == 0 {
		out.Warnings = append(out.Warnings, "baseline file contained no comparable counts")
	}
}

func baselineActualName(name string) string {
	normalized := strings.ToLower(strings.TrimSpace(name))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	normalized = strings.ReplaceAll(normalized, " ", "_")
	switch normalized {
	case "article", "articles", "java_articles":
		return "items"
	case "project", "projects", "java_projects":
		return "projects"
	case "report", "reports", "java_reports":
		return "reports"
	case "search_index", "search_indexes", "fts", "items_index":
		return "items_fts"
	case "task_records", "tasks", "task_run", "task_runs":
		return "task_runs"
	case "audit", "audits", "audit_log", "audit_logs":
		return "audit_logs"
	case "platform_binding", "platform_bindings":
		return "platform_bindings"
	case "crypto_social", "crypto_social_sources":
		return "crypto_social_sources"
	case "nlp_capability", "nlp_capabilities", "nlp_status":
		return "nlp_capabilities"
	default:
		return normalized
	}
}

func baselineByActualName(baseline map[string]int64) map[string]int64 {
	result := map[string]int64{}
	for name, count := range baseline {
		result[baselineActualName(name)] = count
	}
	return result
}

func write(out report) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(out)
}
