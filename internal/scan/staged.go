package scan

import (
	"context"

	"github.com/nebarg/litterbug/internal/config"
	"github.com/nebarg/litterbug/internal/git"
	"github.com/nebarg/litterbug/internal/walk"
)

// Staged finds the debug statements on the lines added in the staged
// changes of the work tree whose top level is root, with paths from root.
// Files exclude skips, with its paths from root, are left out. Git tells
// it which index to read, so it sees what a commit will even for git
// commit -a, which stages into an index of its own.
func Staged(ctx context.Context, root string, exclude walk.Exclude, rules config.Rules) ([]Finding, error) {
	diff, err := git.Run(ctx, root, "-c", "core.quotePath=false", "diff", "--cached",
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
