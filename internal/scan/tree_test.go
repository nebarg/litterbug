package scan

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nebarg/litterbug/internal/config"
	"github.com/nebarg/litterbug/internal/walk"
)

const testConfig = `
languages:
  go:
    extensions: [.go]
    patterns:
      fmt-print: {regex: '\bfmt\.Print'}
  php:
    extensions: [.php]
    patterns:
      var-dump: {regex: '\bvar_dump\s*\('}
`

func TestTree(t *testing.T) {
	rules := loadRules(t)
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"main.go":          "package main\n\nfunc main() {\n\tfmt.Println(1)\n}\n",
		"clean.go":         "package main\n",
		"web/index.php":    "<?php\n$x = 1;\nvar_dump($x);\nvar_dump ($x);\n",
		"notes.txt":        "fmt.Println(1)\n",
		"binary.go":        "\x00fmt.Println(1)\n",
		"vendor/lib.go":    "fmt.Println(1)\n",
		"web/gen/out.php":  "var_dump($x);\n",
		".cache/cached.go": "fmt.Println(1)\n",
	})
	keepGitConfigOut(t)
	exclude, err := walk.ParseExclude(dir, dir, []string{"vendor", "web/gen"})
	if err != nil {
		t.Fatal(err)
	}
	findings, err := Tree(t.Context(), dir, exclude, rules)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"main.go:4 go/fmt-print", "web/index.php:3 php/var-dump", "web/index.php:4 php/var-dump"}
	if got := describe(findings); !slices.Equal(got, want) {
		t.Errorf("findings:\n got  %v\n want %v", got, want)
	}
}

func TestTreeIsSortedAndDeterministic(t *testing.T) {
	rules := loadRules(t)
	dir := t.TempDir()
	files := map[string]string{}
	var want []string
	for i := range 40 {
		name := fmt.Sprintf("f%02d.go", i)
		files[name] = "x := 1\nfmt.Println(x)\nfmt.Print(x)\n"
		want = append(want, name+":2 go/fmt-print", name+":3 go/fmt-print")
	}
	writeFiles(t, dir, files)
	keepGitConfigOut(t)
	for range 5 {
		findings, err := Tree(t.Context(), dir, walk.Exclude{}, rules)
		if err != nil {
			t.Fatal(err)
		}
		if got := describe(findings); !slices.Equal(got, want) {
			t.Fatalf("findings:\n got  %v\n want %v", got, want)
		}
	}
}

func TestTreeStopsWhenCancelled(t *testing.T) {
	rules := loadRules(t)
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"main.go": "fmt.Println(1)\n"})
	keepGitConfigOut(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if findings, err := Tree(ctx, dir, walk.Exclude{}, rules); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled scan = %v, %v", describe(findings), err)
	}
}

func TestTreeReportsAnUnreadableDirectory(t *testing.T) {
	rules := loadRules(t)
	if _, err := Tree(t.Context(), filepath.Join(t.TempDir(), "missing"), walk.Exclude{}, rules); err == nil {
		t.Fatal("scanning a missing directory gave no error")
	}
}

func TestFile(t *testing.T) {
	rules := loadRules(t)
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"main.go":     "x := 1\nfmt.Println(x)\n",
		"notes.txt":   "fmt.Println(x)\n",
		"binary.go":   "\x00fmt.Println(x)\n",
		"big.go":      "fmt.Println(x)\n" + strings.Repeat("x", 2<<20) + "\n", // Over the size limit of a walk.
		"Upper.GO":    "fmt.Println(x)\n",
		"clean.php":   "<?php\n",
		"nested/a.go": "fmt.Println(x)\n",
	})
	if err := os.Symlink("main.go", filepath.Join(dir, "link.go")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	for path, want := range map[string][]string{
		"main.go":     {"main.go:2 go/fmt-print"},
		"Upper.GO":    {"Upper.GO:1 go/fmt-print"},
		"nested/a.go": {"nested/a.go:1 go/fmt-print"},
		"binary.go":   {"binary.go:1 go/fmt-print"},
		"big.go":      {"big.go:1 go/fmt-print"},
		"link.go":     {"link.go:2 go/fmt-print"},
		"notes.txt":   nil,
		"clean.php":   nil,
		"missing.txt": nil, // Without rules for it, it isn't read.
	} {
		t.Run(path, func(t *testing.T) {
			findings, err := File(filepath.Join(dir, filepath.FromSlash(path)), path, rules)
			if got := describe(findings); err != nil || !slices.Equal(got, want) {
				t.Errorf("findings = %v, %v; want %v", got, err, want)
			}
		})
	}
}

func TestFileReportsAFileItCannotRead(t *testing.T) {
	rules := loadRules(t)
	dir := t.TempDir()
	if _, err := File(filepath.Join(dir, "missing.go"), "missing.go", rules); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a missing file: %v", err)
	}
	locked := filepath.Join(dir, "locked.go")
	writeFiles(t, dir, map[string]string{"locked.go": "fmt.Println(x)\n"})
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile(locked); err == nil {
		t.Skip("file permissions aren't enforced for this user")
	}
	if _, err := File(locked, "locked.go", rules); !errors.Is(err, fs.ErrPermission) {
		t.Errorf("an unreadable file: %v", err)
	}
}

func TestSortFindings(t *testing.T) {
	findings := []Finding{{Path: "b.go", Line: 1}, {Path: "a.go", Line: 9}, {Path: "a.go", Line: 2}, {Path: "a/b.go", Line: 1}}
	SortFindings(findings)
	want := []Finding{{Path: "a.go", Line: 2}, {Path: "a.go", Line: 9}, {Path: "a/b.go", Line: 1}, {Path: "b.go", Line: 1}}
	if !slices.Equal(findings, want) {
		t.Fatalf("sorted = %+v", findings)
	}
}

// loadRules compiles testConfig as the only config.
func loadRules(t *testing.T) config.Rules {
	t.Helper()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	writeFiles(t, home, map[string]string{"litterbug/config.yml": testConfig})
	rules, err := config.Load("", nil)
	if err != nil {
		t.Fatal(err)
	}
	return rules
}

// describe lists findings as path:line language/name.
func describe(findings []Finding) []string {
	var lines []string
	for _, f := range findings {
		lines = append(lines, fmt.Sprintf("%s:%d %s/%s", f.Path, f.Line, f.Rule.Language, f.Rule.Name))
	}
	return lines
}

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

// keepGitConfigOut stops the user's global gitignore from hiding test
// files. It leaves XDG_CONFIG_HOME, which loadRules points at its config.
func keepGitConfigOut(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}
