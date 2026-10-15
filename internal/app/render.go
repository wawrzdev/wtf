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
		cmd.Env = pagerEnv(os.Environ())
		if e := cmd.Run(); e == nil {
			return nil
		} else {
			fmt.Fprintf(errOut, "wtf: mdcat failed, using plain output: %v\n", e)
		}
	}
	plain := plainMarkdown(markdown)
	if pager == "" && os.Getenv("PAGER") == "" {
		if _, e := exec.LookPath("less"); e == nil {
			pager = defaultPager
		}
	}
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

// renderPreview formats docs for the picker's preview pane: mdcat with ANSI styling
// at the pane width and no pager, or plain text when mdcat is unavailable or fails.
func renderPreview(ctx context.Context, out io.Writer, markdown string) error {
	if mdcat, e := exec.LookPath("mdcat"); e == nil {
		args := []string{"--ansi", "--local"}
		if cols := os.Getenv("FZF_PREVIEW_COLUMNS"); cols != "" {
			args = append(args, "--columns", cols)
		}
		var b strings.Builder
		cmd := exec.CommandContext(ctx, mdcat, append(args, "-")...)
		cmd.Stdin = strings.NewReader(markdown)
		cmd.Stdout = &b
		cmd.Env = os.Environ()
		if cmd.Run() == nil {
			_, e := io.WriteString(out, b.String())
			return e
		}
	}
	_, e := io.WriteString(out, plainMarkdown(markdown))
	return e
}

// defaultPager pages only output longer than the screen (-F), keeps colors (-R) and
// leaves short output on screen (-X).
const defaultPager = "less -FRX"

// pagerEnv gives mdcat the default pager unless the user chose one.
func pagerEnv(env []string) []string {
	for _, item := range env {
		if strings.HasPrefix(item, "MDCAT_PAGER=") || (strings.HasPrefix(item, "PAGER=") && item != "PAGER=") {
			return env
		}
	}
	return append(env, "MDCAT_PAGER="+defaultPager)
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
