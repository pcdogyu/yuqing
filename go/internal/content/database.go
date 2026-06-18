package content

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/pcdogyu/yuqing/go/internal/apiutil"
	"github.com/pcdogyu/yuqing/go/internal/model"
)

type databaseConfigRequest struct {
	Driver           string `json:"driver"`
	SQLitePath       string `json:"sqlite_path"`
	PostgresDSN      string `json:"postgres_dsn"`
	PostgresHost     string `json:"postgres_host"`
	PostgresPort     string `json:"postgres_port"`
	PostgresDatabase string `json:"postgres_database"`
	PostgresUser     string `json:"postgres_user"`
	PostgresPassword string `json:"postgres_password"`
	PostgresSSLMode  string `json:"postgres_sslmode"`
}

type runtimeDatabaseConfigFile struct {
	Driver           string `json:"driver"`
	SQLitePath       string `json:"sqlite_path,omitempty"`
	PostgresDSN      string `json:"postgres_dsn,omitempty"`
	PostgresHost     string `json:"postgres_host,omitempty"`
	PostgresPort     string `json:"postgres_port,omitempty"`
	PostgresDatabase string `json:"postgres_database,omitempty"`
	PostgresUser     string `json:"postgres_user,omitempty"`
	PostgresPassword string `json:"postgres_password,omitempty"`
	PostgresSSLMode  string `json:"postgres_sslmode,omitempty"`
	UpdatedAt        string `json:"updated_at,omitempty"`
}

func (s *Service) handleDatabaseConfig(w http.ResponseWriter, r *http.Request) {
	apiutil.WriteJSON(w, http.StatusOK, "ok", s.databaseConfigStatus(r.Context(), databaseConfigRequest{}))
}

func (s *Service) handleDatabaseCheck(w http.ResponseWriter, r *http.Request) {
	var req databaseConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiutil.WriteJSON(w, http.StatusBadRequest, err.Error(), nil)
		return
	}
	status := s.databaseConfigStatus(r.Context(), req)
	code := http.StatusOK
	if status.Status == "failed" {
		code = http.StatusBadGateway
	}
	apiutil.WriteJSON(w, code, status.Message, status)
}

func (s *Service) handleDatabaseSave(w http.ResponseWriter, r *http.Request) {
	var req databaseConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiutil.WriteJSON(w, http.StatusBadRequest, err.Error(), nil)
		return
	}
	cfg := s.databaseConfigFromRequest(req)
	driver := normalizeDatabaseDriver(cfg.Driver)
	if driver == "" {
		driver = "sqlite"
	}
	if driver != "sqlite" && driver != "postgres" {
		apiutil.WriteJSON(w, http.StatusBadRequest, "unsupported database driver: "+driver, nil)
		return
	}
	cfg.Driver = driver
	if strings.TrimSpace(cfg.SQLitePath) == "" {
		cfg.SQLitePath = filepath.Join("data", "yuqing.db")
	}
	configPath := runtimeDatabaseConfigPath(s.cfg.DatabaseConfigPath)
	if err := writeRuntimeDatabaseConfig(configPath, cfg); err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	status := s.databaseConfigStatus(r.Context(), cfg)
	status.ConfiguredDriver = driver
	status.ConfigPath = configPath
	status.RestartRequired = normalizeDatabaseDriver(s.cfg.DatabaseDriver) != driver
	status.Message = "database connection config saved"
	apiutil.WriteJSON(w, http.StatusOK, status.Message, status)
}

func (s *Service) handleDatabaseSwitch(w http.ResponseWriter, r *http.Request) {
	var req databaseConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiutil.WriteJSON(w, http.StatusBadRequest, err.Error(), nil)
		return
	}
	cfg := s.databaseConfigFromRequest(req)
	configPath := runtimeDatabaseConfigPath(s.cfg.DatabaseConfigPath)
	driver := normalizeDatabaseDriver(cfg.Driver)
	if driver == "" {
		driver = "sqlite"
	}
	cfg.Driver = driver
	switch driver {
	case "sqlite":
		if strings.TrimSpace(cfg.SQLitePath) == "" {
			cfg.SQLitePath = filepath.Join("data", "yuqing.db")
		}
	case "postgres":
		status, message := checkPostgres(r.Context(), cfg)
		if status != "ok" {
			apiutil.WriteJSON(w, http.StatusBadGateway, message, model.DatabaseConfigStatus{
				Driver:           driver,
				ConfiguredDriver: currentConfiguredDatabaseDriver(configPath),
				RuntimeDriver:    "sqlite",
				Status:           status,
				Message:          message,
				ConfigPath:       configPath,
				RestartRequired:  true,
			})
			return
		}
	default:
		apiutil.WriteJSON(w, http.StatusBadRequest, "unsupported database driver: "+driver, nil)
		return
	}
	if err := writeRuntimeDatabaseConfig(configPath, cfg); err != nil {
		apiutil.WriteJSON(w, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	restartErr := startAllServicesRestart()
	status := s.databaseConfigStatus(r.Context(), databaseConfigRequest{})
	status.ConfiguredDriver = driver
	status.ConfigPath = configPath
	status.RestartRequired = true
	status.Message = "database switch saved; restarting all services to apply"
	if restartErr != nil {
		status.Message = "database switch saved; failed to submit service restart: " + restartErr.Error()
	}
	apiutil.WriteJSON(w, http.StatusOK, status.Message, status)
}

func (s *Service) databaseConfigStatus(ctx context.Context, override databaseConfigRequest) model.DatabaseConfigStatus {
	cfg := s.databaseConfigFromRequest(override)
	configPath := runtimeDatabaseConfigPath(s.cfg.DatabaseConfigPath)
	driver := normalizeDatabaseDriver(cfg.Driver)
	if driver == "" {
		driver = "sqlite"
	}
	runtimeDriver := normalizeDatabaseDriver(s.cfg.DatabaseDriver)
	if runtimeDriver == "" {
		runtimeDriver = "sqlite"
	}
	configuredDriver := currentConfiguredDatabaseDriver(configPath)
	if configuredDriver == "" {
		configuredDriver = driver
	}
	status := model.DatabaseConfigStatus{
		Driver:             driver,
		ConfiguredDriver:   configuredDriver,
		RuntimeDriver:      runtimeDriver,
		SQLitePath:         cfg.SQLitePath,
		ConfigPath:         configPath,
		RestartRequired:    configuredDriver != runtimeDriver,
		PostgresHost:       cfg.PostgresHost,
		PostgresPort:       cfg.PostgresPort,
		PostgresDatabase:   cfg.PostgresDatabase,
		PostgresUser:       cfg.PostgresUser,
		PostgresSSLMode:    cfg.PostgresSSLMode,
		PostgresConfigured: strings.TrimSpace(cfg.PostgresDSN) != "" || postgresPartsConfigured(cfg),
		PostgresDSN:        maskPostgresDSN(cfg.PostgresDSN),
	}

	switch driver {
	case "sqlite":
		status.Status = "ok"
		status.Message = sqliteStatusMessage(cfg.SQLitePath)
	case "postgres":
		status.Status, status.Message = checkPostgres(ctx, cfg)
		if status.Status == "ok" {
			if runtimeDriver == "postgres" {
				status.Message += "; runtime store is postgres"
			} else {
				status.Message += "; runtime store remains " + runtimeDriver
			}
		}
	default:
		status.Status = "failed"
		status.Message = "unsupported database driver: " + driver
	}
	return status
}

func (s *Service) databaseConfigFromRequest(req databaseConfigRequest) databaseConfigRequest {
	cfg := databaseConfigRequest{
		Driver:           s.cfg.DatabaseDriver,
		SQLitePath:       s.cfg.DatabasePath,
		PostgresDSN:      s.cfg.DatabaseURL,
		PostgresHost:     s.cfg.PostgresHost,
		PostgresPort:     s.cfg.PostgresPort,
		PostgresDatabase: s.cfg.PostgresDatabase,
		PostgresUser:     s.cfg.PostgresUser,
		PostgresPassword: s.cfg.PostgresPassword,
		PostgresSSLMode:  s.cfg.PostgresSSLMode,
	}
	if strings.TrimSpace(req.Driver) != "" {
		cfg.Driver = req.Driver
	}
	if strings.TrimSpace(req.SQLitePath) != "" {
		cfg.SQLitePath = req.SQLitePath
	}
	if strings.TrimSpace(req.PostgresDSN) != "" {
		cfg.PostgresDSN = req.PostgresDSN
	}
	if strings.TrimSpace(req.PostgresHost) != "" {
		cfg.PostgresHost = req.PostgresHost
	}
	if strings.TrimSpace(req.PostgresPort) != "" {
		cfg.PostgresPort = req.PostgresPort
	}
	if strings.TrimSpace(req.PostgresDatabase) != "" {
		cfg.PostgresDatabase = req.PostgresDatabase
	}
	if strings.TrimSpace(req.PostgresUser) != "" {
		cfg.PostgresUser = req.PostgresUser
	}
	if strings.TrimSpace(req.PostgresPassword) != "" {
		cfg.PostgresPassword = req.PostgresPassword
	}
	if strings.TrimSpace(req.PostgresSSLMode) != "" {
		cfg.PostgresSSLMode = req.PostgresSSLMode
	}
	if strings.TrimSpace(cfg.PostgresPort) == "" {
		cfg.PostgresPort = "5432"
	}
	if strings.TrimSpace(cfg.PostgresSSLMode) == "" {
		cfg.PostgresSSLMode = "disable"
	}
	return cfg
}

func normalizeDatabaseDriver(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "sqlite", "sqlite3":
		return "sqlite"
	case "postgres", "postgresql", "pg":
		return "postgres"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func sqliteStatusMessage(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		path = filepath.Join("data", "yuqing.db")
	}
	if _, err := os.Stat(path); err != nil {
		return "sqlite configured: " + path
	}
	return "sqlite ready: " + path
}

func postgresPartsConfigured(cfg databaseConfigRequest) bool {
	return strings.TrimSpace(cfg.PostgresHost) != "" &&
		strings.TrimSpace(cfg.PostgresDatabase) != "" &&
		strings.TrimSpace(cfg.PostgresUser) != ""
}

func postgresDSN(cfg databaseConfigRequest) string {
	if strings.TrimSpace(cfg.PostgresDSN) != "" {
		return strings.TrimSpace(cfg.PostgresDSN)
	}
	host := strings.TrimSpace(cfg.PostgresHost)
	port := strings.TrimSpace(cfg.PostgresPort)
	if port == "" {
		port = "5432"
	}
	database := strings.TrimSpace(cfg.PostgresDatabase)
	user := strings.TrimSpace(cfg.PostgresUser)
	sslMode := strings.TrimSpace(cfg.PostgresSSLMode)
	if sslMode == "" {
		sslMode = "disable"
	}
	u := url.URL{Scheme: "postgres", Host: net.JoinHostPort(host, port), Path: "/" + database}
	if strings.TrimSpace(cfg.PostgresPassword) != "" {
		u.User = url.UserPassword(user, cfg.PostgresPassword)
	} else if user != "" {
		u.User = url.User(user)
	}
	q := u.Query()
	q.Set("sslmode", sslMode)
	u.RawQuery = q.Encode()
	return u.String()
}

func checkPostgres(ctx context.Context, cfg databaseConfigRequest) (string, string) {
	if !postgresPartsConfigured(cfg) && strings.TrimSpace(cfg.PostgresDSN) == "" {
		return "warning", "PostgreSQL config is incomplete"
	}
	dsn := postgresDSN(cfg)
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return "failed", err.Error()
	}
	defer db.Close()
	if maxOpen := envInt("YUQING_POSTGRES_MAX_OPEN_CONNS", 10); maxOpen > 0 {
		db.SetMaxOpenConns(maxOpen)
	}
	if maxIdle := envInt("YUQING_POSTGRES_MAX_IDLE_CONNS", 5); maxIdle > 0 {
		db.SetMaxIdleConns(maxIdle)
	}
	if err := db.PingContext(checkCtx); err != nil {
		return "failed", err.Error()
	}
	return "ok", "postgresql connection ok"
}

func maskPostgresDSN(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User == nil {
		return raw
	}
	username := parsed.User.Username()
	if _, hasPassword := parsed.User.Password(); hasPassword {
		parsed.User = url.UserPassword(username, "redacted")
	}
	return parsed.String()
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func databaseEnvExample(cfg model.DatabaseConfigStatus) string {
	host := strings.TrimSpace(cfg.PostgresHost)
	if host == "" {
		host = "127.0.0.1"
	}
	port := strings.TrimSpace(cfg.PostgresPort)
	if port == "" {
		port = "5432"
	}
	database := strings.TrimSpace(cfg.PostgresDatabase)
	if database == "" {
		database = "yuqing"
	}
	user := strings.TrimSpace(cfg.PostgresUser)
	if user == "" {
		user = "postgres"
	}
	sslMode := strings.TrimSpace(cfg.PostgresSSLMode)
	if sslMode == "" {
		sslMode = "disable"
	}
	return fmt.Sprintf("$env:YUQING_DB_DRIVER='postgres'; $env:YUQING_POSTGRES_HOST='%s'; $env:YUQING_POSTGRES_PORT='%s'; $env:YUQING_POSTGRES_DB='%s'; $env:YUQING_POSTGRES_USER='%s'; $env:YUQING_POSTGRES_PASSWORD='<password>'; $env:YUQING_POSTGRES_SSLMODE='%s'", host, port, database, user, sslMode)
}

func currentConfiguredDatabaseDriver(path string) string {
	cfg, err := readRuntimeDatabaseConfig(path)
	if err != nil {
		return ""
	}
	return normalizeDatabaseDriver(cfg.Driver)
}

func runtimeDatabaseConfigPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return filepath.Join("data", "database-config.json")
	}
	return path
}

func readRuntimeDatabaseConfig(path string) (runtimeDatabaseConfigFile, error) {
	path = runtimeDatabaseConfigPath(path)
	if path == "" {
		return runtimeDatabaseConfigFile{}, os.ErrNotExist
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return runtimeDatabaseConfigFile{}, err
	}
	var cfg runtimeDatabaseConfigFile
	if err := json.Unmarshal(payload, &cfg); err != nil {
		return runtimeDatabaseConfigFile{}, err
	}
	return cfg, nil
}

func writeRuntimeDatabaseConfig(path string, cfg databaseConfigRequest) error {
	path = runtimeDatabaseConfigPath(path)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload := runtimeDatabaseConfigFile{
		Driver:           normalizeDatabaseDriver(cfg.Driver),
		SQLitePath:       strings.TrimSpace(cfg.SQLitePath),
		PostgresDSN:      strings.TrimSpace(cfg.PostgresDSN),
		PostgresHost:     strings.TrimSpace(cfg.PostgresHost),
		PostgresPort:     strings.TrimSpace(cfg.PostgresPort),
		PostgresDatabase: strings.TrimSpace(cfg.PostgresDatabase),
		PostgresUser:     strings.TrimSpace(cfg.PostgresUser),
		PostgresPassword: strings.TrimSpace(cfg.PostgresPassword),
		PostgresSSLMode:  strings.TrimSpace(cfg.PostgresSSLMode),
		UpdatedAt:        time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}
