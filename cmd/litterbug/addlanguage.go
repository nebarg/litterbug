package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/nebarg/litterbug/internal/config"
	"github.com/spf13/pflag"
)

const addLanguageUsage = `Usage:
  litterbug add-language [--global] [language...]

Adds the patterns litterbug has built in for each language, separated by
spaces or commas, to .litterbug.yml at the root of this repository, or
with --global, to the global config. They go after the languages the
config has, and the rest of it is kept as it is. Without a language,
lists the built-in ones.

Flags:
`

// addLanguage runs litterbug add-language with the arguments after it.
func addLanguage(args []string, stdout, stderr io.Writer) int {
	flags := pflag.NewFlagSet("litterbug add-language", pflag.ContinueOnError)
	global := flags.Bool("global", false, "add to the global config, for repositories without their own")
	flags.Usage = func() { _, _ = fmt.Fprint(stdout, addLanguageUsage+flags.FlagUsages()) }
	err := flags.Parse(args)
	if errors.Is(err, pflag.ErrHelp) {
		return exitClean
	}
	if err != nil {
		return fail(stderr, fmt.Errorf("%w\nRun litterbug add-language --help for usage", err))
	}
	langs, err := languageArgs(flags.Args())
	if err != nil {
		return fail(stderr, err)
	}
	if len(langs) == 0 {
		listPresets(stdout)
		return exitClean
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fail(stderr, err)
	}
	path, err := configPath(cwd, *global, "add-language")
	if err != nil {
		return fail(stderr, fmt.Errorf("add-language: %w", err))
	}
	// Errors name the config as shown, which is also a path to it from cwd.
	err = config.AddLanguages(shownPath(cwd, path), langs)
	if errors.Is(err, fs.ErrNotExist) {
		command := "litterbug init-config"
		if *global {
			command += " --global"
		}
		return fail(stderr, fmt.Errorf("there is no config at %s to add to.\nRun %s --lang %s to write one with only %s",
			shownPath(cwd, path), command, strings.Join(langs, ","), listNames(langs)))
	}
	if err != nil {
		return fail(stderr, err)
	}
	_, _ = fmt.Fprintf(stdout, "Added %s to %s.\n", listNames(langs), shownPath(cwd, path))
	return exitClean
}

// languageArgs reads the language names add-language is given, which can
// be separated by commas as well as spaces, as --lang takes them, dropping
// repeats.
func languageArgs(args []string) ([]string, error) {
	var langs []string
	for _, arg := range args {
		for name := range strings.SplitSeq(arg, ",") {
			name = strings.TrimSpace(name)
			if name == "" {
				return nil, fmt.Errorf("%q has an empty language name", arg)
			}
			langs = append(langs, name)
		}
	}
	return unique(langs), nil
}

// listPresets prints the built-in languages and the file name endings each
// covers.
func listPresets(stdout io.Writer) {
	presets := config.Presets()
	width := 0
	for _, p := range presets {
		width = max(width, len(p.Name))
	}
	for _, p := range presets {
		_, _ = fmt.Fprintf(stdout, "%-*s  %s\n", width, p.Name, strings.Join(p.Extensions, " "))
	}
}
