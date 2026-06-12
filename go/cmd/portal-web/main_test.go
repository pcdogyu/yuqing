package main

import (
	"testing"

	"github.com/pcdogyu/yuqing/go/internal/testutil"
)

func TestMainWiresPortalWeb(t *testing.T) {
	testutil.AssertFileContains(t, "main.go",
		`app.LogStartup("portal-web"`,
		`portal.NewServer(cfg).Router()`,
	)
}
