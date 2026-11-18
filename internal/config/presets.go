package config

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// header starts the default config: its comments, its exclude list, and
// the languages key that the built-in languages go under.
//
//go:embed header.yml
var header string

// presetFiles are the built-in languages, each in a file named for it that
// maps its name to its config, as it would be under languages.
//
//go:embed languages/*.yml
var presetFiles embed.FS

// preset is a built-in language.
type preset struct {
	name   string
	text   string // as in its file, so its comments are kept
	config language
}

// presets are the built-in languages, sorted by name, and defaultConfig
// is the built-in config, which has them all.
var (
	presets       = mustLoadPresets()
	defaultConfig = defaultWith(presets)
)

// Preset describes a built-in language.
type Preset struct {
	Name       string
	Extensions []string
}

// Presets lists the built-in languages, sorted by name.
func Presets() []Preset {
	var list []Preset
	for _, p := range presets {
		list = append(list, Preset{Name: p.name, Extensions: p.config.Extensions})
	}
	return list
}

// Default returns the default config, with the built-in languages named in
// langs, or every one without any.
func Default(langs []string) ([]byte, error) {
	if len(langs) == 0 {
		return defaultConfig, nil
	}
	picked, err := pickPresets(langs)
	if err != nil {
		return nil, err
	}
	return defaultWith(picked), nil
}

func defaultWith(picked []preset) []byte {
	var b strings.Builder
	b.WriteString(header)
	b.WriteString(presetEntries(picked, "  "))
	return []byte(b.String())
}

// presetEntries is how picked are written under languages: each indented
// by prefix, with a blank line between them.
func presetEntries(picked []preset, prefix string) string {
	var entries []string
	for _, p := range picked {
		entries = append(entries, indent(p.text, prefix))
	}
	return strings.Join(entries, "\n")
}

// pickPresets returns the built-in languages named in langs, sorted by
// name, or an error naming one that isn't built in.
func pickPresets(langs []string) ([]preset, error) {
	for _, name := range langs {
		if !isPreset(name) {
			return nil, fmt.Errorf("there is no built-in language %q; the built-in ones are %s", name, strings.Join(presetNames(), ", "))
		}
	}
	return slices.DeleteFunc(slices.Clone(presets), func(p preset) bool { return !slices.Contains(langs, p.name) }), nil
}

func isPreset(name string) bool {
	return slices.ContainsFunc(presets, func(p preset) bool { return p.name == name })
}

func presetNames() []string {
	var names []string
	for _, p := range presets {
		names = append(names, p.name)
	}
	return names
}

// indent prefixes each line of text that isn't blank.
func indent(text, prefix string) string {
	var b strings.Builder
	for line := range strings.Lines(text) {
		if strings.TrimSpace(line) != "" {
			b.WriteString(prefix)
		}
		b.WriteString(line)
	}
	return b.String()
}

// mustLoadPresets reads the built-in languages. They are part of the
// program, so one that doesn't load is a bug, which the tests catch.
func mustLoadPresets() []preset {
	list, err := loadPresets(presetFiles)
	if err != nil {
		panic(err)
	}
	return list
}

func loadPresets(files fs.FS) ([]preset, error) {
	names, err := fs.Glob(files, "languages/*.yml")
	if err != nil {
		return nil, err
	}
	var list []preset
	for _, name := range names {
		p, err := loadPreset(files, name)
		if err != nil {
			return nil, fmt.Errorf("built-in language %s: %w", name, err)
		}
		list = append(list, p)
	}
	return list, nil
}

// loadPreset reads the built-in language in the file at name, which must
// hold that language alone, under the file's name.
func loadPreset(files fs.FS, name string) (preset, error) {
	text, err := fs.ReadFile(files, name)
	if err != nil {
		return preset{}, err
	}
	var entry map[string]language
	decoder := yaml.NewDecoder(bytes.NewReader(text))
	decoder.KnownFields(true)
	if err := decoder.Decode(&entry); err != nil {
		return preset{}, err
	}
	languageName := strings.TrimSuffix(path.Base(name), ".yml")
	config, ok := entry[languageName]
	if !ok || len(entry) != 1 {
		return preset{}, fmt.Errorf("the file should hold %s alone", languageName)
	}
	return preset{name: languageName, text: string(text), config: config}, nil
}
