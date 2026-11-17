package scan

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/nebarg/litterbug/internal/config"
	"github.com/nebarg/litterbug/internal/walk"
)

// Staged finds the debug statements on the lines added in the staged
// changes of the work tree whose top level is root, with paths from root.
// Files exclude skips, with its paths from root, are left out. Git tells
// it which index to read, so it sees what a commit will even for git
// commit -a, which stages into an index of its own.
func Staged(ctx context.Context, root string, exclude walk.Exclude, rules config.Rules) ([]Finding, error) {
	diff, err := git(ctx, root, "-c", "core.quotePath=false", "diff", "--cached",
		"--no-color", "--no-ext-diff", "--no-textconv", "--unified=0", "--diff-filter=ACMR", "--src-prefix=a/", "--dst-prefix=b/")
	if err != nil {
		return nil, err
	}
	added, err := addedLines(diff)
	if err != nil {
		return nil, err
	}
	var findings []Finding
	var path string
	var fileRules []config.Rule
	for _, a := range added {
		if a.path != path {
			path, fileRules = a.path, nil
			if !exclude.SkipsPath(a.path) {
				fileRules = rules.For(a.path)
			}
		}
		if f, ok := matchLine(fileRules, a.text); ok {
			f.Path, f.Line = a.path, a.line
			findings = append(findings, f)
		}
	}
	SortFindings(findings)
	return findings, nil
}

// WorkTreeRoot is the top level of the Git work tree dir is in, with
// symbolic links resolved.
func WorkTreeRoot(ctx context.Context, dir string) (string, error) {
	root, err := git(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(root, "\n"), nil
}

// git runs a Git command in dir and returns what it prints. Its error
// carries what Git printed on failing, such as "not a git repository".
func git(ctx context.Context, dir string, args ...string) (string, error) {
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
