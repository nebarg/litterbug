package scan

import (
	"cmp"
	"context"
	"path/filepath"
	"runtime"
	"slices"
	"sync"

	"github.com/nebarg/litterbug/internal/config"
	"github.com/nebarg/litterbug/internal/walk"
)

// Tree finds the debug statements under dir, in the files rules have
// patterns for, with paths relative to dir. Files are read in parallel
// while the walk is still finding them, and findings are sorted by path,
// then line. Files that aren't text, are over the size limit or can't be
// read are skipped. Cancelling ctx stops the scan.
func Tree(ctx context.Context, dir string, exclude walk.Exclude, rules config.Rules) ([]Finding, error) {
	type job struct {
		rel   string
		rules []config.Rule
	}
	jobs := make(chan job, 128)
	found := make(chan []Finding, 128)
	var wg sync.WaitGroup
	for range min(runtime.GOMAXPROCS(0), 8) {
		wg.Go(func() {
			for j := range jobs {
				if ctx.Err() != nil {
					return
				}
				if findings := findInFile(filepath.Join(dir, j.rel), filepath.ToSlash(j.rel), j.rules); len(findings) > 0 {
					found <- findings
				}
			}
		})
	}
	var walkErr error
	wg.Go(func() {
		defer close(jobs)
		walkErr = walk.Files(ctx, dir, exclude, func(rel string) {
			fileRules := rules.For(rel)
			if len(fileRules) == 0 {
				return
			}
			select {
			case jobs <- job{rel, fileRules}:
			case <-ctx.Done():
			}
		})
	})
	go func() { wg.Wait(); close(found) }()

	var findings []Finding
	for fileFindings := range found {
		findings = append(findings, fileFindings...)
	}
	if err := cmp.Or(ctx.Err(), walkErr); err != nil {
		return nil, err
	}
	SortFindings(findings)
	return findings, nil
}

// File finds the debug statements in the file at path, if rules have
// patterns for it, with path, slash-separated, as their Path. It skips the
// files Tree skips.
func File(path string, rules config.Rules) []Finding {
	fileRules := rules.For(path)
	if len(fileRules) == 0 {
		return nil
	}
	return findInFile(path, filepath.ToSlash(path), fileRules)
}

// findInFile finds the debug statements in the file at path, giving them
// display as their Path.
func findInFile(path, display string, rules []config.Rule) []Finding {
	content, ok := walk.ReadText(path)
	if !ok {
		return nil
	}
	return findInContent(display, content, rules)
}

// SortFindings orders findings by path, then line.
func SortFindings(findings []Finding) {
	slices.SortFunc(findings, func(a, b Finding) int {
		return cmp.Or(cmp.Compare(a.Path, b.Path), cmp.Compare(a.Line, b.Line))
	})
}
