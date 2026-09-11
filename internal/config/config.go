package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type Config struct {
	Annotations struct{ Paths []string }
	Topics      struct {
		Dirs      []string
		WriteDir  string
		PostWrite []string
	}
	Cheat   struct{ Enabled bool }
	Pager   string
	Sources []string
}

type fragment struct {
	path     string
	priority *int
	values   map[string]any
}

func Paths() (configDir, dataDir, cacheDir string, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", "", err
	}
	configDir = envPath("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	dataDir = envPath("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	cacheDir = envPath("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	return
}

func envPath(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func Load() (Config, error) {
	configHome, dataHome, _, err := Paths()
	if err != nil {
		return Config{}, err
	}
	base := filepath.Join(configHome, "wtf", "config.toml")
	c := Config{}
	c.Cheat.Enabled = true
	c.Topics.Dirs = []string{filepath.Join(dataHome, "wtf", "topics")}
	c.Topics.WriteDir = filepath.Join(dataHome, "wtf", "topics")
	c.Annotations.Paths = []string{filepath.Join(dataHome, "wtf", "annotations")}
	fragments := []fragment{}
	if _, err := os.Stat(base); err == nil {
		f, err := parse(base)
		if err != nil {
			return c, err
		}
		fragments = append(fragments, f)
	} else if !errors.Is(err, os.ErrNotExist) {
		return c, err
	}
	matches, err := filepath.Glob(filepath.Join(configHome, "wtf", "conf.d", "*.toml"))
	if err != nil {
		return c, err
	}
	seen := map[int]string{}
	extra := make([]fragment, 0, len(matches))
	for _, path := range matches {
		f, err := parse(path)
		if err != nil {
			return c, err
		}
		if f.priority == nil {
			return c, fmt.Errorf("%s: fragment requires numeric priority", path)
		}
		if prev, ok := seen[*f.priority]; ok {
			return c, fmt.Errorf("duplicate fragment priority %d in %s and %s", *f.priority, prev, path)
		}
		seen[*f.priority] = path
		extra = append(extra, f)
	}
	sort.Slice(extra, func(i, j int) bool { return *extra[i].priority < *extra[j].priority })
	fragments = append(fragments, extra...)
	origins := map[string]string{}
	for _, f := range fragments {
		if err := apply(&c, f); err != nil {
			return c, err
		}
		for key := range f.values {
			origins[key] = f.path
		}
		c.Sources = append(c.Sources, f.path)
	}
	if err := validate(c); err != nil {
		key := ""
		switch {
		case strings.Contains(err.Error(), "topics.dirs"):
			key = "topics.dirs"
		case strings.Contains(err.Error(), "topics.write_dir"):
			key = "topics.write_dir"
		case strings.Contains(err.Error(), "topics.post_write"):
			key = "topics.post_write"
		case strings.Contains(err.Error(), "annotations"):
			key = "annotations.paths"
		}
		if source := origins[key]; source != "" {
			return c, fmt.Errorf("%s: %w", source, err)
		}
		return c, err
	}
	return c, nil
}

func parse(path string) (fragment, error) {
	f, err := os.Open(path)
	if err != nil {
		return fragment{}, err
	}
	defer f.Close()
	r := fragment{path: path, values: map[string]any{}}
	section := ""
	s := bufio.NewScanner(f)
	var pending, pendingKey string
	for line := 1; s.Scan(); line++ {
		raw := strings.TrimSpace(stripComment(s.Text()))
		if pending != "" {
			pending += " " + raw
			if strings.Contains(raw, "]") {
				v, e := parseArray(pending)
				if e != nil {
					return r, fmt.Errorf("%s:%d: %w", path, line, e)
				}
				r.values[pendingKey] = v
				pending = ""
			}
			continue
		}
		if raw == "" {
			continue
		}
		if strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]") {
			section = strings.TrimSpace(raw[1 : len(raw)-1])
			continue
		}
		parts := strings.SplitN(raw, "=", 2)
		if len(parts) != 2 {
			return r, fmt.Errorf("%s:%d: expected key = value", path, line)
		}
		key := strings.TrimSpace(parts[0])
		if section != "" {
			key = section + "." + key
		}
		val := strings.TrimSpace(parts[1])
		if strings.HasPrefix(val, "[") && !strings.Contains(val, "]") {
			pending = val
			pendingKey = key
			continue
		}
		v, e := parseValue(val)
		if e != nil {
			return r, fmt.Errorf("%s:%d: %w", path, line, e)
		}
		r.values[key] = v
	}
	if err := s.Err(); err != nil {
		return r, err
	}
	if pending != "" {
		return r, fmt.Errorf("%s: unterminated array", path)
	}
	if v, ok := r.values["priority"]; ok {
		n, ok := v.(int)
		if !ok {
			return r, fmt.Errorf("%s: priority must be numeric", path)
		}
		r.priority = &n
		delete(r.values, "priority")
	}
	return r, nil
}

func stripComment(s string) string {
	in := false
	for i, r := range s {
		if r == '"' {
			in = !in
		}
		if r == '#' && !in {
			return s[:i]
		}
	}
	return s
}
func parseValue(v string) (any, error) {
	if strings.HasPrefix(v, "[") {
		return parseArray(v)
	}
	if v == "true" {
		return true, nil
	}
	if v == "false" {
		return false, nil
	}
	if n, e := strconv.Atoi(v); e == nil {
		return n, nil
	}
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		q, e := strconv.Unquote(v)
		return q, e
	}
	return nil, fmt.Errorf("unsupported TOML value %q", v)
}
func parseArray(v string) ([]string, error) {
	if !strings.HasPrefix(v, "[") || !strings.HasSuffix(v, "]") {
		return nil, errors.New("invalid array")
	}
	v = strings.TrimSpace(v[1 : len(v)-1])
	if v == "" {
		return []string{}, nil
	}
	var out []string
	var cur strings.Builder
	quoted := false
	esc := false
	flush := func() error {
		x := strings.TrimSpace(cur.String())
		cur.Reset()
		if x == "" {
			return nil
		}
		q, e := strconv.Unquote(x)
		if e != nil {
			return e
		}
		out = append(out, q)
		return nil
	}
	for _, r := range v {
		if esc {
			cur.WriteRune(r)
			esc = false
			continue
		}
		if r == '\\' && quoted {
			cur.WriteRune(r)
			esc = true
			continue
		}
		if r == '"' {
			quoted = !quoted
			cur.WriteRune(r)
			continue
		}
		if r == ',' && !quoted {
			if e := flush(); e != nil {
				return nil, e
			}
			continue
		}
		cur.WriteRune(r)
	}
	if quoted {
		return nil, errors.New("unterminated string")
	}
	if e := flush(); e != nil {
		return nil, e
	}
	return out, nil
}

func apply(c *Config, f fragment) error {
	for k, v := range f.values {
		var ok bool
		switch k {
		case "annotations.paths":
			c.Annotations.Paths, ok = v.([]string)
		case "topics.dirs":
			c.Topics.Dirs, ok = v.([]string)
		case "topics.write_dir":
			c.Topics.WriteDir, ok = v.(string)
		case "topics.post_write":
			c.Topics.PostWrite, ok = v.([]string)
		case "providers.cheat.enabled":
			c.Cheat.Enabled, ok = v.(bool)
		case "pager":
			c.Pager, ok = v.(string)
		default:
			return fmt.Errorf("%s: unknown configuration key %q", f.path, k)
		}
		if !ok {
			return fmt.Errorf("%s: invalid value for %s", f.path, k)
		}
	}
	for i, p := range c.Annotations.Paths {
		c.Annotations.Paths[i] = expand(p)
	}
	for i, p := range c.Topics.Dirs {
		c.Topics.Dirs[i] = expand(p)
	}
	c.Topics.WriteDir = expand(c.Topics.WriteDir)
	return nil
}
func expand(s string) string {
	home, _ := os.UserHomeDir()
	if s == "~" {
		s = home
	} else if strings.HasPrefix(s, "~/") {
		s = filepath.Join(home, s[2:])
	}
	return os.Expand(s, func(k string) string {
		switch k {
		case "HOME":
			return home
		case "XDG_CONFIG_HOME":
			v, _, _, _ := Paths()
			return v
		case "XDG_DATA_HOME":
			_, v, _, _ := Paths()
			return v
		case "XDG_CACHE_HOME":
			_, _, v, _ := Paths()
			return v
		}
		return os.Getenv(k)
	})
}
func validate(c Config) error {
	if len(c.Topics.Dirs) == 0 {
		return errors.New("topics.dirs must not be empty")
	}
	if c.Topics.WriteDir == "" {
		return errors.New("topics.write_dir must not be empty")
	}
	if !filepath.IsAbs(c.Topics.WriteDir) {
		return fmt.Errorf("topics.write_dir is not absolute: %s", c.Topics.WriteDir)
	}
	for _, p := range c.Annotations.Paths {
		if !filepath.IsAbs(p) {
			return fmt.Errorf("annotations.paths contains a path that is not absolute: %s", p)
		}
	}
	for _, p := range c.Topics.Dirs {
		if !filepath.IsAbs(p) {
			return fmt.Errorf("topics.dirs contains a path that is not absolute: %s", p)
		}
	}
	for _, a := range c.Topics.PostWrite {
		if a == "" {
			return errors.New("topics.post_write contains an empty argument")
		}
	}
	return nil
}
