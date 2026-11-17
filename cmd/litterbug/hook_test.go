package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// runAsLitterbug makes the test binary run as litterbug, so a hook a test
// installs, which runs the binary that installed it, works.
const runAsLitterbug = "LITTERBUG_TEST_RUN_AS_LITTERBUG"

func TestMain(m *testing.M) {
	if os.Getenv(runAsLitterbug) == "1" {
		main()
	}
	os.Exit(m.Run())
}

func TestTheHookBlocksCommitsAddingDebugStatements(t *testing.T) {
	dir := setup(t, map[string]string{"main.go": "package main\n"})
	gitInit(t, dir)
	commitAll(t, dir)
	t.Setenv(runAsLitterbug, "1")

	stdout, stderr, status := runCLI(t, "install-hook")
	if status != exitClean || !strings.HasPrefix(stdout, "Installed the pre-commit hook at "+filepath.Join(".git", "hooks", "pre-commit")+", running ") {
		t.Fatalf("install-hook: status %d, stdout %q, stderr %q", status, stdout, stderr)
	}
	if !strings.Contains(stderr, "is a temporary build") {
		t.Errorf("no warning that the test binary is a temporary build: %q", stderr)
	}

	writeFiles(t, dir, map[string]string{"main.go": "package main\n\nfmt.Println(1)\n"})
	out, err := gitCommit(t, dir, "-am", "debug")
	if err == nil || !strings.Contains(out, "main.go:3: fmt.Println(1)  [go/fmt-print]") || !strings.Contains(out, stagedHint) {
		t.Fatalf("a commit adding a debug statement: %v\n%s", err, out)
	}
	if n := commitCount(t, dir); n != 1 {
		t.Fatalf("%d commits after a blocked one, want 1", n)
	}

	// A commit of only clean files goes through, though main.go still
	// holds its debug statement, unstaged.
	writeFiles(t, dir, map[string]string{"clean.go": "package main\n"})
	gitIn(t, dir, "add", "clean.go")
	if out, err := gitCommit(t, dir, "-m", "clean"); err != nil || !strings.Contains(out, "No debug statements found.") {
		t.Fatalf("a clean commit: %v\n%s", err, out)
	}

	if out, err := gitCommit(t, dir, "-am", "debug", "--no-verify"); err != nil {
		t.Fatalf("a commit with --no-verify: %v\n%s", err, out)
	}
	if n := commitCount(t, dir); n != 3 {
		t.Fatalf("%d commits, want 3", n)
	}

	stdout, _, status = runCLI(t, "uninstall-hook")
	if status != exitClean || !strings.HasPrefix(stdout, "Removed the pre-commit hook at ") {
		t.Fatalf("uninstall-hook: status %d, stdout %q", status, stdout)
	}
	writeFiles(t, dir, map[string]string{"main.go": "package main\n\nfmt.Println(2)\n"})
	if out, err := gitCommit(t, dir, "-am", "debug without the hook"); err != nil {
		t.Fatalf("a commit after uninstalling: %v\n%s", err, out)
	}
}

func TestHookCommands(t *testing.T) {
	dir := setup(t, testTree)
	gitInit(t, dir)
	hook := filepath.Join(dir, ".git", "hooks", "pre-commit")
	foreign := "#!/bin/sh\necho another tool\n"

	stdout, _, status := runCLI(t, "uninstall-hook")
	if status != exitClean || stdout != "No pre-commit hook to remove at "+filepath.Join(".git", "hooks", "pre-commit")+".\n" {
		t.Errorf("uninstall-hook without one: status %d, stdout %q", status, stdout)
	}

	writeFiles(t, dir, map[string]string{filepath.Join(".git", "hooks", "pre-commit"): foreign})
	stdout, stderr, status := runCLI(t, "install-hook")
	if status != exitError || stdout != "" || !strings.Contains(stderr, "didn't install") || !strings.Contains(stderr, "--force") {
		t.Errorf("install-hook over another hook: status %d, stdout %q, stderr %q", status, stdout, stderr)
	}
	stdout, stderr, status = runCLI(t, "uninstall-hook")
	if status != exitError || !strings.Contains(stderr, "left alone") {
		t.Errorf("uninstall-hook of another hook: status %d, stdout %q, stderr %q", status, stdout, stderr)
	}
	if content, err := os.ReadFile(hook); err != nil || string(content) != foreign {
		t.Fatalf("the other hook changed: %q, %v", content, err)
	}

	// From a subdirectory, with --force.
	t.Chdir(filepath.Join(dir, "web"))
	stdout, _, status = runCLI(t, "install-hook", "--force")
	want := "Installed the pre-commit hook at " + hook
	if status != exitClean || !strings.HasPrefix(stdout, want) {
		t.Errorf("install-hook --force: status %d, stdout %q, want it to start %q", status, stdout, want)
	}
	if content, err := os.ReadFile(hook); err != nil || !strings.Contains(string(content), "--staged") {
		t.Errorf("hook after --force: %q, %v", content, err)
	}

	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"install-hook", "extra"}, "install-hook takes no arguments"},
		{[]string{"uninstall-hook", "--force"}, "unknown flag: --force"},
		{[]string{"install-hook", "--wat"}, "Run litterbug install-hook --help"},
	} {
		if _, stderr, status := runCLI(t, c.args...); status != exitError || !strings.Contains(stderr, c.want) {
			t.Errorf("%v: status %d, stderr %q", c.args, status, stderr)
		}
	}
	for _, command := range []string{"install-hook", "uninstall-hook"} {
		if stdout, _, status := runCLI(t, command, "--help"); status != exitClean || !strings.HasPrefix(stdout, "Usage:\n  litterbug "+command) {
			t.Errorf("%s --help: status %d, stdout %q", command, status, stdout)
		}
	}

	t.Chdir(t.TempDir())
	for _, command := range []string{"install-hook", "uninstall-hook"} {
		if _, stderr, status := runCLI(t, command); status != exitError || !strings.Contains(stderr, "litterbug: "+command+": ") || !strings.Contains(stderr, "not a git repository") {
			t.Errorf("%s outside a repository: status %d, stderr %q", command, status, stderr)
		}
	}
}

// gitCommit runs git commit in dir, returning what it and the hook print.
func gitCommit(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir, "commit"}, args...)...).CombinedOutput()
	return string(out), err
}

func commitCount(t *testing.T, dir string) int {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "rev-list", "--count", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatal(err)
	}
	return n
}
