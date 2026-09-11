package content

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type Source struct {
	Path string `json:"path"`
	Line int    `json:"line"`
}
type Entry struct {
	Schema  int      `json:"schema"`
	ID      string   `json:"id"`
	Kind    string   `json:"kind"`
	Title   string   `json:"title,omitempty"`
	Summary string   `json:"summary,omitempty"`
	Body    string   `json:"body,omitempty"`
	Sources []Source `json:"sources"`
	fields  map[string]bool
}

func Load(annotationPaths, topicDirs []string) (map[string]Entry, map[string]Entry, []error) {
	anns := map[string]Entry{}
	topics := map[string]Entry{}
	var errs []error
	for _, p := range annotationPaths {
		entries, e := readPath(p, false)
		errs = append(errs, e...)
		for _, x := range entries {
			overlay(anns, x)
		}
	}
	for _, p := range topicDirs {
		entries, e := readPath(p, true)
		errs = append(errs, e...)
		for _, x := range entries {
			overlay(topics, x)
		}
	}
	for id, x := range anns {
		if err := Validate(x); err != nil {
			errs = append(errs, fmt.Errorf("annotation %s: %w", id, err))
			delete(anns, id)
		}
	}
	for id, x := range topics {
		if err := Validate(x); err != nil || x.Kind != "topic" {
			if err == nil {
				err = errors.New("kind must be topic")
			}
			errs = append(errs, fmt.Errorf("topic %s: %w", id, err))
			delete(topics, id)
		}
	}
	return anns, topics, errs
}

func readPath(path string, topics bool) ([]Entry, []error) {
	st, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}
	if !st.IsDir() {
		x, e := ParseFile(path, topics)
		return x, errorSlice(e)
	}
	var files []string
	filepath.WalkDir(path, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if !d.IsDir() {
			files = append(files, p)
		}
		return nil
	})
	sort.Strings(files)
	var out []Entry
	var errs []error
	for _, p := range files {
		x, e := ParseFile(p, topics)
		if e != nil {
			errs = append(errs, e)
		} else {
			out = append(out, x...)
		}
	}
	return out, errs
}
func errorSlice(e error) []error {
	if e == nil {
		return nil
	}
	return []error{e}
}

func ParseFile(path string, expectTopic bool) ([]Entry, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	s := string(b)
	if strings.HasPrefix(s, "---\n") || strings.HasPrefix(s, "---\r\n") {
		x, e := parseMarkdown(path, s)
		if e != nil {
			return nil, e
		}
		if expectTopic && x.Kind != "topic" {
			return nil, fmt.Errorf("%s: topics must have kind: topic", path)
		}
		return []Entry{x}, nil
	}
	if expectTopic {
		return nil, fmt.Errorf("%s: topic is missing frontmatter", path)
	}
	return parseInline(path, s), nil
}

func parseMarkdown(path, s string) (Entry, error) {
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	end := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return Entry{}, fmt.Errorf("%s: unterminated frontmatter", path)
	}
	x := Entry{Sources: []Source{{Path: path, Line: 1}}, fields: map[string]bool{}}
	for i := 1; i < end; i++ {
		p := strings.SplitN(lines[i], ":", 2)
		if len(p) != 2 {
			return x, fmt.Errorf("%s:%d: invalid frontmatter", path, i+1)
		}
		k, v := strings.TrimSpace(p[0]), strings.Trim(strings.TrimSpace(p[1]), "\"")
		x.fields[k] = true
		switch k {
		case "schema":
			n, e := strconv.Atoi(v)
			if e != nil {
				return x, fmt.Errorf("%s:%d: invalid schema", path, i+1)
			}
			x.Schema = n
		case "id":
			x.ID = v
		case "kind":
			x.Kind = v
		case "title":
			x.Title = v
		case "summary":
			x.Summary = v
		default:
			return x, fmt.Errorf("%s:%d: unknown frontmatter field %q", path, i+1, k)
		}
	}
	x.Body = strings.TrimSpace(strings.Join(lines[end+1:], "\n"))
	if e := validatePartial(x); e != nil {
		return x, fmt.Errorf("%s: %w", path, e)
	}
	return x, nil
}

func Validate(x Entry) error {
	if x.Schema != 1 {
		return fmt.Errorf("schema must be 1")
	}
	if !validID(x.ID) {
		return fmt.Errorf("invalid id %q", x.ID)
	}
	if x.Kind != "command" && x.Kind != "topic" && x.Kind != "tool" && x.Kind != "func" && x.Kind != "alias" {
		return fmt.Errorf("invalid kind %q", x.Kind)
	}
	return nil
}
func validatePartial(x Entry) error {
	if x.Schema != 0 && x.Schema != 1 {
		return errors.New("schema must be 1 when supplied")
	}
	if !validID(x.ID) {
		return fmt.Errorf("invalid id %q", x.ID)
	}
	if x.Kind != "" && x.Kind != "command" && x.Kind != "topic" {
		return fmt.Errorf("invalid kind %q", x.Kind)
	}
	return nil
}
func validID(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}

func parseInline(path, s string) []Entry {
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	var out []Entry
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "# @") {
			continue
		}
		rest := strings.TrimPrefix(line, "# @")
		kind, rest, ok := strings.Cut(rest, " ")
		if !ok {
			continue
		}
		if kind == "detail" {
			id := strings.TrimSpace(rest)
			var body []string
			for j := i + 1; j < len(lines); j++ {
				v := lines[j]
				if strings.HasPrefix(strings.TrimSpace(v), "# @") {
					break
				}
				t := strings.TrimSpace(v)
				if !strings.HasPrefix(t, "#") {
					break
				}
				t = strings.TrimSpace(strings.TrimPrefix(t, "#"))
				body = append(body, t)
				i = j
			}
			if validID(id) {
				out = append(out, Entry{Schema: 1, ID: id, Body: strings.TrimSpace(strings.Join(body, "\n")), Sources: []Source{{path, i - len(body) + 1}}})
			}
			continue
		}
		if kind != "tool" && kind != "func" && kind != "alias" {
			continue
		}
		idSummary := strings.SplitN(rest, " — ", 2)
		id := strings.TrimSpace(strings.Fields(idSummary[0])[0])
		summary := ""
		if len(idSummary) == 2 {
			summary = strings.TrimSpace(idSummary[1])
		}
		if validID(id) {
			fields := map[string]bool{"schema": true, "id": true, "kind": true}
			if len(idSummary) == 2 {
				fields["summary"] = true
			}
			out = append(out, Entry{Schema: 1, ID: id, Kind: kind, Summary: summary, Sources: []Source{{path, i + 1}}, fields: fields})
		}
	}
	return out
}

func overlay(dst map[string]Entry, next Entry) {
	cur, ok := dst[next.ID]
	if !ok {
		dst[next.ID] = next
		return
	}
	if next.fields["schema"] {
		cur.Schema = next.Schema
	}
	if next.fields["kind"] {
		cur.Kind = next.Kind
	}
	if next.fields["title"] {
		cur.Title = next.Title
	}
	if next.fields["summary"] {
		cur.Summary = next.Summary
	}
	if next.Body != "" {
		cur.Body = next.Body
	}
	cur.Sources = append(cur.Sources, next.Sources...)
	dst[next.ID] = cur
}

func Slug(title string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(strings.TrimSpace(title)) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			dash = false
		} else {
			dash = true
		}
	}
	return strings.Trim(b.String(), "-")
}
