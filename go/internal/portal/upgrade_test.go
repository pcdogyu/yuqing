package portal

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/config"
)

func TestRunPortalUpgradeSkipsWhenAlreadyLatest(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required for upgrade integration test")
	}
	root := t.TempDir()
	remoteDir := filepath.Join(root, "remote.git")
	workDir := filepath.Join(root, "work")
	goDir := filepath.Join(workDir, "go")
	runGit(t, root, "init", "--bare", remoteDir)
	runGit(t, root, "clone", remoteDir, workDir)
	runGit(t, workDir, "config", "user.email", "codex@example.test")
	runGit(t, workDir, "config", "user.name", "Codex Test")
	if err := os.MkdirAll(goDir, 0o755); err != nil {
		t.Fatalf("create go dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(goDir, "go.mod"), []byte("module example.test/upgrade\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	runGit(t, workDir, "add", "go/go.mod")
	runGit(t, workDir, "commit", "-m", "init")
	runGit(t, workDir, "push", "-u", "origin", "HEAD")

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	if err := os.Chdir(goDir); err != nil {
		t.Fatalf("chdir worktree: %v", err)
	}
	defer func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	}()

	var progress []string
	result := runPortalUpgrade(context.Background(), config.Config{}, func(message string, _ string) {
		progress = append(progress, message)
	})

	if !result.OK || result.Status != "success" || result.Message != "已经是最新版本了" {
		t.Fatalf("expected already-latest success, got %+v", result)
	}
	for _, want := range []string{"阶段: 版本检测", "当前版本:", "最新版本:", "已经是最新版本了"} {
		if !strings.Contains(result.Log, want) {
			t.Fatalf("expected upgrade log to include %q, got %s", want, result.Log)
		}
	}
	for _, notWant := range []string{"$ git pull", "$ go test", "$ go build"} {
		if strings.Contains(result.Log, notWant) {
			t.Fatalf("expected already-latest upgrade to skip %q, got %s", notWant, result.Log)
		}
	}
	if !containsString(progress, "已经是最新版本了") {
		t.Fatalf("expected progress callback to publish latest-version message, got %+v", progress)
	}
}

func TestPortalUpgradeVersionHelpers(t *testing.T) {
	if got := portalUpgradeRemoteRef(""); got != "refs/remotes/origin/golang" {
		t.Fatalf("unexpected default remote ref: %s", got)
	}
	if got := portalUpgradeRemoteRef("feature/x"); got != "refs/remotes/origin/feature/x" {
		t.Fatalf("unexpected remote ref: %s", got)
	}
	if got := shortGitCommit("1234567890abcdef"); got != "12345678" {
		t.Fatalf("unexpected short commit: %s", got)
	}
	if got := shortGitCommit("abc"); got != "abc" {
		t.Fatalf("unexpected short commit: %s", got)
	}
}

func TestPortalUpgradeCommandEnvUsesIsolatedGoCache(t *testing.T) {
	dir := t.TempDir()
	env := portalUpgradeCommandEnv("go", dir)
	want := "GOCACHE=" + filepath.Join(dir, ".upgrade-cache", "go-build")

	if !containsString(env, want) {
		t.Fatalf("expected go command env to include %q, got %+v", want, env)
	}
	if !containsString(env, "GIT_TERMINAL_PROMPT=0") {
		t.Fatalf("expected git prompt to be disabled, got %+v", env)
	}
}

func TestPortalUpgradeCommandEnvLeavesGitCacheUntouched(t *testing.T) {
	env := portalUpgradeCommandEnv("git", t.TempDir())

	for _, value := range env {
		if strings.HasPrefix(value, "GOCACHE=") {
			t.Fatalf("expected git command env to avoid upgrade GOCACHE, got %+v", env)
		}
	}
}

func TestPortalUpgradeStatusPersistsAndLoads(t *testing.T) {
	statusPath := filepath.Join(t.TempDir(), "portal-upgrade-status.json")
	t.Setenv(portalUpgradeStatusFileEnv, statusPath)
	startedAt := time.Date(2026, 7, 6, 17, 50, 0, 0, time.UTC)
	finishedAt := startedAt.Add(time.Minute)
	result := portalUpgradeResult{
		OK:         true,
		Status:     "success",
		Message:    "升级已生效版本: abc12345",
		Log:        "restart ok",
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
	}

	persistPortalUpgradeStatus(result)

	loaded, ok := loadPortalUpgradeStatus()
	if !ok {
		t.Fatal("expected persisted upgrade status to load")
	}
	if loaded.Status != "success" || loaded.Message != result.Message || loaded.Log != "restart ok" || !loaded.FinishedAt.Equal(finishedAt) {
		t.Fatalf("unexpected loaded upgrade status: %+v", loaded)
	}
}

func TestSchedulePortalUpgradeServiceRestartCanBeDisabledForTests(t *testing.T) {
	t.Setenv(portalUpgradeRestartDisabledEnv, "true")
	var log bytes.Buffer

	if err := schedulePortalUpgradeServiceRestart(t.TempDir(), t.TempDir(), time.Now(), &log); err != nil {
		t.Fatalf("expected disabled restart scheduling to succeed, got %v", err)
	}
	if !strings.Contains(log.String(), "已跳过服务重启调度") {
		t.Fatalf("expected disabled restart to be logged, got %s", log.String())
	}
}

func TestPortalUpgradeRestartScriptRunsRunBatAndWritesEffectiveVersion(t *testing.T) {
	script := portalUpgradeRestartScript(`C:\yuqing\go`, `C:\yuqing`, `C:\yuqing\go\run.bat`, `C:\yuqing\go\runtime-logs\portal-upgrade-status.json`, `C:\yuqing\go\runtime-logs\portal-upgrade-restart.log`, time.Date(2026, 7, 6, 17, 50, 0, 0, time.UTC))

	for _, want := range []string{
		`& $RunBat --skip-pull`,
		`Write-UpgradeStatus 'restarting' '正在重启服务，页面会自动重新连接...' $true $true`,
		`Write-UpgradeStatus 'success' ('升级已生效版本: ' + $commit) $true $false`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("expected restart script to include %q, got %s", want, script)
		}
	}
}

func TestRunPortalUpgradeCommandWithProgressPublishesHeartbeat(t *testing.T) {
	t.Setenv("PORTAL_UPGRADE_SLEEP_HELPER", "1")
	var log bytes.Buffer
	var snapshotsMu sync.Mutex
	var snapshots []string

	err := runPortalUpgradeCommandWithProgress(
		context.Background(),
		&log,
		t.TempDir(),
		false,
		func(_ string, logText string) {
			snapshotsMu.Lock()
			defer snapshotsMu.Unlock()
			snapshots = append(snapshots, logText)
		},
		"阶段: 调试",
		10*time.Millisecond,
		os.Args[0],
		"-test.run=TestPortalUpgradeSleepHelper",
	)
	if err != nil {
		t.Fatalf("runPortalUpgradeCommandWithProgress error: %v\n%s", err, log.String())
	}
	if !strings.Contains(log.String(), "debug: 工作目录:") || !strings.Contains(log.String(), "debug: 命令结束") {
		t.Fatalf("expected debug command metadata, got %s", log.String())
	}
	for _, line := range strings.Split(log.String(), "\n") {
		if strings.Contains(line, "debug: 命令仍在运行") && strings.Contains(line, "-test.run=TestPortalUpgradeSleepHelper") {
			t.Fatalf("expected heartbeat to avoid repeating full command, got %q", line)
		}
	}
	foundHeartbeat := false
	snapshotsMu.Lock()
	for _, snapshot := range snapshots {
		if strings.Contains(snapshot, "debug: 命令仍在运行") {
			foundHeartbeat = true
			break
		}
	}
	snapshotsMu.Unlock()
	if !foundHeartbeat {
		t.Fatalf("expected heartbeat progress snapshot, got %+v", snapshots)
	}
}

func TestPortalUpgradeSleepHelper(t *testing.T) {
	if os.Getenv("PORTAL_UPGRADE_SLEEP_HELPER") != "1" {
		return
	}
	time.Sleep(60 * time.Millisecond)
	fmt.Println("sleep helper done")
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
