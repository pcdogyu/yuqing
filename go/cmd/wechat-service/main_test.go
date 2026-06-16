package main

import (
	"testing"

	"github.com/pcdogyu/yuqing/go/internal/testutil"
)

func TestMainWiresWechatService(t *testing.T) {
	testutil.AssertFileContains(t, "main.go",
		`app.LogStartup("wechat-service"`,
		`auth.NewService(cfg, store).WechatRouter()`,
	)
}
