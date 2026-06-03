package main

import (
	"testing"

	"github.com/stonedt-yuqing/go-jin10/internal/testutil"
)

func TestMainWiresContentService(t *testing.T) {
	testutil.AssertFileContains(t, "main.go",
		`app.LogStartup("content-service"`,
		`content.NewService(cfg, store)`,
		`contentSvc.Router()`,
	)
}
