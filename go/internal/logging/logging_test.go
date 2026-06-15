package logging

import (
	"io"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func TestSetupSetsExpectedGlobalLevel(t *testing.T) {
	originalLevel := zerolog.GlobalLevel()
	originalLogger := log.Logger
	originalOutput := consoleWriterOutput
	originalFileWriterFactory := fileWriterFactory
	consoleWriterOutput = io.Discard
	fileWriterFactory = func(string) (zerolog.ConsoleWriter, func() error, error) {
		return zerolog.ConsoleWriter{Out: io.Discard}, func() error { return nil }, nil
	}
	t.Cleanup(func() {
		zerolog.SetGlobalLevel(originalLevel)
		log.Logger = originalLogger
		consoleWriterOutput = originalOutput
		fileWriterFactory = originalFileWriterFactory
	})

	cases := []struct {
		name  string
		input string
		want  zerolog.Level
	}{
		{name: "debug", input: "debug", want: zerolog.DebugLevel},
		{name: "warn", input: "warn", want: zerolog.WarnLevel},
		{name: "error", input: "error", want: zerolog.ErrorLevel},
		{name: "default", input: "unexpected", want: zerolog.InfoLevel},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			Setup(tc.input, "test-service")
			if got := zerolog.GlobalLevel(); got != tc.want {
				t.Fatalf("expected global level %s, got %s", tc.want, got)
			}
		})
	}
}
