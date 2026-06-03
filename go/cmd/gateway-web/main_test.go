package main

import (
	"testing"

	"github.com/stonedt-yuqing/go-jin10/internal/testutil"
)

func TestMainWiresGatewayWeb(t *testing.T) {
	testutil.AssertFileContains(t, "main.go",
		`app.LogStartup("gateway-web"`,
		`portal.NewServer(cfg).Router()`,
	)
}
