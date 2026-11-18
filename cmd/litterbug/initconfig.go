package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/nebarg/litterbug/internal/config"
	"github.com/nebarg/litterbug/internal/walk"
	"github.com/spf13/pflag"
)

const initConfigUsage = `Usage:
  litterbug init-config [--global] [--force] [--lang languages]

Writes litterbug's default config to .litterbug.yml at the root of this
repository, or with --global, to the global config, for you to edit. It
has every built-in language, or with --lang, only those it names; run
litterbug add-language to list them, or to add more later.

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
	langs := flags.StringSliceP("lang", "l", nil, "only these built-in `languages` (comma-separated or repeated)")
	flags.Usage = func() { _, _ = fmt.Fprint(stdout, initConfigUsage+flags.FlagUsages()) }
	if status, done := parseCommandArgs(flags, "init-config", args, stderr); done {
		return status
	}
	if err := trimLangs(*langs); err != nil {
		return fail(stderr, err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fail(stderr, err)
	}
	path, err := configPath(cwd, *global, "init-config")
	if err != nil {
		return fail(stderr, fmt.Errorf("init-config: %w", err))
	}
	backup, err := config.WriteDefault(path, *langs, *force)
	if errors.Is(err, config.ErrExists) {
		command := "litterbug init-config"
		if *global {
			command += " --global"
		}
		hint := "Edit it, or run " + command + " --force to replace it with the defaults"
		if len(*langs) > 0 {
			hint = "Run litterbug add-language " + strings.Join(*langs, " ") + " to add to it, or " + command + " --force to replace it"
		}
		return fail(stderr, fmt.Errorf("%w, at %s.\n%s", err, shownPath(cwd, path), hint))
	}
	if err != nil {
		return fail(stderr, err)
	}
	wrote := "the default config"
	if len(*langs) > 0 {
		wrote = "a config for " + listNames(unique(*langs))
	}
	if backup != "" {
		_, _ = fmt.Fprintf(stdout, "Wrote %s to %s, and kept the old one as %s.\n", wrote, shownPath(cwd, path), shownPath(cwd, backup))
	} else {
		_, _ = fmt.Fprintf(stdout, "Wrote %s to %s.\n", wrote, shownPath(cwd, path))
	}
	return exitClean
}

// configPath is the config that command writes: the global config, or the
// RepoFile at the root of the repository cwd is in.
func configPath(cwd string, global bool, command string) (string, error) {
	if global {
		return config.GlobalPath()
	}
	root := walk.RepoRoot(cwd)
	if root == "" {
		return "", fmt.Errorf("not in a repository; run litterbug %s --global for the global config", command)
	}
	return filepath.Join(root, config.RepoFile), nil
}

// unique returns names without repeats, in the order they came.
func unique(names []string) []string {
	var seen []string
	for _, name := range names {
		if !slices.Contains(seen, name) {
			seen = append(seen, name)
		}
	}
	return seen
}

// listNames joins names as in a sentence: "a", "a and b", "a, b and c".
func listNames(names []string) string {
	if len(names) < 2 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}
