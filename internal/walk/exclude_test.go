package walk

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseExclude(t *testing.T) {
	root := filepath.FromSlash("/repo")
	sub := filepath.Join(root, "web")
	for _, c := range []struct {
		name      string
		base, dir string
		values    []string
		want      Exclude
	}{
		{"nothing to exclude", root, root, nil, Exclude{}},
		{"bare names skip at any depth", root, root, []string{"dist/", "build"}, Exclude{Names: []string{"dist", "build"}}},
		{"paths are from the base directory", sub, sub, []string{"./gen", "src/out/"}, Exclude{Paths: []string{"gen", "src/out"}}},
		{"paths are made relative to a walked directory elsewhere", root, sub, []string{"web/gen", "other/gen", "..", "."}, Exclude{Paths: []string{"gen"}}},
		{"absolute paths", sub, sub, []string{filepath.Join(sub, "gen")}, Exclude{Paths: []string{"gen"}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseExclude(c.base, c.dir, c.values)
			if err != nil || !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %+v, %v; want %+v", got, err, c.want)
			}
		})
	}
	if _, err := ParseExclude(root, root, []string{"/"}); err == nil {
		t.Fatal("an empty exclude was accepted")
	}
}
