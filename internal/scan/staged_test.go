package scan

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nebarg/litterbug/internal/walk"
)

func TestStaged(t *testing.T) {
	rules := loadRules(t)
	repo := newRepo(t)
	writeFiles(t, repo, map[string]string{
		"mod.go":       numberedLines(10),
		"tomove.go":    numberedLines(30),
		"gone.go":      "fmt.Println(1)\n",
		"old_debug.go": "fmt.Println(\"already committed\")\n",
	})
	gitIn(t, repo, "add", ".")
	gitIn(t, repo, "commit", "-qm", "base")

	// mod.go gains a line after line 3 and one at the end, and loses line 8.
	lines := strings.Split(strings.TrimSuffix(numberedLines(10), "\n"), "\n")
	mod := slices.Concat(lines[:3], []string{`fmt.Println("inserted")`}, lines[3:7], lines[8:], []string{`fmt.Println("end")`})
	writeFiles(t, repo, map[string]string{
		"mod.go":            strings.Join(mod, "\n") + "\n",
		"new.php":           "<?php\nvar_dump($x);\n",
		"with space.go":     "fmt.Println(1)\n",
		"quo\"te.go":        "fmt.Println(1)\n",
		"crlf.go":           "x := 1\r\nfmt.Println(x)\r\n",
		"binary.go":         "\x00fmt.Println(1)\n",
		"notes.txt":         "fmt.Println(1)\n",
		"vendor/dep.go":     "fmt.Println(1)\n",
		".github/script.go": "fmt.Println(1)\n",
		"old_debug.go":      "fmt.Println(\"already committed\")\nx := 2\n",
	})
	gitIn(t, repo, "mv", "tomove.go", "moved.go")
	writeFiles(t, repo, map[string]string{"moved.go": numberedLines(30) + "fmt.Println(\"moved\")\n"})
	gitIn(t, repo, "rm", "-q", "gone.go")
	gitIn(t, repo, "add", ".")
	// Changes made after staging aren't committed, so aren't reported.
	writeFiles(t, repo, map[string]string{"mod.go": strings.Join(mod, "\n") + "\nfmt.Println(\"unstaged\")\n", "untracked.go": "fmt.Println(1)\n"})

	exclude, err := walk.ParseExclude(repo, repo, []string{"vendor"})
	if err != nil {
		t.Fatal(err)
	}
	findings, err := Staged(t.Context(), repo, exclude, rules)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"crlf.go:2 go/fmt-print",
		"mod.go:4 go/fmt-print",
		"mod.go:11 go/fmt-print",
		"moved.go:31 go/fmt-print",
		"new.php:2 php/var-dump",
		"quo\"te.go:1 go/fmt-print",
		"with space.go:1 go/fmt-print",
	}
	if got := describe(findings); !slices.Equal(got, want) {
		t.Errorf("findings:\n got  %q\n want %q", got, want)
	}
	for _, f := range findings {
		if f.Path == "crlf.go" && f.Text != "fmt.Println(x)" {
			t.Errorf("crlf.go text = %q", f.Text)
		}
	}
}

func TestStagedOnAFirstCommit(t *testing.T) {
	rules := loadRules(t)
	repo := newRepo(t)
	writeFiles(t, repo, map[string]string{"main.go": "package main\n\nfmt.Println(1)\n"})
	gitIn(t, repo, "add", ".")
	findings, err := Staged(t.Context(), repo, walk.Exclude{}, rules)
	if got, want := describe(findings), []string{"main.go:3 go/fmt-print"}; err != nil || !slices.Equal(got, want) {
		t.Fatalf("findings = %q, %v; want %q", got, err, want)
	}
}

func TestStagedReadsTheIndexGitNames(t *testing.T) {
	rules := loadRules(t)
	repo := newRepo(t)
	writeFiles(t, repo, map[string]string{"main.go": "package main\n"})
	gitIn(t, repo, "add", ".")
	gitIn(t, repo, "commit", "-qm", "base")
	// git commit -a stages into an index of its own and names it in
	// GIT_INDEX_FILE for its hooks.
	index := filepath.Join(t.TempDir(), "index")
	data, err := os.ReadFile(filepath.Join(repo, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	writeFiles(t, filepath.Dir(index), map[string]string{"index": string(data)})
	writeFiles(t, repo, map[string]string{"main.go": "package main\n\nfmt.Println(1)\n"})
	add := exec.Command("git", "-C", repo, "add", "main.go")
	add.Env = append(os.Environ(), "GIT_INDEX_FILE="+index)
	if out, err := add.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v %s", err, out)
	}

	if findings, err := Staged(t.Context(), repo, walk.Exclude{}, rules); err != nil || len(findings) > 0 {
		t.Fatalf("with the repository's index: %q, %v", describe(findings), err)
	}
	t.Setenv("GIT_INDEX_FILE", index)
	findings, err := Staged(t.Context(), repo, walk.Exclude{}, rules)
	if got, want := describe(findings), []string{"main.go:3 go/fmt-print"}; err != nil || !slices.Equal(got, want) {
		t.Fatalf("with GIT_INDEX_FILE: %q, %v; want %q", got, err, want)
	}
}

func TestStagedOverridesDiffSettings(t *testing.T) {
	rules := loadRules(t)
	repo := newRepo(t)
	for _, setting := range [][2]string{
		{"diff.noprefix", "true"}, {"diff.mnemonicPrefix", "true"}, {"color.ui", "always"}, {"core.quotePath", "true"},
		{"diff.relative", "true"}, {"diff.external", "false"}, {"diff.renames", "copies"},
	} {
		gitIn(t, repo, "config", setting[0], setting[1])
	}
	writeFiles(t, repo, map[string]string{"ünï/main.go": "fmt.Println(1)\n"})
	gitIn(t, repo, "add", ".")
	findings, err := Staged(t.Context(), repo, walk.Exclude{}, rules)
	if got, want := describe(findings), []string{"ünï/main.go:1 go/fmt-print"}; err != nil || !slices.Equal(got, want) {
		t.Fatalf("findings = %q, %v; want %q", got, err, want)
	}
}

// Outside a repository, git diff falls back to comparing files and
// rejects --cached; WorkTreeRoot gives the clearer error.
func TestStagedOutsideARepository(t *testing.T) {
	rules := loadRules(t)
	if findings, err := Staged(t.Context(), t.TempDir(), walk.Exclude{}, rules); err == nil {
		t.Fatalf("findings = %q, no error", describe(findings))
	}
}

func TestWorkTreeRoot(t *testing.T) {
	repo := newRepo(t)
	sub := filepath.Join(repo, "a", "b")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{repo, sub} {
		if got, err := WorkTreeRoot(t.Context(), dir); err != nil || got != want {
			t.Errorf("WorkTreeRoot(%s) = %q, %v; want %q", dir, got, err, want)
		}
	}
	if _, err := WorkTreeRoot(t.Context(), t.TempDir()); err == nil || !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("outside a repository: %v", err)
	}
}

// newRepo makes a repository that commits as a test user, with the
// user's own Git configuration kept out.
func newRepo(t *testing.T) string {
	t.Helper()
	keepGitConfigOut(t)
	for _, v := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(v, "Test")
	}
	for _, v := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(v, "test@example.com")
	}
	repo := t.TempDir()
	gitIn(t, repo, "init", "-q")
	return repo
}

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
}

// numberedLines is "line 1" to "line n", one a line.
func numberedLines(n int) string {
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, "line %d\n", i+1)
	}
	return b.String()
}
