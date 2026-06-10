package logging

import (
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func TestSetupSetsExpectedGlobalLevel(t *testing.T) {
	originalLevel := zerolog.GlobalLevel()
	originalLogger := log.Logger
	t.Cleanup(func() {
		zerolog.SetGlobalLevel(originalLevel)
		log.Logger = originalLogger
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
