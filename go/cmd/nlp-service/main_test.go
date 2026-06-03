package main

import (
	"testing"

	"github.com/stonedt-yuqing/go-jin10/internal/testutil"
)

func TestMainWiresNLPService(t *testing.T) {
	testutil.AssertFileContains(t, "main.go",
		`app.LogStartup("nlp-service"`,
		`nlp.NewService().Router()`,
	)
}
