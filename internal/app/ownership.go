package app

import "strings"

func originLabel(x Record) string {
	label := x.Origin
	if len(x.Packages) > 0 && (label == "dependency" || label == "package") {
		p := x.Packages[0]
		label += " · " + p.Provider + "/" + p.Name
	}
	return label
}

func labeledSummary(x Record) string {
	if label := originLabel(x); label != "" {
		return strings.TrimSpace("[" + label + "] " + x.Summary)
	}
	return x.Summary
}
