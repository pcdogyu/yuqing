package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRepeatFlagAndSplitArgs(t *testing.T) {
	var values repeatFlag
	if err := values.Set("  -X utf8  "); err != nil {
		t.Fatalf("set repeat flag: %v", err)
	}
	if err := values.Set("   "); err != nil {
		t.Fatalf("set blank repeat flag: %v", err)
	}
	if err := values.Set("--fast"); err != nil {
		t.Fatalf("set repeat flag: %v", err)
	}
	if got, want := []string(values), []string{"-X utf8", "--fast"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("repeat flag values = %#v, want %#v", got, want)
	}
	if got := values.String(); got != "-X utf8 --fast" {
		t.Fatalf("repeat flag string = %q", got)
	}

	if got, want := splitArgs("  -X utf8   --fast "), []string{"-X", "utf8", "--fast"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("split args = %#v, want %#v", got, want)
	}
	if got := splitArgs("   "); got != nil {
		t.Fatalf("blank split args = %#v, want nil", got)
	}
}

func TestEnvIntFallsBackForBlankInvalidAndNegativeValues(t *testing.T) {
	t.Setenv("YUQING_TEST_INT", "")
	if got := envInt("YUQING_TEST_INT", 7); got != 7 {
		t.Fatalf("blank env int = %d, want 7", got)
	}
	t.Setenv("YUQING_TEST_INT", "bad")
	if got := envInt("YUQING_TEST_INT", 7); got != 7 {
		t.Fatalf("invalid env int = %d, want 7", got)
	}
	t.Setenv("YUQING_TEST_INT", "-1")
	if got := envInt("YUQING_TEST_INT", 7); got != 7 {
		t.Fatalf("negative env int = %d, want 7", got)
	}
	t.Setenv("YUQING_TEST_INT", "12")
	if got := envInt("YUQING_TEST_INT", 7); got != 12 {
		t.Fatalf("valid env int = %d, want 12", got)
	}
}

func TestResolvePythonAcceptsAbsoluteExecutable(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	got, args, err := resolvePython(exe, []string{"-u"})
	if err != nil {
		t.Fatalf("resolve python: %v", err)
	}
	if got != exe {
		t.Fatalf("resolved executable = %q, want %q", got, exe)
	}
	if !reflect.DeepEqual(args, []string{"-u"}) {
		t.Fatalf("resolved args = %#v", args)
	}
}

func TestResolveScriptUsesExplicitExistingPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "akshare_auction_service.py")
	if err := os.WriteFile(path, []byte("print('ok')\n"), 0o644); err != nil {
		t.Fatalf("write script: %v", err)
	}
	got, err := resolveScript(path)
	if err != nil {
		t.Fatalf("resolve script: %v", err)
	}
	if got != path {
		t.Fatalf("resolved script = %q, want %q", got, path)
	}
}
