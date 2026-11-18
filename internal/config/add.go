package config

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

// AddLanguages adds the built-in languages named in langs to the config
// file at path, after the languages it has, leaving the rest of the file as
// it was. A symbolic link at path is followed, so the file it points to is
// changed. Nothing is changed when a language is already there, or when
// the file, as written, has no place to add one, such as a languages map
// written in flow style, between braces. With no file at path, the error
// wraps fs.ErrNotExist.
func AddLanguages(path string, langs []string) error {
	if len(langs) == 0 {
		return errors.New("no languages to add")
	}
	picked, err := pickPresets(langs)
	if err != nil {
		return err
	}
	file, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	old, err := parse(data)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	for _, p := range picked {
		if _, ok := old.Languages[p.name]; ok {
			return fmt.Errorf("%s already has %s", path, p.name)
		}
	}
	edited, err := insertPresets(string(data), picked)
	if err != nil {
		return fmt.Errorf("%s: %w; add %s by hand", path, err, strings.Join(langs, ", "))
	}
	if err := checkAdded(old, edited, picked); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return replaceFile(file, edited)
}

// documentMarker matches a line that starts or ends a YAML document.
var documentMarker = regexp.MustCompile(`^(---|\.\.\.)(\s|$)`)

// insertPresets returns a config file's content with picked added as the
// last of its languages. It works on the file's lines, using the parsed
// file only to find where its languages are, so everything else, its
// comments and blank lines included, is kept as it was.
func insertPresets(data string, picked []preset) (string, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(data), &doc); err != nil {
		return "", err
	}
	lines := strings.SplitAfter(data, "\n")
	newline := "\n"
	if strings.Contains(data, "\r\n") {
		newline = "\r\n"
	}

	root := documentRoot(&doc)
	if root == nil {
		// A file without settings, perhaps with comments: add them after it.
		return insertAt(lines, len(lines), "languages:\n"+presetEntries(picked, "  "), true, newline), nil
	}
	if root.Kind != yaml.MappingNode || root.Style&yaml.FlowStyle != 0 {
		return "", errors.New("the config is written in flow style, between braces")
	}
	rootIndent := strings.Repeat(" ", root.Content[0].Column-1)

	key, value := mapValue(root, "languages")
	if key == nil {
		at := blockEnd(lines, root.Content[0].Line-1, documentMarker.MatchString)
		text := rootIndent + "languages:\n" + presetEntries(picked, rootIndent+"  ")
		return insertAt(lines, at, text, true, newline), nil
	}
	keyIndent := key.Column - 1
	at := blockEnd(lines, key.Line-1, func(line string) bool { return indentOf(line) <= keyIndent })
	switch {
	case value.Kind == yaml.ScalarNode && value.Tag == "!!null" && value.Value == "":
		// languages: with nothing under it yet.
		return insertAt(lines, at, presetEntries(picked, rootIndent+"  "), false, newline), nil
	case value.Kind == yaml.MappingNode && value.Style&yaml.FlowStyle == 0 && len(value.Content) > 0:
		prefix := strings.Repeat(" ", value.Content[0].Column-1)
		return insertAt(lines, at, presetEntries(picked, prefix), true, newline), nil
	case value.Kind == yaml.MappingNode:
		return "", errors.New("its languages are written in flow style, between braces")
	default:
		return "", fmt.Errorf("languages is set to %s", value.Value)
	}
}

// documentRoot returns the first document's content, or nil when it has
// none.
func documentRoot(doc *yaml.Node) *yaml.Node {
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil
	}
	root := doc.Content[0]
	if root.Kind == yaml.ScalarNode && root.Tag == "!!null" {
		return nil
	}
	return root
}

// mapValue returns the key and value nodes for name in mapping, or nils.
func mapValue(mapping *yaml.Node, name string) (key, value *yaml.Node) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == name {
			return mapping.Content[i], mapping.Content[i+1]
		}
	}
	return nil, nil
}

// blockEnd returns the index of the line after the last line of the block
// that starts at lines[from], which runs up to the first line of content,
// neither blank nor a comment, that ends reports ends it. Comments ends
// reports would end it, such as those about whatever comes next, are left
// after it, and so are blank lines.
func blockEnd(lines []string, from int, ends func(line string) bool) int {
	end := from + 1
	for i := from + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			continue
		}
		if ends(lines[i]) {
			if !strings.HasPrefix(trimmed, "#") {
				break
			}
			continue
		}
		end = i + 1
	}
	return end
}

func indentOf(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}

// insertAt inserts text, whose lines end in "\n", before lines[at], after
// a blank line if spaced, and returns all the lines, joined, with
// newline ending the inserted ones.
func insertAt(lines []string, at int, text string, spaced bool, newline string) string {
	before := strings.Join(lines[:at], "")
	if before != "" && !strings.HasSuffix(before, "\n") {
		before += newline
	}
	if spaced && before != "" {
		text = "\n" + text
	}
	text = strings.ReplaceAll(text, "\n", newline)
	return before + text + strings.Join(lines[at:], "")
}

// checkAdded checks that edited, the config old was edited to, has picked
// as they are built in, and everything else as it was in old.
func checkAdded(old file, edited string, picked []preset) error {
	got, err := parse([]byte(edited))
	if err != nil {
		return fmt.Errorf("adding the languages would break the config (%w); add them by hand", err)
	}
	want := old
	want.Languages = maps.Clone(old.Languages)
	if want.Languages == nil {
		want.Languages = map[string]language{}
	}
	for _, p := range picked {
		want.Languages[p.name] = p.config
	}
	if !reflect.DeepEqual(got, want) {
		return errors.New("adding the languages would change the rest of the config; add them by hand")
	}
	_, err = compile(got, nil)
	return err
}

// replaceFile replaces the file at path with content, keeping its
// permissions, by renaming a new file over it, so a failed write leaves
// it as it was.
func replaceFile(path, content string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	// After the rename there is nothing left to remove; before it, this
	// cleans up a failed write.
	defer func() { _ = os.Remove(file.Name()) }()
	if _, err := file.WriteString(content); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Chmod(file.Name(), info.Mode().Perm()); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
