package walk

import (
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// Exclude lists directories a walk skips, on top of every directory whose
// name starts with a dot. Names skip a directory at any depth; Paths are
// slash-separated and relative to the walked directory.
type Exclude struct {
	Names []string
	Paths []string
}

func (e Exclude) skipsDir(rel string) bool {
	rel = filepath.ToSlash(rel)
	name := path.Base(rel)
	return strings.HasPrefix(name, ".") || slices.Contains(e.Names, name) || slices.Contains(e.Paths, rel)
}

// ParseExclude turns exclude values into directories to skip under dir. A
// bare name skips that directory at any depth; anything with a slash is a
// path from base, and one outside dir has nothing to skip.
func ParseExclude(base, dir string, values []string) (Exclude, error) {
	var exclude Exclude
	for _, value := range values {
		name := strings.TrimRight(filepath.ToSlash(value), "/")
		if name == "" {
			return Exclude{}, fmt.Errorf("exclude %q needs a directory name or path", value)
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
