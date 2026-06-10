package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func Setup(level, serviceName string) {
	consoleOutput := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: time.RFC3339,
	}
	zerolog.TimeFieldFormat = time.RFC3339
	writer := zerolog.MultiLevelWriter(consoleOutput)
	if fileWriter, cleanup, err := newFileWriter(serviceName); err == nil {
		writer = zerolog.MultiLevelWriter(consoleOutput, fileWriter)
		log.Logger = zerolog.New(writer).With().Timestamp().Logger()
		log.Debug().Str("service", serviceName).Msg("file logging enabled")
		_ = cleanup
	} else {
		log.Logger = zerolog.New(writer).With().Timestamp().Logger()
		log.Warn().Err(err).Str("service", serviceName).Msg("file logging disabled")
	}

	switch strings.ToLower(level) {
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "warn":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
}

func newFileWriter(serviceName string) (zerolog.ConsoleWriter, func() error, error) {
	logPath, err := resolveLogPath(serviceName)
	if err != nil {
		return zerolog.ConsoleWriter{}, nil, err
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return zerolog.ConsoleWriter{}, nil, fmt.Errorf("create log dir: %w", err)
	}
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return zerolog.ConsoleWriter{}, nil, fmt.Errorf("open log file: %w", err)
	}
	return zerolog.ConsoleWriter{
		Out:        file,
		TimeFormat: time.RFC3339,
	}, file.Close, nil
}

func resolveLogPath(serviceName string) (string, error) {
	serviceName = strings.TrimSpace(serviceName)
	if serviceName == "" {
		serviceName = "service"
	}
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve executable path: %w", err)
	}
	exeDir := filepath.Dir(exePath)
	rootDir := filepath.Dir(exeDir)
	return filepath.Join(rootDir, "runtime-logs", serviceName+".out.log"), nil
}
