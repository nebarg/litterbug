package config

import (
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
)

// Rules are the patterns a scan looks for, compiled from a config.
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
	// Unless, if set, stops the rule matching a line that it also matches,
	// such as one defining a function the rule looks for calls to.
	Unless *regexp.Regexp
}

// Match returns where the rule's regex first matches line, as
// FindStringIndex does, or nil when it doesn't match or Unless matches
// line too. Unless only runs on a line the regex matches, which few are.
func (r Rule) Match(line string) []int {
	match := r.Regex.FindStringIndex(line)
	if match == nil || (r.Unless != nil && r.Unless.MatchString(line)) {
		return nil
	}
	return match
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

// compile checks every language and pattern in a config, whether enabled
// or not, and compiles those being scanned: the languages in langs, or
// without any, those the config enables.
func compile(f file, langs []string) (Rules, error) {
	names := slices.Sorted(maps.Keys(f.Languages))
	for _, name := range langs {
		if slices.Contains(names, name) {
			continue
		}
		err := fmt.Errorf("unknown language %q: the config has %s", name, strings.Join(names, ", "))
		if isPreset(name) {
			err = fmt.Errorf("%w; litterbug add-language %s adds it", err, name)
		}
		return Rules{}, err
	}
	rules := Rules{Exclude: f.Exclude}
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

func compileLanguage(name string, l language) (compiledLanguage, error) {
	if len(l.Extensions) == 0 {
		return compiledLanguage{}, fmt.Errorf("language %s needs at least one extension", name)
	}
	if len(l.Patterns) == 0 {
		return compiledLanguage{}, fmt.Errorf("language %s needs at least one pattern", name)
	}
	var compiled compiledLanguage
	for _, extension := range l.Extensions {
		if len(extension) < 2 || extension[0] != '.' || strings.ContainsAny(extension, `/\`) {
			return compiledLanguage{}, fmt.Errorf("language %s: extension %q should be a file name ending, such as .go", name, extension)
		}
		compiled.extensions = append(compiled.extensions, strings.ToLower(extension))
	}
	for _, patternName := range slices.Sorted(maps.Keys(l.Patterns)) {
		p := l.Patterns[patternName]
		// An empty regex would match every line.
		if p.Regex == "" {
			return compiledLanguage{}, fmt.Errorf("pattern %s/%s needs a regex", name, patternName)
		}
		regex, err := regexp.Compile(p.Regex)
		if err != nil {
			return compiledLanguage{}, fmt.Errorf("pattern %s/%s: %w", name, patternName, err)
		}
		unless, err := compileUnless(p.Unless)
		if err != nil {
			return compiledLanguage{}, fmt.Errorf("pattern %s/%s: unless: %w", name, patternName, err)
		}
		if enabled(p.Enabled) {
			compiled.rules = append(compiled.rules, Rule{Language: name, Name: patternName, Regex: regex, Unless: unless})
		}
	}
	return compiled, nil
}

// compileUnless compiles a pattern's unless regex, or returns nil for an
// empty one, which is the same as leaving it out. As a regex, an empty one
// would match every line, switching the pattern off.
func compileUnless(unless string) (*regexp.Regexp, error) {
	if unless == "" {
		return nil, nil
	}
	return regexp.Compile(unless)
}

// enabled reads an enabled field, which is true when left out.
func enabled(field *bool) bool {
	return field == nil || *field
}
