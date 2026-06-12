package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	_ "modernc.org/sqlite"
)

type checkResult struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	Count   *int64 `json:"count,omitempty"`
}

type report struct {
	DatabasePath string        `json:"database_path"`
	GeneratedAt  time.Time     `json:"generated_at"`
	Status       string        `json:"status"`
	Checks       []checkResult `json:"checks"`
}

func main() {
	dbPath := flag.String("db", "", "SQLite database path")
	flag.Parse()
	if *dbPath == "" {
		*dbPath = os.Getenv("YUQING_DB_PATH")
	}
	if *dbPath == "" {
		*dbPath = "data/yuqing.db"
	}

	out := report{
		DatabasePath: *dbPath,
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
	out.Checks = append(out.Checks, checkResult{Name: "ping_database", Status: "ok"})

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
		out.Checks = append(out.Checks, checkResult{Name: table, Status: "ok", Count: &value})
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
		out.Checks = append(out.Checks, checkResult{Name: check.name, Status: "ok", Count: &value})
	}

	var quickCheck string
	if err := db.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&quickCheck); err != nil {
		fail(&out, "quick_check", err.Error())
	} else if quickCheck != "ok" {
		fail(&out, "quick_check", quickCheck)
	} else {
		out.Checks = append(out.Checks, checkResult{Name: "quick_check", Status: "ok"})
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
}

func write(out report) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(out)
}
