package config

import (
	"cmp"
	"maps"
)

// merge lays over on top of base. Languages and patterns merge by name, so
// over can change one field of an entry or add an entry; any other field
// over sets replaces base's. Neither file is changed.
func merge(base, over file) file {
	merged := file{
		Exclude:   cmp.Or(over.Exclude, base.Exclude),
		Languages: maps.Clone(base.Languages),
	}
	if merged.Languages == nil {
		merged.Languages = map[string]*language{}
	}
	for name, l := range over.Languages {
		merged.Languages[name] = mergeLanguage(base.Languages[name], l)
	}
	return merged
}

func mergeLanguage(base, over *language) *language {
	if base == nil {
		return over
	}
	if over == nil {
		return base
	}
	merged := &language{
		Enabled:    cmp.Or(over.Enabled, base.Enabled),
		Extensions: cmp.Or(over.Extensions, base.Extensions),
		Patterns:   maps.Clone(base.Patterns),
	}
	if merged.Patterns == nil {
		merged.Patterns = map[string]*pattern{}
	}
	for name, p := range over.Patterns {
		merged.Patterns[name] = mergePattern(base.Patterns[name], p)
	}
	return merged
}

func mergePattern(base, over *pattern) *pattern {
	if base == nil {
		return over
	}
	if over == nil {
		return base
	}
	return &pattern{
		Enabled: cmp.Or(over.Enabled, base.Enabled),
		Regex:   cmp.Or(over.Regex, base.Regex),
	}
}
