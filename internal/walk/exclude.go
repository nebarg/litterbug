package walk

import (
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// Exclude lists files and directories a walk skips, on top of every
// directory whose name starts with a dot. Names skip any file or directory
// so named, at any depth; Paths are slash-separated and relative to the
// walked directory.
type Exclude struct {
	Names []string
	Paths []string
}

// excludes reports whether the file or directory at rel is excluded.
func (e Exclude) excludes(rel string) bool {
	rel = filepath.ToSlash(rel)
	return slices.Contains(e.Names, path.Base(rel)) || slices.Contains(e.Paths, rel)
}

// skipsDir reports whether the directory at rel is skipped: excluded, or
// starting with a dot. Files starting with a dot are walked.
func (e Exclude) skipsDir(rel string) bool {
	return strings.HasPrefix(filepath.Base(rel), ".") || e.excludes(rel)
}

// ParseExclude turns exclude values into files and directories to skip
// under dir. A bare name skips any file or directory so named, at any
// depth; anything with a slash is a path from base, and one outside dir has
// nothing to skip.
func ParseExclude(base, dir string, values []string) (Exclude, error) {
	var exclude Exclude
	for _, value := range values {
		name := strings.TrimRight(filepath.ToSlash(value), "/")
		if name == "" {
			return Exclude{}, fmt.Errorf("exclude %q needs a file or directory name or path", value)
		}
		if !strings.Contains(name, "/") && name != "." && name != ".." {
			exclude.Names = append(exclude.Names, name)
			continue
		}
		target := value
		if !filepath.IsAbs(target) {
			target = filepath.Join(base, target)
		}
		rel, err := filepath.Rel(dir, target)
		if err != nil || rel == "." || !filepath.IsLocal(rel) {
			continue
		}
		exclude.Paths = append(exclude.Paths, filepath.ToSlash(rel))
	}
	return exclude, nil
}
