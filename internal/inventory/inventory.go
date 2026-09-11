package inventory

import (
	"bufio"
	"context"
	"debug/buildinfo"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const Schema = 1

var cacheMu sync.Mutex

type Package struct {
	Provider string   `json:"provider"`
	Name     string   `json:"name"`
	Version  string   `json:"version,omitempty"`
	Commands []string `json:"commands"`
	Public   bool     `json:"public"`
	Stale    bool     `json:"stale,omitempty"`
}
type ProviderState struct {
	Name        string    `json:"name"`
	Fingerprint string    `json:"fingerprint"`
	RefreshedAt time.Time `json:"refreshed_at"`
	Stale       bool      `json:"stale"`
	Error       string    `json:"error,omitempty"`
	Packages    []Package `json:"packages,omitempty"`
}
type Cache struct {
	Schema    int                      `json:"schema"`
	Providers map[string]ProviderState `json:"providers"`
}
type Status struct {
	Provider    string `json:"provider"`
	AgeSeconds  int64  `json:"age_seconds"`
	Fingerprint string `json:"fingerprint"`
	Changed     bool   `json:"changed"`
	Stale       bool   `json:"stale"`
	Error       string `json:"error,omitempty"`
}

type Provider struct {
	Name        string
	Fingerprint func() string
	Discover    func(context.Context) ([]Package, error)
}

func DefaultProviders() []Provider {
	return []Provider{
		{Name: "homebrew", Fingerprint: func() string { return fingerprintPaths(brewRoots()) }, Discover: discoverBrew},
		{Name: "mise", Fingerprint: func() string { return fingerprintPaths([]string{filepath.Join(dataHome(), "mise", "installs")}) }, Discover: discoverMise},
		{Name: "uv", Fingerprint: func() string { return fingerprintPaths([]string{filepath.Join(dataHome(), "uv", "tools")}) }, Discover: discoverUV},
		{Name: "cargo", Fingerprint: func() string {
			return fingerprintPaths([]string{filepath.Join(cargoHome(), ".crates.toml"), filepath.Join(cargoHome(), "bin")})
		}, Discover: discoverCargo},
		{Name: "go", Fingerprint: func() string { return fingerprintPaths([]string{goBin()}) }, Discover: discoverGo},
		{Name: "path", Fingerprint: func() string { return fingerprintPaths(filepath.SplitList(os.Getenv("PATH"))) }, Discover: func(context.Context) ([]Package, error) { return nil, nil }},
		{Name: "apt", Fingerprint: func() string {
			return fingerprintPaths([]string{rooted("/var/lib/dpkg/status"), rooted("/var/lib/apt/extended_states")})
		}, Discover: func(context.Context) ([]Package, error) { return nil, nil }},
		{Name: "pacman", Fingerprint: func() string { return fingerprintPaths([]string{rooted("/var/lib/pacman/local")}) }, Discover: func(context.Context) ([]Package, error) { return nil, nil }},
	}
}

func Load(ctx context.Context, path string, refresh bool, providers []Provider) (Cache, []Status, error) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	unlock, err := lockCache(ctx, path+".lock")
	if err != nil {
		return Cache{}, nil, err
	}
	defer unlock()
	c := Cache{Schema: Schema, Providers: map[string]ProviderState{}}
	b, err := os.ReadFile(path)
	if err == nil {
		if json.Unmarshal(b, &c) != nil || c.Schema != Schema || c.Providers == nil {
			c = Cache{Schema: Schema, Providers: map[string]ProviderState{}}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return c, nil, err
	}
	now := time.Now()
	changed := false
	var statuses []Status
	for _, p := range providers {
		fp := p.Fingerprint()
		old, has := c.Providers[p.Name]
		needs := refresh || !has || old.Stale || old.Fingerprint != fp || now.Sub(old.RefreshedAt) > 24*time.Hour
		if needs {
			pkgs, e := p.Discover(ctx)
			changed = true
			if e != nil && has {
				old.Stale = true
				old.Error = e.Error()
				for i := range old.Packages {
					old.Packages[i].Stale = true
				}
				c.Providers[p.Name] = old
			} else if e != nil {
				c.Providers[p.Name] = ProviderState{Name: p.Name, Fingerprint: fp, Stale: true, Error: e.Error()}
			} else {
				c.Providers[p.Name] = ProviderState{Name: p.Name, Fingerprint: fp, RefreshedAt: now, Packages: pkgs}
			}
		}
		cur := c.Providers[p.Name]
		statuses = append(statuses, Status{Provider: p.Name, AgeSeconds: max(0, int64(now.Sub(cur.RefreshedAt).Seconds())), Fingerprint: cur.Fingerprint, Changed: needs, Stale: cur.Stale, Error: cur.Error})
	}
	if changed {
		if err := writeAtomic(path, c); err != nil {
			return c, statuses, err
		}
	}
	sort.Slice(statuses, func(i, j int) bool { return statuses[i].Provider < statuses[j].Provider })
	return c, statuses, nil
}

func writeAtomic(path string, c Cache) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	b, e := json.MarshalIndent(c, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".packages-*.tmp")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(append(b, '\n'))
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	return os.Rename(name, path)
}
func AllPackages(c Cache) []Package {
	var out []Package
	for _, s := range c.Providers {
		out = append(out, s.Packages...)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Provider == out[j].Provider {
			return out[i].Name < out[j].Name
		}
		return out[i].Provider < out[j].Provider
	})
	return out
}

func run(ctx context.Context, name string, args ...string) (string, error) {
	p, e := exec.LookPath(name)
	if e != nil {
		return "", nil
	}
	cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, p, args...)
	cmd.Stdin = nil
	cmd.Env = commandEnv()
	b, e := cmd.Output()
	if cctx.Err() != nil {
		return "", fmt.Errorf("%s timed out", name)
	}
	return string(b), e
}
func commandEnv() []string {
	blocked := map[string]bool{"PAGER": true, "GIT_PAGER": true, "NO_COLOR": true, "LC_ALL": true, "LANG": true}
	env := make([]string, 0, len(os.Environ())+5)
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if !blocked[key] {
			env = append(env, item)
		}
	}
	return append(env, "PAGER=cat", "GIT_PAGER=cat", "NO_COLOR=1", "LC_ALL=C", "LANG=C")
}
func discoverBrew(ctx context.Context) ([]Package, error) {
	out, e := run(ctx, "brew", "leaves")
	if e != nil {
		return nil, e
	}
	var result []Package
	for _, name := range strings.Fields(out) {
		prefix, _ := run(ctx, "brew", "--prefix", name)
		p := Package{Provider: "homebrew", Name: name}
		root := strings.TrimSpace(prefix)
		if versions, _ := run(ctx, "brew", "list", "--versions", name); versions != "" {
			if f := strings.Fields(versions); len(f) > 1 {
				p.Version = f[1]
			}
		}
		p.Commands = executables(filepath.Join(root, "bin"))
		if info, err := run(ctx, "brew", "info", "--json=v2", name); err == nil {
			var data struct {
				Formulae []struct {
					Tap string `json:"tap"`
				} `json:"formulae"`
			}
			if json.Unmarshal([]byte(info), &data) == nil && len(data.Formulae) == 1 && data.Formulae[0].Tap == "homebrew/core" {
				p.Public = true
			}
		}
		if len(p.Commands) > 0 {
			result = append(result, p)
		}
	}
	return result, nil
}
func discoverMise(ctx context.Context) ([]Package, error) {
	out, e := run(ctx, "mise", "ls", "--installed")
	if e != nil {
		return nil, e
	}
	var r []Package
	seen := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 || seen[f[0]] {
			continue
		}
		seen[f[0]] = true
		name := strings.TrimPrefix(f[0], "mise:")
		commands := miseCommands(name, f[1])
		if len(commands) == 0 {
			commands = []string{filepath.Base(name)}
		}
		r = append(r, Package{Provider: "mise", Name: name, Version: f[1], Commands: commands})
	}
	return r, nil
}
func miseCommands(name, version string) []string {
	root := filepath.Join(dataHome(), "mise", "installs")
	candidates := []string{filepath.Join(root, name, version, "bin"), filepath.Join(root, filepath.Base(name), version, "bin")}
	seen := map[string]bool{}
	var out []string
	for _, dir := range candidates {
		for _, cmd := range executables(dir) {
			if !seen[cmd] {
				seen[cmd] = true
				out = append(out, cmd)
			}
		}
	}
	sort.Strings(out)
	return out
}
func discoverUV(ctx context.Context) ([]Package, error) {
	out, e := run(ctx, "uv", "tool", "list")
	if e != nil {
		return nil, e
	}
	var r []Package
	var cur *Package
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(line), "- ") && cur != nil {
			entry := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "-"))
			if f := strings.Fields(entry); len(f) > 0 {
				cur.Commands = append(cur.Commands, strings.TrimSuffix(f[0], ","))
			}
		} else if line[0] != ' ' && line[0] != '\t' {
			if cur != nil {
				r = append(r, *cur)
			}
			f := strings.Fields(line)
			cur = &Package{Provider: "uv", Name: f[0]}
			if len(f) > 1 {
				cur.Version = strings.TrimPrefix(f[1], "v")
			}
		} else if cur != nil {
			entry := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "-"))
			if f := strings.Fields(entry); len(f) > 0 {
				cur.Commands = append(cur.Commands, strings.TrimSuffix(f[0], ","))
			}
		}
	}
	if cur != nil {
		r = append(r, *cur)
	}
	return r, nil
}
func discoverCargo(context.Context) ([]Package, error) {
	f, e := os.Open(filepath.Join(cargoHome(), ".crates.toml"))
	if errors.Is(e, os.ErrNotExist) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	defer f.Close()
	var r []Package
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if !strings.Contains(line, "=") || !strings.Contains(line, " ") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		coord := strings.Trim(strings.TrimSpace(parts[0]), "\"")
		cf := strings.Fields(coord)
		if len(cf) < 2 {
			continue
		}
		arr := strings.TrimSpace(parts[1])
		var cmds []string
		for _, x := range strings.Split(strings.Trim(arr, "[]"), ",") {
			x = strings.Trim(strings.TrimSpace(x), "\"")
			if x != "" {
				cmds = append(cmds, x)
			}
		}
		public := strings.Contains(coord, "registry+https://github.com/rust-lang/crates.io-index") || strings.Contains(coord, "registry+https://index.crates.io/")
		r = append(r, Package{Provider: "cargo", Name: cf[0], Version: cf[1], Commands: cmds, Public: public})
	}
	return r, s.Err()
}
func discoverGo(context.Context) ([]Package, error) {
	var r []Package
	for _, p := range executablePaths(goBin()) {
		bi, e := buildinfo.ReadFile(p)
		if e != nil || bi.Main.Path == "" {
			continue
		}
		r = append(r, Package{Provider: "go", Name: bi.Main.Path, Version: bi.Main.Version, Commands: []string{filepath.Base(p)}})
	}
	return r, nil
}

func ExactSystem(ctx context.Context, command, path string) []Package {
	var out []Package
	if s, e := run(ctx, "dpkg-query", "-S", path); e == nil && s != "" {
		name := strings.TrimSpace(strings.SplitN(s, ":", 2)[0])
		meta, _ := run(ctx, "dpkg-query", "-W", "-f=${Version} ${Essential} ${Priority}", name)
		f := strings.Fields(meta)
		if len(f) > 0 && !aptAutomatic(name) && (len(f) < 2 || f[1] != "yes") && (len(f) < 3 || (f[2] != "required" && f[2] != "important")) {
			out = append(out, Package{Provider: "apt", Name: name, Version: f[0], Commands: []string{command}})
		}
	}
	if s, e := run(ctx, "pacman", "-Qo", path); e == nil && strings.Contains(s, " is owned by ") {
		f := strings.Fields(s)
		if len(f) >= 5 {
			name := f[len(f)-2]
			info, _ := run(ctx, "pacman", "-Qi", name)
			if !fieldContains(info, "Groups", "base") && !fieldContains(info, "Install Reason", "dependency") {
				out = append(out, Package{Provider: "pacman", Name: name, Version: f[len(f)-1], Commands: []string{command}})
			}
		}
	}
	return out
}

func aptAutomatic(name string) bool {
	b, e := os.ReadFile(rooted("/var/lib/apt/extended_states"))
	if e != nil {
		return false
	}
	for _, block := range strings.Split(string(b), "\n\n") {
		if fieldContains(block, "Package", name) && fieldContains(block, "Auto-Installed", "1") {
			return true
		}
	}
	return false
}

func fieldContains(text, key, value string) bool {
	for _, line := range strings.Split(text, "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 && strings.TrimSpace(parts[0]) == key {
			for _, item := range strings.Fields(strings.TrimSpace(parts[1])) {
				if strings.EqualFold(item, value) {
					return true
				}
			}
		}
	}
	return false
}

func rooted(p string) string {
	if r := os.Getenv("WTF_TEST_ROOT"); r != "" {
		return filepath.Join(r, p)
	}
	return p
}
func dataHome() string {
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return v
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".local", "share")
}
func cargoHome() string {
	if v := os.Getenv("CARGO_HOME"); v != "" {
		return v
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".cargo")
}
func goBin() string {
	if v := os.Getenv("GOBIN"); v != "" {
		return v
	}
	if v := os.Getenv("GOPATH"); v != "" {
		return filepath.Join(strings.Split(v, string(os.PathListSeparator))[0], "bin")
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, "go", "bin")
}
func brewRoots() []string {
	if v := os.Getenv("HOMEBREW_CELLAR"); v != "" {
		return []string{v}
	}
	return []string{"/opt/homebrew/Cellar", "/usr/local/Cellar"}
}
func executables(p string) []string {
	var r []string
	for _, x := range executablePaths(p) {
		r = append(r, filepath.Base(x))
	}
	sort.Strings(r)
	return r
}
func executablePaths(p string) []string {
	e, _ := os.ReadDir(p)
	var r []string
	for _, x := range e {
		if x.IsDir() {
			continue
		}
		st, er := x.Info()
		if er == nil && st.Mode()&0111 != 0 {
			r = append(r, filepath.Join(p, x.Name()))
		}
	}
	return r
}
func fingerprintPaths(paths []string) string {
	var b strings.Builder
	sort.Strings(paths)
	for _, p := range paths {
		err := filepath.WalkDir(p, func(path string, d fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if path != p && d.IsDir() {
				return filepath.SkipDir
			}
			i, e := d.Info()
			if e == nil {
				fmt.Fprintf(&b, "%s:%d:%d;", path, i.ModTime().UnixNano(), i.Size())
			}
			return nil
		})
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(&b, "error:%s:%v;", p, err)
		}
	}
	return b.String()
}
