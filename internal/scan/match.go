// Package scan finds debug statements in files by matching each line
// against the config's rules for the file.
package scan

import (
	"strings"

	"github.com/nebarg/litterbug/internal/config"
)

// Finding is a line that matches one of the rules for its file.
type Finding struct {
	Path string // slash-separated, from where the scan started
	Line int
	Text string // the line, without its ending
	Rule config.Rule
	// Start and End are the byte offsets in Text of what Rule matched, so
	// the match can be shown from a line too long to show whole.
	Start, End int
}

// matchLine finds the first of rules that matches line, so a line that
// several rules match is reported once. The finding has no Path or Line.
func matchLine(rules []config.Rule, line string) (Finding, bool) {
	for _, rule := range rules {
		if match := rule.Regex.FindStringIndex(line); match != nil {
			return Finding{Text: line, Rule: rule, Start: match[0], End: match[1]}, true
		}
	}
	return Finding{}, false
}

// findInContent finds the lines of a file's content that match its rules.
func findInContent(path, content string, rules []config.Rule) []Finding {
	var findings []Finding
	number := 0
	for line := range strings.Lines(content) {
		number++
		line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if f, ok := matchLine(rules, line); ok {
			f.Path, f.Line = path, number
			findings = append(findings, f)
		}
	}
	return findings
}
