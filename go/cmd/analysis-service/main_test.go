package main

import (
	"testing"

	"github.com/stonedt-yuqing/go-jin10/internal/testutil"
)

func TestMainWiresAnalysisService(t *testing.T) {
	testutil.AssertFileContains(t, "main.go",
		`app.LogStartup("analysis-service"`,
		`analysis.NewService(cfg, store).Router()`,
	)
}
