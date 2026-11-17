package scan

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// addedLine is a line a diff adds, with where it is in the new file.
type addedLine struct {
	path string // slash-separated, from the repository root
	line int
	text string // without its ending
}

var hunkHeader = regexp.MustCompile(`^@@ -\d+(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// addedLines reads the lines a diff adds, from git diff run with a/ and b/
// as its prefixes. A hunk ends when its line counts are used up rather
// than at the next line that looks like a header, as an added line can
// start "+++" and a removed one "---".
func addedLines(diff string) ([]addedLine, error) {
	var added []addedLine
	var path string // the file being read, or "" for one whose lines are skipped
	var oldLeft, newLeft, next int
	for line := range strings.Lines(diff) {
		line = strings.TrimSuffix(line, "\n")
		if oldLeft > 0 || newLeft > 0 {
			switch {
			case strings.HasPrefix(line, "+"):
				if path != "" {
					added = append(added, addedLine{path: path, line: next, text: strings.TrimSuffix(line[1:], "\r")})
				}
				next++
				newLeft--
			case strings.HasPrefix(line, "-"):
				oldLeft--
			case strings.HasPrefix(line, " ") || line == "":
				next++
				oldLeft--
				newLeft--
			case !strings.HasPrefix(line, `\`): // "\ No newline at end of file" follows a line of either side.
				return nil, fmt.Errorf("unexpected line in a hunk of %s: %q", path, line)
			}
			continue
		}
		switch {
		case strings.HasPrefix(line, "diff "):
			path = ""
		case strings.HasPrefix(line, "+++ "):
			var err error
			if path, err = newFilePath(line[len("+++ "):]); err != nil {
				return nil, err
			}
		case strings.HasPrefix(line, "@@ "):
			var err error
			if oldLeft, next, newLeft, err = parseHunkHeader(line); err != nil {
				return nil, err
			}
		}
	}
	if oldLeft > 0 || newLeft > 0 {
		return nil, fmt.Errorf("the diff of %s ended mid-hunk", path)
	}
	return added, nil
}

// newFilePath reads the path on a "+++" line: "b/" and the path, quoted
// as C quotes a string when it holds a quote, a backslash or a control
// character, and followed by a tab when it holds a space. It is "" for
// /dev/null, a deleted file.
func newFilePath(name string) (string, error) {
	name = strings.TrimSuffix(name, "\t")
	if strings.HasPrefix(name, `"`) {
		unquoted, err := strconv.Unquote(name)
		if err != nil {
			return "", fmt.Errorf("unreadable path in diff: %s", name)
		}
		name = unquoted
	}
	if name == "/dev/null" {
		return "", nil
	}
	path, ok := strings.CutPrefix(name, "b/")
	if !ok {
		return "", fmt.Errorf("unexpected path in diff: %q", name)
	}
	return path, nil
}

// parseHunkHeader reads a header such as "@@ -3,0 +4,2 @@": how many old
// and new lines the hunk has, and the new file's line it starts at. A
// count left out is 1.
func parseHunkHeader(line string) (oldCount, newStart, newCount int, err error) {
	match := hunkHeader.FindStringSubmatch(line)
	if match == nil {
		return 0, 0, 0, fmt.Errorf("unexpected hunk header: %q", line)
	}
	numbers := make([]int, 3)
	for i, s := range match[1:] {
		if s == "" {
			numbers[i] = 1
		} else if numbers[i], err = strconv.Atoi(s); err != nil {
			return 0, 0, 0, fmt.Errorf("unexpected hunk header: %q: %w", line, err)
		}
	}
	return numbers[0], numbers[1], numbers[2], nil
}
