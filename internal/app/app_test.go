package app

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	goRuntime "runtime"
	"strings"
	"testing"
	"time"

	"github.com/wawrzdev/wtf/internal/content"
	"github.com/wawrzdev/wtf/internal/inventory"
)

func TestStableListAndQuery(t *testing.T) {
	h := setup(t)
	bin := filepath.Join(h, "bin")
	mustFile(t, filepath.Join(bin, "rg"), "#!/bin/sh\necho rg help\n", 0700)
	mustFile(t, filepath.Join(h, "data", "wtf", "annotations", "rg.md"), "---\nschema: 1\nid: rg\nkind: command\nsummary: Fast search\n---\n\n# rg body\n", 0600)
	var out, err bytes.Buffer
	if e := Run(context.Background(), []string{"list"}, strings.NewReader(""), &out, &err, "test"); e != nil {
		t.Fatal(e)
	}
	if got := out.String(); got != "command\trg\tavailable\tFast search\n" {
		t.Fatalf("list %q stderr %q", got, err.String())
	}
	out.Reset()
	if e := Run(context.Background(), []string{"rg"}, strings.NewReader(""), &out, &err, "test"); e != nil {
		t.Fatal(e)
	}
	for _, want := range []string{"# rg", "Fast search", "# rg body", "Live shell resolution: unavailable"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in %s", want, out.String())
		}
	}
}
func TestUnavailableAllAndJSON(t *testing.T) {
	h := setup(t)
	mustFile(t, filepath.Join(h, "data", "wtf", "annotations", "gone.md"), "---\nschema: 1\nid: gone\nkind: command\nsummary: Missing\n---\n", 0600)
	var o, e bytes.Buffer
	if err := Run(context.Background(), []string{"list"}, nil, &o, &e, "x"); err != nil {
		t.Fatal(err)
	}
	if o.Len() != 0 {
		t.Fatalf("default included unavailable: %q", o.String())
	}
	if err := Run(context.Background(), []string{"list", "-a", "--json"}, nil, &o, &e, "x"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(o.String(), `"available": false`) {
		t.Fatal(o.String())
	}
}
func TestExplicitPathAndShellComplex(t *testing.T) {
	h := setup(t)
	p := filepath.Join(h, "odd tool")
	mustFile(t, p, "#!/bin/sh\nexit 1\n", 0700)
	var o, e bytes.Buffer
	if err := Run(context.Background(), []string{p}, nil, &o, &e, "x"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(o.String(), p) {
		t.Fatal(o.String())
	}
	o.Reset()
	mustFile(t, filepath.Join(h, "data", "wtf", "annotations", "ll.md"), "# @alias ll — list\n", 0600)
	if err := Run(context.Background(), []string{"--shell-kind", "alias", "ll", "ls | less", "ll"}, nil, &o, &e, "x"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(o.String(), "complex expansion") {
		t.Fatal(o.String())
	}
}
func TestCheatEligibilityCacheAndOffline(t *testing.T) {
	calls := 0
	old := cheatBaseURL
	oldClient := cheatHTTPClient
	cheatBaseURL = "https://local.invalid/"
	cheatHTTPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("public cheat")), Header: make(http.Header)}, nil
	})}
	defer func() { cheatBaseURL = old; cheatHTTPClient = oldClient }()
	cache := t.TempDir()
	x := Record{ID: "rg", Kind: "command", Packages: []inventory.Package{{Provider: "x", Name: "ripgrep"}}}
	doc := compose(context.Background(), x, docOptions{CheatEnabled: true, CacheDir: cache})
	if !strings.Contains(doc, "public cheat") || calls != 1 {
		t.Fatal(doc, calls)
	}
	cheatHTTPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, os.ErrNotExist })}
	doc = compose(context.Background(), x, docOptions{CheatEnabled: true, CacheDir: cache})
	if !strings.Contains(doc, "public cheat") || calls != 1 {
		t.Fatal(doc, calls)
	}
	calls = 0
	ann := Record{ID: "private-name", Kind: "command", Annotation: &content.Entry{ID: "private-name"}}
	_ = compose(context.Background(), ann, docOptions{CheatEnabled: true, CacheDir: cache})
	if calls != 0 {
		t.Fatal("annotation-only contacted server")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestNativeHelpTimeoutAndFailure(t *testing.T) {
	if goRuntime.GOOS == "windows" {
		t.Skip()
	}
	d := t.TempDir()
	slow := filepath.Join(d, "slow")
	mustFile(t, slow, "#!/bin/sh\nsleep 10\n", 0700)
	start := time.Now()
	out, _ := nativeHelp(context.Background(), slow, "definitely-no-such-manpage")
	if time.Since(start) > 4*time.Second {
		t.Fatal("help did not time out")
	}
	if out != "" {
		t.Fatalf("unexpected %q", out)
	}
}
func TestTopicNewHookAndNoChange(t *testing.T) {
	h := setup(t)
	log := filepath.Join(h, "hook.log")
	editor := filepath.Join(h, "editor")
	hook := filepath.Join(h, "hook")
	mustFile(t, editor, "#!/bin/sh\nprintf '\\nEdited.\\n' >> \"$1\"\n", 0700)
	mustFile(t, hook, "#!/bin/sh\nprintf '%s' \"$1\" > \"$HOOK_LOG\"\n", 0700)
	t.Setenv("EDITOR", editor)
	t.Setenv("HOOK_LOG", log)
	cfg := filepath.Join(h, "config", "wtf", "config.toml")
	mustFile(t, cfg, "[annotations]\npaths = []\n[topics]\ndirs = [\""+filepath.Join(h, "data", "wtf", "topics")+"\"]\nwrite_dir = \""+filepath.Join(h, "data", "wtf", "topics")+"\"\npost_write = [\""+hook+"\", \"{path}\"]\n[providers.cheat]\nenabled = false\n", 0600)
	var o, e bytes.Buffer
	if err := Run(context.Background(), []string{"topic", "new", "Git staging"}, nil, &o, &e, "x"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(log)
	if err != nil || !strings.HasSuffix(string(b), "git-staging.md") {
		t.Fatalf("hook %q %v", b, err)
	}
}
func TestInitContainsNoEvalOfMetadata(t *testing.T) {
	var o bytes.Buffer
	if err := initCommand([]string{"zsh"}, &o); err != nil {
		t.Fatal(err)
	}
	s := o.String()
	if strings.Contains(s, "eval $_wtf") || !strings.Contains(s, "command wtf --shell-kind") {
		t.Fatal(s)
	}
}

func TestInitializedZshAliasCycleProtocol(t *testing.T) {
	zsh, e := exec.LookPath("zsh")
	if e != nil {
		t.Skip("zsh unavailable")
	}
	d := t.TempDir()
	initPath := filepath.Join(d, "init.zsh")
	mustFile(t, initPath, zshInit, 0600)
	stub := filepath.Join(d, "wtf")
	mustFile(t, stub, "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$CAPTURE\"\n", 0700)
	capture := filepath.Join(d, "args")
	cmd := exec.Command(zsh, "-fc", "source \"$INIT\"; alias aa=bb; alias bb=aa; wtf aa")
	cmd.Env = append(os.Environ(), "PATH="+d+":"+os.Getenv("PATH"), "INIT="+initPath, "CAPTURE="+capture)
	if b, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("zsh: %v: %s", e, b)
	}
	b, e := os.ReadFile(capture)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(b), "--shell-cycle") {
		t.Fatalf("protocol %q", b)
	}
}

func TestPickerUsesNativeKeysAndLocalPreview(t *testing.T) {
	d := t.TempDir()
	capture := filepath.Join(d, "args")
	fzf := filepath.Join(d, "fzf")
	mustFile(t, fzf, "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$CAPTURE\"\nexit 130\n", 0700)
	t.Setenv("PATH", d+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CAPTURE", capture)
	r := runtime{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
	if e := r.picker(context.Background(), []Record{{ID: "rg", Kind: "command"}}, ""); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(capture)
	if e != nil {
		t.Fatal(e)
	}
	s := string(b)
	if strings.Contains(s, "--bind") || !strings.Contains(s, "--local-only") {
		t.Fatalf("fzf args %q", s)
	}
}
func setup(t *testing.T) string {
	t.Helper()
	h := t.TempDir()
	for _, x := range []string{"config", "data", "cache", "bin"} {
		if e := os.MkdirAll(filepath.Join(h, x), 0700); e != nil {
			t.Fatal(e)
		}
	}
	t.Setenv("HOME", h)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(h, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(h, "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(h, "cache"))
	t.Setenv("PATH", filepath.Join(h, "bin"))
	mustFile(t, filepath.Join(h, "config", "wtf", "config.toml"), "[annotations]\npaths = [\""+filepath.Join(h, "data", "wtf", "annotations")+"\"]\n[topics]\ndirs = [\""+filepath.Join(h, "data", "wtf", "topics")+"\"]\nwrite_dir = \""+filepath.Join(h, "data", "wtf", "topics")+"\"\npost_write = []\n[providers.cheat]\nenabled = false\n", 0600)
	return h
}
func mustFile(t *testing.T, p, s string, mode os.FileMode) {
	t.Helper()
	if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(p, []byte(s), mode); e != nil {
		t.Fatal(e)
	}
}
