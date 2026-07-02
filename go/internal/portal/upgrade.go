package portal

import (
	"bytes"
	"context"
	"fmt"
	"io"
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

type portalUpgradeProgress func(message string, logText string)

type portalUpgradeRunner interface {
	Run(context.Context, config.Config, portalUpgradeProgress) portalUpgradeResult
}

type portalUpgradeResult struct {
	OK         bool      `json:"ok"`
	Status     string    `json:"status"`
	Message    string    `json:"message"`
	Log        string    `json:"log"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	Running    bool      `json:"running"`
}

type defaultPortalUpgradeRunner struct{}

func (defaultPortalUpgradeRunner) Run(ctx context.Context, cfg config.Config, progress portalUpgradeProgress) portalUpgradeResult {
	return runPortalUpgrade(ctx, cfg, progress)
}

func (s *Server) requirePortalUpgradeSession(next func(http.ResponseWriter, *http.Request, any)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil || strings.TrimSpace(cookie.Value) == "" {
			writePortalUpgradeFailureJSON(w, http.StatusForbidden, "未登录")
			return
		}
		user, err := s.getSessionUser(cookie.Value)
		if err != nil {
			writePortalUpgradeFailureJSON(w, http.StatusForbidden, "会话无效")
			return
		}
		next(w, r, user)
	}
}

func writePortalUpgradeFailureJSON(w http.ResponseWriter, status int, message string) {
	writeRawJSON(w, status, portalUpgradeResult{
		OK:      false,
		Status:  "failed",
		Message: message,
		Log:     timestampedUpgradeLine("升级请求失败: " + message),
	})
}

func (s *Server) handleSystemUpgrade(w http.ResponseWriter, r *http.Request, _ any) {
	if r.Method != http.MethodPost {
		writeRawJSON(w, http.StatusMethodNotAllowed, map[string]any{"message": "method not allowed"})
		return
	}
	result := s.startPortalUpgrade()
	writeRawJSON(w, http.StatusAccepted, result)
}

func (s *Server) handleSystemUpgradeStatus(w http.ResponseWriter, r *http.Request, _ any) {
	if r.Method != http.MethodGet {
		writeRawJSON(w, http.StatusMethodNotAllowed, map[string]any{"message": "method not allowed"})
		return
	}
	writeRawJSON(w, http.StatusOK, s.portalUpgradeSnapshot())
}

func (s *Server) startPortalUpgrade() portalUpgradeResult {
	s.upgradeMu.Lock()
	if s.upgradeState.Running {
		result := s.upgradeState
		s.upgradeMu.Unlock()
		return result
	}
	runner := s.upgradeRunner
	if runner == nil {
		runner = defaultPortalUpgradeRunner{}
	}
	cfg := s.cfg
	startedAt := time.Now()
	s.upgradeState = portalUpgradeResult{
		OK:        true,
		Status:    "running",
		Message:   "升级执行中",
		Log:       timestampedUpgradeLine("升级已在后台执行，页面会自动刷新状态。"),
		StartedAt: startedAt,
		Running:   true,
	}
	result := s.upgradeState
	s.upgradeMu.Unlock()

	progress := func(message string, logText string) {
		s.upgradeMu.Lock()
		defer s.upgradeMu.Unlock()
		if !s.upgradeState.Running || !s.upgradeState.StartedAt.Equal(startedAt) {
			return
		}
		if strings.TrimSpace(message) != "" {
			s.upgradeState.Message = strings.TrimSpace(message)
		}
		if strings.TrimSpace(logText) != "" {
			s.upgradeState.Log = strings.TrimRight(logText, "\r\n")
		}
	}
	go func() {
		completed := runner.Run(context.Background(), cfg, progress)
		completed.Running = false
		s.upgradeMu.Lock()
		s.upgradeState = completed
		s.upgradeMu.Unlock()
	}()

	return result
}

func (s *Server) portalUpgradeSnapshot() portalUpgradeResult {
	s.upgradeMu.Lock()
	defer s.upgradeMu.Unlock()
	if s.upgradeState.StartedAt.IsZero() {
		return portalUpgradeResult{
			Status:  "idle",
			Message: "等待执行",
			Log:     "等待升级日志",
		}
	}
	result := s.upgradeState
	if result.Running && !result.StartedAt.IsZero() {
		elapsed := time.Since(result.StartedAt).Round(time.Second)
		if elapsed < 0 {
			elapsed = 0
		}
		if result.Message == "" || result.Message == "升级执行中" {
			result.Message = fmt.Sprintf("升级执行中，已运行 %s", elapsed)
		} else {
			result.Message = fmt.Sprintf("%s，已运行 %s", result.Message, elapsed)
		}
	}
	return result
}

func runPortalUpgrade(parent context.Context, _ config.Config, progress portalUpgradeProgress) portalUpgradeResult {
	startedAt := time.Now()
	ctx, cancel := context.WithTimeout(parent, portalUpgradeTimeout)
	defer cancel()

	var log bytes.Buffer
	publish := func(message string) {
		if progress != nil {
			progress(message, strings.TrimRight(log.String(), "\r\n"))
		}
	}
	stage := func(name string) {
		message := "阶段: " + name
		appendUpgradeLog(&log, message)
		publish(message)
	}
	runStage := func(name string, dir string, allowFailure bool, command string, args ...string) error {
		stage(name)
		err := runPortalUpgradeCommand(ctx, &log, dir, allowFailure, command, args...)
		publish("阶段: " + name)
		return err
	}

	stage("初始化")
	appendUpgradeLog(&log, "开始系统升级")
	goDir, err := resolvePortalGoDir()
	if err != nil {
		return finishPortalUpgrade(false, startedAt, log.String(), err)
	}
	repoRoot := filepath.Dir(goDir)
	appendUpgradeLog(&log, "Go 目录: "+goDir)
	appendUpgradeLog(&log, "仓库目录: "+repoRoot)
	publish("阶段: 初始化")

	_ = runStage("准备仓库", repoRoot, true, "git", "fsmonitor--daemon", "stop")
	stage("读取当前分支")
	branch := strings.TrimSpace(portalUpgradeCommandOutput(ctx, repoRoot, "git", "rev-parse", "--abbrev-ref", "HEAD"))
	if branch == "" || strings.EqualFold(branch, "HEAD") {
		branch = "golang"
	}
	appendUpgradeLog(&log, "当前分支: "+branch)
	publish("阶段: 读取当前分支")
	if err := runStage("拉取代码信息", repoRoot, false, "git", "fetch", "--prune", "origin"); err != nil {
		return finishPortalUpgrade(false, startedAt, log.String(), err)
	}
	stage("版本检测")
	currentCommit := strings.TrimSpace(portalUpgradeCommandOutput(ctx, repoRoot, "git", "rev-parse", "HEAD"))
	latestCommit := strings.TrimSpace(portalUpgradeCommandOutput(ctx, repoRoot, "git", "rev-parse", portalUpgradeRemoteRef(branch)))
	if currentCommit == "" || latestCommit == "" {
		return finishPortalUpgrade(false, startedAt, log.String(), fmt.Errorf("无法读取当前版本或最新版本"))
	}
	appendUpgradeLog(&log, "当前版本: "+shortGitCommit(currentCommit))
	appendUpgradeLog(&log, "最新版本: "+shortGitCommit(latestCommit))
	if currentCommit == latestCommit {
		appendUpgradeLog(&log, "已经是最新版本了")
		publish("已经是最新版本了")
		return finishPortalUpgradeWithMessage(true, startedAt, log.String(), nil, "已经是最新版本了")
	}
	publish("阶段: 版本检测")
	if err := runStage("拉取代码", repoRoot, false, "git", "pull", "--ff-only", "origin", branch); err != nil {
		return finishPortalUpgrade(false, startedAt, log.String(), err)
	}
	_ = runStage("清理构建缓存", goDir, true, "go", "clean", "-cache", "-testcache")
	stage("测试")
	logPortalUpgradeTestDebug(ctx, &log, goDir, progress)
	if err := runPortalUpgradeCommandStreaming(ctx, &log, goDir, false, progress, "测试", "go", "test", "-v", "./..."); err != nil {
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
	if err := runStage("打包构建", goDir, false, "go", args...); err != nil {
		return finishPortalUpgrade(false, startedAt, log.String(), err)
	}
	stage("完成")
	appendUpgradeLog(&log, "升级构建完成。新二进制已写入 "+binDir+"；如需让当前服务加载新代码，请重启对应服务。")
	publish("阶段: 完成")
	return finishPortalUpgrade(true, startedAt, log.String(), nil)
}

func finishPortalUpgrade(ok bool, startedAt time.Time, logText string, err error) portalUpgradeResult {
	return finishPortalUpgradeWithMessage(ok, startedAt, logText, err, "升级完成")
}

func finishPortalUpgradeWithMessage(ok bool, startedAt time.Time, logText string, err error, successMessage string) portalUpgradeResult {
	finishedAt := time.Now()
	status := "success"
	message := successMessage
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

func portalUpgradeRemoteRef(branch string) string {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		branch = "golang"
	}
	return "refs/remotes/origin/" + branch
}

func shortGitCommit(commit string) string {
	commit = strings.TrimSpace(commit)
	if len(commit) <= 8 {
		return commit
	}
	return commit[:8]
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
	cmd.Env = portalUpgradeCommandEnv(name, dir)
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

func runPortalUpgradeCommandStreaming(ctx context.Context, log *bytes.Buffer, dir string, allowFailure bool, progress portalUpgradeProgress, stageName string, name string, args ...string) error {
	appendUpgradeLog(log, "$ "+shellQuoteCommand(name, args))
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = portalUpgradeCommandEnv(name, dir)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	startedAt := time.Now()
	if err := cmd.Start(); err != nil {
		return err
	}
	lines := make(chan string, 64)
	done := make(chan struct{}, 2)
	readOutput := func(reader io.Reader) {
		defer func() { done <- struct{}{} }()
		buf := make([]byte, 4096)
		var pending strings.Builder
		for {
			n, readErr := reader.Read(buf)
			if n > 0 {
				chunk := string(buf[:n])
				for {
					index := strings.IndexByte(chunk, '\n')
					if index < 0 {
						pending.WriteString(chunk)
						break
					}
					pending.WriteString(chunk[:index])
					lines <- strings.TrimRight(pending.String(), "\r")
					pending.Reset()
					chunk = chunk[index+1:]
				}
			}
			if readErr != nil {
				if pending.Len() > 0 {
					lines <- strings.TrimRight(pending.String(), "\r")
				}
				return
			}
		}
	}
	go readOutput(stdout)
	go readOutput(stderr)

	waitDone := make(chan error, 1)
	go func() {
		<-done
		<-done
		waitDone <- cmd.Wait()
	}()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case line := <-lines:
			if strings.TrimSpace(line) != "" {
				log.WriteString(line)
				log.WriteByte('\n')
				publishUpgradeProgress(progress, stageName, log)
			}
		case <-ticker.C:
			appendUpgradeLog(log, fmt.Sprintf("%s仍在运行，已耗时 %s", stageName, time.Since(startedAt).Round(time.Second)))
			publishUpgradeProgress(progress, stageName, log)
		case err := <-waitDone:
			for {
				select {
				case line := <-lines:
					if strings.TrimSpace(line) != "" {
						log.WriteString(line)
						log.WriteByte('\n')
					}
				default:
					appendUpgradeLog(log, fmt.Sprintf("%s完成，耗时 %s", stageName, time.Since(startedAt).Round(time.Second)))
					publishUpgradeProgress(progress, stageName, log)
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
			}
		}
	}
}

func publishUpgradeProgress(progress portalUpgradeProgress, stageName string, log *bytes.Buffer) {
	if progress == nil {
		return
	}
	progress("阶段: "+stageName, strings.TrimRight(log.String(), "\r\n"))
}

func logPortalUpgradeTestDebug(ctx context.Context, log *bytes.Buffer, goDir string, progress portalUpgradeProgress) {
	appendUpgradeLog(log, "测试目录: "+goDir)
	appendUpgradeLog(log, "测试命令: go test -v ./...")
	if version := strings.TrimSpace(portalUpgradeCommandOutput(ctx, goDir, "go", "version")); version != "" {
		appendUpgradeLog(log, "Go 版本: "+version)
	}
	if cacheDir := portalUpgradeGoCacheDir(goDir); cacheDir != "" {
		appendUpgradeLog(log, "GOCACHE: "+cacheDir)
	}
	packages := strings.Fields(portalUpgradeCommandOutput(ctx, goDir, "go", "list", "./..."))
	if len(packages) > 0 {
		appendUpgradeLog(log, fmt.Sprintf("待测试包数量: %d", len(packages)))
		for i, pkg := range packages {
			if i >= 12 {
				appendUpgradeLog(log, fmt.Sprintf("待测试包列表: 仅显示前 12 个，其余 %d 个略过", len(packages)-i))
				break
			}
			appendUpgradeLog(log, "待测试包: "+pkg)
		}
	}
	publishUpgradeProgress(progress, "测试", log)
}

func portalUpgradeCommandOutput(ctx context.Context, dir string, name string, args ...string) string {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = portalUpgradeCommandEnv(name, dir)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(output)
}

func portalUpgradeCommandEnv(name string, dir string) []string {
	env := append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if isPortalUpgradeGoCommand(name) {
		if cacheDir := portalUpgradeGoCacheDir(dir); cacheDir != "" {
			env = append(env, "GOCACHE="+cacheDir)
		}
	}
	return env
}

func isPortalUpgradeGoCommand(name string) bool {
	base := strings.TrimSuffix(strings.ToLower(filepath.Base(strings.TrimSpace(name))), ".exe")
	return base == "go"
}

func portalUpgradeGoCacheDir(dir string) string {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return ""
	}
	cacheDir := filepath.Join(dir, ".upgrade-cache", "go-build")
	if abs, err := filepath.Abs(cacheDir); err == nil {
		return abs
	}
	return cacheDir
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
