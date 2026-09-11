package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wawrzdev/wtf/internal/app"
)

func main() {
	out := flag.String("out", "completions", "output directory")
	flag.Parse()
	if err := os.MkdirAll(*out, 0755); err != nil {
		fatal(err)
	}
	for _, shell := range []string{"bash", "zsh", "fish"} {
		script, err := app.CompletionScript(shell)
		if err != nil {
			fatal(err)
		}
		path := filepath.Join(*out, "wtf."+shell)
		if err := os.WriteFile(path, []byte(script), 0644); err != nil {
			fatal(err)
		}
	}
}

func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
