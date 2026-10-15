package app

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

var kindHeadings = map[string]string{"alias": "Aliases", "func": "Functions", "command": "Commands", "group": "Groups", "topic": "Topics"}

// outputWidth reports the terminal width for w, falling back to $COLUMNS and then 100.
func outputWidth(w io.Writer) int {
	if f, ok := w.(*os.File); ok {
		if n := terminalColumns(f); n > 0 {
			return n
		}
	}
	if n, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && n > 0 {
		return n
	}
	return 100
}

type listRow struct{ kind, name, summary string }

// listRows shows each record whose parent is not listed, with its members (package
// extras or declared children) summarized after its own description.
func listRows(records []Record) []listRow {
	listed := map[string]bool{}
	for _, x := range records {
		listed[x.ID] = true
	}
	var rows []listRow
	for _, x := range records {
		if x.Parent != "" && listed[x.Parent] {
			continue
		}
		summary := x.Summary
		if !x.Available && x.Kind == "command" {
			summary = strings.TrimSpace(summary + " (not installed)")
		}
		var shown []string
		for _, m := range x.Members {
			if listed[m] {
				shown = append(shown, m)
			}
		}
		if len(shown) > 0 {
			list := strings.Join(shown, ", ")
			if len(shown) > 3 {
				list = strings.Join(shown[:3], ", ") + fmt.Sprintf(" +%d", len(shown)-3)
			}
			if summary == "" {
				summary = list
			} else {
				summary += " · " + list
			}
		}
		rows = append(rows, listRow{x.Kind, x.ID, summary})
	}
	sortRows(rows)
	return rows
}

func sortRows(rows []listRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		if a, b := kindRank(rows[i].kind), kindRank(rows[j].kind); a != b {
			return a < b
		}
		return rows[i].name < rows[j].name
	})
}

// writeListTable renders records (already ordered by kind) as headed groups of
// name and summary, truncated to width. Redirected output keeps the TSV format.
func writeListTable(w io.Writer, records []Record, width int, bold bool) {
	rows := listRows(records)
	nameWidth := 0
	for _, x := range rows {
		if n := utf8.RuneCountInString(x.name); n > nameWidth {
			nameWidth = n
		}
	}
	if nameWidth > 24 {
		nameWidth = 24
	}
	kind := ""
	for _, x := range rows {
		if x.kind != kind {
			if kind != "" {
				fmt.Fprintln(w)
			}
			kind = x.kind
			heading := kindHeadings[kind]
			if heading == "" {
				heading = strings.ToUpper(kind[:1]) + kind[1:]
			}
			if bold {
				heading = "\x1b[1m" + heading + "\x1b[0m"
			}
			fmt.Fprintln(w, heading)
		}
		summary := sanitizeDisplay(sanitizeTSV(x.summary))
		line := fmt.Sprintf("  %-*s  %s", nameWidth, sanitizeDisplay(x.name), summary)
		fmt.Fprintln(w, strings.TrimRight(truncateRunes(line, width), " "))
	}
}

func truncateRunes(s string, width int) string {
	if width <= 1 || utf8.RuneCountInString(s) <= width {
		return s
	}
	r := []rune(s)
	return string(r[:width-1]) + "…"
}
