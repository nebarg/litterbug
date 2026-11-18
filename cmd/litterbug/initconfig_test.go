package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nebarg/litterbug/internal/config"
	"github.com/nebarg/litterbug/internal/repotest"
)

func TestInitConfig(t *testing.T) {
	dir := setup(t, map[string]string{"web/index.php": "<?php\nprint_r($x);\n"})
	repotest.Init(t, dir)
	path := filepath.Join(dir, config.RepoFile)

	// From a subdirectory, the config goes at the repository's root, and
	// applies: print-r is in the defaults, not in the global config.
	t.Chdir(filepath.Join(dir, "web"))
	if stdout, stderr, status := runCLI(t, "init-config"); status != exitClean || stderr != "" || !strings.HasPrefix(stdout, "Wrote the default config to ") {
		t.Fatalf("init-config: status %d, stdout %q, stderr %q", status, stdout, stderr)
	}
	written := readFile(t, path)
	if stdout, _, _ := runCLI(t); !strings.Contains(stdout, "index.php:2: print_r($x);  [php/print-r]") {
		t.Errorf("scanning with the written config: %q", stdout)
	}

	t.Chdir(dir)
	repotest.WriteFiles(t, dir, map[string]string{config.RepoFile: phpConfig})
	stdout, stderr, status := runCLI(t, "init-config")
	if status != exitError || stdout != "" || !strings.Contains(stderr, "there is already a config, at .litterbug.yml.") || !strings.Contains(stderr, "litterbug init-config --force") {
		t.Errorf("over an existing config: status %d, stdout %q, stderr %q", status, stdout, stderr)
	}
	if got := readFile(t, path); got != phpConfig {
		t.Errorf("the existing config was changed to %q", got)
	}

	stdout, stderr, status = runCLI(t, "init-config", "--force")
	if want := "Wrote the default config to .litterbug.yml, and kept the old one as .litterbug.yml.bak.\n"; stdout != want || stderr != "" || status != exitClean {
		t.Errorf("--force: status %d, stdout %q, stderr %q", status, stdout, stderr)
	}
	if got := readFile(t, path); got != written {
		t.Errorf("--force wrote %q, want the defaults", got)
	}
	if got := readFile(t, path+".bak"); got != phpConfig {
		t.Errorf("the backup is %q, want the old config", got)
	}
}

func TestInitConfigGlobal(t *testing.T) {
	setup(t, map[string]string{"main.go": "println(1)\n"})
	global := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "litterbug", "config.yml")

	stdout, stderr, status := runCLI(t, "init-config", "--global")
	if status != exitError || stdout != "" || !strings.Contains(stderr, "there is already a config") || !strings.Contains(stderr, "litterbug init-config --global --force") {
		t.Errorf("over the existing global config: status %d, stdout %q, stderr %q", status, stdout, stderr)
	}

	stdout, stderr, status = runCLI(t, "init-config", "--global", "--force")
	if want := "Wrote the default config to " + global + ", and kept the old one as " + global + ".bak.\n"; stdout != want || stderr != "" || status != exitClean {
		t.Fatalf("--global --force: status %d, stdout %q, stderr %q", status, stdout, stderr)
	}
	if got := readFile(t, global+".bak"); got != testConfig {
		t.Errorf("the backup is %q, want the old global config", got)
	}
	// builtin-print is in the defaults, not in the old global config.
	if stdout, _, _ := runCLI(t); !strings.Contains(stdout, "main.go:1: println(1)  [go/builtin-print]") {
		t.Errorf("scanning with the written global config: %q", stdout)
	}
}

func TestInitConfigLang(t *testing.T) {
	dir := setup(t, map[string]string{"app.py": "breakpoint()\n", "main.go": "println(1)\n"})
	repotest.Init(t, dir)
	path := filepath.Join(dir, config.RepoFile)

	stdout, stderr, status := runCLI(t, "init-config", "--lang", "python, ruby", "-l", "python")
	if status != exitClean || stderr != "" || stdout != "Wrote a config for python and ruby to .litterbug.yml.\n" {
		t.Fatalf("init-config --lang: status %d, stdout %q, stderr %q", status, stdout, stderr)
	}
	want, err := config.Default([]string{"python", "ruby"})
	if err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != string(want) {
		t.Errorf("wrote %q, want python and ruby alone", got)
	}
	if stdout, _, _ := runCLI(t); findingLines(stdout) == nil || strings.Contains(stdout, "main.go") {
		t.Errorf("scanning with python and ruby alone: %q", stdout)
	}

	stdout, stderr, status = runCLI(t, "init-config", "--lang", "go")
	if status != exitError || !strings.Contains(stderr, "Run litterbug add-language go to add to it, or litterbug init-config --force to replace it") {
		t.Errorf("over an existing config: status %d, stdout %q, stderr %q", status, stdout, stderr)
	}
	for _, c := range []struct {
		args []string
		want string // in stderr
	}{
		{[]string{"--lang", "elixir", "--force"}, `there is no built-in language "elixir"`},
		{[]string{"--lang", " "}, "--lang needs a language name"},
	} {
		stdout, stderr, status := runCLI(t, append([]string{"init-config"}, c.args...)...)
		if status != exitError || stdout != "" || !strings.Contains(stderr, c.want) {
			t.Errorf("init-config %v: status %d, stdout %q, stderr %q", c.args, status, stdout, stderr)
		}
	}
	if got := readFile(t, path); got != string(want) {
		t.Errorf("a failed init-config changed the config to %q", got)
	}
	if _, err := os.Stat(path + ".bak"); err == nil {
		t.Error("a failed init-config --force made a backup")
	}
}

func TestInitConfigErrors(t *testing.T) {
	setup(t, nil)
	for _, c := range []struct {
		args []string
		want []string // in stderr
	}{
		{nil, []string{"init-config: not in a repository", "litterbug init-config --global"}},
		{[]string{"extra"}, []string{"init-config takes no arguments"}},
		{[]string{"--wat"}, []string{"unknown flag: --wat", "litterbug init-config --help"}},
	} {
		args := append([]string{"init-config"}, c.args...)
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			stdout, stderr, status := runCLI(t, args...)
			if status != exitError || stdout != "" {
				t.Errorf("status %d, stdout %q", status, stdout)
			}
			for _, want := range c.want {
				if !strings.Contains(stderr, want) {
					t.Errorf("stderr %q doesn't mention %q", stderr, want)
				}
			}
		})
	}
	if _, err := os.Stat(config.RepoFile); err == nil {
		t.Error("a config was written outside a repository")
	}

	stdout, stderr, status := runCLI(t, "init-config", "--help")
	if status != exitClean || stderr != "" || !strings.HasPrefix(stdout, "Usage:\n  litterbug init-config") || !strings.Contains(stdout, "--global") || !strings.Contains(stdout, "--force") {
		t.Errorf("--help: status %d, stdout %q, stderr %q", status, stdout, stderr)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
