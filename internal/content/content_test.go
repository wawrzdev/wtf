package content

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInlineAndOverlay(t *testing.T) {
	d := t.TempDir()
	a := filepath.Join(d, "a")
	b := filepath.Join(d, "b.md")
	write(t, a, "# @tool rg — fast search\n# @detail rg\n#   rg thing\n#   rg -t go thing\n")
	write(t, b, "---\nschema: 1\nid: rg\nkind: command\nsummary: Better summary\n---\n\n# rg\n\nBody.\n")
	anns, _, errs := Load([]string{a, b}, nil)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	x := anns["rg"]
	if x.Summary != "Better summary" || !strings.Contains(x.Body, "Body.") || len(x.Sources) != 3 {
		t.Fatalf("entry %#v", x)
	}
}
func TestPartialMarkdownOverlayInheritsFields(t *testing.T) {
	d := t.TempDir()
	low := filepath.Join(d, "low.md")
	high := filepath.Join(d, "high.md")
	write(t, low, "---\nschema: 1\nid: rg\nkind: command\nsummary: Low\n---\n\nLow body\n")
	write(t, high, "---\nid: rg\nsummary: High\n---\n")
	anns, _, errs := Load([]string{low, high}, nil)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	x := anns["rg"]
	if x.Schema != 1 || x.Kind != "command" || x.Summary != "High" || x.Body != "Low body" {
		t.Fatalf("overlay %#v", x)
	}
}
func TestOverlayCanClearFrontmatterField(t *testing.T) {
	d := t.TempDir()
	low := filepath.Join(d, "low.md")
	high := filepath.Join(d, "high.md")
	write(t, low, "---\nschema: 1\nid: rg\nkind: command\nsummary: Low\n---\n")
	write(t, high, "---\nid: rg\nsummary: \"\"\n---\n")
	anns, _, errs := Load([]string{low, high}, nil)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if anns["rg"].Summary != "" {
		t.Fatalf("summary not cleared: %#v", anns["rg"])
	}
}
func TestTopicAndMalformed(t *testing.T) {
	d := t.TempDir()
	good := filepath.Join(d, "good.md")
	bad := filepath.Join(d, "bad.md")
	write(t, good, "---\nschema: 1\nid: staging\nkind: topic\ntitle: Git staging\nsummary: Index\n---\n\n# Git staging\n")
	write(t, bad, "---\nschema: nope\nid: bad\nkind: topic\n---\n")
	_, topics, errs := Load(nil, []string{d})
	if topics["staging"].Title != "Git staging" {
		t.Fatal(topics)
	}
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), bad) {
		t.Fatalf("errs %v", errs)
	}
}
func TestSlug(t *testing.T) {
	cases := map[string]string{"Git staging": "git-staging", "  Hello, Wörld!  ": "hello-w-rld", "---": ""}
	for in, want := range cases {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q)=%q want %q", in, got, want)
		}
	}
}
func write(t *testing.T, p, s string) {
	t.Helper()
	if e := os.WriteFile(p, []byte(s), 0600); e != nil {
		t.Fatal(e)
	}
}

func TestInlineSummaryDropsPackageTags(t *testing.T) {
	d := t.TempDir()
	a := d + "/tags"
	write(t, a, "# @tool rg — faster grep (ripgrep) @brew ripgrep\n# @tool lsp — language servers @brew pyright gopls\n")
	got, _, errs := Load([]string{a}, nil)
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	if got["rg"].Summary != "faster grep (ripgrep)" || got["lsp"].Summary != "language servers" {
		t.Fatalf("%q %q", got["rg"].Summary, got["lsp"].Summary)
	}
}

func TestParentFieldAndGroupKind(t *testing.T) {
	d := t.TempDir()
	write(t, d+"/compose.md", "---\nschema: 1\nid: docker-compose\nkind: command\nparent: docker\nsummary: Compose plugin\n---\n")
	write(t, d+"/inline", "# @group lsp — language servers\n")
	write(t, d+"/self.md", "---\nschema: 1\nid: loop\nkind: command\nparent: loop\nsummary: x\n---\n")
	got, _, errs := Load([]string{d}, nil)
	if got["docker-compose"].Parent != "docker" || got["lsp"].Kind != "group" {
		t.Fatalf("%+v %+v", got["docker-compose"], got["lsp"])
	}
	if _, ok := got["loop"]; ok || len(errs) != 1 {
		t.Fatalf("self-parent accepted: %v", errs)
	}
}

func TestInlineDetailKeepsRelativeIndentation(t *testing.T) {
	d := t.TempDir()
	write(t, d+"/x", "# @tool up — keys\n# @detail up\n#   Up / Down    prefix search: type a few chars,\n#                then Up shows matches\n#\n#   PageUp       step\n")
	got, _, _ := Load([]string{d + "/x"}, nil)
	want := "Up / Down    prefix search: type a few chars,\n             then Up shows matches\n\nPageUp       step"
	if got["up"].Body != want {
		t.Fatalf("%q", got["up"].Body)
	}
}
