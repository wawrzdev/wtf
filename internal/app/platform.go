package app

import (
	"io"
	"os"
	"path/filepath"
)

func isTerminalWriter(w io.Writer) bool { f, ok := w.(*os.File); return ok && isTerminalFile(f) }

func envPath() string             { return os.Getenv("PATH") }
func stringPathSep() string       { return string(os.PathListSeparator) }
func joinPath(a, b string) string { return filepath.Join(a, b) }
func executable(p string) bool {
	i, e := os.Stat(p)
	return e == nil && !i.IsDir() && i.Mode()&0111 != 0
}
