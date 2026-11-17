package config

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// testConfig is a config for the tests, kept apart from the defaults so
// they can change without breaking the tests.
const testConfig = `
exclude: [node_modules, vendor]
languages:
  go:
    extensions: [.go]
    patterns:
      fmt-print: {regex: '\bfmt\.Print'}
      spew: {regex: '\bspew\.'}
  javascript:
    enabled: true
    extensions: [.js, .ts]
    patterns:
      console: {regex: '\bconsole\.log\('}
  php:
    extensions: [.php]
    patterns:
      var-dump: {regex: '\bvar_dump\('}
  ruby:
    enabled: false
    extensions: [.rb]
    patterns:
      pry: {regex: 'binding\.pry'}
`

func TestDefaultConfig(t *testing.T) {
	f, err := parse(defaultConfig)
	if err != nil {
		t.Fatal(err)
	}
	for name, l := range f.Languages {
		if l.Enabled == nil {
			t.Errorf("language %s doesn't show its enabled switch", name)
		}
	}
	rules, err := compile(f, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"node_modules", "vendor"}; !slices.Equal(rules.Exclude, want) {
		t.Errorf("exclude = %v, want %v", rules.Exclude, want)
	}
	for path, want := range map[string][]string{
		"main.go": {"go"}, "web/app.TS": {"javascript"}, "App.vue": {"javascript"}, "views/home.blade.php": {"php"}, "README.md": nil,
	} {
		var languages []string
		for _, rule := range rules.For(path) {
			if !slices.Contains(languages, rule.Language) {
				languages = append(languages, rule.Language)
			}
		}
		if !slices.Equal(languages, want) {
			t.Errorf("languages for %s = %v, want %v", path, languages, want)
		}
	}
}

func TestLoadUsesTheBuiltInConfigWithoutAFile(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"without a global config": {"XDG_CONFIG_HOME": t.TempDir()},
		"without a home":          {"XDG_CONFIG_HOME": "", "HOME": ""},
	} {
		t.Run(name, func(t *testing.T) {
			for key, value := range env {
				t.Setenv(key, value)
			}
			rules, err := Load(t.TempDir(), nil)
			if err != nil {
				t.Fatal(err)
			}
			if got := ruleNames(rules, "main.go"); !slices.Contains(got, "go/fmt-print") {
				t.Errorf("rules for main.go = %v, want the built-in ones", got)
			}
			if home := env["XDG_CONFIG_HOME"]; home != "" {
				if entries, err := os.ReadDir(home); err != nil || len(entries) > 0 {
					t.Errorf("the config home has %v, %v; want nothing written to it", entries, err)
				}
			}
		})
	}
}

func TestGlobalPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	if path, err := GlobalPath(); err != nil || path != filepath.FromSlash("/xdg/litterbug/config.yml") {
		t.Errorf("with XDG_CONFIG_HOME: %q, %v", path, err)
	}
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/home/me")
	if path, err := GlobalPath(); err != nil || path != filepath.FromSlash("/home/me/.config/litterbug/config.yml") {
		t.Errorf("without XDG_CONFIG_HOME: %q, %v", path, err)
	}
}

func TestLoadUsesTheGlobalConfigAlone(t *testing.T) {
	path := useGlobalConfig(t, testConfig)
	rules, err := Load("", nil)
	if err != nil {
		t.Fatal(err)
	}
	// The defaults have more patterns for both, which don't apply.
	for path, want := range map[string][]string{
		"a.go": {"go/fmt-print", "go/spew"},
		"a.js": {"javascript/console"},
		"a.rb": nil,
	} {
		if got := ruleNames(rules, path); !slices.Equal(got, want) {
			t.Errorf("rules for %s = %v, want %v", path, got, want)
		}
	}
	if want := []string{"node_modules", "vendor"}; !slices.Equal(rules.Exclude, want) {
		t.Errorf("exclude = %v, want %v", rules.Exclude, want)
	}
	if content, err := os.ReadFile(path); err != nil || string(content) != testConfig {
		t.Errorf("the global config changed: %q, %v", content, err)
	}
}

func TestLoadUsesTheRepoConfigAlone(t *testing.T) {
	useGlobalConfig(t, testConfig)
	python := "languages:\n  python:\n    extensions: [.py]\n    patterns:\n      print: {regex: '^\\s*print\\('}\n"
	rules, err := loadWithRepo(t, "exclude: [build]\n"+python, nil)
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string][]string{"a.py": {"python/print"}, "a.go": nil, "a.php": nil} {
		if got := ruleNames(rules, path); !slices.Equal(got, want) {
			t.Errorf("rules for %s = %v, want %v", path, got, want)
		}
	}
	if want := []string{"build"}; !slices.Equal(rules.Exclude, want) {
		t.Errorf("exclude = %v, want %v", rules.Exclude, want)
	}

	if rules, err := loadWithRepo(t, python, nil); err != nil || rules.Exclude != nil {
		t.Errorf("a repo config without excludes: exclude = %v, %v; want none", rules.Exclude, err)
	}
	rules, err = loadWithRepo(t, "", nil)
	if got, want := ruleNames(rules, "a.go"), []string{"go/fmt-print", "go/spew"}; err != nil || !slices.Equal(got, want) {
		t.Errorf("without a repo config: rules for a.go = %v, %v; want the global config's %v", got, err, want)
	}
}

func TestLoadRejectsAnInvalidRepoConfig(t *testing.T) {
	useGlobalConfig(t, testConfig)
	goWith := func(patterns string) string {
		return "languages:\n  go:\n    extensions: [.go]\n    patterns:\n" + patterns
	}
	for _, c := range []struct {
		name, repo string
		want       []string // in the error, as well as RepoFile
	}{
		{"a blank file", "\n", []string{"no languages are enabled"}},
		{"only comments", "# nothing yet\n", []string{"no languages are enabled"}},
		{"an unknown key", "languages:\n  go: {enable: false}\n", []string{"enable"}},
		{"malformed YAML", "languages: [\n", nil},
		{"a regex that doesn't compile", goWith("      broken: {regex: '('}\n"), []string{"go/broken", "missing closing )"}},
		{"an empty regex", goWith("      empty: {regex: ''}\n"), []string{"go/empty", "needs a regex"}},
		{"a pattern without a regex", goWith("      new: {enabled: true}\n"), []string{"go/new", "needs a regex"}},
		{"a disabled pattern is still checked", goWith("      ok: {regex: 'x'}\n      bad: {enabled: false, regex: '('}\n"), []string{"go/bad"}},
		{"an extension without a dot", "languages:\n  python:\n    extensions: [py]\n    patterns:\n      print: {regex: 'print'}\n", []string{"python", `"py"`}},
		{"an extension with a slash", "languages:\n  go:\n    extensions: [.go/x]\n    patterns:\n      p: {regex: 'p'}\n", []string{"go", `".go/x"`}},
		{"a language without extensions", "languages:\n  python:\n    patterns:\n      print: {regex: 'print'}\n", []string{"language python", "extension"}},
		{"a language with no extensions", "languages:\n  go:\n    extensions: []\n    patterns:\n      p: {regex: 'p'}\n", []string{"language go", "extension"}},
		{"a language left empty", "languages:\n  python:\n", []string{"language python", "extension"}},
		{"a language without patterns", "languages:\n  python: {extensions: [.py]}\n", []string{"language python", "pattern"}},
		{"a disabled language is still checked", goWith("      p: {regex: 'p'}\n") + "  ruby:\n    enabled: false\n    extensions: [.rb]\n    patterns:\n      bad: {regex: '('}\n", []string{"ruby/bad"}},
		{"every language disabled", "languages:\n  go:\n    enabled: false\n    extensions: [.go]\n    patterns:\n      p: {regex: 'p'}\n", []string{"no languages are enabled"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := loadWithRepo(t, c.repo, nil)
			if err == nil {
				t.Fatal("no error")
			}
			for _, want := range append([]string{RepoFile}, c.want...) {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q doesn't mention %q", err, want)
				}
			}
		})
	}
}

func TestLoadRejectsAnInvalidGlobalConfig(t *testing.T) {
	path := useGlobalConfig(t, "languages: [\n")
	if _, err := Load("", nil); err == nil || !strings.Contains(err.Error(), path) {
		t.Errorf("malformed global config: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := Load("", nil); err == nil {
		t.Error("an unreadable global config gave no error")
	}
	notADirectory := filepath.Join(t.TempDir(), "file")
	writeFile(t, notADirectory, "")
	t.Setenv("XDG_CONFIG_HOME", notADirectory)
	if _, err := Load("", nil); err == nil {
		t.Error("a config home that is a file gave no error")
	}
}

func TestLangsPickTheLanguagesToScan(t *testing.T) {
	useGlobalConfig(t, testConfig)
	rules, err := loadWithRepo(t, "", []string{"ruby", "php"})
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string][]string{"a.rb": {"ruby/pry"}, "a.php": {"php/var-dump"}, "a.go": nil, "a.js": nil} {
		if got := ruleNames(rules, path); !slices.Equal(got, want) {
			t.Errorf("rules for %s = %v, want %v", path, got, want)
		}
	}

	_, err = loadWithRepo(t, "", []string{"go", "python"})
	if err == nil || !strings.Contains(err.Error(), `unknown language "python": the config has go, javascript, php, ruby`) {
		t.Errorf("unknown language: %v", err)
	}

	noGoPatterns := "languages:\n" +
		"  go:\n    extensions: [.go]\n    patterns:\n      fmt-print: {enabled: false, regex: 'fmt'}\n" +
		"  php:\n    extensions: [.php]\n    patterns:\n      var-dump: {regex: 'var_dump'}\n"
	if _, err := loadWithRepo(t, noGoPatterns, nil); err != nil {
		t.Errorf("an enabled language without enabled patterns was rejected: %v", err)
	}
	if _, err := loadWithRepo(t, noGoPatterns, []string{"go"}); err == nil || !strings.Contains(err.Error(), "language go has no enabled patterns") {
		t.Errorf("picking a language without enabled patterns: %v", err)
	}
}

func TestLanguagesSharingAnExtensionBothApply(t *testing.T) {
	useGlobalConfig(t, `
languages:
  php:
    extensions: [.php]
    patterns:
      dd: {regex: '\bdd\('}
  blade:
    extensions: [.blade.php]
    patterns:
      at-dd: {regex: '@dd\('}
`)
	rules, err := Load("", nil)
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string][]string{
		"views/home.blade.php": {"blade/at-dd", "php/dd"},
		"src/App.PHP":          {"php/dd"},
		"blade.php.txt":        nil,
	} {
		if got := ruleNames(rules, path); !slices.Equal(got, want) {
			t.Errorf("rules for %s = %v, want %v", path, got, want)
		}
	}
}

func TestWriteDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new", "config.yml")
	check := func(path, want string) {
		t.Helper()
		if content, err := os.ReadFile(path); err != nil || string(content) != want {
			t.Fatalf("%s = %q, %v; want %q", path, content, err, want)
		}
	}
	if backup, err := WriteDefault(path, false); err != nil || backup != "" {
		t.Fatalf("writing a new config: backup %q, %v", backup, err)
	}
	check(path, string(defaultConfig))

	writeFile(t, path, "# mine\n")
	if _, err := WriteDefault(path, false); !errors.Is(err, ErrExists) {
		t.Fatalf("writing over a config without force: %v", err)
	}
	check(path, "# mine\n")

	// Forcing again replaces the earlier backup.
	for _, old := range []string{"# mine\n", string(defaultConfig)} {
		backup, err := WriteDefault(path, true)
		if err != nil || backup != path+".bak" {
			t.Fatalf("forcing: backup %q, %v", backup, err)
		}
		check(path, string(defaultConfig))
		check(backup, old)
	}
}

// useGlobalConfig points the global config at a new directory, writes
// content to it, and returns its path.
func useGlobalConfig(t *testing.T, content string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	path := filepath.Join(home, "litterbug", "config.yml")
	writeFile(t, path, content)
	return path
}

// loadWithRepo loads the config for a repository with repo as its
// RepoFile, or without one when repo is empty.
func loadWithRepo(t *testing.T, repo string, langs []string) (Rules, error) {
	t.Helper()
	root := t.TempDir()
	if repo != "" {
		writeFile(t, filepath.Join(root, RepoFile), repo)
	}
	return Load(root, langs)
}

// ruleNames lists the rules for path as language/name.
func ruleNames(rules Rules, path string) []string {
	var names []string
	for _, rule := range rules.For(path) {
		names = append(names, rule.Language+"/"+rule.Name)
	}
	return names
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
