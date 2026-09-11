package app

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

func render(ctx context.Context, out, errOut io.Writer, markdown, pager string) error {
	if !isTerminalWriter(out) {
		_, e := io.WriteString(out, markdown)
		return e
	}
	if mdcat, e := exec.LookPath("mdcat"); e == nil {
		cmd := exec.CommandContext(ctx, mdcat, "--local", "--paginate", "-")
		cmd.Stdin = strings.NewReader(markdown)
		cmd.Stdout = out
		cmd.Stderr = errOut
		cmd.Env = os.Environ()
		if e := cmd.Run(); e == nil {
			return nil
		} else {
			fmt.Fprintf(errOut, "wtf: mdcat failed, using plain output: %v\n", e)
		}
	}
	plain := plainMarkdown(markdown)
	if pager != "" {
		parts := strings.Fields(pager)
		if len(parts) > 0 {
			if p, e := exec.LookPath(parts[0]); e == nil {
				cmd := exec.CommandContext(ctx, p, parts[1:]...)
				cmd.Stdin = strings.NewReader(plain)
				cmd.Stdout = out
				cmd.Stderr = errOut
				if e := cmd.Run(); e == nil {
					return nil
				} else {
					fmt.Fprintf(errOut, "wtf: pager failed, using plain output: %v\n", e)
				}
			}
		}
	}
	_, e := io.WriteString(out, plain)
	return e
}
func plainMarkdown(s string) string {
	var b strings.Builder
	scan := bufio.NewScanner(strings.NewReader(s))
	fence := false
	for scan.Scan() {
		line := scan.Text()
		if strings.HasPrefix(line, "```") {
			fence = !fence
			continue
		}
		if !fence {
			line = strings.TrimPrefix(line, "### ")
			line = strings.TrimPrefix(line, "## ")
			line = strings.TrimPrefix(line, "# ")
			line = strings.ReplaceAll(line, "`", "")
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}
