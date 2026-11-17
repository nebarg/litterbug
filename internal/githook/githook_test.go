package githook

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

const foreign = "#!/bin/sh\necho someone else's hook\n"

func TestInstallCreatesTheHook(t *testing.T) {
	hooks := filepath.Join(t.TempDir(), "missing", "hooks")
	path, err := Install(hooks, "/usr/local/bin/litterbug", false)
	if err != nil || path != filepath.Join(hooks, Name) {
		t.Fatalf("Install = %q, %v", path, err)
	}
	want := "#!/bin/sh\n# Installed by litterbug. Remove it with: litterbug uninstall-hook\nexec '/usr/local/bin/litterbug' --staged\n"
	checkHook(t, path, want)
	checkOnlyHookIn(t, hooks)
}

func TestInstallReplacesItsOwnHook(t *testing.T) {
	hooks := t.TempDir()
	if _, err := Install(hooks, "/old/litterbug", false); err != nil {
		t.Fatal(err)
	}
	path, err := Install(hooks, "/new/litterbug", false)
	if err != nil {
		t.Fatal(err)
	}
	checkHook(t, path, script("/new/litterbug"))
	checkOnlyHookIn(t, hooks)
}

func TestInstallLeavesAnotherHookUnlessForced(t *testing.T) {
	hooks := t.TempDir()
	path := filepath.Join(hooks, Name)
	writeFile(t, path, foreign, 0644)
	if _, err := Install(hooks, "/bin/litterbug", false); !errors.Is(err, ErrNotOurs) {
		t.Fatalf("Install = %v, want ErrNotOurs", err)
	}
	checkContent(t, path, foreign)
	// Forcing replaces it, and makes the hook executable though the file
	// it replaced wasn't.
	if _, err := Install(hooks, "/bin/litterbug", true); err != nil {
		t.Fatal(err)
	}
	checkHook(t, path, script("/bin/litterbug"))
}

func TestInstallReplacesALinkNotWhatItLeadsTo(t *testing.T) {
	hooks := t.TempDir()
	target := filepath.Join(t.TempDir(), "husky-hook")
	writeFile(t, target, foreign, 0755)
	path := filepath.Join(hooks, Name)
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(hooks, "/bin/litterbug", false); !errors.Is(err, ErrNotOurs) {
		t.Fatalf("Install over a link = %v, want ErrNotOurs", err)
	}
	if _, err := Install(hooks, "/bin/litterbug", true); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(path); err != nil || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("the hook is still a link: %v", err)
	}
	checkHook(t, path, script("/bin/litterbug"))
	checkContent(t, target, foreign)
}

func TestInstallTreatsALinkToNothingAsAnotherHook(t *testing.T) {
	hooks := t.TempDir()
	if err := os.Symlink(filepath.Join(hooks, "nowhere"), filepath.Join(hooks, Name)); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(hooks, "/bin/litterbug", false); !errors.Is(err, ErrNotOurs) {
		t.Fatalf("Install = %v, want ErrNotOurs", err)
	}
	if _, _, err := Uninstall(hooks); !errors.Is(err, ErrNotOurs) {
		t.Fatalf("Uninstall = %v, want ErrNotOurs", err)
	}
}

func TestUninstall(t *testing.T) {
	hooks := t.TempDir()
	path, removed, err := Uninstall(hooks)
	if err != nil || removed || path != filepath.Join(hooks, Name) {
		t.Fatalf("without a hook: %q, %v, %v", path, removed, err)
	}

	if _, err := Install(hooks, "/bin/litterbug", false); err != nil {
		t.Fatal(err)
	}
	if _, removed, err := Uninstall(hooks); err != nil || !removed {
		t.Fatalf("its own hook: %v, %v", removed, err)
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the hook is still there: %v", err)
	}

	writeFile(t, path, foreign, 0755)
	if _, removed, err := Uninstall(hooks); !errors.Is(err, ErrNotOurs) || removed {
		t.Fatalf("another hook: %v, %v", removed, err)
	}
	checkContent(t, path, foreign)
}

// The hook runs litterbug at its path, however awkward, with --staged.
func TestTheHookRunsExeWithStaged(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "it's a $dir")
	args := filepath.Join(t.TempDir(), "args")
	exe := filepath.Join(dir, "fake litterbug")
	writeFile(t, exe, "#!/bin/sh\nprintf '%s\\n' \"$@\" > '"+args+"'\n", 0755)
	path, err := Install(t.TempDir(), exe, false)
	if err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(path).CombinedOutput(); err != nil {
		t.Fatalf("running the hook: %v %s", err, out)
	}
	checkContent(t, args, "--staged\n")
}

func TestShellQuote(t *testing.T) {
	for _, s := range []string{"/usr/bin/litterbug", "/a b/c", "/it's/here", `/$HOME/"x"/\n`, "''"} {
		out, err := exec.Command("sh", "-c", "printf %s "+shellQuote(s)).Output()
		if err != nil || string(out) != s {
			t.Errorf("sh read %s back as %q, %v", shellQuote(s), out, err)
		}
	}
}

func checkHook(t *testing.T, path, want string) {
	t.Helper()
	checkContent(t, path, want)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0755 {
		t.Errorf("hook mode = %v, want 0755", info.Mode().Perm())
	}
}

func checkContent(t *testing.T, path, want string) {
	t.Helper()
	if content, err := os.ReadFile(path); err != nil || string(content) != want {
		t.Errorf("%s = %q, %v; want %q", path, content, err, want)
	}
}

// checkOnlyHookIn checks that writing the hook left nothing else behind.
func checkOnlyHookIn(t *testing.T, hooks string) {
	t.Helper()
	entries, err := os.ReadDir(hooks)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if !slices.Equal(names, []string{Name}) {
		t.Errorf("hooks directory holds %v", names)
	}
}

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}
