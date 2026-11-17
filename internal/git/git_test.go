package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunReportsWhatGitPrints(t *testing.T) {
	keepGitConfigOut(t)
	if _, err := Run(t.Context(), t.TempDir(), "rev-parse", "--show-toplevel"); err == nil || !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("err = %v", err)
	}
	if out, err := Run(t.Context(), t.TempDir(), "--version"); err != nil || !strings.HasPrefix(out, "git version") {
		t.Errorf("git --version = %q, %v", out, err)
	}
}

func TestWorkTreeRoot(t *testing.T) {
	repo := newRepo(t)
	sub := filepath.Join(repo, "a", "b")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	want := resolved(t, repo)
	for _, dir := range []string{repo, sub} {
		if got, err := WorkTreeRoot(t.Context(), dir); err != nil || got != want {
			t.Errorf("WorkTreeRoot(%s) = %q, %v; want %q", dir, got, err, want)
		}
	}
	if _, err := WorkTreeRoot(t.Context(), t.TempDir()); err == nil || !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("outside a repository: %v", err)
	}
}

func TestHooksDir(t *testing.T) {
	repo := newRepo(t)
	sub := filepath.Join(repo, "a", "b")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	gitIn(t, repo, "commit", "-q", "--allow-empty", "-m", "init")
	elsewhere := t.TempDir()
	worktree := filepath.Join(t.TempDir(), "worktree")
	gitIn(t, repo, "worktree", "add", "-q", worktree)

	for _, c := range []struct {
		name, hooksPath, dir, want string
	}{
		{"the default", "", repo, filepath.Join(repo, ".git", "hooks")},
		{"from a subdirectory", "", sub, filepath.Join(repo, ".git", "hooks")},
		{"a linked work tree shares them", "", worktree, filepath.Join(repo, ".git", "hooks")},
		{"a relative core.hooksPath", "githooks", sub, filepath.Join(repo, "githooks")},
		{"an absolute core.hooksPath", elsewhere, repo, elsewhere},
	} {
		t.Run(c.name, func(t *testing.T) {
			if c.hooksPath != "" {
				gitIn(t, repo, "config", "core.hooksPath", c.hooksPath)
				t.Cleanup(func() { gitIn(t, repo, "config", "--unset", "core.hooksPath") })
			}
			got, err := HooksDir(t.Context(), c.dir)
			if err != nil || resolved(t, got) != resolved(t, c.want) {
				t.Errorf("HooksDir = %q, %v; want %q", got, err, c.want)
			}
		})
	}
	if _, err := HooksDir(t.Context(), t.TempDir()); err == nil {
		t.Error("outside a repository: no error")
	}
}

// resolved is path with symbolic links resolved, as far as it exists, so
// paths under macOS's /var and /private/var compare equal.
func resolved(t *testing.T, path string) string {
	t.Helper()
	dir, rest := path, ""
	for {
		if real, err := filepath.EvalSymlinks(dir); err == nil {
			return filepath.Join(real, rest)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return path
		}
		dir, rest = parent, filepath.Join(filepath.Base(dir), rest)
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

func keepGitConfigOut(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
}
