package main

import (
	"bytes"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/nebarg/litterbug/internal/config"
	"github.com/nebarg/litterbug/internal/repotest"
	"github.com/nebarg/litterbug/internal/scan"
)

const testConfig = `
exclude: [vendor]
languages:
  go:
    extensions: [.go]
    patterns:
      fmt-print: {regex: '\bfmt\.Print'}
  php:
    extensions: [.php]
    patterns:
      var-dump: {regex: '\bvar_dump\s*\('}
  ruby:
    enabled: false
    extensions: [.rb]
    patterns:
      pry: {regex: 'binding\.pry'}
`

// goConfig and phpConfig are repository configs with one language each.
const (
	goConfig  = "languages:\n  go:\n    extensions: [.go]\n    patterns:\n      fmt-print: {regex: 'fmt\\.Print'}\n"
	phpConfig = "languages:\n  php:\n    extensions: [.php]\n    patterns:\n      var-dump: {regex: 'var_dump'}\n"
)

var testTree = map[string]string{
	"main.go":       "package main\n\nfunc main() {\n\tfmt.Println(1)\n}\n",
	"clean.go":      "package main\n",
	"web/index.php": "<?php\nvar_dump($x);\n",
	"vendor/lib.go": "fmt.Println(1)\n",
	"app.rb":        "binding.pry\n",
	"notes.txt":     "fmt.Println(1)\n",
}

const (
	mainFinding = "main.go:4: fmt.Println(1)  [go/fmt-print]"
	webFinding  = "web/index.php:2: var_dump($x);  [php/var-dump]"
)

func TestRunReportsFindings(t *testing.T) {
	setup(t, testTree)
	stdout, stderr, status := runCLI(t)
	if want := mainFinding + "\n" + webFinding + "\n\n2 debug statements in 2 files\n"; stdout != want || stderr != "" || status != exitFound {
		t.Fatalf("status %d\nstdout %q\nstderr %q\nwant stdout %q", status, stdout, stderr, want)
	}
}

func TestRunWithNothingFound(t *testing.T) {
	setup(t, map[string]string{"clean.go": "package main\n", "notes.txt": "fmt.Println(1)\n"})
	stdout, stderr, status := runCLI(t)
	if stdout != "No debug statements found.\n" || stderr != "" || status != exitClean {
		t.Fatalf("status %d\nstdout %q\nstderr %q", status, stdout, stderr)
	}
}

func TestRunCountsOneOfEach(t *testing.T) {
	setup(t, map[string]string{"main.go": "fmt.Println(1)\n"})
	stdout, _, status := runCLI(t)
	if want := "main.go:1: fmt.Println(1)  [go/fmt-print]\n\n1 debug statement in 1 file\n"; stdout != want || status != exitFound {
		t.Fatalf("status %d, stdout %q", status, stdout)
	}
}

func TestRunScansThePathsGiven(t *testing.T) {
	dir := setup(t, testTree)
	for _, c := range []struct {
		args []string
		want []string
	}{
		{[]string{"web"}, []string{webFinding}},
		{[]string{"./web/"}, []string{webFinding}},
		{[]string{filepath.Join(dir, "web")}, []string{filepath.ToSlash(dir) + "/" + webFinding}},
		{[]string{"main.go"}, []string{mainFinding}},
		{[]string{"./main.go"}, []string{mainFinding}},
		{[]string{"web", "main.go"}, []string{mainFinding, webFinding}},
		{[]string{".", "main.go"}, []string{mainFinding, webFinding}},
		{[]string{"main.go", "main.go"}, []string{mainFinding}},
		{[]string{"vendor/lib.go"}, []string{"vendor/lib.go:1: fmt.Println(1)  [go/fmt-print]"}},
		{[]string{"vendor"}, []string{"vendor/lib.go:1: fmt.Println(1)  [go/fmt-print]"}},
		{[]string{"notes.txt"}, nil},
	} {
		t.Run(strings.Join(c.args, " "), func(t *testing.T) {
			stdout, stderr, status := runCLI(t, c.args...)
			if got := findingLines(stdout); !slices.Equal(got, c.want) || stderr != "" || status != wantStatus(c.want) {
				t.Fatalf("status %d, stderr %q\n got  %q\n want %q", status, stderr, got, c.want)
			}
		})
	}
}

func TestRunScansANamedFileAWalkWouldSkip(t *testing.T) {
	setup(t, map[string]string{"big.go": "fmt.Println(1)\n" + strings.Repeat("x", 2<<20) + "\n"})
	if err := os.Symlink("big.go", "link.go"); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if stdout, stderr, status := runCLI(t); stdout != "No debug statements found.\n" || stderr != "" || status != exitClean {
		t.Errorf("walking: status %d, stdout %q, stderr %q", status, stdout, stderr)
	}
	stdout, stderr, status := runCLI(t, "big.go", "link.go")
	want := []string{"big.go:1: fmt.Println(1)  [go/fmt-print]", "link.go:1: fmt.Println(1)  [go/fmt-print]"}
	if got := findingLines(stdout); !slices.Equal(got, want) || stderr != "" || status != exitFound {
		t.Errorf("naming them: status %d, stderr %q\n got  %q\n want %q", status, stderr, got, want)
	}
}

func TestRunReportsANamedFileItCannotRead(t *testing.T) {
	setup(t, map[string]string{"locked.go": "fmt.Println(1)\n"})
	if err := os.Chmod("locked.go", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile("locked.go"); err == nil {
		t.Skip("file permissions aren't enforced for this user")
	}
	stdout, stderr, status := runCLI(t, "locked.go")
	if status != exitError || stdout != "" || !strings.Contains(stderr, "locked.go: permission denied") {
		t.Errorf("status %d, stdout %q, stderr %q", status, stdout, stderr)
	}
}

func TestRunColor(t *testing.T) {
	setup(t, testTree)
	t.Setenv("NO_COLOR", "1")
	colored := "\x1b[1mmain.go\x1b[0m:\x1b[2m4\x1b[0m: \x1b[31mfmt.Print\x1b[0mln(1)  \x1b[2m[go/fmt-print]\x1b[0m\n"
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"main.go"}, mainFinding + "\n"},
		{[]string{"--color", "never", "main.go"}, mainFinding + "\n"},
		{[]string{"--color=always", "main.go"}, colored},
	} {
		t.Run(strings.Join(c.args, " "), func(t *testing.T) {
			stdout, stderr, status := runCLI(t, c.args...)
			if want := c.want + "\n1 debug statement in 1 file\n"; stdout != want || stderr != "" || status != exitFound {
				t.Errorf("status %d, stderr %q\n got  %q\n want %q", status, stderr, stdout, want)
			}
		})
	}
}

func TestRunLang(t *testing.T) {
	setup(t, testTree)
	for _, c := range []struct {
		args []string
		want []string
	}{
		{[]string{"--lang", "ruby"}, []string{"app.rb:1: binding.pry  [ruby/pry]"}},
		{[]string{"-l", "go,php"}, []string{mainFinding, webFinding}},
		{[]string{"--lang", "go", "--lang", "php"}, []string{mainFinding, webFinding}},
		{[]string{"--lang", " php, ruby "}, []string{"app.rb:1: binding.pry  [ruby/pry]", webFinding}},
	} {
		t.Run(strings.Join(c.args, " "), func(t *testing.T) {
			stdout, stderr, status := runCLI(t, c.args...)
			if got := findingLines(stdout); !slices.Equal(got, c.want) || stderr != "" || status != wantStatus(c.want) {
				t.Fatalf("status %d, stderr %q\n got  %q\n want %q", status, stderr, got, c.want)
			}
		})
	}
}

func TestRunExclude(t *testing.T) {
	setup(t, testTree)
	for _, c := range []struct {
		args []string
		want []string
	}{
		{[]string{"-e", "web"}, []string{mainFinding}},
		{[]string{"--exclude", "web/index.php"}, []string{mainFinding}},
		{[]string{"-e", "main.go"}, []string{webFinding}},
		{[]string{"-e", "main.go", "-e", "index.php"}, nil},
		{[]string{"-e", "web", "web"}, []string{webFinding}},
		{[]string{"-e", "*.php"}, []string{mainFinding}},
		{[]string{"-e", "w*/"}, []string{mainFinding}},
		{[]string{"-e", "**/index.php"}, []string{mainFinding}},
		{[]string{"-e", "web/**"}, []string{mainFinding}},
		{[]string{"-e", "*.go", "-e", "*.php"}, nil},
	} {
		t.Run(strings.Join(c.args, " "), func(t *testing.T) {
			stdout, stderr, status := runCLI(t, c.args...)
			if got := findingLines(stdout); !slices.Equal(got, c.want) || stderr != "" || status != wantStatus(c.want) {
				t.Fatalf("status %d, stderr %q\n got  %q\n want %q", status, stderr, got, c.want)
			}
		})
	}
}

func TestRunUsesTheRepoConfigFromASubdirectory(t *testing.T) {
	files := map[string]string{
		config.RepoFile:     "exclude: [web/gen, '*.min.php']\n" + phpConfig,
		"web/gen/out.php":   "var_dump($y);\n",
		"web/admin.min.php": "var_dump($y);\n",
	}
	maps.Copy(files, testTree)
	dir := setup(t, files)
	repotest.Init(t, dir)
	stdout, _, _ := runCLI(t)
	if got, want := findingLines(stdout), []string{webFinding}; !slices.Equal(got, want) {
		t.Errorf("from the root:\n got  %q\n want %q", got, want)
	}
	t.Chdir(filepath.Join(dir, "web"))
	stdout, _, _ = runCLI(t)
	if got, want := findingLines(stdout), []string{"index.php:2: var_dump($x);  [php/var-dump]"}; !slices.Equal(got, want) {
		t.Errorf("from web:\n got  %q\n want %q", got, want)
	}
}

func TestRunUsesTheConfigOfTheRepositoryEachPathIsIn(t *testing.T) {
	dir := setup(t, map[string]string{
		"loose.go":               "fmt.Println(1)\n",
		"app/" + config.RepoFile: "exclude: [gen/**]\n" + goConfig,
		"app/main.go":            "fmt.Println(1)\n",
		"app/index.php":          "<?php\nvar_dump($x);\n",
		"app/gen/out.go":         "fmt.Println(1)\n",
		"lib/" + config.RepoFile: phpConfig,
		"lib/lib.go":             "fmt.Println(1)\n",
		"lib/lib.php":            "<?php\nvar_dump($x);\n",
	})
	repotest.Init(t, filepath.Join(dir, "app"))
	repotest.Init(t, filepath.Join(dir, "lib"))
	for _, c := range []struct {
		from string
		args []string
		want []string
	}{
		{".", []string{"app", "lib/lib.go", "lib/lib.php", "loose.go"}, []string{
			"app/main.go:1: fmt.Println(1)  [go/fmt-print]",
			"lib/lib.php:2: var_dump($x);  [php/var-dump]",
			"loose.go:1: fmt.Println(1)  [go/fmt-print]",
		}},
		{"app", []string{".", "../lib"}, []string{
			"../lib/lib.php:2: var_dump($x);  [php/var-dump]",
			"main.go:1: fmt.Println(1)  [go/fmt-print]",
		}},
	} {
		t.Run(c.from+": "+strings.Join(c.args, " "), func(t *testing.T) {
			t.Chdir(filepath.Join(dir, c.from))
			stdout, stderr, status := runCLI(t, c.args...)
			if got := findingLines(stdout); !slices.Equal(got, c.want) || stderr != "" || status != exitFound {
				t.Fatalf("status %d, stderr %q\n got  %q\n want %q", status, stderr, got, c.want)
			}
		})
	}
}

func TestRunErrors(t *testing.T) {
	dir := setup(t, testTree)
	for _, c := range []struct {
		args []string
		want []string // in stderr
	}{
		{[]string{"missing"}, []string{"litterbug: ", "missing", "no such file or directory"}},
		{[]string{"main.go", "missing"}, []string{"missing"}},
		{[]string{"--wat"}, []string{"unknown flag: --wat", "Run litterbug --help"}},
		{[]string{"--lang", "python"}, []string{`unknown language "python"`}},
		{[]string{"--lang", "go,"}, []string{"--lang needs a language name"}},
		{[]string{"-e", "/"}, []string{"--exclude", "needs a file or directory"}},
		{[]string{"-e", "[a"}, []string{"--exclude", "not a valid pattern"}},
		{[]string{"--color", "sometimes"}, []string{`--color must be auto, always or never, not "sometimes"`}},
	} {
		t.Run(strings.Join(c.args, " "), func(t *testing.T) {
			stdout, stderr, status := runCLI(t, c.args...)
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

	repotest.Init(t, dir)
	repotest.WriteFiles(t, dir, map[string]string{config.RepoFile: "languages:\n  go: {enable: false}\n"})
	if stdout, stderr, status := runCLI(t); status != exitError || stdout != "" || !strings.Contains(stderr, config.RepoFile) {
		t.Errorf("invalid repo config: status %d, stdout %q, stderr %q", status, stdout, stderr)
	}
}

func TestRunHelpAndVersion(t *testing.T) {
	setup(t, testTree)
	for _, flag := range []string{"--help", "-h"} {
		stdout, stderr, status := runCLI(t, flag)
		if status != exitClean || stderr != "" || !strings.HasPrefix(stdout, "Usage:") || !strings.Contains(stdout, "--lang") || !strings.Contains(stdout, "--exclude") {
			t.Errorf("%s: status %d, stderr %q, stdout %q", flag, status, stderr, stdout)
		}
	}
	stdout, stderr, status := runCLI(t, "--version", "missing")
	if status != exitClean || stderr != "" || !regexp.MustCompile(`^litterbug \S+\n$`).MatchString(stdout) {
		t.Errorf("--version: status %d, stderr %q, stdout %q", status, stderr, stdout)
	}
}

func TestRunUsesTheBuiltInConfigWithoutOne(t *testing.T) {
	setup(t, map[string]string{"main.go": "fmt.Println(1)\n"})
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	stdout, stderr, status := runCLI(t)
	if got, want := findingLines(stdout), []string{"main.go:1: fmt.Println(1)  [go/fmt-print]"}; !slices.Equal(got, want) || stderr != "" || status != exitFound {
		t.Fatalf("status %d, stderr %q\n got  %q\n want %q", status, stderr, got, want)
	}
	if entries, err := os.ReadDir(home); err != nil || len(entries) > 0 {
		t.Errorf("the config home has %v, %v; want nothing written to it", entries, err)
	}
}

func TestDisplayLine(t *testing.T) {
	x := func(n int) string { return strings.Repeat("x", n) }
	for _, c := range []struct {
		name, text string
		start, end int
		want       string
	}{
		{"trimmed", "\tfmt.Println(1)  ", 1, 10, "fmt.Println(1)"},
		{"control characters", "x := \"a\x1b[31mred\"", 0, 1, "x := \"a [31mred\""},
		{"as long as can be shown", x(maxLineLength), 0, 1, x(maxLineLength)},
		{"a match at the start of a long line", x(maxLineLength) + "y", 0, 1, x(maxLineLength-1) + "…"},
		{"a match at the end of a long line", x(300) + "console.log(1)", 300, 312, "…" + x(145) + "console.log(1)"},
		{"a match in the middle", x(200) + "dd(1)" + strings.Repeat("y", 200), 200, 203, "…" + x(77) + "dd(1)" + strings.Repeat("y", 76) + "…"},
		{"characters, not bytes", strings.Repeat("é", 300) + "dd(1)", 600, 603, "…" + strings.Repeat("é", 154) + "dd(1)"},
		{"offsets before trimming", "\t\t" + x(300) + "dd(1)", 302, 305, "…" + x(154) + "dd(1)"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := displayLine(c.text, c.start, c.end)
			if got != c.want {
				t.Errorf("excerpt = %q, want %q", got, c.want)
			}
			if n := utf8.RuneCountInString(got); n > maxLineLength {
				t.Errorf("%d characters shown", n)
			}
		})
	}
	// A match longer than the window shows from its start.
	got := displayLine(strings.Repeat("a", 100)+strings.Repeat("b", 300)+strings.Repeat("c", 100), 100, 400)
	if want := "…" + strings.Repeat("b", maxLineLength-2) + "…"; got != want {
		t.Errorf("a long match = %q, want %q", got, want)
	}
}

// displayLine is a finding's line as excerpt shows it, in one piece.
func displayLine(text string, start, end int) string {
	before, match, after := excerpt(text, start, end)
	return before + match + after
}

func TestReportCleansPaths(t *testing.T) {
	findings := []scan.Finding{{Path: "a\x1b[2Jb.go", Line: 3, Text: "fmt.Println(1)", Rule: config.Rule{Language: "go", Name: "fmt-print"}, End: 11}}
	for s, want := range map[style]string{
		{}:              "a [2Jb.go:3: fmt.Println(1)  [go/fmt-print]\n\n1 debug statement in 1 file\n",
		{grouped: true}: "a [2Jb.go\n  3  fmt.Println(1)  [go/fmt-print]\n\n1 debug statement in 1 file\n",
	} {
		var out bytes.Buffer
		report(&out, findings, s)
		if out.String() != want {
			t.Errorf("report(%+v) = %q, want %q", s, out.String(), want)
		}
	}
}

// groupedFindings are in two files, sorted as a scan returns them.
var groupedFindings = []scan.Finding{
	{Path: "internal/api/handler.go", Line: 42, Text: "\tfmt.Println(\"user\", u)", Start: 1, End: 12, Rule: config.Rule{Language: "go", Name: "fmt-print"}},
	{Path: "web/src/cart.ts", Line: 17, Text: "  console.log(items)", Start: 2, End: 14, Rule: config.Rule{Language: "javascript", Name: "console"}},
	{Path: "web/src/cart.ts", Line: 120, Text: "x = 1; console.debug(state)", Start: 7, End: 21, Rule: config.Rule{Language: "javascript", Name: "console"}},
}

func TestReportGrouped(t *testing.T) {
	var out bytes.Buffer
	report(&out, groupedFindings, style{grouped: true})
	want := `internal/api/handler.go
   42  fmt.Println("user", u)  [go/fmt-print]

web/src/cart.ts
   17  console.log(items)  [javascript/console]
  120  x = 1; console.debug(state)  [javascript/console]

3 debug statements in 2 files
`
	if out.String() != want {
		t.Errorf("report:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestReportGroupedInColor(t *testing.T) {
	var out bytes.Buffer
	report(&out, groupedFindings[1:], style{grouped: true, color: true})
	want := "\x1b[1mweb/src/cart.ts\x1b[0m\n" +
		"  \x1b[2m 17\x1b[0m  \x1b[31mconsole.log(\x1b[0mitems)  \x1b[2m[javascript/console]\x1b[0m\n" +
		"  \x1b[2m120\x1b[0m  x = 1; \x1b[31mconsole.debug(\x1b[0mstate)  \x1b[2m[javascript/console]\x1b[0m\n" +
		"\n2 debug statements in 1 file\n"
	if out.String() != want {
		t.Errorf("report:\n got  %q\n want %q", out.String(), want)
	}
}

func TestReportLinesInColor(t *testing.T) {
	var out bytes.Buffer
	report(&out, groupedFindings[1:2], style{color: true})
	want := "\x1b[1mweb/src/cart.ts\x1b[0m:\x1b[2m17\x1b[0m: \x1b[31mconsole.log(\x1b[0mitems)  \x1b[2m[javascript/console]\x1b[0m\n" +
		"\n1 debug statement in 1 file\n"
	if out.String() != want {
		t.Errorf("report:\n got  %q\n want %q", out.String(), want)
	}
}

func TestTerminalStyle(t *testing.T) {
	for _, c := range []struct {
		terminal       bool
		color, noColor string
		want           style
	}{
		{false, "auto", "", style{}},
		{false, "auto", "1", style{}},
		{true, "auto", "", style{grouped: true, color: true}},
		{true, "auto", "1", style{grouped: true}},
		{false, "always", "1", style{color: true}},
		{true, "always", "1", style{grouped: true, color: true}},
		{false, "never", "", style{}},
		{true, "never", "", style{grouped: true}},
	} {
		if got := terminalStyle(c.terminal, c.color, c.noColor); got != c.want {
			t.Errorf("terminalStyle(%v, %q, %q) = %+v, want %+v", c.terminal, c.color, c.noColor, got, c.want)
		}
	}
}

func TestPipesAndFilesAreNotTerminals(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close(); _ = writer.Close() }()
	file, err := os.Create(filepath.Join(t.TempDir(), "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	for _, f := range []*os.File{writer, file} {
		if isTerminal(f) {
			t.Errorf("%s is taken for a terminal", f.Name())
		}
	}
	if got := styleFor(&bytes.Buffer{}, "auto"); got != (style{}) {
		t.Errorf("styleFor(a buffer) = %+v", got)
	}
}

func TestExcerptSplitsAroundTheMatch(t *testing.T) {
	long := strings.Repeat("x", 300)
	for _, c := range []struct {
		name, text           string
		start, end           int
		before, match, after string
	}{
		{"trimmed", "\tfmt.Println(1)  ", 1, 12, "", "fmt.Println", "(1)"},
		{"in the middle", "x := 1; dd($x);", 8, 11, "x := 1; ", "dd(", "$x);"},
		{"control characters", "a\x1b;dd(\x1b)", 3, 6, "a ;", "dd(", " )"},
		{"a long line", long + "dd(1)", 300, 303, "…" + strings.Repeat("x", 154), "dd(", "1)"},
		{"an empty match", "x", 0, 0, "", "", "x"},
	} {
		t.Run(c.name, func(t *testing.T) {
			before, match, after := excerpt(c.text, c.start, c.end)
			if before != c.before || match != c.match || after != c.after {
				t.Errorf("excerpt = %q, %q, %q; want %q, %q, %q", before, match, after, c.before, c.match, c.after)
			}
		})
	}
}

// setup writes files to a new directory and makes it the working
// directory, with testConfig as the global config and the user's Git
// configuration kept out. It returns the directory.
func setup(t *testing.T, files map[string]string) string {
	t.Helper()
	repotest.IsolateGit(t)
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	repotest.WriteFiles(t, home, map[string]string{"litterbug/config.yml": testConfig})
	dir := t.TempDir()
	repotest.WriteFiles(t, dir, files)
	t.Chdir(dir)
	return dir
}

func runCLI(t *testing.T, args ...string) (stdout, stderr string, status int) {
	t.Helper()
	var out, errOut bytes.Buffer
	status = run(t.Context(), args, &out, &errOut)
	return out.String(), errOut.String(), status
}

// findingLines are the findings stdout lists, without the count after
// them, or nil when it reports none.
func findingLines(stdout string) []string {
	list, _, _ := strings.Cut(stdout, "\n\n")
	if list == "" || strings.HasPrefix(list, "No debug statements found.") {
		return nil
	}
	return strings.Split(list, "\n")
}

func wantStatus(findings []string) int {
	if len(findings) > 0 {
		return exitFound
	}
	return exitClean
}

func TestRunStaged(t *testing.T) {
	dir := setup(t, map[string]string{"main.go": "package main\n", "web/index.php": "<?php\n"})
	repotest.Init(t, dir)
	commitAll(t, dir)
	repotest.WriteFiles(t, dir, map[string]string{
		"main.go":       "package main\n\nfmt.Println(1)\n",
		"web/index.php": "<?php\nvar_dump($x);\n",
		"vendor/lib.go": "fmt.Println(1)\n",
		"app.rb":        "binding.pry\n",
	})
	repotest.Git(t, dir, "add", ".")
	repotest.WriteFiles(t, dir, map[string]string{"untracked.go": "fmt.Println(1)\n", "web/untracked.php": "<?php var_dump(1);\n"})

	stdout, stderr, status := runCLI(t, "--staged")
	want := "main.go:3: fmt.Println(1)  [go/fmt-print]\n" + webFinding + "\n\n2 debug statements in 2 files\n" + stagedHint + "\n"
	if stdout != want || stderr != "" || status != exitFound {
		t.Fatalf("status %d\nstdout %q\nstderr %q\nwant   %q", status, stdout, stderr, want)
	}

	t.Chdir(filepath.Join(dir, "web"))
	for _, c := range []struct {
		args []string
		want []string
	}{
		{nil, []string{"../main.go:3: fmt.Println(1)  [go/fmt-print]", "index.php:2: var_dump($x);  [php/var-dump]"}},
		{[]string{"--lang", "ruby"}, []string{"../app.rb:1: binding.pry  [ruby/pry]"}},
		{[]string{"-e", "*.php"}, []string{"../main.go:3: fmt.Println(1)  [go/fmt-print]"}},
		{[]string{"-e", "./index.php"}, []string{"../main.go:3: fmt.Println(1)  [go/fmt-print]"}},
		{[]string{"-e", "../main.go", "-e", "index.php"}, nil},
	} {
		t.Run(strings.Join(append([]string{"--staged"}, c.args...), " "), func(t *testing.T) {
			stdout, stderr, status := runCLI(t, append([]string{"--staged"}, c.args...)...)
			if got := findingLines(stdout); !slices.Equal(got, c.want) || stderr != "" || status != wantStatus(c.want) {
				t.Fatalf("status %d, stderr %q\n got  %q\n want %q", status, stderr, got, c.want)
			}
		})
	}
}

func TestRunStagedWithNothingStaged(t *testing.T) {
	dir := setup(t, testTree)
	repotest.Init(t, dir)
	commitAll(t, dir)
	stdout, stderr, status := runCLI(t, "--staged")
	if stdout != "No debug statements found.\n" || stderr != "" || status != exitClean {
		t.Fatalf("status %d\nstdout %q\nstderr %q", status, stdout, stderr)
	}
}

func TestRunStagedErrors(t *testing.T) {
	setup(t, testTree)
	stdout, stderr, status := runCLI(t, "--staged")
	if status != exitError || stdout != "" || !strings.Contains(stderr, "litterbug: --staged: ") || !strings.Contains(stderr, "not a git repository") {
		t.Errorf("outside a repository: status %d, stdout %q, stderr %q", status, stdout, stderr)
	}
	stdout, stderr, status = runCLI(t, "--staged", "main.go")
	if status != exitError || stdout != "" || !strings.Contains(stderr, "takes no paths") {
		t.Errorf("with a path: status %d, stdout %q, stderr %q", status, stdout, stderr)
	}
}

// commitAll commits everything in the repository at dir.
func commitAll(t *testing.T, dir string) {
	t.Helper()
	repotest.Git(t, dir, "add", ".")
	repotest.Git(t, dir, "commit", "-qm", "commit")
}

func TestRunCount(t *testing.T) {
	dir := setup(t, testTree)
	for _, c := range []struct {
		args   []string
		stdout string
		status int
	}{
		{[]string{"--count"}, "2 debug statements in 2 files\n", exitFound},
		{[]string{"-c", "main.go"}, "1 debug statement in 1 file\n", exitFound},
		{[]string{"-c", "--lang", "ruby,php"}, "2 debug statements in 2 files\n", exitFound},
		{[]string{"-c", "clean.go"}, "No debug statements found.\n", exitClean},
	} {
		t.Run(strings.Join(c.args, " "), func(t *testing.T) {
			stdout, stderr, status := runCLI(t, c.args...)
			if stdout != c.stdout || stderr != "" || status != c.status {
				t.Errorf("status %d, stdout %q, stderr %q; want status %d, stdout %q", status, stdout, stderr, c.status, c.stdout)
			}
		})
	}
	if stdout, stderr, status := runCLI(t, "--count", "missing"); status != exitError || stdout != "" || !strings.Contains(stderr, "missing") {
		t.Errorf("an error: status %d, stdout %q, stderr %q", status, stdout, stderr)
	}

	// With --staged, the count leaves out the hint about committing.
	repotest.Init(t, dir)
	repotest.Git(t, dir, "add", "main.go", "app.rb")
	if stdout, _, status := runCLI(t, "--staged", "--count", "--lang", "go,ruby"); stdout != "2 debug statements in 2 files\n" || status != exitFound {
		t.Errorf("--staged --count: status %d, stdout %q", status, stdout)
	}
}
