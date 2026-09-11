package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/wawrzdev/wtf/internal/inventory"
)

type docOptions struct {
	LocalOnly    bool
	CheatEnabled bool
	CacheDir     string
}

func compose(ctx context.Context, r Record, opt docOptions) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", r.ID)
	fmt.Fprintf(&b, "## Identity\n\n- Kind: %s\n- Availability: %s\n", r.Kind, map[bool]string{true: "available", false: "unavailable"}[r.Available])
	if r.Path != "" {
		fmt.Fprintf(&b, "- Path: `%s`\n", r.Path)
	}
	if r.Version != "" {
		fmt.Fprintf(&b, "- Version: %s\n", r.Version)
	}
	for _, p := range r.Packages {
		fmt.Fprintf(&b, "- Package: %s `%s`", p.Provider, p.Name)
		if p.Version != "" {
			fmt.Fprintf(&b, " %s", p.Version)
		}
		if p.Stale {
			fmt.Fprint(&b, " (stale cache)")
		}
		fmt.Fprintln(&b)
	}
	for _, p := range r.ShadowedPaths {
		fmt.Fprintf(&b, "- Shadowed executable: `%s`\n", p)
	}
	if r.Shell != nil {
		fmt.Fprintf(&b, "- Shell resolution: %s", r.Shell.Kind)
		if r.Shell.Expansion != "" {
			fmt.Fprintf(&b, " → `%s`", r.Shell.Expansion)
		}
		if r.Shell.Cycle {
			fmt.Fprint(&b, " (alias cycle)")
		}
		if r.Shell.Complex {
			fmt.Fprint(&b, " (complex expansion; target providers omitted)")
		}
		fmt.Fprintln(&b)
	} else {
		fmt.Fprintln(&b, "- Live shell resolution: unavailable (run `eval \"$(wtf init zsh)\"`)")
	}
	if a := r.Annotation; a != nil {
		fmt.Fprint(&b, "\n## Our annotation\n\n")
		if a.Summary != "" {
			fmt.Fprintln(&b, a.Summary)
			fmt.Fprintln(&b)
		}
		if a.Body != "" {
			fmt.Fprintln(&b, a.Body)
		}
	}
	if t := r.Topic; t != nil {
		fmt.Fprint(&b, "\n## Topic\n\n")
		fmt.Fprintln(&b, t.Body)
	}
	name := r.ID
	if r.Shell != nil && r.Shell.Target != "" && !r.Shell.Complex && !r.Shell.Cycle {
		name = r.Shell.Target
	}
	tldr, tldrErr := localTLDR(name)
	if tldr != "" {
		r.HasTLDR = true
		fmt.Fprintf(&b, "\n## tldr (local)\n\n%s\n", tldr)
	}
	if tldrErr != nil {
		fmt.Fprintf(&b, "\n## tldr (local)\n\nUnavailable: %v\n", tldrErr)
	}
	if !opt.LocalOnly && r.Available && r.Kind != "topic" && !(r.Shell != nil && (r.Shell.Kind == "function" || r.Shell.Complex || r.Shell.Cycle)) {
		if h, label := nativeHelp(ctx, r.Path, name); h != "" {
			fmt.Fprintf(&b, "\n## %s\n\n```text\n%s\n```\n", label, trimOutput(h, 24000))
		}
	}
	eligible := tldr != "" || hasPublicPackage(r.Packages)
	commandRecord := r.Kind != "topic" && r.Kind != "alias" && r.Kind != "func" && r.Kind != "function"
	if !opt.LocalOnly && opt.CheatEnabled && commandRecord && eligible && (r.Shell == nil || (r.Shell.Kind != "alias" && r.Shell.Kind != "function")) {
		text, stale, note := cheat(ctx, name, opt.CacheDir)
		if text != "" {
			label := "cheat.sh"
			if stale {
				label += " (stale cache)"
			}
			fmt.Fprintf(&b, "\n## %s\n\n%s\n", label, text)
		} else if note != "" {
			fmt.Fprintf(&b, "\n## cheat.sh\n\n%s\n", note)
		}
	}
	var sources []string
	if r.Annotation != nil {
		for _, s := range r.Annotation.Sources {
			sources = append(sources, fmt.Sprintf("%s:%d", s.Path, s.Line))
		}
	}
	if r.Topic != nil {
		for _, s := range r.Topic.Sources {
			sources = append(sources, fmt.Sprintf("%s:%d", s.Path, s.Line))
		}
	}
	if len(sources) > 0 {
		sort.Strings(sources)
		fmt.Fprint(&b, "\n## Sources\n\n")
		for _, s := range sources {
			fmt.Fprintf(&b, "- `%s`\n", s)
		}
	}
	return strings.TrimSpace(b.String()) + "\n"
}

func nativeHelp(ctx context.Context, path, name string) (string, string) {
	if man, e := exec.LookPath("man"); e == nil {
		if out, ok := runDoc(ctx, man, []string{name}, 2*time.Second); ok && strings.TrimSpace(out) != "" {
			if col, e := exec.LookPath("col"); e == nil {
				if cleaned, ok := pipeDoc(ctx, col, []string{"-bx"}, out); ok {
					out = cleaned
				}
			}
			return out, "Manual"
		}
	}
	if path == "" {
		return "", ""
	}
	if out, ok := runDoc(ctx, path, []string{"--help"}, 2*time.Second); ok {
		return out, name + " --help"
	}
	return "", ""
}
func runDoc(ctx context.Context, path string, args []string, timeout time.Duration) (string, bool) {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.Command(path, args...)
	cmd.Stdin = nil
	cmd.Env = docEnv()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if e := cmd.Start(); e != nil {
		return "", false
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case e := <-done:
		return out.String(), e == nil
	case <-cctx.Done():
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-done
		return out.String(), false
	}
}
func docEnv() []string {
	blocked := map[string]bool{"PAGER": true, "MANPAGER": true, "GIT_PAGER": true, "NO_COLOR": true, "TERM": true}
	env := make([]string, 0, len(os.Environ())+5)
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if !blocked[key] {
			env = append(env, item)
		}
	}
	return append(env, "PAGER=cat", "MANPAGER=cat", "GIT_PAGER=cat", "NO_COLOR=1", "TERM=dumb")
}
func pipeDoc(ctx context.Context, path string, args []string, input string) (string, bool) {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Stdin = strings.NewReader(input)
	b, e := cmd.Output()
	return string(b), e == nil
}

func localTLDR(name string) (string, error) {
	if name == "" || strings.Contains(name, "/") {
		return "", nil
	}
	h, _ := os.UserHomeDir()
	roots := localTLDRRoots(h)
	for _, root := range roots {
		if root == "" {
			continue
		}
		var found string
		walkErr := filepath.WalkDir(root, func(p string, d os.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if !d.IsDir() && d.Name() == name+".md" {
				found = p
				return errTLDRFound
			}
			return nil
		})
		if walkErr != nil && !errors.Is(walkErr, errTLDRFound) && !errors.Is(walkErr, os.ErrNotExist) {
			return "", walkErr
		}
		if found != "" {
			b, e := os.ReadFile(found)
			if e != nil {
				return "", e
			}
			return strings.TrimSpace(string(b)), nil
		}
	}
	return "", nil
}

var errTLDRFound = errors.New("tldr page found")

func localTLDRRoots(home string) []string {
	roots := []string{filepath.Join(home, ".cache", "tealdeer"), filepath.Join(home, ".local", "share", "tldr")}
	if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
		roots = append([]string{filepath.Join(xdg, "tealdeer")}, roots...)
	}
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		roots = append(roots, filepath.Join(xdg, "tldr"))
	}
	return roots
}

func localTLDRIDs() ([]string, error) {
	h, _ := os.UserHomeDir()
	seen := map[string]bool{}
	var ids []string
	for _, root := range localTLDRRoots(h) {
		if root == "" {
			continue
		}
		err := filepath.WalkDir(root, func(_ string, d os.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if d.IsDir() || filepath.Ext(d.Name()) != ".md" {
				return nil
			}
			id := strings.TrimSuffix(d.Name(), ".md")
			if safeCommandID(id) && !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
			return nil
		})
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return ids, err
		}
	}
	sort.Strings(ids)
	return ids, nil
}

func hasPublicPackage(packages []inventory.Package) bool {
	for _, p := range packages {
		if p.Public {
			return true
		}
	}
	return false
}

var cheatBaseURL = "https://cheat.sh/"
var cheatHTTPClient = &http.Client{Timeout: 4 * time.Second}

func cheat(ctx context.Context, name, cacheDir string) (string, bool, string) {
	key := sha256.Sum256([]byte(name))
	path := filepath.Join(cacheDir, "cheat", hex.EncodeToString(key[:])+".txt")
	if b, e := os.ReadFile(path); e == nil {
		if st, e := os.Stat(path); e == nil && time.Since(st.ModTime()) < 7*24*time.Hour {
			return string(b), false, ""
		}
	}
	old, _ := os.ReadFile(path)
	u := cheatBaseURL + url.PathEscape(name) + "?T"
	req, e := http.NewRequestWithContext(ctx, "GET", u, nil)
	if e == nil {
		var resp *http.Response
		resp, e = cheatHTTPClient.Do(req)
		if e == nil {
			defer resp.Body.Close()
			if resp.StatusCode == 200 {
				var b []byte
				b, e = io.ReadAll(io.LimitReader(resp.Body, 1<<20))
				if e == nil && len(bytes.TrimSpace(b)) > 0 {
					_ = os.MkdirAll(filepath.Dir(path), 0700)
					_ = os.WriteFile(path, b, 0600)
					return string(b), false, ""
				}
			} else {
				e = fmt.Errorf("HTTP %s", resp.Status)
			}
		}
	}
	if len(old) > 0 {
		return string(old), true, ""
	}
	if e != nil {
		return "", false, "Unavailable: " + e.Error()
	}
	return "", false, "Unavailable"
}
func trimOutput(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n] + "\n…"
	}
	return s
}
