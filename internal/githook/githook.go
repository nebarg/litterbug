// Package githook installs and removes the Git pre-commit hook that runs
// litterbug --staged.
package githook

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Name is the hook litterbug installs.
const Name = "pre-commit"

// marker starts the second line of a hook litterbug installed, so it only
// ever replaces or removes its own.
const marker = "# Installed by litterbug."

// ErrNotOurs is returned for a pre-commit hook litterbug didn't install,
// which it leaves alone.
var ErrNotOurs = errors.New("there is already a pre-commit hook litterbug didn't install")

// Install writes the pre-commit hook that runs the litterbug at exe on
// what is staged into hooksDir, which it creates if need be, and returns
// the hook's path. It replaces a hook it installed, so installing again
// updates exe, but returns ErrNotOurs for any other hook unless force.
func Install(hooksDir, exe string, force bool) (string, error) {
	path := filepath.Join(hooksDir, Name)
	exists, ours, err := existing(path)
	if err != nil {
		return path, err
	}
	if exists && !ours && !force {
		return path, ErrNotOurs
	}
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		return path, err
	}
	return path, writeExecutable(path, script(exe))
}

// Uninstall removes the pre-commit hook from hooksDir if litterbug
// installed it, and returns its path and whether there was one. It returns
// ErrNotOurs, leaving the hook, if litterbug didn't install it.
func Uninstall(hooksDir string) (path string, removed bool, err error) {
	path = filepath.Join(hooksDir, Name)
	exists, ours, err := existing(path)
	switch {
	case err != nil:
		return path, false, err
	case !exists:
		return path, false, nil
	case !ours:
		return path, false, ErrNotOurs
	}
	return path, true, os.Remove(path)
}

// script is the hook that runs the litterbug at exe on what is staged.
func script(exe string) string {
	return "#!/bin/sh\n" + marker + " Remove it with: litterbug uninstall-hook\nexec " + shellQuote(exe) + " --staged\n"
}

// shellQuote quotes s for sh, in single quotes, which keep everything
// literal but a single quote. Each of those ends the quoting, is escaped,
// and starts it again:
//
//	it's  ->  'it'\''s'
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// existing reports whether there is a hook at path, and whether litterbug
// installed it. A link to nothing is a hook litterbug didn't install.
func existing(path string) (exists, ours bool, err error) {
	if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
		return false, false, nil
	} else if err != nil {
		return false, false, err
	}
	content, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return true, false, nil
	}
	if err != nil {
		return true, false, err
	}
	_, rest, _ := strings.Cut(string(content), "\n")
	return true, strings.HasPrefix(rest, marker), nil
}

// writeExecutable writes content to a new file beside path and renames it
// over path, so a hook is never left half-written, and a link at path is
// replaced rather than the file it leads to. Rewriting a file in place
// would also keep its mode, which may not be executable.
func writeExecutable(path, content string) error {
	file, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	// After the rename there is nothing left to remove; before it, this
	// cleans up a failed write.
	defer func() { _ = os.Remove(file.Name()) }()
	if _, err := file.WriteString(content); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Chmod(file.Name(), 0755); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
