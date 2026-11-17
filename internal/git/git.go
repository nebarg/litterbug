// Package git runs the Git commands litterbug needs.
package git

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// Run runs a Git command in dir and returns what it prints. Its error
// carries what Git printed on failing, such as "not a git repository".
func Run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if message := strings.TrimSpace(stderr.String()); message != "" {
			return "", errors.New(message)
		}
		return "", fmt.Errorf("git: %w", err)
	}
	return string(out), nil
}

// WorkTreeRoot is the top level of the work tree dir is in, with symbolic
// links resolved.
func WorkTreeRoot(ctx context.Context, dir string) (string, error) {
	root, err := Run(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(root, "\n"), nil
}

// HooksDir is the directory Git runs the hooks of dir's repository from:
// core.hooksPath if it is set, and the hooks directory every work tree of
// the repository shares otherwise. It may not exist yet.
func HooksDir(ctx context.Context, dir string) (string, error) {
	hooks, err := Run(ctx, dir, "rev-parse", "--git-path", "hooks")
	if err != nil {
		return "", err
	}
	hooks = strings.TrimSuffix(hooks, "\n")
	if !filepath.IsAbs(hooks) {
		hooks = filepath.Join(dir, hooks)
	}
	return hooks, nil
}
