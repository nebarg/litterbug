package main

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/nebarg/litterbug/internal/scan"
)

// maxLineLength is how many characters of a finding's line are shown, so
// a line of minified code can't flood the terminal.
const maxLineLength = 160

// report prints each finding as path:line: text [language/pattern], which
// editors and terminals can follow to the line, then how many there are.
func report(out io.Writer, findings []scan.Finding) {
	if len(findings) == 0 {
		_, _ = fmt.Fprintln(out, "No debug statements found.")
		return
	}
	files := map[string]bool{}
	for _, f := range findings {
		files[f.Path] = true
		_, _ = fmt.Fprintf(out, "%s:%d: %s  [%s/%s]\n",
			cleanDisplay(f.Path), f.Line, displayLine(f.Text, f.Start, f.End), cleanDisplay(f.Rule.Language), cleanDisplay(f.Rule.Name))
	}
	_, _ = fmt.Fprintf(out, "\n%s in %s\n", plural(len(findings), "debug statement", "debug statements"), plural(len(files), "file", "files"))
}

// displayLine trims a finding's line, whose match is the bytes from start
// to end. A line over maxLineLength characters is cut to a window around
// the match, with as much of the line before it as after, and an ellipsis
// where the line goes on.
func displayLine(text string, start, end int) string {
	lead := len(text) - len(strings.TrimLeftFunc(text, unicode.IsSpace))
	text = strings.TrimSpace(text)
	start = min(max(start-lead, 0), len(text))
	end = min(max(end-lead, start), len(text))
	runes := []rune(text)
	if len(runes) <= maxLineLength {
		return cleanDisplay(text)
	}
	matchStart := utf8.RuneCountInString(text[:start])
	matchLength := utf8.RuneCountInString(text[start:end])
	from := max(matchStart-max(maxLineLength-matchLength, 0)/2, 0)
	from = min(from, len(runes)-maxLineLength)
	to := from + maxLineLength
	window := runes[from:to]
	if from > 0 {
		window = slices.Concat([]rune("…"), window[1:])
	}
	if to < len(runes) {
		window = slices.Concat(window[:len(window)-1], []rune("…"))
	}
	return cleanDisplay(string(window))
}

// cleanDisplay replaces control characters with spaces. Paths and lines
// come from the files scanned, and a terminal would act on them.
func cleanDisplay(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
}

// plural reads like "1 file" or "3 files".
func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}
