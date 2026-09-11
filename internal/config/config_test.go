package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFragmentsAndExpansion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	dir := filepath.Join(home, "config", "wtf", "conf.d")
	mustWrite(t, filepath.Join(home, "config", "wtf", "config.toml"), `[annotations]
paths = ["~/base"]
[topics]
dirs = ["$XDG_DATA_HOME/wtf/topics"]
write_dir = "~/write"
post_write = []
[providers.cheat]
enabled = true
`)
	mustWrite(t, filepath.Join(dir, "late.toml"), `priority = 20
[annotations]
paths = ["~/late"]
`)
	mustWrite(t, filepath.Join(dir, "early.toml"), `priority = 10
[providers.cheat]
enabled = false
`)
	c, e := Load()
	if e != nil {
		t.Fatal(e)
	}
	if got := c.Annotations.Paths[0]; got != filepath.Join(home, "late") {
		t.Fatalf("path %q", got)
	}
	if c.Cheat.Enabled {
		t.Fatal("cheat should be disabled")
	}
	if len(c.Sources) != 3 {
		t.Fatalf("sources %#v", c.Sources)
	}
}
func TestDuplicatePriority(t *testing.T) {
	home := isolated(t)
	dir := filepath.Join(home, "config", "wtf", "conf.d")
	mustWrite(t, filepath.Join(dir, "a.toml"), "priority = 4\n")
	mustWrite(t, filepath.Join(dir, "b.toml"), "priority = 4\n")
	_, e := Load()
	if e == nil || !strings.Contains(e.Error(), "duplicate fragment priority 4") {
		t.Fatalf("error %v", e)
	}
}
func TestInvalidNamesSource(t *testing.T) {
	home := isolated(t)
	p := filepath.Join(home, "config", "wtf", "config.toml")
	mustWrite(t, p, "mystery = true\n")
	_, e := Load()
	if e == nil || !strings.Contains(e.Error(), p) {
		t.Fatalf("error %v", e)
	}
}
func TestMissingDirectoriesAreNormal(t *testing.T) {
	isolated(t)
	if _, e := Load(); e != nil {
		t.Fatal(e)
	}
}
func isolated(t *testing.T) string {
	t.Helper()
	h := t.TempDir()
	t.Setenv("HOME", h)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(h, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(h, "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(h, "cache"))
	return h
}
func mustWrite(t *testing.T, p, s string) {
	t.Helper()
	if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(p, []byte(s), 0600); e != nil {
		t.Fatal(e)
	}
}
