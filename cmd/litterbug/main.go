// Command litterbug finds debug statements left in source code, such as
// fmt.Println, console.log and var_dump, in the files and directories it is
// given.
package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"

	"github.com/nebarg/litterbug/internal/config"
	"github.com/nebarg/litterbug/internal/scan"
	"github.com/nebarg/litterbug/internal/walk"
	"github.com/spf13/pflag"
)

const usageText = `Usage:
  litterbug [flags] [path...]    find debug statements in files and directories (default: here)
  litterbug --staged [flags]     find them in the lines staged for commit, as the hook does

Directories are walked, skipping what Git ignores and what the config or
--exclude excludes. A file or directory named here is scanned even if
they would skip it. Exclude patterns are written as in a .gitignore, such
as '*_test.go' or 'dist/**'; quote them so the shell doesn't expand them.

Exits 0 when nothing is found, 1 when something is, and 2 on errors.

Flags:
`

// Exit statuses follow grep, so a hook or CI can tell findings from a
// broken run.
const (
	exitClean = 0
	exitFound = 1
	exitError = 2
)

type options struct {
	paths   []string // as given; none means the current directory
	staged  bool
	langs   []string
	exclude []string
	version bool
}

// stagedHint follows what --staged finds, which blocks a commit when the
// hook runs it.
const stagedHint = "Remove them, or commit anyway with git commit --no-verify."

// target is a file or directory to scan.
type target struct {
	given string // as the command line gave it, which starts each path reported
	path  string // absolute
	isDir bool
}

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

// run runs litterbug with the arguments after the program name, and
// returns its exit status.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	o, err := parseArgs(args, stdout)
	if errors.Is(err, pflag.ErrHelp) {
		return exitClean
	}
	if err != nil {
		return fail(stderr, err)
	}
	if o.version {
		_, _ = fmt.Fprintf(stdout, "litterbug %s\n", version())
		return exitClean
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fail(stderr, err)
	}
	scanner := scanPaths
	if o.staged {
		scanner = scanStaged
	}
	findings, err := scanner(ctx, cwd, o, stderr)
	if err != nil {
		return fail(stderr, err)
	}
	report(stdout, findings)
	if len(findings) == 0 {
		return exitClean
	}
	if o.staged {
		_, _ = fmt.Fprintln(stdout, stagedHint)
	}
	return exitFound
}

// scanPaths scans the files and directories o names, or cwd without any.
// Config notices are written to notices.
func scanPaths(ctx context.Context, cwd string, o options, notices io.Writer) ([]scan.Finding, error) {
	targets, err := resolveTargets(cwd, o.paths)
	if err != nil {
		return nil, err
	}
	root := repoRoot(cwd)
	rules, err := config.Load(root, o.langs, notices)
	if err != nil {
		return nil, err
	}
	return scanTargets(ctx, cwd, root, targets, rules, o.exclude)
}

// scanStaged scans the lines added in the staged changes of the work tree
// cwd is in, with paths from cwd.
func scanStaged(ctx context.Context, cwd string, o options, notices io.Writer) ([]scan.Finding, error) {
	if len(o.paths) > 0 {
		return nil, errors.New("--staged scans what is staged, so it takes no paths")
	}
	root, err := scan.WorkTreeRoot(ctx, cwd)
	if err != nil {
		return nil, fmt.Errorf("--staged: %w", err)
	}
	// Git resolves symbolic links in root, so cwd must be resolved too for
	// paths between them.
	here, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return nil, err
	}
	rules, err := config.Load(root, o.langs, notices)
	if err != nil {
		return nil, err
	}
	exclude, err := excludeFor(here, root, root, rules.Exclude, o.exclude)
	if err != nil {
		return nil, err
	}
	findings, err := scan.Staged(ctx, root, exclude, rules)
	if err != nil {
		return nil, err
	}
	for i := range findings {
		if rel, err := filepath.Rel(here, filepath.Join(root, filepath.FromSlash(findings[i].Path))); err == nil {
			findings[i].Path = filepath.ToSlash(rel)
		}
	}
	return findings, nil
}

// parseArgs reads the command line. --help prints the usage on out.
func parseArgs(argv []string, out io.Writer) (options, error) {
	var o options
	flags := pflag.NewFlagSet("litterbug", pflag.ContinueOnError)
	flags.BoolVar(&o.staged, "staged", false, "scan only the lines added in the changes staged for commit")
	flags.StringSliceVarP(&o.langs, "lang", "l", nil, "only these `languages`, even if the config disables them (comma-separated or repeated)")
	flags.StringArrayVarP(&o.exclude, "exclude", "e", nil, "also skip files and directories matching a `pattern`: a name at any depth, or with a slash, a path from here (repeatable)")
	flags.BoolVar(&o.version, "version", false, "print the version")
	flags.Usage = func() {
		_, _ = fmt.Fprint(out, usageText+flags.FlagUsages())
	}
	if err := flags.Parse(argv); err != nil {
		if errors.Is(err, pflag.ErrHelp) {
			return o, err
		}
		return o, fmt.Errorf("%w\nRun litterbug --help for usage", err)
	}
	for i, lang := range o.langs {
		o.langs[i] = strings.TrimSpace(lang)
		if o.langs[i] == "" {
			return o, errors.New("--lang needs a language name, such as go")
		}
	}
	o.paths = flags.Args()
	return o, nil
}

// resolveTargets finds the files and directories to scan, from paths
// relative to cwd, or cwd itself without any.
func resolveTargets(cwd string, paths []string) ([]target, error) {
	if len(paths) == 0 {
		return []target{{path: cwd, isDir: true}}, nil
	}
	targets := make([]target, 0, len(paths))
	for _, given := range paths {
		abs := given
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(cwd, abs)
		}
		info, err := os.Stat(abs)
		if err != nil {
			return nil, err
		}
		targets = append(targets, target{given: given, path: abs, isDir: info.IsDir()})
	}
	return targets, nil
}

// repoRoot is the root of the repository dir is in: the nearest directory
// at or above it with a .git, which is a file in a worktree or submodule.
// It is empty outside a repository.
func repoRoot(dir string) string {
	for {
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// scanTargets scans each target, and returns all they find sorted, with
// their paths as the command line gave them. A file named on the command
// line is read even if it is excluded or ignored; a directory is walked
// with the config's excludes, from root, and --exclude's, from cwd.
func scanTargets(ctx context.Context, cwd, root string, targets []target, rules config.Rules, flagExcludes []string) ([]scan.Finding, error) {
	var all []scan.Finding
	for _, t := range targets {
		findings, err := scanTarget(ctx, cwd, root, t, rules, flagExcludes)
		if err != nil {
			return nil, err
		}
		all = append(all, findings...)
	}
	scan.SortFindings(all)
	// Overlapping targets, such as "." and "main.go", find the same lines.
	return slices.CompactFunc(all, func(a, b scan.Finding) bool { return a.Path == b.Path && a.Line == b.Line }), nil
}

// scanTarget scans one target, giving its findings paths that start as
// the command line gave it.
func scanTarget(ctx context.Context, cwd, root string, t target, rules config.Rules, flagExcludes []string) ([]scan.Finding, error) {
	given := filepath.ToSlash(t.given)
	if !t.isDir {
		findings := scan.File(t.path, rules)
		for i := range findings {
			findings[i].Path = path.Clean(given)
		}
		return findings, nil
	}
	exclude, err := excludeFor(cwd, cmp.Or(root, cwd), t.path, rules.Exclude, flagExcludes)
	if err != nil {
		return nil, err
	}
	findings, err := scan.Tree(ctx, t.path, exclude, rules)
	if err != nil {
		return nil, err
	}
	for i := range findings {
		findings[i].Path = path.Join(given, findings[i].Path)
	}
	return findings, nil
}

// excludeFor combines the config's excludes, with paths from configBase,
// and --exclude's, with paths from cwd, for a walk of dir.
func excludeFor(cwd, configBase, dir string, configExcludes, flagExcludes []string) (walk.Exclude, error) {
	fromConfig, err := walk.ParseExclude(configBase, dir, configExcludes)
	if err != nil {
		return walk.Exclude{}, fmt.Errorf("config exclude: %w", err)
	}
	fromFlags, err := walk.ParseExclude(cwd, dir, flagExcludes)
	if err != nil {
		return walk.Exclude{}, fmt.Errorf("--exclude: %w", err)
	}
	return fromConfig.With(fromFlags), nil
}

// version is the module version recorded in the binary, such as v1.2.0, or
// "(devel)" when the build recorded none, as go run does.
func version() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(devel)"
}

// fail reports err and returns the exit status for an error.
func fail(stderr io.Writer, err error) int {
	_, _ = fmt.Fprintf(stderr, "litterbug: %v\n", err)
	return exitError
}
