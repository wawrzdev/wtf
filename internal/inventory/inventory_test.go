package inventory

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestProviderFixturesHomebrewMiseUV(t *testing.T) {
	d := t.TempDir()
	bin := filepath.Join(d, "bin")
	if e := os.MkdirAll(bin, 0700); e != nil {
		t.Fatal(e)
	}
	brewPrefix := filepath.Join(d, "brew", "ripgrep")
	writeExec(t, filepath.Join(brewPrefix, "bin", "rg"), "#!/bin/sh\n")
	writeExec(t, filepath.Join(bin, "brew"), "#!/bin/sh\ncase \"$1 $2 $3\" in\n  leaves*) echo ripgrep;;\n  '--prefix ripgrep'*) echo \"$BREW_FIXTURE\";;\n  'list --versions ripgrep') echo 'ripgrep 14.1.0';;\nesac\n")
	writeExec(t, filepath.Join(bin, "mise"), "#!/bin/sh\necho 'node 24.0.0'\n")
	writeExec(t, filepath.Join(bin, "uv"), "#!/bin/sh\nprintf 'black v25.1.0\\n- black\\n- blackd\\n'\n")
	t.Setenv("PATH", bin)
	t.Setenv("BREW_FIXTURE", brewPrefix)
	t.Setenv("XDG_DATA_HOME", filepath.Join(d, "data"))
	writeExec(t, filepath.Join(d, "data", "mise", "installs", "node", "24.0.0", "bin", "node"), "#!/bin/sh\n")
	b, e := discoverBrew(context.Background())
	if e != nil || len(b) != 1 || b[0].Version != "14.1.0" || b[0].Commands[0] != "rg" {
		t.Fatalf("brew %#v %v", b, e)
	}
	m, e := discoverMise(context.Background())
	if e != nil || len(m) != 1 || m[0].Commands[0] != "node" {
		t.Fatalf("mise %#v %v", m, e)
	}
	u, e := discoverUV(context.Background())
	if e != nil || len(u) != 1 || len(u[0].Commands) != 2 || u[0].Commands[1] != "blackd" {
		t.Fatalf("uv %#v %v", u, e)
	}
}

func TestAPTAndPacmanFilteringFixtures(t *testing.T) {
	d := t.TempDir()
	bin := filepath.Join(d, "bin")
	if e := os.MkdirAll(bin, 0700); e != nil {
		t.Fatal(e)
	}
	writeExec(t, filepath.Join(bin, "dpkg-query"), "#!/bin/sh\nif [ \"$1\" = -S ]; then echo 'ripgrep: /usr/bin/rg'; else printf '14.1 no optional'; fi\n")
	writeExec(t, filepath.Join(bin, "pacman"), "#!/bin/sh\nif [ \"$1\" = -Qo ]; then echo '/usr/bin/rg is owned by ripgrep 14.1'; else printf 'Groups : None\\nInstall Reason : Explicitly installed'; fi\n")
	t.Setenv("PATH", bin)
	t.Setenv("WTF_TEST_ROOT", d)
	pkgs := ExactSystem(context.Background(), "rg", "/usr/bin/rg")
	if len(pkgs) != 2 {
		t.Fatalf("packages %#v", pkgs)
	}
	extended := filepath.Join(d, "var", "lib", "apt", "extended_states")
	if e := os.MkdirAll(filepath.Dir(extended), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(extended, []byte("Package: ripgrep\nAuto-Installed: 1\n"), 0600); e != nil {
		t.Fatal(e)
	}
	pkgs = ExactSystem(context.Background(), "rg", "/usr/bin/rg")
	if len(pkgs) != 1 || pkgs[0].Provider != "pacman" {
		t.Fatalf("automatic apt not filtered: %#v", pkgs)
	}
}

func writeExec(t *testing.T, path, body string) {
	t.Helper()
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(path, []byte(body), 0700); e != nil {
		t.Fatal(e)
	}
}

func TestCacheLifecycleAndStaleFallback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.json")
	var calls atomic.Int32
	fp := "one"
	fail := false
	p := Provider{Name: "fixture", Fingerprint: func() string { return fp }, Discover: func(context.Context) ([]Package, error) {
		calls.Add(1)
		if fail {
			return nil, errors.New("offline")
		}
		return []Package{{Provider: "fixture", Name: "ripgrep", Commands: []string{"rg"}}}, nil
	}}
	c, s, e := Load(context.Background(), path, false, []Provider{p})
	if e != nil || calls.Load() != 1 || len(c.Providers["fixture"].Packages) != 1 || !s[0].Changed {
		t.Fatalf("cold %#v %#v %v", c, s, e)
	}
	_, s, e = Load(context.Background(), path, false, []Provider{p})
	if e != nil || calls.Load() != 1 || s[0].Changed {
		t.Fatalf("fresh %#v %v", s, e)
	}
	fp = "two"
	fail = true
	c, _, e = Load(context.Background(), path, false, []Provider{p})
	if e != nil || !c.Providers["fixture"].Stale || len(c.Providers["fixture"].Packages) != 1 {
		t.Fatalf("stale %#v %v", c, e)
	}
}
func TestCorruptExpiredAndConcurrentCache(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.json")
	if e := os.WriteFile(path, []byte("{"), 0600); e != nil {
		t.Fatal(e)
	}
	var n atomic.Int32
	p := Provider{Name: "x", Fingerprint: func() string { return "f" }, Discover: func(context.Context) ([]Package, error) { n.Add(1); return nil, nil }}
	if _, _, e := Load(context.Background(), path, false, []Provider{p}); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, e := Load(context.Background(), path, true, []Provider{p}); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if n.Load() != 9 {
		t.Fatalf("discover count %d", n.Load())
	}
	if _, e := os.Stat(path); e != nil {
		t.Fatal(e)
	}
}
func TestExpiredRefresh(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.json")
	old := Cache{Schema: Schema, Providers: map[string]ProviderState{"x": {Name: "x", Fingerprint: "f", RefreshedAt: time.Now().Add(-25 * time.Hour)}}}
	if e := writeAtomic(path, old); e != nil {
		t.Fatal(e)
	}
	called := false
	p := Provider{Name: "x", Fingerprint: func() string { return "f" }, Discover: func(context.Context) ([]Package, error) { called = true; return nil, nil }}
	_, _, e := Load(context.Background(), path, false, []Provider{p})
	if e != nil || !called {
		t.Fatalf("called %v error %v", called, e)
	}
}
