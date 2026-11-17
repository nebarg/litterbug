package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/nebarg/litterbug/internal/config"
	"github.com/nebarg/litterbug/internal/walk"
	"github.com/spf13/pflag"
)

const initConfigUsage = `Usage:
  litterbug init-config [--global] [--force]

Writes litterbug's default config to .litterbug.yml at the root of this
repository, or with --global, to the global config, for you to edit.

Only one config is used: a repository's if it has one, the global config
if not, and the built-in defaults without either. They are never
combined, so a repository's config, once committed, gives everyone the
same checks.

An existing config is left alone unless --force is given, which keeps it
as a .bak file beside the new one.

Flags:
`

// initConfig runs litterbug init-config with the arguments after it.
func initConfig(args []string, stdout, stderr io.Writer) int {
	flags := pflag.NewFlagSet("litterbug init-config", pflag.ContinueOnError)
	global := flags.Bool("global", false, "write the global config, for repositories without their own")
	force := flags.Bool("force", false, "replace an existing config, keeping it as a .bak file")
	flags.Usage = func() { _, _ = fmt.Fprint(stdout, initConfigUsage+flags.FlagUsages()) }
	if status, done := parseCommandArgs(flags, "init-config", args, stderr); done {
		return status
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fail(stderr, err)
	}
	path, err := configPath(cwd, *global)
	if err != nil {
		return fail(stderr, fmt.Errorf("init-config: %w", err))
	}
	backup, err := config.WriteDefault(path, *force)
	if errors.Is(err, config.ErrExists) {
		command := "litterbug init-config"
		if *global {
			command += " --global"
		}
		return fail(stderr, fmt.Errorf("%w, at %s.\nEdit it, or run %s --force to replace it with the defaults", err, shownPath(cwd, path), command))
	}
	if err != nil {
		return fail(stderr, err)
	}
	if backup != "" {
		_, _ = fmt.Fprintf(stdout, "Wrote the default config to %s, and kept the old one as %s.\n", shownPath(cwd, path), shownPath(cwd, backup))
	} else {
		_, _ = fmt.Fprintf(stdout, "Wrote the default config to %s.\n", shownPath(cwd, path))
	}
	return exitClean
}

// configPath is where init-config writes: the global config, or the
// RepoFile at the root of the repository cwd is in.
func configPath(cwd string, global bool) (string, error) {
	if global {
		return config.GlobalPath()
	}
	root := walk.RepoRoot(cwd)
	if root == "" {
		return "", errors.New("not in a repository; run litterbug init-config --global to write the global config")
	}
	return filepath.Join(root, config.RepoFile), nil
}
