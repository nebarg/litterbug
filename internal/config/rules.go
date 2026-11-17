package config

import (
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
)

// Rules are the patterns a scan looks for, compiled from the merged config.
type Rules struct {
	// Exclude lists the patterns of files and directories to skip, as the
	// config gives them: written as in a .gitignore, with paths from the
	// repository root.
	Exclude   []string
	languages []compiledLanguage
}

// Rule is an enabled pattern of a language being scanned.
type Rule struct {
	Language, Name string
	Regex          *regexp.Regexp
}

type compiledLanguage struct {
	extensions []string // lower case
	rules      []Rule   // sorted by name
}

// For returns the rules for the file at path: those of every language
// being scanned with an extension that ends the path, ignoring case.
// Languages can share extensions, so .php and .blade.php both apply to a
// Blade template. Rules are sorted by language, then by name.
func (r Rules) For(path string) []Rule {
	path = strings.ToLower(path)
	var rules []Rule
	for _, l := range r.languages {
		if slices.ContainsFunc(l.extensions, func(extension string) bool { return strings.HasSuffix(path, extension) }) {
			rules = append(rules, l.rules...)
		}
	}
	return rules
}

// compile checks that every language and pattern in the merged config is
// complete, whether enabled or not, and compiles those being scanned: the
// languages in langs, or without any, those the config enables.
func compile(f file, langs []string) (Rules, error) {
	names := slices.Sorted(maps.Keys(f.Languages))
	for _, name := range langs {
		if !slices.Contains(names, name) {
			return Rules{}, fmt.Errorf("unknown language %q: the config has %s", name, strings.Join(names, ", "))
		}
	}
	var rules Rules
	if f.Exclude != nil {
		rules.Exclude = *f.Exclude
	}
	for _, name := range names {
		l := f.Languages[name]
		compiled, err := compileLanguage(name, l)
		if err != nil {
			return Rules{}, err
		}
		picked := slices.Contains(langs, name)
		if (len(langs) > 0 && !picked) || (len(langs) == 0 && !enabled(l.Enabled)) {
			continue
		}
		if len(compiled.rules) == 0 {
			if picked {
				return Rules{}, fmt.Errorf("language %s has no enabled patterns", name)
			}
			continue
		}
		rules.languages = append(rules.languages, compiled)
	}
	if len(rules.languages) == 0 {
		return Rules{}, errors.New("no languages are enabled, so there is nothing to scan for")
	}
	return rules, nil
}

func compileLanguage(name string, l *language) (compiledLanguage, error) {
	if l == nil || l.Extensions == nil || len(*l.Extensions) == 0 {
		return compiledLanguage{}, fmt.Errorf("language %s needs at least one extension", name)
	}
	if len(l.Patterns) == 0 {
		return compiledLanguage{}, fmt.Errorf("language %s needs at least one pattern", name)
	}
	var compiled compiledLanguage
	for _, extension := range *l.Extensions {
		compiled.extensions = append(compiled.extensions, strings.ToLower(extension))
	}
	for _, patternName := range slices.Sorted(maps.Keys(l.Patterns)) {
		p := l.Patterns[patternName]
		if p == nil || p.Regex == nil {
			return compiledLanguage{}, fmt.Errorf("pattern %s/%s needs a regex", name, patternName)
		}
		if !enabled(p.Enabled) {
			continue
		}
		regex, err := regexp.Compile(*p.Regex)
		if err != nil {
			return compiledLanguage{}, fmt.Errorf("pattern %s/%s: %w", name, patternName, err)
		}
		compiled.rules = append(compiled.rules, Rule{Language: name, Name: patternName, Regex: regex})
	}
	return compiled, nil
}

// enabled reads an enabled field, which is true when left out.
func enabled(field *bool) bool {
	return field == nil || *field
}
