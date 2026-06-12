package main

import (
	"testing"

	"github.com/pcdogyu/yuqing/go/internal/testutil"
)

func TestMainWiresNLPService(t *testing.T) {
	testutil.AssertFileContains(t, "main.go",
		`app.LogStartup("nlp-service"`,
		`nlp.NewService().Router()`,
	)
}
