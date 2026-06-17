package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const (
	defaultHost = "127.0.0.1"
	defaultPort = 8087
)

type repeatFlag []string

func (f *repeatFlag) String() string {
	return strings.Join(*f, " ")
}

func (f *repeatFlag) Set(value string) error {
	value = strings.TrimSpace(value)
	if value != "" {
		*f = append(*f, value)
	}
	return nil
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	var pythonArgs repeatFlag
	pythonArgs = append(pythonArgs, splitArgs(os.Getenv("YUQING_AKSHARE_PYTHON_ARGS"))...)

	flags := flag.NewFlagSet("akshare-service", flag.ContinueOnError)
	flags.SetOutput(os.Stdout)
	python := flags.String("python", strings.TrimSpace(os.Getenv("YUQING_AKSHARE_PYTHON")), "python executable path")
	script := flags.String("script", defaultScriptPath(), "akshare python adapter script path")
	host := flags.String("host", envOrDefault("AKSHARE_AUCTION_HOST", defaultHost), "listen host")
	port := flags.Int("port", envInt("AKSHARE_AUCTION_PORT", defaultPort), "listen port")
	workers := flags.Int("workers", envInt("AKSHARE_AUCTION_WORKERS", 0), "worker count; 0 keeps python default")
	limit := flags.Int("limit", envInt("AKSHARE_AUCTION_LIMIT", 0), "stock count limit; 0 keeps python default")
	cacheDir := flags.String("cache-dir", strings.TrimSpace(os.Getenv("AKSHARE_AUCTION_CACHE_DIR")), "cache directory; empty keeps python default")
	selfTest := flags.Bool("self-test", false, "run python adapter self-test")
	flags.Var(&pythonArgs, "python-arg", "extra argument passed before the python script, repeatable")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	pythonExe, launchArgs, err := resolvePython(*python, pythonArgs)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	scriptPath, err := resolveScript(*script)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cmdArgs := append([]string{}, launchArgs...)
	cmdArgs = append(cmdArgs, scriptPath, "--host", *host, "--port", strconv.Itoa(*port))
	if *workers > 0 {
		cmdArgs = append(cmdArgs, "--workers", strconv.Itoa(*workers))
	}
	if *limit > 0 {
		cmdArgs = append(cmdArgs, "--limit", strconv.Itoa(*limit))
	}
	if strings.TrimSpace(*cacheDir) != "" {
		cmdArgs = append(cmdArgs, "--cache-dir", strings.TrimSpace(*cacheDir))
	}
	if *selfTest {
		cmdArgs = append(cmdArgs, "--self-test")
	}
	cmdArgs = append(cmdArgs, flags.Args()...)

	cmd := exec.CommandContext(ctx, pythonExe, cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	fmt.Printf("akshare-service starting on http://%s:%d via %s\n", *host, *port, pythonExe)
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "start akshare python adapter: %v\n", err)
		return 1
	}
	err = cmd.Wait()
	if ctx.Err() != nil {
		_ = cmd.Process.Kill()
		return 0
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "akshare python adapter stopped: %v\n", err)
		return 1
	}
	return 0
}

func resolvePython(value string, extraArgs []string) (string, []string, error) {
	value = strings.TrimSpace(value)
	if value != "" {
		if filepath.IsAbs(value) {
			if _, err := os.Stat(value); err != nil {
				return "", nil, fmt.Errorf("python executable not found: %s", value)
			}
			return value, extraArgs, nil
		}
		found, err := exec.LookPath(value)
		if err != nil {
			return "", nil, fmt.Errorf("python executable not found in PATH: %s", value)
		}
		return found, extraArgs, nil
	}
	if found, err := exec.LookPath("python"); err == nil {
		return found, extraArgs, nil
	}
	if found, err := exec.LookPath("py"); err == nil {
		args := append([]string{"-3"}, extraArgs...)
		return found, args, nil
	}
	return "", nil, fmt.Errorf("python was not found; install Python 3 or set YUQING_AKSHARE_PYTHON")
}

func resolveScript(value string) (string, error) {
	value = strings.TrimSpace(value)
	candidates := []string{}
	if value != "" {
		candidates = append(candidates, value)
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, "services", "akshare_auction_service.py"))
	}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(exeDir, "..", "services", "akshare_auction_service.py"),
			filepath.Join(exeDir, "services", "akshare_auction_service.py"),
		)
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		abs, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if _, err := os.Stat(abs); err == nil {
			return abs, nil
		}
	}
	return "", fmt.Errorf("akshare adapter script not found; set --script or YUQING_AKSHARE_SCRIPT")
}

func defaultScriptPath() string {
	return strings.TrimSpace(os.Getenv("YUQING_AKSHARE_SCRIPT"))
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}

func splitArgs(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return strings.Fields(value)
}
