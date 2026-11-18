package config

import (
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

func TestPresetsAreTheDefaultConfigsLanguages(t *testing.T) {
	f, err := parse(defaultConfig)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := slices.Sorted(maps.Keys(f.Languages)), presetNames(); !slices.Equal(got, want) || !slices.IsSorted(want) {
		t.Fatalf("the default config has %v, want the presets %v, sorted", got, want)
	}
	// The default config holds each preset as its file does, so indenting
	// them under languages lost nothing.
	for _, p := range presets {
		if !reflect.DeepEqual(f.Languages[p.name], p.config) {
			t.Errorf("%s in the default config = %+v, want %+v", p.name, f.Languages[p.name], p.config)
		}
		if !strings.Contains(string(defaultConfig), "\n  "+p.name+":\n    enabled: true\n") {
			t.Errorf("%s isn't laid out under languages as the other keys are", p.name)
		}
	}
	if !strings.HasPrefix(string(defaultConfig), header) {
		t.Error("the default config doesn't start with the header")
	}
	if want := []Preset{{"rust", []string{".rs"}}}; !slices.ContainsFunc(Presets(), func(p Preset) bool { return reflect.DeepEqual(p, want[0]) }) {
		t.Errorf("Presets() = %v, want it to have %v", Presets(), want)
	}
}

func TestDefaultWithLanguages(t *testing.T) {
	for _, c := range []struct {
		langs []string
		want  []string
	}{
		{nil, presetNames()},
		{[]string{"python"}, []string{"python"}},
		{[]string{"rust", "go", "rust"}, []string{"go", "rust"}},
	} {
		data, err := Default(c.langs)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parse(data)
		if err != nil {
			t.Fatalf("Default(%v) doesn't parse: %v", c.langs, err)
		}
		if got := slices.Sorted(maps.Keys(f.Languages)); !slices.Equal(got, c.want) {
			t.Errorf("Default(%v) has %v, want %v", c.langs, got, c.want)
		}
		if want := []string{"node_modules", "vendor"}; !slices.Equal(f.Exclude, want) {
			t.Errorf("Default(%v) excludes %v, want %v", c.langs, f.Exclude, want)
		}
		if _, err := compile(f, nil); err != nil {
			t.Errorf("Default(%v): %v", c.langs, err)
		}
	}

	_, err := Default([]string{"go", "elixir"})
	if err == nil || !strings.Contains(err.Error(), `"elixir"`) || !strings.Contains(err.Error(), strings.Join(presetNames(), ", ")) {
		t.Errorf("an unknown language: %v", err)
	}
}

func TestLoadPresetsRejectsABadFile(t *testing.T) {
	for _, c := range []struct {
		name, content, want string
	}{
		{"misnamed", "ruby:\n  extensions: [.rb]\n", "should hold python alone"},
		{"two languages", "python:\n  extensions: [.py]\nruby:\n  extensions: [.rb]\n", "should hold python alone"},
		{"an unknown key", "python:\n  extension: [.py]\n", "extension"},
		{"malformed", "python: [\n", "python.yml"},
	} {
		t.Run(c.name, func(t *testing.T) {
			files := fstest.MapFS{"languages/python.yml": {Data: []byte(c.content)}}
			if _, err := loadPresets(files); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want it to mention %q", err, c.want)
			}
		})
	}
}

func TestUnknownLangSuggestsAddingAPreset(t *testing.T) {
	f, err := parse([]byte("languages:\n  go:\n    extensions: [.go]\n    patterns:\n      p: {regex: 'p'}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := compile(f, []string{"python"}); err == nil || !strings.Contains(err.Error(), "litterbug add-language python adds it") {
		t.Errorf("a built-in language: %v", err)
	}
	if _, err := compile(f, []string{"elixir"}); err == nil || strings.Contains(err.Error(), "add-language") {
		t.Errorf("a language that isn't built in: %v", err)
	}
}
