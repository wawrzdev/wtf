package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckedCompletionsMatchRuntimeOutput(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish"} {
		want, err := CompletionScript(shell)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join("..", "..", "completions", "wtf."+shell)
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Errorf("%s is stale; run go generate ./...", path)
		}
	}
}
