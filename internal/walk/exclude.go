package walk

import (
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// Exclude is what a walk skips on top of every directory whose name starts
// with a dot: the files and directories that match its patterns, written as
// in a .gitignore. ParseExclude makes one.
type Exclude struct {
	rules []excludeRule
}

// excludeRule is one exclude pattern, ready to match paths relative to the
// walked directory.
type excludeRule struct {
	glob     []string // the pattern's segments, in path.Match syntax
	anchored bool     // whether glob is a path from the pattern's base, rather than a name at any depth
	dirOnly  bool
	// An anchored pattern matches paths relative to its base. A path
	// relative to the walked directory becomes one by dropping strip from
	// its start, when the base is inside the walked directory, or by
	// adding prefix, when the walked directory is inside the base.
	strip, prefix []string
}

// ParseExclude turns exclude patterns into what a walk of dir skips. As in
// a .gitignore, a pattern without a slash matches a name at any depth, and
// one with a slash is a path from base; "*", "?" and "[...]" match within a
// name, "**" matches any number of directories, and a trailing slash
// matches only directories. A path outside both base and dir has nothing
// to skip.
func ParseExclude(base, dir string, patterns []string) (Exclude, error) {
	var exclude Exclude
	for _, pattern := range patterns {
		rule, ok, err := parseExcludeRule(base, dir, pattern)
		if err != nil {
			return Exclude{}, err
		}
		if ok {
			exclude.rules = append(exclude.rules, rule)
		}
	}
	return exclude, nil
}

// With adds other's patterns, parsed for the same walked directory.
func (e Exclude) With(other Exclude) Exclude {
	return Exclude{rules: slices.Concat(e.rules, other.rules)}
}

func parseExcludeRule(base, dir, pattern string) (rule excludeRule, ok bool, err error) {
	p := filepath.ToSlash(pattern)
	trimmed := strings.TrimRight(p, "/")
	rule.dirOnly = trimmed != p
	if trimmed == "" {
		return rule, false, fmt.Errorf("%q needs a file or directory name, path or pattern", pattern)
	}
	if !strings.Contains(trimmed, "/") && trimmed != "." && trimmed != ".." {
		rule.glob, err = globSegments(pattern, trimmed)
		return rule, err == nil, err
	}
	rule.anchored = true
	if filepath.IsAbs(pattern) {
		volume := filepath.VolumeName(pattern)
		base, trimmed = volume+string(filepath.Separator), strings.TrimLeft(trimmed[len(volume):], "/")
	}
	trimmed = path.Clean(trimmed)
	for trimmed == ".." || strings.HasPrefix(trimmed, "../") {
		base, trimmed = filepath.Dir(base), strings.TrimPrefix(strings.TrimPrefix(trimmed, ".."), "/")
	}
	// A pattern naming base itself, such as ".", leaves no segments: it
	// skips base when base is inside the walked directory.
	if trimmed != "." && trimmed != "" {
		if rule.glob, err = globSegments(pattern, trimmed); err != nil {
			return rule, false, err
		}
	}
	if rel, err := filepath.Rel(base, dir); err == nil && filepath.IsLocal(rel) {
		rule.prefix = splitPath(rel)
		return rule, true, nil
	}
	if rel, err := filepath.Rel(dir, base); err == nil && filepath.IsLocal(rel) {
		rule.strip = splitPath(rel)
		return rule, true, nil
	}
	return rule, false, nil
}

// globSegments splits a slash-separated pattern into path.Match patterns,
// rejecting a malformed one, such as an unclosed "[".
func globSegments(pattern, slashed string) ([]string, error) {
	var glob []string
	for segment := range strings.SplitSeq(slashed, "/") {
		segment = pathMatchGlob(segment)
		if _, err := path.Match(segment, ""); err != nil {
			return nil, fmt.Errorf("%q is not a valid pattern: %w", pattern, err)
		}
		glob = append(glob, segment)
	}
	return glob, nil
}

// splitPath splits a relative path into its segments, or none for ".".
func splitPath(rel string) []string {
	if rel == "." {
		return nil
	}
	return strings.Split(filepath.ToSlash(rel), "/")
}

// SkipsPath reports whether a walk would skip the file at rel, a
// slash-separated path relative to the walked directory, because it or a
// directory it is in is skipped. It is for files found some other way,
// such as in a diff.
func (e Exclude) SkipsPath(rel string) bool {
	segments := strings.Split(rel, "/")
	for i := 1; i < len(segments); i++ {
		if e.skips(strings.Join(segments[:i], "/"), true) {
			return true
		}
	}
	return e.skips(rel, false)
}

// skips reports whether a walk skips the file or directory at rel, relative
// to the walked directory: one starting with a dot, if it is a directory,
// or one an exclude pattern matches.
func (e Exclude) skips(rel string, isDir bool) bool {
	segments := splitPath(rel)
	if isDir && strings.HasPrefix(segments[len(segments)-1], ".") {
		return true
	}
	return slices.ContainsFunc(e.rules, func(r excludeRule) bool { return r.matches(segments, isDir) })
}

func (r excludeRule) matches(segments []string, isDir bool) bool {
	if r.dirOnly && !isDir {
		return false
	}
	if !r.anchored {
		return matchSegment(r.glob[0], segments[len(segments)-1], false)
	}
	if len(r.strip) > 0 {
		if len(segments) < len(r.strip) || !slices.Equal(segments[:len(r.strip)], r.strip) {
			return false
		}
		segments = segments[len(r.strip):]
	}
	return matchSegments(r.glob, slices.Concat(r.prefix, segments), false)
}
