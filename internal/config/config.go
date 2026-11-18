// Package config loads litterbug's config: the languages to scan, the
// patterns that find debug statements in them, and the files and
// directories to skip. Exactly one config applies: a repository's own, the
// user's global config without it, or the built-in defaults without either.
// They are never combined, so a repository's config gives everyone working
// in it the same checks, whatever their global config says.
package config

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"
)

// RepoFile is the name of a repository's config, at its root.
const RepoFile = ".litterbug.yml"

//go:embed default.yml
var defaultConfig []byte

// file is one config file as written.
type file struct {
	Exclude   []string            `yaml:"exclude"`
	Languages map[string]language `yaml:"languages"`
}

type language struct {
	Enabled    *bool              `yaml:"enabled"`
	Extensions []string           `yaml:"extensions"`
	Patterns   map[string]pattern `yaml:"patterns"`
}

type pattern struct {
	Enabled *bool  `yaml:"enabled"`
	Regex   string `yaml:"regex"`
	Unless  string `yaml:"unless"`
}

// Load reads the config that applies in the repository at repoRoot, or
// outside any repository when repoRoot is empty, and compiles the rules
// to scan with. langs, from the command line, picks the languages to scan
// in place of those the config enables.
func Load(repoRoot string, langs []string) (Rules, error) {
	name, data, err := find(repoRoot)
	if err != nil {
		return Rules{}, err
	}
	rules, err := parseRules(data, langs)
	if err != nil {
		return Rules{}, fmt.Errorf("%s: %w", name, err)
	}
	return rules, nil
}

// find returns the name and content of the config that applies in the
// repository at repoRoot: its RepoFile, or else the global config, or else
// the built-in defaults. Without a home directory, there is no global
// config.
func find(repoRoot string) (name string, data []byte, err error) {
	var paths []string
	if repoRoot != "" {
		paths = append(paths, filepath.Join(repoRoot, RepoFile))
	}
	if global, err := GlobalPath(); err == nil {
		paths = append(paths, global)
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err == nil {
			return path, data, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", nil, err
		}
	}
	return "built-in config", defaultConfig, nil
}

// GlobalPath is where the global config lives: litterbug/config.yml in
// $XDG_CONFIG_HOME, or in ~/.config without it.
func GlobalPath() (string, error) {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "litterbug", "config.yml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "litterbug", "config.yml"), nil
}

// ErrExists is returned by WriteDefault for a config that is already there.
var ErrExists = errors.New("there is already a config")

// WriteDefault writes the built-in defaults to path, as a config to edit,
// making its directory if need be. It returns ErrExists for a file already
// at path, unless force, when that file is first moved to path + ".bak",
// replacing any earlier backup, and the backup's path is returned.
func WriteDefault(path string, force bool) (backup string, err error) {
	if _, err := os.Lstat(path); err == nil {
		if !force {
			return "", ErrExists
		}
		backup = path + ".bak"
		if err := os.Rename(path, backup); err != nil {
			return "", err
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return backup, err
	}
	return backup, os.WriteFile(path, defaultConfig, 0644)
}

// parseRules reads a config file's content and compiles its rules.
func parseRules(data []byte, langs []string) (Rules, error) {
	f, err := parse(data)
	if err != nil {
		return Rules{}, err
	}
	return compile(f, langs)
}

// parse reads a config file's content, rejecting keys it doesn't know, so
// a typo such as "enable: false" isn't silently ignored. For the same
// reason, a config is one YAML document: any after it, past a "---", may
// only be empty.
func parse(data []byte) (file, error) {
	var f file
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&f); err != nil && !errors.Is(err, io.EOF) {
		return file{}, err
	}
	for {
		var rest any
		err := decoder.Decode(&rest)
		if errors.Is(err, io.EOF) {
			return f, nil
		}
		if err != nil {
			return file{}, err
		}
		if rest != nil {
			return file{}, errors.New("a config is one YAML document, but there is another after a ---")
		}
	}
}
