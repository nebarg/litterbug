// Package config loads litterbug's config: the languages to scan, the
// patterns that find debug statements in them, and the directories to skip.
// The global config is created with the defaults the first time it is
// needed, and a repository's own config file overrides it.
package config

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// RepoFile is the name of a repository's config, at its root.
const RepoFile = ".litterbug.yml"

//go:embed default.yml
var defaultConfig []byte

// file is one config file as written. Its pointers, slices and maps are nil
// for a field the file leaves out, which inherits the value when files
// merge, so it can be told from a field set empty or false.
type file struct {
	Exclude   *[]string            `yaml:"exclude"`
	Languages map[string]*language `yaml:"languages"`
}

type language struct {
	Enabled    *bool               `yaml:"enabled"`
	Extensions *[]string           `yaml:"extensions"`
	Patterns   map[string]*pattern `yaml:"patterns"`
}

type pattern struct {
	Enabled *bool   `yaml:"enabled"`
	Regex   *string `yaml:"regex"`
}

// Load reads the global config, creating it with the defaults if it is
// missing, and merges in the RepoFile at repoRoot if there is one. An empty
// repoRoot is outside any repository. langs, from the command line, picks
// the languages to scan in place of those the config enables. Notices,
// such as the global config being created, are written to notices.
func Load(repoRoot string, langs []string, notices io.Writer) (Rules, error) {
	merged, err := loadGlobal(notices)
	if err != nil {
		return Rules{}, err
	}
	if repoRoot != "" {
		repo, found, err := readFile(filepath.Join(repoRoot, RepoFile))
		if err != nil {
			return Rules{}, err
		}
		if found {
			merged = merge(merged, repo)
		}
	}
	return compile(merged, langs)
}

// loadGlobal reads the global config. When it is missing, it is created
// with the defaults; when it can't be, the defaults are used anyway, so an
// unwritable home directory doesn't stop a commit.
func loadGlobal(notices io.Writer) (file, error) {
	path, err := globalPath()
	if err != nil {
		_, _ = fmt.Fprintf(notices, "litterbug: using the built-in config: %v\n", err)
		return parse("built-in config", defaultConfig)
	}
	f, found, err := readFile(path)
	if err != nil || found {
		return f, err
	}
	if err := writeDefault(path); err != nil {
		_, _ = fmt.Fprintf(notices, "litterbug: using the built-in config, as it could not be saved: %v\n", err)
		return parse("built-in config", defaultConfig)
	}
	_, _ = fmt.Fprintf(notices, "litterbug: created default config at %s\n", path)
	return parse(path, defaultConfig)
}

// globalPath is where the global config lives: litterbug/config.yml in
// $XDG_CONFIG_HOME, or in ~/.config without it.
func globalPath() (string, error) {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "litterbug", "config.yml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "litterbug", "config.yml"), nil
}

func writeDefault(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, defaultConfig, 0644)
}

// readFile reads and parses the config file at path, reporting whether
// there is one.
func readFile(path string) (file, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return file{}, false, nil
	}
	if err != nil {
		return file{}, false, err
	}
	f, err := parse(path, data)
	return f, true, err
}

// parse reads a config file's content, rejecting keys it doesn't know, so
// a typo such as "enable: false" isn't silently ignored. name says which
// file it is in errors.
func parse(name string, data []byte) (file, error) {
	var f file
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&f); err != nil && !errors.Is(err, io.EOF) {
		return file{}, fmt.Errorf("%s: %w", name, err)
	}
	if err := f.check(); err != nil {
		return file{}, fmt.Errorf("%s: %w", name, err)
	}
	return f, nil
}

// check finds the mistakes a file can be judged on alone, while its name
// can still be given: extensions without a dot, and regexes that are empty
// or don't compile. What a language or pattern lacks is only known once
// the files are merged.
func (f file) check() error {
	for _, name := range slices.Sorted(maps.Keys(f.Languages)) {
		l := f.Languages[name]
		if l == nil {
			continue
		}
		if l.Extensions != nil {
			for _, extension := range *l.Extensions {
				if len(extension) < 2 || extension[0] != '.' || strings.ContainsAny(extension, `/\`) {
					return fmt.Errorf("language %s: extension %q should be a file name ending, such as .go", name, extension)
				}
			}
		}
		for _, patternName := range slices.Sorted(maps.Keys(l.Patterns)) {
			p := l.Patterns[patternName]
			if p == nil || p.Regex == nil {
				continue
			}
			if *p.Regex == "" {
				return fmt.Errorf("pattern %s/%s: the regex is empty, so it would match every line", name, patternName)
			}
			if _, err := regexp.Compile(*p.Regex); err != nil {
				return fmt.Errorf("pattern %s/%s: %w", name, patternName, err)
			}
		}
	}
	return nil
}
