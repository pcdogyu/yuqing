package main

import (
	"testing"

	"github.com/pcdogyu/yuqing/go/internal/testutil"
)

func TestMainWiresContentService(t *testing.T) {
	testutil.AssertFileContains(t, "main.go",
		`app.LogStartup("content-service"`,
		`content.NewService(cfg, store)`,
		`contentSvc.Router()`,
	)
}
