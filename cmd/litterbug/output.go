package main

import (
	"cmp"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/nebarg/litterbug/internal/scan"
)

// maxLineLength is how many characters of a finding's line are shown, so
// a line of minified code can't flood the terminal.
const maxLineLength = 160

// style is how findings are printed: grouped under their file, for a person
// at a terminal, or one to a line as path:line: text, for scripts and
// editors; and whether in color.
type style struct {
	grouped bool
	color   bool
}

// styleFor picks how to print to out: grouped when it is a terminal, and
// in color as color, --color's value, says.
func styleFor(out io.Writer, color string) style {
	file, ok := out.(*os.File)
	return terminalStyle(ok && isTerminal(file), color, os.Getenv("NO_COLOR"))
}

// terminalStyle is the style for output to a terminal or not, with
// --color's value, always, never or auto, and NO_COLOR's. auto colors
// output to a terminal unless NO_COLOR has a value, which always, as a
// choice made for this run, overrides.
func terminalStyle(terminal bool, color, noColor string) style {
	s := style{grouped: terminal}
	switch color {
	case "always":
		s.color = true
	case "auto":
		s.color = terminal && noColor == ""
	}
	return s
}

// isTerminal reports whether file is a character device, as a terminal
// is. A pipe or a file, as when an IDE runs a hook, is not.
func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// SGR codes for the parts of a finding.
const (
	bold = "1"
	dim  = "2"
	red  = "31"
)

// report prints the findings in style s, then their summary.
func report(out io.Writer, findings []scan.Finding, s style) {
	if len(findings) == 0 {
		_, _ = fmt.Fprintln(out, summary(findings))
		return
	}
	if s.grouped {
		reportGrouped(out, findings, s.color)
	} else {
		reportLines(out, findings, s.color)
	}
	_, _ = fmt.Fprintf(out, "\n%s\n", summary(findings))
}

// summary says how many findings there are and in how many files, or that
// there are none.
func summary(findings []scan.Finding) string {
	if len(findings) == 0 {
		return "No debug statements found."
	}
	files := map[string]bool{}
	for _, f := range findings {
		files[f.Path] = true
	}
	return plural(len(findings), "debug statement", "debug statements") + " in " + plural(len(files), "file", "files")
}

// reportLines prints each finding as path:line: text [language/pattern],
// which editors and terminals can follow to the line. In color, the path
// is bold, the match red, and the line number and rule dim.
func reportLines(out io.Writer, findings []scan.Finding, color bool) {
	for _, f := range findings {
		before, match, after := excerpt(f.Text, f.Start, f.End)
		_, _ = fmt.Fprintf(out, "%s:%s: %s%s%s  %s\n",
			paint(cleanDisplay(f.Path), bold, color), paint(strconv.Itoa(f.Line), dim, color), before, paint(match, red, color), after, paint("["+ruleName(f)+"]", dim, color))
	}
}

// reportGrouped prints each file's path, then its findings under it, with
// their line numbers in a column. In color, the path is bold, the match
// red, and the line numbers and rules dim.
func reportGrouped(out io.Writer, findings []scan.Finding, color bool) {
	width := len(strconv.Itoa(slices.MaxFunc(findings, func(a, b scan.Finding) int { return cmp.Compare(a.Line, b.Line) }).Line))
	for i, f := range findings {
		if i == 0 || f.Path != findings[i-1].Path {
			if i > 0 {
				_, _ = fmt.Fprintln(out)
			}
			_, _ = fmt.Fprintln(out, paint(cleanDisplay(f.Path), bold, color))
		}
		before, match, after := excerpt(f.Text, f.Start, f.End)
		_, _ = fmt.Fprintf(out, "  %s  %s%s%s  %s\n",
			paint(fmt.Sprintf("%*d", width, f.Line), dim, color), before, paint(match, red, color), after, paint("["+ruleName(f)+"]", dim, color))
	}
}

// paint wraps s in an SGR code when color is on.
func paint(s, code string, color bool) string {
	if !color || s == "" {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func ruleName(f scan.Finding) string {
	return cleanDisplay(f.Rule.Language) + "/" + cleanDisplay(f.Rule.Name)
}

// excerpt trims a finding's line, whose match is the bytes from start to
// end, and splits it into the parts before, of and after the match. A line
// over maxLineLength characters is cut to a window around the match, with
// as much of the line before it as after, and an ellipsis where the line
// goes on. Control characters are replaced with spaces.
func excerpt(text string, start, end int) (before, match, after string) {
	lead := len(text) - len(strings.TrimLeftFunc(text, unicode.IsSpace))
	text = strings.TrimSpace(text)
	start = min(max(start-lead, 0), len(text))
	end = min(max(end-lead, start), len(text))
	runes := []rune(text)
	matchStart := utf8.RuneCountInString(text[:start])
	matchEnd := matchStart + utf8.RuneCountInString(text[start:end])
	from, to := 0, len(runes)
	if len(runes) > maxLineLength {
		from = max(matchStart-max(maxLineLength-(matchEnd-matchStart), 0)/2, 0)
		from = min(from, len(runes)-maxLineLength)
		to = from + maxLineLength
	}
	window := slices.Clone(runes[from:to])
	if from > 0 {
		window[0] = '…'
	}
	if to < len(runes) {
		window[len(window)-1] = '…'
	}
	matchStart = min(max(matchStart-from, 0), len(window))
	matchEnd = min(max(matchEnd-from, matchStart), len(window))
	return cleanDisplay(string(window[:matchStart])), cleanDisplay(string(window[matchStart:matchEnd])), cleanDisplay(string(window[matchEnd:]))
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
