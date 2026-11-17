package walk

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nebarg/litterbug/internal/repotest"
)

func TestFiles(t *testing.T) {
	tree := map[string]string{
		"code.go":               "x\n",
		"notes.md":              "x\n",
		".hidden.go":            "x\n",
		"web/.eslintrc.js":      "x\n",
		"binary.dat":            "x\x00x",
		".cache/cached.go":      "x\n",
		"src/.venv/lib.go":      "x\n",
		"node_modules/dep.js":   "x\n",
		"web/node_modules/d.js": "x\n",
		"vendor/lib.go":         "x\n",
		"dist/out.js":           "x\n",
		"pkg/dist/out.js":       "x\n",
		"pkg/gen/gen.go":        "x\n",
		"odd[1]/x.go":           "x\n",
	}
	cases := []struct {
		name     string
		patterns []string
		want     []string
	}{
		{"hidden files, Markdown and binaries are listed, dot directories and excludes skipped",
			[]string{"node_modules", "vendor"},
			[]string{".hidden.go", "binary.dat", "code.go", "dist/out.js", "notes.md", "odd[1]/x.go", "pkg/dist/out.js", "pkg/gen/gen.go", "web/.eslintrc.js"}},
		{"names skip at any depth, paths only from the walked directory, and glob characters can be escaped",
			[]string{"gen", `odd\[1\]`, "pkg/dist"},
			[]string{".hidden.go", "binary.dat", "code.go", "dist/out.js", "node_modules/dep.js", "notes.md", "vendor/lib.go", "web/.eslintrc.js", "web/node_modules/d.js"}},
	}
	for _, setup := range setups {
		t.Run(setup.name, func(t *testing.T) {
			dir := t.TempDir()
			repotest.WriteFiles(t, dir, tree)
			repotest.IsolateGit(t)
			if setup.repo {
				repotest.Init(t, dir)
			}
			for _, c := range cases {
				if got := listFiles(t, dir, excludeIn(t, dir, c.patterns...)); !slices.Equal(got, c.want) {
					t.Errorf("%s:\n got  %v\n want %v", c.name, got, c.want)
				}
			}
		})
	}
}

func TestFilesSkipsExcludedFiles(t *testing.T) {
	tree := map[string]string{
		"Dumper.php":        "x\n",
		"lib/Dumper.php":    "x\n",
		"src/debug.php":     "x\n",
		"web/src/debug.php": "x\n",
		"src/keep.php":      "x\n",
		".hidden.php":       "x\n",
		"main_test.go":      "x\n",
		"pkg/a_test.go":     "x\n",
		"pkg/a.go":          "x\n",
		"web/app.min.js":    "x\n",
		"web/app.js":        "x\n",
		"dist/bundle.js":    "x\n",
		"dist/x/y.js":       "x\n",
		"src/dist/keep.js":  "x\n",
	}
	patterns := []string{"Dumper.php", "src/debug.php", "*_test.go", "*.min.js", "dist/**"}
	want := []string{".hidden.php", "pkg/a.go", "src/dist/keep.js", "src/keep.php", "web/app.js", "web/src/debug.php"}
	for _, setup := range setups {
		t.Run(setup.name, func(t *testing.T) {
			dir := t.TempDir()
			repotest.WriteFiles(t, dir, tree)
			repotest.IsolateGit(t)
			if setup.repo {
				repotest.Init(t, dir)
			}
			if got := listFiles(t, dir, excludeIn(t, dir, patterns...)); !slices.Equal(got, want) {
				t.Errorf("files = %v, want %v", got, want)
			}
		})
	}
}

// excludeIn parses patterns for a walk of dir, with paths from dir.
func excludeIn(t *testing.T, dir string, patterns ...string) Exclude {
	t.Helper()
	exclude, err := ParseExclude(dir, dir, patterns)
	if err != nil {
		t.Fatal(err)
	}
	return exclude
}

func TestFilesSkipsUnreadableDirectories(t *testing.T) {
	for _, setup := range setups {
		t.Run(setup.name, func(t *testing.T) {
			dir := t.TempDir()
			repotest.WriteFiles(t, dir, map[string]string{"ok/a.go": "x\n", "locked/b.go": "x\n"})
			locked := filepath.Join(dir, "locked")
			if err := os.Chmod(locked, 0); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(locked, 0755) }) // So TempDir can remove it.
			if _, err := os.ReadDir(locked); err == nil {
				t.Skip("directory permissions aren't enforced for this user")
			}
			repotest.IsolateGit(t)
			if setup.repo {
				repotest.Init(t, dir)
			}
			if got := listFiles(t, dir, Exclude{}); !slices.Equal(got, []string{"ok/a.go"}) {
				t.Fatalf("files = %v", got)
			}
			if err := Files(t.Context(), locked, Exclude{}, func(string) {}); err == nil {
				t.Fatal("walking an unreadable directory gave no error")
			}
		})
	}
}

func TestFilesSkipsSymlinks(t *testing.T) {
	dir := t.TempDir()
	repotest.WriteFiles(t, dir, map[string]string{"real/a.go": "x\n"})
	for link, target := range map[string]string{"file-link.go": "real/a.go", "dir-link": "real"} {
		if err := os.Symlink(target, filepath.Join(dir, link)); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
	}
	repotest.IsolateGit(t)
	if got := listFiles(t, dir, Exclude{}); !slices.Equal(got, []string{"real/a.go"}) {
		t.Fatalf("files = %v, want only the real file", got)
	}
}

func TestFilesWalksADirectoryItsRepositoryIgnores(t *testing.T) {
	repo := t.TempDir()
	repotest.WriteFiles(t, repo, map[string]string{".gitignore": "/*\n", "ignored/a.go": "x\n"})
	repotest.IsolateGit(t)
	repotest.Init(t, repo)
	if got := listFiles(t, filepath.Join(repo, "ignored"), Exclude{}); !slices.Equal(got, []string{"a.go"}) {
		t.Fatalf("files = %v", got)
	}
}

func TestFilesFollowsEachRepositorysGitignore(t *testing.T) {
	dir := t.TempDir()
	repotest.WriteFiles(t, dir, map[string]string{
		"loose.go":           "x\n",
		"app/.gitignore":     "/build/\n",
		"app/main.go":        "x\n",
		"app/build/out.go":   "x\n",
		"app/lib/.gitignore": "*.gen.go\n",
		"app/lib/lib.go":     "x\n",
		"app/lib/x.gen.go":   "x\n",
		"app/.cache/c.go":    "x\n",
	})
	repotest.IsolateGit(t)
	repotest.Init(t, filepath.Join(dir, "app"))
	repotest.Init(t, filepath.Join(dir, "app", "lib"))
	inApp := []string{".gitignore", "lib/.gitignore", "lib/lib.go", "main.go"}
	var fromAbove []string
	for _, path := range inApp {
		fromAbove = append(fromAbove, "app/"+path)
	}
	fromAbove = append(fromAbove, "loose.go")
	for _, c := range []struct {
		dir  string
		want []string
	}{{dir, fromAbove}, {filepath.Join(dir, "app"), inApp}} {
		if got := listFiles(t, c.dir, Exclude{}); !slices.Equal(got, c.want) {
			t.Errorf("walking %s = %v, want %v", c.dir, got, c.want)
		}
	}
}

func TestRepoRoot(t *testing.T) {
	dir := t.TempDir()
	repotest.WriteFiles(t, dir, map[string]string{
		"repo/.git/HEAD":   "ref: refs/heads/main\n",
		"repo/a/b/x.go":    "x\n",
		"linked/.git":      "gitdir: /elsewhere\n", // As in a linked work tree or a submodule.
		"linked/sub/y.go":  "x\n",
		"loose/z.go":       "x\n",
		"repo/nested/.git": "gitdir: /elsewhere\n",
	})
	for from, want := range map[string]string{
		"repo":        "repo",
		"repo/a/b":    "repo",
		"repo/nested": "repo/nested",
		"linked/sub":  "linked",
		"loose":       "",
	} {
		if want != "" {
			want = filepath.Join(dir, want)
		}
		if got := RepoRoot(filepath.Join(dir, from)); got != want {
			t.Errorf("RepoRoot(%s) = %q, want %q", from, got, want)
		}
	}
}

func TestFilesStopsWhenCancelled(t *testing.T) {
	for _, setup := range setups {
		t.Run(setup.name, func(t *testing.T) {
			dir := t.TempDir()
			repotest.WriteFiles(t, dir, map[string]string{"a.go": "x\n"})
			repotest.IsolateGit(t)
			if setup.repo {
				repotest.Init(t, dir)
			}
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			var got []string
			err := Files(ctx, dir, Exclude{}, func(rel string) { got = append(got, rel) })
			if !errors.Is(err, context.Canceled) || len(got) > 0 {
				t.Fatalf("cancelled walk = %v, %v", got, err)
			}
		})
	}
}

func TestReadTextChecksOnlyTheFirstBytesForBinary(t *testing.T) {
	line := "fmt.Println(x)\n"
	dir := t.TempDir()
	for _, c := range []struct {
		name, content string
		text          bool
	}{
		{"nul-first.go", "\x00" + line, false},
		{"nul-in-check.go", line + strings.Repeat("\n", binaryCheckLen-len(line)-1) + "\x00", false},
		{"nul-past-check.go", line + strings.Repeat("\n", binaryCheckLen-len(line)) + "\x00 later", true},
		{"short.go", line, true},
		{"empty.go", "", true},
		{"no-final-newline.go", "fmt.Println(x)", true},
	} {
		path := filepath.Join(dir, c.name)
		if err := os.WriteFile(path, []byte(c.content), 0644); err != nil {
			t.Fatal(err)
		}
		got, ok := ReadText(path)
		if ok != c.text || (ok && got != c.content) {
			t.Errorf("ReadText(%s) = %q, %v; want text %v", c.name, got, ok, c.text)
		}
	}
}

func TestReadTextStopsAtABinaryStart(t *testing.T) {
	binary := append(make([]byte, binaryCheckLen-1), 'x')
	if _, ok := readText(io.MultiReader(bytes.NewReader(binary), readFails{t}), 1<<30); ok {
		t.Fatal("a file starting with NUL bytes was read as text")
	}
	long := strings.Repeat("a", binaryCheckLen) + "\x00 later"
	if text, ok := readText(strings.NewReader(long), int64(len(long))); !ok || text != long {
		t.Fatalf("a NUL byte past the check made the file binary: %v", ok)
	}
}

// readFails fails the test when read.
type readFails struct{ t *testing.T }

func (r readFails) Read([]byte) (int, error) {
	r.t.Error("read past the binary check")
	return 0, io.EOF
}

func TestReadTextSkipsFilesOverTheSizeLimit(t *testing.T) {
	dir := t.TempDir()
	for name, size := range map[string]int{"limit.go": maxFileSize, "over.go": maxFileSize + 1} {
		if err := os.WriteFile(filepath.Join(dir, name), bytes.Repeat([]byte("\n"), size), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := ReadText(filepath.Join(dir, "limit.go")); !ok {
		t.Error("a file at the size limit was skipped")
	}
	if _, ok := ReadText(filepath.Join(dir, "over.go")); ok {
		t.Error("a file over the size limit was read")
	}
}

func TestReadTextSkipsWhatIsNotAReadableRegularFile(t *testing.T) {
	dir := t.TempDir()
	repotest.WriteFiles(t, dir, map[string]string{"real.go": "x\n", "locked.go": "x\n"})
	if err := os.Symlink("real.go", filepath.Join(dir, "link.go")); err != nil {
		t.Fatal(err)
	}
	locked := filepath.Join(dir, "locked.go")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	paths := []string{dir, filepath.Join(dir, "link.go"), filepath.Join(dir, "missing.go")}
	if _, err := os.ReadFile(locked); err != nil {
		paths = append(paths, locked) // File permissions are only enforced for some users.
	}
	for _, path := range paths {
		if text, ok := ReadText(path); ok {
			t.Errorf("ReadText(%s) = %q, want it skipped", path, text)
		}
	}
	if text, ok := ReadText(filepath.Join(dir, "real.go")); !ok || text != "x\n" {
		t.Errorf("ReadText(real.go) = %q, %v", text, ok)
	}
}

// listFiles lists what Files finds under dir, slash-separated and sorted.
func listFiles(t *testing.T, dir string, exclude Exclude) []string {
	t.Helper()
	var got []string
	if err := Files(t.Context(), dir, exclude, func(rel string) { got = append(got, filepath.ToSlash(rel)) }); err != nil {
		t.Fatal(err)
	}
	slices.Sort(got)
	return got
}

// setups run a walk test on a directory outside a repository, and on one
// made a repository.
var setups = []struct {
	name string
	repo bool
}{{"outside a repository", false}, {"in a repository", true}}
