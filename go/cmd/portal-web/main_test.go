package main

import (
	"testing"

	"github.com/stonedt-yuqing/go-jin10/internal/testutil"
)

func TestMainWiresPortalWeb(t *testing.T) {
	testutil.AssertFileContains(t, "main.go",
		`app.LogStartup("portal-web"`,
		`portal.NewServer(cfg).Router()`,
	)
}
