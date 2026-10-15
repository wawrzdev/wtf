package app

import (
	"context"
	"github.com/wawrzdev/wtf/internal/inventory"
	"strings"
	"testing"
)

func TestOwnershipLabelsPreserveDescriptionsAndDefaultRows(t *testing.T) {
	dep := Record{ID: "envsubst", Kind: "command", Available: true, Summary: "Substitute variables", Origin: "dependency", Packages: []inventory.Package{{Provider: "homebrew", Name: "gettext"}}}
	if got := listRows([]Record{dep})[0].summary; got != "[dependency · homebrew/gettext] Substitute variables" {
		t.Fatal(got)
	}
	if got := labeledSummary(Record{Origin: "unknown"}); got != "[unknown]" {
		t.Fatal(got)
	}
	if got := labeledSummary(Record{Origin: "system"}); got != "[system]" {
		t.Fatal(got)
	}
	if got := labeledSummary(Record{Summary: "Existing row"}); got != "Existing row" {
		t.Fatal(got)
	}
	page := compose(context.Background(), dep, docOptions{LocalOnly: true})
	if !strings.Contains(page, "- Origin: dependency · homebrew/gettext") {
		t.Fatal(page)
	}
}
