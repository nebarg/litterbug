package scan

import (
	"fmt"
	"regexp"
	"slices"
	"testing"

	"github.com/nebarg/litterbug/internal/config"
)

var (
	fmtPrint = config.Rule{Language: "go", Name: "fmt-print", Regex: regexp.MustCompile(`\bfmt\.Print`)}
	logPrint = config.Rule{Language: "go", Name: "log-print", Regex: regexp.MustCompile(`\blog\.Print`)}
	spew     = config.Rule{Language: "go", Name: "spew", Regex: regexp.MustCompile(`\bspew\.`)}
)

func TestMatchLineReportsTheFirstMatchingRule(t *testing.T) {
	rules := []config.Rule{fmtPrint, logPrint, spew}
	for _, c := range []struct {
		line string
		want string // the rule's name, or empty for no match
	}{
		{`fmt.Println(x)`, "fmt-print"},
		{`log.Println(x)`, "log-print"},
		{`spew.Dump(x)`, "spew"},
		{`fmt.Println(x); spew.Dump(x)`, "fmt-print"},
		{`x := 1`, ""},
		{``, ""},
	} {
		rule, ok := matchLine(rules, c.line)
		if ok != (c.want != "") || rule.Name != c.want {
			t.Errorf("matchLine(%q) = %q, %v; want %q", c.line, rule.Name, ok, c.want)
		}
	}
	if _, ok := matchLine(nil, `fmt.Println(x)`); ok {
		t.Error("a line matched without any rules")
	}
}

func TestFindInContent(t *testing.T) {
	rules := []config.Rule{fmtPrint, spew}
	for _, c := range []struct {
		name, content string
		want          []string
	}{
		{"line numbers count from one", "package main\n\nfunc main() {\n\tfmt.Println(1)\n}\n", []string{"4 fmt-print \"\\tfmt.Println(1)\""}},
		{"each matching line", "fmt.Println(1)\nx := 2\nspew.Dump(x)\n", []string{"1 fmt-print \"fmt.Println(1)\"", "3 spew \"spew.Dump(x)\""}},
		{"a line once, for its first rule", "spew.Dump(x); fmt.Println(x)\n", []string{"1 fmt-print \"spew.Dump(x); fmt.Println(x)\""}},
		{"Windows line endings are dropped", "x := 1\r\nfmt.Println(x)\r\n", []string{"2 fmt-print \"fmt.Println(x)\""}},
		{"a last line without an ending", "x := 1\nspew.Dump(x)", []string{"2 spew \"spew.Dump(x)\""}},
		{"a lone carriage return is part of the line", "fmt.Println(x)\rspew.Dump(x)\n", []string{"1 fmt-print \"fmt.Println(x)\\rspew.Dump(x)\""}},
		{"nothing to find", "x := 1\n", nil},
		{"an empty file", "", nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			var got []string
			for _, f := range findInContent("main.go", c.content, rules) {
				if f.Path != "main.go" {
					t.Errorf("path = %q", f.Path)
				}
				got = append(got, fmt.Sprintf("%d %s %q", f.Line, f.Rule.Name, f.Text))
			}
			if !slices.Equal(got, c.want) {
				t.Errorf("findings:\n got  %v\n want %v", got, c.want)
			}
		})
	}
}
