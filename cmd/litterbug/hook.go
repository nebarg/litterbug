package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nebarg/litterbug/internal/git"
	"github.com/nebarg/litterbug/internal/githook"
	"github.com/spf13/pflag"
)

const installUsage = `Usage:
  litterbug install-hook [--force]

Installs a Git pre-commit hook in this repository that runs this litterbug
with --staged, so a commit adding debug statements is stopped. Installing
again updates the hook, after litterbug has moved, say. Another tool's
pre-commit hook is left alone unless --force is given; to keep it, add
litterbug --staged to it instead.

Flags:
`

const uninstallUsage = `Usage:
  litterbug uninstall-hook

Removes the pre-commit hook litterbug installed. A pre-commit hook another
tool installed is left alone.
`

// installHook runs litterbug install-hook with the arguments after it.
func installHook(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := pflag.NewFlagSet("litterbug install-hook", pflag.ContinueOnError)
	force := flags.Bool("force", false, "replace a pre-commit hook litterbug didn't install")
	flags.Usage = func() { _, _ = fmt.Fprint(stdout, installUsage+flags.FlagUsages()) }
	if status, done := parseHookArgs(flags, "install-hook", args, stderr); done {
		return status
	}
	cwd, hooks, err := hooksDir(ctx)
	if err != nil {
		return fail(stderr, fmt.Errorf("install-hook: %w", err))
	}
	exe, err := os.Executable()
	if err != nil {
		return fail(stderr, err)
	}
	path, err := githook.Install(hooks, exe, *force)
	if errors.Is(err, githook.ErrNotOurs) {
		return fail(stderr, fmt.Errorf("%w, at %s.\nAdd litterbug --staged to it, or run litterbug install-hook --force to replace it", err, shownPath(cwd, path)))
	}
	if err != nil {
		return fail(stderr, err)
	}
	if strings.Contains(filepath.ToSlash(exe), "/go-build") {
		_, _ = fmt.Fprintf(stderr, "litterbug: warning: %s is a temporary build, as go run makes, so the hook will stop working when it is deleted. Install litterbug with go install, then run litterbug install-hook again.\n", exe)
	}
	_, _ = fmt.Fprintf(stdout, "Installed the pre-commit hook at %s, running %s --staged.\n", shownPath(cwd, path), exe)
	return exitClean
}

// uninstallHook runs litterbug uninstall-hook with the arguments after it.
func uninstallHook(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := pflag.NewFlagSet("litterbug uninstall-hook", pflag.ContinueOnError)
	flags.Usage = func() { _, _ = fmt.Fprint(stdout, uninstallUsage) }
	if status, done := parseHookArgs(flags, "uninstall-hook", args, stderr); done {
		return status
	}
	cwd, hooks, err := hooksDir(ctx)
	if err != nil {
		return fail(stderr, fmt.Errorf("uninstall-hook: %w", err))
	}
	path, removed, err := githook.Uninstall(hooks)
	switch {
	case errors.Is(err, githook.ErrNotOurs):
		return fail(stderr, fmt.Errorf("%w, at %s, so it was left alone", err, shownPath(cwd, path)))
	case err != nil:
		return fail(stderr, err)
	case removed:
		_, _ = fmt.Fprintf(stdout, "Removed the pre-commit hook at %s.\n", shownPath(cwd, path))
	default:
		_, _ = fmt.Fprintf(stdout, "No pre-commit hook to remove at %s.\n", shownPath(cwd, path))
	}
	return exitClean
}

// parseHookArgs reads a hook command's flags, which take no other
// arguments. It reports whether the command is done, and its status: after
// --help, or a mistake.
func parseHookArgs(flags *pflag.FlagSet, command string, args []string, stderr io.Writer) (status int, done bool) {
	err := flags.Parse(args)
	switch {
	case errors.Is(err, pflag.ErrHelp):
		return exitClean, true
	case err != nil:
		return fail(stderr, fmt.Errorf("%w\nRun litterbug %s --help for usage", err, command)), true
	case flags.NArg() > 0:
		return fail(stderr, fmt.Errorf("%s takes no arguments", command)), true
	}
	return 0, false
}

// hooksDir finds the current directory and the hooks directory of the
// repository it is in.
func hooksDir(ctx context.Context) (cwd, hooks string, err error) {
	if cwd, err = os.Getwd(); err != nil {
		return "", "", err
	}
	hooks, err = git.HooksDir(ctx, cwd)
	return cwd, hooks, err
}

// shownPath is path from cwd when it is below cwd, and in full otherwise.
func shownPath(cwd, path string) string {
	if rel, err := filepath.Rel(cwd, path); err == nil && filepath.IsLocal(rel) {
		return rel
	}
	return path
}
