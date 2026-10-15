package inventory

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPathOwnershipRequiresReceiptAndFollowsExecutableSymlink(t *testing.T) {
	d := t.TempDir()
	cellar := filepath.Join(d, "Cellar")
	t.Setenv("HOMEBREW_CELLAR", cellar)
	target := filepath.Join(cellar, "gettext", "1.0", "bin", "envsubst")
	writeExec(t, target, "#!/bin/sh\n")
	link := filepath.Join(d, "envsubst")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	receipt := filepath.Join(cellar, "gettext", "1.0", "INSTALL_RECEIPT.json")
	for _, tc := range []struct{ body, want string }{
		{`{}`, "unknown"}, {`invalid`, "unknown"},
		{`{"installed_on_request":false}`, "dependency"},
		{`{"installed_on_request":true}`, "package"},
	} {
		if err := os.WriteFile(receipt, []byte(tc.body), 0600); err != nil {
			t.Fatal(err)
		}
		origin, pkgs := PathOwnership("envsubst", link)
		if origin != tc.want {
			t.Fatalf("%s: %s", tc.body, origin)
		}
		if origin != "unknown" && (len(pkgs) != 1 || pkgs[0].Name != "gettext" || pkgs[0].Version != "1.0") {
			t.Fatalf("packages: %#v", pkgs)
		}
	}
	if err := os.Remove(receipt); err != nil {
		t.Fatal(err)
	}
	if origin, _ := PathOwnership("envsubst", link); origin != "unknown" {
		t.Fatal(origin)
	}
}
