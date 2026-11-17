// Package walk finds the files to scan under a directory, skipping what Git
// ignores, and reads them as text.
//
// It is copied from todo-cli's internal/scan package at commit 9e9adc4,
// without its to-do matching, so a fix to one may apply to the other.
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
)

// Files calls add with each regular file under dir, as a path relative to
// dir. It skips the files and directories in exclude, directories starting
// with a dot, and whatever a repository's gitignores, .git/info/exclude or
// the user's global gitignore ignore. Hidden files are included. Cancelling
// ctx stops the walk.
func Files(ctx context.Context, dir string, exclude Exclude, add func(rel string)) error {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	w := walker{ctx: ctx, dir: dir, exclude: exclude, add: add}
	if content, err := os.ReadFile(globalIgnoreFile()); err == nil {
		w.global = parseIgnore(string(content), 0)
	}
	return w.walk(".", w.enclosingScope())
}

// walker finds the files under dir.
type walker struct {
	ctx     context.Context
	dir     string
	exclude Exclude
	global  []ignoreRule // the user's global gitignore, which applies in every repository
	add     func(path string)
}

// walk adds the files in rel, a directory, and below. scope is the
// directory's place in its repository, before its own entries are seen.
func (w *walker) walk(rel string, scope ignoreScope) error {
	if err := w.ctx.Err(); err != nil {
		return err
	}
	path := filepath.Join(w.dir, rel)
	entries, err := os.ReadDir(path)
	if err != nil {
		if rel == "." {
			return err
		}
		return nil // An unreadable directory is skipped, as unreadable files are.
	}
	if hasEntry(entries, ".git") {
		scope = w.repoScope(path)
	}
	if scope.inRepo && hasEntry(entries, ".gitignore") {
		scope.rules = withIgnoreFile(scope.rules, filepath.Join(path, ".gitignore"), len(scope.segments))
	}
	segments := slices.Concat(scope.segments, []string{""})
	for _, entry := range entries {
		name := entry.Name()
		segments[len(segments)-1] = name
		switch child := filepath.Join(rel, name); {
		case entry.IsDir():
			if w.exclude.skips(child, true) || scope.ignores(segments, true) {
				continue
			}
			sub := scope
			sub.segments = segments
			if err := w.walk(child, sub); err != nil {
				return err
			}
		// .git is a file in a linked work tree or a submodule, and Git's own.
		case entry.Type().IsRegular() && name != ".git" && !w.exclude.skips(child, false) && !scope.ignores(segments, false):
			w.add(child)
		}
	}
	return nil
}

// repoScope starts the scope of a repository at its root.
func (w *walker) repoScope(root string) ignoreScope {
	common := commonDir(root)
	return ignoreScope{
		inRepo:   true,
		foldCase: ignoresCase(common),
		rules:    withIgnoreFile(w.global, filepath.Join(common, "info", "exclude"), 0),
	}
}

// enclosingScope finds the repository the walked directory is in, with the
// rules of the gitignores above it. The directory itself is walked even if
// they ignore it.
func (w *walker) enclosingScope() ignoreScope {
	var segments []string // the walked directory's path from parent
	for child, parent := w.dir, filepath.Dir(w.dir); parent != child; child, parent = parent, filepath.Dir(parent) {
		segments = slices.Insert(segments, 0, filepath.Base(child))
		if _, err := os.Lstat(filepath.Join(parent, ".git")); err != nil {
			continue
		}
		scope := w.repoScope(parent)
		for depth := range segments {
			gitignore := filepath.Join(parent, filepath.Join(segments[:depth]...), ".gitignore")
			scope.rules = withIgnoreFile(scope.rules, gitignore, depth)
		}
		scope.segments = segments
		return scope
	}
	return ignoreScope{}
}

// hasEntry reports whether a directory's entries, sorted as os.ReadDir
// sorts them, include name.
func hasEntry(entries []os.DirEntry, name string) bool {
	_, found := slices.BinarySearchFunc(entries, name, func(e os.DirEntry, name string) int {
		return strings.Compare(e.Name(), name)
	})
	return found
}

// maxFileSize is the largest file ReadText reads. Larger files are data,
// bundles or generated code, which are read whole, so they are skipped.
const maxFileSize = 1 << 20

// ReadText reads the file at path. It reports false, without an error, for
// anything that isn't a regular text file of at most maxFileSize bytes, or
// can't be read, as a scan skips those.
func ReadText(path string) (string, bool) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxFileSize {
		return "", false
	}
	file, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer func() { _ = file.Close() }() // Read-only, so a close error cannot lose data.
	return readText(file, info.Size())
}

// binaryCheckLen is how much of a file is checked for a NUL byte, which
// marks it as binary.
const binaryCheckLen = 8192

// readText reads a file of about size bytes from r, unless its first
// binaryCheckLen bytes hold a NUL byte. A binary file is read no further.
func readText(r io.Reader, size int64) (string, bool) {
	head := make([]byte, binaryCheckLen)
	n, err := io.ReadFull(r, head)
	head = head[:n]
	if bytes.IndexByte(head, 0) >= 0 {
		return "", false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return string(head), true
	}
	if err != nil {
		return "", false
	}
	content := bytes.NewBuffer(make([]byte, 0, max(int(size), n)+bytes.MinRead))
	content.Write(head)
	if _, err := content.ReadFrom(r); err != nil {
		return "", false
	}
	return content.String(), true
}
