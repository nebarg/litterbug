package walk

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestExcludePatterns(t *testing.T) {
	root := filepath.FromSlash("/repo")
	web := filepath.Join(root, "web")
	api := filepath.Join(root, "api")
	for _, c := range []struct {
		name      string
		base, dir string
		pattern   string
		skipped   []string // relative to dir; a trailing slash marks a directory
		kept      []string
	}{
		{"a name at any depth", root, root, "node_modules",
			[]string{"node_modules/", "web/node_modules/", "a/b/node_modules"}, []string{"node_modules.js", "web/x"}},
		{"a name for directories only", root, root, "build/",
			[]string{"build/", "web/build/"}, []string{"build", "web/build"}},
		{"a glob name", root, root, "*_test.go",
			[]string{"main_test.go", "pkg/a_test.go"}, []string{"main.go", "test.go", "pkg/a_test.go.orig"}},
		{"a glob extension", root, root, "*.min.js",
			[]string{"app.min.js", "web/dist/app.min.js"}, []string{"app.js", "min.js.map"}},
		{"a negated class", root, root, "[!x]y.go",
			[]string{"ay.go"}, []string{"xy.go", "y.go"}},
		{"an escaped glob character", root, root, `odd\[1\]`,
			[]string{"odd[1]/"}, []string{"odd1/"}},
		{"a path from the base", root, root, "web/gen",
			[]string{"web/gen/", "web/gen"}, []string{"gen/", "api/web/gen/", "web/gen2/"}},
		{"a file path", root, root, "src/Debug/Dumper.php",
			[]string{"src/Debug/Dumper.php"}, []string{"Dumper.php", "lib/src/Debug/Dumper.php"}},
		{"everything below a directory", root, root, "dist/**",
			[]string{"dist/a.js", "dist/x/", "dist/x/b.js"}, []string{"dist/", "src/dist/a.js"}},
		{"a directory at any depth", root, root, "**/gen",
			[]string{"gen/", "a/b/gen/"}, []string{"gen2/", "a/gen.go"}},
		{"a single-directory wildcard", root, root, "web/*/out",
			[]string{"web/a/out/"}, []string{"web/out/", "web/a/b/out/"}},
		{"a leading ./", root, root, "./gen",
			[]string{"gen/"}, []string{"web/gen/"}},
		{"a path from the base, walking inside it", root, web, "web/gen",
			[]string{"gen/"}, []string{"web/gen/", "x/gen/"}},
		{"a path elsewhere in the base, walking inside it", root, web, "api/gen",
			nil, []string{"gen/", "api/gen/"}},
		{"a path from the base, walking around it", web, root, "./gen",
			[]string{"web/gen/"}, []string{"gen/", "api/gen/"}},
		{"the base itself, walking around it", web, root, ".",
			[]string{"web/"}, []string{"api/", "x"}},
		{"a path up from the base", web, root, "../api",
			[]string{"api/"}, []string{"web/api/"}},
		{"a base beside the walked directory", web, api, "./gen",
			nil, []string{"gen/"}},
		{"a leading slash", root, root, "/gen",
			[]string{"gen/", "gen"}, []string{"web/gen/", "gen2/"}},
		{"a leading slash for directories only", root, root, "/gen/",
			[]string{"gen/"}, []string{"gen", "web/gen/"}},
		{"a leading slash with a glob", root, root, "/*.go",
			[]string{"main.go"}, []string{"web/main.go"}},
		{"a leading slash, walking inside the base", root, web, "/web/gen",
			[]string{"gen/"}, []string{"web/gen/"}},
		{"a leading slash is never the file system's root", web, web, filepath.ToSlash(filepath.Join(web, "gen")),
			nil, []string{"gen/", "x/gen/"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			exclude, err := ParseExclude(c.base, c.dir, []string{c.pattern})
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []struct {
				paths   []string
				skipped bool
			}{{c.skipped, true}, {c.kept, false}} {
				for _, p := range want.paths {
					rel, isDir := strings.CutSuffix(p, "/")
					if got := exclude.skips(filepath.FromSlash(rel), isDir); got != want.skipped {
						t.Errorf("%q skips %s = %v, want %v", c.pattern, p, got, want.skipped)
					}
				}
			}
		})
	}
}

func TestDotDirectoriesAreAlwaysSkipped(t *testing.T) {
	var none Exclude
	for p, want := range map[string]bool{".cache/": true, "web/.venv/": true, ".env": false, "web/.eslintrc.js": false, "web/": false} {
		rel, isDir := strings.CutSuffix(p, "/")
		if got := none.skips(filepath.FromSlash(rel), isDir); got != want {
			t.Errorf("skips %s = %v, want %v", p, got, want)
		}
	}
}

func TestSkipsPath(t *testing.T) {
	root := filepath.FromSlash("/repo")
	exclude, err := ParseExclude(root, root, []string{"vendor", "web/gen", "*.min.js", "build/"})
	if err != nil {
		t.Fatal(err)
	}
	for rel, want := range map[string]bool{
		"vendor/x.go":       true,
		"a/vendor/b/x.go":   true,
		"web/gen/x.go":      true,
		"x/web/gen/y.go":    false,
		"app.min.js":        true,
		"web/app.min.js":    true,
		"build/x.js":        true,
		"build":             false,
		".github/script.js": true,
		"a/.cache/x.go":     true,
		".eslintrc.js":      false,
		"src/a.go":          false,
	} {
		if got := exclude.SkipsPath(rel); got != want {
			t.Errorf("SkipsPath(%s) = %v, want %v", rel, got, want)
		}
	}
}

func TestExcludeWith(t *testing.T) {
	root := filepath.FromSlash("/repo")
	a, errA := ParseExclude(root, root, []string{"vendor"})
	b, errB := ParseExclude(root, root, []string{"*.min.js"})
	if errA != nil || errB != nil {
		t.Fatal(errA, errB)
	}
	both := a.With(b)
	if !both.skips("vendor", true) || !both.skips("app.min.js", false) || both.skips("app.js", false) {
		t.Error("combined excludes don't skip what each did")
	}
	if a.skips("app.min.js", false) {
		t.Error("combining changed an exclude")
	}
}

func TestParseExcludeRejectsBadPatterns(t *testing.T) {
	root := filepath.FromSlash("/repo")
	for _, pattern := range []string{"", "/", "//", "[", "web/[a", `x\`} {
		if _, err := ParseExclude(root, root, []string{"ok", pattern}); err == nil || !strings.Contains(err.Error(), fmt.Sprintf("%q", pattern)) {
			t.Errorf("ParseExclude(%q) = %v, want an error", pattern, err)
		}
	}
	if exclude, err := ParseExclude(root, root, nil); err != nil || len(exclude.rules) > 0 {
		t.Errorf("no patterns = %+v, %v", exclude, err)
	}
}
