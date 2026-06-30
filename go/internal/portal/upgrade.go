package portal

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/config"
)

const portalUpgradeTimeout = 10 * time.Minute

var portalUpgradeBuildPackages = []string{
	"./cmd/auth-service",
	"./cmd/wechat-service",
	"./cmd/content-service",
	"./cmd/crawler-service",
	"./cmd/analysis-service",
	"./cmd/nlp-service",
	"./cmd/gateway-web",
	"./cmd/akshare-service",
	"./cmd/scheduler-service",
	"./cmd/release-service",
}

type portalUpgradeRunner interface {
	Run(context.Context, config.Config) portalUpgradeResult
}

type portalUpgradeResult struct {
	OK         bool      `json:"ok"`
	Status     string    `json:"status"`
	Message    string    `json:"message"`
	Log        string    `json:"log"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
}

type defaultPortalUpgradeRunner struct{}

func (defaultPortalUpgradeRunner) Run(ctx context.Context, cfg config.Config) portalUpgradeResult {
	return runPortalUpgrade(ctx, cfg)
}

func (s *Server) handleSystemUpgrade(w http.ResponseWriter, r *http.Request, _ any) {
	if r.Method != http.MethodPost {
		writeRawJSON(w, http.StatusMethodNotAllowed, map[string]any{"message": "method not allowed"})
		return
	}
	runner := s.upgradeRunner
	if runner == nil {
		runner = defaultPortalUpgradeRunner{}
	}
	result := runner.Run(r.Context(), s.cfg)
	status := http.StatusOK
	if !result.OK {
		status = http.StatusInternalServerError
	}
	writeRawJSON(w, status, result)
}

func runPortalUpgrade(parent context.Context, _ config.Config) portalUpgradeResult {
	startedAt := time.Now()
	ctx, cancel := context.WithTimeout(parent, portalUpgradeTimeout)
	defer cancel()

	var log bytes.Buffer
	appendUpgradeLog(&log, "开始系统升级")
	goDir, err := resolvePortalGoDir()
	if err != nil {
		return finishPortalUpgrade(false, startedAt, log.String(), err)
	}
	repoRoot := filepath.Dir(goDir)
	appendUpgradeLog(&log, "Go 目录: "+goDir)
	appendUpgradeLog(&log, "仓库目录: "+repoRoot)

	_ = runPortalUpgradeCommand(ctx, &log, repoRoot, true, "git", "fsmonitor--daemon", "stop")
	if err := runPortalUpgradeCommand(ctx, &log, repoRoot, false, "git", "fetch", "--prune", "origin"); err != nil {
		return finishPortalUpgrade(false, startedAt, log.String(), err)
	}

	branch := strings.TrimSpace(portalUpgradeCommandOutput(ctx, repoRoot, "git", "rev-parse", "--abbrev-ref", "HEAD"))
	if branch == "" || strings.EqualFold(branch, "HEAD") {
		branch = "golang"
	}
	appendUpgradeLog(&log, "当前分支: "+branch)
	if err := runPortalUpgradeCommand(ctx, &log, repoRoot, false, "git", "pull", "--ff-only", "origin", branch); err != nil {
		return finishPortalUpgrade(false, startedAt, log.String(), err)
	}
	if err := runPortalUpgradeCommand(ctx, &log, goDir, false, "go", "clean", "-cache", "-testcache"); err != nil {
		return finishPortalUpgrade(false, startedAt, log.String(), err)
	}

	binDir := filepath.Join(goDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return finishPortalUpgrade(false, startedAt, log.String(), err)
	}
	commit := strings.TrimSpace(portalUpgradeCommandOutput(ctx, repoRoot, "git", "rev-parse", "--short", "HEAD"))
	if commit == "" {
		commit = "unknown"
	}
	buildTime := time.Now().Format(time.RFC3339)
	ldflags := strings.Join([]string{
		"-X github.com/pcdogyu/yuqing/go/internal/app.Version=local",
		"-X github.com/pcdogyu/yuqing/go/internal/app.GitCommit=" + commit,
		"-X github.com/pcdogyu/yuqing/go/internal/app.BuildTime=" + buildTime,
		"-X github.com/pcdogyu/yuqing/go/internal/app.BranchName=" + branch,
	}, " ")
	args := []string{"build", "-ldflags", ldflags, "-o", binDir + string(os.PathSeparator)}
	args = append(args, portalUpgradeBuildPackages...)
	if err := runPortalUpgradeCommand(ctx, &log, goDir, false, "go", args...); err != nil {
		return finishPortalUpgrade(false, startedAt, log.String(), err)
	}
	appendUpgradeLog(&log, "升级构建完成。新二进制已写入 "+binDir+"；如需让当前服务加载新代码，请重启对应服务。")
	return finishPortalUpgrade(true, startedAt, log.String(), nil)
}

func finishPortalUpgrade(ok bool, startedAt time.Time, logText string, err error) portalUpgradeResult {
	finishedAt := time.Now()
	status := "success"
	message := "升级完成"
	if err != nil {
		status = "failed"
		message = err.Error()
		logText = strings.TrimRight(logText, "\r\n") + "\n" + timestampedUpgradeLine("升级失败: "+err.Error())
	}
	return portalUpgradeResult{
		OK:         ok,
		Status:     status,
		Message:    message,
		Log:        strings.TrimRight(logText, "\r\n"),
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
	}
}

func resolvePortalGoDir() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := cwd
	for i := 0; i < 8; i++ {
		if fileExists(filepath.Join(dir, "go.mod")) {
			return dir, nil
		}
		nestedGoDir := filepath.Join(dir, "go")
		if fileExists(filepath.Join(nestedGoDir, "go.mod")) {
			return nestedGoDir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("go.mod not found from %s", cwd)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func runPortalUpgradeCommand(ctx context.Context, log *bytes.Buffer, dir string, allowFailure bool, name string, args ...string) error {
	appendUpgradeLog(log, "$ "+shellQuoteCommand(name, args))
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := cmd.CombinedOutput()
	if len(output) > 0 {
		log.Write(output)
		if output[len(output)-1] != '\n' {
			log.WriteByte('\n')
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		if allowFailure {
			appendUpgradeLog(log, "忽略非关键命令失败: "+err.Error())
			return nil
		}
		return err
	}
	return nil
}

func portalUpgradeCommandOutput(ctx context.Context, dir string, name string, args ...string) string {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(output)
}

func appendUpgradeLog(log *bytes.Buffer, message string) {
	if log == nil {
		return
	}
	log.WriteString(timestampedUpgradeLine(message))
	log.WriteByte('\n')
}

func timestampedUpgradeLine(message string) string {
	return time.Now().Format("15:04:05") + " " + strings.TrimSpace(message)
}

func shellQuoteCommand(name string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, shellQuotePart(name))
	for _, arg := range args {
		parts = append(parts, shellQuotePart(arg))
	}
	return strings.Join(parts, " ")
}

func shellQuotePart(value string) string {
	if value == "" {
		if runtime.GOOS == "windows" {
			return `""`
		}
		return "''"
	}
	if !strings.ContainsAny(value, " \t\r\n\"'") {
		return value
	}
	if runtime.GOOS == "windows" {
		return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
	}
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
