package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nebarg/litterbug/internal/config"
	"github.com/nebarg/litterbug/internal/repotest"
)

func TestAddLanguage(t *testing.T) {
	dir := setup(t, map[string]string{"app.py": "breakpoint()\n"})
	repotest.Init(t, dir)
	path := filepath.Join(dir, config.RepoFile)

	stdout, stderr, status := runCLI(t, "add-language", "python")
	if status != exitError || stdout != "" || !strings.Contains(stderr, "there is no config at .litterbug.yml to add to.") ||
		!strings.Contains(stderr, "litterbug init-config --lang python to write one with only python") {
		t.Errorf("without a config: status %d, stdout %q, stderr %q", status, stdout, stderr)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("a config was written")
	}

	if stdout, stderr, status := runCLI(t, "init-config", "--lang", "go"); status != exitClean || stderr != "" || stdout != "Wrote a config for go to .litterbug.yml.\n" {
		t.Fatalf("init-config --lang go: status %d, stdout %q, stderr %q", status, stdout, stderr)
	}
	if stdout, _, status := runCLI(t); status != exitClean {
		t.Errorf("without python in the config: status %d, stdout %q", status, stdout)
	}

	stdout, stderr, status = runCLI(t, "add-language", "python", "python, python")
	if status != exitClean || stderr != "" || stdout != "Added python to .litterbug.yml.\n" {
		t.Fatalf("add-language python: status %d, stdout %q, stderr %q", status, stdout, stderr)
	}
	if stdout, _, _ := runCLI(t); !strings.Contains(stdout, "app.py:1: breakpoint()  [python/breakpoint]") {
		t.Errorf("scanning with python added: %q", stdout)
	}
	added := readFile(t, path)

	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"python"}, ".litterbug.yml already has python"},
		{[]string{"rust", "elixir"}, `there is no built-in language "elixir"`},
		{[]string{"rust,"}, `"rust," has an empty language name`},
		{[]string{"rust,,java"}, `"rust,,java" has an empty language name`},
	} {
		stdout, stderr, status := runCLI(t, append([]string{"add-language"}, c.args...)...)
		if status != exitError || stdout != "" || !strings.Contains(stderr, c.want) {
			t.Errorf("add-language %v: status %d, stdout %q, stderr %q", c.args, status, stdout, stderr)
		}
	}
	if got := readFile(t, path); got != added {
		t.Errorf("a failed add-language changed the config to %q", got)
	}
}

func TestAddLanguageGlobal(t *testing.T) {
	setup(t, nil)
	global := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "litterbug", "config.yml")

	stdout, stderr, status := runCLI(t, "add-language", "--global", "ruby")
	if status != exitError || !strings.Contains(stderr, global+" already has ruby") {
		t.Errorf("a language the global config has: status %d, stdout %q, stderr %q", status, stdout, stderr)
	}
	stdout, stderr, status = runCLI(t, "add-language", "--global", "rust,java")
	if status != exitClean || stderr != "" || stdout != "Added rust and java to "+global+".\n" {
		t.Fatalf("add-language --global: status %d, stdout %q, stderr %q", status, stdout, stderr)
	}
	if got := readFile(t, global); !strings.HasPrefix(got, testConfig) || !strings.Contains(got, "\n  rust:\n") || !strings.Contains(got, "\n  java:\n") {
		t.Errorf("the global config is now %q", got)
	}

	if err := os.Remove(global); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, status = runCLI(t, "add-language", "--global", "rust")
	if status != exitError || !strings.Contains(stderr, "litterbug init-config --global --lang rust") {
		t.Errorf("without a global config: status %d, stdout %q, stderr %q", status, stdout, stderr)
	}
}

func TestAddLanguageListsTheBuiltInLanguages(t *testing.T) {
	setup(t, nil)
	stdout, stderr, status := runCLI(t, "add-language")
	if status != exitClean || stderr != "" {
		t.Fatalf("status %d, stderr %q", status, stderr)
	}
	lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
	if len(lines) != len(config.Presets()) {
		t.Errorf("listed %d languages, want %d: %q", len(lines), len(config.Presets()), stdout)
	}
	for _, want := range []string{"go          .go", "python      .py", "javascript  .js .jsx"} {
		if !strings.Contains(stdout, want+"\n") && !strings.Contains(stdout, want+" ") {
			t.Errorf("the list doesn't have %q: %q", want, stdout)
		}
	}
}

func TestAddLanguageErrors(t *testing.T) {
	setup(t, nil)
	for _, c := range []struct {
		args []string
		want []string // in stderr
	}{
		{[]string{"python"}, []string{"add-language: not in a repository", "litterbug add-language --global"}},
		{[]string{"--wat"}, []string{"unknown flag: --wat", "litterbug add-language --help"}},
	} {
		args := append([]string{"add-language"}, c.args...)
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

	stdout, stderr, status := runCLI(t, "add-language", "--help")
	if status != exitClean || stderr != "" || !strings.HasPrefix(stdout, "Usage:\n  litterbug add-language") || !strings.Contains(stdout, "--global") {
		t.Errorf("--help: status %d, stdout %q, stderr %q", status, stdout, stderr)
	}
}
