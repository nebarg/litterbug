package scan

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// The diffs here are as git diff --cached --unified=0 printed them.
func TestAddedLines(t *testing.T) {
	for _, c := range []struct {
		name, diff string
		want       []string
	}{
		{"insertions and a deletion in one file", `diff --git a/mod.go b/mod.go
index fa2da6e..8da834a 100644
--- a/mod.go
+++ b/mod.go
@@ -3,0 +4 @@ line 3
+fmt.Println("inserted")
@@ -8 +8,0 @@ line 7
-line 8
@@ -10,0 +11 @@ line 10
+fmt.Println("end")
`, []string{`mod.go:4 "fmt.Println(\"inserted\")"`, `mod.go:11 "fmt.Println(\"end\")"`}},
		{"a new file", `diff --git a/first.go b/first.go
new file mode 100644
index 0000000..422c2b7
--- /dev/null
+++ b/first.go
@@ -0,0 +1,2 @@
+a
+b
`, []string{`first.go:1 "a"`, `first.go:2 "b"`}},
		{"a renamed file", `diff --git a/tomove.go b/moved.go
similarity index 91%
rename from tomove.go
rename to moved.go
index 916e0cf..a0bcd99 100644
--- a/tomove.go
+++ b/moved.go
@@ -30,0 +31 @@ keep 30
+fmt.Println("moved")
`, []string{`moved.go:31 "fmt.Println(\"moved\")"`}},
		{"a mode change and a binary file", `diff --git a/first.go b/first.go
old mode 100644
new mode 100755
diff --git a/bin.go b/bin.go
new file mode 100644
index 0000000..88768ef
Binary files /dev/null and b/bin.go differ
`, nil},
		{"lines that look like headers", `diff --git a/plus.go b/plus.go
new file mode 100644
index 0000000..123fd7a
--- /dev/null
+++ b/plus.go
@@ -0,0 +1,3 @@
+++ starts with plus
+--- starts with minus
+@@ -1 +1 @@
`, []string{`plus.go:1 "++ starts with plus"`, `plus.go:2 "--- starts with minus"`, `plus.go:3 "@@ -1 +1 @@"`}},
		{"a removed line that looks like a header", `diff --git a/minus.go b/minus.go
index 1111111..2222222 100644
--- a/minus.go
+++ b/minus.go
@@ -1,2 +1 @@
--- a/other.go
-+++ b/other.go
+kept
`, []string{`minus.go:1 "kept"`}},
		{"no newline at the end, before and after", `diff --git a/nonl.go b/nonl.go
index 20cbb4d..5fee9c3 100644
--- a/nonl.go
+++ b/nonl.go
@@ -1 +1,2 @@
-no newline
\ No newline at end of file
+no newline
+still none
\ No newline at end of file
`, []string{`nonl.go:1 "no newline"`, `nonl.go:2 "still none"`}},
		{"Windows line endings", "diff --git a/crlf.go b/crlf.go\n--- /dev/null\n+++ b/crlf.go\n@@ -0,0 +1,2 @@\n+x\r\n+y\r\n",
			[]string{`crlf.go:1 "x"`, `crlf.go:2 "y"`}},
		{"quoted paths", `diff --git "a/quo\"te.go" "b/quo\"te.go"
--- /dev/null
+++ "b/quo\"te.go"
@@ -0,0 +1 @@
+x
diff --git "a/tab\there.go" "b/tab\there.go"
--- /dev/null
+++ "b/tab\there.go"
@@ -0,0 +1 @@
+x
diff --git "a/\303\274.go" "b/\303\274.go"
--- /dev/null
+++ "b/\303\274.go"
@@ -0,0 +1 @@
+x
`, []string{`quo"te.go:1 "x"`, "tab\there.go:1 \"x\"", `ü.go:1 "x"`}},
		{"a path with a space, which ends with a tab", "diff --git a/with space.go b/with space.go\n--- /dev/null\n+++ b/with space.go\t\n@@ -0,0 +1 @@\n+x\n",
			[]string{`with space.go:1 "x"`}},
		{"a deleted file", "diff --git a/gone.go b/gone.go\ndeleted file mode 100644\n--- a/gone.go\n+++ /dev/null\n@@ -1,2 +0,0 @@\n-a\n-b\n",
			nil},
		{"context lines", "diff --git a/c.go b/c.go\n--- a/c.go\n+++ b/c.go\n@@ -1,3 +1,4 @@\n one\n-two\n+2\n+2.5\n three\n\n",
			[]string{`c.go:2 "2"`, `c.go:3 "2.5"`}},
		{"an empty diff", "", nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			added, err := addedLines(c.diff)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, a := range added {
				got = append(got, fmt.Sprintf("%s:%d %q", a.path, a.line, a.text))
			}
			if !slices.Equal(got, c.want) {
				t.Errorf("added lines:\n got  %v\n want %v", got, c.want)
			}
		})
	}
}

func TestAddedLinesRejectsMalformedDiffs(t *testing.T) {
	header := "diff --git a/x.go b/x.go\n--- a/x.go\n+++ b/x.go\n"
	for _, c := range []struct{ name, diff, want string }{
		{"a hunk header without counts", header + "@@ nonsense @@\n", "hunk header"},
		{"a count too big to read", header + "@@ -1 +1,99999999999999999999 @@\n", "hunk header"},
		{"a hunk cut short", header + "@@ -0,0 +1,2 @@\n+a\n", "ended mid-hunk"},
		{"a line no hunk has", header + "@@ -0,0 +1,2 @@\n+a\n*b\n", "unexpected line"},
		{"a path without its prefix", "--- a/x.go\n+++ x.go\n", "unexpected path"},
		{"a badly quoted path", "--- a/x.go\n+++ \"b/x.go\n", "unreadable path"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if added, err := addedLines(c.diff); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("addedLines = %v, %v; want an error about %q", added, err, c.want)
			}
		})
	}
}
