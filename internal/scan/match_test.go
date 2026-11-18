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
		line       string
		want       string // the rule's name, or empty for no match
		start, end int
	}{
		{`fmt.Println(x)`, "fmt-print", 0, 9},
		{`log.Println(x)`, "log-print", 0, 9},
		{`spew.Dump(x)`, "spew", 0, 5},
		{`x := 1; spew.Dump(x)`, "spew", 8, 13},
		{`spew.Dump(x); fmt.Println(x)`, "fmt-print", 14, 23},
		{`x := 1`, "", 0, 0},
		{``, "", 0, 0},
	} {
		f, ok := matchLine(rules, c.line)
		if ok != (c.want != "") || f.Rule.Name != c.want || f.Start != c.start || f.End != c.end {
			t.Errorf("matchLine(%q) = %q at %d:%d, %v; want %q at %d:%d", c.line, f.Rule.Name, f.Start, f.End, ok, c.want, c.start, c.end)
		}
		if ok && f.Text != c.line {
			t.Errorf("matchLine(%q) text = %q", c.line, f.Text)
		}
	}
	if _, ok := matchLine(nil, `fmt.Println(x)`); ok {
		t.Error("a line matched without any rules")
	}
}

func TestMatchLinePassesOverARuleWhoseUnlessMatches(t *testing.T) {
	builtinPrint := config.Rule{
		Language: "go", Name: "builtin-print",
		Regex: regexp.MustCompile(`\bprint\(`), Unless: regexp.MustCompile(`\bfunc\s+print\(`),
	}
	rules := []config.Rule{builtinPrint, spew}
	for line, want := range map[string]string{
		`print(x)`:                           "builtin-print",
		`func print(x any) {`:                "",
		`func print(x any) { spew.Dump(x) }`: "spew",
	} {
		if f, ok := matchLine(rules, line); ok != (want != "") || f.Rule.Name != want {
			t.Errorf("matchLine(%q) = %q, %v; want %q", line, f.Rule.Name, ok, want)
		}
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
