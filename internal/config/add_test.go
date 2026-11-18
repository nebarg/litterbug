package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func presetText(t *testing.T, name string) string {
	t.Helper()
	for _, p := range presets {
		if p.name == name {
			return p.text
		}
	}
	t.Fatalf("no preset %s", name)
	return ""
}

func TestAddLanguages(t *testing.T) {
	rust := indent(presetText(t, "rust"), "  ")
	java := indent(presetText(t, "java"), "  ")
	const goConfig = "languages:\n  go:\n    extensions: [.go]\n    patterns:\n      p: {regex: 'x'}\n"
	for _, c := range []struct {
		name, before string
		langs        []string
		want         string
	}{
		{"after the last language", "exclude: [vendor]\n\n" + goConfig, []string{"rust"},
			"exclude: [vendor]\n\n" + goConfig + "\n" + rust},
		{"before what follows the languages and comments about it", goConfig + "    # still go\n\n# about exclude\nexclude: [vendor]\n", []string{"rust"},
			goConfig + "    # still go\n\n" + rust + "\n# about exclude\nexclude: [vendor]\n"},
		{"between languages and a comment", "languages:\n  go:\n    extensions: [.go]\n    patterns: {p: {regex: 'x'}}\n# off for now\n  php:\n    extensions: [.php]\n    patterns:\n      p: {regex: 'x'}\n    enabled: false\n", []string{"rust"},
			"languages:\n  go:\n    extensions: [.go]\n    patterns: {p: {regex: 'x'}}\n# off for now\n  php:\n    extensions: [.php]\n    patterns:\n      p: {regex: 'x'}\n    enabled: false\n\n" + rust},
		{"at the languages' indent", "languages:\n    go:\n        extensions: [.go]\n        patterns:\n            p: {regex: 'x'}\n", []string{"rust"},
			"languages:\n    go:\n        extensions: [.go]\n        patterns:\n            p: {regex: 'x'}\n\n" + indent(rust, "  ")},
		{"several, by name", goConfig, []string{"rust", "java"},
			goConfig + "\n" + java + "\n" + rust},
		{"without languages", "exclude: [vendor]\n", []string{"rust"},
			"exclude: [vendor]\n\nlanguages:\n" + rust},
		{"without languages, before a document end", "exclude: [vendor]\n# the end\n...\n", []string{"rust"},
			"exclude: [vendor]\n# the end\n\nlanguages:\n" + rust + "...\n"},
		{"with nothing under languages", "languages:\n# none yet\nexclude: [vendor]\n", []string{"rust"},
			"languages:\n" + rust + "# none yet\nexclude: [vendor]\n"},
		{"in a file of comments", "# nothing yet\n", []string{"rust"},
			"# nothing yet\n\nlanguages:\n" + rust},
		{"in an empty file", "", []string{"rust"},
			"languages:\n" + rust},
		{"without a newline at the end", strings.TrimSuffix(goConfig, "\n"), []string{"rust"},
			goConfig + "\n" + rust},
		{"with Windows line endings", strings.ReplaceAll(goConfig, "\n", "\r\n"), []string{"rust"},
			strings.ReplaceAll(goConfig+"\n"+rust, "\n", "\r\n")},
		{"after a document start", "---\n" + goConfig, []string{"rust"},
			"---\n" + goConfig + "\n" + rust},
		{"to a config with every language off", "languages:\n  go:\n    enabled: false\n    extensions: [.go]\n    patterns:\n      p: {regex: 'x'}\n", []string{"rust"},
			"languages:\n  go:\n    enabled: false\n    extensions: [.go]\n    patterns:\n      p: {regex: 'x'}\n\n" + rust},
	} {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), RepoFile)
			writeFile(t, path, c.before)
			if err := AddLanguages(path, c.langs); err != nil {
				t.Fatal(err)
			}
			if got := readConfig(t, path); got != c.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, c.want)
			}
		})
	}
}

func TestAddLanguagesLeavesTheFileAloneOnErrors(t *testing.T) {
	const goConfig = "languages:\n  go:\n    extensions: [.go]\n    patterns:\n      p: {regex: 'x'}\n"
	for _, c := range []struct {
		name, before string
		langs        []string
		want         string // in the error
	}{
		{"a language it has", goConfig, []string{"rust", "go"}, "already has go"},
		{"an unknown language", goConfig, []string{"elixir"}, `"elixir"`},
		{"no languages", goConfig, nil, "no languages"},
		{"languages in flow style", "languages: {go: {extensions: [.go], patterns: {p: {regex: x}}}}\n", []string{"rust"}, "flow style"},
		{"an empty flow map", "languages: {}\n", []string{"rust"}, "flow style"},
		{"a config in flow style", "{exclude: [vendor]}\n", []string{"rust"}, "flow style"},
		{"a config that doesn't parse", "languages: [\n", []string{"rust"}, RepoFile},
		{"a config with an unknown key", "language: {}\n", []string{"rust"}, "language"},
		{"a config with a bad regex", goConfig + "      q: {regex: '('}\n", []string{"rust"}, "go/q"},
		{"languages set to null", "languages: ~\n", []string{"rust"}, "languages is set to ~"},
	} {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), RepoFile)
			writeFile(t, path, c.before)
			err := AddLanguages(path, c.langs)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want it to mention %q", err, c.want)
			}
			if got := readConfig(t, path); got != c.before {
				t.Errorf("the config changed to %q", got)
			}
		})
	}

	missing := filepath.Join(t.TempDir(), RepoFile)
	if err := AddLanguages(missing, []string{"rust"}); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("without a config: %v", err)
	}
	if _, err := os.Lstat(missing); err == nil {
		t.Error("a config was made")
	}
}

func TestAddLanguagesFollowsALinkAndKeepsPermissions(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "dotfiles", "config.yml")
	writeFile(t, target, "languages:\n  go:\n    extensions: [.go]\n    patterns:\n      p: {regex: 'x'}\n")
	if err := os.Chmod(target, 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "config.yml")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := AddLanguages(link, []string{"rust"}); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the link was replaced: %v, %v", info, err)
	}
	info, err := os.Stat(target)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Errorf("the config's permissions = %v, %v; want 0600", info, err)
	}
	if !strings.Contains(readConfig(t, target), "  rust:\n") {
		t.Error("rust wasn't added to the linked file")
	}
	entries, err := os.ReadDir(filepath.Dir(target))
	if err != nil || len(entries) != 1 {
		t.Errorf("files beside the config: %v, %v; want it alone", entries, err)
	}
}

func TestCheckAddedRejectsOtherChanges(t *testing.T) {
	const goConfig = "exclude: [vendor]\nlanguages:\n  go:\n    extensions: [.go]\n    patterns:\n      p: {regex: 'x'}\n"
	old, err := parse([]byte(goConfig))
	if err != nil {
		t.Fatal(err)
	}
	picked, err := pickPresets([]string{"rust"})
	if err != nil {
		t.Fatal(err)
	}
	rust := indent(presetText(t, "rust"), "  ")
	if err := checkAdded(old, goConfig+rust, picked); err != nil {
		t.Errorf("rust added as built in: %v", err)
	}
	for edited, want := range map[string]string{
		strings.Replace(goConfig, "vendor", "dist", 1) + rust:    "change the rest",
		strings.Replace(goConfig, "'x'", "'y'", 1) + rust:        "change the rest",
		goConfig + strings.Replace(rust, `\bdbg!`, `\bdbug!`, 1): "change the rest",
		goConfig:                 "change the rest",
		goConfig + "  rust: [\n": "break the config",
	} {
		if err := checkAdded(old, edited, picked); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("checkAdded(%q) = %v, want it to mention %q", edited, err, want)
		}
	}
}

func readConfig(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
