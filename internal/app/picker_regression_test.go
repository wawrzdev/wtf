package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wawrzdev/wtf/internal/inventory"
)

// Exercise the actual two-stage picker protocol without a TTY. The fake fzf
// captures each input and selects a requested row; it never runs the preview.
func pickerFixture(t *testing.T, home, group, member string) string {
	t.Helper()
	bin := filepath.Join(home, "picker-bin")
	capture := filepath.Join(home, "member-rows")
	t.Setenv("PICKER_STATE", filepath.Join(home, "picker-state"))
	t.Setenv("PICKER_MEMBERS", capture)
	t.Setenv("PICKER_GROUP", group)
	t.Setenv("PICKER_MEMBER", member)
	mustFile(t, filepath.Join(bin, "fzf"), `#!/bin/sh
if [ ! -f "$PICKER_STATE" ]; then
 : > "$PICKER_STATE"
 /bin/cat > /dev/null
 printf '%s\tselected\n' "$PICKER_GROUP"
else
 /bin/cat > "$PICKER_MEMBERS"
 printf '%s\tselected\n' "$PICKER_MEMBER"
fi
`, 0700)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return capture
}

func TestPickerDrillInKeepsUninstalledMembersHidden(t *testing.T) {
	for _, all := range []bool{false, true} {
		name := "default"
		if all {
			name = "all"
		}
		t.Run(name, func(t *testing.T) {
			home := setup(t)
			capture := pickerFixture(t, home, "review-group", "installed")
			records := []Record{
				{ID: "review-group", Kind: "group", Members: []string{"installed", "missing", "shell-alias"}},
				{ID: "installed", Kind: "command", Available: true, Parent: "review-group"},
				{ID: "missing", Kind: "command", Parent: "review-group"},
				{ID: "shell-alias", Kind: "alias", Parent: "review-group"},
			}
			var out, errs bytes.Buffer
			r := runtime{records: records, stdout: &out, stderr: &errs, all: all}
			if err := r.picker(context.Background(), r.candidates(), ""); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(capture)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "missing") {
				t.Fatalf("uninstalled member leaked into picker: %s", data)
			}
			for _, id := range []string{"installed", "shell-alias"} {
				if !strings.Contains(string(data), id+"\t") {
					t.Fatalf("visible member %s missing: %s", id, data)
				}
			}
		})
	}
}

func TestPickerCanOpenCaskGroupWithSpacesInPackageName(t *testing.T) {
	home := setup(t)
	capture := pickerFixture(t, home, "visual-studio-code", "code")
	packages := []inventory.Package{{Provider: "homebrew", Name: "visual studio code", Commands: []string{"code", "code-tunnel"}}}
	records := linkPackages([]Record{
		{ID: "code", Kind: "command", Available: true, Packages: packages},
		{ID: "code-tunnel", Kind: "command", Available: true, Packages: packages},
	})
	rows := listRows(records)
	if len(rows) != 1 || rows[0].name != "visual-studio-code" || !safeCommandID(rows[0].name) {
		t.Fatalf("cask group is not a usable picker ID: %#v", rows)
	}
	var out, errs bytes.Buffer
	r := runtime{records: records, stdout: &out, stderr: &errs}
	if err := r.picker(context.Background(), r.candidates(), ""); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"code", "code-tunnel"} {
		if !strings.Contains(string(data), id+"\t") {
			t.Fatalf("cask command %s hidden: %s", id, data)
		}
	}
	if !strings.Contains(out.String(), "# code\n") {
		t.Fatalf("selected command not documented: %s", out.String())
	}
}

func TestUnsupportedPackageGroupIDLeavesCommandsVisible(t *testing.T) {
	packages := []inventory.Package{{Provider: "fixture", Name: "invalid:group", Commands: []string{"first", "second"}}}
	records := linkPackages([]Record{
		{ID: "first", Kind: "command", Available: true, Packages: packages},
		{ID: "second", Kind: "command", Available: true, Packages: packages},
	})
	rows := listRows(records)
	if len(rows) != 2 || rows[0].name != "first" || rows[1].name != "second" {
		t.Fatalf("invalid group hid commands: %#v", rows)
	}
}
