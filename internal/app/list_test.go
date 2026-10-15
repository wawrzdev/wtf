package app

import (
	"bytes"
	"strings"
	"testing"

	"github.com/wawrzdev/wtf/internal/inventory"
)

func TestListTableGroupsByKindAndTruncates(t *testing.T) {
	records := []Record{
		{ID: "ll", Kind: "alias", Summary: "long list"},
		{ID: "mkcd", Kind: "func", Summary: "make and enter a directory"},
		{ID: "act", Kind: "command", Available: true, Summary: "Run GitHub Actions workflows locally"},
		{ID: "gone", Kind: "command", Summary: "Missing"},
		{ID: "history", Kind: "topic", Summary: "Shell history"},
	}
	var b bytes.Buffer
	writeListTable(&b, records, 30, false)
	want := "Aliases\n  ll       long list\n\nFunctions\n  mkcd     make and enter a d…\n\n" +
		"Commands\n  act      Run GitHub Actions…\n  gone     Missing (not insta…\n\nTopics\n  history  Shell history\n"
	if b.String() != want {
		t.Fatalf("got:\n%s\nwant:\n%s", b.String(), want)
	}
}

func TestListFoldsMultiCommandPackages(t *testing.T) {
	pq := []inventory.Package{{Provider: "homebrew", Name: "libpq", Commands: []string{"createdb", "pg_dump", "psql", "dropdb", "pg_restore"}}}
	nm := []inventory.Package{{Provider: "homebrew", Name: "nmap", Commands: []string{"ncat", "nmap"}}}
	records := []Record{
		{ID: "createdb", Kind: "command", Available: true, Packages: pq},
		{ID: "dropdb", Kind: "command", Available: true, Packages: pq},
		{ID: "jq", Kind: "command", Available: true, Summary: "JSON processor",
			Packages: []inventory.Package{{Provider: "homebrew", Name: "jq", Commands: []string{"jq"}}}},
		{ID: "ncat", Kind: "command", Available: true, Packages: nm},
		{ID: "nmap", Kind: "command", Available: true, Summary: "Scan networks", Packages: nm},
		{ID: "pg_dump", Kind: "command", Available: true, Packages: pq},
		{ID: "pg_restore", Kind: "command", Available: true, Packages: pq},
		{ID: "psql", Kind: "command", Available: true, Summary: "PostgreSQL client", Packages: pq},
	}
	var b bytes.Buffer
	writeListTable(&b, linkPackages(records), 200, false)
	want := "Commands\n  jq     JSON processor\n  nmap   Scan networks · ncat\n\nGroups\n  libpq  PostgreSQL client · psql, createdb, dropdb +2\n"
	if b.String() != want {
		t.Fatalf("got:\n%s\nwant:\n%s", b.String(), want)
	}
}

func TestListFoldsChildrenIntoParentAndListsGroups(t *testing.T) {
	records := []Record{
		{ID: "docker", Kind: "command", Available: true, Summary: "Containers"},
		{ID: "docker-compose", Kind: "command", Available: true, Parent: "docker"},
		{ID: "gopls", Kind: "command", Available: true, Parent: "lsp"},
		{ID: "orphan", Kind: "command", Available: true, Parent: "missing", Summary: "Shown alone"},
		{ID: "lsp", Kind: "group", Summary: "Language servers"},
	}
	var b bytes.Buffer
	writeListTable(&b, linkPackages(records), 200, false)
	want := "Commands\n  docker  Containers · docker-compose\n  orphan  Shown alone\n\nGroups\n  lsp     Language servers · gopls\n"
	if b.String() != want {
		t.Fatalf("got:\n%s\nwant:\n%s", b.String(), want)
	}
}

func TestLinkPackagesCreatesGroupsAndParents(t *testing.T) {
	pq := []inventory.Package{{Provider: "homebrew", Name: "libpq", Commands: []string{"psql", "pg_dump"}}}
	node := []inventory.Package{{Provider: "mise", Name: "node", Commands: []string{"node", "npm"}}}
	got := linkPackages([]Record{
		{ID: "pg_dump", Kind: "command", Packages: pq},
		{ID: "psql", Kind: "command", Summary: "client", Packages: pq},
		{ID: "node", Kind: "command", Packages: node},
		{ID: "npm", Kind: "command", Packages: node},
		{ID: "docker-compose", Kind: "command", Parent: "docker", Packages: []inventory.Package{{Provider: "homebrew", Name: "docker-compose", Commands: []string{"docker-compose"}}}},
		{ID: "docker", Kind: "command"},
	})
	ids := map[string]Record{}
	for _, x := range got {
		ids[x.ID] = x
	}
	if g := ids["libpq"]; g.Kind != "group" || g.Summary != "client" || strings.Join(g.Members, ",") != "psql,pg_dump" {
		t.Fatalf("group %+v", g)
	}
	if ids["pg_dump"].Parent != "libpq" || ids["npm"].Parent != "node" || ids["node"].Parent != "" {
		t.Fatalf("parents %q %q %q", ids["pg_dump"].Parent, ids["npm"].Parent, ids["node"].Parent)
	}
	if strings.Join(ids["docker"].Members, ",") != "docker-compose" {
		t.Fatalf("declared parent lost: %+v", ids["docker"])
	}
}
