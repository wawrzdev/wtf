package inventory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// PathOwnership uses local receipts only; an unrecognized executable is unknown,
// rather than assumed to belong to the OS. No executable is run during discovery.
func PathOwnership(command, path string) (string, []Package) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "unknown", nil
	}
	for _, root := range brewRoots() {
		if filepath.Base(root) == "Caskroom" {
			continue
		}
		root, err = filepath.EvalSymlinks(root)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(root, resolved)
		if err != nil || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		parts := strings.Split(rel, string(filepath.Separator))
		if len(parts) < 4 {
			continue
		}
		receipt, err := os.ReadFile(filepath.Join(root, parts[0], parts[1], "INSTALL_RECEIPT.json"))
		if err != nil {
			continue
		}
		var data struct {
			OnRequest *bool `json:"installed_on_request"`
		}
		if json.Unmarshal(receipt, &data) != nil || data.OnRequest == nil {
			continue
		}
		origin := "dependency"
		if *data.OnRequest {
			origin = "package"
		}
		return origin, []Package{{Provider: "homebrew", Name: parts[0], Version: parts[1], Commands: []string{command}}}
	}
	// These directories are OS-owned on macOS. Linux /usr/bin also contains user
	// packages, so leave it unknown until a native package receipt is consulted.
	if runtime.GOOS == "darwin" {
		for _, root := range []string{"/usr/bin/", "/usr/sbin/", "/bin/", "/sbin/", "/System/"} {
			if strings.HasPrefix(resolved, root) {
				return "system", nil
			}
		}
	}
	return "unknown", nil
}
