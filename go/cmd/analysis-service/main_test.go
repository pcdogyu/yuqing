package main

import (
	"testing"

	"github.com/pcdogyu/yuqing/go/internal/testutil"
)

func TestMainWiresAnalysisService(t *testing.T) {
	testutil.AssertFileContains(t, "main.go",
		`app.LogStartup("analysis-service"`,
		`analysis.NewService(cfg, store).Router()`,
	)
}
