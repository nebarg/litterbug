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
}

// matchLine returns the first of rules that matches line, so a line that
// several rules match is reported once.
func matchLine(rules []config.Rule, line string) (config.Rule, bool) {
	for _, rule := range rules {
		if rule.Regex.MatchString(line) {
			return rule, true
		}
	}
	return config.Rule{}, false
}

// findInContent finds the lines of a file's content that match its rules.
func findInContent(path, content string, rules []config.Rule) []Finding {
	var findings []Finding
	number := 0
	for line := range strings.Lines(content) {
		number++
		line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if rule, ok := matchLine(rules, line); ok {
			findings = append(findings, Finding{Path: path, Line: number, Text: line, Rule: rule})
		}
	}
	return findings
}
