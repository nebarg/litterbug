// Package repotest helps tests that need files and Git repositories.
package repotest

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// IsolateGit keeps the user's Git configuration out of the test, so it
// sees only what it sets up: their config files, and the global gitignore
// and attributes Git finds through XDG_CONFIG_HOME, which it points at a
// new empty directory. A test that keeps a config of its own there sets
// XDG_CONFIG_HOME after calling it. Commits are made as a test user.
func IsolateGit(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, v := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(v, "Test")
	}
	for _, v := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(v, "test@example.com")
	}
}

// NewRepo keeps the user's Git configuration out of the test, as
// IsolateGit does, and makes a repository in a new directory, which it
// returns.
func NewRepo(t *testing.T) string {
	t.Helper()
	IsolateGit(t)
	dir := t.TempDir()
	Init(t, dir)
	return dir
}

// Init makes dir, which must exist, a repository.
func Init(t *testing.T, dir string) {
	t.Helper()
	Git(t, dir, "init", "-q")
}

// Git runs git in dir and returns what it prints, failing the test, with
// what Git printed on failing, if it fails.
func Git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, stderr.Bytes())
	}
	return string(out)
}

// WriteFiles writes files, by their slash-separated paths from dir, making
// the directories they are in.
func WriteFiles(t *testing.T, dir string, files map[string]string) {
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
