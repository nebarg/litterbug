package config

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// testGlobal is a global config for the tests, kept apart from the
// defaults so they can change without breaking the tests.
const testGlobal = `
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
	f, err := parse("default.yml", defaultConfig)
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

func TestLoadCreatesTheGlobalConfig(t *testing.T) {
	path := useGlobalConfig(t, "")
	var notices bytes.Buffer
	rules, err := Load("", nil, &notices)
	if err != nil {
		t.Fatal(err)
	}
	if written, err := os.ReadFile(path); err != nil || !bytes.Equal(written, defaultConfig) {
		t.Fatalf("written config = %q, %v; want the default", written, err)
	}
	if want := "created default config at " + path; !strings.Contains(notices.String(), want) {
		t.Errorf("notices = %q, want %q", notices.String(), want)
	}
	if len(rules.For("main.go")) == 0 {
		t.Error("the created config has no rules for Go")
	}
	notices.Reset()
	if _, err := Load("", nil, &notices); err != nil || notices.Len() > 0 {
		t.Fatalf("second load: notices %q, %v", notices.String(), err)
	}
}

func TestLoadFallsBackToTheBuiltInConfig(t *testing.T) {
	readOnly := t.TempDir()
	if err := os.Chmod(readOnly, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(readOnly, 0755) }) // So TempDir can remove it.
	cases := map[string]map[string]string{"without a home": {"XDG_CONFIG_HOME": "", "HOME": ""}}
	if err := os.Mkdir(filepath.Join(readOnly, "probe"), 0755); err != nil {
		cases["when it can't be saved"] = map[string]string{"XDG_CONFIG_HOME": readOnly}
	} else {
		t.Log("directory permissions aren't enforced for this user, so saving can't fail")
	}
	for name, env := range cases {
		t.Run(name, func(t *testing.T) {
			for key, value := range env {
				t.Setenv(key, value)
			}
			var notices bytes.Buffer
			rules, err := Load("", nil, &notices)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(notices.String(), "using the built-in config") {
				t.Errorf("notices = %q", notices.String())
			}
			if len(rules.For("main.go")) == 0 {
				t.Error("the built-in config has no rules for Go")
			}
		})
	}
}

func TestGlobalPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	if path, err := globalPath(); err != nil || path != filepath.FromSlash("/xdg/litterbug/config.yml") {
		t.Errorf("with XDG_CONFIG_HOME: %q, %v", path, err)
	}
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/home/me")
	if path, err := globalPath(); err != nil || path != filepath.FromSlash("/home/me/.config/litterbug/config.yml") {
		t.Errorf("without XDG_CONFIG_HOME: %q, %v", path, err)
	}
}

func TestLoadReadsTheGlobalConfig(t *testing.T) {
	path := useGlobalConfig(t, testGlobal)
	var notices bytes.Buffer
	rules, err := Load("", nil, &notices)
	if err != nil || notices.Len() > 0 {
		t.Fatalf("notices %q, %v", notices.String(), err)
	}
	if got, want := ruleNames(rules, "a.go"), []string{"go/fmt-print", "go/spew"}; !slices.Equal(got, want) {
		t.Errorf("rules for a.go = %v, want %v", got, want)
	}
	if got := ruleNames(rules, "a.rb"); len(got) > 0 {
		t.Errorf("a disabled language has rules: %v", got)
	}
	if want := []string{"node_modules", "vendor"}; !slices.Equal(rules.Exclude, want) {
		t.Errorf("exclude = %v, want %v", rules.Exclude, want)
	}
	if content, err := os.ReadFile(path); err != nil || string(content) != testGlobal {
		t.Errorf("the global config changed: %q, %v", content, err)
	}
}

func TestLoadMergesTheRepoConfig(t *testing.T) {
	useGlobalConfig(t, testGlobal)
	for _, c := range []struct {
		name, repo, path string
		want             []string
	}{
		{"without a repo config", "", "a.go", []string{"go/fmt-print", "go/spew"}},
		{"a repo config of only comments", "# nothing yet\n", "a.go", []string{"go/fmt-print", "go/spew"}},
		{"a language left empty", "languages:\n  go:\n", "a.go", []string{"go/fmt-print", "go/spew"}},
		{"a disabled language", "languages:\n  php: {enabled: false}\n", "a.php", nil},
		{"a disabled pattern", "languages:\n  go:\n    patterns:\n      fmt-print: {enabled: false}\n", "a.go", []string{"go/spew"}},
		{"an enabled language", "languages:\n  ruby: {enabled: true}\n", "a.rb", []string{"ruby/pry"}},
		{"replaced extensions drop the old ones", "languages:\n  javascript: {extensions: [.mjs]}\n", "a.js", nil},
		{"replaced extensions add the new ones", "languages:\n  javascript: {extensions: [.mjs]}\n", "a.mjs", []string{"javascript/console"}},
		{"an added pattern", "languages:\n  go:\n    patterns:\n      pp: {regex: '\\bpp\\.'}\n", "a.go", []string{"go/fmt-print", "go/pp", "go/spew"}},
		{"an added language", "languages:\n  python:\n    extensions: [.py]\n    patterns:\n      print: {regex: '^\\s*print\\('}\n", "a.py", []string{"python/print"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			rules, err := loadWithRepo(t, c.repo, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got := ruleNames(rules, c.path); !slices.Equal(got, c.want) {
				t.Errorf("rules for %s = %v, want %v", c.path, got, c.want)
			}
		})
	}
}

func TestLoadReadsAnEmptyRepoConfig(t *testing.T) {
	useGlobalConfig(t, testGlobal)
	root := t.TempDir()
	writeFile(t, filepath.Join(root, RepoFile), "")
	rules, err := Load(root, nil, io.Discard)
	if err != nil || len(rules.For("a.go")) != 2 {
		t.Fatalf("rules for a.go = %v, %v", ruleNames(rules, "a.go"), err)
	}
}

func TestRepoConfigReplacesARegex(t *testing.T) {
	useGlobalConfig(t, testGlobal)
	rules, err := loadWithRepo(t, "languages:\n  go:\n    patterns:\n      spew: {regex: 'spew\\.Dump\\('}\n", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range rules.For("a.go") {
		if rule.Name == "spew" && rule.Regex.String() != `spew\.Dump\(` {
			t.Errorf("spew regex = %s", rule.Regex)
		}
	}
}

func TestRepoConfigReplacesTheExcludes(t *testing.T) {
	useGlobalConfig(t, testGlobal)
	for _, c := range []struct {
		name, repo string
		want       []string
	}{
		{"inherited when left out", "languages: {}\n", []string{"node_modules", "vendor"}},
		{"replaced", "exclude: [build]\n", []string{"build"}},
		{"emptied", "exclude: []\n", nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			rules, err := loadWithRepo(t, c.repo, nil)
			if err != nil || !slices.Equal(rules.Exclude, c.want) {
				t.Fatalf("exclude = %v, %v; want %v", rules.Exclude, err, c.want)
			}
		})
	}
}

func TestLoadRejectsAnInvalidRepoConfig(t *testing.T) {
	useGlobalConfig(t, testGlobal)
	for _, c := range []struct {
		name, repo string
		want       []string // in the error
	}{
		{"an unknown key", "languages:\n  go: {enable: false}\n", []string{RepoFile, "enable"}},
		{"malformed YAML", "languages: [\n", []string{RepoFile}},
		{"a regex that doesn't compile", "languages:\n  go:\n    patterns:\n      broken: {regex: '('}\n", []string{RepoFile, "go/broken", "missing closing )"}},
		{"an empty regex", "languages:\n  go:\n    patterns:\n      empty: {regex: ''}\n", []string{RepoFile, "go/empty", "empty"}},
		{"an extension without a dot", "languages:\n  python: {extensions: [py]}\n", []string{RepoFile, "python", `"py"`}},
		{"an extension with a slash", "languages:\n  go: {extensions: [.go/x]}\n", []string{RepoFile, "go", `".go/x"`}},
		{"a language without extensions", "languages:\n  python:\n    patterns:\n      print: {regex: 'print'}\n", []string{"language python", "extension"}},
		{"a language left with no extensions", "languages:\n  go: {extensions: []}\n", []string{"language go", "extension"}},
		{"a new language left empty", "languages:\n  python:\n", []string{"language python", "extension"}},
		{"a language without patterns", "languages:\n  python: {extensions: [.py]}\n", []string{"language python", "pattern"}},
		{"a pattern without a regex", "languages:\n  go:\n    patterns:\n      new: {enabled: true}\n", []string{"go/new", "regex"}},
		{"a disabled language is still checked", "languages:\n  ruby:\n    patterns:\n      bad: {enabled: false}\n", []string{"ruby/bad", "regex"}},
		{"every language disabled", "languages:\n  go: {enabled: false}\n  javascript: {enabled: false}\n  php: {enabled: false}\n", []string{"no languages are enabled"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := loadWithRepo(t, c.repo, nil)
			if err == nil {
				t.Fatal("no error")
			}
			for _, want := range c.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q doesn't mention %q", err, want)
				}
			}
		})
	}
}

func TestLoadRejectsAnInvalidGlobalConfig(t *testing.T) {
	path := useGlobalConfig(t, "languages: [\n")
	if _, err := Load("", nil, io.Discard); err == nil || !strings.Contains(err.Error(), path) {
		t.Errorf("malformed global config: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := Load("", nil, io.Discard); err == nil {
		t.Error("an unreadable global config gave no error")
	}
	notADirectory := filepath.Join(t.TempDir(), "file")
	writeFile(t, notADirectory, "")
	t.Setenv("XDG_CONFIG_HOME", notADirectory)
	if _, err := Load("", nil, io.Discard); err == nil {
		t.Error("a config home that is a file gave no error")
	}
}

func TestLangsPickTheLanguagesToScan(t *testing.T) {
	useGlobalConfig(t, testGlobal)
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

	noPatterns := "languages:\n  go:\n    patterns:\n      fmt-print: {enabled: false}\n      spew: {enabled: false}\n"
	if _, err := loadWithRepo(t, noPatterns, nil); err != nil {
		t.Errorf("an enabled language without enabled patterns was rejected: %v", err)
	}
	if _, err := loadWithRepo(t, noPatterns, []string{"go"}); err == nil || !strings.Contains(err.Error(), "language go has no enabled patterns") {
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
	rules, err := Load("", nil, io.Discard)
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

func TestMergeLeavesItsInputsUnchanged(t *testing.T) {
	base, err := parse("base", []byte(testGlobal))
	if err != nil {
		t.Fatal(err)
	}
	over, err := parse("over", []byte("languages:\n  go:\n    enabled: false\n    patterns:\n      pp: {regex: 'pp'}\n      spew: {enabled: false}\n"))
	if err != nil {
		t.Fatal(err)
	}
	merge(base, over)
	golang := base.Languages["go"]
	if golang.Enabled != nil || len(golang.Patterns) != 2 || golang.Patterns["spew"].Enabled != nil {
		t.Errorf("merging changed the base config's go: %+v", golang)
	}
}

// useGlobalConfig points the global config at a new directory, writes
// content to it unless content is empty, and returns its path.
func useGlobalConfig(t *testing.T, content string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	path := filepath.Join(home, "litterbug", "config.yml")
	if content != "" {
		writeFile(t, path, content)
	}
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
	return Load(root, langs, io.Discard)
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
