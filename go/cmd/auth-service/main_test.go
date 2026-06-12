package main

import (
	"testing"

	"github.com/pcdogyu/yuqing/go/internal/testutil"
)

func TestMainWiresAuthService(t *testing.T) {
	testutil.AssertFileContains(t, "main.go",
		`app.LogStartup("auth-service"`,
		`auth.NewService(cfg, store).Router()`,
	)
}
